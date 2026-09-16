package codexappserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/chatdriver/codexappserver/codexproto"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// eventBuffer bounds the normalized event stream. Deltas are dropped when a
// consumer falls this far behind; lifecycle events are not.
var _ ports.ChatTurnDispatcher = (*conversation)(nil)

const eventBuffer = 4096

// approvalWait bounds how long the provider is left blocked on an unanswered
// request. Codex holds its turn until the client replies, so an approval nobody
// resolves would hang the session indefinitely. On expiry Kennel refuses rather than
// deciding on the user's behalf.
const approvalWait = 30 * time.Minute

// interruptQuiescenceWait is the provider grace period after it accepts Stop.
// A command that remains active after this bound is not safe to leave attached:
// Codex 0.154.0 can report the turn interrupted while its shell keeps writing.
const interruptQuiescenceWait = 1500 * time.Millisecond

// errConversationClosed reports a decision arriving after the controller ended.
var errConversationClosed = errors.New("conversation closed")

// approvalMethods are the server->client requests that represent a decision the
// user must make. Anything else the provider asks is refused: answering a request
// Kennel does not model risks consenting to something on the user's behalf.
var approvalMethods = map[string]domain.ActivityKind{
	codexproto.MethodItemCommandExecutionRequestApproval: domain.ActivityKindCommand,
	codexproto.MethodItemFileChangeRequestApproval:       domain.ActivityKindFileChange,
	codexproto.MethodItemPermissionsRequestApproval:      domain.ActivityKindApproval,
	codexproto.MethodItemToolRequestUserInput:            domain.ActivityKindApproval,
}

// conversation is one live Codex thread. It is the only writer to that thread.
type conversation struct {
	conn *conn
	proc *process
	log  *slog.Logger
	// caps is the negotiated capability set for the installed build, attached
	// by the driver after protocol negotiation. Nil means never negotiated —
	// only true for conversations built directly in pipe tests.
	caps ports.ChatCapabilities

	threadID string
	events   chan ports.ChatEvent
	// Effective defaults returned when Codex opened or resumed this thread.
	threadModel, threadEffort string
	// governedSandboxPolicy is sent on every governed turn. Thread/start only
	// accepts a broad sandbox name; turn/start carries the effective boundary.
	governedSandboxPolicy map[string]any
	// nativeSandboxPolicy is the immutable Stage 1 substrate constraint. It is
	// separate from governed Outcome execution and is pinned after caller turn
	// settings. nativePolicyEvidence stores bounded records without raw frames or
	// prompt content.
	nativeSandboxPolicy  map[string]any
	nativePolicyEvidence []ports.ChatNativePolicyEvidence
	runtimeBinarySHA256  string
	protocolDigest       string
	// intelligencePermissions names the request-scoped, injected permission
	// profile pinned on every Waldo proposal turn. It must never be inferred
	// from ordinary Chat permission modes.
	intelligencePermissions string
	intelligenceWorkspace   string

	mu      sync.Mutex
	pending map[string]*parkedRequest
	// rawCodeModeExec pairs the public opt-in rawResponseItem/completed call/output
	// records that Codex emits for Code Mode exec. Pump is its sole owner.
	rawCodeModeExec map[string]codeModeExecCall
	// rawExecCompleted holds raw fallback completions until turn/completed, so a
	// standard commandExecution item can win without duplicate activities. Pump owns it.
	rawExecCompleted map[string]ports.ChatEvent
	closed           bool
	// emitWG tracks in-flight emit calls so c.events is only closed once every
	// send that started before closed flipped true has returned. Without this,
	// a caller outside pump (Interrupt's clearInterrupt, in particular) can be
	// sending on c.events at the exact moment pump's own shutdown closes it.
	emitWG sync.WaitGroup
	// interruptWG keeps the event stream open for an Interrupt already in
	// progress. Only that caller can release a deferred interrupted terminal
	// after it verifies process-tree quiescence; generic connection shutdown
	// cannot make that claim.
	interruptWG sync.WaitGroup

	// sendMu serializes turn dispatch so only one operation mutates the provider
	// conversation at a time.
	sendMu sync.Mutex

	// activeTurn is the most recent provider turn id, used when a caller asks to
	// interrupt without naming one.
	activeTurn string
	// terminalTurns and activeCommands let Interrupt distinguish a settled stop
	// from Codex's early interrupted notification. They are bounded to the live
	// thread and reset as turns settle.
	terminalTurns    map[string]bool
	activeCommands   map[string]int
	interrupting     map[string]bool
	deferredTerminal map[string]ports.ChatEvent

	// contextTokens is the conversation's latest position in the model's context,
	// and contextWindow the size of that context. Both come from the provider's
	// token-usage reports; zero means it has not reported one yet.
	contextTokens int64
	contextWindow int64
	// contextAtTurnStart is contextTokens as it stood when the current provider turn
	// began. It is the "before" half of a compaction reclaim: a compaction runs as
	// its own provider turn, so the figure captured at that turn's start is the
	// context position the compaction is about to shrink.
	contextAtTurnStart int64
	// compactedTurn is the last turn a compaction was reported for, so a provider
	// build that emits both the notification and the item reports one compaction
	// once.
	compactedTurn string

	pumpDone  chan struct{}
	closeOnce sync.Once
}

var _ ports.ChatConversation = (*conversation)(nil)

// Asserted here so a refactor cannot silently drop model listing: the service
// feature-detects this interface, and a missed method would just mean "no models"
// with nothing to notice.
var _ ports.ChatModelLister = (*conversation)(nil)

// Same reasoning for the quota read: a dropped method would silently become "this
// account has no limits to report", which is the wrong answer to show a user
// whose turns are about to start failing.
var _ ports.ChatUsageReporter = (*conversation)(nil)

// Same reason, for compaction. Losing this method does not break a build; it just
// makes the control disappear and long conversations start failing again.
var _ ports.ChatCompactor = (*conversation)(nil)

// Same reason, for the MCP reload: a dropped method makes the affordance vanish and
// leaves a session with a dead tool server no way back.
var _ ports.ChatMCPReloader = (*conversation)(nil)

func newConversation(proc *process, log *slog.Logger) *conversation {
	c := &conversation{
		proc:             proc,
		log:              log,
		events:           make(chan ports.ChatEvent, eventBuffer),
		pending:          make(map[string]*parkedRequest),
		rawCodeModeExec:  make(map[string]codeModeExecCall),
		rawExecCompleted: make(map[string]ports.ChatEvent),
		terminalTurns:    make(map[string]bool),
		activeCommands:   make(map[string]int),
		interrupting:     make(map[string]bool),
		deferredTerminal: make(map[string]ports.ChatEvent),
		pumpDone:         make(chan struct{}),
	}
	c.conn = newConn(proc.stdin, proc.stdout, log, c.handleServerRequest)
	return c
}

// start records the opened thread and begins translating notifications. It is
// called once, after the thread is open, so no event is emitted for a
// conversation the caller does not yet have a handle to.
func (c *conversation) start(threadID, model, effort string, governedSandboxPolicy map[string]any) {
	c.threadID = threadID
	c.threadModel = model
	c.threadEffort = effort
	c.governedSandboxPolicy = governedSandboxPolicy
	go c.pump()
}

func (c *conversation) configureNativeSandbox(policy map[string]any, initial ports.ChatNativePolicyEvidence) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	initial.Sequence = 1
	initial.RuntimeBinarySHA256 = c.runtimeBinarySHA256
	initial.ProtocolDigest = c.protocolDigest
	records := []ports.ChatNativePolicyEvidence{cloneNativePolicyEvidence(initial)}
	if err := validateNativePolicyEvidence(records, policy, initial.ThreadID, c.runtimeBinarySHA256, c.protocolDigest); err != nil {
		return err
	}
	c.nativeSandboxPolicy = cloneSandboxPolicy(policy)
	c.nativePolicyEvidence = records
	return nil
}

