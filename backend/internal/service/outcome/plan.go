package outcome

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apierr"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// PlanView is the service projection of a plan revision.
type PlanView struct {
	Outcome domain.Outcome
	Plan    domain.PlanRevision
}

// AuthorizedPlanView is the projection of an owner-approved plan.
type AuthorizedPlanView struct {
	Outcome domain.Outcome
	Plan    domain.PlanRevision
}

// ApprovePlanInput identifies the plan revision the owner approves.
type ApprovePlanInput struct {
	PlanRevisionID           domain.PlanRevisionID
	ExpectedContractRevision int64
}

func mandatoryPlanStopConditions() []string {
	return []string{
		"Stop before an unapproved dependency",
		"Stop before any remote effect (network, push, PR, deploy) unless the approved Plan explicitly authorizes it",
		"Stop before writes outside the isolated worktree",
		"Stop on contradictory Project policy",
	}
}

// ProposePlan turns non-authoritative planning output into one canonical,
// fully-routed proposal. Project provider/model state is consulted here only as
// preference. The persisted WorkUnit bindings are the authority used later.
func (s *Service) ProposePlan(ctx context.Context, outcomeID domain.OutcomeID, expectedContractRevision int64) (PlanView, error) {
	return s.proposePlan(ctx, outcomeID, expectedContractRevision, "")
}

// ReplanPlan is an explicit owner-directed revision request. Feedback is
// carried into a new immutable proposal; it never mutates or reuses an
// approved/proposed plan in place.
func (s *Service) ReplanPlan(ctx context.Context, outcomeID domain.OutcomeID, expectedContractRevision int64, feedback string) (PlanView, error) {
	if strings.TrimSpace(feedback) == "" {
		return PlanView{}, apierr.Invalid("PLAN_REPLAN_FEEDBACK_REQUIRED", "Explain what the next proposal must change", nil)
	}
	return s.proposePlan(ctx, outcomeID, expectedContractRevision, strings.TrimSpace(feedback))
}

