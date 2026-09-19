package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/gen"
)

// Typed custody-close fences. See migration 0160.

// RecordAttemptCustodyFence writes the fence, insert-once: the primary key
// refuses a second fence, surfaced as ErrAttemptCustodyFenceSealed.
func (s *Store) RecordAttemptCustodyFence(ctx context.Context, fence domain.AttemptCustodyFence) error {
	if err := fence.Validate(); err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	err := s.qw.InsertAttemptCustodyFence(ctx, gen.InsertAttemptCustodyFenceParams{
		AttemptID: string(fence.AttemptID),
		SessionID: fence.SessionID,
		Detail:    fence.Detail,
		FencedAt:  fence.FencedAt.UTC(),
	})
	switch {
	case err == nil:
		return nil
	case isSQLiteUnique(err) || isSQLitePrimaryKey(err):
		return ports.ErrAttemptCustodyFenceSealed
	default:
		return fmt.Errorf("record custody fence for %s: %w", fence.AttemptID, err)
	}
}

func (s *Store) GetAttemptCustodyFence(ctx context.Context, attemptID domain.AttemptID) (domain.AttemptCustodyFence, bool, error) {
	row, err := s.qr.GetAttemptCustodyFence(ctx, string(attemptID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AttemptCustodyFence{}, false, nil
	}
	if err != nil {
		return domain.AttemptCustodyFence{}, false, fmt.Errorf("read custody fence for %s: %w", attemptID, err)
	}
	fence := domain.AttemptCustodyFence{
		AttemptID: domain.AttemptID(row.AttemptID),
		SessionID: row.SessionID,
		Detail:    row.Detail,
		FencedAt:  row.FencedAt,
	}
	if err := fence.Validate(); err != nil {
		return domain.AttemptCustodyFence{}, false, fmt.Errorf("stored custody fence for %s is invalid: %w", attemptID, err)
	}
	return fence, true, nil
}
