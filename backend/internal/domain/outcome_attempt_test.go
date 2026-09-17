package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestAttemptTransitionLegality pins the trigger-guarded lifecycle: only the
// ratified transitions are legal, terminal statuses accept nothing, and the
// running -> succeeded shortcut stays absent: success is never assigned from a
// live process. The only route into succeeded is from reconciled — execution
// ended — and only once the WorkUnit's proof is satisfied.
func TestAttemptTransitionLegality(t *testing.T) {
	legal := [][2]AttemptStatus{
		{AttemptQueued, AttemptRunning},
		{AttemptQueued, AttemptFailed},
		{AttemptQueued, AttemptCancelled},
		{AttemptQueued, AttemptLost},
		{AttemptRunning, AttemptPaused},
		{AttemptRunning, AttemptFailed},
		{AttemptRunning, AttemptCancelled},
		{AttemptRunning, AttemptLost},
		{AttemptRunning, AttemptReconciled},
		{AttemptPaused, AttemptRunning},
		{AttemptPaused, AttemptCancelled},
		{AttemptPaused, AttemptLost},
		{AttemptReconciled, AttemptSucceeded},
	}
	for _, pair := range legal {
		if !AttemptTransitionLegal(pair[0], pair[1]) {
			t.Fatalf("%s -> %s must be legal", pair[0], pair[1])
		}
	}

	illegal := [][2]AttemptStatus{
		// The load-bearing absence: success can never be assigned straight from
		// a running process, only after execution has ended.
		{AttemptRunning, AttemptSucceeded},
		{AttemptQueued, AttemptSucceeded},
		{AttemptPaused, AttemptSucceeded},
		// A classified success cannot be walked back.
		{AttemptSucceeded, AttemptReconciled},
		// A reconciled attempt is never re-opened for execution.
		{AttemptReconciled, AttemptPaused},
		{AttemptQueued, AttemptReconciled},
		{AttemptQueued, AttemptPaused},
		// Terminal states accept nothing.
		{AttemptFailed, AttemptRunning},
		{AttemptFailed, AttemptLost},
		{AttemptCancelled, AttemptRunning},
		{AttemptLost, AttemptRunning},
		{AttemptLost, AttemptReconciled},
		{AttemptReconciled, AttemptRunning},
		{AttemptSucceeded, AttemptRunning},
	}
	for _, pair := range illegal {
		if AttemptTransitionLegal(pair[0], pair[1]) {
			t.Fatalf("%s -> %s must be rejected", pair[0], pair[1])
		}
	}

	for _, status := range []AttemptStatus{
		AttemptQueued, AttemptRunning, AttemptPaused, AttemptSucceeded,
		AttemptFailed, AttemptCancelled, AttemptLost, AttemptReconciled,
	} {
		if !AttemptTransitionLegal(status, status) {
			t.Fatalf("no-op %s -> %s must be tolerated", status, status)
		}
	}

	// unconfirmed must never appear as a stored status.
	for _, status := range []AttemptStatus{
		AttemptQueued, AttemptRunning, AttemptPaused, AttemptSucceeded,
		AttemptFailed, AttemptCancelled, AttemptLost, AttemptReconciled,
	} {
		if string(status) == "unconfirmed" {
			t.Fatal("unconfirmed must never become a stored status")
		}
	}
}

func heartbeat(present bool, firstSignal time.Time, terminated bool) SessionHeartbeatFacts {
	return SessionHeartbeatFacts{
		Present:       present,
		ActivityState: ActivityActive,
		FirstSignalAt: firstSignal,
		IsTerminated:  terminated,
	}
}

// TestDeriveAttemptPresentation pins the read-time derivation rules: missing
// heartbeats derive UNCONFIRMED (never dead), provider termination derives
// ended-unclassified rather than success, and stored paused/terminal states
// speak for themselves.
func TestDeriveAttemptPresentation(t *testing.T) {
	signaled := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name             string
		status           AttemptStatus
		facts            SessionHeartbeatFacts
		unresolvedStart  bool
		wantPhase        AttemptPhase
		wantUnconfirmed  bool
		wantUnclassified bool
	}{
		{"queued", AttemptQueued, heartbeat(true, signaled, false), false, AttemptPhaseAwaitingStart, false, false},
		{"queued with unknown start outcome", AttemptQueued, SessionHeartbeatFacts{}, true, AttemptPhaseUnconfirmed, true, false},
		{"paused", AttemptPaused, heartbeat(true, signaled, false), false, AttemptPhaseSuspended, false, false},
		{"healthy run", AttemptRunning, heartbeat(true, signaled, false), false, AttemptPhaseExecuting, false, false},
		{"session row gone", AttemptRunning, SessionHeartbeatFacts{}, false, AttemptPhaseUnconfirmed, true, false},
		{"never signalled", AttemptRunning, heartbeat(true, time.Time{}, false), false, AttemptPhaseUnconfirmed, true, false},
		{"terminated mid-run", AttemptRunning, heartbeat(true, signaled, true), false, AttemptPhaseEndedUnclassified, false, true},
		{"failed", AttemptFailed, SessionHeartbeatFacts{}, false, AttemptPhaseHaltedFailed, false, false},
		{"cancelled", AttemptCancelled, SessionHeartbeatFacts{}, false, AttemptPhaseHaltedCancelled, false, false},
		{"lost", AttemptLost, SessionHeartbeatFacts{}, false, AttemptPhaseSuspectLost, false, false},
		{"reconciled", AttemptReconciled, heartbeat(true, signaled, true), false, AttemptPhaseEndedUnclassified, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveAttemptPresentation(tc.status, tc.facts, tc.unresolvedStart, false, LivenessPolicy{Now: signaled.Add(time.Minute)})
			if got.Phase != tc.wantPhase {
				t.Fatalf("phase = %q, want %q", got.Phase, tc.wantPhase)
			}
			if got.Unconfirmed != tc.wantUnconfirmed {
				t.Fatalf("unconfirmed = %v, want %v", got.Unconfirmed, tc.wantUnconfirmed)
			}
			if got.EndedUnclassified != tc.wantUnclassified {
				t.Fatalf("endedUnclassified = %v, want %v", got.EndedUnclassified, tc.wantUnclassified)
			}
			if strings.TrimSpace(got.NextAction) == "" {
				t.Fatal("every derived state must carry a next-action hint")
			}
			raw, err := json.Marshal(got)
			if err != nil || len(raw) == 0 {
				t.Fatalf("presentation must serialize for the API read model: %v", err)
			}
		})
	}
}

