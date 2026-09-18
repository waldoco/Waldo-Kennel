package outcome_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apierr"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

// ---- fixtures ----

func readinessFence() domain.PlanningReadinessFence {
	return domain.PlanningReadinessFence{
		PlanningSessionID: "ps-1", SessionRevision: 1,
		ContractRevisionID: "crev-1", ContextDigest: "digest-1",
	}
}

func readinessContract(authority domain.ProposedAuthority, criteria ...domain.ContractCriterion) domain.ContractRevision {
	return domain.ContractRevision{
		ID: "crev-1", OutcomeID: "out-1", Number: 1, Goal: "Ship the bounded change.",
		Criteria: criteria, SuccessCriteria: []string{"criterion"}, Review: "review",
		AuthorityCeiling: authority,
	}
}

func readinessCriterion(position int64) domain.ContractCriterion {
	return domain.ContractCriterion{ID: domain.CriterionID(fmt.Sprintf("crit-%d", position)), ContractRevisionID: "crev-1", Position: position, Text: fmt.Sprintf("criterion %d", position)}
}

func readinessUnit(key string, intent domain.WorkUnitIntent, aliases ...string) domain.PlanDraftWorkUnit {
	return domain.PlanDraftWorkUnit{
		Key: key, Title: "Unit " + key, Intent: intent, Role: domain.WorkUnitRoleImplement,
		OutputSummary: key + " summary", CriteriaCovered: aliases,
	}
}

func readinessService(router *routingInventoryFake) *outcome.Service {
	return outcome.New(newPlanningFakeStore(), nil).WithPlanning(nil, router)
}

// ---- happy path ----

func TestEvaluatePlanReadiness_ReadyCarriesProposalAndBindsSnapshot(t *testing.T) {
	router := &routingInventoryFake{snapshotIDs: []string{"snap-ready"}, candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}
	svc := readinessService(router)
	draft := domain.PlanDraftProposal{Summary: "Implement.", WorkUnits: []domain.PlanDraftWorkUnit{
		readinessUnit("implement", domain.WorkUnitIntentExecute, "C1"),
	}}
	result, snapshot, err := svc.EvaluatePlanReadiness(context.Background(), readinessFence(), "p1",
		readinessContract(fullLocalAuthority(), readinessCriterion(1)), nil, draft, nil, "ready")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if result.Status != domain.PlanningReady || result.Proposal == nil {
		t.Fatalf("status = %q, proposal nil = %v; want ready with proposal", result.Status, result.Proposal == nil)
	}
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %v, want none", result.Issues)
	}
	if snapshot.SnapshotID != "snap-ready" {
		t.Fatalf("snapshot = %q, want snap-ready", snapshot.SnapshotID)
	}
	if router.calls != 1 {
		t.Fatalf("routing snapshot calls = %d, want exactly 1 per evaluation", router.calls)
	}
}

// ---- step 5: structural invalidity is provider error, never a typed issue ----

func TestEvaluatePlanReadiness_InvalidDraftIsProviderErrorNotIssue(t *testing.T) {
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}
	svc := readinessService(router)
	for name, mutate := range map[string]func(*domain.PlanDraftProposal){
		"unknown dependency": func(p *domain.PlanDraftProposal) { p.WorkUnits[0].DependsOn = []string{"ghost"} },
		"unsupported intent": func(p *domain.PlanDraftProposal) { p.WorkUnits[0].Intent = domain.WorkUnitIntent("teleport") },
		"blank title":        func(p *domain.PlanDraftProposal) { p.WorkUnits[0].Title = " " },
	} {
		t.Run(name, func(t *testing.T) {
			draft := domain.PlanDraftProposal{Summary: "x", WorkUnits: []domain.PlanDraftWorkUnit{readinessUnit("u", domain.WorkUnitIntentModify, "C1")}}
			mutate(&draft)
			result, _, err := svc.EvaluatePlanReadiness(context.Background(), readinessFence(), "p1",
				readinessContract(fullLocalAuthority(), readinessCriterion(1)), nil, draft, nil, "m")
			if err == nil {
				t.Fatalf("expected provider error, got result %+v", result)
			}
			var apiErr *apierr.Error
			if !errors.As(err, &apiErr) || apiErr.Kind != apierr.KindInvalid {
				t.Fatalf("err = %v, want 400 invalid provider output", err)
			}
			if router.calls != 0 {
				t.Fatalf("routing snapshot was read (%d calls) before the draft was validated", router.calls)
			}
		})
	}
	t.Run("unknown unit criterion alias", func(t *testing.T) {
		draft := domain.PlanDraftProposal{Summary: "x", WorkUnits: []domain.PlanDraftWorkUnit{readinessUnit("u", domain.WorkUnitIntentModify, "C9")}}
		_, _, err := svc.EvaluatePlanReadiness(context.Background(), readinessFence(), "p1",
			readinessContract(fullLocalAuthority(), readinessCriterion(1)), nil, draft, nil, "m")
		var apiErr *apierr.Error
		if !errors.As(err, &apiErr) || apiErr.Code != "PLAN_DRAFT_CRITERION_UNKNOWN" {
			t.Fatalf("err = %v, want PLAN_DRAFT_CRITERION_UNKNOWN", err)
		}
	})
}

