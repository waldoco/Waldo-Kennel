package controllers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apispec"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/envelope"
	outcomevc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

// OutcomeService is the controller-facing Outcome contract (#21) plus the
// Decide & Authorize boundary (#26).
type OutcomeService interface {
	Create(ctx context.Context, in outcomevc.CreateInput) (outcomevc.View, error)
	ReviseContract(ctx context.Context, id domain.OutcomeID, in outcomevc.ReviseContractInput) (outcomevc.View, error)
	Get(ctx context.Context, id domain.OutcomeID) (outcomevc.View, error)
	ListByProject(ctx context.Context, projectID domain.ProjectID) ([]outcomevc.View, error)
	ProposePlan(ctx context.Context, id domain.OutcomeID, expectedContractRevision int64) (outcomevc.PlanView, error)
	ApprovePlan(ctx context.Context, id domain.OutcomeID, in outcomevc.ApprovePlanInput) (outcomevc.AuthorizedPlanView, error)
	GetLatestPlan(ctx context.Context, id domain.OutcomeID) (outcomevc.PlanView, error)

	// Composed Outcomes (ADR 0007). A contributing Outcome is born only by
	// authorizing a decomposition: there is deliberately no ad-hoc create
	// route, because one would bypass the coverage, containment, and ordering
	// gates that make a decomposition trustworthy.
	Composition(ctx context.Context, id domain.OutcomeID) (outcomevc.CompositionView, error)
	ProposeDecomposition(ctx context.Context, parentID domain.OutcomeID, in outcomevc.ProposeDecompositionInput) (outcomevc.DecompositionView, error)
	AuthorizeDecomposition(ctx context.Context, parentID domain.OutcomeID, decompositionID domain.DecompositionRevisionID) (outcomevc.DecompositionView, error)
	LatestDecomposition(ctx context.Context, parentID domain.OutcomeID) (outcomevc.DecompositionView, error)
	WaiveContributionDependency(ctx context.Context, parentID domain.OutcomeID, in outcomevc.WaiveDependencyInput) (outcomevc.DecompositionView, error)

	// Agent-authored decomposition (ADR 0007). The daemon has no synchronous
	// model call, so the ask returns immediately and the proposal arrives on
	// the callback route.
	AskForDecomposition(ctx context.Context, outcomeID domain.OutcomeID, expectedContractRevision int64) (outcomevc.DecompositionRequestView, error)
	SubmitAgentProposal(ctx context.Context, requestID domain.DecompositionRequestID, token string, in outcomevc.ProposeDecompositionInput, raw string) (outcomevc.DecompositionRequestView, error)
	LatestDecompositionRequest(ctx context.Context, outcomeID domain.OutcomeID) (outcomevc.DecompositionRequestView, error)
}

// AttemptManager is the Act & Observe boundary (#31). A nil field answers 501
// on the attempt routes, mirroring every other unwired capability.
type NeedsYouManager interface {
	CurrentNeedsYou(context.Context, domain.OutcomeID) ([]domain.NeedsYouQuestion, error)
	AnswerNeedsYou(context.Context, domain.OutcomeID, string, domain.NeedsYouAnswer) (domain.NeedsYouQuestion, error)
	ReconcileNeedsYou(context.Context, domain.OutcomeID, string, string) (domain.NeedsYouQuestion, error)
}

type AttemptManager interface {
	StartAttempt(ctx context.Context, outcomeID domain.OutcomeID, in outcomevc.StartAttemptInput) (outcomevc.AttemptView, error)
	GetAttempt(ctx context.Context, outcomeID domain.OutcomeID, attemptID domain.AttemptID) (outcomevc.AttemptView, error)
	ListAttempts(ctx context.Context, outcomeID domain.OutcomeID) ([]outcomevc.AttemptView, error)
	CancelAttempt(ctx context.Context, outcomeID domain.OutcomeID, attemptID domain.AttemptID) (outcomevc.AttemptView, error)
	RecordObservation(ctx context.Context, outcomeID domain.OutcomeID, attemptID domain.AttemptID, in outcomevc.RecordObservationInput) (domain.AttemptObservation, error)
	RecoverAttempt(ctx context.Context, outcomeID domain.OutcomeID, attemptID domain.AttemptID, in outcomevc.RecoveryInput) (outcomevc.RecoveryView, error)
}

