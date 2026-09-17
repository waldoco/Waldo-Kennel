package ports

import (
	"context"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

type GovernedControlCommandStore interface {
	CreateGovernedControlCommandClaim(context.Context, domain.GovernedControlCommand) (domain.GovernedControlCommand, bool, error)
	GetGovernedControlCommand(context.Context, string) (domain.GovernedControlCommand, bool, error)
	AdoptClaimedGovernedControlCommandGeneration(context.Context, domain.GovernedControlCommand, string, time.Time) (bool, error)
	ListUnsettledGovernedControlCommands(context.Context) ([]domain.GovernedControlCommand, error)
	AdvanceGovernedControlCommand(context.Context, domain.GovernedControlCommand, domain.GovernedCommandState, string, string, string) (bool, error)
}
