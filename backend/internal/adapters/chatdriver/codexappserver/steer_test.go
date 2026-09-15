package codexappserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/chatdriver/codexappserver/codexproto"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// Scripted replies must be single-line JSON: readFrame is newline-delimited, so a
// pretty-printed reply is read as several unparseable frames and the request never
// completes.

// steerConversation opens a conversation with a turn already in flight, which is the
// only state a steer is meaningful in.
func steerConversation(t *testing.T) (*conversation, *scriptedServer) {
	t.Helper()
	d, srv := newTestDriver(t)
	srv.respondTo(codexproto.MethodTurnSteer, `{"turnId":"turn-1"}`)

	opened, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = opened.Close() })

	conv, ok := opened.(*conversation)
	if !ok {
		t.Fatalf("Start returned %T, want *conversation", opened)
	}
	if _, err := conv.SendTurn(context.Background(), ports.ChatUserMessage{
		Text:   "do the long thing",
		Origin: domain.MessageOriginHuman,
	}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	return conv, srv
}

// refuseSteer scripts a raw JSON-RPC error object, so a test can reproduce the
// provider's structured refusal payload verbatim. scriptedServer.replyError only
// carries a code and a message, and the payload is the whole point here.
func refuseSteer(srv *scriptedServer, errorJSON string) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.failures[codexproto.MethodTurnSteer] = errorJSON
}

// The shape on the wire, checked against the generated params rather than a
// hand-written struct: expectedTurnId is a precondition the provider enforces, and
// sending the wrong field name would make every steer land as "no active turn".
func TestSteerSendsTheProvidersPreconditionShape(t *testing.T) {
	conv, srv := steerConversation(t)

	ref, err := conv.Steer(context.Background(), "turn-1", ports.ChatUserMessage{
		Text:            "actually, stop and just summarize",
		ClientMessageID: "steer-1",
		Origin:          domain.MessageOriginHuman,
		Content: []ports.ChatContent{{
			Type: "image", Data: "aGVsbG8=", MIMEType: "image/png",
		}},
	})
	if err != nil {
		t.Fatalf("Steer: %v", err)
	}
	// Observed against codex-cli 0.146.0: the turn id is preserved, so the guidance
	// belongs to the turn the user is already watching.
	if ref.ProviderTurnID != "turn-1" {
		t.Errorf("steered turn = %q, want turn-1", ref.ProviderTurnID)
	}

	sent := srv.awaitFrame(func(f frame) bool { return f.Method == codexproto.MethodTurnSteer })
	var params codexproto.TurnSteerParams
	if err := json.Unmarshal(sent.Params, &params); err != nil {
		t.Fatalf("turn/steer params: %v", err)
	}
	if params.ThreadID != "thread-1" {
		t.Errorf("threadId = %q, want thread-1", params.ThreadID)
	}
	if params.ExpectedTurnID != "turn-1" {
		t.Errorf("expectedTurnId = %q, want turn-1", params.ExpectedTurnID)
	}
	if params.ClientUserMessageID == nil || *params.ClientUserMessageID != "steer-1" {
		t.Errorf("clientUserMessageId = %v, want steer-1", params.ClientUserMessageID)
	}
	if len(params.Input) != 2 || params.Input[0].Type != codexproto.UserInputTypeText {
		t.Fatalf("input = %+v, want text followed by image", params.Input)
	}
	if params.Input[0].Text == nil || *params.Input[0].Text != "actually, stop and just summarize" {
		t.Errorf("input text = %v", params.Input[0].Text)
	}
	if params.Input[1].Type != codexproto.UserInputTypeImage || params.Input[1].URL == nil ||
		*params.Input[1].URL != "data:image/png;base64,aGVsbG8=" {
		t.Errorf("image input = %+v", params.Input[1])
	}
}

// An empty turn id falls back to the turn the conversation last saw, so a caller
// that has not tracked one is not forced to guess.
func TestSteerFallsBackToTheKnownActiveTurn(t *testing.T) {
	conv, srv := steerConversation(t)

	if _, err := conv.Steer(context.Background(), "", ports.ChatUserMessage{Text: "narrow it down"}); err != nil {
		t.Fatalf("Steer: %v", err)
	}
	sent := srv.awaitFrame(func(f frame) bool { return f.Method == codexproto.MethodTurnSteer })
	var params codexproto.TurnSteerParams
	if err := json.Unmarshal(sent.Params, &params); err != nil {
		t.Fatalf("turn/steer params: %v", err)
	}
	if params.ExpectedTurnID != "turn-1" {
		t.Errorf("expectedTurnId = %q, want the last known turn", params.ExpectedTurnID)
	}
}

