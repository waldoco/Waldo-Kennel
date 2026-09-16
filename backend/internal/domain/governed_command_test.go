package domain

import (
	"errors"
	"testing"
)

func validTurnCommand() GovernedCommandContract {
	return GovernedCommandContract{
		ID: "command-1", IdempotencyKey: "request-1", RequestFingerprint: "v1:abc",
		Class: GovernedCommandTurn, State: GovernedCommandClaimed, SessionID: "session-1",
		ControllerGeneration: "generation-1", ExpectedRevision: "plan-7", CapabilityFingerprint: "caps-1",
		Correlation:    GovernedCommandCorrelation{ProviderConversationID: "thread-1", ClientMessageID: "message-1"},
		ReplayStrategy: GovernedCommandReplayStableHistory,
		Quiescence:     GovernedCommandQuiescenceNotApplicable,
	}
}

func TestGovernedCommandStateMachine(t *testing.T) {
	states := []GovernedCommandState{
		GovernedCommandClaimed, GovernedCommandDispatching, GovernedCommandAcknowledged,
		GovernedCommandRejected, GovernedCommandDeliveryUnknown, GovernedCommandReconciled,
	}
	allowed := map[[2]GovernedCommandState]bool{
		{GovernedCommandClaimed, GovernedCommandDispatching}:         true,
		{GovernedCommandDispatching, GovernedCommandAcknowledged}:    true,
		{GovernedCommandDispatching, GovernedCommandRejected}:        true,
		{GovernedCommandDispatching, GovernedCommandDeliveryUnknown}: true,
		{GovernedCommandDeliveryUnknown, GovernedCommandReconciled}:  true,
	}
	for _, from := range states {
		for _, to := range states {
			if got := CanTransitionGovernedCommand(from, to); got != allowed[[2]GovernedCommandState{from, to}] {
				t.Errorf("transition %s -> %s = %t, want %t", from, to, got, allowed[[2]GovernedCommandState{from, to}])
			}
		}
	}
}

func TestDeliveryUnknownRemainsBlocked(t *testing.T) {
	for _, tc := range []struct {
		state GovernedCommandState
		want  bool
	}{
		{GovernedCommandClaimed, true}, {GovernedCommandDispatching, true},
		{GovernedCommandDeliveryUnknown, true}, {GovernedCommandAcknowledged, false},
		{GovernedCommandRejected, false}, {GovernedCommandReconciled, false},
	} {
		if got := tc.state.BlocksConflictingDispatch(); got != tc.want {
			t.Errorf("%s blocks=%t, want %t", tc.state, got, tc.want)
		}
	}
	if CanTransitionGovernedCommand(GovernedCommandDeliveryUnknown, GovernedCommandDispatching) {
		t.Fatal("delivery_unknown permitted unsafe redelivery")
	}
}

func TestGovernedCommandContractRejectsAmbiguousOrInventedEvidence(t *testing.T) {
	tests := []struct {
		name string
		edit func(*GovernedCommandContract)
	}{
		{"missing fingerprint", func(c *GovernedCommandContract) { c.RequestFingerprint = "" }},
		{"unsupported future class", func(c *GovernedCommandContract) { c.Class = "interrupt" }},
		{"turn without client correlation", func(c *GovernedCommandContract) { c.Correlation.ClientMessageID = "" }},
		{"synthetic cursor on history replay", func(c *GovernedCommandContract) { c.Correlation.ProviderCursor = "made-up" }},
		{"cursor strategy without provider cursor", func(c *GovernedCommandContract) { c.ReplayStrategy = GovernedCommandReplayProviderCursor }},
		{"acknowledged without provider turn", func(c *GovernedCommandContract) { c.State = GovernedCommandAcknowledged }},
		{"outcome before reconciliation", func(c *GovernedCommandContract) { c.ReconciliationOutcome = GovernedCommandReconciledRejected }},
		{"reconciled without outcome", func(c *GovernedCommandContract) { c.State = GovernedCommandReconciled }},
		{"reconciled accepted without provider turn", func(c *GovernedCommandContract) {
			c.State = GovernedCommandReconciled
			c.ReconciliationOutcome = GovernedCommandReconciledAcknowledged
		}},
		{"quiescence claim without evidence", func(c *GovernedCommandContract) { c.Quiescence = GovernedCommandQuiescenceCodexTree }},
		{"evidence on pending quiescence", func(c *GovernedCommandContract) {
			c.Quiescence = GovernedCommandQuiescencePending
			c.QuiescenceEvidenceRef = "receipt"
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			command := validTurnCommand()
			tc.edit(&command)
			if err := command.Validate(); !errors.Is(err, ErrGovernedCommandInvalid) {
				t.Fatalf("Validate error=%v", err)
			}
		})
	}
}

