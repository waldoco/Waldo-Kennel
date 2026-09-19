package outcome

import (
	"context"
	"fmt"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// lineageStaleness derives the dependency staleness walk for one Outcome from
// sealed input manifests and durable receipts. The custody stores are
// optional wiring: absent means no staleness can be established, which
// reports as none rather than being fabricated - admission and the retention
// path stay the refusing authorities. Read errors fail closed: proof and
// scheduling must never count evidence whose currency could not be checked.
func (s *Service) lineageStaleness(ctx context.Context, outcomeID domain.OutcomeID, attempts []domain.Attempt) (domain.LineageStaleness, error) {
	merged := domain.LineageStaleness{StaleAttempts: map[domain.AttemptID][]domain.LineageStaleFact{}, StaleWorkUnits: map[domain.WorkUnitID][]domain.LineageStaleFact{}}
	if s.manifests == nil || s.receipts == nil {
		return merged, nil
	}
	manifests, err := s.manifests.ListAttemptManifestsForOutcome(ctx, outcomeID)
	if err != nil {
		return merged, fmt.Errorf("list attempt manifests for staleness walk: %w", err)
	}
	inputRefs := map[domain.AttemptID][]domain.AttemptManifestInputRef{}
	for _, manifest := range manifests {
		if manifest.Half != domain.AttemptManifestInput {
			continue
		}
		body, err := manifest.DecodeInput()
		if err != nil {
			return merged, fmt.Errorf("decode input manifest %s: %w", manifest.AttemptID, err)
		}
		inputRefs[manifest.AttemptID] = body.Inputs
	}
	receipts := map[domain.AttemptID]domain.AttemptReceipt{}
	for _, attempt := range attempts {
		receipt, found, err := s.receipts.GetAttemptReceipt(ctx, attempt.ID)
		if err != nil {
			return merged, fmt.Errorf("read attempt receipt for staleness walk: %w", err)
		}
		if found {
			receipts[attempt.ID] = receipt
		}
	}
	planIDs := map[domain.PlanRevisionID]bool{}
	for _, attempt := range attempts {
		planIDs[attempt.PlanRevisionID] = true
	}
	for planID := range planIDs {
		plan, found, err := s.store.GetPlanRevision(ctx, outcomeID, planID)
		if err != nil {
			return merged, err
		}
		if !found {
			continue
		}
		derived, err := domain.DeriveLineageStaleness(plan, attempts, inputRefs, receipts)
		if err != nil {
			return merged, fmt.Errorf("derive lineage staleness for plan %s: %w", planID, err)
		}
		for attemptID, facts := range derived.StaleAttempts {
			merged.StaleAttempts[attemptID] = facts
		}
		for unitID, facts := range derived.StaleWorkUnits {
			merged.StaleWorkUnits[unitID] = facts
		}
	}
	return merged, nil
}

// staleProofSubject reports whether one evidence or verification subject is a
// stale Attempt. Only Attempt subjects can go stale; Contract, Plan, Outcome
// and WorkUnit subjects name the durable objects themselves.
func staleProofSubject(staleness domain.LineageStaleness, subjectType domain.ProofSubjectType, subjectID string) bool {
	if subjectType != domain.ProofSubjectAttempt {
		return false
	}
	return staleness.AttemptStale(domain.AttemptID(subjectID))
}
