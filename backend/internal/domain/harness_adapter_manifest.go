package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

// HarnessAdapterManifest is the signed-or-content-addressed description of a
// Kennel-owned adapter. Its digest is over the canonical JSON representation
// of the manifest with Digest omitted.
type HarnessAdapterManifest struct {
	AdapterID            string                   `json:"adapter_id"`
	AdapterDigest        SHA256Digest             `json:"adapter_digest"`
	TransportClasses     []HarnessCapabilityClass `json:"transport_classes"`
	MinimumVersion       string                   `json:"minimum_version"`
	MaximumVersion       string                   `json:"maximum_version"`
	ProtocolFingerprints []SHA256Digest           `json:"protocol_fingerprints"`
	Digest               SHA256Digest             `json:"digest"`
}

type HarnessManifestClassification string

const (
	ManifestKnownCompatible          HarnessManifestClassification = "known_compatible"
	ManifestOptionalDegradation      HarnessManifestClassification = "optional_degradation"
	ManifestRequiredMissing          HarnessManifestClassification = "required_capability_missing"
	ManifestProtocolDrift            HarnessManifestClassification = "protocol_drift"
	ManifestDigestTamper             HarnessManifestClassification = "digest_tamper"
	ManifestUnsupportedVersion       HarnessManifestClassification = "unsupported_version"
	ManifestSuppliedArtifactRollback HarnessManifestClassification = "supplied_artifact_rollback"
)

var ErrHarnessManifestInvalid = errors.New("invalid harness adapter manifest")

func (m HarnessAdapterManifest) canonicalBytes() ([]byte, error) {
	type unsigned HarnessAdapterManifest
	u := unsigned(m)
	u.Digest = ""
	u.TransportClasses = append([]HarnessCapabilityClass(nil), m.TransportClasses...)
	u.ProtocolFingerprints = append([]SHA256Digest(nil), m.ProtocolFingerprints...)
	sort.Slice(u.TransportClasses, func(i, j int) bool { return u.TransportClasses[i] < u.TransportClasses[j] })
	sort.Slice(u.ProtocolFingerprints, func(i, j int) bool { return u.ProtocolFingerprints[i] < u.ProtocolFingerprints[j] })
	return json.Marshal(u)
}

// ContentDigest computes the address of a manifest without trusting its
// supplied Digest field.
func (m HarnessAdapterManifest) ContentDigest() (SHA256Digest, error) {
	b, err := m.canonicalBytes()
	if err != nil {
		return "", err
	}
	return DigestSHA256(b), nil
}

func (m HarnessAdapterManifest) Validate() error {
	if strings.TrimSpace(m.AdapterID) == "" || !m.AdapterDigest.Valid() || strings.TrimSpace(m.MinimumVersion) == "" || strings.TrimSpace(m.MaximumVersion) == "" || !m.Digest.Valid() || len(m.TransportClasses) == 0 || len(m.ProtocolFingerprints) == 0 {
		return ErrHarnessManifestInvalid
	}
	seen := map[HarnessCapabilityClass]bool{}
	for _, c := range m.TransportClasses {
		if !c.Valid() || seen[c] {
			return ErrHarnessManifestInvalid
		}
		seen[c] = true
	}
	for _, d := range m.ProtocolFingerprints {
		if !d.Valid() {
			return ErrHarnessManifestInvalid
		}
	}
	want, err := m.ContentDigest()
	if err != nil || want != m.Digest {
		return fmt.Errorf("%w: content digest mismatch", ErrHarnessManifestInvalid)
	}
	return nil
}

// Verify classifies compatibility in a stable order. Provenance and a
// manifest never grant authority; this only verifies transport prerequisites.
func (m HarnessAdapterManifest) Verify(version string, protocol SHA256Digest, required, optional []HarnessCapabilityClass) HarnessManifestClassification {
	if err := m.Validate(); err != nil {
		if strings.Contains(err.Error(), "content digest") {
			return ManifestDigestTamper
		}
		return ManifestDigestTamper
	}
	if compareVersions(version, m.MinimumVersion) < 0 || compareVersions(version, m.MaximumVersion) > 0 {
		return ManifestUnsupportedVersion
	}
	for _, c := range required {
		found := false
		for _, declared := range m.TransportClasses {
			if c == declared {
				found = true
			}
		}
		if !found {
			return ManifestRequiredMissing
		}
	}
	for _, c := range optional {
		found := false
		for _, declared := range m.TransportClasses {
			if c == declared {
				found = true
			}
		}
		if !found {
			return ManifestOptionalDegradation
		}
	}
	for _, d := range m.ProtocolFingerprints {
		if d == protocol {
			return ManifestKnownCompatible
		}
	}
	return ManifestProtocolDrift
}

// VerifySuppliedArtifact hashes a caller-supplied local artifact only. It
// never downloads, installs, replaces, or rolls back an existing adapter.
func VerifySuppliedArtifact(path string, expected SHA256Digest, currentVersion, suppliedVersion string) (HarnessManifestClassification, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return ManifestDigestTamper, err
	}
	if DigestSHA256(b) != expected {
		return ManifestDigestTamper, nil
	}
	if compareVersions(suppliedVersion, currentVersion) < 0 {
		return ManifestSuppliedArtifactRollback, nil
	}
	return ManifestKnownCompatible, nil
}

func compareVersions(a, b string) int {
	parse := func(v string) []int {
		var out []int
		for _, part := range strings.Split(strings.TrimPrefix(v, "v"), ".") {
			n := 0
			for _, r := range part {
				if r < '0' || r > '9' {
					break
				}
				n = n*10 + int(r-'0')
			}
			out = append(out, n)
		}
		return out
	}
	x, y := parse(a), parse(b)
	for i := 0; i < len(x) || i < len(y); i++ {
		xv, yv := 0, 0
		if i < len(x) {
			xv = x[i]
		}
		if i < len(y) {
			yv = y[i]
		}
		if xv != yv {
			if xv < yv {
				return -1
			}
			return 1
		}
	}
	return 0
}
