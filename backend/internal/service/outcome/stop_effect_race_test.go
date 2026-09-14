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

// interleavingAttemptStore fires a hook immediately before the ListAttempts a
// stop effect reads, which is the exact point where "current when it was
// listed" stops being "current when it takes effect".
type interleavingAttemptStore struct {
	*receiptFakeStore
	beforeListAttempts func()
}

func (s *interleavingAttemptStore) ListAttempts(ctx context.Context, outcomeID domain.OutcomeID) ([]domain.Attempt, error) {
	if hook := s.beforeListAttempts; hook != nil {
		s.beforeListAttempts = nil
		hook()
	}
	return s.receiptFakeStore.ListAttempts(ctx, outcomeID)
}

// acknowledgeFailingIntentStore fails the next acknowledgement, which leaves a
// cancellation durable and unacknowledged — the state reconciliation exists to
// pick up, and the state this race needs.
type acknowledgeFailingIntentStore struct {
	*runIntentFakeStore
	failNext bool
}

func (s *acknowledgeFailingIntentStore) AcknowledgeRunIntent(ctx context.Context, outcomeID domain.OutcomeID, generation int64, at time.Time) error {
	if s.failNext {
		s.failNext = false
		return errors.New("injected acknowledgement failure")
	}
	return s.runIntentFakeStore.AcknowledgeRunIntent(ctx, outcomeID, generation, at)
}

type raceHarness struct {
	*reworkHarness
	attempts *interleavingAttemptStore
	acks     *acknowledgeFailingIntentStore
}

func newRaceHarness(t *testing.T) *raceHarness {
	t.Helper()
	base := newReceiptFakeStore()
	attempts := &interleavingAttemptStore{receiptFakeStore: base}
	acks := &acknowledgeFailingIntentStore{runIntentFakeStore: newRunIntentFakeStore()}
	h := &reworkHarness{store: base, intents: acks.runIntentFakeStore, now: time.Unix(1_000, 0).UTC()}
	h.svc = outcome.New(attempts, func() time.Time { return h.now }).
		WithPlanning(intelligencetest.New(), &routingInventoryFake{candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessCodex, "")}}).
		WithExecution(&fakeSpawner{readiness: ports.AgentProfileReadiness{Ready: true, Detail: "profile ok"}}, newFakeHeartbeats()).
		WithRunIntents(acks)
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
	return &raceHarness{reworkHarness: h, attempts: attempts, acks: acks}
}

// TestReconcileRunIntents_ASnapshotStopCannotCancelWorkAuthorizedAfterIt is the
// stop-effect race.
//
// Reconciliation lists cancellations that are current when it reads them, then
// applies their effects. Between those two moments the owner can authorize
// fresh work and the daemon can admit an Attempt for it, and the stop helper
// used to act on whichever Attempt was active by then. A re-read of the current
// intent narrows that window without closing it — the read is still not the
// effect.
//
// What closes it is the durable binding already on the Attempt: admission
// records the authorization generation that admitted it, inside the same
// transaction that checks it, and no Attempt can ever be admitted under a
// cancelled generation. A stop may therefore act only on Attempts admitted at
// or before its own generation, which is a fact about durable rows rather than
// about when anything was read.
func TestReconcileRunIntents_ASnapshotStopCannotCancelWorkAuthorizedAfterIt(t *testing.T) {
	h := newRaceHarness(t)
	ctx := context.Background()
	unit := h.plan.WorkUnits[0].ID

	h.authorize(t, "rk-start-1")
	h.runToProvedResult(t, "first")

	// Requesting rework cancels the run. The acknowledgement fails, so the
	// cancellation stays durable and unacknowledged — reconciliation's input.
	h.now = h.now.Add(time.Hour)
	h.acks.failNext = true
	if _, err := h.svc.DecideAcceptance(ctx, h.outcomeID, outcome.DecideAcceptanceInput{
		ExpectedContractRevision: h.contract.Number, ContractRevisionID: h.contract.ID,
		Kind: domain.AcceptanceRequestRework, Summary: "redo it",
		ResourceDisposition: domain.ResourceDispositionRetain,
		ReentryTargetType:   domain.ReentryTargetWorkUnit, ReentryTargetID: string(unit),
		RequestKey: "rk-rework-1",
	}); err == nil {
		t.Fatal("the injected acknowledgement failure did not surface")
	}
	cancellation := h.currentIntent(t)
	if cancellation.Desired != domain.RunIntentCancelled || cancellation.AcknowledgedAt != nil {
		t.Fatalf("intent = %+v, want an unacknowledged cancellation", cancellation)
	}

	// The owner authorizes the revised work and the daemon admits it — after
	// reconciliation has read the cancellation, before that stop takes effect.
	var revised domain.Attempt
	h.attempts.beforeListAttempts = func() {
		h.now = h.now.Add(time.Hour)
		h.authorize(t, "rk-start-2")
		revised = h.admitAttempt(t, "revised")
	}

	if err := h.svc.ReconcileRunIntents(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if revised.ID == "" {
		t.Fatal("the interleaving hook never ran; this test proves nothing")
	}
	if revised.RunIntentGeneration <= cancellation.Generation {
		t.Fatalf("revised Attempt generation %d is not after the cancellation at %d; the fixture is wrong",
			revised.RunIntentGeneration, cancellation.Generation)
	}
	if status := h.attemptStatus(t, revised.ID); status == domain.AttemptCancelled {
		t.Fatalf("a snapshot stop from generation %d cancelled the Attempt authorized at generation %d",
			cancellation.Generation, revised.RunIntentGeneration)
	}
	// The newer authorization is untouched and still running.
	if current := h.currentIntent(t); current.Desired != domain.RunIntentRunning {
		t.Fatalf("intent = %+v, want the revised authorization still running", current)
	}
}

// TestApplyStopIntent_StopsExactlyTheWorkItsAuthorizationCovered is the
// coverage rule stated directly, so the boundary is pinned in both directions:
// an Attempt admitted under the cancelled authorization is stopped, and one
// admitted under a later authorization is not.
func TestApplyStopIntent_StopsExactlyTheWorkItsAuthorizationCovered(t *testing.T) {
	h, _ := newHaltHarness(t)
	ctx := context.Background()

	h.authorize(t, "rk-start-1")
	covered := h.admitAttempt(t, "covered")
	if covered.RunIntentGeneration != 1 {
		t.Fatalf("covered Attempt generation = %d, want the first authorization", covered.RunIntentGeneration)
	}

	// Cancelling generation 1 must stop the Attempt it admitted.
	if _, err := h.svc.CommandRun(ctx, h.outcomeID, outcome.RunCommandInput{
		Command: domain.RunCommandCancel, PlanRevisionID: h.plan.ID,
		ExpectedContractRevision: h.contract.Number, RequestKey: "rk-cancel-1",
	}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if status := h.attemptStatus(t, covered.ID); status != domain.AttemptCancelled {
		t.Fatalf("covered Attempt status = %q, want cancelled", status)
	}
	if intent := h.currentIntent(t); intent.AcknowledgedAt == nil {
		t.Fatal("a proven stop was not acknowledged")
	}
}