// NativePolicyEvidence returns a defensive copy of the bounded Stage 1 records.
func (c *conversation) NativePolicyEvidence() []ports.ChatNativePolicyEvidence {
	c.mu.Lock()
	defer c.mu.Unlock()
	records := make([]ports.ChatNativePolicyEvidence, len(c.nativePolicyEvidence))
	for i := range c.nativePolicyEvidence {
		records[i] = cloneNativePolicyEvidence(c.nativePolicyEvidence[i])
	}
	return records
}

// ProviderConversationID is the Codex thread id Kennel persists for resume.
func (c *conversation) ProviderConversationID() string { return c.threadID }

// Capabilities reports what this conversation can do.
func (c *conversation) Capabilities() ports.ChatCapabilities {
	// When the driver negotiated a live surface it attaches the result; pipe
	// tests that build a conversation directly keep the static table.
	if c.caps != nil {
		return c.caps
	}
	return capabilities()
}

// Events is the normalized stream. It closes when the conversation ends.
func (c *conversation) Events() <-chan ports.ChatEvent { return c.events }

// pump translates provider notifications into neutral events until the
// connection ends, then reports why and closes the stream.
func (c *conversation) pump() {
	defer close(c.pumpDone)
	defer c.closeEvents()

	for n := range c.conn.notifs() {
		// Before normalizing, because a token-usage report is the only place the
		// context position is stated and a compaction event that arrives in the same
		// batch has to be able to read it.
		c.trackContext(n)

		// The clock is passed in rather than read inside: a rate-limit reset arrives
		// as an absolute instant and has to become a remaining duration, and a
		// normalizer that reads the clock itself cannot be tested deterministically.
		c.normalizeRawExec(n)

		for _, ev := range normalizeNotification(n, time.Now()) {
			c.trackInterruptState(ev)
			if c.deferInterruptedTerminal(ev) {
				continue
			}
			if ev.Kind == ports.ChatEventActivityCompleted && ev.ActivityKind == domain.ActivityKindCommand {
				delete(c.rawExecCompleted, ev.ProviderItemID)
			}
			if ev.Kind == ports.ChatEventTurnCompleted {
				for callID, fallback := range c.rawExecCompleted {
					if fallback.ProviderTurnID == ev.ProviderTurnID {
						c.emit(fallback)
						delete(c.rawExecCompleted, callID)
					}
				}
			}
			rootConversation := ev.ProviderConversationID == "" || ev.ProviderConversationID == c.threadID
			if ev.Kind == ports.ChatEventTurnStarted && ev.ProviderTurnID != "" && rootConversation {
				c.mu.Lock()
				c.activeTurn = ev.ProviderTurnID
				// Snapshot the context position this turn starts from. Cheap on every
				// turn, and the only way to know what a compaction reclaimed.
				c.contextAtTurnStart = c.contextTokens
				c.mu.Unlock()
			}
			if ev.Kind == ports.ChatEventTurnCompleted && ev.ProviderTurnID != "" && rootConversation {
				c.mu.Lock()
				if c.activeTurn == ev.ProviderTurnID {
					c.activeTurn = ""
				}
				c.mu.Unlock()
			}
			if ev.Kind == ports.ChatEventCompacted {
				settled, ok := c.settleCompaction(ev)
				if !ok {
					continue
				}
				ev = settled
			}
			if ev.Kind == ports.ChatEventApprovalResolved {
				// The provider resolved it (possibly via another client), so any
				// card Kennel is still showing is stale.
				c.discardPending(ev.RequestID)
			}
			c.emit(ev)
		}
	}

	// The connection ended. Say so explicitly rather than letting the stream go
	// quiet: a silent channel close is indistinguishable from an idle agent.
	state := ports.ChatEvent{Kind: ports.ChatEventControllerState, ControllerState: ports.ChatControllerStopped}
	if err := c.conn.err(); err != nil {
		state.Err = err
	}
	c.emit(state)
	c.failPendingApprovals()
}

// emit delivers an event, preferring to drop a delta over blocking the reader. A
// lifecycle event is never dropped silently.
//
// pump is not the only emitter: Interrupt's forced-stop path emits its own
// terminal event from the caller's goroutine, after the app-server process (and
// so pump's connection) may already have ended. Sending on c.events after pump
// has closed it would panic, so every send is gated on closed, and closeEvents
// waits for every emit that got past that gate before it closes the channel.
func (c *conversation) emit(ev ports.ChatEvent) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.emitWG.Add(1)
	c.mu.Unlock()
	defer c.emitWG.Done()

	select {
	case c.events <- ev:
		return
	default:
	}

	switch ev.Kind {
	case ports.ChatEventMessageDelta, ports.ChatEventReasoningDelta:
		// The settled text arrives on item/completed for both of these — a message's
		// as `text`, a reasoning item's as `summary` — so a dropped delta costs
		// smoothness, not correctness.
		//
		// The other streams are deliberately NOT droppable. Command output, terminal
		// keystrokes and tool progress have no settled restatement: the delta is the
		// only account of them, so losing one loses the record.
		c.log.Warn("dropped chat delta: consumer behind",
			"kind", ev.Kind, "item", ev.ProviderItemID)
		return
	}

	select {
	case c.events <- ev:
	case <-time.After(5 * time.Second):
		c.log.Error("dropped chat lifecycle event: consumer stalled", "kind", ev.Kind)
	}
}

// closeEvents ends the event stream once every emit already admitted past the
// closed gate has returned. Setting closed here is redundant with the common
// path (failPendingApprovals already set it before pump's defers run), but
// Close can end the connection before pump ever reaches that point, so this
// stays the single place that guarantees closed is true before the channel is.
func (c *conversation) closeEvents() {
	// If an Interrupt is already verifying quiescence, let it release (or
	// discard) its deferred terminal before closing the stream. A connection
	// close with no active Interrupt never releases deferred terminals.
	c.interruptWG.Wait()
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.emitWG.Wait()
	close(c.events)
}

// SendTurn delivers one message to the provider.
func (c *conversation) SendTurn(ctx context.Context, msg ports.ChatUserMessage) (ports.ChatTurnRef, error) {
	dispatch, err := c.dispatchTurn(ctx, msg, nil)
	return dispatch.Ref, err
}

// DispatchTurn preserves the distinction between a provider rejection and an
// ambiguous result after the complete request frame crossed the transport.
func (c *conversation) DispatchTurn(ctx context.Context, msg ports.ChatUserMessage) (ports.ChatTurnDispatch, error) {
	return c.dispatchTurn(ctx, msg, nil)
}

// sendTurn is the shared turn/start boundary. Structured reasoning uses the
// provider's native outputSchema field; keeping that option inside this
// adapter prevents a generic chat caller from smuggling provider wire fields
// through the ports contract.
func (c *conversation) sendTurn(ctx context.Context, msg ports.ChatUserMessage, outputSchema json.RawMessage) (ports.ChatTurnRef, error) {
	dispatch, err := c.dispatchTurn(ctx, msg, outputSchema)
	return dispatch.Ref, err
}

