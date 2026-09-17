package harnesscommand

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/harnessconnection"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

type peerVerifier struct{ err error }

func (p peerVerifier) VerifyLocalPeer(net.Conn) error { return p.err }

type connectionStore struct{ record domain.HarnessConnection }

func (s connectionStore) CreateHarnessConnection(context.Context, domain.HarnessConnection) (domain.HarnessConnection, bool, error) {
	return domain.HarnessConnection{}, false, errors.New("unused")
}
func (s connectionStore) GetHarnessConnection(context.Context, domain.HarnessConnectionID) (domain.HarnessConnection, bool, error) {
	return s.record, true, nil
}
func (s connectionStore) RotateHarnessConnection(context.Context, domain.HarnessConnectionID, int64, string, time.Time, time.Time) (domain.HarnessConnection, bool, error) {
	return domain.HarnessConnection{}, false, errors.New("unused")
}
func (s connectionStore) RevokeHarnessConnection(context.Context, domain.HarnessConnectionID, int64, time.Time) (domain.HarnessConnection, bool, error) {
	return domain.HarnessConnection{}, false, errors.New("unused")
}

type commandStore struct {
	claim domain.CommandAuthorityClaim
	err   error
	calls int
}

func (s *commandStore) ValidateAuthoritiesAndCreateCommandClaim(_ context.Context, _ ports.HarnessCommandRequest) (domain.CommandAuthorityClaim, bool, error) {
	s.calls++
	return s.claim, s.err == nil, s.err
}
func (*commandStore) ListPendingHarnessCommandOutbox(context.Context) ([]domain.HarnessCommandOutboxRecord, error) {
	return nil, nil
}

func serverRequest() request {
	return request{Type: "command", ConnectionBearer: "bearer", ConnectionID: "hc", InstallationID: "install", AdapterDigest: string(domain.DigestSHA256([]byte("adapter"))), HarnessIdentity: "codex", ProviderVersion: "1", ProtocolFingerprint: string(domain.DigestSHA256([]byte("protocol"))), MissionID: "mission", AppRunID: "run", ConnectionGeneration: 1, TransportClass: "answer", OwnerProofID: "proof", OwnerProofBearer: "proof-bearer", Target: domain.OwnerProofTarget{Version: "v1", Class: domain.OwnerCommandAnswer, QuestionID: "q", QuestionGeneration: "g"}, Command: domain.CanonicalHarnessCommand{Version: "v1", Class: domain.OwnerCommandAnswer, DecisionJSON: `{"id":"yes"}`}, AdapterRequestKey: "request"}
}
func runHandle(t *testing.T, srv *Server, payload []byte) []byte {
	t.Helper()
	client, server := net.Pipe()
	done := make(chan struct{})
	go func() { srv.handle(server); close(done) }()
	if len(payload) > 0 {
		_, _ = client.Write(payload)
	}
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	line, _ := bufio.NewReader(client).ReadBytes('\n')
	_ = client.Close()
	<-done
	return line
}
func validServer(t *testing.T, peers ports.LocalPeerVerifier, commands *commandStore) *Server {
	t.Helper()
	now := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	sum := domain.DigestSHA256([]byte("bearer"))
	rec := domain.HarnessConnection{ID: "hc", InstallationID: "install", AdapterDigest: domain.DigestSHA256([]byte("adapter")), HarnessIdentity: "codex", ProviderVersion: "1", ProtocolFingerprint: domain.DigestSHA256([]byte("protocol")), MissionID: "mission", AppRunID: "run", CapabilityClasses: []domain.HarnessCapabilityClass{domain.HarnessCapabilityAnswer}, CapabilityVerifier: sum.String(), Generation: 1, ExpiresAt: now.Add(time.Hour), CreatedAt: now, UpdatedAt: now}
	return &Server{peers: peers, connections: harnessconnection.New(connectionStore{rec}), store: commands, now: func() time.Time { return now }}
}
func TestServerCreatesClaimFromAuthenticatedFrame(t *testing.T) {
	commands := &commandStore{claim: domain.CommandAuthorityClaim{ID: "claim", DestinationID: "destination"}}
	srv := validServer(t, peerVerifier{}, commands)
	payload, _ := json.Marshal(serverRequest())
	payload = append(payload, '\n')
	got := runHandle(t, srv, payload)
	if commands.calls != 1 || !bytes.Contains(got, []byte(`"ok":true`)) || !bytes.Contains(got, []byte(`"claim_id":"claim"`)) {
		t.Fatalf("calls=%d response=%s", commands.calls, got)
	}
}
func TestServerFailuresAreGenericAndPeerFailureReadsNothing(t *testing.T) {
	base := validServer(t, peerVerifier{}, &commandStore{})
	badAuth := serverRequest()
	badAuth.ConnectionBearer = "wrong"
	auth, _ := json.Marshal(badAuth)
	auth = append(auth, '\n')
	cases := [][]byte{[]byte("{bad\n"), auth}
	for _, in := range cases {
		if got := runHandle(t, base, in); !bytes.Equal(got, genericFailure) {
			t.Fatalf("response=%q", got)
		}
	}
	peerCommands := &commandStore{}
	peer := validServer(t, peerVerifier{err: errors.New("denied")}, peerCommands)
	client, server := net.Pipe()
	done := make(chan struct{})
	go func() { peer.handle(server); close(done) }()
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	b := make([]byte, 1)
	_, _ = client.Read(b)
	_ = client.Close()
	<-done
	if peerCommands.calls != 0 {
		t.Fatal("peer failure reached command store")
	}
}
