package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/artifactstore"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	sessionmanager "github.com/Pin4sf/Waldo-Kennel/backend/internal/session_manager"
)

// attemptLivenessInterval is how often the daemon reconcile hook re-evaluates
// running attempts against their bound session's heartbeat facts.
const attemptLivenessInterval = 15 * time.Second

type projectConfigSource interface {
	GetProject(ctx context.Context, id string) (domain.ProjectRecord, bool, error)
}

// attemptSessionControl is the subordinate runtime surface used by Outcome
// execution. SpawnExactAttempt is intentionally distinct from ordinary Spawn:
// approved WorkUnits have frozen provider/model semantics that mutable Project
// preferences are not allowed to reinterpret.
type attemptSessionControl interface {
	SpawnExactAttempt(ctx context.Context, cfg ports.SpawnConfig, binding domain.ExecutionBinding) (domain.Session, int, int, error)
	Kill(ctx context.Context, id domain.SessionID) (bool, error)
	Get(ctx context.Context, id domain.SessionID) (domain.Session, error)
}

type attemptRetentionSource interface {
	ports.AttemptReceiptStore
	LatestAttemptSessionRef(context.Context, domain.AttemptID) (domain.AttemptSessionRef, bool, error)
}

type attemptSpawner struct {
	sessions  attemptSessionControl
	projects  projectConfigSource
	agents    ports.AgentResolver
	retention ports.AttemptRetainer
}

// RetainAttempt resolves the latest daemon-owned session binding and captures
// its workspace. It never accepts a path from the client or from provider
// prose. Existing complete receipts are checked for durable blobs so a daemon
// restart can safely retry the database half of publication.
func (a attemptSpawner) RetainAttempt(ctx context.Context, attempt domain.Attempt) error {
	if a.retention == nil {
		return fmt.Errorf("attempt artifact retention is not configured")
	}
	return a.retention.RetainAttempt(ctx, attempt)
}

type attemptArtifactRetainer struct {
	sessions  attemptSessionControl
	refs      attemptRetentionSource
	artifacts *artifactstore.Store
	// manifests seals the output half of the Attempt's custody record once
	// the receipt is durable. Nil only in test wiring; the daemon always
	// wires it.
	manifests ports.AttemptManifestStore
	// fences records the typed custody-close fence: the provider had exited
	// and the workspace had settled before the snapshot. Nil only in test
	// wiring; the daemon always wires it.
	fences ports.AttemptCustodyFenceStore
}

var _ ports.AttemptRetainer = (*attemptArtifactRetainer)(nil)

func (r *attemptArtifactRetainer) RetainAttempt(ctx context.Context, attempt domain.Attempt) error {
	if r == nil || r.sessions == nil || r.refs == nil || r.artifacts == nil {
		return fmt.Errorf("attempt artifact retainer is not fully wired")
	}
	if existing, ok, err := r.refs.GetAttemptReceipt(ctx, attempt.ID); err != nil {
		return err
	} else if ok && existing.RetentionState.Complete() {
		for _, file := range existing.Files {
			if file.ChangeKind == domain.ArtifactDeleted {
				continue
			}
			if _, _, err := r.artifacts.Read(ctx, existing, file); err != nil {
				return fmt.Errorf("verify retained blob %s: %w", file.RelativePath, err)
			}
		}
		return r.sealOutputManifest(ctx, existing)
	}
	ref, found, err := r.refs.LatestAttemptSessionRef(ctx, attempt.ID)
	if err != nil {
		return fmt.Errorf("read session binding: %w", err)
	}
	if !found || strings.TrimSpace(ref.SessionID) == "" {
		return fmt.Errorf("attempt %s has no daemon-owned session binding", attempt.ID)
	}
	session, err := r.sessions.Get(ctx, domain.SessionID(ref.SessionID))
	if err != nil {
		return fmt.Errorf("read session %s: %w", ref.SessionID, err)
	}
	// The settle fence precedes the snapshot: no tree is captured while the
	// provider that wrote it may still be alive.
	if err := r.settleCustodyFence(ctx, attempt, session); err != nil {
		return err
	}
	kind := domain.WorkspaceStagedFolder
	if strings.TrimSpace(session.Metadata.DiffBaseSHA) != "" || strings.TrimSpace(session.Metadata.WorkspaceRepoPath) != "" {
		kind = domain.WorkspaceGitWorktree
	}
	result, err := r.artifacts.Retain(ctx, artifactstore.Input{
		AttemptID: attempt.ID, OutcomeID: attempt.OutcomeID, PlanRevisionID: attempt.PlanRevisionID, WorkUnitID: attempt.WorkUnitID,
		ContractRevisionNumber: attempt.ContractRevisionNumber, WorkspaceKind: kind, WorkspacePath: session.Metadata.WorkspacePath,
		RepositoryPath: session.Metadata.WorkspaceRepoPath, BaseRevision: session.Metadata.DiffBaseSHA, BaseRef: session.Metadata.DiffBaseRef,
		TerminationReason: string(attempt.Status),
	})
	if err != nil {
		return err
	}
	if err := r.refs.SaveAttemptReceipt(ctx, result.Receipt); err != nil {
		return fmt.Errorf("persist retained receipt: %w", err)
	}
	return r.sealOutputManifest(ctx, result.Receipt)
}

