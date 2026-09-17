package outcome

import (
	"context"
	"fmt"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apierr"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// Stable refusals for a successor that cannot be given its predecessors' work.
const (
	// CodeUpstreamArtifactMissing means a dependency produced nothing that was
	// retained, so there is nothing to hand down.
	CodeUpstreamArtifactMissing = "WORK_UNIT_INPUT_ARTIFACT_MISSING"
	// CodeUpstreamArtifactIncomplete means a dependency's snapshot hit a bound
	// or held something it could not represent.
	CodeUpstreamArtifactIncomplete = "WORK_UNIT_INPUT_ARTIFACT_MISSING"
	// CodeUpstreamArtifactUnreviewed means a dependency's result was never
	// frozen, so what it holds can still change under the successor.
	CodeUpstreamArtifactUnreviewed = "WORK_UNIT_INPUT_NOT_RETAINED"
	// CodeUpstreamLineageMismatch means a retained receipt does not belong to
	// the attempt or plan the successor is being admitted under.
	CodeUpstreamLineageMismatch = "WORK_UNIT_INPUT_LINEAGE_MISMATCH"
)

// resolveUpstreamReceipts returns, in dependency order, the exact retained
// results a successor must be given before it may start.
//
// This is the admission half of artifact continuity. Starting a successor from
// a fresh worktree off the original branch is not a handoff: the predecessor's
// work would simply be absent, and the successor would redo or contradict it
// without anyone being told. So a dependency that has no retained, complete,
// frozen result blocks admission with a named reason rather than being started
// on an empty workspace.
//
// A unit with no dependencies resolves to nothing, which is not an error.
func (s *Service) resolveUpstreamReceipts(
	ctx context.Context,
	plan domain.PlanRevision,
	unit domain.WorkUnit,
) ([]domain.AttemptReceipt, error) {
	if len(unit.DependsOn) == 0 {
		return nil, nil
	}
	if s.receipts == nil {
		// Refusing is the only truthful option: the successor's inputs cannot be
		// established at all, so admitting it would start work on an unknown base.
		return nil, apierr.Conflict(CodeUpstreamArtifactMissing,
			"Artifact retention is unavailable, so this WorkUnit's inputs cannot be established",
			map[string]any{"workUnitId": unit.ID})
	}
	attempts, err := s.store.ListAttempts(ctx, plan.OutcomeID)
	if err != nil {
		return nil, err
	}
	return upstreamReceiptsFor(unit, attemptsForPlan(plan, attempts), func(id domain.AttemptID) (domain.AttemptReceipt, bool, error) {
		return s.receipts.GetAttemptReceipt(ctx, id)
	})
}

// upstreamReceiptsFor is the decision itself, separated from the two store
// reads so every refusal is exercised directly rather than through a fake of
// the whole Outcome store.
func upstreamReceiptsFor(
	unit domain.WorkUnit,
	scoped []domain.Attempt,
	lookup func(domain.AttemptID) (domain.AttemptReceipt, bool, error),
) ([]domain.AttemptReceipt, error) {
	receipts := make([]domain.AttemptReceipt, 0, len(unit.DependsOn))
	for _, dependencyID := range unit.DependsOn {
		producer, ok := producingAttempt(dependencyID, scoped)
		succeeded := 0
		for _, candidate := range attemptsForWorkUnit(dependencyID, scoped) {
			if candidate.Status == domain.AttemptSucceeded {
				succeeded++
			}
		}
		if succeeded > 1 {
			return nil, apierr.Conflict(CodeUpstreamLineageMismatch, "More than one succeeded predecessor lineage exists; select an exact producer before admission", map[string]any{"workUnitId": unit.ID, "dependencyId": dependencyID})
		}
		if !ok {
			return nil, apierr.Conflict(CodeUpstreamArtifactMissing,
				"A WorkUnit this one depends on has not produced a proved result yet",
				map[string]any{"workUnitId": unit.ID, "dependencyId": dependencyID})
		}
		receipt, found, err := lookup(producer.ID)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, apierr.Conflict(CodeUpstreamArtifactMissing,
				"The Attempt that satisfied a dependency retained no result",
				map[string]any{"workUnitId": unit.ID, "dependencyId": dependencyID, "attemptId": producer.ID})
		}
		if err := upstreamReceiptUsable(unit, dependencyID, producer, receipt); err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	return receipts, nil
}

// producingAttempt finds the succeeded attempt of a dependency WorkUnit.
//
// Success is the only status that may hand work down: it is the one status
// derived from retained bytes plus proof about those bytes. A reconciled
// attempt has ended without being classified, and its output may still change.
func producingAttempt(unitID domain.WorkUnitID, scoped []domain.Attempt) (domain.Attempt, bool) {
	var producer domain.Attempt
	found := false
	for _, attempt := range attemptsForWorkUnit(unitID, scoped) {
		if attempt.Status != domain.AttemptSucceeded {
			continue
		}
		if found {
			return domain.Attempt{}, false
		}
		producer, found = attempt, true
	}
	return producer, found
}

// upstreamReceiptUsable refuses a receipt that cannot honestly be handed down.
func upstreamReceiptUsable(unit domain.WorkUnit, dependencyID domain.WorkUnitID, producer domain.Attempt, receipt domain.AttemptReceipt) error {
	detail := map[string]any{
		"workUnitId": unit.ID, "dependencyId": dependencyID,
		"attemptId": producer.ID, "artifactVersion": receipt.ArtifactVersion,
	}
	if err := receipt.Validate(); err != nil {
		detail["detail"] = err.Error()
		return apierr.Conflict(CodeUpstreamLineageMismatch, "A predecessor's retained result is not a valid receipt", detail)
	}
	// The receipt must describe the attempt that actually produced it. A
	// mismatch means the successor would inherit somebody else's work.
	if receipt.AttemptID != producer.ID || receipt.WorkUnitID != dependencyID ||
		receipt.PlanRevisionID != producer.PlanRevisionID ||
		receipt.ContractRevisionNumber != producer.ContractRevisionNumber {
		return apierr.Conflict(CodeUpstreamLineageMismatch,
			"A predecessor's retained result does not belong to the Attempt that satisfied the dependency", detail)
	}
	if !receipt.RetentionState.Complete() {
		detail["retentionState"] = string(receipt.RetentionState)
		detail["retentionDetail"] = receipt.RetentionDetail
		return apierr.Conflict(CodeUpstreamArtifactIncomplete,
			fmt.Sprintf("A predecessor's result was retained as %s, so it cannot be handed to this WorkUnit", receipt.RetentionState), detail)
	}
	if !receipt.Frozen() {
		// An unfrozen receipt can still be replaced by a later retention pass,
		// so the successor could be built on bytes that change underneath it.
		return apierr.Conflict(CodeUpstreamArtifactUnreviewed,
			"A predecessor's result is not frozen yet, so it could still change under this WorkUnit", detail)
	}
	return nil
}

// CodeUpstreamMaterializationFailed reports that validated predecessor results
// could not be placed in the successor's workspace. No provider was launched.
const CodeUpstreamMaterializationFailed = "UPSTREAM_MATERIALIZATION_FAILED"

// admittedInputsFor resolves the exact retained predecessor results this
// WorkUnit may consume, as immutable references.
//
// Pinning the artifact version here — not just the producing Attempt — is what
// makes the handoff replayable. Admission authorized these bytes; a restart or
// a later retry must materialize the same ones, even if the upstream WorkUnit
// has since produced something newer.
func (s *Service) admittedInputsFor(ctx context.Context, plan domain.PlanRevision, unit domain.WorkUnit) ([]ports.AttemptInputRef, error) {
	receipts, err := s.resolveUpstreamReceipts(ctx, plan, unit)
	if err != nil {
		return nil, err
	}
	required := make(map[domain.WorkUnitID]string, len(unit.Inputs))
	for _, input := range unit.Inputs {
		required[input.FromWorkUnitID] = input.Required
	}
	inputs := make([]ports.AttemptInputRef, 0, len(receipts))
	for _, receipt := range receipts {
		inputs = append(inputs, ports.AttemptInputRef{
			AttemptID: receipt.AttemptID, WorkUnitID: receipt.WorkUnitID, ArtifactVersion: receipt.ArtifactVersion, Required: required[receipt.WorkUnitID],
		})
	}
	return inputs, nil
}

// inputArtifactVersions is the ordered version list recorded on the admission
// snapshot, so what a successor consumed stays inspectable after the fact.
func inputArtifactVersions(inputs []ports.AttemptInputRef) []string {
	versions := make([]string, 0, len(inputs))
	for _, input := range inputs {
		versions = append(versions, input.ArtifactVersion)
	}
	return versions
}
func inputDigests(inputs []ports.AttemptInputRef) []string {
	values := make([]string, 0, len(inputs))
	for _, input := range inputs {
		values = append(values, input.ArtifactVersion+"@"+input.WorkUnitID.String()+"@"+input.Required)
	}
	return values
}

// materializationFailed converts a provisioning failure into a refusal that
// says a provider was NOT launched.
//
// Provisioning runs before any process exists, so this is a known failure, not
// an ambiguous start. Reporting it as unresolved would send the owner to
// reconcile a run that never began; reporting it as an ordinary conflict would
// hide that a workspace may hold partial inputs.
func materializationFailed(unit domain.WorkUnit, attemptID domain.AttemptID, cause error) *apierr.Error {
	return apierr.Conflict(CodeUpstreamMaterializationFailed,
		"This WorkUnit's inputs could not be provisioned, so nothing was started",
		map[string]any{"workUnitId": unit.ID, "attemptId": attemptID, "detail": cause.Error()})
}
