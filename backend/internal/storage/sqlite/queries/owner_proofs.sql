-- name: InsertOwnerProof :execrows
INSERT INTO owner_proofs(id,verifier,app_run_id,mission_id,content_digest,target_id,target_generation,command_class,confirmation_ref,expires_at,created_at,consumed_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT DO NOTHING;
-- name: GetOwnerProof :one
SELECT * FROM owner_proofs WHERE id=?;
-- name: ConsumeOwnerProof :execrows
UPDATE owner_proofs SET consumed_at=sqlc.arg(consumed_at) WHERE id=sqlc.arg(id) AND consumed_at IS NULL AND expires_at>sqlc.arg(consumed_at);
