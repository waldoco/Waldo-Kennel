-- name: CreatePlanningSession :exec
INSERT INTO planning_sessions (
    id, outcome_id, project_id, contract_revision_id, contract_revision_number,
    revision, latest_turn_sequence, status, waiting_on, mode,
    requested_provider, model_selection, requested_model, requested_effort,
    context_mode, planning_grant_digest, context_digest, context_snapshot_json,
    effective_provider, effective_model, native_conversation_ref,
    proposed_plan_revision_id, last_failure_code, last_failure_detail,
    request_key, request_fingerprint, created_at, updated_at, closed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetPlanningSession :one
SELECT * FROM planning_sessions WHERE id = ? AND outcome_id = ?;

-- name: GetPlanningSessionByID :one
SELECT * FROM planning_sessions WHERE id = ?;

-- name: GetPlanningSessionByRequestKey :one
SELECT * FROM planning_sessions WHERE request_key = ?;

-- name: GetCurrentPlanningSession :one
SELECT * FROM planning_sessions WHERE outcome_id = ? ORDER BY created_at DESC, id DESC LIMIT 1;

-- name: CreatePlanningTurn :exec
INSERT INTO planning_turns (
    id, planning_session_id, sequence, reply_to_turn_id, role, kind, text,
    structured_payload_json, intelligence_run_id, request_key,
    request_fingerprint, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetPlanningTurnByRequestKey :one
SELECT * FROM planning_turns WHERE planning_session_id = ? AND request_key = ?;

-- name: GetPlanningReplyTurn :one
SELECT * FROM planning_turns WHERE planning_session_id = ? AND reply_to_turn_id = ?;

-- name: ListPlanningTurns :many
SELECT * FROM planning_turns WHERE planning_session_id = ? ORDER BY sequence;

-- name: AdvancePlanningSessionForOwnerTurn :execrows
UPDATE planning_sessions
SET revision = revision + 1, latest_turn_sequence = ?, waiting_on = 'provider',
    last_failure_code = '', last_failure_detail = '', updated_at = ?
WHERE id = ? AND revision = ? AND status = 'active' AND waiting_on = 'owner';

-- name: AdvancePlanningSessionForProviderTurn :execrows
UPDATE planning_sessions
SET revision = revision + 1, latest_turn_sequence = ?, waiting_on = ?,
    effective_provider = ?, effective_model = ?, native_conversation_ref = ?,
    last_failure_code = '', last_failure_detail = '', updated_at = ?
WHERE id = ? AND revision = ? AND status = 'active' AND waiting_on = 'provider';

-- name: SetPlanningSessionFailure :execrows
UPDATE planning_sessions
SET revision = revision + 1, waiting_on = 'owner', last_failure_code = ?,
    last_failure_detail = ?, updated_at = ?
WHERE id = ? AND revision = ? AND status = 'active' AND waiting_on = 'provider';

-- name: RecoverInterruptedPlanningSessions :execrows
UPDATE planning_sessions
SET revision = revision + 1, waiting_on = 'owner',
    last_failure_code = 'PLANNING_REPLY_AMBIGUOUS',
    last_failure_detail = 'The daemon restarted before the planning reply was recorded. The original request will not be replayed automatically; send a new message to retry.',
    updated_at = ?
WHERE status = 'active' AND waiting_on = 'provider';

-- name: ClosePlanningSession :execrows
UPDATE planning_sessions
SET revision = revision + 1, status = ?, waiting_on = 'none', updated_at = ?, closed_at = ?
WHERE id = ? AND revision = ? AND status = 'active';

-- name: LinkPlanningSessionPlan :execrows
UPDATE planning_sessions
SET revision = revision + 1, status = 'proposal_ready', waiting_on = 'none',
    proposed_plan_revision_id = ?, updated_at = ?, closed_at = ?
WHERE planning_sessions.id = ?
  AND planning_sessions.revision = ?
  AND planning_sessions.status = 'active'
  AND planning_sessions.waiting_on = 'owner'
  AND EXISTS (
      SELECT 1
      FROM outcomes o
      JOIN contract_revisions cr
        ON cr.id = planning_sessions.contract_revision_id
       AND cr.outcome_id = planning_sessions.outcome_id
       AND cr.number = planning_sessions.contract_revision_number
      WHERE o.id = planning_sessions.outcome_id
        AND o.current_revision_number = planning_sessions.contract_revision_number
  )
  AND EXISTS (
      SELECT 1 FROM plan_revisions p
      WHERE p.id = ? AND p.planning_session_id = planning_sessions.id
        AND p.source_intelligence_run_id = ?
  );

-- name: GetPlanRevisionByPlanningSession :one
SELECT id, outcome_id, number, contract_revision_number, status, summary,
       assumptions_json, blockers_json, run_brief_core_digest,
       run_brief_compiled_digest, created_at, routing_decisions_json,
       planning_session_id, source_intelligence_run_id
FROM plan_revisions WHERE outcome_id = ? AND planning_session_id = ? LIMIT 1;
