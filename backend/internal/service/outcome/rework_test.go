package outcome_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/intelligence/intelligencetest"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

// reworkHarness drives the whole owner loop on one Outcome: authorize, let the
// daemon admit the unit, prove it, then have the owner reject the result.
//
// It differs from classificationHarness in one way that matters: the Attempt is
// admitted by ContinueAuthorizedRuns under durable run intent, exactly as the
// Mission Start path does, rather than by a direct StartAttempt. Whether the
// loop can continue afterwards depends on that authorization.
type reworkHarness struct {
	svc       *outcome.Service
	store     *receiptFakeStore
	intents   *runIntentFakeStore
	outcomeID domain.OutcomeID
	plan      domain.PlanRevision
	contract  domain.ContractRevision
	now       time.Time
}

func newReworkHarness(t *testing.T) *reworkHarness {
	t.Helper()
	store := newReceiptFakeStore()
	h := &reworkHarness{store: store, intents: newRunIntentFakeStore(), now: time.Unix(1_000, 0).UTC()}
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
	return h
}

// authorize records the owner's Start, the way the Mission does.
func (h *reworkHarness) authorize(t *testing.T, key string) {
	t.Helper()
	if _, err := h.svc.CommandRun(context.Background(), h.outcomeID, outcome.RunCommandInput{
		Command: domain.RunCommandStart, PlanRevisionID: h.plan.ID,
		ExpectedContractRevision: h.contract.Number, RequestKey: key,
	}); err != nil {
		t.Fatalf("authorize (%s): %v", key, err)
	}
}

// admitAttempt lets the daemon admit the next unit under the current
// authorization and leaves that Attempt running.
func (h *reworkHarness) admitAttempt(t *testing.T, key string) domain.Attempt {
	t.Helper()
	before := h.attemptIDs(t)
	if err := h.svc.ContinueAuthorizedRuns(context.Background()); err != nil {
		t.Fatalf("continue authorized runs (%s): %v", key, err)
	}
	return h.newAttemptSince(t, before)
}

// attemptStatus re-reads one Attempt's durable status.
func (h *reworkHarness) attemptStatus(t *testing.T, id domain.AttemptID) domain.AttemptStatus {
	t.Helper()
	attempt, ok, err := h.store.GetAttempt(context.Background(), h.outcomeID, id)
	if err != nil || !ok {
		t.Fatalf("read attempt %s: ok=%v err=%v", id, ok, err)
	}
	return attempt.Status
}

// currentIntent re-reads the authorization in force.
func (h *reworkHarness) currentIntent(t *testing.T) domain.OutcomeRunIntent {
	t.Helper()
	intent, found, err := h.intents.CurrentRunIntent(context.Background(), h.outcomeID)
	if err != nil || !found {
		t.Fatalf("read intent: found=%v err=%v", found, err)
	}
	return intent
}

// runToProvedResult lets the daemon admit the next unit under the current
// authorization, ends that Attempt with a retained artifact, and records the
// proof the criterion needs. It returns the Attempt the daemon admitted.
func (h *reworkHarness) runToProvedResult(t *testing.T, key string) domain.Attempt {
	t.Helper()
	ctx := context.Background()
	attempt := h.admitAttempt(t, key)

	if _, err := h.store.TransitionAttemptStatus(ctx, h.outcomeID, attempt.ID,
		attempt.Status, domain.AttemptReconciled, h.now); err != nil {
		t.Fatalf("end attempt (%s): %v", key, err)
	}
	attempt.Status = domain.AttemptReconciled
	receipt := retainedReceiptFor(attempt)
	if err := h.store.SaveAttemptReceipt(ctx, receipt); err != nil {
		t.Fatalf("save receipt (%s): %v", key, err)
	}
	h.prove(t, attempt, receipt, key)
	if err := h.svc.ReconcileAttemptOutcomes(ctx); err != nil {
		t.Fatalf("reconcile (%s): %v", key, err)
	}
	return attempt
}

