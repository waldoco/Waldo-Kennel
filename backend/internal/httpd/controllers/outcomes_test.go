package controllers_test

import (
	"context"
	"encoding/json"
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
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/service/intelligence/intelligencetest"
	outcomevc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
)

type fakeOutcomeService struct {
	create func(context.Context, outcomevc.CreateInput) (outcomevc.View, error)
	revise func(context.Context, domain.OutcomeID, outcomevc.ReviseContractInput) (outcomevc.View, error)
	get    func(context.Context, domain.OutcomeID) (outcomevc.View, error)
	list   func(context.Context, domain.ProjectID) ([]outcomevc.View, error)

	proposePlan func(context.Context, domain.OutcomeID, int64) (outcomevc.PlanView, error)

	contribute  func(context.Context, domain.OutcomeID, outcomevc.CreateContributionInput) (outcomevc.View, error)
	composition func(context.Context, domain.OutcomeID) (outcomevc.CompositionView, error)

	proposeDecomposition   func(context.Context, domain.OutcomeID, outcomevc.ProposeDecompositionInput) (outcomevc.DecompositionView, error)
	authorizeDecomposition func(context.Context, domain.OutcomeID, domain.DecompositionRevisionID) (outcomevc.DecompositionView, error)
	latestDecomposition    func(context.Context, domain.OutcomeID) (outcomevc.DecompositionView, error)

	waiveDependency func(context.Context, domain.OutcomeID, outcomevc.WaiveDependencyInput) (outcomevc.DecompositionView, error)

	askForDecomposition        func(domain.OutcomeID, int64) (outcomevc.DecompositionRequestView, error)
	submitAgentProposal        func(domain.DecompositionRequestID, string, outcomevc.ProposeDecompositionInput, string) (outcomevc.DecompositionRequestView, error)
	latestDecompositionRequest func(domain.OutcomeID) (outcomevc.DecompositionRequestView, error)

	lastAskOf       domain.OutcomeID
	lastAskRevision int64
	lastSubmitID    domain.DecompositionRequestID
	lastSubmitToken string
	lastSubmitInput outcomevc.ProposeDecompositionInput
	lastSubmitRaw   string

	lastWaiverOf domain.OutcomeID
	lastWaiver   outcomevc.WaiveDependencyInput

	lastDecompositionOf    domain.OutcomeID
	lastDecompositionInput outcomevc.ProposeDecompositionInput

	lastInput            outcomevc.CreateInput
	lastContributionOf   domain.OutcomeID
	lastContributionData outcomevc.CreateContributionInput
}

func (f *fakeOutcomeService) CreateContribution(ctx context.Context, parentID domain.OutcomeID, in outcomevc.CreateContributionInput) (outcomevc.View, error) {
	f.lastContributionOf, f.lastContributionData = parentID, in
	if f.contribute == nil {
		return outcomevc.View{}, apierr.NotFound("OUTCOME_NOT_FOUND", "not implemented in fake")
	}
	return f.contribute(ctx, parentID, in)
}

func (f *fakeOutcomeService) ProposeDecomposition(ctx context.Context, parentID domain.OutcomeID, in outcomevc.ProposeDecompositionInput) (outcomevc.DecompositionView, error) {
	f.lastDecompositionOf, f.lastDecompositionInput = parentID, in
	if f.proposeDecomposition == nil {
		return outcomevc.DecompositionView{}, apierr.NotFound("OUTCOME_NOT_FOUND", "not implemented in fake")
	}
	return f.proposeDecomposition(ctx, parentID, in)
}

func (f *fakeOutcomeService) AuthorizeDecomposition(ctx context.Context, parentID domain.OutcomeID, decompositionID domain.DecompositionRevisionID) (outcomevc.DecompositionView, error) {
	if f.authorizeDecomposition == nil {
		return outcomevc.DecompositionView{}, apierr.NotFound("DECOMPOSITION_NOT_FOUND", "not implemented in fake")
	}
	return f.authorizeDecomposition(ctx, parentID, decompositionID)
}

