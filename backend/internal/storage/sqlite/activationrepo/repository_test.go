package activationrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/bridgeactivation"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/devicebridge"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/bridgepersist"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var bg = context.Background()

func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func want(t *testing.T, e, target error) {
	t.Helper()
	if !errors.Is(e, target) {
		t.Fatalf("got %v want %v", e, target)
	}
}
func connections(t *testing.T, dir string) (*sql.DB, *sql.DB) {
	t.Helper()
	s, e := sqlite.Open(dir)
	must(t, e)
	must(t, s.Close())
	open := func() *sql.DB {
		db, e := sql.Open("sqlite", "file:"+filepath.Join(dir, "kennel.db")+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)")
		must(t, e)
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { db.Close() })
		return db
	}
	return open(), open()
}
func repos(t *testing.T) (*Repository, *Repository) {
	t.Helper()
	a, b := connections(t, t.TempDir())
	return New(a), New(b)
}
func pairedDevice(owner string) domain.DeviceBridgeDevice {
	now := time.Now().UTC().Truncate(time.Second)
	return domain.DeviceBridgeDevice{DeviceID: domain.DeviceBridgeDeviceID("dev-" + owner), OwnerID: owner, Label: "Mac", PublicKey: "public", KeyCustodyRef: "opaque-ref", ContractVersion: devicebridge.ContractVersion, CapabilityClasses: []string{"machine_state_query", "notify_local"}, State: domain.DeviceBridgeStatePaired, PairedAt: &now, CreatedAt: now, UpdatedAt: now}
}
func reserve(t *testing.T, r *Repository, owner string) bridgeactivation.Record {
	t.Helper()
	v, e := r.Reserve(bg, owner)
	must(t, e)
	return v
}
func load(t *testing.T, r *Repository, owner string) bridgeactivation.Record {
	t.Helper()
	v, e := r.Load(bg, owner)
	must(t, e)
	return v
}
func seed(t *testing.T, r *Repository, owner string) domain.DeviceBridgeDevice {
	t.Helper()
	v := reserve(t, r, owner)
	d := pairedDevice(owner)
	must(t, r.Complete(bg, owner, v.Attempt, d))
	return d
}
func snapshot(t *testing.T, db *sql.DB) []string {
	t.Helper()
	var out []string
	for _, table := range []string{"device_bridge_activations", "device_bridge_devices"} {
		rows, e := db.Query(`SELECT * FROM ` + table + ` ORDER BY 1`)
		must(t, e)
		cols, e := rows.Columns()
		must(t, e)
		for rows.Next() {
			values := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range values {
				ptrs[i] = &values[i]
			}
			must(t, rows.Scan(ptrs...))
			out = append(out, fmt.Sprint(values))
		}
		must(t, rows.Err())
		must(t, rows.Close())
	}
	return out
}
func TestReserveConcurrentTwoConnectionsOneWinner(t *testing.T) {
	a, b := repos(t)
	var wins atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 32)
	for _, r := range []*Repository{a, b} {
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func(r *Repository) {
				defer wg.Done()
				<-start
				v, e := r.Reserve(bg, "owner")
				if e == nil {
					wins.Add(1)
					if len(v.Attempt) < 22 {
						errs <- errors.New("short token")
					}
				} else if !errors.Is(e, ErrReservationActive) {
					errs <- e
				}
			}(r)
		}
	}
	close(start)
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	if wins.Load() != 1 {
		t.Fatalf("winners %d", wins.Load())
	}
	var n int
	must(t, a.db.QueryRow(`SELECT count(*) FROM device_bridge_activations`).Scan(&n))
	if n != 1 {
		t.Fatal(n)
	}
}
func TestReservationSurvivesCloseReopen(t *testing.T) {
	dir := t.TempDir()
	a, b := connections(t, dir)
	v := reserve(t, New(a), "owner")
	must(t, a.Close())
	must(t, b.Close())
	a, b = connections(t, dir)
	if load(t, New(a), "owner").Attempt != v.Attempt {
		t.Fatal("lost token")
	}
	_, e := New(b).Reserve(bg, "owner")
	want(t, e, ErrReservationActive)
}
func TestBlockWithCheckpointSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	a, b := connections(t, dir)
	r := New(a)
	v := reserve(t, r, "owner")
	cp := &devicebridge.PairingRecoveryError{OwnerID: "owner", DeviceID: "checkpoint-dev", PublicKey: "pub", KeyCustodyRef: "ref"}
	must(t, r.Block(bg, "owner", v.Attempt, cp))
	must(t, a.Close())
	must(t, b.Close())
	a, b = connections(t, dir)
	r = New(a)
	got := load(t, r, "owner")
	if got.Attempt != v.Attempt || got.Phase != "blocked" || !reflect.DeepEqual(got.Checkpoint, cp) {
		t.Fatalf("checkpoint lost %+v", got)
	}
	_, e := New(b).Reserve(bg, "owner")
	want(t, e, ErrReservationActive)
	want(t, r.Complete(bg, "owner", v.Attempt, pairedDevice("owner")), ErrAttemptMismatch)
}
func TestBlockWithoutCheckpointSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	a, b := connections(t, dir)
	r := New(a)
	v := reserve(t, r, "owner")
	must(t, r.Block(bg, "owner", v.Attempt, nil))
	must(t, a.Close())
	must(t, b.Close())
	a, _ = connections(t, dir)
	r = New(a)
	got := load(t, r, "owner")
	if got.Attempt != v.Attempt || got.Phase != "blocked" || got.Checkpoint != nil {
		t.Fatalf("nil block %+v", got)
	}
	cp := &devicebridge.PairingRecoveryError{OwnerID: "owner", KeyCustodyRef: "ref", PublicKey: "pub"}
	must(t, r.Block(bg, "owner", v.Attempt, cp))
	must(t, r.Block(bg, "owner", v.Attempt, nil))
	if !reflect.DeepEqual(load(t, r, "owner").Checkpoint, cp) {
		t.Fatal("checkpoint wiped")
	}
}
func TestBlockWrongAttemptRejected(t *testing.T) {
	r, _ := repos(t)
	v := reserve(t, r, "owner")
	before := load(t, r, "owner")
	want(t, r.Block(bg, "owner", "wrong", nil), ErrAttemptMismatch)
	want(t, r.Block(bg, "other", v.Attempt, nil), ErrAttemptMismatch)
	if r.Block(bg, "owner", v.Attempt, &devicebridge.PairingRecoveryError{OwnerID: "other"}) == nil {
		t.Fatal("wrong owner")
	}
	if !reflect.DeepEqual(before, load(t, r, "owner")) {
		t.Fatal("changed")
	}
}
func TestCompleteIsAtomicWithDeviceRow(t *testing.T) {
	for _, kind := range []string{"absent", "adopt", "public-mismatch", "ref-mismatch", "rollback"} {
		t.Run(kind, func(t *testing.T) {
			r, _ := repos(t)
			v := reserve(t, r, "owner")
			d := pairedDevice("owner")
			if kind != "absent" && kind != "rollback" {
				store := bridgepersist.New(r.db)
				initial := d
				initial.State = domain.DeviceBridgeStatePairing
				initial.PairedAt = nil
				must(t, store.CreateDeviceBridgeDevice(bg, initial))
				ok, e := store.TransitionDeviceBridgeDevice(bg, d.DeviceID, domain.DeviceBridgeStatePaired, *d.PairedAt)
				must(t, e)
				if !ok {
					t.Fatal("transition")
				}
			}
			if kind == "public-mismatch" {
				d.PublicKey = "different"
			}
			if kind == "ref-mismatch" {
				d.KeyCustodyRef = "different"
			}
			if kind == "rollback" {
				_, e := r.db.Exec(`CREATE TRIGGER reject_completion BEFORE UPDATE ON device_bridge_activations WHEN NEW.phase='paired' BEGIN SELECT RAISE(ABORT,'test'); END`)
				must(t, e)
			}
			before := snapshot(t, r.db)
			e := r.Complete(bg, "owner", v.Attempt, d)
			if kind == "public-mismatch" || kind == "ref-mismatch" || kind == "rollback" {
				if e == nil {
					t.Fatal("expected failure")
				}
				if !reflect.DeepEqual(before, snapshot(t, r.db)) {
					t.Fatal("partial commit")
				}
				return
			}
			must(t, e)
			got := load(t, r, "owner")
			if got.Attempt != "" || got.Device.DeviceID != d.DeviceID || got.Device.State != domain.DeviceBridgeStatePaired {
				t.Fatalf("complete %+v", got)
			}
			want(t, r.Complete(bg, "owner", v.Attempt, d), ErrAttemptMismatch)
		})
	}
}
func TestNoSecretMaterialInAnyColumn(t *testing.T) {
	r, _ := repos(t)
	code := strings.Repeat("A", 43)
	private := "RECOGNISABLE-PRIVATE-KEY-MATERIAL"
	v := reserve(t, r, "blocked")
	cp := &devicebridge.PairingRecoveryError{OwnerID: "blocked", KeyCustodyRef: "opaque", PublicKey: "public", Cause: errors.New(code + private)}
	must(t, r.Block(bg, "blocked", v.Attempt, cp))
	seed(t, r, "paired")
	for _, s := range snapshot(t, r.db) {
		if strings.Contains(s, code) || strings.Contains(s, private) {
			t.Fatal("secret stored")
		}
	}
	if load(t, r, "blocked").Checkpoint.Cause != nil {
		t.Fatal("cause persisted")
	}
}
func TestRevokeVsClaimTwoConnectionsFence(t *testing.T) {
	a, b := repos(t)
	for i := 0; i < 200; i++ {
		owner := fmt.Sprintf("owner-%d", i)
		d := seed(t, a, owner)
		start := make(chan struct{})
		var wg sync.WaitGroup
		var gen int64
		var claimErr, revokeErr error
		wg.Add(2)
		go func() { defer wg.Done(); <-start; gen, claimErr = a.ClaimActivation(bg, owner, d.DeviceID) }()
		go func() { defer wg.Done(); <-start; revokeErr = b.Revoke(bg, owner, d.DeviceID) }()
		close(start)
		wg.Wait()
		must(t, revokeErr)
		if claimErr != nil {
			want(t, claimErr, ErrRevoked)
		} else {
			e := a.VerifyClaim(bg, owner, d.DeviceID, gen)
			if !errors.Is(e, ErrRevoked) && !errors.Is(e, ErrStaleClaim) {
				t.Fatalf("claim survived revoke: %v", e)
			}
		}
	}
}
func TestClaimAfterRevokeRefused(t *testing.T) {
	a, b := repos(t)
	d := seed(t, a, "owner")
	_, e := a.db.Exec(`UPDATE device_bridge_activations SET checkpoint_device_id='audit',checkpoint_key_custody_ref='ref',checkpoint_public_key='pub' WHERE owner_id='owner'`)
	must(t, e)
	must(t, a.Revoke(bg, "owner", d.DeviceID))
	before := snapshot(t, a.db)
	for _, r := range []*Repository{a, b} {
		_, e = r.ClaimActivation(bg, "owner", d.DeviceID)
		want(t, e, ErrRevoked)
	}
	must(t, b.Revoke(bg, "owner", d.DeviceID))
	if !reflect.DeepEqual(before, snapshot(t, a.db)) {
		t.Fatal("repeat revoke changed data")
	}
	got := load(t, a, "owner")
	if got.Device.State != domain.DeviceBridgeStateRevoked || got.Checkpoint.DeviceID != "audit" {
		t.Fatal("audit lost")
	}
}
func TestRevokedNeverResurrected(t *testing.T) {
	r, _ := repos(t)
	d := seed(t, r, "owner")
	must(t, r.Revoke(bg, "owner", d.DeviceID))
	before := snapshot(t, r.db)
	var e error
	want(t, r.Complete(bg, "owner", "old", d), ErrAttemptMismatch)
	for _, q := range []string{`UPDATE device_bridge_activations SET phase='paired',revoked=0`, `UPDATE device_bridge_activations SET revoked=0`, `UPDATE device_bridge_activations SET claim_generation=0`, `DELETE FROM device_bridge_activations`, `UPDATE device_bridge_activations SET owner_id='other'`, `UPDATE device_bridge_activations SET created_at='2000-01-01'`, `UPDATE device_bridge_activations SET device_id='other'`} {
		if _, e = r.db.Exec(q); e == nil {
			t.Fatalf("accepted %s", q)
		}
	}
	if !reflect.DeepEqual(before, snapshot(t, r.db)) {
		t.Fatal("changed revoked row")
	}
}
func TestRevokeWrongDeviceChangesNothing(t *testing.T) {
	r, _ := repos(t)
	seed(t, r, "owner")
	before := snapshot(t, r.db)
	if r.Revoke(bg, "owner", "wrong") == nil {
		t.Fatal("wrong device accepted")
	}
	if !reflect.DeepEqual(before, snapshot(t, r.db)) {
		t.Fatal("changed")
	}
}

