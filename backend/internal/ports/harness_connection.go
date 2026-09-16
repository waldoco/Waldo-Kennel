package ports

import (
	"context"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"time"
)

type HarnessConnectionStore interface {
	CreateHarnessConnection(context.Context, domain.HarnessConnection) (domain.HarnessConnection, bool, error)
	GetHarnessConnection(context.Context, domain.HarnessConnectionID) (domain.HarnessConnection, bool, error)
	RotateHarnessConnection(context.Context, domain.HarnessConnectionID, int64, string, time.Time, time.Time) (domain.HarnessConnection, bool, error)
	RevokeHarnessConnection(context.Context, domain.HarnessConnectionID, int64, time.Time) (domain.HarnessConnection, bool, error)
}
