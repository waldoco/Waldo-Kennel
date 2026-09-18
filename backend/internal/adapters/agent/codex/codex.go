// Package codex implements the Codex agent adapter: launching new sessions,
// resuming hook-tracked sessions, installing workspace-local hooks, and reading
// hook-derived session info.
//
// Kennel-managed sessions derive native session identity and display
// metadata from Codex hooks instead of transcript/cache scans.
package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/agent/agentbase"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/agent/binaryutil"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/agent/terminalui"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/codexpolicy"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	kennelprocess "github.com/Pin4sf/Waldo-Kennel/backend/internal/process"
	"github.com/Pin4sf/Waldo-Kennel/backend/pkg/agentruntime"
)

// Plugin is the Codex agent adapter. It is safe for concurrent use; the binary
// path is resolved fresh at every launch so a re-pinned KENNEL_CODEX_BIN can
// never be frozen out by a stale first resolution.
type Plugin struct {
	agentbase.Base
}

// New returns a ready-to-register Codex adapter.
func New() *Plugin {
	return &Plugin{}
}

// EmitsSubmitActivity signals Codex fires a user-prompt-submit hook under Kennel's
// launch. See ports.SubmitActivitySignaler.
func (p *Plugin) EmitsSubmitActivity() bool { return true }

// EmitsBlockedActivity is false: codex reports permission prompts as
// waiting_input — it installs no post-tool-use hook, so a blocked state could
// never be cleared mid-turn. confirmActive must not nudge it (an Enter could
// answer a pending decision it cannot report as blocked). See
// ports.BlockedActivitySignaler.
func (p *Plugin) EmitsBlockedActivity() bool { return false }

// ExitDetectionMode opts Codex into Kennel's process supervisor. Codex hooks
// expose turn boundaries but no reliable session-end event.
func (p *Plugin) ExitDetectionMode() ports.AgentExitDetectionMode {
	return ports.AgentExitDetectionSupervisor
}

// GovernedCompletionBoundary reports that a governed Codex session's
// completion is bound to its supervised process exit.
func (p *Plugin) GovernedCompletionBoundary() domain.AttemptCompletionBoundary {
	return domain.AttemptCompletionProcessExit
}

// SteersActiveTurn is true: submitting input to the codex TUI mid-turn steers
// the running turn rather than being swallowed or queued, so Kennel may write an
// unsolicited coordination message into an active codex session. See
// ports.ActiveTurnSteerer.
func (p *Plugin) SteersActiveTurn() bool { return true }

var _ adapters.Adapter = (*Plugin)(nil)
var _ ports.Agent = (*Plugin)(nil)

// ValidateExecutionPolicy admits only the two Codex sandbox postures that
// preserve the WorkUnit capability boundary. Native Codex always remains
// sandboxed read-only for governed work; writes and exact checks cross only
// the private Kennel tool boundary. A
// write-only or execute-without-write WorkUnit cannot be represented by Codex's
// sandbox without widening authority, so it is refused before launch.
func (p *Plugin) ValidateExecutionPolicy(ctx context.Context, _ ports.AgentConfig, policy domain.AttemptExecutionPolicy) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := codexpolicy.SandboxFor(policy)
	return err
}

var _ ports.ActiveTurnSteerer = (*Plugin)(nil)
var _ ports.AgentAuthChecker = (*Plugin)(nil)
var _ ports.AgentInterfaceHandoff = (*Plugin)(nil)
var _ ports.AgentInterfaceHandoffHistoryProbe = (*Plugin)(nil)
var _ ports.TerminalActivityDetector = (*Plugin)(nil)
var _ ports.EmptyComposerDetector = (*Plugin)(nil)

// ComposerIsEmpty recognizes Codex's blank composer or its dim placeholder.
// Normal text after the prompt marker is treated as a human draft and causes
// optional semantic handoff collection to fail closed.
func (p *Plugin) ComposerIsEmpty(output string) bool {
	return terminalui.LastPromptIsEmptyOrDimPlaceholder(output, "›")
}

