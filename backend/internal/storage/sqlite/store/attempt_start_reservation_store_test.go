package store_test

import (
	"context"
	"errors"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"sync"
	"testing"
	"time"
)

type reservationAttachment struct{ fail bool }

func (a reservationAttachment) CreateInAttemptStartTransaction(ctx context.Context, tx ports.AttemptStartReservationTx, x ports.AttemptStartEscalationAttachment) error {
	if a.fail {
		return errors.New("injected escalation failure")
	}
	_, err := tx.SQLTx().ExecContext(ctx, `INSERT INTO attempt_observations(id,attempt_id,seq,kind,payload,created_at) VALUES(?,?,1,'capability_escalation_attached','{}',?)`, "obs-"+x.ReservationID, x.AttemptID, time.Now())
	return err
}
func reservationValue(t *testing.T, out domain.OutcomeID, plan domain.PlanRevision, key string) domain.AttemptStartReservation {
	t.Helper()
	d, err := domain.NewCapabilityDenialDetail(plan.WorkUnits[0].ID, []string{domain.CapabilityWorktreeWrite}, domain.AdmissionDenialRoutingCandidate, "snap", "route-gen", domain.ExecutionBinding{Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault}, "codex", domain.HarnessCodex)
	if err != nil {
		t.Fatal(err)
	}
	return domain.AttemptStartReservation{ID: "res-" + key, AttemptID: domain.AttemptID("att-" + key), OutcomeID: out, PlanRevisionID: plan.ID, WorkUnitID: plan.WorkUnits[0].ID, ContractRevisionNumber: plan.ContractRevisionNumber, RequestKey: key, RequestFingerprint: "fp-" + key, RoutingSnapshotID: "snap", RoutingGenerationID: "route-gen", AdmissionEvaluationID: "eval-" + key, RefusalStatus: domain.AttemptStartRefusalOpen, Denial: d, CreatedAt: time.Now().UTC().Truncate(time.Second)}
}
func TestAttemptStartReservationAtomicReplayAndNoCustody(t *testing.T) {
	s := newTestStore(t)
	plan, out := seedApprovedPlan(t, s, "start-reservation")
	r := reservationValue(t, out, plan, "key")
	got, created, err := s.ReserveAttemptStart(context.Background(), ports.AttemptStartReservationRequest{Reservation: r}, reservationAttachment{})
	if err != nil || !created || got.AttemptID != r.AttemptID {
		t.Fatalf("reserve created=%v got=%+v err=%v", created, got, err)
	}
	got, created, err = s.ReserveAttemptStart(context.Background(), ports.AttemptStartReservationRequest{Reservation: r}, reservationAttachment{})
	if err != nil || created || got.ID != r.ID {
		t.Fatalf("replay created=%v got=%+v err=%v", created, got, err)
	}
	attempts, err := s.ListAttempts(context.Background(), out)
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempts=%v err=%v", attempts, err)
	}
	sessions, err := s.ListAttemptSessionRefs(context.Background(), r.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	_, fenceFound, err := s.OpenFenceForSubject(context.Background(), domain.FenceSubjectForProject("start-reservation"))
	if err != nil {
		t.Fatal(err)
	}
	if attempts[0].Status != domain.AttemptAwaitingAuthority || len(sessions) != 0 || fenceFound {
		t.Fatalf("status=%s sessions=%d fence=%v", attempts[0].Status, len(sessions), fenceFound)
	}
}
func TestAttemptStartReservationRollbackWhenEscalationFails(t *testing.T) {
	s := newTestStore(t)
	plan, out := seedApprovedPlan(t, s, "start-rollback")
	r := reservationValue(t, out, plan, "rollback")
	if _, _, err := s.ReserveAttemptStart(context.Background(), ports.AttemptStartReservationRequest{Reservation: r}, reservationAttachment{fail: true}); err == nil {
		t.Fatal("expected failure")
	}
	attempts, err := s.ListAttempts(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	_, found, err := s.FindAttemptStartReservation(context.Background(), r.RequestKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 0 || found {
		t.Fatalf("partial commit attempts=%d reservation=%v", len(attempts), found)
	}
}
func TestAttemptStartReservationConcurrentDuplicatesConverge(t *testing.T) {
	s := newTestStore(t)
	plan, out := seedApprovedPlan(t, s, "start-race")
	r := reservationValue(t, out, plan, "race")
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := s.ReserveAttemptStart(context.Background(), ports.AttemptStartReservationRequest{Reservation: r}, reservationAttachment{})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	attempts, err := s.ListAttempts(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	_, found, err := s.FindAttemptStartReservation(context.Background(), r.RequestKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || !found {
		t.Fatalf("attempts=%d found=%v", len(attempts), found)
	}
}

func TestAttemptStartReservationSameKeyDifferentFingerprintConflicts(t *testing.T) {
	s := newTestStore(t)
	plan, out := seedApprovedPlan(t, s, "start-conflict")
	r := reservationValue(t, out, plan, "same-key")
	if _, _, err := s.ReserveAttemptStart(context.Background(), ports.AttemptStartReservationRequest{Reservation: r}, reservationAttachment{}); err != nil {
		t.Fatal(err)
	}
	changed := r
	changed.RequestFingerprint = "different"
	if _, _, err := s.ReserveAttemptStart(context.Background(), ports.AttemptStartReservationRequest{Reservation: changed}, reservationAttachment{}); err == nil {
		t.Fatal("same key with different fingerprint passed")
	} else {
		var conflict *ports.AttemptStartReplayConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("wrong error %T: %v", err, err)
		}
	}
}
