package outcome_test

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/intelligence/intelligencetest"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

// threeUnitProvider drafts the smallest honest chain: three dependent units
// A -> B -> C, each owning exactly one criterion so every attempt can be
// proved and classified in turn. It keeps the shared fake for everything but
// the plan shape. The three criteria come from the create input.
type threeUnitProvider struct {
	intelligencetest.Provider
}

func (threeUnitProvider) DraftPlan(_ context.Context, request ports.PlanIntelligenceRequest) (ports.PlanIntelligenceResponse, error) {
	covered := make([]string, 0, len(request.CriterionAliases))
	for alias := range request.CriterionAliases {
		covered = append(covered, alias)
	}
	sort.Strings(covered)
	intent := domain.WorkUnitIntentModifyAndExecute
	ceiling := request.Contract.AuthorityCeiling
	if !ceiling.WriteWorkspace || !ceiling.ExecuteLocal {
		intent = domain.WorkUnitIntentInspect
	}
	unit := func(key, title string, dependsOn []string, criteria []string) domain.PlanDraftWorkUnit {
		inputs := make([]domain.PlanDraftDependencyInput, 0, len(dependsOn))
		for _, dep := range dependsOn {
			inputs = append(inputs, domain.PlanDraftDependencyInput{FromKey: dep, Required: dep + " output"})
		}
		return domain.PlanDraftWorkUnit{
			Key: key, Title: title, Intent: intent, Role: domain.WorkUnitRoleImplement,
			Inputs: inputs, OutputSummary: title + " output, built and verified inside the isolated project worktree.",
			CriteriaCovered: criteria, DependsOn: dependsOn,
			EvidenceIdeas: []string{"A deterministic check demonstrates the " + title + " result."},
		}
	}
	criterion := func(i int) []string {
		if i < len(covered) {
			return []string{covered[i]}
		}
		return nil
	}
	return ports.PlanIntelligenceResponse{
		Readiness: domain.NewPlanningReadinessResult("Ready.", &domain.PlanDraftProposal{
			Summary: "Deliver the outcome as a serial three-unit chain.",
			WorkUnits: []domain.PlanDraftWorkUnit{
				unit("W1", "Stage A", nil, criterion(0)),
				unit("W2", "Stage B", []string{"W1"}, criterion(1)),
				unit("W3", "Stage C", []string{"W2"}, criterion(2)),
			},
		}, nil),
		Provenance: ports.IntelligenceProvenance{EffectiveProvider: intelligencetest.ProviderID, EffectiveModel: "fixed"},
	}, nil
}

// serialHarness is a three-unit chain wired through the real service: receipt
// store, execution double, approved plan. Nothing has run yet.
type serialHarness struct {
	svc             *outcome.Service
	store           *receiptFakeStore
	spawner         *fakeSpawner
	outcomeID       domain.OutcomeID
	planID          domain.PlanRevisionID
	contract        domain.ContractRevision
	criterionByUnit map[domain.WorkUnitID]domain.CriterionID
	unitA           domain.WorkUnitID
	unitB           domain.WorkUnitID
	unitC           domain.WorkUnitID
}

