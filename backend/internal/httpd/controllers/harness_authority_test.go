package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
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
	receiptErr error
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
	return s.receipts, s.receiptErr
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
	r := domain.HarnessAuthorityReceipt{ID: "receipt-1", Action: "approve", TargetType: "pairing_intent", TargetID: "intent-1", TargetDigest: intent.Digest, ExpectedGeneration: intent.ExpectedGeneration, RequestKey: "private-request-key-canary", RequestFingerprint: domain.DigestSHA256([]byte("request")), OwnerPrincipal: "private-owner-canary", ConfirmationRef: "private-confirmation-canary", CreatedAt: now}
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
	r := domain.HarnessAuthorityReceipt{ID: "receipt-1", Action: "approve", TargetType: "pairing_intent", TargetID: "intent-1", TargetDigest: intent.Digest, ExpectedGeneration: intent.ExpectedGeneration, RequestKey: "PRIVATE_RECEIPT_KEY", RequestFingerprint: domain.DigestSHA256([]byte("PRIVATE_RECEIPT_FP")), OwnerPrincipal: "PRIVATE_OWNER", ConfirmationRef: "PRIVATE_CONFIRMATION", CreatedAt: now}
	connectionDigest, err := conn.AuthorityDigest()
	if err != nil {
		t.Fatal(err)
	}
	cr := r
	cr.ID = "receipt-connection"
	cr.TargetType = "harness_connection"
	cr.TargetID = string(conn.ID)
	cr.TargetDigest = connectionDigest
	cr.ExpectedGeneration = conn.Generation
	c := &HarnessAuthorityController{Svc: harnessAuthorityStub{intent: intent, connection: conn, receipts: []domain.HarnessAuthorityReceipt{r, cr}}, Now: func() time.Time { return now }}
	router := chi.NewRouter()
	c.Register(router)
	expected := map[string]map[string][]string{
		"/harness-pairing-intents":          {"intent": {"id", "version", "digest", "kind", "connectionId", "installationId", "harnessIdentity", "providerVersion", "adapterDigest", "protocolFingerprint", "missionId", "capabilities", "expectedGeneration", "connectionExpiresAt", "expiresAt", "status", "proofState", "updatedAt"}},
		"/harness-pairing-intents/intent-1": {"intent": {"id", "version", "digest", "kind", "connectionId", "installationId", "harnessIdentity", "providerVersion", "adapterDigest", "protocolFingerprint", "missionId", "capabilities", "expectedGeneration", "connectionExpiresAt", "expiresAt", "status", "proofState", "updatedAt"}, "receipt": {"id", "action", "targetType", "targetId", "targetDigest", "expectedGeneration", "confirmed", "createdAt"}},
		"/harness-connections":              {"connection": {"id", "version", "digest", "installationId", "harnessIdentity", "providerVersion", "adapterDigest", "protocolFingerprint", "missionId", "capabilities", "generation", "expiresAt", "state", "reason", "repair", "actionNeededCommands", "updatedAt"}},
		"/harness-connections/connection-1": {"connection": {"id", "version", "digest", "installationId", "harnessIdentity", "providerVersion", "adapterDigest", "protocolFingerprint", "missionId", "capabilities", "generation", "expiresAt", "state", "reason", "repair", "actionNeededCommands", "updatedAt"}, "receipt": {"id", "action", "targetType", "targetId", "targetDigest", "expectedGeneration", "confirmed", "createdAt"}},
	}
	for _, path := range []string{"/harness-pairing-intents", "/harness-pairing-intents/intent-1", "/harness-connections", "/harness-connections/connection-1"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != 200 {
			t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
		assertRouteKeys(t, path, rec.Body.Bytes(), expected[path])
		for _, canary := range []string{"PRIVATE_APP_RUN", "PRIVATE_REQUEST_KEY", "PRIVATE_REQUEST_FINGERPRINT", "PRIVATE_CONNECTION_APP_RUN", "PRIVATE_RECEIPT_KEY", "PRIVATE_RECEIPT_FP", "PRIVATE_OWNER", "PRIVATE_CONFIRMATION", "proofVerifier", "pairingSecret"} {
			if strings.Contains(rec.Body.String(), canary) {
				t.Fatalf("%s leaked %q: %s", path, canary, rec.Body.String())
			}
		}
	}
}

