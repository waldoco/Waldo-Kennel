package outcome_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apierr"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

// planFakeStore keeps immutable Plan revisions and their unit/grant payloads so
// service tests exercise numbering, replay and approval CAS semantics without
// depending on SQLite.
type planFakeStore struct {
	*fakeStore

	mu         sync.Mutex
	plans      map[domain.OutcomeID][]domain.PlanRevision
	units      map[domain.PlanRevisionID][]domain.WorkUnit
	grants     map[domain.PlanRevisionID][]domain.CapabilityGrant
	admissions map[domain.PlanRevisionID]domain.AdmissionVerdict
	specs      map[domain.PlanRevisionID]map[domain.WorkUnitID]domain.ApprovedExecutableSpec
	launches   map[domain.AttemptID]domain.WorkspaceBoundLaunchPacket
}

func newPlanFakeStore() *planFakeStore {
	return &planFakeStore{
		fakeStore: &fakeStore{
			spaces:   map[domain.ProjectID]domain.ResponsibilitySpace{},
			outcomes: map[domain.OutcomeID]domain.Outcome{},
			revs:     map[domain.OutcomeID][]domain.ContractRevision{},
			keys:     map[string]domain.OutcomeID{},
		},
		plans:      map[domain.OutcomeID][]domain.PlanRevision{},
		units:      map[domain.PlanRevisionID][]domain.WorkUnit{},
		grants:     map[domain.PlanRevisionID][]domain.CapabilityGrant{},
		admissions: map[domain.PlanRevisionID]domain.AdmissionVerdict{},
		specs:      map[domain.PlanRevisionID]map[domain.WorkUnitID]domain.ApprovedExecutableSpec{},
		launches:   map[domain.AttemptID]domain.WorkspaceBoundLaunchPacket{},
	}
}

func (f *planFakeStore) AppendPlanRevision(_ context.Context, outcomeID domain.OutcomeID, plan domain.PlanRevision) (domain.PlanRevision, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	plan.Number = int64(len(f.plans[outcomeID]) + 1)
	if err := plan.Validate(); err != nil {
		return domain.PlanRevision{}, err
	}
	f.plans[outcomeID] = append(f.plans[outcomeID], plan)
	f.units[plan.ID] = append([]domain.WorkUnit(nil), plan.WorkUnits...)
	f.grants[plan.ID] = append([]domain.CapabilityGrant(nil), plan.Grants...)
	rememberFirstWorkUnit(plan)
	return plan, nil
}

func (f *planFakeStore) LatestProposedPlanRevision(_ context.Context, outcomeID domain.OutcomeID, contractRevision int64) (domain.PlanRevision, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var best domain.PlanRevision
	found := false
	for _, plan := range f.plans[outcomeID] {
		if plan.Status == domain.PlanStatusProposed && plan.ContractRevisionNumber == contractRevision {
			if !found || plan.Number > best.Number {
				best, found = plan, true
			}
		}
	}
	return best, found, nil
}

func (f *planFakeStore) GetPlanRevision(_ context.Context, outcomeID domain.OutcomeID, planID domain.PlanRevisionID) (domain.PlanRevision, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, plan := range f.plans[outcomeID] {
		if plan.ID == planID {
			out := plan
			out.WorkUnits, out.Grants = append([]domain.WorkUnit(nil), f.units[planID]...), append([]domain.CapabilityGrant(nil), f.grants[planID]...)
			return out, true, nil
		}
	}
	return domain.PlanRevision{}, false, nil
}

func (f *planFakeStore) GetLatestPlanRevision(_ context.Context, outcomeID domain.OutcomeID) (domain.PlanRevision, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := len(f.plans[outcomeID])
	if n == 0 {
		return domain.PlanRevision{}, false, nil
	}
	plan := f.plans[outcomeID][n-1]
	plan.WorkUnits, plan.Grants = append([]domain.WorkUnit(nil), f.units[plan.ID]...), append([]domain.CapabilityGrant(nil), f.grants[plan.ID]...)
	return plan, true, nil
}

func (f *planFakeStore) ApprovePlanRevision(_ context.Context, outcomeID domain.OutcomeID, planID domain.PlanRevisionID) (domain.PlanRevision, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, plan := range f.plans[outcomeID] {
		if plan.ID != planID {
			continue
		}
		if plan.Status == domain.PlanStatusApproved {
			out := plan
			out.WorkUnits, out.Grants = append([]domain.WorkUnit(nil), f.units[planID]...), append([]domain.CapabilityGrant(nil), f.grants[planID]...)
			return out, true, nil
		}
		f.plans[outcomeID][i].Status = domain.PlanStatusApproved
		out := f.plans[outcomeID][i]
		out.WorkUnits, out.Grants = append([]domain.WorkUnit(nil), f.units[planID]...), append([]domain.CapabilityGrant(nil), f.grants[planID]...)
		return out, true, nil
	}
	return domain.PlanRevision{}, false, nil
}

