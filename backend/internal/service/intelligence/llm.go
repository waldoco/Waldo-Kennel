package intelligence

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// LLMProviderID names model-backed reasoning in provenance records.
const LLMProviderID domain.IntelligenceProviderID = "waldo-llm"

// LLMProvider is Waldo's reasoning implementation. It proposes Contract and
// Plan material and nothing else: it starts no session, holds no authority,
// and its output is validated by the Go control plane before it can bind
// anything.
type LLMProvider struct {
	client ports.LLMClient
}

var _ ports.IntelligenceProvider = (*LLMProvider)(nil)

// NewLLMProvider wires model-backed intelligence behind the provider port.
func NewLLMProvider(client ports.LLMClient) *LLMProvider {
	return &LLMProvider{client: client}
}

// ID returns the model-backed provider identity.
func (*LLMProvider) ID() domain.IntelligenceProviderID { return LLMProviderID }

const contractSystemPrompt = `You are Waldo, the reasoning half of an outcome control plane for software work.

A person has stated something they want to be true. Your job is to turn that into a precise, verifiable Contract, or to ask ONE question if a genuinely material fact is missing.

Rules:
- Derive as much as you safely can. Do not interrogate. Ask a question ONLY when a reasonable person could build two materially different results from the same statement, and the choice changes what "done" means.
- Never ask about anything you could reasonably assume and state as an assumption instead.
- Success criteria must be observable and checkable by someone who did not do the work. "Works well" is not a criterion; "the CLI exits 0 and prints the parsed config" is.
- Each criterion carries the evidence that would prove it.
- The authority ceiling is the MAXIMUM you would ever need, and it must be the least that could do the job. Default to workspace read/write/execute. Only request network, commit, PR, deploy, or external effects when the stated outcome plainly cannot be reached without them.
- Stop conditions name the moments a human must decide before work continues.
- Non-goals matter: name the adjacent work you are deliberately NOT doing.

Return only the structured object.`

const planSystemPrompt = `You are Waldo, planning execution for an approved Contract in an outcome control plane.

Return one readiness envelope: status "ready" with a proposal, or status "needs_context" with every material missing fact or bounded context request batched as issues. A proposal carrying real blockers is never ready: if a missing fact could change the graph, proof, or safe execution, ask instead of proposing. You are proposing work, not authorizing it: the control plane derives capabilities, routing, stop policy, and verification from what you return.

Rules:
- Prefer few units. One unit is correct when the work is genuinely one step. Never split work just to look thorough.
- Each unit must produce an observable output. A criterion-free enabling unit is valid only when a later unit directly consumes it.
- role describes orchestration purpose only: investigate, implement, verify, or consolidate. It never grants authority; intent alone determines local capabilities. Verify cannot mutate. Consolidate requires at least two direct predecessors.
- inputs must contain exactly one semantic handoff requirement for every direct dependsOn key, and no others. Describe what predecessor result is needed, never a path, URI, command, secret, capability, or artifact id.
- Use dependsOn only for real ordering constraints. Independent units keep future execution options open, but the current launch executes the canonical Plan order serially. Do not add fake dependencies merely to force display or execution order.
- intent classifies the work: "inspect" reads only; "modify" edits files; "execute" runs commands; "modify_and_execute" does both. Choose the LEAST intent that can do the unit's job — it decides how much authority the unit is granted.
- criteriaCovered references the criterion aliases given to you (C1, C2, ...). Every criterion should be covered by at least one unit.
- evidenceIdeas are the artifacts that would prove the unit did its job.
- checkCommands are proposed deterministic local checks: criterionAlias, exact argv array (not shell text), and timeoutSeconds. Never invoke sh, bash, zsh, or another shell, including shell -c: the daemon rejects shell-based checks even when local command execution is allowed. Do not use pipes, redirection, command substitution, or shell operators. Invoke a permitted executable directly with individual arguments. For exact file content checks, an available python3 interpreter may use -c with a read-only assertion that exits nonzero for missing or incorrect bytes. Do not propose a check that only prints the file. Ground them in the approved context. A check must exit nonzero when its criterion is false, not merely print output. Use execute or modify_and_execute intent when checks execute commands. Never widen the Contract authority. Return an empty list only when no safe deterministic check is available; explain the verification limitation in blockers.
- Issues are batched: return every material question in one needs_context packet, each with a reason explaining why the fact changes the graph, proof, or safe execution. If the Contract itself seems wrong, ask a context question whose choices name the change — the owner revises the Contract, you never do.
- Record real assumptions. An empty list is the honest answer when there are none; never invent them.

Return only the structured object.`

