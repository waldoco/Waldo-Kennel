package harnessmanifest

import "github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"

func Verify(manifest domain.HarnessAdapterManifest, version string, protocol domain.SHA256Digest, required, optional []domain.HarnessCapabilityClass) domain.HarnessManifestClassification {
	return manifest.Verify(version, protocol, required, optional)
}
