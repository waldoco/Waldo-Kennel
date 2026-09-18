// Package missionplugin embeds the Kennel mission plugin - the versioned
// marketplace + plugin tree that installs the /mission planning command into a
// Codex harness home - and verifies installed copies against it. The embedded
// tree is the single source of truth: Materialize clobbers and rewrites the
// marketplace directory on every call, so the daemon binary is the version.
package missionplugin

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"embed"
)

const (
	// MarketplaceName is the local marketplace the plugin installs from.
	MarketplaceName = "kennel"
	// PluginName is the plugin's name inside the marketplace.
	PluginName = "mission"
	// PluginID is the installed plugin selector Codex reports.
	PluginID = PluginName + "@" + MarketplaceName
	// Version is the plugin version; bumping it forces reinstall at the next
	// provisioning check because the installed cache no longer matches.
	Version = "0.1.0"
	// SkillName is the provider-visible skill name. Codex namespaces plugin
	// skills as <plugin>:<skill>; verified against app-server skills/list on
	// the pinned 0.153.4 binary.
	SkillName = "mission:mission"
)

// pluginSubtree is the embedded path prefix of the plugin itself; everything
// under it is digest-verified after installation.
const pluginSubtree = "plugin/plugins/" + PluginName

//go:embed all:plugin
var tree embed.FS

// Materialize writes the embedded marketplace tree into marketRoot, replacing
// any existing copy. Callers own choosing a Kennel-controlled destination.
func Materialize(marketRoot string) error {
	if strings.TrimSpace(marketRoot) == "" {
		return fmt.Errorf("missionplugin.Materialize: marketRoot is required")
	}
	if err := os.RemoveAll(marketRoot); err != nil {
		return fmt.Errorf("clear mission marketplace dir %q: %w", marketRoot, err)
	}
	return fs.WalkDir(tree, "plugin", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, "plugin")
		rel = strings.TrimPrefix(rel, "/")
		target := marketRoot
		if rel != "" {
			target = filepath.Join(marketRoot, filepath.FromSlash(rel))
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		b, err := tree.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read embedded %q: %w", p, err)
		}
		if err := os.WriteFile(target, b, 0o600); err != nil {
			return fmt.Errorf("write %q: %w", target, err)
		}
		return nil
	})
}

// Manifest returns the sha256 digest of every file in the plugin subtree,
// keyed by slash-separated path relative to the plugin root. It is computed
// from the embedded bytes, so it always describes the running binary's plugin.
func Manifest() (map[string]string, error) {
	manifest := map[string]string{}
	err := fs.WalkDir(tree, pluginSubtree, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := tree.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read embedded %q: %w", p, err)
		}
		sum := sha256.Sum256(b)
		manifest[strings.TrimPrefix(p, pluginSubtree+"/")] = hex.EncodeToString(sum[:])
		return nil
	})
	return manifest, err
}

// VerifyInstalled checks that pluginDir holds exactly the manifest file set
// with matching digests. A missing, extra, or altered file is a verification
// failure naming the file - the installed plugin is either byte-identical to
// what this binary ships or it is not ours.
func VerifyInstalled(pluginDir string) error {
	manifest, err := Manifest()
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	walkErr := filepath.WalkDir(pluginDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(pluginDir, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		want, ok := manifest[rel]
		if !ok {
			return fmt.Errorf("installed mission plugin carries unexpected file %q", rel)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read installed mission plugin file %q: %w", rel, err)
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != want {
			return fmt.Errorf("installed mission plugin file %q digest drifted", rel)
		}
		seen[rel] = true
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	missing := []string{}
	for rel := range manifest {
		if !seen[rel] {
			missing = append(missing, rel)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("installed mission plugin is missing %s", strings.Join(missing, ", "))
	}
	return nil
}
