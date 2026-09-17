package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/gen"
)

var _ ports.HarnessPairingChallengeStore = (*Store)(nil)

func (s *Store) SupersedePendingHarnessPairingChallenges(ctx context.Context, connectionID domain.HarnessConnectionID, now time.Time) (int64, error) {
	if now.IsZero() {
		return 0, domain.ErrHarnessPairingInvalid
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n, err := s.qw.SupersedePendingHarnessPairingChallenges(ctx, gen.SupersedePendingHarnessPairingChallengesParams{UpdatedAt: now.UTC(), ConnectionID: string(connectionID)})
	if err != nil {
		return 0, fmt.Errorf("supersede pending harness pairing challenges: %w", err)
	}
	return n, nil
}

func (s *Store) CreateHarnessPairingChallenge(ctx context.Context, rec domain.HarnessPairingChallenge) (domain.HarnessPairingChallenge, bool, error) {
	if err := rec.Validate(); err != nil {
		return domain.HarnessPairingChallenge{}, false, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n, err := s.qw.InsertHarnessPairingChallenge(ctx, harnessPairingChallengeInsert(rec))
	if err != nil {
		return domain.HarnessPairingChallenge{}, false, fmt.Errorf("create harness pairing challenge: %w", err)
	}
	if n == 0 {
		return domain.HarnessPairingChallenge{}, false, domain.ErrHarnessPairingConflict
	}
	return rec, true, nil
}

func (s *Store) GetHarnessPairingChallenge(ctx context.Context, id domain.PairingChallengeID) (domain.HarnessPairingChallenge, bool, error) {
	row, err := s.qr.GetHarnessPairingChallenge(ctx, string(id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.HarnessPairingChallenge{}, false, nil
	}
	if err != nil {
		return domain.HarnessPairingChallenge{}, false, fmt.Errorf("get harness pairing challenge: %w", err)
	}
	rec, err := harnessPairingChallengeFromGen(row)
	return rec, true, err
}

// ConsumeHarnessPairingChallenge is the sole point of no return in the
// pairing lifecycle: it transitions pending -> consumed only when unexpired.
// A crash after this returns true but before the caller mints/delivers a
// bearer leaves the challenge permanently consumed with no path to replay.
func (s *Store) ConsumeHarnessPairingChallenge(ctx context.Context, id domain.PairingChallengeID, now time.Time) (bool, error) {
	if now.IsZero() {
		return false, domain.ErrHarnessPairingInvalid
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n, err := s.qw.ConsumeHarnessPairingChallenge(ctx, gen.ConsumeHarnessPairingChallengeParams{UpdatedAt: now.UTC(), ID: string(id), Now: now.UTC()})
	if err != nil {
		return false, fmt.Errorf("consume harness pairing challenge: %w", err)
	}
	return n > 0, nil
}

func (s *Store) RecordHarnessPairingResult(ctx context.Context, id domain.PairingChallengeID, code domain.HarnessPairingResultCode, now time.Time) (bool, error) {
	if !code.Valid() || now.IsZero() {
		return false, domain.ErrHarnessPairingInvalid
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n, err := s.qw.RecordHarnessPairingResult(ctx, gen.RecordHarnessPairingResultParams{ResultCode: sql.NullString{String: string(code), Valid: true}, UpdatedAt: now.UTC(), ID: string(id)})
	if err != nil {
		return false, fmt.Errorf("record harness pairing result: %w", err)
	}
	return n > 0, nil
}

func harnessPairingChallengeInsert(r domain.HarnessPairingChallenge) gen.InsertHarnessPairingChallengeParams {
	return gen.InsertHarnessPairingChallengeParams{
		ID: string(r.ID), Kind: string(r.Kind), ConnectionID: string(r.ConnectionID),
		InstallationID: r.InstallationID, AdapterDigest: r.AdapterDigest.String(), HarnessIdentity: r.HarnessIdentity,
		ProviderVersion: r.ProviderVersion, ProtocolFingerprint: r.ProtocolFingerprint.String(),
		MissionID: r.MissionID, AppRunID: r.AppRunID, CapabilityClasses: encodeHarnessCapabilities(r.CapabilityClasses),
		ExpectedGeneration: r.ExpectedGeneration, ProofVerifier: r.ProofVerifier, Status: string(r.Status),
		ResultCode: harnessPairingResultCodeToNull(r.ResultCode), ConnectionExpiresAt: r.ConnectionExpiresAt.UTC(),
		ExpiresAt: r.ExpiresAt.UTC(), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

func harnessPairingChallengeFromGen(r gen.HarnessPairingChallenge) (domain.HarnessPairingChallenge, error) {
	classes, err := decodeHarnessCapabilities(r.CapabilityClasses)
	if err != nil {
		return domain.HarnessPairingChallenge{}, err
	}
	rec := domain.HarnessPairingChallenge{
		ID: domain.PairingChallengeID(r.ID), Kind: domain.HarnessPairingKind(r.Kind), ConnectionID: domain.HarnessConnectionID(r.ConnectionID),
		InstallationID: r.InstallationID, AdapterDigest: domain.SHA256Digest(r.AdapterDigest), HarnessIdentity: r.HarnessIdentity,
		ProviderVersion: r.ProviderVersion, ProtocolFingerprint: domain.SHA256Digest(r.ProtocolFingerprint),
		MissionID: r.MissionID, AppRunID: r.AppRunID, CapabilityClasses: classes, ExpectedGeneration: r.ExpectedGeneration,
		ProofVerifier: r.ProofVerifier, Status: domain.HarnessPairingStatus(r.Status), ResultCode: harnessPairingResultCodeFromNull(r.ResultCode),
		ConnectionExpiresAt: r.ConnectionExpiresAt.UTC(), ExpiresAt: r.ExpiresAt.UTC(), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	return rec, rec.Validate()
}

func harnessPairingResultCodeToNull(code *domain.HarnessPairingResultCode) sql.NullString {
	if code == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: string(*code), Valid: true}
}

func harnessPairingResultCodeFromNull(v sql.NullString) *domain.HarnessPairingResultCode {
	if !v.Valid {
		return nil
	}
	code := domain.HarnessPairingResultCode(v.String)
	return &code
}
