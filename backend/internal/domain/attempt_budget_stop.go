package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
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

// CanonicalJSON preserves exact numeric value while normalizing object key order.
// JSON numbers are finite decimals, so each is reduced to an arbitrary-precision
// integer coefficient and base-10 exponent. No float conversion occurs.
func CanonicalJSON(raw string) (string, error) {
	dec := json.NewDecoder(bytes.NewBufferString(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return "", err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return "", fmt.Errorf("multiple JSON values")
		}
		return "", err
	}
	normalized, err := canonicalJSONValue(value)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

type canonicalJSONNumber string

func (n canonicalJSONNumber) MarshalJSON() ([]byte, error) { return []byte(n), nil }

func canonicalJSONValue(value any) (any, error) {
	switch v := value.(type) {
	case json.Number:
		n, err := canonicalizeJSONNumber(v.String())
		if err != nil {
			return nil, err
		}
		return canonicalJSONNumber(n), nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			n, err := canonicalJSONValue(item)
			if err != nil {
				return nil, err
			}
			out[i] = n
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			n, err := canonicalJSONValue(item)
			if err != nil {
				return nil, err
			}
			out[key] = n
		}
		return out, nil
	default:
		return value, nil
	}
}

func canonicalizeJSONNumber(raw string) (string, error) {
	sign := ""
	if len(raw) > 0 && raw[0] == '-' {
		sign = "-"
		raw = raw[1:]
	}
	mantissa, expText := raw, ""
	for i, c := range raw {
		if c == 'e' || c == 'E' {
			mantissa, expText = raw[:i], raw[i+1:]
			break
		}
	}
	exp := new(big.Int)
	if expText != "" {
		if _, ok := exp.SetString(expText, 10); !ok {
			return "", fmt.Errorf("invalid JSON number exponent")
		}
	}
	fraction := int64(0)
	digits := mantissa
	if dot := strings.IndexByte(mantissa, '.'); dot >= 0 {
		fraction = int64(len(mantissa) - dot - 1)
		digits = mantissa[:dot] + mantissa[dot+1:]
	}
	// Strip coefficient zeros textually before big.Int parsing. A numeric
	// coefficient with N trailing zeros changes only the decimal exponent by N.
	// This is one linear scan instead of N successively smaller big divisions.
	trimmedEnd := len(digits)
	for trimmedEnd > 0 && digits[trimmedEnd-1] == '0' {
		trimmedEnd--
	}
	if trimmedEnd == 0 {
		return "0", nil
	}
	trimmed := digits[:trimmedEnd]
	coefficient := new(big.Int)
	if _, ok := coefficient.SetString(trimmed, 10); !ok {
		return "", fmt.Errorf("invalid JSON number %q", raw)
	}
	if sign == "-" {
		coefficient.Neg(coefficient)
	}
	scale := new(big.Int).Sub(exp, big.NewInt(fraction))
	scale.Add(scale, new(big.Int).SetUint64(uint64(len(digits)-trimmedEnd)))
	if scale.Sign() == 0 {
		return coefficient.String(), nil
	}
	return coefficient.String() + "e" + scale.String(), nil
}

func CanonicalJSONEqual(left, right string) bool {
	a, e1 := CanonicalJSON(left)
	b, e2 := CanonicalJSON(right)
	return e1 == nil && e2 == nil && a == b
}
