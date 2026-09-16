package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/gen"
)

var _ ports.HarnessConnectionStore = (*Store)(nil)

func (s *Store) CreateHarnessConnection(ctx context.Context, rec domain.HarnessConnection) (domain.HarnessConnection, bool, error) {
	if err := rec.Validate(); err != nil {
		return domain.HarnessConnection{}, false, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n, err := s.qw.InsertHarnessConnection(ctx, harnessConnectionInsert(rec))
	if err != nil {
		return domain.HarnessConnection{}, false, fmt.Errorf("create harness connection: %w", err)
	}
	if n > 0 {
		return rec, true, nil
	}
	row, err := s.qw.GetHarnessConnection(ctx, string(rec.ID))
	if err != nil {
		return domain.HarnessConnection{}, false, fmt.Errorf("read harness connection replay: %w", err)
	}
	existing, err := harnessConnectionFromGen(row)
	if err != nil {
		return domain.HarnessConnection{}, false, err
	}
	if !sameHarnessConnection(existing, rec) {
		return existing, false, domain.ErrHarnessConnectionConflict
	}
	return existing, false, nil
}

func (s *Store) GetHarnessConnection(ctx context.Context, id domain.HarnessConnectionID) (domain.HarnessConnection, bool, error) {
	row, err := s.qr.GetHarnessConnection(ctx, string(id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.HarnessConnection{}, false, nil
	}
	if err != nil {
		return domain.HarnessConnection{}, false, fmt.Errorf("get harness connection: %w", err)
	}
	rec, err := harnessConnectionFromGen(row)
	return rec, true, err
}

func (s *Store) RotateHarnessConnection(ctx context.Context, id domain.HarnessConnectionID, generation int64, verifier string, expiresAt, updatedAt time.Time) (domain.HarnessConnection, bool, error) {
	if strings.TrimSpace(verifier) == "" || generation < 1 || expiresAt.IsZero() || updatedAt.IsZero() {
		return domain.HarnessConnection{}, false, domain.ErrHarnessConnectionInvalid
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n, err := s.qw.RotateHarnessConnection(ctx, gen.RotateHarnessConnectionParams{CapabilityVerifier: verifier, ExpiresAt: expiresAt.UTC(), UpdatedAt: updatedAt.UTC(), ID: string(id), ExpectedGeneration: generation})
	if err != nil {
		return domain.HarnessConnection{}, false, fmt.Errorf("rotate harness connection: %w", err)
	}
	if n == 0 {
		return domain.HarnessConnection{}, false, nil
	}
	row, err := s.qw.GetHarnessConnection(ctx, string(id))
	if err != nil {
		return domain.HarnessConnection{}, false, err
	}
	rec, err := harnessConnectionFromGen(row)
	return rec, true, err
}

func (s *Store) RevokeHarnessConnection(ctx context.Context, id domain.HarnessConnectionID, generation int64, at time.Time) (domain.HarnessConnection, bool, error) {
	if generation < 1 || at.IsZero() {
		return domain.HarnessConnection{}, false, domain.ErrHarnessConnectionInvalid
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n, err := s.qw.RevokeHarnessConnection(ctx, gen.RevokeHarnessConnectionParams{RevokedAt: sql.NullTime{Time: at.UTC(), Valid: true}, UpdatedAt: at.UTC(), ID: string(id), ExpectedGeneration: generation})
	if err != nil {
		return domain.HarnessConnection{}, false, fmt.Errorf("revoke harness connection: %w", err)
	}
	if n == 0 {
		return domain.HarnessConnection{}, false, nil
	}
	row, err := s.qw.GetHarnessConnection(ctx, string(id))
	if err != nil {
		return domain.HarnessConnection{}, false, err
	}
	rec, err := harnessConnectionFromGen(row)
	return rec, true, err
}

func harnessConnectionInsert(r domain.HarnessConnection) gen.InsertHarnessConnectionParams {
	return gen.InsertHarnessConnectionParams{ID: string(r.ID), InstallationID: r.InstallationID, AdapterDigest: r.AdapterDigest.String(), HarnessIdentity: r.HarnessIdentity, ProviderVersion: r.ProviderVersion, ProtocolFingerprint: r.ProtocolFingerprint.String(), MissionID: r.MissionID, AppRunID: r.AppRunID, CapabilityClasses: encodeHarnessCapabilities(r.CapabilityClasses), CapabilityVerifier: r.CapabilityVerifier, Generation: r.Generation, ExpiresAt: r.ExpiresAt.UTC(), RevokedAt: harnessNullTime(r.RevokedAt), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
}
func harnessConnectionFromGen(r gen.HarnessConnection) (domain.HarnessConnection, error) {
	classes, err := decodeHarnessCapabilities(r.CapabilityClasses)
	if err != nil {
		return domain.HarnessConnection{}, err
	}
	var revoked *time.Time
	if r.RevokedAt.Valid {
		v := r.RevokedAt.Time.UTC()
		revoked = &v
	}
	rec := domain.HarnessConnection{ID: domain.HarnessConnectionID(r.ID), InstallationID: r.InstallationID, AdapterDigest: domain.SHA256Digest(r.AdapterDigest), HarnessIdentity: r.HarnessIdentity, ProviderVersion: r.ProviderVersion, ProtocolFingerprint: domain.SHA256Digest(r.ProtocolFingerprint), MissionID: r.MissionID, AppRunID: r.AppRunID, CapabilityClasses: classes, CapabilityVerifier: r.CapabilityVerifier, Generation: r.Generation, ExpiresAt: r.ExpiresAt.UTC(), RevokedAt: revoked, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
	return rec, rec.Validate()
}
func encodeHarnessCapabilities(in []domain.HarnessCapabilityClass) string {
	out, _ := domain.NormalizeHarnessCapabilities(in)
	values := make([]string, len(out))
	for i, c := range out {
		values[i] = string(c)
	}
	return strings.Join(values, ",")
}
func decodeHarnessCapabilities(v string) ([]domain.HarnessCapabilityClass, error) {
	parts := strings.Split(v, ",")
	out := make([]domain.HarnessCapabilityClass, len(parts))
	for i, p := range parts {
		out[i] = domain.HarnessCapabilityClass(p)
	}
	return domain.NormalizeHarnessCapabilities(out)
}
func harnessNullTime(v *time.Time) sql.NullTime {
	if v == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: v.UTC(), Valid: true}
}
func sameHarnessConnection(a, b domain.HarnessConnection) bool {
	return a.ID == b.ID && a.InstallationID == b.InstallationID && a.AdapterDigest == b.AdapterDigest && a.HarnessIdentity == b.HarnessIdentity && a.ProviderVersion == b.ProviderVersion && a.ProtocolFingerprint == b.ProtocolFingerprint && a.MissionID == b.MissionID && a.AppRunID == b.AppRunID && encodeHarnessCapabilities(a.CapabilityClasses) == encodeHarnessCapabilities(b.CapabilityClasses) && a.Generation == b.Generation && a.ExpiresAt.Equal(b.ExpiresAt) && a.CreatedAt.Equal(b.CreatedAt)
}
