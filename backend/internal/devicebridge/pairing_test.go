package devicebridge

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/secretstore"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testPairCode = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"

func TestPairingBackendAssignedIdentityAfterSignedRedeem(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	keys := secretstore.NewDeviceKeyStore(t.TempDir())
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		key, err := ParseRedeemBody(body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(401)
			return
		}
		if r.Header.Get("X-Waldo-Device-Id") != "" || bytes.Contains(body, []byte("device_id")) {
			t.Error("redeem leaked locally assigned identity")
		}
		if _, found, _ := store.GetPairedDeviceBridgeDevice(r.Context(), "owner-1"); found {
			t.Error("paired before backend success")
		}
		stamp, _ := strconv.ParseInt(r.Header.Get("X-Waldo-Timestamp"), 10, 64)
		n, _ := base64.RawURLEncoding.DecodeString(r.Header.Get("X-Waldo-Nonce"))
		var nonce [NonceBytes]byte
		copy(nonce[:], n)
		base, _ := HTTPBaseString(time.Unix(stamp, 0), nonce, "POST", RedeemPath, body)
		sig, _ := base64.RawURLEncoding.DecodeString(r.Header.Get("X-Waldo-Signature"))
		if !ed25519.Verify(key, []byte(base), sig) {
			t.Error("signature invalid")
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"device_id": "backend_dev", "owner_id": "owner-1", "accepted_contract_version": ContractVersion})
	}))
	defer server.Close()
	p, err := NewPairingCoordinator(store, keys, server.Client(), nil, server.URL+RedeemPath)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := p.Pair(context.Background(), PairRequest{OwnerID: "owner-1", Code: testPairCode, Label: "Café <Go> 𐀀", Capabilities: []string{ClassMachineStateQuery, ClassNotifyLocal}})
	if err != nil {
		t.Fatal(err)
	}
	if rec.DeviceID != "backend_dev" || rec.State != domain.DeviceBridgeStatePaired || rec.PairedAt == nil {
		t.Fatal("backend identity not paired")
	}
	key, err := keys.Get(context.Background(), rec.KeyCustodyRef)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(key.Public().(ed25519.PublicKey), mustDecodePublicKey(t, rec.PublicKey)) {
		t.Fatal("custody mismatch")
	}
}
func TestPairingRejectedOrInvalidResponseNeverPaired(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"rejection", 401, `{"error":"invalid_request"}`, ErrPairingRejected},
		{"version", 200, `{"device_id":"backend_dev","owner_id":"owner-1","accepted_contract_version":"0.1"}`, ErrPairingVersion},
		{"duplicate", 200, `{"device_id":"backend_dev","device_id":"other","owner_id":"owner-1","accepted_contract_version":"0.3.0"}`, ErrPairingResponse},
		{"owner", 200, `{"device_id":"backend_dev","owner_id":"other","accepted_contract_version":"0.3.0"}`, ErrPairingResponse},
		{"unknown-delivery", 500, `{}`, ErrPairingUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := sqlitetest.MustOpen(t)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			p, err := NewPairingCoordinator(store, secretstore.NewDeviceKeyStore(t.TempDir()), server.Client(), nil, server.URL+RedeemPath)
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.Pair(context.Background(), PairRequest{OwnerID: "owner-1", Code: testPairCode, Label: "Test", Capabilities: []string{ClassMachineStateQuery}})
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v", err)
			}
			if _, found, _ := store.GetPairedDeviceBridgeDevice(context.Background(), "owner-1"); found {
				t.Fatal("paired on failed redeem")
			}
			if _, found, _ := store.GetDeviceBridgeDevice(context.Background(), "backend_dev"); found {
				t.Fatal("premature identity persisted")
			}
		})
	}
}
func mustDecodePublicKey(t *testing.T, encoded string) ed25519.PublicKey {
	t.Helper()
	key, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return ed25519.PublicKey(key)
}

func TestPairingAmbiguityRetainsRecoveryAndBlocksAnotherRedeem(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	keys := secretstore.NewDeviceKeyStore(t.TempDir())
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	coordinator, err := NewPairingCoordinator(store, keys, server.Client(), nil, server.URL+RedeemPath)
	if err != nil {
		t.Fatal(err)
	}
	req := PairRequest{OwnerID: "owner-1", Code: testPairCode, Label: "Test", Capabilities: []string{ClassMachineStateQuery}}
	_, err = coordinator.Pair(context.Background(), req)
	var recovery *PairingRecoveryError
	if !errors.As(err, &recovery) || recovery.KeyCustodyRef == "" || recovery.PublicKey == "" {
		t.Fatal("missing safe recovery checkpoint")
	}
	if _, err := keys.Get(context.Background(), recovery.KeyCustodyRef); err != nil {
		t.Fatal("lost ambiguous custody")
	}
	_, err = coordinator.Pair(context.Background(), req)
	if !errors.As(err, &recovery) || calls != 1 {
		t.Fatal("blind redeem retry after ambiguity")
	}
}

func TestConcurrentPairRedeemsOnlyOnce(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]string{"device_id": "backend_dev", "owner_id": "owner-1", "accepted_contract_version": ContractVersion})
	}))
	defer server.Close()
	coordinator, err := NewPairingCoordinator(store, secretstore.NewDeviceKeyStore(t.TempDir()), server.Client(), nil, server.URL+RedeemPath)
	if err != nil {
		t.Fatal(err)
	}
	req := PairRequest{OwnerID: "owner-1", Code: testPairCode, Label: "Test", Capabilities: []string{ClassMachineStateQuery}}
	outcomes := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := coordinator.Pair(context.Background(), req); outcomes <- err }()
	}
	wg.Wait()
	close(outcomes)
	success, rejected := 0, 0
	for err := range outcomes {
		if err == nil {
			success++
		} else if errors.Is(err, ErrPairingAlreadyLive) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 || success != 1 || rejected != 1 {
		t.Fatal("concurrent pairing made multiple redemptions")
	}
}
