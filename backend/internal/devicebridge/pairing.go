package devicebridge

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

var (
	ErrPairingInvalid     = errors.New("invalid device pairing request")
	ErrPairingRejected    = errors.New("device pairing rejected")
	ErrPairingUnavailable = errors.New("device pairing delivery unknown; do not automatically retry")
	ErrPairingResponse    = errors.New("invalid device pairing response; backend redemption may have succeeded")
	ErrPairingVersion     = errors.New("unsupported device bridge version")
	ErrPairingAlreadyLive = errors.New("device already paired for owner")
)

type DeviceKeyCustody interface {
	Put(context.Context, ed25519.PrivateKey) (string, error)
	Clear(context.Context, string) error
}

// PairingStore is only device registration. It cannot spool unscoped results.
// Create must fail on conflicting identity and preserve any existing record.
type PairingStore interface {
	GetPairedDeviceBridgeDevice(context.Context, string) (domain.DeviceBridgeDevice, bool, error)
	CreateDeviceBridgeDevice(context.Context, domain.DeviceBridgeDevice) error
	TransitionDeviceBridgeDevice(context.Context, domain.DeviceBridgeDeviceID, domain.DeviceBridgeState, time.Time) (bool, error)
}

type PairingCoordinator struct {
	store    PairingStore
	keys     DeviceKeyCustody
	client   *http.Client
	logger   *slog.Logger
	now      func() time.Time
	redeem   string
	mu       sync.Mutex
	recovery *PairingRecoveryError
}

// PairingRecoveryError exposes only an opaque custody locator and public key.
// The caller must retain this checkpoint across daemon restart until backend
// evidence resolves redeem. It must not create a fresh coordinator to retry.
// Durable pending-redemption activation is a separate integration requirement.
type PairingRecoveryError struct {
	Cause                                       error
	KeyCustodyRef, PublicKey, OwnerID, DeviceID string
}

func (e *PairingRecoveryError) Error() string {
	return "device pairing requires reconciliation before another redemption"
}
func (e *PairingRecoveryError) Unwrap() error { return e.Cause }

type PairRequest struct {
	OwnerID, Code, Label string
	Capabilities         []string
	SessionConsent       bool
	Checkpoint           func(context.Context, PairingRecoveryError) error
}