// ---- step 6: intent-derived capability above the confirmed ceiling ----

func TestEvaluatePlanReadiness_AboveCeilingCapabilityIsAuthorityIssue(t *testing.T) {
	for name, tc := range map[string]struct {
		intent     domain.WorkUnitIntent
		ceiling    domain.ProposedAuthority
		capability string
	}{
		"write with read-only ceiling":   {domain.WorkUnitIntentModify, domain.ProposedAuthority{ReadWorkspace: true}, domain.CapabilityWorktreeWrite},
		"exec without execute authority": {domain.WorkUnitIntentExecute, domain.ProposedAuthority{ReadWorkspace: true, WriteWorkspace: true}, domain.CapabilityWorktreeExec},
	} {
		t.Run(name, func(t *testing.T) {
			svc := readinessService(&routingInventoryFake{snapshotIDs: []string{"snap-1"}, candidates: []domain.RoutingCandidate{readyClaudeCandidate()}})
			draft := domain.PlanDraftProposal{Summary: "x", WorkUnits: []domain.PlanDraftWorkUnit{readinessUnit("u", tc.intent, "C1")}}
			result, _, err := svc.EvaluatePlanReadiness(context.Background(), readinessFence(), "p1",
				readinessContract(tc.ceiling, readinessCriterion(1)), nil, draft, nil, "m")
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if result.Status != domain.PlanningBlocked || result.Proposal != nil {
				t.Fatalf("status = %q, proposal nil = %v; want blocked without proposal", result.Status, result.Proposal == nil)
			}
			if len(result.Issues) != 1 {
				t.Fatalf("issues = %d, want 1", len(result.Issues))
			}
			issue := result.Issues[0]
			if issue.Kind != domain.ReadinessAuthorityInsufficient || issue.Route != domain.RouteReviseContract || issue.Source != domain.ReadinessSourceControlPlane {
				t.Fatalf("issue = %+v", issue)
			}
			if issue.RequestedCapability != tc.capability || len(issue.WorkUnitKeys) != 1 || issue.WorkUnitKeys[0] != "u" {
				t.Fatalf("issue capability/keys = %q %v, want %q [u]", issue.RequestedCapability, issue.WorkUnitKeys, tc.capability)
			}
			// The key is minted under the snapshot-bound fence, never supplied:
			// it must equal the canonical key under the bound fence and differ
			// from the key under the unbound one.
			bound := readinessFence()
			bound.RoutingSnapshotID = "snap-1"
			unkeyed := issue
			unkeyed.Key = ""
			if issue.Key != domain.CanonicalPlanningIssueKey(bound, unkeyed) {
				t.Fatalf("issue key %q is not canonical under the bound fence", issue.Key)
			}
			if issue.Key == domain.CanonicalPlanningIssueKey(readinessFence(), unkeyed) {
				t.Fatalf("issue key did not bind the routing snapshot")
			}
		})
	}
}

// ---- step 7: check dry-run ----

