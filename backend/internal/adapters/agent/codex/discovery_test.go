package codex

import (
	"context"
	"errors"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

type discoveryProtocol struct {
	provenance ports.ChatProtocolProvenance
	wait       bool
}

func (f discoveryProtocol) ProtocolProvenance(ctx context.Context) (ports.ChatProtocolProvenance, error) {
	if f.wait {
		<-ctx.Done()
		return ports.ChatProtocolProvenance{}, ctx.Err()
	}
	return f.provenance, nil
}
func TestDiscoverCapturesCanonicalNonSecretProvenance(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "codex-real")
	if err := os.WriteFile(target, []byte("#!/bin/sh\necho codex-cli 0.154.0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "codex")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	protocol := ports.ChatProtocolProvenance{Provider: "codex app-server", InstalledVersion: "0.154.0", ProtocolDigest: string(domain.DigestSHA256([]byte("protocol")))}
	got, err := (&Plugin{resolvedBinary: link}).DiscoverWithOptions(context.Background(), discoveryProtocol{provenance: protocol}, DiscoveryOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	canonical, _ := filepath.EvalSymlinks(target)
	if got.ExecutablePath != canonical || got.Version != "0.154.0" || got.ExecutableDigest != domain.DigestSHA256([]byte("#!/bin/sh\necho codex-cli 0.154.0\n")) || got.ObservedAt != now || got.Protocol.ProtocolDigest != protocol.ProtocolDigest {
		t.Fatalf("unexpected: %+v", got)
	}
	if got.Source != "path" {
		t.Fatalf("source=%s", got.Source)
	}
}
func TestDiscoverFailsClosedOnTimeoutAndMalformedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(path, []byte("x"), 0700); err != nil {
		t.Fatal(err)
	}
	p := &Plugin{resolvedBinary: path}
	_, err := p.DiscoverWithOptions(context.Background(), discoveryProtocol{wait: true}, DiscoveryOptions{Timeout: time.Millisecond, VersionProbe: func(context.Context, string) (string, error) { return "codex-cli 0.154.0", nil }})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout=%v", err)
	}
	_, err = p.DiscoverWithOptions(context.Background(), discoveryProtocol{}, DiscoveryOptions{VersionProbe: func(context.Context, string) (string, error) { return "latest", nil }})
	if err == nil {
		t.Fatal("malformed version accepted")
	}
}
func TestFileDigestCancelledAndRejectsDirectory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fileDigest(ctx, "unused"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
	if _, err := fileDigest(context.Background(), t.TempDir()); err == nil {
		t.Fatal("directory accepted")
	}
}
