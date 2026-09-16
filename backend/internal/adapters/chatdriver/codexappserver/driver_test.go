package codexappserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

type fakePlugin struct {
	bin        string
	binErr     error
	authStatus ports.AgentAuthStatus
	authErr    error
}

func (f fakePlugin) ResolveBinary(context.Context) (string, error) { return f.bin, f.binErr }
func (f fakePlugin) AuthStatus(context.Context) (ports.AgentAuthStatus, error) {
	return f.authStatus, f.authErr
}

// scriptedServer answers client requests from a canned table and lets a test push
// notifications and server->client requests at the driver.
type scriptedServer struct {
	t        *testing.T
	toClient io.WriteCloser

	mu          sync.Mutex
	responses   map[string]string
	failures    map[string]string
	seen        []frame
	seenCh      chan frame
	responsesCh chan string
}

// replyError scripts a JSON-RPC error for a method, which is how a test exercises a
// provider refusal. app-server answers -32600 for everything it declines.
func (s *scriptedServer) replyError(method string, code int, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures[method] = `{"code":` + strconv.Itoa(code) + `,"message":` + strconv.Quote(message) + `}`
}

func (s *scriptedServer) respondTo(method, resultJSON string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses[method] = resultJSON
}

func (s *scriptedServer) push(raw string) {
	s.t.Helper()
	if err := s.tryPush(raw); err != nil {
		s.t.Fatalf("push: %v", err)
	}
}

// tryPush is used only when a test deliberately races a provider frame with
// client shutdown. A closed pipe is then one valid outcome of the boundary,
// rather than a test-harness failure that masks the client's result.
func (s *scriptedServer) tryPush(raw string) error {
	_, err := io.WriteString(s.toClient, raw+"\n")
	return err
}

// reply scripts the result for a method. Guarded because the server goroutine
// reads the same map while it is serving the connection.
func (s *scriptedServer) reply(method, result string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses[method] = result
}

// sentMethod reports whether the client ever sent a request for the method. Guarded
// because the server goroutine appends to the same slice.
func (s *scriptedServer) sentMethod(method string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.seen {
		if f.Method == method {
			return true
		}
	}
	return false
}

// awaitFrame waits for a frame matching pred among everything the client sent.
func (s *scriptedServer) awaitFrame(pred func(frame) bool) frame {
	s.t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		s.mu.Lock()
		for _, f := range s.seen {
			if pred(f) {
				s.mu.Unlock()
				return f
			}
		}
		s.mu.Unlock()
		select {
		case <-s.seenCh:
		case <-deadline:
			s.t.Fatal("timed out waiting for an expected client frame")
			return frame{}
		}
	}
}

// newTestDriver wires a Driver to a scripted server over in-memory pipes, so no
// process is ever spawned.
func newTestDriver(t *testing.T) (*Driver, *scriptedServer) {
	t.Helper()

	clientReads, serverWrites := io.Pipe()
	serverReads, clientWrites := io.Pipe()

	srv := &scriptedServer{
		t:        t,
		toClient: serverWrites,
		responses: map[string]string{
			"initialize":     `{"userAgent":"ao/test","codexHome":"/tmp/.codex"}`,
			"model/list":     `{"data":[{"id":"gpt-test","displayName":"GPT Test","isDefault":true}]}`,
			"thread/start":   `{"thread":{"id":"thread-1"},"model":"gpt-test","cwd":"/tmp/ws","approvalPolicy":"never","activePermissionProfile":{"id":":read-only"}}`,
			"turn/start":     `{"turn":{"id":"turn-1","status":"inProgress","items":[]}}`,
			"turn/interrupt": `{}`,
			"thread/resume":  `{"thread":{"id":"thread-1"}}`,
		},
		failures:    map[string]string{},
		seenCh:      make(chan frame, 64),
		responsesCh: make(chan string, 64),
	}

	go func() {
		br := bufio.NewReader(serverReads)
		for {
			line, err := readFrame(br)
			if err != nil {
				return
			}
			if len(line) == 0 {
				continue
			}
			var f frame
			if err := json.Unmarshal(line, &f); err != nil {
				continue
			}

			srv.mu.Lock()
			srv.seen = append(srv.seen, f)
			reply, known := srv.responses[f.Method]
			failure, refused := srv.failures[f.Method]
			srv.mu.Unlock()

			select {
			case srv.seenCh <- f:
			default:
			}

			switch {
			case f.ID == nil || f.Method == "":
			case refused:
				srv.push(`{"id":` + string(*f.ID) + `,"error":` + failure + `}`)
			case known:
				srv.push(`{"id":` + string(*f.ID) + `,"result":` + reply + `}`)
				select {
				case srv.responsesCh <- f.Method:
				default:
				}
			}
		}
	}()

	d := &Driver{
		plugin: fakePlugin{bin: "codex", authStatus: ports.AgentAuthStatusAuthorized},
		log:    slog.New(slog.DiscardHandler),
		versionProbe: func(context.Context, string) (string, error) {
			return "codex-cli 0.153.4", nil
		},
		surfaceProbe: func(context.Context, string) (protocolSurface, error) {
			return fullTestSurface(), nil
		},
		spawn: func(context.Context, string, string, []string) (*process, error) {
			return &process{
				stdin:  clientWrites,
				stdout: clientReads,
				stop:   func() error { return serverWrites.Close() },
			}, nil
		},
	}
	t.Cleanup(func() { _ = serverWrites.Close() })
	return d, srv
}

func (s *scriptedServer) awaitResponse(method string) {
	s.t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case got := <-s.responsesCh:
			if got == method {
				return
			}
		case <-deadline:
			s.t.Fatalf("timed out waiting for a response to %s", method)
			return
		}
	}
}

// nextEvent returns the next event of interest, skipping ones the test does not
// assert on.
func nextEvent(t *testing.T, events <-chan ports.ChatEvent, want ports.ChatEventKind) ports.ChatEvent {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("event stream closed while waiting for %q", want)
			}
			if ev.Kind == want {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out waiting for event %q", want)
			return ports.ChatEvent{}
		}
	}
}

func TestStartCompletesHandshakeAndOpensThread(t *testing.T) {
	d, srv := newTestDriver(t)

	conv, err := d.Start(context.Background(), ports.ChatStartConfig{
		SessionID:     "kennel-1",
		WorkspacePath: "/tmp/ws",
		Permissions:   ports.PermissionModeDefault,
		SystemPrompt:  "standing rules",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = conv.Close() }()

	if got := conv.ProviderConversationID(); got != "thread-1" {
		t.Fatalf("provider conversation id = %q, want thread-1", got)
	}

	// initialized must be notified, or the provider never leaves handshake.
	srv.awaitFrame(func(f frame) bool { return f.Method == "initialized" && f.ID == nil })

	start := srv.awaitFrame(func(f frame) bool { return f.Method == "thread/start" })
	var params struct {
		Cwd                   string `json:"cwd"`
		ApprovalPolicy        string `json:"approvalPolicy"`
		Sandbox               string `json:"sandbox"`
		DeveloperInstructions string `json:"developerInstructions"`
	}
	if err := json.Unmarshal(start.Params, &params); err != nil {
		t.Fatalf("thread/start params: %v", err)
	}
	if params.Cwd != "/tmp/ws" {
		t.Errorf("cwd = %q", params.Cwd)
	}
	if params.DeveloperInstructions != "standing rules" {
		t.Errorf("developerInstructions = %q", params.DeveloperInstructions)
	}
	// Default permissions must match what Kennel already gives a Codex TUI session.
	if params.ApprovalPolicy != "never" || params.Sandbox != "danger-full-access" {
		t.Errorf("default posture = %q/%q, want never/danger-full-access", params.ApprovalPolicy, params.Sandbox)
	}
}

// A relative cwd would put the agent in a directory relative to app-server's own
// process, silently editing the wrong tree.
func TestStartRejectsRelativeWorkspacePath(t *testing.T) {
	d, _ := newTestDriver(t)
	_, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "workspace"})
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("err = %v, want a rejection naming the absolute-path requirement", err)
	}
}

