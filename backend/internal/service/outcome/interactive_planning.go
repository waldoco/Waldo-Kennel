package outcome

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apierr"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	intelligencesvc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/intelligence"
)

// StartPlanningInput selects one exact planner and approved context source.
type StartPlanningInput struct {
	ExpectedContractRevision int64
	CandidateID              string
	ContextMode              domain.PlanningContextMode
	RequestKey               string
}

// PlanningMessageInput appends one idempotent owner message.
type PlanningMessageInput struct {
	ExpectedSessionRevision int64
	Text                    string
	RequestKey              string
}

// PlanningFinalizeInput requests a Plan proposal from the selected planner.
type PlanningFinalizeInput struct {
	ExpectedSessionRevision int64
	RequestKey              string
}

// planningTurnCancellation is process-local liveness for one direct provider
// call. Durable session state remains canonical; this entry only lets an owner
// cancellation interrupt work that is still executing in this daemon.
type planningTurnCancellation struct {
	cancel context.CancelFunc
}

// PlanningView is the daemon-owned projection rendered by Mission Control.
type PlanningView struct {
	Outcome      domain.Outcome
	Session      domain.PlanningSession
	Turns        []domain.PlanningTurn
	ProposedPlan *domain.PlanRevision
	// Readiness carries the typed readiness packet for the latest evaluation
	// when planning stopped before a Plan existed (needs_context or blocked).
	// It is nil once a Plan is proposed; the durable receipt arrives in S3.3.
	Readiness *domain.PlanningReadinessResult
}

// PlanningCandidates lists exact bindings available for the confirmed Contract.
func (s *Service) PlanningCandidates(ctx context.Context, outcomeID domain.OutcomeID, expectedContractRevision int64) ([]ports.PlanningCandidate, error) {
	outcomeRecord, revision, err := s.planningLineage(ctx, outcomeID, expectedContractRevision)
	if err != nil {
		return nil, err
	}
	_ = outcomeRecord
	if s.planningDialogue == nil {
		return nil, apierr.Internal("PLANNING_UNWIRED", "Interactive planning is unavailable in this environment")
	}
	_ = revision
	candidates, err := s.planningDialogue.PlanningCandidates(ctx)
	if err != nil {
		return nil, intelligencesvc.APIError(err)
	}
	return candidates, nil
}