// ProofManager is the Prove & Close boundary (#35). It remains separate from
// the earlier Outcome and Attempt interfaces so unwired proof routes fail 501
// without granting acceptance authority to another surface.
type ProofManager interface {
	GetProof(context.Context, domain.OutcomeID) (outcomevc.ProofView, error)
	RecordEvidence(context.Context, domain.OutcomeID, outcomevc.RecordEvidenceInput) (outcomevc.ProofView, error)
	RecordVerification(context.Context, domain.OutcomeID, outcomevc.RecordVerificationInput) (outcomevc.ProofView, error)
	DecideAcceptance(context.Context, domain.OutcomeID, outcomevc.DecideAcceptanceInput) (outcomevc.ProofView, error)

	// Batched parent acceptance (ADR 0007). One sitting produces N separate
	// immutable decisions; the daemon may only withhold a contributor from the
	// batch, never accept one.
	BatchEligibility(context.Context, domain.OutcomeID) ([]domain.BatchEntryVerdict, error)
	AcceptContributorBatch(context.Context, domain.OutcomeID, outcomevc.AcceptBatchInput) (outcomevc.AcceptBatchView, error)
}

// OutcomesController owns the canonical Outcome contract routes.
type OutcomesController struct {
	Svc      OutcomeService
	Attempts AttemptManager
	Proof    ProofManager
	NeedsYou NeedsYouManager
}

// Register mounts the Outcome routes on the supplied router.
func (c *OutcomesController) Register(r chi.Router) {
	r.Get("/projects/{id}/outcomes", c.list)
	r.Post("/projects/{id}/outcomes", c.create)
	r.Get("/outcomes/{outcomeId}", c.get)
	r.Get("/outcomes/{outcomeId}/needs-you", c.currentNeedsYou)
	r.Post("/outcomes/{outcomeId}/needs-you/{questionId}/answers", c.answerNeedsYou)
	r.Post("/outcomes/{outcomeId}/needs-you/{questionId}/reconcile", c.reconcileNeedsYou)
	r.Get("/projects/{id}/outcome-trash", c.trashedOutcomes)
	r.Get("/outcomes/{outcomeId}/deletion", c.deletionPreview)
	r.Post("/outcomes/{outcomeId}/deletion", c.changeDeletion)
	r.Post("/outcomes/{outcomeId}/revisions", c.revise)
	r.Post("/outcomes/{outcomeId}/plans", c.proposePlan)
	r.Post("/outcomes/{outcomeId}/plans/replan", c.replanPlan)
	r.Post("/outcomes/{outcomeId}/plans/{planId}/approval", c.approvePlan)
	r.Get("/outcomes/{outcomeId}/plan", c.latestPlan)
	r.Get("/outcomes/{outcomeId}/planning-candidates", c.planningCandidates)
	r.Post("/outcomes/{outcomeId}/planning-sessions", c.startPlanning)
	r.Get("/outcomes/{outcomeId}/planning-session", c.currentPlanning)
	r.Get("/outcomes/{outcomeId}/planning-sessions/{planningSessionId}", c.getPlanning)
	r.Post("/outcomes/{outcomeId}/planning-sessions/{planningSessionId}/messages", c.continuePlanning)
	r.Post("/outcomes/{outcomeId}/planning-sessions/{planningSessionId}/proposal", c.finalizePlanning)
	r.Post("/outcomes/{outcomeId}/planning-sessions/{planningSessionId}/cancel", c.cancelPlanning)
	r.Get("/outcomes/{outcomeId}/plans/{planId}/schedule", c.schedule)
	r.Post("/outcomes/{outcomeId}/attempts", c.startAttempt)
	r.Get("/outcomes/{outcomeId}/attempts", c.listAttempts)
	r.Get("/outcomes/{outcomeId}/attempts/{attemptId}", c.getAttempt)
	r.Post("/outcomes/{outcomeId}/attempts/{attemptId}/observations", c.recordObservation)
	r.Post("/outcomes/{outcomeId}/attempts/{attemptId}/cancel", c.cancelAttempt)
	r.Post("/outcomes/{outcomeId}/attempts/{attemptId}/recovery", c.recoverAttempt)
	r.Get("/outcomes/{outcomeId}/composition", c.getComposition)
	r.Post("/outcomes/{outcomeId}/decompositions", c.proposeDecomposition)
	r.Post("/outcomes/{outcomeId}/decompositions/{decompositionId}/authorization", c.authorizeDecomposition)
	r.Get("/outcomes/{outcomeId}/decomposition", c.latestDecomposition)
	r.Post("/outcomes/{outcomeId}/decomposition/waivers", c.waiveContributionDependency)
	r.Post("/outcomes/{outcomeId}/decomposition-requests", c.askForDecomposition)
	r.Get("/outcomes/{outcomeId}/decomposition-request", c.latestDecompositionRequest)
	// Addressed by REQUEST, not by Outcome: the answering agent knows only the
	// request it was handed, and scoping the callback that way is what stops a
	// confused agent answering for a different Outcome.
	r.Post("/decomposition-requests/{requestId}/proposal", c.submitAgentProposal)
	r.Get("/outcomes/{outcomeId}/proof", c.getProof)
	r.Post("/outcomes/{outcomeId}/evidence", c.recordEvidence)
	r.Post("/outcomes/{outcomeId}/verifications", c.recordVerification)
	r.Post("/outcomes/{outcomeId}/acceptance-decisions", c.decideAcceptance)
	r.Get("/outcomes/{outcomeId}/acceptance-batch", c.batchEligibility)
	r.Post("/outcomes/{outcomeId}/acceptance-batch", c.acceptContributorBatch)
	c.registerRunRoutes(r)
}