func (c *conversation) dispatchTurn(ctx context.Context, msg ports.ChatUserMessage, outputSchema json.RawMessage) (ports.ChatTurnDispatch, error) {
	if strings.TrimSpace(msg.Text) == "" {
		// There is no keystroke concept here: an empty message is a caller bug,
		// not a way to nudge the agent.
		return ports.ChatTurnDispatch{Acceptance: ports.ChatTurnNotSent}, errors.New("chat message text is empty")
	}

	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	params := map[string]any{
		"threadId": c.threadID,
		"input":    []any{map[string]any{"type": "text", "text": msg.Text}},
	}
	if msg.ClientMessageID != "" {
		// The provider's own idempotency handle: a retry carrying the same id
		// must not produce a second turn.
		params["clientUserMessageId"] = msg.ClientMessageID
	}
	if len(outputSchema) > 0 {
		params["outputSchema"] = append(json.RawMessage(nil), outputSchema...)
	}
	if c.intelligencePermissions != "" {
		// The named profile carries exact filesystem roots plus network denial.
		// Pin it and the runtime root on every proposal turn; sandboxPolicy cannot
		// be combined with a permissions profile in the app-server protocol.
		params["approvalPolicy"] = "never"
		params["permissions"] = c.intelligencePermissions
		params["runtimeWorkspaceRoots"] = []string{c.intelligenceWorkspace}
		params["environments"] = []any{}
	}
	if c.governedSandboxPolicy != nil {
		params["approvalPolicy"] = "on-request"
		params["sandboxPolicy"] = cloneSandboxPolicy(c.governedSandboxPolicy)
	}
	applyTurnSettings(params, msg.Settings)
	if c.governedSandboxPolicy != nil {
		// Per-turn UI settings must not widen the immutable Attempt boundary.
		params["approvalPolicy"] = "on-request"
		params["sandboxPolicy"] = cloneSandboxPolicy(c.governedSandboxPolicy)
	}
	if c.nativeSandboxPolicy != nil {
		// This Stage 1 profile is a substrate constraint, not governed authority.
		// Pin it after every caller setting so a per-turn choice cannot widen or
		// drop any frozen field.
		params["approvalPolicy"] = "on-request"
		params["sandboxPolicy"] = cloneSandboxPolicy(c.nativeSandboxPolicy)
	}

	var resp struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	var receipt transportReceipt
	var err error
	receipt, err = c.conn.requestWithTransportReceipt(ctx, "turn/start", params, &resp, c.nativeSandboxPolicy)
	dispatch := turnDispatchFromReceipt(receipt)
	if err != nil {
		var providerErr *rpcError
		switch {
		case !receipt.SuccessfulWrite:
			dispatch.Acceptance = ports.ChatTurnNotSent
		case errors.As(err, &providerErr):
			dispatch.Acceptance = ports.ChatTurnRejected
		default:
			dispatch.Acceptance = ports.ChatTurnDeliveryUnknown
		}
		return dispatch, fmt.Errorf("turn/start: %w", err)
	}
	dispatch.Acceptance = ports.ChatTurnAcknowledged
	if strings.TrimSpace(resp.Turn.ID) == "" {
		dispatch.Acceptance = ports.ChatTurnDeliveryUnknown
		return dispatch, errors.New("turn/start returned no turn id")
	}

	if c.nativeSandboxPolicy != nil {
		if err := c.appendNativeTurnPolicyEvidence(resp.Turn.ID, receipt, params); err != nil {
			dispatch.Acceptance = ports.ChatTurnAcknowledged
			dispatch.Ref.ProviderTurnID = resp.Turn.ID
			return dispatch, err
		}
	}
	c.mu.Lock()
	c.activeTurn = resp.Turn.ID
	c.mu.Unlock()

	dispatch.Ref.ProviderTurnID = resp.Turn.ID
	return dispatch, nil
}

// applyTurnSettings folds the caller's per-turn choices into a turn/start payload.
//
// Only fields the caller actually chose are sent. An omitted field lets the
// provider fall back to what the thread was started with, which is why a caller
// that chooses nothing behaves exactly as it did before per-turn settings existed.
func turnDispatchFromReceipt(receipt transportReceipt) ports.ChatTurnDispatch {
	return ports.ChatTurnDispatch{
		TransportRequestID: receipt.RequestID, TransportSHA256: receipt.SHA256,
		TransportBytes: receipt.ByteCount, TransportSequence: receipt.WriteSequence,
	}
}

func applyTurnSettings(params map[string]any, settings ports.ChatTurnSettings) {
	if settings.Model != "" {
		params["model"] = settings.Model
	}
	if settings.Effort != "" {
		params["effort"] = settings.Effort
	}
	if settings.Approval != "" {
		// The same posture thread/start applies, so a per-turn choice and a launch
		// choice cannot mean different things. The wire shapes differ though: a
		// thread takes `sandbox: "workspace-write"`, a turn takes a tagged
		// `sandboxPolicy: {type: "workspaceWrite"}`. Sending a thread's shape to a
		// turn is rejected as a missing `type`, so the two are mapped separately
		// rather than assumed to be interchangeable.
		policy, sandbox := approvalSettings(settings.Approval)
		params["approvalPolicy"] = policy
		params["sandboxPolicy"] = turnSandboxPolicy(sandbox)
	}
}

// turnSandboxPolicy converts a thread-level sandbox name into the tagged object
// turn/start expects.
func turnSandboxPolicy(sandbox string) map[string]any {
	switch sandbox {
	case "workspace-write":
		return map[string]any{"type": "workspaceWrite"}
	case "read-only":
		return map[string]any{"type": "readOnly"}
	default:
		return map[string]any{"type": "dangerFullAccess"}
	}
}

func turnSandboxPolicyForExecution(sandbox string) map[string]any {
	if sandbox == "read-only" {
		return map[string]any{
			"type":          "readOnly",
			"networkAccess": false,
		}
	}
	return map[string]any{
		"type":                "workspaceWrite",
		"networkAccess":       false,
		"writableRoots":       []string{},
		"excludeSlashTmp":     true,
		"excludeTmpdirEnvVar": true,
	}
}

func cloneSandboxPolicy(policy map[string]any) map[string]any {
	if policy == nil {
		return nil
	}
	clone := make(map[string]any, len(policy))
	for key, value := range policy {
		clone[key] = value
	}
	return clone
}

func cloneNativePolicyEvidence(record ports.ChatNativePolicyEvidence) ports.ChatNativePolicyEvidence {
	record.RequestedPolicy = cloneSandboxPolicy(record.RequestedPolicy)
	record.ObservedPolicy = cloneSandboxPolicy(record.ObservedPolicy)
	return record
}

func (c *conversation) appendNativeTurnPolicyEvidence(turnID string, receipt transportReceipt, params map[string]any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	expectedSHA256, expectedByteCount, err := canonicalRequestFrameDigest(receipt.RequestID, "turn/start", params)
	if err != nil || receipt.Method != "turn/start" || receipt.SHA256 != expectedSHA256 || receipt.ByteCount != expectedByteCount {
		return fmt.Errorf("native turn transport receipt mismatch: method=%q sha_match=%t bytes=%d want=%d: %v",
			receipt.Method, receipt.SHA256 == expectedSHA256, receipt.ByteCount, expectedByteCount, err)
	}
	record := ports.ChatNativePolicyEvidence{
		Sequence: int64(len(c.nativePolicyEvidence) + 1), Boundary: ports.ChatNativePolicyBoundaryTurnStart,
		ClaimClass: ports.ChatNativePolicyClaimRequestIntegrity, ThreadID: c.threadID, ProviderTurnID: turnID,
		RequestedPolicy: cloneSandboxPolicy(c.nativeSandboxPolicy), ObservedPolicy: nil,
		ObservationSource:               "public_protocol_unavailable",
		ProviderObservationAvailability: ports.ChatNativePolicyObservationPublicProtocolUnavailable,
		ComparisonResult:                ports.ChatNativePolicyComparisonNotObservable,
		CanonicalizationVersion:         nativePolicyCanonicalizationVersion,
		RuntimeBinarySHA256:             c.runtimeBinarySHA256, ProtocolDigest: c.protocolDigest,
		RequestWireShape:   "jsonrpc2 newline-delimited exact-frame-sha256",
		TransportRequestID: receipt.RequestID, TransportMethod: receipt.Method, TransportWriteSequence: receipt.WriteSequence,
		TransportSHA256: receipt.SHA256, TransportByteCount: receipt.ByteCount,
		TransportSuccessfulWrite: receipt.SuccessfulWrite, Timestamp: receipt.WrittenAt,
	}
	records := append(append([]ports.ChatNativePolicyEvidence(nil), c.nativePolicyEvidence...), record)
	if err := validateNativePolicyEvidence(records, c.nativeSandboxPolicy, c.threadID, c.runtimeBinarySHA256, c.protocolDigest); err != nil {
		return fmt.Errorf("native turn policy evidence: %w", err)
	}
	c.nativePolicyEvidence = records
	return nil
}

