package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/harnessconnection"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/harnesspairing"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
)

func seedIntent(t *testing.T) (*harnesspairing.Coordinator, *harnessconnection.Kernel, interface {
	CreateHarnessPairingIntent(context.Context, domain.HarnessPairingIntent) (domain.HarnessPairingIntent, bool, error)
}, domain.HarnessPairingIntent) {
	t.Helper()
	s := sqlitetest.MustOpen(t)
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	if e := s.UpsertProject(context.Background(), domain.ProjectRecord{ID: "project-1", Path: "/tmp/p", RegisteredAt: now}); e != nil {
		t.Fatal(e)
	}
	k := harnessconnection.New(s)
	c := harnesspairing.New(s, k)
	v := domain.HarnessPairingIntent{ID: "intent-1", ProjectID: "project-1", Kind: domain.HarnessPairingKindPair, ConnectionID: "connection-1", InstallationID: "install", AdapterDigest: domain.DigestSHA256([]byte("adapter")), HarnessIdentity: "codex", ProviderVersion: "1", ProtocolFingerprint: domain.DigestSHA256([]byte("protocol")), MissionID: "mission", AppRunID: "run", CapabilityClasses: []domain.HarnessCapabilityClass{domain.HarnessCapabilityTurn}, ExpectedGeneration: 1, ConnectionExpiresAt: now.Add(24 * time.Hour), ExpiresAt: now.Add(time.Hour), Status: domain.HarnessPairingIntentRequested, CreatedAt: now, UpdatedAt: now}
	v.Digest, _ = v.ComputedDigest()
	if _, _, e := s.CreateHarnessPairingIntent(context.Background(), v); e != nil {
		t.Fatal(e)
	}
	return c, k, s, v
}
func receipt(v domain.HarnessPairingIntent, action, key string, now time.Time) domain.HarnessAuthorityReceipt {
	r := domain.HarnessAuthorityReceipt{ID: "receipt-" + key, Action: action, TargetType: "pairing_intent", TargetID: string(v.ID), TargetDigest: v.Digest, RequestKey: key, RequestFingerprint: domain.DigestSHA256([]byte(action + key)), OwnerPrincipal: "local-owner:run", ConfirmationRef: "native:1", CreatedAt: now}
	return r
}
func TestHarnessPairingDecisionIsIdempotentGenerationFenced(t *testing.T) {
	_, _, raw, v := seedIntent(t)
	s := raw.(interface {
		DecideHarnessPairingIntent(context.Context, domain.PairingChallengeID, domain.SHA256Digest, string, domain.HarnessAuthorityReceipt, time.Time) (domain.HarnessPairingIntent, bool, error)
	})
	now := v.CreatedAt.Add(time.Minute)
	r := receipt(v, "approve", "key", now)
	got, changed, e := s.DecideHarnessPairingIntent(context.Background(), v.ID, v.Digest, "approve", r, now)
	if e != nil || !changed || got.Status != domain.HarnessPairingIntentApproved {
		t.Fatalf("approve=%+v,%v,%v", got, changed, e)
	}
	_, changed, e = s.DecideHarnessPairingIntent(context.Background(), v.ID, v.Digest, "approve", r, now)
	if e != nil || changed {
		t.Fatalf("replay changed=%v err=%v", changed, e)
	}
	r2 := receipt(v, "deny", "key", now)
	if _, _, e = s.DecideHarnessPairingIntent(context.Background(), v.ID, v.Digest, "deny", r2, now); !errors.Is(e, domain.ErrHarnessAuthorityConflict) {
		t.Fatalf("conflict=%v", e)
	}
}
func TestHarnessPairingActivationIsOneWinnerAndSecretNotDurable(t *testing.T) {
	c, _, raw, v := seedIntent(t)
	s := raw.(interface {
		DecideHarnessPairingIntent(context.Context, domain.PairingChallengeID, domain.SHA256Digest, string, domain.HarnessAuthorityReceipt, time.Time) (domain.HarnessPairingIntent, bool, error)
		ActivateHarnessPairingIntent(context.Context, domain.PairingChallengeID, domain.SHA256Digest, time.Time, func(domain.HarnessPairingIntent) (domain.HarnessPairingChallenge, domain.PairingChallengeSecret, error)) (domain.HarnessPairingIntent, domain.HarnessPairingChallenge, domain.PairingChallengeSecret, error)
	})
	now := v.CreatedAt.Add(time.Minute)
	if _, _, e := s.DecideHarnessPairingIntent(context.Background(), v.ID, v.Digest, "approve", receipt(v, "approve", "key", now), now); e != nil {
		t.Fatal(e)
	}
	issue := func(i domain.HarnessPairingIntent) (domain.HarnessPairingChallenge, domain.PairingChallengeSecret, error) {
		return c.Prepare(harnesspairing.IssueChallengeRequest{Kind: i.Kind, ConnectionID: i.ConnectionID, InstallationID: i.InstallationID, AdapterDigest: i.AdapterDigest, HarnessIdentity: i.HarnessIdentity, ProviderVersion: i.ProviderVersion, ProtocolFingerprint: i.ProtocolFingerprint, MissionID: i.MissionID, AppRunID: i.AppRunID, CapabilityClasses: i.CapabilityClasses, ExpectedGeneration: i.ExpectedGeneration, ConnectionExpiresAt: i.ConnectionExpiresAt, TTL: i.ExpiresAt.Sub(now), Now: now})
	}
	got, ch, secret, e := s.ActivateHarnessPairingIntent(context.Background(), v.ID, v.Digest, now, issue)
	if e != nil || got.Status != domain.HarnessPairingIntentActive || ch.ID == "" || !secret.Valid() {
		t.Fatalf("activate=%+v %+v %v", got, ch, e)
	}
	if _, _, _, e = s.ActivateHarnessPairingIntent(context.Background(), v.ID, v.Digest, now, issue); !errors.Is(e, domain.ErrHarnessAuthorityStale) {
		t.Fatalf("second activation=%v", e)
	}
}

