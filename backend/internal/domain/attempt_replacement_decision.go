package domain

import (
	"fmt"
	"strings"
	"time"
)

type AttemptReplacementDecisionID string
type AttemptReplacementDecision struct {
	ID                     AttemptReplacementDecisionID `json:"id"`
	OutcomeID              OutcomeID                    `json:"outcomeId"`
	PredecessorAttemptID   AttemptID                    `json:"predecessorAttemptId"`
	PlanRevisionID         PlanRevisionID               `json:"planRevisionId"`
	WorkUnitID             WorkUnitID                   `json:"workUnitId"`
	RunIntentGeneration    int64                        `json:"runIntentGeneration"`
	ContractRevisionNumber int64                        `json:"contractRevisionNumber"`
	Action                 string                       `json:"action"`
	RequestKey             string                       `json:"requestKey"`
	RequestFingerprint     string                       `json:"requestFingerprint"`
	OwnerPrincipal         string                       `json:"ownerPrincipal"`
	CreatedAt              time.Time                    `json:"createdAt"`
}

func (d AttemptReplacementDecision) Validate() error {
	if strings.TrimSpace(string(d.ID)) == "" || d.OutcomeID.IsZero() || d.PredecessorAttemptID.IsZero() || d.PlanRevisionID.IsZero() || d.WorkUnitID.IsZero() || d.RunIntentGeneration < 1 || d.ContractRevisionNumber < 1 || d.Action != "replace" || strings.TrimSpace(d.RequestKey) == "" || !isLowerHexDigest(d.RequestFingerprint) || !validLocalOwnerPrincipal(d.OwnerPrincipal) || d.CreatedAt.IsZero() {
		return fmt.Errorf("replacement decision requires exact immutable command binding")
	}
	return nil
}

func validLocalOwnerPrincipal(v string) bool {
	const prefix = "local-owner:apprun-"
	if !strings.HasPrefix(v, prefix) || len(v) <= len(prefix) || len(v) > len("local-owner:")+128 {
		return false
	}
	for _, r := range v[len(prefix):] {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-' {
			return false
		}
	}
	return true
}

func isLowerHexDigest(v string) bool {
	if len(v) != 64 {
		return false
	}
	for _, r := range v {
		if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
