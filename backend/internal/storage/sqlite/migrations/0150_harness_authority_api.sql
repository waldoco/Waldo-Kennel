-- +goose Up
CREATE TABLE harness_pairing_intents (
 id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES projects(id), kind TEXT NOT NULL CHECK(kind IN ('pair','rotate')),
 connection_id TEXT NOT NULL, installation_id TEXT NOT NULL, adapter_digest TEXT NOT NULL CHECK(length(adapter_digest)=64),
 harness_identity TEXT NOT NULL, provider_version TEXT NOT NULL, protocol_fingerprint TEXT NOT NULL CHECK(length(protocol_fingerprint)=64),
 mission_id TEXT NOT NULL, app_run_id TEXT NOT NULL, capability_classes TEXT NOT NULL, expected_generation INTEGER NOT NULL CHECK(expected_generation>=1),
 connection_expires_at TIMESTAMP NOT NULL, expires_at TIMESTAMP NOT NULL, digest TEXT NOT NULL CHECK(length(digest)=64),
 status TEXT NOT NULL CHECK(status IN ('requested','approved','activating','challenge_active','denied','activation_failed')),
 challenge_id TEXT REFERENCES harness_pairing_challenges(id), decision_id TEXT NOT NULL DEFAULT '', decision TEXT NOT NULL DEFAULT '',
 decision_request_key TEXT NOT NULL DEFAULT '', owner_principal TEXT NOT NULL DEFAULT '', confirmation_ref TEXT NOT NULL DEFAULT '', decided_at TIMESTAMP,
 created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL
);
DROP TRIGGER IF EXISTS command_authority_claims_immutable;
CREATE TRIGGER command_authority_claims_immutable BEFORE UPDATE ON command_authority_claims
WHEN NEW.id<>OLD.id OR NEW.adapter_request_key<>OLD.adapter_request_key OR NEW.request_fingerprint<>OLD.request_fingerprint OR NEW.owner_proof_id<>OLD.owner_proof_id OR NEW.harness_connection_id<>OLD.harness_connection_id OR NEW.connection_generation<>OLD.connection_generation OR NEW.connection_binding_digest<>OLD.connection_binding_digest OR NEW.connection_expires_at<>OLD.connection_expires_at OR NEW.transport_class<>OLD.transport_class OR NEW.app_run_id<>OLD.app_run_id OR NEW.mission_id<>OLD.mission_id OR NEW.content_digest<>OLD.content_digest OR NEW.target_digest<>OLD.target_digest OR NEW.owner_class<>OLD.owner_class OR NEW.canonical_version<>OLD.canonical_version OR NEW.canonical_payload<>OLD.canonical_payload OR NEW.destination_type<>OLD.destination_type OR NEW.destination_id<>OLD.destination_id OR NEW.created_at<>OLD.created_at
BEGIN SELECT RAISE(ABORT,'command authority claim identity is immutable'); END;
CREATE INDEX harness_pairing_intents_project_updated ON harness_pairing_intents(project_id,updated_at DESC,id);
CREATE INDEX harness_pairing_intents_connection_updated ON harness_pairing_intents(connection_id,updated_at DESC,id);
CREATE UNIQUE INDEX harness_pairing_intents_decision_request ON harness_pairing_intents(owner_principal,decision_request_key) WHERE decision_request_key<>'';
CREATE TRIGGER harness_pairing_intents_identity_immutable BEFORE UPDATE ON harness_pairing_intents WHEN
 NEW.id<>OLD.id OR NEW.project_id<>OLD.project_id OR NEW.kind<>OLD.kind OR NEW.connection_id<>OLD.connection_id OR NEW.installation_id<>OLD.installation_id OR
 NEW.adapter_digest<>OLD.adapter_digest OR NEW.harness_identity<>OLD.harness_identity OR NEW.provider_version<>OLD.provider_version OR
 NEW.protocol_fingerprint<>OLD.protocol_fingerprint OR NEW.mission_id<>OLD.mission_id OR NEW.app_run_id<>OLD.app_run_id OR
 NEW.capability_classes<>OLD.capability_classes OR NEW.expected_generation<>OLD.expected_generation OR NEW.connection_expires_at<>OLD.connection_expires_at OR
 NEW.expires_at<>OLD.expires_at OR NEW.digest<>OLD.digest OR NEW.created_at<>OLD.created_at
 BEGIN SELECT RAISE(ABORT,'harness pairing intent identity is immutable'); END;
CREATE TABLE harness_authority_receipts (
 id TEXT PRIMARY KEY, action TEXT NOT NULL CHECK(action IN ('approve','deny','revoke')), target_type TEXT NOT NULL CHECK(target_type IN ('pairing_intent','harness_connection')),
 target_id TEXT NOT NULL, target_digest TEXT NOT NULL CHECK(length(target_digest)=64), expected_generation INTEGER NOT NULL DEFAULT 0,
 request_key TEXT NOT NULL, request_fingerprint TEXT NOT NULL CHECK(length(request_fingerprint)=64), owner_principal TEXT NOT NULL,
 confirmation_ref TEXT NOT NULL, created_at TIMESTAMP NOT NULL, UNIQUE(owner_principal,request_key)
);
CREATE TRIGGER harness_authority_receipts_immutable BEFORE UPDATE ON harness_authority_receipts BEGIN SELECT RAISE(ABORT,'harness authority receipt is immutable'); END;
-- +goose Down
DROP TRIGGER IF EXISTS command_authority_claims_immutable;
CREATE TRIGGER command_authority_claims_immutable BEFORE UPDATE ON command_authority_claims
WHEN NEW.id<>OLD.id OR NEW.adapter_request_key<>OLD.adapter_request_key OR NEW.request_fingerprint<>OLD.request_fingerprint OR NEW.owner_proof_id<>OLD.owner_proof_id OR NEW.harness_connection_id<>OLD.harness_connection_id OR NEW.connection_generation<>OLD.connection_generation OR NEW.connection_binding_digest<>OLD.connection_binding_digest OR NEW.connection_expires_at<>OLD.connection_expires_at OR NEW.connection_revoked_at IS NOT OLD.connection_revoked_at OR NEW.transport_class<>OLD.transport_class OR NEW.app_run_id<>OLD.app_run_id OR NEW.mission_id<>OLD.mission_id OR NEW.content_digest<>OLD.content_digest OR NEW.target_digest<>OLD.target_digest OR NEW.owner_class<>OLD.owner_class OR NEW.canonical_version<>OLD.canonical_version OR NEW.canonical_payload<>OLD.canonical_payload OR NEW.destination_type<>OLD.destination_type OR NEW.destination_id<>OLD.destination_id OR NEW.created_at<>OLD.created_at
BEGIN SELECT RAISE(ABORT,'command authority claim identity is immutable'); END;
DROP TRIGGER IF EXISTS harness_authority_receipts_immutable;
DROP TABLE IF EXISTS harness_authority_receipts;
DROP TRIGGER IF EXISTS harness_pairing_intents_identity_immutable;
DROP INDEX IF EXISTS harness_pairing_intents_decision_request;
DROP INDEX IF EXISTS harness_pairing_intents_connection_updated;
DROP INDEX IF EXISTS harness_pairing_intents_project_updated;
DROP TABLE IF EXISTS harness_pairing_intents;
