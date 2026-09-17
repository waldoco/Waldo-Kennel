package outcome

import (
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

func admissionTestPolicy() *domain.AdmissionPolicy {
	b := domain.ExecutionBudget{WallTimeLimit: time.Hour, RetryLimit: 1, TokenAccounting: domain.TokenAccountingUnsupported, Source: domain.ExecutionBudgetPolicyDefault, PolicyID: "test", PolicyVersion: "v1", PolicyDigest: "digest"}
	policy := &domain.AdmissionPolicy{ID: "test", Version: "v1", Default: b, MaxWallTime: 2 * time.Hour, MaxRetries: 2, MaxTokens: 1000}
	policy.Digest, _ = policy.ComputedDigest()
	policy.Default.PolicyDigest = policy.Digest
	return policy
}

func admissionFixture() admissionInput {
	criterion := domain.ContractCriterion{ID: "criterion", ContractRevisionID: "cr", Position: 1, Text: "inspect"}
	contract := domain.ContractRevision{ID: "cr", OutcomeID: "out", Number: 1, Goal: "inspect", Criteria: []domain.ContractCriterion{criterion}, SuccessCriteria: []string{"inspect"}, Review: "review", AuthorityCeiling: domain.ProposedAuthority{ReadWorkspace: true}}
	policy := admissionTestPolicy()
	unit := domain.WorkUnit{ID: "wu", Intent: domain.WorkUnitIntentInspect, Role: domain.WorkUnitRoleInvestigate, Kind: domain.WorkUnitDirect, Title: "Inspect", ContractRevisionNumber: 1, Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault, OutputSummary: "report", EvidenceChecks: []string{"inspect"}, VerificationRequirement: "review", CriterionIDs: []domain.CriterionID{"criterion"}, RequiredCapabilities: []string{domain.CapabilityWorktreeRead}, ExecutionBudget: policy.Default}
	grant := domain.CapabilityGrant{ID: "g", Name: domain.CapabilityWorktreeRead, Scope: "worktree/*"}
	candidate := domain.RoutingCandidate{ID: "codex", Provider: "codex", ModelSelection: domain.ExecutionBindingModelProviderDefault, WorkerEligible: true, Readiness: domain.CapabilitySupported, Capabilities: map[string]domain.CapabilitySupport{domain.CapabilityWorktreeRead: domain.CapabilitySupported}}
	decision := domain.RouteExecution(domain.RoutingRequirements{Role: domain.RoutingRoleWorker, HardCapabilities: unit.RequiredCapabilities}, []domain.RoutingCandidate{candidate}, "snap")
	plan := domain.PlanRevision{ID: "plan", OutcomeID: "out", Number: 1, ContractRevisionNumber: 1, Status: domain.PlanStatusProposed, Summary: "inspect", WorkUnits: []domain.WorkUnit{unit}, Grants: []domain.CapabilityGrant{grant}, RoutingDecisions: []domain.WorkUnitRoutingDecision{{WorkUnitID: "wu", Decision: decision}}}
	plan.RunBriefCoreDigest, _ = domain.ComputePlanRunBriefCoreDigest(contract, plan.WorkUnits, plan.Grants)
	return admissionInput{outcome: domain.Outcome{ID: "out", CurrentRevisionNumber: 1}, contract: contract, plan: &plan, workspaceKind: domain.WorkspaceGitWorktree, leaseSubject: "project:project", snapshots: map[domain.WorkUnitID]ports.RoutingInventorySnapshot{"wu": {GenerationID: "generation", SnapshotID: "snap", Candidates: []domain.RoutingCandidate{candidate}}}}
}
func TestAdmissionEvaluatorRejectsPreVerificationContractIntake(t *testing.T) {
	in := admissionFixture()
	in.plan = nil
	got := (admissionEvaluator{policy: admissionTestPolicy(), now: func() time.Time { return time.Unix(1, 0) }}).evaluate(in)
	if got.Status != domain.AdmissionRejected || got.Reasons[0] != domain.AdmissionPlanRevisionMissing {
		t.Fatalf("%+v", got)
	}
}
func TestAdmissionEvaluatorRejectsCapabilityInfeasiblePlan(t *testing.T) {
	in := admissionFixture()
	in.snapshots["wu"].Candidates[0].Capabilities[domain.CapabilityWorktreeRead] = domain.CapabilityUnsupported
	got := (admissionEvaluator{policy: admissionTestPolicy(), now: time.Now}).evaluate(in)
	if got.Status != domain.AdmissionRejected || got.Reasons[0] != domain.AdmissionCapabilityMissing {
		t.Fatalf("%+v", got)
	}
}
func TestAdmissionEvaluatorRejectsReadOnlyUnitDemandingWrite(t *testing.T) {
	in := admissionFixture()
	in.plan.WorkUnits[0].RequiredCapabilities = []string{domain.CapabilityWorktreeWrite}
	got := (admissionEvaluator{policy: admissionTestPolicy(), now: time.Now}).evaluate(in)
	if got.Status != domain.AdmissionRejected {
		t.Fatalf("%+v", got)
	}
}
func TestAdmissionEvaluatorApprovalStartParity(t *testing.T) {
	in := admissionFixture()
	e := admissionEvaluator{policy: admissionTestPolicy(), now: func() time.Time { return time.Unix(1, 0) }}
	a, b := e.evaluate(in), e.evaluate(in)
	if a.Status != domain.AdmissionAdmitted || b.Status != a.Status || a.WorkUnits[0].Executable.Digest != b.WorkUnits[0].Executable.Digest {
		t.Fatalf("%+v %+v", a, b)
	}
}

func TestAdmissionEvaluatorReturnsStaleWhenApprovedSpecChanges(t *testing.T) {
	in := admissionFixture()
	e := admissionEvaluator{policy: admissionTestPolicy(), now: time.Now}
	approved := e.evaluate(in)
	spec := *approved.WorkUnits[0].Executable
	in.snapshots["wu"] = ports.RoutingInventorySnapshot{GenerationID: "generation", SnapshotID: "new-snapshot", Candidates: in.snapshots["wu"].Candidates}
	current := e.evaluate(in)
	got := e.revalidate(spec, current)
	if got.Status != domain.AdmissionStale || len(got.Reasons) != 1 || got.Reasons[0] != domain.AdmissionVerdictStale {
		t.Fatalf("%+v", got)
	}
}

func TestAdmissionEvaluatorRejectsMissingResolvedBudget(t *testing.T) {
	in := admissionFixture()
	in.plan.WorkUnits[0].ExecutionBudget = domain.ExecutionBudget{}
	got := (admissionEvaluator{policy: admissionTestPolicy(), now: time.Now}).evaluate(in)
	if got.Status != domain.AdmissionRejected || got.Reasons[0] != domain.AdmissionTimeBudgetMissing {
		t.Fatalf("%+v", got)
	}
}

func TestAdmissionWorkspaceKindMatchesProvisioner(t *testing.T) {
	for _, tc := range []struct {
		kind domain.ProjectKind
		want domain.WorkspaceKind
	}{{domain.ProjectKindSingleRepo, domain.WorkspaceGitWorktree}, {domain.ProjectKindWorkspace, domain.WorkspaceGitWorktree}, {domain.ProjectKindScratch, domain.WorkspaceStagedFolder}} {
		got, err := admissionWorkspaceKind(tc.kind)
		if err != nil || got != tc.want {
			t.Fatalf("%s got %s err=%v", tc.kind, got, err)
		}
	}
	if _, err := admissionWorkspaceKind(domain.ProjectKind("future")); err == nil {
		t.Fatal("unknown project kind defaulted")
	}
}

func TestAdmissionRejectionClearsEarlierExecutableWork(t *testing.T) {
	in := admissionFixture()
	second := in.plan.WorkUnits[0]
	second.ID = "wu-2"
	second.Title = "Second"
	second.ExecutionBudget = domain.ExecutionBudget{}
	in.plan.WorkUnits = append(in.plan.WorkUnits, second)
	in.plan.RoutingDecisions = append(in.plan.RoutingDecisions, domain.WorkUnitRoutingDecision{WorkUnitID: second.ID, Decision: in.plan.RoutingDecisions[0].Decision})
	in.snapshots[second.ID] = in.snapshots["wu"]
	in.plan.RunBriefCoreDigest, _ = domain.ComputePlanRunBriefCoreDigest(in.contract, in.plan.WorkUnits, in.plan.Grants)
	got := (admissionEvaluator{policy: admissionTestPolicy(), now: time.Now}).evaluate(in)
	if got.Status != domain.AdmissionRejected || len(got.WorkUnits) != 0 || got.Reasons[0] != domain.AdmissionTimeBudgetMissing {
		t.Fatalf("%+v", got)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("rejected verdict invalid: %v", err)
	}
}

func TestAdmissionBudgetMissingReasonsAreExact(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*domain.ExecutionBudget)
		want   domain.AdmissionReasonCode
	}{
		{"time", func(b *domain.ExecutionBudget) { b.WallTimeLimit = 0 }, domain.AdmissionTimeBudgetMissing},
		{"retry", func(b *domain.ExecutionBudget) { b.RetryLimit = -1 }, domain.AdmissionRetryBudgetMissing},
		{"token", func(b *domain.ExecutionBudget) { b.TokenAccounting = domain.TokenAccountingEnforced; b.TokenLimit = 0 }, domain.AdmissionTokenBudgetMissing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := admissionFixture()
			tc.mutate(&in.plan.WorkUnits[0].ExecutionBudget)
			got := (admissionEvaluator{policy: admissionTestPolicy(), now: time.Now}).evaluate(in)
			if got.Reasons[0] != tc.want {
				t.Fatalf("%v", got.Reasons)
			}
		})
	}
}

