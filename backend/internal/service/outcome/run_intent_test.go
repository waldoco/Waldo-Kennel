package outcome_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/intelligence/intelligencetest"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

// runHarness is an approved Outcome with durable run intent wired and no
// Attempt started yet.
type runHarness struct {
	svc       *outcome.Service
	store     *attemptFakeStore
	spawner   *fakeSpawner
	intents   *runIntentFakeStore
	outcomeID domain.OutcomeID
	planID    domain.PlanRevisionID
}

func newRunHarness(t *testing.T) *runHarness {
	t.Helper()
	svc, store, spawner, _, outcomeID, planID := newAttemptHarness(t)
	intents := newRunIntentFakeStore()
	svc = svc.WithRunIntents(intents)

	plan, err := svc.GetLatestPlan(context.Background(), outcomeID)
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	rememberFirstWorkUnit(plan.Plan)
	return &runHarness{svc: svc, store: store, spawner: spawner, intents: intents, outcomeID: outcomeID, planID: planID}
}

func (h *runHarness) command(t *testing.T, command domain.RunCommand, key string) (outcome.RunStateView, error) {
	t.Helper()
	return h.svc.CommandRun(context.Background(), h.outcomeID, outcome.RunCommandInput{
		Command: command, PlanRevisionID: h.planID, ExpectedContractRevision: 1, RequestKey: key,
	})
}

func (h *runHarness) mustCommand(t *testing.T, command domain.RunCommand, key string) outcome.RunStateView {
	t.Helper()
	view, err := h.command(t, command, key)
	if err != nil {
		t.Fatalf("%s: %v", command, err)
	}
	return view
}

// TestCommandRun_StartAuthorizesWithoutLaunching keeps authorization and
// execution separate. Start records intent; the daemon admits work.
func TestCommandRun_StartAuthorizesWithoutLaunching(t *testing.T) {
	h := newRunHarness(t)
	view := h.mustCommand(t, domain.RunCommandStart, "rk-start")

	if view.Intent == nil || view.Intent.Desired != string(domain.RunIntentRunning) {
		t.Fatalf("intent = %+v, want running", view.Intent)
	}
	if view.Intent.Generation != 1 {
		t.Fatalf("generation = %d, want the first", view.Intent.Generation)
	}
	if calls := h.spawner.spawnCalls(); calls != 0 {
		t.Fatalf("recording intent launched %d providers; authorization is not execution", calls)
	}
}

// TestCommandRun_ARepeatedCommandAuthorizesOnce is the replay guard a double
// click and a reconnect retry both land on.
func TestCommandRun_ARepeatedCommandAuthorizesOnce(t *testing.T) {
	h := newRunHarness(t)
	first := h.mustCommand(t, domain.RunCommandStart, "rk-start")
	second := h.mustCommand(t, domain.RunCommandStart, "rk-start")

	if first.Intent.Generation != second.Intent.Generation {
		t.Fatalf("generations = %d and %d, want the replay to return the first",
			first.Intent.Generation, second.Intent.Generation)
	}
	if got := h.intents.generations(h.outcomeID); got != 1 {
		t.Fatalf("authorization history has %d generations, want one", got)
	}
}

// TestCommandRun_RefusesCommandsThatDoNotApply keeps the vocabulary honest:
// resuming a cancelled run would restart work the owner ended, and a fresh
// Start is the honest way to ask for more.
func TestCommandRun_RefusesCommandsThatDoNotApply(t *testing.T) {
	h := newRunHarness(t)

	if _, err := h.command(t, domain.RunCommandPause, "rk-pause-idle"); requireAPICode(t, err) != outcome.CodeRunActionUnavailable {
		t.Fatalf("pausing an idle Outcome = %v, want RUN_ACTION_UNAVAILABLE", err)
	}
	h.mustCommand(t, domain.RunCommandStart, "rk-start")
	if _, err := h.command(t, domain.RunCommandResume, "rk-resume-running"); requireAPICode(t, err) != outcome.CodeRunActionUnavailable {
		t.Fatalf("resuming a running Outcome = %v, want RUN_ACTION_UNAVAILABLE", err)
	}
	h.mustCommand(t, domain.RunCommandCancel, "rk-cancel")
	if _, err := h.command(t, domain.RunCommandResume, "rk-resume-cancelled"); requireAPICode(t, err) != outcome.CodeRunActionUnavailable {
		t.Fatalf("resuming a cancelled run = %v, want RUN_ACTION_UNAVAILABLE", err)
	}
	// Start after a cancellation is allowed: it is a new authorization.
	if _, err := h.command(t, domain.RunCommandStart, "rk-restart"); err != nil {
		t.Fatalf("start after cancel: %v", err)
	}
}

