//go:build !windows

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The driver-script bar binds bytes at exec time through placement: the
// script must be immutable to the confined shell. These falsifiers attack
// the placement guard itself.

// A symlinked driverDir pointing INTO the workspace must be rejected even
// though the lexical path escapes it.
func TestDriverPlacementRejectsSymlinkIntoWorkspace(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, "inner"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "driver-link")
	if err := os.Symlink(filepath.Join(workspace, "inner"), link); err != nil {
		t.Fatal(err)
	}
	err := driverDirOutsideWritable(link, []string{workspace})
	if err == nil {
		t.Fatalf("symlinked driver dir %q -> workspace accepted; the laundering attack re-opens", link)
	}
	if !strings.Contains(err.Error(), "inside confined-writable") {
		t.Fatalf("unexpected rejection reason: %v", err)
	}
}

// A symlinked driverDir pointing into the temp root must be rejected: the
// confined shell can write $TMPDIR.
func TestDriverPlacementRejectsSymlinkIntoTmpdir(t *testing.T) {
	inner, err := os.MkdirTemp("", "kennel-driver-inner-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(inner) })
	link := filepath.Join(t.TempDir(), "driver-link")
	if err := os.Symlink(inner, link); err != nil {
		t.Fatal(err)
	}
	if err := driverDirOutsideWritable(link, []string{os.TempDir()}); err == nil {
		t.Fatalf("symlinked driver dir %q -> $TMPDIR accepted; the laundering attack re-opens", link)
	}
}

// A real dir contained in the workspace resolves to containment and must be
// rejected without any symlinks involved.
func TestDriverPlacementRejectsRealDirInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	driver := filepath.Join(workspace, "driver")
	if err := os.MkdirAll(driver, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := driverDirOutsideWritable(driver, []string{workspace}); err == nil {
		t.Fatalf("driver dir %q inside workspace accepted", driver)
	}
}

// A symlinked WORKSPACE must not launder containment either: both sides are
// resolved before the comparison.
func TestDriverPlacementResolvesSymlinkedWorkspace(t *testing.T) {
	root := t.TempDir()
	realWorkspace := filepath.Join(root, "real-workspace")
	driver := filepath.Join(realWorkspace, "driver")
	if err := os.MkdirAll(driver, 0o755); err != nil {
		t.Fatal(err)
	}
	workspaceLink := filepath.Join(root, "workspace-link")
	if err := os.Symlink(realWorkspace, workspaceLink); err != nil {
		t.Fatal(err)
	}
	if err := driverDirOutsideWritable(driver, []string{workspaceLink}); err == nil {
		t.Fatalf("driver dir inside symlinked workspace accepted")
	}
}

// A driver dir that cannot be resolved is rejected: unverifiable placement
// is never accepted.
func TestDriverPlacementRejectsUnresolvableDir(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "does-not-exist")
	if err := driverDirOutsideWritable(missing, []string{filepath.Join(root, "workspace")}); err == nil {
		t.Fatalf("unresolvable driver dir accepted")
	}
}

// Positive control: the production shape - a real dir under $HOME, outside
// the workspace and $TMPDIR - is accepted.
func TestDriverPlacementAcceptsRealDirOutside(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	driver, err := os.MkdirTemp(os.Getenv("HOME"), "kennel-driver-placement-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(driver) })
	if err := driverDirOutsideWritable(driver, []string{workspace, os.TempDir()}); err != nil {
		t.Fatalf("real $HOME driver dir outside writable roots rejected: %v", err)
	}
}
