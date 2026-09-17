package httpd

import (
	"context"
	"encoding/json"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/config"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/harnessconnection"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/harnesspairing"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ownercommand"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ownerproof"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type decisionStore struct {
	d domain.AttemptReplacementDecision
	n int
}

func (s *decisionStore) CreateAttemptReplacementDecision(_ context.Context, d domain.AttemptReplacementDecision) (domain.AttemptReplacementDecision, bool, error) {
	s.n++
	s.d = d
	return d, true, nil
}
func (s *decisionStore) GetAttemptReplacementDecision(context.Context, domain.AttemptReplacementDecisionID) (domain.AttemptReplacementDecision, bool, error) {
	return domain.AttemptReplacementDecision{}, false, nil
}
func TestOwnerCommandRequiresPrivateCapability(t *testing.T) {
	store := &decisionStore{}
	r := NewRouterWithControl(config.Config{}, discardLogger(), nil, APIDeps{}, ControlDeps{OwnerAuthority: ownercommand.NewAuthority(strings.Repeat("t", 32), "apprun-test"), ReplacementDecisions: store})
	body := `{"outcomeId":"out","predecessorAttemptId":"a","planRevisionId":"p","workUnitId":"w","runIntentGeneration":1,"contractRevisionNumber":1,"action":"replace","requestKey":"key"}`
	for _, tc := range []struct {
		name, auth string
		want       int
	}{{"missing", "", 401}, {"wrong", "KennelOwner " + strings.Repeat("x", 32), 401}, {"valid", "KennelOwner " + strings.Repeat("t", 32), 201}} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/internal/owner-commands/attempt-replacement-decisions", strings.NewReader(body))
			req.Header.Set("Authorization", tc.auth)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
	if store.n != 1 || store.d.OwnerPrincipal != "local-owner:apprun-test" {
		b, _ := json.Marshal(store.d)
		t.Fatalf("writes=%d decision=%s", store.n, b)
	}
}
func TestOwnerCommandRejectsRebindingHost(t *testing.T) {
	s := &decisionStore{}
	r := NewRouterWithControl(config.Config{}, discardLogger(), nil, APIDeps{}, ControlDeps{OwnerAuthority: ownercommand.NewAuthority(strings.Repeat("t", 32), "apprun-test"), ReplacementDecisions: s})
	req := httptest.NewRequest(http.MethodPost, "http://evil.example/internal/owner-commands/attempt-replacement-decisions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "KennelOwner "+strings.Repeat("t", 32))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 404 || s.n != 0 {
		t.Fatalf("code=%d writes=%d", w.Code, s.n)
	}
}

func TestOwnerCommandRouteAbsentWithoutAuthority(t *testing.T) {
	r := NewRouterWithControl(config.Config{}, discardLogger(), nil, APIDeps{}, ControlDeps{ReplacementDecisions: &decisionStore{}})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/internal/owner-commands/attempt-replacement-decisions", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestOwnerCommandRejectsTrailingJSONWithoutWriting(t *testing.T) {
	store := &decisionStore{}
	r := NewRouterWithControl(config.Config{}, discardLogger(), nil, APIDeps{}, ControlDeps{OwnerAuthority: ownercommand.NewAuthority(strings.Repeat("t", 32), "apprun-test"), ReplacementDecisions: store})
	body := `{"outcomeId":"out","predecessorAttemptId":"a","planRevisionId":"p","workUnitId":"w","runIntentGeneration":1,"contractRevisionNumber":1,"action":"replace","requestKey":"key"} {}`
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/internal/owner-commands/attempt-replacement-decisions", strings.NewReader(body))
	req.Header.Set("Authorization", "KennelOwner "+strings.Repeat("t", 32))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || store.n != 0 {
		t.Fatalf("code=%d writes=%d", w.Code, store.n)
	}
}

