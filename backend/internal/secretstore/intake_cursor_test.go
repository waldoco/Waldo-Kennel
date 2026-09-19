package secretstore

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func cursorKeyPath(d string) string { return filepath.Join(d, "secrets", intakeCursorMACKeyFilename) }
func TestLoadOrCreateIntakeCursorMACKeyIsRestartStableAndPrivate(t *testing.T) {
	d := t.TempDir()
	a, e := LoadOrCreateIntakeCursorMACKey(d)
	if e != nil {
		t.Fatal(e)
	}
	b, e := LoadOrCreateIntakeCursorMACKey(d)
	if e != nil || !bytes.Equal(a, b) || len(a) != 32 {
		t.Fatalf("restart key mismatch len=%d err=%v", len(a), e)
	}
	i, e := os.Lstat(cursorKeyPath(d))
	if e != nil {
		t.Fatal(e)
	}
	if !i.Mode().IsRegular() || i.Mode().Perm() != 0600 {
		t.Fatalf("mode=%v", i.Mode())
	}
}
func TestLoadOrCreateIntakeCursorMACKeyRejectsCorruptExistingKey(t *testing.T) {
	d := t.TempDir()
	p := cursorKeyPath(d)
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte("short"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := LoadOrCreateIntakeCursorMACKey(d); e == nil {
		t.Fatal("corrupt key accepted")
	}
}
func TestLoadOrCreateIntakeCursorMACKeyRejectsPublicExistingFile(t *testing.T) {
	d := t.TempDir()
	p := cursorKeyPath(d)
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, bytes.Repeat([]byte{1}, 32), 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := LoadOrCreateIntakeCursorMACKey(d); e == nil {
		t.Fatal("public key accepted")
	}
	i, _ := os.Lstat(p)
	if i.Mode().Perm() != 0644 {
		t.Fatal("suspicious key was repaired")
	}
}
func TestLoadOrCreateIntakeCursorMACKeyRejectsPublicDirectory(t *testing.T) {
	d := t.TempDir()
	dir := filepath.Join(d, "secrets")
	if e := os.MkdirAll(dir, 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(cursorKeyPath(d), bytes.Repeat([]byte{1}, 32), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := LoadOrCreateIntakeCursorMACKey(d); e == nil {
		t.Fatal("public directory accepted")
	}
	i, _ := os.Lstat(dir)
	if i.Mode().Perm() != 0755 {
		t.Fatal("suspicious directory was repaired")
	}
}
func TestLoadOrCreateIntakeCursorMACKeyRejectsSymlink(t *testing.T) {
	d := t.TempDir()
	dir := filepath.Join(d, "secrets")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	target := filepath.Join(d, "target")
	if e := os.WriteFile(target, bytes.Repeat([]byte{1}, 32), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(target, cursorKeyPath(d)); e != nil {
		t.Skipf("symlink unsupported: %v", e)
	}
	if _, e := LoadOrCreateIntakeCursorMACKey(d); e == nil {
		t.Fatal("symlink accepted")
	}
}
