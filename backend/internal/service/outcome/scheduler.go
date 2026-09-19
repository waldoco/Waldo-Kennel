package outcome

import (
	"context"
	"errors"
	"fmt"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apierr"
)

// WorkUnitScheduleState is derived control-plane state. It is never persisted;
// canonical Plan, Attempt and proof facts remain the source of truth.
type WorkUnitScheduleState string

const (
	// WorkUnitScheduleBlocked means dependencies or proof prevent admission.
	WorkUnitScheduleBlocked WorkUnitScheduleState = "blocked"
	// WorkUnitScheduleRunnable means the unit can be admitted.
	WorkUnitScheduleRunnable WorkUnitScheduleState = "runnable"
	// WorkUnitScheduleExecuting means an attempt currently owns the unit.
	WorkUnitScheduleExecuting WorkUnitScheduleState = "executing"
	// WorkUnitScheduleProven means execution produced recorded proof.
	WorkUnitScheduleProven WorkUnitScheduleState = "proven"
	// WorkUnitScheduleRetryable means a later governed retry may be admitted.
	WorkUnitScheduleRetryable WorkUnitScheduleState = "retryable"
	// WorkUnitSchedulePaused means this unit's own attempt is paused. It still
	// holds custody, so it is not merely "blocked": the owner has to resume or
	// cancel it before anything else can run.
	WorkUnitSchedulePaused WorkUnitScheduleState = "paused"
)

// There is deliberately no "unresolved" schedule state yet.
//
// Unresolved liveness is real and it does block duplicate execution, but it
// is enforced before an attempt reaches a terminal status: EvaluateAttemptLiveness
// and refuseUnprovenCustody keep an unaccountable attempt Running rather than
// closing it. AttemptLost is the opposite situation — it is reached only
// through explicit owner recovery, which deliberately releases custody so a
// replacement may start. Deriving "unresolved" from Lost would therefore
// misreport an attempt the owner has already reconciled, and would discourage
// a legitimate retry.
//
// deriveSchedule receives no heartbeat facts, so there is no truthful input
// for the distinction today. Phase C makes runtime observations durable; the
// state belongs here once a fact can populate it, not before.

// WorkUnitBlockedReason says why a blocked unit is blocked.
//
// "Blocked" used to cover two unrelated situations — this unit's dependencies
// are unproven, and some other unit is holding the serial custody fence. The
// owner's next move is completely different between them, and a graph that
// draws both the same way cannot explain itself.
type WorkUnitBlockedReason string

const (
	// BlockedAwaitingDependencyProof means an upstream unit has no accepted
	// proof yet. Waiting is correct; nothing is wrong.
	BlockedAwaitingDependencyProof WorkUnitBlockedReason = "awaiting_dependency_proof"
	// BlockedCustodyHeld means this unit is ready on its own terms but another
	// unit currently owns the workspace. This is the serial execution limit,
	// not a fault.
	BlockedCustodyHeld WorkUnitBlockedReason = "custody_held"
	// BlockedUpstreamArtifactUnavailable means a dependency is proved but the
	// bytes it produced cannot be handed down — missing, incomplete, unfrozen
	// or mislineaged retention.
	//
	// Proof and artifact eligibility are different facts, and a graph that
	// showed such a unit as runnable would be inviting a Start the daemon is
	// certain to refuse. The Contract criterion is satisfied; the successor
	// still has nothing to build on.
	BlockedUpstreamArtifactUnavailable WorkUnitBlockedReason = "upstream_artifact_unavailable"
)

// ScheduleNoRunnableReason explains an empty runnable set.
//
// A Mission with nothing to start must say why. Rendering a spinner for a
// schedule that is permanently waiting on the owner is the specific failure
// this replaces.
type ScheduleNoRunnableReason string

const (
	// NoRunnableAllProven means the plan's work is done and proof is recorded.
	NoRunnableAllProven ScheduleNoRunnableReason = "all_units_proven"
	// NoRunnableExecuting means an attempt is currently running.
	NoRunnableExecuting ScheduleNoRunnableReason = "attempt_executing"
	// NoRunnablePaused means a paused attempt holds custody.
	NoRunnablePaused ScheduleNoRunnableReason = "attempt_paused"
	// NoRunnableAwaitingProof means every remaining unit waits on proof that
	// no running attempt is currently producing — the owner has to record or
	// repair evidence.
	NoRunnableAwaitingProof ScheduleNoRunnableReason = "awaiting_proof"
)

