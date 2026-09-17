package outcome_test

import (
	"context"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

func TestLegacyRecoveryCustodyMatrix(t *testing.T) {
	states := []string{"live", "stale", "missing", "terminated"}
	actions := []outcome.RecoveryAction{outcome.RecoveryActionReconcile, outcome.RecoveryActionReplace}
	for _, unavailable := range []bool{false, true} {
		for _, state := range states {
			for _, action := range actions {
				for _, confirm := range []bool{false, true} {
					name := state + "/" + string(action)
					if unavailable {
						name = "store-unavailable/" + name
					}
					if confirm {
						name += "/confirmed"
					}
					t.Run(name, func(t *testing.T) {
						ctx := context.Background()
						svc, store, spawner, hb, outcomeID, planID := newAttemptHarness(t)
						view, err := svc.StartAttempt(ctx, outcomeID, startInput(planID))
						if err != nil {
							t.Fatal(err)
						}
						sid := domain.SessionID(view.Sessions[0].SessionID)
						store.mu.Lock()
						store.launches = map[domain.AttemptID]domain.WorkspaceBoundLaunchPacket{}
						store.mu.Unlock()
						if unavailable {
							svc.WithAdmissionStore(nil)
						}
						switch state {
						case "live":
							hb.signal(sid)
						case "stale":
							hb.signal(sid)
							hb.backdate(sid, 2*time.Hour)
						case "missing":
							hb.forget(sid)
						case "terminated":
							hb.terminate(sid)
						}
						before := view.Attempt.Status
						got, err := svc.RecoverAttempt(ctx, outcomeID, view.Attempt.ID, outcome.RecoveryInput{Action: action, ConfirmProviderStopped: confirm})
						validRelease := state == "terminated"
						liveResume := state == "live" && action == outcome.RecoveryActionReconcile
						if validRelease {
							if err != nil {
								t.Fatal(err)
							}
							if got.Attempt.Fence != nil {
								t.Fatal("terminated legacy retained fence")
							}
							if !unavailable {
								assertReplacementStartable(t, svc, spawner, outcomeID, planID)
							}
							return
						}
						if liveResume {
							if err != nil {
								t.Fatal(err)
							}
							if got.Attempt.Fence == nil || got.Attempt.Attempt.Status != before {
								t.Fatal("live reconcile changed custody/status")
							}
							return
						}
						if err == nil {
							t.Fatal("legacy uncertainty did not refuse")
						}
						if gotCode := requireAPICode(t, err); gotCode != outcome.CodeAttemptCustodyUnproven {
							t.Fatalf("code=%s", gotCode)
						}
						held, getErr := svc.GetAttempt(ctx, outcomeID, view.Attempt.ID)
						if getErr != nil {
							t.Fatal(getErr)
						}
						if held.Attempt.Status != before || held.Fence == nil || !held.Fence.Open() {
							t.Fatalf("status/fence changed: %+v", held)
						}
						if len(held.Receipts) == 0 || held.Receipts[len(held.Receipts)-1].Resolution != domain.RecoveryNeedsAttention {
							t.Fatal("needs-attention receipt missing")
						}
					})
				}
			}
		}
	}
}
