package macsession

import (
	"context"
	"errors"
	"fmt"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/macaction"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	chatsvc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/chat"
	sessionsvc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/session"
	"strings"
	"testing"
)

type sessionsFake struct {
	row    domain.Session
	rows   []domain.Session
	calls  []string
	config ports.SpawnConfig
}

func (f *sessionsFake) Get(context.Context, domain.SessionID) (domain.Session, error) {
	return f.row, nil
}
func (f *sessionsFake) List(context.Context, sessionsvc.ListFilter) ([]domain.Session, error) {
	return f.rows, nil
}
func (f *sessionsFake) Spawn(_ context.Context, c ports.SpawnConfig) (domain.Session, int, int, error) {
	f.config = c
	f.calls = append(f.calls, "spawn")
	return f.row, 0, 0, nil
}
func (f *sessionsFake) Send(context.Context, domain.SessionID, string, *ports.SpawnAttachment) error {
	f.calls = append(f.calls, "send")
	return nil
}
func (f *sessionsFake) Kill(context.Context, domain.SessionID) (bool, error) {
	f.calls = append(f.calls, "kill")
	return true, nil
}

type chatFake struct {
	snapshot chatsvc.Snapshot
	calls    []string
	origin   domain.MessageOrigin
}

func (f *chatFake) Snapshot(context.Context, domain.SessionID) (chatsvc.Snapshot, error) {
	return f.snapshot, nil
}
func (f *chatFake) SnapshotPage(context.Context, domain.SessionID, int64, int64) (chatsvc.Snapshot, error) {
	return f.snapshot, nil
}
func (f *chatFake) Send(_ context.Context, _ domain.SessionID, m ports.ChatUserMessage) (domain.ConversationTurn, error) {
	f.calls = append(f.calls, "send")
	f.origin = m.Origin
	return domain.ConversationTurn{}, nil
}
func (f *chatFake) Interrupt(context.Context, domain.SessionID) error {
	f.calls = append(f.calls, "interrupt")
	return nil
}
func sessionRow() domain.Session {
	return domain.Session{SessionRecord: domain.SessionRecord{ID: "session", ProjectID: "p", Harness: domain.HarnessCodex, Mode: domain.SessionModeChat}}
}
func TestAdapterBoundedReadAndProjectBinding(t *testing.T) {
	ctx := context.Background()
	sessions := &sessionsFake{row: sessionRow()}
	chat := &chatFake{snapshot: chatsvc.Snapshot{Messages: []domain.ConversationMessage{{Text: strings.Repeat("x", 9000)}}, HasMoreBefore: true}}
	a := Adapter{Sessions: sessions, Chat: chat}
	p := map[string]any{"action": "read_session", "session_id": "session"}
	if _, e := a.Query(ctx, "foreign", p); e == nil {
		t.Fatal("cross-project read")
	}
	result, e := a.Query(ctx, "p", p)
	if e != nil {
		t.Fatal(e)
	}
	answer := result["answer"].(map[string]any)
	if len(answer["output_tail"].(string)) > 6144 || answer["truncated"] != true {
		t.Fatal(answer)
	}
	chat.snapshot.Messages = []domain.ConversationMessage{{Text: "small"}}
	result, e = a.Query(ctx, "p", p)
	if e != nil || result["answer"].(map[string]any)["truncated"] != true {
		t.Fatal(result, e)
	}
	for i := 0; i < 22; i++ {
		row := sessionRow()
		row.ID = domain.SessionID(fmt.Sprintf("s%d", i))
		sessions.rows = append(sessions.rows, row)
	}
	result, e = a.Query(ctx, "p", map[string]any{"action": "list_sessions"})
	if e != nil {
		t.Fatal(e)
	}
	answer = result["answer"].(map[string]any)
	if len(answer["sessions"].([]any)) != 20 || answer["truncated"] != true {
		t.Fatal(answer)
	}
}
func TestAdapterReusesChatAndRefusesPermissionAnswers(t *testing.T) {
	ctx := context.Background()
	sessions := &sessionsFake{row: sessionRow()}
	chat := &chatFake{}
	a := Adapter{Sessions: sessions, Chat: chat}
	p := map[string]any{"action": "message_session", "session_id": "session", "prompt": "continue"}
	sessions.row.Activity.State = domain.ActivityBlocked
	if _, e := a.Control(ctx, "p", p); !errors.Is(e, ErrBlocked) {
		t.Fatal(e)
	}
	sessions.row.Activity.State = domain.ActivityIdle
	chat.snapshot.Activities = []domain.ConversationActivity{{Kind: domain.ActivityKindApproval, Status: domain.ActivityStatusPending}}
	if _, e := a.Control(ctx, "p", p); !errors.Is(e, ErrBlocked) {
		t.Fatal(e)
	}
	if len(chat.calls) > 0 || len(sessions.calls) > 0 {
		t.Fatal("permission answer attempted")
	}
	chat.snapshot.Activities = nil
	if _, e := a.Control(ctx, "p", p); e != nil || chat.origin != domain.MessageOriginAutomation {
		t.Fatal(e, chat.origin)
	}
	p["action"] = "interrupt_session"
	if _, e := a.Control(ctx, "p", p); e != nil {
		t.Fatal(e)
	}
	p["action"] = "stop_session"
	if _, e := a.Control(ctx, "p", p); e != nil {
		t.Fatal(e)
	}
	if strings.Join(chat.calls, ",") != "send,interrupt" || strings.Join(sessions.calls, ",") != "kill" {
		t.Fatal(chat.calls, sessions.calls)
	}
	p = map[string]any{"action": "start_session", "agent": "claude-code", "prompt": "tests"}
	if _, e := a.Control(ctx, "p", p); e != nil || sessions.config.Harness != domain.HarnessClaudeCode || sessions.config.ProjectID != "p" {
		t.Fatal(e, sessions.config)
	}
}

type projectsFake []domain.ProjectRecord

func (p projectsFake) ListProjects(context.Context) ([]domain.ProjectRecord, error) { return p, nil }
func TestRegistryRejectsUnregisteredAndAmbiguousFolders(t *testing.T) {
	ctx := context.Background()
	folder := t.TempDir()
	r := Registry{Store: projectsFake{{ID: "p", DisplayName: "Project", Path: folder}}}
	if _, _, e := r.Resolve(ctx, "/unregistered"); !errors.Is(e, macaction.ErrInvalidProject) {
		t.Fatal(e)
	}
	if id, path, e := r.Resolve(ctx, "Project"); e != nil || id != "p" || path == "" {
		t.Fatal(id, path, e)
	}
	r.Store = projectsFake{{ID: "p", DisplayName: "same", Path: folder}, {ID: "q", DisplayName: "same", Path: folder}}
	if _, _, e := r.Resolve(ctx, "same"); !errors.Is(e, macaction.ErrInvalidProject) {
		t.Fatal(e)
	}
}
