// Package harnesspairing implements the S3.3 one-time pairing/rotation
// challenge lifecycle. It authenticates adapter transport possession only
// and wires a successful proof into the frozen S3.1 harnessconnection
// kernel; it creates no owner authority of its own and never touches
// harness_connection.go, ports/harness_connection.go, or the kernel's
// authority/capability semantics.
package harnesspairing

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"io"
	"strings"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/harnessconnection"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

const (
	challengeSecretBytes = 32
	challengeIDBytes     = 16
)

// Coordinator owns the pairing-challenge lifecycle. It is the only package
// permitted to call into the frozen S3.1 kernel on a proved challenge's
// behalf; it duplicates none of the kernel's own tuple/capability checks.
type Coordinator struct {
	store  ports.HarnessPairingChallengeStore
	kernel *harnessconnection.Kernel
	random io.Reader
}

func New(store ports.HarnessPairingChallengeStore, kernel *harnessconnection.Kernel) *Coordinator {
	return &Coordinator{store: store, kernel: kernel, random: rand.Reader}
}

func NewWithRandom(store ports.HarnessPairingChallengeStore, kernel *harnessconnection.Kernel, random io.Reader) *Coordinator {
	return &Coordinator{store: store, kernel: kernel, random: random}
}

// IssueChallengeRequest is supplied by an already-authenticated internal
// caller — the S1 owner path, out of this slice's scope — that already knows
// the exact tuple it expects the adapter to present. The adapter never
// selects any of these fields, including ConnectionExpiresAt: the resulting
// S3.1 connection's expiry is fixed here, at issue time.
type IssueChallengeRequest struct {
	Kind                domain.HarnessPairingKind
	ConnectionID        domain.HarnessConnectionID
	InstallationID      string
	AdapterDigest       domain.SHA256Digest
	HarnessIdentity     string
	ProviderVersion     string
	ProtocolFingerprint domain.SHA256Digest
	MissionID           string
	AppRunID            string
	CapabilityClasses   []domain.HarnessCapabilityClass
	// ExpectedGeneration is 1 for Kind==pair (Kernel.Issue always mints
	// generation 1) and the CURRENT generation being superseded for
	// Kind==rotate (Kernel.Rotate's own CAS target).
	ExpectedGeneration  int64
	ConnectionExpiresAt time.Time
	TTL                 time.Duration
	Now                 time.Time
}

// IssuedChallenge is returned once, to the trusted issuing caller only. The
// caller is responsible for delivering Secret to the adapter over a
// protected local channel; the coordinator never persists or logs it.
type IssuedChallenge struct {
	Challenge domain.HarnessPairingChallenge
	Secret    domain.PairingChallengeSecret
}

