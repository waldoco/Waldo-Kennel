package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ExecutionBudgetSource string

const (
	ExecutionBudgetPolicyDefault ExecutionBudgetSource = "policy_default"
	ExecutionBudgetUserOverride  ExecutionBudgetSource = "user_override"
)

type TokenAccountingState string

const (
	TokenAccountingEnforced    TokenAccountingState = "enforced"
	TokenAccountingUnsupported TokenAccountingState = "unsupported"
)

type ExecutionBudget struct {
	WallTimeLimit   time.Duration         `json:"wallTimeLimit"`
	RetryLimit      int                   `json:"retryLimit"`
	TokenLimit      int64                 `json:"tokenLimit,omitempty"`
	TokenAccounting TokenAccountingState  `json:"tokenAccounting"`
	Source          ExecutionBudgetSource `json:"source"`
	PolicyID        string                `json:"policyId"`
	PolicyVersion   string                `json:"policyVersion"`
	PolicyDigest    string                `json:"policyDigest"`
}

func (b ExecutionBudget) Validate() error {
	if b.WallTimeLimit <= 0 {
		return fmt.Errorf("wall time limit is required")
	}
	if b.RetryLimit < 0 {
		return fmt.Errorf("retry limit cannot be negative")
	}
	if strings.TrimSpace(b.PolicyID) == "" || strings.TrimSpace(b.PolicyVersion) == "" || strings.TrimSpace(b.PolicyDigest) == "" {
		return fmt.Errorf("budget policy identity is required")
	}
	if b.Source != ExecutionBudgetPolicyDefault && b.Source != ExecutionBudgetUserOverride {
		return fmt.Errorf("budget source is invalid")
	}
	switch b.TokenAccounting {
	case TokenAccountingEnforced:
		if b.TokenLimit <= 0 {
			return fmt.Errorf("enforced token accounting requires a limit")
		}
	case TokenAccountingUnsupported:
		if b.TokenLimit != 0 {
			return fmt.Errorf("unsupported token accounting cannot claim a limit")
		}
	default:
		return fmt.Errorf("token accounting state is required")
	}
	return nil
}

type AdmissionPolicy struct {
	ID          string          `json:"id"`
	Version     string          `json:"version"`
	Digest      string          `json:"digest"`
	Default     ExecutionBudget `json:"default"`
	MaxWallTime time.Duration   `json:"maxWallTime"`
	MaxRetries  int             `json:"maxRetries"`
	MaxTokens   int64           `json:"maxTokens"`
}

func (p AdmissionPolicy) ComputedDigest() (string, error) {
	p.Digest = ""
	p.Default.PolicyDigest = ""
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return string(DigestSHA256(raw)), nil
}

func (p AdmissionPolicy) Validate() error {
	want, err := p.ComputedDigest()
	if err != nil {
		return err
	}
	if p.Digest != want {
		return fmt.Errorf("admission policy digest mismatch")
	}
	if err := p.Default.Validate(); err != nil {
		return err
	}
	if p.Default.Source != ExecutionBudgetPolicyDefault {
		return fmt.Errorf("default budget provenance is invalid")
	}
	if p.Default.PolicyID != p.ID || p.Default.PolicyVersion != p.Version || p.Default.PolicyDigest != p.Digest {
		return fmt.Errorf("default budget policy identity mismatch")
	}
	if p.MaxWallTime <= 0 || p.MaxRetries < 0 {
		return fmt.Errorf("budget policy ceilings are required")
	}
	return nil
}
