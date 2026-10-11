package devicebridge

import "context"

// QueryEvidenceSource supplies only durable state evidence. Selection of a
// daemon session/Attempt/worktree for a query is deliberately left to a future
// reviewed adapter: v0.2.3 carries no target selector or aggregate rule.
type QueryEvidenceSource interface {
	QueryState(context.Context, Scope, string, string) (state string, found bool, err error)
}

// DisplaySurface is an injected, display-only boundary. The runtime injects
// bridgeruntime.DisplayRelay, which reports delivered only on the desktop shell's
// acknowledgement that the OS showed the text; it never creates an owner grant.
type DisplaySurface interface {
	Display(context.Context, Scope, Notification) (delivered bool, err error)
}
type Notification struct{ ID, Title, Body, Severity string }

type Handlers struct {
	Queries QueryEvidenceSource
	Display DisplaySurface
}

func (h Handlers) Handle(ctx context.Context, scope Scope, command map[string]any) (map[string]any, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if !scope.matches(command) || validateFrameShape(command, false) != nil || command["type"] != TypeCommand {
		return nil, ErrInvalidFrameShape
	}
	if command["class"] == ClassMachineStateQuery {
		return h.query(ctx, scope, command)
	}
	p := command["payload"].(map[string]any)
	if h.Display == nil {
		return ResultPayloadFailure(ReasonDeliveryUnknown), nil
	}
	delivered, err := h.Display.Display(ctx, scope, Notification{p["notification_id"].(string), p["title"].(string), p["body"].(string), p["severity"].(string)})
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil || !delivered {
		return ResultPayloadFailure(ReasonDeliveryUnknown), nil
	}
	return map[string]any{"status": ResultDelivered}, nil
}
func (h Handlers) Reconcile(ctx context.Context, scope Scope, command map[string]any) (map[string]any, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if !scope.matches(command) || validateFrameShape(command, false) != nil || command["type"] != TypeCommand {
		return nil, ErrInvalidFrameShape
	}
	if command["class"] == ClassMachineStateQuery {
		return h.query(ctx, scope, command)
	}
	// Acceptance does not prove display. With no separately durable delivery
	// proof, never invoke Display again or silently mark the notification sent.
	return ResultPayloadFailure(ReasonDeliveryUnknown), nil
}
func (h Handlers) query(ctx context.Context, scope Scope, command map[string]any) (map[string]any, error) {
	p := command["payload"].(map[string]any)
	id := p["query_id"].(string)
	kind := p["query_kind"].(string)
	state := StateUnknown
	if h.Queries != nil {
		value, found, err := h.Queries.QueryState(ctx, scope, id, kind)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			return ResultPayloadFailure(ReasonProcessingFailed), nil
		}
		if found {
			if !validQueryState(kind, value) {
				return ResultPayloadFailure(ReasonProcessingFailed), nil
			}
			state = value
		}
	}
	return map[string]any{"status": ResultAnswered, "answer": map[string]any{"query_id": id, "query_kind": kind, "state": state}}, nil
}
