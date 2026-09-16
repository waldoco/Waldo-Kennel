package harnessmanifest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

func Verify(manifest domain.HarnessAdapterManifest, version string, protocol domain.SHA256Digest, required, optional []domain.HarnessCapabilityClass) domain.HarnessManifestClassification {
	return manifest.Verify(version, protocol, required, optional)
}

// VerifySuppliedArtifact inspects only a caller-supplied local regular file. It
// performs no download, install, replacement, rollback, credential read or network call.
func VerifySuppliedArtifact(ctx context.Context, path string, expected domain.SHA256Digest, currentVersion, suppliedVersion string) (domain.HarnessManifestClassification, error) {
	if !expected.Valid() {
		return domain.ManifestInvalid, domain.ErrHarnessManifestInvalid
	}
	f, err := os.Open(path)
	if err != nil {
		return domain.ManifestInvalid, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return domain.ManifestInvalid, fmt.Errorf("adapter artifact is not a regular file")
	}
	h := sha256.New()
	buf := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return domain.ManifestInvalid, err
		}
		n, readErr := f.Read(buf)
		if n > 0 {
			_, _ = h.Write(buf[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return domain.ManifestInvalid, readErr
		}
	}
	if domain.SHA256Digest(hex.EncodeToString(h.Sum(nil))) != expected {
		return domain.ManifestDigestTamper, nil
	}
	cmp, err := domain.CompareHarnessVersions(suppliedVersion, currentVersion)
	if err != nil {
		return domain.ManifestInvalid, err
	}
	if cmp < 0 {
		return domain.ManifestSuppliedArtifactRollback, nil
	}
	return domain.ManifestKnownCompatible, nil
}
