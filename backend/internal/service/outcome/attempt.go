// Act & Observe: governed execution of one scheduler-selected WorkUnit from an
// approved Plan. Admission is fail-closed before durable execution state:
// current Contract -> approved immutable Plan -> dependency-ready WorkUnit ->
// exact capability/binding validation -> exact provider/model readiness. After
// the Attempt row and custody fence exist, unknown launch outcomes remain
// ambiguous and reconcileable rather than being relabeled as clean failure.
package outcome

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apierr"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

type heartbeatSource interface {
	GetSession(ctx context.Context, id domain.SessionID) (domain.SessionRecord, bool, error)
}

// prelaunchObservationPayload is the typed durable bridge between the Attempt
// transaction and run-intent reconciliation. Error remains diagnostic prose;
// AdmissionFailure is the only field recovery may use as policy identity.
type prelaunchObservationPayload struct {
	Error            string                      `json:"error"`
	WorkUnitID       domain.WorkUnitID           `json:"workUnitId"`
	ProviderLaunched *bool                       `json:"providerLaunched"`
	AdmissionFailure *domain.RunAdmissionFailure `json:"admissionFailure"`
}

// AttemptManager is the controller-facing Act & Observe boundary.
type AttemptManager interface {
	StartAttempt(ctx context.Context, outcomeID domain.OutcomeID, in StartAttemptInput) (AttemptView, error)
	GetAttempt(ctx context.Context, outcomeID domain.OutcomeID, attemptID domain.AttemptID) (AttemptView, error)
	ListAttempts(ctx context.Context, outcomeID domain.OutcomeID) ([]AttemptView, error)
	GetSchedule(ctx context.Context, outcomeID domain.OutcomeID, planID domain.PlanRevisionID) (ScheduleView, error)
	CancelAttempt(ctx context.Context, outcomeID domain.OutcomeID, attemptID domain.AttemptID) (AttemptView, error)
	RecordObservation(ctx context.Context, outcomeID domain.OutcomeID, attemptID domain.AttemptID, in RecordObservationInput) (domain.AttemptObservation, error)
	RecoverAttempt(ctx context.Context, outcomeID domain.OutcomeID, attemptID domain.AttemptID, in RecoveryInput) (RecoveryView, error)
}

// StartAttemptInput names the exact approved WorkUnit this request intends to
// execute. Harness remains a temporary compatibility assertion only; it never
// chooses execution and may be removed from the HTTP surface after clients
// migrate to WorkUnitID.
type StartAttemptInput struct {
	PlanRevisionID domain.PlanRevisionID
	WorkUnitID     domain.WorkUnitID
	Harness        domain.AgentHarness
	RequestKey     string
}

// RecordObservationInput contains one bounded attempt observation.
type RecordObservationInput struct {
	Kind    string
	Payload string
}

// RecoveryAction identifies the governed response to an uncertain attempt.
type RecoveryAction string

const (
	// RecoveryActionContain preserves custody while the attempt is investigated.
	RecoveryActionContain RecoveryAction = "contain"
	// RecoveryActionReconcile requests reconciliation of ambiguous runtime state.
	RecoveryActionReconcile RecoveryAction = "reconcile"
	// RecoveryActionReplace requests a new attempt after reconciliation.
	RecoveryActionReplace RecoveryAction = "replace"
	// RecoveryActionAttention requests owner or operator attention.
	RecoveryActionAttention RecoveryAction = "attention"
)

// Valid reports whether the recovery action is supported.
func (a RecoveryAction) Valid() bool {
	switch a {
	case RecoveryActionContain, RecoveryActionReconcile, RecoveryActionReplace, RecoveryActionAttention:
		return true
	default:
		return false
	}
}

// RecoveryInput contains the owner's recovery decision and provider-stop proof.
type RecoveryInput struct {
	Action                 RecoveryAction
	ConfirmProviderStopped bool
}

// AttemptView is the service projection of one attempt and its evidence.
type AttemptView struct {
	Outcome      domain.Outcome
	Attempt      domain.Attempt
	Sessions     []domain.AttemptSessionRef
	Observations []domain.AttemptObservation
	Receipts     []domain.AttemptRecoveryReceipt
	Fence        *domain.AttemptFence
	// ProtocolProvenance carries the persisted protocol-negotiation episode
	// that answers for each session binding (ADR 0016, migration 0136),
	// keyed by AttemptSessionRefID. A binding with no recorded provenance
	// (a driver that cannot report it, or a record lost to the fail-soft
	// write) is simply absent from the map; absence is observability,
	// never an error.
	ProtocolProvenance map[domain.AttemptSessionRefID]domain.ChatProtocolProvenance
	LaunchPacket       *domain.WorkspaceBoundLaunchPacket
	Presentation       domain.AttemptPresentation
}

// RecoveryView is the service projection of a recovery receipt.
type RecoveryView struct {
	Attempt AttemptView
	Receipt *domain.AttemptRecoveryReceipt
}

