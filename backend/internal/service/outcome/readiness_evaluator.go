package outcome

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apierr"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// EvaluatePlanReadiness is the S3.1 readiness evaluator: the deterministic
// control-plane dry-run run at the Plan-draft seam, before any immutable Plan
// persistence. Given one frozen evaluation fence and one ready-claim draft, it
// validates the draft graph under S2 rules, derives every WorkUnit capability
// from intent and compares it with the confirmed Contract ceiling, dry-run
// compiles every proposed check (criterion ownership, command shape, execution
// capability) without minting durable IDs, evaluates every provisional unit
// against one normalized routing inventory snapshot, and folds planner-declared
// blockers. Every material condition becomes a typed issue built through
// domain.NewPlanningReadinessIssue - constructor plus validation, never
// unnormalized struct assembly - and the issues are canonicalized into one
// packet. The status is derived, never accepted: zero issues is ready and may
// enter the canonical compiler; any issue means no Plan is written.
//
// Planner-declared issues arrive parsed and validated from the strict
// envelope; they are re-keyed here under the final snapshot-bound fence so one
// packet generation always binds one fence.
func (s *Service) EvaluatePlanReadiness(
	ctx context.Context,
	fence domain.PlanningReadinessFence,
	projectID domain.ProjectID,
	revision domain.ContractRevision,
	preference *domain.RoutingPreference,
	draft *domain.PlanDraftProposal,
	plannerIssues []domain.PlanningReadinessIssue,
	message string,
) (domain.PlanningReadinessResult, ports.RoutingInventorySnapshot, error) {
	if s.routing == nil {
		return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, apierr.Internal("PLAN_CONTROL_PLANE_UNWIRED", "Planning is not fully wired in this environment")
	}
	// The fence must describe exactly the Contract revision being evaluated:
	// issue keys are minted under the fence, so evaluating revision B's
	// ceiling while labeling keys with revision A would silently mis-fence the
	// packet. Zero or blank fence identity fails the same way - with one
	// packet-sanctioned exception: a sessionless one-shot evaluation carries
	// no planning lineage, so the session identity is zero AND the session
	// revision is zero. A half-zero session identity is always malformed.
	// This check runs before any inventory read: a malformed evaluation never
	// touches routing.
	sessionless := fence.PlanningSessionID.IsZero() && fence.SessionRevision == 0
	if !sessionless && (fence.PlanningSessionID.IsZero() || fence.SessionRevision <= 0) {
		return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, apierr.Internal("PLANNING_READINESS_FENCE_INVALID", "planning readiness fence identity is zero or blank")
	}
	if fence.ContractRevisionID.IsZero() || strings.TrimSpace(fence.ContextDigest.String()) == "" {
		return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, apierr.Internal("PLANNING_READINESS_FENCE_INVALID", "planning readiness fence identity is zero or blank")
	}
	if fence.ContractRevisionID != revision.ID {
		return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, apierr.Internal("PLANNING_READINESS_FENCE_MISMATCH", "planning readiness fence names a different Contract revision than the one under evaluation")
	}
	// A nil draft is the planner-declared non-ready path: the strict envelope
	// carried issues and no proposal, so steps 5-9 have nothing to evaluate.
	// Draft-less evaluations must carry at least one planner issue.
	if draft == nil && len(plannerIssues) == 0 {
		return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, apierr.Invalid("PLANNING_READINESS_PAYLOAD_INVALID", "a readiness evaluation without a proposal must carry planner-declared issues", nil)
	}
	var order []string
	if draft != nil {
		// Evaluator step 5: the draft graph must survive S2 validation before any
		// dry-run. A structurally invalid proposal is invalid provider output,
		// never a typed readiness issue.
		if err := draft.Validate(); err != nil {
			return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, apierr.Invalid(planDraftRefusalCode(err), err.Error(), nil)
		}
		var err error
		order, err = draft.TopologicalOrder()
		if err != nil {
			return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, apierr.Invalid("PLAN_DRAFT_INVALID", err.Error(), nil)
		}
	}
	aliases, err := criterionAliases(revision)
	if err != nil {
		return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, err
	}

	// Evaluator step 8's input: exactly one normalized inventory snapshot per
	// evaluation, bound into the fence before any issue key is minted.
	snapshot, err := s.routing.RoutingSnapshot(ctx, projectID, preference)
	if err != nil {
		return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, err
	}
	// A snapshot without a stable identity cannot fence anything: fail before
	// packet construction rather than mint keys bound to a blank generation.
	if err := validateRoutingSnapshotIdentity(snapshot); err != nil {
		return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, err
	}
	fence.RoutingSnapshotID = snapshot.SnapshotID

	draftsByKey := map[string]domain.PlanDraftWorkUnit{}
	draftUnitCount := 0
	if draft != nil {
		draftUnitCount = len(draft.WorkUnits) + len(draft.Blockers)
		for _, unit := range draft.WorkUnits {
			draftsByKey[strings.TrimSpace(unit.Key)] = unit
		}
	}

	issues := make([]domain.PlanningReadinessIssue, 0, len(plannerIssues)+draftUnitCount)
	addIssue := func(issue domain.PlanningReadinessIssue) error {
		built, err := domain.NewPlanningReadinessIssue(fence, issue)
		if err != nil {
			return fmt.Errorf("evaluator built an invalid readiness issue: %w", err)
		}
		issues = append(issues, built)
		return nil
	}

	// Planner-declared issues re-keyed under the final fence. Re-keying under
	// the current evaluation is only sound if the issue still describes THIS
	// draft and THIS Contract revision: every referenced work unit key and
	// criterion alias must exist now, or a stale issue parsed for an earlier
	// draft would surface fresh-keyed references to things that do not exist.
	for _, issue := range plannerIssues {
		for _, unitKey := range issue.WorkUnitKeys {
			if _, ok := draftsByKey[strings.TrimSpace(unitKey)]; !ok {
				return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, apierr.Invalid("PLANNING_READINESS_ISSUE_STALE", "planner-declared readiness issue references a work unit the current draft does not carry", map[string]any{"workUnitKey": unitKey})
			}
		}
		for _, alias := range issue.CriterionAliases {
			if _, ok := aliases[strings.TrimSpace(alias)]; !ok {
				return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, apierr.Invalid("PLANNING_READINESS_ISSUE_STALE", "planner-declared readiness issue references a Contract criterion alias the current revision does not carry", map[string]any{"criterionAlias": alias})
			}
		}
		if err := addIssue(issue); err != nil {
			return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, apierr.Invalid("PLANNING_READINESS_ISSUE_INVALID", err.Error(), nil)
		}
	}

	ceilingSet := map[string]bool{}
	for _, capability := range contractCapabilityCeiling(revision) {
		ceilingSet[capability] = true
	}

	for _, key := range order {
		unit := draftsByKey[key]
		required, err := unit.Intent.RequiredCapabilities()
		if err != nil {
			// draft.Validate already rejected unsupported intents, so reaching
			// this is an evaluator/domain drift, not provider output.
			return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, apierr.Internal("PLAN_DRAFT_INTENT_INVALID", err.Error())
		}

		// Evaluator step 6: Kennel derives the capability union from intent;
		// any requirement above the confirmed ceiling is one typed issue.
		for _, capability := range required {
			if ceilingSet[capability] {
				continue
			}
			if err := addIssue(domain.PlanningReadinessIssue{
				Kind:                domain.ReadinessAuthorityInsufficient,
				Route:               domain.RouteReviseContract,
				Source:              domain.ReadinessSourceControlPlane,
				Prompt:              fmt.Sprintf("Work unit %q needs %s, which the confirmed Contract does not grant.", key, capability),
				Reason:              "Kennel derives capability requirements from declared WorkUnit intent, and this requirement exceeds the confirmed Contract ceiling.",
				Recommendation:      "Revise or reconfirm the Contract to grant this authority, or replan the WorkUnit within the current ceiling.",
				WorkUnitKeys:        []string{key},
				RequestedCapability: capability,
			}); err != nil {
				return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, err
			}
		}

		// Evaluator step 7: dry-run compile of proposed checks. Criterion
		// aliases are resolved against the frozen Contract, ownership against
		// the unit's own coverage, command shape against the same domain
		// validation the canonical compiler uses. No durable check ID is
		// minted; every defect is one typed revise_plan issue.
		ownedCriteria := make([]domain.CriterionID, 0, len(unit.CriteriaCovered))
		owned := map[domain.CriterionID]bool{}
		for _, alias := range unit.CriteriaCovered {
			criterionID, ok := aliases[strings.TrimSpace(alias)]
			if !ok {
				// A unit-level coverage reference to a criterion the Contract
				// does not carry is invalid provider output, on par with an
				// unknown dependency: the graph itself does not close.
				return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, apierr.Invalid("PLAN_DRAFT_CRITERION_UNKNOWN", "Plan intelligence referenced an unknown Contract criterion alias", map[string]any{"alias": alias})
			}
			ownedCriteria = append(ownedCriteria, criterionID)
			owned[criterionID] = true
		}
		checkAliases := make([]string, 0, len(unit.CheckCommands))
		for _, check := range unit.CheckCommands {
			alias := strings.TrimSpace(check.CriterionAlias)
			checkAliases = append(checkAliases, alias)
			criterionID, ok := aliases[alias]
			if !ok {
				if err := addIssue(checkUnrepresentableIssue(key, alias, "the check names a Contract criterion alias that does not exist")); err != nil {
					return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, err
				}
				continue
			}
			if !owned[criterionID] {
				if err := addIssue(checkUnrepresentableIssue(key, alias, "the check names a criterion this WorkUnit does not own, so its result could silently satisfy another unit's obligation")); err != nil {
					return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, err
				}
				continue
			}
			dryRun := domain.ApprovedCheck{
				ID:             domain.ApprovedCheckID(alias),
				CriterionID:    criterionID,
				Argv:           append([]string(nil), check.Argv...),
				TimeoutSeconds: check.TimeoutSeconds,
			}
			if dryRun.TimeoutSeconds <= 0 || dryRun.TimeoutSeconds > domain.ApprovedCheckMaxTimeoutSeconds {
				dryRun.TimeoutSeconds = defaultApprovedCheckTimeoutSeconds
			}
			if err := dryRun.Validate(); err != nil {
				if err := addIssue(checkUnrepresentableIssue(key, alias, err.Error())); err != nil {
					return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, err
				}
			}
		}
		// A unit proposing checks without execute intent can never run them:
		// the criteria they were meant to prove would go unproved.
		hasExec := false
		for _, capability := range required {
			if capability == domain.CapabilityWorktreeExec {
				hasExec = true
			}
		}
		if len(unit.CheckCommands) > 0 && !hasExec {
			if err := addIssue(domain.PlanningReadinessIssue{
				Kind:             domain.ReadinessCheckUnrepresentable,
				Route:            domain.RouteRevisePlan,
				Source:           domain.ReadinessSourceControlPlane,
				Prompt:           fmt.Sprintf("Work unit %q proposes deterministic checks but declares no execution intent, so the checks could never run.", key),
				Reason:           "Checks run only under worktree execution authority; naming the wrong intent leaves this unit's criteria unprovable.",
				Recommendation:   "Move the checks to an executing WorkUnit or re-declare this unit's intent.",
				WorkUnitKeys:     []string{key},
				CriterionAliases: sortedStrings(checkAliases),
			}); err != nil {
				return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, err
			}
		}

		// Evaluator step 8: deterministic routing evaluation of the provisional
		// unit against the frozen inventory snapshot.
		decision := domain.RouteExecution(domain.RoutingRequirements{
			Role:             domain.RoutingRoleWorker,
			HardCapabilities: append([]string(nil), required...),
			Preference:       preference,
		}, snapshot.Candidates, snapshot.SnapshotID)
		if _, ok := decision.RecommendedBinding(); !ok {
			if err := addIssue(workerUnavailableIssue(key, decision)); err != nil {
				return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, err
			}
		}
	}

	// Evaluator step 9: a ready claim carrying blockers cannot normalize to
	// ready. The planner declared the blockers, so each becomes a
	// planner-declared, owner-answerable context issue - prose is never read
	// for connectors, capabilities, paths, or authority.
	if draft != nil {
		for _, blocker := range draft.Blockers {
			if err := addIssue(domain.PlanningReadinessIssue{
				Kind:           domain.ReadinessContextInsufficient,
				Route:          domain.RouteAnswerContext,
				Source:         domain.ReadinessSourcePlannerDeclared,
				Prompt:         blocker,
				Reason:         "The planner declared this blocker on a ready proposal; a proposal carrying blockers cannot normalize to ready.",
				Recommendation: "Answer or resolve the blocker, then replan.",
			}); err != nil {
				return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, err
			}
		}
	}

	// Evaluator step 10: one packet, deduplicated and stably ordered.
	canonical, err := domain.CanonicalizePlanningReadinessIssues(issues, order)
	if err != nil {
		return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, fmt.Errorf("canonicalize readiness issues: %w", err)
	}
	result := domain.NewPlanningReadinessResult(message, draft, canonical)
	if err := result.Validate(); err != nil {
		return domain.PlanningReadinessResult{}, ports.RoutingInventorySnapshot{}, fmt.Errorf("evaluator produced an invalid readiness result: %w", err)
	}
	return result, snapshot, nil
}