func TestSendTurnCarriesIdempotencyKey(t *testing.T) {
	d, srv := newTestDriver(t)
	conv, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = conv.Close() }()

	ref, err := conv.SendTurn(context.Background(), ports.ChatUserMessage{
		Text:            "what changed?",
		ClientMessageID: "client-msg-7",
		Origin:          domain.MessageOriginHuman,
	})
	if err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if ref.ProviderTurnID != "turn-1" {
		t.Fatalf("turn id = %q", ref.ProviderTurnID)
	}

	sent := srv.awaitFrame(func(f frame) bool { return f.Method == "turn/start" })
	var params struct {
		ThreadID            string `json:"threadId"`
		ClientUserMessageID string `json:"clientUserMessageId"`
		Input               []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"input"`
	}
	if err := json.Unmarshal(sent.Params, &params); err != nil {
		t.Fatalf("turn/start params: %v", err)
	}
	if params.ThreadID != "thread-1" {
		t.Errorf("threadId = %q", params.ThreadID)
	}
	if params.ClientUserMessageID != "client-msg-7" {
		t.Errorf("clientUserMessageId = %q, want the caller's key", params.ClientUserMessageID)
	}
	if len(params.Input) != 1 || params.Input[0].Text != "what changed?" {
		t.Errorf("input = %+v", params.Input)
	}
}

// An empty send is a caller bug, not a way to nudge the agent: there is no
// keystroke concept in Chat mode.
func TestSendTurnRejectsEmptyText(t *testing.T) {
	d, _ := newTestDriver(t)
	conv, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = conv.Close() }()

	if _, err := conv.SendTurn(context.Background(), ports.ChatUserMessage{Text: "  "}); err == nil {
		t.Fatal("expected empty text to be rejected")
	}
}

// The whole approval design in one test: the provider blocks on a server->client
// request, Kennel surfaces it with the provider's own decision list, and the user's
// choice is what unblocks the turn.
func TestApprovalIsParkedUntilResolved(t *testing.T) {
	d, srv := newTestDriver(t)
	conv, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = conv.Close() }()

	// Shaped after a real captured approval: no requestId field, so the JSON-RPC
	// id is the only correlation key, and decline is not on offer.
	srv.push(`{"id":0,"method":"item/commandExecution/requestApproval","params":{` +
		`"threadId":"thread-1","turnId":"turn-1","itemId":"exec-1",` +
		`"command":"/bin/zsh -lc 'date -u'","cwd":"/tmp/ws",` +
		`"availableDecisions":["accept",{"acceptWithExecpolicyAmendment":{"execpolicy_amendment":["date","-u"]}},"cancel"]}}`)

	ev := nextEvent(t, conv.Events(), ports.ChatEventApprovalRequested)
	if ev.RequestID != "0" {
		t.Fatalf("request id = %q, want the JSON-RPC id 0", ev.RequestID)
	}
	if ev.ActivityStatus != domain.ActivityStatusPending {
		t.Errorf("status = %q, want pending", ev.ActivityStatus)
	}
	if ev.Summary != "Run date -u" {
		t.Errorf("summary = %q, want the shell wrapper stripped", ev.Summary)
	}

	var ids []string
	for _, opt := range ev.Decisions {
		ids = append(ids, opt.ID)
	}
	want := []string{"accept", "acceptWithExecpolicyAmendment", "cancel"}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("decisions = %v, want %v (from the provider's own list)", ids, want)
	}

	// Nothing has been answered yet, so the provider is still blocked.
	srv.mu.Lock()
	for _, f := range srv.seen {
		if f.ID != nil && string(*f.ID) == "0" {
			srv.mu.Unlock()
			t.Fatal("approval was answered before the user decided")
		}
	}
	srv.mu.Unlock()

	if err := conv.ResolveRequest(context.Background(), "0", ports.ChatDecision{ID: "accept"}); err != nil {
		t.Fatalf("ResolveRequest: %v", err)
	}

	reply := srv.awaitFrame(func(f frame) bool { return f.ID != nil && string(*f.ID) == "0" && f.Method == "" })
	var payload struct {
		Decision string `json:"decision"`
	}
	if err := json.Unmarshal(reply.Result, &payload); err != nil {
		t.Fatalf("reply not decodable: %v (%s)", err, reply.Result)
	}
	if payload.Decision != "accept" {
		t.Fatalf("decision sent = %q", payload.Decision)
	}
}

// A structured decision must round-trip exactly, or the provider rejects it.
// A structured decision must round-trip with the parameters the provider attached
// to it. The client sends an id and nothing else — it has no way to reconstruct
// an execpolicy amendment — so Kennel answers with the provider's own payload for the
// option that was offered.
func TestStructuredDecisionIsAnsweredWithTheProvidersOwnPayload(t *testing.T) {
	d, srv := newTestDriver(t)
	conv, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = conv.Close() }()

	// The real captured shape: a mix of plain and object-shaped decisions.
	srv.push(`{"id":3,"method":"item/commandExecution/requestApproval","params":{"command":"ls","availableDecisions":["accept",{"acceptWithExecpolicyAmendment":{"execpolicy_amendment":["ls"]}},"cancel"]}}`)
	ev := nextEvent(t, conv.Events(), ports.ChatEventApprovalRequested)

	if err := conv.ResolveRequest(context.Background(), ev.RequestID, ports.ChatDecision{
		ID: "acceptWithExecpolicyAmendment",
	}); err != nil {
		t.Fatalf("ResolveRequest: %v", err)
	}

	reply := srv.awaitFrame(func(f frame) bool { return f.ID != nil && string(*f.ID) == "3" && f.Method == "" })
	if !strings.Contains(string(reply.Result), "execpolicy_amendment") {
		t.Fatalf("the provider's own decision payload was not echoed: %s", reply.Result)
	}
}

// A decision the provider never offered is consent Kennel would be inventing. It must
// be refused, and — just as important — the request must stay pending so the
// user's real answer still has something to answer.
func TestDecisionNotOfferedIsRefusedAndLeavesTheRequestPending(t *testing.T) {
	d, srv := newTestDriver(t)
	conv, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = conv.Close() }()

	// Note there is no decline on offer, which is a real captured case.
	srv.push(`{"id":4,"method":"item/commandExecution/requestApproval","params":{"command":"rm -rf /","availableDecisions":["accept","cancel"]}}`)
	ev := nextEvent(t, conv.Events(), ports.ChatEventApprovalRequested)

	err = conv.ResolveRequest(context.Background(), ev.RequestID, ports.ChatDecision{ID: "decline"})
	if !errors.Is(err, ports.ErrChatDecisionNotOffered) {
		t.Fatalf("err = %v, want ErrChatDecisionNotOffered", err)
	}

	// The request survived the bad answer, so the offered decision still works.
	if err := conv.ResolveRequest(context.Background(), ev.RequestID, ports.ChatDecision{ID: "cancel"}); err != nil {
		t.Fatalf("a refused decision consumed the request: %v", err)
	}
	reply := srv.awaitFrame(func(f frame) bool { return f.ID != nil && string(*f.ID) == "4" && f.Method == "" })
	if !strings.Contains(string(reply.Result), "cancel") {
		t.Fatalf("reply did not carry the offered decision: %s", reply.Result)
	}
}

// Answering a request that is no longer waiting is ordinary — two clients can
// watch the same approval — so it comes back typed rather than as a raw failure.
func TestResolvingAnUnknownRequestIsTyped(t *testing.T) {
	d, _ := newTestDriver(t)
	conv, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = conv.Close() }()

	err = conv.ResolveRequest(context.Background(), "no-such-request", ports.ChatDecision{ID: "accept"})
	if !errors.Is(err, ports.ErrChatRequestNotPending) {
		t.Fatalf("err = %v, want ErrChatRequestNotPending", err)
	}
}

// A card the user clicks after the request is gone must fail, never resolve
// something newer.
func TestResolveUnknownRequestIsRefused(t *testing.T) {
	d, _ := newTestDriver(t)
	conv, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = conv.Close() }()

	err = conv.ResolveRequest(context.Background(), "999", ports.ChatDecision{ID: "accept"})
	if err == nil {
		t.Fatal("expected resolving an unknown request to fail")
	}
}

// Answering a request Kennel does not model could consent to something on the user's
// behalf, so it must be refused with an error instead.
func TestUnmodelledServerRequestIsRefused(t *testing.T) {
	d, srv := newTestDriver(t)
	conv, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = conv.Close() }()

	srv.push(`{"id":11,"method":"mcpServer/elicitation/request","params":{}}`)

	reply := srv.awaitFrame(func(f frame) bool { return f.ID != nil && string(*f.ID) == "11" && f.Method == "" })
	if reply.Error == nil {
		t.Fatalf("unmodelled request was answered with a result: %s", reply.Result)
	}
}

func TestNotificationsBecomeNeutralEvents(t *testing.T) {
	d, srv := newTestDriver(t)
	conv, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = conv.Close() }()

	srv.push(`{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"inProgress","items":[]}}}`)
	srv.push(`{"method":"item/agentMessage/delta","params":{"turnId":"turn-1","itemId":"m1","delta":"hello"}}`)
	srv.push(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[]}}}`)

	if ev := nextEvent(t, conv.Events(), ports.ChatEventMessageDelta); ev.Delta != "hello" {
		t.Fatalf("delta = %q", ev.Delta)
	}
	if ev := nextEvent(t, conv.Events(), ports.ChatEventTurnCompleted); ev.TurnState != domain.TurnStateCompleted {
		t.Fatalf("turn state = %q", ev.TurnState)
	}
}