// TestCommandRun_RefusesAStaleGeneration stops a command composed against
// state the owner has already moved past.
func TestCommandRun_RefusesAStaleGeneration(t *testing.T) {
	h := newRunHarness(t)
	h.mustCommand(t, domain.RunCommandStart, "rk-start")
	h.mustCommand(t, domain.RunCommandPause, "rk-pause")

	_, err := h.svc.CommandRun(context.Background(), h.outcomeID, outcome.RunCommandInput{
		Command: domain.RunCommandCancel, PlanRevisionID: h.planID,
		ExpectedContractRevision: 1, ExpectedGeneration: 1, RequestKey: "rk-stale",
	})
	if requireAPICode(t, err) != outcome.CodeRunIntentStale {
		t.Fatalf("stale command = %v, want RUN_INTENT_STALE", err)
	}
}

// TestRunIntent_PauseStopsSubsequentAdmission is the behaviour that makes a
// pause a pause: it applies to every admission path, including a direct
// per-Attempt Start.
func TestRunIntent_PauseStopsSubsequentAdmission(t *testing.T) {
	h := newRunHarness(t)
	h.mustCommand(t, domain.RunCommandStart, "rk-start")
	h.mustCommand(t, domain.RunCommandPause, "rk-pause")

	_, err := h.svc.StartAttempt(context.Background(), h.outcomeID, startInput(h.planID))
	if requireAPICode(t, err) != outcome.CodeRunActionUnavailable {
		t.Fatalf("start while paused = %v, want it refused", err)
	}
	if calls := h.spawner.spawnCalls(); calls != 0 {
		t.Fatalf("a paused Outcome launched %d providers", calls)
	}

	// Resuming makes work admissible again.
	h.mustCommand(t, domain.RunCommandResume, "rk-resume")
	if _, err := h.svc.StartAttempt(context.Background(), h.outcomeID, startInput(h.planID)); err != nil {
		t.Fatalf("start after resume: %v", err)
	}
}

// TestRunIntent_PauseWithNoActiveWorkIsAcknowledgedImmediately separates the
// request from its effect. With nothing running, nothing can follow, so the
// pause has already taken effect.
func TestRunIntent_PauseWithNoActiveWorkIsAcknowledgedImmediately(t *testing.T) {
	h := newRunHarness(t)
	h.mustCommand(t, domain.RunCommandStart, "rk-start")
	view := h.mustCommand(t, domain.RunCommandPause, "rk-pause")

	if view.Intent == nil || view.Intent.AcknowledgedAt == nil {
		t.Fatalf("intent = %+v, want an acknowledged pause", view.Intent)
	}
}

// TestContinueAuthorizedRuns_AdmitsEachEligibleWorkUnitOnce is serial
// continuation. Repeated ticks must converge on one Attempt, not accumulate.
func TestContinueAuthorizedRuns_AdmitsEachEligibleWorkUnitOnce(t *testing.T) {
	h := newRunHarness(t)
	h.mustCommand(t, domain.RunCommandStart, "rk-start")
	ctx := context.Background()

	for tick := 0; tick < 3; tick++ {
		if err := h.svc.ContinueAuthorizedRuns(ctx); err != nil {
			t.Fatalf("tick %d: %v", tick, err)
		}
	}
	attempts, err := h.store.ListAttempts(ctx, h.outcomeID)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("attempts = %d after three ticks, want exactly one", len(attempts))
	}
	if calls := h.spawner.spawnCalls(); calls != 1 {
		t.Fatalf("providers launched = %d, want one", calls)
	}
}

