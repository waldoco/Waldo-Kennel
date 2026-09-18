package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

type fixedSupervisorValidator struct{}

func (fixedSupervisorValidator) Valid(sessionID domain.SessionID, launchID, token, verifier string) bool {
	return sessionID == "session-1" && launchID == "launch-1" && token == "token-1" && verifier == "verifier-1"
}

type completionEvidenceStore struct {
	*fakeStore
	ref domain.AttemptSessionRef
}

func (s *completionEvidenceStore) LatestAttemptSessionRefForSession(_ context.Context, sessionID string) (domain.AttemptSessionRef, bool, error) {
	return s.ref, s.ref.SessionID == sessionID, nil
}

func TestGovernedProcessExitRequiresCapabilityAndExactGenerationBeforeTermination(t *testing.T) {
	policy := domain.AttemptExecutionPolicy{
		OutcomeID: "out-1", PlanRevisionID: "plan-1", WorkUnitID: "wu-1", ContractRevisionNumber: 1,
		RunBriefCoreDigest: strings.Repeat("a", 64), RequiredCapabilities: []string{domain.CapabilityWorktreeRead},
		Grants: []domain.CapabilityGrant{{ID: "read", Name: domain.CapabilityWorktreeRead, Scope: "worktree/*"}},
	}
	var err error
	policy, err = policy.BindWorkspaceRoot("/ws/session-1")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := policy.Digest()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := json.Marshal(domain.AdmissionSnapshot{
		SnapshotVersion: domain.AdmissionSnapshotVersion, Harness: string(domain.HarnessCodex),
		ModelSelection: domain.ExecutionBindingModelProviderDefault, WorkUnitID: "wu-1", Mode: domain.SessionModeTUI,
		RunBriefCoreDigest: strings.Repeat("a", 64), RunBriefCompiledDigest: strings.Repeat("b", 64),
		ExecutionPolicy: policy, ExecutionPolicyDigest: digest, SessionID: "session-1",
		CompletionBoundary: domain.AttemptCompletionProcessExit,
	})
	if err != nil {
		t.Fatal(err)
	}
	base := newFakeStore()
	base.sessions["session-1"] = domain.SessionRecord{
		ID: "session-1", Harness: domain.HarnessCodex, Mode: domain.SessionModeTUI,
		Metadata: domain.SessionMetadata{
			WorkspacePath: "/ws/session-1", RuntimeLaunchID: "launch-1", GovernedExecutionPolicyDigest: digest,
			SupervisorCapabilityVerifier: "verifier-1",
		},
	}
	store := &completionEvidenceStore{fakeStore: base, ref: domain.AttemptSessionRef{
		ID: "ref-1", AttemptID: "att-1", Seq: 1, SessionID: "session-1", Harness: domain.HarnessCodex, Mode: domain.SessionModeTUI,
		RunBriefCoreDigest: strings.Repeat("a", 64), RunBriefCompiledDigest: strings.Repeat("b", 64), AdmissionSnapshot: string(snapshot),
	}}
	manager := New(store, nil, WithSupervisorCapabilityValidator(fixedSupervisorValidator{}))

	code := 0
	exit := ports.SupervisedProcessExit{LaunchID: "launch-1", ExitCode: &code, Reason: "exited"}
	if err := manager.RecordSupervisedProcessExit(ctx, "session-1", exit, "wrong"); !errors.Is(err, ports.ErrSupervisorCapabilityInvalid) {
		t.Fatalf("wrong capability error = %v", err)
	}
	if base.sessions["session-1"].Metadata.SupervisedProcessExitCode != nil {
		t.Fatal("unauthorized report mutated exit facts")
	}
	if err := manager.RecordSupervisedProcessExit(ctx, "session-1", exit, "token-1"); err != nil {
		t.Fatal(err)
	}
	if err := manager.ApplyRuntimeObservation(ctx, "session-1", ports.RuntimeFacts{
		ObservedAt: time.Now(), Runtime: ports.ProbeAlive, Workload: ports.ProbeDead, LaunchID: "launch-1",
	}); err != nil {
		t.Fatal(err)
	}
	if !base.sessions["session-1"].IsTerminated {
		t.Fatal("authenticated exact-generation process exit did not terminate governed session")
	}
}

