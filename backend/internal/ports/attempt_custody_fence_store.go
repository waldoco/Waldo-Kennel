package ports

import (
	"context"
	"errors"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// ErrAttemptCustodyFenceSealed reports a second fence for an Attempt that
// already crossed custody close. The fence is exactly-once.
var ErrAttemptCustodyFenceSealed = errors.New("attempt custody fence already recorded")

// AttemptCustodyFenceStore owns the typed custody-close fences: the durable
// record that an Attempt's provider had exited and its workspace had settled
// before the result snapshot was taken.
type AttemptCustodyFenceStore interface {
	// RecordAttemptCustodyFence writes the fence, insert-once. A second
	// record for the same Attempt returns ErrAttemptCustodyFenceSealed.
	RecordAttemptCustodyFence(context.Context, domain.AttemptCustodyFence) error
	// GetAttemptCustodyFence reads the fence of one Attempt.
	GetAttemptCustodyFence(context.Context, domain.AttemptID) (domain.AttemptCustodyFence, bool, error)
}
