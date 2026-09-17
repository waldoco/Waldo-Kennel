package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
)

func TestAdmissionPacketTablesAreInstalled(t *testing.T) {
	store, err := sqlite.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, found, err := store.GetApprovedExecutableSpec(context.Background(), "missing", "missing"); err != nil || found {
		t.Fatalf("missing spec found=%v err=%v", found, err)
	}
	if _, found, err := store.GetWorkspaceBoundLaunchPacket(context.Background(), domain.AttemptID("missing")); err != nil || found {
		t.Fatalf("missing packet found=%v err=%v", found, err)
	}
}

func TestAdmissionEvidenceIsImmutableAndSQLJSONIdentityIsChecked(t *testing.T) {
	dir := t.TempDir()
	store, err := sqlite.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db, err := sql.Open("sqlite", filepath.Join(dir, "kennel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO admission_verdicts(id,outcome_id,status,policy_version,evaluated_at,verdict_json) VALUES('v','o','rejected','p',CURRENT_TIMESTAMP,'{"id":"v","outcomeId":"o","evaluatedAt":"2026-01-01T00:00:00Z","status":"rejected","policyVersion":"p","reasons":["provider_unavailable"]}')`); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{`UPDATE admission_verdicts SET policy_version='x' WHERE id='v'`, `DELETE FROM admission_verdicts WHERE id='v'`} {
		if _, err = db.Exec(statement); err == nil {
			t.Fatalf("immutable evidence accepted %s", statement)
		}
	}
	if _, err = db.Exec(`INSERT INTO approved_executable_specs(plan_revision_id,work_unit_id,digest,spec_json) VALUES('plan-sql','wu-sql','` + strings.Repeat("a", 64) + `','{"planRevisionId":"plan-json","workUnitId":"wu-sql","digest":"` + strings.Repeat("a", 64) + `"}')`); err == nil {
		t.Fatal("SQL/JSON identity corruption was accepted")
	}
}
