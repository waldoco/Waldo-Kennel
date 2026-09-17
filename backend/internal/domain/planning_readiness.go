package domain

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// PlanningReadinessEnvelopeVersion identifies the strict provider envelope this
// package validates. The evaluator stamps it on every result so a receipt can
// be read against the exact contract that produced it.
const PlanningReadinessEnvelopeVersion = "planning-readiness/v1"

// Bounds on untrusted planning prose. These are operational fences on provider
// output, not laws of planning.
const (
	MaxPlanningReadinessIssues          = 32
	MaxPlanningReadinessChoices         = 8
	MaxPlanningReadinessPromptLength    = 1000
	MaxPlanningReadinessReasonLength    = 1000
	MaxPlanningReadinessChoiceKeyLength = 100
	MaxPlanningReadinessChoiceLabelLen  = 200
)

// PlanningReadinessStatus is the closed planning-readiness verdict.
type PlanningReadinessStatus string

// Supported readiness verdicts.
const (
	PlanningReady        PlanningReadinessStatus = "ready"
	PlanningNeedsContext PlanningReadinessStatus = "needs_context"
	PlanningBlocked      PlanningReadinessStatus = "blocked"
)

// Valid reports whether the readiness status is supported.
func (s PlanningReadinessStatus) Valid() bool {
	switch s {
	case PlanningReady, PlanningNeedsContext, PlanningBlocked:
		return true
	default:
		return false
	}
}

// PlanningReadinessIssueKind is the closed taxonomy of what is missing.
type PlanningReadinessIssueKind string

// Supported readiness issue kinds.
const (
	ReadinessFactMissing           PlanningReadinessIssueKind = "fact_missing"
	ReadinessContextInsufficient   PlanningReadinessIssueKind = "context_insufficient"
	ReadinessAuthorityInsufficient PlanningReadinessIssueKind = "authority_insufficient"
	ReadinessConnectorMissing      PlanningReadinessIssueKind = "connector_missing"
	ReadinessWorkerUnavailable     PlanningReadinessIssueKind = "worker_unavailable"
	ReadinessCheckUnrepresentable  PlanningReadinessIssueKind = "check_unrepresentable"
	ReadinessEffectUnresolved      PlanningReadinessIssueKind = "effect_unresolved"
	ReadinessProviderUnavailable   PlanningReadinessIssueKind = "provider_unavailable"
)

// readinessIssueKindRank freezes the declaration order used for stable sorting.
var readinessIssueKindRank = []PlanningReadinessIssueKind{
	ReadinessFactMissing,
	ReadinessContextInsufficient,
	ReadinessAuthorityInsufficient,
	ReadinessConnectorMissing,
	ReadinessWorkerUnavailable,
	ReadinessCheckUnrepresentable,
	ReadinessEffectUnresolved,
	ReadinessProviderUnavailable,
}

// Valid reports whether the issue kind is supported.
func (k PlanningReadinessIssueKind) Valid() bool {
	for _, known := range readinessIssueKindRank {
		if k == known {
			return true
		}
	}
	return false
}

// PlanningEscalationRoute is the closed set of surfaces an issue routes to.
// The Mission Supervisor routes strictly from these codes; it never parses
// prose to decide where an issue goes.
type PlanningEscalationRoute string

// Supported escalation routes.
const (
	RouteAnswerContext       PlanningEscalationRoute = "answer_context"
	RouteReviseContract      PlanningEscalationRoute = "revise_contract"
	RouteConfigureConnector  PlanningEscalationRoute = "configure_connector"
	RouteChooseHarness       PlanningEscalationRoute = "choose_harness"
	RouteAuthenticateHarness PlanningEscalationRoute = "authenticate_harness"
	RouteRevisePlan          PlanningEscalationRoute = "revise_plan"
	RouteRetryPlanning       PlanningEscalationRoute = "retry_planning"
)

// readinessRouteRank freezes the declaration order used for stable sorting.
var readinessRouteRank = []PlanningEscalationRoute{
	RouteAnswerContext,
	RouteReviseContract,
	RouteConfigureConnector,
	RouteChooseHarness,
	RouteAuthenticateHarness,
	RouteRevisePlan,
	RouteRetryPlanning,
}

