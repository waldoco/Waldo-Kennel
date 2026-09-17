package domain

import (
	"reflect"
	"testing"
)

func validPlanDraftWorkUnit(key string) PlanDraftWorkUnit {
	return PlanDraftWorkUnit{
		Key:             key,
		Title:           "Do " + key,
		Intent:          WorkUnitIntentInspect,
		Role:            WorkUnitRoleInvestigate,
		OutputSummary:   "A reviewable result for " + key,
		CriteriaCovered: []string{"C1"},
		EvidenceIdeas:   []string{"inspect the resulting repository state"},
	}
}

func TestWorkUnitIntentMapsToMinimumCapabilities(t *testing.T) {
	cases := []struct {
		intent WorkUnitIntent
		want   []string
	}{
		{WorkUnitIntentInspect, []string{CapabilityWorktreeRead}},
		{WorkUnitIntentModify, []string{CapabilityWorktreeRead, CapabilityWorktreeWrite}},
		{WorkUnitIntentExecute, []string{CapabilityWorktreeRead, CapabilityWorktreeExec}},
		{WorkUnitIntentModifyAndExecute, []string{CapabilityWorktreeRead, CapabilityWorktreeWrite, CapabilityWorktreeExec}},
	}
	for _, tc := range cases {
		got, err := tc.intent.RequiredCapabilities()
		if err != nil {
			t.Fatalf("%s: %v", tc.intent, err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s capabilities = %v, want %v", tc.intent, got, tc.want)
		}
	}
}

func TestPlanDraftProposalRejectsMissingIntent(t *testing.T) {
	unit := validPlanDraftWorkUnit("inspect")
	unit.Intent = ""
	proposal := PlanDraftProposal{Summary: "missing intent", WorkUnits: []PlanDraftWorkUnit{unit}}
	if err := proposal.Validate(); err == nil {
		t.Fatal("work unit without bounded intent should be rejected")
	}
}

func TestPlanDraftProposalDependenciesAreGraphTruthNotListOrder(t *testing.T) {
	change := validPlanDraftWorkUnit("change")
	change.Intent = WorkUnitIntentModify
	change.Role = WorkUnitRoleImplement
	change.DependsOn = []string{"inspect"}
	change.Inputs = []PlanDraftDependencyInput{{FromKey: "inspect", Required: "reviewed findings"}}
	inspect := validPlanDraftWorkUnit("inspect")
	proposal := PlanDraftProposal{
		Summary:   "Inspect first, then make the bounded change.",
		WorkUnits: []PlanDraftWorkUnit{change, inspect}, // deliberately reverse serialization order
	}
	if err := proposal.Validate(); err != nil {
		t.Fatalf("valid graph should not depend on list order: %v", err)
	}
	order, err := proposal.TopologicalOrder()
	if err != nil {
		t.Fatalf("topological order: %v", err)
	}
	if len(order) != 2 || order[0] != "inspect" || order[1] != "change" {
		t.Fatalf("topological order = %v", order)
	}
}

func TestPlanDraftProposalRejectsCycle(t *testing.T) {
	first := validPlanDraftWorkUnit("inspect")
	second := validPlanDraftWorkUnit("change")
	first.DependsOn = []string{"change"}
	second.DependsOn = []string{"inspect"}
	proposal := PlanDraftProposal{Summary: "cycle", WorkUnits: []PlanDraftWorkUnit{first, second}}
	if err := proposal.Validate(); err == nil {
		t.Fatal("cyclic dependencies should be rejected")
	}
}

func TestPlanDraftProposalRejectsUnknownDependency(t *testing.T) {
	unit := validPlanDraftWorkUnit("inspect")
	unit.DependsOn = []string{"missing"}
	proposal := PlanDraftProposal{Summary: "unknown dependency", WorkUnits: []PlanDraftWorkUnit{unit}}
	if err := proposal.Validate(); err == nil {
		t.Fatal("unknown dependency should be rejected")
	}
}

func TestPlanDraftProposalRejectsDuplicateDependency(t *testing.T) {
	first := validPlanDraftWorkUnit("inspect")
	second := validPlanDraftWorkUnit("change")
	second.DependsOn = []string{"inspect", "inspect"}
	proposal := PlanDraftProposal{Summary: "duplicate edge", WorkUnits: []PlanDraftWorkUnit{first, second}}
	if err := proposal.Validate(); err == nil {
		t.Fatal("duplicate dependency should be rejected")
	}
}

func TestPlanDraftProposalRejectsDuplicateKeys(t *testing.T) {
	proposal := PlanDraftProposal{
		Summary:   "duplicate keys",
		WorkUnits: []PlanDraftWorkUnit{validPlanDraftWorkUnit("same"), validPlanDraftWorkUnit("same")},
	}
	if err := proposal.Validate(); err == nil {
		t.Fatal("duplicate work-unit keys should be rejected")
	}
}

func TestPlanDraftProposalRequiresCriterionCoverage(t *testing.T) {
	unit := validPlanDraftWorkUnit("inspect")
	unit.CriteriaCovered = nil
	proposal := PlanDraftProposal{Summary: "missing coverage", WorkUnits: []PlanDraftWorkUnit{unit}}
	if err := proposal.Validate(); err == nil {
		t.Fatal("work unit without criterion aliases should be rejected")
	}
}

func TestPlanDraftTopologicalOrderUsesProposalOrderForIndependentBranches(t *testing.T) {
	root := validPlanDraftWorkUnit("root")
	second := validPlanDraftWorkUnit("second")
	second.DependsOn = []string{"root"}
	first := validPlanDraftWorkUnit("first")
	first.DependsOn = []string{"root"}
	join := validPlanDraftWorkUnit("join")
	join.DependsOn = []string{"first", "second"}
	proposal := PlanDraftProposal{Summary: "Stable fork and join.", WorkUnits: []PlanDraftWorkUnit{root, second, first, join}}
	order, err := proposal.TopologicalOrder()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"root", "second", "first", "join"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want proposal-order tie break %v", order, want)
	}
}

