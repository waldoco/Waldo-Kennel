package ports

import (
	"context"
	"errors"
)

// ChatAnswerDispatch is the acceptance result for answering one exact provider
// request generation. TargetGeneration is Kennel's durable identity for the
// pending request, not the provider's reusable request id.
type ChatAnswerDispatch struct {
	Acceptance ChatTurnAcceptance
}

func (d ChatAnswerDispatch) Validate() error {
	if !d.Acceptance.Valid() {
		return errors.New("chat answer dispatch acceptance is invalid")
	}
	return nil
}

// ChatAnswerDispatcher is the governed answer seam. Implementations must not
// report acknowledged until the answer crossed their provider boundary.
type ChatAnswerDispatcher interface {
	DispatchAnswer(ctx context.Context, requestID, targetGeneration string, decision ChatDecision) (ChatAnswerDispatch, error)
}