func TestAdmissionRejectsTokenCapWithoutProviderAccounting(t *testing.T) {
	in := admissionFixture()
	in.plan.WorkUnits[0].ExecutionBudget.TokenAccounting = domain.TokenAccountingEnforced
	in.plan.WorkUnits[0].ExecutionBudget.TokenLimit = 500
	got := (admissionEvaluator{policy: admissionTestPolicy(), now: time.Now}).evaluate(in)
	if got.Reasons[0] != domain.AdmissionPlatformUnsupported {
		t.Fatalf("reasons=%v", got.Reasons)
	}
	in.snapshots["wu"].Candidates[0].ExecutionTokenAccounting = domain.CapabilitySupported
	got = (admissionEvaluator{policy: admissionTestPolicy(), now: time.Now}).evaluate(in)
	if got.Status != domain.AdmissionAdmitted {
		t.Fatalf("verdict=%+v", got)
	}
}

func TestAdmissionPolicyMissingAndInvalidAreDistinctOperatorErrors(t *testing.T) {
	in := admissionFixture()
	got := (admissionEvaluator{policy: nil, now: time.Now}).evaluate(in)
	if got.Status != domain.AdmissionRejected || len(got.Reasons) != 1 || got.Reasons[0] != domain.AdmissionPolicyMissing {
		t.Fatalf("nil policy: %+v", got)
	}
	invalid := admissionTestPolicy()
	invalid.Digest = "corrupted"
	got = (admissionEvaluator{policy: invalid, now: time.Now}).evaluate(in)
	if got.Status != domain.AdmissionRejected || len(got.Reasons) != 1 || got.Reasons[0] != domain.AdmissionPolicyInvalid {
		t.Fatalf("invalid policy: %+v", got)
	}
}

func TestAdmissionMalformedBudgetIdentityIsNotTimeBudgetMissing(t *testing.T) {
	in := admissionFixture()
	// Wall-time, retry, and token pieces are all present; the policy identity
	// is blank. ExecutionBudget.Validate rejects this, and it must surface as
	// malformed budget, never as a missing wall-time budget.
	in.plan.WorkUnits[0].ExecutionBudget.PolicyID = ""
	in.plan.WorkUnits[0].ExecutionBudget.PolicyVersion = ""
	in.plan.WorkUnits[0].ExecutionBudget.PolicyDigest = ""
	got := (admissionEvaluator{policy: admissionTestPolicy(), now: time.Now}).evaluate(in)
	if got.Status != domain.AdmissionRejected || len(got.Reasons) != 1 || got.Reasons[0] != domain.AdmissionBudgetInvalid {
		t.Fatalf("%+v", got)
	}
}