func (s *Service) proposePlan(ctx context.Context, outcomeID domain.OutcomeID, expectedContractRevision int64, replanFeedback string) (PlanView, error) {
	outcomeRecord, ok, err := s.store.GetOutcome(ctx, outcomeID)
	if err != nil {
		return PlanView{}, err
	}
	if !ok {
		return PlanView{}, apierr.NotFound("OUTCOME_NOT_FOUND", "That Outcome does not exist")
	}
	if expectedContractRevision < 1 {
		return PlanView{}, apierr.Invalid("EXPECTED_REVISION_REQUIRED", "State which contract revision the plan executes", nil)
	}
	if outcomeRecord.CurrentRevisionNumber != expectedContractRevision {
		return PlanView{}, apierr.New(apierr.KindConflict, "PLAN_CONTRACT_STALE",
			fmt.Sprintf("Plan must bind revision %s; reload the Outcome", formatI64(outcomeRecord.CurrentRevisionNumber)),
			map[string]any{"outcomeId": string(outcomeID), "expectedRevision": expectedContractRevision, "currentRevision": outcomeRecord.CurrentRevisionNumber})
	}
	if s.planIntelligence == nil || s.intelligenceRuns == nil || s.routing == nil {
		return PlanView{}, apierr.Internal("PLAN_CONTROL_PLANE_UNWIRED", "Planning is not fully wired in this environment")
	}

	revision, err := s.currentRevision(ctx, outcomeRecord)
	if err != nil {
		return PlanView{}, err
	}
	projectID, project, err := s.projectForOutcome(ctx, outcomeID)
	if err != nil {
		return PlanView{}, err
	}
	preference, hasPreference, err := domain.ResolveEffectiveExecutionPreference(revision.ExecutionPreference, project.Config)
	if err != nil {
		return PlanView{}, apierr.Invalid("PLAN_PREFERENCE_INVALID", err.Error(), nil)
	}
	routingPreference := routingPreferenceFromExecution(preference, hasPreference)

	// Ordinary re-entry is idempotent. A different immutable Contract or
	// execution preference naturally produces a fresh proposal. Explicit
	// re-planning is a separate command surface rather than pretending current
	// runtime readiness is part of immutable Plan identity.
	if replanFeedback == "" {
		if existing, found, err := s.store.LatestProposedPlanRevision(ctx, outcomeID, revision.Number); err != nil {
			return PlanView{}, err
		} else if found && planUsesPreference(existing, routingPreference) {
			return PlanView{Outcome: outcomeRecord, Plan: existing}, nil
		}
	}

	aliases, err := criterionAliases(revision)
	if err != nil {
		return PlanView{}, err
	}
	draft, err := s.draftPlanWithProvenance(ctx, projectID, outcomeRecord, revision, aliases, replanFeedback)
	if err != nil {
		return PlanView{}, err
	}
	units, routingDecisions, proposalSnapshot, err := s.compileAndRoutePlan(ctx, projectID, revision, draft, aliases, routingPreference)
	if err != nil {
		return PlanView{}, err
	}
	grants := grantsForUnits(units)
	if err := s.authorizeCapabilities(revision, grants, units); err != nil {
		return PlanView{}, err
	}
	digest, err := domain.ComputePlanRunBriefCoreDigest(revision, units, grants)
	if err != nil {
		return PlanView{}, err
	}
	proposal := domain.PlanRevision{
		ID:                     domain.PlanRevisionID("plan-" + uuid.NewString()),
		OutcomeID:              outcomeID,
		ContractRevisionNumber: revision.Number,
		Status:                 domain.PlanStatusProposed,
		Summary:                draft.Summary,
		Assumptions:            append([]string(nil), draft.Assumptions...),
		Blockers:               append([]string(nil), draft.Blockers...),
		WorkUnits:              units,
		Grants:                 grants,
		RoutingDecisions:       routingDecisions,
		RunBriefCoreDigest:     digest,
	}
	validation := proposal
	validation.Number = 1
	if err := validation.ValidateAgainstContract(revision); err != nil {
		return PlanView{}, apierr.Invalid("PLAN_DRAFT_CRITERIA_INVALID", err.Error(), nil)
	}
	proposalStage, err := s.EvaluateAdmissionStage(ctx, ports.AdmissionStageInput{Stage: ports.AdmissionStageProposal, ProjectID: projectID, Outcome: &outcomeRecord, Contract: &revision, Plan: &validation, RoutingSnapshot: &proposalSnapshot})
	if err != nil {
		return PlanView{}, err
	}
	if !proposalStage.Eligible {
		if s.admission == nil {
			return PlanView{}, apierr.Internal("ADMISSION_STORE_UNWIRED", "Admission persistence is unavailable in this environment")
		}
		proposalStage.Verdict.PlanRevisionID = nil
		if err := s.admission.AppendAdmissionEvaluation(ctx, proposalStage.Verdict); err != nil {
			return PlanView{}, fmt.Errorf("persist rejected proposal admission: %w", err)
		}
		return PlanView{}, apierr.New(apierr.KindConflict, "PLAN_PROPOSAL_NOT_ADMITTED", "This proposal cannot be routed under the verified capabilities", map[string]any{"reasons": proposalStage.Verdict.Reasons})
	}
	saved, err := s.store.AppendPlanRevision(ctx, outcomeID, proposal)
	if err != nil {
		return PlanView{}, err
	}
	return PlanView{Outcome: outcomeRecord, Plan: saved}, nil
}

func routingPreferenceFromExecution(preference domain.ExecutionPreference, ok bool) *domain.RoutingPreference {
	if !ok {
		return nil
	}
	return &domain.RoutingPreference{
		Provider:       string(preference.Provider),
		ModelSelection: preference.ModelSelection,
		Model:          preference.Model,
	}
}

func planUsesPreference(plan domain.PlanRevision, preference *domain.RoutingPreference) bool {
	if len(plan.RoutingDecisions) == 0 {
		return false
	}
	for _, record := range plan.RoutingDecisions {
		got := record.Decision.EffectivePreference
		switch {
		case got == nil && preference == nil:
			continue
		case got == nil || preference == nil:
			return false
		case got.Provider != preference.Provider || got.ModelSelection != preference.ModelSelection || got.Model != preference.Model:
			return false
		}
	}
	return true
}

