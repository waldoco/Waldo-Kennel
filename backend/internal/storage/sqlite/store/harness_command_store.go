package store

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/gen"
)

var _ ports.HarnessCommandStore = (*Store)(nil)

type commandClaimHookKey struct{}
type CommandClaimHook func()

// WithCommandClaimHook installs a deterministic test barrier after every
// authority/target re-read and before writes, while the store writer is held.
func WithCommandClaimHook(ctx context.Context, hook CommandClaimHook) context.Context {
	return context.WithValue(ctx, commandClaimHookKey{}, hook)
}

func (s *Store) ValidateAuthoritiesAndCreateCommandClaim(ctx context.Context, in ports.HarnessCommandRequest) (domain.CommandAuthorityClaim, bool, error) {
	if in.Now.IsZero() || strings.TrimSpace(in.AdapterRequestKey) == "" || in.Target.Class != in.Command.Class || in.Command.Class.Material() {
		return domain.CommandAuthorityClaim{}, false, domain.ErrHarnessCommandInvalid
	}
	payload, err := in.Command.Bytes()
	if err != nil {
		return domain.CommandAuthorityClaim{}, false, err
	}
	contentDigest := domain.DigestSHA256(payload)
	targetDigest, err := in.Target.Digest()
	if err != nil {
		return domain.CommandAuthorityClaim{}, false, err
	}
	requestFingerprint := digestJSON(struct {
		Connection domain.HarnessConnectionBinding
		ProofID    domain.OwnerProofID
		Target     domain.SHA256Digest
		Content    domain.SHA256Digest
		Key        string
	}{in.ConnectionBinding, in.OwnerProofID, targetDigest, contentDigest, in.AdapterRequestKey})
	claimID := "authority-claim-" + requestFingerprint
	destinationType := "governed_control_command"
	if in.Command.Class == domain.OwnerCommandTurn {
		destinationType = "governed_command"
	}
	destinationID := "adapter-command-" + requestFingerprint

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	var result domain.CommandAuthorityClaim
	created := false
	err = s.inTx(ctx, "validate command authorities and create claim", func(q *gen.Queries) error {
		if existing, readErr := q.GetCommandAuthorityClaimByRequest(ctx, gen.GetCommandAuthorityClaimByRequestParams{HarnessConnectionID: string(in.ConnectionBinding.ConnectionID), ConnectionGeneration: in.ConnectionBinding.Generation, AdapterRequestKey: in.AdapterRequestKey}); readErr == nil {
			if existing.RequestFingerprint != requestFingerprint {
				return domain.ErrHarnessCommandConflict
			}
			result = commandAuthorityClaimFromGen(existing)
			return nil
		} else if !errors.Is(readErr, sql.ErrNoRows) {
			return readErr
		}

		mappedClass, mapped := domain.OwnerCommandClassForTransport(in.ConnectionBinding.Class)
		if !mapped || mappedClass != in.Command.Class {
			return domain.ErrHarnessCommandAuthentication
		}
		connectionRow, err := q.GetCommandHarnessConnection(ctx, string(in.ConnectionBinding.ConnectionID))
		if err != nil {
			return domain.ErrHarnessCommandAuthentication
		}
		connection, err := harnessConnectionFromGen(connectionRow)
		if err != nil || !commandConnectionMatches(connection, in.ConnectionBinding, in.Now) || !bearerMatches(in.ConnectionBearer, connection.CapabilityVerifier) {
			return domain.ErrHarnessCommandAuthentication
		}
		proofRow, err := q.GetCommandOwnerProof(ctx, string(in.OwnerProofID))
		if err != nil {
			return domain.ErrHarnessCommandAuthentication
		}
		proof, err := ownerProofFromGen(proofRow)
		if err != nil || proof.ConsumedAt != nil || !in.Now.UTC().Before(proof.ExpiresAt) || proof.AppRunID != connection.AppRunID || proof.MissionID != connection.MissionID || proof.ContentDigest != contentDigest || proof.TargetDigest != targetDigest || proof.Class != in.Command.Class || !bearerMatches(in.OwnerProofBearer, proof.Verifier.String()) {
			return domain.ErrHarnessCommandAuthentication
		}
		if err := validateCommandTarget(ctx, q, in.Target); err != nil {
			return err
		}
		if hook, ok := ctx.Value(commandClaimHookKey{}).(CommandClaimHook); ok && hook != nil {
			hook()
		}

		connectionDigest := digestJSON(connection)
		n, err := q.InsertCommandAuthorityClaim(ctx, gen.InsertCommandAuthorityClaimParams{ID: claimID, AdapterRequestKey: in.AdapterRequestKey, RequestFingerprint: requestFingerprint, OwnerProofID: string(in.OwnerProofID), HarnessConnectionID: string(connection.ID), ConnectionGeneration: connection.Generation, ConnectionBindingDigest: connectionDigest, ConnectionExpiresAt: connection.ExpiresAt, ConnectionRevokedAt: timeToNull(connection.RevokedAt), TransportClass: string(in.ConnectionBinding.Class), AppRunID: connection.AppRunID, MissionID: connection.MissionID, ContentDigest: contentDigest.String(), TargetDigest: targetDigest.String(), OwnerClass: string(in.Command.Class), CanonicalVersion: in.Command.Version, CanonicalPayload: payload, DestinationType: destinationType, DestinationID: destinationID, State: string(domain.CommandAuthorityClaimPending), CreatedAt: in.Now.UTC(), UpdatedAt: in.Now.UTC()})
		if err != nil || n != 1 {
			return domain.ErrHarnessCommandConflict
		}
		consumed, err := q.ConsumeOwnerProof(ctx, gen.ConsumeOwnerProofParams{ConsumedAt: sql.NullTime{Time: in.Now.UTC(), Valid: true}, ID: string(in.OwnerProofID)})
		if err != nil || consumed != 1 {
			return domain.ErrHarnessCommandAuthentication
		}
		outbox, err := q.InsertHarnessCommandOutbox(ctx, gen.InsertHarnessCommandOutboxParams{ClaimID: claimID, DestinationType: destinationType, DestinationID: destinationID, CanonicalPayload: payload, State: string(domain.CommandAuthorityClaimPending), CreatedAt: in.Now.UTC(), UpdatedAt: in.Now.UTC()})
		if err != nil || outbox != 1 {
			return domain.ErrHarnessCommandConflict
		}
		row, err := q.GetCommandAuthorityClaimByRequest(ctx, gen.GetCommandAuthorityClaimByRequestParams{HarnessConnectionID: string(connection.ID), ConnectionGeneration: connection.Generation, AdapterRequestKey: in.AdapterRequestKey})
		if err != nil {
			return err
		}
		result, created = commandAuthorityClaimFromGen(row), true
		return nil
	})
	if err != nil {
		return domain.CommandAuthorityClaim{}, false, err
	}
	return result, created, result.Validate()
}