func (f *planFakeStore) GetAdmittedVerdict(_ context.Context, planID domain.PlanRevisionID) (domain.AdmissionVerdict, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	verdict, ok := f.admissions[planID]
	return verdict, ok, nil
}

func (f *planFakeStore) AppendAdmissionEvaluation(_ context.Context, verdict domain.AdmissionVerdict) error {
	return verdict.Validate()
}

func (f *planFakeStore) ApprovePlanWithAdmission(ctx context.Context, outcomeID domain.OutcomeID, planID domain.PlanRevisionID, verdict domain.AdmissionVerdict) (domain.PlanRevision, bool, error) {
	if err := verdict.Validate(); err != nil {
		return domain.PlanRevision{}, true, err
	}
	approved, found, err := f.ApprovePlanRevision(ctx, outcomeID, planID)
	if err != nil || !found {
		return approved, found, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.admissions[planID] = verdict
	f.specs[planID] = map[domain.WorkUnitID]domain.ApprovedExecutableSpec{}
	for _, wu := range verdict.WorkUnits {
		if wu.Executable != nil {
			f.specs[planID][wu.WorkUnitID] = *wu.Executable
		}
	}
	return approved, true, nil
}
func (f *planFakeStore) GetApprovedExecutableSpec(_ context.Context, planID domain.PlanRevisionID, unitID domain.WorkUnitID) (domain.ApprovedExecutableSpec, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	spec, ok := f.specs[planID][unitID]
	return spec, ok, nil
}
func (f *planFakeStore) PersistWorkspaceBoundLaunchPacket(_ context.Context, packet domain.WorkspaceBoundLaunchPacket) error {
	if err := packet.Validate(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.launches[packet.AttemptID]; ok {
		return errors.New("duplicate launch packet")
	}
	f.launches[packet.AttemptID] = packet
	return nil
}
func (f *planFakeStore) GetWorkspaceBoundLaunchPacket(_ context.Context, attemptID domain.AttemptID) (domain.WorkspaceBoundLaunchPacket, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.launches[attemptID]
	return p, ok, nil
}

func apiCode(t *testing.T, err error) string {
	t.Helper()
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected apierr, got %T: %v", err, err)
	}
	return apiErr.Code
}

func TestProposePlanReentryIsIdempotent(t *testing.T) {
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}
	svc, store, outcomeID, provider := newPlanningTestService(t, router)

	first, err := svc.ProposePlan(context.Background(), outcomeID, 2)
	if err != nil {
		t.Fatalf("first propose: %v", err)
	}
	second, err := svc.ProposePlan(context.Background(), outcomeID, 2)
	if err != nil {
		t.Fatalf("re-entry propose: %v", err)
	}
	if second.Plan.ID != first.Plan.ID || second.Plan.RunBriefCoreDigest != first.Plan.RunBriefCoreDigest {
		t.Fatal("ordinary re-entry must return the immutable existing proposal")
	}
	if provider.calls != 1 {
		t.Fatalf("plan intelligence calls = %d, want 1", provider.calls)
	}
	if got := len(store.plans[outcomeID]); got != 1 {
		t.Fatalf("persisted plans = %d, want 1", got)
	}
}

func TestProposePlanSettingsReadFailurePreventsIntelligenceRun(t *testing.T) {
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}
	svc, store, outcomeID, provider := newPlanningTestService(t, router)
	want := errors.New("settings read failed")
	svc.WithRepositoryContextLimits(fixedRepositoryContextLimits{err: want})

	_, err := svc.ProposePlan(context.Background(), outcomeID, 2)
	if !errors.Is(err, want) {
		t.Fatalf("ProposePlan() error = %v, want surfaced settings failure", err)
	}
	if provider.calls != 0 {
		t.Fatalf("plan intelligence calls = %d, want zero", provider.calls)
	}
	if got := len(store.plans[outcomeID]); got != 0 {
		t.Fatalf("settings failure persisted %d plan proposals, want zero", got)
	}
}

