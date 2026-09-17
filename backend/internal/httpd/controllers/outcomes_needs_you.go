package controllers

import (
	"encoding/json"
	"errors"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/envelope"
	outcomevc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
	"github.com/go-chi/chi/v5"
	"io"
	"net/http"
)

func (c *OutcomesController) currentNeedsYou(w http.ResponseWriter, r *http.Request) {
	if c.NeedsYou == nil {
		writeNeedsYouUnavailable(w, r)
		return
	}
	q, err := c.NeedsYou.CurrentNeedsYou(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")))
	if err != nil {
		writeNeedsYouError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, map[string]any{"questions": q})
}
func (c *OutcomesController) answerNeedsYou(w http.ResponseWriter, r *http.Request) {
	if c.NeedsYou == nil {
		writeNeedsYouUnavailable(w, r)
		return
	}
	var in domain.NeedsYouAnswer
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
		envelope.WriteAPIError(w, r, 400, "validation", "INVALID_BODY", "request body is not valid JSON", nil)
		return
	}
	q, err := c.NeedsYou.AnswerNeedsYou(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")), chi.URLParam(r, "questionId"), in)
	if err != nil {
		writeNeedsYouError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, q)
}
func (c *OutcomesController) reconcileNeedsYou(w http.ResponseWriter, r *http.Request) {
	if c.NeedsYou == nil {
		writeNeedsYouUnavailable(w, r)
		return
	}
	var in struct {
		Generation string `json:"generation"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
		envelope.WriteAPIError(w, r, 400, "validation", "INVALID_BODY", "request body is not valid JSON", nil)
		return
	}
	q, err := c.NeedsYou.ReconcileNeedsYou(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")), chi.URLParam(r, "questionId"), in.Generation)
	if err != nil {
		writeNeedsYouError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, q)
}
func writeNeedsYouUnavailable(w http.ResponseWriter, r *http.Request) {
	envelope.WriteAPIError(w, r, 501, "not_implemented", "NEEDS_YOU_UNAVAILABLE", "needs-you service is unavailable", nil)
}
func writeNeedsYouError(w http.ResponseWriter, r *http.Request, err error) {
	status, code := 500, "NEEDS_YOU_ERROR"
	switch {
	case errors.Is(err, outcomevc.ErrNeedsYouUnavailable):
		status, code = 501, "NEEDS_YOU_UNAVAILABLE"
	case errors.Is(err, outcomevc.ErrNeedsYouNotFound):
		status, code = 404, "NEEDS_YOU_NOT_FOUND"
	case errors.Is(err, outcomevc.ErrNeedsYouStale):
		status, code = 409, "NEEDS_YOU_STALE"
	case errors.Is(err, outcomevc.ErrNeedsYouAnswerInvalid):
		status, code = 400, "NEEDS_YOU_ANSWER_INVALID"
	}
	envelope.WriteAPIError(w, r, status, "needs_you", code, err.Error(), nil)
}
