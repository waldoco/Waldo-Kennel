package outcome_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

type planningFakeStore struct {
	*attemptFakeStore
	project      domain.ProjectRecord
	projectReads int
	runs         map[domain.IntelligenceRunID]domain.IntelligenceRun
}

func testAdmissionPolicy() *domain.AdmissionPolicy {
	b := domain.ExecutionBudget{WallTimeLimit: time.Hour, RetryLimit: 1, TokenAccounting: domain.TokenAccountingUnsupported, Source: domain.ExecutionBudgetPolicyDefault, PolicyID: "test-policy", PolicyVersion: "v1", PolicyDigest: strings.Repeat("a", 64)}
	policy := &domain.AdmissionPolicy{ID: b.PolicyID, Version: b.PolicyVersion, Default: b, MaxWallTime: 2 * time.Hour, MaxRetries: 2, MaxTokens: 100000}
	policy.Digest, _ = policy.ComputedDigest()
	policy.Default.PolicyDigest = policy.Digest
	return policy
}

func newPlanningFakeStore() *planningFakeStore {
	return &planningFakeStore{
		attemptFakeStore: newAttemptFakeStore(),
		project: domain.ProjectRecord{
			ID: "mer",
			Config: domain.ProjectConfig{Worker: domain.RoleOverride{
				Harness:     domain.HarnessClaudeCode,
				AgentConfig: domain.AgentConfig{Model: "sonnet-test"},
			}},
		},
		runs: map[domain.IntelligenceRunID]domain.IntelligenceRun{},
	}
}

func (f *planningFakeStore) GetProject(_ context.Context, id string) (domain.ProjectRecord, bool, error) {
	f.projectReads++
	if id != f.project.ID {
		return domain.ProjectRecord{}, false, nil
	}
	return f.project, true, nil
}

func (f *planningFakeStore) CreateIntelligenceRun(_ context.Context, run domain.IntelligenceRun) error {
	if err := run.Validate(); err != nil {
		return err
	}
	if _, exists := f.runs[run.ID]; exists {
		return fmt.Errorf("duplicate intelligence run %s", run.ID)
	}
	f.runs[run.ID] = run
	return nil
}
func (f *planningFakeStore) GetIntelligenceRun(_ context.Context, id domain.IntelligenceRunID) (domain.IntelligenceRun, bool, error) {
	run, ok := f.runs[id]
	return run, ok, nil
}
func (f *planningFakeStore) ListNonTerminalIntelligenceRuns(_ context.Context) ([]domain.IntelligenceRun, error) {
	var out []domain.IntelligenceRun
	for _, run := range f.runs {
		if !run.Status.Terminal() {
			out = append(out, run)
		}
	}
	return out, nil
}
func (f *planningFakeStore) RecordIntelligenceRunEffectiveProvenance(_ context.Context, id domain.IntelligenceRunID, provider domain.IntelligenceProviderID, model, native string) error {
	run, ok := f.runs[id]
	if !ok {
		return fmt.Errorf("missing intelligence run %s", id)
	}
	run.EffectiveProvider, run.EffectiveModel, run.NativeSessionRef = provider, model, native
	f.runs[id] = run
	return nil
}
func (f *planningFakeStore) RecordIntelligenceRunMetrics(_ context.Context, id domain.IntelligenceRunID, input, output, duration *int64) error {
	run, ok := f.runs[id]
	if !ok {
		return fmt.Errorf("missing intelligence run %s", id)
	}
	if run.InputTokens == nil {
		run.InputTokens = input
	}
	if run.OutputTokens == nil {
		run.OutputTokens = output
	}
	if run.DurationMS == nil {
		run.DurationMS = duration
	}
	f.runs[id] = run
	return nil
}
func (f *planningFakeStore) UpdateIntelligenceRunStatus(_ context.Context, id domain.IntelligenceRunID, status domain.IntelligenceRunStatus, output domain.SHA256Digest, code, detail string, completed *time.Time) error {
	run, ok := f.runs[id]
	if !ok {
		return fmt.Errorf("missing intelligence run %s", id)
	}
	run.Status, run.OutputDigest, run.FailureCode, run.FailureDetail, run.CompletedAt = status, output, code, detail, completed
	f.runs[id] = run
	return nil
}