// Issue mints a fresh, single-use challenge and supersedes any prior pending
// challenge for the same connection ID.
func (c *Coordinator) Issue(ctx context.Context, req IssueChallengeRequest) (IssuedChallenge, error) {
	if c == nil || c.store == nil || c.kernel == nil || c.random == nil {
		return IssuedChallenge{}, domain.ErrHarnessPairingInvalid
	}
	if !req.Kind.Valid() || strings.TrimSpace(string(req.ConnectionID)) == "" || req.TTL <= 0 ||
		req.Now.IsZero() || req.ConnectionExpiresAt.IsZero() || req.ExpectedGeneration < 1 {
		return IssuedChallenge{}, domain.ErrHarnessPairingInvalid
	}
	if req.Kind == domain.HarnessPairingKindPair && req.ExpectedGeneration != 1 {
		return IssuedChallenge{}, domain.ErrHarnessPairingInvalid
	}
	classes, err := domain.NormalizeHarnessCapabilities(req.CapabilityClasses)
	if err != nil {
		return IssuedChallenge{}, err
	}

	id, err := randomToken(c.random, challengeIDBytes)
	if err != nil {
		return IssuedChallenge{}, err
	}
	secret, verifier, err := randomSecretAndVerifier(c.random)
	if err != nil {
		return IssuedChallenge{}, err
	}

	if _, err := c.store.SupersedePendingHarnessPairingChallenges(ctx, req.ConnectionID, req.Now.UTC()); err != nil {
		return IssuedChallenge{}, err
	}

	rec := domain.HarnessPairingChallenge{
		ID:                  domain.PairingChallengeID(id),
		Kind:                req.Kind,
		ConnectionID:        req.ConnectionID,
		InstallationID:      strings.TrimSpace(req.InstallationID),
		AdapterDigest:       req.AdapterDigest,
		HarnessIdentity:     strings.TrimSpace(req.HarnessIdentity),
		ProviderVersion:     strings.TrimSpace(req.ProviderVersion),
		ProtocolFingerprint: req.ProtocolFingerprint,
		MissionID:           strings.TrimSpace(req.MissionID),
		AppRunID:            strings.TrimSpace(req.AppRunID),
		CapabilityClasses:   classes,
		ExpectedGeneration:  req.ExpectedGeneration,
		ProofVerifier:       verifier,
		Status:              domain.HarnessPairingPending,
		ConnectionExpiresAt: req.ConnectionExpiresAt.UTC(),
		ExpiresAt:           req.Now.Add(req.TTL).UTC(),
		CreatedAt:           req.Now.UTC(),
		UpdatedAt:           req.Now.UTC(),
	}
	stored, created, err := c.store.CreateHarnessPairingChallenge(ctx, rec)
	if err != nil {
		return IssuedChallenge{}, err
	}
	if !created {
		return IssuedChallenge{}, domain.ErrHarnessPairingConflict
	}
	return IssuedChallenge{Challenge: redacted(stored), Secret: domain.PairingChallengeSecret(secret)}, nil
}

// ProveRequest is the tuple an adapter presents over the protected local
// transport. Every field is compared, constant-time, against the challenge's
// stored binding; a mismatch on any single field is indistinguishable from a
// wrong secret to the caller.
type ProveRequest struct {
	ChallengeID                                     domain.PairingChallengeID
	Secret                                           domain.PairingChallengeSecret
	InstallationID, HarnessIdentity, ProviderVersion string
	MissionID, AppRunID                              string
	AdapterDigest, ProtocolFingerprint               domain.SHA256Digest
	CapabilityClasses                                []domain.HarnessCapabilityClass
	ExpectedGeneration                                int64
	Now                                               time.Time
}

type ProveResult struct {
	Issued harnessconnection.IssuedConnection
}

