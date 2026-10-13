package devicebridge

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type evidenceFixture struct {
	state string
	found bool
	err   error
	calls int
}

func (e *evidenceFixture) QueryState(ctx context.Context, scope Scope, id, kind string) (string, bool, error) {
	e.calls++
	return e.state, e.found, e.err
}

type displayFixture struct {
	calls     int
	delivered bool
	err       error
	before    func()
}

func (d *displayFixture) Display(ctx context.Context, scope Scope, n Notification) (bool, error) {
	if d.before != nil {
		d.before()
	}
	d.calls++
	return d.delivered, d.err
}
func s4Fixture(t *testing.T) (*Client, *fileSessionStore, *displayFixture) {
	t.Helper()
	c, _ := sessionFixture(t)
	s, err := openFileSessionStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatal(err)
	}
	d := &displayFixture{delivered: true}
	c.Store = s
	c.Capabilities = []string{ClassMachineStateQuery, ClassNotifyLocal}
	c.Executor = Handlers{Display: d}
	return c, s, d
}
func s4Command(t *testing.T, class string, index int) []byte {
	t.Helper()
	f, _ := ParseBackendFrame([]byte(commandFixture))
	if index > 1 {
		f["command_id"] = fmt.Sprintf("cmd_%d", index)
		f["idempotency_key"] = fmt.Sprintf("idem_%d", index)
		f["message_id"] = fmt.Sprintf("01ARZ3NDEKTSV4RRFFQ69G5F%02d", index)
	}
	if class == ClassNotifyLocal {
		f["class"] = class
		f["payload"] = map[string]any{"notification_id": fmt.Sprintf("notice_%d", index), "title": "Status", "body": "Finished", "severity": SeverityInfo}
	}
	raw, err := marshalCanonical(f)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func s4Receipt(t *testing.T, result []byte) []byte {
	t.Helper()
	r, _ := parseJSONObject(result)
	f, _ := ParseBackendFrame([]byte(receiptFixture))
	for _, k := range []string{"device_id", "owner_id", "command_id", "revision", "idempotency_key"} {
		f[k] = r[k]
	}
	f["payload"].(map[string]any)["result_message_id"] = r["message_id"]
	raw, err := marshalCanonical(f)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func s4ResultFrames(t *testing.T, c *Client, s *mockSocket) []map[string]any {
	t.Helper()
	var frames []map[string]any
	for _, raw := range s.frames {
		f, err := VerifyFrame(raw, c.Key.Public().(ed25519.PublicKey), c.clock())
		if err != nil {
			t.Fatal(err)
		}
		if f["type"] == TypeResult {
			frames = append(frames, f)
		}
	}
	return frames
}

// Each of the seven named entry points is a separately runnable matrix row.
func TestS4MatrixDurableAcceptance(t *testing.T) {
	c, s, d := s4Fixture(t)
	raw := s4Command(t, ClassNotifyLocal, 1)
	d.before = func() {
		fresh, err := openFileSessionStore(s.path)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := fresh.AcceptedUnresulted(context.Background(), c.Scope)
		if err != nil || len(rows) != 1 {
			t.Fatal("effect preceded persisted acceptance", err)
		}
	}
	if err := c.process(context.Background(), &mockSocket{}, raw); err != nil {
		t.Fatal(err)
	}
	if d.calls != 1 {
		t.Fatal("missing display")
	}
	runS4Restart(t, s.path, "completed")
	c, s, _ = s4Fixture(t)
	if _, err := s.Admit(context.Background(), c.Scope, raw, c.clock()); err != nil {
		t.Fatal(err)
	}
	runS4Restart(t, s.path, "interrupted")
}
func TestS4MatrixDuplicateDelivery(t *testing.T) {
	c, s, d := s4Fixture(t)
	raw := s4Command(t, ClassNotifyLocal, 1)
	sock := &mockSocket{}
	if err := c.process(context.Background(), sock, raw); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.Pending(context.Background(), c.Scope)
	original := rows[0].Frame
	if err := c.process(context.Background(), sock, s4Receipt(t, original)); err != nil {
		t.Fatal(err)
	}
	f, _ := ParseBackendFrame(raw)
	f["message_id"] = "01ARZ3NDEKTSV4RRFFQ69G5FB0"
	redelivery := s4Marshal(t, f)
	if err := c.process(context.Background(), sock, redelivery); err != nil {
		t.Fatal(err)
	}
	results := s4ResultFrames(t, c, sock)
	if d.calls != 1 || len(results) != 2 {
		t.Fatalf("effects=%d results=%d", d.calls, len(results))
	}
	a, _ := LogicalFingerprint(results[0])
	b, _ := LogicalFingerprint(results[1])
	if a != b || results[0]["nonce"] == results[1]["nonce"] {
		t.Fatal("result lost identity or fresh signing")
	}
	depth, _ := s.Depth(context.Background(), c.Scope)
	if depth != 0 {
		t.Fatal("redelivery resurrected outbox")
	}
	t.Run("unresulted-reconciles-without-display", func(t *testing.T) {
		c, s, d := s4Fixture(t)
		_, err := s.Admit(context.Background(), c.Scope, raw, c.clock())
		if err != nil {
			t.Fatal(err)
		}
		sock := &mockSocket{}
		if err = c.process(context.Background(), sock, raw); err != nil {
			t.Fatal(err)
		}
		rs := s4ResultFrames(t, c, sock)
		if d.calls != 0 || rs[0]["payload"].(map[string]any)["reason"] != ReasonDeliveryUnknown {
			t.Fatal("unknown display repeated")
		}
	})
	t.Run("conflicting-payload-rejected", func(t *testing.T) {
		f["payload"].(map[string]any)["body"] = "Changed"
		conflict := s4Marshal(t, f)
		if err := c.process(context.Background(), sock, conflict); err != nil {
			t.Fatal(err)
		}
		last, _ := parseJSONObject(sock.frames[len(sock.frames)-1])
		if last["payload"].(map[string]any)["reason"] != ReasonIdempotencyConflict || d.calls != 1 {
			t.Fatal("conflicting duplicate effected")
		}
	})
}
func TestS4MatrixRestartRecovery(t *testing.T) {
	c, s, _ := s4Fixture(t)
	if err := c.process(context.Background(), &mockSocket{}, s4Command(t, ClassNotifyLocal, 1)); err != nil {
		t.Fatal(err)
	}
	runS4Restart(t, s.path, "completed")
	c, s, _ = s4Fixture(t)
	_, err := s.Admit(context.Background(), c.Scope, s4Command(t, ClassNotifyLocal, 1), c.clock())
	if err != nil {
		t.Fatal(err)
	}
	runS4Restart(t, s.path, "interrupted")
}
func runS4Restart(t *testing.T, path, mode string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestS4FreshProcessHelper$", "-test.v")
	cmd.Env = append(os.Environ(), "K0_S4_FAKE_PATH="+path, "K0_S4_FAKE_MODE="+mode)
	output, err := cmd.CombinedOutput()
	t.Logf("fresh-process %s: %s", mode, output)
	if err != nil {
		t.Fatal(err)
	}
}
func TestS4FreshProcessHelper(t *testing.T) {
	path := os.Getenv("K0_S4_FAKE_PATH")
	if path == "" {
		return
	}
	s, err := openFileSessionStore(path)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := sessionFixture(t)
	d := &displayFixture{delivered: true}
	c.Store = s
	c.Executor = Handlers{Display: d}
	c.Capabilities = []string{ClassMachineStateQuery, ClassNotifyLocal}
	rows, _ := s.Pending(context.Background(), c.Scope)
	mode := os.Getenv("K0_S4_FAKE_MODE")
	if mode == "completed" && len(rows) != 1 {
		t.Fatal("restart lost outbox")
	}
	if mode == "interrupted" {
		unresulted, _ := s.AcceptedUnresulted(context.Background(), c.Scope)
		if len(unresulted) != 1 {
			t.Fatal("restart lost acceptance")
		}
	}
	sock := &mockSocket{closeAfterWrites: 2}
	if err := c.RunConnection(context.Background(), sock); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	results := s4ResultFrames(t, c, sock)
	if len(results) != 1 || d.calls != 0 {
		t.Fatal("restart duplicated effect/result")
	}
	if mode == "interrupted" && results[0]["payload"].(map[string]any)["reason"] != ReasonDeliveryUnknown {
		t.Fatal("interrupted display assumed delivered")
	}
}
func TestS4MatrixOfflineDrain(t *testing.T) {
	c, s, _ := s4Fixture(t)
	for _, i := range []int{1, 2, 3} {
		if err := c.process(context.Background(), &mockSocket{}, s4Command(t, ClassNotifyLocal, i)); err != nil {
			t.Fatal(err)
		}
	}
	// Offline beyond expiry + replay horizon does not discard pending results.
	c.now = func() time.Time { return time.Unix(1790700400, 0) }
	sock := &mockSocket{}
	if err := c.RunConnection(context.Background(), sock); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	results := s4ResultFrames(t, c, sock)
	if len(results) != 3 {
		t.Fatalf("drained %d", len(results))
	}
	for i, f := range results {
		if f["command_id"] != fmt.Sprintf("cmd_%d", i+1) {
			t.Fatal("drain order")
		}
	}
	depth, _ := s.Depth(context.Background(), c.Scope)
	if depth != 3 {
		t.Fatal("send treated as receipt")
	}
	t.Run("duplicate-rows-dedup", func(t *testing.T) {
		mc, ms := sessionFixture(t)
		r := signedResult(t, mc)
		ms.pending = []PendingResult{{1, r}, {2, r}}
		sock := &mockSocket{}
		if err := mc.drain(context.Background(), sock); err != nil {
			t.Fatal(err)
		}
		if len(sock.frames) != 1 {
			t.Fatal("duplicate write")
		}
	})
	t.Run("conflicting-rows-close", func(t *testing.T) {
		mc, ms := sessionFixture(t)
		r := signedResult(t, mc)
		f, _ := parseJSONObject(r)
		f["payload"].(map[string]any)["answer"].(map[string]any)["state"] = StateUnknown
		changed := s4Marshal(t, f)
		ms.pending = []PendingResult{{1, r}, {2, changed}}
		if err := mc.drain(context.Background(), &mockSocket{}); !errors.Is(err, ErrTerminalFrame) {
			t.Fatal("conflict drained")
		}
	})
}
func TestS4MatrixReceiptApplication(t *testing.T) {
	c, s, _ := s4Fixture(t)
	if err := c.process(context.Background(), &mockSocket{}, s4Command(t, ClassNotifyLocal, 1)); err != nil {
		t.Fatal(err)
	}
	rows, _ := s.Pending(context.Background(), c.Scope)
	receipt := s4Receipt(t, rows[0].Frame)
	sock := &mockSocket{}
	for _, field := range []string{"device_id", "owner_id", "command_id", "revision", "idempotency_key"} {
		t.Run(field, func(t *testing.T) {
			f, _ := ParseBackendFrame(receipt)
			if field == "revision" {
				f[field] = json.Number("2")
			} else {
				f[field] = "different"
			}
			bad := s4Marshal(t, f)
			if err := c.process(context.Background(), sock, bad); !errors.Is(err, ErrTerminalFrame) {
				t.Fatal("widened receipt accepted")
			}
			depth, _ := s.Depth(context.Background(), c.Scope)
			if depth != 1 || len(s.state.Tombstones) != 0 {
				t.Fatal("bad receipt mutated")
			}
		})
	}
	s.failCommit = true
	if err := c.process(context.Background(), sock, receipt); err == nil {
		t.Fatal("transaction failure hidden")
	}
	fresh, _ := openFileSessionStore(s.path)
	depth, _ := fresh.Depth(context.Background(), c.Scope)
	if depth != 1 || len(fresh.state.Tombstones) != 0 {
		t.Fatal("partial receipt transaction")
	}
	s.failCommit = false
	for i := 0; i < 2; i++ {
		if err := c.process(context.Background(), sock, receipt); err != nil {
			t.Fatal(err)
		}
	}
	depth, _ = s.Depth(context.Background(), c.Scope)
	if depth != 0 || len(s.state.Tombstones) != 1 || len(sock.frames) != 0 {
		t.Fatal("receipt lifecycle")
	}
	for _, tomb := range s.state.Tombstones {
		if tomb.RetainUntil != 1790700000+ReplayWindowSeconds {
			t.Fatal("retention boundary")
		}
	}
	fresh, _ = openFileSessionStore(s.path)
	if err := fresh.ApplyReceipt(context.Background(), c.Scope, receipt, c.clock()); err != nil {
		t.Fatal("restart duplicate receipt", err)
	}
	mismatch, _ := ParseBackendFrame(receipt)
	mismatch["idempotency_key"] = "mismatched"
	if err := fresh.ApplyReceipt(context.Background(), c.Scope, s4Marshal(t, mismatch), c.clock()); err == nil {
		t.Fatal("conflicting tombstone duplicate accepted")
	}
	var diagnostics []string
	c.Diagnostic = func(reason string) { diagnostics = append(diagnostics, reason) }
	f, _ := ParseBackendFrame(receipt)
	f["payload"].(map[string]any)["result_message_id"] = "01ARZ3NDEKTSV4RRFFQ69G5FB0"
	bad := s4Marshal(t, f)
	if err := c.process(context.Background(), sock, bad); !errors.Is(err, ErrUnknownReceiptMessage) || !errors.Is(err, ErrTerminalFrame) {
		t.Fatal("unknown receipt")
	}
	if !reflect.DeepEqual(diagnostics, []string{ReasonUnknownMessage}) {
		t.Fatal("missing unknown receipt diagnostic", diagnostics)
	}
}
func TestS4MatrixForgedStaleWidened(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		value       any
		reason      string
	}{{"device", "device_id", "other", ""}, {"owner", "owner_id", "other", ""}, {"revision", "revision", json.Number("2"), ""}, {"class", "class", "shell", ReasonUnknownCommand}, {"ttl", "expires_at", json.Number("1790786400"), ReasonInvalidShape}, {"extra", "additional", true, ReasonInvalidShape}, {"expired", "expires_at", json.Number("1790699998"), ReasonExpired}} {
		t.Run(tc.name, func(t *testing.T) {
			c, s, d := s4Fixture(t)
			f, _ := ParseBackendFrame(s4Command(t, ClassNotifyLocal, 1))
			f[tc.field] = tc.value
			raw := s4Marshal(t, f)
			sock := &mockSocket{}
			err := c.process(context.Background(), sock, raw)
			if tc.reason == "" {
				if !errors.Is(err, ErrTerminalFrame) || len(sock.frames) != 0 {
					t.Fatal("unrouteable reply")
				}
			} else {
				if err != nil || len(sock.frames) != 1 {
					t.Fatal(err)
				}
				f, _ := parseJSONObject(sock.frames[0])
				if f["payload"].(map[string]any)["reason"] != tc.reason {
					t.Fatal("wrong rejection")
				}
			}
			if d.calls != 0 || len(s.state.Commands) != 0 {
				t.Fatal("rejected frame effected")
			}
		})
	}
	t.Run("undeclared", TestUndeclaredCapabilityNeverAdmitted)
	t.Run("expired-redelivery-no-ttl-extension", func(t *testing.T) {
		c, s, d := s4Fixture(t)
		raw := s4Command(t, ClassNotifyLocal, 1)
		if err := c.process(context.Background(), &mockSocket{}, raw); err != nil {
			t.Fatal(err)
		}
		before, _ := json.Marshal(s.state)
		c.now = func() time.Time { return time.Unix(1790700000, 0) }
		sock := &mockSocket{}
		if err := c.process(context.Background(), sock, raw); err != nil {
			t.Fatal(err)
		}
		after, _ := json.Marshal(s.state)
		if string(before) != string(after) || d.calls != 1 || len(sock.frames) != 1 {
			t.Fatal("expired redelivery changed durable state")
		}
		ack, _ := parseJSONObject(sock.frames[0])
		if ack["payload"].(map[string]any)["state"] != AckExpired {
			t.Fatal("expired redelivery not acked")
		}
	})
}
func TestS4MatrixInvalidFrames(t *testing.T) {
	t.Run("existing-boundary", TestAttributableRejectionAndTerminalFrames)
	for _, raw := range []string{"{" + commandFixture[1:], strings.Replace(commandFixture, `"command_id":"cmd_1",`, `"command_id":"cmd_1","command_id":"cmd_2",`, 1), " " + commandFixture} {
		c, s, d := s4Fixture(t)
		if raw == commandFixture {
			raw = "{"
		}
		sock := &mockSocket{}
		var diagnostics []string
		c.Diagnostic = func(reason string) { diagnostics = append(diagnostics, reason) }
		if err := c.process(context.Background(), sock, []byte(raw)); !errors.Is(err, ErrTerminalFrame) || len(sock.frames) != 0 || d.calls != 0 || len(s.state.Commands) != 0 {
			t.Fatal("invalid frame escaped boundary")
		}
		if !reflect.DeepEqual(diagnostics, []string{ReasonInvalidShape}) {
			t.Fatal("missing invalid-frame diagnostic", diagnostics)
		}
	}
}

func TestS4HandlersDurableEvidence(t *testing.T) {
	c, _, _ := s4Fixture(t)
	for _, kind := range []string{QuerySessionStatus, QueryAttemptStatus, QueryWorktreeWatch} {
		for _, tc := range []struct {
			name, state string
			found       bool
			err         error
		}{{"absent", StateRunning, false, nil}, {"present", map[string]string{QuerySessionStatus: StateRunning, QueryAttemptStatus: StateDone, QueryWorktreeWatch: StateWatching}[kind], true, nil}, {"processing", "", false, errors.New("storage unavailable")}, {"invalid-label", "invented", true, nil}} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				command, _ := ParseBackendFrame([]byte(commandFixture))
				command["payload"].(map[string]any)["query_kind"] = kind
				e := &evidenceFixture{state: tc.state, found: tc.found, err: tc.err}
				payload, err := (Handlers{Queries: e}).Handle(context.Background(), c.Scope, command)
				if err != nil {
					t.Fatal(err)
				}
				if tc.err != nil || tc.state == "invented" {
					if !reflect.DeepEqual(payload, ResultPayloadFailure(ReasonProcessingFailed)) {
						t.Fatal(payload)
					}
				} else {
					answer := payload["answer"].(map[string]any)
					want := tc.state
					if !tc.found {
						want = StateUnknown
					}
					if answer["state"] != want || len(answer) != 3 {
						t.Fatal(payload)
					}
				}
			})
		}
	}
}