// validateRoutingSnapshotIdentity refuses a snapshot without a stable
// identity: nothing can fence to a blank generation, at the evaluator seam or
// at the compiler seam.
func validateRoutingSnapshotIdentity(snapshot ports.RoutingInventorySnapshot) error {
	if strings.TrimSpace(snapshot.SnapshotID) == "" || strings.TrimSpace(snapshot.GenerationID) == "" {
		return apierr.Internal("ROUTING_INVENTORY_MALFORMED", "routing inventory returned a snapshot without a stable identity")
	}
	return nil
}

// checkUnrepresentableIssue builds the typed issue for one proposed check the
// dry-run compile cannot represent safely.
func checkUnrepresentableIssue(workUnitKey, alias, why string) domain.PlanningReadinessIssue {
	return domain.PlanningReadinessIssue{
		Kind:             domain.ReadinessCheckUnrepresentable,
		Route:            domain.RouteRevisePlan,
		Source:           domain.ReadinessSourceControlPlane,
		Prompt:           fmt.Sprintf("Replan the proposed check %q on work unit %q: %s.", alias, workUnitKey, why),
		Reason:           "The proposed deterministic check cannot be compiled into approved authority as written.",
		Recommendation:   "Revise the check so it names a criterion the WorkUnit owns and a bounded non-shell command.",
		WorkUnitKeys:     []string{workUnitKey},
		CriterionAliases: []string{alias},
	}
}

