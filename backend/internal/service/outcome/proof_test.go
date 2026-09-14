package outcome_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apierr"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

type proofFakeStore struct {
	*fakeStore
	receiptBook   *receiptBook
	proofMu       sync.Mutex
	evidence      []domain.EvidenceItem
	verifications []domain.VerificationRun
	decisions     []domain.AcceptanceDecision
	corrections   []domain.OutcomeCorrection
}

func newProofService(t *testing.T) (*outcome.Service, *proofFakeStore, outcome.View) {
	t.Helper()
	store := &proofFakeStore{fakeStore: newFakeStore()}
	now := time.Date(2026, 8, 25, 20, 0, 0, 0, time.UTC)
	service := outcome.New(store, func() time.Time {
		now = now.Add(time.Minute)
		return now
	}).WithProofStore(store)
	in := validCreateInput()
	in.SuccessCriteria = []string{"A block can be recorded.", "The block survives restart."}
	view, err := service.Create(context.Background(), in)
	if err != nil {
		t.Fatalf("create outcome: %v", err)
	}
	if len(view.Current.Criteria) != 2 {
		t.Fatalf("stable criteria = %+v", view.Current.Criteria)
	}
	return service, store, view
}

func evidenceInput(view outcome.View, criterion domain.ContractCriterion, key string) outcome.RecordEvidenceInput {
	return outcome.RecordEvidenceInput{
		ExpectedContractRevision: view.Current.Number,
		ContractRevisionID:       view.Current.ID,
		CriterionID:              criterion.ID,
		SubjectType:              domain.ProofSubjectOutcome,
		SubjectID:                string(view.Outcome.ID),
		SubjectRevision:          string(view.Current.ID),
		Kind:                     domain.EvidenceSupporting,
		SourceType:               domain.EvidenceSourceOwnerWalkthrough,
		SourceRef:                "walkthrough-" + key,
		ProducerType:             domain.EvidenceProducerUser,
		ProducerRef:              "owner",
		Summary:                  "Observed the criterion directly.",
		ContentDigest:            "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		RequestKey:               key,
	}
}

func verificationInput(view outcome.View, criterion domain.ContractCriterion, evidenceID domain.EvidenceItemID, key string) outcome.RecordVerificationInput {
	return outcome.RecordVerificationInput{
		ExpectedContractRevision: view.Current.Number,
		ContractRevisionID:       view.Current.ID,
		CriterionID:              criterion.ID,
		SubjectType:              domain.ProofSubjectOutcome,
		SubjectID:                string(view.Outcome.ID),
		SubjectRevision:          string(view.Current.ID),
		EvidenceItemIDs:          []domain.EvidenceItemID{evidenceID},
		Method:                   "Owner walkthrough against the stated criterion.",
		IndependenceClass:        domain.VerificationOwnerWalkthrough,
		Result:                   domain.VerificationPassed,
		VerifierRef:              "owner",
		RequestKey:               key,
	}
}

