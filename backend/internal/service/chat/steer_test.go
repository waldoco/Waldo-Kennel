package chat_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	chatsvc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/chat"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/store"
)

// Steering scenarios.
//
// The promise under test is not "the provider was called". It is that guidance goes
// to the turn the user is actually watching, that it shows up on the timeline so
// they can see it landed, and that every refusal is a typed answer rather than a
// failure — because the moment someone steers is the moment a turn is ending
// underneath them.

/* ---- a provider double that can be steered ----------------------------- */

type steerCall struct {
	turnID string
	msg    ports.ChatUserMessage
}

type steerRecorder struct {
	*fakeConversation

	mu     sync.Mutex
	calls  []steerCall
	err    error
	landed string
}

type cancelAfterSteerRecorder struct {
	*steerRecorder
	cancel context.CancelFunc
}

func (s *cancelAfterSteerRecorder) Steer(
	ctx context.Context,
	providerTurnID string,
	msg ports.ChatUserMessage,
) (ports.ChatTurnRef, error) {
	ref, err := s.steerRecorder.Steer(ctx, providerTurnID, msg)
	s.cancel()
	return ref, err
}

func newSteerRecorder() *steerRecorder {
	return &steerRecorder{fakeConversation: newFakeConversation()}
}

func (s *steerRecorder) Steer(
	_ context.Context,
	providerTurnID string,
	msg ports.ChatUserMessage,
) (ports.ChatTurnRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, steerCall{turnID: providerTurnID, msg: msg})
	if s.err != nil {
		return ports.ChatTurnRef{}, s.err
	}
	landed := s.landed
	if landed == "" {
		landed = providerTurnID
	}
	return ports.ChatTurnRef{ProviderTurnID: landed}, nil
}

func (s *steerRecorder) steers() []steerCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]steerCall(nil), s.calls...)
}

