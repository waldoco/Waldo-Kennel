package outcome_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/artifactstore"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
)

type interactivePlanningFake struct {
	repositoryObserved        bool
	repositoryToolUseObserved bool
	candidateCalls            int
	discussCalls              int
	candidateErr              error
	candidates                []ports.PlanningCandidate
	effectiveModel            string
	nativeRefs                []string
	beforeDiscuss             func()
	discussStarted            chan struct{}
	discussRelease            chan struct{}
	ignoreCancellation        bool
}

func initPlanningRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("planning fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.name", "Planning Test"}, {"config", "user.email", "planning@example.invalid"}, {"add", "README.md"}, {"commit", "-qm", "fixture"}} {
		command := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	return repo
}

func (*interactivePlanningFake) ID() domain.IntelligenceProviderID { return "openai" }
func (*interactivePlanningFake) AnalyzeContract(context.Context, ports.ContractIntelligenceRequest) (ports.ContractIntelligenceResponse, error) {
	return ports.ContractIntelligenceResponse{}, fmt.Errorf("not used")
}
func (*interactivePlanningFake) DraftPlan(context.Context, ports.PlanIntelligenceRequest) (ports.PlanIntelligenceResponse, error) {
	return ports.PlanIntelligenceResponse{}, fmt.Errorf("not used")
}

func (f *interactivePlanningFake) PlanningCandidates(context.Context) ([]ports.PlanningCandidate, error) {
	f.candidateCalls++
	if f.candidateErr != nil {
		return nil, f.candidateErr
	}
	if f.candidates != nil {
		return f.candidates, nil
	}
	return []ports.PlanningCandidate{{
		ID: "direct-openai-planner", Ready: true,
		Binding: domain.PlanningBinding{Mode: domain.PlanningModeDirectAPI, Provider: "openai", ModelSelection: domain.PlanningModelExplicit, Model: "planner-test"},
	}}, nil
}
func (f *interactivePlanningFake) DiscussPlan(ctx context.Context, request ports.PlanningDiscussionRequest) (ports.PlanningDiscussionResponse, error) {
	f.discussCalls++
	f.repositoryToolUseObserved = f.repositoryToolUseObserved || request.RepositoryToolUse
	if f.discussStarted != nil {
		close(f.discussStarted)
	}
	if f.discussRelease != nil {
		if f.ignoreCancellation {
			<-f.discussRelease
		} else {
			select {
			case <-f.discussRelease:
			case <-ctx.Done():
				return ports.PlanningDiscussionResponse{}, ports.ClassifyReasoningTransport(ctx, 0, ctx.Err())
			}
		}
	}
	if f.beforeDiscuss != nil {
		hook := f.beforeDiscuss
		f.beforeDiscuss = nil
		hook()
	}
	for _, file := range request.RepositoryContext.Files {
		if file.Path == "README.md" && strings.Contains(file.Content, "planning fixture") {
			f.repositoryObserved = true
		}
	}
	effectiveModel := f.effectiveModel
	if effectiveModel == "" {
		effectiveModel = request.Binding.Model
		if request.Binding.ModelSelection == domain.PlanningModelProviderDefault {
			effectiveModel = "provider-resolved-model"
		}
	}
	nativeRef := ""
	if len(f.nativeRefs) >= f.discussCalls {
		nativeRef = f.nativeRefs[f.discussCalls-1]
	}
	provenance := ports.IntelligenceProvenance{EffectiveProvider: request.Binding.Provider, EffectiveModel: effectiveModel, NativeSessionRef: nativeRef}
	latest := request.Turns[len(request.Turns)-1].Text
	if request.Finalize {
		return ports.PlanningDiscussionResponse{Provenance: provenance, Result: ports.PlanningResult{
			Kind: ports.PlanningResultPlanProposal, Message: "A bounded implementation Plan is ready for review.",
			PlanProposal: &domain.PlanDraftProposal{Summary: "Make the bounded local change.", WorkUnits: []domain.PlanDraftWorkUnit{{
				Key: "implement", Title: "Implement the confirmed Outcome", Intent: domain.WorkUnitIntentModify,
				OutputSummary: "The requested local change is ready for review.", CriteriaCovered: []string{"C1"}, EvidenceIdeas: []string{"inspect the retained diff"},
			}}},
		}}, nil
	}
	if strings.Contains(latest, "Contract") {
		return ports.PlanningDiscussionResponse{Provenance: provenance, Result: ports.PlanningResult{
			Kind: ports.PlanningResultContractChange, Message: "The success criterion could be narrower.",
			ContractChange: &ports.PlanContractChangeProposal{Summary: "Narrow the criterion before approval if this distinction matters.", ChangedFields: []string{"successCriteria"}},
		}}, nil
	}
	return ports.PlanningDiscussionResponse{Provenance: provenance, Result: ports.PlanningResult{
		Kind: ports.PlanningResultClarification, Message: "One choice will keep the Plan small.",
		Clarification: &ports.PlanClarification{Question: "Should the first slice stay local only?", Reason: "Remote effects need separate authority.", Recommendation: "Keep it local.", Alternatives: []string{"Include remote delivery later"}},
	}}, nil
}

