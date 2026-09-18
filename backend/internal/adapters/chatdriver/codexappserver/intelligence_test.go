package codexappserver

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

func testIntelligenceRequest() ports.LLMRequest {
	return ports.LLMRequest{
		System:     "You are a bounded planner.",
		User:       "Draft the smallest plan for the approved outcome.",
		SchemaName: "plan_draft",
		Schema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []any{"summary"},
			"properties":           map[string]any{"summary": map[string]any{"type": "string"}},
		},
	}
}

func TestIntelligenceCapabilityFailsClosedBeforePermissionProfileProtocol(t *testing.T) {
	d, _ := newTestDriver(t)
	d.versionProbe = func(context.Context, string) (string, error) { return "codex-cli 0.153.3", nil }
	if err := d.ProbeIntelligence(context.Background()); !errors.Is(err, ports.ErrChatDriverIncompatible) {
		t.Fatalf("ProbeIntelligence error = %v, want incompatible permission-profile protocol", err)
	}
}

func TestIntelligenceClientPinsBoundedStructuredTurn(t *testing.T) {
	d, srv := newTestDriver(t)
	client := NewIntelligenceClient(d, IntelligenceConfig{Model: "approved-model", Effort: "high", Timeout: time.Second})

	result := make(chan struct {
		response ports.LLMResponse
		err      error
	}, 1)
	go func() {
		response, err := client.Complete(context.Background(), testIntelligenceRequest())
		result <- struct {
			response ports.LLMResponse
			err      error
		}{response, err}
	}()

	start := srv.awaitFrame(func(f frame) bool { return f.Method == "thread/start" })
	var startParams struct {
		ApprovalPolicy string         `json:"approvalPolicy"`
		Permissions    string         `json:"permissions"`
		RuntimeRoots   []string       `json:"runtimeWorkspaceRoots"`
		Environments   []any          `json:"environments"`
		Ephemeral      bool           `json:"ephemeral"`
		Model          string         `json:"model"`
		Config         map[string]any `json:"config"`
	}
	if err := json.Unmarshal(start.Params, &startParams); err != nil {
		t.Fatalf("thread/start params: %v", err)
	}
	if startParams.ApprovalPolicy != "never" || startParams.Permissions != intelligencePermissionProfile || !startParams.Ephemeral {
		t.Fatalf("unsafe intelligence thread posture: %+v", startParams)
	}
	if len(startParams.RuntimeRoots) != 1 || len(startParams.Environments) != 0 {
		t.Fatalf("intelligence runtime scope = roots:%v environments:%v", startParams.RuntimeRoots, startParams.Environments)
	}
	if startParams.Model != "approved-model" {
		t.Fatalf("thread model = %q, want approved-model", startParams.Model)
	}
	if got, ok := startParams.Config["mcp_servers"].(map[string]any); !ok || len(got) != 0 {
		t.Fatalf("ambient MCP config = %#v, want an explicit empty map", startParams.Config["mcp_servers"])
	}
	if _, found := startParams.Config["permissions"]; found {
		t.Fatalf("native permission table was shadowed: %#v", startParams.Config["permissions"])
	}
	if _, found := startParams.Config["default_permissions"]; found {
		t.Fatalf("native default permission was shadowed: %#v", startParams.Config["default_permissions"])
	}
	features, ok := startParams.Config["features"].(map[string]any)
	if !ok || features["apps"] != false {
		t.Fatalf("ambient app config = %#v", startParams.Config["features"])
	}
	if features["plugins"] == false {
		t.Fatalf("Kennel plugins were disabled: %#v", startParams.Config["features"])
	}
	if _, found := startParams.Config["skills"]; found {
		t.Fatalf("repository-local skills were overconstrained: %#v", startParams.Config["skills"])
	}

	turn := srv.awaitFrame(func(f frame) bool { return f.Method == "turn/start" })
	var turnParams struct {
		Model          string          `json:"model"`
		Effort         string          `json:"effort"`
		ApprovalPolicy string          `json:"approvalPolicy"`
		Permissions    string          `json:"permissions"`
		RuntimeRoots   []string        `json:"runtimeWorkspaceRoots"`
		OutputSchema   json.RawMessage `json:"outputSchema"`
		ClientMessage  string          `json:"clientUserMessageId"`
		Input          []struct {
			Text string `json:"text"`
		} `json:"input"`
	}
	if err := json.Unmarshal(turn.Params, &turnParams); err != nil {
		t.Fatalf("turn/start params: %v", err)
	}
	if turnParams.Model != "approved-model" || turnParams.Effort != "high" || turnParams.ApprovalPolicy != "never" || turnParams.Permissions != intelligencePermissionProfile {
		t.Fatalf("turn selection/posture = %+v", turnParams)
	}
	if len(turnParams.RuntimeRoots) != 1 || turnParams.RuntimeRoots[0] != startParams.RuntimeRoots[0] {
		t.Fatalf("turn runtime roots = %#v", turnParams.RuntimeRoots)
	}
	if len(turnParams.Input) != 1 || !strings.Contains(turnParams.Input[0].Text, "Do not use tools") {
		t.Fatalf("packet-only prompt did not retain no-tool instruction: %#v", turnParams.Input)
	}
	if !json.Valid(turnParams.OutputSchema) || turnParams.ClientMessage == "" {
		t.Fatalf("structured output or idempotency key missing: schema=%s id=%q", turnParams.OutputSchema, turnParams.ClientMessage)
	}

	// Identity-less and stale responses are ignored. The settled response is
	// accepted only for the provider turn the adapter started.
	srv.push(`{"method":"item/completed","params":{"threadId":"thread-1","item":{"id":"missing-turn","type":"agentMessage","text":"not this request"}}}`)
	srv.push(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"status":"completed","items":[]}}}`)
	srv.push(`{"method":"item/completed","params":{"threadId":"thread-1","turnId":"stale-turn","item":{"id":"stale","type":"agentMessage","text":"not this request"}}}`)
	srv.push(`{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","item":{"id":"msg-1","type":"agentMessage","text":"{\"summary\":\"bounded\"}"}}}`)
	srv.push(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[]}}}`)

	got := <-result
	if got.err != nil {
		t.Fatalf("Complete: %v", got.err)
	}
	if string(got.response.JSON) != `{"summary":"bounded"}` {
		t.Fatalf("JSON = %s", got.response.JSON)
	}
	if got.response.EffectiveModel != "gpt-test" || got.response.NativeSessionRef != "thread-1" {
		t.Fatalf("provenance = %+v", got.response)
	}
}

