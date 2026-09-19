package outcome

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apierr"
)

// ProofManager records evidence, verification, and owner acceptance decisions.
type ProofManager interface {
	GetProof(context.Context, domain.OutcomeID) (ProofView, error)
	RecordEvidence(context.Context, domain.OutcomeID, RecordEvidenceInput) (ProofView, error)
	RecordVerification(context.Context, domain.OutcomeID, RecordVerificationInput) (ProofView, error)
	DecideAcceptance(context.Context, domain.OutcomeID, DecideAcceptanceInput) (ProofView, error)
	BatchEligibility(context.Context, domain.OutcomeID) ([]domain.BatchEntryVerdict, error)
	AcceptContributorBatch(context.Context, domain.OutcomeID, AcceptBatchInput) (AcceptBatchView, error)
}

// ProofStatus is the lifecycle state of an Outcome's proof projection.
type ProofStatus string

const (
	// ProofStatusActive indicates that proof collection remains open.
	ProofStatusActive ProofStatus = "active"
	// ProofStatusReadyForAcceptance indicates proof is ready for owner review.
	ProofStatusReadyForAcceptance ProofStatus = "ready_for_acceptance"
	// ProofStatusAccepted indicates the owner accepted the Outcome.
	ProofStatusAccepted ProofStatus = "accepted"
	// ProofStatusReworkRequired indicates proof does not support acceptance.
	ProofStatusReworkRequired ProofStatus = "rework_required"
)

// CriterionProofView projects proof state for one acceptance criterion.
type CriterionProofView struct {
	Criterion     domain.ContractCriterion
	Evidence      []domain.EvidenceItem
	Verifications []domain.VerificationRun
	Ready         bool
	Gap           string
	Delegated     bool               `json:"delegated,omitempty"`
	ClaimedBy     []domain.OutcomeID `json:"claimedBy,omitempty"`
}

// ProofView is the service projection of Outcome proof.
type ProofView struct {
	OutcomeID   domain.OutcomeID
	Contract    domain.ContractRevision
	Status      ProofStatus
	NextAction  string
	Criteria    []CriterionProofView
	Decisions   []domain.AcceptanceDecision
	Corrections []domain.OutcomeCorrection
	// ActiveCorrection is the correction attached to the decision that set the
	// horizon — what the owner most recently asked to be changed. Nil when no
	// rework or reopen stands against the current Contract revision.
	ActiveCorrection *domain.OutcomeCorrection
	ProofHorizon     time.Time
	// Changes is the measured file-change projection for retained Attempts on
	// the current Contract revision, so the Result can say what changed, not
	// just that bytes were retained. Empty when nothing is retained yet.
	Changes []AttemptChangesView
	// ReentryTargets are the daemon-derived identities a correction may target
	// on the current Contract revision, so rework and reopen never ask the
	// owner to type a raw identifier.
	ReentryTargets []ReentryTargetView
	// LineageStaleness reports Attempts and WorkUnits whose proving lineage is
	// superseded by upstream rework. Evidence and verifications bound to a
	// stale Attempt are already excluded from Criteria; this explains why, and
	// is what the scheduler and mission projection surface.
	LineageStaleness domain.LineageStaleness
}

// AttemptChangesView projects one retained Attempt's measured file changes.
// Files come from Kennel's own receipt of the leased workspace, never from
// provider claims. Truncated is explicit when the projection shortens the
// list; the receipt itself stays complete.
type AttemptChangesView struct {
	AttemptID       domain.AttemptID
	WorkUnitID      domain.WorkUnitID
	ArtifactVersion string
	RetentionState  domain.RetentionState
	Files           []domain.ArtifactFile
	Truncated       bool
}

// ReentryTargetView is one daemon-derived correction target for the current
// Contract revision.
type ReentryTargetView struct {
	TargetType domain.ReentryTargetType
	TargetID   string
	Label      string
}

// RecordEvidenceInput contains evidence tied to an Outcome criterion.
type RecordEvidenceInput struct {
	ExpectedContractRevision int64
	ContractRevisionID       domain.ContractRevisionID
	CriterionID              domain.CriterionID
	SubjectType              domain.ProofSubjectType
	SubjectID                string
	SubjectRevision          string
	Kind                     domain.EvidenceKind
	SourceType               domain.EvidenceSourceType
	SourceRef                string
	ProducerType             domain.EvidenceProducerType
	ProducerRef              string
	Summary                  string
	ContentDigest            string
	RequestKey               string
}