func TestProofRequiresExactCriterionAndExplicitUserAcceptance(t *testing.T) {
	service, store, view := newProofService(t)
	ctx := context.Background()

	for i, criterion := range view.Current.Criteria {
		proof, err := service.RecordEvidence(ctx, view.Outcome.ID, evidenceInput(view, criterion, "ev-key-"+string(rune('a'+i))))
		if err != nil {
			t.Fatalf("record evidence %d: %v", i, err)
		}
		item := proof.Criteria[i].Evidence[len(proof.Criteria[i].Evidence)-1]
		proof, err = service.RecordVerification(ctx, view.Outcome.ID, verificationInput(view, criterion, item.ID, "ver-key-"+string(rune('a'+i))))
		if err != nil {
			t.Fatalf("record verification %d: %v", i, err)
		}
		if i == 0 && proof.Status != outcome.ProofStatusActive {
			t.Fatalf("one proven criterion status = %s, want active", proof.Status)
		}
	}
	proof, err := service.GetProof(ctx, view.Outcome.ID)
	if err != nil {
		t.Fatalf("get proof: %v", err)
	}
	if proof.Status != outcome.ProofStatusReadyForAcceptance {
		t.Fatalf("status = %s, want ready_for_acceptance", proof.Status)
	}

	accepted, err := service.DecideAcceptance(ctx, view.Outcome.ID, outcome.DecideAcceptanceInput{
		ExpectedContractRevision: view.Current.Number,
		ContractRevisionID:       view.Current.ID,
		Kind:                     domain.AcceptanceAccept,
		Summary:                  "I reviewed both criteria and accept the Outcome.",
		ResourceDisposition:      domain.ResourceDispositionRetain,
		RequestKey:               "accept-key",
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if accepted.Status != outcome.ProofStatusAccepted || accepted.Decisions[len(accepted.Decisions)-1].ActorType != domain.AcceptanceActorUser {
		t.Fatalf("accepted proof = %+v", accepted)
	}
	if len(store.decisions) != 1 {
		t.Fatalf("decision writes = %d, want 1", len(store.decisions))
	}
	if _, err := service.DecideAcceptance(ctx, view.Outcome.ID, outcome.DecideAcceptanceInput{
		ExpectedContractRevision: view.Current.Number,
		ContractRevisionID:       view.Current.ID,
		Kind:                     domain.AcceptanceAccept,
		Summary:                  "I reviewed both criteria and accept the Outcome.",
		ResourceDisposition:      domain.ResourceDispositionRetain,
		RequestKey:               "accept-key",
	}); err != nil || len(store.decisions) != 1 {
		t.Fatalf("identical acceptance replay err=%v writes=%d", err, len(store.decisions))
	}
}

func TestVerificationRejectsCrossCriterionBindingAndFalseIndependence(t *testing.T) {
	service, _, view := newProofService(t)
	ctx := context.Background()
	proof, err := service.RecordEvidence(ctx, view.Outcome.ID, evidenceInput(view, view.Current.Criteria[0], "evidence-binding"))
	if err != nil {
		t.Fatalf("record evidence: %v", err)
	}
	evidenceID := proof.Criteria[0].Evidence[0].ID

	cross := verificationInput(view, view.Current.Criteria[1], evidenceID, "verification-cross")
	_, err = service.RecordVerification(ctx, view.Outcome.ID, cross)
	assertAPIErrorCode(t, err, "EVIDENCE_BINDING_MISMATCH")

	falseClaim := verificationInput(view, view.Current.Criteria[0], evidenceID, "verification-false-claim")
	falseClaim.IndependenceClass = domain.VerificationSeparateSession
	falseClaim.ProducerRef = "session-1"
	falseClaim.VerifierRef = "session-1"
	_, err = service.RecordVerification(ctx, view.Outcome.ID, falseClaim)
	assertAPIErrorCode(t, err, "VERIFICATION_INVALID")
}

func TestReopenCreatesCorrectionAndRequiresFreshProof(t *testing.T) {
	service, store, view := newProofService(t)
	ctx := context.Background()
	for i, criterion := range view.Current.Criteria {
		proof, err := service.RecordEvidence(ctx, view.Outcome.ID, evidenceInput(view, criterion, "before-reopen-ev-"+string(rune('a'+i))))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.RecordVerification(ctx, view.Outcome.ID, verificationInput(view, criterion, proof.Criteria[i].Evidence[0].ID, "before-reopen-ver-"+string(rune('a'+i)))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.DecideAcceptance(ctx, view.Outcome.ID, outcome.DecideAcceptanceInput{
		ExpectedContractRevision: view.Current.Number, ContractRevisionID: view.Current.ID,
		Kind: domain.AcceptanceAccept, Summary: "Accepted after review.", ResourceDisposition: domain.ResourceDispositionRetain, RequestKey: "accept-before-reopen",
	}); err != nil {
		t.Fatal(err)
	}
	_, err := service.DecideAcceptance(ctx, view.Outcome.ID, outcome.DecideAcceptanceInput{
		ExpectedContractRevision: view.Current.Number, ContractRevisionID: view.Current.ID,
		Kind: domain.AcceptanceReopen, Summary: "This must not point outside the lineage.", ResourceDisposition: domain.ResourceDispositionRetain,
		ReentryTargetType: domain.ReentryTargetContract, ReentryTargetID: "another-contract", RequestKey: "reopen-invalid-target",
	})
	assertAPIErrorCode(t, err, "REENTRY_TARGET_MISMATCH")

	reopened, err := service.DecideAcceptance(ctx, view.Outcome.ID, outcome.DecideAcceptanceInput{
		ExpectedContractRevision: view.Current.Number, ContractRevisionID: view.Current.ID,
		Kind: domain.AcceptanceReopen, Summary: "The midnight boundary needs another run.", ResourceDisposition: domain.ResourceDispositionRetain,
		ReentryTargetType: domain.ReentryTargetContract, ReentryTargetID: string(view.Current.ID), RequestKey: "reopen-key",
	})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.Status != outcome.ProofStatusReworkRequired || len(store.corrections) != 1 {
		t.Fatalf("reopened proof=%+v corrections=%+v", reopened, store.corrections)
	}
	if _, err := service.DecideAcceptance(ctx, view.Outcome.ID, outcome.DecideAcceptanceInput{
		ExpectedContractRevision: view.Current.Number, ContractRevisionID: view.Current.ID,
		Kind: domain.AcceptanceAccept, Summary: "Old proof should not count.", ResourceDisposition: domain.ResourceDispositionRetain, RequestKey: "premature-reaccept",
	}); err == nil {
		t.Fatal("acceptance must stay blocked until proof after the reopen horizon")
	}
}

func TestProofForSupersededContractCannotReadyCurrentRevision(t *testing.T) {
	service, _, view := newProofService(t)
	ctx := context.Background()
	criterion := view.Current.Criteria[0]
	proof, err := service.RecordEvidence(ctx, view.Outcome.ID, evidenceInput(view, criterion, "old-proof-evidence"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordVerification(ctx, view.Outcome.ID, verificationInput(view, criterion, proof.Criteria[0].Evidence[0].ID, "old-proof-verification")); err != nil {
		t.Fatal(err)
	}

	revised, err := service.ReviseContract(ctx, view.Outcome.ID, outcome.ReviseContractInput{
		ExpectedRevision: view.Current.Number,
		Goal:             view.Current.Goal,
		SuccessCriteria:  []string{"The revised criterion is proven."},
		Review:           view.Current.Review,
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := service.GetProof(ctx, view.Outcome.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Contract.ID != revised.Current.ID || current.Status != outcome.ProofStatusActive || current.Criteria[0].Ready {
		t.Fatalf("superseded proof leaked into current view: %+v", current)
	}
}

func assertAPIErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var apiError *apierr.Error
	if !errors.As(err, &apiError) || apiError.Code != code {
		t.Fatalf("err=%v, want api code %s", err, code)
	}
}

func (f *proofFakeStore) CreateEvidenceItem(_ context.Context, item domain.EvidenceItem) error {
	f.proofMu.Lock()
	defer f.proofMu.Unlock()
	for _, existing := range f.evidence {
		if existing.RequestKey == item.RequestKey {
			return errors.New("duplicate evidence request key")
		}
	}
	f.evidence = append(f.evidence, item)
	return nil
}

func (f *proofFakeStore) FindEvidenceItemByRequestKey(_ context.Context, key string) (domain.EvidenceItem, bool, error) {
	f.proofMu.Lock()
	defer f.proofMu.Unlock()
	for _, item := range f.evidence {
		if item.RequestKey == key {
			return item, true, nil
		}
	}
	return domain.EvidenceItem{}, false, nil
}

func (f *proofFakeStore) GetEvidenceItem(_ context.Context, outcomeID domain.OutcomeID, id domain.EvidenceItemID) (domain.EvidenceItem, bool, error) {
	f.proofMu.Lock()
	defer f.proofMu.Unlock()
	for _, item := range f.evidence {
		if item.OutcomeID == outcomeID && item.ID == id {
			return item, true, nil
		}
	}
	return domain.EvidenceItem{}, false, nil
}

func (f *proofFakeStore) ListEvidenceItems(_ context.Context, outcomeID domain.OutcomeID) ([]domain.EvidenceItem, error) {
	f.proofMu.Lock()
	defer f.proofMu.Unlock()
	var out []domain.EvidenceItem
	for _, item := range f.evidence {
		if item.OutcomeID == outcomeID {
			out = append(out, item)
		}
	}
	return out, nil
}

func (f *proofFakeStore) CreateVerificationRun(_ context.Context, run domain.VerificationRun) error {
	f.proofMu.Lock()
	defer f.proofMu.Unlock()
	for _, existing := range f.verifications {
		if existing.RequestKey == run.RequestKey {
			return errors.New("duplicate verification request key")
		}
	}
	f.verifications = append(f.verifications, run)
	return nil
}

func (f *proofFakeStore) FindVerificationRunByRequestKey(_ context.Context, key string) (domain.VerificationRun, bool, error) {
	f.proofMu.Lock()
	defer f.proofMu.Unlock()
	for _, run := range f.verifications {
		if run.RequestKey == key {
			return run, true, nil
		}
	}
	return domain.VerificationRun{}, false, nil
}

func (f *proofFakeStore) ListVerificationRuns(_ context.Context, outcomeID domain.OutcomeID) ([]domain.VerificationRun, error) {
	f.proofMu.Lock()
	defer f.proofMu.Unlock()
	var out []domain.VerificationRun
	for _, run := range f.verifications {
		if run.OutcomeID == outcomeID {
			out = append(out, run)
		}
	}
	return out, nil
}

func (f *proofFakeStore) CreateAcceptanceDecision(_ context.Context, decision domain.AcceptanceDecision, correction *domain.OutcomeCorrection) error {
	f.proofMu.Lock()
	defer f.proofMu.Unlock()
	for _, existing := range f.decisions {
		if existing.RequestKey == decision.RequestKey {
			return errors.New("duplicate decision request key")
		}
	}
	f.decisions = append(f.decisions, decision)
	if correction != nil {
		f.corrections = append(f.corrections, *correction)
	}
	return nil
}

func (f *proofFakeStore) FindAcceptanceDecisionByRequestKey(_ context.Context, key string) (domain.AcceptanceDecision, bool, error) {
	f.proofMu.Lock()
	defer f.proofMu.Unlock()
	for _, decision := range f.decisions {
		if decision.RequestKey == key {
			return decision, true, nil
		}
	}
	return domain.AcceptanceDecision{}, false, nil
}

func (f *proofFakeStore) ListAcceptanceDecisions(_ context.Context, outcomeID domain.OutcomeID) ([]domain.AcceptanceDecision, error) {
	f.proofMu.Lock()
	defer f.proofMu.Unlock()
	var out []domain.AcceptanceDecision
	for _, decision := range f.decisions {
		if decision.OutcomeID == outcomeID {
			out = append(out, decision)
		}
	}
	return out, nil
}

func (f *proofFakeStore) ListOutcomeCorrections(_ context.Context, outcomeID domain.OutcomeID) ([]domain.OutcomeCorrection, error) {
	f.proofMu.Lock()
	defer f.proofMu.Unlock()
	var out []domain.OutcomeCorrection
	for _, correction := range f.corrections {
		if correction.OutcomeID == outcomeID {
			out = append(out, correction)
		}
	}
	return out, nil
}

// CreateAcceptanceDecisionBatch mirrors storage: every decision stays its own
// record, and the whole sitting is all-or-nothing.
func (f *proofFakeStore) CreateAcceptanceDecisionBatch(_ context.Context, decisions []domain.AcceptanceDecision) error {
	if len(decisions) == 0 {
		return errors.New("an acceptance batch requires at least one decision")
	}
	f.proofMu.Lock()
	defer f.proofMu.Unlock()
	for _, decision := range decisions {
		if err := decision.Validate(); err != nil {
			return err
		}
		for _, existing := range f.decisions {
			if existing.RequestKey == decision.RequestKey {
				return errors.New("duplicate decision request key")
			}
		}
	}
	f.decisions = append(f.decisions, decisions...)
	return nil
}

// receiptBook gives proofFakeStore the AttemptReceiptStore surface so the
// Result facts projection can be exercised against durable attempts and
// Kennel-retained receipts.
type receiptBook struct {
	mu       sync.Mutex
	attempts []domain.Attempt
	receipts map[domain.AttemptID]domain.AttemptReceipt
}

func (f *proofFakeStore) withAttemptReceipts(book *receiptBook) { f.receiptBook = book }

func (f *proofFakeStore) ListAttempts(context.Context, domain.OutcomeID) ([]domain.Attempt, error) {
	if f.receiptBook == nil {
		return nil, nil
	}
	f.receiptBook.mu.Lock()
	defer f.receiptBook.mu.Unlock()
	return append([]domain.Attempt(nil), f.receiptBook.attempts...), nil
}

func (f *proofFakeStore) SaveAttemptReceipt(_ context.Context, receipt domain.AttemptReceipt) error {
	f.receiptBook.mu.Lock()
	defer f.receiptBook.mu.Unlock()
	f.receiptBook.receipts[receipt.AttemptID] = receipt
	return nil
}

func (f *proofFakeStore) GetAttemptReceipt(_ context.Context, id domain.AttemptID) (domain.AttemptReceipt, bool, error) {
	f.receiptBook.mu.Lock()
	defer f.receiptBook.mu.Unlock()
	receipt, ok := f.receiptBook.receipts[id]
	return receipt, ok, nil
}

func (f *proofFakeStore) FreezeAttemptReceipt(_ context.Context, id domain.AttemptID, at time.Time) error {
	f.receiptBook.mu.Lock()
	defer f.receiptBook.mu.Unlock()
	receipt, ok := f.receiptBook.receipts[id]
	if !ok {
		return nil
	}
	frozen := at.UTC()
	receipt.FrozenAt = &frozen
	f.receiptBook.receipts[id] = receipt
	return nil
}

func TestGetProofProjectsMeasuredChangesAndReentryTargets(t *testing.T) {
	service, store, view := newProofService(t)
	book := &receiptBook{receipts: map[domain.AttemptID]domain.AttemptReceipt{}}
	store.withAttemptReceipts(book)
	current := domain.Attempt{
		ID:                     "att-current",
		OutcomeID:              view.Outcome.ID,
		WorkUnitID:             "wu-1",
		ContractRevisionNumber: view.Current.Number,
		Status:                 domain.AttemptSucceeded,
	}
	superseded := domain.Attempt{
		ID:                     "att-old",
		OutcomeID:              view.Outcome.ID,
		WorkUnitID:             "wu-1",
		ContractRevisionNumber: view.Current.Number + 1,
		Status:                 domain.AttemptSucceeded,
	}
	book.attempts = []domain.Attempt{current, superseded}
	book.receipts[current.ID] = domain.AttemptReceipt{
		AttemptID:       current.ID,
		ArtifactVersion: "v1",
		RetentionState:  domain.RetentionRetained,
		Files: []domain.ArtifactFile{
			{RelativePath: "report.md", ChangeKind: domain.ArtifactModified, ContentDigest: "digest-report"},
			{RelativePath: "notes/todo.md", ChangeKind: domain.ArtifactAdded, ContentDigest: "digest-notes"},
		},
	}
	book.receipts[superseded.ID] = domain.AttemptReceipt{
		AttemptID:       superseded.ID,
		ArtifactVersion: "v9",
		RetentionState:  domain.RetentionRetained,
		Files:           []domain.ArtifactFile{{RelativePath: "stale.md", ChangeKind: domain.ArtifactAdded, ContentDigest: "digest-stale"}},
	}

	proof, err := service.GetProof(context.Background(), view.Outcome.ID)
	if err != nil {
		t.Fatalf("get proof: %v", err)
	}
	if len(proof.Changes) != 1 {
		t.Fatalf("expected changes only for the current-revision attempt, got %+v", proof.Changes)
	}
	changes := proof.Changes[0]
	if changes.AttemptID != current.ID || changes.ArtifactVersion != "v1" || changes.RetentionState != domain.RetentionRetained {
		t.Fatalf("change identity = %+v", changes)
	}
	if changes.Truncated || len(changes.Files) != 2 {
		t.Fatalf("change files = %+v truncated=%v", changes.Files, changes.Truncated)
	}
	if changes.Files[0].RelativePath != "report.md" || changes.Files[0].ChangeKind != domain.ArtifactModified {
		t.Fatalf("first change = %+v", changes.Files[0])
	}

	var contractTarget, attemptTarget *outcome.ReentryTargetView
	for i := range proof.ReentryTargets {
		target := &proof.ReentryTargets[i]
		switch {
		case target.TargetType == domain.ReentryTargetContract && target.TargetID == string(view.Current.ID):
			contractTarget = target
		case target.TargetType == domain.ReentryTargetAttempt && target.TargetID == string(current.ID):
			attemptTarget = target
		}
		if target.TargetID == string(superseded.ID) {
			t.Fatalf("superseded attempt must not be a re-entry target: %+v", target)
		}
	}
	if contractTarget == nil || attemptTarget == nil {
		t.Fatalf("re-entry targets = %+v", proof.ReentryTargets)
	}
}
