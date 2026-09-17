package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// PairingChallengeID identifies one one-time pairing or rotation challenge.
// It is never the secret itself and is safe to log.
type PairingChallengeID string

// HarnessPairingKind selects which frozen S3.1 kernel operation a proved
// challenge drives: Issue for a fresh connection, Rotate for an existing one.
type HarnessPairingKind string

const (
	HarnessPairingKindPair   HarnessPairingKind = "pair"
	HarnessPairingKindRotate HarnessPairingKind = "rotate"
)

func (k HarnessPairingKind) Valid() bool {
	return k == HarnessPairingKindPair || k == HarnessPairingKindRotate
}

// HarnessPairingStatus is the durable challenge lifecycle state. It is
// written exactly once at issue (pending) and exactly once more at its
// terminal transition (consumed or superseded); it is never reverted.
type HarnessPairingStatus string

const (
	HarnessPairingPending    HarnessPairingStatus = "pending"
	HarnessPairingConsumed   HarnessPairingStatus = "consumed"
	HarnessPairingSuperseded HarnessPairingStatus = "superseded"
)

func (s HarnessPairingStatus) Valid() bool {
	switch s {
	case HarnessPairingPending, HarnessPairingConsumed, HarnessPairingSuperseded:
		return true
	default:
		return false
	}
}

// HarnessPairingResultCode is a closed, non-secret audit classification kept
// as daemon-local evidence. It is never sent to a caller: every wire failure
// collapses to one generic response regardless of which code produced it, so
// a peer cannot learn which tuple field or proof byte was wrong.
type HarnessPairingResultCode string

const (
	HarnessPairingResultSucceeded         HarnessPairingResultCode = "succeeded"
	HarnessPairingResultReplayed          HarnessPairingResultCode = "replayed"
	HarnessPairingResultExpired           HarnessPairingResultCode = "expired"
	HarnessPairingResultSuperseded        HarnessPairingResultCode = "superseded"
	HarnessPairingResultTupleMismatch     HarnessPairingResultCode = "tuple_mismatch"
	HarnessPairingResultUndeclaredClass   HarnessPairingResultCode = "undeclared_capability_class"
	HarnessPairingResultNotFound          HarnessPairingResultCode = "not_found"
	HarnessPairingResultBearerUnavailable HarnessPairingResultCode = "bearer_unavailable"
	HarnessPairingResultInternalError     HarnessPairingResultCode = "internal_error"
)

var harnessPairingResultCodes = map[HarnessPairingResultCode]struct{}{
	HarnessPairingResultSucceeded: {}, HarnessPairingResultReplayed: {}, HarnessPairingResultExpired: {},
	HarnessPairingResultSuperseded: {}, HarnessPairingResultTupleMismatch: {}, HarnessPairingResultUndeclaredClass: {},
	HarnessPairingResultNotFound: {}, HarnessPairingResultBearerUnavailable: {}, HarnessPairingResultInternalError: {},
}

func (c HarnessPairingResultCode) Valid() bool { _, ok := harnessPairingResultCodes[c]; return ok }

var (
	ErrHarnessPairingInvalid  = errors.New("invalid harness pairing challenge")
	ErrHarnessPairingFailed   = errors.New("harness pairing proof failed")
	ErrHarnessPairingConflict = errors.New("harness pairing challenge conflict")
)

// HarnessPairingChallenge is the durable, non-secret record of one pairing or
// rotation challenge. Only a one-way verifier of the raw challenge secret is
// stored; the secret itself is never durable. This is not a HarnessConnection
// and grants no authority by itself.
type HarnessPairingChallenge struct {
	ID                  PairingChallengeID
	Kind                HarnessPairingKind
	ConnectionID        HarnessConnectionID
	InstallationID      string
	AdapterDigest       SHA256Digest
	HarnessIdentity     string
	ProviderVersion     string
	ProtocolFingerprint SHA256Digest
	MissionID           string
	AppRunID            string
	CapabilityClasses   []HarnessCapabilityClass
	ExpectedGeneration  int64
	// ConnectionExpiresAt is the expiry the issuing caller decided for the
	// resulting S3.1 connection generation, fixed at issue time so the
	// adapter can never influence its own bearer's expiry at prove time.
	ConnectionExpiresAt time.Time
	ExpiresAt           time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
	Status              HarnessPairingStatus
	ResultCode          *HarnessPairingResultCode
	// ProofVerifier is a one-way verifier of the raw challenge secret. It is
	// intentionally excluded from JSON and String so a durable record can
	// never become a usable proof.
	ProofVerifier string `json:"-"`
}

func (c HarnessPairingChallenge) String() string {
	return fmt.Sprintf("HarnessPairingChallenge(%s,kind=%s,status=%s)", c.ID, c.Kind, c.Status)
}

func (c HarnessPairingChallenge) Validate() error {
	if strings.TrimSpace(string(c.ID)) == "" || !c.Kind.Valid() || strings.TrimSpace(string(c.ConnectionID)) == "" ||
		strings.TrimSpace(c.InstallationID) == "" || !c.AdapterDigest.Valid() || strings.TrimSpace(c.HarnessIdentity) == "" ||
		strings.TrimSpace(c.ProviderVersion) == "" || !c.ProtocolFingerprint.Valid() ||
		strings.TrimSpace(c.MissionID) == "" || strings.TrimSpace(c.AppRunID) == "" ||
		c.ExpectedGeneration < 1 || c.ExpiresAt.IsZero() || c.CreatedAt.IsZero() || c.UpdatedAt.IsZero() ||
		c.ConnectionExpiresAt.IsZero() || !SHA256Digest(c.ProofVerifier).Valid() || len(c.CapabilityClasses) == 0 ||
		!c.ExpiresAt.After(c.CreatedAt) || c.UpdatedAt.Before(c.CreatedAt) || !c.Status.Valid() {
		return ErrHarnessPairingInvalid
	}
	if c.ResultCode != nil && !c.ResultCode.Valid() {
		return ErrHarnessPairingInvalid
	}
	seen := make(map[HarnessCapabilityClass]struct{}, len(c.CapabilityClasses))
	for _, class := range c.CapabilityClasses {
		if !class.Valid() {
			return ErrHarnessPairingInvalid
		}
		if _, ok := seen[class]; ok {
			return ErrHarnessPairingInvalid
		}
		seen[class] = struct{}{}
	}
	return nil
}

// PairingChallengeSecret is the caller-only, one-time possession secret. Its
// String and MarshalJSON are redacted so an accidental %v, log line, or
// exported JSON shape can never disclose it.
type PairingChallengeSecret string

func (PairingChallengeSecret) String() string { return "[redacted-pairing-challenge-secret]" }

func (PairingChallengeSecret) MarshalJSON() ([]byte, error) { return []byte(`"[redacted]"`), nil }

// Valid reports whether the secret is present. It intentionally does not
// validate shape/length beyond emptiness: the coordinator is the sole place
// that compares a presented secret against a stored verifier.
func (s PairingChallengeSecret) Valid() bool { return strings.TrimSpace(string(s)) != "" }
