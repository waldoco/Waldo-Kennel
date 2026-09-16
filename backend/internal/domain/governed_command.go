package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// GovernedCommandClass identifies the provider effect a durable command claims.
// Stage 2 starts with turn dispatch only; later classes must add their own
// correlation and quiescence invariants rather than borrowing turn semantics.
type GovernedCommandClass string

const (
	GovernedCommandTurn GovernedCommandClass = "turn"
)

// GovernedCommandState is the durable delivery state of one exact command.
// DeliveryUnknown is deliberately nonterminal and blocking: lack of provider
// evidence never becomes permission to redeliver the effect.
type GovernedCommandState string

const (
	GovernedCommandClaimed         GovernedCommandState = "claimed"
	GovernedCommandDispatching     GovernedCommandState = "dispatching"
	GovernedCommandAcknowledged    GovernedCommandState = "acknowledged"
	GovernedCommandRejected        GovernedCommandState = "rejected"
	GovernedCommandDeliveryUnknown GovernedCommandState = "delivery_unknown"
	GovernedCommandReconciled      GovernedCommandState = "reconciled"
)

// GovernedCommandReconciliationOutcome states what later provider evidence
// established for a command whose delivery was unknown.
type GovernedCommandReconciliationOutcome string

const (
	GovernedCommandReconciledAcknowledged GovernedCommandReconciliationOutcome = "acknowledged"
	GovernedCommandReconciledRejected     GovernedCommandReconciliationOutcome = "rejected"
)

// GovernedCommandReplayStrategy is a negotiated provider capability, not a
// synthetic cross-provider cursor. A cursor is carried only when the provider
// actually supplies one.
type GovernedCommandReplayStrategy string

const (
	GovernedCommandReplayStableHistory  GovernedCommandReplayStrategy = "stable_history_replay"
	GovernedCommandReplayProviderCursor GovernedCommandReplayStrategy = "provider_cursor"
	GovernedCommandReplayUnavailable    GovernedCommandReplayStrategy = "unavailable"
)

// GovernedCommandQuiescence records whether an effect boundary is safe to
// release. Codex process-tree verification is the Stage 1 stop result; provider
// turn lifecycle alone is not equivalent evidence.
type GovernedCommandQuiescence string

const (
	GovernedCommandQuiescenceNotApplicable GovernedCommandQuiescence = "not_applicable"
	GovernedCommandQuiescencePending       GovernedCommandQuiescence = "pending"
	GovernedCommandQuiescenceCodexTree     GovernedCommandQuiescence = "codex_process_tree_verified"
)

// GovernedCommandCorrelation contains provider identities used to reconcile a
// claimed command. ClientMessageID is mandatory for turn dispatch. Provider
// identities appear only after the provider supplies them.
type GovernedCommandCorrelation struct {
	ProviderConversationID string
	ClientMessageID        string
	ProviderTurnID         string
	ProviderEventID        string
	ProviderCursor         string
}

// GovernedCommandContract is the immutable semantic contract plus current
// delivery evidence for one command. Persistence and service wiring are later
// Stage 2 slices; this type freezes their shared meaning first.
type GovernedCommandContract struct {
	ID                    string
	IdempotencyKey        string
	RequestFingerprint    string
	Class                 GovernedCommandClass
	State                 GovernedCommandState
	SessionID             SessionID
	ControllerGeneration  string
	ExpectedRevision      string
	CapabilityFingerprint string
	Correlation           GovernedCommandCorrelation
	ReplayStrategy        GovernedCommandReplayStrategy
	ReconciliationOutcome GovernedCommandReconciliationOutcome
	Quiescence            GovernedCommandQuiescence
	QuiescenceEvidenceRef string
}

// GovernedCommandRecord is the durable form of a command claim. CreatedAt is
// fixed at claim time; UpdatedAt is reserved for later compare-and-set state
// transitions and equals CreatedAt for S2.1 claims.
type GovernedCommandRecord struct {
	GovernedCommandContract
	CreatedAt time.Time
	UpdatedAt time.Time
}

var (
	ErrGovernedCommandInvalid             = errors.New("governed command contract is invalid")
	ErrGovernedCommandTransition          = errors.New("governed command state transition is invalid")
	ErrGovernedCommandIdempotencyConflict = errors.New("governed command idempotency conflict")
)

