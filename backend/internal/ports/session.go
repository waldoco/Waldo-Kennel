package ports

import (
	"context"
	"errors"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// ErrSessionNotFound indicates that the requested session does not exist.
var ErrSessionNotFound = errors.New("session not found")

// SpawnConfig starts one subordinate provider session. Ordinary session callers
// may rely on Project role defaults. ExactExecutionBinding is reserved for an
// approved Outcome WorkUnit: when present, provider/model selection is already
// authority and the session manager must not inherit or substitute mutable
// Project provider/model preferences.
type SpawnConfig struct {
	ProjectID       domain.ProjectID
	IssueID         domain.IssueID
	TrackerProvider domain.TrackerProvider
	IssueContext    string
	Kind            domain.SessionKind
	Harness         domain.AgentHarness
	Branch          string
	Prompt          string
	AgentConfig     AgentConfig

	// ExactExecutionBinding is non-nil only for governed Attempt execution.
	// provider_default deliberately clears any Project model preference;
	// explicit requires exactly Binding.Model. Governed Attempts additionally
	// carry an immutable ExecutionPolicy that owns the capability posture.
	ExactExecutionBinding *domain.ExecutionBinding
	// ExecutionPolicy is the immutable capability packet for a governed Attempt.
	// It is validated by readiness and again at the actual provider boundary.
	ExecutionPolicy *domain.AttemptExecutionPolicy

	// AttemptInputs are the retained predecessor results a governed successor
	// was admitted with. Non-empty requires an AttemptInputProvisioner; the
	// session manager refuses to launch rather than start the successor on a
	// workspace missing its inputs.
	AttemptInputs []AttemptInputRef
	// AttemptDocuments is the approved supplied-document snapshot a staged
	// Outcome runs against.
	AttemptDocuments *AttemptDocumentInputs

	// BeforeProviderLaunch is the mandatory governed crash boundary. Session
	// identity, canonical workspace and bound policy exist, but no provider
	// process/controller may be started until it returns successfully.
	BeforeProviderLaunch func(context.Context, domain.SessionRecord, domain.AttemptExecutionPolicy) error

	RequestedMode domain.SessionMode
	DisplayName   string
	Attachments   []SpawnAttachment
}

// SpawnAttachment carries bounded context attached to a spawned session.
type SpawnAttachment struct {
	Ext  string
	Data []byte
}
