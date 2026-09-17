package ports

import (
	"context"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

type HarnessAuthorityStore interface {
	CreateHarnessPairingIntent(context.Context, domain.HarnessPairingIntent) (domain.HarnessPairingIntent, bool, error)
	GetHarnessPairingIntent(context.Context, domain.PairingChallengeID) (domain.HarnessPairingIntent, bool, error)
	ListHarnessPairingIntents(context.Context, domain.ProjectID, int) ([]domain.HarnessPairingIntent, error)
	DecideHarnessPairingIntent(context.Context, domain.PairingChallengeID, domain.SHA256Digest, string, domain.HarnessAuthorityReceipt, time.Time) (domain.HarnessPairingIntent, bool, error)
	ListHarnessConnections(context.Context, string, int) ([]domain.HarnessConnection, error)
	RevokeHarnessConnectionWithReceipt(context.Context, domain.HarnessConnectionID, domain.SHA256Digest, int64, domain.HarnessAuthorityReceipt, time.Time) (domain.HarnessConnection, bool, int64, error)
	ListHarnessAuthorityReceipts(context.Context, string, string) ([]domain.HarnessAuthorityReceipt, error)
	CountHarnessCommandConsequences(context.Context, domain.HarnessConnectionID, int64) (int64, error)
}

type HarnessPairingActivator interface {
	ActivateHarnessPairingIntent(context.Context, domain.PairingChallengeID, domain.SHA256Digest, time.Time, func(domain.HarnessPairingIntent) (domain.HarnessPairingChallenge, domain.PairingChallengeSecret, error)) (domain.HarnessPairingIntent, domain.HarnessPairingChallenge, domain.PairingChallengeSecret, error)
}
