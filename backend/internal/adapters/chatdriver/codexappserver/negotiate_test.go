package codexappserver

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/chatdriver/codexappserver/codexproto"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// fullTestSurface is the checked-in generated surface as a live-surface fake:
// what a provider build that exactly matches the pin would declare.
func fullTestSurface() protocolSurface {
	methods := map[string]string{}
	for _, m := range codexproto.Methods {
		methods[m.Name] = m.Direction
	}
	return protocolSurface{methods: methods, digest: digestMethods(methods)}
}

// withoutMethods returns a copy of the surface with the named methods removed,
// simulating a provider build that dropped them.
func (s protocolSurface) withoutMethods(names ...string) protocolSurface {
	methods := map[string]string{}
	for name, direction := range s.methods {
		methods[name] = direction
	}
	for _, name := range names {
		delete(methods, name)
	}
	return protocolSurface{methods: methods, digest: digestMethods(methods)}
}

// withMethod adds or moves a method, simulating upstream additions and
// direction drift.
func (s protocolSurface) withMethod(direction, name string) protocolSurface {
	methods := map[string]string{}
	for n, d := range s.methods {
		methods[n] = d
	}
	methods[name] = direction
	return protocolSurface{methods: methods, digest: digestMethods(methods)}
}

// The runtime digest must be the generator's digest: negotiation compares a
// live surface against codexproto.ProtocolDigest, so the two algorithms may
// never drift apart.
func TestRuntimeDigestMatchesTheGeneratedPin(t *testing.T) {
	if got := fullTestSurface().digest; got != codexproto.ProtocolDigest {
		t.Fatalf("runtime digest %q != generated pin %q: the runtime and generator "+
			"digest algorithms disagree", got, codexproto.ProtocolDigest)
	}
}

func TestNegotiateFullSurfaceKeepsEveryCapability(t *testing.T) {
	n := negotiateProtocol(fullTestSurface())
	if len(n.missingFloor) != 0 {
		t.Fatalf("missing floor methods on the full surface: %v", n.missingFloor)
	}
	if len(n.degraded) != 0 {
		t.Fatalf("degraded capabilities on the full surface: %v", n.degraded)
	}
	for capability, enabled := range n.caps {
		if !enabled {
			t.Errorf("capability %s disabled on the full surface", capability)
		}
	}
}

func TestNegotiateMissingOptionalMethodDegradesOnlyThatCapability(t *testing.T) {
	n := negotiateProtocol(fullTestSurface().withoutMethods(codexproto.MethodThreadRollback))
	if err := n.refuse(); err != nil {
		t.Fatalf("refuse() = %v; a missing optional method must not refuse the driver", err)
	}
	if n.caps.Has(ports.ChatCapabilityRollback) {
		t.Error("rollback still advertised after thread/rollback disappeared")
	}
	if len(n.degraded) != 1 || n.degraded[0] != ports.ChatCapabilityRollback {
		t.Errorf("degraded = %v, want exactly [rollback]", n.degraded)
	}
	if !n.caps.Has(ports.ChatCapabilityFork) || !n.caps.Has(ports.ChatCapabilityHistory) {
		t.Error("unrelated history features degraded along with rollback")
	}
}

func TestNegotiateMissingMultiMethodCapabilityDegradesOnce(t *testing.T) {
	n := negotiateProtocol(fullTestSurface().withoutMethods(
		codexproto.MethodAccountRateLimitsRead, codexproto.MethodAccountRateLimitsUpdated))
	if n.caps.Has(ports.ChatCapabilityRateLimits) {
		t.Error("rate limits still advertised with both backing methods gone")
	}
	count := 0
	for _, d := range n.degraded {
		if d == ports.ChatCapabilityRateLimits {
			count++
		}
	}
	if count != 1 {
		t.Errorf("rate_limits appears %d times in degraded %v, want 1", count, n.degraded)
	}
}

func TestNegotiateMissingFloorMethodRefusesAndNamesIt(t *testing.T) {
	n := negotiateProtocol(fullTestSurface().withoutMethods(codexproto.MethodTurnInterrupt))
	err := n.refuse()
	if !errors.Is(err, ports.ErrChatDriverIncompatible) {
		t.Fatalf("refuse() = %v, want ErrChatDriverIncompatible", err)
	}
	if got := err.Error(); !strings.Contains(got, "turn/interrupt") {
		t.Errorf("refusal %q does not name the missing floor method", got)
	}
}

