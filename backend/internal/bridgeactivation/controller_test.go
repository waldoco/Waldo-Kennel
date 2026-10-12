package bridgeactivation

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/devicebridge"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// Memory repository is test-only. Its mutex proves the boundary contract in a
// single process, NOT durable SQLite/Gate B or multi-process behavior.
type memoryRepo struct {
	mu           sync.Mutex
	r            Record
	failComplete bool
	failRevoke   bool
	next         int
}

func (m *memoryRepo) Load(_ context.Context, owner string) (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.r, nil
}
func (m *memoryRepo) Reserve(_ context.Context, owner string) (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.r.Attempt != "" || m.r.Device.DeviceID != "" {
		return Record{}, ErrBlocked
	}
	m.next++
	m.r = Record{OwnerID: owner, Attempt: fmt.Sprint(m.next), Phase: Pairing}
	return m.r, nil
}
func (m *memoryRepo) Complete(_ context.Context, owner, attempt string, d domain.DeviceBridgeDevice) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failComplete || owner != m.r.OwnerID || attempt != m.r.Attempt {
		return ErrBlocked
	}
	m.r = Record{OwnerID: owner, Device: d, Phase: Offline}
	return nil
}
func (m *memoryRepo) Block(_ context.Context, owner, attempt string, c *devicebridge.PairingRecoveryError) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if owner != m.r.OwnerID || attempt != m.r.Attempt {
		return ErrBlocked
	}
	m.r.Phase = Recovery
	m.r.Checkpoint = c
	return nil
}

func (m *memoryRepo) Revoke(_ context.Context, owner string, id domain.DeviceBridgeDeviceID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failRevoke {
		return errors.New("revoke persistence failed")
	}
	if m.r.OwnerID != owner || m.r.Device.DeviceID != id {
		return ErrInvalid
	}
	now := time.Now()
	m.r.Device.State = domain.DeviceBridgeStateRevoked
	m.r.Device.RevokedAt = &now
	m.r.Device.UpdatedAt = now
	m.r.Phase = Unpaired
	return nil
}

type pairFunc func(context.Context, devicebridge.PairRequest) (domain.DeviceBridgeDevice, error)

