package outcome

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	intelligencesvc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/intelligence"
)

func criterionAliases(revision domain.ContractRevision) (map[string]domain.CriterionID, error) {
	if len(revision.Criteria) == 0 {
		return nil, fmt.Errorf("contract revision %s has no canonical criterion identities", revision.ID)
	}
	criteria := append([]domain.ContractCriterion(nil), revision.Criteria...)
	sort.Slice(criteria, func(i, j int) bool { return criteria[i].Position < criteria[j].Position })
	aliases := make(map[string]domain.CriterionID, len(criteria))
	for i, criterion := range criteria {
		if criterion.ID.IsZero() {
			return nil, fmt.Errorf("contract revision %s contains a blank criterion id", revision.ID)
		}
		aliases[fmt.Sprintf("C%d", i+1)] = criterion.ID
	}
	return aliases, nil
}

// draftPlanWithProvenance obtains the one-shot readiness envelope from the
// configured planning provider and returns it with the evaluation fence the
// envelope's issues were keyed under. One-shot planning is sessionless per
// the packet's compatibility clause: the fence carries no session identity
// (S3.3's receipts introduce the bounded internal session for durable
// lineage). The fence's context digest binds the exact repository packet the
// provider saw, so it is filled after the context is built.
func (s *Service) draftPlanWithProvenance(
	ctx context.Context,
	projectID domain.ProjectID,
	outcome domain.Outcome,
	revision domain.ContractRevision,
	aliases map[string]domain.CriterionID,
	replanFeedback string,
) (domain.PlanningReadinessResult, domain.PlanningReadinessFence, error) {
	if s.planIntelligence == nil || s.intelligenceRuns == nil {
		return domain.PlanningReadinessResult{}, domain.PlanningReadinessFence{}, fmt.Errorf("plan intelligence is not wired")
	}
	request := ports.PlanIntelligenceRequest{Outcome: outcome, Contract: revision, CriterionAliases: aliases, ReplanFeedback: replanFeedback}
	_, project, projectErr := s.projectForOutcome(ctx, outcome.ID)
	if projectErr != nil {
		return domain.PlanningReadinessResult{}, domain.PlanningReadinessFence{}, projectErr
	}
	var briefSource interface {
		GetCurrentProjectBriefRevision(context.Context, domain.ProjectID) (domain.ProjectBriefRevision, bool, error)
	}
	if candidate, ok := s.store.(interface {
		GetCurrentProjectBriefRevision(context.Context, domain.ProjectID) (domain.ProjectBriefRevision, bool, error)
	}); ok {
		briefSource = candidate
	}
	limits, projectErr := s.repositoryContextLimits(ctx)
	if projectErr != nil {
		return domain.PlanningReadinessResult{}, domain.PlanningReadinessFence{}, projectErr
	}
	request.RepositoryContext, projectErr = intelligencesvc.BuildRepositoryContext(ctx, project, briefSource, limits)
	if projectErr != nil {
		return domain.PlanningReadinessResult{}, domain.PlanningReadinessFence{}, projectErr
	}
	// A supplied-document Outcome is grounded in the snapshot the owner
	// selected, not in whatever those files contain now and not in the
	// Project directory. The context digest travels into the run's input
	// digest below, so a proposal is bound to the exact material it saw.
	if err := s.groundInSelectedDocuments(ctx, outcome.ID, &request.RepositoryContext); err != nil {
		return domain.PlanningReadinessResult{}, domain.PlanningReadinessFence{}, err
	}
	// The fence binds the exact context packet the provider sees. A
	// supplied-document packet carries no repository digest, so bind its own
	// canonical encoding instead (mirrors StartPlanning's supplied-packet
	// handling).
	contextDigest := request.RepositoryContext.Digest
	if contextDigest == "" {
		digestInput, digestErr := json.Marshal(request.RepositoryContext)
		if digestErr != nil {
			return domain.PlanningReadinessResult{}, domain.PlanningReadinessFence{}, fmt.Errorf("encode supplied planning context: %w", digestErr)
		}
		contextDigest = domain.DigestSHA256(digestInput)
	}
	fence := domain.PlanningReadinessFence{ContractRevisionID: revision.ID, ContextDigest: contextDigest}
	request.Fence = fence
	encoded, err := json.Marshal(request)
	if err != nil {
		return domain.PlanningReadinessResult{}, domain.PlanningReadinessFence{}, fmt.Errorf("encode plan intelligence input: %w", err)
	}
	now := s.clock().UTC()
	run := domain.IntelligenceRun{
		ID:                 domain.IntelligenceRunID("intel-" + uuid.NewString()),
		Kind:               domain.IntelligenceRunPlanDraft,
		ProjectID:          projectID,
		OutcomeID:          outcome.ID,
		ContractRevisionID: revision.ID,
		SourceRevision:     revision.Number,
		RequestedProvider:  s.planIntelligence.ID(),
		InputDigest:        domain.DigestSHA256(encoded),
		Status:             domain.IntelligenceRunRequested,
		CreatedAt:          now,
	}
	if err := s.intelligenceRuns.CreateIntelligenceRun(ctx, run); err != nil {
		return domain.PlanningReadinessResult{}, domain.PlanningReadinessFence{}, err
	}
	if err := s.intelligenceRuns.UpdateIntelligenceRunStatus(ctx, run.ID, domain.IntelligenceRunRunning, "", "", "", nil); err != nil {
		return domain.PlanningReadinessResult{}, domain.PlanningReadinessFence{}, err
	}

	started := time.Now()
	response, err := s.planIntelligence.DraftPlan(ctx, request)
	if err != nil {
		completed := s.clock().UTC()
		duration := time.Since(started).Milliseconds()
		// Terminalizing has to outlive the caller's context; see
		// intelligence.TerminalizationContext for why.
		cleanup, cancelCleanup := intelligencesvc.TerminalizationContext(ctx)
		defer cancelCleanup()
		if metricErr := s.intelligenceRuns.RecordIntelligenceRunMetrics(cleanup, run.ID, nil, nil, &duration); metricErr != nil {
			_ = s.intelligenceRuns.UpdateIntelligenceRunStatus(cleanup, run.ID, domain.IntelligenceRunFailed, "", "INTELLIGENCE_STATUS_PERSIST_FAILED", "Reasoning status could not be persisted safely", &completed)
			return domain.PlanningReadinessResult{}, domain.PlanningReadinessFence{}, fmt.Errorf("plan intelligence failed and recovery state could not be recorded: %w", metricErr)
		}
		// The classified reason, so a missing credential, a throttle and a
		// refusal stay distinguishable in durable provenance.
		failureCode, failureDetail := intelligencesvc.TerminalReason(err)
		_ = s.intelligenceRuns.UpdateIntelligenceRunStatus(cleanup, run.ID, domain.IntelligenceRunFailed, "", failureCode, failureDetail, &completed)
		return domain.PlanningReadinessResult{}, domain.PlanningReadinessFence{}, intelligencesvc.APIError(err)
	}
	duration := time.Since(started).Milliseconds()
	if err := s.intelligenceRuns.RecordIntelligenceRunMetrics(ctx, run.ID, response.Provenance.InputTokens, response.Provenance.OutputTokens, &duration); err != nil {
		completed := s.clock().UTC()
		_ = s.intelligenceRuns.UpdateIntelligenceRunStatus(ctx, run.ID, domain.IntelligenceRunFailed, "", "INTELLIGENCE_STATUS_PERSIST_FAILED", "Reasoning status could not be persisted safely", &completed)
		return domain.PlanningReadinessResult{}, domain.PlanningReadinessFence{}, fmt.Errorf("record plan intelligence metrics: %w", err)
	}
	if err := response.Readiness.Validate(); err != nil {
		completed := s.clock().UTC()
		_ = s.intelligenceRuns.UpdateIntelligenceRunStatus(ctx, run.ID, domain.IntelligenceRunFailed, "", "PLANNING_READINESS_PAYLOAD_INVALID", "Plan intelligence returned an invalid readiness envelope", &completed)
		return domain.PlanningReadinessResult{}, domain.PlanningReadinessFence{}, fmt.Errorf("plan intelligence readiness: %w", err)
	}
	if err := s.intelligenceRuns.RecordIntelligenceRunEffectiveProvenance(ctx, run.ID,
		response.Provenance.EffectiveProvider, response.Provenance.EffectiveModel, response.Provenance.NativeSessionRef); err != nil {
		return domain.PlanningReadinessResult{}, domain.PlanningReadinessFence{}, err
	}
	output, err := json.Marshal(response.Readiness)
	if err != nil {
		return domain.PlanningReadinessResult{}, domain.PlanningReadinessFence{}, fmt.Errorf("encode plan intelligence output: %w", err)
	}
	completed := s.clock().UTC()
	if err := s.intelligenceRuns.UpdateIntelligenceRunStatus(ctx, run.ID, domain.IntelligenceRunFulfilled, domain.DigestSHA256(output), "", "", &completed); err != nil {
		return domain.PlanningReadinessResult{}, domain.PlanningReadinessFence{}, err
	}
	return response.Readiness, fence, nil
}
