package activationrepo

import (
	"context"
	"crypto/ed25519"
	"errors"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/bridgeactivation"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/devicebridge"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/bridgepersist"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type crashKeys struct {
	mu  sync.Mutex
	key ed25519.PrivateKey
}

func (k *crashKeys) Put(_ context.Context, key ed25519.PrivateKey) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.key = append(ed25519.PrivateKey(nil), key...)
	return "opaque", nil
}
func (k *crashKeys) Clear(context.Context, string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.key = nil
	return nil
}

type crashRepo struct {
	*Repository
	closeDB func()
	mode    string
}

func (r crashRepo) Checkpoint(ctx context.Context, owner, attempt string, cp devicebridge.PairingRecoveryError) error {
	if r.mode == "before-checkpoint" {
		r.closeDB()
		return errors.New("simulated process death")
	}
	return r.Repository.Checkpoint(ctx, owner, attempt, cp)
}
func (r crashRepo) Complete(context.Context, string, string, domain.DeviceBridgeDevice) error {
	r.closeDB()
	return errors.New("simulated process death")
}
func crashCase(t *testing.T, mode string) {
	t.Helper()
	dir := t.TempDir()
	a, b := connections(t, dir)
	repo := New(a)
	keys := &crashKeys{}
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"device_id":"dev","owner_id":"owner","accepted_contract_version":"0.3.0"}`))
	}))
	defer server.Close()
	var once sync.Once
	closeDB := func() { once.Do(func() { a.Close(); b.Close() }) }
	client := server.Client()
	if mode == "before-redeem" {
		transport := client.Transport.(*http.Transport).Clone()
		transport.DialContext = func(context.Context, string, string) (net.Conn, error) {
			closeDB()
			return nil, errors.New("simulated process death before response")
		}
		client = &http.Client{Transport: transport}
	}
	p, e := devicebridge.NewPairingCoordinator(bridgepersist.New(b), keys, client, nil, server.URL+devicebridge.RedeemPath)
	must(t, e)
	wrapper := crashRepo{repo, closeDB, mode}
	c, e := bridgeactivation.New("owner", server.URL, wrapper, p, testFactory{})
	must(t, e)
	want(t, c.Pair(bg, strings.Repeat("A", 43), "Mac", []string{"machine_state_query", "notify_local"}), bridgeactivation.ErrBlocked)
	closeDB()
	a, b = connections(t, dir)
	repo = New(a)
	v := load(t, repo, "owner")
	if v.Attempt == "" || v.Phase != "pending" {
		t.Fatalf("lost reservation %+v", v)
	}
	if mode == "before-checkpoint" {
		if v.Checkpoint != nil || calls.Load() != 0 {
			t.Fatal("checkpoint or request before crash")
		}
	} else {
		if v.Checkpoint == nil || v.Checkpoint.KeyCustodyRef != "opaque" || v.Checkpoint.PublicKey == "" {
			t.Fatal("lost custody checkpoint")
		}
		keys.mu.Lock()
		present := len(keys.key) > 0
		keys.mu.Unlock()
		if !present {
			t.Fatal("lost key")
		}
		if mode == "before-redeem" && calls.Load() != 0 || mode == "after-response" && calls.Load() != 1 {
			t.Fatal("unexpected redeem count")
		}
	}
	_, e = New(b).Reserve(bg, "owner")
	want(t, e, ErrReservationActive)
	c, e = bridgeactivation.New("owner", server.URL, repo, p, testFactory{})
	must(t, e)
	s, e := c.Status(bg)
	must(t, e)
	if s.Phase != bridgeactivation.Recovery {
		t.Fatal("retryable after crash")
	}
}
func TestCrashAfterPutBeforeCheckpoint(t *testing.T)    { crashCase(t, "before-checkpoint") }
func TestCrashAfterCheckpointBeforeRedeem(t *testing.T) { crashCase(t, "before-redeem") }
func TestCrashAfterRedeemResponse(t *testing.T)         { crashCase(t, "after-response") }
