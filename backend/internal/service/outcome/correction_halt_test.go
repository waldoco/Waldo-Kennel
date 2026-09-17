package outcome_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/intelligence/intelligencetest"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

// listAttemptsFailingStore fails the next ListAttempts read, which is the call
// the stop effect of a recorded cancellation depends on. It models the window
// where the intent row committed and the effect it implies did not.
type listAttemptsFailingStore struct {
	*receiptFakeStore
	failNext bool
}

func (s *listAttemptsFailingStore) ListAttempts(ctx context.Context, outcomeID domain.OutcomeID) ([]domain.Attempt, error) {
	if s.failNext {
		s.failNext = false
		return nil, errors.New("injected list-attempts failure")
	}
	return s.receiptFakeStore.ListAttempts(ctx, outcomeID)
}

// newHaltHarness is the rework harness with a store whose ListAttempts can be
// made to fail once.
func newHaltHarness(t *testing.T) (*reworkHarness, *listAttemptsFailingStore) {
	t.Helper()
	base := newReceiptFakeStore()
	store := &listAttemptsFailingStore{receiptFakeStore: base}
	h := &reworkHarness{store: base, intents: newRunIntentFakeStore(), now: time.Unix(1_000, 0).UTC()}
	h.svc = outcome.New(store, func() time.Time { return h.now }).
		WithPlanning(intelligencetest.New(), &routingInventoryFake{candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessCodex, "")}}).
		WithExecution(&fakeSpawner{readiness: ports.AgentProfileReadiness{Ready: true, Detail: "profile ok"}}, newFakeHeartbeats()).
		WithRunIntents(h.intents)
	h.svc.AdmissionPolicy = testAdmissionPolicy()

	ctx := context.Background()
	view, err := h.svc.Create(ctx, validCreateInput())
	if err != nil {
		t.Fatalf("create outcome: %v", err)
	}
	h.outcomeID, h.contract = view.Outcome.ID, view.Current
	planView, err := h.svc.ProposePlan(ctx, h.outcomeID, 1)
	if err != nil {
		t.Fatalf("propose plan: %v", err)
	}
	if _, err := h.svc.ApprovePlan(ctx, h.outcomeID, outcome.ApprovePlanInput{
		PlanRevisionID: planView.Plan.ID, ExpectedContractRevision: 1,
	}); err != nil {
		t.Fatalf("approve plan: %v", err)
	}
	h.plan = planView.Plan
	rememberFirstWorkUnit(planView.Plan)
	return h, store
}

// TestHaltRunForCorrection_AnOldCorrectionReplayCannotStopNewlyAuthorizedWork
// is the replay fence. A correction's halt is idempotent on its decision, so
// retrying the acceptance request re-enters it — and the durable store resolves
// a repeated request key before it checks generations, handing back the old
// cancellation. Applying that to whatever is running now would let a decision
// the owner has already moved past cancel the work they authorized after it.
func TestHaltRunForCorrection_AnOldCorrectionReplayCannotStopNewlyAuthorizedWork(t *testing.T) {
	h := newReworkHarness(t)
	unit := h.plan.WorkUnits[0].ID

	h.authorize(t, "rk-start-1")
	h.runToProvedResult(t, "first")
	h.now = h.now.Add(time.Hour)
	h.requestRework(t, "redo it", domain.ReentryTargetWorkUnit, string(unit), "rk-rework-1")

	// The owner authorizes the revised work and the daemon admits it.
	h.now = h.now.Add(time.Hour)
	h.authorize(t, "rk-start-2")
	revised := h.admitAttempt(t, "revised")
	authorized := h.currentIntent(t)
	if authorized.Desired != domain.RunIntentRunning {
		t.Fatalf("intent = %+v, want the revised run authorized", authorized)
	}

	// The original acceptance request arrives again — a client retry, or a
	// reconnect replaying the same key.
	h.now = h.now.Add(time.Hour)
	h.requestRework(t, "redo it", domain.ReentryTargetWorkUnit, string(unit), "rk-rework-1")

	if status := h.attemptStatus(t, revised.ID); status == domain.AttemptCancelled {
		t.Fatal("replaying an old correction cancelled the Attempt authorized after it")
	}
	if current := h.currentIntent(t); current.Generation != authorized.Generation ||
		current.Desired != domain.RunIntentRunning {
		t.Fatalf("intent = %+v, want the newer authorization untouched at generation %d",
			current, authorized.Generation)
	}
}