// prove records supporting Evidence and a passing Verification for every
// criterion, bound to this exact Attempt and artifact version.
func (h *reworkHarness) prove(t *testing.T, attempt domain.Attempt, receipt domain.AttemptReceipt, key string) {
	t.Helper()
	ctx := context.Background()
	for i, criterion := range h.contract.Criteria {
		evidenceKey := "ev-" + key + "-" + string(criterion.ID)
		if _, err := h.svc.RecordEvidence(ctx, h.outcomeID, outcome.RecordEvidenceInput{
			ExpectedContractRevision: h.contract.Number, ContractRevisionID: h.contract.ID,
			CriterionID: criterion.ID, SubjectType: domain.ProofSubjectAttempt,
			SubjectID: string(attempt.ID), SubjectRevision: receipt.ArtifactVersion,
			Kind: domain.EvidenceSupporting, SourceType: domain.EvidenceSourceDeterministicCheck,
			SourceRef: "go test ./...", ProducerType: domain.EvidenceProducerTool, ProducerRef: string(attempt.ID),
			Summary: "exited 0", ContentDigest: strings.Repeat("b", 64), RequestKey: evidenceKey,
		}); err != nil {
			t.Fatalf("record evidence %d (%s): %v", i, key, err)
		}
		item, found, err := h.store.FindEvidenceItemByRequestKey(ctx, evidenceKey)
		if err != nil || !found {
			t.Fatalf("read evidence %d (%s): found=%v err=%v", i, key, found, err)
		}
		if _, err := h.svc.RecordVerification(ctx, h.outcomeID, outcome.RecordVerificationInput{
			ExpectedContractRevision: h.contract.Number, ContractRevisionID: h.contract.ID,
			CriterionID: criterion.ID, SubjectType: domain.ProofSubjectAttempt,
			SubjectID: string(attempt.ID), SubjectRevision: receipt.ArtifactVersion,
			EvidenceItemIDs: []domain.EvidenceItemID{item.ID}, Method: "go test ./...",
			IndependenceClass: domain.VerificationDeterministic, Result: domain.VerificationPassed,
			ProducerRef: string(attempt.ID), VerifierRef: "kennel-governed-check/test",
			RequestKey: "ver-" + key + "-" + string(criterion.ID),
		}); err != nil {
			t.Fatalf("record verification %d (%s): %v", i, key, err)
		}
	}
}

// requestRework is the owner rejecting the recorded result and saying what has
// to change. target names the lineage seam their feedback points at.
func (h *reworkHarness) requestRework(t *testing.T, feedback string, target domain.ReentryTargetType, targetID, key string) outcome.ProofView {
	t.Helper()
	proof, err := h.svc.DecideAcceptance(context.Background(), h.outcomeID, outcome.DecideAcceptanceInput{
		ExpectedContractRevision: h.contract.Number, ContractRevisionID: h.contract.ID,
		Kind: domain.AcceptanceRequestRework, Summary: feedback,
		ResourceDisposition: domain.ResourceDispositionRetain,
		ReentryTargetType:   target, ReentryTargetID: targetID, RequestKey: key,
	})
	if err != nil {
		t.Fatalf("request rework (%s): %v", key, err)
	}
	return proof
}

func (h *reworkHarness) attemptIDs(t *testing.T) map[domain.AttemptID]bool {
	t.Helper()
	attempts, err := h.store.ListAttempts(context.Background(), h.outcomeID)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	seen := make(map[domain.AttemptID]bool, len(attempts))
	for _, attempt := range attempts {
		seen[attempt.ID] = true
	}
	return seen
}

func (h *reworkHarness) newAttemptSince(t *testing.T, before map[domain.AttemptID]bool) domain.Attempt {
	t.Helper()
	attempts, err := h.store.ListAttempts(context.Background(), h.outcomeID)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	var fresh []domain.Attempt
	for _, attempt := range attempts {
		if !before[attempt.ID] {
			fresh = append(fresh, attempt)
		}
	}
	if len(fresh) != 1 {
		t.Fatalf("the daemon admitted %d new Attempts, want exactly one", len(fresh))
	}
	return fresh[0]
}