// StartPlanning opens one Contract-bound conversation with frozen context.
func (s *Service) StartPlanning(ctx context.Context, outcomeID domain.OutcomeID, in StartPlanningInput) (PlanningView, error) {
	if s.planningSessions == nil || s.planningDialogue == nil || s.intelligenceRuns == nil || s.routing == nil {
		return PlanningView{}, apierr.Internal("PLANNING_UNWIRED", "Interactive planning is unavailable in this environment")
	}
	if strings.TrimSpace(in.RequestKey) == "" {
		return PlanningView{}, apierr.Invalid("PLANNING_REQUEST_KEY_REQUIRED", "A request key is required", nil)
	}
	contextMode := in.ContextMode
	if contextMode == "" {
		contextMode = domain.PlanningContextRepositoryRead
	}
	if !contextMode.Valid() {
		return PlanningView{}, apierr.Invalid("PLANNING_CONTEXT_INVALID", "Choose repository context or an approved supplied-document packet", nil)
	}
	requestKey := strings.TrimSpace(in.RequestKey)
	fingerprint := planningStartFingerprint(outcomeID, in.ExpectedContractRevision, strings.TrimSpace(in.CandidateID), contextMode)
	if existing, found, err := s.planningSessions.GetPlanningSessionByRequestKey(ctx, requestKey); err != nil {
		return PlanningView{}, err
	} else if found {
		if existing.RequestFingerprint != fingerprint {
			return PlanningView{}, planningAPIError(&ports.PlanningRequestConflictError{RequestKey: requestKey})
		}
		outcomeRecord, found, err := s.store.GetOutcome(ctx, existing.OutcomeID)
		if err != nil {
			return PlanningView{}, err
		}
		if !found {
			return PlanningView{}, apierr.NotFound("OUTCOME_NOT_FOUND", "That Outcome does not exist")
		}
		existing, err = s.reconcilePlanningContract(ctx, outcomeRecord, existing)
		if err != nil {
			return PlanningView{}, err
		}
		return s.planningView(ctx, outcomeRecord, existing)
	}
	outcomeRecord, revision, err := s.planningLineage(ctx, outcomeID, in.ExpectedContractRevision)
	if err != nil {
		return PlanningView{}, err
	}
	if contextMode == domain.PlanningContextRepositoryRead && !planningRepositoryReadAllowed(revision) {
		return PlanningView{}, apierr.New(apierr.KindConflict, "PLANNING_REPOSITORY_READ_REQUIRED",
			"Confirm repository-reading authority before starting planning", nil)
	}
	if current, found, currentErr := s.planningSessions.GetCurrentPlanningSession(ctx, outcomeID); currentErr != nil {
		return PlanningView{}, currentErr
	} else if found && current.Status == domain.PlanningSessionActive {
		if current.ContractRevisionNumber != revision.Number {
			if _, closeErr := s.planningSessions.ClosePlanningSession(ctx, current.ID, current.Revision, domain.PlanningSessionSuperseded); closeErr != nil {
				return PlanningView{}, planningAPIError(closeErr)
			}
		} else if current.RequestKey != strings.TrimSpace(in.RequestKey) {
			return PlanningView{}, apierr.New(apierr.KindConflict, "PLANNING_ALREADY_ACTIVE", "Continue or cancel the current planning conversation first", map[string]any{"planningSessionId": current.ID.String()})
		}
	}
	candidates, err := s.planningDialogue.PlanningCandidates(ctx)
	if err != nil {
		return PlanningView{}, intelligencesvc.APIError(err)
	}
	var selected *ports.PlanningCandidate
	for index := range candidates {
		if candidates[index].ID == strings.TrimSpace(in.CandidateID) {
			selected = &candidates[index]
			break
		}
	}
	if selected == nil {
		return PlanningView{}, apierr.Invalid("PLANNING_CANDIDATE_UNKNOWN", "Choose an available planning agent", nil)
	}
	if !selected.Ready {
		return PlanningView{}, apierr.Unavailable(selected.UnavailableCode, selected.UnavailableDetail, map[string]any{"candidateId": selected.ID})
	}
	projectID, project, err := s.projectForOutcome(ctx, outcomeID)
	if err != nil {
		return PlanningView{}, err
	}
	var snapshot ports.RepositoryContextSnapshot
	if contextMode == domain.PlanningContextSuppliedPacket {
		snapshot.ProjectID = projectID
		if err := s.groundInSelectedDocuments(ctx, outcomeID, &snapshot); err != nil {
			return PlanningView{}, err
		}
		if snapshot.Root != "supplied-documents" {
			return PlanningView{}, apierr.New(apierr.KindConflict, "PLANNING_DOCUMENTS_NOT_APPROVED", "Approve the selected documents before planning from them", nil)
		}
		// The document adapter replaces repository facts, so bind this session
		// to the approved packet rather than retaining the old repository digest.
		snapshot.Digest = ""
		digestInput, digestErr := json.Marshal(snapshot)
		if digestErr != nil {
			return PlanningView{}, fmt.Errorf("encode supplied planning context: %w", digestErr)
		}
		snapshot.Digest = domain.DigestSHA256(digestInput)
	} else {
		var briefSource interface {
			GetCurrentProjectBriefRevision(context.Context, domain.ProjectID) (domain.ProjectBriefRevision, bool, error)
		}
		if candidate, ok := s.store.(interface {
			GetCurrentProjectBriefRevision(context.Context, domain.ProjectID) (domain.ProjectBriefRevision, bool, error)
		}); ok {
			briefSource = candidate
		}
		limits, limitErr := s.repositoryContextLimits(ctx)
		if limitErr != nil {
			return PlanningView{}, limitErr
		}
		snapshot, err = intelligencesvc.BuildRepositoryContext(ctx, project, briefSource, limits)
		if err != nil {
			return PlanningView{}, err
		}
		// A partial snapshot (e.g. one that hit a bounded discovery limit) is
		// not the same as an unusable one: BuildRepositoryContext deliberately
		// keeps whatever files/instructions it found before stopping, and
		// appendRepositoryContext already discloses UnavailableReason in the
		// prompt as an honest "context limitation" line. Only refuse planning
		// outright when there is truly nothing to reason from — an
		// uninspectable revision, or zero files and zero instructions.
		if snapshot.Revision == "" || (len(snapshot.Files) == 0 && len(snapshot.Instructions) == 0) {
			detail := snapshot.UnavailableReason
			if detail == "" {
				detail = "repository context is unavailable for planning"
			}
			return PlanningView{}, apierr.Unavailable("PLANNING_REPOSITORY_UNAVAILABLE", detail, nil)
		}
	}
	contextJSON, err := json.Marshal(snapshot)
	if err != nil {
		return PlanningView{}, fmt.Errorf("encode planning context: %w", err)
	}
	grantJSON, _ := json.Marshal(struct {
		RepositoryPacketRead bool `json:"repositoryPacketRead"`
		SuppliedPacketRead   bool `json:"suppliedPacketRead"`
		Commands             bool `json:"commands"`
		Writes               bool `json:"writes"`
		ExternalEffects      bool `json:"externalEffects"`
	}{RepositoryPacketRead: contextMode == domain.PlanningContextRepositoryRead, SuppliedPacketRead: contextMode == domain.PlanningContextSuppliedPacket})
	now := s.clock().UTC()
	session := domain.PlanningSession{
		ID: domain.PlanningSessionID("planning-" + uuid.NewString()), OutcomeID: outcomeID, ProjectID: projectID,
		ContractRevisionID: revision.ID, ContractRevisionNumber: revision.Number, Revision: 1,
		Status: domain.PlanningSessionActive, WaitingOn: domain.PlanningWaitingOwner, Binding: selected.Binding,
		ContextMode: contextMode, PlanningGrantDigest: domain.DigestSHA256(grantJSON), ContextDigest: snapshot.Digest,
		ContextSnapshotJSON: contextJSON, RequestKey: requestKey, RequestFingerprint: fingerprint,
		CreatedAt: now, UpdatedAt: now,
	}
	saved, _, err := s.planningSessions.CreatePlanningSession(ctx, session)
	if err != nil {
		return PlanningView{}, planningAPIError(err)
	}
	return s.planningView(ctx, outcomeRecord, saved)
}

