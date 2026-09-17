package store_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

func TestOutcomeStore_EnsureWorkResponsibilitySpaceIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "mer")

	first, err := s.EnsureWorkResponsibilitySpace(ctx, "mer")
	if err != nil {
		t.Fatalf("ensure space: %v", err)
	}
	if first.Kind != domain.ResponsibilitySpaceWorkProject || first.ProjectID != "mer" {
		t.Fatalf("space = %+v", first)
	}
	second, err := s.EnsureWorkResponsibilitySpace(ctx, "mer")
	if err != nil {
		t.Fatalf("re-ensure space: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("space id changed across calls: %s then %s", first.ID, second.ID)
	}

	// Distinct projects get distinct spaces.
	seedProject(t, s, "other")
	third, err := s.EnsureWorkResponsibilitySpace(ctx, "other")
	if err != nil {
		t.Fatalf("ensure other space: %v", err)
	}
	if third.ID == first.ID {
		t.Fatal("distinct projects must not share a Work space")
	}
}

func TestOutcomeStore_EnsureWorkSpaceRejectsUnknownProject(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if _, err := s.EnsureWorkResponsibilitySpace(ctx, "ghost"); err == nil {
		t.Fatal("space for unknown project must violate the projects foreign key")
	}
}

func focusLedgerContract(spaceID domain.ResponsibilitySpaceID, suffix string) (domain.Outcome, domain.ContractRevision) {
	outcome := domain.Outcome{
		ID:        domain.OutcomeID("out-" + suffix),
		SpaceID:   spaceID,
		Title:     "Local Focus Ledger",
		CreatedAt: time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC),
	}
	first := domain.ContractRevision{
		ID:              domain.ContractRevisionID("cr-" + suffix),
		OutcomeID:       outcome.ID,
		Goal:            "A user can record and review today's protected focus time locally.",
		SuccessCriteria: []string{"Entering positive whole minutes creates one focus block."},
		Review:          "Deterministic checks plus owner walkthrough.",
		Constraints:     []string{"Local only."},
		CreatedAt:       outcome.CreatedAt,
	}
	return outcome, first
}

func TestOutcomeStore_CreateOutcomeWithContractIsAtomic(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "mer")
	space, err := s.EnsureWorkResponsibilitySpace(ctx, "mer")
	if err != nil {
		t.Fatalf("ensure space: %v", err)
	}

	outcome, first := focusLedgerContract(space.ID, "1")
	const requestKey = "req-abc-123"
	if err := s.CreateOutcomeWithContract(ctx, outcome, first, requestKey); err != nil {
		t.Fatalf("create outcome with contract: %v", err)
	}

	got, ok, err := s.GetOutcome(ctx, outcome.ID)
	if err != nil || !ok {
		t.Fatalf("get outcome ok=%v err=%v", ok, err)
	}
	if got.CurrentRevisionNumber != 1 {
		t.Fatalf("pointer = %d, want 1 after first contract", got.CurrentRevisionNumber)
	}
	history, err := s.ListContractRevisions(ctx, outcome.ID)
	if err != nil || len(history) != 1 {
		t.Fatalf("history len=%d err=%v, want exactly one immutable revision", len(history), err)
	}
	if history[0].Number != 1 || history[0].Goal != first.Goal {
		t.Fatalf("revision 1 = %+v", history[0])
	}

	replayed, ok, err := s.FindOutcomeByIdempotencyKey(ctx, requestKey)
	if err != nil || !ok || replayed.ID != outcome.ID {
		t.Fatalf("replay ok=%v id=%s err=%v", ok, replayed.ID, err)
	}
	if _, ok, err := s.FindOutcomeByIdempotencyKey(ctx, "req-never-seen"); err != nil || ok {
		t.Fatalf("unknown key ok=%v err=%v", ok, err)
	}

	// A second create with the same client key is impossible.
	dupOutcome, dupFirst := focusLedgerContract(space.ID, "duplicate")
	if err := s.CreateOutcomeWithContract(ctx, dupOutcome, dupFirst, requestKey); err == nil {
		t.Fatal("duplicate idempotency key must violate the unique index")
	}
	// And nothing partial was written for the rejected duplicate.
	if _, ok, _ := s.GetOutcome(ctx, dupOutcome.ID); ok {
		t.Fatal("rejected duplicate must not persist an outcome row")
	}
}

