// Package cdc is the change-data-capture delivery layer. Change events are
// captured durably by SQLite triggers into the change_log table (see the storage
// migrations); this package POLLS that log and fans new events out, in order, to
// in-process subscribers such as terminal session-state fan-out. Future SSE/event
// endpoints can subscribe here too.
//
// There is no durable outbox/JSONL/janitor machinery: the change_log table IS
// the durable, ordered source of truth, and clients catch up by reading it from
// their own offset (SSE Last-Event-ID). The poller + broadcaster here are only
// the LIVE push on top of that.
package cdc

import (
	"encoding/json"
	"time"
)

// EventType mirrors the event_type values the DB triggers write.
type EventType string

// Event types, one per row-change the DB triggers emit into change_log.
const (
	EventSessionCreated                 EventType = "session_created"
	EventSessionUpdated                 EventType = "session_updated"
	EventPRCreated                      EventType = "pr_created"
	EventPRUpdated                      EventType = "pr_updated"
	EventPRCheckRecorded                EventType = "pr_check_recorded"
	EventPRSessionChanged               EventType = "pr_session_changed"
	EventPRReviewThreadAdded            EventType = "pr_review_thread_added"
	EventPRReviewThreadResolved         EventType = "pr_review_thread_resolved"
	EventOutcomeCreated                 EventType = "outcome_created"
	EventOutcomeUpdated                 EventType = "outcome_updated"
	EventOutcomeContractRevised         EventType = "outcome_contract_revised"
	EventOutcomePlanProposed            EventType = "outcome_plan_proposed"
	EventOutcomePlanApproved            EventType = "outcome_plan_approved"
	EventOutcomeAttemptStarted          EventType = "outcome_attempt_started"
	EventOutcomeAttemptUpdated          EventType = "outcome_attempt_updated"
	EventOutcomeAttemptBound            EventType = "outcome_attempt_session_bound"
	EventOutcomeAttemptObserved         EventType = "outcome_attempt_observed"
	EventOutcomeAttemptRecovered        EventType = "outcome_attempt_recovered"
	EventOutcomeEvidenceRecorded        EventType = "outcome_evidence_recorded"
	EventOutcomeVerificationRecorded    EventType = "outcome_verification_recorded"
	EventOutcomeAcceptanceDecided       EventType = "outcome_acceptance_decided"
	EventOutcomeCorrectionRecorded      EventType = "outcome_correction_recorded"
	EventIntakeCaptured                 EventType = "intake_captured"
	EventIntakeUpdated                  EventType = "intake_updated"
	EventIntakeProposalRevised          EventType = "intake_proposal_revised"
	EventIntakeConfirmed                EventType = "intake_confirmed"
	EventResponsibilityLinkCreated      EventType = "responsibility_link_created"
	EventResponsibilityLinkEnded        EventType = "responsibility_link_ended"
	EventWaldoConversationCreated       EventType = "waldo_conversation_created"
	EventWaldoEpisodeOpened             EventType = "waldo_conversation_episode_opened"
	EventWaldoEpisodeSealed             EventType = "waldo_conversation_episode_sealed"
	EventWaldoTurnAppended              EventType = "waldo_conversation_turn_appended"
	EventWaldoContextAttached           EventType = "waldo_conversation_context_attached"
	EventWaldoContextDetached           EventType = "waldo_conversation_context_detached"
	EventWaldoContinuationPrepared      EventType = "waldo_conversation_continuation_prepared"
	EventWaldoContinuationProgressed    EventType = "waldo_conversation_continuation_progressed"
	EventWaldoContinuationRecorded      EventType = "waldo_conversation_continuation_recorded"
	EventOutcomeContributionBound       EventType = "outcome_contribution_bound"
	EventOutcomeDecompositionProposed   EventType = "outcome_decomposition_proposed"
	EventOutcomeDecompositionAuthorized EventType = "outcome_decomposition_authorized"
	EventOutcomeContributionWaived      EventType = "outcome_contribution_dependency_waived"
	EventOutcomeDecompositionRequested  EventType = "outcome_decomposition_requested"
	EventOutcomeDecompositionAnswered   EventType = "outcome_decomposition_request_answered"
	EventProjectBriefRevised            EventType = "project_brief_revised"
	EventOutcomeRunIntentChanged        EventType = "outcome_run_intent_changed"
	EventOutcomeAttemptRetained         EventType = "outcome_attempt_retained"
	EventOutcomeDeliveryChanged         EventType = "outcome_delivery_changed"
	EventHarnessPairingIntentChanged    EventType = "harness_pairing_intent_changed"
	EventHarnessConnectionChanged       EventType = "harness_connection_changed"
)