type twoUnitPlanIntelligence struct{ calls int }

func (*twoUnitPlanIntelligence) ID() domain.IntelligenceProviderID { return "test-plan-intelligence" }
func (*twoUnitPlanIntelligence) AnalyzeContract(context.Context, ports.ContractIntelligenceRequest) (ports.ContractIntelligenceResponse, error) {
	return ports.ContractIntelligenceResponse{}, fmt.Errorf("contract intelligence not used")
}
func (p *twoUnitPlanIntelligence) DraftPlan(context.Context, ports.PlanIntelligenceRequest) (ports.PlanIntelligenceResponse, error) {
	p.calls++
	return ports.PlanIntelligenceResponse{
		Readiness: domain.NewPlanningReadinessResult("Ready.", &domain.PlanDraftProposal{
			Summary: "Edit, then verify the confirmed Contract.",
			WorkUnits: []domain.PlanDraftWorkUnit{
				{Key: "verify", Title: "Verify outcome", Intent: domain.WorkUnitIntentExecute, Role: domain.WorkUnitRoleVerify, Inputs: []domain.PlanDraftDependencyInput{{FromKey: "edit", Required: "implemented result"}}, OutputSummary: "Verified result", CriteriaCovered: []string{"C2"}, DependsOn: []string{"edit"}, EvidenceIdeas: []string{"verification output"}},
				{Key: "edit", Title: "Implement outcome", Intent: domain.WorkUnitIntentModify, Role: domain.WorkUnitRoleImplement, OutputSummary: "Implemented result", CriteriaCovered: []string{"C1"}, EvidenceIdeas: []string{"workspace diff"}},
			},
		}, nil),
		Provenance: ports.IntelligenceProvenance{EffectiveProvider: "test-plan-intelligence", EffectiveModel: "planner-test"},
	}, nil
}

type routingInventoryFake struct {
	calls         int
	preference    *domain.RoutingPreference
	candidates    []domain.RoutingCandidate
	generationIDs []string
	snapshotIDs   []string
	err           error
}

func (r *routingInventoryFake) RoutingSnapshot(_ context.Context, _ domain.ProjectID, preference *domain.RoutingPreference) (ports.RoutingInventorySnapshot, error) {
	r.calls++
	if r.err != nil {
		return ports.RoutingInventorySnapshot{}, r.err
	}
	if preference != nil {
		preferenceCopy := *preference
		r.preference = &preferenceCopy
	}
	generationID, snapshotID := "generation-test", "snapshot-test"
	if len(r.generationIDs) >= r.calls {
		generationID = r.generationIDs[r.calls-1]
	}
	if len(r.snapshotIDs) >= r.calls {
		snapshotID = r.snapshotIDs[r.calls-1]
	}
	return ports.RoutingInventorySnapshot{GenerationID: generationID, SnapshotID: snapshotID, Candidates: r.candidates}, nil
}

func readyClaudeCandidate() domain.RoutingCandidate {
	return domain.RoutingCandidate{
		ID: "claude-code", Provider: "claude-code", ModelSelection: domain.ExecutionBindingModelProviderDefault,
		WorkerEligible: true, CoordinatorEligible: true, Readiness: domain.CapabilitySupported,
		Capabilities: map[string]domain.CapabilitySupport{
			domain.CapabilityWorktreeRead:  domain.CapabilitySupported,
			domain.CapabilityWorktreeWrite: domain.CapabilitySupported,
			domain.CapabilityWorktreeExec:  domain.CapabilitySupported,
		},
		Models: map[string]domain.CapabilitySupport{"sonnet-test": domain.CapabilitySupported},
	}
}

func fullLocalAuthority() domain.ProposedAuthority {
	return domain.ProposedAuthority{ReadWorkspace: true, WriteWorkspace: true, ExecuteLocal: true}
}

