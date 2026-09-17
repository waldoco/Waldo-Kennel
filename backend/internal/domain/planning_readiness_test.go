package domain

import (
	"strings"
	"testing"
)

func readinessFence() PlanningReadinessFence {
	return PlanningReadinessFence{
		PlanningSessionID:  "ps-1",
		SessionRevision:    3,
		ContractRevisionID: "cr-1",
		ContextDigest:      DigestSHA256([]byte("context")),
		RoutingSnapshotID:  "rs-1",
	}
}

func validIssue(kind PlanningReadinessIssueKind, route PlanningEscalationRoute) PlanningReadinessIssue {
	issue := PlanningReadinessIssue{
		Key:    "pri-test",
		Kind:   kind,
		Route:  route,
		Source: ReadinessSourceControlPlane,
		Prompt: "What should happen?",
		Reason: "It changes the graph.",
	}
	switch kind {
	case ReadinessFactMissing, ReadinessContextInsufficient:
		issue.Source = ReadinessSourcePlannerDeclared
	case ReadinessAuthorityInsufficient:
		issue.RequestedCapability = CapabilityWorktreeWrite
	case ReadinessConnectorMissing:
		issue.ConnectorClass = "issue_tracker"
	case ReadinessWorkerUnavailable:
		issue.AdmissionReasonCodes = []AdmissionReasonCode{AdmissionProviderUnavailable}
	}
	return issue
}

func validationCode(t *testing.T, err error) PlanningReadinessValidationCode {
	t.Helper()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	coded, ok := err.(*PlanningReadinessValidationError)
	if !ok {
		t.Fatalf("error %v is not a coded readiness validation error", err)
	}
	return coded.Code
}

func TestPlanningReadinessStatusPayloadShapes(t *testing.T) {
	ready := PlanningReadinessResult{
		Version: PlanningReadinessEnvelopeVersion, Status: PlanningReady,
		Message: "Planned.", Proposal: &PlanDraftProposal{Summary: "do the thing"},
	}
	if err := ready.Validate(); err != nil {
		t.Fatalf("ready envelope: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*PlanningReadinessResult)
		code   PlanningReadinessValidationCode
	}{
		{"unknown status", func(r *PlanningReadinessResult) { r.Status = "maybe" }, ReadinessStatusInvalid},
		{"blank message", func(r *PlanningReadinessResult) { r.Message = "  " }, ReadinessPayloadInvalid},
		{"ready without proposal", func(r *PlanningReadinessResult) { r.Proposal = nil }, ReadinessPayloadInvalid},
		{"ready with issues", func(r *PlanningReadinessResult) {
			r.Issues = []PlanningReadinessIssue{validIssue(ReadinessFactMissing, RouteAnswerContext)}
		}, ReadinessPayloadInvalid},
		{"needs_context with proposal", func(r *PlanningReadinessResult) {
			r.Status = PlanningNeedsContext
			r.Issues = []PlanningReadinessIssue{validIssue(ReadinessFactMissing, RouteAnswerContext)}
		}, ReadinessPayloadInvalid},
		{"needs_context without issues", func(r *PlanningReadinessResult) {
			r.Status, r.Proposal = PlanningNeedsContext, nil
		}, ReadinessPayloadInvalid},
		{"needs_context with operational route", func(r *PlanningReadinessResult) {
			r.Status, r.Proposal = PlanningNeedsContext, nil
			r.Issues = []PlanningReadinessIssue{validIssue(ReadinessConnectorMissing, RouteConfigureConnector)}
		}, ReadinessPayloadInvalid},
		{"blocked without issues", func(r *PlanningReadinessResult) {
			r.Status, r.Proposal = PlanningBlocked, nil
		}, ReadinessPayloadInvalid},
		{"blocked with only answerable issues", func(r *PlanningReadinessResult) {
			r.Status, r.Proposal = PlanningBlocked, nil
			r.Issues = []PlanningReadinessIssue{validIssue(ReadinessFactMissing, RouteAnswerContext)}
		}, ReadinessPayloadInvalid},
		{"blocked with proposal", func(r *PlanningReadinessResult) {
			r.Status = PlanningBlocked
			r.Issues = []PlanningReadinessIssue{validIssue(ReadinessConnectorMissing, RouteConfigureConnector)}
		}, ReadinessPayloadInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := ready
			tc.mutate(&result)
			if got := validationCode(t, result.Validate()); got != tc.code {
				t.Fatalf("code = %q, want %q (err: %v)", got, tc.code, result.Validate())
			}
		})
	}

	// Mixed answerable + operational issues normalize to one blocked packet.
	mixed := PlanningReadinessResult{
		Version: PlanningReadinessEnvelopeVersion, Status: PlanningBlocked, Message: "Setup needed.",
		Issues: []PlanningReadinessIssue{
			validIssue(ReadinessFactMissing, RouteAnswerContext),
			validIssue(ReadinessConnectorMissing, RouteConfigureConnector),
		},
	}
	mixed.Issues[0].Key = "pri-a"
	mixed.Issues[1].Key = "pri-b"
	if err := mixed.Validate(); err != nil {
		t.Fatalf("mixed packet must remain valid as blocked: %v", err)
	}

	needsContext := PlanningReadinessResult{
		Version: PlanningReadinessEnvelopeVersion, Status: PlanningNeedsContext, Message: "One answer needed.",
		Issues: []PlanningReadinessIssue{validIssue(ReadinessFactMissing, RouteAnswerContext)},
	}
	if err := needsContext.Validate(); err != nil {
		t.Fatalf("needs_context envelope: %v", err)
	}
}