func TestEvaluatePlanReadiness_CheckDryRun(t *testing.T) {
	newSvc := func() *outcome.Service {
		return readinessService(&routingInventoryFake{snapshotIDs: []string{"snap-1"}, candidates: []domain.RoutingCandidate{readyClaudeCandidate()}})
	}
	contract := readinessContract(fullLocalAuthority(), readinessCriterion(1), readinessCriterion(2))
	for name, tc := range map[string]struct {
		mutate   func(*domain.PlanDraftWorkUnit)
		wantFrag string
	}{
		"check names criterion the unit does not own": {
			func(u *domain.PlanDraftWorkUnit) {
				u.CheckCommands = []domain.PlanDraftCheck{{CriterionAlias: "C2", Argv: []string{"go", "test"}}}
			}, "does not own"},
		"check names unknown criterion alias": {
			func(u *domain.PlanDraftWorkUnit) {
				u.CheckCommands = []domain.PlanDraftCheck{{CriterionAlias: "C9", Argv: []string{"go", "test"}}}
			}, "does not exist"},
		"shell-form command is not approved authority": {
			func(u *domain.PlanDraftWorkUnit) {
				u.CheckCommands = []domain.PlanDraftCheck{{CriterionAlias: "C1", Argv: []string{"sh", "-c", "go test && go vet"}}}
			}, "may not run a shell"},
		"checks without execution intent can never run": {
			func(u *domain.PlanDraftWorkUnit) {
				u.Intent = domain.WorkUnitIntentModify
				u.CheckCommands = []domain.PlanDraftCheck{{CriterionAlias: "C1", Argv: []string{"go", "test"}}}
			}, "no execution intent"},
	} {
		t.Run(name, func(t *testing.T) {
			unit := readinessUnit("u", domain.WorkUnitIntentExecute, "C1")
			tc.mutate(&unit)
			draft := domain.PlanDraftProposal{Summary: "x", WorkUnits: []domain.PlanDraftWorkUnit{unit}}
			result, _, err := newSvc().EvaluatePlanReadiness(context.Background(), readinessFence(), "p1", contract, nil, draft, nil, "m")
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if result.Status != domain.PlanningBlocked {
				t.Fatalf("status = %q, want blocked", result.Status)
			}
			if len(result.Issues) != 1 {
				t.Fatalf("issues = %d, want 1: %+v", len(result.Issues), result.Issues)
			}
			issue := result.Issues[0]
			if issue.Kind != domain.ReadinessCheckUnrepresentable || issue.Route != domain.RouteRevisePlan || issue.Source != domain.ReadinessSourceControlPlane {
				t.Fatalf("issue = %+v", issue)
			}
			if !strings.Contains(issue.Prompt, tc.wantFrag) {
				t.Fatalf("prompt %q missing %q", issue.Prompt, tc.wantFrag)
			}
		})
	}
	t.Run("timeout above cap is bounded not blocked", func(t *testing.T) {
		unit := readinessUnit("u", domain.WorkUnitIntentExecute, "C1")
		unit.CheckCommands = []domain.PlanDraftCheck{{CriterionAlias: "C1", Argv: []string{"go", "test"}, TimeoutSeconds: 99999}}
		draft := domain.PlanDraftProposal{Summary: "x", WorkUnits: []domain.PlanDraftWorkUnit{unit}}
		result, _, err := newSvc().EvaluatePlanReadiness(context.Background(), readinessFence(), "p1", contract, nil, draft, nil, "m")
		if err != nil || result.Status != domain.PlanningReady {
			t.Fatalf("err = %v status = %q, want ready (timeout bounded)", err, result.Status)
		}
	})
}

// ---- step 8: provisional unit routing ----