func (c *OutcomesController) getProof(w http.ResponseWriter, r *http.Request) {
	if c.Proof == nil {
		apispec.NotImplemented(w, r, http.MethodGet, "/api/v1/outcomes/{outcomeId}/proof")
		return
	}
	view, err := c.Proof.GetProof(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")))
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, OutcomeProofEnvelope{Proof: outcomeProofResponse(view)})
}

func (c *OutcomesController) recordEvidence(w http.ResponseWriter, r *http.Request) {
	if c.Proof == nil {
		apispec.NotImplemented(w, r, http.MethodPost, "/api/v1/outcomes/{outcomeId}/evidence")
		return
	}
	var req RecordEvidenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "bad_request", "INVALID_JSON", "Invalid JSON body", nil)
		return
	}
	view, err := c.Proof.RecordEvidence(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")), outcomevc.RecordEvidenceInput{
		ExpectedContractRevision: req.ExpectedContractRevision, ContractRevisionID: domain.ContractRevisionID(req.ContractRevisionID),
		CriterionID: domain.CriterionID(req.CriterionID), SubjectType: domain.ProofSubjectType(req.SubjectType), SubjectID: req.SubjectID,
		SubjectRevision: req.SubjectRevision, Kind: domain.EvidenceKind(req.Kind), SourceType: domain.EvidenceSourceType(req.SourceType),
		SourceRef: req.SourceRef, ProducerType: domain.EvidenceProducerType(req.ProducerType), ProducerRef: req.ProducerRef,
		Summary: req.Summary, ContentDigest: req.ContentDigest, RequestKey: req.RequestKey,
	})
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusCreated, OutcomeProofEnvelope{Proof: outcomeProofResponse(view)})
}

func (c *OutcomesController) recordVerification(w http.ResponseWriter, r *http.Request) {
	if c.Proof == nil {
		apispec.NotImplemented(w, r, http.MethodPost, "/api/v1/outcomes/{outcomeId}/verifications")
		return
	}
	var req RecordVerificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "bad_request", "INVALID_JSON", "Invalid JSON body", nil)
		return
	}
	evidenceIDs := make([]domain.EvidenceItemID, 0, len(req.EvidenceItemIDs))
	for _, id := range req.EvidenceItemIDs {
		evidenceIDs = append(evidenceIDs, domain.EvidenceItemID(id))
	}
	view, err := c.Proof.RecordVerification(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")), outcomevc.RecordVerificationInput{
		ExpectedContractRevision: req.ExpectedContractRevision, ContractRevisionID: domain.ContractRevisionID(req.ContractRevisionID),
		CriterionID: domain.CriterionID(req.CriterionID), SubjectType: domain.ProofSubjectType(req.SubjectType), SubjectID: req.SubjectID,
		SubjectRevision: req.SubjectRevision, EvidenceItemIDs: evidenceIDs, Method: req.Method,
		IndependenceClass: domain.VerificationIndependenceClass(req.IndependenceClass), Result: domain.VerificationResult(req.Result),
		ProducerRef: req.ProducerRef, VerifierRef: req.VerifierRef, ProducerProvider: req.ProducerProvider,
		VerifierProvider: req.VerifierProvider, Detail: req.Detail, RequestKey: req.RequestKey,
	})
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusCreated, OutcomeProofEnvelope{Proof: outcomeProofResponse(view)})
}