func TestReplanPlanWithFeedbackCreatesNewImmutableProposal(t *testing.T) {
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}
	svc, store, outcomeID, provider := newPlanningTestService(t, router)
	first, err := svc.ProposePlan(context.Background(), outcomeID, 2)
	if err != nil {
		t.Fatalf("first propose: %v", err)
	}
	second, err := svc.ReplanPlan(context.Background(), outcomeID, 2, "Use the repository's distinctive check command")
	if err != nil {
		t.Fatalf("replan: %v", err)
	}
	if second.Plan.ID == first.Plan.ID || second.Plan.Number != 2 {
		t.Fatalf("replan reused proposal: first=%+v second=%+v", first.Plan, second.Plan)
	}
	if provider.calls != 2 || len(store.plans[outcomeID]) != 2 {
		t.Fatalf("replan calls/plans = %d/%d, want 2/2", provider.calls, len(store.plans[outcomeID]))
	}
}

func TestProposePlanRejectsStaleContractPointer(t *testing.T) {
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}
	svc, _, outcomeID, _ := newPlanningTestService(t, router)
	if _, err := svc.ProposePlan(context.Background(), outcomeID, 99); err == nil {
		t.Fatal("stale pointer must be refused")
	} else if code := apiCode(t, err); code != "PLAN_CONTRACT_STALE" {
		t.Fatalf("code = %s, want PLAN_CONTRACT_STALE", code)
	}
}

func TestContractRevisionMovementInvalidatesEarlierPlan(t *testing.T) {
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}
	svc, _, outcomeID, _ := newPlanningTestService(t, router)
	ctx := context.Background()

	first, err := svc.ProposePlan(ctx, outcomeID, 2)
	if err != nil {
		t.Fatalf("propose r2: %v", err)
	}
	if _, err := svc.ReviseContract(ctx, outcomeID, outcome.ReviseContractInput{
		ExpectedRevision: 2,
		Goal:             "Ship and verify the bounded change with an owner note.",
		SuccessCriteria: []string{
			"Implementation is present.",
			"Verification proves the implementation behaves as required.",
		},
		Review:           "Run deterministic verification and inspect the owner note.",
		AuthorityCeiling: fullLocalAuthority(),
		StopConditions:   []string{"Stop before remote effects."},
	}); err != nil {
		t.Fatalf("revise contract: %v", err)
	}

	if _, err := svc.ApprovePlan(ctx, outcomeID, outcome.ApprovePlanInput{PlanRevisionID: first.Plan.ID, ExpectedContractRevision: 3}); err == nil {
		t.Fatal("plan bound to superseded Contract must not approve")
	} else if code := apiCode(t, err); code != "PLAN_CONTRACT_STALE" {
		t.Fatalf("code = %s, want PLAN_CONTRACT_STALE", code)
	}

	next, err := svc.ProposePlan(ctx, outcomeID, 3)
	if err != nil {
		t.Fatalf("propose r3: %v", err)
	}
	if next.Plan.ContractRevisionNumber != 3 || next.Plan.RunBriefCoreDigest == first.Plan.RunBriefCoreDigest {
		t.Fatal("material Contract revision must produce a fresh frozen Plan")
	}
	approved, err := svc.ApprovePlan(ctx, outcomeID, outcome.ApprovePlanInput{PlanRevisionID: next.Plan.ID, ExpectedContractRevision: 3})
	if err != nil {
		t.Fatalf("approve r3: %v", err)
	}
	if approved.Plan.Status != domain.PlanStatusApproved {
		t.Fatalf("status = %s, want approved", approved.Plan.Status)
	}
}

func TestProposeFailsClosedWhenDaemonPolicyNarrows(t *testing.T) {
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}
	svc, store, outcomeID, _ := newPlanningTestService(t, router)
	svc.PolicyLayers = [][]string{{domain.CapabilityWorktreeRead}}

	if _, err := svc.ProposePlan(context.Background(), outcomeID, 2); err == nil {
		t.Fatal("narrowed daemon authority must fail proposal closed")
	} else if code := apiCode(t, err); code != "PLAN_CAPABILITY_UNAUTHORIZED" {
		t.Fatalf("code = %s, want PLAN_CAPABILITY_UNAUTHORIZED", code)
	}
	if got := len(store.plans[outcomeID]); got != 0 {
		t.Fatalf("refused proposal persisted %d plans", got)
	}
}

func TestLowerPolicyLayerCannotWidenAuthority(t *testing.T) {
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}
	svc, store, outcomeID, _ := newPlanningTestService(t, router)
	svc.PolicyLayers = [][]string{
		{domain.CapabilityWorktreeRead},
		{domain.CapabilityWorktreeRead, domain.CapabilityWorktreeWrite, domain.CapabilityWorktreeExec, "network.fetch"},
	}
	if _, err := svc.ProposePlan(context.Background(), outcomeID, 2); err == nil {
		t.Fatal("lower layer widening must fail closed")
	} else if code := apiCode(t, err); code != "PLAN_CAPABILITY_UNAUTHORIZED" {
		t.Fatalf("code = %s, want PLAN_CAPABILITY_UNAUTHORIZED", code)
	}
	if got := len(store.plans[outcomeID]); got != 0 {
		t.Fatalf("refused proposal persisted %d plans", got)
	}
}

