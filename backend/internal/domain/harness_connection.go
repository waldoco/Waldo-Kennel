package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// HarnessConnection authenticates one adapter transport. It is not an owner
// identity and does not grant any command authority beyond carrying a class.
type HarnessConnectionID string

type HarnessCapabilityClass string

const (
	HarnessCapabilityTurn      HarnessCapabilityClass = "turn"
	HarnessCapabilitySteer     HarnessCapabilityClass = "steer"
	HarnessCapabilityAnswer    HarnessCapabilityClass = "answer"
	HarnessCapabilityInterrupt HarnessCapabilityClass = "interrupt"
	HarnessCapabilityCancel    HarnessCapabilityClass = "cancel"
	HarnessCapabilityReplace   HarnessCapabilityClass = "replace"
	HarnessCapabilityApproval  HarnessCapabilityClass = "approval"
	HarnessCapabilityAccept    HarnessCapabilityClass = "accept"
)

var harnessCapabilityClasses = map[HarnessCapabilityClass]struct{}{
	HarnessCapabilityTurn: {}, HarnessCapabilitySteer: {}, HarnessCapabilityAnswer: {},
	HarnessCapabilityInterrupt: {}, HarnessCapabilityCancel: {}, HarnessCapabilityReplace: {},
	HarnessCapabilityApproval: {}, HarnessCapabilityAccept: {},
}

func (c HarnessCapabilityClass) Valid() bool { _, ok := harnessCapabilityClasses[c]; return ok }

type HarnessConnection struct {
	ID                  HarnessConnectionID
	InstallationID      string
	AdapterDigest       SHA256Digest
	HarnessIdentity     string
	ProviderVersion     string
	ProtocolFingerprint SHA256Digest
	MissionID           string
	AppRunID            string
	CapabilityClasses   []HarnessCapabilityClass
	Generation          int64
	ExpiresAt           time.Time
	RevokedAt           *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
	// CapabilityVerifier is a one-way verifier. It is intentionally excluded
	// from JSON and String so a durable record cannot become a bearer.
	CapabilityVerifier string `json:"-"`
}

func (h HarnessConnection) String() string {
	return fmt.Sprintf("HarnessConnection(%s,generation=%d)", h.ID, h.Generation)
}

func (h HarnessConnection) Validate() error {
	if strings.TrimSpace(string(h.ID)) == "" || strings.TrimSpace(h.InstallationID) == "" ||
		!h.AdapterDigest.Valid() || strings.TrimSpace(h.HarnessIdentity) == "" ||
		strings.TrimSpace(h.ProviderVersion) == "" || !h.ProtocolFingerprint.Valid() ||
		strings.TrimSpace(h.MissionID) == "" || strings.TrimSpace(h.AppRunID) == "" ||
		h.Generation < 1 || h.ExpiresAt.IsZero() || h.CreatedAt.IsZero() || h.UpdatedAt.IsZero() ||
		!SHA256Digest(h.CapabilityVerifier).Valid() || len(h.CapabilityClasses) == 0 ||
		!h.ExpiresAt.After(h.CreatedAt) || h.UpdatedAt.Before(h.CreatedAt) ||
		(h.RevokedAt != nil && h.RevokedAt.Before(h.CreatedAt)) {
		return ErrHarnessConnectionInvalid
	}
	seen := make(map[HarnessCapabilityClass]struct{}, len(h.CapabilityClasses))
	for _, class := range h.CapabilityClasses {
		if !class.Valid() {
			return ErrHarnessConnectionInvalid
		}
		if _, ok := seen[class]; ok {
			return ErrHarnessConnectionInvalid
		}
		seen[class] = struct{}{}
	}
	return nil
}

func (h HarnessConnection) HasCapability(class HarnessCapabilityClass) bool {
	for _, current := range h.CapabilityClasses {
		if current == class {
			return true
		}
	}
	return false
}

