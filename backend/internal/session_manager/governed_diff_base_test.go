package sessionmanager

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

func governedDiffBasePolicy() *domain.AttemptExecutionPolicy {
	return &domain.AttemptExecutionPolicy{
		OutcomeID: "out", PlanRevisionID: "plan", WorkUnitID: "wu", ContractRevisionNumber: 1,
		RunBriefCoreDigest: "brief", RequiredCapabilities: []string{domain.CapabilityWorktreeRead},
		Grants: []domain.CapabilityGrant{{ID: "read", Name: domain.CapabilityWorktreeRead, Scope: "worktree/*"}},
	}
}

// governedChatLauncher adds the governed-attempt preflight seam to the
// recording launcher, mirroring the production chat adapter.
type governedChatLauncher struct {
	*recordingLauncher
}

func (l *governedChatLauncher) PreflightChatExecutionPolicy(context.Context, domain.AgentHarness, domain.AttemptExecutionPolicy) error {
	return nil
}

// A governed TUI spawn must resolve the source-tree base BEFORE the
// provider-launch crash boundary: the custody record sealed inside the
// callback reads it from the prelaunch SessionRecord, it is durable in the
// store before the runtime starts, and the final session metadata binds
// exactly the same value.
func TestSpawn_GovernedDiffBaseResolvedBeforeProviderLaunch(t *testing.T) {
	st := newFakeStore()
	repo := newManagerGitRepo(t)
	cfg := testRoleAgents()
	cfg.DefaultBranch = "main"
	st.projects["mer"] = domain.ProjectRecord{ID: "mer", Path: repo, Config: cfg}
	runtime := &fakeRuntime{}
	ws := &fakeWorkspace{}
	ws.path = repo
	m := New(Deps{Runtime: runtime, Agents: singleAgent{agent: &recordingAgent{}}, Workspace: ws, Store: st, Messenger: &fakeMessenger{}, Lifecycle: &fakeLCM{store: st}, LookPath: func(string) (string, error) { return "/bin/true", nil }})
	binding := domain.ExecutionBinding{Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault}
	wantBase := strings.TrimSpace(runManagerGit(t, repo, "rev-parse", "main"))

	var cbSHA, cbRef, cbStoredSHA string
	cbRuntimeCreated := -1
	rec, _, _, err := m.Spawn(ctx, ports.SpawnConfig{
		ProjectID: "mer", Kind: domain.KindWorker, ExactExecutionBinding: &binding,
		ExecutionPolicy: governedDiffBasePolicy(),
		BeforeProviderLaunch: func(_ context.Context, cbRec domain.SessionRecord, _ domain.AttemptExecutionPolicy, _ []domain.SessionWorktreeRecord) error {
			cbSHA, cbRef = cbRec.Metadata.DiffBaseSHA, cbRec.Metadata.DiffBaseRef
			cbRuntimeCreated = runtime.created
			stored, found, err := st.GetSession(context.Background(), cbRec.ID)
			if err != nil || !found {
				t.Fatalf("prelaunch evidence not durable inside the boundary: found=%v err=%v", found, err)
			}
			cbStoredSHA = stored.Metadata.DiffBaseSHA
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if cbSHA == "" || cbSHA != wantBase || cbRef != "main" {
		t.Fatalf("boundary-observed base = sha:%q ref:%q, want %s main", cbSHA, cbRef, wantBase)
	}
	if cbStoredSHA != wantBase {
		t.Fatalf("prelaunch evidence base = %q, want %s durable before launch", cbStoredSHA, wantBase)
	}
	if cbRuntimeCreated != 0 {
		t.Fatalf("runtime had already created %d sessions inside the boundary; the provider must not start first", cbRuntimeCreated)
	}
	if rec.Metadata.DiffBaseSHA != wantBase || rec.Metadata.DiffBaseRef != "main" {
		t.Fatalf("final metadata base = sha:%q ref:%q, want the boundary value %s main", rec.Metadata.DiffBaseSHA, rec.Metadata.DiffBaseRef, wantBase)
	}
}

// The chat controller path binds the same prelaunch-resolved base: the
// boundary observes it, the controller starts only after the boundary, and
// the stored final metadata matches.
func TestChatSpawn_GovernedDiffBaseResolvedBeforeControllerStart(t *testing.T) {
	st := newFakeStore()
	repo := newManagerGitRepo(t)
	cfg := testRoleAgents()
	cfg.DefaultBranch = "main"
	st.projects["mer"] = domain.ProjectRecord{ID: "mer", Path: repo, Config: cfg}
	launcher := &governedChatLauncher{&recordingLauncher{}}
	runtime := &fakeRuntime{}
	ws := &fakeWorkspace{}
	ws.path = repo
	m := New(Deps{
		Runtime: runtime, Agents: singleAgent{agent: &recordingAgent{}}, Workspace: ws, Store: st,
		Messenger: &fakeMessenger{}, Chat: launcher, Lifecycle: &fakeLCM{store: st},
		LookPath: func(string) (string, error) { return "/bin/true", nil },
	})
	binding := domain.ExecutionBinding{Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault}
	wantBase := strings.TrimSpace(runManagerGit(t, repo, "rev-parse", "main"))

	var cbSHA string
	launcherStartsAtBoundary := -1
	rec, _, _, err := m.Spawn(ctx, ports.SpawnConfig{
		ProjectID: "mer", Kind: domain.KindWorker, RequestedMode: domain.SessionModeChat,
		ExactExecutionBinding: &binding, ExecutionPolicy: governedDiffBasePolicy(),
		BeforeProviderLaunch: func(_ context.Context, cbRec domain.SessionRecord, _ domain.AttemptExecutionPolicy, _ []domain.SessionWorktreeRecord) error {
			cbSHA = cbRec.Metadata.DiffBaseSHA
			launcherStartsAtBoundary = len(launcher.started)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if cbSHA == "" || cbSHA != wantBase {
		t.Fatalf("boundary-observed base = %q, want %s", cbSHA, wantBase)
	}
	if launcherStartsAtBoundary != 0 {
		t.Fatalf("chat controller had already started %d times inside the boundary", launcherStartsAtBoundary)
	}
	stored, found, err := st.GetSession(ctx, rec.ID)
	if err != nil || !found {
		t.Fatalf("final session: found=%v err=%v", found, err)
	}
	if stored.Metadata.DiffBaseSHA != wantBase || stored.Metadata.DiffBaseRef != "main" {
		t.Fatalf("final chat metadata base = sha:%q ref:%q, want the boundary value %s main", stored.Metadata.DiffBaseSHA, stored.Metadata.DiffBaseRef, wantBase)
	}
}

// A callback failure on the chat path starts no controller, exactly like the
// terminal path: custody evidence that cannot be sealed stops the launch.
func TestChatSpawn_GovernedCallbackFailureStartsNoController(t *testing.T) {
	st := newFakeStore()
	repo := newManagerGitRepo(t)
	cfg := testRoleAgents()
	cfg.DefaultBranch = "main"
	st.projects["mer"] = domain.ProjectRecord{ID: "mer", Path: repo, Config: cfg}
	launcher := &governedChatLauncher{&recordingLauncher{}}
	runtime := &fakeRuntime{}
	ws := &fakeWorkspace{}
	ws.path = repo
	m := New(Deps{
		Runtime: runtime, Agents: singleAgent{agent: &recordingAgent{}}, Workspace: ws, Store: st,
		Messenger: &fakeMessenger{}, Chat: launcher, Lifecycle: &fakeLCM{store: st},
		LookPath: func(string) (string, error) { return "/bin/true", nil },
	})
	binding := domain.ExecutionBinding{Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault}
	called := false
	_, _, _, err := m.Spawn(ctx, ports.SpawnConfig{
		ProjectID: "mer", Kind: domain.KindWorker, RequestedMode: domain.SessionModeChat,
		ExactExecutionBinding: &binding, ExecutionPolicy: governedDiffBasePolicy(),
		BeforeProviderLaunch: func(_ context.Context, _ domain.SessionRecord, _ domain.AttemptExecutionPolicy, _ []domain.SessionWorktreeRecord) error {
			called = true
			return fmt.Errorf("simulated custody seal failure")
		},
	})
	if err == nil || !called {
		t.Fatalf("spawn err=%v called=%v, want the boundary failure to abort the spawn", err, called)
	}
	if len(launcher.started) != 0 {
		t.Fatalf("chat controller started %d times despite the boundary failure", len(launcher.started))
	}
	if runtime.created != 0 {
		t.Fatalf("terminal runtime created %d sessions despite the boundary failure", runtime.created)
	}
}

// Falsifier: when the single-repo source-tree base cannot be resolved at all,
// a governed spawn must refuse BEFORE the launch boundary - no callback, no
// runtime, no durable session - instead of sealing a custody record with an
// empty base.
func TestSpawn_GovernedSingleRepoBaseUnresolvableRefusesBeforeBoundary(t *testing.T) {
	st := newFakeStore()
	st.projects["mer"] = domain.ProjectRecord{ID: "mer", Path: "/repo/mer", Config: testRoleAgents()}
	runtime := &fakeRuntime{}
	ws := &fakeWorkspace{}
	ws.path = t.TempDir() // not a git repository: every base probe fails
	m := New(Deps{Runtime: runtime, Agents: singleAgent{agent: &recordingAgent{}}, Workspace: ws, Store: st, Messenger: &fakeMessenger{}, Lifecycle: &fakeLCM{store: st}, LookPath: func(string) (string, error) { return "/bin/true", nil }})
	binding := domain.ExecutionBinding{Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault}

	callbackRan := false
	_, _, _, err := m.Spawn(ctx, ports.SpawnConfig{
		ProjectID: "mer", Kind: domain.KindWorker, ExactExecutionBinding: &binding,
		ExecutionPolicy: governedDiffBasePolicy(),
		BeforeProviderLaunch: func(_ context.Context, _ domain.SessionRecord, _ domain.AttemptExecutionPolicy, _ []domain.SessionWorktreeRecord) error {
			callbackRan = true
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "resolved source-tree base") {
		t.Fatalf("Spawn err = %v, want refusal to name the unresolved source-tree base", err)
	}
	if callbackRan {
		t.Fatal("the launch boundary ran despite the missing base")
	}
	if runtime.created != 0 {
		t.Fatalf("runtime created %d sessions despite the refusal", runtime.created)
	}
	if _, found, _ := st.GetSession(context.Background(), "mer-1"); found {
		t.Fatal("refused spawn left a durable session row behind")
	}
}
