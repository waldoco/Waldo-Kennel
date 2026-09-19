//go:build linux || darwin

package secretstore

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const IntakeCursorMACKeySize = 32
const intakeCursorMACKeyFilename = "waldo-intake-cursor-hmac-v1"

// LoadOrCreateIntakeCursorMACKey returns the restart-stable signing key. Existing
// state is never repaired: unexpected type, ownership surface, or permissions fail closed.
func LoadOrCreateIntakeCursorMACKey(dataDir string) ([]byte, error) {
	dir := filepath.Join(dataDir, "secrets")
	path := filepath.Join(dir, intakeCursorMACKeyFilename)
	if info, err := os.Lstat(dir); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
			return nil, fmt.Errorf("intake cursor secret directory is not private regular directory")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect intake cursor secret directory: %w", err)
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 {
			return nil, fmt.Errorf("intake cursor MAC key is not a private regular file")
		}
		return readExistingCursorKeyNoFollow(path, info)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect intake cursor MAC key: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create cursor secret directory: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
		return nil, fmt.Errorf("cursor secret directory changed during creation")
	}
	key := make([]byte, IntakeCursorMACKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate intake cursor MAC key: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".waldo-intake-cursor-*")
	if err != nil {
		return nil, fmt.Errorf("create intake cursor MAC key: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return nil, err
	}
	if _, err := tmp.Write(key); err != nil {
		_ = tmp.Close()
		return nil, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	if err := os.Link(tmpPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return LoadOrCreateIntakeCursorMACKey(dataDir)
		}
		return nil, fmt.Errorf("install intake cursor MAC key: %w", err)
	}
	return readExistingCursorKeyNoFollow(path, nil)
}

func readExistingCursorKeyNoFollow(path string, before os.FileInfo) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open intake cursor MAC key without following links: %w", err)
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	after, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat opened intake cursor MAC key: %w", err)
	}
	if !after.Mode().IsRegular() || after.Mode().Perm() != 0o600 {
		return nil, fmt.Errorf("intake cursor MAC key changed type or permissions")
	}
	if before != nil && !os.SameFile(before, after) {
		return nil, fmt.Errorf("intake cursor MAC key changed during open")
	}
	key, err := io.ReadAll(io.LimitReader(f, IntakeCursorMACKeySize+1))
	if err != nil {
		return nil, fmt.Errorf("read intake cursor MAC key: %w", err)
	}
	if len(key) != IntakeCursorMACKeySize {
		return nil, fmt.Errorf("intake cursor MAC key has invalid length %d", len(key))
	}
	return key, nil
}