// Manifest returns the adapter's static self-description.
func (p *Plugin) Manifest() adapters.Manifest {
	return adapters.Manifest{
		ID:          "codex",
		Name:        "Codex",
		Description: "Run Codex worker sessions.",
		Version:     "0.0.1",
		Capabilities: []adapters.Capability{
			adapters.CapabilityAgent,
		},
	}
}

// GetConfigSpec reports the per-project agent config keys Codex understands.
func (p *Plugin) GetConfigSpec(ctx context.Context) (ports.ConfigSpec, error) {
	if err := ctx.Err(); err != nil {
		return ports.ConfigSpec{}, err
	}
	return ports.ConfigSpec{
		Fields: []ports.ConfigField{
			{
				Key:         "model",
				Type:        ports.ConfigFieldString,
				Description: "Model override passed to `codex --model`.",
			},
		},
	}, nil
}

// GetLaunchCommand builds the argv to start a new Codex session, applying the
// no-update-check, hook-trust bypass, and approval flags, Kennel's session-flag
// activity hooks, the workspace trust override, optional system-prompt
// instructions, and the initial prompt (passed after `--` so a leading "-" is
// not read as a flag).
func (p *Plugin) GetLaunchCommand(ctx context.Context, cfg ports.LaunchConfig) (cmd []string, err error) {
	binary, err := p.codexBinary(ctx)
	if err != nil {
		return nil, err
	}

	var providerArgs []string
	if err := appendSessionHookFlags(&providerArgs); err != nil {
		return nil, err
	}
	appendTerminalCompatibilityFlags(&providerArgs)
	permission := cfg.Permissions
	var envPrefix []string
	if cfg.ExecutionPolicy != nil {
		if err := p.ValidateExecutionPolicy(ctx, cfg.Config, *cfg.ExecutionPolicy); err != nil {
			return nil, err
		}
		governedEnv, governedArgs, err := governedRepositoryArgs(*cfg.ExecutionPolicy, cfg.WorkspacePath, cfg.DataDir, cfg.SessionID)
		if err != nil {
			return nil, err
		}
		envPrefix = governedEnv
		providerArgs = append(providerArgs, governedArgs...)
		permission = ports.PermissionModeAcceptEdits
	}
	// A governed Attempt is a persistent interactive Codex session, never a
	// one-shot exec: the launch-cut proof steers the same live pane across a
	// daemon restart, and completion plus approved checks reconcile on real
	// session termination (bbcb3607's exit path, re-scoped to session end).
	cmd, err = agentruntime.BuildLaunchCommand(agentruntime.LaunchConfig{
		Harness:          agentruntime.HarnessCodex,
		Binary:           binary,
		WorkspacePath:    cfg.WorkspacePath,
		Model:            cfg.Config.Model,
		Prompt:           cfg.Prompt,
		SystemPrompt:     cfg.SystemPrompt,
		SystemPromptFile: cfg.SystemPromptFile,
		Permission:       agentruntime.PermissionPolicy(permission),
		ProviderArgs:     providerArgs,
	})
	if err != nil {
		return nil, err
	}
	return append(envPrefix, cmd...), nil
}

