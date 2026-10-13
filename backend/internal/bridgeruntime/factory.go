package bridgeruntime

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/bridgeactivation"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/bridgecontract"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/devicebridge"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

type Fence interface {
	ClaimActivation(context.Context, string, domain.DeviceBridgeDeviceID) (int64, error)
	VerifyClaim(context.Context, string, domain.DeviceBridgeDeviceID, int64) error
}
type Keys interface {
	Get(context.Context, string) (ed25519.PrivateKey, error)
}
type Dependencies struct {
	Fence        Fence
	Keys         Keys
	SessionStore devicebridge.SessionStore
	Dialer       devicebridge.Dialer
	Executor     devicebridge.Executor
	Clock        func() time.Time
	Sleep        func(context.Context, time.Duration) error
	Jitter       func() float64
	Logger       *slog.Logger
	ReadyProbe   func(context.Context) error
	KeyDirectory string
	// Display is the notify_local surface. Nil keeps every notification
	// delivery_unknown; no adapter is invented here.
	Display devicebridge.DisplaySurface
}
type Factory struct{ deps Dependencies }

// KeyClearer is key custody that can destroy a key it holds. Clearing an absent
// key must succeed so a retried retirement is harmless.
type KeyClearer interface {
	Clear(context.Context, string) error
}

var (
	_ bridgeactivation.Factory    = (*Factory)(nil)
	_ bridgeactivation.KeyRetirer = (*Factory)(nil)
)

// RetireKey destroys a durably revoked identity's private key. Custody that cannot
// clear is reported rather than silently keeping the key.
func (f *Factory) RetireKey(ctx context.Context, d domain.DeviceBridgeDevice) error {
	if f == nil || d.KeyCustodyRef == "" {
		return fmt.Errorf("bridge custody: %w", bridgeactivation.ErrNotReady)
	}
	clearer, ok := f.deps.Keys.(KeyClearer)
	if !ok {
		return fmt.Errorf("bridge custody cannot clear keys: %w", bridgeactivation.ErrNotReady)
	}
	return clearer.Clear(ctx, d.KeyCustodyRef)
}

func NewFactory(deps Dependencies) *Factory { return &Factory{deps: deps} }
func (f *Factory) Ready(ctx context.Context) error {
	if f == nil || f.deps.Fence == nil || f.deps.Keys == nil || f.deps.SessionStore == nil || f.deps.ReadyProbe == nil || f.deps.KeyDirectory == "" {
		return fmt.Errorf("bridge dependencies: %w", bridgeactivation.ErrNotReady)
	}
	if f.deps.ReadyProbe(ctx) != nil {
		return fmt.Errorf("bridge storage: %w", bridgeactivation.ErrNotReady)
	}
	dir, e := os.Open(f.deps.KeyDirectory)
	if e != nil {
		return fmt.Errorf("bridge custody: %w", bridgeactivation.ErrNotReady)
	}
	defer dir.Close()
	info, e := dir.Stat()
	if e != nil || !info.IsDir() {
		return fmt.Errorf("bridge custody: %w", bridgeactivation.ErrNotReady)
	}
	if _, e = dir.Readdirnames(-1); e != nil {
		return fmt.Errorf("bridge custody: %w", bridgeactivation.ErrNotReady)
	}
	return nil
}
func validID(s string) bool {
	if len(s) == 0 || len(s) > devicebridge.IdentifierMaxBytes {
		return false
	}
	for i := range s {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}
func validDevice(d domain.DeviceBridgeDevice) bool {
	c := d.CapabilityClasses
	return d.Validate() == nil && d.State == domain.DeviceBridgeStatePaired && d.ContractVersion == devicebridge.ContractVersion && validID(d.OwnerID) && validID(string(d.DeviceID)) && bridgecontract.ValidCapabilities(c)
}
func (f *Factory) New(ctx context.Context, origin string, d domain.DeviceBridgeDevice, state func(devicebridge.ConnectionState)) (bridgeactivation.Session, error) {
	if f == nil || f.deps.Fence == nil || f.deps.Keys == nil || f.deps.SessionStore == nil || !validDevice(d) {
		return nil, bridgeactivation.ErrNotReady
	}
	generation, e := f.deps.Fence.ClaimActivation(ctx, d.OwnerID, d.DeviceID)
	if e != nil {
		return nil, e
	}
	key, e := f.deps.Keys.Get(ctx, d.KeyCustodyRef)
	if e != nil || len(key) != ed25519.PrivateKeySize {
		return nil, bridgeactivation.ErrNotReady
	}
	derived := ed25519.NewKeyFromSeed(key[:ed25519.SeedSize])
	if !key.Equal(derived) || base64.RawURLEncoding.EncodeToString(derived.Public().(ed25519.PublicKey)) != d.PublicKey {
		return nil, bridgeactivation.ErrNotReady
	}
	dialer := f.deps.Dialer
	if dialer == nil {
		dialer = devicebridge.WebSocketDialer{}
	}
	rejected := &atomic.Bool{}
	executor := f.deps.Executor
	if executor == nil {
		executor = devicebridge.Handlers{Queries: UnknownQueries{}, Display: f.deps.Display}
	}
	client := &devicebridge.Client{Origin: origin, Scope: devicebridge.Scope{OwnerID: d.OwnerID, DeviceID: string(d.DeviceID)}, Capabilities: append([]string(nil), d.CapabilityClasses...), Key: append(ed25519.PrivateKey(nil), key...), Store: f.deps.SessionStore, Executor: executor, Dialer: authDialer{dialer, rejected}, State: state, Diagnostic: func(reason string) {
		if f.deps.Logger != nil {
			switch reason {
			case devicebridge.ReasonInvalidShape, devicebridge.ReasonUnknownMessage:
				f.deps.Logger.Warn("local Waldo bridge frame rejected", "reason", reason)
			}
		}
	}}
	clock := f.deps.Clock
	if clock == nil {
		clock = time.Now
	}
	sleep := f.deps.Sleep
	if sleep == nil {
		sleep = interruptibleSleep
	}
	jitter := f.deps.Jitter
	if jitter == nil {
		jitter = rand.Float64
	}
	return &Session{client: client, connect: client.Connect, fence: f.deps.Fence, owner: d.OwnerID, device: d.DeviceID, generation: generation, clock: clock, sleep: sleep, jitter: jitter, rejected: rejected}, nil
}

// Observe terminal authentication before Client tries to persist ClearPaired.
// Even a failed ClearPaired must never turn an auth rejection into a retry.
type authDialer struct {
	devicebridge.Dialer
	rejected *atomic.Bool
}

func (d authDialer) Dial(ctx context.Context, target string, headers http.Header) (devicebridge.Socket, error) {
	s, e := d.Dialer.Dial(ctx, target, headers)
	if errors.Is(e, devicebridge.ErrAuthenticationRejected) {
		d.rejected.Store(true)
	}
	return s, e
}
