package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	for _, value := range []string{"rm -rf build", "git checkout main", "echo ok | tee x", "cat file", "cp a b", "ruby script.rb", "perl script.pl", "C:\\tmp\\x", "~/secret", "file:/tmp/x", "relative/path.txt", "$(whoami)"} {
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
	p.WorkUnits[0].Provider = HarnessCodex
	p.WorkUnits[0].ModelSelection = ExecutionBindingModelProviderDefault
	p.RoutingDecisions = []WorkUnitRoutingDecision{{WorkUnitID: p.WorkUnits[0].ID, Decision: RoutingDecision{Status: RoutingDecisionRecommended, PolicyVersion: RoutingPolicyVersion, Role: RoutingRoleWorker, RecommendedCandidateID: "candidate", RecommendedProvider: string(HarnessCodex), RecommendedModelSelection: ExecutionBindingModelProviderDefault}}}
	rev := ContractRevision{ID: "cr", OutcomeID: p.OutcomeID, Number: 1, Goal: "g", SuccessCriteria: []string{"c"}, Criteria: []ContractCriterion{{ID: "c", ContractRevisionID: "cr", Position: 1, Text: "c"}}, Review: "r", AuthorityCeiling: ProposedAuthority{ReadWorkspace: true, WriteWorkspace: true, ExecuteLocal: true}}
	p.WorkUnits[0].CriterionIDs = []CriterionID{"c"}
	if err := p.ValidateForApproval(rev); err == nil {
		t.Fatal("legacy role approved")
	}
}

func TestInvestigateRoleMayUseBoundedMutatingIntent(t *testing.T) {
	unit := validPlanDraftWorkUnit("investigate")
	unit.Role = WorkUnitRoleInvestigate
	unit.Intent = WorkUnitIntentModifyAndExecute
	if err := (PlanDraftProposal{Summary: "investigate and reproduce", WorkUnits: []PlanDraftWorkUnit{unit}}).Validate(); err != nil {
		t.Fatalf("investigate bounded intent rejected: %v", err)
	}
}

func TestPlanDraftValidationFamiliesAreTyped(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*PlanDraftProposal)
		want   PlanDraftValidationCode
	}{
		{"role", func(p *PlanDraftProposal) { p.WorkUnits[0].Role = "bogus" }, PlanDraftRoleInvalid},
		{"role intent", func(p *PlanDraftProposal) {
			p.WorkUnits[0].Role = WorkUnitRoleVerify
			p.WorkUnits[0].Intent = WorkUnitIntentModify
		}, PlanDraftRoleIntentConflict},
		{"input blank source", func(p *PlanDraftProposal) { p.WorkUnits[0].Inputs = []PlanDraftDependencyInput{{Required: "findings"}} }, PlanDraftInputMismatch},
		{"verify criterion", func(p *PlanDraftProposal) {
			p.WorkUnits[0].Role = WorkUnitRoleVerify
			p.WorkUnits[0].Intent = WorkUnitIntentExecute
			p.WorkUnits[0].CriteriaCovered = nil
		}, PlanDraftVerifyRequiresCriterion},
		{"consolidate fan in", func(p *PlanDraftProposal) { p.WorkUnits[0].Role = WorkUnitRoleConsolidate }, PlanDraftConsolidateRequiresFanIn},
		{"enabling", func(p *PlanDraftProposal) { p.WorkUnits[0].CriteriaCovered = nil }, PlanDraftEnablingUnconsumed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := PlanDraftProposal{Summary: "typed", WorkUnits: []PlanDraftWorkUnit{validPlanDraftWorkUnit("u")}}
			tc.mutate(&p)
			err := p.Validate()
			var typed *PlanDraftValidationError
			if !errors.As(err, &typed) || typed.Code != tc.want {
				t.Fatalf("err=%v typed=%+v want=%s", err, typed, tc.want)
			}
		})
	}
}

