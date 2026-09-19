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

func wantWorkspaceFacts() []domain.SessionWorktreeFact {
	return []domain.SessionWorktreeFact{
		{RepoName: "api", BaseSHA: "sha-api", BaseRef: "main", WorktreePath: "/ws/mer-1/api"},
		{RepoName: "root", BaseSHA: "sha-root", BaseRef: "main", WorktreePath: "/ws/mer-1/root"},
	}
}

func assertFactsMatchWorktreeRows(t *testing.T, facts []domain.SessionWorktreeFact, rows []domain.SessionWorktreeRecord) {
	t.Helper()
	if len(rows) != len(facts) {
		t.Fatalf("worktree rows = %d, want %d sealed facts", len(rows), len(facts))
	}
	byName := map[string]domain.SessionWorktreeRecord{}
	for _, row := range rows {
		byName[row.RepoName] = row
	}
	for _, fact := range facts {
		row, ok := byName[fact.RepoName]
		if !ok {
			t.Fatalf("sealed repo %q has no durable session_worktrees row", fact.RepoName)
		}
		if row.BaseSHA != fact.BaseSHA || row.BaseRef != fact.BaseRef || row.WorktreePath != fact.WorktreePath {
			t.Fatalf("row for %q = %#v, sealed fact = %#v: custody must match the durable inventory", fact.RepoName, row, fact)
		}
	}
}

// A governed workspace-project spawn must carry the canonical per-repo
// inventory into the prelaunch SessionRecord: the custody record sealed
// inside the callback observes the exact same facts that are durable in
// session_worktrees, and the final session metadata binds them.
func TestSpawn_GovernedWorkspaceFactsBoundBeforeProviderLaunch(t *testing.T) {
	st := newFakeStore()
	project, ws := workspaceProjectFixture()
	st.projects["mer"] = project
	st.workspaceRepo["mer"] = []domain.WorkspaceRepoRecord{{Name: "api", RelativePath: "api"}}
	runtime := &fakeRuntime{}
	m := New(Deps{Runtime: runtime, Agents: singleAgent{agent: &recordingAgent{}}, Workspace: ws, Store: st, Messenger: &fakeMessenger{}, Lifecycle: &fakeLCM{store: st}, LookPath: func(string) (string, error) { return "/bin/true", nil }})
	binding := domain.ExecutionBinding{Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault}

	var cbFacts []domain.SessionWorktreeFact
	cbRuntimeCreated := -1
	rec, _, _, err := m.Spawn(ctx, ports.SpawnConfig{
		ProjectID: "mer", Kind: domain.KindWorker, ExactExecutionBinding: &binding,
		ExecutionPolicy: governedDiffBasePolicy(),
		BeforeProviderLaunch: func(cbCtx context.Context, cbRec domain.SessionRecord, _ domain.AttemptExecutionPolicy) error {
			cbFacts = cbRec.Metadata.Worktrees
			cbRuntimeCreated = runtime.created
			rows, err := st.ListSessionWorktrees(cbCtx, cbRec.ID)
			if err != nil {
				t.Fatalf("read durable worktree rows inside the boundary: %v", err)
			}
			assertFactsMatchWorktreeRows(t, cbRec.Metadata.Worktrees, rows)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if cbRuntimeCreated != 0 {
		t.Fatalf("runtime had already created %d sessions inside the boundary; the provider must not start first", cbRuntimeCreated)
	}
	if !reflect.DeepEqual(cbFacts, wantWorkspaceFacts()) {
		t.Fatalf("boundary-observed inventory = %#v, want canonical %#v", cbFacts, wantWorkspaceFacts())
	}
	if !reflect.DeepEqual(rec.Metadata.Worktrees, cbFacts) {
		t.Fatalf("final metadata inventory = %#v, want the boundary value %#v", rec.Metadata.Worktrees, cbFacts)
	}
	finalRows, err := st.ListSessionWorktrees(context.Background(), rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertFactsMatchWorktreeRows(t, rec.Metadata.Worktrees, finalRows)
}

// The chat controller path binds the same prelaunch inventory: the boundary
// observes it before the controller starts and the stored final metadata
// matches every durable session_worktrees row.
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

	var cbFacts []domain.SessionWorktreeFact
	launcherStartsAtBoundary := -1
	rec, _, _, err := m.Spawn(ctx, ports.SpawnConfig{
		ProjectID: "mer", Kind: domain.KindWorker, RequestedMode: domain.SessionModeChat,
		ExactExecutionBinding: &binding, ExecutionPolicy: governedDiffBasePolicy(),
		BeforeProviderLaunch: func(cbCtx context.Context, cbRec domain.SessionRecord, _ domain.AttemptExecutionPolicy) error {
			cbFacts = cbRec.Metadata.Worktrees
			launcherStartsAtBoundary = len(launcher.started)
			rows, err := st.ListSessionWorktrees(cbCtx, cbRec.ID)
			if err != nil {
				t.Fatalf("read durable worktree rows inside the boundary: %v", err)
			}
			assertFactsMatchWorktreeRows(t, cbRec.Metadata.Worktrees, rows)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if launcherStartsAtBoundary != 0 {
		t.Fatalf("controller had already started %d sessions inside the boundary", launcherStartsAtBoundary)
	}
	if !reflect.DeepEqual(cbFacts, wantWorkspaceFacts()) {
		t.Fatalf("boundary-observed inventory = %#v, want canonical %#v", cbFacts, wantWorkspaceFacts())
	}
	stored, found, err := st.GetSession(context.Background(), rec.ID)
	if err != nil || !found {
		t.Fatalf("final session row: found=%v err=%v", found, err)
	}
	if !reflect.DeepEqual(stored.Metadata.Worktrees, cbFacts) {
		t.Fatalf("stored final inventory = %#v, want the boundary value %#v", stored.Metadata.Worktrees, cbFacts)
	}
	finalRows, err := st.ListSessionWorktrees(context.Background(), rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertFactsMatchWorktreeRows(t, stored.Metadata.Worktrees, finalRows)
}
