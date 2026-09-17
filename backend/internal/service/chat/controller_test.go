package chat_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	chatsvc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/chat"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/store"
)

// These run against a real SQLite store rather than a mock, because the point is
// that provider events actually land as durable rows in the right order — which a
// mock store cannot demonstrate.

const (
	testProject = domain.ProjectID("p1")
	testSession = domain.SessionID("p1-1")
)

func openStore(t *testing.T) *sqlite.Store {
	t.Helper()
	dir := t.TempDir()
	st := sqlitetest.MustOpenAt(t, dir)

	ctx := context.Background()
	if err := st.UpsertProject(ctx, domain.ProjectRecord{
		ID:           string(testProject),
		Path:         dir,
		RegisteredAt: time.Now().UTC().Truncate(time.Second),
	}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := st.CreateSession(ctx, domain.SessionRecord{
		ID:        testSession,
		ProjectID: testProject,
		Kind:      domain.KindOrchestrator,
		Harness:   domain.HarnessCodex,
		Mode:      domain.SessionModeChat,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return st
}

/* ---- a fake conversation the controller can drive ---------------------- */

type fakeConversation struct {
	events                 chan ports.ChatEvent
	providerConversationID string

	mu        sync.Mutex
	sent      []ports.ChatUserMessage
	resolved  map[string]ports.ChatDecision
	turnSeq   int
	sendErr   error
	onSend    func(providerTurnID string)
	closeOnce sync.Once
}

type nativeHistoryConversation struct {
	*fakeConversation
	events []ports.ChatEvent
	err    error
}

type blockingHistoryConversation struct {
	*fakeConversation
	started chan struct{}
	release chan struct{}
}

func (c *blockingHistoryConversation) ReadHistory(ctx context.Context) ([]ports.ChatEvent, error) {
	close(c.started)
	select {
	case <-c.release:
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *nativeHistoryConversation) ReadHistory(context.Context) ([]ports.ChatEvent, error) {
	return c.events, c.err
}

type deferredConversation struct {
	*fakeConversation
	start func(string) error
}

type stuckConversation struct {
	*fakeConversation
	closeErr error
}

func (s *stuckConversation) Close() error { return s.closeErr }

func (f *deferredConversation) StartDeferredTurn(providerTurnID string) error {
	return f.start(providerTurnID)
}

func (f *deferredConversation) DiscardDeferredTurn(string) {}

func newFakeConversation() *fakeConversation {
	return &fakeConversation{
		events:                 make(chan ports.ChatEvent, 64),
		providerConversationID: "thread-1",
		resolved:               map[string]ports.ChatDecision{},
	}
}

func (f *fakeConversation) ProviderConversationID() string       { return f.providerConversationID }
func (f *fakeConversation) Capabilities() ports.ChatCapabilities { return productionCaps() }
func (f *fakeConversation) Events() <-chan ports.ChatEvent       { return f.events }

func (f *fakeConversation) SendTurn(_ context.Context, msg ports.ChatUserMessage) (ports.ChatTurnRef, error) {
	f.mu.Lock()
	if f.sendErr != nil {
		f.mu.Unlock()
		return ports.ChatTurnRef{}, f.sendErr
	}
	f.sent = append(f.sent, msg)
	f.turnSeq++
	providerTurnID := fmt.Sprintf("provider-turn-%d", f.turnSeq)
	onSend := f.onSend
	f.mu.Unlock()
	if onSend != nil {
		onSend(providerTurnID)
	}
	return ports.ChatTurnRef{ProviderTurnID: providerTurnID}, nil
}

// sentTexts is what actually reached the provider, in order. Queuing is only real
// if a message the user typed mid-turn is absent from this until the turn ends.
func (f *fakeConversation) sentTexts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	texts := make([]string, 0, len(f.sent))
	for _, msg := range f.sent {
		texts = append(texts, msg.Text)
	}
	return texts
}

func (f *fakeConversation) sentMessages() []ports.ChatUserMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ports.ChatUserMessage(nil), f.sent...)
}

func (f *fakeConversation) Interrupt(context.Context, string) error { return nil }

func (f *fakeConversation) ResolveRequest(_ context.Context, id string, d ports.ChatDecision) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolved[id] = d
	return nil
}

type answerDispatchConversation struct {
	*fakeConversation
	dispatch   ports.ChatAnswerDispatch
	err        error
	calls      int
	generation string
	onDispatch func()
}

func (c *answerDispatchConversation) DispatchAnswer(_ context.Context, requestID, generation string, decision ports.ChatDecision) (ports.ChatAnswerDispatch, error) {
	c.calls++
	c.generation = generation
	if c.onDispatch != nil {
		c.onDispatch()
	}
	return c.dispatch, c.err
}

func (f *fakeConversation) Close() error {
	f.closeOnce.Do(func() { close(f.events) })
	return nil
}

func (f *fakeConversation) emit(events ...ports.ChatEvent) {
	for _, event := range events {
		f.events <- event
	}
}

func (f *fakeConversation) decisionFor(id string) (ports.ChatDecision, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.resolved[id]
	return d, ok
}

// fakeDriver hands back whatever conversation double the test supplied, so a
// scenario can replace how the provider ANSWERS without reimplementing how it
// streams.
type fakeDriver struct {
	conv      ports.ChatConversation
	startCfg  *ports.ChatStartConfig
	resumeCfg *ports.ChatResumeConfig
	start     func(ports.ChatStartConfig) (ports.ChatConversation, error)
	resume    func(ports.ChatResumeConfig) (ports.ChatConversation, error)
}

type sequenceDriver struct {
	mu            sync.Mutex
	conversations []ports.ChatConversation
}

func (d *sequenceDriver) Harness() domain.AgentHarness { return domain.HarnessCodex }
func (d *sequenceDriver) Probe(context.Context) (ports.ChatCapabilities, error) {
	return productionCaps(), nil
}
func (d *sequenceDriver) next() (ports.ChatConversation, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.conversations) == 0 {
		return nil, errors.New("no conversation queued")
	}
	conversation := d.conversations[0]
	d.conversations = d.conversations[1:]
	return conversation, nil
}
func (d *sequenceDriver) Start(context.Context, ports.ChatStartConfig) (ports.ChatConversation, error) {
	return d.next()
}
func (d *sequenceDriver) Resume(context.Context, ports.ChatResumeConfig) (ports.ChatConversation, error) {
	return d.next()
}

func (d fakeDriver) Harness() domain.AgentHarness { return domain.HarnessCodex }
func (d fakeDriver) Probe(context.Context) (ports.ChatCapabilities, error) {
	return productionCaps(), nil
}
func (d fakeDriver) Start(_ context.Context, cfg ports.ChatStartConfig) (ports.ChatConversation, error) {
	if d.startCfg != nil {
		*d.startCfg = cfg
	}
	if d.start != nil {
		return d.start(cfg)
	}
	return d.conv, nil
}
func (d fakeDriver) Resume(_ context.Context, cfg ports.ChatResumeConfig) (ports.ChatConversation, error) {
	if d.resumeCfg != nil {
		*d.resumeCfg = cfg
	}
	if d.resume != nil {
		return d.resume(cfg)
	}
	return d.conv, nil
}

type fakeRegistry struct{ driver ports.ChatDriver }

func (r fakeRegistry) Driver(domain.AgentHarness) (ports.ChatDriver, error) { return r.driver, nil }
func (r fakeRegistry) SupportsChat(domain.AgentHarness) bool                { return true }

type recordingActivity struct {
	mu      sync.Mutex
	signals []ports.ActivitySignal
}

func (r *recordingActivity) ApplyActivitySignal(
	_ context.Context,
	_ domain.SessionID,
	signal ports.ActivitySignal,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.signals = append(r.signals, signal)
	return nil
}

func (r *recordingActivity) snapshot() []ports.ActivitySignal {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ports.ActivitySignal(nil), r.signals...)
}

func productionCaps() ports.ChatCapabilities {
	return ports.ChatCapabilities{
		ports.ChatCapabilityStreaming: true,
		ports.ChatCapabilityApprovals: true,
		ports.ChatCapabilityInterrupt: true,
		ports.ChatCapabilityResume:    true,
	}
}

func TestServicePassesRecomputedSystemPromptToResume(t *testing.T) {
	st := openStore(t)
	conv := newFakeConversation()
	var resumed ports.ChatResumeConfig
	svc := chatsvc.New(chatsvc.Options{
		Store: st, Sessions: st,
		Drivers: fakeRegistry{driver: fakeDriver{conv: conv, resumeCfg: &resumed}},
		Log:     slog.New(slog.DiscardHandler),
		NewID:   func() string { return "conversation-resume" },
	})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })

	workspace := t.TempDir()
	dataDir := t.TempDir()
	_, err := svc.Start(context.Background(), chatsvc.StartConfig{
		SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex,
		DataDir: dataDir, WorkspacePath: workspace, ProviderConversationID: "thread-1",
		SystemPrompt: "Recomputed Kennel orchestrator instructions",
	})
	if err != nil {
		t.Fatalf("Start resume: %v", err)
	}
	if resumed.ProviderConversationID != "thread-1" || resumed.DataDir != dataDir || resumed.WorkspacePath != workspace ||
		resumed.SystemPrompt != "Recomputed Kennel orchestrator instructions" {
		t.Fatalf("resume config = %#v", resumed)
	}
}

func TestResumeImportsNativeHistoryBeforeTheChatControllerStarts(t *testing.T) {
	st := openStore(t)
	now := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	existing, err := st.CreateConversation(context.Background(), "existing-conversation",
		domain.ConversationScopeSession, testProject, testSession, now)
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if err := st.ClaimChatControllerGeneration(context.Background(), testSession, "old-generation", "", "", now); err != nil {
		t.Fatalf("ClaimChatControllerGeneration: %v", err)
	}
	created, err := st.AppendUserMessage(context.Background(), existing.ID, testSession, "old-generation",
		domain.ConversationMessage{
			ID: "existing-user", Text: "What changed?", Origin: domain.MessageOriginHuman,
			ClientMessageID: "original-chat-client-id",
		}, "existing-turn", now)
	if err != nil || !created {
		t.Fatalf("AppendUserMessage: created=%v err=%v", created, err)
	}
	if err := st.BindTurnToProvider(context.Background(), "existing-turn", "native-turn-1", now); err != nil {
		t.Fatalf("BindTurnToProvider: %v", err)
	}
	if err := st.SettleAssistantMessage(context.Background(), existing.ID,
		"native-answer-1", "native-turn-1", "Nothing is dirty.", "existing-answer", now); err != nil {
		t.Fatalf("SettleAssistantMessage: %v", err)
	}
	if err := st.UpsertActivity(context.Background(), existing.ID, "native-turn-1",
		domain.ConversationActivity{
			ID: "existing-command", Kind: domain.ActivityKindCommand, Status: domain.ActivityStatusCompleted,
			Summary: "Ran git status", Detail: json.RawMessage(`{"command":"git status"}`), ProviderItemID: "native-command-1",
		}, now); err != nil {
		t.Fatalf("UpsertActivity: %v", err)
	}
	if err := st.SettleTurn(context.Background(), existing.ID, "native-turn-1", domain.TurnStateCompleted, "", now); err != nil {
		t.Fatalf("SettleTurn: %v", err)
	}

	base := newFakeConversation()
	conv := &nativeHistoryConversation{
		fakeConversation: base,
		events: []ports.ChatEvent{
			{Kind: ports.ChatEventTurnStarted, ProviderEventID: "history-start", ProviderTurnID: "native-turn-1"},
			{
				Kind: ports.ChatEventUserMessageCompleted, ProviderEventID: "history-user",
				ProviderTurnID: "native-turn-1", ProviderItemID: "history-item-1",
				ClientMessageID: "native-client-1", Text: "What changed?",
			},
			{
				Kind: ports.ChatEventActivityCompleted, ProviderEventID: "history-command",
				ProviderTurnID: "native-turn-1", ProviderItemID: "history-item-2",
				ActivityKind: domain.ActivityKindCommand, ActivityStatus: domain.ActivityStatusCompleted,
				Summary: "Ran git status", Detail: json.RawMessage(`{"command":"git status"}`),
			},
			{
				Kind: ports.ChatEventActivityCompleted, ProviderEventID: "history-new-command",
				ProviderTurnID: "native-turn-1", ProviderItemID: "history-item-new-command",
				ActivityKind: domain.ActivityKindCommand, ActivityStatus: domain.ActivityStatusCompleted,
				Summary: "Ran git diff", Detail: json.RawMessage(`{"command":"git diff"}`),
			},
			{
				Kind: ports.ChatEventMessageCompleted, ProviderEventID: "history-answer",
				ProviderTurnID: "native-turn-1", ProviderItemID: "history-item-3", Text: "Nothing is dirty.",
			},
			{
				Kind: ports.ChatEventTurnCompleted, ProviderEventID: "history-complete",
				ProviderTurnID: "native-turn-1", TurnState: domain.TurnStateCompleted,
			},
		},
	}

	var idMu sync.Mutex
	nextID := 0
	svc := chatsvc.New(chatsvc.Options{
		Store: st, Sessions: st,
		Reader: chatsvc.SnapshotReaderFunc(func(ctx context.Context, conversationID string) (chatsvc.ConversationRows, error) {
			rows, err := st.LoadConversationSnapshot(ctx, conversationID)
			if err != nil {
				return chatsvc.ConversationRows{}, err
			}
			return chatsvc.ConversationRows{
				Conversation: rows.Conversation,
				Turns:        rows.Turns,
				Messages:     rows.Messages,
				Activities:   rows.Activities,
			}, nil
		}),
		Drivers: fakeRegistry{driver: fakeDriver{conv: conv}},
		Log:     slog.New(slog.DiscardHandler),
		NewID: func() string {
			idMu.Lock()
			defer idMu.Unlock()
			nextID++
			return fmt.Sprintf("history-id-%d", nextID)
		},
	})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })

	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{
		SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex,
		WorkspacePath: t.TempDir(), ProviderConversationID: "thread-1",
	})
	if err != nil {
		t.Fatalf("Start resume: %v", err)
	}
	snapshot, err := st.LoadConversationSnapshot(context.Background(), ctrl.ConversationID())
	if err != nil {
		t.Fatalf("LoadConversationSnapshot: %v", err)
	}
	// The turn already existed from an earlier Chat interval. Codex can omit its
	// persisted item ids, so the replay uses synthetic item ids even though the
	// live assistant message used native-answer-1. Stable turn identity and the
	// settled content keep the replay from duplicating either message, while the
	// command Kennel already knew is deduplicated too, while the new command that Kennel
	// had not seen yet is still imported.
	if len(snapshot.Messages) != 2 || snapshot.Messages[0].Text != "What changed?" || snapshot.Messages[1].Text != "Nothing is dirty." {
		t.Fatalf("imported messages = %#v", snapshot.Messages)
	}
	if len(snapshot.Activities) != 2 || snapshot.Activities[0].Summary != "Ran git status" || snapshot.Activities[1].Summary != "Ran git diff" {
		t.Fatalf("imported activities = %#v", snapshot.Activities)
	}
	if len(snapshot.Turns) != 1 || snapshot.Turns[0].State != domain.TurnStateCompleted {
		t.Fatalf("imported turns = %#v", snapshot.Turns)
	}
	if snapshot.Turns[0].ProviderTurnID != "native-turn-1" {
		t.Fatalf("provider turn = %q, want durable native-turn-1", snapshot.Turns[0].ProviderTurnID)
	}
	if snapshot.Turns[0].CompletedAt == nil || !snapshot.Turns[0].CompletedAt.Equal(now) {
		t.Fatalf("replayed completion = %v, want original %s", snapshot.Turns[0].CompletedAt, now)
	}
}

func TestSlowNativeHistoryDoesNotBlockOtherControllerLookups(t *testing.T) {
	st := openStore(t)
	conv := &blockingHistoryConversation{
		fakeConversation: newFakeConversation(),
		started:          make(chan struct{}),
		release:          make(chan struct{}),
	}
	svc := chatsvc.New(chatsvc.Options{
		Store: st, Sessions: st,
		Reader: chatsvc.SnapshotReaderFunc(func(ctx context.Context, conversationID string) (chatsvc.ConversationRows, error) {
			rows, err := st.LoadConversationSnapshot(ctx, conversationID)
			if err != nil {
				return chatsvc.ConversationRows{}, err
			}
			return chatsvc.ConversationRows{
				Conversation: rows.Conversation,
				Turns:        rows.Turns, Messages: rows.Messages, Activities: rows.Activities,
			}, nil
		}),
		Drivers: fakeRegistry{driver: fakeDriver{conv: conv}},
		Log:     slog.New(slog.DiscardHandler),
		NewID:   func() string { return fmt.Sprintf("lock-id-%d", time.Now().UnixNano()) },
	})
	workspace := t.TempDir()
	startDone := make(chan error, 1)
	go func() {
		_, err := svc.Start(context.Background(), chatsvc.StartConfig{
			SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex,
			WorkspacePath: workspace, ProviderConversationID: "thread-1",
		})
		startDone <- err
	}()
	select {
	case <-conv.started:
	case <-time.After(time.Second):
		t.Fatal("native history import did not start")
	}

	lookupDone := make(chan error, 1)
	go func() {
		_, err := svc.Controller(domain.SessionID("another-session"))
		lookupDone <- err
	}()
	select {
	case err := <-lookupDone:
		if !errors.Is(err, chatsvc.ErrNoController) {
			t.Fatalf("Controller error = %v, want ErrNoController", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("a slow native history import blocked an unrelated controller lookup")
	}

	close(conv.release)
	if err := <-startDone; err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
}

func TestFreshProjectControllerRecordsNativeContextBoundary(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	conversation, err := st.CreateConversation(ctx, "project-conversation",
		domain.ConversationScopeProject, testProject, testSession, now)
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if err := st.UpsertActivity(ctx, conversation.ID, "", domain.ConversationActivity{
		ID: "old-activity", Kind: domain.ActivityKindSystem, Status: domain.ActivityStatusCompleted,
		Summary: "Earlier project history", ProviderItemID: "old-project-history",
	}, now); err != nil {
		t.Fatalf("seed project history: %v", err)
	}

	const replacement = domain.SessionID("p1-2")
	if _, err := st.CreateSession(ctx, domain.SessionRecord{
		ID: replacement, ProjectID: testProject, Kind: domain.KindOrchestrator,
		Harness: domain.HarnessCodex, Mode: domain.SessionModeChat,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed replacement session: %v", err)
	}

	var idMu sync.Mutex
	nextID := 0
	svc := chatsvc.New(chatsvc.Options{
		Store: st, Sessions: st,
		Drivers: fakeRegistry{driver: fakeDriver{conv: newFakeConversation()}},
		Log:     slog.New(slog.DiscardHandler),
		NewID: func() string {
			idMu.Lock()
			defer idMu.Unlock()
			nextID++
			return fmt.Sprintf("boundary-id-%d", nextID)
		},
		Now: func() time.Time { return now.Add(time.Minute) },
	})
	if _, err := svc.Start(ctx, chatsvc.StartConfig{
		SessionID: replacement, ProjectID: testProject, Kind: domain.KindOrchestrator,
		Harness: domain.HarnessCodex, WorkspacePath: t.TempDir(),
	}); err != nil {
		t.Fatalf("Start replacement: %v", err)
	}
	t.Cleanup(func() { _ = svc.Stop(context.Background(), replacement) })

	rebound, err := st.ConversationForSession(ctx, replacement)
	if err != nil {
		t.Fatalf("ConversationForSession: %v", err)
	}
	if rebound.ID != conversation.ID {
		t.Fatalf("conversation = %q, want project narrative %q", rebound.ID, conversation.ID)
	}
	snapshot, err := st.LoadConversationSnapshot(ctx, rebound.ID)
	if err != nil {
		t.Fatalf("LoadConversationSnapshot: %v", err)
	}
	if len(snapshot.Activities) != 2 {
		t.Fatalf("activities = %#v, want old history plus context boundary", snapshot.Activities)
	}
	boundary := snapshot.Activities[1]
	if boundary.Kind != domain.ActivityKindSystem || boundary.ProviderItemID != "kennel-context-reset:p1-2" {
		t.Fatalf("boundary = %#v", boundary)
	}
	var detail map[string]string
	if err := json.Unmarshal(boundary.Detail, &detail); err != nil {
		t.Fatalf("decode boundary detail: %v", err)
	}
	if detail["event"] != "context.reset" {
		t.Fatalf("boundary event = %q", detail["event"])
	}
}

/* ---- harness ----------------------------------------------------------- */

type harness struct {
	svc      *chatsvc.Service
	st       *sqlite.Store
	conv     *fakeConversation
	ctrl     *chatsvc.Controller
	activity *recordingActivity

	clockMu sync.Mutex
	clock   time.Time
}

// advance moves the injected clock. Needed where an ordering rule is expressed in
// timestamps — the queue cancellation cutoff — rather than in call order.
func (h *harness) advance(d time.Duration) {
	h.clockMu.Lock()
	defer h.clockMu.Unlock()
	h.clock = h.clock.Add(d)
}

func (h *harness) now() time.Time {
	h.clockMu.Lock()
	defer h.clockMu.Unlock()
	return h.clock
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessWithConversation(t, nil)
}

// newHarnessWithConversation lets a test supply its own provider double, for the
// cases where the interesting behavior is how the provider answers rather than
// what it streams. A nil conv gets the plain fake.
func newHarnessWithConversation(t *testing.T, conv ports.ChatConversation) *harness {
	return newHarnessWithConversationAndStore(t, conv, func(st *sqlite.Store) chatsvc.Store { return st })
}

func newHarnessWithConversationAndStore(
	t *testing.T,
	conv ports.ChatConversation,
	wrapStore func(*sqlite.Store) chatsvc.Store,
) *harness {
	t.Helper()
	st := openStore(t)
	base := newFakeConversation()
	if conv == nil {
		conv = base
	} else if recorder, ok := conv.(*interruptRecorder); ok {
		base = recorder.fakeConversation
	} else if recorder, ok := conv.(*historyRecorder); ok {
		base = recorder.fakeConversation
	} else if recorder, ok := conv.(*answerDispatchConversation); ok {
		base = recorder.fakeConversation
	}
	h := &harness{
		st:       st,
		conv:     base,
		activity: &recordingActivity{},
		clock:    time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC),
	}

	// Guarded because the id factory is called from both the projection goroutine and
	// whichever goroutine a test drives commands from, and an unsynchronized counter
	// is a data race the -race build fails on rather than a harmless test detail.
	var (
		counterMu sync.Mutex
		counter   int
	)
	chatStore := wrapStore(st)
	svc := chatsvc.New(chatsvc.Options{
		Store:    chatStore,
		Sessions: st,
		Drivers:  fakeRegistry{driver: fakeDriver{conv: conv}},
		Activity: h.activity,
		Log:      slog.New(slog.DiscardHandler),
		NewID: func() string {
			counterMu.Lock()
			defer counterMu.Unlock()
			counter++
			return fmt.Sprintf("id-%03d", counter)
		},
		Now: h.now,
	})

	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{
		SessionID:     testSession,
		ProjectID:     testProject,
		Harness:       domain.HarnessCodex,
		WorkspacePath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })

	h.svc, h.ctrl = svc, ctrl
	return h
}

// awaitSnapshot polls until pred holds, so a test does not race the projector.
func (h *harness) awaitSnapshot(t *testing.T, pred func(store.ConversationSnapshot) bool) store.ConversationSnapshot {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last store.ConversationSnapshot
	for time.Now().Before(deadline) {
		snapshot, err := h.st.LoadConversationSnapshot(context.Background(), h.ctrl.ConversationID())
		if err != nil {
			t.Fatalf("load snapshot: %v", err)
		}
		last = snapshot
		if pred(snapshot) {
			return snapshot
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("snapshot never satisfied the condition; last had %d messages, %d activities, %d turns",
		len(last.Messages), len(last.Activities), len(last.Turns))
	return last
}

func TestStaleControllerEventsDoNotReachTheTimeline(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.st.ClaimChatControllerGeneration(ctx, testSession, "replacement-generation", "", "", h.now()); err != nil {
		t.Fatalf("replace controller generation: %v", err)
	}

	h.conv.emit(ports.ChatEvent{
		Kind:           ports.ChatEventMessageDelta,
		ProviderTurnID: "stale-turn",
		ProviderItemID: "stale-message",
		Delta:          "must not survive",
	})
	if err := h.svc.Stop(ctx, testSession); err != nil {
		t.Fatalf("stop stale controller: %v", err)
	}

	snapshot, err := h.st.LoadConversationSnapshot(ctx, h.ctrl.ConversationID())
	if err != nil {
		t.Fatalf("load conversation: %v", err)
	}
	for _, message := range snapshot.Messages {
		if message.Text == "must not survive" {
			t.Fatalf("stale controller message was projected: %+v", message)
		}
	}
}

/* ---- tests ------------------------------------------------------------- */

// The whole point: a message goes out, provider events come back, and the durable
// timeline reflects them in sequence order.
func TestProjectsAFullTurnIntoDurableRows(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	turn, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text:            "what changed?",
		ClientMessageID: "client-1",
		Origin:          domain.MessageOriginHuman,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if turn.ProviderTurnID != "provider-turn-1" {
		t.Fatalf("provider turn = %q", turn.ProviderTurnID)
	}

	h.conv.emit(
		ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"},
		ports.ChatEvent{
			Kind: ports.ChatEventActivityCompleted, ProviderTurnID: "provider-turn-1",
			ProviderItemID: "exec-1", ActivityKind: domain.ActivityKindCommand,
			ActivityStatus: domain.ActivityStatusCompleted, Summary: "git status --short",
		},
		// Streaming arrives in pieces and must fold into one message.
		ports.ChatEvent{Kind: ports.ChatEventMessageDelta, ProviderTurnID: "provider-turn-1", ProviderItemID: "msg-1", Delta: "Two "},
		ports.ChatEvent{Kind: ports.ChatEventMessageDelta, ProviderTurnID: "provider-turn-1", ProviderItemID: "msg-1", Delta: "files "},
		ports.ChatEvent{Kind: ports.ChatEventMessageDelta, ProviderTurnID: "provider-turn-1", ProviderItemID: "msg-1", Delta: "changed."},
		ports.ChatEvent{Kind: ports.ChatEventMessageCompleted, ProviderTurnID: "provider-turn-1", ProviderItemID: "msg-1", Text: "Two files changed."},
		ports.ChatEvent{Kind: ports.ChatEventTurnCompleted, ProviderTurnID: "provider-turn-1", TurnState: domain.TurnStateCompleted},
	)

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State.Terminal() && len(s.Messages) == 2
	})

	if got := snapshot.Turns[0].State; got != domain.TurnStateCompleted {
		t.Errorf("turn state = %q, want completed", got)
	}

	user, assistant := snapshot.Messages[0], snapshot.Messages[1]
	if user.Role != domain.MessageRoleUser || user.Text != "what changed?" {
		t.Errorf("user message = %+v", user)
	}
	if user.Origin != domain.MessageOriginHuman {
		t.Errorf("user origin = %q", user.Origin)
	}
	// Three deltas folded into one message, not three timeline entries.
	if assistant.Role != domain.MessageRoleAssistant || assistant.Text != "Two files changed." {
		t.Errorf("assistant message = %+v", assistant)
	}
	if assistant.Streaming {
		t.Error("assistant message still marked streaming after completion")
	}
	if assistant.Revision == 0 {
		t.Error("assistant revision never advanced despite streaming rewrites")
	}

	// Sequence is conversation-scoped and strictly increasing across both tables.
	var sequences []int64
	for _, m := range snapshot.Messages {
		sequences = append(sequences, m.Sequence)
	}
	for _, a := range snapshot.Activities {
		sequences = append(sequences, a.Sequence)
	}
	seen := map[int64]bool{}
	for _, seq := range sequences {
		if seen[seq] {
			t.Fatalf("sequence %d was handed out twice", seq)
		}
		seen[seq] = true
	}

	if len(snapshot.Activities) != 1 || snapshot.Activities[0].Summary != "git status --short" {
		t.Fatalf("activities = %+v", snapshot.Activities)
	}
}

func TestEarlyTurnStartedBindsDispatchingTurn(t *testing.T) {
	conv := newFakeConversation()
	conv.onSend = func(providerTurnID string) {
		conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: providerTurnID})
	}
	h := newHarnessWithConversation(t, conv)
	ctx := context.Background()

	turn, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text:            "race turn/start",
		ClientMessageID: "early-start-1",
		Origin:          domain.MessageOriginHuman,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if turn.ProviderTurnID != "provider-turn-1" {
		t.Fatalf("provider turn = %q, want provider-turn-1", turn.ProviderTurnID)
	}
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		if len(s.Turns) != 1 {
			return false
		}
		turn := s.Turns[0]
		return turn.ProviderTurnID == "provider-turn-1" && turn.State == domain.TurnStateRunning
	})

	conv.emit(ports.ChatEvent{
		Kind:           ports.ChatEventTurnCompleted,
		ProviderTurnID: "provider-turn-1",
		TurnState:      domain.TurnStateCompleted,
	})
	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State == domain.TurnStateCompleted
	})
	if len(snapshot.Turns) != 1 {
		t.Fatalf("turns = %d, want 1: %+v", len(snapshot.Turns), snapshot.Turns)
	}
}

