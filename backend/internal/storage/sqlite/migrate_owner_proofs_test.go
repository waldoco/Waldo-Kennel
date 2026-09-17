package sqlite

import (
	"context"
	"database/sql"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
	"path/filepath"
	"slices"
	"testing"
)

func TestOwnerProofMigrationUpgradeAndRollback(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "kennel.db")+pragmas)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	upTo(t, db, 147)
	ctx := context.Background()
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := db.ExecContext(ctx, `INSERT INTO owner_proofs(id,verifier,app_run_id,mission_id,content_digest,target_id,target_generation,command_class,confirmation_ref,expires_at,created_at) VALUES('p',?,'run','mission',?,'target',1,'turn','','2026-09-18T00:00:00Z','2026-09-17T00:00:00Z')`, digest, digest); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpTo(db, "migrations", 148, goose.WithAllowMissing()); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM owner_proofs`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("legacy proof count after upgrade = %d, want 0", n)
	}
	for _, table := range []string{"chat_command_targets", "owner_answer_questions", "command_authority_claims", "harness_command_outbox"} {
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n); err != nil || n != 1 {
			t.Fatalf("upgraded table %s count = %d, err = %v", table, n, err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO owner_answer_questions(id,conversation_id,request_id,generation,status,created_at,updated_at) VALUES('q','missing','r','g','pending',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err == nil {
		t.Fatal("question with missing conversation unexpectedly inserted")
	}
	if err := goose.DownTo(db, "migrations", 147); err != nil {
		t.Fatal(err)
	}
	if got := tableColumns(t, db, "owner_proofs"); !containsAll(got, "target_id", "target_generation") {
		t.Fatalf("rolled-back owner_proofs columns = %v", got)
	}
	for _, table := range []string{"chat_command_targets", "owner_answer_questions", "command_authority_claims", "harness_command_outbox"} {
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("rolled-back table %s count = %d, err = %v", table, n, err)
		}
	}
	if err := goose.DownTo(db, "migrations", 145); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='owner_proofs'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("owner_proofs survived rollback below its introduction")
	}
}

func containsAll(values []string, wants ...string) bool {
	for _, want := range wants {
		if !slices.Contains(values, want) {
			return false
		}
	}
	return true
}
