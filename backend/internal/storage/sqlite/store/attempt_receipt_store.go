package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/gen"
)

// Durable record of what one Attempt produced. See migration 0119.

// SaveAttemptReceipt writes a receipt and its file manifest as one unit.
//
// Receipt and manifest must move together: a receipt whose artifact version
// describes a manifest that failed to write would claim provenance it does not
// have, and a downstream handoff verifies against exactly that version.
//
// A frozen receipt is never replaced. The service refuses first, but this
// returns the typed refusal even if it does not, because "later work cannot
// silently overwrite a reviewed artifact" has to hold at the write path.
func (s *Store) SaveAttemptReceipt(ctx context.Context, receipt domain.AttemptReceipt) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.writeDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin save attempt receipt %s: %w", receipt.AttemptID, err)
	}
	defer func() { _ = tx.Rollback() }()
	txq := s.qw.WithTx(tx)

	existing, err := txq.GetAttemptReceipt(ctx, string(receipt.AttemptID))
	switch {
	case err == nil && existing.FrozenAt.Valid:
		return ports.ErrAttemptReceiptFrozen
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("read attempt receipt %s: %w", receipt.AttemptID, err)
	}
	if err := receipt.Validate(); err != nil {
		return err
	}
	attempt, err := txq.GetAttempt(ctx, gen.GetAttemptParams{ID: receipt.AttemptID, OutcomeID: receipt.OutcomeID})
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("receipt %s has no matching producing Attempt", receipt.AttemptID)
	}
	if err != nil {
		return fmt.Errorf("read producing Attempt %s: %w", receipt.AttemptID, err)
	}
	if attempt.PlanRevisionID != receipt.PlanRevisionID || attempt.WorkUnitID != receipt.WorkUnitID || attempt.ContractRevisionNumber != receipt.ContractRevisionNumber {
		return fmt.Errorf("receipt %s does not match its producing Attempt lineage", receipt.AttemptID)
	}

	now := receipt.UpdatedAt
	if now.IsZero() {
		now = receipt.ObservedAt
	}
	created := receipt.CreatedAt
	if created.IsZero() {
		created = now
	}

	if err := txq.UpsertAttemptReceipt(ctx, gen.UpsertAttemptReceiptParams{
		AttemptID:              string(receipt.AttemptID),
		OutcomeID:              string(receipt.OutcomeID),
		PlanRevisionID:         string(receipt.PlanRevisionID),
		WorkUnitID:             string(receipt.WorkUnitID),
		ContractRevisionNumber: receipt.ContractRevisionNumber,
		ArtifactVersion:        receipt.ArtifactVersion,
		WorkspaceKind:          string(receipt.WorkspaceKind),
		WorkspacePath:          receipt.WorkspacePath,
		RepositoryPath:         receipt.RepositoryPath,
		RepositoryIdentity:     receipt.RepositoryIdentity,
		BaseRevision:           receipt.BaseRevision,
		ResultRevision:         receipt.ResultRevision,
		WorkspaceDirty:         boolToInt(receipt.WorkspaceDirty),
		RetentionState:         string(receipt.RetentionState),
		RetentionDetail:        receipt.RetentionDetail,
		TerminationReason:      receipt.TerminationReason,
		ObservedAt:             receipt.ObservedAt.UTC(),
		CreatedAt:              created.UTC(),
		UpdatedAt:              now.UTC(),
	}); err != nil {
		return fmt.Errorf("save attempt receipt %s: %w", receipt.AttemptID, err)
	}

	// Replace the manifest wholesale: a re-run of retention describes the
	// workspace as it is now, and merging with a previous partial read would
	// produce a manifest matching no actual state.
	if err := txq.DeleteAttemptArtifactFiles(ctx, string(receipt.AttemptID)); err != nil {
		return fmt.Errorf("clear artifact manifest for %s: %w", receipt.AttemptID, err)
	}
	for _, file := range receipt.Files {
		id := file.ID
		if id == "" {
			id = "artifact-" + uuid.NewString()
		}
		if err := txq.InsertAttemptArtifactFile(ctx, gen.InsertAttemptArtifactFileParams{
			ID:                id,
			AttemptID:         string(receipt.AttemptID),
			RelativePath:      file.RelativePath,
			ChangeKind:        string(file.ChangeKind),
			ContentDigest:     file.ContentDigest,
			SizeBytes:         nullInt64(file.SizeBytes),
			FileMode:          nullInt64(file.FileMode),
			IsBinary:          boolToInt(file.IsBinary),
			UnsupportedReason: file.UnsupportedReason,
			Additions:         nullInt64(file.Additions),
			Deletions:         nullInt64(file.Deletions),
		}); err != nil {
			return fmt.Errorf("save artifact file %s for %s: %w", file.RelativePath, receipt.AttemptID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit attempt receipt %s: %w", receipt.AttemptID, err)
	}
	return nil
}

// GetAttemptReceipt reads one receipt with its manifest.
func (s *Store) GetAttemptReceipt(ctx context.Context, attemptID domain.AttemptID) (domain.AttemptReceipt, bool, error) {
	// Receipt and manifest are one immutable snapshot. Reading them through
	// separate pool queries can mix the old parent with a replacement manifest.
	tx, err := s.readDB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.AttemptReceipt{}, false, fmt.Errorf("begin read attempt receipt %s: %w", attemptID, err)
	}
	defer func() { _ = tx.Rollback() }()
	q := s.qw.WithTx(tx)
	row, err := q.GetAttemptReceipt(ctx, string(attemptID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AttemptReceipt{}, false, nil
	}
	if err != nil {
		return domain.AttemptReceipt{}, false, fmt.Errorf("read attempt receipt %s: %w", attemptID, err)
	}
	files, err := q.ListAttemptArtifactFiles(ctx, string(attemptID))
	if err != nil {
		return domain.AttemptReceipt{}, false, fmt.Errorf("read artifact manifest %s: %w", attemptID, err)
	}
	return attemptReceiptFromRow(row, files), true, nil
}

// FreezeAttemptReceipt marks a receipt as review evidence, after which it is
// never replaced. Freezing twice is a no-op so the caller stays idempotent.
func (s *Store) FreezeAttemptReceipt(ctx context.Context, attemptID domain.AttemptID, at time.Time) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	receipt, err := s.qw.GetAttemptReceipt(ctx, string(attemptID))
	if errors.Is(err, sql.ErrNoRows) {
		return ports.ErrAttemptReceiptMissing
	}
	if err != nil {
		return fmt.Errorf("read attempt receipt %s before freeze: %w", attemptID, err)
	}
	if receipt.RetentionState != string(domain.RetentionRetained) {
		return fmt.Errorf("%w: retention state is %q", ports.ErrAttemptReceiptNotReady, receipt.RetentionState)
	}
	if err := s.qw.FreezeAttemptReceipt(ctx, gen.FreezeAttemptReceiptParams{
		FrozenAt:  sql.NullTime{Time: at.UTC(), Valid: true},
		UpdatedAt: at.UTC(),
		AttemptID: string(attemptID),
	}); err != nil {
		return fmt.Errorf("freeze attempt receipt %s: %w", attemptID, err)
	}
	return nil
}