func validateNativePolicyEvidence(records []ports.ChatNativePolicyEvidence, expectedPolicy map[string]any, threadID, runtimeSHA256, protocolDigest string) error {
	if len(records) == 0 {
		return errors.New("native policy evidence is empty")
	}
	if !isLowerHexSHA256(runtimeSHA256) || protocolDigest == "" {
		return errors.New("native policy runtime/protocol provenance is missing")
	}
	seenTurns := map[string]bool{}
	seenRequests := map[int64]bool{}
	seenWrites := map[int64]bool{}
	var previousWrite int64
	for i, record := range records {
		if record.Sequence != int64(i+1) || record.ThreadID != threadID || record.Timestamp.IsZero() ||
			record.RuntimeBinarySHA256 != runtimeSHA256 || record.ProtocolDigest != protocolDigest ||
			record.CanonicalizationVersion != nativePolicyCanonicalizationVersion {
			return fmt.Errorf("record %d has invalid sequence, identity, time, or provenance", i)
		}
		if i == 0 {
			if record.Boundary != ports.ChatNativePolicyBoundaryThreadStart && record.Boundary != ports.ChatNativePolicyBoundaryThreadResume {
				return errors.New("first native policy record is not Start or Resume")
			}
			expectedSource := "thread_start_response.sandbox"
			expectedWireShape := "thread/start sandbox enum"
			if record.Boundary == ports.ChatNativePolicyBoundaryThreadResume {
				expectedSource = "thread_resume_response.sandbox"
				expectedWireShape = "thread/resume sandbox enum"
			}
			if record.ClaimClass != ports.ChatNativePolicyClaimProviderAcknowledgment ||
				record.ProviderObservationAvailability != ports.ChatNativePolicyObservationAvailable ||
				record.ComparisonResult != ports.ChatNativePolicyComparisonMatchComparableFields || record.ObservedPolicy == nil ||
				record.ObservationSource != expectedSource || record.RequestWireShape != expectedWireShape ||
				!semanticJSONEqual(record.RequestedPolicy, map[string]any{"sandbox": "workspace-write"}) ||
				record.TransportSuccessfulWrite || record.TransportRequestID != 0 || record.TransportWriteSequence != 0 ||
				record.TransportMethod != "" || record.TransportSHA256 != "" || record.TransportByteCount != 0 {
				return errors.New("native Start/Resume acknowledgment is incomplete")
			}
			if err := validateNativeThreadSandbox(record.ObservedPolicy); err != nil {
				return err
			}
			continue
		}
		if record.Boundary != ports.ChatNativePolicyBoundaryTurnStart || record.ClaimClass != ports.ChatNativePolicyClaimRequestIntegrity ||
			record.ProviderTurnID == "" || record.ObservedPolicy != nil || record.ObservationSource != "public_protocol_unavailable" ||
			record.ProviderObservationAvailability != ports.ChatNativePolicyObservationPublicProtocolUnavailable ||
			record.ComparisonResult != ports.ChatNativePolicyComparisonNotObservable || !record.TransportSuccessfulWrite ||
			record.RequestWireShape != "jsonrpc2 newline-delimited exact-frame-sha256" ||
			record.TransportRequestID <= 0 || record.TransportMethod != "turn/start" || record.TransportWriteSequence <= previousWrite ||
			!isLowerHexSHA256(record.TransportSHA256) || record.TransportByteCount <= 0 ||
			!semanticJSONEqual(record.RequestedPolicy, expectedPolicy) {
			return fmt.Errorf("turn policy record %d is incomplete, widened, mutated, or out of order", i)
		}
		if seenTurns[record.ProviderTurnID] || seenRequests[record.TransportRequestID] || seenWrites[record.TransportWriteSequence] {
			return fmt.Errorf("turn policy record %d duplicates turn, request, or write identity", i)
		}
		seenTurns[record.ProviderTurnID] = true
		seenRequests[record.TransportRequestID] = true
		seenWrites[record.TransportWriteSequence] = true
		previousWrite = record.TransportWriteSequence
	}
	return nil
}

func isLowerHexSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

// ListModels asks the provider which models this account may use.
//
// The provider is the only honest source: models get added, renamed, hidden per
// account, and gated by entitlement that Kennel cannot see. A table in Kennel would be
// wrong within a week.
func (c *conversation) ListModels(ctx context.Context) ([]ports.ChatModel, error) {
	var resp struct {
		Data []struct {
			ID          string `json:"id"`
			Model       string `json:"model"`
			DisplayName string `json:"displayName"`
			Description string `json:"description"`
			IsDefault   bool   `json:"isDefault"`
			Hidden      bool   `json:"hidden"`
			DefaultEff  string `json:"defaultReasoningEffort"`
			Efforts     []struct {
				ReasoningEffort string `json:"reasoningEffort"`
			} `json:"supportedReasoningEfforts"`
		} `json:"data"`
	}
	if err := c.conn.request(ctx, "model/list", map[string]any{}, &resp); err != nil {
		return nil, fmt.Errorf("model/list: %w", err)
	}

	models := make([]ports.ChatModel, 0, len(resp.Data))
	for _, entry := range resp.Data {
		if entry.Hidden {
			// The provider marks a model hidden when the account should not be
			// offered it. Showing it anyway would offer a choice that then fails.
			continue
		}
		id := entry.ID
		if id == "" {
			id = entry.Model
		}
		if id == "" {
			continue
		}
		efforts := make([]string, 0, len(entry.Efforts))
		for _, effort := range entry.Efforts {
			if effort.ReasoningEffort != "" {
				efforts = append(efforts, effort.ReasoningEffort)
			}
		}
		display := entry.DisplayName
		if display == "" {
			display = id
		}
		models = append(models, ports.ChatModel{
			ID:            id,
			DisplayName:   display,
			Description:   entry.Description,
			Default:       entry.IsDefault,
			Efforts:       efforts,
			DefaultEffort: entry.DefaultEff,
		})
	}
	// Thread settings include the user's config; model/list only has generic defaults.
	for i := range models {
		if models[i].ID == c.threadModel && c.threadEffort != "" {
			models[i].DefaultEffort = c.threadEffort
			break
		}
	}
	return models, nil
}

// ReadRateLimits asks the provider where the account stands right now.
//
// The provider also pushes account/rateLimits/updated, but only alongside a turn.
// That is too late for the question this answers: a user opening a conversation
// wants to know whether they have quota BEFORE spending a turn finding out. The
// controller reads once at startup for exactly that reason.
func (c *conversation) ReadRateLimits(ctx context.Context) (ports.ChatRateLimits, error) {
	var resp rateLimitsEnvelope
	if err := c.conn.request(ctx, "account/rateLimits/read", map[string]any{}, &resp); err != nil {
		return ports.ChatRateLimits{}, fmt.Errorf("account/rateLimits/read: %w", err)
	}
	// The read result also carries rateLimitsByLimitId, a per-model breakdown, and
	// rateLimitResetCredits. Neither is read: the meter's job is to say whether the
	// account is near a wall, and a per-model table would be a second, finer answer
	// to a question the user has not asked yet.
	return rateLimitsFrom(resp, time.Now()), nil
}

