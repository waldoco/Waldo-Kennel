package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/artifactstore"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

type fakeCustodyFenceStore struct {
	saved []domain.AttemptCustodyFence
}

func (f *fakeCustodyFenceStore) RecordAttemptCustodyFence(_ context.Context, fence domain.AttemptCustodyFence) error {
	for _, existing := range f.saved {
		if existing.AttemptID == fence.AttemptID {
			return ports.ErrAttemptCustodyFenceSealed
		}
	}
	f.saved = append(f.saved, fence)
	return nil
}

func (f *fakeCustodyFenceStore) GetAttemptCustodyFence(_ context.Context, attemptID domain.AttemptID) (domain.AttemptCustodyFence, bool, error) {
	for _, existing := range f.saved {
		if existing.AttemptID == attemptID {
			return existing, true, nil
		}
	}
	return domain.AttemptCustodyFence{}, false, nil
}

type fenceSessionSource struct {
	session domain.Session
	err     error
}

func (s fenceSessionSource) SpawnExactAttempt(context.Context, ports.SpawnConfig, domain.ExecutionBinding) (domain.Session, int, int, error) {
	return domain.Session{}, 0, 0, errors.New("not used")
}

func (s fenceSessionSource) Kill(context.Context, domain.SessionID) (bool, error) {
	return false, errors.New("not used")
}

func (s fenceSessionSource) Get(context.Context, domain.SessionID) (domain.Session, error) {
	return s.session, s.err
}

type fenceRefSource struct {
	receipt   domain.AttemptReceipt
	has       bool
	saved     []domain.AttemptReceipt
	sessionID string
}

func (f *fenceRefSource) SaveAttemptReceipt(_ context.Context, receipt domain.AttemptReceipt) error {
	f.saved = append(f.saved, receipt)
	f.receipt, f.has = receipt, true
	return nil
}

func (f *fenceRefSource) GetAttemptReceipt(_ context.Context, id domain.AttemptID) (domain.AttemptReceipt, bool, error) {
	if !f.has || f.receipt.AttemptID != id {
		return domain.AttemptReceipt{}, false, nil
	}
	return f.receipt, true, nil
}

func (f *fenceRefSource) FreezeAttemptReceipt(context.Context, domain.AttemptID, time.Time) error {
	return nil
}

func (f *fenceRefSource) LatestAttemptSessionRef(context.Context, domain.AttemptID) (domain.AttemptSessionRef, bool, error) {
	if f.sessionID == "" {
		return domain.AttemptSessionRef{}, false, nil
	}
	return domain.AttemptSessionRef{AttemptID: "att-fence", SessionID: f.sessionID}, true, nil
}

func fenceTestAttempt() domain.Attempt {
	return domain.Attempt{
		ID: "att-fence", OutcomeID: "out-1", PlanRevisionID: "plan-1", WorkUnitID: "wu-1",
		ContractRevisionNumber: 1, Status: domain.AttemptReconciled,
	}
}

func fenceTestWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "result.txt"), []byte("done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func newFenceRetainer(t *testing.T, terminated bool) (*attemptArtifactRetainer, *fakeCustodyFenceStore, *fenceRefSource) {
	t.Helper()
	artifacts, err := artifactstore.New(artifactstore.Config{Root: filepath.Join(t.TempDir(), "artifacts")})
	if err != nil {
		t.Fatalf("artifact store: %v", err)
	}
	fences := &fakeCustodyFenceStore{}
	refs := &fenceRefSource{sessionID: "sess-fence"}
	retainer := &attemptArtifactRetainer{
		sessions: fenceSessionSource{session: domain.Session{SessionRecord: domain.SessionRecord{
			ID:           "sess-fence",
			IsTerminated: terminated,
			Metadata:     domain.SessionMetadata{WorkspacePath: fenceTestWorkspace(t)},
		}}},
		refs: refs, artifacts: artifacts, fences: fences,
	}
	return retainer, fences, refs
}