// RecordVerificationInput contains one verification result.
type RecordVerificationInput struct {
	ExpectedContractRevision int64
	ContractRevisionID       domain.ContractRevisionID
	CriterionID              domain.CriterionID
	SubjectType              domain.ProofSubjectType
	SubjectID                string
	SubjectRevision          string
	EvidenceItemIDs          []domain.EvidenceItemID
	Method                   string
	IndependenceClass        domain.VerificationIndependenceClass
	Result                   domain.VerificationResult
	ProducerRef              string
	VerifierRef              string
	ProducerProvider         string
	VerifierProvider         string
	Detail                   string
	RequestKey               string
}

// DecideAcceptanceInput contains the owner's acceptance decision.
type DecideAcceptanceInput struct {
	ExpectedContractRevision int64
	ContractRevisionID       domain.ContractRevisionID
	Kind                     domain.AcceptanceDecisionKind
	Summary                  string
	ResourceDisposition      domain.ResourceDisposition
	ReentryTargetType        domain.ReentryTargetType
	ReentryTargetID          string
	RequestKey               string
}

var _ ProofManager = (*Service)(nil)

// GetProof returns the current proof projection for an Outcome.
func (s *Service) GetProof(ctx context.Context, outcomeID domain.OutcomeID) (ProofView, error) {
	if s.proof == nil {
		return ProofView{}, apierr.Internal("OUTCOME_PROOF_UNAVAILABLE", "Outcome proof storage is unavailable")
	}
	outcomeView, err := s.Get(ctx, outcomeID)
	if err != nil {
		return ProofView{}, err
	}
	evidence, err := s.proof.ListEvidenceItems(ctx, outcomeID)
	if err != nil {
		return ProofView{}, err
	}
	verifications, err := s.proof.ListVerificationRuns(ctx, outcomeID)
	if err != nil {
		return ProofView{}, err
	}
	decisions, err := s.proof.ListAcceptanceDecisions(ctx, outcomeID)
	if err != nil {
		return ProofView{}, err
	}
	corrections, err := s.proof.ListOutcomeCorrections(ctx, outcomeID)
	if err != nil {
		return ProofView{}, err
	}
	delegated, err := s.delegatedCriteria(ctx, outcomeView)
	if err != nil {
		return ProofView{}, err
	}
	attempts, err := s.store.ListAttempts(ctx, outcomeID)
	if err != nil {
		return ProofView{}, err
	}
	staleness, err := s.lineageStaleness(ctx, outcomeID, attempts)
	if err != nil {
		return ProofView{}, err
	}
	view := deriveProof(outcomeView, evidence, verifications, decisions, corrections, delegated, staleness)
	if err := s.attachResultFacts(ctx, outcomeView, &view); err != nil {
		return ProofView{}, err
	}
	return view, nil
}

// maxResultChangeFiles bounds one Attempt's projected change list. A larger
// receipt stays fully retained; only the review projection is shortened, and
// says so through Truncated.
const maxResultChangeFiles = 200

// attachResultFacts adds the measured artifact-change projection and the
// daemon-derived re-entry targets to the proof view. Both derive only from
// the durable Contract, Plan, Attempts and Kennel's own retained receipts; no
// provider claim becomes a fact here.
func (s *Service) attachResultFacts(ctx context.Context, outcomeView View, view *ProofView) error {
	view.Changes = []AttemptChangesView{}
	view.ReentryTargets = []ReentryTargetView{{
		TargetType: domain.ReentryTargetContract,
		TargetID:   string(outcomeView.Current.ID),
		Label:      fmt.Sprintf("Contract revision %d", outcomeView.Current.Number),
	}}
	unitTitles := map[domain.WorkUnitID]string{}
	if outcomeView.LatestPlan != nil && outcomeView.LatestPlan.ContractRevisionNumber == outcomeView.Current.Number {
		view.ReentryTargets = append(view.ReentryTargets, ReentryTargetView{
			TargetType: domain.ReentryTargetPlan,
			TargetID:   string(outcomeView.LatestPlan.ID),
			Label:      "Current plan",
		})
		for _, unit := range outcomeView.LatestPlan.WorkUnits {
			unitTitles[unit.ID] = unit.Title
			view.ReentryTargets = append(view.ReentryTargets, ReentryTargetView{
				TargetType: domain.ReentryTargetWorkUnit,
				TargetID:   string(unit.ID),
				Label:      unit.Title,
			})
		}
	}
	if s.store == nil || s.receipts == nil {
		return nil
	}
	attempts, err := s.store.ListAttempts(ctx, outcomeView.Outcome.ID)
	if err != nil {
		return err
	}
	for _, attempt := range attempts {
		if attempt.ContractRevisionNumber != outcomeView.Current.Number {
			continue
		}
		label := "Attempt (" + string(attempt.Status) + ")"
		if title := strings.TrimSpace(unitTitles[attempt.WorkUnitID]); title != "" {
			label = title + " (" + string(attempt.Status) + ")"
		}
		view.ReentryTargets = append(view.ReentryTargets, ReentryTargetView{
			TargetType: domain.ReentryTargetAttempt,
			TargetID:   string(attempt.ID),
			Label:      label,
		})
		receipt, retained, err := s.receipts.GetAttemptReceipt(ctx, attempt.ID)
		if err != nil {
			return err
		}
		if !retained {
			continue
		}
		files := append([]domain.ArtifactFile(nil), receipt.Files...)
		truncated := false
		if len(files) > maxResultChangeFiles {
			files = files[:maxResultChangeFiles]
			truncated = true
		}
		view.Changes = append(view.Changes, AttemptChangesView{
			AttemptID:       attempt.ID,
			WorkUnitID:      attempt.WorkUnitID,
			ArtifactVersion: receipt.ArtifactVersion,
			RetentionState:  receipt.RetentionState,
			Files:           files,
			Truncated:       truncated,
		})
	}
	return nil
}

