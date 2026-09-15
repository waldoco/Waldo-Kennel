package store_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
)

// appendWithBusyRetry delivers one cumulative sample, retrying transient
// SQLITE_BUSY_SNAPSHOT upgrades (a peer process committed between this tx's
// read and its write upgrade). Cumulative counters make a dropped attempt
// self-healing on the next sample, and the retry must land exact totals.
func appendWithBusyRetry(t *testing.T, s *sqlite.Store, sample domain.ExecutionUsageSample) {
	t.Helper()
	for attempt := 0; attempt < 12; attempt++ {
		_, _, err := s.AppendAttemptExecutionUsage(context.Background(), sample)
		if err == nil {
			return
		}
		var busy *ports.ExecutionUsageBusyError
		if errors.As(err, &busy) {
			time.Sleep(time.Duration(10+attempt*15) * time.Millisecond)
			continue
		}
		// A newer sequence from the peer already committed: this older sample
		// is redundant (its tokens are inside the newer row's delta), so the
		// fail-closed non-monotonic rejection is the correct outcome. The
		// exact-totals assertion below is the invariant under test.
		if strings.Contains(err.Error(), "not monotonic") {
			t.Logf("seq %d rejected as stale after peer committed newer: %v", sample.Sequence, err)
			return
		}
		t.Fatalf("unexpected append error for seq %d: %v", sample.Sequence, err)
	}
	t.Fatalf("seq %d never landed after busy retries", sample.Sequence)
}

