package store_test

import (
	"context"
	"errors"
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

func TestGovernedControlTransitionFencesAndRetainsUnknown(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "control-transition")
	session, err := s.CreateSession(ctx, sampleRecord("control-transition"))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Second)
	claim := controlClaim(session.ID, "control-1", "key-1", "fp-1", at)
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
