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
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
	sqlitestore "github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/store"
)

// sqliteLifetime is one daemon process lifetime over a real sqlite store: a
// fresh service wired to a freshly opened store in the same data directory.
// Closing the store and opening the next lifetime simulates a daemon restart
// without any fake durability. The heartbeats fake stands in for provider
// liveness evidence only; it holds no durable state, so a fresh lifetime
// starts with no knowledge of pre-crash sessions - exactly what a restarted
// process can prove.
type sqliteLifetime struct {
	svc        *outcome.Service
	store      *sqlitestore.Store
	spawner    *fakeSpawner
	heartbeats *fakeHeartbeats
}

func openSQLiteLifetime(t *testing.T, dataDir string) *sqliteLifetime {
	t.Helper()
	store, err := sqlite.Open(dataDir)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	spawner := &fakeSpawner{readiness: ports.AgentProfileReadiness{Ready: true, Detail: "profile ok"}}
	heartbeats := newFakeHeartbeats()
	svc := outcome.New(store, func() time.Time { return time.Now().UTC() }).
		WithPlanning(intelligencetest.Provider{}, &routingInventoryFake{candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessCodex, "")}}).
		WithExecution(spawner, heartbeats).
		WithRunIntents(store).
		WithNeedsYou(store, nil).
		WithCapabilityEscalations(store)
	svc.AdmissionPolicy = testAdmissionPolicy()
	project := domain.ProjectRecord{ID: "mer", Path: dataDir, DisplayName: "mer", RegisteredAt: time.Now().UTC()}
	if err := store.UpsertProject(context.Background(), project); err != nil {
		t.Fatalf("register project: %v", err)
	}
	return &sqliteLifetime{svc: svc, store: store, spawner: spawner, heartbeats: heartbeats}
}

func (l *sqliteLifetime) crash(t *testing.T) {
	t.Helper()
	if err := l.store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
}

// startRunOutcome builds an approved one-unit plan in this lifetime and runs
// the Start command, returning the outcome and attempt ids. The unit is left
// RUNNING: no end, no receipt, no reconcile - the state a crashed daemon
// leaves behind.
func startRunOutcome(t *testing.T, l *sqliteLifetime) (domain.OutcomeID, domain.AttemptID) {
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
	if attempts[0].Status != domain.AttemptRunning {
		t.Fatalf("attempt status = %s, want running", attempts[0].Status)
	}
	if calls := l.spawner.spawnCalls(); calls != 1 {
		t.Fatalf("spawn calls = %d, want one", calls)
	}
	return view.Outcome.ID, attempts[0].ID
}

func mustAttempt(t *testing.T, store *sqlitestore.Store, outcomeID domain.OutcomeID, attemptID domain.AttemptID) domain.Attempt {
	t.Helper()
	attempts, err := store.ListAttempts(context.Background(), outcomeID)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	for _, attempt := range attempts {
		if attempt.ID == attemptID {
			return attempt
		}
	}
	t.Fatalf("attempt %s not found among %d", attemptID, len(attempts))
	return domain.Attempt{}
}

