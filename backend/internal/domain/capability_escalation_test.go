package domain

import "testing"

func validEscalation() CapabilityEscalation {
	e := CapabilityEscalation{Version: CapabilityEscalationVersion, OutcomeID: "o", ContractRevisionNumber: 1, PlanRevisionID: "p", WorkUnitID: "w", AttemptID: "a", AttemptGeneration: 3, SessionID: "s", SessionGeneration: 4, ExecutorKind: "governed_check", AttemptSessionRefID: "ref", PolicyDigest: "pd", ArtifactVersion: "av", CheckID: "check", RequestedCapability: "worktree.exec", DenialSource: "governed_check", GrantFingerprint: "gf", OperationID: "check:x", RequestFingerprint: "rf", QuestionGeneration: "q", WithinContractCeiling: true}
	e.Digest, _ = e.ComputedDigest()
	return e
}
func TestCapabilityEscalationDigestBindsEveryFence(t *testing.T) {
	base := validEscalation()
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	mutations := []func(*CapabilityEscalation){func(e *CapabilityEscalation) { e.AttemptGeneration++ }, func(e *CapabilityEscalation) { e.SessionGeneration++ }, func(e *CapabilityEscalation) { e.RequestedCapability = "worktree.write" }, func(e *CapabilityEscalation) { e.OperationID = "check:y" }, func(e *CapabilityEscalation) { e.ControllerGeneration = "other" }, func(e *CapabilityEscalation) { e.QuestionGeneration = "q2" }}
	for i, mutate := range mutations {
		e := base
		mutate(&e)
		if err := e.Validate(); err == nil {
			t.Fatalf("mutation %d retained digest", i)
		}
	}
}
func TestCapabilityEscalationOptionsStableClosedOrder(t *testing.T) {
	o := CapabilityEscalationOptionsFor("governed_tool")
	want := []string{"grant_once", "widen_contract", "deny"}
	if len(o) != len(want) {
		t.Fatal(len(o))
	}
	for i := range want {
		if o[i].ID != want[i] {
			t.Fatalf("option %d=%s", i, o[i].ID)
		}
	}
}

func TestCapabilityEscalationCheckOptionsExcludeGrantOnce(t *testing.T) {
	o := CapabilityEscalationOptionsFor("governed_check")
	if len(o) != 2 || o[0].ID != "widen_contract" || o[1].ID != "deny" {
		t.Fatalf("options=%+v", o)
	}
}