// GetPlanning returns one planning conversation and its optional proposed Plan.
func (s *Service) GetPlanning(ctx context.Context, outcomeID domain.OutcomeID, sessionID domain.PlanningSessionID) (PlanningView, error) {
	if s.planningSessions == nil {
		return PlanningView{}, apierr.Internal("PLANNING_UNWIRED", "Interactive planning is unavailable in this environment")
	}
	outcomeRecord, found, err := s.store.GetOutcome(ctx, outcomeID)
	if err != nil {
		return PlanningView{}, err
	}
	if !found {
		return PlanningView{}, apierr.NotFound("OUTCOME_NOT_FOUND", "That Outcome does not exist")
	}
	session, found, err := s.planningSessions.GetPlanningSession(ctx, outcomeID, sessionID)
	if err != nil {
		return PlanningView{}, err
	}
	if !found {
		return PlanningView{}, apierr.NotFound("PLANNING_SESSION_NOT_FOUND", "That planning conversation does not exist")
	}
	session, err = s.reconcilePlanningContract(ctx, outcomeRecord, session)
	if err != nil {
		return PlanningView{}, err
	}
	return s.planningView(ctx, outcomeRecord, session)
}

// GetCurrentPlanning returns the newest planning conversation for an Outcome.
func (s *Service) GetCurrentPlanning(ctx context.Context, outcomeID domain.OutcomeID) (PlanningView, error) {
	if s.planningSessions == nil {
		return PlanningView{}, apierr.Internal("PLANNING_UNWIRED", "Interactive planning is unavailable in this environment")
	}
	outcomeRecord, found, err := s.store.GetOutcome(ctx, outcomeID)
	if err != nil {
		return PlanningView{}, err
	}
	if !found {
		return PlanningView{}, apierr.NotFound("OUTCOME_NOT_FOUND", "That Outcome does not exist")
	}
	session, found, err := s.planningSessions.GetCurrentPlanningSession(ctx, outcomeID)
	if err != nil {
		return PlanningView{}, err
	}
	if !found {
		return PlanningView{}, apierr.NotFound("PLANNING_SESSION_NOT_FOUND", "This Outcome has no planning conversation yet")
	}
	session, err = s.reconcilePlanningContract(ctx, outcomeRecord, session)
	if err != nil {
		return PlanningView{}, err
	}
	return s.planningView(ctx, outcomeRecord, session)
}

// ContinuePlanning exchanges one owner message for one structured planner turn.
func (s *Service) ContinuePlanning(ctx context.Context, outcomeID domain.OutcomeID, sessionID domain.PlanningSessionID, in PlanningMessageInput) (PlanningView, error) {
	if strings.TrimSpace(in.Text) == "" {
		return PlanningView{}, apierr.Invalid("PLANNING_MESSAGE_REQUIRED", "Write a short message for the planning agent", nil)
	}
	return s.runPlanningTurn(ctx, outcomeID, sessionID, in.ExpectedSessionRevision, strings.TrimSpace(in.Text), in.RequestKey, false)
}

// FinalizePlanning requests a proposal without bypassing validation or approval.
func (s *Service) FinalizePlanning(ctx context.Context, outcomeID domain.OutcomeID, sessionID domain.PlanningSessionID, in PlanningFinalizeInput) (PlanningView, error) {
	return s.runPlanningTurn(ctx, outcomeID, sessionID, in.ExpectedSessionRevision, "Propose the plan now.", in.RequestKey, true)
}

// CancelPlanning closes an active conversation without creating execution state.
func (s *Service) CancelPlanning(ctx context.Context, outcomeID domain.OutcomeID, sessionID domain.PlanningSessionID, expectedRevision int64) (PlanningView, error) {
	view, err := s.GetPlanning(ctx, outcomeID, sessionID)
	if err != nil {
		return PlanningView{}, err
	}
	if view.Session.Status == domain.PlanningSessionCancelled {
		s.cancelPlanningTurn(sessionID)
		return view, nil
	}
	if view.Session.Status != domain.PlanningSessionActive {
		return PlanningView{}, apierr.New(apierr.KindConflict, "PLANNING_SESSION_CLOSED", "This planning conversation is closed", nil)
	}
	closeRevision := expectedRevision
	// The owner starts a turn from revision N, while the durable owner-turn write
	// advances the session to N+1 before the provider call begins. Accept that
	// exact one-step fence while waiting on the provider so Cancel remains usable
	// before the original HTTP request can return its newer revision.
	if view.Session.WaitingOn == domain.PlanningWaitingProvider && expectedRevision+1 == view.Session.Revision {
		closeRevision = view.Session.Revision
	}
	closed, err := s.planningSessions.ClosePlanningSession(ctx, sessionID, closeRevision, domain.PlanningSessionCancelled)
	if err != nil {
		// Two concurrent or response-lost cancellation requests converge on the
		// same terminal fact instead of turning success into an ambiguous conflict.
		current, currentErr := s.GetPlanning(ctx, outcomeID, sessionID)
		if currentErr == nil && current.Session.Status == domain.PlanningSessionCancelled {
			s.cancelPlanningTurn(sessionID)
			return current, nil
		}
		return PlanningView{}, planningAPIError(err)
	}
	s.cancelPlanningTurn(sessionID)
	return s.planningView(ctx, view.Outcome, closed)
}

// RecoverInterruptedPlanning returns crash-interrupted direct-API waits to the
// owner. It never replays a possibly billed provider request automatically.
func (s *Service) RecoverInterruptedPlanning(ctx context.Context) (int64, error) {
	if s.planningSessions == nil {
		return 0, apierr.Internal("PLANNING_UNWIRED", "Interactive planning is unavailable in this environment")
	}
	recovered, err := s.planningSessions.RecoverInterruptedPlanningSessions(ctx, s.clock())
	if err != nil {
		return 0, apierr.Internal("PLANNING_RECOVERY_FAILED", "Interrupted planning conversations could not be recovered")
	}
	return recovered, nil
}

