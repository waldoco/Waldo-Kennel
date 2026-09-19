package ports

import (
	"context"
	"errors"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// AttemptSpawnRequest is the complete immutable execution binding admission
// hands to the provider seam. Provider/model values come from the approved
// WorkUnit; the spawner must not reread mutable Project preferences.
type AttemptSpawnRequest struct {
	ProjectID      domain.ProjectID
	Harness        domain.AgentHarness
	ModelSelection domain.ExecutionBindingModelSelection
	Model          string
	// ExecutionPolicy is present only for a governed Attempt. A nil policy
	// preserves ordinary session spawning; it must never be synthesized from
	// mutable Project preferences.
	ExecutionPolicy *domain.AttemptExecutionPolicy
	Prompt          string
	DisplayName     string
	// Inputs are the exact retained predecessor results this Attempt was
	// admitted with. They must be materialized into the successor workspace
	// before the provider starts; an empty slice means the WorkUnit has no
	// dependencies, never that provisioning may be skipped.
	Inputs []AttemptInputRef
	// Documents is the approved supplied-document snapshot for a staged
	// Outcome. Nil for repository work.
	Documents *AttemptDocumentInputs
	// BeforeProviderLaunch persists the WorkspaceBoundLaunchPacket after
	// workspace preparation. A governed launch must fail closed when absent.
	BeforeProviderLaunch func(context.Context, domain.SessionRecord, domain.AttemptExecutionPolicy, []domain.SessionWorktreeRecord) error
}

// AttemptSpawnResult reports the spawned subordinate session and, when the
// provider/runtime can truthfully report it, the concrete effective model.
// Empty EffectiveModel is valid for provider-default runtimes that do not
// expose the selected model.
type AttemptSpawnResult struct {
	Session domain.Session
	// ExecutionPolicy is the same packet after the session manager freezes the
	// allocated workspace root. Governed callers persist this exact value.
	ExecutionPolicy    *domain.AttemptExecutionPolicy
	EffectiveModel     string
	CompletionBoundary domain.AttemptCompletionBoundary
}

// AttemptSessionSpawner is the narrow boundary between governed Attempt
// admission and the real session spawn path. Readiness and spawn both consume
// the same exact immutable binding; no Project provider/model fallback is
// permitted after Plan approval.
type AttemptSessionSpawner interface {
	ProfileReadiness(ctx context.Context, projectID domain.ProjectID, binding domain.ExecutionBinding, policy *domain.AttemptExecutionPolicy) (AgentProfileReadiness, error)
	Spawn(ctx context.Context, req AttemptSpawnRequest) (AttemptSpawnResult, error)
	Terminate(ctx context.Context, projectID domain.ProjectID, sessionID string) (TerminationResult, error)
}

// TerminationResult reports which parts of attempt termination were proven.
type TerminationResult struct {
	ProviderStopped bool
	WorkspaceFreed  bool
}

// ErrProviderStopUnproven indicates that provider termination is ambiguous.
var ErrProviderStopUnproven = errors.New("provider stop could not be proven")
