package intelligence

import (
	"encoding/json"
	"errors"
	"log/slog"
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
      "workUnitKeys": [],
      "criterionAliases": ["C1"]
    },
    {
      "kind": "context_insufficient",
      "prompt": "Share the rollout doc?",
      "reason": "Proof obligations depend on the rollout stages.",
      "recommendation": "",
      "choices": [],
      "workUnitKeys": [],
      "criterionAliases": ["C2"]
    }
  ]
}`

func TestParsePlanningReadinessReadyEnvelope(t *testing.T) {
	result, err := parsePlanningReadinessReply([]byte(readyEnvelope), readinessTestFence(), []string{"C1", "C2"})
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
	result, err := parsePlanningReadinessReply([]byte(needsContextEnvelope), readinessTestFence(), []string{"C1", "C2"})
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
	again, err := parsePlanningReadinessReply([]byte(needsContextEnvelope), readinessTestFence(), []string{"C1", "C2"})
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
	regen, err := parsePlanningReadinessReply([]byte(needsContextEnvelope), moved, []string{"C1", "C2"})
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
			_, err := parsePlanningReadinessReply([]byte(tc.envelope), readinessTestFence(), []string{"C1", "C2"})
			if err == nil {
				t.Fatalf("%s was accepted", name)
			}
			if !strings.Contains(err.Error(), tc.wantCode) {
				t.Fatalf("%s error = %v, want code %s", name, err, tc.wantCode)
			}
		})
	}
}

func TestParsePlanningReadinessUnitReferenceNeedsProposal(t *testing.T) {
	// A no-proposal envelope defines no units, so a planner reference to one
	// cannot resolve - but on the sessionless one-shot lane an unresolvable
	// reference on an advisory issue must degrade, not 500 plan creation
	// (run 35306197815: "waldo referenced unknown work unit key W1" 500ed
	// POST /outcomes/:id/plans). The reference is stripped, the planner's
	// issue survives reference-free, and one control-plane degrade issue
	// records the drop; no invented key reaches the evaluated result.
	res, err := parsePlanningReadinessReply([]byte(`{
	  "status": "needs_context",
	  "message": "One answer needed.",
	  "proposal": null,
	  "issues": [{"kind": "fact_missing", "prompt": "Which release channel?", "reason": "The verification graph depends on it.", "recommendation": "", "choices": [], "workUnitKeys": ["verify-release"], "criterionAliases": []}]
	}`), readinessTestFence(), []string{"C1", "C2"})
	if err != nil {
		t.Fatalf("unresolvable reference must degrade, not invalidate: %v", err)
	}
	var plannerIssue, degradeIssue *domain.PlanningReadinessIssue
	for i := range res.Issues {
		switch res.Issues[i].Kind {
		case domain.ReadinessFactMissing:
			plannerIssue = &res.Issues[i]
		case domain.ReadinessReferenceUnresolvable:
			degradeIssue = &res.Issues[i]
		}
	}
	if plannerIssue == nil {
		t.Fatal("the planner's issue must survive the degrade")
	}
	if len(plannerIssue.WorkUnitKeys) != 0 {
		t.Fatalf("invented key rode the issue into the result: %v", plannerIssue.WorkUnitKeys)
	}
	if degradeIssue == nil {
		t.Fatal("the drop must surface as a control-plane degrade issue")
	}
	if degradeIssue.Source != domain.ReadinessSourceControlPlane {
		t.Fatalf("degrade issue source = %s, want control_plane", degradeIssue.Source)
	}
	if degradeIssue.Route != domain.RouteAnswerContext {
		t.Fatalf("needs_context packets admit only answer_context routes, got %s", degradeIssue.Route)
	}
	if len(degradeIssue.WorkUnitKeys) != 0 || len(degradeIssue.CriterionAliases) != 0 {
		t.Fatal("the degrade issue itself must carry no references")
	}
	if !strings.Contains(degradeIssue.Prompt, "verify-release") {
		t.Fatal("the dropped key must be named in the degrade issue's prose")
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
			if _, err := parsePlanningReadinessReply([]byte("{"+body+"}"), readinessTestFence(), []string{"C1", "C2"}); err == nil {
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
		readinessTestFence(), nil)
	if err == nil || !strings.Contains(err.Error(), string(domain.ReadinessPayloadInvalid)) {
		t.Fatalf("trailing object error = %v", err)
	}
	// A trailing scalar is rejected too.
	if _, err := parsePlanningReadinessReply([]byte(readyEnvelope+` 42`),
		readinessTestFence(), nil); err == nil {
		t.Fatal("trailing scalar accepted")
	}
	// Trailing whitespace and newlines are fine.
	if _, err := parsePlanningReadinessReply([]byte(readyEnvelope+" \n\t "),
		readinessTestFence(), []string{"C1"}); err != nil {
		t.Fatalf("trailing whitespace: %v", err)
	}
}

func TestParsePlanningReadinessFencesPlannerReferences(t *testing.T) {
	// A WorkUnit key reference the envelope does not define is DEGRADED
	// (stripped + control-plane degrade issue), never carried through.
	unknown := strings.Replace(needsContextEnvelope, `"criterionAliases": ["C1"]`, `"workUnitKeys": ["verify-release"], "criterionAliases": ["C1"]`, 1)
	res, err := parsePlanningReadinessReply([]byte(unknown),
		readinessTestFence(), []string{"C1", "C2"})
	if err != nil {
		t.Fatalf("unresolvable unit reference must degrade: %v", err)
	}
	assertRefsStripped(t, res)
	// Unknown criterion alias references degrade the same way.
	unknown = strings.Replace(needsContextEnvelope, `"criterionAliases": ["C1"]`, `"criterionAliases": ["C99"]`, 1)
	res, err = parsePlanningReadinessReply([]byte(unknown),
		readinessTestFence(), []string{"C1", "C2"})
	if err != nil {
		t.Fatalf("unresolvable alias reference must degrade: %v", err)
	}
	assertRefsStripped(t, res)
	// The fence still bites for defined keys: a reference to a unit the
	// proposal DOES define survives untouched (strictness unchanged for
	// resolvable references).
	// Duplicate references are rejected by the domain contract.
	dup := strings.Replace(needsContextEnvelope, `"criterionAliases": ["C1"]`, `"criterionAliases": ["C1", "C1"]`, 1)
	if _, err := parsePlanningReadinessReply([]byte(dup),
		readinessTestFence(), []string{"C1", "C2"}); err == nil {
		t.Fatal("duplicate criterion alias accepted")
	}
	// Blank references are rejected.
	blank := strings.Replace(needsContextEnvelope, `"criterionAliases": ["C1"]`, `"workUnitKeys": [" "], "criterionAliases": ["C1"]`, 1)
	if _, err := parsePlanningReadinessReply([]byte(blank),
		readinessTestFence(), []string{"C1", "C2"}); err == nil {
		t.Fatal("blank work unit key accepted")
	}
}

// assertRefsStripped fails unless every planner-declared issue in res carries
// only envelope-defined references and exactly one control-plane degrade issue
// recorded a drop.
func assertRefsStripped(t *testing.T, res domain.PlanningReadinessResult) {
	t.Helper()
	degrades := 0
	for _, issue := range res.Issues {
		if issue.Kind == domain.ReadinessReferenceUnresolvable {
			degrades++
			if issue.Source != domain.ReadinessSourceControlPlane {
				t.Fatalf("degrade issue source = %s, want control_plane", issue.Source)
			}
			continue
		}
		for _, key := range issue.WorkUnitKeys {
			if key == "verify-release" {
				t.Fatalf("invented unit key rode issue into result: %v", issue.WorkUnitKeys)
			}
		}
		for _, alias := range issue.CriterionAliases {
			if alias != "C1" && alias != "C2" {
				t.Fatalf("invented criterion alias rode issue into result: %v", issue.CriterionAliases)
			}
		}
	}
	if degrades != 1 {
		t.Fatalf("degrade issues = %d, want exactly 1 per envelope", degrades)
	}
}

// TestParsePlanningReadinessSelectiveDegrade: within one issue, references
// that resolve against the Contract's frozen aliases survive while
// references the envelope cannot define are stripped - the fence bites
// selectively, it does not blanket-drop. (Unit keys can never resolve in a
// planner envelope: planner issues are only legal in needs_context packets,
// which carry no proposal - the degrade is what keeps that shape alive.)
func TestParsePlanningReadinessSelectiveDegrade(t *testing.T) {
	envelope := `{
	  "status": "needs_context",
	  "message": "One answer needed.",
	  "proposal": null,
	  "issues": [{"kind": "fact_missing", "prompt": "Which release channel?", "reason": "The verification graph depends on it.", "recommendation": "", "choices": [], "workUnitKeys": ["verify-release"], "criterionAliases": ["C1", "C99"]}]
	}`
	res, err := parsePlanningReadinessReply([]byte(envelope), readinessTestFence(), []string{"C1", "C2"})
	if err != nil {
		t.Fatalf("mixed references must degrade selectively: %v", err)
	}
	var plannerIssue *domain.PlanningReadinessIssue
	degrades := 0
	for i := range res.Issues {
		if res.Issues[i].Kind == domain.ReadinessReferenceUnresolvable {
			degrades++
			continue
		}
		plannerIssue = &res.Issues[i]
	}
	if plannerIssue == nil {
		t.Fatal("planner issue lost")
	}
	if len(plannerIssue.CriterionAliases) != 1 || plannerIssue.CriterionAliases[0] != "C1" {
		t.Fatalf("defined alias C1 must survive, got %v", plannerIssue.CriterionAliases)
	}
	if len(plannerIssue.WorkUnitKeys) != 0 {
		t.Fatalf("undefined unit key must be stripped, got %v", plannerIssue.WorkUnitKeys)
	}
	if degrades != 1 {
		t.Fatalf("degrade issues = %d, want 1", degrades)
	}
}

// TestParsePlanningReadinessDuplicateUnknownRefsRejected: duplicates are
// malformed, not unresolvable - they must be caught on the RAW reference
// lists before stripping, so ["ghost","ghost"] cannot degrade into a valid
// envelope (review MEDIUM on d9e45405).
func TestParsePlanningReadinessDuplicateUnknownRefsRejected(t *testing.T) {
	dupUnknown := strings.Replace(needsContextEnvelope, `"criterionAliases": ["C1"]`, `"workUnitKeys": ["ghost", "ghost"], "criterionAliases": ["C1"]`, 1)
	_, err := parsePlanningReadinessReply([]byte(dupUnknown), readinessTestFence(), []string{"C1", "C2"})
	if err == nil || !strings.Contains(err.Error(), string(domain.ReadinessIssueDuplicate)) {
		t.Fatalf("duplicate unknown unit keys must be hard-invalid, err = %v", err)
	}
	// Mixed: a duplicate formed across known and unknown refs is still a dup.
	dupMixed := strings.Replace(needsContextEnvelope, `"criterionAliases": ["C1"]`, `"criterionAliases": ["C1", "ghost", "C1"]`, 1)
	_, err = parsePlanningReadinessReply([]byte(dupMixed), readinessTestFence(), []string{"C1", "C2"})
	if err == nil || !strings.Contains(err.Error(), string(domain.ReadinessIssueDuplicate)) {
		t.Fatalf("mixed known/unknown duplicate aliases must be hard-invalid, err = %v", err)
	}
}

// TestParsePlanningReadinessMalformedRefsRejected: nonblank references that
// violate the closed identifier contract (control chars, oversized,
// out-of-alphabet) are rejected by ValidateReadinessIdentifier BEFORE
// partitioning - never laundered into degrade prose or logs (review
// adversarial note on d9e45405).
func TestParsePlanningReadinessMalformedRefsRejected(t *testing.T) {
	cases := map[string]string{
		"control character": `"workUnitKeys": ["bad\nkey"], "criterionAliases": ["C1"]`,
		"out of alphabet":   `"workUnitKeys": ["bad key!"], "criterionAliases": ["C1"]`,
		"oversized":         `"workUnitKeys": ["` + strings.Repeat("a", domain.MaxPlanningReadinessIdentifierLength+1) + `"], "criterionAliases": ["C1"]`,
	}
	for name, refs := range cases {
		t.Run(name, func(t *testing.T) {
			env := strings.Replace(needsContextEnvelope, `"criterionAliases": ["C1"]`, refs, 1)
			if _, err := parsePlanningReadinessReply([]byte(env), readinessTestFence(), []string{"C1", "C2"}); err == nil {
				t.Fatal("malformed reference must be rejected, not degraded")
			}
		})
	}
}

// TestLogReadinessInvalidNeverLogsProviderContent (review HIGH on d9e45405):
// the invalid-envelope WARN carries the typed error plus structural metadata
// (byte length, truncated digest) and NEVER raw model output.
func TestLogReadinessInvalidNeverLogsProviderContent(t *testing.T) {
	var buf strings.Builder
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	secret := "sk-live-secret-bearer-token-value"
	logReadinessInvalid([]byte(`{"status":"ready","token":"`+secret+`"}`), errors.New("typed validation error"))

	out := buf.String()
	if strings.Contains(out, secret) || strings.Contains(out, `"status"`) {
		t.Fatalf("provider-controlled content reached the log: %s", out)
	}
	for _, want := range []string{"typed validation error", "envelope_bytes", "envelope_sha256"} {
		if !strings.Contains(out, want) {
			t.Fatalf("log line missing %q: %s", want, out)
		}
	}
}
