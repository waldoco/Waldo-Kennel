package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
)

// seedApprovedPlan builds a full Decide & Authorize lineage for one project
// and returns its approved plan plus the outcome id.
func seedApprovedPlan(t *testing.T, s *sqlite.Store, projectID string) (domain.PlanRevision, domain.OutcomeID) {
	t.Helper()
	ctx := context.Background()
	seedProject(t, s, projectID)
	space, err := s.EnsureWorkResponsibilitySpace(ctx, domain.ProjectID(projectID))
	if err != nil {
		t.Fatalf("ensure space: %v", err)
	}
	outcome := domain.Outcome{
		ID:      domain.OutcomeID("out-" + projectID),
		SpaceID: space.ID,
		Title:   "Local Focus Ledger",
	}
	first := domain.ContractRevision{
		ID:              domain.ContractRevisionID("cr-" + projectID),
		OutcomeID:       outcome.ID,
		Number:          1,
		Goal:            "Record focus locally.",
		SuccessCriteria: []string{"Blocks are recorded."},
		Review:          "Deterministic checks.",
	}
	if err := s.CreateOutcomeWithContract(ctx, outcome, first, "rk-create-"+projectID); err != nil {
		t.Fatalf("create outcome: %v", err)
	}
	unit := domain.WorkUnit{
		ID:                      domain.WorkUnitID("wu-" + projectID),
		Kind:                    domain.WorkUnitDirect,
		Title:                   "Deliver Local Focus Ledger",
		ContractRevisionNumber:  1,
		OutputSummary:           "Working local feature in the isolated worktree.",
		EvidenceChecks:          []string{"checks pass"},
		VerificationRequirement: "Deterministic checks.",
		StopConditions:          []string{"stop before remote effects"},
		// An approved unit carries the exact provider/model binding an Attempt
		// must execute; there is no runtime default to fall back to.
		Provider:       domain.HarnessCodex,
		ModelSelection: domain.ExecutionBindingModelProviderDefault,
		// Plan grants must be exactly what its units require: an unused grant
		// is authority nobody asked for, and the plan is now rejected for it.
		RequiredCapabilities: []string{
			domain.CapabilityWorktreeRead,
			domain.CapabilityWorktreeWrite,
			domain.CapabilityWorktreeExec,
		},
	}
	grants := []domain.CapabilityGrant{
		{ID: domain.CapabilityGrantID("cg-read-" + projectID), Name: domain.CapabilityWorktreeRead, Scope: "worktree/*"},
		{ID: domain.CapabilityGrantID("cg-write-" + projectID), Name: domain.CapabilityWorktreeWrite, Scope: "worktree/*"},
		{ID: domain.CapabilityGrantID("cg-exec-" + projectID), Name: domain.CapabilityWorktreeExec, Scope: "worktree/*"},
	}
	digest, err := domain.ComputeRunBriefCoreDigest(first, unit, grants)
	if err != nil {
		t.Fatalf("compute digest: %v", err)
	}
	plan, err := s.AppendPlanRevision(ctx, outcome.ID, domain.PlanRevision{
		ID:                     domain.PlanRevisionID("plan-" + projectID),
		OutcomeID:              outcome.ID,
		ContractRevisionNumber: 1,
		Status:                 domain.PlanStatusProposed,
		Summary:                "One direct Work Unit",
		WorkUnits:              []domain.WorkUnit{unit},
		Grants:                 grants,
		// Approval freezes why this unit runs on this provider, so the plan
		// carries the routing decision beside the binding it produced.
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
	})
	if err != nil {
		t.Fatalf("append plan: %v", err)
	}
	approved, found, err := s.ApprovePlanRevision(ctx, outcome.ID, plan.ID)
	if err != nil || !found {
		t.Fatalf("approve plan: found=%v err=%v", found, err)
	}
	return approved, outcome.ID
}