func TestEvaluatePlanReadiness_WorkerUnavailableRouting(t *testing.T) {
	contract := readinessContract(fullLocalAuthority(), readinessCriterion(1))
	draft := domain.PlanDraftProposal{Summary: "x", WorkUnits: []domain.PlanDraftWorkUnit{readinessUnit("u", domain.WorkUnitIntentExecute, "C1")}}

	t.Run("no capable candidate routes to choose_harness", func(t *testing.T) {
		weak := readyClaudeCandidate()
		weak.Capabilities[domain.CapabilityWorktreeExec] = domain.CapabilityUnsupported
		svc := readinessService(&routingInventoryFake{candidates: []domain.RoutingCandidate{weak}})
		result, _, err := svc.EvaluatePlanReadiness(context.Background(), readinessFence(), "p1", contract, nil, draft, nil, "m")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if result.Status != domain.PlanningBlocked || len(result.Issues) != 1 {
			t.Fatalf("status = %q issues = %v", result.Status, result.Issues)
		}
		issue := result.Issues[0]
		if issue.Kind != domain.ReadinessWorkerUnavailable || issue.Route != domain.RouteChooseHarness {
			t.Fatalf("issue = %+v", issue)
		}
		if !strings.Contains(issue.Reason, "CAPABILITY_UNSUPPORTED") {
			t.Fatalf("reason %q should name the deterministic rejection codes", issue.Reason)
		}
	})

	t.Run("capable candidate blocked only by readiness routes to authenticate_harness", func(t *testing.T) {
		unauthed := readyClaudeCandidate()
		unauthed.Readiness = domain.CapabilityUnsupported
		svc := readinessService(&routingInventoryFake{candidates: []domain.RoutingCandidate{unauthed}})
		result, _, err := svc.EvaluatePlanReadiness(context.Background(), readinessFence(), "p1", contract, nil, draft, nil, "m")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(result.Issues) != 1 || result.Issues[0].Route != domain.RouteAuthenticateHarness {
			t.Fatalf("issues = %+v, want one authenticate_harness issue", result.Issues)
		}
	})

	t.Run("ready candidate passes", func(t *testing.T) {
		svc := readinessService(&routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}})
		result, _, err := svc.EvaluatePlanReadiness(context.Background(), readinessFence(), "p1", contract, nil, draft, nil, "m")
		if err != nil || result.Status != domain.PlanningReady {
			t.Fatalf("err = %v status = %q, want ready", err, result.Status)
		}
	})
}

func TestEvaluatePlanReadiness_InventoryErrorIsUnavailableNotIssue(t *testing.T) {
	svc := readinessService(&routingInventoryFake{err: errors.New("inventory down")})
	draft := domain.PlanDraftProposal{Summary: "x", WorkUnits: []domain.PlanDraftWorkUnit{readinessUnit("u", domain.WorkUnitIntentModify, "C1")}}
	result, _, err := svc.EvaluatePlanReadiness(context.Background(), readinessFence(), "p1",
		readinessContract(fullLocalAuthority(), readinessCriterion(1)), nil, draft, nil, "m")
	if err == nil || err.Error() != "inventory down" {
		t.Fatalf("err = %v, want the raw inventory failure", err)
	}
	if result.Status != "" || len(result.Issues) != 0 {
		t.Fatalf("inventory failure must not fabricate a readiness packet: %+v", result)
	}
}

// ---- step 9: planner-declared blockers and issues ----

func TestEvaluatePlanReadiness_PlannerBlockersBecomeContextIssues(t *testing.T) {
	svc := readinessService(&routingInventoryFake{snapshotIDs: []string{"snap-1"}, candidates: []domain.RoutingCandidate{readyClaudeCandidate()}})
	draft := domain.PlanDraftProposal{Summary: "x", Blockers: []string{"Which service owns retry policy?"}, WorkUnits: []domain.PlanDraftWorkUnit{
		readinessUnit("u", domain.WorkUnitIntentModify, "C1"),
	}}
	result, _, err := svc.EvaluatePlanReadiness(context.Background(), readinessFence(), "p1",
		readinessContract(fullLocalAuthority(), readinessCriterion(1)), nil, draft, nil, "m")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// All issues are owner-answerable, so the status derives to needs_context.
	if result.Status != domain.PlanningNeedsContext || result.Proposal != nil {
		t.Fatalf("status = %q proposal nil = %v", result.Status, result.Proposal == nil)
	}
	if len(result.Issues) != 1 {
		t.Fatalf("issues = %d, want 1", len(result.Issues))
	}
	issue := result.Issues[0]
	if issue.Kind != domain.ReadinessContextInsufficient || issue.Route != domain.RouteAnswerContext || issue.Source != domain.ReadinessSourcePlannerDeclared {
		t.Fatalf("issue = %+v", issue)
	}
	if issue.Prompt != "Which service owns retry policy?" {
		t.Fatalf("prompt = %q", issue.Prompt)
	}
}

