package outcome_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apierr"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/intelligence/intelligencetest"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

type governedCheckUncertaintyFake struct {
	fact ports.GovernedCheckUncertainty
}

func (f governedCheckUncertaintyFake) GovernedCheckUncertainty(_ context.Context, sessionID domain.SessionID) (ports.GovernedCheckUncertainty, bool, error) {
	if f.fact.SessionID != sessionID {
		return ports.GovernedCheckUncertainty{}, false, nil
	}
	return f.fact, true, nil
}

// newAttemptHarness builds a fully wired Act & Observe fixture: an in-memory
// store, an execution seam double, and a heartbeat table. The returned service
// has already created the Local Focus Ledger Outcome and gotten its v0 plan
// proposed AND approved.
func newAttemptHarness(t *testing.T) (*outcome.Service, *attemptFakeStore, *fakeSpawner, *fakeHeartbeats, domain.OutcomeID, domain.PlanRevisionID) {
	t.Helper()
	store := newAttemptFakeStore()
	spawner := &fakeSpawner{readiness: ports.AgentProfileReadiness{Ready: true, Detail: "profile ok"}}
	heartbeats := newFakeHeartbeats()
	svc := outcome.New(store, nil).
		WithPlanning(intelligencetest.New(), &routingInventoryFake{candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessCodex, "")}}).
		WithExecution(spawner, heartbeats)
	svc.AdmissionPolicy = testAdmissionPolicy()

	ctx := context.Background()
	view, err := svc.Create(ctx, validCreateInput())
	if err != nil {
		t.Fatalf("create outcome: %v", err)
	}
	planView, err := svc.ProposePlan(ctx, view.Outcome.ID, 1)
	if err != nil {
		t.Fatalf("propose plan: %v", err)
	}
	if _, err := svc.ApprovePlan(ctx, view.Outcome.ID, outcome.ApprovePlanInput{
		PlanRevisionID:           planView.Plan.ID,
		ExpectedContractRevision: 1,
	}); err != nil {
		t.Fatalf("approve plan: %v", err)
	}
	return svc, store, spawner, heartbeats, view.Outcome.ID, planView.Plan.ID
}

// firstWorkUnitOfPlan lets startInput name the unit an Attempt executes without
// every harness threading a WorkUnitID through its return signature. Naming the
// unit is required now that a Plan may hold more than one.
var firstWorkUnitOfPlan = map[domain.PlanRevisionID]domain.WorkUnitID{}

func rememberFirstWorkUnit(plan domain.PlanRevision) {
	if len(plan.WorkUnits) > 0 {
		firstWorkUnitOfPlan[plan.ID] = plan.WorkUnits[0].ID
	}
}

func startInput(planID domain.PlanRevisionID) outcome.StartAttemptInput {
	return outcome.StartAttemptInput{
		PlanRevisionID: planID,
		WorkUnitID:     firstWorkUnitOfPlan[planID],
		RequestKey:     "req-start-1",
	}
}

func requireAPICode(t *testing.T, err error) string {
	t.Helper()
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want apierr.Error", err)
	}
	return apiErr.Code
}

func TestStartAttemptWithoutWorkUnitUsesDaemonSchedule(t *testing.T) {
	svc, _, spawner, _, outcomeID, planID := newAttemptHarness(t)
	view, err := svc.StartAttempt(context.Background(), outcomeID, outcome.StartAttemptInput{PlanRevisionID: planID, RequestKey: "req-daemon-selected"})
	if err != nil {
		t.Fatalf("daemon-selected start: %v", err)
	}
	want := firstWorkUnitOfPlan[planID]
	if view.Attempt.WorkUnitID != want {
		t.Fatalf("selected WorkUnit = %s, want scheduler unit %s", view.Attempt.WorkUnitID, want)
	}
	if spawner.spawnCalls() != 1 {
		t.Fatalf("provider spawn calls = %d, want one", spawner.spawnCalls())
	}
}

// TestStartAttemptFailClosedQuartetLeavesZeroRows maps the four pre-durable
// refusals: unapproved plan, invalidated brief, narrowed authority, and an
// unready provider profile. NONE may leave any durable row behind.
func TestStartAttemptFailClosedQuartetLeavesZeroRows(t *testing.T) {
	durableRows := func(store *attemptFakeStore, outcomeID domain.OutcomeID) int {
		attempts, err := store.ListAttempts(context.Background(), outcomeID)
		if err != nil {
			t.Fatal(err)
		}
		return len(attempts)
	}

	t.Run("plan not approved", func(t *testing.T) {
		svc, store, _, _, outcomeID, _ := newAttemptHarness(t)
		// Re-propose leaves an unapproved proposal; craft one by proposing on a
		// fresh revision instead: simplest is to use the approved plan but
		// demote it through a direct store append.
		_, err := store.AppendPlanRevision(context.Background(), outcomeID, mustApprovedShape(t, svc, outcomeID))
		if err != nil {
			t.Fatalf("append proposal: %v", err)
		}
		plans, _ := store.ListAttempts(context.Background(), outcomeID)
		_ = plans
		// The newest proposed plan for revision 1 replays; force a genuinely
		// unapproved target by revising the pointer away and back is complex —
		// assert through the service using the freshly appended proposal id.
		proposal, ok, err := store.LatestProposedPlanRevision(context.Background(), outcomeID, 1)
		if err != nil || !ok {
			t.Fatalf("expected replayed proposal, ok=%v err=%v", ok, err)
		}
		_, err = svc.StartAttempt(context.Background(), outcomeID, outcome.StartAttemptInput{
			PlanRevisionID: proposal.ID, WorkUnitID: firstWorkUnitOfPlan[proposal.ID], RequestKey: "rk-unapproved",
		})
		if code := requireAPICode(t, err); code != outcome.CodePlanNotApproved {
			t.Fatalf("code = %s, want PLAN_NOT_APPROVED", code)
		}
		if n := durableRows(store, outcomeID); n != 0 {
			t.Fatalf("refused admission persisted %d attempts, want 0", n)
		}
	})

	t.Run("brief invalidated by material change", func(t *testing.T) {
		svc, store, _, _, outcomeID, planID := newAttemptHarness(t)
		if _, err := svc.ReviseContract(context.Background(), outcomeID, outcome.ReviseContractInput{
			ExpectedRevision: 1,
			Goal:             "A materially different goal invalidates the frozen brief.",
			SuccessCriteria:  []string{"Different evidence."},
			Review:           "Different review.",
		}); err != nil {
			t.Fatalf("revise: %v", err)
		}
		_, err := svc.StartAttempt(context.Background(), outcomeID, startInput(planID))
		if code := requireAPICode(t, err); code != outcome.CodePlanBriefInvalidated {
			t.Fatalf("code = %s, want PLAN_BRIEF_INVALIDATED", code)
		}
		if n := durableRows(store, outcomeID); n != 0 {
			t.Fatalf("refused admission persisted %d attempts, want 0", n)
		}
	})

	t.Run("capability unauthorized", func(t *testing.T) {
		store := newAttemptFakeStore()
		spawner := &fakeSpawner{readiness: ports.AgentProfileReadiness{Ready: true}}
		planning := &routingInventoryFake{candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessCodex, "")}}
		wide := outcome.New(store, nil).
			WithPlanning(intelligencetest.New(), planning).
			WithExecution(spawner, newFakeHeartbeats())
		wide.AdmissionPolicy = testAdmissionPolicy()
		narrow := outcome.New(store, nil).
			WithPlanning(intelligencetest.New(), planning).
			WithExecution(spawner, newFakeHeartbeats())
		narrow.AdmissionPolicy = testAdmissionPolicy()
		narrow.PolicyLayers = [][]string{{domain.CapabilityWorktreeRead}}
		ctx := context.Background()
		view, err := wide.Create(ctx, validCreateInput())
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		planView, err := wide.ProposePlan(ctx, view.Outcome.ID, 1)
		if err != nil {
			t.Fatalf("propose: %v", err)
		}
		// Authority may narrow AFTER approval; admission must re-check it.
		if _, err := wide.ApprovePlan(ctx, view.Outcome.ID, outcome.ApprovePlanInput{PlanRevisionID: planView.Plan.ID}); err != nil {
			t.Fatalf("approve: %v", err)
		}
		_, err = narrow.StartAttempt(ctx, view.Outcome.ID, startInput(planView.Plan.ID))
		if code := requireAPICode(t, err); code != outcome.CodeAttemptCapabilityUnauthorized {
			t.Fatalf("code = %s, want ATTEMPT_CAPABILITY_UNAUTHORIZED", code)
		}
		if n := durableRows(store, view.Outcome.ID); n != 0 {
			t.Fatalf("refused admission persisted %d attempts, want 0", n)
		}
	})

	t.Run("provider not ready", func(t *testing.T) {
		svc, store, spawner, _, outcomeID, planID := newAttemptHarness(t)
		spawner.mu.Lock()
		spawner.readiness = ports.AgentProfileReadiness{Ready: false, Detail: "not logged in"}
		spawner.mu.Unlock()
		_, err := svc.StartAttempt(context.Background(), outcomeID, startInput(planID))
		if code := requireAPICode(t, err); code != outcome.CodeAgentProfileNotReady {
			t.Fatalf("code = %s, want AGENT_PROFILE_NOT_READY", code)
		}
		spawner.readinessErr = ports.ErrAgentBinaryNotFound
		spawner.readiness = ports.AgentProfileReadiness{}
		_, err = svc.StartAttempt(context.Background(), outcomeID, outcome.StartAttemptInput{PlanRevisionID: planID, WorkUnitID: firstWorkUnitOfPlan[planID], RequestKey: "rk-2"})
		if code := requireAPICode(t, err); code != outcome.CodeAgentBinaryNotFound {
			t.Fatalf("code = %s, want AGENT_BINARY_NOT_FOUND", code)
		}
		if n := durableRows(store, outcomeID); n != 2 {
			t.Fatalf("refused admissions persisted %d failed attempts, want 2 durable prelaunch records", n)
		}
	})
}