func TestControllerCloseHonorsContextWhenProviderStreamStaysOpen(t *testing.T) {
	providerErr := errors.New("provider close failed")
	conv := &stuckConversation{fakeConversation: newFakeConversation(), closeErr: providerErr}
	h := newHarnessWithConversation(t, conv)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := h.ctrl.Close(ctx)
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, providerErr) {
		t.Fatalf("Close error = %v, want provider error joined with deadline", err)
	}
	// Let the projection goroutine exit so the harness cleanup remains bounded.
	close(conv.events)
	h.ctrl.Wait()
}

// A retried send under the same client message id must not create a second turn.
func TestDuplicateSendDoesNotCreateASecondTurn(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	msg := ports.ChatUserMessage{Text: "hello", ClientMessageID: "client-dup", Origin: domain.MessageOriginHuman}

	if _, err := h.svc.Send(ctx, testSession, msg); err != nil {
		t.Fatalf("first Send: %v", err)
	}
	second, err := h.svc.Send(ctx, testSession, msg)
	if err != nil {
		t.Fatalf("retried Send returned an error instead of being ignored: %v", err)
	}
	if second.ProviderTurnID != "" {
		t.Errorf("retry reported a new provider turn %q", second.ProviderTurnID)
	}

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool { return len(s.Turns) >= 1 })
	if len(snapshot.Turns) != 1 {
		t.Fatalf("turns = %d, want 1", len(snapshot.Turns))
	}
	if len(snapshot.Messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(snapshot.Messages))
	}
}

func TestDeferredDriverStartsOnlyAfterProviderTurnIDIsDurable(t *testing.T) {
	deferred := &deferredConversation{fakeConversation: newFakeConversation()}
	h := newHarnessWithConversation(t, deferred)
	deferred.start = func(providerTurnID string) error {
		snapshot, err := h.st.LoadConversationSnapshot(context.Background(), h.ctrl.ConversationID())
		if err != nil {
			return err
		}
		for _, turn := range snapshot.Turns {
			if turn.ProviderTurnID == providerTurnID {
				return nil
			}
		}
		return fmt.Errorf("provider turn %q was not bound before deferred start", providerTurnID)
	}

	turn, err := h.svc.Send(context.Background(), testSession, ports.ChatUserMessage{
		Text: "start through ACP", ClientMessageID: "deferred-1", Origin: domain.MessageOriginHuman,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if turn.ProviderTurnID == "" {
		t.Fatal("deferred turn has no provider id")
	}
}

// An approval must be stored pending, carry the provider's own decision list, and
// only resolve through a typed action.
func TestApprovalIsStoredPendingWithProviderDecisions(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{Text: "go", ClientMessageID: "c1"}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// The real captured shape: no decline on offer, plus a structured decision.
	h.conv.emit(ports.ChatEvent{
		Kind:           ports.ChatEventApprovalRequested,
		ProviderTurnID: "provider-turn-1",
		ProviderItemID: "0",
		RequestID:      "0",
		ActivityKind:   domain.ActivityKindCommand,
		ActivityStatus: domain.ActivityStatusPending,
		Summary:        "Run kennel spawn",
		Decisions: []ports.ChatDecisionOption{
			{ID: "accept", Label: "Approve"},
			{ID: "acceptWithExecpolicyAmendment", Label: "Approve and remember this command"},
			{ID: "cancel", Label: "Cancel"},
		},
	})

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Activities) == 1
	})
	approval := snapshot.Activities[0]
	if approval.Kind != domain.ActivityKindApproval {
		t.Fatalf("kind = %q, want approval", approval.Kind)
	}
	if approval.Status != domain.ActivityStatusPending {
		t.Fatalf("status = %q, want pending", approval.Status)
	}
	if approval.RequestID != "0" {
		t.Fatalf("request id = %q; zero is a legitimate id and must survive", approval.RequestID)
	}

	var detail struct {
		Decisions []struct{ ID, Label string } `json:"decisions"`
	}
	if err := json.Unmarshal(approval.Detail, &detail); err != nil {
		t.Fatalf("detail not decodable: %v (%s)", err, approval.Detail)
	}
	if len(detail.Decisions) != 3 {
		t.Fatalf("stored %d decisions, want the provider's 3: %+v", len(detail.Decisions), detail.Decisions)
	}
	for _, d := range detail.Decisions {
		if d.ID == "decline" {
			t.Error("stored a decline option the provider never offered")
		}
	}

	// Resolving reaches the provider and then updates the row.
	if err := h.svc.Resolve(ctx, testSession, "0", ports.ChatDecision{ID: "accept"}); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, ok := h.conv.decisionFor("0"); !ok {
		t.Error("decision never reached the provider")
	}
	resolved := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Activities) == 1 && s.Activities[0].Status == domain.ActivityStatusResolved
	})
	if resolved.Activities[0].Status != domain.ActivityStatusResolved {
		t.Fatalf("status = %q, want resolved", resolved.Activities[0].Status)
	}
}

// A controller that dies mid-turn must not leave the turn looking like it is still
// working, and must not leave an approval the user can never answer.
func TestControllerDeathSettlesInFlightWork(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{Text: "go", ClientMessageID: "c1"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	h.conv.emit(
		ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"},
		startedCommand("provider-turn-1", "exec-1", "sleep 60"),
	)
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool { return len(s.Activities) == 1 })
	h.conv.emit(
		ports.ChatEvent{
			Kind: ports.ChatEventApprovalRequested, ProviderTurnID: "provider-turn-1",
			ProviderItemID: "0", RequestID: "0", ActivityKind: domain.ActivityKindCommand,
			ActivityStatus: domain.ActivityStatusPending, Summary: "Run something",
		},
	)
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool { return len(s.Activities) == 2 })

	// The provider process dies: the stream closes with the turn still open.
	_ = h.conv.Close()
	h.ctrl.Wait()
	if err := h.svc.Stop(ctx, testSession); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State.Terminal()
	})
	if got := snapshot.Turns[0].State; got != domain.TurnStateFailed {
		t.Errorf("turn state = %q; an interrupted controller is not a completed turn", got)
	}
	if snapshot.Turns[0].ErrorMessage == "" {
		t.Error("orphaned turn carries no explanation")
	}
	if got := findActivity(t, snapshot, "0").Status; got == domain.ActivityStatusPending {
		t.Error("approval left pending after its controller died — the user can never answer it")
	}
	if got := findActivity(t, snapshot, "exec-1").Status; got != domain.ActivityStatusFailed {
		t.Errorf("running activity after controller death = %q, want failed", got)
	}
}

func TestControllerStreamClosureReportsSessionExited(t *testing.T) {
	h := newHarness(t)

	// A provider can disappear without emitting a final controller-state event.
	// Closing the stream is still authoritative proof that this controller ended.
	if err := h.conv.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	h.ctrl.Wait()

	for _, signal := range h.activity.snapshot() {
		if signal.State == domain.ActivityExited && signal.Event == "chat.controller.stopped" {
			return
		}
	}
	t.Fatalf("controller stream ended without an exited lifecycle signal: %+v", h.activity.snapshot())
}

func TestControllerReadyRunsBeforeStreamProjection(t *testing.T) {
	st := openStore(t)
	conv := newFakeConversation()
	if err := conv.Close(); err != nil {
		t.Fatalf("close provider stream: %v", err)
	}
	activity := &recordingActivity{}
	ready := false
	svc := chatsvc.New(chatsvc.Options{
		Store: st, Sessions: st,
		Drivers:  fakeRegistry{driver: fakeDriver{conv: conv}},
		Activity: activity,
		Log:      slog.New(slog.DiscardHandler),
		NewID:    func() string { return "controller-ready-id" },
	})

	controller, err := svc.Start(context.Background(), chatsvc.StartConfig{
		SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex,
		WorkspacePath: t.TempDir(),
		ControllerReady: func(started chatsvc.StartResult) error {
			if signals := activity.snapshot(); len(signals) != 0 {
				t.Fatalf("provider events projected before controller-ready commit: %+v", signals)
			}
			if started.ProviderConversationID == "" || started.ControllerGeneration == "" {
				t.Fatalf("controller-ready result = %+v", started)
			}
			ready = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	controller.Wait()
	if !ready {
		t.Fatal("controller-ready callback was not called")
	}
	for _, signal := range activity.snapshot() {
		if signal.State == domain.ActivityExited && signal.Event == "chat.controller.stopped" {
			return
		}
	}
	t.Fatalf("stream closure was not projected after controller-ready: %+v", activity.snapshot())
}

// Dispatch reads the persisted mode. A TUI session must be refused even if a
// controller somehow exists, because the mode is the authority.
func TestSendRefusedForTUISession(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	tuiSession := domain.SessionID("p1-2")
	if _, err := h.st.CreateSession(ctx, domain.SessionRecord{
		ID:        tuiSession,
		ProjectID: testProject,
		Kind:      domain.KindWorker,
		Harness:   domain.HarnessCodex,
		Mode:      domain.SessionModeTUI,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed tui session: %v", err)
	}

	_, err := h.svc.Send(ctx, tuiSession, ports.ChatUserMessage{Text: "hi", ClientMessageID: "c9"})
	if err == nil {
		t.Fatal("a TUI session accepted a chat send")
	}
	if !errorsIs(err, chatsvc.ErrNotChatMode) {
		t.Fatalf("err = %v, want ErrNotChatMode", err)
	}
}

// Every projected event is also archived, so a wrong projection can be repaired
// from the raw record instead of being the only surviving account.
func TestProviderEventsAreArchived(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{Text: "go", ClientMessageID: "c1"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	h.conv.emit(
		ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"},
		ports.ChatEvent{Kind: ports.ChatEventMessageDelta, ProviderTurnID: "provider-turn-1", ProviderItemID: "msg-1", Delta: "hi"},
		ports.ChatEvent{Kind: ports.ChatEventTurnCompleted, ProviderTurnID: "provider-turn-1", TurnState: domain.TurnStateCompleted},
	)
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State.Terminal()
	})

	events, err := h.st.ProviderEventsSince(ctx, h.ctrl.ConversationID(), 0, 100)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if len(events) < 3 {
		t.Fatalf("archived %d events, want at least the 3 emitted", len(events))
	}
}

/* ---- the send queue ---------------------------------------------------- */

// turnStateByText is how the queue tests read the timeline: a turn matters here
// only as the fate of one message the user typed.
func turnStateByText(t *testing.T, s store.ConversationSnapshot) map[string]domain.TurnState {
	t.Helper()
	turns := map[string]domain.ConversationTurn{}
	for _, turn := range s.Turns {
		turns[turn.ID] = turn
	}
	states := map[string]domain.TurnState{}
	for _, msg := range s.Messages {
		if msg.Role != domain.MessageRoleUser {
			continue
		}
		if turn, ok := turns[msg.TurnID]; ok {
			states[msg.Text] = turn.State
		}
	}
	return states
}

// The composer tells the user a mid-turn message is queued until the agent
// finishes. That has to be true of the daemon, not just of the placeholder: a
// second turn/start against a busy provider is not a thing the agent can run.
func TestSendWhileBusyQueuesUntilTheTurnEnds(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "first", ClientMessageID: "c1", Origin: domain.MessageOriginHuman,
	}); err != nil {
		t.Fatalf("first Send: %v", err)
	}
	h.conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State == domain.TurnStateRunning
	})

	queued, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "second", ClientMessageID: "c2", Origin: domain.MessageOriginHuman,
	})
	if err != nil {
		t.Fatalf("mid-turn Send: %v", err)
	}
	if queued.State != domain.TurnStateQueued {
		t.Errorf("mid-turn send reported state %q, want queued", queued.State)
	}
	if queued.ProviderTurnID != "" {
		t.Errorf("mid-turn send claimed provider turn %q; it was never dispatched", queued.ProviderTurnID)
	}
	if got := h.conv.sentTexts(); len(got) != 1 {
		t.Fatalf("provider received %v; the second message must wait", got)
	}

	// The running turn ends, so the queued message goes out on its own.
	h.conv.emit(ports.ChatEvent{
		Kind: ports.ChatEventTurnCompleted, ProviderTurnID: "provider-turn-1",
		TurnState: domain.TurnStateCompleted,
	})

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return turnStateByText(t, s)["second"] == domain.TurnStateRunning
	})
	states := turnStateByText(t, snapshot)
	if states["first"] != domain.TurnStateCompleted {
		t.Errorf("first turn = %q, want completed", states["first"])
	}
	if got := h.conv.sentTexts(); len(got) != 2 || got[1] != "second" {
		t.Fatalf("provider received %v, want the queued message dispatched second", got)
	}
}

// Codex can start nested turns while the root turn is still working. A child
// completion is not conversation quiescence: dispatching queued automation at
// that point injects it into the still-running root and leaves the Kennel turn minted
// for that automation with no matching provider lifecycle.
func TestNestedTurnCompletionDoesNotDrainQueueWhilePrimaryTurnRuns(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "root work", ClientMessageID: "c1", Origin: domain.MessageOriginHuman,
	}); err != nil {
		t.Fatalf("send root turn: %v", err)
	}
	h.conv.emit(ports.ChatEvent{
		Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1",
		ProviderConversationID: "thread-1",
	})
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return turnStateByText(t, s)["root work"] == domain.TurnStateRunning
	})

	queued, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "queued automation", ClientMessageID: "c2", Origin: domain.MessageOriginAutomation,
	})
	if err != nil {
		t.Fatalf("queue automation: %v", err)
	}
	if queued.State != domain.TurnStateQueued {
		t.Fatalf("automation state = %q, want queued", queued.State)
	}

	h.conv.emit(
		ports.ChatEvent{
			Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-child-1",
			ProviderConversationID: "child-thread-1",
		},
		ports.ChatEvent{
			Kind: ports.ChatEventTurnCompleted, ProviderTurnID: "provider-child-1",
			ProviderConversationID: "child-thread-1", TurnState: domain.TurnStateCompleted,
		},
		// This later root event is a deterministic barrier: once projected, the
		// child's afterProject hook (including any incorrect drain) has finished.
		ports.ChatEvent{
			Kind: ports.ChatEventActivityCompleted, ProviderTurnID: "provider-turn-1",
			ProviderItemID: "root-still-working", ActivityKind: domain.ActivityKindCommand,
			ActivityStatus: domain.ActivityStatusCompleted, Summary: "root continued",
		},
	)

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		for _, activity := range s.Activities {
			if activity.ProviderItemID == "root-still-working" {
				return true
			}
		}
		return false
	})
	states := turnStateByText(t, snapshot)
	if states["queued automation"] != domain.TurnStateQueued {
		t.Errorf("automation state after child completion = %q, want queued", states["queued automation"])
	}
	if got := h.conv.sentTexts(); len(got) != 1 {
		t.Fatalf("provider received %v; child completion must not drain queued automation", got)
	}
	if got := h.ctrl.State(); got != ports.ChatControllerBusy {
		t.Errorf("controller state after child completion = %q, want busy", got)
	}
	for _, signal := range h.activity.snapshot() {
		if signal.State == domain.ActivityIdle {
			t.Fatalf("child completion reported the session idle: %+v", signal)
		}
	}

	h.conv.emit(ports.ChatEvent{
		Kind: ports.ChatEventTurnCompleted, ProviderTurnID: "provider-turn-1",
		ProviderConversationID: "thread-1", TurnState: domain.TurnStateCompleted,
	})
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return turnStateByText(t, s)["queued automation"] == domain.TurnStateRunning
	})
	if got := h.conv.sentTexts(); len(got) != 2 || got[1] != "queued automation" {
		t.Fatalf("provider received %v, want automation only after root completion", got)
	}
	idleReported := false
	for _, signal := range h.activity.snapshot() {
		idleReported = idleReported || signal.State == domain.ActivityIdle
	}
	if !idleReported {
		t.Fatal("root completion did not report the session idle")
	}
}

// A Chat -> TUI handoff waits for the root turn and everything already queued
// behind it. Nested Codex lifecycle must not make that drain look complete, nor
// may it release the accepted queue into a root turn that is still running.
func TestChatHandoffDrainWaitsForRootAfterNestedTurnCompletes(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "root work", ClientMessageID: "handoff-nested-1", Origin: domain.MessageOriginHuman,
	}); err != nil {
		t.Fatalf("send root turn: %v", err)
	}
	h.conv.emit(ports.ChatEvent{
		Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1",
		ProviderConversationID: "thread-1",
	})
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return turnStateByText(t, s)["root work"] == domain.TurnStateRunning
	})
	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "accepted queue", ClientMessageID: "handoff-nested-2", Origin: domain.MessageOriginAutomation,
	}); err != nil {
		t.Fatalf("queue accepted turn: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- h.svc.PrepareChatHandoff(ctx, testSession, domain.SessionInterfaceTransitionDrain)
	}()

	h.conv.emit(
		ports.ChatEvent{
			Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-child-1",
			ProviderConversationID: "child-thread-1",
		},
		ports.ChatEvent{
			Kind: ports.ChatEventTurnCompleted, ProviderTurnID: "provider-child-1",
			ProviderConversationID: "child-thread-1", TurnState: domain.TurnStateCompleted,
		},
		ports.ChatEvent{
			Kind: ports.ChatEventActivityCompleted, ProviderTurnID: "provider-turn-1",
			ProviderItemID: "handoff-root-still-working", ActivityKind: domain.ActivityKindCommand,
			ActivityStatus: domain.ActivityStatusCompleted, Summary: "root continued",
		},
	)
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		for _, activity := range s.Activities {
			if activity.ProviderItemID == "handoff-root-still-working" {
				return true
			}
		}
		return false
	})

	select {
	case err := <-done:
		t.Fatalf("handoff finished after child completion while root was active: %v", err)
	default:
	}
	if got := h.conv.sentTexts(); len(got) != 1 {
		t.Fatalf("provider received %v; nested completion released the accepted queue", got)
	}

	h.conv.emit(ports.ChatEvent{
		Kind: ports.ChatEventTurnCompleted, ProviderTurnID: "provider-turn-1",
		ProviderConversationID: "thread-1", TurnState: domain.TurnStateCompleted,
	})
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return turnStateByText(t, s)["accepted queue"] == domain.TurnStateRunning
	})
	select {
	case err := <-done:
		t.Fatalf("handoff finished before its accepted queue completed: %v", err)
	default:
	}

	h.conv.emit(ports.ChatEvent{
		Kind: ports.ChatEventTurnCompleted, ProviderTurnID: "provider-turn-2",
		ProviderConversationID: "thread-1", TurnState: domain.TurnStateCompleted,
	})
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("prepare handoff: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("handoff did not become quiescent after the root and accepted queue completed")
	}
}

