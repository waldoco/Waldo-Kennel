-- Immutable decisions minted only by the app-run owner-command capability.
-- +goose Up
CREATE TABLE attempt_replacement_decisions (
    id                       TEXT PRIMARY KEY,
    outcome_id               TEXT NOT NULL REFERENCES outcomes (id),
    predecessor_attempt_id   TEXT NOT NULL REFERENCES attempts (id),
    plan_revision_id         TEXT NOT NULL REFERENCES plan_revisions (id),
    work_unit_id             TEXT NOT NULL REFERENCES work_units (id),
    run_intent_generation    INTEGER NOT NULL CHECK (run_intent_generation >= 1),
    contract_revision_number INTEGER NOT NULL CHECK (contract_revision_number >= 1),
    action                   TEXT NOT NULL CHECK (action = 'replace'),
    request_key              TEXT NOT NULL UNIQUE CHECK (length(trim(request_key)) > 0),
    request_fingerprint      TEXT NOT NULL CHECK (
                                  length(request_fingerprint) = 64
                                  AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
    owner_principal          TEXT NOT NULL CHECK (owner_principal GLOB 'local-owner:apprun-*'),
    created_at               TIMESTAMP NOT NULL
);
-- +goose StatementBegin
CREATE TRIGGER attempt_replacement_decisions_authority_insert
BEFORE INSERT ON attempt_replacement_decisions
WHEN NOT EXISTS (
    SELECT 1
    FROM attempts AS a
    JOIN outcomes AS o ON o.id = a.outcome_id
    WHERE a.id = NEW.predecessor_attempt_id
      AND a.outcome_id = NEW.outcome_id
      AND a.plan_revision_id = NEW.plan_revision_id
      AND a.work_unit_id = NEW.work_unit_id
      AND a.run_intent_generation = NEW.run_intent_generation
      AND a.contract_revision_number = NEW.contract_revision_number
      AND a.status IN ('succeeded','failed','cancelled','lost','reconciled')
      AND o.current_revision_number = NEW.contract_revision_number
      AND EXISTS (
          SELECT 1 FROM outcome_run_intents AS r
          WHERE r.outcome_id = NEW.outcome_id
            AND r.generation = NEW.run_intent_generation
            AND r.plan_revision_id = NEW.plan_revision_id
            AND r.contract_revision_number = NEW.contract_revision_number
            AND r.desired = 'running'
            AND r.generation = (SELECT MAX(latest.generation) FROM outcome_run_intents AS latest WHERE latest.outcome_id = NEW.outcome_id)
      )
)
BEGIN
    SELECT RAISE(ABORT, 'attempt replacement decision authority is not current');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER attempt_replacement_decisions_immutable_update
BEFORE UPDATE ON attempt_replacement_decisions
BEGIN
    SELECT RAISE(ABORT, 'attempt replacement decisions are immutable');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER attempt_replacement_decisions_immutable_delete
BEFORE DELETE ON attempt_replacement_decisions
BEGIN
    SELECT RAISE(ABORT, 'attempt replacement decisions are immutable');
END;
-- +goose StatementEnd
-- +goose Down
DROP TRIGGER IF EXISTS attempt_replacement_decisions_immutable_delete;
DROP TRIGGER IF EXISTS attempt_replacement_decisions_immutable_update;
DROP TRIGGER IF EXISTS attempt_replacement_decisions_authority_insert;
DROP TABLE IF EXISTS attempt_replacement_decisions;