// RecordEvidence appends evidence for an Outcome criterion.
func (s *Service) RecordEvidence(ctx context.Context, outcomeID domain.OutcomeID, in RecordEvidenceInput) (ProofView, error) {
	if s.proof == nil {
		return ProofView{}, apierr.Internal("OUTCOME_PROOF_UNAVAILABLE", "Outcome proof storage is unavailable")
	}
	fingerprint, err := requestFingerprint("evidence", outcomeID, in)
	if err != nil {
		return ProofView{}, err
	}
	if replay, ok, err := s.proof.FindEvidenceItemByRequestKey(ctx, strings.TrimSpace(in.RequestKey)); err != nil {
		return ProofView{}, err
	} else if ok {
		if replay.OutcomeID != outcomeID || replay.RequestFingerprint != fingerprint {
			return ProofView{}, replayConflict("EVIDENCE_REQUEST_CONFLICT", in.RequestKey)
		}
		return s.GetProof(ctx, outcomeID)
	}
	if err := s.validateProofTarget(ctx, outcomeID, in.ExpectedContractRevision, in.ContractRevisionID, in.CriterionID, in.SubjectType, in.SubjectID, in.SubjectRevision); err != nil {
		return ProofView{}, err
	}
	item := domain.EvidenceItem{
		ID:                 domain.EvidenceItemID("ev-" + uuid.NewString()),
		OutcomeID:          outcomeID,
		ContractRevisionID: in.ContractRevisionID,
		CriterionID:        in.CriterionID,
		SubjectType:        in.SubjectType,
		SubjectID:          strings.TrimSpace(in.SubjectID),
		SubjectRevision:    strings.TrimSpace(in.SubjectRevision),
		Kind:               in.Kind,
		SourceType:         in.SourceType,
		SourceRef:          strings.TrimSpace(in.SourceRef),
		ProducerType:       in.ProducerType,
		ProducerRef:        strings.TrimSpace(in.ProducerRef),
		Summary:            strings.TrimSpace(in.Summary),
		ContentDigest:      strings.ToLower(strings.TrimSpace(in.ContentDigest)),
		RequestKey:         strings.TrimSpace(in.RequestKey),
		RequestFingerprint: fingerprint,
		CreatedAt:          s.clock(),
	}
	if err := item.Validate(); err != nil {
		return ProofView{}, apierr.Invalid("EVIDENCE_INVALID", err.Error(), nil)
	}
	if err := s.proof.CreateEvidenceItem(ctx, item); err != nil {
		if replay, ok, findErr := s.proof.FindEvidenceItemByRequestKey(ctx, item.RequestKey); findErr == nil && ok {
			if replay.OutcomeID == outcomeID && replay.RequestFingerprint == fingerprint {
				return s.GetProof(ctx, outcomeID)
			}
			return ProofView{}, replayConflict("EVIDENCE_REQUEST_CONFLICT", item.RequestKey)
		}
		return ProofView{}, err
	}
	return s.GetProof(ctx, outcomeID)
}

