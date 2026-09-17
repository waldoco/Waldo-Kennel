package harnessmanifest

import (
	"context"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifySuppliedArtifact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "adapter")
	body := []byte("artifact")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	digest := domain.DigestSHA256(body)
	if got, err := VerifySuppliedArtifact(context.Background(), path, digest, "1.2.0", "1.3.0"); err != nil || got != domain.ManifestKnownCompatible {
		t.Fatalf("got %s %v", got, err)
	}
	if got, _ := VerifySuppliedArtifact(context.Background(), path, digest, "1.3.0", "1.2.0"); got != domain.ManifestSuppliedArtifactRollback {
		t.Fatalf("got %s", got)
	}
	if got, _ := VerifySuppliedArtifact(context.Background(), path, domain.DigestSHA256([]byte("other")), "1.2.0", "1.2.0"); got != domain.ManifestDigestTamper {
		t.Fatalf("got %s", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := VerifySuppliedArtifact(ctx, path, digest, "1.2.0", "1.2.0"); err == nil {
		t.Fatal("cancel accepted")
	}
}
