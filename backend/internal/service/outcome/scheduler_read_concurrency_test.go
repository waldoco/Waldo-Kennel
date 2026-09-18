package outcome

import (
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// schedulerReadOnlyPlanFixture gives ADR 0009 §6 concurrent read-only
// admission its own dedicated fixture, separate from schedulerPlanFixture
// (which represents the pre-existing serialized write model). wu-r1..wu-r4
// are independent read-only WorkUnits; wu-w is write-capable and independent
// of all of them, so custody blocking can be observed without conflating it
// with dependency-proof blocking; wu-unclassified has no RequiredCapabilities
// at all, to prove the fail-closed default.
func schedulerReadOnlyPlanFixture() domain.PlanRevision {
	readOnlyUnit := func(id domain.WorkUnitID) domain.WorkUnit {
		return domain.WorkUnit{
			ID: id, Kind: domain.WorkUnitDirect, Title: string(id),
			ContractRevisionNumber: 1, Provider: domain.HarnessCodex,
			ModelSelection: domain.ExecutionBindingModelProviderDefault,
			OutputSummary:  string(id) + " output", EvidenceChecks: []string{string(id) + " evidence"},
			VerificationRequirement: "verify " + string(id), StopConditions: []string{"stop"},
			RequiredCapabilities: []string{domain.CapabilityWorktreeRead},
		}
	}
	return domain.PlanRevision{
		ID: "plan-read-concurrency", OutcomeID: "out-read-concurrency", Number: 1, ContractRevisionNumber: 1,
		Status: domain.PlanStatusApproved, Summary: "independent reads plus one write",
		WorkUnits: []domain.WorkUnit{
			readOnlyUnit("wu-r1"), readOnlyUnit("wu-r2"), readOnlyUnit("wu-r3"), readOnlyUnit("wu-r4"),
			{
				ID: "wu-w", Kind: domain.WorkUnitDirect, Title: "W", ContractRevisionNumber: 1, Provider: domain.HarnessCodex,
				ModelSelection: domain.ExecutionBindingModelProviderDefault, OutputSummary: "W output",
				EvidenceChecks: []string{"W evidence"}, VerificationRequirement: "verify W", StopConditions: []string{"stop"},
				RequiredCapabilities: []string{domain.CapabilityWorktreeRead, domain.CapabilityWorktreeWrite},
			},
			{
				ID: "wu-unclassified", Kind: domain.WorkUnitDirect, Title: "U", ContractRevisionNumber: 1, Provider: domain.HarnessCodex,
				ModelSelection: domain.ExecutionBindingModelProviderDefault, OutputSummary: "U output",
				EvidenceChecks: []string{"U evidence"}, VerificationRequirement: "verify U", StopConditions: []string{"stop"},
			},
		},
	}
}

func schedulerReadOnlyProofFixture(plan domain.PlanRevision) ProofView {
	return ProofView{
		OutcomeID: plan.OutcomeID,
		Contract:  domain.ContractRevision{ID: "cr-read", OutcomeID: plan.OutcomeID, Number: plan.ContractRevisionNumber, Goal: "reads", Review: "verify"},
	}
}

func readOnlyAttempt(id domain.AttemptID, unit domain.WorkUnitID, status domain.AttemptStatus) domain.Attempt {
	return domain.Attempt{ID: id, OutcomeID: "out-read-concurrency", PlanRevisionID: "plan-read-concurrency", WorkUnitID: unit, ContractRevisionNumber: 1, Status: status}
}

func TestAdmissibleWorkUnitsAllowsConcurrentReadOnlySiblings(t *testing.T) {
	plan := schedulerReadOnlyPlanFixture()
	proof := schedulerReadOnlyProofFixture(plan)

	admissible, err := admissibleWorkUnits(plan, nil, proof)
	if err != nil {
		t.Fatalf("admissibleWorkUnits error = %v", err)
	}
	ids := map[domain.WorkUnitID]bool{}
	for _, unit := range admissible {
		ids[unit.ID] = true
	}
	for _, want := range []domain.WorkUnitID{"wu-r1", "wu-r2", "wu-r3", "wu-r4", "wu-w"} {
		if !ids[want] {
			t.Fatalf("admissible = %v, want %s included with nothing active", admissible, want)
		}
	}

	// wu-r1 is now active: its read-only sibling wu-r2 must still be
	// independently admissible (this is the ADR 0009 §6 guarantee), while the
	// write-capable wu-w must NOT be admissible until reads finish.
	attempts := []domain.Attempt{readOnlyAttempt("att-r1", "wu-r1", domain.AttemptRunning)}
	admissible, err = admissibleWorkUnits(plan, attempts, proof)
	if err != nil {
		t.Fatalf("admissibleWorkUnits error = %v", err)
	}
	ids = map[domain.WorkUnitID]bool{}
	for _, unit := range admissible {
		ids[unit.ID] = true
	}
	if !ids["wu-r2"] {
		t.Fatalf("admissible = %v, want wu-r2 admissible alongside active wu-r1", admissible)
	}
	if ids["wu-w"] {
		t.Fatalf("admissible = %v, want wu-w blocked while a read-only Attempt is active", admissible)
	}
	if ids["wu-r1"] {
		t.Fatalf("admissible = %v, want wu-r1 excluded: it already has an active attempt", admissible)
	}
}

func TestAdmissibleWorkUnitsBlocksReadsWhileWriteActive(t *testing.T) {
	plan := schedulerReadOnlyPlanFixture()
	proof := schedulerReadOnlyProofFixture(plan)
	attempts := []domain.Attempt{readOnlyAttempt("att-w", "wu-w", domain.AttemptRunning)}

	admissible, err := admissibleWorkUnits(plan, attempts, proof)
	if err != nil {
		t.Fatalf("admissibleWorkUnits error = %v", err)
	}
	if len(admissible) != 0 {
		t.Fatalf("admissible = %v, want none while a write Attempt is active", admissible)
	}
}

func TestAdmissibleWorkUnitsEnforcesReadConcurrencyBudget(t *testing.T) {
	plan := schedulerReadOnlyPlanFixture()
	proof := schedulerReadOnlyProofFixture(plan)
	if MaxConcurrentReadOnlyAttempts != 3 {
		t.Fatalf("test assumes MaxConcurrentReadOnlyAttempts == 3, got %d", MaxConcurrentReadOnlyAttempts)
	}
	attempts := []domain.Attempt{
		readOnlyAttempt("att-r1", "wu-r1", domain.AttemptRunning),
		readOnlyAttempt("att-r2", "wu-r2", domain.AttemptRunning),
		readOnlyAttempt("att-r3", "wu-r3", domain.AttemptRunning),
	}
	admissible, err := admissibleWorkUnits(plan, attempts, proof)
	if err != nil {
		t.Fatalf("admissibleWorkUnits error = %v", err)
	}
	for _, unit := range admissible {
		if unit.ID == "wu-r4" {
			t.Fatalf("admissible = %v, want wu-r4 blocked once the read concurrency budget is spent", admissible)
		}
	}

	view, err := deriveSchedule(plan, attempts, proof, nil)
	if err != nil {
		t.Fatalf("deriveSchedule error = %v", err)
	}
	r4 := unitState(t, view, "wu-r4")
	if r4.State != WorkUnitScheduleBlocked || r4.BlockedReason != BlockedReadConcurrencyBudget {
		t.Fatalf("wu-r4 = %s/%s, want blocked on the read concurrency budget", r4.State, r4.BlockedReason)
	}
}

func TestAdmissibleWorkUnitsTreatsUnclassifiedAsExclusive(t *testing.T) {
	plan := schedulerReadOnlyPlanFixture()
	proof := schedulerReadOnlyProofFixture(plan)
	attempts := []domain.Attempt{readOnlyAttempt("att-u", "wu-unclassified", domain.AttemptRunning)}

	admissible, err := admissibleWorkUnits(plan, attempts, proof)
	if err != nil {
		t.Fatalf("admissibleWorkUnits error = %v", err)
	}
	if len(admissible) != 0 {
		t.Fatalf("admissible = %v, want none: an unclassified active WorkUnit must be treated as exclusive", admissible)
	}
}

func TestDeriveScheduleReportsConcurrentReadsWithoutASingleCustodyHolder(t *testing.T) {
	plan := schedulerReadOnlyPlanFixture()
	proof := schedulerReadOnlyProofFixture(plan)
	attempts := []domain.Attempt{
		readOnlyAttempt("att-r1", "wu-r1", domain.AttemptRunning),
		readOnlyAttempt("att-r2", "wu-r2", domain.AttemptRunning),
	}
	view, err := deriveSchedule(plan, attempts, proof, nil)
	if err != nil {
		t.Fatalf("deriveSchedule error = %v", err)
	}
	if unitState(t, view, "wu-r1").State != WorkUnitScheduleExecuting {
		t.Fatalf("wu-r1 = %s, want executing", unitState(t, view, "wu-r1").State)
	}
	if unitState(t, view, "wu-r2").State != WorkUnitScheduleExecuting {
		t.Fatalf("wu-r2 = %s, want executing", unitState(t, view, "wu-r2").State)
	}
	// No single exclusive custody holder exists while only read-only
	// Attempts are active; the singular field stays empty rather than
	// falsely naming one of several concurrent holders.
	if !view.CustodyHeldBy.IsZero() {
		t.Fatalf("custodyHeldBy = %q, want empty while only concurrent read-only Attempts are active", view.CustodyHeldBy)
	}
	w := unitState(t, view, "wu-w")
	if w.State != WorkUnitScheduleBlocked || w.BlockedReason != BlockedCustodyHeld {
		t.Fatalf("wu-w = %s/%s, want blocked/custody_held while read-only Attempts are active", w.State, w.BlockedReason)
	}
	// The read concurrency budget (3) is not yet spent (only 2 reads active),
	// so a third independent read-only unit remains genuinely runnable and
	// the plan still reports something admissible.
	if view.NextRunnableID != "wu-r3" {
		t.Fatalf("nextRunnableId = %q, want wu-r3 still runnable under budget", view.NextRunnableID)
	}
	if view.NoRunnableReason != "" {
		t.Fatalf("noRunnableReason = %q, want empty while wu-r3 is still runnable", view.NoRunnableReason)
	}
}
