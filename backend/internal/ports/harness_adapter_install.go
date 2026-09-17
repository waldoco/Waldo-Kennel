package ports

import (
	"context"
	"errors"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// HarnessAdapterQuiescer fences calls to the active generation before pointer activation.
type HarnessAdapterQuiescer interface {
	QuiesceHarnessAdapter(context.Context, domain.SHA256Digest) error
}

// HarnessAdapterHealthProbe is metadata/capability-only. Implementations must not
// submit owner content or production effects.
type HarnessAdapterHealthProbe interface {
	ProbeHarnessAdapter(context.Context, string) (HarnessInstallation, error)
}

// HarnessAdapterPairingActivator is the explicit S3.3 seam. Until S3.3 lands,
// production wiring must use UnavailableHarnessAdapterPairing and fail closed.
type HarnessAdapterPairingActivator interface {
	ActivateHarnessAdapterPairing(context.Context, HarnessInstallation) error
}
type UnavailableHarnessAdapterPairing struct{}

func (UnavailableHarnessAdapterPairing) ActivateHarnessAdapterPairing(context.Context, HarnessInstallation) error {
	return domain.ErrHarnessAdapterPairingUnavailable
}

var ErrHarnessAdapterNotQuiescent = errors.New("harness adapter is not quiescent")