const planningDiscussionSystemPrompt = `You are Waldo, discussing an execution plan for a confirmed Contract.

You receive an immutable Contract, a bounded read-only repository packet, and the visible planning conversation. Investigate the supplied facts and make the next useful planning move.

Return one readiness envelope: status "ready" with a proposal, or status "needs_context" with every material missing fact or bounded context request batched as issues. A proposal carrying real blockers is never ready: if a missing fact could change the graph, proof, or safe execution, ask instead of proposing.

Rules:
- Do not ask the owner to perform repository analysis already present in the packet.
- Do not claim commands ran. Planning authorizes no commands, writes, network access, commits, pull requests, deployment, or external effects.
- If the Contract itself seems wrong, ask a context question whose choices name the change. The owner revises the Contract; never treat conversational agreement as Contract confirmation.
- Prefer a small plan. Cover every criterion alias with proof-bearing units and use the least WorkUnit intent. Roles describe investigate/implement/verify/consolidate purpose but grant no authority. Every dependency must have one matching semantic input requirement.
- Proposed checks are exact argv arrays and must fail when their criterion is false. Never use a shell or shell operators.
- The control plane, not you, derives authority, routing, stop policy, verification, approval, and execution.

Return only the structured object.`

// contractSchema constrains the reply to one clarification or one proposal.
//
// Codex's strict structured-output mode requires every key under
// additionalProperties:false to appear in "required" — a key that is only
// sometimes meaningful (like "question" vs "proposal", chosen by "decision")
// must instead be nullable (nullableObject) rather than omitted from
// required, or the schema itself is rejected before the model ever runs.
func contractSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"decision", "question", "proposal"},
		"properties": map[string]any{
			"decision": map[string]any{
				"type":        "string",
				"enum":        []any{"ask", "propose"},
				"description": "ask only when a material fact is missing",
			},
			"question": nullableObject(map[string]any{
				"additionalProperties": false,
				"required":             []any{"question", "reason", "recommendation", "alternatives", "deferralConsequence"},
				"properties": map[string]any{
					"question":            map[string]any{"type": "string"},
					"reason":              map[string]any{"type": "string", "description": "why this changes what done means"},
					"recommendation":      map[string]any{"type": "string", "description": "what you would choose"},
					"alternatives":        stringArray("the concrete options"),
					"deferralConsequence": map[string]any{"type": "string", "description": "what happens if unanswered"},
				},
			}),
			"proposal": nullableObject(map[string]any{
				"additionalProperties": false,
				"required":             []any{"title", "desiredState", "criteria", "reviewMethod", "constraints", "nonGoals", "authorityCeiling", "stopConditions", "assumptions", "facet"},
				"properties": map[string]any{
					"title":        map[string]any{"type": "string", "description": "short result-shaped name, not a task name"},
					"desiredState": map[string]any{"type": "string", "description": "what will be true when this is done"},
					"criteria": map[string]any{
						"type":     "array",
						"minItems": 1,
						"maxItems": 12,
						"items": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []any{"text", "evidenceExpected"},
							"properties": map[string]any{
								"text":             map[string]any{"type": "string"},
								"evidenceExpected": stringArray("artifacts that would prove this criterion"),
							},
						},
					},
					"reviewMethod":     map[string]any{"type": "string"},
					"constraints":      stringArray("limits the work must respect"),
					"nonGoals":         stringArray("adjacent work deliberately excluded"),
					"authorityCeiling": authoritySchema(),
					"stopConditions":   stringArray("moments a human must decide"),
					"assumptions":      stringArray("what you assumed rather than asked"),
					"facet": map[string]any{
						"type": "string",
						"enum": []any{"software", "research", "design", "documentation", "investigation", "evaluation", "operations"},
					},
				},
			}),
		},
	}
}

// nullableObject marks an object schema as optionally null: Codex's strict
// mode requires the key to be listed in the parent's "required" array, but
// the value itself may be JSON null when the field does not apply (e.g. a
// "proposal" that is absent while "decision" chose "ask"). schema must not
// itself set "type"; nullableObject supplies it.
func nullableObject(schema map[string]any) map[string]any {
	schema["type"] = []any{"object", "null"}
	return schema
}

