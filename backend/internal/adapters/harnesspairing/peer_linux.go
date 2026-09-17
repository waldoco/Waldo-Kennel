//go:build linux

package harnesspairing

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// NewLocalPeerVerifier returns the platform identity/permission check: the
// connected peer must be a genuine Unix-domain socket client running as the
// same OS user as this process (SO_PEERCRED). This is enforced in addition
// to, not instead of, the 0700/0600 filesystem permissions Listen sets up.
func NewLocalPeerVerifier() ports.LocalPeerVerifier { return localPeerVerifier{} }

type localPeerVerifier struct{}

func (localPeerVerifier) VerifyLocalPeer(conn net.Conn) error {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("harnesspairing: peer connection is not a unix socket (%T)", conn)
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return fmt.Errorf("harnesspairing: inspect peer connection: %w", err)
	}
	var ucred *unix.Ucred
	var sockErr error
	if err := raw.Control(func(fd uintptr) {
		ucred, sockErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return fmt.Errorf("harnesspairing: read peer credentials: %w", err)
	}
	if sockErr != nil {
		return fmt.Errorf("harnesspairing: SO_PEERCRED: %w", sockErr)
	}
	if int(ucred.Uid) != os.Getuid() {
		return fmt.Errorf("harnesspairing: peer uid %d does not match local user %d", ucred.Uid, os.Getuid())
	}
	return nil
}