// RecordVerification appends a verification run for an Outcome.
func (s *Service) RecordVerification(ctx context.Context, outcomeID domain.OutcomeID, in RecordVerificationInput) (ProofView, error) {
	if s.proof == nil {
		return ProofView{}, apierr.Internal("OUTCOME_PROOF_UNAVAILABLE", "Outcome proof storage is unavailable")
	}
	fingerprint, err := requestFingerprint("verification", outcomeID, in)
	if err != nil {
		return ProofView{}, err
	}
	if replay, ok, err := s.proof.FindVerificationRunByRequestKey(ctx, strings.TrimSpace(in.RequestKey)); err != nil {
		return ProofView{}, err
	} else if ok {
		if replay.OutcomeID != outcomeID || replay.RequestFingerprint != fingerprint {
			return ProofView{}, replayConflict("VERIFICATION_REQUEST_CONFLICT", in.RequestKey)
		}
		return s.GetProof(ctx, outcomeID)
	}
	if err := s.validateProofTarget(ctx, outcomeID, in.ExpectedContractRevision, in.ContractRevisionID, in.CriterionID, in.SubjectType, in.SubjectID, in.SubjectRevision); err != nil {
		return ProofView{}, err
	}
	for _, id := range in.EvidenceItemIDs {
		item, ok, err := s.proof.GetEvidenceItem(ctx, outcomeID, id)
		if err != nil {
			return ProofView{}, err
		}
		if !ok {
			return ProofView{}, apierr.NotFound("EVIDENCE_NOT_FOUND", "One of the Evidence items does not exist")
		}
		if item.ContractRevisionID != in.ContractRevisionID || item.CriterionID != in.CriterionID || item.SubjectType != in.SubjectType || item.SubjectID != strings.TrimSpace(in.SubjectID) || item.SubjectRevision != strings.TrimSpace(in.SubjectRevision) {
			return ProofView{}, apierr.Invalid("EVIDENCE_BINDING_MISMATCH", "Verification Evidence must bind the exact same criterion and subject revision", map[string]any{"evidenceItemId": id})
		}
	}
	run := domain.VerificationRun{
		ID:                 domain.VerificationRunID("ver-" + uuid.NewString()),
		OutcomeID:          outcomeID,
		ContractRevisionID: in.ContractRevisionID,
		CriterionID:        in.CriterionID,
		SubjectType:        in.SubjectType,
		SubjectID:          strings.TrimSpace(in.SubjectID),
		SubjectRevision:    strings.TrimSpace(in.SubjectRevision),
		EvidenceItemIDs:    append([]domain.EvidenceItemID(nil), in.EvidenceItemIDs...),
		Method:             strings.TrimSpace(in.Method),
		IndependenceClass:  in.IndependenceClass,
		Result:             in.Result,
		ProducerRef:        strings.TrimSpace(in.ProducerRef),
		VerifierRef:        strings.TrimSpace(in.VerifierRef),
		ProducerProvider:   strings.TrimSpace(in.ProducerProvider),
		VerifierProvider:   strings.TrimSpace(in.VerifierProvider),
		Detail:             strings.TrimSpace(in.Detail),
		RequestKey:         strings.TrimSpace(in.RequestKey),
		RequestFingerprint: fingerprint,
		CreatedAt:          s.clock(),
	}
	if err := run.Validate(); err != nil {
		return ProofView{}, apierr.Invalid("VERIFICATION_INVALID", err.Error(), nil)
	}
	if err := s.proof.CreateVerificationRun(ctx, run); err != nil {
		if replay, ok, findErr := s.proof.FindVerificationRunByRequestKey(ctx, run.RequestKey); findErr == nil && ok {
			if replay.OutcomeID == outcomeID && replay.RequestFingerprint == fingerprint {
				return s.GetProof(ctx, outcomeID)
			}
			return ProofView{}, replayConflict("VERIFICATION_REQUEST_CONFLICT", run.RequestKey)
		}
		return ProofView{}, err
	}
	return s.GetProof(ctx, outcomeID)
}

