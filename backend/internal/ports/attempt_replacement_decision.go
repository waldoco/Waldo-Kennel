package ports

import (
	"context"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

type AttemptReplacementDecisionStore interface {
	CreateAttemptReplacementDecision(context.Context, domain.AttemptReplacementDecision) (domain.AttemptReplacementDecision, bool, error)
	GetAttemptReplacementDecision(context.Context, domain.AttemptReplacementDecisionID) (domain.AttemptReplacementDecision, bool, error)
}

type AttemptReplacementDecisionConflictError struct {
	Existing domain.AttemptReplacementDecision
}

func (e *AttemptReplacementDecisionConflictError) Error() string {
	return "replacement decision request key already names different semantics"
}

type AttemptReplacementDecisionBindingError struct{ Reason string }

func (e *AttemptReplacementDecisionBindingError) Error() string {
	return "replacement decision binding is not current"
}
