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
	intent     domain.HarnessPairingIntent
	receipts   []domain.HarnessAuthorityReceipt
	connection domain.HarnessConnection
	challenge  domain.HarnessPairingChallenge
}

func (s harnessAuthorityStub) ListHarnessPairingIntents(context.Context, domain.ProjectID, int) ([]domain.HarnessPairingIntent, error) {
	return []domain.HarnessPairingIntent{s.intent}, nil
}
func (s harnessAuthorityStub) GetHarnessPairingIntent(context.Context, domain.PairingChallengeID) (domain.HarnessPairingIntent, bool, error) {
	return s.intent, true, nil
}
func (s harnessAuthorityStub) ListHarnessConnections(context.Context, string, int) ([]domain.HarnessConnection, error) {
	if s.connection.ID == "" {
		return nil, nil
	}
	return []domain.HarnessConnection{s.connection}, nil
}
func (s harnessAuthorityStub) GetHarnessConnection(context.Context, domain.HarnessConnectionID) (domain.HarnessConnection, bool, error) {
	if s.connection.ID == "" {
		return domain.HarnessConnection{}, false, nil
	}
	return s.connection, true, nil
}
func (s harnessAuthorityStub) ListHarnessAuthorityReceipts(context.Context, string, string) ([]domain.HarnessAuthorityReceipt, error) {
	return s.receipts, nil
}
func (s harnessAuthorityStub) CountHarnessCommandConsequences(context.Context, domain.HarnessConnectionID, int64) (int64, error) {
	return 0, nil
}
func (s harnessAuthorityStub) GetHarnessPairingChallenge(context.Context, domain.PairingChallengeID) (domain.HarnessPairingChallenge, bool, error) {
	if s.challenge.ID == "" {
		return domain.HarnessPairingChallenge{}, false, nil
	}
	return s.challenge, true, nil
}