func (s *steerRecorder) failWith(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

// steerHarness starts a session whose provider can be steered, and puts a turn in
// flight — the only state steering is meaningful in.
func steerHarness(t *testing.T) (*harness, *steerRecorder) {
	t.Helper()
	provider := newSteerRecorder()
	h := newHarnessWithConversation(t, provider)

	if _, err := h.svc.Send(context.Background(), testSession, ports.ChatUserMessage{
		Text:            "do the long thing",
		ClientMessageID: "turn-1",
		Origin:          domain.MessageOriginHuman,
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	// The provider's own acknowledgement. Steering is refused for a turn the provider
	// has not announced, so nothing can be steered before this arrives.
	provider.emit(ports.ChatEvent{
		Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1",
	})
	return h, provider
}

// steerMarkers reads the steer entries out of a timeline the way a renderer must: by
// the discriminator in the detail payload. `system` is a general bucket, so the
// activity kind alone does not identify one.
func steerMarkers(s store.ConversationSnapshot) []struct {
	activity domain.ConversationActivity
	detail   struct {
		Event           string `json:"event"`
		Text            string `json:"text"`
		Origin          string `json:"origin"`
		ClientMessageID string `json:"clientMessageId"`
	}
} {
	type marker = struct {
		activity domain.ConversationActivity
		detail   struct {
			Event           string `json:"event"`
			Text            string `json:"text"`
			Origin          string `json:"origin"`
			ClientMessageID string `json:"clientMessageId"`
		}
	}
	var found []marker
	for _, a := range s.Activities {
		if a.Kind != domain.ActivityKindSystem || len(a.Detail) == 0 {
			continue
		}
		var m marker
		if err := json.Unmarshal(a.Detail, &m.detail); err != nil {
			continue
		}
		if m.detail.Event != "steer" {
			continue
		}
		m.activity = a
		found = append(found, m)
	}
	return found
}

/* ---- tests ------------------------------------------------------------- */

// The whole feature: guidance reaches the running turn and the timeline says so.
func TestSteerReachesTheRunningTurnAndLandsOnTheTimeline(t *testing.T) {
	h, provider := steerHarness(t)
	ctx := context.Background()

	result, err := h.svc.Steer(ctx, testSession, ports.ChatUserMessage{
		Text:            "actually, just summarize what you have",
		ClientMessageID: "steer-1",
		Origin:          domain.MessageOriginHuman,
	})
	if err != nil {
		t.Fatalf("Steer: %v", err)
	}
	if result.ProviderTurnID != "provider-turn-1" {
		t.Errorf("steered turn = %q, want provider-turn-1", result.ProviderTurnID)
	}
	if result.ActivityID == "" {
		t.Error("no activity id reported; a client cannot reconcile its own bubble")
	}

	calls := provider.steers()
	if len(calls) != 1 {
		t.Fatalf("provider saw %d steers, want 1", len(calls))
	}
	// The turn is named as a precondition rather than left to the provider to guess,
	// which is what stops a correction landing on work the user was not watching.
	if calls[0].turnID != "provider-turn-1" {
		t.Errorf("steered turn id = %q, want provider-turn-1", calls[0].turnID)
	}
	if calls[0].msg.Text != "actually, just summarize what you have" {
		t.Errorf("steer text = %q", calls[0].msg.Text)
	}
	if calls[0].msg.ClientMessageID != "steer-1" {
		t.Errorf("idempotency handle = %q, want steer-1", calls[0].msg.ClientMessageID)
	}

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(steerMarkers(s)) == 1
	})
	markers := steerMarkers(snapshot)
	if markers[0].detail.Text != "actually, just summarize what you have" {
		t.Errorf("recorded text = %q", markers[0].detail.Text)
	}
	if markers[0].detail.Origin != string(domain.MessageOriginHuman) {
		t.Errorf("recorded origin = %q, want human", markers[0].detail.Origin)
	}
	if markers[0].detail.ClientMessageID != "steer-1" {
		t.Errorf("recorded client message id = %q", markers[0].detail.ClientMessageID)
	}
	if markers[0].activity.Summary == "" {
		t.Error("the row has no summary; a collapsed timeline would show an empty entry")
	}

	// Bound to the turn it steered, not floating: an unattached row would leave the
	// guidance rendering outside the conversation it changed.
	var running string
	for _, turn := range snapshot.Turns {
		if turn.ProviderTurnID == "provider-turn-1" {
			running = turn.ID
		}
	}
	if running == "" {
		t.Fatalf("no turn row for provider-turn-1:\n%+v", snapshot.Turns)
	}
	if markers[0].activity.TurnID != running {
		t.Errorf("steer recorded on turn %q, want the running turn %q",
			markers[0].activity.TurnID, running)
	}

	// And it must not have opened a turn of its own. A second turn row would be
	// dispatched by the drain loop later, sending the correction twice.
	if len(snapshot.Turns) != 1 {
		t.Errorf("steering produced %d turns, want 1:\n%+v", len(snapshot.Turns), snapshot.Turns)
	}
}

// A retry with the same handle is the same guidance, not a second piece of it.
func TestSteerIsIdempotentOnTheClientHandle(t *testing.T) {
	h, _ := steerHarness(t)
	ctx := context.Background()

	msg := ports.ChatUserMessage{Text: "narrow the search", ClientMessageID: "steer-retry"}
	if _, err := h.svc.Steer(ctx, testSession, msg); err != nil {
		t.Fatalf("first Steer: %v", err)
	}
	if _, err := h.svc.Steer(ctx, testSession, msg); err != nil {
		t.Fatalf("retried Steer: %v", err)
	}

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(steerMarkers(s)) >= 1
	})
	if got := len(steerMarkers(snapshot)); got != 1 {
		t.Errorf("a retried steer produced %d timeline entries, want 1", got)
	}
}

// Nothing in flight is an ordinary outcome — the turn finished while the user was
// typing — and the provider must not be asked.
func TestSteerWithNothingInFlightIsTypedAndNeverReachesTheProvider(t *testing.T) {
	provider := newSteerRecorder()
	h := newHarnessWithConversation(t, provider)

	_, err := h.svc.Steer(context.Background(), testSession,
		ports.ChatUserMessage{Text: "too late"})
	if !errors.Is(err, chatsvc.ErrNoActiveTurn) {
		t.Fatalf("err = %v, want ErrNoActiveTurn", err)
	}
	if len(provider.steers()) != 0 {
		t.Error("asked the provider to steer with no turn in flight")
	}
}

