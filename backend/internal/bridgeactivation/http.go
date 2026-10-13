package bridgeactivation

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"unicode/utf8"
)

// Authorize must verify the existing local owner boundary, including origin/CSRF
// protections. Loopback alone is NOT authorization. The handler stays unmounted.
type Authorize func(*http.Request) bool

func Handler(c *Controller, authorize Authorize) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		ip := net.ParseIP(host)
		if c == nil || err != nil || ip == nil || !ip.IsLoopback() || authorize == nil || !authorize(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.RawQuery != "" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		switch {
		case r.URL.Path == "/status" && r.Method == http.MethodGet:
			s, err := c.Status(r.Context())
			if err != nil {
				http.Error(w, "status unavailable", http.StatusServiceUnavailable)
				return
			}
			_ = json.NewEncoder(w).Encode(s)
		case r.URL.Path == "/pair" && r.Method == http.MethodPost:
			// Decoder.DisallowUnknownFields does not reject duplicate keys. Decode raw
			// fields explicitly before any request reaches the coordinator.
			media := strings.Split(r.Header.Get("Content-Type"), ";")[0]
			if strings.TrimSpace(media) != "application/json" {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			raw, err := io.ReadAll(io.LimitReader(r.Body, 2049))
			if err != nil || len(raw) > 2048 {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			req, err := decodePair(raw)
			if err != nil {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			if err = c.Pair(r.Context(), req.Code, req.Label, req.Capabilities, req.SessionConsent); err != nil {
				status := http.StatusConflict
				if errors.Is(err, ErrNotReady) {
					status = http.StatusServiceUnavailable
				}
				if err == ErrInvalid {
					status = http.StatusBadRequest
				}
				http.Error(w, "pairing unavailable", status)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

type pairBody struct {
	Code           string
	Label          string
	Capabilities   []string
	SessionConsent bool
}

func decodePair(raw []byte) (pairBody, error) {
	var out pairBody
	if !utf8.Valid(raw) {
		return out, ErrInvalid
	}
	d := json.NewDecoder(strings.NewReader(string(raw)))
	t, err := d.Token()
	if err != nil || t != json.Delim('{') {
		return out, ErrInvalid
	}
	seen := map[string]bool{}
	for d.More() {
		t, err = d.Token()
		name, ok := t.(string)
		if err != nil || !ok || seen[name] {
			return out, ErrInvalid
		}
		seen[name] = true
		switch name {
		case "code":
			err = d.Decode(&out.Code)
		case "label":
			err = d.Decode(&out.Label)
		case "session_consent":
			err = d.Decode(&out.SessionConsent)
		case "capabilities":
			err = d.Decode(&out.Capabilities)
		default:
			return out, ErrInvalid
		}
		if err != nil {
			return out, ErrInvalid
		}
	}
	t, err = d.Token()
	if err != nil || t != json.Delim('}') || (len(seen) != 3 && len(seen) != 4) {
		return out, ErrInvalid
	}
	if _, err = d.Token(); err != io.EOF {
		return out, ErrInvalid
	}
	return out, nil
}
