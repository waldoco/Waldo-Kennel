package daemon

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

type fakeAttemptManifestStore struct {
	saved []domain.AttemptManifest
}

func (f *fakeAttemptManifestStore) SaveAttemptManifest(_ context.Context, m domain.AttemptManifest) error {
	for _, existing := range f.saved {
		if existing.AttemptID == m.AttemptID && existing.Half == m.Half {
			return ports.ErrAttemptManifestSealed
		}
	}
	f.saved = append(f.saved, m)
	return nil
}

func (f *fakeAttemptManifestStore) GetAttemptManifest(_ context.Context, attemptID domain.AttemptID, half domain.AttemptManifestHalf) (domain.AttemptManifest, bool, error) {
	for _, m := range f.saved {
		if m.AttemptID == attemptID && m.Half == half {
			return m, true, nil
		}
	}
	return domain.AttemptManifest{}, false, nil
}

func (f *fakeAttemptManifestStore) ListAttemptManifestsForOutcome(context.Context, domain.OutcomeID) ([]domain.AttemptManifest, error) {
	return f.saved, nil
}

func outputManifestTestReceipt(artifactSeed string) domain.AttemptReceipt {
	return domain.AttemptReceipt{
		AttemptID:              "att-11111111-1111-1111-1111-111111111111",
		OutcomeID:              "out-22222222-2222-2222-2222-222222222222",
		PlanRevisionID:         "plr-33333333-3333-3333-3333-333333333333",
		WorkUnitID:             "wu-44444444-4444-4444-4444-444444444444",
		ContractRevisionNumber: 1,
		ArtifactVersion:        fmt.Sprintf("%x", sha256.Sum256([]byte(artifactSeed))),
		RetentionState:         domain.RetentionRetained,
		ObservedAt:             time.Date(2026, 9, 20, 1, 0, 0, 0, time.UTC),
	}
}

// The output half is sealed exactly once: a daemon restart that re-runs
// retention with the receipt already durable must replay as a no-op, and a
// replay whose retained bytes diverge from the sealed record must be refused.
func TestSealOutputManifestRestartReplay(t *testing.T) {
	manifests := &fakeAttemptManifestStore{}
	retainer := &attemptArtifactRetainer{manifests: manifests}
	ctx := context.Background()

	receipt := outputManifestTestReceipt("retained bytes v1")
	if err := retainer.sealOutputManifest(ctx, receipt); err != nil {
		t.Fatalf("first seal: %v", err)
	}
	if len(manifests.saved) != 1 {
		t.Fatalf("saved manifests = %d, want one output half", len(manifests.saved))
	}
	sealed := manifests.saved[0]
	if sealed.Half != domain.AttemptManifestOutput {
		t.Fatalf("half = %q, want output", sealed.Half)
	}
	body, err := sealed.DecodeOutput()
	if err != nil {
		t.Fatal(err)
	}
	if body.ArtifactVersion != receipt.ArtifactVersion || body.RetentionState != receipt.RetentionState {
		t.Fatalf("sealed output body does not bind the receipt: %+v", body)
	}

	// Daemon restart, receipt already durable, identical replay: no-op.
	if err := retainer.sealOutputManifest(ctx, receipt); err != nil {
		t.Fatalf("identical replay after restart must be a no-op: %v", err)
	}
	if len(manifests.saved) != 1 {
		t.Fatalf("replay wrote %d manifests, want still one", len(manifests.saved))
	}

	// Replay claiming different retained bytes for the same Attempt: refused.
	diverged := outputManifestTestReceipt("retained bytes v2")
	err = retainer.sealOutputManifest(ctx, diverged)
	if err == nil || !strings.Contains(err.Error(), "already sealed with different content") {
		t.Fatalf("diverged replay err = %v, want a sealed-with-different-content refusal", err)
	}
	if len(manifests.saved) != 1 {
		t.Fatalf("refused replay wrote %d manifests, want still one", len(manifests.saved))
	}
}

// Test wiring without a manifest store must stay inert.
func TestSealOutputManifestNilStoreIsInert(t *testing.T) {
	retainer := &attemptArtifactRetainer{}
	if err := retainer.sealOutputManifest(context.Background(), outputManifestTestReceipt("x")); err != nil {
		t.Fatalf("nil manifest store must be inert: %v", err)
	}
}
