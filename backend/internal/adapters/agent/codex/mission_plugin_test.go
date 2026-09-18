package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/missionplugin"
)

// fakeCodex emulates the pinned CLI's plugin surface: marketplaces register in
// a state file, plugin add copies the marketplace's plugin tree into the
// CODEX_HOME cache, list reads the cache back. Every invocation is logged.
func fakeCodex(t *testing.T) (bin string, logPath *string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	script := `#!/usr/bin/env bash
set -euo pipefail
home="$CODEX_HOME"
echo "$*" >> "` + log + `"
state="$home/fake-marketplaces"
case "$1 $2" in
  "plugin marketplace")
    case "${3:-}" in
      list)
        echo "MARKETPLACE  ROOT"
        [ -f "$state" ] && cat "$state" || true
        ;;
      add)
        mkdir -p "$home"
        echo "kennel $4" >> "$state"
        echo "Added marketplace"
        ;;
      remove)
        [ -f "$state" ] && grep -v "^kennel " "$state" > "$state.tmp" || true
        [ -f "$state.tmp" ] && mv "$state.tmp" "$state"
        rm -rf "$home/plugins/cache/kennel"
        echo "Removed marketplace"
        ;;
    esac
    ;;
  "plugin list")
    echo "PLUGIN  STATUS  VERSION  SOURCE"
    cache="$home/plugins/cache/kennel/mission"
    if [ -d "$cache" ]; then
      ver="$(ls "$cache")"
      echo "mission@kennel  installed, enabled  $ver  /somewhere"
    fi
    ;;
  "plugin add")
    root="$(awk '$1=="kennel"{print $2}' "$state")"
    src="$root/plugins/mission"
    ver="$(grep -o '"version": "[^"]*"' "$src/.codex-plugin/plugin.json" | cut -d'"' -f4)"
    dest="$home/plugins/cache/kennel/mission/$ver"
    mkdir -p "$dest"
    cp -R "$src/." "$dest/"
    echo "Added plugin"
    ;;
  "plugin remove")
    rm -rf "$home/plugins/cache/kennel"
    echo "Removed plugin"
    ;;
esac
`
	bin = filepath.Join(dir, "codex")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, &log
}

func readCalls(t *testing.T, logPath string) string {
	t.Helper()
	b, err := os.ReadFile(logPath)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func missionMarket(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "marketplace")
	if err := missionplugin.Materialize(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestEnsureMissionPluginFreshInstall(t *testing.T) {
	bin, log := fakeCodex(t)
	home := filepath.Join(t.TempDir(), "home")
	market := missionMarket(t)
	if err := EnsureMissionPlugin(context.Background(), bin, home, market); err != nil {
		t.Fatal(err)
	}
	calls := readCalls(t, *log)
	for _, want := range []string{"plugin marketplace add " + market, "plugin add mission@kennel"} {
		if !strings.Contains(calls, want) {
			t.Fatalf("expected %q in calls:\n%s", want, calls)
		}
	}
	cache := filepath.Join(home, "plugins", "cache", "kennel", "mission", missionplugin.Version)
	if err := missionplugin.VerifyInstalled(cache); err != nil {
		t.Fatalf("installed cache must verify: %v", err)
	}
}

func TestEnsureMissionPluginIdempotent(t *testing.T) {
	bin, log := fakeCodex(t)
	home := filepath.Join(t.TempDir(), "home")
	market := missionMarket(t)
	ctx := context.Background()
	if err := EnsureMissionPlugin(ctx, bin, home, market); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(*log, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureMissionPlugin(ctx, bin, home, market); err != nil {
		t.Fatal(err)
	}
	if calls := readCalls(t, *log); strings.Contains(calls, "plugin add") || strings.Contains(calls, "marketplace add") {
		t.Fatalf("second run must not reinstall:\n%s", calls)
	}
}

func TestEnsureMissionPluginRepairsTamperedCache(t *testing.T) {
	bin, log := fakeCodex(t)
	home := filepath.Join(t.TempDir(), "home")
	market := missionMarket(t)
	ctx := context.Background()
	if err := EnsureMissionPlugin(ctx, bin, home, market); err != nil {
		t.Fatal(err)
	}
	skill := filepath.Join(home, "plugins", "cache", "kennel", "mission", missionplugin.Version, "skills", "mission", "SKILL.md")
	b, err := os.ReadFile(skill)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skill, append(b, 'x'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(*log, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureMissionPlugin(ctx, bin, home, market); err != nil {
		t.Fatalf("a tampered cache must be repaired, not trusted: %v", err)
	}
	if calls := readCalls(t, *log); !strings.Contains(calls, "plugin add mission@kennel") {
		t.Fatalf("tampered cache must trigger reinstall:\n%s", calls)
	}
	if err := missionplugin.VerifyInstalled(filepath.Join(home, "plugins", "cache", "kennel", "mission", missionplugin.Version)); err != nil {
		t.Fatalf("repaired cache must verify: %v", err)
	}
}

func TestEnsureMissionPluginFailsClosedOnProviderError(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "codex")
	if err := os.WriteFile(bin, []byte("#!/usr/bin/env bash\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := EnsureMissionPlugin(context.Background(), bin, filepath.Join(dir, "home"), missionMarket(t)); err == nil {
		t.Fatal("a failing provider CLI must fail mission start closed")
	} else if !strings.Contains(err.Error(), "marketplace") {
		t.Fatalf("error should name the failed step: %v", err)
	}
}

func TestProvisionMissionCodexHome(t *testing.T) {
	dataDir := t.TempDir()
	authHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(authHome, "auth.json"), []byte(`{"token":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", authHome)
	home, err := ProvisionMissionCodexHome(dataDir, "space-1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(home, filepath.Join("codex-home", "mission-space-1")) {
		t.Fatalf("home = %q", home)
	}
	if _, err := os.Stat(filepath.Join(home, "auth.json")); err != nil {
		t.Fatalf("auth not reseeded: %v", err)
	}
	if _, err := ProvisionMissionCodexHome(dataDir, "../escape"); err == nil {
		t.Fatal("unsafe space key must fail closed")
	}
}