const (
	// CodeAgentProfileNotReady indicates that the selected profile cannot run yet.
	CodeAgentProfileNotReady = "AGENT_PROFILE_NOT_READY"
	// CodeAgentBinaryNotFound indicates that the selected provider binary is absent.
	CodeAgentBinaryNotFound = "AGENT_BINARY_NOT_FOUND"
	// CodeAttemptPrelaunchFailed proves launch failed before the provider boundary.
	CodeAttemptPrelaunchFailed = "ATTEMPT_PRELAUNCH_FAILED"
	// CodeAttemptWorkspacePreparationFailed is the specialized workspace form.
	CodeAttemptWorkspacePreparationFailed = "ATTEMPT_WORKSPACE_PREPARATION_FAILED"
	// CodePlanNotApproved indicates that owner approval is missing.
	CodePlanNotApproved = "PLAN_NOT_APPROVED"
	// CodePlanBriefInvalidated indicates the approved plan no longer matches context.
	CodePlanBriefInvalidated = "PLAN_BRIEF_INVALIDATED"
	// CodeAttemptCapabilityUnauthorized indicates a grant exceeds current authority.
	CodeAttemptCapabilityUnauthorized = "ATTEMPT_CAPABILITY_UNAUTHORIZED"
	// CodeAttemptFenceHeld indicates another attempt owns the custody fence.
	CodeAttemptFenceHeld = "ATTEMPT_FENCE_HELD"
	// CodeNoRunnableWorkUnit reports that scheduling, not custody, is what
	// refused: every WorkUnit in the approved Plan is either already running
	// or still waiting on a dependency or required proof.
	CodeNoRunnableWorkUnit = "NO_RUNNABLE_WORK_UNIT"
	// CodeAttemptNotFound indicates that the requested attempt does not exist.
	CodeAttemptNotFound = "ATTEMPT_NOT_FOUND"
	// CodeAttemptLivenessUnproven indicates that runtime liveness is unknown.
	CodeAttemptLivenessUnproven = "ATTEMPT_LIVENESS_UNPROVEN"
	// CodeAttemptActivationUnresolved indicates that activation could not be proven.
	CodeAttemptActivationUnresolved = "ATTEMPT_ACTIVATION_UNRESOLVED"
	// CodeAttemptCustodyUnproven indicates that workspace custody is ambiguous.
	CodeAttemptCustodyUnproven = "ATTEMPT_CUSTODY_UNPROVEN"
	// CodeAttemptProviderStopFailed indicates provider termination failed.
	CodeAttemptProviderStopFailed = "ATTEMPT_PROVIDER_STOP_FAILED"
	// CodeAttemptExecutionPolicyUnsupported indicates the selected adapter cannot
	// prove enforcement of the approved WorkUnit capabilities.
	CodeAttemptExecutionPolicyUnsupported = "ATTEMPT_EXECUTION_POLICY_UNSUPPORTED"
	// CodeAttemptRequestKeyConflict indicates idempotency-key reuse for different
	// canonical Outcome/Plan/WorkUnit semantics.
	CodeAttemptRequestKeyConflict = "ATTEMPT_REQUEST_KEY_CONFLICT"
	// CodeAttemptStartUnresolved indicates that attempt activation is ambiguous.
	CodeAttemptStartUnresolved = "ATTEMPT_START_UNRESOLVED"
)

var _ AttemptManager = (*Service)(nil)