// TestAttemptStore_FencedAdmissionIsAtomicAndExclusive pins D3/D4 at the store
// layer: the winner gets a queued attempt plus the open fence; the loser gets
// a typed fence conflict with ZERO durable rows (fail-closed admission).
func TestAttemptStore_FencedAdmissionIsAtomicAndExclusive(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	plan, outcomeID := seedApprovedPlan(t, s, "mer")
	subject := domain.FenceSubjectForProject("mer")

	at, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-att-1", subject))
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	if at.Status != domain.AttemptQueued || at.Number != 1 {
		t.Fatalf("attempt = %+v, want queued #1", at)
	}

	held, ok, err := s.OpenFenceForSubject(ctx, subject)
	if err != nil || !ok {
		t.Fatalf("open fence ok=%v err=%v", ok, err)
	}
	if held.AttemptID != at.ID || !held.Open() {
		t.Fatalf("fence = %+v, want open custody by %s", held, at.ID)
	}

	before, err := s.ListAttempts(ctx, outcomeID)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	_, err = s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-att-2", subject))
	var fenced *ports.AttemptFenceHeldError
	if !errors.As(err, &fenced) {
		t.Fatalf("second admission must fail with AttemptFenceHeldError, got %v", err)
	}
	if fenced.Holder != at.ID {
		t.Fatalf("conflict holder = %s, want %s", fenced.Holder, at.ID)
	}
	after, err := s.ListAttempts(ctx, outcomeID)
	if err != nil {
		t.Fatalf("re-list attempts: %v", err)
	}
	if len(before) != 1 || len(after) != 1 {
		t.Fatalf("failed admission must leave zero rows: before=%d after=%d", len(before), len(after))
	}

	// Replay through the request key resolves the ORIGINAL attempt without a
	// second write.
	replay, found, err := s.FindAttemptByIdempotencyKey(ctx, "rk-att-1")
	if err != nil || !found || replay.ID != at.ID {
		t.Fatalf("replay found=%v id=%s err=%v", found, replay.ID, err)
	}
	finalList, err := s.ListAttempts(ctx, outcomeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(finalList) != 1 {
		t.Fatalf("replayed start must not stack attempts, got %d", len(finalList))
	}
}

// TestAttemptStore_ReadOnlyFencesDoNotContend pins ADR 0009 §6 at the store
// layer: two read-only Attempts against the same project subject both get
// their own durable fence row and neither is refused, while a write-capable
// admission against the exclusive project subject still conflicts exactly as
// TestAttemptStore_FencedAdmissionIsAtomicAndExclusive proves above.
func TestAttemptStore_ReadOnlyFencesDoNotContend(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	plan, outcomeID := seedApprovedPlan(t, s, "read-fence")
	subject := domain.FenceSubjectForProject("read-fence")

	firstAdmission := admissionFor(outcomeID, plan, "rk-read-1", subject)
	firstAdmission.FenceReadOnly = true
	first, err := s.CreateAttemptWithFence(ctx, firstAdmission)
	if err != nil {
		t.Fatalf("create first read-only attempt: %v", err)
	}

	secondAdmission := admissionFor(outcomeID, plan, "rk-read-2", subject)
	secondAdmission.FenceReadOnly = true
	second, err := s.CreateAttemptWithFence(ctx, secondAdmission)
	if err != nil {
		t.Fatalf("second concurrent read-only admission must not conflict: %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("expected two distinct attempts, got the same id twice: %s", first.ID)
	}

	firstFence, ok, err := s.OpenFenceForSubject(ctx, domain.FenceSubjectForReadOnlyAttempt(subject, first.ID))
	if err != nil || !ok || firstFence.AttemptID != first.ID {
		t.Fatalf("first read fence ok=%v err=%v fence=%+v, want open custody by %s", ok, err, firstFence, first.ID)
	}
	secondFence, ok, err := s.OpenFenceForSubject(ctx, domain.FenceSubjectForReadOnlyAttempt(subject, second.ID))
	if err != nil || !ok || secondFence.AttemptID != second.ID {
		t.Fatalf("second read fence ok=%v err=%v fence=%+v, want open custody by %s", ok, err, secondFence, second.ID)
	}

	// The exclusive project subject itself must remain unheld: a write-
	// capable admission is still free to take it while only read-only
	// Attempts are open.
	if _, ok, err := s.OpenFenceForSubject(ctx, subject); err != nil || ok {
		t.Fatalf("exclusive subject open=%v err=%v, want it unheld while only read-only Attempts are open", ok, err)
	}
	writeAdmission := admissionFor(outcomeID, plan, "rk-write-1", subject)
	if _, err := s.CreateAttemptWithFence(ctx, writeAdmission); err != nil {
		t.Fatalf("write-capable admission must succeed while only read-only fences are open: %v", err)
	}

	// A second write-capable admission against the same exclusive subject
	// still conflicts, exactly as today.
	var fenced *ports.AttemptFenceHeldError
	_, err = s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-write-2", subject))
	if !errors.As(err, &fenced) {
		t.Fatalf("second write-capable admission must still fail with AttemptFenceHeldError, got %v", err)
	}
}

func TestAttemptStore_AdmissionBindsRunIntentGenerationAndRejectsPauseWinner(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	plan, outcomeID := seedApprovedPlan(t, s, "attempt-run-intent")
	started, err := s.AppendRunIntent(ctx, commandedRunIntent(outcomeID, plan.ID, domain.RunIntentRunning, domain.RunCommandStart, 0, "run-start", "start/attempt"))
	if err != nil {
		t.Fatalf("append start: %v", err)
	}

	admission := admissionFor(outcomeID, plan, "attempt-before-pause", domain.FenceSubjectForProject("attempt-run-intent"))
	admission.RunIntentGeneration = started.Generation
	attempt, err := s.CreateAttemptWithFence(ctx, admission)
	if err != nil {
		t.Fatalf("admit running attempt: %v", err)
	}
	if attempt.RunIntentGeneration != started.Generation {
		t.Fatalf("attempt run generation = %d, want %d", attempt.RunIntentGeneration, started.Generation)
	}

	paused, err := s.AppendRunIntent(ctx, commandedRunIntent(outcomeID, plan.ID, domain.RunIntentPaused, domain.RunCommandPause, started.Generation, "run-pause", "pause/attempt"))
	if err != nil {
		t.Fatalf("append pause: %v", err)
	}

	stale := admissionFor(outcomeID, plan, "attempt-after-pause", "different-fence")
	stale.RunIntentGeneration = started.Generation
	_, err = s.CreateAttemptWithFence(ctx, stale)
	var conflict *ports.AttemptRunIntentConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("stale admission = %v, want AttemptRunIntentConflictError", err)
	}
	if conflict.Current != paused.Generation || conflict.Desired != domain.RunIntentPaused {
		t.Fatalf("conflict = %+v, want paused generation %d", conflict, paused.Generation)
	}
	attempts, err := s.ListAttempts(ctx, outcomeID)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("stale admission created %d attempts, want the original one only", len(attempts))
	}
}

