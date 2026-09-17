package domain

import (
	"strings"
	"time"
)

type OwnerAnswerQuestion struct {
	ID, ConversationID, RequestID, Generation, Status string
	CreatedAt, UpdatedAt                              time.Time
}

func (q OwnerAnswerQuestion) Validate() error {
	if strings.TrimSpace(q.ID) == "" || strings.TrimSpace(q.ConversationID) == "" || strings.TrimSpace(q.RequestID) == "" || strings.TrimSpace(q.Generation) == "" || (q.Status != "pending" && q.Status != "resolved" && q.Status != "failed") || q.CreatedAt.IsZero() || q.UpdatedAt.Before(q.CreatedAt) {
		return ErrOwnerProofInvalid
	}
	return nil
}