// TestRestartMatrix_DaemonRestartRecoversOneContinuation is cell 1: the
// daemon dies mid-attempt. A fresh process over the same sqlite file can
// prove nothing about the pre-crash provider, so it must act on nothing:
// liveness with no evidence mutates nothing and continuation mints no blind
// duplicate. Only when durable termination evidence exists does the stranded
// row reconcile (ended, result unclassified - never a success), and only an
// explicit owner replacement decision authorizes the next generation, which
// mints exactly one fresh attempt for the unit.
func TestRestartMatrix_DaemonRestartRecoversOneContinuation(t *testing.T) {
	dataDir := t.TempDir()
	ctx := context.Background()

	first := openSQLiteLifetime(t, dataDir)
	outcomeID, attemptID := startRunOutcome(t, first)
	ref, bound, err := first.store.LatestAttemptSessionRef(ctx, attemptID)
	if err != nil || !bound {
		t.Fatalf("running attempt has no bound session ref: %v", err)
	}
	firstAttempt := mustAttempt(t, first.store, outcomeID, attemptID)
	first.crash(t)

	second := openSQLiteLifetime(t, dataDir)

	// No evidence in the fresh lifetime: the reconcile loop must decide
	// nothing, and continuation must not mint a blind duplicate beside the
	// possibly-live original.
	if err := second.svc.EvaluateAttemptLiveness(ctx); err != nil {
		t.Fatalf("liveness without evidence: %v", err)
	}
	if err := second.svc.ContinueAuthorizedRuns(ctx); err != nil {
		t.Fatalf("continuation without evidence: %v", err)
	}
	stranded := mustAttempt(t, second.store, outcomeID, attemptID)
	if stranded.Status != domain.AttemptRunning {
		t.Fatalf("evidence-free liveness rewrote the stranded attempt to %s; missing heartbeats must mutate nothing", stranded.Status)
	}
	if calls := second.spawner.spawnCalls(); calls != 0 {
		t.Fatalf("continuation spawned %d providers beside an unaccounted attempt, want none", calls)
	}

	// Durable termination evidence arrives: the bound provider session ended
	// with the old process. The reconcile loop now accounts for the row as
	// reconciled - ended and accounted for, result unclassified. It must NOT
	// become a success, and the terminal record alone must not authorize a
	// retry.
	second.heartbeats.terminate(domain.SessionID(ref.SessionID))
	if err := second.svc.EvaluateAttemptLiveness(ctx); err != nil {
		t.Fatalf("liveness with termination evidence: %v", err)
	}
	reconciled := mustAttempt(t, second.store, outcomeID, attemptID)
	if reconciled.Status != domain.AttemptReconciled {
		t.Fatalf("terminated provider evidence settled the attempt as %s, want %s", reconciled.Status, domain.AttemptReconciled)
	}
	if err := second.svc.ContinueAuthorizedRuns(ctx); err != nil {
		t.Fatalf("continuation before replacement decision: %v", err)
	}
	if calls := second.spawner.spawnCalls(); calls != 0 {
		t.Fatalf("continuation spawned %d providers before an owner replacement decision, want none", calls)
	}
	view, err := second.svc.GetRunState(ctx, outcomeID)
	if err != nil {
		t.Fatalf("run state before replacement decision: %v", err)
	}
	if view.State != outcome.MissionNeedsYou || view.AttentionReason != outcome.ReasonAttemptReplacementRequired {
		t.Fatalf("run state = %s/%s, want needs_you/%s", view.State, view.AttentionReason, outcome.ReasonAttemptReplacementRequired)
	}

	// The owner directs replacement. The reconciled record is terminal and
	// never rewritten; custody moves and a replacement receipt lands.
	recovery, err := second.svc.RecoverAttempt(ctx, outcomeID, attemptID, outcome.RecoveryInput{
		Action: outcome.RecoveryActionReplace, ConfirmProviderStopped: true,
	})
	if err != nil {
		t.Fatalf("owner replacement: %v", err)
	}
	if recovery.Receipt == nil || recovery.Receipt.Resolution != domain.RecoveryReplacement {
		t.Fatalf("owner replacement produced receipt %+v, want a replacement receipt", recovery.Receipt)
	}
	if still := mustAttempt(t, second.store, outcomeID, attemptID); still.Status != domain.AttemptReconciled {
		t.Fatalf("replacement rewrote the terminal record to %s; terminal records are never rewritten", still.Status)
	}

	// The next continuation mints exactly one fresh attempt - new row, new
	// number, new request identity, same work unit - and repeated ticks do
	// not multiply it.
	for tick := 0; tick < 3; tick++ {
		if err := second.svc.ContinueAuthorizedRuns(ctx); err != nil {
			t.Fatalf("replacement tick %d: %v", tick, err)
		}
	}
	attempts, err := second.store.ListAttempts(ctx, outcomeID)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	if len(attempts) != 2 {
		t.Fatalf("attempts after replacement = %d, want exactly two (reconciled original + one fresh)", len(attempts))
	}
	replacement := attempts[1]
	if replacement.Number != 2 || replacement.ID == attemptID || replacement.RequestKey == firstAttempt.RequestKey || replacement.WorkUnitID != firstAttempt.WorkUnitID {
		t.Fatalf("replacement = %+v, want fresh attempt #2 with new request identity over the same work unit", replacement)
	}
	switch replacement.Status {
	case domain.AttemptQueued, domain.AttemptRunning, domain.AttemptAwaitingAuthority:
	default:
		t.Fatalf("replacement status = %s, want an active continuation", replacement.Status)
	}
	if calls := second.spawner.spawnCalls(); calls != 1 {
		t.Fatalf("post-restart replacement launches = %d, want exactly one", calls)
	}
	for _, attempt := range attempts {
		if attempt.Status == domain.AttemptSucceeded {
			t.Fatalf("attempt %s succeeded with no result recorded - recovery invented a success", attempt.ID)
		}
	}
}

