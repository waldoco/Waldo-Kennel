package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// AttemptID identifies one governed execution attempt of an Outcome's plan.
type AttemptID string

// IsZero reports whether the id is unset or blank.
func (id AttemptID) IsZero() bool {
	return strings.TrimSpace(string(id)) == ""
}

// String returns the raw identifier value.
func (id AttemptID) String() string {
	return string(id)
}

// AttemptStatus is the DURABLE status stored on an attempt row. Only the
// trigger-guarded legal transitions below may change it, and every mutation
// goes through the daemon's canonical writer.
//
// `unconfirmed` is deliberately ABSENT from this set: it is never stored. It
// is derived at read time from the bound session's heartbeat facts
// (activity_state, first_signal_at, is_terminated), because a missing
// heartbeat is unconfirmed — not dead — and suspicion must never be written
// as fact.
type AttemptStatus string

const (
	// AttemptQueued marks an admitted-but-not-yet-running attempt: its row and
	// fence exist, provider admission is in flight.
	AttemptQueued AttemptStatus = "queued"
	// AttemptRunning marks an attempt whose provider session was admitted and
	// bound. It stays running until a legal transition says otherwise; a
	// silent heartbeat downgrades only the DERIVED presentation.
	AttemptRunning AttemptStatus = "running"
	// AttemptPaused marks an owner-suspended attempt.
	AttemptPaused AttemptStatus = "paused"
	// AttemptSucceeded is reserved for #35's Verification binding. No #31 path
	// writes it: provider completion is never success.
	AttemptSucceeded AttemptStatus = "succeeded"
	// AttemptFailed marks an attempt that truthfully could not run (e.g. the
	// spawner crashed after the durable row existed).
	AttemptFailed AttemptStatus = "failed"
	// AttemptCancelled marks an owner-cancelled attempt.
	AttemptCancelled AttemptStatus = "cancelled"
	// AttemptLost marks an attempt whose custody reconcile could not prove
	// alive. Its effects stay suspect; replacement happens on a NEW row.
	AttemptLost AttemptStatus = "lost"
	// AttemptReconciled marks an ended attempt that reconcile has accounted
	// for WITHOUT classifying its result: the provider session exited and the
	// observation is recorded, but result classification awaits Verification
	// (#35). Provider completion ≠ done.
	AttemptReconciled AttemptStatus = "reconciled"
)

// Valid reports whether s is a supported attempt status.
func (s AttemptStatus) Valid() bool {
	switch s {
	case AttemptQueued, AttemptRunning, AttemptPaused, AttemptSucceeded,
		AttemptFailed, AttemptCancelled, AttemptLost, AttemptReconciled:
		return true
	}
	return false
}

// Terminal reports whether s closes the attempt's lifecycle.
func (s AttemptStatus) Terminal() bool {
	switch s {
	case AttemptSucceeded, AttemptFailed, AttemptCancelled, AttemptLost, AttemptReconciled:
		return true
	}
	return false
}

// LegalAttemptTransitions enumerates every stored-status transition the
// database triggers accept. Anything outside this map aborts the write at the
// SQL layer, so no service bug can invent a lifecycle.
var LegalAttemptTransitions = map[AttemptStatus][]AttemptStatus{
	// Queued attempts may be declared lost when reconcile cannot account for
	// an admission whose outcome is unknown (vanished start): custody must
	// resolve without dressing the ambiguity up as a clean failure.
	AttemptQueued: {AttemptRunning, AttemptFailed, AttemptCancelled, AttemptLost},
	AttemptPaused: {AttemptRunning, AttemptCancelled, AttemptLost},
	// Running attempts end through reconcile (lost/reconciled), owner action
	// (paused/cancelled), or truthful spawn/runner failure. There is
	// deliberately still no running -> succeeded edge: success is never
	// assigned straight from a live process.
	AttemptRunning: {AttemptPaused, AttemptFailed, AttemptCancelled, AttemptLost, AttemptReconciled},
	// Reconciled means execution ended with the result unclassified. It is the
	// only route to succeeded, and only once the WorkUnit's proof is satisfied
	// — see docs/verification/2026-09-09-execution-to-admission-sequence.md.
	// Reconciled is otherwise terminal: a classified success cannot be walked
	// back, and a reconciled attempt is never re-opened for execution.
	AttemptReconciled: {AttemptSucceeded},
}

