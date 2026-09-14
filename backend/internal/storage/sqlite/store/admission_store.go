package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/gen"
)

var _ ports.AdmissionStore = (*Store)(nil)

func (s *Store) GetAdmittedVerdict(ctx context.Context, planID domain.PlanRevisionID) (domain.AdmissionVerdict, bool, error) {
	var raw string
	if err := s.readDB.QueryRowContext(ctx, `SELECT verdict_json FROM admission_verdicts WHERE plan_revision_id=? AND status='admitted'`, planID).Scan(&raw); errors.Is(err, sql.ErrNoRows) {
		return domain.AdmissionVerdict{}, false, nil
	} else if err != nil {
		return domain.AdmissionVerdict{}, false, err
	}
	var verdict domain.AdmissionVerdict
	if err := json.Unmarshal([]byte(raw), &verdict); err != nil {
		return verdict, true, err
	}
	return verdict, true, verdict.Validate()
}

func (s *Store) AppendAdmissionEvaluation(ctx context.Context, verdict domain.AdmissionVerdict) error {
	if err := verdict.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(verdict)
	if err != nil {
		return err
	}
	var plan any
	if verdict.PlanRevisionID != nil {
		plan = *verdict.PlanRevisionID
	}
	var rev any
	if verdict.ContractRevisionNumber != nil {
		rev = *verdict.ContractRevisionNumber
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err = s.writeDB.ExecContext(ctx, `INSERT INTO admission_verdicts(id,outcome_id,plan_revision_id,contract_revision_number,status,policy_version,evaluated_at,verdict_json) VALUES(?,?,?,?,?,?,?,?)`, verdict.ID, verdict.OutcomeID, plan, rev, verdict.Status, verdict.PolicyVersion, verdict.EvaluatedAt, string(raw))
	return err
}

func (s *Store) ApprovePlanWithAdmission(ctx context.Context, outcomeID domain.OutcomeID, planID domain.PlanRevisionID, verdict domain.AdmissionVerdict) (domain.PlanRevision, bool, error) {
	if verdict.Status != domain.AdmissionAdmitted {
		return domain.PlanRevision{}, true, fmt.Errorf("plan admission is %s", verdict.Status)
	}
	if err := verdict.Validate(); err != nil {
		return domain.PlanRevision{}, true, err
	}
	raw, err := json.Marshal(verdict)
	if err != nil {
		return domain.PlanRevision{}, true, err
	}
	s.writeMu.Lock()
	tx, err := s.writeDB.BeginTx(ctx, nil)
	if err != nil {
		s.writeMu.Unlock()
		return domain.PlanRevision{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	txq := s.qw.WithTx(tx)
	rows, err := txq.ApprovePlanRevision(ctx, gen.ApprovePlanRevisionParams{ID: planID, OutcomeID: outcomeID})
	if err != nil {
		s.writeMu.Unlock()
		return domain.PlanRevision{}, false, err
	}
	if rows == 0 {
		row, readErr := txq.GetPlanRevision(ctx, gen.GetPlanRevisionParams{ID: planID, OutcomeID: outcomeID})
		if errors.Is(readErr, sql.ErrNoRows) {
			s.writeMu.Unlock()
			return domain.PlanRevision{}, false, nil
		}
		if readErr != nil || domain.PlanStatus(row.Status) != domain.PlanStatusApproved {
			s.writeMu.Unlock()
			return domain.PlanRevision{}, true, fmt.Errorf("plan is not approvable")
		}
		var storedRaw string
		if readErr := tx.QueryRowContext(ctx, `SELECT verdict_json FROM admission_verdicts WHERE plan_revision_id=? AND status='admitted'`, planID).Scan(&storedRaw); readErr != nil {
			s.writeMu.Unlock()
			return domain.PlanRevision{}, true, fmt.Errorf("approved plan admission evidence is incomplete: %w", readErr)
		}
		var stored domain.AdmissionVerdict
		if err := json.Unmarshal([]byte(storedRaw), &stored); err != nil || stored.OutcomeID != outcomeID || stored.PlanRevisionID == nil || *stored.PlanRevisionID != planID || stored.ContractRevisionNumber == nil || verdict.ContractRevisionNumber == nil || *stored.ContractRevisionNumber != *verdict.ContractRevisionNumber || stored.PolicyVersion != verdict.PolicyVersion || !equalAdmissionSpecs(stored, verdict) {
			s.writeMu.Unlock()
			return domain.PlanRevision{}, true, fmt.Errorf("approval replay differs from persisted admission evidence")
		}
		_ = tx.Rollback()
		s.writeMu.Unlock()
		return s.GetPlanRevision(ctx, outcomeID, planID)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO admission_verdicts(id,outcome_id,plan_revision_id,contract_revision_number,status,policy_version,evaluated_at,verdict_json) VALUES(?,?,?,?,?,?,?,?)`, verdict.ID, outcomeID, planID, *verdict.ContractRevisionNumber, verdict.Status, verdict.PolicyVersion, verdict.EvaluatedAt, string(raw)); err != nil {
		s.writeMu.Unlock()
		return domain.PlanRevision{}, true, fmt.Errorf("persist admission verdict: %w", err)
	}
	for _, wu := range verdict.WorkUnits {
		specRaw, marshalErr := json.Marshal(wu.Executable)
		if marshalErr != nil {
			s.writeMu.Unlock()
			return domain.PlanRevision{}, true, marshalErr
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO approved_executable_specs(plan_revision_id,work_unit_id,digest,spec_json) VALUES(?,?,?,?)`, planID, wu.WorkUnitID, wu.Executable.Digest, string(specRaw)); err != nil {
			s.writeMu.Unlock()
			return domain.PlanRevision{}, true, fmt.Errorf("persist approved executable spec: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		s.writeMu.Unlock()
		return domain.PlanRevision{}, true, err
	}
	s.writeMu.Unlock()
	return s.GetPlanRevision(ctx, outcomeID, planID)
}

func (s *Store) GetApprovedExecutableSpec(ctx context.Context, planID domain.PlanRevisionID, unitID domain.WorkUnitID) (domain.ApprovedExecutableSpec, bool, error) {
	var raw, digest string
	err := s.readDB.QueryRowContext(ctx, `SELECT plan_revision_id,work_unit_id,digest,spec_json FROM approved_executable_specs WHERE plan_revision_id=? AND work_unit_id=?`, planID, unitID).Scan(&planID, &unitID, &digest, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ApprovedExecutableSpec{}, false, nil
	}
	if err != nil {
		return domain.ApprovedExecutableSpec{}, false, err
	}
	var spec domain.ApprovedExecutableSpec
	if err = json.Unmarshal([]byte(raw), &spec); err != nil {
		return spec, true, err
	}
	if spec.PlanRevisionID != planID || spec.WorkUnitID != unitID || spec.Digest != digest {
		return spec, true, fmt.Errorf("approved spec SQL/JSON identity mismatch")
	}
	return spec, true, spec.Validate()
}

func (s *Store) PersistWorkspaceBoundLaunchPacket(ctx context.Context, packet domain.WorkspaceBoundLaunchPacket) error {
	if err := packet.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(packet)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err = s.writeDB.ExecContext(ctx, `INSERT INTO workspace_bound_launch_packets(attempt_id,spec_digest,session_id,digest,packet_json) VALUES(?,?,?,?,?)`, packet.AttemptID, packet.SpecDigest, packet.SessionID, packet.Digest, string(raw))
	if err != nil {
		return fmt.Errorf("persist workspace-bound launch packet: %w", err)
	}
	return nil
}
func (s *Store) GetWorkspaceBoundLaunchPacket(ctx context.Context, attemptID domain.AttemptID) (domain.WorkspaceBoundLaunchPacket, bool, error) {
	var raw, specDigest, sessionID, digest string
	var storedAttempt domain.AttemptID
	err := s.readDB.QueryRowContext(ctx, `SELECT attempt_id,spec_digest,session_id,digest,packet_json FROM workspace_bound_launch_packets WHERE attempt_id=?`, attemptID).Scan(&storedAttempt, &specDigest, &sessionID, &digest, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.WorkspaceBoundLaunchPacket{}, false, nil
	}
	if err != nil {
		return domain.WorkspaceBoundLaunchPacket{}, false, err
	}
	var packet domain.WorkspaceBoundLaunchPacket
	if err = json.Unmarshal([]byte(raw), &packet); err != nil {
		return packet, true, err
	}
	if packet.AttemptID != storedAttempt || packet.SpecDigest != specDigest || packet.SessionID != sessionID || packet.Digest != digest {
		return packet, true, fmt.Errorf("launch packet SQL/JSON identity mismatch")
	}
	return packet, true, packet.Validate()
}

func equalAdmissionSpecs(a, b domain.AdmissionVerdict) bool {
	if a.Status != b.Status || len(a.WorkUnits) != len(b.WorkUnits) {
		return false
	}
	m := map[domain.WorkUnitID]string{}
	for _, u := range a.WorkUnits {
		if u.Executable == nil {
			return false
		}
		m[u.WorkUnitID] = u.Executable.Digest
	}
	for _, u := range b.WorkUnits {
		if u.Executable == nil || m[u.WorkUnitID] != u.Executable.Digest {
			return false
		}
	}
	return true
}
