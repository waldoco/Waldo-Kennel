package devicebridge

import "encoding/json"

func validCommandClass(v any) bool {
	return oneOf(v, ClassMachineStateQuery, ClassNotifyLocal, ClassShellExec, ClassSessionQuery, ClassSessionControl)
}
func newCommandClass(v any) bool {
	return oneOf(v, ClassShellExec, ClassSessionQuery, ClassSessionControl)
}
func frameHasReference(f map[string]any) bool {
	if f["type"] == TypeHeartbeat || f["type"] == TypeSessionEvent {
		return false
	}
	p, _ := f["payload"].(map[string]any)
	return !(f["type"] == TypeReceipt && p["event_message_id"] != nil)
}
func validateMacCommand(class any, p map[string]any) error {
	if !boundedString(p["project"], 128) {
		return ErrInvalidFrameShape
	}
	if class == ClassShellExec {
		n, ok := parseInteger(p["timeout_seconds"])
		if exactFields(p, "project", "command", "reason", "timeout_seconds") && boundedString(p["command"], 4096) && boundedString(p["reason"], 512) && ok && n >= 1 && n <= 600 {
			return nil
		}
		return ErrInvalidFrameShape
	}
	if class == ClassSessionQuery {
		if p["action"] == "list_sessions" && exactFields(p, "action", "project") {
			return nil
		}
		if p["action"] == "read_session" && exactFields(p, "action", "project", "session_id") && stringID(p["session_id"]) {
			return nil
		}
		return ErrInvalidFrameShape
	}
	if !boundedString(p["reason"], 512) {
		return ErrInvalidFrameShape
	}
	switch p["action"] {
	case "start_session":
		if exactFields(p, "action", "project", "reason", "agent", "prompt") && oneOf(p["agent"], "claude-code", "codex") && boundedString(p["prompt"], 4096) {
			return nil
		}
	case "message_session":
		if exactFields(p, "action", "project", "reason", "session_id", "prompt") && stringID(p["session_id"]) && boundedString(p["prompt"], 4096) {
			return nil
		}
	case "interrupt_session", "stop_session":
		if exactFields(p, "action", "project", "reason", "session_id") && stringID(p["session_id"]) {
			return nil
		}
	}
	return ErrInvalidFrameShape
}
func macTail(p map[string]any) bool {
	s, ok := p["output_tail"].(string)
	_, b := p["truncated"].(bool)
	return ok && len(s) <= 6144 && b
}
func validMacResult(p map[string]any) bool {
	if oneOf(p["status"], ResultDenied, ResultExpired, ResultOutcomeUnknown) {
		return exactFields(p, "status")
	}
	if p["status"] == ResultCompleted {
		n, ok := p["exit_code"].(json.Number)
		i, e := n.Int64()
		fields := exactFields(p, "status", "exit_code", "output_tail", "truncated") || (exactFields(p, "status", "exit_code", "output_tail", "truncated", "session_id") && stringID(p["session_id"]))
		return fields && ok && e == nil && i >= -2147483648 && i <= 2147483647 && macTail(p)
	}
	if p["status"] != ResultAnswered || !exactFields(p, "status", "answer") {
		return false
	}
	a, ok := p["answer"].(map[string]any)
	if !ok {
		return false
	}
	if exactFields(a, "session_id", "status", "output_tail", "truncated") {
		return stringID(a["session_id"]) && boundedString(a["status"], 64) && macTail(a)
	}
	list, ok := a["sessions"].([]any)
	_, b := a["truncated"].(bool)
	if !exactFields(a, "sessions", "truncated") || !ok || !b || len(list) > 20 {
		return false
	}
	for _, v := range list {
		r, ok := v.(map[string]any)
		if !ok || !exactFields(r, "session_id", "project", "agent", "status") || !stringID(r["session_id"]) || !boundedString(r["project"], 128) || !boundedString(r["agent"], 64) || !boundedString(r["status"], 64) {
			return false
		}
	}
	return true
}

func commandTTLMax(f map[string]any) int64 {
	if newCommandClass(f["class"]) {
		return ApprovalTTLSeconds
	}
	return CommandTTLMaxSeconds
}
