package claudecode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

// CompatibilityState is deliberately closed. C1 cannot report supported or
// connected: native behavioral probes are a later admission gate.
type CompatibilityState string

const (
	CompatibilityAbsent             CompatibilityState = "absent"
	CompatibilityUnauthenticated    CompatibilityState = "unauthenticated"
	CompatibilityUnsupported        CompatibilityState = "unsupported"
	CompatibilityRepairNeeded       CompatibilityState = "repair_needed"
	CompatibilityNativeProbesNeeded CompatibilityState = "native_probes_required"
)

func (s CompatibilityState) Valid() bool {
	switch s {
	case CompatibilityAbsent, CompatibilityUnauthenticated, CompatibilityUnsupported,
		CompatibilityRepairNeeded, CompatibilityNativeProbesNeeded:
		return true
	default:
		return false
	}
}

// ProbeResult records a closed compatibility observation. Unknown is not a
// pass and therefore can never admit governed execution.
type ProbeResult string

const (
	ProbeUnknown ProbeResult = "unknown"
	ProbePassed  ProbeResult = "passed"
	ProbeFailed  ProbeResult = "failed"
)

// CompatibilityFingerprint binds the installation identity and every native
// behavior C1 knows must be proven. Fields stay explicit so adding a required
// behavior changes the canonical digest rather than silently inheriting a pass.
type CompatibilityFingerprint struct {
	SchemaVersion         int         `json:"schema_version"`
	ExecutablePath        string      `json:"executable_path"`
	ExecutableSHA256      string      `json:"executable_sha256"`
	Version               string      `json:"version"`
	InstallSource         string      `json:"install_source"`
	AuthStatusShape       ProbeResult `json:"auth_status_shape"`
	StructuredStream      ProbeResult `json:"structured_stream"`
	SessionResume         ProbeResult `json:"session_resume"`
	ScopedHomeIsolation   ProbeResult `json:"scoped_home_isolation"`
	PluginMission         ProbeResult `json:"plugin_mission"`
	StrictMCPIsolation    ProbeResult `json:"strict_mcp_isolation"`
	MCPInitializeFailStop ProbeResult `json:"mcp_initialize_fail_stop"`
	SandboxEnforcement    ProbeResult `json:"sandbox_enforcement"`
	PermissionPrecedence  ProbeResult `json:"permission_precedence"`
}

func (f CompatibilityFingerprint) Validate() error {
	if f.SchemaVersion != 1 || strings.TrimSpace(f.ExecutablePath) == "" ||
		!strings.HasPrefix(f.ExecutableSHA256, "sha256:") || strings.TrimSpace(f.InstallSource) == "" {
		return errors.New("claude-code: incomplete compatibility fingerprint identity")
	}
	for _, result := range f.probeResults() {
		if result != ProbeUnknown && result != ProbePassed && result != ProbeFailed {
			return errors.New("claude-code: invalid compatibility probe result")
		}
	}
	return nil
}

// MarshalCanonical returns the stable persistence representation used by the
// digest. Loading it still does not make the installation supported.
func (f CompatibilityFingerprint) MarshalCanonical() ([]byte, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(f)
}

func ParseCompatibilityFingerprint(data []byte) (CompatibilityFingerprint, error) {
	var f CompatibilityFingerprint
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&f); err != nil {
		return CompatibilityFingerprint{}, fmt.Errorf("claude-code: parse compatibility fingerprint: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return CompatibilityFingerprint{}, fmt.Errorf("claude-code: trailing compatibility fingerprint data: %w", err)
	}
	if err := f.Validate(); err != nil {
		return CompatibilityFingerprint{}, err
	}
	return f, nil
}

