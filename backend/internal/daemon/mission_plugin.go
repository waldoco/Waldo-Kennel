package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	codexagent "github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/agent/codex"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/missionplugin"
)

// missionPluginProvisioner installs and verifies the Kennel mission plugin in
// the mission-scoped Codex home of one responsibility space. It exists so a
// native-harness planning start can prove the /mission command is a real,
// byte-verified runtime artifact before offering mission start.
type missionPluginProvisioner struct {
	dataDir string
	log     *slog.Logger
}

func newMissionPluginProvisioner(dataDir string, log *slog.Logger) *missionPluginProvisioner {
	if log == nil {
		log = slog.Default()
	}
	return &missionPluginProvisioner{dataDir: dataDir, log: log}
}

func (p *missionPluginProvisioner) EnsureMissionPlugin(ctx context.Context, spaceID domain.ResponsibilitySpaceID) (string, error) {
	if spaceID.IsZero() {
		return "", fmt.Errorf("mission plugin provisioning requires a responsibility space")
	}
	agent := codexagent.New()
	bin, err := agent.ResolveBinary(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve the Codex binary: %w - repair: install Codex and sign in, then retry mission start", err)
	}
	marketDir := filepath.Join(p.dataDir, "mission-plugin", "marketplace")
	if err := missionplugin.Materialize(marketDir); err != nil {
		return "", err
	}
	home, err := codexagent.ProvisionMissionCodexHome(p.dataDir, string(spaceID))
	if err != nil {
		return "", err
	}
	if err := codexagent.EnsureMissionPlugin(ctx, bin, home, marketDir); err != nil {
		return "", err
	}
	return home, nil
}