func TestEvaluatePlanReadiness_PlannerIssuesRekeyedUnderSnapshotBoundFence(t *testing.T) {
	svc := readinessService(&routingInventoryFake{snapshotIDs: []string{"snap-9"}, candidates: []domain.RoutingCandidate{readyClaudeCandidate()}})
	planner := domain.PlanningReadinessIssue{
		Kind: domain.ReadinessContextInsufficient, Route: domain.RouteAnswerContext, Source: domain.ReadinessSourcePlannerDeclared,
		Prompt: "Should the first slice stay local only?", Reason: "Remote effects need separate authority.",
		Recommendation: "Keep it local.",
	}
	draft := domain.PlanDraftProposal{Summary: "x", WorkUnits: []domain.PlanDraftWorkUnit{readinessUnit("u", domain.WorkUnitIntentModify, "C1")}}
	result, _, err := svc.EvaluatePlanReadiness(context.Background(), readinessFence(), "p1",
		readinessContract(fullLocalAuthority(), readinessCriterion(1)), nil, draft, []domain.PlanningReadinessIssue{planner}, "m")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(result.Issues) != 1 {
		t.Fatalf("issues = %d, want 1", len(result.Issues))
	}
	fence := readinessFence()
	fence.RoutingSnapshotID = "snap-9"
	if want := domain.CanonicalPlanningIssueKey(fence, planner); result.Issues[0].Key != want {
		t.Fatalf("planner issue key = %q, want re-keyed %q", result.Issues[0].Key, want)
	}
	// And an unparseable planner issue is invalid provider output, not a packet entry.
	planner.Source = "mystery"
	_, _, err = svc.EvaluatePlanReadiness(context.Background(), readinessFence(), "p1",
		readinessContract(fullLocalAuthority(), readinessCriterion(1)), nil, draft, []domain.PlanningReadinessIssue{planner}, "m")
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) || apiErr.Kind != apierr.KindInvalid {
		t.Fatalf("err = %v, want 400 invalid planner issue", err)
	}
}

// ---- fence integrity: stale planner references and blank inventory identity ----

func TestEvaluatePlanReadiness_StalePlannerIssueRejected(t *testing.T) {
	draft := domain.PlanDraftProposal{Summary: "x", WorkUnits: []domain.PlanDraftWorkUnit{readinessUnit("u", domain.WorkUnitIntentModify, "C1")}}
	contract := readinessContract(fullLocalAuthority(), readinessCriterion(1))
	base := domain.PlanningReadinessIssue{
		Kind: domain.ReadinessContextInsufficient, Route: domain.RouteAnswerContext, Source: domain.ReadinessSourcePlannerDeclared,
		Prompt: "Which service owns retry policy?", Reason: "The planner could not tell.", Recommendation: "Answer it.",
	}
	for name, mutate := range map[string]func(*domain.PlanningReadinessIssue){
		"work unit from another draft":    func(i *domain.PlanningReadinessIssue) { i.WorkUnitKeys = []string{"ghost"} },
		"criterion from another contract": func(i *domain.PlanningReadinessIssue) { i.CriterionAliases = []string{"C9"} },
	} {
		t.Run(name, func(t *testing.T) {
			svc := readinessService(&routingInventoryFake{snapshotIDs: []string{"snap-1"}, candidates: []domain.RoutingCandidate{readyClaudeCandidate()}})
			issue := base
			mutate(&issue)
			result, _, err := svc.EvaluatePlanReadiness(context.Background(), readinessFence(), "p1", contract, nil, draft, []domain.PlanningReadinessIssue{issue}, "m")
			var apiErr *apierr.Error
			if !errors.As(err, &apiErr) || apiErr.Code != "PLANNING_READINESS_ISSUE_STALE" {
				t.Fatalf("err = %v, want PLANNING_READINESS_ISSUE_STALE", err)
			}
			if result.Status != "" || len(result.Issues) != 0 {
				t.Fatalf("stale planner issue must not produce a packet: %+v", result)
			}
		})
	}

	// A planner issue whose references all exist in the current draft still passes.
	svc := readinessService(&routingInventoryFake{snapshotIDs: []string{"snap-1"}, candidates: []domain.RoutingCandidate{readyClaudeCandidate()}})
	issue := base
	issue.WorkUnitKeys = []string{"u"}
	issue.CriterionAliases = []string{"C1"}
	result, _, err := svc.EvaluatePlanReadiness(context.Background(), readinessFence(), "p1", contract, nil, draft, []domain.PlanningReadinessIssue{issue}, "m")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(result.Issues) != 1 || result.Issues[0].Prompt != base.Prompt {
		t.Fatalf("current-referencing planner issue lost: %+v", result.Issues)
	}
}

