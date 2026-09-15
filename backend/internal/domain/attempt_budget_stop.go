package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type RuntimeBudgetReasonCode string

const (
	RuntimeRetryBudgetExhausted    RuntimeBudgetReasonCode = "retry_budget_exhausted"
	RuntimeTokenBudgetExhausted    RuntimeBudgetReasonCode = "token_budget_exhausted"
	RuntimeWallTimeBudgetExhausted RuntimeBudgetReasonCode = "wall_time_budget_exhausted"
)

func (c RuntimeBudgetReasonCode) Valid() bool {
	return c == RuntimeRetryBudgetExhausted || c == RuntimeTokenBudgetExhausted || c == RuntimeWallTimeBudgetExhausted
}

type AttemptBudgetStop struct {
	AttemptID         AttemptID
	SessionID         string
	Reason            RuntimeBudgetReasonCode
	MeasuredUsage     string
	ClaimedAt         time.Time
	ProviderStoppedAt *time.Time
	MachineResult     string
}

func (s AttemptBudgetStop) Validate() error {
	if s.AttemptID.IsZero() || strings.TrimSpace(s.SessionID) == "" || !s.Reason.Valid() || s.ClaimedAt.IsZero() || !json.Valid([]byte(s.MeasuredUsage)) {
		return fmt.Errorf("budget stop claim is incomplete")
	}
	if s.ProviderStoppedAt != nil && !json.Valid([]byte(s.MachineResult)) {
		return fmt.Errorf("budget stop machine result is invalid")
	}
	return nil
}
func (s AttemptBudgetStop) ProviderStopped() bool { return s.ProviderStoppedAt != nil }

// CanonicalJSON preserves integer precision while normalizing object key order.
// Budget evidence uses exact counters, so decoding through float64 is forbidden.
func CanonicalJSON(raw string) (string, error) {
	dec := json.NewDecoder(bytes.NewBufferString(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return "", err
	}
	if dec.More() {
		return "", fmt.Errorf("multiple JSON values")
	}
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
func CanonicalJSONEqual(left, right string) bool {
	a, e1 := CanonicalJSON(left)
	b, e2 := CanonicalJSON(right)
	return e1 == nil && e2 == nil && a == b
}