// With no turn ever started there is nothing to name, and the provider must not be
// asked to steer the empty string.
func TestSteerWithoutAnyTurnIsTypedAndNeverReachesTheProvider(t *testing.T) {
	d, srv := newTestDriver(t)
	opened, err := d.Start(context.Background(), ports.ChatStartConfig{WorkspacePath: "/tmp/ws"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = opened.Close() }()

	_, err = opened.(*conversation).Steer(context.Background(), "", ports.ChatUserMessage{Text: "hello"})
	if !errors.Is(err, ports.ErrChatNoSteerableTurn) {
		t.Fatalf("err = %v, want ErrChatNoSteerableTurn", err)
	}
	if srv.sentMethod(codexproto.MethodTurnSteer) {
		t.Error("asked the provider to steer a turn Kennel does not have")
	}
}

func TestSteerRejectsEmptyText(t *testing.T) {
	conv, srv := steerConversation(t)

	_, err := conv.Steer(context.Background(), "turn-1", ports.ChatUserMessage{Text: "   \n"})
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("err = %v, want a rejection naming the empty text", err)
	}
	if srv.sentMethod(codexproto.MethodTurnSteer) {
		t.Error("sent an empty steer to the provider")
	}
}

// The provider's refusal when nothing is in flight. Observed verbatim from a live
// app-server: code -32600, message "no active turn to steer", no data. It reaches a
// user as "there is no turn to steer", never as an internal error.
func TestSteerTranslatesNoActiveTurnRefusal(t *testing.T) {
	conv, srv := steerConversation(t)
	refuseSteer(srv, `{"code":-32600,"message":"no active turn to steer"}`)

	_, err := conv.Steer(context.Background(), "turn-1", ports.ChatUserMessage{Text: "too late"})
	if !errors.Is(err, ports.ErrChatNoSteerableTurn) {
		t.Fatalf("err = %v, want ErrChatNoSteerableTurn", err)
	}
}

// The other refusal, and the one that is distinguishable without reading prose: a
// turn IS running but its kind cannot take guidance. Payload copied from a live
// app-server refusing a steer during a manual compaction.
func TestSteerTranslatesNotSteerableRefusalFromItsStructuredPayload(t *testing.T) {
	conv, srv := steerConversation(t)
	refuseSteer(srv, `{"code":-32600,"message":"cannot steer a compact turn",`+
		`"data":{"message":"cannot steer a compact turn",`+
		`"codexErrorInfo":{"activeTurnNotSteerable":{"turnKind":"compact"}},`+
		`"additionalDetails":null}}`)

	_, err := conv.Steer(context.Background(), "turn-1", ports.ChatUserMessage{Text: "hurry up"})
	if !errors.Is(err, ports.ErrChatTurnNotSteerable) {
		t.Fatalf("err = %v, want ErrChatTurnNotSteerable", err)
	}
	// The kind is what makes the refusal actionable: "wait for the compaction" is
	// advice a user can follow.
	if !strings.Contains(err.Error(), "compact") {
		t.Errorf("err = %v, want it to name the turn kind", err)
	}
	if errors.Is(err, ports.ErrChatNoSteerableTurn) {
		t.Error("a running-but-unsteerable turn was reported as no turn at all")
	}
}

// A refusal Kennel does not model must stay a plain failure rather than being
// mistranslated into "nothing to steer", which would tell the user to resend
// guidance the agent may already have.
func TestSteerLeavesUnknownProviderErrorsAlone(t *testing.T) {
	conv, srv := steerConversation(t)
	refuseSteer(srv, `{"code":-32603,"message":"internal error"}`)

	_, err := conv.Steer(context.Background(), "turn-1", ports.ChatUserMessage{Text: "guidance"})
	if err == nil {
		t.Fatal("Steer succeeded against a failing provider")
	}
	if errors.Is(err, ports.ErrChatNoSteerableTurn) || errors.Is(err, ports.ErrChatTurnNotSteerable) {
		t.Fatalf("err = %v, want an untranslated failure", err)
	}
	if !strings.Contains(err.Error(), codexproto.MethodTurnSteer) {
		t.Errorf("err = %v, want it to name the failing method", err)
	}
}