// Compact asks the provider to summarize earlier history and reclaim context.
//
// This is what keeps a long session usable: every turn re-sends the conversation,
// so context fills whether or not the user is doing anything unusual, and once it
// is full the thread cannot accept another turn at all. Compaction is the
// difference between a session that works for an hour and one that works for a day.
//
// The provider accepts the request and returns an empty result immediately; the
// work then runs as its own turn and took 9 to 15 seconds in every measured run.
// So this reports what is about to be reclaimed rather than what was: TokensAfter
// stays zero, and the settled figures reach the client as a ChatEventCompacted on
// the timeline. Blocking here would hold the controller's dispatch lock for the
// duration and make the whole conversation unresponsive while it ran.
func (c *conversation) Compact(ctx context.Context) (ports.ChatCompactionResult, error) {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	c.mu.Lock()
	before := c.contextTokens
	c.mu.Unlock()

	// thread/compact/start takes the thread id and nothing else. Notably NOT a turn
	// id: compaction is a property of the thread, and passing turn-shaped params
	// here is rejected.
	if err := c.conn.request(ctx, "thread/compact/start", map[string]any{
		"threadId": c.threadID,
	}, nil); err != nil {
		return ports.ChatCompactionResult{}, fmt.Errorf("thread/compact/start: %w", err)
	}

	return ports.ChatCompactionResult{TokensBefore: before}, nil
}

// trackContext folds a token-usage report into the conversation's known context
// position.
func (c *conversation) trackContext(n notification) {
	used, window, ok := contextPositionFrom(n)
	if !ok {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.contextTokens = used
	if window > 0 {
		// The provider omits the window on some reports; a remembered one is better
		// than dropping the scale the used figure is measured against.
		c.contextWindow = window
	}
}

// settleCompaction fills in what a compaction reclaimed, or reports false for one
// already accounted for.
//
// The figures are bracketed rather than read off the event because the provider
// reports neither: `thread/compacted` carries only ids, and the contextCompaction
// item carries only its own id. The reduced token figure arrives as an ordinary
// token-usage report between the item starting and completing, so by the time this
// runs the "after" side is already known.
func (c *conversation) settleCompaction(ev ports.ChatEvent) (ports.ChatEvent, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ev.ProviderTurnID != "" && ev.ProviderTurnID == c.compactedTurn {
		return ports.ChatEvent{}, false
	}
	c.compactedTurn = ev.ProviderTurnID

	before, after, window := c.contextAtTurnStart, c.contextTokens, c.contextWindow
	ev.Summary = compactionSummary(before, after)

	detail := map[string]any{}
	if before > 0 {
		detail["tokensBefore"] = before
	}
	if after > 0 {
		detail["tokensAfter"] = after
	}
	if before > after && after > 0 {
		detail["tokensReclaimed"] = before - after
	}
	if window > 0 {
		detail["contextWindow"] = window
	}
	if encoded, err := json.Marshal(detail); err == nil {
		ev.Detail = encoded
	}
	return ev, true
}

// compactionSummary labels the reclaim for a timeline row.
//
// It only claims a number it actually has. Kennel has no context figure until the
// provider reports one, so a compaction right after a restart genuinely does not
// know what it saved, and "reclaimed 0 tokens" would be a lie rather than a gap.
func compactionSummary(before, after int64) string {
	if before <= 0 || after <= 0 || after >= before {
		return "Compacted the conversation history"
	}
	return fmt.Sprintf("Compacted history, freeing %s of context", formatTokens(before-after))
}

// formatTokens renders a token count the way a reader scans it. Exact below a
// thousand, because that is where the digits still mean something.
func formatTokens(tokens int64) string {
	if tokens < 1000 {
		return fmt.Sprintf("%d tokens", tokens)
	}
	return fmt.Sprintf("%.1fk tokens", float64(tokens)/1000)
}

// deferInterruptedTerminal keeps the UI in its working/stopping state until
// Interrupt has either observed command settlement or killed the owned tree.
func (c *conversation) deferInterruptedTerminal(ev ports.ChatEvent) bool {
	if ev.Kind != ports.ChatEventTurnCompleted || ev.TurnState != domain.TurnStateInterrupted || ev.ProviderTurnID == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.interrupting[ev.ProviderTurnID] {
		return false
	}
	c.deferredTerminal[ev.ProviderTurnID] = ev
	return true
}

func (c *conversation) clearInterrupt(turnID string, emitTerminal bool) {
	c.mu.Lock()
	delete(c.interrupting, turnID)
	ev, ok := c.deferredTerminal[turnID]
	delete(c.deferredTerminal, turnID)
	c.mu.Unlock()
	if emitTerminal && ok {
		c.emit(ev)
	}
}

// trackInterruptState records only the lifecycle needed to know whether Stop
// has actually settled. Provider prose and output are deliberately irrelevant.
func (c *conversation) trackInterruptState(ev ports.ChatEvent) {
	if ev.ProviderTurnID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case ev.Kind == ports.ChatEventTurnStarted:
		delete(c.terminalTurns, ev.ProviderTurnID)
	case ev.Kind == ports.ChatEventTurnCompleted:
		c.terminalTurns[ev.ProviderTurnID] = true
	case ev.Kind == ports.ChatEventActivityStarted && ev.ActivityKind == domain.ActivityKindCommand:
		c.activeCommands[ev.ProviderTurnID]++
	case ev.Kind == ports.ChatEventActivityCompleted && ev.ActivityKind == domain.ActivityKindCommand:
		if c.activeCommands[ev.ProviderTurnID] > 1 {
			c.activeCommands[ev.ProviderTurnID]--
		} else {
			delete(c.activeCommands, ev.ProviderTurnID)
		}
	}
}

// Interrupt preserves the legacy error contract while governed callers consume
// the stronger dispatch and quiescence receipt.
func (c *conversation) Interrupt(ctx context.Context, providerTurnID string) error {
	_, err := c.DispatchInterrupt(ctx, providerTurnID)
	return err
}

