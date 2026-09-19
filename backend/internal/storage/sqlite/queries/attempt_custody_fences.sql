-- Typed custody-close fences. See migration 0160.

-- name: InsertAttemptCustodyFence :exec
INSERT INTO attempt_custody_fences (attempt_id, session_id, detail, fenced_at)
VALUES (?, ?, ?, ?);

-- name: GetAttemptCustodyFence :one
SELECT * FROM attempt_custody_fences WHERE attempt_id = ?;
