package devicebridge

import (
	"encoding/json"
	"testing"
)

// e2e 2026-10-13: macaction.Engine returns exit_code as a native int (and the
// ledger reloads it as float64). marshalCanonical only accepts json.Number, so
// persistResult failed with "invalid device bridge frame signature" on every
// reconnect: the result never left the Mac and heartbeats never landed.
func TestCanonicalResultPayloadAcceptsExecutorNumbers(t *testing.T) {
	payload := map[string]any{"status": "completed", "exit_code": 0, "output_tail": "ok\n", "truncated": false,
		"nested": map[string]any{"from_ledger": float64(7), "list": []any{int64(2)}}}
	out, err := canonicalResultPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	if out["exit_code"] != json.Number("0") || out["nested"].(map[string]any)["from_ledger"] != json.Number("7") {
		t.Fatalf("numbers not canonical: %#v", out)
	}
	if _, err := marshalCanonical(map[string]any{"payload": out}); err != nil {
		t.Fatalf("normalized payload must marshal canonically: %v", err)
	}
	for _, bad := range []map[string]any{{"exit_code": 1.5}, {"exit_code": func() {}}} {
		if _, err := canonicalResultPayload(bad); err == nil {
			t.Fatalf("non-integer payload accepted: %#v", bad)
		}
	}
}