func newPlanningTestService(t *testing.T, router *routingInventoryFake) (*outcome.Service, *planningFakeStore, domain.OutcomeID, *twoUnitPlanIntelligence) {
	t.Helper()
	store := newPlanningFakeStore()
	provider := &twoUnitPlanIntelligence{}
	svc := outcome.New(store, nil).WithPlanning(provider, router)
	svc.AdmissionPolicy = testAdmissionPolicy()

	store.planFakeStore.mu.Lock()
	store.spaces["mer"] = domain.ResponsibilitySpace{ID: "rsp-plan-compiler", Kind: domain.ResponsibilitySpaceWorkProject, ProjectID: "mer"}
	store.planFakeStore.mu.Unlock()

	ctx := context.Background()
	view, err := svc.Create(ctx, outcome.CreateInput{
		ProjectID: "mer", Title: "Compiler outcome", Goal: "Ship and verify a bounded change.",
		SuccessCriteria: []string{"Implementation is present."}, Review: "Run deterministic verification.",
		AuthorityCeiling: fullLocalAuthority(), StopConditions: []string{"Stop before remote effects."},
		RequestKey: "req-plan-compiler",
	})
	if err != nil {
		t.Fatalf("seed outcome: %v", err)
	}
	view, err = svc.ReviseContract(ctx, view.Outcome.ID, outcome.ReviseContractInput{
		ExpectedRevision: 1, Goal: "Ship and verify a bounded change.",
		SuccessCriteria: []string{"Implementation is present.", "Verification proves the implementation behaves as required."},
		Review:          "Run deterministic verification.", AuthorityCeiling: fullLocalAuthority(), StopConditions: []string{"Stop before remote effects."},
	})
	if err != nil {
		t.Fatalf("seed second criterion: %v", err)
	}
	return svc, store, view.Outcome.ID, provider
}

func TestProposePlanCompilesIntelligenceGraphAndRoutesEveryWorkUnit(t *testing.T) {
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}
	svc, store, outcomeID, provider := newPlanningTestService(t, router)
	view, err := svc.ProposePlan(context.Background(), outcomeID, 2)
	if err != nil {
		t.Fatalf("propose plan: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("plan intelligence calls = %d, want 1", provider.calls)
	}
	if len(view.Plan.WorkUnits) != 2 || len(view.Plan.RoutingDecisions) != 2 {
		t.Fatalf("compiled plan = %+v", view.Plan)
	}
	if view.Plan.WorkUnits[0].Title != "Implement outcome" || view.Plan.WorkUnits[0].Position != 1 || view.Plan.WorkUnits[1].Title != "Verify outcome" || view.Plan.WorkUnits[1].Position != 2 {
		t.Fatalf("compiled frozen order = %+v", view.Plan.WorkUnits)
	}
	ordered, err := view.Plan.TopologicalWorkUnits()
	if err != nil {
		t.Fatalf("topological order: %v", err)
	}
	if ordered[0].Title != "Implement outcome" || ordered[1].Title != "Verify outcome" {
		t.Fatalf("topological titles = %q -> %q", ordered[0].Title, ordered[1].Title)
	}
	if !reflect.DeepEqual(ordered[0].RequiredCapabilities, []string{domain.CapabilityWorktreeRead, domain.CapabilityWorktreeWrite}) {
		t.Fatalf("edit capabilities = %v, want read+write only", ordered[0].RequiredCapabilities)
	}
	if !reflect.DeepEqual(ordered[1].RequiredCapabilities, []string{domain.CapabilityWorktreeRead, domain.CapabilityWorktreeExec}) {
		t.Fatalf("verify capabilities = %v, want read+exec only", ordered[1].RequiredCapabilities)
	}
	for _, unit := range view.Plan.WorkUnits {
		if unit.Provider != domain.HarnessClaudeCode || unit.ModelSelection != domain.ExecutionBindingModelExplicit || unit.Model != "sonnet-test" {
			t.Fatalf("work unit %s binding = %s/%s/%s", unit.ID, unit.Provider, unit.ModelSelection, unit.Model)
		}
		if len(unit.CriterionIDs) != 1 || unit.CriterionIDs[0].IsZero() {
			t.Fatalf("work unit %s criteria = %+v", unit.ID, unit.CriterionIDs)
		}
	}
	if err := domain.ValidateExactPlanCapabilityGrants(view.Plan.Grants, view.Plan.WorkUnits); err != nil {
		t.Fatalf("compiled grants: %v", err)
	}
	if router.preference == nil || router.preference.Provider != string(domain.HarnessClaudeCode) || router.preference.Model != "sonnet-test" {
		t.Fatalf("preference = %+v", router.preference)
	}
	if router.calls != 1 {
		t.Fatalf("routing calls = %d, want one shared snapshot", router.calls)
	}
	var runs []domain.IntelligenceRun
	for _, run := range store.runs {
		if run.Kind == domain.IntelligenceRunPlanDraft {
			runs = append(runs, run)
		}
	}
	if len(runs) != 1 || runs[0].Status != domain.IntelligenceRunFulfilled || runs[0].OutcomeID != outcomeID || runs[0].ContractRevisionID.IsZero() {
		t.Fatalf("plan runs = %+v", runs)
	}
}

