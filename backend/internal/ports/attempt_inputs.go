package ports

import (
	"context"
	"errors"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// ErrAttemptInputProvisioning reports that a successor's predecessor outputs
// could not be placed in its workspace.
//
// It is deliberately distinct from an ambiguous start: provisioning happens
// before any provider process exists, so this error proves nothing was
// launched. Reporting it as an unknown activation would leave the owner
// reconciling a run that never began.
var ErrAttemptInputProvisioning = errors.New("attempt input provisioning failed")

// ErrAttemptWorkspacePreparation is emitted only before provider launch when
// the session manager cannot create the workspace. Partial workspace debris may
// remain; it does not imply an unknown provider process.
var ErrAttemptWorkspacePreparation = errors.New("attempt workspace preparation failed")

// AttemptPrelaunchError proves Spawn failed before any provider launch API was
// invoked. Stage is adapter-neutral diagnostic provenance; callers may use the
// proof to terminalize the Attempt and release custody without guessing that a
// provider might still be alive.
type AttemptPrelaunchError struct {
	Stage string
	Err   error
}

func (e *AttemptPrelaunchError) Error() string {
	if e == nil || e.Err == nil {
		return "attempt failed before provider launch"
	}
	return e.Err.Error()
}

func (e *AttemptPrelaunchError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// AttemptInputRef names one exact retained predecessor result admitted to a
// successor.
//
// The artifact version is part of the reference, not something resolved later:
// admission authorized *these bytes*, and a restart or retry that re-resolved
// "the latest result of that WorkUnit" could silently hand the successor work
// the owner never authorized.
type AttemptInputRef struct {
	AttemptID       domain.AttemptID
	WorkUnitID      domain.WorkUnitID
	ArtifactVersion string
	// Required is the frozen semantic reason this predecessor output is consumed.
	// It is provider guidance, never a locator or authority source.
	Required string
}

// AttemptDocumentInputs names the approved supplied-document snapshot a
// staged Outcome runs against. Naming the revision and digest rather than
// paths is what stops execution re-reading the owner's originals.
type AttemptDocumentInputs struct {
	ContextID domain.DocumentContextID
	Revision  int64
	Digest    string
}

// AttemptInputProvisionRequest is the daemon-owned materialization boundary.
// Every field is derived from durable Attempt and workspace records; no client
// path or provider claim reaches it.
type AttemptInputProvisionRequest struct {
	Inputs []AttemptInputRef
	// Documents is the approved supplied-document snapshot, when this Outcome
	// is a document Outcome. Predecessor artifacts and approved documents
	// share one seam so both obey one fail-closed rule.
	Documents     *AttemptDocumentInputs
	WorkspacePath string
	WorkspaceKind domain.WorkspaceKind
	// BaseRevision is the successor workspace's own resolved base. It must
	// match the base the predecessors produced their changes against.
	BaseRevision string
}

// AttemptInputProvisioner materializes verified predecessor output into a
// successor's freshly created workspace, before any provider is launched.
//
// It is a port rather than a direct dependency because the session manager
// must not learn how artifacts are stored, and because a daemon without
// retention has to fail closed here rather than start a successor on an empty
// base and call that a handoff.
type AttemptInputProvisioner interface {
	ProvisionAttemptInputs(ctx context.Context, req AttemptInputProvisionRequest) error
}
