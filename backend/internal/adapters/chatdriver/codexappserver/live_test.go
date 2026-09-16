package codexappserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

const nativeWorktreeProfileInstructions = "You are running the codex_native_worktree_v1 compatibility proof. Use only local tools in the assigned disposable worktree; do not request wider filesystem, network, or external-effect authority. Follow exact proof commands and keep replies short."

type nativePathBoundary struct {
	Workspace           string
	AdditionalRoots     []string
	TMPDIR              string
	ExcludeSlashTmp     bool
	ExcludeTmpdirEnvVar bool
}

type safeNegativeTargetEvidence struct {
	Root              string `json:"root"`
	Target            string `json:"target"`
	CanonicalRoot     string `json:"canonical_root"`
	CanonicalTarget   string `json:"canonical_target"`
	HostPreflightPath string `json:"host_preflight_path"`
	HostPreflightOK   bool   `json:"host_preflight_ok"`
}

func canonicalBoundaryPath(path string) (string, error) {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	current := absolute
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func pathContainedBy(target, root string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func effectiveWritableRoots(boundary nativePathBoundary) ([]string, error) {
	roots := append([]string{boundary.Workspace}, boundary.AdditionalRoots...)
	if !boundary.ExcludeSlashTmp {
		roots = append(roots, "/tmp")
	}
	if !boundary.ExcludeTmpdirEnvVar && boundary.TMPDIR != "" {
		roots = append(roots, boundary.TMPDIR)
	}
	canonical := make([]string, 0, len(roots))
	seen := map[string]bool{}
	for _, root := range roots {
		resolved, err := canonicalBoundaryPath(root)
		if err != nil {
			return nil, err
		}
		if !seen[resolved] {
			seen[resolved] = true
			canonical = append(canonical, resolved)
		}
	}
	return canonical, nil
}

func classifyNativeWriteTarget(target string, boundary nativePathBoundary) (string, []string, bool, error) {
	canonicalTarget, err := canonicalBoundaryPath(target)
	if err != nil {
		return "", nil, false, err
	}
	roots, err := effectiveWritableRoots(boundary)
	if err != nil {
		return "", nil, false, err
	}
	for _, root := range roots {
		if pathContainedBy(canonicalTarget, root) {
			return canonicalTarget, roots, true, nil
		}
	}
	return canonicalTarget, roots, false, nil
}

func newSafeNegativeTarget(boundary nativePathBoundary) (safeNegativeTargetEvidence, func(), error) {
	for _, parent := range []string{"/private/var/tmp", "/var/tmp"} {
		canonicalParent, err := canonicalBoundaryPath(parent)
		if err != nil {
			continue
		}
		_, _, writable, err := classifyNativeWriteTarget(filepath.Join(canonicalParent, "kennel-stage1-probe"), boundary)
		if err != nil || writable {
			continue
		}
		root, err := os.MkdirTemp(canonicalParent, "kennel-stage1-negative-")
		if err != nil {
			continue
		}
		cleanup := func() { _ = os.RemoveAll(root) }
		canonicalRoot, err := canonicalBoundaryPath(root)
		if err != nil {
			cleanup()
			continue
		}
		target := filepath.Join(root, "provider", "outside-sentinel.txt")
		canonicalTarget, _, writable, err := classifyNativeWriteTarget(target, boundary)
		if err != nil || writable {
			cleanup()
			continue
		}
		preflight := filepath.Join(root, "host-preflight.txt")
		if err := os.WriteFile(preflight, []byte("HOST-PREFLIGHT"), 0o600); err != nil {
			cleanup()
			continue
		}
		if err := os.Remove(preflight); err != nil {
			cleanup()
			continue
		}
		return safeNegativeTargetEvidence{
			Root: root, Target: target, CanonicalRoot: canonicalRoot, CanonicalTarget: canonicalTarget,
			HostPreflightPath: preflight, HostPreflightOK: true,
		}, cleanup, nil
	}
	return safeNegativeTargetEvidence{}, nil, errors.New("no_safe_negative_target")
}

func TestNativeWriteTargetClassification(t *testing.T) {
	fixture := t.TempDir()
	workspace := filepath.Join(fixture, "workspace")
	extra := filepath.Join(fixture, "extra")
	tmpdir := filepath.Join(fixture, "tmpdir")
	outside := filepath.Join(fixture, "outside")
	for _, dir := range []string{workspace, extra, tmpdir, outside} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	insideLink := filepath.Join(fixture, "inside-link")
	outsideLink := filepath.Join(fixture, "outside-link")
	if err := os.Symlink(workspace, insideLink); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, outsideLink); err != nil {
		t.Fatal(err)
	}

	base := nativePathBoundary{Workspace: workspace, TMPDIR: tmpdir, ExcludeSlashTmp: true, ExcludeTmpdirEnvVar: true}
	tests := []struct {
		name     string
		target   string
		boundary nativePathBoundary
		inside   bool
	}{
		{name: "direct child", target: filepath.Join(workspace, "child"), boundary: base, inside: true},
		{name: "sibling", target: outside, boundary: base, inside: false},
		{name: "dot dot normalization", target: filepath.Join(workspace, "child", "..", "kept"), boundary: base, inside: true},
		{name: "tmp alias private", target: "/private/tmp/child", boundary: nativePathBoundary{Workspace: workspace, ExcludeSlashTmp: false, ExcludeTmpdirEnvVar: true}, inside: runtime.GOOS == "darwin"},
		{name: "tmp alias short", target: "/tmp/child", boundary: nativePathBoundary{Workspace: workspace, ExcludeSlashTmp: false, ExcludeTmpdirEnvVar: true}, inside: true},
		{name: "tmpdir included", target: filepath.Join(tmpdir, "child"), boundary: nativePathBoundary{Workspace: workspace, TMPDIR: tmpdir, ExcludeSlashTmp: true, ExcludeTmpdirEnvVar: false}, inside: true},
		{name: "tmpdir excluded", target: filepath.Join(tmpdir, "child"), boundary: base, inside: false},
		{name: "extra root", target: filepath.Join(extra, "child"), boundary: nativePathBoundary{Workspace: workspace, AdditionalRoots: []string{extra}, TMPDIR: tmpdir, ExcludeSlashTmp: true, ExcludeTmpdirEnvVar: true}, inside: true},
		{name: "symlink points inside", target: filepath.Join(insideLink, "child"), boundary: base, inside: true},
		{name: "symlink points outside", target: filepath.Join(outsideLink, "child"), boundary: base, inside: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, inside, err := classifyNativeWriteTarget(tc.target, tc.boundary)
			if err != nil {
				t.Fatalf("classify: %v", err)
			}
			if inside != tc.inside {
				t.Fatalf("inside = %t, want %t", inside, tc.inside)
			}
		})
	}
	if runtime.GOOS == "darwin" {
		short, _ := canonicalBoundaryPath("/var/tmp")
		private, _ := canonicalBoundaryPath("/private/var/tmp")
		if short != private {
			t.Fatalf("macOS /var alias = %q, want %q", short, private)
		}
	}
}

type controlledNetworkProbe struct {
	server      *httptest.Server
	connections atomic.Int64
}

func newControlledNetworkProbe(t *testing.T) *controlledNetworkProbe {
	t.Helper()
	probe := &controlledNetworkProbe{}
	probe.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		probe.connections.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(probe.server.Close)
	return probe
}