func TestS4ConnectionDrainBeforeNewCommand(t *testing.T) {
	c, s, _ := s4Fixture(t)
	if err := c.process(context.Background(), &mockSocket{}, s4Command(t, ClassNotifyLocal, 1)); err != nil {
		t.Fatal(err)
	}
	old, _ := s.Pending(context.Background(), c.Scope)
	prior, _ := parseJSONObject(old[0].Frame)
	socket := &mockSocket{reads: [][]byte{s4Command(t, ClassNotifyLocal, 2)}, closeAfterWrites: 4}
	if err := c.RunConnection(context.Background(), socket); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	results := s4ResultFrames(t, c, socket)
	count := 0
	for _, r := range results {
		if r["message_id"] == prior["message_id"] {
			count++
		}
	}
	if count != 1 || len(socket.frames) != 4 {
		t.Fatalf("old result writes=%d total frames=%d", count, len(socket.frames))
	}
}
func TestS4ExpiredRecoveryFencesNewWork(t *testing.T) {
	c, s, d := s4Fixture(t)
	raw := s4Command(t, ClassNotifyLocal, 1)
	if _, err := s.Admit(context.Background(), c.Scope, raw, c.clock()); err != nil {
		t.Fatal(err)
	}
	c.now = func() time.Time { return time.Unix(1790700000, 0) }
	reads := 0
	socket := &mockSocket{readHook: func() { reads++ }, reads: [][]byte{s4Command(t, ClassNotifyLocal, 2)}}
	if err := c.RunConnection(context.Background(), socket); !errors.Is(err, ErrExpiredRecovery) {
		t.Fatal(err)
	}
	accepted, _ := s.AcceptedUnresulted(context.Background(), c.Scope)
	if reads != 0 || d.calls != 0 || len(accepted) != 1 || len(s.state.Outbox) != 0 {
		t.Fatal("expired recovery effected/mutated/read new command")
	}
}
func TestS4CanceledHandlerHasNoDisplay(t *testing.T) {
	c, _, d := s4Fixture(t)
	command, _ := ParseBackendFrame(s4Command(t, ClassNotifyLocal, 1))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Executor.Handle(ctx, c.Scope, command); !errors.Is(err, context.Canceled) || d.calls != 0 {
		t.Fatal("canceled notification displayed")
	}
}