// AttemptTransitionLegal reports whether from -> to is a legal stored
// transition.
func AttemptTransitionLegal(from, to AttemptStatus) bool {
	if from == to {
		return true // no-op writes are permitted and emit nothing
	}
	for _, next := range LegalAttemptTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// Attempt is one governed execution of an Outcome's approved plan. Attempts
// are append-only lineage: a replacement is always a NEW attempt row, never an
// update of the old one, so restart replays the exact custody history.
type Attempt struct {
	ID             AttemptID
	OutcomeID      OutcomeID
	PlanRevisionID PlanRevisionID
	WorkUnitID     WorkUnitID
	// Number is the attempt's 1-based position in the Outcome's lineage.
	Number int64
	Status AttemptStatus
	// ContractRevisionNumber snapshots which contract revision the executing
	// plan bound at admission; immutable for the attempt's life.
	ContractRevisionNumber int64
	// RunIntentGeneration binds this Attempt to the exact owner authorization
	// that admitted it. Zero means this was an individually started Attempt
	// before any durable run intent existed.
	RunIntentGeneration int64
	RequestKey          string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Validate checks intrinsic attempt invariants. Number uniqueness per Outcome,
// status transitions, and immutability are enforced by storage.
func (a Attempt) Validate() error {
	if a.ID.IsZero() {
		return fmt.Errorf("attempt id is required")
	}
	if a.OutcomeID.IsZero() {
		return fmt.Errorf("attempt outcome id is required")
	}
	if a.PlanRevisionID.IsZero() {
		return fmt.Errorf("attempt plan revision id is required")
	}
	if a.WorkUnitID.IsZero() {
		return fmt.Errorf("attempt work unit id is required")
	}
	if a.Number < 1 {
		return fmt.Errorf("attempt number must be at least 1")
	}
	if a.ContractRevisionNumber < 1 {
		return fmt.Errorf("attempt contract revision number must be at least 1")
	}
	if !a.Status.Valid() {
		return fmt.Errorf("unsupported attempt status %q", a.Status)
	}
	return nil
}

// AttemptSessionRef binds one attempt to one provider session identity.
//
// Per locked ruling D6, SessionID is plain TEXT with NO foreign key into
// sessions(id): a spawn rollback deletes seed session rows, but the ref
// history must outlive session-row GC so lineage survives. Harness, mode,
// digests, and the admission snapshot live on the ref.
type AttemptSessionRef struct {
	ID                     AttemptSessionRefID
	AttemptID              AttemptID
	Seq                    int64
	SessionID              string
	Harness                AgentHarness
	Mode                   SessionMode
	RunBriefCoreDigest     string
	RunBriefCompiledDigest string
	AdmissionSnapshot      string
	BoundAt                time.Time
}

// AttemptSessionRefID identifies one session binding inside an attempt.
type AttemptSessionRefID string

// IsZero reports whether the id is unset or blank.
func (id AttemptSessionRefID) IsZero() bool {
	return strings.TrimSpace(string(id)) == ""
}

// String returns the raw identifier value.
func (id AttemptSessionRefID) String() string {
	return string(id)
}

// Validate checks intrinsic session-ref invariants.
func (r AttemptSessionRef) Validate() error {
	if r.ID.IsZero() {
		return fmt.Errorf("attempt session ref id is required")
	}
	if r.AttemptID.IsZero() {
		return fmt.Errorf("attempt session ref attempt id is required")
	}
	if r.Seq < 1 {
		return fmt.Errorf("attempt session ref seq must be at least 1")
	}
	if strings.TrimSpace(r.SessionID) == "" {
		return fmt.Errorf("attempt session ref session id is required")
	}
	if len(r.RunBriefCoreDigest) != 64 {
		return fmt.Errorf("attempt session ref requires the run brief core digest")
	}
	return nil
}

// AdmissionSnapshotVersion pins the shape of the recorded admission snapshot
// so later slices can evolve the payload without reinterpreting old rows.
// Version 3 adds the immutable leased-workspace root. Version 2 records remain
// readable history but cannot be resumed because rebinding them would invent
// authority they never froze.
const AdmissionSnapshotVersion = 3

// AttemptObservation is one append-only, ordered fact observed about an
// attempt. Observations are insertable ALWAYS — including for stale attempts
// — so inspection stays possible after replacement, but no observation ever
// mutates current truth by itself.
type AttemptObservation struct {
	ID        string
	AttemptID AttemptID
	Seq       int64
	Kind      string
	Payload   string
	CreatedAt time.Time
}

// Validate checks intrinsic observation invariants.
func (o AttemptObservation) Validate() error {
	if strings.TrimSpace(o.ID) == "" {
		return fmt.Errorf("attempt observation id is required")
	}
	if o.AttemptID.IsZero() {
		return fmt.Errorf("attempt observation attempt id is required")
	}
	if o.Seq < 1 {
		return fmt.Errorf("attempt observation seq must be at least 1")
	}
	if strings.TrimSpace(o.Kind) == "" {
		return fmt.Errorf("attempt observation kind is required")
	}
	if o.Payload != "" && !json.Valid([]byte(o.Payload)) {
		return fmt.Errorf("attempt observation payload must be valid JSON")
	}
	return nil
}

// Observation kinds recorded by the v0 Act & Observe paths. Free-form kinds
// remain insertable; these are the canonical ones the service emits.
const (
	ObservationAttemptContained = "contained"
	ObservationAttemptResumed   = "resumed"
	ObservationProviderExit     = "provider_exit"
	// ObservationAttemptClassified marks the point where an ended attempt was
	// judged against its WorkUnit's proof and classified. It exists so the
	// owner can see that success was derived from evidence rather than from
	// the provider having finished.
	ObservationAttemptClassified = "attempt_classified"
	ObservationAdmissionFailed   = "admission_failed"
	// ObservationAdmissionAmbiguous marks a start whose outcome is UNKNOWN:
	// the request may or may not have reached the provider. The attempt stays
	// queued and derives as unconfirmed until reconcile decides.
	ObservationAdmissionAmbiguous = "admission_ambiguous"
	// ObservationActivationAmbiguous marks a LIVE provider session whose
	// durable promotion to running could not be recorded. Same unconfirmed
	// contract as admission ambiguity; custody stays held.
	ObservationActivationAmbiguous = "activation_ambiguous"
	// ObservationGovernedCheckTerminationUnknown records that an approved
	// repository check may still have effects in flight. The Attempt remains
	// Running and keeps custody until explicit recovery reconciles that risk.
	ObservationGovernedCheckTerminationUnknown = "governed_check_termination_unknown"
	// ObservationProviderStopFailed records a failed terminate through the
	// execution seam; cancellation was refused, not silently dropped.
	ObservationProviderStopFailed = "provider_stop_failed"
	// ObservationOwnerContained records the OWNER's explicit assertion that
	// the bound provider is stopped — the auditable basis for releasing
	// custody without machine proof (invariant 5: user edits outrank).
	ObservationOwnerContained    = "owner_contained"
	ObservationOwnerCancel       = "owner_cancelled"
	ObservationOwnerPause        = "owner_paused"
	ObservationOwnerResume       = "owner_resumed"
	ObservationRecoveryAttention = "needs_attention"
	// ObservationInputProvisioningFailed records a successor whose predecessor
	// results could not be placed in its workspace. It is deliberately not an
	// ambiguous start: provisioning precedes any provider process, so this
	// fact asserts that nothing was launched.
	ObservationInputProvisioningFailed = "input_provisioning_failed"
)

// AttemptFence is the custody lock over one worktree subject. At most ONE
// open fence per subject may exist (partial unique index on released_at IS
// NULL); replacement inherits custody only through reconcile releasing the
// old fence before the new attempt issues its own.
type AttemptFence struct {
	ID string
	// Subject names the guarded worktree (v0: "project:<id>").
	Subject   string
	AttemptID AttemptID
	IssuedAt  time.Time
	// LastRenewedAt is the renewable lease stamp: refreshed by the liveness
	// loop while the custodian runs; staleness here flags custody that may
	// outlive its provider.
	LastRenewedAt time.Time
	ReleasedAt    time.Time
	ReleaseReason string
}

// Open reports whether the fence currently holds custody.
func (f AttemptFence) Open() bool {
	return f.ReleasedAt.IsZero()
}

// Validate checks intrinsic fence invariants.
func (f AttemptFence) Validate() error {
	if strings.TrimSpace(f.ID) == "" {
		return fmt.Errorf("attempt fence id is required")
	}
	if strings.TrimSpace(f.Subject) == "" {
		return fmt.Errorf("attempt fence subject is required")
	}
	if f.AttemptID.IsZero() {
		return fmt.Errorf("attempt fence attempt id is required")
	}
	if !f.Open() && f.ReleaseReason == "" {
		return fmt.Errorf("a released attempt fence must record why")
	}
	return nil
}

// FenceSubjectForProject names the v0 worktree fence subject: one governed
// isolated worktree per project, so two Outcomes can never hold simultaneous
// writers against the same tree.
func FenceSubjectForProject(projectID ProjectID) string {
	return "project:" + string(projectID)
}

// FenceSubjectForReadOnlyAttempt turns the exclusive project fence subject
// into a non-exclusive, per-Attempt subject: concurrent read-only Attempts
// (ADR 0009 §6) never collide on the unique open-fence-per-subject index,
// while each still gets a durable fence row so renew/release/recovery code
// paths (recover.go) keep treating every Attempt uniformly.
func FenceSubjectForReadOnlyAttempt(projectSubject string, attemptID AttemptID) string {
	return projectSubject + ":read:" + string(attemptID)
}

// RecoveryResolution is the verdict a recovery receipt records.
type RecoveryResolution string

const (
	// RecoveryResumed proves the SAME attempt stayed (or returned) alive; no
	// replacement is warranted.
	RecoveryResumed RecoveryResolution = "resumed"
	// RecoveryReplacement declares the attempt unrecoverable and hands custody
	// to a future replacement attempt row.
	RecoveryReplacement RecoveryResolution = "replacement_attempt"
	// RecoveryNeedsAttention escalates to the owner without deciding custody.
	RecoveryNeedsAttention RecoveryResolution = "needs_attention"
)

// Valid reports whether r is a supported recovery resolution.
func (r RecoveryResolution) Valid() bool {
	switch r {
	case RecoveryResumed, RecoveryReplacement, RecoveryNeedsAttention:
		return true
	}
	return false
}

// AttemptRecoveryReceipt is the immutable record of one containment/reconcile
// decision. Receipts are append-only evidence; they never rewrite the
// attempt's own history.
type AttemptRecoveryReceipt struct {
	ID                   string
	AttemptID            AttemptID
	Resolution           RecoveryResolution
	ReplacementAttemptID AttemptID
	Detail               string
	CreatedAt            time.Time
}

// Validate checks intrinsic receipt invariants. ReplacementAttemptID is
// intentionally NOT foreign-keyed: it may name an attempt that does not exist
// yet when the receipt is written before the replacement starts.
func (r AttemptRecoveryReceipt) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("recovery receipt id is required")
	}
	if r.AttemptID.IsZero() {
		return fmt.Errorf("recovery receipt attempt id is required")
	}
	if !r.Resolution.Valid() {
		return fmt.Errorf("unsupported recovery resolution %q", r.Resolution)
	}
	return nil
}