// DecideAcceptance records the owner's acceptance decision.
func (s *Service) DecideAcceptance(ctx context.Context, outcomeID domain.OutcomeID, in DecideAcceptanceInput) (ProofView, error) {
	if s.proof == nil {
		return ProofView{}, apierr.Internal("OUTCOME_PROOF_UNAVAILABLE", "Outcome proof storage is unavailable")
	}
	fingerprint, err := requestFingerprint("acceptance", outcomeID, in)
	if err != nil {
		return ProofView{}, err
	}
	if replay, ok, err := s.proof.FindAcceptanceDecisionByRequestKey(ctx, strings.TrimSpace(in.RequestKey)); err != nil {
		return ProofView{}, err
	} else if ok {
		if replay.OutcomeID != outcomeID || replay.RequestFingerprint != fingerprint {
			return ProofView{}, replayConflict("ACCEPTANCE_REQUEST_CONFLICT", in.RequestKey)
		}
		// The halt is retried on replay, not skipped. A correction whose
		// decision committed but whose halt failed is exactly the state a retry
		// exists to finish, and the halt is idempotent on the decision.
		if err := s.haltRunFor(ctx, outcomeID, replay); err != nil {
			return ProofView{}, err
		}
		return s.GetProof(ctx, outcomeID)
	}
	current, err := s.requireCurrentContract(ctx, outcomeID, in.ExpectedContractRevision, in.ContractRevisionID)
	if err != nil {
		return ProofView{}, err
	}
	proof, err := s.GetProof(ctx, outcomeID)
	if err != nil {
		return ProofView{}, err
	}
	switch in.Kind {
	case domain.AcceptanceAccept:
		if proof.Status != ProofStatusReadyForAcceptance {
			return ProofView{}, apierr.Conflict("OUTCOME_NOT_READY_FOR_ACCEPTANCE", "Every current criterion needs supporting Evidence and passing Verification before acceptance", map[string]any{"status": proof.Status})
		}
		if in.ReentryTargetType != "" || strings.TrimSpace(in.ReentryTargetID) != "" {
			return ProofView{}, apierr.Invalid("ACCEPTANCE_REENTRY_INVALID", "Acceptance does not create re-entry lineage", nil)
		}
	case domain.AcceptanceRequestRework:
		if proof.Status == ProofStatusAccepted {
			return ProofView{}, apierr.Conflict("OUTCOME_REOPEN_REQUIRED", "Reopen an accepted Outcome before requesting more work", nil)
		}
		if err := s.validateReentryTarget(ctx, outcomeID, current, in.ReentryTargetType, in.ReentryTargetID); err != nil {
			return ProofView{}, err
		}
	case domain.AcceptanceReopen:
		if proof.Status != ProofStatusAccepted {
			return ProofView{}, apierr.Conflict("OUTCOME_NOT_ACCEPTED", "Only an accepted Outcome can be reopened", map[string]any{"status": proof.Status})
		}
		if err := s.validateReentryTarget(ctx, outcomeID, current, in.ReentryTargetType, in.ReentryTargetID); err != nil {
			return ProofView{}, err
		}
	default:
		return ProofView{}, apierr.Invalid("ACCEPTANCE_INVALID", "Choose accept, request_rework, or reopen", nil)
	}
	decision := domain.AcceptanceDecision{
		ID:                  domain.AcceptanceDecisionID("acc-" + uuid.NewString()),
		OutcomeID:           outcomeID,
		ContractRevisionID:  current.ID,
		Kind:                in.Kind,
		ActorType:           domain.AcceptanceActorUser,
		Summary:             strings.TrimSpace(in.Summary),
		ResourceDisposition: in.ResourceDisposition,
		RequestKey:          strings.TrimSpace(in.RequestKey),
		RequestFingerprint:  fingerprint,
		CreatedAt:           s.clock(),
	}
	if err := decision.Validate(); err != nil {
		return ProofView{}, apierr.Invalid("ACCEPTANCE_INVALID", err.Error(), nil)
	}
	var correction *domain.OutcomeCorrection
	if in.Kind == domain.AcceptanceRequestRework || in.Kind == domain.AcceptanceReopen {
		correction = &domain.OutcomeCorrection{
			ID:                 domain.OutcomeCorrectionID("corr-" + uuid.NewString()),
			DecisionID:         decision.ID,
			OutcomeID:          outcomeID,
			ContractRevisionID: current.ID,
			Feedback:           decision.Summary,
			TargetType:         in.ReentryTargetType,
			TargetID:           strings.TrimSpace(in.ReentryTargetID),
			CreatedAt:          decision.CreatedAt,
		}
	}
	if err := s.proof.CreateAcceptanceDecision(ctx, decision, correction); err != nil {
		if replay, ok, findErr := s.proof.FindAcceptanceDecisionByRequestKey(ctx, decision.RequestKey); findErr == nil && ok {
			if replay.OutcomeID == outcomeID && replay.RequestFingerprint == fingerprint {
				if haltErr := s.haltRunFor(ctx, outcomeID, replay); haltErr != nil {
					return ProofView{}, haltErr
				}
				return s.GetProof(ctx, outcomeID)
			}
			return ProofView{}, replayConflict("ACCEPTANCE_REQUEST_CONFLICT", decision.RequestKey)
		}
		return ProofView{}, err
	}
	// The decision is durable before the run is stopped, so the owner's record
	// can never be lost to a failed halt. A failed halt surfaces as an error the
	// same request key retries, and the retry finishes it.
	if err := s.haltRunFor(ctx, outcomeID, decision); err != nil {
		return ProofView{}, err
	}
	return s.GetProof(ctx, outcomeID)
}

// haltRunFor ends the run authorization a rework or reopen decision
// invalidates. Acceptance is deliberately not included: accepting does not
// reject a result, and an Outcome with nothing left to run has no authorization
// worth cancelling.
func (s *Service) haltRunFor(ctx context.Context, outcomeID domain.OutcomeID, decision domain.AcceptanceDecision) error {
	if decision.Kind != domain.AcceptanceRequestRework && decision.Kind != domain.AcceptanceReopen {
		return nil
	}
	return s.HaltRunForCorrection(ctx, outcomeID, decision.ID)
}

