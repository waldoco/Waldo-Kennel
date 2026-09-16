//go:build !windows

package codexappserver

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestKillAppServerProcessTreeReachesDetachedDescendant reproduces, without a
// real Codex install, the failure observed live on Mac: a descendant that
// leaves the app-server's process group (setsid) kept writing to disk for
// seconds after killpg(-pid) returned success. killAppServerProcessTree must
// still reach it by walking the live process table.
func TestKillAppServerProcessTreeReachesDetachedDescendant(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker.log")

	supervisor := exec.Command(os.Args[0], "-test.run=TestHelperProcess_SpawnDetachedWriter")
	supervisor.Env = append(os.Environ(),
		"KENNEL_TEST_HELPER_PROCESS=1",
		"KENNEL_TEST_HELPER_MARKER="+marker,
	)
	configureAppServerProcess(supervisor)
	if err := supervisor.Start(); err != nil {
		t.Fatalf("start supervisor: %v", err)
	}
	t.Cleanup(func() { _ = supervisor.Process.Kill() })

	waitForFileGrowth(t, marker, "", 5*time.Second)

	if err := killAppServerProcessTree(supervisor); err != nil {
		t.Fatalf("killAppServerProcessTree: %v", err)
	}

	before := readFileOrEmpty(t, marker)
	time.Sleep(1500 * time.Millisecond)
	after := readFileOrEmpty(t, marker)
	if before != after {
		t.Fatalf("detached descendant kept writing after kill: before=%q after=%q", before, after)
	}
}

func waitForFileGrowth(t *testing.T, path, baseline string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if content := readFileOrEmpty(t, path); content != baseline && content != "" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s to grow past %q", path, baseline)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func readFileOrEmpty(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// TestHelperProcess_SpawnDetachedWriter is not a real test. It only runs when
// re-exec'd as a standalone process by
// TestKillAppServerProcessTreeReachesDetachedDescendant, standing in for
// Codex's app-server: it starts a child, detaches that child into its own
// session (exactly what live evidence shows a sandboxed command doing on
// Mac), then idles so only killAppServerProcessTree's tree-walk can reach it.
func TestHelperProcess_SpawnDetachedWriter(t *testing.T) {
	if os.Getenv("KENNEL_TEST_HELPER_PROCESS") != "1" {
		t.Skip("re-exec helper only")
	}
	marker := os.Getenv("KENNEL_TEST_HELPER_MARKER")
	child := exec.Command(os.Args[0], "-test.run=TestHelperProcess_DetachedWriter")
	child.Env = append(os.Environ(), "KENNEL_TEST_HELPER_PROCESS=1", "KENNEL_TEST_HELPER_MARKER="+marker)
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Start(); err != nil {
		os.Exit(1)
	}
	_ = child.Wait()
}

// TestHelperProcess_DetachedWriter is not a real test; see
// TestHelperProcess_SpawnDetachedWriter.
func TestHelperProcess_DetachedWriter(t *testing.T) {
	if os.Getenv("KENNEL_TEST_HELPER_PROCESS") != "1" {
		t.Skip("re-exec helper only")
	}
	marker := os.Getenv("KENNEL_TEST_HELPER_MARKER")
	f, err := os.OpenFile(marker, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		os.Exit(1)
	}
	defer f.Close()
	for i := 0; i < 300; i++ {
		if _, err := f.WriteString("tick\n"); err != nil {
			os.Exit(1)
		}
		_ = f.Sync()
		time.Sleep(50 * time.Millisecond)
	}
}
