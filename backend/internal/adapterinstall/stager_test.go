package adapterinstall

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

type quiescer struct{ err error }

func (q quiescer) QuiesceHarnessAdapter(context.Context, domain.SHA256Digest) error { return q.err }

type probe struct {
	installation ports.HarnessInstallation
	err          error
}

func (p probe) ProbeHarnessAdapter(context.Context, string) (ports.HarnessInstallation, error) {
	return p.installation, p.err
}

type pairing struct{ err error }

func (p pairing) ActivateHarnessAdapterPairing(context.Context, ports.HarnessInstallation) error {
	return p.err
}

func fixture(t *testing.T, root string, sequence uint64, currentSequence uint64) (Request, ports.HarnessInstallation) {
	t.Helper()
	now := time.Date(2026, 9, 17, 5, 0, 0, 0, time.UTC)
	content := []byte("adapter-v2")
	artifact := filepath.Join(t.TempDir(), "candidate")
	if err := os.WriteFile(artifact, content, 0600); err != nil {
		t.Fatal(err)
	}
	digest := domain.DigestSHA256(content)
	protocol := domain.DigestSHA256([]byte("protocol"))
	manifest := domain.HarnessAdapterManifest{AdapterID: "kennel-codex", AdapterDigest: digest, TransportClasses: []domain.HarnessCapabilityClass{domain.HarnessCapabilityTurn}, MinimumVersion: "1.0.0", MaximumVersion: "2.0.0", ProtocolFingerprints: []domain.SHA256Digest{protocol}}
	md, err := manifest.ContentDigest()
	if err != nil {
		t.Fatal(err)
	}
	manifest.Digest = md
	release := domain.TrustedHarnessAdapterRelease{TrustRootID: "test-root", AdapterID: manifest.AdapterID, Version: "1.2.0", Sequence: sequence, ArtifactDigest: digest, ManifestDigest: md, OS: runtime.GOOS, Architecture: runtime.GOARCH, Size: int64(len(content)), ExpiresAt: now.Add(time.Hour)}
	req := Request{OperationID: "op-1", ArtifactPath: artifact, Release: release, Manifest: manifest, CurrentVersion: "1.1.0", CurrentSequence: currentSequence, Required: []domain.HarnessCapabilityClass{domain.HarnessCapabilityTurn}, Now: now}
	installation := ports.HarnessInstallation{Harness: "codex", ExecutablePath: filepath.Join(root, "probe"), ExecutableDigest: digest, Version: release.Version, Protocol: ports.ChatProtocolProvenance{ProtocolDigest: protocol.String()}, ObservedAt: now}
	return req, installation
}
func installOld(t *testing.T, root string) domain.SHA256Digest {
	t.Helper()
	content := []byte("adapter-v1")
	d := domain.DigestSHA256(content)
	dir := filepath.Join(root, "generations", "old")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, artifactName), content, 0500); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("generations", "old"), filepath.Join(root, "active")); err != nil {
		t.Fatal(err)
	}
	return d
}
func TestInstallFreshCommitAndReplay(t *testing.T) {
	root := t.TempDir()
	req, got := fixture(t, root, 1, 0)
	s := Stager{Root: root, Quiescer: quiescer{}, Health: probe{installation: got}, Pairing: pairing{}}
	op, err := s.Install(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if op.State != domain.AdapterInstallCommitted {
		t.Fatalf("state %s", op.State)
	}
	target, err := os.Readlink(filepath.Join(root, "active"))
	if err != nil {
		t.Fatal(err)
	}
	d, _, err := fileDigest(context.Background(), filepath.Join(root, target, artifactName))
	if err != nil || d != req.Release.ArtifactDigest {
		t.Fatalf("active digest %s err %v", d, err)
	}
	replay, err := s.Install(context.Background(), req)
	if err != nil || replay.State != domain.AdapterInstallCommitted {
		t.Fatalf("replay %+v %v", replay, err)
	}
	changed := req
	changed.Release.Sequence = 2
	if _, err = s.Install(context.Background(), changed); !errors.Is(err, domain.ErrHarnessAdapterInstallConflict) {
		t.Fatalf("changed replay %v", err)
	}
}
func TestPairingUnavailableFailsClosedAndRollsBack(t *testing.T) {
	root := t.TempDir()
	old := installOld(t, root)
	req, got := fixture(t, root, 2, 1)
	req.ExpectedActive = old
	s := Stager{Root: root, Quiescer: quiescer{}, Health: probe{installation: got}}
	op, err := s.Install(context.Background(), req)
	if !errors.Is(err, domain.ErrHarnessAdapterPairingUnavailable) {
		t.Fatalf("error %v", err)
	}
	if op.State != domain.AdapterInstallRolledBack {
		t.Fatalf("state %s", op.State)
	}
	a, err := s.activeTargetAndDigest(context.Background())
	if err != nil || a.digest != old {
		t.Fatalf("rollback digest %s err %v", a.digest, err)
	}
}
func TestTamperAndRollbackReleaseFailBeforeActivation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Request)
	}{{"tamper", func(r *Request) { _ = os.WriteFile(r.ArtifactPath, []byte("wrong-data"), 0600) }}, {"rollback", func(r *Request) { r.Release.Sequence = 1 }}} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			old := installOld(t, root)
			req, got := fixture(t, root, 2, 1)
			req.ExpectedActive = old
			tc.mutate(&req)
			s := Stager{Root: root, Quiescer: quiescer{}, Health: probe{installation: got}, Pairing: pairing{}}
			op, err := s.Install(context.Background(), req)
			if err == nil {
				t.Fatal("expected failure")
			}
			if op.State != domain.AdapterInstallActionNeeded {
				t.Fatalf("state %s", op.State)
			}
			a, e := s.activeTargetAndDigest(context.Background())
			if e != nil || a.digest != old {
				t.Fatalf("active changed %s %v", a.digest, e)
			}
		})
	}
}
func TestConcurrentExactInstallConverges(t *testing.T) {
	root := t.TempDir()
	req, got := fixture(t, root, 1, 0)
	s := Stager{Root: root, Quiescer: quiescer{}, Health: probe{installation: got}, Pairing: pairing{}}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			op, err := s.Install(context.Background(), req)
			if err == nil && op.State != domain.AdapterInstallCommitted {
				err = errors.New("not committed")
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestCandidateSymlinkRejected(t *testing.T) {
	root := t.TempDir()
	req, got := fixture(t, root, 1, 0)
	real := req.ArtifactPath
	link := real + "-link"
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	req.ArtifactPath = link
	s := Stager{Root: root, Quiescer: quiescer{}, Health: probe{installation: got}, Pairing: pairing{}}
	op, err := s.Install(context.Background(), req)
	if err == nil || op.State != domain.AdapterInstallActionNeeded {
		t.Fatalf("op=%+v err=%v", op, err)
	}
	if _, err := os.Lstat(filepath.Join(root, "active")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("active created: %v", err)
	}
}

func TestRestartPendingActivationRollsBack(t *testing.T) {
	root := t.TempDir()
	old := installOld(t, root)
	req, _ := fixture(t, root, 2, 1)
	req.ExpectedActive = old
	s := Stager{Root: root}
	digest, err := requestDigest(req)
	if err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(root, "generations", "pending")
	if err := os.MkdirAll(candidate, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(candidate, artifactName), []byte("candidate"), 0500); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Readlink(filepath.Join(root, "active"))
	if err != nil {
		t.Fatal(err)
	}
	previous = filepath.Join(root, previous)
	if err := s.swapLink(candidate); err != nil {
		t.Fatal(err)
	}
	op := domain.HarnessAdapterInstallOperation{ID: req.OperationID, RequestDigest: digest, State: domain.AdapterInstallActivatedPendingHealth, PreviousPath: previous, PreviousDigest: old, CandidatePath: candidate, CandidateDigest: req.Release.ArtifactDigest, ReleaseSequence: req.Release.Sequence, CreatedAt: req.Now, UpdatedAt: req.Now}
	if err := s.persist(&op); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.Install(context.Background(), req)
	if err != nil || recovered.State != domain.AdapterInstallRolledBack {
		t.Fatalf("recovered=%+v err=%v", recovered, err)
	}
	a, err := s.activeTargetAndDigest(context.Background())
	if err != nil || a.digest != old {
		t.Fatalf("active=%+v err=%v", a, err)
	}
}

func TestCommitPinsLastKnownGood(t *testing.T) {
	root := t.TempDir()
	req, got := fixture(t, root, 1, 0)
	s := Stager{Root: root, Quiescer: quiescer{}, Health: probe{installation: got}, Pairing: pairing{}}
	if _, err := s.Install(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	activeTarget, err := os.Readlink(filepath.Join(root, "active"))
	if err != nil {
		t.Fatal(err)
	}
	lastTarget, err := os.Readlink(filepath.Join(root, "last-known-good"))
	if err != nil {
		t.Fatal(err)
	}
	if activeTarget != lastTarget {
		t.Fatalf("active=%q last-known-good=%q", activeTarget, lastTarget)
	}
}