func (s *Service) compileAndRoutePlan(
	ctx context.Context,
	projectID domain.ProjectID,
	revision domain.ContractRevision,
	draft domain.PlanDraftProposal,
	aliases map[string]domain.CriterionID,
	preference *domain.RoutingPreference,
) ([]domain.WorkUnit, []domain.WorkUnitRoutingDecision, ports.RoutingInventorySnapshot, error) {
	if err := draft.Validate(); err != nil {
		code := planDraftRefusalCode(err)
		return nil, nil, ports.RoutingInventorySnapshot{}, apierr.Invalid(code, err.Error(), nil)
	}

	order, err := draft.TopologicalOrder()
	if err != nil {
		return nil, nil, ports.RoutingInventorySnapshot{}, apierr.Invalid("PLAN_DRAFT_INVALID", err.Error(), nil)
	}
	drafts := make(map[string]domain.PlanDraftWorkUnit, len(draft.WorkUnits))
	ids := make(map[string]domain.WorkUnitID, len(draft.WorkUnits))
	for _, draftUnit := range draft.WorkUnits {
		key := strings.TrimSpace(draftUnit.Key)
		drafts[key] = draftUnit
		ids[key] = domain.WorkUnitID("wu-" + uuid.NewString())
	}

	units := make([]domain.WorkUnit, 0, len(draft.WorkUnits))
	for index, key := range order {
		draftUnit := drafts[key]
		unitID := ids[strings.TrimSpace(draftUnit.Key)]
		criteria := make([]domain.CriterionID, 0, len(draftUnit.CriteriaCovered))
		checks := make([]string, 0, len(draftUnit.EvidenceIdeas))
		for _, alias := range draftUnit.CriteriaCovered {
			criterionID, ok := aliases[strings.TrimSpace(alias)]
			if !ok {
				return nil, nil, ports.RoutingInventorySnapshot{}, apierr.Invalid("PLAN_DRAFT_CRITERION_UNKNOWN", "Plan intelligence referenced an unknown Contract criterion alias", map[string]any{"alias": alias})
			}
			criteria = append(criteria, criterionID)
		}
		checks = append(checks, draftUnit.EvidenceIdeas...)
		if len(checks) == 0 {
			for _, criterionID := range criteria {
				for _, criterion := range revision.Criteria {
					if criterion.ID == criterionID {
						checks = append(checks, criterion.Text)
					}
				}
			}
		}
		dependencies := make([]domain.WorkUnitID, 0, len(draftUnit.DependsOn))
		for _, dependency := range draftUnit.DependsOn {
			dependencyID, ok := ids[strings.TrimSpace(dependency)]
			if !ok {
				return nil, nil, ports.RoutingInventorySnapshot{}, apierr.Invalid("PLAN_DRAFT_DEPENDENCY_UNKNOWN", "Plan intelligence referenced an unknown WorkUnit dependency", map[string]any{"dependency": dependency})
			}
			dependencies = append(dependencies, dependencyID)
		}
		inputs := make([]domain.WorkUnitInput, 0, len(draftUnit.Inputs))
		for i, input := range draftUnit.Inputs {
			inputs = append(inputs, domain.WorkUnitInput{FromWorkUnitID: ids[strings.TrimSpace(input.FromKey)], Required: strings.TrimSpace(input.Required), Position: int64(i + 1)})
		}
		requiredCapabilities, err := draftUnit.Intent.RequiredCapabilities()
		if err != nil {
			return nil, nil, ports.RoutingInventorySnapshot{}, apierr.Invalid("PLAN_DRAFT_INTENT_INVALID", err.Error(), map[string]any{"workUnitKey": draftUnit.Key})
		}
		approvedChecks, err := compileApprovedChecks(unitID, draftUnit.CheckCommands, aliases, criteria)
		if err != nil {
			return nil, nil, ports.RoutingInventorySnapshot{}, err
		}
		unit := domain.WorkUnit{
			ID:                      unitID,
			Kind:                    domain.WorkUnitDirect,
			Intent:                  draftUnit.Intent,
			Role:                    draftUnit.Role,
			Inputs:                  inputs,
			Title:                   strings.TrimSpace(draftUnit.Title),
			Position:                int64(index + 1),
			ContractRevisionNumber:  revision.Number,
			OutputSummary:           strings.TrimSpace(draftUnit.OutputSummary),
			EvidenceChecks:          uniqueNonBlank(checks),
			VerificationRequirement: revision.Review,
			StopConditions:          uniqueNonBlank(append(append([]string{}, revision.StopConditions...), mandatoryPlanStopConditions()...)),
			DependsOn:               dependencies,
			CriterionIDs:            criteria,
			RequiredCapabilities:    requiredCapabilities,
			Checks:                  approvedChecks,
		}
		if s.AdmissionPolicy != nil && s.AdmissionPolicy.Validate() == nil {
			unit.ExecutionBudget = s.AdmissionPolicy.Default
		}
		if err := validateWorkUnitChecksAreExecutable(unit); err != nil {
			return nil, nil, ports.RoutingInventorySnapshot{}, err
		}
		if err := validateWorkUnitWithinContractCeiling(revision, unit); err != nil {
			return nil, nil, ports.RoutingInventorySnapshot{}, err
		}
		units = append(units, unit)
	}

	snapshot, err := s.routing.RoutingSnapshot(ctx, projectID, preference)
	if err != nil {
		return nil, nil, ports.RoutingInventorySnapshot{}, err
	}
	decisions := make([]domain.WorkUnitRoutingDecision, 0, len(units))
	for i := range units {
		decision := domain.RouteExecution(domain.RoutingRequirements{
			Role:             domain.RoutingRoleWorker,
			HardCapabilities: append([]string(nil), units[i].RequiredCapabilities...),
			Preference:       preference,
		}, snapshot.Candidates, snapshot.SnapshotID)
		binding, ok := decision.RecommendedBinding()
		if !ok {
			return nil, nil, ports.RoutingInventorySnapshot{}, apierr.New(apierr.KindConflict, "PLAN_NO_VALID_ROUTE",
				"No installed and ready worker can satisfy this WorkUnit's approved requirements",
				map[string]any{"workUnitId": string(units[i].ID), "evaluations": decision.Evaluations})
		}
		if err := units[i].BindExecution(binding); err != nil {
			return nil, nil, ports.RoutingInventorySnapshot{}, err
		}
		decisions = append(decisions, domain.WorkUnitRoutingDecision{WorkUnitID: units[i].ID, Decision: decision})
	}
	return units, decisions, snapshot, nil
}

