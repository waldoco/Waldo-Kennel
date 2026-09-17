package domain

import (
	"fmt"
	"strings"
	"time"
)

type AttemptStartRefusalStatus string

const (
	AttemptStartRefusalOpen       AttemptStartRefusalStatus = "open"
	AttemptStartRefusalSuperseded AttemptStartRefusalStatus = "superseded"
	AttemptStartRefusalDenied     AttemptStartRefusalStatus = "denied"
	AttemptStartRefusalAdmitted   AttemptStartRefusalStatus = "admitted"
)

func (s AttemptStartRefusalStatus) Valid() bool {
	return s == AttemptStartRefusalOpen || s == AttemptStartRefusalSuperseded || s == AttemptStartRefusalDenied || s == AttemptStartRefusalAdmitted
}

type AttemptStartReservation struct {
	ID                     string                    `json:"id"`
	AttemptID              AttemptID                 `json:"attemptId"`
	OutcomeID              OutcomeID                 `json:"outcomeId"`
	PlanRevisionID         PlanRevisionID            `json:"planRevisionId"`
	WorkUnitID             WorkUnitID                `json:"workUnitId"`
	ContractRevisionNumber int64                     `json:"contractRevisionNumber"`
	RunIntentGeneration    int64                     `json:"runIntentGeneration"`
	RequestKey             string                    `json:"requestKey"`
	RequestFingerprint     string                    `json:"requestFingerprint"`
	RoutingSnapshotID      string                    `json:"routingSnapshotId"`
	RoutingGenerationID    string                    `json:"routingGenerationId"`
	AdmissionEvaluationID  string                    `json:"admissionEvaluationId"`
	RefusalStatus          AttemptStartRefusalStatus `json:"refusalStatus"`
	Denial                 CapabilityDenialDetail    `json:"denial"`
	CreatedAt              time.Time                 `json:"createdAt"`
}

func (r AttemptStartReservation) Validate() error {
	if strings.TrimSpace(r.ID) == "" || r.AttemptID.IsZero() || r.OutcomeID.IsZero() || r.PlanRevisionID.IsZero() || r.WorkUnitID.IsZero() || r.ContractRevisionNumber < 1 || r.RunIntentGeneration < 0 || strings.TrimSpace(r.RequestKey) == "" || strings.TrimSpace(r.RequestFingerprint) == "" || strings.TrimSpace(r.RoutingSnapshotID) == "" || strings.TrimSpace(r.RoutingGenerationID) == "" || strings.TrimSpace(r.AdmissionEvaluationID) == "" || r.CreatedAt.IsZero() {
		return fmt.Errorf("attempt start reservation identity is incomplete")
	}
	if !r.RefusalStatus.Valid() {
		return fmt.Errorf("invalid refusal status %q", r.RefusalStatus)
	}
	if r.RefusalStatus == AttemptStartRefusalOpen {
		if err := r.Denial.Validate(); err != nil {
			return err
		}
		if r.Denial.WorkUnitID != r.WorkUnitID || r.Denial.RoutingSnapshotID != r.RoutingSnapshotID || r.Denial.RoutingGenerationID != r.RoutingGenerationID {
			return fmt.Errorf("refusal evidence attribution mismatch")
		}
	}
	return nil
}
