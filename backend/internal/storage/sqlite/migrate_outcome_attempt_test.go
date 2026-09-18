package sqlite

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestMigration0102AttemptExecutionLifecycle exercises the Act & Observe seam
// at the SQL layer: attempts are append-only lineage whose only mutation is a
// trigger-guarded status transition; observations and receipts are
// append-only; exactly one open fence may exist per subject; session refs
// carry no FK into sessions; and every new writer emits canonical CDC events
// with the project resolved through outcome -> responsibility space.
func TestMigration0102AttemptExecutionLifecycle(t *testing.T) {
	db := openContractTestDB(t)
	upTo(t, db, 102)
	// Raw handles skip migrate(); restore the writers the way daemon startup
	// does immediately after goose completes.
	if err := restoreChangeLogWriters(db); err != nil {
		t.Fatalf("restore change log writers: %v", err)
	}
	seedContractProject(t, db)
	spaceID := seedWorkSpace(t, db)

	countEvents := func(eventType string) int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM change_log WHERE event_type = ?`, eventType).Scan(&n); err != nil {
			t.Fatalf("count %s events: %v", eventType, err)
		}
		return n
	}

	if _, err := db.Exec(`INSERT INTO outcomes (id, space_id, title, current_revision_number)
		VALUES ('out_att', ?, 'Local Focus Ledger', 1)`, spaceID); err != nil {
		t.Fatalf("insert outcome: %v", err)
	}
	criteria, err := json.Marshal([]string{"Entering a duration creates one focus block."})
	if err != nil {
		t.Fatalf("marshal criteria: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO contract_revisions (id, outcome_id, number, goal, success_criteria, review)
		VALUES ('cr_att_1', 'out_att', 1, 'Record focus locally.', ?, 'Deterministic checks.')`,
		string(criteria)); err != nil {
		t.Fatalf("insert revision: %v", err)
	}
	digest := strings.Repeat("ef", 32)
	if _, err := db.Exec(`INSERT INTO plan_revisions
		(id, outcome_id, number, contract_revision_number, status, summary, run_brief_core_digest)
		VALUES ('plan_att', 'out_att', 1, 1, 'approved', 'One direct Work Unit', ?)`, digest); err != nil {
		t.Fatalf("insert plan: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO work_units
		(id, plan_revision_id, kind, title, contract_revision_number, output_summary,
		 evidence_checks, verification_requirement, stop_conditions)
		VALUES ('wu_att', 'plan_att', 'direct', 'Build and prove Local Focus Ledger', 1,
			'Working local feature', json('["checks pass"]'), 'Deterministic checks',
			json('["stop before remote effects"]'))`); err != nil {
		t.Fatalf("insert work unit: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO attempts
		(id, outcome_id, plan_revision_id, work_unit_id, number, status, contract_revision_number, request_key)
		VALUES ('att_1', 'out_att', 'plan_att', 'wu_att', 1, 'queued', 1, 'rk-1')`); err != nil {
		t.Fatalf("insert attempt: %v", err)
	}
	if countEvents("outcome_attempt_started") != 1 {
		t.Fatalf("outcome_attempt_started events = %d, want 1", countEvents("outcome_attempt_started"))
	}
	var attemptEventProject string
	var attemptPayloadRaw string
	if err := db.QueryRow(`SELECT project_id, payload FROM change_log WHERE event_type = 'outcome_attempt_started'`).
		Scan(&attemptEventProject, &attemptPayloadRaw); err != nil {
		t.Fatalf("read attempt started event: %v", err)
	}
	if attemptEventProject != "p1" {
		t.Fatalf("event project_id = %q, want p1 resolved through outcome and space", attemptEventProject)
	}
	var startPayload map[string]any
	if err := json.Unmarshal([]byte(attemptPayloadRaw), &startPayload); err != nil {
		t.Fatalf("decode start payload %q: %v", attemptPayloadRaw, err)
	}
	if startPayload["attemptId"] != "att_1" || startPayload["status"] != "queued" || startPayload["contractRevisionNumber"] != float64(1) {
		t.Fatalf("unexpected start payload %#v", startPayload)
	}

	// Session ref binds a provider identity that has NO row in sessions — the
	// ref must survive without any FK.
	snapshot := `{"snapshotVersion":1,"harness":"codex"}`
	if _, err := db.Exec(`INSERT INTO attempt_sessions
		(id, attempt_id, seq, session_id, harness, mode, run_brief_core_digest, run_brief_compiled_digest, admission_snapshot)
		VALUES ('asr_1', 'att_1', 1, 'provider-session-x', 'codex', 'tui', ?, '', json(?))`,
		digest, snapshot); err != nil {
		t.Fatalf("insert session ref: %v", err)
	}
	if countEvents("outcome_attempt_session_bound") != 1 {
		t.Fatalf("outcome_attempt_session_bound events = %d, want 1", countEvents("outcome_attempt_session_bound"))
	}

	// The only legal transition path emits exactly one update event per hop.
	for _, hop := range []string{"running", "paused", "running"} {
		if _, err := db.Exec(`UPDATE attempts SET status = ? WHERE id = 'att_1'`, hop); err != nil {
			t.Fatalf("transition to %s: %v", hop, err)
		}
	}
	if countEvents("outcome_attempt_updated") != 3 {
		t.Fatalf("outcome_attempt_updated events = %d, want 3", countEvents("outcome_attempt_updated"))
	}
	// No-op status writes emit nothing.
	if _, err := db.Exec(`UPDATE attempts SET status = 'running' WHERE id = 'att_1'`); err != nil {
		t.Fatalf("redundant status write: %v", err)
	}
	if countEvents("outcome_attempt_updated") != 3 {
		t.Fatal("no-op status write must not emit CDC")
	}

	// Every illegal transition is refused by the database itself. The attempt
	// currently sits at 'running'; a second attempt stays queued.
	if _, err := db.Exec(`INSERT INTO attempts
		(id, outcome_id, plan_revision_id, work_unit_id, number, status, contract_revision_number)
		VALUES ('att_queued', 'out_att', 'plan_att', 'wu_att', 2, 'queued', 1)`); err != nil {
		t.Fatalf("insert second attempt: %v", err)
	}
	for name, stmt := range map[string]string{
		"running -> succeeded": `UPDATE attempts SET status = 'succeeded' WHERE id = 'att_1'`,
		"running -> queued":    `UPDATE attempts SET status = 'queued' WHERE id = 'att_1'`,
		"queued -> reconciled": `UPDATE attempts SET status = 'reconciled' WHERE id = 'att_queued'`,
	} {
		if _, err := db.Exec(stmt); err == nil {
			t.Fatalf("%s must be rejected by the transition trigger", name)
		} else if !strings.Contains(err.Error(), "illegal attempt status transition") {
			t.Fatalf("%s rejected with unexpected error: %v", name, err)
		}
	}
	// Terminal states accept nothing: walk legally to failed, then try to
	// resurrect it.
	if _, err := db.Exec(`UPDATE attempts SET status = 'failed' WHERE id = 'att_1'`); err != nil {
		t.Fatalf("legal transition to failed: %v", err)
	}
	for name, target := range map[string]string{"failed -> running": "running", "failed -> lost": "lost"} {
		if _, err := db.Exec(`UPDATE attempts SET status = ? WHERE id = 'att_1'`, target); err == nil {
			t.Fatalf("%s must be rejected by the transition trigger", name)
		}
	}
	if countEvents("outcome_attempt_updated") != 4 {
		t.Fatalf("outcome_attempt_updated events = %d, want 4 (three hops + failed)", countEvents("outcome_attempt_updated"))
	}

	// Observations are append-only and ordered per attempt.
	if _, err := db.Exec(`INSERT INTO attempt_observations (id, attempt_id, seq, kind, payload)
		VALUES ('obs_1', 'att_1', 1, 'contained', json('{"reason":"probe"}'))`); err != nil {
		t.Fatalf("insert observation: %v", err)
	}
	if countEvents("outcome_attempt_observed") != 1 {
		t.Fatalf("outcome_attempt_observed events = %d, want 1", countEvents("outcome_attempt_observed"))
	}
	for name, stmt := range map[string]string{
		"observation rewrite":  `UPDATE attempt_observations SET kind = 'rewritten' WHERE id = 'obs_1'`,
		"observation deletion": `DELETE FROM attempt_observations WHERE id = 'obs_1'`,
	} {
		if _, err := db.Exec(stmt); err == nil {
			t.Fatalf("%s must be rejected by an append-only trigger", name)
		} else if !strings.Contains(err.Error(), "append-only") && !strings.Contains(err.Error(), "immutable") {
			t.Fatalf("%s rejected with unexpected error: %v", name, err)
		}
	}

	// Exactly ONE open fence per subject: the second open fence aborts.
	if _, err := db.Exec(`INSERT INTO attempt_fences (id, subject, attempt_id)
		VALUES ('fence_1', 'project:p1', 'att_1')`); err != nil {
		t.Fatalf("issue fence: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO attempt_fences (id, subject, attempt_id)
		VALUES ('fence_2', 'project:p1', 'att_1')`); err == nil {
		t.Fatal("second open fence for the same subject must violate the partial unique index")
	}
	// A fence for another subject is fine.
	if _, err := db.Exec(`INSERT INTO attempt_fences (id, subject, attempt_id)
		VALUES ('fence_other', 'project:p2', 'att_1')`); err != nil {
		t.Fatalf("fence on other subject: %v", err)
	}
	// Release requires a reason and is final.
	if _, err := db.Exec(`UPDATE attempt_fences SET released_at = datetime('now') WHERE id = 'fence_1'`); err == nil {
		t.Fatal("release without reason must be refused")
	}
	if _, err := db.Exec(`UPDATE attempt_fences SET released_at = datetime('now'), release_reason = 'replacement' WHERE id = 'fence_1'`); err != nil {
		t.Fatalf("release fence: %v", err)
	}
	if _, err := db.Exec(`UPDATE attempt_fences SET released_at = datetime('now'), release_reason = 'again' WHERE id = 'fence_1'`); err == nil {
		t.Fatal("fence release must be final")
	}
	// After release the subject may be fenced again (custody handover).
	if _, err := db.Exec(`INSERT INTO attempt_fences (id, subject, attempt_id)
		VALUES ('fence_3', 'project:p1', 'att_1')`); err != nil {
		t.Fatalf("re-issue fence after release: %v", err)
	}

	// Receipts record reconcile verdicts and are append-only.
	if _, err := db.Exec(`INSERT INTO attempt_recovery_receipts (id, attempt_id, resolution, detail)
		VALUES ('rcpt_1', 'att_1', 'resumed', json('{"evidence":"heartbeat"}'))`); err != nil {
		t.Fatalf("insert receipt: %v", err)
	}
	if countEvents("outcome_attempt_recovered") != 1 {
		t.Fatalf("outcome_attempt_recovered events = %d, want 1", countEvents("outcome_attempt_recovered"))
	}
	var receiptProject string
	if err := db.QueryRow(`SELECT project_id FROM change_log WHERE event_type = 'outcome_attempt_recovered'`).Scan(&receiptProject); err != nil {
		t.Fatalf("read recovered event: %v", err)
	}
	if receiptProject != "p1" {
		t.Fatalf("receipt event project = %q, want p1", receiptProject)
	}
	if _, err := db.Exec(`UPDATE attempt_recovery_receipts SET resolution = 'needs_attention' WHERE id = 'rcpt_1'`); err == nil {
		t.Fatal("receipt rewrite must be rejected")
	}

	// Attempt identity is immutable and rows can never be deleted.
	for name, stmt := range map[string]string{
		"attempt rebinding":    `UPDATE attempts SET plan_revision_id = 'plan_att2' WHERE id = 'att_1'`,
		"request key rewrite":  `UPDATE attempts SET request_key = 'rk-2' WHERE id = 'att_1'`,
		"attempt deletion":     `DELETE FROM attempts WHERE id = 'att_1'`,
		"fence identity drift": `UPDATE attempt_fences SET subject = 'project:p9' WHERE id = 'fence_3'`,
	} {
		if _, err := db.Exec(stmt); err == nil {
			t.Fatalf("%s must be rejected by an immutability trigger", name)
		}
	}

	// Every writer detached by the rebuild is restored by the seam.
	for _, trigger := range []string{
		"sessions_cdc_insert",
		"outcome_plans_cdc_insert",
		"attempts_cdc_insert",
		"attempts_cdc_update",
		"attempt_sessions_cdc_insert",
		"attempt_observations_cdc_insert",
		"attempt_recovery_receipts_cdc_insert",
	} {
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name = ?`, trigger,
		).Scan(&n); err != nil {
			t.Fatalf("inspect trigger %s: %v", trigger, err)
		}
		if n != 1 {
			t.Fatalf("trigger %s count = %d, want 1 after migrate()", trigger, n)
		}
	}
}