// The provider is the authority on whether its turn is still steerable, and losing
// that race must read as "nothing to steer", not as a failure.
func TestSteerRaceLostToTheProviderIsReportedAsNoActiveTurn(t *testing.T) {
	h, provider := steerHarness(t)
	provider.failWith(ports.ErrChatNoSteerableTurn)

	_, err := h.svc.Steer(context.Background(), testSession,
		ports.ChatUserMessage{Text: "guidance"})
	if !errors.Is(err, chatsvc.ErrNoActiveTurn) {
		t.Fatalf("err = %v, want ErrNoActiveTurn", err)
	}

	// Nothing recorded: a timeline claiming guidance the agent never received would
	// have the user waiting for an answer to something it never heard.
	snapshot, loadErr := h.st.LoadConversationSnapshot(context.Background(), h.ctrl.ConversationID())
	if loadErr != nil {
		t.Fatalf("load snapshot: %v", loadErr)
	}
	if got := len(steerMarkers(snapshot)); got != 0 {
		t.Errorf("recorded %d steers for a refused one", got)
	}
}

// A turn that is running but cannot take guidance (a compaction, a review) is a
// different answer: retryable once it ends, so it keeps its own sentinel.
func TestSteerOfAnUnsteerableTurnKeepsItsOwnOutcome(t *testing.T) {
	h, provider := steerHarness(t)
	provider.failWith(ports.ErrChatTurnNotSteerable)

	_, err := h.svc.Steer(context.Background(), testSession,
		ports.ChatUserMessage{Text: "guidance"})
	if !errors.Is(err, chatsvc.ErrTurnNotSteerable) {
		t.Fatalf("err = %v, want ErrTurnNotSteerable", err)
	}
	if errors.Is(err, chatsvc.ErrNoActiveTurn) {
		t.Error("an unsteerable running turn was reported as no turn at all")
	}
}

// A provider with no steering at all: a permanent answer, so a client hides the
// control instead of retrying.
func TestSteerIsRefusedWhenTheDriverCannotDoIt(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "do the long thing", ClientMessageID: "turn-1",
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	h.conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})

	_, err := h.svc.Steer(ctx, testSession, ports.ChatUserMessage{Text: "guidance"})
	if !errors.Is(err, chatsvc.ErrSteerUnsupported) {
		t.Fatalf("err = %v, want ErrSteerUnsupported", err)
	}
}

func TestSteerRejectsEmptyText(t *testing.T) {
	h, provider := steerHarness(t)

	_, err := h.svc.Steer(context.Background(), testSession,
		ports.ChatUserMessage{Text: "  \n "})
	if !errors.Is(err, chatsvc.ErrSteerTextRequired) {
		t.Fatalf("err = %v, want ErrSteerTextRequired", err)
	}
	if len(provider.steers()) != 0 {
		t.Error("sent an empty steer to the provider")
	}
}

// The trap this waits for is real: the provider refuses a steer for a turn it has
// accepted but not yet announced, and steering is most useful in exactly that
// window. So a steer that arrives between dispatch and acknowledgement must WAIT for
// the acknowledgement rather than being refused or fired early.
func TestSteerWaitsForTheProviderToAcknowledgeTheTurn(t *testing.T) {
	provider := newSteerRecorder()
	h := newHarnessWithConversation(t, provider)
	ctx := context.Background()

	if _, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "do the long thing", ClientMessageID: "turn-1",
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	// Dispatched but NOT acknowledged: no turn/started has arrived.

	done := make(chan error, 1)
	go func() {
		_, err := h.svc.Steer(ctx, testSession, ports.ChatUserMessage{Text: "guidance"})
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("steer resolved before the provider acknowledged the turn (err=%v); "+
			"the provider would have refused it", err)
	case <-time.After(150 * time.Millisecond):
	}

	provider.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Steer after acknowledgement: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("steer never completed after the turn was acknowledged")
	}

	calls := provider.steers()
	if len(calls) != 1 || calls[0].turnID != "provider-turn-1" {
		t.Fatalf("provider saw %+v, want one steer for provider-turn-1", calls)
	}
}