func TestPlanCompilerRejectsWorkUnitOutsideContractCeiling(t *testing.T) {
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}
	svc, store, outcomeID, _ := newPlanningTestService(t, router)
	store.planFakeStore.mu.Lock()
	revs := store.revs[outcomeID]
	revs[len(revs)-1].AuthorityCeiling = domain.ProposedAuthority{ReadWorkspace: true, WriteWorkspace: true}
	store.revs[outcomeID] = revs
	store.planFakeStore.mu.Unlock()

	// S3: authority shortfalls are typed readiness issues, not API errors.
	view, err := svc.ProposePlan(context.Background(), outcomeID, 2)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if view.Readiness == nil || view.Readiness.Status != domain.PlanningBlocked {
		t.Fatalf("execute intent outside Contract ceiling must produce a blocked packet: %+v", view.Readiness)
	}
	authorityIssue := false
	for _, issue := range view.Readiness.Issues {
		if issue.Kind == domain.ReadinessAuthorityInsufficient && issue.Route == domain.RouteReviseContract {
			authorityIssue = true
		}
	}
	if !authorityIssue {
		t.Fatalf("blocked packet carries no authority_insufficient/revise_contract issue: %+v", view.Readiness.Issues)
	}
	if got := len(store.plans[outcomeID]); got != 0 {
		t.Fatalf("blocked packet persisted %d plans", got)
	}
}

func TestPlanCompilerDoesNotWidenMissingNewWorkAuthority(t *testing.T) {
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}
	svc, store, outcomeID, _ := newPlanningTestService(t, router)
	store.planFakeStore.mu.Lock()
	revs := store.revs[outcomeID]
	revs[len(revs)-1].AuthorityCeiling = domain.ProposedAuthority{}
	store.revs[outcomeID] = revs
	store.planFakeStore.mu.Unlock()

	// S3: an empty ceiling cannot error its way into a Plan either. The
	// evaluator reads its single normalized snapshot and returns a blocked
	// authority packet; no grants are minted and nothing is persisted.
	view, err := svc.ProposePlan(context.Background(), outcomeID, 2)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if view.Readiness == nil || view.Readiness.Status != domain.PlanningBlocked {
		t.Fatalf("missing new-work authority must produce a blocked packet: %+v", view.Readiness)
	}
	authorityIssue := false
	for _, issue := range view.Readiness.Issues {
		if issue.Kind == domain.ReadinessAuthorityInsufficient && issue.Route == domain.RouteReviseContract {
			authorityIssue = true
		}
	}
	if !authorityIssue {
		t.Fatalf("blocked packet carries no authority_insufficient/revise_contract issue: %+v", view.Readiness.Issues)
	}
	if router.calls != 1 {
		t.Fatalf("evaluation must read exactly one routing snapshot, got %d", router.calls)
	}
	if got := len(store.plans[outcomeID]); got != 0 {
		t.Fatalf("blocked packet persisted %d plans", got)
	}
}

func TestApprovePlanDoesNotRereadMutableProjectPreference(t *testing.T) {
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}
	svc, store, outcomeID, _ := newPlanningTestService(t, router)
	proposal, err := svc.ProposePlan(context.Background(), outcomeID, 2)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	reads := store.projectReads
	store.project.Config.Worker = domain.RoleOverride{Harness: domain.HarnessCodex, AgentConfig: domain.AgentConfig{Model: "changed-after-proposal"}}
	approved, err := svc.ApprovePlan(context.Background(), outcomeID, outcome.ApprovePlanInput{PlanRevisionID: proposal.Plan.ID, ExpectedContractRevision: 2})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if store.projectReads != reads+1 {
		t.Fatalf("approval project-kind reads = %d -> %d, want one custody-kind read", reads, store.projectReads)
	}
	for _, unit := range approved.Plan.WorkUnits {
		if unit.Provider != domain.HarnessClaudeCode || unit.Model != "sonnet-test" {
			t.Fatalf("approved binding changed: %+v", unit)
		}
	}
}

