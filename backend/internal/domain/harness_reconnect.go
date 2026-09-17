package domain

import (
	"errors"
	"strings"
	"time"
)

type HarnessReconnectReason string
type HarnessReconnectRepair string

const (
	ReconnectReasonDaemonUnreachable       HarnessReconnectReason = "daemon_unreachable"
	ReconnectReasonDaemonIdentityMismatch  HarnessReconnectReason = "daemon_identity_mismatch"
	ReconnectReasonMissionNotFound         HarnessReconnectReason = "mission_not_found"
	ReconnectReasonMissionProfileMissing   HarnessReconnectReason = "mission_profile_missing"
	ReconnectReasonDeliveryUnknown         HarnessReconnectReason = "delivery_unknown"
	ReconnectReasonPairingFactsUnavailable HarnessReconnectReason = "pairing_facts_unavailable"
	ReconnectReasonObservationStale        HarnessReconnectReason = "observation_stale"

	ReconnectRepairStartOrRepairDaemon HarnessReconnectRepair = "start_or_repair_daemon"
	ReconnectRepairDaemonEndpoint      HarnessReconnectRepair = "repair_daemon_endpoint"
	ReconnectRepairChooseMission       HarnessReconnectRepair = "choose_existing_mission"
	ReconnectRepairMissionProfile      HarnessReconnectRepair = "repair_mission_profile"
	ReconnectRepairDeliveryEvidence    HarnessReconnectRepair = "review_delivery_evidence"
	ReconnectRepairPairAdapter         HarnessReconnectRepair = "pair_adapter"
	ReconnectRepairRefreshObservation  HarnessReconnectRepair = "refresh_connection_observation"
)

// HarnessMissionProfile is the durable profile pinned to one mission. Reconnect
// may read this binding but cannot migrate it.
type HarnessMissionProfile struct {
	MissionID            string
	ConnectionID         HarnessConnectionID
	InstallationID       string
	AdapterDigest        SHA256Digest
	HarnessIdentity      string
	ProviderVersion      string
	ProtocolFingerprint  SHA256Digest
	Generation           int64
	RequiredCapabilities []HarnessCapabilityClass
	OptionalCapabilities []HarnessCapabilityClass
}

func (p HarnessMissionProfile) Validate() error {
	if strings.TrimSpace(p.MissionID) == "" || strings.TrimSpace(string(p.ConnectionID)) == "" || strings.TrimSpace(p.InstallationID) == "" || !p.AdapterDigest.Valid() || strings.TrimSpace(p.HarnessIdentity) == "" || strings.TrimSpace(p.ProviderVersion) == "" || !p.ProtocolFingerprint.Valid() || p.Generation < 1 {
		return ErrHarnessReconnectInvalid
	}
	if _, err := NormalizeHarnessCapabilities(p.RequiredCapabilities); err != nil {
		return err
	}
	seen := map[HarnessCapabilityClass]bool{}
	for _, c := range p.RequiredCapabilities {
		seen[c] = true
	}
	for _, c := range p.OptionalCapabilities {
		if !c.Valid() || seen[c] {
			return ErrHarnessReconnectInvalid
		}
		seen[c] = true
	}
	return nil
}

type HarnessDaemonReadiness struct {
	InstanceID    string
	Generation    int64
	APICompatible bool
	ObservedAt    time.Time
}

func (d HarnessDaemonReadiness) Validate() error {
	if strings.TrimSpace(d.InstanceID) == "" || d.Generation < 1 || d.ObservedAt.IsZero() {
		return ErrHarnessReconnectInvalid
	}
	return nil
}

type HarnessPairingObservation struct {
	InstallationID      string
	AdapterDigest       SHA256Digest
	HarnessIdentity     string
	ProviderVersion     string
	ProtocolFingerprint SHA256Digest
	MissionID           string
	AppRunID            string
	Generation          int64
	ObservedAt          time.Time
}

func (o HarnessPairingObservation) Validate() error {
	if strings.TrimSpace(o.InstallationID) == "" || !o.AdapterDigest.Valid() || strings.TrimSpace(o.HarnessIdentity) == "" || strings.TrimSpace(o.ProviderVersion) == "" || !o.ProtocolFingerprint.Valid() || strings.TrimSpace(o.MissionID) == "" || strings.TrimSpace(o.AppRunID) == "" || o.Generation < 1 || o.ObservedAt.IsZero() {
		return ErrHarnessReconnectInvalid
	}
	return nil
}

type HarnessDeliveryBlock struct {
	CommandID, CommandClass, CorrelationID string
	DispatchGeneration                     int64
	ObservedAt                             time.Time
}

func (b HarnessDeliveryBlock) Validate() error {
	if strings.TrimSpace(b.CommandID) == "" || strings.TrimSpace(b.CommandClass) == "" || b.DispatchGeneration < 1 || b.ObservedAt.IsZero() {
		return ErrHarnessReconnectInvalid
	}
	return nil
}

type HarnessReconnectEvaluation struct {
	State                HarnessConnectionState
	Reason               string
	Repair               string
	MissionID            string
	DaemonInstanceID     string
	ConnectionGeneration int64
	EvaluatedAt          time.Time
}

var (
	ErrHarnessReconnectInvalid        = errors.New("invalid harness reconnect facts")
	ErrHarnessPairingFactsUnavailable = errors.New("S3.3 pairing facts are unavailable")
)
