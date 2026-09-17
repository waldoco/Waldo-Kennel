package sqlite

import (
	"context"
	"database/sql"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttemptArtifactMeasurementsUpDownUpPreservesFrozenRowsAndGuards(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "kennel.db")+pragmas)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpTo(db, "migrations", 155, goose.WithAllowMissing()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	seedMeasurementMigrationLineage(t, ctx, db)
	if err := goose.UpTo(db, "migrations", 156, goose.WithAllowMissing()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE attempt_artifact_files SET additions=3,deletions=1 WHERE id='file'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE attempt_receipts SET frozen_at='2026-09-18T00:00:00Z' WHERE attempt_id='att'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE attempt_artifact_files SET additions=4 WHERE id='file'`); err == nil {
		t.Fatal("frozen measured fact was mutable")
	}
	if err := goose.DownTo(db, "migrations", 155); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM attempt_artifact_files WHERE id='file'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("row lost: count=%d err=%v", count, err)
	}
	var cols int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info('attempt_artifact_files') WHERE name IN ('additions','deletions')`).Scan(&cols); err != nil || cols != 0 {
		t.Fatalf("down columns=%d err=%v", cols, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE attempt_artifact_files SET relative_path='other.txt' WHERE id='file'`); err == nil {
		t.Fatal("pre-0156 frozen guard was not restored")
	}
	if err := goose.UpTo(db, "migrations", 156, goose.WithAllowMissing()); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info('attempt_artifact_files') WHERE name IN ('additions','deletions')`).Scan(&cols); err != nil || cols != 2 {
		t.Fatalf("re-up columns=%d err=%v", cols, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE attempt_artifact_files SET additions=1,deletions=0 WHERE id='file'`); err == nil {
		t.Fatal("re-up frozen measured guard was not restored")
	}
}

func seedMeasurementMigrationLineage(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	digest := strings.Repeat("a", 64)
	statements := []string{
		`INSERT INTO projects(id,path,display_name,registered_at,archived_at,kind) VALUES('p','/tmp/p','P','2026-09-17T00:00:00Z',NULL,'single_repo')`,
		`INSERT INTO responsibility_spaces(id,kind,project_id,created_at) VALUES('rsp','WorkProject','p','2026-09-17T00:00:00Z')`,
		`INSERT INTO outcomes(id,space_id,title,current_revision_number,created_at,updated_at) VALUES('out','rsp','Outcome',1,'2026-09-17T00:00:00Z','2026-09-17T00:00:00Z')`,
		`INSERT INTO contract_revisions(id,outcome_id,number,goal,success_criteria,review,constraints,non_goals,clarification,created_at) VALUES('cr','out',1,'g','["c"]','r','[]','[]','','2026-09-17T00:00:00Z')`,
		`INSERT INTO plan_revisions(id,outcome_id,number,contract_revision_number,status,summary,run_brief_core_digest,created_at) VALUES('plan','out',1,1,'approved','p','` + digest + `','2026-09-17T00:00:00Z')`,
		`INSERT INTO work_units(id,plan_revision_id,kind,title,contract_revision_number,output_summary,evidence_checks,verification_requirement,stop_conditions,position,role) VALUES('wu','plan','direct','W',1,'o','["e"]','v','["s"]',1,'implement')`,
		`INSERT INTO attempts(id,outcome_id,plan_revision_id,work_unit_id,number,status,contract_revision_number,request_key,created_at,updated_at) VALUES('att','out','plan','wu',1,'reconciled',1,'rk','2026-09-17T00:00:00Z','2026-09-17T00:00:00Z')`,
		`INSERT INTO attempt_receipts(attempt_id,outcome_id,plan_revision_id,work_unit_id,contract_revision_number,artifact_version,workspace_kind,retention_state,observed_at,created_at,updated_at) VALUES('att','out','plan','wu',1,'v','git_worktree','retained','2026-09-17T00:00:00Z','2026-09-17T00:00:00Z','2026-09-17T00:00:00Z')`,
		`INSERT INTO attempt_artifact_files(id,attempt_id,relative_path,change_kind,content_digest) VALUES('file','att','a.txt','modified','d')`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed %s: %v", statement, err)
		}
	}
}
