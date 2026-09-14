package codexappserver

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/chatdriver/codexappserver/codexproto"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// chatFloorSurface is the method vocabulary a Codex build must declare before
// Kennel lets it carry a session that can mutate a workspace. It backs the
// production capability floor (streaming, approvals, interrupt, resume) plus
// the session lifecycle and the smoke reads Probe performs. A build missing
// any of these is refused before a session row, worktree, or provider thread
// exists: a turn blocked on an approval shape nobody declared presents as a
// session that silently stopped.
var chatFloorSurface = []struct {
	direction string
	method    string
}{
	{"ClientRequest", codexproto.MethodInitialize},
	{"ClientRequest", codexproto.MethodThreadStart},
	{"ClientRequest", codexproto.MethodThreadResume},
	{"ClientRequest", codexproto.MethodTurnStart},
	{"ClientRequest", codexproto.MethodTurnInterrupt},
	{"ClientRequest", codexproto.MethodModelList},
	{"ServerNotification", codexproto.MethodTurnStarted},
	{"ServerNotification", codexproto.MethodTurnCompleted},
	{"ServerNotification", codexproto.MethodItemStarted},
	{"ServerNotification", codexproto.MethodItemCompleted},
	{"ServerNotification", codexproto.MethodItemAgentMessageDelta},
	{"ServerRequest", codexproto.MethodItemCommandExecutionRequestApproval},
	{"ServerRequest", codexproto.MethodItemFileChangeRequestApproval},
	{"ServerRequest", codexproto.MethodItemPermissionsRequestApproval},
}

// optionalCapabilitySurface maps every non-floor capability onto the methods
// that back it. When an installed build drops one of these, that capability —
// and only that capability — turns off: the UI reads the negotiated set and
// hides the feature instead of offering a button that fails at runtime. New
// provider methods change nothing here; Kennel does not have to use everything
// a build declares.
var optionalCapabilitySurface = []struct {
	capability ports.ChatCapability
	direction  string
	method     string
}{
	{ports.ChatCapabilityTools, "ServerRequest", codexproto.MethodItemToolCall},
	{ports.ChatCapabilitySteer, "ClientRequest", codexproto.MethodTurnSteer},
	{ports.ChatCapabilityHistory, "ClientRequest", codexproto.MethodThreadRead},
	{ports.ChatCapabilityUsage, "ServerNotification", codexproto.MethodThreadTokenUsageUpdated},
	{ports.ChatCapabilityDiffs, "ServerNotification", codexproto.MethodTurnDiffUpdated},
	{ports.ChatCapabilityPlans, "ServerNotification", codexproto.MethodTurnPlanUpdated},
	{ports.ChatCapabilityCompaction, "ClientRequest", codexproto.MethodThreadCompactStart},
	{ports.ChatCapabilityRollback, "ClientRequest", codexproto.MethodThreadRollback},
	{ports.ChatCapabilityFork, "ClientRequest", codexproto.MethodThreadFork},
	{ports.ChatCapabilityRename, "ClientRequest", codexproto.MethodThreadNameSet},
	{ports.ChatCapabilitySkills, "ClientRequest", codexproto.MethodSkillsList},
	{ports.ChatCapabilityRateLimits, "ClientRequest", codexproto.MethodAccountRateLimitsRead},
	{ports.ChatCapabilityRateLimits, "ServerNotification", codexproto.MethodAccountRateLimitsUpdated},
	{ports.ChatCapabilityInteractive, "ServerRequest", codexproto.MethodItemToolRequestUserInput},
	{ports.ChatCapabilityMCPReload, "ClientRequest", codexproto.MethodConfigMcpServerReload},
	{ports.ChatCapabilityMCPReload, "ClientRequest", codexproto.MethodMcpServerStatusList},
}

// negotiation is the outcome of comparing one installed build's declared
// surface against what Kennel needs.
type negotiation struct {
	// caps is the capability set honest for this build: the static table with
	// every degraded entry switched off.
	caps ports.ChatCapabilities
	// degraded lists capabilities switched off because their backing methods
	// are absent, in deterministic order.
	degraded []ports.ChatCapability
	// missingFloor lists floor methods the build does not declare. Non-empty
	// refuses the driver before any side effect.
	missingFloor []string
	surface      protocolSurface
}