// defaultApprovedCheckTimeoutSeconds is the bound applied when a proposal
// names none. Named operational policy: a check has to be bounded, and the
// model's silence is not permission to run indefinitely.
const defaultApprovedCheckTimeoutSeconds int64 = 300

// compileApprovedChecks turns proposed check commands into approved authority.
//
// Model output is a proposal, so every part of it is re-decided here: the
// criterion alias is resolved to internal identity and must be one this
// WorkUnit actually owns, the identifier is minted by the daemon rather than
// accepted from the proposal, an unbounded timeout is bounded, and the command
// shape is validated by the domain — which refuses a shell outright.
func compileApprovedChecks(
	unitID domain.WorkUnitID,
	proposed []domain.PlanDraftCheck,
	aliases map[string]domain.CriterionID,
	owned []domain.CriterionID,
) ([]domain.ApprovedCheck, error) {
	if len(proposed) == 0 {
		return nil, nil
	}
	checks := make([]domain.ApprovedCheck, 0, len(proposed))
	for i, draft := range proposed {
		criterionID, ok := aliases[strings.TrimSpace(draft.CriterionAlias)]
		if !ok {
			return nil, apierr.Invalid("PLAN_DRAFT_CHECK_CRITERION_UNKNOWN",
				"A proposed check named an unknown Contract criterion alias",
				map[string]any{"workUnitId": string(unitID), "alias": draft.CriterionAlias})
		}
		timeout := draft.TimeoutSeconds
		if timeout <= 0 || timeout > domain.ApprovedCheckMaxTimeoutSeconds {
			timeout = defaultApprovedCheckTimeoutSeconds
		}
		checks = append(checks, domain.ApprovedCheck{
			// The daemon mints the identity: a proposal-supplied id could
			// collide with, or impersonate, a check from another Plan.
			ID:             domain.ApprovedCheckID(fmt.Sprintf("chk-%s-%d", uuid.NewString(), i+1)),
			CriterionID:    criterionID,
			Argv:           append([]string(nil), draft.Argv...),
			TimeoutSeconds: timeout,
		})
	}
	if err := domain.ValidateApprovedChecks(checks, owned); err != nil {
		return nil, apierr.Invalid("PLAN_DRAFT_CHECK_INVALID", err.Error(), map[string]any{"workUnitId": string(unitID)})
	}
	return checks, nil
}

