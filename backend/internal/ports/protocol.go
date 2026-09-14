package ports

import "context"

// ChatProtocolProvenance records which provider protocol a Chat driver
// negotiated with the installed build: the build's version, the live method
// surface digest, how that compares to the generated pin the driver was built
// against, and what negotiation changed. It is provenance only — it grants no
// authority and never relaxes a fail-closed gate.
type ChatProtocolProvenance struct {
	// Provider names the transport, e.g. "codex app-server".
	Provider string
	// InstalledVersion is the provider build's own reported version, empty
	// when the version could not be read.
	InstalledVersion string
	// GeneratedFrom names the provider build whose schema produced the
	// checked-in protocol bindings.
	GeneratedFrom string
	// ProtocolDigest fingerprints the live method surface.
	ProtocolDigest string
	// GeneratedDigest is the pin the checked-in bindings carry.
	GeneratedDigest string
	// MatchesGenerated reports whether the live surface and the pin agree.
	// A mismatch is not a failure on its own: new provider methods change the
	// digest without breaking anything Kennel depends on.
	MatchesGenerated bool
	// DegradedCapabilities lists optional capabilities negotiation switched
	// off because the installed build no longer declares their methods.
	DegradedCapabilities []ChatCapability
	// MissingFloor lists required methods the installed build does not
	// declare. Non-empty means the driver refused (or would refuse) work.
	MissingFloor []string
}

// ChatProtocolProvenanceDriver is the optional interface a Chat driver
// implements when it can report negotiated protocol provenance. Callers must
// tolerate drivers that do not implement it.
type ChatProtocolProvenanceDriver interface {
	ProtocolProvenance(ctx context.Context) (ChatProtocolProvenance, error)
}
