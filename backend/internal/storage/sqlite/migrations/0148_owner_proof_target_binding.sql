-- Replace the lossy target id/int generation tuple with one exact typed-target digest.
-- Pending proofs are intentionally invalidated across this upgrade: proof bearers are
-- short-lived caller-only capabilities and cannot be reconstructed safely.
-- +goose Up
DROP TRIGGER IF EXISTS owner_proofs_identity_immutable;
DROP TABLE IF EXISTS owner_proofs;
CREATE TABLE owner_proofs(id TEXT PRIMARY KEY,verifier TEXT NOT NULL CHECK(length(verifier)=64),app_run_id TEXT NOT NULL,mission_id TEXT NOT NULL,content_digest TEXT NOT NULL CHECK(length(content_digest)=64),target_digest TEXT NOT NULL CHECK(length(target_digest)=64),command_class TEXT NOT NULL CHECK(command_class IN ('turn','steer','answer','interrupt','cancel','replace','approval','accept')),confirmation_ref TEXT NOT NULL DEFAULT '',expires_at TIMESTAMP NOT NULL,created_at TIMESTAMP NOT NULL,consumed_at TIMESTAMP);
CREATE TRIGGER owner_proofs_identity_immutable BEFORE UPDATE ON owner_proofs WHEN NEW.id<>OLD.id OR NEW.verifier<>OLD.verifier OR NEW.app_run_id<>OLD.app_run_id OR NEW.mission_id<>OLD.mission_id OR NEW.content_digest<>OLD.content_digest OR NEW.target_digest<>OLD.target_digest OR NEW.command_class<>OLD.command_class OR NEW.confirmation_ref<>OLD.confirmation_ref OR NEW.expires_at<>OLD.expires_at OR NEW.created_at<>OLD.created_at BEGIN SELECT RAISE(ABORT,'owner proof identity is immutable'); END;
CREATE TABLE owner_answer_questions(
 id TEXT PRIMARY KEY,
 conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
 request_id TEXT NOT NULL,
 generation TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('pending','resolved','failed')),
 created_at TIMESTAMP NOT NULL,
 updated_at TIMESTAMP NOT NULL,
 UNIQUE(conversation_id,request_id,generation)
);
CREATE INDEX owner_answer_questions_pending ON owner_answer_questions(conversation_id,request_id,status);
CREATE TRIGGER owner_answer_questions_identity_immutable BEFORE UPDATE ON owner_answer_questions
WHEN NEW.id<>OLD.id OR NEW.conversation_id<>OLD.conversation_id OR NEW.request_id<>OLD.request_id OR NEW.generation<>OLD.generation OR NEW.created_at<>OLD.created_at
BEGIN SELECT RAISE(ABORT,'owner answer question identity is immutable'); END;
CREATE TABLE command_authority_claims(
 id TEXT PRIMARY KEY, adapter_request_key TEXT NOT NULL, request_fingerprint TEXT NOT NULL,
 owner_proof_id TEXT NOT NULL UNIQUE REFERENCES owner_proofs(id),
 harness_connection_id TEXT NOT NULL REFERENCES harness_connections(id), connection_generation INTEGER NOT NULL,
 connection_binding_digest TEXT NOT NULL CHECK(length(connection_binding_digest)=64), transport_class TEXT NOT NULL,
 app_run_id TEXT NOT NULL, mission_id TEXT NOT NULL, content_digest TEXT NOT NULL CHECK(length(content_digest)=64),
 target_digest TEXT NOT NULL CHECK(length(target_digest)=64), owner_class TEXT NOT NULL,
 canonical_version TEXT NOT NULL, canonical_payload BLOB NOT NULL, destination_type TEXT NOT NULL, destination_id TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('pending','action_needed')), created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL,
 UNIQUE(harness_connection_id,connection_generation,adapter_request_key)
);
CREATE TRIGGER command_authority_claims_immutable BEFORE UPDATE ON command_authority_claims
WHEN NEW.id<>OLD.id OR NEW.adapter_request_key<>OLD.adapter_request_key OR NEW.request_fingerprint<>OLD.request_fingerprint OR NEW.owner_proof_id<>OLD.owner_proof_id OR NEW.harness_connection_id<>OLD.harness_connection_id OR NEW.connection_generation<>OLD.connection_generation OR NEW.connection_binding_digest<>OLD.connection_binding_digest OR NEW.transport_class<>OLD.transport_class OR NEW.app_run_id<>OLD.app_run_id OR NEW.mission_id<>OLD.mission_id OR NEW.content_digest<>OLD.content_digest OR NEW.target_digest<>OLD.target_digest OR NEW.owner_class<>OLD.owner_class OR NEW.canonical_version<>OLD.canonical_version OR NEW.canonical_payload<>OLD.canonical_payload OR NEW.destination_type<>OLD.destination_type OR NEW.destination_id<>OLD.destination_id OR NEW.created_at<>OLD.created_at
BEGIN SELECT RAISE(ABORT,'command authority claim identity is immutable'); END;
CREATE TABLE harness_command_outbox(
 claim_id TEXT PRIMARY KEY REFERENCES command_authority_claims(id), destination_type TEXT NOT NULL,
 destination_id TEXT NOT NULL, canonical_payload BLOB NOT NULL, state TEXT NOT NULL CHECK(state IN ('pending','action_needed')),
 created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL
);
CREATE TRIGGER harness_command_outbox_identity_immutable BEFORE UPDATE ON harness_command_outbox
WHEN NEW.claim_id<>OLD.claim_id OR NEW.destination_type<>OLD.destination_type OR NEW.destination_id<>OLD.destination_id OR NEW.canonical_payload<>OLD.canonical_payload OR NEW.created_at<>OLD.created_at
BEGIN SELECT RAISE(ABORT,'command outbox identity is immutable'); END;
-- +goose Down
DROP TRIGGER IF EXISTS harness_command_outbox_identity_immutable;
DROP TABLE IF EXISTS harness_command_outbox;
DROP TRIGGER IF EXISTS command_authority_claims_immutable;
DROP TABLE IF EXISTS command_authority_claims;
DROP TRIGGER IF EXISTS owner_answer_questions_identity_immutable;
DROP INDEX IF EXISTS owner_answer_questions_pending;
DROP TABLE IF EXISTS owner_answer_questions;
DROP TRIGGER IF EXISTS owner_proofs_identity_immutable;
DROP TABLE IF EXISTS owner_proofs;
CREATE TABLE owner_proofs(id TEXT PRIMARY KEY,verifier TEXT NOT NULL CHECK(length(verifier)=64),app_run_id TEXT NOT NULL,mission_id TEXT NOT NULL,content_digest TEXT NOT NULL CHECK(length(content_digest)=64),target_id TEXT NOT NULL,target_generation INTEGER NOT NULL CHECK(target_generation>=1),command_class TEXT NOT NULL CHECK(command_class IN ('turn','steer','answer','interrupt','cancel','replace','approval','accept')),confirmation_ref TEXT NOT NULL DEFAULT '',expires_at TIMESTAMP NOT NULL,created_at TIMESTAMP NOT NULL,consumed_at TIMESTAMP);
CREATE TRIGGER owner_proofs_identity_immutable BEFORE UPDATE ON owner_proofs WHEN NEW.id<>OLD.id OR NEW.verifier<>OLD.verifier OR NEW.app_run_id<>OLD.app_run_id OR NEW.mission_id<>OLD.mission_id OR NEW.content_digest<>OLD.content_digest OR NEW.target_id<>OLD.target_id OR NEW.target_generation<>OLD.target_generation OR NEW.command_class<>OLD.command_class OR NEW.confirmation_ref<>OLD.confirmation_ref OR NEW.expires_at<>OLD.expires_at OR NEW.created_at<>OLD.created_at BEGIN SELECT RAISE(ABORT,'owner proof identity is immutable'); END;