func NormalizeHarnessCapabilities(in []HarnessCapabilityClass) ([]HarnessCapabilityClass, error) {
	out := append([]HarnessCapabilityClass(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	for i, class := range out {
		if !class.Valid() || (i > 0 && out[i-1] == class) {
			return nil, ErrHarnessConnectionInvalid
		}
	}
	if len(out) == 0 {
		return nil, ErrHarnessConnectionInvalid
	}
	return out, nil
}

var (
	ErrHarnessConnectionInvalid  = errors.New("invalid harness connection")
	ErrHarnessConnectionConflict = errors.New("harness connection conflict")
	ErrHarnessAuthentication     = errors.New("harness authentication failed")
)

type HarnessConnectionState string
type HarnessConnectionReason string
type HarnessConnectionRepair string

const (
	HarnessConnected    HarnessConnectionState = "connected"
	HarnessDegraded     HarnessConnectionState = "degraded"
	HarnessActionNeeded HarnessConnectionState = "action_needed"

	HarnessReasonUnpaired                  HarnessConnectionReason = "unpaired"
	HarnessReasonExpired                   HarnessConnectionReason = "expired"
	HarnessReasonRevoked                   HarnessConnectionReason = "revoked"
	HarnessReasonStaleGeneration           HarnessConnectionReason = "stale_generation"
	HarnessReasonBindingMismatch           HarnessConnectionReason = "binding_mismatch"
	HarnessReasonRequiredCapabilityMissing HarnessConnectionReason = "required_capability_missing"
	HarnessReasonProtocolDrift             HarnessConnectionReason = "protocol_drift"
	HarnessReasonOptionalCapabilityMissing HarnessConnectionReason = "optional_capability_missing"

	HarnessRepairPairAdapter              HarnessConnectionRepair = "pair_adapter"
	HarnessRepairRotateCapability         HarnessConnectionRepair = "rotate_capability"
	HarnessRepairPairing                  HarnessConnectionRepair = "repair_pairing"
	HarnessRepairReconnectAdapter         HarnessConnectionRepair = "reconnect_adapter"
	HarnessRepairCompatibility            HarnessConnectionRepair = "repair_compatibility"
	HarnessRepairReviewDegradedCapability HarnessConnectionRepair = "review_degraded_capability"
)

type HarnessConnectionEvaluation struct {
	State  HarnessConnectionState
	Reason HarnessConnectionReason
	Repair HarnessConnectionRepair
}

type HarnessConnectionFacts struct {
	InstallationID, HarnessIdentity, MissionID, AppRunID string
	AdapterDigest, ProtocolFingerprint                   SHA256Digest
	Generation                                           int64
	RequiredCapabilities, OptionalCapabilities           []HarnessCapabilityClass
	Now                                                  time.Time
}

func EvaluateHarnessConnection(connection *HarnessConnection, facts HarnessConnectionFacts) HarnessConnectionEvaluation {
	if connection == nil {
		return HarnessConnectionEvaluation{HarnessActionNeeded, HarnessReasonUnpaired, HarnessRepairPairAdapter}
	}
	if connection.RevokedAt != nil {
		return HarnessConnectionEvaluation{HarnessActionNeeded, HarnessReasonRevoked, HarnessRepairPairing}
	}
	if !facts.Now.Before(connection.ExpiresAt) {
		return HarnessConnectionEvaluation{HarnessActionNeeded, HarnessReasonExpired, HarnessRepairRotateCapability}
	}
	if facts.Generation != connection.Generation {
		return HarnessConnectionEvaluation{HarnessActionNeeded, HarnessReasonStaleGeneration, HarnessRepairReconnectAdapter}
	}
	if facts.InstallationID != connection.InstallationID || facts.AdapterDigest != connection.AdapterDigest || facts.HarnessIdentity != connection.HarnessIdentity || facts.MissionID != connection.MissionID || facts.AppRunID != connection.AppRunID {
		return HarnessConnectionEvaluation{HarnessActionNeeded, HarnessReasonBindingMismatch, HarnessRepairPairing}
	}
	if facts.ProtocolFingerprint != connection.ProtocolFingerprint {
		return HarnessConnectionEvaluation{HarnessActionNeeded, HarnessReasonProtocolDrift, HarnessRepairCompatibility}
	}
	for _, c := range facts.RequiredCapabilities {
		if !connection.HasCapability(c) {
			return HarnessConnectionEvaluation{HarnessActionNeeded, HarnessReasonRequiredCapabilityMissing, HarnessRepairCompatibility}
		}
	}
	for _, c := range facts.OptionalCapabilities {
		if !connection.HasCapability(c) {
			return HarnessConnectionEvaluation{HarnessDegraded, HarnessReasonOptionalCapabilityMissing, HarnessRepairReviewDegradedCapability}
		}
	}
	return HarnessConnectionEvaluation{State: HarnessConnected}
}

// HarnessConnectionBinding is the complete adapter-presented tuple revalidated
// inside command claim transactions.
type HarnessConnectionBinding struct {
	ConnectionID                                          HarnessConnectionID
	InstallationID                                        string
	AdapterDigest                                         SHA256Digest
	HarnessIdentity, ProviderVersion, MissionID, AppRunID string
	ProtocolFingerprint                                   SHA256Digest
	Generation                                            int64
	Class                                                 HarnessCapabilityClass
}
