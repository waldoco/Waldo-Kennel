//go:build !windows

package harnesspairing

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"syscall"
)

// Darwin's sockaddr_un.sun_path is 104 bytes including the trailing NUL. Keep
// the address at or below 103 bytes so the same path is valid on macOS and
// Linux regardless of the caller's (possibly long, e.g. under a test's
// t.TempDir()) runtime-directory path. This mirrors the exact alias
// technique already proven in internal/browserruntime/listen_unix.go rather
// than inventing a second one.
const maxUnixSocketPathBytes = 103

var runtimeAliasPattern = regexp.MustCompile(`^kennel-pair-(\d+)-[0-9a-f]{16}$`)

// Listen creates the protected local pairing-transport socket: a private
// 0700 directory holding a 0600 socket file, both owned by the current
// process's user. Filesystem permission is the first protection layer; the
// peer-credential check in VerifyLocalPeer is the second, enforced per
// connection by Server before any RPC is served. There is no fallback TCP
// listener on any platform.
func Listen(runtimeDir string) (net.Listener, string, error) {
	aliasRoot := os.TempDir()
	if info, err := os.Stat("/tmp"); err == nil && info.IsDir() {
		aliasRoot = "/tmp"
	}
	return listenUnix(runtimeDir, aliasRoot)
}

func listenUnix(runtimeDir, aliasRoot string) (net.Listener, string, error) {
	runtimeDir, err := filepath.Abs(runtimeDir)
	if err != nil {
		return nil, "", fmt.Errorf("resolve harness pairing runtime directory: %w", err)
	}
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return nil, "", fmt.Errorf("create harness pairing runtime directory: %w", err)
	}
	if err := os.Chmod(runtimeDir, 0o700); err != nil {
		return nil, "", fmt.Errorf("restrict harness pairing runtime directory: %w", err)
	}
	cleanupStaleRuntimeAliases(aliasRoot, runtimeDir, unixProcessAlive)

	aliasPath, err := createRuntimeAlias(aliasRoot, runtimeDir)
	if err != nil {
		return nil, "", err
	}
	sockPath := filepath.Join(aliasPath, "pair.sock")
	if len([]byte(sockPath)) > maxUnixSocketPathBytes {
		_ = os.Remove(aliasPath)
		return nil, "", fmt.Errorf("harness pairing socket path is %d bytes; maximum is %d", len([]byte(sockPath)), maxUnixSocketPathBytes)
	}
	_ = os.Remove(filepath.Join(runtimeDir, "pair.sock"))
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		_ = os.Remove(aliasPath)
		return nil, "", err
	}
	wrapped := &cleanupUnixListener{Listener: ln, socketPath: sockPath, aliasPath: aliasPath}
	if err := os.Chmod(sockPath, 0o600); err != nil {
		_ = wrapped.Close()
		return nil, "", err
	}
	return wrapped, sockPath, nil
}

func createRuntimeAlias(root, runtimeDir string) (string, error) {
	for range 16 {
		random := make([]byte, 8)
		if _, err := rand.Read(random); err != nil {
			return "", fmt.Errorf("generate harness pairing runtime alias: %w", err)
		}
		aliasPath := filepath.Join(root, fmt.Sprintf("kennel-pair-%d-%s", os.Getpid(), hex.EncodeToString(random)))
		if err := os.Symlink(runtimeDir, aliasPath); err == nil {
			return aliasPath, nil
		} else if !os.IsExist(err) {
			return "", fmt.Errorf("create harness pairing runtime alias: %w", err)
		}
	}
	return "", fmt.Errorf("create harness pairing runtime alias: exhausted random names")
}

func cleanupStaleRuntimeAliases(root, runtimeDir string, processAlive func(int) bool) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		match := runtimeAliasPattern.FindStringSubmatch(entry.Name())
		if len(match) != 2 || entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		pid, err := strconv.Atoi(match[1])
		if err != nil || pid <= 0 || processAlive(pid) {
			continue
		}
		aliasPath := filepath.Join(root, entry.Name())
		target, err := os.Readlink(aliasPath)
		if err != nil {
			continue
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(root, target)
		}
		target, err = filepath.Abs(target)
		if err != nil || filepath.Clean(target) != runtimeDir {
			continue
		}
		_ = os.Remove(aliasPath)
	}
}

func unixProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

type cleanupUnixListener struct {
	net.Listener
	socketPath string
	aliasPath  string
	once       sync.Once
}

func (l *cleanupUnixListener) Close() error {
	err := l.Listener.Close()
	l.once.Do(func() {
		_ = os.Remove(l.socketPath)
		_ = os.Remove(l.aliasPath)
	})
	return err
}
