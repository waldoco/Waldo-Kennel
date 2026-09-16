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

var _ ports.GovernedCommandStore = (*Store)(nil)

// CreateGovernedCommandClaim records the pre-effect claim. A retry with the
// same session, idempotency key, and request fingerprint returns the original
// claim. Reusing the key for different request content is a hard conflict.
func (s *Store) CreateGovernedCommandClaim(ctx context.Context, rec domain.GovernedCommandRecord) (domain.GovernedCommandRecord, bool, error) {
	if err := validateGovernedCommandClaim(rec); err != nil {
		return domain.GovernedCommandRecord{}, false, err
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	n, err := s.qw.InsertGovernedCommandClaim(ctx, governedCommandToInsert(rec))
	if err != nil {
		return domain.GovernedCommandRecord{}, false, fmt.Errorf("create governed command %s: %w", rec.ID, err)
	}
	if n > 0 {
		return rec, true, nil
	}

	row, err := s.qw.GetGovernedCommandByIdempotencyKey(ctx, gen.GetGovernedCommandByIdempotencyKeyParams{
		SessionID: string(rec.SessionID), IdempotencyKey: rec.IdempotencyKey,
	})
	if err == nil {
		existing := governedCommandFromGen(row)
		if existing.RequestFingerprint == rec.RequestFingerprint {
			return existing, false, nil
		}
		return existing, false, fmt.Errorf("create governed command %s: %w", rec.ID, domain.ErrGovernedCommandIdempotencyConflict)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.GovernedCommandRecord{}, false, fmt.Errorf("read governed command idempotency key for %s: %w", rec.ID, err)
	}

	row, err = s.qw.GetGovernedCommand(ctx, rec.ID)
	if err == nil {
		return governedCommandFromGen(row), false, fmt.Errorf("create governed command %s: %w", rec.ID, domain.ErrGovernedCommandIdempotencyConflict)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.GovernedCommandRecord{}, false, fmt.Errorf("read conflicting governed command %s: %w", rec.ID, err)
	}
	return domain.GovernedCommandRecord{}, false, fmt.Errorf("create governed command %s: conflict row was not found: %w", rec.ID, domain.ErrGovernedCommandIdempotencyConflict)
}

// AdoptClaimedGovernedCommandGeneration transfers a pre-dispatch claim to the
// currently active controller. Once dispatching begins the command is never
// adoptable, because provider contact may already have happened.
func (s *Store) AdoptClaimedGovernedCommandGeneration(ctx context.Context, rec domain.GovernedCommandRecord, nextGeneration string, now time.Time) (bool, error) {
	if rec.State != domain.GovernedCommandClaimed || strings.TrimSpace(nextGeneration) == "" || now.IsZero() {
		return false, domain.ErrGovernedCommandInvalid
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n, err := s.qw.AdoptClaimedGovernedCommandGeneration(ctx, gen.AdoptClaimedGovernedCommandGenerationParams{
		NextControllerGeneration: nextGeneration, UpdatedAt: now, ID: rec.ID,
		ExpectedControllerGeneration: rec.ControllerGeneration, ExpectedRevision: rec.ExpectedRevision,
		ExpectedCapabilityFingerprint: rec.CapabilityFingerprint, ExpectedRequestFingerprint: rec.RequestFingerprint,
	})
	if err != nil {
		return false, fmt.Errorf("adopt governed command %s: %w", rec.ID, err)
	}
	return n > 0, nil
}

func (s *Store) GetGovernedCommand(ctx context.Context, id string) (domain.GovernedCommandRecord, bool, error) {
	row, err := s.qr.GetGovernedCommand(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.GovernedCommandRecord{}, false, nil
	}
	if err != nil {
		return domain.GovernedCommandRecord{}, false, fmt.Errorf("get governed command %s: %w", id, err)
	}
	return governedCommandFromGen(row), true, nil
}

// AdvanceGovernedCommand compare-and-sets delivery evidence under all
// ownership bindings captured by the caller. Those bindings are deliberately
// separate arguments so mutating rec cannot also move its own fence.
func (s *Store) AdvanceGovernedCommand(ctx context.Context, rec domain.GovernedCommandRecord, expectedState domain.GovernedCommandState, expectedGeneration, expectedRevision, expectedCapabilityFingerprint string) (bool, error) {
	if err := rec.GovernedCommandContract.Validate(); err != nil {
		return false, err
	}
	if !domain.CanTransitionGovernedCommand(expectedState, rec.State) {
		return false, fmt.Errorf("%w: %s -> %s", domain.ErrGovernedCommandTransition, expectedState, rec.State)
	}
	if strings.TrimSpace(expectedGeneration) == "" || strings.TrimSpace(expectedRevision) == "" || strings.TrimSpace(expectedCapabilityFingerprint) == "" {
		return false, fmt.Errorf("%w: transition fences are required", domain.ErrGovernedCommandInvalid)
	}
	if rec.ControllerGeneration != expectedGeneration || rec.ExpectedRevision != expectedRevision || rec.CapabilityFingerprint != expectedCapabilityFingerprint {
		return false, fmt.Errorf("%w: record and expected ownership bindings differ", domain.ErrGovernedCommandInvalid)
	}
	if rec.UpdatedAt.IsZero() {
		return false, fmt.Errorf("%w: transition time is required", domain.ErrGovernedCommandInvalid)
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n, err := s.qw.AdvanceGovernedCommand(ctx, gen.AdvanceGovernedCommandParams{
		NextState: string(rec.State), ProviderTurnID: rec.Correlation.ProviderTurnID,
		ProviderEventID: rec.Correlation.ProviderEventID, ProviderCursor: rec.Correlation.ProviderCursor,
		ReconciliationOutcome: string(rec.ReconciliationOutcome), Quiescence: string(rec.Quiescence),
		QuiescenceEvidenceRef: rec.QuiescenceEvidenceRef, UpdatedAt: rec.UpdatedAt, ID: rec.ID,
		ExpectedState: string(expectedState), ExpectedControllerGeneration: expectedGeneration,
		ExpectedRevision: expectedRevision, ExpectedCapabilityFingerprint: expectedCapabilityFingerprint,
	})
	if err != nil {
		return false, fmt.Errorf("advance governed command %s: %w", rec.ID, err)
	}
	return n > 0, nil
}

// ListUnsettledGovernedCommands returns claims that need dispatch or recovery.
// delivery_unknown remains included because restart never turns ambiguity into
// permission to redeliver.
func (s *Store) ListUnsettledGovernedCommands(ctx context.Context) ([]domain.GovernedCommandRecord, error) {
	rows, err := s.qr.ListUnsettledGovernedCommands(ctx)
	if err != nil {
		return nil, fmt.Errorf("list unsettled governed commands: %w", err)
	}
	out := make([]domain.GovernedCommandRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, governedCommandFromGen(row))
	}
	return out, nil
}

func validateGovernedCommandClaim(rec domain.GovernedCommandRecord) error {
	if err := rec.GovernedCommandContract.Validate(); err != nil {
		return err
	}
	if rec.State != domain.GovernedCommandClaimed {
		return fmt.Errorf("%w: a new durable claim must be in claimed state", domain.ErrGovernedCommandInvalid)
	}
	if rec.CreatedAt.IsZero() || !rec.UpdatedAt.Equal(rec.CreatedAt) {
		return fmt.Errorf("%w: claim timestamps must be nonzero and equal", domain.ErrGovernedCommandInvalid)
	}
	return nil
}

func governedCommandToInsert(rec domain.GovernedCommandRecord) gen.InsertGovernedCommandClaimParams {
	return gen.InsertGovernedCommandClaimParams{
		ID: rec.ID, SessionID: string(rec.SessionID), IdempotencyKey: rec.IdempotencyKey,
		RequestFingerprint: rec.RequestFingerprint, CommandClass: string(rec.Class), State: string(rec.State),
		ControllerGeneration: rec.ControllerGeneration, ExpectedRevision: rec.ExpectedRevision,
		CapabilityFingerprint:  rec.CapabilityFingerprint,
		ProviderConversationID: rec.Correlation.ProviderConversationID, ClientMessageID: rec.Correlation.ClientMessageID,
		ProviderTurnID: rec.Correlation.ProviderTurnID, ProviderEventID: rec.Correlation.ProviderEventID,
		ProviderCursor: rec.Correlation.ProviderCursor, ReplayStrategy: string(rec.ReplayStrategy),
		ReconciliationOutcome: string(rec.ReconciliationOutcome), Quiescence: string(rec.Quiescence),
		QuiescenceEvidenceRef: rec.QuiescenceEvidenceRef, CreatedAt: rec.CreatedAt, UpdatedAt: rec.UpdatedAt,
	}
}

func governedCommandFromGen(row gen.GovernedCommand) domain.GovernedCommandRecord {
	return domain.GovernedCommandRecord{
		GovernedCommandContract: domain.GovernedCommandContract{
			ID: row.ID, SessionID: domain.SessionID(row.SessionID), IdempotencyKey: row.IdempotencyKey,
			RequestFingerprint: row.RequestFingerprint, Class: domain.GovernedCommandClass(row.CommandClass),
			State: domain.GovernedCommandState(row.State), ControllerGeneration: row.ControllerGeneration,
			ExpectedRevision: row.ExpectedRevision, CapabilityFingerprint: row.CapabilityFingerprint,
			Correlation: domain.GovernedCommandCorrelation{
				ProviderConversationID: row.ProviderConversationID, ClientMessageID: row.ClientMessageID,
				ProviderTurnID: row.ProviderTurnID, ProviderEventID: row.ProviderEventID, ProviderCursor: row.ProviderCursor,
			},
			ReplayStrategy:        domain.GovernedCommandReplayStrategy(row.ReplayStrategy),
			ReconciliationOutcome: domain.GovernedCommandReconciliationOutcome(row.ReconciliationOutcome),
			Quiescence:            domain.GovernedCommandQuiescence(row.Quiescence), QuiescenceEvidenceRef: row.QuiescenceEvidenceRef,
		},
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}
