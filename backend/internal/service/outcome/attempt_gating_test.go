package outcome_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/intelligence/intelligencetest"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

// newGatedContributorHarness builds a parent with two ordered contributors and
// returns an approved plan for the DOWNSTREAM one, so the only thing standing
// between it and execution is the dependency gate.
func newGatedContributorHarness(t *testing.T) (*outcome.Service, *attemptFakeStore, domain.OutcomeID, domain.OutcomeID, domain.PlanRevisionID) {
	t.Helper()
	store := newAttemptFakeStore()
	spawner := &fakeSpawner{readiness: ports.AgentProfileReadiness{Ready: true, Detail: "profile ok"}}
	svc := outcome.New(store, nil).
		WithPlanning(intelligencetest.New(), &routingInventoryFake{candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessCodex, "")}}).
		WithExecution(spawner, newFakeHeartbeats())
	svc.AdmissionPolicy = testAdmissionPolicy()

	ctx := context.Background()
	in := validCreateInput()
	in.SuccessCriteria = []string{"The first slice is true.", "The second slice is true."}
	parent, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	store.mu.Lock()
	revs := store.revs[parent.Outcome.ID]
	revs[0].AuthorityCeiling = domain.ProposedAuthority{ReadWorkspace: true, WriteWorkspace: true, ExecuteLocal: true}
	criteria := revs[0].Criteria
	store.revs[parent.Outcome.ID] = revs
	store.mu.Unlock()

	proposed, err := svc.ProposeDecomposition(ctx, parent.Outcome.ID, outcome.ProposeDecompositionInput{
		ExpectedContractRevision: 1,
		Rationale:                "The second slice builds on the first.",
		Contributors: []outcome.ProposedContributionInput{
			contributionOffer("c1", criteria[0].ID),
			contributionOffer("c2", criteria[1].ID),
		},
		Dependencies: []outcome.ContributionDependencyInput{{FromRef: "c1", ToRef: "c2"}},
	})
	if err != nil {
		t.Fatalf("propose decomposition: %v", err)
	}
	authorized, err := svc.AuthorizeDecomposition(ctx, parent.Outcome.ID, proposed.Decomposition.ID)
	if err != nil {
		t.Fatalf("authorize decomposition: %v", err)
	}

	upstream := contributorOutcomeID(t, authorized.Decomposition, "c1")
	downstream := contributorOutcomeID(t, authorized.Decomposition, "c2")

	plan, err := svc.ProposePlan(ctx, downstream, 1)
	if err != nil {
		t.Fatalf("propose downstream plan: %v", err)
	}
	if _, err := svc.ApprovePlan(ctx, downstream, outcome.ApprovePlanInput{
		PlanRevisionID: plan.Plan.ID, ExpectedContractRevision: 1,
	}); err != nil {
		t.Fatalf("approve downstream plan: %v", err)
	}
	return svc, store, upstream, downstream, plan.Plan.ID
}

// The gate is a pre-durable refusal like every other start gate: nothing is
// written, and no provider session is spawned.
func TestStartAttemptRefusesABlockedContributorWithoutWriting(t *testing.T) {
	svc, store, _, downstream, planID := newGatedContributorHarness(t)
	ctx := context.Background()

	_, err := svc.StartAttempt(ctx, downstream, startInput(planID))
	if err == nil {
		t.Fatal("a contributor whose upstream is unaccepted must not start")
	}
	if got := requireAPICode(t, err); got != outcome.CodeContributionBlocked {
		t.Fatalf("code = %q, want %q", got, outcome.CodeContributionBlocked)
	}
	// The refusal must be actionable without opening a transcript.
	if !strings.Contains(err.Error(), "Accept the upstream contribution") {
		t.Fatalf("refusal %q must name the ways forward", err.Error())
	}

	attempts, listErr := store.ListAttempts(ctx, downstream)
	if listErr != nil {
		t.Fatalf("list attempts: %v", listErr)
	}
	if len(attempts) != 0 {
		t.Fatalf("a blocked start must leave no durable attempt, got %d", len(attempts))
	}
}

// The upstream itself is never gated by its own dependents.
func TestStartAttemptAdmitsTheUpstreamContributor(t *testing.T) {
	svc, _, upstream, _, _ := newGatedContributorHarness(t)
	ctx := context.Background()

	plan, err := svc.ProposePlan(ctx, upstream, 1)
	if err != nil {
		t.Fatalf("propose upstream plan: %v", err)
	}
	if _, err := svc.ApprovePlan(ctx, upstream, outcome.ApprovePlanInput{
		PlanRevisionID: plan.Plan.ID, ExpectedContractRevision: 1,
	}); err != nil {
		t.Fatalf("approve upstream plan: %v", err)
	}
	view, err := svc.StartAttempt(ctx, upstream, outcome.StartAttemptInput{
		PlanRevisionID: plan.Plan.ID, WorkUnitID: firstWorkUnitOfPlan[plan.Plan.ID], RequestKey: "req-upstream-start",
	})
	if err != nil {
		t.Fatalf("the upstream contributor must be admitted: %v", err)
	}
	if view.Attempt.ID == "" {
		t.Fatal("admission must produce a durable attempt")
	}
}

// An explicit owner waiver is the second way past the gate, and it is the
// only one that does not require the upstream to be accepted first.
func TestStartAttemptAdmitsAfterAnExplicitWaiver(t *testing.T) {
	svc, _, upstream, downstream, planID := newGatedContributorHarness(t)
	ctx := context.Background()

	child, err := svc.Get(ctx, downstream)
	if err != nil {
		t.Fatalf("read downstream: %v", err)
	}
	parentID := child.Outcome.ParentID
	if parentID.IsZero() {
		t.Fatal("the downstream contributor must name its parent")
	}
	if _, err := svc.WaiveContributionDependency(ctx, parentID, outcome.WaiveDependencyInput{
		FromRef: "c1", ToRef: "c2",
		Reason: "The interface the second slice needs is already frozen on main.",
	}); err != nil {
		t.Fatalf("waive: %v", err)
	}

	view, err := svc.StartAttempt(ctx, downstream, startInput(planID))
	if err != nil {
		t.Fatalf("a waived dependency must admit the contributor: %v", err)
	}
	if view.Attempt.OutcomeID != downstream {
		t.Fatalf("attempt = %+v, want one for the downstream contributor", view.Attempt)
	}
	_ = upstream
}

// A Project-level Outcome with no parent is never gated, so composition
// cannot regress ordinary single-Outcome execution.
func TestStartAttemptOnAnUngatedOutcomeIsUnchanged(t *testing.T) {
	svc, _, _, _, outcomeID, planID := newAttemptHarness(t)
	ctx := context.Background()

	view, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
	if err != nil {
		t.Fatalf("an Outcome with no parent must start exactly as before: %v", err)
	}
	if view.Attempt.ID == "" {
		t.Fatal("admission must produce a durable attempt")
	}
}