// Prove is fail-closed: every failure path returns domain.ErrHarnessPairingFailed.
// The returned domain.HarnessPairingResultCode is daemon-local audit
// evidence only. The transport layer MUST NOT let this code, or anything
// derived from it, reach the wire — every failure must produce byte-identical
// bytes to the peer regardless of which code fired.
func (c *Coordinator) Prove(ctx context.Context, req ProveRequest) (ProveResult, domain.HarnessPairingResultCode, error) {
	if c == nil || c.store == nil || c.kernel == nil ||
		strings.TrimSpace(string(req.ChallengeID)) == "" || !req.Secret.Valid() || req.Now.IsZero() {
		return ProveResult{}, domain.HarnessPairingResultNotFound, domain.ErrHarnessPairingFailed
	}
	classes, classErr := domain.NormalizeHarnessCapabilities(req.CapabilityClasses)

	rec, found, err := c.store.GetHarnessPairingChallenge(ctx, req.ChallengeID)
	if err != nil {
		return ProveResult{}, domain.HarnessPairingResultInternalError, err
	}
	if !found {
		return ProveResult{}, domain.HarnessPairingResultNotFound, domain.ErrHarnessPairingFailed
	}

	valid := compareTuple(rec, req, classes, classErr)
	notExpired := req.Now.UTC().Before(rec.ExpiresAt)
	isPending := rec.Status == domain.HarnessPairingPending

	if valid != 1 || !notExpired || !isPending {
		code := classifyFailure(rec, notExpired, classErr)
		if !isPending {
			// The row is already terminal (replayed/superseded): recording
			// is corroborating evidence, harmless if already set.
			c.recordResultBestEffort(ctx, rec.ID, code, req.Now)
		}
		// A rejected attempt against a still-pending challenge (wrong
		// tuple/secret/class, or expiry not yet swept) is NOT this
		// challenge's terminal outcome: the legitimate holder may still
		// prove it correctly afterward. Recording here would permanently
		// poison the audit trail with a failure code for what may become a
		// successfully paired challenge.
		return ProveResult{}, code, domain.ErrHarnessPairingFailed
	}

	// Point of no return: only a CAS-won consume may proceed to the kernel.
	// Losing this race — including an exact retry of this same request —
	// must not call the kernel again or mint a second bearer.
	consumed, err := c.store.ConsumeHarnessPairingChallenge(ctx, rec.ID, req.Now)
	if err != nil {
		return ProveResult{}, domain.HarnessPairingResultInternalError, err
	}
	if !consumed {
		c.recordResultBestEffort(ctx, rec.ID, domain.HarnessPairingResultReplayed, req.Now)
		return ProveResult{}, domain.HarnessPairingResultReplayed, domain.ErrHarnessPairingFailed
	}

	issued, err := c.issueOrRotate(ctx, rec)
	if err != nil {
		c.recordResultBestEffort(ctx, rec.ID, domain.HarnessPairingResultInternalError, req.Now)
		return ProveResult{}, domain.HarnessPairingResultInternalError, domain.ErrHarnessPairingFailed
	}
	if issued.Bearer == "" {
		// The kernel silently converged on an already-existing connection
		// generation instead of minting a fresh bearer (harnessconnection.
		// Kernel.Issue's exact-retry convergence path). The challenge is
		// already burned and cannot be replayed, so no bearer can ever reach
		// this caller: fail closed. A new challenge is required.
		c.recordResultBestEffort(ctx, rec.ID, domain.HarnessPairingResultBearerUnavailable, req.Now)
		return ProveResult{}, domain.HarnessPairingResultBearerUnavailable, domain.ErrHarnessPairingFailed
	}
	c.recordResultBestEffort(ctx, rec.ID, domain.HarnessPairingResultSucceeded, req.Now)
	return ProveResult{Issued: issued}, domain.HarnessPairingResultSucceeded, nil
}

func (c *Coordinator) issueOrRotate(ctx context.Context, rec domain.HarnessPairingChallenge) (harnessconnection.IssuedConnection, error) {
	switch rec.Kind {
	case domain.HarnessPairingKindPair:
		return c.kernel.Issue(ctx, harnessconnection.IssueRequest{
			ConnectionID: rec.ConnectionID, InstallationID: rec.InstallationID, AdapterDigest: rec.AdapterDigest,
			HarnessIdentity: rec.HarnessIdentity, ProviderVersion: rec.ProviderVersion, ProtocolFingerprint: rec.ProtocolFingerprint,
			MissionID: rec.MissionID, AppRunID: rec.AppRunID, CapabilityClasses: rec.CapabilityClasses,
			ExpiresAt: rec.ConnectionExpiresAt, Now: rec.UpdatedAt,
		})
	case domain.HarnessPairingKindRotate:
		return c.kernel.Rotate(ctx, rec.ConnectionID, rec.ExpectedGeneration, rec.ConnectionExpiresAt, rec.UpdatedAt)
	default:
		return harnessconnection.IssuedConnection{}, domain.ErrHarnessPairingInvalid
	}
}

// recordResultBestEffort attaches non-secret audit evidence. Its own failure
// is deliberately swallowed: a Prove outcome the caller already has must
// never flip to an internal error because the audit write failed, and a
// crash here is exactly the "consumed with no recorded result" state the
// contract allows.
func (c *Coordinator) recordResultBestEffort(ctx context.Context, id domain.PairingChallengeID, code domain.HarnessPairingResultCode, now time.Time) {
	_, _ = c.store.RecordHarnessPairingResult(ctx, id, code, now)
}

