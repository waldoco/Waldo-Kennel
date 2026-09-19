-- 8A verification custody: content-addressed per-attempt custody manifests.
--
-- Receipts already record what an Attempt produced; what had no durable,
-- sealed record is what the Attempt was admitted WITH and the binding between
-- admission and custody close. Two insert-once halves per attempt: admission
-- writes 'input' at launch, custody close writes 'output' with the retained
-- artifact version. The payload digest is computed over the exact stored
-- bytes, so any later edit fails verification on read.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE attempt_manifests (
    attempt_id     TEXT NOT NULL REFERENCES attempts(id) ON DELETE RESTRICT,
    half           TEXT NOT NULL CHECK (half IN ('input', 'output')),
    outcome_id     TEXT NOT NULL,
    payload        TEXT NOT NULL,
    payload_digest TEXT NOT NULL,
    created_at     TIMESTAMP NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (attempt_id, half)
);

CREATE INDEX attempt_manifests_outcome_idx ON attempt_manifests(outcome_id);

-- A sealed manifest is never rewritten: custody evidence that can be edited
-- after the fact is not evidence. Outcome deletion removes rows outright -
-- erasing an Outcome is not mutating its evidence - so only UPDATE is refused.
CREATE TRIGGER attempt_manifests_no_update
BEFORE UPDATE ON attempt_manifests
BEGIN
    SELECT RAISE(ABORT, 'attempt manifests are immutable custody records');
END;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS attempt_manifests_no_update;
DROP TABLE IF EXISTS attempt_manifests;
-- +goose StatementEnd