// The provider names the turn its guidance joined, and Kennel attributes the row to
// that turn rather than to the one it asked about. Same id in practice; asserted so
// a provider that answered differently could not be silently misfiled.
func TestSteerRecordsTheTurnTheProviderNames(t *testing.T) {
	h, provider := steerHarness(t)
	provider.mu.Lock()
	provider.landed = "provider-turn-1"
	provider.mu.Unlock()

	result, err := h.svc.Steer(context.Background(), testSession,
		ports.ChatUserMessage{Text: "guidance"})
	if err != nil {
		t.Fatalf("Steer: %v", err)
	}
	if result.ProviderTurnID != "provider-turn-1" {
		t.Errorf("reported turn = %q, want the one the provider named", result.ProviderTurnID)
	}
}

// Promoting a selected queued turn must use Kennel's durable content, attach it to
// the running provider turn, and remove only that source turn from the visible
// queue. If this regresses to queue-head-only behavior, the second message below
// is never the one the provider receives.
func TestPromoteSelectedQueuedTurnIntoTheRunningTurn(t *testing.T) {
	h, provider := steerHarness(t)
	ctx := context.Background()

	first, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "first queued", ClientMessageID: "queued-1", Origin: domain.MessageOriginHuman,
	})
	if err != nil {
		t.Fatalf("queue first: %v", err)
	}
	selected, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "second queued", ClientMessageID: "queued-2", Origin: domain.MessageOriginHuman,
		Content: []ports.ChatContent{{Type: "image", Data: "aGVsbG8=", MIMEType: "image/png"}},
	})
	if err != nil {
		t.Fatalf("queue selected: %v", err)
	}

	result, err := h.svc.PromoteQueuedTurn(ctx, testSession, selected.ID)
	if err != nil {
		t.Fatalf("PromoteQueuedTurn: %v", err)
	}
	if result.SourceTurnID != selected.ID || result.ProviderTurnID != "provider-turn-1" || result.ActivityID == "" {
		t.Fatalf("promotion result = %+v", result)
	}
	calls := provider.steers()
	if len(calls) != 1 {
		t.Fatalf("provider steers = %+v, want one", calls)
	}
	if calls[0].msg.Text != "second queued" || calls[0].msg.ClientMessageID != "queued-2" {
		t.Fatalf("provider message = %+v, want selected durable message", calls[0].msg)
	}
	if len(calls[0].msg.Content) != 1 || calls[0].msg.Content[0].MIMEType != "image/png" {
		t.Fatalf("provider content = %+v, want stored image", calls[0].msg.Content)
	}

	snapshot := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		return len(steerMarkers(s)) == 1
	})
	for _, turn := range snapshot.Turns {
		if turn.ID == selected.ID {
			t.Fatalf("promoted source turn remains visible: %+v", turn)
		}
	}
	next, err := h.st.NextQueuedTurn(ctx, h.ctrl.ConversationID())
	if err != nil {
		t.Fatalf("remaining queue: %v", err)
	}
	if next.TurnID != first.ID {
		t.Fatalf("remaining queue head = %q, want %q", next.TurnID, first.ID)
	}
}

// A provider refusal has not delivered anything, so the exact selected message
// must return to its original queue position instead of being lost or failed.
func TestPromoteQueuedTurnRefusalRestoresItsQueuePosition(t *testing.T) {
	h, provider := steerHarness(t)
	ctx := context.Background()
	queued, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "keep me queued", ClientMessageID: "queued-refused", Origin: domain.MessageOriginHuman,
	})
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	provider.failWith(ports.ErrChatTurnNotSteerable)

	_, err = h.svc.PromoteQueuedTurn(ctx, testSession, queued.ID)
	if !errors.Is(err, chatsvc.ErrTurnNotSteerable) {
		t.Fatalf("promotion error = %v, want ErrTurnNotSteerable", err)
	}
	next, err := h.st.NextQueuedTurn(ctx, h.ctrl.ConversationID())
	if err != nil || next.TurnID != queued.ID {
		t.Fatalf("restored queue head = %+v, %v; want %s", next, err, queued.ID)
	}
}