func (c *OutcomesController) decideAcceptance(w http.ResponseWriter, r *http.Request) {
	if c.Proof == nil {
		apispec.NotImplemented(w, r, http.MethodPost, "/api/v1/outcomes/{outcomeId}/acceptance-decisions")
		return
	}
	var req DecideAcceptanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "bad_request", "INVALID_JSON", "Invalid JSON body", nil)
		return
	}
	view, err := c.Proof.DecideAcceptance(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")), outcomevc.DecideAcceptanceInput{
		ExpectedContractRevision: req.ExpectedContractRevision, ContractRevisionID: domain.ContractRevisionID(req.ContractRevisionID),
		Kind: domain.AcceptanceDecisionKind(req.Kind), Summary: req.Summary, ResourceDisposition: domain.ResourceDisposition(req.ResourceDisposition),
		ReentryTargetType: domain.ReentryTargetType(req.ReentryTargetType), ReentryTargetID: req.ReentryTargetID, RequestKey: req.RequestKey,
	})
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusCreated, OutcomeProofEnvelope{Proof: outcomeProofResponse(view)})
}

func (c *OutcomesController) list(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, http.MethodGet, "/api/v1/projects/{id}/outcomes")
		return
	}
	views, err := c.Svc.ListByProject(r.Context(), domain.ProjectID(chi.URLParam(r, "id")))
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	outcomes := make([]OutcomeResponse, 0, len(views))
	for _, view := range views {
		outcomes = append(outcomes, outcomeResponse(view))
	}
	envelope.WriteJSON(w, http.StatusOK, OutcomesEnvelope{Outcomes: outcomes})
}

func (c *OutcomesController) proposePlan(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, http.MethodPost, "/api/v1/outcomes/{outcomeId}/plans")
		return
	}
	var req ProposePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "bad_request", "INVALID_JSON", "Invalid JSON body", nil)
		return
	}
	view, err := c.Svc.ProposePlan(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")), req.ExpectedContractRevision)
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusCreated, PlanEnvelope{Plan: planRevisionResponse(view.Plan)})
}

func (c *OutcomesController) replanPlan(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, http.MethodPost, "/api/v1/outcomes/{outcomeId}/plans/replan")
		return
	}
	var req ReplanPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "bad_request", "INVALID_JSON", "Invalid JSON body", nil)
		return
	}
	replanner, ok := c.Svc.(interface {
		ReplanPlan(context.Context, domain.OutcomeID, int64, string) (outcomevc.PlanView, error)
	})
	if !ok {
		apispec.NotImplemented(w, r, http.MethodPost, "/api/v1/outcomes/{outcomeId}/plans/replan")
		return
	}
	view, err := replanner.ReplanPlan(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")), req.ExpectedContractRevision, req.Feedback)
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusCreated, PlanEnvelope{Plan: planRevisionResponse(view.Plan)})
}

func (c *OutcomesController) approvePlan(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, http.MethodPost, "/api/v1/outcomes/{outcomeId}/plans/{planId}/approval")
		return
	}
	var req ApprovePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "bad_request", "INVALID_JSON", "Invalid JSON body", nil)
		return
	}
	view, err := c.Svc.ApprovePlan(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")), outcomevc.ApprovePlanInput{
		PlanRevisionID:           domain.PlanRevisionID(chi.URLParam(r, "planId")),
		ExpectedContractRevision: req.ExpectedContractRevision,
	})
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, PlanEnvelope{Plan: planRevisionResponse(view.Plan)})
}

