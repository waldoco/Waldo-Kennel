-- Durable adapter authentication. capability_verifier is one-way; no bearer is persisted.
-- +goose Up
CREATE TABLE harness_connections (
 id TEXT PRIMARY KEY,
 installation_id TEXT NOT NULL,
 adapter_digest TEXT NOT NULL CHECK(length(adapter_digest)=64),
 harness_identity TEXT NOT NULL,
 provider_version TEXT NOT NULL,
 protocol_fingerprint TEXT NOT NULL CHECK(length(protocol_fingerprint)=64),
 mission_id TEXT NOT NULL,
 app_run_id TEXT NOT NULL,
 capability_classes TEXT NOT NULL,
 capability_verifier TEXT NOT NULL CHECK(length(capability_verifier)=64),
 generation INTEGER NOT NULL CHECK(generation >= 1),
 expires_at TIMESTAMP NOT NULL,
 revoked_at TIMESTAMP,
 created_at TIMESTAMP NOT NULL,
 updated_at TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX harness_connections_binding_generation ON harness_connections(installation_id,adapter_digest,harness_identity,mission_id,app_run_id,generation);
CREATE TRIGGER harness_connections_binding_immutable BEFORE UPDATE ON harness_connections
WHEN NEW.id<>OLD.id OR NEW.installation_id<>OLD.installation_id OR NEW.adapter_digest<>OLD.adapter_digest
 OR NEW.harness_identity<>OLD.harness_identity OR NEW.provider_version<>OLD.provider_version
 OR NEW.protocol_fingerprint<>OLD.protocol_fingerprint OR NEW.mission_id<>OLD.mission_id
 OR NEW.app_run_id<>OLD.app_run_id OR NEW.capability_classes<>OLD.capability_classes OR NEW.created_at<>OLD.created_at
BEGIN SELECT RAISE(ABORT, 'harness connection binding is immutable'); END;
-- +goose Down
DROP TRIGGER IF EXISTS harness_connections_binding_immutable;
DROP INDEX IF EXISTS harness_connections_binding_generation;
DROP TABLE IF EXISTS harness_connections;
