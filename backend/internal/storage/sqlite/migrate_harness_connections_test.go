package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func TestHarnessConnectionMigrationUpgradeAndRollback(t *testing.T) {
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
	if _, err := db.ExecContext(ctx, `INSERT INTO harness_connections (id,installation_id,adapter_digest,harness_identity,provider_version,protocol_fingerprint,mission_id,app_run_id,capability_classes,capability_verifier,generation,expires_at,created_at,updated_at) VALUES ('hc','installation',?, 'codex','0.154.0',?,'mission','run','turn',?,1,'2026-09-18T00:00:00Z','2026-09-17T00:00:00Z','2026-09-17T00:00:00Z')`, digest64, digest64, digest64); err != nil {
		t.Fatal(err)
	}
	if err := goose.DownTo(db, "migrations", 144); err != nil {
		t.Fatal(err)
	}
	var present int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='harness_connections'`).Scan(&present); err != nil {
		t.Fatal(err)
	}
	if present != 0 {
		t.Fatal("harness_connections survived rollback")
	}
}

const digest64 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