func (c *OutcomesController) schedule(w http.ResponseWriter, r *http.Request) {
	if c.Attempts == nil {
		apispec.NotImplemented(w, r, http.MethodGet, "/api/v1/outcomes/{outcomeId}/plans/{planId}/schedule")
		return
	}
	scheduler, ok := c.Attempts.(interface {
		GetSchedule(context.Context, domain.OutcomeID, domain.PlanRevisionID) (outcomevc.ScheduleView, error)
	})
	if !ok {
		apispec.NotImplemented(w, r, http.MethodGet, "/api/v1/outcomes/{outcomeId}/plans/{planId}/schedule")
		return
	}
	view, err := scheduler.GetSchedule(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")), domain.PlanRevisionID(chi.URLParam(r, "planId")))
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, ScheduleEnvelope{Schedule: scheduleResponse(view)})
}

func (c *OutcomesController) latestPlan(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, http.MethodGet, "/api/v1/outcomes/{outcomeId}/plan")
		return
	}
	view, err := c.Svc.GetLatestPlan(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")))
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, PlanEnvelope{Plan: planRevisionResponse(view.Plan)})
}

func (c *OutcomesController) create(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, http.MethodPost, "/api/v1/projects/{id}/outcomes")
		return
	}
	var req CreateOutcomeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "bad_request", "INVALID_JSON", "Invalid JSON body", nil)
		return
	}
	view, err := c.Svc.Create(r.Context(), outcomevc.CreateInput{
		ProjectID:       domain.ProjectID(chi.URLParam(r, "id")),
		Title:           req.Title,
		Goal:            req.Goal,
		SuccessCriteria: req.SuccessCriteria,
		Review:          req.Review,
		Constraints:     req.Constraints,
		NonGoals:        req.NonGoals,
		Clarification:   req.Clarification,
		// Without a ceiling the Contract grants nothing and planning can never
		// authorize a WorkUnit, so the direct-create path must be able to state
		// one exactly as the Understand surfaces do.
		AuthorityCeiling: proposedAuthority(req.AuthorityCeiling),
		StopConditions:   req.StopConditions,
		RequestKey:       req.RequestKey,
	})
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusCreated, OutcomeEnvelope{Outcome: outcomeResponse(view)})
}

func (c *OutcomesController) get(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, http.MethodGet, "/api/v1/outcomes/{outcomeId}")
		return
	}
	view, err := c.Svc.Get(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")))
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, OutcomeEnvelope{Outcome: outcomeResponse(view)})
}

func (c *OutcomesController) revise(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, http.MethodPost, "/api/v1/outcomes/{outcomeId}/revisions")
		return
	}
	var req ReviseOutcomeContractRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "bad_request", "INVALID_JSON", "Invalid JSON body", nil)
		return
	}
	facets := make([]domain.ContractFacet, 0, len(req.Facets))
	for _, facet := range req.Facets {
		facets = append(facets, domain.ContractFacet{Kind: domain.ContractFacetKind(facet.Kind), Summary: facet.Summary, Requirements: facet.Requirements})
	}
	view, err := c.Svc.ReviseContract(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")), outcomevc.ReviseContractInput{
		CriterionEvidence: req.CriterionEvidence,
		TemporalCondition: req.TemporalCondition,
		Facets:            facets,
		ExpectedRevision:  req.ExpectedRevision,
		Goal:              req.Goal,
		SuccessCriteria:   req.SuccessCriteria,
		Review:            req.Review,
		Constraints:       req.Constraints,
		NonGoals:          req.NonGoals,
		Clarification:     req.Clarification,
		AuthorityCeiling:  proposedAuthority(req.AuthorityCeiling),
		StopConditions:    req.StopConditions,
	})
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, OutcomeEnvelope{Outcome: outcomeResponse(view)})
}