func TestEvaluatePlanReadiness_BlankSnapshotIdentityRejected(t *testing.T) {
	draft := domain.PlanDraftProposal{Summary: "x", WorkUnits: []domain.PlanDraftWorkUnit{readinessUnit("u", domain.WorkUnitIntentModify, "C1")}}
	contract := readinessContract(fullLocalAuthority(), readinessCriterion(1))
	for name, router := range map[string]*routingInventoryFake{
		"blank snapshot id":   {snapshotIDs: []string{""}, candidates: []domain.RoutingCandidate{readyClaudeCandidate()}},
		"blank generation id": {generationIDs: []string{""}, snapshotIDs: []string{"snap-1"}, candidates: []domain.RoutingCandidate{readyClaudeCandidate()}},
	} {
		t.Run(name, func(t *testing.T) {
			svc := readinessService(router)
			result, _, err := svc.EvaluatePlanReadiness(context.Background(), readinessFence(), "p1", contract, nil, draft, nil, "m")
			var apiErr *apierr.Error
			if !errors.As(err, &apiErr) || apiErr.Code != "ROUTING_INVENTORY_MALFORMED" {
				t.Fatalf("err = %v, want ROUTING_INVENTORY_MALFORMED", err)
			}
			if result.Status != "" || len(result.Issues) != 0 {
				t.Fatalf("malformed inventory must not produce a packet with unfenced keys: %+v", result)
			}
		})
	}
}

// ---- steps 10-11: one packet, deduplicated, deterministic ----

func TestEvaluatePlanReadiness_DeterministicAndDeduplicated(t *testing.T) {
	contract := readinessContract(domain.ProposedAuthority{ReadWorkspace: true}, readinessCriterion(1))
	draft := domain.PlanDraftProposal{
		Summary: "x",
		WorkUnits: []domain.PlanDraftWorkUnit{
			readinessUnit("b", domain.WorkUnitIntentExecute, "C1"),
			readinessUnit("a", domain.WorkUnitIntentModify, "C1"),
		},
	}
	duplicatePlanner := domain.PlanningReadinessIssue{
		Kind: domain.ReadinessContextInsufficient, Route: domain.RouteAnswerContext, Source: domain.ReadinessSourcePlannerDeclared,
		Prompt: "Same blocker.", Reason: "Declared twice.", Recommendation: "Answer it.",
	}
	newSvc := func() *outcome.Service {
		return readinessService(&routingInventoryFake{snapshotIDs: []string{"snap-1"}, candidates: nil})
	}
	first, _, err := newSvc().EvaluatePlanReadiness(context.Background(), readinessFence(), "p1", contract, nil, draft, []domain.PlanningReadinessIssue{duplicatePlanner, duplicatePlanner}, "m")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	second, _, err := newSvc().EvaluatePlanReadiness(context.Background(), readinessFence(), "p1", contract, nil, draft, []domain.PlanningReadinessIssue{duplicatePlanner, duplicatePlanner}, "m")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(first.Issues) != len(second.Issues) {
		t.Fatalf("issue counts differ across runs: %d vs %d", len(first.Issues), len(second.Issues))
	}
	for i := range first.Issues {
		if first.Issues[i].Key != second.Issues[i].Key {
			t.Fatalf("issue %d order unstable: %q vs %q", i, first.Issues[i].Key, second.Issues[i].Key)
		}
	}
	// The duplicated planner issue collapsed to one packet entry.
	dupeCount := 0
	for _, issue := range first.Issues {
		if issue.Prompt == "Same blocker." {
			dupeCount++
		}
	}
	if dupeCount != 1 {
		t.Fatalf("duplicate planner issues in packet = %d, want 1", dupeCount)
	}
	// Issue order follows the draft's topological order (declaration order
	// here), so unit b's authority issue precedes unit a's.
	var authorityOrder []string
	for _, issue := range first.Issues {
		if issue.Kind == domain.ReadinessAuthorityInsufficient {
			authorityOrder = append(authorityOrder, issue.WorkUnitKeys[0])
		}
	}
	if len(authorityOrder) != 2 || authorityOrder[0] != "b" || authorityOrder[1] != "a" {
		t.Fatalf("authority issue order = %v, want topo order [b a]", authorityOrder)
	}
}
