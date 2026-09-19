package sessionmanager

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// launchOrderProvisioner records what the runtime had done by the time
// provisioning was asked for, so "before launch" is measured rather than
// assumed.
type launchOrderProvisioner struct {
	runtime *fakeRuntime
	err     error

	called                bool
	runtimeCreatesAtCall  int
	seenInputs            []ports.AttemptInputRef
	seenWorkspacePath     string
	seenBaseRevisionEmpty bool
}

func (p *launchOrderProvisioner) ProvisionAttemptInputs(_ context.Context, req ports.AttemptInputProvisionRequest) error {
	p.called = true
	p.runtimeCreatesAtCall = p.runtime.created
	p.seenInputs = req.Inputs
	p.seenWorkspacePath = req.WorkspacePath
	p.seenBaseRevisionEmpty = req.BaseRevision == ""
	return p.err
}

func launchInputs() []ports.AttemptInputRef {
	return []ports.AttemptInputRef{{AttemptID: "att-a", WorkUnitID: "wu-a", ArtifactVersion: "artifact-v1"}}
}

// TestSpawn_MaterializesInputsBeforeTheProviderIsLaunched is the launch-side
// half of artifact continuity. A successor whose inputs arrived after its
// provider started would have read the wrong tree, so the ordering is the
// guarantee — not that provisioning happens somewhere in the spawn.
func TestSpawn_MaterializesInputsBeforeTheProviderIsLaunched(t *testing.T) {
	m, _, runtime, _ := newManager()
	provisioner := &launchOrderProvisioner{runtime: runtime}
	m.SetAttemptInputProvisioner(provisioner)

	binding := domain.ExecutionBinding{Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault}
	if _, _, _, err := m.Spawn(context.Background(), ports.SpawnConfig{
		ProjectID: "mer", Kind: domain.KindWorker, ExactExecutionBinding: &binding,
		AttemptInputs: launchInputs(),
	}); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if !provisioner.called {
		t.Fatal("a successor with admitted inputs was spawned without provisioning them")
	}
	if provisioner.runtimeCreatesAtCall != 0 {
		t.Fatalf("the runtime had already created %d sessions when inputs were provisioned; the provider must not start first",
			provisioner.runtimeCreatesAtCall)
	}
	if runtime.created != 1 {
		t.Fatalf("runtime creations = %d, want the provider launched once after provisioning", runtime.created)
	}
	if len(provisioner.seenInputs) != 1 || provisioner.seenInputs[0] != launchInputs()[0] {
		t.Fatalf("inputs = %#v, want exactly the admitted reference", provisioner.seenInputs)
	}
	// The path comes from the workspace the daemon just created, never from a
	// caller.
	if provisioner.seenWorkspacePath == "" {
		t.Fatal("provisioning was given no daemon-owned workspace path")
	}
}

// TestSpawn_RefusesToLaunchWhenInputsCannotBeMaterialized is the fail-closed
// half: a corrupt or missing predecessor result must stop the provider from
// starting at all, not start it on a tree missing the work it builds on.
func TestSpawn_RefusesToLaunchWhenInputsCannotBeMaterialized(t *testing.T) {
	cases := []struct {
		name    string
		wire    func(*Manager, *fakeRuntime) *launchOrderProvisioner
		wantErr error
	}{
		{
			name: "a corrupt predecessor artifact",
			wire: func(m *Manager, runtime *fakeRuntime) *launchOrderProvisioner {
				p := &launchOrderProvisioner{
					runtime: runtime,
					err:     fmt.Errorf("%w: artifact blob digest mismatch", ports.ErrAttemptInputProvisioning),
				}
				m.SetAttemptInputProvisioner(p)
				return p
			},
			wantErr: ports.ErrAttemptInputProvisioning,
		},
		{
			name: "a daemon with no artifact handoff wired",
			wire: func(*Manager, *fakeRuntime) *launchOrderProvisioner {
				// Deliberately wire nothing: an absent provisioner is a
				// refusal, never a silent skip.
				return nil
			},
			wantErr: ports.ErrAttemptInputProvisioning,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, _, runtime, _ := newManager()
			provisioner := tc.wire(m, runtime)

			binding := domain.ExecutionBinding{Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault}
			_, _, _, err := m.Spawn(context.Background(), ports.SpawnConfig{
				ProjectID: "mer", Kind: domain.KindWorker, ExactExecutionBinding: &binding,
				AttemptInputs: launchInputs(),
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Spawn err = %v, want a provisioning refusal", err)
			}
			if runtime.created != 0 {
				t.Fatalf("the provider was launched %d times despite unusable inputs", runtime.created)
			}
			if provisioner != nil && !provisioner.called {
				t.Fatal("provisioning was never attempted")
			}
		})
	}
}