func (p *controlledNetworkProbe) hostPreflight(t *testing.T) int64 {
	t.Helper()
	resp, err := http.Get(p.server.URL)
	if err != nil {
		t.Fatalf("controlled network host preflight: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("controlled network status = %d", resp.StatusCode)
	}
	return p.connections.Load()
}

// TestLiveCodexAppServer drives a real `codex app-server`. It is skipped unless
// KENNEL_CODEX_LIVE=1, because it needs a local Codex install, working auth, and it
// makes real model calls. Everything else in this package runs against pipes.
//
// Run it after changing the protocol layer:
//
//	KENNEL_CODEX_LIVE=1 go test ./internal/adapters/chatdriver/codexappserver/ -run Live -v
func TestLiveCodexAppServer(t *testing.T) {
	if os.Getenv("KENNEL_CODEX_LIVE") != "1" {
		t.Skip("set KENNEL_CODEX_LIVE=1 to run against a real codex app-server")
	}

	bin := os.Getenv("KENNEL_CODEX_BIN")
	if bin == "" {
		bin = "codex"
	}
	if _, err := exec.LookPath(bin); err != nil {
		t.Skipf("codex binary %q not on PATH: %v", bin, err)
	}

	workspace := t.TempDir()
	seedGitWorkspace(t, workspace)

	d := New(livePlugin{bin: bin}, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))

	caps, err := d.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if missing := ports.MissingProductionCapabilities(caps); len(missing) != 0 {
		t.Fatalf("missing production capabilities: %v", missing)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	conv, err := d.Start(ctx, ports.ChatStartConfig{
		SessionID:     "kennel-live",
		WorkspacePath: workspace,
		Env:           envMap(),
		Permissions:   ports.PermissionModeDefault,
		SystemPrompt:  "You are in an automated test. Answer in one short sentence.",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = conv.Close() }()

	threadID := conv.ProviderConversationID()
	if threadID == "" {
		t.Fatal("no provider conversation id after Start")
	}
	t.Logf("thread %s", threadID)

	if _, err := conv.SendTurn(ctx, ports.ChatUserMessage{
		Text:            "Reply with exactly the word: acknowledged",
		ClientMessageID: "live-1",
		Origin:          domain.MessageOriginHuman,
	}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	var (
		sawDelta bool
		state    domain.TurnState
	)
collect:
	for {
		select {
		case ev, ok := <-conv.Events():
			if !ok {
				t.Fatal("event stream closed before the turn completed")
			}
			switch ev.Kind {
			case ports.ChatEventMessageDelta:
				sawDelta = true
			case ports.ChatEventApprovalRequested:
				// Default posture is never-ask, so an approval here means the
				// permission mapping regressed.
				t.Errorf("unexpected approval request under default permissions: %s", ev.Summary)
				_ = conv.ResolveRequest(ctx, ev.RequestID, ports.ChatDecision{ID: "accept"})
			case ports.ChatEventTurnCompleted:
				state = ev.TurnState
				break collect
			case ports.ChatEventControllerState:
				if ev.ControllerState == ports.ChatControllerStopped {
					t.Fatalf("controller stopped before the turn completed: %v", ev.Err)
				}
			}
		case <-ctx.Done():
			t.Fatalf("timed out: %v", ctx.Err())
		}
	}

	if !sawDelta {
		t.Error("no streaming deltas observed")
	}
	if state != domain.TurnStateCompleted {
		t.Errorf("turn state = %q, want completed", state)
	}

	// Resume on a fresh process must recover the same thread — this is the
	// daemon-restart path.
	if err := conv.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	resumed, err := d.Resume(ctx, ports.ChatResumeConfig{
		SessionID:              "kennel-live",
		ProviderConversationID: threadID,
		WorkspacePath:          workspace,
		Env:                    envMap(),
		Permissions:            ports.PermissionModeDefault,
	})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	defer func() { _ = resumed.Close() }()

	if got := resumed.ProviderConversationID(); got != threadID {
		t.Fatalf("resumed thread = %q, want %q", got, threadID)
	}
	t.Logf("resumed thread %s on a fresh app-server process", threadID)
}

func TestCanonicalLiveCodexBinaryAcceptsDirectPathAndRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix symlink identity")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("canonicalize temp dir: %v", err)
	}
	direct := filepath.Join(dir, "codex")
	if err := os.WriteFile(direct, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := canonicalLiveCodexBinary(direct); err != nil || got != direct {
		t.Fatalf("direct canonical identity = %q, %v", got, err)
	}
	shim := filepath.Join(t.TempDir(), "codex")
	if err := os.Symlink(direct, shim); err != nil {
		t.Fatal(err)
	}
	if _, err := canonicalLiveCodexBinary(shim); err == nil || !strings.Contains(err.Error(), direct) {
		t.Fatalf("symlink identity error = %v, want canonical target", err)
	}
}

func TestFailedAssertionPredicateParsesRealVerboseAndParallelOrder(t *testing.T) {
	fixtures := []string{
		"--- FAIL: TestValue (0.00s)\n    proof_test.go:5: Value = \"wrong\"\nFAIL\nFAIL\texample.com/substrate\t0.002s\n",
		"=== RUN   TestValue\n    proof_test.go:5: Value = \"wrong\"\n--- FAIL: TestValue (0.00s)\nFAIL\n",
		"=== RUN   TestValue\n=== PAUSE TestValue\n=== CONT  TestValue\n=== NAME  TestValue\n    proof_test.go:5: Value = \"wrong\"\n--- FAIL: TestValue (0.00s)\n",
	}
	for i, output := range fixtures {
		r := liveTurnResult{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, output: output}}}
		if !r.failedTestValueAssertion() {
			t.Fatalf("real fixture %d rejected", i)
		}
	}
	negatives := []liveTurnResult{
		{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, output: `log: --- FAIL: TestValue and Value = "wrong" were expected`}}},
		{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, output: `=== RUN   TestOther
    proof_test.go:5: Value = "wrong"
--- FAIL: TestOther (0.00s)
--- FAIL: TestValue (0.00s)
`}}},
		{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, output: `=== RUN TestValue
    proof_test.go:5: Value = "wrong"
`}, {status: domain.ActivityStatusFailed, output: "--- FAIL: TestValue (0.00s)\n"}}},
		{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, output: `{"output":"=== RUN TestValue\\n proof_test.go:5: Value = \\\"wrong\\\"\\n--- FAIL: TestValue"}`}}},
		{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, output: "--- FAIL: TestOther (0.00s)\n    proof_test.go:5: Value = \"wrong\"\n--- FAIL: TestValue (0.00s)\n"}}},
		{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, output: "--- FAIL: TestValue (0.00s)\nproof_test.go:5: Value = \"wrong\"\n"}}},
		{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, output: "    proof_test.go:5: Value = \"wrong\"\n--- FAIL: TestValue was expected\n"}}},
		{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, output: "    proof_test.go:5: Value = \"wrong\"\n--- FAIL: TestValue (not-a-duration)\n"}}},
		{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, output: "    proof_test.go:5: Value = \"wrong\"\n--- FAIL: TestValue (0.00s) trailing prose\n"}}},
		{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, output: "    --- FAIL: TestValue (0.00s)\n    proof_test.go:5: Value = \"wrong\"\n"}}},
		{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, output: "=== RUN   TestValue\n    proof_test.go:5: Value = \"wrong\"\n    --- FAIL: TestValue (0.00s)\n"}}},
		{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, output: "    proof_test.go:5: Value = \"wrong\"\n    --- FAIL: TestValue (0.00s)\n"}}},
		{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, output: "--- FAIL: TestValue (0.00s)\n    proof_test.go:: Value = \"wrong\"\n"}}},
		{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, output: "--- FAIL: TestValue (0.00s)\n    :5: Value = \"wrong\"\n"}}},
		{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, output: "=== RUNNER TestValue\n    proof_test.go:5: Value = \"wrong\"\n--- FAIL: TestValue (0.00s)\n"}}},
	}
	for i, r := range negatives {
		if r.failedTestValueAssertion() {
			t.Fatalf("negative %d accepted", i)
		}
	}
}

func TestDenialAttributionUsesExactArgumentAndNonCommandSemantics(t *testing.T) {
	const url = "https://example.com/"
	const path = "/outside/target"
	markers := []string{"operation not permitted", "permission denied", "denied by policy", "network access denied", "sandbox policy denied", "blocked by sandbox policy"}
	for _, marker := range markers {
		r := liveTurnResult{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, command: "echo " + marker + "; curl " + url, output: "generic error"}}}
		if r.denialAttributedTo(url) {
			t.Fatalf("command echo %q counted as semantics", marker)
		}
	}
	negatives := []struct {
		r         liveTurnResult
		operation string
	}{
		{liveTurnResult{activities: []liveActivityEvidence{{status: domain.ActivityStatusPending, command: "curl " + url + " 'approval required'", summary: "generic error", approval: true}}}, url},
		{liveTurnResult{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, command: "write " + path + "-other", output: "permission denied"}}}, path},
		{liveTurnResult{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, command: "write /outside", output: "permission denied"}}}, path},
		{liveTurnResult{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, command: "write " + path + "/child", output: "permission denied"}}}, path},
		{liveTurnResult{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, command: "curl " + url + "longer", output: "network access denied"}}}, url},
		{liveTurnResult{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, command: "curl " + url, output: "echo: approval required"}}}, url},
	}
	for i, tc := range negatives {
		if tc.r.denialAttributedTo(tc.operation) {
			t.Fatalf("negative %d accepted", i)
		}
	}
	failed := liveTurnResult{activities: []liveActivityEvidence{{status: domain.ActivityStatusFailed, command: "curl " + url, output: "network access denied"}}}
	if !failed.denialAttributedTo(url) {
		t.Fatal("exact failed enforcement rejected")
	}
	approval := liveTurnResult{activities: []liveActivityEvidence{{status: domain.ActivityStatusPending, command: "write " + path, summary: "approval required for outside write", approval: true}}}
	if !approval.denialAttributedTo(path) {
		t.Fatal("exact approval rejected")
	}
}