// workerUnavailableIssue builds the typed issue for a provisional WorkUnit no
// candidate can execute, routed by the narrowest fix the evaluation codes
// justify: a candidate blocked only by readiness/authentication routes to
// harness authentication; anything else is a harness choice.
func workerUnavailableIssue(workUnitKey string, decision domain.RoutingDecision) domain.PlanningReadinessIssue {
	route := domain.RouteChooseHarness
	for _, eval := range decision.Evaluations {
		if len(eval.RejectionCodes) == 0 {
			continue
		}
		authOnly := true
		for _, code := range eval.RejectionCodes {
			if code != "NOT_READY" && code != "READINESS_UNKNOWN" {
				authOnly = false
				break
			}
		}
		if authOnly {
			route = domain.RouteAuthenticateHarness
			break
		}
	}
	codes := map[string]bool{}
	for _, eval := range decision.Evaluations {
		for _, code := range eval.RejectionCodes {
			codes[code] = true
		}
	}
	reason := "No installed candidate was evaluated."
	if len(codes) > 0 {
		flat := make([]string, 0, len(codes))
		for code := range codes {
			flat = append(flat, code)
		}
		sort.Strings(flat)
		reason = "Every routing evaluation rejected the unit: " + strings.Join(flat, ", ") + "."
	}
	recommendation := "Choose or install a harness that satisfies the unit's approved requirements, then replan."
	if route == domain.RouteAuthenticateHarness {
		recommendation = "Authenticate the candidate harness (its profile or credential is missing), then replan."
	}
	return domain.PlanningReadinessIssue{
		Kind:           domain.ReadinessWorkerUnavailable,
		Route:          route,
		Source:         domain.ReadinessSourceControlPlane,
		Prompt:         fmt.Sprintf("No installed and ready worker can satisfy work unit %q's approved requirements.", workUnitKey),
		Reason:         reason,
		Recommendation: recommendation,
		WorkUnitKeys:   []string{workUnitKey},
	}
}

func sortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}
