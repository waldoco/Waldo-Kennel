package codexappserver

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	kennelprocess "github.com/Pin4sf/Waldo-Kennel/backend/internal/process"
)

// protocolSurface is the method vocabulary one installed Codex build declares,
// parsed out of the schema the provider emits about itself. Binary presence and
// even a version number say nothing about this surface: the provider marks
// app-server experimental and has already changed the schema's layout once
// (flat files, then versioned v1/v2 bundles), so the only honest check is to
// ask the installed build what it declares.
type protocolSurface struct {
	// methods maps a method name onto the direction that declares it
	// (ClientRequest, ClientNotification, ServerRequest, ServerNotification).
	// Direction is load-bearing: a method that moved directions is drift, not
	// an alias.
	methods map[string]string
	// digest fingerprints the surface with the generator's exact algorithm, so
	// a live build can be compared against codexproto.ProtocolDigest directly.
	digest string
}

// declares reports whether the provider declared this method in this direction.
func (s protocolSurface) declares(direction, method string) bool {
	return s.methods[method] == direction
}

// digestMethods fingerprints a method surface. It must stay byte-identical to
// the generator's digest in cmd/gencodexproto: sha256 over the sorted
// "Direction name" lines, truncated to 16 hex chars. The pin test in
// negotiate_test.go proves the two have not drifted apart.
func digestMethods(methods map[string]string) string {
	lines := make([]string, 0, len(methods))
	for name, direction := range methods {
		lines = append(lines, direction+" "+name)
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, line := range lines {
		_, _ = fmt.Fprintln(h, line)
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// surfaceProbeFunc reads the protocol surface of one provider binary. Injected
// on Driver so tests never exec anything.
type surfaceProbeFunc func(ctx context.Context, bin string) (protocolSurface, error)

// fetchProtocolSurface asks the installed provider for its own schema and
// parses the method unions out of it. This is a local metadata command: no
// auth, no model call, no provider state created.
func fetchProtocolSurface(ctx context.Context, bin string) (protocolSurface, error) {
	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	dir, err := os.MkdirTemp("", "kennel-codex-surface-")
	if err != nil {
		return protocolSurface{}, fmt.Errorf("stage protocol schema: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	out, err := kennelprocess.CommandContext(fetchCtx, bin, "app-server", "generate-json-schema", "--out", dir).CombinedOutput()
	if err != nil {
		return protocolSurface{}, fmt.Errorf("provider declined to emit its protocol schema (%w): %s", err, strings.TrimSpace(string(out)))
	}
	methods, err := parseProtocolSurface(dir)
	if err != nil {
		return protocolSurface{}, err
	}
	return protocolSurface{methods: methods, digest: digestMethods(methods)}, nil
}

// parseProtocolSurface reads the four JSON-RPC method unions out of an emitted
// schema directory and maps every declared method onto its direction. An empty
// result is an error: a parse that finds nothing proves the schema layout
// changed, which is exactly the drift this exists to surface.
func parseProtocolSurface(dir string) (map[string]string, error) {
	found := map[string]string{}
	for _, file := range []string{
		"ClientRequest.json", "ClientNotification.json",
		"ServerRequest.json", "ServerNotification.json",
	} {
		raw, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			continue
		}
		var doc struct {
			OneOf []struct {
				Properties struct {
					Method struct {
						Enum []string `json:"enum"`
					} `json:"method"`
				} `json:"properties"`
			} `json:"oneOf"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("parse %s: %w", file, err)
		}
		direction := strings.TrimSuffix(file, ".json")
		for _, arm := range doc.OneOf {
			for _, name := range arm.Properties.Method.Enum {
				found[name] = direction
			}
		}
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("no methods parsed out of %s: the provider's schema layout changed", dir)
	}
	return found, nil
}

// liveSurfaceCache memoizes a successful fetch per binary identity. The key
// carries the resolved file's mtime and size so a Codex upgrade underneath a
// long-lived daemon is renegotiated rather than served stale claims. Failures
// are never cached: a transient fetch error must not lock the provider out for
// the daemon's lifetime.
var liveSurfaceCache = struct {
	sync.Mutex
	entries map[string]protocolSurface
}{entries: map[string]protocolSurface{}}

func cachedFetchProtocolSurface(ctx context.Context, bin string) (protocolSurface, error) {
	key := bin
	if st, err := os.Stat(bin); err == nil {
		key = fmt.Sprintf("%s@%d:%d", bin, st.ModTime().UnixNano(), st.Size())
	}

	liveSurfaceCache.Lock()
	cached, ok := liveSurfaceCache.entries[key]
	liveSurfaceCache.Unlock()
	if ok {
		return cached, nil
	}

	surface, err := fetchProtocolSurface(ctx, bin)
	if err != nil {
		return protocolSurface{}, err
	}
	liveSurfaceCache.Lock()
	liveSurfaceCache.entries[key] = surface
	liveSurfaceCache.Unlock()
	return surface, nil
}
