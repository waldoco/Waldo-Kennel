// Package harnesspairing implements the thin, protected local transport an
// adapter uses to request a pairing/rotation challenge and submit possession
// proof. It carries data only: it adds no owner-proof command, no
// confirmation bypass, no policy interpretation, and no filesystem/network
// grant. Every RPC failure — whatever the internal cause — produces the
// exact same wire response, so a peer can never learn which tuple field or
// proof byte was wrong.
package harnesspairing

// wireRequestChallenge asks the daemon to mint a fresh challenge for the
// exact tuple the caller claims. The daemon independently decides whether to
// honor this (a decision explicitly out of this slice's scope); the adapter
// selects none of the resulting identity, capability, generation, or expiry.
type wireRequestChallenge struct {
	Type                string   `json:"type"`
	Kind                string   `json:"kind"`
	ConnectionID        string   `json:"connection_id"`
	InstallationID      string   `json:"installation_id"`
	AdapterDigest       string   `json:"adapter_digest"`
	HarnessIdentity     string   `json:"harness_identity"`
	ProviderVersion     string   `json:"provider_version"`
	ProtocolFingerprint string   `json:"protocol_fingerprint"`
	MissionID           string   `json:"mission_id"`
	AppRunID            string   `json:"app_run_id"`
	CapabilityClasses   []string `json:"capability_classes"`
	ExpectedGeneration  int64    `json:"expected_generation"`
}

// wireChallengeIssued carries the one-time secret back to the caller that
// asked for it, over this connection only. It is never repeated afterward.
type wireChallengeIssued struct {
	OK          bool   `json:"ok"`
	ChallengeID string `json:"challenge_id,omitempty"`
	Secret      string `json:"secret,omitempty"`
}

// wireProve submits possession proof for a previously issued challenge.
type wireProve struct {
	Type                string   `json:"type"`
	ChallengeID         string   `json:"challenge_id"`
	Secret              string   `json:"secret"`
	InstallationID      string   `json:"installation_id"`
	AdapterDigest       string   `json:"adapter_digest"`
	HarnessIdentity     string   `json:"harness_identity"`
	ProviderVersion     string   `json:"provider_version"`
	ProtocolFingerprint string   `json:"protocol_fingerprint"`
	MissionID           string   `json:"mission_id"`
	AppRunID            string   `json:"app_run_id"`
	CapabilityClasses   []string `json:"capability_classes"`
	ExpectedGeneration  int64    `json:"expected_generation"`
}

// wireProveResult is the ONLY response shape Prove ever returns. Every
// failure reason collapses to {"ok":false} with no other field populated:
// this is the byte-identical generic response the invariant requires.
type wireProveResult struct {
	OK     bool   `json:"ok"`
	Bearer string `json:"bearer,omitempty"`
}

const (
	wireTypeRequestChallenge = "request_challenge"
	wireTypeProve            = "prove"
)

// genericFailure is the single, constant response for every request-channel
// failure: malformed input, unknown type, or any Prove failure regardless of
// its internal domain.HarnessPairingResultCode. It is a package-level value
// (not a fresh literal per call) so tests can assert on identical bytes by
// construction, not by accident.
var genericFailure = []byte(`{"ok":false}` + "\n")