// Validate checks intrinsic Stage 2.0 invariants. Authority, persistence and
// current-generation ownership remain service/store responsibilities.
func (c GovernedCommandContract) Validate() error {
	if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.IdempotencyKey) == "" ||
		strings.TrimSpace(c.RequestFingerprint) == "" || c.SessionID == "" ||
		strings.TrimSpace(c.ControllerGeneration) == "" || strings.TrimSpace(c.ExpectedRevision) == "" ||
		strings.TrimSpace(c.CapabilityFingerprint) == "" {
		return fmt.Errorf("%w: identity, request, generation, revision, and capability bindings are required", ErrGovernedCommandInvalid)
	}
	if c.Class != GovernedCommandTurn {
		return fmt.Errorf("%w: unsupported command class %q", ErrGovernedCommandInvalid, c.Class)
	}
	if strings.TrimSpace(c.Correlation.ProviderConversationID) == "" || strings.TrimSpace(c.Correlation.ClientMessageID) == "" {
		return fmt.Errorf("%w: turn correlation requires provider conversation and client message ids", ErrGovernedCommandInvalid)
	}
	if !validGovernedCommandState(c.State) {
		return fmt.Errorf("%w: unknown state %q", ErrGovernedCommandInvalid, c.State)
	}
	if err := validateReplayStrategy(c.ReplayStrategy, c.Correlation.ProviderCursor); err != nil {
		return err
	}
	if c.State == GovernedCommandAcknowledged && strings.TrimSpace(c.Correlation.ProviderTurnID) == "" {
		return fmt.Errorf("%w: acknowledged turn requires provider turn id", ErrGovernedCommandInvalid)
	}
	if c.State == GovernedCommandReconciled {
		if c.ReconciliationOutcome != GovernedCommandReconciledAcknowledged && c.ReconciliationOutcome != GovernedCommandReconciledRejected {
			return fmt.Errorf("%w: reconciled command requires an evidence-backed outcome", ErrGovernedCommandInvalid)
		}
		if c.ReconciliationOutcome == GovernedCommandReconciledAcknowledged && strings.TrimSpace(c.Correlation.ProviderTurnID) == "" {
			return fmt.Errorf("%w: reconciled acknowledgment requires provider turn id", ErrGovernedCommandInvalid)
		}
	} else if c.ReconciliationOutcome != "" {
		return fmt.Errorf("%w: reconciliation outcome is only valid in reconciled state", ErrGovernedCommandInvalid)
	}
	if c.Quiescence == GovernedCommandQuiescenceCodexTree {
		if strings.TrimSpace(c.QuiescenceEvidenceRef) == "" {
			return fmt.Errorf("%w: verified Codex quiescence requires Stage 1 evidence", ErrGovernedCommandInvalid)
		}
	} else if c.Quiescence != GovernedCommandQuiescenceNotApplicable && c.Quiescence != GovernedCommandQuiescencePending {
		return fmt.Errorf("%w: unknown quiescence state %q", ErrGovernedCommandInvalid, c.Quiescence)
	} else if c.QuiescenceEvidenceRef != "" {
		return fmt.Errorf("%w: unverified quiescence cannot carry evidence", ErrGovernedCommandInvalid)
	}
	return nil
}

func validateReplayStrategy(strategy GovernedCommandReplayStrategy, cursor string) error {
	switch strategy {
	case GovernedCommandReplayStableHistory, GovernedCommandReplayUnavailable:
		if cursor != "" {
			return fmt.Errorf("%w: replay strategy %q cannot carry a provider cursor", ErrGovernedCommandInvalid, strategy)
		}
	case GovernedCommandReplayProviderCursor:
		if strings.TrimSpace(cursor) == "" {
			return fmt.Errorf("%w: provider-cursor replay requires an opaque cursor", ErrGovernedCommandInvalid)
		}
	default:
		return fmt.Errorf("%w: unknown replay strategy %q", ErrGovernedCommandInvalid, strategy)
	}
	return nil
}

func validGovernedCommandState(state GovernedCommandState) bool {
	switch state {
	case GovernedCommandClaimed, GovernedCommandDispatching, GovernedCommandAcknowledged,
		GovernedCommandRejected, GovernedCommandDeliveryUnknown, GovernedCommandReconciled:
		return true
	default:
		return false
	}
}

// CanTransitionGovernedCommand defines the only legal delivery-state edges.
// Unknown remains blocked until evidence reconciles it; it never transitions
// back to claimed or dispatching.
func CanTransitionGovernedCommand(from, to GovernedCommandState) bool {
	switch from {
	case GovernedCommandClaimed:
		return to == GovernedCommandDispatching
	case GovernedCommandDispatching:
		return to == GovernedCommandAcknowledged || to == GovernedCommandRejected || to == GovernedCommandDeliveryUnknown
	case GovernedCommandDeliveryUnknown:
		return to == GovernedCommandReconciled
	default:
		return false
	}
}

// BlocksConflictingDispatch reports states that still own the command effect.
// Terminal acknowledgment/rejection/reconciliation can be replayed to the
// caller; claimed, dispatching and unknown prevent a conflicting dispatch.
func (s GovernedCommandState) BlocksConflictingDispatch() bool {
	return s == GovernedCommandClaimed || s == GovernedCommandDispatching || s == GovernedCommandDeliveryUnknown
}

// GovernedSessionEffectFence is checked at the pre-effect boundary, before a
// provider process starts or a restore can observe/emit provider events. A
// generation check only at the final database write is too late: teardown may
// already be destroying the same process tree or worktree.
type GovernedSessionEffectFence struct {
	ExpectedGeneration string
	CurrentGeneration  string
	ClaimVisible       bool
	TeardownInFlight   bool
}

// AllowsDispatch reports whether a durable command may cross into provider
// effects. The claimed row must already be visible, its state must have moved
// durably to dispatching, the ownership generation must still match, and no
// teardown may hold the same per-session fence.
func (f GovernedSessionEffectFence) AllowsDispatch(state GovernedCommandState) bool {
	return state == GovernedCommandDispatching && f.ClaimVisible &&
		strings.TrimSpace(f.ExpectedGeneration) != "" && f.ExpectedGeneration == f.CurrentGeneration &&
		!f.TeardownInFlight
}

// AllowsProviderEvent reports whether an event can escape into projection.
// Early provider events are rejected until the durable claim is visible under
// the same live ownership fence as dispatch.
func (f GovernedSessionEffectFence) AllowsProviderEvent(state GovernedCommandState) bool {
	if !f.ClaimVisible || strings.TrimSpace(f.ExpectedGeneration) == "" ||
		f.ExpectedGeneration != f.CurrentGeneration || f.TeardownInFlight {
		return false
	}
	switch state {
	case GovernedCommandDispatching, GovernedCommandAcknowledged, GovernedCommandDeliveryUnknown, GovernedCommandReconciled:
		return true
	default:
		return false
	}
}
