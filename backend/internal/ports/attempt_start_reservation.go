package ports

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

type AttemptStartReservationRequest struct {
	Reservation domain.AttemptStartReservation
}
type AttemptStartEscalationAttachment struct {
	ReservationID         string
	AttemptID             domain.AttemptID
	AdmissionEvaluationID string
	Denial                domain.CapabilityDenialDetail
}

// AttemptStartReservationTx is the narrow transaction-aware seam lane B must
// implement against. SQLTx never owns commit or rollback. Lane B must build its
// query writer with Queries.WithTx(SQLTx()) and insert every question/escalation
// row through it. Calling an ordinary service/store method here would re-take
// writeMu or open another transaction, causing a deadlock or a two-commit gap.
type AttemptStartReservationTx interface{ SQLTx() *sql.Tx }
type AttemptStartEscalationCreator interface {
	CreateInAttemptStartTransaction(context.Context, AttemptStartReservationTx, AttemptStartEscalationAttachment) error
}
type AttemptStartReservationStore interface {
	ReserveAttemptStart(context.Context, AttemptStartReservationRequest, AttemptStartEscalationCreator) (domain.AttemptStartReservation, bool, error)
	FindAttemptStartReservation(context.Context, string) (domain.AttemptStartReservation, bool, error)
}
type AttemptStartReplayConflictError struct {
	Existing domain.AttemptStartReservation
}

func (e *AttemptStartReplayConflictError) Error() string {
	return fmt.Sprintf("attempt start request key conflicts with reservation %s", e.Existing.ID)
}
