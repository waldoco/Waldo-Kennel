package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func TestAttemptCustodyFencesMigrationSealsRowsWithUpdateGuard(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "kennel.db")+pragmas)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpTo(db, "migrations", 159, goose.WithAllowMissing()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	seedMeasurementMigrationLineage(t, ctx, db)
	if err := goose.UpTo(db, "migrations", 160, goose.WithAllowMissing()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO attempt_custody_fences(attempt_id,session_id,detail,fenced_at) VALUES('att','ses','provider exited','2026-09-20 02:00:00')`); err != nil {
		t.Fatalf("fence insert: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO attempt_custody_fences(attempt_id,session_id,detail,fenced_at) VALUES('att','ses2','second close','2026-09-20 02:01:00')`); err == nil {
		t.Fatal("second fence for one attempt was allowed")
	}
	if _, err := db.ExecContext(ctx, `UPDATE attempt_custody_fences SET detail='rewritten' WHERE attempt_id='att'`); err == nil {
		t.Fatal("recorded fence was mutable")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM attempt_custody_fences WHERE attempt_id='att'`); err != nil {
		t.Fatalf("outcome-scope erasure must stay possible: %v", err)
	}
	if err := goose.DownTo(db, "migrations", 159); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='attempt_custody_fences'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("table survived down migration: count=%d err=%v", count, err)
	}
	if err := goose.UpTo(db, "migrations", 160, goose.WithAllowMissing()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO attempt_custody_fences(attempt_id,session_id,detail,fenced_at) VALUES('att','ses','provider exited','2026-09-20 02:00:00')`); err != nil {
		t.Fatalf("insert after re-up: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE attempt_custody_fences SET session_id='sesX' WHERE attempt_id='att'`); err == nil {
		t.Fatal("re-up guard was not restored")
	}
}
