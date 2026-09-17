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
