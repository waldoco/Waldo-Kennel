package harnessauthority

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/harnessconnection"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/harnesspairing"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
)

func intentRequest(project domain.ProjectID, now time.Time) CreateIntentRequest {
	return CreateIntentRequest{ProjectID: project, Kind: domain.HarnessPairingKindPair, ConnectionID: "hc-1", InstallationID: "install", AdapterDigest: domain.DigestSHA256([]byte("adapter")), HarnessIdentity: "codex", ProviderVersion: "1.0.0", ProtocolFingerprint: domain.DigestSHA256([]byte("protocol")), MissionID: "renderer-controlled", AppRunID: "run", CapabilityClasses: []domain.HarnessCapabilityClass{domain.HarnessCapabilityTurn}, ExpectedGeneration: 1, ConnectionExpiresAt: now.Add(time.Hour), ExpiresAt: now.Add(time.Minute), RequestKey: "key", RequestFingerprint: string(domain.DigestSHA256([]byte("request")))}
}

func TestCreateIntentBindsProjectWorkSpaceBeforeOutcome(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	now := time.Now().UTC()
	if err := store.UpsertProject(context.Background(), domain.ProjectRecord{ID: "project-1", Path: t.TempDir(), RegisteredAt: now}); err != nil {
		t.Fatal(err)
	}
	svc := New(store, harnesspairing.New(store, harnessconnection.New(store)), harnessconnection.New(store)).WithProjectScope(store)
	intent, created, err := svc.CreateIntent(context.Background(), intentRequest("project-1", now))
	if err != nil || !created {
		t.Fatalf("create=(%v,%v)", created, err)
	}
	space, err := store.EnsureWorkResponsibilitySpace(context.Background(), "project-1")
	if err != nil || intent.MissionID != string(space.ID) || intent.MissionID == "renderer-controlled" {
		t.Fatalf("mission=%q space=%q err=%v", intent.MissionID, space.ID, err)
	}
}

func TestCreateIntentConcurrentProjectScopeConverges(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	now := time.Now().UTC()
	if err := store.UpsertProject(context.Background(), domain.ProjectRecord{ID: "project-1", Path: t.TempDir(), RegisteredAt: now}); err != nil {
		t.Fatal(err)
	}
	svc := New(store, harnesspairing.New(store, harnessconnection.New(store)), harnessconnection.New(store)).WithProjectScope(store)
	missions := make(chan string, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := intentRequest("project-1", now)
			r.ID = domain.PairingChallengeID("intent-" + string(rune('a'+i)))
			r.ConnectionID = domain.HarnessConnectionID("hc-" + string(rune('a'+i)))
			r.RequestKey = "key-" + string(rune('a'+i))
			r.RequestFingerprint = string(domain.DigestSHA256([]byte(r.RequestKey)))
			v, _, e := svc.CreateIntent(context.Background(), r)
			errs <- e
			missions <- v.MissionID
		}(i)
	}
	wg.Wait()
	close(errs)
	close(missions)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first string
	for mission := range missions {
		if first == "" {
			first = mission
		} else if mission != first {
			t.Fatalf("missions %q != %q", mission, first)
		}
	}
}

func TestCreateIntentRejectsAbsentProjectAndMissingResolver(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	now := time.Now().UTC()
	base := New(store, harnesspairing.New(store, harnessconnection.New(store)), harnessconnection.New(store))
	if _, _, err := base.CreateIntent(context.Background(), intentRequest("missing", now)); err == nil {
		t.Fatal("missing resolver accepted")
	}
	if _, _, err := base.WithProjectScope(store).CreateIntent(context.Background(), intentRequest("missing", now)); err == nil {
		t.Fatal("missing project accepted")
	}
}
