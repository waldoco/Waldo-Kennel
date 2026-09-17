package outcome_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/intelligence/intelligencetest"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

// classificationHarness drives one Attempt to the point where terminal
// classification decides between succeeded and still-reconciled.
type classificationHarness struct {
	svc       *outcome.Service
	store     *receiptFakeStore
	outcomeID domain.OutcomeID
	plan      domain.PlanRevision
	attempt   domain.Attempt
	receipt   domain.AttemptReceipt
	contract  domain.ContractRevision
}

func newClassificationHarness(t *testing.T) *classificationHarness {
	t.Helper()
	store := newReceiptFakeStore()
	spawner := &fakeSpawner{readiness: ports.AgentProfileReadiness{Ready: true, Detail: "profile ok"}}
	svc := outcome.New(store, func() time.Time { return time.Unix(1_000, 0).UTC() }).
		WithPlanning(intelligencetest.New(), &routingInventoryFake{candidates: []domain.RoutingCandidate{executionCandidate(domain.HarnessCodex, "")}}).
		WithExecution(spawner, newFakeHeartbeats())
	svc.AdmissionPolicy = testAdmissionPolicy()

	ctx := context.Background()
	view, err := svc.Create(ctx, validCreateInput())
	if err != nil {
		t.Fatalf("create outcome: %v", err)
	}
	planView, err := svc.ProposePlan(ctx, view.Outcome.ID, 1)
	if err != nil {
		t.Fatalf("propose plan: %v", err)
	}
	if _, err := svc.ApprovePlan(ctx, view.Outcome.ID, outcome.ApprovePlanInput{
		PlanRevisionID: planView.Plan.ID, ExpectedContractRevision: 1,
	}); err != nil {
		t.Fatalf("approve plan: %v", err)
	}
	rememberFirstWorkUnit(planView.Plan)
	if _, err := svc.StartAttempt(ctx, view.Outcome.ID, startInput(planView.Plan.ID)); err != nil {
		t.Fatalf("start attempt: %v", err)
	}
	attempts, err := store.ListAttempts(ctx, view.Outcome.ID)
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempts = %d err=%v", len(attempts), err)
	}
	attempt := attempts[0]
	if _, err := store.TransitionAttemptStatus(ctx, view.Outcome.ID, attempt.ID,
		attempt.Status, domain.AttemptReconciled, time.Unix(1_000, 0).UTC()); err != nil {
		t.Fatalf("end attempt: %v", err)
	}
	attempt.Status = domain.AttemptReconciled

	receipt := retainedReceiptFor(attempt)
	if err := store.SaveAttemptReceipt(ctx, receipt); err != nil {
		t.Fatalf("save receipt: %v", err)
	}
	return &classificationHarness{
		svc: svc, store: store, outcomeID: view.Outcome.ID, plan: planView.Plan,
		attempt: attempt, receipt: receipt, contract: view.Current,
	}
}

func retainedReceiptFor(attempt domain.Attempt) domain.AttemptReceipt {
	at := time.Unix(900, 0).UTC()
	files := []domain.ArtifactFile{{
		ID: "artifact-1", AttemptID: attempt.ID, RelativePath: "result.md",
		ChangeKind: domain.ArtifactAdded, ContentDigest: strings.Repeat("a", 64),
	}}
	return domain.AttemptReceipt{
		AttemptID: attempt.ID, OutcomeID: attempt.OutcomeID, PlanRevisionID: attempt.PlanRevisionID,
		WorkUnitID: attempt.WorkUnitID, ContractRevisionNumber: attempt.ContractRevisionNumber,
		ArtifactVersion: string(domain.ArtifactManifestDigest(files)),
		WorkspaceKind:   domain.WorkspaceStagedFolder, WorkspacePath: "/tmp/ws",
		RetentionState: domain.RetentionRetained, ObservedAt: at, CreatedAt: at, UpdatedAt: at, Files: files,
	}
}

