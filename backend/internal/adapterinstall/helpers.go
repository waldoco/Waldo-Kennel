package adapterinstall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func validateRequest(r Request) error {
	if strings.TrimSpace(r.OperationID) == "" || filepath.Base(r.OperationID) != r.OperationID || r.Now.IsZero() {
		return domain.ErrHarnessAdapterReleaseInvalid
	}
	if err := r.Release.Validate(r.Now); err != nil {
		return err
	}
	if r.Release.OS != runtime.GOOS || r.Release.Architecture != runtime.GOARCH {
		return fmt.Errorf("release target mismatch")
	}
	if err := r.Manifest.ValidateShape(); err != nil {
		return err
	}
	return nil
}
func requestDigest(r Request) (domain.SHA256Digest, error) {
	v := struct {
		ID, Artifact, CurrentVersion string
		Release                      domain.TrustedHarnessAdapterRelease
		Manifest                     domain.HarnessAdapterManifest
		CurrentSequence              uint64
		Expected                     domain.SHA256Digest
		Required, Optional           []domain.HarnessCapabilityClass
	}{r.OperationID, r.ArtifactPath, r.CurrentVersion, r.Release, r.Manifest, r.CurrentSequence, r.ExpectedActive, r.Required, r.Optional}
	b, e := json.Marshal(v)
	if e != nil {
		return "", e
	}
	return domain.DigestSHA256(b), nil
}
func copyContext(ctx context.Context, w io.Writer, r io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var n int64
	for {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		k, e := r.Read(buf)
		if k > 0 {
			m, we := w.Write(buf[:k])
			n += int64(m)
			if we != nil {
				return n, we
			}
			if m != k {
				return n, io.ErrShortWrite
			}
		}
		if e == io.EOF {
			return n, nil
		}
		if e != nil {
			return n, e
		}
	}
}
func fileDigest(ctx context.Context, p string) (domain.SHA256Digest, int64, error) {
	f, e := os.Open(p)
	if e != nil {
		return "", 0, e
	}
	defer f.Close()
	i, e := f.Stat()
	if e != nil || !i.Mode().IsRegular() {
		return "", 0, fmt.Errorf("artifact is not regular")
	}
	h := sha256.New()
	n, e := copyContext(ctx, h, f)
	if e != nil {
		return "", n, e
	}
	return domain.SHA256Digest(hex.EncodeToString(h.Sum(nil))), n, nil
}
func syncDir(p string) error {
	f, e := os.Open(p)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func mapManifest(c domain.HarnessManifestClassification) domain.HarnessAdapterDrift {
	switch c {
	case domain.ManifestOptionalDegradation:
		return domain.AdapterDriftOptionalDegradation
	case domain.ManifestRequiredMissing:
		return domain.AdapterDriftRequiredCapabilityMissing
	case domain.ManifestProtocolDrift:
		return domain.AdapterDriftProtocol
	case domain.ManifestDigestTamper:
		return domain.AdapterDriftArtifactTamper
	case domain.ManifestSuppliedArtifactRollback:
		return domain.AdapterDriftRollbackAttempt
	default:
		return domain.AdapterDriftMixedGeneration
	}
}
func classifyError(e error) domain.HarnessAdapterDrift {
	if e == domain.ErrHarnessAdapterPairingUnavailable {
		return domain.AdapterDriftActivationIncomplete
	}
	s := e.Error()
	if strings.Contains(s, "digest mismatch") {
		return domain.AdapterDriftArtifactTamper
	}
	if strings.Contains(s, "expired") {
		return domain.AdapterDriftMetadataExpired
	}
	return domain.AdapterDriftActivationIncomplete
}
func repairFor(d domain.HarnessAdapterDrift) domain.HarnessAdapterInstallRepair {
	if d == domain.AdapterDriftInSync {
		return ""
	}
	switch d {
	case domain.AdapterDriftUpgradeAvailable:
		return domain.AdapterRepairStageUpgrade
	case domain.AdapterDriftOptionalDegradation:
		return domain.AdapterRepairReviewDegraded
	case domain.AdapterDriftRequiredCapabilityMissing, domain.AdapterDriftProtocol:
		return domain.AdapterRepairKeepCurrent
	case domain.AdapterDriftArtifactTamper:
		return domain.AdapterRepairQuarantineRestore
	case domain.AdapterDriftLocalModification:
		return domain.AdapterRepairReviewLocal
	case domain.AdapterDriftSource:
		return domain.AdapterRepairReviewSource
	case domain.AdapterDriftRollbackAttempt, domain.AdapterDriftMixedGeneration:
		return domain.AdapterRepairRejectCandidate
	case domain.AdapterDriftMetadataExpired:
		return domain.AdapterRepairRefreshMetadata
	case domain.AdapterDriftRollbackFailed:
		return domain.AdapterRepairManual
	default:
		return domain.AdapterRepairReconcileRollback
	}
}
