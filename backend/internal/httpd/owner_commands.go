package httpd

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/harnessauthority"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/harnesspairing"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/httpd/envelope"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ownercommand"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ownerproof"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type replacementDecisionRequest struct {
	OutcomeID              domain.OutcomeID      `json:"outcomeId"`
	PredecessorAttemptID   domain.AttemptID      `json:"predecessorAttemptId"`
	PlanRevisionID         domain.PlanRevisionID `json:"planRevisionId"`
	WorkUnitID             domain.WorkUnitID     `json:"workUnitId"`
	RunIntentGeneration    int64                 `json:"runIntentGeneration"`
	ContractRevisionNumber int64                 `json:"contractRevisionNumber"`
	Action                 string                `json:"action"`
	RequestKey             string                `json:"requestKey"`
}
type replacementDecisionFingerprint struct {
	OwnerPrincipal string `json:"ownerPrincipal"`
	replacementDecisionRequest
}

func mountOwnerCommands(r chi.Router, authority *ownercommand.Authority, store ports.AttemptReplacementDecisionStore, pairing *harnesspairing.Coordinator, proofs *ownerproof.Kernel, harnesses *harnessauthority.Service) {
	if authority == nil {
		return
	}
	if store != nil {
		r.Post("/internal/owner-commands/attempt-replacement-decisions", func(w http.ResponseWriter, req *http.Request) {
			if !localControlRequest(req) {
				notFoundJSON(w, req)
				return
			}
			authentication, ok := authority.Authenticate(req.Header.Get("Authorization"))
			if !ok {
				envelope.WriteJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]any{"code": "OWNER_COMMAND_UNAUTHORIZED", "message": "Trusted local-owner command authentication failed"}})
				return
			}
			var in replacementDecisionRequest
			dec := json.NewDecoder(http.MaxBytesReader(w, req.Body, 16<<10))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&in); err != nil {
				envelope.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "OWNER_COMMAND_INVALID", "message": "Replacement decision body is invalid"}})
				return
			}
			if dec.Decode(&struct{}{}) != io.EOF {
				envelope.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "OWNER_COMMAND_INVALID", "message": "Replacement decision body must contain exactly one JSON object"}})
				return
			}
			if strings.TrimSpace(in.Action) != "replace" {
				envelope.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "OWNER_COMMAND_INVALID", "message": "Replacement decision action must be replace"}})
				return
			}
			fp, err := ownercommand.Fingerprint(replacementDecisionFingerprint{OwnerPrincipal: authentication.Principal, replacementDecisionRequest: in})
			if err != nil {
				envelope.WriteJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"code": "OWNER_COMMAND_FAILED", "message": "Replacement decision could not be bound"}})
				return
			}
			d := domain.AttemptReplacementDecision{ID: domain.AttemptReplacementDecisionID("replacement-decision-" + uuid.NewString()), OutcomeID: in.OutcomeID, PredecessorAttemptID: in.PredecessorAttemptID, PlanRevisionID: in.PlanRevisionID, WorkUnitID: in.WorkUnitID, RunIntentGeneration: in.RunIntentGeneration, ContractRevisionNumber: in.ContractRevisionNumber, Action: "replace", RequestKey: strings.TrimSpace(in.RequestKey), RequestFingerprint: fp, OwnerPrincipal: authentication.Principal, CreatedAt: time.Now().UTC()}
			if err := d.Validate(); err != nil {
				envelope.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "OWNER_COMMAND_INVALID", "message": "Replacement decision body is invalid"}})
				return
			}
			stored, created, err := store.CreateAttemptReplacementDecision(req.Context(), d)
			if err != nil {
				status, code, message := http.StatusInternalServerError, "OWNER_COMMAND_FAILED", "Replacement decision could not be stored"
				switch err.(type) {
				case *ports.AttemptReplacementDecisionConflictError:
					status, code, message = http.StatusConflict, "OWNER_COMMAND_CONFLICT", "The request key already names a different replacement decision"
				case *ports.AttemptReplacementDecisionBindingError:
					status, code, message = http.StatusConflict, "OWNER_COMMAND_STALE", "The replacement decision no longer matches current Outcome authority"
				}
				envelope.WriteJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message}})
				return
			}
			status := http.StatusOK
			if created {
				status = http.StatusCreated
			}
			envelope.WriteJSON(w, status, map[string]any{"data": map[string]any{"decision": stored, "created": created}})
		})
	}
	if harnesses == nil {
		mountPairingIntentOwnerCommand(r, authority, pairing)
	}
	mountOwnerProofCommand(r, authority, proofs)
	mountHarnessAuthorityCommands(r, authority, harnesses)
}

