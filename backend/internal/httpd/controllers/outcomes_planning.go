package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apispec"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/envelope"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	outcomevc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

type interactivePlanningService interface {
	PlanningCandidates(context.Context, domain.OutcomeID, int64) ([]ports.PlanningCandidate, error)
	StartPlanning(context.Context, domain.OutcomeID, outcomevc.StartPlanningInput) (outcomevc.PlanningView, error)
	GetPlanning(context.Context, domain.OutcomeID, domain.PlanningSessionID) (outcomevc.PlanningView, error)
	GetCurrentPlanning(context.Context, domain.OutcomeID) (outcomevc.PlanningView, error)
	ContinuePlanning(context.Context, domain.OutcomeID, domain.PlanningSessionID, outcomevc.PlanningMessageInput) (outcomevc.PlanningView, error)
	FinalizePlanning(context.Context, domain.OutcomeID, domain.PlanningSessionID, outcomevc.PlanningFinalizeInput) (outcomevc.PlanningView, error)
	CancelPlanning(context.Context, domain.OutcomeID, domain.PlanningSessionID, int64) (outcomevc.PlanningView, error)
}

func (c *OutcomesController) planningService(w http.ResponseWriter, r *http.Request, method, path string) (interactivePlanningService, bool) {
	service, ok := c.Svc.(interactivePlanningService)
	if !ok {
		apispec.NotImplemented(w, r, method, path)
	}
	return service, ok
}

func (c *OutcomesController) planningCandidates(w http.ResponseWriter, r *http.Request) {
	service, ok := c.planningService(w, r, http.MethodGet, "/api/v1/outcomes/{outcomeId}/planning-candidates")
	if !ok {
		return
	}
	revision, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("contractRevision")), 10, 64)
	if err != nil || revision < 1 {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "bad_request", "EXPECTED_REVISION_REQUIRED", "State which Contract revision planning uses", nil)
		return
	}
	candidates, err := service.PlanningCandidates(r.Context(), outcomeID(r), revision)
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	response := make([]PlanningCandidateResponse, 0, len(candidates))
	for _, candidate := range candidates {
		response = append(response, planningCandidateResponse(candidate))
	}
	envelope.WriteJSON(w, http.StatusOK, PlanningCandidatesEnvelope{Candidates: response})
}

func (c *OutcomesController) startPlanning(w http.ResponseWriter, r *http.Request) {
	service, ok := c.planningService(w, r, http.MethodPost, "/api/v1/outcomes/{outcomeId}/planning-sessions")
	if !ok {
		return
	}
	var req StartPlanningRequest
	if !decodePlanningRequest(w, r, &req) {
		return
	}
	view, err := service.StartPlanning(r.Context(), outcomeID(r), outcomevc.StartPlanningInput{
		ExpectedContractRevision: req.ExpectedContractRevision, CandidateID: req.CandidateID,
		ContextMode: domain.PlanningContextMode(req.ContextMode), RequestKey: req.RequestKey,
	})
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusCreated, PlanningEnvelope{Planning: planningResponse(view)})
}

func (c *OutcomesController) currentPlanning(w http.ResponseWriter, r *http.Request) {
	service, ok := c.planningService(w, r, http.MethodGet, "/api/v1/outcomes/{outcomeId}/planning-session")
	if !ok {
		return
	}
	view, err := service.GetCurrentPlanning(r.Context(), outcomeID(r))
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, PlanningEnvelope{Planning: planningResponse(view)})
}

func (c *OutcomesController) getPlanning(w http.ResponseWriter, r *http.Request) {
	service, ok := c.planningService(w, r, http.MethodGet, "/api/v1/outcomes/{outcomeId}/planning-sessions/{planningSessionId}")
	if !ok {
		return
	}
	view, err := service.GetPlanning(r.Context(), outcomeID(r), planningSessionID(r))
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, PlanningEnvelope{Planning: planningResponse(view)})
}

func (c *OutcomesController) continuePlanning(w http.ResponseWriter, r *http.Request) {
	service, ok := c.planningService(w, r, http.MethodPost, "/api/v1/outcomes/{outcomeId}/planning-sessions/{planningSessionId}/messages")
	if !ok {
		return
	}
	var req PlanningMessageRequest
	if !decodePlanningRequest(w, r, &req) {
		return
	}
	view, err := service.ContinuePlanning(r.Context(), outcomeID(r), planningSessionID(r), outcomevc.PlanningMessageInput{ExpectedSessionRevision: req.ExpectedSessionRevision, Text: req.Text, RequestKey: req.RequestKey})
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, PlanningEnvelope{Planning: planningResponse(view)})
}

