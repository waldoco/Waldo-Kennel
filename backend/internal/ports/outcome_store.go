package ports

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// OutcomeConflictError reports an optimistic-concurrency failure on the
// Outcome's immutable Contract pointer.
type OutcomeConflictError struct {
	OutcomeID           domain.OutcomeID
	ExpectedRevisionNum int64
	CurrentRevisionNum  int64
}

func (e *OutcomeConflictError) Error() string {
	return fmt.Sprintf("outcome %s moved past contract revision %s (current %s)",
		e.OutcomeID, strconv.FormatInt(e.ExpectedRevisionNum, 10), strconv.FormatInt(e.CurrentRevisionNum, 10))
}

// AttemptAdmission is the exact canonical identity storage needs to create one
// queued Attempt. The scheduler/service must choose the approved WorkUnit
// before this boundary; storage never indexes into a Plan to guess what runs.
type AttemptAdmission struct {
	OutcomeID              domain.OutcomeID
	PlanRevisionID         domain.PlanRevisionID
	WorkUnitID             domain.WorkUnitID
	ContractRevisionNumber int64
	// RunIntentGeneration is the owner authorization generation observed by
	// the service. Storage revalidates it in the same transaction that inserts
	// the Attempt and fence, so a pause/cancel cannot land between a read and
	// admission.
	RunIntentGeneration int64
	RequestKey          string
	FenceSubject        string
	At                  time.Time
	RetryLimit          *int
}

// AttemptRetryBudgetExceededError means a new row was refused atomically; no execution budget was consumed.
type AttemptRetryBudgetExceededError struct {
	WorkUnitID    domain.WorkUnitID
	RetryLimit    int
	PriorAttempts int
}

func (e *AttemptRetryBudgetExceededError) Error() string {
	return fmt.Sprintf("work unit %s retry budget exhausted: %d prior attempts for limit %d", e.WorkUnitID, e.PriorAttempts, e.RetryLimit)
}

// AttemptExecutionUsageStore is the normalized provider-neutral execution ledger.
type AttemptExecutionUsageStore interface {
	AppendAttemptExecutionUsage(context.Context, domain.ExecutionUsageSample) (domain.ExecutionUsageSample, bool, error)
	WorkUnitExecutionUsage(context.Context, domain.WorkUnitID) (domain.ExecutionUsageTotals, error)
}

// RunIntentReplayConflictError reports reuse of a run-command key with a
// different complete request fingerprint. The key is not a generic lock: it
// identifies one exact owner command.
type RunIntentReplayConflictError struct {
	Existing domain.OutcomeRunIntent
	Request  domain.OutcomeRunIntent
}

func (e *RunIntentReplayConflictError) Error() string {
	return fmt.Sprintf("run intent request key is already bound to different command semantics for outcome %s", e.Existing.OutcomeID)
}

// RunIntentGenerationConflictError reports a failed durable compare-and-swap
// for an owner command.
type RunIntentGenerationConflictError struct {
	OutcomeID domain.OutcomeID
	Expected  int64
	Current   int64
	Found     bool
}

func (e *RunIntentGenerationConflictError) Error() string {
	return fmt.Sprintf("run intent for outcome %s changed from expected generation %d to %d", e.OutcomeID, e.Expected, e.Current)
}

// AttemptRunIntentConflictError reports an Attempt admission that lost the
// authorization race in the same storage transaction.
type AttemptRunIntentConflictError struct {
	OutcomeID domain.OutcomeID
	Expected  int64
	Current   int64
	Desired   domain.RunIntentDesired
	Found     bool
}

func (e *AttemptRunIntentConflictError) Error() string {
	return fmt.Sprintf("attempt admission lost run authorization for outcome %s: expected generation %d, current %d (%s)", e.OutcomeID, e.Expected, e.Current, e.Desired)
}