// Valid reports whether the escalation route is supported.
func (r PlanningEscalationRoute) Valid() bool {
	for _, known := range readinessRouteRank {
		if r == known {
			return true
		}
	}
	return false
}

// PlanningReadinessIssueSource records who identified the issue. The planner
// may declare missing facts and bounded context requests; every authority,
// routing, connector, check, effect, or provider condition is derived by the
// Kennel control plane from the canonical Contract and normalized inventory.
type PlanningReadinessIssueSource string

// Supported issue origins.
const (
	ReadinessSourcePlannerDeclared PlanningReadinessIssueSource = "planner_declared"
	ReadinessSourceControlPlane    PlanningReadinessIssueSource = "control_plane"
)

// Valid reports whether the issue source is supported.
func (s PlanningReadinessIssueSource) Valid() bool {
	return s == ReadinessSourcePlannerDeclared || s == ReadinessSourceControlPlane
}

// PlanningReadinessReasonCode is the stable presentation code attached to a
// normalized issue. Detailed admission and routing reasons stay attached as
// evidence; these codes never replace them.
type PlanningReadinessReasonCode string

// Stable reason codes for normalized readiness issues.
const (
	PlanningContextFactMissing        PlanningReadinessReasonCode = "PLANNING_CONTEXT_FACT_MISSING"
	PlanningContextPacketInsufficient PlanningReadinessReasonCode = "PLANNING_CONTEXT_PACKET_INSUFFICIENT"
	PlanningContractAuthority         PlanningReadinessReasonCode = "PLANNING_CONTRACT_AUTHORITY_INSUFFICIENT"
	PlanningConnectorMissing          PlanningReadinessReasonCode = "PLANNING_CONNECTOR_MISSING"
	PlanningWorkerUnavailable         PlanningReadinessReasonCode = "PLANNING_WORKER_UNAVAILABLE"
	PlanningCheckUnrepresentable      PlanningReadinessReasonCode = "PLANNING_CHECK_UNREPRESENTABLE"
	PlanningEffectUnresolved          PlanningReadinessReasonCode = "PLANNING_EFFECT_UNRESOLVED"
	PlanningProviderUnavailable       PlanningReadinessReasonCode = "PLANNING_PROVIDER_UNAVAILABLE"
)

// ReasonCode maps an issue kind to its stable presentation code.
func (k PlanningReadinessIssueKind) ReasonCode() PlanningReadinessReasonCode {
	switch k {
	case ReadinessFactMissing:
		return PlanningContextFactMissing
	case ReadinessContextInsufficient:
		return PlanningContextPacketInsufficient
	case ReadinessAuthorityInsufficient:
		return PlanningContractAuthority
	case ReadinessConnectorMissing:
		return PlanningConnectorMissing
	case ReadinessWorkerUnavailable:
		return PlanningWorkerUnavailable
	case ReadinessCheckUnrepresentable:
		return PlanningCheckUnrepresentable
	case ReadinessEffectUnresolved:
		return PlanningEffectUnresolved
	case ReadinessProviderUnavailable:
		return PlanningProviderUnavailable
	default:
		return ""
	}
}

// PlanningReadinessValidationCode is a stable provider/envelope failure code.
type PlanningReadinessValidationCode string

// Envelope validation failure codes.
const (
	ReadinessStatusInvalid  PlanningReadinessValidationCode = "PLANNING_READINESS_STATUS_INVALID"
	ReadinessPayloadInvalid PlanningReadinessValidationCode = "PLANNING_READINESS_PAYLOAD_INVALID"
	ReadinessIssueInvalid   PlanningReadinessValidationCode = "PLANNING_READINESS_ISSUE_INVALID"
	ReadinessIssueDuplicate PlanningReadinessValidationCode = "PLANNING_READINESS_ISSUE_DUPLICATE"
	ReadinessAuthorityClaim PlanningReadinessValidationCode = "PLANNING_READINESS_AUTHORITY_CLAIM_INVALID"
)