// contractCapabilityCeiling converts the confirmed typed ceiling to capability
// names. It never invents authority for a missing/zero ceiling. Read is implied
// by write/execute because neither operation can be performed meaningfully on
// a workspace Kennel is forbidden to inspect.
func contractCapabilityCeiling(revision domain.ContractRevision) []string {
	ceiling := revision.AuthorityCeiling
	var allowed []string
	if ceiling.ReadWorkspace || ceiling.WriteWorkspace || ceiling.ExecuteLocal {
		allowed = append(allowed, domain.CapabilityWorktreeRead)
	}
	if ceiling.WriteWorkspace {
		allowed = append(allowed, domain.CapabilityWorktreeWrite)
	}
	if ceiling.ExecuteLocal {
		allowed = append(allowed, domain.CapabilityWorktreeExec)
	}
	return allowed
}

// validateWorkUnitChecksAreExecutable refuses a Plan proposing checks the
// WorkUnit carrying them could never run.
//
// governedcheck launches nothing without worktree execution in the Attempt's
// frozen policy, so checks on a unit that only reads or writes are recorded as
// unavailable at reconciliation time — truthfully, but far too late. The owner
// has by then approved a Plan whose criterion coverage was never real. Refusing
// here moves that discovery to approval, where it is a Plan to revise rather
// than a result to explain.
//
// The refusal deliberately does not widen the unit's capabilities to fit the
// checks. Execution authority granted to satisfy a check would also be granted
// to the provider running the work, which is a larger authority than anyone
// proposed. Naming the wrong intent is the proposal's defect to correct.
func validateWorkUnitChecksAreExecutable(unit domain.WorkUnit) error {
	if len(unit.Checks) == 0 {
		return nil
	}
	for _, capability := range unit.RequiredCapabilities {
		if capability == domain.CapabilityWorktreeExec {
			return nil
		}
	}
	commands := make([]string, 0, len(unit.Checks))
	for _, check := range unit.Checks {
		commands = append(commands, strings.Join(check.Argv, " "))
	}
	return apierr.New(apierr.KindConflict, "PLAN_CHECK_EXECUTION_REQUIRED",
		"This WorkUnit proposes deterministic checks but does not require "+domain.CapabilityWorktreeExec+
			", so the checks could never run and its criteria would go unproved; propose them on an executing WorkUnit",
		map[string]any{
			"workUnitId": string(unit.ID), "checks": commands,
			"requiredCapabilities": unit.RequiredCapabilities,
		})
}

