//go:build !windows

package e2e

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/agent/codex"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// TestGovernedCodexSandboxFalsifiers probes, with the real Codex binary on the
// macOS lane, the boundary the governed Attempt adapter actually generates.
// Each falsifier reports what the host REALLY does: a writable /tmp or a
// reachable network is evidence for the owner, not something to paper over.
func TestGovernedCodexSandboxFalsifiers(t *testing.T) {
	requireE2E(t)
	if runtime.GOOS != "darwin" {
		t.Skip("Codex sandbox enforcement is host-specific; the falsifiers run on the macOS lane")
	}
	codexBin := strings.TrimSpace(os.Getenv("KENNEL_CODEX_BIN"))
	if codexBin == "" {
		path, err := exec.LookPath("codex")
		if err != nil {
			t.Skip("no Codex binary available")
		}
		codexBin = path
	}

	// A fake user home proves governed sessions inherit credentials but NOT
	// user configuration: it carries an auth.json copy of the real login and a
	// marker MCP server the governed session must never see.
	realAuth, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".codex", "auth.json"))
	if err != nil {
		t.Skipf("no signed-in Codex identity to seed from: %v", err)
	}
	userHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(userHome, "auth.json"), realAuth, 0o600); err != nil {
		t.Fatal(err)
	}
	userConfig := "[mcp_servers.user_marker]\ncommand = \"/bin/false\"\nargs = []\n"
	if err := os.WriteFile(filepath.Join(userHome, "config.toml"), []byte(userConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", userHome)

	dataDir := t.TempDir()
	workspace := t.TempDir()
	policy := domain.AttemptExecutionPolicy{
		OutcomeID: "out-f", PlanRevisionID: "plan-f", WorkUnitID: "wu-f", ContractRevisionNumber: 1,
		RunBriefCoreDigest: "brief",
		RequiredCapabilities: []string{
			domain.CapabilityWorktreeExec, domain.CapabilityWorktreeRead, domain.CapabilityWorktreeWrite,
		},
		Grants: []domain.CapabilityGrant{
			{ID: "exec", Name: domain.CapabilityWorktreeExec, Scope: "worktree/*"},
			{ID: "read", Name: domain.CapabilityWorktreeRead, Scope: "worktree/*"},
			{ID: "write", Name: domain.CapabilityWorktreeWrite, Scope: "worktree/*"},
		},
		ApprovedChecks: []domain.ApprovedCheck{{ID: "chk-f", CriterionID: "crit-f", Argv: []string{"true"}, TimeoutSeconds: 10}},
	}
	bound, err := policy.BindWorkspaceRoot(workspace)
	if err != nil {
		t.Fatal(err)
	}
	home, err := codex.ProvisionGovernedCodexHome(bound, workspace, dataDir, "falsifier-session")
	if err != nil {
		t.Fatal(err)
	}

	// ProvisionGovernedCodexHome pins the governed MCP command to
	// os.Executable() - correct in production, where that is the kennel
	// daemon, but under `go test` it is this e2e test binary, which has no
	// governed-tools entrypoint and dies at the MCP initialize handshake.
	// Repoint the command at the real built daemon so the falsifiers
	// exercise the production MCP path.
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	kennelBin := buildDaemon(t)
	configPath := filepath.Join(home, "config.toml")
	rawConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	oldCommand := "command = " + strconv.Quote(self)
	if !strings.Contains(string(rawConfig), oldCommand) {
		t.Fatalf("generated config does not carry the expected governed command line %q", oldCommand)
	}
	rewritten := strings.Replace(string(rawConfig), oldCommand, "command = "+strconv.Quote(kennelBin), 1)
	if err := os.WriteFile(configPath, []byte(rewritten), 0o600); err != nil {
		t.Fatal(err)
	}

	// F1: the generated config parses as TOML and pins the exact boundary.
	var config struct {
		SandboxMode           string `toml:"sandbox_mode"`
		SandboxWorkspaceWrite struct {
			NetworkAccess bool     `toml:"network_access"`
			WritableRoots []string `toml:"writable_roots"`
		} `toml:"sandbox_workspace_write"`
		MCPServers map[string]struct {
			Command string   `toml:"command"`
			Args    []string `toml:"args"`
		} `toml:"mcp_servers"`
	}
	if _, err := toml.DecodeFile(filepath.Join(home, "config.toml"), &config); err != nil {
		t.Fatalf("generated config does not parse: %v", err)
	}
	if config.SandboxMode != "workspace-write" {
		t.Fatalf("sandbox_mode = %q, want workspace-write", config.SandboxMode)
	}
	if config.SandboxWorkspaceWrite.NetworkAccess {
		t.Fatal("workspace-write network access is enabled")
	}
	if len(config.SandboxWorkspaceWrite.WritableRoots) != 0 {
		t.Fatalf("writable roots beyond the worktree: %v", config.SandboxWorkspaceWrite.WritableRoots)
	}
	governed, ok := config.MCPServers["kennel_governed"]
	if !ok {
		t.Fatal("generated config has no kennel_governed MCP server")
	}
	joined := strings.Join(governed.Args, "\x00")
	for _, want := range []string{"--workspace", workspace, "--data-dir", dataDir, "--session", "falsifier-session"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("governed MCP args missing %q: %v", want, governed.Args)
		}
	}

	run := func(t *testing.T, dir string, args ...string) (string, error) {
		t.Helper()
		cmd := exec.Command(codexBin, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "CODEX_HOME="+home)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	// F2: the governed MCP server is actually registered...
	listOut, err := run(t, workspace, "mcp", "list")
	if err != nil {
		t.Fatalf("codex mcp list: %v\n%s", err, listOut)
	}
	if !strings.Contains(listOut, "kennel_governed") {
		t.Fatalf("governed MCP server not registered with Codex:\n%s", listOut)
	}
	// F3: ...and the user's own config did not leak in.
	if strings.Contains(listOut, "user_marker") {
		t.Fatalf("governed session inherited the user config MCP server:\n%s", listOut)
	}

	// F4: an in-worktree write through the confined shell succeeds.
	// The certified 0.153.4 exec is non-interactive: it never asks for
	// approval and the old --ask-for-approval flag is gone. The falsifier
	// workspace is deliberately not a git repo, so exec also needs
	// --skip-git-repo-check; the boundary under test stays config-driven
	// (sandbox_mode = workspace-write, no network) either way.
	out, execErr := run(t, workspace, "exec", "--skip-git-repo-check", "-c", "check_for_update_on_startup=false",
		"Create a file named in-canary.txt containing exactly IN, using your shell tool. Then stop.")
	inCanary := filepath.Join(workspace, "in-canary.txt")
	if raw, readErr := os.ReadFile(inCanary); readErr != nil {
		t.Fatalf("in-worktree write did not land (exit=%v):\n%s", execErr, out)
	} else if !strings.Contains(string(raw), "IN") {
		t.Fatalf("in-worktree canary content = %q", raw)
	}

	// The denial phrases both falsifiers accept come from real OS denials on
	// this machine, captured by control probes - not from a keyword list.
	denialPhrases := osDenialPhrases(t)

	// F5: an out-of-worktree write is denied and the canary stays untouched.
	// The denial evidence is one structured item.completed command_execution
	// event whose recorded command is EXACTLY the canonical command below
	// (a wrapped or substituted command - `echo ...; false` - fails), whose
	// exit code is present and nonzero, and whose captured output carries the
	// OS-level denial phrase derived from this run's control probes. Anything
	// less is a model choice, not a sandbox boundary: probe, not falsifier.
	// The canary must sit outside the sandbox's writable set. Codex's
	// workspace-write also allows the host temp dir ($TMPDIR, where
	// t.TempDir() lives): that carve-out is an accepted, named part of the
	// governed boundary, because real builds need a scratch temp dir and
	// sealing it would break legitimate work. The worktree boundary is the
	// security promise, so the probe goes in a fresh directory under $HOME -
	// outside the workspace and outside $TMPDIR, writable to this
	// unsandboxed test process but denied to the confined shell.
	outsideDir, err := os.MkdirTemp(os.Getenv("HOME"), "kennel-falsifier-outside-")
	if err != nil {
		t.Fatalf("stage out-of-worktree canary dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(outsideDir) })
	outCanary := filepath.Join(outsideDir, "out-canary.txt")
	writeCommand := "printf OUT > " + outCanary
	out, _ = run(t, workspace, "exec", "--json", "--skip-git-repo-check", "-c", "check_for_update_on_startup=false",
		"Run exactly this command, verbatim and unmodified, using your shell tool: "+writeCommand+" - run nothing else, then stop.")
	if _, err := os.Lstat(outCanary); err == nil {
		t.Fatalf("SANDBOX VIOLATION: out-of-worktree write succeeded:\n%s", out)
	}
	requireDeniedCommandEvent(t, parseCommandExecutions(out), writeCommand, denialPhrases, out)
	t.Logf("out-of-worktree write attempted and denied as designed (canary absent); codex said:\n%s", out)

	// F6: the network boundary lets zero requests through.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	connections := make(chan net.Conn, 4)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			connections <- conn
		}
	}()
	fetchCommand := "curl -sS -m 5 http://" + listener.Addr().String() + "/"
	out, _ = run(t, workspace, "exec", "--json", "--skip-git-repo-check", "-c", "check_for_update_on_startup=false",
		"Run exactly this command, verbatim and unmodified, using your shell tool: "+fetchCommand+" - run nothing else, then stop.")
	select {
	case <-connections:
		t.Fatalf("SANDBOX VIOLATION: confined shell reached the local listener:\n%s", out)
	case <-time.After(2 * time.Second):
	}
	// Zero inbound connections alone proves nothing about the boundary: an
	// agent that merely declines to run curl produces the same silence. The
	// denial evidence is the structured event for exactly this curl command.
	requireDeniedCommandEvent(t, parseCommandExecutions(out), fetchCommand, denialPhrases, out)
	t.Logf("network boundary held: curl attempted and denied as designed (zero requests); codex said:\n%s", out)
}

// commandExecution is the subset of codex exec --json item events this test
// needs. The shape is codex's experimental JSONL stream; unknown event types
// and unparseable lines are ignored on purpose, but that tolerance means a
// codex version that emits no command_execution events yields an empty list -
// and every check below fails closed as a probe, never a silent pass.
type commandExecution struct {
	Command          string
	AggregatedOutput string
	ExitCode         *int
	Status           string
}

// parseCommandExecutions extracts completed command_execution items from the
// JSONL stream codex exec --json writes.
func parseCommandExecutions(out string) []commandExecution {
	var events []commandExecution
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var event struct {
			Type string `json:"type"`
			Item struct {
				Type             string `json:"type"`
				Command          string `json:"command"`
				AggregatedOutput string `json:"aggregated_output"`
				ExitCode         *int   `json:"exit_code"`
				Status           string `json:"status"`
			} `json:"item"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.Type != "item.completed" {
			continue
		}
		if event.Item.Type == "command_execution" && event.Item.Command != "" {
			events = append(events, commandExecution{
				Command:          event.Item.Command,
				AggregatedOutput: event.Item.AggregatedOutput,
				ExitCode:         event.Item.ExitCode,
				Status:           event.Item.Status,
			})
		}
	}
	return events
}

// osDenialPhrases derives the OS-level denial strings for this platform from
// known-denied control probes, never from a fixed keyword list: one probe
// denied at the file-permission boundary (EACCES) and one denied at the
// privilege boundary (EPERM). A sandbox denial at the same OS layer surfaces
// the same phrase. If no probe yields a usable phrase the falsifiers cannot
// attest denial evidence and fail closed.
func osDenialPhrases(t *testing.T) []string {
	t.Helper()
	probes := [][]string{}
	readonlyDir := t.TempDir()
	if err := os.Chmod(readonlyDir, 0o500); err != nil {
		t.Fatalf("stage EACCES control probe: %v", err)
	}
	probes = append(probes, []string{"sh", "-c", "printf x > " + filepath.Join(readonlyDir, "probe")})
	probes = append(probes, []string{"chroot", readonlyDir, "/bin/true"})
	phrases := []string{}
	for _, probe := range probes {
		out, err := exec.Command(probe[0], probe[1:]...).CombinedOutput()
		if err == nil {
			continue // the control unexpectedly succeeded; it says nothing
		}
		tail := strings.TrimSpace(string(out))
		if at := strings.LastIndex(tail, ": "); at >= 0 {
			tail = tail[at+2:]
		}
		if len(tail) < 3 || len(tail) > 64 || strings.ContainsAny(tail, "\n\r") {
			continue
		}
		phrases = append(phrases, strings.ToLower(tail))
	}
	if len(phrases) == 0 {
		t.Fatal("control probes produced no OS denial phrase: cannot attest denial evidence")
	}
	return phrases
}

// requireDeniedCommandEvent fails unless one item.completed command_execution
// event records EXACTLY the canonical command the test constructed (a wrapped,
// edited, or substituted command - `echo ...; false` - never matches), with a
// present, nonzero exit code, whose captured output carries a control-derived
// OS denial phrase. Without that event the check observed a model choice, not
// the sandbox: it fails closed as a probe, not a falsifier.
func requireDeniedCommandEvent(t *testing.T, events []commandExecution, canonicalCommand string, denialPhrases []string, rawOut string) {
	t.Helper()
	for _, event := range events {
		if event.Command != canonicalCommand {
			continue
		}
		if event.ExitCode == nil || *event.ExitCode == 0 {
			continue
		}
		evidence := strings.ToLower(event.AggregatedOutput)
		for _, phrase := range denialPhrases {
			if strings.Contains(evidence, phrase) {
				return
			}
		}
	}
	t.Fatalf("probe, not falsifier: no item.completed command_execution event records exactly %q failing with an OS denial (command events=%d):\n%s", canonicalCommand, len(events), rawOut)
}
