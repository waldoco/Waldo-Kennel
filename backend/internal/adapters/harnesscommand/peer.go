package harnesscommand

import (
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/harnesspairing"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// NewLocalPeerVerifier applies the same frozen platform peer-credential check
// as the pairing endpoint. Both endpoints are same-user Unix sockets.
func NewLocalPeerVerifier() ports.LocalPeerVerifier { return harnesspairing.NewLocalPeerVerifier() }