// No snapshot without the settle fence: a workspace whose provider may still
// be alive is never captured.
func TestRetainAttempt_RefusesSnapshotBeforeProviderExit(t *testing.T) {
	retainer, fences, refs := newFenceRetainer(t, false)
	err := retainer.RetainAttempt(context.Background(), fenceTestAttempt())
	if err == nil || !strings.Contains(err.Error(), "has not exited") {
		t.Fatalf("retain err = %v, want a custody-close refusal", err)
	}
	if len(fences.saved) != 0 {
		t.Fatalf("fence recorded for a live provider: %+v", fences.saved)
	}
	if len(refs.saved) != 0 {
		t.Fatalf("receipt saved without a settle fence: %+v", refs.saved)
	}
}

// Custody close on a settled provider: the fence is durable, then the
// snapshot. A restart replay records no second fence and seals identically.
func TestRetainAttempt_RecordsFenceBeforeSnapshot(t *testing.T) {
	retainer, fences, refs := newFenceRetainer(t, true)
	attempt := fenceTestAttempt()
	if err := retainer.RetainAttempt(context.Background(), attempt); err != nil {
		t.Fatalf("retain: %v", err)
	}
	if len(fences.saved) != 1 {
		t.Fatalf("fences = %d, want exactly one", len(fences.saved))
	}
	fence := fences.saved[0]
	if fence.AttemptID != attempt.ID || fence.SessionID != "sess-fence" {
		t.Fatalf("fence = %+v, want attempt %s session sess-fence", fence, attempt.ID)
	}
	if len(refs.saved) != 1 {
		t.Fatalf("receipts = %d, want one snapshot after the fence", len(refs.saved))
	}
	// Daemon restart: the receipt is durable, retention replays, and the
	// fence stays exactly-once.
	if err := retainer.RetainAttempt(context.Background(), attempt); err != nil {
		t.Fatalf("restart replay: %v", err)
	}
	if len(fences.saved) != 1 {
		t.Fatalf("replay wrote %d fences, want still one", len(fences.saved))
	}
}

// A fence recorded against a different session binding than the one being
// retained is refused: the close must bind the same provider whose exit it
// attests.
func TestSettleCustodyFence_RefusesDivergentSessionBinding(t *testing.T) {
	retainer, fences, _ := newFenceRetainer(t, true)
	attempt := fenceTestAttempt()
	session := domain.Session{SessionRecord: domain.SessionRecord{ID: "sess-original", IsTerminated: true}}
	if err := retainer.settleCustodyFence(context.Background(), attempt, session); err != nil {
		t.Fatalf("first settle: %v", err)
	}
	if len(fences.saved) != 1 {
		t.Fatalf("fences = %d, want one", len(fences.saved))
	}
	// Identical replay: accepted, no rewrite.
	if err := retainer.settleCustodyFence(context.Background(), attempt, session); err != nil {
		t.Fatalf("identical replay: %v", err)
	}
	if len(fences.saved) != 1 {
		t.Fatalf("replay wrote %d fences, want still one", len(fences.saved))
	}
	// Divergent binding: refused.
	other := domain.Session{SessionRecord: domain.SessionRecord{ID: "sess-other", IsTerminated: true}}
	err := retainer.settleCustodyFence(context.Background(), attempt, other)
	if err == nil || !strings.Contains(err.Error(), "already recorded against session") {
		t.Fatalf("divergent settle err = %v, want a refusal", err)
	}
	if len(fences.saved) != 1 {
		t.Fatalf("refused settle wrote %d fences, want still one", len(fences.saved))
	}
}

// Test wiring without a fence store keeps the pre-8B behavior: custody-close
// evidence is unavailable, never fabricated.
func TestSettleCustodyFence_NilStoreIsInert(t *testing.T) {
	retainer := &attemptArtifactRetainer{}
	session := domain.Session{SessionRecord: domain.SessionRecord{ID: "sess-x", IsTerminated: false}}
	if err := retainer.settleCustodyFence(context.Background(), fenceTestAttempt(), session); err != nil {
		t.Fatalf("nil fence store must be inert: %v", err)
	}
}
