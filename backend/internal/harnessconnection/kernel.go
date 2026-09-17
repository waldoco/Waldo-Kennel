// Package harnessconnection owns the fail-closed adapter transport identity
// kernel. It authenticates transport only; owner authority remains separate.
package harnessconnection

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

const bearerBytes = 32

type Kernel struct {
	store    ports.HarnessConnectionStore
	random   io.Reader
	randomMu sync.Mutex
}

func New(store ports.HarnessConnectionStore) *Kernel {
	return &Kernel{store: store, random: rand.Reader}
}
func NewWithRandom(store ports.HarnessConnectionStore, random io.Reader) *Kernel {
	return &Kernel{store: store, random: random}
}

func (k *Kernel) Get(ctx context.Context, id domain.HarnessConnectionID) (domain.HarnessConnection, bool, error) {
	if k == nil || k.store == nil {
		return domain.HarnessConnection{}, false, domain.ErrHarnessConnectionInvalid
	}
	rec, found, err := k.store.GetHarnessConnection(ctx, id)
	if err != nil || !found {
		return domain.HarnessConnection{}, found, err
	}
	return publicConnection(rec), true, nil
}

type IssueRequest struct {
	ConnectionID        domain.HarnessConnectionID
	InstallationID      string
	AdapterDigest       domain.SHA256Digest
	HarnessIdentity     string
	ProviderVersion     string
	ProtocolFingerprint domain.SHA256Digest
	MissionID           string
	AppRunID            string
	CapabilityClasses   []domain.HarnessCapabilityClass
	ExpiresAt           time.Time
	Now                 time.Time
}
type IssuedConnection struct {
	Connection domain.HarnessConnection
	Bearer     string
}

func (k *Kernel) Issue(ctx context.Context, in IssueRequest) (IssuedConnection, error) {
	if k == nil || k.store == nil || k.random == nil {
		return IssuedConnection{}, domain.ErrHarnessConnectionInvalid
	}
	classes, err := domain.NormalizeHarnessCapabilities(in.CapabilityClasses)
	if err != nil {
		return IssuedConnection{}, err
	}
	k.randomMu.Lock()
	bearer, verifier, err := newBearer(k.random)
	k.randomMu.Unlock()
	if err != nil {
		return IssuedConnection{}, err
	}
	rec := domain.HarnessConnection{ID: in.ConnectionID, InstallationID: strings.TrimSpace(in.InstallationID), AdapterDigest: in.AdapterDigest, HarnessIdentity: strings.TrimSpace(in.HarnessIdentity), ProviderVersion: strings.TrimSpace(in.ProviderVersion), ProtocolFingerprint: in.ProtocolFingerprint, MissionID: strings.TrimSpace(in.MissionID), AppRunID: strings.TrimSpace(in.AppRunID), CapabilityClasses: classes, CapabilityVerifier: verifier, Generation: 1, ExpiresAt: in.ExpiresAt.UTC(), CreatedAt: in.Now.UTC(), UpdatedAt: in.Now.UTC()}
	stored, created, err := k.store.CreateHarnessConnection(ctx, rec)
	if err != nil {
		return IssuedConnection{}, err
	}
	if !created {
		// An exact concurrent/retried issuance converges on the durable identity,
		// but never re-returns or replaces the original caller-only bearer.
		return IssuedConnection{Connection: publicConnection(stored)}, nil
	}
	return IssuedConnection{Connection: publicConnection(stored), Bearer: bearer}, nil
}

type Binding struct {
	ConnectionID                                          domain.HarnessConnectionID
	InstallationID                                        string
	AdapterDigest                                         domain.SHA256Digest
	HarnessIdentity, ProviderVersion, MissionID, AppRunID string
	ProtocolFingerprint                                   domain.SHA256Digest
	Generation                                            int64
	Class                                                 domain.HarnessCapabilityClass
}

