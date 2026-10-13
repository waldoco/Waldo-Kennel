package devicebridge

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockSessionStore struct {
	mu         sync.Mutex
	scope      Scope
	events     []string
	pending    []PendingResult
	accepted   [][]byte
	tombstones map[string][]byte
	capacity   bool
	cleared    bool
}

func (m *mockSessionStore) event(scope Scope, event string) error {
	if scope != m.scope {
		return ErrTerminalFrame
	}
	m.events = append(m.events, event)
	return nil
}
func (m *mockSessionStore) Pending(ctx context.Context, s Scope) ([]PendingResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	err := m.event(s, "pending")
	return append([]PendingResult(nil), m.pending...), err
}
func (m *mockSessionStore) Depth(ctx context.Context, s Scope) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	err := m.event(s, "depth")
	return int64(len(m.pending)), err
}
func (m *mockSessionStore) AcceptedUnresulted(ctx context.Context, s Scope) ([][]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	err := m.event(s, "reconcile-list")
	return m.accepted, err
}
func (m *mockSessionStore) ResultForCommand(ctx context.Context, s Scope, raw []byte) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	command, err := ParseBackendFrame(raw)
	if err != nil {
		return nil, false, err
	}
	for _, row := range m.pending {
		result, _ := parseJSONObject(row.Frame)
		if result["command_id"] == command["command_id"] {
			return row.Frame, true, nil
		}
	}
	return nil, false, m.event(s, "result-lookup")
}
func (m *mockSessionStore) Admit(ctx context.Context, s Scope, raw []byte, now time.Time) (Admission, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	err := m.event(s, "admit")
	if m.capacity {
		return Admission{State: AckRejected, Reason: ReasonInvalidShape}, err
	}
	return Admission{State: AckAccepted, New: true}, err
}
func (m *mockSessionStore) CommitResult(ctx context.Context, s Scope, command, result []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.event(s, "commit"); err != nil {
		return err
	}
	seq := int64(1)
	if len(m.pending) > 0 {
		seq = m.pending[len(m.pending)-1].Seq + 1
	}
	m.pending = append(m.pending, PendingResult{seq, result})
	return nil
}

// The mock models one transaction: named result binding, delete, tombstone.
// This tests the client seam only; it is not Gate B persistence proof.
func (m *mockSessionStore) ApplyReceipt(ctx context.Context, s Scope, raw []byte, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.event(s, "receipt"); err != nil {
		return err
	}
	receipt, err := ParseBackendFrame(raw)
	if err != nil {
		return err
	}
	id := receipt["payload"].(map[string]any)["result_message_id"].(string)
	for i, row := range m.pending {
		result, _ := parseJSONObject(row.Frame)
		if result["message_id"] != id {
			continue
		}
		if err := ValidateReceiptForResult(receipt, result); err != nil {
			return err
		}
		if m.tombstones == nil {
			m.tombstones = map[string][]byte{}
		}
		m.tombstones[id] = row.Frame
		m.pending = append(m.pending[:i], m.pending[i+1:]...)
		return nil
	}
	if prior, ok := m.tombstones[id]; ok {
		result, _ := parseJSONObject(prior)
		return ValidateReceiptForResult(receipt, result)
	}
	return ErrUnknownReceiptMessage
}
func (m *mockSessionStore) ClearPaired(ctx context.Context, s Scope) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.event(s, "clear"); err != nil {
		return err
	}
	m.cleared = true
	return nil
}

type mockExecutor struct {
	store   *mockSessionStore
	entered chan struct{}
	block   bool
}

func (e mockExecutor) Handle(ctx context.Context, s Scope, c map[string]any) (map[string]any, error) {
	return e.work(ctx, s, "handle")
}
func (e mockExecutor) Reconcile(ctx context.Context, s Scope, c map[string]any) (map[string]any, error) {
	return e.work(ctx, s, "reconcile")
}
func (e mockExecutor) work(ctx context.Context, s Scope, event string) (map[string]any, error) {
	e.store.mu.Lock()
	err := e.store.event(s, event)
	e.store.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if e.entered != nil {
		close(e.entered)
	}
	if e.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return map[string]any{"status": ResultAnswered, "answer": map[string]any{"query_id": "query_1", "query_kind": QuerySessionStatus, "state": StateUnknown}}, nil
}

type mockSocket struct {
	mu               sync.Mutex
	frames           [][]byte
	reads            [][]byte
	writes           chan []byte
	readHook         func()
	failure          error
	blockRead        bool
	closeAfterWrites int
}

