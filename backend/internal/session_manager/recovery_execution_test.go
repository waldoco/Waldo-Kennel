package sessionmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

type recoveryEvidenceFakeStore struct {
	*fakeStore
	ref        domain.AttemptSessionRef
	found      bool
	err        error
	attempt    *domain.Attempt
	attemptErr error
}

func (s *recoveryEvidenceFakeStore) LatestAttemptSessionRefForSession(context.Context, string) (domain.AttemptSessionRef, bool, error) {
	return s.ref, s.found, s.err
}

func (s *recoveryEvidenceFakeStore) GetAttempt(_ context.Context, outcomeID domain.OutcomeID, attemptID domain.AttemptID) (domain.Attempt, bool, error) {
	if s.attemptErr != nil {
		return domain.Attempt{}, false, s.attemptErr
	}
	if s.attempt != nil {
		return *s.attempt, true, nil
	}
	// Existing recovery tests model an admitted Attempt that is still running.
	return domain.Attempt{ID: attemptID, OutcomeID: outcomeID, Status: domain.AttemptRunning}, true, nil
}

func recoveryPolicy(t *testing.T) (domain.AttemptExecutionPolicy, string) {
	t.Helper()
	policy := domain.AttemptExecutionPolicy{
		OutcomeID:              "out-1",
		PlanRevisionID:         "plan-1",
		WorkUnitID:             "wu-1",
		ContractRevisionNumber: 1,
		RunBriefCoreDigest:     strings.Repeat("a", 64),
		RequiredCapabilities:   []string{domain.CapabilityWorktreeRead},
		Grants: []domain.CapabilityGrant{{
			ID: "grant-read", Name: domain.CapabilityWorktreeRead, Scope: "worktree/*",
		}},
	}
	var err error
	policy, err = policy.BindWorkspaceRoot("/ws/mer-1")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := policy.Digest()
	if err != nil {
		t.Fatal(err)
	}
	return policy, digest
}

func recoveryRef(t *testing.T, sessionID domain.SessionID, mode domain.SessionMode, policy domain.AttemptExecutionPolicy, policyDigest string) domain.AttemptSessionRef {
	t.Helper()
	snapshot, err := json.Marshal(map[string]any{
		"snapshotVersion":        domain.AdmissionSnapshotVersion,
		"harness":                string(domain.HarnessCodex),
		"modelSelection":         string(domain.ExecutionBindingModelExplicit),
		"requestedModel":         "approved-model",
		"effectiveModel":         "approved-model",
		"workUnitId":             string(policy.WorkUnitID),
		"mode":                   string(mode),
		"runBriefCoreDigest":     policy.RunBriefCoreDigest,
		"runBriefCompiledDigest": strings.Repeat("b", 64),
		"executionPolicy":        policy,
		"executionPolicyDigest":  policyDigest,
		"sessionId":              string(sessionID),
	})
	if err != nil {
		t.Fatal(err)
	}
	return domain.AttemptSessionRef{
		ID: "asr-1", AttemptID: "att-1", Seq: 1, SessionID: string(sessionID),
		Harness: domain.HarnessCodex, Mode: mode,
		RunBriefCoreDigest: policy.RunBriefCoreDigest, RunBriefCompiledDigest: strings.Repeat("b", 64),
		AdmissionSnapshot: string(snapshot),
	}
}

func TestLoadRecoveryExecutionRejectsMissingGovernedEvidence(t *testing.T) {
	policy, digest := recoveryPolicy(t)
	_ = policy
	store := newFakeStore()
	store.sessions["mer-1"] = domain.SessionRecord{
		ID: "mer-1", ProjectID: "mer", Harness: domain.HarnessCodex,
		Metadata: domain.SessionMetadata{GovernedExecutionPolicyDigest: digest},
	}
	mgr := &Manager{store: store}

	if _, err := mgr.loadRecoveryExecution(context.Background(), store.sessions["mer-1"]); err == nil || !strings.Contains(err.Error(), "evidence unavailable") {
		t.Fatalf("loadRecoveryExecution error = %v, want missing evidence to block", err)
	}
}

func TestLoadRecoveryExecutionRejectsInvalidGovernedEvidence(t *testing.T) {
	policy, digest := recoveryPolicy(t)
	base := newFakeStore()
	base.sessions["mer-1"] = domain.SessionRecord{
		ID: "mer-1", ProjectID: "mer", Harness: domain.HarnessCodex,
		Metadata: domain.SessionMetadata{WorkspacePath: "/ws/mer-1", GovernedExecutionPolicyDigest: digest},
	}
	store := &recoveryEvidenceFakeStore{
		fakeStore: base,
		ref:       recoveryRef(t, "mer-1", domain.SessionModeTUI, policy, "not-the-policy-digest"),
		found:     true,
	}
	mgr := &Manager{store: store}

	if _, err := mgr.loadRecoveryExecution(context.Background(), base.sessions["mer-1"]); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("loadRecoveryExecution error = %v, want invalid evidence to block", err)
	}
}