func TestContinueAuthorizedRuns_ReplacementDecisionMintsOneFreshAttempt(t *testing.T) {
	h := newRunHarness(t)
	h.mustCommand(t, domain.RunCommandStart, "rk-start")
	ctx := context.Background()
	if err := h.svc.ContinueAuthorizedRuns(ctx); err != nil {
		t.Fatalf("initial continuation: %v", err)
	}
	attempts, err := h.store.ListAttempts(ctx, h.outcomeID)
	if err != nil || len(attempts) != 1 {
		t.Fatalf("initial attempts = %+v err=%v", attempts, err)
	}
	first := attempts[0]
	if _, err := h.store.TransitionAttemptStatus(ctx, h.outcomeID, first.ID, domain.AttemptRunning, domain.AttemptFailed, time.Now()); err != nil {
		t.Fatalf("record failed predecessor: %v", err)
	}
	// A terminal failure alone is not blanket authorization to retry forever.
	if err := h.svc.ContinueAuthorizedRuns(ctx); err != nil {
		t.Fatalf("continuation before replacement decision: %v", err)
	}
	if calls := h.spawner.spawnCalls(); calls != 1 {
		t.Fatalf("providers launched before replacement decision = %d, want one", calls)
	}
	view, err := h.svc.GetRunState(ctx, h.outcomeID)
	if err != nil {
		t.Fatalf("run state before replacement decision: %v", err)
	}
	if view.State != outcome.MissionNeedsYou || view.AttentionReason != outcome.ReasonAttemptReplacementRequired {
		t.Fatalf("run state before replacement = %s/%s, want needs_you/%s", view.State, view.AttentionReason, outcome.ReasonAttemptReplacementRequired)
	}
	if _, err := h.svc.RecoverAttempt(ctx, h.outcomeID, first.ID, outcome.RecoveryInput{
		Action: outcome.RecoveryActionReplace, ConfirmProviderStopped: true,
	}); err != nil {
		t.Fatalf("authorize replacement: %v", err)
	}
	view, err = h.svc.GetRunState(ctx, h.outcomeID)
	if err != nil {
		t.Fatalf("run state after replacement decision: %v", err)
	}
	if view.State != outcome.MissionInProgress {
		t.Fatalf("run state after replacement authorization = %s, want in_progress", view.State)
	}
	for tick := 0; tick < 3; tick++ {
		if err := h.svc.ContinueAuthorizedRuns(ctx); err != nil {
			t.Fatalf("replacement tick %d: %v", tick, err)
		}
	}
	attempts, err = h.store.ListAttempts(ctx, h.outcomeID)
	if err != nil || len(attempts) != 2 {
		t.Fatalf("replacement attempts = %+v err=%v, want two", attempts, err)
	}
	if attempts[1].Number != 2 || attempts[1].RequestKey == first.RequestKey {
		t.Fatalf("replacement = %+v, want fresh attempt #2 and replay identity", attempts[1])
	}
	if calls := h.spawner.spawnCalls(); calls != 2 {
		t.Fatalf("providers launched after repeated ticks = %d, want exactly two", calls)
	}
}

func TestContinueAuthorizedRuns_ReadinessFailureIsActionableAndNotBlindlyRetried(t *testing.T) {
	h := newRunHarness(t)
	h.spawner.setReadiness(ports.AgentProfileReadiness{Ready: false, Detail: "Sign in to the selected Codex profile"})
	h.mustCommand(t, domain.RunCommandStart, "rk-start")
	ctx := context.Background()

	if err := h.svc.ContinueAuthorizedRuns(ctx); err != nil {
		t.Fatalf("first continuation: %v", err)
	}
	view, err := h.svc.GetRunState(ctx, h.outcomeID)
	if err != nil {
		t.Fatalf("run state: %v", err)
	}
	if view.State != outcome.MissionNeedsYou || view.Blocker == nil || view.Blocker.Code != outcome.CodeAgentProfileNotReady {
		t.Fatalf("state = %q blocker=%+v, want an actionable readiness blocker", view.State, view.Blocker)
	}
	if view.Intent == nil || !strings.Contains(view.Intent.LastError, "not ready") {
		t.Fatalf("intent = %+v, want persisted last error", view.Intent)
	}
	if err := h.svc.ContinueAuthorizedRuns(ctx); err != nil {
		t.Fatalf("second continuation: %v", err)
	}
	if calls := h.spawner.readinessCalls(); calls != 1 {
		t.Fatalf("readiness probes = %d, want one until deliberate re-authorization", calls)
	}

	h.mustCommand(t, domain.RunCommandPause, "rk-pause")
	h.spawner.setReadiness(ports.AgentProfileReadiness{Ready: true})
	h.mustCommand(t, domain.RunCommandResume, "rk-resume")
	if err := h.svc.ContinueAuthorizedRuns(ctx); err != nil {
		t.Fatalf("continuation after deliberate retry: %v", err)
	}
	if calls := h.spawner.spawnCalls(); calls != 2 {
		t.Fatalf("workspace preparations = %d, want failed generation plus the new generation", calls)
	}
}