func validateWorkUnitWithinContractCeiling(revision domain.ContractRevision, unit domain.WorkUnit) error {
	allowed := contractCapabilityCeiling(revision)
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, capability := range allowed {
		allowedSet[capability] = struct{}{}
	}
	var missing []string
	for _, capability := range unit.RequiredCapabilities {
		if _, ok := allowedSet[capability]; !ok {
			missing = append(missing, capability)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	if len(allowed) == 0 {
		return apierr.New(apierr.KindConflict, "PLAN_AUTHORITY_REQUIRED",
			"The confirmed Contract does not grant workspace authority for new execution; revise or reconfirm the Contract before planning",
			map[string]any{"workUnitId": string(unit.ID), "required": missing, "contractCeiling": allowed})
	}
	return apierr.New(apierr.KindConflict, "PLAN_AUTHORITY_INSUFFICIENT",
		"This WorkUnit needs authority outside the confirmed Contract ceiling",
		map[string]any{"workUnitId": string(unit.ID), "required": unit.RequiredCapabilities, "missing": missing, "contractCeiling": allowed})
}

func grantsForUnits(units []domain.WorkUnit) []domain.CapabilityGrant {
	names := domain.RequiredPlanCapabilities(units)
	grants := make([]domain.CapabilityGrant, 0, len(names))
	for _, name := range names {
		grants = append(grants, domain.CapabilityGrant{
			ID: domain.CapabilityGrantID("cg-" + uuid.NewString()), Name: name, Scope: "worktree/*",
		})
	}
	return grants
}

func uniqueNonBlank(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// ApprovePlan converts an already-routed immutable proposal into authority.
// It deliberately does not read Project preferences or routing inventory.
func (s *Service) ApprovePlan(ctx context.Context, outcomeID domain.OutcomeID, in ApprovePlanInput) (AuthorizedPlanView, error) {
	outcomeRecord, ok, err := s.store.GetOutcome(ctx, outcomeID)
	if err != nil {
		return AuthorizedPlanView{}, err
	}
	if !ok {
		return AuthorizedPlanView{}, apierr.NotFound("OUTCOME_NOT_FOUND", "That Outcome does not exist")
	}
	if in.PlanRevisionID.IsZero() {
		return AuthorizedPlanView{}, apierr.Invalid("PLAN_ID_REQUIRED", "Name the plan to authorize", nil)
	}
	if in.ExpectedContractRevision >= 1 && in.ExpectedContractRevision != outcomeRecord.CurrentRevisionNumber {
		return AuthorizedPlanView{}, apierr.New(apierr.KindConflict, "PLAN_CONTRACT_STALE",
			fmt.Sprintf("Approval was prepared against revision %s; the Outcome is at %s", formatI64(in.ExpectedContractRevision), formatI64(outcomeRecord.CurrentRevisionNumber)),
			map[string]any{"outcomeId": string(outcomeID), "expectedRevision": in.ExpectedContractRevision, "currentRevision": outcomeRecord.CurrentRevisionNumber})
	}
	plan, found, err := s.store.GetPlanRevision(ctx, outcomeID, in.PlanRevisionID)
	if err != nil {
		return AuthorizedPlanView{}, err
	}
	if !found {
		return AuthorizedPlanView{}, apierr.NotFound("PLAN_NOT_FOUND", "That plan does not exist")
	}
	if !plan.BindsCurrentContract(outcomeRecord.CurrentRevisionNumber) {
		return AuthorizedPlanView{}, apierr.New(apierr.KindConflict, "PLAN_CONTRACT_STALE",
			fmt.Sprintf("Plan binds contract revision %s; the Outcome is at %s — propose a new plan", formatI64(plan.ContractRevisionNumber), formatI64(outcomeRecord.CurrentRevisionNumber)),
			map[string]any{"outcomeId": string(outcomeID), "planId": string(plan.ID), "planRevisionBinding": plan.ContractRevisionNumber, "currentRevision": outcomeRecord.CurrentRevisionNumber})
	}
	revision, err := s.currentRevision(ctx, outcomeRecord)
	if err != nil {
		return AuthorizedPlanView{}, err
	}
	if err := plan.ValidateForApproval(revision); err != nil {
		return AuthorizedPlanView{}, apierr.New(apierr.KindConflict, "PLAN_NOT_APPROVABLE", err.Error(), map[string]any{"planId": string(plan.ID)})
	}
	if err := s.authorizeCapabilities(revision, plan.Grants, plan.WorkUnits); err != nil {
		return AuthorizedPlanView{}, err
	}
	if s.admission == nil {
		return AuthorizedPlanView{}, apierr.Internal("ADMISSION_STORE_UNWIRED", "Admission persistence is unavailable in this environment")
	}
	projectID, found, err := s.store.GetOutcomeProjectID(ctx, outcomeID)
	if err != nil {
		return AuthorizedPlanView{}, err
	}
	if !found {
		return AuthorizedPlanView{}, apierr.NotFound("PROJECT_NOT_FOUND", "Register that Project before approving this Plan")
	}
	if plan.Status == domain.PlanStatusApproved {
		persisted, found, loadErr := s.admission.GetAdmittedVerdict(ctx, plan.ID)
		if loadErr != nil {
			return AuthorizedPlanView{}, loadErr
		}
		if !found || persisted.OutcomeID != outcomeID || persisted.PlanRevisionID == nil || *persisted.PlanRevisionID != plan.ID || persisted.ContractRevisionNumber == nil || *persisted.ContractRevisionNumber != revision.Number {
			return AuthorizedPlanView{}, apierr.Conflict("PLAN_APPROVAL_REPLAY_MISMATCH", "Approved Plan admission evidence does not match its immutable Contract and Plan identity", nil)
		}
		return AuthorizedPlanView{Outcome: outcomeRecord, Plan: plan}, nil
	}
	staged, err := s.EvaluateAdmissionStage(ctx, ports.AdmissionStageInput{Stage: ports.AdmissionStageApproval, ProjectID: projectID, Outcome: &outcomeRecord, Contract: &revision, Plan: &plan})
	if err != nil {
		return AuthorizedPlanView{}, err
	}
	if !staged.Eligible {
		if err := s.admission.AppendAdmissionEvaluation(ctx, staged.Verdict); err != nil {
			return AuthorizedPlanView{}, fmt.Errorf("persist rejected approval admission: %w", err)
		}
		return AuthorizedPlanView{}, apierr.New(apierr.KindConflict, "PLAN_NOT_ADMITTED", "This Plan cannot be executed with the current verified capabilities", map[string]any{"reasons": staged.Verdict.Reasons})
	}
	approved, found, err := s.admission.ApprovePlanWithAdmission(ctx, outcomeID, plan.ID, staged.Verdict)
	if err != nil {
		return AuthorizedPlanView{}, err
	}
	if !found {
		return AuthorizedPlanView{}, apierr.NotFound("PLAN_NOT_FOUND", "That plan does not exist")
	}
	return AuthorizedPlanView{Outcome: outcomeRecord, Plan: approved}, nil
}

// GetLatestPlan returns the latest plan revision for an Outcome.
func (s *Service) GetLatestPlan(ctx context.Context, outcomeID domain.OutcomeID) (PlanView, error) {
	outcomeRecord, ok, err := s.store.GetOutcome(ctx, outcomeID)
	if err != nil {
		return PlanView{}, err
	}
	if !ok {
		return PlanView{}, apierr.NotFound("OUTCOME_NOT_FOUND", "That Outcome does not exist")
	}
	plan, found, err := s.store.GetLatestPlanRevision(ctx, outcomeID)
	if err != nil {
		return PlanView{}, err
	}
	if !found {
		return PlanView{}, apierr.NotFound("PLAN_NOT_FOUND", "This Outcome has no plan yet")
	}
	return PlanView{Outcome: outcomeRecord, Plan: plan}, nil
}

func (s *Service) currentRevision(ctx context.Context, outcomeRecord domain.Outcome) (domain.ContractRevision, error) {
	history, err := s.store.ListContractRevisions(ctx, outcomeRecord.ID)
	if err != nil {
		return domain.ContractRevision{}, err
	}
	for _, revision := range history {
		if revision.Number == outcomeRecord.CurrentRevisionNumber {
			return revision, nil
		}
	}
	return domain.ContractRevision{}, fmt.Errorf("outcome %s points at missing revision %d", outcomeRecord.ID, outcomeRecord.CurrentRevisionNumber)
}

// authoritativeCapabilities is the daemon/runtime policy ceiling only. The
// Contract ceiling is checked separately and never defaults from this value.
func (s *Service) authoritativeCapabilities() []string {
	if len(s.PolicyLayers) == 0 {
		return []string{domain.CapabilityWorktreeRead, domain.CapabilityWorktreeWrite, domain.CapabilityWorktreeExec}
	}
	return domain.AuthorityIntersection(s.PolicyLayers...)
}

func (s *Service) authorizeCapabilities(revision domain.ContractRevision, grants []domain.CapabilityGrant, units []domain.WorkUnit) error {
	for _, unit := range units {
		if err := validateWorkUnitWithinContractCeiling(revision, unit); err != nil {
			return err
		}
	}
	if err := domain.ValidateExactPlanCapabilityGrants(grants, units); err != nil {
		return apierr.Invalid("PLAN_CAPABILITY_NOT_MINIMAL", err.Error(), nil)
	}
	authoritative := s.authoritativeCapabilities()
	if err := domain.GrantsFailClosed(grants, authoritative); err != nil {
		offenders := make([]string, 0, len(grants))
		for _, grant := range grants {
			offenders = append(offenders, grant.Name)
		}
		sort.Strings(offenders)
		return apierr.New(apierr.KindConflict, "PLAN_CAPABILITY_UNAUTHORIZED", err.Error(),
			map[string]any{"granted": offenders, "authoritative": authoritative})
	}
	return nil
}

func formatI64(v int64) string { return strconv.FormatInt(v, 10) }

func planDraftRefusalCode(err error) string {
	var typed *domain.PlanDraftValidationError
	if errors.As(err, &typed) {
		return string(typed.Code)
	}
	return "PLAN_DRAFT_INVALID"
}
