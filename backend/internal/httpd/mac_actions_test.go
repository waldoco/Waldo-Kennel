package httpd

import (
	"context"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/macaction"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ownercommand"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type macActionsFake struct{ decisions int }

func (*macActionsFake) Open(context.Context) ([]macaction.Request, error) {
	return []macaction.Request{}, nil
}
func (*macActionsFake) History(context.Context) ([]macaction.Request, error) {
	return []macaction.Request{}, nil
}
func (*macActionsFake) Output(context.Context, string, string) ([]byte, error) {
	return []byte("local full output"), nil
}
func (f *macActionsFake) Decide(context.Context, string, string, string, bool, time.Time) error {
	f.decisions++
	return nil
}
func TestMacActionsLocalOwnerAuthority(t *testing.T) {
	f := &macActionsFake{}
	r := chi.NewRouter()
	mountMacActions(r, ownercommand.NewAuthority(strings.Repeat("t", 32), "test-run"), f)
	valid := `{"device_id":"dev","id":"cmd","fingerprint":"` + strings.Repeat("a", 64) + `","decision":"approve"}`
	for _, tc := range []struct {
		name, host, auth, body string
		want                   int
	}{
		{"no authority", "127.0.0.1", "", valid, 401},
		{"wrong authority", "127.0.0.1", "KennelOwner wrong", valid, 401},
		{"rebinding host", "evil.example", "KennelOwner " + strings.Repeat("t", 32), valid, 404},
		{"duplicate field", "127.0.0.1", "KennelOwner " + strings.Repeat("t", 32), strings.Replace(valid, `"id":"cmd"`, `"id":"cmd","id":"other"`, 1), 400},
		{"exact decision", "127.0.0.1", "KennelOwner " + strings.Repeat("t", 32), valid, 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := httptest.NewRequest(http.MethodPost, "http://"+tc.host+"/internal/owner-commands/mac-actions/decision", strings.NewReader(tc.body))
			q.Header.Set("Authorization", tc.auth)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, q)
			if w.Code != tc.want {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	if f.decisions != 1 {
		t.Fatal(f.decisions)
	}
	absent := chi.NewRouter()
	mountMacActions(absent, nil, f)
	q := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/internal/owner-commands/mac-actions/pending", nil)
	w := httptest.NewRecorder()
	absent.ServeHTTP(w, q)
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}