func TestProposePlanNoValidRoutePersistsNothing(t *testing.T) {
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{{
		ID: "claude-code", Provider: "claude-code", WorkerEligible: true,
		ModelSelection: domain.ExecutionBindingModelProviderDefault, Readiness: domain.CapabilityUnsupported,
		Capabilities: map[string]domain.CapabilitySupport{}, Models: map[string]domain.CapabilitySupport{},
	}}}
	svc, store, outcomeID, _ := newPlanningTestService(t, router)
	// S3: no admissible route is a blocked worker_unavailable packet, not an
	// API error, and it persists no Plan.
	view, err := svc.ProposePlan(context.Background(), outcomeID, 2)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if view.Readiness == nil || view.Readiness.Status != domain.PlanningBlocked {
		t.Fatalf("no admissible route must produce a blocked packet: %+v", view.Readiness)
	}
	routeIssue := false
	for _, issue := range view.Readiness.Issues {
		if issue.Kind == domain.ReadinessWorkerUnavailable && issue.Route == domain.RouteChooseHarness {
			routeIssue = true
		}
	}
	if !routeIssue {
		t.Fatalf("blocked packet carries no worker_unavailable/choose_harness issue: %+v", view.Readiness.Issues)
	}
	if persisted := len(store.plans[outcomeID]); persisted != 0 {
		t.Fatalf("persisted %d plans", persisted)
	}
}

func TestEvaluateAdmissionUsesOneSnapshotAndExplicitModelPreference(t *testing.T) {
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{{ID: "codex", Provider: "codex", ModelSelection: domain.ExecutionBindingModelProviderDefault, WorkerEligible: true, Readiness: domain.CapabilitySupported, Capabilities: map[string]domain.CapabilitySupport{domain.CapabilityWorktreeRead: domain.CapabilitySupported, domain.CapabilityWorktreeWrite: domain.CapabilitySupported, domain.CapabilityWorktreeExec: domain.CapabilitySupported}, Models: map[string]domain.CapabilitySupport{"gpt-explicit": domain.CapabilitySupported}}}}
	svc, _, outcomeID, _ := newPlanningTestService(t, router)
	view, err := svc.Get(context.Background(), outcomeID)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := svc.ProposePlan(context.Background(), outcomeID, view.Current.Number)
	if err != nil {
		t.Fatal(err)
	}
	for i := range plan.Plan.WorkUnits {
		plan.Plan.WorkUnits[i].Provider = domain.HarnessCodex
		plan.Plan.WorkUnits[i].ModelSelection = domain.ExecutionBindingModelExplicit
		plan.Plan.WorkUnits[i].Model = "gpt-explicit"
		plan.Plan.WorkUnits[i].ExecutionBudget = testAdmissionPolicy().Default
	}
	before := router.calls
	_, err = svc.EvaluateAdmissionStage(context.Background(), ports.AdmissionStageInput{Stage: ports.AdmissionStageApproval, ProjectID: "mer", Outcome: &view.Outcome, Contract: &view.Current, Plan: &plan.Plan})
	if err != nil {
		t.Fatal(err)
	}
	if router.calls-before != 1 {
		t.Fatalf("snapshots=%d", router.calls-before)
	}
	if router.preference == nil || router.preference.Model != "gpt-explicit" {
		t.Fatalf("preference=%+v", router.preference)
	}
}