// settleCustodyFence records the typed custody-close fence exactly once: the
// bound provider session has exited, so the workspace it wrote is settled and
// safe to snapshot. A live session is refused - retaining a tree its writer
// can still change would make the snapshot, and every check bound to it,
// unattributable. A restart replay with the same binding is a no-op; a fence
// recorded against a different session is refused.
func (r *attemptArtifactRetainer) settleCustodyFence(ctx context.Context, attempt domain.Attempt, session domain.Session) error {
	if r.fences == nil {
		return nil
	}
	if !session.IsTerminated {
		return fmt.Errorf("attempt %s custody close refused: provider session %s has not exited", attempt.ID, session.ID)
	}
	fence := domain.AttemptCustodyFence{
		AttemptID: attempt.ID,
		SessionID: string(session.ID),
		Detail:    "provider session terminated; workspace settled before snapshot",
		FencedAt:  time.Now().UTC(),
	}
	if existing, found, err := r.fences.GetAttemptCustodyFence(ctx, attempt.ID); err != nil {
		return fmt.Errorf("read custody fence for %s: %w", attempt.ID, err)
	} else if found {
		if existing.SessionID != fence.SessionID {
			return fmt.Errorf("custody fence for %s already recorded against session %s, not %s", attempt.ID, existing.SessionID, fence.SessionID)
		}
		return nil
	}
	if err := r.fences.RecordAttemptCustodyFence(ctx, fence); err != nil {
		if errors.Is(err, ports.ErrAttemptCustodyFenceSealed) {
			if existing, found, readErr := r.fences.GetAttemptCustodyFence(ctx, attempt.ID); readErr == nil && found && existing.SessionID == fence.SessionID {
				return nil
			}
			return fmt.Errorf("custody fence for %s already recorded with different content: %w", attempt.ID, err)
		}
		return fmt.Errorf("record custody fence for %s: %w", attempt.ID, err)
	}
	return nil
}

// sealOutputManifest writes the output half of the Attempt's custody record,
// bound to the receipt's immutable artifact version. It is idempotent: a
// daemon restart that finds the receipt durable but the manifest absent
// re-seals from the receipt, and a sealed half with identical content is
// accepted, with different content refused.
func (r *attemptArtifactRetainer) sealOutputManifest(ctx context.Context, receipt domain.AttemptReceipt) error {
	if r.manifests == nil {
		return nil
	}
	manifest, err := domain.NewAttemptOutputManifest(domain.AttemptOutputManifest{
		AttemptID: receipt.AttemptID, OutcomeID: receipt.OutcomeID, PlanRevisionID: receipt.PlanRevisionID,
		WorkUnitID: receipt.WorkUnitID, ContractRevisionNumber: receipt.ContractRevisionNumber,
		ArtifactVersion: receipt.ArtifactVersion, ResultRevision: receipt.ResultRevision,
		RetentionState: receipt.RetentionState, RetentionDetail: receipt.RetentionDetail,
		TerminationReason: receipt.TerminationReason, ObservedAt: receipt.ObservedAt,
	}, receipt.ObservedAt)
	if err != nil {
		return fmt.Errorf("seal output custody manifest for %s: %w", receipt.AttemptID, err)
	}
	if existing, found, err := r.manifests.GetAttemptManifest(ctx, receipt.AttemptID, domain.AttemptManifestOutput); err != nil {
		return fmt.Errorf("read output custody manifest for %s: %w", receipt.AttemptID, err)
	} else if found {
		if existing.PayloadDigest != manifest.PayloadDigest {
			return fmt.Errorf("output custody manifest for %s already sealed with different content", receipt.AttemptID)
		}
		return nil
	}
	if err := r.manifests.SaveAttemptManifest(ctx, manifest); err != nil {
		if errors.Is(err, ports.ErrAttemptManifestSealed) {
			existing, found, readErr := r.manifests.GetAttemptManifest(ctx, receipt.AttemptID, domain.AttemptManifestOutput)
			if readErr == nil && found && existing.PayloadDigest == manifest.PayloadDigest {
				return nil
			}
			return fmt.Errorf("output custody manifest for %s already sealed with different content: %w", receipt.AttemptID, err)
		}
		return fmt.Errorf("persist output custody manifest for %s: %w", receipt.AttemptID, err)
	}
	return nil
}

