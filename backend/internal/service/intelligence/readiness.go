package intelligence

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// planningReadinessSchema is the strict S3 provider envelope. The planner may
// only say ready-with-proposal, or declare answerable fact/context issues. It
// cannot name routes, capabilities, connectors, admission evidence, or owner
// answers: those properties do not exist in the schema, and the parser rejects
// them if the provider emits them anyway.
func planningReadinessSchema(aliases []string) map[string]any {
	criterionAlias := map[string]any{"type": "string", "enum": aliases}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"status", "message", "proposal", "issues"},
		"properties": map[string]any{
			"status": map[string]any{"type": "string", "enum": []any{
				string(domain.PlanningReady), string(domain.PlanningNeedsContext), string(domain.PlanningBlocked),
			}},
			"message":  map[string]any{"type": "string", "description": "concise owner-facing explanation"},
			"proposal": nullableObject(planSchema(aliases)),
			"issues": map[string]any{
				"type":        "array",
				"description": "every material missing fact or bounded context request, batched in one packet",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []any{"kind", "prompt", "reason", "recommendation", "choices", "workUnitKeys", "criterionAliases"},
					"properties": map[string]any{
						"kind": map[string]any{"type": "string", "enum": []any{
							string(domain.ReadinessFactMissing), string(domain.ReadinessContextInsufficient),
						}},
						"prompt":         map[string]any{"type": "string"},
						"reason":         map[string]any{"type": "string", "description": "why the fact changes the graph, proof, or safe execution"},
						"recommendation": map[string]any{"type": "string"},
						"choices": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type":                 "object",
								"additionalProperties": false,
								"required":             []any{"key", "label"},
								"properties": map[string]any{
									"key":   map[string]any{"type": "string"},
									"label": map[string]any{"type": "string"},
								},
							},
						},
						"workUnitKeys":     stringArray("draft WorkUnit keys this issue affects"),
						"criterionAliases": map[string]any{"type": "array", "items": criterionAlias},
					},
				},
			},
		},
	}
}

// readinessIssueReply is the planner-declared issue wire shape. It has no
// route, source, capability, connector, or evidence fields by design.
type readinessIssueReply struct {
	Kind           string `json:"kind"`
	Prompt         string `json:"prompt"`
	Reason         string `json:"reason"`
	Recommendation string `json:"recommendation"`
	Choices        []struct {
		Key   string `json:"key"`
		Label string `json:"label"`
	} `json:"choices"`
	WorkUnitKeys     []string `json:"workUnitKeys"`
	CriterionAliases []string `json:"criterionAliases"`
}

type planningReadinessReply struct {
	Status   string                `json:"status"`
	Message  string                `json:"message"`
	Proposal *planReply            `json:"proposal"`
	Issues   []readinessIssueReply `json:"issues"`
}

// authorityClaimFields are wire properties that would let the provider select
// routes, authority, connectors, evidence, or owner answers. Their presence is
// an authority-claim violation, not a generic shape error.
var authorityClaimFields = []string{
	"route", "source", "requestedCapability", "capability", "capabilities",
	"connectorClass", "connector", "admissionReasonCodes", "admissionReasons",
	"grant", "grants", "path", "command", "url", "credential", "credentials",
	"token", "bearer", "secret", "ownerAnswer", "answer",
}