// TestMigration0102SurvivesSkippedOutcomeMigrations pins the degraded-profile
// rule: when a burned ledger entry makes 0102 run without the Outcome tables
// existing, the migration must still apply cleanly and leave none of the
// attempt writers behind (they are restored by cdc_restore once their subject
// tables land).
func TestMigration0102SurvivesSkippedOutcomeMigrations(t *testing.T) {
	db := openContractTestDB(t)
	upTo(t, db, 98)
	// Burn versions 99 through 101 the way upstream-ledger collisions do.
	for v := int64(99); v <= 101; v++ {
		mustExec(t, db, `INSERT INTO goose_db_version (version_id, is_applied) VALUES (?, 1)`, v)
	}

	if err := migrate(db); err != nil {
		t.Fatalf("migrate with burned 0099-0101: %v", err)
	}

	for _, table := range []string{
		"attempts", "attempt_sessions", "attempt_observations",
		"attempt_fences", "attempt_recovery_receipts",
	} {
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = ?`, table,
		).Scan(&n); err != nil || n != 1 {
			t.Fatalf("table %s present=%d err=%v, want created", table, n, err)
		}
	}
	for _, trigger := range []string{
		"responsibility_outcomes_cdc_insert",
		"outcome_plans_cdc_insert",
		"attempts_cdc_insert",
		"attempts_cdc_update",
		"attempt_sessions_cdc_insert",
		"attempt_observations_cdc_insert",
		"attempt_recovery_receipts_cdc_insert",
	} {
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name = ?`, trigger,
		).Scan(&n); err != nil {
			t.Fatalf("inspect trigger %s: %v", trigger, err)
		}
		if n != 0 {
			t.Fatalf("trigger %s exists without its subject tables; want deferred to cdc_restore", trigger)
		}
	}
}

