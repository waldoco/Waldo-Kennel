package controllers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/config"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apierr"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/intelligence/intelligencetest"
	outcomevc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
)

type fakeAttemptManager struct {
	start   func(context.Context, domain.OutcomeID, outcomevc.StartAttemptInput) (outcomevc.AttemptView, error)
	get     func(context.Context, domain.OutcomeID, domain.AttemptID) (outcomevc.AttemptView, error)
	list    func(context.Context, domain.OutcomeID) ([]outcomevc.AttemptView, error)
	cancel  func(context.Context, domain.OutcomeID, domain.AttemptID) (outcomevc.AttemptView, error)
	observe func(context.Context, domain.OutcomeID, domain.AttemptID, outcomevc.RecordObservationInput) (domain.AttemptObservation, error)
	recover func(context.Context, domain.OutcomeID, domain.AttemptID, outcomevc.RecoveryInput) (outcomevc.RecoveryView, error)
}

func (f *fakeAttemptManager) StartAttempt(ctx context.Context, id domain.OutcomeID, in outcomevc.StartAttemptInput) (outcomevc.AttemptView, error) {
	return f.start(ctx, id, in)
}
func (f *fakeAttemptManager) GetAttempt(ctx context.Context, o domain.OutcomeID, a domain.AttemptID) (outcomevc.AttemptView, error) {
	return f.get(ctx, o, a)
}
func (f *fakeAttemptManager) ListAttempts(ctx context.Context, o domain.OutcomeID) ([]outcomevc.AttemptView, error) {
	return f.list(ctx, o)
}
func (f *fakeAttemptManager) CancelAttempt(ctx context.Context, o domain.OutcomeID, a domain.AttemptID) (outcomevc.AttemptView, error) {
	return f.cancel(ctx, o, a)
}
func (f *fakeAttemptManager) RecordObservation(ctx context.Context, o domain.OutcomeID, a domain.AttemptID, in outcomevc.RecordObservationInput) (domain.AttemptObservation, error) {
	return f.observe(ctx, o, a, in)
}
func (f *fakeAttemptManager) RecoverAttempt(ctx context.Context, o domain.OutcomeID, a domain.AttemptID, in outcomevc.RecoveryInput) (outcomevc.RecoveryView, error) {
	return f.recover(ctx, o, a, in)
}