// TestLiveCodexRuntimeCanary separates protocol compatibility from native tool
// viability. It uses the exact canonical runtime selected by the wrapper and
// requires an attributable local command completion before the expensive journey.
func TestLiveCodexRuntimeCanary(t *testing.T) {
	requireLiveCodex(t)
	bin := liveCodexBin(t)
	workspace := newDisposableGitWorktree(t)
	d := New(livePlugin{bin: bin}, slog.New(slog.DiscardHandler))
	opened := startLiveConversation(t, d, workspace)
	defer func() { _ = opened.Close() }()
	result := runLiveTurn(t, phaseContext(t, 60*time.Second), opened, ports.ChatUserMessage{
		Text:            "Run this exact local command: printf RUNTIME-CANARY > runtime-canary.txt. Then reply CANARY-DONE.",
		ClientMessageID: "native-runtime-canary", Origin: domain.MessageOriginHuman,
	})
	if result.state != domain.TurnStateCompleted || !result.sawCompletedCommand {
		t.Fatalf("runtime companion canary failed: %s", result.sanitizedEvidence())
	}
	assertFileTrimmed(t, filepath.Join(workspace, "runtime-canary.txt"), "RUNTIME-CANARY")
	t.Logf("runtime_canary=true turn=%s client=native-runtime-canary canonical_binary=%q", result.ref.ProviderTurnID, bin)
}