// Stop is a brake. Releasing the queue when the user presses it would be the
// opposite of what the button says, so anything waiting is cancelled with the turn.
func TestInterruptCancelsWhatIsQueuedBehindTheTurn(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "running", ClientMessageID: "c1",
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	h.conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State == domain.TurnStateRunning
	})
	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "queued", ClientMessageID: "c2",
	}); err != nil {
		t.Fatalf("mid-turn Send: %v", err)
	}

	if err := h.svc.Interrupt(ctx, testSession); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	h.conv.emit(ports.ChatEvent{
		Kind: ports.ChatEventTurnCompleted, ProviderTurnID: "provider-turn-1",
		TurnState: domain.TurnStateInterrupted,
	})

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		states := turnStateByText(t, s)
		return states["queued"].Terminal() && states["running"].Terminal()
	})
	states := turnStateByText(t, snapshot)
	if states["queued"] != domain.TurnStateInterrupted {
		t.Errorf("queued turn = %q; a message never dispatched did not fail, it was cancelled",
			states["queued"])
	}
	if got := h.conv.sentTexts(); len(got) != 1 {
		t.Fatalf("provider received %v; stop must not release the queue", got)
	}
}

func TestChatHandoffDrainFinishesAcceptedQueueAndClosesNewIntake(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "running", ClientMessageID: "handoff-1",
	}); err != nil {
		t.Fatalf("send running turn: %v", err)
	}
	h.conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State == domain.TurnStateRunning
	})
	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "already queued", ClientMessageID: "handoff-2",
	}); err != nil {
		t.Fatalf("queue second turn: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- h.svc.PrepareChatHandoff(ctx, testSession, domain.SessionInterfaceTransitionDrain)
	}()
	// Completion of the first accepted turn must dispatch the accepted queue,
	// rather than declaring the source quiescent early.
	h.conv.emit(ports.ChatEvent{
		Kind: ports.ChatEventTurnCompleted, ProviderTurnID: "provider-turn-1",
		TurnState: domain.TurnStateCompleted,
	})
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return turnStateByText(t, s)["already queued"] == domain.TurnStateRunning
	})
	h.conv.emit(ports.ChatEvent{
		Kind: ports.ChatEventTurnCompleted, ProviderTurnID: "provider-turn-2",
		TurnState: domain.TurnStateCompleted,
	})
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("prepare handoff: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("handoff did not become quiescent after its accepted queue completed")
	}

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "too late", ClientMessageID: "handoff-3",
	}); !errors.Is(err, chatsvc.ErrControllerHandoff) {
		t.Fatalf("send after handoff gate = %v, want ErrControllerHandoff", err)
	}
	h.svc.AbortChatHandoff(testSession)
	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "source reopened", ClientMessageID: "handoff-4",
	}); err != nil {
		t.Fatalf("send after aborting handoff: %v", err)
	}
}

func TestChatHandoffInterruptArmBlocksCompletionFromPromotingQueue(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "run the dev server", ClientMessageID: "handoff-arm-1",
	}); err != nil {
		t.Fatalf("send running turn: %v", err)
	}
	h.conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State == domain.TurnStateRunning
	})
	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "touch /tmp/ao3945-queued-ran", ClientMessageID: "handoff-arm-2",
	}); err != nil {
		t.Fatalf("queue second turn: %v", err)
	}
	// Once intake is fenced, eventual cancellation must cover the whole queue
	// rather than trusting wall-clock ordering. A host clock correction cannot
	// make an older accepted command look newer than the handoff cutoff.
	h.advance(-time.Hour)

	// Session Manager performs this fast step before returning the accepted
	// transition and before target preflight. It is a reversible dispatch fence:
	// the active turn can finish, but its completion must not release the queue.
	if err := h.svc.ArmChatHandoff(ctx, testSession, domain.SessionInterfaceTransitionInterrupt); err != nil {
		t.Fatalf("arm interrupt handoff: %v", err)
	}
	h.conv.emit(ports.ChatEvent{
		Kind: ports.ChatEventTurnCompleted, ProviderTurnID: "provider-turn-1",
		TurnState: domain.TurnStateInterrupted,
	})
	fenced := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		states := turnStateByText(t, s)
		return states["run the dev server"].Terminal()
	})
	if got := turnStateByText(t, fenced)["touch /tmp/ao3945-queued-ran"]; got != domain.TurnStateQueued {
		t.Fatalf("queued command while target preflights = %q, want reversibly fenced", got)
	}
	if got := h.conv.sentTexts(); len(got) != 1 {
		t.Fatalf("provider received %v before handoff preparation; armed completion released the queue", got)
	}

	// Target preflight has now succeeded. Preparation makes the fence durable
	// before touching the provider and remains independent of clock movement.
	if err := h.svc.PrepareChatHandoff(ctx, testSession, domain.SessionInterfaceTransitionInterrupt); err != nil {
		t.Fatalf("prepare armed handoff: %v", err)
	}
	if err := h.svc.Stop(ctx, testSession); err != nil {
		t.Fatalf("stop source controller: %v", err)
	}

	if got := h.conv.sentTexts(); len(got) != 1 {
		t.Fatalf("provider received %v; the queued command crossed the armed handoff", got)
	}
	snapshot, err := h.st.LoadConversationSnapshot(ctx, h.ctrl.ConversationID())
	if err != nil {
		t.Fatalf("load stopped conversation: %v", err)
	}
	if got := turnStateByText(t, snapshot)["touch /tmp/ao3945-queued-ran"]; got != domain.TurnStateInterrupted {
		t.Fatalf("queued command = %q, want interrupted without provider dispatch", got)
	}
}

func TestChatHandoffInterruptAbortAfterPreflightFailureResumesFencedQueue(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "run the dev server", ClientMessageID: "handoff-abort-1",
	}); err != nil {
		t.Fatalf("send running turn: %v", err)
	}
	h.conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State == domain.TurnStateRunning
	})
	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "queued work survives preflight", ClientMessageID: "handoff-abort-2",
	}); err != nil {
		t.Fatalf("queue second turn: %v", err)
	}
	if err := h.svc.ArmChatHandoff(ctx, testSession, domain.SessionInterfaceTransitionInterrupt); err != nil {
		t.Fatalf("arm interrupt handoff: %v", err)
	}

	// Completion while target preflight is running cannot dispatch the queue.
	h.conv.emit(ports.ChatEvent{
		Kind: ports.ChatEventTurnCompleted, ProviderTurnID: "provider-turn-1",
		TurnState: domain.TurnStateCompleted,
	})
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		states := turnStateByText(t, s)
		return states["run the dev server"] == domain.TurnStateCompleted &&
			states["queued work survives preflight"] == domain.TurnStateQueued
	})
	if got := h.conv.sentTexts(); len(got) != 1 {
		t.Fatalf("provider received %v while target preflight was fenced", got)
	}

	// A failed target preflight reopens the source and explicitly resumes the
	// queue; there may be no later completion event to trigger this drain.
	h.svc.AbortChatHandoff(testSession)
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return turnStateByText(t, s)["queued work survives preflight"] == domain.TurnStateRunning
	})
	if got := h.conv.sentTexts(); len(got) != 2 || got[1] != "queued work survives preflight" {
		t.Fatalf("provider received %v after abort, want preserved queue to resume", got)
	}
}

type completionOnInterruptConversation struct{ *fakeConversation }

func (c *completionOnInterruptConversation) Interrupt(_ context.Context, turn string) error {
	// ACP completes its prompt goroutine from the same cancellation handshake;
	// this event can therefore be queued before session/cancel returns.
	c.emit(ports.ChatEvent{
		Kind: ports.ChatEventTurnCompleted, ProviderTurnID: turn,
		TurnState: domain.TurnStateInterrupted,
	})
	return nil
}

func TestChatHandoffInterruptCompletionDuringProviderCancellationCannotPromoteQueue(t *testing.T) {
	provider := &completionOnInterruptConversation{fakeConversation: newFakeConversation()}
	h := newHarnessWithConversation(t, provider)
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "run the dev server", ClientMessageID: "handoff-during-1",
	}); err != nil {
		t.Fatalf("send running turn: %v", err)
	}
	provider.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State == domain.TurnStateRunning
	})
	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "touch /tmp/ao3945-queued-ran", ClientMessageID: "handoff-during-2",
	}); err != nil {
		t.Fatalf("queue second turn: %v", err)
	}

	if err := h.svc.PrepareChatHandoff(ctx, testSession, domain.SessionInterfaceTransitionInterrupt); err != nil {
		t.Fatalf("prepare interrupt handoff: %v", err)
	}
	if err := h.svc.Stop(ctx, testSession); err != nil {
		t.Fatalf("stop source controller: %v", err)
	}
	if got := provider.sentTexts(); len(got) != 1 {
		t.Fatalf("provider received %v; ACP-style completion promoted the queue", got)
	}
}

func TestChatHandoffInterruptDoesNotWaitForTurnCompletion(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "run the dev server", ClientMessageID: "handoff-interrupt-1",
	}); err != nil {
		t.Fatalf("send running turn: %v", err)
	}
	h.conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State == domain.TurnStateRunning
	})
	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "queued behind it", ClientMessageID: "handoff-interrupt-2",
	}); err != nil {
		t.Fatalf("queue second turn: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- h.svc.PrepareChatHandoff(ctx, testSession, domain.SessionInterfaceTransitionInterrupt)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("prepare interrupt handoff: %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		// Unblock the old implementation before failing so the test leaves no
		// handoff goroutine behind. A force switch must not need this completion.
		h.conv.emit(ports.ChatEvent{
			Kind: ports.ChatEventTurnCompleted, ProviderTurnID: "provider-turn-1",
			TurnState: domain.TurnStateInterrupted,
		})
		<-done
		t.Fatal("interrupt handoff waited for the running turn to report completion")
	}

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return turnStateByText(t, s)["queued behind it"].Terminal()
	})
	if got := turnStateByText(t, snapshot)["queued behind it"]; got != domain.TurnStateInterrupted {
		t.Fatalf("queued turn = %q, want interrupted before the source controller stops", got)
	}
	// Codex may deliver turn/completed after turn/interrupt has already answered.
	// The armed dispatch gate must still make that late completion a no-op for the
	// queue.
	h.conv.emit(ports.ChatEvent{
		Kind: ports.ChatEventTurnCompleted, ProviderTurnID: "provider-turn-1",
		TurnState: domain.TurnStateInterrupted,
	})
	if err := h.svc.Stop(ctx, testSession); err != nil {
		t.Fatalf("stop armed controller: %v", err)
	}
	if got := h.conv.sentTexts(); len(got) != 1 {
		t.Fatalf("provider received %v; late completion promoted an interrupted queue", got)
	}
}

type failFirstQueueCancellationStore struct {
	*sqlite.Store

	mu    sync.Mutex
	calls int
}

func (s *failFirstQueueCancellationStore) CancelAllQueuedTurns(
	ctx context.Context,
	conversationID string,
	now time.Time,
) error {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.mu.Unlock()
	if call == 1 {
		return errors.New("injected queue cancellation failure")
	}
	return s.Store.CancelAllQueuedTurns(ctx, conversationID, now)
}

func (s *failFirstQueueCancellationStore) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestChatHandoffInterruptDoesNotReachProviderUntilQueueCancellationSucceeds(t *testing.T) {
	provider := newInterruptRecorder()
	var flakyStore *failFirstQueueCancellationStore
	h := newHarnessWithConversationAndStore(t, provider, func(st *sqlite.Store) chatsvc.Store {
		flakyStore = &failFirstQueueCancellationStore{Store: st}
		return flakyStore
	})
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "run the dev server", ClientMessageID: "handoff-race-1",
	}); err != nil {
		t.Fatalf("send running turn: %v", err)
	}
	provider.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	provider.markActive("provider-turn-1")
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State == domain.TurnStateRunning
	})
	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "queued behind it", ClientMessageID: "handoff-race-2",
	}); err != nil {
		t.Fatalf("queue second turn: %v", err)
	}

	if err := h.svc.ArmChatHandoff(ctx, testSession, domain.SessionInterfaceTransitionInterrupt); err != nil {
		t.Fatalf("arm reversible handoff fence: %v", err)
	}
	if got := flakyStore.callCount(); got != 0 {
		t.Fatalf("queue cancellation calls during reversible arm = %d, want 0", got)
	}
	if err := h.svc.PrepareChatHandoff(ctx, testSession, domain.SessionInterfaceTransitionInterrupt); err == nil {
		t.Fatal("first handoff preparation error = nil, want injected queue cancellation failure")
	}
	if got := provider.attemptCount(); got != 0 {
		t.Fatalf("provider interrupt attempts = %d before durable queue cancellation, want 0", got)
	}

	// Preparation aborts the reversible fence after a local-store failure. A
	// caller may re-arm and retry; provider I/O begins only after the durable queue
	// is terminal.
	if err := h.svc.ArmChatHandoff(ctx, testSession, domain.SessionInterfaceTransitionInterrupt); err != nil {
		t.Fatalf("re-arm handoff after preparation failure: %v", err)
	}
	if err := h.svc.PrepareChatHandoff(ctx, testSession, domain.SessionInterfaceTransitionInterrupt); err != nil {
		t.Fatalf("prepare armed interrupt handoff: %v", err)
	}
	if got := flakyStore.callCount(); got != 2 {
		t.Fatalf("queue cancellation calls = %d, want one failed arm and one successful retry", got)
	}
	if got := provider.attemptCount(); got != 1 {
		t.Fatalf("provider interrupt attempts = %d, want 1 after queue cancellation", got)
	}
	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return turnStateByText(t, s)["queued behind it"].Terminal()
	})
	if got := turnStateByText(t, snapshot)["queued behind it"]; got != domain.TurnStateInterrupted {
		t.Fatalf("queued turn = %q, want interrupted before provider cancellation", got)
	}
}

func TestChatHandoffTreatsMissingControllerAsAlreadyQuiescent(t *testing.T) {
	svc := chatsvc.New(chatsvc.Options{})
	if err := svc.PrepareChatHandoff(
		context.Background(), "missing-controller", domain.SessionInterfaceTransitionDrain,
	); err != nil {
		t.Fatalf("prepare missing controller: %v", err)
	}
}

func TestServiceStopRetainsControllerUntilItsEventStreamActuallyEnds(t *testing.T) {
	base := newFakeConversation()
	h := newHarnessWithConversation(t, &stuckConversation{
		fakeConversation: base,
		closeErr:         errors.New("provider close failed"),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := h.svc.Stop(ctx, testSession); err == nil {
		t.Fatal("Stop error = nil, want provider close failure or deadline")
	}
	if _, err := h.svc.Controller(testSession); err != nil {
		t.Fatalf("controller was forgotten while its stream was still live: %v", err)
	}
	if !h.svc.HasLiveChatController(testSession) {
		t.Fatal("live-controller guard cleared before the provider stream ended")
	}

	base.closeOnce.Do(func() { close(base.events) })
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := h.svc.Controller(testSession); errors.Is(err, chatsvc.ErrNoController) {
			if h.svc.HasLiveChatController(testSession) {
				t.Fatal("live-controller guard remained set after registry release")
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("controller registry did not release the stopped stream")
}

func TestStartWaitsForStoppedControllerCleanupBeforeRelaunch(t *testing.T) {
	st := openStore(t)
	first := newFakeConversation()
	second := newFakeConversation()
	driver := &sequenceDriver{conversations: []ports.ChatConversation{first, second}}
	var idMu sync.Mutex
	nextID := 0
	svc := chatsvc.New(chatsvc.Options{
		Store: st, Sessions: st,
		Drivers: fakeRegistry{driver: driver},
		Log:     slog.New(slog.DiscardHandler),
		NewID: func() string {
			idMu.Lock()
			defer idMu.Unlock()
			nextID++
			return fmt.Sprintf("relaunch-id-%d", nextID)
		},
	})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })

	firstController, err := svc.Start(context.Background(), chatsvc.StartConfig{
		SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex,
		WorkspacePath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("first Start: %v", err)
	}
	first.emit(ports.ChatEvent{
		Kind:            ports.ChatEventControllerState,
		ControllerState: ports.ChatControllerStopped,
	})
	deadline := time.Now().Add(time.Second)
	for firstController.State() != ports.ChatControllerStopped && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := firstController.State(); got != ports.ChatControllerStopped {
		t.Fatalf("first controller state = %q, want stopped", got)
	}
	if svc.HasLiveChatController(testSession) {
		t.Fatal("stopped controller reported live")
	}

	replacementWorkspace := t.TempDir()
	waitCtx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	controller, err := svc.Start(waitCtx, chatsvc.StartConfig{
		SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex,
		WorkspacePath: replacementWorkspace, ProviderConversationID: "thread-1",
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Start while stopped controller was cleaning up = controller=%p err=%v, want deadline", controller, err)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("close first conversation: %v", err)
	}
	firstController.Wait()
	replacement, err := svc.Start(context.Background(), chatsvc.StartConfig{
		SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex,
		WorkspacePath: replacementWorkspace, ProviderConversationID: "thread-1",
	})
	if err != nil {
		t.Fatalf("relaunch: %v", err)
	}
	if replacement == firstController {
		t.Fatal("relaunch returned the stopped controller")
	}
}

// The cancellation belongs to the moment stop was pressed. A message typed after
// that is the user asking for new work, and must not be swept up by it.
func TestMessageTypedAfterStopIsStillDelivered(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "running", ClientMessageID: "c1",
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	h.conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State == domain.TurnStateRunning
	})
	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "before stop", ClientMessageID: "c2",
	}); err != nil {
		t.Fatalf("Send before stop: %v", err)
	}

	if err := h.svc.Interrupt(ctx, testSession); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}

	// The user changes their mind and types again while the interrupt lands.
	h.advance(time.Second)
	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "after stop", ClientMessageID: "c3",
	}); err != nil {
		t.Fatalf("Send after stop: %v", err)
	}
	h.conv.emit(ports.ChatEvent{
		Kind: ports.ChatEventTurnCompleted, ProviderTurnID: "provider-turn-1",
		TurnState: domain.TurnStateInterrupted,
	})

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return turnStateByText(t, s)["after stop"] == domain.TurnStateRunning
	})
	states := turnStateByText(t, snapshot)
	if states["before stop"] != domain.TurnStateInterrupted {
		t.Errorf("pre-stop message = %q, want interrupted", states["before stop"])
	}
	if got := h.conv.sentTexts(); len(got) != 2 || got[1] != "after stop" {
		t.Fatalf("provider received %v, want the post-stop message delivered", got)
	}
}

func errorsIs(err, target error) bool {
	for err != nil {
		if errors.Is(err, target) {
			return true
		}
		unwrapped, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapped.Unwrap()
	}
	return false
}

// The initial prompt is the user's task brief, so it must render as their message
// rather than as a system notice. Origin records who authored a message, not who
// delivered it to the provider.
func TestInitialPromptIsAttributedToTheUser(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if _, err := h.svc.StartChatTurn(ctx, testSession, "Explain the whole design system"); err != nil {
		t.Fatalf("StartChatTurn: %v", err)
	}

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Messages) >= 1
	})
	first := snapshot.Messages[0]
	if first.Origin != domain.MessageOriginHuman {
		t.Fatalf("initial prompt origin = %q, want %q — the daemon delivers it but the user wrote it",
			first.Origin, domain.MessageOriginHuman)
	}
	if first.Role != domain.MessageRoleUser {
		t.Errorf("initial prompt role = %q, want user", first.Role)
	}
}

// A relayed message is Kennel carrying someone else's words: `kennel send`, or an
// orchestrator writing to a worker. It must be attributed to automation, not
// passed off as something the user typed here — the timeline distinguishes the
// two structurally, and a reader should never have to infer it from a prefix.
func TestRelayedMessageIsAttributedToAutomation(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if _, err := h.svc.RelayChatTurn(ctx, testSession, "orchestrator: rebase onto main"); err != nil {
		t.Fatalf("RelayChatTurn: %v", err)
	}

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Messages) >= 1
	})
	msg := snapshot.Messages[0]
	if msg.Origin != domain.MessageOriginAutomation {
		t.Fatalf("relay origin = %q, want %q", msg.Origin, domain.MessageOriginAutomation)
	}
	if msg.Role != domain.MessageRoleUser {
		t.Errorf("relay role = %q; a relay is still an inbound request", msg.Role)
	}
	if got := h.conv.sentTexts(); len(got) != 1 || got[0] != "orchestrator: rebase onto main" {
		t.Fatalf("provider received %v, want the relayed text dispatched", got)
	}
}

// interruptRecorder answers turn/interrupt the way the provider does: it refuses
// any turn it has not been told to consider active.
type interruptRecorder struct {
	*fakeConversation
	activeMu sync.Mutex
	active   map[string]bool
	attempts []string
}

// blockingDispatchConversation exposes the interval in which Send owns sendMu
// but has not yet published pendingTurnID. Stop must wait for dispatch and then
// retain the normal provider-ack grace period.
type blockingDispatchConversation struct {
	*interruptRecorder
	sendStarted chan struct{}
	releaseSend chan struct{}
	attempted   chan struct{}
	startOnce   sync.Once
	attemptOnce sync.Once
}

func newBlockingDispatchConversation() *blockingDispatchConversation {
	return &blockingDispatchConversation{
		interruptRecorder: newInterruptRecorder(),
		sendStarted:       make(chan struct{}),
		releaseSend:       make(chan struct{}),
		attempted:         make(chan struct{}),
	}
}

func (c *blockingDispatchConversation) SendTurn(
	ctx context.Context,
	msg ports.ChatUserMessage,
) (ports.ChatTurnRef, error) {
	c.startOnce.Do(func() { close(c.sendStarted) })
	select {
	case <-c.releaseSend:
		return c.fakeConversation.SendTurn(ctx, msg)
	case <-ctx.Done():
		return ports.ChatTurnRef{}, ctx.Err()
	}
}

func (c *blockingDispatchConversation) Interrupt(ctx context.Context, turn string) error {
	c.attemptOnce.Do(func() { close(c.attempted) })
	return c.interruptRecorder.Interrupt(ctx, turn)
}