// PlanningReadinessValidationError is a coded, display-safe rejection of a
// readiness envelope or issue.
type PlanningReadinessValidationError struct {
	Code    PlanningReadinessValidationCode
	Message string
}

func (e *PlanningReadinessValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func readinessValidation(code PlanningReadinessValidationCode, format string, args ...any) error {
	return &PlanningReadinessValidationError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// readinessIssueSpec is the closed per-kind law: which routes are legal, which
// source may raise it, and which optional fields apply. Fields not listed as
// applicable for a kind must be empty.
type readinessIssueSpec struct {
	routes            []PlanningEscalationRoute
	allowPlanner      bool
	choices           bool // Choices apply
	admissionEvidence bool // AdmissionReasonCodes apply
}

var readinessIssueSpecs = map[PlanningReadinessIssueKind]readinessIssueSpec{
	ReadinessFactMissing: {
		routes: []PlanningEscalationRoute{RouteAnswerContext}, allowPlanner: true, choices: true,
	},
	ReadinessContextInsufficient: {
		routes: []PlanningEscalationRoute{RouteAnswerContext}, allowPlanner: true, choices: true,
	},
	ReadinessAuthorityInsufficient: {
		routes: []PlanningEscalationRoute{RouteReviseContract},
	},
	ReadinessConnectorMissing: {
		routes: []PlanningEscalationRoute{RouteConfigureConnector},
	},
	ReadinessWorkerUnavailable: {
		routes:            []PlanningEscalationRoute{RouteChooseHarness, RouteAuthenticateHarness},
		admissionEvidence: true,
	},
	ReadinessCheckUnrepresentable: {
		routes: []PlanningEscalationRoute{RouteRevisePlan},
	},
	ReadinessEffectUnresolved: {
		routes: []PlanningEscalationRoute{RouteReviseContract, RouteAnswerContext}, choices: true,
	},
	ReadinessProviderUnavailable: {
		routes:            []PlanningEscalationRoute{RouteRetryPlanning, RouteAuthenticateHarness},
		admissionEvidence: true,
	},
}

// ConnectorClass is the frozen vocabulary of connector classes the normalized
// routing inventory can represent. The inventory is closed on purpose: an
// arbitrary provider string is never a connector class, and a class enters
// this registry only when the normalized inventory actually models it.
type ConnectorClass string

// Connector classes represented by the normalized inventory at launch.
const (
	ConnectorClassIssueTracker ConnectorClass = "issue_tracker"
)

// connectorClassRegistry is the exact membership of the frozen inventory.
var connectorClassRegistry = []ConnectorClass{
	ConnectorClassIssueTracker,
}

// Valid reports whether the connector class is in the frozen inventory.
func (c ConnectorClass) Valid() bool {
	for _, known := range connectorClassRegistry {
		if c == known {
			return true
		}
	}
	return false
}

// closedLocalCapabilities is the exact local capability inventory a readiness
// issue may name. It mirrors the capability constants the Contract ceiling is
// expressed in; anything else is not a capability Kennel can grant.
var closedLocalCapabilities = []string{
	CapabilityWorktreeRead,
	CapabilityWorktreeWrite,
	CapabilityWorktreeExec,
}

// ValidClosedCapability reports whether the capability is a member of the
// closed local capability inventory.
func ValidClosedCapability(capability string) bool {
	for _, known := range closedLocalCapabilities {
		if capability == known {
			return true
		}
	}
	return false
}

// MaxPlanningReadinessIdentifierLength bounds machine identifiers carried by
// readiness issues.
const MaxPlanningReadinessIdentifierLength = 100

// ValidateReadinessIdentifier enforces the identifier contract for WorkUnit
// keys and criterion aliases carried on issues: printable ASCII only, so no
// control character, combining mark, or Unicode normalization form can split
// one logical identifier into two canonical identities.
func ValidateReadinessIdentifier(value string) error {
	if value == "" {
		return fmt.Errorf("identifier is blank")
	}
	if len(value) > MaxPlanningReadinessIdentifierLength {
		return fmt.Errorf("identifier exceeds %d bytes", MaxPlanningReadinessIdentifierLength)
	}
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' || r == '/'
		if !ok {
			return fmt.Errorf("identifier %q contains character %q outside [A-Za-z0-9._/-]", value, r)
		}
	}
	return nil
}

// PlanningReadinessChoice is one owner-selectable answer. Keys are stable for
// selection; labels are display-only and never define identity.
type PlanningReadinessChoice struct {
	Key   string
	Label string
}

// PlanningReadinessIssue is one typed, bounded escalation. Free text is
// display-only: Prompt, Reason, Recommendation, and choice labels are never
// used for routing, identity, or authority.
type PlanningReadinessIssue struct {
	Key                  string
	Kind                 PlanningReadinessIssueKind
	Route                PlanningEscalationRoute
	Source               PlanningReadinessIssueSource
	Prompt               string
	Reason               string
	Recommendation       string
	Choices              []PlanningReadinessChoice
	WorkUnitKeys         []string
	CriterionAliases     []string
	RequestedCapability  string
	ConnectorClass       ConnectorClass
	AdmissionReasonCodes []AdmissionReasonCode
}

// normalizedRequestedClass is the canonical capability/connector class used
// for identity and ordering. Case and surrounding whitespace never define
// identity.
func (issue PlanningReadinessIssue) normalizedRequestedClass() string {
	if issue.RequestedCapability != "" {
		return strings.ToLower(strings.TrimSpace(issue.RequestedCapability))
	}
	return strings.ToLower(strings.TrimSpace(string(issue.ConnectorClass)))
}

// Validate enforces the closed issue contract: known taxonomy, legal route for
// the kind, legal source for the kind, non-blank display fields, and no
// inapplicable fields. A violation is invalid provider or evaluator output,
// never a silently-coerced issue.
func (issue PlanningReadinessIssue) Validate() error {
	if strings.TrimSpace(issue.Key) == "" {
		return readinessValidation(ReadinessIssueInvalid, "readiness issue key is blank")
	}
	if !issue.Kind.Valid() {
		return readinessValidation(ReadinessIssueInvalid, "unknown readiness issue kind %q", issue.Kind)
	}
	if !issue.Route.Valid() {
		return readinessValidation(ReadinessIssueInvalid, "unknown readiness route %q", issue.Route)
	}
	if !issue.Source.Valid() {
		return readinessValidation(ReadinessIssueInvalid, "unknown readiness issue source %q", issue.Source)
	}
	spec := readinessIssueSpecs[issue.Kind]
	if issue.Source == ReadinessSourcePlannerDeclared && !spec.allowPlanner {
		return readinessValidation(ReadinessAuthorityClaim,
			"planner may not declare readiness kind %q; it is control-plane derived", issue.Kind)
	}
	routeOK := false
	for _, route := range spec.routes {
		if issue.Route == route {
			routeOK = true
			break
		}
	}
	if !routeOK {
		return readinessValidation(ReadinessIssueInvalid,
			"route %q is not a legal route for readiness kind %q", issue.Route, issue.Kind)
	}
	// Planner-declared issues never carry canonical capability, connector, or
	// routing evidence: those values are Kennel-derived by construction, so a
	// provider supplying them is making an authority claim.
	if issue.Source == ReadinessSourcePlannerDeclared &&
		(issue.RequestedCapability != "" || issue.ConnectorClass != "" || len(issue.AdmissionReasonCodes) > 0) {
		return readinessValidation(ReadinessAuthorityClaim,
			"planner-declared issue %q carries control-plane fields", issue.Key)
	}
	if strings.TrimSpace(issue.Prompt) == "" {
		return readinessValidation(ReadinessIssueInvalid, "readiness issue %q prompt is blank", issue.Key)
	}
	if len(issue.Prompt) > MaxPlanningReadinessPromptLength {
		return readinessValidation(ReadinessIssueInvalid, "readiness issue %q prompt exceeds %d bytes", issue.Key, MaxPlanningReadinessPromptLength)
	}
	if strings.TrimSpace(issue.Reason) == "" {
		return readinessValidation(ReadinessIssueInvalid, "readiness issue %q reason is blank", issue.Key)
	}
	if len(issue.Reason) > MaxPlanningReadinessReasonLength {
		return readinessValidation(ReadinessIssueInvalid, "readiness issue %q reason exceeds %d bytes", issue.Key, MaxPlanningReadinessReasonLength)
	}
	if issue.Kind == ReadinessConnectorMissing {
		if !issue.ConnectorClass.Valid() {
			return readinessValidation(ReadinessIssueInvalid,
				"connector_missing issue %q names connector class %q outside the frozen inventory", issue.Key, issue.ConnectorClass)
		}
	} else if issue.ConnectorClass != "" {
		return readinessValidation(ReadinessIssueInvalid,
			"readiness kind %q does not carry a connector class", issue.Kind)
	}
	if issue.Kind == ReadinessAuthorityInsufficient {
		if !ValidClosedCapability(issue.RequestedCapability) {
			return readinessValidation(ReadinessIssueInvalid,
				"authority_insufficient issue %q names capability %q outside the closed inventory", issue.Key, issue.RequestedCapability)
		}
	} else if issue.RequestedCapability != "" {
		return readinessValidation(ReadinessIssueInvalid,
			"readiness kind %q does not carry a requested capability", issue.Kind)
	}
	if err := validateReadinessIdentifierList("work unit key", issue.WorkUnitKeys); err != nil {
		return readinessValidation(ReadinessIssueInvalid, "readiness issue %q: %v", issue.Key, err)
	}
	if err := validateReadinessIdentifierList("criterion alias", issue.CriterionAliases); err != nil {
		return readinessValidation(ReadinessIssueInvalid, "readiness issue %q: %v", issue.Key, err)
	}
	if len(issue.Choices) > 0 && !spec.choices {
		return readinessValidation(ReadinessIssueInvalid,
			"readiness kind %q does not carry answer choices", issue.Kind)
	}
	if len(issue.Choices) > MaxPlanningReadinessChoices {
		return readinessValidation(ReadinessIssueInvalid,
			"readiness issue %q exceeds %d choices", issue.Key, MaxPlanningReadinessChoices)
	}
	seenChoiceKeys := map[string]bool{}
	for _, choice := range issue.Choices {
		if strings.TrimSpace(choice.Key) == "" || len(choice.Key) > MaxPlanningReadinessChoiceKeyLength {
			return readinessValidation(ReadinessIssueInvalid,
				"readiness issue %q carries a blank or oversized choice key", issue.Key)
		}
		if seenChoiceKeys[choice.Key] {
			return readinessValidation(ReadinessIssueDuplicate,
				"readiness issue %q repeats choice key %q", issue.Key, choice.Key)
		}
		seenChoiceKeys[choice.Key] = true
		if strings.TrimSpace(choice.Label) == "" || len(choice.Label) > MaxPlanningReadinessChoiceLabelLen {
			return readinessValidation(ReadinessIssueInvalid,
				"readiness issue %q carries a blank or oversized choice label", issue.Key)
		}
	}
	if len(issue.AdmissionReasonCodes) > 0 && !spec.admissionEvidence {
		return readinessValidation(ReadinessIssueInvalid,
			"readiness kind %q does not carry admission reason evidence", issue.Kind)
	}
	return nil
}

// PlanningReadinessResult is the exhaustive planning envelope. Exactly one
// shape is legal per status, and a non-ready result never carries a proposal.
type PlanningReadinessResult struct {
	Version  string
	Status   PlanningReadinessStatus
	Message  string // display only
	Proposal *PlanDraftProposal
	Issues   []PlanningReadinessIssue
}

// Validate enforces the closed-shape envelope rules.
func (r PlanningReadinessResult) Validate() error {
	if r.Version != PlanningReadinessEnvelopeVersion {
		return readinessValidation(ReadinessPayloadInvalid,
			"readiness envelope version %q is not %q", r.Version, PlanningReadinessEnvelopeVersion)
	}
	if !r.Status.Valid() {
		return readinessValidation(ReadinessStatusInvalid, "unknown readiness status %q", r.Status)
	}
	if strings.TrimSpace(r.Message) == "" {
		return readinessValidation(ReadinessPayloadInvalid, "readiness %q result carries a blank message", r.Status)
	}
	switch r.Status {
	case PlanningReady:
		if r.Proposal == nil {
			return readinessValidation(ReadinessPayloadInvalid, "ready result carries no proposal")
		}
		if len(r.Issues) != 0 {
			return readinessValidation(ReadinessPayloadInvalid, "ready result carries %d issues", len(r.Issues))
		}
	case PlanningNeedsContext:
		if r.Proposal != nil {
			return readinessValidation(ReadinessPayloadInvalid, "needs_context result carries a proposal")
		}
		if len(r.Issues) == 0 {
			return readinessValidation(ReadinessPayloadInvalid, "needs_context result carries no issues")
		}
		// needs_context means every issue is owner-answerable; one operational
		// issue normalizes the whole packet to blocked.
		for _, issue := range r.Issues {
			if issue.Route != RouteAnswerContext {
				return readinessValidation(ReadinessPayloadInvalid,
					"needs_context result carries non-answerable route %q; mixed packets are blocked", issue.Route)
			}
		}
	case PlanningBlocked:
		if r.Proposal != nil {
			return readinessValidation(ReadinessPayloadInvalid, "blocked result carries a proposal")
		}
		if len(r.Issues) == 0 {
			return readinessValidation(ReadinessPayloadInvalid, "blocked result carries no issues")
		}
		operational := false
		for _, issue := range r.Issues {
			if issue.Route != RouteAnswerContext {
				operational = true
				break
			}
		}
		if !operational {
			return readinessValidation(ReadinessPayloadInvalid,
				"blocked result carries only owner-answerable issues; that packet is needs_context")
		}
	}
	if len(r.Issues) > MaxPlanningReadinessIssues {
		return readinessValidation(ReadinessPayloadInvalid, "readiness result exceeds %d issues", MaxPlanningReadinessIssues)
	}
	seen := map[string]bool{}
	for _, issue := range r.Issues {
		if err := issue.Validate(); err != nil {
			return err
		}
		if seen[issue.Key] {
			return readinessValidation(ReadinessIssueDuplicate, "readiness issue key %q is duplicated", issue.Key)
		}
		seen[issue.Key] = true
	}
	return nil
}

// NormalizePlanningReadinessStatus is the single canonical status derivation.
// A packet is ready only when a proposal exists and zero issues remain. Any
// operational issue normalizes the whole packet to blocked while retaining
// every owner-answerable issue in the same generation. Callers never label a
// packet themselves; they assemble proposal and issues and normalize.
func NormalizePlanningReadinessStatus(proposal *PlanDraftProposal, issues []PlanningReadinessIssue) PlanningReadinessStatus {
	if proposal != nil && len(issues) == 0 {
		return PlanningReady
	}
	for _, issue := range issues {
		if issue.Route != RouteAnswerContext {
			return PlanningBlocked
		}
	}
	return PlanningNeedsContext
}

// NewPlanningReadinessResult assembles one normalized envelope. The status is
// always derived, never accepted from a caller: a proposal carrying issues is
// not ready, and a non-ready packet carries no proposal, matching the rule
// that no PlanRevision is ever written for needs_context or blocked.
func NewPlanningReadinessResult(
	message string,
	proposal *PlanDraftProposal,
	issues []PlanningReadinessIssue,
) PlanningReadinessResult {
	status := NormalizePlanningReadinessStatus(proposal, issues)
	if status != PlanningReady {
		proposal = nil
	}
	return PlanningReadinessResult{
		Version:  PlanningReadinessEnvelopeVersion,
		Status:   status,
		Message:  message,
		Proposal: proposal,
		Issues:   issues,
	}
}

// PlanningReadinessFence binds one packet to the exact frozen inputs of its
// evaluation. Any change to the fence is a new packet generation.
type PlanningReadinessFence struct {
	PlanningSessionID  PlanningSessionID
	SessionRevision    int64
	ContractRevisionID ContractRevisionID
	ContextDigest      SHA256Digest
	RoutingSnapshotID  string
}

// writeLengthDelimited appends one field in length-delimited form so no
// concatenation of prose can ever alias two distinct canonical inputs.
func writeLengthDelimited(builder *strings.Builder, field string) {
	builder.WriteString(strconv.Itoa(len(field)))
	builder.WriteByte(':')
	builder.WriteString(field)
}

// writeLengthDelimitedList appends a list as its element count followed by
// each element length-delimited individually. Comma-joining before delimiting
// would let ["a,b","c"] and ["a","b,c"] alias; per-element delimiting with a
// count cannot.
func writeLengthDelimitedList(builder *strings.Builder, values []string) {
	writeLengthDelimited(builder, strconv.Itoa(len(values)))
	for _, value := range values {
		writeLengthDelimited(builder, value)
	}
}

// validateReadinessIdentifierList fences one issue's machine references: every
// entry is a well-formed identifier, unique, and bounded in count.
func validateReadinessIdentifierList(label string, values []string) error {
	if len(values) > MaxPlanDraftWorkUnits*2 {
		return fmt.Errorf("%s list exceeds %d entries", label, MaxPlanDraftWorkUnits*2)
	}
	seen := map[string]bool{}
	for _, value := range values {
		if err := ValidateReadinessIdentifier(value); err != nil {
			return fmt.Errorf("%s %q is not a valid identifier: %v", label, value, err)
		}
		if seen[value] {
			return fmt.Errorf("%s %q is duplicated", label, value)
		}
		seen[value] = true
	}
	return nil
}

func sortedCopy(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

// writeIssueSemanticIdentity appends the semantic identity of one issue: the
// inputs both coalescing and key derivation share. Kind, route, normalized
// requested class, and criterion aliases identify a control-plane condition.
// A planner-declared issue carries its meaning in the question itself, so its
// normalized answer contract (prompt and choice keys) is identity too —
// otherwise two distinct questions would mint one key and collide downstream.
// Display text beyond the answer contract (recommendation, choice labels)
// never defines identity.
func writeIssueSemanticIdentity(builder *strings.Builder, issue PlanningReadinessIssue) {
	writeLengthDelimited(builder, string(issue.Kind))
	writeLengthDelimited(builder, string(issue.Route))
	writeLengthDelimited(builder, issue.normalizedRequestedClass())
	writeLengthDelimitedList(builder, sortedCopy(issue.CriterionAliases))
	if issue.Source == ReadinessSourcePlannerDeclared {
		writeLengthDelimited(builder, normalizePlannerAnswerText(issue.Prompt))
		choiceKeys := make([]string, 0, len(issue.Choices))
		for _, choice := range issue.Choices {
			choiceKeys = append(choiceKeys, choice.Key)
		}
		writeLengthDelimitedList(builder, sortedCopy(choiceKeys))
	}
}

// writeCanonicalIssueIdentity appends the full canonical identity of one issue
// within a packet: the semantic identity plus the affected WorkUnit keys.
func writeCanonicalIssueIdentity(builder *strings.Builder, issue PlanningReadinessIssue) {
	writeIssueSemanticIdentity(builder, issue)
	writeLengthDelimitedList(builder, sortedCopy(issue.WorkUnitKeys))
}

// issueCoalesceDigest hashes the coalescing identity of one issue: exactly the
// shared semantic identity, with WorkUnit keys deliberately excluded.
// Duplicate issues raised from several units coalesce into one issue whose
// affected unit keys are the sorted union; two issues with distinct semantic
// identities never merge, because merging them would silently discard content.
func issueCoalesceDigest(issue PlanningReadinessIssue) SHA256Digest {
	var builder strings.Builder
	writeIssueSemanticIdentity(&builder, issue)
	return DigestSHA256([]byte(builder.String()))
}

// normalizePlannerAnswerText folds case and whitespace so superficially
// reworded duplicates of the same question still coalesce.
func normalizePlannerAnswerText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}

// CanonicalPlanningIssueKey derives the stable issue key from the packet fence
// and the issue's identity inputs. Replaying the same evaluation returns the
// same keys; any fence or identity change mints new ones.
func CanonicalPlanningIssueKey(fence PlanningReadinessFence, issue PlanningReadinessIssue) string {
	var builder strings.Builder
	writeLengthDelimited(&builder, fence.PlanningSessionID.String())
	writeLengthDelimited(&builder, strconv.FormatInt(fence.SessionRevision, 10))
	writeLengthDelimited(&builder, fence.ContractRevisionID.String())
	writeLengthDelimited(&builder, fence.ContextDigest.String())
	writeLengthDelimited(&builder, fence.RoutingSnapshotID)
	writeCanonicalIssueIdentity(&builder, issue)
	return "pri-" + DigestSHA256([]byte(builder.String())).String()
}

// PlanningReadinessIssueSetDigest fingerprints the packet's canonical issue
// set under its fence. It is stable under input ordering and changes whenever
// the canonical set or the fence changes.
func PlanningReadinessIssueSetDigest(fence PlanningReadinessFence, issues []PlanningReadinessIssue) SHA256Digest {
	keys := make([]string, 0, len(issues))
	for _, issue := range issues {
		keys = append(keys, CanonicalPlanningIssueKey(fence, issue))
	}
	sort.Strings(keys)
	var builder strings.Builder
	for _, key := range keys {
		writeLengthDelimited(&builder, key)
	}
	return DigestSHA256([]byte(builder.String()))
}

func rankOf[T comparable](order []T, value T) int {
	for i, known := range order {
		if known == value {
			return i
		}
	}
	return len(order)
}

// CanonicalizePlanningReadinessIssues deduplicates and stably orders one
// packet's issues. Exact identity duplicates coalesce; coalesced issues keep
// the sorted union of their WorkUnit keys and criterion aliases. Ordering is
// route rank, kind rank, normalized requested class, frozen draft WorkUnit
// order, criterion alias, then key. workUnitOrder is the frozen draft order;
// units absent from it sort after listed units by key.
func CanonicalizePlanningReadinessIssues(issues []PlanningReadinessIssue, workUnitOrder []string) ([]PlanningReadinessIssue, error) {
	unitRank := make(map[string]int, len(workUnitOrder))
	for i, key := range workUnitOrder {
		unitRank[key] = i
	}
	byIdentity := map[SHA256Digest]int{}
	out := make([]PlanningReadinessIssue, 0, len(issues))
	for _, issue := range issues {
		identity := issueCoalesceDigest(issue)
		if at, ok := byIdentity[identity]; ok {
			merged := out[at]
			merged.WorkUnitKeys = sortedUnion(merged.WorkUnitKeys, issue.WorkUnitKeys)
			merged.CriterionAliases = sortedUnion(merged.CriterionAliases, issue.CriterionAliases)
			out[at] = merged
			continue
		}
		byIdentity[identity] = len(out)
		out = append(out, issue)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if ra, rb := rankOf(readinessRouteRank, a.Route), rankOf(readinessRouteRank, b.Route); ra != rb {
			return ra < rb
		}
		if ka, kb := rankOf(readinessIssueKindRank, a.Kind), rankOf(readinessIssueKindRank, b.Kind); ka != kb {
			return ka < kb
		}
		if ca, cb := a.normalizedRequestedClass(), b.normalizedRequestedClass(); ca != cb {
			return ca < cb
		}
		if ua, ub := firstUnitRank(unitRank, a.WorkUnitKeys), firstUnitRank(unitRank, b.WorkUnitKeys); ua != ub {
			return ua < ub
		}
		if aa, ab := strings.Join(sortedCopy(a.CriterionAliases), ","), strings.Join(sortedCopy(b.CriterionAliases), ","); aa != ab {
			return aa < ab
		}
		return a.Key < b.Key
	})
	return out, nil
}

func sortedUnion(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range append(append([]string(nil), a...), b...) {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

// firstUnitRank is the earliest frozen draft position among an issue's units;
// issues touching earlier draft units surface first.
func firstUnitRank(unitRank map[string]int, keys []string) int {
	best := len(unitRank) + len(keys) + 1
	for _, key := range sortedCopy(keys) {
		if rank, ok := unitRank[key]; ok && rank < best {
			best = rank
		}
	}
	return best
}
