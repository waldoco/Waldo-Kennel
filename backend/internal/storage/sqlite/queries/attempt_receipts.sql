-- Durable record of what one Attempt produced. See migration 0119.

-- name: UpsertAttemptReceipt :exec
INSERT INTO attempt_receipts (
    attempt_id, outcome_id, plan_revision_id, work_unit_id, contract_revision_number,
    artifact_version, workspace_kind, workspace_path,
    repository_path, repository_identity,
    base_revision, result_revision, workspace_dirty,
    retention_state, retention_detail, termination_reason,
    observed_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(attempt_id) DO UPDATE SET
    artifact_version = excluded.artifact_version,
    workspace_kind = excluded.workspace_kind,
    workspace_path = excluded.workspace_path,
    repository_path = excluded.repository_path,
    repository_identity = excluded.repository_identity,
    base_revision = excluded.base_revision,
    result_revision = excluded.result_revision,
    workspace_dirty = excluded.workspace_dirty,
    retention_state = excluded.retention_state,
    retention_detail = excluded.retention_detail,
    termination_reason = excluded.termination_reason,
    observed_at = excluded.observed_at,
    updated_at = excluded.updated_at
WHERE attempt_receipts.frozen_at IS NULL;

-- name: GetAttemptReceipt :one
SELECT * FROM attempt_receipts WHERE attempt_id = ?;

-- name: ListAttemptReceiptsForOutcome :many
SELECT * FROM attempt_receipts WHERE outcome_id = ? ORDER BY created_at;

-- name: FreezeAttemptReceipt :exec
UPDATE attempt_receipts SET frozen_at = ?, updated_at = ? WHERE attempt_id = ? AND frozen_at IS NULL;

-- name: DeleteAttemptArtifactFiles :exec
DELETE FROM attempt_artifact_files WHERE attempt_id = ?;

-- name: InsertAttemptArtifactFile :exec
INSERT INTO attempt_artifact_files (
    id, attempt_id, relative_path, change_kind,
    content_digest, size_bytes, file_mode, is_binary, unsupported_reason, additions, deletions
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListAttemptArtifactFiles :many
SELECT * FROM attempt_artifact_files WHERE attempt_id = ? ORDER BY relative_path;
