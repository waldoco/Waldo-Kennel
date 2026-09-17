package outcome

import (
	"context"
	"errors"
	"fmt"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"strings"
)

var (
	ErrNeedsYouUnavailable   = errors.New("needs-you projection unavailable")
	ErrNeedsYouNotFound      = errors.New("needs-you question not found")
	ErrNeedsYouStale         = errors.New("needs-you question is stale")
	ErrNeedsYouAnswerInvalid = errors.New("needs-you answer is invalid")
)

type NeedsYouDispatcher interface {
	ResolveWithKey(context.Context, domain.SessionID, string, string, ports.ChatDecision) error
	ResolveInputWithKey(context.Context, domain.SessionID, string, string, ports.ChatInputResponse) error
}

func (s *Service) WithNeedsYou(store ports.NeedsYouStore, dispatcher NeedsYouDispatcher) *Service {
	s.needsYou = store
	s.needsYouDispatcher = dispatcher
	return s
}
func (s *Service) CurrentNeedsYou(ctx context.Context, id domain.OutcomeID) ([]domain.NeedsYouQuestion, error) {
	if s.needsYou == nil {
		return nil, ErrNeedsYouUnavailable
	}
	return s.needsYou.ListCurrentNeedsYouQuestions(ctx, id)
}
func (s *Service) AnswerNeedsYou(ctx context.Context, outcomeID domain.OutcomeID, questionID string, in domain.NeedsYouAnswer) (domain.NeedsYouQuestion, error) {
	if s.needsYou == nil || s.needsYouDispatcher == nil {
		return domain.NeedsYouQuestion{}, ErrNeedsYouUnavailable
	}
	q, ok, err := s.needsYou.GetNeedsYouQuestion(ctx, outcomeID, questionID)
	if err != nil {
		return q, err
	}
	if !ok {
		return q, ErrNeedsYouNotFound
	}
	if in.Generation != q.Generation {
		return q, ErrNeedsYouStale
	}
	if q.Status == domain.NeedsYouSuperseded {
		return q, ErrNeedsYouStale
	}
	if strings.TrimSpace(in.RequestKey) == "" {
		return q, ErrNeedsYouAnswerInvalid
	}
	// The controller's key is the immutable generation. Keep requestKey at this facade as a required client retry identity;
	// the underlying claim fingerprint still rejects a different payload for the same generation.
	switch q.Kind {
	case domain.NeedsYouApproval, domain.NeedsYouChoice:
		if in.Decision == nil || in.Input != nil || strings.TrimSpace(in.Decision.ID) == "" {
			return q, ErrNeedsYouAnswerInvalid
		}
		allowed := false
		for _, o := range q.Options {
			if o.ID == in.Decision.ID {
				allowed = true
				break
			}
		}
		if !allowed {
			return q, ErrNeedsYouAnswerInvalid
		}
		err = s.needsYouDispatcher.ResolveWithKey(ctx, q.SessionID, q.RequestID, in.RequestKey, ports.ChatDecision{ID: in.Decision.ID, Raw: in.Decision.Raw})
	case domain.NeedsYouInput:
		if in.Input == nil || in.Decision != nil {
			return q, ErrNeedsYouAnswerInvalid
		}
		action := ports.ChatInputAction(in.Input.Action)
		if !action.Valid() || (action != ports.ChatInputActionAccept && len(in.Input.Content) > 0) {
			return q, ErrNeedsYouAnswerInvalid
		}
		err = s.needsYouDispatcher.ResolveInputWithKey(ctx, q.SessionID, q.RequestID, in.RequestKey, ports.ChatInputResponse{Action: action, Content: in.Input.Content})
	default:
		return q, ErrNeedsYouAnswerInvalid
	}
	latest, found, readErr := s.needsYou.GetNeedsYouQuestion(ctx, outcomeID, questionID)
	if readErr == nil && found {
		q = latest
	}
	if err != nil {
		return q, err
	}
	if readErr != nil {
		return q, fmt.Errorf("read answered needs-you question: %w", readErr)
	}
	return q, nil
}
func (s *Service) ReconcileNeedsYou(ctx context.Context, outcomeID domain.OutcomeID, questionID, generation string) (domain.NeedsYouQuestion, error) {
	if s.needsYou == nil {
		return domain.NeedsYouQuestion{}, ErrNeedsYouUnavailable
	}
	return s.needsYou.ReconcileNeedsYouAnswer(ctx, outcomeID, questionID, generation)
}