func TestHarnessPairingActivationConcurrentWinner(t *testing.T) {
	c, _, raw, v := seedIntent(t)
	s := raw.(interface {
		DecideHarnessPairingIntent(context.Context, domain.PairingChallengeID, domain.SHA256Digest, string, domain.HarnessAuthorityReceipt, time.Time) (domain.HarnessPairingIntent, bool, error)
		ActivateHarnessPairingIntent(context.Context, domain.PairingChallengeID, domain.SHA256Digest, time.Time, func(domain.HarnessPairingIntent) (domain.HarnessPairingChallenge, domain.PairingChallengeSecret, error)) (domain.HarnessPairingIntent, domain.HarnessPairingChallenge, domain.PairingChallengeSecret, error)
	})
	now := v.CreatedAt.Add(time.Minute)
	if _, _, err := s.DecideHarnessPairingIntent(context.Background(), v.ID, v.Digest, "approve", receipt(v, "approve", "concurrent", now), now); err != nil {
		t.Fatal(err)
	}
	issue := func(i domain.HarnessPairingIntent) (domain.HarnessPairingChallenge, domain.PairingChallengeSecret, error) {
		return c.Prepare(harnesspairing.IssueChallengeRequest{Kind: i.Kind, ConnectionID: i.ConnectionID, InstallationID: i.InstallationID, AdapterDigest: i.AdapterDigest, HarnessIdentity: i.HarnessIdentity, ProviderVersion: i.ProviderVersion, ProtocolFingerprint: i.ProtocolFingerprint, MissionID: i.MissionID, AppRunID: i.AppRunID, CapabilityClasses: i.CapabilityClasses, ExpectedGeneration: i.ExpectedGeneration, ConnectionExpiresAt: i.ConnectionExpiresAt, TTL: i.ExpiresAt.Sub(now), Now: now})
	}
	const contenders = 12
	start := make(chan struct{})
	results := make(chan error, contenders)
	for range contenders {
		go func() {
			<-start
			_, _, _, err := s.ActivateHarnessPairingIntent(context.Background(), v.ID, v.Digest, now, issue)
			results <- err
		}()
	}
	close(start)
	winners, stale := 0, 0
	for range contenders {
		err := <-results
		switch {
		case err == nil:
			winners++
		case errors.Is(err, domain.ErrHarnessAuthorityStale):
			stale++
		default:
			t.Fatalf("unexpected activation error: %v", err)
		}
	}
	if winners != 1 || stale != contenders-1 {
		t.Fatalf("winners=%d stale=%d", winners, stale)
	}
}