func TestContinueAuthorizedRuns_ReconstructsPrelaunchBlockerAfterRestart(t *testing.T) {
	h := newRunHarness(t)
	h.mustCommand(t, domain.RunCommandStart, "rk-start")
	ctx := context.Background()
	unitID := firstWorkUnitOfPlan[h.planID]
	requestKey := fmt.Sprintf("run:%s:%d:%s", h.outcomeID, 1, unitID)
	h.spawner.failNextSpawn(&ports.AttemptPrelaunchError{
		Stage: "prepare_tui_launch",
		Err:   errors.New("launch command could not be prepared"),
	})
	_, err := h.svc.StartAttempt(ctx, h.outcomeID, outcome.StartAttemptInput{
		PlanRevisionID: h.planID,
		WorkUnitID:     unitID,
		RequestKey:     requestKey,
	})
	if requireAPICode(t, err) != outcome.CodeAttemptPrelaunchFailed {
		t.Fatalf("prelaunch failure = %v, want %s", err, outcome.CodeAttemptPrelaunchFailed)
	}
	intent, found, err := h.intents.CurrentRunIntent(ctx, h.outcomeID)
	if err != nil || !found || intent.AdmissionFailure != nil {
		t.Fatalf("pre-crash intent = %+v found=%v err=%v, want failure not yet recorded", intent, found, err)
	}

	// Simulate a daemon restart after FailAttemptBeforeLaunch committed but
	// before continuation copied its typed blocker onto the run intent.
	restarted := outcome.New(h.store, nil).
		WithPlanning(intelligencetest.New(), &routingInventoryFake{candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessCodex, "")}}).
		WithExecution(h.spawner, newFakeHeartbeats()).
		WithRunIntents(h.intents)
	restarted.AdmissionPolicy = testAdmissionPolicy()
	if err := restarted.ContinueAuthorizedRuns(ctx); err != nil {
		t.Fatalf("restart continuation: %v", err)
	}
	intent, found, err = h.intents.CurrentRunIntent(ctx, h.outcomeID)
	if err != nil || !found || intent.AdmissionFailure == nil {
		t.Fatalf("recovered intent = %+v found=%v err=%v, want durable blocker", intent, found, err)
	}
	if intent.AdmissionFailure.Code != outcome.CodeAttemptPrelaunchFailed || intent.AdmissionFailure.WorkUnitID != unitID {
		t.Fatalf("recovered blocker = %+v, want typed prelaunch failure for %s", intent.AdmissionFailure, unitID)
	}
	if calls := h.spawner.spawnCalls(); calls != 1 {
		t.Fatalf("provider spawn calls = %d, want the original failed call only", calls)
	}
	view, err := restarted.GetRunState(ctx, h.outcomeID)
	if err != nil {
		t.Fatalf("run state: %v", err)
	}
	if view.State != outcome.MissionNeedsYou || view.Blocker == nil || view.Blocker.Code != outcome.CodeAttemptPrelaunchFailed {
		t.Fatalf("state=%s blocker=%+v, want recovered Needs You blocker", view.State, view.Blocker)
	}
}

func TestContinueAuthorizedRuns_DoesNotCarryRecoveredFailureIntoNewerGeneration(t *testing.T) {
	h := newRunHarness(t)
	h.mustCommand(t, domain.RunCommandStart, "rk-start")
	ctx := context.Background()
	unitID := firstWorkUnitOfPlan[h.planID]
	h.spawner.failNextSpawn(&ports.AttemptPrelaunchError{
		Stage: "prepare_tui_launch",
		Err:   errors.New("launch command could not be prepared"),
	})
	_, err := h.svc.StartAttempt(ctx, h.outcomeID, outcome.StartAttemptInput{
		PlanRevisionID: h.planID,
		WorkUnitID:     unitID,
		RequestKey:     fmt.Sprintf("run:%s:%d:%s", h.outcomeID, 1, unitID),
	})
	if requireAPICode(t, err) != outcome.CodeAttemptPrelaunchFailed {
		t.Fatalf("prelaunch failure = %v, want %s", err, outcome.CodeAttemptPrelaunchFailed)
	}

	h.mustCommand(t, domain.RunCommandPause, "rk-pause")
	h.mustCommand(t, domain.RunCommandResume, "rk-resume")
	restarted := outcome.New(h.store, nil).
		WithPlanning(intelligencetest.New(), &routingInventoryFake{candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessCodex, "")}}).
		WithExecution(h.spawner, newFakeHeartbeats()).
		WithRunIntents(h.intents)
	restarted.AdmissionPolicy = testAdmissionPolicy()
	if err := restarted.ContinueAuthorizedRuns(ctx); err != nil {
		t.Fatalf("new-generation continuation: %v", err)
	}
	intent, found, err := h.intents.CurrentRunIntent(ctx, h.outcomeID)
	if err != nil || !found || intent.Generation != 3 || intent.AdmissionFailure != nil {
		t.Fatalf("current intent = %+v found=%v err=%v, want clean running generation 3", intent, found, err)
	}
	if calls := h.spawner.spawnCalls(); calls != 2 {
		t.Fatalf("provider spawn calls = %d, want failed generation 1 plus one generation 3 launch", calls)
	}
	if err := restarted.ContinueAuthorizedRuns(ctx); err != nil {
		t.Fatalf("repeated continuation: %v", err)
	}
	if calls := h.spawner.spawnCalls(); calls != 2 {
		t.Fatalf("provider spawn calls after repeated tick = %d, want no duplicate launch", calls)
	}
}

