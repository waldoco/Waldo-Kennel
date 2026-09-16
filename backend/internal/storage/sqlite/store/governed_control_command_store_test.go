package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

func controlClaim(session domain.SessionID, id, key, fingerprint string, at time.Time) domain.GovernedControlCommand {
	return domain.GovernedControlCommand{ID: id, SessionID: session, IdempotencyKey: key, RequestFingerprint: fingerprint, Class: domain.GovernedControlSteer, State: domain.GovernedCommandClaimed, ControllerGeneration: "gen-1", ExpectedRevision: "plan-1", CapabilityFingerprint: "caps-1", ProviderConversationID: "thread-1", ClientMessageID: "message-1", ProviderTurnID: "turn-1", Quiescence: domain.GovernedCommandQuiescenceNotApplicable, CreatedAt: at, UpdatedAt: at}
}

func TestGovernedControlClaimIsIdempotentAndConflictsOnChangedRequest(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "control-claim")
	session, err := s.CreateSession(ctx, sampleRecord("control-claim"))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Second)
	claim := controlClaim(session.ID, "control-1", "key-1", "fp-1", at)
	got, made, err := s.CreateGovernedControlCommandClaim(ctx, claim)
	if err != nil || !made || got != claim {
		t.Fatalf("first made=%v got=%+v err=%v", made, got, err)
	}
	got, made, err = s.CreateGovernedControlCommandClaim(ctx, claim)
	if err != nil || made || got != claim {
		t.Fatalf("retry made=%v got=%+v err=%v", made, got, err)
	}
	changed := claim
	changed.ID = "control-2"
	changed.RequestFingerprint = "fp-2"
	_, made, err = s.CreateGovernedControlCommandClaim(ctx, changed)
	if made || !errors.Is(err, domain.ErrGovernedCommandIdempotencyConflict) {
		t.Fatalf("conflict made=%v err=%v", made, err)
	}
}

func TestGovernedControlClaimConcurrentDuplicateAndConflictHaveOneWinner(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "control-race")
	session, err := s.CreateSession(ctx, sampleRecord("control-race"))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Second)
	first := controlClaim(session.ID, "control-a", "same-key", "same-fingerprint", at)

	const workers = 12
	var wg sync.WaitGroup
	wg.Add(workers)
	created := make(chan bool, workers)
	errs := make(chan error, workers)
	for range workers {
		go func() {
			defer wg.Done()
			got, made, callErr := s.CreateGovernedControlCommandClaim(ctx, first)
			if callErr == nil && got.ID != first.ID {
				callErr = errors.New("duplicate returned wrong control claim")
			}
			created <- made
			errs <- callErr
		}()
	}
	wg.Wait()
	close(created)
	close(errs)
	winners := 0
	for made := range created {
		if made {
			winners++
		}
	}
	for callErr := range errs {
		if callErr != nil {
			t.Fatal(callErr)
		}
	}
	if winners != 1 {
		t.Fatalf("created winners=%d, want 1", winners)
	}

	conflict := first
	conflict.ID = "control-b"
	conflict.RequestFingerprint = "changed-fingerprint"
	got, made, err := s.CreateGovernedControlCommandClaim(ctx, conflict)
	if made || !errors.Is(err, domain.ErrGovernedCommandIdempotencyConflict) || got.ID != first.ID {
		t.Fatalf("conflict made=%v existing=%+v err=%v", made, got, err)
	}
}

