package outcome_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/artifactstore"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/intelligence/intelligencetest"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

// documentContextFakeStore is the append-only selection history.
type documentContextFakeStore struct {
	byOutcome map[domain.OutcomeID][]domain.OutcomeDocumentContext
	byID      map[domain.DocumentContextID]domain.OutcomeDocumentContext
}

func newDocumentContextFakeStore() *documentContextFakeStore {
	return &documentContextFakeStore{
		byOutcome: map[domain.OutcomeID][]domain.OutcomeDocumentContext{},
		byID:      map[domain.DocumentContextID]domain.OutcomeDocumentContext{},
	}
}

func (f *documentContextFakeStore) AppendDocumentContext(_ context.Context, selection domain.OutcomeDocumentContext) (domain.OutcomeDocumentContext, error) {
	history := f.byOutcome[selection.OutcomeID]
	selection.Revision = int64(len(history) + 1)
	selection.State = domain.DocumentContextSelected
	selection.ApprovedAt = nil
	if err := selection.Validate(); err != nil {
		return domain.OutcomeDocumentContext{}, err
	}
	f.byOutcome[selection.OutcomeID] = append(history, selection)
	f.byID[selection.ID] = selection
	return selection, nil
}

func (f *documentContextFakeStore) CurrentDocumentContext(_ context.Context, outcomeID domain.OutcomeID) (domain.OutcomeDocumentContext, bool, error) {
	history := f.byOutcome[outcomeID]
	if len(history) == 0 {
		return domain.OutcomeDocumentContext{}, false, nil
	}
	return history[len(history)-1], true, nil
}

func (f *documentContextFakeStore) GetDocumentContext(_ context.Context, id domain.DocumentContextID) (domain.OutcomeDocumentContext, bool, error) {
	selection, ok := f.byID[id]
	return selection, ok, nil
}

func (f *documentContextFakeStore) ApproveDocumentContext(_ context.Context, outcomeID domain.OutcomeID, id domain.DocumentContextID, digest string, at time.Time) error {
	selection, ok := f.byID[id]
	if !ok {
		return &ports.DocumentContextApprovalConflictError{OutcomeID: outcomeID, ContextID: id}
	}
	if selection.OutcomeID != outcomeID || selection.Digest != digest {
		return &ports.DocumentContextApprovalConflictError{OutcomeID: outcomeID, ContextID: id}
	}
	current, found, _ := f.CurrentDocumentContext(context.Background(), outcomeID)
	if !found || current.ID != id || current.Digest != digest {
		return &ports.DocumentContextApprovalConflictError{OutcomeID: outcomeID, ContextID: id}
	}
	if selection.Approved() {
		return nil
	}
	approved := at.UTC()
	selection.State, selection.ApprovedAt = domain.DocumentContextApproved, &approved
	f.byID[id] = selection
	history := f.byOutcome[selection.OutcomeID]
	for i := range history {
		if history[i].ID == id {
			history[i] = selection
		}
	}
	return nil
}

var _ ports.DocumentContextStore = (*documentContextFakeStore)(nil)

// documentHarness is an approved Outcome with the supplied-document path
// wired over a real snapshot store.
type documentHarness struct {
	svc       *outcome.Service
	store     *attemptFakeStore
	spawner   *fakeSpawner
	snapshots *artifactstore.Store
	outcomeID domain.OutcomeID
	planID    domain.PlanRevisionID
	dir       string
}

func newDocumentHarness(t *testing.T) *documentHarness {
	t.Helper()
	svc, store, spawner, _, outcomeID, planID := newAttemptHarness(t)
	snapshots, err := artifactstore.New(artifactstore.Config{Root: filepath.Join(t.TempDir(), "artifacts")})
	if err != nil {
		t.Fatalf("artifact store: %v", err)
	}
	svc = svc.WithDocuments(newDocumentContextFakeStore(), snapshots)

	plan, err := svc.GetLatestPlan(context.Background(), outcomeID)
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	rememberFirstWorkUnit(plan.Plan)
	return &documentHarness{
		svc: svc, store: store, spawner: spawner, snapshots: snapshots,
		outcomeID: outcomeID, planID: planID, dir: t.TempDir(),
	}
}