func (f pairFunc) Pair(c context.Context, r devicebridge.PairRequest) (domain.DeviceBridgeDevice, error) {
	return f(c, r)
}
func code() string { return base64.RawURLEncoding.EncodeToString(make([]byte, 32)) }
func device() domain.DeviceBridgeDevice {
	now := time.Now()
	return domain.DeviceBridgeDevice{DeviceID: "device-1", OwnerID: "owner-1", Label: "Mac", PublicKey: base64.RawURLEncoding.EncodeToString(make([]byte, 32)), KeyCustodyRef: "opaque-ref", ContractVersion: devicebridge.ContractVersion, CapabilityClasses: []string{"machine_state_query"}, State: domain.DeviceBridgeStatePaired, CreatedAt: now, UpdatedAt: now, PairedAt: &now}
}
func newController(t *testing.T, m *memoryRepo, p Pairer, f Factory) *Controller {
	t.Helper()
	if f == nil {
		f = pairingReadyFactory{}
	}
	c, e := New("owner-1", "https://backend.test", m, p, f)
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func goodPair(_ context.Context, r devicebridge.PairRequest) (domain.DeviceBridgeDevice, error) {
	return device(), nil
}

func TestReservationConcurrentControllers(t *testing.T) {
	m := &memoryRepo{}
	var calls atomic.Int32
	p := pairFunc(func(context.Context, devicebridge.PairRequest) (domain.DeviceBridgeDevice, error) {
		calls.Add(1)
		return device(), nil
	})
	var wg sync.WaitGroup
	var wins atomic.Int32
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := newController(t, m, p, nil)
			if c.Pair(context.Background(), code(), "Mac", []string{"machine_state_query"}) == nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 || wins.Load() != 1 {
		t.Fatalf("calls=%d wins=%d", calls.Load(), wins.Load())
	}
}
func TestAmbiguousReturnRetainedNoSecrets(t *testing.T) {
	m := &memoryRepo{}
	p := pairFunc(func(context.Context, devicebridge.PairRequest) (domain.DeviceBridgeDevice, error) {
		return domain.DeviceBridgeDevice{}, &devicebridge.PairingRecoveryError{OwnerID: "owner-1", PublicKey: "public", KeyCustodyRef: "opaque", Cause: errors.New("secret code")}
	})
	c := newController(t, m, p, nil)
	if e := c.Pair(context.Background(), code(), "Mac", []string{"machine_state_query"}); e != ErrBlocked {
		t.Fatal(e)
	}
	restarted := newController(t, m, p, nil)
	if e := restarted.Pair(context.Background(), code(), "Mac", []string{"machine_state_query"}); e != ErrBlocked {
		t.Fatal(e)
	}
	r, _ := m.Load(context.Background(), "owner-1")
	if r.Checkpoint == nil || r.Checkpoint.Cause != nil {
		t.Fatal("unsafe/missing checkpoint")
	}
	s, _ := restarted.Status(context.Background())
	if s.Phase != Recovery || s.DeviceID != "" {
		t.Fatal(s)
	}
}
func TestInterruptedReservationBlocksRestart(t *testing.T) {
	m := &memoryRepo{}
	_, _ = m.Reserve(context.Background(), "owner-1")
	c := newController(t, m, pairFunc(goodPair), nil)
	if e := c.Start(context.Background()); e != ErrBlocked {
		t.Fatal(e)
	}
	if e := c.Pair(context.Background(), code(), "Mac", []string{"machine_state_query"}); e != ErrBlocked {
		t.Fatal(e)
	}
}
func TestBadIdentityAndCompletionFailureBlock(t *testing.T) {
	for _, bad := range []bool{false, true} {
		m := &memoryRepo{failComplete: !bad}
		p := pairFunc(func(context.Context, devicebridge.PairRequest) (domain.DeviceBridgeDevice, error) {
			d := device()
			if bad {
				d.OwnerID = "other"
			}
			return d, nil
		})
		c := newController(t, m, p, nil)
		if e := c.Pair(context.Background(), code(), "Mac", []string{"machine_state_query"}); e != ErrBlocked {
			t.Fatal(e)
		}
		r, _ := m.Load(context.Background(), "owner-1")
		if r.Attempt == "" {
			t.Fatal("lost reservation")
		}
	}
}
func TestInvalidDoesNotReserve(t *testing.T) {
	m := &memoryRepo{}
	c := newController(t, m, pairFunc(goodPair), nil)
	for _, classes := range [][]string{nil, {"approve"}, {"notify_local", "machine_state_query"}} {
		if c.Pair(context.Background(), code(), "Mac", classes) != ErrInvalid {
			t.Fatal(classes)
		}
	}
	if m.next != 0 {
		t.Fatal("invalid request reserved")
	}
}

type sessionFunc func(context.Context) error

func (f sessionFunc) Run(c context.Context) error { return f(c) }

type factory struct {
	ready   bool
	calls   int
	started chan struct{}
}

func (f *factory) Ready(context.Context) error {
	if !f.ready {
		return ErrNotReady
	}
	return nil
}
func (f *factory) New(_ context.Context, origin string, _ domain.DeviceBridgeDevice, state func(devicebridge.ConnectionState)) (Session, error) {
	if origin != "https://backend.test" {
		return nil, ErrInvalid
	}
	f.calls++
	state(devicebridge.ConnectionOffline)
	return sessionFunc(func(ctx context.Context) error {
		state(devicebridge.ConnectionOnline)
		close(f.started)
		<-ctx.Done()
		return ctx.Err()
	}), nil
}
func TestReadinessAndLifecycle(t *testing.T) {
	m := &memoryRepo{r: Record{OwnerID: "owner-1", Device: device()}}
	f := &factory{started: make(chan struct{})}
	c := newController(t, m, pairFunc(goodPair), f)
	if e := c.Start(context.Background()); e != ErrNotReady || f.calls != 0 {
		t.Fatal(e)
	}
	f.ready = true
	if e := c.Start(context.Background()); e != nil {
		t.Fatal(e)
	}
	<-f.started
	deadline := time.Now().Add(time.Second)
	for {
		s, _ := c.Status(context.Background())
		if s.Phase == Online {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(s)
		}
		time.Sleep(time.Millisecond)
	}
	if e := c.Start(context.Background()); e != ErrBlocked {
		t.Fatal(e)
	}
	if e := c.Stop(context.Background()); e != nil {
		t.Fatal(e)
	}
	s, _ := c.Status(context.Background())
	if s.Phase != Offline {
		t.Fatal(s)
	}
}

func TestHTTPAuthStrictDecodeAndRedaction(t *testing.T) {
	m := &memoryRepo{}
	c := newController(t, m, pairFunc(goodPair), nil)
	h := Handler(c, func(r *http.Request) bool { return r.Header.Get("Authorization") == "local-test" })
	cases := []struct {
		body, addr, auth string
		want             int
	}{
		{`{"code":"secret","code":"duplicate","label":"Mac","capabilities":["machine_state_query"]}`, "127.0.0.1:42", "local-test", 400},
		{`{"code":"secret","label":"Mac","capabilities":["machine_state_query"],"owner_id":"other"}`, "127.0.0.1:42", "local-test", 400},
		{`{}`, "10.0.0.1:42", "local-test", 401}, {`{}`, "127.0.0.1:42", "", 401},
		{`{"code":"secret","label":"Mac","capabilities":["approve"]}`, "127.0.0.1:42", "local-test", 400},
	}
	for _, tc := range cases {
		r := httptest.NewRequest("POST", "/pair", strings.NewReader(tc.body))
		r.RemoteAddr = tc.addr
		r.Header.Set("Authorization", tc.auth)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	if m.next != 0 {
		t.Fatal("bad API input reserved")
	}
	body := fmt.Sprintf(`{"code":%q,"label":"Mac","capabilities":["machine_state_query"]}`, code())
	r := httptest.NewRequest("POST", "/pair", strings.NewReader(body))
	r.RemoteAddr = "[::1]:42"
	r.Header.Set("Authorization", "local-test")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestPairStatusAndPublicProjection(t *testing.T) {
	m := &memoryRepo{}
	c := newController(t, m, pairFunc(goodPair), nil)
	if e := c.Pair(context.Background(), code(), "Mac", []string{"machine_state_query"}); e != nil {
		t.Fatal(e)
	}
	s, e := c.Status(context.Background())
	if e != nil || s.Phase != Offline || s.DeviceID != "device-1" {
		t.Fatalf("%+v %v", s, e)
	}
	if e = c.Start(context.Background()); e != ErrNotReady {
		t.Fatal("activated without reviewed factory", e)
	}
}
func TestStrictJSON(t *testing.T) {
	for _, raw := range [][]byte{[]byte(`{"code":"x","label":"y","capabilities":[]} {}`), []byte(`{"code":"x","label":"y","capabilities":[],"label":"z"}`), {0xff}, []byte(`[]`)} {
		if _, e := decodePair(raw); e == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
}
func TestOriginRejectsOverrides(t *testing.T) {
	for _, o := range []string{"http://backend.test", "https://backend.test/path", "https://backend.test?x=y", "https://user@backend.test", "https://backend.test#x"} {
		if _, e := New("owner-1", o, &memoryRepo{}, pairFunc(goodPair), nil); e == nil {
			t.Fatal(o)
		}
	}
}

type revokedFactory struct{}

func (revokedFactory) Ready(context.Context) error { return nil }
func (revokedFactory) New(context.Context, string, domain.DeviceBridgeDevice, func(devicebridge.ConnectionState)) (Session, error) {
	return nil, ErrInvalid
}

type revokeSessionFactory struct{ done chan struct{} }

func (f revokeSessionFactory) Ready(context.Context) error { return nil }
func (f revokeSessionFactory) New(_ context.Context, _ string, _ domain.DeviceBridgeDevice, state func(devicebridge.ConnectionState)) (Session, error) {
	return sessionFunc(func(context.Context) error {
		state(devicebridge.ConnectionUnpaired)
		state(devicebridge.ConnectionOnline)
		close(f.done)
		return nil
	}), nil
}
func TestRevocationIsTerminalForSession(t *testing.T) {
	m := &memoryRepo{r: Record{OwnerID: "owner-1", Device: device()}}
	f := revokeSessionFactory{done: make(chan struct{})}
	c := newController(t, m, pairFunc(goodPair), f)
	if e := c.Start(context.Background()); e != nil {
		t.Fatal(e)
	}
	<-f.done
	if e := c.Stop(context.Background()); e != nil {
		t.Fatal(e)
	}
	s, _ := c.Status(context.Background())
	if s.Phase != Unpaired {
		t.Fatal(s)
	}
}

func TestPairCannotWidenCapabilities(t *testing.T) {
	m := &memoryRepo{}
	p := pairFunc(func(context.Context, devicebridge.PairRequest) (domain.DeviceBridgeDevice, error) {
		d := device()
		d.CapabilityClasses = []string{"machine_state_query", "notify_local"}
		return d, nil
	})
	c := newController(t, m, p, nil)
	if e := c.Pair(context.Background(), code(), "Mac", []string{"machine_state_query"}); e != ErrBlocked {
		t.Fatal(e)
	}
}

func TestRevocationBlocksSessionRestartAndControllerRecreation(t *testing.T) {
	m := &memoryRepo{r: Record{OwnerID: "owner-1", Device: device()}}
	f := revokeSessionFactory{done: make(chan struct{})}
	c := newController(t, m, pairFunc(goodPair), f)
	if e := c.Start(context.Background()); e != nil {
		t.Fatal(e)
	}
	<-f.done
	if e := c.Stop(context.Background()); e != nil {
		t.Fatal(e)
	}
	ready := &factory{ready: true, started: make(chan struct{})}
	c.factory = ready
	if e := c.Start(context.Background()); e == nil {
		t.Fatal("revoked identity restarted")
	}
	restarted := newController(t, m, pairFunc(goodPair), ready)
	if e := restarted.Start(context.Background()); e == nil {
		t.Fatal("revoked record activated by recreated controller")
	}
	if ready.calls != 0 {
		t.Fatal("factory called for revoked device")
	}
}
func TestFailedDurableRevokeStaysLocallyBlockedAndReportsError(t *testing.T) {
	m := &memoryRepo{r: Record{OwnerID: "owner-1", Device: device()}, failRevoke: true}
	f := revokeSessionFactory{done: make(chan struct{})}
	c := newController(t, m, pairFunc(goodPair), f)
	if e := c.Start(context.Background()); e != nil {
		t.Fatal(e)
	}
	<-f.done
	if e := c.Stop(context.Background()); e == nil {
		t.Fatal("persistence failure hidden")
	}
	ready := &factory{ready: true, started: make(chan struct{})}
	c.factory = ready
	if e := c.Start(context.Background()); e != ErrBlocked {
		t.Fatal("terminal latch bypassed", e)
	}
	if ready.calls != 0 {
		t.Fatal("factory called after revoke failure")
	}
	// Do not claim process-restart safety on failed persistence: production must
	// implement fencing and repair. Test only proves this controller stays closed.
}

func (m *memoryRepo) Checkpoint(_ context.Context, owner, attempt string, cp devicebridge.PairingRecoveryError) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if owner != m.r.OwnerID || attempt != m.r.Attempt || cp.OwnerID != owner {
		return ErrInvalid
	}
	safe := cp
	m.r.Checkpoint = &safe
	return nil
}

type repositoryWithoutCheckpoint struct{ Repository }

func TestControllerPairFailsClosedWithoutCheckpointer(t *testing.T) {
	m := &memoryRepo{}
	var called bool
	c, e := New("owner-1", "https://backend.test", repositoryWithoutCheckpoint{m}, pairFunc(func(context.Context, devicebridge.PairRequest) (domain.DeviceBridgeDevice, error) {
		called = true
		return device(), nil
	}), nil)
	if e != nil {
		t.Fatal(e)
	}
	if !errors.Is(c.Pair(context.Background(), code(), "Mac", []string{"machine_state_query"}), ErrNotReady) || m.next != 0 || called {
		t.Fatal("did not fail closed")
	}
}
func TestControllerCheckpointCarriesOnlySafeFields(t *testing.T) {
	m := &memoryRepo{}
	ctx, cancel := context.WithCancel(context.Background())
	c := newController(t, m, pairFunc(func(_ context.Context, r devicebridge.PairRequest) (domain.DeviceBridgeDevice, error) {
		cancel()
		e := r.Checkpoint(ctx, devicebridge.PairingRecoveryError{OwnerID: "owner-1", KeyCustodyRef: "opaque-ref", PublicKey: "public", DeviceID: "excluded", Cause: errors.New(code() + "Mac")})
		if e != nil {
			t.Fatal(e)
		}
		m.mu.Lock()
		cp := *m.r.Checkpoint
		m.mu.Unlock()
		if cp.Cause != nil || cp.DeviceID != "" || cp.OwnerID != "owner-1" || cp.KeyCustodyRef != "opaque-ref" || cp.PublicKey != "public" {
			t.Fatal("unsafe checkpoint")
		}
		return domain.DeviceBridgeDevice{}, errors.New("interrupted")
	}), nil)
	if !errors.Is(c.Pair(ctx, code(), "Mac", []string{"machine_state_query"}), ErrBlocked) {
		t.Fatal("pair not blocked")
	}
}

type pairingReadyFactory struct{}

func (pairingReadyFactory) Ready(context.Context) error { return nil }
func (pairingReadyFactory) New(context.Context, string, domain.DeviceBridgeDevice, func(devicebridge.ConnectionState)) (Session, error) {
	return nil, ErrNotReady
}

func TestControllerBlocksResponseWithDifferentOwnerID(t *testing.T) {
	m := &memoryRepo{}
	c := newController(t, m, pairFunc(func(context.Context, devicebridge.PairRequest) (domain.DeviceBridgeDevice, error) {
		d := device()
		d.OwnerID = "other"
		return d, nil
	}), nil)
	if !errors.Is(c.Pair(context.Background(), code(), "Mac", []string{"machine_state_query"}), ErrBlocked) || m.r.Attempt == "" || m.r.Phase != Recovery || m.r.Device.DeviceID != "" {
		t.Fatal("different owner published")
	}
}
func TestStatusReadyReflectsFactory(t *testing.T) {
	for _, kind := range []string{"nil", "not-ready", "ready"} {
		c := newController(t, &memoryRepo{}, pairFunc(goodPair), nil)
		if kind == "nil" {
			c.factory = nil
		} else {
			c.factory = &factory{ready: kind == "ready"}
		}
		s, e := c.Status(context.Background())
		if e != nil || s.Ready != (kind == "ready") || s.Phase != Unpaired {
			t.Fatalf("%s %+v %v", kind, s, e)
		}
	}
}

// retiringFactory revokes through its session and records key retirement calls,
// including whether the revocation was already durable at that moment.
type retiringFactory struct {
	revokeSessionFactory
	repo        *memoryRepo
	retired     []string
	durableSeen []bool
	fail        error
}

func (f *retiringFactory) RetireKey(_ context.Context, d domain.DeviceBridgeDevice) error {
	f.repo.mu.Lock()
	f.durableSeen = append(f.durableSeen, f.repo.r.Device.State == domain.DeviceBridgeStateRevoked)
	f.repo.mu.Unlock()
	f.retired = append(f.retired, d.KeyCustodyRef)
	return f.fail
}

func TestDurableRevocationRetiresPrivateKeyAfterPersisting(t *testing.T) {
	m := &memoryRepo{r: Record{OwnerID: "owner-1", Device: device()}}
	f := &retiringFactory{revokeSessionFactory: revokeSessionFactory{done: make(chan struct{})}, repo: m}
	c := newController(t, m, pairFunc(goodPair), f)
	if e := c.Start(context.Background()); e != nil {
		t.Fatal(e)
	}
	<-f.done
	if e := c.Stop(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(f.retired) != 1 || f.retired[0] != "opaque-ref" || !f.durableSeen[0] {
		t.Fatalf("key retirement must happen exactly once, after durable revoke: %v %v", f.retired, f.durableSeen)
	}
}

func TestFailedRevocationKeepsPrivateKey(t *testing.T) {
	m := &memoryRepo{r: Record{OwnerID: "owner-1", Device: device()}, failRevoke: true}
	f := &retiringFactory{revokeSessionFactory: revokeSessionFactory{done: make(chan struct{})}, repo: m}
	c := newController(t, m, pairFunc(goodPair), f)
	if e := c.Start(context.Background()); e != nil {
		t.Fatal(e)
	}
	<-f.done
	if e := c.Stop(context.Background()); e == nil {
		t.Fatal("persistence failure hidden")
	}
	if len(f.retired) != 0 {
		t.Fatal("private key destroyed although revocation was not durable")
	}
}

func TestKeyRetirementFailureIsReportedWithoutUndoingRevocation(t *testing.T) {
	m := &memoryRepo{r: Record{OwnerID: "owner-1", Device: device()}}
	f := &retiringFactory{revokeSessionFactory: revokeSessionFactory{done: make(chan struct{})}, repo: m, fail: errors.New("custody unavailable")}
	c := newController(t, m, pairFunc(goodPair), f)
	if e := c.Start(context.Background()); e != nil {
		t.Fatal(e)
	}
	<-f.done
	e := c.Stop(context.Background())
	if !errors.Is(e, ErrKeyRetirement) {
		t.Fatalf("retirement failure not reported: %v", e)
	}
	if s, _ := c.Status(context.Background()); s.Phase != Unpaired || m.r.Device.State != domain.DeviceBridgeStateRevoked {
		t.Fatal("revocation must stand even when key retirement fails")
	}
}