// TestLivePersistentCodexSubstrate proves the native primitives vNext relies on
// without creating an Outcome or changing daemon admission. It deliberately uses
// one disposable git worktree and one provider thread across many turns and a
// fresh app-server process.
func TestLivePersistentCodexSubstrate(t *testing.T) {
	requireLiveCodex(t)
	bin := liveCodexBin(t)
	workspace := newDisposableGitWorktree(t)
	d := New(livePlugin{bin: bin}, slog.New(slog.DiscardHandler))
	provenance, err := d.ProtocolProvenance(phaseContext(t, 30*time.Second))
	if err != nil {
		t.Fatalf("protocol provenance: %v", err)
	}
	t.Logf("profile=codex_native_worktree_v1 installed_version=%s protocol_digest=%s generated_from=%s generated_digest=%s matches_generated=%t degraded=%v missing_floor=%v",
		provenance.InstalledVersion, provenance.ProtocolDigest, provenance.GeneratedFrom,
		provenance.GeneratedDigest, provenance.MatchesGenerated, provenance.DegradedCapabilities, provenance.MissingFloor)

	opened := startLiveConversation(t, d, workspace)
	defer func() { _ = opened.Close() }()
	logLatestNativePolicyEvidence(t, opened, "thread_start", "")
	threadID := opened.ProviderConversationID()
	if threadID == "" {
		t.Fatal("thread/start returned no provider conversation id")
	}
	t.Logf("thread/start thread=%s workspace=%s", threadID, workspace)

	t.Logf("profile_instructions_sha256=%x", sha256.Sum256([]byte(nativeWorktreeProfileInstructions)))
	skillLister, ok := opened.(ports.ChatSkillLister)
	if !ok {
		t.Fatalf("conversation %T has no skill lister", opened)
	}
	skills, err := skillLister.ListSkills(phaseContext(t, 30*time.Second))
	if err != nil {
		t.Fatalf("list inherited skills: %v", err)
	}
	skillJSON, err := json.Marshal(skills)
	if err != nil {
		t.Fatalf("marshal inherited skill provenance: %v", err)
	}
	t.Logf("inherited_skills_count=%d inherited_skills_sha256=%x", len(skills), sha256.Sum256(skillJSON))

	// The harness owns fixture bytes. Asking a model to synthesize Go source inside a
	// shell command adds three quoting layers and can turn the intended assertion
	// failure into an unrelated compile failure before the proof begins.
	writePersistentSubstrateFixture(t, workspace)
	first := runLiveTurn(t, liveTurnContext(t), opened, ports.ChatUserMessage{
		Text: "Inspect the assigned repository, then run this exact local command: " +
			"pwd > observed-pwd.txt && git rev-parse --show-toplevel > observed-git-root.txt && " +
			"printf PERSISTENT-SUBSTRATE > substrate-marker.txt && go test ./... . " +
			"The failing TestValue assertion is expected; report FIRST-FAIL-OBSERVED after it runs.",
		ClientMessageID: "persistent-substrate-failing-test", Origin: domain.MessageOriginHuman,
	})
	logLatestNativePolicyEvidence(t, opened, "turn_start", first.ref.ProviderTurnID)
	if first.ref.ProviderTurnID == "" || first.state != domain.TurnStateCompleted || !first.sawFailedCommand ||
		!first.failedTestValueAssertion() {
		t.Fatalf("expected TestValue assertion was not the failed command: %s", first.sanitizedEvidence())
	}
	assertCanonicalPathFile(t, filepath.Join(workspace, "observed-pwd.txt"), workspace)
	assertCanonicalPathFile(t, filepath.Join(workspace, "observed-git-root.txt"), workspace)
	assertFileTrimmed(t, filepath.Join(workspace, "substrate-marker.txt"), "PERSISTENT-SUBSTRATE")
	marker, err := os.ReadFile(filepath.Join(workspace, "substrate-marker.txt"))
	if err != nil {
		t.Fatalf("read marker evidence: %v", err)
	}
	t.Logf("evidence turn=%s client=%s marker=%q marker_sha256=%x expected_assertion_observed=true", first.ref.ProviderTurnID,
		"persistent-substrate-failing-test", strings.TrimSpace(string(marker)), sha256.Sum256(marker))

	second := runLiveTurn(t, liveTurnContext(t), opened, ports.ChatUserMessage{
		Text: "Repair the failing local test by running this exact command: " +
			"printf 'package substrate\\n\\nfunc Value() string { return \"right\" }\\n' > proof.go && " +
			"go test ./... && tr '[:upper:]' '[:lower:]' < substrate-marker.txt > turn2-derived.txt. " +
			"Then reply SECOND-REPAIRED.",
		ClientMessageID: "persistent-substrate-repair-test", Origin: domain.MessageOriginHuman,
	})
	logLatestNativePolicyEvidence(t, opened, "turn_start", second.ref.ProviderTurnID)
	if second.ref.ProviderTurnID == first.ref.ProviderTurnID || second.state != domain.TurnStateCompleted ||
		!second.sawCommand || !second.sawCompletedCommand {
		t.Fatalf("repair turn = %#v", second)
	}
	assertFileTrimmed(t, filepath.Join(workspace, "turn2-derived.txt"), "persistent-substrate")
	assertFileContains(t, filepath.Join(workspace, "proof.go"), `return "right"`)
	t.Logf("evidence turn=%s client=%s repaired_test_passed=true", second.ref.ProviderTurnID, "persistent-substrate-repair-test")

	fsPositive := runLiveTurn(t, phaseContext(t, 60*time.Second), opened, ports.ChatUserMessage{
		Text:            "Run this exact local write control: printf LOCAL-WRITE-OK > filesystem-positive-control.txt. Then reply LOCAL-WRITE-OK.",
		ClientMessageID: "persistent-substrate-fs-positive", Origin: domain.MessageOriginHuman,
	})
	logLatestNativePolicyEvidence(t, opened, "turn_start", fsPositive.ref.ProviderTurnID)
	if fsPositive.state != domain.TurnStateCompleted || !fsPositive.sawCompletedCommand {
		t.Fatalf("filesystem positive control failed: %s", fsPositive.sanitizedEvidence())
	}
	assertFileTrimmed(t, filepath.Join(workspace, "filesystem-positive-control.txt"), "LOCAL-WRITE-OK")

	boundary := nativePathBoundary{
		Workspace: workspace, TMPDIR: os.Getenv("TMPDIR"), ExcludeSlashTmp: true, ExcludeTmpdirEnvVar: true,
	}
	targetEvidence, cleanupTarget, err := newSafeNegativeTarget(boundary)
	if err != nil {
		t.Fatalf("no_safe_negative_target: %v", err)
	}
	defer cleanupTarget()
	targetJSON, _ := json.Marshal(targetEvidence)
	t.Logf("path_classification=%s", targetJSON)
	outside := targetEvidence.Target
	deniedFS := runLiveTurnAllowDeniedRequest(t, liveTurnContext(t), opened, ports.ChatUserMessage{
		Text: "Test the profile boundary by running this exact command, without requesting wider authority: " +
			"mkdir -p " + shellQuote(filepath.Dir(outside)) + " && printf ESCAPED > " + shellQuote(outside) +
			". This operation is expected to be denied; report FS-DENIED.",
		ClientMessageID: "persistent-substrate-fs-denied", Origin: domain.MessageOriginHuman,
	})
	logLatestNativePolicyEvidence(t, opened, "turn_start", deniedFS.ref.ProviderTurnID)
	if deniedFS.state != domain.TurnStateCompleted {
		t.Fatalf("filesystem boundary turn did not complete: %s", deniedFS.sanitizedEvidence())
	}
	fsGovernance := "partial"
	if deniedFS.denialAttributedTo(outside) {
		fsGovernance = "full"
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		_ = os.Remove(outside)
		t.Fatalf("undeclared filesystem write escaped profile: %v", err)
	}
	t.Logf("evidence turn=%s client=%s filesystem_escape_denied=true approval_request_denied=%t compatible=true governance=%s governance_reason=%s", deniedFS.ref.ProviderTurnID,
		"persistent-substrate-fs-denied", deniedFS.sawDeniedRequest, fsGovernance, governanceReason(fsGovernance))

	networkProbe := newControlledNetworkProbe(t)
	hostConnections := networkProbe.hostPreflight(t)
	t.Logf("network_host_preflight=true endpoint=%s accepted_connections=%d", networkProbe.server.URL, hostConnections)
	networkProbeURL := networkProbe.server.URL + "/stage1"
	deniedNetwork := runLiveTurnAllowDeniedRequest(t, liveTurnContext(t), opened, ports.ChatUserMessage{
		Text: "Test the profile's explicit network boundary by running this exact command, without requesting wider authority: " +
			"curl --fail --silent --show-error --max-time 5 " + shellQuote(networkProbeURL) + " >/dev/null && printf REACHED > network-reached.txt" +
			". Network is expected to be denied; report NETWORK-DENIED.",
		ClientMessageID: "persistent-substrate-network-denied", Origin: domain.MessageOriginHuman,
	})
	logLatestNativePolicyEvidence(t, opened, "turn_start", deniedNetwork.ref.ProviderTurnID)
	if deniedNetwork.state != domain.TurnStateCompleted {
		t.Fatalf("network boundary turn did not complete: %s", deniedNetwork.sanitizedEvidence())
	}
	networkGovernance := "partial"
	if deniedNetwork.denialAttributedTo(networkProbeURL) {
		networkGovernance = "full"
	}
	if _, err := os.Stat(filepath.Join(workspace, "network-reached.txt")); !os.IsNotExist(err) {
		t.Fatalf("undeclared network command reached its success sentinel: %v", err)
	}
	if got := networkProbe.connections.Load(); got != hostConnections {
		t.Fatalf("provider reached controlled endpoint: connections=%d want host-only %d", got, hostConnections)
	}
	t.Logf("evidence turn=%s client=%s network_denied=true approval_request_denied=%t compatible=true governance=%s governance_reason=%s", deniedNetwork.ref.ProviderTurnID,
		"persistent-substrate-network-denied", deniedNetwork.sawDeniedRequest, networkGovernance, governanceReason(networkGovernance))

	interruptCtx := phaseContext(t, 25*time.Second)
	interruptRef, err := opened.SendTurn(interruptCtx, ports.ChatUserMessage{
		Text: "Run this exact local command and wait: " +
			"for i in 1 2 3 4 5 6 7 8 9 10; do echo interrupt-$i | tee -a interrupt.log; sleep 3; done; " +
			"printf TERMINAL > interrupt-terminal.txt",
		ClientMessageID: "persistent-substrate-interrupt", Origin: domain.MessageOriginHuman,
	})
	if err != nil {
		t.Fatalf("start interrupt turn: %v", err)
	}
	waitForLiveTurnActivity(t, interruptCtx, opened.Events(), interruptRef.ProviderTurnID)
	waitForNonEmptyFile(t, interruptCtx, filepath.Join(workspace, "interrupt.log"))
	startedInterrupt := time.Now()
	interruptErr := opened.Interrupt(interruptCtx, interruptRef.ProviderTurnID)
	restartedAfterInterrupt := errors.Is(interruptErr, ports.ErrChatInterruptRestartRequired)
	if interruptErr != nil && !restartedAfterInterrupt {
		t.Fatalf("turn/interrupt: %v", interruptErr)
	}
	interrupted := waitForLiveTurnCompletion(t, interruptCtx, opened.Events(), interruptRef.ProviderTurnID)
	if interrupted != domain.TurnStateInterrupted {
		t.Fatalf("interrupted turn state = %q", interrupted)
	}
	if elapsed := time.Since(startedInterrupt); elapsed > 15*time.Second {
		t.Fatalf("interrupt completion took %s", elapsed)
	}
	t.Logf("evidence turn=%s client=%s state=%s", interruptRef.ProviderTurnID,
		"persistent-substrate-interrupt", interrupted)
	logLatestNativePolicyEvidence(t, opened, "turn_start", interruptRef.ProviderTurnID)
	if restartedAfterInterrupt {
		if err := opened.Close(); err != nil {
			t.Fatalf("close force-stopped app-server: %v", err)
		}
		restartCtx := phaseContext(t, 90*time.Second)
		opened, err = d.Resume(restartCtx, ports.ChatResumeConfig{
			SessionID: "kennel-live-persistent-substrate", ProviderConversationID: threadID,
			WorkspacePath: workspace, Env: liveCodexEnv(), Permissions: ports.PermissionModeAcceptEdits,
			SystemPrompt: nativeWorktreeProfileInstructions, NativeSandboxProfile: nativeLiveProfile(),
		})
		if err != nil {
			t.Fatalf("resume after interrupt fail-safe: %v", err)
		}
		t.Logf("interrupt_fail_safe=process_tree_killed thread_resumed=true")
	}
	beforeQuiescence := snapshotInterruptFile(t, filepath.Join(workspace, "interrupt.log"))
	assertInterruptQuiescent(t, opened.Events(), interruptRef.ProviderTurnID, filepath.Join(workspace, "interrupt.log"), beforeQuiescence, 5500*time.Millisecond)
	if _, err := os.Stat(filepath.Join(workspace, "interrupt-terminal.txt")); !os.IsNotExist(err) {
		t.Fatalf("terminal sentinel exists after interrupt: %v", err)
	}

	if err := opened.Close(); err != nil {
		t.Fatalf("close first app-server: %v", err)
	}
	resumeCtx := phaseContext(t, 90*time.Second)
	resumed, err := d.Resume(resumeCtx, ports.ChatResumeConfig{
		SessionID: "kennel-live-persistent-substrate", ProviderConversationID: threadID,
		WorkspacePath: workspace, Env: liveCodexEnv(), Permissions: ports.PermissionModeAcceptEdits,
		SystemPrompt: nativeWorktreeProfileInstructions, NativeSandboxProfile: nativeLiveProfile(),
	})
	if err != nil {
		t.Fatalf("thread/resume on fresh app-server: %v", err)
	}
	defer func() { _ = resumed.Close() }()
	logLatestNativePolicyEvidence(t, resumed, "thread_resume", "")
	if got := resumed.ProviderConversationID(); got != threadID {
		t.Fatalf("resumed thread = %q, want %q", got, threadID)
	}

	historyReader, ok := resumed.(ports.ChatHistoryReader)
	if !ok {
		t.Fatalf("resumed conversation %T has no history reader", resumed)
	}
	history1, err := historyReader.ReadHistory(resumeCtx)
	if err != nil {
		t.Fatalf("first history read: %v", err)
	}
	history2, err := historyReader.ReadHistory(resumeCtx)
	if err != nil {
		t.Fatalf("second history read: %v", err)
	}
	ids1 := historyIDs(t, history1)
	ids2 := historyIDs(t, history2)
	if !reflect.DeepEqual(ids1, ids2) {
		t.Fatalf("history ids changed:\n%v\n%v", ids1, ids2)
	}
	assertHistoryTurns(t, history1, map[string]string{
		first.ref.ProviderTurnID:         "persistent-substrate-failing-test",
		second.ref.ProviderTurnID:        "persistent-substrate-repair-test",
		fsPositive.ref.ProviderTurnID:    "persistent-substrate-fs-positive",
		deniedFS.ref.ProviderTurnID:      "persistent-substrate-fs-denied",
		deniedNetwork.ref.ProviderTurnID: "persistent-substrate-network-denied",
		interruptRef.ProviderTurnID:      "persistent-substrate-interrupt",
	})

	third := runLiveTurn(t, liveTurnContext(t), resumed, ports.ChatUserMessage{
		Text: "Run this exact local command: " +
			"wc -c < turn2-derived.txt | tr -d ' ' > resumed-count.txt. Then reply RESUMED-DONE.",
		ClientMessageID: "persistent-substrate-after-resume", Origin: domain.MessageOriginHuman,
	})
	logLatestNativePolicyEvidence(t, resumed, "turn_start", third.ref.ProviderTurnID)
	if third.state != domain.TurnStateCompleted {
		t.Fatalf("post-resume turn = %#v", third)
	}
	assertFileTrimmed(t, filepath.Join(workspace, "resumed-count.txt"), "20")

	postResumeOutside := filepath.Join(targetEvidence.Root, "post-resume", "outside-sentinel.txt")
	canonicalPostResume, _, writable, err := classifyNativeWriteTarget(postResumeOutside, boundary)
	if err != nil || writable {
		t.Fatalf("post-resume target classification target=%q writable=%t err=%v", canonicalPostResume, writable, err)
	}
	postResumeFS := runLiveTurnAllowDeniedRequest(t, liveTurnContext(t), resumed, ports.ChatUserMessage{
		Text: "After Resume, repeat the filesystem boundary with this exact command and no wider authority: mkdir -p " +
			shellQuote(filepath.Dir(postResumeOutside)) + " && printf ESCAPED > " + shellQuote(postResumeOutside) + ". Report POST-RESUME-FS-DENIED.",
		ClientMessageID: "persistent-substrate-post-resume-fs-denied", Origin: domain.MessageOriginHuman,
	})
	logLatestNativePolicyEvidence(t, resumed, "turn_start", postResumeFS.ref.ProviderTurnID)
	if postResumeFS.state != domain.TurnStateCompleted {
		t.Fatalf("post-resume filesystem boundary turn did not complete: %s", postResumeFS.sanitizedEvidence())
	}
	postResumeFSGovernance := "partial"
	if postResumeFS.denialAttributedTo(postResumeOutside) {
		postResumeFSGovernance = "full"
	}
	if _, err := os.Stat(postResumeOutside); !os.IsNotExist(err) {
		t.Fatalf("post-resume outside sentinel exists: %v", err)
	}

	postResumeURL := networkProbe.server.URL + "/post-resume"
	postResumeNetwork := runLiveTurnAllowDeniedRequest(t, liveTurnContext(t), resumed, ports.ChatUserMessage{
		Text: "After Resume, repeat the network boundary with this exact command and no wider authority: curl --fail --silent --show-error --max-time 5 " +
			shellQuote(postResumeURL) + " >/dev/null && printf REACHED > post-resume-network-reached.txt. Report POST-RESUME-NETWORK-DENIED.",
		ClientMessageID: "persistent-substrate-post-resume-network-denied", Origin: domain.MessageOriginHuman,
	})
	logLatestNativePolicyEvidence(t, resumed, "turn_start", postResumeNetwork.ref.ProviderTurnID)
	if postResumeNetwork.state != domain.TurnStateCompleted {
		t.Fatalf("post-resume network boundary turn did not complete: %s", postResumeNetwork.sanitizedEvidence())
	}
	postResumeNetworkGovernance := "partial"
	if postResumeNetwork.denialAttributedTo(postResumeURL) {
		postResumeNetworkGovernance = "full"
	}
	if got := networkProbe.connections.Load(); got != hostConnections {
		t.Fatalf("post-resume provider reached controlled endpoint: connections=%d want %d", got, hostConnections)
	}
	t.Logf("evidence turn=%s client=%s post_resume_filesystem_denied=true post_resume_network_denied=true compatible=true filesystem_governance=%s network_governance=%s governance_reason=%s", third.ref.ProviderTurnID, "persistent-substrate-after-resume", postResumeFSGovernance, postResumeNetworkGovernance, governanceReason("partial"))
	status := exec.Command("git", "status", "--short")
	status.Dir = workspace
	statusOut, err := status.CombinedOutput()
	if err != nil {
		t.Fatalf("git status --short: %v: %s", err, statusOut)
	}
	t.Logf("evidence git_status_short=%q", string(statusOut))
	t.Logf("thread/resume thread=%s post-resume-turn=%s history-events=%d", threadID, third.ref.ProviderTurnID, len(history1))
}

