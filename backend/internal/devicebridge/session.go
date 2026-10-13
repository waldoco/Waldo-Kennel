package devicebridge

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"
	"unicode/utf8"
)

var ErrTerminalFrame = errors.New("terminal device bridge frame rejection")

// No contract outcome is defined for expired accepted-unresulted recovery.
// Preserve the journal and stop locally until that decision is resolved.
var ErrExpiredRecovery = errors.New("expired accepted command requires recovery decision")

// Executor is the separate S4 boundary. It returns only the class-specific
// result payload. Reconcile must inspect durable evidence and never blindly
// repeat an effect; uncertain notification delivery becomes delivery_unknown.
// Methods must honor cancellation; heartbeat failure cancels active work.
// No query or notification handlers are supplied by S3.
type Executor interface {
	Handle(context.Context, Scope, map[string]any) (map[string]any, error)
	Reconcile(context.Context, Scope, map[string]any) (map[string]any, error)
}
type Dialer interface {
	Dial(context.Context, string, http.Header) (Socket, error)
}
type ConnectionState string

const (
	ConnectionOffline  ConnectionState = "offline"
	ConnectionOnline   ConnectionState = "online"
	ConnectionUnpaired ConnectionState = "unpaired"
)

type Client struct {
	Origin       string
	Scope        Scope
	Capabilities []string
	Key          ed25519.PrivateKey
	Store        SessionStore
	Executor     Executor
	Dialer       Dialer
	// State projects transport truth only; it grants no owner authority.
	State func(ConnectionState)
	// Diagnostic exposes a local reason only, never a wire response.
	Diagnostic     func(string)
	now            func() time.Time
	heartbeatTicks <-chan time.Time
}

func (c *Client) clock() time.Time {
	if c.now != nil {
		return c.now().UTC()
	}
	return time.Now().UTC()
}
func (c *Client) state(s ConnectionState) {
	if c.State != nil {
		c.State(s)
	}
}
func (c *Client) valid() bool {
	return c != nil && c.Scope.valid() && validCapabilities(c.Capabilities) && len(c.Key) == ed25519.PrivateKeySize && c.Store != nil && c.Executor != nil
}

// Connect performs one outbound connection. Its caller controls retry; neither
// authentication rejection nor ambiguous pairing is automatically retried.
func (c *Client) Connect(ctx context.Context) error {
	if !c.valid() {
		return ErrInvalidSigningInput
	}
	c.state(ConnectionOffline)
	nonce, err := NewNonce(nil)
	if err != nil {
		return err
	}
	target, headers, err := SignedConnect(c.Origin, c.Capabilities, c.Key, c.clock(), nonce, c.Scope.DeviceID)
	if err != nil {
		return err
	}
	dialer := c.Dialer
	if dialer == nil {
		dialer = WebSocketDialer{}
	}
	socket, err := dialer.Dial(ctx, target, headers)
	if err != nil {
		if errors.Is(err, ErrAuthenticationRejected) {
			if clearErr := c.Store.ClearPaired(ctx, c.Scope); clearErr != nil {
				return clearErr
			}
			c.state(ConnectionUnpaired)
		}
		return err
	}
	defer socket.Close()
	return c.RunConnection(ctx, socket)
}

