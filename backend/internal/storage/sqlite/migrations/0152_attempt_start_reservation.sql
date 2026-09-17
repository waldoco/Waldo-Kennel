-- Prelaunch Attempt reservations. Expected integration collision: renumber this migration.
-- +goose NO TRANSACTION
-- +goose Up
PRAGMA foreign_keys=OFF;
PRAGMA legacy_alter_table=ON;

DROP TRIGGER IF EXISTS attempts_immutable_update;
DROP TRIGGER IF EXISTS attempts_status_transition;
DROP TRIGGER IF EXISTS attempts_immutable_delete;
ALTER TABLE attempts RENAME TO attempts_pre_start_reservation;
CREATE TABLE attempts (
 id TEXT PRIMARY KEY,
 outcome_id TEXT NOT NULL REFERENCES outcomes(id),
 plan_revision_id TEXT NOT NULL REFERENCES plan_revisions(id),
 work_unit_id TEXT NOT NULL REFERENCES work_units(id),
 number INTEGER NOT NULL CHECK(number >= 1),
 status TEXT NOT NULL CHECK(status IN ('awaiting_authority','queued','running','paused','succeeded','failed','cancelled','lost','reconciled')),
 contract_revision_number INTEGER NOT NULL CHECK(contract_revision_number >= 1),
 request_key TEXT,
 created_at TIMESTAMP NOT NULL DEFAULT(datetime('now')),
 updated_at TIMESTAMP NOT NULL DEFAULT(datetime('now')),
 run_intent_generation INTEGER NOT NULL DEFAULT 0 CHECK(run_intent_generation >= 0)
);
INSERT INTO attempts SELECT id,outcome_id,plan_revision_id,work_unit_id,number,status,contract_revision_number,request_key,created_at,updated_at,run_intent_generation FROM attempts_pre_start_reservation;
DROP TABLE attempts_pre_start_reservation;
CREATE UNIQUE INDEX idx_attempts_outcome_number ON attempts(outcome_id,number);
CREATE UNIQUE INDEX idx_attempts_request_key ON attempts(request_key) WHERE request_key IS NOT NULL;
CREATE UNIQUE INDEX idx_attempts_reservation_parent ON attempts(id,outcome_id,plan_revision_id,work_unit_id,contract_revision_number,run_intent_generation);

CREATE TRIGGER attempts_immutable_update BEFORE UPDATE ON attempts
WHEN OLD.id<>NEW.id OR OLD.outcome_id<>NEW.outcome_id OR OLD.plan_revision_id<>NEW.plan_revision_id OR OLD.work_unit_id<>NEW.work_unit_id OR OLD.number<>NEW.number OR OLD.contract_revision_number<>NEW.contract_revision_number OR OLD.run_intent_generation<>NEW.run_intent_generation OR OLD.request_key IS NOT NEW.request_key OR OLD.created_at<>NEW.created_at
BEGIN SELECT RAISE(ABORT,'attempts are immutable'); END;
CREATE TRIGGER attempts_status_transition BEFORE UPDATE ON attempts
WHEN OLD.status<>NEW.status AND NOT (
 (OLD.status='awaiting_authority' AND NEW.status IN ('queued','failed','cancelled')) OR
 (OLD.status='queued' AND NEW.status IN ('running','failed','cancelled','lost')) OR
 (OLD.status='running' AND NEW.status IN ('paused','failed','cancelled','lost','reconciled')) OR
 (OLD.status='paused' AND NEW.status IN ('running','cancelled','lost')) OR
 (OLD.status='reconciled' AND NEW.status='succeeded'))
BEGIN SELECT RAISE(ABORT,'illegal attempt status transition'); END;
CREATE TRIGGER attempts_immutable_delete BEFORE DELETE ON attempts BEGIN SELECT RAISE(ABORT,'attempts are append-only'); END;

CREATE TABLE attempt_start_reservations (
 id TEXT PRIMARY KEY,
 attempt_id TEXT NOT NULL UNIQUE REFERENCES attempts(id) ON DELETE RESTRICT,
 outcome_id TEXT NOT NULL,
 plan_revision_id TEXT NOT NULL,
 work_unit_id TEXT NOT NULL,
 contract_revision_number INTEGER NOT NULL CHECK(contract_revision_number>=1),
 run_intent_generation INTEGER NOT NULL CHECK(run_intent_generation>=0),
 request_key TEXT NOT NULL UNIQUE CHECK(length(trim(request_key))>0),
 request_fingerprint TEXT NOT NULL CHECK(length(trim(request_fingerprint))>0),
 routing_snapshot_id TEXT NOT NULL CHECK(length(trim(routing_snapshot_id))>0),
 routing_generation_id TEXT NOT NULL CHECK(length(trim(routing_generation_id))>0),
 admission_evaluation_id TEXT NOT NULL UNIQUE CHECK(length(trim(admission_evaluation_id))>0),
 refusal_status TEXT NOT NULL CHECK(refusal_status IN ('open','superseded','denied','admitted')),
 denial_detail TEXT NOT NULL CHECK(json_valid(denial_detail)),
 created_at TIMESTAMP NOT NULL,
 FOREIGN KEY(attempt_id,outcome_id,plan_revision_id,work_unit_id,contract_revision_number,run_intent_generation)
 REFERENCES attempts(id,outcome_id,plan_revision_id,work_unit_id,contract_revision_number,run_intent_generation)
);
CREATE UNIQUE INDEX idx_attempts_reservation_identity ON attempt_start_reservations(outcome_id,plan_revision_id,work_unit_id,run_intent_generation,request_fingerprint);
CREATE TRIGGER attempt_start_reservations_immutable_identity BEFORE UPDATE ON attempt_start_reservations
WHEN OLD.id<>NEW.id OR OLD.attempt_id<>NEW.attempt_id OR OLD.outcome_id<>NEW.outcome_id OR OLD.plan_revision_id<>NEW.plan_revision_id OR OLD.work_unit_id<>NEW.work_unit_id OR OLD.contract_revision_number<>NEW.contract_revision_number OR OLD.run_intent_generation<>NEW.run_intent_generation OR OLD.request_key<>NEW.request_key OR OLD.request_fingerprint<>NEW.request_fingerprint OR OLD.routing_snapshot_id<>NEW.routing_snapshot_id OR OLD.routing_generation_id<>NEW.routing_generation_id OR OLD.admission_evaluation_id<>NEW.admission_evaluation_id OR OLD.denial_detail<>NEW.denial_detail OR OLD.created_at<>NEW.created_at
BEGIN SELECT RAISE(ABORT,'attempt start reservation identity is immutable'); END;
CREATE TRIGGER attempt_start_reservations_status_transition BEFORE UPDATE ON attempt_start_reservations
WHEN OLD.refusal_status<>NEW.refusal_status AND NOT (OLD.refusal_status='open' AND NEW.refusal_status IN ('superseded','denied','admitted'))
BEGIN SELECT RAISE(ABORT,'illegal attempt start refusal transition'); END;
CREATE TRIGGER attempt_start_reservations_no_delete BEFORE DELETE ON attempt_start_reservations BEGIN SELECT RAISE(ABORT,'attempt start reservations are append-only'); END;
PRAGMA legacy_alter_table=OFF;
PRAGMA foreign_keys=ON;

-- +goose Down
-- Durable prelaunch identity is intentionally non-destructive. Integration should not down-migrate past used reservations.
SELECT 1;