// app-server multiplexes child-agent thread notifications over the root
// connection. The adapter's fallback target must remain the root turn; otherwise
// an interrupt without an explicit id can stop a child and leave the requested
// root work running.
func TestNestedThreadDoesNotReplaceRootActiveTurn(t *testing.T) {
	d, srv := newTestDriver(t)
	conv, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = conv.Close() }()

	srv.push(`{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"root-turn","status":"inProgress","items":[]}}}`)
	srv.push(`{"method":"turn/started","params":{"threadId":"child-thread-1","turn":{"id":"child-turn","status":"inProgress","items":[]}}}`)
	root := nextEvent(t, conv.Events(), ports.ChatEventTurnStarted)
	child := nextEvent(t, conv.Events(), ports.ChatEventTurnStarted)
	if root.ProviderConversationID != "thread-1" || child.ProviderConversationID != "child-thread-1" {
		t.Fatalf("normalized lifecycle threads = root %q child %q", root.ProviderConversationID, child.ProviderConversationID)
	}

	if err := conv.Interrupt(context.Background(), ""); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	sent := srv.awaitFrame(func(f frame) bool { return f.Method == "turn/interrupt" })
	var params struct {
		ThreadID string `json:"threadId"`
		TurnID   string `json:"turnId"`
	}
	if err := json.Unmarshal(sent.Params, &params); err != nil {
		t.Fatalf("decode turn/interrupt params: %v", err)
	}
	if params.ThreadID != "thread-1" || params.TurnID != "root-turn" {
		t.Fatalf("interrupt target = %q/%q, want thread-1/root-turn", params.ThreadID, params.TurnID)
	}
}