func TestRecordObservation_RejectsSystemOwnedPrelaunchKinds(t *testing.T) {
	h := newRunHarness(t)
	ctx := context.Background()
	attempt, err := h.svc.StartAttempt(ctx, h.outcomeID, startInput(h.planID))
	if err != nil {
		t.Fatalf("start attempt: %v", err)
	}
	for _, kind := range []string{domain.ObservationAdmissionFailed, domain.ObservationInputProvisioningFailed} {
		t.Run(kind, func(t *testing.T) {
			_, err := h.svc.RecordObservation(ctx, h.outcomeID, attempt.Attempt.ID, outcome.RecordObservationInput{
				Kind: kind, Payload: `{"admissionFailure":{"code":"forged"}}`,
			})
			if requireAPICode(t, err) != "OBSERVATION_KIND_RESERVED" {
				t.Fatalf("reserved observation = %v, want OBSERVATION_KIND_RESERVED", err)
			}
		})
	}
}

func TestContinueAuthorizedRuns_RejectsInvalidTypedPrelaunchFacts(t *testing.T) {
	for _, tc := range []struct {
		name          string
		rewrite       func(string, domain.WorkUnitID) string
		errorContains string
	}{
		{
			name: "malformed typed failure",
			rewrite: func(_ string, unitID domain.WorkUnitID) string {
				return fmt.Sprintf(`{"workUnitId":%q,"providerLaunched":false,"admissionFailure":"not-an-object"}`, unitID)
			},
			errorContains: "decode typed prelaunch observation",
		},
		{
			name: "mismatched work unit",
			rewrite: func(payload string, unitID domain.WorkUnitID) string {
				return strings.ReplaceAll(payload, string(unitID), "wu-other")
			},
			errorContains: "belongs to work unit",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newRunHarness(t)
			ctx, unitID, attemptID := leavePrelaunchFailureUncopied(t, h)
			h.store.mu.Lock()
			observation := h.store.obs[attemptID][0]
			observation.Payload = tc.rewrite(observation.Payload, unitID)
			h.store.obs[attemptID][0] = observation
			h.store.mu.Unlock()

			restarted := outcome.New(h.store, nil).
				WithExecution(h.spawner, newFakeHeartbeats()).
				WithRunIntents(h.intents)
			restarted.AdmissionPolicy = testAdmissionPolicy()
			err := restarted.ContinueAuthorizedRuns(ctx)
			if err == nil || !strings.Contains(err.Error(), tc.errorContains) {
				t.Fatalf("continuation error = %v, want %q", err, tc.errorContains)
			}
			intent, found, readErr := h.intents.CurrentRunIntent(ctx, h.outcomeID)
			if readErr != nil || !found || intent.AdmissionFailure != nil {
				t.Fatalf("intent = %+v found=%v err=%v, want no invented blocker", intent, found, readErr)
			}
			if calls := h.spawner.spawnCalls(); calls != 1 {
				t.Fatalf("provider spawn calls = %d, want no replay launch", calls)
			}
		})
	}
}