// WorkUnitScheduleView is the Mission-Control-ready scheduler projection for
// one canonical WorkUnit.
type WorkUnitScheduleView struct {
	WorkUnit             domain.WorkUnit
	State                WorkUnitScheduleState
	Attempts             []domain.Attempt
	BlockingDependencies []domain.WorkUnitID
	// BlockedReason is set only when State is blocked, and says which kind of
	// blocked it is.
	BlockedReason WorkUnitBlockedReason
	// BlockedDetail carries the specific refusal code behind a blocked state
	// when there is one, so "upstream artifact unavailable" can say which of
	// missing, incomplete, unreviewed or mislineaged it actually is.
	BlockedDetail  string
	CriterionReady map[domain.CriterionID]bool
	// StaleLineage marks a WorkUnit whose proving lineage is superseded:
	// upstream rework moved a dependency's retained result past the artifact
	// version this unit's current Attempt was admitted to consume. Stale proof
	// is already excluded from the criteria behind State; this is the
	// explanation, and the signal that fresh execution is owed.
	StaleLineage       bool
	StaleLineageDetail string
}

// ScheduleView is derived from one approved current Plan plus canonical proof.
type ScheduleView struct {
	Plan           domain.PlanRevision
	WorkUnits      []WorkUnitScheduleView
	NextRunnableID domain.WorkUnitID
	ActiveAttempt  *domain.Attempt
	// CustodyHeldBy names the unit holding the serial fence, when one does.
	// Serial execution is the launch limit, so saying which unit is holding it
	// is what lets the graph explain why an otherwise-ready branch is waiting.
	CustodyHeldBy domain.WorkUnitID
	// NoRunnableReason is set when nothing can start. Empty means something can.
	NoRunnableReason ScheduleNoRunnableReason
	// LineageStaleness is the single staleness walk derived for this view:
	// the entry flags above are computed from it, and any downstream proof
	// surface (the mission projections evidence links) must filter through
	// this same report rather than re-deriving, so one view is one snapshot.
	LineageStaleness domain.LineageStaleness
}

func attemptActiveForScheduling(status domain.AttemptStatus) bool {
	switch status {
	case domain.AttemptQueued, domain.AttemptRunning, domain.AttemptPaused:
		return true
	default:
		return false
	}
}

// nextRunnableWorkUnit is the pure serial scheduler decision. It uses the same
// criterionReady evaluator as Prove & Close after narrowing facts to the exact
// dependency WorkUnit/Attempt lineage.
func nextRunnableWorkUnit(plan domain.PlanRevision, attempts []domain.Attempt, proof ProofView) (domain.WorkUnit, bool, error) {
	if proof.OutcomeID != plan.OutcomeID {
		return domain.WorkUnit{}, false, fmt.Errorf("scheduler proof outcome %s does not match plan outcome %s", proof.OutcomeID, plan.OutcomeID)
	}
	if proof.Contract.Number != plan.ContractRevisionNumber {
		return domain.WorkUnit{}, false, fmt.Errorf("scheduler proof contract revision %d does not match plan revision binding %d", proof.Contract.Number, plan.ContractRevisionNumber)
	}
	ordered, err := plan.TopologicalWorkUnits()
	if err != nil {
		return domain.WorkUnit{}, false, err
	}
	currentAttempts := attemptsForPlan(plan, attempts)
	for _, attempt := range currentAttempts {
		if attemptActiveForScheduling(attempt.Status) {
			return domain.WorkUnit{}, false, nil
		}
	}

	proven := make(map[domain.WorkUnitID]bool, len(ordered))
	for _, unit := range ordered {
		proven[unit.ID] = workUnitProven(plan, unit, currentAttempts, proof)
	}
	for _, unit := range ordered {
		if proven[unit.ID] {
			continue
		}
		ready := true
		for _, dependency := range unit.DependsOn {
			if !proven[dependency] {
				ready = false
				break
			}
		}
		if ready {
			return unit, true, nil
		}
	}
	return domain.WorkUnit{}, false, nil
}

func attemptsForPlan(plan domain.PlanRevision, attempts []domain.Attempt) []domain.Attempt {
	out := make([]domain.Attempt, 0, len(attempts))
	for _, attempt := range attempts {
		if attempt.OutcomeID != plan.OutcomeID || attempt.PlanRevisionID != plan.ID || attempt.ContractRevisionNumber != plan.ContractRevisionNumber {
			continue
		}
		out = append(out, attempt)
	}
	return out
}

