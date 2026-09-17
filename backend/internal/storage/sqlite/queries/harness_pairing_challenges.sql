-- name: InsertHarnessPairingChallenge :execrows
INSERT INTO harness_pairing_challenges (id,kind,connection_id,installation_id,adapter_digest,harness_identity,provider_version,protocol_fingerprint,mission_id,app_run_id,capability_classes,expected_generation,proof_verifier,status,result_code,connection_expires_at,expires_at,created_at,updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?);

-- name: GetHarnessPairingChallenge :one
SELECT * FROM harness_pairing_challenges WHERE id=?;

-- name: SupersedePendingHarnessPairingChallenges :execrows
UPDATE harness_pairing_challenges SET status='superseded', updated_at=sqlc.arg(updated_at)
WHERE connection_id=sqlc.arg(connection_id) AND status='pending';

-- name: ConsumeHarnessPairingChallenge :execrows
UPDATE harness_pairing_challenges SET status='consumed', updated_at=sqlc.arg(updated_at)
WHERE id=sqlc.arg(id) AND status='pending' AND expires_at > sqlc.arg(now);

-- name: RecordHarnessPairingResult :execrows
UPDATE harness_pairing_challenges SET result_code=sqlc.arg(result_code), updated_at=sqlc.arg(updated_at)
WHERE id=sqlc.arg(id) AND result_code IS NULL;

-- name: InsertHarnessPairingIntent :execrows
INSERT INTO harness_pairing_intents(id,project_id,kind,connection_id,installation_id,adapter_digest,harness_identity,provider_version,protocol_fingerprint,mission_id,app_run_id,capability_classes,expected_generation,connection_expires_at,expires_at,digest,status,proposal_request_key,proposal_request_fingerprint,created_at,updated_at)
SELECT ?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,? FROM projects p WHERE p.id=? AND p.archived_at IS NULL LIMIT 1;
-- name: GetHarnessPairingIntentByProposalRequest :one
SELECT * FROM harness_pairing_intents WHERE app_run_id=? AND proposal_request_key=?;
-- name: SupersedeLiveHarnessPairingIntents :execrows
UPDATE harness_pairing_intents SET status='superseded',updated_at=? WHERE connection_id=? AND expected_generation=? AND status IN ('requested','approved');
-- name: GetHarnessPairingIntent :one
SELECT * FROM harness_pairing_intents WHERE id=?;
-- name: ListHarnessPairingIntents :many
SELECT * FROM harness_pairing_intents WHERE (?='' OR project_id=?) ORDER BY updated_at DESC,id LIMIT ?;
-- name: DecideHarnessPairingIntent :execrows
UPDATE harness_pairing_intents SET status=?,decision_id=?,decision=?,decision_request_key=?,owner_principal=?,confirmation_ref=?,decided_at=?,updated_at=? WHERE id=? AND digest=? AND status='requested' AND expires_at>?;
-- name: BeginHarnessPairingIntentActivation :execrows
UPDATE harness_pairing_intents SET status='activating',updated_at=? WHERE id=? AND digest=? AND status='approved' AND expires_at>?;
-- name: CompleteHarnessPairingIntentActivation :execrows
UPDATE harness_pairing_intents SET status='challenge_active',challenge_id=?,updated_at=? WHERE id=? AND status='activating';
-- name: FailHarnessPairingIntentActivation :execrows
UPDATE harness_pairing_intents SET status='activation_failed',updated_at=? WHERE id=? AND status='activating';
-- name: InsertHarnessAuthorityReceipt :execrows
INSERT INTO harness_authority_receipts(id,action,target_type,target_id,target_digest,expected_generation,request_key,request_fingerprint,owner_principal,confirmation_ref,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT DO NOTHING;
-- name: GetHarnessAuthorityReceiptByRequest :one
SELECT * FROM harness_authority_receipts WHERE owner_principal=? AND request_key=?;
-- name: ListHarnessConnections :many
SELECT * FROM harness_connections WHERE (?='' OR mission_id=?) ORDER BY updated_at DESC,id LIMIT ?;
-- name: ListHarnessAuthorityReceiptsForTarget :many
SELECT * FROM harness_authority_receipts WHERE target_type=? AND target_id=? ORDER BY created_at DESC,id;
-- name: CountHarnessCommandConsequences :one
SELECT count(*) FROM command_authority_claims WHERE harness_connection_id=? AND connection_generation=? AND state='action_needed';
-- name: MarkHarnessCommandClaimsActionNeeded :execrows
UPDATE command_authority_claims SET state='action_needed',connection_revoked_at=?,updated_at=? WHERE harness_connection_id=? AND connection_generation=? AND state='pending';
-- name: MarkHarnessCommandOutboxActionNeededForConnection :execrows
UPDATE harness_command_outbox SET state='action_needed',updated_at=? WHERE claim_id IN (SELECT id FROM command_authority_claims WHERE harness_connection_id=? AND connection_generation=?) AND state='pending';
-- name: GetApprovedHarnessPairingIntentForActivation :one
SELECT * FROM harness_pairing_intents WHERE id=? AND digest=? AND status='approved' AND expires_at>?;