// attemptInputProvisioner materializes the exact retained predecessor results
// a successor was admitted with.
//
// It resolves each input by Attempt identity *and* artifact version, so a
// retained result that has since been replaced is refused rather than
// substituted. That is the whole point of recording versions at admission: the
// successor must receive the bytes the owner's approved schedule authorized,
// not whatever the upstream WorkUnit happens to hold now.
type attemptInputProvisioner struct {
	receipts  ports.AttemptReceiptStore
	artifacts *artifactstore.Store
	// contexts resolves an approved supplied-document selection. Absent means
	// document Outcomes cannot be staged, which is refused rather than
	// approximated with the owner's live files.
	contexts ports.DocumentContextStore
}

var _ ports.AttemptInputProvisioner = (*attemptInputProvisioner)(nil)

func (p *attemptInputProvisioner) ProvisionAttemptInputs(ctx context.Context, req ports.AttemptInputProvisionRequest) error {
	if p == nil || p.receipts == nil || p.artifacts == nil {
		return fmt.Errorf("%w: artifact retention is not wired", ports.ErrAttemptInputProvisioning)
	}
	if len(req.Inputs) == 0 && req.Documents == nil {
		return nil
	}
	if req.Documents != nil {
		if err := p.provisionDocuments(req); err != nil {
			return err
		}
	}
	if len(req.Inputs) == 0 {
		return nil
	}
	receipts := make([]domain.AttemptReceipt, 0, len(req.Inputs))
	for _, input := range req.Inputs {
		receipt, found, err := p.receipts.GetAttemptReceipt(ctx, input.AttemptID)
		if err != nil {
			return fmt.Errorf("%w: read retained result for %s: %w", ports.ErrAttemptInputProvisioning, input.AttemptID, err)
		}
		if !found {
			return fmt.Errorf("%w: attempt %s retained no result", ports.ErrAttemptInputProvisioning, input.AttemptID)
		}
		if receipt.ArtifactVersion != input.ArtifactVersion || receipt.WorkUnitID != input.WorkUnitID {
			return fmt.Errorf("%w: attempt %s now holds artifact %s for %s, not the admitted %s for %s",
				ports.ErrAttemptInputProvisioning, input.AttemptID, receipt.ArtifactVersion, receipt.WorkUnitID,
				input.ArtifactVersion, input.WorkUnitID)
		}
		if !receipt.Frozen() {
			return fmt.Errorf("%w: attempt %s result is not frozen and could still change", ports.ErrAttemptInputProvisioning, input.AttemptID)
		}
		receipts = append(receipts, receipt)
	}
	// Compose reads every blob and verifies its digest and mode, so a corrupt
	// or missing artifact fails here rather than reaching the workspace.
	handoff, err := p.artifacts.Compose(ctx, receipts)
	if err != nil {
		return fmt.Errorf("%w: %w", ports.ErrAttemptInputProvisioning, err)
	}
	if handoff.WorkspaceKind != req.WorkspaceKind {
		return fmt.Errorf("%w: predecessors worked in a %s but this successor has a %s",
			ports.ErrAttemptInputProvisioning, handoff.WorkspaceKind, req.WorkspaceKind)
	}
	if err := p.artifacts.Materialize(ctx, handoff, req.WorkspacePath, req.BaseRevision); err != nil {
		return fmt.Errorf("%w: %w", ports.ErrAttemptInputProvisioning, err)
	}
	return nil
}

