package bridgepersist

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/devicebridge"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/store"
)

// BridgeSession is deliberately opt-in. Daemon activation belongs to a later,
// reviewed integration slice. Pairing uses the existing device lifecycle store.
type BridgeSession struct {
	*store.Store
	writeDB *sql.DB
	writeMu sync.Mutex
}

var _ devicebridge.SessionStore = (*BridgeSession)(nil)
var _ devicebridge.PairingStore = (*BridgeSession)(nil)

// New takes ownership of a migrated database connection. Use a dedicated
// connection with foreign_keys and busy_timeout enabled, not the daemon Store pool.
func New(db *sql.DB) *BridgeSession {
	db.SetMaxOpenConns(1)
	return &BridgeSession{Store: store.NewStore(db, db), writeDB: db}
}

func bridgeJSON(raw []byte) (map[string]any, error) {
	var f map[string]any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&f); err != nil {
		return nil, err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, devicebridge.ErrInvalidFrameShape
	}
	if _, err := devicebridge.LogicalFingerprint(f); err != nil {
		return nil, err
	}
	return f, nil
}
func bridgeFP(f map[string]any) (string, error) {
	h, e := devicebridge.LogicalFingerprint(f)
	return hex.EncodeToString(h[:]), e
}
func bridgeID(f map[string]any, k string) string { v, _ := f[k].(string); return v }
func bridgeBound(f map[string]any, s devicebridge.Scope) bool {
	return bridgeID(f, "device_id") == s.DeviceID && bridgeID(f, "owner_id") == s.OwnerID
}
func bridgeExpires(f map[string]any) int64 {
	n, _ := f["expires_at"].(json.Number)
	i, _ := n.Int64()
	return i
}