// OutcomeStore is the canonical durable boundary for Outcome control-plane
// state. Implementations own atomic writes; services own policy and authority.
type OutcomeStore interface {
	EnsureWorkResponsibilitySpace(context.Context, domain.ProjectID) (domain.ResponsibilitySpace, error)
	FindOutcomeByIdempotencyKey(context.Context, string) (domain.Outcome, bool, error)
	CreateOutcomeWithContract(context.Context, domain.Outcome, domain.ContractRevision, string) error
	GetOutcome(context.Context, domain.OutcomeID) (domain.Outcome, bool, error)
	ListOutcomesByProject(context.Context, domain.ProjectID) ([]domain.Outcome, error)
	AppendContractRevision(context.Context, domain.OutcomeID, int64, domain.ContractRevision) (int64, error)
	ListContractRevisions(context.Context, domain.OutcomeID) ([]domain.ContractRevision, error)

	CreateContributionWithContract(context.Context, domain.Outcome, domain.ContractRevision, []domain.ContributionLink, string) error
	ListContributingOutcomes(context.Context, domain.OutcomeID) ([]domain.Outcome, error)
	ListContributionLinksForParent(context.Context, domain.OutcomeID) ([]domain.ContributionLink, error)
	ListContributionLinksForChild(context.Context, domain.OutcomeID) ([]domain.ContributionLink, error)

	AppendDecompositionRevision(context.Context, domain.DecompositionRevision) (domain.DecompositionRevision, error)
	AuthorizeDecompositionRevision(context.Context, domain.OutcomeID, domain.DecompositionRevisionID, []AuthorizedContribution, time.Time) error
	GetDecompositionRevision(context.Context, domain.OutcomeID, domain.DecompositionRevisionID) (domain.DecompositionRevision, bool, error)
	LatestDecompositionRevision(context.Context, domain.OutcomeID) (domain.DecompositionRevision, bool, error)
	AppendContributionDependencyWaiver(context.Context, domain.ContributionDependencyWaiver) error
	ListContributionDependencyWaivers(context.Context, domain.DecompositionRevisionID) ([]domain.ContributionDependencyWaiver, error)
	CreateDecompositionRequest(context.Context, domain.DecompositionRequest) error
	GetDecompositionRequest(context.Context, domain.DecompositionRequestID) (domain.DecompositionRequest, bool, error)
	LatestDecompositionRequest(context.Context, domain.OutcomeID) (domain.DecompositionRequest, bool, error)
	AnswerDecompositionRequest(context.Context, DecompositionRequestAnswer) error
	ListOpenDecompositionRequests(context.Context) ([]domain.DecompositionRequest, error)
	BindDecompositionRequestSession(context.Context, domain.DecompositionRequestID, string) error

	AppendPlanRevision(context.Context, domain.OutcomeID, domain.PlanRevision) (domain.PlanRevision, error)
	LatestProposedPlanRevision(context.Context, domain.OutcomeID, int64) (domain.PlanRevision, bool, error)
	GetPlanRevision(context.Context, domain.OutcomeID, domain.PlanRevisionID) (domain.PlanRevision, bool, error)
	GetLatestPlanRevision(context.Context, domain.OutcomeID) (domain.PlanRevision, bool, error)
	ApprovePlanRevision(context.Context, domain.OutcomeID, domain.PlanRevisionID) (domain.PlanRevision, bool, error)

	GetOutcomeProjectID(context.Context, domain.OutcomeID) (domain.ProjectID, bool, error)
	FindAttemptByIdempotencyKey(context.Context, string) (domain.Attempt, bool, error)
	CreateAttemptWithFence(context.Context, AttemptAdmission) (domain.Attempt, error)
	FailAttemptBeforeLaunch(context.Context, AttemptPrelaunchFailure) (domain.AttemptObservation, error)
	TerminateRunningAttemptWithObservation(context.Context, AttemptRunningTermination) (domain.AttemptObservation, bool, error)
	GetAttempt(context.Context, domain.OutcomeID, domain.AttemptID) (domain.Attempt, bool, error)
	ListAttempts(context.Context, domain.OutcomeID) ([]domain.Attempt, error)
	TransitionAttemptStatus(context.Context, domain.OutcomeID, domain.AttemptID, domain.AttemptStatus, domain.AttemptStatus, time.Time) (int64, error)
	ListAttemptsByStatus(context.Context, domain.AttemptStatus) ([]domain.Attempt, error)
	BindAttemptSession(context.Context, domain.AttemptSessionRef) (domain.AttemptSessionRef, error)
	LatestAttemptSessionRef(context.Context, domain.AttemptID) (domain.AttemptSessionRef, bool, error)
	ListAttemptSessionRefs(context.Context, domain.AttemptID) ([]domain.AttemptSessionRef, error)
	// ChatProtocolProvenanceForBinding returns the protocol-negotiation
	// episode that answers for one provider-session binding: the latest
	// episode at or before the binding time, so a later renegotiation never
	// answers for work bound earlier; the earliest episode when none
	// predates the binding (NegotiatedAt stays visible on the record).
	// found is false when the session has no recorded provenance, which is
	// observability absence, never an error.
	ChatProtocolProvenanceForBinding(context.Context, string, time.Time) (domain.ChatProtocolProvenance, bool, error)
	AppendAttemptObservation(context.Context, domain.AttemptID, string, string, time.Time) (domain.AttemptObservation, error)
	ListAttemptObservations(context.Context, domain.AttemptID) ([]domain.AttemptObservation, error)
	OpenFenceForSubject(context.Context, string) (domain.AttemptFence, bool, error)
	ReleaseFenceForAttempt(context.Context, domain.AttemptID, string, time.Time) (int64, error)
	RenewFenceForAttempt(context.Context, domain.AttemptID, time.Time) (int64, error)
	CreateRecoveryReceipt(context.Context, domain.AttemptRecoveryReceipt) error
	ListRecoveryReceipts(context.Context, domain.AttemptID) ([]domain.AttemptRecoveryReceipt, error)
}

