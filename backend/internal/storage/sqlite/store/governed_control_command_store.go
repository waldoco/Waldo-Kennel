package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/gen"
)

var _ ports.GovernedControlCommandStore = (*Store)(nil)

func (s *Store) CreateGovernedControlCommandClaim(ctx context.Context, rec domain.GovernedControlCommand) (domain.GovernedControlCommand, bool, error) {
	if err := rec.Validate(); err != nil {
		return domain.GovernedControlCommand{}, false, err
	}
	if rec.State != domain.GovernedCommandClaimed || rec.CreatedAt.IsZero() || !rec.CreatedAt.Equal(rec.UpdatedAt) {
		return domain.GovernedControlCommand{}, false, fmt.Errorf("%w: new control claim must be claimed with equal timestamps", domain.ErrGovernedCommandInvalid)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n, err := s.qw.InsertGovernedControlCommandClaim(ctx, controlInsert(rec))
	if err != nil {
		return domain.GovernedControlCommand{}, false, err
	}
	if n > 0 {
		return rec, true, nil
	}
	row, err := s.qw.GetGovernedControlCommandByKey(ctx, gen.GetGovernedControlCommandByKeyParams{SessionID: string(rec.SessionID), IdempotencyKey: rec.IdempotencyKey})
	if err == nil {
		existing := controlFromGen(row)
		if existing.RequestFingerprint == rec.RequestFingerprint {
			return existing, false, nil
		}
		return existing, false, domain.ErrGovernedCommandIdempotencyConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.GovernedControlCommand{}, false, err
	}
	return domain.GovernedControlCommand{}, false, domain.ErrGovernedCommandIdempotencyConflict
}

// AdoptClaimedGovernedControlCommandGeneration transfers a pre-dispatch
// control claim to the active controller. Dispatching and later states are
// deliberately immovable because provider contact may already have happened.
func (s *Store) AdoptClaimedGovernedControlCommandGeneration(ctx context.Context, rec domain.GovernedControlCommand, nextGeneration string, now time.Time) (bool, error) {
	if rec.State != domain.GovernedCommandClaimed || strings.TrimSpace(nextGeneration) == "" || now.IsZero() {
		return false, domain.ErrGovernedCommandInvalid
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n, err := s.qw.AdoptClaimedGovernedControlCommandGeneration(ctx, gen.AdoptClaimedGovernedControlCommandGenerationParams{
		NextControllerGeneration: nextGeneration, UpdatedAt: now, ID: rec.ID,
		ExpectedControllerGeneration: rec.ControllerGeneration, ExpectedRevision: rec.ExpectedRevision,
		ExpectedCapabilityFingerprint: rec.CapabilityFingerprint, ExpectedRequestFingerprint: rec.RequestFingerprint,
	})
	if err != nil {
		return false, fmt.Errorf("adopt governed control command %s: %w", rec.ID, err)
	}
	return n > 0, nil
}

func (s *Store) GetGovernedControlCommand(ctx context.Context, id string) (domain.GovernedControlCommand, bool, error) {
	row, err := s.qr.GetGovernedControlCommand(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.GovernedControlCommand{}, false, nil
	}
	if err != nil {
		return domain.GovernedControlCommand{}, false, err
	}
	return controlFromGen(row), true, nil
}
func (s *Store) ListUnsettledGovernedControlCommands(ctx context.Context) ([]domain.GovernedControlCommand, error) {
	rows, err := s.qr.ListUnsettledGovernedControlCommands(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.GovernedControlCommand, 0, len(rows))
	for _, r := range rows {
		out = append(out, controlFromGen(r))
	}
	return out, nil
}
func (s *Store) AdvanceGovernedControlCommand(ctx context.Context, rec domain.GovernedControlCommand, expected domain.GovernedCommandState, generation, revision, capability string) (bool, error) {
	if err := rec.Validate(); err != nil {
		return false, err
	}
	if !domain.CanTransitionGovernedCommand(expected, rec.State) {
		return false, domain.ErrGovernedCommandTransition
	}
	if rec.ControllerGeneration != generation || rec.ExpectedRevision != revision || rec.CapabilityFingerprint != capability || rec.UpdatedAt.IsZero() {
		return false, domain.ErrGovernedCommandInvalid
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n, err := s.qw.AdvanceGovernedControlCommand(ctx, gen.AdvanceGovernedControlCommandParams{NextState: string(rec.State), Quiescence: string(rec.Quiescence), QuiescenceEvidenceRef: rec.QuiescenceEvidenceRef, UpdatedAt: rec.UpdatedAt, ID: rec.ID, ExpectedState: string(expected), ExpectedControllerGeneration: generation, ExpectedRevision: revision, ExpectedCapabilityFingerprint: capability})
	return n > 0, err
}

func controlInsert(r domain.GovernedControlCommand) gen.InsertGovernedControlCommandClaimParams {
	return gen.InsertGovernedControlCommandClaimParams{ID: r.ID, SessionID: string(r.SessionID), IdempotencyKey: r.IdempotencyKey, RequestFingerprint: r.RequestFingerprint, CommandClass: string(r.Class), State: string(r.State), ControllerGeneration: r.ControllerGeneration, ExpectedRevision: r.ExpectedRevision, CapabilityFingerprint: r.CapabilityFingerprint, ProviderConversationID: r.ProviderConversationID, ClientMessageID: r.ClientMessageID, ProviderTurnID: r.ProviderTurnID, RequestInstanceID: r.RequestInstanceID, Quiescence: string(r.Quiescence), QuiescenceEvidenceRef: r.QuiescenceEvidenceRef, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
func controlFromGen(r gen.GovernedControlCommand) domain.GovernedControlCommand {
	return domain.GovernedControlCommand{ID: r.ID, SessionID: domain.SessionID(r.SessionID), IdempotencyKey: r.IdempotencyKey, RequestFingerprint: r.RequestFingerprint, Class: domain.GovernedControlClass(r.CommandClass), State: domain.GovernedCommandState(r.State), ControllerGeneration: r.ControllerGeneration, ExpectedRevision: r.ExpectedRevision, CapabilityFingerprint: r.CapabilityFingerprint, ProviderConversationID: r.ProviderConversationID, ClientMessageID: r.ClientMessageID, ProviderTurnID: r.ProviderTurnID, RequestInstanceID: r.RequestInstanceID, Quiescence: domain.GovernedCommandQuiescence(r.Quiescence), QuiescenceEvidenceRef: r.QuiescenceEvidenceRef, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