func authoritySchema() map[string]any {
	props := map[string]any{}
	names := []string{"readWorkspace", "writeWorkspace", "executeLocal", "useNetwork", "commitLocal", "createPR", "deploy", "externalEffect"}
	required := make([]any, 0, len(names))
	for _, name := range names {
		props[name] = map[string]any{"type": "boolean"}
		required = append(required, name)
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             required,
		"properties":           props,
		"description":          "the least authority that could complete the outcome",
	}
}

func stringArray(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"items":       map[string]any{"type": "string"},
		"description": description,
	}
}

func planSchema(aliases []string) map[string]any {
	criterionAlias := map[string]any{"type": "string", "enum": aliases}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"summary", "workUnits", "assumptions", "blockers"},
		"properties": map[string]any{
			"summary": map[string]any{"type": "string", "description": "one line on how this plan reaches the outcome"},
			"workUnits": map[string]any{
				"type":     "array",
				"minItems": 1,
				"maxItems": float64(domain.MaxPlanDraftWorkUnits),
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []any{"key", "title", "intent", "role", "inputs", "outputSummary", "criteriaCovered", "dependsOn", "evidenceIdeas", "checkCommands"},
					"properties": map[string]any{
						"key":   map[string]any{"type": "string", "description": "stable short id such as W1"},
						"title": map[string]any{"type": "string"},
						"intent": map[string]any{
							"type": "string",
							"enum": []any{"inspect", "modify", "execute", "modify_and_execute"},
						},
						"role":            map[string]any{"type": "string", "enum": []any{"investigate", "implement", "verify", "consolidate"}},
						"inputs":          map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "required": []any{"fromKey", "required"}, "properties": map[string]any{"fromKey": map[string]any{"type": "string"}, "required": map[string]any{"type": "string"}}}},
						"outputSummary":   map[string]any{"type": "string", "description": "the observable result of this unit"},
						"criteriaCovered": map[string]any{"type": "array", "items": criterionAlias},
						"dependsOn":       stringArray("keys of units that must finish first"),
						"evidenceIdeas":   stringArray("artifacts that would prove this unit"),
						"checkCommands": map[string]any{
							"type":        "array",
							"description": "Proposed local checks, subject to owner approval and daemon validation",
							"items": map[string]any{
								"type":                 "object",
								"additionalProperties": false,
								"required":             []any{"criterionAlias", "argv", "timeoutSeconds"},
								"properties": map[string]any{
									"criterionAlias": criterionAlias,
									"argv":           stringArray("Exact executable and arguments; preserve each argument verbatim"),
									"timeoutSeconds": map[string]any{"type": "integer", "minimum": 1},
								},
							},
						},
					},
				},
			},
			"assumptions": stringArray("real assumptions only"),
			"blockers":    stringArray("real blockers only"),
		},
	}
}

type contractReply struct {
	Decision string `json:"decision"`
	Question *struct {
		Question            string   `json:"question"`
		Reason              string   `json:"reason"`
		Recommendation      string   `json:"recommendation"`
		Alternatives        []string `json:"alternatives"`
		DeferralConsequence string   `json:"deferralConsequence"`
	} `json:"question"`
	Proposal *struct {
		Title        string `json:"title"`
		DesiredState string `json:"desiredState"`
		Criteria     []struct {
			Text             string   `json:"text"`
			EvidenceExpected []string `json:"evidenceExpected"`
		} `json:"criteria"`
		ReviewMethod     string   `json:"reviewMethod"`
		Constraints      []string `json:"constraints"`
		NonGoals         []string `json:"nonGoals"`
		AuthorityCeiling struct {
			ReadWorkspace  bool `json:"readWorkspace"`
			WriteWorkspace bool `json:"writeWorkspace"`
			ExecuteLocal   bool `json:"executeLocal"`
			UseNetwork     bool `json:"useNetwork"`
			CommitLocal    bool `json:"commitLocal"`
			CreatePR       bool `json:"createPR"`
			Deploy         bool `json:"deploy"`
			ExternalEffect bool `json:"externalEffect"`
		} `json:"authorityCeiling"`
		StopConditions []string `json:"stopConditions"`
		Assumptions    []string `json:"assumptions"`
		Facet          string   `json:"facet"`
	} `json:"proposal"`
}

