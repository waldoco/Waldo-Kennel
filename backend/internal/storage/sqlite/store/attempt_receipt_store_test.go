package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
)

func receiptFixture(attemptID domain.AttemptID, outcomeID domain.OutcomeID, plan domain.PlanRevision, at time.Time) domain.AttemptReceipt {
	files := []domain.ArtifactFile{
		{
			ID: "artifact-1", AttemptID: attemptID, RelativePath: "internal/parser/parse.go",
			ChangeKind: domain.ArtifactModified, ContentDigest: string(domain.DigestSHA256([]byte("parse"))),
		},
		{
			ID: "artifact-2", AttemptID: attemptID, RelativePath: "internal/parser/legacy.go",
			ChangeKind: domain.ArtifactDeleted,
		},
	}
	return domain.AttemptReceipt{
		AttemptID: attemptID, OutcomeID: outcomeID, PlanRevisionID: plan.ID,
		WorkUnitID: plan.WorkUnits[0].ID, ContractRevisionNumber: plan.ContractRevisionNumber,
		ArtifactVersion: string(domain.ArtifactManifestDigest(files)),
		WorkspaceKind:   domain.WorkspaceGitWorktree,
		WorkspacePath:   "/tmp/kennel-workspace",
		RepositoryPath:  "/tmp/repo", BaseRevision: "base-sha", ResultRevision: "result-sha",
		WorkspaceDirty: true,
		RetentionState: domain.RetentionRetained,
		ObservedAt:     at, CreatedAt: at, UpdatedAt: at,
		Files: files,
	}
}

func TestAttemptReceiptRoundTripsProducingLineageAndManifest(t *testing.T) {
	s := sqlitetest.MustOpen(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
	plan, outcomeID := seedApprovedPlan(t, s, "receipt-round-trip")
	attempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-receipt-1", domain.FenceSubjectForProject("receipt-round-trip")))
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}

	want := receiptFixture(attempt.ID, outcomeID, plan, at)
	add, del := int64(7), int64(2)
	want.Files[0].Additions, want.Files[0].Deletions = &add, &del
	if err := s.SaveAttemptReceipt(ctx, want); err != nil {
		t.Fatalf("save receipt: %v", err)
	}

	got, ok, err := s.GetAttemptReceipt(ctx, attempt.ID)
	if err != nil || !ok {
		t.Fatalf("read receipt: ok=%v err=%v", ok, err)
	}
	if got.ArtifactVersion != want.ArtifactVersion {
		t.Fatalf("artifact version = %q, want %q", got.ArtifactVersion, want.ArtifactVersion)
	}
	if got.OutcomeID != outcomeID || got.PlanRevisionID != plan.ID || got.WorkUnitID != plan.WorkUnits[0].ID {
		t.Fatalf("producing lineage lost: %+v", got)
	}
	if got.BaseRevision != "base-sha" || got.ResultRevision != "result-sha" || !got.WorkspaceDirty {
		t.Fatalf("revision facts lost: %+v", got)
	}
	if len(got.Files) != 2 {
		t.Fatalf("files = %d, want both", len(got.Files))
	}
	var measured *domain.ArtifactFile
	for i := range got.Files {
		if got.Files[i].RelativePath == "internal/parser/parse.go" {
			measured = &got.Files[i]
		}
	}
	if measured == nil || measured.Additions == nil || *measured.Additions != 7 || measured.Deletions == nil || *measured.Deletions != 2 {
		t.Fatalf("measured changes lost: %+v", measured)
	}
	// A deletion is retained as output: a successor that re-creates a file the
	// predecessor removed has not received its work.
	var deleted *domain.ArtifactFile
	for i := range got.Files {
		if got.Files[i].ChangeKind == domain.ArtifactDeleted {
			deleted = &got.Files[i]
		}
	}
	if deleted == nil || deleted.RelativePath != "internal/parser/legacy.go" {
		t.Fatalf("deleted path not retained: %+v", got.Files)
	}
	if deleted.ContentDigest != "" {
		t.Fatal("a deletion must not carry a content digest")
	}
}

