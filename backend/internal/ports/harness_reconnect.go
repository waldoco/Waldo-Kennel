package ports

import (
	"context"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

type HarnessDaemonAttacher interface {
	AttachHarnessDaemon(context.Context) (domain.HarnessDaemonReadiness, error)
}
type HarnessMissionReader interface {
	HarnessMissionExists(context.Context, string) (bool, error)
}
type HarnessMissionProfileReader interface {
	ReadHarnessMissionProfile(context.Context, string) (domain.HarnessMissionProfile, bool, error)
}
type HarnessDeliveryBlockReader interface {
	ReadHarnessDeliveryBlock(context.Context, string) (domain.HarnessDeliveryBlock, bool, error)
}

// HarnessPairingFactsReader is the reviewed S3.3 seam. It reports authenticated,
// fresh facts; reconnect must not reconstruct them from discovery or process state.
type HarnessPairingFactsReader interface {
	ReadHarnessPairingFacts(context.Context, domain.HarnessConnectionID) (domain.HarnessPairingObservation, error)
}
type UnavailableHarnessPairingFacts struct{}

func (UnavailableHarnessPairingFacts) ReadHarnessPairingFacts(context.Context, domain.HarnessConnectionID) (domain.HarnessPairingObservation, error) {
	return domain.HarnessPairingObservation{}, domain.ErrHarnessPairingFactsUnavailable
}
