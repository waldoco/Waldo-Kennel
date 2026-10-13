package macactionrepo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/macaction"
	"time"
)

var ErrConflict = errors.New("Mac approval binding conflict")

type Repository struct{ db *sql.DB }

func New(db *sql.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Load(ctx context.Context, device, id string) (out macaction.Request, err error) {
	var raw string
	var result sql.NullString
	err = r.db.QueryRowContext(ctx, `SELECT request,state,result FROM mac_action_requests WHERE device_id=? AND command_id=?`, device, id).Scan(&raw, &out.State, &result)
	if err != nil {
		return
	}
	state := out.State
	err = json.Unmarshal([]byte(raw), &out)
	out.State = state
	if err == nil && result.Valid {
		err = json.Unmarshal([]byte(result.String), &out.Result)
	}
	return
}
func (r *Repository) Pending(ctx context.Context, q macaction.Request) (macaction.Request, error) {
	raw, e := json.Marshal(q)
	if e != nil {
		return q, e
	}
	_, e = r.db.ExecContext(ctx, `INSERT INTO mac_action_requests(device_id,command_id,owner_id,fingerprint,project_id,request,expires_at,state) SELECT ?,?,?,?,?,?,?,'pending' WHERE EXISTS(SELECT 1 FROM device_bridge_devices WHERE device_id=? AND owner_id=? AND state='paired') ON CONFLICT(device_id,command_id) DO NOTHING`, q.DeviceID, q.ID, q.OwnerID, q.Fingerprint, q.Project, string(raw), q.ExpiresAt, q.DeviceID, q.OwnerID)
	if e != nil {
		return q, e
	}
	old, e := r.Load(ctx, q.DeviceID, q.ID)
	if e == nil && old.Fingerprint != q.Fingerprint {
		return q, ErrConflict
	}
	return old, e
}
func (r *Repository) Start(ctx context.Context, q macaction.Request, now int64) (bool, error) {
	v, e := r.db.ExecContext(ctx, `UPDATE mac_action_requests SET state='started',started_at=? WHERE device_id=? AND command_id=? AND fingerprint=? AND (state='approved' OR (state='pending' AND json_extract(request,'$.class')='session_control' AND json_extract(request,'$.action')='interrupt_session')) AND expires_at>? AND EXISTS(SELECT 1 FROM device_bridge_devices WHERE device_id=? AND state='paired')`, now, q.DeviceID, q.ID, q.Fingerprint, now, q.DeviceID)
	if e != nil {
		return false, e
	}
	n, e := v.RowsAffected()
	return n == 1, e
}
func (r *Repository) Finish(ctx context.Context, q macaction.Request, result map[string]any, output []byte) error {
	raw, e := json.Marshal(result)
	if e != nil {
		return e
	}
	v, e := r.db.ExecContext(ctx, `UPDATE mac_action_requests SET state='finished',result=?,local_output=? WHERE device_id=? AND command_id=? AND fingerprint=? AND state<>'finished'`, string(raw), output, q.DeviceID, q.ID, q.Fingerprint)
	if e != nil {
		return e
	}
	n, e := v.RowsAffected()
	if e != nil {
		return e
	}
	if n == 1 {
		return nil
	}
	old, e := r.Load(ctx, q.DeviceID, q.ID)
	if e != nil {
		return e
	}
	a, _ := json.Marshal(old.Result)
	if string(a) != string(raw) {
		return ErrConflict
	}
	return nil
}

// Decide is called only by the authenticated native owner-command route. It
// races safely with expiry and Start; a late click cannot create permission.
func (r *Repository) Decide(ctx context.Context, device, id, fingerprint string, approve bool, now time.Time) error {
	state := "denied"
	if approve {
		state = "approved"
	}
	v, e := r.db.ExecContext(ctx, `UPDATE mac_action_requests SET state=? WHERE device_id=? AND command_id=? AND fingerprint=? AND state='pending' AND expires_at>? AND EXISTS(SELECT 1 FROM device_bridge_devices WHERE device_id=? AND state='paired')`, state, device, id, fingerprint, now.Unix(), device)
	if e != nil {
		return e
	}
	n, e := v.RowsAffected()
	if n != 1 {
		return ErrConflict
	}
	return e
}
func (r *Repository) Open(ctx context.Context) ([]macaction.Request, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT request,state FROM mac_action_requests WHERE state='pending' AND json_extract(request,'$.action') IS NOT 'interrupt_session' AND expires_at>? ORDER BY expires_at LIMIT 20`, time.Now().Unix())
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []macaction.Request
	for rows.Next() {
		var raw, state string
		if e = rows.Scan(&raw, &state); e != nil {
			return nil, e
		}
		var q macaction.Request
		if e = json.Unmarshal([]byte(raw), &q); e != nil {
			return nil, e
		}
		q.State = state
		out = append(out, q)
	}
	return out, rows.Err()
}
func (r *Repository) Output(ctx context.Context, device, id string) ([]byte, error) {
	var out []byte
	e := r.db.QueryRowContext(ctx, `SELECT local_output FROM mac_action_requests WHERE device_id=? AND command_id=? AND state='finished'`, device, id).Scan(&out)
	return out, e
}

// History is a bounded, local-only projection. Full output is read separately
// by the authenticated main process; it is never added to a bridge frame.
func (r *Repository) History(ctx context.Context) ([]macaction.Request, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT request,state FROM mac_action_requests WHERE state='finished' ORDER BY rowid DESC LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []macaction.Request{}
	for rows.Next() {
		var raw, state string
		if err = rows.Scan(&raw, &state); err != nil {
			return nil, err
		}
		var q macaction.Request
		if err = json.Unmarshal([]byte(raw), &q); err != nil {
			return nil, err
		}
		q.State = state
		out = append(out, q)
	}
	return out, rows.Err()
}
