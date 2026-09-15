package httpd

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/envelope"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ownercommand"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type replacementDecisionRequest struct {
	OutcomeID              domain.OutcomeID      `json:"outcomeId"`
	PredecessorAttemptID   domain.AttemptID      `json:"predecessorAttemptId"`
	PlanRevisionID         domain.PlanRevisionID `json:"planRevisionId"`
	WorkUnitID             domain.WorkUnitID     `json:"workUnitId"`
	RunIntentGeneration    int64                 `json:"runIntentGeneration"`
	ContractRevisionNumber int64                 `json:"contractRevisionNumber"`
	Action                 string                `json:"action"`
	RequestKey             string                `json:"requestKey"`
}
type replacementDecisionFingerprint struct {
	OwnerPrincipal string `json:"ownerPrincipal"`
	replacementDecisionRequest
}

func mountOwnerCommands(r chi.Router, authority *ownercommand.Authority, store ports.AttemptReplacementDecisionStore) {
	if authority == nil || store == nil {
		return
	}
	r.Post("/internal/owner-commands/attempt-replacement-decisions", func(w http.ResponseWriter, req *http.Request) {
		if !localControlRequest(req) {
			notFoundJSON(w, req)
			return
		}
		principal, ok := authority.Authenticate(req.Header.Get("Authorization"))
		if !ok {
			envelope.WriteJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]any{"code": "OWNER_COMMAND_UNAUTHORIZED", "message": "Trusted local-owner command authentication failed"}})
			return
		}
		var in replacementDecisionRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, req.Body, 16<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil {
			envelope.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "OWNER_COMMAND_INVALID", "message": "Replacement decision body is invalid"}})
			return
		}
		if dec.Decode(&struct{}{}) != io.EOF {
			envelope.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "OWNER_COMMAND_INVALID", "message": "Replacement decision body must contain exactly one JSON object"}})
			return
		}
		if strings.TrimSpace(in.Action) != "replace" {
			envelope.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "OWNER_COMMAND_INVALID", "message": "Replacement decision action must be replace"}})
			return
		}
		fp, err := ownercommand.Fingerprint(replacementDecisionFingerprint{OwnerPrincipal: principal, replacementDecisionRequest: in})
		if err != nil {
			envelope.WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"code": "OWNER_COMMAND_FAILED", "message": "Replacement decision could not be bound"}})
			return
		}
		d := domain.AttemptReplacementDecision{ID: domain.AttemptReplacementDecisionID("replacement-decision-" + uuid.NewString()), OutcomeID: in.OutcomeID, PredecessorAttemptID: in.PredecessorAttemptID, PlanRevisionID: in.PlanRevisionID, WorkUnitID: in.WorkUnitID, RunIntentGeneration: in.RunIntentGeneration, ContractRevisionNumber: in.ContractRevisionNumber, Action: "replace", RequestKey: strings.TrimSpace(in.RequestKey), RequestFingerprint: fp, OwnerPrincipal: principal, CreatedAt: time.Now().UTC()}
		if err := d.Validate(); err != nil {
			envelope.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "OWNER_COMMAND_INVALID", "message": "Replacement decision body is invalid"}})
			return
		}
		stored, created, err := store.CreateAttemptReplacementDecision(req.Context(), d)
		if err != nil {
			status, code, message := http.StatusInternalServerError, "OWNER_COMMAND_FAILED", "Replacement decision could not be stored"
			switch err.(type) {
			case *ports.AttemptReplacementDecisionConflictError:
				status, code, message = http.StatusConflict, "OWNER_COMMAND_CONFLICT", "The request key already names a different replacement decision"
			case *ports.AttemptReplacementDecisionBindingError:
				status, code, message = http.StatusConflict, "OWNER_COMMAND_STALE", "The replacement decision no longer matches current Outcome authority"
			}
			envelope.WriteJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message}})
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		envelope.WriteJSON(w, status, map[string]any{"data": map[string]any{"decision": stored, "created": created}})
	})
}