func TestAttemptStore_ReusedRequestKeyWithDifferentOutcomeConflicts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	firstPlan, firstOutcome := seedApprovedPlan(t, s, "replay-first")
	secondPlan, secondOutcome := seedApprovedPlan(t, s, "replay-second")
	key := "same-request-key"
	first, err := s.CreateAttemptWithFence(ctx, admissionFor(firstOutcome, firstPlan, key, domain.FenceSubjectForProject("replay-first")))
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateAttemptWithFence(ctx, admissionFor(secondOutcome, secondPlan, key, domain.FenceSubjectForProject("replay-second")))
	var conflict *ports.AttemptReplayConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %v, want AttemptReplayConflictError", err)
	}
	if second.ID != first.ID || conflict.Attempt.ID != first.ID {
		t.Fatalf("conflict attempt = %s/%s, want original %s", second.ID, conflict.Attempt.ID, first.ID)
	}
	if got, err := s.ListAttempts(ctx, secondOutcome); err != nil {
		t.Fatal(err)
	} else if len(got) != 0 {
		t.Fatalf("conflicting replay created %d attempts for second outcome", len(got))
	}
}

func TestAttemptStore_ConcurrentIdenticalAdmissionReturnsOneAttempt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	plan, outcomeID := seedApprovedPlan(t, s, "replay-concurrent")
	admission := admissionFor(outcomeID, plan, "concurrent-request-key", domain.FenceSubjectForProject("replay-concurrent"))
	results := make(chan struct {
		attempt domain.Attempt
		err     error
	}, 2)
	for range 2 {
		go func() {
			attempt, err := s.CreateAttemptWithFence(ctx, admission)
			results <- struct {
				attempt domain.Attempt
				err     error
			}{attempt: attempt, err: err}
		}()
	}
	var winner domain.Attempt
	for range 2 {
		result := <-results
		if result.err == nil {
			winner = result.attempt
			continue
		}
		var replay *ports.AttemptReplayError
		if !errors.As(result.err, &replay) {
			t.Fatalf("concurrent result err = %v, want replay", result.err)
		}
		if replay.Attempt.ID == "" {
			t.Fatal("replay omitted canonical attempt")
		}
	}
	if winner.ID == "" {
		t.Fatal("no admission winner")
	}
	if attempts, err := s.ListAttempts(ctx, outcomeID); err != nil {
		t.Fatal(err)
	} else if len(attempts) != 1 || attempts[0].ID != winner.ID {
		t.Fatalf("attempts = %+v, want one winner %s", attempts, winner.ID)
	}
}