// DispatchInterrupt cancels a turn and does not report process quiescence until
// Stage 1's owned-process boundary has been crossed.
func (c *conversation) DispatchInterrupt(ctx context.Context, providerTurnID string) (ports.ChatInterruptDispatch, error) {
	if providerTurnID == "" {
		c.mu.Lock()
		providerTurnID = c.activeTurn
		c.mu.Unlock()
	}
	if providerTurnID == "" {
		return ports.ChatInterruptDispatch{Acceptance: ports.ChatTurnNotSent}, ports.ErrChatNoActiveTurn
	}
	c.interruptWG.Add(1)
	defer c.interruptWG.Done()
	c.mu.Lock()
	c.interrupting[providerTurnID] = true
	c.mu.Unlock()
	receipt, err := c.conn.requestWithTransportReceipt(ctx, "turn/interrupt", map[string]any{
		"threadId": c.threadID,
		"turnId":   providerTurnID,
	}, nil, nil)
	dispatch := ports.ChatInterruptDispatch{TransportRequestID: receipt.RequestID, TransportSHA256: receipt.SHA256, TransportBytes: receipt.ByteCount, TransportSequence: receipt.WriteSequence, Quiescence: domain.GovernedCommandQuiescencePending}
	if err != nil {
		// The provider refuses an interrupt for a turn it does not consider
		// active — which happens either side of the turn: pressed before it has
		// acknowledged the start, or after it already finished. Neither is an
		// internal failure, so it is translated here, where the provider's
		// vocabulary is known, instead of escaping as a protocol error and
		// reaching the user as "Internal server error".
		c.clearInterrupt(providerTurnID, false)
		if !receipt.SuccessfulWrite {
			dispatch.Acceptance = ports.ChatTurnNotSent
		} else {
			dispatch.Acceptance = ports.ChatTurnDeliveryUnknown
		}
		if isNoActiveTurn(err) {
			dispatch.Acceptance = ports.ChatTurnRejected
			return dispatch, ports.ErrChatNoActiveTurn
		}
		return dispatch, fmt.Errorf("turn/interrupt: %w", err)
	}
	dispatch.Acceptance = ports.ChatTurnAcknowledged
	// Pipe-backed tests have no owned process to police. A real app-server does:
	// do not report Stop complete until its command lifecycle has settled.
	if c.proc.forceStop == nil {
		c.clearInterrupt(providerTurnID, true)
		dispatch.Quiescence = domain.GovernedCommandQuiescenceCodexTree
		dispatch.QuiescenceEvidenceRef = "codex-process-tree:no-owned-process:" + receipt.SHA256
		return dispatch, nil
	}
	deadline := time.NewTimer(interruptQuiescenceWait)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			c.clearInterrupt(providerTurnID, true)
			dispatch.Acceptance = ports.ChatTurnDeliveryUnknown
			return dispatch, ctx.Err()
		case <-ticker.C:
			// Provider lifecycle settlement is useful transcript evidence, but Codex
			// 0.154.0 can settle both turn and command before the shell's final write.
			// There is no process-level quiescence signal on the public protocol, so
			// only terminating the owned process group closes the effect boundary.
		case <-deadline.C:
			if err := c.proc.forceStop(); err != nil {
				c.clearInterrupt(providerTurnID, true)
				return dispatch, fmt.Errorf("force-stop interrupted app-server: %w", err)
			}
			c.clearInterrupt(providerTurnID, true)
			dispatch.Quiescence = domain.GovernedCommandQuiescenceCodexTree
			dispatch.QuiescenceEvidenceRef = "codex-process-tree:force-stopped:" + receipt.SHA256
			return dispatch, ports.ErrChatInterruptRestartRequired
		}
	}
}

// isNoActiveTurn recognizes the provider's "nothing to interrupt" refusal.
//
// Matched on the message because that is all app-server gives: the code is the
// generic -32600 it uses for any invalid request, so the code alone cannot
// distinguish this from a malformed call.
func isNoActiveTurn(err error) bool {
	var rpcErr *rpcError
	if !errors.As(err, &rpcErr) {
		return false
	}
	return strings.Contains(strings.ToLower(rpcErr.Message), "no active turn")
}