func TestPlanningReadinessIssueTaxonomyClosed(t *testing.T) {
	base := validIssue(ReadinessFactMissing, RouteAnswerContext)

	unknownKind := base
	unknownKind.Kind = "vibes_missing"
	if got := validationCode(t, unknownKind.Validate()); got != ReadinessIssueInvalid {
		t.Fatalf("unknown kind code = %q", got)
	}
	unknownRoute := base
	unknownRoute.Route = "call_the_owner"
	if got := validationCode(t, unknownRoute.Validate()); got != ReadinessIssueInvalid {
		t.Fatalf("unknown route code = %q", got)
	}
	unknownSource := base
	unknownSource.Source = "telemetry"
	if got := validationCode(t, unknownSource.Validate()); got != ReadinessIssueInvalid {
		t.Fatalf("unknown source code = %q", got)
	}
	blankPrompt := base
	blankPrompt.Prompt = " "
	if got := validationCode(t, blankPrompt.Validate()); got != ReadinessIssueInvalid {
		t.Fatalf("blank prompt code = %q", got)
	}
	oversized := base
	oversized.Prompt = strings.Repeat("x", MaxPlanningReadinessPromptLength+1)
	if got := validationCode(t, oversized.Validate()); got != ReadinessIssueInvalid {
		t.Fatalf("oversized prompt code = %q", got)
	}
	blankKey := base
	blankKey.Key = ""
	if got := validationCode(t, blankKey.Validate()); got != ReadinessIssueInvalid {
		t.Fatalf("blank key code = %q", got)
	}
}

func TestPlanningReadinessKindRouteLaw(t *testing.T) {
	// A kind may only travel on its packet-defined routes.
	fact := validIssue(ReadinessFactMissing, RouteReviseContract)
	if got := validationCode(t, fact.Validate()); got != ReadinessIssueInvalid {
		t.Fatalf("fact_missing on revise_contract code = %q", got)
	}
	connector := validIssue(ReadinessConnectorMissing, RouteAnswerContext)
	if got := validationCode(t, connector.Validate()); got != ReadinessIssueInvalid {
		t.Fatalf("connector_missing on answer_context code = %q", got)
	}
	// worker_unavailable legitimately routes to harness choice or authentication.
	for _, route := range []PlanningEscalationRoute{RouteChooseHarness, RouteAuthenticateHarness} {
		issue := validIssue(ReadinessWorkerUnavailable, route)
		if err := issue.Validate(); err != nil {
			t.Fatalf("worker_unavailable on %s: %v", route, err)
		}
	}
	// effect_unresolved routes to Contract revision or owner context only.
	effect := validIssue(ReadinessEffectUnresolved, RouteRetryPlanning)
	if got := validationCode(t, effect.Validate()); got != ReadinessIssueInvalid {
		t.Fatalf("effect_unresolved on retry_planning code = %q", got)
	}
	for _, route := range []PlanningEscalationRoute{RouteReviseContract, RouteAnswerContext} {
		issue := validIssue(ReadinessEffectUnresolved, route)
		if err := issue.Validate(); err != nil {
			t.Fatalf("effect_unresolved on %s: %v", route, err)
		}
	}
}