func s4Marshal(t *testing.T, f map[string]any) []byte {
	t.Helper()
	raw, err := marshalCanonical(f)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestS4AtomicResultFailure(t *testing.T) {
	c, s, d := s4Fixture(t)
	raw := s4Command(t, ClassNotifyLocal, 1)
	d.before = func() { s.failCommit = true }
	sock := &mockSocket{}
	if err := c.process(context.Background(), sock, raw); err == nil {
		t.Fatal("result failure hidden")
	}
	fresh, err := openFileSessionStore(s.path)
	if err != nil {
		t.Fatal(err)
	}
	accepted, _ := fresh.AcceptedUnresulted(context.Background(), c.Scope)
	if len(accepted) != 1 || len(fresh.state.Outbox) != 0 || len(s4ResultFrames(t, c, sock)) != 0 {
		t.Fatal("partial result transaction")
	}
	runS4Restart(t, s.path, "interrupted")
	t.Run("admission-failure-no-display", func(t *testing.T) {
		c, s, d := s4Fixture(t)
		s.failCommit = true
		sock := &mockSocket{}
		if err := c.process(context.Background(), sock, raw); err == nil {
			t.Fatal("admission failure hidden")
		}
		if d.calls != 0 || len(sock.frames) != 0 || len(s.state.Commands) != 0 {
			t.Fatal("failed admission effected")
		}
	})
}