func validateCommandTarget(ctx context.Context, q *gen.Queries, target domain.OwnerProofTarget) error {
	switch target.Class {
	case domain.OwnerCommandTurn:
		s, err := q.GetCommandSessionTarget(ctx, target.SessionID)
		if err != nil || s.ControllerGeneration != target.ControllerGeneration || s.ExpectedRevision != target.ExpectedRevision || s.CapabilityFingerprint != target.CapabilityFingerprint {
			return domain.ErrHarnessCommandAuthentication
		}
	case domain.OwnerCommandSteer, domain.OwnerCommandInterrupt:
		s, err := q.GetCommandSessionTarget(ctx, target.SessionID)
		if err != nil || s.ControllerGeneration != target.ControllerGeneration || (target.Class == domain.OwnerCommandSteer && s.ExpectedRevision != target.ExpectedRevision) || s.CapabilityFingerprint != target.CapabilityFingerprint {
			return domain.ErrHarnessCommandAuthentication
		}
		turn, err := q.GetCommandActiveTurnTarget(ctx, gen.GetCommandActiveTurnTargetParams{HandledBySessionID: domain.SessionID(target.SessionID), ProviderTurnID: target.ProviderTurnID})
		if err != nil || turn.State != domain.TurnStateRunning {
			return domain.ErrHarnessCommandAuthentication
		}
	case domain.OwnerCommandAnswer:
		question, err := q.GetCommandAnswerQuestionTarget(ctx, target.QuestionID)
		if err != nil || question.Generation != target.QuestionGeneration || question.Status != "pending" {
			return domain.ErrHarnessCommandAuthentication
		}
	default:
		return domain.ErrHarnessCommandInvalid
	}
	return nil
}

