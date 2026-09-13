package governedtools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

func testPolicy(write, exec bool) domain.AttemptExecutionPolicy {
	names := []string{domain.CapabilityWorktreeRead}
	grants := []domain.CapabilityGrant{{ID: "read", Name: domain.CapabilityWorktreeRead, Scope: "worktree/*"}}
	var checks []domain.ApprovedCheck
	if write {
		names = append(names, domain.CapabilityWorktreeWrite)
		grants = append(grants, domain.CapabilityGrant{ID: "write", Name: domain.CapabilityWorktreeWrite, Scope: "worktree/*"})
	}
	if exec {
		names = []string{domain.CapabilityWorktreeExec, domain.CapabilityWorktreeRead, domain.CapabilityWorktreeWrite}
		grants = []domain.CapabilityGrant{{ID: "exec", Name: domain.CapabilityWorktreeExec, Scope: "worktree/*"}, {ID: "read", Name: domain.CapabilityWorktreeRead, Scope: "worktree/*"}, {ID: "write", Name: domain.CapabilityWorktreeWrite, Scope: "worktree/*"}}
		checks = []domain.ApprovedCheck{{ID: "check-1", CriterionID: "criterion-1", Argv: []string{"go", "test", "./..."}, TimeoutSeconds: 30}}
	}
	return domain.AttemptExecutionPolicy{OutcomeID: "out-1", PlanRevisionID: "plan-1", WorkUnitID: "wu-1", ContractRevisionNumber: 1, RunBriefCoreDigest: "brief", RequiredCapabilities: names, Grants: grants, ApprovedChecks: checks}
}

func testServer(t *testing.T, root string, write, exec bool) Server {
	t.Helper()
	policy, err := testPolicy(write, exec).BindWorkspaceRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = opened.Close() })
	server := Server{Policy: policy, WorkspaceRoot: root, root: opened}
	return server
}

func TestRepositoryToolsEnforceReadWriteAndLeaseBoundary(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source.txt"), []byte("evidence"), 0o644); err != nil {
		t.Fatal(err)
	}
	readOnly := testServer(t, root, false, false)
	if got, err := readOnly.call("read_text_file", map[string]interface{}{"path": "source.txt"}); err != nil || got != "evidence" {
		t.Fatalf("read = %q, %v", got, err)
	}
	if _, err := readOnly.call("write_text_file", map[string]interface{}{"path": "report.md", "content": "no"}); err == nil {
		t.Fatal("read-only policy allowed write")
	}
	if _, err := readOnly.call("read_text_file", map[string]interface{}{"path": "../outside"}); err == nil {
		t.Fatal("reader escaped workspace")
	}
	writer := testServer(t, root, true, false)
	if _, err := writer.call("write_text_file", map[string]interface{}{"path": "report.md", "content": "bounded"}); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(root, "report.md")); err != nil || string(b) != "bounded" {
		t.Fatalf("report = %q, %v", b, err)
	}
}

func TestApprovedChecksAreNotExposedToProviderMCP(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := testServer(t, root, true, true)
	for _, advertised := range server.tools() {
		if advertised["name"] == "run_approved_check" {
			t.Fatal("provider MCP advertised daemon-owned approved check executor")
		}
	}
	if _, err := server.call("run_approved_check", map[string]interface{}{"check_id": "check-1"}); err == nil {
		t.Fatal("provider MCP invoked daemon-owned approved check executor")
	}
}

func TestRepositoryToolsProtectGitCustodyAndSymlinkBoundary(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	server := testServer(t, root, true, false)
	for _, path := range []string{".git", ".git/config", "nested/.git/config", ".GIT/config", "nested/.GiT/config"} {
		if _, err := server.call("write_text_file", map[string]interface{}{"path": path, "content": "corrupt"}); err == nil {
			t.Fatalf("write to custody path %q succeeded", path)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := server.call("write_text_file", map[string]interface{}{"path": "escape/pwned", "content": "no"}); err == nil {
		t.Fatal("write followed an escaping directory symlink")
	}
	if _, err := os.Stat(filepath.Join(outside, "pwned")); !os.IsNotExist(err) {
		t.Fatalf("outside file exists after refused write: %v", err)
	}
}

func TestRepositoryToolsDoNotWidenWriteOnlyPolicyIntoRead(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	policy, err := (domain.AttemptExecutionPolicy{
		OutcomeID: "out-1", PlanRevisionID: "plan-1", WorkUnitID: "wu-1", ContractRevisionNumber: 1,
		RunBriefCoreDigest: "brief", RequiredCapabilities: []string{domain.CapabilityWorktreeWrite},
		Grants: []domain.CapabilityGrant{{ID: "write", Name: domain.CapabilityWorktreeWrite, Scope: "worktree/*"}},
	}).BindWorkspaceRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = opened.Close() })
	server := Server{Policy: policy, WorkspaceRoot: root, root: opened}
	for _, advertised := range server.tools() {
		name, _ := advertised["name"].(string)
		if name == "list_repository" || name == "read_text_file" {
			t.Fatalf("write-only policy advertised %q", name)
		}
	}
	if _, err := server.call("read_text_file", map[string]interface{}{"path": "source.txt"}); err == nil {
		t.Fatal("write-only policy allowed direct read invocation")
	}
	if _, err := server.call("write_text_file", map[string]interface{}{"path": "report.md", "content": "bounded"}); err != nil {
		t.Fatalf("write-only policy lost approved write: %v", err)
	}
}

func TestRepositoryWriteResistsConcurrentDirectorySymlinkSwap(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	server := testServer(t, root, true, false)
	dir := filepath.Join(root, "changing")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 300; i++ {
			_ = os.RemoveAll(dir)
			_ = os.Symlink(outside, dir)
			_ = os.Remove(dir)
			_ = os.Mkdir(dir, 0o755)
		}
	}()
	for i := 0; i < 300; i++ {
		_, _ = server.call("write_text_file", map[string]interface{}{"path": "changing/pwned", "content": "bounded"})
	}
	<-done
	if _, err := os.Stat(filepath.Join(outside, "pwned")); !os.IsNotExist(err) {
		t.Fatalf("symlink swap redirected write outside root: %v", err)
	}
}

func TestRepositoryWritePreservesExistingExecutableMode(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "verify.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "--quiet"}, {"add", "verify.sh"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	server := testServer(t, root, true, false)
	if _, err := server.call("write_text_file", map[string]interface{}{
		"path": "verify.sh", "content": "#!/bin/sh\nexit 0\n",
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("mode after atomic overwrite = %04o, want 0755", got)
	}
	cmd := exec.Command("git", "diff", "--summary", "--", "verify.sh")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git diff mode check: %v: %s", err, output)
	} else if strings.Contains(string(output), "mode change") {
		t.Fatalf("atomic overwrite changed Git executable mode: %s", output)
	}
}
