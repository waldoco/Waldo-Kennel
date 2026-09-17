package controllers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/apispec"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/envelope"
	"github.com/go-chi/chi/v5"
)

type HarnessAuthorityReader interface {
	ListHarnessPairingIntents(context.Context, domain.ProjectID, int) ([]domain.HarnessPairingIntent, error)
	GetHarnessPairingIntent(context.Context, domain.PairingChallengeID) (domain.HarnessPairingIntent, bool, error)
	ListHarnessConnections(context.Context, string, int) ([]domain.HarnessConnection, error)
	GetHarnessConnection(context.Context, domain.HarnessConnectionID) (domain.HarnessConnection, bool, error)
	ListHarnessAuthorityReceipts(context.Context, string, string) ([]domain.HarnessAuthorityReceipt, error)
	CountHarnessCommandConsequences(context.Context, domain.HarnessConnectionID, int64) (int64, error)
}
type HarnessAuthorityController struct {
	Svc HarnessAuthorityReader
	Now func() time.Time
}

func (c *HarnessAuthorityController) Register(r chi.Router) {
	r.Get("/harness-pairing-intents", c.listIntents)
	r.Get("/harness-pairing-intents/{intentId}", c.getIntent)
	r.Get("/harness-connections", c.listConnections)
	r.Get("/harness-connections/{connectionId}", c.getConnection)
}
func (c *HarnessAuthorityController) unavailable(w http.ResponseWriter, r *http.Request, method, path string) bool {
	if c == nil || c.Svc == nil {
		apispec.NotImplemented(w, r, method, path)
		return true
	}
	return false
}
func (c *HarnessAuthorityController) listIntents(w http.ResponseWriter, r *http.Request) {
	if c.unavailable(w, r, "GET", "/api/v1/harness-pairing-intents") {
		return
	}
	v, e := c.Svc.ListHarnessPairingIntents(r.Context(), domain.ProjectID(r.URL.Query().Get("projectId")), limit(r))
	if e != nil {
		internalHarnessError(w, r)
		return
	}
	out := make([]PairingIntentView, len(v))
	for i, x := range v {
		out[i] = c.intentView(r.Context(), x)
	}
	envelope.WriteJSON(w, http.StatusOK, HarnessPairingIntentListResponse{Data: HarnessPairingIntentListData{Intents: out}})
}
func (c *HarnessAuthorityController) getIntent(w http.ResponseWriter, r *http.Request) {
	if c.unavailable(w, r, "GET", "/api/v1/harness-pairing-intents/{intentId}") {
		return
	}
	v, ok, e := c.Svc.GetHarnessPairingIntent(r.Context(), domain.PairingChallengeID(chi.URLParam(r, "intentId")))
	if e != nil {
		internalHarnessError(w, r)
		return
	}
	if !ok {
		envelope.WriteAPIError(w, r, 404, "not_found", "PAIRING_INTENT_NOT_FOUND", "Pairing intent was not found", nil)
		return
	}
	receipts, _ := c.Svc.ListHarnessAuthorityReceipts(r.Context(), "pairing_intent", string(v.ID))
	envelope.WriteJSON(w, 200, HarnessPairingIntentDetailResponse{Data: HarnessPairingIntentDetailData{Intent: c.intentView(r.Context(), v), Receipts: receiptViews(receipts)}})
}
func (c *HarnessAuthorityController) listConnections(w http.ResponseWriter, r *http.Request) {
	if c.unavailable(w, r, "GET", "/api/v1/harness-connections") {
		return
	}
	v, e := c.Svc.ListHarnessConnections(r.Context(), r.URL.Query().Get("missionId"), limit(r))
	if e != nil {
		internalHarnessError(w, r)
		return
	}
	out := make([]ConnectionView, 0, len(v))
	for _, x := range v {
		out = append(out, c.connection(r, x))
	}
	envelope.WriteJSON(w, 200, HarnessConnectionListResponse{Data: HarnessConnectionListData{Connections: out}})
}
func (c *HarnessAuthorityController) getConnection(w http.ResponseWriter, r *http.Request) {
	if c.unavailable(w, r, "GET", "/api/v1/harness-connections/{connectionId}") {
		return
	}
	v, ok, e := c.Svc.GetHarnessConnection(r.Context(), domain.HarnessConnectionID(chi.URLParam(r, "connectionId")))
	if e != nil {
		internalHarnessError(w, r)
		return
	}
	if !ok {
		envelope.WriteAPIError(w, r, 404, "not_found", "HARNESS_CONNECTION_NOT_FOUND", "Harness connection was not found", nil)
		return
	}
	receipts, _ := c.Svc.ListHarnessAuthorityReceipts(r.Context(), "harness_connection", string(v.ID))
	envelope.WriteJSON(w, 200, HarnessConnectionDetailResponse{Data: HarnessConnectionDetailData{Connection: c.connection(r, v), Receipts: receiptViews(receipts)}})
}