// StartAttempt admits one exact approved WorkUnit for execution.
func (s *Service) StartAttempt(ctx context.Context, outcomeID domain.OutcomeID, in StartAttemptInput) (AttemptView, error) {
	if s.spawner == nil || s.heartbeats == nil {
		return AttemptView{}, apierr.Internal("ATTEMPT_EXECUTION_UNWIRED", "Attempt execution is not wired in this environment")
	}
	if strings.TrimSpace(in.RequestKey) == "" {
		return AttemptView{}, apierr.Invalid("REQUEST_KEY_REQUIRED", "Provide an idempotency key for this start request", nil)
	}
	if existing, ok, err := s.store.FindAttemptByIdempotencyKey(ctx, in.RequestKey); err != nil {
		return AttemptView{}, err
	} else if ok {
		if existing.OutcomeID != outcomeID || existing.PlanRevisionID != in.PlanRevisionID || (!in.WorkUnitID.IsZero() && existing.WorkUnitID != in.WorkUnitID) {
			return AttemptView{}, apierr.Conflict(CodeAttemptRequestKeyConflict,
				"That idempotency key is already bound to different Outcome/Plan/WorkUnit semantics",
				map[string]any{"requestKey": strings.TrimSpace(in.RequestKey), "attemptId": existing.ID, "outcomeId": existing.OutcomeID, "planId": existing.PlanRevisionID, "workUnitId": existing.WorkUnitID})
		}
		return s.GetAttempt(ctx, existing.OutcomeID, existing.ID)
	}

	outcomeRecord, ok, err := s.store.GetOutcome(ctx, outcomeID)
	if err != nil {
		return AttemptView{}, err
	}
	if !ok {
		return AttemptView{}, apierr.NotFound("OUTCOME_NOT_FOUND", "That Outcome does not exist")
	}
	gate, err := s.startGateFor(ctx, outcomeRecord)
	if err != nil {
		return AttemptView{}, err
	}
	if !gate.Clear() {
		return AttemptView{}, blockedError(outcomeID, gate)
	}
	// A pause prevents subsequent admission. Checking it here, before any
	// durable row is written, is what makes "paused" mean the work stops
	// rather than the button stops being offered.
	runIntentGeneration, err := s.refuseAdmissionAgainstRunIntent(ctx, outcomeID)
	if err != nil {
		return AttemptView{}, err
	}
	// So does a standing correction naming the Plan or Contract. Admission has
	// to enforce it too: continuation admits through here, and the direct
	// per-Attempt Start would otherwise be the way around a refusal the
	// Mission shows the owner.
	if err := s.refuseExecutionAgainstCorrection(ctx, outcomeID); err != nil {
		return AttemptView{}, err
	}

	plan, found, err := s.store.GetPlanRevision(ctx, outcomeID, in.PlanRevisionID)
	if err != nil {
		return AttemptView{}, err
	}
	if !found {
		return AttemptView{}, apierr.NotFound("PLAN_NOT_FOUND", "That plan does not exist")
	}
	if plan.Status != domain.PlanStatusApproved {
		return AttemptView{}, apierr.Conflict(CodePlanNotApproved, "Authorize this plan before starting an Attempt", map[string]any{"planId": plan.ID, "status": plan.Status})
	}
	if !plan.BindsCurrentContract(outcomeRecord.CurrentRevisionNumber) {
		return AttemptView{}, apierr.Conflict(CodePlanBriefInvalidated,
			fmt.Sprintf("Plan binds Contract revision %s; the Outcome is at %s — propose and approve a fresh Plan", formatI64(plan.ContractRevisionNumber), formatI64(outcomeRecord.CurrentRevisionNumber)),
			map[string]any{"outcomeId": outcomeID, "planId": plan.ID, "planRevisionBinding": plan.ContractRevisionNumber, "currentRevision": outcomeRecord.CurrentRevisionNumber})
	}

	revision, err := s.currentRevision(ctx, outcomeRecord)
	if err != nil {
		return AttemptView{}, err
	}
	if err := plan.ValidateForApproval(revision); err != nil {
		return AttemptView{}, apierr.Conflict(CodePlanBriefInvalidated, "The approved Plan no longer satisfies its Contract binding", map[string]any{"detail": err.Error()})
	}
	if err := s.authorizeAttemptCapabilities(revision, plan); err != nil {
		return AttemptView{}, err
	}

	unit, err := s.selectWorkUnitForAttempt(ctx, outcomeID, plan, in.WorkUnitID)
	if err != nil {
		return AttemptView{}, err
	}
	binding, err := unit.ExecutionBindingForNewWork()
	if err != nil {
		return AttemptView{}, providerUnboundError(outcomeID)
	}
	if requested := domain.AgentHarness(strings.TrimSpace(string(in.Harness))); requested != "" && requested != binding.Provider {
		return AttemptView{}, apierr.Conflict(CodeAttemptProviderMismatch,
			"The requested provider does not match the exact provider authorized for this WorkUnit",
			map[string]any{"planId": plan.ID, "workUnitId": unit.ID, "authorizedProvider": binding.Provider, "requestedProvider": requested})
	}

	recomputed, err := domain.ComputePlanRunBriefCoreDigest(revision, plan.WorkUnits, plan.Grants)
	if err != nil {
		return AttemptView{}, err
	}
	if recomputed != plan.RunBriefCoreDigest {
		return AttemptView{}, apierr.Conflict(CodePlanBriefInvalidated,
			"The frozen RunBrief no longer matches the Contract and Plan — propose and approve a fresh Plan",
			map[string]any{"outcomeId": outcomeID, "planId": plan.ID})
	}
	policy, err := domain.BuildAttemptExecutionPolicy(outcomeID, plan, unit, recomputed)
	if err != nil {
		return AttemptView{}, apierr.Conflict(CodeAttemptCapabilityUnauthorized, "The approved WorkUnit capability packet is invalid", map[string]any{"detail": err.Error(), "workUnitId": unit.ID})
	}
	prompt, err := renderRunBriefPrompt(revision, unit)
	if err != nil {
		return AttemptView{}, apierr.Conflict(CodePlanBriefInvalidated,
			"The frozen Contract and WorkUnit could not be compiled into an exact RunBrief",
			map[string]any{"detail": err.Error(), "planId": plan.ID, "workUnitId": unit.ID})
	}

	projectID, found, err := s.store.GetOutcomeProjectID(ctx, outcomeID)
	if err != nil {
		return AttemptView{}, err
	}
	if !found {
		return AttemptView{}, apierr.NotFound("PROJECT_NOT_FOUND", "Register that Project before starting Attempts")
	}
	if s.admission == nil {
		return AttemptView{}, apierr.Internal("ADMISSION_STORE_UNWIRED", "Admission persistence is unavailable in this environment")
	}
	approvedSpec, found, err := s.admission.GetApprovedExecutableSpec(ctx, plan.ID, unit.ID)
	if err != nil {
		return AttemptView{}, err
	}
	if !found {
		return AttemptView{}, apierr.Conflict("ATTEMPT_ADMISSION_MISSING", "The approved Plan has no executable admission spec", nil)
	}
	// Validate supplied material before replaying the frozen admission identity so
	// document-specific refusals remain actionable and no changed bytes launch.
	documents, hasDocuments, err := s.approvedDocumentsForAdmission(ctx, outcomeID)
	if err != nil {
		return AttemptView{}, err
	}
	staged, err := s.EvaluateAdmissionStage(ctx, ports.AdmissionStageInput{Stage: ports.AdmissionStageStart, ProjectID: projectID, Outcome: &outcomeRecord, Contract: &revision, Plan: &plan})
	if err != nil {
		return AttemptView{}, err
	}
	currentVerdict := (admissionEvaluator{now: s.clock, policy: s.AdmissionPolicy}).revalidate(approvedSpec, staged.Verdict)
	if currentVerdict.Status != domain.AdmissionAdmitted {
		if err := s.admission.AppendAdmissionEvaluation(ctx, currentVerdict); err != nil {
			return AttemptView{}, err
		}
		return AttemptView{}, apierr.Conflict("ATTEMPT_ADMISSION_STALE", "Execution admission changed after approval; replan and approve again", map[string]any{"reasons": currentVerdict.Reasons})
	}
	// A successor may not be admitted until its predecessors' exact results are
	// retained, complete and frozen. Resolving here, before the fence is taken,
	// means a blocked successor never holds custody it cannot use — and the
	// versions resolved now are the ones launch must materialize.
	inputs, err := s.admittedInputsFor(ctx, plan, unit)
	if err != nil {
		return AttemptView{}, err
	}
	// A supplied-document Outcome stages its approved snapshot the same way,
	// at the same seam, under the same refusal: unreviewed or edited material
	// never reaches a provider.
	var documentInputs *ports.AttemptDocumentInputs
	if hasDocuments {
		documentInputs = &ports.AttemptDocumentInputs{
			ContextID: documents.ID, Revision: documents.Revision, Digest: documents.Digest,
		}
	}

	now := s.clock()
	attempt, err := s.store.CreateAttemptWithFence(ctx, ports.AttemptAdmission{
		OutcomeID: outcomeID, PlanRevisionID: plan.ID, WorkUnitID: unit.ID,
		ContractRevisionNumber: plan.ContractRevisionNumber,
		RunIntentGeneration:    runIntentGeneration,
		RequestKey:             strings.TrimSpace(in.RequestKey), FenceSubject: domain.FenceSubjectForProject(projectID), At: now,
	})
	if err != nil {
		var replayConflict *ports.AttemptReplayConflictError
		if errors.As(err, &replayConflict) {
			return AttemptView{}, apierr.Conflict(CodeAttemptRequestKeyConflict,
				"That idempotency key is already bound to different Outcome/Plan/WorkUnit semantics",
				map[string]any{"requestKey": strings.TrimSpace(in.RequestKey), "attemptId": replayConflict.Attempt.ID, "outcomeId": replayConflict.Attempt.OutcomeID, "planId": replayConflict.Attempt.PlanRevisionID, "workUnitId": replayConflict.Attempt.WorkUnitID})
		}
		var replay *ports.AttemptReplayError
		if errors.As(err, &replay) {
			return s.GetAttempt(ctx, replay.Attempt.OutcomeID, replay.Attempt.ID)
		}
		var held *ports.AttemptFenceHeldError
		if errors.As(err, &held) {
			return AttemptView{}, apierr.Conflict(CodeAttemptFenceHeld,
				"Another Attempt holds custody of this Project worktree — reconcile it first",
				map[string]any{"subject": held.Subject, "holder": held.Holder, "attemptedFor": held.OutcomeID})
		}
		var runConflict *ports.AttemptRunIntentConflictError
		if errors.As(err, &runConflict) {
			if runConflict.Desired == domain.RunIntentPaused || runConflict.Desired == domain.RunIntentCancelled {
				return AttemptView{}, apierr.Conflict(CodeRunActionUnavailable,
					"The Outcome was paused or cancelled before this Attempt could be admitted", nil)
			}
			return AttemptView{}, apierr.Conflict(CodeRunIntentStale,
				"The Outcome's run authorization changed before this Attempt could be admitted", nil)
		}
		return AttemptView{}, err
	}

	fence, found, err := s.store.OpenFenceForSubject(ctx, domain.FenceSubjectForProject(projectID))
	if err != nil || !found || fence.AttemptID != attempt.ID {
		if err == nil {
			err = errors.New("attempt fence is missing after admission")
		}
		return AttemptView{}, s.admitPrelaunchFailure(ctx, outcomeID, unit, attempt, err)
	}
	spawned, err := s.spawner.Spawn(ctx, ports.AttemptSpawnRequest{
		ProjectID: projectID, Harness: binding.Provider, ModelSelection: binding.ModelSelection, Model: binding.Model,
		ExecutionPolicy: &policy,
		Prompt:          prompt, DisplayName: fmt.Sprintf("%s · %s · attempt %d", outcomeRecord.Title, unit.Title, attempt.Number),
		Inputs: inputs, Documents: documentInputs,
		BeforeProviderLaunch: func(ctx context.Context, session domain.SessionRecord, bound domain.AttemptExecutionPolicy) error {
			readinessReceipt, readinessErr := s.probeReadiness(ctx, projectID, binding, &bound)
			if readinessErr != nil {
				return readinessErr
			}
			versions := inputArtifactVersions(inputs)
			packet := domain.WorkspaceBoundLaunchPacket{Spec: approvedSpec, SpecDigest: approvedSpec.Digest, AttemptID: attempt.ID, FenceID: fence.ID, SessionID: string(session.ID), CanonicalWorkspaceRoot: bound.WorkspaceRoot, InputArtifactVersions: versions, CurrentReadinessReceipts: []domain.ReadinessReceipt{readinessReceipt}, Policy: bound}
			policyDigest, digestErr := bound.Digest()
			if digestErr != nil {
				return digestErr
			}
			packet.LaunchFacts = domain.LaunchFacts{AttemptID: attempt.ID, FenceID: fence.ID, SessionID: string(session.ID), CanonicalWorkspaceRoot: bound.WorkspaceRoot, SpecDigest: approvedSpec.Digest, ReadinessReceipts: packet.CurrentReadinessReceipts, DocumentContextID: approvedSpec.DocumentContextID, DocumentContextRevision: approvedSpec.DocumentContextRevision, DocumentContextDigest: approvedSpec.DocumentContextDigest, InputArtifactVersions: versions, PolicyDigest: policyDigest}
			packet.LaunchFactsDigest, digestErr = packet.LaunchFacts.Digest()
			if digestErr != nil {
				return digestErr
			}
			packet.Digest, digestErr = packet.ComputedDigest()
			if digestErr != nil {
				return digestErr
			}
			return s.admission.PersistWorkspaceBoundLaunchPacket(ctx, packet)
		},
	})
	if err != nil {
		// Input provisioning happens before any provider process exists, so
		// this failure is known rather than ambiguous. It is recorded and the
		// Attempt is ended, leaving any partially provisioned workspace
		// attributable instead of holding custody for a run that never began.
		var prelaunch *ports.AttemptPrelaunchError
		if errors.Is(err, ports.ErrAttemptInputProvisioning) || errors.Is(err, ports.ErrAttemptWorkspacePreparation) || errors.As(err, &prelaunch) {
			return AttemptView{}, s.admitPrelaunchFailure(ctx, outcomeID, unit, attempt, err)
		}
		return AttemptView{}, s.admitUnresolved(ctx, attempt.ID, domain.ObservationAdmissionAmbiguous, err)
	}
	if binding.ModelSelection == domain.ExecutionBindingModelExplicit && strings.TrimSpace(spawned.EffectiveModel) != "" && strings.TrimSpace(spawned.EffectiveModel) != binding.Model {
		return AttemptView{}, s.admitUnresolved(ctx, attempt.ID, domain.ObservationActivationAmbiguous,
			fmt.Errorf("provider reported effective model %q but Plan authorized %q", spawned.EffectiveModel, binding.Model))
	}

	session := spawned.Session
	if spawned.ExecutionPolicy == nil {
		return AttemptView{}, s.admitUnresolved(ctx, attempt.ID, domain.ObservationActivationAmbiguous,
			errors.New("governed spawn did not return its workspace-bound execution policy"))
	}
	policy = *spawned.ExecutionPolicy
	if err := policy.ValidateWorkspaceRoot(session.Metadata.WorkspacePath); err != nil {
		return AttemptView{}, s.admitUnresolved(ctx, attempt.ID, domain.ObservationActivationAmbiguous,
			fmt.Errorf("governed spawn workspace evidence mismatch: %w", err))
	}
	mode := session.Mode
	policyDigest, err := policy.Digest()
	if err != nil {
		return AttemptView{}, s.admitUnresolved(ctx, attempt.ID, domain.ObservationActivationAmbiguous, fmt.Errorf("execution policy digest failed: %w", err))
	}
	compiled := computeCompiledBriefDigest(binding, mode, recomputed, policyDigest, inputs)
	snapshot, err := json.Marshal(map[string]any{
		"snapshotVersion":        domain.AdmissionSnapshotVersion,
		"harness":                string(binding.Provider),
		"modelSelection":         string(binding.ModelSelection),
		"requestedModel":         binding.Model,
		"effectiveModel":         strings.TrimSpace(spawned.EffectiveModel),
		"workUnitId":             string(unit.ID),
		"mode":                   string(mode),
		"runBriefCoreDigest":     recomputed,
		"runBriefCompiledDigest": compiled,
		"executionPolicy":        policy,
		"executionPolicyDigest":  policyDigest,
		"sessionId":              session.ID,
		"completionBoundary":     spawned.CompletionBoundary,
		"requestedAt":            now,
		// The exact predecessor artifacts this Attempt consumed. Recording
		// them is what lets a replay or an audit say which bytes the successor
		// was actually built on, rather than re-resolving "the latest".
		"inputArtifactVersions": inputArtifactVersions(inputs),
		"documentContext":       documentInputs,
	})
	if err != nil {
		return AttemptView{}, s.admitUnresolved(ctx, attempt.ID, domain.ObservationActivationAmbiguous, fmt.Errorf("admission snapshot failed: %w", err))
	}
	if _, err := s.store.BindAttemptSession(ctx, domain.AttemptSessionRef{
		AttemptID: attempt.ID, SessionID: string(session.ID), Harness: binding.Provider, Mode: mode,
		RunBriefCoreDigest: recomputed, RunBriefCompiledDigest: compiled, AdmissionSnapshot: string(snapshot), BoundAt: s.clock(),
	}); err != nil {
		return AttemptView{}, s.admitUnresolved(ctx, attempt.ID, domain.ObservationActivationAmbiguous, fmt.Errorf("session binding failed: %w", err))
	}
	rows, err := s.store.TransitionAttemptStatus(ctx, outcomeID, attempt.ID, domain.AttemptQueued, domain.AttemptRunning, s.clock())
	if err != nil || rows != 1 {
		return AttemptView{}, s.admitUnresolved(ctx, attempt.ID, domain.ObservationActivationAmbiguous, fmt.Errorf("activation not recorded (rows=%d): %w", rows, err))
	}
	return s.GetAttempt(ctx, outcomeID, attempt.ID)
}