// startAttempt admits an approved plan onto a real provider session (#31).
func (c *OutcomesController) startAttempt(w http.ResponseWriter, r *http.Request) {
	if c.Attempts == nil {
		apispec.NotImplemented(w, r, http.MethodPost, "/api/v1/outcomes/{outcomeId}/attempts")
		return
	}
	var req StartOutcomeAttemptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "bad_request", "INVALID_JSON", "Invalid JSON body", nil)
		return
	}
	view, err := c.Attempts.StartAttempt(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")), outcomevc.StartAttemptInput{
		PlanRevisionID: domain.PlanRevisionID(req.PlanRevisionID),
		WorkUnitID:     domain.WorkUnitID(req.WorkUnitID),
		Harness:        domain.AgentHarness(req.Harness),
		RequestKey:     req.RequestKey,
	})
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusCreated, AttemptEnvelope{Attempt: attemptResponse(view)})
}

func (c *OutcomesController) listAttempts(w http.ResponseWriter, r *http.Request) {
	if c.Attempts == nil {
		apispec.NotImplemented(w, r, http.MethodGet, "/api/v1/outcomes/{outcomeId}/attempts")
		return
	}
	views, err := c.Attempts.ListAttempts(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")))
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	attempts := make([]AttemptResponse, 0, len(views))
	for _, view := range views {
		attempts = append(attempts, attemptResponse(view))
	}
	envelope.WriteJSON(w, http.StatusOK, AttemptListEnvelope{Attempts: attempts})
}

func (c *OutcomesController) getAttempt(w http.ResponseWriter, r *http.Request) {
	if c.Attempts == nil {
		apispec.NotImplemented(w, r, http.MethodGet, "/api/v1/outcomes/{outcomeId}/attempts/{attemptId}")
		return
	}
	view, err := c.Attempts.GetAttempt(r.Context(),
		domain.OutcomeID(chi.URLParam(r, "outcomeId")),
		domain.AttemptID(chi.URLParam(r, "attemptId")))
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, AttemptEnvelope{Attempt: attemptResponse(view)})
}

func (c *OutcomesController) recordObservation(w http.ResponseWriter, r *http.Request) {
	if c.Attempts == nil {
		apispec.NotImplemented(w, r, http.MethodPost, "/api/v1/outcomes/{outcomeId}/attempts/{attemptId}/observations")
		return
	}
	var req RecordObservationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "bad_request", "INVALID_JSON", "Invalid JSON body", nil)
		return
	}
	obs, err := c.Attempts.RecordObservation(r.Context(),
		domain.OutcomeID(chi.URLParam(r, "outcomeId")),
		domain.AttemptID(chi.URLParam(r, "attemptId")),
		outcomevc.RecordObservationInput{Kind: req.Kind, Payload: req.Payload})
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusCreated, ObservationEnvelope{Observation: attemptObservationResponse(obs)})
}

func (c *OutcomesController) cancelAttempt(w http.ResponseWriter, r *http.Request) {
	if c.Attempts == nil {
		apispec.NotImplemented(w, r, http.MethodPost, "/api/v1/outcomes/{outcomeId}/attempts/{attemptId}/cancel")
		return
	}
	view, err := c.Attempts.CancelAttempt(r.Context(),
		domain.OutcomeID(chi.URLParam(r, "outcomeId")),
		domain.AttemptID(chi.URLParam(r, "attemptId")))
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, AttemptEnvelope{Attempt: attemptResponse(view)})
}

func (c *OutcomesController) recoverAttempt(w http.ResponseWriter, r *http.Request) {
	if c.Attempts == nil {
		apispec.NotImplemented(w, r, http.MethodPost, "/api/v1/outcomes/{outcomeId}/attempts/{attemptId}/recovery")
		return
	}
	var req AttemptRecoveryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "bad_request", "INVALID_JSON", "Invalid JSON body", nil)
		return
	}
	view, err := c.Attempts.RecoverAttempt(r.Context(),
		domain.OutcomeID(chi.URLParam(r, "outcomeId")),
		domain.AttemptID(chi.URLParam(r, "attemptId")),
		outcomevc.RecoveryInput{Action: outcomevc.RecoveryAction(req.Action), ConfirmProviderStopped: req.ConfirmProviderStopped})
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	resp := AttemptRecoveryEnvelope{Attempt: attemptResponse(view.Attempt)}
	if view.Receipt != nil {
		receipt := recoveryReceiptResponse(*view.Receipt)
		resp.Receipt = &receipt
	}
	envelope.WriteJSON(w, http.StatusOK, resp)
}

