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

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/config"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apierr"
	outcomevc "github.com/Pin4sf/Waldo-Kennel/backend/internal/service/outcome"
)

type fakeProofManager struct {
	get          func(context.Context, domain.OutcomeID) (outcomevc.ProofView, error)
	evidence     func(context.Context, domain.OutcomeID, outcomevc.RecordEvidenceInput) (outcomevc.ProofView, error)
	verification func(context.Context, domain.OutcomeID, outcomevc.RecordVerificationInput) (outcomevc.ProofView, error)
	decision     func(context.Context, domain.OutcomeID, outcomevc.DecideAcceptanceInput) (outcomevc.ProofView, error)

	batchEligibility func() ([]domain.BatchEntryVerdict, error)
	acceptBatch      func(domain.OutcomeID, outcomevc.AcceptBatchInput) (outcomevc.AcceptBatchView, error)

	lastBatchOf domain.OutcomeID
	lastBatch   outcomevc.AcceptBatchInput
}

func (f *fakeProofManager) GetProof(ctx context.Context, id domain.OutcomeID) (outcomevc.ProofView, error) {
	return f.get(ctx, id)
}
func (f *fakeProofManager) RecordEvidence(ctx context.Context, id domain.OutcomeID, in outcomevc.RecordEvidenceInput) (outcomevc.ProofView, error) {
	return f.evidence(ctx, id, in)
}
func (f *fakeProofManager) RecordVerification(ctx context.Context, id domain.OutcomeID, in outcomevc.RecordVerificationInput) (outcomevc.ProofView, error) {
	return f.verification(ctx, id, in)
}
func (f *fakeProofManager) DecideAcceptance(ctx context.Context, id domain.OutcomeID, in outcomevc.DecideAcceptanceInput) (outcomevc.ProofView, error) {
	return f.decision(ctx, id, in)
}

func proofFixture() outcomevc.ProofView {
	revisionID := domain.ContractRevisionID("cr-proof")
	criterion := domain.ContractCriterion{ID: "crit-proof", ContractRevisionID: revisionID, Position: 1, Text: "It works."}
	return outcomevc.ProofView{
		OutcomeID: "out-proof",
		Contract: domain.ContractRevision{
			ID: revisionID, OutcomeID: "out-proof", Number: 1, Goal: "Prove it.",
			Criteria: []domain.ContractCriterion{criterion}, SuccessCriteria: []string{criterion.Text}, Review: "Owner review.",
		},
		Status: outcomevc.ProofStatusReadyForAcceptance, NextAction: "Review and accept.",
		Criteria: []outcomevc.CriterionProofView{{Criterion: criterion, Ready: true}},
	}
}

func newProofServer(t *testing.T, proof *fakeProofManager) *httptest.Server {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return httptest.NewServer(httpd.NewRouterWithControl(config.Config{}, log, nil, httpd.APIDeps{Proof: proof}, httpd.ControlDeps{}))
}

func TestOutcomeProofRoutesUseTypedContract(t *testing.T) {
	fixture := proofFixture()
	manager := &fakeProofManager{}
	manager.get = func(_ context.Context, id domain.OutcomeID) (outcomevc.ProofView, error) {
		if id != fixture.OutcomeID {
			t.Fatalf("outcome id=%s", id)
		}
		return fixture, nil
	}
	manager.evidence = func(_ context.Context, id domain.OutcomeID, in outcomevc.RecordEvidenceInput) (outcomevc.ProofView, error) {
		if id != fixture.OutcomeID || in.ContractRevisionID != fixture.Contract.ID || in.CriterionID != "crit-proof" || in.SubjectType != domain.ProofSubjectOutcome || in.RequestKey != "ev-key" {
			t.Fatalf("evidence input=%+v id=%s", in, id)
		}
		return fixture, nil
	}
	manager.verification = func(_ context.Context, id domain.OutcomeID, in outcomevc.RecordVerificationInput) (outcomevc.ProofView, error) {
		if id != fixture.OutcomeID || len(in.EvidenceItemIDs) != 1 || in.IndependenceClass != domain.VerificationOwnerWalkthrough {
			t.Fatalf("verification input=%+v id=%s", in, id)
		}
		return fixture, nil
	}
	manager.decision = func(_ context.Context, id domain.OutcomeID, in outcomevc.DecideAcceptanceInput) (outcomevc.ProofView, error) {
		if id != fixture.OutcomeID || in.Kind != domain.AcceptanceAccept || in.ResourceDisposition != domain.ResourceDispositionRetain {
			t.Fatalf("decision input=%+v id=%s", in, id)
		}
		return fixture, nil
	}
	srv := newProofServer(t, manager)
	defer srv.Close()

	get, status, _ := doRequest(t, srv, http.MethodGet, "/api/v1/outcomes/out-proof/proof", "")
	if status != http.StatusOK || !strings.Contains(string(get), `"criterionId":"crit-proof"`) || !strings.Contains(string(get), `"status":"ready_for_acceptance"`) {
		t.Fatalf("get proof status=%d body=%s", status, get)
	}

	evidenceBody := `{"expectedContractRevision":1,"contractRevisionId":"cr-proof","criterionId":"crit-proof","subjectType":"outcome","subjectId":"out-proof","subjectRevision":"cr-proof","kind":"supporting","sourceType":"owner_walkthrough","sourceRef":"walkthrough","producerType":"user","producerRef":"owner","summary":"Works.","contentDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","requestKey":"ev-key"}`
	if body, status, _ := doRequest(t, srv, http.MethodPost, "/api/v1/outcomes/out-proof/evidence", evidenceBody); status != http.StatusCreated {
		t.Fatalf("evidence status=%d body=%s", status, body)
	}
	verificationBody := `{"expectedContractRevision":1,"contractRevisionId":"cr-proof","criterionId":"crit-proof","subjectType":"outcome","subjectId":"out-proof","subjectRevision":"cr-proof","evidenceItemIds":["ev-proof"],"method":"Owner review.","independenceClass":"owner_walkthrough","result":"passed","verifierRef":"owner","requestKey":"ver-key"}`
	if body, status, _ := doRequest(t, srv, http.MethodPost, "/api/v1/outcomes/out-proof/verifications", verificationBody); status != http.StatusCreated {
		t.Fatalf("verification status=%d body=%s", status, body)
	}
	decisionBody := `{"expectedContractRevision":1,"contractRevisionId":"cr-proof","kind":"accept","summary":"Accepted.","resourceDisposition":"retain","requestKey":"acc-key"}`
	if body, status, _ := doRequest(t, srv, http.MethodPost, "/api/v1/outcomes/out-proof/acceptance-decisions", decisionBody); status != http.StatusCreated {
		t.Fatalf("decision status=%d body=%s", status, body)
	}
}

