package adapterinstall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/harnessmanifest"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/harnessdiscovery"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

const artifactName = "adapter"

type Request struct {
	OperationID     string
	ArtifactPath    string
	Release         domain.TrustedHarnessAdapterRelease
	Manifest        domain.HarnessAdapterManifest
	CurrentVersion  string
	CurrentSequence uint64
	ExpectedActive  domain.SHA256Digest
	Required        []domain.HarnessCapabilityClass
	Optional        []domain.HarnessCapabilityClass
	Now             time.Time
}

type Stager struct {
	Root     string
	Quiescer ports.HarnessAdapterQuiescer
	Health   ports.HarnessAdapterHealthProbe
	Pairing  ports.HarnessAdapterPairingActivator
	mu       sync.Mutex
}

func (s *Stager) Install(ctx context.Context, req Request) (domain.HarnessAdapterInstallOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return domain.HarnessAdapterInstallOperation{}, err
	}
	digest, err := requestDigest(req)
	if err != nil {
		return domain.HarnessAdapterInstallOperation{}, err
	}
	if existing, ok, err := s.readOperation(req.OperationID); err != nil {
		return domain.HarnessAdapterInstallOperation{}, err
	} else if ok {
		if existing.RequestDigest != digest {
			return existing, domain.ErrHarnessAdapterInstallConflict
		}
		if existing.State == domain.AdapterInstallActivatedPendingHealth {
			if rollbackErr := s.rollback(existing.PreviousPath); rollbackErr != nil {
				existing.State = domain.AdapterInstallActionNeeded
				existing.Drift = domain.AdapterDriftRollbackFailed
				existing.Repair = domain.AdapterRepairManual
				existing.Failure = rollbackErr.Error()
			} else {
				existing.State = domain.AdapterInstallRolledBack
				existing.Drift = domain.AdapterDriftActivationIncomplete
				existing.Repair = domain.AdapterRepairReconcileRollback
				existing.Failure = "recovered activation pending health after restart"
			}
			existing.UpdatedAt = time.Now().UTC()
			if persistErr := s.persist(&existing); persistErr != nil {
				return existing, persistErr
			}
		}
		return existing, nil
	}
	if s.Pairing == nil {
		s.Pairing = ports.UnavailableHarnessAdapterPairing{}
	}
	if err := validateRequest(req); err != nil {
		return domain.HarnessAdapterInstallOperation{}, err
	}
	if err := os.MkdirAll(filepath.Join(s.Root, "operations"), 0700); err != nil {
		return domain.HarnessAdapterInstallOperation{}, err
	}
	lease, err := s.acquireLease(req.OperationID)
	if err != nil {
		return domain.HarnessAdapterInstallOperation{}, err
	}
	defer os.Remove(lease)

	op := domain.HarnessAdapterInstallOperation{ID: req.OperationID, RequestDigest: digest, State: domain.AdapterInstallIdle, CandidateDigest: req.Release.ArtifactDigest, PreviousDigest: req.ExpectedActive, ReleaseSequence: req.Release.Sequence, CreatedAt: req.Now.UTC(), UpdatedAt: req.Now.UTC()}
	if err := s.persist(&op); err != nil {
		return op, err
	}

	candidate, err := s.prepare(ctx, req)
	if err != nil {
		return s.fail(&op, classifyError(err), err)
	}
	op.CandidatePath = candidate
	op.State = domain.AdapterInstallPrepared
	if err = s.persist(&op); err != nil {
		return op, err
	}

	class, err := s.verify(ctx, req, candidate)
	if err != nil {
		return s.fail(&op, class, err)
	}
	if class != domain.AdapterDriftInSync && class != domain.AdapterDriftUpgradeAvailable {
		return s.fail(&op, class, fmt.Errorf("candidate blocked: %s", class))
	}
	op.Drift = class
	op.State = domain.AdapterInstallVerified
	if err = s.persist(&op); err != nil {
		return op, err
	}

	previous, err := s.activeTargetAndDigest(ctx)
	if err != nil {
		return s.fail(&op, domain.AdapterDriftActivationIncomplete, err)
	}
	if req.ExpectedActive.IsZero() {
		if previous.digest.Valid() {
			return s.fail(&op, domain.AdapterDriftLocalModification, fmt.Errorf("expected empty installation"))
		}
	} else if previous.digest != req.ExpectedActive {
		return s.fail(&op, domain.AdapterDriftLocalModification, fmt.Errorf("active generation changed"))
	}
	op.PreviousPath = previous.path
	if previous.digest.Valid() {
		op.PreviousDigest = previous.digest
	}
	if s.Quiescer == nil {
		return s.fail(&op, domain.AdapterDriftActivationIncomplete, ports.ErrHarnessAdapterNotQuiescent)
	}
	if err = s.Quiescer.QuiesceHarnessAdapter(ctx, previous.digest); err != nil {
		return s.fail(&op, domain.AdapterDriftActivationIncomplete, err)
	}
	if previous.path != "" {
		if err = s.swapLinkNamed("last-known-good", previous.path); err != nil {
			return s.fail(&op, domain.AdapterDriftActivationIncomplete, err)
		}
	}
	if err = s.activate(candidate); err != nil {
		return s.fail(&op, domain.AdapterDriftActivationIncomplete, err)
	}
	op.State = domain.AdapterInstallActivatedPendingHealth
	if err = s.persist(&op); err != nil {
		_ = s.rollback(previous.path)
		return op, err
	}

	if err = s.healthAndPair(ctx, req, candidate); err != nil {
		if rb := s.rollback(previous.path); rb != nil {
			return s.fail(&op, domain.AdapterDriftRollbackFailed, fmt.Errorf("health: %v; rollback: %w", err, rb))
		}
		op.State = domain.AdapterInstallRolledBack
		op.Failure = err.Error()
		op.Drift = classifyError(err)
		op.Repair = repairFor(op.Drift)
		op.UpdatedAt = time.Now().UTC()
		if persistErr := s.persist(&op); persistErr != nil {
			return op, persistErr
		}
		return op, err
	}
	if err = s.swapLinkNamed("last-known-good", candidate); err != nil {
		if rb := s.rollback(previous.path); rb != nil {
			return s.fail(&op, domain.AdapterDriftRollbackFailed, fmt.Errorf("last-known-good: %v; rollback: %w", err, rb))
		}
		return s.fail(&op, domain.AdapterDriftActivationIncomplete, err)
	}
	op.State = domain.AdapterInstallCommitted
	op.Drift = domain.AdapterDriftInSync
	op.Repair = ""
	op.UpdatedAt = time.Now().UTC()
	return op, s.persist(&op)
}

