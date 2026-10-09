package store

import (
	"context"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// AutomationInputBlocked reads the same branch-filtered pending activities the
// owner sees. A normal instruction is never interpreted as a permission answer.
func (s *Store) AutomationInputBlocked(ctx context.Context, conversation string) (bool, error) {
	rows, err := s.conversationReader(ctx).SelectConversationActivities(ctx, conversation)
	if err != nil {
		return true, err
	}
	for _, row := range rows {
		if row.Kind == domain.ActivityKindApproval && row.Status == domain.ActivityStatusPending {
			return true, nil
		}
	}
	return false, nil
}