// TestAttemptStore_GuardedTransitionsAndSessionRefs covers the trigger-backed
// lifecycle seam, FK-free session refs, and ordered observations.
func TestAttemptStore_GuardedTransitionsAndSessionRefs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	plan, outcomeID := seedApprovedPlan(t, s, "mer")
	at, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-att-transitions", domain.FenceSubjectForProject("mer")))
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}

	// Wrong expectation mutates nothing.
	rows, err := s.TransitionAttemptStatus(ctx, outcomeID, at.ID, domain.AttemptRunning, domain.AttemptPaused, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("stale-guard transition affected %d rows, want 0", rows)
	}

	now := time.Now().UTC()
	for _, hop := range []domain.AttemptStatus{domain.AttemptRunning, domain.AttemptPaused} {
		expected := domain.AttemptQueued
		if hop == domain.AttemptPaused {
			expected = domain.AttemptRunning
		}
		rows, err := s.TransitionAttemptStatus(ctx, outcomeID, at.ID, expected, hop, now)
		if err != nil || rows != 1 {
			t.Fatalf("transition to %s: rows=%d err=%v", hop, rows, err)
		}
	}

	// The database itself refuses an illegal transition even when the guard
	// matches: paused -> succeeded is not a legal edge.
	_, err = s.TransitionAttemptStatus(ctx, outcomeID, at.ID, domain.AttemptPaused, domain.AttemptSucceeded, now)
	if err == nil || !strings.Contains(err.Error(), "illegal attempt status transition") {
		t.Fatalf("paused -> succeeded must abort at the trigger, got %v", err)
	}

	ref, err := s.BindAttemptSession(ctx, domain.AttemptSessionRef{
		AttemptID:              at.ID,
		SessionID:              "provider-session-1",
		Harness:                domain.HarnessCodex,
		Mode:                   domain.SessionModeTUI,
		RunBriefCoreDigest:     strings.Repeat("ab", 32),
		RunBriefCompiledDigest: strings.Repeat("cd", 32),
		AdmissionSnapshot:      `{"snapshotVersion":1}`,
	})
	if err != nil {
		t.Fatalf("bind session: %v", err)
	}
	if ref.Seq != 1 {
		t.Fatalf("first ref seq = %d, want 1", ref.Seq)
	}
	latest, ok, err := s.LatestAttemptSessionRef(ctx, at.ID)
	if err != nil || !ok || latest.SessionID != "provider-session-1" {
		t.Fatalf("latest ref ok=%v err=%v", ok, err)
	}
	bySession, ok, err := s.LatestAttemptSessionRefForSession(ctx, "provider-session-1")
	if err != nil || !ok || bySession.AttemptID != at.ID {
		t.Fatalf("latest ref by session ok=%v err=%v attempt=%s", ok, err, bySession.AttemptID)
	}

	firstObs, err := s.AppendAttemptObservation(ctx, at.ID, domain.ObservationOwnerPause, `{"by":"owner"}`, now)
	if err != nil {
		t.Fatalf("append observation: %v", err)
	}
	secondObs, err := s.AppendAttemptObservation(ctx, at.ID, domain.ObservationProviderExit, `{}`, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if secondObs.Seq != firstObs.Seq+1 {
		t.Fatalf("observation seqs must be monotonic: %d then %d", firstObs.Seq, secondObs.Seq)
	}

	projectID, ok, err := s.GetOutcomeProjectID(ctx, outcomeID)
	if err != nil || !ok || projectID != "mer" {
		t.Fatalf("outcome project = (%q, %v) err=%v", projectID, ok, err)
	}
}