type planReply struct {
	Summary   string `json:"summary"`
	WorkUnits []struct {
		Key    string `json:"key"`
		Title  string `json:"title"`
		Intent string `json:"intent"`
		Role   string `json:"role"`
		Inputs []struct {
			FromKey  string `json:"fromKey"`
			Required string `json:"required"`
		} `json:"inputs"`
		OutputSummary   string   `json:"outputSummary"`
		CriteriaCovered []string `json:"criteriaCovered"`
		DependsOn       []string `json:"dependsOn"`
		EvidenceIdeas   []string `json:"evidenceIdeas"`
		CheckCommands   []struct {
			CriterionAlias string   `json:"criterionAlias"`
			Argv           []string `json:"argv"`
			TimeoutSeconds int64    `json:"timeoutSeconds"`
		} `json:"checkCommands"`
	} `json:"workUnits"`
	Assumptions []string `json:"assumptions"`
	Blockers    []string `json:"blockers"`
}

// AnalyzeContract asks the model for one clarification or one editable
// proposal. Nothing it returns creates responsibility: the owner still
// confirms, and the control plane still validates.
func (p *LLMProvider) AnalyzeContract(ctx context.Context, request ports.ContractIntelligenceRequest) (ports.ContractIntelligenceResponse, error) {
	if p == nil || p.client == nil {
		// A classified failure so this reaches the owner as an actionable
		// setup state rather than an opaque 500.
		return ports.ContractIntelligenceResponse{}, ports.NewReasoningFailure(
			ports.ReasoningNotConfigured, "Waldo reasoning is not configured", nil)
	}

	var input strings.Builder
	fmt.Fprintf(&input, "Stated outcome:\n%s\n", strings.TrimSpace(request.Session.Statement))
	if len(request.ConversationRefs) > 0 {
		fmt.Fprintf(&input, "\nConversation evidence references:\n")
		for _, ref := range request.ConversationRefs {
			fmt.Fprintf(&input, "- episode=%s turn=%s position=%d\n", ref.EpisodeID, ref.TurnID, ref.Position)
		}
	}
	if answer := strings.TrimSpace(request.ClarificationText); answer != "" {
		question := ""
		if request.Clarification != nil {
			question = strings.TrimSpace(request.Clarification.Question)
		}
		fmt.Fprintf(&input, "\nThe owner was asked: %s\nThey answered: %s\nDo not ask again; propose the Contract.\n", question, answer)
	}
	if request.PreviousProposal != nil {
		previous, _ := json.Marshal(request.PreviousProposal)
		fmt.Fprintf(&input, "\nThe previous Contract proposal was not accepted. Preserve useful material, correct it where needed, and produce an immutable replacement:\n%s\n", previous)
	}
	appendRepositoryContext(&input, request.RepositoryContext)

	contextAccess, err := reasoningContextAccess(request.RepositoryToolUse, request.RepositoryContext)
	if err != nil {
		return ports.ContractIntelligenceResponse{}, err
	}
	response, err := p.client.Complete(ctx, ports.LLMRequest{
		System:        contractSystemPrompt,
		User:          input.String(),
		SchemaName:    "contract_proposal",
		Schema:        contractSchema(),
		ContextAccess: contextAccess,
	})
	if err != nil {
		return ports.ContractIntelligenceResponse{}, err
	}

	var reply contractReply
	if err := json.Unmarshal(response.JSON, &reply); err != nil {
		return ports.ContractIntelligenceResponse{}, fmt.Errorf("waldo returned an unreadable contract proposal: %w", err)
	}

	provenance := ports.IntelligenceProvenance{
		EffectiveProvider: domain.IntelligenceProviderID(p.client.ID()),
		EffectiveModel:    response.EffectiveModel,
		NativeSessionRef:  response.NativeSessionRef,
		InputTokens:       response.InputTokens,
		OutputTokens:      response.OutputTokens,
	}

	// A clarification is only honoured when the owner has not already
	// answered one: the control plane allows exactly one open ask.
	if reply.Decision == "ask" && reply.Question != nil && strings.TrimSpace(request.ClarificationText) == "" {
		return ports.ContractIntelligenceResponse{
			Result: ports.IntakeAnalysisResult{Clarification: &domain.ClarificationRequest{
				Question:            strings.TrimSpace(reply.Question.Question),
				Reason:              strings.TrimSpace(reply.Question.Reason),
				Recommendation:      strings.TrimSpace(reply.Question.Recommendation),
				Alternatives:        trimAll(reply.Question.Alternatives),
				DeferralConsequence: strings.TrimSpace(reply.Question.DeferralConsequence),
			}},
			Provenance: provenance,
		}, nil
	}

	if reply.Proposal == nil {
		return ports.ContractIntelligenceResponse{}, fmt.Errorf("waldo returned neither a question nor a contract proposal")
	}
	source := reply.Proposal

	criteria := make([]domain.ProposedCriterion, 0, len(source.Criteria))
	for _, criterion := range source.Criteria {
		text := strings.TrimSpace(criterion.Text)
		if text == "" {
			continue
		}
		criteria = append(criteria, domain.ProposedCriterion{
			Text:             text,
			EvidenceExpected: trimAll(criterion.EvidenceExpected),
		})
	}
	if len(criteria) == 0 {
		return ports.ContractIntelligenceResponse{}, fmt.Errorf("waldo proposed a contract with no success criteria")
	}

	proposal := &domain.OutcomeContractProposal{
		Title:        strings.TrimSpace(source.Title),
		DesiredState: strings.TrimSpace(source.DesiredState),
		Criteria:     criteria,
		ReviewMethod: strings.TrimSpace(source.ReviewMethod),
		Constraints:  trimAll(source.Constraints),
		NonGoals:     trimAll(source.NonGoals),
		AuthorityCeiling: domain.ProposedAuthority{
			ReadWorkspace:  source.AuthorityCeiling.ReadWorkspace,
			WriteWorkspace: source.AuthorityCeiling.WriteWorkspace,
			ExecuteLocal:   source.AuthorityCeiling.ExecuteLocal,
			UseNetwork:     source.AuthorityCeiling.UseNetwork,
			CommitLocal:    source.AuthorityCeiling.CommitLocal,
			CreatePR:       source.AuthorityCeiling.CreatePR,
			Deploy:         source.AuthorityCeiling.Deploy,
			ExternalEffect: source.AuthorityCeiling.ExternalEffect,
		},
		StopConditions:     trimAll(source.StopConditions),
		ClarificationNotes: trimAll(source.Assumptions),
		Facets:             []domain.ContractFacet{{Kind: facetKind(source.Facet), Summary: strings.TrimSpace(source.Title)}},
	}
	return ports.ContractIntelligenceResponse{
		Result:     ports.IntakeAnalysisResult{Proposal: proposal},
		Provenance: provenance,
	}, nil
}