// TestRestartMatrix_SameRequestKeyReplaysAcrossRestart is cell 3: a Start
// retried after the crash with the original request key returns the recorded
// receipt of the first admission - never a second intent, never a second
// provider launch - and the continuation that runs afterwards still sees the
// original attempt as the only one.
func TestRestartMatrix_SameRequestKeyReplaysAcrossRestart(t *testing.T) {
	dataDir := t.TempDir()
	ctx := context.Background()

	first := openSQLiteLifetime(t, dataDir)
	outcomeID, attemptID := startRunOutcome(t, first)
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
	// The continuation tick that follows the replay still sees exactly the
	// original attempt: no fresh spawn, no new row, no status rewrite.
	if err := second.svc.ContinueAuthorizedRuns(ctx); err != nil {
		t.Fatalf("post-replay continuation: %v", err)
	}
	attempts, err := second.store.ListAttempts(ctx, outcomeID)
	if err != nil {
		t.Fatalf("list attempts after replay: %v", err)
	}
	if len(attempts) != 1 || attempts[0].ID != attemptID || attempts[0].Status != domain.AttemptRunning {
		t.Fatalf("attempts after replay+continuation = %+v, want only the original attempt still running", attempts)
	}
	if calls := second.spawner.spawnCalls(); calls != 0 {
		t.Fatalf("post-replay continuation spawned %d providers, want none", calls)
	}
}

