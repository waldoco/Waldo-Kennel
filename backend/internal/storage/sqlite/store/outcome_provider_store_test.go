package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
)

func seedProviderPlanOutcome(t *testing.T, s *sqlite.Store) domain.ContractRevision {
	t.Helper()
	ctx := context.Background()
	seedProject(t, s, "provider-project")
	space, err := s.EnsureWorkResponsibilitySpace(ctx, "provider-project")
	if err != nil {
		t.Fatalf("ensure provider project space: %v", err)
	}
	outcome, revision := focusLedgerContract(space.ID, "provider-plan")
	outcome.ID = "out-provider-plan"
	revision.ID = "cr-provider-plan"
	revision.OutcomeID = outcome.ID
	if err := s.CreateOutcomeWithContract(ctx, outcome, revision, "req-provider-plan"); err != nil {
		t.Fatalf("create provider plan outcome: %v", err)
	}
	history, err := s.ListContractRevisions(ctx, outcome.ID)
	if err != nil || len(history) != 1 {
		t.Fatalf("read seeded contract history: len=%d err=%v", len(history), err)
	}
	return history[0]
}

func recommendedRouting(unit domain.WorkUnit, candidateID string) domain.WorkUnitRoutingDecision {
	return domain.WorkUnitRoutingDecision{
		WorkUnitID: unit.ID,
		Decision: domain.RoutingDecision{
			Status:                    domain.RoutingDecisionRecommended,
			PolicyVersion:             domain.RoutingPolicyVersion,
			Role:                      domain.RoutingRoleWorker,
			RecommendedCandidateID:    candidateID,
			RecommendedProvider:       string(unit.Provider),
			RecommendedModelSelection: unit.ModelSelection,
			RecommendedModel:          unit.Model,
		},
	}
}

func canonicalGraphPlan(t *testing.T, revision domain.ContractRevision) domain.PlanRevision {
	t.Helper()
	if len(revision.Criteria) == 0 {
		t.Fatal("seeded contract has no canonical criteria")
	}
	criterionIDs := make([]domain.CriterionID, 0, len(revision.Criteria))
	for _, criterion := range revision.Criteria {
		criterionIDs = append(criterionIDs, criterion.ID)
	}

	inspect := domain.WorkUnit{
		ID: "wu-inspect", Kind: domain.WorkUnitDirect, Title: "Inspect the repository",
		ContractRevisionNumber: revision.Number,
		Provider:               domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault,
		OutputSummary: "A bounded repository assessment.", EvidenceChecks: []string{"repository state inspected"},
		VerificationRequirement: "inspection is captured", StopConditions: []string{"stop before writes"},
		CriterionIDs: []domain.CriterionID{criterionIDs[0]}, RequiredCapabilities: []string{domain.CapabilityWorktreeRead},
	}
	change := domain.WorkUnit{
		ID: "wu-change", Kind: domain.WorkUnitDirect, Title: "Make the bounded change",
		ContractRevisionNumber: revision.Number,
		Provider:               domain.HarnessClaudeCode, ModelSelection: domain.ExecutionBindingModelExplicit, Model: "sonnet-test",
		OutputSummary: "The requested change is ready for review.", EvidenceChecks: []string{"change is inspectable"},
		VerificationRequirement: "deterministic verification passes", StopConditions: []string{"stop before remote effects"},
		DependsOn: []domain.WorkUnitID{inspect.ID}, CriterionIDs: criterionIDs,
		RequiredCapabilities: []string{domain.CapabilityWorktreeRead, domain.CapabilityWorktreeWrite},
	}
	grants := []domain.CapabilityGrant{
		{ID: "cg-read", Name: domain.CapabilityWorktreeRead, Scope: "worktree/*"},
		{ID: "cg-write", Name: domain.CapabilityWorktreeWrite, Scope: "worktree/*"},
	}
	digest, err := domain.ComputePlanRunBriefCoreDigest(revision, []domain.WorkUnit{inspect, change}, grants)
	if err != nil {
		t.Fatalf("compute graph digest: %v", err)
	}
	return domain.PlanRevision{
		ID: "plan-graph", OutcomeID: revision.OutcomeID, ContractRevisionNumber: revision.Number,
		Status: domain.PlanStatusProposed, Summary: "Inspect, then make the bounded change.",
		WorkUnits: []domain.WorkUnit{change, inspect}, // deliberately reverse serialization order
		Grants:    grants,
		RoutingDecisions: []domain.WorkUnitRoutingDecision{
			recommendedRouting(change, "claude-candidate"),
			recommendedRouting(inspect, "codex-candidate"),
		},
		RunBriefCoreDigest: digest,
	}
}

