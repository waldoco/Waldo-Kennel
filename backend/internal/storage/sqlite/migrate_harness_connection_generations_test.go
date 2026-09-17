package sqlite

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func TestHarnessConnectionGenerationRotationGuardsAndRollback(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "m.db")+pragmas)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	goose.SetBaseFS(migrationsFS)
	if err = goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	upTo(t, db, 152)
	if err = goose.UpTo(db, "migrations", 153, goose.WithAllowMissing()); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 17, 1, 0, 0, 0, time.UTC)
	digest := strings.Repeat("a", 64)
	verifier := strings.Repeat("b", 64)
	nextVerifier := strings.Repeat("c", 64)
	_, err = db.Exec(`INSERT INTO harness_connections(id,installation_id,adapter_digest,harness_identity,provider_version,protocol_fingerprint,mission_id,app_run_id,capability_classes,capability_verifier,generation,expires_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, "c1", "i", digest, "codex", "1", digest, "m", "old", `["turn"]`, verifier, 1, now.Add(time.Hour), now, now)
	if err != nil {
		t.Fatal(err)
	}
	mustReject := func(q string, args ...any) {
		t.Helper()
		if _, e := db.Exec(q, args...); e == nil {
			t.Fatalf("accepted raw update: %s", q)
		}
	}
	mustReject(`UPDATE harness_connections SET app_run_id='new' WHERE id='c1'`)
	mustReject(`UPDATE harness_connections SET app_run_id='new',generation=2,capability_verifier=? WHERE id='c1'`, nextVerifier)
	// A mismatched archive cannot satisfy the guard.
	_, err = db.Exec(`INSERT INTO harness_connection_generations SELECT id,generation,installation_id,adapter_digest,harness_identity,provider_version,protocol_fingerprint,mission_id,'wrong',capability_classes,capability_verifier,expires_at,revoked_at,created_at,updated_at FROM harness_connections WHERE id='c1'`)
	if err != nil {
		t.Fatal(err)
	}
	mustReject(`UPDATE harness_connections SET app_run_id='new',generation=2,capability_verifier=?,expires_at=?,updated_at=? WHERE id='c1'`, nextVerifier, now.Add(2*time.Hour), now.Add(time.Minute))
	if _, err = db.Exec(`DELETE FROM harness_connection_generations WHERE connection_id='c1'`); err == nil {
		t.Fatal("history delete accepted")
	}
	if _, err = db.Exec(`UPDATE harness_connection_generations SET app_run_id='old' WHERE connection_id='c1'`); err == nil {
		t.Fatal("history update accepted")
	}
	// Remove mismatch by recreating fixture under another connection for legitimate path.
	_, err = db.Exec(`INSERT INTO harness_connections(id,installation_id,adapter_digest,harness_identity,provider_version,protocol_fingerprint,mission_id,app_run_id,capability_classes,capability_verifier,generation,expires_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, "c2", "i2", digest, "codex", "1", digest, "m", "old", `["turn"]`, verifier, 1, now.Add(time.Hour), now, now)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(`INSERT INTO harness_connection_generations SELECT id,generation,installation_id,adapter_digest,harness_identity,provider_version,protocol_fingerprint,mission_id,app_run_id,capability_classes,capability_verifier,expires_at,revoked_at,created_at,updated_at FROM harness_connections WHERE id='c2'`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(`UPDATE harness_connections SET app_run_id='new',generation=2,capability_verifier=?,expires_at=?,updated_at=? WHERE id='c2'`, nextVerifier, now.Add(2*time.Hour), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	// Injected failure after archive rolls archive back.
	_, err = db.Exec(`INSERT INTO harness_connections(id,installation_id,adapter_digest,harness_identity,provider_version,protocol_fingerprint,mission_id,app_run_id,capability_classes,capability_verifier,generation,expires_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, "c3", "i3", digest, "codex", "1", digest, "m", "old", `["turn"]`, verifier, 1, now.Add(time.Hour), now, now)
	if err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(`INSERT INTO harness_connection_generations SELECT id,generation,installation_id,adapter_digest,harness_identity,provider_version,protocol_fingerprint,mission_id,app_run_id,capability_classes,capability_verifier,expires_at,revoked_at,created_at,updated_at FROM harness_connections WHERE id='c3'`)
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback()
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM harness_connection_generations WHERE connection_id='c3'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rollback archive=%d err=%v", n, err)
	}
	if err = goose.DownTo(db, "migrations", 152); err != nil {
		t.Fatal(err)
	}
	var trigger string
	if err = db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='trigger' AND name='harness_connections_binding_immutable'`).Scan(&trigger); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(trigger, "NEW.app_run_id<>OLD.app_run_id") {
		t.Fatalf("strict trigger not restored: %s", trigger)
	}
}