func attemptsForWorkUnit(unitID domain.WorkUnitID, attempts []domain.Attempt) []domain.Attempt {
	out := make([]domain.Attempt, 0)
	for _, attempt := range attempts {
		if attempt.WorkUnitID == unitID {
			out = append(out, attempt)
		}
	}
	return out
}

func workUnitProven(plan domain.PlanRevision, unit domain.WorkUnit, attempts []domain.Attempt, proof ProofView) bool {
	if len(unit.CriterionIDs) == 0 {
		return false
	}
	for _, criterionID := range unit.CriterionIDs {
		criterion, ok := criterionProofForID(proof, criterionID)
		if !ok || criterion.Delegated {
			return false
		}
		scoped := CriterionProofView{Criterion: criterion.Criterion}
		for _, item := range criterion.Evidence {
			if proofFactBelongsToWorkUnit(plan, unit, attempts, item.SubjectType, item.SubjectID, item.SubjectRevision) {
				scoped.Evidence = append(scoped.Evidence, item)
			}
		}
		for _, run := range criterion.Verifications {
			if proofFactBelongsToWorkUnit(plan, unit, attempts, run.SubjectType, run.SubjectID, run.SubjectRevision) {
				scoped.Verifications = append(scoped.Verifications, run)
			}
		}
		ready, _ := criterionReady(scoped, proof.ProofHorizon)
		if !ready {
			return false
		}
	}
	return true
}

func criterionProofForID(proof ProofView, criterionID domain.CriterionID) (CriterionProofView, bool) {
	for _, criterion := range proof.Criteria {
		if criterion.Criterion.ID == criterionID {
			return criterion, true
		}
	}
	return CriterionProofView{}, false
}

func proofFactBelongsToWorkUnit(plan domain.PlanRevision, unit domain.WorkUnit, attempts []domain.Attempt, subjectType domain.ProofSubjectType, subjectID, subjectRevision string) bool {
	switch subjectType {
	case domain.ProofSubjectWorkUnit:
		return subjectID == string(unit.ID) && subjectRevision == string(plan.ID)
	case domain.ProofSubjectAttempt:
		// The artifact version in subjectRevision is deliberately ignored here.
		// Scheduling asks whether the unit is proved at all, so proof naming any
		// attempt of this unit counts, whichever version of its output was
		// examined. Classifying a *particular* attempt is the stricter question
		// attemptProven answers, and that one does bind to the version.
		for _, attempt := range attempts {
			if string(attempt.ID) == subjectID && attempt.WorkUnitID == unit.ID && attempt.PlanRevisionID == plan.ID && attempt.OutcomeID == plan.OutcomeID && attempt.ContractRevisionNumber == plan.ContractRevisionNumber {
				return true
			}
		}
	}
	return false
}

// GetSchedule returns derived scheduler truth for Mission Control. The Plan
// must be approved and bind the current Contract; no frontend lifecycle state
// is invented or persisted.
func (s *Service) GetSchedule(ctx context.Context, outcomeID domain.OutcomeID, planID domain.PlanRevisionID) (ScheduleView, error) {
	outcomeRecord, ok, err := s.store.GetOutcome(ctx, outcomeID)
	if err != nil {
		return ScheduleView{}, err
	}
	if !ok {
		return ScheduleView{}, apierr.NotFound("OUTCOME_NOT_FOUND", "That Outcome does not exist")
	}
	plan, found, err := s.store.GetPlanRevision(ctx, outcomeID, planID)
	if err != nil {
		return ScheduleView{}, err
	}
	if !found {
		return ScheduleView{}, apierr.NotFound("PLAN_NOT_FOUND", "That plan does not exist")
	}
	if plan.Status != domain.PlanStatusApproved {
		return ScheduleView{}, apierr.Conflict(CodePlanNotApproved, "Authorize this plan before scheduling work", map[string]any{"planId": plan.ID})
	}
	if !plan.BindsCurrentContract(outcomeRecord.CurrentRevisionNumber) {
		return ScheduleView{}, apierr.Conflict(CodePlanBriefInvalidated, "This plan no longer binds the current Contract", map[string]any{"planId": plan.ID})
	}
	attempts, err := s.store.ListAttempts(ctx, outcomeID)
	if err != nil {
		return ScheduleView{}, err
	}
	proof, err := s.GetProof(ctx, outcomeID)
	if err != nil {
		return ScheduleView{}, err
	}
	view, err := deriveSchedule(plan, attempts, proof, s.upstreamArtifactBlocks(ctx, plan, attempts))
	if err != nil {
		return ScheduleView{}, err
	}
	view.LineageStaleness = proof.LineageStaleness
	return view, nil
}