// TestRestartMatrix_KillLeavesCustodyDebrisRefused is cell 4: the daemon is
// killed mid-flight with no cancel and no cleanup. In the next lifetime the
// surviving attempt's runtime status is unknowable, so resume over the
// debris is refused RUN_CUSTODY_UNKNOWN until reconciliation accounts for
// it. The owner cancel is that conservative cleanup: it accounts for the
// debris idempotently (a replay completes the same cancellation), and
// afterwards nothing is running.
func TestRestartMatrix_KillLeavesCustodyDebrisRefused(t *testing.T) {
	dataDir := t.TempDir()
	ctx := context.Background()

	first := openSQLiteLifetime(t, dataDir)
	outcomeID, _ := startRunOutcome(t, first)
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

// capabilityEscalationFor builds a valid post-run governed-check escalation
// against the real attempt/session lineage the harness created, so the store
// accepts it and the needs-you projection can serve it.
func capabilityEscalationFor(t *testing.T, attempt domain.Attempt, ref domain.AttemptSessionRef, questionGeneration, requestFingerprint string) domain.CapabilityEscalation {
	t.Helper()
	e := domain.CapabilityEscalation{
		Version:                domain.CapabilityEscalationVersion,
		ExecutorKind:           "governed_check",
		OutcomeID:              attempt.OutcomeID,
		ContractRevisionNumber: attempt.ContractRevisionNumber,
		PlanRevisionID:         attempt.PlanRevisionID,
		WorkUnitID:             attempt.WorkUnitID,
		AttemptID:              attempt.ID,
		AttemptGeneration:      attempt.Number,
		SessionID:              domain.SessionID(ref.SessionID),
		SessionGeneration:      ref.Seq,
		AttemptSessionRefID:    ref.ID,
		CheckID:                "check-1",
		PolicyDigest:           "policy",
		RequestedCapability:    domain.CapabilityWorktreeExec,
		DenialSource:           "governed_check",
		GrantFingerprint:       "policy",
		OperationID:            "check:check-1",
		RequestFingerprint:     requestFingerprint,
		QuestionGeneration:     questionGeneration,
	}
	digest, err := e.ComputedDigest()
	if err != nil {
		t.Fatalf("compute escalation digest: %v", err)
	}
	e.Digest = digest
	return e
}

// TestRestartMatrix_NeedsYouQuestionSurvivesRestartAckRefusalAndSupersession
// is cell 5: a durable needs-you question outlives the daemon. Across two
// restarts over the same sqlite file the question survives with its
// generation; stale-generation and invented-option answers are refused
// without effect; a successor session supersedes the pending question and
// the superseded row can never be answered; the successor's answer lands
// durably; and after a second restart the recorded answer still stands, an
// exact replay returns the same receipt without a new effect, and a
// conflicting replay under the same question is refused.
func TestRestartMatrix_NeedsYouQuestionSurvivesRestartAckRefusalAndSupersession(t *testing.T) {
	dataDir := t.TempDir()
	ctx := context.Background()

	first := openSQLiteLifetime(t, dataDir)
	outcomeID, attemptID := startRunOutcome(t, first)
	attempt := mustAttempt(t, first.store, outcomeID, attemptID)
	spawnRef, bound, err := first.store.LatestAttemptSessionRef(ctx, attemptID)
	if err != nil || !bound {
		t.Fatalf("running attempt has no bound session ref: %v", err)
	}
	// The escalation's controller session is a real durable session row: the
	// attempt's latest ref is bound to it and the conversation hangs off it.
	sessionRec, err := first.store.CreateSession(ctx, domain.SessionRecord{ProjectID: "mer", Kind: domain.KindWorker, Harness: domain.HarnessCodex})
	if err != nil {
		t.Fatalf("create controller session: %v", err)
	}
	ctrlRef := spawnRef
	ctrlRef.ID = spawnRef.ID + "-ctrl"
	ctrlRef.Seq = spawnRef.Seq + 1
	ctrlRef.SessionID = string(sessionRec.ID)
	ctrlRef.BoundAt = time.Now().UTC()
	ref, err := first.store.BindAttemptSession(ctx, ctrlRef)
	if err != nil {
		t.Fatalf("bind controller session ref: %v", err)
	}
	if _, err := first.store.CreateConversation(ctx, "conv-"+string(sessionRec.ID), domain.ConversationScopeSession, "mer", sessionRec.ID, time.Now().UTC()); err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	q1, created, err := first.svc.CreateCapabilityEscalation(ctx, capabilityEscalationFor(t, attempt, ref, "q1-generation", "operation-fp-1"))
	if err != nil || !created {
		t.Fatalf("create first escalation: created=%v err=%v", created, err)
	}
	first.crash(t)

	second := openSQLiteLifetime(t, dataDir)

	// Durable across the restart: the pending question is still there, open,
	// with its immutable generation.
	current, err := second.svc.CurrentNeedsYou(ctx, outcomeID)
	if err != nil {
		t.Fatalf("current needs-you after restart: %v", err)
	}
	if len(current) != 1 || current[0].ID != q1.ID || current[0].Status != domain.NeedsYouOpen || current[0].Generation != "q1-generation" {
		t.Fatalf("current needs-you after restart = %+v, want the original question open with its generation", current)
	}

	// Refusals across the restart: a stale generation and an invented option
	// are both rejected without touching the durable row.
	if _, err := second.svc.AnswerNeedsYou(ctx, outcomeID, q1.ID, domain.NeedsYouAnswer{
		RequestKey: "rk-stale", Generation: "older-generation",
		Decision: &domain.ChatDecisionAnswer{ID: domain.CapabilityEscalationDeny},
	}); !errors.Is(err, outcome.ErrNeedsYouStale) {
		t.Fatalf("stale-generation answer err = %v, want %v", err, outcome.ErrNeedsYouStale)
	}
	if _, err := second.svc.AnswerNeedsYou(ctx, outcomeID, q1.ID, domain.NeedsYouAnswer{
		RequestKey: "rk-invented", Generation: "q1-generation",
		Decision: &domain.ChatDecisionAnswer{ID: "invented-option"},
	}); err == nil {
		t.Fatal("invented option accepted")
	}
	if still, ok, err := second.store.GetNeedsYouQuestion(ctx, outcomeID, q1.ID); err != nil || !ok || still.Status != domain.NeedsYouOpen {
		t.Fatalf("question after refused answers = %+v ok=%v err=%v, want still open", still, ok, err)
	}

	// Supersession across the restart: a successor session ref makes the
	// successor escalation the live one; the pending row is durably
	// superseded and can never be answered.
	// The durable successor is a NEW session with its own conversation: the
	// pending question's request identity stays unique per conversation.
	successorSession, err := second.store.CreateSession(ctx, domain.SessionRecord{ProjectID: "mer", Kind: domain.KindWorker, Harness: domain.HarnessCodex})
	if err != nil {
		t.Fatalf("create successor session: %v", err)
	}
	succ := ref
	succ.ID = ref.ID + "-successor"
	succ.Seq = ref.Seq + 1
	succ.SessionID = string(successorSession.ID)
	succ.BoundAt = time.Now().UTC()
	successorRef, err := second.store.BindAttemptSession(ctx, succ)
	if err != nil {
		t.Fatalf("bind successor session ref: %v", err)
	}
	if _, err := second.store.CreateConversation(ctx, "conv-"+string(successorSession.ID), domain.ConversationScopeSession, "mer", successorSession.ID, time.Now().UTC()); err != nil {
		t.Fatalf("create successor conversation: %v", err)
	}
	attemptForQ2 := mustAttempt(t, second.store, outcomeID, attemptID)
	q2, created, err := second.svc.CreateCapabilityEscalation(ctx, capabilityEscalationFor(t, attemptForQ2, successorRef, "q2-generation", "operation-fp-2"))
	if err != nil || !created {
		t.Fatalf("create successor escalation: created=%v err=%v", created, err)
	}
	// The superseded row leaves the current projection entirely: only the
	// successor is open. Applying an answer to the predecessor is durably
	// refused as stale, and the service facade no longer addresses it.
	currentAfterSupersession, err := second.svc.CurrentNeedsYou(ctx, outcomeID)
	if err != nil {
		t.Fatalf("current needs-you after supersession: %v", err)
	}
	if len(currentAfterSupersession) != 1 || currentAfterSupersession[0].ID != q2.ID || currentAfterSupersession[0].Status != domain.NeedsYouOpen {
		t.Fatalf("current needs-you after supersession = %+v, want only the successor open", currentAfterSupersession)
	}
	if _, _, err := second.store.ApplyCapabilityEscalationAnswer(ctx, outcomeID, q1.ID, "q1-generation", domain.CapabilityEscalationDeny, "rk-late"); err == nil {
		t.Fatal("answer applied to the superseded question")
	}
	if _, err := second.svc.AnswerNeedsYou(ctx, outcomeID, q1.ID, domain.NeedsYouAnswer{
		RequestKey: "rk-late", Generation: "q1-generation",
		Decision: &domain.ChatDecisionAnswer{ID: domain.CapabilityEscalationDeny},
	}); !errors.Is(err, outcome.ErrNeedsYouNotFound) {
		t.Fatalf("answer to superseded question err = %v, want %v", err, outcome.ErrNeedsYouNotFound)
	}

	// The successor's answer lands durably: deny resolves the question.
	if _, err := second.svc.AnswerNeedsYou(ctx, outcomeID, q2.ID, domain.NeedsYouAnswer{
		RequestKey: "rk-deny-2", Generation: "q2-generation",
		Decision: &domain.ChatDecisionAnswer{ID: domain.CapabilityEscalationDeny},
	}); err != nil {
		t.Fatalf("answer successor across restart: %v", err)
	}
	second.crash(t)

	third := openSQLiteLifetime(t, dataDir)

	// The recorded answer survives the second restart: the question is no
	// longer open or answerable, the exact replay returns the same receipt
	// with no new effect, and a conflicting replay is refused.
	answered, ok, err := third.store.GetNeedsYouQuestion(ctx, outcomeID, q2.ID)
	if err != nil || !ok {
		t.Fatalf("read answered question: ok=%v err=%v", ok, err)
	}
	if answered.Status == domain.NeedsYouOpen {
		t.Fatalf("answered question reopened across restart: %+v", answered)
	}
	receipt, replayed, err := third.store.ApplyCapabilityEscalationAnswer(ctx, outcomeID, q2.ID, "q2-generation", domain.CapabilityEscalationDeny, "rk-deny-2")
	if err != nil {
		t.Fatalf("exact replay of recorded answer: %v", err)
	}
	if replayed {
		t.Fatal("exact replay created a second receipt effect")
	}
	if receipt.AnswerRequestKey != "rk-deny-2" || receipt.Consequence != domain.CapabilityConsequenceDenied {
		t.Fatalf("replayed receipt = %+v, want the recorded denial under rk-deny-2", receipt)
	}
	if _, _, err := third.store.ApplyCapabilityEscalationAnswer(ctx, outcomeID, q2.ID, "q2-generation", domain.CapabilityEscalationDeny, "rk-different"); err == nil {
		t.Fatal("conflicting replay under a new request key accepted")
	}
	if _, err := third.svc.AnswerNeedsYou(ctx, outcomeID, q1.ID, domain.NeedsYouAnswer{
		RequestKey: "rk-late-2", Generation: "q1-generation",
		Decision: &domain.ChatDecisionAnswer{ID: domain.CapabilityEscalationDeny},
	}); !errors.Is(err, outcome.ErrNeedsYouNotFound) {
		t.Fatalf("superseded answer after second restart err = %v, want %v", err, outcome.ErrNeedsYouNotFound)
	}
	open, err := third.svc.CurrentNeedsYou(ctx, outcomeID)
	if err != nil {
		t.Fatalf("current needs-you after second restart: %v", err)
	}
	for _, q := range open {
		if q.Status == domain.NeedsYouOpen {
			t.Fatalf("question %+v still open after recorded answer and supersession", q)
		}
	}
}