// blockingInterruptRefusalConversation holds the provider refusal open so a
// message can arrive after Stop began but before reconciliation runs.
type blockingInterruptRefusalConversation struct {
	*fakeConversation
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

func newBlockingInterruptRefusalConversation() *blockingInterruptRefusalConversation {
	return &blockingInterruptRefusalConversation{
		fakeConversation: newFakeConversation(),
		started:          make(chan struct{}),
		release:          make(chan struct{}),
	}
}

func (c *blockingInterruptRefusalConversation) Interrupt(ctx context.Context, _ string) error {
	c.startedOnce.Do(func() { close(c.started) })
	select {
	case <-c.release:
		return ports.ErrChatNoActiveTurn
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *blockingInterruptRefusalConversation) unblock() {
	c.releaseOnce.Do(func() { close(c.release) })
}

// completionBeforeRefusalConversation lets the committed completion win the
// controller's send lock before the provider's stale refusal reaches Interrupt.
type completionBeforeRefusalConversation struct {
	*fakeConversation
	emitted     chan struct{}
	release     chan struct{}
	emittedOnce sync.Once
	releaseOnce sync.Once
}

func newCompletionBeforeRefusalConversation() *completionBeforeRefusalConversation {
	return &completionBeforeRefusalConversation{
		fakeConversation: newFakeConversation(),
		emitted:          make(chan struct{}),
		release:          make(chan struct{}),
	}
}

func (c *completionBeforeRefusalConversation) Interrupt(ctx context.Context, turn string) error {
	c.emittedOnce.Do(func() {
		c.emit(ports.ChatEvent{
			Kind: ports.ChatEventTurnCompleted, ProviderTurnID: turn,
			TurnState: domain.TurnStateCompleted,
		})
		close(c.emitted)
	})
	select {
	case <-c.release:
		return ports.ErrChatNoActiveTurn
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *completionBeforeRefusalConversation) unblock() {
	c.releaseOnce.Do(func() { close(c.release) })
}

func newInterruptRecorder() *interruptRecorder {
	return &interruptRecorder{fakeConversation: newFakeConversation(), active: map[string]bool{}}
}

func (r *interruptRecorder) markActive(turn string) {
	r.activeMu.Lock()
	defer r.activeMu.Unlock()
	r.active[turn] = true
}

func (r *interruptRecorder) Interrupt(_ context.Context, turn string) error {
	r.activeMu.Lock()
	defer r.activeMu.Unlock()
	r.attempts = append(r.attempts, turn)
	if !r.active[turn] {
		return ports.ErrChatNoActiveTurn
	}
	return nil
}

func (r *interruptRecorder) attemptCount() int {
	r.activeMu.Lock()
	defer r.activeMu.Unlock()
	return len(r.attempts)
}

// Stop appears the moment a message is sent, so a user can press it before the
// provider has acknowledged the turn — and a provider refuses to cancel a turn it
// does not yet consider active. Interrupt waits out that gap rather than handing
// back a failure in the exact moment someone realizes they sent the wrong thing.
func TestInterruptWaitsForTheProviderToAcknowledgeTheTurn(t *testing.T) {
	conv := newInterruptRecorder()
	h := newHarnessWithConversation(t, conv)
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{Text: "go", ClientMessageID: "c1"}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// The acknowledgement lands while Interrupt is already waiting.
	go func() {
		time.Sleep(150 * time.Millisecond)
		conv.markActive("provider-turn-1")
		conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	}()

	if err := h.svc.Interrupt(ctx, testSession); err != nil {
		t.Fatalf("Interrupt raced the provider's acknowledgement: %v", err)
	}
	if got := conv.attemptCount(); got != 1 {
		t.Errorf("interrupt attempts = %d, want 1", got)
	}
}

// Interrupt can begin while Send still owns the dispatch lock. Once dispatch
// finishes, Stop must still wait for turn-started rather than treating the
// provider's early refusal as proof that the new turn is stale.
func TestInterruptWaitsForAcknowledgementAfterRacingDispatch(t *testing.T) {
	conv := newBlockingDispatchConversation()
	h := newHarnessWithConversation(t, conv)
	ctx := context.Background()

	sendDone := make(chan error, 1)
	go func() {
		_, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
			Text: "go", ClientMessageID: "dispatch-race",
		})
		sendDone <- err
	}()
	select {
	case <-conv.sendStarted:
	case <-time.After(4 * time.Second):
		t.Fatal("Send did not enter provider dispatch")
	}

	interruptDone := make(chan error, 1)
	go func() { interruptDone <- h.svc.Interrupt(ctx, testSession) }()
	close(conv.releaseSend)
	if err := <-sendDone; err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case <-conv.attempted:
		t.Fatal("Interrupt reached the provider before turn-started acknowledgement")
	case <-time.After(100 * time.Millisecond):
	}
	conv.markActive("provider-turn-1")
	conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	if err := <-interruptDone; err != nil {
		t.Fatalf("Interrupt after acknowledgement: %v", err)
	}
}

// A provider that refuses because the turn is genuinely gone must not strand the
// user: the durable row still says running — that is what the UI renders — so
// Interrupt reconciles it as interrupted instead of answering "nothing to stop"
// while the Working bar stays up.
func TestProviderRefusalReconcilesTheDurableTurnAsInterrupted(t *testing.T) {
	conv := newInterruptRecorder() // never marks anything active
	h := newHarnessWithConversation(t, conv)
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{Text: "go", ClientMessageID: "c1"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State == domain.TurnStateRunning
	})

	if err := h.svc.Interrupt(ctx, testSession); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State == domain.TurnStateInterrupted
	})
	if got := snapshot.Turns[0].ErrorMessage; got != "" {
		t.Errorf("interrupted turn error message = %q, want empty", got)
	}
	if !hasActivitySignal(h.activity.snapshot(), domain.ActivityIdle, "chat.interrupt.reconciled") {
		t.Errorf("no idle signal after reconciliation; signals = %v", h.activity.snapshot())
	}
}

// The recovery path must keep the same cutoff as the Stop request. The composer
// remains available while provider cancellation is pending, and a later prompt
// is new user intent rather than part of the queue that Stop was asked to clear.
func TestProviderRefusalPreservesMessageQueuedAfterStop(t *testing.T) {
	conv := newBlockingInterruptRefusalConversation()
	t.Cleanup(conv.unblock)
	h := newHarnessWithConversation(t, conv)
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "running", ClientMessageID: "c1",
	}); err != nil {
		t.Fatalf("Send running: %v", err)
	}
	conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State == domain.TurnStateRunning
	})
	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "before stop", ClientMessageID: "c2",
	}); err != nil {
		t.Fatalf("Send before stop: %v", err)
	}

	interruptDone := make(chan error, 1)
	go func() { interruptDone <- h.svc.Interrupt(ctx, testSession) }()
	select {
	case <-conv.started:
	case <-time.After(4 * time.Second):
		t.Fatal("provider interrupt did not start")
	}

	h.advance(time.Second)
	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "after stop", ClientMessageID: "c3",
	}); err != nil {
		t.Fatalf("Send after stop: %v", err)
	}
	conv.unblock()
	if err := <-interruptDone; err != nil {
		t.Fatalf("Interrupt: %v", err)
	}

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		states := turnStateByText(t, s)
		return states["before stop"] == domain.TurnStateInterrupted &&
			states["after stop"] != domain.TurnStateQueued
	})
	states := turnStateByText(t, snapshot)
	if got := states["after stop"]; got != domain.TurnStateRunning {
		t.Fatalf("post-stop message = %q, want running", got)
	}
	if got := conv.sentTexts(); len(got) != 2 || got[1] != "after stop" {
		t.Fatalf("provider received %v, want the post-stop message delivered", got)
	}
}

// A provider can publish completion just before its interrupt call reports that
// no active turn remains. If that completion commits first, reconciliation must
// not overwrite it or the queue transition it already performed.
func TestProviderRefusalDoesNotOverwriteCommittedCompletion(t *testing.T) {
	conv := newCompletionBeforeRefusalConversation()
	t.Cleanup(conv.unblock)
	h := newHarnessWithConversation(t, conv)
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "running", ClientMessageID: "c1",
	}); err != nil {
		t.Fatalf("Send running: %v", err)
	}
	conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State == domain.TurnStateRunning
	})
	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "before stop", ClientMessageID: "c2",
	}); err != nil {
		t.Fatalf("Send before stop: %v", err)
	}

	interruptDone := make(chan error, 1)
	go func() { interruptDone <- h.svc.Interrupt(ctx, testSession) }()
	select {
	case <-conv.emitted:
	case <-time.After(4 * time.Second):
		t.Fatal("provider completion was not emitted")
	}
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		states := turnStateByText(t, s)
		return states["running"] == domain.TurnStateCompleted &&
			states["before stop"] == domain.TurnStateInterrupted
	})

	conv.unblock()
	if err := <-interruptDone; err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return turnStateByText(t, s)["running"].Terminal()
	})
	if got := turnStateByText(t, snapshot)["running"]; got != domain.TurnStateCompleted {
		t.Fatalf("turn after committed completion = %q, want completed", got)
	}
}

func hasActivitySignal(signals []ports.ActivitySignal, state domain.ActivityState, event string) bool {
	for _, s := range signals {
		if s.State == state && s.Event == event {
			return true
		}
	}
	return false
}

// The #3749 desync: a turn is 'running' on disk while the in-memory controller
// has no pendingTurnID (a completed projection failed, or the daemon restarted
// mid-turn). The UI reads disk and shows "Working"; Stop must act on what the
// user sees rather than refuse with CHAT_NO_ACTIVE_TURN.
func TestInterruptReconcilesStaleRunningTurnOnDisk(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// Create the running turn directly in the store, bypassing the controller so
	// its pendingTurnID stays empty — the desynced state.
	const providerTurnID = "stale-running-turn"
	if err := h.st.AdoptProviderTurn(ctx, h.ctrl.ConversationID(), testSession,
		h.ctrl.Generation(), "stale-turn-id", providerTurnID, h.now()); err != nil {
		t.Fatalf("AdoptProviderTurn: %v", err)
	}
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State == domain.TurnStateRunning
	})

	if err := h.svc.Interrupt(ctx, testSession); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}

	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State == domain.TurnStateInterrupted
	})
	if !hasActivitySignal(h.activity.snapshot(), domain.ActivityIdle, "chat.interrupt.reconciled") {
		t.Errorf("no idle signal after reconciliation; signals = %v", h.activity.snapshot())
	}
}

// A root and a nested provider turn can both be durably running. The UI renders
// the oldest visible row first, but clearing only that row would merely reveal a
// second Working bar. Recovery cancels the visible root and settles the full
// visible running set.
func TestInterruptReconcilesAllVisibleRunningTurns(t *testing.T) {
	conv := newInterruptRecorder()
	h := newHarnessWithConversation(t, conv)
	ctx := context.Background()

	if err := h.st.AdoptProviderTurn(ctx, h.ctrl.ConversationID(), testSession,
		h.ctrl.Generation(), "root-turn", "provider-root", h.now()); err != nil {
		t.Fatalf("AdoptProviderTurn root: %v", err)
	}
	h.advance(time.Second)
	if err := h.st.AdoptProviderTurn(ctx, h.ctrl.ConversationID(), testSession,
		h.ctrl.Generation(), "child-turn", "provider-child", h.now()); err != nil {
		t.Fatalf("AdoptProviderTurn child: %v", err)
	}
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 2 && s.Turns[0].State == domain.TurnStateRunning &&
			s.Turns[1].State == domain.TurnStateRunning
	})

	if err := h.svc.Interrupt(ctx, testSession); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 2 && s.Turns[0].State.Terminal() && s.Turns[1].State.Terminal()
	})
	for _, turn := range snapshot.Turns {
		if turn.State != domain.TurnStateInterrupted {
			t.Errorf("turn %s = %q, want interrupted", turn.ProviderTurnID, turn.State)
		}
	}
	conv.activeMu.Lock()
	attempts := append([]string(nil), conv.attempts...)
	conv.activeMu.Unlock()
	if len(attempts) != 1 || attempts[0] != "provider-root" {
		t.Fatalf("provider interrupt attempts = %v, want visible root first", attempts)
	}
}

// In the no-memory recovery path, provider cancellation happens under sendMu.
// A prompt submitted after Stop therefore waits for stale settlement and then
// starts as new work instead of replacing the recovery target.
func TestInterruptDurableFallbackPreservesPostStopSend(t *testing.T) {
	conv := newBlockingInterruptRefusalConversation()
	t.Cleanup(conv.unblock)
	h := newHarnessWithConversation(t, conv)
	ctx := context.Background()

	if err := h.st.AdoptProviderTurn(ctx, h.ctrl.ConversationID(), testSession,
		h.ctrl.Generation(), "stale-turn", "provider-stale", h.now()); err != nil {
		t.Fatalf("AdoptProviderTurn: %v", err)
	}
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State == domain.TurnStateRunning
	})

	interruptDone := make(chan error, 1)
	go func() { interruptDone <- h.svc.Interrupt(ctx, testSession) }()
	select {
	case <-conv.started:
	case <-time.After(4 * time.Second):
		t.Fatal("durable fallback did not reach provider interrupt")
	}

	h.advance(time.Second)
	sendDone := make(chan error, 1)
	go func() {
		_, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
			Text: "after stop", ClientMessageID: "post-stop-fallback",
		})
		sendDone <- err
	}()
	select {
	case err := <-sendDone:
		t.Fatalf("post-Stop Send crossed reconciliation lock early: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	conv.unblock()
	if err := <-interruptDone; err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	if err := <-sendDone; err != nil {
		t.Fatalf("post-Stop Send: %v", err)
	}
	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 2 && s.Turns[0].State == domain.TurnStateInterrupted &&
			s.Turns[1].State == domain.TurnStateRunning
	})
	if snapshot.Turns[0].ProviderTurnID != "provider-stale" {
		t.Fatalf("reconciled turn = %q, want provider-stale", snapshot.Turns[0].ProviderTurnID)
	}
	if got := conv.sentTexts(); len(got) != 1 || got[0] != "after stop" {
		t.Fatalf("provider received %v, want only post-Stop work", got)
	}
}

// When memory and disk agree there is nothing running, Interrupt must still
// answer ErrNoActiveTurn — the disk fallback must not invent work to stop.
func TestInterruptReturnsNoActiveTurnWhenNoRunningTurnAnywhere(t *testing.T) {
	h := newHarness(t)

	err := h.svc.Interrupt(context.Background(), testSession)
	if !errorsIs(err, chatsvc.ErrNoActiveTurn) {
		t.Fatalf("err = %v, want ErrNoActiveTurn", err)
	}
}

// Reconciliation is still the user's brake: anything queued behind the stale
// turn is cancelled, not released into the provider.
func TestInterruptReconciliationCancelsQueuedTurns(t *testing.T) {
	conv := newInterruptRecorder() // never marks anything active
	h := newHarnessWithConversation(t, conv)
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "running", ClientMessageID: "c1",
	}); err != nil {
		t.Fatalf("Send running: %v", err)
	}
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State == domain.TurnStateRunning
	})
	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "queued", ClientMessageID: "c2",
	}); err != nil {
		t.Fatalf("Send queued: %v", err)
	}

	if err := h.svc.Interrupt(ctx, testSession); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		states := turnStateByText(t, s)
		return states["running"] == domain.TurnStateInterrupted &&
			states["queued"] == domain.TurnStateInterrupted
	})
	states := turnStateByText(t, snapshot)
	if states["queued"] != domain.TurnStateInterrupted {
		t.Errorf("queued turn = %q, want interrupted (cancelled, not dispatched)", states["queued"])
	}
	if got := h.conv.sentTexts(); len(got) != 1 {
		t.Fatalf("provider received %v; reconciliation must not release the queue", got)
	}
}

// A daemon that is killed never runs its own cleanup, so whatever the dead
// controller left in flight is still marked live on disk. The next controller to
// come up has to close it out: nothing else ever will, and until then the timeline
// claims a turn is running and a queued message is waiting to be sent behind a
// controller that no longer exists.
func TestStartSettlesWorkLeftByAKilledController(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// A turn in flight and a message queued behind it, exactly as a crash leaves them.
	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{Text: "running", ClientMessageID: "c1"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	h.conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State == domain.TurnStateRunning
	})
	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{Text: "queued", ClientMessageID: "c2"}); err != nil {
		t.Fatalf("mid-turn Send: %v", err)
	}
	h.conv.emit(ports.ChatEvent{
		Kind: ports.ChatEventApprovalRequested, ProviderTurnID: "provider-turn-1",
		ProviderItemID: "9", RequestID: "9", ActivityKind: domain.ActivityKindCommand,
		ActivityStatus: domain.ActivityStatusPending, Summary: "Run something",
	})
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool { return len(s.Activities) == 1 })

	// A killed daemon leaves the rows mid-flight and takes its service with it, so
	// the next controller comes up in a NEW service over the SAME store. Building
	// it that way rather than reusing this one is the point: nothing in the old
	// process gets a chance to clean up.
	next := chatsvc.New(chatsvc.Options{
		Store:    h.st,
		Sessions: h.st,
		Drivers:  fakeRegistry{driver: fakeDriver{conv: newFakeConversation()}},
		Log:      slog.New(slog.DiscardHandler),
		NewID:    func() string { return "next-" + fmt.Sprint(time.Now().UnixNano()) },
		Now:      h.now,
	})
	t.Cleanup(func() { _ = next.Stop(context.Background(), testSession) })

	if _, err := next.Start(ctx, chatsvc.StartConfig{
		SessionID:              testSession,
		ProjectID:              testProject,
		Harness:                domain.HarnessCodex,
		WorkspacePath:          t.TempDir(),
		ProviderConversationID: "thread-1",
	}); err != nil {
		t.Fatalf("Start after crash: %v", err)
	}

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		for _, turn := range s.Turns {
			if !turn.State.Terminal() {
				return false
			}
		}
		return len(s.Turns) == 2
	})
	states := turnStateByText(t, snapshot)
	if states["running"] != domain.TurnStateFailed {
		t.Errorf("turn abandoned mid-flight = %q, want failed", states["running"])
	}
	if states["queued"] != domain.TurnStateFailed {
		t.Errorf("message left queued by a dead controller = %q; nothing would ever send it",
			states["queued"])
	}
	if got := snapshot.Activities[0].Status; got == domain.ActivityStatusPending {
		t.Error("approval left pending by a dead controller; the user can never answer it")
	}
}

// Usage is current state, not history. The provider reports it after every tool
// call, so the projection must overwrite: a row per report is what buried the
// conversation, and the conversation is only ever one amount full.
func TestUsageProjectionKeepsOnlyTheLatest(t *testing.T) {
	h := newHarness(t)

	h.conv.emit(
		ports.ChatEvent{Kind: ports.ChatEventUsage, Usage: &ports.ChatUsage{
			ContextUsed: 18055, ContextWindow: 258400,
			InputTokens: 18050, OutputTokens: 5, CachedTokens: 11008, TotalTokens: 18055,
		}},
		ports.ChatEvent{Kind: ports.ChatEventUsage, Usage: &ports.ChatUsage{
			ContextUsed: 42100, ContextWindow: 258400,
			InputTokens: 41000, OutputTokens: 1100, CachedTokens: 20000, TotalTokens: 60155,
		}},
	)

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return s.Conversation.Usage != nil && s.Conversation.Usage.ContextUsed == 42100
	})

	usage := snapshot.Conversation.Usage
	if usage.ContextWindow != 258400 {
		t.Errorf("context window = %d, want 258400", usage.ContextWindow)
	}
	if usage.TotalTokens != 60155 {
		t.Errorf("cumulative total = %d, want the later 60155", usage.TotalTokens)
	}
	// The readout is only meaningful as a fraction; without the window it is a
	// number with no scale, which is what the header used to show.
	if got := usage.ContextFraction(); got < 0.16 || got > 0.17 {
		t.Errorf("context fraction = %v, want roughly 0.163", got)
	}

	// Usage must not become a timeline entry, under any kind.
	for _, activity := range snapshot.Activities {
		if activity.Kind == domain.ActivityKindUsage {
			t.Fatalf("usage was projected as an activity: %+v", activity)
		}
	}
}

// ACP reports context fullness and cumulative token totals in separate messages.
// A later totals update must not erase the context window received just before it.
func TestUsageProjectionMergesIndependentProviderUpdates(t *testing.T) {
	h := newHarness(t)

	h.conv.emit(
		ports.ChatEvent{Kind: ports.ChatEventUsage, Usage: &ports.ChatUsage{
			ContextUsed: 74300, ContextWindow: 1_000_000, ContextKnown: true,
		}},
		ports.ChatEvent{Kind: ports.ChatEventUsage, Usage: &ports.ChatUsage{
			InputTokens: 2, OutputTokens: 59, CachedTokens: 74216,
			TotalTokens: 74277, TotalsKnown: true,
		}},
	)

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return s.Conversation.Usage != nil && s.Conversation.Usage.TotalTokens == 74277
	})
	usage := snapshot.Conversation.Usage
	if usage.ContextUsed != 74300 || usage.ContextWindow != 1_000_000 {
		t.Fatalf("context usage was erased by totals update: %+v", usage)
	}
	if usage.CachedTokens != 74216 {
		t.Fatalf("cumulative totals were not merged: %+v", usage)
	}
}

// A model the provider states no window for still reports its tokens. The meter
// has to say "unknown" rather than draw an empty bar for a conversation that may
// be nearly full.
func TestUsageProjectionWithoutContextWindow(t *testing.T) {
	h := newHarness(t)

	h.conv.emit(ports.ChatEvent{Kind: ports.ChatEventUsage, Usage: &ports.ChatUsage{
		ContextUsed: 900, TotalTokens: 900, InputTokens: 800, OutputTokens: 100,
	}})

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return s.Conversation.Usage != nil
	})
	if got := snapshot.Conversation.Usage.ContextFraction(); got != -1 {
		t.Errorf("context fraction = %v, want -1 for an unknown window", got)
	}
}

// Rate limits are current state too, and an unreported window must survive a round
// trip through the database as unreported rather than as a reassuring zero.
func TestRateLimitProjectionKeepsOnlyTheLatest(t *testing.T) {
	h := newHarness(t)

	h.conv.emit(
		ports.ChatEvent{Kind: ports.ChatEventRateLimits, RateLimits: &ports.ChatRateLimits{
			PrimaryUsedPercent: 12, SecondaryUsedPercent: -1,
			PrimaryResetsInSeconds: 600, PlanLabel: "pro",
		}},
		ports.ChatEvent{Kind: ports.ChatEventRateLimits, RateLimits: &ports.ChatRateLimits{
			PrimaryUsedPercent: 71, SecondaryUsedPercent: -1,
			PrimaryResetsInSeconds: 490444, PlanLabel: "pro",
		}},
	)

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return s.Conversation.RateLimits != nil && s.Conversation.RateLimits.PrimaryUsedPercent == 71
	})

	limits := snapshot.Conversation.RateLimits
	if limits.SecondaryUsedPercent >= 0 {
		t.Errorf("secondary = %v; an unreported window must not read as untouched quota",
			limits.SecondaryUsedPercent)
	}
	if limits.PrimaryResetsInSeconds != 490444 {
		t.Errorf("primary resets in %d, want 490444", limits.PrimaryResetsInSeconds)
	}
	if limits.PlanLabel != "pro" {
		t.Errorf("plan = %q, want pro", limits.PlanLabel)
	}
	if got := limits.WorstUsedPercent(); got != 71 {
		t.Errorf("worst window = %v, want 71", got)
	}
}

