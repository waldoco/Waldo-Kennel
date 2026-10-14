package daemon

import (
	"context"
	"database/sql"
	"errors"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/bridgeactivation"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/bridgeruntime"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/devicebridge"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ownercommand"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/secretstore"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/activationrepo"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/bridgepersist"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type bridgeWiring struct {
	handler    http.Handler
	controller *bridgeactivation.Controller
	dbs        []*sql.DB
	log        *slog.Logger
	start      func(context.Context) error
	stop       func(context.Context) error
	closeOnce  sync.Once
	bootOnce   sync.Once
	runCtx     context.Context
	cancel     context.CancelFunc
}

func (b *bridgeWiring) close() {
	if b == nil {
		return
	}
	b.closeOnce.Do(func() {
		if b.cancel != nil {
			b.cancel()
		}
		if b.stop != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if e := b.stop(ctx); e != nil && b.log != nil {
				msg := "local Waldo bridge revocation persistence or stop failed"
				if errors.Is(e, bridgeactivation.ErrKeyRetirement) {
					msg = "local Waldo bridge revocation persistence, stop or private key retirement failed"
				}
				b.log.Warn(msg)
			}
		}
		for _, db := range b.dbs {
			_ = db.Close()
		}
	})
}

// Boot only starts an existing paired identity. Pending, blocked and revoked
// records are not started or recovered, and never become new identities.
func (b *bridgeWiring) boot(ctx context.Context) {
	if b == nil {
		return
	}
	b.bootOnce.Do(func() {
		status, e := b.controller.Status(ctx)
		if e != nil || status.DeviceID == "" || status.Phase == bridgeactivation.Recovery || !status.Ready {
			return
		}
		if e = b.start(b.runCtx); e != nil && b.log != nil {
			if errors.Is(e, bridgeactivation.ErrBlocked) || errors.Is(e, bridgeactivation.ErrNotReady) {
				b.log.Info("local Waldo bridge boot activation unavailable")
			} else {
				b.log.Warn("local Waldo bridge boot activation stopped")
			}
		}
	})
}

type bridgeResponse struct {
	http.ResponseWriter
	status int
}

func (w *bridgeResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *bridgeResponse) Write(raw []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(raw)
}
func (b *bridgeWiring) afterPair(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture := &bridgeResponse{ResponseWriter: w}
		h.ServeHTTP(capture, r)
		if r.Method == http.MethodPost && r.URL.Path == "/pair" && capture.status == http.StatusNoContent {
			if b.start(b.runCtx) != nil && b.log != nil {
				b.log.Info("local Waldo bridge paired; transport offline")
			}
		}
	})
}

