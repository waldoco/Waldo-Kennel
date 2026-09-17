package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/gen"
)

// GetOutcomeProjectID resolves the project backing an Outcome.
func (s *Store) GetOutcomeProjectID(ctx context.Context, outcomeID domain.OutcomeID) (domain.ProjectID, bool, error) {
	projectID, err := s.qr.GetOutcomeProjectID(ctx, outcomeID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("resolve project for outcome %s: %w", outcomeID, err)
	}
	return projectID, true, nil
}

// FindAttemptByIdempotencyKey resolves a previously delivered start request.
func (s *Store) FindAttemptByIdempotencyKey(ctx context.Context, key string) (domain.Attempt, bool, error) {
	row, err := s.qr.FindAttemptByIdempotencyKey(ctx, sql.NullString{String: key, Valid: key != ""})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Attempt{}, false, nil
	}
	if err != nil {
		return domain.Attempt{}, false, fmt.Errorf("find attempt by idempotency key: %w", err)
	}
	return attemptFromFindRow(row), true, nil
}

// CreateAttemptWithFence atomically persists one scheduler-selected WorkUnit
// Attempt and issues its custody fence. Storage receives exact canonical
// identity; it never inspects a Plan or chooses a WorkUnit itself.
func (s *Store) CreateAttemptWithFence(ctx context.Context, in ports.AttemptAdmission) (domain.Attempt, error) {
	if in.OutcomeID.IsZero() || in.PlanRevisionID.IsZero() || in.WorkUnitID.IsZero() {
		return domain.Attempt{}, fmt.Errorf("attempt admission requires outcome, plan revision, and work unit ids")
	}
	if in.ContractRevisionNumber < 1 {
		return domain.Attempt{}, fmt.Errorf("attempt admission requires a contract revision")
	}
	if strings.TrimSpace(in.RequestKey) == "" {
		return domain.Attempt{}, fmt.Errorf("attempt admission requires a request key")
	}
	if strings.TrimSpace(in.FenceSubject) == "" {
		return domain.Attempt{}, fmt.Errorf("attempt admission requires a fence subject")
	}
	if in.At.IsZero() {
		return domain.Attempt{}, fmt.Errorf("attempt admission requires a timestamp")
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.writeDB.BeginTx(ctx, nil)
	if err != nil {
		return domain.Attempt{}, fmt.Errorf("begin create attempt for %s: %w", in.OutcomeID, err)
	}
	defer func() { _ = tx.Rollback() }()
	txq := s.qw.WithTx(tx)

	key := sql.NullString{String: strings.TrimSpace(in.RequestKey), Valid: true}
	if row, findErr := txq.FindAttemptByIdempotencyKey(ctx, key); findErr == nil {
		winner := attemptFromFindRow(row)
		if winner.OutcomeID != in.OutcomeID || winner.PlanRevisionID != in.PlanRevisionID || winner.WorkUnitID != in.WorkUnitID || winner.ContractRevisionNumber != in.ContractRevisionNumber {
			return winner, &ports.AttemptReplayConflictError{Attempt: winner, OutcomeID: in.OutcomeID, PlanRevisionID: in.PlanRevisionID, WorkUnitID: in.WorkUnitID}
		}
		return winner, &ports.AttemptReplayError{Attempt: winner}
	} else if !errors.Is(findErr, sql.ErrNoRows) {
		return domain.Attempt{}, findErr
	}

	var priorAttempts int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM attempts WHERE work_unit_id=?`, in.WorkUnitID).Scan(&priorAttempts); err != nil {
		return domain.Attempt{}, fmt.Errorf("count attempt lineage for %s: %w", in.WorkUnitID, err)
	}
	if in.RetryLimit != nil && priorAttempts > *in.RetryLimit {
		return domain.Attempt{}, &ports.AttemptRetryBudgetExceededError{WorkUnitID: in.WorkUnitID, RetryLimit: *in.RetryLimit, PriorAttempts: priorAttempts}
	}

	maxNum, err := txq.MaxAttemptNumber(ctx, in.OutcomeID)
	if err != nil {
		return domain.Attempt{}, fmt.Errorf("max attempt number for %s: %w", in.OutcomeID, err)
	}
	priorCount, ok := maxNum.(int64)
	if !ok {
		return domain.Attempt{}, fmt.Errorf("max attempt number for %s: unexpected type %T", in.OutcomeID, maxNum)
	}

	// Re-read the owner authorization on the same transaction that creates the
	// Attempt and fence. A service-level precheck is useful for fast refusal,
	// but only this boundary can prevent a pause/cancel from landing between
	// that read and durable admission.
	currentIntent, intentErr := txq.CurrentOutcomeRunIntent(ctx, string(in.OutcomeID))
	currentGeneration := int64(0)
	currentDesired := domain.RunIntentDesired("")
	if intentErr == nil {
		currentGeneration = currentIntent.Generation
		currentDesired = domain.RunIntentDesired(currentIntent.Desired)
	} else if !errors.Is(intentErr, sql.ErrNoRows) {
		return domain.Attempt{}, fmt.Errorf("read run authorization for %s: %w", in.OutcomeID, intentErr)
	}
	if currentDesired == "" {
		currentDesired = domain.RunIntentIdle
	}
	if currentDesired != domain.RunIntentIdle && currentDesired != domain.RunIntentRunning {
		return domain.Attempt{}, &ports.AttemptRunIntentConflictError{
			OutcomeID: in.OutcomeID, Expected: in.RunIntentGeneration,
			Current: currentGeneration, Desired: currentDesired, Found: currentGeneration > 0,
		}
	}
	if in.RunIntentGeneration > 0 {
		if currentGeneration != in.RunIntentGeneration || currentDesired != domain.RunIntentRunning || domain.PlanRevisionID(currentIntent.PlanRevisionID) != in.PlanRevisionID || currentIntent.ContractRevisionNumber != in.ContractRevisionNumber {
			return domain.Attempt{}, &ports.AttemptRunIntentConflictError{
				OutcomeID: in.OutcomeID, Expected: in.RunIntentGeneration,
				Current: currentGeneration, Desired: currentDesired, Found: currentGeneration > 0,
			}
		}
	} else if currentDesired == domain.RunIntentRunning {
		// A direct Attempt start that raced with a newly recorded Start is
		// still bound to the current authorization, so its durable lineage is
		// explicit rather than silently bypassing the run intent.
		in.RunIntentGeneration = currentGeneration
		if domain.PlanRevisionID(currentIntent.PlanRevisionID) != in.PlanRevisionID || currentIntent.ContractRevisionNumber != in.ContractRevisionNumber {
			return domain.Attempt{}, &ports.AttemptRunIntentConflictError{
				OutcomeID: in.OutcomeID, Expected: 0,
				Current: currentGeneration, Desired: currentDesired, Found: true,
			}
		}
	}

	attempt := domain.Attempt{
		ID:                     domain.AttemptID("att-" + uuid.NewString()),
		OutcomeID:              in.OutcomeID,
		PlanRevisionID:         in.PlanRevisionID,
		WorkUnitID:             in.WorkUnitID,
		Number:                 priorCount + 1,
		Status:                 domain.AttemptQueued,
		RequestKey:             key.String,
		CreatedAt:              in.At,
		UpdatedAt:              in.At,
		ContractRevisionNumber: in.ContractRevisionNumber,
		RunIntentGeneration:    in.RunIntentGeneration,
	}
	if err := attempt.Validate(); err != nil {
		return domain.Attempt{}, err
	}
	if err := txq.CreateAttempt(ctx, gen.CreateAttemptParams{
		ID:                     attempt.ID,
		OutcomeID:              attempt.OutcomeID,
		PlanRevisionID:         attempt.PlanRevisionID,
		WorkUnitID:             attempt.WorkUnitID,
		Number:                 attempt.Number,
		Status:                 attempt.Status,
		ContractRevisionNumber: attempt.ContractRevisionNumber,
		RunIntentGeneration:    attempt.RunIntentGeneration,
		RequestKey:             key,
	}); err != nil {
		if isSQLiteUnique(err) && strings.Contains(err.Error(), "request_key") {
			row, findErr := txq.FindAttemptByIdempotencyKey(ctx, key)
			if findErr == nil {
				winner := attemptFromFindRow(row)
				if winner.OutcomeID != in.OutcomeID || winner.PlanRevisionID != in.PlanRevisionID || winner.WorkUnitID != in.WorkUnitID || winner.ContractRevisionNumber != in.ContractRevisionNumber || winner.RunIntentGeneration != in.RunIntentGeneration {
					return winner, &ports.AttemptReplayConflictError{
						Attempt: winner, OutcomeID: in.OutcomeID, PlanRevisionID: in.PlanRevisionID, WorkUnitID: in.WorkUnitID,
					}
				}
				return winner, &ports.AttemptReplayError{Attempt: winner}
			}
		}
		return domain.Attempt{}, fmt.Errorf("create attempt for %s: %w", in.OutcomeID, err)
	}

	fenceID := "fence-" + uuid.NewString()
	if err := txq.IssueAttemptFence(ctx, gen.IssueAttemptFenceParams{
		ID:        fenceID,
		Subject:   in.FenceSubject,
		AttemptID: attempt.ID,
	}); err != nil {
		if isSQLiteUnique(err) {
			holder := domain.AttemptID("")
			if open, findErr := txq.FindOpenFenceBySubject(ctx, in.FenceSubject); findErr == nil {
				holder = open.AttemptID
			}
			return domain.Attempt{}, &ports.AttemptFenceHeldError{Subject: in.FenceSubject, Holder: holder, OutcomeID: in.OutcomeID}
		}
		return domain.Attempt{}, fmt.Errorf("issue fence for %s: %w", attempt.ID, err)
	}

	if err := tx.Commit(); err != nil {
		return domain.Attempt{}, fmt.Errorf("commit create attempt for %s: %w", in.OutcomeID, err)
	}
	return attempt, nil
}

// GetAttempt loads one attempt scoped to its Outcome.
func (s *Store) GetAttempt(ctx context.Context, outcomeID domain.OutcomeID, attemptID domain.AttemptID) (domain.Attempt, bool, error) {
	row, err := s.qr.GetAttempt(ctx, gen.GetAttemptParams{ID: attemptID, OutcomeID: outcomeID})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Attempt{}, false, nil
	}
	if err != nil {
		return domain.Attempt{}, false, fmt.Errorf("get attempt %s: %w", attemptID, err)
	}
	return attemptFromGetRow(row), true, nil
}

// ListAttempts loads attempts belonging to an Outcome.
func (s *Store) ListAttempts(ctx context.Context, outcomeID domain.OutcomeID) ([]domain.Attempt, error) {
	rows, err := s.qr.ListAttemptsForOutcome(ctx, outcomeID)
	if err != nil {
		return nil, fmt.Errorf("list attempts for %s: %w", outcomeID, err)
	}
	out := make([]domain.Attempt, 0, len(rows))
	for _, row := range rows {
		out = append(out, attemptFromListOutcomeRow(row))
	}
	return out, nil
}

// TransitionAttemptStatus advances an attempt with optimistic concurrency.
func (s *Store) TransitionAttemptStatus(ctx context.Context, outcomeID domain.OutcomeID, attemptID domain.AttemptID, expected, next domain.AttemptStatus, at time.Time) (int64, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	rows, err := s.qw.TransitionAttemptStatus(ctx, gen.TransitionAttemptStatusParams{
		Status: next, UpdatedAt: at, ID: attemptID, OutcomeID: outcomeID, Status_2: expected,
	})
	if err != nil {
		return 0, fmt.Errorf("transition attempt %s %s->%s: %w", attemptID, expected, next, err)
	}
	return rows, nil
}

// ListAttemptsByStatus loads attempts in a durable status.
func (s *Store) ListAttemptsByStatus(ctx context.Context, status domain.AttemptStatus) ([]domain.Attempt, error) {
	rows, err := s.qr.ListAttemptsByStatus(ctx, status)
	if err != nil {
		return nil, fmt.Errorf("list attempts by status %s: %w", status, err)
	}
	out := make([]domain.Attempt, 0, len(rows))
	for _, row := range rows {
		out = append(out, attemptFromListStatusRow(row))
	}
	return out, nil
}

// BindAttemptSession records a provider session reference for an attempt.
func (s *Store) BindAttemptSession(ctx context.Context, ref domain.AttemptSessionRef) (domain.AttemptSessionRef, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.writeDB.BeginTx(ctx, nil)
	if err != nil {
		return domain.AttemptSessionRef{}, fmt.Errorf("begin bind session for %s: %w", ref.AttemptID, err)
	}
	defer func() { _ = tx.Rollback() }()
	txq := s.qw.WithTx(tx)
	var status domain.AttemptStatus
	if err := tx.QueryRowContext(ctx, `SELECT status FROM attempts WHERE id=?`, ref.AttemptID).Scan(&status); err != nil {
		return domain.AttemptSessionRef{}, fmt.Errorf("read attempt status for session bind: %w", err)
	}
	if status == domain.AttemptAwaitingAuthority {
		return domain.AttemptSessionRef{}, fmt.Errorf("awaiting-authority attempt cannot bind a session")
	}

	seq, err := latestSessionRefSeq(ctx, txq, ref.AttemptID)
	if err != nil {
		return domain.AttemptSessionRef{}, err
	}
	ref.Seq = seq + 1
	if ref.ID.IsZero() {
		ref.ID = domain.AttemptSessionRefID("asr-" + uuid.NewString())
	}
	if ref.BoundAt.IsZero() {
		ref.BoundAt = time.Now().UTC()
	}
	if err := ref.Validate(); err != nil {
		return domain.AttemptSessionRef{}, err
	}
	if err := txq.CreateAttemptSessionRef(ctx, gen.CreateAttemptSessionRefParams{
		ID: string(ref.ID), AttemptID: ref.AttemptID, Seq: ref.Seq, SessionID: ref.SessionID,
		Harness: ref.Harness, Mode: ref.Mode, RunBriefCoreDigest: ref.RunBriefCoreDigest,
		RunBriefCompiledDigest: ref.RunBriefCompiledDigest, AdmissionSnapshot: ref.AdmissionSnapshot,
	}); err != nil {
		return domain.AttemptSessionRef{}, fmt.Errorf("bind session for %s: %w", ref.AttemptID, err)
	}
	if err := tx.Commit(); err != nil {
		return domain.AttemptSessionRef{}, fmt.Errorf("commit bind session for %s: %w", ref.AttemptID, err)
	}
	return ref, nil
}

func latestSessionRefSeq(ctx context.Context, q *gen.Queries, attemptID domain.AttemptID) (int64, error) {
	latest, err := q.LatestAttemptSessionRef(ctx, attemptID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("latest session ref for %s: %w", attemptID, err)
	}
	return latest.Seq, nil
}

// LatestAttemptSessionRef loads the latest provider session reference.
func (s *Store) LatestAttemptSessionRef(ctx context.Context, attemptID domain.AttemptID) (domain.AttemptSessionRef, bool, error) {
	row, err := s.qr.LatestAttemptSessionRef(ctx, attemptID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AttemptSessionRef{}, false, nil
	}
	if err != nil {
		return domain.AttemptSessionRef{}, false, fmt.Errorf("latest session ref for %s: %w", attemptID, err)
	}
	return attemptSessionRefFromRow(row), true, nil
}

// LatestAttemptSessionRefForSession loads the latest governed Attempt binding
// for a provider session. Recovery uses this as evidence, never as a mutable
// preference source.
func (s *Store) LatestAttemptSessionRefForSession(ctx context.Context, sessionID string) (domain.AttemptSessionRef, bool, error) {
	row, err := s.qr.LatestAttemptSessionRefForSession(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AttemptSessionRef{}, false, nil
	}
	if err != nil {
		return domain.AttemptSessionRef{}, false, fmt.Errorf("latest session ref for %s: %w", sessionID, err)
	}
	return attemptSessionRefFromRow(row), true, nil
}

// ListAttemptSessionRefs loads all provider session references for an attempt.
func (s *Store) ListAttemptSessionRefs(ctx context.Context, attemptID domain.AttemptID) ([]domain.AttemptSessionRef, error) {
	rows, err := s.qr.ListAttemptSessionRefsForAttempt(ctx, attemptID)
	if err != nil {
		return nil, fmt.Errorf("list session refs for %s: %w", attemptID, err)
	}
	out := make([]domain.AttemptSessionRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, attemptSessionRefFromRow(row))
	}
	return out, nil
}

// AppendAttemptObservation records one bounded attempt observation.
func (s *Store) AppendAttemptObservation(ctx context.Context, attemptID domain.AttemptID, kind, payload string, at time.Time) (domain.AttemptObservation, error) {
	if payload == "" {
		payload = "{}"
	}
	obs := domain.AttemptObservation{ID: "obs-" + uuid.NewString(), AttemptID: attemptID, Kind: kind, Payload: payload, CreatedAt: at}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.writeDB.BeginTx(ctx, nil)
	if err != nil {
		return domain.AttemptObservation{}, fmt.Errorf("begin observation for %s: %w", attemptID, err)
	}
	defer func() { _ = tx.Rollback() }()
	txq := s.qw.WithTx(tx)

	maxSeq, err := txq.MaxAttemptObservationSeq(ctx, attemptID)
	if err != nil {
		return domain.AttemptObservation{}, fmt.Errorf("max observation seq for %s: %w", attemptID, err)
	}
	switch v := maxSeq.(type) {
	case int64:
		obs.Seq = v + 1
	default:
		return domain.AttemptObservation{}, fmt.Errorf("max observation seq for %s: unexpected type %T", attemptID, maxSeq)
	}
	if err := obs.Validate(); err != nil {
		return domain.AttemptObservation{}, err
	}
	if err := txq.CreateAttemptObservation(ctx, gen.CreateAttemptObservationParams{ID: obs.ID, AttemptID: obs.AttemptID, Seq: obs.Seq, Kind: obs.Kind, Payload: obs.Payload}); err != nil {
		return domain.AttemptObservation{}, fmt.Errorf("append observation for %s: %w", attemptID, err)
	}
	if err := tx.Commit(); err != nil {
		return domain.AttemptObservation{}, fmt.Errorf("commit observation for %s: %w", attemptID, err)
	}
	return obs, nil
}

// FailAttemptBeforeLaunch atomically records the known failure, ends the
// queued Attempt, and releases its custody. A crash can therefore never expose
// a failed Attempt whose open fence permanently blocks a fresh admission.
func (s *Store) FailAttemptBeforeLaunch(ctx context.Context, in ports.AttemptPrelaunchFailure) (domain.AttemptObservation, error) {
	if in.OutcomeID.IsZero() || in.AttemptID.IsZero() || strings.TrimSpace(in.ObservationKind) == "" || strings.TrimSpace(in.ReleaseReason) == "" || in.At.IsZero() {
		return domain.AttemptObservation{}, fmt.Errorf("prelaunch failure requires outcome, attempt, observation kind, release reason, and timestamp")
	}
	payload := in.ObservationPayload
	if payload == "" {
		payload = "{}"
	}
	obs := domain.AttemptObservation{ID: "obs-" + uuid.NewString(), AttemptID: in.AttemptID, Kind: in.ObservationKind, Payload: payload, CreatedAt: in.At}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.writeDB.BeginTx(ctx, nil)
	if err != nil {
		return domain.AttemptObservation{}, fmt.Errorf("begin prelaunch failure for %s: %w", in.AttemptID, err)
	}
	defer func() { _ = tx.Rollback() }()
	txq := s.qw.WithTx(tx)
	maxSeq, err := txq.MaxAttemptObservationSeq(ctx, in.AttemptID)
	if err != nil {
		return domain.AttemptObservation{}, fmt.Errorf("max observation seq for %s: %w", in.AttemptID, err)
	}
	prior, ok := maxSeq.(int64)
	if !ok {
		return domain.AttemptObservation{}, fmt.Errorf("max observation seq for %s: unexpected type %T", in.AttemptID, maxSeq)
	}
	obs.Seq = prior + 1
	if err := obs.Validate(); err != nil {
		return domain.AttemptObservation{}, err
	}
	if err := txq.CreateAttemptObservation(ctx, gen.CreateAttemptObservationParams{ID: obs.ID, AttemptID: obs.AttemptID, Seq: obs.Seq, Kind: obs.Kind, Payload: obs.Payload}); err != nil {
		return domain.AttemptObservation{}, fmt.Errorf("append prelaunch observation for %s: %w", in.AttemptID, err)
	}
	rows, err := txq.TransitionAttemptStatus(ctx, gen.TransitionAttemptStatusParams{Status: domain.AttemptFailed, UpdatedAt: in.At, ID: in.AttemptID, OutcomeID: in.OutcomeID, Status_2: domain.AttemptQueued})
	if err != nil {
		return domain.AttemptObservation{}, fmt.Errorf("end prelaunch attempt %s: %w", in.AttemptID, err)
	}
	if rows != 1 {
		return domain.AttemptObservation{}, fmt.Errorf("end prelaunch attempt %s: %d rows changed", in.AttemptID, rows)
	}
	rows, err = txq.ReleaseAttemptFence(ctx, gen.ReleaseAttemptFenceParams{ReleasedAt: sql.NullTime{Time: in.At, Valid: true}, ReleaseReason: in.ReleaseReason, AttemptID: in.AttemptID})
	if err != nil {
		return domain.AttemptObservation{}, fmt.Errorf("release prelaunch fence for %s: %w", in.AttemptID, err)
	}
	if rows != 1 {
		return domain.AttemptObservation{}, fmt.Errorf("release prelaunch fence for %s: %d rows changed", in.AttemptID, rows)
	}
	if err := tx.Commit(); err != nil {
		return domain.AttemptObservation{}, fmt.Errorf("commit prelaunch failure for %s: %w", in.AttemptID, err)
	}
	return obs, nil
}

// TerminateRunningAttemptWithObservation atomically classifies a Running
// attempt the reconcile loop found durably terminated: the classification
// observation, the Running-to-target transition, and (when ReleaseReason is
// set) custody release either all commit or all roll back together. This
// closes the gap where a crash or later write failure between the status
// flip and custody release could strand a terminal Attempt's workspace
// fence, since normal liveness scanning only revisits Running attempts.
//
// A false second return means the attempt had already moved off Running
// (transitioned concurrently, or moved by an earlier retry that committed):
// nothing was written, and the caller should treat this tick as a no-op —
// the next pass observes whatever the durable winner left behind.
func (s *Store) TerminateRunningAttemptWithObservation(ctx context.Context, in ports.AttemptRunningTermination) (domain.AttemptObservation, bool, error) {
	if in.OutcomeID.IsZero() || in.AttemptID.IsZero() || strings.TrimSpace(string(in.TargetStatus)) == "" ||
		strings.TrimSpace(in.ObservationKind) == "" || in.At.IsZero() {
		return domain.AttemptObservation{}, false, fmt.Errorf("running attempt termination requires outcome, attempt, target status, observation kind, and timestamp")
	}
	payload := in.ObservationPayload
	if payload == "" {
		payload = "{}"
	}
	obs := domain.AttemptObservation{ID: "obs-" + uuid.NewString(), AttemptID: in.AttemptID, Kind: in.ObservationKind, Payload: payload, CreatedAt: in.At}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.writeDB.BeginTx(ctx, nil)
	if err != nil {
		return domain.AttemptObservation{}, false, fmt.Errorf("begin running attempt termination for %s: %w", in.AttemptID, err)
	}
	defer func() { _ = tx.Rollback() }()
	txq := s.qw.WithTx(tx)

	rows, err := txq.TransitionAttemptStatus(ctx, gen.TransitionAttemptStatusParams{
		Status: in.TargetStatus, UpdatedAt: in.At, ID: in.AttemptID, OutcomeID: in.OutcomeID, Status_2: domain.AttemptRunning,
	})
	if err != nil {
		return domain.AttemptObservation{}, false, fmt.Errorf("transition running attempt %s: %w", in.AttemptID, err)
	}
	if rows == 0 {
		// Moved concurrently since the caller listed it as Running. Nothing to
		// append or release: whatever committed that transition owns this
		// Attempt's terminal facts now.
		return domain.AttemptObservation{}, false, nil
	}

	maxSeq, err := txq.MaxAttemptObservationSeq(ctx, in.AttemptID)
	if err != nil {
		return domain.AttemptObservation{}, false, fmt.Errorf("max observation seq for %s: %w", in.AttemptID, err)
	}
	prior, ok := maxSeq.(int64)
	if !ok {
		return domain.AttemptObservation{}, false, fmt.Errorf("max observation seq for %s: unexpected type %T", in.AttemptID, maxSeq)
	}
	obs.Seq = prior + 1
	if err := obs.Validate(); err != nil {
		return domain.AttemptObservation{}, false, err
	}
	if err := txq.CreateAttemptObservation(ctx, gen.CreateAttemptObservationParams{ID: obs.ID, AttemptID: obs.AttemptID, Seq: obs.Seq, Kind: obs.Kind, Payload: obs.Payload}); err != nil {
		return domain.AttemptObservation{}, false, fmt.Errorf("append running attempt observation for %s: %w", in.AttemptID, err)
	}

	if in.ReleaseReason != "" {
		fenceRows, err := txq.ReleaseAttemptFence(ctx, gen.ReleaseAttemptFenceParams{ReleasedAt: sql.NullTime{Time: in.At, Valid: true}, ReleaseReason: in.ReleaseReason, AttemptID: in.AttemptID})
		if err != nil {
			return domain.AttemptObservation{}, false, fmt.Errorf("release fence for terminated attempt %s: %w", in.AttemptID, err)
		}
		// fenceRows == 0 means no open fence was held (already released, or this
		// Attempt never held one): idempotent, not an error.
		_ = fenceRows
	}

	if err := tx.Commit(); err != nil {
		return domain.AttemptObservation{}, false, fmt.Errorf("commit running attempt termination for %s: %w", in.AttemptID, err)
	}
	return obs, true, nil
}

// ListAttemptObservations loads observations for an attempt.
func (s *Store) ListAttemptObservations(ctx context.Context, attemptID domain.AttemptID) ([]domain.AttemptObservation, error) {
	rows, err := s.qr.ListAttemptObservationsForAttempt(ctx, attemptID)
	if err != nil {
		return nil, fmt.Errorf("list observations for %s: %w", attemptID, err)
	}
	out := make([]domain.AttemptObservation, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.AttemptObservation{ID: row.ID, AttemptID: row.AttemptID, Seq: row.Seq, Kind: row.Kind, Payload: row.Payload, CreatedAt: row.CreatedAt})
	}
	return out, nil
}

// OpenFenceForSubject loads the active custody fence for a subject.
func (s *Store) OpenFenceForSubject(ctx context.Context, subject string) (domain.AttemptFence, bool, error) {
	row, err := s.qr.FindOpenFenceBySubject(ctx, subject)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AttemptFence{}, false, nil
	}
	if err != nil {
		return domain.AttemptFence{}, false, fmt.Errorf("open fence for %s: %w", subject, err)
	}
	return attemptFenceFromRow(row), true, nil
}

// ReleaseFenceForAttempt releases custody held by an attempt.
func (s *Store) ReleaseFenceForAttempt(ctx context.Context, attemptID domain.AttemptID, reason string, at time.Time) (int64, error) {
	if reason == "" {
		return 0, fmt.Errorf("release fence for %s: a released fence must record why", attemptID)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	rows, err := s.qw.ReleaseAttemptFence(ctx, gen.ReleaseAttemptFenceParams{ReleasedAt: sql.NullTime{Time: at, Valid: true}, ReleaseReason: reason, AttemptID: attemptID})
	if err != nil {
		return 0, fmt.Errorf("release fence for %s: %w", attemptID, err)
	}
	return rows, nil
}

// RenewFenceForAttempt renews custody held by an attempt.
func (s *Store) RenewFenceForAttempt(ctx context.Context, attemptID domain.AttemptID, at time.Time) (int64, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	rows, err := s.qw.RenewAttemptFence(ctx, gen.RenewAttemptFenceParams{LastRenewedAt: at, AttemptID: attemptID})
	if err != nil {
		return 0, fmt.Errorf("renew fence for %s: %w", attemptID, err)
	}
	return rows, nil
}

// CreateRecoveryReceipt persists one recovery decision receipt.
func (s *Store) CreateRecoveryReceipt(ctx context.Context, receipt domain.AttemptRecoveryReceipt) error {
	if receipt.ID == "" {
		receipt.ID = "rcpt-" + uuid.NewString()
	}
	if err := receipt.Validate(); err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	detail := receipt.Detail
	if detail == "" {
		detail = "{}"
	} else if !json.Valid([]byte(detail)) {
		return fmt.Errorf("recovery receipt %s detail must be valid JSON", receipt.ID)
	}
	if err := s.qw.CreateRecoveryReceipt(ctx, gen.CreateRecoveryReceiptParams{
		ID: receipt.ID, AttemptID: receipt.AttemptID, Resolution: string(receipt.Resolution),
		ReplacementAttemptID: string(receipt.ReplacementAttemptID), Detail: detail,
	}); err != nil {
		return fmt.Errorf("create recovery receipt for %s: %w", receipt.AttemptID, err)
	}
	return nil
}

// ListRecoveryReceipts loads recovery receipts for an attempt.
func (s *Store) ListRecoveryReceipts(ctx context.Context, attemptID domain.AttemptID) ([]domain.AttemptRecoveryReceipt, error) {
	rows, err := s.qr.ListRecoveryReceiptsForAttempt(ctx, attemptID)
	if err != nil {
		return nil, fmt.Errorf("list receipts for %s: %w", attemptID, err)
	}
	out := make([]domain.AttemptRecoveryReceipt, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.AttemptRecoveryReceipt{
			ID: row.ID, AttemptID: row.AttemptID, Resolution: domain.RecoveryResolution(row.Resolution),
			ReplacementAttemptID: domain.AttemptID(row.ReplacementAttemptID), Detail: row.Detail, CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}

func attemptFromFindRow(row gen.FindAttemptByIdempotencyKeyRow) domain.Attempt {
	return attemptFromValues(row.ID, row.OutcomeID, row.PlanRevisionID, row.WorkUnitID, row.Number, row.Status, row.ContractRevisionNumber, row.RunIntentGeneration, row.RequestKey, row.CreatedAt, row.UpdatedAt)
}

func attemptFromGetRow(row gen.GetAttemptRow) domain.Attempt {
	return attemptFromValues(row.ID, row.OutcomeID, row.PlanRevisionID, row.WorkUnitID, row.Number, row.Status, row.ContractRevisionNumber, row.RunIntentGeneration, row.RequestKey, row.CreatedAt, row.UpdatedAt)
}

func attemptFromListOutcomeRow(row gen.ListAttemptsForOutcomeRow) domain.Attempt {
	return attemptFromValues(row.ID, row.OutcomeID, row.PlanRevisionID, row.WorkUnitID, row.Number, row.Status, row.ContractRevisionNumber, row.RunIntentGeneration, row.RequestKey, row.CreatedAt, row.UpdatedAt)
}

func attemptFromListStatusRow(row gen.ListAttemptsByStatusRow) domain.Attempt {
	return attemptFromValues(row.ID, row.OutcomeID, row.PlanRevisionID, row.WorkUnitID, row.Number, row.Status, row.ContractRevisionNumber, row.RunIntentGeneration, row.RequestKey, row.CreatedAt, row.UpdatedAt)
}

func attemptFromValues(id domain.AttemptID, outcomeID domain.OutcomeID, planID domain.PlanRevisionID, unitID domain.WorkUnitID, number int64, status domain.AttemptStatus, contractRevision, runIntentGeneration int64, requestKeyValue sql.NullString, createdAt, updatedAt time.Time) domain.Attempt {
	var requestKey string
	if requestKeyValue.Valid {
		requestKey = requestKeyValue.String
	}
	return domain.Attempt{
		ID: id, OutcomeID: outcomeID, PlanRevisionID: planID, WorkUnitID: unitID,
		Number: number, Status: status, RequestKey: requestKey, CreatedAt: createdAt, UpdatedAt: updatedAt,
		ContractRevisionNumber: contractRevision, RunIntentGeneration: runIntentGeneration,
	}
}

func attemptSessionRefFromRow(row gen.AttemptSession) domain.AttemptSessionRef {
	return domain.AttemptSessionRef{
		ID: domain.AttemptSessionRefID(row.ID), AttemptID: row.AttemptID, Seq: row.Seq, SessionID: row.SessionID,
		Harness: row.Harness, Mode: row.Mode, RunBriefCoreDigest: row.RunBriefCoreDigest,
		RunBriefCompiledDigest: row.RunBriefCompiledDigest, AdmissionSnapshot: row.AdmissionSnapshot, BoundAt: row.BoundAt,
	}
}

func attemptFenceFromRow(row gen.AttemptFence) domain.AttemptFence {
	return domain.AttemptFence{
		ID: row.ID, Subject: row.Subject, AttemptID: row.AttemptID, IssuedAt: row.IssuedAt,
		LastRenewedAt: row.LastRenewedAt, ReleasedAt: row.ReleasedAt.Time, ReleaseReason: row.ReleaseReason,
	}
}

// AppendAttemptExecutionUsage stores one cumulative provider sample, derives its delta,
// and treats an exact sequence replay as idempotent.
func (s *Store) AppendAttemptExecutionUsage(ctx context.Context, sample domain.ExecutionUsageSample) (domain.ExecutionUsageSample, bool, error) {
	if err := sample.ValidateCumulative(); err != nil {
		return domain.ExecutionUsageSample{}, false, classifyExecutionUsageError(err)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.writeDB.BeginTx(ctx, nil)
	if err != nil {
		return domain.ExecutionUsageSample{}, false, classifyExecutionUsageError(err)
	}
	defer tx.Rollback()
	var pi, po, ps int64
	err = tx.QueryRowContext(ctx, `SELECT sequence,cumulative_input_tokens,cumulative_output_tokens FROM attempt_execution_usage WHERE attempt_id=? AND provider=? AND session_id=? ORDER BY sequence DESC LIMIT 1`, sample.AttemptID, sample.Provider, sample.SessionID).Scan(&ps, &pi, &po)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.ExecutionUsageSample{}, false, classifyExecutionUsageError(err)
	}
	if err == nil {
		if sample.Sequence == ps && sample.InputTokens == pi && sample.OutputTokens == po {
			sample.InputDelta = 0
			sample.OutputDelta = 0
			return sample, false, nil
		}
		if sample.Sequence <= ps || sample.InputTokens < pi || sample.OutputTokens < po {
			return domain.ExecutionUsageSample{}, false, fmt.Errorf("execution usage is not monotonic")
		}
	} else {
		pi, po = 0, 0
	}
	var status domain.AttemptStatus
	if err := tx.QueryRowContext(ctx, `SELECT status FROM attempts WHERE id=?`, sample.AttemptID).Scan(&status); err != nil {
		return domain.ExecutionUsageSample{}, false, classifyExecutionUsageError(err)
	}
	if status != domain.AttemptRunning {
		return domain.ExecutionUsageSample{}, false, fmt.Errorf("execution usage attempt is not running")
	}
	var claimed int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM attempt_budget_stops WHERE attempt_id=?`, sample.AttemptID).Scan(&claimed); err != nil {
		return domain.ExecutionUsageSample{}, false, classifyExecutionUsageError(err)
	}
	if claimed > 0 {
		return domain.ExecutionUsageSample{}, false, fmt.Errorf("execution usage budget stop already claimed")
	}
	sample.InputDelta = sample.InputTokens - pi
	sample.OutputDelta = sample.OutputTokens - po
	_, err = tx.ExecContext(ctx, `INSERT INTO attempt_execution_usage(attempt_id,provider,session_id,sequence,cumulative_input_tokens,cumulative_output_tokens,input_delta,output_delta,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, sample.AttemptID, sample.Provider, sample.SessionID, sample.Sequence, sample.InputTokens, sample.OutputTokens, sample.InputDelta, sample.OutputDelta, sample.CreatedAt)
	if err != nil {
		return domain.ExecutionUsageSample{}, false, classifyExecutionUsageError(err)
	}
	if err = tx.Commit(); err != nil {
		return domain.ExecutionUsageSample{}, false, classifyExecutionUsageError(err)
	}
	return sample, true, nil
}

func classifyExecutionUsageError(err error) error {
	if isSQLiteBusy(err) {
		return &ports.ExecutionUsageBusyError{Err: err}
	}
	return err
}

func (s *Store) WorkUnitExecutionUsage(ctx context.Context, unitID domain.WorkUnitID) (domain.ExecutionUsageTotals, error) {
	var t domain.ExecutionUsageTotals
	err := s.readDB.QueryRowContext(ctx, `SELECT COALESCE(SUM(u.input_delta),0),COALESCE(SUM(u.output_delta),0) FROM attempt_execution_usage u JOIN attempts a ON a.id=u.attempt_id WHERE a.work_unit_id=?`, unitID).Scan(&t.InputTokens, &t.OutputTokens)
	return t, err
}