func (s *Service) requireCurrentContract(ctx context.Context, outcomeID domain.OutcomeID, expected int64, revisionID domain.ContractRevisionID) (domain.ContractRevision, error) {
	view, err := s.Get(ctx, outcomeID)
	if err != nil {
		return domain.ContractRevision{}, err
	}
	if expected < 1 || expected != view.Current.Number || revisionID != view.Current.ID {
		return domain.ContractRevision{}, apierr.Conflict("OUTCOME_PROOF_CONTRACT_CONFLICT", "Proof must bind the Outcome's current immutable contract revision", map[string]any{
			"expectedRevision": expected, "currentRevision": view.Current.Number, "contractRevisionId": revisionID, "currentContractRevisionId": view.Current.ID,
		})
	}
	return view.Current, nil
}

func (s *Service) validateProofTarget(ctx context.Context, outcomeID domain.OutcomeID, expected int64, revisionID domain.ContractRevisionID, criterionID domain.CriterionID, subjectType domain.ProofSubjectType, subjectID, subjectRevision string) error {
	current, err := s.requireCurrentContract(ctx, outcomeID, expected, revisionID)
	if err != nil {
		return err
	}
	criterionFound := false
	for _, candidate := range current.Criteria {
		if candidate.ID == criterionID {
			criterionFound = true
			break
		}
	}
	if !criterionFound {
		return apierr.Invalid("CRITERION_BINDING_INVALID", "Evidence and Verification must name a criterion in the current Contract revision", nil)
	}
	subjectID = strings.TrimSpace(subjectID)
	subjectRevision = strings.TrimSpace(subjectRevision)
	switch subjectType {
	case domain.ProofSubjectOutcome:
		if subjectID != string(outcomeID) || subjectRevision != string(current.ID) {
			return subjectMismatch()
		}
	case domain.ProofSubjectContract:
		if subjectID != string(current.ID) || subjectRevision != string(current.ID) {
			return subjectMismatch()
		}
	case domain.ProofSubjectPlan:
		plan, ok, err := s.store.GetPlanRevision(ctx, outcomeID, domain.PlanRevisionID(subjectID))
		if err != nil {
			return err
		}
		if !ok || plan.ContractRevisionNumber != current.Number || subjectRevision != string(plan.ID) || !planCoversCriterion(plan, criterionID) {
			return subjectMismatch()
		}
	case domain.ProofSubjectWorkUnit:
		plan, ok, err := s.store.GetPlanRevision(ctx, outcomeID, domain.PlanRevisionID(subjectRevision))
		if err != nil {
			return err
		}
		if !ok || plan.ContractRevisionNumber != current.Number || !planWorkUnitCoversCriterion(plan, domain.WorkUnitID(subjectID), criterionID) {
			return subjectMismatch()
		}
	case domain.ProofSubjectAttempt:
		attempt, ok, err := s.store.GetAttempt(ctx, outcomeID, domain.AttemptID(subjectID))
		if err != nil {
			return err
		}
		if !ok || attempt.ContractRevisionNumber != current.Number {
			return subjectMismatch()
		}
		// An Attempt's revision is the artifact version it retained, the same
		// way a Plan subject's revision is the plan id. Proof about an Attempt
		// is proof about the bytes it produced, and bytes that were never
		// retained cannot have been checked -- so recording proof before
		// retention is refused rather than accepted and bound to nothing.
		if s.receipts == nil {
			return apierr.Internal("ATTEMPT_RECEIPTS_UNAVAILABLE", "Attempt receipt storage is unavailable")
		}
		receipt, retained, err := s.receipts.GetAttemptReceipt(ctx, attempt.ID)
		if err != nil {
			return err
		}
		if !retained || !receipt.RetentionState.Complete() {
			return apierr.Invalid("ATTEMPT_ARTIFACT_NOT_RETAINED", "This Attempt has no complete retained result to verify yet", map[string]any{"attemptId": string(attempt.ID)})
		}
		if subjectRevision != receipt.ArtifactVersion {
			return subjectMismatch()
		}
		plan, ok, err := s.store.GetPlanRevision(ctx, outcomeID, attempt.PlanRevisionID)
		if err != nil {
			return err
		}
		if !ok || plan.ContractRevisionNumber != current.Number || !planWorkUnitCoversCriterion(plan, attempt.WorkUnitID, criterionID) {
			return subjectMismatch()
		}
	default:
		return subjectMismatch()
	}
	return nil
}