func TestOwnerCommandOriginAndLoopbackHostMatrix(t *testing.T) {
	for _, tc := range []struct {
		name, host, origin string
		want               int
	}{
		{"127 no capability", "127.0.0.1", "", http.StatusUnauthorized},
		{"localhost no capability", "localhost", "", http.StatusUnauthorized},
		{"ipv6 no capability", "[::1]", "", http.StatusUnauthorized},
		{"arbitrary origin", "127.0.0.1", "https://evil.example", http.StatusForbidden},
		{"alternate loopback origin", "127.0.0.1", "http://localhost:9999", http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRouterWithControl(config.Config{}, discardLogger(), nil, APIDeps{}, ControlDeps{OwnerAuthority: ownercommand.NewAuthority(strings.Repeat("t", 32), "apprun-test"), ReplacementDecisions: &decisionStore{}})
			req := httptest.NewRequest(http.MethodPost, "http://"+tc.host+"/internal/owner-commands/attempt-replacement-decisions", strings.NewReader(`{}`))
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("code=%d body=%s want=%d", w.Code, w.Body.String(), tc.want)
			}
		})
	}
}

func pairingIntentBody() string {
	return `{"kind":"pair","connectionId":"hc-owner","installationId":"install","adapterDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","harnessIdentity":"codex","providerVersion":"0.154.0","protocolFingerprint":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","missionId":"mission","capabilityClasses":["turn"],"expectedGeneration":1}`
}
func TestPairingIntentRequiresOwnerCapabilityAndNeverExposesAdapterIssue(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	coordinator := harnesspairing.New(store, harnessconnection.New(store))
	authority := ownercommand.NewAuthority(strings.Repeat("t", 32), "apprun-test")
	r := NewRouterWithControl(config.Config{}, discardLogger(), nil, APIDeps{}, ControlDeps{OwnerAuthority: authority, PairingCoordinator: coordinator})
	for _, tc := range []struct {
		name, auth string
		want       int
	}{{"missing", "", 401}, {"wrong", "KennelOwner " + strings.Repeat("x", 32), 401}, {"owner", "KennelOwner " + strings.Repeat("t", 32), 201}} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/internal/owner-commands/harness-pairing-intents", strings.NewReader(pairingIntentBody()))
			req.Header.Set("Authorization", tc.auth)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
			}
			if tc.want == 201 {
				var body struct {
					Data struct {
						IntentID string `json:"intentId"`
						Secret   string `json:"secret"`
					} `json:"data"`
				}
				if json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Data.IntentID == "" || body.Data.Secret == "" {
					t.Fatalf("body=%s", w.Body.String())
				}
				stored, found, err := store.GetHarnessPairingChallenge(context.Background(), domain.PairingChallengeID(body.Data.IntentID))
				if err != nil || !found || stored.AppRunID != "apprun-test" {
					t.Fatalf("stored app run=(%q,%v,%v)", stored.AppRunID, found, err)
				}
			}
		})
	}
	// The public/adapter-shaped route does not exist on HTTP, even with an exact tuple.
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/harness-pairing/request-challenge", strings.NewReader(pairingIntentBody()))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("adapter Issue route code=%d", w.Code)
	}
}

func TestPairingIntentRejectsRequestControlledAppRunID(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	coordinator := harnesspairing.New(store, harnessconnection.New(store))
	r := NewRouterWithControl(config.Config{}, discardLogger(), nil, APIDeps{}, ControlDeps{OwnerAuthority: ownercommand.NewAuthority(strings.Repeat("t", 32), "apprun-authenticated"), PairingCoordinator: coordinator})
	body := strings.TrimSuffix(pairingIntentBody(), "}") + `,"appRunId":"apprun-other"}`
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/internal/owner-commands/harness-pairing-intents", strings.NewReader(body))
	req.Header.Set("Authorization", "KennelOwner "+strings.Repeat("t", 32))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestPairingIntentOwnerRouteRejectsLANHostAndUnknownFields(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	c := harnesspairing.New(store, harnessconnection.New(store))
	r := NewRouterWithControl(config.Config{}, discardLogger(), nil, APIDeps{}, ControlDeps{OwnerAuthority: ownercommand.NewAuthority(strings.Repeat("t", 32), "run"), PairingCoordinator: c})
	auth := "KennelOwner " + strings.Repeat("t", 32)
	for _, tc := range []struct {
		url, body string
		want      int
	}{{"http://evil.example/internal/owner-commands/harness-pairing-intents", pairingIntentBody(), 404}, {"http://127.0.0.1/internal/owner-commands/harness-pairing-intents", strings.TrimSuffix(pairingIntentBody(), "}") + `,"approval":true}`, 400}} {
		req := httptest.NewRequest(http.MethodPost, tc.url, strings.NewReader(tc.body))
		req.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatalf("url=%s code=%d body=%s", tc.url, w.Code, w.Body.String())
		}
	}
}

