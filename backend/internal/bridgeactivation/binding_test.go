package bridgeactivation

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/devicebridge"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// memoryBinding is test-only; activationrepo proves the durable, transactional rule.
type memoryBinding struct {
	mu      sync.Mutex
	owner   string
	refuse  bool
	bindLog []string
}

func (b *memoryBinding) BoundOwner(context.Context) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.owner, nil
}
func (b *memoryBinding) BindOwner(_ context.Context, owner string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bindLog = append(b.bindLog, owner)
	if b.refuse && b.owner != owner {
		return errors.New("bound owner still active")
	}
	b.owner = owner
	return nil
}

func scoped(owner string) string { return PairingCodePrefix + "." + owner + "." + code() }

func newBound(t *testing.T, m *memoryRepo, b *memoryBinding, p Pairer) *Controller {
	t.Helper()
	c, e := NewBound("https://backend.test", b, m, p, pairingReadyFactory{})
	if e != nil {
		t.Fatal(e)
	}
	return c
}

func TestBoundControllerIsUnpairedBeforeItsFirstCode(t *testing.T) {
	c := newBound(t, &memoryRepo{}, &memoryBinding{}, pairFunc(goodPair))
	s, e := c.Status(context.Background())
	if e != nil || s.Phase != Unpaired || !s.Ready {
		t.Fatalf("status = %+v, %v", s, e)
	}
	if e := c.Start(context.Background()); e != ErrNotReady {
		t.Fatalf("unbound start = %v", e)
	}
}

func TestBoundControllerLearnsOwnerFromScopedCodeAndRedeemsOnlyTheCode(t *testing.T) {
	m, b := &memoryRepo{}, &memoryBinding{}
	var got devicebridge.PairRequest
	c := newBound(t, m, b, pairFunc(func(_ context.Context, r devicebridge.PairRequest) (domain.DeviceBridgeDevice, error) {
		got = r
		return device(), nil
	}))
	if e := c.Pair(context.Background(), scoped("owner-1"), "Mac", []string{"machine_state_query"}); e != nil {
		t.Fatal(e)
	}
	if b.owner != "owner-1" || got.OwnerID != "owner-1" || got.Code != code() {
		t.Fatalf("bound %q, redeem request owner %q code %q", b.owner, got.OwnerID, got.Code)
	}
	s, e := c.Status(context.Background())
	if e != nil || s.Phase != Offline || s.DeviceID != "device-1" {
		t.Fatalf("status = %+v, %v", s, e)
	}
}

func TestBoundControllerRefusesCodesThatDoNotNameAnOwner(t *testing.T) {
	for _, bad := range []string{code(), "wp1." + code(), "wp1..", "wp1.owner/1." + code(), "wp1.owner-1." + code() + "x", "wp2.owner-1." + code()} {
		m, b := &memoryRepo{}, &memoryBinding{}
		c := newBound(t, m, b, pairFunc(goodPair))
		if e := c.Pair(context.Background(), bad, "Mac", []string{"machine_state_query"}); e != ErrInvalid {
			t.Fatalf("%q: %v", bad, e)
		}
		if len(b.bindLog) != 0 || m.next != 0 {
			t.Fatalf("%q bound or reserved before validation", bad)
		}
	}
}

func TestBoundControllerNeverOrphansAnActiveOwner(t *testing.T) {
	m, b := &memoryRepo{}, &memoryBinding{owner: "owner-1", refuse: true}
	c := newBound(t, m, b, pairFunc(goodPair))
	if e := c.Pair(context.Background(), scoped("owner-2"), "Mac", []string{"machine_state_query"}); e != ErrInvalid {
		t.Fatal(e)
	}
	if b.owner != "owner-1" || m.next != 0 {
		t.Fatalf("rebound to %q or reserved", b.owner)
	}
}

func TestBoundControllerBlocksAResponseForAnotherOwner(t *testing.T) {
	m, b := &memoryRepo{}, &memoryBinding{}
	c := newBound(t, m, b, pairFunc(goodPair)) // goodPair answers for owner-1
	if e := c.Pair(context.Background(), scoped("owner-2"), "Mac", []string{"machine_state_query"}); e != ErrBlocked {
		t.Fatal(e)
	}
	s, _ := c.Status(context.Background())
	if s.Phase != Recovery {
		t.Fatalf("status = %+v", s)
	}
}

func TestConfiguredOwnerAcceptsOnlyItsOwnScopedCode(t *testing.T) {
	c := newController(t, &memoryRepo{}, pairFunc(goodPair), nil)
	if e := c.Pair(context.Background(), scoped("owner-2"), "Mac", []string{"machine_state_query"}); e != ErrInvalid {
		t.Fatal(e)
	}
	if e := c.Pair(context.Background(), scoped("owner-1"), "Mac", []string{"machine_state_query"}); e != nil {
		t.Fatal(e)
	}
}

func TestNewBoundRequiresABindingStore(t *testing.T) {
	if _, e := NewBound("https://backend.test", nil, &memoryRepo{}, pairFunc(goodPair), pairingReadyFactory{}); e != ErrInvalid {
		t.Fatal(e)
	}
}
