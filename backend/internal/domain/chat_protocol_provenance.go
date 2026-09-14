package domain

import "time"

// ChatProtocolProvenance is one persisted protocol-negotiation episode for a
// Chat session: which provider build the driver talked to, how the live
// method surface digested against the generated pin, and what negotiation
// changed (ADR 0016). Episodes are append-only per session - a renegotiation
// after a provider upgrade must never rewrite the evidence an older Attempt
// was reviewed against.
//
// It is provenance only: it grants no authority, and a missing or failed
// record changes nothing about what a session may do.
type ChatProtocolProvenance struct {
	// SessionID is plain text with no foreign key, per ruling D6: provenance
	// is review evidence and must outlive session-row GC.
	SessionID string
	// Seq is the episode's 1-based position in the session's negotiation
	// history, assigned by the store inside the write transaction.
	Seq int64
	// Harness is the Kennel harness whose driver negotiated (e.g. codex).
	Harness AgentHarness
	// Provider names the transport, e.g. "codex app-server".
	Provider string
	// InstalledVersion is the provider build's own reported version, empty
	// when it could not be read.
	InstalledVersion string
	// GeneratedFrom names the provider build whose schema produced the
	// checked-in protocol bindings.
	GeneratedFrom string
	// ProtocolDigest fingerprints the live negotiated method surface.
	ProtocolDigest string
	// GeneratedDigest is the pin the checked-in bindings carry.
	GeneratedDigest string
	// MatchesGenerated reports whether live surface and pin agree. A mismatch
	// is not a failure: new provider methods change the digest without
	// breaking anything Kennel depends on.
	MatchesGenerated bool
	// DegradedCapabilities lists optional capabilities negotiation switched
	// off because the installed build no longer declares their methods.
	DegradedCapabilities []string
	// MissingFloor lists required methods the installed build did not
	// declare. Non-empty means the driver refused (or would refuse) work.
	MissingFloor []string
	// NegotiatedAt is when this episode was recorded.
	NegotiatedAt time.Time
}
