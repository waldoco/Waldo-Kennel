package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// GovernedControlClass identifies control effects applied to an existing
// provider turn or request. Turn dispatch remains in governed_commands.
type GovernedControlClass string

const (
	GovernedControlSteer     GovernedControlClass = "steer"
	GovernedControlAnswer    GovernedControlClass = "answer"
	GovernedControlInterrupt GovernedControlClass = "interrupt"
)

// GovernedControlCommand is an additive claim for an effect that does not open
// a conversation turn. RequestInstanceID is Kennel's durable activity identity
// for the exact request card being answered. It deliberately has no provider
// wire counterpart. Steer/interrupt instead target ProviderTurnID.
type GovernedControlCommand struct {
	ID, IdempotencyKey, RequestFingerprint string
	Class                                  GovernedControlClass
	State                                  GovernedCommandState
	SessionID                              SessionID
	ControllerGeneration, ExpectedRevision string
	CapabilityFingerprint                  string
	ProviderConversationID                 string
	ClientMessageID, ProviderTurnID        string
	RequestInstanceID                      string
	Quiescence                             GovernedCommandQuiescence
	QuiescenceEvidenceRef                  string
	CreatedAt, UpdatedAt                   time.Time
}

func (c GovernedControlCommand) Validate() error {
	if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.IdempotencyKey) == "" ||
		strings.TrimSpace(c.RequestFingerprint) == "" || c.SessionID == "" ||
		strings.TrimSpace(c.ControllerGeneration) == "" || strings.TrimSpace(c.ExpectedRevision) == "" ||
		strings.TrimSpace(c.CapabilityFingerprint) == "" || strings.TrimSpace(c.ProviderConversationID) == "" {
		return fmt.Errorf("%w: control identity and fences are required", ErrGovernedCommandInvalid)
	}
	if !validGovernedCommandState(c.State) {
		return fmt.Errorf("%w: unknown control state %q", ErrGovernedCommandInvalid, c.State)
	}
	switch c.Class {
	case GovernedControlSteer:
		if strings.TrimSpace(c.ClientMessageID) == "" || strings.TrimSpace(c.ProviderTurnID) == "" || c.RequestInstanceID != "" {
			return fmt.Errorf("%w: steer requires client message and provider turn only", ErrGovernedCommandInvalid)
		}
	case GovernedControlAnswer:
		if strings.TrimSpace(c.RequestInstanceID) == "" || c.ClientMessageID != "" || c.ProviderTurnID != "" {
			return fmt.Errorf("%w: answer requires exact durable request instance only", ErrGovernedCommandInvalid)
		}
	case GovernedControlInterrupt:
		if strings.TrimSpace(c.ProviderTurnID) == "" || c.ClientMessageID != "" || c.RequestInstanceID != "" {
			return fmt.Errorf("%w: interrupt requires exact provider turn only", ErrGovernedCommandInvalid)
		}
	default:
		return fmt.Errorf("%w: unsupported control class %q", ErrGovernedCommandInvalid, c.Class)
	}
	if c.Quiescence == GovernedCommandQuiescenceCodexTree {
		if strings.TrimSpace(c.QuiescenceEvidenceRef) == "" {
			return fmt.Errorf("%w: verified quiescence requires evidence", ErrGovernedCommandInvalid)
		}
	} else if c.Quiescence != GovernedCommandQuiescenceNotApplicable && c.Quiescence != GovernedCommandQuiescencePending {
		return fmt.Errorf("%w: unknown quiescence %q", ErrGovernedCommandInvalid, c.Quiescence)
	} else if c.QuiescenceEvidenceRef != "" {
		return fmt.Errorf("%w: unverified quiescence cannot carry evidence", ErrGovernedCommandInvalid)
	}
	return nil
}

func ComputeGovernedControlFingerprint(session SessionID, class GovernedControlClass, key, target, payloadJSON string) string {
	payload, _ := json.Marshal(struct {
		Session              SessionID            `json:"session"`
		Class                GovernedControlClass `json:"class"`
		Key, Target, Payload string
	}{session, class, strings.TrimSpace(key), strings.TrimSpace(target), payloadJSON})
	sum := sha256.Sum256(payload)
	return "v1:" + hex.EncodeToString(sum[:])
}
