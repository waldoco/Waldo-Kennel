package outcome_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/intelligence/intelligencetest"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
)

// This is the migration compatibility path end to end. It first creates the
// complete historical executable prerequisites, then rewrites the fixture DB
// to the exact pre-0155 schema and digest shape before reopening through the
// current migrations. Production never performs these fixture rewrites.
func TestStartAttemptAfterPre0155ApprovedPlanMigration(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := sqlite.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	projectID := domain.ProjectID("legacy-sqlite")
	if err := store.UpsertProject(ctx, domain.ProjectRecord{ID: string(projectID), Path: t.TempDir(), RegisteredAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessCodex, "")}}
	spawner := &fakeSpawner{readiness: ports.AgentProfileReadiness{Ready: true}}
	svc := outcome.New(store, nil).WithPlanning(intelligencetest.New(), router).WithExecution(spawner, newFakeHeartbeats())
	svc.AdmissionPolicy = testAdmissionPolicy()
	created, err := svc.Create(ctx, outcome.CreateInput{ProjectID: projectID, Title: "Legacy", Goal: "Deliver local result", SuccessCriteria: []string{"result exists"}, Review: "inspect", AuthorityCeiling: domain.ProposedAuthority{ReadWorkspace: true, WriteWorkspace: true, ExecuteLocal: true}, RequestKey: "legacy-create"})
	if err != nil {
		t.Fatal(err)
	}
	proposed, err := svc.ProposePlan(ctx, created.Outcome.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ApprovePlan(ctx, created.Outcome.ID, outcome.ApprovePlanInput{PlanRevisionID: proposed.Plan.ID, ExpectedContractRevision: 1}); err != nil {
		t.Fatal(err)
	}
	plan, found, err := store.GetPlanRevision(ctx, created.Outcome.ID, proposed.Plan.ID)
	if err != nil || !found {
		t.Fatalf("read plan found=%v err=%v", found, err)
	}
	revisions, err := store.ListContractRevisions(ctx, created.Outcome.ID)
	if err != nil || len(revisions) != 1 {
		t.Fatalf("read contract count=%d err=%v", len(revisions), err)
	}
	revision := revisions[0]
	for i := range plan.WorkUnits {
		plan.WorkUnits[i].Role = domain.WorkUnitRoleLegacy
		plan.WorkUnits[i].Inputs = nil
	}
	oldDigest, err := domain.ComputePlanRunBriefCoreDigest(revision, plan.WorkUnits, plan.Grants)
	if err != nil {
		t.Fatal(err)
	}
	verdict, found, err := store.GetAdmittedVerdict(ctx, plan.ID)
	if err != nil || !found {
		t.Fatalf("read verdict found=%v err=%v", found, err)
	}
	for i := range verdict.WorkUnits {
		spec := verdict.WorkUnits[i].Executable
		spec.RunBriefCoreDigest = oldDigest
		spec.Digest, err = spec.ComputedDigest()
		if err != nil {
			t.Fatal(err)
		}
		verdict.WorkUnits[i].Executable = spec
	}
	verdictJSON, _ := json.Marshal(verdict)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "kennel.db")+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	// Preserve the pre-0155 immutability triggers while making the already-created
	// fixture data historical. They must exist when the current store reopens it.
	immutableTriggers := []string{
		"plan_revisions_immutable_update",
		"work_units_immutable_update",
		"approved_executable_specs_immutable_update",
		"admission_verdicts_immutable_update",
	}
	triggerSQL := make([]string, 0, len(immutableTriggers))
	for _, name := range immutableTriggers {
		var definition string
		if err = db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='trigger' AND name=?`, name).Scan(&definition); err != nil {
			t.Fatalf("read trigger %s: %v", name, err)
		}
		triggerSQL = append(triggerSQL, definition)
		if _, err = db.Exec(`DROP TRIGGER ` + name); err != nil {
			t.Fatalf("drop trigger %s: %v", name, err)
		}
	}
	if _, err = db.Exec(`UPDATE plan_revisions SET run_brief_core_digest=? WHERE id=?`, oldDigest, plan.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE admission_verdicts SET verdict_json=? WHERE plan_revision_id=?`, string(verdictJSON), plan.ID); err != nil {
		t.Fatal(err)
	}
	for _, wu := range verdict.WorkUnits {
		raw, _ := json.Marshal(wu.Executable)
		if _, err = db.Exec(`UPDATE approved_executable_specs SET digest=?,spec_json=? WHERE plan_revision_id=? AND work_unit_id=?`, wu.Executable.Digest, string(raw), plan.ID, wu.WorkUnitID); err != nil {
			t.Fatal(err)
		}
	}
	for _, definition := range triggerSQL {
		if _, err = db.Exec(definition); err != nil {
			t.Fatalf("restore historical immutability trigger: %v", err)
		}
	}
	// Remove every schema effect of 0155 and its applied-version marker. The next
	// sqlite.Open must therefore execute the real migration, not merely hydrate
	// legacy values from an already-current schema.
	for _, stmt := range []string{
		`DROP TRIGGER work_unit_inputs_immutable_update`,
		`DROP TRIGGER work_unit_inputs_immutable_delete`,
		`DROP TABLE work_unit_inputs`,
		`ALTER TABLE work_units DROP COLUMN role`,
		`DELETE FROM goose_db_version WHERE version_id=155`,
	} {
		if _, err = db.Exec(stmt); err != nil {
			t.Fatalf("restore pre-0155 schema with %q: %v", stmt, err)
		}
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	migrated, err := sqlite.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	readback, found, err := migrated.GetPlanRevision(ctx, created.Outcome.ID, plan.ID)
	if err != nil || !found {
		t.Fatalf("migrated read found=%v err=%v", found, err)
	}
	if readback.WorkUnits[0].Role != domain.WorkUnitRoleLegacy || readback.RunBriefCoreDigest != oldDigest {
		t.Fatalf("readback role/digest=%s/%s", readback.WorkUnits[0].Role, readback.RunBriefCoreDigest)
	}
	run := outcome.New(migrated, nil).WithPlanning(intelligencetest.New(), router).WithExecution(spawner, newFakeHeartbeats())
	run.AdmissionPolicy = testAdmissionPolicy()
	_, err = run.StartAttempt(ctx, created.Outcome.ID, outcome.StartAttemptInput{PlanRevisionID: plan.ID, WorkUnitID: readback.WorkUnits[0].ID, RequestKey: "legacy-start"})
	if err != nil {
		t.Fatalf("start migrated approved Plan: %v", err)
	}
	if spawner.spawnCalls() != 1 {
		t.Fatalf("spawn calls=%d", spawner.spawnCalls())
	}
	// The immutable historical Plan identity is still the old digest after start.
	after, _, _ := migrated.GetPlanRevision(ctx, created.Outcome.ID, plan.ID)
	if after.RunBriefCoreDigest != oldDigest || !strings.EqualFold(after.RunBriefCoreDigest, readback.RunBriefCoreDigest) {
		t.Fatal("start rewrote historical Plan")
	}
}
