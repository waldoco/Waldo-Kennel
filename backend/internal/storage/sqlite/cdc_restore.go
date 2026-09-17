package sqlite

import (
	"database/sql"
	"fmt"
)

// changeLogWriters lists every CDC trigger the shipped schema attaches to
// tables that write change_log, in their latest migrated form. Migration 0099
// rebuilds the checked change_log relation and must detach these writers
// first; restoring them inside that same SQL file breaks degraded profiles
// whose skipped migrations have not physically created every subject table.
//
// restoreChangeLogWriters therefore runs from migrate() after goose completes:
// it recreates each writer only when the trigger is absent AND its subject
// table exists, healing clean databases, upgraded Kennel-derived databases, and
// burned/skipped-migration profiles alike. Bodies must be kept verbatim with
// their defining migration.
var changeLogWriters = []struct {
	name string
	// table is the writer's direct subject table.
	table string
	// deps lists every additional table the trigger body touches. The writer
	// is restored only when the subject table and every dependency physically
	// exist, so partially migrated profiles never carry half-built triggers.
	deps []string
	// columns lists subject-table columns introduced after the table itself.
	// Degraded or intentionally partial migration tests must not receive a
	// trigger that references columns their schema does not yet have.
	columns []string
	// since is the migration filename prefix that introduced this writer, for
	// writers added after the 0106 rebuild-detach guard began. A rebuild
	// migration cannot be expected to detach a writer that did not exist when
	// it shipped, and merged migrations must never be edited to add one.
	// Empty means the writer predates the guard and every rebuild must drop it.
	since string
	sql   string
}{
	{
		name:  "agent_switches_cdc_insert",
		table: "agent_switches",
		sql:   "CREATE TRIGGER agent_switches_cdc_insert\nAFTER INSERT ON agent_switches\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT project_id FROM sessions WHERE id = NEW.session_id),\n        NEW.session_id, 'session_updated', json_object('id', NEW.session_id), NEW.updated_at\n    );\nEND;",
	},
	{
		name:  "agent_switches_cdc_update",
		table: "agent_switches",
		sql:   "CREATE TRIGGER agent_switches_cdc_update\nAFTER UPDATE ON agent_switches\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT project_id FROM sessions WHERE id = NEW.session_id),\n        NEW.session_id, 'session_updated', json_object('id', NEW.session_id), NEW.updated_at\n    );\nEND;",
	},
	{
		name:  "conversation_activities_cdc_insert",
		table: "conversation_activities",
		sql:   "CREATE TRIGGER conversation_activities_cdc_insert\nAFTER INSERT ON conversation_activities\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    SELECT s.project_id, s.id, 'session_updated',\n\t\t   json_object('id', s.id, 'sessionId', s.id, 'conversationId', c.id,\n\t\t\t\t\t   'activity', s.activity_state,\n                       'isTerminated', json(CASE WHEN s.is_terminated THEN 'true' ELSE 'false' END)),\n           NEW.updated_at\n    FROM conversations c\n    JOIN sessions s ON s.id = c.current_session_id\n    WHERE c.id = NEW.conversation_id;\nEND;",
	},
	{
		name:  "conversation_activities_cdc_update",
		table: "conversation_activities",
		sql:   "CREATE TRIGGER conversation_activities_cdc_update\nAFTER UPDATE ON conversation_activities\nWHEN OLD.revision <> NEW.revision\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    SELECT s.project_id, s.id, 'session_updated',\n\t\t   json_object('id', s.id, 'sessionId', s.id, 'conversationId', c.id,\n\t\t\t\t\t   'activity', s.activity_state,\n                       'isTerminated', json(CASE WHEN s.is_terminated THEN 'true' ELSE 'false' END)),\n           NEW.updated_at\n    FROM conversations c\n    JOIN sessions s ON s.id = c.current_session_id\n    WHERE c.id = NEW.conversation_id;\nEND;",
	},
	{
		name:  "conversation_messages_cdc_insert",
		table: "conversation_messages",
		sql:   "CREATE TRIGGER conversation_messages_cdc_insert\nAFTER INSERT ON conversation_messages\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    SELECT s.project_id, s.id, 'session_updated',\n\t\t   json_object('id', s.id, 'sessionId', s.id, 'conversationId', NEW.conversation_id,\n\t\t\t\t\t   'activity', s.activity_state,\n                       'isTerminated', json(CASE WHEN s.is_terminated THEN 'true' ELSE 'false' END)),\n           NEW.updated_at\n    FROM conversations c\n\tJOIN sessions s ON s.id = c.current_session_id\n    WHERE c.id = NEW.conversation_id;\nEND;",
	},
	{
		name:  "conversation_messages_cdc_update",
		table: "conversation_messages",
		sql:   "CREATE TRIGGER conversation_messages_cdc_update\nAFTER UPDATE ON conversation_messages\nWHEN OLD.revision <> NEW.revision\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    SELECT s.project_id, s.id, 'session_updated',\n           json_object('id', s.id, 'sessionId', s.id, 'conversationId', c.id,\n\t\t\t\t\t   'activity', s.activity_state,\n                       'isTerminated', json(CASE WHEN s.is_terminated THEN 'true' ELSE 'false' END)),\n           NEW.updated_at\n    FROM conversations c\n    JOIN sessions s ON s.id = c.current_session_id\n    WHERE c.id = NEW.conversation_id;\nEND;",
	},
	{
		name:  "conversation_turns_cdc_update",
		table: "conversation_turns",
		sql:   "CREATE TRIGGER conversation_turns_cdc_update\nAFTER UPDATE ON conversation_turns\nWHEN OLD.state <> NEW.state\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    SELECT s.project_id, s.id, 'session_updated',\n\t\t   json_object('id', s.id, 'sessionId', s.id, 'conversationId', NEW.conversation_id,\n\t\t\t\t\t   'activity', s.activity_state,\n                       'isTerminated', json(CASE WHEN s.is_terminated THEN 'true' ELSE 'false' END)),\n           COALESCE(NEW.completed_at, NEW.started_at, NEW.requested_at)\n    FROM sessions s\n    WHERE s.id = NEW.handled_by_session_id;\nEND;",
	},
	{
		name:  "pr_cdc_insert",
		table: "pr",
		sql:   "CREATE TRIGGER pr_cdc_insert\nAFTER INSERT ON pr\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES ((SELECT project_id FROM sessions WHERE id = NEW.session_id), NEW.session_id, 'pr_created',\n        json_object('url', NEW.url, 'session', NEW.session_id, 'state', NEW.pr_state,\n                    'ci', NEW.ci_state, 'review', NEW.review_decision, 'mergeability', NEW.mergeability),\n        NEW.updated_at);\nEND;",
	},
	{
		name:  "pr_cdc_update",
		table: "pr",
		sql:   "CREATE TRIGGER pr_cdc_update\nAFTER UPDATE ON pr\nWHEN OLD.pr_state <> NEW.pr_state\n    OR OLD.ci_state <> NEW.ci_state\n    OR OLD.review_decision <> NEW.review_decision\n    OR OLD.mergeability <> NEW.mergeability\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES ((SELECT project_id FROM sessions WHERE id = NEW.session_id), NEW.session_id, 'pr_updated',\n        json_object('url', NEW.url, 'session', NEW.session_id, 'state', NEW.pr_state,\n                    'ci', NEW.ci_state, 'review', NEW.review_decision, 'mergeability', NEW.mergeability),\n        NEW.updated_at);\nEND;",
	},
	{
		name:  "pr_checks_cdc_insert",
		table: "pr_checks",
		sql:   "CREATE TRIGGER pr_checks_cdc_insert\nAFTER INSERT ON pr_checks\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id FROM pr p JOIN sessions s ON s.id = p.session_id WHERE p.url = NEW.pr_url),\n        (SELECT session_id FROM pr WHERE url = NEW.pr_url),\n        'pr_check_recorded',\n        json_object('pr', NEW.pr_url, 'name', NEW.name, 'commit', NEW.commit_hash, 'status', NEW.status),\n        NEW.created_at);\nEND;",
	},
	{
		name:  "pr_checks_cdc_update",
		table: "pr_checks",
		sql:   "CREATE TRIGGER pr_checks_cdc_update\nAFTER UPDATE ON pr_checks\nWHEN OLD.status <> NEW.status\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id FROM pr p JOIN sessions s ON s.id = p.session_id WHERE p.url = NEW.pr_url),\n        (SELECT session_id FROM pr WHERE url = NEW.pr_url),\n        'pr_check_recorded',\n        json_object('pr', NEW.pr_url, 'name', NEW.name, 'commit', NEW.commit_hash, 'status', NEW.status),\n        datetime('now'));\nEND;",
	},
	{
		name:  "pr_review_threads_cdc_insert",
		table: "pr_review_threads",
		sql:   "CREATE TRIGGER pr_review_threads_cdc_insert\nAFTER INSERT ON pr_review_threads\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id FROM pr p JOIN sessions s ON s.id = p.session_id WHERE p.url = NEW.pr_url),\n        (SELECT session_id FROM pr WHERE url = NEW.pr_url),\n        'pr_review_thread_added',\n        json_object(\n            'pr', NEW.pr_url,\n            'thread', NEW.thread_id,\n            'path', NEW.path,\n            'line', NEW.line,\n            'resolved', json(CASE WHEN NEW.resolved THEN 'true' ELSE 'false' END),\n            'isBot', json(CASE WHEN NEW.is_bot THEN 'true' ELSE 'false' END)\n        ),\n        NEW.updated_at);\nEND;",
	},
	{
		name:  "pr_review_threads_cdc_update",
		table: "pr_review_threads",
		sql:   "CREATE TRIGGER pr_review_threads_cdc_update\nAFTER UPDATE ON pr_review_threads\nWHEN OLD.resolved <> NEW.resolved\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id FROM pr p JOIN sessions s ON s.id = p.session_id WHERE p.url = NEW.pr_url),\n        (SELECT session_id FROM pr WHERE url = NEW.pr_url),\n        'pr_review_thread_resolved',\n        json_object(\n            'pr', NEW.pr_url,\n            'thread', NEW.thread_id,\n            'path', NEW.path,\n            'line', NEW.line,\n            'resolved', json(CASE WHEN NEW.resolved THEN 'true' ELSE 'false' END)\n        ),\n        NEW.updated_at);\nEND;",
	},
	{
		name:  "pr_session_cdc_update",
		table: "pr",
		sql:   "CREATE TRIGGER pr_session_cdc_update\nAFTER UPDATE ON pr\nWHEN OLD.session_id <> NEW.session_id\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT project_id FROM sessions WHERE id = NEW.session_id),\n        NEW.session_id,\n        'pr_session_changed',\n        json_object(\n            'url', NEW.url,\n            'fromSession', OLD.session_id,\n            'toSession', NEW.session_id),\n        NEW.updated_at);\nEND;",
	},
	{
		name:  "session_cleanup_facts_cdc_insert",
		table: "session_cleanup_facts",
		sql:   "CREATE TRIGGER session_cleanup_facts_cdc_insert\nAFTER INSERT ON session_cleanup_facts\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES ((SELECT project_id FROM sessions WHERE id = NEW.session_id), NEW.session_id, 'session_updated',\n        json_object('id', NEW.session_id),\n        datetime('now'));\nEND;",
	},
	{
		name:  "session_cleanup_facts_cdc_update",
		table: "session_cleanup_facts",
		sql:   "CREATE TRIGGER session_cleanup_facts_cdc_update\nAFTER UPDATE ON session_cleanup_facts\nWHEN OLD.workspace_disposition <> NEW.workspace_disposition\n    OR (OLD.runtime_released_at IS NULL) <> (NEW.runtime_released_at IS NULL)\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES ((SELECT project_id FROM sessions WHERE id = NEW.session_id), NEW.session_id, 'session_updated',\n        json_object('id', NEW.session_id),\n        datetime('now'));\nEND;",
	},
	{
		name:  "session_interface_transitions_cdc_insert",
		table: "session_interface_transitions",
		sql:   "CREATE TRIGGER session_interface_transitions_cdc_insert\nAFTER INSERT ON session_interface_transitions\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    SELECT s.project_id, s.id, 'session_updated',\n           json_object('id', s.id, 'sessionId', s.id,\n                       'interfaceTransitionId', NEW.id,\n                       'interfaceTransitionPhase', NEW.phase,\n                       'activity', s.activity_state,\n                       'isTerminated', json(CASE WHEN s.is_terminated THEN 'true' ELSE 'false' END)),\n           NEW.updated_at\n    FROM sessions s WHERE s.id = NEW.session_id;\nEND;",
	},
	{
		name:  "session_interface_transitions_cdc_update",
		table: "session_interface_transitions",
		sql:   "CREATE TRIGGER session_interface_transitions_cdc_update\nAFTER UPDATE ON session_interface_transitions\nWHEN OLD.phase <> NEW.phase\n    OR OLD.error_code <> NEW.error_code\n    OR OLD.error_detail <> NEW.error_detail\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    SELECT s.project_id, s.id, 'session_updated',\n           json_object('id', s.id, 'sessionId', s.id,\n                       'interfaceTransitionId', NEW.id,\n                       'interfaceTransitionPhase', NEW.phase,\n                       'activity', s.activity_state,\n                       'isTerminated', json(CASE WHEN s.is_terminated THEN 'true' ELSE 'false' END)),\n           NEW.updated_at\n    FROM sessions s WHERE s.id = NEW.session_id;\nEND;",
	},
	{
		name:  "sessions_cdc_insert",
		table: "sessions",
		sql:   "CREATE TRIGGER sessions_cdc_insert\nAFTER INSERT ON sessions\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (NEW.project_id, NEW.id, 'session_created',\n        json_object('id', NEW.id, 'activity', NEW.activity_state, 'isTerminated', json(CASE WHEN NEW.is_terminated THEN 'true' ELSE 'false' END)),\n        NEW.updated_at);\nEND;",
	},
	{
		name:  "sessions_cdc_update",
		table: "sessions",
		sql:   "CREATE TRIGGER sessions_cdc_update\nAFTER UPDATE ON sessions\nWHEN OLD.activity_state <> NEW.activity_state\n    OR OLD.is_terminated <> NEW.is_terminated\n    OR (OLD.first_signal_at IS NULL AND NEW.first_signal_at IS NOT NULL)\n    OR OLD.preview_url <> NEW.preview_url\n    OR OLD.preview_revision <> NEW.preview_revision\n    OR OLD.display_name <> NEW.display_name\n    OR OLD.terminate_on_pr_merge <> NEW.terminate_on_pr_merge\n    OR OLD.is_pinned <> NEW.is_pinned\n    OR OLD.pinned_at <> NEW.pinned_at\n    OR (OLD.pinned_at IS NULL AND NEW.pinned_at IS NOT NULL)\n    OR (OLD.pinned_at IS NOT NULL AND NEW.pinned_at IS NULL)\n    OR OLD.session_mode <> NEW.session_mode\n    OR OLD.auto_inject_review <> NEW.auto_inject_review\n    OR OLD.auto_review_enabled <> NEW.auto_review_enabled\n    OR OLD.harness <> NEW.harness\n    OR OLD.runtime_launch_id <> NEW.runtime_launch_id\n    OR OLD.agent_session_id <> NEW.agent_session_id\n    OR OLD.native_transcript_path <> NEW.native_transcript_path\n    OR OLD.auto_inject_ci <> NEW.auto_inject_ci\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (NEW.project_id, NEW.id, 'session_updated',\n        json_object(\n            'id', NEW.id,\n            'activity', NEW.activity_state,\n            'isTerminated', json(CASE WHEN NEW.is_terminated THEN 'true' ELSE 'false' END),\n            'terminateOnPrMerge', json(CASE WHEN NEW.terminate_on_pr_merge THEN 'true' ELSE 'false' END),\n            'previewUrl', NEW.preview_url,\n            'previewRevision', NEW.preview_revision,\n            'isPinned', json(CASE WHEN NEW.is_pinned THEN 'true' ELSE 'false' END),\n            'mode', NEW.session_mode,\n            'autoInjectReview', json(CASE WHEN NEW.auto_inject_review THEN 'true' ELSE 'false' END),\n            'autoInjectCI', json(CASE WHEN NEW.auto_inject_ci THEN 'true' ELSE 'false' END),\n            'autoReviewEnabled', json(CASE WHEN NEW.auto_review_enabled THEN 'true' ELSE 'false' END)\n        ),\n        NEW.updated_at);\nEND;",
	},
	{
		name:  "usage_bindings_cdc_insert",
		table: "usage_bindings",
		sql:   "CREATE TRIGGER usage_bindings_cdc_insert AFTER INSERT ON usage_bindings BEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES ((SELECT project_id FROM sessions WHERE id = NEW.session_id),\n            NEW.session_id, 'session_updated', json_object('id', NEW.session_id), NEW.updated_at);\nEND;",
	},
	{
		name:  "usage_bindings_cdc_update",
		table: "usage_bindings",
		sql:   "CREATE TRIGGER usage_bindings_cdc_update AFTER UPDATE ON usage_bindings BEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES ((SELECT project_id FROM sessions WHERE id = NEW.session_id),\n            NEW.session_id, 'session_updated', json_object('id', NEW.session_id), NEW.updated_at);\nEND;",
	},
	{
		name:  "usage_sources_cdc_update",
		table: "usage_sources",
		sql:   "CREATE TRIGGER usage_sources_cdc_update AFTER UPDATE ON usage_sources\nWHEN OLD.anomaly_count IS NOT NEW.anomaly_count\n  OR OLD.last_error_code IS NOT NEW.last_error_code\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    SELECT s.project_id, ub.session_id, 'session_updated', json_object('id', ub.session_id), NEW.updated_at\n    FROM usage_bindings ub JOIN sessions s ON s.id = ub.session_id WHERE ub.id = NEW.binding_id;\nEND;",
	},
	// Outcome contract writers introduced by 0099. Migration 0100 also
	// rebuilds change_log and detaches these; like every writer above they are
	// restored here — never inside migration SQL — because skipped-migration
	// profiles may not have the subject tables when a later rebuild runs.
	{
		name:  "responsibility_outcomes_cdc_insert",
		table: "outcomes",
		deps:  []string{"responsibility_spaces"},
		sql:   "CREATE TRIGGER responsibility_outcomes_cdc_insert\nAFTER INSERT ON outcomes\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT project_id FROM responsibility_spaces WHERE id = NEW.space_id),\n        NULL,\n        'outcome_created',\n        json_object(\n            'id', NEW.id,\n            'spaceId', NEW.space_id,\n            'title', NEW.title,\n            'currentRevisionNumber', NEW.current_revision_number\n        ),\n        NEW.created_at);\nEND;",
	},
	{
		name:  "responsibility_outcomes_cdc_update",
		table: "outcomes",
		deps:  []string{"responsibility_spaces"},
		sql:   "CREATE TRIGGER responsibility_outcomes_cdc_update\nAFTER UPDATE ON outcomes\nWHEN OLD.title <> NEW.title\n     OR OLD.current_revision_number <> NEW.current_revision_number\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT project_id FROM responsibility_spaces WHERE id = NEW.space_id),\n        NULL,\n        'outcome_updated',\n        json_object(\n            'id', NEW.id,\n            'spaceId', NEW.space_id,\n            'title', NEW.title,\n            'previousRevisionNumber', OLD.current_revision_number,\n            'currentRevisionNumber', NEW.current_revision_number\n        ),\n        NEW.updated_at);\nEND;",
	},
	{
		name:  "responsibility_contract_revisions_cdc_insert",
		table: "contract_revisions",
		deps:  []string{"outcomes", "responsibility_spaces"},
		sql:   "CREATE TRIGGER responsibility_contract_revisions_cdc_insert\nAFTER INSERT ON contract_revisions\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id\n           FROM outcomes o\n           JOIN responsibility_spaces s ON s.id = o.space_id\n          WHERE o.id = NEW.outcome_id),\n        NULL,\n        'outcome_contract_revised',\n        json_object(\n            'revisionId', NEW.id,\n            'outcomeId', NEW.outcome_id,\n            'number', NEW.number,\n            'goal', NEW.goal\n        ),\n        NEW.created_at);\nEND;",
	},
	// Plan authority writers introduced by 0100.
	{
		name:  "outcome_plans_cdc_insert",
		table: "plan_revisions",
		deps:  []string{"outcomes", "responsibility_spaces"},
		sql:   "CREATE TRIGGER outcome_plans_cdc_insert\nAFTER INSERT ON plan_revisions\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id\n           FROM outcomes o\n           JOIN responsibility_spaces s ON s.id = o.space_id\n          WHERE o.id = NEW.outcome_id),\n        NULL,\n        'outcome_plan_proposed',\n        json_object(\n            'planId', NEW.id,\n            'outcomeId', NEW.outcome_id,\n            'number', NEW.number,\n            'contractRevisionNumber', NEW.contract_revision_number,\n            'runBriefCoreDigest', NEW.run_brief_core_digest\n        ),\n        NEW.created_at);\nEND;",
	},
	{
		name:  "outcome_plans_cdc_update",
		table: "plan_revisions",
		deps:  []string{"outcomes", "responsibility_spaces"},
		sql:   "CREATE TRIGGER outcome_plans_cdc_update\nAFTER UPDATE ON plan_revisions\nWHEN OLD.status <> NEW.status\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id\n           FROM outcomes o\n           JOIN responsibility_spaces s ON s.id = o.space_id\n          WHERE o.id = NEW.outcome_id),\n        NULL,\n        'outcome_plan_approved',\n        json_object(\n            'planId', NEW.id,\n            'outcomeId', NEW.outcome_id,\n            'number', NEW.number,\n            'previousStatus', OLD.status,\n            'status', NEW.status\n        ),\n        datetime('now'));\nEND;",
	},
	// Work attempt writers introduced by 0102 (#31). Migration 0102 rebuilds
	// change_log and detaches every writer; like all the above these are
	// restored here — never inside migration SQL — because skipped-migration
	// profiles may not have the outcomes tables their bodies join through.
	{
		name:  "attempts_cdc_insert",
		table: "attempts",
		deps:  []string{"outcomes", "responsibility_spaces"},
		sql:   "CREATE TRIGGER attempts_cdc_insert\nAFTER INSERT ON attempts\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id\n           FROM outcomes o\n           JOIN responsibility_spaces s ON s.id = o.space_id\n          WHERE o.id = NEW.outcome_id),\n        NULL,\n        'outcome_attempt_started',\n        json_object(\n            'attemptId', NEW.id,\n            'outcomeId', NEW.outcome_id,\n            'planId', NEW.plan_revision_id,\n            'workUnitId', NEW.work_unit_id,\n            'number', NEW.number,\n            'contractRevisionNumber', NEW.contract_revision_number,\n            'status', NEW.status\n        ),\n        NEW.created_at);\nEND;",
	},
	{
		name:  "attempts_cdc_update",
		table: "attempts",
		deps:  []string{"outcomes", "responsibility_spaces"},
		sql:   "CREATE TRIGGER attempts_cdc_update\nAFTER UPDATE ON attempts\nWHEN OLD.status <> NEW.status\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id\n           FROM outcomes o\n           JOIN responsibility_spaces s ON s.id = o.space_id\n          WHERE o.id = NEW.outcome_id),\n        NULL,\n        'outcome_attempt_updated',\n        json_object(\n            'attemptId', NEW.id,\n            'outcomeId', NEW.outcome_id,\n            'number', NEW.number,\n            'previousStatus', OLD.status,\n            'status', NEW.status\n        ),\n        datetime('now'));\nEND;",
	},
	{
		name:  "attempt_sessions_cdc_insert",
		table: "attempt_sessions",
		deps:  []string{"outcomes", "responsibility_spaces"},
		sql:   "CREATE TRIGGER attempt_sessions_cdc_insert\nAFTER INSERT ON attempt_sessions\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id\n           FROM attempts a\n           JOIN outcomes o ON o.id = a.outcome_id\n           JOIN responsibility_spaces s ON s.id = o.space_id\n          WHERE a.id = NEW.attempt_id),\n        NULL,\n        'outcome_attempt_session_bound',\n        json_object(\n            'attemptId', NEW.attempt_id,\n            'sessionId', NEW.session_id,\n            'seq', NEW.seq,\n            'harness', NEW.harness,\n            'mode', NEW.mode,\n            'runBriefCoreDigest', NEW.run_brief_core_digest,\n            'runBriefCompiledDigest', NEW.run_brief_compiled_digest\n        ),\n        NEW.bound_at);\nEND;",
	},
	{
		name:  "attempt_observations_cdc_insert",
		table: "attempt_observations",
		deps:  []string{"outcomes", "responsibility_spaces"},
		sql:   "CREATE TRIGGER attempt_observations_cdc_insert\nAFTER INSERT ON attempt_observations\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id\n           FROM attempts a\n           JOIN outcomes o ON o.id = a.outcome_id\n           JOIN responsibility_spaces s ON s.id = o.space_id\n          WHERE a.id = NEW.attempt_id),\n        NULL,\n        'outcome_attempt_observed',\n        json_object(\n            'attemptId', NEW.attempt_id,\n            'seq', NEW.seq,\n            'kind', NEW.kind\n        ),\n        NEW.created_at);\nEND;",
	},
	{
		name:  "attempt_recovery_receipts_cdc_insert",
		table: "attempt_recovery_receipts",
		deps:  []string{"outcomes", "responsibility_spaces"},
		sql:   "CREATE TRIGGER attempt_recovery_receipts_cdc_insert\nAFTER INSERT ON attempt_recovery_receipts\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id\n           FROM attempts a\n           JOIN outcomes o ON o.id = a.outcome_id\n           JOIN responsibility_spaces s ON s.id = o.space_id\n          WHERE a.id = NEW.attempt_id),\n        NULL,\n        'outcome_attempt_recovered',\n        json_object(\n            'attemptId', NEW.attempt_id,\n            'resolution', NEW.resolution,\n            'replacementAttemptId', NEW.replacement_attempt_id\n        ),\n        NEW.created_at);\nEND;",
	},
	// Proof and explicit owner-decision writers introduced by 0103 (#35).
	{
		name:  "evidence_items_cdc_insert",
		table: "evidence_items",
		deps:  []string{"outcomes", "responsibility_spaces"},
		sql:   "CREATE TRIGGER evidence_items_cdc_insert\nAFTER INSERT ON evidence_items\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id FROM outcomes o JOIN responsibility_spaces s ON s.id = o.space_id WHERE o.id = NEW.outcome_id),\n        NULL, 'outcome_evidence_recorded',\n        json_object('evidenceId', NEW.id, 'outcomeId', NEW.outcome_id, 'contractRevisionId', NEW.contract_revision_id, 'criterionId', NEW.criterion_id, 'kind', NEW.kind),\n        NEW.created_at);\nEND;",
	},
	{
		name:  "verification_runs_cdc_insert",
		table: "verification_runs",
		deps:  []string{"outcomes", "responsibility_spaces"},
		sql:   "CREATE TRIGGER verification_runs_cdc_insert\nAFTER INSERT ON verification_runs\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id FROM outcomes o JOIN responsibility_spaces s ON s.id = o.space_id WHERE o.id = NEW.outcome_id),\n        NULL, 'outcome_verification_recorded',\n        json_object('verificationId', NEW.id, 'outcomeId', NEW.outcome_id, 'contractRevisionId', NEW.contract_revision_id, 'criterionId', NEW.criterion_id, 'independenceClass', NEW.independence_class, 'result', NEW.result),\n        NEW.created_at);\nEND;",
	},
	{
		name:  "acceptance_decisions_cdc_insert",
		table: "acceptance_decisions",
		deps:  []string{"outcomes", "responsibility_spaces"},
		sql:   "CREATE TRIGGER acceptance_decisions_cdc_insert\nAFTER INSERT ON acceptance_decisions\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id FROM outcomes o JOIN responsibility_spaces s ON s.id = o.space_id WHERE o.id = NEW.outcome_id),\n        NULL, 'outcome_acceptance_decided',\n        json_object('decisionId', NEW.id, 'outcomeId', NEW.outcome_id, 'contractRevisionId', NEW.contract_revision_id, 'kind', NEW.kind, 'actorType', NEW.actor_type, 'resourceDisposition', NEW.resource_disposition),\n        NEW.created_at);\nEND;",
	},
	{
		name:  "outcome_corrections_cdc_insert",
		table: "outcome_corrections",
		deps:  []string{"outcomes", "responsibility_spaces"},
		sql:   "CREATE TRIGGER outcome_corrections_cdc_insert\nAFTER INSERT ON outcome_corrections\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id FROM outcomes o JOIN responsibility_spaces s ON s.id = o.space_id WHERE o.id = NEW.outcome_id),\n        NULL, 'outcome_correction_recorded',\n        json_object('correctionId', NEW.id, 'decisionId', NEW.decision_id, 'outcomeId', NEW.outcome_id, 'contractRevisionId', NEW.contract_revision_id, 'targetType', NEW.target_type, 'targetId', NEW.target_id),\n        NEW.created_at);\nEND;",
	},
	// Composed Outcomes (0106, ADR 0007). The binding is the fact worth
	// publishing: a contributing Outcome's own 'outcome_created' event says
	// nothing about what it contributes to, and this one carries the parent,
	// the exact parent revision, and the criterion claimed. The existing
	// outcomes writers are deliberately left untouched — restoration gates on
	// table existence, not column existence, so a writer referencing
	// parent_outcome_id would fail on a profile that skipped 0106.
	{
		name:  "contribution_links_cdc_insert",
		table: "contribution_links",
		deps:  []string{"outcomes", "responsibility_spaces"},
		sql:   "CREATE TRIGGER contribution_links_cdc_insert\nAFTER INSERT ON contribution_links\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id FROM outcomes o JOIN responsibility_spaces s ON s.id = o.space_id WHERE o.id = NEW.parent_outcome_id),\n        NULL, 'outcome_contribution_bound',\n        json_object('linkId', NEW.id, 'parentOutcomeId', NEW.parent_outcome_id, 'childOutcomeId', NEW.child_outcome_id, 'parentContractRevisionId', NEW.parent_contract_revision_id, 'parentCriterionId', NEW.parent_criterion_id),\n        NEW.created_at);\nEND;",
	},
	// Decomposition authority writers (0107). Proposal and authorization are
	// separate events because they mean different things: one records what was
	// offered, the other that contributing Outcomes now exist.
	{
		name:  "decomposition_revisions_cdc_insert",
		table: "decomposition_revisions",
		deps:  []string{"outcomes", "responsibility_spaces"},
		sql:   "CREATE TRIGGER decomposition_revisions_cdc_insert\nAFTER INSERT ON decomposition_revisions\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id FROM outcomes o JOIN responsibility_spaces s ON s.id = o.space_id WHERE o.id = NEW.outcome_id),\n        NULL, 'outcome_decomposition_proposed',\n        json_object('decompositionId', NEW.id, 'outcomeId', NEW.outcome_id, 'number', NEW.number, 'contractRevisionId', NEW.contract_revision_id, 'status', NEW.status),\n        NEW.created_at);\nEND;",
	},
	{
		name:  "decomposition_revisions_cdc_update",
		table: "decomposition_revisions",
		deps:  []string{"outcomes", "responsibility_spaces"},
		sql:   "CREATE TRIGGER decomposition_revisions_cdc_update\nAFTER UPDATE ON decomposition_revisions\nWHEN OLD.status <> NEW.status\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id FROM outcomes o JOIN responsibility_spaces s ON s.id = o.space_id WHERE o.id = NEW.outcome_id),\n        NULL, 'outcome_decomposition_authorized',\n        json_object('decompositionId', NEW.id, 'outcomeId', NEW.outcome_id, 'number', NEW.number, 'previousStatus', OLD.status, 'status', NEW.status),\n        datetime('now'));\nEND;",
	},
	// Dependency waivers (0108). Overriding an authorized ordering is a real
	// decision about risk, so it publishes like every other owner decision.
	{
		name:  "contribution_waivers_cdc_insert",
		table: "contribution_dependency_waivers",
		deps:  []string{"decomposition_revisions", "outcomes", "responsibility_spaces"},
		sql:   "CREATE TRIGGER contribution_waivers_cdc_insert\nAFTER INSERT ON contribution_dependency_waivers\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id FROM decomposition_revisions d JOIN outcomes o ON o.id = d.outcome_id JOIN responsibility_spaces s ON s.id = o.space_id WHERE d.id = NEW.decomposition_id),\n        NULL, 'outcome_contribution_dependency_waived',\n        json_object('waiverId', NEW.id, 'decompositionId', NEW.decomposition_id, 'fromRef', NEW.from_ref, 'toRef', NEW.to_ref, 'waivedBy', NEW.waived_by),\n        NEW.created_at);\nEND;",
	},
	// Agent-authored decomposition asks (0109). The refusal is published as
	// well as the success: an agent proposal the daemon turned down is a fact
	// the owner needs, not a silent non-event.
	{
		name:  "decomposition_requests_cdc_insert",
		table: "decomposition_requests",
		deps:  []string{"outcomes", "responsibility_spaces"},
		sql:   "CREATE TRIGGER decomposition_requests_cdc_insert\nAFTER INSERT ON decomposition_requests\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id FROM outcomes o JOIN responsibility_spaces s ON s.id = o.space_id WHERE o.id = NEW.outcome_id),\n        NULL, 'outcome_decomposition_requested',\n        json_object('requestId', NEW.id, 'outcomeId', NEW.outcome_id, 'contractRevisionId', NEW.contract_revision_id, 'expiresAt', NEW.expires_at),\n        NEW.created_at);\nEND;",
	},
	{
		name:  "decomposition_requests_cdc_update",
		table: "decomposition_requests",
		deps:  []string{"outcomes", "responsibility_spaces"},
		sql:   "CREATE TRIGGER decomposition_requests_cdc_update\nAFTER UPDATE ON decomposition_requests\nWHEN OLD.status <> NEW.status\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT s.project_id FROM outcomes o JOIN responsibility_spaces s ON s.id = o.space_id WHERE o.id = NEW.outcome_id),\n        NULL, 'outcome_decomposition_request_answered',\n        json_object('requestId', NEW.id, 'outcomeId', NEW.outcome_id, 'previousStatus', OLD.status, 'status', NEW.status, 'decompositionId', NEW.decomposition_id),\n        datetime('now'));\nEND;",
	},
	// Shared adaptive intake and explicit Home-to-Work lineage (#32).
	{
		name:  "intake_sessions_cdc_insert",
		table: "intake_sessions",
		sql:   "CREATE TRIGGER intake_sessions_cdc_insert\nAFTER INSERT ON intake_sessions\nWHEN NEW.project_id IS NOT NULL\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (NEW.project_id, NULL, 'intake_captured', json_object('intakeId', NEW.id, 'sourceSurface', NEW.source_surface, 'purpose', NEW.purpose, 'status', NEW.status), NEW.created_at);\nEND;",
	},
	{
		name:  "intake_sessions_cdc_update",
		table: "intake_sessions",
		sql:   "CREATE TRIGGER intake_sessions_cdc_update\nAFTER UPDATE ON intake_sessions\nWHEN NEW.project_id IS NOT NULL AND (OLD.status <> NEW.status OR OLD.current_proposal_revision <> NEW.current_proposal_revision OR OLD.failure_code <> NEW.failure_code)\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (NEW.project_id, NULL, 'intake_updated', json_object('intakeId', NEW.id, 'previousStatus', OLD.status, 'status', NEW.status, 'proposalRevision', NEW.current_proposal_revision), NEW.updated_at);\nEND;",
	},
	{
		name:  "intake_proposals_cdc_insert",
		table: "intake_proposal_revisions",
		deps:  []string{"intake_sessions"},
		sql:   "CREATE TRIGGER intake_proposals_cdc_insert\nAFTER INSERT ON intake_proposal_revisions\nWHEN (SELECT project_id FROM intake_sessions WHERE id = NEW.intake_id) IS NOT NULL\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES ((SELECT project_id FROM intake_sessions WHERE id = NEW.intake_id), NULL, 'intake_proposal_revised', json_object('intakeId', NEW.intake_id, 'proposalId', NEW.id, 'revision', NEW.revision), NEW.created_at);\nEND;",
	},
	{
		name:  "intake_confirmations_cdc_insert",
		table: "intake_confirmations",
		deps:  []string{"intake_sessions"},
		sql:   "CREATE TRIGGER intake_confirmations_cdc_insert\nAFTER INSERT ON intake_confirmations\nWHEN (SELECT project_id FROM intake_sessions WHERE id = NEW.intake_id) IS NOT NULL\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES ((SELECT project_id FROM intake_sessions WHERE id = NEW.intake_id), NULL, 'intake_confirmed', json_object('intakeId', NEW.intake_id, 'proposalRevision', NEW.proposal_revision, 'outcomeId', NEW.outcome_id, 'contractRevisionId', NEW.contract_revision_id), NEW.confirmed_at);\nEND;",
	},
	{
		name:  "responsibility_links_cdc_insert",
		table: "responsibility_links",
		sql:   "CREATE TRIGGER responsibility_links_cdc_insert\nAFTER INSERT ON responsibility_links\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (NEW.project_id, NULL, 'responsibility_link_created', json_object('linkId', NEW.id, 'sourceOpenLoopId', NEW.source_open_loop_id, 'destinationOutcomeId', NEW.destination_outcome_id), NEW.created_at);\nEND;",
	},
	{
		name:  "responsibility_links_cdc_update",
		table: "responsibility_links",
		sql:   "CREATE TRIGGER responsibility_links_cdc_update\nAFTER UPDATE ON responsibility_links\nWHEN OLD.ended_at IS NULL AND NEW.ended_at IS NOT NULL\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (NEW.project_id, NULL, 'responsibility_link_ended', json_object('linkId', NEW.id, 'sourceOpenLoopId', NEW.source_open_loop_id, 'destinationOutcomeId', NEW.destination_outcome_id, 'endedBy', NEW.ended_by), NEW.ended_at);\nEND;",
	},
	// Durable Project-Waldo conversation and bounded provider continuation (#77).
	// Payloads deliberately contain identifiers and policy facts only; visible
	// messages and provider transcript content never enter the CDC stream.
	{
		name:  "waldo_conversations_cdc_insert",
		table: "waldo_conversations",
		sql:   "CREATE TRIGGER waldo_conversations_cdc_insert\nAFTER INSERT ON waldo_conversations\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (NEW.project_id, NULL, 'waldo_conversation_created', json_object('conversationId', NEW.id), NEW.created_at);\nEND;",
	},
	{
		name:  "waldo_conversation_episodes_cdc_insert",
		table: "waldo_conversation_episodes",
		sql:   "CREATE TRIGGER waldo_conversation_episodes_cdc_insert\nAFTER INSERT ON waldo_conversation_episodes\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (NEW.project_id, NULL, 'waldo_conversation_episode_opened', json_object('conversationId', NEW.conversation_id, 'episodeId', NEW.id, 'ordinal', NEW.ordinal), NEW.created_at);\nEND;",
	},
	{
		name:  "waldo_conversation_episodes_cdc_update",
		table: "waldo_conversation_episodes",
		sql:   "CREATE TRIGGER waldo_conversation_episodes_cdc_update\nAFTER UPDATE ON waldo_conversation_episodes\nWHEN OLD.state = 'active' AND NEW.state = 'sealed'\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (NEW.project_id, NULL, 'waldo_conversation_episode_sealed', json_object('conversationId', NEW.conversation_id, 'episodeId', NEW.id, 'reason', NEW.seal_reason), NEW.sealed_at);\nEND;",
	},
	{
		name:  "waldo_conversation_turns_cdc_insert",
		table: "waldo_conversation_turns",
		sql:   "CREATE TRIGGER waldo_conversation_turns_cdc_insert\nAFTER INSERT ON waldo_conversation_turns\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (NEW.project_id, NULL, 'waldo_conversation_turn_appended', json_object('conversationId', NEW.conversation_id, 'episodeId', NEW.episode_id, 'turnId', NEW.id, 'sequence', NEW.sequence, 'role', NEW.role), NEW.created_at);\nEND;",
	},
	{
		name:  "waldo_context_attachments_cdc_insert",
		table: "waldo_context_attachments",
		sql:   "CREATE TRIGGER waldo_context_attachments_cdc_insert\nAFTER INSERT ON waldo_context_attachments\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (NEW.project_id, NULL, 'waldo_conversation_context_attached', json_object('conversationId', NEW.conversation_id, 'attachmentId', NEW.id, 'kind', NEW.kind, 'objectId', NEW.object_id, 'objectRevision', NEW.object_revision), NEW.created_at);\nEND;",
	},
	{
		name:  "waldo_context_attachments_cdc_update",
		table: "waldo_context_attachments",
		sql:   "CREATE TRIGGER waldo_context_attachments_cdc_update\nAFTER UPDATE ON waldo_context_attachments\nWHEN OLD.detached_at IS NULL AND NEW.detached_at IS NOT NULL\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (NEW.project_id, NULL, 'waldo_conversation_context_detached', json_object('conversationId', NEW.conversation_id, 'attachmentId', NEW.id), NEW.detached_at);\nEND;",
	},
	{
		name:  "waldo_continuation_operations_cdc_insert",
		table: "waldo_continuation_operations",
		sql:   "CREATE TRIGGER waldo_continuation_operations_cdc_insert\nAFTER INSERT ON waldo_continuation_operations\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (NEW.project_id, NULL, 'waldo_conversation_continuation_prepared', json_object('conversationId', NEW.conversation_id, 'operationId', NEW.id, 'fromEpisodeId', NEW.from_episode_id, 'state', NEW.state, 'reason', NEW.reason), NEW.created_at);\nEND;",
	},
	{
		name:  "waldo_continuation_operations_cdc_update",
		table: "waldo_continuation_operations",
		sql:   "CREATE TRIGGER waldo_continuation_operations_cdc_update\nAFTER UPDATE ON waldo_continuation_operations\nWHEN OLD.state <> NEW.state\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (NEW.project_id, NULL, 'waldo_conversation_continuation_progressed', json_object('conversationId', NEW.conversation_id, 'operationId', NEW.id, 'fromState', OLD.state, 'toState', NEW.state), NEW.updated_at);\nEND;",
	},
	{
		name:  "waldo_continuation_receipts_cdc_insert",
		table: "waldo_continuation_receipts",
		sql:   "CREATE TRIGGER waldo_continuation_receipts_cdc_insert\nAFTER INSERT ON waldo_continuation_receipts\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (NEW.project_id, NULL, 'waldo_conversation_continuation_recorded', json_object('conversationId', NEW.conversation_id, 'receiptId', NEW.id, 'fromEpisodeId', NEW.from_episode_id, 'toEpisodeId', COALESCE(NEW.to_episode_id, ''), 'action', NEW.action, 'reason', NEW.reason, 'materialChange', json(CASE WHEN NEW.material_change THEN 'true' ELSE 'false' END)), NEW.created_at);\nEND;",
	},
	// Durable Project Brief revisions (#94). Identifier-only payload: the Brief's
	// prose is durable Project context, not a change-feed fact.
	{
		name:  "project_brief_revisions_cdc_insert",
		table: "project_brief_revisions",
		since: "0111",
		sql:   "CREATE TRIGGER project_brief_revisions_cdc_insert\nAFTER INSERT ON project_brief_revisions\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        NEW.project_id,\n        NULL,\n        'project_brief_revised',\n        json_object('revisionId', NEW.id, 'revision', NEW.revision_number),\n        NEW.created_at\n    );\nEND;",
	},
	// Durable run intent. Both writers carry identity only: the Mission reads
	// the current generation through the canonical projection, and a change
	// feed that duplicated authority would become a second source of it.
	{
		name:  "outcome_run_intents_cdc_insert",
		table: "outcome_run_intents",
		deps:  []string{"outcomes", "responsibility_spaces"},
		since: "0123",
		sql:   "CREATE TRIGGER outcome_run_intents_cdc_insert\nAFTER INSERT ON outcome_run_intents\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT rs.project_id FROM outcomes o JOIN responsibility_spaces rs ON rs.id = o.space_id WHERE o.id = NEW.outcome_id),\n        NULL,\n        'outcome_run_intent_changed',\n        json_object('outcomeId', NEW.outcome_id, 'generation', NEW.generation, 'desired', NEW.desired),\n        NEW.requested_at\n    );\nEND;",
	},
	{
		name:  "outcome_run_intents_cdc_acknowledged",
		table: "outcome_run_intents",
		deps:  []string{"outcomes", "responsibility_spaces"},
		since: "0123",
		sql:   "CREATE TRIGGER outcome_run_intents_cdc_acknowledged\nAFTER UPDATE ON outcome_run_intents\nWHEN OLD.acknowledged_at IS NULL AND NEW.acknowledged_at IS NOT NULL\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT rs.project_id FROM outcomes o JOIN responsibility_spaces rs ON rs.id = o.space_id WHERE o.id = NEW.outcome_id),\n        NULL,\n        'outcome_run_intent_changed',\n        json_object('outcomeId', NEW.outcome_id, 'generation', NEW.generation, 'desired', NEW.desired, 'acknowledged', 1),\n        NEW.acknowledged_at\n    );\nEND;",
	},
	{
		name:    "outcome_run_intents_cdc_failure",
		table:   "outcome_run_intents",
		deps:    []string{"outcomes", "responsibility_spaces"},
		columns: []string{"admission_failure_code", "admission_failed_at"},
		since:   "0132",
		sql:     "CREATE TRIGGER outcome_run_intents_cdc_failure\nAFTER UPDATE ON outcome_run_intents\nWHEN OLD.admission_failure_code = '' AND NEW.admission_failure_code <> ''\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (\n        (SELECT rs.project_id FROM outcomes o JOIN responsibility_spaces rs ON rs.id = o.space_id WHERE o.id = NEW.outcome_id),\n        NULL,\n        'outcome_run_intent_changed',\n        json_object('outcomeId', NEW.outcome_id, 'generation', NEW.generation, 'desired', NEW.desired, 'admissionFailed', 1),\n        NEW.admission_failed_at\n    );\nEND;",
	},
	{
		name:  "outcome_deliveries_cdc_insert",
		table: "outcome_deliveries",
		deps:  []string{"outcomes", "responsibility_spaces"},
		since: "0127",
		sql:   "CREATE TRIGGER outcome_deliveries_cdc_insert\nAFTER INSERT ON outcome_deliveries\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES ((SELECT rs.project_id FROM outcomes o JOIN responsibility_spaces rs ON rs.id = o.space_id WHERE o.id = NEW.outcome_id), NULL, 'outcome_delivery_changed', json_object('outcomeId', NEW.outcome_id, 'deliveryId', NEW.id, 'state', NEW.state, 'artifactVersion', NEW.artifact_version), NEW.requested_at);\nEND;",
	},
	{
		name:  "outcome_deliveries_cdc_update",
		table: "outcome_deliveries",
		deps:  []string{"outcomes", "responsibility_spaces"},
		since: "0127",
		sql:   "CREATE TRIGGER outcome_deliveries_cdc_update\nAFTER UPDATE ON outcome_deliveries\nWHEN OLD.state <> NEW.state\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES ((SELECT rs.project_id FROM outcomes o JOIN responsibility_spaces rs ON rs.id = o.space_id WHERE o.id = NEW.outcome_id), NULL, 'outcome_delivery_changed', json_object('outcomeId', NEW.outcome_id, 'deliveryId', NEW.id, 'state', NEW.state, 'artifactVersion', NEW.artifact_version), COALESCE(NEW.completed_at, NEW.requested_at));\nEND;",
	},
	{
		name:  "planning_sessions_cdc_insert",
		table: "planning_sessions",
		since: "0133",
		sql:   "CREATE TRIGGER planning_sessions_cdc_insert\nAFTER INSERT ON planning_sessions\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (NEW.project_id, NULL, 'outcome_updated',\n        json_object('id', NEW.outcome_id, 'type', 'planning_session', 'planningSessionId', NEW.id, 'revision', NEW.revision),\n        NEW.updated_at);\nEND;",
	},
	{
		name:  "planning_sessions_cdc_update",
		table: "planning_sessions",
		since: "0133",
		sql:   "CREATE TRIGGER planning_sessions_cdc_update\nAFTER UPDATE ON planning_sessions\nBEGIN\n    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)\n    VALUES (NEW.project_id, NULL, 'outcome_updated',\n        json_object('id', NEW.outcome_id, 'type', 'planning_session', 'planningSessionId', NEW.id, 'revision', NEW.revision),\n        NEW.updated_at);\nEND;",
	},
	{
		name: "owner_answer_questions_needs_you_insert", table: "owner_answer_questions", deps: []string{"conversations", "sessions"}, since: "0149",
		sql: "CREATE TRIGGER owner_answer_questions_needs_you_insert AFTER INSERT ON owner_answer_questions BEGIN INSERT INTO change_log(project_id,session_id,event_type,payload,created_at) SELECT s.project_id,s.id,'session_updated',json_object('id',s.id,'needsYou',json_object('questionId',NEW.id,'generation',NEW.generation,'status','open')),NEW.updated_at FROM conversations c JOIN sessions s ON s.id=c.current_session_id WHERE c.id=NEW.conversation_id; END;",
	},
	{
		name: "owner_answer_questions_needs_you_update", table: "owner_answer_questions", deps: []string{"conversations", "sessions"}, since: "0149",
		sql: "CREATE TRIGGER owner_answer_questions_needs_you_update AFTER UPDATE ON owner_answer_questions WHEN OLD.status<>NEW.status BEGIN INSERT INTO change_log(project_id,session_id,event_type,payload,created_at) SELECT s.project_id,s.id,'session_updated',json_object('id',s.id,'needsYou',json_object('questionId',NEW.id,'generation',NEW.generation,'status',NEW.status)),NEW.updated_at FROM conversations c JOIN sessions s ON s.id=c.current_session_id WHERE c.id=NEW.conversation_id; END;",
	},
	{
		name: "governed_controls_needs_you_insert", table: "governed_control_commands", deps: []string{"sessions"}, columns: []string{"request_instance_id"}, since: "0149",
		sql: "CREATE TRIGGER governed_controls_needs_you_insert AFTER INSERT ON governed_control_commands WHEN NEW.command_class='answer' BEGIN INSERT INTO change_log(project_id,session_id,event_type,payload,created_at) SELECT s.project_id,s.id,'session_updated',json_object('id',s.id,'needsYou',json_object('generation',NEW.request_instance_id,'commandId',NEW.id,'status',NEW.state)),NEW.updated_at FROM sessions s WHERE s.id=NEW.session_id; END;",
	},
	{
		name: "governed_controls_needs_you_update", table: "governed_control_commands", deps: []string{"sessions"}, columns: []string{"request_instance_id"}, since: "0149",
		sql: "CREATE TRIGGER governed_controls_needs_you_update AFTER UPDATE ON governed_control_commands WHEN NEW.command_class='answer' AND OLD.state<>NEW.state BEGIN INSERT INTO change_log(project_id,session_id,event_type,payload,created_at) SELECT s.project_id,s.id,'session_updated',json_object('id',s.id,'needsYou',json_object('generation',NEW.request_instance_id,'commandId',NEW.id,'status',NEW.state)),NEW.updated_at FROM sessions s WHERE s.id=NEW.session_id; END;",
	},
}

