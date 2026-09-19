package domain_test

import (
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

func stalenessPlan() domain.PlanRevision {
	return domain.PlanRevision{
		ID: "plan-1", OutcomeID: "out-1", Number: 1, ContractRevisionNumber: 1,
		Status: domain.PlanStatusApproved,
		WorkUnits: []domain.WorkUnit{
			{ID: "wu-a", Kind: domain.WorkUnitDirect, Title: "A", ContractRevisionNumber: 1},
			{ID: "wu-b", Kind: domain.WorkUnitDirect, Title: "B", ContractRevisionNumber: 1, DependsOn: []domain.WorkUnitID{"wu-a"}},
			{ID: "wu-c", Kind: domain.WorkUnitDirect, Title: "C", ContractRevisionNumber: 1, DependsOn: []domain.WorkUnitID{"wu-b"}},
		},
	}
}

func stalenessAttempt(id domain.AttemptID, unit domain.WorkUnitID, number int64) domain.Attempt {
	return domain.Attempt{ID: id, OutcomeID: "out-1", PlanRevisionID: "plan-1", WorkUnitID: unit, ContractRevisionNumber: 1, Number: number}
}

func retainedReceipt(attemptID domain.AttemptID, unit domain.WorkUnitID, version string) domain.AttemptReceipt {
	at := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	return domain.AttemptReceipt{
		AttemptID: attemptID, OutcomeID: "out-1", PlanRevisionID: "plan-1", WorkUnitID: unit,
		ContractRevisionNumber: 1, ArtifactVersion: version, RetentionState: domain.RetentionRetained,
		ObservedAt: at, CreatedAt: at, UpdatedAt: at,
	}
}

func inputRef(attemptID domain.AttemptID, unit domain.WorkUnitID, version string) domain.AttemptManifestInputRef {
	return domain.AttemptManifestInputRef{AttemptID: attemptID, WorkUnitID: unit, ArtifactVersion: version}
}

func TestDeriveLineageStalenessMarksSupersededInputsAndPropagates(t *testing.T) {
	plan := stalenessPlan()
	attempts := []domain.Attempt{
		stalenessAttempt("a1", "wu-a", 1), stalenessAttempt("a2", "wu-a", 2),
		stalenessAttempt("b1", "wu-b", 1), stalenessAttempt("c1", "wu-c", 1),
	}
	receipts := map[domain.AttemptID]domain.AttemptReceipt{
		"a1": retainedReceipt("a1", "wu-a", "v1"),
		"a2": retainedReceipt("a2", "wu-a", "v2"),
		"b1": retainedReceipt("b1", "wu-b", "vb1"),
		"c1": retainedReceipt("c1", "wu-c", "vc1"),
	}
	inputRefs := map[domain.AttemptID][]domain.AttemptManifestInputRef{
		"b1": {inputRef("a1", "wu-a", "v1")},
		"c1": {inputRef("b1", "wu-b", "vb1")},
	}
	stale, err := domain.DeriveLineageStaleness(plan, attempts, inputRefs, receipts)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if stale.AttemptStale("a1") || stale.AttemptStale("a2") {
		t.Fatalf("upstream attempts must never go stale: %+v", stale.StaleAttempts)
	}
	if !stale.AttemptStale("b1") {
		t.Fatalf("b1 admitted a1@v1 and wu-a now retains v2")
	}
	if !stale.AttemptStale("c1") {
		t.Fatalf("c1 consumed b1, whose lineage is stale; transitivity must mark c1")
	}
	facts := stale.StaleAttempts["c1"]
	if len(facts) != 1 || facts[0].DependencyUnitID != "wu-a" || facts[0].AdmittedVersion != "v1" || facts[0].CurrentVersion != "v2" {
		t.Fatalf("c1 must name the original supersession, got %+v", facts)
	}
	if !stale.WorkUnitStale("wu-b") || !stale.WorkUnitStale("wu-c") {
		t.Fatalf("wu-b and wu-c must be stale at unit level: %+v", stale.StaleWorkUnits)
	}
	if stale.WorkUnitStale("wu-a") {
		t.Fatalf("wu-a has no inputs and cannot be stale")
	}
}

func TestDeriveLineageStalenessCurrentVersionMovesToTheNewestRetainedAttempt(t *testing.T) {
	plan := stalenessPlan()
	attempts := []domain.Attempt{
		stalenessAttempt("a1", "wu-a", 1), stalenessAttempt("a2", "wu-a", 2),
		stalenessAttempt("b1", "wu-b", 1), stalenessAttempt("b2", "wu-b", 2),
	}
	receipts := map[domain.AttemptID]domain.AttemptReceipt{
		"a1": retainedReceipt("a1", "wu-a", "v1"),
		"a2": retainedReceipt("a2", "wu-a", "v2"),
		"b1": retainedReceipt("b1", "wu-b", "vb1"),
		"b2": retainedReceipt("b2", "wu-b", "vb2"),
	}
	inputRefs := map[domain.AttemptID][]domain.AttemptManifestInputRef{
		"b1": {inputRef("a1", "wu-a", "v1")},
		"b2": {inputRef("a2", "wu-a", "v2")},
	}
	stale, err := domain.DeriveLineageStaleness(plan, attempts, inputRefs, receipts)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if !stale.AttemptStale("b1") {
		t.Fatalf("the old attempt stays stale after re-execution; it is never resurrected")
	}
	if stale.AttemptStale("b2") {
		t.Fatalf("b2 admitted the current version and must be current")
	}
	if stale.WorkUnitStale("wu-b") {
		t.Fatalf("wu-b's current proving attempt b2 is fresh, so the unit is no longer stale")
	}
}

func TestDeriveLineageStalenessIgnoresNonRetainedAndLegacyShapes(t *testing.T) {
	plan := stalenessPlan()
	attempts := []domain.Attempt{
		stalenessAttempt("a1", "wu-a", 1), stalenessAttempt("a2", "wu-a", 2),
		stalenessAttempt("b1", "wu-b", 1),
	}
	incomplete := retainedReceipt("a2", "wu-a", "v2")
	incomplete.RetentionState = domain.RetentionIncomplete
	receipts := map[domain.AttemptID]domain.AttemptReceipt{
		"a1": retainedReceipt("a1", "wu-a", "v1"),
		"a2": incomplete,
	}
	// b1 admits a1@v1; a2's receipt is incomplete, so wu-a's current version
	// is still v1 and nothing is stale. A legacy attempt with no sealed input
	// half carries no supersession risk either.
	inputRefs := map[domain.AttemptID][]domain.AttemptManifestInputRef{
		"b1": {inputRef("a1", "wu-a", "v1")},
	}
	stale, err := domain.DeriveLineageStaleness(plan, attempts, inputRefs, receipts)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if stale.AttemptStale("b1") || stale.AttemptStale("a1") || stale.AttemptStale("a2") {
		t.Fatalf("no current retained version moved: %+v", stale.StaleAttempts)
	}
	if len(stale.StaleWorkUnits) != 0 {
		t.Fatalf("no unit may be stale: %+v", stale.StaleWorkUnits)
	}
}

func TestDeriveLineageStalenessIdenticalVersionIsCurrency(t *testing.T) {
	plan := stalenessPlan()
	attempts := []domain.Attempt{
		stalenessAttempt("a1", "wu-a", 1), stalenessAttempt("a2", "wu-a", 2),
		stalenessAttempt("b1", "wu-b", 1),
	}
	receipts := map[domain.AttemptID]domain.AttemptReceipt{
		"a1": retainedReceipt("a1", "wu-a", "v1"),
		"a2": retainedReceipt("a2", "wu-a", "v1"), // rework produced identical bytes
		"b1": retainedReceipt("b1", "wu-b", "vb1"),
	}
	inputRefs := map[domain.AttemptID][]domain.AttemptManifestInputRef{
		"b1": {inputRef("a1", "wu-a", "v1")},
	}
	stale, err := domain.DeriveLineageStaleness(plan, attempts, inputRefs, receipts)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if stale.AttemptStale("b1") {
		t.Fatalf("content-addressed identity is currency, not staleness: %+v", stale.StaleAttempts["b1"])
	}
}