// Configuration failure degrades only the optional bridge, never the daemon.
// The opener is injectable so flag-off proves zero storage access.
func wireBridge(dataDir string, getenv func(string) string, authority *ownercommand.BridgeAuthority, log *slog.Logger, open func(string) (*sql.DB, error), executorFactory ...func(*sql.DB, devicebridge.Executor) devicebridge.Executor) *bridgeWiring {
	return wireBridgeWithFactory(dataDir, getenv, authority, log, open, func(deps bridgeruntime.Dependencies) bridgeactivation.Factory { return bridgeruntime.NewFactory(deps) }, executorFactory...)
}
func wireBridgeWithFactory(dataDir string, getenv func(string) string, authority *ownercommand.BridgeAuthority, log *slog.Logger, open func(string) (*sql.DB, error), makeFactory func(bridgeruntime.Dependencies) bridgeactivation.Factory, executorFactory ...func(*sql.DB, devicebridge.Executor) devicebridge.Executor) *bridgeWiring {
	cfg, e := bridgeactivation.LoadLocalConfig(getenv)
	if e != nil {
		log.Warn("local Waldo bridge configuration invalid; bridge disabled")
		return nil
	}
	if !cfg.Enabled {
		return nil
	}
	if authority == nil {
		log.Warn("local Waldo bridge authority unavailable; bridge disabled")
		return nil
	}
	runCtx, cancel := context.WithCancel(context.Background())
	b := &bridgeWiring{log: log, runCtx: runCtx, cancel: cancel}
	fail := func() *bridgeWiring {
		b.close()
		log.Warn("local Waldo bridge dependencies unavailable; bridge disabled")
		return nil
	}
	repoDB, e := open(dataDir)
	if e != nil {
		return fail()
	}
	b.dbs = append(b.dbs, repoDB)
	sessionDB, e := open(dataDir)
	if e != nil {
		return fail()
	}
	b.dbs = append(b.dbs, sessionDB)
	// macOS keeps new device keys in the login Keychain; other platforms and
	// existing file references stay with restrictive file custody.
	custody := secretstore.NewDeviceKeyCustody(dataDir)
	pairer, e := devicebridge.NewPairingCoordinator(bridgepersist.New(sessionDB), custody, nil, log, cfg.Origin+devicebridge.RedeemPath)
	if e != nil {
		return fail()
	}
	runtimeDB, e := open(dataDir)
	if e != nil {
		return fail()
	}
	b.dbs = append(b.dbs, runtimeDB)
	keyDir := filepath.Join(dataDir, "secrets", "device-keys")
	if e = os.MkdirAll(keyDir, 0700); e != nil {
		return fail()
	}
	// One relay per daemon: notify_local text reaches the desktop shell only through
	// the authenticated loopback display endpoints below.
	display := bridgeruntime.NewDisplayRelay()
	// K0 classes keep the honest display relay; v0.3 machine requests wrap them.
	k0 := devicebridge.Handlers{Queries: bridgeruntime.UnknownQueries{}, Display: display}
	var executor devicebridge.Executor
	if len(executorFactory) == 1 {
		executor = executorFactory[0](runtimeDB, k0)
	}
	factory := makeFactory(bridgeruntime.Dependencies{Executor: executor, Fence: activationrepo.New(repoDB), Keys: custody, SessionStore: bridgepersist.New(runtimeDB), Logger: log, KeyDirectory: keyDir, Display: display, ReadyProbe: func(ctx context.Context) error {
		var n int
		return repoDB.QueryRowContext(ctx, "SELECT count(*) FROM device_bridge_activations").Scan(&n)
	}})
	// Without a configured owner the bridge learns it from the owner-scoped pairing
	// code the Waldo console shows, and keeps it in the activation store.
	repo := activationrepo.New(repoDB)
	var c *bridgeactivation.Controller
	if cfg.OwnerID == "" {
		c, e = bridgeactivation.NewBound(cfg.Origin, repo, repo, pairer, factory)
	} else {
		c, e = bridgeactivation.New(cfg.OwnerID, cfg.Origin, repo, pairer, factory)
	}
	if e != nil {
		return fail()
	}
	b.controller = c
	b.start = c.Start
	b.stop = c.Stop
	authorize := func(r *http.Request) bool { return authority.Authenticate(r.Header.Get("Authorization")) }
	activation := b.afterPair(bridgeactivation.Handler(c, authorize))
	displayHandler := display.Handler(authorize)
	b.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/display/") {
			displayHandler.ServeHTTP(w, r)
			return
		}
		activation.ServeHTTP(w, r)
	})
	return b
}

// The listener is bound before Run, but the HTTP serve goroutine starts inside
// Run. Boot activation waits for that local readiness endpoint, never Waldo.
func (b *bridgeWiring) bootWhenServing(ctx context.Context, address string) {
	client := &http.Client{Timeout: time.Second}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address+"/readyz", nil)
		if e != nil {
			return
		}
		res, e := client.Do(req)
		if e == nil {
			res.Body.Close()
			if res.StatusCode == http.StatusOK {
				b.boot(ctx)
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-b.runCtx.Done():
			return
		case <-ticker.C:
		}
	}
}
