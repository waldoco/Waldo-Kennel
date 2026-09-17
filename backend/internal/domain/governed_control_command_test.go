package domain

import (
	"errors"
	"testing"
	"time"
)

func validControl(class GovernedControlClass) GovernedControlCommand {
	c := GovernedControlCommand{ID: "control-1", IdempotencyKey: "key-1", RequestFingerprint: "v1:abc", Class: class, State: GovernedCommandClaimed, SessionID: "session-1", ControllerGeneration: "gen-1", ExpectedRevision: "plan-1", CapabilityFingerprint: "caps-1", ProviderConversationID: "thread-1", Quiescence: GovernedCommandQuiescenceNotApplicable, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	switch class {
	case GovernedControlSteer:
		c.ClientMessageID, c.ProviderTurnID = "message-1", "turn-1"
	case GovernedControlAnswer:
		c.RequestInstanceID = "request-7"
	case GovernedControlInterrupt:
		c.ProviderTurnID, c.Quiescence = "turn-1", GovernedCommandQuiescencePending
	}
	return c
}

func TestGovernedControlClassCorrelation(t *testing.T) {
	for _, class := range []GovernedControlClass{GovernedControlSteer, GovernedControlAnswer, GovernedControlInterrupt} {
		if err := validControl(class).Validate(); err != nil {
			t.Fatalf("%s: %v", class, err)
		}
	}
	invalid := []GovernedControlCommand{
		func() GovernedControlCommand {
			c := validControl(GovernedControlSteer)
			c.ProviderTurnID = ""
			return c
		}(),
		func() GovernedControlCommand {
			c := validControl(GovernedControlAnswer)
			c.RequestInstanceID = ""
			return c
		}(),
		func() GovernedControlCommand {
			c := validControl(GovernedControlInterrupt)
			c.ProviderTurnID = ""
			return c
		}(),
	}
	for _, c := range invalid {
		if err := c.Validate(); !errors.Is(err, ErrGovernedCommandInvalid) {
			t.Fatalf("%s invalid err=%v", c.Class, err)
		}
	}
}

func TestGovernedControlFingerprintBindsClassTargetAndPayload(t *testing.T) {
	base := ComputeGovernedControlFingerprint("session-1", GovernedControlSteer, " key ", " turn-1 ", `{"text":"stop"}`)
	if base != ComputeGovernedControlFingerprint("session-1", GovernedControlSteer, "key", "turn-1", `{"text":"stop"}`) {
		t.Fatal("trim-equivalent values changed fingerprint")
	}
	for name, got := range map[string]string{
		"class":   ComputeGovernedControlFingerprint("session-1", GovernedControlInterrupt, "key", "turn-1", `{"text":"stop"}`),
		"target":  ComputeGovernedControlFingerprint("session-1", GovernedControlSteer, "key", "turn-2", `{"text":"stop"}`),
		"payload": ComputeGovernedControlFingerprint("session-1", GovernedControlSteer, "key", "turn-1", `{"text":"go"}`),
	} {
		if got == base {
			t.Fatalf("%s did not change fingerprint", name)
		}
	}
}
