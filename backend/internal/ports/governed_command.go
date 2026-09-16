package ports

import (
	"context"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// GovernedCommandStore owns durable pre-effect claims and fenced delivery-state
// transitions. A false transition result means the caller lost at least one
// state/generation/revision/capability fence and no row was changed.
type GovernedCommandStore interface {
	CreateGovernedCommandClaim(context.Context, domain.GovernedCommandRecord) (domain.GovernedCommandRecord, bool, error)
	GetGovernedCommand(context.Context, string) (domain.GovernedCommandRecord, bool, error)
	ListUnsettledGovernedCommands(context.Context) ([]domain.GovernedCommandRecord, error)
	AdvanceGovernedCommand(context.Context, domain.GovernedCommandRecord, domain.GovernedCommandState, string, string, string) (bool, error)
}