// TestAttemptStore_ReconcileReleasesCustodyForReplacement walks the recovery
// path: reconcile releases the old fence with a reason, records receipts, and
// only then may a replacement attempt acquire the subject.
func TestAttemptStore_ReconcileReleasesCustodyForReplacement(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	plan, outcomeID := seedApprovedPlan(t, s, "mer")
	subject := domain.FenceSubjectForProject("mer")
	at, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-a1", subject))
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}

	if _, err := s.ReleaseFenceForAttempt(ctx, at.ID, "", time.Now()); err == nil {
		t.Fatal("release without reason must be refused")
	}
	rows, err := s.ReleaseFenceForAttempt(ctx, at.ID, "replacement_attempt", time.Now())
	if err != nil || rows != 1 {
		t.Fatalf("release rows=%d err=%v", rows, err)
	}
	rows, err = s.ReleaseFenceForAttempt(ctx, at.ID, "again", time.Now())
	if err != nil || rows != 0 {
		t.Fatalf("second release rows=%d err=%v, want 0", rows, err)
	}

	if err := s.CreateRecoveryReceipt(ctx, domain.AttemptRecoveryReceipt{
		ID:         "rcpt-lost",
		AttemptID:  at.ID,
		Resolution: domain.RecoveryReplacement,
		Detail:     `{"evidence":"heartbeat missing"}`,
	}); err != nil {
		t.Fatalf("record receipt: %v", err)
	}
	receipts, err := s.ListRecoveryReceipts(ctx, at.ID)
	if err != nil || len(receipts) != 1 {
		t.Fatalf("receipts len=%d err=%v", len(receipts), err)
	}

	// Custody handover: the replacement is always a NEW attempt row.
	replacement, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-a2", subject))
	if err != nil {
		t.Fatalf("replacement admission: %v", err)
	}
	if replacement.Number != 2 || replacement.ID == at.ID {
		t.Fatalf("replacement = %+v, want new row #2", replacement)
	}
	observations, err := s.ListAttemptObservations(ctx, at.ID)
	if err != nil {
		t.Fatal(err)
	}
	_ = observations
	// Stale-attempt inertness (D5): observations remain INSERTABLE on the
	// terminal predecessor after replacement — inspectable history that never
	// touches current truth.
	if _, err := s.AppendAttemptObservation(ctx, at.ID, "late_stale_report", `{}`, time.Now()); err != nil {
		t.Fatalf("stale observation must stay insertable: %v", err)
	}
	all, err := s.ListAttempts(ctx, outcomeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("lineage must hold both attempts, got %d", len(all))
	}
}