// TestLiveNativePrerequisiteDiagnostics collects independent, explicitly
// non-acceptance evidence. The wrapper runs it even when the fatal one-thread
// chain fails, so one failure does not hide the remaining host prerequisites.
func TestLiveNativePrerequisiteDiagnostics(t *testing.T) {
	requireLiveCodex(t)
	workspace := newDisposableGitWorktree(t)
	boundary := nativePathBoundary{
		Workspace: workspace, TMPDIR: os.Getenv("TMPDIR"),
		ExcludeSlashTmp: true, ExcludeTmpdirEnvVar: true,
	}

	t.Run("filesystem host preflight", func(t *testing.T) {
		target, cleanup, err := newSafeNegativeTarget(boundary)
		if err != nil {
			t.Fatalf("diagnostic_only=true diagnostic=filesystem error=%v", err)
		}
		defer cleanup()
		raw, _ := json.Marshal(map[string]any{
			"diagnostic_only": true, "diagnostic": "filesystem_host_preflight",
			"target": target, "writable_roots": []string{workspace},
		})
		t.Logf("diagnostic=%s", raw)
	})

	t.Run("network host preflight", func(t *testing.T) {
		probe := newControlledNetworkProbe(t)
		connections := probe.hostPreflight(t)
		raw, _ := json.Marshal(map[string]any{
			"diagnostic_only": true, "diagnostic": "network_host_preflight",
			"endpoint": probe.server.URL, "accepted_connections": connections,
		})
		t.Logf("diagnostic=%s", raw)
	})

	t.Run("interrupt quiescence", func(t *testing.T) {
		d := New(livePlugin{bin: liveCodexBin(t)}, slog.New(slog.DiscardHandler))
		opened := startLiveConversation(t, d, workspace)
		defer func() { _ = opened.Close() }()
		ctx := phaseContext(t, 25*time.Second)
		ref, err := opened.SendTurn(ctx, ports.ChatUserMessage{
			Text:            "Run this exact local command and wait: for i in 1 2 3 4 5; do echo diagnostic-$i | tee -a diagnostic-interrupt.log; sleep 3; done",
			ClientMessageID: "diagnostic-only-interrupt", Origin: domain.MessageOriginHuman,
		})
		if err != nil {
			t.Fatalf("diagnostic_only=true start interrupt turn: %v", err)
		}
		waitForLiveTurnActivity(t, ctx, opened.Events(), ref.ProviderTurnID)
		logPath := filepath.Join(workspace, "diagnostic-interrupt.log")
		waitForNonEmptyFile(t, ctx, logPath)
		interruptErr := opened.Interrupt(ctx, ref.ProviderTurnID)
		forced := errors.Is(interruptErr, ports.ErrChatInterruptRestartRequired)
		if interruptErr != nil && !forced {
			t.Fatalf("diagnostic_only=true interrupt: %v", interruptErr)
		}
		if state := waitForLiveTurnCompletion(t, ctx, opened.Events(), ref.ProviderTurnID); state != domain.TurnStateInterrupted {
			t.Fatalf("diagnostic_only=true interrupt state=%s", state)
		}
		before := snapshotInterruptFile(t, logPath)
		if forced {
			time.Sleep(5500 * time.Millisecond)
			if after := snapshotInterruptFile(t, logPath); after != before {
				t.Fatalf("diagnostic_only=true interrupt effect changed after process-tree kill: before=%+v after=%+v", before, after)
			}
		} else {
			assertInterruptQuiescent(t, opened.Events(), ref.ProviderTurnID, logPath, before, 5500*time.Millisecond)
		}
		raw, _ := json.Marshal(map[string]any{
			"diagnostic_only": true, "diagnostic": "interrupt_quiescence", "turn_id": ref.ProviderTurnID, "process_tree_killed": forced,
		})
		t.Logf("diagnostic=%s", raw)
	})
}

const (
	persistentSubstrateGoMod  = "module example.com/substrate\n\ngo 1.22\n"
	persistentSubstrateSource = "package substrate\n\nfunc Value() string { return \"wrong\" }\n"
	persistentSubstrateTest   = `package substrate

import "testing"

func TestValue(t *testing.T) {
	if Value() != "right" {
		t.Fatalf("Value = %q", Value())
	}
}
`
)

func writePersistentSubstrateFixture(t *testing.T, workspace string) {
	t.Helper()
	for name, content := range map[string]string{
		"go.mod":        persistentSubstrateGoMod,
		"proof.go":      persistentSubstrateSource,
		"proof_test.go": persistentSubstrateTest,
	} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write persistent substrate fixture %s: %v", name, err)
		}
	}
}

func TestPersistentSubstrateFixtureProducesExactAssertionFailure(t *testing.T) {
	workspace := t.TempDir()
	writePersistentSubstrateFixture(t, workspace)
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = workspace
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("fixture unexpectedly passed:\n%s", output)
	}
	result := liveTurnResult{activities: []liveActivityEvidence{{
		status:  domain.ActivityStatusFailed,
		command: "go test ./...",
		output:  string(output),
	}}}
	if !result.failedTestValueAssertion() {
		t.Fatalf("fixture did not produce the exact TestValue assertion:\n%s", output)
	}
}

func requireLiveCodex(t *testing.T) {
	t.Helper()
	if os.Getenv("KENNEL_CODEX_LIVE") != "1" {
		t.Skip("set KENNEL_CODEX_LIVE=1")
	}
}