func (k *Kernel) Authenticate(ctx context.Context, bearer string, binding Binding, now time.Time) (domain.HarnessConnection, error) {
	if k == nil || k.store == nil || strings.TrimSpace(bearer) == "" || !binding.Class.Valid() || now.IsZero() {
		return domain.HarnessConnection{}, domain.ErrHarnessAuthentication
	}
	rec, found, err := k.store.GetHarnessConnection(ctx, binding.ConnectionID)
	if err != nil {
		return domain.HarnessConnection{}, err
	}
	if !found {
		return domain.HarnessConnection{}, domain.ErrHarnessAuthentication
	}
	presented := sha256.Sum256([]byte(bearer))
	stored, decodeErr := hex.DecodeString(rec.CapabilityVerifier)
	valid := 1
	if decodeErr != nil || len(stored) != sha256.Size {
		stored = make([]byte, sha256.Size)
		valid = 0
	}
	valid &= subtle.ConstantTimeCompare(presented[:], stored)
	valid &= equalString(rec.InstallationID, binding.InstallationID)
	valid &= equalString(rec.AdapterDigest.String(), binding.AdapterDigest.String())
	valid &= equalString(rec.HarnessIdentity, binding.HarnessIdentity)
	valid &= equalString(rec.ProviderVersion, binding.ProviderVersion)
	valid &= equalString(rec.ProtocolFingerprint.String(), binding.ProtocolFingerprint.String())
	valid &= equalString(rec.MissionID, binding.MissionID)
	valid &= equalString(rec.AppRunID, binding.AppRunID)
	valid &= equalGeneration(rec.Generation, binding.Generation)
	if rec.RevokedAt != nil || !now.UTC().Before(rec.ExpiresAt) || !rec.HasCapability(binding.Class) {
		valid = 0
	}
	if valid != 1 {
		return domain.HarnessConnection{}, domain.ErrHarnessAuthentication
	}
	return publicConnection(rec), nil
}

func (k *Kernel) Rotate(ctx context.Context, id domain.HarnessConnectionID, generation int64, expiresAt, now time.Time) (IssuedConnection, error) {
	if k == nil || k.store == nil || k.random == nil {
		return IssuedConnection{}, domain.ErrHarnessConnectionInvalid
	}
	k.randomMu.Lock()
	bearer, verifier, err := newBearer(k.random)
	k.randomMu.Unlock()
	if err != nil {
		return IssuedConnection{}, err
	}
	rec, changed, err := k.store.RotateHarnessConnection(ctx, id, generation, verifier, expiresAt.UTC(), now.UTC())
	if err != nil {
		return IssuedConnection{}, err
	}
	if !changed {
		return IssuedConnection{}, domain.ErrHarnessConnectionConflict
	}
	return IssuedConnection{Connection: publicConnection(rec), Bearer: bearer}, nil
}
func (k *Kernel) Revoke(ctx context.Context, id domain.HarnessConnectionID, generation int64, now time.Time) (domain.HarnessConnection, error) {
	rec, changed, err := k.store.RevokeHarnessConnection(ctx, id, generation, now.UTC())
	if err != nil {
		return domain.HarnessConnection{}, err
	}
	if !changed {
		return domain.HarnessConnection{}, domain.ErrHarnessConnectionConflict
	}
	return publicConnection(rec), nil
}

func newBearer(source io.Reader) (string, string, error) {
	raw := make([]byte, bearerBytes)
	if _, err := io.ReadFull(source, raw); err != nil {
		return "", "", err
	}
	bearer := hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(bearer))
	return bearer, hex.EncodeToString(sum[:]), nil
}
func equalString(a, b string) int {
	aDigest := sha256.Sum256([]byte(a))
	bDigest := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(aDigest[:], bDigest[:])
}

func equalGeneration(a, b int64) int {
	var left, right [8]byte
	binary.BigEndian.PutUint64(left[:], uint64(a))
	binary.BigEndian.PutUint64(right[:], uint64(b))
	return subtle.ConstantTimeCompare(left[:], right[:])
}
func publicConnection(in domain.HarnessConnection) domain.HarnessConnection {
	in.CapabilityVerifier = ""
	return in
}