// GetRestoreCommand rebuilds the argv that continues an existing Codex
// session: `codex resume <agentSessionId>`. ok is false when the hook-derived
// native session id has not landed yet, so callers can fall back to fresh
// launch behavior.
func (p *Plugin) GetRestoreCommand(ctx context.Context, cfg ports.RestoreConfig) (cmd []string, ok bool, err error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if _, ok := agentruntime.RestoreIdentity(
		agentruntime.HarnessCodex,
		cfg.Session.ID,
		cfg.Session.Metadata,
	); !ok {
		return nil, false, nil
	}
	binary, err := p.codexBinary(ctx)
	if err != nil {
		return nil, false, err
	}

	var providerArgs []string
	if err := appendSessionHookFlags(&providerArgs); err != nil {
		return nil, false, err
	}
	appendTerminalCompatibilityFlags(&providerArgs)
	permission := cfg.Permissions
	var envPrefix []string
	if cfg.ExecutionPolicy != nil {
		if err := p.ValidateExecutionPolicy(ctx, cfg.Config, *cfg.ExecutionPolicy); err != nil {
			return nil, false, err
		}
		governedEnv, governedArgs, err := governedRepositoryArgs(*cfg.ExecutionPolicy, cfg.Session.WorkspacePath, cfg.DataDir, cfg.Session.ID)
		if err != nil {
			return nil, false, err
		}
		envPrefix = governedEnv
		providerArgs = append(providerArgs, governedArgs...)
		permission = ports.PermissionModeAcceptEdits
	}
	cmd, ok, err = agentruntime.BuildRestoreCommand(agentruntime.RestoreConfig{
		Harness:          agentruntime.HarnessCodex,
		Binary:           binary,
		SessionID:        cfg.Session.ID,
		Metadata:         cfg.Session.Metadata,
		WorkspacePath:    cfg.Session.WorkspacePath,
		Model:            cfg.Config.Model,
		Prompt:           cfg.Prompt,
		SystemPrompt:     cfg.SystemPrompt,
		SystemPromptFile: cfg.SystemPromptFile,
		Permission:       agentruntime.PermissionPolicy(permission),
		ProviderArgs:     providerArgs,
	})
	if err != nil {
		return nil, false, err
	}
	return append(envPrefix, cmd...), ok, nil
}

func governedRepositoryArgs(policy domain.AttemptExecutionPolicy, workspace, dataDir, sessionID string) (env []string, args []string, err error) {
	sandbox, err := codexpolicy.SandboxFor(policy)
	if err != nil {
		return nil, nil, err
	}
	canonicalWorkspace, err := filepath.EvalSymlinks(filepath.Clean(strings.TrimSpace(workspace)))
	if err != nil || !filepath.IsAbs(canonicalWorkspace) {
		return nil, nil, fmt.Errorf("codex governed repository tools require an existing absolute workspace path")
	}
	if err := policy.ValidateWorkspaceRoot(canonicalWorkspace); err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(sessionID) == "" || !filepath.IsAbs(filepath.Clean(dataDir)) {
		return nil, nil, fmt.Errorf("codex governed repository tools require session identity and an absolute Kennel data directory")
	}
	if sessionID != filepath.Base(sessionID) || sessionID == "." || sessionID == ".." {
		return nil, nil, fmt.Errorf("codex governed repository tools require a path-safe session identity, got %q", sessionID)
	}
	// Approved checks are daemon-owned post-termination work. Do not expose a
	// provider-side executor: it would race the durable Attempt/check/artifact
	// reservation path and could run the same exact check twice.
	home, err := ProvisionGovernedCodexHome(policy, canonicalWorkspace, dataDir, sessionID)
	if err != nil {
		return nil, nil, err
	}
	args = []string{
		"-c", "web_search=\"disabled\"",
	}
	// Feature toggles stay CLI flags: unlike `mcp_servers`, scalar and list
	// overrides apply correctly through `-c`. Native effect tools stay disabled
	// for read-only policies; an exec-capable policy maps to the workspace-write
	// sandbox, under which Codex's confined shell is the blessed way to run the
	// approved local commands - the sandbox, not tool removal, is the boundary.
	disabled := []string{"plugins", "apps", "remote_plugin", "browser_use", "computer_use", "standalone_web_search", "multi_agent"}
	if sandbox != "workspace-write" {
		disabled = append([]string{"shell_tool", "unified_exec"}, disabled...)
	}
	for _, feature := range disabled {
		args = append(args, "--disable", feature)
	}
	return []string{"env", "CODEX_HOME=" + home}, args, nil
}

