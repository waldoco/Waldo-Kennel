package bridgepersist

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/devicebridge"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/activationrepo"
	"testing"
	"time"
)

func TestSessionEventDurableCaptureSpoolReceiptAndRevocation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, e := sqlite.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if e = store.UpsertProject(ctx, domain.ProjectRecord{ID: "p", DisplayName: "Project", Path: dir, RegisteredAt: now}); e != nil {
		t.Fatal(e)
	}
	session, e := store.CreateSession(ctx, domain.SessionRecord{ProjectID: "p", Kind: domain.KindWorker, Harness: domain.HarnessCodex, Activity: domain.Activity{State: domain.ActivityActive}, CreatedAt: now, UpdatedAt: now})
	if e != nil {
		t.Fatal(e)
	}
	store.Close()
	db, e := activationrepo.OpenDedicated(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	b := New(db)
	scope := devicebridge.Scope{DeviceID: "dev", OwnerID: "owner"}
	_, e = db.Exec(`INSERT INTO device_bridge_devices VALUES('dev','owner','Mac','public','custody','0.3.0','["session_query"]','paired',?,NULL,NULL,?,?)`, now, now, now)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`UPDATE sessions SET activity_state='blocked' WHERE id=?`, session.ID); e != nil {
		t.Fatal(e)
	}
	events, e := b.UnspooledSessionEvents(ctx, scope)
	if e != nil || len(events) != 1 || events[0].Event != "blocked_on_tool" {
		t.Fatal(events, e)
	}
	unsigned, _ := json.Marshal(map[string]any{"contract_version": "0.3.0", "type": "session_event", "message_id": "01ARZ3NDEKTSV4RRFFQ69G5FAX", "device_id": "dev", "owner_id": "owner", "payload": map[string]any{"session_id": string(session.ID), "project": "p", "event": "blocked_on_tool"}})
	wire, e := devicebridge.SignFrame(unsigned, ed25519.NewKeyFromSeed(make([]byte, 32)), now, [devicebridge.NonceBytes]byte{})
	if e != nil {
		t.Fatal(e)
	}
	if e = b.CommitSessionEvent(ctx, scope, events[0], wire); e != nil {
		t.Fatal(e)
	}
	// Reconnection repeats custody commit without manufacturing a new frame.
	if e = New(db).CommitSessionEvent(ctx, scope, events[0], wire); e != nil {
		t.Fatal(e)
	}
	if depth, e := b.Depth(ctx, scope); e != nil || depth != 1 {
		t.Fatal(depth, e)
	}
	receipt, _ := json.Marshal(map[string]any{"contract_version": "0.3.0", "type": "receipt", "message_id": "01ARZ3NDEKTSV4RRFFQ69G5FAY", "device_id": "dev", "owner_id": "owner", "payload": map[string]any{"event_message_id": "01ARZ3NDEKTSV4RRFFQ69G5FAX", "received_at": now.Unix()}})
	for i := 0; i < 2; i++ {
		if e = b.ApplyReceipt(ctx, scope, receipt, now); e != nil {
			t.Fatal(e)
		}
	}
	if depth, e := b.Depth(ctx, scope); e != nil || depth != 0 {
		t.Fatal(depth, e)
	}
	if _, e = db.Exec(`UPDATE sessions SET activity_state='exited' WHERE id=?`, session.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`UPDATE sessions SET is_terminated=1 WHERE id=?`, session.ID); e != nil {
		t.Fatal(e)
	}
	events, e = b.UnspooledSessionEvents(ctx, scope)
	if e != nil || len(events) != 1 || events[0].Event != "exited" {
		t.Fatal("duplicate projected exit", events, e)
	}
	if e = b.ClearPaired(ctx, scope); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`UPDATE sessions SET is_terminated=0,activity_state='waiting_input' WHERE id=?`, session.ID); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = db.QueryRow(`SELECT count(*) FROM device_bridge_session_events`).Scan(&count); e != nil || count != 2 {
		t.Fatal("revoked device emitted", count, e)
	}
}