func canonicalLiveCodexBinary(path string) (string, error) {
	if path == "" {
		return "", errors.New("canonical Codex runtime is required")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if canonical != path {
		return "", fmt.Errorf("path %q is not canonical; target is %q", path, canonical)
	}
	return canonical, nil
}

func liveCodexBin(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("KENNEL_CODEX_BIN")
	canonical, err := canonicalLiveCodexBinary(bin)
	if err != nil {
		t.Fatalf("invalid KENNEL_CODEX_BIN: %v", err)
	}
	t.Logf("runtime_identity selected=%q canonical=%q", os.Getenv("KENNEL_CODEX_SELECTED_BIN"), canonical)
	return canonical
}

func startLiveConversation(t *testing.T, d *Driver, workspace string) ports.ChatConversation {
	t.Helper()
	ctx := phaseContext(t, 30*time.Second)
	opened, err := d.Start(ctx, ports.ChatStartConfig{
		SessionID: "kennel-live-persistent-substrate", WorkspacePath: workspace,
		Env: liveCodexEnv(), Permissions: ports.PermissionModeAcceptEdits,
		SystemPrompt: nativeWorktreeProfileInstructions, NativeSandboxProfile: nativeLiveProfile(),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return opened
}

func nativeLiveProfile() *ports.ChatNativeSandboxProfile {
	return &ports.ChatNativeSandboxProfile{
		Sandbox: ports.ChatNativeSandboxWorkspaceWrite, NetworkAccess: false, WritableRoots: []string{},
		ExcludeSlashTmp: true, ExcludeTmpdirEnvVar: true,
	}
}

func logLatestNativePolicyEvidence(t *testing.T, conv ports.ChatConversation, boundary ports.ChatNativePolicyBoundary, turnID string) {
	t.Helper()
	reader, ok := conv.(ports.ChatNativePolicyEvidenceReader)
	if !ok {
		t.Fatalf("conversation %T has no native policy evidence", conv)
	}
	records := reader.NativePolicyEvidence()
	if len(records) == 0 {
		t.Fatal("native policy evidence is empty")
	}
	first := records[0]
	if err := validateNativePolicyEvidence(records, nativeTurnSandboxPolicy(), first.ThreadID, first.RuntimeBinarySHA256, first.ProtocolDigest); err != nil {
		t.Fatalf("invalid native policy evidence chain: %v", err)
	}
	record := records[len(records)-1]
	if record.Boundary != boundary || (turnID != "" && record.ProviderTurnID != turnID) {
		t.Fatalf("latest policy record = %+v, want boundary=%s turn=%s", record, boundary, turnID)
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal native policy evidence: %v", err)
	}
	t.Logf("policy_record=%s", raw)
}

func nativeTurnSandboxPolicy() map[string]any {
	return map[string]any{
		"type": "workspaceWrite", "networkAccess": false, "writableRoots": []string{},
		"excludeSlashTmp": true, "excludeTmpdirEnvVar": true,
	}
}

type interruptFileSnapshot struct {
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	LineCount int    `json:"line_count"`
}

func snapshotInterruptFile(t *testing.T, path string) interruptFileSnapshot {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read interrupt file: %v", err)
	}
	return interruptFileSnapshot{Size: int64(len(content)), SHA256: fmt.Sprintf("%x", sha256.Sum256(content)), LineCount: bytes.Count(content, []byte("\n"))}
}

type interruptDrain struct{ active map[string]bool }

func newInterruptDrain() *interruptDrain { return &interruptDrain{active: map[string]bool{}} }

func (d *interruptDrain) observe(ev ports.ChatEvent, turnID string) error {
	if ev.ProviderTurnID != turnID {
		return nil
	}
	switch ev.Kind {
	case ports.ChatEventActivityStarted:
		if ev.ProviderItemID == "" {
			return errors.New("post-interrupt activity has no item id")
		}
		d.active[ev.ProviderItemID] = true
	case ports.ChatEventActivityCompleted:
		if ev.ProviderItemID == "" {
			return errors.New("post-interrupt completion has no item id")
		}
		if ev.ActivityStatus == domain.ActivityStatusCompleted {
			return fmt.Errorf("item %s completed successfully after interrupt", ev.ProviderItemID)
		}
		delete(d.active, ev.ProviderItemID)
	case ports.ChatEventCommandOutputDelta, ports.ChatEventActivityText:
		if ev.Delta != "" {
			return fmt.Errorf("continuing output after interrupt for item %s", ev.ProviderItemID)
		}
	}
	return nil
}

func (d *interruptDrain) settled() bool { return len(d.active) == 0 }

func TestInterruptDrainInterleavings(t *testing.T) {
	failed := domain.ActivityStatusFailed
	completed := domain.ActivityStatusCompleted
	tests := []struct {
		name    string
		events  []ports.ChatEvent
		settled bool
		wantErr bool
	}{
		{name: "late start matched failed completion", events: []ports.ChatEvent{{Kind: ports.ChatEventActivityStarted, ProviderTurnID: "t", ProviderItemID: "i"}, {Kind: ports.ChatEventActivityCompleted, ProviderTurnID: "t", ProviderItemID: "i", ActivityStatus: failed}}, settled: true},
		{name: "unmatched item", events: []ports.ChatEvent{{Kind: ports.ChatEventActivityStarted, ProviderTurnID: "t", ProviderItemID: "i"}}, settled: false},
		{name: "success after interrupt", events: []ports.ChatEvent{{Kind: ports.ChatEventActivityCompleted, ProviderTurnID: "t", ProviderItemID: "i", ActivityStatus: completed}}, settled: true, wantErr: true},
		{name: "output after interrupt", events: []ports.ChatEvent{{Kind: ports.ChatEventCommandOutputDelta, ProviderTurnID: "t", ProviderItemID: "i", Delta: "late"}}, settled: true, wantErr: true},
		{name: "other turn ignored", events: []ports.ChatEvent{{Kind: ports.ChatEventActivityStarted, ProviderTurnID: "other", ProviderItemID: "i"}}, settled: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := newInterruptDrain()
			var err error
			for _, ev := range tc.events {
				if err = d.observe(ev, "t"); err != nil {
					break
				}
			}
			if (err != nil) != tc.wantErr || d.settled() != tc.settled {
				t.Fatalf("err=%v settled=%t, want error=%t settled=%t", err, d.settled(), tc.wantErr, tc.settled)
			}
		})
	}
}

func assertInterruptQuiescent(t *testing.T, events <-chan ports.ChatEvent, turnID, path string, before interruptFileSnapshot, wait time.Duration) {
	t.Helper()
	drain := newInterruptDrain()
	providerQuiescent := true
	providerReason := "provider_items_settled"
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	stableTicker := time.NewTicker(100 * time.Millisecond)
	defer stableTicker.Stop()
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatal("event stream closed during interrupt quiescence")
			}
			if ev.ProviderTurnID != turnID {
				continue
			}
			if err := drain.observe(ev, turnID); err != nil {
				providerQuiescent = false
				providerReason = "provider_activity_after_interrupt"
			}
		case <-stableTicker.C:
			if after := snapshotInterruptFile(t, path); after != before {
				t.Fatalf("interrupt file changed during quiescence: before=%+v after=%+v", before, after)
			}
		case <-deadline.C:
			if !drain.settled() {
				providerQuiescent = false
				providerReason = "provider_items_unsettled"
			}
			after := snapshotInterruptFile(t, path)
			if after != before {
				t.Fatalf("interrupt file changed during quiescence: before=%+v after=%+v", before, after)
			}
			raw, _ := json.Marshal(map[string]any{"turn_id": turnID, "before": before, "after": after, "effect_quiescent": true, "compatible": true, "governance": map[bool]string{true: "full", false: "partial"}[providerQuiescent], "governance_reason": providerReason})
			t.Logf("interrupt_quiescence=%s", raw)
			return
		}
	}
}

func phaseContext(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}

// A live model turn is bounded by liveness, not by a short wall clock. The outer
// cap protects CI from a provider that emits noise forever; the idle window is
// the actual stall rule and is reset by every meaningful turn event.
const (
	liveTurnTotalLimit = 12 * time.Minute
	liveTurnIdleLimit  = 3 * time.Minute
)

func liveTurnContext(t *testing.T) context.Context {
	t.Helper()
	return phaseContext(t, liveTurnTotalLimit)
}

func meaningfulTurnLiveness(ev ports.ChatEvent) bool {
	switch ev.Kind {
	case ports.ChatEventTurnStarted,
		ports.ChatEventMessageDelta, ports.ChatEventMessageCompleted,
		ports.ChatEventReasoningDelta,
		ports.ChatEventActivityStarted, ports.ChatEventActivityCompleted,
		ports.ChatEventCommandOutputDelta, ports.ChatEventCommandInput,
		ports.ChatEventApprovalRequested, ports.ChatEventInputRequested,
		ports.ChatEventUsage, ports.ChatEventPlanUpdated, ports.ChatEventTurnDiff:
		return true
	default:
		return false
	}
}

func resetIdleTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(liveTurnIdleLimit)
}

func assertCanonicalPathFile(t *testing.T, observedFile, expected string) {
	t.Helper()
	got, err := os.ReadFile(observedFile)
	if err != nil {
		t.Fatalf("read %s: %v", observedFile, err)
	}
	canonicalObserved, err := filepath.EvalSymlinks(strings.TrimSpace(string(got)))
	if err != nil {
		t.Fatalf("canonicalize observed path from %s: %v", observedFile, err)
	}
	canonicalExpected, err := filepath.EvalSymlinks(expected)
	if err != nil {
		t.Fatalf("canonicalize expected path %s: %v", expected, err)
	}
	if filepath.Clean(canonicalObserved) != filepath.Clean(canonicalExpected) {
		t.Fatalf("%s = %q (canonical %q), want %q (canonical %q)", observedFile, got, canonicalObserved, expected, canonicalExpected)
	}
}

