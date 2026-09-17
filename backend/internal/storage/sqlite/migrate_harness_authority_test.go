package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func TestHarnessAuthorityMigrationUpgradeAndRollback(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "kennel.db")+pragmas)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	upTo(t, db, 149)
	if err := goose.UpTo(db, "migrations", 150, goose.WithAllowMissing()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var n int
	for _, table := range []string{"harness_pairing_intents", "harness_authority_receipts"} {
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n); err != nil || n != 1 {
			t.Fatalf("table %s count=%d err=%v", table, n, err)
		}
	}
	// B2 intentionally replaces only the claim trigger so revocation can record its mutable consequence fields.
	var sqlText string
	if err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='trigger' AND name='command_authority_claims_immutable'`).Scan(&sqlText); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sqlText, "connection_revoked_at") {
		t.Fatalf("B2 trigger still makes revocation evidence immutable: %s", sqlText)
	}
	if err := goose.DownTo(db, "migrations", 149); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"harness_pairing_intents", "harness_authority_receipts"} {
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("rolled back table %s count=%d err=%v", table, n, err)
		}
	}
	if err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='trigger' AND name='command_authority_claims_immutable'`).Scan(&sqlText); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sqlText, "connection_revoked_at") {
		t.Fatalf("rollback did not restore prior immutable trigger: %s", sqlText)
	}
}
