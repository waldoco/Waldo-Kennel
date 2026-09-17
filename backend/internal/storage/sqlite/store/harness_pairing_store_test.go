package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

func sampleHarnessPairingChallenge(id domain.PairingChallengeID, connectionID domain.HarnessConnectionID, now time.Time) domain.HarnessPairingChallenge {
	return domain.HarnessPairingChallenge{
		ID: id, Kind: domain.HarnessPairingKindPair, ConnectionID: connectionID,
		InstallationID: "installation", AdapterDigest: digest64Value, HarnessIdentity: "codex",
		ProviderVersion: "0.154.0", ProtocolFingerprint: digest64Value, MissionID: "mission", AppRunID: "run",
		CapabilityClasses: []domain.HarnessCapabilityClass{domain.HarnessCapabilityTurn},
		ExpectedGeneration: 1, ProofVerifier: string(digest64Value), Status: domain.HarnessPairingPending,
		ConnectionExpiresAt: now.Add(time.Hour), ExpiresAt: now.Add(time.Minute), CreatedAt: now, UpdatedAt: now,
	}
}

var digest64Value = domain.SHA256Digest("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

func TestHarnessPairingChallengeStore_CreateGetConsume(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	rec := sampleHarnessPairingChallenge("ch1", "hc1", now)

	created, ok, err := s.CreateHarnessPairingChallenge(ctx, rec)
	if err != nil || !ok {
		t.Fatalf("create: created=%v err=%v", ok, err)
	}
	if created.ProofVerifier == "" {
		t.Fatal("expected verifier to round-trip on the in-memory return")
	}

	got, found, err := s.GetHarnessPairingChallenge(ctx, "ch1")
	if err != nil || !found {
		t.Fatalf("get: found=%v err=%v", found, err)
	}
	if got.Status != domain.HarnessPairingPending {
		t.Fatalf("status = %v, want pending", got.Status)
	}

	changed, err := s.ConsumeHarnessPairingChallenge(ctx, "ch1", now.Add(time.Second))
	if err != nil || !changed {
		t.Fatalf("consume: changed=%v err=%v", changed, err)
	}
	// Replaying an already-consumed challenge must not change anything again.
	changed, err = s.ConsumeHarnessPairingChallenge(ctx, "ch1", now.Add(time.Second))
	if err != nil || changed {
		t.Fatalf("replay consume: changed=%v err=%v, want changed=false", changed, err)
	}

	got, found, err = s.GetHarnessPairingChallenge(ctx, "ch1")
	if err != nil || !found {
		t.Fatalf("get after consume: found=%v err=%v", found, err)
	}
	if got.Status != domain.HarnessPairingConsumed {
		t.Fatalf("status after consume = %v, want consumed", got.Status)
	}
	if got.ResultCode != nil {
		t.Fatal("result code must stay unset until explicitly recorded")
	}
}

func TestHarnessPairingChallengeStore_ConsumeExpiredFails(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	rec := sampleHarnessPairingChallenge("ch2", "hc2", now)
	if _, _, err := s.CreateHarnessPairingChallenge(ctx, rec); err != nil {
		t.Fatal(err)
	}
	changed, err := s.ConsumeHarnessPairingChallenge(ctx, "ch2", rec.ExpiresAt.Add(time.Second))
	if err != nil || changed {
		t.Fatalf("consume after expiry: changed=%v err=%v, want changed=false", changed, err)
	}
}

func TestHarnessPairingChallengeStore_SupersedePending(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	rec := sampleHarnessPairingChallenge("ch3", "hc3", now)
	if _, _, err := s.CreateHarnessPairingChallenge(ctx, rec); err != nil {
		t.Fatal(err)
	}
	n, err := s.SupersedePendingHarnessPairingChallenges(ctx, "hc3", now.Add(time.Second))
	if err != nil || n != 1 {
		t.Fatalf("supersede: n=%d err=%v", n, err)
	}
	got, _, err := s.GetHarnessPairingChallenge(ctx, "ch3")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.HarnessPairingSuperseded {
		t.Fatalf("status = %v, want superseded", got.Status)
	}
	// A superseded challenge can never be consumed.
	changed, err := s.ConsumeHarnessPairingChallenge(ctx, "ch3", now.Add(2*time.Second))
	if err != nil || changed {
		t.Fatalf("consume after supersede: changed=%v err=%v, want changed=false", changed, err)
	}
}

func TestHarnessPairingChallengeStore_RecordResultOnlyOnce(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	rec := sampleHarnessPairingChallenge("ch4", "hc4", now)
	if _, _, err := s.CreateHarnessPairingChallenge(ctx, rec); err != nil {
		t.Fatal(err)
	}
	changed, err := s.RecordHarnessPairingResult(ctx, "ch4", domain.HarnessPairingResultSucceeded, now)
	if err != nil || !changed {
		t.Fatalf("first record: changed=%v err=%v", changed, err)
	}
	changed, err = s.RecordHarnessPairingResult(ctx, "ch4", domain.HarnessPairingResultReplayed, now)
	if err != nil || changed {
		t.Fatalf("second record must not overwrite: changed=%v err=%v", changed, err)
	}
	got, _, err := s.GetHarnessPairingChallenge(ctx, "ch4")
	if err != nil {
		t.Fatal(err)
	}
	if got.ResultCode == nil || *got.ResultCode != domain.HarnessPairingResultSucceeded {
		t.Fatalf("result code = %v, want succeeded and immutable", got.ResultCode)
	}
}
