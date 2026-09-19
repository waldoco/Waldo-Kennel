package outcome_test

import (
	"context"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
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

// The input half is sealed INSIDE the mandatory provider-launch crash
// boundary: by the time Spawn has launched anything, the custody record is
// durable. A crash after provider launch can never leave an executing Attempt
// without its admitted inputs sealed.
func TestStartAttemptSealsInputManifestInsideLaunchBoundary(t *testing.T) {
	svc, _, spawner, _, outcomeID, planID := newAttemptHarness(t)
	manifests := &fakeManifestStore{}
	svc.WithAttemptManifests(manifests)
	sealedAtBoundary := false
	spawner.afterPrelaunch = func() {
		sealedAtBoundary = len(manifests.saved) == 1
	}
	if _, err := svc.StartAttempt(context.Background(), outcomeID, startInput(planID)); err != nil {
		t.Fatalf("start attempt: %v", err)
	}
	if !sealedAtBoundary {
		t.Fatal("input custody half was not sealed inside the provider-launch boundary")
	}
}

// A crash between launch-packet persistence and custody sealing leaves a
// packet-bound Attempt with no input half. Recovery must refuse to continue
// it: launch evidence is inconsistent and custody stays held.
func TestRecoverAttemptPacketBoundWithoutInputManifestStaysHeld(t *testing.T) {
	svc, _, _, _, outcomeID, planID := newAttemptHarness(t)
	manifests := &fakeManifestStore{}
	svc.WithAttemptManifests(manifests)
	view, err := svc.StartAttempt(context.Background(), outcomeID, startInput(planID))
	if err != nil {
		t.Fatalf("start attempt: %v", err)
	}
	if len(manifests.saved) != 1 {
		t.Fatalf("sealed manifests = %d, want one", len(manifests.saved))
	}
	// Simulate the crash window: packet durable, custody half lost.
	manifests.saved = nil
	_, err = svc.RecoverAttempt(context.Background(), outcomeID, view.Attempt.ID, outcome.RecoveryInput{Action: outcome.RecoveryActionReconcile})
	if code := requireAPICode(t, err); code != outcome.CodeAttemptCustodyUnproven {
		t.Fatalf("reconcile code = %s, want %s", code, outcome.CodeAttemptCustodyUnproven)
	}
}

// A workspace-project Attempt seals the canonical per-repo inventory into the
// custody manifest: every repo fact observed at the prelaunch boundary is
// bound with its exact base.
func TestStartAttemptSealsWorkspaceRepoInventory(t *testing.T) {
	svc, _, spawner, _, outcomeID, planID := newAttemptHarness(t)
	manifests := &fakeManifestStore{}
	svc.WithAttemptManifests(manifests)
	spawner.sessionMetadata = domain.SessionMetadata{WorkspaceRepoPath: "/repo/root"}
	spawner.sessionWorktrees = []domain.SessionWorktreeRecord{
		{RepoName: "root", WorktreePath: "/ws/root", BaseSHA: "sha-root", BaseRef: "main"},
		{RepoName: "api", WorktreePath: "/ws/api", BaseSHA: "sha-api", BaseRef: "main"},
	}

	if _, err := svc.StartAttempt(context.Background(), outcomeID, startInput(planID)); err != nil {
		t.Fatalf("start attempt: %v", err)
	}
	if len(manifests.saved) != 1 {
		t.Fatalf("saved manifests = %d, want the sealed input half", len(manifests.saved))
	}
	body, err := manifests.saved[0].DecodeInput()
	if err != nil {
		t.Fatal(err)
	}
	if err := manifests.saved[0].Validate(); err != nil {
		t.Fatalf("sealed manifest must validate: %v", err)
	}
	if len(body.Repos) != 2 {
		t.Fatalf("sealed repos = %#v, want both workspace repos", body.Repos)
	}
	if body.BaseRevision != "" || body.BaseRef != "" {
		t.Fatalf("workspace shape must bind only the repo inventory, got base %q ref %q", body.BaseRevision, body.BaseRef)
	}
	if body.Repos[0].RepoName != "root" || body.Repos[0].BaseSHA != "sha-root" || body.Repos[0].WorktreePath != "/ws/root" {
		t.Fatalf("root repo fact = %#v", body.Repos[0])
	}
	if body.Repos[1].RepoName != "api" || body.Repos[1].BaseSHA != "sha-api" || body.Repos[1].WorktreePath != "/ws/api" {
		t.Fatalf("api repo fact = %#v", body.Repos[1])
	}
}

// Falsifier (the review seam): a git-worktree session whose base resolved to
// nothing AND whose inventory is empty has no source-tree representation at
// all - the seal must refuse it and no provider may start.
func TestStartAttemptRefusesGitWorktreeWithoutAnySourceTreeRepresentation(t *testing.T) {
	svc, _, spawner, _, outcomeID, planID := newAttemptHarness(t)
	manifests := &fakeManifestStore{}
	svc.WithAttemptManifests(manifests)
	spawner.sessionMetadata = domain.SessionMetadata{WorkspaceRepoPath: "/repo/root", DiffBaseSHA: "", DiffBaseRef: ""}

	pastBoundary := false
	spawner.afterPrelaunch = func() { pastBoundary = true }
	if _, err := svc.StartAttempt(context.Background(), outcomeID, startInput(planID)); err == nil {
		t.Fatal("a git worktree with neither a base revision nor a repo inventory must refuse admission")
	}
	if pastBoundary {
		t.Fatal("launch crossed the provider boundary despite the seal refusing the representationless source tree")
	}
	if len(manifests.saved) != 0 {
		t.Fatalf("sealed %d manifests without a source-tree representation, want none", len(manifests.saved))
	}
}

// Falsifier: a workspace shape whose durable inventory lost a base revision
// must be refused at the seal, not admitted with fabricated custody.
func TestStartAttemptRefusesWorkspaceShapeWithoutRepoBase(t *testing.T) {
	svc, _, spawner, _, outcomeID, planID := newAttemptHarness(t)
	manifests := &fakeManifestStore{}
	svc.WithAttemptManifests(manifests)
	spawner.sessionMetadata = domain.SessionMetadata{WorkspaceRepoPath: "/repo/root"}
	spawner.sessionWorktrees = []domain.SessionWorktreeRecord{{RepoName: "root", WorktreePath: "/ws/root", BaseSHA: ""}}

	pastBoundary := false
	spawner.afterPrelaunch = func() { pastBoundary = true }
	if _, err := svc.StartAttempt(context.Background(), outcomeID, startInput(planID)); err == nil {
		t.Fatal("a workspace source tree without a resolved base must refuse admission")
	}
	if pastBoundary {
		t.Fatal("launch crossed the provider boundary despite the seal refusing the baseless inventory")
	}
	if len(manifests.saved) != 0 {
		t.Fatalf("sealed %d manifests from a baseless inventory, want none", len(manifests.saved))
	}
}