func TestApprovePlanReplayReturnsPersistedAdmissionWithoutReevaluation(t *testing.T) {
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}
	svc, _, outcomeID, _ := newPlanningTestService(t, router)
	proposal, err := svc.ProposePlan(context.Background(), outcomeID, 2)
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.ApprovePlan(context.Background(), outcomeID, outcome.ApprovePlanInput{PlanRevisionID: proposal.Plan.ID, ExpectedContractRevision: 2})
	if err != nil {
		t.Fatal(err)
	}
	calls := router.calls
	replay, err := svc.ApprovePlan(context.Background(), outcomeID, outcome.ApprovePlanInput{PlanRevisionID: proposal.Plan.ID, ExpectedContractRevision: 2})
	if err != nil {
		t.Fatal(err)
	}
	if replay.Plan.ID != first.Plan.ID || router.calls != calls {
		t.Fatalf("replay=%s calls=%d->%d", replay.Plan.ID, calls, router.calls)
	}
}

func TestEvaluateAdmissionInventoriesDistinctFrozenBindingsOnOneSnapshotVersion(t *testing.T) {
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessClaudeCode, "sonnet-test"), executionCandidate(domain.HarnessCodex, "gpt-test")}}
	svc, _, outcomeID, _ := newPlanningTestService(t, router)
	view, _ := svc.Get(context.Background(), outcomeID)
	plan, err := svc.ProposePlan(context.Background(), outcomeID, view.Current.Number)
	if err != nil {
		t.Fatal(err)
	}
	plan.Plan.WorkUnits[1].Provider = domain.HarnessCodex
	plan.Plan.WorkUnits[1].ModelSelection = domain.ExecutionBindingModelExplicit
	plan.Plan.WorkUnits[1].Model = "gpt-test"
	plan.Plan.RoutingDecisions[1].Decision.RecommendedProvider = string(domain.HarnessCodex)
	plan.Plan.RoutingDecisions[1].Decision.RecommendedModelSelection = domain.ExecutionBindingModelExplicit
	plan.Plan.RoutingDecisions[1].Decision.RecommendedModel = "gpt-test"
	before := router.calls
	result, err := svc.EvaluateAdmissionStage(context.Background(), ports.AdmissionStageInput{Stage: ports.AdmissionStageApproval, ProjectID: "mer", Outcome: &view.Outcome, Contract: &view.Current, Plan: &plan.Plan})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Eligible || router.calls-before != 2 {
		t.Fatalf("eligible=%v calls=%d", result.Eligible, router.calls-before)
	}
}

func TestEvaluateAdmissionRejectsDifferentInventoryGenerations(t *testing.T) {
	router := &routingInventoryFake{generationIDs: []string{"g1", "g2"}, snapshotIDs: []string{"same", "same"}, candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessClaudeCode, "sonnet-test"), executionCandidate(domain.HarnessCodex, "gpt-test")}}
	svc, _, outcomeID, _ := newPlanningTestService(t, router)
	view, _ := svc.Get(context.Background(), outcomeID)
	plan, err := svc.ProposePlan(context.Background(), outcomeID, view.Current.Number)
	if err != nil {
		t.Fatal(err)
	}
	plan.Plan.WorkUnits[1].Provider = domain.HarnessCodex
	plan.Plan.WorkUnits[1].ModelSelection = domain.ExecutionBindingModelExplicit
	plan.Plan.WorkUnits[1].Model = "gpt-test"
	result, err := svc.EvaluateAdmissionStage(context.Background(), ports.AdmissionStageInput{Stage: ports.AdmissionStageApproval, ProjectID: "mer", Outcome: &view.Outcome, Contract: &view.Current, Plan: &plan.Plan})
	if err != nil {
		t.Fatal(err)
	}
	if result.Eligible || result.Verdict.Reasons[0] != domain.AdmissionCapabilitySnapshotChanged {
		t.Fatalf("%+v", result)
	}
}

