package outcome

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// WithAttemptManifests wires the sealed custody-manifest store. It is
// optional for the same reason receipts are: a degraded profile must keep
// scheduling and reporting truthfully, but without it custody lineage is
// unavailable rather than fabricated.
func (s *Service) WithAttemptManifests(manifests ports.AttemptManifestStore) *Service {
	s.manifests = manifests
	return s
}

// sealAttemptInputManifest writes the input half of an Attempt's custody
// record: the exact lineage, frozen digests, predecessor artifact versions,
// approved documents, and authorized checks the Attempt was admitted with.
// It runs inside the mandatory provider-launch crash boundary. The record is
// insert-once: an identical re-seal (a boundary replay) is accepted, and a
// divergent one is refused, exactly like the output half.
func (s *Service) sealAttemptInputManifest(ctx context.Context, outcomeID domain.OutcomeID, plan domain.PlanRevision, unit domain.WorkUnit, attempt domain.Attempt, session domain.SessionRecord, inputs []ports.AttemptInputRef, documents *ports.AttemptDocumentInputs, coreDigest, compiledDigest, policyDigest string) error {
	if s.manifests == nil {
		return nil
	}
	kind := domain.WorkspaceStagedFolder
	if strings.TrimSpace(session.Metadata.DiffBaseSHA) != "" || strings.TrimSpace(session.Metadata.WorkspaceRepoPath) != "" {
		kind = domain.WorkspaceGitWorktree
	}
	refs := make([]domain.AttemptManifestInputRef, 0, len(inputs))
	for _, input := range inputs {
		refs = append(refs, domain.AttemptManifestInputRef{AttemptID: input.AttemptID, WorkUnitID: input.WorkUnitID, ArtifactVersion: input.ArtifactVersion})
	}
	var docs *domain.AttemptManifestDocument
	if documents != nil {
		docs = &domain.AttemptManifestDocument{ContextID: documents.ContextID, Revision: documents.Revision, Digest: documents.Digest}
	}
	checks := make([]domain.AttemptManifestCheck, 0, len(unit.Checks))
	for _, check := range unit.Checks {
		checks = append(checks, domain.AttemptManifestCheck{ID: check.ID, CriterionID: check.CriterionID, Argv: check.Argv, TimeoutSeconds: check.TimeoutSeconds})
	}
	manifest, err := domain.NewAttemptInputManifest(domain.AttemptInputManifest{
		AttemptID: attempt.ID, OutcomeID: outcomeID, PlanRevisionID: plan.ID, WorkUnitID: unit.ID,
		ContractRevisionNumber: plan.ContractRevisionNumber,
		WorkspaceKind:          kind, BaseRevision: session.Metadata.DiffBaseSHA, BaseRef: session.Metadata.DiffBaseRef,
		RunBriefCoreDigest: coreDigest, RunBriefCompiledDigest: compiledDigest, ExecutionPolicyDigest: policyDigest,
		Inputs: refs, Documents: docs, Checks: checks,
	}, s.clock())
	if err != nil {
		return fmt.Errorf("seal input custody manifest: %w", err)
	}
	if existing, found, err := s.manifests.GetAttemptManifest(ctx, attempt.ID, domain.AttemptManifestInput); err != nil {
		return fmt.Errorf("read input custody manifest for %s: %w", attempt.ID, err)
	} else if found {
		if existing.PayloadDigest != manifest.PayloadDigest {
			return fmt.Errorf("input custody manifest for %s already sealed with different content", attempt.ID)
		}
		return nil
	}
	if err := s.manifests.SaveAttemptManifest(ctx, manifest); err != nil {
		if errors.Is(err, ports.ErrAttemptManifestSealed) {
			if existing, found, readErr := s.manifests.GetAttemptManifest(ctx, attempt.ID, domain.AttemptManifestInput); readErr == nil && found && existing.PayloadDigest == manifest.PayloadDigest {
				return nil
			}
			return fmt.Errorf("input custody manifest for %s already sealed with different content: %w", attempt.ID, err)
		}
		return fmt.Errorf("persist input custody manifest: %w", err)
	}
	return nil
}