func TestLoadRecoveryExecutionLeavesHistoricalSnapshotReadable(t *testing.T) {
	base := newFakeStore()
	rec := domain.SessionRecord{ID: "mer-1", ProjectID: "mer", Harness: domain.HarnessCodex}
	base.sessions[rec.ID] = rec
	store := &recoveryEvidenceFakeStore{
		fakeStore: base,
		ref: domain.AttemptSessionRef{
			ID: "asr-legacy", AttemptID: "att-legacy", Seq: 1, SessionID: string(rec.ID),
			Harness: domain.HarnessCodex, Mode: domain.SessionModeTUI,
			RunBriefCoreDigest: strings.Repeat("a", 64), RunBriefCompiledDigest: strings.Repeat("b", 64),
			AdmissionSnapshot: `{"snapshotVersion":1,"harness":"codex"}`,
		},
		found: true,
	}
	mgr := &Manager{store: store}

	if execution, err := mgr.loadRecoveryExecution(context.Background(), rec); err != nil || execution != nil {
		t.Fatalf("loadRecoveryExecution = (%v, %v), want legacy snapshot readable without synthesized policy", execution, err)
	}
}

func TestRestoreGovernedSessionRejectsTerminalAttemptBeforeWorkspaceOrRuntimeMutation(t *testing.T) {
	policy, digest := recoveryPolicy(t)
	base := newFakeStore()
	base.projects["mer"] = domain.ProjectRecord{ID: "mer", Config: testRoleAgents()}
	base.sessions["mer-1"] = domain.SessionRecord{
		ID: "mer-1", ProjectID: "mer", Kind: domain.KindWorker, Harness: domain.HarnessCodex,
		IsTerminated: true,
		Metadata: domain.SessionMetadata{
			WorkspacePath: "/ws/mer-1", Branch: "kennel/mer-1", AgentSessionID: "native-1",
			GovernedExecutionPolicyDigest: digest,
		},
	}
	store := &recoveryEvidenceFakeStore{
		fakeStore: base,
		ref:       recoveryRef(t, "mer-1", domain.SessionModeTUI, policy, digest),
		found:     true,
		attempt: &domain.Attempt{
			ID: "att-1", OutcomeID: policy.OutcomeID, PlanRevisionID: policy.PlanRevisionID,
			WorkUnitID: policy.WorkUnitID, Number: 1, Status: domain.AttemptSucceeded,
		},
	}
	workspace := &fakeWorkspace{}
	runtime := &fakeRuntime{}
	mgr := New(Deps{
		Runtime: runtime, Agents: singleAgent{agent: &recordingAgent{}}, Workspace: workspace,
		Store: store, Messenger: &fakeMessenger{}, Lifecycle: &fakeLCM{store: base},
		LookPath: func(string) (string, error) { return "/bin/true", nil },
	})

	_, err := mgr.RestoreWithMode(context.Background(), "mer-1")
	if !errors.Is(err, ErrGovernedAttemptClosed) {
		t.Fatalf("RestoreWithMode error = %v, want ErrGovernedAttemptClosed", err)
	}
	if len(workspace.restoreConfigs) != 0 || runtime.created != 0 {
		t.Fatalf("terminal Attempt restore mutated execution boundary: workspace restores=%d runtime creates=%d", len(workspace.restoreConfigs), runtime.created)
	}
}