func (s *Service) admitUnresolved(ctx context.Context, attemptID domain.AttemptID, kind string, cause error) error {
	detail := ""
	if cause != nil {
		detail = cause.Error()
	}
	payload, _ := json.Marshal(map[string]any{"error": detail, "unresolved": true})
	var errs []error
	if _, err := s.store.AppendAttemptObservation(ctx, attemptID, kind, string(payload), s.clock()); err != nil {
		errs = append(errs, fmt.Errorf("record ambiguous start (%s) for %s: %w", kind, attemptID, err))
	}
	if err := s.store.CreateRecoveryReceipt(ctx, domain.AttemptRecoveryReceipt{
		ID: "rcpt-" + uuid.NewString(), AttemptID: attemptID, Resolution: domain.RecoveryNeedsAttention,
		Detail: string(payload), CreatedAt: s.clock(),
	}); err != nil {
		errs = append(errs, fmt.Errorf("record ambiguous activation receipt for %s: %w", attemptID, err))
	}
	code, headline := CodeAttemptActivationUnresolved, "The activation outcome is unknown"
	if kind == domain.ObservationAdmissionAmbiguous {
		code, headline = CodeAttemptStartUnresolved, "The start outcome is unknown"
	}
	unresolved := apierr.Conflict(code, headline+" — the Attempt stays unconfirmed until you reconcile it", map[string]any{"attemptId": attemptID})
	if len(errs) > 0 {
		return errors.Join(append(errs, unresolved)...)
	}
	return unresolved
}

