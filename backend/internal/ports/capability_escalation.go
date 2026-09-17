package ports

import (
	"context"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

type CapabilityEscalationStore interface {
	LatestAttemptSessionRefForSession(context.Context, string) (domain.AttemptSessionRef, bool, error)
	GetAttempt(context.Context, domain.OutcomeID, domain.AttemptID) (domain.Attempt, bool, error)
	GetSession(context.Context, domain.SessionID) (domain.SessionRecord, bool, error)
	ListContractRevisions(context.Context, domain.OutcomeID) ([]domain.ContractRevision, error)
	CreateCapabilityEscalation(context.Context, domain.CapabilityEscalation) (domain.NeedsYouQuestion, bool, error)
	ApplyCapabilityEscalationAnswer(context.Context, domain.OutcomeID, string, string, string, string) (domain.CapabilityEscalationReceipt, bool, error)
	ConsumeCapabilityGrantOnce(context.Context, domain.CapabilityEscalation, string) (domain.CapabilityEscalationReceipt, bool, error)
}