// provisionDocuments stages the approved supplied-document snapshot.
//
// It reads the snapshot the owner approved, never their original files: an
// Outcome must not come to mean something different because a document was
// edited after approval. The digest recorded at admission is re-checked here,
// so a snapshot that is not the one authorized refuses rather than runs.
func (p *attemptInputProvisioner) provisionDocuments(req ports.AttemptInputProvisionRequest) error {
	if p.contexts == nil {
		return fmt.Errorf("%w: supplied-document context is not wired", ports.ErrAttemptInputProvisioning)
	}
	selection, found, err := p.contexts.GetDocumentContext(context.Background(), req.Documents.ContextID)
	if err != nil {
		return fmt.Errorf("%w: read approved documents: %w", ports.ErrAttemptInputProvisioning, err)
	}
	if !found {
		return fmt.Errorf("%w: approved document context %s is missing", ports.ErrAttemptInputProvisioning, req.Documents.ContextID)
	}
	if !selection.Approved() {
		return fmt.Errorf("%w: document context %s is not approved", ports.ErrAttemptInputProvisioning, selection.ID)
	}
	if selection.Digest != req.Documents.Digest || selection.Revision != req.Documents.Revision {
		return fmt.Errorf("%w: document context %s is revision %d/%s, not the admitted %d/%s",
			ports.ErrAttemptInputProvisioning, selection.ID, selection.Revision, selection.Digest,
			req.Documents.Revision, req.Documents.Digest)
	}
	handoff, err := p.artifacts.DocumentHandoff(selection.ID, selection.Sources)
	if err != nil {
		return fmt.Errorf("%w: %w", ports.ErrAttemptInputProvisioning, err)
	}
	// A staged folder has no revisions, so no base is claimed for it.
	if err := p.artifacts.Materialize(context.Background(), handoff, req.WorkspacePath, ""); err != nil {
		return fmt.Errorf("%w: %w", ports.ErrAttemptInputProvisioning, err)
	}
	return nil
}

var _ ports.AttemptSessionSpawner = attemptSpawner{}

func (a attemptSpawner) ProfileReadiness(ctx context.Context, projectID domain.ProjectID, binding domain.ExecutionBinding, policy *domain.AttemptExecutionPolicy) (ports.AgentProfileReadiness, error) {
	if a.projects == nil || a.agents == nil {
		return ports.AgentProfileReadiness{}, fmt.Errorf("attempt spawner is not fully wired")
	}
	if err := binding.ValidateForNewWork(); err != nil {
		return profileNotReady(err.Error())
	}
	rec, ok, err := a.projects.GetProject(ctx, string(projectID))
	if err != nil {
		return ports.AgentProfileReadiness{}, err
	}
	if !ok {
		return ports.AgentProfileReadiness{Ready: false, Detail: "project is not registered"}, nil
	}

	// Readiness consumes the same exact execution binding as launch. Project
	// configuration may still contribute provider-neutral runtime settings, but
	// it cannot rewrite the frozen provider/model selection after approval.
	return sessionmanager.ProfileReadinessForExactSpawn(
		ctx,
		a.agents,
		rec.Config,
		domain.KindWorker,
		binding,
		ports.AgentConfig{},
		policy,
	)
}

func profileNotReady(detail string) (ports.AgentProfileReadiness, error) {
	return ports.AgentProfileReadiness{Ready: false, Detail: detail}, nil
}

