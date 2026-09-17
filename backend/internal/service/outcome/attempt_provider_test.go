package outcome_test

import (
	"context"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/intelligence/intelligencetest"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

func executionCandidate(provider domain.AgentHarness, model string) domain.RoutingCandidate {
	models := map[string]domain.CapabilitySupport{}
	if model != "" {
		models[model] = domain.CapabilitySupported
	}
	return domain.RoutingCandidate{
		ID: string(provider), Provider: string(provider), ModelSelection: domain.ExecutionBindingModelProviderDefault,
		WorkerEligible: true, CoordinatorEligible: provider.IsSelectableAsCoordinator(), Readiness: domain.CapabilitySupported,
		Capabilities: map[string]domain.CapabilitySupport{
			domain.CapabilityWorktreeRead:  domain.CapabilitySupported,
			domain.CapabilityWorktreeWrite: domain.CapabilitySupported,
			domain.CapabilityWorktreeExec:  domain.CapabilitySupported,
		},
		Models: models,
	}
}

func newConfiguredAttemptHarness(t *testing.T, provider domain.AgentHarness, model string) (*outcome.Service, *planningFakeStore, *fakeSpawner, domain.OutcomeID, domain.PlanRevisionID) {
	t.Helper()
	store := newPlanningFakeStore()
	store.project.Config.Worker = domain.RoleOverride{Harness: provider, AgentConfig: domain.AgentConfig{Model: model}}
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{executionCandidate(provider, model)}}
	spawner := &fakeSpawner{readiness: ports.AgentProfileReadiness{Ready: true, Detail: "ready"}}
	svc := outcome.New(store, nil).
		WithPlanning(intelligencetest.New(), router).
		WithExecution(spawner, newFakeHeartbeats())
	svc.AdmissionPolicy = testAdmissionPolicy()

	view, err := svc.Create(context.Background(), outcome.CreateInput{
		ProjectID: "mer", Title: "Provider admission", Goal: "Execute only the authorized provider and model.",
		SuccessCriteria: []string{"no provider or model substitution occurs"}, Review: "deterministic tests",
		AuthorityCeiling: domain.ProposedAuthority{ReadWorkspace: true, WriteWorkspace: true, ExecuteLocal: true},
		RequestKey:       "req-provider-attempt-create",
	})
	if err != nil {
		t.Fatalf("create outcome: %v", err)
	}
	planView, err := svc.ProposePlan(context.Background(), view.Outcome.ID, 1)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if _, err := svc.ApprovePlan(context.Background(), view.Outcome.ID, outcome.ApprovePlanInput{
		PlanRevisionID: planView.Plan.ID, ExpectedContractRevision: 1,
	}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	return svc, store, spawner, view.Outcome.ID, planView.Plan.ID
}

func TestStartAttemptUsesFrozenProviderAndModelAfterProjectPreferenceChanges(t *testing.T) {
	svc, store, spawner, outcomeID, planID := newConfiguredAttemptHarness(t, domain.HarnessClaudeCode, "sonnet-test")
	readsBeforeStart := store.projectReads
	store.project.Config.Worker = domain.RoleOverride{Harness: domain.HarnessCodex, AgentConfig: domain.AgentConfig{Model: "codex-mutated"}}

	if _, err := svc.StartAttempt(context.Background(), outcomeID, outcome.StartAttemptInput{
		PlanRevisionID: planID, WorkUnitID: firstWorkUnitOfPlan[planID], RequestKey: "req-provider-attempt-start",
	}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if store.projectReads != readsBeforeStart+1 {
		t.Fatalf("Attempt project-kind reads = %d -> %d, want one custody-kind read", readsBeforeStart, store.projectReads)
	}
	spawner.mu.Lock()
	defer spawner.mu.Unlock()
	if len(spawner.spawned) != 1 {
		t.Fatalf("spawn calls = %d, want 1", len(spawner.spawned))
	}
	req := spawner.spawned[0]
	if req.Harness != domain.HarnessClaudeCode {
		t.Fatalf("provider = %q", req.Harness)
	}
	if req.ModelSelection != domain.ExecutionBindingModelExplicit || req.Model != "sonnet-test" {
		t.Fatalf("model binding = %s/%q, want explicit/sonnet-test", req.ModelSelection, req.Model)
	}
}

func TestStartAttemptRejectsProviderMismatchBeforePersistenceOrSpawn(t *testing.T) {
	svc, store, spawner, outcomeID, planID := newConfiguredAttemptHarness(t, domain.HarnessClaudeCode, "sonnet-test")
	_, err := svc.StartAttempt(context.Background(), outcomeID, outcome.StartAttemptInput{
		PlanRevisionID: planID, WorkUnitID: firstWorkUnitOfPlan[planID], Harness: domain.HarnessCodex, RequestKey: "req-provider-mismatch",
	})
	if code := requireAPICode(t, err); code != outcome.CodeAttemptProviderMismatch {
		t.Fatalf("code = %s", code)
	}
	attempts, listErr := store.ListAttempts(context.Background(), outcomeID)
	if listErr != nil {
		t.Fatalf("list attempts: %v", listErr)
	}
	if len(attempts) != 0 {
		t.Fatalf("mismatch persisted %d attempts", len(attempts))
	}
	spawner.mu.Lock()
	spawned := len(spawner.spawned)
	spawner.mu.Unlock()
	if spawned != 0 {
		t.Fatalf("mismatch spawned %d sessions", spawned)
	}
}

func TestStartAttemptRejectsLegacyUnboundPlanBeforePersistenceOrSpawn(t *testing.T) {
	svc, store, spawner, outcomeID, planID := newConfiguredAttemptHarness(t, domain.HarnessClaudeCode, "sonnet-test")
	store.planFakeStore.mu.Lock()
	for i := range store.plans[outcomeID] {
		if store.plans[outcomeID][i].ID == planID {
			store.plans[outcomeID][i].WorkUnits[0].Provider = ""
			store.plans[outcomeID][i].WorkUnits[0].ModelSelection = ""
			store.plans[outcomeID][i].WorkUnits[0].Model = ""
		}
	}
	units := store.units[planID]
	if len(units) == 1 {
		units[0].Provider, units[0].ModelSelection, units[0].Model = "", "", ""
		store.units[planID] = units
	}
	store.planFakeStore.mu.Unlock()

	_, err := svc.StartAttempt(context.Background(), outcomeID, outcome.StartAttemptInput{PlanRevisionID: planID, WorkUnitID: firstWorkUnitOfPlan[planID], RequestKey: "req-provider-unbound"})
	// Stripping the binding also changes the frozen RunBrief digest, so the
	// brief check refuses first. Either code is a correct refusal, and the
	// property under test is the one asserted below: nothing was persisted and
	// no provider session was started.
	if code := requireAPICode(t, err); code != outcome.CodePlanProviderUnbound && code != outcome.CodePlanBriefInvalidated {
		t.Fatalf("code = %s, want an unbound-plan or invalidated-brief refusal", code)
	}
	attempts, listErr := store.ListAttempts(context.Background(), outcomeID)
	if listErr != nil {
		t.Fatalf("list attempts: %v", listErr)
	}
	if len(attempts) != 0 {
		t.Fatalf("unbound plan persisted %d attempts", len(attempts))
	}
	spawner.mu.Lock()
	spawned := len(spawner.spawned)
	spawner.mu.Unlock()
	if spawned != 0 {
		t.Fatalf("unbound plan spawned %d sessions", spawned)
	}
}
