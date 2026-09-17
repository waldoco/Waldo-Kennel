package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

func governedCommandClaim(session domain.SessionID, id, key, fingerprint string, at time.Time) domain.GovernedCommandRecord {
	return domain.GovernedCommandRecord{
		GovernedCommandContract: domain.GovernedCommandContract{
			ID: id, SessionID: session, IdempotencyKey: key, RequestFingerprint: fingerprint,
			Class: domain.GovernedCommandTurn, State: domain.GovernedCommandClaimed,
			ControllerGeneration: "generation-7", ExpectedRevision: "revision-11",
			CapabilityFingerprint: "capabilities-v3",
			Correlation: domain.GovernedCommandCorrelation{
				ProviderConversationID: "provider-conversation-5", ClientMessageID: "message-19",
			},
			ReplayStrategy: domain.GovernedCommandReplayStableHistory,
			Quiescence:     domain.GovernedCommandQuiescencePending,
		},
		CreatedAt: at, UpdatedAt: at,
	}
}

func TestCreateGovernedCommandClaimIsIdempotentAndDurable(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "governed-claim")
	session, err := s.CreateSession(ctx, sampleRecord("governed-claim"))
	if err != nil {
		t.Fatal(err)
	}
	want := governedCommandClaim(session.ID, "command-1", "turn-key-1", "request-fingerprint-1", time.Now().UTC().Truncate(time.Second))

	got, created, err := s.CreateGovernedCommandClaim(ctx, want)
	if err != nil || !created || got.ID != want.ID {
		t.Fatalf("first claim: created=%v got=%+v err=%v", created, got, err)
	}
	got, created, err = s.CreateGovernedCommandClaim(ctx, want)
	if err != nil || created || got != want {
		t.Fatalf("retry: created=%v got=%+v want=%+v err=%v", created, got, want, err)
	}

	reopened, ok, err := s.GetGovernedCommand(ctx, want.ID)
	if err != nil || !ok || reopened != want {
		t.Fatalf("read claim: ok=%v got=%+v want=%+v err=%v", ok, reopened, want, err)
	}
}

func TestCreateGovernedCommandClaimRejectsIdempotencyKeyReuse(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "governed-conflict")
	session, err := s.CreateSession(ctx, sampleRecord("governed-conflict"))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Second)
	first := governedCommandClaim(session.ID, "command-a", "same-key", "fingerprint-a", at)
	if _, created, err := s.CreateGovernedCommandClaim(ctx, first); err != nil || !created {
		t.Fatalf("create first: created=%v err=%v", created, err)
	}
	conflicting := governedCommandClaim(session.ID, "command-b", "same-key", "fingerprint-b", at)
	existing, created, err := s.CreateGovernedCommandClaim(ctx, conflicting)
	if created || !errors.Is(err, domain.ErrGovernedCommandIdempotencyConflict) || existing.ID != first.ID {
		t.Fatalf("conflict: created=%v existing=%+v err=%v", created, existing, err)
	}
}

func TestCreateGovernedCommandClaimConcurrentSameKeyHasOneWinner(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "governed-race")
	session, err := s.CreateSession(ctx, sampleRecord("governed-race"))
	if err != nil {
		t.Fatal(err)
	}
	claim := governedCommandClaim(session.ID, "command-race", "race-key", "race-fingerprint", time.Now().UTC().Truncate(time.Second))

	const workers = 12
	var wg sync.WaitGroup
	wg.Add(workers)
	created := make(chan bool, workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			got, made, err := s.CreateGovernedCommandClaim(ctx, claim)
			if err == nil && got.ID != claim.ID {
				err = errors.New("retry returned wrong command")
			}
			created <- made
			errs <- err
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
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("created winners=%d, want 1", winners)
	}
}

func TestCreateGovernedCommandClaimRejectsNonClaimedOrMismatchedTimes(t *testing.T) {
	at := time.Now().UTC().Truncate(time.Second)
	claim := governedCommandClaim("session", "command", "key", "fingerprint", at)
	claim.State = domain.GovernedCommandDispatching
	s := newTestStore(t)
	if _, _, err := s.CreateGovernedCommandClaim(context.Background(), claim); !errors.Is(err, domain.ErrGovernedCommandInvalid) {
		t.Fatalf("dispatching claim err=%v", err)
	}
	claim.State = domain.GovernedCommandClaimed
	claim.UpdatedAt = at.Add(time.Second)
	if _, _, err := s.CreateGovernedCommandClaim(context.Background(), claim); !errors.Is(err, domain.ErrGovernedCommandInvalid) {
		t.Fatalf("mismatched timestamps err=%v", err)
	}
}

func advanceGovernedCommand(t *testing.T, s interface {
	AdvanceGovernedCommand(context.Context, domain.GovernedCommandRecord, domain.GovernedCommandState, string, string, string) (bool, error)
}, rec *domain.GovernedCommandRecord, next domain.GovernedCommandState, at time.Time, mutate func(*domain.GovernedCommandRecord)) bool {
	t.Helper()
	expected := rec.State
	if mutate != nil {
		mutate(rec)
	}
	rec.State = next
	rec.UpdatedAt = at
	ok, err := s.AdvanceGovernedCommand(context.Background(), *rec, expected, rec.ControllerGeneration, rec.ExpectedRevision, rec.CapabilityFingerprint)
	if err != nil {
		t.Fatalf("advance %s -> %s: %v", expected, next, err)
	}
	return ok
}