func newSerialHarness(t *testing.T) *serialHarness {
	t.Helper()
	store := newReceiptFakeStore()
	spawner := &fakeSpawner{readiness: ports.AgentProfileReadiness{Ready: true, Detail: "profile ok"}}
	svc := outcome.New(store, func() time.Time { return time.Unix(1_000, 0).UTC() }).
		WithPlanning(threeUnitProvider{}, &routingInventoryFake{candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessCodex, "")}}).
		WithExecution(spawner, newFakeHeartbeats())
	svc.AdmissionPolicy = testAdmissionPolicy()

	ctx := context.Background()
	create := validCreateInput()
	create.SuccessCriteria = []string{
		"The stage A result is observable.",
		"The stage B result is observable.",
		"The stage C result is observable.",
	}
	view, err := svc.Create(ctx, create)
	if err != nil {
		t.Fatalf("create outcome: %v", err)
	}
	planView, err := svc.ProposePlan(ctx, view.Outcome.ID, 1)
	if err != nil {
		t.Fatalf("propose three-unit plan: %v", err)
	}
	if got := len(planView.Plan.WorkUnits); got != 3 {
		t.Fatalf("plan units = %d, want three", got)
	}
	if _, err := svc.ApprovePlan(ctx, view.Outcome.ID, outcome.ApprovePlanInput{
		PlanRevisionID: planView.Plan.ID, ExpectedContractRevision: 1,
	}); err != nil {
		t.Fatalf("approve plan: %v", err)
	}
	byTitle := map[string]domain.WorkUnitID{}
	criterionByUnit := map[domain.WorkUnitID]domain.CriterionID{}
	for _, unit := range planView.Plan.WorkUnits {
		byTitle[unit.Title] = unit.ID
		if len(unit.CriterionIDs) != 1 {
			t.Fatalf("unit %s criteria = %d, want exactly one", unit.Title, len(unit.CriterionIDs))
		}
		criterionByUnit[unit.ID] = unit.CriterionIDs[0]
	}
	return &serialHarness{
		svc: svc, store: store, spawner: spawner,
		outcomeID: view.Outcome.ID, planID: planView.Plan.ID,
		contract: view.Current, criterionByUnit: criterionByUnit,
		unitA: byTitle["Stage A"], unitB: byTitle["Stage B"], unitC: byTitle["Stage C"],
	}
}

// startDaemon drives the scheduler-selected admission, the way a run would.
func (h *serialHarness) startDaemon(t *testing.T, key string) outcome.AttemptView {
	t.Helper()
	view, err := h.svc.StartAttempt(context.Background(), h.outcomeID, outcome.StartAttemptInput{
		PlanRevisionID: h.planID, RequestKey: key,
	})
	if err != nil {
		t.Fatalf("daemon-selected start: %v", err)
	}
	return view
}

