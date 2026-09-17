package controllers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/envelope"
	outcomevc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

type NeedsYouQuestionIDParam struct {
	QuestionID string `path:"questionId" description:"Durable Needs-You question identifier."`
}
type NeedsYouQuestionsEnvelope struct {
	Questions []domain.NeedsYouQuestion `json:"questions"`
}
type NeedsYouAnswerRequest struct {
	RequestKey string                     `json:"requestKey"`
	Generation string                     `json:"generation"`
	Decision   *domain.ChatDecisionAnswer `json:"decision,omitempty"`
	Input      *domain.ChatInputAnswer    `json:"input,omitempty"`
}
type NeedsYouQuestionEnvelope struct {
	Question domain.NeedsYouQuestion `json:"question"`
}
type NeedsYouReconcileRequest struct {
	Generation string `json:"generation"`
}

const maxNeedsYouBody = 1 << 20

func decodeNeedsYouBody(w http.ResponseWriter, r *http.Request, into any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxNeedsYouBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "validation", "INVALID_BODY", "request body is not valid JSON", nil)
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "validation", "INVALID_BODY", "request body must contain exactly one JSON value", nil)
		return false
	}
	return true
}

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
	envelope.WriteJSON(w, http.StatusOK, NeedsYouQuestionsEnvelope{Questions: q})
}
func (c *OutcomesController) answerNeedsYou(w http.ResponseWriter, r *http.Request) {
	if c.NeedsYou == nil {
		writeNeedsYouUnavailable(w, r)
		return
	}
	var req NeedsYouAnswerRequest
	if !decodeNeedsYouBody(w, r, &req) {
		return
	}
	q, err := c.NeedsYou.AnswerNeedsYou(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")), chi.URLParam(r, "questionId"), domain.NeedsYouAnswer{RequestKey: req.RequestKey, Generation: req.Generation, Decision: req.Decision, Input: req.Input})
	if err != nil {
		writeNeedsYouError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, NeedsYouQuestionEnvelope{Question: q})
}
func (c *OutcomesController) reconcileNeedsYou(w http.ResponseWriter, r *http.Request) {
	if c.NeedsYou == nil {
		writeNeedsYouUnavailable(w, r)
		return
	}
	var in NeedsYouReconcileRequest
	if !decodeNeedsYouBody(w, r, &in) {
		return
	}
	q, err := c.NeedsYou.ReconcileNeedsYou(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")), chi.URLParam(r, "questionId"), in.Generation)
	if err != nil {
		writeNeedsYouError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, NeedsYouQuestionEnvelope{Question: q})
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