// A missing or invalid admission policy is a plan-wide operator error: at the
// staged service boundary (approval and Attempt start alike) it must surface
// as its typed reason before any routing inventory is read, never masked by a
// routing read failure.
func TestEvaluateAdmissionPolicyFailureBeatsRoutingReadFailureAtStagedBoundary(t *testing.T) {
	for _, stage := range []ports.AdmissionStage{ports.AdmissionStageApproval, ports.AdmissionStageStart} {
		t.Run(string(stage)+"-nil-policy", func(t *testing.T) {
			router := &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}
			svc, _, outcomeID, _ := newPlanningTestService(t, router)
			view, _ := svc.Get(context.Background(), outcomeID)
			plan, err := svc.ProposePlan(context.Background(), outcomeID, view.Current.Number)
			if err != nil {
				t.Fatal(err)
			}
			svc.AdmissionPolicy = nil
			router.err = fmt.Errorf("routing inventory read failed")
			calls := router.calls
			result, err := svc.EvaluateAdmissionStage(context.Background(), ports.AdmissionStageInput{Stage: stage, ProjectID: "mer", Outcome: &view.Outcome, Contract: &view.Current, Plan: &plan.Plan})
			if err != nil {
				t.Fatal(err)
			}
			if result.Eligible || len(result.Verdict.Reasons) != 1 || result.Verdict.Reasons[0] != domain.AdmissionPolicyMissing {
				t.Fatalf("%+v", result)
			}
			if router.calls != calls {
				t.Fatalf("routing inventory read despite policy failure: %d -> %d", calls, router.calls)
			}
		})
		t.Run(string(stage)+"-invalid-policy", func(t *testing.T) {
			router := &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}
			svc, _, outcomeID, _ := newPlanningTestService(t, router)
			view, _ := svc.Get(context.Background(), outcomeID)
			plan, err := svc.ProposePlan(context.Background(), outcomeID, view.Current.Number)
			if err != nil {
				t.Fatal(err)
			}
			policy := testAdmissionPolicy()
			policy.Digest = "corrupted"
			svc.AdmissionPolicy = policy
			router.err = fmt.Errorf("routing inventory read failed")
			result, err := svc.EvaluateAdmissionStage(context.Background(), ports.AdmissionStageInput{Stage: stage, ProjectID: "mer", Outcome: &view.Outcome, Contract: &view.Current, Plan: &plan.Plan})
			if err != nil {
				t.Fatal(err)
			}
			if result.Eligible || len(result.Verdict.Reasons) != 1 || result.Verdict.Reasons[0] != domain.AdmissionPolicyInvalid {
				t.Fatalf("%+v", result)
			}
		})
	}
}

// The same precedence holds against a routing generation split: the policy
// reason wins and no snapshot is read.
func TestEvaluateAdmissionPolicyFailureBeatsGenerationSplitAtStagedBoundary(t *testing.T) {
	router := &routingInventoryFake{generationIDs: []string{"g1", "g2"}, snapshotIDs: []string{"same", "same"}, candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessClaudeCode, "sonnet-test"), executionCandidate(domain.HarnessCodex, "gpt-test")}}
	svc, _, outcomeID, _ := newPlanningTestService(t, router)
	view, _ := svc.Get(context.Background(), outcomeID)
	plan, err := svc.ProposePlan(context.Background(), outcomeID, view.Current.Number)
	if err != nil {
		t.Fatal(err)
	}
	plan.Plan.WorkUnits[1].Provider = domain.HarnessCodex
	plan.Plan.WorkUnits[1].ModelSelection = domain.ExecutionBindingModelExplicit
	plan.Plan.WorkUnits[1].Model = "gpt-test"
	plan.Plan.RoutingDecisions[1].Decision.RecommendedProvider = string(domain.HarnessCodex)
	plan.Plan.RoutingDecisions[1].Decision.RecommendedModelSelection = domain.ExecutionBindingModelExplicit
	plan.Plan.RoutingDecisions[1].Decision.RecommendedModel = "gpt-test"
	policy := testAdmissionPolicy()
	policy.Digest = "corrupted"
	svc.AdmissionPolicy = policy
	calls := router.calls
	result, err := svc.EvaluateAdmissionStage(context.Background(), ports.AdmissionStageInput{Stage: ports.AdmissionStageApproval, ProjectID: "mer", Outcome: &view.Outcome, Contract: &view.Current, Plan: &plan.Plan})
	if err != nil {
		t.Fatal(err)
	}
	if result.Eligible || len(result.Verdict.Reasons) != 1 || result.Verdict.Reasons[0] != domain.AdmissionPolicyInvalid {
		t.Fatalf("%+v", result)
	}
	if router.calls != calls {
		t.Fatalf("routing inventory read despite policy failure: %d -> %d", calls, router.calls)
	}
}
