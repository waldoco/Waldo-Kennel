package outcome_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apierr"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/intelligence/intelligencetest"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

// needsContextPlanner answers the sessionless one-shot DraftPlan call with a
// needs_context envelope: legal from a planner, unservable on a lane with no
// session to answer the questions (run 35306197815's "W1" envelope was
// exactly this shape).
type needsContextPlanner struct{ intelligencetest.Provider }

func (needsContextPlanner) DraftPlan(_ context.Context, _ ports.PlanIntelligenceRequest) (ports.PlanIntelligenceResponse, error) {
	return ports.PlanIntelligenceResponse{
		Readiness: domain.NewPlanningReadinessResult("One answer needed.", nil, []domain.PlanningReadinessIssue{{
			Key:     "release-channel",
			Kind:    domain.ReadinessFactMissing,
			Route:   domain.RouteAnswerContext,
			Source:  domain.ReadinessSourcePlannerDeclared,
			Prompt:  "Which release channel should the build target?",
			Reason:  "The verification graph depends on it.",
			Choices: []domain.PlanningReadinessChoice{{Key: "stable", Label: "Stable"}, {Key: "beta", Label: "Beta"}},
		}}),
		Provenance: ports.IntelligenceProvenance{EffectiveProvider: intelligencetest.ProviderID, EffectiveModel: "fixed"},
	}, nil
}

// TestProposePlanOneShotRejectsNeedsContext pins the one-shot lane contract:
// planner questions are a dead end without a session, so a needs_context
// reply fails loudly (questions visible, interactive planning named) and
// writes no PlanRevision. The prompt instructs the planner to fold
// uncertainties into assumptions/blockers instead.
func TestProposePlanOneShotRejectsNeedsContext(t *testing.T) {
	project := domain.ProjectRecord{ID: "nc-project", Path: "/tmp/nc-project"}
	base := newPlanFakeStore()
	store := &configuredPlanStore{planFakeStore: base, project: project}
	store.spaces[domain.ProjectID(project.ID)] = domain.ResponsibilitySpace{
		ID:        "rsp-nc",
		Kind:      domain.ResponsibilitySpaceWorkProject,
		ProjectID: domain.ProjectID(project.ID),
	}
	candidates := make([]domain.RoutingCandidate, 0, len(domain.AllHarnesses))
	for _, harness := range domain.AllHarnesses {
		candidates = append(candidates, executionCandidate(harness, ""))
	}
	svc := outcome.New(store, nil).
		WithPlanning(needsContextPlanner{}, &routingInventoryFake{candidates: candidates})
	svc.AdmissionPolicy = testAdmissionPolicy()
	seed, err := svc.Create(context.Background(), outcome.CreateInput{
		ProjectID:        domain.ProjectID(project.ID),
		Title:            "Needs context",
		Goal:             "Build the thing.",
		SuccessCriteria:  []string{"the thing exists"},
		Review:           "inspect",
		AuthorityCeiling: domain.ProposedAuthority{ReadWorkspace: true, WriteWorkspace: true, ExecuteLocal: true},
		RequestKey:       "req-nc-1",
	})
	if err != nil {
		t.Fatalf("seed outcome: %v", err)
	}

	_, err = svc.ProposePlan(context.Background(), seed.Outcome.ID, 1)
	if code := apiCode(t, err); code != "PLAN_NEEDS_INTERACTIVE_PLANNING" {
		t.Fatalf("code = %s, want PLAN_NEEDS_INTERACTIVE_PLANNING (err %v)", code, err)
	}
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err type %T", err)
	}
	questions, ok := apiErr.Details["questions"].([]string)
	if !ok || len(questions) != 1 || !strings.Contains(questions[0], "release channel") {
		t.Fatalf("the planner's question must reach the owner in the error detail: %+v", apiErr.Details)
	}
	store.mu.Lock()
	persisted := len(store.plans[seed.Outcome.ID])
	store.mu.Unlock()
	if persisted != 0 {
		t.Fatalf("a needs_context one-shot reply must not persist a plan, got %d", persisted)
	}
}
