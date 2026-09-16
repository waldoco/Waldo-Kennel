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
