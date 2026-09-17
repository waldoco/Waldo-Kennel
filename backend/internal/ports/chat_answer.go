package ports

import (
	"context"
	"errors"
)

// ChatAnswerWriteOutcome describes the strongest boundary an adapter can prove
// for a reply to a provider-initiated request. A reply has no reply of its own,
// so this must never be modeled as provider acknowledgement or rejection.
type ChatAnswerWriteOutcome string

const (
	ChatAnswerWriteNotStarted    ChatAnswerWriteOutcome = "not_started"
	ChatAnswerSDKHandoffComplete ChatAnswerWriteOutcome = "sdk_handoff_complete"
	ChatAnswerFrameWriteComplete ChatAnswerWriteOutcome = "frame_write_complete"
	ChatAnswerWriteUnknown       ChatAnswerWriteOutcome = "write_unknown"
)

func (o ChatAnswerWriteOutcome) Valid() bool {
	switch o {
	case ChatAnswerWriteNotStarted, ChatAnswerSDKHandoffComplete, ChatAnswerFrameWriteComplete, ChatAnswerWriteUnknown:
		return true
	default:
		return false
	}
}

// ChatAnswerDispatch is the transport result for answering one exact local
// request-card instance. RequestInstanceID has no provider wire counterpart.
type ChatAnswerDispatch struct {
	WriteOutcome ChatAnswerWriteOutcome
}

func (d ChatAnswerDispatch) Validate() error {
	if !d.WriteOutcome.Valid() {
		return errors.New("chat answer dispatch write outcome is invalid")
	}
	return nil
}

// ChatAnswerDispatcher is the governed answer seam. Implementations report
// only the write/handoff boundary they can observe, never provider acceptance.
type ChatAnswerDispatcher interface {
	DispatchAnswer(ctx context.Context, requestID, requestInstanceID string, decision ChatDecision) (ChatAnswerDispatch, error)
}

// ChatInputDispatcher is the governed typed-input seam. Like approval answers,
// a successful handoff is the strongest observable boundary and is never
// mislabeled as provider acknowledgement.
type ChatInputDispatcher interface {
	DispatchInput(ctx context.Context, requestID, requestInstanceID string, response ChatInputResponse) (ChatAnswerDispatch, error)
}