func TestHarnessAuthorityDetailRedactsCustodyAndRequestEvidence(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	intent := domain.HarnessPairingIntent{ID: "intent-1", ProposalRequestKey: "proposal-key", ProposalRequestFingerprint: domain.DigestSHA256([]byte("proposal-request")), ProjectID: "project-1", Kind: domain.HarnessPairingKindPair, ConnectionID: "connection-1", InstallationID: "install", AdapterDigest: domain.DigestSHA256([]byte("adapter")), HarnessIdentity: "codex", ProviderVersion: "1", ProtocolFingerprint: domain.DigestSHA256([]byte("protocol")), MissionID: "mission", AppRunID: "private-app-run-canary", CapabilityClasses: []domain.HarnessCapabilityClass{domain.HarnessCapabilityTurn}, ExpectedGeneration: 1, ConnectionExpiresAt: now.Add(time.Hour), ExpiresAt: now.Add(time.Minute), Digest: domain.DigestSHA256([]byte("intent")), Status: domain.HarnessPairingIntentApproved, CreatedAt: now, UpdatedAt: now}
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

func TestHarnessAuthorityAllRoutesForbidPrivateFields(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	intent := domain.HarnessPairingIntent{ID: "intent-1", ProjectID: "project-1", Kind: domain.HarnessPairingKindPair, ConnectionID: "connection-1", InstallationID: "install", AdapterDigest: domain.DigestSHA256([]byte("adapter")), HarnessIdentity: "codex", ProviderVersion: "1", ProtocolFingerprint: domain.DigestSHA256([]byte("protocol")), MissionID: "mission", AppRunID: "PRIVATE_APP_RUN", CapabilityClasses: []domain.HarnessCapabilityClass{domain.HarnessCapabilityTurn}, ExpectedGeneration: 1, ConnectionExpiresAt: now.Add(time.Hour), ExpiresAt: now.Add(time.Minute), Digest: domain.DigestSHA256([]byte("intent")), Status: domain.HarnessPairingIntentRequested, ProposalRequestKey: "PRIVATE_REQUEST_KEY", ProposalRequestFingerprint: domain.DigestSHA256([]byte("PRIVATE_REQUEST_FINGERPRINT")), CreatedAt: now, UpdatedAt: now}
	conn := domain.HarnessConnection{ID: "connection-1", InstallationID: "install", HarnessIdentity: "codex", ProviderVersion: "1", AdapterDigest: intent.AdapterDigest, ProtocolFingerprint: intent.ProtocolFingerprint, MissionID: "mission", AppRunID: "PRIVATE_CONNECTION_APP_RUN", CapabilityClasses: intent.CapabilityClasses, Generation: 1, ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now}
	r := domain.HarnessAuthorityReceipt{ID: "receipt-1", Action: "approve", TargetType: "pairing_intent", TargetID: "intent-1", TargetDigest: intent.Digest, RequestKey: "PRIVATE_RECEIPT_KEY", RequestFingerprint: domain.DigestSHA256([]byte("PRIVATE_RECEIPT_FP")), OwnerPrincipal: "PRIVATE_OWNER", ConfirmationRef: "PRIVATE_CONFIRMATION", CreatedAt: now}
	c := &HarnessAuthorityController{Svc: harnessAuthorityStub{intent: intent, connection: conn, receipts: []domain.HarnessAuthorityReceipt{r}}, Now: func() time.Time { return now }}
	router := chi.NewRouter()
	c.Register(router)
	for _, path := range []string{"/harness-pairing-intents", "/harness-pairing-intents/intent-1", "/harness-connections", "/harness-connections/connection-1"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != 200 {
			t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
		for _, canary := range []string{"PRIVATE_APP_RUN", "PRIVATE_REQUEST_KEY", "PRIVATE_REQUEST_FINGERPRINT", "PRIVATE_CONNECTION_APP_RUN", "PRIVATE_RECEIPT_KEY", "PRIVATE_RECEIPT_FP", "PRIVATE_OWNER", "PRIVATE_CONFIRMATION", "proofVerifier", "pairingSecret"} {
			if strings.Contains(rec.Body.String(), canary) {
				t.Fatalf("%s leaked %q: %s", path, canary, rec.Body.String())
			}
		}
	}
}

func TestHarnessAuthorityProofStateMapping(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	cid := domain.PairingChallengeID("challenge-1")
	base := domain.HarnessPairingIntent{ID: "intent-1", Kind: domain.HarnessPairingKindPair, ConnectionID: "connection-1", InstallationID: "i", AdapterDigest: domain.DigestSHA256([]byte("a")), HarnessIdentity: "h", ProviderVersion: "1", ProtocolFingerprint: domain.DigestSHA256([]byte("p")), MissionID: "m", CapabilityClasses: []domain.HarnessCapabilityClass{domain.HarnessCapabilityTurn}, ExpectedGeneration: 2, ConnectionExpiresAt: now.Add(time.Hour), ExpiresAt: now.Add(time.Minute), Digest: domain.DigestSHA256([]byte("i")), Status: domain.HarnessPairingIntentActive, ChallengeID: &cid, CreatedAt: now, UpdatedAt: now}
	conn := domain.HarnessConnection{ID: "connection-1", Generation: 2}
	for _, tc := range []struct {
		name       string
		code       *domain.HarnessPairingResultCode
		status     domain.HarnessPairingStatus
		generation int64
		want       string
	}{{"pending", nil, domain.HarnessPairingPending, 2, "pending"}, {"success", resultCode(domain.HarnessPairingResultSucceeded), domain.HarnessPairingConsumed, 2, "succeeded"}, {"failed", resultCode(domain.HarnessPairingResultTupleMismatch), domain.HarnessPairingConsumed, 2, "failed"}, {"superseded", resultCode(domain.HarnessPairingResultSucceeded), domain.HarnessPairingConsumed, 3, "superseded"}, {"expired", resultCode(domain.HarnessPairingResultExpired), domain.HarnessPairingConsumed, 2, "expired"}} {
		t.Run(tc.name, func(t *testing.T) {
			conn.Generation = tc.generation
			c := &HarnessAuthorityController{Svc: harnessAuthorityStub{intent: base, connection: conn, challenge: domain.HarnessPairingChallenge{ID: cid, Status: tc.status, ResultCode: tc.code}}, Now: func() time.Time { return now }}
			if got := c.intentView(context.Background(), base).ProofState; got != tc.want {
				t.Fatalf("got=%s want=%s", got, tc.want)
			}
		})
	}
}
func resultCode(v domain.HarnessPairingResultCode) *domain.HarnessPairingResultCode { return &v }

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