func (s *mockSocket) Read(ctx context.Context) ([]byte, error) {
	if s.readHook != nil {
		s.readHook()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.reads) == 0 {
		if s.closeAfterWrites > 0 {
			s.mu.Unlock()
			deadline := time.NewTimer(time.Second)
			defer deadline.Stop()
			for {
				s.mu.Lock()
				n := len(s.frames)
				s.mu.Unlock()
				if n >= s.closeAfterWrites {
					break
				}
				select {
				case <-ctx.Done():
					s.mu.Lock()
					return nil, ctx.Err()
				case <-deadline.C:
					s.mu.Lock()
					return nil, errors.New("async test result not drained")
				case <-time.After(time.Millisecond):
				}
			}
			s.mu.Lock()
		}

		if s.blockRead {
			s.mu.Unlock()
			<-ctx.Done()
			s.mu.Lock()
			return nil, ctx.Err()
		}
		return nil, io.EOF
	}
	raw := s.reads[0]
	s.reads = s.reads[1:]
	return raw, nil
}
func (s *mockSocket) Write(ctx context.Context, raw []byte) error {
	s.mu.Lock()
	s.frames = append(s.frames, append([]byte(nil), raw...))
	s.mu.Unlock()
	if s.writes != nil {
		select {
		case s.writes <- raw:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.failure
}
func (s *mockSocket) Close() error { return nil }
func sessionFixture(t *testing.T) (*Client, *mockSessionStore) {
	t.Helper()
	scope := Scope{"dev_1", "owner_1"}
	store := &mockSessionStore{scope: scope}
	client := &Client{Scope: scope, Origin: "https://bridge.example", Capabilities: []string{ClassMachineStateQuery}, Key: ed25519.NewKeyFromSeed(bytes.Repeat([]byte{8}, 32)), Store: store, Executor: mockExecutor{store: store}, now: func() time.Time { return time.Unix(1790699999, 0) }}
	return client, store
}
func signedResult(t *testing.T, c *Client) []byte {
	t.Helper()
	var nonce [NonceBytes]byte
	wire, err := SignFrame([]byte(resultFixture), c.Key, c.clock().Add(-time.Hour), nonce)
	if err != nil {
		t.Fatal(err)
	}
	return wire
}

func TestReconnectDrainThenReconcileBeforeNewWork(t *testing.T) {
	c, store := sessionFixture(t)
	original := signedResult(t, c)
	store.pending = []PendingResult{{1, original}}
	store.accepted = [][]byte{[]byte(strings.NewReplacer("cmd_1", "cmd_2", "idem_1", "idem_2", "01ARZ3NDEKTSV4RRFFQ69G5FAV", "01ARZ3NDEKTSV4RRFFQ69G5FB0").Replace(commandFixture))}
	socket := &mockSocket{writes: make(chan []byte, 10)}
	socket.readHook = func() {
		// Recovery now runs off-loop. Keep the socket alive until the queued
		// reconciliation result has been drained, then simulate a disconnect.
		for i := 0; i < 3; i++ {
			select {
			case <-socket.writes:
			case <-time.After(time.Second):
				t.Error("missing startup result")
				return
			}
		}
	}

	if err := c.RunConnection(context.Background(), socket); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	if len(socket.frames) != 3 {
		t.Fatalf("writes=%d", len(socket.frames))
	}
	first, _ := parseJSONObject(socket.frames[0])
	if first["type"] != TypeHeartbeat {
		t.Fatal("no immediate heartbeat")
	}
	resend, err := VerifyFrame(socket.frames[1], c.Key.Public().(ed25519.PublicKey), c.clock())
	if err != nil {
		t.Fatal(err)
	}
	prior, _ := parseJSONObject(original)
	a, _ := LogicalFingerprint(prior)
	b, _ := LogicalFingerprint(resend)
	if a != b || prior["nonce"] == resend["nonce"] {
		t.Fatal("resend identity/signature freshness failure")
	}
}
func TestHeartbeatWhileExecutorBlocked(t *testing.T) {
	for _, recovering := range []bool{true, false} {
		t.Run(map[bool]string{true: "reconcile", false: "handle"}[recovering], func(t *testing.T) {
			c, store := sessionFixture(t)
			entered := make(chan struct{})
			c.Executor = mockExecutor{store: store, entered: entered, block: true}
			ticks := make(chan time.Time)
			c.heartbeatTicks = ticks
			socket := &mockSocket{writes: make(chan []byte, 10), blockRead: true}
			if recovering {
				store.accepted = [][]byte{[]byte(commandFixture)}
			} else {
				socket.reads = [][]byte{[]byte(commandFixture)}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- c.RunConnection(ctx, socket) }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("executor not entered")
			}
			<-socket.writes
			if !recovering {
				<-socket.writes
			} // volatile accepted ack precedes handler.
			ticks <- c.clock()
			select {
			case raw := <-socket.writes:
				frame, _ := parseJSONObject(raw)
				if frame["type"] != TypeHeartbeat {
					t.Fatal("expected heartbeat")
				}
			case <-time.After(time.Second):
				t.Fatal("heartbeat blocked by executor")
			}
			cancel()
			<-done
		})
	}
}
func TestAttributableRejectionAndTerminalFrames(t *testing.T) {
	for _, tc := range []struct{ name, raw, reason string }{
		{"version", strings.Replace(commandFixture, "0.3.0", "0.1", 1), ReasonVersionMismatch},
		{"payload", strings.Replace(commandFixture, "session_status", "unknown_kind", 1), ReasonInvalidShape},
		{"payload-null", strings.Replace(commandFixture, `{"query_id":"query_1","query_kind":"session_status"}`, `null`, 1), ReasonInvalidShape},
		{"expired", strings.Replace(commandFixture, "1790700000", "1790699998", 1), ReasonExpired},
		{"missing-reference", strings.Replace(commandFixture, `"command_id":"cmd_1",`, "", 1), ""},
		{"cross-owner", strings.Replace(commandFixture, "owner_1", "owner_2", 1), ""},
		{"noncanonical", " " + commandFixture, ""},
		{"direction", resultFixture, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, store := sessionFixture(t)
			socket := &mockSocket{}
			err := c.process(context.Background(), socket, []byte(tc.raw))
			if tc.reason == "" {
				if !errors.Is(err, ErrTerminalFrame) || len(socket.frames) != 0 {
					t.Fatal("terminal frame generated reply")
				}
			} else {
				if err != nil || len(socket.frames) != 1 {
					t.Fatal(err)
				}
				frame, _ := VerifyFrame(socket.frames[0], c.Key.Public().(ed25519.PublicKey), c.clock())
				if frame["payload"].(map[string]any)["reason"] != tc.reason {
					t.Fatal("wrong reason")
				}
			}
			if len(store.events) != 0 {
				t.Fatal("rejection touched store/effects")
			}
		})
	}
}
func TestUndeclaredCapabilityNeverAdmitted(t *testing.T) {
	c, store := sessionFixture(t)
	command, _ := ParseBackendFrame([]byte(commandFixture))
	command["class"] = ClassNotifyLocal
	command["payload"] = map[string]any{"notification_id": "n", "title": "Title", "body": "Body", "severity": "info"}
	raw, _ := marshalCanonical(command)
	socket := &mockSocket{}
	if err := c.process(context.Background(), socket, raw); err != nil {
		t.Fatal(err)
	}
	if len(store.events) != 0 {
		t.Fatal("undeclared capability admitted")
	}
	frame, _ := parseJSONObject(socket.frames[0])
	if frame["payload"].(map[string]any)["reason"] != ReasonUnknownCommand {
		t.Fatal("wrong capability rejection")
	}
}
func TestReceiptAtomicBoundaryDuplicateAndUnknown(t *testing.T) {
	c, store := sessionFixture(t)
	store.pending = []PendingResult{{1, signedResult(t, c)}}
	socket := &mockSocket{}
	for i := 0; i < 2; i++ {
		if err := c.process(context.Background(), socket, []byte(receiptFixture)); err != nil {
			t.Fatal(err)
		}
	}
	if len(store.pending) != 0 || len(store.tombstones) != 1 || len(socket.frames) != 0 {
		t.Fatal("receipt lifecycle invalid")
	}
	bad := strings.Replace(receiptFixture, "01ARZ3NDEKTSV4RRFFQ69G5FAX", "01ARZ3NDEKTSV4RRFFQ69G5FAZ", 1)
	if err := c.process(context.Background(), socket, []byte(bad)); !errors.Is(err, ErrTerminalFrame) {
		t.Fatal("unknown receipt not terminal")
	}
}
func TestTombstoneCapacityRejectsNewCommand(t *testing.T) {
	c, store := sessionFixture(t)
	store.capacity = true
	socket := &mockSocket{}
	if err := c.process(context.Background(), socket, []byte(commandFixture)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(store.events, []string{"admit"}) {
		t.Fatal("capacity rejection produced effect")
	}
	frame, _ := parseJSONObject(socket.frames[0])
	if frame["payload"].(map[string]any)["reason"] != ReasonInvalidShape {
		t.Fatal("wrong capacity reason")
	}
}
func TestQueryFailureDistinctFromUnknown(t *testing.T) {
	c, _ := sessionFixture(t)
	command, _ := ParseBackendFrame([]byte(commandFixture))
	for _, reason := range []string{ReasonProcessingFailed, ReasonDeliveryUnknown} {
		frame := map[string]any{"contract_version": ContractVersion, "type": TypeResult, "message_id": "01ARZ3NDEKTSV4RRFFQ69G5FAX", "payload": ResultPayloadFailure(reason)}
		for _, field := range []string{"device_id", "owner_id", "command_id", "revision", "idempotency_key"} {
			frame[field] = command[field]
		}
		unsigned, _ := marshalCanonical(frame)
		signed, _ := SignFrame(unsigned, c.Key, c.clock(), [NonceBytes]byte{})
		parsed, _ := parseJSONObject(signed)
		err := ValidateResultForCommand(parsed, command)
		if (reason == ReasonProcessingFailed) != (err == nil) {
			t.Fatal("query failure reason crossed classes")
		}
	}
}