// Only human-originated queue items are eligible for mid-turn guidance. The
// service must enforce that boundary even when a caller bypasses the frontend,
// without consuming or reordering the automation item.
func TestPromoteQueuedTurnRejectsNonHumanSourceWithoutContactingProvider(t *testing.T) {
	h, provider := steerHarness(t)
	ctx := context.Background()
	queued, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text: "automation follow-up", ClientMessageID: "queued-automation", Origin: domain.MessageOriginAutomation,
	})
	if err != nil {
		t.Fatalf("queue automation turn: %v", err)
	}

	_, err = h.svc.PromoteQueuedTurn(ctx, testSession, queued.ID)
	if !errors.Is(err, chatsvc.ErrTurnNotQueued) {
		t.Fatalf("promotion error = %v, want ErrTurnNotQueued", err)
	}
	if calls := provider.steers(); len(calls) != 0 {
		t.Fatalf("provider received %d steer attempts, want none", len(calls))
	}
	next, err := h.st.NextQueuedTurn(ctx, h.ctrl.ConversationID())
	if err != nil {
		t.Fatalf("load queue after rejection: %v", err)
	}
	if next.TurnID != queued.ID || next.Origin != domain.MessageOriginAutomation {
		t.Fatalf("queue head after rejection = %+v, want unchanged automation turn %s", next, queued.ID)
	}
}

// A transport failure after the request leaves delivery unknowable. Returning the
// source to the queue would let drain send guidance the provider may already have
// accepted, so it must settle failed and require an explicit user decision.
func TestPromoteQueuedTurnAmbiguousProviderFailureSettlesUncertainWithoutRedelivery(t *testing.T) {
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	t.Cleanup(cancelRequest)
	provider := &cancelAfterSteerRecorder{steerRecorder: newSteerRecorder(), cancel: cancelRequest}
	h := newHarnessWithConversation(t, provider)
	storeCtx := context.Background()
	if _, err := h.svc.Send(storeCtx, testSession, ports.ChatUserMessage{
		Text: "do the long thing", ClientMessageID: "turn-1", Origin: domain.MessageOriginHuman,
	}); err != nil {
		t.Fatalf("start running turn: %v", err)
	}
	provider.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	queued, err := h.svc.Send(storeCtx, testSession, ports.ChatUserMessage{
		Text: "deliver me at most once", ClientMessageID: "queued-uncertain", Origin: domain.MessageOriginHuman,
	})
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	transportErr := errors.New("connection lost after request write")
	provider.failWith(transportErr)

	_, err = h.svc.PromoteQueuedTurn(requestCtx, testSession, queued.ID)
	if !errors.Is(err, chatsvc.ErrPromotionUncertain) {
		t.Fatalf("promotion error = %v, want ErrPromotionUncertain", err)
	}
	if !errors.Is(err, transportErr) {
		t.Fatalf("promotion error = %v, want transport cause", err)
	}

	snapshot, err := h.st.LoadConversationSnapshot(storeCtx, h.ctrl.ConversationID())
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	var source *domain.ConversationTurn
	for index := range snapshot.Turns {
		if snapshot.Turns[index].ID == queued.ID {
			source = &snapshot.Turns[index]
			break
		}
	}
	if source == nil {
		t.Fatalf("uncertain source turn %s is not visible", queued.ID)
	}
	if source.State != domain.TurnStateFailed || source.ErrorMessage != chatsvc.ErrPromotionUncertain.Error() {
		t.Fatalf("uncertain source = %+v, want failed with promotion-uncertain error", *source)
	}
	if _, err := h.st.NextQueuedTurn(storeCtx, h.ctrl.ConversationID()); !errors.Is(err, domain.ErrNoQueuedTurn) {
		t.Fatalf("uncertain source remained drainable: %v", err)
	}

	_, retryErr := h.svc.PromoteQueuedTurn(storeCtx, testSession, queued.ID)
	if !errors.Is(retryErr, chatsvc.ErrTurnNotQueued) {
		t.Fatalf("retry error = %v, want ErrTurnNotQueued", retryErr)
	}
	if calls := provider.steers(); len(calls) != 1 {
		t.Fatalf("provider received %d steer attempts, want one", len(calls))
	}
}