type pairingIntentRequest struct {
	Kind                domain.HarnessPairingKind       `json:"kind"`
	ConnectionID        domain.HarnessConnectionID      `json:"connectionId"`
	InstallationID      string                          `json:"installationId"`
	AdapterDigest       domain.SHA256Digest             `json:"adapterDigest"`
	HarnessIdentity     string                          `json:"harnessIdentity"`
	ProviderVersion     string                          `json:"providerVersion"`
	ProtocolFingerprint domain.SHA256Digest             `json:"protocolFingerprint"`
	MissionID           string                          `json:"missionId"`
	CapabilityClasses   []domain.HarnessCapabilityClass `json:"capabilityClasses"`
	ExpectedGeneration  int64                           `json:"expectedGeneration"`
}

func mountPairingIntentOwnerCommand(r chi.Router, authority *ownercommand.Authority, coordinator *harnesspairing.Coordinator) {
	if coordinator == nil {
		return
	}
	r.Post("/internal/owner-commands/harness-pairing-intents", func(w http.ResponseWriter, req *http.Request) {
		if !localControlRequest(req) {
			notFoundJSON(w, req)
			return
		}
		authentication, ok := authority.Authenticate(req.Header.Get("Authorization"))
		if !ok {
			envelope.WriteJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]any{"code": "OWNER_COMMAND_UNAUTHORIZED", "message": "Trusted local-owner command authentication failed"}})
			return
		}
		var in pairingIntentRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, req.Body, 16<<10))
		dec.DisallowUnknownFields()
		if dec.Decode(&in) != nil || dec.Decode(&struct{}{}) != io.EOF {
			envelope.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "PAIRING_INTENT_INVALID", "message": "Pairing intent body is invalid"}})
			return
		}
		now := time.Now().UTC()
		issued, err := coordinator.Issue(req.Context(), harnesspairing.IssueChallengeRequest{Kind: in.Kind, ConnectionID: in.ConnectionID, InstallationID: in.InstallationID, AdapterDigest: in.AdapterDigest, HarnessIdentity: in.HarnessIdentity, ProviderVersion: in.ProviderVersion, ProtocolFingerprint: in.ProtocolFingerprint, MissionID: in.MissionID, AppRunID: authentication.AppRunID, CapabilityClasses: in.CapabilityClasses, ExpectedGeneration: in.ExpectedGeneration, ConnectionExpiresAt: now.Add(24 * time.Hour), TTL: 2 * time.Minute, Now: now})
		if err != nil {
			envelope.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "PAIRING_INTENT_INVALID", "message": "Pairing intent could not be opened"}})
			return
		}
		envelope.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"intentId": issued.Challenge.ID, "secret": string(issued.Secret), "expiresAt": issued.Challenge.ExpiresAt}})
	})
}

type ownerProofRequest struct {
	MissionID     string                   `json:"missionId"`
	ContentDigest domain.SHA256Digest      `json:"contentDigest"`
	Target        domain.OwnerProofTarget  `json:"target"`
	Class         domain.OwnerCommandClass `json:"class"`
}