func classifyFailure(rec domain.HarnessPairingChallenge, notExpired bool, classErr error) domain.HarnessPairingResultCode {
	switch {
	case classErr != nil:
		return domain.HarnessPairingResultUndeclaredClass
	case rec.Status == domain.HarnessPairingConsumed:
		return domain.HarnessPairingResultReplayed
	case rec.Status == domain.HarnessPairingSuperseded:
		return domain.HarnessPairingResultSuperseded
	case !notExpired:
		return domain.HarnessPairingResultExpired
	default:
		return domain.HarnessPairingResultTupleMismatch
	}
}

// compareTuple constant-time-compares every bound field so a caller cannot
// learn, by timing or by response shape, which field or byte of the secret
// was wrong. It mirrors the hash-then-subtle.ConstantTimeCompare idiom in
// harnessconnection.Kernel.Authenticate exactly, rather than reimplementing
// a weaker variant.
func compareTuple(rec domain.HarnessPairingChallenge, req ProveRequest, classes []domain.HarnessCapabilityClass, classErr error) int {
	presented := sha256.Sum256([]byte(string(req.Secret)))
	stored, decodeErr := hex.DecodeString(rec.ProofVerifier)
	valid := 1
	if decodeErr != nil || len(stored) != sha256.Size {
		stored = make([]byte, sha256.Size)
		valid = 0
	}
	valid &= subtle.ConstantTimeCompare(presented[:], stored)
	valid &= equalString(rec.InstallationID, req.InstallationID)
	valid &= equalString(rec.AdapterDigest.String(), req.AdapterDigest.String())
	valid &= equalString(rec.HarnessIdentity, req.HarnessIdentity)
	valid &= equalString(rec.ProviderVersion, req.ProviderVersion)
	valid &= equalString(rec.ProtocolFingerprint.String(), req.ProtocolFingerprint.String())
	valid &= equalString(rec.MissionID, req.MissionID)
	valid &= equalString(rec.AppRunID, req.AppRunID)
	valid &= equalGeneration(rec.ExpectedGeneration, req.ExpectedGeneration)
	if classErr == nil {
		valid &= equalString(encodeClasses(rec.CapabilityClasses), encodeClasses(classes))
	} else {
		valid = 0
	}
	return valid
}

func equalString(a, b string) int {
	left := sha256.Sum256([]byte(a))
	right := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(left[:], right[:])
}

// equalGeneration mirrors harnessconnection.Kernel.Authenticate's own
// generation comparison idiom exactly (kernel.go's equalGeneration), so a
// wrong challenge generation is checked with the same constant-time
// technique rather than a plain integer `==`.
func equalGeneration(a, b int64) int {
	var left, right [8]byte
	binary.BigEndian.PutUint64(left[:], uint64(a))
	binary.BigEndian.PutUint64(right[:], uint64(b))
	return subtle.ConstantTimeCompare(left[:], right[:])
}

func encodeClasses(in []domain.HarnessCapabilityClass) string {
	values := make([]string, len(in))
	for i, c := range in {
		values[i] = string(c)
	}
	return strings.Join(values, ",")
}

func randomToken(source io.Reader, n int) (string, error) {
	raw := make([]byte, n)
	if _, err := io.ReadFull(source, raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func randomSecretAndVerifier(source io.Reader) (secret, verifier string, err error) {
	secret, err = randomToken(source, challengeSecretBytes)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(secret))
	return secret, hex.EncodeToString(sum[:]), nil
}

// redacted strips the one-way verifier before a challenge record ever
// leaves this package, mirroring harnessconnection's publicConnection.
func redacted(in domain.HarnessPairingChallenge) domain.HarnessPairingChallenge {
	in.ProofVerifier = ""
	return in
}