func TestGovernedControlTransitionFencesAndRetainsUnknown(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "control-transition")
	rec := sampleRecord("control-transition")
	rec.Mode = domain.SessionModeChat
	session, err := s.CreateSession(ctx, rec)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Second)
	claim := controlClaim(session.ID, "control-1", "key-1", "fp-1", at)
	if err := s.ClaimChatControllerGeneration(ctx, session.ID, claim.ControllerGeneration, at); err != nil {
		t.Fatal(err)
	}
	if _, made, err := s.CreateGovernedControlCommandClaim(ctx, claim); err != nil || !made {
		t.Fatal(err)
	}
	next := claim
	next.State = domain.GovernedCommandDispatching
	next.UpdatedAt = at.Add(time.Second)
	ok, err := s.AdvanceGovernedControlCommand(ctx, next, domain.GovernedCommandClaimed, next.ControllerGeneration, next.ExpectedRevision, next.CapabilityFingerprint)
	if err != nil || !ok {
		t.Fatalf("dispatch ok=%v err=%v", ok, err)
	}
	next.State = domain.GovernedCommandDeliveryUnknown
	next.UpdatedAt = at.Add(2 * time.Second)
	ok, err = s.AdvanceGovernedControlCommand(ctx, next, domain.GovernedCommandDispatching, next.ControllerGeneration, next.ExpectedRevision, next.CapabilityFingerprint)
	if err != nil || !ok {
		t.Fatalf("unknown ok=%v err=%v", ok, err)
	}
	rows, err := s.ListUnsettledGovernedControlCommands(ctx)
	if err != nil || len(rows) != 1 || rows[0].State != domain.GovernedCommandDeliveryUnknown {
		t.Fatalf("unsettled=%+v err=%v", rows, err)
	}
	stale := next
	stale.State = domain.GovernedCommandReconciled
	stale.UpdatedAt = at.Add(3 * time.Second)
	ok, err = s.AdvanceGovernedControlCommand(ctx, stale, domain.GovernedCommandDeliveryUnknown, "stale", stale.ExpectedRevision, stale.CapabilityFingerprint)
	if err == nil || ok {
		t.Fatalf("stale ok=%v err=%v", ok, err)
	}
}

func TestGovernedControlClaimAdoptionIsPreDispatchAndGenerationFenced(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "control-adopt")
	rec := sampleRecord("control-adopt")
	rec.Mode = domain.SessionModeChat
	session, err := s.CreateSession(ctx, rec)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Second)
	claim := controlClaim(session.ID, "control-adopt-1", "key-adopt", "fp-adopt", at)
	if err := s.ClaimChatControllerGeneration(ctx, session.ID, claim.ControllerGeneration, at); err != nil {
		t.Fatal(err)
	}
	if _, made, err := s.CreateGovernedControlCommandClaim(ctx, claim); err != nil || !made {
		t.Fatalf("create made=%v err=%v", made, err)
	}
	if err := s.ClaimChatControllerGeneration(ctx, session.ID, "gen-2", at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.AdoptClaimedGovernedControlCommandGeneration(ctx, claim, "stale-gen", at.Add(2*time.Second)); err != nil || ok {
		t.Fatalf("inactive generation adopted ok=%v err=%v", ok, err)
	}
	if ok, err := s.AdoptClaimedGovernedControlCommandGeneration(ctx, claim, "gen-2", at.Add(2*time.Second)); err != nil || !ok {
		t.Fatalf("active generation adoption ok=%v err=%v", ok, err)
	}
	adopted, found, err := s.GetGovernedControlCommand(ctx, claim.ID)
	if err != nil || !found || adopted.ControllerGeneration != "gen-2" || adopted.State != domain.GovernedCommandClaimed {
		t.Fatalf("adopted=%+v found=%v err=%v", adopted, found, err)
	}
	adopted.State = domain.GovernedCommandDispatching
	adopted.UpdatedAt = at.Add(3 * time.Second)
	if ok, err := s.AdvanceGovernedControlCommand(ctx, adopted, domain.GovernedCommandClaimed, "gen-2", adopted.ExpectedRevision, adopted.CapabilityFingerprint); err != nil || !ok {
		t.Fatalf("dispatch ok=%v err=%v", ok, err)
	}
	if ok, err := s.AdoptClaimedGovernedControlCommandGeneration(ctx, adopted, "gen-2", at.Add(4*time.Second)); !errors.Is(err, domain.ErrGovernedCommandInvalid) || ok {
		t.Fatalf("dispatching adoption ok=%v err=%v", ok, err)
	}
}
