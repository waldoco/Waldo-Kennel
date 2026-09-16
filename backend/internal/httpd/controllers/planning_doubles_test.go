package controllers_test

import (
	"context"
	"strings"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// controllerRouting is the execution-routing double for the functional HTTP
// tests. It reports one fully capable, ready candidate so Plan compilation can
// bind a provider without probing the machine.
type controllerRouting struct{}

var _ ports.ExecutionRoutingInventory = controllerRouting{}

// controllerAdmissionPolicy is the production-shape policy fixture used by
// functional HTTP tests. Leaving it nil makes planning deterministically reject
// every generated work unit for missing budgets.
func controllerAdmissionPolicy(t interface {
	Helper()
	Fatal(...any)
}) *domain.AdmissionPolicy {
	t.Helper()
	budget := domain.ExecutionBudget{
		WallTimeLimit: time.Hour, RetryLimit: 1,
		TokenAccounting: domain.TokenAccountingUnsupported,
		Source:          domain.ExecutionBudgetPolicyDefault,
		PolicyID:        "controller-test-policy", PolicyVersion: "v1",
		PolicyDigest: strings.Repeat("0", 64),
	}
	policy := &domain.AdmissionPolicy{ID: budget.PolicyID, Version: budget.PolicyVersion, Default: budget, MaxWallTime: 2 * time.Hour, MaxRetries: 2, MaxTokens: 100000}
	digest, err := policy.ComputedDigest()
	if err != nil {
		t.Fatal(err)
	}
	policy.Digest, policy.Default.PolicyDigest = digest, digest
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
	return policy
}

func (controllerRouting) RoutingSnapshot(context.Context, domain.ProjectID, *domain.RoutingPreference) (ports.RoutingInventorySnapshot, error) {
	return ports.RoutingInventorySnapshot{
		GenerationID: "controller-test-generation",
		SnapshotID:   "controller-test-snapshot",
		Candidates: []domain.RoutingCandidate{{
			ID:                  string(domain.HarnessCodex),
			Provider:            string(domain.HarnessCodex),
			ModelSelection:      domain.ExecutionBindingModelProviderDefault,
			WorkerEligible:      true,
			CoordinatorEligible: domain.HarnessCodex.IsSelectableAsCoordinator(),
			Readiness:           domain.CapabilitySupported,
			Capabilities: map[string]domain.CapabilitySupport{
				domain.CapabilityWorktreeRead:  domain.CapabilitySupported,
				domain.CapabilityWorktreeWrite: domain.CapabilitySupported,
				domain.CapabilityWorktreeExec:  domain.CapabilitySupported,
			},
			Models: map[string]domain.CapabilitySupport{},
		}},
	}, nil
}
