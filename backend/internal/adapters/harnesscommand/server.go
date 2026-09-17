// Package harnesscommand is the protected post-pair command ingress. It is a
// separate service from pairing and cannot issue or rotate pairing intents.
package harnesscommand

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/harnessconnection"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

const maxFrame = 256 << 10

type Server struct {
	listener    net.Listener
	peers       ports.LocalPeerVerifier
	connections *harnessconnection.Kernel
	store       ports.HarnessCommandStore
	now         func() time.Time
}
type ServerConfig struct {
	PeerVerifier ports.LocalPeerVerifier
	Connections  *harnessconnection.Kernel
	Store        ports.HarnessCommandStore
	Now          func() time.Time
}

func NewServer(listener net.Listener, cfg ServerConfig) (*Server, error) {
	if listener == nil || cfg.Connections == nil || cfg.Store == nil {
		return nil, fmt.Errorf("harnesscommand: listener, connection kernel and store are required")
	}
	p := cfg.PeerVerifier
	if p == nil {
		return nil, fmt.Errorf("harnesscommand: peer verifier is required")
	}
	n := cfg.Now
	if n == nil {
		n = time.Now
	}
	return &Server{listener: listener, peers: p, connections: cfg.Connections, store: cfg.Store, now: n}, nil
}
func (s *Server) Serve(ctx context.Context) error {
	go func() { <-ctx.Done(); _ = s.listener.Close() }()
	for {
		c, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go s.handle(c)
	}
}
func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	if s.peers.VerifyLocalPeer(conn) != nil {
		return
	}
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	line, err := bufio.NewReader(io.LimitReader(conn, maxFrame+1)).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return
	}
	if len(line) > maxFrame {
		s.fail(conn)
		return
	}
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.DisallowUnknownFields()
	var in request
	if dec.Decode(&in) != nil || dec.Decode(&struct{}{}) != io.EOF || in.Type != "command" {
		s.fail(conn)
		return
	}
	binding := domain.HarnessConnectionBinding{ConnectionID: domain.HarnessConnectionID(in.ConnectionID), InstallationID: in.InstallationID, AdapterDigest: domain.SHA256Digest(in.AdapterDigest), HarnessIdentity: in.HarnessIdentity, ProviderVersion: in.ProviderVersion, ProtocolFingerprint: domain.SHA256Digest(in.ProtocolFingerprint), MissionID: in.MissionID, AppRunID: in.AppRunID, Generation: in.ConnectionGeneration, Class: domain.HarnessCapabilityClass(in.TransportClass)}
	kernelBinding := harnessconnection.Binding{ConnectionID: binding.ConnectionID, InstallationID: binding.InstallationID, AdapterDigest: binding.AdapterDigest, HarnessIdentity: binding.HarnessIdentity, ProviderVersion: binding.ProviderVersion, ProtocolFingerprint: binding.ProtocolFingerprint, MissionID: binding.MissionID, AppRunID: binding.AppRunID, Generation: binding.Generation, Class: binding.Class}
	now := s.now().UTC()
	if _, err := s.connections.Authenticate(context.Background(), in.ConnectionBearer, kernelBinding, now); err != nil {
		s.fail(conn)
		return
	}
	claim, _, err := s.store.ValidateAuthoritiesAndCreateCommandClaim(context.Background(), ports.HarnessCommandRequest{ConnectionBearer: in.ConnectionBearer, ConnectionBinding: binding, OwnerProofID: domain.OwnerProofID(in.OwnerProofID), OwnerProofBearer: in.OwnerProofBearer, Target: in.Target, Command: in.Command, AdapterRequestKey: in.AdapterRequestKey, Now: now})
	if err != nil {
		s.fail(conn)
		return
	}
	s.write(conn, response{OK: true, ClaimID: claim.ID, DestinationID: claim.DestinationID})
}
func (s *Server) fail(c net.Conn) { _, _ = c.Write(genericFailure) }
func (s *Server) write(c net.Conn, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		s.fail(c)
		return
	}
	b = append(b, '\n')
	_, _ = c.Write(b)
}

var _ = errors.Is