func TestNegotiateToleratesNewProviderMethods(t *testing.T) {
	n := negotiateProtocol(fullTestSurface().withMethod("ClientRequest", "thread/crystalBall"))
	if err := n.refuse(); err != nil {
		t.Fatalf("refuse() = %v; new provider methods are not drift that breaks Kennel", err)
	}
	if len(n.degraded) != 0 {
		t.Errorf("degraded = %v; new provider methods must not degrade anything", n.degraded)
	}
}

func TestNegotiateTreatsAMovedMethodAsMissing(t *testing.T) {
	// turn/steer declared as a notification is not something Kennel can send.
	n := negotiateProtocol(fullTestSurface().
		withoutMethods(codexproto.MethodTurnSteer).
		withMethod("ServerNotification", codexproto.MethodTurnSteer))
	if n.caps.Has(ports.ChatCapabilitySteer) {
		t.Error("steer still advertised after the method moved to a notification direction")
	}
}

func TestParseProtocolSurfaceReadsDirections(t *testing.T) {
	dir := t.TempDir()
	writeUnion := func(file string, names ...string) {
		arms := ""
		for i, name := range names {
			if i > 0 {
				arms += ","
			}
			arms += `{"properties":{"method":{"enum":["` + name + `"]}}}`
		}
		body := `{"oneOf":[` + arms + `]}`
		if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeUnion("ClientRequest.json", "initialize", "thread/start")
	writeUnion("ServerNotification.json", "turn/started")

	methods, err := parseProtocolSurface(dir)
	if err != nil {
		t.Fatal(err)
	}
	surface := protocolSurface{methods: methods}
	if !surface.declares("ClientRequest", "thread/start") {
		t.Error("thread/start not read as a ClientRequest")
	}
	if surface.declares("ServerNotification", "thread/start") {
		t.Error("thread/start misattributed to ServerNotification")
	}
	if !surface.declares("ServerNotification", "turn/started") {
		t.Error("turn/started not read as a ServerNotification")
	}
}

func TestParseProtocolSurfaceRefusesAnUnparseableLayout(t *testing.T) {
	if _, err := parseProtocolSurface(t.TempDir()); err == nil {
		t.Fatal("empty schema dir parsed as an empty surface; a layout change must be loud")
	}
}

// Probe must refuse a build that dropped a floor method before spending even a
// process on it, and must name what disappeared.
func TestProbeFailsClosedWhenLiveSurfaceDropsAFloorMethod(t *testing.T) {
	d, srv := newTestDriver(t)
	d.surfaceProbe = func(context.Context, string) (protocolSurface, error) {
		return fullTestSurface().withoutMethods(codexproto.MethodThreadResume), nil
	}

	_, err := d.Probe(context.Background())
	if !errors.Is(err, ports.ErrChatDriverIncompatible) {
		t.Fatalf("Probe err = %v, want ErrChatDriverIncompatible", err)
	}
	if !strings.Contains(err.Error(), "thread/resume") {
		t.Errorf("refusal %q does not name thread/resume", err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.seen) != 0 {
		t.Errorf("provider process saw %d frames; a drifted build must be refused before spawn", len(srv.seen))
	}
}

// A surface that cannot be read at all is unverifiable, and an unverifiable
// protocol is refused rather than papered over with the static table.
func TestProbeFailsClosedWhenTheSurfaceCannotBeRead(t *testing.T) {
	d, _ := newTestDriver(t)
	d.surfaceProbe = func(context.Context, string) (protocolSurface, error) {
		return protocolSurface{}, errors.New("schema command vanished")
	}

	if _, err := d.Probe(context.Background()); !errors.Is(err, ports.ErrChatDriverIncompatible) {
		t.Fatalf("Probe err = %v, want ErrChatDriverIncompatible", err)
	}
}

// Optional drift degrades the matching feature and nothing else; the session
// still opens and the conversation reports the negotiated set.
func TestNegotiatedCapabilitiesReachProbeStartAndConversation(t *testing.T) {
	d, _ := newTestDriver(t)
	d.surfaceProbe = func(context.Context, string) (protocolSurface, error) {
		return fullTestSurface().withoutMethods(codexproto.MethodTurnSteer), nil
	}

	caps, err := d.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if caps.Has(ports.ChatCapabilitySteer) {
		t.Error("Probe still advertises steer after turn/steer disappeared")
	}
	if missing := ports.MissingProductionCapabilities(caps); len(missing) != 0 {
		t.Fatalf("floor broke over an optional method: %v", missing)
	}

	// The scripted server's pipes do not survive Probe's close, so Start gets
	// its own driver with the same drifted surface.
	d2, _ := newTestDriver(t)
	d2.surfaceProbe = d.surfaceProbe
	conv, err := d2.Start(context.Background(), ports.ChatStartConfig{
		SessionID:     "s-1",
		WorkspacePath: t.TempDir(),
		Permissions:   ports.PermissionModeDefault,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = conv.Close() }()
	if conv.Capabilities().Has(ports.ChatCapabilitySteer) {
		t.Error("conversation still advertises steer after turn/steer disappeared")
	}
	if !conv.Capabilities().Has(ports.ChatCapabilityStreaming) {
		t.Error("conversation lost an unrelated capability to steer's drift")
	}
}

// Floor drift at Start refuses before a provider thread exists, even when the
// caller skipped Probe.
func TestStartRefusesFloorDriftBeforeOpeningAThread(t *testing.T) {
	d, srv := newTestDriver(t)
	d.surfaceProbe = func(context.Context, string) (protocolSurface, error) {
		return fullTestSurface().withoutMethods(codexproto.MethodThreadStart), nil
	}

	_, err := d.Start(context.Background(), ports.ChatStartConfig{
		SessionID:     "s-1",
		WorkspacePath: t.TempDir(),
		Permissions:   ports.PermissionModeDefault,
	})
	if !errors.Is(err, ports.ErrChatDriverIncompatible) {
		t.Fatalf("Start err = %v, want ErrChatDriverIncompatible", err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	for _, f := range srv.seen {
		if f.Method == "thread/start" {
			t.Error("thread/start reached a provider build that does not declare it")
		}
	}
}

func TestProtocolProvenanceReportsTheNegotiatedSurface(t *testing.T) {
	d, _ := newTestDriver(t)

	provenance, err := d.ProtocolProvenance(context.Background())
	if err != nil {
		t.Fatalf("ProtocolProvenance: %v", err)
	}
	if provenance.InstalledVersion != "0.153.4" {
		t.Errorf("InstalledVersion = %q, want 0.153.4", provenance.InstalledVersion)
	}
	if !provenance.MatchesGenerated {
		t.Error("full surface should match the generated pin")
	}
	if provenance.ProtocolDigest != codexproto.ProtocolDigest {
		t.Errorf("ProtocolDigest = %q, want the pin %q", provenance.ProtocolDigest, codexproto.ProtocolDigest)
	}
	if len(provenance.DegradedCapabilities) != 0 || len(provenance.MissingFloor) != 0 {
		t.Errorf("full surface reported degraded=%v missing=%v", provenance.DegradedCapabilities, provenance.MissingFloor)
	}

	d.surfaceProbe = func(context.Context, string) (protocolSurface, error) {
		return fullTestSurface().withoutMethods(codexproto.MethodTurnSteer), nil
	}
	provenance, err = d.ProtocolProvenance(context.Background())
	if err != nil {
		t.Fatalf("ProtocolProvenance with drift: %v", err)
	}
	if provenance.MatchesGenerated {
		t.Error("drifted surface reported as matching the pin")
	}
	if len(provenance.DegradedCapabilities) != 1 || provenance.DegradedCapabilities[0] != ports.ChatCapabilitySteer {
		t.Errorf("DegradedCapabilities = %v, want [steer]", provenance.DegradedCapabilities)
	}
}

// TestFetchProtocolSurfaceReadsTheInstalledProvider exercises the real fetch:
// spawn the provider's schema command, parse, digest. Skipped without a codex
// install, like the other provider-dependent tests.
func TestFetchProtocolSurfaceReadsTheInstalledProvider(t *testing.T) {
	bin, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("codex not installed; nothing to fetch a live surface from")
	}

	surface, err := fetchProtocolSurface(context.Background(), bin)
	if err != nil {
		t.Fatalf("fetchProtocolSurface: %v", err)
	}
	if len(surface.methods) == 0 {
		t.Fatal("live surface is empty")
	}
	// The live build must still declare the whole floor; anything else fails
	// here before it can fail a user's session.
	n := negotiateProtocol(surface)
	if err := n.refuse(); err != nil {
		t.Fatalf("installed provider fails the floor: %v", err)
	}
	if len(n.degraded) != 0 {
		t.Logf("installed provider degrades optional capabilities: %v", n.degraded)
	}
	t.Logf("live surface: %d methods, digest %s (pin %s)", len(surface.methods), surface.digest, codexproto.ProtocolDigest)
}