// SessionHeartbeatFacts are the read-time heartbeat facts presentation is
// derived from. They mirror the durable session columns and nothing else —
// no transcript parsing, no derived-status reads.
type SessionHeartbeatFacts struct {
	// Present reports whether a session row exists for the bound ref. A
	// missing row (GC'd or rolled back) is unconfirmed, not dead.
	Present bool
	// ActivityState is the session's durable activity state, empty when absent.
	ActivityState ActivityState
	// FirstSignalAt is when the first agent hook signal arrived; zero means
	// the session has never signalled.
	FirstSignalAt time.Time
	// LastActivityAt is when the session last showed durable activity; zero
	// inherits FirstSignalAt for recency purposes.
	LastActivityAt time.Time
	// IsTerminated mirrors sessions.is_terminated.
	IsTerminated bool
}

// RecentlyActive reports durable activity inside the policy window. Sticky
// states are always recent: waiting on the USER cannot go stale.
func (f SessionHeartbeatFacts) RecentlyActive(policy LivenessPolicy) bool {
	if f.ActivityState.IsSticky() {
		return true
	}
	ref := f.LastActivityAt
	if ref.IsZero() {
		ref = f.FirstSignalAt
	}
	if ref.IsZero() {
		return false
	}
	return policy.now().Sub(ref) <= policy.window()
}

