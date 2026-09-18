-- S3 planning readiness: blocked packets wait on system setup (or the owner
-- when an issue route is an owner decision), and a blocked planner turn is
-- recorded as readiness_blocked. Both are CHECK widenings, so SQLite requires
-- a table rebuild.

-- +goose NO TRANSACTION
-- +goose Up
-- +goose StatementBegin
PRAGMA foreign_keys=OFF;

-- RENAME re-parses triggers attached to tables whose DDL references the
-- renamed table. plan_revisions references planning_sessions, and its
-- immutability trigger names routing_decisions_json, a column installed by
-- startup reconciliation rather than a migration, so the re-parse fails on a
-- complete migration-only profile. Drop it here and recreate it with its
-- latest body after the rebuilds; the table rebuilds themselves drop every
-- trigger attached to planning_sessions/planning_turns, so those are
-- recreated below as well.
DROP TRIGGER IF EXISTS plan_revisions_immutable_update;
DROP TRIGGER IF EXISTS plan_revisions_planning_source_guard;

CREATE TABLE planning_sessions_new (
    id                       TEXT PRIMARY KEY,
    outcome_id               TEXT NOT NULL REFERENCES outcomes (id),
    project_id               TEXT NOT NULL REFERENCES projects (id),
    contract_revision_id     TEXT NOT NULL REFERENCES contract_revisions (id),
    contract_revision_number INTEGER NOT NULL CHECK (contract_revision_number >= 1),
    revision                 INTEGER NOT NULL CHECK (revision >= 1),
    latest_turn_sequence     INTEGER NOT NULL DEFAULT 0 CHECK (latest_turn_sequence >= 0),
    status                   TEXT NOT NULL CHECK (status IN ('active','proposal_ready','superseded','cancelled')),
    waiting_on               TEXT NOT NULL CHECK (waiting_on IN ('owner','provider','system','none')),
    mode                     TEXT NOT NULL CHECK (mode IN ('direct_api','native_harness')),
    requested_provider       TEXT NOT NULL,
    model_selection          TEXT NOT NULL CHECK (model_selection IN ('provider_default','explicit')),
    requested_model          TEXT NOT NULL DEFAULT '',
    requested_effort         TEXT NOT NULL DEFAULT '',
    context_mode             TEXT NOT NULL CHECK (context_mode IN ('repository_read','supplied_packet')),
    planning_grant_digest    TEXT NOT NULL CHECK (length(planning_grant_digest) = 64 AND planning_grant_digest NOT GLOB '*[^0-9a-f]*'),
    context_digest           TEXT NOT NULL CHECK (length(context_digest) = 64 AND context_digest NOT GLOB '*[^0-9a-f]*'),
    context_snapshot_json    TEXT NOT NULL CHECK (json_valid(context_snapshot_json)),
    effective_provider       TEXT NOT NULL DEFAULT '',
    effective_model          TEXT NOT NULL DEFAULT '',
    native_conversation_ref  TEXT NOT NULL DEFAULT '',
    proposed_plan_revision_id TEXT REFERENCES plan_revisions (id),
    last_failure_code        TEXT NOT NULL DEFAULT '',
    last_failure_detail      TEXT NOT NULL DEFAULT '',
    request_key              TEXT NOT NULL UNIQUE,
    request_fingerprint      TEXT NOT NULL CHECK (length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
    created_at               TIMESTAMP NOT NULL,
    updated_at               TIMESTAMP NOT NULL,
    closed_at                TIMESTAMP,
    CHECK ((model_selection = 'provider_default' AND requested_model = '') OR (model_selection = 'explicit' AND requested_model <> '')),
    CHECK (effective_provider <> '' OR (effective_model = '' AND native_conversation_ref = '')),
    CHECK ((status = 'active' AND waiting_on <> 'none' AND closed_at IS NULL AND proposed_plan_revision_id IS NULL)
        OR (status <> 'active' AND waiting_on = 'none' AND closed_at IS NOT NULL)),
    CHECK ((status = 'proposal_ready') = (proposed_plan_revision_id IS NOT NULL)),
    UNIQUE (id, outcome_id)
);

INSERT INTO planning_sessions_new (id, outcome_id, project_id, contract_revision_id, contract_revision_number,
    revision, latest_turn_sequence, status, waiting_on, mode, requested_provider, model_selection,
    requested_model, requested_effort, context_mode, planning_grant_digest, context_digest, context_snapshot_json,
    effective_provider, effective_model, native_conversation_ref, proposed_plan_revision_id,
    last_failure_code, last_failure_detail, request_key, request_fingerprint, created_at, updated_at, closed_at)
SELECT id, outcome_id, project_id, contract_revision_id, contract_revision_number,
    revision, latest_turn_sequence, status, waiting_on, mode, requested_provider, model_selection,
    requested_model, requested_effort, context_mode, planning_grant_digest, context_digest, context_snapshot_json,
    effective_provider, effective_model, native_conversation_ref, proposed_plan_revision_id,
    last_failure_code, last_failure_detail, request_key, request_fingerprint, created_at, updated_at, closed_at
FROM planning_sessions;

DROP TABLE planning_sessions;
ALTER TABLE planning_sessions_new RENAME TO planning_sessions;

CREATE UNIQUE INDEX idx_planning_sessions_one_active_outcome
    ON planning_sessions (outcome_id) WHERE status = 'active';
CREATE INDEX idx_planning_sessions_outcome_created
    ON planning_sessions (outcome_id, created_at DESC, id DESC);

CREATE TABLE planning_turns_new (
    id                       TEXT PRIMARY KEY,
    planning_session_id      TEXT NOT NULL REFERENCES planning_sessions (id),
    sequence                 INTEGER NOT NULL CHECK (sequence >= 1),
    reply_to_turn_id         TEXT REFERENCES planning_turns (id),
    role                     TEXT NOT NULL CHECK (role IN ('owner','planner')),
    kind                     TEXT NOT NULL CHECK (kind IN ('message','finalize_request','clarification','contract_change_proposal','plan_proposal','readiness_blocked')),
    text                     TEXT NOT NULL,
    structured_payload_json  TEXT CHECK (structured_payload_json IS NULL OR json_valid(structured_payload_json)),
    intelligence_run_id      TEXT REFERENCES intelligence_runs (id),
    request_key              TEXT,
    request_fingerprint      TEXT CHECK (request_fingerprint IS NULL OR (length(request_fingerprint) = 64 AND request_fingerprint NOT GLOB '*[^0-9a-f]*')),
    created_at               TIMESTAMP NOT NULL,
    CHECK ((role = 'owner' AND reply_to_turn_id IS NULL AND intelligence_run_id IS NULL AND request_key IS NOT NULL AND request_fingerprint IS NOT NULL AND kind IN ('message','finalize_request'))
        OR (role = 'planner' AND reply_to_turn_id IS NOT NULL AND intelligence_run_id IS NOT NULL AND request_key IS NULL AND request_fingerprint IS NULL AND kind IN ('clarification','contract_change_proposal','plan_proposal','readiness_blocked'))),
    UNIQUE (planning_session_id, sequence),
    UNIQUE (planning_session_id, request_key)
);

INSERT INTO planning_turns_new (id, planning_session_id, sequence, reply_to_turn_id, role, kind, text,
    structured_payload_json, intelligence_run_id, request_key, request_fingerprint, created_at)
SELECT id, planning_session_id, sequence, reply_to_turn_id, role, kind, text,
    structured_payload_json, intelligence_run_id, request_key, request_fingerprint, created_at
FROM planning_turns;

DROP TABLE planning_turns;
ALTER TABLE planning_turns_new RENAME TO planning_turns;

CREATE UNIQUE INDEX idx_planning_turns_reply
    ON planning_turns (reply_to_turn_id) WHERE reply_to_turn_id IS NOT NULL;

CREATE TRIGGER planning_sessions_update_guard
BEFORE UPDATE ON planning_sessions
WHEN OLD.id <> NEW.id
     OR OLD.outcome_id <> NEW.outcome_id
     OR OLD.project_id <> NEW.project_id
     OR OLD.contract_revision_id <> NEW.contract_revision_id
     OR OLD.contract_revision_number <> NEW.contract_revision_number
     OR OLD.mode <> NEW.mode
     OR OLD.requested_provider <> NEW.requested_provider
     OR OLD.model_selection <> NEW.model_selection
     OR OLD.requested_model <> NEW.requested_model
     OR OLD.requested_effort <> NEW.requested_effort
     OR OLD.context_mode <> NEW.context_mode
     OR OLD.planning_grant_digest <> NEW.planning_grant_digest
     OR OLD.context_digest <> NEW.context_digest
     OR OLD.context_snapshot_json <> NEW.context_snapshot_json
     OR OLD.request_key <> NEW.request_key
     OR OLD.request_fingerprint <> NEW.request_fingerprint
     OR OLD.created_at <> NEW.created_at
     OR NEW.revision <> OLD.revision + 1
     OR NEW.latest_turn_sequence < OLD.latest_turn_sequence
     OR (OLD.status <> 'active' AND NEW.status <> OLD.status)
     OR (OLD.effective_provider <> '' AND NEW.effective_provider <> OLD.effective_provider)
     OR (OLD.effective_model <> '' AND NEW.effective_model <> OLD.effective_model)
     OR (OLD.native_conversation_ref <> '' AND NEW.native_conversation_ref <> OLD.native_conversation_ref)
     OR (OLD.proposed_plan_revision_id IS NOT NULL AND NEW.proposed_plan_revision_id IS NOT OLD.proposed_plan_revision_id)
BEGIN
    SELECT RAISE(ABORT, 'planning session binding, lineage, and terminal state are immutable');
END;

CREATE TRIGGER planning_turns_immutable_update
BEFORE UPDATE ON planning_turns BEGIN
    SELECT RAISE(ABORT, 'planning turns are immutable');
END;
CREATE TRIGGER planning_turns_immutable_delete
BEFORE DELETE ON planning_turns BEGIN
    SELECT RAISE(ABORT, 'planning turns are immutable');
END;

CREATE TRIGGER planning_sessions_cdc_insert
AFTER INSERT ON planning_sessions
BEGIN
    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)
    VALUES (NEW.project_id, NULL, 'outcome_updated',
        json_object('id', NEW.outcome_id, 'type', 'planning_session', 'planningSessionId', NEW.id, 'revision', NEW.revision),
        NEW.updated_at);
