package devicebridge

import (
	"context"
	"fmt"
)

type SessionEvent struct {
	Sequence                  int64
	SessionID, Project, Event string
}
type SessionEventStore interface {
	UnspooledSessionEvents(context.Context, Scope) ([]SessionEvent, error)
	CommitSessionEvent(context.Context, Scope, SessionEvent, []byte) error
}

func (c *Client) spoolSessionEvents(ctx context.Context) error {
	store, ok := c.Store.(SessionEventStore)
	if !ok || (!c.declares(ClassSessionQuery) && !c.declares(ClassSessionControl)) {
		return nil
	}
	events, err := store.UnspooledSessionEvents(ctx, c.Scope)
	if err != nil {
		return err
	}
	for _, event := range events {
		id, err := NewMessageID(c.clock(), nil)
		if err != nil {
			return err
		}
		f := map[string]any{"contract_version": ContractVersion, "type": TypeSessionEvent, "message_id": id, "device_id": c.Scope.DeviceID, "owner_id": c.Scope.OwnerID, "payload": map[string]any{"session_id": event.SessionID, "project": event.Project, "event": event.Event}}
		raw, err := marshalCanonical(f)
		if err != nil {
			return err
		}
		nonce, err := NewNonce(nil)
		if err != nil {
			return err
		}
		wire, err := SignFrame(raw, c.Key, c.clock(), nonce)
		if err != nil {
			return err
		}
		if err = store.CommitSessionEvent(ctx, c.Scope, event, wire); err != nil {
			return err
		}
	}
	return nil
}
func ValidateReceiptForEvent(receipt, event map[string]any) error {
	if validateFrameShape(receipt, false) != nil || validateFrameShape(event, true) != nil || receipt["type"] != TypeReceipt || event["type"] != TypeSessionEvent {
		return ErrInvalidFrameShape
	}
	for _, field := range []string{"contract_version", "device_id", "owner_id"} {
		if receipt[field] != event[field] {
			return ErrInvalidFrameShape
		}
	}
	p := receipt["payload"].(map[string]any)
	if p["event_message_id"] != event["message_id"] {
		return ErrInvalidFrameShape
	}
	return nil
}

// Synthetic storage references never become envelope fields.
func SessionEventStorageID(sequence int64) string { return fmt.Sprintf("event_%d", sequence) }