func TestPlanningReadinessPlannerCannotClaimAuthority(t *testing.T) {
	// The planner may only declare missing facts and bounded context requests.
	for _, kind := range []PlanningReadinessIssueKind{
		ReadinessAuthorityInsufficient, ReadinessConnectorMissing, ReadinessWorkerUnavailable,
		ReadinessCheckUnrepresentable, ReadinessEffectUnresolved, ReadinessProviderUnavailable,
	} {
		issue := validIssue(kind, readinessIssueSpecs[kind].routes[0])
		issue.Source = ReadinessSourcePlannerDeclared
		if got := validationCode(t, issue.Validate()); got != ReadinessAuthorityClaim {
			t.Fatalf("planner-declared %s code = %q, want %q", kind, got, ReadinessAuthorityClaim)
		}
	}
	// A planner-declared issue carrying canonical capability, connector, or
	// routing evidence is an authority claim, not a fact request.
	issue := validIssue(ReadinessFactMissing, RouteAnswerContext)
	issue.RequestedCapability = CapabilityWorktreeWrite
	if got := validationCode(t, issue.Validate()); got != ReadinessAuthorityClaim {
		t.Fatalf("planner capability claim code = %q", got)
	}

	plain := validIssue(ReadinessFactMissing, RouteAnswerContext)
	plain.ConnectorClass = "issue_tracker"
	if got := validationCode(t, plain.Validate()); got != ReadinessAuthorityClaim {
		t.Fatalf("planner connector claim code = %q", got)
	}

	withEvidence := validIssue(ReadinessContextInsufficient, RouteAnswerContext)
	withEvidence.AdmissionReasonCodes = []AdmissionReasonCode{AdmissionProviderUnavailable}
	if got := validationCode(t, withEvidence.Validate()); got != ReadinessAuthorityClaim {
		t.Fatalf("planner admission evidence code = %q", got)
	}
}

func TestPlanningReadinessInapplicableFieldsEmpty(t *testing.T) {
	authority := validIssue(ReadinessAuthorityInsufficient, RouteReviseContract)
	authority.ConnectorClass = "issue_tracker"
	if err := authority.Validate(); err == nil {
		t.Fatal("authority issue carrying a connector class must fail")
	}
	choices := validIssue(ReadinessCheckUnrepresentable, RouteRevisePlan)
	choices.Choices = []PlanningReadinessChoice{{Key: "a", Label: "A"}}
	if err := choices.Validate(); err == nil {
		t.Fatal("check_unrepresentable carrying choices must fail")
	}
	evidence := validIssue(ReadinessFactMissing, RouteAnswerContext)
	evidence.AdmissionReasonCodes = []AdmissionReasonCode{AdmissionProviderUnavailable}
	if err := evidence.Validate(); err == nil {
		t.Fatal("fact_missing carrying admission evidence must fail")
	}
	// Required canonical fields must actually be present.
	noCapability := validIssue(ReadinessAuthorityInsufficient, RouteReviseContract)
	noCapability.RequestedCapability = ""
	if err := noCapability.Validate(); err == nil {
		t.Fatal("authority_insufficient without a requested capability must fail")
	}
	noConnector := validIssue(ReadinessConnectorMissing, RouteConfigureConnector)
	noConnector.ConnectorClass = ""
	if err := noConnector.Validate(); err == nil {
		t.Fatal("connector_missing without a connector class must fail")
	}
}

func TestPlanningReadinessChoiceContract(t *testing.T) {
	issue := validIssue(ReadinessFactMissing, RouteAnswerContext)
	issue.Choices = []PlanningReadinessChoice{{Key: "a", Label: "A"}, {Key: "a", Label: "duplicate"}}
	if got := validationCode(t, issue.Validate()); got != ReadinessIssueDuplicate {
		t.Fatalf("duplicate choice key code = %q", got)
	}
	blank := validIssue(ReadinessFactMissing, RouteAnswerContext)
	blank.Choices = []PlanningReadinessChoice{{Key: "", Label: "A"}}
	if err := blank.Validate(); err == nil {
		t.Fatal("blank choice key must fail")
	}
	ok := validIssue(ReadinessFactMissing, RouteAnswerContext)
	ok.Choices = []PlanningReadinessChoice{{Key: "a", Label: "A"}, {Key: "b", Label: "B"}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid choices: %v", err)
	}
}

