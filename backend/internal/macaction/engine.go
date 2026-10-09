package macaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/devicebridge"
)

// Request is local approval custody. The token and decision never cross the
// bridge. Fingerprint binds the exact request, registered folder and device.
type Request struct {
	ID          string         `json:"id"`
	DeviceID    string         `json:"device_id"`
	OwnerID     string         `json:"-"`
	Fingerprint string         `json:"fingerprint"`
	Kind        string         `json:"kind"`
	Class       string         `json:"class"`
	Project     string         `json:"project"`
	Folder      string         `json:"folder"`
	SessionID   string         `json:"session_id,omitempty"`
	Command     string         `json:"command,omitempty"`
	Prompt      string         `json:"prompt,omitempty"`
	Reason      string         `json:"reason"`
	Action      string         `json:"action,omitempty"`
	Risks       []string       `json:"risks"`
	ExpiresAt   int64          `json:"expires_at"`
	State       string         `json:"state"`
	Result      map[string]any `json:"-"`
}
type Ledger interface {
	Pending(context.Context, Request) (Request, error)
	Load(context.Context, string, string) (Request, error)
	Start(context.Context, Request, int64) (bool, error)
	Finish(context.Context, Request, map[string]any, []byte) error
}
type Projects interface {
	Resolve(context.Context, string) (id, folder string, err error)
}
type Sessions interface {
	Query(context.Context, string, map[string]any) (map[string]any, error)
	Control(context.Context, string, map[string]any) (string, error)
}
type CommandRunner interface {
	Run(context.Context, string, string, int64) (Execution, error)
}