// Resume must not quietly become a fresh thread: that would present unrelated
// history as continuous.
func TestResumeFailureDoesNotFallBackToStart(t *testing.T) {
	d, srv := newTestDriver(t)
	srv.mu.Lock()
	delete(srv.responses, "thread/resume")
	srv.mu.Unlock()

	// Answer thread/resume with an error instead.
	go func() {
		f := srv.awaitFrame(func(f frame) bool { return f.Method == "thread/resume" })
		srv.push(`{"id":` + string(*f.ID) + `,"error":{"code":-32602,"message":"unknown thread"}}`)
	}()

	_, err := d.Resume(context.Background(), ports.ChatResumeConfig{
		SessionID:              "kennel-1",
		ProviderConversationID: "thread-gone",
		WorkspacePath:          "/tmp/ws",
	})
	if !errors.Is(err, ports.ErrChatResumeFailed) {
		t.Fatalf("err = %v, want ErrChatResumeFailed", err)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	for _, f := range srv.seen {
		if f.Method == "thread/start" {
			t.Fatal("driver fell back to thread/start after a failed resume")
		}
	}
}

func TestResumeReappliesWorkspaceAndStandingInstructions(t *testing.T) {
	d, srv := newTestDriver(t)
	conv, err := d.Resume(context.Background(), ports.ChatResumeConfig{
		SessionID:              "kennel-1",
		ProviderConversationID: "thread-1",
		WorkspacePath:          "/tmp/ws",
		SystemPrompt:           "current Kennel standing instructions",
	})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	defer func() { _ = conv.Close() }()

	resume := srv.awaitFrame(func(f frame) bool { return f.Method == "thread/resume" })
	var params struct {
		ThreadID              string `json:"threadId"`
		Cwd                   string `json:"cwd"`
		DeveloperInstructions string `json:"developerInstructions"`
	}
	if err := json.Unmarshal(resume.Params, &params); err != nil {
		t.Fatalf("thread/resume params: %v", err)
	}
	if params.ThreadID != "thread-1" || params.Cwd != "/tmp/ws" {
		t.Fatalf("thread resume identity = %#v", params)
	}
	if params.DeveloperInstructions != "current Kennel standing instructions" {
		t.Fatalf("developerInstructions = %q", params.DeveloperInstructions)
	}
}

func TestResumeRefusesGovernedPolicyWithoutPrivateToolInjection(t *testing.T) {
	d, _ := newTestDriver(t)
	policy := domain.AttemptExecutionPolicy{
		OutcomeID: "out-1", PlanRevisionID: "plan-1", WorkUnitID: "wu-1", ContractRevisionNumber: 1,
		RunBriefCoreDigest: "brief", RequiredCapabilities: []string{domain.CapabilityWorktreeRead},
		Grants: []domain.CapabilityGrant{{ID: "read", Name: domain.CapabilityWorktreeRead, Scope: "worktree/*"}},
	}
	_, err := d.Resume(context.Background(), ports.ChatResumeConfig{
		SessionID: "kennel-1", ProviderConversationID: "thread-1", WorkspacePath: "/tmp/ws",
		Model: "approved-model", Permissions: ports.PermissionModeBypassPermissions, ExecutionPolicy: &policy,
	})
	if err == nil || !errors.Is(err, ports.ErrExecutionPolicyUnsupported) {
		t.Fatalf("Resume error = %v, want governed App Server refusal", err)
	}
}

func TestResumeRequiresStoredThreadID(t *testing.T) {
	d, _ := newTestDriver(t)
	_, err := d.Resume(context.Background(), ports.ChatResumeConfig{WorkspacePath: "/tmp/ws"})
	if !errors.Is(err, ports.ErrChatResumeFailed) {
		t.Fatalf("err = %v, want ErrChatResumeFailed", err)
	}
}

func TestProbeReportsAuthRequired(t *testing.T) {
	d := &Driver{
		plugin: fakePlugin{bin: "codex", authStatus: ports.AgentAuthStatusUnauthorized},
		log:    slog.New(slog.DiscardHandler),
	}
	if _, err := d.Probe(context.Background()); !errors.Is(err, ports.ErrChatAuthRequired) {
		t.Fatalf("err = %v, want ErrChatAuthRequired", err)
	}
}

// An inconclusive auth probe is not proof of failure, matching how Kennel already
// treats runtime probes.
func TestProbeTreatsUnknownAuthAsUsable(t *testing.T) {
	d, _ := newTestDriver(t)
	d.plugin = fakePlugin{bin: "codex", authStatus: ports.AgentAuthStatusUnknown, authErr: errors.New("probe timed out")}
	caps, err := d.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if missing := ports.MissingProductionCapabilities(caps); len(missing) != 0 {
		t.Fatalf("codex is missing production capabilities: %v", missing)
	}
}

func TestProbeRejectsIncompatibleProtocolBeforeCreation(t *testing.T) {
	d, srv := newTestDriver(t)
	srv.mu.Lock()
	delete(srv.responses, "model/list")
	srv.failures["model/list"] = `{"code":-32601,"message":"method not found"}`
	srv.mu.Unlock()

	if _, err := d.Probe(context.Background()); !errors.Is(err, ports.ErrChatDriverIncompatible) {
		t.Fatalf("err = %v, want ErrChatDriverIncompatible", err)
	}
}

func TestProbeRejectsCodexOlderThanTheTestedProtocolFloor(t *testing.T) {
	d, _ := newTestDriver(t)
	d.versionProbe = func(context.Context, string) (string, error) {
		return "codex-cli 0.145.9", nil
	}

	if _, err := d.Probe(context.Background()); !errors.Is(err, ports.ErrChatDriverIncompatible) {
		t.Fatalf("err = %v, want ErrChatDriverIncompatible", err)
	}
}

func TestProbeAcceptsNewerCodexVersion(t *testing.T) {
	d, _ := newTestDriver(t)
	d.versionProbe = func(context.Context, string) (string, error) {
		return "codex-cli 1.2.3", nil
	}

	if _, err := d.Probe(context.Background()); err != nil {
		t.Fatalf("Probe: %v", err)
	}
}

func TestProbeRejectsUnparseableCodexVersion(t *testing.T) {
	d, _ := newTestDriver(t)
	d.versionProbe = func(context.Context, string) (string, error) {
		return "codex development build", nil
	}

	if _, err := d.Probe(context.Background()); !errors.Is(err, ports.ErrChatDriverIncompatible) {
		t.Fatalf("err = %v, want ErrChatDriverIncompatible", err)
	}
}

func TestProbeReportsMissingBinary(t *testing.T) {
	d := &Driver{
		plugin: fakePlugin{binErr: errors.New("codex not found on PATH")},
		log:    slog.New(slog.DiscardHandler),
	}
	if _, err := d.Probe(context.Background()); !errors.Is(err, ports.ErrChatDriverUnavailable) {
		t.Fatalf("err = %v, want ErrChatDriverUnavailable", err)
	}
}

// Chat must not be quietly stricter than the terminal path for the same setting.
func TestApprovalSettingsMirrorTUIPosture(t *testing.T) {
	for _, tc := range []struct {
		mode            ports.PermissionMode
		policy, sandbox string
	}{
		{ports.PermissionModeDefault, "never", "danger-full-access"},
		{ports.PermissionModeBypassPermissions, "never", "danger-full-access"},
		{ports.PermissionModeAcceptEdits, "on-request", "workspace-write"},
		{ports.PermissionModeAuto, "on-request", "workspace-write"},
		{ports.PermissionMode("nonsense"), "never", "danger-full-access"},
	} {
		policy, sandbox := approvalSettings(tc.mode)
		if policy != tc.policy || sandbox != tc.sandbox {
			t.Errorf("approvalSettings(%q) = %q/%q, want %q/%q", tc.mode, policy, sandbox, tc.policy, tc.sandbox)
		}
	}
}

func TestStartRefusesGovernedPolicyWithoutPrivateToolInjection(t *testing.T) {
	d, _ := newTestDriver(t)
	policy := domain.AttemptExecutionPolicy{
		OutcomeID: "out-1", PlanRevisionID: "plan-1", WorkUnitID: "wu-1", ContractRevisionNumber: 1,
		RunBriefCoreDigest:   "brief",
		RequiredCapabilities: []string{domain.CapabilityWorktreeRead},
		Grants:               []domain.CapabilityGrant{{ID: "read", Name: domain.CapabilityWorktreeRead, Scope: "worktree/*"}},
	}
	_, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws", Permissions: ports.PermissionModeBypassPermissions, ExecutionPolicy: &policy})
	if err == nil || !errors.Is(err, ports.ErrExecutionPolicyUnsupported) {
		t.Fatalf("Start error = %v, want governed App Server refusal", err)
	}
}

func TestValidateExecutionPolicyRejectsRestrictedScope(t *testing.T) {
	policy := domain.AttemptExecutionPolicy{
		OutcomeID: "out-1", PlanRevisionID: "plan-1", WorkUnitID: "wu-1", ContractRevisionNumber: 1,
		RunBriefCoreDigest: "brief", RequiredCapabilities: []string{domain.CapabilityWorktreeRead},
		Grants: []domain.CapabilityGrant{{ID: "read", Name: domain.CapabilityWorktreeRead, Scope: "worktree/docs/*"}},
	}
	if err := (&Driver{}).ValidateExecutionPolicy(context.Background(), policy); err == nil {
		t.Fatal("restricted worktree scope was accepted")
	} else if !errors.Is(err, ports.ErrExecutionPolicyUnsupported) {
		t.Fatalf("err = %v, want typed unsupported policy", err)
	}
}

func TestValidateExecutionPolicyRejectsUnknownCapability(t *testing.T) {
	policy := domain.AttemptExecutionPolicy{
		OutcomeID: "out-1", PlanRevisionID: "plan-1", WorkUnitID: "wu-1", ContractRevisionNumber: 1,
		RunBriefCoreDigest:   "brief",
		RequiredCapabilities: []string{"provider.unknown", domain.CapabilityWorktreeRead},
		Grants: []domain.CapabilityGrant{
			{ID: "unknown", Name: "provider.unknown", Scope: "worktree/*"},
			{ID: "read", Name: domain.CapabilityWorktreeRead, Scope: "worktree/*"},
		},
	}
	if err := (&Driver{}).ValidateExecutionPolicy(context.Background(), policy); err == nil {
		t.Fatal("unknown capability was accepted alongside worktree.read")
	} else if !errors.Is(err, ports.ErrExecutionPolicyUnsupported) {
		t.Fatalf("err = %v, want typed unsupported policy", err)
	}
}

func TestStartRefusesGovernedWritePolicyWithoutPrivateToolInjection(t *testing.T) {
	d, _ := newTestDriver(t)
	policy := domain.AttemptExecutionPolicy{
		OutcomeID: "out-1", PlanRevisionID: "plan-1", WorkUnitID: "wu-1", ContractRevisionNumber: 1,
		RunBriefCoreDigest: "brief",
		RequiredCapabilities: []string{
			domain.CapabilityWorktreeExec, domain.CapabilityWorktreeRead, domain.CapabilityWorktreeWrite,
		},
		Grants: []domain.CapabilityGrant{
			{ID: "exec", Name: domain.CapabilityWorktreeExec, Scope: "worktree/*"},
			{ID: "read", Name: domain.CapabilityWorktreeRead, Scope: "worktree/*"},
			{ID: "write", Name: domain.CapabilityWorktreeWrite, Scope: "worktree/*"},
		},
		ApprovedChecks: []domain.ApprovedCheck{{ID: "check-1", CriterionID: "criterion-1", Argv: []string{"go", "test", "./..."}, TimeoutSeconds: 60}},
	}
	_, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws", ExecutionPolicy: &policy})
	if err == nil || !errors.Is(err, ports.ErrExecutionPolicyUnsupported) {
		t.Fatalf("Start error = %v, want governed App Server refusal", err)
	}
}

func TestEnvSliceIsSortedForReproducibleRelaunch(t *testing.T) {
	// Sortedness is still the contract: a relaunch should be byte-identical so a
	// process diff is readable. What changed is that the overlay is merged over the
	// daemon's environment rather than replacing it.
	//
	// This test used to assert envSlice(nil) == nil, "so exec inherits the parent
	// env". The intent was right and the mechanism never worked: the slice is only
	// nil when the overlay is empty, which it never is in practice, so the provider
	// was always launched with a replaced environment.
	got := envSlice(map[string]string{"ZZ_LAST": "z", "AA_FIRST": "a"})
	var previous string
	for _, entry := range got {
		if previous != "" && entry < previous {
			t.Fatalf("env not sorted: %q came after %q", entry, previous)
		}
		previous = entry
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{"AA_FIRST=a", "ZZ_LAST=z"} {
		if !strings.Contains(joined, want) {
			t.Errorf("overlay entry %q missing from %v", want, got)
		}
	}
}

// When the process dies, the stream must say so rather than just going quiet.
func TestControllerStopIsAnnounced(t *testing.T) {
	d, srv := newTestDriver(t)
	conv, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	_ = srv.toClient.Close()

	ev := nextEvent(t, conv.Events(), ports.ChatEventControllerState)
	if ev.ControllerState != ports.ChatControllerStopped {
		t.Fatalf("controller state = %q, want stopped", ev.ControllerState)
	}
	_ = conv.Close()
}

// The per-turn shapes are not the same as the thread-level ones: a thread takes
// `sandbox: "workspace-write"`, a turn takes a tagged
// `sandboxPolicy: {type:"workspaceWrite"}`. Sending a thread's shape to a turn is
// rejected as a missing `type`, so this pins the difference.
func TestTurnSettingsUseTheTurnLevelWireShapes(t *testing.T) {
	d, srv := newTestDriver(t)
	conv, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = conv.Close() }()

	if _, err := conv.SendTurn(context.Background(), ports.ChatUserMessage{
		Text: "go",
		Settings: ports.ChatTurnSettings{
			Model:    "gpt-5.6-terra",
			Effort:   "high",
			Approval: ports.PermissionModeAcceptEdits,
		},
	}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	sent := srv.awaitFrame(func(f frame) bool { return f.Method == "turn/start" })
	var params struct {
		Model          string `json:"model"`
		Effort         string `json:"effort"`
		ApprovalPolicy string `json:"approvalPolicy"`
		SandboxPolicy  struct {
			Type string `json:"type"`
		} `json:"sandboxPolicy"`
	}
	if err := json.Unmarshal(sent.Params, &params); err != nil {
		t.Fatalf("decode turn/start params: %v: %s", err, sent.Params)
	}
	if params.Model != "gpt-5.6-terra" || params.Effort != "high" {
		t.Errorf("model/effort not forwarded: %+v", params)
	}
	if params.ApprovalPolicy != "on-request" {
		t.Errorf("approvalPolicy = %q, want on-request", params.ApprovalPolicy)
	}
	if params.SandboxPolicy.Type != "workspaceWrite" {
		t.Errorf("sandboxPolicy.type = %q; a turn needs the tagged shape", params.SandboxPolicy.Type)
	}
}

// A caller that chooses nothing must produce exactly the payload it did before
// per-turn settings existed: an empty field is not a value the provider has to
// interpret.
func TestNoTurnSettingsSendsNoSettingsFields(t *testing.T) {
	d, srv := newTestDriver(t)
	conv, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = conv.Close() }()

	if _, err := conv.SendTurn(context.Background(), ports.ChatUserMessage{Text: "go"}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	sent := srv.awaitFrame(func(f frame) bool { return f.Method == "turn/start" })
	var params map[string]json.RawMessage
	if err := json.Unmarshal(sent.Params, &params); err != nil {
		t.Fatalf("decode turn/start params: %v", err)
	}
	for _, key := range []string{"model", "effort", "approvalPolicy", "sandboxPolicy"} {
		if _, present := params[key]; present {
			t.Errorf("unset setting %q was sent anyway", key)
		}
	}
}

// The catalog is the provider's. Hidden models are dropped because offering one
// would fail, while the opened thread's effort is more specific than the generic
// model default returned by model/list.
func TestListModelsKeepsCatalogAndUsesThreadEffort(t *testing.T) {
	d, srv := newTestDriver(t)
	// Scripted before Start: the server goroutine reads this map, so writing it
	// afterwards would race the connection it is already serving.
	srv.reply("thread/start", `{"thread":{"id":"thread-1"},"model":"a","reasoningEffort":"xhigh","cwd":"/tmp/ws"}`)
	srv.reply("model/list", `{"data":[{"id":"a","displayName":"Model A","description":"first","isDefault":true,"hidden":false,"defaultReasoningEffort":"medium","supportedReasoningEfforts":[{"reasoningEffort":"low"},{"reasoningEffort":"xhigh"}]},{"id":"secret","displayName":"Hidden","isDefault":false,"hidden":true,"defaultReasoningEffort":"low","supportedReasoningEfforts":[]},{"id":"b","displayName":"Model B","isDefault":false,"hidden":false,"defaultReasoningEffort":"low","supportedReasoningEfforts":[]}]}`)

	conv, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = conv.Close() }()

	lister, ok := conv.(ports.ChatModelLister)
	if !ok {
		t.Fatal("conversation does not implement ChatModelLister")
	}

	models, err := lister.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models, want the 2 visible ones: %+v", len(models), models)
	}
	if models[0].ID != "a" || models[1].ID != "b" {
		t.Errorf("provider order not preserved: %+v", models)
	}
	if !models[0].Default {
		t.Error("the provider's default was not carried through")
	}
	if got := models[0].Efforts; len(got) != 2 || got[0] != "low" || got[1] != "xhigh" {
		t.Errorf("efforts = %v, want [low xhigh]", got)
	}
	if models[0].DefaultEffort != "xhigh" {
		t.Errorf("default effort = %q, want the thread's xhigh", models[0].DefaultEffort)
	}
}