// RunConnection requires an authenticated TLS socket. Execution and recovery run
// off the read loop; acknowledgements, receipts and heartbeats remain responsive.
func (c *Client) RunConnection(ctx context.Context, socket Socket) (runErr error) {
	if !c.valid() || socket == nil {
		return ErrInvalidSigningInput
	}
	ctx, cancel := context.WithCancel(ctx)
	defer socket.Close()
	socket = &serializedSocket{Socket: socket}
	heartbeatErrors := make(chan error, 1)
	eventPulse := make(chan struct{}, 1)
	ticks := c.heartbeatTicks
	var ticker *time.Ticker
	if ticks == nil {
		ticker = time.NewTicker(time.Duration(HeartbeatSeconds) * time.Second)
		ticks = ticker.C
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticks:
				select {
				case eventPulse <- struct{}{}:
				default:
				}
				if err := c.heartbeat(ctx, socket); err != nil {
					heartbeatErrors <- err
					cancel()
					return
				}
			}
		}
	}()
	defer func() {
		cancel()
		if ticker != nil {
			ticker.Stop()
		}
		<-done
		select {
		case err := <-heartbeatErrors:
			runErr = err
		default:
		}
	}()
	defer c.state(ConnectionOffline)
	if err := c.heartbeat(ctx, socket); err != nil {
		return err
	}
	c.state(ConnectionOnline)
	sent := newDrainIdentities()
	if err := c.spoolSessionEvents(ctx); err != nil {
		return err
	}
	if err := c.drainUnique(ctx, socket, sent); err != nil {
		return err
	}
	commands, err := c.Store.AcceptedUnresulted(ctx, c.Scope)
	if err != nil {
		return err
	}
	type completion struct{ err error }
	completed := make(chan completion, 1)
	var active string
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	dispatch := func(raw []byte, command map[string]any, reconcile bool) error {
		id := command["idempotency_key"].(string)
		if active == id {
			return nil
		} // A redelivery never repeats active work.
		if active != "" {
			return ErrTerminalFrame
		}
		active = id
		workers.Add(1)
		go func() {
			defer workers.Done()
			var payload map[string]any
			var err error
			if reconcile {
				payload, err = c.Executor.Reconcile(ctx, c.Scope, command)
			} else {
				payload, err = c.Executor.Handle(ctx, c.Scope, command)
			}
			if err == nil {
				err = c.persistResult(ctx, raw, command, payload)
			}
			completed <- completion{err}
		}()
		return nil
	}
	if len(commands) > 1 {
		return ErrTerminalFrame
	}
	for _, raw := range commands {
		command, err := ParseBackendFrame(raw)
		if err != nil || command["type"] != TypeCommand || !c.Scope.matches(command) || !c.declares(command["class"]) {
			return ErrTerminalFrame
		}
		expires, _ := parseInteger(command["expires_at"])
		if expires <= c.clock().Unix() && !newCommandClass(command["class"]) {
			return ErrExpiredRecovery
		}
		// New Mac commands reconcile durable started evidence before expiry:
		// started without a result is unknown, never an invitation to rerun.
		if err = dispatch(raw, command, true); err != nil {
			return err
		}
	}
	type incoming struct {
		raw []byte
		err error
	}
	reads := make(chan incoming)
	go func() {
		for {
			raw, err := socket.Read(ctx)
			select {
			case reads <- incoming{raw, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-eventPulse:
			if err := c.spoolSessionEvents(ctx); err != nil {
				return err
			}
			if err := c.drainUnique(ctx, socket, sent); err != nil {
				return err
			}
		case result := <-completed:
			active = ""
			if result.err != nil {
				return result.err
			}
			if err := c.drainUnique(ctx, socket, sent); err != nil {
				return err
			}
		case read := <-reads:
			if read.err != nil {
				return read.err
			}
			if err := c.processDispatched(ctx, socket, read.raw, sent, dispatch); err != nil {
				return err
			}
		}
	}
}
func (c *Client) heartbeat(ctx context.Context, s Socket) error {
	depth, err := c.Store.Depth(ctx, c.Scope)
	if err != nil {
		return err
	}
	frame, err := NewHeartbeat(c.Scope.DeviceID, c.Scope.OwnerID, c.Capabilities, depth, c.Key, c.clock(), nil)
	if err != nil {
		return err
	}
	return s.Write(ctx, frame)
}

// Deduplicate both durable identities across the two startup drain passes.
// A new connection gets a new set; explicit command redelivery bypasses it.
type drainIdentities struct{ messages, idempotency map[string][sha256.Size]byte }

func newDrainIdentities() *drainIdentities {
	return &drainIdentities{map[string][sha256.Size]byte{}, map[string][sha256.Size]byte{}}
}
func (c *Client) drain(ctx context.Context, socket Socket) error {
	return c.drainUnique(ctx, socket, newDrainIdentities())
}
func (c *Client) drainUnique(ctx context.Context, socket Socket, sent *drainIdentities) error {
	rows, err := c.Store.Pending(ctx, c.Scope)
	if err != nil {
		return err
	}
	var previous int64
	for _, row := range rows {
		if row.Seq <= previous {
			return ErrTerminalFrame
		}
		previous = row.Seq
		frame, err := parseJSONObject(row.Frame)
		if err != nil || !c.Scope.matches(frame) || (frame["type"] != TypeResult && frame["type"] != TypeSessionEvent) {
			return ErrTerminalFrame
		}
		fingerprint, err := LogicalFingerprint(frame)
		if err != nil {
			return errors.Join(ErrTerminalFrame, err)
		}
		message := frame["message_id"].(string)
		idem, _ := frame["idempotency_key"].(string)
		if frame["type"] == TypeSessionEvent {
			idem = message
		}
		oldMessage, hasMessage := sent.messages[message]
		oldIdem, hasIdem := sent.idempotency[idem]
		if (hasMessage && oldMessage != fingerprint) || (hasIdem && oldIdem != fingerprint) {
			return ErrTerminalFrame
		}
		if hasMessage || hasIdem {
			continue
		}
		if err := c.sendResult(ctx, socket, row.Frame); err != nil {
			return err
		}
		sent.messages[message] = fingerprint
		sent.idempotency[idem] = fingerprint
	}
	return nil
}
func (c *Client) sendResult(ctx context.Context, socket Socket, raw []byte) error {
	nonce, err := NewNonce(nil)
	if err != nil {
		return err
	}
	wire, err := ResignFrame(raw, c.Key, c.clock(), nonce)
	if err != nil {
		return err
	}
	return socket.Write(ctx, wire)
}
func (c *Client) process(ctx context.Context, s Socket, raw []byte) error {
	return c.processUnique(ctx, s, raw, newDrainIdentities())
}
func (c *Client) processUnique(ctx context.Context, s Socket, raw []byte, sent *drainIdentities) error {
	return c.processDispatched(ctx, s, raw, sent, nil)
}
func (c *Client) processDispatched(ctx context.Context, s Socket, raw []byte, sent *drainIdentities, dispatch func([]byte, map[string]any, bool) error) error {
	frame, reason, err := classifyInbound(raw, c.Scope, c.clock())
	if err != nil {
		if errors.Is(err, ErrTerminalFrame) && c.Diagnostic != nil {
			c.Diagnostic(ReasonInvalidShape)
		}
		return err
	}
	if frame["type"] == TypeReceipt {
		// Store must perform binding validation and tombstone/outbox mutation in
		// one transaction; lookup-then-delete here would introduce a receipt race.
		if err := c.Store.ApplyReceipt(ctx, c.Scope, raw, c.clock()); err != nil {
			if errors.Is(err, ErrUnknownReceiptMessage) && c.Diagnostic != nil {
				c.Diagnostic(ReasonUnknownMessage)
			}
			return errors.Join(ErrTerminalFrame, err)
		}
		return nil
	}
	if reason == "" && frame["type"] == TypeCommand && !c.declares(frame["class"]) {
		reason = ReasonUnknownCommand
	}
	if reason != "" {
		state := AckRejected
		if reason == ReasonExpired {
			state = AckExpired
		}
		return c.ack(ctx, s, frame, state, reason)
	}
	admitted, err := c.Store.Admit(ctx, c.Scope, raw, c.clock())
	if err != nil {
		return err
	}
	if admitted.State != AckAccepted && admitted.State != AckRejected && admitted.State != AckExpired {
		return ErrTerminalFrame
	}
	if err = c.ack(ctx, s, frame, admitted.State, admitted.Reason); err != nil {
		return err
	}
	if admitted.State != AckAccepted {
		return nil
	}
	if !admitted.New {
		result, exists, err := c.Store.ResultForCommand(ctx, c.Scope, raw)
		if err != nil {
			return err
		}
		if exists {
			parsed, err := parseJSONObject(result)
			if err != nil || ValidateResultForCommand(parsed, frame) != nil {
				return ErrTerminalFrame
			}
			return c.sendResult(ctx, s, result)
		}
		if dispatch != nil {
			return dispatch(raw, frame, true)
		}
		payload, err := c.Executor.Reconcile(ctx, c.Scope, frame)
		if err != nil {
			return err
		}
		if err := c.persistResult(ctx, raw, frame, payload); err != nil {
			return err
		}
		return c.drainUnique(ctx, s, sent)
	}
	if dispatch != nil {
		return dispatch(raw, frame, false)
	}
	payload, err := c.Executor.Handle(ctx, c.Scope, frame)
	if err != nil {
		return err
	}
	if err = c.persistResult(ctx, raw, frame, payload); err != nil {
		return err
	}
	return c.drainUnique(ctx, s, sent)
}
func (c *Client) ack(ctx context.Context, s Socket, command map[string]any, state, reason string) error {
	frame, err := newAttributedAck(command, state, reason, c.Key, c.clock())
	if err != nil {
		return err
	}
	return s.Write(ctx, frame)
}
func (c *Client) persistResult(ctx context.Context, raw []byte, command, payload map[string]any) error {
	now := c.clock()
	id, err := NewMessageID(now, nil)
	if err != nil {
		return err
	}
	payload, err = canonicalResultPayload(payload)
	if err != nil {
		return err
	}
	frame := map[string]any{"contract_version": ContractVersion, "type": TypeResult, "message_id": id, "payload": payload}
	for _, field := range []string{"device_id", "owner_id", "command_id", "revision", "idempotency_key"} {
		frame[field] = command[field]
	}
	unsigned, err := marshalCanonical(frame)
	if err != nil {
		return err
	}
	nonce, err := NewNonce(nil)
	if err != nil {
		return err
	}
	// Escaping may expand a 6144-byte tail beyond the signed 8192-byte frame.
	// Shrink only output, preserving valid UTF-8 and the complete local log.
	var signed []byte
	for {
		signed, err = SignFrame(unsigned, c.Key, now, nonce)
		if err == nil {
			break
		}
		tailMap := payload
		if answer, ok := payload["answer"].(map[string]any); ok {
			tailMap = answer
		}
		tail, ok := tailMap["output_tail"].(string)
		if !ok || len(tail) == 0 {
			if sessions, ok := tailMap["sessions"].([]any); ok && len(sessions) > 0 {
				tailMap["sessions"] = sessions[:len(sessions)-1]
				tailMap["truncated"] = true
				unsigned, err = marshalCanonical(frame)
				if err != nil {
					return err
				}
				continue
			}
			return err
		}
		_, size := utf8.DecodeRuneInString(tail)
		tailMap["output_tail"] = tail[size:]
		tailMap["truncated"] = true
		unsigned, err = marshalCanonical(frame)
		if err != nil {
			return err
		}
	}
	parsed, err := parseJSONObject(signed)
	if err != nil {
		return err
	}
	if err = ValidateResultForCommand(parsed, command); err != nil {
		return err
	}
	return c.Store.CommitResult(ctx, c.Scope, raw, signed)
}

// classifyInbound separates trustworthy command references from command body
// rejection. Noncanonical, signed backend, direction or binding errors close.
func classifyInbound(raw []byte, scope Scope, now time.Time) (map[string]any, string, error) {
	if len(raw) > FrameMaxBytes {
		return nil, "", ErrTerminalFrame
	}
	frame, err := parseJSONObject(raw)
	canonical, ce := marshalCanonical(frame)
	if err != nil || ce != nil || !bytes.Equal(raw, canonical) {
		return nil, "", ErrTerminalFrame
	}
	for _, field := range []string{"timestamp", "nonce", "signature"} {
		if _, ok := frame[field]; ok {
			return nil, "", ErrTerminalFrame
		}
	}
	if !scope.matches(frame) {
		return nil, "", ErrTerminalFrame
	}
	if frame["type"] == TypeReceipt {
		if validateFrameShape(frame, false) != nil {
			return nil, "", ErrTerminalFrame
		}
		return frame, "", nil
	}
	if frame["type"] != TypeCommand {
		return nil, "", ErrTerminalFrame
	}
	for _, field := range []string{"command_id", "idempotency_key"} {
		if !stringID(frame[field]) {
			return nil, "", ErrTerminalFrame
		}
	}
	if !integerEqual(frame["revision"], CommandRevision) {
		return nil, "", ErrTerminalFrame
	}
	id, ok := frame["message_id"].(string)
	if !ok || !validULID(id) {
		return nil, "", ErrTerminalFrame
	}
	if frame["contract_version"] != ContractVersion {
		return frame, ReasonVersionMismatch, nil
	}
	if !validCommandClass(frame["class"]) {
		return frame, ReasonUnknownCommand, nil
	}
	if validateFrameShape(frame, false) != nil {
		return frame, ReasonInvalidShape, nil
	}
	expires, _ := parseInteger(frame["expires_at"])
	if expires > now.Unix()+commandTTLMax(frame) {
		return frame, ReasonInvalidShape, nil
	}
	if expires <= now.Unix() {
		return frame, ReasonExpired, nil
	}
	return frame, "", nil
}
func newAttributedAck(command map[string]any, state, reason string, key ed25519.PrivateKey, now time.Time) ([]byte, error) {
	id, err := NewMessageID(now, nil)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"state": state}
	if reason != "" {
		payload["reason"] = reason
	}
	frame := map[string]any{"contract_version": ContractVersion, "type": TypeAck, "message_id": id, "payload": payload}
	for _, field := range []string{"device_id", "owner_id", "command_id", "revision", "idempotency_key"} {
		frame[field] = command[field]
	}
	raw, err := marshalCanonical(frame)
	if err != nil {
		return nil, err
	}
	nonce, err := NewNonce(nil)
	if err != nil {
		return nil, err
	}
	return SignFrame(raw, key, now, nonce)
}

