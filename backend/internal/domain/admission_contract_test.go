package domain

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func intp(v int) *int                        { return &v }
func i64p(v int64) *int64                    { return &v }
func planp(v PlanRevisionID) *PlanRevisionID { return &v }
func spec(t *testing.T) ApprovedExecutableSpec {
	t.Helper()
	s := ApprovedExecutableSpec{CompilerPolicyVersion: "compiler/v1", OutcomeID: "out", ContractRevisionNumber: 1, PlanRevisionID: "plan", WorkUnitID: "wu", RunBriefCoreDigest: strings.Repeat("a", 64), Binding: ExecutionBinding{Provider: HarnessCodex, ModelSelection: ExecutionBindingModelProviderDefault}, NativeMappingVersion: "codex/v1", RequiredCapabilities: []string{CapabilityWorktreeRead}, Grants: []CapabilityGrant{{ID: "g", Name: CapabilityWorktreeRead, Scope: "worktree/*"}}, Workspace: WorkspaceRequirements{Kind: WorkspaceGitWorktree, LeaseSubject: "project:p"}, Budget: AdmissionBudget{PolicyVersion: "b1", AccountingVersion: "a1", WallTimeLimit: time.Hour, RetryLimit: intp(1), RetryLineageScope: AdmissionRetryLineageWorkUnit}, RoutingReceipt: func() RoutingAdmissionReceipt {
		r := RoutingAdmissionReceipt{GenerationID: "g", SnapshotID: "s", Preference: RoutingPreference{Provider: "codex"}, Candidates: []RoutingCandidate{{ID: "codex", Provider: "codex"}}}
		r.Digest, _ = r.ComputedDigest()
		return r
	}(), AdmissionReceipts: []ReadinessReceipt{{Producer: "codex_probe", Version: "v1", ReceiptID: "r1", Digest: "abc"}}}
	d, e := s.ComputedDigest()
	if e != nil {
		t.Fatal(e)
	}
	s.Digest = d
	return s
}
func launch(t *testing.T) WorkspaceBoundLaunchPacket {
	t.Helper()
	s := spec(t)
	p := AttemptExecutionPolicy{OutcomeID: s.OutcomeID, PlanRevisionID: s.PlanRevisionID, WorkUnitID: s.WorkUnitID, ContractRevisionNumber: s.ContractRevisionNumber, RunBriefCoreDigest: s.RunBriefCoreDigest, WorkspaceRoot: "/tmp/wu", RequiredCapabilities: append([]string(nil), s.RequiredCapabilities...), Grants: append([]CapabilityGrant(nil), s.Grants...), ApprovedChecks: append([]ApprovedCheck(nil), s.ApprovedChecks...)}
	x := WorkspaceBoundLaunchPacket{Spec: s, SpecDigest: s.Digest, AttemptID: "attempt", FenceID: "fence", SessionID: "session", CanonicalWorkspaceRoot: "/tmp/wu", InputArtifactVersions: []string{"artifact:v1"}, CurrentReadinessReceipts: []ReadinessReceipt{{Producer: "codex_probe", Version: "v1", ReceiptID: "live", Digest: "def"}}, Policy: p}
	policyDigest, _ := p.Digest()
	x.LaunchFacts = LaunchFacts{AttemptID: x.AttemptID, FenceID: x.FenceID, SessionID: x.SessionID, CanonicalWorkspaceRoot: x.CanonicalWorkspaceRoot, SpecDigest: x.SpecDigest, ReadinessReceipts: x.CurrentReadinessReceipts, InputArtifactVersions: x.InputArtifactVersions, PolicyDigest: policyDigest}
	x.LaunchFactsDigest, _ = x.LaunchFacts.Digest()
	d, e := x.ComputedDigest()
	if e != nil {
		t.Fatal(e)
	}
	x.Digest = d
	return x
}
func TestAdmissionAllowedShapes(t *testing.T) {
	now := time.Now()
	for _, v := range []AdmissionVerdict{{ID: "v", OutcomeID: "out", EvaluatedAt: now, Status: AdmissionRejected, PolicyVersion: "v1", Reasons: []AdmissionReasonCode{AdmissionContractRevisionMissing}}, {ID: "v", OutcomeID: "out", ContractRevisionNumber: i64p(1), EvaluatedAt: now, Status: AdmissionRejected, PolicyVersion: "v1", Reasons: []AdmissionReasonCode{AdmissionPlanRevisionMissing}}, {ID: "v", OutcomeID: "out", EvaluatedAt: now, Status: AdmissionStale, PolicyVersion: "v1", Reasons: []AdmissionReasonCode{AdmissionVerdictStale}}} {
		if e := v.Validate(); e != nil {
			t.Fatal(e)
		}
	}
	s := spec(t)
	v := AdmissionVerdict{ID: "v", OutcomeID: "out", ContractRevisionNumber: i64p(1), PlanRevisionID: planp("plan"), EvaluatedAt: now, Status: AdmissionAdmitted, PolicyVersion: "v1", WorkUnits: []WorkUnitAdmissionVerdict{{WorkUnitID: "wu", Status: AdmissionAdmitted, Executable: &s}}}
	if e := v.Validate(); e != nil {
		t.Fatal(e)
	}
}
func TestApprovedSpecHasNoRuntimeIdentityOrRoot(t *testing.T) {
	typ := reflect.TypeOf(ApprovedExecutableSpec{})
	for _, name := range []string{"AttemptID", "FenceID", "SessionID", "WorkspaceRoot", "CanonicalWorkspaceRoot"} {
		if _, ok := typ.FieldByName(name); ok {
			t.Fatalf("spec contains %s", name)
		}
	}
	if e := spec(t).Validate(); e != nil {
		t.Fatal(e)
	}
}
func TestLaunchRequiresCompleteRuntimeBindingAndCanonicalRoot(t *testing.T) {
	x := launch(t)
	if e := x.Validate(); e != nil {
		t.Fatal(e)
	}
	x.SessionID = ""
	if e := x.Validate(); e == nil {
		t.Fatal("missing session passed")
	}
	x = launch(t)
	x.CanonicalWorkspaceRoot = "relative"
	if e := x.Validate(); e == nil {
		t.Fatal("noncanonical root passed")
	}
}
func TestSpecAndLaunchDigestTamperAndMismatchFail(t *testing.T) {
	s := spec(t)
	s.NativeMappingVersion = "changed"
	if s.Validate() == nil {
		t.Fatal("spec tamper passed")
	}
	x := launch(t)
	x.SpecDigest = "other"
	if x.Validate() == nil {
		t.Fatal("spec mismatch passed")
	}
	x = launch(t)
	x.AttemptID = "other"
	if x.Validate() == nil {
		t.Fatal("launch tamper passed")
	}
}
func TestLaunchCannotWidenApprovedAuthority(t *testing.T) {
	mutations := []func(*WorkspaceBoundLaunchPacket){func(x *WorkspaceBoundLaunchPacket) {
		x.Policy.RequiredCapabilities = append(x.Policy.RequiredCapabilities, CapabilityWorktreeWrite)
	}, func(x *WorkspaceBoundLaunchPacket) {
		x.Policy.Grants = append(x.Policy.Grants, CapabilityGrant{ID: "w", Name: CapabilityWorktreeWrite, Scope: "worktree/*"})
	}, func(x *WorkspaceBoundLaunchPacket) {
		x.Policy.ApprovedChecks = append(x.Policy.ApprovedChecks, ApprovedCheck{ID: "c", CriterionID: "criterion", Argv: []string{"go", "test"}, TimeoutSeconds: 10})
	}, func(x *WorkspaceBoundLaunchPacket) { x.Spec.Budget.RetryLimit = intp(99) }}
	for i, m := range mutations {
		x := launch(t)
		m(&x)
		if x.Validate() == nil {
			t.Fatalf("mutation %d passed", i)
		}
	}
}
func TestCrossAttributionAndLocalHarnessOnly(t *testing.T) {
	x := launch(t)
	x.Policy.OutcomeID = "other"
	if x.Validate() == nil {
		t.Fatal("cross attribution passed")
	}
	s := spec(t)
	s.Binding.Provider = "openai-owner-key"
	d, _ := s.ComputedDigest()
	s.Digest = d
	if s.Validate() == nil {
		t.Fatal("nonlocal harness passed")
	}
	for _, h := range AllHarnesses {
		if !h.IsSelectableForNewWork() {
			t.Fatal(h)
		}
	}
}
func TestNonAdmittedForbidsExecutableAndAdmittedNeedsNoOwnerAction(t *testing.T) {
	s := spec(t)
	v := AdmissionVerdict{ID: "v", OutcomeID: "out", EvaluatedAt: time.Now(), Status: AdmissionRejected, PolicyVersion: "v1", Reasons: []AdmissionReasonCode{AdmissionProviderUnavailable}, WorkUnits: []WorkUnitAdmissionVerdict{{WorkUnitID: "wu", Status: AdmissionAdmitted, Executable: &s}}}
	if v.Validate() == nil {
		t.Fatal("contradiction passed")
	}
	v = AdmissionVerdict{ID: "v", OutcomeID: "out", ContractRevisionNumber: i64p(1), PlanRevisionID: planp("plan"), EvaluatedAt: time.Now(), Status: AdmissionAdmitted, PolicyVersion: "v1", OwnerActions: []AdmissionOwnerAction{AdmissionActionReevaluate}, WorkUnits: []WorkUnitAdmissionVerdict{{WorkUnitID: "wu", Status: AdmissionAdmitted, Executable: &s}}}
	if v.Validate() == nil {
		t.Fatal("owner action passed")
	}
}
func TestCompilerRejectsDuplicateNormalizedCapabilitiesAndGrantsAndChecksDoNotWiden(t *testing.T) {
	u := WorkUnit{ID: "wu", RequiredCapabilities: []string{" worktree.read ", "worktree.read"}}
	p := PlanRevision{ID: "plan", OutcomeID: "out", ContractRevisionNumber: 1, Grants: []CapabilityGrant{{ID: "g", Name: CapabilityWorktreeRead, Scope: "worktree/*"}}}
	if _, e := BuildAttemptExecutionPolicy("out", p, u, strings.Repeat("a", 64)); e == nil {
		t.Fatal("duplicate capability passed")
	}
	u.RequiredCapabilities = []string{CapabilityWorktreeRead}
	p.Grants = append(p.Grants, CapabilityGrant{ID: "g2", Name: CapabilityWorktreeRead, Scope: "other"})
	if _, e := BuildAttemptExecutionPolicy("out", p, u, strings.Repeat("a", 64)); e == nil {
		t.Fatal("duplicate grant passed")
	}
	p.Grants = p.Grants[:1]
	u.Checks = []ApprovedCheck{{ID: "c", CriterionID: "criterion", Argv: []string{"go", "test"}, TimeoutSeconds: 10}}
	policy, e := BuildAttemptExecutionPolicy("out", p, u, strings.Repeat("a", 64))
	if e != nil {
		t.Fatal(e)
	}
	if policy.Has(CapabilityWorktreeExec) {
		t.Fatal("checks widened provider")
	}
}
func TestBudgetAndMetadataDefenses(t *testing.T) {
	b := spec(t).Budget
	b.TokenLimit = -1
	if b.Validate() == nil {
		t.Fatal("negative token passed")
	}
	if len(SortedAdmissionReasonCodes()) != 28 {
		t.Fatal("reason count")
	}
}

