package domain

// LifecycleProjection is a normalized event meaning, separate from admission
// and from the current Work UI phase vocabulary.
type LifecycleProjection string

const (
	LifecycleAttemptCreated            LifecycleProjection = "attempt_created"
	LifecycleAttemptStarted            LifecycleProjection = "attempt_started"
	LifecycleAttemptPaused             LifecycleProjection = "attempt_paused"
	LifecycleAttemptStopped            LifecycleProjection = "attempt_stopped"
	LifecycleAttemptFailed             LifecycleProjection = "attempt_failed"
	LifecycleAttemptUnconfirmed        LifecycleProjection = "attempt_unconfirmed"
	LifecycleAttemptRunningUnconfirmed LifecycleProjection = "attempt_running_unconfirmed"
	LifecycleAttemptReconciled         LifecycleProjection = "attempt_reconciled"
	LifecycleAttemptVerified           LifecycleProjection = "attempt_verified"
	LifecycleActivityObserved          LifecycleProjection = "activity_observed"
	LifecycleOwnerInputRequested       LifecycleProjection = "owner_input_requested"
	LifecycleAttemptResumed            LifecycleProjection = "attempt_resumed"
	LifecycleAttemptReplaced           LifecycleProjection = "attempt_replaced"
	LifecycleRecoveryNeedsAttention    LifecycleProjection = "recovery_needs_attention"
)

func LifecycleForAttemptStatus(s AttemptStatus) (LifecycleProjection, bool) {
	switch s {
	case AttemptQueued:
		return LifecycleAttemptCreated, true
	case AttemptRunning:
		return LifecycleAttemptStarted, true
	case AttemptPaused:
		return LifecycleAttemptPaused, true
	case AttemptSucceeded:
		return LifecycleAttemptVerified, true
	case AttemptFailed:
		return LifecycleAttemptFailed, true
	case AttemptCancelled:
		return LifecycleAttemptStopped, true
	case AttemptLost:
		return LifecycleAttemptUnconfirmed, true
	case AttemptReconciled:
		return LifecycleAttemptReconciled, true
	}
	return "", false
}
func LifecycleForActivity(s ActivityState) (LifecycleProjection, bool) {
	switch s {
	case ActivityWaitingInput, ActivityBlocked:
		return LifecycleOwnerInputRequested, true
	case ActivityActive, ActivityIdle, ActivityExited:
		return LifecycleActivityObserved, true
	}
	return "", false
}
func LifecycleForRecovery(r RecoveryResolution) (LifecycleProjection, bool) {
	switch r {
	case RecoveryResumed:
		return LifecycleAttemptResumed, true
	case RecoveryReplacement:
		return LifecycleAttemptReplaced, true
	case RecoveryNeedsAttention:
		return LifecycleRecoveryNeedsAttention, true
	}
	return "", false
}
func LifecycleForObservation(k string) (LifecycleProjection, bool) {
	switch k {
	case ObservationExecutionUsage:
		return LifecycleAttemptStarted, true
	case ObservationBudgetExceeded:
		return LifecycleAttemptFailed, true
	case ObservationAttemptContained, ObservationOwnerContained, ObservationOwnerCancel:
		return LifecycleAttemptStopped, true
	case ObservationAttemptResumed, ObservationOwnerResume:
		return LifecycleAttemptResumed, true
	case ObservationProviderExit:
		return LifecycleAttemptReconciled, true
	case ObservationAttemptClassified:
		return LifecycleAttemptVerified, true
	case ObservationAdmissionFailed, ObservationInputProvisioningFailed:
		return LifecycleAttemptFailed, true
	case ObservationAdmissionAmbiguous:
		return LifecycleAttemptUnconfirmed, true
	case ObservationActivationAmbiguous, ObservationGovernedCheckTerminationUnknown, ObservationProviderStopFailed:
		return LifecycleAttemptRunningUnconfirmed, true
	case ObservationOwnerPause:
		return LifecycleAttemptPaused, true
	case ObservationRecoveryAttention:
		return LifecycleRecoveryNeedsAttention, true
	}
	return "", false
}