// DraftPlan asks the model how the frozen Contract should be executed. The
// reply is non-authoritative: Kennel compiles the canonical PlanRevision.
func (p *LLMProvider) DraftPlan(ctx context.Context, request ports.PlanIntelligenceRequest) (ports.PlanIntelligenceResponse, error) {
	if p == nil || p.client == nil {
		return ports.PlanIntelligenceResponse{}, ports.NewReasoningFailure(
			ports.ReasoningNotConfigured, "Waldo reasoning is not configured", nil)
	}

	var input strings.Builder
	fmt.Fprintf(&input, "Outcome: %s\n\nGoal:\n%s\n\nSuccess criteria:\n", request.Outcome.Title, request.Contract.Goal)
	for _, alias := range sortedAliasKeys(request.CriterionAliases) {
		fmt.Fprintf(&input, "%s. %s\n", alias, criterionText(request.Contract, request.CriterionAliases[alias]))
	}
	if review := strings.TrimSpace(request.Contract.Review); review != "" {
		fmt.Fprintf(&input, "\nHow the result will be reviewed:\n%s\n", review)
	}
	if len(request.Contract.Constraints) > 0 {
		fmt.Fprintf(&input, "\nConstraints:\n- %s\n", strings.Join(request.Contract.Constraints, "\n- "))
	}
	if len(request.Contract.NonGoals) > 0 {
		fmt.Fprintf(&input, "\nExplicitly not in scope:\n- %s\n", strings.Join(request.Contract.NonGoals, "\n- "))
	}
	ceiling := request.Contract.AuthorityCeiling
	fmt.Fprintf(&input, "\nFrozen Contract permissions (false means forbidden; this proposal cannot change them):\nreadWorkspace=%t\nwriteWorkspace=%t\nexecuteLocal=%t\nuseNetwork=%t\ncommitLocal=%t\ncreatePR=%t\ndeploy=%t\nexternalEffect=%t\n",
		ceiling.ReadWorkspace, ceiling.WriteWorkspace, ceiling.ExecuteLocal, ceiling.UseNetwork,
		ceiling.CommitLocal, ceiling.CreatePR, ceiling.Deploy, ceiling.ExternalEffect)
	if !ceiling.ExecuteLocal {
		input.WriteString("Local command execution is forbidden: checkCommands must be null and no unit may use an executing intent. Use permitted inspection and evidence for owner review; do not invent executable verification or claim manual review proves a criterion automatically. If the result needs more authority, return needs_context asking for an owner Contract revision.")
	}
	if feedback := strings.TrimSpace(request.ReplanFeedback); feedback != "" {
		fmt.Fprintf(&input, "\nOwner replan feedback (this is an explicit new proposal request):\n%s\n", feedback)
	}
	appendRepositoryContext(&input, request.RepositoryContext)

	aliases := sortedAliasKeys(request.CriterionAliases)
	schema := planningReadinessSchema(aliases)
	if !ceiling.ExecuteLocal {
		proposal, ok := schema["properties"].(map[string]any)["proposal"].(map[string]any)
		if !ok {
			return ports.PlanIntelligenceResponse{}, fmt.Errorf("invalid readiness proposal schema")
		}
		if err := restrictPlanSchemaToContract(proposal, ceiling); err != nil {
			return ports.PlanIntelligenceResponse{}, err
		}
		input.WriteString("For this read-only command boundary, encode checkCommands as null as required by the schema.\n")
	}
	response, err := p.client.Complete(ctx, ports.LLMRequest{
		System:     planSystemPrompt,
		User:       input.String(),
		SchemaName: "planning_readiness",
		Schema:     schema,
	})
	if err != nil {
		return ports.PlanIntelligenceResponse{}, err
	}

	result, err := parsePlanningReadinessReply(response.JSON, request.Fence, aliases)
	if err != nil {
		logReadinessInvalid(response.JSON, err)
		return ports.PlanIntelligenceResponse{}, err
	}
	logUnresolvableRefs(result.Issues)
	return ports.PlanIntelligenceResponse{
		Readiness: result,
		Provenance: ports.IntelligenceProvenance{
			EffectiveProvider: domain.IntelligenceProviderID(p.client.ID()),
			EffectiveModel:    response.EffectiveModel,
			NativeSessionRef:  response.NativeSessionRef,
			InputTokens:       response.InputTokens,
			OutputTokens:      response.OutputTokens,
		},
	}, nil
}