func (h *documentHarness) replan(t *testing.T) {
	t.Helper()
	plan, err := h.svc.ProposePlan(context.Background(), h.outcomeID, 1)
	if err != nil {
		t.Fatalf("replan: %v", err)
	}
	if _, err = h.svc.ApprovePlan(context.Background(), h.outcomeID, outcome.ApprovePlanInput{PlanRevisionID: plan.Plan.ID, ExpectedContractRevision: 1}); err != nil {
		t.Fatalf("approve replanned document context: %v", err)
	}
	h.planID = plan.Plan.ID
	rememberFirstWorkUnit(plan.Plan)
}

func (h *documentHarness) writeDoc(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(h.dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestDocumentOutcome_SelectApproveThenStage is the supplied-document journey
// through to launch: the owner selects material, reviews it, and only then
// may work be staged from the approved snapshot.
func TestDocumentOutcome_SelectApproveThenStage(t *testing.T) {
	h := newDocumentHarness(t)
	ctx := context.Background()
	brief := h.writeDoc(t, "brief.md", "# What good looks like\n")

	selected, err := h.svc.SelectDocuments(ctx, h.outcomeID, []string{brief})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if selected.Context.Approved() {
		t.Fatal("selecting a document approved it; the owner has not reviewed the scope yet")
	}

	// Starting before approval is refused: unreviewed material must not reach
	// a provider.
	_, err = h.svc.StartAttempt(ctx, h.outcomeID, startInput(h.planID))
	if requireAPICode(t, err) != outcome.CodeDocumentContextNotApproved {
		t.Fatalf("start before approval = %v, want DOCUMENT_CONTEXT_NOT_APPROVED", err)
	}
	if calls := h.spawner.spawnCalls(); calls != 0 {
		t.Fatalf("an unapproved document Outcome launched %d providers", calls)
	}

	approved, err := h.svc.ApproveDocuments(ctx, h.outcomeID, selected.Context.Digest)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if !approved.Context.Approved() || approved.Context.ApprovedAt == nil {
		t.Fatalf("context = %+v, want approved", approved.Context)
	}
	h.replan(t)

	if _, err := h.svc.StartAttempt(ctx, h.outcomeID, startInput(h.planID)); err != nil {
		t.Fatalf("start after approval: %v", err)
	}
	if len(h.spawner.spawned) != 1 {
		t.Fatalf("spawn calls = %d", len(h.spawner.spawned))
	}
	// The launch names the exact approved revision, so what gets staged is
	// what was reviewed rather than whatever is selected now.
	documents := h.spawner.spawned[0].Documents
	if documents == nil {
		t.Fatal("a document Outcome launched with no approved snapshot named")
	}
	if documents.ContextID != approved.Context.ID || documents.Digest != approved.Context.Digest {
		t.Fatalf("staged context = %+v, want the approved %s/%s", documents, approved.Context.ID, approved.Context.Digest)
	}
}

// TestDocumentOutcome_EditedSourcesRefuseAdmission stops a run producing a
// result about bytes the owner has since replaced.
func TestDocumentOutcome_EditedSourcesRefuseAdmission(t *testing.T) {
	h := newDocumentHarness(t)
	ctx := context.Background()
	brief := h.writeDoc(t, "brief.md", "# Original\n")

	selected, err := h.svc.SelectDocuments(ctx, h.outcomeID, []string{brief})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if _, err := h.svc.ApproveDocuments(ctx, h.outcomeID, selected.Context.Digest); err != nil {
		t.Fatalf("approve: %v", err)
	}
	h.replan(t)
	h.writeDoc(t, "brief.md", "# Rewritten after approval\n")

	view, err := h.svc.GetDocumentContext(ctx, h.outcomeID)
	if err != nil {
		t.Fatalf("read context: %v", err)
	}
	if len(view.ChangedSources) != 1 || view.ChangedSources[0] != "brief.md" {
		t.Fatalf("changed sources = %v, want brief.md reported", view.ChangedSources)
	}

	_, err = h.svc.StartAttempt(ctx, h.outcomeID, startInput(h.planID))
	if requireAPICode(t, err) != outcome.CodeDocumentSourcesChanged {
		t.Fatalf("start with edited sources = %v, want DOCUMENT_SOURCES_CHANGED", err)
	}
	if calls := h.spawner.spawnCalls(); calls != 0 {
		t.Fatalf("an Outcome with edited sources launched %d providers", calls)
	}

	// Re-selecting produces a new revision that must be approved on its own.
	reselected, err := h.svc.SelectDocuments(ctx, h.outcomeID, []string{brief})
	if err != nil {
		t.Fatalf("reselect: %v", err)
	}
	if reselected.Context.Revision != 2 || reselected.Context.Approved() {
		t.Fatalf("re-selection = revision %d approved=%v, want an unapproved revision 2",
			reselected.Context.Revision, reselected.Context.Approved())
	}
	if reselected.Context.Digest == selected.Context.Digest {
		t.Fatal("changed material produced the same context digest")
	}
}

// TestDocumentOutcome_ApprovalRequiresTheDigestTheOwnerReviewed stops a
// selection made after the owner looked away being approved by their click.
func TestDocumentOutcome_ApprovalRequiresTheDigestTheOwnerReviewed(t *testing.T) {
	h := newDocumentHarness(t)
	ctx := context.Background()
	first := h.writeDoc(t, "brief.md", "# First\n")

	reviewed, err := h.svc.SelectDocuments(ctx, h.outcomeID, []string{first})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	h.writeDoc(t, "brief.md", "# Second\n")
	if _, err := h.svc.SelectDocuments(ctx, h.outcomeID, []string{first}); err != nil {
		t.Fatalf("reselect: %v", err)
	}

	_, err = h.svc.ApproveDocuments(ctx, h.outcomeID, reviewed.Context.Digest)
	if requireAPICode(t, err) != outcome.CodeDocumentContextStale {
		t.Fatalf("approving a superseded selection = %v, want DOCUMENT_CONTEXT_STALE", err)
	}
}

// TestDocumentOutcome_RepositoryWorkIsUnaffected keeps this path additive:
// an Outcome that selected nothing still starts.
func TestDocumentOutcome_RepositoryWorkIsUnaffected(t *testing.T) {
	h := newDocumentHarness(t)
	if _, err := h.svc.StartAttempt(context.Background(), h.outcomeID, startInput(h.planID)); err != nil {
		t.Fatalf("start with no selected documents: %v", err)
	}
	if h.spawner.spawned[0].Documents != nil {
		t.Fatalf("repository work carried a document context: %+v", h.spawner.spawned[0].Documents)
	}
}

// TestDocumentOutcome_UnwiredDaemonReportsUnavailable keeps a degraded daemon
// honest rather than silently treating a document Outcome as repository work.
func TestDocumentOutcome_UnwiredDaemonReportsUnavailable(t *testing.T) {
	store := newAttemptFakeStore()
	svc := outcome.New(store, func() time.Time { return time.Unix(100, 0).UTC() }).
		WithPlanning(intelligencetest.New(), &routingInventoryFake{candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessCodex, "")}}).
		WithExecution(&fakeSpawner{readiness: ports.AgentProfileReadiness{Ready: true}}, newFakeHeartbeats())

	if svc.DocumentsEnabled() {
		t.Fatal("a daemon with no document storage reported the capability as available")
	}
	if _, err := svc.SelectDocuments(context.Background(), "out-1", []string{"/tmp/brief.md"}); err == nil {
		t.Fatal("selecting documents succeeded on a daemon that cannot hold them")
	}
}