func (h *reworkHarness) runState(t *testing.T) outcome.RunStateView {
	t.Helper()
	view, err := h.svc.GetRunState(context.Background(), h.outcomeID)
	if err != nil {
		t.Fatalf("run state: %v", err)
	}
	return view
}

func (h *reworkHarness) proof(t *testing.T) outcome.ProofView {
	t.Helper()
	proof, err := h.svc.GetProof(context.Background(), h.outcomeID)
	if err != nil {
		t.Fatalf("proof: %v", err)
	}
	return proof
}

// TestRework_OwnerFeedbackReachesRevisedAuthorizedExecutionAndFreshProof is the
// whole loop the checkpoint left open: the owner rejects a proved result, the
// revised work is authorized and executed as a distinct Attempt, and fresh
// proof returns the Outcome to review — without erasing what came before.
func TestRework_OwnerFeedbackReachesRevisedAuthorizedExecutionAndFreshProof(t *testing.T) {
	h := newReworkHarness(t)
	unit := h.plan.WorkUnits[0].ID

	h.authorize(t, "rk-start-1")
	first := h.runToProvedResult(t, "first")
	if got := h.proof(t).Status; got != outcome.ProofStatusReadyForAcceptance {
		t.Fatalf("proof after the first run = %q, want ready_for_acceptance", got)
	}

	// The owner rejects it and says the WorkUnit has to be done again.
	h.now = h.now.Add(time.Hour)
	rejected := h.requestRework(t, "The summary is wrong; redo it.", domain.ReentryTargetWorkUnit, string(unit), "rk-rework-1")
	if rejected.Status != outcome.ProofStatusReworkRequired {
		t.Fatalf("proof after rework = %q, want rework_required", rejected.Status)
	}

	// Rejecting a result ends the authorization that produced it. Leaving it in
	// force would let the daemon keep admitting against work the owner just
	// rejected, and would leave Start refused as already-authorized.
	state := h.runState(t)
	if state.Intent != nil && state.Intent.Desired == string(domain.RunIntentRunning) {
		t.Fatalf("intent = %+v, want the rejected run no longer authorized", state.Intent)
	}
	if start := runActionFor(state, outcome.RunActionStart); !start.Available {
		t.Fatalf("start = %+v, want the owner able to authorize the revised work", start)
	}

	// The revised work is a distinct Attempt under a fresh authorization, not a
	// replay of the one the owner rejected.
	h.now = h.now.Add(time.Hour)
	h.authorize(t, "rk-start-2")
	second := h.runToProvedResult(t, "second")
	if second.ID == first.ID {
		t.Fatalf("the revised run reused Attempt %s instead of admitting a fresh one", first.ID)
	}
	if second.WorkUnitID != unit {
		t.Fatalf("revised Attempt ran %s, want the corrected unit %s", second.WorkUnitID, unit)
	}

	// Fresh proof recorded after the correction returns the Outcome to review.
	final := h.proof(t)
	if final.Status != outcome.ProofStatusReadyForAcceptance {
		t.Fatalf("proof after the revised run = %q, want ready_for_acceptance", final.Status)
	}

	// History is preserved, not overwritten: the rejected Attempt, the decision
	// that rejected it and the recorded feedback all remain readable.
	if !h.attemptIDs(t)[first.ID] {
		t.Fatal("the rejected Attempt was removed from the record")
	}
	if len(final.Decisions) != 1 || final.Decisions[0].Kind != domain.AcceptanceRequestRework {
		t.Fatalf("decisions = %+v, want the rework decision retained", final.Decisions)
	}
	if len(final.Corrections) != 1 || final.Corrections[0].Feedback != "The summary is wrong; redo it." {
		t.Fatalf("corrections = %+v, want the owner's feedback retained", final.Corrections)
	}
	// And acceptance is still the owner's alone: fresh proof supports it, it
	// does not perform it.
	if final.Status == outcome.ProofStatusAccepted {
		t.Fatal("a revised run accepted the Outcome by itself")
	}
}

