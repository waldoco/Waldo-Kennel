package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func TestAttemptManifestsMigrationSealsRowsWithUpdateGuard(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "kennel.db")+pragmas)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpTo(db, "migrations", 158, goose.WithAllowMissing()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	seedMeasurementMigrationLineage(t, ctx, db)
	if err := goose.UpTo(db, "migrations", 159, goose.WithAllowMissing()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO attempt_manifests(attempt_id,half,outcome_id,payload,payload_digest) VALUES('att','input','out','{}','d')`); err != nil {
		t.Fatalf("sealed insert: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO attempt_manifests(attempt_id,half,outcome_id,payload,payload_digest) VALUES('att','input','out','{}','d2')`); err == nil {
		t.Fatal("second write to a sealed half was allowed")
	}
	if _, err := db.ExecContext(ctx, `UPDATE attempt_manifests SET payload='{}x' WHERE attempt_id='att'`); err == nil {
		t.Fatal("sealed manifest was mutable")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM attempt_manifests WHERE attempt_id='att'`); err != nil {
		t.Fatalf("outcome-scope erasure must stay possible: %v", err)
	}
	if err := goose.DownTo(db, "migrations", 158); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='attempt_manifests'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("table survived down migration: count=%d err=%v", count, err)
	}
	if err := goose.UpTo(db, "migrations", 159, goose.WithAllowMissing()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO attempt_manifests(attempt_id,half,outcome_id,payload,payload_digest) VALUES('att','output','out','{}','d')`); err != nil {
		t.Fatalf("insert after re-up: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE attempt_manifests SET payload_digest='e' WHERE attempt_id='att'`); err == nil {
		t.Fatal("re-up guard was not restored")
	}
}