type Engine struct {
	Ledger   Ledger
	Projects Projects
	Sessions Sessions
	Runner   CommandRunner
	K0       devicebridge.Executor
	Now      func() time.Time
	Poll     time.Duration
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}
func outcome(status string) map[string]any { return map[string]any{"status": status} }
func (e *Engine) Handle(ctx context.Context, scope devicebridge.Scope, f map[string]any) (map[string]any, error) {
	return e.run(ctx, scope, f, false)
}
func (e *Engine) Reconcile(ctx context.Context, scope devicebridge.Scope, f map[string]any) (map[string]any, error) {
	return e.run(ctx, scope, f, true)
}
func (e *Engine) run(ctx context.Context, scope devicebridge.Scope, f map[string]any, recovery bool) (map[string]any, error) {
	class, _ := f["class"].(string)
	if class == devicebridge.ClassMachineStateQuery || class == devicebridge.ClassNotifyLocal {
		if e.K0 == nil {
			return devicebridge.ResultPayloadFailure(devicebridge.ReasonProcessingFailed), nil
		}
		if recovery {
			return e.K0.Reconcile(ctx, scope, f)
		}
		return e.K0.Handle(ctx, scope, f)
	}
	p, _ := f["payload"].(map[string]any)
	id, _ := f["command_id"].(string)
	failed := devicebridge.ResultPayloadFailure(devicebridge.ReasonProcessingFailed)
	if e.Ledger == nil || e.Projects == nil {
		return failed, nil
	}
	// Reconcile a started marker BEFORE inspecting the approval deadline.
	// Restart does not turn unknown execution into expiry or run it again.
	if recovery {
		old, err := e.Ledger.Load(ctx, scope.DeviceID, id)
		if err == nil {
			if old.Result != nil {
				return old.Result, nil
			}
			if old.State == "started" {
				r := outcome(devicebridge.ResultOutcomeUnknown)
				if err = e.Ledger.Finish(ctx, old, r, nil); err != nil {
					return nil, err
				}
				return r, nil
			}
		}
	}
	project, folder, err := e.Projects.Resolve(ctx, stringValue(p["project"]))
	if err != nil {
		return failed, nil
	}
	expNumber, ok := f["expires_at"].(json.Number)
	if !ok {
		return failed, nil
	}
	expires, err := expNumber.Int64()
	if err != nil {
		return failed, nil
	}
	if expires <= e.now().Unix() {
		return outcome(devicebridge.ResultExpired), nil
	}
	if class == devicebridge.ClassSessionQuery {
		if e.Sessions == nil {
			return failed, nil
		}
		return e.Sessions.Query(ctx, project, p)
	}
	action := stringValue(p["action"])
	request := Request{ID: id, DeviceID: scope.DeviceID, OwnerID: scope.OwnerID, Kind: "approval", Class: class, Project: project, Folder: folder, SessionID: stringValue(p["session_id"]), Command: stringValue(p["command"]), Prompt: stringValue(p["prompt"]), Reason: stringValue(p["reason"]), Action: action, ExpiresAt: expires, State: "pending"}
	request.Risks = Risks(request.Command+request.Prompt, folder)
	// Backend redelivery may change message_id; fingerprint the stable request.
	deleteStable := make(map[string]any)
	for k, v := range f {
		if k != "message_id" {
			deleteStable[k] = v
		}
	}
	exact, _ := json.Marshal(struct {
		Request Request
		Frame   map[string]any
	}{request, deleteStable})
	digest := sha256.Sum256(exact)
	request.Fingerprint = hex.EncodeToString(digest[:])
	request, err = e.Ledger.Pending(ctx, request)
	if err != nil {
		return nil, err
	}
	if class == devicebridge.ClassSessionControl && action == "interrupt_session" {
		if request.Result != nil {
			return request.Result, nil
		}
		started, err := e.Ledger.Start(ctx, request, e.now().Unix())
		if err != nil {
			return nil, err
		}
		if !started {
			return outcome(devicebridge.ResultExpired), nil
		}
		if e.Sessions == nil {
			return failed, e.Ledger.Finish(ctx, request, failed, nil)
		}
		sid, err := e.Sessions.Control(ctx, project, p)
		result := completed(0, "", false, sid)
		if err != nil {
			result = failed
		}
		return result, e.Ledger.Finish(ctx, request, result, nil)
	}
	poll := e.Poll
	if poll <= 0 {
		poll = 250 * time.Millisecond
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		if request.Result != nil {
			return request.Result, nil
		}
		if request.State == "started" {
			r := outcome(devicebridge.ResultOutcomeUnknown)
			return r, e.Ledger.Finish(ctx, request, r, nil)
		}
		if request.State == "denied" {
			r := outcome(devicebridge.ResultDenied)
			return r, e.Ledger.Finish(ctx, request, r, nil)
		}
		if e.now().Unix() >= request.ExpiresAt {
			r := outcome(devicebridge.ResultExpired)
			return r, e.Ledger.Finish(ctx, request, r, nil)
		}
		if request.State == "approved" {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			request, err = e.Ledger.Load(ctx, scope.DeviceID, id)
			if err != nil {
				return nil, err
			}
		}
	}
	// The registry can change while the dialog is open. Its exact folder remains
	// part of the approval; fail closed if it no longer resolves identically.
	current, newFolder, err := e.Projects.Resolve(ctx, project)
	if err != nil || current != project || newFolder != folder {
		return failed, e.Ledger.Finish(ctx, request, failed, nil)
	}
	started, err := e.Ledger.Start(ctx, request, e.now().Unix())
	if err != nil {
		return nil, err
	}
	if !started {
		r := outcome(devicebridge.ResultExpired)
		return r, e.Ledger.Finish(ctx, request, r, nil)
	}
	var result map[string]any
	var local []byte
	if class == devicebridge.ClassShellExec {
		n, ok := p["timeout_seconds"].(json.Number)
		if !ok {
			return nil, errors.New("invalid execution timeout")
		}
		seconds, err := n.Int64()
		if err != nil {
			return nil, err
		}
		run, err := e.Runner.Run(ctx, folder, request.Command, seconds)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		} // durable started evidence handles reconnect.
		if err != nil {
			result = failed
		} else {
			result = completed(run.ExitCode, run.OutputTail, run.Truncated, "")
		}
		local = run.LocalOutput
	} else {
		if e.Sessions == nil {
			result = failed
		} else {
			sid, err := e.Sessions.Control(ctx, project, p)
			if err != nil {
				result = failed
			} else {
				result = completed(0, "", false, sid)
			}
		}
	}
	return result, e.Ledger.Finish(ctx, request, result, local)
}
func stringValue(v any) string { s, _ := v.(string); return s }
func completed(code int, tail string, truncated bool, session string) map[string]any {
	r := map[string]any{"status": devicebridge.ResultCompleted, "exit_code": code, "output_tail": tail, "truncated": truncated}
	if session != "" {
		r["session_id"] = session
	}
	return r
}