/* ---- governed steer acceptance --------------------------------------- */

type governedSteerRecorder struct {
	*steerRecorder
	dispatch    ports.ChatSteerDispatch
	dispatchErr error
	onDispatch  func()
}

func (s *governedSteerRecorder) DispatchSteer(_ context.Context, turn string, msg ports.ChatUserMessage) (ports.ChatSteerDispatch, error) {
	s.mu.Lock()
	s.calls = append(s.calls, steerCall{turnID: turn, msg: msg})
	s.mu.Unlock()
	if s.onDispatch != nil {
		s.onDispatch()
	}
	return s.dispatch, s.dispatchErr
}

type failSteerActivityStore struct {
	chatsvc.Store
	err error
}

func (s *failSteerActivityStore) UpsertActivity(ctx context.Context, conversationID, providerTurnID string, activity domain.ConversationActivity, now time.Time) error {
	var detail struct {
		Event string `json:"event"`
	}
	if json.Unmarshal(activity.Detail, &detail) == nil && detail.Event == "steer" {
		return s.err
	}
	return s.Store.UpsertActivity(ctx, conversationID, providerTurnID, activity, now)
}

func (s *governedSteerRecorder) DispatchTurn(_ context.Context, msg ports.ChatUserMessage) (ports.ChatTurnDispatch, error) {
	s.mu.Lock()
	s.sent = append(s.sent, msg)
	s.mu.Unlock()
	return ports.ChatTurnDispatch{
		Acceptance:         ports.ChatTurnAcknowledged,
		Ref:                ports.ChatTurnRef{ProviderTurnID: "provider-turn-1"},
		TransportRequestID: 1, TransportSHA256: "turn-sha", TransportBytes: 12, TransportSequence: 1,
	}, nil
}

func governedSteerHarness(t *testing.T, conv *governedSteerRecorder, wrap func(*store.Store) chatsvc.Store) (*harness, domain.AttemptExecutionPolicy) {
	t.Helper()
	st := openStore(t)
	if wrap == nil {
		wrap = func(st *store.Store) chatsvc.Store { return st }
	}
	workspace := t.TempDir()
	policy := governedPolicy(t, workspace)
	h := &harness{st: st, conv: conv.fakeConversation, activity: &recordingActivity{}, clock: time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)}
	svc := chatsvc.New(chatsvc.Options{
		Store: wrap(st), Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}},
		Activity: h.activity, Log: slog.New(slog.DiscardHandler), NewID: sequentialID("governed-steer"),
	})
	ctrl, err := svc.Start(context.Background(), chatsvc.StartConfig{
		SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessCodex,
		WorkspacePath: workspace, ExecutionPolicy: &policy,
	})
	if err != nil {
		t.Fatalf("start governed controller: %v", err)
	}
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })
	h.svc, h.ctrl = svc, ctrl
	if _, err := svc.Send(context.Background(), testSession, ports.ChatUserMessage{Text: "work", ClientMessageID: "turn-governed"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	conv.emit(ports.ChatEvent{Kind: ports.ChatEventTurnStarted, ProviderTurnID: "provider-turn-1"})
	return h, policy
}
func TestGovernedSteerPersistsDispatchingBeforeProviderContact(t *testing.T) {
	conv := &governedSteerRecorder{steerRecorder: newSteerRecorder(), dispatch: ports.ChatSteerDispatch{
		Acceptance: ports.ChatTurnAcknowledged, Ref: ports.ChatTurnRef{ProviderTurnID: "provider-turn-1"},
		TransportRequestID: 2, TransportSHA256: "steer-sha", TransportBytes: 17, TransportSequence: 2,
	}}
	h, _ := governedSteerHarness(t, conv, nil)
	conv.onDispatch = func() {
		claims, err := h.st.ListUnsettledGovernedControlCommands(context.Background())
		if err != nil || len(claims) != 1 || claims[0].State != domain.GovernedCommandDispatching || claims[0].Class != domain.GovernedControlSteer {
			t.Fatalf("provider contacted before durable governed steer dispatch: claims=%+v err=%v", claims, err)
		}
	}
	result, err := h.svc.Steer(context.Background(), testSession, ports.ChatUserMessage{Text: "narrow it", ClientMessageID: "governed-steer-1"})
	if err != nil {
		t.Fatalf("Steer: %v", err)
	}
	claim, ok, err := h.st.GetGovernedControlCommand(context.Background(), result.ActivityID)
	if err != nil || !ok || claim.State != domain.GovernedCommandAcknowledged || claim.ProviderTurnID != "provider-turn-1" {
		t.Fatalf("claim=%+v ok=%v err=%v", claim, ok, err)
	}
}

func TestGovernedSteerRejectedAndUnknownStayDistinct(t *testing.T) {
	for _, tc := range []struct {
		name   string
		accept ports.ChatTurnAcceptance
		want   error
		state  domain.GovernedCommandState
	}{
		{"rejected", ports.ChatTurnRejected, chatsvc.ErrProviderRefused, domain.GovernedCommandRejected},
		{"unknown", ports.ChatTurnDeliveryUnknown, chatsvc.ErrSteerDeliveryUnknown, domain.GovernedCommandDeliveryUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conv := &governedSteerRecorder{steerRecorder: newSteerRecorder(), dispatch: ports.ChatSteerDispatch{
				Acceptance: tc.accept, TransportRequestID: 2, TransportSHA256: "steer-sha", TransportBytes: 17, TransportSequence: 2,
			}}
			h, _ := governedSteerHarness(t, conv, nil)
			msg := ports.ChatUserMessage{Text: "narrow it", ClientMessageID: "governed-steer-" + tc.name}
			if _, err := h.svc.Steer(context.Background(), testSession, msg); !errors.Is(err, tc.want) {
				t.Fatalf("Steer err=%v, want %v", err, tc.want)
			}
			claims, err := h.st.ListUnsettledGovernedControlCommands(context.Background())
			if tc.state == domain.GovernedCommandRejected {
				// Rejection is settled, so read the claim via its stable generated ID
				// from the full snapshot is unnecessary; no unsettled claim may remain.
				if err != nil || len(claims) != 0 {
					t.Fatalf("rejected claims=%+v err=%v", claims, err)
				}
			} else if err != nil || len(claims) != 1 || claims[0].State != tc.state {
				t.Fatalf("unknown claims=%+v err=%v", claims, err)
			}
		})
	}
}

