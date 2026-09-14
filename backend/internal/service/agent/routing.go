package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// RoutingSnapshot adapts the existing machine-aware agent inventory into the
// provider-neutral facts consumed by WT3. It reports capability/readiness/model
// support but never chooses a provider; domain.RouteExecution owns policy.
func (s *Service) RoutingSnapshot(ctx context.Context, projectID domain.ProjectID, preference *domain.RoutingPreference) (ports.RoutingInventorySnapshot, error) {
	inventory, err := s.Refresh(ctx)
	if err != nil {
		return ports.RoutingInventorySnapshot{}, err
	}
	installed := make(map[string]Info, len(inventory.Installed))
	for _, info := range inventory.Installed {
		installed[info.ID] = info
	}

	generationRaw, err := json.Marshal(struct {
		Supported any
		Installed any
	}{inventory.Supported, inventory.Installed})
	if err != nil {
		return ports.RoutingInventorySnapshot{}, fmt.Errorf("encode routing generation: %w", err)
	}
	generationID := string(domain.DigestSHA256(generationRaw))
	candidates := make([]domain.RoutingCandidate, 0, len(inventory.Supported))
	for _, supported := range inventory.Supported {
		harness := domain.AgentHarness(supported.ID)
		if !harness.IsSelectableForNewWork() {
			continue
		}
		local, isInstalled := installed[supported.ID]
		candidate := domain.RoutingCandidate{
			ID:                  supported.ID,
			Provider:            supported.ID,
			ModelSelection:      domain.ExecutionBindingModelProviderDefault,
			WorkerEligible:      supported.Roles.Worker,
			CoordinatorEligible: supported.Roles.Coordinator,
			Readiness:           domain.CapabilityUnsupported,
			Capabilities:        map[string]domain.CapabilitySupport{},
			Models:              map[string]domain.CapabilitySupport{},
		}
		if supported.Roles.Worker {
			candidate.Capabilities[domain.CapabilityWorktreeRead] = domain.CapabilitySupported
			candidate.Capabilities[domain.CapabilityWorktreeWrite] = domain.CapabilitySupported
			candidate.Capabilities[domain.CapabilityWorktreeExec] = domain.CapabilitySupported
		}
		if isInstalled {
			candidate.Readiness = readinessForRouting(s, local)
		}

		// An explicit model preference belongs only to its preferred provider.
		// Loading every provider's catalog just to validate another provider's
		// model would recreate the candidate-local WT3 bug and waste CLI calls.
		if preference != nil && preference.Provider == supported.ID && preference.ModelSelection == domain.ExecutionPreferenceModelExplicit {
			status := domain.CapabilityUnknown
			catalog, catalogErr := s.Models(ctx, supported.ID, string(projectID), false)
			if catalogErr == nil {
				status = domain.CapabilityUnsupported
				for _, model := range catalog.Models {
					if model.ID == preference.Model {
						status = domain.CapabilitySupported
						break
					}
				}
				if status != domain.CapabilitySupported && catalog.AllowCustom {
					status = domain.CapabilitySupported
				}
			}
			candidate.Models[preference.Model] = status
		}
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	encoded, err := json.Marshal(candidates)
	if err != nil {
		return ports.RoutingInventorySnapshot{}, fmt.Errorf("encode routing inventory: %w", err)
	}
	return ports.RoutingInventorySnapshot{
		GenerationID: generationID,
		SnapshotID:   string(domain.DigestSHA256(encoded)),
		Candidates:   candidates,
	}, nil
}

func readinessForRouting(s *Service, info Info) domain.CapabilitySupport {
	if info.Ready != nil && !*info.Ready {
		return domain.CapabilityUnsupported
	}
	switch info.AuthStatus {
	case ports.AgentAuthStatusAuthorized:
		return domain.CapabilitySupported
	case ports.AgentAuthStatusUnauthorized:
		return domain.CapabilityUnsupported
	case ports.AgentAuthStatusUnknown, "":
		if item, ok := s.agent(info.ID); ok {
			if optional, ok := item.Agent.(ports.AgentOptionalAuth); ok && optional.AuthOptional() {
				return domain.CapabilitySupported
			}
		}
		return domain.CapabilityUnknown
	default:
		return domain.CapabilityUnknown
	}
}

var _ ports.ExecutionRoutingInventory = (*Service)(nil)