func sampleAttemptView(status domain.AttemptStatus) outcomevc.AttemptView {
	now := time.Date(2026, 8, 24, 9, 30, 0, 0, time.UTC)
	attempt := domain.Attempt{
		ID: "att-1", OutcomeID: "out-fix", PlanRevisionID: "plan-1", WorkUnitID: "wu-1",
		Number: 1, Status: status, ContractRevisionNumber: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	return outcomevc.AttemptView{
		Outcome: domain.Outcome{ID: "out-fix", SpaceID: "rsp-1", Title: "Ledger", CurrentRevisionNumber: 1},
		Attempt: attempt,
		Sessions: []domain.AttemptSessionRef{{
			ID: "asr-1", AttemptID: attempt.ID, Seq: 1, SessionID: "provider-x",
			Harness: domain.HarnessCodex, Mode: domain.SessionModeTUI,
			RunBriefCoreDigest: strings.Repeat("ab", 32), BoundAt: now,
		}},
		Observations: []domain.AttemptObservation{{
			ID: "obs-1", AttemptID: attempt.ID, Seq: 1,
			Kind: domain.ObservationProviderExit, Payload: `{}`, CreatedAt: now,
		}},
		Fence: &domain.AttemptFence{ID: "fence-1", Subject: "project:mer", AttemptID: attempt.ID, IssuedAt: now},
		Presentation: domain.AttemptPresentation{
			Phase:             domain.AttemptPhaseEndedUnclassified,
			EndedUnclassified: true,
			NextAction:        "classify through Verification",
		},
	}
}

func newAttemptsTestServer(t *testing.T, mgr *fakeAttemptManager) *httptest.Server {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return httptest.NewServer(httpd.NewRouterWithControl(config.Config{}, log, nil, httpd.APIDeps{
		Outcomes: &fakeOutcomeService{},
		Attempts: mgr,
	}, httpd.ControlDeps{}))
}

// TestAttemptRoutesUnwiredAnswer501 pins the optional-surface contract: until
// execution is wired the routes exist in the spec but answer NotImplemented.
func TestAttemptRoutesUnwiredAnswer501(t *testing.T) {
	srv := newOutcomesTestServer(t, &fakeOutcomeService{})
	body := `{"planRevisionId":"plan-1","requestKey":"rk"}`
	if _, status, _ := doRequest(t, srv, http.MethodPost, "/api/v1/outcomes/out-fix/attempts", body); status != http.StatusNotImplemented {
		t.Fatalf("start = %d, want 501", status)
	}
	if _, status, _ := doRequest(t, srv, http.MethodGet, "/api/v1/outcomes/out-fix/attempts/att-1", ""); status != http.StatusNotImplemented {
		t.Fatalf("get = %d, want 501", status)
	}
	// pause/resume are unrouted entirely until provider control exists.
	for _, gone := range []string{"pause", "resume"} {
		if _, st, _ := doRequest(t, srv, http.MethodPost,
			"/api/v1/outcomes/out-fix/attempts/att-1/"+gone, ""); st == http.StatusNotImplemented {
			t.Fatalf("%s must not be a wired-or-reserved route", gone)
		}
	}
}

// TestAttemptRouteSurface exercises every attempt route through the router
// with a manager double: status codes, envelope shapes, and error pass-through.
func TestAttemptRouteSurface(t *testing.T) {
	mgr := &fakeAttemptManager{}
	mgr.start = func(_ context.Context, _ domain.OutcomeID, in outcomevc.StartAttemptInput) (outcomevc.AttemptView, error) {
		if in.PlanRevisionID == "" || in.RequestKey == "" {
			return outcomevc.AttemptView{}, apierr.Invalid("REQUEST_KEY_REQUIRED", "Provide an idempotency key", nil)
		}
		if in.PlanRevisionID == "plan-stale" {
			return outcomevc.AttemptView{}, apierr.Conflict(outcomevc.CodePlanBriefInvalidated, "brief invalidated", nil)
		}
		return sampleAttemptView(domain.AttemptRunning), nil
	}
	mgr.get = func(_ context.Context, _ domain.OutcomeID, id domain.AttemptID) (outcomevc.AttemptView, error) {
		if id == "att-missing" {
			return outcomevc.AttemptView{}, apierr.NotFound(outcomevc.CodeAttemptNotFound, "no such attempt")
		}
		return sampleAttemptView(domain.AttemptReconciled), nil
	}
	mgr.list = func(_ context.Context, _ domain.OutcomeID) ([]outcomevc.AttemptView, error) {
		return []outcomevc.AttemptView{sampleAttemptView(domain.AttemptRunning)}, nil
	}
	mgr.cancel = func(context.Context, domain.OutcomeID, domain.AttemptID) (outcomevc.AttemptView, error) {
		return sampleAttemptView(domain.AttemptCancelled), nil
	}
	mgr.observe = func(_ context.Context, _ domain.OutcomeID, _ domain.AttemptID, in outcomevc.RecordObservationInput) (domain.AttemptObservation, error) {
		if in.Kind == "" {
			return domain.AttemptObservation{}, apierr.Invalid("OBSERVATION_KIND_REQUIRED", "Name what was observed", nil)
		}
		return domain.AttemptObservation{ID: "obs-new", AttemptID: "att-1", Seq: 2, Kind: in.Kind}, nil
	}
	mgr.recover = func(_ context.Context, _ domain.OutcomeID, _ domain.AttemptID, in outcomevc.RecoveryInput) (outcomevc.RecoveryView, error) {
		if !in.Action.Valid() {
			return outcomevc.RecoveryView{}, apierr.Invalid("RECOVERY_ACTION_INVALID", "Unknown action", nil)
		}
		receipt := domain.AttemptRecoveryReceipt{
			ID: "rcpt-1", AttemptID: "att-1", Resolution: domain.RecoveryReplacement,
		}
		return outcomevc.RecoveryView{Attempt: sampleAttemptView(domain.AttemptLost), Receipt: &receipt}, nil
	}
	srv := newAttemptsTestServer(t, mgr)

	resp, status, _ := doRequest(t, srv, http.MethodPost, "/api/v1/outcomes/out-fix/attempts",
		`{"planRevisionId":"plan-1","requestKey":"rk-1"}`)
	if status != http.StatusCreated {
		t.Fatalf("start = %d: %s", status, resp)
	}
	for _, want := range []string{
		`"status":"running"`, `"number":1`, `"presentation":`,
		`"endedUnclassified":true`, `"sessionId":"provider-x"`, `"fence":`,
	} {
		if !strings.Contains(string(resp), want) {
			t.Fatalf("start body missing %s: %s", want, resp)
		}
	}

	if _, status, _ = doRequest(t, srv, http.MethodPost, "/api/v1/outcomes/out-fix/attempts",
		`{"planRevisionId":"plan-stale","requestKey":"rk-2"}`); status != http.StatusConflict {
		t.Fatalf("invalidated brief = %d, want 409", status)
	}

	listBytes, listStatus, _ := doRequest(t, srv, http.MethodGet, "/api/v1/outcomes/out-fix/attempts", "")
	if listStatus != http.StatusOK || !strings.Contains(string(listBytes), `"attempts":[`) {
		t.Fatalf("list = %d: %s", listStatus, listBytes)
	}

	getBytes, getStatus, _ := doRequest(t, srv, http.MethodGet, "/api/v1/outcomes/out-fix/attempts/att-1", "")
	if getStatus != http.StatusOK || !strings.Contains(string(getBytes), `"phase":"ended_unclassified"`) {
		t.Fatalf("get = %d: %s", getStatus, getBytes)
	}
	if _, missingStatus, _ := doRequest(t, srv, http.MethodGet, "/api/v1/outcomes/out-fix/attempts/att-missing", ""); missingStatus != http.StatusNotFound {
		t.Fatalf("missing attempt = %d, want 404", missingStatus)
	}

	obsBytes, obsStatus, _ := doRequest(t, srv, http.MethodPost, "/api/v1/outcomes/out-fix/attempts/att-1/observations",
		`{"kind":"contained","payload":"{\\\"reason\\\":\\\"probe\\\"}"}`)
	if obsStatus != http.StatusCreated || !strings.Contains(string(obsBytes), `"seq":2`) {
		t.Fatalf("observation = %d: %s", obsStatus, obsBytes)
	}

	actionBytes, actionStatus, _ := doRequest(t, srv, http.MethodPost,
		"/api/v1/outcomes/out-fix/attempts/att-1/cancel", "")
	if actionStatus != http.StatusOK || !strings.Contains(string(actionBytes), `"attempt":`) {
		t.Fatalf("cancel = %d: %s", actionStatus, actionBytes)
	}
	// Pause/resume are deliberately unrouted until provider control exists.
	for _, gone := range []string{"pause", "resume"} {
		if _, st, _ := doRequest(t, srv, http.MethodPost,
			"/api/v1/outcomes/out-fix/attempts/att-1/"+gone, ""); st == http.StatusOK {
			t.Fatalf("%s route must not exist", gone)
		}
	}

	recBytes, recStatus, _ := doRequest(t, srv, http.MethodPost, "/api/v1/outcomes/out-fix/attempts/att-1/recovery",
		`{"action":"replace"}`)
	if recStatus != http.StatusOK ||
		!strings.Contains(string(recBytes), `"resolution":"replacement_attempt"`) ||
		!strings.Contains(string(recBytes), `"status":"lost"`) {
		t.Fatalf("recovery = %d: %s", recStatus, recBytes)
	}
}

// controllerSpawner is the execution double for the functional HTTP test.
type controllerSpawner struct{}

func (controllerSpawner) ProfileReadiness(context.Context, domain.ProjectID, domain.ExecutionBinding, *domain.AttemptExecutionPolicy) (ports.AgentProfileReadiness, error) {
	return ports.AgentProfileReadiness{Ready: true, Detail: "test profile ok"}, nil
}
func (controllerSpawner) Terminate(_ context.Context, _ domain.ProjectID, _ string) (ports.TerminationResult, error) {
	return ports.TerminationResult{ProviderStopped: true, WorkspaceFreed: true}, nil
}

func (controllerSpawner) Spawn(_ context.Context, req ports.AttemptSpawnRequest) (ports.AttemptSpawnResult, error) {
	rec := domain.SessionRecord{
		ID:       domain.SessionID("sess-func-" + req.Harness),
		Mode:     domain.SessionModeTUI,
		Harness:  req.Harness,
		Metadata: domain.SessionMetadata{WorkspacePath: "/tmp/kennel-controller-attempt-workspace"},
		Activity: domain.Activity{
			State:          domain.ActivityActive,
			LastActivityAt: time.Now(),
		},
	}
	bound, err := req.ExecutionPolicy.BindWorkspaceRoot(rec.Metadata.WorkspacePath)
	if err != nil {
		return ports.AttemptSpawnResult{}, err
	}
	if req.BeforeProviderLaunch != nil {
		if err := req.BeforeProviderLaunch(context.Background(), rec, bound, nil); err != nil {
			return ports.AttemptSpawnResult{}, err
		}
	}
	return ports.AttemptSpawnResult{Session: domain.Session{SessionRecord: rec}, ExecutionPolicy: &bound}, nil
}

// TestAttemptRoutesFunctionalThroughRealStore is the end-to-end proof for
// #31's happy path over HTTP: create → plan → approve → start → running with
// bound session ref, open fence custody, and derived unconfirmed presentation
// (the fake session has no heartbeat facts yet).
func TestAttemptRoutesFunctionalThroughRealStore(t *testing.T) {
	storeHandle := sqlitetest.MustOpen(t)
	ctx := context.Background()
	if err := storeHandle.UpsertProject(ctx, domain.ProjectRecord{
		ID: "mer", Path: "/tmp/kennel-attempt-fixture", RegisteredAt: time.Now().UTC(),
		Kind: domain.ProjectKindSingleRepo,
	}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	spawner := controllerSpawner{}
	svc := outcomevc.New(storeHandle, nil).WithPlanning(intelligencetest.New(), controllerRouting{})
	svc = svc.WithExecution(spawner, storeHandle)
	svc.AdmissionPolicy = controllerAdmissionPolicy(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(httpd.NewRouterWithControl(config.Config{}, log, nil, httpd.APIDeps{
		Outcomes: svc,
		Attempts: svc,
	}, httpd.ControlDeps{}))
	defer srv.Close()

	// Build the approved lineage exactly like the Understand surfaces do.
	respBytes, status, _ := doRequest(t, srv, http.MethodPost, "/api/v1/projects/mer/outcomes",
		`{"title":"Local Focus Ledger","goal":"Record focus locally.","successCriteria":["Blocks are recorded."],"review":"Deterministic checks.","authorityCeiling":{"readWorkspace":true,"writeWorkspace":true,"executeLocal":true},"requestKey":"req-att-e2e"}`)
	if status != http.StatusCreated {
		t.Fatalf("create = %d: %s", status, respBytes)
	}
	id := extractOutcomeID(t, respBytes)
	planBytes, pStatus, _ := doRequest(t, srv, http.MethodPost, "/api/v1/outcomes/"+id+"/plans", `{"expectedContractRevision":1}`)
	if pStatus != http.StatusCreated {
		t.Fatalf("propose = %d: %s", pStatus, planBytes)
	}
	var planEnvelope struct {
		Plan struct {
			ID        string `json:"id"`
			WorkUnits []struct {
				ID string `json:"id"`
			} `json:"workUnits"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(planBytes, &planEnvelope); err != nil {
		t.Fatal(err)
	}
	if len(planEnvelope.Plan.WorkUnits) == 0 {
		t.Fatalf("plan carried no work units: %s", planBytes)
	}
	if _, aStatus, _ := doRequest(t, srv, http.MethodPost,
		"/api/v1/outcomes/"+id+"/plans/"+planEnvelope.Plan.ID+"/approval", `{"expectedContractRevision":1}`); aStatus != http.StatusOK {
		t.Fatalf("approve = %d", aStatus)
	}

	// Start the attempt.
	startBytes, sStatus, _ := doRequest(t, srv, http.MethodPost, "/api/v1/outcomes/"+id+"/attempts",
		fmt.Sprintf(`{"planRevisionId":%q,"workUnitId":%q,"requestKey":"rk-start-e2e"}`,
			planEnvelope.Plan.ID, planEnvelope.Plan.WorkUnits[0].ID))
	if sStatus != http.StatusCreated {
		t.Fatalf("start = %d: %s", sStatus, startBytes)
	}
	var attemptEnvelope struct {
		Attempt struct {
			ID           string `json:"id"`
			Status       string `json:"status"`
			Number       int64  `json:"number"`
			Presentation struct {
				Phase       string `json:"phase"`
				Unconfirmed bool   `json:"unconfirmed"`
			} `json:"presentation"`
			Sessions []struct {
				SessionID string `json:"sessionId"`
			} `json:"sessions"`
		} `json:"attempt"`
	}
	if err := json.Unmarshal(startBytes, &attemptEnvelope); err != nil {
		t.Fatalf("decode start: %v", err)
	}
	if attemptEnvelope.Attempt.Status != "running" || attemptEnvelope.Attempt.Number != 1 {
		t.Fatalf("attempt = %s #%d, want running #1: %s", attemptEnvelope.Attempt.Status, attemptEnvelope.Attempt.Number, startBytes)
	}
	if len(attemptEnvelope.Attempt.Sessions) != 1 {
		t.Fatalf("session refs = %+v, want one binding", attemptEnvelope.Attempt.Sessions)
	}
	// No session row exists for the fake ref: derived presentation MUST be
	// unconfirmed, never dead.
	if !attemptEnvelope.Attempt.Presentation.Unconfirmed ||
		attemptEnvelope.Attempt.Presentation.Phase != "unconfirmed" {
		t.Fatalf("presentation = %+v, want unconfirmed", attemptEnvelope.Attempt.Presentation)
	}

	// Replay resolves the same attempt without a second row.
	replayBytes, replayStatus, _ := doRequest(t, srv, http.MethodPost, "/api/v1/outcomes/"+id+"/attempts",
		fmt.Sprintf(`{"planRevisionId":%q,"workUnitId":%q,"requestKey":"rk-start-e2e"}`,
			planEnvelope.Plan.ID, planEnvelope.Plan.WorkUnits[0].ID))
	if replayStatus != http.StatusCreated || !strings.Contains(string(replayBytes), attemptEnvelope.Attempt.ID) {
		t.Fatalf("replay = %d: %s", replayStatus, replayBytes)
	}

	// Custody is exclusive while the first attempt holds the fence. With a
	// WorkUnit graph the refusal is more precise than the old project-wide
	// fence: the plan's only unit is already running, so nothing is runnable.
	heldBytes, heldStatus, _ := doRequest(t, srv, http.MethodPost, "/api/v1/outcomes/"+id+"/attempts",
		fmt.Sprintf(`{"planRevisionId":%q,"workUnitId":%q,"requestKey":"rk-second"}`,
			planEnvelope.Plan.ID, planEnvelope.Plan.WorkUnits[0].ID))
	if heldStatus != http.StatusConflict ||
		(!strings.Contains(string(heldBytes), "ATTEMPT_FENCE_HELD") &&
			!strings.Contains(string(heldBytes), "NO_RUNNABLE_WORK_UNIT")) {
		t.Fatalf("second admission = %d want 409 fence-held or no-runnable-unit: %s", heldStatus, heldBytes)
	}

	// Observations append ordered history.
	obsBytes, obsStatus, _ := doRequest(t, srv, http.MethodPost,
		"/api/v1/outcomes/"+id+"/attempts/"+attemptEnvelope.Attempt.ID+"/observations",
		`{"kind":"note"}`)
	if obsStatus != http.StatusCreated || !strings.Contains(string(obsBytes), `"seq":1`) {
		t.Fatalf("observation = %d: %s", obsStatus, obsBytes)
	}

	// Recovery reconcile cannot prove liveness for the never-signalled fake:
	// lost verdict + replacement receipt, then a replacement may start.
	unprovenBytes, unprovenStatus, _ := doRequest(t, srv, http.MethodPost,
		"/api/v1/outcomes/"+id+"/attempts/"+attemptEnvelope.Attempt.ID+"/recovery", `{"action":"replace"}`)
	recBytes, recStatus, _ := doRequest(t, srv, http.MethodPost,
		"/api/v1/outcomes/"+id+"/attempts/"+attemptEnvelope.Attempt.ID+"/recovery",
		`{"action":"replace","confirmProviderStopped":true}`)
	if unprovenStatus != http.StatusConflict || !strings.Contains(string(unprovenBytes), "ATTEMPT_CUSTODY_UNPROVEN") {
		t.Fatalf("unproven replace = %d want 409: %s", unprovenStatus, unprovenBytes)
	}
	if recStatus != http.StatusOK || !strings.Contains(string(recBytes), `"status":"lost"`) {
		t.Fatalf("recover = %d: %s", recStatus, recBytes)
	}
	repBytes, repStatus, _ := doRequest(t, srv, http.MethodPost, "/api/v1/outcomes/"+id+"/attempts",
		fmt.Sprintf(`{"planRevisionId":%q,"workUnitId":%q,"requestKey":"rk-replacement"}`,
			planEnvelope.Plan.ID, planEnvelope.Plan.WorkUnits[0].ID))
	if repStatus != http.StatusCreated || !strings.Contains(string(repBytes), `"number":2`) {
		t.Fatalf("replacement = %d: %s", repStatus, repBytes)
	}

	// The public resume verb is GONE: a label-only paused->running flip lies
	// about provider control, so the route must answer 400, never mutate.
	resumeBytes, resumeStatus, _ := doRequest(t, srv,
		http.MethodPost,
		"/api/v1/outcomes/"+id+"/attempts/"+attemptEnvelope.Attempt.ID+"/recovery", `{"action":"resume"}`)
	if resumeStatus != http.StatusBadRequest || !strings.Contains(string(resumeBytes), "RECOVERY_ACTION_INVALID") {
		t.Fatalf("removed resume verb = %d want 400 RECOVERY_ACTION_INVALID: %s", resumeStatus, resumeBytes)
	}
}
