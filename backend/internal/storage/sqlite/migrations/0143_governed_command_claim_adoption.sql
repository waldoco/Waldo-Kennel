-- A replacement controller may adopt a governed turn only before dispatching.
-- The state transition to dispatching is the durable pre-effect boundary; its
-- generation CAS fences the old controller before provider contact.
-- +goose Up
-- +goose StatementBegin
DROP TRIGGER IF EXISTS governed_commands_claim_identity_immutable;
CREATE TRIGGER governed_commands_claim_identity_immutable
BEFORE UPDATE ON governed_commands
WHEN NEW.id <> OLD.id
  OR NEW.session_id <> OLD.session_id
  OR NEW.idempotency_key <> OLD.idempotency_key
  OR NEW.request_fingerprint <> OLD.request_fingerprint
  OR NEW.command_class <> OLD.command_class
  OR (NEW.controller_generation <> OLD.controller_generation AND (OLD.state <> 'claimed' OR NEW.state <> 'claimed'))
  OR NEW.expected_revision <> OLD.expected_revision
  OR NEW.capability_fingerprint <> OLD.capability_fingerprint
  OR NEW.provider_conversation_id <> OLD.provider_conversation_id
  OR NEW.client_message_id <> OLD.client_message_id
  OR NEW.replay_strategy <> OLD.replay_strategy
BEGIN
    SELECT RAISE(ABORT, 'governed command claim identity is immutable');
END;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS governed_commands_claim_identity_immutable;
CREATE TRIGGER governed_commands_claim_identity_immutable
BEFORE UPDATE ON governed_commands
WHEN NEW.id <> OLD.id
  OR NEW.session_id <> OLD.session_id
  OR NEW.idempotency_key <> OLD.idempotency_key
  OR NEW.request_fingerprint <> OLD.request_fingerprint
  OR NEW.command_class <> OLD.command_class
  OR NEW.controller_generation <> OLD.controller_generation
  OR NEW.expected_revision <> OLD.expected_revision
  OR NEW.capability_fingerprint <> OLD.capability_fingerprint
  OR NEW.provider_conversation_id <> OLD.provider_conversation_id
  OR NEW.client_message_id <> OLD.client_message_id
  OR NEW.replay_strategy <> OLD.replay_strategy
BEGIN
    SELECT RAISE(ABORT, 'governed command claim identity is immutable');
END;
-- +goose StatementEnd