func (s *Service) probeReadiness(ctx context.Context, projectID domain.ProjectID, binding domain.ExecutionBinding, policy *domain.AttemptExecutionPolicy) (domain.ReadinessReceipt, error) {
	readiness, err := s.spawner.ProfileReadiness(ctx, projectID, binding, policy)
	if err != nil {
		var unsupported *ports.ExecutionPolicyUnsupportedError
		if errors.As(err, &unsupported) {
			return domain.ReadinessReceipt{}, apierr.Conflict(CodeAttemptExecutionPolicyUnsupported, "The selected provider cannot enforce this approved WorkUnit policy", map[string]any{"harness": binding.Provider, "capability": unsupported.Capability, "detail": unsupported.Detail})
		}
		if errors.Is(err, ports.ErrAgentBinaryNotFound) {
			return domain.ReadinessReceipt{}, apierr.Conflict(CodeAgentBinaryNotFound, "The authorized agent binary is not installed on this machine", map[string]any{"harness": binding.Provider})
		}
		return domain.ReadinessReceipt{}, err
	}
	if !readiness.Ready {
		return domain.ReadinessReceipt{}, apierr.Conflict(CodeAgentProfileNotReady, "The authorized agent profile/model is not ready to launch", map[string]any{"harness": binding.Provider, "modelSelection": binding.ModelSelection, "model": binding.Model, "detail": readiness.Detail})
	}
	digest := domain.DigestSHA256([]byte(string(binding.Provider) + "\x00" + readiness.Detail))
	return domain.ReadinessReceipt{Producer: "provider_profile", Version: "w1.1-v1", ReceiptID: string(digest), Digest: string(digest)}, nil
}

