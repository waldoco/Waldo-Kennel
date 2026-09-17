package store_test

import (
	"context"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"testing"
	"time"
)

func TestBudgetStopClaimAndMachineResultAreDurableIdempotent(t *testing.T) {
	s := newTestStore(t)
	plan, out := seedApprovedPlan(t, s, "budget-stop")
	ctx := context.Background()
	a, err := s.CreateAttemptWithFence(ctx, ports.AttemptAdmission{OutcomeID: out, PlanRevisionID: plan.ID, WorkUnitID: plan.WorkUnits[0].ID, ContractRevisionNumber: 1, RequestKey: "r", FenceSubject: "p", At: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	claim := domain.AttemptBudgetStop{AttemptID: a.ID, SessionID: "s", Reason: domain.RuntimeTokenBudgetExhausted, MeasuredUsage: `{"inputTokens":5}`, ClaimedAt: time.Now()}
	got, created, err := s.ClaimAttemptBudgetStop(ctx, claim)
	if err != nil || !created {
		t.Fatalf("%+v %v %v", got, created, err)
	}
	_, created, err = s.ClaimAttemptBudgetStop(ctx, claim)
	if err != nil || created {
		t.Fatalf("replay %v %v", created, err)
	}
	if _, _, err = s.ClaimAttemptBudgetStop(ctx, domain.AttemptBudgetStop{AttemptID: a.ID, SessionID: "other", Reason: claim.Reason, MeasuredUsage: claim.MeasuredUsage, ClaimedAt: claim.ClaimedAt}); err == nil {
		t.Fatal("changed claim accepted")
	}
	stopped := time.Now()
	got, err = s.RecordAttemptBudgetProviderStopped(ctx, a.ID, "s", claim.Reason, `{"providerStopped":true}`, stopped)
	if err != nil || !got.ProviderStopped() {
		t.Fatalf("%+v %v", got, err)
	}
	got, err = s.RecordAttemptBudgetProviderStopped(ctx, a.ID, "s", claim.Reason, `{"providerStopped":true}`, stopped)
	if err != nil || !got.ProviderStopped() {
		t.Fatalf("replay %+v %v", got, err)
	}
}
