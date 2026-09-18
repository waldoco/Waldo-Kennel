package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// PlanningSessionID identifies one Contract-bound planning conversation.
type PlanningSessionID string

// IsZero reports whether the planning-session id is unset or blank.
func (id PlanningSessionID) IsZero() bool   { return strings.TrimSpace(string(id)) == "" }
func (id PlanningSessionID) String() string { return string(id) }

// PlanningTurnID identifies one owner or planner turn.
type PlanningTurnID string

// IsZero reports whether the planning-turn id is unset or blank.
func (id PlanningTurnID) IsZero() bool { return strings.TrimSpace(string(id)) == "" }

// PlanningMode describes how the selected reasoning provider is invoked.
type PlanningMode string

// Supported planning modes distinguish direct APIs from native harnesses.
const (
	PlanningModeDirectAPI     PlanningMode = "direct_api"
	PlanningModeNativeHarness PlanningMode = "native_harness"
)

// Valid reports whether the planning mode is supported.
func (m PlanningMode) Valid() bool {
	return m == PlanningModeDirectAPI || m == PlanningModeNativeHarness
}

// PlanningModelSelection preserves explicit versus provider-default semantics.
type PlanningModelSelection string

// Supported planning model-selection semantics.
const (
	PlanningModelProviderDefault PlanningModelSelection = "provider_default"
	PlanningModelExplicit        PlanningModelSelection = "explicit"
)

// Valid reports whether the model-selection semantic is supported.
func (s PlanningModelSelection) Valid() bool {
	return s == PlanningModelProviderDefault || s == PlanningModelExplicit
}

// PlanningContextMode identifies the owner-approved source context.
type PlanningContextMode string

// Supported planning context sources.
const (
	PlanningContextRepositoryRead PlanningContextMode = "repository_read"
	PlanningContextSuppliedPacket PlanningContextMode = "supplied_packet"
)

// Valid reports whether the planning context source is supported.
func (m PlanningContextMode) Valid() bool {
	return m == PlanningContextRepositoryRead || m == PlanningContextSuppliedPacket
}

// PlanningSessionStatus is the durable planning-conversation lifecycle.
type PlanningSessionStatus string

// Supported planning-session states.
const (
	PlanningSessionActive        PlanningSessionStatus = "active"
	PlanningSessionProposalReady PlanningSessionStatus = "proposal_ready"
	PlanningSessionSuperseded    PlanningSessionStatus = "superseded"
	PlanningSessionCancelled     PlanningSessionStatus = "cancelled"
)

// Valid reports whether the planning-session state is supported.
func (s PlanningSessionStatus) Valid() bool {
	switch s {
	case PlanningSessionActive, PlanningSessionProposalReady, PlanningSessionSuperseded, PlanningSessionCancelled:
		return true
	default:
		return false
	}
}

// Terminal reports whether the conversation no longer accepts turns.
func (s PlanningSessionStatus) Terminal() bool { return s != PlanningSessionActive }

// PlanningWaitingOn records whose turn is required next.
type PlanningWaitingOn string

// Supported planning turn owners.
const (
	PlanningWaitingOwner    PlanningWaitingOn = "owner"
	PlanningWaitingProvider PlanningWaitingOn = "provider"
	// PlanningWaitingSystem marks a blocked planning evaluation: the next move
	// is a setup, harness, or Contract action routed from the readiness
	// packet's route codes, not an owner answer to an ordinary question and
	// never provider thinking.
	PlanningWaitingSystem PlanningWaitingOn = "system"
	PlanningWaitingNone   PlanningWaitingOn = "none"
)

// Valid reports whether the waiting actor is supported.
func (w PlanningWaitingOn) Valid() bool {
	return w == PlanningWaitingOwner || w == PlanningWaitingProvider || w == PlanningWaitingSystem || w == PlanningWaitingNone
}

// PlanningBinding freezes the owner-selected reasoner. It is proposal
// provenance, never a WorkUnit execution binding.
type PlanningBinding struct {
	Mode           PlanningMode
	Provider       IntelligenceProviderID
	ModelSelection PlanningModelSelection
	Model          string
	Effort         string
}

