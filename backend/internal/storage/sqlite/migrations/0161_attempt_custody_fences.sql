-- 8B verification custody: typed custody-close fences.
--
-- A fence is the durable record that the provider process had exited and the
-- workspace had settled before the result snapshot was taken. Insert-once per
-- attempt: custody close is crossed once, and a replayed close after a daemon
-- restart reads the existing fence instead of writing a second one.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE attempt_custody_fences (
    attempt_id TEXT NOT NULL PRIMARY KEY REFERENCES attempts(id) ON DELETE RESTRICT,
    session_id TEXT NOT NULL,
    detail     TEXT NOT NULL DEFAULT '',
    fenced_at  TIMESTAMP NOT NULL
);

-- A recorded fence is never rewritten, like every other custody record.
-- Outcome deletion removes rows outright, so only UPDATE is refused.
CREATE TRIGGER attempt_custody_fences_no_update
BEFORE UPDATE ON attempt_custody_fences
BEGIN
    SELECT RAISE(ABORT, 'attempt custody fences are immutable custody records');
END;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS attempt_custody_fences_no_update;
DROP TABLE IF EXISTS attempt_custody_fences;
-- +goose StatementEnd
