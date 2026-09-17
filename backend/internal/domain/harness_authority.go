package domain

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

type HarnessPairingIntentStatus string

const (
	HarnessPairingIntentRequested        HarnessPairingIntentStatus = "requested"
	HarnessPairingIntentApproved         HarnessPairingIntentStatus = "approved"
	HarnessPairingIntentActivating       HarnessPairingIntentStatus = "activating"
	HarnessPairingIntentActive           HarnessPairingIntentStatus = "challenge_active"
	HarnessPairingIntentDenied           HarnessPairingIntentStatus = "denied"
	HarnessPairingIntentActivationFailed HarnessPairingIntentStatus = "activation_failed"
	HarnessPairingIntentSuperseded       HarnessPairingIntentStatus = "superseded"
)

func (s HarnessPairingIntentStatus) Valid() bool {
	switch s {
	case HarnessPairingIntentRequested, HarnessPairingIntentApproved, HarnessPairingIntentActivating, HarnessPairingIntentActive, HarnessPairingIntentDenied, HarnessPairingIntentActivationFailed, HarnessPairingIntentSuperseded:
		return true
	}
	return false
}

type HarnessPairingIntent struct {
	ID                                                                        PairingChallengeID         `json:"id"`
	ProjectID                                                                 ProjectID                  `json:"projectId"`
	Kind                                                                      HarnessPairingKind         `json:"kind"`
	ConnectionID                                                              HarnessConnectionID        `json:"connectionId"`
	InstallationID                                                            string                     `json:"installationId"`
	AdapterDigest                                                             SHA256Digest               `json:"adapterDigest"`
	HarnessIdentity                                                           string                     `json:"harnessIdentity"`
	ProviderVersion                                                           string                     `json:"providerVersion"`
	ProtocolFingerprint                                                       SHA256Digest               `json:"protocolFingerprint"`
	MissionID                                                                 string                     `json:"missionId"`
	AppRunID                                                                  string                     `json:"-"`
	CapabilityClasses                                                         []HarnessCapabilityClass   `json:"capabilityClasses"`
	ExpectedGeneration                                                        int64                      `json:"expectedGeneration"`
	ConnectionExpiresAt                                                       time.Time                  `json:"connectionExpiresAt"`
	ExpiresAt                                                                 time.Time                  `json:"expiresAt"`
	Digest                                                                    SHA256Digest               `json:"digest"`
	Status                                                                    HarnessPairingIntentStatus `json:"status"`
	ChallengeID                                                               *PairingChallengeID        `json:"challengeId,omitempty"`
	ProposalRequestKey                                                        string
	ProposalRequestFingerprint                                                SHA256Digest
	DecisionID, Decision, DecisionRequestKey, OwnerPrincipal, ConfirmationRef string
	DecidedAt                                                                 *time.Time
	CreatedAt, UpdatedAt                                                      time.Time
}

type pairingIntentDigest struct {
	Version                        string              `json:"version"`
	ProjectID                      ProjectID           `json:"projectId"`
	Kind                           HarnessPairingKind  `json:"kind"`
	ConnectionID                   HarnessConnectionID `json:"connectionId"`
	InstallationID                 string              `json:"installationId"`
	AdapterDigest                  SHA256Digest        `json:"adapterDigest"`
	HarnessIdentity                string              `json:"harnessIdentity"`
	ProviderVersion                string              `json:"providerVersion"`
	ProtocolFingerprint            SHA256Digest        `json:"protocolFingerprint"`
	MissionID, AppRunID            string
	CapabilityClasses              []HarnessCapabilityClass `json:"capabilityClasses"`
	ExpectedGeneration             int64                    `json:"expectedGeneration"`
	ConnectionExpiresAt, ExpiresAt time.Time
}

