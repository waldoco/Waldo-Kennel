package domain

import "testing"

func TestCanonicalJSONPreservesLargeIntegerPrecision(t *testing.T) {
	left := `{"tokens":9007199254740993,"nested":{"b":2,"a":1}}`
	right := `{"nested":{"a":1,"b":2},"tokens":9007199254740993}`
	if !CanonicalJSONEqual(left, right) {
		t.Fatal("canonical claim comparison lost structural equality")
	}
	if CanonicalJSONEqual(left, `{"tokens":9007199254740992,"nested":{"a":1,"b":2}}`) {
		t.Fatal("large counters compared through float precision")
	}
}