func NewPairingCoordinator(store PairingStore, keys DeviceKeyCustody, client *http.Client, logger *slog.Logger, redeemURL string) (*PairingCoordinator, error) {
	u, err := url.Parse(redeemURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Path != RedeemPath || u.RawPath != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || store == nil || keys == nil {
		return nil, ErrPairingInvalid
	}
	safe, err := secureHTTPClient(client, u.Hostname())
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &PairingCoordinator{store: store, keys: keys, client: safe, logger: logger, now: time.Now, redeem: redeemURL}, nil
}

func (p *PairingCoordinator) Pair(ctx context.Context, req PairRequest) (domain.DeviceBridgeDevice, error) {
	empty := domain.DeviceBridgeDevice{}
	if p == nil {
		return empty, ErrPairingInvalid
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.recovery != nil {
		return empty, p.recovery
	}
	if p == nil || !validIdentifier(req.OwnerID) || !validPairingCode(req.Code) || !boundedString(req.Label, LabelMaxBytes) || !validCapabilities(req.Capabilities) || (requiresSessionConsent(req.Capabilities) && !req.SessionConsent) {
		return empty, ErrPairingInvalid
	}
	if _, found, err := p.store.GetPairedDeviceBridgeDevice(ctx, req.OwnerID); err != nil {
		return empty, err
	} else if found {
		return empty, ErrPairingAlreadyLive
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return empty, ErrPairingUnavailable
	}
	ref, err := p.keys.Put(ctx, private)
	if err != nil {
		return empty, ErrPairingUnavailable
	}
	if req.Checkpoint != nil {
		if err := req.Checkpoint(ctx, PairingRecoveryError{OwnerID: req.OwnerID, KeyCustodyRef: ref, PublicKey: base64.RawURLEncoding.EncodeToString(public)}); err != nil {
			_ = p.keys.Clear(ctx, ref)
			return empty, ErrPairingUnavailable
		}
	}
	ambiguous := func(cause error) (domain.DeviceBridgeDevice, error) {
		p.recovery = &PairingRecoveryError{Cause: cause, KeyCustodyRef: ref, PublicKey: base64.RawURLEncoding.EncodeToString(public), OwnerID: req.OwnerID}
		return empty, p.recovery
	}
	// Keep custody on ambiguous transport/response/persistence failures: redemption
	// may have committed remotely. No identity is shown as paired without a valid reply.
	body, err := RedeemBody(req.Code, public, req.Label, req.Capabilities, req.SessionConsent)
	if err != nil {
		_ = p.keys.Clear(ctx, ref)
		return empty, err
	}
	nonce, err := NewNonce(nil)
	if err != nil {
		_ = p.keys.Clear(ctx, ref)
		return empty, ErrPairingUnavailable
	}
	now := p.now().UTC()
	headers, err := SignHTTPHeaders(private, now, http.MethodPost, RedeemPath, body, nonce, "")
	if err != nil {
		_ = p.keys.Clear(ctx, ref)
		return empty, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.redeem, bytes.NewReader(body))
	if err != nil {
		_ = p.keys.Clear(ctx, ref)
		return empty, ErrPairingInvalid
	}
	request.Header = headers
	request.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return ambiguous(ErrPairingUnavailable)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusUnauthorized {
			_ = p.keys.Clear(ctx, ref)
			return empty, ErrPairingRejected
		}
		return ambiguous(ErrPairingUnavailable)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, RedeemResponseMaxBytes+1))
	if err != nil || len(raw) > RedeemResponseMaxBytes {
		return ambiguous(ErrPairingResponse)
	}
	accepted, err := parseJSONObject(raw)
	if err != nil || !exactFields(accepted, "device_id", "owner_id", "accepted_contract_version") || !stringID(accepted["device_id"]) || accepted["owner_id"] != req.OwnerID {
		return ambiguous(ErrPairingResponse)
	}
	if accepted["accepted_contract_version"] != ContractVersion {
		return ambiguous(ErrPairingVersion)
	}
	pairedAt := p.now().UTC()
	backendID := accepted["device_id"].(string)
	rec := domain.DeviceBridgeDevice{DeviceID: domain.DeviceBridgeDeviceID(accepted["device_id"].(string)), OwnerID: req.OwnerID, Label: req.Label, PublicKey: base64.RawURLEncoding.EncodeToString(public), KeyCustodyRef: ref, ContractVersion: ContractVersion, CapabilityClasses: append([]string(nil), req.Capabilities...), State: domain.DeviceBridgeStatePairing, CreatedAt: now, UpdatedAt: pairedAt}
	if err := p.store.CreateDeviceBridgeDevice(ctx, rec); err != nil {
		_, recoveryErr := ambiguous(err)
		p.recovery.DeviceID = backendID
		return empty, recoveryErr
	}
	won, err := p.store.TransitionDeviceBridgeDevice(ctx, rec.DeviceID, domain.DeviceBridgeStatePaired, pairedAt)
	if err != nil || !won {
		if err == nil {
			err = domain.ErrDeviceBridgeConflict
		}
		_, recoveryErr := ambiguous(err)
		p.recovery.DeviceID = backendID
		return empty, recoveryErr
	}
	rec.State = domain.DeviceBridgeStatePaired
	rec.PairedAt = &pairedAt
	fingerprint := sha256.Sum256(public)
	p.logger.Info("device bridge paired", "device_id", rec.DeviceID, "public_key_fingerprint", hex.EncodeToString(fingerprint[:]))
	return rec, nil
}
