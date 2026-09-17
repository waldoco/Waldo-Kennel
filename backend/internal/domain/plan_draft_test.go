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
	change.DependsOn = []string{"inspect"}
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