func planCoversCriterion(plan domain.PlanRevision, criterionID domain.CriterionID) bool {
	for _, unit := range plan.WorkUnits {
		for _, id := range unit.CriterionIDs {
			if id == criterionID {
				return true
			}
		}
	}
	return false
}

func planWorkUnitCoversCriterion(plan domain.PlanRevision, workUnitID domain.WorkUnitID, criterionID domain.CriterionID) bool {
	for _, unit := range plan.WorkUnits {
		if unit.ID != workUnitID {
			continue
		}
		for _, id := range unit.CriterionIDs {
			if id == criterionID {
				return true
			}
		}
		return false
	}
	return false
}

func deriveProof(view View, allEvidence []domain.EvidenceItem, allVerifications []domain.VerificationRun, allDecisions []domain.AcceptanceDecision, corrections []domain.OutcomeCorrection, delegated map[domain.CriterionID]domain.DelegatedCriterion, staleness domain.LineageStaleness) ProofView {
	currentDecisions := make([]domain.AcceptanceDecision, 0)
	var horizon time.Time
	var horizonDecision domain.AcceptanceDecisionID
	for _, decision := range allDecisions {
		if decision.ContractRevisionID != view.Current.ID {
			continue
		}
		currentDecisions = append(currentDecisions, decision)
		if (decision.Kind == domain.AcceptanceRequestRework || decision.Kind == domain.AcceptanceReopen) && decision.CreatedAt.After(horizon) {
			horizon, horizonDecision = decision.CreatedAt, decision.ID
		}
	}
	currentCorrections := make([]domain.OutcomeCorrection, 0)
	var active *domain.OutcomeCorrection
	for _, correction := range corrections {
		if correction.ContractRevisionID != view.Current.ID {
			continue
		}
		currentCorrections = append(currentCorrections, correction)
		if correction.DecisionID == horizonDecision {
			held := correction
			active = &held
		}
	}

	proof := ProofView{
		OutcomeID: view.Outcome.ID, Contract: view.Current, Status: ProofStatusActive,
		Decisions: currentDecisions, Corrections: currentCorrections,
		ActiveCorrection: active, ProofHorizon: horizon,
		LineageStaleness: staleness,
	}
	allReady := len(view.Current.Criteria) > 0
	for _, criterion := range view.Current.Criteria {
		criterionView := CriterionProofView{Criterion: criterion, Gap: "Add supporting Evidence for this criterion."}
		for _, item := range allEvidence {
			if item.ContractRevisionID == view.Current.ID && item.CriterionID == criterion.ID && !staleProofSubject(staleness, item.SubjectType, item.SubjectID) {
				criterionView.Evidence = append(criterionView.Evidence, item)
			}
		}
		for _, run := range allVerifications {
			if run.ContractRevisionID == view.Current.ID && run.CriterionID == criterion.ID && !staleProofSubject(staleness, run.SubjectType, run.SubjectID) {
				criterionView.Verifications = append(criterionView.Verifications, run)
			}
		}
		if entry, isDelegated := delegated[criterion.ID]; isDelegated {
			criterionView.Delegated = true
			criterionView.ClaimedBy = entry.ClaimedBy
			criterionView.Ready, criterionView.Gap = entry.Proved, entry.Gap
		} else {
			criterionView.Ready, criterionView.Gap = criterionReady(criterionView, horizon)
		}
		allReady = allReady && criterionView.Ready
		proof.Criteria = append(proof.Criteria, criterionView)
	}
	latestDecision := domain.AcceptanceDecision{}
	if len(currentDecisions) > 0 {
		latestDecision = currentDecisions[len(currentDecisions)-1]
	}
	switch {
	case latestDecision.Kind == domain.AcceptanceAccept:
		proof.Status = ProofStatusAccepted
		proof.NextAction = "Accepted. Reopen explicitly if the result needs more work."
	case allReady:
		proof.Status = ProofStatusReadyForAcceptance
		proof.NextAction = "Review the current proof and explicitly accept or request rework."
	case !horizon.IsZero():
		proof.Status = ProofStatusReworkRequired
		proof.NextAction = "Follow the recorded correction, then add fresh Evidence and Verification."
	default:
		proof.NextAction = "Complete current criterion Evidence and Verification."
	}
	return proof
}