// TestRework_TheOfferedNextMoveFollowsTheCorrectionTarget keeps the feedback
// and the offered move together. A correction naming the Plan is not satisfied
// by running that same approved Plan again.
func TestRework_TheOfferedNextMoveFollowsTheCorrectionTarget(t *testing.T) {
	cases := []struct {
		name        string
		target      domain.ReentryTargetType
		targetID    func(*reworkHarness) string
		wantStart   bool
		wantWhy     string
		wantPropose bool
	}{
		{
			name:   "a WorkUnit correction is satisfied by running the approved Plan again",
			target: domain.ReentryTargetWorkUnit,
			targetID: func(h *reworkHarness) string {
				return string(h.plan.WorkUnits[0].ID)
			},
			wantStart: true,
		},
		{
			name:        "a Plan correction asks for a different Plan",
			target:      domain.ReentryTargetPlan,
			targetID:    func(h *reworkHarness) string { return string(h.plan.ID) },
			wantWhy:     outcome.ReasonPlanRevisionRequired,
			wantPropose: true,
		},
		{
			name:     "a Contract correction stops everything below it",
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
			h.requestRework(t, "not what I asked for", tc.target, tc.targetID(h), "rk-rework-1")

			state := h.runState(t)
			if state.AttentionReason != outcome.ReasonReworkRequired {
				t.Fatalf("attention = %q, want %s", state.AttentionReason, outcome.ReasonReworkRequired)
			}
			// The blocker carries the owner's own words and what they named, so
			// the Mission can say more than "rework required".
			if state.Blocker == nil || state.Blocker.Detail["feedback"] != "not what I asked for" ||
				state.Blocker.Detail["targetType"] != string(tc.target) {
				t.Fatalf("blocker detail = %+v, want the recorded correction", state.Blocker)
			}
			start := runActionFor(state, outcome.RunActionStart)
			if start.Available != tc.wantStart {
				t.Fatalf("start available = %v (reason %q), want %v", start.Available, start.Reason, tc.wantStart)
			}
			if !tc.wantStart && start.Reason != tc.wantWhy {
				t.Fatalf("start refusal = %q, want %q", start.Reason, tc.wantWhy)
			}
			if propose := runActionFor(state, outcome.RunActionProposePlan); propose.Available != tc.wantPropose {
				t.Fatalf("propose_plan available = %v (reason %q), want %v",
					propose.Available, propose.Reason, tc.wantPropose)
			}
		})
	}
}

// TestRework_RetryingACorrectionFinishesItRatherThanRepeatingIt covers the
// crash window between the durable decision and the halt it implies.
func TestRework_RetryingACorrectionFinishesItRatherThanRepeatingIt(t *testing.T) {
	h := newReworkHarness(t)
	h.authorize(t, "rk-start-1")
	h.runToProvedResult(t, "first")
	h.now = h.now.Add(time.Hour)

	h.requestRework(t, "redo it", domain.ReentryTargetWorkUnit, string(h.plan.WorkUnits[0].ID), "rk-rework-1")
	afterFirst, _, err := h.intents.CurrentRunIntent(context.Background(), h.outcomeID)
	if err != nil {
		t.Fatalf("read intent: %v", err)
	}

	// The same request key arriving again is one owner decision, not two.
	h.requestRework(t, "redo it", domain.ReentryTargetWorkUnit, string(h.plan.WorkUnits[0].ID), "rk-rework-1")
	afterReplay, _, err := h.intents.CurrentRunIntent(context.Background(), h.outcomeID)
	if err != nil {
		t.Fatalf("read intent after replay: %v", err)
	}
	if afterReplay.Generation != afterFirst.Generation {
		t.Fatalf("generation moved %d -> %d on replay; the halt was applied twice",
			afterFirst.Generation, afterReplay.Generation)
	}
	if len(h.proof(t).Decisions) != 1 {
		t.Fatalf("decisions = %d, want the replayed correction recorded once", len(h.proof(t).Decisions))
	}
}