type active struct {
	path   string
	digest domain.SHA256Digest
}

func (s *Stager) activeTargetAndDigest(ctx context.Context) (active, error) {
	link := filepath.Join(s.Root, "active")
	target, err := os.Readlink(link)
	if errors.Is(err, os.ErrNotExist) {
		return active{}, nil
	}
	if err != nil {
		return active{}, err
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(s.Root, target)
	}
	d, _, err := fileDigest(ctx, filepath.Join(target, artifactName))
	return active{path: target, digest: d}, err
}
func (s *Stager) prepare(ctx context.Context, req Request) (string, error) {
	dir := filepath.Join(s.Root, "generations", fmt.Sprintf("%020d-%s", req.Release.Sequence, req.Release.ArtifactDigest))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, artifactName+".partial")
	in, err := os.Open(req.ArtifactPath)
	if err != nil {
		return "", err
	}
	defer in.Close()
	info, err := in.Stat()
	pathInfo, pathErr := os.Lstat(req.ArtifactPath)
	if err != nil || pathErr != nil || !info.Mode().IsRegular() || pathInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, pathInfo) {
		return "", fmt.Errorf("candidate is not one stable regular file")
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if errors.Is(err, os.ErrExist) {
		_ = os.Remove(dst)
		out, err = os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	}
	if err != nil {
		return "", err
	}
	h := sha256.New()
	n, copyErr := copyContext(ctx, io.MultiWriter(out, h), in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if syncErr != nil {
		return "", syncErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if n != req.Release.Size {
		return "", fmt.Errorf("candidate size mismatch")
	}
	if domain.SHA256Digest(hex.EncodeToString(h.Sum(nil))) != req.Release.ArtifactDigest {
		return "", fmt.Errorf("candidate digest mismatch")
	}
	final := filepath.Join(dir, artifactName)
	if err = os.Rename(dst, final); err != nil {
		return "", err
	}
	if err = os.Chmod(final, 0500); err != nil {
		return "", err
	}
	if err = syncDir(dir); err != nil {
		return "", err
	}
	return dir, nil
}
func (s *Stager) verify(ctx context.Context, req Request, candidate string) (domain.HarnessAdapterDrift, error) {
	if req.Release.AdapterID != req.Manifest.AdapterID || req.Release.ManifestDigest != req.Manifest.Digest || req.Release.ArtifactDigest != req.Manifest.AdapterDigest {
		return domain.AdapterDriftMixedGeneration, fmt.Errorf("release, manifest and artifact are not one generation")
	}
	if req.Release.Sequence <= req.CurrentSequence {
		return domain.AdapterDriftRollbackAttempt, fmt.Errorf("release sequence is not newer")
	}
	class, err := harnessmanifest.VerifySuppliedArtifact(ctx, filepath.Join(candidate, artifactName), req.Release.ArtifactDigest, req.CurrentVersion, req.Release.Version)
	if err != nil {
		return mapManifest(class), err
	}
	if class != domain.ManifestKnownCompatible {
		return mapManifest(class), fmt.Errorf("artifact verification: %s", class)
	}
	class = req.Manifest.Verify(req.Release.Version, req.Manifest.ProtocolFingerprints[0], req.Required, req.Optional)
	if class != domain.ManifestKnownCompatible {
		return mapManifest(class), fmt.Errorf("manifest verification: %s", class)
	}
	if req.ExpectedActive.IsZero() {
		return domain.AdapterDriftInSync, nil
	}
	return domain.AdapterDriftUpgradeAvailable, nil
}
func (s *Stager) healthAndPair(ctx context.Context, req Request, candidate string) error {
	if s.Health == nil {
		return fmt.Errorf("health probe unavailable")
	}
	got, err := s.Health.ProbeHarnessAdapter(ctx, filepath.Join(candidate, artifactName))
	if err != nil {
		return err
	}
	if got.ExecutableDigest != req.Release.ArtifactDigest || got.Version != req.Release.Version {
		return fmt.Errorf("health identity mismatch")
	}
	class := harnessdiscovery.Classify(req.Manifest, got, req.Required, req.Optional)
	if class != domain.ManifestKnownCompatible {
		return fmt.Errorf("health compatibility: %s", class)
	}
	return s.Pairing.ActivateHarnessAdapterPairing(ctx, got)
}
func (s *Stager) activate(candidate string) error { return s.swapLink(candidate) }
func (s *Stager) rollback(previous string) error {
	if previous == "" {
		if err := os.Remove(filepath.Join(s.Root, "active")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return syncDir(s.Root)
	}
	return s.swapLink(previous)
}
func (s *Stager) swapLink(target string) error { return s.swapLinkNamed("active", target) }
func (s *Stager) swapLinkNamed(name, target string) error {
	if err := os.MkdirAll(s.Root, 0700); err != nil {
		return err
	}
	tmp := filepath.Join(s.Root, "."+name+"-next")
	_ = os.Remove(tmp)
	rel, err := filepath.Rel(s.Root, target)
	if err != nil {
		return err
	}
	if err = os.Symlink(rel, tmp); err != nil {
		return err
	}
	if err = os.Rename(tmp, filepath.Join(s.Root, name)); err != nil {
		return err
	}
	return syncDir(s.Root)
}
func (s *Stager) acquireLease(id string) (string, error) {
	if err := os.MkdirAll(s.Root, 0700); err != nil {
		return "", err
	}
	p := filepath.Join(s.Root, "install.lease")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if errors.Is(err, os.ErrExist) {
		return "", domain.ErrHarnessAdapterInstallBusy
	}
	if err != nil {
		return "", err
	}
	_, _ = f.WriteString(id)
	_ = f.Sync()
	_ = f.Close()
	return p, nil
}
func (s *Stager) operationPath(id string) string {
	return filepath.Join(s.Root, "operations", id+".json")
}
func (s *Stager) readOperation(id string) (domain.HarnessAdapterInstallOperation, bool, error) {
	if strings.TrimSpace(id) == "" || filepath.Base(id) != id {
		return domain.HarnessAdapterInstallOperation{}, false, domain.ErrHarnessAdapterReleaseInvalid
	}
	b, err := os.ReadFile(s.operationPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return domain.HarnessAdapterInstallOperation{}, false, nil
	}
	if err != nil {
		return domain.HarnessAdapterInstallOperation{}, false, err
	}
	var op domain.HarnessAdapterInstallOperation
	err = json.Unmarshal(b, &op)
	return op, true, err
}
func (s *Stager) persist(op *domain.HarnessAdapterInstallOperation) error {
	if err := os.MkdirAll(filepath.Join(s.Root, "operations"), 0700); err != nil {
		return err
	}
	op.UpdatedAt = op.UpdatedAt.UTC()
	b, err := json.Marshal(op)
	if err != nil {
		return err
	}
	p := s.operationPath(op.ID)
	tmp := p + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmp, p); err != nil {
		return err
	}
	return syncDir(filepath.Dir(p))
}
func (s *Stager) fail(op *domain.HarnessAdapterInstallOperation, d domain.HarnessAdapterDrift, err error) (domain.HarnessAdapterInstallOperation, error) {
	op.State = domain.AdapterInstallActionNeeded
	op.Drift = d
	op.Repair = repairFor(d)
	op.Failure = err.Error()
	op.UpdatedAt = time.Now().UTC()
	if pe := s.persist(op); pe != nil {
		return *op, pe
	}
	return *op, err
}
