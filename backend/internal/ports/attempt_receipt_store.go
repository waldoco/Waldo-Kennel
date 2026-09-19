package ports

import (
	"context"
	"errors"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// ErrAttemptReceiptFrozen reports a refused overwrite of a receipt that has
// already served as review evidence.
//
// Freezing is what makes "what the owner reviewed" a stable thing. Without it,
// a later retention pass over the same workspace could replace the manifest an
// acceptance decision was made against.
var ErrAttemptReceiptFrozen = errors.New("attempt receipt is frozen and cannot be replaced")

// ErrAttemptReceiptDiverged reports a refused overwrite of a durable COMPLETE
// receipt by a receipt with a different artifact version. Two retention paths
// racing one Attempt must converge on one canonical snapshot: an identical
// replay is a no-op, a divergent one is refused BEFORE anything is
// overwritten. An incomplete receipt carries no such guarantee and may still
// be replaced by the retry that finishes it.
var ErrAttemptReceiptDiverged = errors.New("attempt receipt diverges from the durable complete receipt")

// ErrAttemptReceiptMissing means an Attempt has no retained result to judge.
// Absence is not an empty artifact: an empty result must be represented by an
// explicit retained receipt whose manifest is empty.
var ErrAttemptReceiptMissing = errors.New("attempt receipt is missing")

// ErrAttemptReceiptNotReady means retention did not produce a complete result
// at the version the caller is trying to review.
var ErrAttemptReceiptNotReady = errors.New("attempt receipt is not a complete retained result")

// ErrAttemptClassificationStale means the optimistic Attempt status or
// artifact version changed before the classification transaction committed.
var ErrAttemptClassificationStale = errors.New("attempt classification became stale")

// AttemptReceiptStore owns the durable record of what each Attempt produced.
//
// Provider claims about output are claims. This store holds Kennel's own
// observation of the workspace, which is what a downstream WorkUnit consumes
// and what a delivery manifest is built from.
type AttemptReceiptStore interface {
	// SaveAttemptReceipt writes the receipt and its file manifest atomically,
	// refusing with ErrAttemptReceiptFrozen once the receipt is frozen.
	SaveAttemptReceipt(context.Context, domain.AttemptReceipt) error
	GetAttemptReceipt(context.Context, domain.AttemptID) (domain.AttemptReceipt, bool, error)
	// FreezeAttemptReceipt marks the receipt as review evidence. Idempotent.
	FreezeAttemptReceipt(context.Context, domain.AttemptID, time.Time) error
}

// ClassifyAttemptInput is the complete database-side transition from an ended
// Attempt to a successful, reviewed result. Implementations must commit the
// status, receipt freeze, observation and custody release as one unit.
type ClassifyAttemptInput struct {
	OutcomeID       domain.OutcomeID
	AttemptID       domain.AttemptID
	ExpectedStatus  domain.AttemptStatus
	ArtifactVersion string
	// ContractRevisionNumber is the revision the classification was judged
	// under. The transaction refuses if the Outcome has since been revised,
	// because proof bound to the old revision cannot classify work under a new
	// one.
	ContractRevisionNumber int64
	// ProofGeneration is the append-only proof record count observed before
	// reading proof. Nil means no validated observation was made.
	ProofGeneration    *int64
	ObservationKind    string
	ObservationPayload string
	At                 time.Time
}

// AttemptSuccessFinalizer prevents the terminal service from implementing a
// crash-sensitive multi-write protocol above storage.
type AttemptSuccessFinalizer interface {
	OutcomeProofGeneration(context.Context, domain.OutcomeID) (int64, error)
	ClassifyAttemptSucceeded(context.Context, ClassifyAttemptInput) error
}

// AttemptRetainer materializes the daemon-owned workspace for an ended
// Attempt. It is separate from AttemptReceiptStore because filesystem
// publication and database receipt persistence have different crash/retry
// boundaries.
type AttemptRetainer interface {
	RetainAttempt(context.Context, domain.Attempt) error
}
