-- Versioned clarification round history. Existing scalar clarification rows are
-- intentionally not backfilled because they carry no proposal-revision proof.
-- +goose Up
-- +goose StatementBegin
CREATE TABLE intake_clarification_rounds (
    id                         TEXT PRIMARY KEY,
    intake_id                  TEXT NOT NULL REFERENCES intake_sessions (id),
    ordinal                    INTEGER NOT NULL CHECK (ordinal >= 1),
    version                    TEXT NOT NULL CHECK (version = 'intake.clarification-round.v1'),
    expected_proposal_revision INTEGER NOT NULL CHECK (expected_proposal_revision >= 0),
    explicit_reanalysis        INTEGER NOT NULL CHECK (explicit_reanalysis IN (0, 1)),
    created_at                 TIMESTAMP NOT NULL,
    UNIQUE (intake_id, ordinal),
    UNIQUE (intake_id, id)
);

CREATE TABLE intake_clarification_round_questions (
    round_id              TEXT NOT NULL REFERENCES intake_clarification_rounds (id),
    question_id           TEXT NOT NULL,
    position              INTEGER NOT NULL CHECK (position >= 1),
    question              TEXT NOT NULL CHECK (length(CAST(question AS BLOB)) BETWEEN 1 AND 4096),
    reason                TEXT NOT NULL CHECK (length(CAST(reason AS BLOB)) BETWEEN 1 AND 4096),
    recommendation        TEXT NOT NULL DEFAULT '' CHECK (length(CAST(recommendation AS BLOB)) <= 4096),
    alternatives          TEXT NOT NULL CHECK (json_valid(alternatives) AND length(CAST(alternatives AS BLOB)) <= 16384),
    deferral_consequence  TEXT NOT NULL DEFAULT '' CHECK (length(CAST(deferral_consequence AS BLOB)) <= 4096),
    PRIMARY KEY (round_id, question_id),
    UNIQUE (round_id, position)
);

CREATE TABLE intake_clarification_round_answers (
    answer_ordinal INTEGER PRIMARY KEY AUTOINCREMENT,
    round_id     TEXT NOT NULL,
    question_id  TEXT NOT NULL,
    answer       TEXT NOT NULL CHECK (length(CAST(answer AS BLOB)) BETWEEN 1 AND 4096),
    answered_at  TIMESTAMP NOT NULL,
    UNIQUE (round_id, question_id),
    FOREIGN KEY (round_id, question_id)
        REFERENCES intake_clarification_round_questions (round_id, question_id)
);

CREATE INDEX idx_intake_clarification_rounds_page
    ON intake_clarification_rounds (intake_id, ordinal);

CREATE TRIGGER intake_clarification_rounds_immutable_update
BEFORE UPDATE ON intake_clarification_rounds
BEGIN SELECT RAISE(ABORT, 'intake clarification rounds are append-only'); END;
CREATE TRIGGER intake_clarification_rounds_immutable_delete
BEFORE DELETE ON intake_clarification_rounds
BEGIN SELECT RAISE(ABORT, 'intake clarification rounds are append-only'); END;
CREATE TRIGGER intake_clarification_round_questions_immutable_update
BEFORE UPDATE ON intake_clarification_round_questions
BEGIN SELECT RAISE(ABORT, 'intake clarification questions are append-only'); END;
CREATE TRIGGER intake_clarification_round_questions_immutable_delete
BEFORE DELETE ON intake_clarification_round_questions
BEGIN SELECT RAISE(ABORT, 'intake clarification questions are append-only'); END;
CREATE TRIGGER intake_clarification_round_answers_immutable_update
BEFORE UPDATE ON intake_clarification_round_answers
BEGIN SELECT RAISE(ABORT, 'intake clarification answers are append-only'); END;
CREATE TRIGGER intake_clarification_round_answers_immutable_delete
BEFORE DELETE ON intake_clarification_round_answers
BEGIN SELECT RAISE(ABORT, 'intake clarification answers are append-only'); END;
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS intake_clarification_round_answers;
DROP TABLE IF EXISTS intake_clarification_round_questions;
DROP TABLE IF EXISTS intake_clarification_rounds;