func TestFailAttemptBeforeLaunch_AtomicallyTerminalizesAndReleasesForFreshGeneration(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	plan, outcomeID := seedApprovedPlan(t, s, "prelaunch-release")
	subject := domain.FenceSubjectForProject("prelaunch-release")
	started, err := s.AppendRunIntent(ctx, commandedRunIntent(outcomeID, plan.ID, domain.RunIntentRunning, domain.RunCommandStart, 0, "run-start-prelaunch", "start/prelaunch"))
	if err != nil {
		t.Fatal(err)
	}
	firstAdmission := admissionFor(outcomeID, plan, "attempt-prelaunch-1", subject)
	firstAdmission.RunIntentGeneration = started.Generation
	first, err := s.CreateAttemptWithFence(ctx, firstAdmission)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FailAttemptBeforeLaunch(ctx, ports.AttemptPrelaunchFailure{
		OutcomeID: outcomeID, AttemptID: first.ID, ObservationKind: domain.ObservationAdmissionFailed,
		ObservationPayload: `{"providerLaunched":false}`, ReleaseReason: "provider_not_launched", At: time.Unix(200, 0).UTC(),
	}); err != nil {
		t.Fatalf("fail before launch: %v", err)
	}
	stored, found, err := s.GetAttempt(ctx, outcomeID, first.ID)
	if err != nil || !found || stored.Status != domain.AttemptFailed {
		t.Fatalf("attempt = %+v found=%v err=%v, want failed", stored, found, err)
	}
	if _, open, err := s.OpenFenceForSubject(ctx, subject); err != nil || open {
		t.Fatalf("fence open=%v err=%v, want released", open, err)
	}
	paused, err := s.AppendRunIntent(ctx, commandedRunIntent(outcomeID, plan.ID, domain.RunIntentPaused, domain.RunCommandPause, started.Generation, "run-pause-prelaunch", "pause/prelaunch"))
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := s.AppendRunIntent(ctx, commandedRunIntent(outcomeID, plan.ID, domain.RunIntentRunning, domain.RunCommandResume, paused.Generation, "run-resume-prelaunch", "resume/prelaunch"))
	if err != nil {
		t.Fatal(err)
	}
	secondAdmission := admissionFor(outcomeID, plan, "attempt-prelaunch-2", subject)
	secondAdmission.RunIntentGeneration = resumed.Generation
	second, err := s.CreateAttemptWithFence(ctx, secondAdmission)
	if err != nil {
		t.Fatalf("fresh generation/key admission: %v", err)
	}
	if second.ID == first.ID || second.Number != 2 {
		t.Fatalf("second = %+v, want distinct attempt #2", second)
	}
}

func TestFailAttemptBeforeLaunch_RollsBackAllFactsWhenTerminalizationLosesItsGuard(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	plan, outcomeID := seedApprovedPlan(t, s, "prelaunch-rollback")
	subject := domain.FenceSubjectForProject("prelaunch-rollback")
	attempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "attempt-prelaunch-rollback", subject))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.FailAttemptBeforeLaunch(ctx, ports.AttemptPrelaunchFailure{
		OutcomeID: "wrong-outcome", AttemptID: attempt.ID, ObservationKind: domain.ObservationAdmissionFailed,
		ObservationPayload: `{"providerLaunched":false}`, ReleaseReason: "provider_not_launched", At: time.Unix(300, 0).UTC(),
	})
	if err == nil {
		t.Fatal("mismatched lineage must fail the atomic operation")
	}
	observations, err := s.ListAttemptObservations(ctx, attempt.ID)
	if err != nil || len(observations) != 0 {
		t.Fatalf("observations = %+v err=%v, want rollback", observations, err)
	}
	if fence, open, err := s.OpenFenceForSubject(ctx, subject); err != nil || !open || fence.AttemptID != attempt.ID {
		t.Fatalf("fence = %+v open=%v err=%v, want original custody intact", fence, open, err)
	}
}

