package outcome_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apierr"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
)

type fakeMissionPluginProvisioner struct {
	home  string
	err   error
	calls []domain.ResponsibilitySpaceID
}

func (f *fakeMissionPluginProvisioner) EnsureMissionPlugin(_ context.Context, spaceID domain.ResponsibilitySpaceID) (string, error) {
	f.calls = append(f.calls, spaceID)
	return f.home, f.err
}

// A native-harness planning start must prove the /mission command is a
// verified runtime artifact first: provisioner failure closes mission start
// and creates no planning session.
func TestStartPlanning_NativeHarnessFailsClosedWhenMissionPluginUnverifiable(t *testing.T) {
	svc, created := missionGateFixture(t, domain.PlanningModeNativeHarness)
	provisioner := &fakeMissionPluginProvisioner{err: errors.New("verify the installed mission plugin: digest drifted")}
	svc.WithMissionPluginProvisioner(provisioner)

	_, err := svc.StartPlanning(context.Background(), created.ID, outcome.StartPlanningInput{
		ExpectedContractRevision: 1, CandidateID: "planner", RequestKey: "gate-fail-closed",
	})
	if err == nil {
		t.Fatal("mission start proceeded without a verified mission plugin")
	}
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "MISSION_PLUGIN_UNAVAILABLE" {
		t.Fatalf("error = %v, want MISSION_PLUGIN_UNAVAILABLE", err)
	}
	if len(provisioner.calls) != 1 || provisioner.calls[0] != created.SpaceID {
		t.Fatalf("provisioner calls = %v, want exactly one attempt for space %q", provisioner.calls, created.SpaceID)
	}
	if _, getErr := svc.GetCurrentPlanning(context.Background(), created.ID); getErr == nil {
		t.Fatal("a planning session exists after the closed gate")
	}
}

// Nil provisioner is not a bypass: native-harness start stays closed.
func TestStartPlanning_NativeHarnessWithoutProvisionerFailsClosed(t *testing.T) {
	svc, created := missionGateFixture(t, domain.PlanningModeNativeHarness)
	_, err := svc.StartPlanning(context.Background(), created.ID, outcome.StartPlanningInput{
		ExpectedContractRevision: 1, CandidateID: "planner", RequestKey: "gate-unwired",
	})
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "MISSION_PLUGIN_UNAVAILABLE" {
		t.Fatalf("error = %v, want MISSION_PLUGIN_UNAVAILABLE", err)
	}
}

// A verified plugin lets mission start proceed.
func TestStartPlanning_NativeHarnessVerifiedPluginProceeds(t *testing.T) {
	svc, created := missionGateFixture(t, domain.PlanningModeNativeHarness)
	provisioner := &fakeMissionPluginProvisioner{home: "/scoped/home"}
	svc.WithMissionPluginProvisioner(provisioner)
	view, err := svc.StartPlanning(context.Background(), created.ID, outcome.StartPlanningInput{
		ExpectedContractRevision: 1, CandidateID: "planner", RequestKey: "gate-verified",
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.Session.Status != domain.PlanningSessionActive {
		t.Fatalf("planning session = %+v", view.Session)
	}
	if len(provisioner.calls) != 1 {
		t.Fatalf("provisioner calls = %v, want one", provisioner.calls)
	}
}

// Direct-API planning does not consume the harness command and must not pay
// its verification.
func TestStartPlanning_DirectAPISkipsMissionPluginGate(t *testing.T) {
	svc, created := missionGateFixture(t, domain.PlanningModeDirectAPI)
	provisioner := &fakeMissionPluginProvisioner{err: errors.New("must never be called")}
	svc.WithMissionPluginProvisioner(provisioner)
	if _, err := svc.StartPlanning(context.Background(), created.ID, outcome.StartPlanningInput{
		ExpectedContractRevision: 1, CandidateID: "planner", RequestKey: "gate-direct-api",
	}); err != nil {
		t.Fatal(err)
	}
	if len(provisioner.calls) != 0 {
		t.Fatalf("direct-API planning invoked the mission plugin gate: %v", provisioner.calls)
	}
}

func missionGateFixture(t *testing.T, mode domain.PlanningMode) (*outcome.Service, domain.Outcome) {
	t.Helper()
	ctx := context.Background()
	store := sqlitetest.MustOpen(t)
	project := domain.ProjectRecord{
		ID: "mission-gate-project", Path: initPlanningRepo(t), DisplayName: "Mission gate", RegisteredAt: time.Now().UTC(),
	}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	provider := &interactivePlanningFake{
		candidates: []ports.PlanningCandidate{{
			ID: "planner", Ready: true,
			Binding: domain.PlanningBinding{Mode: mode, Provider: "codex", ModelSelection: domain.PlanningModelProviderDefault},
		}},
	}
	svc := outcome.New(store, nil).WithPlanning(provider, &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}})
	svc.AdmissionPolicy = testAdmissionPolicy()
	created, err := svc.Create(ctx, outcome.CreateInput{
		ProjectID: domain.ProjectID(project.ID), Title: "Mission gate", Goal: "Prove the mission command gate.",
		SuccessCriteria: []string{"The gate holds."}, Review: "Owner reviews.",
		AuthorityCeiling: domain.ProposedAuthority{ReadWorkspace: true}, RequestKey: "mission-gate-outcome",
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, created.Outcome
}