// AttemptPhase is the derived read-time phase vocabulary. Phases cross the
// API as these exact strings; the renderer keys controls off the constants,
// never off repeated literals.
type AttemptPhase string

const (
	// AttemptPhaseAwaitingStart marks an admitted-but-not-yet-running attempt.
	AttemptPhaseAwaitingStart AttemptPhase = "awaiting_start"
	// AttemptPhaseExecuting marks a proven-alive running attempt.
	AttemptPhaseExecuting AttemptPhase = "executing"
	// AttemptPhaseSuspended marks an owner-paused attempt.
	AttemptPhaseSuspended AttemptPhase = "suspended"
	// AttemptPhaseUnconfirmed marks liveness as unproven (missing heartbeat,
	// never signalled, stale activity, or an unknown start/activation outcome).
	AttemptPhaseUnconfirmed AttemptPhase = "unconfirmed"
	// AttemptPhaseNeedsInput marks a live session blocked on its user —
	// waiting_input or blocked activity. This is Needs You, not Waiting.
	AttemptPhaseNeedsInput AttemptPhase = "needs_input"
	// AttemptPhaseEndedUnclassified marks an ended attempt whose result awaits
	// Verification classification (#35).
	AttemptPhaseEndedUnclassified AttemptPhase = "ended_unclassified"
	// AttemptPhaseHaltedFailed marks a truthful pre-start failure.
	AttemptPhaseHaltedFailed AttemptPhase = "halted_failed"
	// AttemptPhaseHaltedCancelled marks an owner-cancelled attempt whose
	// custody may still need reconcile.
	AttemptPhaseHaltedCancelled AttemptPhase = "halted_cancelled"
	// AttemptPhaseSuspectLost marks an attempt custody could not account for.
	AttemptPhaseSuspectLost AttemptPhase = "suspect_lost"
	// AttemptPhaseSucceeded is reserved for #35 verification binding.
	AttemptPhaseSucceeded AttemptPhase = "succeeded"
)