// proveCriterion records supporting evidence and a passing verification bound
// to this exact Attempt and artifact version, which is what makes the Attempt
// classifiable at all.
func (h *classificationHarness) proveCriterion(t *testing.T, criterionID domain.CriterionID, key string) {
	t.Helper()
	ctx := context.Background()
	if _, err := h.svc.RecordEvidence(ctx, h.outcomeID, outcome.RecordEvidenceInput{
		ExpectedContractRevision: h.contract.Number, ContractRevisionID: h.contract.ID,
		CriterionID: criterionID, SubjectType: domain.ProofSubjectAttempt,
		SubjectID: string(h.attempt.ID), SubjectRevision: h.receipt.ArtifactVersion,
		Kind: domain.EvidenceSupporting, SourceType: domain.EvidenceSourceDeterministicCheck,
		SourceRef: "go test ./...", ProducerType: domain.EvidenceProducerTool, ProducerRef: string(h.attempt.ID),
		Summary: "exited 0", ContentDigest: strings.Repeat("b", 64), RequestKey: "ev-" + key,
	}); err != nil {
		t.Fatalf("record evidence: %v", err)
	}
	item, found, err := h.store.FindEvidenceItemByRequestKey(ctx, "ev-"+key)
	if err != nil || !found {
		t.Fatalf("read evidence: found=%v err=%v", found, err)
	}
	if _, err := h.svc.RecordVerification(ctx, h.outcomeID, outcome.RecordVerificationInput{
		ExpectedContractRevision: h.contract.Number, ContractRevisionID: h.contract.ID,
		CriterionID: criterionID, SubjectType: domain.ProofSubjectAttempt,
		SubjectID: string(h.attempt.ID), SubjectRevision: h.receipt.ArtifactVersion,
		EvidenceItemIDs: []domain.EvidenceItemID{item.ID}, Method: "go test ./...",
		IndependenceClass: domain.VerificationDeterministic, Result: domain.VerificationPassed,
		ProducerRef: string(h.attempt.ID), VerifierRef: "kennel-governed-check/test", RequestKey: "ver-" + key,
	}); err != nil {
		t.Fatalf("record verification: %v", err)
	}
}

// contradict records a failing verification for the same criterion and
// artifact. It is the record a late writer commits.
func (h *classificationHarness) contradict(t *testing.T, criterionID domain.CriterionID) {
	t.Helper()
	ctx := context.Background()
	if _, err := h.svc.RecordEvidence(ctx, h.outcomeID, outcome.RecordEvidenceInput{
		ExpectedContractRevision: h.contract.Number, ContractRevisionID: h.contract.ID,
		CriterionID: criterionID, SubjectType: domain.ProofSubjectAttempt,
		SubjectID: string(h.attempt.ID), SubjectRevision: h.receipt.ArtifactVersion,
		Kind: domain.EvidenceContradicting, SourceType: domain.EvidenceSourceDeterministicCheck,
		SourceRef: "go vet ./...", ProducerType: domain.EvidenceProducerTool, ProducerRef: string(h.attempt.ID),
		Summary: "exited 1", ContentDigest: strings.Repeat("c", 64), RequestKey: "ev-late",
	}); err != nil {
		t.Fatalf("record contradicting evidence: %v", err)
	}
}

func (h *classificationHarness) reload(t *testing.T) domain.Attempt {
	t.Helper()
	attempts, err := h.store.ListAttempts(context.Background(), h.outcomeID)
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempts = %d err=%v", len(attempts), err)
	}
	return attempts[0]
}

func (h *classificationHarness) criterion(t *testing.T) domain.CriterionID {
	t.Helper()
	if len(h.contract.Criteria) == 0 {
		t.Fatal("fixture contract has no criteria")
	}
	return h.contract.Criteria[0].ID
}

// TestReconcileAttemptOutcomes_ClassifiesAProvedAttempt is the green half:
// without an interleaved write the very same setup does classify, so the
// refusal below is caused by the race and not by a broken fixture.
func TestReconcileAttemptOutcomes_ClassifiesAProvedAttempt(t *testing.T) {
	h := newClassificationHarness(t)
	h.proveCriterion(t, h.criterion(t), "pass")

	if err := h.svc.ReconcileAttemptOutcomes(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if got := h.reload(t).Status; got != domain.AttemptSucceeded {
		t.Fatalf("status = %s, want succeeded", got)
	}
}

// TestReconcileAttemptOutcomes_RefusesProofThatChangedUnderTheSnapshot is R1.
//
// A contradicting record commits immediately after the proof snapshot is
// taken. Reading the generation BEFORE the proof means the observed
// generation is already behind, so the commit refuses. Reading it after would
// count the record in the generation while the snapshot missed it, and the
// Attempt would be classified as succeeded on proof that no longer holds.
func TestReconcileAttemptOutcomes_RefusesProofThatChangedUnderTheSnapshot(t *testing.T) {
	h := newClassificationHarness(t)
	criterion := h.criterion(t)
	h.proveCriterion(t, criterion, "pass")

	h.store.resetReads()
	h.store.afterEvidenceRead = func() { h.contradict(t, criterion) }

	err := h.svc.ReconcileAttemptOutcomes(context.Background())
	if !h.store.evidenceReadFired {
		t.Fatal("the interleaved record never committed; the test proves nothing")
	}
	if got := h.reload(t).Status; got == domain.AttemptSucceeded {
		t.Fatalf("an Attempt was classified as succeeded on a proof snapshot taken before a contradicting record committed (reconcile err = %v)", err)
	}
	// The generation read has to come first for that refusal to be possible
	// at all; naming it here keeps the ordering from silently regressing.
	if first := h.store.firstRead(); first != "generation" {
		t.Fatalf("first classification read = %q, want the generation before the proof", first)
	}
}
