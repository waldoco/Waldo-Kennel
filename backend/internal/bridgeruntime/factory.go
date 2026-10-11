package bridgeruntime

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/bridgeactivation"
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

var _ bridgeactivation.Factory = (*Factory)(nil)

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
	return d.Validate() == nil && d.State == domain.DeviceBridgeStatePaired && d.ContractVersion == devicebridge.ContractVersion && validID(d.OwnerID) && validID(string(d.DeviceID)) && (len(c) == 1 && (c[0] == "machine_state_query" || c[0] == "notify_local") || len(c) == 2 && c[0] == "machine_state_query" && c[1] == "notify_local")
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
	client := &devicebridge.Client{Origin: origin, Scope: devicebridge.Scope{OwnerID: d.OwnerID, DeviceID: string(d.DeviceID)}, Capabilities: append([]string(nil), d.CapabilityClasses...), Key: append(ed25519.PrivateKey(nil), key...), Store: f.deps.SessionStore, Executor: devicebridge.Handlers{Queries: UnknownQueries{}, Display: f.deps.Display}, Dialer: authDialer{dialer, rejected}, State: state, Diagnostic: func(reason string) {
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
