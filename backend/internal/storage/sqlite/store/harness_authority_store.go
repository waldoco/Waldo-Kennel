package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/gen"
)

func (s *Store) CreateHarnessPairingIntent(ctx context.Context, in domain.HarnessPairingIntent) (domain.HarnessPairingIntent, bool, error) {
	if err := in.Validate(); err != nil {
		return domain.HarnessPairingIntent{}, false, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	var out domain.HarnessPairingIntent
	created := false
	err := s.inTx(ctx, "replace live harness pairing intent", func(q *gen.Queries) error {
		row, err := q.GetHarnessPairingIntentByProposalRequest(ctx, gen.GetHarnessPairingIntentByProposalRequestParams{AppRunID: in.AppRunID, ProposalRequestKey: in.ProposalRequestKey})
		if err == nil {
			out, err = harnessPairingIntentFromGen(row)
			if err != nil {
				return err
			}
			if out.ProposalRequestFingerprint != in.ProposalRequestFingerprint {
				return domain.ErrHarnessAuthorityConflict
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err = q.SupersedeLiveHarnessPairingIntents(ctx, gen.SupersedeLiveHarnessPairingIntentsParams{UpdatedAt: in.UpdatedAt, ConnectionID: string(in.ConnectionID), ExpectedGeneration: in.ExpectedGeneration}); err != nil {
			return err
		}
		n, err := q.InsertHarnessPairingIntent(ctx, harnessPairingIntentInsert(in))
		if err != nil || n != 1 {
			return domain.ErrHarnessAuthorityConflict
		}
		out, created = in, true
		return nil
	})
	return out, created, err
}
func (s *Store) GetHarnessPairingIntent(ctx context.Context, id domain.PairingChallengeID) (domain.HarnessPairingIntent, bool, error) {
	row, e := s.qr.GetHarnessPairingIntent(ctx, string(id))
	if errors.Is(e, sql.ErrNoRows) {
		return domain.HarnessPairingIntent{}, false, nil
	}
	if e != nil {
		return domain.HarnessPairingIntent{}, false, e
	}
	v, e := harnessPairingIntentFromGen(row)
	return v, true, e
}
func (s *Store) ListHarnessPairingIntents(ctx context.Context, p domain.ProjectID, limit int) ([]domain.HarnessPairingIntent, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	rows, e := s.qr.ListHarnessPairingIntents(ctx, gen.ListHarnessPairingIntentsParams{Column1: string(p), ProjectID: string(p), Limit: int64(limit)})
	if e != nil {
		return nil, e
	}
	out := make([]domain.HarnessPairingIntent, 0, len(rows))
	for _, r := range rows {
		v, e := harnessPairingIntentFromGen(r)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Store) DecideHarnessPairingIntent(ctx context.Context, id domain.PairingChallengeID, digest domain.SHA256Digest, decision string, receipt domain.HarnessAuthorityReceipt, now time.Time) (domain.HarnessPairingIntent, bool, error) {
	if (decision != "approve" && decision != "deny") || receipt.Action != decision || receipt.TargetType != "pairing_intent" || receipt.TargetID != string(id) || receipt.TargetDigest != digest || receipt.Validate() != nil || now.IsZero() {
		return domain.HarnessPairingIntent{}, false, domain.ErrHarnessAuthorityInvalid
	}
	status := domain.HarnessPairingIntentApproved
	if decision == "deny" {
		status = domain.HarnessPairingIntentDenied
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	var out domain.HarnessPairingIntent
	changed := false
	e := s.inTx(ctx, "decide harness pairing intent", func(q *gen.Queries) error {
		existing, e := q.GetHarnessAuthorityReceiptByRequest(ctx, gen.GetHarnessAuthorityReceiptByRequestParams{OwnerPrincipal: receipt.OwnerPrincipal, RequestKey: receipt.RequestKey})
		if e == nil {
			r := authorityReceiptFromGen(existing)
			if r.RequestFingerprint != receipt.RequestFingerprint {
				return domain.ErrHarnessAuthorityConflict
			}
			row, e := q.GetHarnessPairingIntent(ctx, string(id))
			if e != nil {
				return e
			}
			out, e = harnessPairingIntentFromGen(row)
			return e
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		n, e := q.DecideHarnessPairingIntent(ctx, gen.DecideHarnessPairingIntentParams{Status: string(status), DecisionID: receipt.ID, Decision: decision, DecisionRequestKey: receipt.RequestKey, OwnerPrincipal: receipt.OwnerPrincipal, ConfirmationRef: receipt.ConfirmationRef, DecidedAt: sql.NullTime{Time: now.UTC(), Valid: true}, UpdatedAt: now.UTC(), ID: string(id), Digest: digest.String(), ExpiresAt: now.UTC()})
		if e != nil {
			return e
		}
		if n != 1 {
			return domain.ErrHarnessAuthorityStale
		}
		n, e = q.InsertHarnessAuthorityReceipt(ctx, authorityReceiptInsert(receipt))
		if e != nil || n != 1 {
			return domain.ErrHarnessAuthorityConflict
		}
		row, e := q.GetHarnessPairingIntent(ctx, string(id))
		if e != nil {
			return e
		}
		out, e = harnessPairingIntentFromGen(row)
		changed = true
		return e
	})
	return out, changed, e
}
func (s *Store) ListHarnessConnections(ctx context.Context, mission string, limit int) ([]domain.HarnessConnection, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	rows, e := s.qr.ListHarnessConnections(ctx, gen.ListHarnessConnectionsParams{Column1: mission, MissionID: mission, Limit: int64(limit)})
	if e != nil {
		return nil, e
	}
	out := make([]domain.HarnessConnection, 0, len(rows))
	for _, r := range rows {
		v, e := harnessConnectionFromGen(r)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Store) RevokeHarnessConnectionWithReceipt(ctx context.Context, id domain.HarnessConnectionID, digest domain.SHA256Digest, generation int64, receipt domain.HarnessAuthorityReceipt, now time.Time) (domain.HarnessConnection, bool, int64, error) {
	if receipt.Action != "revoke" || receipt.TargetType != "harness_connection" || receipt.TargetID != string(id) || receipt.TargetDigest != digest || receipt.ExpectedGeneration != generation || receipt.Validate() != nil {
		return domain.HarnessConnection{}, false, 0, domain.ErrHarnessAuthorityInvalid
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	var out domain.HarnessConnection
	changed := false
	var count int64
	e := s.inTx(ctx, "revoke harness connection with receipt", func(q *gen.Queries) error {
		existing, e := q.GetHarnessAuthorityReceiptByRequest(ctx, gen.GetHarnessAuthorityReceiptByRequestParams{OwnerPrincipal: receipt.OwnerPrincipal, RequestKey: receipt.RequestKey})
		if e == nil {
			r := authorityReceiptFromGen(existing)
			if r.RequestFingerprint != receipt.RequestFingerprint {
				return domain.ErrHarnessAuthorityConflict
			}
			row, e := q.GetHarnessConnection(ctx, string(id))
			if e != nil {
				return e
			}
			out, e = harnessConnectionFromGen(row)
			if e != nil {
				return e
			}
			count, e = q.CountHarnessCommandConsequences(ctx, gen.CountHarnessCommandConsequencesParams{HarnessConnectionID: string(id), ConnectionGeneration: generation})
			return e
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		row, e := q.GetHarnessConnection(ctx, string(id))
		if e != nil {
			return domain.ErrHarnessAuthorityStale
		}
		current, e := harnessConnectionFromGen(row)
		if e != nil {
			return e
		}
		actual, e := current.AuthorityDigest()
		if e != nil || actual != digest || current.Generation != generation || current.RevokedAt != nil || !now.UTC().Before(current.ExpiresAt) {
			return domain.ErrHarnessAuthorityStale
		}
		n, e := q.RevokeHarnessConnection(ctx, gen.RevokeHarnessConnectionParams{RevokedAt: sql.NullTime{Time: now.UTC(), Valid: true}, UpdatedAt: now.UTC(), ID: string(id), ExpectedGeneration: generation})
		if e != nil || n != 1 {
			return domain.ErrHarnessAuthorityStale
		}
		if _, e = q.MarkHarnessCommandClaimsActionNeeded(ctx, gen.MarkHarnessCommandClaimsActionNeededParams{ConnectionRevokedAt: sql.NullTime{Time: now.UTC(), Valid: true}, UpdatedAt: now.UTC(), HarnessConnectionID: string(id), ConnectionGeneration: generation}); e != nil {
			return e
		}
		if _, e = q.MarkHarnessCommandOutboxActionNeededForConnection(ctx, gen.MarkHarnessCommandOutboxActionNeededForConnectionParams{UpdatedAt: now.UTC(), HarnessConnectionID: string(id), ConnectionGeneration: generation}); e != nil {
			return e
		}
		n, e = q.InsertHarnessAuthorityReceipt(ctx, authorityReceiptInsert(receipt))
		if e != nil || n != 1 {
			return domain.ErrHarnessAuthorityConflict
		}
		row, e = q.GetHarnessConnection(ctx, string(id))
		if e != nil {
			return e
		}
		out, e = harnessConnectionFromGen(row)
		if e != nil {
			return e
		}
		count, e = q.CountHarnessCommandConsequences(ctx, gen.CountHarnessCommandConsequencesParams{HarnessConnectionID: string(id), ConnectionGeneration: generation})
		changed = true
		return e
	})
	return out, changed, count, e
}
func (s *Store) ListHarnessAuthorityReceipts(ctx context.Context, typ, id string) ([]domain.HarnessAuthorityReceipt, error) {
	rows, e := s.qr.ListHarnessAuthorityReceiptsForTarget(ctx, gen.ListHarnessAuthorityReceiptsForTargetParams{TargetType: typ, TargetID: id})
	if e != nil {
		return nil, e
	}
	out := make([]domain.HarnessAuthorityReceipt, len(rows))
	for i, r := range rows {
		out[i] = authorityReceiptFromGen(r)
	}
	return out, nil
}
func (s *Store) CountHarnessCommandConsequences(ctx context.Context, id domain.HarnessConnectionID, g int64) (int64, error) {
	return s.qr.CountHarnessCommandConsequences(ctx, gen.CountHarnessCommandConsequencesParams{HarnessConnectionID: string(id), ConnectionGeneration: g})
}
func harnessPairingIntentInsert(v domain.HarnessPairingIntent) gen.InsertHarnessPairingIntentParams {
	return gen.InsertHarnessPairingIntentParams{ID: string(v.ID), ProjectID: string(v.ProjectID), Kind: string(v.Kind), ConnectionID: string(v.ConnectionID), InstallationID: v.InstallationID, AdapterDigest: v.AdapterDigest.String(), HarnessIdentity: v.HarnessIdentity, ProviderVersion: v.ProviderVersion, ProtocolFingerprint: v.ProtocolFingerprint.String(), MissionID: v.MissionID, AppRunID: v.AppRunID, CapabilityClasses: encodeHarnessCapabilities(v.CapabilityClasses), ExpectedGeneration: v.ExpectedGeneration, ConnectionExpiresAt: v.ConnectionExpiresAt.UTC(), ExpiresAt: v.ExpiresAt.UTC(), Digest: v.Digest.String(), Status: string(v.Status), ProposalRequestKey: v.ProposalRequestKey, ProposalRequestFingerprint: v.ProposalRequestFingerprint.String(), CreatedAt: v.CreatedAt.UTC(), UpdatedAt: v.UpdatedAt.UTC()}
}
func harnessPairingIntentFromGen(r gen.HarnessPairingIntent) (domain.HarnessPairingIntent, error) {
	classes, e := decodeHarnessCapabilities(r.CapabilityClasses)
	if e != nil {
		return domain.HarnessPairingIntent{}, e
	}
	v := domain.HarnessPairingIntent{ID: domain.PairingChallengeID(r.ID), ProjectID: domain.ProjectID(r.ProjectID), Kind: domain.HarnessPairingKind(r.Kind), ConnectionID: domain.HarnessConnectionID(r.ConnectionID), InstallationID: r.InstallationID, AdapterDigest: domain.SHA256Digest(r.AdapterDigest), HarnessIdentity: r.HarnessIdentity, ProviderVersion: r.ProviderVersion, ProtocolFingerprint: domain.SHA256Digest(r.ProtocolFingerprint), MissionID: r.MissionID, AppRunID: r.AppRunID, CapabilityClasses: classes, ExpectedGeneration: r.ExpectedGeneration, ConnectionExpiresAt: r.ConnectionExpiresAt.UTC(), ExpiresAt: r.ExpiresAt.UTC(), Digest: domain.SHA256Digest(r.Digest), Status: domain.HarnessPairingIntentStatus(r.Status), ProposalRequestKey: r.ProposalRequestKey, ProposalRequestFingerprint: domain.SHA256Digest(r.ProposalRequestFingerprint), DecisionID: r.DecisionID, Decision: r.Decision, DecisionRequestKey: r.DecisionRequestKey, OwnerPrincipal: r.OwnerPrincipal, ConfirmationRef: r.ConfirmationRef, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
	if r.ChallengeID.Valid {
		x := domain.PairingChallengeID(r.ChallengeID.String)
		v.ChallengeID = &x
	}
	if r.DecidedAt.Valid {
		x := r.DecidedAt.Time.UTC()
		v.DecidedAt = &x
	}
	return v, v.Validate()
}
func authorityReceiptInsert(r domain.HarnessAuthorityReceipt) gen.InsertHarnessAuthorityReceiptParams {
	return gen.InsertHarnessAuthorityReceiptParams{ID: r.ID, Action: r.Action, TargetType: r.TargetType, TargetID: r.TargetID, TargetDigest: r.TargetDigest.String(), ExpectedGeneration: r.ExpectedGeneration, RequestKey: r.RequestKey, RequestFingerprint: r.RequestFingerprint.String(), OwnerPrincipal: r.OwnerPrincipal, ConfirmationRef: r.ConfirmationRef, CreatedAt: r.CreatedAt.UTC()}
}
func authorityReceiptFromGen(r gen.HarnessAuthorityReceipt) domain.HarnessAuthorityReceipt {
	return domain.HarnessAuthorityReceipt{ID: r.ID, Action: r.Action, TargetType: r.TargetType, TargetID: r.TargetID, TargetDigest: domain.SHA256Digest(r.TargetDigest), ExpectedGeneration: r.ExpectedGeneration, RequestKey: r.RequestKey, RequestFingerprint: domain.SHA256Digest(r.RequestFingerprint), OwnerPrincipal: r.OwnerPrincipal, ConfirmationRef: r.ConfirmationRef, CreatedAt: r.CreatedAt.UTC()}
}

func (s *Store) ActivateHarnessPairingIntent(ctx context.Context, id domain.PairingChallengeID, digest domain.SHA256Digest, now time.Time, issue func(domain.HarnessPairingIntent) (domain.HarnessPairingChallenge, domain.PairingChallengeSecret, error)) (domain.HarnessPairingIntent, domain.HarnessPairingChallenge, domain.PairingChallengeSecret, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	var out domain.HarnessPairingIntent
	var challenge domain.HarnessPairingChallenge
	var secret domain.PairingChallengeSecret
	err := s.inTx(ctx, "activate harness pairing intent", func(q *gen.Queries) error {
		row, e := q.GetApprovedHarnessPairingIntentForActivation(ctx, gen.GetApprovedHarnessPairingIntentForActivationParams{ID: string(id), Digest: digest.String(), ExpiresAt: now.UTC()})
		if e != nil {
			return domain.ErrHarnessAuthorityStale
		}
		intent, e := harnessPairingIntentFromGen(row)
		if e != nil {
			return e
		}
		n, e := q.BeginHarnessPairingIntentActivation(ctx, gen.BeginHarnessPairingIntentActivationParams{UpdatedAt: now.UTC(), ID: string(id), Digest: digest.String(), ExpiresAt: now.UTC()})
		if e != nil || n != 1 {
			return domain.ErrHarnessAuthorityStale
		}
		challenge, secret, e = issue(intent)
		if e != nil {
			return e
		}
		if _, e = q.SupersedePendingHarnessPairingChallenges(ctx, gen.SupersedePendingHarnessPairingChallengesParams{UpdatedAt: now.UTC(), ConnectionID: string(challenge.ConnectionID)}); e != nil {
			return e
		}
		if n, e = q.InsertHarnessPairingChallenge(ctx, harnessPairingChallengeInsert(challenge)); e != nil || n != 1 {
			return domain.ErrHarnessPairingConflict
		}
		if n, e = q.CompleteHarnessPairingIntentActivation(ctx, gen.CompleteHarnessPairingIntentActivationParams{ChallengeID: sql.NullString{String: string(challenge.ID), Valid: true}, UpdatedAt: now.UTC(), ID: string(id)}); e != nil || n != 1 {
			return domain.ErrHarnessAuthorityStale
		}
		row, e = q.GetHarnessPairingIntent(ctx, string(id))
		if e != nil {
			return e
		}
		out, e = harnessPairingIntentFromGen(row)
		return e
	})
	return out, challenge, secret, err
}
