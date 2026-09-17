package harnessrelease

import (
	"context"
	"errors"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"runtime"
	"sort"
	"strings"
	"time"
)

type Request struct {
	AdapterID, OS, Architecture string
	MinimumSequence             uint64
	Now                         time.Time
}
type Resolver struct {
	Trust ports.HarnessReleaseTrustVerifier
}

func (r Resolver) Resolve(ctx context.Context, envelope []byte, req Request) (domain.TrustedHarnessAdapterRelease, error) {
	if err := ctx.Err(); err != nil {
		return domain.TrustedHarnessAdapterRelease{}, err
	}
	if strings.TrimSpace(req.AdapterID) == "" || req.Now.IsZero() {
		return domain.TrustedHarnessAdapterRelease{}, domain.ErrHarnessAdapterReleaseIndexInvalid
	}
	if req.OS == "" {
		req.OS = runtime.GOOS
	}
	if req.Architecture == "" {
		req.Architecture = runtime.GOARCH
	}
	if r.Trust == nil {
		r.Trust = ports.UnavailableHarnessReleaseTrustVerifier{}
	}
	idx, err := r.Trust.VerifyHarnessReleaseIndex(ctx, envelope)
	if err != nil {
		return domain.TrustedHarnessAdapterRelease{}, err
	}
	if err = idx.Validate(req.Now); err != nil {
		return domain.TrustedHarnessAdapterRelease{}, err
	}
	matches := make([]domain.TrustedHarnessAdapterRelease, 0)
	for _, release := range idx.Releases {
		if release.AdapterID == req.AdapterID && release.OS == req.OS && release.Architecture == req.Architecture && release.Sequence > req.MinimumSequence {
			matches = append(matches, release)
		}
	}
	if len(matches) == 0 {
		return domain.TrustedHarnessAdapterRelease{}, domain.ErrHarnessAdapterReleaseNotFound
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Sequence > matches[j].Sequence })
	if len(matches) > 1 && matches[0].Sequence == matches[1].Sequence {
		return domain.TrustedHarnessAdapterRelease{}, domain.ErrHarnessAdapterReleaseIndexInvalid
	}
	return matches[0], nil
}

var _ = errors.Is