// TestRecordSupervisedProcessExitRejectsContradictoryFacts covers the
// Manager-level defense-in-depth check: even with a valid capability and
// generation, a contradictory report (exit code 0 paired with a non-"exited"
// reason) must be refused before it is ever persisted, so a caller that
// bypasses the HTTP boundary cannot smuggle a "zero-plus-failure" report
// into durable state.
func TestRecordSupervisedProcessExitRejectsContradictoryFacts(t *testing.T) {
	base := newFakeStore()
	base.sessions["session-1"] = domain.SessionRecord{
		ID: "session-1", Harness: domain.HarnessCodex,
		Metadata: domain.SessionMetadata{
			RuntimeLaunchID: "launch-1", SupervisorCapabilityVerifier: "verifier-1",
		},
	}
	manager := New(base, nil, WithSupervisorCapabilityValidator(fixedSupervisorValidator{}))

	code := 0
	contradictory := ports.SupervisedProcessExit{LaunchID: "launch-1", ExitCode: &code, Reason: "failed"}
	if err := manager.RecordSupervisedProcessExit(ctx, "session-1", contradictory, "token-1"); !errors.Is(err, ports.ErrSupervisedExitInvalid) {
		t.Fatalf("contradictory exit error = %v, want ErrSupervisedExitInvalid", err)
	}
	if base.sessions["session-1"].Metadata.SupervisedProcessExitCode != nil || base.sessions["session-1"].Metadata.SupervisedProcessExitReason != "" {
		t.Fatal("contradictory report must not be persisted")
	}
}

// TestRecordOwnerTerminationMarksGovernedSession covers the owner-kill
// origin record: the daemon marks the governed session's exit facts with the
// transient pending intent before teardown. The final owner_killed fact
// appears only when the termination boundary is crossed (see
// TestMarkTerminatedPromotesPendingOwnerTermination).
func TestRecordOwnerTerminationMarksGovernedSession(t *testing.T) {
	base := newFakeStore()
	base.sessions["session-1"] = domain.SessionRecord{
		ID: "session-1", Harness: domain.HarnessCodex,
		Metadata: domain.SessionMetadata{
			RuntimeLaunchID: "launch-1", GovernedExecutionPolicyDigest: "digest-1",
		},
	}
	manager := New(base, nil)
	if err := manager.RecordOwnerTermination(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}
	if got := base.sessions["session-1"].Metadata.SupervisedProcessExitReason; got != domain.SupervisedExitReasonOwnerKillPending {
		t.Fatalf("reason = %q, want pending %q", got, domain.SupervisedExitReasonOwnerKillPending)
	}
}

// TestRecordOwnerTerminationKeepsAuthenticatedCrashFacts covers the
// precedence rule: a provider that already died on its own keeps its
// authenticated crash facts — an owner kill afterwards must not relabel a
// real crash as an intentional end.
func TestRecordOwnerTerminationKeepsAuthenticatedCrashFacts(t *testing.T) {
	base := newFakeStore()
	exitCode := 17
	base.sessions["session-1"] = domain.SessionRecord{
		ID: "session-1", Harness: domain.HarnessCodex,
		Metadata: domain.SessionMetadata{
			RuntimeLaunchID: "launch-1", GovernedExecutionPolicyDigest: "digest-1",
			SupervisedProcessExitCode: &exitCode, SupervisedProcessExitReason: "failed",
		},
	}
	manager := New(base, nil)
	if err := manager.RecordOwnerTermination(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}
	rec := base.sessions["session-1"]
	if rec.Metadata.SupervisedProcessExitReason != "failed" ||
		rec.Metadata.SupervisedProcessExitCode == nil || *rec.Metadata.SupervisedProcessExitCode != 17 {
		t.Fatalf("authenticated crash facts overwritten: %+v", rec.Metadata)
	}
}