// Valid reports whether p is a supported derived phase.
func (p AttemptPhase) Valid() bool {
	switch p {
	case AttemptPhaseAwaitingStart, AttemptPhaseExecuting, AttemptPhaseSuspended,
		AttemptPhaseUnconfirmed, AttemptPhaseNeedsInput, AttemptPhaseEndedUnclassified,
		AttemptPhaseHaltedFailed, AttemptPhaseHaltedCancelled, AttemptPhaseSuspectLost,
		AttemptPhaseSucceeded:
		return true
	}
	return false
}

// DefaultStaleHeartbeatWindow bounds how long a non-sticky running session may
// go without durable activity before its liveness downgrades to unconfirmed.
// Sticky states (waiting_input/blocked) are exempt: an agent waiting hours for
// its user is healthy by definition. Stale means UNPROVEN, never dead.
const DefaultStaleHeartbeatWindow = 15 * time.Minute

// LivenessPolicy carries the clock inputs derivation needs so staleness stays
// deterministic under test. Zero Now falls back to wall clock.
type LivenessPolicy struct {
	Now time.Time
	// StaleHeartbeatAfter overrides DefaultStaleHeartbeatWindow when positive.
	StaleHeartbeatAfter time.Duration
}

func (p LivenessPolicy) now() time.Time {
	if p.Now.IsZero() {
		return time.Now().UTC()
	}
	return p.Now
}

func (p LivenessPolicy) window() time.Duration {
	if p.StaleHeartbeatAfter > 0 {
		return p.StaleHeartbeatAfter
	}
	return DefaultStaleHeartbeatWindow
}

// AttemptPresentation is the derived read-time truth about an attempt: what
// phase it is really in, whether its liveness is unproven, whether it ended
// without a result classification, and the safe next action for the owner.
type AttemptPresentation struct {
	Phase             AttemptPhase `json:"phase"`
	Unconfirmed       bool         `json:"unconfirmed"`
	EndedUnclassified bool         `json:"endedUnclassified"`
	// Attention names the durable activity demanding the user when Phase is
	// needs_input ("waiting_input" | "blocked"); empty otherwise. The exact
	// decision/problem drives the surface copy.
	Attention  string `json:"attention,omitempty"`
	NextAction string `json:"nextAction"`
}