// ResolveRequest answers a parked approval or user-input request.
//
// The decision is checked against the set the provider offered for THIS request,
// and the request stays parked unless a valid answer is actually going through.
// Both halves matter: forwarding an invented decision is consent Kennel made up, and
// consuming the request on a bad one would leave the user's real answer with
// nothing left to answer while the provider waits out its timeout.
func (c *conversation) ResolveRequest(ctx context.Context, requestID string, decision ports.ChatDecision) error {
	c.mu.Lock()
	parked, ok := c.pending[requestID]
	closed := c.closed
	if closed {
		c.mu.Unlock()
		return errConversationClosed
	}
	if !ok {
		c.mu.Unlock()
		// Already resolved, superseded, or from a previous controller. Refusing is
		// required: a stale card must never resolve a newer request.
		return fmt.Errorf("%w: %q", ports.ErrChatRequestNotPending, requestID)
	}
	reply, err := parked.reply(decision)
	if err != nil {
		c.mu.Unlock()
		return err
	}
	delete(c.pending, requestID)
	c.mu.Unlock()

	select {
	case parked.ch <- reply:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// parkedRequest is one server-to-client request waiting on a person, together
// with the decisions the provider said were valid for it.
//
// Keeping the provider's own payload for each decision is what makes structured
// decisions answerable at all: some arrive as objects carrying parameters
// (acceptWithExecpolicyAmendment), and a client that only knows the id cannot
// reconstruct them. Kennel echoes what the provider sent rather than rebuilding it.
type parkedRequest struct {
	ch      chan ports.ChatDecision
	method  string
	offered map[string]json.RawMessage
}

// reply resolves a client decision into the payload to send back.
func (p *parkedRequest) reply(decision ports.ChatDecision) (ports.ChatDecision, error) {
	if p.method == "item/tool/requestUserInput" {
		// Not a decision but an answer: the provider offers questions, not options,
		// so there is no set to check it against.
		return decision, nil
	}
	raw, ok := p.offered[decision.ID]
	if !ok {
		return ports.ChatDecision{}, fmt.Errorf("%w: %q (offered: %s)",
			ports.ErrChatDecisionNotOffered, decision.ID, strings.Join(p.offeredIDs(), ", "))
	}
	// The provider's own encoding of the decision wins over whatever the client
	// sent, so an object-shaped decision round-trips exactly.
	decision.Raw = raw
	return decision, nil
}

func (p *parkedRequest) offeredIDs() []string {
	ids := make([]string, 0, len(p.offered))
	for id := range p.offered {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// handleServerRequest parks a provider request until a decision arrives.
//
// The provider blocks its turn on the reply, so this deliberately waits rather
// than answering immediately. Everything Kennel does not model is refused with an
// error, never with a fabricated decision.
func (c *conversation) handleServerRequest(ctx context.Context, req serverRequest) (any, error) {
	switch req.Method {
	case codexproto.MethodAccountChatgptAuthTokensRefresh:
		return nil, c.reportAuthRefreshRequest(req.Params)
	case codexproto.MethodItemToolCall:
		return nil, c.refuseDynamicToolCall(req.Params)
	}

	kind, known := approvalMethods[req.Method]
	if !known {
		c.log.Warn("refusing unmodelled app-server request", "method", req.Method)
		return nil, fmt.Errorf("unsupported request %s", req.Method)
	}

	requestID := rawID(req.ID)
	if requestID == "" {
		return nil, errors.New("server request carried no id")
	}

	decisions, summary, detail := parseApproval(req.Method, req.Params)

	offered := make(map[string]json.RawMessage, len(decisions))
	for _, option := range decisions {
		offered[option.ID] = option.Raw
	}

	ch := make(chan ports.ChatDecision, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errConversationClosed
	}
	c.pending[requestID] = &parkedRequest{ch: ch, method: req.Method, offered: offered}
	c.mu.Unlock()

	event := ports.ChatEvent{
		Kind:           ports.ChatEventApprovalRequested,
		ProviderItemID: requestID,
		RequestID:      requestID,
		ActivityKind:   kind,
		ActivityStatus: domain.ActivityStatusPending,
		Summary:        summary,
		Detail:         detail,
		Decisions:      decisions,
	}
	if req.Method == codexproto.MethodItemToolRequestUserInput {
		event.Kind = ports.ChatEventInputRequested
		event.Input = codexInputRequest(summary, pQuestions(req.Params))
	}
	c.emit(event)

	select {
	case decision := <-ch:
		return approvalReply(req.Method, decision), nil

	case <-time.After(approvalWait):
		c.discardPending(requestID)
		return nil, errors.New("approval timed out without a decision")

	case <-ctx.Done():
		c.discardPending(requestID)
		return nil, ctx.Err()
	}
}

// reportAuthRefreshRequest surfaces the provider asking for ChatGPT credentials and
// returns the refusal to send back.
//
// Kennel does not hold provider credentials. Codex owns its own ChatGPT OAuth tokens
// (authMode `chatgpt`) and refreshes them itself; this request belongs to authMode
// `chatgptAuthTokens`, where an external host app supplies them, and the generated
// schema marks that mode OpenAI-internal. So the honest answer is an error — but the
// error alone would only reach a log, and what the user needs to know is that the
// session has stopped working for a reason no retry will fix.
//
// NEVER OBSERVED live: reaching it requires the provider to take a 401 while running
// in a mode Kennel does not use, which a test cannot arrange without breaking real auth.
func (c *conversation) reportAuthRefreshRequest(params json.RawMessage) error {
	var p codexproto.ChatgptAuthTokensRefreshParams
	// A payload this build cannot parse still means the same thing: the provider
	// asked for credentials Kennel cannot supply.
	_ = json.Unmarshal(params, &p)

	reason := string(p.Reason)
	if reason == "" {
		reason = "unauthorized"
	}
	c.log.Warn("app-server asked for ChatGPT auth tokens Kennel does not hold", "reason", reason)
	c.emit(ports.ChatEvent{
		Kind: ports.ChatEventAccountChanged,
		Account: &ports.ChatAccount{
			ReauthRequired: true,
			ReauthReason:   reason,
		},
	})
	return fmt.Errorf("%w: this client does not supply ChatGPT auth tokens (reason: %s)",
		ports.ErrChatAuthRequired, reason)
}

// refuseDynamicToolCall declines a request to run a tool Kennel never offered.
//
// `item/tool/call` asks the CLIENT to execute a tool the client declared during
// initialize. Kennel declares none, so a well-behaved provider will never send this and
// one that does is asking Kennel to run something it has no definition for. Refusing is
// the only safe answer: inventing a result would feed the model a fabrication.
func (c *conversation) refuseDynamicToolCall(params json.RawMessage) error {
	var p codexproto.DynamicToolCallParams
	_ = json.Unmarshal(params, &p)
	c.log.Warn("refusing dynamic tool call: Kennel declares no client-side tools",
		"tool", p.Tool, "callId", p.CallID)
	return fmt.Errorf("client declares no tools; %q is not available", p.Tool)
}

// ReloadMCPServers restarts the provider's tool servers and reports their state.
//
// Worth a typed operation because of the failure it addresses: a server that failed
// to start stays failed for the life of the app-server process, so without this the
// only way to recover a tool the agent needs is to throw the conversation away. The
// provider re-announces every server's startup state as notifications afterwards, so
// the returned list is a convenience for the caller that asked, not the only path by
// which Kennel learns the outcome.
func (c *conversation) ReloadMCPServers(ctx context.Context) ([]ports.ChatMCPServer, error) {
	// config/mcpServer/reload takes no params, verified against a live app-server.
	if err := c.conn.request(ctx, codexproto.MethodConfigMcpServerReload, map[string]any{}, nil); err != nil {
		return nil, fmt.Errorf("%s: %w", codexproto.MethodConfigMcpServerReload, err)
	}

	var resp struct {
		Data []struct {
			Name       string `json:"name"`
			AuthStatus string `json:"authStatus"`
		} `json:"data"`
	}
	if err := c.conn.request(ctx, codexproto.MethodMcpServerStatusList, map[string]any{
		// The summary form: the full one returns every tool's JSON Schema, which for a
		// handful of servers is hundreds of kilobytes Kennel would immediately discard.
		"detail":   "summary",
		"threadId": c.threadID,
	}, &resp); err != nil {
		// The reload itself succeeded, and that is the part the caller asked for. The
		// pushed notifications will report the outcome regardless.
		c.log.Debug("mcp server list after reload failed", "error", err)
		return nil, nil
	}

	servers := make([]ports.ChatMCPServer, 0, len(resp.Data))
	for _, entry := range resp.Data {
		if entry.Name == "" {
			continue
		}
		// A server that answers the inventory call is running. Its startup state is
		// reported separately by mcpServer/startupStatus/updated, which is the
		// authoritative source; this list only says who came back.
		servers = append(servers, ports.ChatMCPServer{
			Name:   entry.Name,
			Status: string(codexproto.McpServerStartupStateReady),
		})
	}
	return servers, nil
}

func (c *conversation) discardPending(requestID string) {
	c.mu.Lock()
	delete(c.pending, requestID)
	c.mu.Unlock()
}

// failPendingApprovals unblocks every parked handler when the controller ends.
func (c *conversation) failPendingApprovals() {
	c.mu.Lock()
	pending := c.pending
	c.pending = map[string]*parkedRequest{}
	c.closed = true
	c.mu.Unlock()
	for _, parked := range pending {
		close(parked.ch)
	}
}

// Close releases the controller without touching provider-side history.
func (c *conversation) Close() error {
	c.closeOnce.Do(func() {
		c.failPendingApprovals()
		if c.proc.stop != nil {
			_ = c.proc.stop()
		}
	})
	return nil
}

// approvalPayload is the subset of an approval request Kennel renders.
type approvalPayload struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	ItemID   string `json:"itemId"`
	Command  string `json:"command"`
	Cwd      string `json:"cwd"`
	Reason   string `json:"reason"`
	// AvailableDecisions is authoritative: the provider varies the offered set
	// per request and does not always offer a plain decline, so a client must
	// render from this rather than a fixed set of buttons.
	AvailableDecisions []json.RawMessage `json:"availableDecisions"`
	Questions          []struct {
		ID       string `json:"id"`
		Header   string `json:"header"`
		Question string `json:"question"`
		IsSecret *bool  `json:"isSecret,omitempty"`
		IsOther  *bool  `json:"isOther,omitempty"`
		Options  []struct {
			Label       string `json:"label"`
			Description string `json:"description,omitempty"`
		} `json:"options"`
	} `json:"questions"`
}

// parseApproval extracts the decisions, label, and neutral detail for a request.
func parseApproval(method string, params json.RawMessage) ([]ports.ChatDecisionOption, string, []byte) {
	var p approvalPayload
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, "Approval required", nil
	}

	options := make([]ports.ChatDecisionOption, 0, len(p.AvailableDecisions))
	for _, raw := range p.AvailableDecisions {
		if opt, ok := decisionOption(raw); ok {
			options = append(options, opt)
		}
	}

	summary := "Approval required"
	switch {
	case p.Command != "":
		summary = "Run " + commandSummary(p.Command)
	case method == "item/fileChange/requestApproval":
		summary = "Apply file changes"
	case method == "item/tool/requestUserInput" && len(p.Questions) > 0:
		summary = p.Questions[0].Question
	case p.Reason != "":
		summary = p.Reason
	}

	detail := map[string]any{"method": method}
	if p.Command != "" {
		detail["command"] = unwrapShell(p.Command)
		detail["rawCommand"] = p.Command
	}
	if p.Cwd != "" {
		detail["cwd"] = p.Cwd
	}
	if p.ItemID != "" {
		detail["itemId"] = p.ItemID
	}
	if p.Reason != "" {
		detail["reason"] = p.Reason
	}
	if len(p.Questions) > 0 {
		detail["questions"] = p.Questions
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		encoded = nil
	}
	return options, summary, encoded
}

// pQuestions decodes the provider's complete request_user_input questions for
// the typed event. Detail keeps the same provider fields for exact rendering.
func pQuestions(params json.RawMessage) []struct {
	ID       string `json:"id"`
	Header   string `json:"header"`
	Question string `json:"question"`
	IsSecret *bool  `json:"isSecret,omitempty"`
	IsOther  *bool  `json:"isOther,omitempty"`
	Options  []struct {
		Label       string `json:"label"`
		Description string `json:"description,omitempty"`
	} `json:"options"`
} {
	var payload approvalPayload
	if json.Unmarshal(params, &payload) != nil {
		return nil
	}
	return payload.Questions
}

func codexInputRequest(summary string, questions []struct {
	ID       string `json:"id"`
	Header   string `json:"header"`
	Question string `json:"question"`
	IsSecret *bool  `json:"isSecret,omitempty"`
	IsOther  *bool  `json:"isOther,omitempty"`
	Options  []struct {
		Label       string `json:"label"`
		Description string `json:"description,omitempty"`
	} `json:"options"`
}) *ports.ChatInputRequest {
	properties := make(map[string]any, len(questions))
	required := make([]string, 0, len(questions))
	for _, question := range questions {
		property := map[string]any{"type": "string", "title": question.Header, "description": question.Question}
		if question.IsSecret != nil && *question.IsSecret {
			property["format"] = "password"
		}
		if len(question.Options) > 0 {
			values := make([]string, 0, len(question.Options))
			for _, option := range question.Options {
				values = append(values, option.Label)
			}
			if question.IsOther != nil && *question.IsOther {
				property["examples"] = values
				property["x-kennel-allows-other"] = true
			} else {
				property["enum"] = values
			}
		}
		properties[question.ID] = property
		required = append(required, question.ID)
	}
	return &ports.ChatInputRequest{
		Mode:    ports.ChatInputModeForm,
		Message: summary,
		Schema: map[string]any{
			"type":                 "object",
			"properties":           properties,
			"required":             required,
			"additionalProperties": false,
		},
	}
}

// decisionOption reads one entry of availableDecisions, which is either a plain
// string ("accept") or a single-key object carrying parameters
// ({"acceptWithExecpolicyAmendment": {...}}).
func decisionOption(raw json.RawMessage) (ports.ChatDecisionOption, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ports.ChatDecisionOption{}, false
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		if asString == "" {
			return ports.ChatDecisionOption{}, false
		}
		return ports.ChatDecisionOption{ID: asString, Label: decisionLabel(asString), Raw: raw}, true
	}

	var asObject map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asObject); err != nil || len(asObject) != 1 {
		return ports.ChatDecisionOption{}, false
	}
	for key := range asObject {
		return ports.ChatDecisionOption{ID: key, Label: decisionLabel(key), Raw: raw}, true
	}
	return ports.ChatDecisionOption{}, false
}

