package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type CommandAuthorityClaimState string

const (
	CommandAuthorityClaimPending      CommandAuthorityClaimState = "pending"
	CommandAuthorityClaimActionNeeded CommandAuthorityClaimState = "action_needed"
)

// CanonicalHarnessCommand is the complete immutable semantic payload dispatched
// after authority validation. The closed class-specific fields avoid opaque JSON.
type CanonicalHarnessCommand struct {
	Version             string            `json:"version"`
	Class               OwnerCommandClass `json:"class"`
	Text                string            `json:"text,omitempty"`
	DeliveryContentJSON string            `json:"deliveryContentJson,omitempty"`
	ClientMessageID     string            `json:"clientMessageId,omitempty"`
	DecisionJSON        string            `json:"decisionJson,omitempty"`
}

func (c CanonicalHarnessCommand) Bytes() ([]byte, error) {
	if c.Version != "v1" || !c.Class.Valid() || c.Class.Material() {
		return nil, ErrHarnessCommandInvalid
	}
	switch c.Class {
	case OwnerCommandTurn, OwnerCommandSteer:
		if strings.TrimSpace(c.Text) == "" || strings.TrimSpace(c.ClientMessageID) == "" || c.DecisionJSON != "" {
			return nil, ErrHarnessCommandInvalid
		}
	case OwnerCommandAnswer:
		if strings.TrimSpace(c.DecisionJSON) == "" || c.Text != "" || c.DeliveryContentJSON != "" || c.ClientMessageID != "" || !json.Valid([]byte(c.DecisionJSON)) {
			return nil, ErrHarnessCommandInvalid
		}
	case OwnerCommandInterrupt:
		if c.Text != "" || c.DeliveryContentJSON != "" || c.ClientMessageID != "" || c.DecisionJSON != "" {
			return nil, ErrHarnessCommandInvalid
		}
	default:
		return nil, ErrHarnessCommandInvalid
	}
	return json.Marshal(c)
}

type HarnessCommandOutboxRecord struct {
	ClaimID, DestinationType, DestinationID string
	CanonicalPayload                        []byte
	State                                   CommandAuthorityClaimState
	CreatedAt, UpdatedAt                    time.Time
}

type CommandAuthorityClaim struct {
	ID, AdapterRequestKey, RequestFingerprint string
	OwnerProofID                              OwnerProofID
	ConnectionID                              HarnessConnectionID
	ConnectionGeneration                      int64
	ConnectionBindingDigest                   SHA256Digest
	TransportClass                            HarnessCapabilityClass
	AppRunID, MissionID                       string
	ContentDigest, TargetDigest               SHA256Digest
	OwnerClass                                OwnerCommandClass
	CanonicalVersion                          string
	CanonicalPayload                          []byte
	DestinationType, DestinationID            string
	State                                     CommandAuthorityClaimState
	CreatedAt, UpdatedAt                      time.Time
}

func (c CommandAuthorityClaim) Validate() error {
	if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.AdapterRequestKey) == "" || !c.ContentDigest.Valid() || !c.TargetDigest.Valid() || !c.ConnectionBindingDigest.Valid() || c.ConnectionGeneration < 1 || !c.TransportClass.Valid() || !c.OwnerClass.Valid() || c.OwnerClass.Material() || strings.TrimSpace(c.AppRunID) == "" || strings.TrimSpace(c.MissionID) == "" || strings.TrimSpace(c.DestinationType) == "" || strings.TrimSpace(c.DestinationID) == "" || len(c.CanonicalPayload) == 0 || c.CanonicalVersion != "v1" || c.CreatedAt.IsZero() {
		return ErrHarnessCommandInvalid
	}
	return nil
}

var ErrHarnessCommandInvalid = errors.New("invalid harness command")
var ErrHarnessCommandAuthentication = errors.New("harness command authentication failed")
var ErrHarnessCommandConflict = errors.New("harness command conflict")
