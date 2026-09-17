package codex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/chatdriver/codexappserver"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

const discoveryTimeout = 5 * time.Second

type DiscoveryOptions struct {
	Timeout      time.Duration
	Now          func() time.Time
	VersionProbe func(context.Context, string) (string, error)
}

// Discover records local executable identity and asks the supplied app-server
// seam for live protocol provenance. It never invokes auth and never reads
// credential stores or copies provider configuration.
func (p *Plugin) Discover(ctx context.Context, protocol ports.ProtocolProvenanceProbe) (ports.HarnessInstallation, error) {
	return p.DiscoverWithOptions(ctx, protocol, DiscoveryOptions{})
}

func (p *Plugin) DiscoverWithOptions(ctx context.Context, protocol ports.ProtocolProvenanceProbe, opts DiscoveryOptions) (ports.HarnessInstallation, error) {
	if err := ctx.Err(); err != nil {
		return ports.HarnessInstallation{}, err
	}
	bin, err := p.ResolveBinary(ctx)
	if err != nil {
		return ports.HarnessInstallation{}, err
	}
	canonical, err := filepath.EvalSymlinks(bin)
	if err != nil {
		return ports.HarnessInstallation{}, fmt.Errorf("canonical Codex executable: %w", err)
	}
	digest, err := fileDigest(ctx, canonical)
	if err != nil {
		return ports.HarnessInstallation{}, err
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = discoveryTimeout
	}
	versionProbe := opts.VersionProbe
	if versionProbe == nil {
		versionProbe = runVersion
	}
	versionCtx, versionCancel := context.WithTimeout(ctx, timeout)
	versionOut, err := versionProbe(versionCtx, canonical)
	versionCancel()
	if err != nil {
		return ports.HarnessInstallation{}, err
	}
	version, ok := codexappserver.ParseCodexVersion(versionOut)
	if !ok {
		return ports.HarnessInstallation{}, fmt.Errorf("invalid Codex version output %q", strings.TrimSpace(versionOut))
	}
	if protocol == nil {
		return ports.HarnessInstallation{}, fmt.Errorf("Codex protocol provenance probe is required")
	}
	protocolCtx, protocolCancel := context.WithTimeout(ctx, timeout)
	prov, err := protocol.ProtocolProvenance(protocolCtx)
	protocolCancel()
	if err != nil {
		return ports.HarnessInstallation{}, err
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return ports.HarnessInstallation{Harness: "codex", ExecutablePath: canonical, ExecutableDigest: digest, Version: version, Source: codexSource(canonical), Protocol: prov, ObservedAt: now().UTC()}, nil
}

func fileDigest(ctx context.Context, path string) (domain.SHA256Digest, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("Codex executable is not a regular file")
	}
	pathInfo, err := os.Stat(path)
	if err != nil || !os.SameFile(info, pathInfo) {
		return "", fmt.Errorf("Codex executable changed during discovery")
	}
	h := sha256.New()
	buf := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, readErr := f.Read(buf)
		if n > 0 {
			if _, err := h.Write(buf[:n]); err != nil {
				return "", err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	return domain.SHA256Digest(hex.EncodeToString(h.Sum(nil))), nil
}

func runVersion(ctx context.Context, path string) (string, error) {
	// Kept local and bounded; --version is metadata only and receives no env or credentials.
	cmd := exec.CommandContext(ctx, path, "--version")
	// Discovery does not pass the daemon environment (including provider or
	// cloud credentials) into the metadata-only version probe.
	cmd.Env = []string{}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func codexSource(path string) string {
	p := filepath.ToSlash(path)
	switch {
	case strings.Contains(p, "/ChatGPT.app/"):
		return "chatgpt_app"
	case strings.Contains(p, "/.npm") || strings.Contains(p, "/node_modules/"):
		return "npm"
	case strings.Contains(p, "/.cargo/"):
		return "cargo"
	case strings.Contains(p, "/opt/homebrew/"):
		return "homebrew"
	default:
		return "path"
	}
}