// DiscussPlan performs one structured turn in a durable planning conversation.
// Continuity remains canonical in Kennel and is supplied explicitly. The reply
// is the strict readiness envelope: ready with a proposal, or needs_context
// with every material issue batched. Nothing here is authority.
func (p *LLMProvider) DiscussPlan(ctx context.Context, request ports.PlanningDiscussionRequest) (ports.PlanningDiscussionResponse, error) {
	if p == nil || p.client == nil {
		return ports.PlanningDiscussionResponse{}, ports.NewReasoningFailure(
			ports.ReasoningNotConfigured, "Waldo reasoning is not configured", nil)
	}

	var input strings.Builder
	fmt.Fprintf(&input, "Outcome: %s\n\nConfirmed Contract revision: %d\nGoal: %s\n\nCriteria:\n",
		request.Outcome.Title, request.Contract.Number, request.Contract.Goal)
	for _, alias := range sortedAliasKeys(request.CriterionAliases) {
		fmt.Fprintf(&input, "%s. %s\n", alias, criterionText(request.Contract, request.CriterionAliases[alias]))
	}
	if request.Contract.Review != "" {
		fmt.Fprintf(&input, "\nReview: %s\n", request.Contract.Review)
	}
	if len(request.Contract.Constraints) > 0 {
		fmt.Fprintf(&input, "\nConstraints:\n- %s\n", strings.Join(request.Contract.Constraints, "\n- "))
	}
	if len(request.Contract.NonGoals) > 0 {
		fmt.Fprintf(&input, "\nNon-goals:\n- %s\n", strings.Join(request.Contract.NonGoals, "\n- "))
	}
	ceiling := request.Contract.AuthorityCeiling
	fmt.Fprintf(&input, "\nContract authority ceiling:\nreadWorkspace=%t\nwriteWorkspace=%t\nexecuteLocal=%t\nuseNetwork=%t\ncommitLocal=%t\ncreatePR=%t\ndeploy=%t\nexternalEffect=%t\n",
		ceiling.ReadWorkspace, ceiling.WriteWorkspace, ceiling.ExecuteLocal, ceiling.UseNetwork,
		ceiling.CommitLocal, ceiling.CreatePR, ceiling.Deploy, ceiling.ExternalEffect)
	appendRepositoryContext(&input, request.RepositoryContext)
	input.WriteString("\nVisible planning conversation:\n")
	for _, turn := range request.Turns {
		fmt.Fprintf(&input, "- %s (%s): %s\n", turn.Role, turn.Kind, strings.TrimSpace(turn.Text))
	}
	if request.Finalize {
		input.WriteString("\nThe owner requested a Plan proposal now. Return ready with a proposal unless material facts are still missing; batch every missing fact into one needs_context packet otherwise.\n")
	}
	aliases := sortedAliasKeys(request.CriterionAliases)
	schema := planningReadinessSchema(aliases)
	if !ceiling.ExecuteLocal {
		proposal, ok := schema["properties"].(map[string]any)["proposal"].(map[string]any)
		if !ok {
			return ports.PlanningDiscussionResponse{}, fmt.Errorf("invalid readiness proposal schema")
		}
		if err := restrictPlanSchemaToContract(proposal, ceiling); err != nil {
			return ports.PlanningDiscussionResponse{}, err
		}
		input.WriteString("Local command execution is forbidden: a Plan proposal must encode checkCommands as null and cannot use an executing intent.\n")
	}

	contextAccess, err := reasoningContextAccess(request.RepositoryToolUse, request.RepositoryContext)
	if err != nil {
		return ports.PlanningDiscussionResponse{}, err
	}
	response, err := p.client.Complete(ctx, ports.LLMRequest{
		System: planningDiscussionSystemPrompt, User: input.String(), SchemaName: "planning_readiness",
		Schema: schema, ContextAccess: contextAccess,
	})
	if err != nil {
		return ports.PlanningDiscussionResponse{}, err
	}
	result, err := parsePlanningReadinessReply(response.JSON, request.Fence, aliases)
	if err != nil {
		logReadinessInvalid(response.JSON, err)
		return ports.PlanningDiscussionResponse{}, err
	}
	logUnresolvableRefs(result.Issues)
	provenance := ports.IntelligenceProvenance{
		EffectiveProvider: domain.IntelligenceProviderID(p.client.ID()), EffectiveModel: response.EffectiveModel,
		NativeSessionRef: response.NativeSessionRef, InputTokens: response.InputTokens, OutputTokens: response.OutputTokens,
	}
	return ports.PlanningDiscussionResponse{Result: result, Provenance: provenance}, nil
}

