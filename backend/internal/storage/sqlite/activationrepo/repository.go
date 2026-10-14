// Package activationrepo owns durable local activation and revocation fences.
// Codes and private keys never cross this repository boundary.
package activationrepo

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/bridgeactivation"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/devicebridge"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

var (
	ErrRevoked           = errors.New("activation revoked")
	ErrNotPaired         = errors.New("activation not paired")
	ErrStaleClaim        = errors.New("activation claim stale")
	ErrAttemptMismatch   = errors.New("activation attempt mismatch")
	ErrReservationActive = errors.New("activation reservation active")
)

type Repository struct{ db *sql.DB }

var _ bridgeactivation.Repository = (*Repository)(nil)

// New requires a dedicated, migrated connection with foreign_keys and
// busy_timeout enabled. The caller retains responsibility for closing it.
func New(db *sql.DB) *Repository { db.SetMaxOpenConns(1); return &Repository{db: db} }

// immediate serializes read/write decisions across independent processes, not
// merely across one connection pool. Rollback uses a live context even when the
// caller cancels; a cancelled transaction must never return to the pool open.
func (r *Repository) immediate(ctx context.Context, fn func(*sql.Conn) error) error {
	c, err := r.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	if _, err = c.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer c.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
	if err = fn(c); err != nil {
		return err
	}
	_, err = c.ExecContext(ctx, "COMMIT")
	return err
}

// readOnly uses a deferred BEGIN snapshot. In WAL mode a held reader never
// owns the writer reservation needed by the device store's ClearPaired.
func (r *Repository) readOnly(ctx context.Context, fn func(*sql.Conn) error) error {
	c, err := r.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	if _, err = c.ExecContext(ctx, "BEGIN"); err != nil {
		return err
	}
	defer c.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
	if err = fn(c); err != nil {
		return err
	}
	_, err = c.ExecContext(ctx, "COMMIT")
	return err
}

type row struct {
	attempt, phase, checkpointID, ref, public string
	device                                    sql.NullString
	revoked                                   bool
	generation                                int64
}