// The on-demand quota read. The reply below is the verbatim account/rateLimits/read
// result from a live pro account, on ONE line: readFrame is line-delimited, so a
// pretty-printed reply hangs the test forever rather than failing it.
func TestReadRateLimitsFromProviderResult(t *testing.T) {
	d, srv := newTestDriver(t)
	srv.reply("account/rateLimits/read", `{"rateLimits":{"limitId":"codex","limitName":null,"primary":{"usedPercent":71,"windowDurationMins":10080,"resetsAt":4102444800},"secondary":null,"credits":{"hasCredits":false,"unlimited":false,"balance":"0"},"individualLimit":null,"spendControlReached":false,"planType":"pro","rateLimitReachedType":null},"rateLimitsByLimitId":{"codex_bengalfox":{"limitId":"codex_bengalfox","limitName":"GPT-5.3-Codex-Spark","primary":{"usedPercent":0,"windowDurationMins":10080,"resetsAt":4102444800},"secondary":null,"credits":null,"individualLimit":null,"spendControlReached":null,"planType":"pro","rateLimitReachedType":null}},"rateLimitResetCredits":{"availableCount":0,"credits":[]}}`)

	conv, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = conv.Close() }()

	reporter, ok := conv.(ports.ChatUsageReporter)
	if !ok {
		t.Fatal("conversation does not implement ChatUsageReporter")
	}

	limits, err := reporter.ReadRateLimits(context.Background())
	if err != nil {
		t.Fatalf("ReadRateLimits: %v", err)
	}
	if limits.PrimaryUsedPercent != 71 {
		t.Errorf("primary = %v, want 71", limits.PrimaryUsedPercent)
	}
	// The per-model rateLimitsByLimitId breakdown is deliberately not read: the
	// account-level window is what stops the next turn, and reading the finer table
	// would answer a question nobody asked with a much lower number.
	if limits.SecondaryUsedPercent >= 0 {
		t.Errorf("secondary = %v, want negative for a window this account lacks",
			limits.SecondaryUsedPercent)
	}
	if limits.PlanLabel != "pro" {
		t.Errorf("plan = %q, want pro", limits.PlanLabel)
	}
	// resetsAt is an absolute instant far in the future, so a positive remainder is
	// the only correct answer regardless of when the suite runs.
	if limits.PrimaryResetsInSeconds <= 0 {
		t.Errorf("primary resets in %d, want a positive remaining duration",
			limits.PrimaryResetsInSeconds)
	}
}

// The capability gates the readout, so it must be advertised or the UI hides a
// meter the driver can actually feed.
func TestCapabilitiesAdvertiseUsageAndRateLimits(t *testing.T) {
	caps := capabilities()
	if !caps.Has(ports.ChatCapabilityUsage) {
		t.Error("usage capability not advertised")
	}
	if !caps.Has(ports.ChatCapabilityRateLimits) {
		t.Error("rate limit capability not advertised")
	}
}

/* ---- compaction --------------------------------------------------------- */

// The wire shape, and the reason compaction exists: without it a long
// conversation eventually cannot accept another turn at all.
//
// thread/compact/start takes the thread id and nothing else. It is deliberately
// asserted here rather than assumed, because the sibling turn-level calls take a
// turn id and a tagged sandbox policy, and sending a turn's params to a thread
// method is rejected outright.
func TestCompactSendsOnlyTheThreadID(t *testing.T) {
	d, srv := newTestDriver(t)
	srv.reply("thread/compact/start", `{}`)

	conv, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = conv.Close() }()

	compactor, ok := conv.(ports.ChatCompactor)
	if !ok {
		t.Fatal("conversation does not implement ChatCompactor")
	}
	if _, err := compactor.Compact(context.Background()); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	sent := srv.awaitFrame(func(f frame) bool { return f.Method == "thread/compact/start" })
	var params map[string]any
	if err := json.Unmarshal(sent.Params, &params); err != nil {
		t.Fatalf("compact params: %v", err)
	}
	if params["threadId"] != "thread-1" {
		t.Errorf("threadId = %v, want thread-1", params["threadId"])
	}
	if len(params) != 1 {
		t.Errorf("compact sent %v; the method takes threadId alone", params)
	}
}

// The reclaim figures, end to end over the transport.
//
// The provider reports NO tokens on its own compaction event, so before/after are
// bracketed from the token-usage reports either side of the compaction turn. This
// replays the exact sequence a live app-server sent: a usage report with the old
// figure arrives AFTER compaction is requested and before the compaction turn
// starts, which is why "before" is snapshotted at turn start rather than at the
// moment Compact is called.
func TestCompactionReportsWhatItReclaimed(t *testing.T) {
	d, srv := newTestDriver(t)
	conv, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = conv.Close() }()

	srv.push(`{"method":"thread/tokenUsage/updated","params":{"threadId":"thread-1","turnId":"turn-1","tokenUsage":{"total":{"totalTokens":15650},"last":{"totalTokens":15650},"modelContextWindow":258400}}}`)
	srv.push(`{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"compact-turn","status":"inProgress","items":[]}}}`)
	srv.push(`{"method":"item/started","params":{"item":{"type":"contextCompaction","id":"cc-1"},"threadId":"thread-1","turnId":"compact-turn"}}`)
	srv.push(`{"method":"thread/tokenUsage/updated","params":{"threadId":"thread-1","turnId":"compact-turn","tokenUsage":{"total":{"totalTokens":15650},"last":{"totalTokens":4632},"modelContextWindow":258400}}}`)
	srv.push(`{"method":"item/completed","params":{"item":{"type":"contextCompaction","id":"cc-1"},"threadId":"thread-1","turnId":"compact-turn"}}`)

	ev := nextEvent(t, conv.Events(), ports.ChatEventCompacted)
	if ev.ProviderItemID != "cc-1" {
		t.Errorf("provider item id = %q, want cc-1", ev.ProviderItemID)
	}
	var detail struct {
		TokensBefore    int64 `json:"tokensBefore"`
		TokensAfter     int64 `json:"tokensAfter"`
		TokensReclaimed int64 `json:"tokensReclaimed"`
		ContextWindow   int64 `json:"contextWindow"`
	}
	if err := json.Unmarshal(ev.Detail, &detail); err != nil {
		t.Fatalf("compaction detail: %v", err)
	}
	if detail.TokensBefore != 15650 || detail.TokensAfter != 4632 {
		t.Errorf("bracket = %d -> %d, want 15650 -> 4632", detail.TokensBefore, detail.TokensAfter)
	}
	if detail.TokensReclaimed != 11018 {
		t.Errorf("reclaimed = %d, want 11018", detail.TokensReclaimed)
	}
	if detail.ContextWindow != 258400 {
		t.Errorf("context window = %d, want 258400", detail.ContextWindow)
	}
	if !strings.Contains(ev.Summary, "11.0k") {
		t.Errorf("summary = %q, want the reclaimed amount named", ev.Summary)
	}
}

