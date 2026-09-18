package claudecode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

func writeExecutable(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDiscoverFoundationAbsent(t *testing.T) {
	got, err := DiscoverFoundation(context.Background(), DiscoveryOptions{ResolveBinary: func(context.Context) (string, error) {
		return "", ports.ErrAgentBinaryNotFound
	}})
	if err != nil || got.State != CompatibilityAbsent {
		t.Fatalf("got %#v err=%v", got, err)
	}
}

func TestDiscoverFoundationFingerprintsIdentityButRequiresNativeProbes(t *testing.T) {
	binary := writeExecutable(t, t.TempDir(), "claude", "fake-binary")
	command := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "--version":
			return []byte("2.1.198 (Claude Code)\n"), nil
		case "auth status":
			return []byte(`{"loggedIn":true,"authMethod":"oauth"}`), nil
		default:
			return nil, errors.New("unexpected command")
		}
	}
	got, err := DiscoverFoundation(context.Background(), DiscoveryOptions{
		ResolveBinary: func(context.Context) (string, error) { return binary, nil }, Command: command,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.State != CompatibilityNativeProbesNeeded {
		t.Fatalf("state=%s", got.State)
	}
	if got.Fingerprint.Version != "2.1.198" || got.Fingerprint.AuthStatusShape != ProbePassed {
		t.Fatalf("fingerprint=%#v", got.Fingerprint)
	}
	if got.Fingerprint.NativeProbesComplete() {
		t.Fatal("docs-only foundation reported native probes complete")
	}
	if !strings.HasPrefix(got.Fingerprint.ExecutableSHA256, "sha256:") || !strings.HasPrefix(got.Digest, "sha256:") {
		t.Fatalf("digests=%q %q", got.Fingerprint.ExecutableSHA256, got.Digest)
	}
}

func TestDiscoverFoundationClassifiesUnauthenticatedAndUnknownAuth(t *testing.T) {
	binary := writeExecutable(t, t.TempDir(), "claude", "fake")
	for _, tc := range []struct {
		name, auth string
		authErr    error
		want       CompatibilityState
	}{
		{"unauthenticated", `{"loggedIn":false}`, nil, CompatibilityUnauthenticated},
		{"unknown", `not-json`, errors.New("exit 1"), CompatibilityRepairNeeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DiscoverFoundation(context.Background(), DiscoveryOptions{
				ResolveBinary: func(context.Context) (string, error) { return binary, nil },
				Command: func(_ context.Context, _ string, args ...string) ([]byte, error) {
					if args[0] == "--version" {
						return []byte("claude 2.1.198"), nil
					}
					return []byte(tc.auth), tc.authErr
				},
			})
			if err != nil || got.State != tc.want {
				t.Fatalf("got=%#v err=%v", got, err)
			}
		})
	}
}

func TestDiscoverFoundationFailsClosedOnMalformedVersionAndTimeout(t *testing.T) {
	binary := writeExecutable(t, t.TempDir(), "claude", "fake")
	got, err := DiscoverFoundation(context.Background(), DiscoveryOptions{
		ResolveBinary: func(context.Context) (string, error) { return binary, nil },
		Command:       func(context.Context, string, ...string) ([]byte, error) { return []byte("Claude Code unknown"), nil },
	})
	if err != nil || got.State != CompatibilityUnsupported {
		t.Fatalf("got=%#v err=%v", got, err)
	}

	_, err = DiscoverFoundation(context.Background(), DiscoveryOptions{
		ResolveBinary: func(context.Context) (string, error) { return binary, nil }, Timeout: time.Millisecond,
		Command: func(ctx context.Context, _ string, _ ...string) ([]byte, error) { <-ctx.Done(); return nil, ctx.Err() },
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline, got %v", err)
	}
}

func TestCompatibilityFingerprintDigestStableAndClosed(t *testing.T) {
	f := CompatibilityFingerprint{
		SchemaVersion: 1, ExecutablePath: "/bin/claude", ExecutableSHA256: "sha256:abc", Version: "2.1.198", InstallSource: "other",
		AuthStatusShape: ProbeUnknown, StructuredStream: ProbeUnknown, SessionResume: ProbeUnknown,
		ScopedHomeIsolation: ProbeUnknown, PluginMission: ProbeUnknown, StrictMCPIsolation: ProbeUnknown,
		MCPInitializeFailStop: ProbeUnknown, SandboxEnforcement: ProbeUnknown, PermissionPrecedence: ProbeUnknown,
	}
	a, err := f.Digest()
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("digest changed: %q != %q", a, b)
	}
	if f.NativeProbesComplete() {
		t.Fatal("zero-value probes counted as passed")
	}
	if CompatibilityState("connected").Valid() {
		t.Fatal("connected must not be a C1 compatibility state")
	}
	if CompatibilityState("supported").Valid() {
		t.Fatal("supported must not be a C1 compatibility state")
	}
}