// Validate checks exact provider and model-selection semantics.
func (b PlanningBinding) Validate() error {
	if !b.Mode.Valid() || b.Provider.IsZero() || !b.ModelSelection.Valid() {
		return fmt.Errorf("planning provider binding is invalid")
	}
	switch b.ModelSelection {
	case PlanningModelProviderDefault:
		if strings.TrimSpace(b.Model) != "" {
			return fmt.Errorf("provider-default planning selection cannot name a model")
		}
	case PlanningModelExplicit:
		if strings.TrimSpace(b.Model) == "" {
			return fmt.Errorf("explicit planning selection requires a model")
		}
	}
	return nil
}

// PlanningSession owns the bounded conversation between one confirmed Contract
// and one proposed Plan. ContextSnapshotJSON freezes the exact read-only packet
// supplied to direct APIs; it contains no execution authority.
type PlanningSession struct {
	ID                     PlanningSessionID
	OutcomeID              OutcomeID
	ProjectID              ProjectID
	ContractRevisionID     ContractRevisionID
	ContractRevisionNumber int64
	Revision               int64
	LatestTurnSequence     int64
	Status                 PlanningSessionStatus
	WaitingOn              PlanningWaitingOn
	Binding                PlanningBinding
	ContextMode            PlanningContextMode
	PlanningGrantDigest    SHA256Digest
	ContextDigest          SHA256Digest
	ContextSnapshotJSON    json.RawMessage
	EffectiveProvider      IntelligenceProviderID
	EffectiveModel         string
	// NativeConversationRef is reserved for a provider thread that is actually
	// stable across this whole PlanningSession. One-shot native packet calls
	// record their distinct thread references on their IntelligenceRuns instead.
	NativeConversationRef  string
	ProposedPlanRevisionID PlanRevisionID
	LastFailureCode        string
	LastFailureDetail      string
	RequestKey             string
	RequestFingerprint     SHA256Digest
	CreatedAt              time.Time
	UpdatedAt              time.Time
	ClosedAt               *time.Time
}

// Validate checks planning lineage, state, grant, and provenance invariants.
func (s PlanningSession) Validate() error {
	if s.ID.IsZero() || s.OutcomeID.IsZero() || strings.TrimSpace(string(s.ProjectID)) == "" || s.ContractRevisionID.IsZero() {
		return fmt.Errorf("planning session lineage is required")
	}
	if s.ContractRevisionNumber < 1 || s.Revision < 1 || s.LatestTurnSequence < 0 {
		return fmt.Errorf("planning session revision values are invalid")
	}
	if !s.Status.Valid() || !s.WaitingOn.Valid() {
		return fmt.Errorf("planning session state is invalid")
	}
	if err := s.Binding.Validate(); err != nil {
		return err
	}
	if !s.ContextMode.Valid() || !s.PlanningGrantDigest.Valid() || !s.ContextDigest.Valid() || !json.Valid(s.ContextSnapshotJSON) {
		return fmt.Errorf("planning context and grant must be valid and frozen")
	}
	if strings.TrimSpace(s.RequestKey) == "" || !s.RequestFingerprint.Valid() {
		return fmt.Errorf("planning session request identity is required")
	}
	if s.CreatedAt.IsZero() || s.UpdatedAt.IsZero() {
		return fmt.Errorf("planning session timestamps are required")
	}
	if s.Status == PlanningSessionActive {
		if s.ClosedAt != nil || s.WaitingOn == PlanningWaitingNone || !s.ProposedPlanRevisionID.IsZero() {
			return fmt.Errorf("active planning session carries terminal state")
		}
	} else if s.ClosedAt == nil || s.WaitingOn != PlanningWaitingNone {
		return fmt.Errorf("closed planning session requires closed time and no pending actor")
	}
	if s.Status == PlanningSessionProposalReady && s.ProposedPlanRevisionID.IsZero() {
		return fmt.Errorf("proposal-ready planning session requires a Plan revision")
	}
	if s.Status != PlanningSessionProposalReady && !s.ProposedPlanRevisionID.IsZero() {
		return fmt.Errorf("only proposal-ready planning session may link a Plan revision")
	}
	if s.EffectiveProvider.IsZero() && (strings.TrimSpace(s.EffectiveModel) != "" || strings.TrimSpace(s.NativeConversationRef) != "") {
		return fmt.Errorf("effective planning model/session requires effective provider")
	}
	return nil
}

