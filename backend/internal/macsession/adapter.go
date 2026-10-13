// Package macsession adapts bridge requests to the existing Kennel services.
// It never supplies tool-permission answers or creates a second session engine.
package macsession

import (
	"context"
	"errors"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/macaction"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	chatsvc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/chat"
	sessionsvc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/session"
	"path/filepath"
	"sort"
	"strings"
)

var ErrBlocked = errors.New("session blocked on a permission or decision; owner input required")

type ProjectStore interface {
	ListProjects(context.Context) ([]domain.ProjectRecord, error)
}
type Registry struct{ Store ProjectStore }

func (r Registry) Resolve(ctx context.Context, name string) (id, folder string, err error) {
	rows, err := r.Store.ListProjects(ctx)
	if err != nil {
		return "", "", err
	}
	var found *domain.ProjectRecord
	for _, p := range rows {
		if p.ArchivedAt.IsZero() && (p.ID == name || p.DisplayName == name) {
			if found != nil && found.ID != p.ID {
				return "", "", macaction.ErrInvalidProject
			}
			q := p
			found = &q
		}
	}
	if found == nil || !filepath.IsAbs(found.Path) {
		return "", "", macaction.ErrInvalidProject
	}
	folder, err = filepath.EvalSymlinks(found.Path)
	if err != nil {
		return "", "", err
	}
	return found.ID, folder, nil
}

type Runtime interface {
	GetOutput(context.Context, ports.RuntimeHandle, int) (string, error)
	Interrupt(context.Context, ports.RuntimeHandle) error
}
type SessionService interface {
	Get(context.Context, domain.SessionID) (domain.Session, error)
	List(context.Context, sessionsvc.ListFilter) ([]domain.Session, error)
	Spawn(context.Context, ports.SpawnConfig) (domain.Session, int, int, error)
	Send(context.Context, domain.SessionID, string, *ports.SpawnAttachment) error
	Kill(context.Context, domain.SessionID) (bool, error)
}
type ChatService interface {
	SnapshotPage(context.Context, domain.SessionID, int64, int64) (chatsvc.Snapshot, error)
	Snapshot(context.Context, domain.SessionID) (chatsvc.Snapshot, error)
	Send(context.Context, domain.SessionID, ports.ChatUserMessage) (domain.ConversationTurn, error)
	Interrupt(context.Context, domain.SessionID) error
}
type Adapter struct {
	Sessions SessionService
	Chat     ChatService
	Runtime  Runtime
}

func value(p map[string]any, k string) string { s, _ := p[k].(string); return s }
func (a Adapter) bound(ctx context.Context, project, id string) (domain.Session, error) {
	s, err := a.Sessions.Get(ctx, domain.SessionID(id))
	if err != nil {
		return s, err
	}
	if string(s.ProjectID) != project || (s.Harness != domain.HarnessCodex && s.Harness != domain.HarnessClaudeCode) {
		return s, errors.New("session not in registered project")
	}
	return s, nil
}
func (a Adapter) Query(ctx context.Context, project string, p map[string]any) (map[string]any, error) {
	if value(p, "action") == "list_sessions" {
		list, err := a.Sessions.List(ctx, sessionsvc.ListFilter{ProjectID: domain.ProjectID(project)})
		if err != nil {
			return nil, err
		}
		sort.Slice(list, func(i, j int) bool { return list[i].UpdatedAt.After(list[j].UpdatedAt) })
		rows := []any{}
		total := 0
		for _, s := range list {
			if s.Harness != domain.HarnessCodex && s.Harness != domain.HarnessClaudeCode {
				continue
			}
			total++
			if len(rows) < 20 {
				rows = append(rows, map[string]any{"session_id": string(s.ID), "project": project, "agent": string(s.Harness), "status": status(s)})
			}
		}
		return map[string]any{"status": "answered", "answer": map[string]any{"sessions": rows, "truncated": total > 20}}, nil
	}
	s, err := a.bound(ctx, project, value(p, "session_id"))
	if err != nil {
		return nil, err
	}
	var text string
	var omitted bool
	if domain.NormalizeSessionMode(s.Mode) == domain.SessionModeChat {
		snapshot, err := a.Chat.SnapshotPage(ctx, s.ID, 0, 20)
		if err != nil {
			return nil, err
		}
		var tail strings.Builder
		for _, m := range snapshot.Messages {
			tail.WriteString(m.Text)
			tail.WriteByte('\n')
		}
		text = tail.String()
		omitted = snapshot.HasMoreBefore
	} else {
		if a.Runtime == nil {
			return nil, errors.New("terminal unavailable")
		}
		text, err = a.Runtime.GetOutput(ctx, ports.RuntimeHandle{ID: s.Metadata.RuntimeHandleID}, 200)
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{"status": "answered", "answer": map[string]any{"session_id": string(s.ID), "status": status(s), "output_tail": macaction.Tail([]byte(text)), "truncated": omitted || len(text) > macaction.TailLimit}}, nil
}
func status(s domain.Session) string {
	if s.Activity.State == domain.ActivityBlocked {
		return "blocked_on_tool"
	}
	return string(s.Status)
}
func (a Adapter) Control(ctx context.Context, project string, p map[string]any) (string, error) {
	action := value(p, "action")
	if action == "start_session" {
		// Spawn chooses and starts the established Chat/TUI controller, including
		// StartChat when selected by the daemon. No parallel provider launch path.
		s, _, _, err := a.Sessions.Spawn(ctx, ports.SpawnConfig{ProjectID: domain.ProjectID(project), Kind: domain.KindWorker, Harness: domain.AgentHarness(value(p, "agent")), Prompt: value(p, "prompt")})
		return string(s.ID), err
	}
	s, err := a.bound(ctx, project, value(p, "session_id"))
	if err != nil {
		return "", err
	}
	chat := domain.NormalizeSessionMode(s.Mode) == domain.SessionModeChat
	switch action {
	case "message_session":
		if s.Activity.State == domain.ActivityBlocked {
			return "", ErrBlocked
		}
		if chat {
			snapshot, readErr := a.Chat.Snapshot(ctx, s.ID)
			if readErr != nil {
				return "", readErr
			}
			for _, activity := range snapshot.Activities {
				if activity.Kind == domain.ActivityKindApproval && activity.Status == domain.ActivityStatusPending {
					return "", ErrBlocked
				}
			}
			_, err = a.Chat.Send(ctx, s.ID, ports.ChatUserMessage{Text: value(p, "prompt"), Origin: domain.MessageOriginAutomation})
		} else {
			err = a.Sessions.Send(ctx, s.ID, value(p, "prompt"), nil)
		}
	case "interrupt_session":
		if chat {
			err = a.Chat.Interrupt(ctx, s.ID)
		} else {
			if a.Runtime == nil {
				return "", errors.New("terminal unavailable")
			}
			err = a.Runtime.Interrupt(ctx, ports.RuntimeHandle{ID: s.Metadata.RuntimeHandleID})
		}
	case "stop_session":
		// Session Kill dispatches its existing Chat Stop path and lifecycle cleanup.
		_, err = a.Sessions.Kill(ctx, s.ID)
	default:
		return "", errors.New("unknown control")
	}
	return string(s.ID), err
}