func TestLaunchFactsTamperFailsEvenWhenOuterDigestIsRehashed(t *testing.T) {
	x := launch(t)
	x.LaunchFacts.CanonicalWorkspaceRoot = "/tampered"
	x.Digest, _ = x.ComputedDigest()
	if err := x.Validate(); err == nil {
		t.Fatal("tampered typed facts accepted")
	}
}
func TestLaunchFactsDigestDependsOnCanonicalInputOrdering(t *testing.T) {
	x := launch(t)
	first, _ := x.LaunchFacts.Digest()
	x.LaunchFacts.InputArtifactVersions = []string{"b", "a"}
	second, _ := x.LaunchFacts.Digest()
	if first == second {
		t.Fatal("ordered inputs did not affect digest")
	}
}
func TestRoutingReceiptDigestCanonicalAndBindsCandidateModelFacts(t *testing.T) {
	a := RoutingAdmissionReceipt{GenerationID: "g", SnapshotID: "s", Preference: RoutingPreference{Provider: "codex", Model: "m"}, Candidates: []RoutingCandidate{{ID: "b", Provider: "b", Models: map[string]CapabilitySupport{"m": CapabilitySupported}}, {ID: "a", Provider: "a"}}}
	a.Digest, _ = a.ComputedDigest()
	b := a
	b.Candidates = []RoutingCandidate{a.Candidates[1], a.Candidates[0]}
	d, _ := b.ComputedDigest()
	if d != a.Digest {
		t.Fatal("ordering changed digest")
	}
	b = a
	for i := range b.Candidates {
		if b.Candidates[i].Models != nil {
			b.Candidates[i].Models["m"] = CapabilityUnsupported
		}
	}
	if err := b.Validate(); err == nil {
		t.Fatal("model mutation accepted")
	}
}
func TestRoutingReceiptValidationDoesNotReorderCaller(t *testing.T) {
	r := RoutingAdmissionReceipt{GenerationID: "g", SnapshotID: "s", Candidates: []RoutingCandidate{{ID: "z"}, {ID: "a"}}}
	r.Digest, _ = r.ComputedDigest()
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if r.Candidates[0].ID != "z" {
		t.Fatal("validation mutated candidate order")
	}
}