func (f *fakeOutcomeService) LatestDecomposition(ctx context.Context, parentID domain.OutcomeID) (outcomevc.DecompositionView, error) {
	if f.latestDecomposition == nil {
		return outcomevc.DecompositionView{}, apierr.NotFound("DECOMPOSITION_NOT_FOUND", "not implemented in fake")
	}
	return f.latestDecomposition(ctx, parentID)
}

func (f *fakeOutcomeService) WaiveContributionDependency(ctx context.Context, parentID domain.OutcomeID, in outcomevc.WaiveDependencyInput) (outcomevc.DecompositionView, error) {
	f.lastWaiverOf, f.lastWaiver = parentID, in
	if f.waiveDependency == nil {
		return outcomevc.DecompositionView{}, apierr.NotFound("DECOMPOSITION_NOT_FOUND", "not implemented in fake")
	}
	return f.waiveDependency(ctx, parentID, in)
}

func (f *fakeOutcomeService) Composition(ctx context.Context, id domain.OutcomeID) (outcomevc.CompositionView, error) {
	if f.composition == nil {
		return outcomevc.CompositionView{}, apierr.NotFound("OUTCOME_NOT_FOUND", "not implemented in fake")
	}
	return f.composition(ctx, id)
}

func (f *fakeOutcomeService) ProposePlan(ctx context.Context, id domain.OutcomeID, expected int64) (outcomevc.PlanView, error) {
	if f.proposePlan == nil {
		return outcomevc.PlanView{}, apierr.NotFound("PLAN_NOT_FOUND", "not implemented in fake")
	}
	return f.proposePlan(ctx, id, expected)
}

func (f *fakeOutcomeService) ApprovePlan(_ context.Context, _ domain.OutcomeID, _ outcomevc.ApprovePlanInput) (outcomevc.AuthorizedPlanView, error) {
	return outcomevc.AuthorizedPlanView{}, apierr.NotFound("PLAN_NOT_FOUND", "not implemented in fake")
}

func (f *fakeOutcomeService) GetLatestPlan(_ context.Context, _ domain.OutcomeID) (outcomevc.PlanView, error) {
	return outcomevc.PlanView{}, apierr.NotFound("PLAN_NOT_FOUND", "not implemented in fake")
}

func (f *fakeOutcomeService) Create(ctx context.Context, in outcomevc.CreateInput) (outcomevc.View, error) {
	f.lastInput = in
	return f.create(ctx, in)
}

func (f *fakeOutcomeService) ReviseContract(ctx context.Context, id domain.OutcomeID, in outcomevc.ReviseContractInput) (outcomevc.View, error) {
	return f.revise(ctx, id, in)
}

func (f *fakeOutcomeService) Get(ctx context.Context, id domain.OutcomeID) (outcomevc.View, error) {
	return f.get(ctx, id)
}

func (f *fakeOutcomeService) ListByProject(ctx context.Context, projectID domain.ProjectID) ([]outcomevc.View, error) {
	return f.list(ctx, projectID)
}

func sampleOutcomeView() outcomevc.View {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	rev := domain.ContractRevision{
		ID: "cr-1", OutcomeID: "out-fix", Number: 1,
		Goal: "g", SuccessCriteria: []string{"c1"}, Review: "checks", CreatedAt: now,
	}
	return outcomevc.View{
		Outcome: domain.Outcome{
			ID: "out-fix", SpaceID: "rsp-1", Title: "Ledger",
			CurrentRevisionNumber: 1, CreatedAt: now, UpdatedAt: now,
		},
		Current: rev,
		History: []domain.ContractRevision{rev},
	}
}

func newOutcomesTestServer(t *testing.T, svc *fakeOutcomeService) *httptest.Server {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return httptest.NewServer(httpd.NewRouterWithControl(config.Config{}, log, nil, httpd.APIDeps{
		Outcomes: svc,
	}, httpd.ControlDeps{}))
}