func TestApproveRechecksCurrentPolicyWithoutRerouting(t *testing.T) {
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}
	svc, _, outcomeID, _ := newPlanningTestService(t, router)
	view, err := svc.ProposePlan(context.Background(), outcomeID, 2)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	svc.PolicyLayers = [][]string{{domain.CapabilityWorktreeRead}}
	if _, err := svc.ApprovePlan(context.Background(), outcomeID, outcome.ApprovePlanInput{PlanRevisionID: view.Plan.ID, ExpectedContractRevision: 2}); err == nil {
		t.Fatal("approval must recheck the current authority ceiling")
	} else if code := apiCode(t, err); code != "PLAN_CAPABILITY_UNAUTHORIZED" {
		t.Fatalf("code = %s, want PLAN_CAPABILITY_UNAUTHORIZED", code)
	}
}

func TestApproveUnknownPlanIsNotFound(t *testing.T) {
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}
	svc, _, outcomeID, _ := newPlanningTestService(t, router)
	if _, err := svc.ProposePlan(context.Background(), outcomeID, 2); err != nil {
		t.Fatalf("propose: %v", err)
	}
	if _, err := svc.ApprovePlan(context.Background(), outcomeID, outcome.ApprovePlanInput{PlanRevisionID: "plan-missing", ExpectedContractRevision: 2}); err == nil {
		t.Fatal("unknown plan must 404")
	} else if code := apiCode(t, err); code != "PLAN_NOT_FOUND" {
		t.Fatalf("code = %s, want PLAN_NOT_FOUND", code)
	}
	if _, err := svc.GetLatestPlan(context.Background(), "out-ghost"); err == nil {
		t.Fatal("plans for unknown outcomes must 404")
	}
}

func TestProposePlanWithoutAdmissionPolicySkipsModelCall(t *testing.T) {
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}
	svc, store, outcomeID, provider := newPlanningTestService(t, router)
	svc.AdmissionPolicy = nil

	_, err := svc.ProposePlan(context.Background(), outcomeID, 2)
	if err == nil {
		t.Fatal("propose without admission policy must fail closed")
	}
	if code := apiCode(t, err); code != "PLAN_PROPOSAL_NOT_ADMITTED" {
		t.Fatalf("code = %s, want PLAN_PROPOSAL_NOT_ADMITTED", code)
	}
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type %T, want *apierr.Error", err)
	}
	reasons, ok := apiErr.Details["reasons"].([]domain.AdmissionReasonCode)
	if !ok || len(reasons) != 1 || reasons[0] != domain.AdmissionPolicyMissing {
		t.Fatalf("reasons = %v, want [admission_policy_missing]", apiErr.Details["reasons"])
	}
	if provider.calls != 0 {
		t.Fatalf("plan intelligence calls = %d, want zero (no model spend without policy)", provider.calls)
	}
	if got := len(store.plans[outcomeID]); got != 0 {
		t.Fatalf("persisted plans = %d, want zero", got)
	}
}

func TestProposePlanWithInvalidAdmissionPolicySkipsModelCall(t *testing.T) {
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}
	svc, _, outcomeID, provider := newPlanningTestService(t, router)
	policy := testAdmissionPolicy()
	policy.Digest = "corrupted"
	svc.AdmissionPolicy = policy

	_, err := svc.ProposePlan(context.Background(), outcomeID, 2)
	if err == nil {
		t.Fatal("propose with invalid admission policy must fail closed")
	}
	if code := apiCode(t, err); code != "PLAN_PROPOSAL_NOT_ADMITTED" {
		t.Fatalf("code = %s, want PLAN_PROPOSAL_NOT_ADMITTED", code)
	}
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type %T, want *apierr.Error", err)
	}
	reasons, ok := apiErr.Details["reasons"].([]domain.AdmissionReasonCode)
	if !ok || len(reasons) != 1 || reasons[0] != domain.AdmissionPolicyInvalid {
		t.Fatalf("reasons = %v, want [admission_policy_invalid]", apiErr.Details["reasons"])
	}
	if provider.calls != 0 {
		t.Fatalf("plan intelligence calls = %d, want zero (no model spend with invalid policy)", provider.calls)
	}
}