var eventTypes = map[EventType]struct{}{
	EventSessionCreated: {}, EventSessionUpdated: {}, EventPRCreated: {}, EventPRUpdated: {}, EventPRCheckRecorded: {}, EventPRSessionChanged: {}, EventPRReviewThreadAdded: {}, EventPRReviewThreadResolved: {},
	EventOutcomeCreated: {}, EventOutcomeUpdated: {}, EventOutcomeContractRevised: {}, EventOutcomePlanProposed: {}, EventOutcomePlanApproved: {}, EventOutcomeAttemptStarted: {}, EventOutcomeAttemptUpdated: {}, EventOutcomeAttemptBound: {}, EventOutcomeAttemptObserved: {}, EventOutcomeAttemptRecovered: {}, EventOutcomeEvidenceRecorded: {}, EventOutcomeVerificationRecorded: {}, EventOutcomeAcceptanceDecided: {}, EventOutcomeCorrectionRecorded: {}, EventOutcomeRunIntentChanged: {}, EventOutcomeAttemptRetained: {}, EventOutcomeDeliveryChanged: {},
	EventIntakeCaptured: {}, EventIntakeUpdated: {}, EventIntakeProposalRevised: {}, EventIntakeConfirmed: {}, EventResponsibilityLinkCreated: {}, EventResponsibilityLinkEnded: {},
	EventWaldoConversationCreated: {}, EventWaldoEpisodeOpened: {}, EventWaldoEpisodeSealed: {}, EventWaldoTurnAppended: {}, EventWaldoContextAttached: {}, EventWaldoContextDetached: {}, EventWaldoContinuationPrepared: {}, EventWaldoContinuationProgressed: {}, EventWaldoContinuationRecorded: {},
	EventOutcomeContributionBound: {}, EventOutcomeDecompositionProposed: {}, EventOutcomeDecompositionAuthorized: {}, EventOutcomeContributionWaived: {}, EventOutcomeDecompositionRequested: {}, EventOutcomeDecompositionAnswered: {}, EventProjectBriefRevised: {},
	EventHarnessPairingIntentChanged: {}, EventHarnessConnectionChanged: {},
}

// Valid reports whether t belongs to the frozen v1 event registry.
func (t EventType) Valid() bool { _, ok := eventTypes[t]; return ok }

// Event is one CDC change read from change_log. Seq is the monotonic ordering +
// idempotency key (consumers dedup by it). SessionID is empty for project-level
// events. Payload is the trigger-built JSON, kept raw so a typed transport can
// narrow it by Type (the discriminated-union decode lives at the transport edge,
// not here).
const EventEnvelopeVersion = "v1"

// MaxEventPayloadBytes bounds one durable event before it reaches SSE clients.
const MaxEventPayloadBytes = 64 << 10

// Event carries the frozen v1 CDC envelope. New envelope fields require a new
// version; payload evolution remains discriminated by Type.
type Event struct {
	Version   string          `json:"version"`
	Seq       int64           `json:"seq"`
	ProjectID string          `json:"projectId"`
	SessionID string          `json:"sessionId,omitempty"`
	Type      EventType       `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"createdAt"`
}
