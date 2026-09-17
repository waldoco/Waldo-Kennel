package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/gen"
	"time"
)

var _ ports.OwnerProofStore = (*Store)(nil)

func (s *Store) CreateOwnerProof(ctx context.Context, p domain.OwnerProof) (domain.OwnerProof, bool, error) {
	if err := p.Validate(); err != nil {
		return domain.OwnerProof{}, false, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n, err := s.qw.InsertOwnerProof(ctx, ownerProofInsert(p))
	if err != nil {
		return domain.OwnerProof{}, false, fmt.Errorf("create owner proof: %w", err)
	}
	if n > 0 {
		return p, true, nil
	}
	row, err := s.qw.GetOwnerProof(ctx, string(p.ID))
	if err != nil {
		return domain.OwnerProof{}, false, err
	}
	existing, err := ownerProofFromGen(row)
	if err != nil {
		return domain.OwnerProof{}, false, err
	}
	if !sameOwnerProof(existing, p) {
		return existing, false, domain.ErrOwnerProofConflict
	}
	return existing, false, nil
}
func (s *Store) GetOwnerProof(ctx context.Context, id domain.OwnerProofID) (domain.OwnerProof, bool, error) {
	row, err := s.qr.GetOwnerProof(ctx, string(id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.OwnerProof{}, false, nil
	}
	if err != nil {
		return domain.OwnerProof{}, false, err
	}
	p, err := ownerProofFromGen(row)
	return p, true, err
}
func (s *Store) ConsumeOwnerProof(ctx context.Context, id domain.OwnerProofID, at time.Time) (domain.OwnerProof, bool, error) {
	if at.IsZero() {
		return domain.OwnerProof{}, false, domain.ErrOwnerProofInvalid
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n, err := s.qw.ConsumeOwnerProof(ctx, gen.ConsumeOwnerProofParams{ConsumedAt: sql.NullTime{Time: at.UTC(), Valid: true}, ID: string(id)})
	if err != nil {
		return domain.OwnerProof{}, false, err
	}
	if n == 0 {
		return domain.OwnerProof{}, false, nil
	}
	row, err := s.qw.GetOwnerProof(ctx, string(id))
	if err != nil {
		return domain.OwnerProof{}, false, err
	}
	p, err := ownerProofFromGen(row)
	return p, true, err
}
func ownerProofInsert(p domain.OwnerProof) gen.InsertOwnerProofParams {
	return gen.InsertOwnerProofParams{ID: string(p.ID), Verifier: p.Verifier.String(), AppRunID: p.AppRunID, MissionID: p.MissionID, ContentDigest: p.ContentDigest.String(), TargetID: p.TargetID, TargetGeneration: p.TargetGeneration, CommandClass: string(p.Class), ConfirmationRef: p.ConfirmationRef, ExpiresAt: p.ExpiresAt.UTC(), CreatedAt: p.CreatedAt.UTC()}
}
func ownerProofFromGen(r gen.OwnerProof) (domain.OwnerProof, error) {
	var consumed *time.Time
	if r.ConsumedAt.Valid {
		x := r.ConsumedAt.Time.UTC()
		consumed = &x
	}
	p := domain.OwnerProof{ID: domain.OwnerProofID(r.ID), Verifier: domain.SHA256Digest(r.Verifier), AppRunID: r.AppRunID, MissionID: r.MissionID, ContentDigest: domain.SHA256Digest(r.ContentDigest), TargetID: r.TargetID, TargetGeneration: r.TargetGeneration, Class: domain.OwnerCommandClass(r.CommandClass), ConfirmationRef: r.ConfirmationRef, ExpiresAt: r.ExpiresAt.UTC(), CreatedAt: r.CreatedAt.UTC(), ConsumedAt: consumed}
	return p, p.Validate()
}
func sameOwnerProof(a, b domain.OwnerProof) bool {
	return a.ID == b.ID && a.AppRunID == b.AppRunID && a.MissionID == b.MissionID && a.ContentDigest == b.ContentDigest && a.TargetID == b.TargetID && a.TargetGeneration == b.TargetGeneration && a.Class == b.Class && a.ConfirmationRef == b.ConfirmationRef && a.ExpiresAt.Equal(b.ExpiresAt) && a.CreatedAt.Equal(b.CreatedAt)
}