func TestCreateOutcomeRoute(t *testing.T) {
	svc := &fakeOutcomeService{}
	svc.create = func(_ context.Context, in outcomevc.CreateInput) (outcomevc.View, error) {
		if in.ProjectID != "mer" || in.RequestKey == "" || len(in.SuccessCriteria) == 0 {
			return outcomevc.View{}, apierr.Invalid("OUTCOME_CRITERIA_REQUIRED", "Name at least one success criterion", nil)
		}
		return sampleOutcomeView(), nil
	}
	srv := newOutcomesTestServer(t, svc)

	respBytes, status, _ := doRequest(t, srv, http.MethodPost, "/api/v1/projects/mer/outcomes",
		`{"title":"Ledger","goal":"g","successCriteria":["c1"],"review":"r","requestKey":"rk-1"}`)
	if status != http.StatusCreated {
		t.Fatalf("create = %d, want 201: %s", status, respBytes)
	}
	for _, want := range []string{`"spaceId":"rsp-1"`, `"currentRevisionNumber":1`, `"currentRevision":`, `"history":[`, `"number":1`} {
		if !strings.Contains(string(respBytes), want) {
			t.Fatalf("create body missing %s: %s", want, respBytes)
		}
	}
	if svc.lastInput.ProjectID != "mer" {
		t.Fatalf("project id not propagated: %+v", svc.lastInput)
	}
}

