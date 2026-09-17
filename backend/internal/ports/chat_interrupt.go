package ports

import (
	"context"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// ChatInterruptDispatch reports transport acceptance separately from Stage 1's
// process-tree stop boundary. Provider acknowledgement alone is not quiescence.
type ChatInterruptDispatch struct {
	Acceptance            ChatTurnAcceptance
	TransportRequestID    int64
	TransportSHA256       string
	TransportBytes        int
	TransportSequence     int64
	Quiescence            domain.GovernedCommandQuiescence
	QuiescenceEvidenceRef string
}

// ChatInterruptDispatcher is the governed Stop seam. The named turn is a
// precondition; implementations must not redirect Stop to a successor turn.
type ChatInterruptDispatcher interface {
	DispatchInterrupt(context.Context, string) (ChatInterruptDispatch, error)
}
