package harnesscommand

import "github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"

type request struct {
	Type                 string                         `json:"type"`
	ConnectionBearer     string                         `json:"connection_bearer"`
	ConnectionID         string                         `json:"connection_id"`
	InstallationID       string                         `json:"installation_id"`
	AdapterDigest        string                         `json:"adapter_digest"`
	HarnessIdentity      string                         `json:"harness_identity"`
	ProviderVersion      string                         `json:"provider_version"`
	ProtocolFingerprint  string                         `json:"protocol_fingerprint"`
	MissionID            string                         `json:"mission_id"`
	AppRunID             string                         `json:"app_run_id"`
	ConnectionGeneration int64                          `json:"connection_generation"`
	TransportClass       string                         `json:"transport_class"`
	OwnerProofID         string                         `json:"owner_proof_id"`
	OwnerProofBearer     string                         `json:"owner_proof_bearer"`
	Target               domain.OwnerProofTarget        `json:"target"`
	Command              domain.CanonicalHarnessCommand `json:"command"`
	AdapterRequestKey    string                         `json:"adapter_request_key"`
}
type response struct {
	OK            bool   `json:"ok"`
	ClaimID       string `json:"claim_id,omitempty"`
	DestinationID string `json:"destination_id,omitempty"`
}

var genericFailure = []byte("{\"ok\":false}\n")
