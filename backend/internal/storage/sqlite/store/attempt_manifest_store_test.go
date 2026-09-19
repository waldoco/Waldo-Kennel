package store_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
)

func inputManifestFixture(attemptID domain.AttemptID, outcomeID domain.OutcomeID, plan domain.PlanRevision, at time.Time) domain.AttemptInputManifest {
	return domain.AttemptInputManifest{
		AttemptID: attemptID, OutcomeID: outcomeID, PlanRevisionID: plan.ID,
		WorkUnitID: plan.WorkUnits[0].ID, ContractRevisionNumber: plan.ContractRevisionNumber,
		WorkspaceKind:          domain.WorkspaceGitWorktree,
		BaseRevision:           "6293eecfdf7f93342fc25a2ef85b26d3a5982bcd",
		RunBriefCoreDigest:     strings.Repeat("a", 64),
		RunBriefCompiledDigest: strings.Repeat("b", 64),
		ExecutionPolicyDigest:  strings.Repeat("c", 64),
		Checks: []domain.AttemptManifestCheck{
			{ID: "chk-1", CriterionID: "crit-1", Argv: []string{"go", "test", "./..."}, TimeoutSeconds: 600},
		},
	}
}

func TestAttemptManifestRoundTripsBothHalves(t *testing.T) {
	s := sqlitetest.MustOpen(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 20, 1, 0, 0, 0, time.UTC)
	plan, outcomeID := seedApprovedPlan(t, s, "manifest-round-trip")
	attempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-manifest-1", domain.FenceSubjectForProject("manifest-round-trip")))
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}

	input, err := domain.NewAttemptInputManifest(inputManifestFixture(attempt.ID, outcomeID, plan, at), at)
	if err != nil {
		t.Fatalf("seal input: %v", err)
	}
	if err := s.SaveAttemptManifest(ctx, input); err != nil {
		t.Fatalf("save input manifest: %v", err)
	}
	output, err := domain.NewAttemptOutputManifest(domain.AttemptOutputManifest{
		AttemptID: attempt.ID, OutcomeID: outcomeID, PlanRevisionID: plan.ID,
		WorkUnitID: plan.WorkUnits[0].ID, ContractRevisionNumber: plan.ContractRevisionNumber,
		ArtifactVersion: strings.Repeat("f", 64),
		RetentionState:  domain.RetentionRetained, TerminationReason: "succeeded", ObservedAt: at,
	}, at)
	if err != nil {
		t.Fatalf("seal output: %v", err)
	}
	if err := s.SaveAttemptManifest(ctx, output); err != nil {
		t.Fatalf("save output manifest: %v", err)
	}

	gotInput, ok, err := s.GetAttemptManifest(ctx, attempt.ID, domain.AttemptManifestInput)
	if err != nil || !ok {
		t.Fatalf("read input manifest: ok=%v err=%v", ok, err)
	}
	if gotInput.PayloadDigest != input.PayloadDigest {
		t.Fatal("input digest changed in storage")
	}
	body, err := gotInput.DecodeInput()
	if err != nil {
		t.Fatal(err)
	}
	if body.PlanRevisionID != plan.ID || body.WorkUnitID != plan.WorkUnits[0].ID || body.BaseRevision != "6293eecfdf7f93342fc25a2ef85b26d3a5982bcd" {
		t.Fatalf("admitted lineage lost: %+v", body)
	}
	gotOutput, ok, err := s.GetAttemptManifest(ctx, attempt.ID, domain.AttemptManifestOutput)
	if err != nil || !ok {
		t.Fatalf("read output manifest: ok=%v err=%v", ok, err)
	}
	outBody, err := gotOutput.DecodeOutput()
	if err != nil {
		t.Fatal(err)
	}
	if outBody.ArtifactVersion != strings.Repeat("f", 64) {
		t.Fatal("output half lost the retained artifact version")
	}

	listed, err := s.ListAttemptManifestsForOutcome(ctx, outcomeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("listed %d halves, want both", len(listed))
	}
}

func TestAttemptManifestSealedHalfRefusesRewrite(t *testing.T) {
	s := sqlitetest.MustOpen(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 20, 1, 0, 0, 0, time.UTC)
	plan, outcomeID := seedApprovedPlan(t, s, "manifest-sealed")
	attempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-manifest-2", domain.FenceSubjectForProject("manifest-sealed")))
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	first, err := domain.NewAttemptInputManifest(inputManifestFixture(attempt.ID, outcomeID, plan, at), at)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAttemptManifest(ctx, first); err != nil {
		t.Fatalf("save: %v", err)
	}
	changed := inputManifestFixture(attempt.ID, outcomeID, plan, at)
	changed.BaseRevision = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	second, err := domain.NewAttemptInputManifest(changed, at)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAttemptManifest(ctx, second); !errors.Is(err, ports.ErrAttemptManifestSealed) {
		t.Fatalf("rewrite of a sealed half = %v, want ErrAttemptManifestSealed", err)
	}
}