func TestPlanningReadinessIssueKeyAndDigestFences(t *testing.T) {
	fence := readinessFence()
	issue := validIssue(ReadinessConnectorMissing, RouteConfigureConnector)
	issue.WorkUnitKeys = []string{"verify-release"}
	base := CanonicalPlanningIssueKey(fence, issue)
	if !strings.HasPrefix(base, "pri-") {
		t.Fatalf("issue key %q missing pri- prefix", base)
	}

	// Every canonical fence input changes the key.
	mutations := map[string]PlanningReadinessFence{
		"session":         {PlanningSessionID: "ps-2", SessionRevision: 3, ContractRevisionID: "cr-1", ContextDigest: fence.ContextDigest, RoutingSnapshotID: "rs-1"},
		"sessionRevision": {PlanningSessionID: "ps-1", SessionRevision: 4, ContractRevisionID: "cr-1", ContextDigest: fence.ContextDigest, RoutingSnapshotID: "rs-1"},
		"contract":        {PlanningSessionID: "ps-1", SessionRevision: 3, ContractRevisionID: "cr-2", ContextDigest: fence.ContextDigest, RoutingSnapshotID: "rs-1"},
		"contextDigest":   {PlanningSessionID: "ps-1", SessionRevision: 3, ContractRevisionID: "cr-1", ContextDigest: DigestSHA256([]byte("other")), RoutingSnapshotID: "rs-1"},
		"routingSnapshot": {PlanningSessionID: "ps-1", SessionRevision: 3, ContractRevisionID: "cr-1", ContextDigest: fence.ContextDigest, RoutingSnapshotID: "rs-2"},
	}
	for name, mutated := range mutations {
		if got := CanonicalPlanningIssueKey(mutated, issue); got == base {
			t.Fatalf("key unchanged under %s mutation", name)
		}
	}

	// Display text never defines identity.
	renamed := issue
	renamed.Prompt = "Completely different wording."
	renamed.Recommendation = "Do something else."
	if got := CanonicalPlanningIssueKey(fence, renamed); got != base {
		t.Fatal("display text changed the canonical issue key")
	}

	// List order is not semantic for identity.
	reordered := issue
	reordered.WorkUnitKeys = []string{"b", "a"}
	reordered.CriterionAliases = []string{"C2", "C1"}
	ordered := issue
	ordered.WorkUnitKeys = []string{"a", "b"}
	ordered.CriterionAliases = []string{"C1", "C2"}
	if CanonicalPlanningIssueKey(fence, reordered) != CanonicalPlanningIssueKey(fence, ordered) {
		t.Fatal("work-unit/criterion order changed identity")
	}

	// Length delimiting defeats concatenation ambiguity.
	a := validIssue(ReadinessFactMissing, RouteAnswerContext)
	a.WorkUnitKeys = []string{"ab", "c"}
	b := validIssue(ReadinessFactMissing, RouteAnswerContext)
	b.WorkUnitKeys = []string{"a", "bc"}
	if CanonicalPlanningIssueKey(fence, a) == CanonicalPlanningIssueKey(fence, b) {
		t.Fatal("length-delimited encoding aliased distinct unit sets")
	}
}

func TestPlanningReadinessIssueSetDigestStable(t *testing.T) {
	fence := readinessFence()
	first := validIssue(ReadinessFactMissing, RouteAnswerContext)
	second := validIssue(ReadinessConnectorMissing, RouteConfigureConnector)
	forward := PlanningReadinessIssueSetDigest(fence, []PlanningReadinessIssue{first, second})
	reverse := PlanningReadinessIssueSetDigest(fence, []PlanningReadinessIssue{second, first})
	if forward != reverse {
		t.Fatal("issue-set digest depends on input order")
	}
	if !forward.Valid() {
		t.Fatalf("issue-set digest %q is not canonical sha256 hex", forward)
	}
	mutated := fence
	mutated.RoutingSnapshotID = "rs-2"
	if PlanningReadinessIssueSetDigest(mutated, []PlanningReadinessIssue{first, second}) == forward {
		t.Fatal("issue-set digest unchanged under routing snapshot change")
	}
}

