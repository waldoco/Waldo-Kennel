package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

func (s *Store) ClaimAttemptBudgetStop(ctx context.Context, claim domain.AttemptBudgetStop) (domain.AttemptBudgetStop, bool, error) {
	if err := claim.Validate(); err != nil {
		return domain.AttemptBudgetStop{}, false, err
	}
	canonical, err := domain.CanonicalJSON(claim.MeasuredUsage)
	if err != nil {
		return domain.AttemptBudgetStop{}, false, err
	}
	claim.MeasuredUsage = canonical
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	res, err := s.writeDB.ExecContext(ctx, `INSERT INTO attempt_budget_stops(attempt_id,session_id,reason_code,measured_usage,claimed_at) VALUES(?,?,?,?,?) ON CONFLICT(attempt_id) DO NOTHING`, claim.AttemptID, claim.SessionID, claim.Reason, claim.MeasuredUsage, claim.ClaimedAt)
	if err != nil {
		if !isSQLiteBusy(err) {
			return domain.AttemptBudgetStop{}, false, err
		}
		// A peer may be committing the same stop claim. Reread boundedly so
		// same-reason threshold reporters converge on its durable winner.
		for retry := 0; retry < 8; retry++ {
			time.Sleep(time.Duration(retry+1) * time.Millisecond)
			got, found, readErr := s.getAttemptBudgetStop(ctx, s.readDB, claim.AttemptID)
			if readErr == nil && found {
				if got.SessionID == claim.SessionID && got.Reason == claim.Reason {
					return got, false, nil
				}
				return got, false, fmt.Errorf("attempt budget stop claim conflicts with durable claim")
			}
			if readErr != nil && !isSQLiteBusy(readErr) {
				return domain.AttemptBudgetStop{}, false, readErr
			}
		}
		return domain.AttemptBudgetStop{}, false, &ports.ExecutionUsageBusyError{Err: err}
	}
	n, _ := res.RowsAffected()
	got, ok, err := s.getAttemptBudgetStop(ctx, s.writeDB, claim.AttemptID)
	if err != nil || !ok {
		return domain.AttemptBudgetStop{}, false, err
	}
	if got.SessionID != claim.SessionID || got.Reason != claim.Reason {
		return got, false, fmt.Errorf("attempt budget stop claim conflicts with durable claim")
	}
	// Same-reason threshold reporters converge on the first durable measurement.
	// The winning claim owns stop evidence; losers reread rather than overwrite it.
	return got, n == 1, nil
}
func (s *Store) RecordAttemptBudgetProviderStopped(ctx context.Context, id domain.AttemptID, session string, reason domain.RuntimeBudgetReasonCode, result string, at time.Time) (domain.AttemptBudgetStop, error) {
	if !reason.Valid() || session == "" || !jsonValid(result) || at.IsZero() {
		return domain.AttemptBudgetStop{}, fmt.Errorf("invalid budget machine stop result")
	}
	canonical, err := domain.CanonicalJSON(result)
	if err != nil {
		return domain.AttemptBudgetStop{}, err
	}
	result = canonical
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	res, err := s.writeDB.ExecContext(ctx, `UPDATE attempt_budget_stops SET provider_stopped_at=?,machine_result=? WHERE attempt_id=? AND session_id=? AND reason_code=? AND provider_stopped_at IS NULL`, at, result, id, session, reason)
	if err != nil {
		return domain.AttemptBudgetStop{}, err
	}
	n, _ := res.RowsAffected()
	got, ok, err := s.getAttemptBudgetStop(ctx, s.writeDB, id)
	if err != nil || !ok {
		return domain.AttemptBudgetStop{}, sql.ErrNoRows
	}
	if n == 0 && (got.SessionID != session || got.Reason != reason || !domain.CanonicalJSONEqual(got.MachineResult, result)) {
		return got, fmt.Errorf("budget machine stop result conflicts with durable result")
	}
	return got, nil
}
func jsonValid(v string) bool { return len(v) > 0 && (v[0] == '{' || v[0] == '[') }
func (s *Store) GetAttemptBudgetStop(ctx context.Context, id domain.AttemptID) (domain.AttemptBudgetStop, bool, error) {
	return s.getAttemptBudgetStop(ctx, s.readDB, id)
}
func (s *Store) getAttemptBudgetStop(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id domain.AttemptID) (domain.AttemptBudgetStop, bool, error) {
	var x domain.AttemptBudgetStop
	var stopped sql.NullTime
	err := q.QueryRowContext(ctx, `SELECT attempt_id,session_id,reason_code,measured_usage,claimed_at,provider_stopped_at,machine_result FROM attempt_budget_stops WHERE attempt_id=?`, id).Scan(&x.AttemptID, &x.SessionID, &x.Reason, &x.MeasuredUsage, &x.ClaimedAt, &stopped, &x.MachineResult)
	if errors.Is(err, sql.ErrNoRows) {
		return x, false, nil
	}
	if err != nil {
		return x, false, err
	}
	if stopped.Valid {
		x.ProviderStoppedAt = &stopped.Time
	}
	return x, true, nil
}
func (s *Store) ListUnfinishedAttemptBudgetStops(ctx context.Context) ([]domain.AttemptBudgetStop, error) {
	rows, err := s.readDB.QueryContext(ctx, `SELECT attempt_id,session_id,reason_code,measured_usage,claimed_at,provider_stopped_at,machine_result FROM attempt_budget_stops bs JOIN attempts a ON a.id=bs.attempt_id WHERE a.status='running' ORDER BY bs.claimed_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.AttemptBudgetStop
	for rows.Next() {
		var x domain.AttemptBudgetStop
		var stopped sql.NullTime
		if err := rows.Scan(&x.AttemptID, &x.SessionID, &x.Reason, &x.MeasuredUsage, &x.ClaimedAt, &stopped, &x.MachineResult); err != nil {
			return nil, err
		}
		if stopped.Valid {
			x.ProviderStoppedAt = &stopped.Time
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
