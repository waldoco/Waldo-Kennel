//go:build !windows

package governedtools

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

const restrictiveUmaskHelper = "KENNEL_RESTRICTIVE_UMASK_HELPER"

func TestRepositoryWritePreservesExecutableModeUnderRestrictiveUmask(t *testing.T) {
	if os.Getenv(restrictiveUmaskHelper) != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestRepositoryWritePreservesExecutableModeUnderRestrictiveUmask$")
		cmd.Env = append(os.Environ(), restrictiveUmaskHelper+"=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("restrictive-umask subprocess: %v: %s", err, output)
		}
		return
	}

	previous := syscall.Umask(0o077)
	defer syscall.Umask(previous)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "verify.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// WriteFile is also umask-filtered, so establish the tracked source mode
	// explicitly before exercising the governed atomic replacement.
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	server := testServer(t, root, true, false)
	if _, err := server.call("write_text_file", map[string]interface{}{
		"path": "verify.sh", "content": "#!/bin/sh\nexit 0\n",
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("mode under umask 077 = %04o, want 0755", got)
	}
}
