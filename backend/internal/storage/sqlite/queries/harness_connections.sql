-- name: InsertHarnessConnection :execrows
INSERT INTO harness_connections (id,installation_id,adapter_digest,harness_identity,provider_version,protocol_fingerprint,mission_id,app_run_id,capability_classes,capability_verifier,generation,expires_at,revoked_at,created_at,updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT DO NOTHING;

-- name: GetHarnessConnection :one
SELECT * FROM harness_connections WHERE id=?;

-- name: RotateHarnessConnection :execrows
UPDATE harness_connections SET capability_verifier=sqlc.arg(capability_verifier), generation=generation+1,
 expires_at=sqlc.arg(expires_at), revoked_at=NULL, updated_at=sqlc.arg(updated_at)
WHERE id=sqlc.arg(id) AND generation=sqlc.arg(expected_generation) AND revoked_at IS NULL;

-- name: RevokeHarnessConnection :execrows
UPDATE harness_connections SET revoked_at=sqlc.arg(revoked_at), updated_at=sqlc.arg(updated_at)
WHERE id=sqlc.arg(id) AND generation=sqlc.arg(expected_generation) AND revoked_at IS NULL;
