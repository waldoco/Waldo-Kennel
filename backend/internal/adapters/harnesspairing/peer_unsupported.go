//go:build !linux && !darwin

package harnesspairing

import (
	"errors"
	"net"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// ErrPeerVerificationUnsupported is returned on any platform without a
// concrete peer-credential check. There is no permissive fallback: every
// peer fails the check, so the transport is unusable rather than unsafe.
var ErrPeerVerificationUnsupported = errors.New("harnesspairing: local peer verification is not implemented on this platform")

func NewLocalPeerVerifier() ports.LocalPeerVerifier { return unsupportedPeerVerifier{} }

type unsupportedPeerVerifier struct{}

func (unsupportedPeerVerifier) VerifyLocalPeer(net.Conn) error {
	return ErrPeerVerificationUnsupported
}
