package ports

import (
	"context"
	"errors"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// ErrAttemptManifestSealed reports a write against a half that already exists.
// Manifests are insert-once: custody evidence is never rewritten.
var ErrAttemptManifestSealed = errors.New("attempt manifest already sealed")

// ErrAttemptManifestDigestMismatch reports that a stored manifest no longer
// matches its sealed digest. The record is refused, never repaired in place.
var ErrAttemptManifestDigestMismatch = errors.New("attempt manifest payload digest mismatch")

// AttemptManifestStore owns the sealed custody manifests of each Attempt:
// the input half admission wrote and the output half custody close wrote.
type AttemptManifestStore interface {
	// SaveAttemptManifest inserts one sealed half. It returns
	// ErrAttemptManifestSealed when that half already exists for the Attempt.
	SaveAttemptManifest(context.Context, domain.AttemptManifest) error
	// GetAttemptManifest reads one half, verifying the payload against its
	// sealed digest before returning it. A mismatch is
	// ErrAttemptManifestDigestMismatch, never a silently returned record.
	GetAttemptManifest(context.Context, domain.AttemptID, domain.AttemptManifestHalf) (domain.AttemptManifest, bool, error)
	// ListAttemptManifestsForOutcome returns every sealed half of every
	// Attempt of the Outcome, each verified against its sealed digest.
	ListAttemptManifestsForOutcome(context.Context, domain.OutcomeID) ([]domain.AttemptManifest, error)
}
