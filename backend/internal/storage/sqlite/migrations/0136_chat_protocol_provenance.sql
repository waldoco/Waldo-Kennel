-- Persisted protocol-negotiation provenance for Chat sessions (ADR 0016,
-- program checkpoint C1.6). PR #180 negotiates the installed provider's
-- protocol surface at runtime but only LOGS the outcome: after a restart, or
-- weeks later during a Result review, there is no durable answer to "which
-- provider build and negotiated surface actually ran this work". Mission
-- Control can only show what the daemon kept, so the negotiation record must
-- be kept.
--
-- Append-only episodes keyed (session_id, seq): a session re-negotiates every
-- time its controller starts (fresh binary, upgraded CLI, resumed session),
-- and an old Attempt's evidence must never be rewritten by a later
-- negotiation. One row per negotiation, never an update.
--
-- session_id is plain TEXT with NO foreign key into sessions(id), per the
-- locked D6 ruling already applied to attempt_sessions: spawn rollback deletes
-- seed session rows, but provenance is review evidence and must outlive
-- session-row GC.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE chat_protocol_provenance (
    session_id            TEXT      NOT NULL,
    seq                   INTEGER   NOT NULL,
    harness               TEXT      NOT NULL,
    provider              TEXT      NOT NULL,
    installed_version     TEXT      NOT NULL DEFAULT '',
    generated_from        TEXT      NOT NULL DEFAULT '',
    protocol_digest       TEXT      NOT NULL DEFAULT '',
    generated_digest      TEXT      NOT NULL DEFAULT '',
    matches_generated     INTEGER   NOT NULL DEFAULT 0,
    -- JSON arrays of capability / method names; '[]' when none.
    degraded_capabilities TEXT      NOT NULL DEFAULT '[]',
    missing_floor         TEXT      NOT NULL DEFAULT '[]',
    negotiated_at         TIMESTAMP NOT NULL,
    PRIMARY KEY (session_id, seq)
);
-- Read path: latest episode for one session binding, at-or-before its bound_at.
CREATE INDEX idx_chat_protocol_provenance_session_time
    ON chat_protocol_provenance (session_id, negotiated_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_chat_protocol_provenance_session_time;
DROP TABLE IF EXISTS chat_protocol_provenance;
-- +goose StatementEnd
