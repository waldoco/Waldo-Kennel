package harnessconnection

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
)

func issueFixture(t *testing.T, k *Kernel) (IssuedConnection, Binding, time.Time) {
	t.Helper()
	now := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	req := IssueRequest{ConnectionID: "hc-1", InstallationID: "installation", AdapterDigest: domain.DigestSHA256([]byte("adapter")), HarnessIdentity: "codex", ProviderVersion: "0.154.0", ProtocolFingerprint: domain.DigestSHA256([]byte("protocol")), MissionID: "mission", AppRunID: "app-run", CapabilityClasses: []domain.HarnessCapabilityClass{domain.HarnessCapabilityTurn}, ExpiresAt: now.Add(time.Hour), Now: now}
	issued, err := k.Issue(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	binding := Binding{ConnectionID: req.ConnectionID, InstallationID: req.InstallationID, AdapterDigest: req.AdapterDigest, HarnessIdentity: req.HarnessIdentity, ProviderVersion: req.ProviderVersion, ProtocolFingerprint: req.ProtocolFingerprint, MissionID: req.MissionID, AppRunID: req.AppRunID, Generation: 1, Class: domain.HarnessCapabilityTurn}
	return issued, binding, now
}
func TestIssueAuthenticateRestartAndNoBearerPersistence(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	issued, binding, now := issueFixture(t, NewWithRandom(store, bytes.NewReader(make([]byte, 64))))
	if issued.Bearer == "" || issued.Connection.CapabilityVerifier != "" {
		t.Fatal("bad public issue shape")
	}
	stored, found, err := store.GetHarnessConnection(context.Background(), binding.ConnectionID)
	if err != nil || !found {
		t.Fatal(err)
	}
	if stored.CapabilityVerifier == "" || stored.CapabilityVerifier == issued.Bearer {
		t.Fatal("bearer persisted or verifier missing")
	}
	restarted := New(store)
	if _, err := restarted.Authenticate(context.Background(), issued.Bearer, binding, now); err != nil {
		t.Fatal(err)
	}
}
func TestAuthenticateRejectsSpoofStaleUndeclaredExpiredRevokedAndRotation(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	k := NewWithRandom(store, bytes.NewReader(append(bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 224)...)))
	issued, binding, now := issueFixture(t, k)
	tests := []struct {
		name   string
		mutate func(*Binding)
		at     time.Time
	}{{"mission", func(b *Binding) { b.MissionID = "other" }, now}, {"app", func(b *Binding) { b.AppRunID = "other" }, now}, {"digest", func(b *Binding) { b.AdapterDigest = domain.DigestSHA256([]byte("other")) }, now}, {"harness", func(b *Binding) { b.HarnessIdentity = "other" }, now}, {"provider version", func(b *Binding) { b.ProviderVersion = "other" }, now}, {"protocol", func(b *Binding) { b.ProtocolFingerprint = domain.DigestSHA256([]byte("other")) }, now}, {"generation", func(b *Binding) { b.Generation = 2 }, now}, {"class", func(b *Binding) { b.Class = domain.HarnessCapabilityAccept }, now}, {"expired", func(*Binding) {}, now.Add(time.Hour)}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := binding
			tt.mutate(&b)
			if _, err := k.Authenticate(context.Background(), issued.Bearer, b, tt.at); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	rotated, err := k.Rotate(context.Background(), binding.ConnectionID, 1, "app-run-next", now.Add(2*time.Hour), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := k.Authenticate(context.Background(), issued.Bearer, binding, now.Add(time.Minute)); err == nil {
		t.Fatal("old bearer accepted")
	}
	binding.Generation = 2
	binding.AppRunID = "app-run-next"
	if _, err := k.Authenticate(context.Background(), rotated.Bearer, binding, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Revoke(context.Background(), binding.ConnectionID, 2, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Authenticate(context.Background(), rotated.Bearer, binding, now.Add(2*time.Minute)); err == nil {
		t.Fatal("revoked bearer accepted")
	}
}
func TestConcurrentRotationHasOneWinner(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	k := NewWithRandom(store, bytes.NewReader(append(bytes.Repeat([]byte{2}, 32), bytes.Repeat([]byte{3}, 480)...)))
	_, binding, now := issueFixture(t, k)
	var wg sync.WaitGroup
	successes := 0
	var mu sync.Mutex
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := k.Rotate(context.Background(), binding.ConnectionID, 1, binding.AppRunID, now.Add(2*time.Hour), now.Add(time.Minute)); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if successes != 1 {
		t.Fatalf("winners=%d", successes)
	}
}

func TestExactIssueReplayConvergesWithoutReturningBearer(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	k := NewWithRandom(store, bytes.NewReader(bytes.Repeat([]byte{3}, 256)))
	first, _, _ := issueFixture(t, k)
	now := first.Connection.CreatedAt
	replay, err := k.Issue(context.Background(), IssueRequest{ConnectionID: first.Connection.ID, InstallationID: first.Connection.InstallationID, AdapterDigest: first.Connection.AdapterDigest, HarnessIdentity: first.Connection.HarnessIdentity, ProviderVersion: first.Connection.ProviderVersion, ProtocolFingerprint: first.Connection.ProtocolFingerprint, MissionID: first.Connection.MissionID, AppRunID: first.Connection.AppRunID, CapabilityClasses: first.Connection.CapabilityClasses, ExpiresAt: first.Connection.ExpiresAt, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if replay.Bearer != "" {
		t.Fatal("replay returned a new bearer")
	}
	if replay.Connection.ID != first.Connection.ID {
		t.Fatal("replay did not converge")
	}
}

func TestConcurrentIssuanceOneBearerWinsAndChangedSemanticsConflict(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	now := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	req := IssueRequest{ConnectionID: "hc-race", InstallationID: "installation", AdapterDigest: domain.DigestSHA256([]byte("adapter")), HarnessIdentity: "codex", ProviderVersion: "0.154.0", ProtocolFingerprint: domain.DigestSHA256([]byte("protocol")), MissionID: "mission", AppRunID: "app-run", CapabilityClasses: []domain.HarnessCapabilityClass{domain.HarnessCapabilityTurn}, ExpiresAt: now.Add(time.Hour), Now: now}
	var wg sync.WaitGroup
	var mu sync.Mutex
	bearers, successful := 0, 0
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(seed byte) {
			defer wg.Done()
			issued, err := NewWithRandom(store, bytes.NewReader(bytes.Repeat([]byte{seed}, 64))).Issue(context.Background(), req)
			if err == nil {
				mu.Lock()
				successful++
				if issued.Bearer != "" {
					bearers++
				}
				mu.Unlock()
			}
		}(byte(i + 5))
	}
	wg.Wait()
	if successful != 2 || bearers != 1 {
		t.Fatalf("successful=%d bearer_returns=%d", successful, bearers)
	}
	changed := req
	changed.MissionID = "different"
	if _, err := NewWithRandom(store, bytes.NewReader(bytes.Repeat([]byte{9}, 64))).Issue(context.Background(), changed); err == nil {
		t.Fatal("changed replay accepted")
	}
}
