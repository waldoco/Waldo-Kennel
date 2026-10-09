package macaction

import (
	"context"
	"encoding/json"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/devicebridge"
	"testing"
	"time"
)

type memoryLedger struct {
	q       Request
	clock   *time.Time
	started bool
}

func (l *memoryLedger) Pending(_ context.Context, q Request) (Request, error) {
	q.State = "approved"
	l.q = q
	return q, nil
}
func (l *memoryLedger) Load(context.Context, string, string) (Request, error) { return l.q, nil }
func (l *memoryLedger) Start(_ context.Context, q Request, now int64) (bool, error) {
	l.started = now < q.ExpiresAt
	return l.started, nil
}
func (l *memoryLedger) Finish(_ context.Context, q Request, r map[string]any, _ []byte) error {
	l.q = q
	l.q.State = "finished"
	l.q.Result = r
	return nil
}

type registered struct{}

func (registered) Resolve(context.Context, string) (string, string, error) {
	return "project", "/registered", nil
}

type timedRunner struct {
	clock   *time.Time
	calls   int
	timeout int64
}

func (r *timedRunner) Run(_ context.Context, _, _ string, n int64) (Execution, error) {
	r.calls++
	r.timeout = n
	*r.clock = r.clock.Add(DefaultTimeout)
	return Execution{}, nil
}
func macFrame(exp int64) map[string]any {
	return map[string]any{"class": "shell_exec", "command_id": "command", "expires_at": json.Number(time.Unix(exp, 0).Format("2006")), "payload": map[string]any{"project": "project", "command": "true", "reason": "tests", "timeout_seconds": json.Number("120")}}
}
func TestApprovalAtFourteenMinutesGetsFullExecutionBudget(t *testing.T) {
	epoch := time.Unix(2000, 0)
	now := epoch.Add(14*time.Minute + 59*time.Second)
	ledger := &memoryLedger{clock: &now}
	runner := &timedRunner{clock: &now}
	e := Engine{Ledger: ledger, Projects: registered{}, Runner: runner, Now: func() time.Time { return now }}
	f := macFrame(0)
	f["expires_at"] = json.Number("2900")
	got, err := e.Handle(context.Background(), devicebridge.Scope{OwnerID: "owner", DeviceID: "device"}, f)
	if err != nil || got["status"] != "completed" || !ledger.started || runner.calls != 1 || runner.timeout != 120 || now.Before(epoch.Add(15*time.Minute)) {
		t.Fatal(got, err, ledger.started, runner)
	}
}
func TestUnapprovedExpiryDoesNotRun(t *testing.T) {
	now := time.Unix(2900, 0)
	l := &memoryLedger{}
	r := &timedRunner{clock: &now}
	e := Engine{Ledger: l, Projects: registered{}, Runner: r, Now: func() time.Time { return now }}
	f := macFrame(0)
	f["expires_at"] = json.Number("2900")
	got, err := e.Handle(context.Background(), devicebridge.Scope{OwnerID: "owner", DeviceID: "device"}, f)
	if err != nil || got["status"] != "expired" || r.calls != 0 || l.started {
		t.Fatal(got, err, r)
	}
}
func TestRestartAfterStartIsUnknownEvenPastApprovalDeadline(t *testing.T) {
	now := time.Unix(4000, 0)
	l := &memoryLedger{q: Request{ID: "command", DeviceID: "device", State: "started", ExpiresAt: 2900}}
	r := &timedRunner{clock: &now}
	e := Engine{Ledger: l, Projects: registered{}, Runner: r, Now: func() time.Time { return now }}
	got, err := e.Reconcile(context.Background(), devicebridge.Scope{OwnerID: "owner", DeviceID: "device"}, macFrame(0))
	if err != nil || got["status"] != "outcome_unknown" || r.calls != 0 {
		t.Fatal(got, err, r)
	}
}
