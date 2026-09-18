// Package intelligencetest provides a fixed IntelligenceProvider for tests
// that need the Outcome control plane wired without a model call.
//
// It exists so a canned proposal stays test-only. The product deliberately has
// no rule-based intelligence behind Waldo: a fixed answer that looks like
// understanding is exactly what hid a broken front door, so it lives here and
// nowhere else.
package intelligencetest

import (
	"context"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// ProviderID names the fixed test provider in provenance records.
const ProviderID domain.IntelligenceProviderID = "test-fixed"

// Provider answers every request with the same structurally valid material.
type Provider struct{}

var _ ports.IntelligenceProvider = Provider{}

// New builds the fixed test provider.
func New() Provider { return Provider{} }

// ID returns the fixed provider identity.
func (Provider) ID() domain.IntelligenceProviderID { return ProviderID }

// AnalyzeContract returns one editable proposal derived from the statement.
func (Provider) AnalyzeContract(_ context.Context, request ports.ContractIntelligenceRequest) (ports.ContractIntelligenceResponse, error) {
	title := request.Session.Statement
	if title == "" {
		title = "Test outcome"
	}
	return ports.ContractIntelligenceResponse{
		Result: ports.IntakeAnalysisResult{Proposal: &domain.OutcomeContractProposal{
			Title:        title,
			DesiredState: request.Session.Statement,
			Criteria: []domain.ProposedCriterion{{
				Text:             "The requested result is observable and matches the confirmed desired state.",
				EvidenceExpected: []string{"A deterministic check demonstrates the result."},
			}},
			ReviewMethod:     "Run the relevant deterministic checks, then complete an owner walkthrough.",
			AuthorityCeiling: domain.ProposedAuthority{ReadWorkspace: true, WriteWorkspace: true, ExecuteLocal: true},
			StopConditions:   []string{"Stop before any external effect the owner has not authorized."},
			Facets:           []domain.ContractFacet{{Kind: domain.ContractFacetSoftware, Summary: title}},
		}},
		Provenance: ports.IntelligenceProvenance{EffectiveProvider: ProviderID, EffectiveModel: "fixed"},
	}, nil
}

// DraftPlan returns one direct work unit covering every criterion, which is
// the smallest plan shape the compiler accepts.
func (Provider) DraftPlan(_ context.Context, request ports.PlanIntelligenceRequest) (ports.PlanIntelligenceResponse, error) {
	covered := make([]string, 0, len(request.CriterionAliases))
	for alias := range request.CriterionAliases {
		covered = append(covered, alias)
	}
	sortStrings(covered)
	return ports.PlanIntelligenceResponse{
		Readiness: domain.NewPlanningReadinessResult("Ready.", &domain.PlanDraftProposal{
			Summary: "Deliver the outcome in one direct unit.",
			WorkUnits: []domain.PlanDraftWorkUnit{{
				Key:             "W1",
				Title:           "Deliver \"" + request.Outcome.Title + "\"",
				Intent:          intentWithin(request.Contract.AuthorityCeiling),
				Role:            roleForIntent(intentWithin(request.Contract.AuthorityCeiling)),
				OutputSummary:   "The finished result, built and verified inside the isolated project worktree.",
				CriteriaCovered: covered,
				EvidenceIdeas:   []string{"A deterministic check demonstrates the result."},
			}},
		}, nil),
		Provenance: ports.IntelligenceProvenance{EffectiveProvider: ProviderID, EffectiveModel: "fixed"},
	}, nil
}

// intentWithin picks the widest intent the Contract ceiling actually allows.
// Real intelligence must never propose work that needs authority the owner did
// not grant, so the double does not either — it would only ever fail
// compilation at the authority gate.
func intentWithin(ceiling domain.ProposedAuthority) domain.WorkUnitIntent {
	switch {
	case ceiling.WriteWorkspace && ceiling.ExecuteLocal:
		return domain.WorkUnitIntentModifyAndExecute
	case ceiling.WriteWorkspace:
		return domain.WorkUnitIntentModify
	case ceiling.ExecuteLocal:
		return domain.WorkUnitIntentExecute
	default:
		return domain.WorkUnitIntentInspect
	}
}

// sortStrings keeps alias order stable without pulling sort into the import
// list of every consumer.
func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func roleForIntent(intent domain.WorkUnitIntent) domain.WorkUnitRole {
	if intent == domain.WorkUnitIntentInspect {
		return domain.WorkUnitRoleInvestigate
	}
	if intent == domain.WorkUnitIntentExecute {
		return domain.WorkUnitRoleVerify
	}
	return domain.WorkUnitRoleImplement
}