func TestRestoreAllSkipsTerminalGovernedAttemptBeforeWorkspaceOrRuntimeMutation(t *testing.T) {
	policy, digest := recoveryPolicy(t)
	base := newFakeStore()
	base.projects["mer"] = domain.ProjectRecord{ID: "mer", Config: testRoleAgents()}
	base.sessions["mer-1"] = domain.SessionRecord{
		ID: "mer-1", ProjectID: "mer", Kind: domain.KindWorker, Harness: domain.HarnessCodex,
		IsTerminated: true,
		Metadata: domain.SessionMetadata{
			WorkspacePath: "/ws/mer-1", Branch: "kennel/mer-1", AgentSessionID: "native-1",
			GovernedExecutionPolicyDigest: digest,
		},
	}
	base.worktrees["mer-1"] = []domain.SessionWorktreeRecord{{
		SessionID: "mer-1", RepoName: domain.RootWorkspaceRepoName,
		Branch: "kennel/mer-1", WorktreePath: "/ws/mer-1",
	}}
	store := &recoveryEvidenceFakeStore{
		fakeStore: base,
		ref:       recoveryRef(t, "mer-1", domain.SessionModeTUI, policy, digest),
		found:     true,
		attempt: &domain.Attempt{
			ID: "att-1", OutcomeID: policy.OutcomeID, PlanRevisionID: policy.PlanRevisionID,
			WorkUnitID: policy.WorkUnitID, Number: 1, Status: domain.AttemptSucceeded,
		},
	}
	workspace := &fakeWorkspace{}
	runtime := &fakeRuntime{}
	mgr := New(Deps{
		Runtime: runtime, Agents: singleAgent{agent: &recordingAgent{}}, Workspace: workspace,
		Store: store, Messenger: &fakeMessenger{}, Lifecycle: &fakeLCM{store: base},
		LookPath: func(string) (string, error) { return "/bin/true", nil },
	})

	if err := mgr.RestoreAll(context.Background()); err != nil {
		t.Fatalf("RestoreAll: %v", err)
	}
	if len(workspace.restoreConfigs) != 0 || runtime.created != 0 {
		t.Fatalf("startup restored terminal Attempt execution: workspace restores=%d runtime creates=%d", len(workspace.restoreConfigs), runtime.created)
	}
	if rows := base.worktrees["mer-1"]; len(rows) != 0 {
		t.Fatalf("terminal Attempt retained stale shutdown marker: %+v", rows)
	}
}

func TestResumeGovernedSessionRejectsTerminalAttemptBeforeRuntimeMutation(t *testing.T) {
	policy, digest := recoveryPolicy(t)
	base := newFakeStore()
	base.projects["mer"] = domain.ProjectRecord{ID: "mer", Config: testRoleAgents()}
	base.sessions["mer-1"] = domain.SessionRecord{
		ID: "mer-1", ProjectID: "mer", Kind: domain.KindWorker, Harness: domain.HarnessCodex,
		Activity: domain.Activity{State: domain.ActivityExited},
		Metadata: domain.SessionMetadata{
			WorkspacePath: "/ws/mer-1", Branch: "kennel/mer-1", RuntimeHandleID: "h1", AgentSessionID: "native-1",
			GovernedExecutionPolicyDigest: digest,
		},
	}
	store := &recoveryEvidenceFakeStore{
		fakeStore: base,
		ref:       recoveryRef(t, "mer-1", domain.SessionModeTUI, policy, digest),
		found:     true,
		attempt: &domain.Attempt{
			ID: "att-1", OutcomeID: policy.OutcomeID, PlanRevisionID: policy.PlanRevisionID,
			WorkUnitID: policy.WorkUnitID, Number: 1, Status: domain.AttemptReconciled,
		},
	}
	runtime := &fakeRuntime{aliveByHandle: map[string]bool{"h1": true}}
	mgr := New(Deps{
		Runtime: runtime, Agents: singleAgent{agent: &recordingAgent{}}, Workspace: &fakeWorkspace{},
		Store: store, Messenger: &fakeMessenger{}, Lifecycle: &fakeLCM{store: base},
		LookPath: func(string) (string, error) { return "/bin/true", nil },
	})

	_, err := mgr.ResumeAgentWithMode(context.Background(), "mer-1")
	if !errors.Is(err, ErrGovernedAttemptClosed) {
		t.Fatalf("ResumeAgentWithMode error = %v, want ErrGovernedAttemptClosed", err)
	}
	if runtime.created != 0 {
		t.Fatalf("terminal Attempt resume created %d runtimes, want 0", runtime.created)
	}
}

