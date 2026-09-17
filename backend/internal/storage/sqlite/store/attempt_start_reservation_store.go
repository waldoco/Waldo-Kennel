package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

type attemptStartTx struct{ tx *sql.Tx }

func (t attemptStartTx) SQLTx() *sql.Tx { return t.tx }

func (s *Store) FindAttemptStartReservation(ctx context.Context, requestKey string) (domain.AttemptStartReservation, bool, error) {
	row := s.readDB.QueryRowContext(ctx, `SELECT id,attempt_id,outcome_id,plan_revision_id,work_unit_id,contract_revision_number,run_intent_generation,request_key,request_fingerprint,routing_snapshot_id,routing_generation_id,admission_evaluation_id,refusal_status,denial_detail,created_at FROM attempt_start_reservations WHERE request_key=?`, strings.TrimSpace(requestKey))
	r, err := scanAttemptStartReservation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AttemptStartReservation{}, false, nil
	}
	if err != nil {
		return domain.AttemptStartReservation{}, false, fmt.Errorf("find attempt start reservation: %w", err)
	}
	return r, true, nil
}

type rowScanner interface{ Scan(...any) error }

func scanAttemptStartReservation(row rowScanner) (domain.AttemptStartReservation, error) {
	var r domain.AttemptStartReservation
	var detail string
	err := row.Scan(&r.ID, &r.AttemptID, &r.OutcomeID, &r.PlanRevisionID, &r.WorkUnitID, &r.ContractRevisionNumber, &r.RunIntentGeneration, &r.RequestKey, &r.RequestFingerprint, &r.RoutingSnapshotID, &r.RoutingGenerationID, &r.AdmissionEvaluationID, &r.RefusalStatus, &detail, &r.CreatedAt)
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal([]byte(detail), &r.Denial); err != nil {
		return r, fmt.Errorf("decode denial detail: %w", err)
	}
	return r, r.Validate()
}

func (s *Store) ReserveAttemptStart(ctx context.Context, in ports.AttemptStartReservationRequest, attach ports.AttemptStartEscalationCreator) (domain.AttemptStartReservation, bool, error) {
	r := in.Reservation
	if err := r.Validate(); err != nil {
		return domain.AttemptStartReservation{}, false, err
	}
	if r.RefusalStatus != domain.AttemptStartRefusalOpen {
		return domain.AttemptStartReservation{}, false, fmt.Errorf("new reservation must carry open refusal")
	}
	if attach == nil {
		return domain.AttemptStartReservation{}, false, fmt.Errorf("attempt start refusal requires transaction-safe escalation attachment")
	}
	detail, err := json.Marshal(r.Denial)
	if err != nil {
		return domain.AttemptStartReservation{}, false, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.writeDB.BeginTx(ctx, nil)
	if err != nil {
		return domain.AttemptStartReservation{}, false, err
	}
	defer tx.Rollback()
	existing, err := scanAttemptStartReservation(tx.QueryRowContext(ctx, `SELECT id,attempt_id,outcome_id,plan_revision_id,work_unit_id,contract_revision_number,run_intent_generation,request_key,request_fingerprint,routing_snapshot_id,routing_generation_id,admission_evaluation_id,refusal_status,denial_detail,created_at FROM attempt_start_reservations WHERE request_key=?`, r.RequestKey))
	if err == nil {
		if existing.RequestFingerprint != r.RequestFingerprint || existing.OutcomeID != r.OutcomeID || existing.PlanRevisionID != r.PlanRevisionID || existing.WorkUnitID != r.WorkUnitID || existing.ContractRevisionNumber != r.ContractRevisionNumber || existing.RunIntentGeneration != r.RunIntentGeneration || existing.RoutingSnapshotID != r.RoutingSnapshotID || existing.RoutingGenerationID != r.RoutingGenerationID || existing.AdmissionEvaluationID != r.AdmissionEvaluationID {
			return existing, false, &ports.AttemptStartReplayConflictError{Existing: existing}
		}
		return existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.AttemptStartReservation{}, false, err
	}
	var number int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(number),0)+1 FROM attempts WHERE outcome_id=?`, r.OutcomeID).Scan(&number); err != nil {
		return domain.AttemptStartReservation{}, false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO attempts(id,outcome_id,plan_revision_id,work_unit_id,number,status,contract_revision_number,run_intent_generation,request_key,created_at,updated_at) VALUES(?,?,?,?,?,'awaiting_authority',?,?,?,?,?)`, r.AttemptID, r.OutcomeID, r.PlanRevisionID, r.WorkUnitID, number, r.ContractRevisionNumber, r.RunIntentGeneration, r.RequestKey, r.CreatedAt, r.CreatedAt)
	if err != nil {
		return domain.AttemptStartReservation{}, false, fmt.Errorf("create prelaunch attempt: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO attempt_start_reservations(id,attempt_id,outcome_id,plan_revision_id,work_unit_id,contract_revision_number,run_intent_generation,request_key,request_fingerprint,routing_snapshot_id,routing_generation_id,admission_evaluation_id,refusal_status,denial_detail,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, r.ID, r.AttemptID, r.OutcomeID, r.PlanRevisionID, r.WorkUnitID, r.ContractRevisionNumber, r.RunIntentGeneration, r.RequestKey, r.RequestFingerprint, r.RoutingSnapshotID, r.RoutingGenerationID, r.AdmissionEvaluationID, r.RefusalStatus, string(detail), r.CreatedAt)
	if err != nil {
		return domain.AttemptStartReservation{}, false, fmt.Errorf("persist prelaunch refusal: %w", err)
	}
	if err = attach.CreateInAttemptStartTransaction(ctx, attemptStartTx{tx}, ports.AttemptStartEscalationAttachment{ReservationID: r.ID, AttemptID: r.AttemptID, AdmissionEvaluationID: r.AdmissionEvaluationID, Denial: r.Denial}); err != nil {
		return domain.AttemptStartReservation{}, false, fmt.Errorf("attach capability escalation: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return domain.AttemptStartReservation{}, false, err
	}
	return r, true, nil
}
