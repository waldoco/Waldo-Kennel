-- Local approval custody and per-device revocation. No owner token is stored.
-- +goose Up
-- +goose StatementBegin
CREATE TABLE device_bridge_revoked_identities (
 owner_id TEXT NOT NULL, device_id TEXT NOT NULL REFERENCES device_bridge_devices(device_id),
 generation INTEGER NOT NULL, revoked_at TIMESTAMP NOT NULL, PRIMARY KEY(owner_id,device_id)
);
INSERT INTO device_bridge_revoked_identities SELECT owner_id,device_id,claim_generation,updated_at FROM device_bridge_activations WHERE revoked=1;
CREATE TRIGGER device_bridge_revoked_identities_no_update BEFORE UPDATE ON device_bridge_revoked_identities BEGIN SELECT RAISE(ABORT,'revoked device retained'); END;
CREATE TRIGGER device_bridge_revoked_identities_no_delete BEFORE DELETE ON device_bridge_revoked_identities BEGIN SELECT RAISE(ABORT,'revoked device retained'); END;
DROP TRIGGER device_bridge_activations_fence_monotonic;
CREATE TRIGGER device_bridge_activations_fence_monotonic BEFORE UPDATE ON device_bridge_activations
WHEN NEW.claim_generation < OLD.claim_generation OR NEW.owner_id<>OLD.owner_id OR NEW.created_at<>OLD.created_at
 OR (OLD.revoked=1 AND NEW.revoked=0 AND NOT EXISTS(SELECT 1 FROM device_bridge_revoked_identities WHERE owner_id=OLD.owner_id AND device_id=OLD.device_id))
 OR (NEW.phase='paired' AND EXISTS(SELECT 1 FROM device_bridge_revoked_identities WHERE owner_id=NEW.owner_id AND device_id=NEW.device_id))
BEGIN SELECT RAISE(ABORT,'device bridge activation fence is monotonic'); END;
CREATE TRIGGER device_bridge_device_no_resurrection BEFORE UPDATE ON device_bridge_devices WHEN OLD.state='revoked' AND NEW.state<>'revoked'
BEGIN SELECT RAISE(ABORT,'revoked device cannot be resurrected'); END;
CREATE TABLE mac_action_requests (
 device_id TEXT NOT NULL REFERENCES device_bridge_devices(device_id), command_id TEXT NOT NULL,
 owner_id TEXT NOT NULL, fingerprint TEXT NOT NULL, project_id TEXT NOT NULL REFERENCES projects(id),
 request TEXT NOT NULL CHECK(json_valid(request)), expires_at INTEGER NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('pending','approved','denied','started','finished')),
 started_at INTEGER, result TEXT CHECK(result IS NULL OR json_valid(result)), local_output BLOB,
 PRIMARY KEY(device_id,command_id),
 CHECK(state<>'started' OR started_at IS NOT NULL)
);
CREATE TRIGGER mac_action_no_delete BEFORE DELETE ON mac_action_requests BEGIN SELECT RAISE(ABORT,'Mac action custody retained'); END;
CREATE TRIGGER mac_action_binding BEFORE UPDATE ON mac_action_requests
WHEN OLD.device_id<>NEW.device_id OR OLD.command_id<>NEW.command_id OR OLD.owner_id<>NEW.owner_id OR OLD.fingerprint<>NEW.fingerprint OR OLD.request<>NEW.request OR OLD.expires_at<>NEW.expires_at OR OLD.project_id<>NEW.project_id
 OR (OLD.state='finished' AND (NEW.state<>'finished' OR OLD.result IS NOT NEW.result OR OLD.local_output IS NOT NEW.local_output))
 OR (OLD.started_at IS NOT NULL AND NEW.started_at IS NOT OLD.started_at)
