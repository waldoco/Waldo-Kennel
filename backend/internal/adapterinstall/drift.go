package adapterinstall

import (
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// DriftFacts are trusted/observed facts only. ClassifyDrift has no side effects
// and uses safety-first precedence so callers cannot choose a friendlier repair.
type DriftFacts struct {
	Now                    time.Time
	Release                domain.TrustedHarnessAdapterRelease
	HighestTrustedSequence uint64
	ReleaseMatchesManifest bool
	OpenedDigest           domain.SHA256Digest
	ExpectedDigest         domain.SHA256Digest
	CanonicalSourceMatches bool
	KnownReleaseMatches    bool
	ActivationPending      bool
	RollbackFailed         bool
	ManifestClassification domain.HarnessManifestClassification
	NewerCompatibleRelease bool
}

type DriftEvaluation struct {
	Classification domain.HarnessAdapterDrift
	Repair         domain.HarnessAdapterInstallRepair
}

func ClassifyDrift(f DriftFacts) DriftEvaluation {
	class := domain.AdapterDriftInSync
	switch {
	case f.RollbackFailed:
		class = domain.AdapterDriftRollbackFailed
	case !f.ExpectedDigest.IsZero() && f.OpenedDigest != f.ExpectedDigest:
		if f.KnownReleaseMatches {
			class = domain.AdapterDriftLocalModification
		} else {
			class = domain.AdapterDriftArtifactTamper
		}
	case !f.ReleaseMatchesManifest:
		class = domain.AdapterDriftMixedGeneration
	case f.Release.Sequence <= f.HighestTrustedSequence:
		class = domain.AdapterDriftRollbackAttempt
	case f.Release.Validate(f.Now) != nil:
		class = domain.AdapterDriftMetadataExpired
	case f.ActivationPending:
		class = domain.AdapterDriftActivationIncomplete
	case f.ManifestClassification == domain.ManifestRequiredMissing:
		class = domain.AdapterDriftRequiredCapabilityMissing
	case f.ManifestClassification == domain.ManifestProtocolDrift:
		class = domain.AdapterDriftProtocol
	case !f.CanonicalSourceMatches:
		class = domain.AdapterDriftSource
	case f.ManifestClassification == domain.ManifestOptionalDegradation:
		class = domain.AdapterDriftOptionalDegradation
	case f.NewerCompatibleRelease:
		class = domain.AdapterDriftUpgradeAvailable
	}
	return DriftEvaluation{Classification: class, Repair: repairFor(class)}
}