// ProvisionGovernedCodexHome creates (or verifies, for the same session) the
// session-scoped CODEX_HOME carrying the generated config.toml for a governed
// Attempt and reseeds credentials just-in-time. Exposed so the sandbox
// falsifiers exercise the exact launch provisioning path.
func ProvisionGovernedCodexHome(policy domain.AttemptExecutionPolicy, canonicalWorkspace, dataDir, sessionID string) (string, error) {
	sandbox, err := codexpolicy.SandboxFor(policy)
	if err != nil {
		return "", err
	}
	binary, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve Kennel executable: %w", err)
	}
	raw, err := json.Marshal(policy)
	if err != nil {
		return "", fmt.Errorf("marshal governed repository policy: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	enabledTools := []string{"list_repository", "read_text_file"}
	if policy.Has(domain.CapabilityWorktreeWrite) {
		enabledTools = append(enabledTools, "write_text_file")
	}
	home, err := prepareGovernedHome(dataDir, sessionID)
	if err != nil {
		return "", err
	}
	if err := seedCodexAuth(home); err != nil {
		return "", err
	}
	if err := writeGovernedCodexConfig(home, sandbox, binary, canonicalWorkspace, encoded, filepath.Clean(dataDir), sessionID, enabledTools); err != nil {
		return "", err
	}
	return home, nil
}

// governedHomeDir resolves <dataDir>/codex-home/<sessionID> one component at
// a time, verifying the data root and every existing component is a real
// directory - never a symlink. Checking only the leaf is not enough: a
// planted symlink at codex-home would let MkdirAll write credentials through
// the redirect and let cleanup RemoveAll the target. Missing components are
// created individually (never following links) with mode 0700 when create is
// true; with create false a missing component reports os.IsNotExist. existed
// reports whether the session leaf already was a directory.
func governedHomeDir(dataDir, sessionID string, create bool) (home string, existed bool, err error) {
	if strings.TrimSpace(sessionID) == "" || sessionID != filepath.Base(sessionID) || sessionID == "." || sessionID == ".." {
		return "", false, fmt.Errorf("unsafe session identity %q", sessionID)
	}
	dir := filepath.Clean(dataDir)
	info, err := os.Lstat(dir)
	if err != nil {
		return "", false, fmt.Errorf("inspect Kennel data dir: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", false, fmt.Errorf("Kennel data dir %q is not a plain directory", dir)
	}
	for _, component := range []string{"codex-home", sessionID} {
		dir = filepath.Join(dir, component)
		info, err := os.Lstat(dir)
		switch {
		case os.IsNotExist(err):
			if !create {
				return "", false, err
			}
			if err := os.Mkdir(dir, 0o700); err != nil {
				return "", false, fmt.Errorf("create governed Codex home: %w", err)
			}
			// existed must describe only the session leaf: a freshly created
			// component is not a pre-existing home.
			existed = false
		case err != nil:
			return "", false, fmt.Errorf("inspect governed Codex home: %w", err)
		case info.Mode()&os.ModeSymlink != 0:
			return "", false, fmt.Errorf("governed Codex home component %q must not be a symlink", dir)
		case !info.IsDir():
			return "", false, fmt.Errorf("governed Codex home component %q is not a directory", dir)
		default:
			existed = true
		}
	}
	return dir, existed, nil
}

// prepareGovernedHome exclusively creates the session home, or accepts an
// existing one only when it is verifiably ours (a real directory already
// carrying our config.toml - i.e. the same session re-provisioning). Symlinks
// and foreign paths fail closed.
func prepareGovernedHome(dataDir, sessionID string) (string, error) {
	home, existed, err := governedHomeDir(dataDir, sessionID, true)
	if err != nil {
		return "", err
	}
	if existed {
		if _, err := os.Lstat(filepath.Join(home, "config.toml")); err != nil {
			return "", fmt.Errorf("governed Codex home %q already exists without our config: refusing to adopt a foreign path", home)
		}
	}
	return home, nil
}

// writeFileAtomic writes mode-0600 content via a same-directory temp file and
// rename, so a crashed or raced write never leaves a partial config or
// credential behind.
func writeFileAtomic(path string, content []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return fmt.Errorf("stage %s: %w", filepath.Base(path), err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("protect %s: %w", filepath.Base(path), err)
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("install %s: %w", filepath.Base(path), err)
	}
	return nil
}

// writeGovernedCodexConfig generates the session-scoped config.toml for a
// governed Attempt. The governed MCP server MUST be delivered this way: Codex
// silently drops `mcp_servers` inline-table overrides passed via `-c`
// (https://github.com/openai/codex/issues/16045), which previously left
// governed sessions with no repository tools at all.
func writeGovernedCodexConfig(home, sandbox, binary, canonicalWorkspace, encodedPolicy, dataDir, sessionID string, enabledTools []string) error {
	var b strings.Builder
	b.WriteString("sandbox_mode = " + strconv.Quote(sandbox) + "\n")
	if sandbox == "workspace-write" {
		// Nothing writable outside the worktree, no network: the confined
		// shell can run approved local commands and nothing else.
		b.WriteString("\n[sandbox_workspace_write]\nnetwork_access = false\nwritable_roots = []\n")
	}
	mcpArgs := []string{"governed-tools", "--workspace", canonicalWorkspace, "--policy", encodedPolicy, "--data-dir", dataDir, "--session", sessionID}
	quotedArgs := make([]string, 0, len(mcpArgs))
	for _, arg := range mcpArgs {
		quotedArgs = append(quotedArgs, strconv.Quote(arg))
	}
	quotedTools := make([]string, 0, len(enabledTools))
	for _, name := range enabledTools {
		quotedTools = append(quotedTools, strconv.Quote(name))
	}
	b.WriteString("\n[mcp_servers.kennel_governed]\n")
	b.WriteString("command = " + strconv.Quote(binary) + "\n")
	b.WriteString("args = [" + strings.Join(quotedArgs, ", ") + "]\n")
	b.WriteString("required = true\n")
	b.WriteString("enabled_tools = [" + strings.Join(quotedTools, ", ") + "]\n")
	b.WriteString("default_tools_approval_mode = \"approve\"\n")
	if err := writeFileAtomic(filepath.Join(home, "config.toml"), []byte(b.String())); err != nil {
		return fmt.Errorf("write governed Codex config: %w", err)
	}
	return nil
}

// seedCodexAuth copies the user's Codex credentials into the session-scoped
// home so the governed session can sign in without inheriting the user's
// config.toml (which could carry ungoverned MCP servers or settings). A
// missing login is left for Codex itself to report honestly at launch.
func seedCodexAuth(home string) error {
	source := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	if source == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve Codex auth home: %w", err)
		}
		source = filepath.Join(userHome, ".codex")
	}
	if filepath.Clean(source) == filepath.Clean(home) {
		return nil
	}
	auth, err := os.ReadFile(filepath.Join(source, "auth.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read Codex auth: %w", err)
	}
	// Rotation: Codex refreshes this copy in place inside the session home;
	// refreshed tokens deliberately never flow back into the user's real home.
	// Each new governed session re-seeds from the user's current auth.json.
	if err := writeFileAtomic(filepath.Join(home, "auth.json"), auth); err != nil {
		return fmt.Errorf("seed governed Codex auth: %w", err)
	}
	return nil
}