func TestContinueAuthorizedRuns_DoesNotInventFailureFromLegacyUntypedObservation(t *testing.T) {
	h := newRunHarness(t)
	ctx, unitID, attemptID := leavePrelaunchFailureUncopied(t, h)
	h.store.mu.Lock()
	observation := h.store.obs[attemptID][0]
	observation.Payload = fmt.Sprintf(`{"error":"legacy diagnostic only","workUnitId":%q,"providerLaunched":false}`, unitID)
	h.store.obs[attemptID][0] = observation
	h.store.mu.Unlock()

	restarted := outcome.New(h.store, nil).
		WithPlanning(intelligencetest.New(), &routingInventoryFake{candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessCodex, "")}}).
		WithExecution(h.spawner, newFakeHeartbeats()).
		WithRunIntents(h.intents)
	restarted.AdmissionPolicy = testAdmissionPolicy()
	if err := restarted.ContinueAuthorizedRuns(ctx); err != nil {
		t.Fatalf("legacy continuation: %v", err)
	}
	intent, found, err := h.intents.CurrentRunIntent(ctx, h.outcomeID)
	if err != nil || !found || intent.AdmissionFailure != nil {
		t.Fatalf("intent = %+v found=%v err=%v, want no failure inferred from legacy prose", intent, found, err)
	}
	if calls := h.spawner.spawnCalls(); calls != 1 {
		t.Fatalf("provider spawn calls = %d, want no replay launch", calls)
	}
}

func leavePrelaunchFailureUncopied(t *testing.T, h *runHarness) (context.Context, domain.WorkUnitID, domain.AttemptID) {
	t.Helper()
	h.mustCommand(t, domain.RunCommandStart, "rk-start")
	ctx := context.Background()
	unitID := firstWorkUnitOfPlan[h.planID]
	h.spawner.failNextSpawn(&ports.AttemptPrelaunchError{
		Stage: "prepare_tui_launch",
		Err:   errors.New("launch command could not be prepared"),
	})
	_, err := h.svc.StartAttempt(ctx, h.outcomeID, outcome.StartAttemptInput{
		PlanRevisionID: h.planID,
		WorkUnitID:     unitID,
		RequestKey:     fmt.Sprintf("run:%s:%d:%s", h.outcomeID, 1, unitID),
	})
	if requireAPICode(t, err) != outcome.CodeAttemptPrelaunchFailed {
		t.Fatalf("prelaunch failure = %v, want %s", err, outcome.CodeAttemptPrelaunchFailed)
	}
	attempts, err := h.store.ListAttempts(ctx, h.outcomeID)
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempts = %+v err=%v, want one failed attempt", attempts, err)
	}
	return ctx, unitID, attempts[0].ID
}

// TestContinueAuthorizedRuns_DoesNothingWhilePausedOrCancelled is the restart
// case: a reconciler coming up must read the current generation, not resume
// work the owner stopped.
func TestContinueAuthorizedRuns_DoesNothingWhilePausedOrCancelled(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command domain.RunCommand
		key     string
	}{
		{name: "paused", command: domain.RunCommandPause, key: "rk-pause"},
		{name: "cancelled", command: domain.RunCommandCancel, key: "rk-cancel"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newRunHarness(t)
			h.mustCommand(t, domain.RunCommandStart, "rk-start")
			h.mustCommand(t, tc.command, tc.key)

			if err := h.svc.ContinueAuthorizedRuns(context.Background()); err != nil {
				t.Fatalf("continue: %v", err)
			}
			if calls := h.spawner.spawnCalls(); calls != 0 {
				t.Fatalf("a %s run admitted %d Attempts", tc.name, calls)
			}
		})
	}
}

// TestRunState_ReportsTheCurrentAuthorizationAndItsActions keeps the Mission
// projection and the durable intent in agreement.
func TestRunState_ReportsTheCurrentAuthorizationAndItsActions(t *testing.T) {
	h := newRunHarness(t)
	h.mustCommand(t, domain.RunCommandStart, "rk-start")

	view, err := h.svc.GetRunState(context.Background(), h.outcomeID)
	if err != nil {
		t.Fatalf("run state: %v", err)
	}
	if view.Intent == nil || view.Intent.Desired != string(domain.RunIntentRunning) {
		t.Fatalf("intent = %+v, want running", view.Intent)
	}
	pause := runActionFor(view, outcome.RunActionPause)
	if !pause.Available {
		t.Fatalf("pause = %+v, want it offered while running", pause)
	}
	resume := runActionFor(view, outcome.RunActionResume)
	if resume.Available || resume.Reason != outcome.ReasonNoActiveRun {
		t.Fatalf("resume = %+v, want it refused while already running", resume)
	}
}

