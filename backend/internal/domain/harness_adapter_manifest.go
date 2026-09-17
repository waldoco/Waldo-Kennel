package domain

import (
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
)

// HarnessAdapterManifest is a content-addressed description of a Kennel-owned
// adapter. It verifies compatibility only and grants no transport or owner authority.
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
	ManifestInvalid                  HarnessManifestClassification = "invalid_manifest"
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
func (m HarnessAdapterManifest) ContentDigest() (SHA256Digest, error) {
	b, err := m.canonicalBytes()
	if err != nil {
		return "", err
	}
	return DigestSHA256(b), nil
}
func (m HarnessAdapterManifest) ValidateShape() error {
	min, minOK := parseHarnessVersion(m.MinimumVersion)
	max, maxOK := parseHarnessVersion(m.MaximumVersion)
	if strings.TrimSpace(m.AdapterID) == "" || !m.AdapterDigest.Valid() || !m.Digest.Valid() || len(m.TransportClasses) == 0 || len(m.ProtocolFingerprints) == 0 || !minOK || !maxOK || compareHarnessVersion(min, max) > 0 {
		return ErrHarnessManifestInvalid
	}
	classes := map[HarnessCapabilityClass]bool{}
	for _, c := range m.TransportClasses {
		if !c.Valid() || classes[c] {
			return ErrHarnessManifestInvalid
		}
		classes[c] = true
	}
	fingerprints := map[SHA256Digest]bool{}
	for _, d := range m.ProtocolFingerprints {
		if !d.Valid() || fingerprints[d] {
			return ErrHarnessManifestInvalid
		}
		fingerprints[d] = true
	}
	return nil
}
func (m HarnessAdapterManifest) Verify(version string, protocol SHA256Digest, required, optional []HarnessCapabilityClass) HarnessManifestClassification {
	if m.ValidateShape() != nil {
		return ManifestInvalid
	}
	want, err := m.ContentDigest()
	if err != nil || want != m.Digest {
		return ManifestDigestTamper
	}
	installed, ok := parseHarnessVersion(version)
	if !ok {
		return ManifestInvalid
	}
	min, _ := parseHarnessVersion(m.MinimumVersion)
	max, _ := parseHarnessVersion(m.MaximumVersion)
	if compareHarnessVersion(installed, min) < 0 || compareHarnessVersion(installed, max) > 0 {
		return ManifestUnsupportedVersion
	}
	if !protocol.Valid() {
		return ManifestProtocolDrift
	}
	protocolOK := false
	for _, d := range m.ProtocolFingerprints {
		if d == protocol {
			protocolOK = true
			break
		}
	}
	if !protocolOK {
		return ManifestProtocolDrift
	}
	declared := map[HarnessCapabilityClass]bool{}
	for _, c := range m.TransportClasses {
		declared[c] = true
	}
	for _, c := range required {
		if !c.Valid() || !declared[c] {
			return ManifestRequiredMissing
		}
	}
	for _, c := range optional {
		if !c.Valid() || !declared[c] {
			return ManifestOptionalDegradation
		}
	}
	return ManifestKnownCompatible
}

type harnessVersion [3]uint64

func parseHarnessVersion(raw string) (harnessVersion, bool) {
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(raw), "v"), ".")
	if len(parts) != 3 {
		return harnessVersion{}, false
	}
	var out harnessVersion
	for i, p := range parts {
		if p == "" || (len(p) > 1 && p[0] == '0') {
			return harnessVersion{}, false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return harnessVersion{}, false
			}
		}
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return harnessVersion{}, false
		}
		out[i] = n
	}
	return out, true
}
func compareHarnessVersion(a, b harnessVersion) int {
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// CompareHarnessVersions strictly compares numeric major.minor.patch versions.
func CompareHarnessVersions(a, b string) (int, error) {
	left, ok := parseHarnessVersion(a)
	if !ok {
		return 0, ErrHarnessManifestInvalid
	}
	right, ok := parseHarnessVersion(b)
	if !ok {
		return 0, ErrHarnessManifestInvalid
	}
	return compareHarnessVersion(left, right), nil
}