type AuthorityReceiptView struct {
	ID                 string              `json:"id"`
	Action             string              `json:"action"`
	TargetType         string              `json:"targetType"`
	TargetID           string              `json:"targetId"`
	TargetDigest       domain.SHA256Digest `json:"targetDigest"`
	ExpectedGeneration int64               `json:"expectedGeneration"`
	Confirmed          bool                `json:"confirmed"`
	CreatedAt          time.Time           `json:"createdAt"`
}

func receiptViews(in []domain.HarnessAuthorityReceipt) []AuthorityReceiptView {
	out := make([]AuthorityReceiptView, len(in))
	for i, r := range in {
		out[i] = AuthorityReceiptView{ID: r.ID, Action: r.Action, TargetType: r.TargetType, TargetID: r.TargetID, TargetDigest: r.TargetDigest, ExpectedGeneration: r.ExpectedGeneration, Confirmed: r.ConfirmationRef != "", CreatedAt: r.CreatedAt}
	}
	return out
}

type CapabilityView struct {
	Class    domain.HarnessCapabilityClass `json:"class"`
	Effect   string                        `json:"effect"`
	Material bool                          `json:"material"`
}

func capabilities(v []domain.HarnessCapabilityClass) []CapabilityView {
	out := make([]CapabilityView, len(v))
	for i, x := range v {
		owner, mapped := domain.OwnerCommandClassForTransport(x)
		out[i] = CapabilityView{x, string(x), mapped && owner.Material()}
	}
	return out
}

type PairingIntentView struct {
	ID                  domain.PairingChallengeID  `json:"id"`
	Version             string                     `json:"version"`
	Digest              domain.SHA256Digest        `json:"digest"`
	Kind                domain.HarnessPairingKind  `json:"kind"`
	ConnectionID        domain.HarnessConnectionID `json:"connectionId"`
	InstallationID      string                     `json:"installationId"`
	HarnessIdentity     string                     `json:"harnessIdentity"`
	ProviderVersion     string                     `json:"providerVersion"`
	AdapterDigest       domain.SHA256Digest        `json:"adapterDigest"`
	ProtocolFingerprint domain.SHA256Digest        `json:"protocolFingerprint"`
	MissionID           string                     `json:"missionId"`
	Capabilities        []CapabilityView           `json:"capabilities"`
	ExpectedGeneration  int64                      `json:"expectedGeneration"`
	ConnectionExpiresAt time.Time                  `json:"connectionExpiresAt"`
	ExpiresAt           time.Time                  `json:"expiresAt"`
	Status              string                     `json:"status"`
	ProofState          string                     `json:"proofState"`
	UpdatedAt           time.Time                  `json:"updatedAt"`
}

type harnessChallengeReader interface {
	GetHarnessPairingChallenge(context.Context, domain.PairingChallengeID) (domain.HarnessPairingChallenge, bool, error)
}

func (c *HarnessAuthorityController) intentView(ctx context.Context, v domain.HarnessPairingIntent) PairingIntentView {
	status := string(v.Status)
	now := c.now()
	if !now.Before(v.ExpiresAt) && v.Status != domain.HarnessPairingIntentDenied && v.Status != domain.HarnessPairingIntentActive {
		status = "expired"
	}
	proof := "not_started"
	if v.Status == domain.HarnessPairingIntentSuperseded {
		proof = "superseded"
	} else if status == "expired" {
		proof = "expired"
	} else if v.ChallengeID != nil {
		proof = "pending"
		if cr, ok := c.Svc.(harnessChallengeReader); ok {
			if ch, found, err := cr.GetHarnessPairingChallenge(ctx, *v.ChallengeID); err == nil && found {
				switch {
				case ch.Status == domain.HarnessPairingSuperseded || (ch.ResultCode != nil && *ch.ResultCode == domain.HarnessPairingResultSuperseded):
					proof = "superseded"
				case ch.ResultCode != nil && *ch.ResultCode == domain.HarnessPairingResultExpired:
					proof = "expired"
				case ch.ResultCode != nil && *ch.ResultCode == domain.HarnessPairingResultSucceeded:
					proof = "succeeded"
					if conn, found, err := c.Svc.GetHarnessConnection(ctx, v.ConnectionID); err != nil || !found || conn.Generation != v.ExpectedGeneration {
						proof = "superseded"
					}
				case ch.ResultCode != nil:
					proof = "failed"
				}
			}
		}
	}
	return PairingIntentView{v.ID, "v1", v.Digest, v.Kind, v.ConnectionID, v.InstallationID, v.HarnessIdentity, v.ProviderVersion, v.AdapterDigest, v.ProtocolFingerprint, v.MissionID, capabilities(v.CapabilityClasses), v.ExpectedGeneration, v.ConnectionExpiresAt, v.ExpiresAt, status, proof, v.UpdatedAt}
}

