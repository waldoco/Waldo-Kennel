package sqlite

import (
	"database/sql"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttemptStartReservationMigrationUpAndTransitionGuards(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "m.db")+pragmas)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err = goose.Up(db, "migrations", goose.WithAllowMissing()); err != nil {
		t.Fatal(err)
	}
	if err = restoreChangeLogWriters(db); err != nil {
		t.Fatal(err)
	}
	var sqlText string
	if err = db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='attempt_start_reservations'`).Scan(&sqlText); err != nil {
		t.Fatal(err)
	}
	var triggerSQL string
	if err = db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='trigger' AND name='attempts_status_transition'`).Scan(&triggerSQL); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(triggerSQL, "OLD.status='awaiting_authority' AND NEW.status IN ('queued','failed','cancelled')") || strings.Contains(triggerSQL, "OLD.status='awaiting_authority' AND NEW.status IN ('running'") {
		t.Fatalf("unexpected transition guard: %s", triggerSQL)
	}
	for _, name := range []string{"attempts_cdc_insert", "attempts_cdc_update", "attempt_sessions_require_admitted_attempt", "attempt_fences_require_admitted_attempt", "workspace_launch_packets_require_admitted_attempt", "attempt_receipts_require_executed_attempt"} {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='trigger' AND name=? AND tbl_name IN ('attempts','attempt_sessions','attempt_fences','workspace_bound_launch_packets','attempt_receipts')`, name).Scan(&n); err != nil || n != 1 {
			t.Fatalf("trigger %s count=%d err=%v", name, n, err)
		}
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check reported violation")
	}
	seedContractProject(t, db)
	spaceID := seedWorkSpace(t, db)
	if _, err = db.Exec(`INSERT INTO outcomes(id,space_id,title,current_revision_number) VALUES('guard-out',?,'guard',1)`, spaceID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO contract_revisions(id,outcome_id,number,goal,success_criteria,review) VALUES('guard-cr','guard-out',1,'g',json('["ok"]'),'r')`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO plan_revisions(id,outcome_id,number,contract_revision_number,status,summary,run_brief_core_digest) VALUES('guard-plan','guard-out',1,1,'approved','s',?)`, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO work_units(id,plan_revision_id,kind,title,contract_revision_number,output_summary,evidence_checks,verification_requirement,stop_conditions) VALUES('guard-wu','guard-plan','direct','t',1,'o',json('["c"]'),'v',json('["s"]'))`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO attempts(id,outcome_id,plan_revision_id,work_unit_id,number,status,contract_revision_number,request_key) VALUES('guard-att','guard-out','guard-plan','guard-wu',1,'awaiting_authority',1,'guard-key')`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS approved_executable_specs(digest TEXT UNIQUE)`); err != nil {
		t.Fatal(err)
	}
	for name, statement := range map[string]string{
		"session": `INSERT INTO attempt_sessions(id,attempt_id,seq,session_id,run_brief_core_digest,admission_snapshot) VALUES('bad-session','guard-att',1,'provider',?,json('{}'))`,
		"fence":   `INSERT INTO attempt_fences(id,subject,attempt_id) VALUES('bad-fence','subject','guard-att')`,
		"launch":  `INSERT INTO workspace_bound_launch_packets(attempt_id,spec_digest,session_id,digest,packet_json) VALUES('guard-att','missing','session','digest',json('{}'))`,
		"receipt": `INSERT INTO attempt_receipts(attempt_id,outcome_id,plan_revision_id,work_unit_id,contract_revision_number,artifact_version,workspace_kind,workspace_path,base_revision,result_revision,workspace_dirty,retention_state,observed_at) VALUES('guard-att','guard-out','guard-plan','guard-wu',1,'artifact','git_worktree','/tmp/x','','',0,'retained',CURRENT_TIMESTAMP)`,
	} {
		var execErr error
		if name == "session" {
			_, execErr = db.Exec(statement, strings.Repeat("b", 64))
		} else {
			_, execErr = db.Exec(statement)
		}
		if execErr == nil || !strings.Contains(execErr.Error(), "awaiting-authority") {
			t.Fatalf("%s guard err=%v", name, execErr)
		}
	}
	var started int
	if err = db.QueryRow(`SELECT count(*) FROM change_log WHERE event_type='outcome_attempt_started' AND json_extract(payload,'$.attemptId')='guard-att'`).Scan(&started); err != nil || started != 1 {
		t.Fatalf("awaiting CDC count=%d err=%v", started, err)
	}
}
