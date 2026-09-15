package domain

import "testing"

func TestCanonicalJSONNumbersCompareByExactMathematicalValue(t *testing.T) {
	equal := [][2]string{
		{`{"n":1}`, `{"n":1.0}`},
		{`{"n":1e0}`, `{"n":0.10e1}`},
		{`{"n":-1}`, `{"n":-10e-1}`},
		{`{"n":0}`, `{"n":-0.000e99}`},
		{`{"n":125e-2}`, `{"n":1.2500}`},
		{`{"n":1e999999999999999999999}`, `{"n":10e999999999999999999998}`},
	}
	for _, pair := range equal {
		if !CanonicalJSONEqual(pair[0], pair[1]) {
			t.Errorf("not equal: %s %s", pair[0], pair[1])
		}
	}
	if CanonicalJSONEqual(`{"n":1}`, `{"n":1.0000000000000000001}`) {
		t.Fatal("different exact values compared equal")
	}
}

func TestCanonicalJSONPreservesLargeIntegerPrecision(t *testing.T) {
	left := `{"tokens":9007199254740993,"nested":{"b":2,"a":1}}`
	right := `{"nested":{"a":1.0,"b":2e0},"tokens":9007199254740993.0}`
	if !CanonicalJSONEqual(left, right) {
		t.Fatal("canonical claim comparison lost exact structural equality")
	}
	if CanonicalJSONEqual(left, `{"tokens":9007199254740992,"nested":{"a":1,"b":2}}`) {
		t.Fatal("large counters compared through float precision")
	}
}