func runActionFor(view outcome.RunStateView, want outcome.RunAction) outcome.RunActionEligibility {
	for _, action := range view.EligibleActions {
		if action.Action == want {
			return action
		}
	}
	return outcome.RunActionEligibility{Action: want, Reason: "MISSING_FROM_VOCABULARY"}
}

// TestCommandRun_UnwiredRunIntentsReportUnavailable keeps a degraded daemon
// honest rather than pretending it authorized something.
func TestCommandRun_UnwiredRunIntentsReportUnavailable(t *testing.T) {
	store := newAttemptFakeStore()
	svc := outcome.New(store, func() time.Time { return time.Unix(100, 0).UTC() }).
		WithPlanning(intelligencetest.New(), &routingInventoryFake{candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessCodex, "")}}).
		WithExecution(&fakeSpawner{readiness: ports.AgentProfileReadiness{Ready: true}}, newFakeHeartbeats())

	if svc.RunIntentsEnabled() {
		t.Fatal("a daemon with no run-intent store reported the capability as available")
	}
	if err := svc.ContinueAuthorizedRuns(context.Background()); err != nil {
		t.Fatalf("continuation without run intent should be a no-op, got %v", err)
	}
}

// TestRunState_AuthorizedRunBetweenWorkUnitsDoesNotOfferAnotherStart is the
// KUX-002 regression. With running intent recorded and no Attempt yet admitted
// — the gap after Start and the gap between serial WorkUnits — the Mission
// projected needs_you/start_required and offered Start, a command CommandRun
// refuses because Start does not apply to an already-running intent.
func TestRunState_AuthorizedRunBetweenWorkUnitsDoesNotOfferAnotherStart(t *testing.T) {
	h := newRunHarness(t)
	h.mustCommand(t, domain.RunCommandStart, "rk-start")

	view, err := h.svc.GetRunState(context.Background(), h.outcomeID)
	if err != nil {
		t.Fatalf("run state: %v", err)
	}
	if view.State != outcome.MissionInProgress {
		t.Fatalf("state = %q/%q, want in_progress: the daemon admits the next unit itself",
			view.State, view.AttentionReason)
	}
	if start := runActionFor(view, outcome.RunActionStart); start.Available {
		t.Fatal("Start was offered while the run is already authorized")
	}
	// The projection and the command endpoint must refuse for the same reason.
	if _, err := h.command(t, domain.RunCommandStart, "rk-start-again"); requireAPICode(t, err) != outcome.CodeRunActionUnavailable {
		t.Fatalf("second start = %v, want RUN_ACTION_UNAVAILABLE", err)
	}
	// An authorized run with nothing yet running must still be stoppable:
	// CommandRun accepts cancel from a running intent.
	if cancel := runActionFor(view, outcome.RunActionCancel); !cancel.Available {
		t.Fatalf("cancel = %+v, want it offered", cancel)
	}
}

// runNeedsYouFake is the minimal NeedsYouStore the run-gate tests drive: a
// settable question list plus an injectable read error for the fail-closed
// case.
type runNeedsYouFake struct {
	questions []domain.NeedsYouQuestion
	err       error
}

func (f *runNeedsYouFake) ListCurrentNeedsYouQuestions(context.Context, domain.OutcomeID) ([]domain.NeedsYouQuestion, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]domain.NeedsYouQuestion(nil), f.questions...), nil
}
func (f *runNeedsYouFake) GetNeedsYouQuestion(context.Context, domain.OutcomeID, string) (domain.NeedsYouQuestion, bool, error) {
	return domain.NeedsYouQuestion{}, false, nil
}
func (f *runNeedsYouFake) ReconcileNeedsYouAnswer(context.Context, domain.OutcomeID, string, string) (domain.NeedsYouQuestion, error) {
	return domain.NeedsYouQuestion{}, errors.New("not implemented")
}

func (h *runHarness) openQuestion(id string) domain.NeedsYouQuestion {
	// Generation == ID is the store's self-integrity mark of an unresolved
	// production record; open is the status an unanswered question carries.
	return domain.NeedsYouQuestion{
		ID: id, OutcomeID: h.outcomeID, PlanRevisionID: h.planID,
		Generation: id, Kind: domain.NeedsYouChoice, Status: domain.NeedsYouOpen,
	}
}