func assertRouteKeys(t *testing.T, path string, body []byte, expected map[string][]string) {
	t.Helper()
	var root struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &root); err != nil {
		t.Fatal(err)
	}
	for name, want := range expected {
		var raw json.RawMessage
		switch name {
		case "intent":
			if strings.HasSuffix(path, "intent-1") {
				raw = root.Data["intent"]
			} else {
				var xs []json.RawMessage
				if err := json.Unmarshal(root.Data["intents"], &xs); err != nil || len(xs) != 1 {
					t.Fatalf("%s intents: %v", path, err)
				}
				raw = xs[0]
			}
		case "connection":
			if strings.HasSuffix(path, "connection-1") {
				raw = root.Data["connection"]
			} else {
				var xs []json.RawMessage
				if err := json.Unmarshal(root.Data["connections"], &xs); err != nil || len(xs) != 1 {
					t.Fatalf("%s connections: %v", path, err)
				}
				raw = xs[0]
			}
		case "receipt":
			var xs []json.RawMessage
			if err := json.Unmarshal(root.Data["receipts"], &xs); err != nil || len(xs) != 1 {
				t.Fatalf("%s receipts: %v", path, err)
			}
			raw = xs[0]
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			t.Fatal(err)
		}
		got := make([]string, 0, len(obj))
		for k := range obj {
			got = append(got, k)
		}
		sort.Strings(got)
		sort.Strings(want)
		if !slices.Equal(got, want) {
			t.Fatalf("%s %s keys=%v want=%v", path, name, got, want)
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

func TestHarnessAuthorityDetailReceiptFailureDoesNotFabricateEmptyLineage(t *testing.T) {
	now := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	intent := domain.HarnessPairingIntent{ID: "intent-1", Digest: domain.DigestSHA256([]byte("intent")), ExpiresAt: now.Add(time.Hour), Status: domain.HarnessPairingIntentApproved}
	connection := domain.HarnessConnection{ID: "connection-1", InstallationID: "i", HarnessIdentity: "codex", MissionID: "mission", AdapterDigest: domain.DigestSHA256([]byte("adapter")), ProtocolFingerprint: domain.DigestSHA256([]byte("protocol")), Generation: 7, ExpiresAt: now.Add(time.Hour)}
	for _, path := range []string{"/harness-pairing-intents/intent-1", "/harness-connections/connection-1"} {
		t.Run(path, func(t *testing.T) {
			c := &HarnessAuthorityController{Svc: harnessAuthorityStub{intent: intent, connection: connection, receiptErr: errors.New("receipt store unavailable")}, Now: func() time.Time { return now }}
			r := chi.NewRouter()
			c.Register(r)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), `"receipts":[]`) {
				t.Fatalf("fabricated empty receipt lineage: %s", rec.Body.String())
			}
		})
	}
}

func TestAuthorityReceiptViewProjectsOnlyTargetAuthorityLineage(t *testing.T) {
	now := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	digest := domain.DigestSHA256([]byte("target"))
	views := receiptViews([]domain.HarnessAuthorityReceipt{{ID: "r", Action: "revoke", TargetType: "harness_connection", TargetID: "connection-7", TargetDigest: digest, ExpectedGeneration: 7, RequestKey: "PRIVATE_KEY", RequestFingerprint: domain.DigestSHA256([]byte("PRIVATE_FP")), OwnerPrincipal: "PRIVATE_OWNER", ConfirmationRef: "PRIVATE_CONFIRM", CreatedAt: now}})
	if len(views) != 1 || views[0].TargetType != "harness_connection" || views[0].TargetID != "connection-7" || views[0].TargetDigest != digest || views[0].ExpectedGeneration != 7 || !views[0].Confirmed {
		t.Fatalf("view=%+v", views)
	}
	body, err := json.Marshal(views[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"PRIVATE_KEY", "PRIVATE_FP", "PRIVATE_OWNER", "PRIVATE_CONFIRM", "requestKey", "requestFingerprint", "ownerPrincipal", "confirmationRef"} {
		if strings.Contains(string(body), private) {
			t.Fatalf("receipt leaked %q: %s", private, body)
		}
	}
}

func TestHarnessAuthorityDetailFiltersReceiptsToCurrentAuthorityLineage(t *testing.T) {
	now := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	intent := domain.HarnessPairingIntent{ID: "intent-1", Digest: domain.DigestSHA256([]byte("intent")), ExpectedGeneration: 3, ExpiresAt: now.Add(time.Hour), Status: domain.HarnessPairingIntentApproved}
	connection := domain.HarnessConnection{ID: "connection-1", InstallationID: "i", HarnessIdentity: "codex", MissionID: "mission", AdapterDigest: domain.DigestSHA256([]byte("adapter")), ProtocolFingerprint: domain.DigestSHA256([]byte("protocol")), Generation: 7, ExpiresAt: now.Add(time.Hour)}
	connectionDigest, err := connection.AuthorityDigest()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path, targetType, targetID string
		digest                           domain.SHA256Digest
		generation                       int64
	}{
		{"intent", "/harness-pairing-intents/intent-1", "pairing_intent", string(intent.ID), intent.Digest, intent.ExpectedGeneration},
		{"connection", "/harness-connections/connection-1", "harness_connection", string(connection.ID), connectionDigest, connection.Generation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			receipts := []domain.HarnessAuthorityReceipt{
				{ID: "current", TargetType: tc.targetType, TargetID: tc.targetID, TargetDigest: tc.digest, ExpectedGeneration: tc.generation, CreatedAt: now},
				{ID: "other-id", TargetType: tc.targetType, TargetID: "other", TargetDigest: tc.digest, ExpectedGeneration: tc.generation, CreatedAt: now},
				{ID: "wrong-digest", TargetType: tc.targetType, TargetID: tc.targetID, TargetDigest: domain.DigestSHA256([]byte("wrong")), ExpectedGeneration: tc.generation, CreatedAt: now},
				{ID: "wrong-generation", TargetType: tc.targetType, TargetID: tc.targetID, TargetDigest: tc.digest, ExpectedGeneration: tc.generation + 1, CreatedAt: now},
			}
			c := &HarnessAuthorityController{Svc: harnessAuthorityStub{intent: intent, connection: connection, receipts: receipts}, Now: func() time.Time { return now }}
			router := chi.NewRouter()
			c.Register(router)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != 200 {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			var body struct {
				Data struct {
					Receipts []AuthorityReceiptView `json:"receipts"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body.Data.Receipts) != 1 || body.Data.Receipts[0].ID != "current" {
				t.Fatalf("receipts=%+v", body.Data.Receipts)
			}
		})
	}
}
