//go:build !windows

package harnesscommand

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestListenCreatesPrivateSocketCleansUpAndRestarts(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sock")
	check := func() (net.Listener, string) {
		ln, address, err := Listen(dir)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("directory mode = %o, want 0700", got)
		}
		info, err = os.Lstat(address)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("socket mode = %o, want 0600", got)
		}
		return ln, address
	}
	ln, first := check()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(first); !os.IsNotExist(err) {
		t.Fatalf("socket remains after close: %v", err)
	}
	ln, second := check()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(second); !os.IsNotExist(err) {
		t.Fatalf("socket remains after restart close: %v", err)
	}
}
