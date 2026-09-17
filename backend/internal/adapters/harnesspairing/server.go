package harnesspairing

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/harnesspairing"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

const connectionDeadline = 10 * time.Second

// ServerConfig configures Server. Coordinator is required; every other field
// has a fail-safe default.
type ServerConfig struct {
	Coordinator *harnesspairing.Coordinator
	// PeerVerifier defaults to the platform's NewLocalPeerVerifier. Overriding
	// it is intended for tests only — production callers should leave it nil.
	PeerVerifier ports.LocalPeerVerifier
	// Now defaults to time.Now. Overriding it is intended for tests only.
	Now func() time.Time
	// ChallengeTTL bounds how long an issued challenge remains provable.
	// The adapter cannot influence this: it is fixed by the server at
	// issue time, matching invariant 2 (the adapter selects no expiry).
	ChallengeTTL time.Duration
	// ConnectionTTL is the expiry the server assigns to the resulting S3.1
	// connection generation on a successful proof.
	ConnectionTTL time.Duration
	Logger        *slog.Logger
	// IntentStore reads intents created through the trusted owner/internal path.
	IntentStore ports.HarnessPairingChallengeStore
}

// Server is the thin protected local transport for the pairing/rotation
// challenge lifecycle. Each accepted connection serves exactly one RPC and
// then closes: there is no session affinity, so a stolen challenge ID can be
// replayed from any new connection — which the coordinator, not the
// transport, is responsible for rejecting.
type Server struct {
	listener      net.Listener
	coordinator   *harnesspairing.Coordinator
	peerVerifier  ports.LocalPeerVerifier
	now           func() time.Time
	challengeTTL  time.Duration
	connectionTTL time.Duration
	intents       ports.HarnessPairingChallengeStore
	logger        *slog.Logger
}

func NewServer(listener net.Listener, cfg ServerConfig) (*Server, error) {
	if listener == nil {
		return nil, fmt.Errorf("harnesspairing: listener is required")
	}
	if cfg.Coordinator == nil {
		return nil, fmt.Errorf("harnesspairing: coordinator is required")
	}
	if cfg.IntentStore == nil {
		return nil, fmt.Errorf("harnesspairing: owner-opened intent store is required")
	}
	verifier := cfg.PeerVerifier
	if verifier == nil {
		verifier = NewLocalPeerVerifier()
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	challengeTTL := cfg.ChallengeTTL
	if challengeTTL <= 0 {
		challengeTTL = 2 * time.Minute
	}
	connectionTTL := cfg.ConnectionTTL
	if connectionTTL <= 0 {
		connectionTTL = 24 * time.Hour
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		listener: listener, coordinator: cfg.Coordinator, intents: cfg.IntentStore, peerVerifier: verifier,
		now: now, challengeTTL: challengeTTL, connectionTTL: connectionTTL, logger: logger,
	}, nil
}

// Serve accepts connections until ctx is done or the listener closes.
func (s *Server) Serve(ctx context.Context) error {
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = s.listener.Close()
		case <-stop:
		}
	}()
	defer close(stop)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue
			}
			return err
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	if err := s.peerVerifier.VerifyLocalPeer(conn); err != nil {
		// No response of any kind to a peer that fails the platform identity
		// check: not even the generic failure frame. Refuse the transport
		// operation entirely rather than acknowledging a connection exists.
		s.logger.Warn("harnesspairing: rejected peer failing local identity check", "error", err)
		return
	}
	// The connection deadline is real wall-clock time, deliberately
	// independent of s.now (which only times challenge/bearer business logic
	// and may be an injected fixed clock in tests).
	_ = conn.SetDeadline(time.Now().Add(connectionDeadline))
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return
	}
	var probe struct {
		Type string `json:"type"`
	}
	if jsonErr := json.Unmarshal(line, &probe); jsonErr != nil {
		s.writeFailure(conn)
		return
	}
	switch probe.Type {
	case wireTypeRequestChallenge:
		s.handleRequestChallenge(conn, line)
	case wireTypeProve:
		s.handleProve(conn, line)
	default:
		s.writeFailure(conn)
	}
}

// writeFailure is the ONLY place this server writes a failure response, and
// it always writes the same package-level byte slice. This is what makes
// "every failure produces byte-identical bytes" a structural property of the
// code, not a claim to verify by inspection.
func (s *Server) writeFailure(conn net.Conn) {
	_, _ = conn.Write(genericFailure)
}

func (s *Server) handleRequestChallenge(conn net.Conn, line []byte) {
	var req wireRequestChallenge
	if json.Unmarshal(line, &req) != nil || s.intents == nil {
		s.writeFailure(conn)
		return
	}
	intent, found, err := s.intents.GetHarnessPairingChallenge(context.Background(), domain.PairingChallengeID(req.IntentID))
	now := s.now().UTC()
	if err != nil || !found || intent.Status != domain.HarnessPairingPending || !now.Before(intent.ExpiresAt) {
		s.writeFailure(conn)
		return
	}
	// The raw secret was delivered by the trusted intent-opening path, never minted
	// from adapter-supplied tuple data. This route confirms only that it is live.
	s.writeJSON(conn, wireChallengeIssued{OK: true, ChallengeID: string(intent.ID)})
}

func (s *Server) handleProve(conn net.Conn, line []byte) {
	var req wireProve
	if err := json.Unmarshal(line, &req); err != nil {
		s.writeFailure(conn)
		return
	}
	result, _, err := s.coordinator.Prove(context.Background(), harnesspairing.ProveRequest{
		ChallengeID:         domain.PairingChallengeID(req.ChallengeID),
		Secret:              domain.PairingChallengeSecret(req.Secret),
		ConnectionID:        domain.HarnessConnectionID(req.ConnectionID),
		InstallationID:      req.InstallationID,
		HarnessIdentity:     req.HarnessIdentity,
		ProviderVersion:     req.ProviderVersion,
		MissionID:           req.MissionID,
		AppRunID:            req.AppRunID,
		AdapterDigest:       domain.SHA256Digest(req.AdapterDigest),
		ProtocolFingerprint: domain.SHA256Digest(req.ProtocolFingerprint),
		CapabilityClasses:   toCapabilityClasses(req.CapabilityClasses),
		ExpectedGeneration:  req.ExpectedGeneration,
		Now:                 s.now().UTC(),
	})
	if err != nil {
		// Every failure reason the coordinator can produce — replay, expiry,
		// supersession, tuple mismatch, undeclared class, not-found, or an
		// internal error — reaches this one call. The discriminating
		// domain.HarnessPairingResultCode was already recorded as
		// daemon-local audit evidence inside Prove; it never reaches here as
		// anything more than "err != nil".
		s.writeFailure(conn)
		return
	}
	s.writeJSON(conn, wireProveResult{OK: true, Bearer: result.Issued.Bearer})
}

func (s *Server) writeJSON(conn net.Conn, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		s.writeFailure(conn)
		return
	}
	body = append(body, '\n')
	_, _ = conn.Write(body)
}

func toCapabilityClasses(in []string) []domain.HarnessCapabilityClass {
	out := make([]domain.HarnessCapabilityClass, len(in))
	for i, c := range in {
		out[i] = domain.HarnessCapabilityClass(c)
	}
	return out
}