func TestPlanDraftRolesInputsAndEnablingUnits(t *testing.T) {
	root := validPlanDraftWorkUnit("root")
	root.CriteriaCovered = nil
	leaf := validPlanDraftWorkUnit("leaf")
	leaf.Role = WorkUnitRoleVerify
	leaf.Intent = WorkUnitIntentExecute
	leaf.DependsOn = []string{"root"}
	leaf.Inputs = []PlanDraftDependencyInput{{FromKey: "root", Required: "reviewed findings"}}
	if err := (PlanDraftProposal{Summary: "handoff", WorkUnits: []PlanDraftWorkUnit{leaf, root}}).Validate(); err != nil {
		t.Fatalf("valid enabling handoff: %v", err)
	}
	leaf.Inputs = nil
	if err := (PlanDraftProposal{Summary: "missing", WorkUnits: []PlanDraftWorkUnit{leaf, root}}).Validate(); err == nil {
		t.Fatal("missing exact input should fail")
	}
}
func TestPlanDraftRoleDoesNotGrantCapability(t *testing.T) {
	for _, role := range []WorkUnitRole{WorkUnitRoleInvestigate, WorkUnitRoleVerify} {
		u := validPlanDraftWorkUnit(string(role))
		u.Role = role
		u.Intent = WorkUnitIntentExecute
		got, err := u.Intent.RequiredCapabilities()
		if err != nil || !reflect.DeepEqual(got, []string{CapabilityWorktreeRead, CapabilityWorktreeExec}) {
			t.Fatalf("%s widened capability: %v %v", role, got, err)
		}
	}
}
func TestPlanDraftRejectsRoleConflictsAndFakeInputLocators(t *testing.T) {
	u := validPlanDraftWorkUnit("verify")
	u.Role = WorkUnitRoleVerify
	u.Intent = WorkUnitIntentModify
	if err := (PlanDraftProposal{Summary: "bad", WorkUnits: []PlanDraftWorkUnit{u}}).Validate(); err == nil {
		t.Fatal("mutating verify should fail")
	}
	root := validPlanDraftWorkUnit("root")
	child := validPlanDraftWorkUnit("child")
	child.Role = WorkUnitRoleImplement
	child.Intent = WorkUnitIntentModify
	child.DependsOn = []string{"root"}
	child.Inputs = []PlanDraftDependencyInput{{FromKey: "root", Required: "artifact://forged"}}
	if err := (PlanDraftProposal{Summary: "bad locator", WorkUnits: []PlanDraftWorkUnit{root, child}}).Validate(); err == nil {
		t.Fatal("authority-bearing input locator should fail")
	}
}

