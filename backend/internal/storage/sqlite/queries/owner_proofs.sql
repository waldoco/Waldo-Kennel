-- name: InsertOwnerProof :execrows
INSERT INTO owner_proofs(id,verifier,app_run_id,mission_id,content_digest,target_digest,command_class,confirmation_ref,expires_at,created_at,consumed_at) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT DO NOTHING;
-- name: GetOwnerProof :one
SELECT * FROM owner_proofs WHERE id=?;
-- name: ConsumeOwnerProof :execrows
UPDATE owner_proofs SET consumed_at=sqlc.arg(consumed_at) WHERE id=sqlc.arg(id) AND consumed_at IS NULL AND expires_at>sqlc.arg(consumed_at);
-- name: InsertOwnerAnswerQuestion :execrows
INSERT INTO owner_answer_questions(id,conversation_id,request_id,generation,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO NOTHING;
-- name: ResolveOwnerAnswerQuestion :execrows
UPDATE owner_answer_questions SET status='resolved',updated_at=? WHERE conversation_id=? AND request_id=? AND status='pending';
-- name: FailOwnerAnswerQuestions :execrows
UPDATE owner_answer_questions SET status='failed',updated_at=? WHERE conversation_id=? AND status='pending';
-- name: GetOwnerAnswerQuestion :one
SELECT * FROM owner_answer_questions WHERE id=?;
-- name: GetCommandHarnessConnection :one
SELECT * FROM harness_connections WHERE id=?;
-- name: GetCommandOwnerProof :one
SELECT * FROM owner_proofs WHERE id=?;
-- name: GetCommandSessionTarget :one
SELECT session_id AS id,controller_generation,expected_revision,capability_fingerprint FROM chat_command_targets WHERE session_id=?;
-- name: GetCommandActiveTurnTarget :one
SELECT provider_turn_id,state FROM conversation_turns WHERE handled_by_session_id=? AND provider_turn_id=? ORDER BY requested_at DESC LIMIT 1;
-- name: GetCommandAnswerQuestionTarget :one
SELECT * FROM owner_answer_questions WHERE id=?;
-- name: InsertCommandAuthorityClaim :execrows
INSERT INTO command_authority_claims(id,adapter_request_key,request_fingerprint,owner_proof_id,harness_connection_id,connection_generation,connection_binding_digest,connection_expires_at,connection_revoked_at,transport_class,app_run_id,mission_id,content_digest,target_digest,owner_class,canonical_version,canonical_payload,destination_type,destination_id,state,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT DO NOTHING;
-- name: GetCommandAuthorityClaimByRequest :one
SELECT * FROM command_authority_claims WHERE harness_connection_id=? AND connection_generation=? AND adapter_request_key=?;
-- name: InsertHarnessCommandOutbox :execrows
INSERT INTO harness_command_outbox(claim_id,destination_type,destination_id,canonical_payload,state,created_at,updated_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT DO NOTHING;

-- name: ListPendingHarnessCommandOutbox :many
SELECT * FROM harness_command_outbox WHERE state IN ('pending','action_needed') ORDER BY created_at,claim_id;

-- name: ListCurrentNeedsYouQuestionRows :many
SELECT q.id,q.conversation_id,q.request_id,q.generation,q.status AS question_status,q.created_at,q.updated_at,
       ca.kind,ca.summary,ca.detail_json,ca.status AS activity_status,
       a.outcome_id,a.plan_revision_id,a.work_unit_id,a.id AS attempt_id,a.status AS attempt_status,
       ats.session_id, g.id AS command_id,g.state AS command_state
FROM owner_answer_questions q
JOIN conversation_activities ca ON ca.id=q.id AND ca.conversation_id=q.conversation_id
JOIN conversations c ON c.id=q.conversation_id
JOIN attempt_sessions ats ON ats.session_id=c.current_session_id
JOIN attempts a ON a.id=ats.attempt_id
LEFT JOIN governed_control_commands g ON g.session_id=ats.session_id AND g.idempotency_key=('answer:' || q.generation)
WHERE a.outcome_id=?
  AND ats.seq=(SELECT MAX(x.seq) FROM attempt_sessions x WHERE x.attempt_id=a.id)
  AND a.id=(SELECT x.id FROM attempts x WHERE x.outcome_id=a.outcome_id AND x.work_unit_id=a.work_unit_id ORDER BY x.number DESC LIMIT 1)
  AND a.plan_revision_id=(SELECT p.id FROM plan_revisions p WHERE p.outcome_id=a.outcome_id ORDER BY p.number DESC LIMIT 1)
  AND q.id=(SELECT q2.id FROM owner_answer_questions q2 WHERE q2.conversation_id=q.conversation_id AND q2.request_id=q.request_id ORDER BY q2.created_at DESC,q2.id DESC LIMIT 1)
ORDER BY a.number DESC,ca.sequence DESC;

-- name: GetNeedsYouQuestionRow :one
SELECT q.id,q.conversation_id,q.request_id,q.generation,q.status AS question_status,q.created_at,q.updated_at,
       ca.kind,ca.summary,ca.detail_json,ca.status AS activity_status,
       a.outcome_id,a.plan_revision_id,a.work_unit_id,a.id AS attempt_id,a.status AS attempt_status,
       ats.session_id, g.id AS command_id,g.state AS command_state
FROM owner_answer_questions q
JOIN conversation_activities ca ON ca.id=q.id AND ca.conversation_id=q.conversation_id
JOIN conversations c ON c.id=q.conversation_id
JOIN attempt_sessions ats ON ats.session_id=c.current_session_id
JOIN attempts a ON a.id=ats.attempt_id
LEFT JOIN governed_control_commands g ON g.session_id=ats.session_id AND g.idempotency_key=('answer:' || q.generation)
WHERE a.outcome_id=? AND q.id=?
  AND ats.seq=(SELECT MAX(x.seq) FROM attempt_sessions x WHERE x.attempt_id=a.id)
  AND a.id=(SELECT x.id FROM attempts x WHERE x.outcome_id=a.outcome_id AND x.work_unit_id=a.work_unit_id ORDER BY x.number DESC LIMIT 1)
  AND a.plan_revision_id=(SELECT p.id FROM plan_revisions p WHERE p.outcome_id=a.outcome_id ORDER BY p.number DESC LIMIT 1)
LIMIT 1;
