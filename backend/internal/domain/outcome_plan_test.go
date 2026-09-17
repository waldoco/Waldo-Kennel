package domain

import (
	"strings"
	"testing"
)

func validWorkUnit() WorkUnit {
	return WorkUnit{
		ID:                      "wu-test",
		Kind:                    WorkUnitDirect,
		Title:                   "Build and prove the feature",
		ContractRevisionNumber:  1,
		OutputSummary:           "Working local feature in the isolated worktree",
		EvidenceChecks:          []string{"deterministic test suite passes"},
		VerificationRequirement: "verification runs outside the producer session",
		StopConditions:          []string{"stop before any unapproved external effect"},
	}
}

func validGrants() []CapabilityGrant {
	return []CapabilityGrant{
		{ID: "cg-read", Name: CapabilityWorktreeRead, Scope: "worktree/*"},
		{ID: "cg-write", Name: CapabilityWorktreeWrite, Scope: "worktree/*"},
		{ID: "cg-exec", Name: CapabilityWorktreeExec, Scope: "worktree/*"},
	}
}

func validPlanRevision() PlanRevision {
	return PlanRevision{
		ID: "plan-test", OutcomeID: "out-test", Number: 1, ContractRevisionNumber: 1,
		Status: PlanStatusProposed, Summary: "A direct execution graph",
		WorkUnits: []WorkUnit{validWorkUnit()}, Grants: validGrants(),
		RunBriefCoreDigest: strings.Repeat("a", 64),
	}
}

func TestPlanRevisionSupportsBoundedWorkUnitGraph(t *testing.T) {
	first := validWorkUnit()
	first.ID = "wu-a"
	second := validWorkUnit()
	second.ID = "wu-b"
	second.DependsOn = []WorkUnitID{first.ID}
	plan := validPlanRevision()
	plan.WorkUnits = []WorkUnit{second, first} // serialization order is intentionally reversed
	if err := plan.Validate(); err != nil {
		t.Fatalf("multi-unit graph should be valid: %v", err)
	}
	ordered, err := plan.TopologicalWorkUnits()
	if err != nil {
		t.Fatalf("topological order: %v", err)
	}
	if len(ordered) != 2 || ordered[0].ID != first.ID || ordered[1].ID != second.ID {
		t.Fatalf("topological order = %+v", ordered)
	}
}

func TestPlanRevisionRejectsDependencyCycle(t *testing.T) {
	first := validWorkUnit()
	first.ID = "wu-a"
	first.DependsOn = []WorkUnitID{"wu-b"}
	second := validWorkUnit()
	second.ID = "wu-b"
	second.DependsOn = []WorkUnitID{"wu-a"}
	plan := validPlanRevision()
	plan.WorkUnits = []WorkUnit{first, second}
	if err := plan.Validate(); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle validation = %v", err)
	}
}