func TestGovernedSteerExactReplayDoesNotContactProviderAgain(t *testing.T) {
	conv := &governedSteerRecorder{steerRecorder: newSteerRecorder(), dispatch: ports.ChatSteerDispatch{
		Acceptance: ports.ChatTurnAcknowledged, Ref: ports.ChatTurnRef{ProviderTurnID: "provider-turn-1"},
		TransportRequestID: 2, TransportSHA256: "steer-sha", TransportBytes: 17, TransportSequence: 2,
	}}
	h, _ := governedSteerHarness(t, conv, nil)
	msg := ports.ChatUserMessage{Text: "narrow it", ClientMessageID: "governed-steer-replay"}
	first, err := h.svc.Steer(context.Background(), testSession, msg)
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.svc.Steer(context.Background(), testSession, msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(conv.steers()) != 1 || first.ActivityID != second.ActivityID {
		t.Fatalf("calls=%d first=%+v second=%+v", len(conv.steers()), first, second)
	}
}

func TestGovernedSteerProviderAckWithTimelineFailureBecomesUnknown(t *testing.T) {
	conv := &governedSteerRecorder{steerRecorder: newSteerRecorder(), dispatch: ports.ChatSteerDispatch{
		Acceptance: ports.ChatTurnAcknowledged, Ref: ports.ChatTurnRef{ProviderTurnID: "provider-turn-1"},
		TransportRequestID: 2, TransportSHA256: "steer-sha", TransportBytes: 17, TransportSequence: 2,
	}}
	injected := errors.New("timeline write failed")
	h, _ := governedSteerHarness(t, conv, func(st *store.Store) chatsvc.Store {
		return &failSteerActivityStore{Store: st, err: injected}
	})
	msg := ports.ChatUserMessage{Text: "narrow it", ClientMessageID: "governed-steer-local-fail"}
	if _, err := h.svc.Steer(context.Background(), testSession, msg); !errors.Is(err, chatsvc.ErrSteerDeliveryUnknown) || !errors.Is(err, injected) {
		t.Fatalf("Steer err=%v", err)
	}
	if _, err := h.svc.Steer(context.Background(), testSession, msg); !errors.Is(err, chatsvc.ErrSteerDeliveryUnknown) {
		t.Fatalf("replay err=%v", err)
	}
	if len(conv.steers()) != 1 {
		t.Fatalf("provider contacts=%d, want 1", len(conv.steers()))
	}
	claims, err := h.st.ListUnsettledGovernedControlCommands(context.Background())
	if err != nil || len(claims) != 1 || claims[0].State != domain.GovernedCommandDeliveryUnknown {
		t.Fatalf("claims=%+v err=%v", claims, err)
	}
}

func TestGovernedSteerConcurrentDuplicateAndConflictHaveOneEffect(t *testing.T) {
	for _, tc := range []struct {
		name       string
		messages   []ports.ChatUserMessage
		wantErrors int
	}{
		{name: "exact duplicates", messages: []ports.ChatUserMessage{{Text: "narrow it", ClientMessageID: "concurrent-steer"}, {Text: "narrow it", ClientMessageID: "concurrent-steer"}}},
		{name: "changed fingerprint", messages: []ports.ChatUserMessage{{Text: "first", ClientMessageID: "concurrent-steer"}, {Text: "changed", ClientMessageID: "concurrent-steer"}}, wantErrors: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conv := &governedSteerRecorder{steerRecorder: newSteerRecorder(), dispatch: ports.ChatSteerDispatch{
				Acceptance: ports.ChatTurnAcknowledged, Ref: ports.ChatTurnRef{ProviderTurnID: "provider-turn-1"},
				TransportRequestID: 2, TransportSHA256: "steer-sha", TransportBytes: 17, TransportSequence: 2,
			}}
			h, _ := governedSteerHarness(t, conv, nil)
			start := make(chan struct{})
			errs := make(chan error, len(tc.messages))
			var wg sync.WaitGroup
			for _, message := range tc.messages {
				message := message
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					_, err := h.svc.Steer(context.Background(), testSession, message)
					errs <- err
				}()
			}
			close(start)
			wg.Wait()
			close(errs)
			errorCount := 0
			for err := range errs {
				if err != nil {
					errorCount++
					if !errors.Is(err, domain.ErrGovernedCommandIdempotencyConflict) {
						t.Errorf("unexpected error: %v", err)
					}
				}
			}
			if errorCount != tc.wantErrors || len(conv.steers()) != 1 {
				t.Fatalf("errors=%d want=%d provider dispatches=%d", errorCount, tc.wantErrors, len(conv.steers()))
			}
		})
	}
}

