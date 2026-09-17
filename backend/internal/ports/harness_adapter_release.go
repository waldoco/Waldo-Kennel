package ports

import (
	"context"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// HarnessReleaseTrustVerifier owns signature/transparency/bundled-root policy.
// Resolve code may consume only an index returned by this verifier.
type HarnessReleaseTrustVerifier interface {
	VerifyHarnessReleaseIndex(context.Context, []byte) (domain.HarnessAdapterReleaseIndex, error)
}
type UnavailableHarnessReleaseTrustVerifier struct{}

func (UnavailableHarnessReleaseTrustVerifier) VerifyHarnessReleaseIndex(context.Context, []byte) (domain.HarnessAdapterReleaseIndex, error) {
	return domain.HarnessAdapterReleaseIndex{}, domain.ErrHarnessAdapterReleaseTrustUnavailable
}