// A response with no turn id still has to be attributed somewhere, and the turn the
// caller asked about is the only honest answer.
func TestSteerFallsBackToTheRequestedTurnWhenTheProviderNamesNone(t *testing.T) {
	conv, srv := steerConversation(t)
	srv.respondTo(codexproto.MethodTurnSteer, `{}`)

	ref, err := conv.Steer(context.Background(), "turn-1", ports.ChatUserMessage{Text: "guidance"})
	if err != nil {
		t.Fatalf("Steer: %v", err)
	}
	if ref.ProviderTurnID != "turn-1" {
		t.Errorf("steered turn = %q, want the requested turn", ref.ProviderTurnID)
	}
}

// TestLiveSteerKeepsTheTurnAndItsWork drives a real `codex app-server`. Skipped
// unless KENNEL_CODEX_LIVE=1: it needs a local Codex install, working auth, and it makes
// real model calls.
//
//	KENNEL_CODEX_LIVE=1 go test ./internal/adapters/chatdriver/codexappserver/ -run LiveSteer -v
//
// This earns the capability promised by turn/steer: input is appended to the active
// turn, the provider preserves that turn's identity, and the model incorporates the
// added input into later work in that turn. It intentionally does not require the
// already-running child command to stop. turn/interrupt owns cancellation.
func TestLiveSteerKeepsTheTurnAndItsWork(t *testing.T) {
	if os.Getenv("KENNEL_CODEX_LIVE") != "1" {
		t.Skip("set KENNEL_CODEX_LIVE=1 to run against a real codex app-server")
	}
	bin := liveCodexBin(t)
	workspace := newDisposableGitWorktree(t)
	d := New(livePlugin{bin: bin}, slog.New(slog.DiscardHandler))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	opened := startLiveConversation(t, d, workspace)
	defer func() { _ = opened.Close() }()
	threadID := opened.ProviderConversationID()
	if threadID == "" {
		t.Fatal("thread/start returned no provider conversation id")
	}
	steerer, ok := opened.(ports.ChatSteerer)
	if !ok {
		t.Fatalf("conversation %T does not implement ChatSteerer", opened)
	}

	turnStartedAt := time.Now()
	ref, err := opened.SendTurn(ctx, ports.ChatUserMessage{
		Text: "Run this exact shell command and then wait for further guidance before replying: " +
			"for i in 1 2 3 4 5 6 7 8 9 10; do echo tick-$i; sleep 3; done",
		ClientMessageID: "live-steer-turn", Origin: domain.MessageOriginHuman,
	})
	if err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	var starts, completions int
	var preSteerOutput strings.Builder
	for {
		select {
		case ev, live := <-opened.Events():
			if !live {
				t.Fatal("event stream closed before the turn started working")
			}
			switch ev.Kind {
			case ports.ChatEventTurnStarted:
				starts++
			case ports.ChatEventCommandOutputDelta:
				preSteerOutput.WriteString(ev.Delta)
				if starts == 1 && strings.Contains(preSteerOutput.String(), "tick-") {
					goto workObserved
				}
			case ports.ChatEventTurnCompleted:
				t.Fatal("the turn finished before there was anything to steer")
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for the turn to start working: %v", ctx.Err())
		}
	}

workObserved:
	steerRequestedAt := time.Now()
	steered, err := steerer.Steer(ctx, ref.ProviderTurnID, ports.ChatUserMessage{
		Text: "Let the current command finish. Then, in this same turn, run this exact command: " +
			"printf STEER-INCORPORATED > steer-incorporated.txt. After it succeeds, reply STEERED.",
		ClientMessageID: "live-steer-1", Origin: domain.MessageOriginHuman,
	})
	if err != nil {
		t.Fatalf("Steer: %v", err)
	}
	steerAcknowledgedAt := time.Now()
	if steered.ProviderTurnID != ref.ProviderTurnID {
		t.Errorf("steer moved the turn: %q -> %q", ref.ProviderTurnID, steered.ProviderTurnID)
	}

	var text, postSteerOutput strings.Builder
	var state domain.TurnState
collect:
	for {
		select {
		case ev, live := <-opened.Events():
			if !live {
				t.Fatal("event stream closed before the turn completed")
			}
			switch ev.Kind {
			case ports.ChatEventTurnStarted:
				starts++
			case ports.ChatEventCommandOutputDelta:
				postSteerOutput.WriteString(ev.Delta)
			case ports.ChatEventMessageCompleted:
				text.WriteString(ev.Text)
				text.WriteString("\n")
			case ports.ChatEventTurnCompleted:
				completions++
				state = ev.TurnState
				break collect
			case ports.ChatEventControllerState:
				if ev.ControllerState == ports.ChatControllerStopped {
					t.Fatalf("controller stopped before the turn completed: %v", ev.Err)
				}
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for the steered turn: %v", ctx.Err())
		}
	}

	if state != domain.TurnStateCompleted {
		t.Errorf("steered turn state = %q, want completed", state)
	}
	if starts != 1 || completions != 1 {
		t.Errorf("turn lifecycle = %d started / %d completed, want 1/1", starts, completions)
	}
	assertFileTrimmed(t, filepath.Join(workspace, "steer-incorporated.txt"), "STEER-INCORPORATED")
	if !strings.Contains(text.String(), "STEERED") {
		t.Errorf("the agent never acknowledged the guidance; final text was:\n%s", text.String())
	}

	historyReader, ok := opened.(ports.ChatHistoryReader)
	if !ok {
		t.Fatalf("conversation %T has no history reader", opened)
	}
	history, err := historyReader.ReadHistory(ctx)
	if err != nil {
		t.Fatalf("read steered history: %v", err)
	}
	var sawOriginalClientID, sawSteerClientID bool
	for _, ev := range history {
		if ev.ProviderTurnID != ref.ProviderTurnID {
			continue
		}
		switch ev.ClientMessageID {
		case "live-steer-turn":
			sawOriginalClientID = true
		case "live-steer-1":
			sawSteerClientID = true
		}
	}
	if !sawOriginalClientID {
		t.Error("provider history lost the original turn client id")
	}
	if !sawSteerClientID {
		t.Log("provider history did not preserve the steer client id on this build")
	}
	t.Logf("evidence thread=%s steer_turn=%s original_client=%s steer_client=%s same_turn=true incorporated=true steer_ack=%s turn_elapsed=%s post_steer_output=%q",
		threadID, ref.ProviderTurnID, "live-steer-turn", "live-steer-1",
		steerAcknowledgedAt.Sub(steerRequestedAt), time.Since(turnStartedAt), postSteerOutput.String())

	if _, err := steerer.Steer(ctx, ref.ProviderTurnID,
		ports.ChatUserMessage{Text: "too late"}); !errors.Is(err, ports.ErrChatNoSteerableTurn) {
		t.Errorf("steering a finished turn returned %v, want ErrChatNoSteerableTurn", err)
	}

	continued := runLiveTurn(t, ctx, opened, ports.ChatUserMessage{
		Text:            "Continue after the completed steer by running this exact local command: printf CONTINUED > steer-continued.txt. Then reply CONTINUED.",
		ClientMessageID: "live-steer-continued", Origin: domain.MessageOriginHuman,
	})
	if continued.state != domain.TurnStateCompleted || !continued.sawCompletedCommand || continued.ref.ProviderTurnID == ref.ProviderTurnID {
		t.Fatalf("post-steer continuation = %#v", continued)
	}
	assertFileTrimmed(t, filepath.Join(workspace, "steer-continued.txt"), "CONTINUED")
	t.Logf("evidence steer_turn=%s steer_client=%s continuation_turn=%s continuation_client=%s",
		ref.ProviderTurnID, "live-steer-1", continued.ref.ProviderTurnID, "live-steer-continued")
}

// The capability and the method have to agree. Advertising steer while the provider
// does not declare turn/steer would put a control in the UI that fails on use; the
// reverse leaves a working feature switched off.
func TestSteerCapabilityMatchesTheDeclaredProtocol(t *testing.T) {
	if !codexproto.Declares(codexproto.MethodTurnSteer) {
		t.Fatalf("generated protocol (%s) does not declare %s",
			codexproto.ProviderVersion, codexproto.MethodTurnSteer)
	}
	if !capabilities().Has(ports.ChatCapabilitySteer) {
		t.Error("the driver implements turn/steer but does not advertise the capability")
	}
}