func TestOutcomeStore_ListOutcomesByProjectKeepsResponsibilityBoundaries(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "mer")
	seedProject(t, s, "other")

	merSpace, err := s.EnsureWorkResponsibilitySpace(ctx, "mer")
	if err != nil {
		t.Fatalf("ensure mer space: %v", err)
	}
	otherSpace, err := s.EnsureWorkResponsibilitySpace(ctx, "other")
	if err != nil {
		t.Fatalf("ensure other space: %v", err)
	}

	first, firstRevision := focusLedgerContract(merSpace.ID, "list-1")
	second, secondRevision := focusLedgerContract(merSpace.ID, "list-2")
	second.Title = "Second Outcome"
	third, thirdRevision := focusLedgerContract(otherSpace.ID, "list-other")
	for _, fixture := range []struct {
		outcome  domain.Outcome
		revision domain.ContractRevision
		key      string
	}{
		{first, firstRevision, "req-list-1"},
		{second, secondRevision, "req-list-2"},
		{third, thirdRevision, "req-list-other"},
	} {
		if err := s.CreateOutcomeWithContract(ctx, fixture.outcome, fixture.revision, fixture.key); err != nil {
			t.Fatalf("create %s: %v", fixture.outcome.ID, err)
		}
	}

	got, err := s.ListOutcomesByProject(ctx, "mer")
	if err != nil {
		t.Fatalf("list mer outcomes: %v", err)
	}
	if len(got) != 2 || got[0].ID != first.ID || got[1].ID != second.ID {
		t.Fatalf("mer outcomes = %+v, want creation order [%s %s]", got, first.ID, second.ID)
	}
	for _, outcome := range got {
		if outcome.SpaceID != merSpace.ID || outcome.CurrentRevisionNumber != 1 {
			t.Fatalf("listed outcome crossed space or lost revision pointer: %+v", outcome)
		}
	}

	empty, err := s.ListOutcomesByProject(ctx, "missing")
	if err != nil || len(empty) != 0 {
		t.Fatalf("missing project list = %+v err=%v, want empty", empty, err)
	}
}

func TestOutcomeStore_AppendContractRevisionHistoryAndConflictRollback(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "mer")
	space, err := s.EnsureWorkResponsibilitySpace(ctx, "mer")
	if err != nil {
		t.Fatalf("ensure space: %v", err)
	}
	outcome, first := focusLedgerContract(space.ID, "h")
	if err := s.CreateOutcomeWithContract(ctx, outcome, first, ""); err != nil {
		t.Fatalf("create outcome with contract: %v", err)
	}

	rev2 := domain.ContractRevision{
		ID:              domain.ContractRevisionID("cr-h-2"),
		OutcomeID:       outcome.ID,
		Goal:            "Revised goal.",
		SuccessCriteria: []string{"Criterion A.", "Criterion B."},
		Review:          "Deterministic checks.",
		NonGoals:        []string{"No timers."},
		Clarification:   "today means local calendar day",
		CreatedAt:       first.CreatedAt.Add(time.Minute),
	}
	number, err := s.AppendContractRevision(ctx, outcome.ID, 1, rev2)
	if err != nil || number != 2 {
		t.Fatalf("append revision number=%d err=%v, want 2", number, err)
	}

	got, _, err := s.GetOutcome(ctx, outcome.ID)
	if err != nil {
		t.Fatalf("reload outcome: %v", err)
	}
	if got.CurrentRevisionNumber != 2 {
		t.Fatalf("pointer = %d, want 2", got.CurrentRevisionNumber)
	}

	history, err := s.ListContractRevisions(ctx, outcome.ID)
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if len(history) != 2 || history[0].Number != 1 || history[1].Number != 2 {
		t.Fatalf("history = %+v", history)
	}
	if history[0].Goal != first.Goal {
		t.Fatal("revision 1 content drifted")
	}
	if len(history[1].SuccessCriteria) != 2 ||
		len(history[1].NonGoals) != 1 || history[1].NonGoals[0] != "No timers." ||
		history[1].Clarification != "today means local calendar day" {
		t.Fatalf("revision 2 optional fields round-trip failed: %+v", history[1])
	}

	// A stale expected pointer conflicts AND persists nothing: the append-only
	// history must be identical after the rejected attempt.
	stale := rev2
	stale.ID = domain.ContractRevisionID("cr-h-stale")
	_, err = s.AppendContractRevision(ctx, outcome.ID, 1, stale)
	var conflict *ports.OutcomeConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("stale append err = %v, want OutcomeConflictError", err)
	}
	if conflict.CurrentRevisionNum != 2 || conflict.ExpectedRevisionNum != 1 {
		t.Fatalf("conflict detail = %+v", conflict)
	}
	historyAfter, err := s.ListContractRevisions(ctx, outcome.ID)
	if err != nil || len(historyAfter) != 2 {
		t.Fatalf("rejected append changed history: len=%d err=%v", len(historyAfter), err)
	}
	for _, rev := range historyAfter {
		if rev.ID == stale.ID {
			t.Fatal("rolled-back revision leaked into history")
		}
	}
}