BEGIN SELECT RAISE(ABORT,'Mac action binding immutable'); END;
-- +goose StatementEnd
-- +goose StatementBegin
-- Existing databases may retain historical triggers whose referenced columns
-- were burned by older migrations. Do not revalidate unrelated trigger bodies
-- during this rename; all change_log writers are detached explicitly below.
PRAGMA legacy_alter_table=ON;
DROP TRIGGER IF EXISTS agent_switches_cdc_insert;
DROP TRIGGER IF EXISTS agent_switches_cdc_update;
DROP TRIGGER IF EXISTS conversation_activities_cdc_insert;
DROP TRIGGER IF EXISTS conversation_activities_cdc_update;
DROP TRIGGER IF EXISTS conversation_messages_cdc_insert;
DROP TRIGGER IF EXISTS conversation_messages_cdc_update;
DROP TRIGGER IF EXISTS conversation_turns_cdc_update;
DROP TRIGGER IF EXISTS pr_cdc_insert;
DROP TRIGGER IF EXISTS pr_cdc_update;
DROP TRIGGER IF EXISTS pr_checks_cdc_insert;
DROP TRIGGER IF EXISTS pr_checks_cdc_update;
DROP TRIGGER IF EXISTS pr_review_threads_cdc_insert;
DROP TRIGGER IF EXISTS pr_review_threads_cdc_update;
DROP TRIGGER IF EXISTS pr_session_cdc_update;
DROP TRIGGER IF EXISTS session_cleanup_facts_cdc_insert;
DROP TRIGGER IF EXISTS session_cleanup_facts_cdc_update;
DROP TRIGGER IF EXISTS session_interface_transitions_cdc_insert;
DROP TRIGGER IF EXISTS session_interface_transitions_cdc_update;
DROP TRIGGER IF EXISTS sessions_cdc_insert;
DROP TRIGGER IF EXISTS sessions_cdc_update;
DROP TRIGGER IF EXISTS usage_bindings_cdc_insert;
DROP TRIGGER IF EXISTS usage_bindings_cdc_update;
DROP TRIGGER IF EXISTS usage_sources_cdc_update;
DROP TRIGGER IF EXISTS responsibility_outcomes_cdc_insert;
DROP TRIGGER IF EXISTS responsibility_outcomes_cdc_update;
DROP TRIGGER IF EXISTS responsibility_contract_revisions_cdc_insert;
DROP TRIGGER IF EXISTS outcome_plans_cdc_insert;
DROP TRIGGER IF EXISTS outcome_plans_cdc_update;
DROP TRIGGER IF EXISTS attempts_cdc_insert;
DROP TRIGGER IF EXISTS attempts_cdc_update;
DROP TRIGGER IF EXISTS attempt_sessions_cdc_insert;
DROP TRIGGER IF EXISTS attempt_observations_cdc_insert;
DROP TRIGGER IF EXISTS attempt_recovery_receipts_cdc_insert;
DROP TRIGGER IF EXISTS evidence_items_cdc_insert;
DROP TRIGGER IF EXISTS verification_runs_cdc_insert;
DROP TRIGGER IF EXISTS acceptance_decisions_cdc_insert;
DROP TRIGGER IF EXISTS outcome_corrections_cdc_insert;
DROP TRIGGER IF EXISTS contribution_links_cdc_insert;
DROP TRIGGER IF EXISTS decomposition_revisions_cdc_insert;
DROP TRIGGER IF EXISTS decomposition_revisions_cdc_update;
DROP TRIGGER IF EXISTS contribution_waivers_cdc_insert;
DROP TRIGGER IF EXISTS decomposition_requests_cdc_insert;
DROP TRIGGER IF EXISTS decomposition_requests_cdc_update;
DROP TRIGGER IF EXISTS intake_sessions_cdc_insert;
DROP TRIGGER IF EXISTS intake_sessions_cdc_update;
DROP TRIGGER IF EXISTS intake_proposals_cdc_insert;
DROP TRIGGER IF EXISTS intake_confirmations_cdc_insert;
DROP TRIGGER IF EXISTS responsibility_links_cdc_insert;
DROP TRIGGER IF EXISTS responsibility_links_cdc_update;
DROP TRIGGER IF EXISTS waldo_conversations_cdc_insert;
DROP TRIGGER IF EXISTS waldo_conversation_episodes_cdc_insert;
DROP TRIGGER IF EXISTS waldo_conversation_episodes_cdc_update;
DROP TRIGGER IF EXISTS waldo_conversation_turns_cdc_insert;
DROP TRIGGER IF EXISTS waldo_context_attachments_cdc_insert;
DROP TRIGGER IF EXISTS waldo_context_attachments_cdc_update;
DROP TRIGGER IF EXISTS waldo_continuation_operations_cdc_insert;
DROP TRIGGER IF EXISTS waldo_continuation_operations_cdc_update;
DROP TRIGGER IF EXISTS waldo_continuation_receipts_cdc_insert;
DROP TRIGGER IF EXISTS project_brief_revisions_cdc_insert;
DROP TRIGGER IF EXISTS outcome_run_intents_cdc_insert;
DROP TRIGGER IF EXISTS outcome_run_intents_cdc_acknowledged;
DROP TRIGGER IF EXISTS outcome_run_intents_cdc_failure;
DROP TRIGGER IF EXISTS outcome_deliveries_cdc_insert;
DROP TRIGGER IF EXISTS outcome_deliveries_cdc_update;
DROP TRIGGER IF EXISTS planning_sessions_cdc_insert;
DROP TRIGGER IF EXISTS planning_sessions_cdc_update;
DROP TRIGGER IF EXISTS owner_answer_questions_needs_you_insert;
DROP TRIGGER IF EXISTS owner_answer_questions_needs_you_update;
DROP TRIGGER IF EXISTS governed_controls_needs_you_insert;
DROP TRIGGER IF EXISTS governed_controls_needs_you_update;
DROP TRIGGER IF EXISTS mac_approval_insert;
DROP TRIGGER IF EXISTS mac_approval_update;
CREATE TABLE change_log_mac (
 seq INTEGER PRIMARY KEY AUTOINCREMENT, project_id TEXT NOT NULL REFERENCES projects(id), session_id TEXT REFERENCES sessions(id),
 event_type TEXT NOT NULL CHECK(event_type IN ('session_created','session_updated','pr_created','pr_updated','pr_check_recorded','pr_session_changed','pr_review_thread_added','pr_review_thread_resolved','outcome_created','outcome_updated','outcome_contract_revised','outcome_plan_proposed','outcome_plan_approved','outcome_attempt_started','outcome_attempt_updated','outcome_attempt_session_bound','outcome_attempt_observed','outcome_attempt_recovered','outcome_evidence_recorded','outcome_verification_recorded','outcome_acceptance_decided','outcome_correction_recorded','intake_captured','intake_updated','intake_proposal_revised','intake_confirmed','responsibility_link_created','responsibility_link_ended','waldo_conversation_created','waldo_conversation_episode_opened','waldo_conversation_episode_sealed','waldo_conversation_turn_appended','waldo_conversation_context_attached','waldo_conversation_context_detached','waldo_conversation_continuation_prepared','waldo_conversation_continuation_progressed','waldo_conversation_continuation_recorded','outcome_contribution_bound','outcome_decomposition_proposed','outcome_decomposition_authorized','outcome_contribution_dependency_waived','outcome_decomposition_requested','outcome_decomposition_request_answered','project_brief_revised','outcome_run_intent_changed','outcome_attempt_retained','outcome_delivery_changed','harness_pairing_intent_changed','harness_connection_changed','mac_approval_changed')),
 payload TEXT NOT NULL CHECK(json_valid(payload)), created_at TIMESTAMP NOT NULL DEFAULT(datetime('now')));