// TestGetOutcomeProofRoute_NamesTheCorrectionStillStanding separates the
// append-only correction history from the one the owner is currently waiting
// on. A renderer that showed the whole list would keep asking for changes that
// were already made.
func TestGetOutcomeProofRoute_NamesTheCorrectionStillStanding(t *testing.T) {
	fixture := proofFixture()
	fixture.Status = outcomevc.ProofStatusReworkRequired
	superseded := domain.OutcomeCorrection{
		ID: "corr-old", DecisionID: "acc-old", OutcomeID: fixture.OutcomeID,
		ContractRevisionID: fixture.Contract.ID, Feedback: "first pass was thin",
		TargetType: domain.ReentryTargetWorkUnit, TargetID: "wu-1",
	}
	standing := domain.OutcomeCorrection{
		ID: "corr-current", DecisionID: "acc-current", OutcomeID: fixture.OutcomeID,
		ContractRevisionID: fixture.Contract.ID, Feedback: "the Plan itself is wrong",
		TargetType: domain.ReentryTargetPlan, TargetID: "plan-1",
	}
	fixture.Corrections = []domain.OutcomeCorrection{superseded, standing}
	fixture.ActiveCorrection = &standing

	manager := &fakeProofManager{}
	manager.get = func(context.Context, domain.OutcomeID) (outcomevc.ProofView, error) { return fixture, nil }
	srv := newProofServer(t, manager)
	defer srv.Close()

	body, status, _ := doRequest(t, srv, http.MethodGet, "/api/v1/outcomes/out-proof/proof", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	var decoded struct {
		Proof struct {
			ActiveCorrectionID string `json:"activeCorrectionId"`
			Corrections        []struct {
				ID string `json:"id"`
			} `json:"corrections"`
		} `json:"proof"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if decoded.Proof.ActiveCorrectionID != "corr-current" {
		t.Fatalf("activeCorrectionId = %q, want corr-current", decoded.Proof.ActiveCorrectionID)
	}
	// The superseded correction stays in the history: rework never erases what
	// the owner asked for before.
	if len(decoded.Proof.Corrections) != 2 {
		t.Fatalf("corrections = %+v, want both retained", decoded.Proof.Corrections)
	}
}

func TestOutcomeProofConflictPreservesRequestIDEnvelope(t *testing.T) {
	manager := &fakeProofManager{}
	manager.get = func(context.Context, domain.OutcomeID) (outcomevc.ProofView, error) {
		return outcomevc.ProofView{}, apierr.Conflict("OUTCOME_PROOF_CONTRACT_CONFLICT", "Contract moved", map[string]any{"currentRevision": 2})
	}
	srv := newProofServer(t, manager)
	defer srv.Close()

	body, status, _ := doRequest(t, srv, http.MethodGet, "/api/v1/outcomes/out-proof/proof", "")
	if status != http.StatusConflict {
		t.Fatalf("status=%d body=%s", status, body)
	}
	var response struct {
		Code      string `json:"code"`
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(body, &response); err != nil || response.Code != "OUTCOME_PROOF_CONTRACT_CONFLICT" || response.RequestID == "" {
		t.Fatalf("envelope=%+v err=%v body=%s", response, err, body)
	}
}

func TestOutcomeProofRoutesAnswer501WhenUnwired(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(httpd.NewRouterWithControl(config.Config{}, log, nil, httpd.APIDeps{}, httpd.ControlDeps{}))
	defer srv.Close()
	for _, route := range []string{"proof", "evidence", "verifications", "acceptance-decisions"} {
		method := http.MethodPost
		if route == "proof" {
			method = http.MethodGet
		}
		body, status, _ := doRequest(t, srv, method, "/api/v1/outcomes/out-proof/"+route, `{}`)
		if status != http.StatusNotImplemented || !strings.Contains(string(body), `"requestId"`) {
			t.Fatalf("%s status=%d body=%s", route, status, body)
		}
	}
}

func (f *fakeProofManager) BatchEligibility(_ context.Context, _ domain.OutcomeID) ([]domain.BatchEntryVerdict, error) {
	if f.batchEligibility == nil {
		return nil, apierr.NotFound("OUTCOME_NOT_FOUND", "not implemented in fake")
	}
	return f.batchEligibility()
}

func (f *fakeProofManager) AcceptContributorBatch(_ context.Context, parentID domain.OutcomeID, in outcomevc.AcceptBatchInput) (outcomevc.AcceptBatchView, error) {
	f.lastBatchOf, f.lastBatch = parentID, in
	if f.acceptBatch == nil {
		return outcomevc.AcceptBatchView{}, apierr.NotFound("OUTCOME_NOT_FOUND", "not implemented in fake")
	}
	return f.acceptBatch(parentID, in)
}

func TestOutcomeResultUsesStructuredVerdictsInsteadOfNarrative(t *testing.T) {
	for _, tc := range []struct {
		name      string
		verdict   domain.VerificationResult
		detail    string
		uncertain bool
	}{
		{"passed with false termination flag", domain.VerificationPassed, "terminationUnknown=false", false},
		{"failed with unknown in output", domain.VerificationFailed, "unknown command", false},
		{"inconclusive without magic words", domain.VerificationInconclusive, "result changed while checking", true},
		{"evidence awaiting verification", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := proofFixture()
			fixture.Criteria[0].Evidence = []domain.EvidenceItem{{ID: "e-check", CriterionID: "crit-proof", SourceType: domain.EvidenceSourceDeterministicCheck, SourceRef: "test-command", SubjectRevision: "artifact-1", Summary: "unknown is ordinary command output"}}
			if tc.verdict != "" {
				fixture.Criteria[0].Verifications = []domain.VerificationRun{{EvidenceItemIDs: []domain.EvidenceItemID{"e-check"}, Result: tc.verdict, Detail: tc.detail}}
			}
			body, status := resultProofResponse(fixture)
			if status != http.StatusOK {
				t.Fatalf("status=%d body=%s", status, body)
			}
			var decoded struct {
				Proof struct {
					Result struct {
						Checks []struct {
							Verdict   string
							Uncertain bool
						}
						Uncertainty []string
					}
				}
			}
			if err := json.Unmarshal(body, &decoded); err != nil {
				t.Fatal(err)
			}
			result := decoded.Proof.Result
			if len(result.Checks) != 1 || result.Checks[0].Uncertain != tc.uncertain || result.Checks[0].Verdict != string(tc.verdict) {
				t.Fatalf("checks=%+v", result.Checks)
			}
			if (len(result.Uncertainty) > 0) != tc.uncertain {
				t.Fatalf("uncertainty=%v", result.Uncertainty)
			}
		})
	}
}

func TestOutcomeResultDoesNotInferArtifactMutationFromSummary(t *testing.T) {
	fixture := proofFixture()
	fixture.Criteria[0].Evidence = []domain.EvidenceItem{{SourceType: domain.EvidenceSourceArtifact, SourceRef: "report.md", SubjectRevision: "artifact-1", Summary: "The report is unchanged", ContentDigest: "digest-1"}}
	body, status := resultProofResponse(fixture)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	var decoded struct {
		Proof struct {
			Result struct{ Artifacts []map[string]any }
		}
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	artifacts := decoded.Proof.Result.Artifacts
	if len(artifacts) != 1 || artifacts[0]["revision"] != "artifact-1" || artifacts[0]["digest"] != "digest-1" {
		t.Fatalf("artifacts=%v", artifacts)
	}
	if _, present := artifacts[0]["changed"]; present {
		t.Fatalf("unmeasured mutation must not be claimed: %v", artifacts)
	}
}

func resultProofResponse(fixture outcomevc.ProofView) ([]byte, int) {
	manager := &fakeProofManager{get: func(context.Context, domain.OutcomeID) (outcomevc.ProofView, error) { return fixture, nil }}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := httpd.NewRouterWithControl(config.Config{}, log, nil, httpd.APIDeps{Proof: manager}, httpd.ControlDeps{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/outcomes/out-proof/proof", nil))
	return response.Body.Bytes(), response.Code
}
