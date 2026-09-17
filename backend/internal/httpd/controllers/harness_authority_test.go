package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/go-chi/chi/v5"
)

type harnessAuthorityStub struct {
	intent   domain.HarnessPairingIntent
	receipts []domain.HarnessAuthorityReceipt
}

func (s harnessAuthorityStub) ListHarnessPairingIntents(context.Context, domain.ProjectID, int) ([]domain.HarnessPairingIntent, error) {
	return []domain.HarnessPairingIntent{s.intent}, nil
}
func (s harnessAuthorityStub) GetHarnessPairingIntent(context.Context, domain.PairingChallengeID) (domain.HarnessPairingIntent, bool, error) {
	return s.intent, true, nil
}
func (s harnessAuthorityStub) ListHarnessConnections(context.Context, string, int) ([]domain.HarnessConnection, error) {
	return nil, nil
}
func (s harnessAuthorityStub) GetHarnessConnection(context.Context, domain.HarnessConnectionID) (domain.HarnessConnection, bool, error) {
	return domain.HarnessConnection{}, false, nil
}
func (s harnessAuthorityStub) ListHarnessAuthorityReceipts(context.Context, string, string) ([]domain.HarnessAuthorityReceipt, error) {
	return s.receipts, nil
}
func (s harnessAuthorityStub) CountHarnessCommandConsequences(context.Context, domain.HarnessConnectionID, int64) (int64, error) {
	return 0, nil
}

func TestHarnessAuthorityDetailRedactsCustodyAndRequestEvidence(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	intent := domain.HarnessPairingIntent{ID: "intent-1", ProjectID: "project-1", Kind: domain.HarnessPairingKindPair, ConnectionID: "connection-1", InstallationID: "install", AdapterDigest: domain.DigestSHA256([]byte("adapter")), HarnessIdentity: "codex", ProviderVersion: "1", ProtocolFingerprint: domain.DigestSHA256([]byte("protocol")), MissionID: "mission", AppRunID: "private-app-run-canary", CapabilityClasses: []domain.HarnessCapabilityClass{domain.HarnessCapabilityTurn}, ExpectedGeneration: 1, ConnectionExpiresAt: now.Add(time.Hour), ExpiresAt: now.Add(time.Minute), Digest: domain.DigestSHA256([]byte("intent")), Status: domain.HarnessPairingIntentApproved, CreatedAt: now, UpdatedAt: now}
	r := domain.HarnessAuthorityReceipt{ID: "receipt-1", Action: "approve", TargetType: "pairing_intent", TargetID: "intent-1", TargetDigest: intent.Digest, RequestKey: "private-request-key-canary", RequestFingerprint: domain.DigestSHA256([]byte("request")), OwnerPrincipal: "private-owner-canary", ConfirmationRef: "private-confirmation-canary", CreatedAt: now}
	c := &HarnessAuthorityController{Svc: harnessAuthorityStub{intent: intent, receipts: []domain.HarnessAuthorityReceipt{r}}, Now: func() time.Time { return now }}
	router := chi.NewRouter()
	c.Register(router)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/harness-pairing-intents/intent-1", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, canary := range []string{"private-app-run-canary", "private-request-key-canary", "private-owner-canary", "private-confirmation-canary"} {
		if strings.Contains(body, canary) {
			t.Fatalf("response leaked %q: %s", canary, body)
		}
	}
	var decoded map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `"confirmed":true`) {
		t.Fatalf("redacted receipt lacks confirmation state: %s", body)
	}
}

func TestHarnessAuthorityReadRoutesAreGETOnlyAndUnwiredReturns501(t *testing.T) {
	c := &HarnessAuthorityController{}
	router := chi.NewRouter()
	c.Register(router)
	for _, tc := range []struct {
		method, path string
		want         int
	}{{http.MethodGet, "/harness-pairing-intents", 501}, {http.MethodPost, "/harness-pairing-intents/intent-1/approve", 404}, {http.MethodDelete, "/harness-connections/connection-1", 405}} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != tc.want {
			t.Errorf("%s %s status=%d want=%d body=%s", tc.method, tc.path, rec.Code, tc.want, rec.Body.String())
		}
	}
}