func (f CompatibilityFingerprint) Digest() (string, error) {
	data, err := f.MarshalCanonical()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func (f CompatibilityFingerprint) HasFailedProbe() bool {
	for _, result := range f.probeResults() {
		if result == ProbeFailed {
			return true
		}
	}
	return false
}

func (f CompatibilityFingerprint) NativeProbesComplete() bool {
	for _, result := range f.probeResults() {
		if result != ProbePassed {
			return false
		}
	}
	return true
}

func (f CompatibilityFingerprint) probeResults() []ProbeResult {
	return []ProbeResult{f.AuthStatusShape, f.StructuredStream, f.SessionResume,
		f.ScopedHomeIsolation, f.PluginMission, f.StrictMCPIsolation,
		f.MCPInitializeFailStop, f.SandboxEnforcement, f.PermissionPrecedence}
}

// DiscoveryResult is installation evidence, not a support or connection claim.
type DiscoveryResult struct {
	State       CompatibilityState
	Fingerprint CompatibilityFingerprint
	Digest      string
}

type discoveryCommand func(context.Context, string, ...string) ([]byte, error)

type DiscoveryOptions struct {
	ResolveBinary func(context.Context) (string, error)
	Command       discoveryCommand
	Timeout       time.Duration
	// CustodyDir optionally selects the private directory used for the pinned
	// probe copy. Empty creates and removes a process-private temporary root.
	CustodyDir string
}

// DiscoverFoundation resolves and hashes the actual executable, reads its
// version and auth status, and returns a fingerprint whose behavioral probes
// remain unknown. It never writes provider configuration.
func DiscoverFoundation(ctx context.Context, opts DiscoveryOptions) (DiscoveryResult, error) {
	resolve := opts.ResolveBinary
	if resolve == nil {
		resolve = ResolveClaudeBinary
	}
	sourceBinary, err := resolve(ctx)
	if errors.Is(err, ports.ErrAgentBinaryNotFound) {
		return DiscoveryResult{State: CompatibilityAbsent}, nil
	}
	if err != nil {
		return DiscoveryResult{}, err
	}
	sourceBinary, err = filepath.EvalSymlinks(sourceBinary)
	if err != nil {
		return DiscoveryResult{}, fmt.Errorf("claude-code: resolve executable symlink: %w", err)
	}
	sourceBinary, err = filepath.Abs(sourceBinary)
	if err != nil {
		return DiscoveryResult{}, fmt.Errorf("claude-code: absolute executable path: %w", err)
	}
	info, err := os.Stat(sourceBinary)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return DiscoveryResult{State: CompatibilityAbsent}, nil
	}

	pinnedBinary, cleanup, err := pinExecutableForDiscovery(sourceBinary, opts.CustodyDir)
	if err != nil {
		return DiscoveryResult{}, err
	}
	defer cleanup()
	pinnedInfo, err := os.Stat(pinnedBinary)
	if err != nil {
		return DiscoveryResult{}, fmt.Errorf("claude-code: stat pinned executable: %w", err)
	}
	digest, err := fileSHA256(pinnedBinary)
	if err != nil {
		return DiscoveryResult{}, err
	}
	finalize := func(state CompatibilityState, version string, auth ProbeResult) (DiscoveryResult, error) {
		if err := verifyExecutableIdentity(pinnedBinary, pinnedInfo, digest); err != nil {
			return DiscoveryResult{}, err
		}
		return foundationResult(state, sourceBinary, digest, version, auth)
	}
	run := opts.Command
	if run == nil {
		run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		}
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	versionOut, versionErr := run(probeCtx, pinnedBinary, "--version")
	cancel()
	if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
		return DiscoveryResult{}, fmt.Errorf("claude-code: version probe: %w", probeCtx.Err())
	}
	version := parseClaudeVersion(versionOut)
	if versionErr != nil || version == "" {
		return finalize(CompatibilityUnsupported, version, ProbeUnknown)
	}
	probeCtx, cancel = context.WithTimeout(ctx, timeout)
	authOut, authErr := run(probeCtx, pinnedBinary, "auth", "status")
	cancel()
	if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
		return DiscoveryResult{}, fmt.Errorf("claude-code: auth probe: %w", probeCtx.Err())
	}
	auth, known := claudeAuthStatusFromOutput(authOut)
	if authErr != nil && !known {
		return finalize(CompatibilityRepairNeeded, version, ProbeFailed)
	}
	if known && auth == ports.AgentAuthStatusUnauthorized {
		return finalize(CompatibilityUnauthenticated, version, ProbePassed)
	}
	if !known {
		return finalize(CompatibilityRepairNeeded, version, ProbeFailed)
	}
	return finalize(CompatibilityNativeProbesNeeded, version, ProbePassed)
}