// parsePlanningReadinessReply decodes one strict provider envelope into the
// canonical readiness result. Unknown fields fail closed; authority-bearing
// fields fail as authority claims; trailing data after the envelope fails
// closed. Planner-declared issues are stamped with their only legal source
// and route, canonicalized, keyed under the evaluation fence, and validated
// against the domain contract. Criterion aliases are fenced to the Contract's
// frozen aliases; work unit keys are fenced to the proposal just decoded, so
// the planner may reference real units and criteria, never invent them - and
// a no-proposal envelope admits no unit references at all.
func parsePlanningReadinessReply(
	data []byte,
	fence domain.PlanningReadinessFence,
	criterionAliases []string,
) (domain.PlanningReadinessResult, error) {
	invalid := func(format string, args ...any) (domain.PlanningReadinessResult, error) {
		return domain.PlanningReadinessResult{}, fmt.Errorf(format, args...)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var reply planningReadinessReply
	if err := decoder.Decode(&reply); err != nil {
		if field, ok := unknownJSONField(err); ok && isAuthorityClaimField(field) {
			return invalid("waldo claimed authority in the readiness envelope: unknown field %q (%s)",
				field, domain.ReadinessAuthorityClaim)
		}
		return invalid("waldo returned an unreadable readiness envelope: %v (%s)", err, domain.ReadinessPayloadInvalid)
	}
	// One envelope, nothing after it. A second value (or trailing garbage that
	// is not whitespace) means the stream was not the strict envelope.
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return invalid("waldo returned trailing data after the readiness envelope (%s)", domain.ReadinessPayloadInvalid)
	}

	// The unit order and allowed unit keys derive from the proposal just
	// decoded; nothing external to this envelope defines them.
	var workUnitOrder []string
	allowedUnits := map[string]bool{}
	result := domain.PlanningReadinessResult{
		Version: domain.PlanningReadinessEnvelopeVersion,
		Status:  domain.PlanningReadinessStatus(reply.Status),
		Message: strings.TrimSpace(reply.Message),
	}
	if reply.Proposal != nil {
		proposal := planProposalFromReply(*reply.Proposal)
		if err := proposal.Validate(); err != nil {
			return invalid("waldo returned an invalid readiness proposal: %v (%s)", err, domain.ReadinessPayloadInvalid)
		}
		order, err := proposal.TopologicalOrder()
		if err != nil {
			return invalid("waldo returned an invalid readiness proposal: %v (%s)", err, domain.ReadinessPayloadInvalid)
		}
		workUnitOrder = order
		for _, key := range order {
			allowedUnits[key] = true
		}
		result.Proposal = &proposal
	}

	allowedAliases := make(map[string]bool, len(criterionAliases))
	for _, alias := range criterionAliases {
		allowedAliases[alias] = true
	}

	var droppedUnitsAll, droppedAliasesAll []string
	var droppedUnits, droppedAliases []string
	for _, raw := range reply.Issues {
		issue := domain.PlanningReadinessIssue{
			Kind:             domain.PlanningReadinessIssueKind(raw.Kind),
			Route:            domain.RouteAnswerContext,
			Source:           domain.ReadinessSourcePlannerDeclared,
			Prompt:           strings.TrimSpace(raw.Prompt),
			Reason:           strings.TrimSpace(raw.Reason),
			Recommendation:   strings.TrimSpace(raw.Recommendation),
			WorkUnitKeys:     trimAll(raw.WorkUnitKeys),
			CriterionAliases: trimAll(raw.CriterionAliases),
		}
		for _, rawKey := range raw.WorkUnitKeys {
			if strings.TrimSpace(rawKey) == "" {
				return invalid("waldo referenced a blank work unit key (%s)", domain.ReadinessIssueInvalid)
			}
		}
		for _, rawAlias := range raw.CriterionAliases {
			if strings.TrimSpace(rawAlias) == "" {
				return invalid("waldo referenced a blank criterion alias (%s)", domain.ReadinessIssueInvalid)
			}
		}
		// Degrade, don't invalidate: the one-shot lane has no conversation to
		// repair an unresolvable reference through, and planner output is
		// non-authoritative. Strip keys/aliases the envelope itself does not
		// define (a no-proposal envelope defines none) so no invented
		// reference can ride an issue into the evaluated result, and surface
		// ONE control-plane issue per envelope so the owner still sees the
		// drop. Blank and duplicate references remain hard-invalid: those are
		// malformed, not unresolvable.
		issue.WorkUnitKeys, droppedUnits = partitionRefs(issue.WorkUnitKeys, allowedUnits)
		issue.CriterionAliases, droppedAliases = partitionRefs(issue.CriterionAliases, allowedAliases)
		droppedUnitsAll = append(droppedUnitsAll, droppedUnits...)
		droppedAliasesAll = append(droppedAliasesAll, droppedAliases...)
		for _, choice := range raw.Choices {
			issue.Choices = append(issue.Choices, domain.PlanningReadinessChoice{
				Key: strings.TrimSpace(choice.Key), Label: strings.TrimSpace(choice.Label),
			})
		}
		result.Issues = append(result.Issues, issue)
	}
	if len(droppedUnitsAll) > 0 || len(droppedAliasesAll) > 0 {
		result.Issues = append(result.Issues, unresolvablePlannerRefIssue(result.Status, droppedUnitsAll, droppedAliasesAll))
	}

	canonical, err := domain.CanonicalizePlanningReadinessIssues(result.Issues, workUnitOrder)
	if err != nil {
		return invalid("waldo returned unreadable readiness issues: %v (%s)", err, domain.ReadinessPayloadInvalid)
	}
	result.Issues = canonical
	for i := range result.Issues {
		result.Issues[i].Key = domain.CanonicalPlanningIssueKey(fence, result.Issues[i])
	}
	if err := result.Validate(); err != nil {
		var coded *domain.PlanningReadinessValidationError
		if errors.As(err, &coded) {
			return invalid("waldo returned an invalid readiness envelope: %s: %s", coded.Code, coded.Message)
		}
		return invalid("waldo returned an invalid readiness envelope: %v", err)
	}
	return result, nil
}