// Nothing reported yet is distinct from a conversation using nothing: the snapshot
// leaves both nil so a client can withhold the meter rather than draw an empty one.
func TestSnapshotOmitsUsageUntilTheProviderReports(t *testing.T) {
	h := newHarness(t)

	snapshot, err := h.st.LoadConversationSnapshot(context.Background(), h.ctrl.ConversationID())
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if snapshot.Conversation.Usage != nil {
		t.Errorf("usage = %+v, want nil before the provider reports", snapshot.Conversation.Usage)
	}
	if snapshot.Conversation.RateLimits != nil {
		t.Errorf("rate limits = %+v, want nil before the provider reports",
			snapshot.Conversation.RateLimits)
	}
}

/* ---- compaction --------------------------------------------------------- */

// compactingConversation is a provider that can reclaim context. The plain fake
// deliberately cannot, so the unsupported path stays exercised by every other test.
type compactingConversation struct {
	*fakeConversation

	compactMu sync.Mutex
	calls     int
}

func newCompactingConversation() *compactingConversation {
	return &compactingConversation{fakeConversation: newFakeConversation()}
}

func (c *compactingConversation) Compact(context.Context) (ports.ChatCompactionResult, error) {
	c.compactMu.Lock()
	defer c.compactMu.Unlock()
	c.calls++
	// What is about to be reclaimed, not what was: the real provider accepts the
	// request and does the work as its own turn afterwards.
	return ports.ChatCompactionResult{TokensBefore: 15650}, nil
}

func (c *compactingConversation) compactCalls() int {
	c.compactMu.Lock()
	defer c.compactMu.Unlock()
	return c.calls
}

// A compaction has to survive a restart, which means it has to be a row. Without
// one, a conversation that quietly lost half its history has nothing in the
// timeline to explain the gap, and reads as if the agent simply forgot.
func TestCompactionIsProjectedAsATimelineFact(t *testing.T) {
	conv := newCompactingConversation()
	h := newHarnessWithConversation(t, conv)

	conv.emit(ports.ChatEvent{
		Kind:           ports.ChatEventCompacted,
		ProviderTurnID: "compact-turn",
		ProviderItemID: "cc-1",
		Summary:        "Compacted history, freeing 11.0k tokens",
		Detail:         []byte(`{"tokensBefore":15650,"tokensAfter":4632,"tokensReclaimed":11018}`),
	})

	// Both writes, because the row and the conversation flag are two statements and
	// this test asserts on both: waiting only on the row races the flag.
	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Activities) == 1 && s.Conversation.CompactedAt != nil
	})
	activity := snapshot.Activities[0]
	if activity.Kind != domain.ActivityKindSystem {
		t.Errorf("kind = %q, want system", activity.Kind)
	}
	if activity.Status != domain.ActivityStatusCompleted {
		t.Errorf("status = %q, want completed", activity.Status)
	}
	if activity.Summary != "Compacted history, freeing 11.0k tokens" {
		t.Errorf("summary = %q", activity.Summary)
	}
	if activity.ProviderItemID != "cc-1" {
		t.Errorf("provider item id = %q, want cc-1 so a replay updates this row", activity.ProviderItemID)
	}
	// Not attached to a turn: the provider ran the compaction in a turn Kennel never
	// dispatched, so filing the row under it would attribute the entry to work the
	// user never asked for.
	if activity.TurnID != "" {
		t.Errorf("turn id = %q, want none", activity.TurnID)
	}
	var detail struct{ TokensReclaimed int64 }
	if err := json.Unmarshal(activity.Detail, &detail); err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.TokensReclaimed != 11018 {
		t.Errorf("reclaimed = %d, want 11018", detail.TokensReclaimed)
	}

	// The conversation itself records that compaction has run, so a client does not
	// have to scan an unbounded timeline to find out.
	if snapshot.Conversation.CompactedAt == nil {
		t.Error("conversation was not marked compacted")
	}
}

// A compaction replayed across a reconnect updates the row it already has. Two
// entries for one compaction would read as two, and the reclaim would look twice
// as large as it was.
func TestCompactionReplayDoesNotDuplicateTheRow(t *testing.T) {
	conv := newCompactingConversation()
	h := newHarnessWithConversation(t, conv)

	for range 2 {
		conv.emit(ports.ChatEvent{
			Kind:           ports.ChatEventCompacted,
			ProviderItemID: "cc-1",
			Summary:        "Compacted the conversation history",
		})
	}
	// A marker after both, so this waits on an event rather than on a timeout.
	conv.emit(ports.ChatEvent{
		Kind: ports.ChatEventActivityCompleted, ProviderItemID: "exec-1",
		ActivityKind: domain.ActivityKindCommand, ActivityStatus: domain.ActivityStatusCompleted,
		Summary: "date -u",
	})

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		for _, activity := range s.Activities {
			if activity.ProviderItemID == "exec-1" {
				return true
			}
		}
		return false
	})
	compactions := 0
	for _, activity := range snapshot.Activities {
		if activity.Kind == domain.ActivityKindSystem {
			compactions++
		}
	}
	if compactions != 1 {
		t.Fatalf("got %d compaction rows for one compaction", compactions)
	}
}

func TestCompactReportsWhatIsAboutToBeReclaimed(t *testing.T) {
	conv := newCompactingConversation()
	h := newHarnessWithConversation(t, conv)

	result, err := h.svc.Compact(context.Background(), testSession)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if result.TokensBefore != 15650 {
		t.Errorf("tokensBefore = %d, want 15650", result.TokensBefore)
	}
	// Zero means "not yet known", not "reclaimed everything". The settled figures
	// reach the client on the timeline.
	if result.TokensAfter != 0 {
		t.Errorf("tokensAfter = %d, want 0 while the reclaim is in flight", result.TokensAfter)
	}
	if conv.compactCalls() != 1 {
		t.Errorf("provider called %d times, want 1", conv.compactCalls())
	}
}

// A provider with no way to reclaim context gets a typed answer, so the client can
// stop offering the control instead of surfacing an internal failure the user
// cannot act on. The plain fake conversation does not implement ChatCompactor.
func TestCompactOnAProviderThatCannotIsTyped(t *testing.T) {
	h := newHarness(t)

	_, err := h.svc.Compact(context.Background(), testSession)
	if !errors.Is(err, chatsvc.ErrCompactionUnsupported) {
		t.Fatalf("err = %v, want ErrCompactionUnsupported", err)
	}
}

// Measured twice against a live app-server: thread/compact/start mid-turn silently
// interrupts the running turn and reports it as interrupted, then compacts. Losing
// work the user is waiting on as a side effect of housekeeping is not something to
// discover afterwards from the timeline, so Kennel refuses and makes them stop it.
func TestCompactRefusesWhileATurnIsInFlight(t *testing.T) {
	conv := newCompactingConversation()
	h := newHarnessWithConversation(t, conv)
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "start something long", Origin: domain.MessageOriginHuman,
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if _, err := h.svc.Compact(ctx, testSession); !errors.Is(err, chatsvc.ErrCompactionWhileBusy) {
		t.Fatalf("err = %v, want ErrCompactionWhileBusy", err)
	}
	if conv.compactCalls() != 0 {
		t.Error("the provider was asked to compact anyway; the running turn would have been discarded")
	}

	// Once the turn settles it is allowed again.
	conv.emit(ports.ChatEvent{
		Kind: ports.ChatEventTurnCompleted, ProviderTurnID: "provider-turn-1",
		TurnState: domain.TurnStateCompleted,
	})
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State == domain.TurnStateCompleted
	})
	if _, err := h.svc.Compact(ctx, testSession); err != nil {
		t.Fatalf("Compact after the turn settled: %v", err)
	}
}

// A provider can start a turn Kennel never dispatched: a compaction runs as its own
// turn, and so does work the provider resumes from its own history. Without a row
// for it, every item that turn emits correlates to no turn — the activities arrive
// with an empty turn id and the timeline silently stops grouping them, which reads
// to a user as the conversation falling apart.
func TestProviderStartedTurnIsAdoptedSoItsItemsCorrelate(t *testing.T) {
	h := newHarness(t)

	// No Send: this turn is entirely the provider's doing.
	h.conv.emit(
		ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-owned-1"},
		ports.ChatEvent{
			Kind: ports.ChatEventActivityCompleted, ProviderTurnID: "provider-owned-1",
			ProviderItemID: "exec-1", ActivityKind: domain.ActivityKindCommand,
			ActivityStatus: domain.ActivityStatusCompleted, Summary: "rg --files",
		},
		ports.ChatEvent{
			Kind: ports.ChatEventActivityCompleted, ProviderTurnID: "provider-owned-1",
			ProviderItemID: "exec-2", ActivityKind: domain.ActivityKindCommand,
			ActivityStatus: domain.ActivityStatusCompleted, Summary: "sed -n 1,40p x",
		},
	)

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(s.Activities) == 2
	})

	if len(snapshot.Turns) != 1 {
		t.Fatalf("turns = %d, want the provider's turn adopted", len(snapshot.Turns))
	}
	if got := snapshot.Turns[0].ProviderTurnID; got != "provider-owned-1" {
		t.Errorf("adopted turn provider id = %q", got)
	}
	for _, activity := range snapshot.Activities {
		if activity.TurnID == "" {
			t.Errorf("activity %q has no turn id; the timeline cannot group it", activity.Summary)
		}
	}
}

// failingProjectStore injects a projection failure for one event kind,
// simulating a SettleTurn error or tx.Commit failure without corrupting SQLite.
type failingProjectStore struct {
	chatsvc.Store
	failMethod string
	failed     chan struct{}
	failOnce   sync.Once
}

func (s *failingProjectStore) ProjectProviderEvent(
	ctx context.Context, conversationID string, session domain.SessionID,
	generation, providerEventID, method, payloadJSON string, now time.Time,
	project func(context.Context) error,
) (bool, error) {
	if method != s.failMethod {
		return s.Store.ProjectProviderEvent(ctx, conversationID, session, generation,
			providerEventID, method, payloadJSON, now, project)
	}
	projected, err := s.Store.ProjectProviderEvent(ctx, conversationID, session, generation,
		providerEventID, method, payloadJSON, now, func(txCtx context.Context) error {
			if err := project(txCtx); err != nil {
				return err
			}
			// Fail after the projection callback so its SQLite writes roll back. This
			// is the exact boundary that used to leak volatile state out of the tx.
			return errors.New("injected projection failure")
		})
	s.failOnce.Do(func() { close(s.failed) })
	return projected, err
}

func TestGovernedTurnCrashMidProjectionReplaysOneAtomicVisibleEvent(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	firstConv := &dispatchConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatTurnDispatch{
		Acceptance: ports.ChatTurnAcknowledged, Ref: ports.ChatTurnRef{ProviderTurnID: "provider-turn-1"},
		TransportRequestID: 1, TransportSHA256: "projection-sha", TransportBytes: 10, TransportSequence: 1,
	}}
	failingStore := &failingProjectStore{Store: st, failMethod: string(ports.ChatEventActivityStarted), failed: make(chan struct{})}
	first := chatsvc.New(chatsvc.Options{Store: failingStore, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: firstConv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("mid-projection-first")})
	firstCtrl, err := first.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := firstCtrl.Send(context.Background(), ports.ChatUserMessage{Text: "do it", ClientMessageID: "request-mid-projection"}); err != nil {
		t.Fatal(err)
	}
	event := ports.ChatEvent{
		Kind: ports.ChatEventActivityStarted, ProviderEventID: "event-mid-projection", ProviderTurnID: "provider-turn-1",
		ProviderConversationID: firstCtrl.ProviderConversationID(), ProviderItemID: "command-1",
		ActivityKind: domain.ActivityKindCommand, ActivityStatus: domain.ActivityStatusRunning, Summary: "go test ./...",
	}
	firstConv.emit(event)
	select {
	case <-failingStore.failed:
	case <-time.After(time.Second):
		t.Fatal("event did not reach the injected mid-projection rollback")
	}
	snapshot, err := st.LoadConversationSnapshot(context.Background(), firstCtrl.ConversationID())
	if err != nil || len(snapshot.Activities) != 0 {
		t.Fatalf("rolled-back activities=%+v err=%v", snapshot.Activities, err)
	}
	events, err := st.ProviderEventsSince(context.Background(), firstCtrl.ConversationID(), 0, 10)
	if err != nil || len(events) != 0 {
		t.Fatalf("rolled-back archive=%+v err=%v", events, err)
	}

	// A fresh daemon resumes the same native thread. Stable history repeats the
	// event whose prior archive+projection transaction never committed; import
	// must make exactly one visible activity and one archive row.
	history := &nativeHistoryConversation{fakeConversation: newFakeConversation(), events: []ports.ChatEvent{event}}
	second := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: history}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("mid-projection-second")})
	t.Cleanup(func() { _ = second.Stop(context.Background(), testSession) })
	secondCtrl, err := second.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ProviderConversationID: firstCtrl.ProviderConversationID(), ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = st.LoadConversationSnapshot(context.Background(), secondCtrl.ConversationID())
	if err != nil || len(snapshot.Activities) != 1 || snapshot.Activities[0].ProviderItemID != event.ProviderItemID || snapshot.Activities[0].Status != domain.ActivityStatusRunning {
		t.Fatalf("replayed activities=%+v err=%v", snapshot.Activities, err)
	}
	events, err = st.ProviderEventsSince(context.Background(), secondCtrl.ConversationID(), 0, 10)
	if err != nil || len(events) != 1 || events[0].ProviderEventID != event.ProviderEventID {
		t.Fatalf("replayed archive=%+v err=%v", events, err)
	}

	// Repeating the same stable event after restart is a no-op for both archive
	// and projection, so recovery cannot duplicate the visible activity.
	history.emit(event)
	time.Sleep(20 * time.Millisecond)
	snapshot, err = st.LoadConversationSnapshot(context.Background(), secondCtrl.ConversationID())
	events, eventsErr := st.ProviderEventsSince(context.Background(), secondCtrl.ConversationID(), 0, 10)
	if err != nil || eventsErr != nil || len(snapshot.Activities) != 1 || len(events) != 1 {
		t.Fatalf("duplicate replay activities=%+v events=%+v err=%v eventsErr=%v", snapshot.Activities, events, err, eventsErr)
	}
}

// The exact #3749 sequence: the turn/completed projection fails (so the durable
// row stays 'running' and the UI keeps showing "Working"), then the user presses
// Stop and the provider refuses because the turn already ended on its side.
// Interrupt must settle the durable row as interrupted, cancel the queue behind
// it, and report idle — not answer CHAT_NO_ACTIVE_TURN and strand the user.
func TestProjectionFailureThenStopStillStopsTheTurn(t *testing.T) {
	conv := newInterruptRecorder() // never marks anything active -> always refuses
	st := openStore(t)

	var counterMu sync.Mutex
	counter := 0
	activity := &recordingActivity{}
	clock := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	failingStore := &failingProjectStore{
		Store: st, failMethod: string(ports.ChatEventTurnCompleted), failed: make(chan struct{}),
	}
	svc := chatsvc.New(chatsvc.Options{
		Store:    failingStore,
		Sessions: st,
		Drivers:  fakeRegistry{driver: fakeDriver{conv: conv}},
		Activity: activity,
		Log:      slog.New(slog.DiscardHandler),
		NewID: func() string {
			counterMu.Lock()
			defer counterMu.Unlock()
			counter++
			return fmt.Sprintf("id-%03d", counter)
		},
		Now: func() time.Time { return clock },
	})

	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{
		SessionID:     testSession,
		ProjectID:     testProject,
		Harness:       domain.HarnessCodex,
		WorkspacePath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
	ctx := context.Background()

	if _, err := svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "running", ClientMessageID: "c1",
	}); err != nil {
		t.Fatalf("Send running: %v", err)
	}
	conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	awaitStoreSnapshot(t, st, ctrl.ConversationID(), func(s store.ConversationSnapshot) bool {
		return len(s.Turns) == 1 && s.Turns[0].State == domain.TurnStateRunning
	})

	if _, err := svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "queued", ClientMessageID: "c2",
	}); err != nil {
		t.Fatalf("Send queued: %v", err)
	}

	// The completion's projection fails: the durable row stays 'running' and
	// afterProject never runs, so nothing drains and nothing reports idle.
	conv.emit(ports.ChatEvent{
		Kind:           ports.ChatEventTurnCompleted,
		ProviderTurnID: "provider-turn-1",
		TurnState:      domain.TurnStateCompleted,
	})
	select {
	case <-failingStore.failed:
	case <-time.After(4 * time.Second):
		t.Fatal("completion projection did not reach the injected rollback")
	}

	if err := svc.Interrupt(ctx, testSession); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}

	snapshot := awaitStoreSnapshot(t, st, ctrl.ConversationID(), func(s store.ConversationSnapshot) bool {
		states := turnStateByText(t, s)
		return states["running"] == domain.TurnStateInterrupted &&
			states["queued"] == domain.TurnStateInterrupted
	})
	states := turnStateByText(t, snapshot)
	if states["running"] != domain.TurnStateInterrupted {
		t.Errorf("running turn = %q, want interrupted", states["running"])
	}
	if states["queued"] != domain.TurnStateInterrupted {
		t.Errorf("queued turn = %q, want interrupted (cancelled, not dispatched)", states["queued"])
	}
	if got := conv.sentTexts(); len(got) != 1 {
		t.Fatalf("provider received %v; reconciliation must not release the queue", got)
	}
	if !hasActivitySignal(activity.snapshot(), domain.ActivityIdle, "chat.interrupt.reconciled") {
		t.Errorf("no idle signal after reconciliation; signals = %v", activity.snapshot())
	}
}

// Controller-state notifications perform durable cleanup before they change the
// volatile controller state. If that transaction rolls back, memory must continue
// reporting the last committed state rather than claiming the controller stopped.
func TestControllerStateChangesOnlyAfterProjectionCommits(t *testing.T) {
	var failingStore *failingProjectStore
	h := newHarnessWithConversationAndStore(t, nil, func(st *sqlite.Store) chatsvc.Store {
		failingStore = &failingProjectStore{
			Store: st, failMethod: string(ports.ChatEventControllerState), failed: make(chan struct{}),
		}
		return failingStore
	})
	want := h.ctrl.State()

	h.conv.emit(ports.ChatEvent{
		Kind: ports.ChatEventControllerState, ControllerState: ports.ChatControllerStopped,
	})
	select {
	case <-failingStore.failed:
	case <-time.After(4 * time.Second):
		t.Fatal("controller-state projection did not reach the injected rollback")
	}
	if got := h.ctrl.State(); got != want {
		t.Fatalf("controller state after rolled-back projection = %q, want committed %q", got, want)
	}
}

// awaitStoreSnapshot is awaitSnapshot for tests that build their own service
// rather than using the harness.
func awaitStoreSnapshot(t *testing.T, st *sqlite.Store, conversationID string,
	pred func(store.ConversationSnapshot) bool) store.ConversationSnapshot {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last store.ConversationSnapshot
	for time.Now().Before(deadline) {
		snapshot, err := st.LoadConversationSnapshot(context.Background(), conversationID)
		if err != nil {
			t.Fatalf("load snapshot: %v", err)
		}
		last = snapshot
		if pred(snapshot) {
			return snapshot
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("snapshot never satisfied the condition; last had %d messages, %d activities, %d turns",
		len(last.Messages), len(last.Activities), len(last.Turns))
	return last
}

type dispatchConversation struct {
	*fakeConversation
	dispatch   ports.ChatTurnDispatch
	err        error
	onDispatch func()
}

func (c *dispatchConversation) DispatchTurn(_ context.Context, msg ports.ChatUserMessage) (ports.ChatTurnDispatch, error) {
	c.mu.Lock()
	c.sent = append(c.sent, msg)
	c.mu.Unlock()
	if c.onDispatch != nil {
		c.onDispatch()
	}
	return c.dispatch, c.err
}

func governedPolicy(t *testing.T, workspace string) domain.AttemptExecutionPolicy {
	t.Helper()
	p := domain.AttemptExecutionPolicy{
		OutcomeID: "out-1", PlanRevisionID: "plan-1", WorkUnitID: "work-1",
		ContractRevisionNumber: 1, RunBriefCoreDigest: "brief", WorkspaceRoot: workspace,
		RequiredCapabilities: []string{domain.CapabilityWorktreeRead},
		Grants:               []domain.CapabilityGrant{{ID: "grant-1", Name: domain.CapabilityWorktreeRead, Scope: "worktree/*"}},
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestGovernedTurnConcurrentDuplicateAndConflictHaveOneEffect(t *testing.T) {
	for _, tc := range []struct {
		name       string
		messages   []ports.ChatUserMessage
		wantErrors int
	}{
		{name: "exact duplicates", messages: []ports.ChatUserMessage{{Text: "do it", ClientMessageID: "concurrent-key"}, {Text: "do it", ClientMessageID: "concurrent-key"}}},
		{name: "changed fingerprint", messages: []ports.ChatUserMessage{{Text: "first", ClientMessageID: "concurrent-key"}, {Text: "changed", ClientMessageID: "concurrent-key"}}, wantErrors: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := openStore(t)
			workspace := t.TempDir()
			policy := governedPolicy(t, workspace)
			conv := &dispatchConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatTurnDispatch{
				Acceptance: ports.ChatTurnAcknowledged, Ref: ports.ChatTurnRef{ProviderTurnID: "provider-winner"},
				TransportRequestID: 1, TransportSHA256: "winner-sha", TransportBytes: 10, TransportSequence: 1,
			}}
			svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("concurrent-turn")})
			t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
			ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
			if err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			errs := make(chan error, len(tc.messages))
			var wg sync.WaitGroup
			for _, message := range tc.messages {
				message := message
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					_, sendErr := ctrl.Send(context.Background(), message)
					errs <- sendErr
				}()
			}
			close(start)
			wg.Wait()
			close(errs)
			errorCount := 0
			for sendErr := range errs {
				if sendErr != nil {
					errorCount++
					if !errors.Is(sendErr, domain.ErrGovernedCommandIdempotencyConflict) {
						t.Errorf("unexpected error: %v", sendErr)
					}
				}
			}
			if errorCount != tc.wantErrors || len(conv.sentMessages()) != 1 {
				t.Fatalf("errors=%d want=%d provider dispatches=%d", errorCount, tc.wantErrors, len(conv.sentMessages()))
			}
			turns, err := st.LoadConversationSnapshot(context.Background(), ctrl.ConversationID())
			if err != nil || len(turns.Turns) != 1 {
				t.Fatalf("turns=%+v err=%v", turns.Turns, err)
			}
			claim, ok, err := st.GetGovernedCommand(context.Background(), turns.Turns[0].ID)
			if err != nil || !ok || claim.State != domain.GovernedCommandAcknowledged || claim.Correlation.ProviderTurnID != "provider-winner" {
				t.Fatalf("claim=%+v ok=%v err=%v", claim, ok, err)
			}
		})
	}
}

