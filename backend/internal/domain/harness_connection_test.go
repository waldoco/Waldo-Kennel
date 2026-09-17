package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testHarnessConnection() HarnessConnection {
	now := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	return HarnessConnection{ID: "hc-1", InstallationID: "install-1", AdapterDigest: DigestSHA256([]byte("adapter")), HarnessIdentity: "codex", ProviderVersion: "0.154.0", ProtocolFingerprint: DigestSHA256([]byte("protocol")), MissionID: "mission-1", AppRunID: "run-1", CapabilityClasses: []HarnessCapabilityClass{HarnessCapabilityAnswer, HarnessCapabilityTurn}, CapabilityVerifier: strings.Repeat("a", 64), Generation: 1, ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now}
}
func TestHarnessConnectionNoSecretStringOrJSON(t *testing.T) {
	rec := testHarnessConnection()
	if strings.Contains(rec.String(), rec.CapabilityVerifier) {
		t.Fatal("String leaks verifier")
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), rec.CapabilityVerifier) || strings.Contains(string(raw), "CapabilityVerifier") {
		t.Fatalf("JSON leaks verifier: %s", raw)
	}
}
func TestEvaluateHarnessConnection(t *testing.T) {
	rec := testHarnessConnection()
	now := rec.CreatedAt
	facts := HarnessConnectionFacts{InstallationID: rec.InstallationID, AdapterDigest: rec.AdapterDigest, HarnessIdentity: rec.HarnessIdentity, MissionID: rec.MissionID, AppRunID: rec.AppRunID, ProtocolFingerprint: rec.ProtocolFingerprint, Generation: 1, RequiredCapabilities: []HarnessCapabilityClass{HarnessCapabilityTurn}, Now: now}
	if got := EvaluateHarnessConnection(&rec, facts); got.State != HarnessConnected {
		t.Fatalf("connected: %+v", got)
	}
	copy := facts
	copy.MissionID = "spoof"
	if got := EvaluateHarnessConnection(&rec, copy); got.Reason != HarnessReasonBindingMismatch || got.Repair != HarnessRepairPairing {
		t.Fatalf("binding: %+v", got)
	}
	copy = facts
	copy.RequiredCapabilities = []HarnessCapabilityClass{HarnessCapabilityCancel}
	if got := EvaluateHarnessConnection(&rec, copy); got.Reason != HarnessReasonRequiredCapabilityMissing {
		t.Fatalf("missing: %+v", got)
	}
	copy = facts
	copy.OptionalCapabilities = []HarnessCapabilityClass{HarnessCapabilityCancel}
	if got := EvaluateHarnessConnection(&rec, copy); got.State != HarnessDegraded {
		t.Fatalf("degraded: %+v", got)
	}
	copy = facts
	copy.Now = rec.ExpiresAt
	if got := EvaluateHarnessConnection(&rec, copy); got.Reason != HarnessReasonExpired {
		t.Fatalf("expired: %+v", got)
	}
}