// getComposition reports derived shape, contributors, and criterion coverage.
// A direct Outcome answers 200 with shape "direct" and no contributors: the
// absence of composition is a fact, not a missing resource.
func (c *OutcomesController) getComposition(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, http.MethodGet, "/api/v1/outcomes/{outcomeId}/composition")
		return
	}
	view, err := c.Svc.Composition(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")))
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	// Each contributor carries its own contract so Mission Control can render
	// the decomposition without a request per child.
	contributors := make([]outcomevc.View, 0, len(view.Contributors))
	for _, contributor := range view.Contributors {
		full, err := c.Svc.Get(r.Context(), contributor.Outcome.ID)
		if err != nil {
			envelope.WriteError(w, r, err)
			return
		}
		contributors = append(contributors, full)
	}
	envelope.WriteJSON(w, http.StatusOK, OutcomeCompositionEnvelope{Composition: outcomeCompositionResponse(view, contributors)})
}

// proposeDecomposition validates a decomposition and records it as a proposal.
// It creates no Outcome: a refused proposal leaves nothing behind, and an
// accepted one is still only an offer until the owner authorizes it.
func (c *OutcomesController) proposeDecomposition(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, http.MethodPost, "/api/v1/outcomes/{outcomeId}/decompositions")
		return
	}
	var req ProposeDecompositionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "bad_request", "INVALID_JSON", "Invalid JSON body", nil)
		return
	}
	view, err := c.Svc.ProposeDecomposition(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")), proposeDecompositionInput(req))
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusCreated, DecompositionEnvelope{Decomposition: decompositionResponse(view)})
}

// authorizeDecomposition is the owner decision that creates the contributing
// Outcomes. Re-authorizing an already authorized decomposition is a replay and
// answers 200 with the same result rather than creating them twice.
func (c *OutcomesController) authorizeDecomposition(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, http.MethodPost, "/api/v1/outcomes/{outcomeId}/decompositions/{decompositionId}/authorization")
		return
	}
	view, err := c.Svc.AuthorizeDecomposition(r.Context(),
		domain.OutcomeID(chi.URLParam(r, "outcomeId")),
		domain.DecompositionRevisionID(chi.URLParam(r, "decompositionId")))
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, DecompositionEnvelope{Decomposition: decompositionResponse(view)})
}

func (c *OutcomesController) latestDecomposition(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, http.MethodGet, "/api/v1/outcomes/{outcomeId}/decomposition")
		return
	}
	view, err := c.Svc.LatestDecomposition(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")))
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, DecompositionEnvelope{Decomposition: decompositionResponse(view)})
}

// waiveContributionDependency records the owner's decision to start a
// contributing Outcome before a declared upstream is accepted. Only the owner
// may waive, the reason is durable, and the waiver never disappears.
func (c *OutcomesController) waiveContributionDependency(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, http.MethodPost, "/api/v1/outcomes/{outcomeId}/decomposition/waivers")
		return
	}
	var req WaiveContributionDependencyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "bad_request", "INVALID_JSON", "Invalid JSON body", nil)
		return
	}
	view, err := c.Svc.WaiveContributionDependency(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")), outcomevc.WaiveDependencyInput{
		FromRef: req.FromRef, ToRef: req.ToRef, Reason: req.Reason,
	})
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusCreated, DecompositionEnvelope{Decomposition: decompositionResponse(view)})
}

// batchEligibility reports who could be accepted right now, and why the others
// could not, without deciding anything.
func (c *OutcomesController) batchEligibility(w http.ResponseWriter, r *http.Request) {
	if c.Proof == nil {
		apispec.NotImplemented(w, r, http.MethodGet, "/api/v1/outcomes/{outcomeId}/acceptance-batch")
		return
	}
	verdicts, err := c.Proof.BatchEligibility(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")))
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, BatchEligibilityEnvelope{Contributors: batchEntryVerdicts(verdicts)})
}

