-- Shared adaptive intake and ResponsibilityLink lineage (#32).

-- name: CreateIntakeSession :exec
INSERT INTO intake_sessions
    (id, source_surface, purpose, project_id, source_open_loop_id, statement, status,
     current_proposal_revision, clarification_count, confirmed_outcome_id,
     failure_code, cancellation_reason, request_key, request_fingerprint, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: FindIntakeByRequestKey :one
SELECT * FROM intake_sessions WHERE request_key = ?;

-- name: GetIntakeSession :one
SELECT * FROM intake_sessions WHERE id = ?;

-- name: CreateIntakeConversationRef :exec
INSERT INTO intake_conversation_refs (intake_id, episode_id, turn_id, position)
VALUES (?, ?, ?, ?);

-- name: ListIntakeConversationRefs :many
SELECT intake_id, episode_id, turn_id, position
FROM intake_conversation_refs WHERE intake_id = ? ORDER BY position;

-- name: GetLatestIntakeProposal :one
SELECT * FROM intake_proposal_revisions WHERE intake_id = ? ORDER BY revision DESC LIMIT 1;

-- name: CreateIntakeProposal :exec
INSERT INTO intake_proposal_revisions
    (id, intake_id, revision, title, desired_state, criteria, review_method, constraints,
     non_goals, authority_ceiling, stop_conditions, clarification_notes, temporal_condition,
     facets, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: CreateIntakeClarification :exec
INSERT INTO intake_clarifications
    (id, intake_id, question, reason, recommendation, alternatives, deferral_consequence, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetIntakeClarification :one
SELECT c.id, c.intake_id, c.question, c.reason, c.recommendation, c.alternatives,
       c.deferral_consequence, c.created_at, a.answer, a.answered_at
FROM intake_clarifications c
LEFT JOIN intake_clarification_answers a ON a.clarification_id = c.id
WHERE c.intake_id = ?;

-- name: CreateIntakeClarificationAnswer :exec
INSERT INTO intake_clarification_answers (clarification_id, answer, answered_at)
VALUES (?, ?, ?);

-- name: UpdateIntakeAnalysisState :execrows
UPDATE intake_sessions SET status = ?, updated_at = ?
WHERE id = ? AND current_proposal_revision = ? AND status = ?;

-- name: FailIntakeAnalysis :execrows
UPDATE intake_sessions SET status = 'analysis_failed', failure_code = ?, updated_at = ?
WHERE id = ? AND current_proposal_revision = ? AND status = 'analyzing';

-- name: RecoverInterruptedIntakeAnalyses :execrows
-- An in-process analysis cannot outlive the daemon, so finding one still
-- `analyzing` at startup means it was interrupted and must become a durable
-- retryable failure.
--
-- An analysis an AGENT is working on is the opposite case: it is supposed to
-- outlive a restart, and reaping it would kill exactly the work this feature
-- exists to do. The open request row is the durable fact that tells the two
-- apart, so the sweep asks it rather than guessing from the status alone.
UPDATE intake_sessions
SET status = 'analysis_failed', failure_code = 'INTAKE_ANALYSIS_INTERRUPTED', updated_at = ?
WHERE status = 'analyzing'
  AND id NOT IN (
    SELECT intake_id FROM intake_analysis_requests WHERE status = 'requested'
  );

-- name: CancelIntake :execrows
UPDATE intake_sessions
SET status = 'cancelled', cancellation_reason = ?, failure_code = '', updated_at = ?
WHERE id = ? AND current_proposal_revision = ?
  AND status IN ('captured', 'needs_user', 'ready', 'analysis_failed');

-- name: UpdateIntakeWithProposal :execrows
UPDATE intake_sessions SET status = 'ready', current_proposal_revision = ?, failure_code = '', updated_at = ?
WHERE id = ? AND current_proposal_revision = ? AND status IN ('analyzing', 'ready');

-- name: UpdateIntakeWithClarification :execrows
UPDATE intake_sessions SET status = 'needs_user', clarification_count = clarification_count + 1, updated_at = ?
WHERE id = ? AND current_proposal_revision = ? AND status = 'analyzing' AND clarification_count = 0;

-- name: ConfirmIntake :execrows
UPDATE intake_sessions SET status = 'confirmed', confirmed_outcome_id = ?, updated_at = ?
WHERE id = ? AND current_proposal_revision = ? AND status = 'ready';

-- name: CreateIntakeConfirmation :exec
INSERT INTO intake_confirmations
    (intake_id, proposal_revision, outcome_id, contract_revision_id, request_key, request_fingerprint, confirmed_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: FindIntakeConfirmationByRequestKey :one
SELECT * FROM intake_confirmations WHERE request_key = ?;

-- name: GetIntakeConfirmation :one
SELECT * FROM intake_confirmations WHERE intake_id = ?;

-- name: CreateContractRevisionIntakeCore :exec
INSERT INTO contract_revision_intake_core
    (contract_revision_id, evidence_expectations, authority_ceiling, stop_conditions,
     temporal_condition, facets, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetContractRevisionIntakeCore :one
SELECT * FROM contract_revision_intake_core WHERE contract_revision_id = ?;

-- name: CreateResponsibilityLink :exec
INSERT INTO responsibility_links
    (id, project_id, source_open_loop_id, destination_outcome_id, creator, reason,
     request_key, request_fingerprint, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: FindResponsibilityLinkByRequestKey :one
SELECT * FROM responsibility_links WHERE request_key = ?;

-- name: GetResponsibilityLink :one
SELECT * FROM responsibility_links WHERE id = ?;

-- name: FindActiveResponsibilityLinkPair :one
SELECT * FROM responsibility_links
WHERE source_open_loop_id = ? AND destination_outcome_id = ? AND ended_at IS NULL;

-- name: EndResponsibilityLink :execrows
UPDATE responsibility_links
SET ended_at = ?, ended_by = ?, ended_reason = ?
WHERE id = ? AND ended_at IS NULL;

-- name: CreateIntakeAnalysisRequest :exec
INSERT INTO intake_analysis_requests
    (id, intake_id, expected_proposal_revision, status, callback_token_digest,
     session_id, harness, expires_at, raw_proposal, refusal_reason, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetIntakeAnalysisRequest :one
SELECT * FROM intake_analysis_requests WHERE id = ?;

-- name: LatestIntakeAnalysisRequest :one
SELECT * FROM intake_analysis_requests
WHERE intake_id = ?
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: ListOpenIntakeAnalysisRequests :many
SELECT * FROM intake_analysis_requests WHERE status = 'requested';

-- name: BindIntakeAnalysisRequestSession :execrows
UPDATE intake_analysis_requests
SET session_id = ?, harness = ?
WHERE id = ? AND status = 'requested';

-- name: AnswerIntakeAnalysisRequest :execrows
-- The status guard is what makes the callback single-use: a second answer
-- matches no open row and changes nothing, rather than overwriting the first.
UPDATE intake_analysis_requests
SET status = ?, raw_proposal = ?, refusal_reason = ?, answered_at = ?
WHERE id = ? AND status = 'requested';

-- name: MaxIntakeClarificationRoundOrdinal :one
SELECT CAST(COALESCE(MAX(ordinal), 0) AS INTEGER) FROM intake_clarification_rounds WHERE intake_id = ?;

-- name: ListIntakeClarificationRoundsPage :many
SELECT * FROM intake_clarification_rounds
WHERE intake_id = sqlc.arg(intake_id)
  AND ordinal > sqlc.arg(after_ordinal)
  AND ordinal <= sqlc.arg(high_water_ordinal)
ORDER BY ordinal
LIMIT sqlc.arg(page_limit);

-- name: GetIntakeClarificationRound :one
SELECT * FROM intake_clarification_rounds WHERE intake_id = ? AND id = ?;

-- name: GetLatestIntakeClarificationRound :one
SELECT * FROM intake_clarification_rounds WHERE intake_id = ? ORDER BY ordinal DESC LIMIT 1;

-- name: CreateIntakeClarificationRound :exec
INSERT INTO intake_clarification_rounds
    (id, intake_id, ordinal, version, expected_proposal_revision, explicit_reanalysis, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: CreateIntakeClarificationRoundQuestion :exec
INSERT INTO intake_clarification_round_questions
    (round_id, question_id, position, question, reason, recommendation, alternatives, deferral_consequence)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListIntakeClarificationRoundQuestions :many
SELECT * FROM intake_clarification_round_questions WHERE round_id = ? ORDER BY position;

-- name: CreateIntakeClarificationRoundAnswer :exec
INSERT INTO intake_clarification_round_answers (round_id, question_id, answer, answered_at)
VALUES (?, ?, ?, ?);

-- name: ListIntakeClarificationRoundAnswers :many
SELECT * FROM intake_clarification_round_answers WHERE round_id = ? ORDER BY question_id;

-- name: MaxIntakeClarificationAnswerRowID :one
SELECT CAST(COALESCE(MAX(a.answer_ordinal), 0) AS INTEGER)
FROM intake_clarification_round_answers a
JOIN intake_clarification_rounds r ON r.id = a.round_id
WHERE r.intake_id = ?;

-- name: ListIntakeClarificationRoundAnswersAtHighWater :many
SELECT a.round_id, a.question_id, a.answer, a.answered_at
FROM intake_clarification_round_answers a
WHERE a.round_id = sqlc.arg(round_id) AND a.answer_ordinal <= sqlc.arg(answer_high_water)
ORDER BY a.question_id;