func (a attemptSpawner) Spawn(ctx context.Context, req ports.AttemptSpawnRequest) (ports.AttemptSpawnResult, error) {
	binding := domain.ExecutionBinding{
		Provider:       req.Harness,
		ModelSelection: req.ModelSelection,
		Model:          strings.TrimSpace(req.Model),
	}
	if err := binding.ValidateForNewWork(); err != nil {
		return ports.AttemptSpawnResult{}, err
	}
	sess, _, _, err := a.sessions.SpawnExactAttempt(ctx, ports.SpawnConfig{
		ProjectID:            req.ProjectID,
		Kind:                 domain.KindWorker,
		Harness:              binding.Provider,
		ExecutionPolicy:      req.ExecutionPolicy,
		Prompt:               req.Prompt,
		DisplayName:          req.DisplayName,
		AttemptInputs:        req.Inputs,
		AttemptDocuments:     req.Documents,
		BeforeProviderLaunch: req.BeforeProviderLaunch,
	}, binding)
	if err != nil {
		return ports.AttemptSpawnResult{}, err
	}
	// The ordinary session read model does not currently expose a runtime-
	// reported effective model. Leave it unknown rather than fabricating one;
	// the immutable requested binding remains recorded separately by Attempt.
	var completionBoundary domain.AttemptCompletionBoundary
	if agent, ok := a.agents.Agent(binding.Provider); ok {
		if provider, ok := agent.(ports.GovernedCompletionBoundaryProvider); ok {
			completionBoundary = provider.GovernedCompletionBoundary()
		}
	}
	var boundPolicy *domain.AttemptExecutionPolicy
	if req.ExecutionPolicy != nil {
		bound, bindErr := req.ExecutionPolicy.BindWorkspaceRoot(sess.Metadata.WorkspacePath)
		if bindErr != nil {
			return ports.AttemptSpawnResult{}, fmt.Errorf("spawn returned a workspace inconsistent with its execution policy: %w", bindErr)
		}
		boundPolicy = &bound
	}
	return ports.AttemptSpawnResult{Session: sess, ExecutionPolicy: boundPolicy, CompletionBoundary: completionBoundary}, nil
}

func (a attemptSpawner) Terminate(ctx context.Context, _ domain.ProjectID, sessionID string) (ports.TerminationResult, error) {
	freed, err := a.sessions.Kill(ctx, domain.SessionID(sessionID))
	if err != nil {
		return ports.TerminationResult{}, err
	}
	rec, err := a.sessions.Get(ctx, domain.SessionID(sessionID))
	if err != nil || !rec.IsTerminated {
		return ports.TerminationResult{}, fmt.Errorf("%w: durable record for %q does not show a terminated session", ports.ErrProviderStopUnproven, sessionID)
	}
	return ports.TerminationResult{ProviderStopped: true, WorkspaceFreed: freed}, nil
}

func runAttemptLivenessLoop(ctx context.Context, attempts attemptLivenessHook, log *slog.Logger) {
	reconcile(ctx, attempts, log, "on boot")
	ticker := time.NewTicker(attemptLivenessInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reconcile(ctx, attempts, log, "")
		}
	}
}

// reconcile runs both halves of the terminal-state sequence in order: decide
// whether execution has ended, then classify what ended.
//
// Liveness runs first on purpose. Classification only ever looks at attempts
// already recorded as reconciled, so running it after liveness lets an attempt
// that just ended be classified in the same tick instead of waiting for the
// next one. Both are pure functions of durable facts, so a failure in either
// leaves state untouched and the next tick retries.
func reconcile(ctx context.Context, attempts attemptLivenessHook, log *slog.Logger, when string) {
	suffix := ""
	if when != "" {
		suffix = " " + when
	}
	if err := attempts.EvaluateAttemptLiveness(ctx); err != nil {
		log.Warn("attempt liveness evaluation"+suffix, "err", err)
	}
	if err := attempts.ReconcileAttemptOutcomes(ctx); err != nil {
		log.Warn("attempt outcome reconciliation"+suffix, "err", err)
	}
	// Stop requests are acknowledged before continuation is considered, so a
	// pause that arrived while the last Attempt was ending takes effect on
	// this tick rather than after one more unit has been admitted.
	if err := attempts.ReconcileRunIntents(ctx); err != nil {
		log.Warn("run intent reconciliation"+suffix, "err", err)
	}
	if err := attempts.ContinueAuthorizedRuns(ctx); err != nil {
		log.Warn("authorized run continuation"+suffix, "err", err)
	}
}

type attemptLivenessHook interface {
	EvaluateAttemptLiveness(ctx context.Context) error
	// ReconcileAttemptOutcomes classifies attempts whose execution has ended.
	ReconcileAttemptOutcomes(ctx context.Context) error
	// ReconcileRunIntents acknowledges stop requests whose work has ended.
	ReconcileRunIntents(ctx context.Context) error
	// ContinueAuthorizedRuns admits the next eligible WorkUnit for Outcomes
	// the owner has authorized to keep running.
	ContinueAuthorizedRuns(ctx context.Context) error
}