// TestRecordOwnerTerminationIgnoresNonGovernedSession covers the scope
// guard: a plain session carries no attempt, so there is nothing to mark.
func TestRecordOwnerTerminationIgnoresNonGovernedSession(t *testing.T) {
	base := newFakeStore()
	base.sessions["session-1"] = domain.SessionRecord{ID: "session-1", Harness: domain.HarnessCodex}
	manager := New(base, nil)
	if err := manager.RecordOwnerTermination(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}
	if got := base.sessions["session-1"].Metadata.SupervisedProcessExitReason; got != "" {
		t.Fatalf("reason = %q, want empty for a non-governed session", got)
	}
}

// TestMarkTerminatedPromotesPendingOwnerTermination covers the proven
// termination boundary: the pending owner-kill intent becomes the final
// owner_killed fact atomically with the session's termination, so the
// attempt liveness pass settles the attempt reconciled.
func TestMarkTerminatedPromotesPendingOwnerTermination(t *testing.T) {
	base := newFakeStore()
	base.sessions["session-1"] = domain.SessionRecord{
		ID: "session-1", Harness: domain.HarnessCodex,
		Metadata: domain.SessionMetadata{
			RuntimeLaunchID: "launch-1", GovernedExecutionPolicyDigest: "digest-1",
		},
	}
	manager := New(base, nil)
	if err := manager.RecordOwnerTermination(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}
	if err := manager.MarkTerminated(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}
	rec := base.sessions["session-1"]
	if !rec.IsTerminated {
		t.Fatal("session must be terminated")
	}
	if got := rec.Metadata.SupervisedProcessExitReason; got != domain.SupervisedExitReasonOwnerKilled {
		t.Fatalf("reason = %q, want %q", got, domain.SupervisedExitReasonOwnerKilled)
	}
}

// TestClearOwnerTerminationRetractsPendingMarker covers the failed-kill
// retraction: the pending marker clears, the still-live session carries no
// owner-killed fact, and a later authenticated crash report settles the exit
// by its own facts (failed).
func TestClearOwnerTerminationRetractsPendingMarker(t *testing.T) {
	base := newFakeStore()
	base.sessions["session-1"] = domain.SessionRecord{
		ID: "session-1", Harness: domain.HarnessCodex,
		Metadata: domain.SessionMetadata{
			RuntimeLaunchID: "launch-1", GovernedExecutionPolicyDigest: "digest-1",
			SupervisorCapabilityVerifier: "verifier-1",
		},
	}
	manager := New(base, nil, WithSupervisorCapabilityValidator(fixedSupervisorValidator{}))
	if err := manager.RecordOwnerTermination(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}
	if err := manager.ClearOwnerTermination(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}
	if got := base.sessions["session-1"].Metadata.SupervisedProcessExitReason; got != "" {
		t.Fatalf("reason after clear = %q, want empty", got)
	}
	code := 1
	exit := ports.SupervisedProcessExit{LaunchID: "launch-1", ExitCode: &code, Reason: "failed"}
	if err := manager.RecordSupervisedProcessExit(ctx, "session-1", exit, "token-1"); err != nil {
		t.Fatal(err)
	}
	rec := base.sessions["session-1"]
	if rec.Metadata.SupervisedProcessExitReason != "failed" || rec.Metadata.SupervisedProcessExitCode == nil || *rec.Metadata.SupervisedProcessExitCode != 1 {
		t.Fatalf("post-clear crash facts = %+v, want failed/1", rec.Metadata)
	}
}