// pinExecutableForDiscovery severs command execution from the mutable source
// pathname. C1 trusts private-custody integrity and executes the pinned path
// immediately; transient replacement inside private custody is outside this
// slice's threat boundary, while persistent replacement is caught by the final
// identity check. Discovery is point-in-time evidence: C2+ admission must
// re-resolve and reverify source identity before every governed execution.
func pinExecutableForDiscovery(source, custodyDir string) (string, func(), error) {
	cleanup := func() {}
	ownsCustody := strings.TrimSpace(custodyDir) == ""
	if ownsCustody {
		var err error
		custodyDir, err = os.MkdirTemp("", "kennel-claude-discovery-")
		if err != nil {
			return "", cleanup, fmt.Errorf("claude-code: create discovery custody: %w", err)
		}
		cleanup = func() { _ = os.RemoveAll(custodyDir) }
	} else {
		if !filepath.IsAbs(custodyDir) {
			return "", cleanup, errors.New("claude-code: discovery custody must be absolute")
		}
		if err := os.MkdirAll(custodyDir, 0o700); err != nil {
			return "", cleanup, fmt.Errorf("claude-code: create discovery custody: %w", err)
		}
	}
	if err := os.Chmod(custodyDir, 0o700); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("claude-code: secure discovery custody: %w", err)
	}
	in, err := os.Open(source)
	if err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("claude-code: open source executable: %w", err)
	}
	defer in.Close()
	pinned := filepath.Join(custodyDir, "claude-pinned")
	out, err := os.OpenFile(pinned, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o500)
	if err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("claude-code: create pinned executable: %w", err)
	}
	if !ownsCustody {
		// Arm caller-custody cleanup only after O_EXCL proves Kennel created
		// this pathname. A colliding foreign file must never be removed.
		cleanup = func() { _ = os.Remove(pinned) }
	}
	_, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("claude-code: copy pinned executable: %v %v %v", copyErr, syncErr, closeErr)
	}
	if err := os.Chmod(pinned, 0o500); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("claude-code: secure pinned executable: %w", err)
	}
	return pinned, cleanup, nil
}

func verifyExecutableIdentity(path string, initial os.FileInfo, initialDigest string) error {
	current, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("claude-code: executable changed during discovery: %w", err)
	}
	if !os.SameFile(initial, current) || !current.Mode().IsRegular() || current.Mode()&0o111 == 0 {
		return errors.New("claude-code: executable identity changed during discovery")
	}
	currentDigest, err := fileSHA256(path)
	if err != nil {
		return fmt.Errorf("claude-code: re-hash executable: %w", err)
	}
	if currentDigest != initialDigest {
		return errors.New("claude-code: executable digest changed during discovery")
	}
	return nil
}

func foundationResult(state CompatibilityState, path, digest, version string, auth ProbeResult) (DiscoveryResult, error) {
	f := CompatibilityFingerprint{
		SchemaVersion: 1, ExecutablePath: path, ExecutableSHA256: digest,
		Version: version, InstallSource: classifyInstallSource(path), AuthStatusShape: auth,
		StructuredStream: ProbeUnknown, SessionResume: ProbeUnknown,
		ScopedHomeIsolation: ProbeUnknown, PluginMission: ProbeUnknown,
		StrictMCPIsolation: ProbeUnknown, MCPInitializeFailStop: ProbeUnknown,
		SandboxEnforcement: ProbeUnknown, PermissionPrecedence: ProbeUnknown,
	}
	fpDigest, err := f.Digest()
	return DiscoveryResult{State: state, Fingerprint: f, Digest: fpDigest}, err
}