func reasoningContextAccess(authorized bool, snapshot ports.RepositoryContextSnapshot) (ports.ReasoningContextAccess, error) {
	if !authorized {
		return ports.ReasoningContextAccess{}, nil
	}
	root := filepath.Clean(strings.TrimSpace(snapshot.Root))
	if snapshot.UnavailableReason != "" || !filepath.IsAbs(root) {
		return ports.ReasoningContextAccess{}, ports.NewReasoningFailure(
			ports.ReasoningInvalidOutput, "Authorized repository context is unavailable for native inspection", nil)
	}
	access := ports.ReasoningContextAccess{Mode: ports.ReasoningContextRepositoryRead, Root: root}
	if err := access.Validate(); err != nil {
		return ports.ReasoningContextAccess{}, ports.NewReasoningFailure(
			ports.ReasoningInvalidOutput, "Authorized repository context is invalid for native inspection", err)
	}
	return access, nil
}

func restrictPlanSchemaToContract(schema map[string]any, ceiling domain.ProposedAuthority) error {
	unit := schema
	for _, key := range []string{"properties", "workUnits", "items", "properties"} {
		next, ok := unit[key].(map[string]any)
		if !ok {
			return fmt.Errorf("invalid plan schema at %s", key)
		}
		unit = next
	}
	// A null field is portable across strict-output providers, unlike a
	// maxItems:0 bound that some schema adapters must strip. It decodes to no
	// proposed checks; the daemon still validates every returned Plan.
	unit["checkCommands"] = map[string]any{"type": "null", "description": "No executable checks: the Contract forbids local command execution"}
	intents := []any{"inspect"}
	if ceiling.WriteWorkspace {
		intents = append(intents, "modify")
	}
	unit["intent"] = map[string]any{"type": "string", "enum": intents}
	return nil
}

func planProposalFromReply(reply planReply) domain.PlanDraftProposal {
	units := make([]domain.PlanDraftWorkUnit, 0, len(reply.WorkUnits))
	for _, unit := range reply.WorkUnits {
		key := strings.TrimSpace(unit.Key)
		if key == "" {
			continue
		}
		units = append(units, domain.PlanDraftWorkUnit{
			Key: key, Title: strings.TrimSpace(unit.Title), Intent: domain.WorkUnitIntent(strings.TrimSpace(unit.Intent)), Role: domain.WorkUnitRole(strings.TrimSpace(unit.Role)), Inputs: planDraftInputs(unit.Inputs),
			OutputSummary: strings.TrimSpace(unit.OutputSummary), CriteriaCovered: trimAll(unit.CriteriaCovered),
			DependsOn: trimAll(unit.DependsOn), EvidenceIdeas: trimAll(unit.EvidenceIdeas), CheckCommands: planDraftChecks(unit.CheckCommands),
		})
	}
	return domain.PlanDraftProposal{Summary: strings.TrimSpace(reply.Summary), WorkUnits: units, Assumptions: trimAll(reply.Assumptions), Blockers: trimAll(reply.Blockers)}
}

