package ownerproof

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"io"
	"strings"
	"sync"
	"time"
)

type Kernel struct {
	store    ports.OwnerProofStore
	random   io.Reader
	randomMu sync.Mutex
}

func New(store ports.OwnerProofStore) *Kernel { return &Kernel{store: store, random: rand.Reader} }
func NewWithRandom(store ports.OwnerProofStore, r io.Reader) *Kernel {
	return &Kernel{store: store, random: r}
}

type MintRequest struct {
	ID                  domain.OwnerProofID
	AppRunID, MissionID string
	ContentDigest       domain.SHA256Digest
	TargetDigest        domain.SHA256Digest
	Class               domain.OwnerCommandClass
	ConfirmationRef     string
	ExpiresAt, Now      time.Time
}
type Minted struct {
	Proof  domain.OwnerProof
	Bearer string
}

func (k *Kernel) Mint(ctx context.Context, in MintRequest) (Minted, error) {
	if k == nil || k.store == nil || k.random == nil {
		return Minted{}, domain.ErrOwnerProofInvalid
	}
	raw := make([]byte, 32)
	k.randomMu.Lock()
	_, randomErr := io.ReadFull(k.random, raw)
	k.randomMu.Unlock()
	if randomErr != nil {
		return Minted{}, randomErr
	}
	bearer := hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(bearer))
	p := domain.OwnerProof{ID: in.ID, Verifier: domain.SHA256Digest(hex.EncodeToString(sum[:])), AppRunID: strings.TrimSpace(in.AppRunID), MissionID: strings.TrimSpace(in.MissionID), ContentDigest: in.ContentDigest, TargetDigest: in.TargetDigest, Class: in.Class, ConfirmationRef: strings.TrimSpace(in.ConfirmationRef), ExpiresAt: in.ExpiresAt.UTC(), CreatedAt: in.Now.UTC()}
	if err := p.Validate(); err != nil {
		return Minted{}, err
	}
	stored, created, err := k.store.CreateOwnerProof(ctx, p)
	if err != nil {
		return Minted{}, err
	}
	if !created {
		return Minted{Proof: public(stored)}, nil
	}
	return Minted{Proof: public(stored), Bearer: bearer}, nil
}

type Binding struct {
	ID                  domain.OwnerProofID
	AppRunID, MissionID string
	ContentDigest       domain.SHA256Digest
	TargetDigest        domain.SHA256Digest
	Class               domain.OwnerCommandClass
	ConfirmationRef     string
}

func (k *Kernel) VerifyAndConsume(ctx context.Context, bearer string, b Binding, now time.Time) (domain.OwnerProof, error) {
	if k == nil || k.store == nil || bearer == "" || now.IsZero() {
		return domain.OwnerProof{}, domain.ErrOwnerProofAuthentication
	}
	p, ok, err := k.store.GetOwnerProof(ctx, b.ID)
	if err != nil {
		return domain.OwnerProof{}, err
	}
	if !ok {
		return domain.OwnerProof{}, domain.ErrOwnerProofAuthentication
	}
	sum := sha256.Sum256([]byte(bearer))
	stored, decodeErr := hex.DecodeString(p.Verifier.String())
	valid := 1
	if decodeErr != nil || len(stored) != sha256.Size {
		stored = make([]byte, sha256.Size)
		valid = 0
	}
	valid &= subtle.ConstantTimeCompare(sum[:], stored)
	for _, pair := range [][2]string{{p.AppRunID, b.AppRunID}, {p.MissionID, b.MissionID}, {p.ContentDigest.String(), b.ContentDigest.String()}, {p.TargetDigest.String(), b.TargetDigest.String()}, {string(p.Class), string(b.Class)}, {p.ConfirmationRef, b.ConfirmationRef}} {
		valid &= equal(pair[0], pair[1])
	}
	if p.ConsumedAt != nil || !now.UTC().Before(p.ExpiresAt) {
		valid = 0
	}
	if valid != 1 {
		return domain.OwnerProof{}, domain.ErrOwnerProofAuthentication
	}
	consumed, changed, err := k.store.ConsumeOwnerProof(ctx, p.ID, now.UTC())
	if err != nil {
		return domain.OwnerProof{}, err
	}
	if !changed {
		return domain.OwnerProof{}, domain.ErrOwnerProofAuthentication
	}
	return public(consumed), nil
}
func equal(a, b string) int {
	x := sha256.Sum256([]byte(a))
	y := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(x[:], y[:])
}
func equalInt(a, b int64) int { return equal(fmtInt(a), fmtInt(b)) }
func fmtInt(v int64) string {
	var buf [20]byte
	i := len(buf)
	if v == 0 {
		return "0"
	}
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
func public(p domain.OwnerProof) domain.OwnerProof { p.Verifier = ""; return p }