// TestCrashReportInFlightDuringOwnerKillKeepsFailedFacts covers the
// concurrent-crash race deterministically: an authenticated crash report
// that lands between the owner-kill intent and the termination boundary
// replaces the pending marker, so the promotion cannot overwrite it and the
// exit stays failed.
func TestCrashReportInFlightDuringOwnerKillKeepsFailedFacts(t *testing.T) {
	base := newFakeStore()
	base.sessions["session-1"] = domain.SessionRecord{
		ID: "session-1", Harness: domain.HarnessCodex,
		Metadata: domain.SessionMetadata{
			RuntimeLaunchID: "launch-1", GovernedExecutionPolicyDigest: "digest-1",
			SupervisorCapabilityVerifier: "verifier-1",
		},
	}
	manager := New(base, nil, WithSupervisorCapabilityValidator(fixedSupervisorValidator{}))
	if err := manager.RecordOwnerTermination(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}
	code := 1
	exit := ports.SupervisedProcessExit{LaunchID: "launch-1", ExitCode: &code, Reason: "failed"}
	if err := manager.RecordSupervisedProcessExit(ctx, "session-1", exit, "token-1"); err != nil {
		t.Fatal(err)
	}
	if err := manager.MarkTerminated(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}
	rec := base.sessions["session-1"]
	if rec.Metadata.SupervisedProcessExitReason != "failed" || rec.Metadata.SupervisedProcessExitCode == nil || *rec.Metadata.SupervisedProcessExitCode != 1 {
		t.Fatalf("crash facts overwritten by owner-kill promotion: %+v", rec.Metadata)
	}
}

// TestOwnerTerminationAndCrashReportRaceSettlesCrashFacts races the
// authenticated crash report against the termination promotion across both
// lock orders: in every interleaving the observed crash facts win, because
// the promotion only rewrites the exact pending marker and the report
// overwrites a promoted owner_killed before the attempt settles.
// raceSafeStore serializes the fake store's map access for the goroutine
// race test; the production store is sqlite, where concurrent statements are
// safe without this.
type raceSafeStore struct {
	mu sync.Mutex
	*fakeStore
}

func (s *raceSafeStore) GetSession(ctx context.Context, id domain.SessionID) (domain.SessionRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fakeStore.GetSession(ctx, id)
}

func (s *raceSafeStore) UpdateSession(ctx context.Context, rec domain.SessionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fakeStore.UpdateSession(ctx, rec)
}

func TestOwnerTerminationAndCrashReportRaceSettlesCrashFacts(t *testing.T) {
	for i := 0; i < 50; i++ {
		base := &raceSafeStore{fakeStore: newFakeStore()}
		base.sessions["session-1"] = domain.SessionRecord{
			ID: "session-1", Harness: domain.HarnessCodex,
			Metadata: domain.SessionMetadata{
				RuntimeLaunchID: "launch-1", GovernedExecutionPolicyDigest: "digest-1",
				SupervisorCapabilityVerifier: "verifier-1",
			},
		}
		manager := New(base, nil, WithSupervisorCapabilityValidator(fixedSupervisorValidator{}))
		if err := manager.RecordOwnerTermination(ctx, "session-1"); err != nil {
			t.Fatal(err)
		}
		code := 1
		exit := ports.SupervisedProcessExit{LaunchID: "launch-1", ExitCode: &code, Reason: "failed"}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = manager.RecordSupervisedProcessExit(ctx, "session-1", exit, "token-1")
		}()
		go func() {
			defer wg.Done()
			_ = manager.MarkTerminated(ctx, "session-1")
		}()
		wg.Wait()
		rec, ok, err := base.GetSession(ctx, "session-1")
		if err != nil || !ok {
			t.Fatalf("iteration %d: reload: ok=%v err=%v", i, ok, err)
		}
		if rec.Metadata.SupervisedProcessExitReason != "failed" ||
			rec.Metadata.SupervisedProcessExitCode == nil || *rec.Metadata.SupervisedProcessExitCode != 1 {
			t.Fatalf("iteration %d: crash lost the race: %+v", i, rec.Metadata)
		}
	}
}
