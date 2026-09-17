package outcome_test

import (
	"context"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/intelligence/intelligencetest"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

// GetProject gives the shared plan/attempt fake an EXPLICIT worker. Keeping
// Codex here preserves older broad fixtures while the dedicated tests below
// prove non-Codex identities bind without any runtime fallback.
func (f *planFakeStore) GetProject(_ context.Context, id string) (domain.ProjectRecord, bool, error) {
	if id != "mer" {
		return domain.ProjectRecord{}, false, nil
	}
	return domain.ProjectRecord{
		ID:   "mer",
		Path: "/tmp/mer",
		Config: domain.ProjectConfig{
			Worker: domain.RoleOverride{Harness: domain.HarnessCodex},
		},
	}, true, nil
}

type configuredPlanStore struct {
	*planFakeStore
	project domain.ProjectRecord
}

func (f *configuredPlanStore) GetProject(_ context.Context, id string) (domain.ProjectRecord, bool, error) {
	if f.project.ID != id {
		return domain.ProjectRecord{}, false, nil
	}
	return f.project, true, nil
}

func seedPlanServiceWithProject(t *testing.T, project domain.ProjectRecord) (*outcome.Service, *configuredPlanStore, domain.OutcomeID) {
	t.Helper()
	base := newPlanFakeStore()
	store := &configuredPlanStore{planFakeStore: base, project: project}
	store.spaces[domain.ProjectID(project.ID)] = domain.ResponsibilitySpace{
		ID:        "rsp-provider-plan",
		Kind:      domain.ResponsibilitySpaceWorkProject,
		ProjectID: domain.ProjectID(project.ID),
	}
	// Every shipped harness is admissible here, so what the router picks is
	// decided by the Project preference under test rather than by which
	// candidate the fixture happened to offer.
	candidates := make([]domain.RoutingCandidate, 0, len(domain.AllHarnesses))
	for _, harness := range domain.AllHarnesses {
		candidates = append(candidates, executionCandidate(harness, ""))
	}
	svc := outcome.New(store, nil).
		WithPlanning(intelligencetest.New(), &routingInventoryFake{candidates: candidates})
	svc.AdmissionPolicy = testAdmissionPolicy()
	view, err := svc.Create(context.Background(), outcome.CreateInput{
		ProjectID:        domain.ProjectID(project.ID),
		Title:            "Provider-bound work",
		Goal:             "Run the authorized provider only.",
		SuccessCriteria:  []string{"the provider is frozen into the WorkUnit"},
		Review:           "deterministic tests",
		AuthorityCeiling: domain.ProposedAuthority{ReadWorkspace: true, WriteWorkspace: true, ExecuteLocal: true},
		RequestKey:       "req-provider-plan-" + project.ID,
	})
	if err != nil {
		t.Fatalf("seed outcome: %v", err)
	}
	return svc, store, view.Outcome.ID
}

func TestProposePlanBindsExplicitNonCodexProjectWorker(t *testing.T) {
	project := domain.ProjectRecord{
		ID:   "claude-project",
		Path: "/tmp/claude-project",
		Config: domain.ProjectConfig{
			Worker: domain.RoleOverride{Harness: domain.HarnessClaudeCode},
		},
	}
	svc, _, outcomeID := seedPlanServiceWithProject(t, project)
	view, err := svc.ProposePlan(context.Background(), outcomeID, 1)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if got := view.Plan.WorkUnits[0].Provider; got != domain.HarnessClaudeCode {
		t.Fatalf("WorkUnit provider = %q, want explicit Project worker %q", got, domain.HarnessClaudeCode)
	}
}

// With no Project worker configured there is no preference to honor, so the
// deterministic router picks among admissible candidates and records why. That
// is not the AO hidden fallback: nothing defaults to Codex regardless of
// config, the choice is made from machine-verified candidates, and it is frozen
// into the WorkUnit binding at approval like any other.
func TestProposePlanRoutesDeterministicallyWhenProjectWorkerIsUnconfigured(t *testing.T) {
	project := domain.ProjectRecord{ID: "unconfigured", Path: "/tmp/unconfigured"}
	svc, store, outcomeID := seedPlanServiceWithProject(t, project)

	view, err := svc.ProposePlan(context.Background(), outcomeID, 1)
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if view.Plan.WorkUnits[0].Provider == "" {
		t.Fatalf("an unconfigured project must still bind an exact provider: %+v", view.Plan.WorkUnits[0])
	}
	if len(view.Plan.RoutingDecisions) == 0 {
		t.Fatal("a routed plan must record why it chose the provider it chose")
	}
	if pref := view.Plan.RoutingDecisions[0].Decision.EffectivePreference; pref != nil {
		t.Fatalf("no Project worker is configured, so no preference may be reported: %+v", pref)
	}
	store.mu.Lock()
	persisted := len(store.plans[outcomeID])
	store.mu.Unlock()
	if persisted != 1 {
		t.Fatalf("routed proposal persisted %d plans, want 1", persisted)
	}
}

func TestChangingProjectWorkerCreatesFreshProviderBoundPlan(t *testing.T) {
	project := domain.ProjectRecord{
		ID:   "switch-worker",
		Path: "/tmp/switch-worker",
		Config: domain.ProjectConfig{
			Worker: domain.RoleOverride{Harness: domain.HarnessClaudeCode},
		},
	}
	svc, store, outcomeID := seedPlanServiceWithProject(t, project)
	first, err := svc.ProposePlan(context.Background(), outcomeID, 1)
	if err != nil {
		t.Fatalf("first proposal: %v", err)
	}
	store.project.Config.Worker.Harness = domain.HarnessPi
	second, err := svc.ProposePlan(context.Background(), outcomeID, 1)
	if err != nil {
		t.Fatalf("second proposal: %v", err)
	}
	if second.Plan.ID == first.Plan.ID {
		t.Fatal("changing the Project worker must create a fresh authorization artifact")
	}
	if second.Plan.WorkUnits[0].Provider != domain.HarnessPi {
		t.Fatalf("second provider = %q, want %q", second.Plan.WorkUnits[0].Provider, domain.HarnessPi)
	}
	if second.Plan.RunBriefCoreDigest == first.Plan.RunBriefCoreDigest {
		t.Fatal("changing the provider must change the RunBrief core digest")
	}
}
