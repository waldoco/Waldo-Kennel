package domain

import (
	"testing"
	"time"
)

func TestExecutionUsageSampleRequiresProviderSessionIdentity(t *testing.T) {
	s := ExecutionUsageSample{AttemptID: "a", Provider: HarnessCodex, SessionID: "s", Sequence: 1, CreatedAt: time.Now()}
	if err := s.ValidateCumulative(); err != nil {
		t.Fatal(err)
	}
	s.Sequence = 0
	if err := s.ValidateCumulative(); err == nil {
		t.Fatal("zero sequence accepted")
	}
}