INSERT INTO change_log_mac SELECT * FROM change_log;
DROP INDEX IF EXISTS idx_change_log_project;
DROP TABLE change_log;
ALTER TABLE change_log_mac RENAME TO change_log;
CREATE INDEX idx_change_log_project ON change_log(project_id,seq);
PRAGMA legacy_alter_table=OFF;

CREATE TRIGGER mac_approval_insert AFTER INSERT ON mac_action_requests BEGIN
 INSERT INTO change_log(project_id,event_type,payload) VALUES(NEW.project_id,'mac_approval_changed',json_object('device_id',NEW.device_id,'command_id',NEW.command_id,'kind','approval','state',NEW.state));
END;
CREATE TRIGGER mac_approval_update AFTER UPDATE ON mac_action_requests WHEN OLD.state<>NEW.state BEGIN
 INSERT INTO change_log(project_id,event_type,payload) VALUES(NEW.project_id,'mac_approval_changed',json_object('device_id',NEW.device_id,'command_id',NEW.command_id,'kind','approval','state',NEW.state));
END;
-- +goose StatementEnd

-- +goose StatementBegin
-- Observe status transitions in the same transaction as the existing Session
-- controller. The queue contains status only; output is never pushed.
CREATE TABLE device_bridge_session_events (
 seq INTEGER PRIMARY KEY AUTOINCREMENT, device_id TEXT NOT NULL REFERENCES device_bridge_devices(device_id),
 session_id TEXT NOT NULL REFERENCES sessions(id), project_id TEXT NOT NULL REFERENCES projects(id),
 event TEXT NOT NULL CHECK(event IN ('needs_input','blocked_on_tool','turn_finished','exited')),
 spooled INTEGER NOT NULL DEFAULT 0 CHECK(spooled IN (0,1))
);
CREATE TRIGGER bridge_session_status AFTER UPDATE ON sessions
WHEN (OLD.activity_state<>NEW.activity_state OR OLD.is_terminated<>NEW.is_terminated)
 AND NOT ((OLD.is_terminated=1 OR OLD.activity_state='exited') AND (NEW.is_terminated=1 OR NEW.activity_state='exited'))
 AND NEW.harness IN ('claude-code','codex')
 AND (NEW.activity_state IN ('waiting_input','blocked','exited') OR NEW.is_terminated=1 OR (OLD.activity_state='active' AND NEW.activity_state='idle'))
BEGIN
 INSERT INTO device_bridge_session_events(device_id,session_id,project_id,event)
 SELECT d.device_id,NEW.id,NEW.project_id,
 CASE WHEN NEW.is_terminated=1 OR NEW.activity_state='exited' THEN 'exited'
 WHEN NEW.activity_state='blocked' THEN 'blocked_on_tool'
 WHEN NEW.activity_state='waiting_input' THEN 'needs_input' ELSE 'turn_finished' END
 FROM device_bridge_devices d WHERE d.state='paired' AND d.contract_version='0.3.0'
 AND EXISTS(SELECT 1 FROM json_each(d.capability_classes) WHERE value IN ('session_query','session_control'));
END;
-- +goose StatementEnd

-- +goose Down
-- Custody and revoked identities are permanent evidence; no automatic rollback.
