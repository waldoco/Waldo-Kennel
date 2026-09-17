package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

func (s *Store) CreateAttemptReplacementDecision(ctx context.Context, d domain.AttemptReplacementDecision) (domain.AttemptReplacementDecision, bool, error) {
	if err := d.Validate(); err != nil {
		return d, false, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	const attempts = 8
	for attempt := 0; attempt < attempts; attempt++ {
		stored, created, err := s.createAttemptReplacementDecisionOnce(ctx, d)
		if err == nil {
			return stored, created, nil
		}
		if !isSQLiteBusy(err) && !isSQLiteUnique(err) {
			return stored, created, err
		}
		// The competing transaction may have committed this request key. Read
		// outside the failed snapshot and converge without asking the caller to retry.
		if existing, ok, readErr := replacementDecisionByKey(ctx, s.readDB, d.RequestKey); readErr == nil && ok {
			if replacementDecisionSameSemantics(existing, d) {
				return existing, false, nil
			}
			return existing, false, &ports.AttemptReplacementDecisionConflictError{Existing: existing}
		} else if readErr != nil && !isSQLiteBusy(readErr) {
			return d, false, readErr
		}
		if err := ctx.Err(); err != nil {
			return d, false, err
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	if existing, ok, err := replacementDecisionByKey(ctx, s.readDB, d.RequestKey); err == nil && ok {
		if replacementDecisionSameSemantics(existing, d) {
			return existing, false, nil
		}
		return existing, false, &ports.AttemptReplacementDecisionConflictError{Existing: existing}
	}
	return d, false, errors.New("replacement decision storage remained busy")
}

func (s *Store) createAttemptReplacementDecisionOnce(ctx context.Context, d domain.AttemptReplacementDecision) (domain.AttemptReplacementDecision, bool, error) {
	tx, err := s.writeDB.BeginTx(ctx, nil)
	if err != nil {
		return d, false, err
	}
	defer tx.Rollback()
	if x, ok, e := replacementDecisionByKey(ctx, tx, d.RequestKey); e != nil {
		return d, false, e
	} else if ok {
		if replacementDecisionSameSemantics(x, d) {
			return x, false, nil
		}
		return x, false, &ports.AttemptReplacementDecisionConflictError{Existing: x}
	}
	var outcome domain.OutcomeID
	var status domain.AttemptStatus
	var plan domain.PlanRevisionID
	var unit domain.WorkUnitID
	var generation, contractRevision int64
	if err := tx.QueryRowContext(ctx, `SELECT outcome_id,status,plan_revision_id,work_unit_id,run_intent_generation,contract_revision_number FROM attempts WHERE id=?`, d.PredecessorAttemptID).Scan(&outcome, &status, &plan, &unit, &generation, &contractRevision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return d, false, &ports.AttemptReplacementDecisionBindingError{Reason: "predecessor_missing"}
		}
		return d, false, err
	}
	if outcome != d.OutcomeID || !status.Terminal() || plan != d.PlanRevisionID || unit != d.WorkUnitID || generation != d.RunIntentGeneration || contractRevision != d.ContractRevisionNumber {
		return d, false, &ports.AttemptReplacementDecisionBindingError{Reason: "predecessor_mismatch"}
	}
	var current, currentContract int64
	var currentPlan domain.PlanRevisionID
	var currentDesired domain.RunIntentDesired
	if err := tx.QueryRowContext(ctx, `SELECT generation, contract_revision_number, plan_revision_id, desired FROM outcome_run_intents WHERE outcome_id=? ORDER BY generation DESC LIMIT 1`, d.OutcomeID).Scan(&current, &currentContract, &currentPlan, &currentDesired); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return d, false, &ports.AttemptReplacementDecisionBindingError{Reason: "run_missing"}
		}
		return d, false, err
	}
	var outcomeContract int64
	if err := tx.QueryRowContext(ctx, `SELECT current_revision_number FROM outcomes WHERE id=?`, d.OutcomeID).Scan(&outcomeContract); err != nil {
		return d, false, err
	}
	if current != d.RunIntentGeneration || currentContract != d.ContractRevisionNumber || currentPlan != d.PlanRevisionID || outcomeContract != d.ContractRevisionNumber || currentDesired != domain.RunIntentRunning {
		return d, false, &ports.AttemptReplacementDecisionBindingError{Reason: "run_not_current"}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO attempt_replacement_decisions(id,outcome_id,predecessor_attempt_id,plan_revision_id,work_unit_id,run_intent_generation,contract_revision_number,action,request_key,request_fingerprint,owner_principal,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, d.ID, d.OutcomeID, d.PredecessorAttemptID, d.PlanRevisionID, d.WorkUnitID, d.RunIntentGeneration, d.ContractRevisionNumber, d.Action, d.RequestKey, d.RequestFingerprint, d.OwnerPrincipal, d.CreatedAt)
	if err != nil {
		if x, ok, e := replacementDecisionByKey(ctx, tx, d.RequestKey); e == nil && ok {
			if replacementDecisionSameSemantics(x, d) {
				return x, false, nil
			}
			return x, false, &ports.AttemptReplacementDecisionConflictError{Existing: x}
		}
		return d, false, err
	}
	if err := tx.Commit(); err != nil {
		return d, false, err
	}
	return d, true, nil
}
func (s *Store) GetAttemptReplacementDecision(ctx context.Context, id domain.AttemptReplacementDecisionID) (domain.AttemptReplacementDecision, bool, error) {
	return replacementDecisionWhere(ctx, s.readDB, "id", id)
}
func replacementDecisionByKey(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, key string) (domain.AttemptReplacementDecision, bool, error) {
	return replacementDecisionWhere(ctx, q, "request_key", key)
}
func replacementDecisionWhere(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, column string, arg any) (domain.AttemptReplacementDecision, bool, error) {
	var d domain.AttemptReplacementDecision
	err := q.QueryRowContext(ctx, `SELECT id,outcome_id,predecessor_attempt_id,plan_revision_id,work_unit_id,run_intent_generation,contract_revision_number,action,request_key,request_fingerprint,owner_principal,created_at FROM attempt_replacement_decisions WHERE `+column+`=?`, arg).Scan(&d.ID, &d.OutcomeID, &d.PredecessorAttemptID, &d.PlanRevisionID, &d.WorkUnitID, &d.RunIntentGeneration, &d.ContractRevisionNumber, &d.Action, &d.RequestKey, &d.RequestFingerprint, &d.OwnerPrincipal, &d.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return d, false, nil
	}
	return d, err == nil, err
}

func replacementDecisionSameSemantics(a, b domain.AttemptReplacementDecision) bool {
	return a.OutcomeID == b.OutcomeID && a.PredecessorAttemptID == b.PredecessorAttemptID &&
		a.PlanRevisionID == b.PlanRevisionID && a.WorkUnitID == b.WorkUnitID &&
		a.RunIntentGeneration == b.RunIntentGeneration && a.ContractRevisionNumber == b.ContractRevisionNumber &&
		a.Action == b.Action && a.RequestKey == b.RequestKey && a.RequestFingerprint == b.RequestFingerprint &&
		a.OwnerPrincipal == b.OwnerPrincipal
}
