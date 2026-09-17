package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

const OwnerProofTargetVersion = "v1"

// OwnerProofTarget is the closed, versioned authority fence for one routine
// command. Every command class has an exact shape; unused fields are rejected.
type OwnerProofTarget struct {
	Version              string            `json:"version"`
	Class                OwnerCommandClass `json:"class"`
	SessionID            string            `json:"sessionId,omitempty"`
	ControllerGeneration string            `json:"controllerGeneration,omitempty"`
	ExpectedRevision     string            `json:"expectedRevision,omitempty"`
	ProviderTurnID       string            `json:"providerTurnId,omitempty"`
	QuestionID           string            `json:"questionId,omitempty"`
	QuestionGeneration   string            `json:"questionGeneration,omitempty"`
}

func (t OwnerProofTarget) Validate() error {
	if t.Version != OwnerProofTargetVersion || !t.Class.Valid() || t.Class.Material() {
		return ErrOwnerProofInvalid
	}
	required := func(v ...string) bool {
		for _, x := range v {
			if strings.TrimSpace(x) == "" {
				return false
			}
		}
		return true
	}
	switch t.Class {
	case OwnerCommandTurn:
		if !required(t.SessionID, t.ControllerGeneration, t.ExpectedRevision) || t.ProviderTurnID != "" || t.QuestionID != "" || t.QuestionGeneration != "" {
			return ErrOwnerProofInvalid
		}
	case OwnerCommandSteer:
		if !required(t.SessionID, t.ControllerGeneration, t.ExpectedRevision, t.ProviderTurnID) || t.QuestionID != "" || t.QuestionGeneration != "" {
			return ErrOwnerProofInvalid
		}
	case OwnerCommandAnswer:
		if !required(t.QuestionID, t.QuestionGeneration) || t.SessionID != "" || t.ControllerGeneration != "" || t.ExpectedRevision != "" || t.ProviderTurnID != "" {
			return ErrOwnerProofInvalid
		}
	case OwnerCommandInterrupt:
		if !required(t.SessionID, t.ControllerGeneration, t.ProviderTurnID) || t.ExpectedRevision != "" || t.QuestionID != "" || t.QuestionGeneration != "" {
			return ErrOwnerProofInvalid
		}
	case OwnerCommandCancel:
		// Cancel remains represented in the closed vocabulary, but has no reviewed
		// target shape yet; accepting it here would alias two distinct effects.
		return ErrOwnerProofInvalid
	default:
		return ErrOwnerProofInvalid
	}
	return nil
}

func (t OwnerProofTarget) CanonicalBytes() ([]byte, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	b, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("encode owner proof target: %w", err)
	}
	return b, nil
}

func (t OwnerProofTarget) Digest() (SHA256Digest, error) {
	b, err := t.CanonicalBytes()
	if err != nil {
		return "", err
	}
	return DigestSHA256(b), nil
}
