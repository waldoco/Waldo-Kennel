package missionplugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMaterializeWritesTheEmbeddedTree(t *testing.T) {
	root := filepath.Join(t.TempDir(), "marketplace")
	if err := Materialize(root); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		".agents/plugins/marketplace.json",
		"plugins/mission/.codex-plugin/plugin.json",
		"plugins/mission/skills/mission/SKILL.md",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("materialized tree missing %s: %v", rel, err)
		}
	}
	// The manifest covers exactly the plugin subtree.
	manifest, err := Manifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 2 {
		t.Fatalf("manifest = %v, want the plugin.json and SKILL.md pair", manifest)
	}
	if _, ok := manifest[".codex-plugin/plugin.json"]; !ok {
		t.Fatalf("manifest missing plugin.json: %v", manifest)
	}
	if _, ok := manifest["skills/mission/SKILL.md"]; !ok {
		t.Fatalf("manifest missing SKILL.md: %v", manifest)
	}
}

func TestVerifyInstalledAcceptsByteIdenticalCopy(t *testing.T) {
	root := filepath.Join(t.TempDir(), "marketplace")
	if err := Materialize(root); err != nil {
		t.Fatal(err)
	}
	if err := VerifyInstalled(filepath.Join(root, "plugins", "mission")); err != nil {
		t.Fatalf("fresh materialization must verify: %v", err)
	}
}

func TestVerifyInstalledFailsClosed(t *testing.T) {
	cases := map[string]func(pluginDir string){
		"altered file": func(dir string) {
			p := filepath.Join(dir, "skills", "mission", "SKILL.md")
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, append(b, 'x'), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"missing file": func(dir string) {
			if err := os.Remove(filepath.Join(dir, ".codex-plugin", "plugin.json")); err != nil {
				t.Fatal(err)
			}
		},
		"extra file": func(dir string) {
			extra := filepath.Join(dir, "skills", "mission", "sneaky.md")
			if err := os.WriteFile(extra, []byte("not ours"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, corrupt := range cases {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "marketplace")
			if err := Materialize(root); err != nil {
				t.Fatal(err)
			}
			pluginDir := filepath.Join(root, "plugins", "mission")
			corrupt(pluginDir)
			if err := VerifyInstalled(pluginDir); err == nil {
				t.Fatalf("corrupted install (%s) verified clean", name)
			}
		})
	}
}
