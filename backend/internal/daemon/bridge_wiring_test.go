package daemon

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/bridgeactivation"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/bridgeruntime"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/devicebridge"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ownercommand"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/activationrepo"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const wiringToken = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func wiringEnv(flag string) func(string) string {
	return func(k string) string {
		return map[string]string{"KENNEL_WALDO_BRIDGE_ENABLED": flag, "KENNEL_WALDO_ORIGIN": "https://example.test", "KENNEL_WALDO_OWNER_ID": "owner"}[k]
	}
}
func TestDaemonBridgeWiringOffOpensNothing(t *testing.T) {
	var opens int
	open := func(string) (*sql.DB, error) { opens++; return nil, nil }
	for _, flag := range []string{"", "0", "true"} {
		b := wireBridge(t.TempDir(), wiringEnv(flag), ownercommand.NewBridgeAuthority(wiringToken), slog.New(slog.NewTextHandler(io.Discard, nil)), open)
		if b != nil || opens != 0 {
			t.Fatal("off opened storage")
		}
	}
}
func TestDaemonBridgeWiringOnMountsNotReady(t *testing.T) {
	dir := t.TempDir()
	s, e := sqlite.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	b := wireBridgeWithFactory(dir, wiringEnv("1"), ownercommand.NewBridgeAuthority(wiringToken), slog.New(slog.NewTextHandler(io.Discard, nil)), activationrepo.OpenDedicated, func(bridgeruntime.Dependencies) bridgeactivation.Factory { return wiringFactory{} })
	if b == nil || b.controller == nil || len(b.dbs) != 3 {
		t.Fatal("not mounted")
	}
	for _, tc := range []struct {
		method, path, body string
		want               int
	}{{"GET", "/status", "", 200}, {"POST", "/pair", `{"code":"` + wiringToken + `","label":"Mac","capabilities":["notify_local"]}`, 503}} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		r.RemoteAddr = "127.0.0.1:42"
		r.Header.Set("Authorization", "KennelBridge "+wiringToken)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		b.handler.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatal(w.Code)
		}
		if tc.method == http.MethodGet && !strings.Contains(w.Body.String(), `"ready":false`) {
			t.Fatal("premature readiness")
		}
	}
	var reservations int
	if e = b.dbs[0].QueryRow(`SELECT count(*) FROM device_bridge_activations`).Scan(&reservations); e != nil || reservations != 0 {
		t.Fatal("not ready reserved")
	}
	b.close()
	for _, db := range b.dbs {
		if db.Ping() == nil {
			t.Fatal("dedicated DB not closed")
		}
	}
}

// Retain the B3 readiness/503 proof through the explicit factory seam.
type wiringFactory struct{ ready bool }