// CancelAttempt records a governed cancellation request for an attempt.
func (s *Service) CancelAttempt(ctx context.Context, outcomeID domain.OutcomeID, attemptID domain.AttemptID) (AttemptView, error) {
	attempt, _, err := s.requireAttempt(ctx, outcomeID, attemptID)
	if err != nil {
		return AttemptView{}, err
	}
	switch attempt.Status {
	case domain.AttemptQueued, domain.AttemptRunning, domain.AttemptPaused:
	default:
		return AttemptView{}, apierr.Conflict("ATTEMPT_ALREADY_ENDED", fmt.Sprintf("Attempt already ended as %s", attempt.Status), map[string]any{"status": attempt.Status})
	}
	ref, bound, err := s.store.LatestAttemptSessionRef(ctx, attemptID)
	if err != nil {
		return AttemptView{}, err
	}
	projectID, _, err := s.store.GetOutcomeProjectID(ctx, outcomeID)
	if err != nil {
		return AttemptView{}, err
	}
	if !bound {
		return AttemptView{}, apierr.Conflict(CodeAttemptStartUnresolved,
			"This start's outcome is unknown — reconcile with a stop confirmation instead of cancelling", map[string]any{"attemptId": attemptID})
	}
	termRes, termErr := s.spawner.Terminate(ctx, projectID, ref.SessionID)
	if termErr == nil && !termRes.ProviderStopped {
		termErr = fmt.Errorf("%w: adapter could not prove the session stopped", ports.ErrProviderStopUnproven)
	}
	if termErr != nil {
		payload := mustJSON(map[string]any{"error": termErr.Error(), "sessionId": ref.SessionID})
		var errs []error
		if _, obsErr := s.store.AppendAttemptObservation(ctx, attemptID, domain.ObservationProviderStopFailed, payload, s.clock()); obsErr != nil {
			errs = append(errs, fmt.Errorf("record stop-failure observation for %s: %w", attemptID, obsErr))
		}
		if rcptErr := s.store.CreateRecoveryReceipt(ctx, domain.AttemptRecoveryReceipt{
			ID: "rcpt-" + uuid.NewString(), AttemptID: attemptID, Resolution: domain.RecoveryNeedsAttention, Detail: payload, CreatedAt: s.clock(),
		}); rcptErr != nil {
			errs = append(errs, fmt.Errorf("record stop-failure receipt for %s: %w", attemptID, rcptErr))
		}
		refused := apierr.Conflict(CodeAttemptProviderStopFailed,
			"The provider session could not be stopped — cancel was NOT recorded; stop it and retry", map[string]any{"attemptId": attemptID, "sessionId": ref.SessionID})
		if len(errs) > 0 {
			return AttemptView{}, errors.Join(append(errs, refused)...)
		}
		return AttemptView{}, refused
	}
	rows, err := s.store.TransitionAttemptStatus(ctx, outcomeID, attemptID, attempt.Status, domain.AttemptCancelled, s.clock())
	if err != nil {
		return AttemptView{}, err
	}
	if rows == 0 {
		return AttemptView{}, apierr.Conflict("ATTEMPT_STATUS_MOVED", "The Attempt changed state concurrently; reload and retry", nil)
	}
	payload, _ := json.Marshal(map[string]any{"previousStatus": attempt.Status, "providerStopped": termRes.ProviderStopped, "workspaceFreed": termRes.WorkspaceFreed})
	if _, err := s.store.AppendAttemptObservation(ctx, attemptID, domain.ObservationOwnerCancel, string(payload), s.clock()); err != nil {
		return AttemptView{}, err
	}
	return s.GetAttempt(ctx, outcomeID, attemptID)
}

// GetAttempt returns the durable attempt projection and related facts.
func (s *Service) GetAttempt(ctx context.Context, outcomeID domain.OutcomeID, attemptID domain.AttemptID) (AttemptView, error) {
	attempt, outcomeRecord, err := s.requireAttempt(ctx, outcomeID, attemptID)
	if err != nil {
		return AttemptView{}, err
	}
	return s.readModel(ctx, outcomeRecord, attempt)
}