func TestGovernedCommandTransitionsFenceStaleWritersAndRetainUnknownForRecovery(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "governed-transition")
	rec := sampleRecord("governed-transition")
	rec.Mode = domain.SessionModeChat
	session, err := s.CreateSession(ctx, rec)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Second)
	claim := governedCommandClaim(session.ID, "command-transition", "transition-key", "transition-fingerprint", at)
	if err := s.ClaimChatControllerGeneration(ctx, session.ID, claim.ControllerGeneration, "", "", at); err != nil {
		t.Fatal(err)
	}
	if _, created, err := s.CreateGovernedCommandClaim(ctx, claim); err != nil || !created {
		t.Fatalf("claim: created=%v err=%v", created, err)
	}

	stale := claim
	stale.State = domain.GovernedCommandDispatching
	stale.UpdatedAt = at.Add(time.Second)
	stale.ControllerGeneration = "stale-generation"
	if ok, err := s.AdvanceGovernedCommand(ctx, stale, domain.GovernedCommandClaimed, stale.ControllerGeneration, claim.ExpectedRevision, claim.CapabilityFingerprint); err != nil || ok {
		t.Fatalf("stale generation: ok=%v err=%v", ok, err)
	}
	stale = claim
	stale.State = domain.GovernedCommandDispatching
	stale.UpdatedAt = at.Add(time.Second)
	stale.ExpectedRevision = "stale-revision"
	if ok, err := s.AdvanceGovernedCommand(ctx, stale, domain.GovernedCommandClaimed, claim.ControllerGeneration, stale.ExpectedRevision, claim.CapabilityFingerprint); err != nil || ok {
		t.Fatalf("stale revision: ok=%v err=%v", ok, err)
	}
	stale = claim
	stale.State = domain.GovernedCommandDispatching
	stale.UpdatedAt = at.Add(time.Second)
	stale.CapabilityFingerprint = "stale-capabilities"
	if ok, err := s.AdvanceGovernedCommand(ctx, stale, domain.GovernedCommandClaimed, claim.ControllerGeneration, claim.ExpectedRevision, stale.CapabilityFingerprint); err != nil || ok {
		t.Fatalf("stale capabilities: ok=%v err=%v", ok, err)
	}
	if !advanceGovernedCommand(t, s, &claim, domain.GovernedCommandDispatching, at.Add(time.Second), nil) {
		t.Fatal("claim did not advance to dispatching")
	}
	if !advanceGovernedCommand(t, s, &claim, domain.GovernedCommandDeliveryUnknown, at.Add(2*time.Second), func(rec *domain.GovernedCommandRecord) {
		rec.Correlation.ProviderEventID = "provider-event-before-disconnect"
	}) {
		t.Fatal("dispatching did not advance to delivery_unknown")
	}

	unsettled, err := s.ListUnsettledGovernedCommands(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(unsettled) != 1 || unsettled[0].ID != claim.ID || unsettled[0].State != domain.GovernedCommandDeliveryUnknown {
		t.Fatalf("unsettled=%+v", unsettled)
	}
	redispatch := claim
	redispatch.State = domain.GovernedCommandDispatching
	redispatch.UpdatedAt = at.Add(3 * time.Second)
	if ok, err := s.AdvanceGovernedCommand(ctx, redispatch, domain.GovernedCommandDeliveryUnknown, claim.ControllerGeneration, claim.ExpectedRevision, claim.CapabilityFingerprint); !errors.Is(err, domain.ErrGovernedCommandTransition) || ok {
		t.Fatalf("unknown redispatch: ok=%v err=%v", ok, err)
	}
	if !advanceGovernedCommand(t, s, &claim, domain.GovernedCommandReconciled, at.Add(3*time.Second), func(rec *domain.GovernedCommandRecord) {
		rec.ReconciliationOutcome = domain.GovernedCommandReconciledAcknowledged
		rec.Correlation.ProviderTurnID = "provider-turn-recovered"
	}) {
		t.Fatal("unknown did not reconcile")
	}
	unsettled, err = s.ListUnsettledGovernedCommands(ctx)
	if err != nil || len(unsettled) != 0 {
		t.Fatalf("settled list=%+v err=%v", unsettled, err)
	}
}

func TestGovernedCommandTransitionRejectsOldStateAndClaimTime(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "governed-transition-race")
	rec := sampleRecord("governed-transition-race")
	rec.Mode = domain.SessionModeChat
	session, err := s.CreateSession(ctx, rec)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Second)
	claim := governedCommandClaim(session.ID, "command-race-transition", "race-transition-key", "race-transition-fingerprint", at)
	if err := s.ClaimChatControllerGeneration(ctx, session.ID, claim.ControllerGeneration, "", "", at); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateGovernedCommandClaim(ctx, claim); err != nil {
		t.Fatal(err)
	}
	next := claim
	next.State = domain.GovernedCommandDispatching
	next.UpdatedAt = at
	if ok, err := s.AdvanceGovernedCommand(ctx, next, domain.GovernedCommandClaimed, claim.ControllerGeneration, claim.ExpectedRevision, claim.CapabilityFingerprint); err != nil || ok {
		t.Fatalf("same-time transition: ok=%v err=%v", ok, err)
	}
	next.UpdatedAt = at.Add(time.Second)
	if ok, err := s.AdvanceGovernedCommand(ctx, next, domain.GovernedCommandClaimed, claim.ControllerGeneration, claim.ExpectedRevision, claim.CapabilityFingerprint); err != nil || !ok {
		t.Fatalf("valid transition: ok=%v err=%v", ok, err)
	}
	if ok, err := s.AdvanceGovernedCommand(ctx, next, domain.GovernedCommandClaimed, claim.ControllerGeneration, claim.ExpectedRevision, claim.CapabilityFingerprint); err != nil || ok {
		t.Fatalf("stale state transition: ok=%v err=%v", ok, err)
	}
}
