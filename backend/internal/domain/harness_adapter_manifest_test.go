package domain

import "testing"

func manifestFixture() HarnessAdapterManifest {
	m := HarnessAdapterManifest{AdapterID: "codex-adapter", AdapterDigest: DigestSHA256([]byte("artifact")), TransportClasses: []HarnessCapabilityClass{HarnessCapabilityTurn, HarnessCapabilitySteer}, MinimumVersion: "0.150.0", MaximumVersion: "0.160.0", ProtocolFingerprints: []SHA256Digest{DigestSHA256([]byte("protocol"))}}
	m.Digest, _ = m.ContentDigest()
	return m
}
func TestHarnessAdapterManifestClassifications(t *testing.T) {
	m := manifestFixture()
	cases := []struct {
		name               string
		mutate             func(*HarnessAdapterManifest)
		version            string
		protocol           SHA256Digest
		required, optional []HarnessCapabilityClass
		want               HarnessManifestClassification
	}{{"compatible", func(*HarnessAdapterManifest) {}, "0.154.0", m.ProtocolFingerprints[0], []HarnessCapabilityClass{HarnessCapabilityTurn}, nil, ManifestKnownCompatible}, {"tamper", func(x *HarnessAdapterManifest) { x.AdapterID = "changed" }, "0.154.0", m.ProtocolFingerprints[0], nil, nil, ManifestDigestTamper}, {"malformed", func(x *HarnessAdapterManifest) { x.MinimumVersion = "latest" }, "0.154.0", m.ProtocolFingerprints[0], nil, nil, ManifestInvalid}, {"oldest", func(*HarnessAdapterManifest) {}, "0.150.0", m.ProtocolFingerprints[0], nil, nil, ManifestKnownCompatible}, {"latest", func(*HarnessAdapterManifest) {}, "0.160.0", m.ProtocolFingerprints[0], nil, nil, ManifestKnownCompatible}, {"unsupported", func(*HarnessAdapterManifest) {}, "0.161.0", m.ProtocolFingerprints[0], nil, nil, ManifestUnsupportedVersion}, {"protocol before optional", func(*HarnessAdapterManifest) {}, "0.154.0", DigestSHA256([]byte("drift")), nil, []HarnessCapabilityClass{HarnessCapabilityCancel}, ManifestProtocolDrift}, {"required", func(*HarnessAdapterManifest) {}, "0.154.0", m.ProtocolFingerprints[0], []HarnessCapabilityClass{HarnessCapabilityCancel}, nil, ManifestRequiredMissing}, {"optional", func(*HarnessAdapterManifest) {}, "0.154.0", m.ProtocolFingerprints[0], nil, []HarnessCapabilityClass{HarnessCapabilityCancel}, ManifestOptionalDegradation}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			x := m
			tc.mutate(&x)
			if got := x.Verify(tc.version, tc.protocol, tc.required, tc.optional); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}
func TestCompareHarnessVersionsStrict(t *testing.T) {
	if got, err := CompareHarnessVersions("0.154.0", "0.153.9"); err != nil || got != 1 {
		t.Fatalf("got %d %v", got, err)
	}
	for _, v := range []string{"latest", "0.1", "01.2.3", "1.2.x"} {
		if _, err := CompareHarnessVersions(v, "1.2.3"); err == nil {
			t.Fatalf("accepted %q", v)
		}
	}
}