func TestInputRequirementRejectsObviousPathsCommandsAndMetacharacters(t *testing.T) {
	for _, value := range []string{"rm -rf build", "git checkout main", "echo ok | tee x", "C:\\tmp\\x", "~/secret", "file:/tmp/x", "relative/path.txt", "$(whoami)"} {
		err := ValidateWorkUnitInputRequirement(value)
		if value == "relative/path.txt" {
			if err == nil {
				t.Errorf("%q accepted", value)
			}
			continue
		}
		if err == nil {
			t.Errorf("%q accepted", value)
		}
	}
}
func TestCanonicalInputRejectsMalformedRequirementAndDigestBindsInput(t *testing.T) {
	unit := validWorkUnit()
	unit.ID = "child"
	unit.Role = WorkUnitRoleImplement
	unit.DependsOn = []WorkUnitID{"parent"}
	unit.Inputs = []WorkUnitInput{{FromWorkUnitID: "parent", Required: "rm -rf build", Position: 1}}
	if err := unit.Validate(); err == nil {
		t.Fatal("canonical command-like input accepted")
	}
	unit.Inputs[0].Required = "reviewed findings"
	parent := validWorkUnit()
	parent.ID = "parent"
	parent.Role = WorkUnitRoleInvestigate
	parent.Intent = WorkUnitIntentInspect
	parent.RequiredCapabilities = []string{CapabilityWorktreeRead}
	plan := validPlanRevision()
	plan.WorkUnits = []WorkUnit{parent, unit}
	rev := ContractRevision{ID: "cr", OutcomeID: plan.OutcomeID, Number: 1, Goal: "g", SuccessCriteria: []string{"c"}, Criteria: []ContractCriterion{{ID: "c", ContractRevisionID: "cr", Position: 1, Text: "c"}}, Review: "r"}
	unit.CriterionIDs = []CriterionID{"c"}
	plan.WorkUnits = []WorkUnit{parent, unit}
	d1, err := ComputePlanRunBriefCoreDigest(rev, plan.WorkUnits, plan.Grants)
	if err != nil {
		t.Fatal(err)
	}
	plan.WorkUnits[1].Inputs[0].Required = "different findings"
	d2, err := ComputePlanRunBriefCoreDigest(rev, plan.WorkUnits, plan.Grants)
	if err != nil {
		t.Fatal(err)
	}
	if d1 == d2 {
		t.Fatal("semantic input mutation retained digest")
	}
}
func TestLegacyRoleCannotBeNewlyApproved(t *testing.T) {
	p := validPlanRevision()
	p.WorkUnits[0].Intent = WorkUnitIntentModifyAndExecute
	p.WorkUnits[0].RequiredCapabilities = []string{CapabilityWorktreeRead, CapabilityWorktreeWrite, CapabilityWorktreeExec}
	p.WorkUnits[0].Role = WorkUnitRoleLegacy
	rev := ContractRevision{ID: "cr", OutcomeID: p.OutcomeID, Number: 1, Goal: "g", SuccessCriteria: []string{"c"}, Criteria: []ContractCriterion{{ID: "c", ContractRevisionID: "cr", Position: 1, Text: "c"}}, Review: "r", AuthorityCeiling: ProposedAuthority{ReadWorkspace: true, WriteWorkspace: true, ExecuteLocal: true}}
	p.WorkUnits[0].CriterionIDs = []CriterionID{"c"}
	if err := p.ValidateForApproval(rev); err == nil {
		t.Fatal("legacy role approved")
	}
}
