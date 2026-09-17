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
INSERT INTO command_authority_claims(id,adapter_request_key,request_fingerprint,owner_proof_id,harness_connection_id,connection_generation,connection_binding_digest,transport_class,app_run_id,mission_id,content_digest,target_digest,owner_class,canonical_version,canonical_payload,destination_type,destination_id,state,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT DO NOTHING;
-- name: GetCommandAuthorityClaimByRequest :one
SELECT * FROM command_authority_claims WHERE harness_connection_id=? AND connection_generation=? AND adapter_request_key=?;
-- name: InsertHarnessCommandOutbox :execrows
INSERT INTO harness_command_outbox(claim_id,destination_type,destination_id,canonical_payload,state,created_at,updated_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT DO NOTHING;