// CleanSessionHome removes a session-scoped CODEX_HOME when the session is
// durably terminated so its credential copy never outlives the session. It is
// part of the ports.AgentSessionHomeCleaner contract.
func (p *Plugin) CleanSessionHome(dataDir, sessionID string) error {
	home, _, err := governedHomeDir(dataDir, sessionID, false)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.RemoveAll(home); err != nil {
		return fmt.Errorf("remove governed Codex home: %w", err)
	}
	return nil
}

// SessionInfo surfaces Codex hook-derived metadata. Metadata is intentionally
// nil for Codex: callers get the normalized fields directly.
func (p *Plugin) SessionInfo(ctx context.Context, session ports.SessionRef) (ports.SessionInfo, bool, error) {
	if err := ctx.Err(); err != nil {
		return ports.SessionInfo{}, false, err
	}
	info, ok := agentbase.StandardSessionInfo(session)
	return info, ok, nil
}

// NativeConversationID bridges Codex's terminal resume id and app-server thread
// id. Codex uses the same native thread UUID on both surfaces; a TUI source must
// have reported it through its hook before it can switch without losing context.
func (p *Plugin) NativeConversationID(
	ctx context.Context,
	session ports.SessionRef,
	currentMode domain.SessionMode,
	providerConversationID string,
) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if currentMode == domain.SessionModeChat {
		id := strings.TrimSpace(providerConversationID)
		return id, id != "", nil
	}
	id := strings.TrimSpace(session.Metadata[ports.MetadataKeyAgentSessionID])
	return id, id != "", nil
}