// TestAttemptRecordValidation covers the intrinsic invariants storage also
// enforces before any row exists.
func TestAttemptRecordValidation(t *testing.T) {
	valid := Attempt{
		ID:                     "att-1",
		OutcomeID:              "out-1",
		PlanRevisionID:         "plan-1",
		WorkUnitID:             "wu-1",
		Number:                 1,
		Status:                 AttemptQueued,
		ContractRevisionNumber: 1,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid attempt rejected: %v", err)
	}
	broken := valid
	broken.Status = "unconfirmed"
	if err := broken.Validate(); err == nil {
		t.Fatal("unconfirmed must fail attempt validation")
	}
	broken = valid
	broken.Number = 0
	if err := broken.Validate(); err == nil {
		t.Fatal("attempt number 0 must fail validation")
	}

	ref := AttemptSessionRef{
		ID:                 "asr-1",
		AttemptID:          "att-1",
		Seq:                1,
		SessionID:          "sess-provider-1",
		Harness:            HarnessCodex,
		RunBriefCoreDigest: strings.Repeat("ab", 32),
	}
	if err := ref.Validate(); err != nil {
		t.Fatalf("valid ref rejected: %v", err)
	}
	noSession := ref
	noSession.SessionID = " "
	if err := noSession.Validate(); err == nil {
		t.Fatal("ref without session id must fail validation")
	}

	fence := AttemptFence{ID: "fence-1", Subject: "project:p1", AttemptID: "att-1", IssuedAt: time.Now()}
	if !fence.Open() {
		t.Fatal("fresh fence must be open")
	}
	if err := fence.Validate(); err != nil {
		t.Fatalf("open fence rejected: %v", err)
	}
	released := fence
	released.ReleasedAt = time.Now()
	if released.Open() {
		t.Fatal("released fence must not be open")
	}
	if err := released.Validate(); err == nil {
		t.Fatal("released fence without reason must fail validation")
	}

	receipt := AttemptRecoveryReceipt{ID: "rcpt-1", AttemptID: "att-1", Resolution: RecoveryResumed}
	if err := receipt.Validate(); err != nil {
		t.Fatalf("valid receipt rejected: %v", err)
	}
	bad := receipt
	bad.Resolution = "duplicate_effects"
	if err := bad.Validate(); err == nil {
		t.Fatal("unknown resolution must fail validation")
	}

	obs := AttemptObservation{ID: "obs-1", AttemptID: "att-1", Seq: 1, Kind: ObservationProviderExit, Payload: `{"exit":"clean"}`}
	if err := obs.Validate(); err != nil {
		t.Fatalf("valid observation rejected: %v", err)
	}
	badObs := obs
	badObs.Payload = "{not json"
	if err := badObs.Validate(); err == nil {
		t.Fatal("non-JSON payload must fail validation")
	}

	subject := FenceSubjectForProject(ProjectID("p1"))
	if subject != "project:p1" {
		t.Fatalf("fence subject = %q", subject)
	}
}

func TestAwaitingAuthorityIsCustodyFreeLifecycleState(t *testing.T) {
	if !AttemptAwaitingAuthority.Valid() || AttemptAwaitingAuthority.Terminal() {
		t.Fatal("awaiting authority must be valid and nonterminal")
	}
	for _, to := range []AttemptStatus{AttemptQueued, AttemptCancelled, AttemptFailed} {
		if !AttemptTransitionLegal(AttemptAwaitingAuthority, to) {
			t.Fatalf("missing legal transition to %s", to)
		}
	}
	for _, to := range []AttemptStatus{AttemptRunning, AttemptPaused, AttemptSucceeded, AttemptLost, AttemptReconciled} {
		if AttemptTransitionLegal(AttemptAwaitingAuthority, to) {
			t.Fatalf("illegal direct transition to %s", to)
		}
	}
}
