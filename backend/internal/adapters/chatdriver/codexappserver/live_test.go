package codexappserver

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

const nativeWorktreeProfileInstructions = "You are running the codex_native_worktree_v1 compatibility proof. Use only local tools in the assigned disposable worktree; do not request wider filesystem, network, or external-effect authority. Follow exact proof commands and keep replies short."

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

	first := runLiveTurn(t, phaseContext(t, 90*time.Second), opened, ports.ChatUserMessage{
		Text: "Inspect the assigned repository, then run this exact local command: " +
			"pwd > observed-pwd.txt && git rev-parse --show-toplevel > observed-git-root.txt && " +
			"printf PERSISTENT-SUBSTRATE > substrate-marker.txt && " +
			"printf 'module example.com/substrate\\n\\ngo 1.22\\n' > go.mod && " +
			"printf 'package substrate\\n\\nfunc Value() string { return \"wrong\" }\\n' > proof.go && " +
			"printf 'package substrate\\n\\nimport \"testing\"\\n\\nfunc TestValue(t *testing.T) { if Value() != \"right\" { t.Fatalf(\"Value = %q\", Value()) } }\\n' > proof_test.go && " +
			"go test ./... . The failing test is expected; report FIRST-FAIL-OBSERVED after it runs.",
		ClientMessageID: "persistent-substrate-failing-test", Origin: domain.MessageOriginHuman,
	})
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

	second := runLiveTurn(t, phaseContext(t, 90*time.Second), opened, ports.ChatUserMessage{
		Text: "Repair the failing local test by running this exact command: " +
			"printf 'package substrate\\n\\nfunc Value() string { return \"right\" }\\n' > proof.go && " +
			"go test ./... && tr '[:upper:]' '[:lower:]' < substrate-marker.txt > turn2-derived.txt. " +
			"Then reply SECOND-REPAIRED.",
		ClientMessageID: "persistent-substrate-repair-test", Origin: domain.MessageOriginHuman,
	})
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
	if fsPositive.state != domain.TurnStateCompleted || !fsPositive.sawCompletedCommand {
		t.Fatalf("filesystem positive control failed: %s", fsPositive.sanitizedEvidence())
	}
	assertFileTrimmed(t, filepath.Join(workspace, "filesystem-positive-control.txt"), "LOCAL-WRITE-OK")

	outsideRoot := t.TempDir()
	assertPathOutside(t, outsideRoot, workspace)
	outside := filepath.Join(outsideRoot, "undeclared", "outside-sentinel.txt")
	deniedFS := runLiveTurnAllowDeniedRequest(t, phaseContext(t, 90*time.Second), opened, ports.ChatUserMessage{
		Text: "Test the profile boundary by running this exact command, without requesting wider authority: " +
			"mkdir -p " + shellQuote(filepath.Dir(outside)) + " && printf ESCAPED > " + shellQuote(outside) +
			". This operation is expected to be denied; report FS-DENIED.",
		ClientMessageID: "persistent-substrate-fs-denied", Origin: domain.MessageOriginHuman,
	})
	if deniedFS.state != domain.TurnStateCompleted || !deniedFS.denialAttributedTo(outside) {
		t.Fatalf("filesystem denial lacks operation/enforcement attribution: %s", deniedFS.sanitizedEvidence())
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		_ = os.Remove(outside)
		t.Fatalf("undeclared filesystem write escaped profile: %v", err)
	}
	t.Logf("evidence turn=%s client=%s filesystem_escape_denied=true approval_request_denied=%t", deniedFS.ref.ProviderTurnID,
		"persistent-substrate-fs-denied", deniedFS.sawDeniedRequest)

	networkPositive := runLiveTurn(t, phaseContext(t, 60*time.Second), opened, ports.ChatUserMessage{
		Text:            "Run this exact local network-tool control without making a request: curl --version > curl-version.txt. Then reply CURL-AVAILABLE.",
		ClientMessageID: "persistent-substrate-network-tool-positive", Origin: domain.MessageOriginHuman,
	})
	if networkPositive.state != domain.TurnStateCompleted || !networkPositive.sawCompletedCommand {
		t.Fatalf("curl availability control failed: %s", networkPositive.sanitizedEvidence())
	}
	assertFileContains(t, filepath.Join(workspace, "curl-version.txt"), "curl")
	const networkProbeURL = "https://example.com/"
	deniedNetwork := runLiveTurnAllowDeniedRequest(t, phaseContext(t, 90*time.Second), opened, ports.ChatUserMessage{
		Text: "Test the profile's explicit network boundary by running this exact command, without requesting wider authority: " +
			"curl --fail --silent --show-error --max-time 5 https://example.com/ >/dev/null && printf REACHED > network-reached.txt" +
			". Network is expected to be denied; report NETWORK-DENIED.",
		ClientMessageID: "persistent-substrate-network-denied", Origin: domain.MessageOriginHuman,
	})
	if deniedNetwork.state != domain.TurnStateCompleted || !deniedNetwork.denialAttributedTo(networkProbeURL) {
		t.Fatalf("network denial lacks request/enforcement attribution: %s", deniedNetwork.sanitizedEvidence())
	}
	if _, err := os.Stat(filepath.Join(workspace, "network-reached.txt")); !os.IsNotExist(err) {
		t.Fatalf("undeclared network command reached its success sentinel: %v", err)
	}
	t.Logf("evidence turn=%s client=%s network_denied=true approval_request_denied=%t", deniedNetwork.ref.ProviderTurnID,
		"persistent-substrate-network-denied", deniedNetwork.sawDeniedRequest)

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
	if err := opened.Interrupt(interruptCtx, interruptRef.ProviderTurnID); err != nil {
		t.Fatalf("turn/interrupt: %v", err)
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
		SystemPrompt: nativeWorktreeProfileInstructions,
	})
	if err != nil {
		t.Fatalf("thread/resume on fresh app-server: %v", err)
	}
	defer func() { _ = resumed.Close() }()
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
		first.ref.ProviderTurnID:           "persistent-substrate-failing-test",
		second.ref.ProviderTurnID:          "persistent-substrate-repair-test",
		fsPositive.ref.ProviderTurnID:      "persistent-substrate-fs-positive",
		networkPositive.ref.ProviderTurnID: "persistent-substrate-network-tool-positive",
		deniedFS.ref.ProviderTurnID:        "persistent-substrate-fs-denied",
		deniedNetwork.ref.ProviderTurnID:   "persistent-substrate-network-denied",
		interruptRef.ProviderTurnID:        "persistent-substrate-interrupt",
	})

	third := runLiveTurn(t, phaseContext(t, 90*time.Second), resumed, ports.ChatUserMessage{
		Text: "Run this exact local command: " +
			"wc -c < turn2-derived.txt | tr -d ' ' > resumed-count.txt. Then reply RESUMED-DONE.",
		ClientMessageID: "persistent-substrate-after-resume", Origin: domain.MessageOriginHuman,
	})
	if third.state != domain.TurnStateCompleted || !third.sawCommand {
		t.Fatalf("post-resume turn = %#v", third)
	}
	assertFileTrimmed(t, filepath.Join(workspace, "resumed-count.txt"), "20")
	t.Logf("evidence turn=%s client=%s", third.ref.ProviderTurnID, "persistent-substrate-after-resume")
	status := exec.Command("git", "status", "--short")
	status.Dir = workspace
	statusOut, err := status.CombinedOutput()
	if err != nil {
		t.Fatalf("git status --short: %v: %s", err, statusOut)
	}
	t.Logf("evidence git_status_short=%q", string(statusOut))
	t.Logf("thread/resume thread=%s post-resume-turn=%s history-events=%d", threadID, third.ref.ProviderTurnID, len(history1))
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
		SystemPrompt: nativeWorktreeProfileInstructions,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return opened
}

func phaseContext(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
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
	for {
		select {
		case ev, ok := <-conv.Events():
			if !ok {
				t.Fatalf("event stream closed during turn %s", ref.ProviderTurnID)
			}
			if ev.ProviderTurnID != "" && ev.ProviderTurnID != ref.ProviderTurnID {
				continue
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
		case <-ctx.Done():
			t.Fatalf("turn %s timed out: %v", ref.ProviderTurnID, ctx.Err())
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
	for {
		select {
		case ev, ok := <-conv.Events():
			if !ok {
				t.Fatalf("event stream closed during denied turn %s", ref.ProviderTurnID)
			}
			if ev.ProviderTurnID != "" && ev.ProviderTurnID != ref.ProviderTurnID {
				continue
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
		case <-ctx.Done():
			t.Fatalf("denied turn %s timed out: %v", ref.ProviderTurnID, ctx.Err())
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
