package outcome

import (
	"context"
	"fmt"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// AttemptStartReservationService is the merge seam for start-time capability
// escalation. It deliberately accepts an evaluator rather than duplicating
// admission.go and never constructs sessions, workspaces, fences, or launch facts.
type AttemptStartReservationService struct {
	Store     ports.AttemptStartReservationStore
	Evaluator ports.AdmissionStageEvaluator
}

func (s AttemptStartReservationService) ReserveRefusal(ctx context.Context, r domain.AttemptStartReservation, attach ports.AttemptStartEscalationCreator) (domain.AttemptStartReservation, bool, error) {
	if s.Store == nil {
		return domain.AttemptStartReservation{}, false, fmt.Errorf("attempt start reservation store is required")
	}
	return s.Store.ReserveAttemptStart(ctx, ports.AttemptStartReservationRequest{Reservation: r}, attach)
}

// FreshStartEvaluation evaluates the Start stage through the injected canonical
// evaluator. Callers must supply a newly read RoutingSnapshot; no stored proof
// is accepted as an implicit default.
func (s AttemptStartReservationService) FreshStartEvaluation(ctx context.Context, in ports.AdmissionStageInput) (ports.AdmissionStageResult, error) {
	if s.Evaluator == nil {
		return ports.AdmissionStageResult{}, fmt.Errorf("admission evaluator is required")
	}
	if in.Stage != ports.AdmissionStageStart || in.RoutingSnapshot == nil {
		return ports.AdmissionStageResult{}, fmt.Errorf("grant-once resume requires Start and a fresh routing snapshot")
	}
	return s.Evaluator.EvaluateAdmissionStage(ctx, in)
}