// TestCommandRun_StartRefusesUnresolvedNeedsYou is the command-level half of
// the projection's attention surface: a question the owner has not answered
// refuses to become authorized work.
func TestCommandRun_StartRefusesUnresolvedNeedsYou(t *testing.T) {
	h := newRunHarness(t)
	needs := &runNeedsYouFake{questions: []domain.NeedsYouQuestion{h.openQuestion("q1")}}
	h.svc = h.svc.WithNeedsYou(needs, nil)

	if _, err := h.command(t, domain.RunCommandStart, "rk-start"); requireAPICode(t, err) != outcome.CodeRunNeedsYouUnresolved {
		t.Fatalf("start err = %v, want %s", err, outcome.CodeRunNeedsYouUnresolved)
	}
	if got := h.intents.generations(h.outcomeID); got != 0 {
		t.Fatalf("refusal appended %d generations, want none", got)
	}
	if calls := h.spawner.spawnCalls(); calls != 0 {
		t.Fatalf("refusal launched %d providers", calls)
	}
}

// TestCommandRun_ResumeRefusesUnresolvedNeedsYou covers the other
// authorization verb: a question raised after Start must hold a Resume.
func TestCommandRun_ResumeRefusesUnresolvedNeedsYou(t *testing.T) {
	h := newRunHarness(t)
	needs := &runNeedsYouFake{}
	h.svc = h.svc.WithNeedsYou(needs, nil)

	h.mustCommand(t, domain.RunCommandStart, "rk-start")
	h.mustCommand(t, domain.RunCommandPause, "rk-pause")
	needs.questions = []domain.NeedsYouQuestion{h.openQuestion("q1")}

	if _, err := h.command(t, domain.RunCommandResume, "rk-resume"); requireAPICode(t, err) != outcome.CodeRunNeedsYouUnresolved {
		t.Fatalf("resume err = %v, want %s", err, outcome.CodeRunNeedsYouUnresolved)
	}
}

// TestCommandRun_AnsweredNeedsYouDoesNotBlock keeps the gate precise:
// acknowledged, superseded and refused questions are resolved, and a question
// bound to a superseded Plan revision cannot veto the reviewed one.
func TestCommandRun_AnsweredNeedsYouDoesNotBlock(t *testing.T) {
	h := newRunHarness(t)
	acknowledged := h.openQuestion("q-ack")
	acknowledged.Status = domain.NeedsYouAcknowledged
	stalePlan := h.openQuestion("q-old-plan")
	stalePlan.PlanRevisionID = domain.PlanRevisionID("prv-superseded")
	needs := &runNeedsYouFake{questions: []domain.NeedsYouQuestion{acknowledged, stalePlan}}
	h.svc = h.svc.WithNeedsYou(needs, nil)

	view := h.mustCommand(t, domain.RunCommandStart, "rk-start")
	if view.Intent == nil || view.Intent.Desired != string(domain.RunIntentRunning) {
		t.Fatalf("intent = %+v, want running", view.Intent)
	}
}

// TestCommandRun_ReplaySkipsTheNeedsYouGate proves the replay check still
// returns the recorded authorization before the gate can judge it: a question
// opened after Start cannot turn the replayed Start into a refusal.
func TestCommandRun_ReplaySkipsTheNeedsYouGate(t *testing.T) {
	h := newRunHarness(t)
	needs := &runNeedsYouFake{}
	h.svc = h.svc.WithNeedsYou(needs, nil)

	first := h.mustCommand(t, domain.RunCommandStart, "rk-start")
	needs.questions = []domain.NeedsYouQuestion{h.openQuestion("q1")}

	second := h.mustCommand(t, domain.RunCommandStart, "rk-start")
	if first.Intent.Generation != second.Intent.Generation {
		t.Fatalf("generations = %d and %d, want the replay to return the first",
			first.Intent.Generation, second.Intent.Generation)
	}
}

// TestCommandRun_NeedsYouReadFailsClosed: when the question store cannot be
// read, the command fails rather than authorizing work over an unknown state.
func TestCommandRun_NeedsYouReadFailsClosed(t *testing.T) {
	h := newRunHarness(t)
	needs := &runNeedsYouFake{err: errors.New("store unavailable")}
	h.svc = h.svc.WithNeedsYou(needs, nil)

	if _, err := h.command(t, domain.RunCommandStart, "rk-start"); err == nil {
		t.Fatal("start authorized over an unreadable needs-you store")
	}
	if got := h.intents.generations(h.outcomeID); got != 0 {
		t.Fatalf("failed read appended %d generations, want none", got)
	}
}
