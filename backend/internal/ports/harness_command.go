package ports

import (
	"context"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"time"
)

type HarnessCommandRequest struct {
	ConnectionBearer  string
	ConnectionBinding domain.HarnessConnectionBinding
	OwnerProofID      domain.OwnerProofID
	OwnerProofBearer  string
	Target            domain.OwnerProofTarget
	Command           domain.CanonicalHarnessCommand
	AdapterRequestKey string
	Now               time.Time
}
type HarnessCommandStore interface {
	ValidateAuthoritiesAndCreateCommandClaim(context.Context, HarnessCommandRequest) (domain.CommandAuthorityClaim, bool, error)
	ListPendingHarnessCommandOutbox(context.Context) ([]domain.HarnessCommandOutboxRecord, error)
}
