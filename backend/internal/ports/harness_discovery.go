package ports

import (
	"context"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// HarnessInstallation is non-secret provenance captured from one local
// installation and one live protocol probe.
type HarnessInstallation struct {
	Harness          string
	ExecutablePath   string
	ExecutableDigest domain.SHA256Digest
	Version          string
	Source           string
	Protocol         ChatProtocolProvenance
	ObservedAt       time.Time
}

type HarnessDiscovery interface {
	Discover(context.Context, ProtocolProvenanceProbe) (HarnessInstallation, error)
}

// ProtocolProvenanceProbe is deliberately read-only and injectable for
// fixtures; implementations must not read or return credentials.
type ProtocolProvenanceProbe interface {
	ProtocolProvenance(context.Context) (ChatProtocolProvenance, error)
}