// AttemptPrelaunchFailure is the single atomic store operation for a failure
// proven to have happened before provider launch. Observation, queued-to-failed
// terminalization, and custody release must either all commit or all roll back.
type AttemptPrelaunchFailure struct {
	OutcomeID          domain.OutcomeID
	AttemptID          domain.AttemptID
	ObservationKind    string
	ObservationPayload string
	ReleaseReason      string
	At                 time.Time
}

// AttemptRunningTermination is the single atomic store operation for the
// reconcile loop's liveness classification of a Running attempt: append the
// classification observation, transition Running to TargetStatus, and (only
// when ReleaseReason is non-empty) release custody, all in one transaction.
// A crash or later write failure can therefore never leave a terminal
// Attempt holding an open workspace fence — either everything commits, or
// nothing does and the Attempt is still Running, still visited by the next
// liveness pass, and the whole operation is retried from scratch. Leave
// ReleaseReason empty for an unclassified reconciliation (target
// AttemptReconciled) that must not touch custody.
type AttemptRunningTermination struct {
	OutcomeID          domain.OutcomeID
	AttemptID          domain.AttemptID
	TargetStatus       domain.AttemptStatus
	ObservationKind    string
	ObservationPayload string
	ReleaseReason      string
	At                 time.Time
}

// AttemptFenceHeldError reports exclusive worktree custody held by another
// Attempt. Admission must leave zero new rows when this is returned.
type AttemptFenceHeldError struct {
	Subject   string
	Holder    domain.AttemptID
	OutcomeID domain.OutcomeID
}

func (e *AttemptFenceHeldError) Error() string {
	return fmt.Sprintf("worktree subject %s is fenced by attempt %s", e.Subject, e.Holder)
}

// AttemptReplayError carries the canonical Attempt for an already-delivered
// idempotency key; callers must serve it and must not spawn again.
type AttemptReplayError struct{ Attempt domain.Attempt }

func (e *AttemptReplayError) Error() string {
	return fmt.Sprintf("attempt %s was already admitted for this request key", e.Attempt.ID)
}

// AttemptReplayConflictError reports reuse of a request key for different
// canonical execution semantics. Returning the existing Attempt in this case
// would cross Outcome or WorkUnit custody boundaries.
type AttemptReplayConflictError struct {
	Attempt        domain.Attempt
	OutcomeID      domain.OutcomeID
	PlanRevisionID domain.PlanRevisionID
	WorkUnitID     domain.WorkUnitID
}

func (e *AttemptReplayConflictError) Error() string {
	return fmt.Sprintf("request key is already bound to attempt %s for outcome %s, plan %s, work unit %s", e.Attempt.ID, e.Attempt.OutcomeID, e.Attempt.PlanRevisionID, e.Attempt.WorkUnitID)
}

// ErrExecutionPolicyUnsupported identifies an adapter that cannot prove
// enforcement of a required capability before provider launch.
var ErrExecutionPolicyUnsupported = errors.New("execution policy enforcement unsupported")

// ExecutionPolicyUnsupportedError is returned before provider launch when an
// adapter cannot prove enforcement of a required capability.
type ExecutionPolicyUnsupportedError struct {
	Harness    domain.AgentHarness
	Capability string
	Detail     string
}

func (e *ExecutionPolicyUnsupportedError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("%s cannot enforce execution capability %q", e.Harness, e.Capability)
	}
	return fmt.Sprintf("%s cannot enforce execution capability %q: %s", e.Harness, e.Capability, e.Detail)
}

func (e *ExecutionPolicyUnsupportedError) Unwrap() error { return ErrExecutionPolicyUnsupported }

// AuthorizedContribution is the atomic persistence payload created from one
// owner-authorized decomposition proposal.
type AuthorizedContribution struct {
	Ref     string
	Outcome domain.Outcome
	First   domain.ContractRevision
	Links   []domain.ContributionLink
}

// ErrDecompositionNotProposed indicates that no open proposal can be answered.
var ErrDecompositionNotProposed = errors.New("decomposition is not an open proposal")

// DecompositionRequestAnswer records the owner's answer to a proposal request.
type DecompositionRequestAnswer struct {
	RequestID       domain.DecompositionRequestID
	Status          domain.DecompositionRequestStatus
	RawProposal     string
	RefusalReason   string
	DecompositionID domain.DecompositionRevisionID
	At              time.Time
}

// ErrDecompositionRequestClosed indicates that a request is no longer answerable.
var ErrDecompositionRequestClosed = errors.New("decomposition request is not open")

// AttemptBudgetStopStore owns durable, crash-recoverable budget stop state.
type AttemptBudgetStopStore interface {
	ClaimAttemptBudgetStop(context.Context, domain.AttemptBudgetStop) (domain.AttemptBudgetStop, bool, error)
	RecordAttemptBudgetProviderStopped(context.Context, domain.AttemptID, string, domain.RuntimeBudgetReasonCode, string, time.Time) (domain.AttemptBudgetStop, error)
	GetAttemptBudgetStop(context.Context, domain.AttemptID) (domain.AttemptBudgetStop, bool, error)
	ListUnfinishedAttemptBudgetStops(context.Context) ([]domain.AttemptBudgetStop, error)
}
