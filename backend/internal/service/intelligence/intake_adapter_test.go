package intelligence

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
)

type countingIntelligenceProvider struct{ analyzeCalls int }

func (*countingIntelligenceProvider) ID() domain.IntelligenceProviderID { return "counting-test" }
func (p *countingIntelligenceProvider) AnalyzeContract(context.Context, ports.ContractIntelligenceRequest) (ports.ContractIntelligenceResponse, error) {
	p.analyzeCalls++
	return ports.ContractIntelligenceResponse{}, errors.New("provider must not be called")
}
func (*countingIntelligenceProvider) DraftPlan(context.Context, ports.PlanIntelligenceRequest) (ports.PlanIntelligenceResponse, error) {
	return ports.PlanIntelligenceResponse{}, errors.New("not used")
}

type failingRepositoryContextLimits struct{ err error }

func (f failingRepositoryContextLimits) RepositoryContextLimits(context.Context) (int, int, int, error) {
	return 0, 0, 0, f.err
}

func TestDigestContractRequestIncludesRepositoryContext(t *testing.T) {
	base := ports.ContractIntelligenceRequest{
		Session: domain.IntakeSession{
			ID:        "intake-1",
			ProjectID: "project-1",
			Statement: "make the repository safer",
		},
		RepositoryContext: ports.RepositoryContextSnapshot{
			ProjectID: "project-1",
			Revision:  "rev-1",
			Files: []ports.RepositoryContextFile{{
				Path:    "README.md",
				Content: "first inspected fact",
			}},
		},
	}
	changed := base
	changed.RepositoryContext.Files = []ports.RepositoryContextFile{{
		Path:    "README.md",
		Content: "different inspected fact",
	}}

	first, err := digestContractRequest(base)
	if err != nil {
		t.Fatalf("digest base request: %v", err)
	}
	second, err := digestContractRequest(changed)
	if err != nil {
		t.Fatalf("digest changed request: %v", err)
	}
	if first == second {
		t.Fatalf("repository context did not affect contract input digest: %q", first)
	}
}

func TestIntakeAnalyzerRefusesSettingsReadFailureBeforeProviderContext(t *testing.T) {
	ctx := context.Background()
	store := sqlitetest.MustOpen(t)
	project := domain.ProjectRecord{ID: "limits-error-project", Path: t.TempDir(), DisplayName: "Limits error", RegisteredAt: time.Now().UTC()}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	provider := &countingIntelligenceProvider{}
	want := errors.New("settings database unavailable")
	analyzer := NewIntakeAnalyzer(provider, store, nil).
		WithRepositoryContextSource(store).
		WithRepositoryContextLimits(failingRepositoryContextLimits{err: want}).
		WithAdmissionEvaluator(admissionEvaluatorFake{eligible: true})

	_, err := analyzer.Analyze(ctx, ports.IntakeAnalysisInput{Session: domain.IntakeSession{
		ID: "intake-limits-error", ProjectID: domain.ProjectID(project.ID), Statement: "Inspect the repository safely",
	}})
	if !errors.Is(err, want) || !strings.Contains(err.Error(), "resolve repository-context limits") {
		t.Fatalf("Analyze() error = %v, want surfaced settings failure", err)
	}
	if provider.analyzeCalls != 0 {
		t.Fatalf("provider calls = %d, want zero before repository context is sent", provider.analyzeCalls)
	}
}

type admissionEvaluatorFake struct{ eligible bool }

func (f admissionEvaluatorFake) EvaluateAdmissionStage(context.Context, ports.AdmissionStageInput) (ports.AdmissionStageResult, error) {
	return ports.AdmissionStageResult{Eligible: f.eligible, Verdict: domain.AdmissionVerdict{Reasons: []domain.AdmissionReasonCode{domain.AdmissionProviderUnavailable}}}, nil
}

func TestIntakeAnalyzerRequiresAndEnforcesContractAdmission(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	provider := &countingIntelligenceProvider{}
	input := ports.IntakeAnalysisInput{Session: domain.IntakeSession{ID: "intake", ProjectID: "project", Statement: "inspect"}}
	if _, err := NewIntakeAnalyzer(provider, store, nil).Analyze(context.Background(), input); err == nil || !strings.Contains(err.Error(), "admission is unavailable") {
		t.Fatalf("missing evaluator err=%v", err)
	}
	if _, err := NewIntakeAnalyzer(provider, store, nil).WithAdmissionEvaluator(admissionEvaluatorFake{}).Analyze(context.Background(), input); err == nil || !strings.Contains(err.Error(), "current verified capabilities") {
		t.Fatalf("rejection err=%v", err)
	}
	if provider.analyzeCalls != 0 {
		t.Fatalf("provider calls=%d", provider.analyzeCalls)
	}
}
