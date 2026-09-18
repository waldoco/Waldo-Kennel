package intelligence

import (
	"context"
	"strings"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

type captureLLMClient struct {
	request ports.LLMRequest
	result  string
}

func (c *captureLLMClient) ID() string { return "test-provider" }
func (c *captureLLMClient) Complete(_ context.Context, request ports.LLMRequest) (ports.LLMResponse, error) {
	c.request = request
	return ports.LLMResponse{JSON: []byte(c.result), EffectiveModel: "test-model"}, nil
}

func TestAnalyzeContractPreservesClarificationAsContextNotTemporalSemantics(t *testing.T) {
	client := &captureLLMClient{result: `{"decision":"propose","proposal":{"title":"Login continuity","desiredState":"Email login remains available","criteria":[{"text":"Email login continues to work","evidenceExpected":["login test"]}],"reviewMethod":"Run the login test","constraints":[],"nonGoals":[],"authorityCeiling":{"readWorkspace":true,"writeWorkspace":false,"executeLocal":true,"useNetwork":false,"commitLocal":false,"createPR":false,"deploy":false,"externalEffect":false},"stopConditions":["Stop on auth data changes"],"assumptions":["Existing auth provider is retained"],"facet":"software"}}`}
	provider := NewLLMProvider(client)
	answer := "preserve email login"
	result, err := provider.AnalyzeContract(context.Background(), ports.ContractIntelligenceRequest{
		Session:           domain.IntakeSession{Statement: "Keep the login flow stable"},
		Clarification:     &domain.ClarificationRequest{Question: "What must remain stable?"},
		ClarificationText: answer,
		PreviousProposal:  &domain.OutcomeContractProposal{Title: "Old login proposal"},
		RepositoryContext: ports.RepositoryContextSnapshot{Revision: "abc123", Files: []ports.RepositoryContextFile{{Path: "auth/login.go", Content: "email login"}}},
	})
	if err != nil {
		t.Fatalf("AnalyzeContract() error = %v", err)
	}
	if result.Result.Proposal == nil || result.Result.Proposal.TemporalCondition != nil {
		t.Fatalf("proposal temporal condition = %#v", result.Result.Proposal)
	}
	for _, want := range []string{"preserve email login", "Old login proposal", "auth/login.go", "abc123"} {
		if !strings.Contains(client.request.User, want) {
			t.Errorf("request omitted grounded context %q: %s", want, client.request.User)
		}
	}
}

func TestAnalyzeContractPropagatesOnlyExplicitRepositoryToolAuthority(t *testing.T) {
	for _, authorized := range []bool{false, true} {
		client := &captureLLMClient{result: `{"decision":"propose","proposal":{"title":"Inspect","desiredState":"Architecture is understood","criteria":[{"text":"Assessment cites source","evidenceExpected":["file references"]}],"reviewMethod":"Review citations","constraints":[],"nonGoals":[],"authorityCeiling":{"readWorkspace":true,"writeWorkspace":false,"executeLocal":false,"useNetwork":false,"commitLocal":false,"createPR":false,"deploy":false,"externalEffect":false},"stopConditions":[],"assumptions":[],"facet":"software"}}`}
		root := t.TempDir()
		_, err := NewLLMProvider(client).AnalyzeContract(context.Background(), ports.ContractIntelligenceRequest{
			Session:           domain.IntakeSession{Statement: "Understand the architecture"},
			RepositoryContext: ports.RepositoryContextSnapshot{Root: root},
			RepositoryToolUse: authorized,
		})
		if err != nil {
			t.Fatalf("authorized=%t: %v", authorized, err)
		}
		if authorized {
			if client.request.ContextAccess.Mode != ports.ReasoningContextRepositoryRead || client.request.ContextAccess.Root != root {
				t.Fatalf("authorized context access = %+v", client.request.ContextAccess)
			}
		} else if client.request.ContextAccess != (ports.ReasoningContextAccess{}) {
			t.Fatalf("packet-only context widened = %+v", client.request.ContextAccess)
		}
	}
}

func TestDraftPlanCarriesExplicitReplanFeedbackAndChecks(t *testing.T) {
	client := &captureLLMClient{result: `{"status":"ready","message":"Ready.","proposal":{"summary":"Use the inspected check","workUnits":[{"key":"W1","title":"Implement","intent":"modify_and_execute","role":"implement","inputs":[],"outputSummary":"Changed code and verified it","criteriaCovered":["C1"],"dependsOn":[],"evidenceIdeas":["test output"]}],"assumptions":[],"blockers":[]},"issues":[]}`}
	provider := NewLLMProvider(client)
	_, err := provider.DraftPlan(context.Background(), ports.PlanIntelligenceRequest{
		Outcome:          domain.Outcome{Title: "Grounded work"},
		Contract:         domain.ContractRevision{Goal: "Make the repo change", Criteria: []domain.ContractCriterion{{ID: "criterion-1", Text: "It works"}}},
		CriterionAliases: map[string]domain.CriterionID{"C1": "criterion-1"},
		ReplanFeedback:   "Use the distinctive repository test command",
		RepositoryContext: ports.RepositoryContextSnapshot{
			Revision:      "abc123",
			CheckCommands: []string{"npm run test:distinctive -> go test ./internal/feature"},
		},
	})
	if err != nil {
		t.Fatalf("DraftPlan() error = %v", err)
	}
	for _, want := range []string{"distinctive repository test command", "npm run test:distinctive", "abc123"} {
		if !strings.Contains(client.request.User, want) {
			t.Errorf("request omitted replan/context %q: %s", want, client.request.User)
		}
	}
}

func TestPlanSchemaAllowsExecutableCriterionChecks(t *testing.T) {
	properties := planSchema([]string{"C1"})["properties"].(map[string]any)
	unit := properties["workUnits"].(map[string]any)["items"].(map[string]any)
	checks, ok := unit["properties"].(map[string]any)["checkCommands"].(map[string]any)
	if !ok {
		t.Fatal("structured plan schema forbids executable checkCommands accepted by the compiler")
	}
	item := checks["items"].(map[string]any)
	for _, name := range []string{"criterionAlias", "argv", "timeoutSeconds"} {
		if _, ok := item["properties"].(map[string]any)[name]; !ok {
			t.Errorf("check schema missing %s", name)
		}
	}
}

func TestDraftPlanPreservesExecutableCheckArguments(t *testing.T) {
	client := &captureLLMClient{result: `{"status":"ready","message":"Ready.","proposal":{"summary":"Verify greeting","workUnits":[{"key":"W1","title":"Change and check","intent":"modify_and_execute","role":"implement","inputs":[],"outputSummary":"Correct greeting","criteriaCovered":["C1"],"checkCommands":[{"criterionAlias":"C1","argv":["python3","-c","import subprocess; assert subprocess.check_output(['python3', 'greet.py']) == b'Hello Kennel\\n'\n"],"timeoutSeconds":12}]}]},"issues":[]}`}
	result, err := NewLLMProvider(client).DraftPlan(context.Background(), ports.PlanIntelligenceRequest{CriterionAliases: map[string]domain.CriterionID{"C1": "criterion-1"}})
	if err != nil {
		t.Fatal(err)
	}
	checks := result.Readiness.Proposal.WorkUnits[0].CheckCommands
	if len(checks) != 1 || checks[0].CriterionAlias != "C1" || checks[0].TimeoutSeconds != 12 || len(checks[0].Argv) != 3 || checks[0].Argv[2] != "import subprocess; assert subprocess.check_output(['python3', 'greet.py']) == b'Hello Kennel\\n'\n" {
		t.Fatalf("check arguments or criterion binding changed: %#v", checks)
	}
}

func TestDraftPlanReceivesFrozenPermissions(t *testing.T) {
	for _, execute := range []bool{false, true} {
		client := &captureLLMClient{result: `{"status":"needs_context","message":"One question.","proposal":null,"issues":[{"kind":"context_insufficient","prompt":"Which area first?","reason":"Scope changes the plan.","recommendation":"Start with the core.","choices":[],"workUnitKeys":[],"criterionAliases":[]}]}`}
		_, err := NewLLMProvider(client).DraftPlan(context.Background(), ports.PlanIntelligenceRequest{
			Contract: domain.ContractRevision{AuthorityCeiling: domain.ProposedAuthority{ReadWorkspace: true, ExecuteLocal: execute}},
		})
		if err != nil {
			t.Fatal(err)
		}
		want := "executeLocal=false"
		if execute {
			want = "executeLocal=true"
		}
		for _, field := range []string{"readWorkspace=true", "writeWorkspace=false", want, "useNetwork=false", "commitLocal=false", "createPR=false", "deploy=false", "externalEffect=false"} {
			if !strings.Contains(client.request.User, field) {
				t.Errorf("missing frozen permission %s", field)
			}
		}
		unit := client.request.Schema["properties"].(map[string]any)["proposal"].(map[string]any)["properties"].(map[string]any)["workUnits"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
		wantType := "null"
		if execute {
			wantType = "array"
		}
		if unit["checkCommands"].(map[string]any)["type"] != wantType {
			t.Fatal("schema permits checks outside the frozen command boundary")
		}
		if !execute && !strings.Contains(client.request.User, "checkCommands must be null") {
			t.Error("read-only planning omitted the command-check restriction")
		}
	}
}

func TestDiscussPlanCarriesConversationAndRestrictsUnapprovedCommands(t *testing.T) {
	client := &captureLLMClient{result: `{"status":"needs_context","message":"One choice remains.","proposal":null,"issues":[{"kind":"fact_missing","prompt":"Keep this local?","reason":"Remote work needs authority.","recommendation":"Keep it local.","choices":[{"key":"local","label":"Keep it local."},{"key":"remote","label":"Add remote delivery later"}],"workUnitKeys":[],"criterionAliases":[]}]}`}
	result, err := NewLLMProvider(client).DiscussPlan(context.Background(), ports.PlanningDiscussionRequest{
		Outcome:           domain.Outcome{Title: "Interactive plan"},
		Contract:          domain.ContractRevision{Number: 2, Goal: "Make one local change", AuthorityCeiling: domain.ProposedAuthority{ReadWorkspace: true, WriteWorkspace: true}, Criteria: []domain.ContractCriterion{{ID: "criterion-1", Text: "The result is reviewable"}}},
		CriterionAliases:  map[string]domain.CriterionID{"C1": "criterion-1"},
		RepositoryContext: ports.RepositoryContextSnapshot{Revision: "abc123", Files: []ports.RepositoryContextFile{{Path: "README.md", Content: "grounded planning"}}},
		Turns:             []domain.PlanningTurn{{Role: domain.PlanningTurnOwner, Kind: domain.PlanningTurnMessage, Text: "Inspect first"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Result.Status != domain.PlanningNeedsContext || len(result.Result.Issues) != 1 ||
		result.Result.Issues[0].Route != domain.RouteAnswerContext || result.Result.Issues[0].Prompt != "Keep this local?" {
		t.Fatalf("planning result = %+v", result.Result)
	}
	for _, want := range []string{"Inspect first", "README.md", "abc123", "Local command execution is forbidden"} {
		if !strings.Contains(client.request.User, want) {
			t.Errorf("planning prompt omitted %q", want)
		}
	}
	proposal := client.request.Schema["properties"].(map[string]any)["proposal"].(map[string]any)
	unit := proposal["properties"].(map[string]any)["workUnits"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	if unit["checkCommands"].(map[string]any)["type"] != "null" {
		t.Fatal("interactive planning schema permitted command checks outside the Contract ceiling")
	}
	intents := unit["intent"].(map[string]any)["enum"].([]any)
	if len(intents) != 2 || intents[0] != "inspect" || intents[1] != "modify" {
		t.Fatalf("interactive planning intents = %#v", intents)
	}
}

func TestDiscussPlanPropagatesFrozenRepositoryToolGrant(t *testing.T) {
	client := &captureLLMClient{result: `{"status":"needs_context","message":"One choice remains.","proposal":null,"issues":[{"kind":"fact_missing","prompt":"Which layer?","reason":"It changes the evidence.","recommendation":"Trace the runtime layer.","choices":[{"key":"runtime","label":"Trace the runtime layer"},{"key":"ui","label":"Trace the UI layer"}],"workUnitKeys":[],"criterionAliases":[]}]}`}
	root := t.TempDir()
	_, err := NewLLMProvider(client).DiscussPlan(context.Background(), ports.PlanningDiscussionRequest{
		Contract:          domain.ContractRevision{Number: 1, Goal: "Assess architecture", AuthorityCeiling: domain.ProposedAuthority{ReadWorkspace: true}},
		RepositoryContext: ports.RepositoryContextSnapshot{Root: root},
		RepositoryToolUse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.request.ContextAccess.Mode != ports.ReasoningContextRepositoryRead || client.request.ContextAccess.Root != root {
		t.Fatalf("planning context access = %+v", client.request.ContextAccess)
	}
}
