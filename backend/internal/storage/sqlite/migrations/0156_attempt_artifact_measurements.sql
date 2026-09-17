-- +goose Up
ALTER TABLE attempt_artifact_files ADD COLUMN additions INTEGER CHECK(additions >= 0);
ALTER TABLE attempt_artifact_files ADD COLUMN deletions INTEGER CHECK(deletions >= 0);

-- +goose Down
-- Retained receipt schema is intentionally non-destructive. Existing frozen
-- evidence may carry these measurements and SQLite cannot safely drop columns
-- while immutable cross-table triggers from mixed-version profiles exist.
SELECT 1;