func TestCanonicalizePlanningReadinessIssues(t *testing.T) {
	fence := readinessFence()

	// Exact identity duplicates coalesce; unit keys keep the sorted union.
	one := validIssue(ReadinessFactMissing, RouteAnswerContext)
	one.Key = CanonicalPlanningIssueKey(fence, one)
	one.WorkUnitKeys = []string{"b"}
	two := one
	two.WorkUnitKeys = []string{"a"}
	out, err := CanonicalizePlanningReadinessIssues([]PlanningReadinessIssue{one, two}, nil)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("coalesced issues = %d, want 1", len(out))
	}
	if got := strings.Join(out[0].WorkUnitKeys, ","); got != "a,b" {
		t.Fatalf("coalesced unit keys = %q, want sorted union a,b", got)
	}

	// Same class and kind on a different route never merges.
	other := validIssue(ReadinessEffectUnresolved, RouteReviseContract)
	other.Key = CanonicalPlanningIssueKey(fence, other)
	otherRoute := other
	otherRoute.Route = RouteAnswerContext
	otherRoute.Key = CanonicalPlanningIssueKey(fence, otherRoute)
	out, err = CanonicalizePlanningReadinessIssues([]PlanningReadinessIssue{other, otherRoute}, nil)
	if err != nil {
		t.Fatalf("canonicalize routes: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("distinct routes coalesced: %d issues", len(out))
	}

	// Stable ordering: route rank, then kind rank, then class, then draft order.
	factIssue := validIssue(ReadinessFactMissing, RouteAnswerContext)
	factIssue.Key = CanonicalPlanningIssueKey(fence, factIssue)
	factIssue.WorkUnitKeys = []string{"z-unit"}
	contractIssue := validIssue(ReadinessAuthorityInsufficient, RouteReviseContract)
	contractIssue.Key = CanonicalPlanningIssueKey(fence, contractIssue)
	connectorIssue := validIssue(ReadinessConnectorMissing, RouteConfigureConnector)
	connectorIssue.Key = CanonicalPlanningIssueKey(fence, connectorIssue)
	out, err = CanonicalizePlanningReadinessIssues(
		[]PlanningReadinessIssue{connectorIssue, contractIssue, factIssue}, []string{"z-unit"})
	if err != nil {
		t.Fatalf("canonicalize order: %v", err)
	}
	if out[0].Route != RouteAnswerContext || out[1].Route != RouteReviseContract || out[2].Route != RouteConfigureConnector {
		t.Fatalf("order = %s,%s,%s; want route rank order", out[0].Route, out[1].Route, out[2].Route)
	}
}

func TestPlanningReadinessReasonCodes(t *testing.T) {
	want := map[PlanningReadinessIssueKind]PlanningReadinessReasonCode{
		ReadinessFactMissing:           PlanningContextFactMissing,
		ReadinessContextInsufficient:   PlanningContextPacketInsufficient,
		ReadinessAuthorityInsufficient: PlanningContractAuthority,
		ReadinessConnectorMissing:      PlanningConnectorMissing,
		ReadinessWorkerUnavailable:     PlanningWorkerUnavailable,
		ReadinessCheckUnrepresentable:  PlanningCheckUnrepresentable,
		ReadinessEffectUnresolved:      PlanningEffectUnresolved,
		ReadinessProviderUnavailable:   PlanningProviderUnavailable,
	}
	for kind, code := range want {
		if got := kind.ReasonCode(); got != code {
			t.Fatalf("%s reason code = %q, want %q", kind, got, code)
		}
	}
	for _, kind := range readinessIssueKindRank {
		if !kind.Valid() {
			t.Fatalf("declared kind %q fails Valid", kind)
		}
	}
	for _, route := range readinessRouteRank {
		if !route.Valid() {
			t.Fatalf("declared route %q fails Valid", route)
		}
	}
}

