package ports

import (
	"context"
	"fmt"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// PlanningSessionRevisionConflictError reports an optimistic-lock mismatch.
type PlanningSessionRevisionConflictError struct {
	SessionID domain.PlanningSessionID
	Expected  int64
	Current   int64
}

func (e *PlanningSessionRevisionConflictError) Error() string {
	return fmt.Sprintf("planning session %s moved from revision %d to %d", e.SessionID, e.Expected, e.Current)
}

// PlanningRequestConflictError reports reuse of a key for different semantics.
type PlanningRequestConflictError struct{ RequestKey string }

func (e *PlanningRequestConflictError) Error() string {
	return fmt.Sprintf("planning request key %q is already bound to different semantics", e.RequestKey)
}

// PlanningFinalizeConflictError reports that a planning-sourced Plan lost its
// Contract/session fence in the same transaction that attempted persistence.
type PlanningFinalizeConflictError struct{ SessionID domain.PlanningSessionID }

func (e *PlanningFinalizeConflictError) Error() string {
	return fmt.Sprintf("planning session %s is no longer finalizable", e.SessionID)
}

// PlanningCandidate is one exact owner-selectable provider/model binding.
type PlanningCandidate struct {
	ID                string
	Binding           domain.PlanningBinding
	Ready             bool
	UnavailableCode   string
	UnavailableDetail string
}

// PlanningDiscussionRequest carries frozen lineage, context, and conversation.
type PlanningDiscussionRequest struct {
	Binding           domain.PlanningBinding
	Outcome           domain.Outcome
	Contract          domain.ContractRevision
	CriterionAliases  map[string]domain.CriterionID
	// Fence is the evaluation fence the readiness envelope's planner-declared
	// issues are keyed under. Its RoutingSnapshotID is blank at request time;
	// the readiness evaluator re-keys every issue under the snapshot it reads.
	Fence domain.PlanningReadinessFence
	RepositoryContext RepositoryContextSnapshot
	// RepositoryToolUse comes from the PlanningSession's frozen, owner-approved
	// repository_read grant. A repository packet alone never grants native tools.
	RepositoryToolUse bool
	Turns             []domain.PlanningTurn
	Finalize          bool
}

// PlanningDiscussionResponse pairs the strict readiness envelope with actual
// provenance. The result is non-authoritative: the control plane evaluates
// every claim before any Plan exists.
type PlanningDiscussionResponse struct {
	Result     domain.PlanningReadinessResult
	Provenance IntelligenceProvenance
}

// PlanningIntelligenceProvider is the direct, structured, multi-turn planning
// seam. It refuses a stale binding rather than selecting a substitute.
type PlanningIntelligenceProvider interface {
	PlanningCandidates(context.Context) ([]PlanningCandidate, error)
	DiscussPlan(context.Context, PlanningDiscussionRequest) (PlanningDiscussionResponse, error)
}

// PlanningSessionStore persists conversations and their canonical Plan link.
type PlanningSessionStore interface {
	CreatePlanningSession(context.Context, domain.PlanningSession) (domain.PlanningSession, bool, error)
	GetPlanningSessionByRequestKey(context.Context, string) (domain.PlanningSession, bool, error)
	GetPlanningSession(context.Context, domain.OutcomeID, domain.PlanningSessionID) (domain.PlanningSession, bool, error)
	GetCurrentPlanningSession(context.Context, domain.OutcomeID) (domain.PlanningSession, bool, error)
	ListPlanningTurns(context.Context, domain.PlanningSessionID) ([]domain.PlanningTurn, error)
	AppendPlanningOwnerTurn(context.Context, domain.PlanningSessionID, int64, domain.PlanningTurn) (domain.PlanningSession, domain.PlanningTurn, bool, error)
	// AppendPlanningProviderTurn records one evaluated reply and advances the
	// session to its post-evaluation wait state - owner or system - in one
	// transaction, so durable history always reflects the derived outcome.
	AppendPlanningProviderTurn(context.Context, domain.PlanningSessionID, int64, domain.PlanningTurn, domain.PlanningWaitingOn, domain.IntelligenceProviderID, string, string) (domain.PlanningSession, error)
	SetPlanningSessionFailure(context.Context, domain.PlanningSessionID, int64, string, string) (domain.PlanningSession, error)
	ClosePlanningSession(context.Context, domain.PlanningSessionID, int64, domain.PlanningSessionStatus) (domain.PlanningSession, error)
	LinkPlanningSessionPlan(context.Context, domain.PlanningSessionID, int64, domain.PlanRevisionID, domain.IntelligenceRunID) (domain.PlanningSession, error)
	GetPlanRevisionByPlanningSession(context.Context, domain.OutcomeID, domain.PlanningSessionID) (domain.PlanRevision, bool, error)
	RecoverInterruptedPlanningSessions(context.Context, time.Time) (int64, error)
}