func parseClaudeVersion(out []byte) string {
	for _, field := range strings.Fields(string(out)) {
		trimmed := strings.Trim(field, "vV,()")
		parts := strings.Split(trimmed, ".")
		if len(parts) >= 2 {
			valid := true
			for _, part := range parts {
				if _, err := strconv.Atoi(part); err != nil {
					valid = false
					break
				}
			}
			if valid {
				return strings.Join(parts, ".")
			}
		}
	}
	return ""
}

func classifyInstallSource(path string) string {
	p := filepath.ToSlash(strings.ToLower(path))
	switch {
	case strings.Contains(p, "/.local/bin/") || strings.Contains(p, "/.claude/local/"):
		return "native-user"
	case strings.Contains(p, "/node_modules/") || strings.Contains(p, "/.npm") || strings.Contains(p, "/.nvm/") || strings.Contains(p, "/.volta/"):
		return "node-managed"
	case strings.Contains(p, "/homebrew/") || strings.HasPrefix(p, "/usr/local/"):
		return "system-package"
	default:
		return "other"
	}
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// ScopedConfigDir returns a deterministic, non-user Claude home. Identifiers
// are hashed so traversal input never becomes a path component.
func ScopedConfigDir(root, projectID, installationDigest string, generation uint64) (string, error) {
	if !filepath.IsAbs(root) || strings.TrimSpace(projectID) == "" || strings.TrimSpace(installationDigest) == "" || generation == 0 {
		return "", errors.New("claude-code: scoped config requires absolute root, project, installation, and non-zero generation")
	}
	cleanRoot, err := evalSymlinksAllowMissing(filepath.Clean(root))
	if err != nil {
		return "", fmt.Errorf("claude-code: resolve custody root: %w", err)
	}
	project := shortHash(projectID)
	install := shortHash(installationDigest)
	path := filepath.Join(cleanRoot, "claude-code", project, install, fmt.Sprintf("generation-%d", generation))
	rel, err := filepath.Rel(cleanRoot, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("claude-code: scoped config escaped custody root")
	}
	return path, nil
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:16])
}

func evalSymlinksAllowMissing(path string) (string, error) {
	current := path
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		// EvalSymlinks also reports ENOENT for an existing dangling symlink.
		// Append only a component proven absent; any existing component whose
		// identity cannot be resolved is rejected.
		if _, lstatErr := os.Lstat(current); lstatErr == nil {
			return "", fmt.Errorf("unresolved existing custody component: %s", current)
		} else if !errors.Is(lstatErr, os.ErrNotExist) {
			return "", lstatErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

// ConfigCanary captures metadata and content digests only. It never returns or
// persists provider config bytes.
type ConfigCanary struct{ Entries map[string]string }

func CaptureConfigCanary(paths ...string) (ConfigCanary, error) {
	c := ConfigCanary{Entries: map[string]string{}}
	for _, path := range paths {
		path = filepath.Clean(path)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			c.Entries[path] = "absent"
			continue
		}
		if err != nil {
			return ConfigCanary{}, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			c.Entries[path] = "symlink:" + filepath.ToSlash(path)
			continue
		}
		if info.IsDir() {
			err = filepath.WalkDir(path, func(p string, d os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() {
					return nil
				}
				fi, err := os.Lstat(p)
				if err != nil {
					return err
				}
				if fi.Mode()&os.ModeSymlink != 0 {
					c.Entries[p] = "symlink"
					return nil
				}
				digest, err := fileSHA256(p)
				if err != nil {
					return err
				}
				c.Entries[p] = fmt.Sprintf("file:%s:%d:%s", fi.Mode().Perm(), fi.Size(), digest)
				return nil
			})
			if err != nil {
				return ConfigCanary{}, err
			}
			continue
		}
		digest, err := fileSHA256(path)
		if err != nil {
			return ConfigCanary{}, err
		}
		c.Entries[path] = fmt.Sprintf("file:%s:%d:%s", info.Mode().Perm(), info.Size(), digest)
	}
	return c, nil
}

func (c ConfigCanary) Equal(other ConfigCanary) bool {
	if len(c.Entries) != len(other.Entries) {
		return false
	}
	for k, v := range c.Entries {
		if other.Entries[k] != v {
			return false
		}
	}
	return true
}