func TestPlanningReadinessPlannerFactsNeverMergeAcrossQuestions(t *testing.T) {
	fence := readinessFence()
	region := validIssue(ReadinessFactMissing, RouteAnswerContext)
	region.Prompt = "Which region?"
	region.WorkUnitKeys = []string{"a"}
	date := validIssue(ReadinessFactMissing, RouteAnswerContext)
	date.Prompt = "Which release date?"
	date.WorkUnitKeys = []string{"a"}

	out, err := CanonicalizePlanningReadinessIssues([]PlanningReadinessIssue{region, date}, nil)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("two different questions coalesced into %d issues; an owner answer would be lost", len(out))
	}

	// The same question asked for two units still coalesces, keeping the union.
	one := validIssue(ReadinessFactMissing, RouteAnswerContext)
	one.Prompt = "  Which   REGION? "
	one.WorkUnitKeys = []string{"b"}
	one.Choices = []PlanningReadinessChoice{{Key: "x", Label: "X"}}
	two := validIssue(ReadinessFactMissing, RouteAnswerContext)
	two.Prompt = "which region?"
	two.WorkUnitKeys = []string{"a"}
	two.Choices = []PlanningReadinessChoice{{Key: "x", Label: "relabelled"}}
	out, err = CanonicalizePlanningReadinessIssues([]PlanningReadinessIssue{one, two}, nil)
	if err != nil {
		t.Fatalf("canonicalize same question: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("same question with superficial rewording did not coalesce: %d issues", len(out))
	}
	if got := strings.Join(out[0].WorkUnitKeys, ","); got != "a,b" {
		t.Fatalf("coalesced unit keys = %q", got)
	}

	// A different choice contract is a different question.
	other := two
	other.Choices = []PlanningReadinessChoice{{Key: "y", Label: "X"}}
	out, err = CanonicalizePlanningReadinessIssues([]PlanningReadinessIssue{one, other}, nil)
	if err != nil {
		t.Fatalf("canonicalize choice contract: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("same prompt with different choice keys coalesced: %d issues", len(out))
	}

	// Control-plane issues still coalesce on kind|route|class|aliases alone.
	c1 := validIssue(ReadinessConnectorMissing, RouteConfigureConnector)
	c1.WorkUnitKeys = []string{"b"}
	c1.Prompt = "Connect the tracker."
	c2 := validIssue(ReadinessConnectorMissing, RouteConfigureConnector)
	c2.WorkUnitKeys = []string{"a"}
	c2.Prompt = "Different wording, same condition."
	out, err = CanonicalizePlanningReadinessIssues([]PlanningReadinessIssue{c1, c2}, nil)
	if err != nil {
		t.Fatalf("canonicalize control-plane: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("control-plane duplicates did not coalesce: %d issues", len(out))
	}
	_ = fence
}

func TestPlanningReadinessListEncodingHasElementBoundaries(t *testing.T) {
	fence := readinessFence()
	a := validIssue(ReadinessFactMissing, RouteAnswerContext)
	a.WorkUnitKeys = []string{"ab", "c"}
	b := validIssue(ReadinessFactMissing, RouteAnswerContext)
	b.WorkUnitKeys = []string{"a", "bc"}
	if CanonicalPlanningIssueKey(fence, a) == CanonicalPlanningIssueKey(fence, b) {
		t.Fatal("list encoding aliased [ab c] with [a bc]")
	}
	c := validIssue(ReadinessFactMissing, RouteAnswerContext)
	c.WorkUnitKeys = []string{"abc"}
	if CanonicalPlanningIssueKey(fence, a) == CanonicalPlanningIssueKey(fence, c) {
		t.Fatal("list encoding aliased [ab c] with [abc]")
	}
	d := validIssue(ReadinessFactMissing, RouteAnswerContext)
	d.WorkUnitKeys = []string{"a", "b", "c"}
	if CanonicalPlanningIssueKey(fence, a) == CanonicalPlanningIssueKey(fence, d) {
		t.Fatal("list encoding aliased [ab c] with [a b c]")
	}
}

func TestPlanningReadinessClosedInventories(t *testing.T) {
	// Connector classes are frozen to the normalized inventory.
	unknown := validIssue(ReadinessConnectorMissing, RouteConfigureConnector)
	unknown.ConnectorClass = "totally-new-class"
	if got := validationCode(t, unknown.Validate()); got != ReadinessIssueInvalid {
		t.Fatalf("unknown connector class code = %q", got)
	}
	known := validIssue(ReadinessConnectorMissing, RouteConfigureConnector)
	known.ConnectorClass = ConnectorClassIssueTracker
	if err := known.Validate(); err != nil {
		t.Fatalf("inventory connector class: %v", err)
	}
	// Requested capability is closed to the local capability inventory.
	badCap := validIssue(ReadinessAuthorityInsufficient, RouteReviseContract)
	badCap.RequestedCapability = "kennel.exec"
	if got := validationCode(t, badCap.Validate()); got != ReadinessIssueInvalid {
		t.Fatalf("unknown capability code = %q", got)
	}
	goodCap := validIssue(ReadinessAuthorityInsufficient, RouteReviseContract)
	goodCap.RequestedCapability = CapabilityWorktreeExec
	if err := goodCap.Validate(); err != nil {
		t.Fatalf("closed capability: %v", err)
	}
}

func TestPlanningReadinessVersionPinned(t *testing.T) {
	result := PlanningReadinessResult{
		Version: "other", Status: PlanningReady,
		Message: "Planned.", Proposal: &PlanDraftProposal{Summary: "x"},
	}
	if got := validationCode(t, result.Validate()); got != ReadinessPayloadInvalid {
		t.Fatalf("wrong version code = %q", got)
	}
	result.Version = ""
	if err := result.Validate(); err == nil {
		t.Fatal("blank version accepted")
	}
	result.Version = PlanningReadinessEnvelopeVersion
	if err := result.Validate(); err != nil {
		t.Fatalf("pinned version: %v", err)
	}
}

func TestPlanningReadinessNormalizationConstructor(t *testing.T) {
	proposal := &PlanDraftProposal{Summary: "do it"}
	if got := NewPlanningReadinessResult("ok", proposal, nil); got.Status != PlanningReady {
		t.Fatalf("proposal without issues = %q, want ready", got.Status)
	}
	answerable := validIssue(ReadinessFactMissing, RouteAnswerContext)
	answerable.Key = "pri-answer"
	got := NewPlanningReadinessResult("answer needed", proposal, []PlanningReadinessIssue{answerable})
	if got.Status != PlanningNeedsContext {
		t.Fatalf("answerable issues = %q, want needs_context", got.Status)
	}
	if got.Proposal != nil {
		t.Fatal("needs_context packet still carries a proposal")
	}
	operational := validIssue(ReadinessConnectorMissing, RouteConfigureConnector)
	operational.Key = "pri-setup"
	got = NewPlanningReadinessResult("setup needed", proposal, []PlanningReadinessIssue{answerable, operational})
	if got.Status != PlanningBlocked {
		t.Fatalf("mixed packet = %q, want blocked", got.Status)
	}
	if len(got.Issues) != 2 {
		t.Fatalf("blocked packet retained %d issues, want both", len(got.Issues))
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("normalized blocked packet is valid: %v", err)
	}
	if got.Version != PlanningReadinessEnvelopeVersion {
		t.Fatalf("constructor version = %q", got.Version)
	}
}

func TestPlanningReadinessIdentifierFencing(t *testing.T) {
	for _, bad := range []string{"", "with space", "with,comma", "café", "tab\there", "newline\nhere", string(rune(0x7f)) + "del"} {
		if err := ValidateReadinessIdentifier(bad); err == nil {
			t.Fatalf("identifier %q accepted", bad)
		}
	}
	for _, good := range []string{"verify-release", "C1", "unit_2", "a.b/c"} {
		if err := ValidateReadinessIdentifier(good); err != nil {
			t.Fatalf("identifier %q rejected: %v", good, err)
		}
	}
	dup := validIssue(ReadinessFactMissing, RouteAnswerContext)
	dup.WorkUnitKeys = []string{"a", "a"}
	if err := dup.Validate(); err == nil {
		t.Fatal("duplicate work unit key accepted")
	}
	ctrl := validIssue(ReadinessFactMissing, RouteAnswerContext)
	ctrl.CriterionAliases = []string{"C1\t"}
	if err := ctrl.Validate(); err == nil {
		t.Fatal("control character in criterion alias accepted")
	}
}
