-- Canonical Work responsibility contracts (#21): ResponsibilitySpace ->
-- Outcome -> immutable ContractRevision.

-- name: GetResponsibilitySpace :one
SELECT id, kind, project_id, created_at
FROM responsibility_spaces WHERE id = ?;

-- name: FindWorkResponsibilitySpaceByProject :one
SELECT id, kind, project_id, created_at
FROM responsibility_spaces WHERE project_id = ? AND kind = 'WorkProject';

-- name: CreateResponsibilitySpace :exec
INSERT INTO responsibility_spaces (id, kind, project_id)
VALUES (?, 'WorkProject', ?);

-- name: CreateOutcome :exec
INSERT INTO outcomes (id, space_id, title, current_revision_number, idempotency_key, parent_outcome_id)
VALUES (?, ?, ?, ?, ?, ?);

-- name: FindOutcomeByIdempotencyKey :one
SELECT id, space_id, title, current_revision_number, idempotency_key, created_at, updated_at, parent_outcome_id
FROM outcomes WHERE idempotency_key = ?;

-- name: GetOutcome :one
SELECT id, space_id, title, current_revision_number, idempotency_key, created_at, updated_at, parent_outcome_id
FROM outcomes WHERE id = ? AND NOT EXISTS (SELECT 1 FROM outcome_trash WHERE outcome_id=outcomes.id);

-- name: ListOutcomesByProject :many
SELECT o.id, o.space_id, o.title, o.current_revision_number, o.idempotency_key, o.created_at, o.updated_at, o.parent_outcome_id
FROM outcomes o
JOIN responsibility_spaces rs ON rs.id = o.space_id
WHERE rs.project_id = ? AND rs.kind = 'WorkProject' AND NOT EXISTS (SELECT 1 FROM outcome_trash WHERE outcome_id=o.id)
ORDER BY o.created_at, o.id;

-- name: AdvanceOutcomeCurrentRevision :execrows
UPDATE outcomes
SET current_revision_number = ?, updated_at = ?
WHERE id = ? AND current_revision_number = ?;

-- Contract revisions are append-only and trigger-guarded against UPDATE, so
-- every immutable field, the execution preference included, has to be written
-- here rather than populated by a follow-up write in the same transaction.
-- name: CreateContractRevision :exec
INSERT INTO contract_revisions (id, outcome_id, number, goal, success_criteria, review, constraints, non_goals, clarification, execution_preference_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- Stable criterion identities added by Work E (#35). The JSON text column on
-- contract_revisions remains a compatibility projection only.
-- name: CreateContractCriterion :exec
INSERT INTO contract_criteria (id, contract_revision_id, position, text)
VALUES (?, ?, ?, ?);

-- name: ListContractCriteriaForRevision :many
SELECT id, contract_revision_id, position, text
FROM contract_criteria WHERE contract_revision_id = ? ORDER BY position;

-- name: GetContractRevisionByNumber :one
SELECT id, outcome_id, number, goal, success_criteria, review, constraints, non_goals, clarification, created_at, execution_preference_json
FROM contract_revisions WHERE outcome_id = ? AND number = ?;

-- name: GetContractRevision :one
SELECT id, outcome_id, number, goal, success_criteria, review, constraints, non_goals, clarification, created_at, execution_preference_json
FROM contract_revisions WHERE id = ?;

-- name: ListContractRevisions :many
SELECT id, outcome_id, number, goal, success_criteria, review, constraints, non_goals, clarification, created_at, execution_preference_json
FROM contract_revisions WHERE outcome_id = ? ORDER BY number;

-- name: MaxContractRevisionNumber :one
SELECT COALESCE(MAX(number), 0) FROM contract_revisions WHERE outcome_id = ?;

-- Canonical Decide & Authorize (#26): Outcome -> immutable PlanRevision with
-- exactly one direct WorkUnit and scoped CapabilityGrants.

-- name: CreatePlanRevision :exec
INSERT INTO plan_revisions (id, outcome_id, number, contract_revision_number, status, summary, assumptions_json, blockers_json, run_brief_core_digest, run_brief_compiled_digest)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '');

-- name: ApprovePlanRevision :execrows
UPDATE plan_revisions SET status = 'approved'
WHERE id = ? AND outcome_id = ? AND status = 'proposed';

-- name: MaxPlanRevisionNumber :one
SELECT COALESCE(MAX(number), 0) FROM plan_revisions WHERE outcome_id = ?;

-- name: LatestProposedPlanRevision :one
SELECT id, outcome_id, number, contract_revision_number, status, summary, assumptions_json, blockers_json, run_brief_core_digest, run_brief_compiled_digest, created_at, planning_session_id, source_intelligence_run_id, routing_decisions_json
FROM plan_revisions WHERE outcome_id = ? AND contract_revision_number = ? AND status = 'proposed'
ORDER BY number DESC LIMIT 1;

-- name: GetPlanRevision :one
SELECT id, outcome_id, number, contract_revision_number, status, summary, assumptions_json, blockers_json, run_brief_core_digest, run_brief_compiled_digest, created_at, planning_session_id, source_intelligence_run_id, routing_decisions_json
FROM plan_revisions WHERE id = ? AND outcome_id = ?;

-- name: GetLatestPlanRevision :one
SELECT id, outcome_id, number, contract_revision_number, status, summary, assumptions_json, blockers_json, run_brief_core_digest, run_brief_compiled_digest, created_at, planning_session_id, source_intelligence_run_id, routing_decisions_json
FROM plan_revisions WHERE outcome_id = ? ORDER BY number DESC LIMIT 1;

-- name: CreateWorkUnit :exec
INSERT INTO work_units (id, plan_revision_id, kind, title, contract_revision_number, output_summary, evidence_checks, verification_requirement, stop_conditions)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListWorkUnitsForPlan :many
SELECT * FROM work_units WHERE plan_revision_id = ?;

-- Approved deterministic checks are frozen Plan authority; there is
-- deliberately no update or delete query.

-- name: CreateWorkUnitCheck :exec
INSERT INTO work_unit_checks (id, work_unit_id, criterion_id, position, argv, timeout_seconds)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ListWorkUnitChecksForWorkUnit :many
SELECT id, work_unit_id, criterion_id, position, argv, timeout_seconds
FROM work_unit_checks WHERE work_unit_id = ? ORDER BY position;

-- name: CreateCapabilityGrant :exec
INSERT INTO capability_grants (id, plan_revision_id, name, scope)
VALUES (?, ?, ?, ?);

-- name: ListCapabilityGrantsForPlan :many
SELECT id, plan_revision_id, name, scope
FROM capability_grants WHERE plan_revision_id = ?;

-- Durable check-run identity. The reservation is inserted before the command
-- is invoked; the observation is written once and never changed.

-- name: ReserveAttemptCheckRun :exec
INSERT INTO attempt_check_runs (id, attempt_id, check_id, artifact_version, state, reservation_epoch, reserved_at)
VALUES (?, ?, ?, ?, 'reserved', ?, ?);

-- name: GetAttemptCheckRun :one
SELECT id, attempt_id, check_id, artifact_version, state, reservation_epoch, ran, passed, exit_code, enforced_by,
       timed_out, cancelled, termination_unknown, output_truncated, output, unavailable,
       baseline_ran, baseline_passed, baseline_detail,
       artifact_changed, observed_artifact_version, reserved_at, observed_at
FROM attempt_check_runs WHERE attempt_id = ? AND check_id = ? AND artifact_version = ?;

-- name: ListAttemptCheckRuns :many
SELECT id, attempt_id, check_id, artifact_version, state, reservation_epoch, ran, passed, exit_code, enforced_by,
       timed_out, cancelled, termination_unknown, output_truncated, output, unavailable,
       baseline_ran, baseline_passed, baseline_detail,
       artifact_changed, observed_artifact_version, reserved_at, observed_at
FROM attempt_check_runs WHERE attempt_id = ? AND artifact_version = ? ORDER BY reserved_at, id;

-- name: RecordAttemptCheckObservation :execrows
UPDATE attempt_check_runs
SET state = 'observed', ran = ?, passed = ?, exit_code = ?, enforced_by = ?,
    timed_out = ?, cancelled = ?, termination_unknown = ?, output_truncated = ?,
    output = ?, unavailable = ?,
    baseline_ran = ?, baseline_passed = ?, baseline_detail = ?,
    artifact_changed = ?, observed_artifact_version = ?,
    observed_at = ?
WHERE attempt_id = ? AND check_id = ? AND artifact_version = ? AND state = 'reserved';

-- name: MarkAttemptCheckRunUnknown :execrows
UPDATE attempt_check_runs SET state = 'unknown', observed_at = ?
WHERE attempt_id = ? AND check_id = ? AND artifact_version = ? AND state = 'reserved';

-- Durable run intent. Generations are append-only; the only permitted
-- mutation is the write-once acknowledgement.

-- name: CreateOutcomeRunIntent :exec
INSERT INTO outcome_run_intents
    (id, outcome_id, generation, desired, plan_revision_id, contract_revision_number, request_key, request_fingerprint, requested_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: CurrentOutcomeRunIntent :one
SELECT id, outcome_id, generation, desired, plan_revision_id, contract_revision_number, request_key, request_fingerprint, requested_at, acknowledged_at,
       admission_failure_code, admission_failure_message, admission_failure_detail, admission_failure_work_unit_id, admission_failed_at
FROM outcome_run_intents WHERE outcome_id = ? ORDER BY generation DESC LIMIT 1;

-- name: FindOutcomeRunIntentByRequestKey :one
SELECT id, outcome_id, generation, desired, plan_revision_id, contract_revision_number, request_key, request_fingerprint, requested_at, acknowledged_at,
       admission_failure_code, admission_failure_message, admission_failure_detail, admission_failure_work_unit_id, admission_failed_at
FROM outcome_run_intents WHERE request_key = ?;

-- name: ListOutcomeRunIntents :many
SELECT id, outcome_id, generation, desired, plan_revision_id, contract_revision_number, request_key, request_fingerprint, requested_at, acknowledged_at,
       admission_failure_code, admission_failure_message, admission_failure_detail, admission_failure_work_unit_id, admission_failed_at
FROM outcome_run_intents WHERE outcome_id = ? ORDER BY generation;

-- name: ListCurrentRunIntentsByDesired :many
SELECT i.id, i.outcome_id, i.generation, i.desired, i.plan_revision_id, i.contract_revision_number, i.request_key, i.request_fingerprint, i.requested_at, i.acknowledged_at,
       i.admission_failure_code, i.admission_failure_message, i.admission_failure_detail, i.admission_failure_work_unit_id, i.admission_failed_at
FROM outcome_run_intents i
WHERE i.desired = ? AND NOT EXISTS (SELECT 1 FROM outcome_trash WHERE outcome_id=i.outcome_id)
  AND i.generation = (SELECT MAX(g.generation) FROM outcome_run_intents g WHERE g.outcome_id = i.outcome_id)
ORDER BY i.outcome_id;

-- name: AcknowledgeOutcomeRunIntent :execrows
UPDATE outcome_run_intents SET acknowledged_at = ?
WHERE outcome_id = ? AND generation = ? AND acknowledged_at IS NULL;

-- name: RecordOutcomeRunAdmissionFailure :execrows
UPDATE outcome_run_intents
SET admission_failure_code = ?, admission_failure_message = ?, admission_failure_detail = ?,
    admission_failure_work_unit_id = ?, admission_failed_at = ?
WHERE outcome_run_intents.outcome_id = ? AND outcome_run_intents.generation = ? AND outcome_run_intents.desired = 'running'
  AND outcome_run_intents.admission_failure_code = ''
  AND outcome_run_intents.generation = (SELECT MAX(current.generation) FROM outcome_run_intents AS current WHERE current.outcome_id = ?);

-- Supplied-document context. Revisions are append-only; the only permitted
-- mutation is the write-once, one-way approval.

-- name: CreateOutcomeDocumentContext :exec
INSERT INTO outcome_document_contexts (id, outcome_id, revision, digest, state, selected_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: CreateOutcomeDocumentSource :exec
INSERT INTO outcome_document_sources (id, context_id, position, source_path, name, content_digest, size_bytes)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: CurrentOutcomeDocumentContext :one
SELECT id, outcome_id, revision, digest, state, selected_at, approved_at
FROM outcome_document_contexts WHERE outcome_id = ? ORDER BY revision DESC LIMIT 1;

-- name: GetOutcomeDocumentContext :one
SELECT id, outcome_id, revision, digest, state, selected_at, approved_at
FROM outcome_document_contexts WHERE id = ?;

-- name: MaxOutcomeDocumentContextRevision :one
SELECT CAST(COALESCE(MAX(revision), 0) AS INTEGER) FROM outcome_document_contexts WHERE outcome_id = ?;

-- name: ListOutcomeDocumentSources :many
SELECT id, context_id, position, source_path, name, content_digest, size_bytes
FROM outcome_document_sources WHERE context_id = ? ORDER BY position;

-- name: ApproveOutcomeDocumentContext :execrows
UPDATE outcome_document_contexts SET state = 'approved', approved_at = ?
WHERE outcome_document_contexts.id = ? AND outcome_document_contexts.outcome_id = ?
  AND outcome_document_contexts.digest = ? AND outcome_document_contexts.state = 'selected'
  AND outcome_document_contexts.revision = (SELECT MAX(current.revision) FROM outcome_document_contexts AS current WHERE current.outcome_id = ?);

-- Composed Outcomes (ADR 0007). Contribution is criterion-bound and
-- append-only; there is deliberately no update or delete query.

-- name: ListContributingOutcomes :many
SELECT id, space_id, title, current_revision_number, idempotency_key, created_at, updated_at, parent_outcome_id
FROM outcomes WHERE parent_outcome_id = ?
ORDER BY created_at, id;

-- name: CountContributingOutcomes :one
SELECT COUNT(*) FROM outcomes WHERE parent_outcome_id = ?;

-- name: CreateContributionLink :exec
INSERT INTO contribution_links (id, parent_outcome_id, child_outcome_id, parent_contract_revision_id, parent_criterion_id)
VALUES (?, ?, ?, ?, ?);

-- name: ListContributionLinksForParent :many
SELECT id, parent_outcome_id, child_outcome_id, parent_contract_revision_id, parent_criterion_id, created_at
FROM contribution_links WHERE parent_outcome_id = ?
ORDER BY created_at, id;

-- name: ListContributionLinksForChild :many
SELECT id, parent_outcome_id, child_outcome_id, parent_contract_revision_id, parent_criterion_id, created_at
FROM contribution_links WHERE child_outcome_id = ?
ORDER BY created_at, id;

-- Decomposition authority (ADR 0007 phase 2). Proposals are append-only; the
-- only permitted mutation is the one-way move to authorized.

-- name: CreateDecompositionRevision :exec
INSERT INTO decomposition_revisions (id, outcome_id, number, contract_revision_id, status, rationale, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: MaxDecompositionRevisionNumber :one
SELECT CAST(COALESCE(MAX(number), 0) AS INTEGER) FROM decomposition_revisions WHERE outcome_id = ?;

-- name: GetDecompositionRevision :one
SELECT id, outcome_id, number, contract_revision_id, status, rationale, created_at, authorized_at
FROM decomposition_revisions WHERE id = ? AND outcome_id = ?;

-- name: LatestDecompositionRevision :one
SELECT id, outcome_id, number, contract_revision_id, status, rationale, created_at, authorized_at
FROM decomposition_revisions WHERE outcome_id = ?
ORDER BY number DESC LIMIT 1;

-- name: AuthorizeDecompositionRevision :execrows
UPDATE decomposition_revisions
SET status = 'authorized', authorized_at = ?
WHERE id = ? AND outcome_id = ? AND status = 'proposed';

-- name: CreateDecompositionContribution :exec
INSERT INTO decomposition_contributions
    (id, decomposition_id, ref, position, title, goal, success_criteria, review, constraints, non_goals, authority, claimed_criteria)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListDecompositionContributions :many
SELECT id, decomposition_id, ref, position, title, goal, success_criteria, review, constraints, non_goals, authority, claimed_criteria, child_outcome_id
FROM decomposition_contributions WHERE decomposition_id = ?
ORDER BY position, ref;

-- name: BindDecompositionContributionOutcome :execrows
UPDATE decomposition_contributions
SET child_outcome_id = ?
WHERE id = ? AND child_outcome_id IS NULL;

-- name: CreateDecompositionRetainedCriterion :exec
INSERT INTO decomposition_retained_criteria (id, decomposition_id, parent_criterion_id)
VALUES (?, ?, ?);

-- name: ListDecompositionRetainedCriteria :many
SELECT id, decomposition_id, parent_criterion_id
FROM decomposition_retained_criteria WHERE decomposition_id = ?
ORDER BY parent_criterion_id;

-- name: CreateContributionDependency :exec
INSERT INTO contribution_dependencies (id, decomposition_id, from_ref, to_ref)
VALUES (?, ?, ?, ?);

-- name: ListContributionDependencies :many
SELECT id, decomposition_id, from_ref, to_ref
FROM contribution_dependencies WHERE decomposition_id = ?
ORDER BY from_ref, to_ref;

-- name: CreateContributionDependencyWaiver :exec
INSERT INTO contribution_dependency_waivers (id, decomposition_id, from_ref, to_ref, reason, waived_by, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListContributionDependencyWaivers :many
SELECT id, decomposition_id, from_ref, to_ref, reason, waived_by, created_at
FROM contribution_dependency_waivers WHERE decomposition_id = ?
ORDER BY created_at, id;

-- name: CreateDecompositionRequest :exec
INSERT INTO decomposition_requests (id, outcome_id, contract_revision_id, status, callback_token_digest, session_id, expires_at, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetDecompositionRequest :one
SELECT id, outcome_id, contract_revision_id, status, callback_token_digest, session_id, expires_at, raw_proposal, refusal_reason, decomposition_id, created_at, answered_at
FROM decomposition_requests WHERE id = ?;

-- name: LatestDecompositionRequest :one
SELECT id, outcome_id, contract_revision_id, status, callback_token_digest, session_id, expires_at, raw_proposal, refusal_reason, decomposition_id, created_at, answered_at
FROM decomposition_requests WHERE outcome_id = ?
ORDER BY created_at DESC, id DESC LIMIT 1;

-- name: AnswerDecompositionRequest :execrows
UPDATE decomposition_requests
SET status = ?, raw_proposal = ?, refusal_reason = ?, decomposition_id = ?, answered_at = ?
WHERE id = ? AND status = 'requested';

-- name: ListOpenDecompositionRequests :many
SELECT id, outcome_id, contract_revision_id, status, callback_token_digest, session_id, expires_at, raw_proposal, refusal_reason, decomposition_id, created_at, answered_at
FROM decomposition_requests WHERE status = 'requested'
ORDER BY expires_at;

-- name: BindDecompositionRequestSession :execrows
UPDATE decomposition_requests
SET session_id = ?
WHERE id = ? AND status = 'requested' AND session_id = '';
