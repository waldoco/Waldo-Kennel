package harnesspairing

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"
)

// ErrPairingFailed is the only error Client returns for a rejected request.
// It deliberately carries no detail: the daemon-local audit code that
// produced the rejection is never exposed to this transport.
var ErrPairingFailed = errors.New("harnesspairing: pairing request failed")

// Client is the thin adapter-side transport client. RequestChallenge carries
// only an owner-opened intent ID. Prove echoes the already-bound tuple and secret.
type Client struct {
	socketPath string
	timeout    time.Duration
}

func NewClient(socketPath string) *Client {
	return &Client{socketPath: socketPath, timeout: 5 * time.Second}
}

type ChallengeRequest struct {
	IntentID, IntentDigest                                                                                                        string
	Kind, ConnectionID, InstallationID, AdapterDigest, HarnessIdentity, ProviderVersion, ProtocolFingerprint, MissionID, AppRunID string
	CapabilityClasses                                                                                                             []string
	ExpectedGeneration                                                                                                            int64
}

// ChallengeIssued carries the one-time secret. Callers must not log it,
// place it in argv or environment variables, or write it to a file that is
// not private (0600) and short-lived.
type ChallengeIssued struct {
	ChallengeID string
	Secret      string
}

func (c *Client) RequestChallenge(req ChallengeRequest) (ChallengeIssued, error) {
	wire := wireRequestChallenge{Type: wireTypeRequestChallenge, IntentID: req.IntentID, IntentDigest: req.IntentDigest}
	var resp wireChallengeIssued
	if err := c.roundTrip(wire, &resp); err != nil {
		return ChallengeIssued{}, err
	}
	if !resp.OK {
		return ChallengeIssued{}, ErrPairingFailed
	}
	return ChallengeIssued{ChallengeID: resp.ChallengeID, Secret: resp.Secret}, nil
}

type ProveRequest struct {
	ChallengeID         string
	Secret              string
	ConnectionID        string
	InstallationID      string
	AdapterDigest       string
	HarnessIdentity     string
	ProviderVersion     string
	ProtocolFingerprint string
	MissionID           string
	AppRunID            string
	CapabilityClasses   []string
	ExpectedGeneration  int64
}

// Prove returns the S3.1 bearer on success. Any failure — replay, expiry,
// wrong tuple, wrong secret, unknown challenge — returns exactly
// ErrPairingFailed with no further distinction available to this caller.
func (c *Client) Prove(req ProveRequest) (string, error) {
	wire := wireProve{
		Type: wireTypeProve, ChallengeID: req.ChallengeID, Secret: req.Secret, ConnectionID: req.ConnectionID, InstallationID: req.InstallationID,
		AdapterDigest: req.AdapterDigest, HarnessIdentity: req.HarnessIdentity, ProviderVersion: req.ProviderVersion,
		ProtocolFingerprint: req.ProtocolFingerprint, MissionID: req.MissionID, AppRunID: req.AppRunID,
		CapabilityClasses: req.CapabilityClasses, ExpectedGeneration: req.ExpectedGeneration,
	}
	var resp wireProveResult
	if err := c.roundTrip(wire, &resp); err != nil {
		return "", err
	}
	if !resp.OK {
		return "", ErrPairingFailed
	}
	return resp.Bearer, nil
}

func (c *Client) roundTrip(req any, resp any) error {
	conn, err := net.DialTimeout("unix", c.socketPath, c.timeout)
	if err != nil {
		return fmt.Errorf("harnesspairing: dial: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(c.timeout))
	line, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("harnesspairing: encode request: %w", err)
	}
	line = append(line, '\n')
	if _, err := conn.Write(line); err != nil {
		return fmt.Errorf("harnesspairing: write request: %w", err)
	}
	reader := bufio.NewReader(conn)
	respLine, err := reader.ReadBytes('\n')
	if err != nil && len(respLine) == 0 {
		return fmt.Errorf("harnesspairing: read response: %w", err)
	}
	if err := json.Unmarshal(respLine, resp); err != nil {
		return fmt.Errorf("harnesspairing: decode response: %w", err)
	}
	return nil
}
