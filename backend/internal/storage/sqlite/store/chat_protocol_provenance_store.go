package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/gen"
)

// Persisted protocol-negotiation provenance for Chat sessions. See migration
// 0136 and ADR 0016: PR #180 negotiates the provider surface at runtime; this
// store keeps the negotiation record so review surfaces can answer "which
// provider build and negotiated surface ran this work" after restarts.

// RecordChatProtocolProvenance appends one negotiation episode. Episodes are
// append-only per session: a renegotiation after a provider upgrade must
// never rewrite the evidence an older Attempt was reviewed against, so the
// seq is assigned inside the write transaction rather than by the caller.
func (s *Store) RecordChatProtocolProvenance(ctx context.Context, rec domain.ChatProtocolProvenance) error {
	degraded, err := json.Marshal(rec.DegradedCapabilities)
	if err != nil {
		return fmt.Errorf("marshal degraded capabilities for session %s: %w", rec.SessionID, err)
	}
	missing, err := json.Marshal(rec.MissingFloor)
	if err != nil {
		return fmt.Errorf("marshal missing floor for session %s: %w", rec.SessionID, err)
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.writeDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin record protocol provenance %s: %w", rec.SessionID, err)
	}
	defer func() { _ = tx.Rollback() }()
	txq := s.qw.WithTx(tx)

	seq, err := txq.NextChatProtocolProvenanceSeq(ctx, rec.SessionID)
	if err != nil {
		return fmt.Errorf("next protocol provenance seq %s: %w", rec.SessionID, err)
	}
	if err := txq.InsertChatProtocolProvenance(ctx, gen.InsertChatProtocolProvenanceParams{
		SessionID:            rec.SessionID,
		Seq:                  seq,
		Harness:              string(rec.Harness),
		Provider:             rec.Provider,
		InstalledVersion:     rec.InstalledVersion,
		GeneratedFrom:        rec.GeneratedFrom,
		ProtocolDigest:       rec.ProtocolDigest,
		GeneratedDigest:      rec.GeneratedDigest,
		MatchesGenerated:     boolToInt(rec.MatchesGenerated),
		DegradedCapabilities: string(degraded),
		MissingFloor:         string(missing),
		NegotiatedAt:         rec.NegotiatedAt,
	}); err != nil {
		return fmt.Errorf("insert protocol provenance %s: %w", rec.SessionID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit protocol provenance %s: %w", rec.SessionID, err)
	}
	return nil
}

// ChatProtocolProvenanceForBinding returns the negotiation episode that
// answers for one provider-session binding: the latest episode at or before
// boundAt, because a later renegotiation (provider upgrade, resume) must
// never answer for work bound earlier. found is false when no episode exists
// at or before the binding: writes are fail-soft at session start, so an
// episode recorded only later must not be attributed to work it postdates -
// honest absence, never a substitute record.
func (s *Store) ChatProtocolProvenanceForBinding(ctx context.Context, sessionID string, boundAt time.Time) (rec domain.ChatProtocolProvenance, found bool, err error) {
	row, err := s.qr.ChatProtocolProvenanceAtOrBefore(ctx, gen.ChatProtocolProvenanceAtOrBeforeParams{SessionID: sessionID, NegotiatedAt: boundAt})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ChatProtocolProvenance{}, false, nil
	}
	if err != nil {
		return domain.ChatProtocolProvenance{}, false, fmt.Errorf("read protocol provenance %s: %w", sessionID, err)
	}
	return chatProtocolProvenanceFromRow(row)
}

func chatProtocolProvenanceFromRow(row gen.ChatProtocolProvenance) (domain.ChatProtocolProvenance, bool, error) {
	var degraded []string
	if err := json.Unmarshal([]byte(row.DegradedCapabilities), &degraded); err != nil {
		return domain.ChatProtocolProvenance{}, false, fmt.Errorf("unmarshal degraded capabilities %s: %w", row.SessionID, err)
	}
	var missing []string
	if err := json.Unmarshal([]byte(row.MissingFloor), &missing); err != nil {
		return domain.ChatProtocolProvenance{}, false, fmt.Errorf("unmarshal missing floor %s: %w", row.SessionID, err)
	}
	return domain.ChatProtocolProvenance{
		SessionID:            row.SessionID,
		Seq:                  row.Seq,
		Harness:              domain.AgentHarness(row.Harness),
		Provider:             row.Provider,
		InstalledVersion:     row.InstalledVersion,
		GeneratedFrom:        row.GeneratedFrom,
		ProtocolDigest:       row.ProtocolDigest,
		GeneratedDigest:      row.GeneratedDigest,
		MatchesGenerated:     row.MatchesGenerated != 0,
		DegradedCapabilities: degraded,
		MissingFloor:         missing,
		NegotiatedAt:         row.NegotiatedAt,
	}, true, nil
}
