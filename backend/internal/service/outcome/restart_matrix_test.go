package outcome_test

import (
	"context"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/intelligence/intelligencetest"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
	sqlitestore "github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/store"
)

// sqliteLifetime is one daemon process lifetime over a real sqlite store: a
// fresh service wired to a freshly opened store in the same data directory.
// Closing the store and opening the next lifetime simulates a daemon restart
// without any fake durability.
type sqliteLifetime struct {
	svc     *outcome.Service
	store   *sqlitestore.Store
	spawner *fakeSpawner
}

func openSQLiteLifetime(t *testing.T, dataDir string) *sqliteLifetime {
	t.Helper()
	store, err := sqlite.Open(dataDir)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	spawner := &fakeSpawner{readiness: ports.AgentProfileReadiness{Ready: true, Detail: "profile ok"}}
	svc := outcome.New(store, func() time.Time { return time.Now().UTC() }).
		WithPlanning(intelligencetest.Provider{}, &routingInventoryFake{candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessCodex, "")}}).
		WithExecution(spawner, newFakeHeartbeats()).
		WithRunIntents(store)
	svc.AdmissionPolicy = testAdmissionPolicy()
	project := domain.ProjectRecord{ID: "mer", Path: dataDir, DisplayName: "mer", RegisteredAt: time.Now().UTC()}
	if err := store.UpsertProject(context.Background(), project); err != nil {
		t.Fatalf("register project: %v", err)
	}
	return &sqliteLifetime{svc: svc, store: store, spawner: spawner}
}

func (l *sqliteLifetime) crash(t *testing.T) {
	t.Helper()
	if err := l.store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
}

// startRunOutcome builds an approved one-unit plan in this lifetime and runs
// the Start command, returning the outcome id. The unit is left RUNNING: no
// end, no receipt, no reconcile - the state a crashed daemon leaves behind.
func startRunOutcome(t *testing.T, l *sqliteLifetime) domain.OutcomeID {
	t.Helper()
	ctx := context.Background()
	view, err := l.svc.Create(ctx, validCreateInput())
	if err != nil {
		t.Fatalf("create outcome: %v", err)
	}
	planView, err := l.svc.ProposePlan(ctx, view.Outcome.ID, 1)
	if err != nil {
		t.Fatalf("propose plan: %v", err)
	}
	if _, err := l.svc.ApprovePlan(ctx, view.Outcome.ID, outcome.ApprovePlanInput{
		PlanRevisionID: planView.Plan.ID, ExpectedContractRevision: 1,
	}); err != nil {
		t.Fatalf("approve plan: %v", err)
	}
	rememberFirstWorkUnit(planView.Plan)
	if _, err := l.svc.CommandRun(ctx, view.Outcome.ID, outcome.RunCommandInput{
		Command: domain.RunCommandStart, PlanRevisionID: planView.Plan.ID,
		ExpectedContractRevision: 1, RequestKey: "rk-start",
	}); err != nil {
		t.Fatalf("command start: %v", err)
	}
	if err := l.svc.ContinueAuthorizedRuns(ctx); err != nil {
		t.Fatalf("continuation: %v", err)
	}
	attempts, err := l.store.ListAttempts(ctx, view.Outcome.ID)
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempts = %d err=%v, want one running", len(attempts), err)
	}
	if calls := l.spawner.spawnCalls(); calls != 1 {
		t.Fatalf("spawn calls = %d, want one", calls)
	}
	return view.Outcome.ID
}

// TestRestartMatrix_DaemonRestartRecoversOneContinuation is cell 1: the
// daemon dies mid-attempt, and a fresh process over the same sqlite file
// recovers exactly one continuation - never two parallel lives for the same
// unit, never a silently dropped one.
func TestRestartMatrix_DaemonRestartRecoversOneContinuation(t *testing.T) {
	dataDir := t.TempDir()
	ctx := context.Background()

	first := openSQLiteLifetime(t, dataDir)
	outcomeID := startRunOutcome(t, first)
	first.crash(t)

	second := openSQLiteLifetime(t, dataDir)
	if err := second.svc.ContinueAuthorizedRuns(ctx); err != nil {
		t.Fatalf("post-restart continuation: %v", err)
	}
	if err := second.svc.ContinueAuthorizedRuns(ctx); err != nil {
		t.Fatalf("second post-restart continuation: %v", err)
	}

	attempts, err := second.store.ListAttempts(ctx, outcomeID)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	active := 0
	for _, attempt := range attempts {
		switch attempt.Status {
		case domain.AttemptSucceeded, domain.AttemptReconciled:
			t.Fatalf("attempt %s reached %s with no result recorded - recovery invented a success", attempt.ID, attempt.Status)
		case domain.AttemptQueued, domain.AttemptRunning, domain.AttemptAwaitingAuthority:
			active++
		}
	}
	if active != 1 {
		t.Fatalf("active attempts after restart recovery = %d (of %d total), want exactly one continuation", active, len(attempts))
	}
	if calls := second.spawner.spawnCalls(); calls > 1 {
		t.Fatalf("post-restart spawns = %d, want at most one replacement launch", calls)
	}
}