// upstreamArtifactBlocks reports, per WorkUnit, why its predecessors' retained
// results cannot be handed down — using exactly the checks admission runs, so
// the graph and Start agree about what can begin.
//
// A store that cannot answer yields no blocks rather than a fabricated one:
// admission stays the authority that refuses, and a read-only projection must
// not invent a blocker it did not establish.
func (s *Service) upstreamArtifactBlocks(ctx context.Context, plan domain.PlanRevision, attempts []domain.Attempt) map[domain.WorkUnitID]string {
	blocks := map[domain.WorkUnitID]string{}
	if s.receipts == nil {
		return blocks
	}
	scoped := attemptsForPlan(plan, attempts)
	lookup := func(id domain.AttemptID) (domain.AttemptReceipt, bool, error) {
		return s.receipts.GetAttemptReceipt(ctx, id)
	}
	for _, unit := range plan.WorkUnits {
		if len(unit.DependsOn) == 0 {
			continue
		}
		_, err := upstreamReceiptsFor(unit, scoped, lookup)
		if err == nil {
			continue
		}
		var api *apierr.Error
		if errors.As(err, &api) && api.Code != CodeUpstreamArtifactMissing {
			// UPSTREAM_ARTIFACT_MISSING already has a truthful graph state:
			// the dependency is simply not proved yet, which the dependency
			// walk below reports as awaiting proof.
			blocks[unit.ID] = api.Code
		}
	}
	return blocks
}

func deriveSchedule(plan domain.PlanRevision, attempts []domain.Attempt, proof ProofView, artifactBlocks map[domain.WorkUnitID]string) (ScheduleView, error) {
	ordered, err := plan.TopologicalWorkUnits()
	if err != nil {
		return ScheduleView{}, err
	}
	currentAttempts := attemptsForPlan(plan, attempts)
	view := ScheduleView{Plan: plan}
	var active *domain.Attempt
	for i := range currentAttempts {
		if attemptActiveForScheduling(currentAttempts[i].Status) {
			attemptCopy := currentAttempts[i]
			active = &attemptCopy
			break
		}
	}
	view.ActiveAttempt = active

	proven := make(map[domain.WorkUnitID]bool, len(ordered))
	for _, unit := range ordered {
		proven[unit.ID] = workUnitProven(plan, unit, currentAttempts, proof)
	}
	for _, unit := range ordered {
		entry := WorkUnitScheduleView{WorkUnit: unit, Attempts: attemptsForWorkUnit(unit.ID, currentAttempts), CriterionReady: map[domain.CriterionID]bool{}}
		if facts := proof.LineageStaleness.WorkUnitStaleFacts(unit.ID); len(facts) > 0 {
			entry.StaleLineage = true
			entry.StaleLineageDetail = staleLineageDetail(facts)
		}
		for _, criterionID := range unit.CriterionIDs {
			criterion, ok := criterionProofForID(proof, criterionID)
			if !ok {
				entry.CriterionReady[criterionID] = false
				continue
			}
			scoped := CriterionProofView{Criterion: criterion.Criterion}
			for _, item := range criterion.Evidence {
				if proofFactBelongsToWorkUnit(plan, unit, currentAttempts, item.SubjectType, item.SubjectID, item.SubjectRevision) {
					scoped.Evidence = append(scoped.Evidence, item)
				}
			}
			for _, run := range criterion.Verifications {
				if proofFactBelongsToWorkUnit(plan, unit, currentAttempts, run.SubjectType, run.SubjectID, run.SubjectRevision) {
					scoped.Verifications = append(scoped.Verifications, run)
				}
			}
			ready, _ := criterionReady(scoped, proof.ProofHorizon)
			entry.CriterionReady[criterionID] = ready
		}
		switch {
		case proven[unit.ID]:
			entry.State = WorkUnitScheduleProven
		case active != nil && active.WorkUnitID == unit.ID:
			// This unit's own attempt holds custody. Paused and unaccounted
			// are reported as themselves rather than as "executing", because
			// neither is making progress and both need the owner.
			if active.Status == domain.AttemptPaused {
				entry.State = WorkUnitSchedulePaused
			} else {
				entry.State = WorkUnitScheduleExecuting
			}
		default:
			for _, dependency := range unit.DependsOn {
				if !proven[dependency] {
					entry.BlockingDependencies = append(entry.BlockingDependencies, dependency)
				}
			}
			switch {
			case len(entry.BlockingDependencies) > 0:
				entry.State, entry.BlockedReason = WorkUnitScheduleBlocked, BlockedAwaitingDependencyProof
			case artifactBlocks[unit.ID] != "":
				// Every dependency is proved, yet its output cannot be given
				// to this unit. Reporting it runnable would offer a Start that
				// admission is certain to refuse.
				entry.State, entry.BlockedReason = WorkUnitScheduleBlocked, BlockedUpstreamArtifactUnavailable
				entry.BlockedDetail = artifactBlocks[unit.ID]
			case active != nil:
				// Ready on its own terms, waiting only for the serial fence.
				entry.State, entry.BlockedReason = WorkUnitScheduleBlocked, BlockedCustodyHeld
			case len(entry.Attempts) > 0:
				entry.State = WorkUnitScheduleRetryable
				if view.NextRunnableID.IsZero() {
					view.NextRunnableID = unit.ID
				}
			default:
				entry.State = WorkUnitScheduleRunnable
				if view.NextRunnableID.IsZero() {
					view.NextRunnableID = unit.ID
				}
			}
		}
		view.WorkUnits = append(view.WorkUnits, entry)
	}
	if active != nil {
		view.CustodyHeldBy = active.WorkUnitID
	}
	view.NoRunnableReason = noRunnableReason(view, active)
	return view, nil
}