// ListAttempts returns durable attempt projections for an Outcome.
func (s *Service) ListAttempts(ctx context.Context, outcomeID domain.OutcomeID) ([]AttemptView, error) {
	outcomeRecord, ok, err := s.store.GetOutcome(ctx, outcomeID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apierr.NotFound("OUTCOME_NOT_FOUND", "That Outcome does not exist")
	}
	attempts, err := s.store.ListAttempts(ctx, outcomeID)
	if err != nil {
		return nil, err
	}
	views := make([]AttemptView, 0, len(attempts))
	for _, attempt := range attempts {
		view, err := s.readModel(ctx, outcomeRecord, attempt)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

// RecordObservation appends a bounded observation to an attempt.
func (s *Service) RecordObservation(ctx context.Context, outcomeID domain.OutcomeID, attemptID domain.AttemptID, in RecordObservationInput) (domain.AttemptObservation, error) {
	if _, _, err := s.requireAttempt(ctx, outcomeID, attemptID); err != nil {
		return domain.AttemptObservation{}, err
	}
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		return domain.AttemptObservation{}, apierr.Invalid("OBSERVATION_KIND_REQUIRED", "Name what was observed", nil)
	}
	if systemOwnedPrelaunchObservationKind(kind) {
		return domain.AttemptObservation{}, apierr.Invalid("OBSERVATION_KIND_RESERVED", "That observation kind is written only by Attempt admission", nil)
	}
	payload := in.Payload
	if payload == "" {
		payload = "{}"
	} else if !json.Valid([]byte(payload)) {
		return domain.AttemptObservation{}, apierr.Invalid("OBSERVATION_PAYLOAD_INVALID", "Payload must be valid JSON", nil)
	}
	return s.store.AppendAttemptObservation(ctx, attemptID, kind, payload, s.clock())
}

func (s *Service) requireAttempt(ctx context.Context, outcomeID domain.OutcomeID, attemptID domain.AttemptID) (domain.Attempt, domain.Outcome, error) {
	outcomeRecord, ok, err := s.store.GetOutcome(ctx, outcomeID)
	if err != nil {
		return domain.Attempt{}, domain.Outcome{}, err
	}
	if !ok {
		return domain.Attempt{}, domain.Outcome{}, apierr.NotFound("OUTCOME_NOT_FOUND", "That Outcome does not exist")
	}
	attempt, found, err := s.store.GetAttempt(ctx, outcomeID, attemptID)
	if err != nil {
		return domain.Attempt{}, domain.Outcome{}, err
	}
	if !found {
		return domain.Attempt{}, domain.Outcome{}, apierr.NotFound(CodeAttemptNotFound, "That Attempt does not exist for this Outcome")
	}
	return attempt, outcomeRecord, nil
}

func (s *Service) readModel(ctx context.Context, outcomeRecord domain.Outcome, attempt domain.Attempt) (AttemptView, error) {
	sessions, err := s.store.ListAttemptSessionRefs(ctx, attempt.ID)
	if err != nil {
		return AttemptView{}, err
	}
	observations, err := s.store.ListAttemptObservations(ctx, attempt.ID)
	if err != nil {
		return AttemptView{}, err
	}
	receipts, err := s.store.ListRecoveryReceipts(ctx, attempt.ID)
	if err != nil {
		return AttemptView{}, err
	}
	facts := domain.SessionHeartbeatFacts{}
	if latest, ok, err := s.store.LatestAttemptSessionRef(ctx, attempt.ID); err != nil {
		return AttemptView{}, err
	} else if ok && s.heartbeats != nil {
		if rec, present, err := s.heartbeats.GetSession(ctx, domain.SessionID(latest.SessionID)); err != nil {
			return AttemptView{}, err
		} else if present {
			facts = domain.SessionHeartbeatFacts{Present: true, ActivityState: rec.Activity.State, FirstSignalAt: rec.FirstSignalAt, LastActivityAt: rec.Activity.LastActivityAt, IsTerminated: rec.IsTerminated}
		}
	}
	unresolvedAdmission := false
	unresolvedCheckTermination := false
	for _, obs := range observations {
		if obs.Kind == domain.ObservationAdmissionAmbiguous || obs.Kind == domain.ObservationActivationAmbiguous {
			unresolvedAdmission = true
		}
		if obs.Kind == domain.ObservationGovernedCheckTerminationUnknown {
			unresolvedCheckTermination = true
		}
	}
	subjectProject, ok, err := s.store.GetOutcomeProjectID(ctx, outcomeRecord.ID)
	if err != nil {
		return AttemptView{}, err
	}
	var fence *domain.AttemptFence
	if ok {
		if open, held, err := s.store.OpenFenceForSubject(ctx, domain.FenceSubjectForProject(subjectProject)); err != nil {
			return AttemptView{}, err
		} else if held && open.AttemptID == attempt.ID {
			fence = &open
		}
	}
	// Provenance rides the same read as the rest of the attempt's evidence so
	// review surfaces never assemble a partial record from separate calls.
	// Lookup errors propagate like every sibling read above: downgrading a
	// storage failure to "no provenance recorded" would be a silent fallback,
	// and genuine absence (driver cannot report, fail-soft write lost) is
	// already represented as a missing map entry.
	provenance := make(map[domain.AttemptSessionRefID]domain.ChatProtocolProvenance, len(sessions))
	for _, ref := range sessions {
		rec, found, err := s.store.ChatProtocolProvenanceForBinding(ctx, ref.SessionID, ref.BoundAt)
		if err != nil {
			return AttemptView{}, fmt.Errorf("read protocol provenance for attempt %s session %s: %w", attempt.ID, ref.SessionID, err)
		}
		if found {
			provenance[ref.ID] = rec
		}
	}
	var launchPacket *domain.WorkspaceBoundLaunchPacket
	if s.admission != nil {
		if packet, found, err := s.admission.GetWorkspaceBoundLaunchPacket(ctx, attempt.ID); err != nil {
			return AttemptView{}, err
		} else if found {
			launchPacket = &packet
		}
	}
	return AttemptView{
		Outcome: outcomeRecord, Attempt: attempt, Sessions: sessions, Observations: observations, Receipts: receipts, Fence: fence, LaunchPacket: launchPacket,
		ProtocolProvenance: provenance,
		Presentation:       domain.DeriveAttemptPresentation(attempt.Status, facts, unresolvedAdmission, unresolvedCheckTermination, domain.LivenessPolicy{Now: s.clock(), StaleHeartbeatAfter: s.staleHeartbeat}),
	}, nil
}

func (s *Service) authorizeAttemptCapabilities(revision domain.ContractRevision, plan domain.PlanRevision) error {
	for _, unit := range plan.WorkUnits {
		if err := validateWorkUnitWithinContractCeiling(revision, unit); err != nil {
			return apierr.New(apierr.KindConflict, CodeAttemptCapabilityUnauthorized, err.Error(), map[string]any{"workUnitId": unit.ID})
		}
	}
	if err := domain.ValidateExactPlanCapabilityGrants(plan.Grants, plan.WorkUnits); err != nil {
		return apierr.Invalid(CodeAttemptCapabilityUnauthorized, err.Error(), nil)
	}
	authoritative := s.authoritativeCapabilities()
	if err := domain.GrantsFailClosed(plan.Grants, authoritative); err != nil {
		var names []string
		for _, grant := range plan.Grants {
			names = append(names, grant.Name)
		}
		return apierr.New(apierr.KindConflict, CodeAttemptCapabilityUnauthorized, err.Error(), map[string]any{"granted": names, "authoritative": authoritative})
	}
	return nil
}

// computeCompiledBriefDigest identifies exactly what this Attempt was launched
// with. Input artifact versions are part of that identity: the same WorkUnit
// run against different predecessor output is different work, and a replay
// fingerprint that ignored the inputs would call the two the same.
func computeCompiledBriefDigest(binding domain.ExecutionBinding, mode domain.SessionMode, core, policyDigest string, inputs []ports.AttemptInputRef) string {
	sum := sha256.Sum256([]byte("v3|" + string(binding.Provider) + "|" + string(binding.ModelSelection) + "|" + binding.Model +
		"|" + string(mode) + "|" + core + "|" + policyDigest + "|" + strings.Join(inputArtifactVersions(inputs), ",")))
	return hex.EncodeToString(sum[:])
}

// admitPrelaunchFailure records a known pre-launch failure and ends the
// Attempt so its custody is released for a deliberate retry.
//
// Nothing ran, so holding the worktree fence would block the owner without
// protecting anything. The workspace itself is left alone by the session
// manager when it holds partial content, so failed custody stays inspectable.
func (s *Service) admitPrelaunchFailure(ctx context.Context, outcomeID domain.OutcomeID, unit domain.WorkUnit, attempt domain.Attempt, cause error) error {
	kind := domain.ObservationAdmissionFailed
	if errors.Is(cause, ports.ErrAttemptInputProvisioning) {
		kind = domain.ObservationInputProvisioningFailed
	}
	refused := prelaunchRefusal(unit, attempt.ID, cause)
	detailJSON := mustJSON(refused.Details)
	if detailJSON == "" || detailJSON == "null" {
		detailJSON = "{}"
	}
	failedAt := s.clock()
	failure := domain.RunAdmissionFailure{
		Code: refused.Code, Message: refused.Message, DetailJSON: detailJSON,
		WorkUnitID: unit.ID, OccurredAt: failedAt,
	}
	providerLaunched := false
	payload := mustJSON(prelaunchObservationPayload{
		Error: cause.Error(), WorkUnitID: unit.ID, ProviderLaunched: &providerLaunched,
		AdmissionFailure: &failure,
	})

	if _, err := s.store.FailAttemptBeforeLaunch(ctx, ports.AttemptPrelaunchFailure{
		OutcomeID: outcomeID, AttemptID: attempt.ID, ObservationKind: kind,
		ObservationPayload: payload, ReleaseReason: "provider_not_launched", At: failedAt,
	}); err != nil {
		return errors.Join(fmt.Errorf("record prelaunch failure for %s: %w", attempt.ID, err), refused)
	}
	return refused
}

func prelaunchRefusal(unit domain.WorkUnit, attemptID domain.AttemptID, cause error) *apierr.Error {
	var existing *apierr.Error
	if errors.As(cause, &existing) {
		return existing
	}
	if errors.Is(cause, ports.ErrAttemptInputProvisioning) {
		return materializationFailed(unit, attemptID, cause)
	}
	if errors.Is(cause, ports.ErrAttemptWorkspacePreparation) {
		return apierr.New(apierr.KindConflict, CodeAttemptWorkspacePreparationFailed, "The workspace could not be prepared; no provider was started", map[string]any{"attemptId": string(attemptID), "detail": cause.Error()})
	}
	if errors.Is(cause, ports.ErrAgentBinaryNotFound) {
		return apierr.New(apierr.KindConflict, CodeAgentBinaryNotFound, "The authorized agent binary is not installed on this machine; no provider was started", map[string]any{"attemptId": string(attemptID), "detail": cause.Error()})
	}
	detail := map[string]any{"attemptId": string(attemptID), "detail": cause.Error()}
	var prelaunch *ports.AttemptPrelaunchError
	if errors.As(cause, &prelaunch) {
		detail["stage"] = prelaunch.Stage
	}
	return apierr.New(apierr.KindConflict, CodeAttemptPrelaunchFailed, "The Attempt could not launch; no provider was started", detail)
}

func systemOwnedPrelaunchObservationKind(kind string) bool {
	return kind == domain.ObservationAdmissionFailed || kind == domain.ObservationInputProvisioningFailed
}

func renderRunBriefPrompt(revision domain.ContractRevision, unit domain.WorkUnit) (string, error) {
	assigned := make(map[domain.CriterionID]struct{}, len(unit.CriterionIDs))
	for _, id := range unit.CriterionIDs {
		assigned[id] = struct{}{}
	}
	criteria := make([]domain.ContractCriterion, 0, len(unit.CriterionIDs))
	for _, criterion := range revision.Criteria {
		if _, ok := assigned[criterion.ID]; ok {
			criteria = append(criteria, criterion)
			delete(assigned, criterion.ID)
		}
	}
	if len(assigned) != 0 {
		missing := make([]string, 0, len(assigned))
		for id := range assigned {
			missing = append(missing, id.String())
		}
		sort.Strings(missing)
		return "", fmt.Errorf("WorkUnit references criteria absent from Contract revision %s: %s", revision.ID, strings.Join(missing, ", "))
	}

	var b strings.Builder
	b.WriteString("Execute the following approved WorkUnit inside your isolated worktree.\n\n")
	b.WriteString("Frozen attribution:\n")
	b.WriteString("- Contract revision: " + revision.ID.String() + " (revision " + fmt.Sprintf("%d", revision.Number) + ")\n")
	b.WriteString("- WorkUnit: " + unit.ID.String() + "\n\n")
	b.WriteString("Goal: " + revision.Goal + "\n")
	b.WriteString("WorkUnit title: " + unit.Title + "\n")
	b.WriteString("Expected output: " + unit.OutputSummary + "\n")
	b.WriteString("\nAssigned Contract criteria (exact approved text):\n")
	for _, criterion := range criteria {
		b.WriteString("Criterion " + criterion.ID.String() + ":\n")
		writeExactRunBriefValue(&b, criterion.Text)
	}
	b.WriteString("Contract review method (exact approved text):\n")
	writeExactRunBriefValue(&b, revision.Review)
	b.WriteString("Evidence checks:\n")
	for _, check := range unit.EvidenceChecks {
		b.WriteString("- " + check + "\n")
	}
	b.WriteString("Verification: " + unit.VerificationRequirement + "\n")
	if len(unit.Checks) > 0 {
		b.WriteString("\nApproved executable checks:\n")
		b.WriteString("Kennel, not this worker, decides which exact commands are executable.\n")
		b.WriteString("Kennel will run this exact approved check after the provider terminates, under the daemon-owned Attempt/check/artifact reservation. Do not reconstruct or run it through a provider tool.\n")
		for _, check := range unit.Checks {
			b.WriteString("Check " + check.ID.String() + " for criterion " + check.CriterionID.String() + ":\n")
			b.WriteString("- working directory: isolated worktree root\n")
			b.WriteString("- timeout seconds: " + fmt.Sprintf("%d", check.TimeoutSeconds) + "\n")
			for index, argument := range check.Argv {
				fmt.Fprintf(&b, "- argv[%d] exact value:\n", index)
				writeExactRunBriefValue(&b, argument)
			}
		}
	}
	b.WriteString("\nUse only the governed repository tools Kennel exposes for repository reads and writes. Approved checks are not provider tools: Kennel runs each exact approved check through the daemon after you terminate, so do not reconstruct or execute one yourself. If a required repository operation is not exposed, stop and report the exact missing affordance.\n")
	if len(unit.StopConditions) > 0 {
		b.WriteString("Stop conditions:\n")
		for _, stop := range unit.StopConditions {
			b.WriteString("- " + stop + "\n")
		}
	}
	if len(revision.Constraints) > 0 {
		b.WriteString("Constraints:\n")
		for _, constraint := range revision.Constraints {
			b.WriteString("- " + constraint + "\n")
		}
	}
	b.WriteString("\nReport completion honestly; provider completion is not final acceptance.\n")
	return b.String(), nil
}

func writeExactRunBriefValue(b *strings.Builder, value string) {
	b.WriteString(value)
	if !strings.HasSuffix(value, "\n") {
		b.WriteByte('\n')
	}
}