// restoreChangeLogWriters recreates any missing change_log-writing trigger
// whose subject table physically exists. It is idempotent and safe to run on
// every daemon start.
func restoreChangeLogWriters(db *sql.DB) error {
	for _, w := range changeLogWriters {
		var n int
		if err := db.QueryRow(
			"SELECT count(*) FROM sqlite_master WHERE type='trigger' AND name=?", w.name,
		).Scan(&n); err != nil {
			return fmt.Errorf("inspect trigger %s: %w", w.name, err)
		}
		if n > 0 {
			continue
		}
		var t int
		if err := db.QueryRow(
			"SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", w.table,
		).Scan(&t); err != nil {
			return fmt.Errorf("inspect table %s: %w", w.table, err)
		}
		if t == 0 {
			// Degraded profile: the subject table arrives through a later
			// schema repair together with its writer.
			continue
		}
		ready := true
		for _, dep := range w.deps {
			var d int
			if err := db.QueryRow(
				"SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", dep,
			).Scan(&d); err != nil {
				return fmt.Errorf("inspect dependency %s of %s: %w", dep, w.name, err)
			}
			if d == 0 {
				ready = false
				break
			}
		}
		if !ready {
			// A trigger body that joins an absent table would abort every
			// future write to its subject; defer until dependencies land.
			continue
		}
		for _, column := range w.columns {
			var c int
			if err := db.QueryRow("SELECT count(*) FROM pragma_table_info(?) WHERE name=?", w.table, column).Scan(&c); err != nil {
				return fmt.Errorf("inspect column %s.%s for %s: %w", w.table, column, w.name, err)
			}
			if c == 0 {
				ready = false
				break
			}
		}
		if !ready {
			continue
		}
		if _, err := db.Exec(w.sql); err != nil {
			return fmt.Errorf("restore trigger %s: %w", w.name, err)
		}
	}
	return nil
}