func criterionReady(view CriterionProofView, horizon time.Time) (bool, string) {
	var latestEvidence domain.EvidenceItem
	for _, item := range view.Evidence {
		if !horizon.IsZero() && !item.CreatedAt.After(horizon) {
			continue
		}
		if latestEvidence.ID == "" || item.CreatedAt.After(latestEvidence.CreatedAt) {
			latestEvidence = item
		}
	}
	if latestEvidence.ID == "" || latestEvidence.Kind != domain.EvidenceSupporting {
		if latestEvidence.Kind == domain.EvidenceContradicting {
			return false, "Resolve the latest contradicting Evidence."
		}
		return false, "Add supporting Evidence for this criterion."
	}
	var latestVerification domain.VerificationRun
	for _, run := range view.Verifications {
		if (!horizon.IsZero() && !run.CreatedAt.After(horizon)) || run.CreatedAt.Before(latestEvidence.CreatedAt) || !containsEvidence(run.EvidenceItemIDs, latestEvidence.ID) {
			continue
		}
		if latestVerification.ID == "" || run.CreatedAt.After(latestVerification.CreatedAt) {
			latestVerification = run
		}
	}
	if latestVerification.ID == "" {
		return false, "Verify the latest supporting Evidence."
	}
	if latestVerification.Result != domain.VerificationPassed && latestVerification.Result != domain.VerificationException {
		return false, "Resolve the latest failed or inconclusive Verification."
	}
	return true, ""
}

func containsEvidence(ids []domain.EvidenceItemID, want domain.EvidenceItemID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func requestFingerprint(kind string, outcomeID domain.OutcomeID, input any) (string, error) {
	payload, err := json.Marshal(struct {
		Kind      string
		OutcomeID domain.OutcomeID
		Input     any
	}{kind, outcomeID, input})
	if err != nil {
		return "", apierr.Internal("PROOF_REQUEST_FINGERPRINT_FAILED", "Could not fingerprint the proof request")
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func replayConflict(code, key string) error {
	return apierr.Conflict(code, "That request key was already used for different proof content", map[string]any{"requestKey": strings.TrimSpace(key)})
}

func subjectMismatch() error {
	return apierr.Invalid("PROOF_SUBJECT_MISMATCH", "The proof subject must name an exact current Outcome lineage revision", nil)
}

func (s *Service) validateReentryTarget(ctx context.Context, outcomeID domain.OutcomeID, current domain.ContractRevision, targetType domain.ReentryTargetType, targetID string) error {
	targetID = strings.TrimSpace(targetID)
	correction := domain.OutcomeCorrection{
		ID: "validation", DecisionID: "validation", OutcomeID: "validation", ContractRevisionID: "validation",
		Feedback: "validation", TargetType: targetType, TargetID: targetID, CreatedAt: time.Now(),
	}
	if err := correction.Validate(); err != nil {
		return apierr.Invalid("REENTRY_TARGET_REQUIRED", err.Error(), nil)
	}
	valid := false
	switch targetType {
	case domain.ReentryTargetContract:
		valid = targetID == string(current.ID)
	case domain.ReentryTargetPlan:
		plan, ok, err := s.store.GetPlanRevision(ctx, outcomeID, domain.PlanRevisionID(targetID))
		if err != nil {
			return err
		}
		valid = ok && plan.ContractRevisionNumber == current.Number
	case domain.ReentryTargetWorkUnit:
		plan, ok, err := s.store.GetLatestPlanRevision(ctx, outcomeID)
		if err != nil {
			return err
		}
		if ok && plan.ContractRevisionNumber == current.Number {
			for _, unit := range plan.WorkUnits {
				if targetID == string(unit.ID) {
					valid = true
					break
				}
			}
		}
	case domain.ReentryTargetAttempt:
		attempt, ok, err := s.store.GetAttempt(ctx, outcomeID, domain.AttemptID(targetID))
		if err != nil {
			return err
		}
		valid = ok && attempt.ContractRevisionNumber == current.Number
	}
	if !valid {
		return apierr.Invalid("REENTRY_TARGET_MISMATCH", "The re-entry target must belong to the Outcome's current contract lineage", map[string]any{"targetType": targetType, "targetId": targetID})
	}
	return nil
}

func (s *Service) delegatedCriteria(ctx context.Context, view View) (map[domain.CriterionID]domain.DelegatedCriterion, error) {
	children, err := s.store.ListContributingOutcomes(ctx, view.Outcome.ID)
	if err != nil {
		return nil, err
	}
	if len(children) == 0 {
		return nil, nil
	}
	links, err := s.store.ListContributionLinksForParent(ctx, view.Outcome.ID)
	if err != nil {
		return nil, err
	}
	accepted := make(map[domain.OutcomeID]bool, len(children))
	titles := make(map[domain.OutcomeID]string, len(children))
	for _, child := range children {
		titles[child.ID] = child.Title
		decisions, err := s.proof.ListAcceptanceDecisions(ctx, child.ID)
		if err != nil {
			return nil, err
		}
		accepted[child.ID] = domain.LatestDecisionAccepts(decisions)
	}
	return domain.DelegatedCriteria(view.Current, links, accepted, titles), nil
}
