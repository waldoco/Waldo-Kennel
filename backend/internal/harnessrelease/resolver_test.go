package harnessrelease

import (
	"context"
	"errors"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"runtime"
	"testing"
	"time"
)

type verifier struct {
	index domain.HarnessAdapterReleaseIndex
	err   error
}

func (v verifier) VerifyHarnessReleaseIndex(context.Context, []byte) (domain.HarnessAdapterReleaseIndex, error) {
	return v.index, v.err
}
func fixture(t *testing.T) domain.HarnessAdapterReleaseIndex {
	t.Helper()
	now := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	release := func(seq uint64, version string) domain.TrustedHarnessAdapterRelease {
		return domain.TrustedHarnessAdapterRelease{TrustRootID: "root", AdapterID: "codex", Version: version, Sequence: seq, ArtifactDigest: domain.DigestSHA256([]byte(version)), ManifestDigest: domain.DigestSHA256([]byte("m" + version)), OS: runtime.GOOS, Architecture: runtime.GOARCH, Size: 10, ExpiresAt: now.Add(time.Hour)}
	}
	idx := domain.HarnessAdapterReleaseIndex{TrustRootID: "root", Sequence: 3, ExpiresAt: now.Add(time.Hour), Releases: []domain.TrustedHarnessAdapterRelease{release(2, "1.1.0"), release(3, "1.2.0")}}
	d, err := idx.ContentDigest()
	if err != nil {
		t.Fatal(err)
	}
	idx.Digest = d
	return idx
}
func TestResolveHighestAboveFloor(t *testing.T) {
	idx := fixture(t)
	now := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	got, err := (Resolver{Trust: verifier{index: idx}}).Resolve(context.Background(), []byte("opaque"), Request{AdapterID: "codex", MinimumSequence: 1, Now: now})
	if err != nil || got.Sequence != 3 {
		t.Fatalf("got %+v err=%v", got, err)
	}
}
func TestResolveFailClosedWithoutTrust(t *testing.T) {
	_, err := (Resolver{}).Resolve(context.Background(), []byte("self signed claim"), Request{AdapterID: "codex", Now: time.Now()})
	if !errors.Is(err, domain.ErrHarnessAdapterReleaseTrustUnavailable) {
		t.Fatalf("err %v", err)
	}
}
func TestIndexTamperExpiryAndFloor(t *testing.T) {
	now := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		mutate func(*domain.HarnessAdapterReleaseIndex)
		floor  uint64
		want   error
	}{{"tamper", func(i *domain.HarnessAdapterReleaseIndex) { i.Releases[0].Size++ }, 0, domain.ErrHarnessAdapterReleaseIndexInvalid}, {"expiry", func(i *domain.HarnessAdapterReleaseIndex) { i.ExpiresAt = now }, 0, domain.ErrHarnessAdapterReleaseIndexInvalid}, {"floor", func(*domain.HarnessAdapterReleaseIndex) {}, 3, domain.ErrHarnessAdapterReleaseNotFound}} {
		t.Run(tc.name, func(t *testing.T) {
			idx := fixture(t)
			tc.mutate(&idx)
			_, err := (Resolver{Trust: verifier{index: idx}}).Resolve(context.Background(), nil, Request{AdapterID: "codex", MinimumSequence: tc.floor, Now: now})
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
		})
	}
}
func TestIndexCanonicalDigestIndependentOfOrder(t *testing.T) {
	a := fixture(t)
	b := a
	b.Releases = []domain.TrustedHarnessAdapterRelease{a.Releases[1], a.Releases[0]}
	d, err := b.ContentDigest()
	if err != nil || d != a.Digest {
		t.Fatalf("digest %s err=%v", d, err)
	}
}

var _ ports.HarnessReleaseTrustVerifier = verifier{}
