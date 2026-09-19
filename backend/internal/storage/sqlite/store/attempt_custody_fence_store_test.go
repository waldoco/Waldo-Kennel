package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
)

func custodyFenceFixture(attemptID domain.AttemptID, at time.Time) domain.AttemptCustodyFence {
	return domain.AttemptCustodyFence{
		AttemptID: attemptID,
		SessionID: "ses-99999999-9999-9999-9999-999999999999",
		Detail:    "provider exit observed; session terminated",
		FencedAt:  at,
	}
}

func TestAttemptCustodyFenceRoundTrip(t *testing.T) {
	s := sqlitetest.MustOpen(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 20, 2, 0, 0, 0, time.UTC)
	plan, outcomeID := seedApprovedPlan(t, s, "fence-round-trip")
	attempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-fence-1", domain.FenceSubjectForProject("fence-round-trip")))
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	fence := custodyFenceFixture(attempt.ID, at)
	if err := s.RecordAttemptCustodyFence(ctx, fence); err != nil {
		t.Fatalf("record fence: %v", err)
	}
	got, found, err := s.GetAttemptCustodyFence(ctx, attempt.ID)
	if err != nil || !found {
		t.Fatalf("read fence: found=%v err=%v", found, err)
	}
	if got.AttemptID != fence.AttemptID || got.SessionID != fence.SessionID || got.Detail != fence.Detail {
		t.Fatalf("fence = %+v, want %+v", got, fence)
	}
	if _, found, err := s.GetAttemptCustodyFence(ctx, "att-00000000-0000-0000-0000-000000000000"); err != nil || found {
		t.Fatalf("unknown fence: found=%v err=%v", found, err)
	}
}

// The fence is exactly-once: a replayed custody close after a daemon restart
// must read the existing fence, never write a second one.
func TestAttemptCustodyFenceIsExactlyOnce(t *testing.T) {
	s := sqlitetest.MustOpen(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 20, 2, 0, 0, 0, time.UTC)
	plan, outcomeID := seedApprovedPlan(t, s, "fence-once")
	attempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-fence-2", domain.FenceSubjectForProject("fence-once")))
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	if err := s.RecordAttemptCustodyFence(ctx, custodyFenceFixture(attempt.ID, at)); err != nil {
		t.Fatalf("record fence: %v", err)
	}
	err = s.RecordAttemptCustodyFence(ctx, custodyFenceFixture(attempt.ID, at.Add(time.Minute)))
	if !errors.Is(err, ports.ErrAttemptCustodyFenceSealed) {
		t.Fatalf("second fence err = %v, want ErrAttemptCustodyFenceSealed", err)
	}
	got, found, err := s.GetAttemptCustodyFence(ctx, attempt.ID)
	if err != nil || !found {
		t.Fatalf("read fence: found=%v err=%v", found, err)
	}
	if !got.FencedAt.Equal(at) {
		t.Fatalf("fence rewritten: fenced-at = %v, want %v", got.FencedAt, at)
	}
}

// Erasing an Outcome erases its custody fences with it.
func TestAttemptCustodyFenceErasesWithOutcome(t *testing.T) {
	s := sqlitetest.MustOpen(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 20, 2, 0, 0, 0, time.UTC)
	plan, outcomeID := seedApprovedPlan(t, s, "fence-erasure")
	attempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-fence-3", domain.FenceSubjectForProject("fence-erasure")))
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	if err := s.RecordAttemptCustodyFence(ctx, custodyFenceFixture(attempt.ID, at)); err != nil {
		t.Fatalf("record fence: %v", err)
	}
	if _, err := s.TransitionAttemptStatus(ctx, outcomeID, attempt.ID, domain.AttemptQueued, domain.AttemptFailed, at); err != nil {
		t.Fatalf("fail attempt: %v", err)
	}
	if _, err := s.ReleaseFenceForAttempt(ctx, attempt.ID, "test-done", at); err != nil {
		t.Fatalf("release fence: %v", err)
	}
	preview, err := s.PreviewOutcomeDeletion(ctx, outcomeID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(preview.Blockers) > 0 {
		t.Fatalf("preview blockers: %+v", preview.Blockers)
	}
	if err := s.ChangeOutcomeTrash(ctx, outcomeID, preview.Revision, true); err != nil {
		t.Fatalf("trash: %v", err)
	}
	if err := s.BeginOutcomePurge(ctx, outcomeID, preview.Revision); err != nil {
		t.Fatalf("begin purge: %v", err)
	}
	if err := s.PurgeOutcomeRecords(ctx, outcomeID, preview.Revision); err != nil {
		t.Fatalf("purge with a custody fence must succeed: %v", err)
	}
	if _, found, err := s.GetAttemptCustodyFence(ctx, attempt.ID); err != nil || found {
		t.Fatalf("fence after erasure: found=%v err=%v, want gone", found, err)
	}
}
