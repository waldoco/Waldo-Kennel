package harnessdiscovery

import (
	"errors"
	"sort"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// Classify is the single deterministic projection used by discovery callers.
func Classify(manifest domain.HarnessAdapterManifest, installation ports.HarnessInstallation, required, optional []domain.HarnessCapabilityClass) domain.HarnessManifestClassification {
	if installation.ExecutablePath == "" || !installation.ExecutableDigest.Valid() {
		return domain.ManifestInvalid
	}
	if !domain.SHA256Digest(installation.Protocol.ProtocolDigest).Valid() {
		return domain.ManifestProtocolDrift
	}
	return manifest.Verify(installation.Version, domain.SHA256Digest(installation.Protocol.ProtocolDigest), required, optional)
}

// NormalizeCapabilities provides deterministic input ordering for callers
// constructing frozen manifest requirements.
func NormalizeCapabilities(in []domain.HarnessCapabilityClass) ([]domain.HarnessCapabilityClass, error) {
	out := append([]domain.HarnessCapabilityClass(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	for i, c := range out {
		if !c.Valid() || (i > 0 && out[i-1] == c) {
			return nil, errors.New("invalid or duplicate capability")
		}
	}
	return out, nil
}