func (s *Service) runPlanningTurn(ctx context.Context, outcomeID domain.OutcomeID, sessionID domain.PlanningSessionID, expectedRevision int64, text, requestKey string, finalize bool) (PlanningView, error) {
	if s.planningSessions == nil || s.planningDialogue == nil || s.intelligenceRuns == nil || s.routing == nil {
		return PlanningView{}, apierr.Internal("PLANNING_UNWIRED", "Interactive planning is unavailable in this environment")
	}
	if strings.TrimSpace(requestKey) == "" {
		return PlanningView{}, apierr.Invalid("PLANNING_REQUEST_KEY_REQUIRED", "A request key is required", nil)
	}
	view, err := s.GetPlanning(ctx, outcomeID, sessionID)
	if err != nil {
		return PlanningView{}, err
	}
	if view.Outcome.CurrentRevisionNumber != view.Session.ContractRevisionNumber || view.Session.Status == domain.PlanningSessionSuperseded {
		return PlanningView{}, apierr.New(apierr.KindConflict, "PLANNING_CONTRACT_STALE", "The Contract changed. Start a new planning conversation.", map[string]any{"currentRevision": view.Outcome.CurrentRevisionNumber})
	}
	payload, _ := json.Marshal(struct {
		SessionID string `json:"sessionId"`
		Revision  int64  `json:"revision"`
		Text      string `json:"text"`
		Finalize  bool   `json:"finalize"`
	}{sessionID.String(), expectedRevision, text, finalize})
	now := s.clock().UTC()
	ownerTurn := domain.PlanningTurn{
		ID: domain.PlanningTurnID("planning-turn-" + uuid.NewString()), PlanningSessionID: sessionID,
		Role: domain.PlanningTurnOwner, Kind: domain.PlanningTurnMessage, Text: text,
		RequestKey: strings.TrimSpace(requestKey), RequestFingerprint: domain.DigestSHA256(payload), CreatedAt: now,
	}
	if finalize {
		ownerTurn.Kind = domain.PlanningTurnFinalizeRequest
	}
	session, storedOwner, replay, err := s.planningSessions.AppendPlanningOwnerTurn(ctx, sessionID, expectedRevision, ownerTurn)
	if err != nil {
		if view.Session.Status != domain.PlanningSessionActive {
			return PlanningView{}, apierr.New(apierr.KindConflict, "PLANNING_SESSION_CLOSED", "This planning conversation is closed", nil)
		}
		return PlanningView{}, planningAPIError(err)
	}
	if replay {
		return s.resumePlanningReply(ctx, view.Outcome, session, storedOwner)
	}
	turnCtx, turn := s.beginPlanningTurn(ctx, sessionID)
	defer s.endPlanningTurn(sessionID, turn)
	// Cancel can commit in the small interval after the owner turn becomes
	// durable and before its process-local cancellation entry is registered.
	// Re-read after registration so that race still prevents provider work.
	currentSession, found, err := s.planningSessions.GetPlanningSession(ctx, outcomeID, sessionID)
	if err != nil {
		return PlanningView{}, err
	}
	if !found {
		return PlanningView{}, apierr.NotFound("PLANNING_SESSION_NOT_FOUND", "That planning conversation does not exist")
	}
	if currentSession.Status != domain.PlanningSessionActive || currentSession.WaitingOn != domain.PlanningWaitingProvider || currentSession.Revision != session.Revision {
		turn.cancel()
		return PlanningView{}, apierr.New(apierr.KindConflict, "PLANNING_SESSION_CLOSED", "This planning conversation is closed", nil)
	}
	var snapshot ports.RepositoryContextSnapshot
	if err := json.Unmarshal(session.ContextSnapshotJSON, &snapshot); err != nil {
		return PlanningView{}, apierr.Internal("PLANNING_CONTEXT_CORRUPT", "The frozen planning context could not be read")
	}
	revision, err := s.currentRevision(ctx, view.Outcome)
	if err != nil {
		return PlanningView{}, err
	}
	aliases, err := criterionAliases(revision)
	if err != nil {
		return PlanningView{}, err
	}
	turns, err := s.planningSessions.ListPlanningTurns(ctx, sessionID)
	if err != nil {
		return PlanningView{}, err
	}
	fence := domain.PlanningReadinessFence{
		PlanningSessionID: session.ID, SessionRevision: session.Revision,
		ContractRevisionID: session.ContractRevisionID, ContextDigest: session.ContextDigest,
	}
	request := ports.PlanningDiscussionRequest{
		Binding: session.Binding, Outcome: view.Outcome, Contract: revision, CriterionAliases: aliases, Fence: fence,
		// The frozen repository snapshot supplies planning context for every
		// provider. Native repository tools remain a separately conformed mode;
		// this launch path never silently upgrades packet access into tool access.
		RepositoryContext: snapshot, RepositoryToolUse: false,
		Turns: turns, Finalize: finalize,
	}
	encodedRequest, _ := json.Marshal(request)
	run := domain.IntelligenceRun{
		ID: domain.IntelligenceRunID("intel-" + uuid.NewString()), Kind: domain.IntelligenceRunPlanDraft,
		ProjectID: session.ProjectID, OutcomeID: outcomeID, ContractRevisionID: revision.ID, SourceRevision: revision.Number,
		RequestedProvider: session.Binding.Provider, RequestedModel: session.Binding.Model, InputDigest: domain.DigestSHA256(encodedRequest),
		Status: domain.IntelligenceRunRequested, CreatedAt: now,
	}
	if err := s.intelligenceRuns.CreateIntelligenceRun(ctx, run); err != nil {
		return PlanningView{}, err
	}
	if err := s.intelligenceRuns.UpdateIntelligenceRunStatus(ctx, run.ID, domain.IntelligenceRunRunning, "", "", "", nil); err != nil {
		return PlanningView{}, err
	}
	started := time.Now()
	response, err := s.planningDialogue.DiscussPlan(turnCtx, request)
	if err == nil && turnCtx.Err() != nil {
		err = ports.ClassifyReasoningTransport(turnCtx, 0, turnCtx.Err())
	}
	if err != nil {
		completed := s.clock().UTC()
		duration := time.Since(started).Milliseconds()
		cleanup, cancel := intelligencesvc.TerminalizationContext(ctx)
		defer cancel()
		_ = s.intelligenceRuns.RecordIntelligenceRunMetrics(cleanup, run.ID, nil, nil, &duration)
		code, detail := intelligencesvc.TerminalReason(err)
		_ = s.intelligenceRuns.UpdateIntelligenceRunStatus(cleanup, run.ID, domain.IntelligenceRunFailed, "", code, detail, &completed)
		_, _ = s.planningSessions.SetPlanningSessionFailure(cleanup, sessionID, session.Revision, code, detail)
		return PlanningView{}, intelligencesvc.APIError(err)
	}
	provenanceFailureCode, provenanceFailureDetail := "", ""
	if response.Provenance.EffectiveProvider.IsZero() || response.Provenance.EffectiveProvider != session.Binding.Provider {
		provenanceFailureCode = "PLANNING_PROVIDER_MISMATCH"
		provenanceFailureDetail = "The planning response did not come from the selected provider"
	} else if session.Binding.ModelSelection == domain.PlanningModelExplicit && strings.TrimSpace(response.Provenance.EffectiveModel) != session.Binding.Model {
		provenanceFailureCode = "PLANNING_MODEL_MISMATCH"
		provenanceFailureDetail = "The planning response did not use the selected explicit model"
	}
	if provenanceFailureCode != "" {
		completed := s.clock().UTC()
		_ = s.intelligenceRuns.UpdateIntelligenceRunStatus(ctx, run.ID, domain.IntelligenceRunFailed, "", provenanceFailureCode, provenanceFailureDetail, &completed)
		_, _ = s.planningSessions.SetPlanningSessionFailure(ctx, sessionID, session.Revision, provenanceFailureCode, provenanceFailureDetail)
		return PlanningView{}, apierr.New(apierr.KindConflict, provenanceFailureCode, "The planning provider or model changed. Start a new planning conversation.", nil)
	}
	encodedResult, err := json.Marshal(response.Result)
	if err != nil {
		return PlanningView{}, fmt.Errorf("encode planning reply: %w", err)
	}
	duration := time.Since(started).Milliseconds()
	if err := s.intelligenceRuns.RecordIntelligenceRunMetrics(ctx, run.ID, response.Provenance.InputTokens, response.Provenance.OutputTokens, &duration); err != nil {
		return PlanningView{}, err
	}
	if err := s.intelligenceRuns.RecordIntelligenceRunEffectiveProvenance(ctx, run.ID, response.Provenance.EffectiveProvider, response.Provenance.EffectiveModel, response.Provenance.NativeSessionRef); err != nil {
		return PlanningView{}, err
	}
	completed := s.clock().UTC()
	if err := s.intelligenceRuns.UpdateIntelligenceRunStatus(ctx, run.ID, domain.IntelligenceRunFulfilled, domain.DigestSHA256(encodedResult), "", "", &completed); err != nil {
		return PlanningView{}, err
	}
	plannerKind, err := planningTurnKind(response.Result)
	if err != nil {
		return PlanningView{}, err
	}
	plannerTurn := domain.PlanningTurn{
		ID: domain.PlanningTurnID("planning-turn-" + uuid.NewString()), PlanningSessionID: sessionID,
		ReplyToTurnID: storedOwner.ID, Role: domain.PlanningTurnPlanner, Kind: plannerKind,
		Text: strings.TrimSpace(response.Result.Message), StructuredPayload: encodedResult, IntelligenceRunID: run.ID, CreatedAt: completed,
	}
	session, err = s.planningSessions.AppendPlanningProviderTurn(ctx, sessionID, session.Revision, plannerTurn,
		response.Provenance.EffectiveProvider, response.Provenance.EffectiveModel, response.Provenance.NativeSessionRef)
	if err != nil {
		return PlanningView{}, planningAPIError(err)
	}
	currentOutcome, found, err := s.store.GetOutcome(ctx, outcomeID)
	if err != nil {
		return PlanningView{}, err
	}
	if !found {
		return PlanningView{}, apierr.NotFound("OUTCOME_NOT_FOUND", "That Outcome does not exist")
	}
	session, err = s.reconcilePlanningContract(ctx, currentOutcome, session)
	if err != nil {
		return PlanningView{}, err
	}
	if session.Status == domain.PlanningSessionSuperseded {
		return PlanningView{}, apierr.New(apierr.KindConflict, "PLANNING_CONTRACT_STALE", "The Contract changed. Start a new planning conversation.", map[string]any{"currentRevision": currentOutcome.CurrentRevisionNumber})
	}
	return s.finishPlanningEvaluation(ctx, currentOutcome, revision, session, run.ID, fence, response.Result)
}

