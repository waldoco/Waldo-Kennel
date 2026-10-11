package bridgeruntime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/devicebridge"
)

// Display timing. A shell that is not already waiting when a notification arrives
// is treated as absent; one that receives it but never answers is treated as lost.
const (
	displayOfferWindow = 2 * time.Second
	displayAckWindow   = 10 * time.Second
	displayPollWindow  = 25 * time.Second
	displayAckBytes    = 256
)

// DisplayRequest is the display-only text handed to the desktop shell. It carries
// no command identity, owner, device, or authority.
type DisplayRequest struct {
	RequestID string `json:"request_id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Severity  string `json:"severity"`
}

type pendingDisplay struct {
	request DisplayRequest
	shown   chan bool
}

// DisplayRelay is the notify_local DisplaySurface. The daemon owns no native UI, so
// it hands the text to the desktop shell and reports delivered only when the shell
// acknowledges that the OS showed it. No shell, timeout, cancellation or refusal
// stays false, which the handler maps to delivery_unknown.
type DisplayRelay struct {
	offers  chan *pendingDisplay
	mu      sync.Mutex
	pending map[string]*pendingDisplay
	// Overridable windows keep tests fast; zero values use the constants above.
	offerWindow, ackWindow, pollWindow time.Duration
}

var _ devicebridge.DisplaySurface = (*DisplayRelay)(nil)

func NewDisplayRelay() *DisplayRelay {
	return &DisplayRelay{offers: make(chan *pendingDisplay), pending: map[string]*pendingDisplay{}}
}

func window(configured, fallback time.Duration) time.Duration {
	if configured > 0 {
		return configured
	}
	return fallback
}

func (r *DisplayRelay) Display(ctx context.Context, _ devicebridge.Scope, n devicebridge.Notification) (bool, error) {
	if r == nil {
		return false, nil
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return false, nil
	}
	p := &pendingDisplay{request: DisplayRequest{RequestID: hex.EncodeToString(id), Title: n.Title, Body: n.Body, Severity: n.Severity}, shown: make(chan bool, 1)}
	r.mu.Lock()
	r.pending[p.request.RequestID] = p
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.pending, p.request.RequestID)
		r.mu.Unlock()
	}()
	offer := time.NewTimer(window(r.offerWindow, displayOfferWindow))
	defer offer.Stop()
	// Unbuffered: the offer succeeds only if a shell is waiting right now.
	select {
	case r.offers <- p:
	case <-offer.C:
		return false, nil
	case <-ctx.Done():
		return false, nil
	}
	ack := time.NewTimer(window(r.ackWindow, displayAckWindow))
	defer ack.Stop()
	select {
	case shown := <-p.shown:
		return shown, nil
	case <-ack.C:
		return false, nil
	case <-ctx.Done():
		return false, nil
	}
}

// next waits for one display request until the poll window or ctx ends.
func (r *DisplayRelay) next(ctx context.Context) (DisplayRequest, bool) {
	poll := time.NewTimer(window(r.pollWindow, displayPollWindow))
	defer poll.Stop()
	select {
	case p := <-r.offers:
		return p.request, true
	case <-poll.C:
	case <-ctx.Done():
	}
	return DisplayRequest{}, false
}

// ack resolves a request still awaiting its answer. Unknown, expired or repeated
// request IDs are refused, so one request yields at most one answer.
func (r *DisplayRelay) ack(id string, shown bool) bool {
	r.mu.Lock()
	p, ok := r.pending[id]
	if ok {
		delete(r.pending, id)
	}
	r.mu.Unlock()
	if !ok {
		return false
	}
	p.shown <- shown
	return true
}

// Handler serves GET /display/next (long poll: 200 request or 204) and
// POST /display/ack. It applies the same local boundary as the activation API:
// loopback peer plus the caller-supplied KennelBridge authorization.
func (r *DisplayRelay) Handler(authorize func(*http.Request) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		host, _, err := net.SplitHostPort(req.RemoteAddr)
		ip := net.ParseIP(host)
		if r == nil || err != nil || ip == nil || !ip.IsLoopback() || authorize == nil || !authorize(req) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if req.URL.RawQuery != "" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		switch {
		case req.URL.Path == "/display/next" && req.Method == http.MethodGet:
			request, ok := r.next(req.Context())
			if !ok {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			_ = json.NewEncoder(w).Encode(request)
		case req.URL.Path == "/display/ack" && req.Method == http.MethodPost:
			if strings.TrimSpace(strings.Split(req.Header.Get("Content-Type"), ";")[0]) != "application/json" {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			raw, err := io.ReadAll(io.LimitReader(req.Body, displayAckBytes+1))
			if err != nil || len(raw) > displayAckBytes {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			id, shown, ok := decodeDisplayAck(raw)
			if !ok {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			if !r.ack(id, shown) {
				http.Error(w, "unknown request", http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, req)
		}
	})
}

// decodeDisplayAck accepts exactly {"request_id": <32 hex>, "shown": <bool>}, with
// no duplicate or unknown keys.
func decodeDisplayAck(raw []byte) (string, bool, bool) {
	if !utf8.Valid(raw) {
		return "", false, false
	}
	d := json.NewDecoder(strings.NewReader(string(raw)))
	if t, err := d.Token(); err != nil || t != json.Delim('{') {
		return "", false, false
	}
	var id string
	var shown bool
	seen := map[string]bool{}
	for d.More() {
		t, err := d.Token()
		name, ok := t.(string)
		if err != nil || !ok || seen[name] {
			return "", false, false
		}
		seen[name] = true
		switch name {
		case "request_id":
			err = d.Decode(&id)
		case "shown":
			// A literal boolean only: decoding null into a bool would silently succeed.
			var literal json.RawMessage
			if err = d.Decode(&literal); err == nil {
				switch string(literal) {
				case "true":
					shown = true
				case "false":
				default:
					return "", false, false
				}
			}
		default:
			return "", false, false
		}
		if err != nil {
			return "", false, false
		}
	}
	if t, err := d.Token(); err != nil || t != json.Delim('}') || len(seen) != 2 {
		return "", false, false
	}
	if _, err := d.Token(); err != io.EOF {
		return "", false, false
	}
	if len(id) != 32 || strings.Trim(id, "0123456789abcdef") != "" {
		return "", false, false
	}
	return id, shown, true
}
