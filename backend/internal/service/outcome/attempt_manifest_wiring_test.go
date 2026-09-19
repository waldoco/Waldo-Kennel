package outcome_test

import (
	"context"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

type fakeManifestStore struct {
	saved []domain.AttemptManifest
}

func (f *fakeManifestStore) SaveAttemptManifest(_ context.Context, m domain.AttemptManifest) error {
	f.saved = append(f.saved, m)
	return nil
}

func (f *fakeManifestStore) GetAttemptManifest(_ context.Context, attemptID domain.AttemptID, half domain.AttemptManifestHalf) (domain.AttemptManifest, bool, error) {
	for _, m := range f.saved {
		if m.AttemptID == attemptID && m.Half == half {
			return m, true, nil
		}
	}
	return domain.AttemptManifest{}, false, nil
}

func (f *fakeManifestStore) ListAttemptManifestsForOutcome(_ context.Context, outcomeID domain.OutcomeID) ([]domain.AttemptManifest, error) {
	var out []domain.AttemptManifest
	for _, m := range f.saved {
		if m.OutcomeID == outcomeID {
			out = append(out, m)
		}
	}
	return out, nil
}

// The input half is sealed at admission: the Attempt's custody record binds
// the exact lineage and frozen digests it was launched under.
func TestStartAttemptSealsInputCustodyManifest(t *testing.T) {
	svc, _, _, _, outcomeID, planID := newAttemptHarness(t)
	manifests := &fakeManifestStore{}
	svc.WithAttemptManifests(manifests)

	view, err := svc.StartAttempt(context.Background(), outcomeID, startInput(planID))
	if err != nil {
		t.Fatalf("start attempt: %v", err)
	}
	if len(manifests.saved) != 1 {
		t.Fatalf("saved manifests = %d, want the sealed input half", len(manifests.saved))
	}
	manifest := manifests.saved[0]
	if manifest.Half != domain.AttemptManifestInput {
		t.Fatalf("half = %q, want input", manifest.Half)
	}
	if manifest.AttemptID != view.Attempt.ID || manifest.OutcomeID != outcomeID {
		t.Fatalf("manifest lineage = %s/%s, want %s/%s", manifest.AttemptID, manifest.OutcomeID, view.Attempt.ID, outcomeID)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("sealed manifest must validate: %v", err)
	}
	body, err := manifest.DecodeInput()
	if err != nil {
		t.Fatal(err)
	}
	if body.PlanRevisionID != planID || body.WorkUnitID != firstWorkUnitOfPlan[planID] {
		t.Fatalf("admitted plan/workunit = %s/%s, want %s/%s", body.PlanRevisionID, body.WorkUnitID, planID, firstWorkUnitOfPlan[planID])
	}
	if len(body.Inputs) != 0 {
		t.Fatalf("root WorkUnit admitted with %d inputs, want none", len(body.Inputs))
	}
	if body.RunBriefCoreDigest == "" || body.RunBriefCompiledDigest == "" || body.ExecutionPolicyDigest == "" {
		t.Fatal("frozen digests missing from the custody record")
	}
}

// Without a manifest store wired the service keeps working exactly as before:
// custody lineage is unavailable, not fabricated.
func TestStartAttemptWithoutManifestStoreStartsNormally(t *testing.T) {
	svc, _, spawner, _, outcomeID, planID := newAttemptHarness(t)
	if _, err := svc.StartAttempt(context.Background(), outcomeID, startInput(planID)); err != nil {
		t.Fatalf("start attempt: %v", err)
	}
	if spawner.spawnCalls() != 1 {
		t.Fatalf("provider spawn calls = %d, want one", spawner.spawnCalls())
	}
}