func (s *Service) beginPlanningTurn(parent context.Context, sessionID domain.PlanningSessionID) (context.Context, *planningTurnCancellation) {
	ctx, cancel := context.WithCancel(parent)
	entry := &planningTurnCancellation{cancel: cancel}
	s.planningTurnMu.Lock()
	if s.planningTurns == nil {
		s.planningTurns = make(map[domain.PlanningSessionID]*planningTurnCancellation)
	}
	previous := s.planningTurns[sessionID]
	s.planningTurns[sessionID] = entry
	s.planningTurnMu.Unlock()
	if previous != nil {
		previous.cancel()
	}
	return ctx, entry
}

func (s *Service) endPlanningTurn(sessionID domain.PlanningSessionID, entry *planningTurnCancellation) {
	entry.cancel()
	s.planningTurnMu.Lock()
	if s.planningTurns[sessionID] == entry {
		delete(s.planningTurns, sessionID)
	}
	s.planningTurnMu.Unlock()
}

func (s *Service) cancelPlanningTurn(sessionID domain.PlanningSessionID) {
	s.planningTurnMu.Lock()
	entry := s.planningTurns[sessionID]
	s.planningTurnMu.Unlock()
	if entry != nil {
		entry.cancel()
	}
}

func (s *Service) resumePlanningReply(ctx context.Context, outcomeRecord domain.Outcome, session domain.PlanningSession, owner domain.PlanningTurn) (PlanningView, error) {
	turns, err := s.planningSessions.ListPlanningTurns(ctx, session.ID)
	if err != nil {
		return PlanningView{}, err
	}
	for _, turn := range turns {
		if turn.ReplyToTurnID != owner.ID {
			continue
		}
		if turn.Kind != domain.PlanningTurnPlanProposal || session.Status != domain.PlanningSessionActive {
			return s.planningView(ctx, outcomeRecord, session)
		}
		var result domain.PlanningReadinessResult
		if err := json.Unmarshal(turn.StructuredPayload, &result); err != nil {
			return PlanningView{}, apierr.Internal("PLANNING_REPLY_CORRUPT", "The saved planning reply could not be read")
		}
		revision, err := s.currentRevision(ctx, outcomeRecord)
		if err != nil {
			return PlanningView{}, err
		}
		// Replay re-runs the same evaluation the original turn ran; the fence
		// rebuilds from durable session state.
		fence := domain.PlanningReadinessFence{
			PlanningSessionID: session.ID, SessionRevision: session.Revision,
			ContractRevisionID: session.ContractRevisionID, ContextDigest: session.ContextDigest,
		}
		return s.finishPlanningEvaluation(ctx, outcomeRecord, revision, session, turn.IntelligenceRunID, fence, result)
	}
	return s.planningView(ctx, outcomeRecord, session)
}

