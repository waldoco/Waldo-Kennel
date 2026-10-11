package bridgeruntime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/devicebridge"
)

func fastRelay() *DisplayRelay {
	r := NewDisplayRelay()
	r.offerWindow, r.ackWindow, r.pollWindow = 200*time.Millisecond, 200*time.Millisecond, 200*time.Millisecond
	return r
}

var neutral = devicebridge.Notification{ID: "n-1", Title: "Waldo status", Body: "Your Mac is connected.", Severity: "info"}

func display(r *DisplayRelay, ctx context.Context) <-chan bool {
	out := make(chan bool, 1)
	go func() {
		shown, err := r.Display(ctx, devicebridge.Scope{}, neutral)
		if err != nil {
			panic(err)
		}
		out <- shown
	}()
	return out
}

func TestDisplayWithoutShellStaysUndelivered(t *testing.T) {
	r := fastRelay()
	if shown := <-display(r, context.Background()); shown {
		t.Fatal("no shell was waiting, yet delivery was claimed")
	}
	if len(r.pending) != 0 {
		t.Fatal("pending request leaked")
	}
}

func TestDisplayReportsOnlyTheShellAnswer(t *testing.T) {
	for _, answer := range []bool{true, false} {
		r := fastRelay()
		result := display(r, context.Background())
		request, ok := r.next(context.Background())
		if !ok {
			t.Fatal("shell did not receive the request")
		}
		if request.Title != neutral.Title || request.Body != neutral.Body || request.Severity != neutral.Severity || len(request.RequestID) != 32 {
			t.Fatalf("request does not carry the exact display text: %+v", request)
		}
		if !r.ack(request.RequestID, answer) {
			t.Fatal("live request refused its answer")
		}
		if got := <-result; got != answer {
			t.Fatalf("shown=%v reported as %v", answer, got)
		}
		if r.ack(request.RequestID, true) {
			t.Fatal("a second answer was accepted for one request")
		}
	}
}

func TestDisplayLostAcknowledgementStaysUndelivered(t *testing.T) {
	r := fastRelay()
	result := display(r, context.Background())
	request, ok := r.next(context.Background())
	if !ok {
		t.Fatal("shell did not receive the request")
	}
	if <-result {
		t.Fatal("unacknowledged display was claimed delivered")
	}
	if r.ack(request.RequestID, true) {
		t.Fatal("a late answer was accepted after the ack window")
	}
}

func TestDisplayCancellationStaysUndelivered(t *testing.T) {
	r := NewDisplayRelay()
	ctx, cancel := context.WithCancel(context.Background())
	result := display(r, ctx)
	if _, ok := r.next(context.Background()); !ok {
		t.Fatal("shell did not receive the request")
	}
	cancel()
	select {
	case shown := <-result:
		if shown {
			t.Fatal("cancelled display was claimed delivered")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not end the wait")
	}
}

func TestNextWithoutWorkTimesOut(t *testing.T) {
	r := fastRelay()
	if _, ok := r.next(context.Background()); ok {
		t.Fatal("idle poll returned a request")
	}
}

func bridgeRequest(method, target, body string, auth bool) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:50000"
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth {
		req.Header.Set("Authorization", "KennelBridge ok")
	}
	return req
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestDisplayHandlerBoundary(t *testing.T) {
	r := fastRelay()
	h := r.Handler(func(req *http.Request) bool { return req.Header.Get("Authorization") == "KennelBridge ok" })
	if rec := serve(h, bridgeRequest(http.MethodGet, "/display/next", "", false)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated next: %d", rec.Code)
	}
	remote := bridgeRequest(http.MethodGet, "/display/next", "", true)
	remote.RemoteAddr = "192.0.2.10:50000"
	if rec := serve(h, remote); rec.Code != http.StatusUnauthorized {
		t.Fatalf("non-loopback next: %d", rec.Code)
	}
	if rec := serve(h, bridgeRequest(http.MethodGet, "/display/next?x=1", "", true)); rec.Code != http.StatusBadRequest {
		t.Fatalf("query string: %d", rec.Code)
	}
	if rec := serve(h, bridgeRequest(http.MethodPost, "/display/next", "", true)); rec.Code != http.StatusNotFound {
		t.Fatalf("wrong method: %d", rec.Code)
	}
	if rec := serve(h, bridgeRequest(http.MethodGet, "/display/next", "", true)); rec.Code != http.StatusNoContent || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("idle poll: %d", rec.Code)
	}
	id := strings.Repeat("a", 32)
	for _, body := range []string{
		`{"request_id":"` + id + `"}`,
		`{"request_id":"` + id + `","shown":null}`,
		`{"request_id":"` + id + `","shown":1}`,
		`{"request_id":"` + id + `","shown":true,"shown":false}`,
		`{"request_id":"` + id + `","shown":true,"extra":1}`,
		`{"request_id":"` + strings.Repeat("A", 32) + `","shown":true}`,
		`{"request_id":"short","shown":true}`,
		`{"request_id":"` + id + `","shown":true} {}`,
		`{"request_id":"` + id + `","shown":true,"pad":"` + strings.Repeat("x", 300) + `"}`,
	} {
		if rec := serve(h, bridgeRequest(http.MethodPost, "/display/ack", body, true)); rec.Code != http.StatusBadRequest {
			t.Fatalf("malformed ack %q: %d", body, rec.Code)
		}
	}
	plain := bridgeRequest(http.MethodPost, "/display/ack", `{"request_id":"`+id+`","shown":true}`, true)
	plain.Header.Set("Content-Type", "text/plain")
	if rec := serve(h, plain); rec.Code != http.StatusBadRequest {
		t.Fatalf("non-JSON ack: %d", rec.Code)
	}
	if rec := serve(h, bridgeRequest(http.MethodPost, "/display/ack", `{"request_id":"`+id+`","shown":true}`, true)); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown request ack: %d", rec.Code)
	}
}

func TestDisplayHandlerRoundTrip(t *testing.T) {
	r := fastRelay()
	r.offerWindow = 2 * time.Second
	h := r.Handler(func(*http.Request) bool { return true })
	result := display(r, context.Background())
	rec := serve(h, bridgeRequest(http.MethodGet, "/display/next", "", true))
	if rec.Code != http.StatusOK {
		t.Fatalf("next: %d", rec.Code)
	}
	var request DisplayRequest
	if err := json.Unmarshal(rec.Body.Bytes(), &request); err != nil || request.Title != neutral.Title || request.Body != neutral.Body {
		t.Fatalf("next body: %s", rec.Body.String())
	}
	ack := bridgeRequest(http.MethodPost, "/display/ack", `{"request_id":"`+request.RequestID+`","shown":true}`, true)
	if rec = serve(h, ack); rec.Code != http.StatusNoContent {
		t.Fatalf("ack: %d", rec.Code)
	}
	if !<-result {
		t.Fatal("acknowledged display not reported delivered")
	}
}