func TestOutcomeStore_CanonicalPlanGraphRoundTripsExactly(t *testing.T) {
	s := newTestStore(t)
	revision := seedProviderPlanOutcome(t, s)
	plan := canonicalGraphPlan(t, revision)

	saved, err := s.AppendPlanRevision(context.Background(), plan.OutcomeID, plan)
	if err != nil {
		t.Fatalf("append canonical graph plan: %v", err)
	}
	got, found, err := s.GetPlanRevision(context.Background(), plan.OutcomeID, saved.ID)
	if err != nil || !found {
		t.Fatalf("get canonical graph plan found=%v err=%v", found, err)
	}
	if len(got.WorkUnits) != 2 || len(got.RoutingDecisions) != 2 {
		t.Fatalf("readback workUnits=%d routing=%d", len(got.WorkUnits), len(got.RoutingDecisions))
	}
	if err := got.ValidateAgainstContract(revision); err != nil {
		t.Fatalf("criterion coverage did not round-trip: %v", err)
	}
	if err := domain.ValidateExactPlanCapabilityGrants(got.Grants, got.WorkUnits); err != nil {
		t.Fatalf("required capabilities/grants did not round-trip: %v", err)
	}
	order, err := got.TopologicalWorkUnits()
	if err != nil {
		t.Fatalf("topological readback: %v", err)
	}
	if len(order) != 2 || order[0].ID != "wu-inspect" || order[1].ID != "wu-change" {
		t.Fatalf("topological order=%v", []domain.WorkUnitID{order[0].ID, order[1].ID})
	}
	for _, unit := range got.WorkUnits {
		binding, err := unit.ExecutionBindingForNewWork()
		if err != nil {
			t.Fatalf("work unit %s lost exact execution binding: %v", unit.ID, err)
		}
		var matching *domain.WorkUnitRoutingDecision
		for i := range got.RoutingDecisions {
			if got.RoutingDecisions[i].WorkUnitID == unit.ID {
				matching = &got.RoutingDecisions[i]
				break
			}
		}
		if matching == nil {
			t.Fatalf("work unit %s lost routing provenance", unit.ID)
		}
		if err := matching.ValidateAgainst(unit); err != nil {
			t.Fatalf("routing/binding mismatch after readback for %s: %v (binding=%+v)", unit.ID, err, binding)
		}
	}
}

func TestOutcomeStore_CanonicalPlanRejectsUnusedCapabilityGrant(t *testing.T) {
	s := newTestStore(t)
	revision := seedProviderPlanOutcome(t, s)
	plan := canonicalGraphPlan(t, revision)
	plan.Grants = append(plan.Grants, domain.CapabilityGrant{ID: "cg-exec", Name: domain.CapabilityWorktreeExec, Scope: "worktree/*"})

	if _, err := s.AppendPlanRevision(context.Background(), plan.OutcomeID, plan); err == nil {
		t.Fatal("canonical writer accepted an unused exec grant")
	}
}

func TestOutcomeStore_LegacyNullPositionsReadBackInHistoricalLexicalIDOrder(t *testing.T) {
	dataDir := t.TempDir()
	s, err := sqlitetest.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	revision := seedProviderPlanOutcome(t, s)
	plan := canonicalGraphPlan(t, revision)
	saved, err := s.AppendPlanRevision(context.Background(), plan.OutcomeID, plan)
	if err != nil {
		t.Fatalf("append plan: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "kennel.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TRIGGER work_units_immutable_update`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE work_units SET position=NULL WHERE plan_revision_id=?`, saved.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER work_units_immutable_update BEFORE UPDATE ON work_units BEGIN SELECT RAISE(ABORT, 'work units are immutable'); END`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = sqlite.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, found, err := s.GetPlanRevision(context.Background(), plan.OutcomeID, saved.ID)
	if err != nil || !found {
		t.Fatalf("legacy readback found=%v err=%v", found, err)
	}
	if len(got.WorkUnits) != 2 || got.WorkUnits[0].ID != "wu-change" || got.WorkUnits[1].ID != "wu-inspect" {
		t.Fatalf("legacy work unit order = %+v, want lexical IDs", got.WorkUnits)
	}
	if got.WorkUnits[0].Position != 0 || got.WorkUnits[1].Position != 0 {
		t.Fatalf("legacy positions should remain explicitly unset: %+v", got.WorkUnits)
	}
}
