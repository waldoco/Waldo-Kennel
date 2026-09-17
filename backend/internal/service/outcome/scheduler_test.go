package outcome

import (
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

func schedulerPlanFixture() domain.PlanRevision {
	return domain.PlanRevision{
		ID: "plan-scheduler", OutcomeID: "out-scheduler", Number: 1, ContractRevisionNumber: 1,
		Status: domain.PlanStatusApproved, Summary: "A then B", RunBriefCoreDigest: strings.Repeat("a", 64),
		WorkUnits: []domain.WorkUnit{
			{ID: "wu-a", Kind: domain.WorkUnitDirect, Role: domain.WorkUnitRoleImplement, Title: "A", ContractRevisionNumber: 1, Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault, OutputSummary: "A output", EvidenceChecks: []string{"A evidence"}, VerificationRequirement: "verify A", StopConditions: []string{"stop"}, CriterionIDs: []domain.CriterionID{"crit-a"}, RequiredCapabilities: []string{domain.CapabilityWorktreeRead}},
			{ID: "wu-b", Kind: domain.WorkUnitDirect, Role: domain.WorkUnitRoleImplement, Title: "B", ContractRevisionNumber: 1, Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault, OutputSummary: "B output", EvidenceChecks: []string{"B evidence"}, VerificationRequirement: "verify B", StopConditions: []string{"stop"}, DependsOn: []domain.WorkUnitID{"wu-a"}, Inputs: []domain.WorkUnitInput{{FromWorkUnitID: "wu-a", Required: "A output", Position: 1}}, CriterionIDs: []domain.CriterionID{"crit-b"}, RequiredCapabilities: []string{domain.CapabilityWorktreeRead}},
		},
	}
}

func schedulerProofFixture(plan domain.PlanRevision) ProofView {
	return ProofView{
		OutcomeID: plan.OutcomeID,
		Contract: domain.ContractRevision{
			ID: "cr-1", OutcomeID: plan.OutcomeID, Number: plan.ContractRevisionNumber,
			Goal: "A then B", SuccessCriteria: []string{"A", "B"}, Review: "verify",
			Criteria: []domain.ContractCriterion{
				{ID: "crit-a", ContractRevisionID: "cr-1", Position: 1, Text: "A"},
				{ID: "crit-b", ContractRevisionID: "cr-1", Position: 2, Text: "B"},
			},
		},
		Criteria: []CriterionProofView{
			{Criterion: domain.ContractCriterion{ID: "crit-a", ContractRevisionID: "cr-1", Position: 1, Text: "A"}},
			{Criterion: domain.ContractCriterion{ID: "crit-b", ContractRevisionID: "cr-1", Position: 2, Text: "B"}},
		},
	}
}

// retainedArtifactV1 is the artifact version the fixture attempts retained.
// Attempt-scoped proof binds to it, so a later version leaves the same proof
// naming an artifact that no longer exists.
const retainedArtifactV1 = "artifact-v1"

func addProvenAttemptCriterion(proof *ProofView, plan domain.PlanRevision, attempt domain.Attempt, artifactVersion string, criterionID domain.CriterionID, at time.Time) {
	var criterion *CriterionProofView
	for i := range proof.Criteria {
		if proof.Criteria[i].Criterion.ID == criterionID {
			criterion = &proof.Criteria[i]
			break
		}
	}
	if criterion == nil {
		panic("missing criterion fixture")
	}
	evidenceID := domain.EvidenceItemID("ev-" + string(criterionID))
	criterion.Evidence = append(criterion.Evidence, domain.EvidenceItem{
		ID: evidenceID, OutcomeID: plan.OutcomeID, ContractRevisionID: proof.Contract.ID, CriterionID: criterionID,
		SubjectType: domain.ProofSubjectAttempt, SubjectID: string(attempt.ID), SubjectRevision: artifactVersion,
		Kind: domain.EvidenceSupporting, SourceType: domain.EvidenceSourceArtifact, SourceRef: "artifact",
		ProducerType: domain.EvidenceProducerProvider, ProducerRef: string(attempt.ID), Summary: "support",
		ContentDigest: strings.Repeat("b", 64), RequestKey: "ev-key", RequestFingerprint: strings.Repeat("c", 64), CreatedAt: at,
	})
	criterion.Verifications = append(criterion.Verifications, domain.VerificationRun{
		ID: domain.VerificationRunID("ver-" + string(criterionID)), OutcomeID: plan.OutcomeID, ContractRevisionID: proof.Contract.ID, CriterionID: criterionID,
		SubjectType: domain.ProofSubjectAttempt, SubjectID: string(attempt.ID), SubjectRevision: artifactVersion, EvidenceItemIDs: []domain.EvidenceItemID{evidenceID},
		Method: "deterministic", IndependenceClass: domain.VerificationDeterministic, Result: domain.VerificationPassed,
		VerifierRef: "test", RequestKey: "ver-key", RequestFingerprint: strings.Repeat("d", 64), CreatedAt: at.Add(time.Second),
	})
}

func TestNextRunnableWorkUnitProofGatesDependencies(t *testing.T) {
	plan := schedulerPlanFixture()
	proof := schedulerProofFixture(plan)

	unit, ok, err := nextRunnableWorkUnit(plan, nil, proof)
	if err != nil || !ok || unit.ID != "wu-a" {
		t.Fatalf("initial runnable = %s ok=%v err=%v, want wu-a", unit.ID, ok, err)
	}

	attempts := []domain.Attempt{{ID: "att-a", OutcomeID: plan.OutcomeID, PlanRevisionID: plan.ID, WorkUnitID: "wu-a", ContractRevisionNumber: 1, Status: domain.AttemptRunning}}
	if unit, ok, err := nextRunnableWorkUnit(plan, attempts, proof); err != nil || ok {
		t.Fatalf("active A must block another unit: unit=%s ok=%v err=%v", unit.ID, ok, err)
	}

	attempts[0].Status = domain.AttemptReconciled
	unit, ok, err = nextRunnableWorkUnit(plan, attempts, proof)
	if err != nil || !ok || unit.ID != "wu-a" {
		t.Fatalf("unverified reconciled A should remain retryable: unit=%s ok=%v err=%v", unit.ID, ok, err)
	}

	addProvenAttemptCriterion(&proof, plan, attempts[0], retainedArtifactV1, "crit-a", time.Unix(10, 0).UTC())
	unit, ok, err = nextRunnableWorkUnit(plan, attempts, proof)
	if err != nil || !ok || unit.ID != "wu-b" {
		t.Fatalf("canonically proven A should release B: unit=%s ok=%v err=%v", unit.ID, ok, err)
	}
}

func TestNextRunnableWorkUnitRequiresEveryDependencyCriterion(t *testing.T) {
	plan := schedulerPlanFixture()
	plan.WorkUnits[0].CriterionIDs = []domain.CriterionID{"crit-a", "crit-a-2"}
	proof := schedulerProofFixture(plan)
	proof.Contract.Criteria = append(proof.Contract.Criteria, domain.ContractCriterion{ID: "crit-a-2", ContractRevisionID: "cr-1", Position: 3, Text: "A2"})
	proof.Criteria = append(proof.Criteria, CriterionProofView{Criterion: domain.ContractCriterion{ID: "crit-a-2", ContractRevisionID: "cr-1", Position: 3, Text: "A2"}})
	attempts := []domain.Attempt{{ID: "att-a", OutcomeID: plan.OutcomeID, PlanRevisionID: plan.ID, WorkUnitID: "wu-a", ContractRevisionNumber: 1, Status: domain.AttemptReconciled}}

	addProvenAttemptCriterion(&proof, plan, attempts[0], retainedArtifactV1, "crit-a", time.Unix(10, 0).UTC())
	unit, ok, err := nextRunnableWorkUnit(plan, attempts, proof)
	if err != nil || !ok || unit.ID != "wu-a" {
		t.Fatalf("partial proof must not release B: unit=%s ok=%v err=%v", unit.ID, ok, err)
	}
	addProvenAttemptCriterion(&proof, plan, attempts[0], retainedArtifactV1, "crit-a-2", time.Unix(20, 0).UTC())
	unit, ok, err = nextRunnableWorkUnit(plan, attempts, proof)
	if err != nil || !ok || unit.ID != "wu-b" {
		t.Fatalf("complete proof should release B: unit=%s ok=%v err=%v", unit.ID, ok, err)
	}
}

func TestSchedulerRejectsWrongOutcomeStaleContractAndUnrelatedAttemptProof(t *testing.T) {
	plan := schedulerPlanFixture()
	proof := schedulerProofFixture(plan)
	attempt := domain.Attempt{ID: "att-a", OutcomeID: plan.OutcomeID, PlanRevisionID: plan.ID, WorkUnitID: "wu-a", ContractRevisionNumber: 1, Status: domain.AttemptReconciled}
	attempts := []domain.Attempt{attempt}
	addProvenAttemptCriterion(&proof, plan, attempt, retainedArtifactV1, "crit-a", time.Unix(10, 0).UTC())

	wrongOutcome := proof
	wrongOutcome.OutcomeID = "out-other"
	if _, _, err := nextRunnableWorkUnit(plan, attempts, wrongOutcome); err == nil {
		t.Fatal("wrong Outcome proof must be rejected")
	}

	stale := proof
	stale.Contract.Number = 2
	if _, _, err := nextRunnableWorkUnit(plan, attempts, stale); err == nil {
		t.Fatal("stale Contract proof must be rejected")
	}

	unrelated := schedulerProofFixture(plan)
	otherAttempt := attempt
	otherAttempt.ID = "att-other"
	otherAttempt.WorkUnitID = "wu-b"
	addProvenAttemptCriterion(&unrelated, plan, otherAttempt, retainedArtifactV1, "crit-a", time.Unix(10, 0).UTC())
	unit, ok, err := nextRunnableWorkUnit(plan, attempts, unrelated)
	if err != nil || !ok || unit.ID != "wu-a" {
		t.Fatalf("unrelated Attempt proof released dependency: unit=%s ok=%v err=%v", unit.ID, ok, err)
	}
}

func TestSchedulerProofHorizonMakesSupersededProofRetryable(t *testing.T) {
	plan := schedulerPlanFixture()
	proof := schedulerProofFixture(plan)
	attempt := domain.Attempt{ID: "att-a", OutcomeID: plan.OutcomeID, PlanRevisionID: plan.ID, WorkUnitID: "wu-a", ContractRevisionNumber: 1, Status: domain.AttemptReconciled}
	addProvenAttemptCriterion(&proof, plan, attempt, retainedArtifactV1, "crit-a", time.Unix(10, 0).UTC())
	proof.ProofHorizon = time.Unix(100, 0).UTC()

	unit, ok, err := nextRunnableWorkUnit(plan, []domain.Attempt{attempt}, proof)
	if err != nil || !ok || unit.ID != "wu-a" {
		t.Fatalf("superseded proof should make A retryable: unit=%s ok=%v err=%v", unit.ID, ok, err)
	}
}