// The manifest identity must change when the produced content changes, because
// a downstream handoff asserts it received this exact artifact version.
func TestArtifactVersionChangesWithContentAndIgnoresOrder(t *testing.T) {
	base := []domain.ArtifactFile{
		{RelativePath: "a.txt", ChangeKind: domain.ArtifactModified, ContentDigest: "digest-a"},
		{RelativePath: "b.txt", ChangeKind: domain.ArtifactAdded, ContentDigest: "digest-b"},
	}
	reordered := []domain.ArtifactFile{base[1], base[0]}
	if domain.ArtifactManifestDigest(base) != domain.ArtifactManifestDigest(reordered) {
		t.Fatal("artifact version must not depend on manifest order")
	}

	changed := []domain.ArtifactFile{
		{RelativePath: "a.txt", ChangeKind: domain.ArtifactModified, ContentDigest: "digest-a-prime"},
		base[1],
	}
	if domain.ArtifactManifestDigest(base) == domain.ArtifactManifestDigest(changed) {
		t.Fatal("changed content must change the artifact version")
	}

	// A deletion is part of the identity too.
	withDeletion := append(append([]domain.ArtifactFile{}, base...),
		domain.ArtifactFile{RelativePath: "c.txt", ChangeKind: domain.ArtifactDeleted})
	if domain.ArtifactManifestDigest(base) == domain.ArtifactManifestDigest(withDeletion) {
		t.Fatal("a deletion must change the artifact version")
	}
	mode := int64(0o755)
	withMode := append([]domain.ArtifactFile(nil), base...)
	withMode[0].FileMode = &mode
	if domain.ArtifactManifestDigest(base) == domain.ArtifactManifestDigest(withMode) {
		t.Fatal("an executable-mode change must change the artifact version")
	}
}

func TestFreezeRequiresAnExplicitCompleteReceipt(t *testing.T) {
	s := sqlitetest.MustOpen(t)
	ctx := context.Background()
	plan, outcomeID := seedApprovedPlan(t, s, "receipt-missing")
	attempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-receipt-missing", domain.FenceSubjectForProject("receipt-missing")))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FreezeAttemptReceipt(ctx, attempt.ID, time.Now().UTC()); !errors.Is(err, ports.ErrAttemptReceiptMissing) {
		t.Fatalf("freeze without receipt = %v, want missing receipt", err)
	}
	partial := receiptFixture(attempt.ID, outcomeID, plan, time.Now().UTC())
	partial.RetentionState = domain.RetentionIncomplete
	if err := s.SaveAttemptReceipt(ctx, partial); err != nil {
		t.Fatal(err)
	}
	if err := s.FreezeAttemptReceipt(ctx, attempt.ID, time.Now().UTC()); !errors.Is(err, ports.ErrAttemptReceiptNotReady) {
		t.Fatalf("freeze incomplete receipt = %v, want not ready", err)
	}
}

func TestClassifyAttemptSucceededCommitsReceiptObservationAndCustody(t *testing.T) {
	s := sqlitetest.MustOpen(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC)
	plan, outcomeID := seedApprovedPlan(t, s, "receipt-classify")
	attempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-receipt-classify", domain.FenceSubjectForProject("receipt-classify")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.TransitionAttemptStatus(ctx, outcomeID, attempt.ID, domain.AttemptQueued, domain.AttemptRunning, at); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TransitionAttemptStatus(ctx, outcomeID, attempt.ID, domain.AttemptRunning, domain.AttemptReconciled, at); err != nil {
		t.Fatal(err)
	}
	receipt := receiptFixture(attempt.ID, outcomeID, plan, at)
	if err := s.SaveAttemptReceipt(ctx, receipt); err != nil {
		t.Fatal(err)
	}
	if err := s.ClassifyAttemptSucceeded(ctx, classifyInput(outcomeID, attempt.ID, receipt.ArtifactVersion, at)); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.GetAttempt(ctx, outcomeID, attempt.ID)
	if err != nil || !ok || got.Status != domain.AttemptSucceeded {
		t.Fatalf("attempt after classification = %#v ok=%v err=%v", got, ok, err)
	}
	fence, held, err := s.OpenFenceForSubject(ctx, domain.FenceSubjectForProject("receipt-classify"))
	if err != nil || held || fence.AttemptID != "" {
		t.Fatalf("fence after classification = %#v held=%v err=%v", fence, held, err)
	}
	frozen, ok, err := s.GetAttemptReceipt(ctx, attempt.ID)
	if err != nil || !ok || !frozen.Frozen() {
		t.Fatalf("receipt after classification = %#v ok=%v err=%v", frozen, ok, err)
	}
	observations, err := s.ListAttemptObservations(ctx, attempt.ID)
	if err != nil || len(observations) != 1 || observations[0].Kind != domain.ObservationAttemptClassified {
		t.Fatalf("observations after classification = %#v err=%v", observations, err)
	}
}

