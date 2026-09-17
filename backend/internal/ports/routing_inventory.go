package ports

import (
	"context"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// RoutingInventorySnapshot is normalized, machine-aware harness truth used by
// deterministic routing. SnapshotID is provenance only; Attempt admission still
// performs provider-native readiness checks before launch.
type RoutingInventorySnapshot struct {
	GenerationID string
	SnapshotID   string
	Candidates   []domain.RoutingCandidate
}

// ExecutionRoutingInventory reports provider-neutral candidates. The optional
// preference is supplied only so inventory can load candidate-local model
// support when an explicit model needs validation; it never chooses the winner.
type ExecutionRoutingInventory interface {
	RoutingSnapshot(context.Context, domain.ProjectID, *domain.RoutingPreference) (RoutingInventorySnapshot, error)
}
