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

type reservationAttachment struct {
	fail                 bool
	insertThenFail       bool
	rollbackBeforeCommit bool
}

func (a reservationAttachment) CreateInAttemptStartTransaction(ctx context.Context, tx ports.AttemptStartReservationTx, x ports.AttemptStartEscalationAttachment) error {
	if a.fail {
		return errors.New("injected escalation failure")
	}
	_, err := tx.SQLTx().ExecContext(ctx, `INSERT INTO attempt_observations(id,attempt_id,seq,kind,payload,created_at) VALUES(?,?,1,'capability_escalation_attached','{}',?)`, "obs-"+x.ReservationID, x.AttemptID, time.Now())
	if err == nil && a.insertThenFail {
		return errors.New("injected failure after escalation insert")
	}
	if err == nil && a.rollbackBeforeCommit {
		return tx.SQLTx().Rollback()
	}
	return err
}
func reservationValue(t *testing.T, out domain.OutcomeID, plan domain.PlanRevision, key string) domain.AttemptStartReservation {
	t.Helper()
	d, err := domain.NewCapabilityDenialDetail(plan.WorkUnits[0].ID, []string{domain.CapabilityWorktreeWrite}, domain.AdmissionDenialRoutingCandidate, "snap", "route-gen", domain.ExecutionBinding{Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault}, "codex", domain.HarnessCodex)
	if err != nil {
		t.Fatal(err)
	}
	r := domain.AttemptStartReservation{ID: "res-" + key, AttemptID: domain.AttemptID("att-" + key), OutcomeID: out, PlanRevisionID: plan.ID, WorkUnitID: plan.WorkUnits[0].ID, ContractRevisionNumber: plan.ContractRevisionNumber, RequestKey: key, RoutingSnapshotID: "snap", RoutingGenerationID: "route-gen", AdmissionEvaluationID: "eval-" + key, RefusalStatus: domain.AttemptStartRefusalOpen, Denial: d, CreatedAt: time.Now().UTC().Truncate(time.Second)}
	if err := r.SetRequestFingerprint(); err != nil {
		t.Fatal(err)
	}
	return r
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
	if _, _, err := s.ReserveAttemptStart(context.Background(), ports.AttemptStartReservationRequest{Reservation: r}, reservationAttachment{insertThenFail: true}); err == nil {
		t.Fatal("expected failure after escalation insert")
	}
	observations, err := s.ListAttemptObservations(context.Background(), r.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 0 {
		t.Fatalf("rolled-back escalation observations=%d", len(observations))
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
	changed.AttemptID = "tampered-attempt"
	if err := changed.SetRequestFingerprint(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ReserveAttemptStart(context.Background(), ports.AttemptStartReservationRequest{Reservation: changed}, reservationAttachment{}); err == nil {
		t.Fatal("same key with different fingerprint passed")
	} else {
		var conflict *ports.AttemptStartReplayConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("wrong error %T: %v", err, err)
		}
	}
}

func TestAwaitingAuthorityRejectsSessionBinding(t *testing.T) {
	s := newTestStore(t)
	plan, out := seedApprovedPlan(t, s, "start-custody-guards")
	r := reservationValue(t, out, plan, "custody")
	if _, _, err := s.ReserveAttemptStart(context.Background(), ports.AttemptStartReservationRequest{Reservation: r}, reservationAttachment{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BindAttemptSession(context.Background(), domain.AttemptSessionRef{AttemptID: r.AttemptID, SessionID: "provider", Harness: domain.HarnessCodex, Mode: domain.SessionModeTUI, RunBriefCoreDigest: plan.RunBriefCoreDigest, AdmissionSnapshot: `{}`}); err == nil {
		t.Fatal("session binding passed")
	}
}

func TestAttemptStartReservationSameFingerprintTamperingRejected(t *testing.T) {
	s := newTestStore(t)
	plan, out := seedApprovedPlan(t, s, "start-tamper")
	r := reservationValue(t, out, plan, "tamper")
	if _, _, err := s.ReserveAttemptStart(context.Background(), ports.AttemptStartReservationRequest{Reservation: r}, reservationAttachment{}); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*domain.AttemptStartReservation){
		"attempt":     func(x *domain.AttemptStartReservation) { x.AttemptID = "other" },
		"reservation": func(x *domain.AttemptStartReservation) { x.ID = "other" },
		"denial": func(x *domain.AttemptStartReservation) {
			x.Denial.MissingCapabilities = []string{domain.CapabilityWorktreeExec}
		},
		"status": func(x *domain.AttemptStartReservation) { x.RefusalStatus = domain.AttemptStartRefusalDenied },
		"time":   func(x *domain.AttemptStartReservation) { x.CreatedAt = x.CreatedAt.Add(time.Second) },
	} {
		changed := r
		mutate(&changed)
		if _, _, err := s.ReserveAttemptStart(context.Background(), ports.AttemptStartReservationRequest{Reservation: changed}, reservationAttachment{}); err == nil {
			t.Fatalf("%s tampering passed", name)
		}
	}
}

func TestAttemptStartReservationRollbackAtEveryBoundary(t *testing.T) {
	s := newTestStore(t)
	plan, out := seedApprovedPlan(t, s, "start-boundaries")
	first := reservationValue(t, out, plan, "first")
	if _, _, err := s.ReserveAttemptStart(context.Background(), ports.AttemptStartReservationRequest{Reservation: first}, reservationAttachment{}); err != nil {
		t.Fatal(err)
	}
	// The second Attempt insert succeeds, then the duplicate evaluation id makes
	// the reservation insert fail. The outer transaction must remove the Attempt.
	second := reservationValue(t, out, plan, "after-attempt")
	second.AdmissionEvaluationID = first.AdmissionEvaluationID
	if err := second.SetRequestFingerprint(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ReserveAttemptStart(context.Background(), ports.AttemptStartReservationRequest{Reservation: second}, reservationAttachment{}); err == nil {
		t.Fatal("expected reservation insert failure")
	}
	attempts, err := s.ListAttempts(context.Background(), out)
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempts after reservation failure=%d err=%v", len(attempts), err)
	}
	// Roll back from the transaction attachment immediately before Store.Commit.
	third := reservationValue(t, out, plan, "before-commit")
	if _, _, err := s.ReserveAttemptStart(context.Background(), ports.AttemptStartReservationRequest{Reservation: third}, reservationAttachment{rollbackBeforeCommit: true}); err == nil {
		t.Fatal("expected commit failure")
	}
	if _, found, err := s.FindAttemptStartReservation(context.Background(), third.RequestKey); err != nil || found {
		t.Fatalf("reservation after commit failure found=%v err=%v", found, err)
	}
	observations, err := s.ListAttemptObservations(context.Background(), third.AttemptID)
	if err != nil || len(observations) != 0 {
		t.Fatalf("escalations after commit failure=%d err=%v", len(observations), err)
	}
	attempts, err = s.ListAttempts(context.Background(), out)
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempts after commit failure=%d err=%v", len(attempts), err)
	}
}