// Freezing is what makes "what the owner reviewed" stable. A later retention
// pass over the same workspace must not be able to replace it.
func TestFrozenAttemptReceiptRefusesReplacement(t *testing.T) {
	s := sqlitetest.MustOpen(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
	plan, outcomeID := seedApprovedPlan(t, s, "receipt-freeze")
	attempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-receipt-freeze", domain.FenceSubjectForProject("receipt-freeze")))
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	reviewed := receiptFixture(attempt.ID, outcomeID, plan, at)
	if err := s.SaveAttemptReceipt(ctx, reviewed); err != nil {
		t.Fatalf("save receipt: %v", err)
	}
	if err := s.FreezeAttemptReceipt(ctx, attempt.ID, at.Add(time.Minute)); err != nil {
		t.Fatalf("freeze receipt: %v", err)
	}

	replacement := reviewed
	replacement.ArtifactVersion = "a-different-version"
	replacement.Files = []domain.ArtifactFile{{
		ID: "artifact-9", AttemptID: attempt.ID, RelativePath: "rewritten.go",
		ChangeKind: domain.ArtifactModified, ContentDigest: "other",
	}}
	if err := s.SaveAttemptReceipt(ctx, replacement); !errors.Is(err, ports.ErrAttemptReceiptFrozen) {
		t.Fatalf("save over frozen receipt = %v, want ErrAttemptReceiptFrozen", err)
	}

	// The reviewed manifest survives intact, not partially replaced.
	got, _, err := s.GetAttemptReceipt(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ArtifactVersion != reviewed.ArtifactVersion || len(got.Files) != 2 {
		t.Fatalf("frozen receipt was modified: %+v", got)
	}
	if !got.Frozen() {
		t.Fatal("receipt should report itself frozen")
	}
	// Freezing twice is a no-op so callers stay idempotent after a restart.
	if err := s.FreezeAttemptReceipt(ctx, attempt.ID, at.Add(2*time.Minute)); err != nil {
		t.Fatalf("re-freeze: %v", err)
	}
}

// Retention that ran into a bound is recorded as incomplete, and incomplete is
// never treated as a usable artifact.
func TestIncompleteRetentionIsRecordedAsIncomplete(t *testing.T) {
	s := sqlitetest.MustOpen(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
	plan, outcomeID := seedApprovedPlan(t, s, "receipt-partial")
	attempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-receipt-partial", domain.FenceSubjectForProject("receipt-partial")))
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	partial := receiptFixture(attempt.ID, outcomeID, plan, at)
	partial.RetentionState = domain.RetentionIncomplete
	partial.RetentionDetail = "stopped at the file-count bound"
	if err := s.SaveAttemptReceipt(ctx, partial); err != nil {
		t.Fatalf("save receipt: %v", err)
	}
	got, _, err := s.GetAttemptReceipt(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.RetentionState.Complete() {
		t.Fatal("incomplete retention must never satisfy a downstream handoff")
	}
	if got.RetentionDetail == "" {
		t.Fatal("an incomplete retention must say why")
	}
}

// A staged folder is not a worktree. Reporting a revision for one would
// misdescribe custody, so the domain refuses it before it can be persisted.
func TestStagedFolderReceiptCannotClaimRevisions(t *testing.T) {
	receipt := domain.AttemptReceipt{
		AttemptID: "att-1", OutcomeID: "out-1", PlanRevisionID: "plan-1", WorkUnitID: "wu-1",
		ContractRevisionNumber: 1, ArtifactVersion: "v", WorkspaceKind: domain.WorkspaceStagedFolder,
		RetentionState: domain.RetentionRetained, ObservedAt: time.Now(), BaseRevision: "sha",
	}
	if err := receipt.Validate(); err == nil {
		t.Fatal("a staged folder must not report a revision")
	}
}

// A receipt path must stay inside the workspace, so a later export cannot be
// pointed outside custody.
func TestArtifactPathCannotEscapeTheWorkspace(t *testing.T) {
	for _, path := range []string{"../outside.txt", "/etc/passwd", "nested/../../escape"} {
		file := domain.ArtifactFile{
			ID: "a", AttemptID: "att-1", RelativePath: path, ChangeKind: domain.ArtifactModified,
		}
		if err := file.Validate(); err == nil {
			t.Fatalf("path %q must be rejected", path)
		}
	}
}

// classifyInput is the shape a reconciler sends: the artifact it judged, the
// Contract revision it judged under, and when it read proof.
func classifyInput(outcomeID domain.OutcomeID, attemptID domain.AttemptID, artifactVersion string, at time.Time) ports.ClassifyAttemptInput {
	return ports.ClassifyAttemptInput{
		OutcomeID: outcomeID, AttemptID: attemptID, ExpectedStatus: domain.AttemptReconciled,
		ArtifactVersion: artifactVersion, ContractRevisionNumber: 1, ProofGeneration: new(int64),
		ObservationKind: domain.ObservationAttemptClassified, ObservationPayload: `{"result":"proved"}`, At: at,
	}
}

func reconciledAttemptWithReceipt(t *testing.T, s *sqlite.Store, project string, at time.Time) (domain.OutcomeID, domain.PlanRevision, domain.Attempt, domain.AttemptReceipt) {
	t.Helper()
	ctx := context.Background()
	plan, outcomeID := seedApprovedPlan(t, s, project)
	attempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-"+project, domain.FenceSubjectForProject(domain.ProjectID(project))))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.TransitionAttemptStatus(ctx, outcomeID, attempt.ID, domain.AttemptQueued, domain.AttemptRunning, at); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TransitionAttemptStatus(ctx, outcomeID, attempt.ID, domain.AttemptRunning, domain.AttemptReconciled, at); err != nil {
		t.Fatal(err)
	}
	receipt := receiptFixture(attempt.ID, outcomeID, plan, at)
	if err := s.SaveAttemptReceipt(ctx, receipt); err != nil {
		t.Fatal(err)
	}
	return outcomeID, plan, attempt, receipt
}

func assertStillReconciled(t *testing.T, s *sqlite.Store, outcomeID domain.OutcomeID, attemptID domain.AttemptID) {
	t.Helper()
	ctx := context.Background()
	got, ok, err := s.GetAttempt(ctx, outcomeID, attemptID)
	if err != nil || !ok {
		t.Fatalf("read attempt: %v ok=%v", err, ok)
	}
	if got.Status != domain.AttemptReconciled {
		t.Fatalf("status = %s, want the attempt left reconciled", got.Status)
	}
	receipt, ok, err := s.GetAttemptReceipt(ctx, attemptID)
	if err != nil || !ok {
		t.Fatalf("read receipt: %v ok=%v", err, ok)
	}
	if receipt.Frozen() {
		t.Fatal("a refused classification froze the receipt")
	}
}

// The judgement is made outside the transaction, so the transaction has to
// check that it still holds. Evidence and verifications are append-only: a
// contradiction landing after the reconciler read proof means the classification
// rests on a proof state that no longer exists.
func TestClassificationRefusesProofThatChangedAfterTheJudgement(t *testing.T) {
	s := sqlitetest.MustOpen(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC)
	outcomeID, _, attempt, receipt := reconciledAttemptWithReceipt(t, s, "classify-race", at)

	// A contradiction commits after the proof read, but its timestamp was
	// assigned earlier. Wall-clock comparison must not miss this writer.
	if err := s.CreateEvidenceItem(ctx, domain.EvidenceItem{
		ID: "ev-contradiction", OutcomeID: outcomeID, ContractRevisionID: "cr-classify-race",
		CriterionID: firstCriterionID(t, s, outcomeID), SubjectType: domain.ProofSubjectAttempt,
		SubjectID: string(attempt.ID), SubjectRevision: receipt.ArtifactVersion,
		Kind: domain.EvidenceContradicting, SourceType: domain.EvidenceSourceDeterministicCheck,
		SourceRef: "check", ProducerType: domain.EvidenceProducerTool, ProducerRef: "tool",
		Summary:       "the retained result does not satisfy the criterion",
		ContentDigest: strings.Repeat("a", 64), RequestKey: "rk-contradiction",
		RequestFingerprint: strings.Repeat("b", 64), CreatedAt: at.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("record contradiction: %v", err)
	}

	err := s.ClassifyAttemptSucceeded(ctx, classifyInput(outcomeID, attempt.ID, receipt.ArtifactVersion, at))
	if !errors.Is(err, ports.ErrAttemptClassificationStale) {
		t.Fatalf("err = %v, want the classification refused as stale", err)
	}
	assertStillReconciled(t, s, outcomeID, attempt.ID)
}

// Proof is bound to a Contract revision. If the Outcome has been revised since
// the judgement, that proof cannot classify work under the new revision.
func TestClassificationRefusesAfterTheContractIsRevised(t *testing.T) {
	s := sqlitetest.MustOpen(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC)
	outcomeID, _, attempt, receipt := reconciledAttemptWithReceipt(t, s, "classify-revised", at)

	if _, err := s.AppendContractRevision(ctx, outcomeID, 1, domain.ContractRevision{
		ID: "cr-classify-revised-2", OutcomeID: outcomeID, Number: 2,
		Goal: "Record focus locally, with weekly rollups.", SuccessCriteria: []string{"Blocks are recorded."},
		Review: "Deterministic checks.",
	}); err != nil {
		t.Fatalf("revise contract: %v", err)
	}

	err := s.ClassifyAttemptSucceeded(ctx, classifyInput(outcomeID, attempt.ID, receipt.ArtifactVersion, at))
	if !errors.Is(err, ports.ErrAttemptClassificationStale) {
		t.Fatalf("err = %v, want the classification refused as stale", err)
	}
	assertStillReconciled(t, s, outcomeID, attempt.ID)
}

// A classification that names neither the revision it judged under nor when it
// read proof is a second unchecked read, and is refused outright.
func TestClassificationRequiresItsJudgementContext(t *testing.T) {
	s := sqlitetest.MustOpen(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC)
	outcomeID, _, attempt, receipt := reconciledAttemptWithReceipt(t, s, "classify-context", at)

	bare := classifyInput(outcomeID, attempt.ID, receipt.ArtifactVersion, at)
	bare.ContractRevisionNumber = 0
	bare.ProofGeneration = nil
	if err := s.ClassifyAttemptSucceeded(ctx, bare); err == nil {
		t.Fatal("classification without its judgement context was accepted")
	}
	assertStillReconciled(t, s, outcomeID, attempt.ID)
}

func firstCriterionID(t *testing.T, s *sqlite.Store, outcomeID domain.OutcomeID) domain.CriterionID {
	t.Helper()
	revisions, err := s.ListContractRevisions(context.Background(), outcomeID)
	if err != nil || len(revisions) == 0 {
		t.Fatalf("read contract: %v revisions=%d", err, len(revisions))
	}
	current := revisions[len(revisions)-1]
	if len(current.Criteria) == 0 {
		t.Fatal("contract fixture has no criteria")
	}
	return current.Criteria[0].ID
}

func TestFrozenAttemptReceiptMeasurementsCannotBeRewritten(t *testing.T) {
	s := sqlitetest.MustOpen(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	plan, outcomeID := seedApprovedPlan(t, s, "frozen-measurements")
	attempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-frozen-measurements", domain.FenceSubjectForProject("frozen-measurements")))
	if err != nil {
		t.Fatal(err)
	}
	receipt := receiptFixture(attempt.ID, outcomeID, plan, now)
	add, del := int64(2), int64(1)
	receipt.Files[0].Additions, receipt.Files[0].Deletions = &add, &del
	if err := s.SaveAttemptReceipt(ctx, receipt); err != nil {
		t.Fatal(err)
	}
	if err := s.FreezeAttemptReceipt(ctx, attempt.ID, now); err != nil {
		t.Fatal(err)
	}
	changed := receipt
	other := int64(9)
	changed.Files = append([]domain.ArtifactFile(nil), receipt.Files...)
	changed.Files[0].Additions = &other
	if err := s.SaveAttemptReceipt(ctx, changed); !errors.Is(err, ports.ErrAttemptReceiptFrozen) {
		t.Fatalf("measurement rewrite err=%v", err)
	}
}

// Two retention paths racing one Attempt converge on one canonical snapshot:
// a durable COMPLETE receipt accepts an identical replay and refuses a
// divergent one before anything is overwritten. An incomplete receipt may
// still be replaced by the retry that finishes it.
func TestSaveAttemptReceiptCompleteDivergentRefused(t *testing.T) {
	s := sqlitetest.MustOpen(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
	plan, outcomeID := seedApprovedPlan(t, s, "receipt-cas")
	attempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-receipt-cas", domain.FenceSubjectForProject("receipt-cas")))
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	canonical := receiptFixture(attempt.ID, outcomeID, plan, at)
	if err := s.SaveAttemptReceipt(ctx, canonical); err != nil {
		t.Fatalf("save canonical receipt: %v", err)
	}

	// Identical replay (a restart retry) is a no-op.
	replay := canonical
	replay.UpdatedAt = at.Add(time.Hour)
	if err := s.SaveAttemptReceipt(ctx, replay); err != nil {
		t.Fatalf("identical replay must be accepted: %v", err)
	}

	// A well-formed but divergent receipt is refused BEFORE overwriting.
	divergent := canonical
	divergent.Files = []domain.ArtifactFile{{
		ID: "artifact-9", AttemptID: attempt.ID, RelativePath: "rewritten.go",
		ChangeKind: domain.ArtifactModified, ContentDigest: "other",
	}}
	divergent.ArtifactVersion = string(domain.ArtifactManifestDigest(divergent.Files))
	if err := s.SaveAttemptReceipt(ctx, divergent); !errors.Is(err, ports.ErrAttemptReceiptDiverged) {
		t.Fatalf("divergent save = %v, want ErrAttemptReceiptDiverged", err)
	}
	got, _, err := s.GetAttemptReceipt(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ArtifactVersion != canonical.ArtifactVersion || len(got.Files) != 2 {
		t.Fatalf("canonical receipt was modified by the refused save: %+v", got)
	}

	// An incomplete receipt carries no single-winner guarantee: the retry
	// that finishes it replaces it wholesale.
	incompleteAttempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-receipt-cas-2", domain.FenceSubjectForProject("receipt-cas-2")))
	if err != nil {
		t.Fatalf("create second attempt: %v", err)
	}
	partial := receiptFixture(incompleteAttempt.ID, outcomeID, plan, at)
	for i := range partial.Files {
		partial.Files[i].ID = partial.Files[i].ID + "-inc"
	}
	partial.ArtifactVersion = string(domain.ArtifactManifestDigest(partial.Files))
	partial.RetentionState = domain.RetentionIncomplete
	if err := s.SaveAttemptReceipt(ctx, partial); err != nil {
		t.Fatalf("save incomplete receipt: %v", err)
	}
	finished := partial
	finished.RetentionState = domain.RetentionRetained
	if err := s.SaveAttemptReceipt(ctx, finished); err != nil {
		t.Fatalf("finishing retry must replace the incomplete receipt: %v", err)
	}
	got, _, err = s.GetAttemptReceipt(ctx, incompleteAttempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ArtifactVersion != partial.ArtifactVersion || got.RetentionState != domain.RetentionRetained {
		t.Fatalf("incomplete receipt not finished: %+v", got)
	}
}

// The single-winner compare-and-set judges the whole canonical receipt, not
// the version string alone. Two receipts can share an ArtifactVersion and
// still disagree about everything downstream custody seals.
func TestSaveAttemptReceiptSameVersionDifferentMetadataRefused(t *testing.T) {
	s := sqlitetest.MustOpen(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 9, 11, 0, 0, 0, time.UTC)
	plan, outcomeID := seedApprovedPlan(t, s, "receipt-cas-meta")
	attempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-receipt-cas-meta", domain.FenceSubjectForProject("receipt-cas-meta")))
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	canonical := receiptFixture(attempt.ID, outcomeID, plan, at)
	if err := s.SaveAttemptReceipt(ctx, canonical); err != nil {
		t.Fatalf("save canonical receipt: %v", err)
	}

	// Same ArtifactVersion, divergent sealed metadata: refused as divergent,
	// never accepted as an identical replay.
	meta := canonical
	meta.ResultRevision = "a-different-result"
	meta.TerminationReason = "a-different-reason"
	meta.RetentionDetail = "a-different-detail"
	meta.ObservedAt = at.Add(5 * time.Minute)
	if err := s.SaveAttemptReceipt(ctx, meta); !errors.Is(err, ports.ErrAttemptReceiptDiverged) {
		t.Fatalf("same-version/different-metadata save = %v, want ErrAttemptReceiptDiverged", err)
	}
	got, _, err := s.GetAttemptReceipt(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResultRevision != canonical.ResultRevision || got.TerminationReason != canonical.TerminationReason || got.RetentionDetail != canonical.RetentionDetail || !got.ObservedAt.Equal(canonical.ObservedAt) {
		t.Fatalf("canonical receipt was modified by the refused save: %+v", got)
	}

	// A malformed receipt must not bypass validation just because its
	// version string matches the durable one.
	malformed := canonical
	malformed.ArtifactVersion = canonical.ArtifactVersion
	malformed.Files = nil
	if err := malformed.Validate(); err == nil {
		t.Fatalf("fixture malformed receipt unexpectedly validates")
	}
	if err := s.SaveAttemptReceipt(ctx, malformed); err == nil {
		t.Fatalf("malformed same-version receipt bypassed validation")
	}
	if errors.Is(err, ports.ErrAttemptReceiptDiverged) {
		t.Fatalf("malformed receipt was misreported as divergent rather than invalid: %v", err)
	}
}

// Line metrics are durable custody facts: ArtifactManifestDigest does not
// cover Additions/Deletions, so two valid complete receipts can share an
// ArtifactVersion while disagreeing about measured line counts. The
// compare-and-set must refuse the second one, and a nil (unmeasured) count
// is a different fact from a measured zero.
func TestSaveAttemptReceiptSameVersionDivergentLineMetricsRefused(t *testing.T) {
	s := sqlitetest.MustOpen(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	plan, outcomeID := seedApprovedPlan(t, s, "receipt-cas-metrics")
	attempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-receipt-cas-metrics", domain.FenceSubjectForProject("receipt-cas-metrics")))
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	canonical := receiptFixture(attempt.ID, outcomeID, plan, at)
	if err := s.SaveAttemptReceipt(ctx, canonical); err != nil {
		t.Fatalf("save canonical receipt: %v", err)
	}

	// Same ArtifactVersion, only line measurements differ: refused, and the
	// durable metrics stay exactly as first recorded.
	measured := canonical
	measured.Files = append([]domain.ArtifactFile(nil), canonical.Files...)
	five, three := int64(5), int64(3)
	measured.Files[0].Additions = &five
	measured.Files[0].Deletions = &three
	if err := s.SaveAttemptReceipt(ctx, measured); !errors.Is(err, ports.ErrAttemptReceiptDiverged) {
		t.Fatalf("metrics-only divergent save = %v, want ErrAttemptReceiptDiverged", err)
	}
	got, _, err := s.GetAttemptReceipt(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Files[0].Additions != nil || got.Files[0].Deletions != nil {
		t.Fatalf("durable metrics were replaced by the refused save: %+v", got.Files[0])
	}

	// A measured zero is a different fact from unmeasured (nil), and from a
	// measured non-zero.
	second, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-receipt-cas-metrics-2", domain.FenceSubjectForProject("receipt-cas-metrics-2")))
	if err != nil {
		t.Fatalf("create second attempt: %v", err)
	}
	zero := receiptFixture(second.ID, outcomeID, plan, at)
	zero.Files = append([]domain.ArtifactFile(nil), zero.Files...)
	zeroA, zeroD := int64(0), int64(0)
	zero.Files[0].Additions = &zeroA
	zero.Files[0].Deletions = &zeroD
	zero.Files[0].ID = "artifact-1z"
	zero.Files[1].ID = "artifact-2z"
	if err := s.SaveAttemptReceipt(ctx, zero); err != nil {
		t.Fatalf("save measured-zero receipt: %v", err)
	}
	unmeasured := zero
	unmeasured.Files = append([]domain.ArtifactFile(nil), zero.Files...)
	unmeasured.Files[0].Additions = nil
	unmeasured.Files[0].Deletions = nil
	if err := s.SaveAttemptReceipt(ctx, unmeasured); !errors.Is(err, ports.ErrAttemptReceiptDiverged) {
		t.Fatalf("nil-vs-zero metrics save = %v, want ErrAttemptReceiptDiverged", err)
	}
	got, _, err = s.GetAttemptReceipt(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	var parseEntry *domain.ArtifactFile
	for i := range got.Files {
		if got.Files[i].RelativePath == "internal/parser/parse.go" {
			parseEntry = &got.Files[i]
		}
	}
	if parseEntry == nil || parseEntry.Additions == nil || *parseEntry.Additions != 0 {
		t.Fatalf("durable measured-zero was replaced by the refused save: %+v", got.Files)
	}
}
