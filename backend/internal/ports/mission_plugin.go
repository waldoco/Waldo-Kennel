package ports

import (
	"context"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// MissionPluginProvisioner ensures the Kennel mission plugin - the installed
// /mission planning command - is present, enabled, and byte-verified in the
// Codex harness home scoped to one responsibility space before mission start
// is offered. It fails closed: any error means planning must not start,
// because an unverified mission command is documentation, not a runtime
// artifact.
type MissionPluginProvisioner interface {
	// EnsureMissionPlugin installs or verifies the plugin and returns the
	// scoped CODEX_HOME carrying it. The home identifies the provisioning so
	// the planning launch path can run inside exactly what was verified.
	EnsureMissionPlugin(ctx context.Context, spaceID domain.ResponsibilitySpaceID) (string, error)
}