func TestGovernedSteerStaleGenerationCannotReachProvider(t *testing.T) {
	conv := &governedSteerRecorder{steerRecorder: newSteerRecorder(), dispatch: ports.ChatSteerDispatch{
		Acceptance: ports.ChatTurnAcknowledged, Ref: ports.ChatTurnRef{ProviderTurnID: "provider-turn-1"},
		TransportRequestID: 2, TransportSHA256: "steer-sha", TransportBytes: 17, TransportSequence: 2,
	}}
	h, _ := governedSteerHarness(t, conv, nil)
	if err := h.st.ClaimChatControllerGeneration(context.Background(), testSession, "replacement-generation", "", "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.ctrl.Steer(context.Background(), ports.ChatUserMessage{Text: "stale guidance", ClientMessageID: "stale-steer"}); err == nil {
		t.Fatal("stale steer unexpectedly succeeded")
	}
	if got := len(conv.steers()); got != 0 {
		t.Fatalf("stale generation reached provider %d times", got)
	}
	claims, err := h.st.ListUnsettledGovernedControlCommands(context.Background())
	if err != nil || len(claims) != 1 || claims[0].State != domain.GovernedCommandClaimed || claims[0].Class != domain.GovernedControlSteer {
		t.Fatalf("claims=%+v err=%v", claims, err)
	}
}