END;
CREATE TRIGGER planning_sessions_cdc_update
AFTER UPDATE ON planning_sessions
BEGIN
    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)
    VALUES (NEW.project_id, NULL, 'outcome_updated',
        json_object('id', NEW.outcome_id, 'type', 'planning_session', 'planningSessionId', NEW.id, 'revision', NEW.revision),
        NEW.updated_at);
END;

CREATE TRIGGER plan_revisions_planning_source_guard
BEFORE INSERT ON plan_revisions
WHEN NEW.planning_session_id IS NOT NULL OR NEW.source_intelligence_run_id IS NOT NULL
BEGIN
    SELECT CASE WHEN NEW.planning_session_id IS NULL OR NEW.source_intelligence_run_id IS NULL
        THEN RAISE(ABORT, 'planning Plan provenance must be complete') END;
    SELECT CASE WHEN NOT EXISTS (
        SELECT 1 FROM planning_sessions s
        JOIN planning_turns t ON t.planning_session_id = s.id
        JOIN contract_revisions cr
          ON cr.id = s.contract_revision_id
         AND cr.outcome_id = s.outcome_id
         AND cr.number = s.contract_revision_number
        JOIN outcomes o
          ON o.id = s.outcome_id
         AND o.current_revision_number = s.contract_revision_number
        WHERE s.id = NEW.planning_session_id
          AND s.outcome_id = NEW.outcome_id
          AND s.contract_revision_number = NEW.contract_revision_number
          AND s.status = 'active'
          AND t.intelligence_run_id = NEW.source_intelligence_run_id
          AND t.kind = 'plan_proposal'
    ) THEN RAISE(ABORT, 'planning Plan provenance does not match active session') END;