func assertFileTrimmed(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

func waitForNonEmptyFile(t *testing.T, ctx context.Context, path string) {
	t.Helper()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if info, err := os.Stat(path); err == nil && info.Size() > 0 {
				return
			}
		case <-ctx.Done():
			t.Fatalf("waiting for nonempty %s: %v", path, ctx.Err())
		}
	}
}

func historyIDs(t *testing.T, events []ports.ChatEvent) []string {
	t.Helper()
	ids := make([]string, 0, len(events))
	seen := map[string]bool{}
	for _, ev := range events {
		if ev.ProviderEventID == "" {
			t.Fatalf("empty history event id: %+v", ev)
		}
		if seen[ev.ProviderEventID] {
			t.Fatalf("duplicate history event id %q", ev.ProviderEventID)
		}
		seen[ev.ProviderEventID] = true
		ids = append(ids, ev.ProviderEventID)
	}
	return ids
}

func assertHistoryTurns(t *testing.T, events []ports.ChatEvent, expected map[string]string) {
	t.Helper()
	type facts struct {
		started, user, completed bool
		clientID                 string
	}
	got := map[string]*facts{}
	for turnID := range expected {
		got[turnID] = &facts{}
	}
	for _, ev := range events {
		f := got[ev.ProviderTurnID]
		if f == nil {
			continue
		}
		switch ev.Kind {
		case ports.ChatEventTurnStarted:
			f.started = true
		case ports.ChatEventUserMessageCompleted:
			f.user = true
			f.clientID = ev.ClientMessageID
		case ports.ChatEventTurnCompleted:
			f.completed = true
		}
	}
	for turnID, wantClient := range expected {
		f := got[turnID]
		if !f.started || !f.user || !f.completed || f.clientID != wantClient {
			t.Errorf("history turn %s facts=%+v want client=%s", turnID, f, wantClient)
		}
	}
}

type liveActivityEvidence struct {
	itemID   string
	status   domain.ActivityStatus
	summary  string
	command  string
	cwd      string
	output   string
	detail   string
	approval bool
}

type liveTurnResult struct {
	ref                 ports.ChatTurnRef
	state               domain.TurnState
	text                string
	sawCommand          bool
	sawFailedCommand    bool
	sawCompletedCommand bool
	sawDeniedRequest    bool
	deniedRequestID     string
	activities          []liveActivityEvidence
}

func (r liveTurnResult) commandEvidence() string {
	var b strings.Builder
	for _, ev := range r.activities {
		fmt.Fprintf(&b, "item=%s status=%s summary=%q detail=%s\n", ev.itemID, ev.status, ev.summary, ev.detail)
	}
	return b.String()
}

func sourceDiagnostic(line string) (string, bool) {
	if line == "" || (line[0] != ' ' && line[0] != '\t') {
		return "", false
	}
	trimmed := strings.TrimSpace(line)
	marker := strings.Index(trimmed, ": ")
	if marker < 0 {
		return "", false
	}
	location := trimmed[:marker]
	lastColon := strings.LastIndex(location, ":")
	if lastColon <= 0 || lastColon == len(location)-1 {
		return "", false
	}
	if strings.TrimSpace(location[:lastColon]) == "" {
		return "", false
	}
	for _, r := range location[lastColon+1:] {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	return strings.TrimSpace(trimmed[marker+2:]), true
}

func exactTestRecord(line, status, name string) bool {
	prefix := "--- " + status + ": " + name
	if strings.TrimLeft(line, " \t") != line {
		return false
	}
	line = strings.TrimRight(line, "\r")
	if line == prefix {
		return true
	}
	if !strings.HasPrefix(line, prefix+" (") || !strings.HasSuffix(line, ")") {
		return false
	}
	duration := strings.TrimSuffix(strings.TrimPrefix(line, prefix+" ("), ")")
	_, err := time.ParseDuration(duration)
	return err == nil
}

func hasExpectedGoTestFailure(output string) bool {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	var pendingDiagnostics []string
	activeName := ""
	afterTestValueFail := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		frameName := ""
		for _, prefix := range []string{"=== RUN   ", "=== NAME  ", "=== PAUSE ", "=== CONT  "} {
			if strings.HasPrefix(line, prefix) && len(line) > len(prefix) {
				frameName = strings.TrimSpace(line[len(prefix):])
				break
			}
		}
		if frameName != "" {
			activeName = frameName
			afterTestValueFail = false
			continue
		}
		if strings.HasPrefix(trimmed, "--- FAIL:") || strings.HasPrefix(trimmed, "--- PASS:") || strings.HasPrefix(trimmed, "--- SKIP:") {
			isTestValueFail := exactTestRecord(line, "FAIL", "TestValue")
			if isTestValueFail {
				for _, diagnostic := range pendingDiagnostics {
					if diagnostic == `Value = "wrong"` {
						return true
					}
				}
			}
			pendingDiagnostics = nil
			activeName = ""
			afterTestValueFail = isTestValueFail
			continue
		}
		if diagnostic, ok := sourceDiagnostic(line); ok {
			if afterTestValueFail && diagnostic == `Value = "wrong"` {
				return true
			}
			if activeName == "TestValue" {
				pendingDiagnostics = append(pendingDiagnostics, diagnostic)
			}
			continue
		}
		// In non-verbose output only immediately associated indented diagnostics may
		// follow the FAIL record. Any other nonblank line closes the block.
		if afterTestValueFail && trimmed != "" {
			afterTestValueFail = false
		}
	}
	return false
}
func (r liveTurnResult) failedTestValueAssertion() bool {
	for _, ev := range r.activities {
		if ev.status == domain.ActivityStatusFailed && hasExpectedGoTestFailure(ev.output) {
			return true
		}
	}
	return false
}

func exactOperationArgument(command, operation string) bool {
	for _, field := range strings.Fields(command) {
		candidate := strings.Trim(field, "'\"")
		candidate = strings.TrimRight(candidate, ";")
		if strings.Contains(operation, "://") {
			if candidate == operation {
				return true
			}
			continue
		}
		if filepath.IsAbs(candidate) && filepath.Clean(candidate) == filepath.Clean(operation) {
			return true
		}
	}
	return false
}

func failedCommandPolicyEnforcement(ev liveActivityEvidence, operation string) bool {
	if ev.status != domain.ActivityStatusFailed || ev.approval || !exactOperationArgument(ev.command, operation) {
		return false
	}
	semantics := strings.ToLower(ev.summary + "\n" + ev.output)
	for _, marker := range []string{"operation not permitted", "permission denied", "denied by policy", "network access denied", "sandbox policy denied", "blocked by sandbox policy"} {
		if strings.Contains(semantics, marker) {
			return true
		}
	}
	return false
}

func approvalPolicyDenial(ev liveActivityEvidence, operation string) bool {
	if !ev.approval || !exactOperationArgument(ev.command, operation) {
		return false
	}
	semantics := strings.ToLower(ev.summary + "\n" + ev.output)
	for _, marker := range []string{"approval required", "request approval", "permission approval", "policy approval"} {
		if strings.Contains(semantics, marker) {
			return true
		}
	}
	return false
}

func governanceReason(level string) string {
	if level == "full" {
		return "provider_operation_attributed"
	}
	return "provider_operation_evidence_unavailable"
}

func (r liveTurnResult) denialAttributedTo(operation string) bool {
	for _, ev := range r.activities {
		if failedCommandPolicyEnforcement(ev, operation) || approvalPolicyDenial(ev, operation) {
			return true
		}
	}
	return false
}

func (r liveTurnResult) sanitizedEvidence() string {
	const limit = 4096
	text := fmt.Sprintf("turn=%s state=%s request=%s activities=[%s]", r.ref.ProviderTurnID, r.state, r.deniedRequestID, r.commandEvidence())
	if len(text) > limit {
		return text[:limit] + "...[truncated]"
	}
	return text
}