// NativeConversationExists distinguishes a Codex thread UUID from a thread
// that app-server can actually resume. Codex returns the UUID from thread/start
// before the first user message materializes a rollout; thread/resume rejects
// that reserved-only UUID with "no rollout found for thread id".
//
// Active rollouts live below CODEX_HOME/sessions. Archived rollouts are
// deliberately excluded because Codex also rejects them from thread/resume.
// We only establish that a non-empty rollout exists; Codex remains responsible
// for parsing its own provider state.
func (p *Plugin) NativeConversationExists(
	ctx context.Context,
	_ ports.SessionRef,
	nativeConversationID string,
	env map[string]string,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	id, valid := canonicalCodexThreadID(nativeConversationID)
	if !valid {
		return false, nil
	}
	codexHome := strings.TrimSpace(env["CODEX_HOME"])
	if codexHome == "" {
		codexHome = strings.TrimSpace(os.Getenv("CODEX_HOME"))
	}
	if codexHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return false, fmt.Errorf("codex: resolve rollout root: %w", err)
		}
		codexHome = filepath.Join(home, ".codex")
	}

	found := false
	sessionsDir := filepath.Join(codexHome, "sessions")
	err := filepath.WalkDir(sessionsDir, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || !codexRolloutNameMatches(entry.Name(), id) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() && info.Size() > 0 {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("codex: inspect rollout root %s: %w", sessionsDir, err)
	}
	return found, nil
}

func canonicalCodexThreadID(value string) (string, bool) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", false
	}
	return parsed.String(), true
}

func codexRolloutNameMatches(name, nativeConversationID string) bool {
	if !strings.HasPrefix(name, "rollout-") {
		return false
	}
	suffix := "-" + nativeConversationID + ".jsonl"
	return strings.HasSuffix(name, suffix) || strings.HasSuffix(name, suffix+".zst")
}

// AuthStatus checks Codex's local login state without making a model call.
func (p *Plugin) AuthStatus(ctx context.Context) (ports.AgentAuthStatus, error) {
	binary, err := p.codexBinary(ctx)
	if err != nil {
		return ports.AgentAuthStatusUnknown, err
	}
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	out, err := kennelprocess.CommandContext(probeCtx, binary, "login", "status").CombinedOutput()
	if probeCtx.Err() != nil {
		return ports.AgentAuthStatusUnknown, probeCtx.Err()
	}
	if status, ok := codexAuthStatusFromOutput(out); ok {
		return status, nil
	}
	// The probe is advisory. Version skew, transient startup failures, and
	// unfamiliar output are not proof that credentials are invalid; the actual
	// launch remains the authoritative check.
	_ = err
	return ports.AgentAuthStatusUnknown, nil
}

func codexAuthStatusFromOutput(out []byte) (ports.AgentAuthStatus, bool) {
	text := strings.ToLower(string(out))
	if strings.Contains(text, "not logged in") || strings.Contains(text, "logged out") {
		return ports.AgentAuthStatusUnauthorized, true
	}
	if strings.Contains(text, "logged in") {
		return ports.AgentAuthStatusAuthorized, true
	}
	return ports.AgentAuthStatusUnknown, false
}

