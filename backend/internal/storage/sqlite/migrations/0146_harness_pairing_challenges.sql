-- Durable one-time pairing-challenge lifecycle. proof_verifier is one-way;
-- no raw challenge secret or S3.1 bearer is ever persisted here.
-- +goose Up
CREATE TABLE harness_pairing_challenges (
 id TEXT PRIMARY KEY,
 kind TEXT NOT NULL CHECK(kind IN ('pair','rotate')),
 connection_id TEXT NOT NULL,
 installation_id TEXT NOT NULL,
 adapter_digest TEXT NOT NULL CHECK(length(adapter_digest)=64),
 harness_identity TEXT NOT NULL,
 provider_version TEXT NOT NULL,
 protocol_fingerprint TEXT NOT NULL CHECK(length(protocol_fingerprint)=64),
 mission_id TEXT NOT NULL,
 app_run_id TEXT NOT NULL,
 capability_classes TEXT NOT NULL,
 expected_generation INTEGER NOT NULL CHECK(expected_generation >= 1),
 proof_verifier TEXT NOT NULL CHECK(length(proof_verifier)=64),
 status TEXT NOT NULL CHECK(status IN ('pending','consumed','superseded')) DEFAULT 'pending',
 result_code TEXT,
 -- connection_expires_at is the expiry the issuing caller decided for the
 -- resulting S3.1 connection generation. It is captured here, at issue time,
 -- so the adapter can never influence its own bearer's expiry at prove time.
 connection_expires_at TIMESTAMP NOT NULL,
 expires_at TIMESTAMP NOT NULL,
 created_at TIMESTAMP NOT NULL,
 updated_at TIMESTAMP NOT NULL
);
CREATE INDEX harness_pairing_challenges_connection_status ON harness_pairing_challenges(connection_id, status);
CREATE TRIGGER harness_pairing_challenges_binding_immutable BEFORE UPDATE ON harness_pairing_challenges
WHEN NEW.id<>OLD.id OR NEW.kind<>OLD.kind OR NEW.connection_id<>OLD.connection_id
 OR NEW.installation_id<>OLD.installation_id OR NEW.adapter_digest<>OLD.adapter_digest
 OR NEW.harness_identity<>OLD.harness_identity OR NEW.provider_version<>OLD.provider_version
 OR NEW.protocol_fingerprint<>OLD.protocol_fingerprint OR NEW.mission_id<>OLD.mission_id
 OR NEW.app_run_id<>OLD.app_run_id OR NEW.capability_classes<>OLD.capability_classes
 OR NEW.expected_generation<>OLD.expected_generation OR NEW.proof_verifier<>OLD.proof_verifier
 OR NEW.connection_expires_at<>OLD.connection_expires_at
 OR NEW.expires_at<>OLD.expires_at OR NEW.created_at<>OLD.created_at
BEGIN SELECT RAISE(ABORT, 'harness pairing challenge binding is immutable'); END;
-- +goose Down
DROP TRIGGER IF EXISTS harness_pairing_challenges_binding_immutable;
DROP INDEX IF EXISTS harness_pairing_challenges_connection_status;
DROP TABLE IF EXISTS harness_pairing_challenges;