func TestGovernedTurnPersistsDispatchingBeforeProviderContactAndAcknowledges(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	base := newFakeConversation()
	conv := &dispatchConversation{fakeConversation: base, dispatch: ports.ChatTurnDispatch{
		Acceptance: ports.ChatTurnAcknowledged, Ref: ports.ChatTurnRef{ProviderTurnID: "provider-turn-1"},
		TransportRequestID: 1, TransportSHA256: "abc", TransportBytes: 10, TransportSequence: 1,
	}}
	conv.onDispatch = func() {
		claims, err := st.ListUnsettledGovernedCommands(context.Background())
		if err != nil || len(claims) != 1 || claims[0].IdempotencyKey != "request-1" || claims[0].State != domain.GovernedCommandDispatching {
			t.Fatalf("provider contacted before durable dispatching: claims=%+v err=%v", claims, err)
		}
	}
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("governed")})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := ctrl.Send(context.Background(), ports.ChatUserMessage{Text: "do it", ClientMessageID: "request-1"})
	if err != nil {
		t.Fatal(err)
	}
	claim, ok, err := st.GetGovernedCommand(context.Background(), turn.ID)
	if err != nil || !ok || claim.State != domain.GovernedCommandAcknowledged || claim.Correlation.ProviderTurnID != "provider-turn-1" || turn.ProviderTurnID != "provider-turn-1" {
		t.Fatalf("acknowledged claim/turn: claim=%+v turn=%+v ok=%v err=%v", claim, turn, ok, err)
	}
}

type failFirstGovernedTurnClaimStore struct {
	chatsvc.Store
	mu     sync.Mutex
	failed bool
	err    error
}

func (s *failFirstGovernedTurnClaimStore) CreateGovernedCommandClaim(ctx context.Context, rec domain.GovernedCommandRecord) (domain.GovernedCommandRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.failed {
		s.failed = true
		return domain.GovernedCommandRecord{}, false, s.err
	}
	return s.Store.CreateGovernedCommandClaim(ctx, rec)
}

func TestGovernedTurnCrashBeforeClaimLeavesNoEffectAndFreshProcessCanRetry(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	injected := errors.New("daemon stopped before durable claim")
	wrapped := &failFirstGovernedTurnClaimStore{Store: st, err: injected}
	firstConv := &dispatchConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatTurnDispatch{
		Acceptance: ports.ChatTurnAcknowledged, Ref: ports.ChatTurnRef{ProviderTurnID: "must-not-dispatch"},
	}}
	first := chatsvc.New(chatsvc.Options{Store: wrapped, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: firstConv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("pre-claim-first")})
	firstCtrl, err := first.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	msg := ports.ChatUserMessage{Text: "do it", ClientMessageID: "request-pre-claim"}
	if _, err = firstCtrl.Send(context.Background(), msg); !errors.Is(err, injected) {
		t.Fatalf("first send err=%v", err)
	}
	claims, err := st.ListUnsettledGovernedCommands(context.Background())
	if err != nil || len(claims) != 0 || len(firstConv.sentMessages()) != 0 {
		t.Fatalf("claims=%+v provider dispatches=%d err=%v", claims, len(firstConv.sentMessages()), err)
	}

	// Model a killed daemon with a new service over the same durable store. Since
	// the crash happened before the claim linearization point, this exact retry is
	// allowed to create the one claim and the one visible provider effect.
	secondConv := &dispatchConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatTurnDispatch{
		Acceptance: ports.ChatTurnAcknowledged, Ref: ports.ChatTurnRef{ProviderTurnID: "provider-retry"},
		TransportRequestID: 1, TransportSHA256: "retry-sha", TransportBytes: 10, TransportSequence: 1,
	}}
	second := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: secondConv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("pre-claim-second")})
	t.Cleanup(func() { _ = second.Stop(context.Background(), testSession) })
	secondCtrl, err := second.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ProviderConversationID: firstCtrl.ProviderConversationID(), ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := secondCtrl.Send(context.Background(), msg)
	if err != nil || turn.ProviderTurnID != "provider-retry" {
		t.Fatalf("retry turn=%+v err=%v", turn, err)
	}
	claims, err = st.ListUnsettledGovernedCommands(context.Background())
	if err != nil || len(claims) != 0 || len(secondConv.sentMessages()) != 1 {
		t.Fatalf("unsettled=%+v retry provider dispatches=%d err=%v", claims, len(secondConv.sentMessages()), err)
	}
	if claim, ok, err := st.GetGovernedCommand(context.Background(), turn.ID); err != nil || !ok || claim.State != domain.GovernedCommandAcknowledged || claim.Correlation.ProviderTurnID != "provider-retry" {
		t.Fatalf("claim=%+v ok=%v err=%v", claim, ok, err)
	}
}

type failFirstGovernedTurnMessageStore struct {
	chatsvc.Store
	mu     sync.Mutex
	failed bool
	err    error
}

func (s *failFirstGovernedTurnMessageStore) AppendUserMessage(ctx context.Context, conversationID string, session domain.SessionID, generation string, msg domain.ConversationMessage, turnID string, now time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.failed {
		s.failed = true
		return false, s.err
	}
	return s.Store.AppendUserMessage(ctx, conversationID, session, generation, msg, turnID, now)
}

func TestGovernedTurnCrashAfterClaimBeforeProviderContactAdoptsOnceOnRestart(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	injected := errors.New("daemon stopped after claim before message projection")
	wrapped := &failFirstGovernedTurnMessageStore{Store: st, err: injected}
	firstConv := &dispatchConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatTurnDispatch{
		Acceptance: ports.ChatTurnAcknowledged, Ref: ports.ChatTurnRef{ProviderTurnID: "must-not-dispatch"},
	}}
	first := chatsvc.New(chatsvc.Options{Store: wrapped, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: firstConv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("post-claim-first")})
	firstCtrl, err := first.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	msg := ports.ChatUserMessage{Text: "do it", ClientMessageID: "request-post-claim"}
	if _, err = firstCtrl.Send(context.Background(), msg); !errors.Is(err, injected) {
		t.Fatalf("first send err=%v", err)
	}
	claims, err := st.ListUnsettledGovernedCommands(context.Background())
	if err != nil || len(claims) != 1 || claims[0].State != domain.GovernedCommandClaimed || len(firstConv.sentMessages()) != 0 {
		t.Fatalf("claims=%+v provider dispatches=%d err=%v", claims, len(firstConv.sentMessages()), err)
	}
	oldGeneration := claims[0].ControllerGeneration

	secondConv := &dispatchConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatTurnDispatch{
		Acceptance: ports.ChatTurnAcknowledged, Ref: ports.ChatTurnRef{ProviderTurnID: "provider-adopted"},
		TransportRequestID: 1, TransportSHA256: "adopted-sha", TransportBytes: 10, TransportSequence: 1,
	}}
	second := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: secondConv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("post-claim-second")})
	t.Cleanup(func() { _ = second.Stop(context.Background(), testSession) })
	secondCtrl, err := second.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ProviderConversationID: firstCtrl.ProviderConversationID(), ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := secondCtrl.Send(context.Background(), msg)
	if err != nil || turn.ProviderTurnID != "provider-adopted" || len(secondConv.sentMessages()) != 1 {
		t.Fatalf("retry turn=%+v provider dispatches=%d err=%v", turn, len(secondConv.sentMessages()), err)
	}
	claim, ok, err := st.GetGovernedCommand(context.Background(), turn.ID)
	if err != nil || !ok || claim.State != domain.GovernedCommandAcknowledged || claim.ControllerGeneration != secondCtrl.Generation() || claim.ControllerGeneration == oldGeneration {
		t.Fatalf("adopted claim=%+v ok=%v oldGeneration=%q err=%v", claim, ok, oldGeneration, err)
	}

	// The stale controller still has the old claim in memory. Active-generation
	// fencing must stop it before provider contact even after the fresh controller
	// has settled the one visible effect.
	if _, err = firstCtrl.Send(context.Background(), msg); err != nil {
		t.Fatalf("stale exact retry err=%v", err)
	}
	if len(firstConv.sentMessages()) != 0 || len(secondConv.sentMessages()) != 1 {
		t.Fatalf("provider dispatches stale=%d active=%d", len(firstConv.sentMessages()), len(secondConv.sentMessages()))
	}
}

type blockingGovernedTurnDispatch struct {
	*fakeConversation
	started chan struct{}
	release chan struct{}
	once    sync.Once
	result  ports.ChatTurnDispatch
}

func (c *blockingGovernedTurnDispatch) DispatchTurn(_ context.Context, msg ports.ChatUserMessage) (ports.ChatTurnDispatch, error) {
	c.mu.Lock()
	c.sent = append(c.sent, msg)
	c.mu.Unlock()
	c.once.Do(func() { close(c.started) })
	<-c.release
	return c.result, nil
}

type historyDispatchConversation struct {
	*dispatchConversation
	history []ports.ChatEvent
}

func (c *historyDispatchConversation) ReadHistory(context.Context) ([]ports.ChatEvent, error) {
	return c.history, nil
}

func TestGovernedTurnCrashMidDispatchRestartsUnknownWithoutRedelivery(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	firstConv := &blockingGovernedTurnDispatch{
		fakeConversation: newFakeConversation(), started: make(chan struct{}), release: make(chan struct{}),
		result: ports.ChatTurnDispatch{Acceptance: ports.ChatTurnAcknowledged, Ref: ports.ChatTurnRef{ProviderTurnID: "late-provider-turn"}, TransportRequestID: 1, TransportSHA256: "late-sha", TransportBytes: 10, TransportSequence: 1},
	}
	first := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: firstConv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("mid-dispatch-first")})
	firstCtrl, err := first.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	msg := ports.ChatUserMessage{Text: "do it", ClientMessageID: "request-mid-dispatch"}
	firstDone := make(chan error, 1)
	go func() {
		_, sendErr := firstCtrl.Send(context.Background(), msg)
		firstDone <- sendErr
	}()
	select {
	case <-firstConv.started:
	case <-time.After(time.Second):
		t.Fatal("provider dispatch did not start")
	}
	claims, err := st.ListUnsettledGovernedCommands(context.Background())
	if err != nil || len(claims) != 1 || claims[0].State != domain.GovernedCommandDispatching || len(firstConv.sentMessages()) != 1 {
		t.Fatalf("mid-dispatch claims=%+v provider contacts=%d err=%v", claims, len(firstConv.sentMessages()), err)
	}

	// A new daemon generation resumes the native thread while the old process is
	// gone from Kennel's point of view. Empty stable history cannot prove whether
	// the in-flight provider call landed, so restart must retain delivery_unknown.
	secondBase := &dispatchConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatTurnDispatch{
		Acceptance: ports.ChatTurnAcknowledged, Ref: ports.ChatTurnRef{ProviderTurnID: "must-not-redispatch"},
	}}
	secondConv := &historyDispatchConversation{dispatchConversation: secondBase}
	second := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: secondConv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("mid-dispatch-second")})
	t.Cleanup(func() { _ = second.Stop(context.Background(), testSession) })
	secondCtrl, err := second.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ProviderConversationID: firstCtrl.ProviderConversationID(), ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	claims, err = st.ListUnsettledGovernedCommands(context.Background())
	if err != nil || len(claims) != 1 || claims[0].State != domain.GovernedCommandDeliveryUnknown {
		t.Fatalf("restart claims=%+v err=%v", claims, err)
	}
	if _, err = secondCtrl.Send(context.Background(), msg); err != nil {
		t.Fatalf("exact retry err=%v", err)
	}
	if _, err = secondCtrl.Send(context.Background(), ports.ChatUserMessage{Text: "later", ClientMessageID: "request-after-mid-dispatch"}); err != nil {
		t.Fatalf("conflicting request err=%v", err)
	}
	if len(secondConv.sentMessages()) != 0 {
		t.Fatalf("replacement provider dispatches=%d, want 0", len(secondConv.sentMessages()))
	}

	// If the dead caller returns after replacement, its stale generation cannot
	// convert the honest ambiguity into guessed success.
	close(firstConv.release)
	select {
	case sendErr := <-firstDone:
		if !errors.Is(sendErr, chatsvc.ErrGovernedDeliveryUnknown) {
			t.Fatalf("stale dispatch return err=%v", sendErr)
		}
	case <-time.After(time.Second):
		t.Fatal("stale dispatch did not return")
	}
	claims, err = st.ListUnsettledGovernedCommands(context.Background())
	var unknown, queued int
	for _, claim := range claims {
		switch claim.IdempotencyKey {
		case msg.ClientMessageID:
			if claim.State == domain.GovernedCommandDeliveryUnknown {
				unknown++
			}
		case "request-after-mid-dispatch":
			if claim.State == domain.GovernedCommandClaimed {
				queued++
			}
		}
	}
	if err != nil || unknown != 1 || queued != 1 || len(firstConv.sentMessages()) != 1 || len(secondConv.sentMessages()) != 0 {
		t.Fatalf("final claims=%+v first contacts=%d replacement contacts=%d err=%v", claims, len(firstConv.sentMessages()), len(secondConv.sentMessages()), err)
	}
}

type failAcknowledgedGovernedTurnStore struct {
	chatsvc.Store
	err error
}

func (s *failAcknowledgedGovernedTurnStore) AdvanceGovernedCommand(ctx context.Context, rec domain.GovernedCommandRecord, expected domain.GovernedCommandState, generation, revision, capability string) (bool, error) {
	if rec.State == domain.GovernedCommandAcknowledged {
		return false, s.err
	}
	return s.Store.AdvanceGovernedCommand(ctx, rec, expected, generation, revision, capability)
}

func TestGovernedTurnCrashAfterProviderAcceptanceLeavesDispatchingAndBlocksReplay(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	conv := &dispatchConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatTurnDispatch{
		Acceptance: ports.ChatTurnAcknowledged, Ref: ports.ChatTurnRef{ProviderTurnID: "provider-accepted"},
		TransportRequestID: 1, TransportSHA256: "abc", TransportBytes: 10, TransportSequence: 1,
	}}
	injected := errors.New("daemon stopped before acknowledged projection")
	wrapped := &failAcknowledgedGovernedTurnStore{Store: st, err: injected}
	svc := chatsvc.New(chatsvc.Options{Store: wrapped, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("post-accept")})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	msg := ports.ChatUserMessage{Text: "do it", ClientMessageID: "request-post-accept"}
	if _, err = ctrl.Send(context.Background(), msg); !errors.Is(err, chatsvc.ErrGovernedDeliveryUnknown) || !errors.Is(err, injected) {
		t.Fatalf("first send err=%v", err)
	}
	claims, err := st.ListUnsettledGovernedCommands(context.Background())
	if err != nil || len(claims) != 1 || claims[0].State != domain.GovernedCommandDispatching || claims[0].Correlation.ClientMessageID != msg.ClientMessageID {
		t.Fatalf("claims=%+v err=%v", claims, err)
	}
	if _, err = ctrl.Send(context.Background(), msg); err != nil {
		t.Fatalf("exact replay err=%v", err)
	}
	if _, err = ctrl.Send(context.Background(), ports.ChatUserMessage{Text: "later", ClientMessageID: "request-later"}); err != nil {
		t.Fatalf("later send err=%v", err)
	}
	if got := len(conv.sentMessages()); got != 1 {
		t.Fatalf("provider dispatches=%d want 1", got)
	}
}

func TestGovernedTurnUnknownStaysBlockingAndExactRetryDoesNotRedispatch(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	conv := &dispatchConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatTurnDispatch{
		Acceptance: ports.ChatTurnDeliveryUnknown, TransportRequestID: 1, TransportSHA256: "abc", TransportBytes: 10, TransportSequence: 1,
	}, err: context.DeadlineExceeded}
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("unknown")})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	msg := ports.ChatUserMessage{Text: "do it", ClientMessageID: "request-unknown"}
	if _, err := ctrl.Send(context.Background(), msg); !errors.Is(err, chatsvc.ErrGovernedDeliveryUnknown) {
		t.Fatalf("first err=%v", err)
	}
	if _, err := ctrl.Send(context.Background(), msg); err != nil {
		t.Fatalf("exact retry=%v", err)
	}
	if _, err := ctrl.Send(context.Background(), ports.ChatUserMessage{Text: "different work", ClientMessageID: "request-after-unknown"}); err != nil {
		t.Fatalf("queue behind unknown: %v", err)
	}
	if got := len(conv.sentMessages()); got != 1 {
		t.Fatalf("provider contacts=%d, want 1", got)
	}
	snapshot, err := st.LoadConversationSnapshot(context.Background(), ctrl.ConversationID())
	if err != nil || len(snapshot.Turns) != 2 || snapshot.Turns[0].State.Terminal() || snapshot.Turns[1].State.Terminal() {
		t.Fatalf("timeline=%+v err=%v", snapshot.Turns, err)
	}
	claim, ok, err := st.GetGovernedCommand(context.Background(), snapshot.Turns[0].ID)
	if err != nil || !ok || claim.State != domain.GovernedCommandDeliveryUnknown {
		t.Fatalf("claim=%+v ok=%v err=%v", claim, ok, err)
	}
	if err := st.SettleOrphanedTurns(context.Background(), testSession, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	snapshot, err = st.LoadConversationSnapshot(context.Background(), ctrl.ConversationID())
	if err != nil || snapshot.Turns[0].State.Terminal() {
		t.Fatalf("restart falsely settled unknown: timeline=%+v err=%v", snapshot.Turns, err)
	}
}

// TestSendDeliveryUnknownStaysDurablyQueued backs the 202-on-delivery-unknown
// response the HTTP layer sends: it proves the turn that response echoes is
// not a value invented for the client, but the same durable row a fresh read
// of the store sees -- non-empty id, queued, and the user's message text
// already committed. If a future refactor ever made Send return a zero-value
// turn alongside ErrGovernedDeliveryUnknown, this fails loudly rather than
// silently starting to 500 every delivery-unknown send.
func TestSendDeliveryUnknownStaysDurablyQueued(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	conv := &dispatchConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatTurnDispatch{
		Acceptance: ports.ChatTurnDeliveryUnknown, TransportRequestID: 1, TransportSHA256: "abc", TransportBytes: 10, TransportSequence: 1,
	}, err: context.DeadlineExceeded}
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("durable-unknown")})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}

	turn, err := svc.Send(context.Background(), testSession, ports.ChatUserMessage{Text: "must not vanish", ClientMessageID: "durable-request"})
	if !errors.Is(err, chatsvc.ErrGovernedDeliveryUnknown) {
		t.Fatalf("err=%v", err)
	}
	// This is the exact turn the send() handler's 202 branch echoes to the
	// client -- it must be real, not a zero value returned alongside the error.
	if turn.ID == "" || turn.State != domain.TurnStateQueued {
		t.Fatalf("send() handler would echo a fabricated turn: turn=%+v", turn)
	}

	stored, err := st.LoadConversationSnapshot(context.Background(), ctrl.ConversationID())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, msg := range stored.Messages {
		if msg.Text == "must not vanish" && msg.ClientMessageID == "durable-request" {
			found = true
		}
	}
	if !found {
		t.Fatalf("message not durably recorded despite 202: messages=%+v", stored.Messages)
	}
	if len(stored.Turns) != 1 || stored.Turns[0].ID != turn.ID || stored.Turns[0].State.Terminal() {
		t.Fatalf("turn not durably queued despite 202: turns=%+v", stored.Turns)
	}
}

func snapshotReader(st *sqlite.Store) chatsvc.SnapshotReader {
	return chatsvc.SnapshotReaderFunc(func(ctx context.Context, conversationID string) (chatsvc.ConversationRows, error) {
		rows, err := st.LoadConversationSnapshot(ctx, conversationID)
		if err != nil {
			return chatsvc.ConversationRows{}, err
		}
		return chatsvc.ConversationRows{Conversation: rows.Conversation, Turns: rows.Turns, Messages: rows.Messages, Activities: rows.Activities}, nil
	})
}

func sequentialID(prefix string) func() string {
	var mu sync.Mutex
	n := 0
	return func() string { mu.Lock(); defer mu.Unlock(); n++; return fmt.Sprintf("%s-%d", prefix, n) }
}

func TestGovernedTurnExplicitRejectionIsNotDeliveryUnknown(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	conv := &dispatchConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatTurnDispatch{
		Acceptance: ports.ChatTurnRejected, TransportRequestID: 2, TransportSHA256: "def", TransportBytes: 11, TransportSequence: 2,
	}, err: errors.New("provider refused request")}
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("rejected")})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.Send(context.Background(), ports.ChatUserMessage{Text: "do it", ClientMessageID: "request-rejected"}); !errors.Is(err, chatsvc.ErrGovernedRejected) || errors.Is(err, chatsvc.ErrGovernedDeliveryUnknown) {
		t.Fatalf("rejection classification=%v", err)
	}
	snapshot, err := st.LoadConversationSnapshot(context.Background(), ctrl.ConversationID())
	if err != nil || len(snapshot.Turns) != 1 || snapshot.Turns[0].State != domain.TurnStateFailed {
		t.Fatalf("timeline=%+v err=%v", snapshot.Turns, err)
	}
	claim, ok, err := st.GetGovernedCommand(context.Background(), snapshot.Turns[0].ID)
	if err != nil || !ok || claim.State != domain.GovernedCommandRejected {
		t.Fatalf("claim=%+v ok=%v err=%v", claim, ok, err)
	}
}

type dispatchHistoryConversation struct {
	*dispatchConversation
	history []ports.ChatEvent
}

func (c *dispatchHistoryConversation) ReadHistory(context.Context) ([]ports.ChatEvent, error) {
	return append([]ports.ChatEvent(nil), c.history...), nil
}

func TestGovernedUnknownReconcilesFromStableNativeHistoryWithoutRedispatch(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	first := &dispatchConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatTurnDispatch{
		Acceptance: ports.ChatTurnDeliveryUnknown, TransportRequestID: 1, TransportSHA256: "abc", TransportBytes: 10, TransportSequence: 1,
	}, err: context.DeadlineExceeded}
	ids := sequentialID("reconcile")
	firstSvc := chatsvc.New(chatsvc.Options{Store: st, Reader: snapshotReader(st), Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: first}}, Log: slog.New(slog.DiscardHandler), NewID: ids})
	firstCtrl, err := firstSvc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := firstCtrl.Send(context.Background(), ports.ChatUserMessage{Text: "do it", ClientMessageID: "request-reconcile"}); !errors.Is(err, chatsvc.ErrGovernedDeliveryUnknown) {
		t.Fatalf("unknown send=%v", err)
	}
	if err := firstSvc.Stop(context.Background(), testSession); err != nil {
		t.Fatal(err)
	}

	secondBase := newFakeConversation()
	secondBase.providerConversationID = "thread-1"
	second := &dispatchHistoryConversation{dispatchConversation: &dispatchConversation{fakeConversation: secondBase}, history: []ports.ChatEvent{
		{Kind: ports.ChatEventTurnStarted, ProviderEventID: "history-start", ProviderTurnID: "provider-turn-recovered", ProviderConversationID: "thread-1"},
		{Kind: ports.ChatEventUserMessageCompleted, ProviderEventID: "history-user", ProviderTurnID: "provider-turn-recovered", ProviderConversationID: "thread-1", ProviderItemID: "provider-user", ClientMessageID: "request-reconcile", Text: "do it"},
		{Kind: ports.ChatEventTurnCompleted, ProviderEventID: "history-complete", ProviderTurnID: "provider-turn-recovered", ProviderConversationID: "thread-1", TurnState: domain.TurnStateCompleted},
	}}
	secondSvc := chatsvc.New(chatsvc.Options{Store: st, Reader: snapshotReader(st), Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: second}}, Log: slog.New(slog.DiscardHandler), NewID: ids})
	t.Cleanup(func() { _ = secondSvc.Stop(context.Background(), testSession) })
	if _, err := secondSvc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ProviderConversationID: "thread-1", ExecutionPolicy: &policy}); err != nil {
		t.Fatal(err)
	}
	claims, err := st.ListUnsettledGovernedCommands(context.Background())
	if err != nil || len(claims) != 0 {
		t.Fatalf("unsettled=%+v err=%v", claims, err)
	}
	if got := len(second.sentMessages()); got != 0 {
		t.Fatalf("restart redispatched %d turns", got)
	}
	snapshot, err := st.LoadConversationSnapshot(context.Background(), firstCtrl.ConversationID())
	if err != nil || len(snapshot.Turns) != 1 || snapshot.Turns[0].State != domain.TurnStateCompleted {
		t.Fatalf("reconciled timeline=%+v err=%v", snapshot.Turns, err)
	}
}

