package outcome

import (
	"context"
	"fmt"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

func (s *Service) WithCapabilityEscalations(store ports.CapabilityEscalationStore) *Service {
	s.capabilityEscalations = store
	return s
}
func (s *Service) CreateCapabilityEscalation(ctx context.Context, e domain.CapabilityEscalation) (domain.NeedsYouQuestion, bool, error) {
	if s.capabilityEscalations == nil {
		return domain.NeedsYouQuestion{}, false, ErrNeedsYouUnavailable
	}
	return s.capabilityEscalations.CreateCapabilityEscalation(ctx, e)
}
func (s *Service) answerCapabilityEscalation(ctx context.Context, q domain.NeedsYouQuestion, in domain.NeedsYouAnswer) (domain.NeedsYouQuestion, error) {
	if s.capabilityEscalations == nil || q.CapabilityEscalation == nil {
		return q, ErrNeedsYouUnavailable
	}
	if in.Decision == nil {
		return q, ErrNeedsYouAnswerInvalid
	}
	_, _, err := s.capabilityEscalations.ApplyCapabilityEscalationAnswer(ctx, q.OutcomeID, q.ID, q.Generation, in.Decision.ID, in.RequestKey)
	if err != nil {
		return q, fmt.Errorf("apply capability escalation answer: %w", err)
	}
	latest, ok, readErr := s.needsYou.GetNeedsYouQuestion(ctx, q.OutcomeID, q.ID)
	if readErr == nil && ok {
		return latest, nil
	}
	return q, readErr
}
func (s *Service) ConsumeCapabilityGrantOnce(ctx context.Context, e domain.CapabilityEscalation, consumerFingerprint string) (domain.CapabilityEscalationReceipt, bool, error) {
	if s.capabilityEscalations == nil {
		return domain.CapabilityEscalationReceipt{}, false, ErrNeedsYouUnavailable
	}
	return s.capabilityEscalations.ConsumeCapabilityGrantOnce(ctx, e, consumerFingerprint)
}