// noRunnableReason explains an empty runnable set, so the Mission can name the
// owner's next move instead of showing an indefinite spinner.
func noRunnableReason(view ScheduleView, active *domain.Attempt) ScheduleNoRunnableReason {
	if !view.NextRunnableID.IsZero() {
		return ""
	}
	if active != nil {
		if active.Status == domain.AttemptPaused {
			return NoRunnablePaused
		}
		return NoRunnableExecuting
	}
	for _, entry := range view.WorkUnits {
		if entry.State != WorkUnitScheduleProven {
			// Nothing is executing and nothing is admissible, so the remaining
			// work is waiting on proof the owner has to record or repair.
			return NoRunnableAwaitingProof
		}
	}
	return NoRunnableAllProven
}

// selectWorkUnitForAttempt enforces the serial scheduler decision. A zero
// request means the daemon selects its derived next runnable unit; a named
// request remains an assertion that must match that same decision.
func (s *Service) selectWorkUnitForAttempt(ctx context.Context, outcomeID domain.OutcomeID, plan domain.PlanRevision, requested domain.WorkUnitID) (domain.WorkUnit, error) {
	attempts, err := s.store.ListAttempts(ctx, outcomeID)
	if err != nil {
		return domain.WorkUnit{}, err
	}
	proof, err := s.GetProof(ctx, outcomeID)
	if err != nil {
		return domain.WorkUnit{}, err
	}
	next, ok, err := nextRunnableWorkUnit(plan, attempts, proof)
	if err != nil {
		return domain.WorkUnit{}, err
	}
	if !ok {
		return domain.WorkUnit{}, apierr.Conflict(CodeNoRunnableWorkUnit, "No WorkUnit is runnable until the active work or required proof is resolved", map[string]any{"planId": plan.ID})
	}
	if requested.IsZero() {
		return next, nil
	}
	if next.ID != requested {
		return domain.WorkUnit{}, apierr.Conflict(CodeWorkUnitNotRunnable, "That WorkUnit is not the next dependency-ready unit", map[string]any{"requestedWorkUnitId": requested, "nextRunnableWorkUnitId": next.ID})
	}
	return next, nil
}

// staleLineageDetail summarizes the first supersession fact for surfaces that
// show one line; the full fact list stays available through the proof view.
func staleLineageDetail(facts []domain.LineageStaleFact) string {
	first := facts[0]
	return fmt.Sprintf("dependency %s result moved from artifact version %s to %s; re-execution is owed", first.DependencyUnitID, first.AdmittedVersion, first.CurrentVersion)
}