// unknownJSONField extracts the field name from a DisallowUnknownFields error.
func unknownJSONField(err error) (string, bool) {
	const prefix = `json: unknown field "`
	text := err.Error()
	if !strings.HasPrefix(text, prefix) {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(text, prefix), `"`), true
}

func isAuthorityClaimField(field string) bool {
	for _, claim := range authorityClaimFields {
		if field == claim {
			return true
		}
	}
	return false
}

// partitionRefs splits refs into those the envelope itself defines (kept) and
// those it does not (dropped). Order is preserved in both partitions.
func partitionRefs(refs []string, allowed map[string]bool) (kept, dropped []string) {
	for _, ref := range refs {
		if allowed[ref] {
			kept = append(kept, ref)
		} else {
			dropped = append(dropped, ref)
		}
	}
	return kept, dropped
}

// unresolvablePlannerRefIssue is the control-plane issue recording that a
// planner-declared issue named units or criteria its own envelope does not
// define. It carries no planner reference: the dropped values live only in
// the prose so no invented key can reach the evaluated result.
func unresolvablePlannerRefIssue(status domain.PlanningReadinessStatus, units, aliases []string) domain.PlanningReadinessIssue {
	// A needs_context envelope admits only answer-context routes (mixed
	// packets are blocked), so the degrade issue follows the envelope status.
	route := domain.RouteRevisePlan
	if status == domain.PlanningNeedsContext {
		route = domain.RouteAnswerContext
	}
	parts := []string{}
	if len(units) > 0 {
		parts = append(parts, fmt.Sprintf("work unit key(s) %s", quotedJoin(units)))
	}
	if len(aliases) > 0 {
		parts = append(parts, fmt.Sprintf("criterion alias(es) %s", quotedJoin(aliases)))
	}
	what := strings.Join(parts, " and ")
	return domain.PlanningReadinessIssue{
		Kind:           domain.ReadinessReferenceUnresolvable,
		Route:          route,
		Source:         domain.ReadinessSourceControlPlane,
		Prompt:         fmt.Sprintf("The planner's notes referenced %s, which its own proposal does not define; the reference was dropped.", what),
		Reason:         "A planner-declared issue named work or criteria outside the returned envelope, so the reference cannot be evaluated against real units.",
		Recommendation: "Re-run planning or revise the plan so every referenced unit and criterion exists in the proposal.",
	}
}

func quotedJoin(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, v := range values {
		quoted = append(quoted, fmt.Sprintf("%q", v))
	}
	return strings.Join(quoted, ", ")
}
