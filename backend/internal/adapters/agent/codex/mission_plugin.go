package codex

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/chatdriver/processenv"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/missionplugin"
)

// Mission-plugin provisioning for the pinned Codex harness. All mechanics here
// were verified live against the pinned 0.153.4 binary: a local marketplace +
// plugin install through the provider's own `codex plugin` CLI, plugin skills
// namespaced "<plugin>:<skill>" in app-server skills/list, and - critically -
// config.toml entries alone do NOT re-materialize a wiped plugin cache, so the
// CLI install path is the only honest one.

// ProvisionMissionCodexHome prepares the mission-scoped Codex home for one
// responsibility space, reseeding credentials just-in-time exactly like a
// governed Attempt home. The home keeps the mission plugin out of the user's
// real ~/.codex: per-project isolation, nothing platform-global.
func ProvisionMissionCodexHome(dataDir, spaceKey string) (string, error) {
	home, _, err := governedHomeDir(dataDir, "mission-"+spaceKey, true)
	if err != nil {
		return "", err
	}
	if err := seedCodexAuth(home); err != nil {
		return "", err
	}
	return home, nil
}

// EnsureMissionPlugin installs the Kennel mission plugin into home through the
// provider's own CLI and verifies the result fail-closed: marketplace
// registered at the exact Kennel tree, plugin installed+enabled at the shipped
// version, and the installed cache byte-identical to the embedded manifest.
// Every failure names what broke and the repair.
func EnsureMissionPlugin(ctx context.Context, codexBin, home, marketDir string) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	out, err := runPluginCLI(ctx, codexBin, home, "plugin", "marketplace", "list")
	if err != nil {
		return fmt.Errorf("list Codex plugin marketplaces: %w: %s - repair: run `codex plugin marketplace list` with CODEX_HOME=%s and check the harness install", err, out, home)
	}
	registered := false
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != missionplugin.MarketplaceName {
			continue
		}
		if strings.Contains(line, marketDir) {
			registered = true
			continue
		}
		// Same name, stale root: the data dir moved. Re-register at ours.
		if out, err := runPluginCLI(ctx, codexBin, home, "plugin", "marketplace", "remove", missionplugin.MarketplaceName); err != nil {
			return fmt.Errorf("remove stale mission marketplace registration: %w: %s", err, out)
		}
	}
	if !registered {
		if out, err := runPluginCLI(ctx, codexBin, home, "plugin", "marketplace", "add", marketDir); err != nil {
			return fmt.Errorf("register the Kennel mission marketplace: %w: %s", err, out)
		}
	}

	row, found, err := missionPluginRow(ctx, codexBin, home)
	if err != nil {
		return err
	}
	install := true
	if found {
		current := strings.Contains(row, "installed, enabled") && pluginRowHasVersion(row)
		if current {
			if err := verifyMissionPluginCache(home); err == nil {
				install = false
			}
		}
		if install {
			if out, err := runPluginCLI(ctx, codexBin, home, "plugin", "remove", missionplugin.PluginID); err != nil {
				return fmt.Errorf("remove drifted mission plugin: %w: %s", err, out)
			}
		}
	}
	if install {
		if out, err := runPluginCLI(ctx, codexBin, home, "plugin", "add", missionplugin.PluginID); err != nil {
			return fmt.Errorf("install the Kennel mission plugin: %w: %s - repair: remove %s and retry mission start so Kennel reinstalls it", err, out, filepath.Join(home, "plugins"))
		}
	}

	// Trust nothing above: re-read the provider's own listing and re-verify the
	// installed bytes before mission start is offered.
	row, found, err = missionPluginRow(ctx, codexBin, home)
	if err != nil {
		return err
	}
	if !found || !strings.Contains(row, "installed, enabled") || !pluginRowHasVersion(row) {
		return fmt.Errorf("mission plugin %s is not installed and enabled at version %s after install (row: %q) - repair: remove %s and retry mission start", missionplugin.PluginID, missionplugin.Version, strings.TrimSpace(row), filepath.Join(home, "plugins"))
	}
	return verifyMissionPluginCache(home)
}

// missionPluginRow returns the provider's `plugin list` row for the mission
// plugin. Absence is not an error; a failed listing is.
func missionPluginRow(ctx context.Context, codexBin, home string) (string, bool, error) {
	out, err := runPluginCLI(ctx, codexBin, home, "plugin", "list")
	if err != nil {
		return "", false, fmt.Errorf("list Codex plugins: %w: %s", err, out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), missionplugin.PluginID+" ") {
			return line, true, nil
		}
	}
	return "", false, nil
}

func pluginRowHasVersion(row string) bool {
	for _, field := range strings.Fields(row) {
		if field == missionplugin.Version {
			return true
		}
	}
	return false
}

// missionPluginCacheDir is the provider's install location for the shipped
// plugin version inside one scoped home.
func missionPluginCacheDir(home string) string {
	return filepath.Join(home, "plugins", "cache", missionplugin.MarketplaceName, missionplugin.PluginName, missionplugin.Version)
}

// MissionSkillMDPath is the exact SKILL.md a planning turn must invoke: the
// verified installed artifact, never a user-writable source tree. Callers pass
// it to the harness launch so the mission command's rules govern the turn.
func MissionSkillMDPath(home string) string {
	return filepath.Join(missionPluginCacheDir(home), missionplugin.SkillRelativePath)
}

// verifyMissionPluginCache checks the installed plugin tree against the
// embedded manifest: the provider cache is either byte-identical to what this
// binary ships or mission start stays closed.
func verifyMissionPluginCache(home string) error {
	cacheDir := missionPluginCacheDir(home)
	if err := missionplugin.VerifyInstalled(cacheDir); err != nil {
		return fmt.Errorf("verify the installed mission plugin: %w - repair: remove %s and retry mission start so Kennel reinstalls it", err, filepath.Join(home, "plugins"))
	}
	return nil
}

func runPluginCLI(ctx context.Context, codexBin, home string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, codexBin, args...)
	// Merge, never append: with an ambient CODEX_HOME set, appending yields
	// duplicate keys and which one the provider honors is runtime-dependent,
	// so the CLI could mutate the user's real home. The overlay replaces any
	// inherited value - every invocation observes exactly one scoped home.
	cmd.Env = processenv.Merge(map[string]string{"CODEX_HOME": home})
	out, err := cmd.CombinedOutput()
	return string(out), err
}