// decisionLabel gives known decision ids readable text. An unknown id falls back
// to its own name rather than being hidden, so a new provider decision still
// renders as a usable button.
func decisionLabel(id string) string {
	switch id {
	case "accept":
		return "Approve"
	case "acceptForSession":
		return "Approve for this session"
	case "acceptWithExecpolicyAmendment":
		return "Approve and remember this command"
	case "decline":
		return "Decline"
	case "cancel":
		return "Cancel"
	default:
		return id
	}
}

// approvalReply builds the provider response for a decision. A decision carrying
// Raw is echoed verbatim so the structured forms round-trip exactly.
func approvalReply(method string, decision ports.ChatDecision) any {
	if method == "item/tool/requestUserInput" {
		// User-input replies are answers, not decisions; the raw payload is the
		// whole response body.
		if len(decision.Raw) > 0 {
			return json.RawMessage(decision.Raw)
		}
		return map[string]any{"answers": map[string]any{}}
	}
	if len(decision.Raw) > 0 {
		return map[string]any{"decision": json.RawMessage(decision.Raw)}
	}
	return map[string]any{"decision": decision.ID}
}

// The provider deliberately excludes this internal/experimental notification from
// generated method constants while still exporting its typed payload. Native Stage 1
// opts into it because gpt-5.6-luna Code Mode otherwise has no public command lifecycle.
const methodRawResponseItemCompleted = "rawResponseItem/completed"

type codeModeExecCall struct{ command, cwd string }

var codeModeExecField = regexp.MustCompile(`(?:^|[,({])\s*(cmd|workdir)\s*:\s*("(?:\\.|[^"\\])*")`)

func parseCodeModeExecInput(input string) (codeModeExecCall, bool) {
	if !strings.Contains(input, "tools.exec_command(") || !strings.Contains(input, "text(JSON.stringify(") {
		return codeModeExecCall{}, false
	}
	var call codeModeExecCall
	for _, match := range codeModeExecField.FindAllStringSubmatch(input, -1) {
		value, err := strconv.Unquote(match[2])
		if err != nil {
			return codeModeExecCall{}, false
		}
		switch match[1] {
		case "cmd":
			call.command = value
		case "workdir":
			call.cwd = value
		}
	}
	return call, call.command != ""
}

func rawOutputText(raw *json.RawMessage) string {
	if raw == nil || len(*raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(*raw, &text) == nil {
		return text
	}
	var items []struct{ Type, Text string }
	if json.Unmarshal(*raw, &items) != nil {
		return ""
	}
	var parts []string
	for _, item := range items {
		if item.Type == "input_text" || item.Type == "output_text" {
			parts = append(parts, item.Text)
		}
	}
	return strings.Join(parts, "\n")
}

type codeModeExecResult struct {
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"`
}

func parseCodeModeExecOutput(text string) (codeModeExecResult, bool) {
	// The host prefixes a human line, then emits one JSON object. Decode only the
	// final complete line; never infer success from prose or assistant text.
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		var result codeModeExecResult
		if json.Unmarshal([]byte(strings.TrimSpace(lines[i])), &result) == nil && strings.Contains(lines[i], `"exit_code"`) {
			return result, true
		}
	}
	return codeModeExecResult{}, false
}

func parseDirectExecArguments(raw json.RawMessage) (codeModeExecCall, bool) {
	var args struct {
		Command string `json:"cmd"`
		CWD     string `json:"workdir"`
	}
	if json.Unmarshal(raw, &args) != nil || args.Command == "" {
		return codeModeExecCall{}, false
	}
	return codeModeExecCall{command: args.Command, cwd: args.CWD}, true
}

func (c *conversation) markRawCommand(turnID string, delta int) {
	if turnID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.activeCommands == nil {
		c.activeCommands = make(map[string]int)
	}
	if delta > 0 {
		c.activeCommands[turnID] += delta
	} else if c.activeCommands[turnID] > 1 {
		c.activeCommands[turnID]--
	} else {
		delete(c.activeCommands, turnID)
	}
}

func (c *conversation) normalizeRawExec(n notification) {
	if n.Method != methodRawResponseItemCompleted {
		return
	}
	var p codexproto.RawResponseItemCompletedNotification
	if json.Unmarshal(n.Params, &p) != nil || p.Item.CallID == nil {
		return
	}
	callID := *p.Item.CallID
	switch p.Item.Type {
	case codexproto.ResponseItemTypeCustomToolCall:
		if p.Item.Name == nil || *p.Item.Name != "exec" || p.Item.Input == nil {
			return
		}
		if call, ok := parseCodeModeExecInput(*p.Item.Input); ok {
			c.rawCodeModeExec[callID] = call
			c.markRawCommand(p.TurnID, 1)
		}
	case codexproto.ResponseItemTypeFunctionCall:
		if p.Item.Name == nil || *p.Item.Name != "exec_command" {
			return
		}
		if call, ok := parseDirectExecArguments(p.Item.Arguments); ok {
			c.rawCodeModeExec[callID] = call
			c.markRawCommand(p.TurnID, 1)
		}
	case codexproto.ResponseItemTypeCustomToolCallOutput, codexproto.ResponseItemTypeFunctionCallOutput:
		call, ok := c.rawCodeModeExec[callID]
		if !ok {
			return
		}
		delete(c.rawCodeModeExec, callID)
		c.markRawCommand(p.TurnID, -1)
		result, ok := parseCodeModeExecOutput(rawOutputText(p.Item.Output))
		if !ok {
			return
		}
		status := domain.ActivityStatusCompleted
		if result.ExitCode != 0 {
			status = domain.ActivityStatusFailed
		}
		c.rawExecCompleted[callID] = ports.ChatEvent{Kind: ports.ChatEventActivityCompleted, ProviderTurnID: p.TurnID, ProviderItemID: callID, ActivityKind: domain.ActivityKindCommand, ActivityStatus: status, Summary: commandSummary(call.command), Detail: encodeDetail(map[string]any{"command": call.command, "cwd": call.cwd, "output": result.Output, "exitCode": result.ExitCode, "source": "rawResponseItem/completed:exec-fallback"})}
	}
}
