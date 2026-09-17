package ports

import (
	"context"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"time"
)

type OwnerProofStore interface {
	CreateOwnerProof(context.Context, domain.OwnerProof) (domain.OwnerProof, bool, error)
	GetOwnerProof(context.Context, domain.OwnerProofID) (domain.OwnerProof, bool, error)
	ConsumeOwnerProof(context.Context, domain.OwnerProofID, time.Time) (domain.OwnerProof, bool, error)
}