func TestTerminateRunningAttemptWithObservation_AtomicallyFailsAndReleasesCustody(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	plan, outcomeID := seedApprovedPlan(t, s, "running-fail")
	subject := domain.FenceSubjectForProject("running-fail")
	attempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "attempt-running-fail", subject))
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := s.TransitionAttemptStatus(ctx, outcomeID, attempt.ID, domain.AttemptQueued, domain.AttemptRunning, time.Now().UTC()); err != nil || rows != 1 {
		t.Fatalf("promote to running: rows=%d err=%v", rows, err)
	}

	obs, applied, err := s.TerminateRunningAttemptWithObservation(ctx, ports.AttemptRunningTermination{
		OutcomeID: outcomeID, AttemptID: attempt.ID, TargetStatus: domain.AttemptFailed,
		ObservationKind: domain.ObservationProviderExit, ObservationPayload: `{"exitCode":17}`,
		ReleaseReason: "governed_provider_process_failed", At: time.Unix(400, 0).UTC(),
	})
	if err != nil || !applied {
		t.Fatalf("terminate running attempt: applied=%v err=%v", applied, err)
	}
	if obs.AttemptID != attempt.ID || obs.Kind != domain.ObservationProviderExit {
		t.Fatalf("observation = %+v, want provider-exit observation for %s", obs, attempt.ID)
	}
	stored, found, err := s.GetAttempt(ctx, outcomeID, attempt.ID)
	if err != nil || !found || stored.Status != domain.AttemptFailed {
		t.Fatalf("attempt = %+v found=%v err=%v, want failed", stored, found, err)
	}
	if _, open, err := s.OpenFenceForSubject(ctx, subject); err != nil || open {
		t.Fatalf("fence open=%v err=%v, want released", open, err)
	}
	observations, err := s.ListAttemptObservations(ctx, attempt.ID)
	if err != nil || len(observations) != 1 {
		t.Fatalf("observations = %+v err=%v, want exactly one", observations, err)
	}
}

// TestTerminateRunningAttemptWithObservation_ReconciledLeavesCustodyUntouched
// covers the unclassified branch: an empty ReleaseReason must never touch
// custody, even though the attempt still terminalizes and records its
// observation.
func TestTerminateRunningAttemptWithObservation_ReconciledLeavesCustodyUntouched(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	plan, outcomeID := seedApprovedPlan(t, s, "running-reconcile")
	subject := domain.FenceSubjectForProject("running-reconcile")
	attempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "attempt-running-reconcile", subject))
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := s.TransitionAttemptStatus(ctx, outcomeID, attempt.ID, domain.AttemptQueued, domain.AttemptRunning, time.Now().UTC()); err != nil || rows != 1 {
		t.Fatalf("promote to running: rows=%d err=%v", rows, err)
	}

	_, applied, err := s.TerminateRunningAttemptWithObservation(ctx, ports.AttemptRunningTermination{
		OutcomeID: outcomeID, AttemptID: attempt.ID, TargetStatus: domain.AttemptReconciled,
		ObservationKind: domain.ObservationProviderExit, ObservationPayload: `{}`, At: time.Unix(500, 0).UTC(),
	})
	if err != nil || !applied {
		t.Fatalf("terminate running attempt: applied=%v err=%v", applied, err)
	}
	stored, found, err := s.GetAttempt(ctx, outcomeID, attempt.ID)
	if err != nil || !found || stored.Status != domain.AttemptReconciled {
		t.Fatalf("attempt = %+v found=%v err=%v, want reconciled", stored, found, err)
	}
	if fence, open, err := s.OpenFenceForSubject(ctx, subject); err != nil || !open || fence.AttemptID != attempt.ID {
		t.Fatalf("fence = %+v open=%v err=%v, want custody still held for unclassified reconciliation", fence, open, err)
	}
}