func appendRepositoryContext(input *strings.Builder, snapshot ports.RepositoryContextSnapshot) {
	input.WriteString("\nBounded repository context (inspected facts only; no checks were run):\n")
	if snapshot.UnavailableReason != "" {
		fmt.Fprintf(input, "- context limitation: %s\n", snapshot.UnavailableReason)
	}
	fmt.Fprintf(input, "- project: %s\n- root: %s\n- revision: %s\n- dirty: %t\n- context digest: %s\n", snapshot.ProjectID, snapshot.Root, snapshot.Revision, snapshot.Dirty, snapshot.Digest)
	if snapshot.ProjectBrief != nil {
		brief, _ := json.Marshal(snapshot.ProjectBrief)
		fmt.Fprintf(input, "- Project Brief:\n%s\n", brief)
	}
	for _, instruction := range snapshot.Instructions {
		fmt.Fprintf(input, "- instruction file %s (truncated: %t):\n%s\n", instruction.Path, instruction.Truncated, instruction.Content)
	}
	for _, file := range snapshot.Files {
		fmt.Fprintf(input, "- inspected file %s (truncated: %t):\n%s\n", file.Path, file.Truncated, file.Content)
	}
	if len(snapshot.CheckCommands) > 0 {
		fmt.Fprintf(input, "- discovered check commands (not executed):\n- %s\n", strings.Join(snapshot.CheckCommands, "\n- "))
	}
}

// sortedAliasKeys returns the model-facing criterion aliases in stable order
// so identical Contracts produce identical prompts.
func sortedAliasKeys(aliases map[string]domain.CriterionID) []string {
	keys := make([]string, 0, len(aliases))
	for alias := range aliases {
		keys = append(keys, alias)
	}
	sort.Strings(keys)
	return keys
}

// criterionText resolves one canonical criterion's display text.
func criterionText(contract domain.ContractRevision, id domain.CriterionID) string {
	for _, criterion := range contract.Criteria {
		if criterion.ID == id {
			return criterion.Text
		}
	}
	return ""
}

func trimAll(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func facetKind(raw string) domain.ContractFacetKind {
	switch kind := domain.ContractFacetKind(strings.TrimSpace(strings.ToLower(raw))); kind {
	case domain.ContractFacetSoftware, domain.ContractFacetResearch, domain.ContractFacetDesign,
		domain.ContractFacetDocumentation, domain.ContractFacetInvestigation,
		domain.ContractFacetEvaluation, domain.ContractFacetOperations:
		return kind
	default:
		return domain.ContractFacetSoftware
	}
}

func planDraftInputs(values []struct {
	FromKey  string `json:"fromKey"`
	Required string `json:"required"`
}) []domain.PlanDraftDependencyInput {
	out := make([]domain.PlanDraftDependencyInput, 0, len(values))
	for _, v := range values {
		out = append(out, domain.PlanDraftDependencyInput{FromKey: strings.TrimSpace(v.FromKey), Required: strings.TrimSpace(v.Required)})
	}
	return out
}

// logReadinessInvalid WARN-logs a bounded prefix of a rejected readiness
// envelope so the next planner/schema inconsistency is diagnosable from
// preserved daemon logs (the e2e artifact path keeps them on failure).
func logReadinessInvalid(raw []byte, err error) {
	const maxEnvelope = 2000
	excerpt := string(raw)
	if len(excerpt) > maxEnvelope {
		excerpt = excerpt[:maxEnvelope]
	}
	slog.Default().Warn("waldo readiness envelope rejected", "error", err, "envelope_prefix", excerpt)
}

// logUnresolvableRefs WARN-logs degrade issues: the plan survives, but the
// dropped planner references stay visible in the daemon log as well as in
// the owner-facing issue.
func logUnresolvableRefs(issues []domain.PlanningReadinessIssue) {
	for _, issue := range issues {
		if issue.Kind == domain.ReadinessReferenceUnresolvable {
			slog.Default().Warn("planner-declared issue referenced undefined units or criteria; references dropped", "detail", issue.Prompt)
		}
	}
}