func (i HarnessPairingIntent) ComputedDigest() (SHA256Digest, error) {
	c := append([]HarnessCapabilityClass(nil), i.CapabilityClasses...)
	sort.Slice(c, func(a, b int) bool { return c[a] < c[b] })
	b, e := json.Marshal(pairingIntentDigest{"v1", i.ProjectID, i.Kind, i.ConnectionID, i.InstallationID, i.AdapterDigest, i.HarnessIdentity, i.ProviderVersion, i.ProtocolFingerprint, i.MissionID, i.AppRunID, c, i.ExpectedGeneration, i.ConnectionExpiresAt.UTC(), i.ExpiresAt.UTC()})
	if e != nil {
		return "", e
	}
	return DigestSHA256(b), nil
}
func (i HarnessPairingIntent) Validate() error {
	if strings.TrimSpace(string(i.ID)) == "" || strings.TrimSpace(string(i.ProjectID)) == "" || !i.Kind.Valid() || strings.TrimSpace(string(i.ConnectionID)) == "" || strings.TrimSpace(i.InstallationID) == "" || !i.AdapterDigest.Valid() || strings.TrimSpace(i.HarnessIdentity) == "" || strings.TrimSpace(i.ProviderVersion) == "" || !i.ProtocolFingerprint.Valid() || strings.TrimSpace(i.MissionID) == "" || strings.TrimSpace(i.AppRunID) == "" || i.ExpectedGeneration < 1 || i.CreatedAt.IsZero() || i.UpdatedAt.Before(i.CreatedAt) || !i.ExpiresAt.After(i.CreatedAt) || !i.ConnectionExpiresAt.After(i.CreatedAt) || !i.Status.Valid() || strings.TrimSpace(i.ProposalRequestKey) == "" || !i.ProposalRequestFingerprint.Valid() {
		return ErrHarnessPairingIntentInvalid
	}
	classes, e := NormalizeHarnessCapabilities(i.CapabilityClasses)
	if e != nil || len(classes) != len(i.CapabilityClasses) {
		return ErrHarnessPairingIntentInvalid
	}
	d, e := i.ComputedDigest()
	if e != nil || d != i.Digest {
		return ErrHarnessPairingIntentInvalid
	}
	if i.Status == HarnessPairingIntentActive && i.ChallengeID == nil {
		return ErrHarnessPairingIntentInvalid
	}
	return nil
}

type HarnessAuthorityReceipt struct {
	ID                 string       `json:"id"`
	Action             string       `json:"action"`
	TargetType         string       `json:"targetType"`
	TargetID           string       `json:"targetId"`
	TargetDigest       SHA256Digest `json:"targetDigest"`
	ExpectedGeneration int64        `json:"expectedGeneration,omitempty"`
	RequestKey         string       `json:"requestKey"`
	RequestFingerprint SHA256Digest `json:"requestFingerprint"`
	OwnerPrincipal     string       `json:"ownerPrincipal"`
	ConfirmationRef    string       `json:"confirmationRef"`
	CreatedAt          time.Time    `json:"createdAt"`
}

func (r HarnessAuthorityReceipt) Validate() error {
	if strings.TrimSpace(r.ID) == "" || !map[string]bool{"approve": true, "deny": true, "revoke": true}[r.Action] || !map[string]bool{"pairing_intent": true, "harness_connection": true}[r.TargetType] || strings.TrimSpace(r.TargetID) == "" || !r.TargetDigest.Valid() || strings.TrimSpace(r.RequestKey) == "" || !r.RequestFingerprint.Valid() || strings.TrimSpace(r.OwnerPrincipal) == "" || strings.TrimSpace(r.ConfirmationRef) == "" || r.CreatedAt.IsZero() {
		return ErrHarnessAuthorityInvalid
	}
	return nil
}

var ErrHarnessPairingIntentInvalid = errors.New("invalid harness pairing intent")
var ErrHarnessAuthorityInvalid = errors.New("invalid harness authority decision")
var ErrHarnessAuthorityConflict = errors.New("harness authority decision conflict")
var ErrHarnessAuthorityStale = errors.New("harness authority target is stale")

func (h HarnessConnection) AuthorityDigest() (SHA256Digest, error) {
	classes := append([]HarnessCapabilityClass(nil), h.CapabilityClasses...)
	sort.Slice(classes, func(i, j int) bool { return classes[i] < classes[j] })
	payload := struct {
		Version             string              `json:"version"`
		ID                  HarnessConnectionID `json:"id"`
		InstallationID      string              `json:"installationId"`
		AdapterDigest       SHA256Digest        `json:"adapterDigest"`
		HarnessIdentity     string              `json:"harnessIdentity"`
		ProviderVersion     string              `json:"providerVersion"`
		ProtocolFingerprint SHA256Digest        `json:"protocolFingerprint"`
		MissionID, AppRunID string
		CapabilityClasses   []HarnessCapabilityClass `json:"capabilityClasses"`
		Generation          int64                    `json:"generation"`
		ExpiresAt           time.Time                `json:"expiresAt"`
	}{"v1", h.ID, h.InstallationID, h.AdapterDigest, h.HarnessIdentity, h.ProviderVersion, h.ProtocolFingerprint, h.MissionID, h.AppRunID, classes, h.Generation, h.ExpiresAt.UTC()}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return DigestSHA256(b), nil
}
