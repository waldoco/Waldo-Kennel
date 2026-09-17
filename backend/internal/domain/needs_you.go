package domain

import (
	"encoding/json"
	"time"
)

type NeedsYouKind string

const (
	NeedsYouApproval NeedsYouKind = "approval"
	NeedsYouChoice   NeedsYouKind = "choice"
	NeedsYouInput    NeedsYouKind = "input"
)

type NeedsYouStatus string

const (
	NeedsYouOpen            NeedsYouStatus = "open"
	NeedsYouAnswerQueued    NeedsYouStatus = "answer_queued"
	NeedsYouAnswerSent      NeedsYouStatus = "answer_sent"
	NeedsYouAcknowledged    NeedsYouStatus = "acknowledged"
	NeedsYouRefused         NeedsYouStatus = "refused"
	NeedsYouDeliveryUnknown NeedsYouStatus = "delivery_unknown"
	NeedsYouSuperseded      NeedsYouStatus = "superseded"
)

type NeedsYouOption struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
}

type NeedsYouQuestion struct {
	ID                   string                `json:"id"`
	OutcomeID            OutcomeID             `json:"outcomeId"`
	PlanRevisionID       PlanRevisionID        `json:"planRevisionId"`
	WorkUnitID           WorkUnitID            `json:"workUnitId"`
	AttemptID            AttemptID             `json:"attemptId"`
	SessionID            SessionID             `json:"sessionId"`
	ConversationID       string                `json:"conversationId"`
	RequestID            string                `json:"requestId"`
	Generation           string                `json:"generation"`
	Kind                 NeedsYouKind          `json:"kind"`
	Reason               string                `json:"reason"`
	Recommendation       string                `json:"recommendation,omitempty"`
	Options              []NeedsYouOption      `json:"options,omitempty"`
	InputMode            string                `json:"inputMode,omitempty"`
	InputSchema          map[string]any        `json:"inputSchema,omitempty"`
	URL                  string                `json:"url,omitempty"`
	Status               NeedsYouStatus        `json:"status"`
	CommandID            string                `json:"commandId,omitempty"`
	CreatedAt            time.Time             `json:"createdAt"`
	UpdatedAt            time.Time             `json:"updatedAt"`
	QuestionStatus       string                `json:"-"`
	ActivityStatus       ActivityStatus        `json:"-"`
	CommandState         GovernedCommandState  `json:"-"`
	CapabilityEscalation *CapabilityEscalation `json:"capabilityEscalation,omitempty"`
}

type NeedsYouAnswer struct {
	RequestKey string              `json:"requestKey"`
	Generation string              `json:"generation"`
	Decision   *ChatDecisionAnswer `json:"decision,omitempty"`
	Input      *ChatInputAnswer    `json:"input,omitempty"`
}
type ChatDecisionAnswer struct {
	ID  string          `json:"id"`
	Raw json.RawMessage `json:"raw,omitempty"`
}
type ChatInputAnswer struct {
	Action  string         `json:"action"`
	Content map[string]any `json:"content,omitempty"`
}