func TestIntelligenceClientPreservesProviderDefaultSelection(t *testing.T) {
	d, srv := newTestDriver(t)
	client := NewIntelligenceClient(d, IntelligenceConfig{Timeout: time.Second})
	result := make(chan struct {
		response ports.LLMResponse
		err      error
	}, 1)
	go func() {
		response, err := client.Complete(context.Background(), testIntelligenceRequest())
		result <- struct {
			response ports.LLMResponse
			err      error
		}{response: response, err: err}
	}()

	start := srv.awaitFrame(func(f frame) bool { return f.Method == "thread/start" })
	var startParams map[string]any
	if err := json.Unmarshal(start.Params, &startParams); err != nil {
		t.Fatal(err)
	}
	if _, found := startParams["model"]; found {
		t.Fatalf("provider-default thread named a model: %#v", startParams["model"])
	}
	turn := srv.awaitFrame(func(f frame) bool { return f.Method == "turn/start" })
	var turnParams map[string]any
	if err := json.Unmarshal(turn.Params, &turnParams); err != nil {
		t.Fatal(err)
	}
	if _, found := turnParams["model"]; found {
		t.Fatalf("provider-default turn named a model: %#v", turnParams["model"])
	}

	srv.push(`{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","item":{"id":"msg-1","type":"agentMessage","text":"{\"summary\":\"default\"}"}}}`)
	srv.push(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[]}}}`)
	got := <-result
	if got.err != nil || got.response.EffectiveModel != "gpt-test" {
		t.Fatalf("provider-default result = %+v err=%v", got.response, got.err)
	}
}

func TestIntelligenceClientRejectsUnverifiedRepositoryToolMode(t *testing.T) {
	d, _ := newTestDriver(t)
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	client := NewIntelligenceClient(d, IntelligenceConfig{Timeout: time.Second})
	request := testIntelligenceRequest()
	request.ContextAccess = ports.ReasoningContextAccess{Mode: ports.ReasoningContextRepositoryRead, Root: root}

	_, err = client.Complete(context.Background(), request)
	var failure *ports.ReasoningFailure
	if !errors.As(err, &failure) || failure.Kind != ports.ReasoningUnavailable || !strings.Contains(failure.Error(), "not available") {
		t.Fatalf("Complete error = %v, want explicit repository-tool unavailability", err)
	}
}

func TestIntelligenceClientFailsClosedOnReportedFileChange(t *testing.T) {
	d, srv := newTestDriver(t)
	client := NewIntelligenceClient(d, IntelligenceConfig{Timeout: time.Second})
	result := make(chan error, 1)
	go func() {
		_, err := client.Complete(context.Background(), testIntelligenceRequest())
		result <- err
	}()
	srv.awaitFrame(func(f frame) bool { return f.Method == "turn/start" })
	srv.push(`{"method":"item/started","params":{"threadId":"thread-1","turnId":"turn-1","item":{"id":"change-1","type":"fileChange","changes":[]}}}`)
	err := <-result
	var failure *ports.ReasoningFailure
	if !errors.As(err, &failure) || failure.Err == nil || !strings.Contains(failure.Err.Error(), "forbidden file change") {
		t.Fatalf("Complete error = %v, want forbidden file change", err)
	}
}

func TestSendTurnRejectsMissingProviderTurnID(t *testing.T) {
	d, srv := newTestDriver(t)
	conv, err := d.connect(context.Background(), "/tmp/ws", nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conv.Close()
	conv.start("thread-1", "gpt-test", "high", nil)
	srv.respondTo("turn/start", `{"turn":{"status":"inProgress","items":[]}}`)
	_, err = conv.sendTurn(context.Background(), ports.ChatUserMessage{Text: "hello"}, nil)
	if err == nil || !strings.Contains(err.Error(), "no turn id") {
		t.Fatalf("sendTurn error = %v, want missing turn id", err)
	}
}

func TestIntelligenceClientRejectsMalformedSettledReply(t *testing.T) {
	d, srv := newTestDriver(t)
	client := NewIntelligenceClient(d, IntelligenceConfig{Timeout: time.Second})
	result := make(chan error, 1)
	go func() {
		_, err := client.Complete(context.Background(), testIntelligenceRequest())
		result <- err
	}()
	srv.awaitFrame(func(f frame) bool { return f.Method == "turn/start" })
	srv.push(`{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","item":{"id":"msg-1","type":"agentMessage","text":"not json"}}}`)
	srv.push(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[]}}}`)
	err := <-result
	var failure *ports.ReasoningFailure
	if !errors.As(err, &failure) || failure.Kind != ports.ReasoningInvalidOutput {
		t.Fatalf("err = %v, want invalid structured output", err)
	}
}

func TestIntelligenceClientCancellationInterruptsNamedTurn(t *testing.T) {
	d, srv := newTestDriver(t)
	client := NewIntelligenceClient(d, IntelligenceConfig{Timeout: time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := client.Complete(ctx, testIntelligenceRequest())
		result <- err
	}()
	srv.awaitFrame(func(f frame) bool { return f.Method == "turn/start" })
	srv.awaitResponse("turn/start")
	cancel()
	interrupt := srv.awaitFrame(func(f frame) bool { return f.Method == "turn/interrupt" })
	var params struct {
		TurnID string `json:"turnId"`
	}
	if err := json.Unmarshal(interrupt.Params, &params); err != nil || params.TurnID != "turn-1" {
		t.Fatalf("interrupt params = %s (%v)", interrupt.Params, err)
	}
	err := <-result
	var failure *ports.ReasoningFailure
	if !errors.As(err, &failure) || failure.Kind != ports.ReasoningCancelled {
		t.Fatalf("err = %v, want cancelled reasoning failure", err)
	}
}

func TestIntelligenceClientDoesNotAcceptLateCompletionAfterCancellation(t *testing.T) {
	d, srv := newTestDriver(t)
	client := NewIntelligenceClient(d, IntelligenceConfig{Timeout: time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := client.Complete(ctx, testIntelligenceRequest())
		result <- err
	}()
	srv.awaitFrame(func(f frame) bool { return f.Method == "turn/start" })
	srv.awaitResponse("turn/start")
	cancel()
	// Race late success frames against cancellation. The client may already have
	// closed its private app-server pipe; that is an equally valid refusal of the
	// late result and must not turn this cancellation regression into a flaky
	// harness-write failure.
	_ = srv.tryPush(`{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","item":{"id":"msg-1","type":"agentMessage","text":"late"}}}`)
	_ = srv.tryPush(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[]}}}`)
	err := <-result
	var failure *ports.ReasoningFailure
	if !errors.As(err, &failure) || failure.Kind != ports.ReasoningCancelled {
		t.Fatalf("err = %v, want cancelled reasoning failure", err)
	}
}

func TestIntelligenceClientRequiresCodexAuth(t *testing.T) {
	d, _ := newTestDriver(t)
	d.plugin = fakePlugin{bin: "codex", authStatus: ports.AgentAuthStatusUnauthorized}
	client := NewIntelligenceClient(d, IntelligenceConfig{})
	_, err := client.Complete(context.Background(), testIntelligenceRequest())
	var failure *ports.ReasoningFailure
	if !errors.As(err, &failure) || failure.Kind != ports.ReasoningUnauthorized {
		t.Fatalf("err = %v, want unauthorized reasoning failure", err)
	}
	if !strings.Contains(failure.Error(), "sign in") {
		t.Fatalf("auth error = %q, want actionable sign-in guidance", failure.Error())
	}
}

// The conflicting-homes falsifier: an ambient CODEX_HOME must never leak into
// a scoped planning launch, and the mission skill must be verified visible on
// the exact connection before any thread starts.
func TestStartIntelligenceScopedHomeBindsAndVerifiesMissionSkill(t *testing.T) {
	t.Setenv("CODEX_HOME", "/user/ambient-home")
	d, srv := newTestDriver(t)
	var capturedEnv []string
	realSpawn := d.spawn
	d.spawn = func(ctx context.Context, bin, workdir string, env []string) (*process, error) {
		capturedEnv = append([]string(nil), env...)
		return realSpawn(ctx, bin, workdir, env)
	}
	srv.reply("skills/list", `{"data":[{"cwd":"/tmp/ws","skills":[{"name":"mission:mission","description":"Plan a mission.","enabled":true,"scope":"plugin"}]}]}`)

	conv, err := d.startIntelligence(context.Background(), t.TempDir(), "system", "", "/scoped/mission-home")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conv.Close() }()

	scoped, leaked := false, false
	for _, entry := range capturedEnv {
		if entry == "CODEX_HOME=/scoped/mission-home" {
			scoped = true
		}
		if entry == "CODEX_HOME=/user/ambient-home" {
			leaked = true
		}
	}
	if !scoped || leaked {
		t.Fatalf("spawn env carries scoped=%v leaked-ambient=%v: %v", scoped, leaked, capturedEnv)
	}

	skillsAt, threadAt := -1, -1
	srv.mu.Lock()
	for i, f := range srv.seen {
		if f.Method == "skills/list" && skillsAt < 0 {
			skillsAt = i
		}
		if f.Method == "thread/start" && threadAt < 0 {
			threadAt = i
		}
	}
	srv.mu.Unlock()
	if skillsAt < 0 || threadAt < 0 || skillsAt > threadAt {
		t.Fatalf("mission skill visibility checked at %d, thread started at %d - want the check first", skillsAt, threadAt)
	}
}

// A plugin whose skill the harness cannot see - absent or disabled - closes
// the launch before any thread exists, however healthy its bytes on disk are.
func TestStartIntelligenceScopedHomeFailsClosedWhenMissionSkillInvisible(t *testing.T) {
	for name, skillsJSON := range map[string]string{
		"absent":   `{"data":[{"cwd":"/tmp/ws","skills":[{"name":"other:thing","enabled":true,"scope":"plugin"}]}]}`,
		"disabled": `{"data":[{"cwd":"/tmp/ws","skills":[{"name":"mission:mission","enabled":false,"scope":"plugin"}]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			d, srv := newTestDriver(t)
			srv.reply("skills/list", skillsJSON)
			_, err := d.startIntelligence(context.Background(), t.TempDir(), "system", "", "/scoped/mission-home")
			if err == nil || !strings.Contains(err.Error(), "mission:mission") {
				t.Fatalf("error = %v, want a closed launch naming mission:mission", err)
			}
			if srv.sentMethod("thread/start") {
				t.Fatal("a planning thread started without the verified mission skill")
			}
		})
	}
}