// finishPlanningEvaluation is the S3 seam every interactive planning result
// passes: the strict envelope is evaluated against the confirmed Contract and
// one normalized inventory snapshot, and only a zero-issue ready packet enters
// the canonical compiler. needs_context keeps the session waiting on the
// owner; a blocked packet marks the session waiting on system setup, never on
// owner silence. No Plan is written for a non-ready packet.
func (s *Service) finishPlanningEvaluation(ctx context.Context, outcomeRecord domain.Outcome, revision domain.ContractRevision, session domain.PlanningSession, runID domain.IntelligenceRunID, fence domain.PlanningReadinessFence, envelope domain.PlanningReadinessResult) (PlanningView, error) {
	projectID, project, err := s.projectForOutcome(ctx, outcomeRecord.ID)
	if err != nil {
		return PlanningView{}, err
	}
	preference, hasPreference, err := domain.ResolveEffectiveExecutionPreference(revision.ExecutionPreference, project.Config)
	if err != nil {
		return PlanningView{}, apierr.Invalid("PLAN_PREFERENCE_INVALID", err.Error(), nil)
	}
	routingPreference := routingPreferenceFromExecution(preference, hasPreference)
	evaluated, snapshot, err := s.EvaluatePlanReadiness(ctx, fence, projectID, revision, routingPreference, envelope.Proposal, envelope.Issues, envelope.Message)
	if err != nil {
		return PlanningView{}, err
	}
	if evaluated.Status == domain.PlanningReady {
		return s.finishPlanningProposal(ctx, outcomeRecord, revision, session, runID, *evaluated.Proposal, snapshot, routingPreference)
	}
	// A blocked packet waits on system setup by default. It claims the owner
	// only when at least one issue route is an owner decision - the appended
	// planner turn already left the session waiting on the owner in that
	// case, so there is nothing to change.
	if evaluated.Status == domain.PlanningBlocked && session.WaitingOn != domain.PlanningWaitingSystem {
		waitsOnOwner := false
		for _, issue := range evaluated.Issues {
			if issue.Route.WaitsOnOwner() {
				waitsOnOwner = true
				break
			}
		}
		if !waitsOnOwner {
			session, err = s.planningSessions.SetPlanningSessionWaitingSystem(ctx, session.ID, session.Revision)
			if err != nil {
				return PlanningView{}, planningAPIError(err)
			}
		}
	}
	view, err := s.planningView(ctx, outcomeRecord, session)
	if err != nil {
		return PlanningView{}, err
	}
	view.Readiness = &evaluated
	return view, nil
}

