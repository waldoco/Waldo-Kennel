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
	svc, created, _ := missionGateFixture(t, domain.PlanningModeNativeHarness)
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
	svc, created, _ := missionGateFixture(t, domain.PlanningModeNativeHarness)
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
	svc, created, _ := missionGateFixture(t, domain.PlanningModeNativeHarness)
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
	svc, created, _ := missionGateFixture(t, domain.PlanningModeDirectAPI)
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

// Mid-session tamper closes the next turn: the plugin verified at mission
// start but wiped afterwards, so Continue must fail closed before any owner
// message becomes durable and before the provider is asked anything.
func TestContinuePlanning_NativeHarnessFailsClosedOnMidSessionTamper(t *testing.T) {
	svc, created, provider := missionGateFixture(t, domain.PlanningModeNativeHarness)
	provider.nativeRefs = []string{"native-ref-tamper"}
	provisioner := &fakeMissionPluginProvisioner{home: "/scoped/home"}
	svc.WithMissionPluginProvisioner(provisioner)
	ctx := context.Background()
	view, err := svc.StartPlanning(ctx, created.ID, outcome.StartPlanningInput{
		ExpectedContractRevision: 1, CandidateID: "planner", RequestKey: "tamper-start",
	})
	if err != nil {
		t.Fatal(err)
	}
	provisioner.err = errors.New("verify the installed mission plugin: digest drifted")

	_, err = svc.ContinuePlanning(ctx, created.ID, view.Session.ID, outcome.PlanningMessageInput{
		ExpectedSessionRevision: view.Session.Revision, Text: "Keep going.", RequestKey: "tamper-continue",
	})
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "MISSION_PLUGIN_UNAVAILABLE" {
		t.Fatalf("error = %v, want MISSION_PLUGIN_UNAVAILABLE", err)
	}
	if provider.discussCalls != 0 {
		t.Fatalf("the provider was asked to plan in an unverified environment (%d calls)", provider.discussCalls)
	}
	// The closed turn left no durable owner message behind, so after repair
	// the same request key is a fresh attempt, not a replay conflict.
	provisioner.err = nil
	if _, err := svc.ContinuePlanning(ctx, created.ID, view.Session.ID, outcome.PlanningMessageInput{
		ExpectedSessionRevision: view.Session.Revision, Text: "Keep going.", RequestKey: "tamper-continue",
	}); err != nil {
		t.Fatalf("the repaired turn did not proceed: %v", err)
	}
	if provider.discussCalls != 1 {
		t.Fatalf("provider calls after repair = %d, want one", provider.discussCalls)
	}
}

// The planning turn must run in exactly the home the provisioner verified:
// the scoped home reaches the provider request, never the ambient one.
func TestContinuePlanning_NativeHarnessBindsVerifiedHome(t *testing.T) {
	svc, created, provider := missionGateFixture(t, domain.PlanningModeNativeHarness)
	provider.nativeRefs = []string{"native-ref-bind"}
	provisioner := &fakeMissionPluginProvisioner{home: "/scoped/mission-home"}
	svc.WithMissionPluginProvisioner(provisioner)
	ctx := context.Background()
	var boundHome string
	provider.discussResult = func(request ports.PlanningDiscussionRequest) domain.PlanningReadinessResult {
		boundHome = request.HarnessHome
		return domain.NewPlanningReadinessResult("One more detail would help.", nil, []domain.PlanningReadinessIssue{{
			Key: "more-context", Kind: domain.ReadinessContextInsufficient, Route: domain.RouteAnswerContext, Source: domain.ReadinessSourcePlannerDeclared,
			Prompt: "Share the missing detail?", Reason: "The approach depends on it.",
			Choices: []domain.PlanningReadinessChoice{{Key: "share", Label: "Share it"}, {Key: "skip", Label: "Skip it"}},
		}})
	}
	view, err := svc.StartPlanning(ctx, created.ID, outcome.StartPlanningInput{
		ExpectedContractRevision: 1, CandidateID: "planner", RequestKey: "bind-start",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ContinuePlanning(ctx, created.ID, view.Session.ID, outcome.PlanningMessageInput{
		ExpectedSessionRevision: view.Session.Revision, Text: "Sketch the approach.", RequestKey: "bind-continue",
	}); err != nil {
		t.Fatal(err)
	}
	if boundHome != "/scoped/mission-home" {
		t.Fatalf("planning turn bound home %q, want the provisioner's verified /scoped/mission-home", boundHome)
	}
}

// Finalize is a turn like any other: a wiped plugin closes it too.
func TestFinalizePlanning_NativeHarnessFailsClosedOnMidSessionTamper(t *testing.T) {
	svc, created, provider := missionGateFixture(t, domain.PlanningModeNativeHarness)
	provisioner := &fakeMissionPluginProvisioner{home: "/scoped/home"}
	svc.WithMissionPluginProvisioner(provisioner)
	ctx := context.Background()
	view, err := svc.StartPlanning(ctx, created.ID, outcome.StartPlanningInput{
		ExpectedContractRevision: 1, CandidateID: "planner", RequestKey: "finalize-tamper-start",
	})
	if err != nil {
		t.Fatal(err)
	}
	provisioner.err = errors.New("verify the installed mission plugin: skill removed")
	_, err = svc.FinalizePlanning(ctx, created.ID, view.Session.ID, outcome.PlanningFinalizeInput{
		ExpectedSessionRevision: view.Session.Revision, RequestKey: "finalize-tamper",
	})
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "MISSION_PLUGIN_UNAVAILABLE" {
		t.Fatalf("error = %v, want MISSION_PLUGIN_UNAVAILABLE", err)
	}
	if provider.discussCalls != 0 {
		t.Fatalf("finalize reached the provider in an unverified environment (%d calls)", provider.discussCalls)
	}
}

// An idempotent start replay still verifies the command instead of skipping
// the gate on the strength of the first call.
func TestStartPlanning_IdempotentReplayReverifiesMissionPlugin(t *testing.T) {
	svc, created, _ := missionGateFixture(t, domain.PlanningModeNativeHarness)
	provisioner := &fakeMissionPluginProvisioner{home: "/scoped/home"}
	svc.WithMissionPluginProvisioner(provisioner)
	ctx := context.Background()
	if _, err := svc.StartPlanning(ctx, created.ID, outcome.StartPlanningInput{
		ExpectedContractRevision: 1, CandidateID: "planner", RequestKey: "replay-key",
	}); err != nil {
		t.Fatal(err)
	}
	provisioner.err = errors.New("verify the installed mission plugin: cache wiped")
	_, err := svc.StartPlanning(ctx, created.ID, outcome.StartPlanningInput{
		ExpectedContractRevision: 1, CandidateID: "planner", RequestKey: "replay-key",
	})
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "MISSION_PLUGIN_UNAVAILABLE" {
		t.Fatalf("error = %v, want MISSION_PLUGIN_UNAVAILABLE", err)
	}
	if len(provisioner.calls) != 2 {
		t.Fatalf("provisioner calls = %v, want one per start call", provisioner.calls)
	}
}

func missionGateFixture(t *testing.T, mode domain.PlanningMode) (*outcome.Service, domain.Outcome, *interactivePlanningFake) {
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
	return svc, created.Outcome, provider
}