func newPlanningCancellationFixture(t *testing.T, provider *interactivePlanningFake) (*outcome.Service, *sqlite.Store, domain.Outcome, domain.PlanningSession) {
	t.Helper()
	ctx := context.Background()
	store := sqlitetest.MustOpen(t)
	project := domain.ProjectRecord{ID: "planning-cancel-project", Path: initPlanningRepo(t), DisplayName: "Cancel", RegisteredAt: time.Now().UTC()}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	svc := outcome.New(store, nil).WithPlanning(provider, &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}})
	created, err := svc.Create(ctx, outcome.CreateInput{
		ProjectID: domain.ProjectID(project.ID), Title: "Cancel planning", Goal: "Keep cancellation bounded.",
		SuccessCriteria: []string{"No late planning reply is published."}, Review: "Inspect the durable session.",
		AuthorityCeiling: domain.ProposedAuthority{ReadWorkspace: true}, RequestKey: "planning-cancel-outcome",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, err := svc.StartPlanning(ctx, created.Outcome.ID, outcome.StartPlanningInput{
		ExpectedContractRevision: 1, CandidateID: "direct-openai-planner", RequestKey: "planning-cancel-start",
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, store, created.Outcome, started.Session
}

func TestInteractivePlanning_NativePacketTurnsProduceOnlyAProposedPlan(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("planning fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	runGit("init", "-q")
	runGit("config", "user.name", "Planning Test")
	runGit("config", "user.email", "planning@example.invalid")
	runGit("add", "README.md")
	runGit("commit", "-qm", "fixture")

	store := sqlitetest.MustOpen(t)
	project := domain.ProjectRecord{
		ID: "interactive-planning-project", Path: repo, DisplayName: "Planning fixture", RegisteredAt: time.Now().UTC(),
		Config: domain.ProjectConfig{Worker: domain.RoleOverride{Harness: domain.HarnessClaudeCode, AgentConfig: domain.AgentConfig{Model: "sonnet-test"}}},
	}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatalf("register project: %v", err)
	}
	provider := &interactivePlanningFake{
		candidates: []ports.PlanningCandidate{{
			ID: "native-codex-planner", Ready: true,
			Binding: domain.PlanningBinding{Mode: domain.PlanningModeNativeHarness, Provider: "codex", ModelSelection: domain.PlanningModelProviderDefault},
		}},
		nativeRefs: []string{"native-turn-1", "native-turn-2", "native-turn-3"},
	}
	router := &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}
	svc := outcome.New(store, nil).WithPlanning(provider, router)
	svc.AdmissionPolicy = testAdmissionPolicy()
	created, err := svc.Create(ctx, outcome.CreateInput{
		ProjectID: domain.ProjectID(project.ID), Title: "Interactive planning", Goal: "Make one bounded local change.",
		SuccessCriteria: []string{"The local change is reviewable."}, Review: "Owner reviews the retained diff.",
		AuthorityCeiling: domain.ProposedAuthority{ReadWorkspace: true, WriteWorkspace: true},
		StopConditions:   []string{"Stop before commands or external effects."}, RequestKey: "interactive-outcome",
	})
	if err != nil {
		t.Fatalf("create Outcome: %v", err)
	}
	candidates, err := svc.PlanningCandidates(ctx, created.Outcome.ID, 1)
	if err != nil || len(candidates) != 1 || !candidates[0].Ready {
		t.Fatalf("planning candidates = %+v err=%v", candidates, err)
	}
	view, err := svc.StartPlanning(ctx, created.Outcome.ID, outcome.StartPlanningInput{
		ExpectedContractRevision: 1, CandidateID: candidates[0].ID, RequestKey: "start-interactive-planning",
	})
	if err != nil {
		t.Fatalf("start planning: %v", err)
	}
	if view.Session.ContextMode != domain.PlanningContextRepositoryRead || view.Session.WaitingOn != domain.PlanningWaitingOwner || len(view.Turns) != 0 {
		t.Fatalf("initial planning view = %+v", view)
	}

	view, err = svc.ContinuePlanning(ctx, created.Outcome.ID, view.Session.ID, outcome.PlanningMessageInput{
		ExpectedSessionRevision: view.Session.Revision, Text: "Inspect it and ask what is ambiguous.", RequestKey: "planning-message-1",
	})
	if err != nil {
		t.Fatalf("continue planning: %v", err)
	}
	if !provider.repositoryObserved || provider.repositoryToolUseObserved || len(view.Turns) != 2 || view.Turns[1].Kind != domain.PlanningTurnClarification || view.ProposedPlan != nil {
		t.Fatalf("clarification planning view = %+v repositoryObserved=%v repositoryToolUse=%v", view, provider.repositoryObserved, provider.repositoryToolUseObserved)
	}

	view, err = svc.ContinuePlanning(ctx, created.Outcome.ID, view.Session.ID, outcome.PlanningMessageInput{
		ExpectedSessionRevision: view.Session.Revision, Text: "Suggest any Contract change, but do not make it.", RequestKey: "planning-message-2",
	})
	if err != nil {
		t.Fatalf("request Contract suggestion: %v", err)
	}
	if view.Turns[len(view.Turns)-1].Kind != domain.PlanningTurnContractChangeProposal {
		t.Fatalf("last turn = %+v", view.Turns[len(view.Turns)-1])
	}
	unchanged, err := svc.Get(ctx, created.Outcome.ID)
	if err != nil || unchanged.Outcome.CurrentRevisionNumber != 1 || len(unchanged.History) != 1 {
		t.Fatalf("Contract changed through suggestion: revision=%d history=%d err=%v", unchanged.Outcome.CurrentRevisionNumber, len(unchanged.History), err)
	}

	finalizeExpectedRevision := view.Session.Revision
	view, err = svc.FinalizePlanning(ctx, created.Outcome.ID, view.Session.ID, outcome.PlanningFinalizeInput{
		ExpectedSessionRevision: finalizeExpectedRevision, RequestKey: "planning-finalize",
	})
	if err != nil {
		t.Fatalf("finalize planning: %v", err)
	}
	if view.Session.Status != domain.PlanningSessionProposalReady || view.ProposedPlan == nil || view.ProposedPlan.Status != domain.PlanStatusProposed {
		t.Fatalf("final planning view = %+v", view)
	}
	if view.ProposedPlan.PlanningSessionID != view.Session.ID || view.ProposedPlan.SourceIntelligenceRunID.IsZero() {
		t.Fatalf("Plan lost planning provenance: %+v", view.ProposedPlan)
	}
	if view.Session.NativeConversationRef != "" {
		t.Fatalf("packet planning session claimed one provider thread: %q", view.Session.NativeConversationRef)
	}
	var nativeRefs []string
	for _, turn := range view.Turns {
		if turn.Role != domain.PlanningTurnPlanner {
			continue
		}
		run, found, err := store.GetIntelligenceRun(ctx, turn.IntelligenceRunID)
		if err != nil || !found {
			t.Fatalf("planning run %s found=%v err=%v", turn.IntelligenceRunID, found, err)
		}
		nativeRefs = append(nativeRefs, run.NativeSessionRef)
	}
	if fmt.Sprint(nativeRefs) != fmt.Sprint(provider.nativeRefs) {
		t.Fatalf("per-turn native refs = %v, want %v", nativeRefs, provider.nativeRefs)
	}
	finalized := view
	view, err = svc.FinalizePlanning(ctx, created.Outcome.ID, view.Session.ID, outcome.PlanningFinalizeInput{
		ExpectedSessionRevision: finalizeExpectedRevision, RequestKey: "planning-finalize",
	})
	if err != nil {
		t.Fatalf("replay finalized request: %v", err)
	}
	if view.ProposedPlan == nil || view.ProposedPlan.ID != finalized.ProposedPlan.ID || len(view.Turns) != len(finalized.Turns) || provider.discussCalls != 3 {
		t.Fatalf("finalize replay duplicated work: view=%+v discussCalls=%d", view, provider.discussCalls)
	}
	attempts, err := store.ListAttempts(ctx, created.Outcome.ID)
	if err != nil || len(attempts) != 0 {
		t.Fatalf("planning created execution attempts: attempts=%+v err=%v", attempts, err)
	}
}

func TestInteractivePlanning_StartReplayPrecedesMutableAdmissionChecks(t *testing.T) {
	ctx := context.Background()
	store := sqlitetest.MustOpen(t)
	project := domain.ProjectRecord{ID: "planning-replay-project", Path: initPlanningRepo(t), DisplayName: "Replay", RegisteredAt: time.Now().UTC()}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	provider := &interactivePlanningFake{}
	svc := outcome.New(store, nil).WithPlanning(provider, &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}})
	created, err := svc.Create(ctx, outcome.CreateInput{
		ProjectID: domain.ProjectID(project.ID), Title: "Replay planning", Goal: "Keep retries exact.",
		SuccessCriteria: []string{"One planning session exists."}, Review: "Inspect the session.",
		AuthorityCeiling: domain.ProposedAuthority{ReadWorkspace: true}, RequestKey: "planning-replay-outcome",
	})
	if err != nil {
		t.Fatal(err)
	}
	input := outcome.StartPlanningInput{ExpectedContractRevision: 1, CandidateID: "direct-openai-planner", RequestKey: "planning-replay-start"}
	first, err := svc.StartPlanning(ctx, created.Outcome.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	provider.candidateErr = fmt.Errorf("inventory changed")
	replayed, err := svc.StartPlanning(ctx, created.Outcome.ID, input)
	if err != nil || replayed.Session.ID != first.Session.ID || provider.candidateCalls != 1 {
		t.Fatalf("start replay session=%s calls=%d err=%v", replayed.Session.ID, provider.candidateCalls, err)
	}
	changed := input
	changed.CandidateID = "another-planner"
	if code := requireAPICode(t, func() error { _, err := svc.StartPlanning(ctx, created.Outcome.ID, changed); return err }()); code != "PLANNING_REQUEST_CONFLICT" {
		t.Fatalf("changed replay code=%s", code)
	}
}

func TestInteractivePlanning_ExplicitModelDriftFailsClosed(t *testing.T) {
	ctx := context.Background()
	store := sqlitetest.MustOpen(t)
	project := domain.ProjectRecord{ID: "planning-model-project", Path: initPlanningRepo(t), DisplayName: "Model", RegisteredAt: time.Now().UTC()}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	provider := &interactivePlanningFake{effectiveModel: "unexpected-model"}
	svc := outcome.New(store, nil).WithPlanning(provider, &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}})
	created, err := svc.Create(ctx, outcome.CreateInput{
		ProjectID: domain.ProjectID(project.ID), Title: "Model provenance", Goal: "Use the selected model.",
		SuccessCriteria: []string{"Model provenance is exact."}, Review: "Inspect provenance.",
		AuthorityCeiling: domain.ProposedAuthority{ReadWorkspace: true}, RequestKey: "planning-model-outcome",
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.StartPlanning(ctx, created.Outcome.ID, outcome.StartPlanningInput{ExpectedContractRevision: 1, CandidateID: "direct-openai-planner", RequestKey: "planning-model-start"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.ContinuePlanning(ctx, created.Outcome.ID, view.Session.ID, outcome.PlanningMessageInput{ExpectedSessionRevision: view.Session.Revision, Text: "Continue.", RequestKey: "planning-model-turn"})
	if code := requireAPICode(t, err); code != "PLANNING_MODEL_MISMATCH" {
		t.Fatalf("model drift code=%s err=%v", code, err)
	}
	recovered, err := svc.GetCurrentPlanning(ctx, created.Outcome.ID)
	if err != nil || recovered.Session.WaitingOn != domain.PlanningWaitingOwner || recovered.Session.LastFailureCode != "PLANNING_MODEL_MISMATCH" {
		t.Fatalf("model mismatch state=%+v err=%v", recovered.Session, err)
	}
}

func TestInteractivePlanning_ProviderDefaultRecordsResolvedModel(t *testing.T) {
	ctx := context.Background()
	store := sqlitetest.MustOpen(t)
	project := domain.ProjectRecord{ID: "planning-default-project", Path: initPlanningRepo(t), DisplayName: "Default model", RegisteredAt: time.Now().UTC()}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	provider := &interactivePlanningFake{candidates: []ports.PlanningCandidate{{
		ID: "provider-default-planner", Ready: true,
		Binding: domain.PlanningBinding{Mode: domain.PlanningModeDirectAPI, Provider: "openai", ModelSelection: domain.PlanningModelProviderDefault},
	}}}
	svc := outcome.New(store, nil).WithPlanning(provider, &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}})
	created, err := svc.Create(ctx, outcome.CreateInput{
		ProjectID: domain.ProjectID(project.ID), Title: "Default model", Goal: "Allow provider model resolution.",
		SuccessCriteria: []string{"Effective model is recorded."}, Review: "Inspect provenance.",
		AuthorityCeiling: domain.ProposedAuthority{ReadWorkspace: true}, RequestKey: "planning-default-outcome",
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.StartPlanning(ctx, created.Outcome.ID, outcome.StartPlanningInput{ExpectedContractRevision: 1, CandidateID: "provider-default-planner", RequestKey: "planning-default-start"})
	if err != nil {
		t.Fatal(err)
	}
	view, err = svc.ContinuePlanning(ctx, created.Outcome.ID, view.Session.ID, outcome.PlanningMessageInput{ExpectedSessionRevision: view.Session.Revision, Text: "Continue.", RequestKey: "planning-default-turn"})
	if err != nil || view.Session.EffectiveModel != "provider-resolved-model" {
		t.Fatalf("provider-default planning model=%q err=%v", view.Session.EffectiveModel, err)
	}
}

func TestInteractivePlanning_ContractChangeDuringProviderCallSupersedesWithoutPlan(t *testing.T) {
	ctx := context.Background()
	store := sqlitetest.MustOpen(t)
	project := domain.ProjectRecord{ID: "planning-race-project", Path: initPlanningRepo(t), DisplayName: "Contract race", RegisteredAt: time.Now().UTC()}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	provider := &interactivePlanningFake{}
	svc := outcome.New(store, nil).WithPlanning(provider, &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}})
	created, err := svc.Create(ctx, outcome.CreateInput{
		ProjectID: domain.ProjectID(project.ID), Title: "Contract race", Goal: "Keep the Plan on the current Contract.",
		SuccessCriteria: []string{"No stale Plan is saved."}, Review: "Inspect Contract lineage.",
		AuthorityCeiling: domain.ProposedAuthority{ReadWorkspace: true, WriteWorkspace: true}, RequestKey: "planning-race-outcome",
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.StartPlanning(ctx, created.Outcome.ID, outcome.StartPlanningInput{ExpectedContractRevision: 1, CandidateID: "direct-openai-planner", RequestKey: "planning-race-start"})
	if err != nil {
		t.Fatal(err)
	}
	provider.beforeDiscuss = func() {
		_, reviseErr := svc.ReviseContract(ctx, created.Outcome.ID, outcome.ReviseContractInput{
			ExpectedRevision: 1, Goal: "Keep the revised Plan on the current Contract.",
			SuccessCriteria: []string{"No stale Plan is ever saved."}, Review: "Inspect revised Contract lineage.",
			AuthorityCeiling: domain.ProposedAuthority{ReadWorkspace: true, WriteWorkspace: true},
		})
		if reviseErr != nil {
			t.Fatalf("revise during provider call: %v", reviseErr)
		}
	}
	_, err = svc.FinalizePlanning(ctx, created.Outcome.ID, view.Session.ID, outcome.PlanningFinalizeInput{ExpectedSessionRevision: view.Session.Revision, RequestKey: "planning-race-finalize"})
	if code := requireAPICode(t, err); code != "PLANNING_CONTRACT_STALE" {
		t.Fatalf("contract race code=%s err=%v", code, err)
	}
	current, err := svc.GetCurrentPlanning(ctx, created.Outcome.ID)
	if err != nil || current.Session.Status != domain.PlanningSessionSuperseded {
		t.Fatalf("superseded planning state=%+v err=%v", current.Session, err)
	}
	if _, found, err := store.GetLatestPlanRevision(ctx, created.Outcome.ID); err != nil || found {
		t.Fatalf("stale Plan persisted found=%v err=%v", found, err)
	}
}

func TestInteractivePlanning_SuppliedPacketDoesNotReadRepository(t *testing.T) {
	ctx := context.Background()
	store := sqlitetest.MustOpen(t)
	missingRepo := filepath.Join(t.TempDir(), "repository-does-not-exist")
	project := domain.ProjectRecord{ID: "planning-doc-project", Path: missingRepo, DisplayName: "Documents", RegisteredAt: time.Now().UTC()}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	snapshots, err := artifactstore.New(artifactstore.Config{Root: filepath.Join(t.TempDir(), "artifacts")})
	if err != nil {
		t.Fatal(err)
	}
	provider := &interactivePlanningFake{}
	svc := outcome.New(store, nil).WithDocuments(store, snapshots).WithPlanning(provider, &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}})
	created, err := svc.Create(ctx, outcome.CreateInput{
		ProjectID: domain.ProjectID(project.ID), Title: "Document planning", Goal: "Plan from supplied material.",
		SuccessCriteria: []string{"The repository is not read."}, Review: "Inspect frozen context.",
		RequestKey: "planning-doc-outcome",
	})
	if err != nil {
		t.Fatal(err)
	}
	doc := filepath.Join(t.TempDir(), "brief.md")
	if err := os.WriteFile(doc, []byte("# Supplied planning brief\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	selected, err := svc.SelectDocuments(ctx, created.Outcome.ID, []string{doc})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApproveDocuments(ctx, created.Outcome.ID, selected.Context.Digest); err != nil {
		t.Fatal(err)
	}
	view, err := svc.StartPlanning(ctx, created.Outcome.ID, outcome.StartPlanningInput{
		ExpectedContractRevision: 1, CandidateID: "direct-openai-planner", ContextMode: domain.PlanningContextSuppliedPacket, RequestKey: "planning-doc-start",
	})
	if err != nil {
		t.Fatalf("start from supplied packet with unavailable repository: %v", err)
	}
	var snapshot ports.RepositoryContextSnapshot
	if err := json.Unmarshal(view.Session.ContextSnapshotJSON, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Root != "supplied-documents" || snapshot.UnavailableReason != "" {
		t.Fatalf("supplied planning snapshot=%+v", snapshot)
	}
}

func TestInteractivePlanning_CancelInterruptsProviderAndReplaysTerminalView(t *testing.T) {
	provider := &interactivePlanningFake{discussStarted: make(chan struct{}), discussRelease: make(chan struct{})}
	svc, store, created, session := newPlanningCancellationFixture(t, provider)
	ctx := context.Background()
	type turnResult struct {
		view outcome.PlanningView
		err  error
	}
	result := make(chan turnResult, 1)
	go func() {
		view, err := svc.ContinuePlanning(ctx, created.ID, session.ID, outcome.PlanningMessageInput{
			ExpectedSessionRevision: session.Revision, Text: "Wait for cancellation.", RequestKey: "planning-cancel-turn",
		})
		result <- turnResult{view: view, err: err}
	}()

	select {
	case <-provider.discussStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("provider call did not start")
	}
	cancelled, err := svc.CancelPlanning(ctx, created.ID, session.ID, session.Revision)
	if err != nil || cancelled.Session.Status != domain.PlanningSessionCancelled {
		t.Fatalf("cancel in-flight planning: session=%+v err=%v", cancelled.Session, err)
	}
	replayed, err := svc.CancelPlanning(ctx, created.ID, session.ID, session.Revision)
	if err != nil || replayed.Session.Status != domain.PlanningSessionCancelled || replayed.Session.ID != cancelled.Session.ID {
		t.Fatalf("replay cancellation: session=%+v err=%v", replayed.Session, err)
	}

	select {
	case completed := <-result:
		if code := requireAPICode(t, completed.err); code != "REASONING_CANCELLED" {
			t.Fatalf("cancelled provider call code=%s err=%v", code, completed.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("provider call did not observe cancellation")
	}
	turns, err := store.ListPlanningTurns(ctx, session.ID)
	if err != nil || len(turns) != 1 || turns[0].Role != domain.PlanningTurnOwner {
		t.Fatalf("cancelled turn history=%+v err=%v", turns, err)
	}
}

func TestInteractivePlanning_LateProviderResponseCannotPublishAfterCancel(t *testing.T) {
	provider := &interactivePlanningFake{
		discussStarted: make(chan struct{}), discussRelease: make(chan struct{}), ignoreCancellation: true,
	}
	svc, store, created, session := newPlanningCancellationFixture(t, provider)
	ctx := context.Background()
	result := make(chan error, 1)
	go func() {
		_, err := svc.FinalizePlanning(ctx, created.ID, session.ID, outcome.PlanningFinalizeInput{
			ExpectedSessionRevision: session.Revision, RequestKey: "planning-late-finalize",
		})
		result <- err
	}()

	select {
	case <-provider.discussStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("provider call did not start")
	}
	if _, err := svc.CancelPlanning(ctx, created.ID, session.ID, session.Revision); err != nil {
		t.Fatalf("cancel in-flight finalization: %v", err)
	}
	close(provider.discussRelease)
	select {
	case err := <-result:
		if code := requireAPICode(t, err); code != "REASONING_CANCELLED" {
			t.Fatalf("late provider response code=%s err=%v", code, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("late provider call did not return")
	}
	turns, err := store.ListPlanningTurns(ctx, session.ID)
	if err != nil || len(turns) != 1 || turns[0].Kind != domain.PlanningTurnFinalizeRequest {
		t.Fatalf("late response published a planning turn: turns=%+v err=%v", turns, err)
	}
	if _, found, err := store.GetLatestPlanRevision(ctx, created.ID); err != nil || found {
		t.Fatalf("late response published a Plan: found=%v err=%v", found, err)
	}
}

// fixedRepositoryContextLimits is a constant RepositoryContextLimitsSource,
// letting a test trigger BuildRepositoryContext's discovery-entry-limit
// without needing thousands of fixture files.
type fixedRepositoryContextLimits struct {
	maxFiles, maxBytes, maxVisited int
	err                            error
}

func (f fixedRepositoryContextLimits) RepositoryContextLimits(context.Context) (int, int, int, error) {
	return f.maxFiles, f.maxBytes, f.maxVisited, f.err
}

func TestInteractivePlanning_SettingsReadFailureCreatesNoSessionOrProviderTurn(t *testing.T) {
	ctx := context.Background()
	store := sqlitetest.MustOpen(t)
	repo := initPlanningRepo(t)
	project := domain.ProjectRecord{ID: "planning-limits-error", Path: repo, DisplayName: "Limits failure", RegisteredAt: time.Now().UTC()}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	provider := &interactivePlanningFake{}
	want := errors.New("settings read failed")
	svc := outcome.New(store, nil).
		WithPlanning(provider, &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}).
		WithRepositoryContextLimits(fixedRepositoryContextLimits{err: want})
	created, err := svc.Create(ctx, outcome.CreateInput{
		ProjectID: domain.ProjectID(project.ID), Title: "Limits failure", Goal: "Do not disguise a settings failure as default context limits.",
		SuccessCriteria: []string{"No context is sent under unintended bounds."}, Review: "Inspect provider calls and durable planning state.",
		AuthorityCeiling: domain.ProposedAuthority{ReadWorkspace: true}, RequestKey: "planning-limits-error-outcome",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.StartPlanning(ctx, created.Outcome.ID, outcome.StartPlanningInput{
		ExpectedContractRevision: 1, CandidateID: "direct-openai-planner", RequestKey: "planning-limits-error-start",
	})
	if !errors.Is(err, want) || !strings.Contains(err.Error(), "resolve repository-context limits") {
		t.Fatalf("StartPlanning() error = %v, want surfaced settings failure", err)
	}
	if provider.discussCalls != 0 {
		t.Fatalf("provider discussion calls = %d, want zero", provider.discussCalls)
	}
	if _, found, err := store.GetCurrentPlanningSession(ctx, created.Outcome.ID); err != nil || found {
		t.Fatalf("settings failure created a planning session: found=%v err=%v", found, err)
	}
}

// TestInteractivePlanning_PartialRepositoryContextStillStartsPlanning covers
// the launch-stabilization fix: a repository-context snapshot that hit a
// bounded discovery limit (UnavailableReason set) but still found usable
// files must not refuse planning outright — StartPlanning previously hard-
// blocked on ANY UnavailableReason, even though BuildRepositoryContext
// deliberately keeps what it found before stopping and the prompt builder
// already discloses the limitation honestly.
func TestInteractivePlanning_PartialRepositoryContextStillStartsPlanning(t *testing.T) {
	ctx := context.Background()
	store := sqlitetest.MustOpen(t)
	repo := initPlanningRepo(t)
	deep := filepath.Join(repo, "source")
	if err := os.Mkdir(deep, 0o700); err != nil {
		t.Fatal(err)
	}
	// A tiny MaxVisited (below the number of entries below) guarantees the
	// discovery-entry-limit is hit deterministically without a large fixture.
	for i := 0; i < 10; i++ {
		if err := os.WriteFile(filepath.Join(deep, fmt.Sprintf("entry-%02d.dat", i)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	project := domain.ProjectRecord{ID: "planning-partial-project", Path: repo, DisplayName: "Partial", RegisteredAt: time.Now().UTC()}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	provider := &interactivePlanningFake{}
	svc := outcome.New(store, nil).
		WithPlanning(provider, &routingInventoryFake{candidates: []domain.RoutingCandidate{readyClaudeCandidate()}}).
		WithRepositoryContextLimits(fixedRepositoryContextLimits{maxFiles: 32, maxBytes: 96 << 10, maxVisited: 3})
	created, err := svc.Create(ctx, outcome.CreateInput{
		ProjectID: domain.ProjectID(project.ID), Title: "Partial context planning", Goal: "Plan despite a bounded discovery limit.",
		SuccessCriteria: []string{"Planning starts from partial but real context."}, Review: "Inspect the durable session.",
		AuthorityCeiling: domain.ProposedAuthority{ReadWorkspace: true}, RequestKey: "planning-partial-outcome",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, err := svc.StartPlanning(ctx, created.Outcome.ID, outcome.StartPlanningInput{
		ExpectedContractRevision: 1, CandidateID: "direct-openai-planner", RequestKey: "planning-partial-start",
	})
	if err != nil {
		t.Fatalf("StartPlanning refused a partial-but-usable repository context: %v", err)
	}
	if started.Session.Status != domain.PlanningSessionActive {
		t.Fatalf("session status = %s, want active", started.Session.Status)
	}
	view, err := svc.ContinuePlanning(ctx, created.Outcome.ID, started.Session.ID, outcome.PlanningMessageInput{
		ExpectedSessionRevision: started.Session.Revision, Text: "Inspect it and ask what is ambiguous.", RequestKey: "planning-partial-message",
	})
	if err != nil {
		t.Fatalf("continue planning: %v", err)
	}
	if len(view.Turns) != 2 {
		t.Fatalf("planning view = %+v, want an owner turn and a provider reply", view)
	}
	if !provider.repositoryObserved {
		t.Fatal("provider did not receive the inspected README content despite a usable partial snapshot")
	}
}