type fakePairer struct {
	fn func(context.Context, devicebridge.PairRequest) (domain.DeviceBridgeDevice, error)
}

func (p fakePairer) Pair(ctx context.Context, req devicebridge.PairRequest) (domain.DeviceBridgeDevice, error) {
	return p.fn(ctx, req)
}
func TestControllerIntegrationWithRealRepository(t *testing.T) {
	dir := t.TempDir()
	a, b := connections(t, dir)
	r := New(a)
	p := fakePairer{fn: func(ctx context.Context, req devicebridge.PairRequest) (domain.DeviceBridgeDevice, error) {
		d := pairedDevice(req.OwnerID)
		d.Label = req.Label
		d.CapabilityClasses = req.Capabilities
		initial := d
		initial.State = domain.DeviceBridgeStatePairing
		initial.PairedAt = nil
		store := bridgepersist.New(b)
		if e := store.CreateDeviceBridgeDevice(ctx, initial); e != nil {
			return d, e
		}
		_, e := store.TransitionDeviceBridgeDevice(ctx, d.DeviceID, domain.DeviceBridgeStatePaired, *d.PairedAt)
		return d, e
	}}
	c, e := bridgeactivation.New("owner", "https://example.test", r, p, testFactory{})
	must(t, e)
	must(t, c.Pair(bg, strings.Repeat("A", 43), "Mac", []string{"machine_state_query", "notify_local"}))
	status, e := c.Status(bg)
	must(t, e)
	if status.Phase != bridgeactivation.Offline {
		t.Fatal(status)
	}
	c2, e := bridgeactivation.New("owner", "https://example.test", New(b), p, testFactory{})
	must(t, e)
	want(t, c2.Pair(bg, strings.Repeat("A", 43), "Mac", []string{"notify_local"}), ErrReservationActive)
	interrupted := fakePairer{fn: func(context.Context, devicebridge.PairRequest) (domain.DeviceBridgeDevice, error) {
		return domain.DeviceBridgeDevice{}, &devicebridge.PairingRecoveryError{OwnerID: "interrupted", KeyCustodyRef: "opaque", PublicKey: "public"}
	}}
	c3, e := bridgeactivation.New("interrupted", "https://example.test", r, interrupted, testFactory{})
	must(t, e)
	want(t, c3.Pair(bg, strings.Repeat("A", 43), "Mac", []string{"notify_local"}), bridgeactivation.ErrBlocked)
	must(t, a.Close())
	must(t, b.Close())
	a, _ = connections(t, dir)
	c3, e = bridgeactivation.New("interrupted", "https://example.test", New(a), interrupted, testFactory{})
	must(t, e)
	status, e = c3.Status(bg)
	must(t, e)
	if status.Phase != bridgeactivation.Recovery {
		t.Fatal(status)
	}
}
func TestRevokeAfterClearPairedDeviceRow(t *testing.T) {
	a, b := repos(t)
	d := seed(t, a, "owner")
	gen, e := a.ClaimActivation(bg, "owner", d.DeviceID)
	must(t, e)
	must(t, bridgepersist.New(a.db).ClearPaired(bg, devicebridge.Scope{OwnerID: "owner", DeviceID: string(d.DeviceID)}))
	cleared := load(t, a, "owner").Device.RevokedAt
	if cleared == nil {
		t.Fatal("not cleared")
	}
	_, e = b.ClaimActivation(bg, "owner", d.DeviceID)
	want(t, e, ErrNotPaired)
	must(t, b.Revoke(bg, "owner", d.DeviceID))
	got := load(t, a, "owner")
	if got.Device.State != domain.DeviceBridgeStateRevoked || !got.Device.RevokedAt.Equal(*cleared) {
		t.Fatal("revoked_at changed")
	}
	var generation int64
	must(t, a.db.QueryRow(`SELECT claim_generation FROM device_bridge_activations WHERE owner_id='owner'`).Scan(&generation))
	if generation != gen+1 {
		t.Fatal(generation)
	}
	_, e = a.ClaimActivation(bg, "owner", d.DeviceID)
	want(t, e, ErrRevoked)
}