END;

CREATE TRIGGER plan_revisions_immutable_update
BEFORE UPDATE ON plan_revisions
WHEN OLD.id <> NEW.id
     OR OLD.outcome_id <> NEW.outcome_id
     OR OLD.number <> NEW.number
     OR OLD.contract_revision_number <> NEW.contract_revision_number
     OR OLD.summary <> NEW.summary
     OR OLD.assumptions_json <> NEW.assumptions_json
     OR OLD.blockers_json <> NEW.blockers_json
     OR OLD.run_brief_core_digest <> NEW.run_brief_core_digest
     OR OLD.run_brief_compiled_digest IS NOT NEW.run_brief_compiled_digest
     OR OLD.routing_decisions_json IS NOT NEW.routing_decisions_json
     OR OLD.planning_session_id IS NOT NEW.planning_session_id
     OR OLD.source_intelligence_run_id IS NOT NEW.source_intelligence_run_id
     OR OLD.created_at <> NEW.created_at
BEGIN
    SELECT RAISE(ABORT, 'plan revisions are immutable');
END;

PRAGMA foreign_keys=ON;
PRAGMA foreign_key_check;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- SQLite cannot narrow these CHECKs safely once readiness_blocked turns or
-- system-waiting sessions may exist. Keep the widened constraints in place,
-- matching the existing best-effort down-migration style for widened CHECKs.
-- +goose StatementEnd
