package daemon

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/agent/codex"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
)

// fakePaneTerminalRuntime implements paneTerminalRuntime over scripted pane
// captures, and records whether the SendMessage fallback fired.
type fakePaneTerminalRuntime struct {
	captures []string
	ops      []string
	sent     string
}

func (f *fakePaneTerminalRuntime) SendMessage(_ context.Context, _ ports.RuntimeHandle, message string) error {
	f.sent = message
	return nil
}
func (f *fakePaneTerminalRuntime) GetOutput(_ context.Context, _ ports.RuntimeHandle, _ int) (string, error) {
	if len(f.captures) == 0 {
		return "", errors.New("no capture scripted")
	}
	out := f.captures[0]
	if len(f.captures) > 1 {
		f.captures = f.captures[1:]
	}
	return out, nil
}
func (f *fakePaneTerminalRuntime) PasteBuffer(_ context.Context, _ ports.RuntimeHandle, text string) error {
	f.ops = append(f.ops, "paste:"+text)
	return nil
}
func (f *fakePaneTerminalRuntime) SendEnter(_ context.Context, _ ports.RuntimeHandle) error {
	f.ops = append(f.ops, "enter")
	return nil
}
func (f *fakePaneTerminalRuntime) SendTab(_ context.Context, _ ports.RuntimeHandle) error {
	f.ops = append(f.ops, "tab")
	return nil
}
func (f *fakePaneTerminalRuntime) CancelCopyMode(_ context.Context, _ ports.RuntimeHandle) {
	f.ops = append(f.ops, "cancel")
}
func (f *fakePaneTerminalRuntime) PaneDead(_ context.Context, _ ports.RuntimeHandle) (bool, error) {
	f.ops = append(f.ops, "probe")
	return false, nil
}

const wiringTestMsg = "Run this shell command: echo hello"

var (
	wiringPaneIdleNoDraft = "› earlier prompt\n\n  done 9:04 AM\n\n› Ask Codex to do anything\n\n  gpt-6-astra default · /tmp/worktrees/mer-1 · title"
	wiringPaneDraft       = strings.Replace(wiringPaneIdleNoDraft, "› Ask Codex to do anything", "› "+wiringTestMsg, 1)
	wiringPaneSubmitted   = "› earlier prompt\n\n  done 9:04 AM\n\n› " + wiringTestMsg + "\n\n  Working (3s, esc to interrupt)\n\n› Ask Codex to do anything\n\n  gpt-6-astra default · /tmp/worktrees/mer-1 · title"
)

func newCodexTUISession(t *testing.T, store *sqlite.Store, harness domain.AgentHarness, mode domain.SessionMode) domain.SessionID {
	t.Helper()
	ctx := context.Background()
	if err := store.UpsertProject(ctx, domain.ProjectRecord{ID: "p", Path: "/repo/p", RegisteredAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	rec, err := store.CreateSession(ctx, domain.SessionRecord{
		ProjectID: "p", Kind: domain.KindWorker, Harness: harness, Mode: mode,
		Activity: domain.Activity{State: domain.ActivityIdle, LastActivityAt: time.Now()},
		Metadata: domain.SessionMetadata{RuntimeHandleID: "kennel-1/terminal_0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return rec.ID
}

// TestWiring_CodexTUISendUsesStateAwareDelivery asserts a codex TUI session
// on a pane-capable runtime goes through the orchestrated path (liveness
// probes, copy-mode cancel, atomic paste, Enter) and that SendMessage never
// fires. The later probes are the mandatory post-wait liveness re-probes
// (pre-dispatch and pre-submit, rounds 5.2/5.3).
func TestWiring_CodexTUISendUsesStateAwareDelivery(t *testing.T) {
	store, err := sqlitetest.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	rt := &fakePaneTerminalRuntime{captures: []string{
		wiringPaneIdleNoDraft, // classify: idle
		wiringPaneIdleNoDraft, // pre-dispatch boundary snapshot (no echo yet)
		wiringPaneDraft,       // settle: draft visible
		wiringPaneDraft,       // pre-submit re-classify
		wiringPaneSubmitted,   // ack: new echo + turn running, draft gone
	}}
	messenger := newSessionMessenger(store, rt, slog.New(slog.NewTextHandler(io.Discard, nil)))
	id := newCodexTUISession(t, store, domain.HarnessCodex, domain.SessionModeTUI)

	if err := messenger.Send(context.Background(), id, wiringTestMsg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if rt.sent != "" {
		t.Fatalf("fire-and-forget SendMessage fired for a codex TUI session: %q", rt.sent)
	}
	want := []string{"probe", "probe", "cancel", "paste:" + wiringTestMsg, "probe", "enter"}
	if strings.Join(rt.ops, ",") != strings.Join(want, ",") {
		t.Fatalf("ops = %v, want %v", rt.ops, want)
	}
}

// TestWiring_CodexTUISendSurfacesDeliveryUnknown asserts the stuck-draft
// failure (run 35303652394) now returns an honest error instead of 200.
func TestWiring_CodexTUISendSurfacesDeliveryUnknown(t *testing.T) {
	store, err := sqlitetest.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	rt := &fakePaneTerminalRuntime{captures: []string{
		wiringPaneIdleNoDraft, // classify: idle
		wiringPaneIdleNoDraft, // pre-dispatch boundary snapshot
		wiringPaneDraft,       // settle + pre-submit + ack polls + recovery probe: draft stuck
	}}
	messenger := newSessionMessenger(store, rt, slog.New(slog.NewTextHandler(io.Discard, nil)))
	id := newCodexTUISession(t, store, domain.HarnessCodex, domain.SessionModeTUI)

	err = messenger.Send(context.Background(), id, wiringTestMsg)
	var unk *codex.DeliveryUnknownError
	if !errors.As(err, &unk) {
		t.Fatalf("Send err = %v, want DeliveryUnknownError", err)
	}
	if rt.sent != "" {
		t.Fatal("SendMessage fallback must not fire on the hardened path")
	}
}

// TestWiring_NonCodexSendKeepsFireAndForget: other harnesses keep the
// pre-hardening contract until their delivery policies exist.
func TestWiring_NonCodexSendKeepsFireAndForget(t *testing.T) {
	store, err := sqlitetest.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	rt := &fakePaneTerminalRuntime{}
	messenger := newSessionMessenger(store, rt, slog.New(slog.NewTextHandler(io.Discard, nil)))
	id := newCodexTUISession(t, store, domain.HarnessClaudeCode, domain.SessionModeTUI)

	if err := messenger.Send(context.Background(), id, "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if rt.sent != "hello" {
		t.Fatalf("SendMessage fallback did not fire, sent=%q ops=%v", rt.sent, rt.ops)
	}
	if len(rt.ops) != 0 {
		t.Fatalf("pane-terminal ops fired for a non-codex session: %v", rt.ops)
	}
}