// The digest falsifier: bytes altered after sealing - here by tampering that
// bypassed the update trigger, e.g. a restored backup or a manual edit - must
// be refused on read, never returned as custody evidence.
func TestAttemptManifestReadRefusesTamperedPayload(t *testing.T) {
	dir := t.TempDir()
	s := sqlitetest.MustOpenAt(t, dir)
	ctx := context.Background()
	at := time.Date(2026, 9, 20, 1, 0, 0, 0, time.UTC)
	plan, outcomeID := seedApprovedPlan(t, s, "manifest-tamper")
	attempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-manifest-3", domain.FenceSubjectForProject("manifest-tamper")))
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	manifest, err := domain.NewAttemptInputManifest(inputManifestFixture(attempt.ID, outcomeID, plan, at), at)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAttemptManifest(ctx, manifest); err != nil {
		t.Fatalf("save: %v", err)
	}

	raw, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "kennel.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.ExecContext(ctx, `DROP TRIGGER attempt_manifests_no_update`); err != nil {
		t.Fatalf("drop trigger for tamper simulation: %v", err)
	}
	if _, err := raw.ExecContext(ctx, `UPDATE attempt_manifests SET payload_digest = ? WHERE attempt_id = ?`, strings.Repeat("0", 64), string(attempt.ID)); err != nil {
		t.Fatalf("tamper: %v", err)
	}

	if _, _, err := s.GetAttemptManifest(ctx, attempt.ID, domain.AttemptManifestInput); !errors.Is(err, ports.ErrAttemptManifestDigestMismatch) {
		t.Fatalf("tampered manifest read = %v, want ErrAttemptManifestDigestMismatch", err)
	}
	if _, err := s.ListAttemptManifestsForOutcome(ctx, outcomeID); !errors.Is(err, ports.ErrAttemptManifestDigestMismatch) {
		t.Fatalf("tampered manifest list = %v, want ErrAttemptManifestDigestMismatch", err)
	}
}

// Erasing an Outcome must erase its custody manifests with it: the record is
// evidence OF the Outcome, not evidence AGAINST the owner that survives
// erasure. This is the falsifier for the deletion seam's owned-table grant.
func TestAttemptManifestsEraseWithTheirOutcome(t *testing.T) {
	s := sqlitetest.MustOpen(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 20, 1, 0, 0, 0, time.UTC)
	plan, outcomeID := seedApprovedPlan(t, s, "manifest-erasure")
	attempt, err := s.CreateAttemptWithFence(ctx, admissionFor(outcomeID, plan, "rk-manifest-erase", domain.FenceSubjectForProject("manifest-erasure")))
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	input, err := domain.NewAttemptInputManifest(inputManifestFixture(attempt.ID, outcomeID, plan, at), at)
	if err != nil {
		t.Fatalf("seal input: %v", err)
	}
	if err := s.SaveAttemptManifest(ctx, input); err != nil {
		t.Fatalf("save input manifest: %v", err)
	}
	if _, err := s.TransitionAttemptStatus(ctx, outcomeID, attempt.ID, domain.AttemptQueued, domain.AttemptFailed, at); err != nil {
		t.Fatalf("fail attempt: %v", err)
	}
	if _, err := s.ReleaseFenceForAttempt(ctx, attempt.ID, "test-done", at); err != nil {
		t.Fatalf("release fence: %v", err)
	}

	preview, err := s.PreviewOutcomeDeletion(ctx, outcomeID)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(preview.Blockers) > 0 {
		t.Fatalf("preview blockers: %+v", preview.Blockers)
	}
	if err := s.ChangeOutcomeTrash(ctx, outcomeID, preview.Revision, true); err != nil {
		t.Fatalf("trash: %v", err)
	}
	if err := s.BeginOutcomePurge(ctx, outcomeID, preview.Revision); err != nil {
		t.Fatalf("begin purge: %v", err)
	}
	if err := s.PurgeOutcomeRecords(ctx, outcomeID, preview.Revision); err != nil {
		t.Fatalf("purge with a sealed manifest must succeed: %v", err)
	}
	if _, found, err := s.GetAttemptManifest(ctx, attempt.ID, domain.AttemptManifestInput); err != nil || found {
		t.Fatalf("manifest after erasure: found=%v err=%v, want gone", found, err)
	}
}
