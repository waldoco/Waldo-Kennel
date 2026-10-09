package httpd

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/macaction"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ownercommand"
	"github.com/go-chi/chi/v5"
	"io"
	"net/http"
	"time"
)

type MacActions interface {
	Open(context.Context) ([]macaction.Request, error)
	Decide(context.Context, string, string, string, bool, time.Time) error
	Output(context.Context, string, string) ([]byte, error)
	History(context.Context) ([]macaction.Request, error)
}

func mountMacActions(r chi.Router, authority *ownercommand.Authority, actions MacActions) {
	if authority == nil || actions == nil {
		return
	}
	r.Route("/internal/owner-commands/mac-actions", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
				if !localControlRequest(q) {
					http.NotFound(w, q)
					return
				}
				if _, ok := authority.Authenticate(q.Header.Get("Authorization")); !ok {
					http.Error(w, "unauthorized", 401)
					return
				}
				w.Header().Set("Cache-Control", "no-store")
				next.ServeHTTP(w, q)
			})
		})
		r.Get("/pending", func(w http.ResponseWriter, q *http.Request) {
			rows, e := actions.Open(q.Context())
			if e != nil {
				http.Error(w, "unavailable", 503)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(rows)
		})
		r.Get("/history", func(w http.ResponseWriter, q *http.Request) {
			rows, err := actions.History(q.Context())
			if err != nil {
				http.Error(w, "unavailable", 503)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(rows)
		})
		r.Post("/decision", func(w http.ResponseWriter, q *http.Request) {
			var in struct {
				DeviceID    string `json:"device_id"`
				ID          string `json:"id"`
				Fingerprint string `json:"fingerprint"`
				Decision    string `json:"decision"`
			}
			// Duplicate keys are rejected before JSON decoding or owner authority use.
			raw, e := io.ReadAll(http.MaxBytesReader(w, q.Body, 2048))
			if e != nil {
				http.Error(w, "invalid", 400)
				return
			}
			d := json.NewDecoder(bytes.NewReader(raw))
			tok, e := d.Token()
			if e != nil || tok != json.Delim('{') {
				http.Error(w, "invalid", 400)
				return
			}
			seen := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				name, ok := key.(string)
				if e != nil || !ok || seen[name] {
					http.Error(w, "invalid", 400)
					return
				}
				seen[name] = true
				var v any
				if d.Decode(&v) != nil {
					http.Error(w, "invalid", 400)
					return
				}
			}
			if _, e = d.Token(); e != nil || len(seen) != 4 {
				http.Error(w, "invalid", 400)
				return
			}
			if _, e = d.Token(); e != io.EOF {
				http.Error(w, "invalid", 400)
				return
			}
			d = json.NewDecoder(bytes.NewReader(raw))
			d.DisallowUnknownFields()
			if d.Decode(&in) != nil || in.DeviceID == "" || in.ID == "" || len(in.Fingerprint) != 64 || (in.Decision != "approve" && in.Decision != "deny") {
				http.Error(w, "invalid", 400)
				return
			}
			if actions.Decide(q.Context(), in.DeviceID, in.ID, in.Fingerprint, in.Decision == "approve", time.Now().UTC()) != nil {
				http.Error(w, "stale decision", 409)
				return
			}
			w.WriteHeader(204)
		})
		r.Get("/{device}/{command}/output", func(w http.ResponseWriter, q *http.Request) {
			raw, e := actions.Output(q.Context(), chi.URLParam(q, "device"), chi.URLParam(q, "command"))
			if e != nil {
				http.NotFound(w, q)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write(raw)
		})
	})
}