func focusLedgerPlan(outcomeID domain.OutcomeID, revision domain.ContractRevision, number int64) domain.PlanRevision {
	// The fixture revision arrives pre-numbering (store assigns 1 on create);
	// brief freezing describes the contract as it exists at approval time.
	if revision.Number < 1 {
		revision.Number = 1
	}
	unit := domain.WorkUnit{
		ID:                      domain.WorkUnitID("wu-" + fmt.Sprintf("%d", number)),
		Kind:                    domain.WorkUnitDirect,
		Title:                   "Build and prove Local Focus Ledger",
		ContractRevisionNumber:  revision.Number,
		OutputSummary:           "Working local feature retained in the isolated worktree",
		EvidenceChecks:          []string{"validation, date boundary, aggregation, persistence checks pass"},
		VerificationRequirement: "deterministic verification outside the producer session plus owner walkthrough",
		StopConditions:          []string{"stop before unapproved dependencies, remote effects, or writes outside the worktree"},
		// An approved unit carries its exact binding, and the plan may only
		// grant what its units actually require.
		Provider:       domain.HarnessCodex,
		ModelSelection: domain.ExecutionBindingModelProviderDefault,
		RequiredCapabilities: []string{
			domain.CapabilityWorktreeRead,
			domain.CapabilityWorktreeWrite,
			domain.CapabilityWorktreeExec,
		},
	}
	grants := []domain.CapabilityGrant{
		{ID: domain.CapabilityGrantID("cg-read-" + fmt.Sprintf("%d", number)), Name: domain.CapabilityWorktreeRead, Scope: "worktree/*"},
		{ID: domain.CapabilityGrantID("cg-write-" + fmt.Sprintf("%d", number)), Name: domain.CapabilityWorktreeWrite, Scope: "worktree/*"},
		{ID: domain.CapabilityGrantID("cg-exec-" + fmt.Sprintf("%d", number)), Name: domain.CapabilityWorktreeExec, Scope: "worktree/*"},
	}
	digest, err := domain.ComputeRunBriefCoreDigest(revision, unit, grants)
	if err != nil {
		panic(err)
	}
	return domain.PlanRevision{
		ID:                     domain.PlanRevisionID(fmt.Sprintf("plan-%d", number)),
		OutcomeID:              outcomeID,
		ContractRevisionNumber: revision.Number,
		Status:                 domain.PlanStatusProposed,
		Summary:                "One direct Work Unit",
		Assumptions:            []string{"The repository keeps the existing test runner"},
		Blockers:               []string{"Owner must confirm the migration window"},
		WorkUnits:              []domain.WorkUnit{unit},
		Grants:                 grants,
		RoutingDecisions: []domain.WorkUnitRoutingDecision{{
			WorkUnitID: unit.ID,
			Decision: domain.RoutingDecision{
				Status:                    domain.RoutingDecisionRecommended,
				PolicyVersion:             domain.RoutingPolicyVersion,
				Role:                      domain.RoutingRoleWorker,
				RecommendedCandidateID:    string(domain.HarnessCodex),
				RecommendedProvider:       string(domain.HarnessCodex),
				RecommendedModelSelection: domain.ExecutionBindingModelProviderDefault,
			},
		}},
		RunBriefCoreDigest: digest,
	}
}

