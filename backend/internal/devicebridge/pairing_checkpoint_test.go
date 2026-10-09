package devicebridge

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type checkpointKeys struct {
	mu     sync.Mutex
	key    ed25519.PrivateKey
	events []string
}

func (k *checkpointKeys) Put(_ context.Context, key ed25519.PrivateKey) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.key = append(ed25519.PrivateKey(nil), key...)
	k.events = append(k.events, "put")
	return "opaque", nil
}
func (k *checkpointKeys) Clear(context.Context, string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.key = nil
	k.events = append(k.events, "clear")
	return nil
}
func TestPairCheckpointCalledAfterPutBeforeNetwork(t *testing.T) {
	keys := &checkpointKeys{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys.mu.Lock()
		keys.events = append(keys.events, "network")
		keys.mu.Unlock()
		_, _ = w.Write([]byte(`{"device_id":"dev","owner_id":"owner","accepted_contract_version":"0.3.0"}`))
	}))
	defer server.Close()
	p, e := NewPairingCoordinator(sqlitetest.MustOpen(t), keys, server.Client(), nil, server.URL+RedeemPath)
	if e != nil {
		t.Fatal(e)
	}
	_, e = p.Pair(context.Background(), PairRequest{OwnerID: "owner", Code: testPairCode, Label: "Mac", Capabilities: []string{ClassNotifyLocal}, Checkpoint: func(_ context.Context, cp PairingRecoveryError) error {
		keys.mu.Lock()
		defer keys.mu.Unlock()
		keys.events = append(keys.events, "checkpoint")
		if cp.OwnerID != "owner" || cp.KeyCustodyRef != "opaque" || cp.Cause != nil || cp.DeviceID != "" || cp.PublicKey != base64.RawURLEncoding.EncodeToString(keys.key.Public().(ed25519.PublicKey)) {
			t.Error("unsafe checkpoint")
		}
		return nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	keys.mu.Lock()
	defer keys.mu.Unlock()
	if !reflect.DeepEqual(keys.events, []string{"put", "checkpoint", "network"}) {
		t.Fatal(keys.events)
	}
}
func TestPairCheckpointFailureFailsClosed(t *testing.T) {
	keys := &checkpointKeys{}
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"device_id":"dev","owner_id":"owner","accepted_contract_version":"0.3.0"}`))
	}))
	defer server.Close()
	p, e := NewPairingCoordinator(sqlitetest.MustOpen(t), keys, server.Client(), nil, server.URL+RedeemPath)
	if e != nil {
		t.Fatal(e)
	}
	req := PairRequest{OwnerID: "owner", Code: testPairCode, Label: "Mac", Capabilities: []string{ClassNotifyLocal}, Checkpoint: func(context.Context, PairingRecoveryError) error { return errors.New("callback-secret") }}
	_, e = p.Pair(context.Background(), req)
	if !errors.Is(e, ErrPairingUnavailable) || strings.Contains(e.Error(), "callback-secret") || calls.Load() != 0 || p.recovery != nil {
		t.Fatal("did not fail closed")
	}
	keys.mu.Lock()
	present := len(keys.key) > 0
	keys.mu.Unlock()
	if present {
		t.Fatal("key retained")
	}
	req.Checkpoint = nil
	_, e = p.Pair(context.Background(), req)
	if e != nil || calls.Load() != 1 {
		t.Fatalf("latched %v", e)
	}
}
func TestPairWithoutCheckpointUnchanged(t *testing.T) {
	keys := &checkpointKeys{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"device_id":"dev","owner_id":"owner","accepted_contract_version":"0.3.0"}`))
	}))
	defer server.Close()
	p, e := NewPairingCoordinator(sqlitetest.MustOpen(t), keys, server.Client(), nil, server.URL+RedeemPath)
	if e != nil {
		t.Fatal(e)
	}
	d, e := p.Pair(context.Background(), PairRequest{OwnerID: "owner", Code: testPairCode, Label: "Mac", Capabilities: []string{ClassNotifyLocal}})
	if e != nil || d.DeviceID != "dev" || p.recovery != nil {
		t.Fatalf("nil hook changed behavior %v", e)
	}
}