// A provider build that emits both the deprecated notification and the item
// reports one compaction, not two. The turn id is the only key both carry.
func TestCompactionIsReportedOncePerTurn(t *testing.T) {
	d, srv := newTestDriver(t)
	conv, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = conv.Close() }()

	srv.push(`{"method":"item/completed","params":{"item":{"type":"contextCompaction","id":"cc-1"},"threadId":"thread-1","turnId":"compact-turn"}}`)
	srv.push(`{"method":"thread/compacted","params":{"threadId":"thread-1","turnId":"compact-turn"}}`)
	// A marker after both, so the test can prove nothing compaction-shaped came
	// between them without waiting on a timeout.
	srv.push(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"compact-turn","status":"completed","items":[]}}}`)

	nextEvent(t, conv.Events(), ports.ChatEventCompacted)
	for {
		ev, ok := <-conv.Events()
		if !ok {
			t.Fatal("stream closed before the marker arrived")
		}
		if ev.Kind == ports.ChatEventCompacted {
			t.Fatal("one compaction was reported twice")
		}
		if ev.Kind == ports.ChatEventTurnCompleted {
			return
		}
	}
}

// Kennel has no context figure until the provider reports one, so a compaction right
// after a restart genuinely does not know what it saved. Claiming "reclaimed 0
// tokens" would be a lie rather than a gap.
func TestCompactionClaimsNoFiguresItDoesNotHave(t *testing.T) {
	if got := compactionSummary(0, 0); got != "Compacted the conversation history" {
		t.Errorf("summary with no figures = %q", got)
	}
	// Context can also grow across a compaction on a nearly-empty thread: the
	// summary plus the system prompt outweighed what was there. Measured: an empty
	// thread compacted from nothing to 4702 tokens.
	if got := compactionSummary(1000, 4702); got != "Compacted the conversation history" {
		t.Errorf("summary with no reclaim = %q, want no invented saving", got)
	}
	if got := compactionSummary(15650, 4632); !strings.Contains(got, "11.0k tokens") {
		t.Errorf("summary = %q", got)
	}
}

// Compaction is advertised so the UI can offer the control at all. A driver that
// can compact but does not say so leaves the affordance hidden and the user with
// no way out of a full context.
func TestCompactionIsAdvertised(t *testing.T) {
	if !capabilities().Has(ports.ChatCapabilityCompaction) {
		t.Fatal("compaction capability is not advertised")
	}
}

// Kennel's session env is an overlay, not a whole environment. Replacing the process
// env with it launched the provider with no HOME, USER, TMPDIR or SSH_AUTH_SOCK --
// and every shell command the agent runs inherits that same env, so `git push` over
// SSH and every toolchain cache would fail. The provider survived it because its
// home lookup falls back to the passwd database, which is exactly why nobody
// noticed.
func TestEnvSliceMergesOverTheDaemonEnvironment(t *testing.T) {
	t.Setenv("HOME", "/Users/someone")
	t.Setenv("SSH_AUTH_SOCK", "/tmp/agent.sock")
	t.Setenv("KENNEL_SESSION", "stale-from-the-shell-that-started-the-daemon")

	got := map[string]string{}
	for _, entry := range envSlice(map[string]string{
		"KENNEL_SESSION": "p1-1",
		"PATH":           "/pinned/bin:/usr/bin",
	}) {
		key, value, _ := strings.Cut(entry, "=")
		got[key] = value
	}

	// Inherited, because the agent's shell needs them.
	if got["HOME"] != "/Users/someone" {
		t.Errorf("HOME = %q, want it inherited from the daemon", got["HOME"])
	}
	if got["SSH_AUTH_SOCK"] != "/tmp/agent.sock" {
		t.Errorf("SSH_AUTH_SOCK = %q; without it the agent cannot push over SSH", got["SSH_AUTH_SOCK"])
	}
	// Kennel's overlay wins: a session must not inherit a stale id.
	if got["KENNEL_SESSION"] != "p1-1" {
		t.Errorf("KENNEL_SESSION = %q, want the session's own id to win", got["KENNEL_SESSION"])
	}
	if got["PATH"] != "/pinned/bin:/usr/bin" {
		t.Errorf("PATH = %q, want the HookPATH-pinned value to win", got["PATH"])
	}
}

// An empty overlay still has to hand the provider a usable environment.
func TestEnvSliceWithNoOverlayStillInheritsTheEnvironment(t *testing.T) {
	t.Setenv("HOME", "/Users/someone")
	entries := envSlice(nil)
	var sawHome bool
	for _, entry := range entries {
		if entry == "HOME=/Users/someone" {
			sawHome = true
		}
	}
	if !sawHome {
		t.Error("an empty overlay produced an environment with no HOME")
	}
}

func TestRequestUserInputEmitsTypedQuestionsAndForwardsExactAnswers(t *testing.T) {
	d, srv := newTestDriver(t)
	conv, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = conv.Close() }()

	srv.push(`{"id":21,"method":"item/tool/requestUserInput","params":{"threadId":"thread-1","turnId":"turn-1","itemId":"item-questions","isBlocking":true,"questions":[{"id":"database","header":"Database","question":"Which database should the proof use?","isOther":true,"options":[{"label":"SQLite","description":"Local file"},{"label":"Postgres","description":"Network service"}]},{"id":"token","header":"Token","question":"Enter the temporary token","isSecret":true,"options":[]},{"id":"mode","header":"Mode","question":"Which run mode?","options":[{"label":"Fast"},{"label":"Thorough"}]}]}}`)
	ev := nextEvent(t, conv.Events(), ports.ChatEventInputRequested)
	if ev.RequestID != "21" || ev.ProviderItemID != "21" {
		t.Fatalf("request ids = %q/%q, want 21/21", ev.RequestID, ev.ProviderItemID)
	}
	if ev.Summary != "Which database should the proof use?" {
		t.Fatalf("summary = %q", ev.Summary)
	}
	if ev.Input == nil || ev.Input.Mode != ports.ChatInputModeForm {
		t.Fatalf("typed input = %#v", ev.Input)
	}
	properties, ok := ev.Input.Schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v", ev.Input.Schema["properties"])
	}
	database := properties["database"].(map[string]any)
	if _, constrained := database["enum"]; constrained {
		t.Fatal("isOther question was constrained to enum-only")
	}
	if got := database["examples"].([]string); strings.Join(got, ",") != "SQLite,Postgres" || database["x-kennel-allows-other"] != true {
		t.Fatalf("database other/options = %#v", database)
	}
	token := properties["token"].(map[string]any)
	if token["format"] != "password" {
		t.Fatalf("secret token schema = %#v", token)
	}
	var detail struct {
		Method    string `json:"method"`
		ItemID    string `json:"itemId"`
		Questions []struct {
			ID       string `json:"id"`
			Header   string `json:"header"`
			Question string `json:"question"`
			IsSecret *bool  `json:"isSecret"`
			IsOther  *bool  `json:"isOther"`
			Options  []struct {
				Label       string `json:"label"`
				Description string `json:"description"`
			} `json:"options"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(ev.Detail, &detail); err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Method != "item/tool/requestUserInput" || detail.ItemID != "item-questions" ||
		len(detail.Questions) != 3 || detail.Questions[0].ID != "database" ||
		detail.Questions[0].IsOther == nil || !*detail.Questions[0].IsOther ||
		detail.Questions[1].IsSecret == nil || !*detail.Questions[1].IsSecret ||
		detail.Questions[0].Options[0].Description != "Local file" {
		t.Fatalf("detail = %#v", detail)
	}

	raw := []byte(`{"answers":{"database":{"answers":["DuckDB"]},"token":{"answers":["ephemeral-test-value"]},"mode":{"answers":["Thorough"]}}}`)
	if err := conv.ResolveRequest(context.Background(), ev.RequestID, ports.ChatDecision{Raw: raw}); err != nil {
		t.Fatalf("ResolveRequest: %v", err)
	}
	reply := srv.awaitFrame(func(f frame) bool { return f.ID != nil && string(*f.ID) == "21" && f.Method == "" })
	if string(reply.Result) != string(raw) {
		t.Fatalf("provider answer = %s, want exact %s", reply.Result, raw)
	}
	for i := 0; i < 2; i++ {
		err := conv.ResolveRequest(context.Background(), ev.RequestID, ports.ChatDecision{Raw: raw})
		if !errors.Is(err, ports.ErrChatRequestNotPending) {
			t.Fatalf("repeat %d = %v, want ErrChatRequestNotPending", i+1, err)
		}
	}
}

