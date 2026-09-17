package harnesspairing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/harnessconnection"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/harnesspairing"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/storage/sqlite/sqlitetest"
)

func nopLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func jsonMarshalLine(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func jsonUnmarshalLine(line []byte, v any) error { return json.Unmarshal(bytes.TrimRight(line, "\n"), v) }

func newTestServer(t *testing.T, verifier ports.LocalPeerVerifier) (*Server, string, *harnessconnection.Kernel) {
	t.Helper()
	store := sqlitetest.MustOpen(t)
	kernel := harnessconnection.New(store)
	coordinator := harnesspairing.New(store, kernel)

	dir := filepath.Join(t.TempDir(), "sock")
	ln, sockPath, err := Listen(dir)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	fixedNow := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	srv, err := NewServer(ln, ServerConfig{
		Coordinator: coordinator, PeerVerifier: verifier, Now: func() time.Time { return fixedNow },
		ChallengeTTL: time.Minute, ConnectionTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = srv.Serve(ctx) }()
	return srv, sockPath, kernel
}

func testChallengeRequest() ChallengeRequest {
	return ChallengeRequest{
		Kind: "pair", ConnectionID: "hc-1", InstallationID: "installation",
		AdapterDigest: string(domain.DigestSHA256([]byte("adapter"))), HarnessIdentity: "codex", ProviderVersion: "0.154.0",
		ProtocolFingerprint: string(domain.DigestSHA256([]byte("protocol"))), MissionID: "mission", AppRunID: "app-run",
		CapabilityClasses: []string{"turn"}, ExpectedGeneration: 1,
	}
}

func TestServer_EndToEndPairThenAuthenticate(t *testing.T) {
	_, sockPath, kernel := newTestServer(t, allowPeerVerifier{})
	client := NewClient(sockPath)

	reqTuple := testChallengeRequest()
	issued, err := client.RequestChallenge(reqTuple)
	if err != nil {
		t.Fatalf("request challenge: %v", err)
	}
	if issued.ChallengeID == "" || issued.Secret == "" {
		t.Fatal("expected challenge id and secret")
	}

	bearer, err := client.Prove(ProveRequest{
		ChallengeID: issued.ChallengeID, Secret: issued.Secret, InstallationID: reqTuple.InstallationID,
		AdapterDigest: reqTuple.AdapterDigest, HarnessIdentity: reqTuple.HarnessIdentity, ProviderVersion: reqTuple.ProviderVersion,
		ProtocolFingerprint: reqTuple.ProtocolFingerprint, MissionID: reqTuple.MissionID, AppRunID: reqTuple.AppRunID,
		CapabilityClasses: reqTuple.CapabilityClasses, ExpectedGeneration: reqTuple.ExpectedGeneration,
	})
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	if bearer == "" {
		t.Fatal("expected a bearer")
	}

	binding := harnessconnection.Binding{
		ConnectionID: domain.HarnessConnectionID(reqTuple.ConnectionID), InstallationID: reqTuple.InstallationID,
		AdapterDigest: domain.SHA256Digest(reqTuple.AdapterDigest), HarnessIdentity: reqTuple.HarnessIdentity,
		ProviderVersion: reqTuple.ProviderVersion, ProtocolFingerprint: domain.SHA256Digest(reqTuple.ProtocolFingerprint),
		MissionID: reqTuple.MissionID, AppRunID: reqTuple.AppRunID, Generation: 1, Class: domain.HarnessCapabilityTurn,
	}
	if _, err := kernel.Authenticate(context.Background(), bearer, binding, time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("bearer from the wire must authenticate: %v", err)
	}
}

func TestServer_ReplayFromADifferentConnectionFails(t *testing.T) {
	_, sockPath, _ := newTestServer(t, allowPeerVerifier{})
	client := NewClient(sockPath)
	reqTuple := testChallengeRequest()
	issued, err := client.RequestChallenge(reqTuple)
	if err != nil {
		t.Fatal(err)
	}
	prove := ProveRequest{
		ChallengeID: issued.ChallengeID, Secret: issued.Secret, InstallationID: reqTuple.InstallationID,
		AdapterDigest: reqTuple.AdapterDigest, HarnessIdentity: reqTuple.HarnessIdentity, ProviderVersion: reqTuple.ProviderVersion,
		ProtocolFingerprint: reqTuple.ProtocolFingerprint, MissionID: reqTuple.MissionID, AppRunID: reqTuple.AppRunID,
		CapabilityClasses: reqTuple.CapabilityClasses, ExpectedGeneration: reqTuple.ExpectedGeneration,
	}
	// First proof, over its own fresh connection, succeeds.
	if _, err := client.Prove(prove); err != nil {
		t.Fatalf("first prove: %v", err)
	}
	// A second, brand-new connection replaying the same (now stolen or
	// simply retried) challenge+secret over the wire must fail.
	if _, err := client.Prove(prove); !errors.Is(err, ErrPairingFailed) {
		t.Fatalf("replay err = %v, want ErrPairingFailed", err)
	}
}

func TestServer_StolenChallengeWithoutSecretFails(t *testing.T) {
	_, sockPath, _ := newTestServer(t, allowPeerVerifier{})
	client := NewClient(sockPath)
	reqTuple := testChallengeRequest()
	issued, err := client.RequestChallenge(reqTuple)
	if err != nil {
		t.Fatal(err)
	}
	// An attacker who only observed the challenge ID (e.g. from a log line)
	// but not the secret cannot prove possession.
	_, err = client.Prove(ProveRequest{
		ChallengeID: issued.ChallengeID, Secret: "guessed-secret-value", InstallationID: reqTuple.InstallationID,
		AdapterDigest: reqTuple.AdapterDigest, HarnessIdentity: reqTuple.HarnessIdentity, ProviderVersion: reqTuple.ProviderVersion,
		ProtocolFingerprint: reqTuple.ProtocolFingerprint, MissionID: reqTuple.MissionID, AppRunID: reqTuple.AppRunID,
		CapabilityClasses: reqTuple.CapabilityClasses, ExpectedGeneration: reqTuple.ExpectedGeneration,
	})
	if !errors.Is(err, ErrPairingFailed) {
		t.Fatalf("err = %v, want ErrPairingFailed", err)
	}
}

// TestServer_EveryFailureReasonProducesIdenticalWireBytes is the invariant-4
// regression test: it drives the server's connection handler directly (over
// an in-memory net.Pipe, bypassing Listen/peer verification, which are
// tested separately) for a battery of distinct failure causes and asserts
// every single response is the exact same byte slice.
func TestServer_EveryFailureReasonProducesIdenticalWireBytes(t *testing.T) {
	store := sqlitetest.MustOpen(t)
	kernel := harnessconnection.New(store)
	coordinator := harnesspairing.New(store, kernel)
	fixedNow := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	srv := &Server{
		coordinator: coordinator, peerVerifier: allowPeerVerifier{}, now: func() time.Time { return fixedNow },
		challengeTTL: time.Minute, connectionTTL: time.Hour, logger: nopLogger(),
	}

	reqTuple := testChallengeRequest()
	issue := func() ChallengeIssued {
		client, serverConn := pipeClientServer(t, srv)
		defer client.Close()
		defer serverConn.Close()
		id, secret := requestChallengeOverConn(t, client, reqTuple)
		return ChallengeIssued{ChallengeID: id, Secret: secret}
	}

	fresh := issue()
	consumeOnce := func(issued ChallengeIssued) {
		client, serverConn := pipeClientServer(t, srv)
		defer client.Close()
		defer serverConn.Close()
		_ = proveOverConn(t, client, issued.ChallengeID, issued.Secret, reqTuple)
	}
	consumeOnce(fresh) // burn it so a second use is a replay

	expired := issue()

	cases := map[string][]byte{
		"malformed_json":     []byte("{not json\n"),
		"unknown_type":       mustJSONLine(t, map[string]string{"type": "not-a-real-type"}),
		"unknown_challenge":  proveLine(t, "does-not-exist", "any-secret", reqTuple),
		"replayed_challenge": proveLine(t, fresh.ChallengeID, fresh.Secret, reqTuple),
		"wrong_secret":       proveLine(t, expired.ChallengeID, "wrong-secret-entirely", reqTuple),
		"wrong_tuple_field":  proveLineWithMission(t, expired.ChallengeID, expired.Secret, reqTuple, "attacker-mission"),
	}

	var first []byte
	for name, line := range cases {
		client, serverConn := pipeClientServer(t, srv)
		if _, err := client.Write(line); err != nil {
			t.Fatalf("%s: write: %v", name, err)
		}
		resp := readLine(t, client)
		client.Close()
		serverConn.Close()
		if first == nil {
			first = resp
		} else if !bytes.Equal(first, resp) {
			t.Fatalf("%s: response %q differs from first failure response %q", name, resp, first)
		}
		if !bytes.Equal(resp, genericFailure) {
			t.Fatalf("%s: response %q, want the exact generic failure frame %q", name, resp, genericFailure)
		}
	}
}

func TestServer_RejectsPeerFailingIdentityCheck(t *testing.T) {
	_, sockPath, _ := newTestServer(t, denyPeerVerifier{})
	client := NewClient(sockPath)
	_, err := client.RequestChallenge(testChallengeRequest())
	if err == nil {
		t.Fatal("expected an error: the peer verifier denies every peer")
	}
}

// --- test doubles and helpers -------------------------------------------

type allowPeerVerifier struct{}

func (allowPeerVerifier) VerifyLocalPeer(net.Conn) error { return nil }

type denyPeerVerifier struct{}

func (denyPeerVerifier) VerifyLocalPeer(net.Conn) error { return errPeerDenied }

var errPeerDenied = &peerDeniedError{}

type peerDeniedError struct{}

func (*peerDeniedError) Error() string { return "peer denied for test" }

func pipeClientServer(t *testing.T, srv *Server) (net.Conn, net.Conn) {
	t.Helper()
	client, server := net.Pipe()
	go srv.handle(server)
	return client, server
}

func readLine(t *testing.T, conn net.Conn) []byte {
	t.Helper()
	buf := make([]byte, 4096)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil && n == 0 {
		t.Fatalf("read response: %v", err)
	}
	return buf[:n]
}

func mustJSONLine(t *testing.T, v any) []byte {
	t.Helper()
	b, err := jsonMarshalLine(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func proveLine(t *testing.T, challengeID, secret string, tuple ChallengeRequest) []byte {
	t.Helper()
	return mustJSONLine(t, wireProve{
		Type: wireTypeProve, ChallengeID: challengeID, Secret: secret, InstallationID: tuple.InstallationID,
		AdapterDigest: tuple.AdapterDigest, HarnessIdentity: tuple.HarnessIdentity, ProviderVersion: tuple.ProviderVersion,
		ProtocolFingerprint: tuple.ProtocolFingerprint, MissionID: tuple.MissionID, AppRunID: tuple.AppRunID,
		CapabilityClasses: tuple.CapabilityClasses, ExpectedGeneration: tuple.ExpectedGeneration,
	})
}

func proveLineWithMission(t *testing.T, challengeID, secret string, tuple ChallengeRequest, mission string) []byte {
	t.Helper()
	return mustJSONLine(t, wireProve{
		Type: wireTypeProve, ChallengeID: challengeID, Secret: secret, InstallationID: tuple.InstallationID,
		AdapterDigest: tuple.AdapterDigest, HarnessIdentity: tuple.HarnessIdentity, ProviderVersion: tuple.ProviderVersion,
		ProtocolFingerprint: tuple.ProtocolFingerprint, MissionID: mission, AppRunID: tuple.AppRunID,
		CapabilityClasses: tuple.CapabilityClasses, ExpectedGeneration: tuple.ExpectedGeneration,
	})
}

func requestChallengeOverConn(t *testing.T, conn net.Conn, tuple ChallengeRequest) (string, string) {
	t.Helper()
	line := mustJSONLine(t, wireRequestChallenge{
		Type: wireTypeRequestChallenge, Kind: tuple.Kind, ConnectionID: tuple.ConnectionID, InstallationID: tuple.InstallationID,
		AdapterDigest: tuple.AdapterDigest, HarnessIdentity: tuple.HarnessIdentity, ProviderVersion: tuple.ProviderVersion,
		ProtocolFingerprint: tuple.ProtocolFingerprint, MissionID: tuple.MissionID, AppRunID: tuple.AppRunID,
		CapabilityClasses: tuple.CapabilityClasses, ExpectedGeneration: tuple.ExpectedGeneration,
	})
	if _, err := conn.Write(line); err != nil {
		t.Fatal(err)
	}
	resp := readLine(t, conn)
	var parsed wireChallengeIssued
	if err := jsonUnmarshalLine(resp, &parsed); err != nil {
		t.Fatalf("decode challenge response %q: %v", resp, err)
	}
	if !parsed.OK {
		t.Fatalf("challenge issuance failed: %q", resp)
	}
	return parsed.ChallengeID, parsed.Secret
}

func proveOverConn(t *testing.T, conn net.Conn, challengeID, secret string, tuple ChallengeRequest) string {
	t.Helper()
	line := proveLine(t, challengeID, secret, tuple)
	if _, err := conn.Write(line); err != nil {
		t.Fatal(err)
	}
	resp := readLine(t, conn)
	var parsed wireProveResult
	if err := jsonUnmarshalLine(resp, &parsed); err != nil {
		t.Fatalf("decode prove response %q: %v", resp, err)
	}
	return parsed.Bearer
}

func TestListen_CreatesPrivateSocketAndDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sock")
	ln, sockPath, err := Listen(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("runtime dir perm = %o, want 0700", dirInfo.Mode().Perm())
	}
	sockInfo, err := os.Lstat(sockPath)
	if err != nil {
		t.Fatal(err)
	}
	if sockInfo.Mode().Perm() != 0o600 {
		t.Fatalf("socket perm = %o, want 0600", sockInfo.Mode().Perm())
	}
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(sockPath); !os.IsNotExist(err) {
		t.Fatal("expected the socket file to be removed on Close")
	}
}
