package domain

import (
	"testing"
	"time"
)

func TestHarnessMissionProfileValidate(t *testing.T) {
	p := HarnessMissionProfile{MissionID: "mission", ConnectionID: "hc", InstallationID: "install", AdapterDigest: DigestSHA256([]byte("adapter")), HarnessIdentity: "codex", ProviderVersion: "1.0.0", ProtocolFingerprint: DigestSHA256([]byte("protocol")), Generation: 1, RequiredCapabilities: []HarnessCapabilityClass{HarnessCapabilityTurn}, OptionalCapabilities: []HarnessCapabilityClass{HarnessCapabilityAnswer}}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	p.OptionalCapabilities = []HarnessCapabilityClass{HarnessCapabilityTurn}
	if err := p.Validate(); err == nil {
		t.Fatal("duplicate required/optional accepted")
	}
}
func TestReadinessAndPairingObservationValidate(t *testing.T) {
	now := time.Now()
	if err := (HarnessDaemonReadiness{InstanceID: "daemon", Generation: 1, APICompatible: true, ObservedAt: now}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (HarnessPairingObservation{InstallationID: "i", AdapterDigest: DigestSHA256([]byte("a")), HarnessIdentity: "codex", ProviderVersion: "1.0.0", ProtocolFingerprint: DigestSHA256([]byte("p")), MissionID: "m", AppRunID: "run", Generation: 1, ObservedAt: now}).Validate(); err != nil {
		t.Fatal(err)
	}
}
