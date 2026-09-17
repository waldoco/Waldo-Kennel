-- name: InsertHarnessConnection :execrows
INSERT INTO harness_connections (id,installation_id,adapter_digest,harness_identity,provider_version,protocol_fingerprint,mission_id,app_run_id,capability_classes,capability_verifier,generation,expires_at,revoked_at,created_at,updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT DO NOTHING;

-- name: GetHarnessConnection :one
SELECT * FROM harness_connections WHERE id=?;

-- name: RotateHarnessConnection :execrows
UPDATE harness_connections SET capability_verifier=sqlc.arg(capability_verifier), app_run_id=sqlc.arg(app_run_id), generation=generation+1,
 expires_at=sqlc.arg(expires_at), revoked_at=NULL, updated_at=sqlc.arg(updated_at)
WHERE id=sqlc.arg(id) AND generation=sqlc.arg(expected_generation) AND revoked_at IS NULL;

-- name: RevokeHarnessConnection :execrows
UPDATE harness_connections SET revoked_at=sqlc.arg(revoked_at), updated_at=sqlc.arg(updated_at)
WHERE id=sqlc.arg(id) AND generation=sqlc.arg(expected_generation) AND revoked_at IS NULL;

-- name: ArchiveHarnessConnectionGeneration :execrows
INSERT INTO harness_connection_generations(connection_id,generation,installation_id,adapter_digest,harness_identity,provider_version,protocol_fingerprint,mission_id,app_run_id,capability_classes,capability_verifier,expires_at,revoked_at,created_at,updated_at)
SELECT h.id,h.generation,h.installation_id,h.adapter_digest,h.harness_identity,h.provider_version,h.protocol_fingerprint,h.mission_id,h.app_run_id,h.capability_classes,h.capability_verifier,h.expires_at,h.revoked_at,h.created_at,h.updated_at FROM harness_connections h WHERE h.id=sqlc.arg(id) AND h.generation=sqlc.arg(expected_generation) AND h.revoked_at IS NULL;