// currentRevisionForTest resolves the Outcome's current revision content.
func currentRevisionForTest(svc *outcome.Service, outcomeID domain.OutcomeID) (domain.ContractRevision, error) {
	view, err := svc.Get(context.Background(), outcomeID)
	if err != nil {
		return domain.ContractRevision{}, err
	}
	return view.Current, nil
}

// mustApprovedShape builds a minimal VALID proposed plan for the not-approved
// refusal test.
func mustApprovedShape(t *testing.T, svc *outcome.Service, outcomeID domain.OutcomeID) domain.PlanRevision {
	t.Helper()
	full, err := svc.GetLatestPlan(context.Background(), outcomeID)
	if err != nil {
		t.Fatalf("latest plan: %v", err)
	}
	revision, err := currentRevisionForTest(svc, outcomeID)
	if err != nil {
		t.Fatalf("current revision: %v", err)
	}
	unit := full.Plan.WorkUnits[0]
	grants := full.Plan.Grants
	digest, err := domain.ComputeRunBriefCoreDigest(revision, unit, grants)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	return domain.PlanRevision{
		ID:                     domain.PlanRevisionID("plan-proposal-2"),
		OutcomeID:              outcomeID,
		ContractRevisionNumber: 1,
		Status:                 domain.PlanStatusProposed,
		Summary:                "Duplicate proposal",
		WorkUnits:              []domain.WorkUnit{unit},
		Grants:                 grants,
		RunBriefCoreDigest:     digest,
	}
}

