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
-- +goose StatementBegin
PRAGMA legacy_alter_table=ON;
DROP TRIGGER IF EXISTS attempt_artifact_files_frozen_delete;
DROP TRIGGER IF EXISTS attempt_artifact_files_frozen_insert;
DROP TRIGGER IF EXISTS attempt_artifact_files_frozen_immutable;

-- Rebuild rather than ALTER DROP COLUMN. ALTER asks SQLite to reparse every
-- unrelated trigger and breaks mixed-version profiles whose repair runs only
-- after goose. The rebuild keeps every row and restores 0120's guards.
CREATE TABLE attempt_artifact_files_0156_down (
    id TEXT PRIMARY KEY,
    attempt_id TEXT NOT NULL REFERENCES attempt_receipts(attempt_id) ON DELETE CASCADE,
    relative_path TEXT NOT NULL,
    change_kind TEXT NOT NULL CHECK (change_kind IN ('added', 'modified', 'deleted', 'untracked')),
    content_digest TEXT NOT NULL DEFAULT '',
    size_bytes INTEGER,
    file_mode INTEGER,
    is_binary INTEGER NOT NULL DEFAULT 0 CHECK (is_binary IN (0, 1)),
    unsupported_reason TEXT NOT NULL DEFAULT '',
    UNIQUE (attempt_id, relative_path)
);
INSERT INTO attempt_artifact_files_0156_down (
    id, attempt_id, relative_path, change_kind, content_digest, size_bytes,
    file_mode, is_binary, unsupported_reason
) SELECT id, attempt_id, relative_path, change_kind, content_digest, size_bytes,
    file_mode, is_binary, unsupported_reason FROM attempt_artifact_files;
DROP TABLE attempt_artifact_files;
ALTER TABLE attempt_artifact_files_0156_down RENAME TO attempt_artifact_files;

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
     )
BEGIN SELECT RAISE(ABORT, 'a frozen attempt receipt cannot be replaced'); END;
CREATE TRIGGER attempt_artifact_files_frozen_insert
BEFORE INSERT ON attempt_artifact_files
WHEN (SELECT frozen_at FROM attempt_receipts WHERE attempt_id = NEW.attempt_id) IS NOT NULL
BEGIN SELECT RAISE(ABORT, 'a frozen attempt receipt cannot be replaced'); END;
CREATE TRIGGER attempt_artifact_files_frozen_delete
BEFORE DELETE ON attempt_artifact_files
WHEN (SELECT frozen_at FROM attempt_receipts WHERE attempt_id = OLD.attempt_id) IS NOT NULL
BEGIN SELECT RAISE(ABORT, 'a frozen attempt receipt cannot be replaced'); END;
PRAGMA legacy_alter_table=OFF;
-- +goose StatementEnd
