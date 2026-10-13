package bridgepersist

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/devicebridge"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"time"
)

func (b *BridgeSession) UnspooledSessionEvents(ctx context.Context, s devicebridge.Scope) (out []devicebridge.SessionEvent, err error) {
	err = b.tx(ctx, s, true, func(tx *sql.Tx) error {
		rows, e := tx.QueryContext(ctx, `SELECT seq,session_id,project_id,event FROM device_bridge_session_events WHERE device_id=? AND spooled=0 ORDER BY seq LIMIT 100`, s.DeviceID)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var event devicebridge.SessionEvent
			if e = rows.Scan(&event.Sequence, &event.SessionID, &event.Project, &event.Event); e != nil {
				return e
			}
			out = append(out, event)
		}
		return rows.Err()
	})
	return
}
func (b *BridgeSession) CommitSessionEvent(ctx context.Context, s devicebridge.Scope, event devicebridge.SessionEvent, wire []byte) error {
	f, e := bridgeJSON(wire)
	if e != nil {
		return e
	}
	if !bridgeBound(f, s) || f["type"] != devicebridge.TypeSessionEvent {
		return domain.ErrDeviceBridgeInvalid
	}
	p := f["payload"].(map[string]any)
	if p["session_id"] != event.SessionID || p["project"] != event.Project || p["event"] != event.Event {
		return domain.ErrDeviceBridgeConflict
	}
	return b.tx(ctx, s, true, func(tx *sql.Tx) error {
		var spooled bool
		var session, project, status string
		if e := tx.QueryRowContext(ctx, `SELECT spooled,session_id,project_id,event FROM device_bridge_session_events WHERE device_id=? AND seq=?`, s.DeviceID, event.Sequence).Scan(&spooled, &session, &project, &status); e != nil {
			return e
		}
		if session != event.SessionID || project != event.Project || status != event.Event {
			return domain.ErrDeviceBridgeConflict
		}
		if spooled {
			return nil
		}
		fp, e := bridgeFP(f)
		if e != nil {
			return e
		}
		if e = bridgeDedup(ctx, tx, s, f); e != nil {
			return e
		}
		payload, _ := json.Marshal(p)
		id := devicebridge.SessionEventStorageID(event.Sequence)
		if _, e = tx.ExecContext(ctx, `INSERT INTO device_bridge_outbox(device_id,message_id,command_id,revision,class,payload,state,created_at,frame,idempotency_key,fingerprint) VALUES(?,?,?,1,'session_event',?,'pending',?,?,?,?)`, s.DeviceID, bridgeID(f, "message_id"), id, string(payload), time.Now().UTC(), string(wire), id, fp); e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `UPDATE device_bridge_session_events SET spooled=1 WHERE device_id=? AND seq=?`, s.DeviceID, event.Sequence)
		return e
	})
}
func (b *BridgeSession) applyEventReceipt(ctx context.Context, s devicebridge.Scope, r map[string]any, now time.Time) error {
	id := r["payload"].(map[string]any)["event_message_id"].(string)
	return b.tx(ctx, s, true, func(tx *sql.Tx) error {
		var frame, fp, command, key string
		e := tx.QueryRowContext(ctx, `SELECT frame,fingerprint,command_id,idempotency_key FROM device_bridge_outbox WHERE device_id=? AND message_id=? AND class='session_event'`, s.DeviceID, id).Scan(&frame, &fp, &command, &key)
		if errors.Is(e, sql.ErrNoRows) {
			var old string
			e = tx.QueryRowContext(ctx, `SELECT result_fingerprint FROM device_bridge_receipts WHERE device_id=? AND result_message_id=? AND command_id LIKE 'event_%'`, s.DeviceID, id).Scan(&old)
			if errors.Is(e, sql.ErrNoRows) {
				return devicebridge.ErrUnknownReceiptMessage
			}
			if e != nil {
				return e
			}
			return bridgeDedup(ctx, tx, s, r)
		}
		if e != nil {
			return e
		}
		event, e := bridgeJSON([]byte(frame))
		if e != nil {
			return e
		}
		if e = devicebridge.ValidateReceiptForEvent(r, event); e != nil {
			return e
		}
		if e = bridgeDedup(ctx, tx, s, r); e != nil {
			return e
		}
		var count int
		if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM device_bridge_receipts WHERE device_id=?`, s.DeviceID).Scan(&count); e != nil {
			return e
		}
		if count >= 100000 {
			return domain.ErrDeviceBridgeInvalid
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO device_bridge_receipts VALUES(?,?,?,1,?,?,?,?)`, s.DeviceID, id, command, key, fp, now.Unix(), now.Unix()); e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `DELETE FROM device_bridge_outbox WHERE device_id=? AND message_id=?`, s.DeviceID, id)
		return e
	})
}