// Attention kinds for needs_input, mirroring the durable activity states.
const (
	AttemptAttentionWaitingInput = "waiting_input"
	AttemptAttentionBlocked      = "blocked"
)

// DeriveAttemptPresentation computes the read model from the STORED status,
// the bound session's heartbeat facts, and any unresolved-start flag. Rules
// pinned here:
//
//   - missing/stale heartbeat facts are UNCONFIRMED, never dead;
//   - provider/session termination is an END, never a success — ended
//     attempts present "ended, result unclassified";
//   - waiting_input/blocked activity IS durable attention: Needs You, not
//     Waiting, and exempt from staleness;
//   - an unknown start/activation outcome presents as unconfirmed too;
//   - only stored statuses speak for themselves; nothing here mutates.
func DeriveAttemptPresentation(status AttemptStatus, facts SessionHeartbeatFacts, unresolvedAdmission, unresolvedCheckTermination bool, policy LivenessPolicy) AttemptPresentation {
	switch status {
	case AttemptQueued:
		if unresolvedAdmission {
			return unconfirmedPresentation("Start outcome is unknown — reconcile before restarting so duplicate writers stay impossible.")
		}
		return AttemptPresentation{
			Phase:      AttemptPhaseAwaitingStart,
			NextAction: "Admitting the authorized plan onto a provider session.",
		}
	case AttemptPaused:
		return AttemptPresentation{
			Phase:      AttemptPhaseSuspended,
			NextAction: "Paused awaits the provider-control contract (ADR 0007); custody stays held.",
		}
	case AttemptRunning:
		switch {
		case unresolvedCheckTermination:
			return unconfirmedPresentation("An approved check may still have effects in flight — explicitly confirm containment before replacement.")
		case facts.IsTerminated:
			return endedUnclassifiedPresentation("The provider session ended. Completion is not done — classify through Verification.")
		case !facts.Present || facts.FirstSignalAt.IsZero() || !facts.RecentlyActive(policy):
			return unconfirmedPresentation("Liveness is unproven — contain and reconcile before replacing. Missing or stale heartbeats are never treated as death.")
		case facts.ActivityState == ActivityWaitingInput:
			return AttemptPresentation{
				Phase:      AttemptPhaseNeedsInput,
				Attention:  AttemptAttentionWaitingInput,
				NextAction: "The provider asked for input — respond to its question in its session.",
			}
		case facts.ActivityState == ActivityBlocked:
			return AttemptPresentation{
				Phase:      AttemptPhaseNeedsInput,
				Attention:  AttemptAttentionBlocked,
				NextAction: "A permission/approval dialog blocks the provider — approve or deny it in its session.",
			}
		default:
			return AttemptPresentation{
				Phase:      AttemptPhaseExecuting,
				NextAction: "Waiting — observe.",
			}
		}
	case AttemptFailed:
		return AttemptPresentation{
			Phase:      AttemptPhaseHaltedFailed,
			NextAction: "Reconcile custody, then start a replacement attempt if needed.",
		}
	case AttemptCancelled:
		return AttemptPresentation{
			Phase:      AttemptPhaseHaltedCancelled,
			NextAction: "Custody stays held until reconcile confirms the provider stopped — then replace if needed.",
		}
	case AttemptLost:
		return AttemptPresentation{
			Phase:      AttemptPhaseSuspectLost,
			NextAction: "Custody is unresolved — replace the attempt or mark it needing attention.",
		}
	case AttemptReconciled:
		return AttemptPresentation{
			Phase:             AttemptPhaseEndedUnclassified,
			EndedUnclassified: true,
			NextAction:        "Ended and accounted for. Result classification awaits Verification.",
		}
	case AttemptSucceeded:
		return AttemptPresentation{
			Phase:      AttemptPhaseSucceeded,
			NextAction: "Proceed to Prove & Close.",
		}
	default:
		return unconfirmedPresentation("Unknown stored state — treat as unconfirmed and reconcile.")
	}
}

func unconfirmedPresentation(next string) AttemptPresentation {
	return AttemptPresentation{Phase: AttemptPhaseUnconfirmed, Unconfirmed: true, NextAction: next}
}

func endedUnclassifiedPresentation(next string) AttemptPresentation {
	return AttemptPresentation{Phase: AttemptPhaseEndedUnclassified, EndedUnclassified: true, NextAction: next}
}
