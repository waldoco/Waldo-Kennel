package intelligence

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

func readinessTestFence() domain.PlanningReadinessFence {
	return domain.PlanningReadinessFence{
		PlanningSessionID:  "ps-1",
		SessionRevision:    2,
		ContractRevisionID: "cr-9",
		ContextDigest:      domain.DigestSHA256([]byte("ctx")),
		RoutingSnapshotID:  "rs-1",
	}
}

const readyEnvelope = `{
  "status": "ready",
  "message": "Plan is ready for review.",
  "proposal": {
    "summary": "Ship the thing",
    "workUnits": [{
      "key": "verify-release",
      "title": "Verify the release",
      "intent": "inspect",
      "role": "investigate",
      "inputs": [],
      "outputSummary": "release facts",
      "criteriaCovered": ["C1"],
      "dependsOn": [],
      "evidenceIdeas": ["release log"],
      "checkCommands": null
    }],
    "assumptions": [],
    "blockers": []
  },
  "issues": []
}`

const needsContextEnvelope = `{
  "status": "needs_context",
  "message": "Two answers needed.",
  "proposal": null,
  "issues": [
    {
      "kind": "fact_missing",
      "prompt": "Which release channel?",
      "reason": "The verification graph depends on the channel.",
      "recommendation": "stable",
      "choices": [{"key": "stable", "label": "Stable"}, {"key": "beta", "label": "Beta"}],
      "workUnitKeys": ["verify-release"],
      "criterionAliases": ["C1"]
    },
    {
      "kind": "context_insufficient",
      "prompt": "Share the rollout doc?",
      "reason": "Proof obligations depend on the rollout stages.",
      "recommendation": "",
      "choices": [],
      "workUnitKeys": ["verify-release"],
      "criterionAliases": ["C2"]
    }
  ]
}`

func TestParsePlanningReadinessReadyEnvelope(t *testing.T) {
	result, err := parsePlanningReadinessReply([]byte(readyEnvelope), readinessTestFence(), []string{"verify-release"}, []string{"C1", "C2"})
	if err != nil {
		t.Fatalf("ready envelope: %v", err)
	}
	if result.Status != domain.PlanningReady || result.Proposal == nil || len(result.Issues) != 0 {
		t.Fatalf("ready shape = %+v", result)
	}
	if result.Version != domain.PlanningReadinessEnvelopeVersion {
		t.Fatalf("version = %q", result.Version)
	}
	if got := result.Proposal.WorkUnits[0].Key; got != "verify-release" {
		t.Fatalf("proposal unit key = %q", got)
	}
}

func TestParsePlanningReadinessNeedsContext(t *testing.T) {
	result, err := parsePlanningReadinessReply([]byte(needsContextEnvelope), readinessTestFence(), []string{"verify-release"}, []string{"C1", "C2"})
	if err != nil {
		t.Fatalf("needs_context envelope: %v", err)
	}
	if result.Status != domain.PlanningNeedsContext || result.Proposal != nil || len(result.Issues) != 2 {
		t.Fatalf("needs_context shape = %+v", result)
	}
	for _, issue := range result.Issues {
		if issue.Source != domain.ReadinessSourcePlannerDeclared {
			t.Fatalf("issue source = %q, want planner_declared", issue.Source)
		}
		if issue.Route != domain.RouteAnswerContext {
			t.Fatalf("planner-declared issue route = %q, want answer_context", issue.Route)
		}
		if !strings.HasPrefix(issue.Key, "pri-") {
			t.Fatalf("issue key %q missing pri- prefix", issue.Key)
		}
	}
	// The batch arrives together in deterministic order: kind rank orders
	// fact_missing before context_insufficient.
	if result.Issues[0].Kind != domain.ReadinessFactMissing || result.Issues[1].Kind != domain.ReadinessContextInsufficient {
		t.Fatalf("issue order = %s,%s", result.Issues[0].Kind, result.Issues[1].Kind)
	}

	// Replay under the same fence is byte-stable; a new fence is a new generation.
	again, err := parsePlanningReadinessReply([]byte(needsContextEnvelope), readinessTestFence(), []string{"verify-release"}, []string{"C1", "C2"})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	for i := range result.Issues {
		if result.Issues[i].Key != again.Issues[i].Key {
			t.Fatal("replay under the same fence minted different keys")
		}
	}
	moved := readinessTestFence()
	moved.SessionRevision = 3
	regen, err := parsePlanningReadinessReply([]byte(needsContextEnvelope), moved, []string{"verify-release"}, []string{"C1", "C2"})
	if err != nil {
		t.Fatalf("regeneration: %v", err)
	}
	for i := range result.Issues {
		if result.Issues[i].Key == regen.Issues[i].Key {
			t.Fatal("fence change did not mint a new issue generation")
		}
	}
}

