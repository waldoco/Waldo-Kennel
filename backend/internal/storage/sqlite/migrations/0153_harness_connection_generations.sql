-- Preserve immutable connection generations while the canonical row advances after proof.
-- +goose Up
CREATE TABLE harness_connection_generations (
 connection_id TEXT NOT NULL, generation INTEGER NOT NULL CHECK(generation>=1),
 installation_id TEXT NOT NULL, adapter_digest TEXT NOT NULL CHECK(length(adapter_digest)=64),
 harness_identity TEXT NOT NULL, provider_version TEXT NOT NULL, protocol_fingerprint TEXT NOT NULL CHECK(length(protocol_fingerprint)=64),
 mission_id TEXT NOT NULL, app_run_id TEXT NOT NULL, capability_classes TEXT NOT NULL,
 capability_verifier TEXT NOT NULL CHECK(length(capability_verifier)=64), expires_at TIMESTAMP NOT NULL,
 revoked_at TIMESTAMP, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL,
 PRIMARY KEY(connection_id,generation)
);
CREATE TRIGGER harness_connection_generations_no_update BEFORE UPDATE ON harness_connection_generations BEGIN SELECT RAISE(ABORT,'harness connection generation is immutable'); END;
CREATE TRIGGER harness_connection_generations_no_delete BEFORE DELETE ON harness_connection_generations BEGIN SELECT RAISE(ABORT,'harness connection generation is immutable'); END;
DROP TRIGGER harness_connections_binding_immutable;
-- +goose StatementBegin
CREATE TRIGGER harness_connections_app_run_rotation_guard BEFORE UPDATE OF app_run_id ON harness_connections
WHEN NEW.app_run_id<>OLD.app_run_id BEGIN
 SELECT CASE WHEN NEW.generation<>OLD.generation+1 OR NEW.capability_verifier=OLD.capability_verifier OR NEW.revoked_at IS NOT NULL OR NEW.expires_at<=NEW.updated_at OR NEW.updated_at<OLD.updated_at OR NOT EXISTS (SELECT 1 FROM harness_connection_generations g WHERE g.connection_id=OLD.id AND g.generation=OLD.generation AND g.installation_id IS OLD.installation_id AND g.adapter_digest IS OLD.adapter_digest AND g.harness_identity IS OLD.harness_identity AND g.provider_version IS OLD.provider_version AND g.protocol_fingerprint IS OLD.protocol_fingerprint AND g.mission_id IS OLD.mission_id AND g.app_run_id IS OLD.app_run_id AND g.capability_classes IS OLD.capability_classes AND g.capability_verifier IS OLD.capability_verifier AND g.expires_at IS OLD.expires_at AND g.revoked_at IS OLD.revoked_at AND g.created_at IS OLD.created_at AND g.updated_at IS OLD.updated_at)
 THEN RAISE(ABORT,'harness app run rotation is not fully archived') END;
END;
-- +goose StatementEnd
CREATE TRIGGER harness_connections_binding_immutable BEFORE UPDATE ON harness_connections
WHEN NEW.id<>OLD.id OR NEW.installation_id<>OLD.installation_id OR NEW.adapter_digest<>OLD.adapter_digest
 OR NEW.harness_identity<>OLD.harness_identity OR NEW.provider_version<>OLD.provider_version
 OR NEW.protocol_fingerprint<>OLD.protocol_fingerprint OR NEW.mission_id<>OLD.mission_id
 OR NEW.capability_classes<>OLD.capability_classes OR NEW.created_at<>OLD.created_at
BEGIN SELECT RAISE(ABORT, 'harness connection binding is immutable'); END;
-- +goose Down
DROP TRIGGER harness_connections_app_run_rotation_guard;
DROP TRIGGER harness_connections_binding_immutable;
CREATE TRIGGER harness_connections_binding_immutable BEFORE UPDATE ON harness_connections
WHEN NEW.id<>OLD.id OR NEW.installation_id<>OLD.installation_id OR NEW.adapter_digest<>OLD.adapter_digest
 OR NEW.harness_identity<>OLD.harness_identity OR NEW.provider_version<>OLD.provider_version
 OR NEW.protocol_fingerprint<>OLD.protocol_fingerprint OR NEW.mission_id<>OLD.mission_id
 OR NEW.app_run_id<>OLD.app_run_id OR NEW.capability_classes<>OLD.capability_classes OR NEW.created_at<>OLD.created_at
BEGIN SELECT RAISE(ABORT, 'harness connection binding is immutable'); END;
DROP TRIGGER harness_connection_generations_no_delete;
DROP TRIGGER harness_connection_generations_no_update;
DROP TABLE harness_connection_generations;