// TestSpawn_WithoutAdmittedInputsDoesNotConsultTheProvisioner keeps the seam
// off the ordinary path: a WorkUnit with no dependencies has nothing to
// receive, and a session spawned outside governed execution has none either.
func TestSpawn_WithoutAdmittedInputsDoesNotConsultTheProvisioner(t *testing.T) {
	m, _, runtime, _ := newManager()
	provisioner := &launchOrderProvisioner{
		runtime: runtime,
		err:     fmt.Errorf("%w: must not be consulted", ports.ErrAttemptInputProvisioning),
	}
	m.SetAttemptInputProvisioner(provisioner)

	binding := domain.ExecutionBinding{Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault}
	if _, _, _, err := m.Spawn(context.Background(), ports.SpawnConfig{
		ProjectID: "mer", Kind: domain.KindWorker, ExactExecutionBinding: &binding,
	}); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if provisioner.called {
		t.Fatal("a spawn with no admitted inputs consulted the provisioner")
	}
	if runtime.created != 1 {
		t.Fatalf("runtime creations = %d, want the ordinary launch to proceed", runtime.created)
	}
}

func TestSpawn_PersistsBoundLaunchPacketBeforeProviderLaunch(t *testing.T) {
	st := newFakeStore()
	repo := newManagerGitRepo(t)
	cfg := testRoleAgents()
	cfg.DefaultBranch = "main"
	st.projects["mer"] = domain.ProjectRecord{ID: "mer", Path: repo, Config: cfg}
	runtime := &fakeRuntime{}
	agent := &recordingAgent{}
	ws := &fakeWorkspace{}
	ws.path = repo
	m := New(Deps{Runtime: runtime, Agents: singleAgent{agent: agent}, Workspace: ws, Store: st, Messenger: &fakeMessenger{}, Lifecycle: &fakeLCM{store: st}, LookPath: func(string) (string, error) { return "/bin/true", nil }})
	binding := domain.ExecutionBinding{Provider: domain.HarnessCodex, ModelSelection: domain.ExecutionBindingModelProviderDefault}
	policy := &domain.AttemptExecutionPolicy{OutcomeID: "out", PlanRevisionID: "plan", WorkUnitID: "wu", ContractRevisionNumber: 1, RunBriefCoreDigest: "brief", RequiredCapabilities: []string{domain.CapabilityWorktreeRead}, Grants: []domain.CapabilityGrant{{ID: "read", Name: domain.CapabilityWorktreeRead, Scope: "worktree/*"}}}
	called := false
	_, _, _, err := m.Spawn(context.Background(), ports.SpawnConfig{ProjectID: "mer", Kind: domain.KindWorker, ExactExecutionBinding: &binding, ExecutionPolicy: policy, BeforeProviderLaunch: func(_ context.Context, rec domain.SessionRecord, bound domain.AttemptExecutionPolicy, _ []domain.SessionWorktreeRecord) error {
		called = true
		if rec.ID == "" || bound.WorkspaceRoot == "" {
			t.Fatalf("callback missing prepared identity/root: rec=%+v policy=%+v", rec, bound)
		}
		if runtime.created != 0 {
			t.Fatalf("provider launched before packet persistence")
		}
		return fmt.Errorf("simulated durable write failure")
	}})
	if err == nil || !called {
		t.Fatalf("spawn err=%v called=%v", err, called)
	}
	if runtime.created != 0 {
		t.Fatalf("provider launched despite persistence failure")
	}
}
