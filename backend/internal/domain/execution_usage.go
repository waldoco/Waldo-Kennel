package domain

import (
	"fmt"
	"strings"
	"time"
)

// ExecutionUsageSample is a provider's monotonic cumulative counter at the
// governed Attempt boundary. It never includes planning/intelligence usage.
type ExecutionUsageSample struct {
	AttemptID    AttemptID
	Provider     AgentHarness
	SessionID    string
	Sequence     int64
	InputTokens  int64
	OutputTokens int64
	InputDelta   int64
	OutputDelta  int64
	CreatedAt    time.Time
}

func (s ExecutionUsageSample) ValidateCumulative() error {
	if s.AttemptID.IsZero() || strings.TrimSpace(string(s.Provider)) == "" || strings.TrimSpace(s.SessionID) == "" {
		return fmt.Errorf("execution usage requires attempt, provider, and session identity")
	}
	if s.Sequence < 1 || s.InputTokens < 0 || s.OutputTokens < 0 {
		return fmt.Errorf("execution usage counters and sequence must be non-negative")
	}
	if s.CreatedAt.IsZero() {
		return fmt.Errorf("execution usage timestamp is required")
	}
	return nil
}

func (s ExecutionUsageSample) TotalTokens() int64 { return s.InputTokens + s.OutputTokens }

type ExecutionUsageTotals struct{ InputTokens, OutputTokens int64 }

func (t ExecutionUsageTotals) TotalTokens() int64 { return t.InputTokens + t.OutputTokens }