func read(ctx context.Context, c *sql.Conn, owner string) (row, error) {
	var v row
	err := c.QueryRowContext(ctx, `SELECT attempt,phase,checkpoint_device_id,checkpoint_key_custody_ref,checkpoint_public_key,device_id,revoked,claim_generation FROM device_bridge_activations WHERE owner_id=?`, owner).Scan(&v.attempt, &v.phase, &v.checkpointID, &v.ref, &v.public, &v.device, &v.revoked, &v.generation)
	return v, err
}
func device(ctx context.Context, c *sql.Conn, id string) (domain.DeviceBridgeDevice, error) {
	var d domain.DeviceBridgeDevice
	var caps string
	var paired, revoked, last sql.NullTime
	err := c.QueryRowContext(ctx, `SELECT device_id,owner_id,label,public_key,key_custody_ref,contract_version,capability_classes,state,paired_at,revoked_at,last_seen_at,created_at,updated_at FROM device_bridge_devices WHERE device_id=?`, id).Scan(&d.DeviceID, &d.OwnerID, &d.Label, &d.PublicKey, &d.KeyCustodyRef, &d.ContractVersion, &caps, &d.State, &paired, &revoked, &last, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return d, err
	}
	if err = json.Unmarshal([]byte(caps), &d.CapabilityClasses); err != nil {
		return d, err
	}
	if paired.Valid {
		d.PairedAt = &paired.Time
	}
	if revoked.Valid {
		d.RevokedAt = &revoked.Time
	}
	if last.Valid {
		d.LastSeenAt = &last.Time
	}
	return d, nil
}
func (r *Repository) Load(ctx context.Context, owner string) (out bridgeactivation.Record, err error) {
	out.OwnerID = owner
	err = r.readOnly(ctx, func(c *sql.Conn) error {
		v, e := read(ctx, c, owner)
		if errors.Is(e, sql.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		out.Phase = bridgeactivation.Phase(v.phase)
		out.Attempt = v.attempt
		if v.checkpointID != "" || v.ref != "" || v.public != "" {
			out.Checkpoint = &devicebridge.PairingRecoveryError{OwnerID: owner, DeviceID: v.checkpointID, KeyCustodyRef: v.ref, PublicKey: v.public}
		}
		if v.phase == "paired" || v.phase == "revoked" {
			out.Device, e = device(ctx, c, v.device.String)
			if e == nil && out.Device.OwnerID != owner {
				return domain.ErrDeviceBridgeInvalid
			}
			return e
		}
		return nil
	})
	return
}

// BoundOwner returns the Waldo owner this Mac is bound to, or "" before its first
// owner-scoped pairing.
func (r *Repository) BoundOwner(ctx context.Context) (owner string, err error) {
	err = r.readOnly(ctx, func(c *sql.Conn) error {
		e := c.QueryRowContext(ctx, `SELECT owner_id FROM device_bridge_owner_binding WHERE singleton=1`).Scan(&owner)
		if errors.Is(e, sql.ErrNoRows) {
			return nil
		}
		return e
	})
	return
}

// BindOwner records owner as this Mac's Waldo owner. Rebinding is refused while the
// currently bound owner has a pending, blocked or paired activation, decided in the
// same transaction so a live or recoverable identity is never orphaned.
func (r *Repository) BindOwner(ctx context.Context, owner string) error {
	if owner == "" {
		return domain.ErrDeviceBridgeInvalid
	}
	return r.immediate(ctx, func(c *sql.Conn) error {
		var current string
		e := c.QueryRowContext(ctx, `SELECT owner_id FROM device_bridge_owner_binding WHERE singleton=1`).Scan(&current)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if current == owner {
			return nil
		}
		if current != "" {
			v, e := read(ctx, c, current)
			if e != nil && !errors.Is(e, sql.ErrNoRows) {
				return e
			}
			if e == nil && v.phase != "revoked" {
				return ErrReservationActive
			}
		}
		_, e = c.ExecContext(ctx, `INSERT INTO device_bridge_owner_binding(singleton,owner_id,bound_at) VALUES(1,?,?) ON CONFLICT(singleton) DO UPDATE SET owner_id=excluded.owner_id,bound_at=excluded.bound_at`, owner, time.Now().UTC())
		return e
	})
}

func (r *Repository) Reserve(ctx context.Context, owner string) (out bridgeactivation.Record, err error) {
	var token [32]byte
	if _, err = rand.Read(token[:]); err != nil {
		return out, err
	}
	out = bridgeactivation.Record{OwnerID: owner, Attempt: base64.RawURLEncoding.EncodeToString(token[:]), Phase: "pending"}
	err = r.immediate(ctx, func(c *sql.Conn) error {
		v, e := read(ctx, c, owner)
		if e == nil {
			if v.revoked {
				var n int
				if e = c.QueryRowContext(ctx, `SELECT count(*) FROM device_bridge_devices WHERE owner_id=? AND state='paired'`, owner).Scan(&n); e != nil {
					return e
				}
				if n != 0 {
					return ErrReservationActive
				}
				_, e = c.ExecContext(ctx, `INSERT INTO device_bridge_revoked_identities(owner_id,device_id,generation,revoked_at) VALUES(?,?,?,?) ON CONFLICT DO NOTHING`, owner, v.device.String, v.generation, time.Now().UTC())
				if e != nil {
					return e
				}
				_, e = c.ExecContext(ctx, `UPDATE device_bridge_activations SET attempt=?,phase='pending',device_id=NULL,revoked=0,checkpoint_device_id='',checkpoint_key_custody_ref='',checkpoint_public_key='',claim_generation=claim_generation+1,updated_at=? WHERE owner_id=?`, out.Attempt, time.Now().UTC(), owner)
				return e
			}
			return fmt.Errorf("reserve: %w", ErrReservationActive)
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		var n int
		if e = c.QueryRowContext(ctx, `SELECT count(*) FROM device_bridge_devices WHERE owner_id=? AND state='paired'`, owner).Scan(&n); e != nil {
			return e
		}
		if n != 0 {
			return fmt.Errorf("reserve: %w", ErrReservationActive)
		}
		now := time.Now().UTC()
		_, e = c.ExecContext(ctx, `INSERT INTO device_bridge_activations(owner_id,attempt,phase,created_at,updated_at) VALUES(?,?,'pending',?,?)`, owner, out.Attempt, now, now)
		return e
	})
	if err != nil {
		out = bridgeactivation.Record{}
	}
	return
}
func (r *Repository) Block(ctx context.Context, owner, attempt string, cp *devicebridge.PairingRecoveryError) error {
	return r.immediate(ctx, func(c *sql.Conn) error {
		v, e := read(ctx, c, owner)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if e != nil || attempt == "" || v.attempt != attempt || (v.phase != "pending" && v.phase != "blocked") {
			return fmt.Errorf("block: %w", ErrAttemptMismatch)
		}
		if cp != nil {
			if cp.OwnerID != owner {
				return domain.ErrDeviceBridgeInvalid
			}
			v.checkpointID = cp.DeviceID
			v.ref = cp.KeyCustodyRef
			v.public = cp.PublicKey
		}
		_, e = c.ExecContext(ctx, `UPDATE device_bridge_activations SET phase='blocked',checkpoint_device_id=?,checkpoint_key_custody_ref=?,checkpoint_public_key=?,updated_at=? WHERE owner_id=?`, v.checkpointID, v.ref, v.public, time.Now().UTC(), owner)
		return e
	})
}
func (r *Repository) Complete(ctx context.Context, owner, attempt string, d domain.DeviceBridgeDevice) error {
	return r.immediate(ctx, func(c *sql.Conn) error {
		v, e := read(ctx, c, owner)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if e != nil || attempt == "" || v.attempt != attempt || v.phase != "pending" {
			return fmt.Errorf("complete: %w", ErrAttemptMismatch)
		}
		if d.OwnerID != owner || d.State != domain.DeviceBridgeStatePaired || d.Validate() != nil {
			return domain.ErrDeviceBridgeInvalid
		}
		existing, e := device(ctx, c, string(d.DeviceID))
		if errors.Is(e, sql.ErrNoRows) {
			caps, err := json.Marshal(d.CapabilityClasses)
			if err != nil {
				return err
			}
			_, e = c.ExecContext(ctx, `INSERT INTO device_bridge_devices(device_id,owner_id,label,public_key,key_custody_ref,contract_version,capability_classes,state,paired_at,revoked_at,last_seen_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, d.DeviceID, d.OwnerID, d.Label, d.PublicKey, d.KeyCustodyRef, d.ContractVersion, string(caps), d.State, d.PairedAt, d.RevokedAt, d.LastSeenAt, d.CreatedAt, d.UpdatedAt)
			if e != nil {
				return e
			}
		} else if e != nil {
			return e
		} else if existing.OwnerID != d.OwnerID || existing.PublicKey != d.PublicKey || existing.KeyCustodyRef != d.KeyCustodyRef || existing.ContractVersion != d.ContractVersion || !slices.Equal(existing.CapabilityClasses, d.CapabilityClasses) || existing.State != domain.DeviceBridgeStatePaired {
			return domain.ErrDeviceBridgeConflict
		}
		_, e = c.ExecContext(ctx, `UPDATE device_bridge_activations SET phase='paired',attempt='',device_id=?,updated_at=? WHERE owner_id=?`, d.DeviceID, time.Now().UTC(), owner)
		return e
	})
}
func (r *Repository) Revoke(ctx context.Context, owner string, id domain.DeviceBridgeDeviceID) error {
	return r.immediate(ctx, func(c *sql.Conn) error {
		v, e := read(ctx, c, owner)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if e != nil || !v.device.Valid || v.device.String != string(id) {
			return domain.ErrDeviceBridgeConflict
		}
		if v.revoked {
			return nil
		}
		d, e := device(ctx, c, string(id))
		if e != nil {
			return e
		}
		if d.OwnerID != owner {
			return domain.ErrDeviceBridgeConflict
		}
		now := time.Now().UTC()
		if d.State != domain.DeviceBridgeStateRevoked {
			if d.State != domain.DeviceBridgeStatePaired && d.State != domain.DeviceBridgeStatePairing {
				return domain.ErrDeviceBridgeInvalid
			}
			if _, e = c.ExecContext(ctx, `UPDATE device_bridge_devices SET state='revoked',revoked_at=?,updated_at=? WHERE device_id=? AND owner_id=?`, now, now, id, owner); e != nil {
				return e
			}
		}
		if _, e = c.ExecContext(ctx, `INSERT INTO device_bridge_revoked_identities(owner_id,device_id,generation,revoked_at) VALUES(?,?,?,?) ON CONFLICT DO NOTHING`, owner, id, v.generation+1, now); e != nil {
			return e
		}
		_, e = c.ExecContext(ctx, `UPDATE device_bridge_activations SET phase='revoked',attempt='',revoked=1,claim_generation=claim_generation+1,updated_at=? WHERE owner_id=?`, now, owner)
		return e
	})
}
func (r *Repository) ClaimActivation(ctx context.Context, owner string, id domain.DeviceBridgeDeviceID) (generation int64, err error) {
	err = r.immediate(ctx, func(c *sql.Conn) error {
		e := c.QueryRowContext(ctx, `UPDATE device_bridge_activations SET claim_generation=claim_generation+1,updated_at=? WHERE owner_id=? AND device_id=? AND phase='paired' AND revoked=0 AND EXISTS(SELECT 1 FROM device_bridge_devices d WHERE d.device_id=device_bridge_activations.device_id AND d.owner_id=device_bridge_activations.owner_id AND d.state='paired') RETURNING claim_generation`, time.Now().UTC(), owner, id).Scan(&generation)
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		v, e := read(ctx, c, owner)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if v.revoked {
			return fmt.Errorf("claim: %w", ErrRevoked)
		}
		return fmt.Errorf("claim: %w", ErrNotPaired)
	})
	return
}
func (r *Repository) VerifyClaim(ctx context.Context, owner string, id domain.DeviceBridgeDeviceID, generation int64) error {
	return r.readOnly(ctx, func(c *sql.Conn) error {
		v, e := read(ctx, c, owner)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if v.revoked {
			return fmt.Errorf("verify: %w", ErrRevoked)
		}
		if e != nil || v.phase != "paired" || !v.device.Valid || v.device.String != string(id) || v.generation != generation || generation < 1 {
			return fmt.Errorf("verify: %w", ErrStaleClaim)
		}
		d, e := device(ctx, c, string(id))
		if e != nil {
			return e
		}
		if d.OwnerID != owner || d.State != domain.DeviceBridgeStatePaired {
			return fmt.Errorf("verify: %w", ErrStaleClaim)
		}
		return nil
	})
}

var _ bridgeactivation.Checkpointer = (*Repository)(nil)

// Checkpoint retains the first custody locator without releasing the reservation.
// A different locator is a conflicting key generation and may not overwrite it.
func (r *Repository) Checkpoint(ctx context.Context, owner, attempt string, cp devicebridge.PairingRecoveryError) error {
	return r.immediate(ctx, func(c *sql.Conn) error {
		v, e := read(ctx, c, owner)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if e != nil || attempt == "" || v.attempt != attempt || v.phase != "pending" {
			return fmt.Errorf("checkpoint: %w", ErrAttemptMismatch)
		}
		if cp.OwnerID != owner || cp.KeyCustodyRef == "" || cp.PublicKey == "" {
			return domain.ErrDeviceBridgeInvalid
		}
		if v.ref != "" || v.public != "" {
			if v.ref != cp.KeyCustodyRef || v.public != cp.PublicKey {
				return domain.ErrDeviceBridgeConflict
			}
			return nil
		}
		_, e = c.ExecContext(ctx, `UPDATE device_bridge_activations SET checkpoint_key_custody_ref=?,checkpoint_public_key=?,updated_at=? WHERE owner_id=?`, cp.KeyCustodyRef, cp.PublicKey, time.Now().UTC(), owner)
		return e
	})
}
