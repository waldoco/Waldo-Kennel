package outcome_test

// W1.1 review seam: smallest viable fixture proving the succeeded-attempt
// custody contract through the public RecoverAttempt API. The harness fake
// lacks ports.AttemptReceiptStore, so outcome.New leaves Service.receipts
// nil and the entire AttemptSucceeded branch of reconcile/replace is
// unreachable in black-box tests. The wrapper below adds exactly the
// three-method receipt port in test code only; no production type changes.

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/intelligence/intelligencetest"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

type receiptAwareAttemptFakeStore struct {
	*attemptFakeStore
	rcMu   sync.Mutex
	rcByID map[domain.AttemptID]domain.AttemptReceipt
}

func (f *receiptAwareAttemptFakeStore) SaveAttemptReceipt(_ context.Context, r domain.AttemptReceipt) error {
	f.rcMu.Lock()
	defer f.rcMu.Unlock()
	if existing, ok := f.rcByID[r.AttemptID]; ok && existing.Frozen() {
		return nil // frozen receipts are never replaced
	}
	f.rcByID[r.AttemptID] = r
	return nil
}

func (f *receiptAwareAttemptFakeStore) GetAttemptReceipt(_ context.Context, id domain.AttemptID) (domain.AttemptReceipt, bool, error) {
	f.rcMu.Lock()
	defer f.rcMu.Unlock()
	r, ok := f.rcByID[id]
	return r, ok, nil
}

func (f *receiptAwareAttemptFakeStore) FreezeAttemptReceipt(_ context.Context, id domain.AttemptID, at time.Time) error {
	f.rcMu.Lock()
	defer f.rcMu.Unlock()
	r, ok := f.rcByID[id]
	if !ok {
		return nil
	}
	if r.FrozenAt == nil {
		r.FrozenAt = &at
		f.rcByID[id] = r
	}
	return nil
}

// newSucceededLegacyHarness mirrors newAttemptHarness, then drives the
// attempt into the crash shape W1.1 must repair: an older process classified
// the result, froze the receipt, and died BEFORE releasing its fence, on an
// attempt admitted pre-W1.1 (no launch packet).
func newSucceededLegacyHarness(t *testing.T) (*outcome.Service, *receiptAwareAttemptFakeStore, *fakeHeartbeats, domain.OutcomeID, domain.PlanRevisionID, domain.AttemptID, domain.SessionID) {
	t.Helper()
	store := &receiptAwareAttemptFakeStore{attemptFakeStore: newAttemptFakeStore(), rcByID: map[domain.AttemptID]domain.AttemptReceipt{}}
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
	started, err := svc.StartAttempt(ctx, view.Outcome.ID, startInput(planView.Plan.ID))
	if err != nil {
		t.Fatalf("start attempt: %v", err)
	}
	attemptID := started.Attempt.ID
	sid := domain.SessionID(started.Sessions[0].SessionID)

	// Crash shape: success classified + receipt frozen, fence never released.
	now := time.Now().UTC()
	store.mu.Lock()
	for i := range store.attempts[view.Outcome.ID] {
		if store.attempts[view.Outcome.ID][i].ID == attemptID {
			store.attempts[view.Outcome.ID][i].Status = domain.AttemptSucceeded
		}
	}
	store.launches = map[domain.AttemptID]domain.WorkspaceBoundLaunchPacket{} // legacy: no packet
	store.mu.Unlock()
	if err := store.SaveAttemptReceipt(ctx, domain.AttemptReceipt{
		AttemptID:              attemptID,
		OutcomeID:              view.Outcome.ID,
		PlanRevisionID:         planView.Plan.ID,
		WorkUnitID:             started.Attempt.WorkUnitID,
		ContractRevisionNumber: 1,
		ArtifactVersion:        "v1",
		RetentionState:         domain.RetentionRetained,
		ObservedAt:             now,
		CreatedAt:              now,
		UpdatedAt:              now,
	}); err != nil {
		t.Fatalf("save receipt: %v", err)
	}
	if err := store.FreezeAttemptReceipt(ctx, attemptID, now); err != nil {
		t.Fatalf("freeze receipt: %v", err)
	}
	return svc, store, heartbeats, view.Outcome.ID, planView.Plan.ID, attemptID, sid
}

// Live legacy provider + succeeded attempt = contradictory evidence. Custody
// must stay held, the attempt record immutable, the retained result intact.
func TestSucceededFrozenLiveLegacyRefusesRelease(t *testing.T) {
	svc, _, heartbeats, outcomeID, _, attemptID, sid := newSucceededLegacyHarness(t)
	heartbeats.signal(sid)
	ctx := context.Background()
	for _, action := range []outcome.RecoveryAction{outcome.RecoveryActionReconcile, outcome.RecoveryActionReplace} {
		t.Run(string(action), func(t *testing.T) {
			_, err := svc.RecoverAttempt(ctx, outcomeID, attemptID, outcome.RecoveryInput{Action: action, ConfirmProviderStopped: true})
			if err == nil {
				t.Fatal("live legacy succeeded attempt released custody")
			}
			if code := requireAPICode(t, err); code != outcome.CodeAttemptCustodyUnproven {
				t.Fatalf("code = %s", code)
			}
			held, getErr := svc.GetAttempt(ctx, outcomeID, attemptID)
			if getErr != nil {
				t.Fatal(getErr)
			}
			if held.Fence == nil || !held.Fence.Open() {
				t.Fatal("fence released for live legacy provider")
			}
			if held.Attempt.Status != domain.AttemptSucceeded {
				t.Fatalf("immutable succeeded record rewritten to %s", held.Attempt.Status)
			}
		})
	}
}

// Terminated legacy provider + frozen retained result: reconcile releases the
// stranded fence without touching the succeeded record or its receipt.
func TestSucceededFrozenTerminatedLegacyReleases(t *testing.T) {
	svc, store, heartbeats, outcomeID, _, attemptID, sid := newSucceededLegacyHarness(t)
	heartbeats.terminate(sid)
	ctx := context.Background()
	got, err := svc.RecoverAttempt(ctx, outcomeID, attemptID, outcome.RecoveryInput{Action: outcome.RecoveryActionReconcile})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if got.Attempt.Fence != nil {
		t.Fatal("terminated succeeded attempt retained its fence")
	}
	if got.Attempt.Attempt.Status != domain.AttemptSucceeded {
		t.Fatalf("succeeded record rewritten to %s", got.Attempt.Attempt.Status)
	}
	receipt, ok, err := store.GetAttemptReceipt(ctx, attemptID)
	if err != nil || !ok || !receipt.Frozen() || !receipt.RetentionState.Complete() {
		t.Fatalf("retained result damaged: ok=%v err=%v frozen=%v retention=%s", ok, err, receipt.Frozen(), receipt.RetentionState)
	}
}