func (s *Service) finishPlanningProposal(ctx context.Context, outcomeRecord domain.Outcome, revision domain.ContractRevision, session domain.PlanningSession, runID domain.IntelligenceRunID, draft domain.PlanDraftProposal, proposalSnapshot ports.RoutingInventorySnapshot, routingPreference *domain.RoutingPreference) (PlanningView, error) {
	if existing, found, err := s.planningSessions.GetPlanRevisionByPlanningSession(ctx, outcomeRecord.ID, session.ID); err != nil {
		return PlanningView{}, err
	} else if found {
		if session.Status == domain.PlanningSessionActive {
			session, err = s.planningSessions.LinkPlanningSessionPlan(ctx, session.ID, session.Revision, existing.ID, existing.SourceIntelligenceRunID)
			if err != nil {
				return PlanningView{}, planningAPIError(err)
			}
		}
		return s.planningView(ctx, outcomeRecord, session)
	}
	projectID, _, err := s.projectForOutcome(ctx, outcomeRecord.ID)
	if err != nil {
		return PlanningView{}, err
	}
	aliases, err := criterionAliases(revision)
	if err != nil {
		return PlanningView{}, err
	}
	units, decisions, _, err := s.compileAndRoutePlan(ctx, projectID, revision, draft, aliases, routingPreference, proposalSnapshot)
	if err != nil {
		return PlanningView{}, err
	}
	grants := grantsForUnits(units)
	if err := s.authorizeCapabilities(revision, grants, units); err != nil {
		return PlanningView{}, err
	}
	digest, err := domain.ComputePlanRunBriefCoreDigest(revision, units, grants)
	if err != nil {
		return PlanningView{}, err
	}
	plan := domain.PlanRevision{
		ID: domain.PlanRevisionID("plan-" + uuid.NewString()), OutcomeID: outcomeRecord.ID, ContractRevisionNumber: revision.Number,
		Status: domain.PlanStatusProposed, Summary: draft.Summary, Assumptions: append([]string(nil), draft.Assumptions...),
		Blockers: append([]string(nil), draft.Blockers...), WorkUnits: units, Grants: grants, RoutingDecisions: decisions,
		RunBriefCoreDigest: digest, PlanningSessionID: session.ID, SourceIntelligenceRunID: runID,
	}
	validation := plan
	validation.Number = 1
	if err := validation.ValidateAgainstContract(revision); err != nil {
		return PlanningView{}, apierr.Invalid("PLAN_DRAFT_CRITERIA_INVALID", err.Error(), nil)
	}
	proposalStage, err := s.EvaluateAdmissionStage(ctx, ports.AdmissionStageInput{Stage: ports.AdmissionStageProposal, ProjectID: projectID, Outcome: &outcomeRecord, Contract: &revision, Plan: &validation, RoutingSnapshot: &proposalSnapshot})
	if err != nil {
		return PlanningView{}, err
	}
	if !proposalStage.Eligible {
		if s.admission == nil {
			return PlanningView{}, apierr.Internal("ADMISSION_STORE_UNWIRED", "Admission persistence is unavailable in this environment")
		}
		proposalStage.Verdict.PlanRevisionID = nil
		if err := s.admission.AppendAdmissionEvaluation(ctx, proposalStage.Verdict); err != nil {
			return PlanningView{}, fmt.Errorf("persist rejected proposal admission: %w", err)
		}
		return PlanningView{}, apierr.New(apierr.KindConflict, "PLAN_PROPOSAL_NOT_ADMITTED", "This proposal cannot be routed under the verified capabilities", map[string]any{"reasons": proposalStage.Verdict.Reasons})
	}
	saved, err := s.store.AppendPlanRevision(ctx, outcomeRecord.ID, plan)
	if err != nil {
		if existing, found, findErr := s.planningSessions.GetPlanRevisionByPlanningSession(ctx, outcomeRecord.ID, session.ID); findErr == nil && found {
			saved = existing
		} else {
			currentOutcome, outcomeFound, outcomeErr := s.store.GetOutcome(ctx, outcomeRecord.ID)
			if outcomeErr != nil {
				return PlanningView{}, outcomeErr
			}
			if outcomeFound {
				session, outcomeErr = s.reconcilePlanningContract(ctx, currentOutcome, session)
				if outcomeErr != nil {
					return PlanningView{}, outcomeErr
				}
				if session.Status == domain.PlanningSessionSuperseded {
					return PlanningView{}, apierr.New(apierr.KindConflict, "PLANNING_CONTRACT_STALE", "The Contract changed. Start a new planning conversation.", map[string]any{"currentRevision": currentOutcome.CurrentRevisionNumber})
				}
			}
			return PlanningView{}, err
		}
	}
	session, found, err := s.planningSessions.GetPlanningSession(ctx, outcomeRecord.ID, session.ID)
	if err != nil {
		return PlanningView{}, err
	}
	if !found {
		return PlanningView{}, apierr.NotFound("PLANNING_SESSION_NOT_FOUND", "That planning conversation does not exist")
	}
	// Compatibility for a Plan written by an older feature build that crashed
	// before the session link. New canonical SQLite writes link atomically.
	if session.Status == domain.PlanningSessionActive {
		session, err = s.planningSessions.LinkPlanningSessionPlan(ctx, session.ID, session.Revision, saved.ID, saved.SourceIntelligenceRunID)
		if err != nil {
			currentOutcome, outcomeFound, outcomeErr := s.store.GetOutcome(ctx, outcomeRecord.ID)
			if outcomeErr != nil {
				return PlanningView{}, outcomeErr
			}
			if outcomeFound {
				session, outcomeErr = s.reconcilePlanningContract(ctx, currentOutcome, session)
				if outcomeErr != nil {
					return PlanningView{}, outcomeErr
				}
				if session.Status == domain.PlanningSessionSuperseded {
					return PlanningView{}, apierr.New(apierr.KindConflict, "PLANNING_CONTRACT_STALE", "The Contract changed. Start a new planning conversation.", map[string]any{"currentRevision": currentOutcome.CurrentRevisionNumber})
				}
			}
			return PlanningView{}, planningAPIError(err)
		}
	}
	return s.planningView(ctx, outcomeRecord, session)
}