func TestOutcomeStore_AppendApproveAndReadBackPlans(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "mer")
	space, err := s.EnsureWorkResponsibilitySpace(ctx, "mer")
	if err != nil {
		t.Fatalf("ensure space: %v", err)
	}
	outcome, first := focusLedgerContract(space.ID, "p1")
	if err := s.CreateOutcomeWithContract(ctx, outcome, first, "req-plan-1"); err != nil {
		t.Fatalf("create outcome: %v", err)
	}

	planIn := focusLedgerPlan(outcome.ID, first, 1)
	saved, err := s.AppendPlanRevision(ctx, outcome.ID, planIn)
	if err != nil {
		t.Fatalf("append plan: %v", err)
	}
	if saved.Number != 1 {
		t.Fatalf("plan number = %d, want store-assigned 1", saved.Number)
	}

	got, ok, err := s.GetPlanRevision(ctx, outcome.ID, saved.ID)
	if err != nil || !ok {
		t.Fatalf("get plan ok=%v err=%v", ok, err)
	}
	if got.Status != domain.PlanStatusProposed {
		t.Fatalf("status = %s, want proposed", got.Status)
	}
	if len(got.WorkUnits) != 1 || got.WorkUnits[0].Title != planIn.WorkUnits[0].Title {
		t.Fatalf("work unit readback = %+v", got.WorkUnits)
	}
	if len(got.Grants) != 3 {
		t.Fatalf("grants = %+v, want the v0 trio", got.Grants)
	}
	if got.RunBriefCoreDigest != planIn.RunBriefCoreDigest {
		t.Fatal("run brief core digest did not survive the round trip")
	}
	if len(got.Assumptions) != 1 || got.Assumptions[0] != planIn.Assumptions[0] || len(got.Blockers) != 1 || got.Blockers[0] != planIn.Blockers[0] {
		t.Fatalf("plan review context = assumptions=%v blockers=%v", got.Assumptions, got.Blockers)
	}

	// Replay lookup binds both proposal status and contract revision.
	if _, ok, err := s.LatestProposedPlanRevision(ctx, outcome.ID, 1); err != nil || !ok {
		t.Fatalf("latest proposed at r1 ok=%v err=%v", ok, err)
	}
	if _, ok, _ := s.LatestProposedPlanRevision(ctx, outcome.ID, 2); ok {
		t.Fatal("no proposed plan may exist for a future contract revision")
	}

	approved, found, err := s.ApprovePlanRevision(ctx, outcome.ID, saved.ID)
	if err != nil || !found {
		t.Fatalf("approve found=%v err=%v", found, err)
	}
	if approved.Status != domain.PlanStatusApproved {
		t.Fatalf("status after approve = %s, want approved", approved.Status)
	}

	again, found, err := s.ApprovePlanRevision(ctx, outcome.ID, saved.ID)
	if err != nil || !found || again.Status != domain.PlanStatusApproved {
		t.Fatalf("re-approve found=%v status=%s err=%v, want idempotent approval", found, again.Status, err)
	}

	if missing, found, err := s.ApprovePlanRevision(ctx, outcome.ID, "plan-ghost"); found || err != nil {
		t.Fatalf("unknown plan found=%v err=%v, want quiet miss", found, err)
	} else {
		_ = missing
	}

	if latest, ok, _ := s.GetLatestPlanRevision(ctx, outcome.ID); !ok || latest.Number != 1 || latest.Status != domain.PlanStatusApproved {
		t.Fatalf("latest plan = %+v ok=%v, want approved r1", latest, ok)
	}
}

func TestOutcomeStore_AppendPlanRequiresProposedStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "mer")
	space, err := s.EnsureWorkResponsibilitySpace(ctx, "mer")
	if err != nil {
		t.Fatalf("ensure space: %v", err)
	}
	outcome, first := focusLedgerContract(space.ID, "p2")
	if err := s.CreateOutcomeWithContract(ctx, outcome, first, "req-plan-2"); err != nil {
		t.Fatalf("create outcome: %v", err)
	}

	plan := focusLedgerPlan(outcome.ID, first, 1)
	plan.Status = domain.PlanStatusApproved
	if _, err := s.AppendPlanRevision(ctx, outcome.ID, plan); err == nil {
		t.Fatal("plans must only be created proposed; approve is a separate owner decision")
	}
}

func TestOutcomeStore_PlanNumbersSerializePerOutcome(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "mer")
	space, err := s.EnsureWorkResponsibilitySpace(ctx, "mer")
	if err != nil {
		t.Fatalf("ensure space: %v", err)
	}
	outcome, first := focusLedgerContract(space.ID, "p3")
	if err := s.CreateOutcomeWithContract(ctx, outcome, first, "req-plan-3"); err != nil {
		t.Fatalf("create outcome: %v", err)
	}

	for want := int64(1); want <= 3; want++ {
		// Each proposal supersedes nothing: they stack as history because the
		// service layer decides staleness; storage only guarantees numbering.
		proposal := focusLedgerPlan(outcome.ID, first, want)
		saved, err := s.AppendPlanRevision(ctx, outcome.ID, proposal)
		if err != nil {
			t.Fatalf("append plan %d: %v", want, err)
		}
		if saved.Number != want {
			t.Fatalf("plan number = %d, want %d", saved.Number, want)
		}
	}
}

func TestWorkUnitIntentRoundTrips(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	plan, outcomeID := seedApprovedPlan(t, s, "intent-roundtrip")
	got, found, err := s.GetPlanRevision(ctx, outcomeID, plan.ID)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if got.WorkUnits[0].Intent != domain.WorkUnitIntentModifyAndExecute {
		t.Fatalf("intent=%q", got.WorkUnits[0].Intent)
	}
}
