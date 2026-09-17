-- name: GetCapabilityEscalation :one
SELECT * FROM capability_escalations WHERE digest=?;
-- name: GetLiveCapabilityEscalation :one
SELECT * FROM capability_escalations WHERE attempt_id=? AND attempt_generation=? AND requested_capability=? AND operation_id=? AND superseded_at IS NULL;
-- name: InsertCapabilityEscalation :execrows
INSERT INTO capability_escalations(digest,question_id,version,outcome_id,contract_revision_number,plan_revision_id,work_unit_id,attempt_id,attempt_generation,session_id,session_generation,executor_kind,attempt_session_ref_id,runtime_launch_id,controller_generation,policy_digest,artifact_version,check_id,requested_capability,denial_source,grant_fingerprint,operation_id,request_fingerprint,question_generation,within_contract_ceiling,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(digest) DO NOTHING;
-- name: SupersedeCapabilityEscalation :execrows
UPDATE capability_escalations SET superseded_at=? WHERE digest=? AND superseded_at IS NULL;
-- name: InsertCapabilityEscalationReceipt :execrows
INSERT INTO capability_escalation_receipts(id,escalation_digest,question_id,question_generation,consequence,attempt_id,attempt_generation,session_id,session_generation,controller_generation,capability,operation_id,request_fingerprint,grant_fingerprint,answer_request_key,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(question_id,question_generation) DO NOTHING;
-- name: GetCapabilityEscalationReceipt :one
SELECT * FROM capability_escalation_receipts WHERE question_id=? AND question_generation=?;
-- name: ConsumeCapabilityEscalationReceipt :execrows
UPDATE capability_escalation_receipts SET consumed_at=? WHERE id=? AND consequence='grant_once' AND consumed_at IS NULL;
-- name: GetCapabilityEscalationByQuestion :one
SELECT * FROM capability_escalations WHERE question_id=?;
-- name: GetOwnerQuestionConversation :one
SELECT conversation_id FROM owner_answer_questions WHERE id=?;
-- name: FailCapabilityEscalationQuestion :execrows
UPDATE owner_answer_questions SET status='failed',updated_at=? WHERE id=? AND status='pending';
-- name: CancelCapabilityEscalationActivity :execrows
UPDATE conversation_activities SET status='cancelled',updated_at=? WHERE id=? AND status='pending';
-- name: ResolveCapabilityEscalationActivity :execrows
UPDATE conversation_activities SET status='resolved',updated_at=? WHERE id=? AND status='pending';