func TestHarnessPairingActivationRollsBackBeforeSecretRelease(t *testing.T) {
	c, _, raw, v := seedIntent(t)
	s := raw.(interface {
		DecideHarnessPairingIntent(context.Context, domain.PairingChallengeID, domain.SHA256Digest, string, domain.HarnessAuthorityReceipt, time.Time) (domain.HarnessPairingIntent, bool, error)
		ActivateHarnessPairingIntent(context.Context, domain.PairingChallengeID, domain.SHA256Digest, time.Time, func(domain.HarnessPairingIntent) (domain.HarnessPairingChallenge, domain.PairingChallengeSecret, error)) (domain.HarnessPairingIntent, domain.HarnessPairingChallenge, domain.PairingChallengeSecret, error)
	})
	now := v.CreatedAt.Add(time.Minute)
	if _, _, err := s.DecideHarnessPairingIntent(context.Background(), v.ID, v.Digest, "approve", receipt(v, "approve", "rollback", now), now); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected issue failure")
	_, _, leaked, err := s.ActivateHarnessPairingIntent(context.Background(), v.ID, v.Digest, now, func(domain.HarnessPairingIntent) (domain.HarnessPairingChallenge, domain.PairingChallengeSecret, error) {
		return domain.HarnessPairingChallenge{}, domain.PairingChallengeSecret(""), injected
	})
	if !errors.Is(err, injected) || leaked.Valid() {
		t.Fatalf("failure err=%v leaked_secret=%v", err, leaked.Valid())
	}
	issue := func(i domain.HarnessPairingIntent) (domain.HarnessPairingChallenge, domain.PairingChallengeSecret, error) {
		return c.Prepare(harnesspairing.IssueChallengeRequest{Kind: i.Kind, ConnectionID: i.ConnectionID, InstallationID: i.InstallationID, AdapterDigest: i.AdapterDigest, HarnessIdentity: i.HarnessIdentity, ProviderVersion: i.ProviderVersion, ProtocolFingerprint: i.ProtocolFingerprint, MissionID: i.MissionID, AppRunID: i.AppRunID, CapabilityClasses: i.CapabilityClasses, ExpectedGeneration: i.ExpectedGeneration, ConnectionExpiresAt: i.ConnectionExpiresAt, TTL: i.ExpiresAt.Sub(now), Now: now})
	}
	got, _, secret, err := s.ActivateHarnessPairingIntent(context.Background(), v.ID, v.Digest, now, issue)
	if err != nil || got.Status != domain.HarnessPairingIntentActive || !secret.Valid() {
		t.Fatalf("retry after rollback: status=%s secret=%v err=%v", got.Status, secret.Valid(), err)
	}
}

func TestHarnessAuthorityRevokeAtomicallyMarksCommandConsequences(t *testing.T) {
	f := newCommandFixture(t)
	claim, made, err := f.store.ValidateAuthoritiesAndCreateCommandClaim(context.Background(), f.request)
	if err != nil || !made {
		t.Fatalf("claim made=%v err=%v", made, err)
	}
	connection, found, err := f.store.GetHarnessConnection(context.Background(), f.request.ConnectionBinding.ConnectionID)
	if err != nil || !found {
		t.Fatalf("connection found=%v err=%v", found, err)
	}
	digest, err := connection.AuthorityDigest()
	if err != nil {
		t.Fatal(err)
	}
	r := domain.HarnessAuthorityReceipt{ID: "receipt-revoke", Action: "revoke", TargetType: "harness_connection", TargetID: string(connection.ID), TargetDigest: digest, ExpectedGeneration: connection.Generation, RequestKey: "revoke-key", RequestFingerprint: domain.DigestSHA256([]byte("revoke-request")), OwnerPrincipal: "local-owner:run", ConfirmationRef: "native:revoke", CreatedAt: f.request.Now.Add(time.Second)}
	got, changed, consequences, err := f.store.RevokeHarnessConnectionWithReceipt(context.Background(), connection.ID, digest, connection.Generation, r, f.request.Now.Add(time.Second))
	if err != nil || !changed || got.RevokedAt == nil || consequences != 1 {
		t.Fatalf("revoked=%+v changed=%v consequences=%d err=%v", got, changed, consequences, err)
	}
	pending, err := f.store.ListPendingHarnessCommandOutbox(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	foundConsequence := false
	for _, entry := range pending {
		if entry.ClaimID == claim.ID {
			foundConsequence = true
			if entry.State != domain.CommandAuthorityClaimActionNeeded {
				t.Fatalf("revoked connection left command %s pending: %s", claim.ID, entry.State)
			}
		}
	}
	if !foundConsequence {
		t.Fatalf("revocation consequence for command %s was not retained", claim.ID)
	}
	got, changed, consequences, err = f.store.RevokeHarnessConnectionWithReceipt(context.Background(), connection.ID, digest, connection.Generation, r, f.request.Now.Add(time.Second))
	if err != nil || changed || got.RevokedAt == nil || consequences != 1 {
		t.Fatalf("replay revoked=%+v changed=%v consequences=%d err=%v", got, changed, consequences, err)
	}
}