// TestRestartMatrix_SameRequestKeyReplaysAcrossRestart is cell 3: a Start
// retried after the crash with the original request key returns the recorded
// receipt of the first admission - never a second intent, never a second
// provider launch.
func TestRestartMatrix_SameRequestKeyReplaysAcrossRestart(t *testing.T) {
	dataDir := t.TempDir()
	ctx := context.Background()

	first := openSQLiteLifetime(t, dataDir)
	outcomeID := startRunOutcome(t, first)
	first.crash(t)

	second := openSQLiteLifetime(t, dataDir)
	plan, err := second.svc.GetLatestPlan(ctx, outcomeID)
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	replay, err := second.svc.CommandRun(ctx, outcomeID, outcome.RunCommandInput{
		Command: domain.RunCommandStart, PlanRevisionID: plan.Plan.ID,
		ExpectedContractRevision: 1, RequestKey: "rk-start",
	})
	if err != nil {
		t.Fatalf("replayed start command: %v", err)
	}
	if replay.Intent == nil {
		t.Fatal("replayed start returned no recorded intent")
	}
	if calls := second.spawner.spawnCalls(); calls != 0 {
		t.Fatalf("replayed command spawned %d providers, want none", calls)
	}
	intents, err := second.store.ListRunIntents(ctx, outcomeID)
	if err != nil {
		t.Fatalf("list run intents: %v", err)
	}
	// The command itself is an admission input, not persisted; the durable
	// proof is that exactly one intent row exists after the cross-restart
	// replay - the retry recorded nothing new.
	if len(intents) != 1 {
		t.Fatalf("run intents = %d after cross-restart replay, want exactly one", len(intents))
	}
}

// TestRestartMatrix_KillLeavesCustodyDebrisRefused is cell 4: the daemon is
// killed mid-flight with no cancel and no cleanup. In the next lifetime the
// surviving attempt's runtime status is unknowable, so authorizing new work
// is refused RUN_CUSTODY_UNKNOWN until reconciliation accounts for it - and
// only then does Start admit exactly one continuation. An owner cancel after
// that is idempotent and leaves nothing running.
func TestRestartMatrix_KillLeavesCustodyDebrisRefused(t *testing.T) {
	dataDir := t.TempDir()
	ctx := context.Background()

	first := openSQLiteLifetime(t, dataDir)
	outcomeID := startRunOutcome(t, first)
	first.crash(t)

	second := openSQLiteLifetime(t, dataDir)
	plan, err := second.svc.GetLatestPlan(ctx, outcomeID)
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	base := outcome.RunCommandInput{PlanRevisionID: plan.Plan.ID, ExpectedContractRevision: 1}

	// The intent still says running, so start does not apply; pause and then
	// try to resume over the debris. Authorization re-validates custody: the
	// killed attempt's runtime status is unknowable in this lifetime, so
	// resume is refused until the debris is accounted for.
	pause := base
	pause.Command, pause.RequestKey = domain.RunCommandPause, "rk-pause"
	if _, err := second.svc.CommandRun(ctx, outcomeID, pause); err != nil {
		t.Fatalf("pause after kill: %v", err)
	}
	resume := base
	resume.Command, resume.RequestKey = domain.RunCommandResume, "rk-resume-over-debris"
	if _, err := second.svc.CommandRun(ctx, outcomeID, resume); requireAPICode(t, err) != outcome.CodeRunCustodyUnknown {
		t.Fatalf("resume over killed work err = %v, want %s", err, outcome.CodeRunCustodyUnknown)
	}
	if calls := second.spawner.spawnCalls(); calls != 0 {
		t.Fatalf("refused resume spawned %d providers, want none", calls)
	}

	// Owner cancel is the conservative cleanup: it accounts for the debris
	// idempotently (a replay completes the same cancellation), and afterwards
	// nothing is running.
	cancel := base
	cancel.Command, cancel.RequestKey = domain.RunCommandCancel, "rk-cancel"
	if _, err := second.svc.CommandRun(ctx, outcomeID, cancel); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := second.svc.CommandRun(ctx, outcomeID, cancel); err != nil {
		t.Fatalf("replayed cancel: %v", err)
	}
	if err := second.svc.ContinueAuthorizedRuns(ctx); err != nil {
		t.Fatalf("continuation after cancel: %v", err)
	}
	attempts, err := second.store.ListAttempts(ctx, outcomeID)
	if err != nil {
		t.Fatalf("relist attempts: %v", err)
	}
	for _, attempt := range attempts {
		if attempt.Status == domain.AttemptRunning || attempt.Status == domain.AttemptQueued || attempt.Status == domain.AttemptAwaitingAuthority {
			t.Fatalf("attempt %s still %s after owner cancel", attempt.ID, attempt.Status)
		}
		if attempt.Status == domain.AttemptSucceeded {
			t.Fatalf("attempt %s succeeded with no result recorded", attempt.ID)
		}
	}
}
