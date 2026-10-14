// Package bridgeactivation defines an inactive integration boundary for K1.
// A durable repository and reviewed S4 implementation are required to activate it.
package bridgeactivation

import (
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/bridgecontract"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/devicebridge"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

var (
	ErrBlocked  = errors.New("pairing requires reconciliation")
	ErrNotReady = errors.New("bridge activation dependencies are not ready")
	ErrInvalid  = errors.New("invalid bridge activation request")
	// ErrKeyRetirement reports that a durably revoked identity's private key could
	// not be destroyed. The revocation itself stands.
	ErrKeyRetirement = errors.New("revoked device private key retirement failed")
)

type Phase string

const (
	Unpaired Phase = "unpaired"
	Pairing  Phase = "pairing"
	Recovery Phase = "recovery_required"
	Offline  Phase = "offline"
	Online   Phase = "online"
)

// Record never contains a code or key bytes. Attempt is an opaque reservation ID.
// A pending record found after restart must stay blocked, never auto-redeemed.
type Record struct {
	OwnerID    string
	Attempt    string
	Phase      Phase
	Device     domain.DeviceBridgeDevice
	Checkpoint *devicebridge.PairingRecoveryError
}

// Repository methods must be atomic across processes and durable before return.
// Reserve succeeds only when no active/pending identity exists for owner.
// Complete/Block compare the attempt token. Block retains the reservation, even
// when checkpoint is nil. Complete publishes paired identity atomically. Neither
// method may overwrite another owner's/device's data. No implementation is
// provided here; Gate B and pairing recovery are independent proof gates.
type Repository interface {
	Load(context.Context, string) (Record, error)
	Reserve(context.Context, string) (Record, error)
	Complete(context.Context, string, string, domain.DeviceBridgeDevice) error
	Block(context.Context, string, string, *devicebridge.PairingRecoveryError) error
	// Revoke atomically marks this exact owner/device as revoked, prevents all
	// later activation claims for that identity, and preserves audit/checkpoints.
	// It must fence concurrent active/starting sessions through the same durable
	// identity gate used by Factory.New. Revoked records are never resurrected.
	Revoke(context.Context, string, domain.DeviceBridgeDeviceID) error
}

// OwnerBinding persists which Waldo owner this Mac pairs with when no owner is
// configured at boot. It is learned from the owner-scoped pairing code. BindOwner
// must refuse a different owner while the bound owner has a pending, blocked or
// paired activation, atomically with that check.
type OwnerBinding interface {
	BoundOwner(context.Context) (string, error)
	BindOwner(context.Context, string) error
}

// PairingCodePrefix marks the owner-scoped code the Waldo console shows:
// wp1.<owner id>.<43-char pairing code>. Only the 43-char code is ever redeemed.
const PairingCodePrefix = "wp1"

type Pairer interface {
	Pair(context.Context, devicebridge.PairRequest) (domain.DeviceBridgeDevice, error)
}

// Checkpointer durably retains only safe custody locators before redeem.
type Checkpointer interface {
	Checkpoint(context.Context, string, string, devicebridge.PairingRecoveryError) error
}

type Session interface{ Run(context.Context) error }

// KeyRetirer is optionally implemented by a Factory whose custody can destroy a
// revoked identity's private key. It is called only after Revoke is durable, so a
// failed revocation never strands a paired record without its key.
type KeyRetirer interface {
	RetireKey(context.Context, domain.DeviceBridgeDevice) error
}

// Factory must bind sessions to the stored owner, device, pinned origin and key
// custody. It must not widen capabilities or create a new identity. New must
// acquire/check the Repository revocation fence atomically with activation;
// Ready alone and a stale Load are not cross-process activation authorization.
type Factory interface {
	Ready(context.Context) error
	New(context.Context, string, domain.DeviceBridgeDevice, func(devicebridge.ConnectionState)) (Session, error)
}

type Status struct {
	Ready    bool   `json:"ready"`
	Phase    Phase  `json:"state"`
	DeviceID string `json:"device_id,omitempty"`
	Label    string `json:"label,omitempty"`
}

type Controller struct {
	owner, origin string
	binding       OwnerBinding
	pairMu        sync.Mutex
	repo          Repository
	pairer        Pairer
	factory       Factory
	mu            sync.Mutex
	stateMu       sync.Mutex
	cancel        context.CancelFunc
	done          chan struct{}
	phase         Phase
	revokedID     domain.DeviceBridgeDeviceID
	revocationErr error
	retireErr     error
}

// New requires an owner bound by trusted local configuration, not caller input.
// origin is one pinned HTTPS origin; no redirects, overrides or query allowed.
func New(owner, origin string, repo Repository, pairer Pairer, factory Factory) (*Controller, error) {
	u, err := url.Parse(origin)
	if owner == "" || err != nil || u.Scheme != "https" || u.Host == "" || u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || repo == nil || pairer == nil {
		return nil, ErrInvalid
	}
	return &Controller{owner: owner, origin: origin, repo: repo, pairer: pairer, factory: factory, phase: Offline}, nil
}

// NewBound learns its owner from the first owner-scoped pairing code and keeps it
// in binding, so a fresh install needs no boot-time owner configuration.
func NewBound(origin string, binding OwnerBinding, repo Repository, pairer Pairer, factory Factory) (*Controller, error) {
	if binding == nil {
		return nil, ErrInvalid
	}
	c, err := New("unbound", origin, repo, pairer, factory)
	if err != nil {
		return nil, err
	}
	c.owner, c.binding = "", binding
	return c, nil
}

// currentOwner is the boot-configured owner, else the bound one ("" before pairing).
func (c *Controller) currentOwner(ctx context.Context) (string, error) {
	if c.binding == nil {
		return c.owner, nil
	}
	return c.binding.BoundOwner(ctx)
}

// splitPairingCode separates an owner-scoped code into its owner and the 43-char
// pairing code. A bare code returns no owner.
func splitPairingCode(code string) (owner, raw string, ok bool) {
	rest, scoped := strings.CutPrefix(code, PairingCodePrefix+".")
	if !scoped {
		return "", code, true
	}
	owner, raw, found := strings.Cut(rest, ".")
	if !found || !validOwnerID(owner) {
		return "", "", false
	}
	return owner, raw, true
}

func (c *Controller) Status(ctx context.Context) (Status, error) {
	owner, err := c.currentOwner(ctx)
	if err != nil {
		return Status{}, err
	}
	ready := c.factory != nil && c.factory.Ready(ctx) == nil
	if owner == "" {
		return Status{Ready: ready, Phase: Unpaired}, nil
	}
	r, err := c.repo.Load(ctx, owner)
	if err != nil {
		return Status{}, err
	}
	if r.OwnerID != "" && r.OwnerID != owner {
		return Status{}, ErrInvalid
	}
	if r.Attempt != "" {
		return Status{Ready: ready, Phase: Recovery}, nil
	}
	if r.Device.DeviceID == "" || r.Device.State != domain.DeviceBridgeStatePaired {
		return Status{Ready: ready, Phase: Unpaired}, nil
	}
	c.stateMu.Lock()
	phase := c.phase
	c.stateMu.Unlock()
	return Status{Ready: ready, Phase: phase, DeviceID: string(r.Device.DeviceID), Label: r.Device.Label}, nil
}

func (c *Controller) Pair(ctx context.Context, code, label string, capabilities []string, consent ...bool) error {
	sessionConsent := len(consent) == 1 && consent[0]
	for _, cap := range capabilities {
		if (cap == devicebridge.ClassSessionQuery || cap == devicebridge.ClassSessionControl) && !sessionConsent {
			return ErrInvalid
		}
	}
	if len(consent) > 1 {
		return ErrInvalid
	}
	// Reject invalid inputs before reserving; secrets never enter Repository.
	codeOwner, code, ok := splitPairingCode(code)
	if !ok {
		return ErrInvalid
	}
	decoded, e := base64.RawURLEncoding.DecodeString(code)
	if e != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != code || !utf8.ValidString(label) || len(label) < 1 || len(label) > 120 || !validCapabilities(capabilities) {
		return ErrInvalid
	}
	// A configured owner accepts only its own scoped code; a bound controller
	// needs the scoped code to learn whom it pairs with.
	owner := c.owner
	if (c.binding == nil && codeOwner != "" && codeOwner != c.owner) || (c.binding != nil && codeOwner == "") {
		return ErrInvalid
	}
	if c.factory == nil || c.factory.Ready(ctx) != nil {
		return ErrNotReady
	}
	// Do not persist code. A durable reservation comes before any network call.
	cp, ok := c.repo.(Checkpointer)
	if !ok {
		return ErrNotReady
	}
	c.pairMu.Lock()
	defer c.pairMu.Unlock()
	if c.binding != nil {
		c.mu.Lock()
		running := c.cancel != nil
		c.mu.Unlock()
		if running {
			return ErrInvalid
		}
		// Bind before reserving so a crash after Reserve still finds this owner.
		if err := c.binding.BindOwner(ctx, codeOwner); err != nil {
			return ErrInvalid
		}
		owner = codeOwner
	}
	r, err := c.repo.Reserve(ctx, owner)
	if err != nil {
		return err
	}
	if r.OwnerID != owner || r.Attempt == "" {
		return ErrInvalid
	}
	d, err := c.pairer.Pair(ctx, devicebridge.PairRequest{OwnerID: owner, Code: code, Label: label, SessionConsent: sessionConsent, Capabilities: append([]string(nil), capabilities...), Checkpoint: func(_ context.Context, observed devicebridge.PairingRecoveryError) error {
		safe := devicebridge.PairingRecoveryError{OwnerID: observed.OwnerID, KeyCustodyRef: observed.KeyCustodyRef, PublicKey: observed.PublicKey}
		return cp.Checkpoint(context.WithoutCancel(ctx), owner, r.Attempt, safe)
	}})
	if err != nil {
		var checkpoint *devicebridge.PairingRecoveryError
		var observed *devicebridge.PairingRecoveryError
		if errors.As(err, &observed) && observed.OwnerID == owner {
			// Copy only safe checkpoint fields, not the original error/Cause chain.
			checkpoint = &devicebridge.PairingRecoveryError{OwnerID: observed.OwnerID, DeviceID: observed.DeviceID, KeyCustodyRef: observed.KeyCustodyRef, PublicKey: observed.PublicKey}
		}
		if blockErr := c.repo.Block(context.WithoutCancel(ctx), owner, r.Attempt, checkpoint); blockErr != nil {
			return ErrBlocked
		}
		return ErrBlocked
	}
	if !sameCapabilities(d.CapabilityClasses, capabilities) || d.OwnerID != owner || d.State != domain.DeviceBridgeStatePaired || d.ContractVersion != devicebridge.ContractVersion || !validCapabilities(d.CapabilityClasses) || d.Validate() != nil {
		_ = c.repo.Block(context.WithoutCancel(ctx), owner, r.Attempt, nil)
		return ErrBlocked
	}
	if err = c.repo.Complete(context.WithoutCancel(ctx), owner, r.Attempt, d); err != nil {
		_ = c.repo.Block(context.WithoutCancel(ctx), owner, r.Attempt, &devicebridge.PairingRecoveryError{OwnerID: owner, DeviceID: string(d.DeviceID), KeyCustodyRef: d.KeyCustodyRef, PublicKey: d.PublicKey})
		return ErrBlocked
	}
	return nil
}

// Start never pairs, and never auto-recovers an interrupted redemption. Factory
// readiness covers durable Gate B, custody/recovery and reviewed S4 handlers.
func (c *Controller) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		return ErrBlocked
	}
	owner, err := c.currentOwner(ctx)
	if err != nil {
		return err
	}
	if owner == "" {
		return ErrNotReady
	}
	r, err := c.repo.Load(ctx, owner)
	if err != nil {
		return err
	}
	if r.Attempt != "" {
		return ErrBlocked
	}
	if r.OwnerID != owner || r.Device.OwnerID != owner || r.Device.State != domain.DeviceBridgeStatePaired || r.Device.ContractVersion != devicebridge.ContractVersion || !validCapabilities(r.Device.CapabilityClasses) || r.Device.Validate() != nil {
		return ErrNotReady
	}
	c.stateMu.Lock()
	revoked := c.revokedID == r.Device.DeviceID
	c.stateMu.Unlock()
	if revoked {
		return ErrBlocked
	}
	if c.factory == nil || c.factory.Ready(ctx) != nil {
		return ErrNotReady
	}
	// State callback uses a separate gate until Start releases the controller lock.
	runctx, cancel := context.WithCancel(ctx)
	state := func(s devicebridge.ConnectionState) {
		c.stateMu.Lock()
		if c.revokedID == r.Device.DeviceID {
			c.stateMu.Unlock()
			return
		}
		if s == devicebridge.ConnectionUnpaired {
			c.revokedID = r.Device.DeviceID
			c.phase = Unpaired
			c.stateMu.Unlock()
			// Immediate local latch is not durable proof. Return a persistence failure
			// through Stop and require repair before reuse; never silently retry it.
			err := c.repo.Revoke(context.WithoutCancel(runctx), owner, r.Device.DeviceID)
			var retireErr error
			if retirer, ok := c.factory.(KeyRetirer); ok && err == nil {
				if e := retirer.RetireKey(context.WithoutCancel(runctx), r.Device); e != nil {
					retireErr = errors.Join(ErrKeyRetirement, e)
				}
			}
			c.stateMu.Lock()
			c.revocationErr = err
			c.retireErr = retireErr
			c.stateMu.Unlock()
			cancel()
			return
		}
		switch s {
		case devicebridge.ConnectionOnline:
			c.phase = Online
		default:
			c.phase = Offline
		}
		c.stateMu.Unlock()
	}
	c.stateMu.Lock()
	c.phase = Offline
	c.revocationErr = nil
	c.retireErr = nil
	c.stateMu.Unlock()
	session, err := c.factory.New(runctx, c.origin, r.Device, state)
	if err != nil || session == nil {
		cancel()
		if err == nil {
			err = ErrNotReady
		}
		return err
	}
	c.cancel = cancel
	c.done = make(chan struct{})
	done := c.done
	go func() {
		defer close(done)
		_ = session.Run(runctx)
		cancel()
		c.mu.Lock()
		c.stateMu.Lock()
		if c.phase != Unpaired {
			c.phase = Offline
		}
		c.stateMu.Unlock()
		c.cancel = nil
		c.mu.Unlock()
	}()
	return nil
}

func (c *Controller) Stop(ctx context.Context) error {
	c.mu.Lock()
	cancel, done := c.cancel, c.done
	c.mu.Unlock()
	if cancel == nil {
		c.stateMu.Lock()
		err := errors.Join(c.revocationErr, c.retireErr)
		c.stateMu.Unlock()
		return err
	}
	cancel()
	select {
	case <-done:
		c.stateMu.Lock()
		err := errors.Join(c.revocationErr, c.retireErr)
		c.stateMu.Unlock()
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func validCapabilities(classes []string) bool {
	return bridgecontract.ValidCapabilities(classes)
}

func sameCapabilities(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