// TestHaltRunForCorrection_RetryFinishesACommittedCancellationsStopEffect
// covers the window the method's own contract promises to close: the intent
// append committed and the stop it implies did not. Retrying used to exit
// immediately, because the intent was already cancelled and nothing checked
// whether its effect had happened — leaving an Attempt running under an
// authorization that says it was cancelled.
func TestHaltRunForCorrection_RetryFinishesACommittedCancellationsStopEffect(t *testing.T) {
	h, store := newHaltHarness(t)
	ctx := context.Background()

	h.authorize(t, "rk-start-1")
	running := h.admitAttempt(t, "first")

	store.failNext = true
	if err := h.svc.HaltRunForCorrection(ctx, h.outcomeID, "acc-rework-1"); err == nil {
		t.Fatal("the injected failure did not surface")
	}
	// The cancellation is durable, so the ledger already says the run is over.
	if intent := h.currentIntent(t); intent.Desired != domain.RunIntentCancelled {
		t.Fatalf("intent = %+v, want cancelled", intent)
	}
	if status := h.attemptStatus(t, running.ID); status == domain.AttemptCancelled {
		t.Fatalf("attempt %s was already stopped; the failure window did not open", running.ID)
	}

	// The retry has to finish what the committed cancellation promised.
	if err := h.svc.HaltRunForCorrection(ctx, h.outcomeID, "acc-rework-1"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if status := h.attemptStatus(t, running.ID); status != domain.AttemptCancelled {
		t.Fatalf("attempt status = %q after retry, want cancelled: a cancelled run must not keep working", status)
	}
}

// TestReconcileRunIntents_StopsWorkStillRunningUnderACancelledAuthorization is
// the restart half of the same window. Reconciliation used to acknowledge only
// cancellations whose work had already ended, so a process that died between
// the append and the stop left the Attempt running forever.
func TestReconcileRunIntents_StopsWorkStillRunningUnderACancelledAuthorization(t *testing.T) {
	h, store := newHaltHarness(t)
	ctx := context.Background()

	h.authorize(t, "rk-start-1")
	running := h.admitAttempt(t, "first")

	store.failNext = true
	if err := h.svc.HaltRunForCorrection(ctx, h.outcomeID, "acc-rework-1"); err == nil {
		t.Fatal("the injected failure did not surface")
	}
	if status := h.attemptStatus(t, running.ID); status == domain.AttemptCancelled {
		t.Fatal("the failure window did not open")
	}

	// A restarted daemon reconciles durable facts and finds a cancelled
	// authorization with work still in flight.
	if err := h.svc.ReconcileRunIntents(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if status := h.attemptStatus(t, running.ID); status != domain.AttemptCancelled {
		t.Fatalf("attempt status = %q after reconciliation, want cancelled", status)
	}
	if intent := h.currentIntent(t); intent.AcknowledgedAt == nil {
		t.Fatal("the cancellation was carried out but never acknowledged")
	}
}

// TestCommandRun_RefusesStartWhileTheCorrectionNamesThePlanOrContract closes
// the gap between the Mission's offered moves and what the daemon accepts.
// Refusing Start only in the projection puts an authority boundary in a client,
// which is the exact defect this branch exists to remove.
func TestCommandRun_RefusesStartWhileTheCorrectionNamesThePlanOrContract(t *testing.T) {
	cases := []struct {
		name     string
		target   domain.ReentryTargetType
		targetID func(*reworkHarness) string
		wantWhy  string
	}{
		{
			name:     "plan",
			target:   domain.ReentryTargetPlan,
			targetID: func(h *reworkHarness) string { return string(h.plan.ID) },
			wantWhy:  outcome.ReasonPlanRevisionRequired,
		},
		{
			name:     "contract",
			target:   domain.ReentryTargetContract,
			targetID: func(h *reworkHarness) string { return string(h.contract.ID) },
			wantWhy:  outcome.ReasonContractRevisionRequired,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newReworkHarness(t)
			h.authorize(t, "rk-start-1")
			h.runToProvedResult(t, "first")
			h.now = h.now.Add(time.Hour)
			h.requestRework(t, "wrong shape entirely", tc.target, tc.targetID(h), "rk-rework-1")

			// The projection already refuses; the command must refuse too.
			state := h.runState(t)
			start := runActionFor(state, outcome.RunActionStart)
			if start.Available || start.Reason != tc.wantWhy {
				t.Fatalf("projected start = %+v, want refused with %s", start, tc.wantWhy)
			}
			_, err := h.svc.CommandRun(context.Background(), h.outcomeID, outcome.RunCommandInput{
				Command: domain.RunCommandStart, PlanRevisionID: h.plan.ID,
				ExpectedContractRevision: h.contract.Number, RequestKey: "rk-start-bypass",
			})
			if code := requireAPICode(t, err); code != outcome.CodeCorrectionRevisionRequired {
				t.Fatalf("start = %v, want %s", err, outcome.CodeCorrectionRevisionRequired)
			}
			// Nothing was admitted, and no authorization was recorded.
			if intent := h.currentIntent(t); intent.Desired == domain.RunIntentRunning {
				t.Fatalf("intent = %+v, want the rejected Start to have authorized nothing", intent)
			}
		})
	}
}

// TestStartAttempt_RefusesWhileTheCorrectionNamesThePlan keeps the direct
// per-Attempt path on the same policy. A boundary only the run-command route
// honours is a boundary with a way around it.
//
// This harness has no run-intent storage, which is where the correction gate is
// the deciding refusal: with durable run intent wired, a correction has already
// cancelled the authorization and admission refuses on that instead. Both are
// closed; only this one needs the new gate.
func TestStartAttempt_RefusesWhileTheCorrectionNamesThePlan(t *testing.T) {
	h := newClassificationHarness(t)
	ctx := context.Background()
	criterion := h.criterion(t)
	h.proveCriterion(t, criterion, "pass")
	if err := h.svc.ReconcileAttemptOutcomes(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if _, err := h.svc.DecideAcceptance(ctx, h.outcomeID, outcome.DecideAcceptanceInput{
		ExpectedContractRevision: h.contract.Number, ContractRevisionID: h.contract.ID,
		Kind: domain.AcceptanceRequestRework, Summary: "the Plan is wrong",
		ResourceDisposition: domain.ResourceDispositionRetain,
		ReentryTargetType:   domain.ReentryTargetPlan, ReentryTargetID: string(h.plan.ID),
		RequestKey: "acc-rework-plan",
	}); err != nil {
		t.Fatalf("request rework: %v", err)
	}

	_, err := h.svc.StartAttempt(ctx, h.outcomeID, outcome.StartAttemptInput{
		PlanRevisionID: h.plan.ID, WorkUnitID: h.plan.WorkUnits[0].ID, RequestKey: "direct-bypass",
	})
	if code := requireAPICode(t, err); code != outcome.CodeCorrectionRevisionRequired {
		t.Fatalf("direct start = %v, want %s", err, outcome.CodeCorrectionRevisionRequired)
	}
}

// TestStartAttempt_ACancelledAuthorizationAlreadyRefusesTheDirectPath records
// which refusal a wired daemon actually gives, so the two are not confused.
func TestStartAttempt_ACancelledAuthorizationAlreadyRefusesTheDirectPath(t *testing.T) {
	h := newReworkHarness(t)
	h.authorize(t, "rk-start-1")
	h.runToProvedResult(t, "first")
	h.now = h.now.Add(time.Hour)
	h.requestRework(t, "the Plan is wrong", domain.ReentryTargetPlan, string(h.plan.ID), "rk-rework-1")

	_, err := h.svc.StartAttempt(context.Background(), h.outcomeID, outcome.StartAttemptInput{
		PlanRevisionID: h.plan.ID, WorkUnitID: h.plan.WorkUnits[0].ID, RequestKey: "direct-bypass",
	})
	if code := requireAPICode(t, err); code != outcome.CodeRunActionUnavailable {
		t.Fatalf("direct start = %v, want the cancelled authorization to refuse it", err)
	}
}