// TestStartAttemptAdmissionOrdering pins the happy path: queued row + fence,
// spawn through the narrow seam, session ref with digests + snapshot,
// running transition, and a deterministic brief prompt.
func TestStartAttemptAdmissionOrdering(t *testing.T) {
	svc, store, spawner, heartbeats, outcomeID, planID := newAttemptHarness(t)
	ctx := context.Background()

	view, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if view.Attempt.Status != domain.AttemptRunning || view.Attempt.Number != 1 {
		t.Fatalf("attempt = %+v, want running #1", view.Attempt)
	}
	if len(view.Sessions) != 1 || view.Sessions[0].Seq != 1 {
		t.Fatalf("session refs = %+v, want exactly one binding", view.Sessions)
	}
	ref := view.Sessions[0]
	if len(ref.RunBriefCoreDigest) != 64 || len(ref.RunBriefCompiledDigest) != 64 {
		t.Fatal("both digests must be recorded on the ref")
	}
	if !strings.Contains(ref.AdmissionSnapshot, fmt.Sprintf(`"snapshotVersion":%d`, domain.AdmissionSnapshotVersion)) {
		t.Fatalf("admission snapshot missing version pin: %s", ref.AdmissionSnapshot)
	}
	if view.Fence == nil || !view.Fence.Open() || view.Fence.AttemptID != view.Attempt.ID {
		t.Fatalf("fence = %+v, want open custody held by this attempt", view.Fence)
	}
	if view.Presentation.Phase != domain.AttemptPhaseUnconfirmed || !view.Presentation.Unconfirmed {
		// The fake session was never signalled: missing heartbeat derives
		// UNCONFIRMED, never dead.
		t.Fatalf("presentation = %+v, want unconfirmed until first signal", view.Presentation)
	}
	if len(spawner.spawned) != 1 {
		t.Fatalf("spawn calls = %d, want 1", spawner.spawnCalls())
	}
	req := spawner.spawned[0]
	if req.Harness != domain.HarnessCodex {
		t.Fatalf("harness = %s, want the locked v0 default codex", req.Harness)
	}
	if !strings.Contains(req.Prompt, "Record focus locally") && !strings.Contains(req.Prompt, validCreateInput().Goal[:20]) {
		t.Fatalf("prompt must carry the frozen goal, got: %.80s", req.Prompt)
	}
	if !strings.Contains(req.Prompt, "Stop conditions:") {
		t.Fatal("prompt must carry the stop conditions")
	}
	if req.ExecutionPolicy == nil || req.ExecutionPolicy.OutcomeID != outcomeID || req.ExecutionPolicy.PlanRevisionID != planID || req.ExecutionPolicy.WorkUnitID != firstWorkUnitOfPlan[planID] {
		t.Fatalf("spawn policy = %+v, want attributed approved WorkUnit policy", req.ExecutionPolicy)
	}

	// First signal arrives: derivation flips to executing.
	heartbeats.signal(domain.SessionID(ref.SessionID))
	reread, err := svc.GetAttempt(ctx, outcomeID, view.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Presentation.Phase != domain.AttemptPhaseExecuting {
		t.Fatalf("phase = %s, want executing after first signal", reread.Presentation.Phase)
	}
	_ = store
}

func TestStartAttemptDeliversExactAssignedContractAndApprovedCheckMaterial(t *testing.T) {
	svc, store, spawner, _, outcomeID, planID := newAttemptHarness(t)
	ctx := context.Background()
	view, err := svc.Get(ctx, outcomeID)
	if err != nil {
		t.Fatal(err)
	}
	revision := view.Current
	assignedText := "The report contains these literal headings:\n## Major components\n## Prioritized gaps"
	unrelatedText := "UNRELATED CRITERION MUST NOT REACH THE FIRST WORKER"
	revision.Criteria[0].Text = assignedText
	revision.SuccessCriteria[0] = assignedText
	revision.Criteria = append(revision.Criteria, domain.ContractCriterion{
		ID: "crit-unrelated-delivery", ContractRevisionID: revision.ID, Position: 2, Text: unrelatedText,
	})
	revision.SuccessCriteria = append(revision.SuccessCriteria, unrelatedText)
	revision.Review = "Review the exact report headings and the daemon-owned validator output."
	if err := revision.Validate(); err != nil {
		t.Fatalf("custom Contract revision: %v", err)
	}
	store.fakeStore.mu.Lock()
	store.revs[outcomeID][0] = revision
	store.fakeStore.mu.Unlock()

	plan, found, err := store.GetPlanRevision(ctx, outcomeID, planID)
	if err != nil || !found {
		t.Fatalf("get approved plan: found=%v err=%v", found, err)
	}
	unit := plan.WorkUnits[0]
	unit.OutputSummary = "Write the report described elsewhere."
	unit.EvidenceChecks = []string{"Apply the normal report checks."}
	unit.VerificationRequirement = "Kennel will inspect the approved check result."
	unit.CriterionIDs = []domain.CriterionID{revision.Criteria[0].ID}
	validator := "from pathlib import Path\np=Path('kennel-architecture-assessment.md')\ns=p.read_text()\nassert '## Major components' in s\nassert '## Prioritized gaps' in s"
	unit.Checks = []domain.ApprovedCheck{{
		ID: "check-report-headings", CriterionID: revision.Criteria[0].ID,
		Argv: []string{"python3", "-c", validator}, TimeoutSeconds: 60,
	}}
	unrelated := unit
	unrelated.ID = "wu-unrelated-delivery"
	unrelated.Title = "Handle a separate responsibility"
	unrelated.OutputSummary = "Produce an unrelated result."
	unrelated.EvidenceChecks = []string{"Inspect the unrelated result."}
	unrelated.VerificationRequirement = "Verify only the unrelated criterion."
	unrelated.DependsOn = []domain.WorkUnitID{unit.ID}
	unrelated.CriterionIDs = []domain.CriterionID{revision.Criteria[1].ID}
	unrelated.Checks = nil
	plan.WorkUnits = []domain.WorkUnit{unit, unrelated}
	routing := plan.RoutingDecisions[0]
	routing.WorkUnitID = unrelated.ID
	plan.RoutingDecisions = append(plan.RoutingDecisions, routing)
	plan.RunBriefCoreDigest, err = domain.ComputePlanRunBriefCoreDigest(revision, plan.WorkUnits, plan.Grants)
	if err != nil {
		t.Fatalf("recompute frozen RunBrief: %v", err)
	}
	if err := plan.ValidateForApproval(revision); err != nil {
		t.Fatalf("custom approved plan: %v", err)
	}
	store.planFakeStore.mu.Lock()
	for index := range store.plans[outcomeID] {
		if store.plans[outcomeID][index].ID == planID {
			store.plans[outcomeID][index] = plan
		}
	}
	store.units[planID] = append([]domain.WorkUnit(nil), plan.WorkUnits...)
	store.planFakeStore.mu.Unlock()
	firstWorkUnitOfPlan[planID] = unit.ID
	// This fixture rewrites its in-memory approved Plan to exercise the exact
	// prompt. Remove its synthetic admission so the fake can create matching
	// evidence; production Plan rows and evidence are immutable.
	store.planFakeStore.mu.Lock()
	delete(store.admissions, planID)
	delete(store.specs, planID)
	for i := range store.plans[outcomeID] {
		if store.plans[outcomeID][i].ID == planID {
			store.plans[outcomeID][i].Status = domain.PlanStatusProposed
		}
	}
	store.planFakeStore.mu.Unlock()
	if _, err := svc.ApprovePlan(ctx, outcomeID, outcome.ApprovePlanInput{PlanRevisionID: planID, ExpectedContractRevision: revision.Number}); err != nil {
		t.Fatalf("refresh synthetic approval admission: %v", err)
	}

	started, err := svc.StartAttempt(ctx, outcomeID, outcome.StartAttemptInput{
		PlanRevisionID: planID, WorkUnitID: unit.ID, RequestKey: "req-exact-task-delivery",
	})
	if err != nil {
		t.Fatalf("StartAttempt: %v", err)
	}
	if spawner.spawnCalls() != 1 {
		t.Fatalf("spawn calls = %d, want one", spawner.spawnCalls())
	}
	req := spawner.spawned[0]
	for name, literal := range map[string]string{
		"assigned Contract criterion": assignedText,
		"approved validator":          validator,
		"Contract review method":      revision.Review,
		"Contract revision identity":  revision.ID.String(),
		"WorkUnit identity":           unit.ID.String(),
	} {
		if !strings.Contains(req.Prompt, literal) {
			t.Fatalf("spawn prompt lost %s %q:\n%s", name, literal, req.Prompt)
		}
	}
	if strings.Contains(req.Prompt, unrelatedText) {
		t.Fatalf("spawn prompt widened criterion scope:\n%s", req.Prompt)
	}
	if !strings.Contains(req.Prompt, "Kennel will run this exact approved check after the provider terminates") || !strings.Contains(req.Prompt, "Do not reconstruct or run it through a provider tool") {
		t.Fatalf("spawn prompt did not distinguish verification ownership from worker authority:\n%s", req.Prompt)
	}
	if req.ExecutionPolicy == nil || req.ExecutionPolicy.ContractRevisionNumber != revision.Number || req.ExecutionPolicy.PlanRevisionID != planID || req.ExecutionPolicy.WorkUnitID != unit.ID || req.ExecutionPolicy.RunBriefCoreDigest != plan.RunBriefCoreDigest {
		t.Fatalf("spawn policy lost frozen attribution: %+v", req.ExecutionPolicy)
	}
	if len(req.ExecutionPolicy.ApprovedChecks) != 1 || req.ExecutionPolicy.ApprovedChecks[0].ID != unit.Checks[0].ID || !reflect.DeepEqual(req.ExecutionPolicy.ApprovedChecks[0].Argv, unit.Checks[0].Argv) {
		t.Fatalf("spawn policy lost frozen executable check: %+v", req.ExecutionPolicy.ApprovedChecks)
	}
	if len(started.Sessions) != 1 || started.Sessions[0].RunBriefCoreDigest != plan.RunBriefCoreDigest {
		t.Fatalf("durable session binding lost frozen RunBrief: %+v", started.Sessions)
	}
	replayed, err := svc.StartAttempt(ctx, outcomeID, outcome.StartAttemptInput{
		PlanRevisionID: planID, WorkUnitID: unit.ID, RequestKey: "req-exact-task-delivery",
	})
	if err != nil || replayed.Attempt.ID != started.Attempt.ID || spawner.spawnCalls() != 1 {
		t.Fatalf("retry did not preserve attributed Attempt: replay=%+v err=%v spawns=%d", replayed.Attempt, err, spawner.spawnCalls())
	}
}

// TestStartAttemptReplayIsIdempotent proves a delivered request key resolves
// to the original attempt without a second admission.
func TestStartAttemptReplayIsIdempotent(t *testing.T) {
	svc, store, spawner, _, outcomeID, planID := newAttemptHarness(t)

	first, err := svc.StartAttempt(context.Background(), outcomeID, startInput(planID))
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	replay, err := svc.StartAttempt(context.Background(), outcomeID, startInput(planID))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Attempt.ID != first.Attempt.ID {
		t.Fatalf("replay produced %s, want original %s", replay.Attempt.ID, first.Attempt.ID)
	}
	if spawner.spawnCalls() != 1 {
		t.Fatalf("spawn calls = %d, want exactly 1 across the replay", spawner.spawnCalls())
	}
	attempts, _ := store.ListAttempts(context.Background(), outcomeID)
	if len(attempts) != 1 {
		t.Fatalf("attempts = %d, want 1", len(attempts))
	}
}

func TestStartAttemptReplayWithDifferentWorkUnitConflicts(t *testing.T) {
	svc, _, spawner, _, outcomeID, planID := newAttemptHarness(t)
	first, err := svc.StartAttempt(context.Background(), outcomeID, startInput(planID))
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	_, err = svc.StartAttempt(context.Background(), outcomeID, outcome.StartAttemptInput{
		PlanRevisionID: planID,
		WorkUnitID:     domain.WorkUnitID("different-work-unit"),
		RequestKey:     startInput(planID).RequestKey,
	})
	if code := requireAPICode(t, err); code != outcome.CodeAttemptRequestKeyConflict {
		t.Fatalf("code = %s, want %s", code, outcome.CodeAttemptRequestKeyConflict)
	}
	if spawner.spawnCalls() != 1 || first.Attempt.ID == "" {
		t.Fatalf("replay conflict changed execution: first=%s spawns=%d", first.Attempt.ID, spawner.spawnCalls())
	}
}

// TestAnySpawnRefusalIsAmbiguousNeverFailed pins the round-2 custody law: NO
// spawn error is classifiable as a clean failure (the adapter may have
// created the runtime before failing), so every refusal routes to admission
// ambiguity — queued + unconfirmed + custody held — and `failed` is never
// written by any #31 path.
func TestAnySpawnRefusalIsAmbiguousNeverFailed(t *testing.T) {
	svc, _, spawner, _, outcomeID, planID := newAttemptHarness(t)
	ctx := context.Background()
	spawner.failNextSpawn(errors.New("worker pane died during launch"))

	_, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
	if code := requireAPICode(t, err); code != outcome.CodeAttemptStartUnresolved {
		t.Fatalf("code = %s, want ATTEMPT_START_UNRESOLVED", code)
	}
	attempts, listErr := storeList(t, svc, outcomeID)
	if listErr != nil || len(attempts) != 1 {
		t.Fatalf("attempts len=%d err=%v", len(attempts), listErr)
	}
	stored := attempts[0]
	if stored.Status != domain.AttemptQueued {
		t.Fatalf("status = %s, want queued — never failed", stored.Status)
	}
	fence, held, ferr := svcGetFence(t, svc, outcomeID)
	if ferr != nil || !held || fence.AttemptID != stored.ID {
		t.Fatalf("fence must stay HELD by the ambiguous attempt (held=%v err=%v)", held, ferr)
	}
	observations, _ := storeObservations(t, svc, outcomeID, stored.ID)
	if len(observations) == 0 || observations[0].Kind != domain.ObservationAdmissionAmbiguous {
		t.Fatalf("observations = %+v, want admission_ambiguous first", observations)
	}
	receipts, _ := storeReceipts(t, svc, outcomeID, stored.ID)
	if len(receipts) != 1 || receipts[0].Resolution != domain.RecoveryNeedsAttention {
		t.Fatalf("receipts = %+v, want needs_attention", receipts)
	}

	// Replacement cannot bypass reconcile while the ambiguity holds custody.
	if _, err := svc.StartAttempt(ctx, outcomeID, outcome.StartAttemptInput{PlanRevisionID: planID, WorkUnitID: firstWorkUnitOfPlan[planID], RequestKey: "rk-replace"}); err == nil {
		t.Fatal("replacement start must be refused while custody is unresolved")
	} else if code := requireAPICode(t, err); code != outcome.CodeAttemptFenceHeld && code != outcome.CodeNoRunnableWorkUnit {
		// Scheduling answers before custody does now: the plan's only unit is
		// still held by the unresolved attempt, so nothing is runnable. Either
		// refusal proves the same thing — no replacement slipped past reconcile.
		t.Fatalf("code = %s, want ATTEMPT_FENCE_HELD or NO_RUNNABLE_WORK_UNIT", code)
	}
}

func svcGetFence(t *testing.T, svc *outcome.Service, outcomeID domain.OutcomeID) (domain.AttemptFence, bool, error) {
	t.Helper()
	views, err := svc.ListAttempts(context.Background(), outcomeID)
	if err != nil {
		return domain.AttemptFence{}, false, err
	}
	for _, v := range views {
		if v.Fence != nil {
			return *v.Fence, true, nil
		}
	}
	return domain.AttemptFence{}, false, nil
}

func storeObservations(t *testing.T, svc *outcome.Service, outcomeID domain.OutcomeID, attemptID domain.AttemptID) ([]domain.AttemptObservation, error) {
	t.Helper()
	views, err := svc.ListAttempts(context.Background(), outcomeID)
	if err != nil {
		return nil, err
	}
	for _, v := range views {
		if v.Attempt.ID == attemptID {
			return v.Observations, nil
		}
	}
	return nil, nil
}

func storeReceipts(t *testing.T, svc *outcome.Service, outcomeID domain.OutcomeID, attemptID domain.AttemptID) ([]domain.AttemptRecoveryReceipt, error) {
	t.Helper()
	views, err := svc.ListAttempts(context.Background(), outcomeID)
	if err != nil {
		return nil, err
	}
	for _, v := range views {
		if v.Attempt.ID == attemptID {
			return v.Receipts, nil
		}
	}
	return nil, nil
}

// TestContainReconcileReplacementFlow walks the injected-loss path from the
// v0 failure matrix end to end.
func TestContainReconcileReplacementFlow(t *testing.T) {
	svc, _, _, heartbeats, outcomeID, planID := newAttemptHarness(t)
	ctx := context.Background()

	first, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	heartbeats.signal(domain.SessionID(first.Sessions[0].SessionID))

	// Provider process disappears mid-work-unit: the session GCs away.
	heartbeats.forget(domain.SessionID(first.Sessions[0].SessionID))
	unconfirmed, err := svc.GetAttempt(ctx, outcomeID, first.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !unconfirmed.Presentation.Unconfirmed || unconfirmed.Presentation.Phase != domain.AttemptPhaseUnconfirmed {
		t.Fatalf("presentation = %+v, want unconfirmed", unconfirmed.Presentation)
	}

	// Containment marks suspicion without deciding anything.
	contained, err := svc.RecoverAttempt(ctx, outcomeID, first.Attempt.ID, outcome.RecoveryInput{Action: outcome.RecoveryActionContain})
	if err != nil {
		t.Fatalf("contain: %v", err)
	}
	if contained.Attempt.Attempt.Status != domain.AttemptRunning {
		t.Fatalf("containment mutated status to %s", contained.Attempt.Attempt.Status)
	}
	lastObs := contained.Attempt.Observations[len(contained.Attempt.Observations)-1]
	if lastObs.Kind != domain.ObservationAttemptContained {
		t.Fatalf("last observation = %s, want contained", lastObs.Kind)
	}

	// Reconcile cannot prove liveness OR provider stop: custody stays held
	// and the refusal escalates as needs_attention (anti-duplicate-writer).
	_, err = svc.RecoverAttempt(ctx, outcomeID, first.Attempt.ID, outcome.RecoveryInput{Action: outcome.RecoveryActionReconcile})
	if code := requireAPICode(t, err); code != outcome.CodeAttemptCustodyUnproven {
		t.Fatalf("unproven reconcile code = %s, want ATTEMPT_CUSTODY_UNPROVEN", code)
	}
	held, err := svc.GetAttempt(ctx, outcomeID, first.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if held.Fence == nil || !held.Fence.Open() {
		t.Fatal("unconfirmed launch must keep custody")
	}
	// Owner-asserted containment then drives lost + release + receipt.
	reconciled, err := svc.RecoverAttempt(ctx, outcomeID, first.Attempt.ID, outcome.RecoveryInput{
		Action: outcome.RecoveryActionReconcile, ConfirmProviderStopped: true,
	})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if reconciled.Attempt.Attempt.Status != domain.AttemptLost {
		t.Fatalf("status = %s, want lost", reconciled.Attempt.Attempt.Status)
	}
	if reconciled.Receipt == nil || reconciled.Receipt.Resolution != domain.RecoveryReplacement {
		t.Fatalf("receipt = %+v, want replacement_attempt", reconciled.Receipt)
	}
	if reconciled.Attempt.Fence != nil {
		t.Fatal("custody must be released after the lost verdict")
	}

	// Safe next action: a replacement attempt acquires the freed subject.
	replacement, err := svc.StartAttempt(ctx, outcomeID, outcome.StartAttemptInput{PlanRevisionID: planID, WorkUnitID: firstWorkUnitOfPlan[planID], RequestKey: "rk-replacement"})
	if err != nil {
		t.Fatalf("replacement start: %v", err)
	}
	if replacement.Attempt.Number != 2 || replacement.Attempt.ID == first.Attempt.ID {
		t.Fatalf("replacement = %+v, want NEW attempt #2", replacement.Attempt)
	}

	// Stale events stay inspectable but inert (D5): appending an observation
	// to the LOST predecessor succeeds and touches nothing current.
	stale, err := svc.RecordObservation(ctx, outcomeID, first.Attempt.ID, outcome.RecordObservationInput{
		Kind: "stale_provider_report", Payload: `{"claims":"success"}`,
	})
	if err != nil {
		t.Fatalf("stale observation must remain insertable: %v", err)
	}
	if stale.Seq < 2 {
		t.Fatalf("stale observation seq = %d", stale.Seq)
	}
	current, err := svc.GetAttempt(ctx, outcomeID, replacement.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Observations) != 0 {
		t.Fatalf("stale observation leaked into current truth: %+v", current.Observations)
	}
	if current.Attempt.Status != domain.AttemptRunning {
		t.Fatalf("current status = %s, want running", current.Attempt.Status)
	}
}

// TestReconcileProvenAliveRecordsResumed proves the resume-same branch:
// present + signalled + not terminated => resumed receipt, no status churn.
func TestReconcileProvenAliveRecordsResumed(t *testing.T) {
	svc, _, _, heartbeats, outcomeID, planID := newAttemptHarness(t)
	ctx := context.Background()
	view, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	heartbeats.signal(domain.SessionID(view.Sessions[0].SessionID))

	result, err := svc.RecoverAttempt(ctx, outcomeID, view.Attempt.ID, outcome.RecoveryInput{Action: outcome.RecoveryActionReconcile})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if result.Receipt == nil || result.Receipt.Resolution != domain.RecoveryResumed {
		t.Fatalf("receipt = %+v, want resumed", result.Receipt)
	}
	if result.Attempt.Attempt.Status != domain.AttemptRunning {
		t.Fatalf("status = %s, want unchanged running", result.Attempt.Attempt.Status)
	}
}

// TestLivenessLoopClassifiesTerminatedProviderSession drives the daemon hook:
// termination becomes reconciled + provider_exit observation — an ordered
// observation only, never success.
func TestLivenessLoopClassifiesTerminatedProviderSession(t *testing.T) {
	svc, _, _, heartbeats, outcomeID, planID := newAttemptHarness(t)
	ctx := context.Background()
	view, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	heartbeats.signal(domain.SessionID(view.Sessions[0].SessionID))
	heartbeats.terminate(domain.SessionID(view.Sessions[0].SessionID))

	if err := svc.EvaluateAttemptLiveness(ctx); err != nil {
		t.Fatalf("liveness evaluation: %v", err)
	}
	reread, err := svc.GetAttempt(ctx, outcomeID, view.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Attempt.Status != domain.AttemptReconciled {
		t.Fatalf("status = %s, want reconciled", reread.Attempt.Status)
	}
	if !reread.Presentation.EndedUnclassified {
		t.Fatalf("presentation = %+v, want ended-unclassified", reread.Presentation)
	}
	lastObs := reread.Observations[len(reread.Observations)-1]
	if lastObs.Kind != domain.ObservationProviderExit {
		t.Fatalf("observation = %s, want provider_exit", lastObs.Kind)
	}
	// A second pass is a no-op: the attempt no longer runs.
	if err := svc.EvaluateAttemptLiveness(ctx); err != nil {
		t.Fatalf("second liveness pass: %v", err)
	}
}

// TestCancelOwnerControls pins the owner path that remains: cancel requires
// provider termination through the seam and refuses cleanly on stop failure.
// Pause/resume endpoints are intentionally absent until a real provider
// control contract exists (ADR 0007 territory).
func TestCancelOwnerControls(t *testing.T) {
	svc, _, spawner, heartbeats, outcomeID, planID := newAttemptHarness(t)
	ctx := context.Background()
	view, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	heartbeats.signal(domain.SessionID(view.Sessions[0].SessionID))

	spawner.failNextTerminate(errors.New("pane refuses to die"))
	if _, err := svc.CancelAttempt(ctx, outcomeID, view.Attempt.ID); err == nil {
		t.Fatal("cancel must refuse while provider stop fails")
	}
	reread, err := svc.GetAttempt(ctx, outcomeID, view.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Attempt.Status != domain.AttemptRunning {
		t.Fatalf("status = %s after refused cancel, want unchanged running", reread.Attempt.Status)
	}

	spawner.failNextTerminate(nil)
	cancelled, err := svc.CancelAttempt(ctx, outcomeID, view.Attempt.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cancelled.Attempt.Status != domain.AttemptCancelled {
		t.Fatalf("status = %s, want cancelled", cancelled.Attempt.Status)
	}
}

// TestAmbiguousStartStaysQueuedAndUnconfirmed pins the intake-spec failure
// row "Provider start outcome unknown": an UNKNOWN spawn outcome never
// becomes failed or running — the attempt stays queued, derives as
// unconfirmed, records the ambiguity, holds custody, and reconcile decides.
func TestAmbiguousStartStaysQueuedAndUnconfirmed(t *testing.T) {
	svc, _, spawner, heartbeats, outcomeID, planID := newAttemptHarness(t)
	ctx := context.Background()
	spawner.mu.Lock()
	spawner.spawnErr = context.DeadlineExceeded // outcome UNKNOWN after send
	spawner.mu.Unlock()

	_, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
	if code := requireAPICode(t, err); code != "ATTEMPT_START_UNRESOLVED" {
		t.Fatalf("code = %s, want ATTEMPT_START_UNRESOLVED", code)
	}
	attempts, _ := storeList(t, svc, outcomeID)
	if len(attempts) != 1 || attempts[0].Status != domain.AttemptQueued {
		t.Fatalf("attempt = %+v, want queued (never failed on unknown)", attempts)
	}
	view, err := svc.GetAttempt(ctx, outcomeID, attempts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !view.Presentation.Unconfirmed || view.Presentation.Phase != domain.AttemptPhaseUnconfirmed {
		t.Fatalf("presentation = %+v, want unconfirmed", view.Presentation)
	}
	lastObs := view.Observations[len(view.Observations)-1]
	if lastObs.Kind != domain.ObservationAdmissionAmbiguous {
		t.Fatalf("observation = %s, want admission_ambiguous", lastObs.Kind)
	}
	if len(view.Receipts) != 1 || view.Receipts[0].Resolution != domain.RecoveryNeedsAttention {
		t.Fatalf("receipts = %+v, want needs_attention", view.Receipts)
	}
	if view.Fence == nil || !view.Fence.Open() {
		t.Fatal("ambiguous start must keep holding custody")
	}

	// Duplicate start stays disabled while the ambiguous attempt holds the
	// fence. Reconcile WITHOUT stop-proof refuses and escalates; the fence
	// stays held (anti-duplicate-writer contract).
	_, err = svc.RecoverAttempt(ctx, outcomeID, attempts[0].ID, outcome.RecoveryInput{Action: outcome.RecoveryActionReconcile})
	if err != nil {
		t.Fatalf("prepared-boundary reconcile: %v", err)
	}
	held, heldErr := svc.GetAttempt(ctx, outcomeID, attempts[0].ID)
	if heldErr != nil {
		t.Fatal(heldErr)
	}
	if held.Fence != nil {
		t.Fatal("LaunchPrepared reconciliation must release custody")
	}
	// Reconciliation is idempotent after prepared custody is accounted.
	reconciled, recErr := svc.RecoverAttempt(ctx, outcomeID, attempts[0].ID, outcome.RecoveryInput{
		Action: outcome.RecoveryActionReconcile,
	})
	if recErr != nil {
		t.Fatalf("confirmed reconcile: %v", recErr)
	}
	if reconciled.Attempt.Attempt.Status != domain.AttemptLost || reconciled.Attempt.Fence != nil {
		t.Fatalf("reconcile verdict = %+v fence=%+v, want lost with custody released", reconciled.Attempt.Attempt, reconciled.Attempt.Fence)
	}
	containedObs := reconciled.Attempt.Observations[len(reconciled.Attempt.Observations)-1]
	if containedObs.Kind != domain.ObservationAdmissionAmbiguous {
		t.Fatalf("prepared evidence changed unexpectedly: %s", containedObs.Kind)
	}
	_ = heartbeats
}

// TestTerminalPredecessorsReleaseCustodyThroughReconcile pins the D4 custody
// handover for EVERY terminal predecessor: failed-before-spawn,
// owner-cancelled, and ended-unclassified attempts must be releasable through
// reconcile/replace without rewriting their immutable history.
func TestTerminalPredecessorsReleaseCustodyThroughReconcile(t *testing.T) {
	ctx := context.Background()

	t.Run("cancelled", func(t *testing.T) {
		svc, _, spawner, heartbeats, outcomeID, planID := newAttemptHarness(t)
		first, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
		if err != nil {
			t.Fatalf("start: %v", err)
		}
		heartbeats.signal(domain.SessionID(first.Sessions[0].SessionID))
		if _, err := svc.CancelAttempt(ctx, outcomeID, first.Attempt.ID); err != nil {
			t.Fatalf("cancel: %v", err)
		}
		// Cancel terminated the provider through the seam; mirror the durable
		// is_terminated fact the real Kill writes before recovering.
		heartbeats.terminate(domain.SessionID(first.Sessions[0].SessionID))
		verdict, err := svc.RecoverAttempt(ctx, outcomeID, first.Attempt.ID, outcome.RecoveryInput{Action: outcome.RecoveryActionReplace})
		if err != nil {
			t.Fatalf("replace after cancel must release custody: %v", err)
		}
		if verdict.Attempt.Attempt.Status != domain.AttemptCancelled {
			t.Fatalf("history rewritten to %s", verdict.Attempt.Attempt.Status)
		}
		if verdict.Attempt.Fence != nil {
			t.Fatal("custody must be released")
		}
		assertReplacementStartable(t, svc, spawner, outcomeID, planID)
	})

	t.Run("failed status alone is not stop-proof", func(t *testing.T) {
		svc, store, spawner, _, outcomeID, planID := newAttemptHarness(t)
		spawner.failNextSpawn(errors.New("boom")) // ambiguous queued attempt
		if _, err := svc.StartAttempt(ctx, outcomeID, startInput(planID)); err == nil {
			t.Fatal("expected unresolved admission")
		}
		attempts, _ := storeList(t, svc, outcomeID)
		// Force the reserved failed status directly to interrogate the law.
		if _, err := store.TransitionAttemptStatus(ctx, outcomeID, attempts[0].ID,
			domain.AttemptQueued, domain.AttemptFailed, time.Now()); err != nil {
			t.Fatalf("force failed: %v", err)
		}
		// LaunchPrepared now durably proves the provider boundary was not crossed.
		if _, err := svc.RecoverAttempt(ctx, outcomeID, attempts[0].ID, outcome.RecoveryInput{Action: outcome.RecoveryActionReplace}); err != nil {
			t.Fatalf("prepared attempt should release custody: %v", err)
		}
		verdict, err := svc.RecoverAttempt(ctx, outcomeID, attempts[0].ID, outcome.RecoveryInput{
			Action: outcome.RecoveryActionReplace, ConfirmProviderStopped: true,
		})
		if err != nil {
			t.Fatalf("owner-confirmed replace: %v", err)
		}
		if verdict.Attempt.Attempt.Status != domain.AttemptFailed {
			t.Fatalf("history rewritten to %s", verdict.Attempt.Attempt.Status)
		}
		assertReplacementStartable(t, svc, spawner, outcomeID, planID)
	})

	t.Run("reconciled after provider exit", func(t *testing.T) {
		svc, _, _, heartbeats, outcomeID, planID := newAttemptHarness(t)
		first, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
		if err != nil {
			t.Fatalf("start: %v", err)
		}
		heartbeats.signal(domain.SessionID(first.Sessions[0].SessionID))
		heartbeats.terminate(domain.SessionID(first.Sessions[0].SessionID))
		if err := svc.EvaluateAttemptLiveness(ctx); err != nil {
			t.Fatal(err)
		}
		// Rework path: replace the ended-unclassified attempt so rework can run.
		if _, err := svc.RecoverAttempt(ctx, outcomeID, first.Attempt.ID, outcome.RecoveryInput{Action: outcome.RecoveryActionReplace}); err != nil {
			t.Fatalf("replace after reconciled must release custody: %v", err)
		}
	})
}

func assertReplacementStartable(t *testing.T, svc *outcome.Service, spawner *fakeSpawner, outcomeID domain.OutcomeID, planID domain.PlanRevisionID) {
	t.Helper()
	spawner.mu.Lock()
	spawner.readiness = ports.AgentProfileReadiness{Ready: true}
	spawner.spawnErr = nil
	spawner.mu.Unlock()
	replacement, err := svc.StartAttempt(context.Background(), outcomeID, outcome.StartAttemptInput{
		PlanRevisionID: planID,
		WorkUnitID:     firstWorkUnitOfPlan[planID],
		RequestKey:     "rk-replacement-" + time.Now().String(),
	})
	if err != nil {
		t.Fatalf("replacement start must succeed after custody release: %v", err)
	}
	if replacement.Fence == nil || !replacement.Fence.Open() {
		t.Fatalf("replacement must hold fresh custody: %+v", replacement.Fence)
	}
}

func storeList(t *testing.T, svc *outcome.Service, outcomeID domain.OutcomeID) ([]domain.Attempt, error) {
	t.Helper()
	views, err := svc.ListAttempts(context.Background(), outcomeID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Attempt, 0, len(views))
	for _, v := range views {
		out = append(out, v.Attempt)
	}
	return out, nil
}

// TestActivationUnknownKeepsLiveProviderUnconfirmed pins the P1 fix: a lost
// queued->running promotion AFTER a successful spawn never surfaces as a raw
// 500 or as a silent queued row — it records activation ambiguity, derives
// unconfirmed, keeps custody, and returns a typed refusal.
func TestActivationUnknownKeepsLiveProviderUnconfirmed(t *testing.T) {
	store := newAttemptFakeStore()
	store.dropActivationOnce = true
	spawner := &fakeSpawner{readiness: ports.AgentProfileReadiness{Ready: true}}
	heartbeats := newFakeHeartbeats()
	svc := outcome.New(store, nil).
		WithPlanning(intelligencetest.New(), &routingInventoryFake{candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessCodex, "")}}).
		WithExecution(spawner, heartbeats)
	svc.AdmissionPolicy = testAdmissionPolicy()

	ctx := context.Background()
	outcomeView, err := svc.Create(ctx, validCreateInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	outcomeID := outcomeView.Outcome.ID
	planView, err := svc.ProposePlan(ctx, outcomeID, 1)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if _, err := svc.ApprovePlan(ctx, outcomeID, outcome.ApprovePlanInput{PlanRevisionID: planView.Plan.ID}); err != nil {
		t.Fatalf("approve: %v", err)
	}

	_, startErr := svc.StartAttempt(ctx, outcomeID, startInput(planView.Plan.ID))
	if code := requireAPICode(t, startErr); code != outcome.CodeAttemptActivationUnresolved {
		t.Fatalf("code = %s, want ATTEMPT_ACTIVATION_UNRESOLVED", code)
	}
	attempts, _ := storeList(t, svc, outcomeID)
	if len(attempts) != 1 || attempts[0].Status != domain.AttemptQueued {
		t.Fatalf("attempt = %+v, want queued (live provider must not be failed)", attempts)
	}
	got, err := svc.GetAttempt(ctx, outcomeID, attempts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Presentation.Unconfirmed || got.Presentation.Phase != domain.AttemptPhaseUnconfirmed {
		t.Fatalf("presentation = %+v, want unconfirmed for the live provider", got.Presentation)
	}
	lastObs := got.Observations[len(got.Observations)-1]
	if lastObs.Kind != domain.ObservationActivationAmbiguous {
		t.Fatalf("observation = %s, want activation_ambiguous", lastObs.Kind)
	}
	if got.Fence == nil || !got.Fence.Open() {
		t.Fatal("fence must stay held")
	}
}

// TestCancelControlsTheProvider pins provider authority on cancellation: the
// bound session is terminated through the execution seam BEFORE the status
// moves; termination failure refuses cancellation entirely.
func TestCancelControlsTheProvider(t *testing.T) {
	svc, _, spawner, heartbeats, outcomeID, planID := newAttemptHarness(t)
	ctx := context.Background()
	first, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	sessionID := domain.SessionID(first.Sessions[0].SessionID)
	heartbeats.signal(sessionID)

	spawner.failNextTerminate(errors.New("pane refuses to die"))
	_, err = svc.CancelAttempt(ctx, outcomeID, first.Attempt.ID)
	if code := requireAPICode(t, err); code != outcome.CodeAttemptProviderStopFailed {
		t.Fatalf("code = %s, want ATTEMPT_PROVIDER_STOP_FAILED", code)
	}
	reread, err := svc.GetAttempt(ctx, outcomeID, first.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Attempt.Status != domain.AttemptRunning {
		t.Fatalf("status = %s after refused cancel, want unchanged running", reread.Attempt.Status)
	}
	if reread.Fence == nil || !reread.Fence.Open() {
		t.Fatal("refused cancel must keep custody held")
	}
	lastObs := reread.Observations[len(reread.Observations)-1]
	if lastObs.Kind != domain.ObservationProviderStopFailed {
		t.Fatalf("observation = %s, want provider_stop_failed", lastObs.Kind)
	}

	spawner.failNextTerminate(nil)
	if _, err := svc.CancelAttempt(ctx, outcomeID, first.Attempt.ID); err != nil {
		t.Fatalf("cancel after stop works: %v", err)
	}
	if len(spawner.terminated) == 0 || spawner.terminated[0] != string(sessionID) {
		t.Fatalf("terminate calls = %v, want the bound session", spawner.terminated)
	}
	// Mirror the durable is_terminated fact real Kill writes.
	heartbeats.terminate(sessionID)
	stopped, err := svc.GetAttempt(ctx, outcomeID, first.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Presentation.Phase != domain.AttemptPhaseHaltedCancelled {
		t.Fatalf("phase = %s after cancelled+terminated provider, want halted_cancelled", stopped.Presentation.Phase)
	}
	// Machine-proved stop: replace releases WITHOUT owner confirmation.
	if _, err := svc.RecoverAttempt(ctx, outcomeID, first.Attempt.ID, outcome.RecoveryInput{Action: outcome.RecoveryActionReplace}); err != nil {
		t.Fatalf("replace after proved stop: %v", err)
	}
}

// TestStaleHeartbeatDerivesUnconfirmedAndNeedsInput pins the renewable
// liveness contract: recency gates aliveness (sticky states exempt), and
// waiting_input/blocked surface as Needs You rather than generic Waiting.
func TestStaleHeartbeatDerivesUnconfirmedAndNeedsInput(t *testing.T) {
	svc, _, _, heartbeats, outcomeID, planID := newAttemptHarness(t)
	svc.WithStaleHeartbeat(time.Millisecond)
	ctx := context.Background()

	first, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	ref := first.Sessions[0]
	heartbeats.signal(domain.SessionID(ref.SessionID))
	time.Sleep(2 * time.Millisecond)

	stale, err := svc.GetAttempt(ctx, outcomeID, first.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stale.Presentation.Unconfirmed {
		t.Fatalf("stale active session derived %+v, want unconfirmed", stale.Presentation)
	}

	// Sticky waiting_input is exempt from staleness and demands attention.
	heartbeats.mu.Lock()
	rec := heartbeats.sessions[domain.SessionID(ref.SessionID)]
	rec.Activity = domain.Activity{
		State:          domain.ActivityWaitingInput,
		LastActivityAt: time.Now().Add(-time.Hour),
	}
	heartbeats.sessions[domain.SessionID(ref.SessionID)] = rec
	heartbeats.mu.Unlock()

	waiting, err := svc.GetAttempt(ctx, outcomeID, first.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if waiting.Presentation.Phase != domain.AttemptPhaseNeedsInput {
		t.Fatalf("phase = %s, want needs_input", waiting.Presentation.Phase)
	}
	if waiting.Presentation.Unconfirmed {
		t.Fatal("waiting_input must not derive unconfirmed")
	}

	// The liveness loop renews the custodian's lease each pass.
	if _, open := storeOpenFence(t, svc, outcomeID); !open {
		t.Fatal("expected open fence")
	}
	if err := svc.EvaluateAttemptLiveness(ctx); err != nil {
		t.Fatalf("liveness pass with needs-input attempt: %v", err)
	}
}

func storeOpenFence(t *testing.T, svc *outcome.Service, outcomeID domain.OutcomeID) (view outcome.AttemptView, open bool) {
	t.Helper()
	views, err := svc.ListAttempts(context.Background(), outcomeID)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range views {
		if v.Fence != nil && v.Fence.Open() {
			return v, true
		}
	}
	return outcome.AttemptView{}, false
}

// TestBindFailureKeepsCustodyUntilOwnerContainment injects the EXACT
// round-3 P1 regression: a live spawn whose durable binding write fails. The
// sequence must be ambiguity -> refused reconcile (no proof) -> recorded
// owner containment -> released fence -> replacement attempt on a NEW row.
func TestBindFailureKeepsCustodyUntilOwnerContainment(t *testing.T) {
	store := newAttemptFakeStore()
	store.failBindOnce = true
	spawner := &fakeSpawner{readiness: ports.AgentProfileReadiness{Ready: true}}
	svc := outcome.New(store, nil).WithPlanning(intelligencetest.New(), &routingInventoryFake{candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessCodex, "")}}).WithExecution(spawner, newFakeHeartbeats())
	svc.AdmissionPolicy = testAdmissionPolicy()

	ctx := context.Background()
	outcomeView, err := svc.Create(ctx, validCreateInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	outcomeID := outcomeView.Outcome.ID
	planView, err := svc.ProposePlan(ctx, outcomeID, 1)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if _, err := svc.ApprovePlan(ctx, outcomeID, outcome.ApprovePlanInput{PlanRevisionID: planView.Plan.ID}); err != nil {
		t.Fatalf("approve: %v", err)
	}

	// Spawn succeeds, the bind write fails: activation ambiguity, never
	// failed and never a raw 500.
	_, startErr := svc.StartAttempt(ctx, outcomeID, startInput(planView.Plan.ID))
	if code := requireAPICode(t, startErr); code != outcome.CodeAttemptActivationUnresolved {
		t.Fatalf("code = %s, want ATTEMPT_ACTIVATION_UNRESOLVED", code)
	}
	attempts, _ := storeList(t, svc, outcomeID)
	if len(attempts) != 1 || attempts[0].Status != domain.AttemptQueued {
		t.Fatalf("attempts = %+v, want exactly one queued row", attempts)
	}
	bound, err := svc.GetAttempt(ctx, outcomeID, attempts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !bound.Presentation.Unconfirmed || bound.Presentation.Phase != domain.AttemptPhaseUnconfirmed {
		t.Fatalf("presentation = %+v, want unconfirmed", bound.Presentation)
	}
	lastObs := bound.Observations[len(bound.Observations)-1]
	if lastObs.Kind != domain.ObservationActivationAmbiguous {
		t.Fatalf("observation = %s, want activation_ambiguous", lastObs.Kind)
	}
	if len(bound.Receipts) != 1 || bound.Receipts[0].Resolution != domain.RecoveryNeedsAttention {
		t.Fatalf("receipts = %+v, want needs_attention", bound.Receipts)
	}
	if bound.Fence == nil || !bound.Fence.Open() {
		t.Fatal("custody must stay held across the bind failure")
	}

	// Reconcile WITHOUT proof refuses closed: an unbound attempt can never
	// obtain machine termination proof by itself.
	if _, err := svc.RecoverAttempt(ctx, outcomeID, attempts[0].ID,
		outcome.RecoveryInput{Action: outcome.RecoveryActionReconcile}); err == nil {
		t.Fatal("unproven reconcile must refuse")
	} else if code := requireAPICode(t, err); code != outcome.CodeAttemptCustodyUnproven {
		t.Fatalf("code = %s, want ATTEMPT_CUSTODY_UNPROVEN", code)
	}

	// The owner asserts containment: recorded as its own observation, the
	// fence releases, and the verdict is lost + replacement receipt.
	released, err := svc.RecoverAttempt(ctx, outcomeID, attempts[0].ID, outcome.RecoveryInput{
		Action: outcome.RecoveryActionReconcile, ConfirmProviderStopped: true,
	})
	if err != nil {
		t.Fatalf("owner-contained reconcile: %v", err)
	}
	if released.Attempt.Attempt.Status != domain.AttemptLost || released.Attempt.Fence != nil {
		t.Fatalf("verdict = %+v fence=%+v, want lost with custody released", released.Attempt.Attempt, released.Attempt.Fence)
	}
	if released.Receipt == nil || released.Receipt.Resolution != domain.RecoveryReplacement {
		t.Fatalf("receipt = %+v, want replacement_attempt", released.Receipt)
	}
	containment := released.Attempt.Observations[len(released.Attempt.Observations)-1]
	if containment.Kind != domain.ObservationOwnerContained {
		t.Fatalf("last observation = %s, want owner_contained", containment.Kind)
	}

	// Replacement acquires the freed subject as a NEW attempt row.
	replacement, err := svc.StartAttempt(ctx, outcomeID, outcome.StartAttemptInput{
		PlanRevisionID: planView.Plan.ID, WorkUnitID: firstWorkUnitOfPlan[planView.Plan.ID], RequestKey: "rk-after-bind-failure",
	})
	if err != nil {
		t.Fatalf("replacement start: %v", err)
	}
	if replacement.Attempt.Number != 2 || replacement.Attempt.ID == attempts[0].ID {
		t.Fatalf("replacement = %+v, want NEW attempt #2", replacement.Attempt)
	}
	if replacement.Fence == nil || !replacement.Fence.Open() {
		t.Fatal("replacement must hold fresh custody")
	}
	if spawner.spawnCalls() != 2 {
		t.Fatalf("spawn calls = %d, want one per attempt", spawner.spawnCalls())
	}
}

// TestCancelRecordsPreservedWorkspace pins the two-fact stop contract: a
// dirty worktree preserved during Kill does NOT make the provider look live —
// cancellation proceeds on ProviderStopped and RECORDS WorkspaceFreed=false.
func TestCancelRecordsPreservedWorkspace(t *testing.T) {
	svc, _, spawner, heartbeats, outcomeID, planID := newAttemptHarness(t)
	ctx := context.Background()
	first, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	heartbeats.signal(domain.SessionID(first.Sessions[0].SessionID))

	// Dirty preserved worktree: provider durably terminated, workspace kept.
	spawner.setTerminateResult(ports.TerminationResult{ProviderStopped: true, WorkspaceFreed: false})
	cancelled, err := svc.CancelAttempt(ctx, outcomeID, first.Attempt.ID)
	if err != nil {
		t.Fatalf("cancel with preserved dirty workspace must proceed once the provider stop is proven: %v", err)
	}
	if cancelled.Attempt.Status != domain.AttemptCancelled {
		t.Fatalf("status = %s, want cancelled", cancelled.Attempt.Status)
	}
	lastObs := cancelled.Observations[len(cancelled.Observations)-1]
	if lastObs.Kind != domain.ObservationOwnerCancel {
		t.Fatalf("observation = %s, want owner_cancelled", lastObs.Kind)
	}
	for _, want := range []string{`"providerStopped":true`, `"workspaceFreed":false`} {
		if !strings.Contains(lastObs.Payload, want) {
			t.Fatalf("observation payload %s missing %s", lastObs.Payload, want)
		}
	}
}

// TestLeaseRenewalGatesOnProvableLiveness pins the health-gated lease: only
// PROVABLY alive custodians refresh last_renewed_at; stale, sticky-exempt
// waiting, and vanished sessions each get exactly the treatment their
// evidence supports.
func TestLeaseRenewalGatesOnProvableLiveness(t *testing.T) {
	svc, store, _, heartbeats, outcomeID, planID := newAttemptHarness(t)
	ctx := context.Background()
	first, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	sessionID := domain.SessionID(first.Sessions[0].SessionID)
	heartbeats.signal(sessionID)

	// Provably alive custody RENEWS.
	if err := svc.EvaluateAttemptLiveness(ctx); err != nil {
		t.Fatalf("liveness pass 1: %v", err)
	}
	renewed, open := storeOpenFence(t, svc, outcomeID)
	if !open {
		t.Fatal("expected open fence")
	}
	if renewed.Fence.LastRenewedAt.IsZero() || !renewed.Fence.LastRenewedAt.After(renewed.Fence.IssuedAt) {
		t.Fatalf("alive custodian must renew: issued=%v renewed=%v", renewed.Fence.IssuedAt, renewed.Fence.LastRenewedAt)
	}
	stamp := renewed.Fence.LastRenewedAt

	// Stale non-sticky custody keeps its OLD stamp: staleness derives
	// UNCONFIRMED and never refreshes the lease.
	heartbeats.backdate(sessionID, 2*domain.DefaultStaleHeartbeatWindow)
	if err := svc.EvaluateAttemptLiveness(ctx); err != nil {
		t.Fatalf("liveness pass 2: %v", err)
	}
	stale, _ := storeOpenFence(t, svc, outcomeID)
	if !stale.Fence.LastRenewedAt.Equal(stamp) {
		t.Fatalf("stale custodian must NOT renew: %v -> %v", stamp, stale.Fence.LastRenewedAt)
	}

	// Sticky waiting_input IS exempt from staleness: waiting on its user is
	// provably-alive attention, so the lease refreshes again.
	heartbeats.mu.Lock()
	rec := heartbeats.sessions[sessionID]
	rec.Activity = domain.Activity{State: domain.ActivityWaitingInput, LastActivityAt: time.Now().Add(-time.Hour)}
	heartbeats.sessions[sessionID] = rec
	heartbeats.mu.Unlock()
	if err := svc.EvaluateAttemptLiveness(ctx); err != nil {
		t.Fatalf("liveness pass 3: %v", err)
	}
	waiting, _ := storeOpenFence(t, svc, outcomeID)
	if waiting.Fence.LastRenewedAt.Before(stamp) {
		t.Fatalf("sticky waiting_input must renew: %v -> %v", stamp, waiting.Fence.LastRenewedAt)
	}

	// A session that VANISHED keeps the old stamp too: absence is unknown.
	heartbeats.forget(sessionID)
	if err := svc.EvaluateAttemptLiveness(ctx); err != nil {
		t.Fatalf("liveness pass 4: %v", err)
	}
	gone, _ := storeOpenFence(t, svc, outcomeID)
	if gone.Fence.LastRenewedAt.Before(waiting.Fence.LastRenewedAt) {
		t.Fatal("vanished session must not rewind or renew the lease")
	}
	if store.renewals < 2 {
		t.Fatalf("renewal count = %d, want >=2 (alive + sticky passes)", store.renewals)
	}
}

func TestLivenessLoopClassifiesNonzeroGovernedProcessExitAsFailed(t *testing.T) {
	svc, _, spawner, heartbeats, outcomeID, planID := newAttemptHarness(t)
	spawner.completionBoundary = domain.AttemptCompletionProcessExit
	view, err := svc.StartAttempt(context.Background(), outcomeID, startInput(planID))
	if err != nil {
		t.Fatal(err)
	}
	sessionID := domain.SessionID(view.Sessions[0].SessionID)
	exitCode := 23
	rec := heartbeats.sessions[sessionID]
	rec.IsTerminated = true
	rec.Metadata.SupervisedProcessExitCode = &exitCode
	rec.Metadata.SupervisedProcessExitReason = "failed"
	heartbeats.sessions[sessionID] = rec

	if err := svc.EvaluateAttemptLiveness(context.Background()); err != nil {
		t.Fatal(err)
	}
	reread, err := svc.GetAttempt(context.Background(), outcomeID, view.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Attempt.Status != domain.AttemptFailed {
		t.Fatalf("status = %s, want failed", reread.Attempt.Status)
	}
}

// TestLivenessLoopClassifiesContradictoryZeroExitAsFailed covers the
// "zero-plus-failure" case: an exit code of 0 paired with a non-"exited"
// reason is a contradiction, not a success, even though a check on exit code
// alone would have read it as zero and let it through to reconciled/eligible
// for succeeded promotion.
func TestLivenessLoopClassifiesContradictoryZeroExitAsFailed(t *testing.T) {
	svc, _, spawner, heartbeats, outcomeID, planID := newAttemptHarness(t)
	spawner.completionBoundary = domain.AttemptCompletionProcessExit
	view, err := svc.StartAttempt(context.Background(), outcomeID, startInput(planID))
	if err != nil {
		t.Fatal(err)
	}
	sessionID := domain.SessionID(view.Sessions[0].SessionID)
	exitCode := 0
	rec := heartbeats.sessions[sessionID]
	rec.IsTerminated = true
	rec.Metadata.SupervisedProcessExitCode = &exitCode
	rec.Metadata.SupervisedProcessExitReason = "failed"
	heartbeats.sessions[sessionID] = rec

	if err := svc.EvaluateAttemptLiveness(context.Background()); err != nil {
		t.Fatal(err)
	}
	reread, err := svc.GetAttempt(context.Background(), outcomeID, view.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Attempt.Status != domain.AttemptFailed {
		t.Fatalf("status = %s, want failed", reread.Attempt.Status)
	}
}

// TestLivenessLoopReconcilesLegitimateZeroExit is the positive counterpart:
// a genuine zero exit code paired with reason "exited" must NOT be
// classified as failed.
func TestLivenessLoopReconcilesLegitimateZeroExit(t *testing.T) {
	svc, _, spawner, heartbeats, outcomeID, planID := newAttemptHarness(t)
	spawner.completionBoundary = domain.AttemptCompletionProcessExit
	view, err := svc.StartAttempt(context.Background(), outcomeID, startInput(planID))
	if err != nil {
		t.Fatal(err)
	}
	sessionID := domain.SessionID(view.Sessions[0].SessionID)
	exitCode := 0
	rec := heartbeats.sessions[sessionID]
	rec.IsTerminated = true
	rec.Metadata.SupervisedProcessExitCode = &exitCode
	rec.Metadata.SupervisedProcessExitReason = domain.SupervisedExitReasonExited
	heartbeats.sessions[sessionID] = rec

	if err := svc.EvaluateAttemptLiveness(context.Background()); err != nil {
		t.Fatal(err)
	}
	reread, err := svc.GetAttempt(context.Background(), outcomeID, view.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Attempt.Status != domain.AttemptReconciled {
		t.Fatalf("status = %s, want reconciled", reread.Attempt.Status)
	}
}

func TestLivenessLoopBlocksCompletionAfterUnknownGovernedCheckTerminationUntilOwnerReconciles(t *testing.T) {
	svc, _, spawner, heartbeats, outcomeID, planID := newAttemptHarness(t)
	spawner.completionBoundary = domain.AttemptCompletionProcessExit
	view, err := svc.StartAttempt(context.Background(), outcomeID, startInput(planID))
	if err != nil {
		t.Fatal(err)
	}
	sessionID := domain.SessionID(view.Sessions[0].SessionID)
	exitCode := 0
	rec := heartbeats.sessions[sessionID]
	rec.IsTerminated = true
	rec.Metadata.SupervisedProcessExitCode = &exitCode
	rec.Metadata.SupervisedProcessExitReason = domain.SupervisedExitReasonExited
	heartbeats.sessions[sessionID] = rec
	svc.WithGovernedCheckUncertainty(governedCheckUncertaintyFake{fact: ports.GovernedCheckUncertainty{
		SessionID: sessionID, CheckID: "check-1", TerminationUnknown: true,
		TimedOut: true, EnforcedBy: "test-fence", ObservedAt: time.Now().UTC(),
	}})

	// Exercise recovery before any periodic liveness import: the durable MCP
	// marker itself must close the race and require explicit containment.
	if _, err := svc.RecoverAttempt(context.Background(), outcomeID, view.Attempt.ID, outcome.RecoveryInput{Action: outcome.RecoveryActionReplace}); err == nil {
		t.Fatal("provider exit alone must not release custody for an unknown check process tree")
	}
	for i := 0; i < 2; i++ {
		if err := svc.EvaluateAttemptLiveness(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	blocked, err := svc.GetAttempt(context.Background(), outcomeID, view.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Attempt.Status != domain.AttemptRunning || blocked.Fence == nil {
		t.Fatalf("unknown termination must retain running custody: status=%s fence=%v", blocked.Attempt.Status, blocked.Fence)
	}
	if !blocked.Presentation.Unconfirmed || !strings.Contains(blocked.Presentation.NextAction, "effects in flight") {
		t.Fatalf("presentation did not surface unknown check effects: %+v", blocked.Presentation)
	}
	unknownObservations := 0
	for _, observation := range blocked.Observations {
		if observation.Kind == domain.ObservationGovernedCheckTerminationUnknown {
			unknownObservations++
		}
	}
	if unknownObservations != 1 {
		t.Fatalf("unknown termination observations = %d, want exactly one", unknownObservations)
	}
	recovered, err := svc.RecoverAttempt(context.Background(), outcomeID, view.Attempt.ID, outcome.RecoveryInput{
		Action: outcome.RecoveryActionReplace, ConfirmProviderStopped: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Attempt.Attempt.Status != domain.AttemptLost || recovered.Attempt.Fence != nil {
		t.Fatalf("owner-confirmed reconciliation = status %s fence %v", recovered.Attempt.Attempt.Status, recovered.Attempt.Fence)
	}
	assertReplacementStartable(t, svc, spawner, outcomeID, planID)
}

// TestLivenessLoopClassifiesMissingExitCodeAsFailed covers a durably
// terminated session whose reason claims "exited" but carries no exit code
// at all — never successful reconciliation from a nil code.
func TestLivenessLoopClassifiesMissingExitCodeAsFailed(t *testing.T) {
	svc, _, spawner, heartbeats, outcomeID, planID := newAttemptHarness(t)
	spawner.completionBoundary = domain.AttemptCompletionProcessExit
	view, err := svc.StartAttempt(context.Background(), outcomeID, startInput(planID))
	if err != nil {
		t.Fatal(err)
	}
	sessionID := domain.SessionID(view.Sessions[0].SessionID)
	rec := heartbeats.sessions[sessionID]
	rec.IsTerminated = true
	rec.Metadata.SupervisedProcessExitCode = nil
	rec.Metadata.SupervisedProcessExitReason = "unknown"
	heartbeats.sessions[sessionID] = rec

	if err := svc.EvaluateAttemptLiveness(context.Background()); err != nil {
		t.Fatal(err)
	}
	reread, err := svc.GetAttempt(context.Background(), outcomeID, view.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Attempt.Status != domain.AttemptFailed {
		t.Fatalf("status = %s, want failed", reread.Attempt.Status)
	}
}

// livenessLoopFailureConvergenceCase drives fix #3: a governed provider
// exit's Running->Failed transition, observation, and custody release must
// commit atomically. Injecting a failure at either step must roll back the
// whole operation (Attempt still Running, fence still held, no observation
// recorded) rather than stranding a partial commit; a retried tick must then
// converge to exactly one terminal outcome with custody released and no
// duplicate observation or Attempt.
func livenessLoopFailureConvergenceCase(t *testing.T, injectAt string) {
	t.Helper()
	svc, store, spawner, heartbeats, outcomeID, planID := newAttemptHarness(t)
	spawner.completionBoundary = domain.AttemptCompletionProcessExit
	view, err := svc.StartAttempt(context.Background(), outcomeID, startInput(planID))
	if err != nil {
		t.Fatal(err)
	}
	sessionID := domain.SessionID(view.Sessions[0].SessionID)
	exitCode := 17
	rec := heartbeats.sessions[sessionID]
	rec.IsTerminated = true
	rec.Metadata.SupervisedProcessExitCode = &exitCode
	rec.Metadata.SupervisedProcessExitReason = "failed"
	heartbeats.sessions[sessionID] = rec

	store.injectRunningTerminationFailureAt = injectAt
	if err := svc.EvaluateAttemptLiveness(context.Background()); err == nil {
		t.Fatalf("expected injected %s failure to surface", injectAt)
	}
	mid, err := svc.GetAttempt(context.Background(), outcomeID, view.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mid.Attempt.Status != domain.AttemptRunning {
		t.Fatalf("status after injected %s failure = %s, want still running (no partial commit)", injectAt, mid.Attempt.Status)
	}
	if mid.Fence == nil {
		t.Fatalf("custody released despite injected %s failure before any commit", injectAt)
	}
	if len(mid.Observations) != 0 {
		t.Fatalf("observation recorded despite injected %s failure: %d", injectAt, len(mid.Observations))
	}

	// Restart/reconcile: the Attempt is still Running, so the next liveness
	// pass revisits it and retries the whole operation from scratch.
	if err := svc.EvaluateAttemptLiveness(context.Background()); err != nil {
		t.Fatalf("retry after injected %s failure: %v", injectAt, err)
	}
	final, err := svc.GetAttempt(context.Background(), outcomeID, view.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Attempt.Status != domain.AttemptFailed {
		t.Fatalf("status after convergence = %s, want failed", final.Attempt.Status)
	}
	if final.Fence != nil {
		t.Fatal("custody must be released once the failure converges")
	}
	if len(final.Observations) != 1 {
		t.Fatalf("observation count after convergence = %d, want exactly one (no duplicate)", len(final.Observations))
	}
}

func TestLivenessLoopConvergesAfterInjectedObservationWriteFailure(t *testing.T) {
	livenessLoopFailureConvergenceCase(t, "observation")
}

func TestLivenessLoopConvergesAfterInjectedCustodyReleaseFailure(t *testing.T) {
	livenessLoopFailureConvergenceCase(t, "release")
}

func TestWallBudgetStopsProviderBeforeFailureAndCustodyRelease(t *testing.T) {
	svc, store, spawner, _, outcomeID, planID := newAttemptHarness(t)
	ctx := context.Background()
	first, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	list := store.attempts[outcomeID]
	list[0].CreatedAt = time.Now().Add(-2 * time.Hour)
	store.attempts[outcomeID] = list
	store.mu.Unlock()
	if err := svc.EvaluateAttemptLiveness(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetAttempt(ctx, outcomeID, first.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Attempt.Status != domain.AttemptFailed {
		t.Fatalf("status=%s", got.Attempt.Status)
	}
	if got.Fence != nil && got.Fence.Open() {
		t.Fatalf("fence=%+v", got.Fence)
	}
	if len(spawner.terminated) != 1 || spawner.terminated[0] != first.Sessions[0].SessionID {
		t.Fatalf("terminated=%v", spawner.terminated)
	}
	last := got.Observations[len(got.Observations)-1]
	if last.Kind != domain.ObservationBudgetExceeded || !strings.Contains(last.Payload, "wall_time_budget_exhausted") {
		t.Fatalf("observation=%+v", last)
	}
}

func TestWallBudgetStopFailureKeepsRunningAndCustody(t *testing.T) {
	svc, store, spawner, _, outcomeID, planID := newAttemptHarness(t)
	ctx := context.Background()
	first, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	list := store.attempts[outcomeID]
	list[0].CreatedAt = time.Now().Add(-2 * time.Hour)
	store.attempts[outcomeID] = list
	store.mu.Unlock()
	spawner.failNextTerminate(errors.New("stop unproven"))
	if err := svc.EvaluateAttemptLiveness(ctx); err == nil {
		t.Fatal("expected stop error")
	}
	got, err := svc.GetAttempt(ctx, outcomeID, first.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Attempt.Status != domain.AttemptRunning || got.Fence == nil || !got.Fence.Open() {
		t.Fatalf("got=%+v", got)
	}
}

func TestBudgetStopClaimRecoveryConvergesWithoutRetry(t *testing.T) {
	svc, store, spawner, _, out, plan := newAttemptHarness(t)
	ctx := context.Background()
	first, err := svc.StartAttempt(ctx, out, startInput(plan))
	if err != nil {
		t.Fatal(err)
	}
	claim := domain.AttemptBudgetStop{AttemptID: first.Attempt.ID, SessionID: first.Sessions[0].SessionID, Reason: domain.RuntimeWallTimeBudgetExhausted, MeasuredUsage: `{"reasonCode":"wall_time_budget_exhausted"}`, ClaimedAt: time.Now()}
	if _, _, err := store.ClaimAttemptBudgetStop(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if err := svc.EvaluateAttemptLiveness(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetAttempt(ctx, out, first.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Attempt.Status != domain.AttemptFailed || len(spawner.terminated) != 1 {
		t.Fatalf("got=%s term=%v", got.Attempt.Status, spawner.terminated)
	}
	attempts, _ := svc.ListAttempts(ctx, out)
	if len(attempts) != 1 {
		t.Fatalf("silent retry: %d attempts", len(attempts))
	}
}

func TestBudgetMachineStopRecoveryFinalizesWithoutSecondTerminate(t *testing.T) {
	svc, store, spawner, _, out, plan := newAttemptHarness(t)
	ctx := context.Background()
	first, err := svc.StartAttempt(ctx, out, startInput(plan))
	if err != nil {
		t.Fatal(err)
	}
	claim := domain.AttemptBudgetStop{AttemptID: first.Attempt.ID, SessionID: first.Sessions[0].SessionID, Reason: domain.RuntimeTokenBudgetExhausted, MeasuredUsage: `{"reasonCode":"token_budget_exhausted"}`, ClaimedAt: time.Now()}
	if _, _, err := store.ClaimAttemptBudgetStop(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordAttemptBudgetProviderStopped(ctx, claim.AttemptID, claim.SessionID, claim.Reason, `{"reasonCode":"token_budget_exhausted","providerStopped":true}`, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := svc.EvaluateAttemptLiveness(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetAttempt(ctx, out, first.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Attempt.Status != domain.AttemptFailed || len(spawner.terminated) != 0 {
		t.Fatalf("got=%s term=%v", got.Attempt.Status, spawner.terminated)
	}
}