func runLiveTurn(t *testing.T, ctx context.Context, conv ports.ChatConversation, msg ports.ChatUserMessage) liveTurnResult {
	t.Helper()
	ref, err := conv.SendTurn(ctx, msg)
	if err != nil {
		t.Fatalf("SendTurn(%s): %v", msg.ClientMessageID, err)
	}
	result := liveTurnResult{ref: ref}
	idle := time.NewTimer(liveTurnIdleLimit)
	defer idle.Stop()
	for {
		select {
		case ev, ok := <-conv.Events():
			if !ok {
				t.Fatalf("event stream closed during turn %s", ref.ProviderTurnID)
			}
			// Liveness must belong to this turn. Thread/account-level usage and rate
			// updates can continue while a model turn is dead; letting empty-turn
			// events reset the timer turns background noise into a false heartbeat.
			if ev.ProviderTurnID != ref.ProviderTurnID {
				continue
			}
			if meaningfulTurnLiveness(ev) {
				resetIdleTimer(idle)
			}
			switch ev.Kind {
			case ports.ChatEventActivityStarted, ports.ChatEventCommandOutputDelta:
				result.sawCommand = true
			case ports.ChatEventActivityCompleted:
				if ev.ActivityKind == domain.ActivityKindCommand {
					result.activities = append(result.activities, nativeToolEvidence(ev, false))
					result.sawCommand = true
					if ev.ActivityStatus == domain.ActivityStatusFailed {
						result.sawFailedCommand = true
					}
					if ev.ActivityStatus == domain.ActivityStatusCompleted {
						result.sawCompletedCommand = true
					}
				}
			case ports.ChatEventMessageCompleted:
				result.text += ev.Text + "\n"
			case ports.ChatEventTurnCompleted:
				result.state = ev.TurnState
				return result
			case ports.ChatEventApprovalRequested:
				t.Fatalf("unexpected approval request %s under default live-test permissions", ev.RequestID)
			case ports.ChatEventInputRequested:
				t.Fatalf("unexpected user-input request %s during deterministic live turn", ev.RequestID)
			case ports.ChatEventControllerState:
				if ev.ControllerState == ports.ChatControllerStopped {
					t.Fatalf("controller stopped during turn %s: %v", ref.ProviderTurnID, ev.Err)
				}
			}
		case <-idle.C:
			t.Fatalf("turn %s stalled: no meaningful provider event for %s", ref.ProviderTurnID, liveTurnIdleLimit)
		case <-ctx.Done():
			t.Fatalf("turn %s exceeded total proof bound: %v", ref.ProviderTurnID, ctx.Err())
		}
	}
}

func runLiveTurnAllowDeniedRequest(t *testing.T, ctx context.Context, conv ports.ChatConversation, msg ports.ChatUserMessage) liveTurnResult {
	t.Helper()
	ref, err := conv.SendTurn(ctx, msg)
	if err != nil {
		t.Fatalf("SendTurn(%s): %v", msg.ClientMessageID, err)
	}
	result := liveTurnResult{ref: ref}
	idle := time.NewTimer(liveTurnIdleLimit)
	defer idle.Stop()
	for {
		select {
		case ev, ok := <-conv.Events():
			if !ok {
				t.Fatalf("event stream closed during denied turn %s", ref.ProviderTurnID)
			}
			// Liveness must belong to this turn. Thread/account-level usage and rate
			// updates can continue while a model turn is dead; letting empty-turn
			// events reset the timer turns background noise into a false heartbeat.
			if ev.ProviderTurnID != ref.ProviderTurnID {
				continue
			}
			if meaningfulTurnLiveness(ev) {
				resetIdleTimer(idle)
			}
			switch ev.Kind {
			case ports.ChatEventActivityStarted, ports.ChatEventCommandOutputDelta:
				result.sawCommand = true
			case ports.ChatEventActivityCompleted:
				if ev.ActivityKind == domain.ActivityKindCommand {
					result.activities = append(result.activities, nativeToolEvidence(ev, false))
					result.sawCommand = true
					if ev.ActivityStatus == domain.ActivityStatusFailed {
						result.sawFailedCommand = true
					}
					if ev.ActivityStatus == domain.ActivityStatusCompleted {
						result.sawCompletedCommand = true
					}
				}
			case ports.ChatEventApprovalRequested:
				result.sawDeniedRequest = true
				result.deniedRequestID = ev.RequestID
				result.activities = append(result.activities, nativeToolEvidence(ev, true))
				if err := conv.ResolveRequest(ctx, ev.RequestID, ports.ChatDecision{ID: "cancel"}); err != nil {
					t.Fatalf("deny profile escape request %s: %v", ev.RequestID, err)
				}
			case ports.ChatEventInputRequested:
				t.Fatalf("unexpected user-input request %s during boundary proof", ev.RequestID)
			case ports.ChatEventMessageCompleted:
				result.text += ev.Text + "\n"
			case ports.ChatEventTurnCompleted:
				result.state = ev.TurnState
				return result
			case ports.ChatEventControllerState:
				if ev.ControllerState == ports.ChatControllerStopped {
					t.Fatalf("controller stopped during denied turn: %v", ev.Err)
				}
			}
		case <-idle.C:
			t.Fatalf("denied turn %s stalled: no meaningful provider event for %s", ref.ProviderTurnID, liveTurnIdleLimit)
		case <-ctx.Done():
			t.Fatalf("denied turn %s exceeded total proof bound: %v", ref.ProviderTurnID, ctx.Err())
		}
	}
}

func nativeToolEvidence(ev ports.ChatEvent, approval bool) liveActivityEvidence {
	raw := map[string]any{}
	_ = json.Unmarshal(ev.Detail, &raw)
	stringField := func(name string) string { value, _ := raw[name].(string); return value }
	output := stringField("output")
	if len(output) > 1024 {
		output = output[:1024] + "...[truncated]"
	}
	safe := map[string]any{}
	for _, key := range []string{"command", "cwd", "exitCode", "durationMs"} {
		if value, ok := raw[key]; ok {
			safe[key] = value
		}
	}
	if output != "" {
		safe["output"] = output
	}
	encoded, _ := json.Marshal(safe)
	return liveActivityEvidence{
		itemID: ev.ProviderItemID, status: ev.ActivityStatus, summary: ev.Summary,
		command: stringField("command"), cwd: stringField("cwd"), output: output,
		detail: string(encoded), approval: approval,
	}
}

func assertPathOutside(t *testing.T, candidate, workspace string) {
	t.Helper()
	c, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		t.Fatal(err)
	}
	w, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(w, c)
	if err != nil {
		t.Fatal(err)
	}
	if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		t.Fatalf("negative-probe path %q is inside allowed worktree %q", c, w)
	}
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(got), want) {
		t.Fatalf("%s does not contain %q: %q", path, want, got)
	}
}

func waitForLiveTurnActivity(t *testing.T, ctx context.Context, events <-chan ports.ChatEvent, turnID string) {
	t.Helper()
	started := false
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("event stream closed before activity for turn %s", turnID)
			}
			if ev.ProviderTurnID != "" && ev.ProviderTurnID != turnID {
				continue
			}
			switch ev.Kind {
			case ports.ChatEventTurnStarted:
				started = true
			case ports.ChatEventActivityStarted, ports.ChatEventCommandOutputDelta:
				if started {
					return
				}
			case ports.ChatEventTurnCompleted:
				t.Fatalf("turn %s completed before interrupt test observed activity", turnID)
			}
		case <-ctx.Done():
			t.Fatalf("waiting for activity on turn %s: %v", turnID, ctx.Err())
		}
	}
}

func waitForLiveTurnCompletion(t *testing.T, ctx context.Context, events <-chan ports.ChatEvent, turnID string) domain.TurnState {
	t.Helper()
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("event stream closed before completion for turn %s", turnID)
			}
			if ev.Kind == ports.ChatEventTurnCompleted && ev.ProviderTurnID == turnID {
				return ev.TurnState
			}
		case <-ctx.Done():
			t.Fatalf("waiting for completion on turn %s: %v", turnID, ctx.Err())
		}
	}
}

// livePlugin stands in for Kennel's Codex agent plugin so this test exercises the
// driver rather than binary discovery.
type livePlugin struct{ bin string }

func (p livePlugin) ResolveBinary(context.Context) (string, error) { return p.bin, nil }
func (p livePlugin) AuthStatus(context.Context) (ports.AgentAuthStatus, error) {
	return ports.AgentAuthStatusAuthorized, nil
}

func envMap() map[string]string { return liveCodexEnv() }

// liveCodexEnv is deliberately an allowlist. It carries provider auth/runtime and
// OS transport facts only; unrelated test-runner and user variables never reach
// the live subprocess or its logs.
func liveCodexEnv() map[string]string {
	allowed := []string{
		"HOME", "USER", "LOGNAME", "PATH", "SHELL", "TMPDIR", "TEMP", "TMP",
		"CODEX_HOME", "OPENAI_API_KEY", "OPENAI_BASE_URL",
		"SSL_CERT_FILE", "SSL_CERT_DIR", "REQUESTS_CA_BUNDLE", "CURL_CA_BUNDLE",
		"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY",
		"http_proxy", "https_proxy", "all_proxy", "no_proxy",
		"LANG", "LC_ALL", "LC_CTYPE", "TERM",
	}
	out := make(map[string]string, len(allowed))
	for _, name := range allowed {
		if value, ok := os.LookupEnv(name); ok {
			out[name] = value
		}
	}
	return out
}

func newDisposableGitWorktree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	workspace := filepath.Join(root, "worktree")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatalf("create source repo: %v", err)
	}
	seedGitWorkspace(t, source)
	cmd := exec.Command("git", "worktree", "add", "--detach", workspace, "HEAD")
	cmd.Dir = source
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v: %s", err, out)
	}
	t.Cleanup(func() {
		cmd := exec.Command("git", "worktree", "remove", "--force", workspace)
		cmd.Dir = source
		_ = cmd.Run()
	})
	return workspace
}

func seedGitWorkspace(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "."},
		{"-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "-q", "-m", "seed"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}