// ClassifyAttemptSucceeded is the single commit boundary for terminal
// classification. The status, exact retained artifact version, immutable
// freeze marker, classification observation and custody release either all
// become visible or none do. This is deliberately below the service so a
// crash cannot strand a half-classified Attempt.
func (s *Store) ClassifyAttemptSucceeded(ctx context.Context, in ports.ClassifyAttemptInput) error {
	if in.At.IsZero() || in.OutcomeID.IsZero() || in.AttemptID.IsZero() || strings.TrimSpace(in.ArtifactVersion) == "" {
		return fmt.Errorf("attempt classification requires identity, artifact version and timestamp")
	}
	if in.ContractRevisionNumber < 1 || in.ProofGeneration == nil {
		// Without these the transaction cannot tell whether the proof it is
		// committing on still holds, and would be a second unchecked read.
		return fmt.Errorf("attempt classification requires the judged contract revision and proof horizon")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.writeDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin classify attempt %s: %w", in.AttemptID, err)
	}
	defer func() { _ = tx.Rollback() }()
	q := s.qw.WithTx(tx)
	attempt, err := q.GetAttempt(ctx, gen.GetAttemptParams{ID: in.AttemptID, OutcomeID: in.OutcomeID})
	if errors.Is(err, sql.ErrNoRows) {
		return ports.ErrAttemptClassificationStale
	}
	if err != nil {
		return fmt.Errorf("read attempt %s for classification: %w", in.AttemptID, err)
	}
	receipt, err := q.GetAttemptReceipt(ctx, string(in.AttemptID))
	if errors.Is(err, sql.ErrNoRows) {
		return ports.ErrAttemptReceiptMissing
	}
	if err != nil {
		return fmt.Errorf("read receipt %s for classification: %w", in.AttemptID, err)
	}
	if receipt.RetentionState != string(domain.RetentionRetained) || receipt.ArtifactVersion != in.ArtifactVersion {
		return fmt.Errorf("%w: expected artifact %q, found %q with retention %q", ports.ErrAttemptReceiptNotReady, in.ArtifactVersion, receipt.ArtifactVersion, receipt.RetentionState)
	}
	if receipt.OutcomeID != string(attempt.OutcomeID) || receipt.PlanRevisionID != string(attempt.PlanRevisionID) ||
		receipt.WorkUnitID != string(attempt.WorkUnitID) || receipt.ContractRevisionNumber != attempt.ContractRevisionNumber {
		return fmt.Errorf("%w: receipt lineage does not match producing attempt", ports.ErrAttemptClassificationStale)
	}
	if receipt.OutcomeID != string(in.OutcomeID) || receipt.PlanRevisionID == "" || receipt.WorkUnitID == "" {
		return fmt.Errorf("receipt %s has invalid producing lineage", in.AttemptID)
	}
	// Revalidate the two things the judgement rested on, inside the same
	// transaction that commits it. Reading the receipt again is not enough:
	// proof naming the Attempt does not establish that the proof still holds,
	// or that it was recorded under the revision now in force.
	if attempt.ContractRevisionNumber != in.ContractRevisionNumber {
		return ports.ErrAttemptClassificationStale
	}
	currentRevision, err := q.MaxContractRevisionNumber(ctx, in.OutcomeID)
	if err != nil {
		return fmt.Errorf("read current contract revision for %s: %w", in.OutcomeID, err)
	}
	if current, ok := currentRevision.(int64); !ok || current != in.ContractRevisionNumber {
		return ports.ErrAttemptClassificationStale
	}
	generation, err := q.OutcomeProofGeneration(ctx, proofGenerationParams(in.OutcomeID))
	if err != nil {
		return fmt.Errorf("read proof generation: %w", err)
	}
	if generation != *in.ProofGeneration {
		return ports.ErrAttemptClassificationStale
	}
	if attempt.Status == domain.AttemptSucceeded {
		if !receipt.FrozenAt.Valid {
			return ports.ErrAttemptClassificationStale
		}
		return nil
	}
	if attempt.Status != in.ExpectedStatus {
		return ports.ErrAttemptClassificationStale
	}
	rows, err := q.TransitionAttemptStatus(ctx, gen.TransitionAttemptStatusParams{
		Status: domain.AttemptSucceeded, UpdatedAt: in.At.UTC(), ID: in.AttemptID,
		OutcomeID: in.OutcomeID, Status_2: in.ExpectedStatus,
	})
	if err != nil {
		return fmt.Errorf("classify attempt %s: %w", in.AttemptID, err)
	}
	if rows == 0 {
		return ports.ErrAttemptClassificationStale
	}
	if err := q.FreezeAttemptReceipt(ctx, gen.FreezeAttemptReceiptParams{
		FrozenAt: sql.NullTime{Time: in.At.UTC(), Valid: true}, UpdatedAt: in.At.UTC(), AttemptID: string(in.AttemptID),
	}); err != nil {
		return fmt.Errorf("freeze receipt %s: %w", in.AttemptID, err)
	}
	maxSeq, err := q.MaxAttemptObservationSeq(ctx, in.AttemptID)
	if err != nil {
		return fmt.Errorf("read observation sequence %s: %w", in.AttemptID, err)
	}
	seq, ok := maxSeq.(int64)
	if !ok {
		return fmt.Errorf("observation sequence for %s has unexpected type %T", in.AttemptID, maxSeq)
	}
	payload := in.ObservationPayload
	if payload == "" {
		payload = "{}"
	}
	if err := q.CreateAttemptObservation(ctx, gen.CreateAttemptObservationParams{
		ID: "obs-" + uuid.NewString(), AttemptID: in.AttemptID, Seq: seq + 1,
		Kind: in.ObservationKind, Payload: payload,
	}); err != nil {
		return fmt.Errorf("record classification observation %s: %w", in.AttemptID, err)
	}
	if _, err := q.ReleaseAttemptFence(ctx, gen.ReleaseAttemptFenceParams{
		ReleasedAt: sql.NullTime{Time: in.At.UTC(), Valid: true}, ReleaseReason: "attempt_succeeded",
		AttemptID: in.AttemptID,
	}); err != nil {
		return fmt.Errorf("release custody for succeeded attempt %s: %w", in.AttemptID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit classification %s: %w", in.AttemptID, err)
	}
	return nil
}

func attemptReceiptFromRow(row gen.AttemptReceipt, files []gen.AttemptArtifactFile) domain.AttemptReceipt {
	receipt := domain.AttemptReceipt{
		AttemptID:              domain.AttemptID(row.AttemptID),
		OutcomeID:              domain.OutcomeID(row.OutcomeID),
		PlanRevisionID:         domain.PlanRevisionID(row.PlanRevisionID),
		WorkUnitID:             domain.WorkUnitID(row.WorkUnitID),
		ContractRevisionNumber: row.ContractRevisionNumber,
		ArtifactVersion:        row.ArtifactVersion,
		WorkspaceKind:          domain.WorkspaceKind(row.WorkspaceKind),
		WorkspacePath:          row.WorkspacePath,
		RepositoryPath:         row.RepositoryPath,
		RepositoryIdentity:     row.RepositoryIdentity,
		BaseRevision:           row.BaseRevision,
		ResultRevision:         row.ResultRevision,
		WorkspaceDirty:         row.WorkspaceDirty == 1,
		RetentionState:         domain.RetentionState(row.RetentionState),
		RetentionDetail:        row.RetentionDetail,
		TerminationReason:      row.TerminationReason,
		ObservedAt:             row.ObservedAt,
		CreatedAt:              row.CreatedAt,
		UpdatedAt:              row.UpdatedAt,
	}
	if row.FrozenAt.Valid {
		frozen := row.FrozenAt.Time
		receipt.FrozenAt = &frozen
	}
	for _, file := range files {
		receipt.Files = append(receipt.Files, domain.ArtifactFile{
			ID:                file.ID,
			AttemptID:         domain.AttemptID(file.AttemptID),
			RelativePath:      file.RelativePath,
			ChangeKind:        domain.ArtifactChangeKind(file.ChangeKind),
			ContentDigest:     file.ContentDigest,
			SizeBytes:         int64Ptr(file.SizeBytes),
			FileMode:          int64Ptr(file.FileMode),
			IsBinary:          file.IsBinary == 1,
			UnsupportedReason: file.UnsupportedReason,
			Additions:         int64Ptr(file.Additions),
			Deletions:         int64Ptr(file.Deletions),
		})
	}
	return receipt
}

func boolToInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func nullInt64(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}

func int64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	out := value.Int64
	return &out
}

// OutcomeProofGeneration returns a commit-ordered token for append-only proof.
func (s *Store) OutcomeProofGeneration(ctx context.Context, outcomeID domain.OutcomeID) (int64, error) {
	return s.qr.OutcomeProofGeneration(ctx, proofGenerationParams(outcomeID))
}

func proofGenerationParams(id domain.OutcomeID) gen.OutcomeProofGenerationParams {
	return gen.OutcomeProofGenerationParams{OutcomeID: string(id), OutcomeID_2: string(id), OutcomeID_3: string(id), OutcomeID_4: string(id)}
}