// TestTerminateRunningAttemptWithObservation_NoOpWhenAlreadyMoved covers the
// concurrent-mover case: an attempt no longer Running (e.g. a competing
// reconciler already committed) must produce no observation and no custody
// change, and must not surface as an error — the caller's next tick observes
// whatever the durable winner left behind.
func TestTerminateRunningAttemptWithObservation_NoOpWhenAlreadyMoved(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	plan, outcomeID := seedApprovedPlan(t, s, "running-noop")
	subject := domain.FenceSubjectForProject("running-noop")
	attempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "attempt-running-noop", subject))
	if err != nil {
		t.Fatal(err)
	}
	// Still Queued, never promoted to Running: TerminateRunningAttemptWithObservation
	// only ever matches a Running row.

	obs, applied, err := s.TerminateRunningAttemptWithObservation(ctx, ports.AttemptRunningTermination{
		OutcomeID: outcomeID, AttemptID: attempt.ID, TargetStatus: domain.AttemptFailed,
		ObservationKind: domain.ObservationProviderExit, ObservationPayload: `{}`,
		ReleaseReason: "governed_provider_process_failed", At: time.Unix(600, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("no-op must not error: %v", err)
	}
	if applied {
		t.Fatal("applied = true, want false: attempt was never running")
	}
	if obs.ID != "" {
		t.Fatalf("observation = %+v, want zero value", obs)
	}
	stored, found, err := s.GetAttempt(ctx, outcomeID, attempt.ID)
	if err != nil || !found || stored.Status != domain.AttemptQueued {
		t.Fatalf("attempt = %+v found=%v err=%v, want untouched queued", stored, found, err)
	}
	if fence, open, err := s.OpenFenceForSubject(ctx, subject); err != nil || !open || fence.AttemptID != attempt.ID {
		t.Fatalf("fence = %+v open=%v err=%v, want untouched custody", fence, open, err)
	}
	observations, err := s.ListAttemptObservations(ctx, attempt.ID)
	if err != nil || len(observations) != 0 {
		t.Fatalf("observations = %+v err=%v, want none", observations, err)
	}
}

// TestAttemptStore_FenceLeaseRenewal pins the renewable-lease facts: renewal
// refreshes only OPEN fences for the custodian, and a released fence freezes
// forever (trigger-refused).
func TestAttemptStore_FenceLeaseRenewal(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	plan, outcomeID := seedApprovedPlan(t, s, "mer")
	subject := domain.FenceSubjectForProject("mer")
	at, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-lease", subject))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	t0 := time.Now().UTC().Add(-time.Hour)
	if rows, err := s.RenewFenceForAttempt(ctx, at.ID, t0); err != nil || rows != 1 {
		t.Fatalf("renew rows=%d err=%v", rows, err)
	}
	fence, ok, err := s.OpenFenceForSubject(ctx, subject)
	if err != nil || !ok {
		t.Fatalf("fence ok=%v err=%v", ok, err)
	}
	if !fence.LastRenewedAt.Equal(t0.Truncate(time.Second)) && fence.LastRenewedAt.Sub(t0).Abs() > time.Second {
		t.Fatalf("lastRenewedAt = %v, want ~%v", fence.LastRenewedAt, t0)
	}

	if _, err := s.ReleaseFenceForAttempt(ctx, at.ID, "replacement_attempt", time.Now()); err != nil {
		t.Fatal(err)
	}
	if rows, err := s.RenewFenceForAttempt(ctx, at.ID, time.Now()); err != nil || rows != 0 {
		t.Fatalf("post-release renewal rows=%d err=%v, want 0", rows, err)
	}
}