func (f wiringFactory) Ready(context.Context) error {
	if f.ready {
		return nil
	}
	return bridgeactivation.ErrNotReady
}
func (f wiringFactory) New(context.Context, string, domain.DeviceBridgeDevice, func(devicebridge.ConnectionState)) (bridgeactivation.Session, error) {
	return nil, bridgeactivation.ErrNotReady
}
func testWiring(t *testing.T, logger *slog.Logger) *bridgeWiring {
	t.Helper()
	dir := t.TempDir()
	s, e := sqlite.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	b := wireBridgeWithFactory(dir, wiringEnv("1"), ownercommand.NewBridgeAuthority(wiringToken), logger, activationrepo.OpenDedicated, func(bridgeruntime.Dependencies) bridgeactivation.Factory { return wiringFactory{ready: true} })
	if b == nil {
		t.Fatal("wiring unavailable")
	}
	t.Cleanup(b.close)
	return b
}
func TestBootStartRules(t *testing.T) {
	for _, phase := range []string{"unpaired", "blocked", "revoked", "paired"} {
		t.Run(phase, func(t *testing.T) {
			b := testWiring(t, slog.New(slog.NewTextHandler(io.Discard, nil)))
			repo := activationrepo.New(b.dbs[0])
			ctx := context.Background()
			if phase != "unpaired" {
				r, e := repo.Reserve(ctx, "owner")
				if e != nil {
					t.Fatal(e)
				}
				if phase == "blocked" {
					if e = repo.Block(ctx, "owner", r.Attempt, nil); e != nil {
						t.Fatal(e)
					}
				} else {
					now := time.Now().UTC()
					d := domain.DeviceBridgeDevice{DeviceID: "dev", OwnerID: "owner", Label: "Mac", PublicKey: "public", KeyCustodyRef: "opaque", ContractVersion: devicebridge.ContractVersion, CapabilityClasses: []string{"notify_local"}, State: domain.DeviceBridgeStatePaired, PairedAt: &now, CreatedAt: now, UpdatedAt: now}
					if e = repo.Complete(ctx, "owner", r.Attempt, d); e != nil {
						t.Fatal(e)
					}
					if phase == "revoked" {
						if e = repo.Revoke(ctx, "owner", d.DeviceID); e != nil {
							t.Fatal(e)
						}
					}
				}
			}
			var starts int
			b.start = func(context.Context) error { starts++; return nil }
			b.boot(ctx)
			b.boot(ctx)
			want := 0
			if phase == "paired" {
				want = 1
			}
			if starts != want {
				t.Fatalf("%s starts %d", phase, starts)
			}
		})
	}
}
func TestPairSuccessStartsSessionOnce(t *testing.T) {
	for _, failed := range []bool{false, true} {
		var calls int
		root, cancel := context.WithCancel(context.Background())
		defer cancel()
		b := &bridgeWiring{runCtx: root, start: func(ctx context.Context) error {
			calls++
			if ctx != root {
				t.Fatal("request context owns persistent session")
			}
			if failed {
				return bridgeactivation.ErrNotReady
			}
			return nil
		}}
		h := b.afterPair(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
		r := httptest.NewRequest("POST", "/pair", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 204 || calls != 1 {
			t.Fatal("pair reply altered or duplicate Start")
		}
	}
}
func TestShutdownStopsBeforeClosingDBs(t *testing.T) {
	var logs bytes.Buffer
	b := testWiring(t, slog.New(slog.NewTextHandler(&logs, nil)))
	var stops int
	b.stop = func(ctx context.Context) error {
		stops++
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 10*time.Second {
			t.Fatal("missing bounded stop")
		}
		for _, db := range b.dbs {
			if db.Ping() != nil {
				t.Fatal("DB closed before Stop")
			}
		}
		return errors.New("secret-key-marker" + wiringToken)
	}
	b.close()
	b.close()
	if stops != 1 {
		t.Fatal("stop not idempotent")
	}
	for _, db := range b.dbs {
		if db.Ping() == nil {
			t.Fatal("DB remains open")
		}
	}
	if !strings.Contains(logs.String(), "revocation persistence") || strings.Contains(logs.String(), "secret-key-marker") || strings.Contains(logs.String(), wiringToken) {
		t.Fatal("unsafe shutdown log")
	}
}

func TestDaemonBridgeWiringRoutesDisplayBehindBridgeAuthority(t *testing.T) {
	dir := t.TempDir()
	s, e := sqlite.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	b := wireBridgeWithFactory(dir, wiringEnv("1"), ownercommand.NewBridgeAuthority(wiringToken), slog.New(slog.NewTextHandler(io.Discard, nil)), activationrepo.OpenDedicated, func(bridgeruntime.Dependencies) bridgeactivation.Factory { return wiringFactory{} })
	if b == nil {
		t.Fatal("not mounted")
	}
	defer b.close()
	ack := func(body string, token string) int {
		r := httptest.NewRequest(http.MethodPost, "/display/ack", strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:42"
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "KennelBridge "+token)
		}
		w := httptest.NewRecorder()
		b.handler.ServeHTTP(w, r)
		return w.Code
	}
	known := `{"request_id":"` + strings.Repeat("a", 32) + `","shown":true}`
	if code := ack(known, ""); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated display ack: %d", code)
	}
	if code := ack(known, "wrong-token"); code != http.StatusUnauthorized {
		t.Fatalf("wrong-token display ack: %d", code)
	}
	// 400 proves the display relay (not the activation handler's 404) served it.
	if code := ack(`{}`, wiringToken); code != http.StatusBadRequest {
		t.Fatalf("malformed display ack: %d", code)
	}
	if code := ack(known, wiringToken); code != http.StatusNotFound {
		t.Fatalf("unknown display request: %d", code)
	}
}
