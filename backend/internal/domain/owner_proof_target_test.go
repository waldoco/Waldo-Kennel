package domain

import (
	"bytes"
	"testing"
)

func TestOwnerProofTargetsHaveExactVersionedShapes(t *testing.T) {
	cases := []OwnerProofTarget{
		{Version: OwnerProofTargetVersion, Class: OwnerCommandTurn, SessionID: "s", ControllerGeneration: "cg", ExpectedRevision: "rev", CapabilityFingerprint: "caps"},
		{Version: OwnerProofTargetVersion, Class: OwnerCommandSteer, SessionID: "s", ControllerGeneration: "cg", ExpectedRevision: "rev", ProviderTurnID: "pt", CapabilityFingerprint: "caps"},
		{Version: OwnerProofTargetVersion, Class: OwnerCommandAnswer, QuestionID: "q", QuestionGeneration: "qg"},
		{Version: OwnerProofTargetVersion, Class: OwnerCommandInterrupt, SessionID: "s", ControllerGeneration: "cg", ProviderTurnID: "pt", CapabilityFingerprint: "caps"},
	}
	for _, target := range cases {
		b1, err := target.CanonicalBytes()
		if err != nil {
			t.Fatalf("%s: %v", target.Class, err)
		}
		b2, err := target.CanonicalBytes()
		if err != nil || !bytes.Equal(b1, b2) {
			t.Fatalf("%s not deterministic", target.Class)
		}
		d, err := target.Digest()
		if err != nil || !d.Valid() {
			t.Fatalf("%s digest=%q err=%v", target.Class, d, err)
		}
	}
}

func TestOwnerProofTargetsRejectMissingExtraAndUnfrozenCancel(t *testing.T) {
	cases := []OwnerProofTarget{
		{Version: "v1", Class: OwnerCommandTurn, SessionID: "s", ControllerGeneration: "cg", ExpectedRevision: "r", CapabilityFingerprint: "caps"},
		{Version: OwnerProofTargetVersion, Class: OwnerCommandTurn, SessionID: "s", ControllerGeneration: "cg"},
		{Version: OwnerProofTargetVersion, Class: OwnerCommandTurn, SessionID: "s", ControllerGeneration: "cg", ExpectedRevision: "r", ProviderTurnID: "extra", CapabilityFingerprint: "caps"},
		{Version: OwnerProofTargetVersion, Class: OwnerCommandAnswer, QuestionID: "q", QuestionGeneration: "qg", SessionID: "extra"},
		{Version: OwnerProofTargetVersion, Class: OwnerCommandCancel, SessionID: "s"},
	}
	for i, target := range cases {
		if _, err := target.CanonicalBytes(); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
}