func TestParsePlanningReadinessRejectsUnknownTaxonomy(t *testing.T) {
	replaceKind := func(kind string) string {
		return strings.Replace(needsContextEnvelope, `"kind": "fact_missing"`, `"kind": "`+kind+`"`, 1)
	}
	cases := map[string]struct {
		envelope string
		wantCode string
	}{
		"unknown status": {
			envelope: strings.Replace(needsContextEnvelope, `"status": "needs_context"`, `"status": "maybe_later"`, 1),
			wantCode: string(domain.ReadinessStatusInvalid),
		},
		"unknown kind": {
			envelope: replaceKind("vibes_missing"),
			wantCode: string(domain.ReadinessIssueInvalid),
		},
		"planner declares control-plane kind": {
			envelope: replaceKind("connector_missing"),
			wantCode: string(domain.ReadinessAuthorityClaim),
		},
		"provider-selected route": {
			envelope: strings.Replace(needsContextEnvelope, `"kind": "fact_missing"`, `"kind": "fact_missing", "route": "revise_contract"`, 1),
			wantCode: string(domain.ReadinessAuthorityClaim),
		},
		"capability claim": {
			envelope: strings.Replace(needsContextEnvelope, `"kind": "fact_missing"`, `"kind": "fact_missing", "requestedCapability": "kennel.exec"`, 1),
			wantCode: string(domain.ReadinessAuthorityClaim),
		},
		"connector claim": {
			envelope: strings.Replace(needsContextEnvelope, `"kind": "fact_missing"`, `"kind": "fact_missing", "connectorClass": "issue_tracker"`, 1),
			wantCode: string(domain.ReadinessAuthorityClaim),
		},
		"owner answer": {
			envelope: strings.Replace(needsContextEnvelope, `"kind": "fact_missing"`, `"kind": "fact_missing", "ownerAnswer": "just ship it"`, 1),
			wantCode: string(domain.ReadinessAuthorityClaim),
		},
		"benign unknown field": {
			envelope: strings.Replace(needsContextEnvelope, `"kind": "fact_missing"`, `"kind": "fact_missing", "sessionHint": "xyz"`, 1),
			wantCode: string(domain.ReadinessPayloadInvalid),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parsePlanningReadinessReply([]byte(tc.envelope), readinessTestFence(), []string{"verify-release"}, []string{"C1", "C2"})
			if err == nil {
				t.Fatalf("%s was accepted", name)
			}
			if !strings.Contains(err.Error(), tc.wantCode) {
				t.Fatalf("%s error = %v, want code %s", name, err, tc.wantCode)
			}
		})
	}
}

func TestParsePlanningReadinessShapeRules(t *testing.T) {
	cases := map[string]string{
		"ready with issues":       `"status": "ready", "message": "x", "proposal": null, "issues": []`,
		"needs_context no issues": `"status": "needs_context", "message": "x", "proposal": null, "issues": []`,
		"blank message":           `"status": "blocked", "message": " ", "proposal": null, "issues": [{"kind":"fact_missing","prompt":"p","reason":"r","recommendation":"","choices":[],"workUnitKeys":[],"criterionAliases":[]}]`,
		"blank prompt":            `"status": "needs_context", "message": "m", "proposal": null, "issues": [{"kind":"fact_missing","prompt":" ","reason":"r","recommendation":"","choices":[],"workUnitKeys":[],"criterionAliases":[]}]`,
		"blank reason":            `"status": "needs_context", "message": "m", "proposal": null, "issues": [{"kind":"fact_missing","prompt":"p","reason":"","recommendation":"","choices":[],"workUnitKeys":[],"criterionAliases":[]}]`,
		"blocked only answerable": `"status": "blocked", "message": "m", "proposal": null, "issues": [{"kind":"fact_missing","prompt":"p","reason":"r","recommendation":"","choices":[],"workUnitKeys":[],"criterionAliases":[]}]`,
		"duplicate choice keys":   `"status": "needs_context", "message": "m", "proposal": null, "issues": [{"kind":"fact_missing","prompt":"p","reason":"r","recommendation":"","choices":[{"key":"a","label":"A"},{"key":"a","label":"again"}],"workUnitKeys":[],"criterionAliases":[]}]`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parsePlanningReadinessReply([]byte("{"+body+"}"), readinessTestFence(), nil, []string{"C1", "C2"}); err == nil {
				t.Fatalf("%s was accepted", name)
			}
		})
	}
}