func TestGovernedClaimBlocksDifferentWork(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	conv := &dispatchConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatTurnDispatch{
		Acceptance: ports.ChatTurnAcknowledged, Ref: ports.ChatTurnRef{ProviderTurnID: "provider-turn-exact"},
		TransportRequestID: 1, TransportSHA256: "abc", TransportBytes: 10, TransportSequence: 1,
	}}
	ids := sequentialID("claimed")
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: ids})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	digest := chatCapsDigestForTest(productionCaps())
	at := time.Now().UTC()
	claim := domain.GovernedCommandRecord{GovernedCommandContract: domain.GovernedCommandContract{
		ID: "claimed-existing", IdempotencyKey: "request-existing", RequestFingerprint: domain.ComputeGovernedTurnRequestFingerprint(testSession, "request-existing", "first", ""),
		Class: domain.GovernedCommandTurn, State: domain.GovernedCommandClaimed, SessionID: testSession,
		ControllerGeneration: ctrl.Generation(), ExpectedRevision: policy.PlanRevisionID.String(), CapabilityFingerprint: digest,
		Correlation: domain.GovernedCommandCorrelation{ProviderConversationID: "thread-1", ClientMessageID: "request-existing"}, ReplayStrategy: domain.GovernedCommandReplayUnavailable, Quiescence: domain.GovernedCommandQuiescenceNotApplicable,
	}, CreatedAt: at, UpdatedAt: at}
	if _, made, err := st.CreateGovernedCommandClaim(context.Background(), claim); err != nil || !made {
		t.Fatalf("seed claim made=%v err=%v", made, err)
	}
	queued, err := ctrl.Send(context.Background(), ports.ChatUserMessage{Text: "second", ClientMessageID: "request-second"})
	if err != nil || queued.State != domain.TurnStateQueued || len(conv.sentMessages()) != 0 {
		t.Fatalf("different request turn=%+v sends=%d err=%v", queued, len(conv.sentMessages()), err)
	}
	claimAfter, ok, err := st.GetGovernedCommand(context.Background(), "claimed-existing")
	if err != nil || !ok || claimAfter.State != domain.GovernedCommandClaimed || len(conv.sentMessages()) != 0 {
		t.Fatalf("claim=%+v ok=%v sends=%d err=%v", claimAfter, ok, len(conv.sentMessages()), err)
	}
}

func chatCapsDigestForTest(capabilities ports.ChatCapabilities) string {
	enabled := make([]string, 0, len(capabilities))
	for capability, available := range capabilities {
		if available {
			enabled = append(enabled, string(capability))
		}
	}
	sort.Strings(enabled)
	payload, _ := json.Marshal(enabled)
	sum := sha256.Sum256(payload)
	return "chat-v1:" + hex.EncodeToString(sum[:])
}

func TestGovernedAnswerBindsExactPendingGenerationAndReplaysWithoutRedispatch(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	conv := &answerDispatchConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatAnswerDispatch{WriteOutcome: ports.ChatAnswerFrameWriteComplete}}
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("answer")})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	conv.emit(ports.ChatEvent{Kind: ports.ChatEventApprovalRequested, RequestID: "7", ProviderItemID: "7", Summary: "allow?", Decisions: []ports.ChatDecisionOption{{ID: "accept"}}})
	snapshot := awaitStoreSnapshot(t, st, ctrl.ConversationID(), func(s store.ConversationSnapshot) bool { return len(s.Activities) == 1 })
	generation := snapshot.Activities[0].ID
	conv.onDispatch = func() {
		claims, listErr := st.ListUnsettledGovernedControlCommands(context.Background())
		if listErr != nil || len(claims) != 1 || claims[0].State != domain.GovernedCommandDispatching || claims[0].RequestInstanceID != generation {
			t.Fatalf("answer was dispatched without exact durable generation: claims=%+v err=%v", claims, listErr)
		}
	}
	decision := ports.ChatDecision{ID: "accept"}
	if err := svc.Resolve(context.Background(), testSession, "7", decision); err != nil {
		t.Fatal(err)
	}
	if conv.generation != generation {
		t.Fatalf("generation=%q want %q", conv.generation, generation)
	}
	// Exact replay recovers the acknowledged governed receipt without contacting
	// the provider again.
	if err := svc.Resolve(context.Background(), testSession, "7", decision); err != nil {
		t.Fatalf("replay err=%v", err)
	}
	if conv.calls != 1 {
		t.Fatalf("answer dispatches=%d want 1", conv.calls)
	}
}

func TestGovernedAnswerNotStartedIsRejectedWithoutResolvingApproval(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	conv := &answerDispatchConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatAnswerDispatch{WriteOutcome: ports.ChatAnswerWriteNotStarted}, err: ports.ErrChatDecisionNotOffered}
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("answer-not-started")})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	conv.emit(ports.ChatEvent{Kind: ports.ChatEventApprovalRequested, RequestID: "8", ProviderItemID: "8", Summary: "allow?", Decisions: []ports.ChatDecisionOption{{ID: "accept"}}})
	awaitStoreSnapshot(t, st, ctrl.ConversationID(), func(s store.ConversationSnapshot) bool { return len(s.Activities) == 1 })
	if err := svc.Resolve(context.Background(), testSession, "8", ports.ChatDecision{ID: "accept"}); !errors.Is(err, chatsvc.ErrProviderRefused) {
		t.Fatalf("resolve err=%v", err)
	}
	snapshot, err := st.LoadConversationSnapshot(context.Background(), ctrl.ConversationID())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Activities[0].Status != domain.ActivityStatusPending {
		t.Fatalf("activity status=%q want pending", snapshot.Activities[0].Status)
	}
}

func TestGovernedAnswerCompleteWriteLocalFailureBecomesUnknown(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	conv := &answerDispatchConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatAnswerDispatch{WriteOutcome: ports.ChatAnswerFrameWriteComplete}}
	injected := errors.New("resolve activity failed")
	wrapped := &failResolveApprovalStore{Store: st, err: injected}
	svc := chatsvc.New(chatsvc.Options{Store: wrapped, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("answer-fail")})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	conv.emit(ports.ChatEvent{Kind: ports.ChatEventApprovalRequested, RequestID: "9", ProviderItemID: "9", Summary: "allow?", Decisions: []ports.ChatDecisionOption{{ID: "accept"}}})
	awaitStoreSnapshot(t, st, ctrl.ConversationID(), func(s store.ConversationSnapshot) bool { return len(s.Activities) == 1 })
	if err := svc.Resolve(context.Background(), testSession, "9", ports.ChatDecision{ID: "accept"}); !errors.Is(err, chatsvc.ErrSteerDeliveryUnknown) || !errors.Is(err, injected) {
		t.Fatalf("resolve err=%v", err)
	}
	if err := svc.Resolve(context.Background(), testSession, "9", ports.ChatDecision{ID: "accept"}); !errors.Is(err, chatsvc.ErrSteerDeliveryUnknown) {
		t.Fatalf("replay err=%v", err)
	}
	if conv.calls != 1 {
		t.Fatalf("answer dispatches=%d want 1", conv.calls)
	}
}

type failResolveApprovalStore struct {
	chatsvc.Store
	err error
}

func (s *failResolveApprovalStore) ResolveApproval(context.Context, string, string, string, time.Time) error {
	return s.err
}

type governedInterruptRecorder struct {
	*interruptRecorder
	dispatch   ports.ChatInterruptDispatch
	err        error
	onDispatch func()
}

func (r *governedInterruptRecorder) DispatchTurn(ctx context.Context, msg ports.ChatUserMessage) (ports.ChatTurnDispatch, error) {
	ref, err := r.fakeConversation.SendTurn(ctx, msg)
	return ports.ChatTurnDispatch{Acceptance: ports.ChatTurnAcknowledged, Ref: ref, TransportRequestID: 1, TransportSHA256: "turn-sha", TransportBytes: 10, TransportSequence: 1}, err
}

func (r *governedInterruptRecorder) DispatchInterrupt(_ context.Context, turn string) (ports.ChatInterruptDispatch, error) {
	r.activeMu.Lock()
	r.attempts = append(r.attempts, turn)
	r.activeMu.Unlock()
	if r.onDispatch != nil {
		r.onDispatch()
	}
	return r.dispatch, r.err
}

func TestGovernedInterruptPersistsDispatchAndQuiescenceBeforeSuccess(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	base := newInterruptRecorder()
	conv := &governedInterruptRecorder{interruptRecorder: base, dispatch: ports.ChatInterruptDispatch{
		Acceptance: ports.ChatTurnAcknowledged, TransportRequestID: 3, TransportSHA256: "interrupt-sha", TransportBytes: 18, TransportSequence: 3,
		Quiescence: domain.GovernedCommandQuiescenceCodexTree, QuiescenceEvidenceRef: "tree-stopped",
	}}
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("interrupt")})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.Send(context.Background(), ports.ChatUserMessage{Text: "work", ClientMessageID: "turn"}); err != nil {
		t.Fatal(err)
	}
	conv.markActive("provider-turn-1")
	conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	conv.onDispatch = func() {
		claims, e := st.ListUnsettledGovernedControlCommands(context.Background())
		if e != nil || len(claims) != 1 || claims[0].State != domain.GovernedCommandDispatching {
			t.Fatalf("claims=%+v err=%v", claims, e)
		}
	}
	if err := svc.Interrupt(context.Background(), testSession); err != nil {
		t.Fatal(err)
	}
	claims, err := st.ListUnsettledGovernedControlCommands(context.Background())
	if err != nil || len(claims) != 0 {
		t.Fatalf("unsettled=%+v err=%v", claims, err)
	}
	if conv.attemptCount() != 1 {
		t.Fatalf("attempts=%d", conv.attemptCount())
	}
	// Exact replay while the same turn remains visible recovers the receipt.
	if err := ctrl.Interrupt(context.Background()); err != nil {
		t.Fatal(err)
	}
	if conv.attemptCount() != 1 {
		t.Fatalf("replay attempts=%d", conv.attemptCount())
	}
}

func TestGovernedInterruptUnknownBlocksExactReplay(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	conv := &governedInterruptRecorder{interruptRecorder: newInterruptRecorder(), dispatch: ports.ChatInterruptDispatch{
		Acceptance: ports.ChatTurnDeliveryUnknown, TransportRequestID: 3, TransportSHA256: "interrupt-sha", TransportBytes: 18, TransportSequence: 3,
		Quiescence: domain.GovernedCommandQuiescencePending,
	}, err: context.DeadlineExceeded}
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("interrupt-unknown")})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.Send(context.Background(), ports.ChatUserMessage{Text: "work", ClientMessageID: "turn"}); err != nil {
		t.Fatal(err)
	}
	conv.markActive("provider-turn-1")
	conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	if err := svc.Interrupt(context.Background(), testSession); !errors.Is(err, chatsvc.ErrSteerDeliveryUnknown) {
		t.Fatalf("first=%v", err)
	}
	if err := ctrl.Interrupt(context.Background()); !errors.Is(err, chatsvc.ErrSteerDeliveryUnknown) {
		t.Fatalf("replay=%v", err)
	}
	if conv.attemptCount() != 1 {
		t.Fatalf("attempts=%d", conv.attemptCount())
	}
	claims, err := st.ListUnsettledGovernedControlCommands(context.Background())
	if err != nil || len(claims) != 1 || claims[0].State != domain.GovernedCommandDeliveryUnknown || claims[0].Quiescence != domain.GovernedCommandQuiescencePending {
		t.Fatalf("claims=%+v err=%v", claims, err)
	}
}

func TestGovernedInterruptAcknowledgedWithoutQuiescenceStaysUnknown(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	conv := &governedInterruptRecorder{interruptRecorder: newInterruptRecorder(), dispatch: ports.ChatInterruptDispatch{
		Acceptance: ports.ChatTurnAcknowledged, TransportRequestID: 3, TransportSHA256: "interrupt-sha", TransportBytes: 18, TransportSequence: 3,
		Quiescence: domain.GovernedCommandQuiescencePending,
	}}
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("interrupt-no-quiesce")})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.Send(context.Background(), ports.ChatUserMessage{Text: "work", ClientMessageID: "turn"}); err != nil {
		t.Fatal(err)
	}
	conv.markActive("provider-turn-1")
	conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	if err := svc.Interrupt(context.Background(), testSession); !errors.Is(err, chatsvc.ErrSteerDeliveryUnknown) {
		t.Fatalf("err=%v", err)
	}
	claims, err := st.ListUnsettledGovernedControlCommands(context.Background())
	if err != nil || len(claims) != 1 || claims[0].State != domain.GovernedCommandDeliveryUnknown {
		t.Fatalf("claims=%+v err=%v", claims, err)
	}
}

func TestGovernedInterruptCrashAfterAcceptanceBeforeQuiescenceStaysBlockedOnRestart(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	firstConv := &governedInterruptRecorder{interruptRecorder: newInterruptRecorder(), dispatch: ports.ChatInterruptDispatch{
		Acceptance: ports.ChatTurnAcknowledged, TransportRequestID: 3, TransportSHA256: "interrupt-accepted", TransportBytes: 18, TransportSequence: 3,
		Quiescence: domain.GovernedCommandQuiescencePending,
	}}
	first := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: firstConv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("interrupt-crash-first")})
	firstCtrl, err := first.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := firstCtrl.Send(context.Background(), ports.ChatUserMessage{Text: "work", ClientMessageID: "interrupt-crash-turn"}); err != nil {
		t.Fatal(err)
	}
	firstConv.markActive("provider-turn-1")
	firstConv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	if err := first.Interrupt(context.Background(), testSession); !errors.Is(err, chatsvc.ErrSteerDeliveryUnknown) {
		t.Fatalf("interrupt err=%v", err)
	}
	claims, err := st.ListUnsettledGovernedControlCommands(context.Background())
	if err != nil || len(claims) != 1 || claims[0].Class != domain.GovernedControlInterrupt || claims[0].State != domain.GovernedCommandDeliveryUnknown || claims[0].Quiescence != domain.GovernedCommandQuiescencePending || firstConv.attemptCount() != 1 {
		t.Fatalf("post-acceptance claims=%+v interrupt attempts=%d err=%v", claims, firstConv.attemptCount(), err)
	}

	// Model the daemon dying after provider Stop acceptance but before process-tree
	// quiescence was proven. A fresh generation may resume the conversation, but
	// must retain the visible unknown and must not release new work to the provider.
	secondConv := &historyDispatchConversation{dispatchConversation: &dispatchConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatTurnDispatch{
		Acceptance: ports.ChatTurnAcknowledged, Ref: ports.ChatTurnRef{ProviderTurnID: "must-not-dispatch"},
	}}}
	second := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: secondConv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("interrupt-crash-second")})
	t.Cleanup(func() { _ = second.Stop(context.Background(), testSession) })
	if _, err := second.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ProviderConversationID: firstCtrl.ProviderConversationID(), ExecutionPolicy: &policy}); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Send(context.Background(), testSession, ports.ChatUserMessage{Text: "must remain queued", ClientMessageID: "after-interrupt-crash"}); err != nil {
		t.Fatal(err)
	}
	claims, err = st.ListUnsettledGovernedControlCommands(context.Background())
	if err != nil || len(claims) != 1 || claims[0].State != domain.GovernedCommandDeliveryUnknown || claims[0].Quiescence != domain.GovernedCommandQuiescencePending || len(secondConv.sentMessages()) != 0 || firstConv.attemptCount() != 1 {
		t.Fatalf("restart claims=%+v replacement sends=%d interrupt attempts=%d err=%v", claims, len(secondConv.sentMessages()), firstConv.attemptCount(), err)
	}
}

func TestRestartRetainsBlockingGovernedControlUnknown(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	first := newFakeConversation()
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: first}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("restart-control")})
	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	claim := domain.GovernedControlCommand{ID: "control-unknown", IdempotencyKey: "interrupt:turn-1", RequestFingerprint: domain.ComputeGovernedControlFingerprint(testSession, domain.GovernedControlInterrupt, "interrupt:turn-1", "turn-1", "{}"), Class: domain.GovernedControlInterrupt, State: domain.GovernedCommandClaimed, SessionID: testSession, ControllerGeneration: ctrl.Generation(), ExpectedRevision: policy.PlanRevisionID.String(), CapabilityFingerprint: chatCapabilityFingerprintForTest(first.Capabilities()), ProviderConversationID: first.ProviderConversationID(), ProviderTurnID: "turn-1", Quiescence: domain.GovernedCommandQuiescencePending, CreatedAt: now, UpdatedAt: now}
	if _, made, err := st.CreateGovernedControlCommandClaim(context.Background(), claim); err != nil || !made {
		t.Fatalf("claim made=%v err=%v", made, err)
	}
	claim.State = domain.GovernedCommandDispatching
	claim.UpdatedAt = now.Add(time.Second)
	if ok, err := st.AdvanceGovernedControlCommand(context.Background(), claim, domain.GovernedCommandClaimed, claim.ControllerGeneration, claim.ExpectedRevision, claim.CapabilityFingerprint); err != nil || !ok {
		t.Fatalf("dispatch ok=%v err=%v", ok, err)
	}
	claim.State = domain.GovernedCommandDeliveryUnknown
	claim.UpdatedAt = now.Add(2 * time.Second)
	if ok, err := st.AdvanceGovernedControlCommand(context.Background(), claim, domain.GovernedCommandDispatching, claim.ControllerGeneration, claim.ExpectedRevision, claim.CapabilityFingerprint); err != nil || !ok {
		t.Fatalf("unknown ok=%v err=%v", ok, err)
	}
	if err := svc.Stop(context.Background(), testSession); err != nil {
		t.Fatal(err)
	}
	second := &dispatchConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatTurnDispatch{Acceptance: ports.ChatTurnAcknowledged, Ref: ports.ChatTurnRef{ProviderTurnID: "provider-new"}, TransportRequestID: 4, TransportSHA256: "new-sha", TransportBytes: 10, TransportSequence: 4}}
	svc = chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: second}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("restart-control-2")})
	if _, err := svc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ProviderConversationID: "thread-1", ExecutionPolicy: &policy}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Send(context.Background(), testSession, ports.ChatUserMessage{Text: "new work", ClientMessageID: "after-control-unknown"}); err != nil {
		t.Fatal(err)
	}
	if got := len(second.sentMessages()); got != 0 {
		t.Fatalf("provider dispatches=%d want 0", got)
	}
}

func chatCapabilityFingerprintForTest(capabilities ports.ChatCapabilities) string {
	enabled := make([]string, 0, len(capabilities))
	for capability, available := range capabilities {
		if available {
			enabled = append(enabled, string(capability))
		}
	}
	sort.Strings(enabled)
	payload, _ := json.Marshal(enabled)
	sum := sha256.Sum256(payload)
	return "chat-v1:" + hex.EncodeToString(sum[:])
}

func TestGovernedInterruptContainmentFailurePreservesCutoffAndBlocksDispatch(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	conv := &governedInterruptRecorder{interruptRecorder: newInterruptRecorder(), dispatch: ports.ChatInterruptDispatch{Acceptance: ports.ChatTurnAcknowledged, TransportRequestID: 3, TransportSHA256: "sha", TransportBytes: 10, TransportSequence: 3, Quiescence: domain.GovernedCommandQuiescencePending}, err: ports.ErrChatInterruptContainmentFailed}
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("containment")})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ctrl.Send(context.Background(), ports.ChatUserMessage{Text: "work", ClientMessageID: "turn"}); err != nil {
		t.Fatal(err)
	}
	conv.markActive("provider-turn-1")
	conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	if err = svc.Interrupt(context.Background(), testSession); !errors.Is(err, ports.ErrChatInterruptContainmentFailed) {
		t.Fatalf("err=%v", err)
	}
	if _, err = svc.Send(context.Background(), testSession, ports.ChatUserMessage{Text: "must stay queued", ClientMessageID: "after-failed-containment"}); err != nil {
		t.Fatal(err)
	}
	if conv.attemptCount() != 1 || len(conv.sentMessages()) != 1 {
		t.Fatalf("interrupts=%d sends=%d", conv.attemptCount(), len(conv.sentMessages()))
	}
	claims, e := st.ListUnsettledGovernedControlCommands(context.Background())
	if e != nil || len(claims) != 1 || claims[0].State != domain.GovernedCommandDeliveryUnknown {
		t.Fatalf("claims=%+v err=%v", claims, e)
	}
}

// TestSnapshotReportsGovernedTurnBlockTruthfully drives a turn claim through
// claimed -> dispatching -> delivery_unknown by hand (mirroring the manual
// claim construction TestRestartRetainsBlockingGovernedControlUnknown already
// uses for the control table) and checks the snapshot's GovernedTurnBlocks
// reports the claim's real state at every step. The point is amendment 5: a
// claim sitting in claimed or dispatching must never be reported (or
// mistakable for) delivery_unknown -- only the literal current state is ever
// echoed.
func TestSnapshotReportsGovernedTurnBlockTruthfully(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	conv := newFakeConversation()
	svc := chatsvc.New(chatsvc.Options{
		Store: st, Sessions: st, Reader: snapshotReader(st),
		Drivers: fakeRegistry{driver: fakeDriver{conv: conv}},
		Log:     slog.New(slog.DiscardHandler), NewID: sequentialID("turn-block"),
	})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{
		SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex,
		WorkspacePath: workspace, ExecutionPolicy: &policy,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	rec := domain.GovernedCommandRecord{GovernedCommandContract: domain.GovernedCommandContract{
		ID: "manual-turn-1", IdempotencyKey: "manual-turn-1",
		RequestFingerprint:    domain.ComputeGovernedTurnRequestFingerprint(testSession, "manual-turn-1", "text", ""),
		Class:                 domain.GovernedCommandTurn,
		State:                 domain.GovernedCommandClaimed,
		SessionID:             testSession,
		ControllerGeneration:  ctrl.Generation(),
		ExpectedRevision:      policy.PlanRevisionID.String(),
		CapabilityFingerprint: chatCapabilityFingerprintForTest(conv.Capabilities()),
		Correlation:           domain.GovernedCommandCorrelation{ProviderConversationID: conv.ProviderConversationID(), ClientMessageID: "manual-turn-1"},
		ReplayStrategy:        domain.GovernedCommandReplayUnavailable,
		Quiescence:            domain.GovernedCommandQuiescenceNotApplicable,
	}, CreatedAt: now, UpdatedAt: now}
	if _, _, err := st.CreateGovernedCommandClaim(ctx, rec); err != nil {
		t.Fatalf("create claim: %v", err)
	}

	// claimed does not yet block: BlocksConflictingDispatch requires
	// claimed/dispatching/delivery_unknown, and claimed itself qualifies, so the
	// claim is already visible here.
	snapshot, err := svc.Snapshot(ctx, testSession)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.GovernedTurnBlocks) != 1 || snapshot.GovernedTurnBlocks[0].State != domain.GovernedCommandClaimed {
		t.Fatalf("claimed blocks=%+v", snapshot.GovernedTurnBlocks)
	}
	if len(snapshot.GovernedControlBlocks) != 0 {
		t.Fatalf("unexpected control blocks=%+v", snapshot.GovernedControlBlocks)
	}

	rec.State = domain.GovernedCommandDispatching
	rec.UpdatedAt = now.Add(time.Second)
	if ok, err := st.AdvanceGovernedCommand(ctx, rec, domain.GovernedCommandClaimed, rec.ControllerGeneration, rec.ExpectedRevision, rec.CapabilityFingerprint); err != nil || !ok {
		t.Fatalf("advance to dispatching ok=%v err=%v", ok, err)
	}
	snapshot, err = svc.Snapshot(ctx, testSession)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.GovernedTurnBlocks) != 1 || snapshot.GovernedTurnBlocks[0].State != domain.GovernedCommandDispatching {
		t.Fatalf("dispatching wrongly reported: blocks=%+v", snapshot.GovernedTurnBlocks)
	}

	rec.State = domain.GovernedCommandDeliveryUnknown
	rec.UpdatedAt = now.Add(2 * time.Second)
	if ok, err := st.AdvanceGovernedCommand(ctx, rec, domain.GovernedCommandDispatching, rec.ControllerGeneration, rec.ExpectedRevision, rec.CapabilityFingerprint); err != nil || !ok {
		t.Fatalf("advance to delivery_unknown ok=%v err=%v", ok, err)
	}
	snapshot, err = svc.Snapshot(ctx, testSession)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.GovernedTurnBlocks) != 1 || snapshot.GovernedTurnBlocks[0].State != domain.GovernedCommandDeliveryUnknown ||
		snapshot.GovernedTurnBlocks[0].TurnID != "manual-turn-1" || !snapshot.GovernedTurnBlocks[0].UpdatedAt.Equal(rec.UpdatedAt) {
		t.Fatalf("delivery_unknown blocks=%+v", snapshot.GovernedTurnBlocks)
	}
}