func TestGovernedCommandContractAcceptsEvidenceBackedVariants(t *testing.T) {
	tests := []struct {
		name string
		edit func(*GovernedCommandContract)
	}{
		{"claimed stable history", func(*GovernedCommandContract) {}},
		{"provider cursor", func(c *GovernedCommandContract) {
			c.ReplayStrategy = GovernedCommandReplayProviderCursor
			c.Correlation.ProviderCursor = "opaque-provider-value"
		}},
		{"unavailable replay", func(c *GovernedCommandContract) { c.ReplayStrategy = GovernedCommandReplayUnavailable }},
		{"acknowledged", func(c *GovernedCommandContract) {
			c.State = GovernedCommandAcknowledged
			c.Correlation.ProviderTurnID = "turn-1"
		}},
		{"reconciled rejection", func(c *GovernedCommandContract) {
			c.State = GovernedCommandReconciled
			c.ReconciliationOutcome = GovernedCommandReconciledRejected
		}},
		{"reconciled acknowledgment", func(c *GovernedCommandContract) {
			c.State = GovernedCommandReconciled
			c.ReconciliationOutcome = GovernedCommandReconciledAcknowledged
			c.Correlation.ProviderTurnID = "turn-1"
			c.Quiescence = GovernedCommandQuiescenceCodexTree
			c.QuiescenceEvidenceRef = "stage1-stop-receipt-1"
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			command := validTurnCommand()
			tc.edit(&command)
			if err := command.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestEarlyProviderEventCannotEscapeBeforeDurableClaim(t *testing.T) {
	fence := GovernedSessionEffectFence{
		ExpectedGeneration: "generation-1", CurrentGeneration: "generation-1",
	}
	if fence.AllowsDispatch(GovernedCommandDispatching) || fence.AllowsProviderEvent(GovernedCommandDispatching) {
		t.Fatal("provider effect/event escaped before the claim was durably visible")
	}
	fence.ClaimVisible = true
	if !fence.AllowsDispatch(GovernedCommandDispatching) || !fence.AllowsProviderEvent(GovernedCommandDispatching) {
		t.Fatal("durably claimed dispatch was not admitted")
	}
	if fence.AllowsDispatch(GovernedCommandClaimed) || fence.AllowsProviderEvent(GovernedCommandClaimed) {
		t.Fatal("claimed-but-not-dispatching command admitted provider effects")
	}
}

func TestRestoreCannotRaceTeardownUnderSameOwnershipFence(t *testing.T) {
	fence := GovernedSessionEffectFence{
		ExpectedGeneration: "generation-7", CurrentGeneration: "generation-7", ClaimVisible: true,
	}
	if !fence.AllowsDispatch(GovernedCommandDispatching) {
		t.Fatal("healthy matching fence rejected dispatch")
	}
	fence.TeardownInFlight = true
	if fence.AllowsDispatch(GovernedCommandDispatching) || fence.AllowsProviderEvent(GovernedCommandAcknowledged) {
		t.Fatal("matching generation bypassed teardown already in flight")
	}
	fence.TeardownInFlight = false
	fence.CurrentGeneration = "generation-8"
	if fence.AllowsDispatch(GovernedCommandDispatching) || fence.AllowsProviderEvent(GovernedCommandAcknowledged) {
		t.Fatal("stale generation admitted restore or provider event")
	}
}