func (s *Service) reconcilePlanningContract(ctx context.Context, outcomeRecord domain.Outcome, session domain.PlanningSession) (domain.PlanningSession, error) {
	for attempt := 0; attempt < 2; attempt++ {
		if session.Status != domain.PlanningSessionActive || session.ContractRevisionNumber == outcomeRecord.CurrentRevisionNumber {
			return session, nil
		}
		closed, err := s.planningSessions.ClosePlanningSession(ctx, session.ID, session.Revision, domain.PlanningSessionSuperseded)
		if err == nil {
			return closed, nil
		}
		var conflict *ports.PlanningSessionRevisionConflictError
		if !errors.As(err, &conflict) {
			return domain.PlanningSession{}, planningAPIError(err)
		}
		var found bool
		session, found, err = s.planningSessions.GetPlanningSession(ctx, outcomeRecord.ID, session.ID)
		if err != nil {
			return domain.PlanningSession{}, err
		}
		if !found {
			return domain.PlanningSession{}, apierr.NotFound("PLANNING_SESSION_NOT_FOUND", "That planning conversation does not exist")
		}
	}
	return domain.PlanningSession{}, apierr.New(apierr.KindConflict, "PLANNING_REVISION_CONFLICT", "The planning conversation changed. Reload and try again.", nil)
}

func (s *Service) planningView(ctx context.Context, outcomeRecord domain.Outcome, session domain.PlanningSession) (PlanningView, error) {
	turns, err := s.planningSessions.ListPlanningTurns(ctx, session.ID)
	if err != nil {
		return PlanningView{}, err
	}
	view := PlanningView{Outcome: outcomeRecord, Session: session, Turns: turns}
	if plan, found, err := s.planningSessions.GetPlanRevisionByPlanningSession(ctx, outcomeRecord.ID, session.ID); err != nil {
		return PlanningView{}, err
	} else if found {
		view.ProposedPlan = &plan
	}
	return view, nil
}

func (s *Service) planningLineage(ctx context.Context, outcomeID domain.OutcomeID, expected int64) (domain.Outcome, domain.ContractRevision, error) {
	outcomeRecord, found, err := s.store.GetOutcome(ctx, outcomeID)
	if err != nil {
		return domain.Outcome{}, domain.ContractRevision{}, err
	}
	if !found {
		return domain.Outcome{}, domain.ContractRevision{}, apierr.NotFound("OUTCOME_NOT_FOUND", "That Outcome does not exist")
	}
	if expected < 1 {
		return domain.Outcome{}, domain.ContractRevision{}, apierr.Invalid("EXPECTED_REVISION_REQUIRED", "State which Contract revision planning uses", nil)
	}
	if outcomeRecord.CurrentRevisionNumber != expected {
		return domain.Outcome{}, domain.ContractRevision{}, apierr.New(apierr.KindConflict, "PLANNING_CONTRACT_STALE", "Reload the Outcome before planning", map[string]any{"currentRevision": outcomeRecord.CurrentRevisionNumber})
	}
	revision, err := s.currentRevision(ctx, outcomeRecord)
	return outcomeRecord, revision, err
}

func planningRepositoryReadAllowed(revision domain.ContractRevision) bool {
	return revision.AuthorityCeiling.ReadWorkspace || revision.AuthorityCeiling.WriteWorkspace || revision.AuthorityCeiling.ExecuteLocal
}

func planningStartFingerprint(outcomeID domain.OutcomeID, revision int64, candidateID string, contextMode domain.PlanningContextMode) domain.SHA256Digest {
	payload, _ := json.Marshal(struct {
		OutcomeID string                     `json:"outcomeId"`
		Revision  int64                      `json:"contractRevision"`
		Candidate string                     `json:"candidateId"`
		Context   domain.PlanningContextMode `json:"contextMode"`
	}{string(outcomeID), revision, candidateID, contextMode})
	return domain.DigestSHA256(payload)
}

// planningTurnKind records what the planner's envelope claimed. A ready claim
// is a proposal turn; needs_context is a clarification turn; a blocked packet
// is a readiness_blocked turn whatever its routes - the packet payload and
// the session wait state carry the detail, never a fabricated kind. The
// envelope schema admits nothing else.
func planningTurnKind(result domain.PlanningReadinessResult) (domain.PlanningTurnKind, error) {
	switch result.Status {
	case domain.PlanningNeedsContext:
		return domain.PlanningTurnClarification, nil
	case domain.PlanningReady:
		return domain.PlanningTurnPlanProposal, nil
	case domain.PlanningBlocked:
		return domain.PlanningTurnReadinessBlocked, nil
	default:
		return "", apierr.Internal("PLANNING_REPLY_INVALID", "The planning agent returned an unsupported reply")
	}
}

func planningAPIError(err error) error {
	var revision *ports.PlanningSessionRevisionConflictError
	if errors.As(err, &revision) {
		return apierr.New(apierr.KindConflict, "PLANNING_REVISION_CONFLICT", "The planning conversation changed. Reload and try again.", map[string]any{"currentRevision": revision.Current})
	}
	var request *ports.PlanningRequestConflictError
	if errors.As(err, &request) {
		return apierr.New(apierr.KindConflict, "PLANNING_REQUEST_CONFLICT", "That request key was already used for a different planning action", nil)
	}
	var finalize *ports.PlanningFinalizeConflictError
	if errors.As(err, &finalize) {
		return apierr.New(apierr.KindConflict, "PLANNING_FINALIZE_CONFLICT", "The Contract or planning conversation changed before the Plan could be saved", nil)
	}
	return err
}
