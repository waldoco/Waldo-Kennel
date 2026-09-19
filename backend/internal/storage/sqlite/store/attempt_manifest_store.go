package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/gen"
	moderncsqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// Sealed per-attempt custody manifests. See migration 0159.

// SaveAttemptManifest inserts one sealed manifest half. The half is
// insert-once: the primary key refuses a second write, which is surfaced as
// ErrAttemptManifestSealed rather than a raw constraint error.
func (s *Store) SaveAttemptManifest(ctx context.Context, manifest domain.AttemptManifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	err := s.qw.InsertAttemptManifest(ctx, gen.InsertAttemptManifestParams{
		AttemptID:     string(manifest.AttemptID),
		Half:          string(manifest.Half),
		OutcomeID:     string(manifest.OutcomeID),
		Payload:       string(manifest.Payload),
		PayloadDigest: manifest.PayloadDigest.String(),
		CreatedAt:     manifest.RecordedAt.UTC(),
	})
	switch {
	case err == nil:
		return nil
	case isSQLiteUnique(err) || isSQLitePrimaryKey(err):
		return ports.ErrAttemptManifestSealed
	default:
		return fmt.Errorf("save attempt manifest %s/%s: %w", manifest.AttemptID, manifest.Half, err)
	}
}

// manifestFromGen reseals a stored row only after the payload proves it still
// matches its digest. A tampered record is refused, never returned.
func manifestFromGen(row gen.AttemptManifest) (domain.AttemptManifest, error) {
	manifest := domain.AttemptManifest{
		AttemptID:     domain.AttemptID(row.AttemptID),
		OutcomeID:     domain.OutcomeID(row.OutcomeID),
		Half:          domain.AttemptManifestHalf(row.Half),
		Payload:       []byte(row.Payload),
		PayloadDigest: domain.SHA256Digest(row.PayloadDigest),
		RecordedAt:    row.CreatedAt,
	}
	if err := manifest.Validate(); err != nil {
		return domain.AttemptManifest{}, errors.Join(ports.ErrAttemptManifestDigestMismatch, err)
	}
	return manifest, nil
}

func (s *Store) GetAttemptManifest(ctx context.Context, attemptID domain.AttemptID, half domain.AttemptManifestHalf) (domain.AttemptManifest, bool, error) {
	row, err := s.qr.GetAttemptManifest(ctx, gen.GetAttemptManifestParams{AttemptID: string(attemptID), Half: string(half)})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AttemptManifest{}, false, nil
	}
	if err != nil {
		return domain.AttemptManifest{}, false, fmt.Errorf("read attempt manifest %s/%s: %w", attemptID, half, err)
	}
	manifest, err := manifestFromGen(row)
	if err != nil {
		return domain.AttemptManifest{}, false, err
	}
	return manifest, true, nil
}

func (s *Store) ListAttemptManifestsForOutcome(ctx context.Context, outcomeID domain.OutcomeID) ([]domain.AttemptManifest, error) {
	rows, err := s.qr.ListAttemptManifestsForOutcome(ctx, string(outcomeID))
	if err != nil {
		return nil, fmt.Errorf("list attempt manifests for %s: %w", outcomeID, err)
	}
	manifests := make([]domain.AttemptManifest, 0, len(rows))
	for _, row := range rows {
		manifest, err := manifestFromGen(row)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, manifest)
	}
	return manifests, nil
}

// isSQLitePrimaryKey matches a PRIMARY KEY violation. Text primary keys are
// enforced by a unique index, so modernc reports SQLITE_CONSTRAINT_PRIMARYKEY
// (1555) rather than SQLITE_CONSTRAINT_UNIQUE; the shared isSQLiteUnique
// helper intentionally covers only the latter and is left untouched.
func isSQLitePrimaryKey(err error) bool {
	var sqliteErr *moderncsqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY
}