// ResolveCodexBinary returns the path to the codex binary on this machine. An
// explicit KENNEL_CODEX_BIN pin wins over every discovery path; only without a
// pin does it search platform-specific well-known install locations and PATH.
func ResolveCodexBinary(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if pin := strings.TrimSpace(os.Getenv("KENNEL_CODEX_BIN")); pin != "" {
		return resolvePinnedCodexBinary(pin)
	}

	if runtime.GOOS == "windows" {
		candidates := []string{}
		if appData := os.Getenv("APPDATA"); appData != "" {
			shim := filepath.Join(appData, "npm", "codex.cmd")
			candidates = append(candidates, windowsNativeCodexCandidatesForShim(shim)...)
			candidates = append(candidates,
				filepath.Join(appData, "npm", "codex.exe"),
				shim,
			)
		}
		if home, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates, filepath.Join(home, ".cargo", "bin", "codex.exe"))
		}
		for _, candidate := range candidates {
			if fileExists(candidate) {
				return resolveNativeWindowsCodex(candidate), nil
			}
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}

		for _, name := range []string{"codex.cmd", "codex", "codex.exe"} {
			path, err := exec.LookPath(name)
			if err == nil && path != "" {
				if isWindowsAppsCodexExecutable(path) {
					continue
				}
				return resolveNativeWindowsCodex(path), nil
			}
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}

		return "", fmt.Errorf("codex: %w", ports.ErrAgentBinaryNotFound)
	}

	if path, err := exec.LookPath("codex"); err == nil && path != "" {
		return resolveCodexExecutable(path), nil
	}

	candidates := []string{
		"/usr/local/bin/codex",
		"/opt/homebrew/bin/codex",
	}
	// ChatGPT for macOS ships the Codex CLI as an app resource. Finder-launched
	// desktop apps do not inherit another app's PATH additions, so discover the
	// bundled CLI directly just as we do Homebrew and package-manager installs.
	if runtime.GOOS == "darwin" {
		candidates = append(candidates, "/Applications/ChatGPT.app/Contents/Resources/codex")
	}
	if home, err := os.UserHomeDir(); err == nil {
		if runtime.GOOS == "darwin" {
			candidates = append(candidates, filepath.Join(home, "Applications", "ChatGPT.app", "Contents", "Resources", "codex"))
		}
		candidates = append(candidates,
			filepath.Join(home, ".npm-global", "bin", "codex"),
			filepath.Join(home, ".npm", "bin", "codex"),
			filepath.Join(home, ".local", "bin", "codex"),
			filepath.Join(home, ".cargo", "bin", "codex"),
		)
		nodeManagerCandidates, err := binaryutil.UnixNodeManagerBinCandidates(ctx, home, "codex")
		if err != nil {
			return "", err
		}
		candidates = append(candidates, nodeManagerCandidates...)
	}

	for _, candidate := range candidates {
		if fileExists(candidate) {
			return resolveCodexExecutable(candidate), nil
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
	}

	return "", fmt.Errorf("codex: %w", ports.ErrAgentBinaryNotFound)
}

// resolveCodexExecutable preserves the provider's installation boundary when a
// PATH entry is a symlink. Native Codex distributions can ship required
// sidecars beside the real executable and locate them relative to argv[0];
// launching the symlink path would make Codex search beside the shim instead.
// resolvePinnedCodexBinary validates an explicit KENNEL_CODEX_BIN pin. A pin
// that does not resolve to an executable file is a certification break, so it
// fails loudly instead of silently falling back to whatever PATH carries.
func resolvePinnedCodexBinary(pin string) (string, error) {
	info, err := os.Stat(pin)
	if err != nil || info.IsDir() {
		return "", fmt.Errorf("codex: KENNEL_CODEX_BIN %q is not an installed file: %w", pin, ports.ErrAgentBinaryNotFound)
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("codex: KENNEL_CODEX_BIN %q is not executable: %w", pin, ports.ErrAgentBinaryNotFound)
	}
	return resolveCodexExecutable(pin), nil
}

func resolveCodexExecutable(path string) string {
	if runtime.GOOS == "windows" {
		return resolveNativeWindowsCodex(path)
	}
	if evaluated, err := filepath.EvalSymlinks(path); err == nil && evaluated != "" {
		return evaluated
	}
	return path
}