func TestScopedConfigDirIsDeterministicAndTraversalSafe(t *testing.T) {
	root := filepath.Join(t.TempDir(), "custody")
	got, err := ScopedConfigDir(root, "../../project", "../sha256:abc", 7)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ScopedConfigDir(root, "../../project", "../sha256:abc", 7)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("not deterministic: %q != %q", got, want)
	}
	rel, err := filepath.Rel(root, got)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatalf("escaped root: %q", got)
	}
	if strings.Contains(got, "..") || strings.Contains(got, "project") {
		t.Fatalf("raw identifier entered path: %q", got)
	}
	if filepath.Base(got) != "generation-7" {
		t.Fatalf("generation missing: %q", got)
	}
}

func TestScopedConfigDirRejectsInvalidAndSymlinkRoot(t *testing.T) {
	if _, err := ScopedConfigDir("relative", "p", "i", 1); err == nil {
		t.Fatal("relative root accepted")
	}
	if _, err := ScopedConfigDir(filepath.Join(t.TempDir(), "r"), "", "i", 1); err == nil {
		t.Fatal("empty project accepted")
	}
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ScopedConfigDir(filepath.Join(link, "custody"), "p", "i", 1); err == nil {
		t.Fatal("symlink root accepted")
	}
}

func TestConfigCanaryDetectsAnyRealConfigChangeWithoutStoringBytes(t *testing.T) {
	home := t.TempDir()
	global := filepath.Join(home, ".claude.json")
	dir := filepath.Join(home, ".claude")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(dir, "settings.json")
	secret := "private-user-config-value"
	if err := os.WriteFile(global, []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := CaptureConfigCanary(global, dir)
	if err != nil {
		t.Fatal(err)
	}
	after, err := CaptureConfigCanary(global, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !before.Equal(after) {
		t.Fatal("unchanged config canary drifted")
	}
	for _, v := range before.Entries {
		if strings.Contains(v, secret) || strings.Contains(v, "theme") {
			t.Fatalf("canary retained config bytes: %q", v)
		}
	}
	if err := os.WriteFile(settings, []byte(`{"theme":"light"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := CaptureConfigCanary(global, dir)
	if err != nil {
		t.Fatal(err)
	}
	if before.Equal(changed) {
		t.Fatal("config mutation was not detected")
	}
}

func TestConfigCanaryMissingPathsStable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	a, err := CaptureConfigCanary(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := CaptureConfigCanary(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) || !a.Equal(b) {
		t.Fatalf("missing canary unstable: %#v %#v", a, b)
	}
}

func TestDiscoverFoundationRejectsNonExecutable(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(binary, []byte("not executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	got, err := DiscoverFoundation(context.Background(), DiscoveryOptions{
		ResolveBinary: func(context.Context) (string, error) { return binary, nil },
		Command:       func(context.Context, string, ...string) ([]byte, error) { called = true; return nil, nil },
	})
	if err != nil || got.State != CompatibilityAbsent {
		t.Fatalf("got=%#v err=%v", got, err)
	}
	if called {
		t.Fatal("ran a non-executable candidate")
	}
}

func TestDiscoverFoundationBindsResolvedSymlinkAndPathChanges(t *testing.T) {
	dir := t.TempDir()
	one := writeExecutable(t, dir, "claude-one", "one")
	two := writeExecutable(t, dir, "claude-two", "two")
	link := filepath.Join(dir, "claude")
	if err := os.Symlink(one, link); err != nil {
		t.Fatal(err)
	}
	command := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "--version" {
			return []byte("Claude Code 2.1.198"), nil
		}
		return []byte(`{"loggedIn":true}`), nil
	}
	discover := func() DiscoveryResult {
		got, err := DiscoverFoundation(context.Background(), DiscoveryOptions{ResolveBinary: func(context.Context) (string, error) { return link, nil }, Command: command})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	first := discover()
	if first.Fingerprint.ExecutablePath != one {
		t.Fatalf("path=%q want=%q", first.Fingerprint.ExecutablePath, one)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(two, link); err != nil {
		t.Fatal(err)
	}
	second := discover()
	if second.Fingerprint.ExecutablePath != two {
		t.Fatalf("path=%q want=%q", second.Fingerprint.ExecutablePath, two)
	}
	if first.Digest == second.Digest || first.Fingerprint.ExecutableSHA256 == second.Fingerprint.ExecutableSHA256 {
		t.Fatal("changed executable retained identity fingerprint")
	}
}

func TestCompatibilityFingerprintCanonicalRoundTripRejectsOpenInput(t *testing.T) {
	f := CompatibilityFingerprint{
		SchemaVersion: 1, ExecutablePath: "/bin/claude", ExecutableSHA256: "sha256:abc",
		Version: "2.1.198", InstallSource: "other", AuthStatusShape: ProbePassed,
		StructuredStream: ProbeUnknown, SessionResume: ProbeUnknown, ScopedHomeIsolation: ProbeUnknown,
		PluginMission: ProbeUnknown, StrictMCPIsolation: ProbeUnknown, MCPInitializeFailStop: ProbeUnknown,
		SandboxEnforcement: ProbeUnknown, PermissionPrecedence: ProbeUnknown,
	}
	data, err := f.MarshalCanonical()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseCompatibilityFingerprint(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, f) {
		t.Fatalf("round trip=%#v want=%#v", got, f)
	}
	if _, err := ParseCompatibilityFingerprint(append(data[:len(data)-1], []byte(`,"connected":true}`)...)); err == nil {
		t.Fatal("unknown/open field accepted")
	}
	f.StructuredStream = ProbeResult("assumed")
	if _, err := f.MarshalCanonical(); err == nil {
		t.Fatal("open probe value accepted")
	}
}

func TestDiscoverFoundationSourceReplaceRestoreDoesNotChangePinnedBehavior(t *testing.T) {
	dir := t.TempDir()
	binary := writeExecutable(t, dir, "claude", "original-binary")
	replacement := writeExecutable(t, dir, "replacement", "replacement-binary")
	originalAside := filepath.Join(dir, "original-aside")
	custody := filepath.Join(t.TempDir(), "custody")
	calls := 0
	got, err := DiscoverFoundation(context.Background(), DiscoveryOptions{
		ResolveBinary: func(context.Context) (string, error) { return binary, nil }, CustodyDir: custody,
		Command: func(_ context.Context, pinned string, args ...string) ([]byte, error) {
			calls++
			if pinned == binary {
				t.Fatal("probe ran mutable source path")
			}
			pinnedBytes, err := os.ReadFile(pinned)
			if err != nil {
				t.Fatal(err)
			}
			if string(pinnedBytes) != "original-binary" {
				t.Fatalf("probe observed %q", pinnedBytes)
			}
			if calls == 1 {
				if err := os.Rename(binary, originalAside); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, binary); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(binary); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(originalAside, binary); err != nil {
					t.Fatal(err)
				}
			}
			// This simulates the output of the exact pinned bytes just inspected,
			// rather than volunteering the expected source version independently.
			if args[0] == "--version" {
				return []byte("Claude Code 2.1.198"), nil
			}
			return []byte(`{"loggedIn":true}`), nil
		},
	})
	if err != nil {
		t.Fatalf("source replace/restore affected pinned discovery: %v", err)
	}
	if got.Fingerprint.ExecutablePath != binary {
		t.Fatalf("source identity=%q want=%q", got.Fingerprint.ExecutablePath, binary)
	}
	if _, err := os.Stat(filepath.Join(custody, "claude-pinned")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pinned copy leaked: %v", err)
	}
}

func TestDiscoverFoundationRejectsPersistentPinnedCopySwap(t *testing.T) {
	dir := t.TempDir()
	binary := writeExecutable(t, dir, "claude", "original-binary")
	replacement := writeExecutable(t, dir, "replacement", "replacement-binary")
	custody := filepath.Join(t.TempDir(), "custody")
	_, err := DiscoverFoundation(context.Background(), DiscoveryOptions{
		ResolveBinary: func(context.Context) (string, error) { return binary, nil }, CustodyDir: custody,
		Command: func(_ context.Context, pinned string, args ...string) ([]byte, error) {
			if args[0] == "--version" {
				if err := os.Rename(replacement, pinned); err != nil {
					t.Fatal(err)
				}
				return []byte("Claude Code 2.1.198"), nil
			}
			return []byte(`{"loggedIn":true}`), nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "executable identity changed") {
		t.Fatalf("persistent pinned-copy swap was not rejected: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(custody, "claude-pinned")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("pinned copy leaked after rejection: %v", statErr)
	}
}

func TestDiscoverFoundationCallerCustodyCleansPinnedCopyOnEveryExit(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command discoveryCommand
		timeout time.Duration
	}{
		{"success", func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if args[0] == "--version" {
				return []byte("Claude Code 2.1.198"), nil
			}
			return []byte(`{"loggedIn":true}`), nil
		}, 0},
		{"unsupported", func(context.Context, string, ...string) ([]byte, error) { return []byte("unknown version"), nil }, 0},
		{"auth error", func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if args[0] == "--version" {
				return []byte("Claude Code 2.1.198"), nil
			}
			return []byte("bad auth"), errors.New("exit 1")
		}, 0},
		{"timeout", func(ctx context.Context, _ string, _ ...string) ([]byte, error) { <-ctx.Done(); return nil, ctx.Err() }, time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			binary := writeExecutable(t, dir, "claude", "binary")
			custody := filepath.Join(t.TempDir(), "caller-custody")
			_, _ = DiscoverFoundation(context.Background(), DiscoveryOptions{ResolveBinary: func(context.Context) (string, error) { return binary, nil }, CustodyDir: custody, Command: tc.command, Timeout: tc.timeout})
			if _, err := os.Stat(filepath.Join(custody, "claude-pinned")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("pinned copy leaked: %v", err)
			}
			if info, err := os.Stat(custody); err != nil || !info.IsDir() {
				t.Fatalf("caller custody was removed: %v", err)
			}
		})
	}
}

func TestDiscoverFoundationCanRepeatInSameCallerCustody(t *testing.T) {
	dir := t.TempDir()
	binary := writeExecutable(t, dir, "claude", "binary")
	custody := filepath.Join(t.TempDir(), "caller-custody")
	command := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "--version" {
			return []byte("Claude Code 2.1.198"), nil
		}
		return []byte(`{"loggedIn":true}`), nil
	}
	for i := 0; i < 2; i++ {
		if _, err := DiscoverFoundation(context.Background(), DiscoveryOptions{ResolveBinary: func(context.Context) (string, error) { return binary, nil }, CustodyDir: custody, Command: command}); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	if _, err := os.Stat(filepath.Join(custody, "claude-pinned")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pinned copy leaked: %v", err)
	}
}

func TestParseCompatibilityFingerprintRejectsTrailingData(t *testing.T) {
	f := CompatibilityFingerprint{
		SchemaVersion: 1, ExecutablePath: "/bin/claude", ExecutableSHA256: "sha256:abc", Version: "2.1.198", InstallSource: "other",
		AuthStatusShape: ProbeUnknown, StructuredStream: ProbeUnknown, SessionResume: ProbeUnknown,
		ScopedHomeIsolation: ProbeUnknown, PluginMission: ProbeUnknown, StrictMCPIsolation: ProbeUnknown,
		MCPInitializeFailStop: ProbeUnknown, SandboxEnforcement: ProbeUnknown, PermissionPrecedence: ProbeUnknown,
	}
	data, err := f.MarshalCanonical()
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{`{"schema_version":1}`, ` garbage`} {
		if _, err := ParseCompatibilityFingerprint(append(append([]byte(nil), data...), []byte(suffix)...)); err == nil {
			t.Fatalf("accepted trailing suffix %q", suffix)
		}
	}
	if _, err := ParseCompatibilityFingerprint(append(append([]byte(nil), data...), []byte(" \n\t")...)); err != nil {
		t.Fatalf("rejected trailing whitespace: %v", err)
	}
}

func TestCompatibilityFingerprintRequiresEveryProbeValue(t *testing.T) {
	f := CompatibilityFingerprint{
		SchemaVersion: 1, ExecutablePath: "/bin/claude", ExecutableSHA256: "sha256:abc", Version: "2.1.198", InstallSource: "other",
		AuthStatusShape: ProbeUnknown, StructuredStream: ProbeUnknown, SessionResume: ProbeUnknown,
		ScopedHomeIsolation: ProbeUnknown, PluginMission: ProbeUnknown, StrictMCPIsolation: ProbeUnknown,
		MCPInitializeFailStop: ProbeUnknown, SandboxEnforcement: ProbeUnknown, PermissionPrecedence: ProbeUnknown,
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("complete fingerprint rejected: %v", err)
	}
	f.PluginMission = ""
	if err := f.Validate(); err == nil {
		t.Fatal("empty probe field accepted")
	}
}

func TestDiscoverFoundationCallerCustodyCollisionPreservesForeignFile(t *testing.T) {
	dir := t.TempDir()
	binary := writeExecutable(t, dir, "claude", "binary")
	custody := filepath.Join(t.TempDir(), "caller-custody")
	if err := os.MkdirAll(custody, 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(custody, "claude-pinned")
	want := []byte("foreign-caller-owned-bytes")
	if err := os.WriteFile(foreign, want, 0o640); err != nil {
		t.Fatal(err)
	}
	_, err := DiscoverFoundation(context.Background(), DiscoveryOptions{
		ResolveBinary: func(context.Context) (string, error) { return binary, nil }, CustodyDir: custody,
	})
	if err == nil {
		t.Fatal("expected exclusive-create collision")
	}
	got, readErr := os.ReadFile(foreign)
	if readErr != nil {
		t.Fatalf("foreign file removed: %v", readErr)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("foreign bytes changed: %q", got)
	}
	info, statErr := os.Stat(foreign)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("foreign mode changed: %o", info.Mode().Perm())
	}
}
