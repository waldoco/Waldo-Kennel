-- Content-addressed per-attempt custody manifests. See migration 0159.

-- name: InsertAttemptManifest :exec
INSERT INTO attempt_manifests (attempt_id, half, outcome_id, payload, payload_digest, created_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: GetAttemptManifest :one
SELECT * FROM attempt_manifests WHERE attempt_id = ? AND half = ?;

-- name: ListAttemptManifestsForOutcome :many
SELECT * FROM attempt_manifests WHERE outcome_id = ? ORDER BY attempt_id, half;
