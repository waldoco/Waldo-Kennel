-- +goose Up
ALTER TABLE attempt_artifact_files ADD COLUMN additions INTEGER CHECK(additions >= 0);
ALTER TABLE attempt_artifact_files ADD COLUMN deletions INTEGER CHECK(deletions >= 0);

-- +goose StatementBegin
-- Measurements on a frozen receipt are review evidence too. Replace 0120's
-- guard so the two new columns cannot be rewritten after review.
DROP TRIGGER IF EXISTS attempt_artifact_files_frozen_immutable;
CREATE TRIGGER attempt_artifact_files_frozen_immutable
BEFORE UPDATE ON attempt_artifact_files
WHEN (SELECT frozen_at FROM attempt_receipts WHERE attempt_id = OLD.attempt_id) IS NOT NULL
     AND (
          OLD.id IS NOT NEW.id
       OR OLD.attempt_id IS NOT NEW.attempt_id
       OR OLD.relative_path IS NOT NEW.relative_path
       OR OLD.change_kind IS NOT NEW.change_kind
       OR OLD.content_digest IS NOT NEW.content_digest
       OR OLD.size_bytes IS NOT NEW.size_bytes
       OR OLD.file_mode IS NOT NEW.file_mode
       OR OLD.is_binary IS NOT NEW.is_binary
       OR OLD.unsupported_reason IS NOT NEW.unsupported_reason
       OR OLD.additions IS NOT NEW.additions
       OR OLD.deletions IS NOT NEW.deletions
     )
BEGIN
    SELECT RAISE(ABORT, 'a frozen attempt receipt cannot be replaced');
END;
-- +goose StatementEnd

-- +goose Down
-- Retained receipt schema is intentionally non-destructive. Existing frozen
-- evidence may carry these measurements and SQLite cannot safely drop columns
-- while immutable cross-table triggers from mixed-version profiles exist.
SELECT 1;
