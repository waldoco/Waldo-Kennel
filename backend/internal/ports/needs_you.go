package ports

import (
	"context"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

type NeedsYouStore interface {
	ListCurrentNeedsYouQuestions(context.Context, domain.OutcomeID) ([]domain.NeedsYouQuestion, error)
	GetNeedsYouQuestion(context.Context, domain.OutcomeID, string) (domain.NeedsYouQuestion, bool, error)
	ReconcileNeedsYouAnswer(context.Context, domain.OutcomeID, string, string) (domain.NeedsYouQuestion, error)
}