func commandConnectionMatches(c domain.HarnessConnection, b domain.HarnessConnectionBinding, now time.Time) bool {
	return c.ID == b.ConnectionID && c.InstallationID == b.InstallationID && c.AdapterDigest == b.AdapterDigest && c.HarnessIdentity == b.HarnessIdentity && c.ProviderVersion == b.ProviderVersion && c.ProtocolFingerprint == b.ProtocolFingerprint && c.MissionID == b.MissionID && c.AppRunID == b.AppRunID && c.Generation == b.Generation && c.RevokedAt == nil && now.UTC().Before(c.ExpiresAt) && c.HasCapability(b.Class)
}
func bearerMatches(bearer, verifier string) bool {
	sum := sha256.Sum256([]byte(bearer))
	stored, err := hex.DecodeString(verifier)
	return err == nil && len(stored) == sha256.Size && subtle.ConstantTimeCompare(sum[:], stored) == 1
}
func digestJSON(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func commandAuthorityClaimFromGen(r gen.CommandAuthorityClaim) domain.CommandAuthorityClaim {
	return domain.CommandAuthorityClaim{ID: r.ID, AdapterRequestKey: r.AdapterRequestKey, RequestFingerprint: r.RequestFingerprint, OwnerProofID: domain.OwnerProofID(r.OwnerProofID), ConnectionID: domain.HarnessConnectionID(r.HarnessConnectionID), ConnectionGeneration: r.ConnectionGeneration, ConnectionBindingDigest: domain.SHA256Digest(r.ConnectionBindingDigest), ConnectionExpiresAt: r.ConnectionExpiresAt.UTC(), ConnectionRevokedAt: harnessNullTimePtr(r.ConnectionRevokedAt), TransportClass: domain.HarnessCapabilityClass(r.TransportClass), AppRunID: r.AppRunID, MissionID: r.MissionID, ContentDigest: domain.SHA256Digest(r.ContentDigest), TargetDigest: domain.SHA256Digest(r.TargetDigest), OwnerClass: domain.OwnerCommandClass(r.OwnerClass), CanonicalVersion: r.CanonicalVersion, CanonicalPayload: r.CanonicalPayload, DestinationType: r.DestinationType, DestinationID: r.DestinationID, State: domain.CommandAuthorityClaimState(r.State), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
}

func (s *Store) ListPendingHarnessCommandOutbox(ctx context.Context) ([]domain.HarnessCommandOutboxRecord, error) {
	rows, err := s.qr.ListPendingHarnessCommandOutbox(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.HarnessCommandOutboxRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.HarnessCommandOutboxRecord{ClaimID: row.ClaimID, DestinationType: row.DestinationType, DestinationID: row.DestinationID, CanonicalPayload: append([]byte(nil), row.CanonicalPayload...), State: domain.CommandAuthorityClaimState(row.State), CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC()})
	}
	return out, nil
}

func timeToNull(value *time.Time) sql.NullTime {
	if value == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: value.UTC(), Valid: true}
}
func harnessNullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time.UTC()
	return &t
}
