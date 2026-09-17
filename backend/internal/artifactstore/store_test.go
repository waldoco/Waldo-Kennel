package artifactstore

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

func TestStoreRetainStagedFolderPublishesBytesAndMode(t *testing.T) {
	data := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "staged")
	if err := os.MkdirAll(workspace, 0o750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace, "result.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho retained\n"), 0o750); err != nil {
		t.Fatal(err)
	}
	s, err := New(Config{Root: filepath.Join(data, "artifacts")})
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Retain(context.Background(), Input{AttemptID: "attempt-1", OutcomeID: "outcome-1", PlanRevisionID: "plan-1", WorkUnitID: "unit-1", ContractRevisionNumber: 1, WorkspaceKind: domain.WorkspaceStagedFolder, WorkspacePath: workspace})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Receipt.RetentionState.Complete() || len(result.Receipt.Files) != 1 {
		t.Fatalf("receipt = %#v", result.Receipt)
	}
	if err := os.RemoveAll(workspace); err != nil {
		t.Fatal(err)
	}
	content, mode, err := s.Read(context.Background(), result.Receipt, result.Receipt.Files[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "#!/bin/sh\necho retained\n" || mode.Perm() != 0o750 {
		t.Fatalf("content/mode = %q/%o", content, mode.Perm())
	}
}

func TestStoreRetainGitIncludesCommittedAndWorkingTreeOutput(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	git(t, repo, "config", "user.email", "test@example.com")
	git(t, repo, "config", "user.name", "Kennel Test")
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("committed.txt", "base\n")
	write("deleted.txt", "remove me\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-qm", "base")
	base := git(t, repo, "rev-parse", "HEAD")
	write("committed.txt", "committed result\n")
	git(t, repo, "add", "committed.txt")
	git(t, repo, "commit", "-qm", "result")
	write("dirty.txt", "dirty result\n")
	if err := os.Remove(filepath.Join(repo, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	s, err := New(Config{Root: filepath.Join(t.TempDir(), "artifacts")})
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Retain(context.Background(), Input{AttemptID: "attempt-git", OutcomeID: "outcome-1", PlanRevisionID: "plan-1", WorkUnitID: "unit-1", ContractRevisionNumber: 1, WorkspaceKind: domain.WorkspaceGitWorktree, WorkspacePath: repo, BaseRevision: base})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Receipt.RetentionState.Complete() || len(result.Receipt.Files) != 3 {
		t.Fatalf("receipt state/files = %s/%d detail=%s", result.Receipt.RetentionState, len(result.Receipt.Files), result.Receipt.RetentionDetail)
	}
	seen := map[string]bool{}
	for _, file := range result.Receipt.Files {
		seen[file.RelativePath] = true
		if file.Additions == nil || file.Deletions == nil {
			t.Fatalf("git change %s was not measured: %+v", file.RelativePath, file)
		}
		if file.ChangeKind == domain.ArtifactDeleted {
			continue
		}
		body, _, readErr := s.Read(context.Background(), result.Receipt, file)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if file.RelativePath == "committed.txt" && string(body) != "committed result\n" {
			t.Fatalf("committed body = %q", body)
		}
	}
	for _, name := range []string{"committed.txt", "dirty.txt", "deleted.txt"} {
		if !seen[name] {
			t.Fatalf("missing %s in %#v", name, seen)
		}
	}
}

func TestStoreRefusesSecretAndSymlinkAsIncomplete(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, ".env"), []byte("TOKEN=do-not-copy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".env", filepath.Join(workspace, "link")); err != nil {
		t.Fatal(err)
	}
	s, err := New(Config{Root: filepath.Join(t.TempDir(), "artifacts")})
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Retain(context.Background(), Input{AttemptID: "attempt-unsupported", OutcomeID: "outcome-1", PlanRevisionID: "plan-1", WorkUnitID: "unit-1", ContractRevisionNumber: 1, WorkspaceKind: domain.WorkspaceStagedFolder, WorkspacePath: workspace})
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.RetentionState != domain.RetentionUnsupported {
		t.Fatalf("state = %s", result.Receipt.RetentionState)
	}
	if _, _, err := s.Read(context.Background(), result.Receipt, result.Receipt.Files[0]); err == nil {
		t.Fatal("unsupported receipt was readable")
	}
}

func TestStoreComposeRejectsConflictAndAppliesExactBytes(t *testing.T) {
	store, err := New(Config{Root: filepath.Join(t.TempDir(), "artifacts")})
	if err != nil {
		t.Fatal(err)
	}
	makeReceipt := func(attempt, name, body string, mode os.FileMode) domain.AttemptReceipt {
		workspace := t.TempDir()
		path := filepath.Join(workspace, name)
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
		result, err := store.Retain(context.Background(), Input{AttemptID: domain.AttemptID(attempt), OutcomeID: "outcome-1", PlanRevisionID: "plan-1", WorkUnitID: domain.WorkUnitID(attempt), ContractRevisionNumber: 1, WorkspaceKind: domain.WorkspaceStagedFolder, WorkspacePath: workspace})
		if err != nil {
			t.Fatal(err)
		}
		return result.Receipt
	}
	first := makeReceipt("a", "answer.txt", "one", 0o640)
	second := makeReceipt("b", "other.txt", "two", 0o600)
	handoff, err := store.Compose(context.Background(), []domain.AttemptReceipt{first, second})
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "successor")
	if err := os.MkdirAll(destination, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := store.Materialize(context.Background(), handoff, destination, ""); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(destination, "answer.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "one" {
		t.Fatalf("answer = %q", body)
	}
	info, err := os.Stat(filepath.Join(destination, "answer.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	conflict := makeReceipt("c", "answer.txt", "different", 0o640)
	if _, err := store.Compose(context.Background(), []domain.AttemptReceipt{first, conflict}); err == nil {
		t.Fatal("conflicting predecessors composed")
	}
}

func TestStoreExportRequiresOwnerAndPreservesDeletion(t *testing.T) {
	store, err := New(Config{Root: filepath.Join(t.TempDir(), "artifacts")})
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "report.md"), []byte("accepted"), 0o640); err != nil {
		t.Fatal(err)
	}
	result, err := store.Retain(context.Background(), Input{AttemptID: "export-attempt", OutcomeID: "outcome-1", PlanRevisionID: "plan-1", WorkUnitID: "unit-1", ContractRevisionNumber: 1, WorkspaceKind: domain.WorkspaceStagedFolder, WorkspacePath: workspace})
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "export")
	if _, err := store.Export(context.Background(), ExportRequest{Receipt: result.Receipt, Destination: destination}); err == nil {
		t.Fatal("export without owner decision succeeded")
	}
	decision := &domain.AcceptanceDecision{ID: "accept-1", OutcomeID: "outcome-1", ContractRevisionID: "contract-1", Kind: domain.AcceptanceAccept, ActorType: domain.AcceptanceActorUser, Summary: "Reviewed", ResourceDisposition: domain.ResourceDispositionRetain, RequestKey: "request-1", RequestFingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	manifest, err := store.Export(context.Background(), ExportRequest{Receipt: result.Receipt, Decision: decision, ContractRevisionID: "contract-1", AcceptedArtifactVersion: result.Receipt.ArtifactVersion, Destination: destination})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Disposition != "accepted" {
		t.Fatalf("manifest = %#v", manifest)
	}
	body, err := os.ReadFile(filepath.Join(destination, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "accepted" {
		t.Fatalf("export = %q", body)
	}
}

func TestReadStableEnforcesGrowthBound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output.txt")
	if err := os.WriteFile(path, []byte("123"), 0o640); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readStable(context.Background(), path, info, 2); err == nil {
		t.Fatal("readStable accepted content beyond the byte bound")
	}
}

func TestStoreRejectsCorruptPublishedVersion(t *testing.T) {
	store, err := New(Config{Root: filepath.Join(t.TempDir(), "artifacts")})
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	path := filepath.Join(workspace, "report.md")
	if err := os.WriteFile(path, []byte("original"), 0o640); err != nil {
		t.Fatal(err)
	}
	in := Input{AttemptID: "repeat-attempt", OutcomeID: "outcome-1", PlanRevisionID: "plan-1", WorkUnitID: "unit-1", ContractRevisionNumber: 1, WorkspaceKind: domain.WorkspaceStagedFolder, WorkspacePath: workspace}
	first, err := store.Retain(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(first.ContentDir, "report.md"), []byte("tampered"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Retain(context.Background(), in); err == nil {
		t.Fatal("repeat retention accepted a corrupt published version")
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}
