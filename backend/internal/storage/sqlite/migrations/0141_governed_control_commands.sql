-- Additive claims for steer, typed answer, and interrupt. Turn claims remain in
-- governed_commands so the reviewed S2.2 relation and timeline FK convention do
-- not change underneath deployed databases.
-- +goose Up
CREATE TABLE governed_control_commands (
    id TEXT PRIMARY KEY CHECK (length(trim(id)) > 0),
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE RESTRICT,
    idempotency_key TEXT NOT NULL CHECK (length(trim(idempotency_key)) > 0),
    request_fingerprint TEXT NOT NULL CHECK (length(trim(request_fingerprint)) > 0),
    command_class TEXT NOT NULL CHECK (command_class IN ('steer','answer','interrupt')),
    state TEXT NOT NULL CHECK (state IN ('claimed','dispatching','acknowledged','rejected','delivery_unknown','reconciled')),
    controller_generation TEXT NOT NULL CHECK (length(trim(controller_generation)) > 0),
    expected_revision TEXT NOT NULL CHECK (length(trim(expected_revision)) > 0),
    capability_fingerprint TEXT NOT NULL CHECK (length(trim(capability_fingerprint)) > 0),
    provider_conversation_id TEXT NOT NULL CHECK (length(trim(provider_conversation_id)) > 0),
    client_message_id TEXT NOT NULL DEFAULT '',
    provider_turn_id TEXT NOT NULL DEFAULT '',
    target_generation TEXT NOT NULL DEFAULT '',
    quiescence TEXT NOT NULL CHECK (quiescence IN ('not_applicable','pending','codex_process_tree_verified')),
    quiescence_evidence_ref TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    UNIQUE(session_id, idempotency_key)
);
-- +goose StatementBegin
CREATE TRIGGER governed_control_commands_identity_immutable
BEFORE UPDATE ON governed_control_commands
WHEN NEW.id <> OLD.id OR NEW.session_id <> OLD.session_id OR NEW.idempotency_key <> OLD.idempotency_key
 OR NEW.request_fingerprint <> OLD.request_fingerprint OR NEW.command_class <> OLD.command_class
 OR NEW.controller_generation <> OLD.controller_generation OR NEW.expected_revision <> OLD.expected_revision
 OR NEW.capability_fingerprint <> OLD.capability_fingerprint OR NEW.provider_conversation_id <> OLD.provider_conversation_id
 OR NEW.client_message_id <> OLD.client_message_id OR NEW.provider_turn_id <> OLD.provider_turn_id
 OR NEW.target_generation <> OLD.target_generation
BEGIN SELECT RAISE(ABORT, 'governed control command identity is immutable'); END;
-- +goose StatementEnd
-- +goose Down
DROP TRIGGER IF EXISTS governed_control_commands_identity_immutable;
DROP TABLE IF EXISTS governed_control_commands;
