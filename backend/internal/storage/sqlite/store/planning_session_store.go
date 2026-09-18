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

// CreatePlanningSession creates or idempotently replays one planning session.
func (s *Store) CreatePlanningSession(ctx context.Context, session domain.PlanningSession) (domain.PlanningSession, bool, error) {
	if err := session.Validate(); err != nil {
		return domain.PlanningSession{}, false, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	var result domain.PlanningSession
	var replay bool
	err := s.inTx(ctx, "create planning session", func(q *gen.Queries) error {
		existing, err := q.GetPlanningSessionByRequestKey(ctx, session.RequestKey)
		if err == nil {
			mapped, mapErr := planningSessionFromRow(existing)
			if mapErr != nil {
				return mapErr
			}
			if mapped.RequestFingerprint != session.RequestFingerprint {
				return &ports.PlanningRequestConflictError{RequestKey: session.RequestKey}
			}
			result, replay = mapped, true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		current, err := q.GetCurrentPlanningSession(ctx, string(session.OutcomeID))
		if err == nil && current.Status == string(domain.PlanningSessionActive) {
			return fmt.Errorf("outcome %s already has active planning session %s", session.OutcomeID, current.ID)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := q.CreatePlanningSession(ctx, createPlanningSessionParams(session)); err != nil {
			return err
		}
		result = session
		return nil
	})
	if err != nil {
		return domain.PlanningSession{}, false, fmt.Errorf("create planning session %s: %w", session.ID, err)
	}
	return result, replay, nil
}

// GetPlanningSessionByRequestKey resolves a start replay before mutable
// candidate readiness or repository state is consulted.
func (s *Store) GetPlanningSessionByRequestKey(ctx context.Context, requestKey string) (domain.PlanningSession, bool, error) {
	row, err := s.qr.GetPlanningSessionByRequestKey(ctx, requestKey)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PlanningSession{}, false, nil
	}
	if err != nil {
		return domain.PlanningSession{}, false, fmt.Errorf("get planning session by request key %q: %w", requestKey, err)
	}
	mapped, err := planningSessionFromRow(row)
	return mapped, true, err
}

// GetPlanningSession reads one session scoped to its Outcome.
func (s *Store) GetPlanningSession(ctx context.Context, outcomeID domain.OutcomeID, sessionID domain.PlanningSessionID) (domain.PlanningSession, bool, error) {
	row, err := s.qr.GetPlanningSession(ctx, gen.GetPlanningSessionParams{ID: sessionID.String(), OutcomeID: string(outcomeID)})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PlanningSession{}, false, nil
	}
	if err != nil {
		return domain.PlanningSession{}, false, fmt.Errorf("get planning session %s: %w", sessionID, err)
	}
	mapped, err := planningSessionFromRow(row)
	return mapped, true, err
}

// GetCurrentPlanningSession reads the newest session for an Outcome.
func (s *Store) GetCurrentPlanningSession(ctx context.Context, outcomeID domain.OutcomeID) (domain.PlanningSession, bool, error) {
	row, err := s.qr.GetCurrentPlanningSession(ctx, string(outcomeID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PlanningSession{}, false, nil
	}
	if err != nil {
		return domain.PlanningSession{}, false, fmt.Errorf("get current planning session for %s: %w", outcomeID, err)
	}
	mapped, err := planningSessionFromRow(row)
	return mapped, true, err
}

// ListPlanningTurns reads normalized turns in canonical sequence order.
func (s *Store) ListPlanningTurns(ctx context.Context, sessionID domain.PlanningSessionID) ([]domain.PlanningTurn, error) {
	rows, err := s.qr.ListPlanningTurns(ctx, sessionID.String())
	if err != nil {
		return nil, fmt.Errorf("list planning turns for %s: %w", sessionID, err)
	}
	turns := make([]domain.PlanningTurn, 0, len(rows))
	for _, row := range rows {
		turn, err := planningTurnFromRow(row)
		if err != nil {
			return nil, err
		}
		turns = append(turns, turn)
	}
	return turns, nil
}

// AppendPlanningOwnerTurn writes or replays an idempotent owner turn.
func (s *Store) AppendPlanningOwnerTurn(ctx context.Context, sessionID domain.PlanningSessionID, expectedRevision int64, turn domain.PlanningTurn) (domain.PlanningSession, domain.PlanningTurn, bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	var result domain.PlanningSession
	var stored domain.PlanningTurn
	var replay bool
	err := s.inTx(ctx, "append planning owner turn", func(q *gen.Queries) error {
		row, err := q.GetPlanningSessionByID(ctx, sessionID.String())
		if err != nil {
			return err
		}
		current, err := planningSessionFromRow(row)
		if err != nil {
			return err
		}
		existing, err := q.GetPlanningTurnByRequestKey(ctx, gen.GetPlanningTurnByRequestKeyParams{
			PlanningSessionID: sessionID.String(), RequestKey: nullableString(turn.RequestKey),
		})
		if err == nil {
			mapped, mapErr := planningTurnFromRow(existing)
			if mapErr != nil {
				return mapErr
			}
			if mapped.RequestFingerprint != turn.RequestFingerprint {
				return &ports.PlanningRequestConflictError{RequestKey: turn.RequestKey}
			}
			result, stored, replay = current, mapped, true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if current.Revision != expectedRevision || current.Status != domain.PlanningSessionActive || current.WaitingOn != domain.PlanningWaitingOwner {
			return planningRevisionConflict(sessionID, expectedRevision, current.Revision)
		}
		turn.PlanningSessionID = sessionID
		turn.Sequence = current.LatestTurnSequence + 1
		if err := turn.Validate(); err != nil {
			return err
		}
		if err := q.CreatePlanningTurn(ctx, createPlanningTurnParams(turn)); err != nil {
			return err
		}
		changed, err := q.AdvancePlanningSessionForOwnerTurn(ctx, gen.AdvancePlanningSessionForOwnerTurnParams{
			LatestTurnSequence: turn.Sequence, UpdatedAt: turn.CreatedAt.UTC(), ID: sessionID.String(), Revision: expectedRevision,
		})
		if err != nil {
			return err
		}
		if changed != 1 {
			return planningRevisionConflict(sessionID, expectedRevision, current.Revision)
		}
		current.Revision++
		current.LatestTurnSequence = turn.Sequence
		current.WaitingOn = domain.PlanningWaitingProvider
		current.LastFailureCode, current.LastFailureDetail = "", ""
		current.UpdatedAt = turn.CreatedAt.UTC()
		result, stored = current, turn
		return nil
	})
	if err != nil {
		return domain.PlanningSession{}, domain.PlanningTurn{}, false, fmt.Errorf("append planning owner turn to %s: %w", sessionID, err)
	}
	return result, stored, replay, nil
}

// AppendPlanningProviderTurn atomically records one reply and its provenance.
func (s *Store) AppendPlanningProviderTurn(ctx context.Context, sessionID domain.PlanningSessionID, expectedRevision int64, turn domain.PlanningTurn, effectiveProvider domain.IntelligenceProviderID, effectiveModel, nativeRef string) (domain.PlanningSession, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	var result domain.PlanningSession
	err := s.inTx(ctx, "append planning provider turn", func(q *gen.Queries) error {
		row, err := q.GetPlanningSessionByID(ctx, sessionID.String())
		if err != nil {
			return err
		}
		current, err := planningSessionFromRow(row)
		if err != nil {
			return err
		}
		if current.Revision != expectedRevision || current.Status != domain.PlanningSessionActive || current.WaitingOn != domain.PlanningWaitingProvider {
			return planningRevisionConflict(sessionID, expectedRevision, current.Revision)
		}
		if !current.EffectiveProvider.IsZero() && current.EffectiveProvider != effectiveProvider {
			return fmt.Errorf("planning session %s effective provider is already %q", sessionID, current.EffectiveProvider)
		}
		persistedModel := current.EffectiveModel
		if persistedModel == "" {
			persistedModel = effectiveModel
		}
		persistedNativeRef := current.NativeConversationRef
		if current.Binding.Mode == domain.PlanningModeNativeHarness {
			if strings.TrimSpace(nativeRef) == "" {
				return fmt.Errorf("planning session %s native reply requires per-run conversation provenance", sessionID)
			}
			if persistedNativeRef != "" && persistedNativeRef != nativeRef {
				return fmt.Errorf("planning session %s native conversation reference is already %q", sessionID, persistedNativeRef)
			}
			// Native packet planning deliberately reconstructs each one-shot provider
			// turn from Kennel's normalized history. Its provider thread belongs to
			// the IntelligenceRun, not to the durable PlanningSession; never overwrite
			// an older stable-session reference. The mismatch fence above preserves
			// truthful semantics for historical sessions that already own one.
		}
		turn.PlanningSessionID = sessionID
		turn.Sequence = current.LatestTurnSequence + 1
		if err := turn.Validate(); err != nil {
			return err
		}
		if err := q.CreatePlanningTurn(ctx, createPlanningTurnParams(turn)); err != nil {
			return err
		}
		changed, err := q.AdvancePlanningSessionForProviderTurn(ctx, gen.AdvancePlanningSessionForProviderTurnParams{
			LatestTurnSequence: turn.Sequence, EffectiveProvider: string(effectiveProvider), EffectiveModel: persistedModel,
			NativeConversationRef: persistedNativeRef, UpdatedAt: turn.CreatedAt.UTC(), ID: sessionID.String(), Revision: expectedRevision,
		})
		if err != nil {
			return err
		}
		if changed != 1 {
			return planningRevisionConflict(sessionID, expectedRevision, current.Revision)
		}
		current.Revision++
		current.LatestTurnSequence = turn.Sequence
		current.WaitingOn = domain.PlanningWaitingOwner
		current.EffectiveProvider = effectiveProvider
		current.EffectiveModel = persistedModel
		current.NativeConversationRef = persistedNativeRef
		current.LastFailureCode, current.LastFailureDetail = "", ""
		current.UpdatedAt = turn.CreatedAt.UTC()
		result = current
		return nil
	})
	if err != nil {
		return domain.PlanningSession{}, fmt.Errorf("append planning provider turn to %s: %w", sessionID, err)
	}
	return result, nil
}

// SetPlanningSessionFailure returns a failed provider turn to owner control.
func (s *Store) SetPlanningSessionFailure(ctx context.Context, sessionID domain.PlanningSessionID, expectedRevision int64, code, detail string) (domain.PlanningSession, error) {
	return s.updatePlanningSession(ctx, sessionID, expectedRevision, func(q *gen.Queries, now time.Time) (int64, error) {
		return q.SetPlanningSessionFailure(ctx, gen.SetPlanningSessionFailureParams{LastFailureCode: code, LastFailureDetail: detail, UpdatedAt: now, ID: sessionID.String(), Revision: expectedRevision})
	})
}

// SetPlanningSessionWaitingSystem marks an active session blocked on setup,
// harness, or Contract action routed from a readiness packet - never provider
// thinking and never an ordinary owner answer.
func (s *Store) SetPlanningSessionWaitingSystem(ctx context.Context, sessionID domain.PlanningSessionID, expectedRevision int64) (domain.PlanningSession, error) {
	return s.updatePlanningSession(ctx, sessionID, expectedRevision, func(q *gen.Queries, now time.Time) (int64, error) {
		return q.SetPlanningSessionWaitingSystem(ctx, gen.SetPlanningSessionWaitingSystemParams{UpdatedAt: now, ID: sessionID.String(), Revision: expectedRevision})
	})
}

// ClosePlanningSession records cancellation or Contract supersession.
func (s *Store) ClosePlanningSession(ctx context.Context, sessionID domain.PlanningSessionID, expectedRevision int64, status domain.PlanningSessionStatus) (domain.PlanningSession, error) {
	if status != domain.PlanningSessionCancelled && status != domain.PlanningSessionSuperseded {
		return domain.PlanningSession{}, fmt.Errorf("planning session close status %q is invalid", status)
	}
	return s.updatePlanningSession(ctx, sessionID, expectedRevision, func(q *gen.Queries, now time.Time) (int64, error) {
		return q.ClosePlanningSession(ctx, gen.ClosePlanningSessionParams{Status: string(status), UpdatedAt: now, ClosedAt: sql.NullTime{Time: now, Valid: true}, ID: sessionID.String(), Revision: expectedRevision})
	})
}

// LinkPlanningSessionPlan closes the conversation on its validated proposal.
func (s *Store) LinkPlanningSessionPlan(ctx context.Context, sessionID domain.PlanningSessionID, expectedRevision int64, planID domain.PlanRevisionID, runID domain.IntelligenceRunID) (domain.PlanningSession, error) {
	return s.updatePlanningSession(ctx, sessionID, expectedRevision, func(q *gen.Queries, now time.Time) (int64, error) {
		return q.LinkPlanningSessionPlan(ctx, gen.LinkPlanningSessionPlanParams{
			ProposedPlanRevisionID: nullableString(string(planID)), UpdatedAt: now, ClosedAt: sql.NullTime{Time: now, Valid: true},
			ID: sessionID.String(), Revision: expectedRevision, ID_2: planID, SourceIntelligenceRunID: nullableString(string(runID)),
		})
	})
}

// GetPlanRevisionByPlanningSession reads the unique proposal from a session.
func (s *Store) GetPlanRevisionByPlanningSession(ctx context.Context, outcomeID domain.OutcomeID, sessionID domain.PlanningSessionID) (domain.PlanRevision, bool, error) {
	row, err := s.qr.GetPlanRevisionByPlanningSession(ctx, gen.GetPlanRevisionByPlanningSessionParams{OutcomeID: outcomeID, PlanningSessionID: nullableString(sessionID.String())})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PlanRevision{}, false, nil
	}
	if err != nil {
		return domain.PlanRevision{}, false, fmt.Errorf("get Plan for planning session %s: %w", sessionID, err)
	}
	return s.planFromRow(ctx, gen.PlanRevision{
		ID: row.ID, OutcomeID: row.OutcomeID, Number: row.Number, ContractRevisionNumber: row.ContractRevisionNumber,
		Status: row.Status, Summary: row.Summary, AssumptionsJson: row.AssumptionsJson, BlockersJson: row.BlockersJson,
		RunBriefCoreDigest: row.RunBriefCoreDigest, RunBriefCompiledDigest: row.RunBriefCompiledDigest,
		CreatedAt: row.CreatedAt, PlanningSessionID: row.PlanningSessionID,
		SourceIntelligenceRunID: row.SourceIntelligenceRunID, RoutingDecisionsJson: row.RoutingDecisionsJson,
	})
}

// RecoverInterruptedPlanningSessions returns crash-interrupted provider waits
// to explicit owner control without replaying a possibly billed model call.
func (s *Store) RecoverInterruptedPlanningSessions(ctx context.Context, at time.Time) (int64, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	count, err := s.qw.RecoverInterruptedPlanningSessions(ctx, at.UTC())
	if err != nil {
		return 0, fmt.Errorf("recover interrupted planning sessions: %w", err)
	}
	return count, nil
}

func (s *Store) updatePlanningSession(ctx context.Context, sessionID domain.PlanningSessionID, expectedRevision int64, update func(*gen.Queries, time.Time) (int64, error)) (domain.PlanningSession, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	var result domain.PlanningSession
	err := s.inTx(ctx, "update planning session", func(q *gen.Queries) error {
		row, err := q.GetPlanningSessionByID(ctx, sessionID.String())
		if err != nil {
			return err
		}
		current, err := planningSessionFromRow(row)
		if err != nil {
			return err
		}
		if current.Revision != expectedRevision {
			return planningRevisionConflict(sessionID, expectedRevision, current.Revision)
		}
		now := time.Now().UTC()
		changed, err := update(q, now)
		if err != nil {
			return err
		}
		if changed != 1 {
			return planningRevisionConflict(sessionID, expectedRevision, current.Revision)
		}
		row, err = q.GetPlanningSessionByID(ctx, sessionID.String())
		if err != nil {
			return err
		}
		result, err = planningSessionFromRow(row)
		return err
	})
	if err != nil {
		return domain.PlanningSession{}, fmt.Errorf("update planning session %s: %w", sessionID, err)
	}
	return result, nil
}

func createPlanningSessionParams(s domain.PlanningSession) gen.CreatePlanningSessionParams {
	return gen.CreatePlanningSessionParams{
		ID: s.ID.String(), OutcomeID: string(s.OutcomeID), ProjectID: string(s.ProjectID), ContractRevisionID: s.ContractRevisionID.String(),
		ContractRevisionNumber: s.ContractRevisionNumber, Revision: s.Revision, LatestTurnSequence: s.LatestTurnSequence,
		Status: string(s.Status), WaitingOn: string(s.WaitingOn), Mode: string(s.Binding.Mode), RequestedProvider: string(s.Binding.Provider),
		ModelSelection: string(s.Binding.ModelSelection), RequestedModel: s.Binding.Model, RequestedEffort: s.Binding.Effort,
		ContextMode: string(s.ContextMode), PlanningGrantDigest: string(s.PlanningGrantDigest), ContextDigest: string(s.ContextDigest), ContextSnapshotJson: string(s.ContextSnapshotJSON),
		EffectiveProvider: string(s.EffectiveProvider), EffectiveModel: s.EffectiveModel, NativeConversationRef: s.NativeConversationRef,
		ProposedPlanRevisionID: nullableString(string(s.ProposedPlanRevisionID)), LastFailureCode: s.LastFailureCode, LastFailureDetail: s.LastFailureDetail,
		RequestKey: s.RequestKey, RequestFingerprint: string(s.RequestFingerprint), CreatedAt: s.CreatedAt.UTC(), UpdatedAt: s.UpdatedAt.UTC(), ClosedAt: nullableTime(s.ClosedAt),
	}
}

func createPlanningTurnParams(t domain.PlanningTurn) gen.CreatePlanningTurnParams {
	return gen.CreatePlanningTurnParams{
		ID: string(t.ID), PlanningSessionID: t.PlanningSessionID.String(), Sequence: t.Sequence,
		ReplyToTurnID: nullableString(string(t.ReplyToTurnID)), Role: string(t.Role), Kind: string(t.Kind), Text: t.Text,
		StructuredPayloadJson: nullableBytes(t.StructuredPayload), IntelligenceRunID: nullableString(string(t.IntelligenceRunID)),
		RequestKey: nullableString(t.RequestKey), RequestFingerprint: nullableString(string(t.RequestFingerprint)), CreatedAt: t.CreatedAt.UTC(),
	}
}

func planningSessionFromRow(row gen.PlanningSession) (domain.PlanningSession, error) {
	session := domain.PlanningSession{
		ID: domain.PlanningSessionID(row.ID), OutcomeID: domain.OutcomeID(row.OutcomeID), ProjectID: domain.ProjectID(row.ProjectID),
		ContractRevisionID: domain.ContractRevisionID(row.ContractRevisionID), ContractRevisionNumber: row.ContractRevisionNumber,
		Revision: row.Revision, LatestTurnSequence: row.LatestTurnSequence, Status: domain.PlanningSessionStatus(row.Status), WaitingOn: domain.PlanningWaitingOn(row.WaitingOn),
		Binding:     domain.PlanningBinding{Mode: domain.PlanningMode(row.Mode), Provider: domain.IntelligenceProviderID(row.RequestedProvider), ModelSelection: domain.PlanningModelSelection(row.ModelSelection), Model: row.RequestedModel, Effort: row.RequestedEffort},
		ContextMode: domain.PlanningContextMode(row.ContextMode), PlanningGrantDigest: domain.SHA256Digest(row.PlanningGrantDigest), ContextDigest: domain.SHA256Digest(row.ContextDigest), ContextSnapshotJSON: []byte(row.ContextSnapshotJson),
		EffectiveProvider: domain.IntelligenceProviderID(row.EffectiveProvider), EffectiveModel: row.EffectiveModel, NativeConversationRef: row.NativeConversationRef,
		ProposedPlanRevisionID: domain.PlanRevisionID(row.ProposedPlanRevisionID.String), LastFailureCode: row.LastFailureCode, LastFailureDetail: row.LastFailureDetail,
		RequestKey: row.RequestKey, RequestFingerprint: domain.SHA256Digest(row.RequestFingerprint), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if row.ClosedAt.Valid {
		closed := row.ClosedAt.Time
		session.ClosedAt = &closed
	}
	if err := session.Validate(); err != nil {
		return domain.PlanningSession{}, fmt.Errorf("planning session %s failed readback validation: %w", row.ID, err)
	}
	return session, nil
}

func planningTurnFromRow(row gen.PlanningTurn) (domain.PlanningTurn, error) {
	turn := domain.PlanningTurn{
		ID: domain.PlanningTurnID(row.ID), PlanningSessionID: domain.PlanningSessionID(row.PlanningSessionID), Sequence: row.Sequence,
		ReplyToTurnID: domain.PlanningTurnID(row.ReplyToTurnID.String), Role: domain.PlanningTurnRole(row.Role), Kind: domain.PlanningTurnKind(row.Kind), Text: row.Text,
		StructuredPayload: []byte(row.StructuredPayloadJson.String), IntelligenceRunID: domain.IntelligenceRunID(row.IntelligenceRunID.String),
		RequestKey: row.RequestKey.String, RequestFingerprint: domain.SHA256Digest(row.RequestFingerprint.String), CreatedAt: row.CreatedAt,
	}
	if err := turn.Validate(); err != nil {
		return domain.PlanningTurn{}, fmt.Errorf("planning turn %s failed readback validation: %w", row.ID, err)
	}
	return turn, nil
}

func planningRevisionConflict(id domain.PlanningSessionID, expected, current int64) error {
	return &ports.PlanningSessionRevisionConflictError{SessionID: id, Expected: expected, Current: current}
}

func nullableBytes(value []byte) sql.NullString {
	return nullableString(string(value))
}
