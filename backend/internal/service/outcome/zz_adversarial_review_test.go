package outcome_test

import (
	"context"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

// Adversarial W1.1 review: an attempt admitted before W1.1 (legacy row) has
// no launch packet. Packet absence must not be treated as proof that no
// provider crossed the launch boundary when machine evidence shows a live
// bound session. Beta behavior resumes a live bound session and holds custody.
func TestAdversarialLegacyAttemptWithoutPacketLiveProvider(t *testing.T) {
	ctx := context.Background()
	svc, store, _, heartbeats, outcomeID, planID := newAttemptHarness(t)
	view, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	heartbeats.signal(domain.SessionID(view.Sessions[0].SessionID))
	store.mu.Lock()
	store.launches = map[domain.AttemptID]domain.WorkspaceBoundLaunchPacket{}
	store.mu.Unlock()
	got, err := svc.RecoverAttempt(ctx, outcomeID, view.Attempt.ID, outcome.RecoveryInput{Action: outcome.RecoveryActionReconcile})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if got.Attempt.Fence == nil {
		t.Fatalf("MISCLASSIFICATION: custody released for a live provider without stop proof (attempt status %s)", got.Attempt.Attempt.Status)
	}
	if got.Attempt.Attempt.Status != domain.AttemptRunning {
		t.Fatalf("MISCLASSIFICATION: live attempt reclassified as %s", got.Attempt.Attempt.Status)
	}
	t.Logf("held: fence open=%v status=%s", got.Attempt.Fence.Open(), got.Attempt.Attempt.Status)
}

// Control: the same live attempt WITH its launch packet must resume and hold.
func TestAdversarialControlPacketPresentLiveProviderResumes(t *testing.T) {
	ctx := context.Background()
	svc, _, _, heartbeats, outcomeID, planID := newAttemptHarness(t)
	view, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	heartbeats.signal(domain.SessionID(view.Sessions[0].SessionID))
	got, err := svc.RecoverAttempt(ctx, outcomeID, view.Attempt.ID, outcome.RecoveryInput{Action: outcome.RecoveryActionReconcile})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if got.Attempt.Fence == nil || got.Attempt.Attempt.Status != domain.AttemptRunning {
		t.Fatalf("packet-present live attempt not resumed: status=%s fence=%+v", got.Attempt.Attempt.Status, got.Attempt.Fence)
	}
}

func TestLegacyAttemptWithoutPacketStoppedProviderCanRelease(t *testing.T) {
	ctx := context.Background()
	svc, store, _, heartbeats, outcomeID, planID := newAttemptHarness(t)
	view, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
	if err != nil {
		t.Fatal(err)
	}
	heartbeats.terminate(domain.SessionID(view.Sessions[0].SessionID))
	store.mu.Lock()
	store.launches = map[domain.AttemptID]domain.WorkspaceBoundLaunchPacket{}
	store.mu.Unlock()
	got, err := svc.RecoverAttempt(ctx, outcomeID, view.Attempt.ID, outcome.RecoveryInput{Action: outcome.RecoveryActionReconcile})
	if err != nil {
		t.Fatal(err)
	}
	if got.Attempt.Fence != nil {
		t.Fatal("stopped legacy provider retained custody")
	}
}

func TestLegacyOwnerConfirmationNeverReleasesWithoutMachineStopProof(t *testing.T) {
	for _, action := range []outcome.RecoveryAction{outcome.RecoveryActionReconcile, outcome.RecoveryActionReplace} {
		t.Run(string(action), func(t *testing.T) {
			ctx := context.Background()
			svc, store, _, heartbeats, outcomeID, planID := newAttemptHarness(t)
			view, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
			if err != nil {
				t.Fatal(err)
			}
			heartbeats.signal(domain.SessionID(view.Sessions[0].SessionID))
			store.mu.Lock()
			store.launches = map[domain.AttemptID]domain.WorkspaceBoundLaunchPacket{}
			store.mu.Unlock()
			_, _ = svc.RecoverAttempt(ctx, outcomeID, view.Attempt.ID, outcome.RecoveryInput{Action: action, ConfirmProviderStopped: true})
			got, _ := svc.GetAttempt(ctx, outcomeID, view.Attempt.ID)
			if got.Fence == nil {
				t.Fatal("legacy custody released")
			}
		})
	}
}