// acceptContributorBatch records one owner sitting. Every accepted Outcome
// gets its own immutable decision; excluded contributors are reported with a
// reason and a remedy rather than silently dropped.
func (c *OutcomesController) acceptContributorBatch(w http.ResponseWriter, r *http.Request) {
	if c.Proof == nil {
		apispec.NotImplemented(w, r, http.MethodPost, "/api/v1/outcomes/{outcomeId}/acceptance-batch")
		return
	}
	var req AcceptContributorBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "bad_request", "INVALID_JSON", "Invalid JSON body", nil)
		return
	}
	ids := make([]domain.OutcomeID, 0, len(req.OutcomeIds))
	for _, id := range req.OutcomeIds {
		ids = append(ids, domain.OutcomeID(id))
	}
	view, err := c.Proof.AcceptContributorBatch(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")), outcomevc.AcceptBatchInput{
		ExpectedContractRevision: req.ExpectedContractRevision,
		OutcomeIDs:               ids,
		Summary:                  req.Summary,
		AcceptParent:             req.AcceptParent,
		ResourceDisposition:      domain.ResourceDisposition(req.ResourceDisposition),
		RequestKey:               req.RequestKey,
	})
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusCreated, AcceptBatchEnvelope{Batch: acceptBatchResponse(view)})
}

// DecompositionCallbackTokenHeader carries the scoping token a spawned agent
// was handed. It is NOT authentication — the loopback listener is
// unauthenticated by deliberate decision (ADR 0001), so this cannot stop a
// hostile local process. It scopes an answer to one request, single-use and
// expiring, which is what stops a confused or retrying agent answering for the
// wrong Outcome. See ADR 0007.
const DecompositionCallbackTokenHeader = "X-Kennel-Decomposition-Token" //nolint:gosec // header NAME, not a credential; the token itself is minted per request and never constant.

// askForDecomposition opens a durable ask and starts an agent on it. It
// returns as soon as the agent is started; the proposal arrives later.
func (c *OutcomesController) askForDecomposition(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, http.MethodPost, "/api/v1/outcomes/{outcomeId}/decomposition-requests")
		return
	}
	var req AskForDecompositionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "bad_request", "INVALID_JSON", "Invalid JSON body", nil)
		return
	}
	view, err := c.Svc.AskForDecomposition(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")), req.ExpectedContractRevision)
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusAccepted, DecompositionRequestEnvelope{Request: decompositionRequestResponse(view)})
}

// latestDecompositionRequest reports the newest ask and what became of it.
func (c *OutcomesController) latestDecompositionRequest(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, http.MethodGet, "/api/v1/outcomes/{outcomeId}/decomposition-request")
		return
	}
	view, err := c.Svc.LatestDecompositionRequest(r.Context(), domain.OutcomeID(chi.URLParam(r, "outcomeId")))
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, DecompositionRequestEnvelope{Request: decompositionRequestResponse(view)})
}

// submitAgentProposal is the callback a spawned agent answers on.
//
// A refused proposal is NOT an error here: the request records the refusal and
// keeps the draft, so this answers 200 with a rejected request rather than
// 4xx. The agent did its job; the daemon disagreed with the result, and the
// owner is the one who acts on that.
func (c *OutcomesController) submitAgentProposal(w http.ResponseWriter, r *http.Request) {
	if c.Svc == nil {
		apispec.NotImplemented(w, r, http.MethodPost, "/api/v1/decomposition-requests/{requestId}/proposal")
		return
	}
	var req SubmitAgentProposalRequest
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxAgentProposalBytes))
	if err != nil {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "bad_request", "INVALID_JSON", "Could not read the proposal body", nil)
		return
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		envelope.WriteAPIError(w, r, http.StatusBadRequest, "bad_request", "INVALID_JSON", "Invalid JSON body", nil)
		return
	}
	view, err := c.Svc.SubmitAgentProposal(
		r.Context(),
		domain.DecompositionRequestID(chi.URLParam(r, "requestId")),
		r.Header.Get(DecompositionCallbackTokenHeader),
		agentProposalInput(req),
		// The agent's own bytes are retained verbatim, so a refused draft is
		// exactly what it sent rather than a re-render of it.
		string(raw),
	)
	if err != nil {
		envelope.WriteError(w, r, err)
		return
	}
	envelope.WriteJSON(w, http.StatusOK, DecompositionRequestEnvelope{Request: decompositionRequestResponse(view)})
}

// maxAgentProposalBytes bounds what a spawned agent may post back. A runaway
// generation should be refused at the door rather than parsed.
const maxAgentProposalBytes = 1 << 20
