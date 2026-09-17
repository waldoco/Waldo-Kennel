-- +goose Up
CREATE TABLE capability_escalations(
 digest TEXT PRIMARY KEY CHECK(length(digest)=64), question_id TEXT NOT NULL UNIQUE REFERENCES owner_answer_questions(id),
 version TEXT NOT NULL, outcome_id TEXT NOT NULL REFERENCES outcomes(id), contract_revision_number INTEGER NOT NULL,
 plan_revision_id TEXT NOT NULL REFERENCES plan_revisions(id),
 work_unit_id TEXT NOT NULL REFERENCES work_units(id), attempt_id TEXT NOT NULL REFERENCES attempts(id), attempt_generation INTEGER NOT NULL,
 session_id TEXT NOT NULL, session_generation INTEGER NOT NULL, executor_kind TEXT NOT NULL, attempt_session_ref_id TEXT NOT NULL, runtime_launch_id TEXT NOT NULL DEFAULT '', controller_generation TEXT NOT NULL DEFAULT '', policy_digest TEXT NOT NULL, artifact_version TEXT NOT NULL DEFAULT '', check_id TEXT NOT NULL DEFAULT '',
 requested_capability TEXT NOT NULL, denial_source TEXT NOT NULL, grant_fingerprint TEXT NOT NULL,
 operation_id TEXT NOT NULL, request_fingerprint TEXT NOT NULL, question_generation TEXT NOT NULL,
 within_contract_ceiling INTEGER NOT NULL CHECK(within_contract_ceiling IN(0,1)), created_at TIMESTAMP NOT NULL, superseded_at TIMESTAMP,
 UNIQUE(attempt_id,attempt_generation,requested_capability,operation_id,request_fingerprint,session_id,session_generation,controller_generation,question_generation)
);
CREATE UNIQUE INDEX capability_escalations_live_operation ON capability_escalations(attempt_id,attempt_generation,requested_capability,operation_id) WHERE superseded_at IS NULL;
CREATE TABLE capability_escalation_receipts(
 id TEXT PRIMARY KEY, escalation_digest TEXT NOT NULL REFERENCES capability_escalations(digest), question_id TEXT NOT NULL,
 question_generation TEXT NOT NULL, consequence TEXT NOT NULL CHECK(consequence IN('grant_once','revision_requested','denied')),
 attempt_id TEXT NOT NULL, attempt_generation INTEGER NOT NULL, session_id TEXT NOT NULL, session_generation INTEGER NOT NULL,
 controller_generation TEXT NOT NULL, capability TEXT NOT NULL, operation_id TEXT NOT NULL, request_fingerprint TEXT NOT NULL,
 grant_fingerprint TEXT NOT NULL, answer_request_key TEXT NOT NULL, created_at TIMESTAMP NOT NULL, consumed_at TIMESTAMP,
 UNIQUE(question_id,question_generation)
);
CREATE TRIGGER capability_escalations_immutable BEFORE UPDATE ON capability_escalations WHEN NEW.digest<>OLD.digest OR NEW.question_id<>OLD.question_id OR NEW.version<>OLD.version OR NEW.outcome_id<>OLD.outcome_id OR NEW.contract_revision_number<>OLD.contract_revision_number OR NEW.plan_revision_id<>OLD.plan_revision_id OR NEW.work_unit_id<>OLD.work_unit_id OR NEW.attempt_id<>OLD.attempt_id OR NEW.attempt_generation<>OLD.attempt_generation OR NEW.session_id<>OLD.session_id OR NEW.session_generation<>OLD.session_generation OR NEW.executor_kind<>OLD.executor_kind OR NEW.attempt_session_ref_id<>OLD.attempt_session_ref_id OR NEW.runtime_launch_id<>OLD.runtime_launch_id OR NEW.controller_generation<>OLD.controller_generation OR NEW.policy_digest<>OLD.policy_digest OR NEW.artifact_version<>OLD.artifact_version OR NEW.check_id<>OLD.check_id OR NEW.requested_capability<>OLD.requested_capability OR NEW.denial_source<>OLD.denial_source OR NEW.grant_fingerprint<>OLD.grant_fingerprint OR NEW.operation_id<>OLD.operation_id OR NEW.request_fingerprint<>OLD.request_fingerprint OR NEW.question_generation<>OLD.question_generation OR NEW.within_contract_ceiling<>OLD.within_contract_ceiling OR NEW.created_at<>OLD.created_at BEGIN SELECT RAISE(ABORT,'capability escalation identity is immutable'); END;
-- +goose Down
DROP TRIGGER IF EXISTS capability_escalations_immutable;
DROP TABLE IF EXISTS capability_escalation_receipts;
DROP INDEX IF EXISTS capability_escalations_live_operation;
DROP TABLE IF EXISTS capability_escalations;