// complete mirrors the daemon's retention path for a finished session: the
// attempt ends, its exact result is retained complete and frozen, and
// reconciliation classifies it.
func (h *serialHarness) complete(t *testing.T, attemptID domain.AttemptID) {
	t.Helper()
	ctx := context.Background()
	attempts, err := h.store.ListAttempts(ctx, h.outcomeID)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	var attempt domain.Attempt
	for _, a := range attempts {
		if a.ID == attemptID {
			attempt = a
		}
	}
	if attempt.ID == "" {
		t.Fatalf("attempt %s not found", attemptID)
	}
	if _, err := h.store.TransitionAttemptStatus(ctx, h.outcomeID, attempt.ID,
		attempt.Status, domain.AttemptReconciled, time.Unix(1_000, 0).UTC()); err != nil {
		t.Fatalf("end attempt: %v", err)
	}
	attempt.Status = domain.AttemptReconciled
	receipt := retainedReceiptFor(attempt)
	if err := h.store.SaveAttemptReceipt(ctx, receipt); err != nil {
		t.Fatalf("retain receipt: %v", err)
	}
	h.prove(t, attempt, receipt)
	if err := h.svc.ReconcileAttemptOutcomes(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	attempts, err = h.store.ListAttempts(ctx, h.outcomeID)
	if err != nil {
		t.Fatalf("relist attempts: %v", err)
	}
	for _, a := range attempts {
		if a.ID == attemptID && a.Status != domain.AttemptSucceeded {
			t.Fatalf("attempt %s status = %s, want succeeded after reconcile", attemptID, a.Status)
		}
	}
}

// prove records the same passing-evidence-plus-verification pair the governed
// check would, bound to this exact attempt and the bytes it retained. That is
// what reconciliation needs to classify the attempt succeeded.
func (h *serialHarness) prove(t *testing.T, attempt domain.Attempt, receipt domain.AttemptReceipt) {
	t.Helper()
	ctx := context.Background()
	criterionID, ok := h.criterionByUnit[attempt.WorkUnitID]
	if !ok {
		t.Fatalf("no criterion mapped for unit %s", attempt.WorkUnitID)
	}
	key := string(attempt.ID)
	if _, err := h.svc.RecordEvidence(ctx, h.outcomeID, outcome.RecordEvidenceInput{
		ExpectedContractRevision: h.contract.Number, ContractRevisionID: h.contract.ID,
		CriterionID: criterionID, SubjectType: domain.ProofSubjectAttempt,
		SubjectID: string(attempt.ID), SubjectRevision: receipt.ArtifactVersion,
		Kind: domain.EvidenceSupporting, SourceType: domain.EvidenceSourceDeterministicCheck,
		SourceRef: "go test ./...", ProducerType: domain.EvidenceProducerTool, ProducerRef: string(attempt.ID),
		Summary: "exited 0", ContentDigest: strings.Repeat("b", 64), RequestKey: "ev-" + key,
	}); err != nil {
		t.Fatalf("record evidence: %v", err)
	}
	item, found, err := h.store.FindEvidenceItemByRequestKey(ctx, "ev-"+key)
	if err != nil || !found {
		t.Fatalf("read evidence: found=%v err=%v", found, err)
	}
	if _, err := h.svc.RecordVerification(ctx, h.outcomeID, outcome.RecordVerificationInput{
		ExpectedContractRevision: h.contract.Number, ContractRevisionID: h.contract.ID,
		CriterionID: criterionID, SubjectType: domain.ProofSubjectAttempt,
		SubjectID: string(attempt.ID), SubjectRevision: receipt.ArtifactVersion,
		EvidenceItemIDs: []domain.EvidenceItemID{item.ID}, Method: "go test ./...",
		IndependenceClass: domain.VerificationDeterministic, Result: domain.VerificationPassed,
		ProducerRef: string(attempt.ID), VerifierRef: "kennel-governed-check/test", RequestKey: "ver-" + key,
	}); err != nil {
		t.Fatalf("record verification: %v", err)
	}
}

func (h *serialHarness) attempts(t *testing.T) []domain.Attempt {
	t.Helper()
	attempts, err := h.store.ListAttempts(context.Background(), h.outcomeID)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	return attempts
}

// TestSerialChain_ThreeUnitsRunInOrderAndClose is the 7B spine: three dependent
// WorkUnits admitted one at a time in dependency order, each successor gated
// on its predecessor's proved result, and nothing left runnable at the end.
func TestSerialChain_ThreeUnitsRunInOrderAndClose(t *testing.T) {
	h := newSerialHarness(t)

	// The scheduler may only ever offer the chain head.
	first := h.startDaemon(t, "rk-a")
	if first.Attempt.WorkUnitID != h.unitA {
		t.Fatalf("first admission = %s, want stage A", first.Attempt.WorkUnitID)
	}
	h.complete(t, first.Attempt.ID)

	second := h.startDaemon(t, "rk-b")
	if second.Attempt.WorkUnitID != h.unitB {
		t.Fatalf("second admission = %s, want stage B", second.Attempt.WorkUnitID)
	}
	// A replayed start of the same unit returns the recorded admission; the
	// serial order does not fork under retry.
	replay, err := h.svc.StartAttempt(context.Background(), h.outcomeID, outcome.StartAttemptInput{
		PlanRevisionID: h.planID, RequestKey: "rk-b",
	})
	if err != nil {
		t.Fatalf("replayed start: %v", err)
	}
	if replay.Attempt.ID != second.Attempt.ID {
		t.Fatalf("replay admitted %s, want the recorded %s", replay.Attempt.ID, second.Attempt.ID)
	}
	if calls := h.spawner.spawnCalls(); calls != 2 {
		t.Fatalf("replay spawned: calls = %d, want 2", calls)
	}
	h.complete(t, second.Attempt.ID)

	third := h.startDaemon(t, "rk-c")
	if third.Attempt.WorkUnitID != h.unitC {
		t.Fatalf("third admission = %s, want stage C", third.Attempt.WorkUnitID)
	}
	h.complete(t, third.Attempt.ID)

	// The chain is done: the scheduler has nothing left to offer.
	if _, err := h.svc.StartAttempt(context.Background(), h.outcomeID, outcome.StartAttemptInput{
		PlanRevisionID: h.planID, RequestKey: "rk-after-close",
	}); requireAPICode(t, err) != outcome.CodeNoRunnableWorkUnit {
		t.Fatalf("start after close err = %v, want %s", err, outcome.CodeNoRunnableWorkUnit)
	}
	if got := len(h.attempts(t)); got != 3 {
		t.Fatalf("attempts = %d, want exactly three", got)
	}
	if calls := h.spawner.spawnCalls(); calls != 3 {
		t.Fatalf("spawn calls = %d, want exactly three", calls)
	}
}

// TestSerialChain_SuccessorRefusedUntilPredecessorProven is the out-of-order
// and skipped-predecessor falsifier: B and C cannot be admitted early - the
// scheduler refuses any unit that is not the next dependency-ready one - and
// a refused admission leaves no durable row and no provider launch.
func TestSerialChain_SuccessorRefusedUntilPredecessorProven(t *testing.T) {
	h := newSerialHarness(t)
	ctx := context.Background()

	for _, unit := range []domain.WorkUnitID{h.unitB, h.unitC} {
		_, err := h.svc.StartAttempt(ctx, h.outcomeID, outcome.StartAttemptInput{
			PlanRevisionID: h.planID, WorkUnitID: unit, RequestKey: "rk-early-" + string(unit),
		})
		if requireAPICode(t, err) != outcome.CodeWorkUnitNotRunnable {
			t.Fatalf("early start of %s err = %v, want %s", unit, err, outcome.CodeWorkUnitNotRunnable)
		}
	}
	if got := len(h.attempts(t)); got != 0 {
		t.Fatalf("refusals left %d attempts, want none", got)
	}
	if calls := h.spawner.spawnCalls(); calls != 0 {
		t.Fatalf("refusals launched %d providers, want none", calls)
	}

	// After A proves, B admits - but C still refuses: one hop of proof opens
	// exactly one hop of admission.
	first := h.startDaemon(t, "rk-a")
	h.complete(t, first.Attempt.ID)
	if _, err := h.svc.StartAttempt(ctx, h.outcomeID, outcome.StartAttemptInput{
		PlanRevisionID: h.planID, WorkUnitID: h.unitC, RequestKey: "rk-early-c2",
	}); requireAPICode(t, err) != outcome.CodeWorkUnitNotRunnable {
		t.Fatalf("C after only A err = %v, want %s", err, outcome.CodeWorkUnitNotRunnable)
	}
	second := h.startDaemon(t, "rk-b")
	if second.Attempt.WorkUnitID != h.unitB {
		t.Fatalf("after A the scheduler offered %s, want stage B", second.Attempt.WorkUnitID)
	}
}

// TestSerialChain_ActiveAttemptHoldsTheChain is the duplicate-successor
// falsifier: while one unit runs, no second admission - of any unit - is
// possible, and a fresh request key does not mint parallel work. These are
// sequential calls; nothing here is a concurrent-StartAttempt race proof.
func TestSerialChain_ActiveAttemptHoldsTheChain(t *testing.T) {
	h := newSerialHarness(t)
	ctx := context.Background()

	first := h.startDaemon(t, "rk-a")
	for _, in := range []outcome.StartAttemptInput{
		{PlanRevisionID: h.planID, RequestKey: "rk-dup-daemon"},
		{PlanRevisionID: h.planID, WorkUnitID: h.unitA, RequestKey: "rk-dup-a"},
		{PlanRevisionID: h.planID, WorkUnitID: h.unitB, RequestKey: "rk-dup-b"},
	} {
		if _, err := h.svc.StartAttempt(ctx, h.outcomeID, in); err == nil {
			t.Fatalf("second admission with key %s succeeded while %s runs", in.RequestKey, first.Attempt.WorkUnitID)
		}
	}
	if got := len(h.attempts(t)); got != 1 {
		t.Fatalf("attempts = %d, want the single running one", got)
	}
	if calls := h.spawner.spawnCalls(); calls != 1 {
		t.Fatalf("spawn calls = %d, want one", calls)
	}
}

// enablingChainProvider drafts a criterion-less enabling unit feeding one
// criterion-owning unit: the shape that used to compile into a deadlock,
// because the enabling attempt could never be proved and its consumer would
// block forever. Draft validation now refuses it truthfully for every intent.
type enablingChainProvider struct {
	intelligencetest.Provider
	intent domain.WorkUnitIntent
}

// enablingChainDraft is the one shared proposal shape, so the provider and
// the direct readiness-surface call exercise exactly the same draft. The role
// matches the intent so only the missing criterion is illegal.
func enablingChainDraft(intent domain.WorkUnitIntent) domain.PlanDraftProposal {
	role := domain.WorkUnitRoleImplement
	if intent == domain.WorkUnitIntentInspect {
		role = domain.WorkUnitRoleInvestigate
	}
	return domain.PlanDraftProposal{
		Summary: "An enabling stage feeding the real work.",
		WorkUnits: []domain.PlanDraftWorkUnit{
			{
				Key: "W1", Title: "Prepare", Intent: intent,
				Role: role, OutputSummary: "Preparation output.",
				DependsOn: nil, CriteriaCovered: nil,
				EvidenceIdeas: []string{"Preparation is observable."},
			},
			{
				Key: "W2", Title: "Deliver", Intent: intent,
				Role:    role,
				Inputs:  []domain.PlanDraftDependencyInput{{FromKey: "W1", Required: "W1 output"}},
				OutputSummary:   "Delivery output.",
				CriteriaCovered: []string{"C1"}, DependsOn: []string{"W1"},
				EvidenceIdeas: []string{"A deterministic check demonstrates the delivery."},
			},
		},
	}
}

func (p enablingChainProvider) DraftPlan(_ context.Context, _ ports.PlanIntelligenceRequest) (ports.PlanIntelligenceResponse, error) {
	draft := enablingChainDraft(p.intent)
	return ports.PlanIntelligenceResponse{
		Readiness:  domain.NewPlanningReadinessResult("Ready.", &draft, nil),
		Provenance: ports.IntelligenceProvenance{EffectiveProvider: intelligencetest.ProviderID, EffectiveModel: "fixed"},
	}, nil
}

// TestSerialChain_CriterionlessUnitRejectedAtDraft is the regression
// falsifier for the launch ruling: a unit with zero covered criteria is
// refused at draft with a truthful typed code for EVERY intent shape,
// through both the proposal surface (ProposePlan) and the readiness surface
// (EvaluatePlanReadiness), instead of compiling into a chain whose enabling
// unit can never succeed. The valid three-unit chain above proves the same
// validation still passes honest chains.
func TestSerialChain_CriterionlessUnitRejectedAtDraft(t *testing.T) {
	for _, intent := range []domain.WorkUnitIntent{
		domain.WorkUnitIntentInspect, domain.WorkUnitIntentModify,
		domain.WorkUnitIntentExecute, domain.WorkUnitIntentModifyAndExecute,
	} {
		t.Run(string(intent), func(t *testing.T) {
			store := newReceiptFakeStore()
			spawner := &fakeSpawner{readiness: ports.AgentProfileReadiness{Ready: true, Detail: "profile ok"}}
			svc := outcome.New(store, func() time.Time { return time.Unix(1_000, 0).UTC() }).
				WithPlanning(enablingChainProvider{intent: intent}, &routingInventoryFake{candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessCodex, "")}}).
				WithExecution(spawner, newFakeHeartbeats())
			svc.AdmissionPolicy = testAdmissionPolicy()

			ctx := context.Background()
			view, err := svc.Create(ctx, validCreateInput())
			if err != nil {
				t.Fatalf("create outcome: %v", err)
			}
			if _, err := svc.ProposePlan(ctx, view.Outcome.ID, 1); requireAPICode(t, err) != string(domain.PlanDraftUnitRequiresCriterion) {
				t.Fatalf("propose err = %v, want %s", err, domain.PlanDraftUnitRequiresCriterion)
			}

			draft := enablingChainDraft(intent)
			if _, _, err := svc.EvaluatePlanReadiness(ctx, readinessFence(), "p1", readinessContract(fullLocalAuthority(), readinessCriterion(1)), nil, &draft, nil, ""); requireAPICode(t, err) != string(domain.PlanDraftUnitRequiresCriterion) {
				t.Fatalf("readiness err = %v, want %s", err, domain.PlanDraftUnitRequiresCriterion)
			}
		})
	}
}