// negotiateProtocol compares a live surface against Kennel's requirements.
// The generated pin (codexproto.ProtocolDigest) is one input to this, not a
// lock: a matching digest short-circuits nothing, and a differing digest is
// fine as long as everything Kennel depends on is still declared.
func negotiateProtocol(surface protocolSurface) negotiation {
	caps := capabilities()
	n := negotiation{caps: caps, surface: surface}

	for _, req := range chatFloorSurface {
		if !surface.declares(req.direction, req.method) {
			n.missingFloor = append(n.missingFloor, req.method)
		}
	}

	seen := map[ports.ChatCapability]bool{}
	for _, opt := range optionalCapabilitySurface {
		if surface.declares(opt.direction, opt.method) {
			continue
		}
		caps[opt.capability] = false
		if !seen[opt.capability] {
			seen[opt.capability] = true
			n.degraded = append(n.degraded, opt.capability)
		}
	}
	sort.Slice(n.degraded, func(i, j int) bool { return n.degraded[i] < n.degraded[j] })
	return n
}

// refuse describes a floor failure as an incompatibility that names exactly
// what the installed build no longer declares.
func (n negotiation) refuse() error {
	if len(n.missingFloor) == 0 {
		return nil
	}
	return fmt.Errorf("%w: Codex %s does not declare required protocol method(s) %s",
		ports.ErrChatDriverIncompatible, n.surface.digest, strings.Join(n.missingFloor, ", "))
}

// negotiate fetches (or reads the cached fetch of) the installed build's
// surface and evaluates it. Any failure is closed: Kennel refuses to claim a
// protocol it could not verify rather than advertising the static table.
func (d *Driver) negotiate(ctx context.Context, bin string) (negotiation, error) {
	probe := d.surfaceProbe
	if probe == nil {
		probe = cachedFetchProtocolSurface
	}
	surface, err := probe(ctx, bin)
	if err != nil {
		return negotiation{}, fmt.Errorf("%w: verify installed Codex protocol surface: %w",
			ports.ErrChatDriverIncompatible, err)
	}
	n := negotiateProtocol(surface)
	if err := n.refuse(); err != nil {
		return negotiation{}, err
	}
	return n, nil
}

// ProtocolProvenance reports which protocol the installed build actually
// speaks: its version, the live surface digest, how that compares to the
// generated pin, and what negotiation switched off. It is provenance only and
// grants nothing.
func (d *Driver) ProtocolProvenance(ctx context.Context) (ports.ChatProtocolProvenance, error) {
	provenance := ports.ChatProtocolProvenance{
		Provider:        "codex app-server",
		GeneratedFrom:   codexproto.ProviderVersion,
		GeneratedDigest: codexproto.ProtocolDigest,
	}
	if d == nil || d.plugin == nil {
		return provenance, fmt.Errorf("%w: Codex app-server plugin is unavailable", ports.ErrChatDriverUnavailable)
	}
	bin, err := d.plugin.ResolveBinary(ctx)
	if err != nil {
		return provenance, fmt.Errorf("%w: %w", ports.ErrChatDriverUnavailable, err)
	}
	versionProbe := d.versionProbe
	if versionProbe == nil {
		versionProbe = installedCodexVersion
	}
	versionCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	versionOutput, err := versionProbe(versionCtx, bin)
	cancel()
	if err == nil {
		if installed, ok := parseCodexVersion(versionOutput); ok {
			provenance.InstalledVersion = installed.String()
		}
	}
	probe := d.surfaceProbe
	if probe == nil {
		probe = cachedFetchProtocolSurface
	}
	surface, err := probe(ctx, bin)
	if err != nil {
		return provenance, fmt.Errorf("%w: verify installed Codex protocol surface: %w",
			ports.ErrChatDriverIncompatible, err)
	}
	n := negotiateProtocol(surface)
	provenance.ProtocolDigest = surface.digest
	provenance.MatchesGenerated = surface.digest == codexproto.ProtocolDigest
	provenance.DegradedCapabilities = n.degraded
	provenance.MissingFloor = n.missingFloor
	return provenance, nil
}