type ConnectionView struct {
	ID                   domain.HarnessConnectionID `json:"id"`
	Version              string                     `json:"version"`
	Digest               domain.SHA256Digest        `json:"digest"`
	InstallationID       string                     `json:"installationId"`
	HarnessIdentity      string                     `json:"harnessIdentity"`
	ProviderVersion      string                     `json:"providerVersion"`
	AdapterDigest        domain.SHA256Digest        `json:"adapterDigest"`
	ProtocolFingerprint  domain.SHA256Digest        `json:"protocolFingerprint"`
	MissionID            string                     `json:"missionId"`
	Capabilities         []CapabilityView           `json:"capabilities"`
	Generation           int64                      `json:"generation"`
	ExpiresAt            time.Time                  `json:"expiresAt"`
	RevokedAt            *time.Time                 `json:"revokedAt,omitempty"`
	State                string                     `json:"state"`
	Reason               string                     `json:"reason"`
	Repair               string                     `json:"repair"`
	ActionNeededCommands int64                      `json:"actionNeededCommands"`
	UpdatedAt            time.Time                  `json:"updatedAt"`
}

func (c *HarnessAuthorityController) connection(r *http.Request, v domain.HarnessConnection) ConnectionView {
	d, _ := v.AuthorityDigest()
	now := c.now()
	eval := domain.EvaluateHarnessConnection(&v, domain.HarnessConnectionFacts{InstallationID: v.InstallationID, HarnessIdentity: v.HarnessIdentity, MissionID: v.MissionID, AppRunID: v.AppRunID, AdapterDigest: v.AdapterDigest, ProtocolFingerprint: v.ProtocolFingerprint, Generation: v.Generation, RequiredCapabilities: v.CapabilityClasses, Now: now})
	n, _ := c.Svc.CountHarnessCommandConsequences(r.Context(), v.ID, v.Generation)
	return ConnectionView{v.ID, "v1", d, v.InstallationID, v.HarnessIdentity, v.ProviderVersion, v.AdapterDigest, v.ProtocolFingerprint, v.MissionID, capabilities(v.CapabilityClasses), v.Generation, v.ExpiresAt, v.RevokedAt, string(eval.State), string(eval.Reason), string(eval.Repair), n, v.UpdatedAt}
}
func (c *HarnessAuthorityController) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}
func limit(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if n < 1 || n > 200 {
		return 100
	}
	return n
}
func internalHarnessError(w http.ResponseWriter, r *http.Request) {
	envelope.WriteAPIError(w, r, 500, "internal", "HARNESS_AUTHORITY_FAILED", "Harness authority state could not be read", nil)
}

// Typed harness-authority response envelopes are shared with the generated API spec.
type HarnessPairingIntentListData struct {
	Intents []PairingIntentView `json:"intents"`
}
type HarnessPairingIntentListResponse struct {
	Data HarnessPairingIntentListData `json:"data"`
}
type HarnessPairingIntentDetailData struct {
	Intent   PairingIntentView      `json:"intent"`
	Receipts []AuthorityReceiptView `json:"receipts"`
}
type HarnessPairingIntentDetailResponse struct {
	Data HarnessPairingIntentDetailData `json:"data"`
}
type HarnessConnectionListData struct {
	Connections []ConnectionView `json:"connections"`
}
type HarnessConnectionListResponse struct {
	Data HarnessConnectionListData `json:"data"`
}
type HarnessConnectionDetailData struct {
	Connection ConnectionView         `json:"connection"`
	Receipts   []AuthorityReceiptView `json:"receipts"`
}
type HarnessConnectionDetailResponse struct {
	Data HarnessConnectionDetailData `json:"data"`
}