func TestResumeGovernedTUIUsesAdmissionBindingAfterProjectChange(t *testing.T) {
	policy, digest := recoveryPolicy(t)
	base := newFakeStore()
	base.projects["mer"] = domain.ProjectRecord{ID: "mer", Config: domain.ProjectConfig{
		Worker: domain.RoleOverride{
			Harness:     domain.HarnessCodex,
			AgentConfig: domain.AgentConfig{Model: "broader-project-model", Permissions: domain.PermissionModeBypassPermissions},
		},
	}}
	base.sessions["mer-1"] = domain.SessionRecord{
		ID: "mer-1", ProjectID: "mer", Kind: domain.KindWorker, Harness: domain.HarnessCodex,
		Activity: domain.Activity{State: domain.ActivityExited},
		Metadata: domain.SessionMetadata{
			WorkspacePath: "/ws/mer-1", Branch: "kennel/mer-1", RuntimeHandleID: "h1", AgentSessionID: "native-1",
			GovernedExecutionPolicyDigest: digest,
		},
	}
	store := &recoveryEvidenceFakeStore{fakeStore: base, ref: recoveryRef(t, "mer-1", domain.SessionModeTUI, policy, digest), found: true}
	agent := &recordingAgent{}
	rt := &fakeRuntime{aliveByHandle: map[string]bool{"h1": true}}
	var logBuf bytes.Buffer
	mgr := New(Deps{Runtime: rt, Agents: singleAgent{agent: agent}, Workspace: &fakeWorkspace{}, Store: store, Messenger: &fakeMessenger{}, Lifecycle: &fakeLCM{store: base}, LookPath: func(string) (string, error) { return "/bin/true", nil }, Logger: slog.New(slog.NewTextHandler(&logBuf, nil))})

	if _, err := mgr.ResumeAgentWithMode(context.Background(), "mer-1"); err != nil {
		t.Fatalf("ResumeAgentWithMode: %v", err)
	}
	if agent.lastRestore.ExecutionPolicy == nil {
		t.Fatalf("restore policy = nil, want durable Attempt policy")
	}
	gotDigest, err := agent.lastRestore.ExecutionPolicy.Digest()
	if err != nil || gotDigest != digest {
		t.Fatalf("restore policy digest = %q, want %q (err=%v)", gotDigest, digest, err)
	}
	if agent.lastRestore.Config.Model != "approved-model" {
		t.Fatalf("restore model = %q, want approved-model", agent.lastRestore.Config.Model)
	}
	if agent.lastRestore.Config.Permissions == domain.PermissionModeBypassPermissions {
		t.Fatal("restore inherited broader Project bypass permissions")
	}
	// The governed restore/relaunch call site must attest too: a daemon
	// restart re-resolves the (possibly re-pinned) binary, and that identity
	// change is exactly what the attestation exists to make loud.
	if out := logBuf.String(); !strings.Contains(out, "governed launch binary") || !strings.Contains(out, "mer-1") || !strings.Contains(out, "version_error") {
		t.Fatalf("governed restore attestation missing from log: %q", out)
	}
}

func TestResumeGovernedChatUsesAdmissionBindingAfterProjectChange(t *testing.T) {
	policy, digest := recoveryPolicy(t)
	base := newFakeStore()
	base.projects["mer"] = domain.ProjectRecord{ID: "mer", Config: domain.ProjectConfig{
		Worker: domain.RoleOverride{
			Harness:     domain.HarnessCodex,
			AgentConfig: domain.AgentConfig{Model: "broader-project-model", Permissions: domain.PermissionModeBypassPermissions},
		},
	}}
	base.sessions["mer-1"] = domain.SessionRecord{
		ID: "mer-1", ProjectID: "mer", Kind: domain.KindWorker, Harness: domain.HarnessCodex,
		Mode: domain.SessionModeChat, Activity: domain.Activity{State: domain.ActivityExited},
		Metadata: domain.SessionMetadata{
			WorkspacePath: "/ws/mer-1", Branch: "kennel/mer-1", ProviderConversationID: "thread-1",
			GovernedExecutionPolicyDigest: digest,
		},
	}
	store := &recoveryEvidenceFakeStore{fakeStore: base, ref: recoveryRef(t, "mer-1", domain.SessionModeChat, policy, digest), found: true}
	launcher := &recordingLauncher{}
	mgr := New(Deps{Runtime: &fakeRuntime{}, Agents: fakeAgents{}, Workspace: &fakeWorkspace{}, Store: store, Messenger: &fakeMessenger{}, Chat: launcher, Lifecycle: &fakeLCM{store: base}, DataDir: "/kennel-test-data"})

	if _, err := mgr.ResumeAgentWithMode(context.Background(), "mer-1"); err != nil {
		t.Fatalf("ResumeAgentWithMode: %v", err)
	}
	if len(launcher.started) != 1 {
		t.Fatalf("started %d chat controllers, want 1", len(launcher.started))
	}
	started := launcher.started[0]
	if started.Model != "approved-model" {
		t.Fatalf("chat resume model = %q, want approved-model", started.Model)
	}
	if started.Permissions == domain.PermissionModeBypassPermissions {
		t.Fatal("chat resume inherited broader Project bypass permissions")
	}
	if started.ExecutionPolicy == nil {
		t.Fatal("chat resume policy = nil, want durable Attempt policy")
	}
	gotDigest, err := started.ExecutionPolicy.Digest()
	if err != nil || gotDigest != digest {
		t.Fatalf("chat resume policy digest = %q, want %q (err=%v)", gotDigest, digest, err)
	}
}