func TestNativeSandboxProfileMapsOnlyExactStage1Boundary(t *testing.T) {
	profile := &ports.ChatNativeSandboxProfile{
		Sandbox: ports.ChatNativeSandboxWorkspaceWrite, NetworkAccess: false, WritableRoots: []string{},
		ExcludeSlashTmp: true, ExcludeTmpdirEnvVar: true,
	}
	got, err := nativeSandboxPolicy(profile)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"type": "workspaceWrite", "networkAccess": false, "writableRoots": []string{}, "excludeSlashTmp": true, "excludeTmpdirEnvVar": true}
	if !semanticJSONEqual(got, want) {
		t.Fatalf("policy = %#v, want %#v", got, want)
	}
	bad := *profile
	bad.NetworkAccess = true
	if _, err := nativeSandboxPolicy(&bad); !errors.Is(err, ports.ErrChatProfileMismatch) {
		t.Fatalf("network-enabled profile error = %v", err)
	}
	bad = *profile
	bad.WritableRoots = []string{"/tmp"}
	if _, err := nativeSandboxPolicy(&bad); !errors.Is(err, ports.ErrChatProfileMismatch) {
		t.Fatalf("extra-root profile error = %v", err)
	}
}

func TestNativeThreadSandboxAcknowledgmentMatchesOnlyRequestedEnum(t *testing.T) {
	// thread/start and thread/resume accept only the coarse sandbox enum. A
	// response can expose resolved detail from provider configuration, but it is
	// not an acknowledgment of the detailed policy sent on each turn.
	for _, observed := range []map[string]any{
		{"type": "workspaceWrite"},
		{"type": "workspaceWrite", "networkAccess": false, "writableRoots": []any{}},
		{"type": "workspaceWrite", "excludeSlashTmp": false, "excludeTmpdirEnvVar": false},
	} {
		if err := validateNativeThreadSandbox(observed); err != nil {
			t.Fatalf("coarse workspace-write acknowledgment %#v: %v", observed, err)
		}
	}
	if err := validateNativeThreadSandbox(nil); !errors.Is(err, ports.ErrChatProfileMismatch) {
		t.Fatalf("missing acknowledgment error = %v", err)
	}
	if err := validateNativeThreadSandbox(map[string]any{"type": "dangerFullAccess"}); !errors.Is(err, ports.ErrChatProfileMismatch) {
		t.Fatalf("widened acknowledgment error = %v", err)
	}
}

func TestNativeTurnWirePolicyIsSemanticAndReceiptDigestIsExact(t *testing.T) {
	params := map[string]any{"threadId": "thread-1", "input": []any{}, "sandboxPolicy": map[string]any{
		"type": "workspaceWrite", "networkAccess": false, "writableRoots": []string{}, "excludeSlashTmp": true, "excludeTmpdirEnvVar": true,
	}}
	digest, bytes, err := canonicalRequestFrameDigest(17, "turn/start", params)
	if err != nil || len(digest) != 64 || bytes <= 0 {
		t.Fatalf("digest = %q bytes=%d err=%v", digest, bytes, err)
	}
	payload := map[string]any{"id": int64(17), "method": "turn/start", "params": params}
	raw, _ := json.Marshal(payload)
	raw = append(raw, '\n')
	if err := validateSerializedTurnSandboxPolicy(raw, params["sandboxPolicy"].(map[string]any)); err != nil {
		t.Fatalf("semantic policy: %v", err)
	}
	widened := map[string]any{"type": "workspaceWrite", "networkAccess": true}
	if err := validateSerializedTurnSandboxPolicy(raw, widened); err == nil {
		t.Fatal("widened expected policy accepted")
	}
}