// PlanningTurnRole identifies the author of a normalized planning turn.
type PlanningTurnRole string

// Supported planning-turn roles.
const (
	PlanningTurnOwner   PlanningTurnRole = "owner"
	PlanningTurnPlanner PlanningTurnRole = "planner"
)

// Valid reports whether the planning-turn role is supported.
func (r PlanningTurnRole) Valid() bool { return r == PlanningTurnOwner || r == PlanningTurnPlanner }

// PlanningTurnKind identifies the typed meaning of one visible turn.
type PlanningTurnKind string

// Supported owner and planner turn kinds.
const (
	PlanningTurnMessage                PlanningTurnKind = "message"
	PlanningTurnFinalizeRequest        PlanningTurnKind = "finalize_request"
	PlanningTurnClarification          PlanningTurnKind = "clarification"
	PlanningTurnContractChangeProposal PlanningTurnKind = "contract_change_proposal"
	PlanningTurnPlanProposal           PlanningTurnKind = "plan_proposal"
	// PlanningTurnReadinessBlocked labels a planner turn whose readiness
	// packet evaluated blocked. S3 mints only this kind for blocked packets;
	// contract_change_proposal remains valid so historical turns still load,
	// but no new turn is minted with it - a Contract-change request now
	// arrives as a blocked envelope with revise_contract issues.
	PlanningTurnReadinessBlocked PlanningTurnKind = "readiness_blocked"
)

// Valid reports whether the planning-turn kind is supported.
func (k PlanningTurnKind) Valid() bool {
	switch k {
	case PlanningTurnMessage, PlanningTurnFinalizeRequest, PlanningTurnClarification,
		PlanningTurnContractChangeProposal, PlanningTurnPlanProposal, PlanningTurnReadinessBlocked:
		return true
	default:
		return false
	}
}

// PlanningTurn stores only the bounded owner-visible exchange. Raw provider
// activity remains provider transcript data and is not duplicated here.
type PlanningTurn struct {
	ID                 PlanningTurnID
	PlanningSessionID  PlanningSessionID
	Sequence           int64
	ReplyToTurnID      PlanningTurnID
	Role               PlanningTurnRole
	Kind               PlanningTurnKind
	Text               string
	StructuredPayload  json.RawMessage
	IntelligenceRunID  IntelligenceRunID
	RequestKey         string
	RequestFingerprint SHA256Digest
	CreatedAt          time.Time
}

// Validate checks turn authorship, request identity, and provenance invariants.
func (t PlanningTurn) Validate() error {
	if t.ID.IsZero() || t.PlanningSessionID.IsZero() || t.Sequence < 1 || !t.Role.Valid() || !t.Kind.Valid() || strings.TrimSpace(t.Text) == "" || t.CreatedAt.IsZero() {
		return fmt.Errorf("planning turn identity, state, text, and time are required")
	}
	if len(t.StructuredPayload) > 0 && !json.Valid(t.StructuredPayload) {
		return fmt.Errorf("planning turn structured payload is invalid JSON")
	}
	if t.Role == PlanningTurnOwner {
		if !t.ReplyToTurnID.IsZero() || !t.IntelligenceRunID.IsZero() || strings.TrimSpace(t.RequestKey) == "" || !t.RequestFingerprint.Valid() {
			return fmt.Errorf("owner planning turn requires request identity and no provider provenance")
		}
		if t.Kind != PlanningTurnMessage && t.Kind != PlanningTurnFinalizeRequest {
			return fmt.Errorf("owner planning turn kind %q is invalid", t.Kind)
		}
	} else {
		if t.ReplyToTurnID.IsZero() || t.IntelligenceRunID.IsZero() || strings.TrimSpace(t.RequestKey) != "" || !t.RequestFingerprint.IsZero() {
			return fmt.Errorf("planner planning turn requires reply and intelligence provenance only")
		}
		if t.Kind == PlanningTurnMessage || t.Kind == PlanningTurnFinalizeRequest {
			return fmt.Errorf("planner planning turn kind %q is invalid", t.Kind)
		}
	}
	return nil
}
