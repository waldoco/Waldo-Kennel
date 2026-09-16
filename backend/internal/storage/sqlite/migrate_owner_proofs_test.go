package sqlite

import (
	"context"
	"database/sql"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
	"path/filepath"
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
	if err := goose.Up(db, "migrations", goose.WithAllowMissing()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := db.ExecContext(ctx, `INSERT INTO owner_proofs(id,verifier,app_run_id,mission_id,content_digest,target_id,target_generation,command_class,confirmation_ref,expires_at,created_at) VALUES('p',?,'run','mission',?,'target',1,'turn','','2026-09-18T00:00:00Z','2026-09-17T00:00:00Z')`, digest, digest); err != nil {
		t.Fatal(err)
	}
	if err := goose.DownTo(db, "migrations", 145); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='owner_proofs'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("owner_proofs survived rollback")
	}
}