func resolveNativeWindowsCodex(path string) string {
	if runtime.GOOS != "windows" || !strings.EqualFold(filepath.Ext(path), ".cmd") {
		return path
	}
	for _, candidate := range windowsNativeCodexCandidatesForShim(path) {
		if fileExists(candidate) {
			return candidate
		}
	}
	return path
}

func windowsNativeCodexCandidatesForShim(shim string) []string {
	dir := filepath.Dir(shim)
	return []string{
		filepath.Join(dir, "node_modules", "@openai", "codex", "node_modules", "@openai", "codex-win32-x64", "vendor", "x86_64-pc-windows-msvc", "bin", "codex.exe"),
		filepath.Join(dir, "node_modules", "@openai", "codex", "bin", "codex.exe"),
	}
}

func isWindowsAppsCodexExecutable(path string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	clean := strings.ToLower(filepath.Clean(path))
	base := filepath.Base(clean)
	return (base == "codex.exe" || base == "codex") &&
		strings.Contains(clean, string(filepath.Separator)+"windowsapps"+string(filepath.Separator)+"openai.codex_")
}

func (p *Plugin) codexBinary(ctx context.Context) (string, error) {
	return ResolveCodexBinary(ctx)
}

// DoctorLaunchProbes returns argv tails `kennel doctor` runs against the installed
// codex binary to smoke-test the launch surface Kennel's hook delivery depends on.
// Probe 1 confirms --dangerously-bypass-hook-trust still exists (clap rejects
// unknown flags with a non-zero exit even alongside --version). Probe 2 loads
// codex's config with Kennel's `-c` session-flag overrides through the offline
// `features list` subcommand, so an override-parse regression surfaces as a
// non-zero exit or warning output. Both are built from the same flag builders
// the launch command uses, so the probes cannot drift from the real spawn argv.
func DoctorLaunchProbes() [][]string {
	flagProbe := make([]string, 0, 2)
	appendHookTrustBypassFlag(&flagProbe)
	flagProbe = append(flagProbe, "--version")

	overrideProbe := []string{"features", "list"}
	appendNoUpdateCheckFlag(&overrideProbe)
	appendHideRateLimitNudgeFlag(&overrideProbe)
	if err := appendSessionHookFlags(&overrideProbe); err != nil {
		// The probe only asks Codex to parse the hook config; a bare fallback
		// keeps that diagnostic available if the current executable cannot be
		// resolved, while real session launches fail closed above.
		appendSessionHookFlagsForExecutable(&overrideProbe, "kennel")
	}
	appendWorkspaceTrustFlag(&overrideProbe, os.TempDir())
	return [][]string{flagProbe, overrideProbe}
}

func appendNoUpdateCheckFlag(cmd *[]string) {
	*cmd = append(*cmd, "-c", "check_for_update_on_startup=false")
}

func appendHideRateLimitNudgeFlag(cmd *[]string) {
	// When the account nears its rate limit, the Codex TUI interposes an
	// interactive "switch to a cheaper model?" dialog before the first turn.
	// In a headless Kennel pane that dialog hangs the session invisibly and
	// swallows the auto-submitted spawn prompt, so suppress it.
	*cmd = append(*cmd, "-c", "notice.hide_rate_limit_model_nudge=true")
}

func appendHookTrustBypassFlag(cmd *[]string) {
	// Kennel's activity hooks ride the launch command as session-flag config (see
	// appendSessionHookFlags) and carry no persisted trust hash in the user's
	// `[hooks.state]`. Without this flag Codex would hold them for an
	// interactive hooks review, leaving Kennel without activity signals.
	*cmd = append(*cmd, "--dangerously-bypass-hook-trust")
}

func appendTerminalCompatibilityFlags(cmd *[]string) {
	if runtime.GOOS == "windows" {
		*cmd = append(*cmd, "--no-alt-screen")
	}
}

// fileExists is a package var so tests can stub it to scope candidate probing.
var fileExists = func(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
