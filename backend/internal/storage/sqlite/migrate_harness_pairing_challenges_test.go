package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func TestHarnessPairingChallengeMigrationUpgradeAndRollback(t *testing.T) {
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
	if _, err := db.ExecContext(ctx, `INSERT INTO harness_pairing_challenges (id,kind,connection_id,installation_id,adapter_digest,harness_identity,provider_version,protocol_fingerprint,mission_id,app_run_id,capability_classes,expected_generation,proof_verifier,status,connection_expires_at,expires_at,created_at,updated_at) VALUES ('ch','pair','hc','installation',?,'codex','0.154.0',?,'mission','run','turn',1,?,'pending','2026-09-18T00:00:00Z','2026-09-18T00:00:00Z','2026-09-17T00:00:00Z','2026-09-17T00:00:00Z')`, digest64, digest64, digest64); err != nil {
		t.Fatal(err)
	}
	// The binding-immutable trigger must still allow the lifecycle transition
	// this table exists for: status/result_code/updated_at can change.
	if _, err := db.ExecContext(ctx, `UPDATE harness_pairing_challenges SET status='consumed', result_code='succeeded', updated_at='2026-09-17T00:01:00Z' WHERE id='ch'`); err != nil {
		t.Fatalf("lifecycle transition must remain mutable: %v", err)
	}
	// But the bound tuple itself must stay immutable once written.
	if _, err := db.ExecContext(ctx, `UPDATE harness_pairing_challenges SET installation_id='other' WHERE id='ch'`); err == nil {
		t.Fatal("expected binding-immutable trigger to reject a tuple change")
	}
	if err := goose.DownTo(db, "migrations", 145); err != nil {
		t.Fatal(err)
	}
	var present int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='harness_pairing_challenges'`).Scan(&present); err != nil {
		t.Fatal(err)
	}
	if present != 0 {
		t.Fatal("harness_pairing_challenges survived rollback to 0145")
	}
	var connectionsPresent int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='harness_connections'`).Scan(&connectionsPresent); err != nil {
		t.Fatal(err)
	}
	if connectionsPresent != 1 {
		t.Fatal("rollback to 0145 must leave harness_connections intact")
	}
}
