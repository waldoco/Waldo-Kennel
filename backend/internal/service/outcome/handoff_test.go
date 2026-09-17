package outcome

import (
	"errors"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apierr"
)

func succeededReceipt(attempt domain.Attempt) domain.AttemptReceipt {
	at := time.Unix(100, 0).UTC()
	frozen := at.Add(time.Minute)
	files := []domain.ArtifactFile{{
		ID: "artifact-1", AttemptID: attempt.ID, RelativePath: "result.md",
		ChangeKind: domain.ArtifactAdded, ContentDigest: "d1",
	}}
	// A retained receipt's version is the digest over its own manifest, so it
	// cannot be a label chosen independently of the bytes.
	version := string(domain.ArtifactManifestDigest(files))
	return domain.AttemptReceipt{
		AttemptID: attempt.ID, OutcomeID: attempt.OutcomeID, PlanRevisionID: attempt.PlanRevisionID,
		WorkUnitID: attempt.WorkUnitID, ContractRevisionNumber: attempt.ContractRevisionNumber,
		ArtifactVersion: version, WorkspaceKind: domain.WorkspaceStagedFolder, WorkspacePath: "/tmp/ws",
		RetentionState: domain.RetentionRetained, ObservedAt: at, CreatedAt: at, UpdatedAt: at,
		FrozenAt: &frozen, Files: files,
	}
}

func lookupOf(receipts ...domain.AttemptReceipt) func(domain.AttemptID) (domain.AttemptReceipt, bool, error) {
	byID := map[domain.AttemptID]domain.AttemptReceipt{}
	for _, r := range receipts {
		byID[r.AttemptID] = r
	}
	return func(id domain.AttemptID) (domain.AttemptReceipt, bool, error) {
		r, ok := byID[id]
		return r, ok, nil
	}
}

func codeOf(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("expected a refusal, got none")
	}
	var api *apierr.Error
	if !errors.As(err, &api) {
		t.Fatalf("err = %v, want a typed apierr", err)
	}
	return api.Code
}

// A unit whose predecessors all produced frozen, complete results resolves to
// exactly those results, in dependency order.
func TestUpstreamResolvesEachDependencysExactResult(t *testing.T) {
	plan := schedulerPlanFixture()
	successor := plan.WorkUnits[1] // wu-b depends on wu-a
	if len(successor.DependsOn) == 0 {
		t.Fatalf("fixture changed: %s has no dependencies", successor.ID)
	}
	producer := attemptOn("att-a", successor.DependsOn[0], plan, domain.AttemptSucceeded)
	receipt := succeededReceipt(producer)

	got, err := upstreamReceiptsFor(successor, []domain.Attempt{producer}, lookupOf(receipt))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(got) != 1 || got[0].ArtifactVersion != receipt.ArtifactVersion || got[0].AttemptID != producer.ID {
		t.Fatalf("resolved %#v", got)
	}
}

// A unit with no dependencies needs nothing handed to it.
func TestUpstreamIsEmptyForARootWorkUnit(t *testing.T) {
	plan := schedulerPlanFixture()
	root := plan.WorkUnits[0]
	got, err := upstreamReceiptsFor(root, nil, lookupOf())
	if err != nil || len(got) != 0 {
		t.Fatalf("root unit = %#v, %v", got, err)
	}
}

// Every way a predecessor can fail to hand its work down, and the named
// refusal for each. Starting the successor anyway would silently drop the
// predecessor's work and let it be redone or contradicted.
func TestUpstreamRefusesAResultItCannotHandDown(t *testing.T) {
	plan := schedulerPlanFixture()
	successor := plan.WorkUnits[1]
	dependency := successor.DependsOn[0]
	producer := attemptOn("att-a", dependency, plan, domain.AttemptSucceeded)

	incomplete := succeededReceipt(producer)
	incomplete.RetentionState = domain.RetentionIncomplete
	incomplete.RetentionDetail = "byte bound exceeded at big.bin"

	unfrozen := succeededReceipt(producer)
	unfrozen.FrozenAt = nil

	foreign := succeededReceipt(producer)
	foreign.WorkUnitID = "wu-somebody-else"

	for _, tc := range []struct {
		name     string
		attempts []domain.Attempt
		lookup   func(domain.AttemptID) (domain.AttemptReceipt, bool, error)
		want     string
	}{
		{"the dependency has not succeeded", []domain.Attempt{attemptOn("att-a", dependency, plan, domain.AttemptReconciled)}, lookupOf(), CodeUpstreamArtifactMissing},
		{"the dependency was never attempted", nil, lookupOf(), CodeUpstreamArtifactMissing},
		{"the succeeded attempt retained nothing", []domain.Attempt{producer}, lookupOf(), CodeUpstreamArtifactMissing},
		{"the snapshot is incomplete", []domain.Attempt{producer}, lookupOf(incomplete), CodeUpstreamArtifactIncomplete},
		{"the result is not frozen", []domain.Attempt{producer}, lookupOf(unfrozen), CodeUpstreamArtifactUnreviewed},
		{"the receipt belongs to another WorkUnit", []domain.Attempt{producer}, lookupOf(foreign), CodeUpstreamLineageMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := upstreamReceiptsFor(successor, tc.attempts, tc.lookup)
			if code := codeOf(t, err); code != tc.want {
				t.Fatalf("code = %s, want %s", code, tc.want)
			}
			if len(got) != 0 {
				t.Fatalf("a refused resolution returned %d receipts", len(got))
			}
		})
	}
}

// Only a succeeded attempt may hand work down. Success is the one status
// derived from retained bytes plus proof about those bytes; a reconciled
// attempt has ended without being classified and its output can still change.
func TestOnlyASucceededAttemptProducesTheHandoff(t *testing.T) {
	plan := schedulerPlanFixture()
	unit := plan.WorkUnits[0]
	for _, status := range []domain.AttemptStatus{
		domain.AttemptQueued, domain.AttemptRunning, domain.AttemptPaused,
		domain.AttemptFailed, domain.AttemptCancelled, domain.AttemptLost, domain.AttemptReconciled,
	} {
		if _, ok := producingAttempt(unit.ID, []domain.Attempt{attemptOn("att-x", unit.ID, plan, status)}); ok {
			t.Fatalf("%s was treated as a producer", status)
		}
	}
	if _, ok := producingAttempt(unit.ID, []domain.Attempt{attemptOn("att-x", unit.ID, plan, domain.AttemptSucceeded)}); !ok {
		t.Fatal("a succeeded attempt must be the producer")
	}
}

// Multiple succeeded lineages are ambiguous and must block rather than selecting "latest".
func TestUpstreamRejectsAmbiguousSucceededLineage(t *testing.T) {
	plan := schedulerPlanFixture()
	successor := plan.WorkUnits[1]
	dep := successor.DependsOn[0]
	first := attemptOn("att-first", dep, plan, domain.AttemptSucceeded)
	first.Number = 1
	second := attemptOn("att-second", dep, plan, domain.AttemptSucceeded)
	second.Number = 2
	if got, err := upstreamReceiptsFor(successor, []domain.Attempt{first, second}, lookupOf(succeededReceipt(first), succeededReceipt(second))); len(got) != 0 || codeOf(t, err) != CodeUpstreamLineageMismatch {
		t.Fatalf("ambiguous lineage got=%v err=%v", got, err)
	}
}
