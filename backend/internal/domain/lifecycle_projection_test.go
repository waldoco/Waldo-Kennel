package domain

import "testing"

func TestLifecycleExhaustsAttemptStatusActivityRecovery(t *testing.T) {
	statuses := []AttemptStatus{AttemptQueued, AttemptRunning, AttemptPaused, AttemptSucceeded, AttemptFailed, AttemptCancelled, AttemptLost, AttemptReconciled}
	for _, v := range statuses {
		if _, ok := LifecycleForAttemptStatus(v); !ok {
			t.Fatal(v)
		}
	}
	activities := []ActivityState{ActivityActive, ActivityIdle, ActivityWaitingInput, ActivityBlocked, ActivityExited}
	for _, v := range activities {
		if _, ok := LifecycleForActivity(v); !ok {
			t.Fatal(v)
		}
	}
	for _, v := range []RecoveryResolution{RecoveryResumed, RecoveryReplacement, RecoveryNeedsAttention} {
		if _, ok := LifecycleForRecovery(v); !ok {
			t.Fatal(v)
		}
	}
	if got, _ := LifecycleForActivity(ActivityBlocked); got != LifecycleOwnerInputRequested {
		t.Fatal("blocked semantics changed")
	}
	if got, _ := LifecycleForRecovery(RecoveryResumed); got != LifecycleAttemptResumed {
		t.Fatal("resume mislabeled")
	}
}
func TestLifecycleExhaustsEnumerableEmittedObservationKinds(t *testing.T) {
	if len(EmittedAttemptObservationKinds) != 15 {
		t.Fatal("emitted vocabulary changed")
	}
	seen := map[string]struct{}{}
	for _, k := range EmittedAttemptObservationKinds {
		if _, dup := seen[k]; dup {
			t.Fatal("duplicate", k)
		}
		seen[k] = struct{}{}
		if _, ok := LifecycleForObservation(k); !ok {
			t.Fatal("unmapped", k)
		}
	}
	if got, _ := LifecycleForObservation(ObservationProviderExit); got != LifecycleAttemptReconciled {
		t.Fatal("exit became completion")
	}
	if got, _ := LifecycleForObservation(ObservationAttemptClassified); got != LifecycleAttemptVerified {
		t.Fatal("classification mapping wrong")
	}
	for _, k := range []string{ObservationActivationAmbiguous, ObservationGovernedCheckTerminationUnknown, ObservationProviderStopFailed} {
		if got, _ := LifecycleForObservation(k); got != LifecycleAttemptRunningUnconfirmed {
			t.Fatal(k, "lost running custody")
		}
	}
}