func mountOwnerProofCommand(r chi.Router, authority *ownercommand.Authority, kernel *ownerproof.Kernel) {
	if kernel == nil {
		return
	}
	r.Post("/internal/owner-commands/owner-proofs", func(w http.ResponseWriter, req *http.Request) {
		if !localControlRequest(req) {
			notFoundJSON(w, req)
			return
		}
		authentication, ok := authority.Authenticate(req.Header.Get("Authorization"))
		if !ok {
			envelope.WriteJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]any{"code": "OWNER_COMMAND_UNAUTHORIZED", "message": "Trusted local-owner command authentication failed"}})
			return
		}
		var in ownerProofRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, req.Body, 16<<10))
		dec.DisallowUnknownFields()
		if dec.Decode(&in) != nil || dec.Decode(&struct{}{}) != io.EOF || !in.Class.Valid() || in.Class.Material() {
			envelope.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "OWNER_PROOF_INVALID", "message": "Routine owner proof body is invalid"}})
			return
		}
		if in.Target.Class != in.Class {
			envelope.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "OWNER_PROOF_INVALID", "message": "Routine owner proof body is invalid"}})
			return
		}
		targetDigest, err := in.Target.Digest()
		if err != nil {
			envelope.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "OWNER_PROOF_INVALID", "message": "Routine owner proof body is invalid"}})
			return
		}
		now := time.Now().UTC()
		minted, err := kernel.Mint(req.Context(), ownerproof.MintRequest{
			ID: domain.OwnerProofID("owner-proof-" + uuid.NewString()), AppRunID: authentication.AppRunID,
			MissionID: in.MissionID, ContentDigest: in.ContentDigest, TargetDigest: targetDigest,
			Class: in.Class, ExpiresAt: now.Add(2 * time.Minute), Now: now,
		})
		if err != nil || minted.Bearer == "" {
			envelope.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": "OWNER_PROOF_INVALID", "message": "Routine owner proof could not be minted"}})
			return
		}
		envelope.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"proofId": minted.Proof.ID, "bearer": minted.Bearer, "expiresAt": minted.Proof.ExpiresAt}})
	})
}

type harnessIntentCreateRequest struct {
	ProjectID           domain.ProjectID                `json:"projectId"`
	Kind                domain.HarnessPairingKind       `json:"kind"`
	ConnectionID        domain.HarnessConnectionID      `json:"connectionId"`
	InstallationID      string                          `json:"installationId"`
	AdapterDigest       domain.SHA256Digest             `json:"adapterDigest"`
	HarnessIdentity     string                          `json:"harnessIdentity"`
	ProviderVersion     string                          `json:"providerVersion"`
	ProtocolFingerprint domain.SHA256Digest             `json:"protocolFingerprint"`
	MissionID           string                          `json:"missionId"`
	CapabilityClasses   []domain.HarnessCapabilityClass `json:"capabilityClasses"`
	ExpectedGeneration  int64                           `json:"expectedGeneration"`
	ConnectionExpiresAt time.Time                       `json:"connectionExpiresAt"`
	ExpiresAt           time.Time                       `json:"expiresAt"`
	RequestKey          string                          `json:"requestKey"`
}
type harnessDecisionRequest struct {
	Digest     domain.SHA256Digest `json:"digest"`
	RequestKey string              `json:"requestKey"`
}
type harnessRevokeRequest struct {
	Digest             domain.SHA256Digest `json:"digest"`
	ExpectedGeneration int64               `json:"expectedGeneration"`
	RequestKey         string              `json:"requestKey"`
}

