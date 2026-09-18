package e2e

import "testing"

// The classifier owns the typed verdict a probe emits when its attempts
// exhaust without an evidenced denial. The discrimination matters: blaming
// model narration (invocation_absent) when positive controls failed would
// misclassify a harness failure as a model finding.
func TestClassifyProbeExhaustion(t *testing.T) {
	t.Run("zero driver turns from control failures", func(t *testing.T) {
		if got := classifyProbeExhaustion(0, 0); got != probeExhaustionControlUnattested {
			t.Fatalf("classifyProbeExhaustion(0, 0) = %v, want control_unattested", got)
		}
	})
	t.Run("mixed control failures and zero-event driver turns", func(t *testing.T) {
		for _, driverTurns := range []int{1, 2} {
			if got := classifyProbeExhaustion(driverTurns, 0); got != probeExhaustionControlUnattested {
				t.Fatalf("classifyProbeExhaustion(%d, 0) = %v, want control_unattested", driverTurns, got)
			}
		}
	})
	t.Run("three genuine zero-event driver turns", func(t *testing.T) {
		if got := classifyProbeExhaustion(3, 0); got != probeExhaustionInvocationAbsent {
			t.Fatalf("classifyProbeExhaustion(3, 0) = %v, want invocation_absent", got)
		}
	})
	t.Run("nonmatching driver event falls through to generic probe failure", func(t *testing.T) {
		for _, tc := range [][2]int{{3, 1}, {2, 1}, {1, 2}, {3, 3}} {
			if got := classifyProbeExhaustion(tc[0], tc[1]); got != probeExhaustionNoExactMatch {
				t.Fatalf("classifyProbeExhaustion(%d, %d) = %v, want generic probe failure", tc[0], tc[1], got)
			}
		}
	})
}
