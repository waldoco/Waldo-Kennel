package harnessauthority

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/harnessconnection"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/harnesspairing"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/google/uuid"
)

type Service struct {
	store       ports.HarnessAuthorityStore
	pairing     *harnesspairing.Coordinator
	connections *harnessconnection.Kernel
	now         func() time.Time
}

func New(store ports.HarnessAuthorityStore, pairing *harnesspairing.Coordinator, connections *harnessconnection.Kernel) *Service {
	return &Service{store: store, pairing: pairing, connections: connections, now: time.Now}
}

// Projection reads delegate to the authority store so the HTTP controller and
// command service share one source of truth.
func (s *Service) ListHarnessPairingIntents(ctx context.Context, projectID domain.ProjectID, limit int) ([]domain.HarnessPairingIntent, error) {
	return s.store.ListHarnessPairingIntents(ctx, projectID, limit)
}
func (s *Service) GetHarnessPairingIntent(ctx context.Context, id domain.PairingChallengeID) (domain.HarnessPairingIntent, bool, error) {
	return s.store.GetHarnessPairingIntent(ctx, id)
}
func (s *Service) ListHarnessConnections(ctx context.Context, missionID string, limit int) ([]domain.HarnessConnection, error) {
	return s.store.ListHarnessConnections(ctx, missionID, limit)
}
func (s *Service) GetHarnessConnection(ctx context.Context, id domain.HarnessConnectionID) (domain.HarnessConnection, bool, error) {
	return s.connections.Get(ctx, id)
}
func (s *Service) ListHarnessAuthorityReceipts(ctx context.Context, typ, id string) ([]domain.HarnessAuthorityReceipt, error) {
	return s.store.ListHarnessAuthorityReceipts(ctx, typ, id)
}
func (s *Service) CountHarnessCommandConsequences(ctx context.Context, id domain.HarnessConnectionID, generation int64) (int64, error) {
	return s.store.CountHarnessCommandConsequences(ctx, id, generation)
}

type CreateIntentRequest struct {
	ID                               domain.PairingChallengeID
	ProjectID                        domain.ProjectID
	Kind                             domain.HarnessPairingKind
	ConnectionID                     domain.HarnessConnectionID
	InstallationID                   string
	AdapterDigest                    domain.SHA256Digest
	HarnessIdentity, ProviderVersion string
	ProtocolFingerprint              domain.SHA256Digest
	MissionID, AppRunID              string
	CapabilityClasses                []domain.HarnessCapabilityClass
	ExpectedGeneration               int64
	ConnectionExpiresAt, ExpiresAt   time.Time
	RequestKey, RequestFingerprint   string
}

func (s *Service) CreateIntent(ctx context.Context, r CreateIntentRequest) (domain.HarnessPairingIntent, bool, error) {
	if s == nil || s.store == nil {
		return domain.HarnessPairingIntent{}, false, domain.ErrHarnessPairingIntentInvalid
	}
	if r.ID == "" {
		r.ID = domain.PairingChallengeID("pair-intent-" + uuid.NewString())
	}
	now := s.now().UTC()
	v := domain.HarnessPairingIntent{ID: r.ID, ProjectID: r.ProjectID, Kind: r.Kind, ConnectionID: r.ConnectionID, InstallationID: strings.TrimSpace(r.InstallationID), AdapterDigest: r.AdapterDigest, HarnessIdentity: strings.TrimSpace(r.HarnessIdentity), ProviderVersion: strings.TrimSpace(r.ProviderVersion), ProtocolFingerprint: r.ProtocolFingerprint, MissionID: strings.TrimSpace(r.MissionID), AppRunID: strings.TrimSpace(r.AppRunID), CapabilityClasses: r.CapabilityClasses, ExpectedGeneration: r.ExpectedGeneration, ConnectionExpiresAt: r.ConnectionExpiresAt.UTC(), ExpiresAt: r.ExpiresAt.UTC(), Status: domain.HarnessPairingIntentRequested, ProposalRequestKey: strings.TrimSpace(r.RequestKey), ProposalRequestFingerprint: domain.SHA256Digest(r.RequestFingerprint), CreatedAt: now, UpdatedAt: now}
	var e error
	v.Digest, e = v.ComputedDigest()
	if e != nil {
		return v, false, e
	}
	return s.store.CreateHarnessPairingIntent(ctx, v)
}

type DecisionRequest struct {
	IntentID                                            domain.PairingChallengeID
	Digest                                              domain.SHA256Digest
	Action, RequestKey, OwnerPrincipal, ConfirmationRef string
}

func (s *Service) DecideIntent(ctx context.Context, r DecisionRequest) (domain.HarnessPairingIntent, bool, error) {
	if r.Action != "approve" && r.Action != "deny" {
		return domain.HarnessPairingIntent{}, false, domain.ErrHarnessAuthorityInvalid
	}
	now := s.now().UTC()
	receipt, e := receipt(r.Action, "pairing_intent", string(r.IntentID), r.Digest, 0, r.RequestKey, r.OwnerPrincipal, r.ConfirmationRef, now)
	if e != nil {
		return domain.HarnessPairingIntent{}, false, e
	}
	return s.store.DecideHarnessPairingIntent(ctx, r.IntentID, r.Digest, r.Action, receipt, now)
}

type RevokeRequest struct {
	ConnectionID                                domain.HarnessConnectionID
	Digest                                      domain.SHA256Digest
	ExpectedGeneration                          int64
	RequestKey, OwnerPrincipal, ConfirmationRef string
}

func (s *Service) Revoke(ctx context.Context, r RevokeRequest) (domain.HarnessConnection, bool, int64, error) {
	now := s.now().UTC()
	receipt, e := receipt("revoke", "harness_connection", string(r.ConnectionID), r.Digest, r.ExpectedGeneration, r.RequestKey, r.OwnerPrincipal, r.ConfirmationRef, now)
	if e != nil {
		return domain.HarnessConnection{}, false, 0, e
	}
	return s.store.RevokeHarnessConnectionWithReceipt(ctx, r.ConnectionID, r.Digest, r.ExpectedGeneration, receipt, now)
}
func receipt(action, typ, id string, digest domain.SHA256Digest, g int64, key, owner, confirmation string, now time.Time) (domain.HarnessAuthorityReceipt, error) {
	sem := struct {
		Action, Type, ID, Digest string
		Generation               int64
		Key, Owner, Confirmation string
	}{action, typ, id, digest.String(), g, key, owner, confirmation}
	b, e := json.Marshal(sem)
	if e != nil {
		return domain.HarnessAuthorityReceipt{}, e
	}
	r := domain.HarnessAuthorityReceipt{ID: "harness-receipt-" + uuid.NewString(), Action: action, TargetType: typ, TargetID: id, TargetDigest: digest, ExpectedGeneration: g, RequestKey: strings.TrimSpace(key), RequestFingerprint: domain.DigestSHA256(b), OwnerPrincipal: strings.TrimSpace(owner), ConfirmationRef: strings.TrimSpace(confirmation), CreatedAt: now}
	return r, r.Validate()
}