// openPeerStore opens a second store on the same DB file: a separate writeMu
// and connection pool, simulating a second daemon process.
func openPeerStore(t *testing.T, dir string) *sqlite.Store {
	t.Helper()
	s, err := sqlite.Open(dir)
	if err != nil {
		t.Fatalf("open peer store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// Two processes race ADJACENT sequences. If the append tx reads the latest row
// before the peer's commit (deferred tx, no write lock held during the read),
// its delta is computed against a stale baseline and the lineage sum
// over-counts: an early, false budget trigger.
func TestCrossProcessUsageLedgerNeverOverCounts(t *testing.T) {
	dir := t.TempDir()
	s1 := sqlitetest.MustOpenAt(t, dir)
	s2 := openPeerStore(t, dir)
	ctx := context.Background()
	plan, out := seedApprovedPlan(t, s1, "xproc-usage")
	a, err := s1.CreateAttemptWithFence(ctx, ports.AttemptAdmission{OutcomeID: out, PlanRevisionID: plan.ID, WorkUnitID: plan.WorkUnits[0].ID, ContractRevisionNumber: plan.ContractRevisionNumber, RequestKey: "xp-1", FenceSubject: "project:xp", At: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s1.TransitionAttemptStatus(ctx, out, a.ID, domain.AttemptQueued, domain.AttemptRunning, time.Now()); err != nil {
		t.Fatal(err)
	}
	const rounds = 60
	var wg sync.WaitGroup
	for r := 1; r <= rounds; r += 2 {
		wg.Add(2)
		go func(seq int64) {
			defer wg.Done()
			appendWithBusyRetry(t, s1, domain.ExecutionUsageSample{AttemptID: a.ID, Provider: domain.HarnessCodex, SessionID: "s", Sequence: seq, InputTokens: seq * 10, OutputTokens: seq * 5, CreatedAt: time.Now()})
		}(int64(r))
		go func(seq int64) {
			defer wg.Done()
			appendWithBusyRetry(t, s2, domain.ExecutionUsageSample{AttemptID: a.ID, Provider: domain.HarnessCodex, SessionID: "s", Sequence: seq, InputTokens: seq * 10, OutputTokens: seq * 5, CreatedAt: time.Now()})
		}(int64(r + 1))
		wg.Wait()
	}
	totals, err := s1.WorkUnitExecutionUsage(ctx, plan.WorkUnits[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	wantIn, wantOut := int64(rounds*10), int64(rounds*5)
	if totals.InputTokens != wantIn || totals.OutputTokens != wantOut {
		t.Fatalf("lineage totals=(%d,%d), want exactly (%d,%d): cross-process write skew over-counted", totals.InputTokens, totals.OutputTokens, wantIn, wantOut)
	}
}

// The retry allowance race: allowance admits exactly one replacement; two
// processes race for it. The fence unique constraint must force the loser's
// whole tx (including its attempt row) to roll back.
func TestCrossProcessRetryAllowanceAdmitsExactlyOne(t *testing.T) {
	dir := t.TempDir()
	s1 := sqlitetest.MustOpenAt(t, dir)
	s2 := openPeerStore(t, dir)
	ctx := context.Background()
	plan, out := seedApprovedPlan(t, s1, "xproc-retry")
	limit := 1
	mk := func(key string) ports.AttemptAdmission {
		return ports.AttemptAdmission{OutcomeID: out, PlanRevisionID: plan.ID, WorkUnitID: plan.WorkUnits[0].ID, ContractRevisionNumber: plan.ContractRevisionNumber, RequestKey: key, FenceSubject: "project:xr", RetryLimit: &limit, At: time.Now()}
	}
	first, err := s1.CreateAttemptWithFence(ctx, mk("xr-1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s1.TransitionAttemptStatus(ctx, out, first.ID, domain.AttemptQueued, domain.AttemptFailed, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := s1.ReleaseFenceForAttempt(ctx, first.ID, "failed", time.Now()); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); _, err := s1.CreateAttemptWithFence(ctx, mk("xr-2a")); results <- err }()
	go func() { defer wg.Done(); _, err := s2.CreateAttemptWithFence(ctx, mk("xr-2b")); results <- err }()
	wg.Wait()
	close(results)
	var succeeded, failed int
	for err := range results {
		if err == nil {
			succeeded++
		} else {
			failed++
			t.Logf("loser error: %v", err)
		}
	}
	if succeeded != 1 || failed != 1 {
		t.Fatalf("succeeded=%d failed=%d, want exactly one winner", succeeded, failed)
	}
	attempts, err := s1.ListAttempts(ctx, out)
	if err != nil || len(attempts) != 2 {
		t.Fatalf("attempts=%d err=%v, want exactly 2 (loser rolled back)", len(attempts), err)
	}
	// The single allowed replacement is now used up: a third start from either
	// process must hit the budget error, never create a row.
	if _, err := s2.CreateAttemptWithFence(ctx, mk("xr-3")); err == nil {
		t.Fatal("third attempt admitted beyond allowance")
	}
	if attempts, _ := s1.ListAttempts(ctx, out); len(attempts) != 2 {
		t.Fatalf("attempts=%d after refused start, want 2", len(attempts))
	}
}

// A conflicting cross-process claim must be rejected and the durable claim
// preserved; convergence (record provider-stopped) is then idempotent for the
// identical machine result and conflicts on a different one.
func TestCrossProcessBudgetStopClaimConflictPreservesDurable(t *testing.T) {
	dir := t.TempDir()
	s1 := sqlitetest.MustOpenAt(t, dir)
	s2 := openPeerStore(t, dir)
	ctx := context.Background()
	plan, out := seedApprovedPlan(t, s1, "xproc-claim")
	a, err := s1.CreateAttemptWithFence(ctx, ports.AttemptAdmission{OutcomeID: out, PlanRevisionID: plan.ID, WorkUnitID: plan.WorkUnits[0].ID, ContractRevisionNumber: plan.ContractRevisionNumber, RequestKey: "xc-1", FenceSubject: "project:xc", At: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	c1 := domain.AttemptBudgetStop{AttemptID: a.ID, SessionID: "s", Reason: domain.RuntimeTokenBudgetExhausted, MeasuredUsage: `{"inputTokens":100}`, ClaimedAt: time.Now()}
	if _, created, err := s1.ClaimAttemptBudgetStop(ctx, c1); err != nil || !created {
		t.Fatalf("first claim created=%v err=%v", created, err)
	}
	c2 := c1
	c2.MeasuredUsage = `{"inputTokens":120}`
	if converged, created, err := s2.ClaimAttemptBudgetStop(ctx, c2); err != nil || created || converged.MeasuredUsage != `{"inputTokens":100}` {
		t.Fatalf("same-reason claim did not converge to durable winner: %+v created=%v err=%v", converged, created, err)
	}
	got, ok, err := s2.GetAttemptBudgetStop(ctx, a.ID)
	if err != nil || !ok || got.MeasuredUsage != `{"inputTokens":100}` {
		t.Fatalf("durable claim=%+v ok=%v err=%v", got, ok, err)
	}
	if _, err := s2.RecordAttemptBudgetProviderStopped(ctx, a.ID, "s", c1.Reason, `{"providerStopped":true}`, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := s1.RecordAttemptBudgetProviderStopped(ctx, a.ID, "s", c1.Reason, `{"providerStopped":true}`, time.Now()); err != nil {
		t.Fatalf("idempotent cross-process machine result: %v", err)
	}
	if _, err := s1.RecordAttemptBudgetProviderStopped(ctx, a.ID, "s", c1.Reason, `{"providerStopped":false}`, time.Now()); err == nil {
		t.Fatal("conflicting machine result accepted after stop")
	}
}
