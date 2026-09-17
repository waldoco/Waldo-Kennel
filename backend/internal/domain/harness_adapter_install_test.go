package domain

import (
	"runtime"
	"testing"
	"time"
)

func TestTrustedHarnessAdapterReleaseValidate(t *testing.T) {
	now := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	r := TrustedHarnessAdapterRelease{TrustRootID: "root", AdapterID: "codex", Version: "1.2.3", Sequence: 2, ArtifactDigest: DigestSHA256([]byte("a")), ManifestDigest: DigestSHA256([]byte("m")), OS: runtime.GOOS, Architecture: runtime.GOARCH, Size: 1, ExpiresAt: now.Add(time.Hour)}
	if err := r.Validate(now); err != nil || !r.MatchesRuntime() {
		t.Fatalf("valid release err=%v match=%v", err, r.MatchesRuntime())
	}
	r.ExpiresAt = now
	if err := r.Validate(now); err == nil {
		t.Fatal("expired release accepted")
	}
}