func ownerProofBody(class string) string {
	return `{"missionId":"mission","contentDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","targetId":"question-1","targetGeneration":2,"class":"` + class + `"}`
}

func TestOwnerProofMintRequiresOwnerAndBindsAuthenticatedAppRun(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	r := NewRouterWithControl(config.Config{}, discardLogger(), nil, APIDeps{}, ControlDeps{
		OwnerAuthority:   ownercommand.NewAuthority(strings.Repeat("t", 32), "apprun-authenticated"),
		OwnerProofKernel: ownerproof.New(store),
	})
	for _, tc := range []struct {
		name, auth string
		want       int
	}{{"missing", "", 401}, {"wrong", "KennelOwner " + strings.Repeat("x", 32), 401}, {"owner", "KennelOwner " + strings.Repeat("t", 32), 201}} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/internal/owner-commands/owner-proofs", strings.NewReader(ownerProofBody("answer")))
			req.Header.Set("Authorization", tc.auth)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
			}
			if tc.want != http.StatusCreated {
				return
			}
			var body struct {
				Data struct {
					ProofID string `json:"proofId"`
					Bearer  string `json:"bearer"`
				} `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Data.ProofID == "" || body.Data.Bearer == "" {
				t.Fatalf("body=%s err=%v", w.Body.String(), err)
			}
			stored, found, err := store.GetOwnerProof(context.Background(), domain.OwnerProofID(body.Data.ProofID))
			if err != nil || !found || stored.AppRunID != "apprun-authenticated" || stored.Verifier == "" {
				t.Fatalf("stored=(%+v,%v,%v)", stored, found, err)
			}
			if strings.Contains(w.Body.String(), stored.Verifier.String()) {
				t.Fatal("response exposed verifier")
			}
		})
	}
}

func TestOwnerProofMintRejectsMaterialAndRequestControlledAuthority(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	r := NewRouterWithControl(config.Config{}, discardLogger(), nil, APIDeps{}, ControlDeps{
		OwnerAuthority:   ownercommand.NewAuthority(strings.Repeat("t", 32), "apprun-authenticated"),
		OwnerProofKernel: ownerproof.New(store),
	})
	auth := "KennelOwner " + strings.Repeat("t", 32)
	for _, body := range []string{
		ownerProofBody("replace"), ownerProofBody("approval"), ownerProofBody("accept"),
		strings.TrimSuffix(ownerProofBody("answer"), "}") + `,"confirmationRef":"untrusted"}`,
		strings.TrimSuffix(ownerProofBody("answer"), "}") + `,"appRunId":"apprun-other"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/internal/owner-commands/owner-proofs", strings.NewReader(body))
		req.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("code=%d body=%s request=%s", w.Code, w.Body.String(), body)
		}
	}
}

func TestOwnerProofRouteAbsentWithoutKernelAndHiddenFromLAN(t *testing.T) {
	authority := ownercommand.NewAuthority(strings.Repeat("t", 32), "run")
	t.Run("absent", func(t *testing.T) {
		r := NewRouterWithControl(config.Config{}, discardLogger(), nil, APIDeps{}, ControlDeps{OwnerAuthority: authority})
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/internal/owner-commands/owner-proofs", strings.NewReader(ownerProofBody("turn")))
		req.Header.Set("Authorization", "KennelOwner "+strings.Repeat("t", 32))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("code=%d", w.Code)
		}
	})
	t.Run("lan", func(t *testing.T) {
		store := sqlitetest.MustOpen(t)
		r := NewRouterWithControl(config.Config{}, discardLogger(), nil, APIDeps{}, ControlDeps{OwnerAuthority: authority, OwnerProofKernel: ownerproof.New(store)})
		req := httptest.NewRequest(http.MethodPost, "http://evil.example/internal/owner-commands/owner-proofs", strings.NewReader(ownerProofBody("turn")))
		req.Header.Set("Authorization", "KennelOwner "+strings.Repeat("t", 32))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("code=%d", w.Code)
		}
	})
}
