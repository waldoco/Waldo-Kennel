package domain

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

type HarnessAdapterReleaseIndex struct {
	TrustRootID string                         `json:"trust_root_id"`
	Sequence    uint64                         `json:"sequence"`
	ExpiresAt   time.Time                      `json:"expires_at"`
	Releases    []TrustedHarnessAdapterRelease `json:"releases"`
	Digest      SHA256Digest                   `json:"digest"`
}

func (i HarnessAdapterReleaseIndex) ContentDigest() (SHA256Digest, error) {
	type unsigned HarnessAdapterReleaseIndex
	u := unsigned(i)
	u.Digest = ""
	u.Releases = append([]TrustedHarnessAdapterRelease(nil), i.Releases...)
	sort.Slice(u.Releases, func(a, b int) bool {
		x, y := u.Releases[a], u.Releases[b]
		if x.AdapterID != y.AdapterID {
			return x.AdapterID < y.AdapterID
		}
		if x.OS != y.OS {
			return x.OS < y.OS
		}
		if x.Architecture != y.Architecture {
			return x.Architecture < y.Architecture
		}
		return x.Sequence < y.Sequence
	})
	b, e := json.Marshal(u)
	if e != nil {
		return "", e
	}
	return DigestSHA256(b), nil
}
func (i HarnessAdapterReleaseIndex) Validate(now time.Time) error {
	if strings.TrimSpace(i.TrustRootID) == "" || i.Sequence == 0 || i.ExpiresAt.IsZero() || !now.Before(i.ExpiresAt) || len(i.Releases) == 0 || !i.Digest.Valid() {
		return ErrHarnessAdapterReleaseIndexInvalid
	}
	want, e := i.ContentDigest()
	if e != nil || want != i.Digest {
		return ErrHarnessAdapterReleaseIndexInvalid
	}
	seen := map[string]bool{}
	for _, r := range i.Releases {
		if r.TrustRootID != i.TrustRootID || r.Sequence > i.Sequence || r.ExpiresAt.After(i.ExpiresAt) || r.Validate(now) != nil {
			return ErrHarnessAdapterReleaseIndexInvalid
		}
		key := r.AdapterID + "\x00" + r.OS + "\x00" + r.Architecture + "\x00" + r.Version
		if seen[key] {
			return ErrHarnessAdapterReleaseIndexInvalid
		}
		seen[key] = true
	}
	return nil
}

var (
	ErrHarnessAdapterReleaseIndexInvalid     = errors.New("invalid trusted adapter release index")
	ErrHarnessAdapterReleaseTrustUnavailable = errors.New("adapter release trust verification is unavailable")
	ErrHarnessAdapterReleaseNotFound         = errors.New("trusted adapter release not found")
)
