package sessionmanager

import (
	"context"
	"reflect"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

func workspaceProjectFixture() (domain.ProjectRecord, *fakeWorkspace) {
	project := domain.ProjectRecord{ID: "mer", Path: "/repo/mer", Kind: domain.ProjectKindWorkspace, Config: testRoleAgents()}
	ws := &fakeWorkspace{}
	ws.projectCreateInfo = ports.WorkspaceProjectInfo{
		Root: ports.WorkspaceInfo{Path: "/ws/mer-1", Branch: "b", BaseRef: "main", SessionID: "mer-1", ProjectID: "mer"},
		Worktrees: []ports.WorkspaceRepoInfo{
			{RepoName: "root", RepoPath: "/repo/mer", Path: "/ws/mer-1/root", Branch: "b", BaseSHA: "sha-root", BaseRef: "main", SessionID: "mer-1", ProjectID: "mer"},
			{RepoName: "api", RepoPath: "/repo/mer/api", Path: "/ws/mer-1/api", Branch: "b", BaseSHA: "sha-api", BaseRef: "main", SessionID: "mer-1", ProjectID: "mer"},
		},
	}
	return project, ws
}

// The custody record sealed inside the boundary must observe the canonical
// order of the same durable session_worktrees rows, never a second copy.
func assertBoundaryInventoryMatchesStore(t *testing.T, cbWorktrees, rows []domain.SessionWorktreeRecord) {
	t.Helper()
	if len(cbWorktrees) != 2 || cbWorktrees[0].RepoName != "api" || cbWorktrees[1].RepoName != "root" {
		t.Fatalf("boundary inventory = %#v, want canonical api,root", cbWorktrees)
	}
	if !reflect.DeepEqual(cbWorktrees, rows) {
		t.Fatalf("boundary inventory = %#v, durable rows = %#v: the seal must read the same rows", cbWorktrees, rows)
	}
	for _, row := range cbWorktrees {
		if row.BaseSHA == "" {
			t.Fatalf("boundary inventory row %q lost its base", row.RepoName)
		}
	}
}

// A governed workspace-project spawn hands the durable per-repo inventory to
// the prelaunch callback, sourced from the session_worktrees rows written
// during workspace preparation.
func TestSpawn_GovernedWorkspaceFactsBoundBeforeProviderLaunch(t *testing.T) {
	st := newFakeStore()
	project, ws := workspaceProjectFixture()
	st.projects["mer"] = project
	st.workspaceRepo["mer"] = []domain.WorkspaceRepoRecord{{Name: "api", RelativePath: "api"}}
	runtime := &fakeRuntime{}
	m := New(Deps{Runtime: runtime, Agents: singleAgent{agent: &recordingAgent{}}, Workspace: ws, Store: st, Messenger: &fakeMessenger{}, Lifecycle: &fakeLCM{store: st}, LookPath: func(string) (string, error) { return "/bin/true", nil }})
	binding := domain.ExecutionBinding{Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault}

	var cbWorktrees []domain.SessionWorktreeRecord
	cbRuntimeCreated := -1
	rec, _, _, err := m.Spawn(ctx, ports.SpawnConfig{
		ProjectID: "mer", Kind: domain.KindWorker, ExactExecutionBinding: &binding,
		ExecutionPolicy: governedDiffBasePolicy(),
		BeforeProviderLaunch: func(cbCtx context.Context, cbRec domain.SessionRecord, _ domain.AttemptExecutionPolicy, worktrees []domain.SessionWorktreeRecord) error {
			cbWorktrees = worktrees
			cbRuntimeCreated = runtime.created
			rows, err := st.ListSessionWorktrees(cbCtx, cbRec.ID)
			if err != nil {
				t.Fatalf("read durable worktree rows inside the boundary: %v", err)
			}
			assertBoundaryInventoryMatchesStore(t, worktrees, rows)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if cbRuntimeCreated != 0 {
		t.Fatalf("runtime had already created %d sessions inside the boundary; the provider must not start first", cbRuntimeCreated)
	}
	finalRows, err := st.ListSessionWorktrees(context.Background(), rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cbWorktrees, finalRows) {
		t.Fatalf("post-launch durable rows = %#v, want the boundary inventory %#v", finalRows, cbWorktrees)
	}
}

// The chat controller path hands the same durable inventory to the boundary
// before the controller starts.
func TestChatSpawn_GovernedWorkspaceFactsBoundBeforeControllerStart(t *testing.T) {
	st := newFakeStore()
	project, ws := workspaceProjectFixture()
	st.projects["mer"] = project
	st.workspaceRepo["mer"] = []domain.WorkspaceRepoRecord{{Name: "api", RelativePath: "api"}}
	launcher := &governedChatLauncher{&recordingLauncher{}}
	runtime := &fakeRuntime{}
	m := New(Deps{
		Runtime: runtime, Agents: singleAgent{agent: &recordingAgent{}}, Workspace: ws, Store: st,
		Messenger: &fakeMessenger{}, Chat: launcher, Lifecycle: &fakeLCM{store: st},
		LookPath: func(string) (string, error) { return "/bin/true", nil },
	})
	binding := domain.ExecutionBinding{Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault}

	var cbWorktrees []domain.SessionWorktreeRecord
	launcherStartsAtBoundary := -1
	rec, _, _, err := m.Spawn(ctx, ports.SpawnConfig{
		ProjectID: "mer", Kind: domain.KindWorker, RequestedMode: domain.SessionModeChat,
		ExactExecutionBinding: &binding, ExecutionPolicy: governedDiffBasePolicy(),
		BeforeProviderLaunch: func(cbCtx context.Context, cbRec domain.SessionRecord, _ domain.AttemptExecutionPolicy, worktrees []domain.SessionWorktreeRecord) error {
			cbWorktrees = worktrees
			launcherStartsAtBoundary = len(launcher.started)
			rows, err := st.ListSessionWorktrees(cbCtx, cbRec.ID)
			if err != nil {
				t.Fatalf("read durable worktree rows inside the boundary: %v", err)
			}
			assertBoundaryInventoryMatchesStore(t, worktrees, rows)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if launcherStartsAtBoundary != 0 {
		t.Fatalf("controller had already started %d sessions inside the boundary", launcherStartsAtBoundary)
	}
	finalRows, err := st.ListSessionWorktrees(context.Background(), rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cbWorktrees, finalRows) {
		t.Fatalf("post-launch durable rows = %#v, want the boundary inventory %#v", finalRows, cbWorktrees)
	}
}