func mountHarnessAuthorityCommands(r chi.Router, authority *ownercommand.Authority, svc *harnessauthority.Service) {
	if svc == nil {
		return
	}
	authenticate := func(w http.ResponseWriter, req *http.Request) (ownercommand.Authentication, bool) {
		if !localControlRequest(req) {
			notFoundJSON(w, req)
			return ownercommand.Authentication{}, false
		}
		a, ok := authority.Authenticate(req.Header.Get("Authorization"))
		if !ok {
			envelope.WriteJSON(w, 401, map[string]any{"error": map[string]any{"code": "OWNER_COMMAND_UNAUTHORIZED", "message": "Trusted local-owner command authentication failed"}})
		}
		return a, ok
	}
	decode := func(w http.ResponseWriter, req *http.Request, out any) bool {
		d := json.NewDecoder(http.MaxBytesReader(w, req.Body, 16<<10))
		d.DisallowUnknownFields()
		if d.Decode(out) != nil || d.Decode(&struct{}{}) != io.EOF {
			envelope.WriteJSON(w, 400, map[string]any{"error": map[string]any{"code": "HARNESS_AUTHORITY_INVALID", "message": "Harness authority command body is invalid"}})
			return false
		}
		return true
	}
	r.Post("/internal/owner-commands/harness-pairing-proposals", func(w http.ResponseWriter, req *http.Request) {
		a, ok := authenticate(w, req)
		if !ok {
			return
		}
		var in harnessIntentCreateRequest
		if !decode(w, req, &in) {
			return
		}
		fingerprint, _ := ownercommand.Fingerprint(struct {
			AppRunID, RequestKey string
			Body                 harnessIntentCreateRequest
		}{a.AppRunID, strings.TrimSpace(in.RequestKey), in})
		v, created, err := svc.CreateIntent(req.Context(), harnessauthority.CreateIntentRequest{ProjectID: in.ProjectID, Kind: in.Kind, ConnectionID: in.ConnectionID, InstallationID: in.InstallationID, AdapterDigest: in.AdapterDigest, HarnessIdentity: in.HarnessIdentity, ProviderVersion: in.ProviderVersion, ProtocolFingerprint: in.ProtocolFingerprint, MissionID: in.MissionID, AppRunID: a.AppRunID, CapabilityClasses: in.CapabilityClasses, ExpectedGeneration: in.ExpectedGeneration, ConnectionExpiresAt: in.ConnectionExpiresAt, ExpiresAt: in.ExpiresAt, RequestKey: in.RequestKey, RequestFingerprint: fingerprint})
		if err != nil {
			envelope.WriteJSON(w, 400, map[string]any{"error": map[string]any{"code": "PAIRING_INTENT_INVALID", "message": "Pairing intent proposal is invalid"}})
			return
		}
		status := 200
		if created {
			status = 201
		}
		envelope.WriteJSON(w, status, map[string]any{"data": map[string]any{"intent": v, "created": created}})
	})
	decision := func(action string) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			a, ok := authenticate(w, req)
			if !ok {
				return
			}
			var in harnessDecisionRequest
			if !decode(w, req, &in) {
				return
			}
			id := domain.PairingChallengeID(chi.URLParam(req, "intentId"))
			fingerprint, _ := ownercommand.Fingerprint(struct {
				Action string
				ID     domain.PairingChallengeID
				Digest domain.SHA256Digest
				Key    string
			}{action, id, in.Digest, in.RequestKey})
			v, changed, err := svc.DecideIntent(req.Context(), harnessauthority.DecisionRequest{IntentID: id, Digest: in.Digest, Action: action, RequestKey: in.RequestKey, OwnerPrincipal: a.Principal, ConfirmationRef: "native-owner-command:" + fingerprint})
			if err != nil {
				writeHarnessAuthorityCommandError(w, err)
				return
			}
			envelope.WriteJSON(w, 200, map[string]any{"data": map[string]any{"intent": v, "changed": changed}})
		}
	}
	r.Post("/internal/owner-commands/harness-pairing-intents/{intentId}/approve", decision("approve"))
	r.Post("/internal/owner-commands/harness-pairing-intents/{intentId}/deny", decision("deny"))
	r.Post("/internal/owner-commands/harness-connections/{connectionId}/revoke", func(w http.ResponseWriter, req *http.Request) {
		a, ok := authenticate(w, req)
		if !ok {
			return
		}
		var in harnessRevokeRequest
		if !decode(w, req, &in) {
			return
		}
		id := domain.HarnessConnectionID(chi.URLParam(req, "connectionId"))
		fingerprint, _ := ownercommand.Fingerprint(struct {
			Action     string
			ID         domain.HarnessConnectionID
			Digest     domain.SHA256Digest
			Generation int64
			Key        string
		}{"revoke", id, in.Digest, in.ExpectedGeneration, in.RequestKey})
		v, changed, n, err := svc.Revoke(req.Context(), harnessauthority.RevokeRequest{ConnectionID: id, Digest: in.Digest, ExpectedGeneration: in.ExpectedGeneration, RequestKey: in.RequestKey, OwnerPrincipal: a.Principal, ConfirmationRef: "native-owner-command:" + fingerprint})
		if err != nil {
			writeHarnessAuthorityCommandError(w, err)
			return
		}
		envelope.WriteJSON(w, 200, map[string]any{"data": map[string]any{"connection": v, "changed": changed, "actionNeededCommands": n}})
	})
}
func writeHarnessAuthorityCommandError(w http.ResponseWriter, err error) {
	status, code := 500, "HARNESS_AUTHORITY_FAILED"
	if errors.Is(err, domain.ErrHarnessAuthorityInvalid) {
		status, code = 400, "HARNESS_AUTHORITY_INVALID"
	}
	if errors.Is(err, domain.ErrHarnessAuthorityStale) {
		status, code = 409, "HARNESS_AUTHORITY_STALE"
	}
	if errors.Is(err, domain.ErrHarnessAuthorityConflict) {
		status, code = 409, "HARNESS_AUTHORITY_CONFLICT"
	}
	envelope.WriteJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": "Harness authority command was not applied"}})
}