// TestInteractivePlanningSchemaReconcilesBurnedMigration pins the companion
// recovery path for 0133: a foreign or interrupted build may record the
// version without installing its physical tables. Startup must restore the
// complete shape without fabricating any planning rows.
func TestInteractivePlanningSchemaReconcilesBurnedMigration(t *testing.T) {
	db := openContractTestDB(t)
	upTo(t, db, 131)
	mustExec(t, db, `INSERT INTO goose_db_version (version_id, is_applied) VALUES (133, 1)`)

	if err := migrate(db); err != nil {
		t.Fatalf("migrate with burned 0133: %v", err)
	}

	for _, table := range []string{"planning_sessions", "planning_turns"} {
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = ?`, table,
		).Scan(&n); err != nil || n != 1 {
			t.Fatalf("table %s present=%d err=%v, want reconciled", table, n, err)
		}
	}
	for _, column := range []string{"planning_session_id", "source_intelligence_run_id"} {
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('plan_revisions') WHERE name = ?`, column,
		).Scan(&n); err != nil || n != 1 {
			t.Fatalf("plan_revisions column %s present=%d err=%v, want reconciled", column, n, err)
		}
	}
	for _, trigger := range []string{
		"plan_revisions_current_contract_guard",
		"plan_revisions_planning_source_guard",
		"planning_sessions_update_guard",
		"planning_turns_immutable_update",
		"planning_turns_immutable_delete",
		"planning_sessions_cdc_insert",
		"planning_sessions_cdc_update",
	} {
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name = ?`, trigger,
		).Scan(&n); err != nil || n != 1 {
			t.Fatalf("trigger %s present=%d err=%v, want reconciled", trigger, n, err)
		}
	}

	var sessions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM planning_sessions`).Scan(&sessions); err != nil {
		t.Fatalf("count planning sessions: %v", err)
	}
	if sessions != 0 {
		t.Fatalf("planning sessions=%d, want no synthesized authority", sessions)
	}
}