// ResultPayloadFailure deliberately has no answer. Evidence absence uses the
// normal answered schema with StateUnknown in the separate S4 handler.
func ResultPayloadFailure(reason string) map[string]any {
	return map[string]any{"status": ResultFailed, "reason": reason}
}

// Serialize heartbeat and result/ack writes without blocking heartbeat during
// handler execution. Store implementations must permit concurrent Depth reads.
type serializedSocket struct {
	Socket
	mu sync.Mutex
}

func (s *serializedSocket) Write(ctx context.Context, raw []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Socket.Write(ctx, raw)
}

func (c *Client) declares(class any) bool {
	for _, capability := range c.Capabilities {
		if class == capability {
			return true
		}
	}
	return false
}

// canonicalResultPayload converts an executor's payload to the wire's JSON value
// model. Executors build results with native Go numbers (an int exit code, or a
// float64 reloaded from a JSON ledger); the canonical encoder accepts only
// json.Number integers, so without this a valid result could never be signed.
// Non-integer numbers and non-JSON values are still refused.
func canonicalResultPayload(payload map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, ErrInvalidSigningInput
	}
	out, err := parseJSONObject(raw)
	if err != nil {
		return nil, ErrInvalidSigningInput
	}
	if _, err := marshalCanonical(out); err != nil {
		return nil, ErrInvalidSigningInput
	}
	return out, nil
}