func TestListProjectOutcomesRoute(t *testing.T) {
	svc := &fakeOutcomeService{}
	svc.list = func(_ context.Context, projectID domain.ProjectID) ([]outcomevc.View, error) {
		if projectID != "mer" {
			t.Fatalf("project id = %s, want mer", projectID)
		}
		first := sampleOutcomeView()
		first.LatestPlan = &domain.PlanRevision{
			ID: "plan-1", OutcomeID: first.Outcome.ID, Number: 1,
			ContractRevisionNumber: 1, Status: domain.PlanStatusProposed,
		}
		second := sampleOutcomeView()
		second.Outcome.ID = "out-second"
		second.Outcome.Title = "Second Outcome"
		second.Current.ID = "cr-second"
		second.Current.OutcomeID = second.Outcome.ID
		second.History = []domain.ContractRevision{second.Current}
		return []outcomevc.View{first, second}, nil
	}
	srv := newOutcomesTestServer(t, svc)

	respBytes, status, _ := doRequest(t, srv, http.MethodGet, "/api/v1/projects/mer/outcomes", "")
	if status != http.StatusOK {
		t.Fatalf("list = %d, want 200: %s", status, respBytes)
	}
	var body struct {
		Outcomes []struct {
			ID                    string `json:"id"`
			Title                 string `json:"title"`
			CurrentRevisionNumber int64  `json:"currentRevisionNumber"`
			LatestPlan            *struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"latestPlan"`
		} `json:"outcomes"`
	}
	if err := json.Unmarshal(respBytes, &body); err != nil {
		t.Fatalf("decode list: %v: %s", err, respBytes)
	}
	if len(body.Outcomes) != 2 || body.Outcomes[0].ID != "out-fix" || body.Outcomes[1].ID != "out-second" {
		t.Fatalf("outcomes = %+v", body.Outcomes)
	}
	if body.Outcomes[0].CurrentRevisionNumber != 1 {
		t.Fatalf("list lost current revision: %+v", body.Outcomes[0])
	}
	if body.Outcomes[0].LatestPlan == nil || body.Outcomes[0].LatestPlan.ID != "plan-1" || body.Outcomes[0].LatestPlan.Status != "proposed" {
		t.Fatalf("list lost latest plan facts: %+v", body.Outcomes[0])
	}
}

func TestListProjectOutcomesRoutePreservesErrorEnvelope(t *testing.T) {
	svc := &fakeOutcomeService{}
	svc.list = func(_ context.Context, _ domain.ProjectID) ([]outcomevc.View, error) {
		return nil, apierr.Invalid("PROJECT_REQUIRED", "Choose a project", nil)
	}
	srv := newOutcomesTestServer(t, svc)

	respBytes, status, _ := doRequest(t, srv, http.MethodGet, "/api/v1/projects/%20/outcomes", "")
	if status != http.StatusBadRequest {
		t.Fatalf("list = %d, want 400: %s", status, respBytes)
	}
	var body struct {
		Code      string `json:"code"`
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(respBytes, &body); err != nil {
		t.Fatalf("decode error envelope: %v: %s", err, respBytes)
	}
	if body.Code != "PROJECT_REQUIRED" || body.RequestID == "" {
		t.Fatalf("error envelope = %+v", body)
	}
}

func TestReviseOutcomeConflictIs409Envelope(t *testing.T) {
	svc := &fakeOutcomeService{}
	svc.revise = func(_ context.Context, _ domain.OutcomeID, _ outcomevc.ReviseContractInput) (outcomevc.View, error) {
		return outcomevc.View{}, apierr.New(apierr.KindConflict, "OUTCOME_CONTRACT_CONFLICT",
			"Contract moved to revision 2; reload and retry against it",
			map[string]any{"expectedRevision": 1, "currentRevision": 2})
	}
	srv := newOutcomesTestServer(t, svc)

	respBytes, status, _ := doRequest(t, srv, http.MethodPost, "/api/v1/outcomes/out-fix/revisions",
		`{"expectedRevision":1,"goal":"g","successCriteria":["c"],"review":"r"}`)
	if status != http.StatusConflict {
		t.Fatalf("conflict status = %d, want 409: %s", status, respBytes)
	}
	for _, want := range []string{`"code":"OUTCOME_CONTRACT_CONFLICT"`, `"currentRevision":2`} {
		if !strings.Contains(string(respBytes), want) {
			t.Fatalf("conflict envelope missing %s: %s", want, respBytes)
		}
	}
}

func TestGetUnknownOutcomeIs404Envelope(t *testing.T) {
	svc := &fakeOutcomeService{}
	svc.get = func(_ context.Context, _ domain.OutcomeID) (outcomevc.View, error) {
		return outcomevc.View{}, apierr.NotFound("OUTCOME_NOT_FOUND", "That Outcome does not exist")
	}
	srv := newOutcomesTestServer(t, svc)
	respBytes, status, _ := doRequest(t, srv, http.MethodGet, "/api/v1/outcomes/out-nope", "")
	if status != http.StatusNotFound || !strings.Contains(string(respBytes), `"code":"OUTCOME_NOT_FOUND"`) {
		t.Fatalf("get = %d %s, want 404 OUTCOME_NOT_FOUND", status, respBytes)
	}
}

// TestOutcomeRoutesFunctionalThroughRealStore is the end-to-end proof for #21:
// real SQLite (migrations + triggers), the real store, the real service, and
// the real router. Create → replay → revise → stale-conflict must behave
// exactly as the contract promises over HTTP.
func TestOutcomeRoutesFunctionalThroughRealStore(t *testing.T) {
	storeHandle := sqlitetest.MustOpen(t)
	ctx := context.Background()
	if err := storeHandle.UpsertProject(ctx, domain.ProjectRecord{
		ID: "mer", Path: "/tmp/kennel-outcome-fixture", RegisteredAt: time.Now().UTC(),
		Kind: domain.ProjectKindSingleRepo,
	}); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	svc := outcomevc.New(storeHandle, nil).WithPlanning(intelligencetest.New(), controllerRouting{})
	svc.AdmissionPolicy = controllerAdmissionPolicy(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(httpd.NewRouterWithControl(config.Config{}, log, nil, httpd.APIDeps{
		Outcomes: svc,
	}, httpd.ControlDeps{}))
	defer srv.Close()

	const createBody = `{"title":"Local Focus Ledger","goal":"Record focus locally.","successCriteria":["Positive minutes create one block."],"review":"Deterministic checks.","requestKey":"req-e2e-1"}`

	// 1) Create lands ContractRevision 1.
	respBytes, status, _ := doRequest(t, srv, http.MethodPost, "/api/v1/projects/mer/outcomes", createBody)
	if status != http.StatusCreated {
		t.Fatalf("create = %d: %s", status, respBytes)
	}
	if !strings.Contains(string(respBytes), `"currentRevisionNumber":1`) ||
		!strings.Contains(string(respBytes), `"id":"out-`) {
		t.Fatalf("create payload unexpected: %s", respBytes)
	}
	id := extractOutcomeID(t, respBytes)

	// 2) Replay with the same key returns the SAME canonical object.
	replayBytes, replayStatus, _ := doRequest(t, srv, http.MethodPost, "/api/v1/projects/mer/outcomes", createBody)
	if replayStatus != http.StatusCreated || extractOutcomeID(t, replayBytes) != id {
		t.Fatalf("replay = %d ids %s vs %s", replayStatus, id, extractOutcomeID(t, replayBytes))
	}

	// 3) Revise against revision 1 appends immutable revision 2.
	revBody := `{"expectedRevision":1,"goal":"Revised goal.","successCriteria":["A","B"],"review":"checks"}`
	revBytes, revStatus, _ := doRequest(t, srv, http.MethodPost, "/api/v1/outcomes/"+id+"/revisions", revBody)
	if revStatus != http.StatusOK || !strings.Contains(string(revBytes), `"currentRevisionNumber":2`) {
		t.Fatalf("revise = %d: %s", revStatus, revBytes)
	}
	if !strings.Contains(string(revBytes), `"number":1`) || !strings.Contains(string(revBytes), `"number":2`) {
		t.Fatalf("revision history missing both entries: %s", revBytes)
	}

	// 4) A stale expected revision is a 409 that changes nothing.
	staleBytes, staleStatus, _ := doRequest(t, srv, http.MethodPost, "/api/v1/outcomes/"+id+"/revisions", revBody)
	if staleStatus != http.StatusConflict || !strings.Contains(string(staleBytes), `"code":"OUTCOME_CONTRACT_CONFLICT"`) {
		t.Fatalf("stale revise = %d: %s", staleStatus, staleBytes)
	}
	readBytes, readStatus, _ := doRequest(t, srv, http.MethodGet, "/api/v1/outcomes/"+id, "")
	if readStatus != http.StatusOK ||
		!strings.Contains(string(readBytes), `"currentRevisionNumber":2`) ||
		strings.Contains(string(readBytes), `"number":3`) {
		t.Fatalf("post-conflict read = %d: %s", readStatus, readBytes)
	}
}

func extractOutcomeID(t *testing.T, body []byte) string {
	t.Helper()
	s := string(body)
	marker := `"id":"out-`
	i := strings.Index(s, marker)
	if i < 0 {
		t.Fatalf("no outcome id in body: %s", s)
	}
	rest := s[i+len(marker):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		t.Fatalf("malformed id in body: %s", s)
	}
	return "out-" + rest[:j]
}

func TestOutcomePlanRoutesFunctionalThroughRealStore(t *testing.T) {
	storeHandle := sqlitetest.MustOpen(t)
	ctx := context.Background()
	if err := storeHandle.UpsertProject(ctx, domain.ProjectRecord{
		ID: "mer", Path: "/tmp/kennel-plan-fixture", RegisteredAt: time.Now().UTC(),
		Kind: domain.ProjectKindSingleRepo,
	}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	svc := outcomevc.New(storeHandle, nil).WithPlanning(intelligencetest.New(), controllerRouting{})
	svc.AdmissionPolicy = controllerAdmissionPolicy(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(httpd.NewRouterWithControl(config.Config{}, log, nil, httpd.APIDeps{
		Outcomes: svc,
	}, httpd.ControlDeps{}))
	defer srv.Close()

	const createBody = `{"title":"Local Focus Ledger","goal":"Record focus locally.","successCriteria":["Positive minutes create one block."],"review":"Deterministic checks.","authorityCeiling":{"readWorkspace":true,"writeWorkspace":true,"executeLocal":true},"requestKey":"req-plan-e2e"}`
	respBytes, status, _ := doRequest(t, srv, http.MethodPost, "/api/v1/projects/mer/outcomes", createBody)
	if status != http.StatusCreated {
		t.Fatalf("create = %d: %s", status, respBytes)
	}
	id := extractOutcomeID(t, respBytes)

	// 1) Propose against revision 1 lands an immutable proposed plan.
	proposeBody := `{"expectedContractRevision":1}`
	planBytes, pStatus, _ := doRequest(t, srv, http.MethodPost, "/api/v1/outcomes/"+id+"/plans", proposeBody)
	if pStatus != http.StatusCreated {
		t.Fatalf("propose = %d: %s", pStatus, planBytes)
	}
	if !strings.Contains(string(planBytes), `"status":"proposed"`) ||
		!strings.Contains(string(planBytes), `"number":1`) ||
		len(strings.Split(string(planBytes), `"scope":"worktree/*"`)) != 4 {
		t.Fatalf("proposal payload unexpected: %s", planBytes)
	}
	var envelope struct {
		Plan struct {
			ID                     string `json:"id"`
			RunBriefCoreDigest     string `json:"runBriefCoreDigest"`
			WorkUnits              []map[string]any
			EvidenceBindsCriterion bool `json:"-"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(planBytes, &envelope); err != nil {
		t.Fatalf("decode proposal: %v", err)
	}
	if len(envelope.Plan.RunBriefCoreDigest) != 64 {
		t.Fatalf("run brief core digest missing: %s", planBytes)
	}

	// 2) Re-proposing the same revision replays the identical plan.
	replayBytes, replayStatus, _ := doRequest(t, srv, http.MethodPost, "/api/v1/outcomes/"+id+"/plans", proposeBody)
	if replayStatus != http.StatusCreated || !strings.Contains(string(replayBytes), envelope.Plan.ID) {
		t.Fatalf("replay = %d want same plan %s: %s", replayStatus, envelope.Plan.ID, replayBytes)
	}

	// 3) Owner approval activates the plan exactly once.
	const approveBody = `{"expectedContractRevision":1}`
	approveBytes, aStatus, _ := doRequest(t, srv, http.MethodPost,
		"/api/v1/outcomes/"+id+"/plans/"+envelope.Plan.ID+"/approval", approveBody)
	if aStatus != http.StatusOK || !strings.Contains(string(approveBytes), `"status":"approved"`) {
		t.Fatalf("approve = %d: %s", aStatus, approveBytes)
	}
	reApproveBytes, reStatus, _ := doRequest(t, srv, http.MethodPost,
		"/api/v1/outcomes/"+id+"/plans/"+envelope.Plan.ID+"/approval", approveBody)
	if reStatus != http.StatusOK || !strings.Contains(string(reApproveBytes), `"status":"approved"`) {
		t.Fatalf("re-approve must be idempotent: %d %s", reStatus, reApproveBytes)
	}

	// 4) Material change: r2 makes the r1-bound plan unapprovable and forces
	//    a fresh brief on the next proposal.
	revBody := `{"expectedRevision":1,"goal":"Record focus locally with notes.","successCriteria":["Blocks","Notes"],"review":"checks","authorityCeiling":{"readWorkspace":true,"writeWorkspace":true,"executeLocal":true}}`
	revBytes, revStatus, _ := doRequest(t, srv, http.MethodPost, "/api/v1/outcomes/"+id+"/revisions", revBody)
	if revStatus != http.StatusOK {
		t.Fatalf("revise = %d: %s", revStatus, revBytes)
	}
	staleBytes, staleStatus, _ := doRequest(t, srv, http.MethodPost,
		"/api/v1/outcomes/"+id+"/plans/"+envelope.Plan.ID+"/approval",
		`{"expectedContractRevision":2}`)
	if staleStatus != http.StatusConflict || !strings.Contains(string(staleBytes), "PLAN_CONTRACT_STALE") {
		t.Fatalf("stale approval = %d want 409 PLAN_CONTRACT_STALE: %s", staleStatus, staleBytes)
	}

	nextBytes, nextStatus, _ := doRequest(t, srv, http.MethodPost, "/api/v1/outcomes/"+id+"/plans", `{"expectedContractRevision":2}`)
	if nextStatus != http.StatusCreated || !strings.Contains(string(nextBytes), `"contractRevisionNumber":2`) {
		t.Fatalf("r2 proposal = %d: %s", nextStatus, nextBytes)
	}
	var nextEnvelope struct {
		Plan struct {
			ID                 string `json:"id"`
			RunBriefCoreDigest string `json:"runBriefCoreDigest"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(nextBytes, &nextEnvelope); err != nil {
		t.Fatalf("decode r2 proposal: %v", err)
	}
	if nextEnvelope.Plan.RunBriefCoreDigest == envelope.Plan.RunBriefCoreDigest {
		t.Fatal("material contract change must produce a fresh RunBrief core digest")
	}

	latestBytes, latestStatus, _ := doRequest(t, srv, http.MethodGet, "/api/v1/outcomes/"+id+"/plan", "")
	if latestStatus != http.StatusOK || !strings.Contains(string(latestBytes), nextEnvelope.Plan.ID) {
		t.Fatalf("latest plan = %d want r2 plan: %s", latestStatus, latestBytes)
	}

	// 5) Unknown outcomes refuse plans.
	if _, missStatus, _ := doRequest(t, srv, http.MethodPost, "/api/v1/outcomes/out-ghost/plans", proposeBody); missStatus != http.StatusNotFound {
		t.Fatalf("unknown outcome propose = %d, want 404", missStatus)
	}
}

func (f *fakeOutcomeService) AskForDecomposition(_ context.Context, outcomeID domain.OutcomeID, expected int64) (outcomevc.DecompositionRequestView, error) {
	f.lastAskOf, f.lastAskRevision = outcomeID, expected
	if f.askForDecomposition == nil {
		return outcomevc.DecompositionRequestView{}, apierr.NotFound("OUTCOME_NOT_FOUND", "not implemented in fake")
	}
	return f.askForDecomposition(outcomeID, expected)
}

func (f *fakeOutcomeService) SubmitAgentProposal(_ context.Context, requestID domain.DecompositionRequestID, token string, in outcomevc.ProposeDecompositionInput, raw string) (outcomevc.DecompositionRequestView, error) {
	f.lastSubmitID, f.lastSubmitToken, f.lastSubmitInput, f.lastSubmitRaw = requestID, token, in, raw
	if f.submitAgentProposal == nil {
		return outcomevc.DecompositionRequestView{}, apierr.NotFound("DECOMPOSITION_REQUEST_NOT_FOUND", "not implemented in fake")
	}
	return f.submitAgentProposal(requestID, token, in, raw)
}

func (f *fakeOutcomeService) LatestDecompositionRequest(_ context.Context, outcomeID domain.OutcomeID) (outcomevc.DecompositionRequestView, error) {
	if f.latestDecompositionRequest == nil {
		return outcomevc.DecompositionRequestView{}, apierr.NotFound("DECOMPOSITION_REQUEST_NOT_FOUND", "not implemented in fake")
	}
	return f.latestDecompositionRequest(outcomeID)
}