// TestSnapshotGovernedBlocksIsolatedPerSessionAndKind seeds two sessions in the
// same store: session A gets a stuck turn claim, session B gets a stuck
// interrupt control claim shaped exactly like a containment-failed interrupt
// (kind=interrupt, state=delivery_unknown, quiescence=pending, ProviderTurnID
// set, RequestInstanceID empty). Each session's snapshot must see only its own
// claim, and only in the list matching that claim's own table.
func TestSnapshotGovernedBlocksIsolatedPerSessionAndKind(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	otherSession := domain.SessionID("p1-2")
	if _, err := st.CreateSession(ctx, domain.SessionRecord{
		ID: otherSession, ProjectID: testProject, Kind: domain.KindOrchestrator,
		Harness: domain.HarnessCodex, Mode: domain.SessionModeChat,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed second session: %v", err)
	}
	workspaceA, workspaceB := t.TempDir(), t.TempDir()
	policy := governedPolicy(t, workspaceA)
	policyB := governedPolicy(t, workspaceB)
	convA, convB := newFakeConversation(), newFakeConversation()
	driver := fakeDriver{start: func(cfg ports.ChatStartConfig) (ports.ChatConversation, error) {
		if cfg.SessionID == otherSession {
			return convB, nil
		}
		return convA, nil
	}}
	svc := chatsvc.New(chatsvc.Options{
		Store: st, Sessions: st, Reader: snapshotReader(st),
		Drivers: fakeRegistry{driver: driver},
		Log:     slog.New(slog.DiscardHandler), NewID: sequentialID("isolation"),
	})
	t.Cleanup(func() { _ = svc.Stop(ctx, testSession) })
	t.Cleanup(func() { _ = svc.Stop(ctx, otherSession) })
	ctrlA, err := svc.Start(ctx, chatsvc.StartConfig{
		SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex,
		WorkspacePath: workspaceA, ExecutionPolicy: &policy,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctrlB, err := svc.Start(ctx, chatsvc.StartConfig{
		SessionID: otherSession, ProjectID: testProject, Harness: domain.HarnessCodex,
		WorkspacePath: workspaceB, ExecutionPolicy: &policyB,
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	turnClaim := domain.GovernedCommandRecord{GovernedCommandContract: domain.GovernedCommandContract{
		ID: "manual-turn-a", IdempotencyKey: "manual-turn-a",
		RequestFingerprint:    domain.ComputeGovernedTurnRequestFingerprint(testSession, "manual-turn-a", "text", ""),
		Class:                 domain.GovernedCommandTurn,
		State:                 domain.GovernedCommandDispatching,
		SessionID:             testSession,
		ControllerGeneration:  ctrlA.Generation(),
		ExpectedRevision:      policy.PlanRevisionID.String(),
		CapabilityFingerprint: chatCapabilityFingerprintForTest(convA.Capabilities()),
		Correlation:           domain.GovernedCommandCorrelation{ProviderConversationID: convA.ProviderConversationID(), ClientMessageID: "manual-turn-a"},
		ReplayStrategy:        domain.GovernedCommandReplayUnavailable,
		Quiescence:            domain.GovernedCommandQuiescenceNotApplicable,
	}, CreatedAt: now, UpdatedAt: now}
	claimedTurn := turnClaim
	claimedTurn.State = domain.GovernedCommandClaimed
	if _, _, err := st.CreateGovernedCommandClaim(ctx, claimedTurn); err != nil {
		t.Fatalf("create turn claim: %v", err)
	}
	// AdvanceGovernedCommand's CAS requires updated_at strictly after created_at.
	turnClaim.UpdatedAt = now.Add(time.Second)
	if ok, err := st.AdvanceGovernedCommand(ctx, turnClaim, domain.GovernedCommandClaimed, turnClaim.ControllerGeneration, turnClaim.ExpectedRevision, turnClaim.CapabilityFingerprint); err != nil || !ok {
		t.Fatalf("advance turn claim ok=%v err=%v", ok, err)
	}

	controlClaim := domain.GovernedControlCommand{
		ID: "manual-interrupt-b", IdempotencyKey: "interrupt:turn-b",
		RequestFingerprint:     domain.ComputeGovernedControlFingerprint(otherSession, domain.GovernedControlInterrupt, "interrupt:turn-b", "turn-b", "{}"),
		Class:                  domain.GovernedControlInterrupt,
		State:                  domain.GovernedCommandClaimed,
		SessionID:              otherSession,
		ControllerGeneration:   ctrlB.Generation(),
		ExpectedRevision:       policyB.PlanRevisionID.String(),
		CapabilityFingerprint:  chatCapabilityFingerprintForTest(convB.Capabilities()),
		ProviderConversationID: convB.ProviderConversationID(),
		ProviderTurnID:         "provider-turn-b",
		Quiescence:             domain.GovernedCommandQuiescencePending,
		CreatedAt:              now, UpdatedAt: now,
	}
	if _, made, err := st.CreateGovernedControlCommandClaim(ctx, controlClaim); err != nil || !made {
		t.Fatalf("create control claim made=%v err=%v", made, err)
	}
	controlClaim.State = domain.GovernedCommandDispatching
	controlClaim.UpdatedAt = now.Add(time.Second)
	if ok, err := st.AdvanceGovernedControlCommand(ctx, controlClaim, domain.GovernedCommandClaimed, controlClaim.ControllerGeneration, controlClaim.ExpectedRevision, controlClaim.CapabilityFingerprint); err != nil || !ok {
		t.Fatalf("dispatch control claim ok=%v err=%v", ok, err)
	}
	controlClaim.State = domain.GovernedCommandDeliveryUnknown
	controlClaim.UpdatedAt = now.Add(2 * time.Second)
	if ok, err := st.AdvanceGovernedControlCommand(ctx, controlClaim, domain.GovernedCommandDispatching, controlClaim.ControllerGeneration, controlClaim.ExpectedRevision, controlClaim.CapabilityFingerprint); err != nil || !ok {
		t.Fatalf("unknown control claim ok=%v err=%v", ok, err)
	}

	snapshotA, err := svc.Snapshot(ctx, testSession)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshotA.GovernedTurnBlocks) != 1 || snapshotA.GovernedTurnBlocks[0].TurnID != "manual-turn-a" {
		t.Fatalf("session A turn blocks=%+v", snapshotA.GovernedTurnBlocks)
	}
	if len(snapshotA.GovernedControlBlocks) != 0 {
		t.Fatalf("session A leaked session B's control block: %+v", snapshotA.GovernedControlBlocks)
	}

	snapshotB, err := svc.Snapshot(ctx, otherSession)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshotB.GovernedTurnBlocks) != 0 {
		t.Fatalf("session B leaked session A's turn block: %+v", snapshotB.GovernedTurnBlocks)
	}
	if len(snapshotB.GovernedControlBlocks) != 1 {
		t.Fatalf("session B control blocks=%+v", snapshotB.GovernedControlBlocks)
	}
	block := snapshotB.GovernedControlBlocks[0]
	if block.Class != domain.GovernedControlInterrupt || block.State != domain.GovernedCommandDeliveryUnknown ||
		block.Quiescence != domain.GovernedCommandQuiescencePending || block.ProviderTurnID != "provider-turn-b" ||
		block.RequestInstanceID != "" {
		t.Fatalf("containment-failed-shaped control block=%+v", block)
	}
}

type concurrentAnswerConversation struct {
	*fakeConversation
	mu         sync.Mutex
	dispatches int
	dispatch   ports.ChatAnswerDispatch
}

func (c *concurrentAnswerConversation) DispatchAnswer(_ context.Context, _, _ string, _ ports.ChatDecision) (ports.ChatAnswerDispatch, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dispatches++
	return c.dispatch, nil
}

func (c *concurrentAnswerConversation) dispatchCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dispatches
}

func TestGovernedAnswerConcurrentDuplicateAndConflictHaveOneEffect(t *testing.T) {
	for _, tc := range []struct {
		name       string
		decisions  []ports.ChatDecision
		wantErrors int
	}{
		{name: "exact duplicates", decisions: []ports.ChatDecision{{ID: "accept"}, {ID: "accept"}}},
		{name: "changed fingerprint", decisions: []ports.ChatDecision{{ID: "accept"}, {ID: "decline"}}, wantErrors: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := openStore(t)
			workspace := t.TempDir()
			policy := governedPolicy(t, workspace)
			conv := &concurrentAnswerConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatAnswerDispatch{WriteOutcome: ports.ChatAnswerFrameWriteComplete}}
			svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("concurrent-answer")})
			t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
			ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
			if err != nil {
				t.Fatal(err)
			}
			conv.emit(ports.ChatEvent{Kind: ports.ChatEventApprovalRequested, RequestID: "answer-race", ProviderItemID: "answer-race", Summary: "allow?", Decisions: []ports.ChatDecisionOption{{ID: "accept"}, {ID: "decline"}}})
			awaitStoreSnapshot(t, st, ctrl.ConversationID(), func(s store.ConversationSnapshot) bool { return len(s.Activities) == 1 })
			start := make(chan struct{})
			errs := make(chan error, len(tc.decisions))
			var wg sync.WaitGroup
			for _, decision := range tc.decisions {
				decision := decision
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					errs <- svc.Resolve(context.Background(), testSession, "answer-race", decision)
				}()
			}
			close(start)
			wg.Wait()
			close(errs)
			errorCount := 0
			for resolveErr := range errs {
				if resolveErr != nil {
					errorCount++
					if tc.wantErrors == 1 && !errors.Is(resolveErr, domain.ErrGovernedCommandIdempotencyConflict) {
						t.Errorf("unexpected error: %v", resolveErr)
					}
				}
			}
			if errorCount != tc.wantErrors || conv.dispatchCount() != 1 {
				t.Fatalf("errors=%d want=%d provider dispatches=%d", errorCount, tc.wantErrors, conv.dispatchCount())
			}
		})
	}
}

func TestGovernedInterruptConcurrentDuplicatesPreserveOneCutoffAndEffect(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	conv := &governedInterruptRecorder{interruptRecorder: newInterruptRecorder(), dispatch: ports.ChatInterruptDispatch{
		Acceptance: ports.ChatTurnAcknowledged, TransportRequestID: 3, TransportSHA256: "interrupt-sha", TransportBytes: 18, TransportSequence: 3,
		Quiescence: domain.GovernedCommandQuiescenceCodexTree, QuiescenceEvidenceRef: "tree-stopped",
	}}
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("concurrent-interrupt")})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.Send(context.Background(), ports.ChatUserMessage{Text: "work", ClientMessageID: "interrupt-race-turn"}); err != nil {
		t.Fatal(err)
	}
	conv.markActive("provider-turn-1")
	conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; errs <- ctrl.Interrupt(context.Background()) }()
	}
	close(start)
	wg.Wait()
	close(errs)
	for interruptErr := range errs {
		if interruptErr != nil {
			t.Errorf("interrupt error: %v", interruptErr)
		}
	}
	if conv.attemptCount() != 1 {
		t.Fatalf("provider interrupts=%d want 1", conv.attemptCount())
	}
	claims, err := st.ListUnsettledGovernedControlCommands(context.Background())
	if err != nil || len(claims) != 0 {
		t.Fatalf("unsettled=%+v err=%v", claims, err)
	}
}

func TestGovernedRestartReconciliationReleasesQueuedWorkExactlyOnce(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	first := &dispatchConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatTurnDispatch{
		Acceptance: ports.ChatTurnDeliveryUnknown, TransportRequestID: 1, TransportSHA256: "unknown-sha", TransportBytes: 10, TransportSequence: 1,
	}, err: context.DeadlineExceeded}
	ids := sequentialID("reconcile-release")
	firstSvc := chatsvc.New(chatsvc.Options{Store: st, Reader: snapshotReader(st), Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: first}}, Log: slog.New(slog.DiscardHandler), NewID: ids})
	firstCtrl, err := firstSvc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := firstCtrl.Send(context.Background(), ports.ChatUserMessage{Text: "uncertain", ClientMessageID: "request-uncertain"}); !errors.Is(err, chatsvc.ErrGovernedDeliveryUnknown) {
		t.Fatalf("unknown send=%v", err)
	}
	if queued, err := firstCtrl.Send(context.Background(), ports.ChatUserMessage{Text: "after", ClientMessageID: "request-after"}); err != nil || queued.State != domain.TurnStateQueued {
		t.Fatalf("queued=%+v err=%v", queued, err)
	}
	conversationID := firstCtrl.ConversationID()
	if err := firstSvc.Stop(context.Background(), testSession); err != nil {
		t.Fatal(err)
	}

	secondBase := newFakeConversation()
	secondBase.providerConversationID = "thread-1"
	second := &dispatchHistoryConversation{dispatchConversation: &dispatchConversation{fakeConversation: secondBase, dispatch: ports.ChatTurnDispatch{
		Acceptance: ports.ChatTurnAcknowledged, Ref: ports.ChatTurnRef{ProviderTurnID: "provider-after"},
		TransportRequestID: 2, TransportSHA256: "after-sha", TransportBytes: 10, TransportSequence: 2,
	}}, history: []ports.ChatEvent{
		{Kind: ports.ChatEventUserMessageCompleted, ProviderEventID: "history-user", ProviderTurnID: "provider-uncertain", ProviderConversationID: "thread-1", ProviderItemID: "provider-user", ClientMessageID: "request-uncertain", Text: "uncertain"},
		{Kind: ports.ChatEventTurnCompleted, ProviderEventID: "history-complete", ProviderTurnID: "provider-uncertain", ProviderConversationID: "thread-1", TurnState: domain.TurnStateCompleted},
	}}
	secondSvc := chatsvc.New(chatsvc.Options{Store: st, Reader: snapshotReader(st), Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: second}}, Log: slog.New(slog.DiscardHandler), NewID: ids})
	t.Cleanup(func() { _ = secondSvc.Stop(context.Background(), testSession) })
	secondCtrl, err := secondSvc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ProviderConversationID: "thread-1", ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	if got := second.sentMessages(); len(got) != 1 || got[0].ClientMessageID != "request-after" {
		t.Fatalf("restart sends=%+v", got)
	}
	if replay, err := secondCtrl.Send(context.Background(), ports.ChatUserMessage{Text: "after", ClientMessageID: "request-after"}); err != nil || replay.ID != "" {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	if got := len(second.sentMessages()); got != 1 {
		t.Fatalf("restart dispatched queued work %d times", got)
	}
	snapshot, err := st.LoadConversationSnapshot(context.Background(), conversationID)
	if err != nil || len(snapshot.Turns) != 2 || snapshot.Turns[0].State != domain.TurnStateCompleted || snapshot.Turns[1].State != domain.TurnStateRunning {
		t.Fatalf("snapshot=%+v err=%v", snapshot.Turns, err)
	}
}

func TestGovernedRestartUnresolvedUnknownDoesNotReleaseQueuedWork(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	first := &dispatchConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatTurnDispatch{Acceptance: ports.ChatTurnDeliveryUnknown, TransportRequestID: 1, TransportSHA256: "unknown-sha", TransportBytes: 10, TransportSequence: 1}, err: context.DeadlineExceeded}
	ids := sequentialID("reconcile-block")
	firstSvc := chatsvc.New(chatsvc.Options{Store: st, Reader: snapshotReader(st), Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: first}}, Log: slog.New(slog.DiscardHandler), NewID: ids})
	firstCtrl, err := firstSvc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := firstCtrl.Send(context.Background(), ports.ChatUserMessage{Text: "uncertain", ClientMessageID: "request-uncertain"}); !errors.Is(err, chatsvc.ErrGovernedDeliveryUnknown) {
		t.Fatalf("unknown send=%v", err)
	}
	if _, err := firstCtrl.Send(context.Background(), ports.ChatUserMessage{Text: "after", ClientMessageID: "request-after"}); err != nil {
		t.Fatal(err)
	}
	if err := firstSvc.Stop(context.Background(), testSession); err != nil {
		t.Fatal(err)
	}
	secondBase := newFakeConversation()
	secondBase.providerConversationID = "thread-1"
	second := &dispatchHistoryConversation{dispatchConversation: &dispatchConversation{fakeConversation: secondBase}, history: nil}
	secondSvc := chatsvc.New(chatsvc.Options{Store: st, Reader: snapshotReader(st), Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: second}}, Log: slog.New(slog.DiscardHandler), NewID: ids})
	t.Cleanup(func() { _ = secondSvc.Stop(context.Background(), testSession) })
	if _, err := secondSvc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ProviderConversationID: "thread-1", ExecutionPolicy: &policy}); err != nil {
		t.Fatal(err)
	}
	if got := len(second.sentMessages()); got != 0 {
		t.Fatalf("unresolved restart dispatched %d turns", got)
	}
	claims, err := st.ListUnsettledGovernedCommands(context.Background())
	if err != nil || len(claims) != 2 {
		t.Fatalf("claims=%+v err=%v", claims, err)
	}
}

func TestGovernedAnswerStaleGenerationCannotReachProvider(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	conv := &answerDispatchConversation{fakeConversation: newFakeConversation(), dispatch: ports.ChatAnswerDispatch{WriteOutcome: ports.ChatAnswerFrameWriteComplete}}
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("stale-answer")})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	conv.emit(ports.ChatEvent{Kind: ports.ChatEventApprovalRequested, RequestID: "stale-answer", ProviderItemID: "stale-answer", Summary: "allow?", Decisions: []ports.ChatDecisionOption{{ID: "accept"}}})
	awaitStoreSnapshot(t, st, ctrl.ConversationID(), func(s store.ConversationSnapshot) bool { return len(s.Activities) == 1 })
	if err := st.ClaimChatControllerGeneration(context.Background(), testSession, "replacement-generation", "", "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := ctrl.Resolve(context.Background(), "stale-answer", ports.ChatDecision{ID: "accept"}); err == nil {
		t.Fatal("stale answer unexpectedly succeeded")
	}
	if conv.calls != 0 {
		t.Fatalf("stale generation reached answer provider %d times", conv.calls)
	}
	claims, err := st.ListUnsettledGovernedControlCommands(context.Background())
	if err != nil || len(claims) != 1 || claims[0].State != domain.GovernedCommandClaimed || claims[0].Class != domain.GovernedControlAnswer {
		t.Fatalf("claims=%+v err=%v", claims, err)
	}
}

func TestGovernedInterruptStaleGenerationCannotReachProviderOrClaimQuiescence(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	conv := &governedInterruptRecorder{interruptRecorder: newInterruptRecorder(), dispatch: ports.ChatInterruptDispatch{
		Acceptance: ports.ChatTurnAcknowledged, TransportRequestID: 3, TransportSHA256: "interrupt-sha", TransportBytes: 18, TransportSequence: 3,
		Quiescence: domain.GovernedCommandQuiescenceCodexTree, QuiescenceEvidenceRef: "must-not-land",
	}}
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("stale-interrupt")})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.Send(context.Background(), ports.ChatUserMessage{Text: "work", ClientMessageID: "stale-interrupt-turn"}); err != nil {
		t.Fatal(err)
	}
	conv.markActive("provider-turn-1")
	conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	if err := st.ClaimChatControllerGeneration(context.Background(), testSession, "replacement-generation", "", "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := ctrl.Interrupt(context.Background()); err == nil {
		t.Fatal("stale interrupt unexpectedly succeeded")
	}
	if conv.attemptCount() != 0 {
		t.Fatalf("stale generation reached interrupt provider %d times", conv.attemptCount())
	}
	claims, err := st.ListUnsettledGovernedControlCommands(context.Background())
	if err != nil || len(claims) != 1 || claims[0].State != domain.GovernedCommandClaimed || claims[0].Quiescence != domain.GovernedCommandQuiescencePending || claims[0].QuiescenceEvidenceRef != "" {
		t.Fatalf("claims=%+v err=%v", claims, err)
	}
}

type concurrentInputAnswerConversation struct {
	*fakeConversation
	mu         sync.Mutex
	dispatches int
}

func (c *concurrentInputAnswerConversation) DispatchInput(_ context.Context, _, _ string, _ ports.ChatInputResponse) (ports.ChatAnswerDispatch, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dispatches++
	return ports.ChatAnswerDispatch{WriteOutcome: ports.ChatAnswerFrameWriteComplete}, nil
}
func TestGovernedTypedInputConcurrentAnswersHaveOneEffect(t *testing.T) {
	st := openStore(t)
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	conv := &concurrentInputAnswerConversation{fakeConversation: newFakeConversation()}
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("typed-answer")})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex, WorkspacePath: workspace, ExecutionPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	conv.emit(ports.ChatEvent{Kind: ports.ChatEventInputRequested, RequestID: "input-race", ProviderItemID: "input-race", Input: &ports.ChatInputRequest{Mode: ports.ChatInputModeForm, Message: "name", Schema: map[string]any{"type": "object"}}})
	awaitStoreSnapshot(t, st, ctrl.ConversationID(), func(s store.ConversationSnapshot) bool { return len(s.Activities) == 1 })
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, v := range []string{"a", "b"} {
		v := v
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- svc.ResolveInputWithKey(context.Background(), testSession, "input-race", "key-"+v, ports.ChatInputResponse{Action: ports.ChatInputActionAccept, Content: map[string]any{"name": v}})
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	errorCount := 0
	for e := range errs {
		if e != nil {
			errorCount++
		}
	}
	conv.mu.Lock()
	calls := conv.dispatches
	conv.mu.Unlock()
	if calls != 1 || errorCount != 1 {
		t.Fatalf("dispatches=%d errors=%d", calls, errorCount)
	}
}