func TestParseCodeModeExecInputExactWrapper(t *testing.T) {
	input := `const r = await tools.exec_command({cmd:"mkdir -p '/var/tmp/x'",workdir:"/repo",yield_time_ms:10000}); text(JSON.stringify(r));`
	got, ok := parseCodeModeExecInput(input)
	if !ok || got.command != "mkdir -p '/var/tmp/x'" || got.cwd != "/repo" {
		t.Fatalf("got=%+v ok=%t", got, ok)
	}
	for _, bad := range []string{`tools.exec_command({cmd:"x"})`, `text(JSON.stringify(r))`, `const r = await tools.exec_command({cmd:foo}); text(JSON.stringify(r));`} {
		if _, ok := parseCodeModeExecInput(bad); ok {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestNormalizeRawExecPairsCallAndOutput(t *testing.T) {
	for _, tc := range []struct {
		name, call, output string
	}{
		{"code-mode", `{"threadId":"th","turnId":"tu","item":{"type":"custom_tool_call","call_id":"call-1","name":"exec","input":"const r = await tools.exec_command({cmd:\"mkdir /var/tmp/x\",workdir:\"/repo\"}); text(JSON.stringify(r));"}}`, `{"threadId":"th","turnId":"tu","item":{"type":"custom_tool_call_output","call_id":"call-1","output":[{"type":"input_text","text":"Script completed\n"},{"type":"input_text","text":"{\"exit_code\":1,\"output\":\"Read-only file system\\n\"}"}]}}`},
		{"direct", `{"threadId":"th","turnId":"tu","item":{"type":"function_call","call_id":"call-1","name":"exec_command","arguments":{"cmd":"mkdir /var/tmp/x","workdir":"/repo"}}}`, `{"threadId":"th","turnId":"tu","item":{"type":"function_call_output","call_id":"call-1","output":"{\"exit_code\":1,\"output\":\"Read-only file system\\n\"}"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &conversation{rawCodeModeExec: map[string]codeModeExecCall{}, rawExecCompleted: map[string]ports.ChatEvent{}}
			c.normalizeRawExec(notification{Method: methodRawResponseItemCompleted, Params: json.RawMessage(tc.call)})
			if len(c.rawExecCompleted) != 0 {
				t.Fatal("call emitted completion")
			}
			c.normalizeRawExec(notification{Method: methodRawResponseItemCompleted, Params: json.RawMessage(tc.output)})
			ev, ok := c.rawExecCompleted["call-1"]
			if !ok || ev.ActivityStatus != domain.ActivityStatusFailed || ev.ProviderTurnID != "tu" {
				t.Fatalf("event=%+v ok=%t", ev, ok)
			}
			var detail map[string]any
			if err := json.Unmarshal(ev.Detail, &detail); err != nil || detail["command"] != "mkdir /var/tmp/x" || detail["output"] != "Read-only file system\n" {
				t.Fatalf("detail=%s err=%v", ev.Detail, err)
			}
			c.normalizeRawExec(notification{Method: methodRawResponseItemCompleted, Params: json.RawMessage(tc.output)})
			if len(c.rawExecCompleted) != 1 {
				t.Fatal("duplicate output changed fallback set")
			}
		})
	}
}

func TestParseDirectExecArgumentsFailsClosed(t *testing.T) {
	got, ok := parseDirectExecArguments(json.RawMessage(`{"cmd":"go test ./...","workdir":"/repo"}`))
	if !ok || got.command != "go test ./..." || got.cwd != "/repo" {
		t.Fatalf("got=%+v ok=%t", got, ok)
	}
	for _, bad := range []string{`{}`, `{"cmd":""}`, `not-json`} {
		if _, ok := parseDirectExecArguments(json.RawMessage(bad)); ok {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestInterruptDefersInterruptedTurnUntilOwnedTreeStops(t *testing.T) {
	d, srv := newTestDriver(t)
	convRaw, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	conv := convRaw.(*conversation)
	forced := make(chan struct{}, 1)
	conv.proc.forceStop = func() error { forced <- struct{}{}; return nil }
	defer func() { _ = conv.Close() }()

	srv.push(`{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"inProgress","items":[]}}}`)
	srv.push(`{"method":"item/started","params":{"threadId":"thread-1","turnId":"turn-1","item":{"type":"commandExecution","id":"cmd-1","command":"sleep 3","status":"inProgress"}}}`)
	_ = nextEvent(t, conv.Events(), ports.ChatEventTurnStarted)
	_ = nextEvent(t, conv.Events(), ports.ChatEventActivityStarted)

	done := make(chan error, 1)
	go func() { done <- conv.Interrupt(context.Background(), "turn-1") }()
	_ = srv.awaitFrame(func(f frame) bool { return f.Method == "turn/interrupt" })
	deadline := time.Now().Add(time.Second)
	for {
		conv.mu.Lock()
		armed := conv.interrupting["turn-1"]
		conv.mu.Unlock()
		if armed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("interrupt was not armed")
		}
		time.Sleep(time.Millisecond)
	}
	srv.push(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"interrupted","items":[]}}}`)
	srv.push(`{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","item":{"type":"commandExecution","id":"cmd-1","command":"sleep 3","status":"completed","exitCode":130}}}`)
	_ = nextEvent(t, conv.Events(), ports.ChatEventActivityCompleted)
	select {
	case ev := <-conv.Events():
		t.Fatalf("interrupted terminal emitted before process-tree stop: %#v", ev)
	case <-time.After(50 * time.Millisecond):
	}
	if err := <-done; !errors.Is(err, ports.ErrChatInterruptRestartRequired) {
		t.Fatalf("Interrupt error=%v", err)
	}
	select {
	case <-forced:
	default:
		t.Fatal("forceStop was not called")
	}
	terminal := nextEvent(t, conv.Events(), ports.ChatEventTurnCompleted)
	if terminal.TurnState != domain.TurnStateInterrupted {
		t.Fatalf("terminal state=%s", terminal.TurnState)
	}
}
func TestInterruptForceStopsNonQuiescentOwnedProcess(t *testing.T) {
	clientReads, serverWrites := io.Pipe()
	serverReads, clientWrites := io.Pipe()
	forced := make(chan struct{}, 1)
	conv := newConversation(&process{stdin: clientWrites, stdout: clientReads, stop: func() error { return nil }, forceStop: func() error { forced <- struct{}{}; _ = serverWrites.Close(); return nil }}, slog.New(slog.DiscardHandler))
	conv.start("thread-1", "", "", nil)
	defer func() { _ = serverReads.Close(); _ = conv.Close() }()
	go func() {
		br := bufio.NewReader(serverReads)
		for {
			line, err := readFrame(br)
			if err != nil {
				return
			}
			var f frame
			if json.Unmarshal(line, &f) != nil || f.ID == nil {
				continue
			}
			_, _ = io.WriteString(serverWrites, `{"id":`+string(*f.ID)+`,"result":{}}`+"\n")
		}
	}()
	conv.mu.Lock()
	conv.activeCommands["turn-1"] = 1
	conv.interrupting["turn-1"] = false
	conv.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := conv.Interrupt(ctx, "turn-1")
	if !errors.Is(err, ports.ErrChatInterruptRestartRequired) {
		t.Fatalf("Interrupt error=%v", err)
	}
	select {
	case <-forced:
	default:
		t.Fatal("forceStop was not called")
	}
}

func TestConnectionCloseDoesNotReleaseUnverifiedInterruptedTerminal(t *testing.T) {
	clientReads, serverWrites := io.Pipe()
	serverReads, clientWrites := io.Pipe()
	conv := newConversation(&process{stdin: clientWrites, stdout: clientReads, stop: func() error { return nil }}, slog.New(slog.DiscardHandler))
	conv.start("thread-1", "", "", nil)
	defer func() { _ = serverReads.Close(); _ = conv.Close() }()

	conv.mu.Lock()
	conv.interrupting["turn-1"] = true
	conv.mu.Unlock()
	serverWrites.Write([]byte(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"interrupted","items":[]}}}` + "\n"))

	deadline := time.Now().Add(time.Second)
	for {
		conv.mu.Lock()
		_, deferred := conv.deferredTerminal["turn-1"]
		conv.mu.Unlock()
		if deferred {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("interrupted terminal was not deferred")
		}
		time.Sleep(time.Millisecond)
	}

	// A generic connection close proves only that app-server transport ended.
	// A detached child can still be producing effects, so this must not release
	// the deferred interrupted terminal.
	_ = serverWrites.Close()
	for ev := range conv.Events() {
		if ev.Kind == ports.ChatEventTurnCompleted && ev.ProviderTurnID == "turn-1" {
			t.Fatalf("unverified interrupted terminal escaped on connection close: %#v", ev)
		}
	}
}

func TestDispatchTurnReportsAcknowledgedWithTransportEvidence(t *testing.T) {
	d, _ := newTestDriver(t)
	opened, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = opened.Close() }()
	dispatcher, ok := opened.(ports.ChatTurnDispatcher)
	if !ok {
		t.Fatal("Codex conversation has no evidence-aware dispatch")
	}
	got, err := dispatcher.DispatchTurn(context.Background(), ports.ChatUserMessage{Text: "go", ClientMessageID: "dispatch-evidence-1"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Acceptance != ports.ChatTurnAcknowledged || got.Ref.ProviderTurnID != "turn-1" || got.TransportRequestID <= 0 || got.TransportSHA256 == "" || got.TransportBytes <= 0 || got.TransportSequence <= 0 {
		t.Fatalf("dispatch evidence=%+v", got)
	}
}

func TestDispatchTurnReportsProviderRejectionAfterFullWrite(t *testing.T) {
	d, srv := newTestDriver(t)
	srv.replyError("turn/start", -32602, "invalid turn")
	opened, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = opened.Close() }()
	got, err := opened.(ports.ChatTurnDispatcher).DispatchTurn(context.Background(), ports.ChatUserMessage{Text: "go"})
	if err == nil {
		t.Fatal("expected provider rejection")
	}
	if got.Acceptance != ports.ChatTurnRejected || got.TransportSHA256 == "" || got.Ref.ProviderTurnID != "" {
		t.Fatalf("rejection evidence=%+v err=%v", got, err)
	}
}

func TestDispatchTurnReportsUnknownAfterFullWriteWithoutResponse(t *testing.T) {
	d, srv := newTestDriver(t)
	srv.mu.Lock()
	delete(srv.responses, "turn/start")
	srv.mu.Unlock()
	opened, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = opened.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	got, err := opened.(ports.ChatTurnDispatcher).DispatchTurn(ctx, ports.ChatUserMessage{Text: "go"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	if got.Acceptance != ports.ChatTurnDeliveryUnknown || got.TransportSHA256 == "" || got.TransportBytes <= 0 || got.TransportSequence <= 0 {
		t.Fatalf("unknown evidence=%+v", got)
	}
}

func TestDispatchTurnReportsNotSentBeforeTransport(t *testing.T) {
	d, srv := newTestDriver(t)
	opened, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = opened.Close() }()
	got, err := opened.(ports.ChatTurnDispatcher).DispatchTurn(context.Background(), ports.ChatUserMessage{Text: "   "})
	if err == nil {
		t.Fatal("expected local validation failure")
	}
	if got.Acceptance != ports.ChatTurnNotSent || got.TransportRequestID != 0 || got.TransportSHA256 != "" || srv.sentMethod("turn/start") {
		t.Fatalf("not-sent evidence=%+v turn/start sent=%v", got, srv.sentMethod("turn/start"))
	}
}

func TestDispatchTurnReportsUnknownWhenWrittenResponseHasNoTurnID(t *testing.T) {
	d, srv := newTestDriver(t)
	srv.respondTo("turn/start", `{"turn":{}}`)
	opened, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = opened.Close() }()
	got, err := opened.(ports.ChatTurnDispatcher).DispatchTurn(context.Background(), ports.ChatUserMessage{Text: "go"})
	if err == nil {
		t.Fatal("expected missing turn id")
	}
	if got.Acceptance != ports.ChatTurnDeliveryUnknown || got.TransportSHA256 == "" || got.Ref.ProviderTurnID != "" {
		t.Fatalf("missing-id evidence=%+v err=%v", got, err)
	}
}