func (b *BridgeSession) tx(ctx context.Context, s devicebridge.Scope, active bool, fn func(*sql.Tx) error) error {
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	tx, e := b.writeDB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var state string
	e = tx.QueryRowContext(ctx, `SELECT state FROM device_bridge_devices WHERE device_id=? AND owner_id=?`, s.DeviceID, s.OwnerID).Scan(&state)
	if e != nil {
		return e
	}
	if active && state != "paired" {
		return domain.ErrDeviceBridgeInvalid
	}
	if e = fn(tx); e != nil {
		return e
	}
	return tx.Commit()
}
func bridgeDedup(ctx context.Context, tx *sql.Tx, s devicebridge.Scope, f map[string]any) error {
	fp, e := bridgeFP(f)
	if e != nil {
		return e
	}
	var old string
	e = tx.QueryRowContext(ctx, `SELECT fingerprint FROM device_bridge_fingerprints WHERE device_id=? AND message_id=?`, s.DeviceID, bridgeID(f, "message_id")).Scan(&old)
	if e == nil {
		if old != fp {
			return domain.ErrDeviceBridgeConflict
		}
		return nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO device_bridge_fingerprints VALUES(?,?,?)`, s.DeviceID, bridgeID(f, "message_id"), fp)
	return e
}
func (b *BridgeSession) Pending(ctx context.Context, s devicebridge.Scope) (out []devicebridge.PendingResult, err error) {
	err = b.tx(ctx, s, true, func(tx *sql.Tx) error {
		rows, e := tx.QueryContext(ctx, `SELECT seq,frame FROM device_bridge_outbox WHERE device_id=? AND state!='receipted' ORDER BY seq`, s.DeviceID)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v devicebridge.PendingResult
			var f sql.NullString
			if e = rows.Scan(&v.Seq, &f); e != nil {
				return e
			}
			if !f.Valid {
				return fmt.Errorf("legacy outbox needs explicit frame repair at seq %d", v.Seq)
			}
			v.Frame = []byte(f.String)
			out = append(out, v)
		}
		return rows.Err()
	})
	return
}
func (b *BridgeSession) Depth(ctx context.Context, s devicebridge.Scope) (n int64, err error) {
	err = b.tx(ctx, s, true, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT count(*) FROM device_bridge_outbox WHERE device_id=? AND state!='receipted'`, s.DeviceID).Scan(&n)
	})
	return
}
func (b *BridgeSession) AcceptedUnresulted(ctx context.Context, s devicebridge.Scope) (out [][]byte, err error) {
	err = b.tx(ctx, s, true, func(tx *sql.Tx) error {
		rows, e := tx.QueryContext(ctx, `SELECT command_id,frame FROM device_bridge_inbox_journal WHERE device_id=? AND state IN ('received','acked') ORDER BY created_at,command_id`, s.DeviceID)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var f sql.NullString
			if e = rows.Scan(&id, &f); e != nil {
				return e
			}
			if !f.Valid {
				return fmt.Errorf("legacy command needs explicit frame repair: %s", id)
			}
			out = append(out, []byte(f.String))
		}
		return rows.Err()
	})
	return
}
func (b *BridgeSession) Admit(ctx context.Context, s devicebridge.Scope, raw []byte, now time.Time) (a devicebridge.Admission, err error) {
	f, e := devicebridge.ParseBackendFrame(raw)
	if e != nil {
		return a, e
	}
	if !bridgeBound(f, s) || f["type"] != devicebridge.TypeCommand || now.IsZero() {
		return a, domain.ErrDeviceBridgeInvalid
	}
	a = devicebridge.Admission{State: devicebridge.AckRejected, Reason: devicebridge.ReasonInvalidShape}
	exp := bridgeExpires(f)
	if exp <= now.Unix() {
		return devicebridge.Admission{State: devicebridge.AckExpired, Reason: devicebridge.ReasonExpired}, nil
	}
	maxTTL := int64(devicebridge.CommandTTLMaxSeconds)
	if f["class"] == devicebridge.ClassShellExec || f["class"] == devicebridge.ClassSessionQuery || f["class"] == devicebridge.ClassSessionControl {
		maxTTL = devicebridge.ApprovalTTLSeconds
	}
	if exp-now.Unix() > maxTTL {
		return a, nil
	}
	err = b.tx(ctx, s, true, func(tx *sql.Tx) error {
		var n int
		// Prune only tombstones outside the command expiry + replay horizon and
		// without in-flight outbox. The permanent command journal is untouched.
		if _, e := tx.ExecContext(ctx, `DELETE FROM device_bridge_receipts WHERE device_id=? AND expires_at+300<=? AND NOT EXISTS(SELECT 1 FROM device_bridge_outbox o WHERE o.device_id=device_bridge_receipts.device_id AND o.message_id=device_bridge_receipts.result_message_id)`, s.DeviceID, now.Unix()); e != nil {
			return e
		}
		if e := tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM device_bridge_receipts WHERE device_id=?)+(SELECT count(*) FROM device_bridge_inbox_journal WHERE device_id=? AND state IN ('received','acked'))+(SELECT count(*) FROM device_bridge_outbox WHERE device_id=? AND state!='receipted')`, s.DeviceID, s.DeviceID, s.DeviceID).Scan(&n); e != nil {
			return e
		}
		var oldID, oldKey, oldFP, state string
		var saved sql.NullString
		// Message-id is excluded here: a fresh backend message may redeliver the
		// same command. Every other stable field, including expiry, is pinned.
		logical := make(map[string]any)
		for k, v := range f {
			if k != "message_id" {
				logical[k] = v
			}
		}
		enc, e := json.Marshal(logical)
		if e != nil {
			return e
		}
		commandFP := domain.DigestSHA256(enc)
		e = tx.QueryRowContext(ctx, `SELECT command_id,idempotency_key,payload_fingerprint,state,result_frame FROM device_bridge_inbox_journal WHERE device_id=? AND (command_id=? OR idempotency_key=?)`, s.DeviceID, bridgeID(f, "command_id"), bridgeID(f, "idempotency_key")).Scan(&oldID, &oldKey, &oldFP, &state, &saved)
		if e == nil {
			if e := bridgeDedup(ctx, tx, s, f); e != nil {
				if errors.Is(e, domain.ErrDeviceBridgeConflict) {
					a.Reason = devicebridge.ReasonIdempotencyConflict
					return nil
				}
				return e
			}
			if oldID != bridgeID(f, "command_id") || oldKey != bridgeID(f, "idempotency_key") || oldFP != string(commandFP) {
				a.Reason = devicebridge.ReasonIdempotencyConflict
				return nil
			}
			if saved.Valid {
				result, e := bridgeJSON([]byte(saved.String))
				if e != nil {
					return e
				}
				fp, e := bridgeFP(result)
				if e != nil {
					return e
				}
				payload, _ := json.Marshal(result["payload"])
				_, e = tx.ExecContext(ctx, `INSERT INTO device_bridge_outbox(device_id,message_id,command_id,revision,class,payload,state,created_at,frame,idempotency_key,fingerprint) VALUES(?,?,?,1,'result',?,'pending',?,?,?,?) ON CONFLICT(device_id,command_id,revision) DO NOTHING`, s.DeviceID, bridgeID(result, "message_id"), oldID, string(payload), now.UTC(), saved.String, oldKey, fp)
				if e != nil {
					return e
				}
			}
			a = devicebridge.Admission{State: devicebridge.AckAccepted}
			return nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if n >= 100000 {
			return nil
		}
		var active int
		if e := tx.QueryRowContext(ctx, `SELECT count(*) FROM device_bridge_inbox_journal WHERE device_id=? AND state IN ('received','acked')`, s.DeviceID).Scan(&active); e != nil {
			return e
		}
		if active != 0 {
			return nil
		}
		if e := bridgeDedup(ctx, tx, s, f); e != nil {
			if errors.Is(e, domain.ErrDeviceBridgeConflict) {
				a.Reason = devicebridge.ReasonIdempotencyConflict
				return nil
			}
			return e
		}
		payload, _ := json.Marshal(f["payload"])
		_, e = tx.ExecContext(ctx, `INSERT INTO device_bridge_inbox_journal(device_id,command_id,revision,idempotency_key,payload_fingerprint,class,payload,state,created_at,updated_at,frame,expires_at) VALUES(?,?,1,?,?,?,?,'acked',?,?,?,?)`, s.DeviceID, bridgeID(f, "command_id"), bridgeID(f, "idempotency_key"), string(commandFP), bridgeID(f, "class"), string(payload), now.UTC(), now.UTC(), string(raw), exp)
		if e != nil {
			return e
		}
		a = devicebridge.Admission{State: devicebridge.AckAccepted, New: true}
		return nil
	})
	return
}
func (b *BridgeSession) CommitResult(ctx context.Context, s devicebridge.Scope, command, result []byte) error {
	c, e := devicebridge.ParseBackendFrame(command)
	if e != nil {
		return e
	}
	r, e := bridgeJSON(result)
	if e != nil {
		return e
	}
	if !bridgeBound(c, s) || !bridgeBound(r, s) {
		return domain.ErrDeviceBridgeInvalid
	}
	if e = devicebridge.ValidateResultForCommand(r, c); e != nil {
		return e
	}
	fp, e := bridgeFP(r)
	if e != nil {
		return e
	}
	return b.tx(ctx, s, true, func(tx *sql.Tx) error {
		var old, previous sql.NullString
		e := tx.QueryRowContext(ctx, `SELECT frame,result_frame FROM device_bridge_inbox_journal WHERE device_id=? AND command_id=? AND revision=1 AND idempotency_key=?`, s.DeviceID, bridgeID(c, "command_id"), bridgeID(c, "idempotency_key")).Scan(&old, &previous)
		if e != nil {
			return e
		}
		if !old.Valid {
			return domain.ErrDeviceBridgeConflict
		}
		priorCommand, e := devicebridge.ParseBackendFrame([]byte(old.String))
		if e != nil {
			return e
		}
		// Backend may redeliver the same command under a new message ID.
		delete(priorCommand, "message_id")
		logical := make(map[string]any)
		for k, v := range c {
			if k != "message_id" {
				logical[k] = v
			}
		}
		priorBytes, e := json.Marshal(priorCommand)
		if e != nil {
			return e
		}
		incomingBytes, e := json.Marshal(logical)
		if e != nil {
			return e
		}
		if !bytes.Equal(priorBytes, incomingBytes) {
			return domain.ErrDeviceBridgeConflict
		}
		if previous.Valid {
			prior, e := bridgeJSON([]byte(previous.String))
			if e != nil {
				return e
			}
			p, e := bridgeFP(prior)
			if e != nil {
				return e
			}
			if p != fp {
				return domain.ErrDeviceBridgeConflict
			}
			return nil
		}
		if e = bridgeDedup(ctx, tx, s, r); e != nil {
			return e
		}
		payload, _ := json.Marshal(r["payload"])
		if _, e = tx.ExecContext(ctx, `INSERT INTO device_bridge_outbox(device_id,message_id,command_id,revision,class,payload,state,created_at,frame,idempotency_key,fingerprint) VALUES(?,?,?,1,'result',?,'pending',?,?,?,?)`, s.DeviceID, bridgeID(r, "message_id"), bridgeID(r, "command_id"), string(payload), time.Now().UTC(), string(result), bridgeID(r, "idempotency_key"), fp); e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `UPDATE device_bridge_inbox_journal SET state='resulted',result_frame=?,updated_at=? WHERE device_id=? AND command_id=? AND revision=1`, string(result), time.Now().UTC(), s.DeviceID, bridgeID(c, "command_id"))
		return e
	})
}
func (b *BridgeSession) ApplyReceipt(ctx context.Context, s devicebridge.Scope, raw []byte, now time.Time) error {
	r, e := devicebridge.ParseBackendFrame(raw)
	if e != nil {
		return e
	}
	if r["type"] != devicebridge.TypeReceipt || !bridgeBound(r, s) || now.IsZero() {
		return domain.ErrDeviceBridgeInvalid
	}
	if r["payload"].(map[string]any)["event_message_id"] != nil {
		return b.applyEventReceipt(ctx, s, r, now)
	}
	id := r["payload"].(map[string]any)["result_message_id"].(string)
	return b.tx(ctx, s, true, func(tx *sql.Tx) error {
		var command, key string
		var rev, exp int64
		e := tx.QueryRowContext(ctx, `SELECT command_id,revision,idempotency_key,expires_at FROM device_bridge_receipts WHERE device_id=? AND result_message_id=?`, s.DeviceID, id).Scan(&command, &rev, &key, &exp)
		if e == nil {
			if command != bridgeID(r, "command_id") || key != bridgeID(r, "idempotency_key") {
				return domain.ErrDeviceBridgeConflict
			}
			if e := bridgeDedup(ctx, tx, s, r); e != nil {
				return e
			}
			// A command redelivery may have re-spooled this same journaled result.
			// Match its retained fingerprint before applying the duplicate receipt.
			var n int
			e = tx.QueryRowContext(ctx, `SELECT count(*) FROM device_bridge_outbox o JOIN device_bridge_receipts r ON o.device_id=r.device_id AND o.message_id=r.result_message_id WHERE o.device_id=? AND o.message_id=? AND o.fingerprint!=r.result_fingerprint`, s.DeviceID, id).Scan(&n)
			if e != nil {
				return e
			}
			if n != 0 {
				return domain.ErrDeviceBridgeConflict
			}
			_, e = tx.ExecContext(ctx, `DELETE FROM device_bridge_outbox WHERE device_id=? AND message_id=?`, s.DeviceID, id)
			return e
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		var frame sql.NullString
		var fp string
		e = tx.QueryRowContext(ctx, `SELECT o.frame,o.fingerprint,j.expires_at FROM device_bridge_outbox o JOIN device_bridge_inbox_journal j ON j.device_id=o.device_id AND j.command_id=o.command_id AND j.revision=o.revision WHERE o.device_id=? AND o.message_id=?`, s.DeviceID, id).Scan(&frame, &fp, &exp)
		if errors.Is(e, sql.ErrNoRows) {
			return devicebridge.ErrUnknownReceiptMessage
		}
		if e != nil {
			return e
		}
		if !frame.Valid {
			return domain.ErrDeviceBridgeInvalid
		}
		result, e := bridgeJSON([]byte(frame.String))
		if e != nil {
			return e
		}
		if e = devicebridge.ValidateReceiptForResult(r, result); e != nil {
			return e
		}
		if e = bridgeDedup(ctx, tx, s, r); e != nil {
			return e
		}
		var n int
		if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM device_bridge_receipts WHERE device_id=?`, s.DeviceID).Scan(&n); e != nil {
			return e
		}
		if n >= 100000 {
			return domain.ErrDeviceBridgeInvalid
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO device_bridge_receipts VALUES(?,?,?,1,?,?,?,?)`, s.DeviceID, id, bridgeID(r, "command_id"), bridgeID(r, "idempotency_key"), fp, exp, now.Unix()); e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `DELETE FROM device_bridge_outbox WHERE device_id=? AND message_id=?`, s.DeviceID, id)
		return e
	})
}
func (b *BridgeSession) ClearPaired(ctx context.Context, s devicebridge.Scope) error {
	return b.tx(ctx, s, false, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `UPDATE device_bridge_devices SET state='revoked',revoked_at=COALESCE(revoked_at,?),updated_at=? WHERE device_id=? AND owner_id=?`, time.Now().UTC(), time.Now().UTC(), s.DeviceID, s.OwnerID)
		return e
	})
}

// ResultForCommand is S4's retained-replay seam. It never starts an effect and
// never guesses a result when acceptance has not reached a committed outcome.
func (b *BridgeSession) ResultForCommand(ctx context.Context, s devicebridge.Scope, raw []byte) (result []byte, found bool, err error) {
	command, e := devicebridge.ParseBackendFrame(raw)
	if e != nil {
		return nil, false, e
	}
	if command["type"] != devicebridge.TypeCommand || !bridgeBound(command, s) {
		return nil, false, domain.ErrDeviceBridgeInvalid
	}
	err = b.tx(ctx, s, true, func(tx *sql.Tx) error {
		var original, retained sql.NullString
		var expected string
		e := tx.QueryRowContext(ctx, `SELECT frame,result_frame,payload_fingerprint FROM device_bridge_inbox_journal WHERE device_id=? AND command_id=? AND revision=1 AND idempotency_key=?`, s.DeviceID, bridgeID(command, "command_id"), bridgeID(command, "idempotency_key")).Scan(&original, &retained, &expected)
		if errors.Is(e, sql.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		if !original.Valid {
			return domain.ErrDeviceBridgeInvalid
		}
		originalCommand, e := devicebridge.ParseBackendFrame([]byte(original.String))
		if e != nil {
			return e
		}
		logical := make(map[string]any)
		for k, v := range command {
			if k != "message_id" {
				logical[k] = v
			}
		}
		incoming, e := json.Marshal(logical)
		if e != nil {
			return e
		}
		delete(originalCommand, "message_id")
		prior, e := json.Marshal(originalCommand)
		if e != nil {
			return e
		}
		if !bytes.Equal(incoming, prior) || string(domain.DigestSHA256(incoming)) != expected {
			return domain.ErrDeviceBridgeConflict
		}
		if !retained.Valid {
			return nil
		}
		frame, e := bridgeJSON([]byte(retained.String))
		if e != nil {
			return e
		}
		if e = devicebridge.ValidateResultForCommand(frame, command); e != nil {
			return e
		}
		actual, e := bridgeFP(frame)
		if e != nil {
			return e
		}
		var durable string
		e = tx.QueryRowContext(ctx, `SELECT fingerprint FROM device_bridge_fingerprints WHERE device_id=? AND message_id=?`, s.DeviceID, bridgeID(frame, "message_id")).Scan(&durable)
		if e != nil {
			return e
		}
		if actual != durable {
			return domain.ErrDeviceBridgeConflict
		}
		result = []byte(retained.String)
		found = true
		return nil
	})
	return
}