// Pairing integration has ready dependencies but never starts a transport.
type testFactory struct{}

func (testFactory) Ready(context.Context) error { return nil }
func (testFactory) New(context.Context, string, domain.DeviceBridgeDevice, func(devicebridge.ConnectionState)) (bridgeactivation.Session, error) {
	return nil, bridgeactivation.ErrNotReady
}

func TestRevokeThenRepairFencesOldDevice(t *testing.T) {
	r, other := repos(t)
	old := seed(t, r, "owner")
	claim, e := r.ClaimActivation(bg, "owner", old.DeviceID)
	must(t, e)
	must(t, r.Revoke(bg, "owner", old.DeviceID))
	reservation := reserve(t, other, "owner")
	next := pairedDevice("owner")
	next.DeviceID = "fresh-device"
	must(t, other.Complete(bg, "owner", reservation.Attempt, next))
	if r.VerifyClaim(bg, "owner", old.DeviceID, claim) == nil {
		t.Fatal("old claim resurrected")
	}
	if _, e = r.ClaimActivation(bg, "owner", old.DeviceID); e == nil {
		t.Fatal("old device claimed")
	}
	if _, e = r.ClaimActivation(bg, "owner", next.DeviceID); e != nil {
		t.Fatal(e)
	}
	if _, e = r.db.Exec(`UPDATE device_bridge_devices SET state='paired',revoked_at=NULL WHERE device_id=?`, old.DeviceID); e == nil {
		t.Fatal("old identity resurrected")
	}
}
