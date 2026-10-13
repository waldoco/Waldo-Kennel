package macactionrepo

import (
	"context"
	"database/sql"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/macaction"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/activationrepo"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Repository, *sql.DB, macaction.Request) {
	t.Helper()
	dir := t.TempDir()
	s, e := sqlite.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.UpsertProject(context.Background(), domain.ProjectRecord{ID: "project", Path: dir, DisplayName: "Project", RegisteredAt: time.Now().UTC()}); e != nil {
		t.Fatal(e)
	}
	s.Close()
	db, e := activationrepo.OpenDedicated(dir)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	_, e = db.Exec(`INSERT INTO device_bridge_devices(device_id,owner_id,label,public_key,key_custody_ref,contract_version,capability_classes,state,paired_at,created_at,updated_at) VALUES('device','owner','Test Mac','test-public-key','test-custody','0.3.0','["shell_exec"]','paired',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	if e != nil {
		t.Fatal(e)
	}
	q := macaction.Request{ID: "command", DeviceID: "device", OwnerID: "owner", Fingerprint: strings.Repeat("a", 64), Kind: "approval", Class: "shell_exec", Project: "project", Folder: dir, Command: "true", Reason: "tests", Risks: []string{}, ExpiresAt: 2900, State: "pending"}
	return New(db), db, q
}
func TestApprovalAndStartDeadlineIsIndependentOfFinish(t *testing.T) {
	r, _, q := fixture(t)
	ctx := context.Background()
	if _, e := r.Pending(ctx, q); e != nil {
		t.Fatal(e)
	}
	if e := r.Decide(ctx, q.DeviceID, q.ID, q.Fingerprint, true, time.Unix(2899, 0)); e != nil {
		t.Fatal(e)
	}
	started, e := r.Start(ctx, q, 2899)
	if e != nil || !started {
		t.Fatal(started, e)
	}
	// Finish is legal after the 15 minute approval/start deadline.
	result := map[string]any{"status": "completed", "exit_code": 0, "output_tail": "done", "truncated": false}
	if e = r.Finish(ctx, q, result, []byte("full local output")); e != nil {
		t.Fatal(e)
	}
	got, e := r.Load(ctx, q.DeviceID, q.ID)
	if e != nil || got.Result["status"] != "completed" {
		t.Fatal(got, e)
	}
	out, e := r.Output(ctx, q.DeviceID, q.ID)
	if e != nil || string(out) != "full local output" {
		t.Fatal(string(out), e)
	}
	history, e := r.History(ctx)
	if e != nil || len(history) != 1 || history[0].State != "finished" {
		t.Fatal(history, e)
	}
	if started, e = r.Start(ctx, q, 3000); e != nil || started {
		t.Fatal("ran twice", started, e)
	}
}
func TestLateApprovalAndDenialCannotStart(t *testing.T) {
	for _, mode := range []string{"late", "deny", "changed"} {
		t.Run(mode, func(t *testing.T) {
			r, _, q := fixture(t)
			ctx := context.Background()
			r.Pending(ctx, q)
			now := time.Unix(2899, 0)
			approve := mode != "deny"
			fp := q.Fingerprint
			if mode == "late" {
				now = time.Unix(2900, 0)
			}
			if mode == "changed" {
				fp = strings.Repeat("b", 64)
			}
			e := r.Decide(ctx, q.DeviceID, q.ID, fp, approve, now)
			if mode != "deny" && e == nil {
				t.Fatal("late/mismatched approved")
			}
			if mode == "deny" && e != nil {
				t.Fatal(e)
			}
			if started, e := r.Start(ctx, q, now.Unix()); e != nil || started {
				t.Fatal("effect started", started, e)
			}
		})
	}
}
func TestStartedMarkerSurvivesRepositoryRestart(t *testing.T) {
	r, db, q := fixture(t)
	ctx := context.Background()
	r.Pending(ctx, q)
	r.Decide(ctx, q.DeviceID, q.ID, q.Fingerprint, true, time.Unix(2899, 0))
	if started, e := r.Start(ctx, q, 2899); e != nil || !started {
		t.Fatal(e)
	}
	got, e := New(db).Load(ctx, q.DeviceID, q.ID)
	if e != nil || got.State != "started" || got.Result != nil {
		t.Fatal(got, e)
	}
}