func TestApprovedLegacyExecutionCompatibilityIsExplicit(t *testing.T) {
	p := validPlanRevision()
	p.Status = PlanStatusApproved
	p.WorkUnits[0].Intent = WorkUnitIntentModifyAndExecute
	p.WorkUnits[0].RequiredCapabilities = []string{CapabilityWorktreeRead, CapabilityWorktreeWrite, CapabilityWorktreeExec}
	p.WorkUnits[0].Role = WorkUnitRoleLegacy
	p.WorkUnits[0].Provider = HarnessCodex
	p.WorkUnits[0].ModelSelection = ExecutionBindingModelProviderDefault
	p.RoutingDecisions = []WorkUnitRoutingDecision{{WorkUnitID: p.WorkUnits[0].ID, Decision: RoutingDecision{Status: RoutingDecisionRecommended, PolicyVersion: RoutingPolicyVersion, Role: RoutingRoleWorker, RecommendedCandidateID: "candidate", RecommendedProvider: string(HarnessCodex), RecommendedModelSelection: ExecutionBindingModelProviderDefault}}}
	rev := ContractRevision{ID: "cr", OutcomeID: p.OutcomeID, Number: 1, Goal: "g", SuccessCriteria: []string{"c"}, Criteria: []ContractCriterion{{ID: "c", ContractRevisionID: "cr", Position: 1, Text: "c"}}, Review: "r", AuthorityCeiling: ProposedAuthority{ReadWorkspace: true, WriteWorkspace: true, ExecuteLocal: true}}
	p.WorkUnits[0].CriterionIDs = []CriterionID{"c"}
	if err := p.ValidateForExecution(rev); err != nil {
		t.Fatalf("approved legacy execution rejected: %v", err)
	}
}

func TestLegacyRunBriefDigestOmitsPostMigrationOrchestrationFields(t *testing.T) {
	p := validPlanRevision()
	u := p.WorkUnits[0]
	u.Role = WorkUnitRoleLegacy
	u.Inputs = nil
	rev := ContractRevision{ID: "cr", OutcomeID: p.OutcomeID, Number: 1, Goal: "g", SuccessCriteria: []string{"c"}, Criteria: []ContractCriterion{{ID: "c", ContractRevisionID: "cr", Position: 1, Text: "c"}}, Review: "r"}
	u.CriterionIDs = []CriterionID{"c"}
	got, err := ComputePlanRunBriefCoreDigest(rev, []WorkUnit{u}, p.Grants)
	if err != nil {
		t.Fatal(err)
	}
	// Exact digest produced by the pre-orchestration schema shape.
	legacy := struct {
		ContractRevisionNumber int64                    `json:"contractRevisionNumber"`
		Goal                   string                   `json:"goal"`
		SuccessCriteria        []string                 `json:"successCriteria"`
		Review                 string                   `json:"review"`
		Constraints            []string                 `json:"constraints"`
		NonGoals               []string                 `json:"nonGoals"`
		Clarification          string                   `json:"clarification"`
		WorkUnits              []legacyRunBriefWorkUnit `json:"workUnits"`
		Grants                 []string                 `json:"grants"`
	}{rev.Number, rev.Goal, sortedTrimmed(rev.SuccessCriteria), rev.Review, sortedTrimmed(rev.Constraints), sortedTrimmed(rev.NonGoals), rev.Clarification, []legacyRunBriefWorkUnit{{ID: u.ID.String(), Title: u.Title, Intent: u.Intent, Provider: string(u.Provider), ModelSelection: string(u.ModelSelection), Model: u.Model, Output: u.OutputSummary, EvidenceChecks: sortedTrimmed(u.EvidenceChecks), VerificationRequirement: u.VerificationRequirement, StopConditions: sortedTrimmed(u.StopConditions), CriterionIDs: []string{"c"}, RequiredCapabilities: sortedTrimmed(u.RequiredCapabilities), Checks: runBriefChecks(u.Checks)}}, []string{"worktree.exec@worktree/*", "worktree.read@worktree/*", "worktree.write@worktree/*"}}
	encoded, _ := json.Marshal(legacy)
	sum := sha256.Sum256(encoded)
	want := hex.EncodeToString(sum[:])
	if got != want {
		t.Fatalf("legacy digest=%s want pre-0155 %s", got, want)
	}
}
