package domain

import (
	"testing"
	"time"
)

func TestAttemptStartReservationRequiresAttributedTypedEvidence(t *testing.T) {
	d, _ := NewCapabilityDenialDetail("wu", []string{"worktree.write"}, AdmissionDenialRoutingCandidate, "snap", "gen", ExecutionBinding{Provider: HarnessCodex, ModelSelection: ExecutionBindingModelProviderDefault}, "candidate", HarnessCodex)
	r := AttemptStartReservation{ID: "res", AttemptID: "att", OutcomeID: "out", PlanRevisionID: "plan", WorkUnitID: "wu", ContractRevisionNumber: 1, RunIntentGeneration: 2, RequestKey: "key", RequestFingerprint: "fingerprint", RoutingSnapshotID: "snap", RoutingGenerationID: "gen", AdmissionEvaluationID: "eval", RefusalStatus: AttemptStartRefusalOpen, Denial: d, CreatedAt: time.Now()}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	r.RoutingGenerationID = "other"
	if r.Validate() == nil {
		t.Fatal("mismatched routing evidence passed")
	}
}