func TestPlanningReadinessSchemaCarriesNoAuthoritySurface(t *testing.T) {
	schema := planningReadinessSchema([]string{"C1", "C2"})
	encoded := mustJSON(t, schema)
	for _, field := range authorityClaimFields {
		if strings.Contains(string(encoded), `"`+field+`"`) {
			t.Fatalf("readiness schema exposes authority-bearing field %q", field)
		}
	}
	props := schema["properties"].(map[string]any)
	if props["additionalProperties"] == true {
		t.Fatal("envelope schema is not closed")
	}
	issueProps := props["issues"].(map[string]any)["items"].(map[string]any)
	if issueProps["additionalProperties"] != false {
		t.Fatal("issue schema is not closed")
	}
	kindEnum := issueProps["properties"].(map[string]any)["kind"].(map[string]any)["enum"].([]any)
	if len(kindEnum) != 2 {
		t.Fatalf("planner-declarable kinds = %v, want fact_missing and context_insufficient only", kindEnum)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	out, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}

func TestParsePlanningReadinessRejectsTrailingData(t *testing.T) {
	// A trailing object carrying authority is rejected, not ignored.
	_, err := parsePlanningReadinessReply([]byte(readyEnvelope+`
{"route":"revise_contract"}`),
		readinessTestFence(), nil, nil)
	if err == nil || !strings.Contains(err.Error(), string(domain.ReadinessPayloadInvalid)) {
		t.Fatalf("trailing object error = %v", err)
	}
	// A trailing scalar is rejected too.
	if _, err := parsePlanningReadinessReply([]byte(readyEnvelope+` 42`),
		readinessTestFence(), nil, nil); err == nil {
		t.Fatal("trailing scalar accepted")
	}
	// Trailing whitespace and newlines are fine.
	if _, err := parsePlanningReadinessReply([]byte(readyEnvelope+" \n\t "),
		readinessTestFence(), nil, []string{"C1"}); err != nil {
		t.Fatalf("trailing whitespace: %v", err)
	}
}

func TestParsePlanningReadinessFencesPlannerReferences(t *testing.T) {
	// Unknown WorkUnit key reference is rejected.
	unknown := strings.Replace(needsContextEnvelope, `"verify-release"`, `"invented-unit"`, 1)
	if _, err := parsePlanningReadinessReply([]byte(unknown),
		readinessTestFence(), []string{"verify-release"}, []string{"C1", "C2"}); err == nil {
		t.Fatal("unknown work unit key accepted")
	}
	// Unknown criterion alias reference is rejected even though it parses as a string.
	unknown = strings.Replace(needsContextEnvelope, `"criterionAliases": ["C1"]`, `"criterionAliases": ["C99"]`, 1)
	if _, err := parsePlanningReadinessReply([]byte(unknown),
		readinessTestFence(), []string{"verify-release"}, []string{"C1", "C2"}); err == nil {
		t.Fatal("unknown criterion alias accepted")
	}
	// Duplicate references are rejected by the domain contract.
	dup := strings.Replace(needsContextEnvelope, `"criterionAliases": ["C1"]`, `"criterionAliases": ["C1", "C1"]`, 1)
	if _, err := parsePlanningReadinessReply([]byte(dup),
		readinessTestFence(), []string{"verify-release"}, []string{"C1", "C2"}); err == nil {
		t.Fatal("duplicate criterion alias accepted")
	}
	// Blank references are rejected.
	blank := strings.Replace(needsContextEnvelope, `"workUnitKeys": ["verify-release"]`, `"workUnitKeys": [" "]`, 1)
	if _, err := parsePlanningReadinessReply([]byte(blank),
		readinessTestFence(), []string{"verify-release"}, []string{"C1", "C2"}); err == nil {
		t.Fatal("blank work unit key accepted")
	}
}
