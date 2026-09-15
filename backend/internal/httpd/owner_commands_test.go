package httpd

import (
	"context"
	"encoding/json"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/config"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ownercommand"
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