func (c *OutcomesController) finalizePlanning(w http.ResponseWriter, r *http.Request) {
	service, ok := c.planningService(w, r, http.MethodPost, "/api/v1/outcomes/{outcomeId}/planning-sessions/{planningSessionId}/proposal")
	if !ok {
		return
	}
	var req PlanningFinalizeRequest
	if !decodePlanningRequest(w, r, &req) {
		return
	}
	view, err := service.FinalizePlanning(r.Context(), outcomeID(r), planningSessionID(r), outcomevc.PlanningFinalizeInput{ExpectedSessionRevision: req.ExpectedSessionRevision, RequestKey: req.RequestKey})
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, PlanningEnvelope{Planning: planningResponse(view)})
}

func (c *OutcomesController) cancelPlanning(w http.ResponseWriter, r *http.Request) {
	service, ok := c.planningService(w, r, http.MethodPost, "/api/v1/outcomes/{outcomeId}/planning-sessions/{planningSessionId}/cancel")
	if !ok {
		return
	}
	var req PlanningCancelRequest
	if !decodePlanningRequest(w, r, &req) {
		return
	}
	view, err := service.CancelPlanning(r.Context(), outcomeID(r), planningSessionID(r), req.ExpectedSessionRevision)
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, PlanningEnvelope{Planning: planningResponse(view)})
}

func decodePlanningRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "bad_request", "INVALID_JSON", "Invalid JSON body", nil)
		return false
	}
	return true
}

func outcomeID(r *http.Request) domain.OutcomeID {
	return domain.OutcomeID(chi.URLParam(r, "outcomeId"))
}
func planningSessionID(r *http.Request) domain.PlanningSessionID {
	return domain.PlanningSessionID(chi.URLParam(r, "planningSessionId"))
}

func planningCandidateResponse(candidate ports.PlanningCandidate) PlanningCandidateResponse {
	return PlanningCandidateResponse{ID: candidate.ID, Binding: planningBindingResponse(candidate.Binding), Ready: candidate.Ready, UnavailableCode: candidate.UnavailableCode, UnavailableDetail: candidate.UnavailableDetail}
}

func planningBindingResponse(binding domain.PlanningBinding) PlanningBindingResponse {
	return PlanningBindingResponse{Mode: string(binding.Mode), Provider: string(binding.Provider), ModelSelection: string(binding.ModelSelection), Model: binding.Model, Effort: binding.Effort}
}

func planningResponse(view outcomevc.PlanningView) PlanningResponse {
	session := view.Session
	response := PlanningResponse{Session: PlanningSessionResponse{
		ID: session.ID.String(), OutcomeID: string(session.OutcomeID), ContractRevisionID: session.ContractRevisionID.String(),
		ContractRevisionNumber: session.ContractRevisionNumber, Revision: session.Revision, Status: string(session.Status), WaitingOn: string(session.WaitingOn),
		Binding: planningBindingResponse(session.Binding), ContextMode: string(session.ContextMode), ContextDigest: string(session.ContextDigest), PlanningGrantDigest: string(session.PlanningGrantDigest),
		EffectiveProvider: string(session.EffectiveProvider), EffectiveModel: session.EffectiveModel, LastFailureCode: session.LastFailureCode, LastFailureDetail: session.LastFailureDetail,
		CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt,
	}, Turns: make([]PlanningTurnResponse, 0, len(view.Turns))}
	for _, turn := range view.Turns {
		response.Turns = append(response.Turns, planningTurnResponse(turn))
	}
	if view.ProposedPlan != nil {
		plan := planRevisionResponse(*view.ProposedPlan)
		response.ProposedPlan = &plan
	}
	return response
}

func planningTurnResponse(turn domain.PlanningTurn) PlanningTurnResponse {
	response := PlanningTurnResponse{ID: string(turn.ID), Sequence: turn.Sequence, ReplyToTurnID: string(turn.ReplyToTurnID), Role: string(turn.Role), Kind: string(turn.Kind), Text: turn.Text, IntelligenceRunID: string(turn.IntelligenceRunID), CreatedAt: turn.CreatedAt}
	if turn.Role == domain.PlanningTurnPlanner && len(turn.StructuredPayload) > 0 {
		// The planner turn payload is the evaluated readiness reply: the
		// canonical packet plus the fence it was minted under. A needs_context
		// packet adapts to the owner-decision card from its message and first
		// owner-routed issue; ready packets surface through ProposedPlan, and
		// blocked packets carry their issues in the packet (S3: one path, no
		// legacy clarification/contract-change payload).
		if reply, ok := domain.DecodePlanningEvaluatedReply(turn.StructuredPayload); ok && reply.Result.Status == domain.PlanningNeedsContext {
			card := &PlanningClarificationResponse{Question: reply.Result.Message}
			if len(reply.Result.Issues) > 0 {
				issue := reply.Result.Issues[0]
				card.Reason = issue.Reason
				card.Recommendation = issue.Recommendation
				for _, choice := range issue.Choices {
					card.Alternatives = append(card.Alternatives, choice.Label)
				}
			}
			response.Clarification = card
		}
	}
	return response
}