func TestPlanRevisionValidationBasics(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*PlanRevision)
		want   string
	}{
		{"missing id", func(p *PlanRevision) { p.ID = "" }, "id is required"},
		{"missing outcome", func(p *PlanRevision) { p.OutcomeID = "" }, "outcome id is required"},
		{"no work units", func(p *PlanRevision) { p.WorkUnits = nil }, "at least one work unit"},
		{"bad digest", func(p *PlanRevision) { p.RunBriefCoreDigest = "not-a-digest" }, "SHA-256"},
		{"duplicate grant", func(p *PlanRevision) {
			p.Grants = append(p.Grants, CapabilityGrant{ID: "dup", Name: CapabilityWorktreeRead, Scope: "worktree/*"})
		}, "duplicate capability grant"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := validPlanRevision()
			tc.mutate(&plan)
			if err := plan.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestPlanCriterionCoverageIsCompleteAndRevisionLocal(t *testing.T) {
	revision := ContractRevision{
		ID: "cr-1", OutcomeID: "out-test", Number: 1, Goal: "Ship docs",
		SuccessCriteria: []string{"docs visible", "command verified"}, Review: "owner reviews",
		Criteria: []ContractCriterion{
			{ID: "crit-1", ContractRevisionID: "cr-1", Position: 1, Text: "docs visible"},
			{ID: "crit-2", ContractRevisionID: "cr-1", Position: 2, Text: "command verified"},
		},
	}
	unit := validWorkUnit()
	unit.CriterionIDs = []CriterionID{"crit-1"}
	plan := validPlanRevision()
	plan.WorkUnits = []WorkUnit{unit}
	if err := plan.ValidateAgainstContract(revision); err == nil || !strings.Contains(err.Error(), "crit-2") {
		t.Fatalf("incomplete coverage = %v", err)
	}
	unit.CriterionIDs = []CriterionID{"crit-1", "crit-2"}
	plan.WorkUnits = []WorkUnit{unit}
	if err := plan.ValidateAgainstContract(revision); err != nil {
		t.Fatalf("complete coverage rejected: %v", err)
	}
	unit.CriterionIDs = []CriterionID{"crit-other"}
	plan.WorkUnits = []WorkUnit{unit}
	if err := plan.ValidateAgainstContract(revision); err == nil {
		t.Fatal("criterion from another revision was accepted")
	}
}

func TestPlanApprovalRequiresRoutingBindingAgreement(t *testing.T) {
	revision := ContractRevision{
		ID: "cr-1", OutcomeID: "out-test", Number: 1, Goal: "Inspect repo", SuccessCriteria: []string{"inspection complete"}, Review: "owner",
		Criteria: []ContractCriterion{{ID: "crit-1", ContractRevisionID: "cr-1", Position: 1, Text: "inspection complete"}},
	}
	unit := validWorkUnit()
	unit.Intent = WorkUnitIntentInspect
	unit.CriterionIDs = []CriterionID{"crit-1"}
	unit.RequiredCapabilities = []string{CapabilityWorktreeRead}
	if err := unit.BindExecution(ExecutionBinding{Provider: AgentHarness("codex"), ModelSelection: ExecutionBindingModelProviderDefault}); err != nil {
		t.Fatal(err)
	}
	decision := RoutingDecision{
		Status: RoutingDecisionRecommended, PolicyVersion: RoutingPolicyVersion, Role: RoutingRoleWorker,
		RecommendedCandidateID: "candidate-a", RecommendedProvider: "codex", RecommendedModelSelection: ExecutionBindingModelProviderDefault,
	}
	plan := validPlanRevision()
	plan.WorkUnits = []WorkUnit{unit}
	plan.Grants = []CapabilityGrant{{ID: "cg-read", Name: CapabilityWorktreeRead, Scope: "worktree/*"}}
	plan.RoutingDecisions = []WorkUnitRoutingDecision{{WorkUnitID: unit.ID, Decision: decision}}
	digest, err := ComputePlanRunBriefCoreDigest(revision, plan.WorkUnits, plan.Grants)
	if err != nil {
		t.Fatal(err)
	}
	plan.RunBriefCoreDigest = digest
	if err := plan.ValidateForApproval(revision); err != nil {
		t.Fatalf("matching routing/binding rejected: %v", err)
	}
	plan.RoutingDecisions[0].Decision.RecommendedProvider = "opencode"
	if err := plan.ValidateForApproval(revision); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched routing/binding = %v", err)
	}
}

func TestLeastPrivilegeCapabilitiesArePerWorkUnit(t *testing.T) {
	readOnly := validWorkUnit()
	readOnly.RequiredCapabilities = []string{CapabilityWorktreeRead}
	grants := []CapabilityGrant{{ID: "cg-read", Name: CapabilityWorktreeRead, Scope: "worktree/*"}}
	if missing := MissingCapabilitiesForWorkUnit(grants, readOnly); len(missing) != 0 {
		t.Fatalf("read-only unit unexpectedly needs more authority: %v", missing)
	}
	write := validWorkUnit()
	write.RequiredCapabilities = []string{CapabilityWorktreeRead, CapabilityWorktreeWrite}
	missing := MissingCapabilitiesForWorkUnit(grants, write)
	if len(missing) != 1 || missing[0] != CapabilityWorktreeWrite {
		t.Fatalf("missing capabilities = %v", missing)
	}
}

func TestComputePlanRunBriefCoreDigestBindsGraphAndExecution(t *testing.T) {
	revision := ContractRevision{
		ID: "cr-1", OutcomeID: "out-1", Number: 1, Goal: "Ship docs", SuccessCriteria: []string{"docs visible"}, Review: "owner",
	}
	unit := validWorkUnit()
	unit.ID = "wu-a"
	baseline, err := ComputePlanRunBriefCoreDigest(revision, []WorkUnit{unit}, validGrants())
	if err != nil {
		t.Fatal(err)
	}
	changed := unit
	changed.Provider = AgentHarness("codex")
	changed.ModelSelection = ExecutionBindingModelProviderDefault
	altered, err := ComputePlanRunBriefCoreDigest(revision, []WorkUnit{changed}, validGrants())
	if err != nil {
		t.Fatal(err)
	}
	if baseline == altered {
		t.Fatal("provider/model semantics did not change digest")
	}

	second := validWorkUnit()
	second.ID = "wu-b"
	second.DependsOn = []WorkUnitID{"wu-a"}
	graphDigest, err := ComputePlanRunBriefCoreDigest(revision, []WorkUnit{second, unit}, validGrants())
	if err != nil {
		t.Fatal(err)
	}
	if graphDigest == baseline {
		t.Fatal("dependency graph did not change digest")
	}
}

func TestAuthorityIntersection(t *testing.T) {
	got := AuthorityIntersection(
		[]string{CapabilityWorktreeRead, CapabilityWorktreeWrite},
		[]string{CapabilityWorktreeRead},
	)
	if len(got) != 1 || got[0] != CapabilityWorktreeRead {
		t.Fatalf("AuthorityIntersection() = %v", got)
	}
	if widened := AuthorityIntersection([]string{CapabilityWorktreeRead}, []string{CapabilityWorktreeRead, CapabilityWorktreeWrite}); len(widened) != 1 || widened[0] != CapabilityWorktreeRead {
		t.Fatalf("lower layer widened authority: %v", widened)
	}
}

func TestGrantsFailClosed(t *testing.T) {
	err := GrantsFailClosed([]CapabilityGrant{{ID: "cg-network", Name: "network.fetch", Scope: "*"}}, []string{CapabilityWorktreeRead})
	if err == nil || !strings.Contains(err.Error(), "network.fetch") {
		t.Fatalf("GrantsFailClosed() = %v", err)
	}
}

func TestPlanApprovalRejectsLegacyIntentAndCapabilityDrift(t *testing.T) {
	revision := ContractRevision{ID: "cr-1", OutcomeID: "out-test", Number: 1, Goal: "Inspect", SuccessCriteria: []string{"done"}, Review: "owner", Criteria: []ContractCriterion{{ID: "crit-1", ContractRevisionID: "cr-1", Position: 1, Text: "done"}}}
	unit := validWorkUnit()
	unit.CriterionIDs = []CriterionID{"crit-1"}
	unit.RequiredCapabilities = []string{CapabilityWorktreeRead}
	_ = unit.BindExecution(ExecutionBinding{Provider: AgentHarness("codex"), ModelSelection: ExecutionBindingModelProviderDefault})
	plan := validPlanRevision()
	plan.WorkUnits = []WorkUnit{unit}
	plan.Grants = []CapabilityGrant{{ID: "g", Name: CapabilityWorktreeRead, Scope: "worktree/*"}}
	plan.RoutingDecisions = []WorkUnitRoutingDecision{{WorkUnitID: unit.ID, Decision: RoutingDecision{Status: RoutingDecisionRecommended, PolicyVersion: RoutingPolicyVersion, Role: RoutingRoleWorker, RecommendedCandidateID: "c", RecommendedProvider: "codex", RecommendedModelSelection: ExecutionBindingModelProviderDefault}}}
	plan.RunBriefCoreDigest, _ = ComputePlanRunBriefCoreDigest(revision, plan.WorkUnits, plan.Grants)
	plan.WorkUnits[0].Intent = WorkUnitIntentLegacy
	if err := plan.ValidateForApproval(revision); err == nil || !strings.Contains(err.Error(), "re-planned") {
		t.Fatalf("legacy err=%v", err)
	}
	plan.WorkUnits[0].Intent = WorkUnitIntentInspect
	plan.WorkUnits[0].RequiredCapabilities = []string{CapabilityWorktreeRead, CapabilityWorktreeWrite}
	if err := plan.ValidateForApproval(revision); err == nil || !strings.Contains(err.Error(), "intent") {
		t.Fatalf("drift err=%v", err)
	}
}

func TestPlanRevisionTopologicalOrderUsesFrozenPositionNotOpaqueIdentity(t *testing.T) {
	root := validWorkUnit()
	root.ID, root.Position = "wu-root", 1
	first := validWorkUnit()
	first.ID, first.Position, first.DependsOn = "wu-z-random", 2, []WorkUnitID{root.ID}
	second := validWorkUnit()
	second.ID, second.Position, second.DependsOn = "wu-a-random", 3, []WorkUnitID{root.ID}
	join := validWorkUnit()
	join.ID, join.Position, join.DependsOn = "wu-join", 4, []WorkUnitID{first.ID, second.ID}
	plan := validPlanRevision()
	plan.WorkUnits = []WorkUnit{second, join, root, first}
	ordered, err := plan.TopologicalWorkUnits()
	if err != nil {
		t.Fatal(err)
	}
	want := []WorkUnitID{root.ID, first.ID, second.ID, join.ID}
	for i := range want {
		if ordered[i].ID != want[i] {
			t.Fatalf("ordered[%d] = %s, want %s", i, ordered[i].ID, want[i])
		}
	}
}

func TestPlanRevisionRejectsPartialOrDuplicateFrozenPositions(t *testing.T) {
	first := validWorkUnit()
	first.ID, first.Position = "wu-a", 1
	second := validWorkUnit()
	second.ID, second.Position = "wu-b", 0
	plan := validPlanRevision()
	plan.WorkUnits = []WorkUnit{first, second}
	if err := plan.Validate(); err == nil || !strings.Contains(err.Error(), "fully populated") {
		t.Fatalf("partial positions validation = %v", err)
	}
	second.Position = 1
	plan.WorkUnits[1] = second
	if err := plan.Validate(); err == nil || !strings.Contains(err.Error(), "repeat position") {
		t.Fatalf("duplicate positions validation = %v", err)
	}
}
