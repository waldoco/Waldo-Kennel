package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const CapabilityEscalationVersion = "capability-escalation-v1"
const (
	CapabilityEscalationGrantOnce     = "grant_once"
	CapabilityEscalationWidenContract = "widen_contract"
	CapabilityEscalationDeny          = "deny"
)

type CapabilityEscalation struct {
	Version                string              `json:"version"`
	OutcomeID              OutcomeID           `json:"outcomeId"`
	ContractRevisionNumber int64               `json:"contractRevisionNumber"`
	PlanRevisionID         PlanRevisionID      `json:"planRevisionId"`
	WorkUnitID             WorkUnitID          `json:"workUnitId"`
	AttemptID              AttemptID           `json:"attemptId"`
	AttemptGeneration      int64               `json:"attemptGeneration"`
	SessionID              SessionID           `json:"sessionId"`
	SessionGeneration      int64               `json:"sessionGeneration"`
	ExecutorKind           string              `json:"executorKind"`
	AttemptSessionRefID    AttemptSessionRefID `json:"attemptSessionRefId"`
	RuntimeLaunchID        string              `json:"runtimeLaunchId,omitempty"`
	ControllerGeneration   string              `json:"controllerGeneration,omitempty"`
	PolicyDigest           string              `json:"policyDigest"`
	ArtifactVersion        string              `json:"artifactVersion,omitempty"`
	CheckID                ApprovedCheckID     `json:"checkId,omitempty"`
	RequestedCapability    string              `json:"requestedCapability"`
	DenialSource           string              `json:"denialSource"`
	GrantFingerprint       string              `json:"grantFingerprint"`
	OperationID            string              `json:"operationId"`
	RequestFingerprint     string              `json:"requestFingerprint"`
	QuestionGeneration     string              `json:"questionGeneration"`
	WithinContractCeiling  bool                `json:"withinContractCeiling"`
	Digest                 string              `json:"digest"`
}

func (e CapabilityEscalation) canonicalBytes() ([]byte, error) {
	clone := e
	clone.Digest = ""
	return json.Marshal(clone)
}
func (e CapabilityEscalation) ComputedDigest() (string, error) {
	b, err := e.canonicalBytes()
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
func (e CapabilityEscalation) Validate() error {
	if e.Version != CapabilityEscalationVersion {
		return fmt.Errorf("unsupported capability escalation version")
	}
	if e.OutcomeID.IsZero() || e.PlanRevisionID.IsZero() || e.WorkUnitID.IsZero() || e.AttemptID.IsZero() {
		return fmt.Errorf("capability escalation lineage is incomplete")
	}
	if e.ContractRevisionNumber < 1 || e.AttemptGeneration < 1 || e.SessionGeneration < 1 {
		return fmt.Errorf("capability escalation generation is incomplete")
	}
	for n, v := range map[string]string{"executor kind": e.ExecutorKind, "session": string(e.SessionID), "session ref": string(e.AttemptSessionRefID), "policy digest": e.PolicyDigest, "capability": e.RequestedCapability, "denial source": e.DenialSource, "grant fingerprint": e.GrantFingerprint, "operation": e.OperationID, "request fingerprint": e.RequestFingerprint, "question generation": e.QuestionGeneration} {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("%s is required", n)
		}
	}
	if e.ExecutorKind == "governed_tool" && strings.TrimSpace(e.RuntimeLaunchID) == "" {
		return fmt.Errorf("governed tool runtime launch is required")
	}
	if e.ExecutorKind == "chat" && strings.TrimSpace(e.ControllerGeneration) == "" {
		return fmt.Errorf("chat controller generation is required")
	}
	got, err := e.ComputedDigest()
	if err != nil || got != e.Digest {
		return fmt.Errorf("capability escalation digest mismatch")
	}
	return nil
}
func CapabilityEscalationOptions() []NeedsYouOption {
	return CapabilityEscalationOptionsFor("governed_tool")
}
func CapabilityEscalationOptionsFor(executor string) []NeedsYouOption {
	if executor == "governed_check" {
		return []NeedsYouOption{{ID: CapabilityEscalationWidenContract, Label: "Widen contract"}, {ID: CapabilityEscalationDeny, Label: "Deny"}}
	}
	return []NeedsYouOption{{ID: CapabilityEscalationGrantOnce, Label: "Grant once"}, {ID: CapabilityEscalationWidenContract, Label: "Widen contract"}, {ID: CapabilityEscalationDeny, Label: "Deny"}}
}

type CapabilityEscalationConsequence string

const (
	CapabilityConsequenceGrantOnce         CapabilityEscalationConsequence = "grant_once"
	CapabilityConsequenceRevisionRequested CapabilityEscalationConsequence = "revision_requested"
	CapabilityConsequenceDenied            CapabilityEscalationConsequence = "denied"
)

func (c CapabilityEscalationConsequence) Valid() bool {
	return c == CapabilityConsequenceGrantOnce || c == CapabilityConsequenceRevisionRequested || c == CapabilityConsequenceDenied
}

type CapabilityEscalationReceipt struct {
	ID                   string
	EscalationDigest     string
	QuestionID           string
	QuestionGeneration   string
	Consequence          CapabilityEscalationConsequence
	AttemptID            AttemptID
	AttemptGeneration    int64
	SessionID            SessionID
	SessionGeneration    int64
	ControllerGeneration string
	Capability           string
	OperationID          string
	RequestFingerprint   string
	GrantFingerprint     string
	AnswerRequestKey     string
	CreatedAt            time.Time
	ConsumedAt           *time.Time
}

func ContractAllowsCapability(revision ContractRevision, capability string) bool {
	switch capability {
	case CapabilityWorktreeRead:
		return revision.AuthorityCeiling.ReadWorkspace || revision.AuthorityCeiling.WriteWorkspace || revision.AuthorityCeiling.ExecuteLocal
	case CapabilityWorktreeWrite:
		return revision.AuthorityCeiling.WriteWorkspace
	case CapabilityWorktreeExec:
		return revision.AuthorityCeiling.ExecuteLocal
	default:
		return false
	}
}
