package adapterinstall

import (
	"runtime"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

func driftFixture() DriftFacts {
	now := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	d := domain.DigestSHA256([]byte("a"))
	return DriftFacts{Now: now, Release: domain.TrustedHarnessAdapterRelease{TrustRootID: "root", AdapterID: "a", Version: "1.0.0", Sequence: 2, ArtifactDigest: d, ManifestDigest: domain.DigestSHA256([]byte("m")), OS: runtime.GOOS, Architecture: runtime.GOARCH, Size: 1, ExpiresAt: now.Add(time.Hour)}, HighestTrustedSequence: 1, ReleaseMatchesManifest: true, OpenedDigest: d, ExpectedDigest: d, CanonicalSourceMatches: true, ManifestClassification: domain.ManifestKnownCompatible}
}
func TestClassifyDriftSafetyPrecedence(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*DriftFacts)
		want   domain.HarnessAdapterDrift
	}{
		{"rollback failure", func(f *DriftFacts) { f.RollbackFailed = true; f.ReleaseMatchesManifest = false }, domain.AdapterDriftRollbackFailed},
		{"tamper before mixed", func(f *DriftFacts) {
			f.OpenedDigest = domain.DigestSHA256([]byte("bad"))
			f.ReleaseMatchesManifest = false
		}, domain.AdapterDriftArtifactTamper},
		{"mixed before rollback", func(f *DriftFacts) { f.ReleaseMatchesManifest = false; f.Release.Sequence = 1 }, domain.AdapterDriftMixedGeneration},
		{"rollback before expiry", func(f *DriftFacts) { f.Release.Sequence = 1; f.Release.ExpiresAt = f.Now }, domain.AdapterDriftRollbackAttempt},
		{"expired", func(f *DriftFacts) { f.Release.ExpiresAt = f.Now }, domain.AdapterDriftMetadataExpired},
		{"pending", func(f *DriftFacts) { f.ActivationPending = true }, domain.AdapterDriftActivationIncomplete},
		{"required", func(f *DriftFacts) { f.ManifestClassification = domain.ManifestRequiredMissing }, domain.AdapterDriftRequiredCapabilityMissing},
		{"protocol", func(f *DriftFacts) { f.ManifestClassification = domain.ManifestProtocolDrift }, domain.AdapterDriftProtocol},
		{"source", func(f *DriftFacts) { f.CanonicalSourceMatches = false }, domain.AdapterDriftSource},
		{"optional", func(f *DriftFacts) { f.ManifestClassification = domain.ManifestOptionalDegradation }, domain.AdapterDriftOptionalDegradation},
		{"upgrade", func(f *DriftFacts) { f.NewerCompatibleRelease = true }, domain.AdapterDriftUpgradeAvailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := driftFixture()
			tc.mutate(&f)
			got := ClassifyDrift(f)
			if got.Classification != tc.want || got.Repair == "" {
				t.Fatalf("got %+v want %s", got, tc.want)
			}
		})
	}
}

func TestClassifyDriftInSyncHasNoRepair(t *testing.T) {
	got := ClassifyDrift(driftFixture())
	if got.Classification != domain.AdapterDriftInSync || got.Repair != "" {
		t.Fatalf("got %+v", got)
	}
}
