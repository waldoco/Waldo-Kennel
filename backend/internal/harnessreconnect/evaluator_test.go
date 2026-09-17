package harnessreconnect

import (
	"context"
	"errors"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"testing"
	"time"
)

type daemon struct {
	v   domain.HarnessDaemonReadiness
	err error
}

func (d daemon) AttachHarnessDaemon(context.Context) (domain.HarnessDaemonReadiness, error) {
	return d.v, d.err
}

type profiles struct {
	v  domain.HarnessMissionProfile
	ok bool
}

func (p profiles) ReadHarnessMissionProfile(context.Context, string) (domain.HarnessMissionProfile, bool, error) {
	return p.v, p.ok, nil
}

type delivery struct {
	v  domain.HarnessDeliveryBlock
	ok bool
}

func (d delivery) ReadHarnessDeliveryBlock(context.Context, string) (domain.HarnessDeliveryBlock, bool, error) {
	return d.v, d.ok, nil
}

type pairing struct {
	v   domain.HarnessPairingObservation
	err error
}

func (p pairing) ReadHarnessPairingFacts(context.Context, domain.HarnessConnectionID) (domain.HarnessPairingObservation, error) {
	return p.v, p.err
}

type connections struct {
	v  domain.HarnessConnection
	ok bool
}

func (c connections) CreateHarnessConnection(context.Context, domain.HarnessConnection) (domain.HarnessConnection, bool, error) {
	panic("not used")
}
func (c connections) GetHarnessConnection(context.Context, domain.HarnessConnectionID) (domain.HarnessConnection, bool, error) {
	return c.v, c.ok, nil
}
func (c connections) RotateHarnessConnection(context.Context, domain.HarnessConnectionID, int64, string, time.Time, time.Time) (domain.HarnessConnection, bool, error) {
	panic("not used")
}
func (c connections) RevokeHarnessConnection(context.Context, domain.HarnessConnectionID, int64, time.Time) (domain.HarnessConnection, bool, error) {
	panic("not used")
}
func fixture() (Request, Evaluator) {
	now := time.Date(2026, 9, 17, 5, 0, 0, 0, time.UTC)
	ad := domain.DigestSHA256([]byte("a"))
	pd := domain.DigestSHA256([]byte("p"))
	profile := domain.HarnessMissionProfile{MissionID: "m", ConnectionID: "hc", InstallationID: "i", AdapterDigest: ad, HarnessIdentity: "codex", ProviderVersion: "1.0.0", ProtocolFingerprint: pd, Generation: 1, RequiredCapabilities: []domain.HarnessCapabilityClass{domain.HarnessCapabilityTurn}}
	obs := domain.HarnessPairingObservation{InstallationID: "i", AdapterDigest: ad, HarnessIdentity: "codex", ProviderVersion: "1.0.0", ProtocolFingerprint: pd, MissionID: "m", AppRunID: "run", Generation: 1, ObservedAt: now}
	conn := domain.HarnessConnection{ID: "hc", InstallationID: "i", AdapterDigest: ad, HarnessIdentity: "codex", ProviderVersion: "1.0.0", ProtocolFingerprint: pd, MissionID: "m", AppRunID: "run", CapabilityClasses: []domain.HarnessCapabilityClass{domain.HarnessCapabilityTurn}, Generation: 1, ExpiresAt: now.Add(time.Hour), CreatedAt: now.Add(-time.Hour), UpdatedAt: now, CapabilityVerifier: domain.DigestSHA256([]byte("secret")).String()}
	return Request{MissionID: "m", AppRunID: "run", Now: now, MaxObservationAge: time.Minute}, Evaluator{Daemon: daemon{v: domain.HarnessDaemonReadiness{InstanceID: "d", Generation: 1, APICompatible: true, ObservedAt: now}}, Profiles: profiles{v: profile, ok: true}, Connections: connections{v: conn, ok: true}, Delivery: delivery{}, Pairing: pairing{v: obs}}
}
func TestEvaluateConnected(t *testing.T) {
	r, e := fixture()
	got, err := e.Evaluate(context.Background(), r)
	if err != nil || got.State != domain.HarnessConnected || got.Reason != "" || got.Repair != "" {
		t.Fatalf("got %+v err=%v", got, err)
	}
}
func TestEvaluatePrecedence(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Request, *Evaluator)
		reason string
	}{{"daemon", func(_ *Request, e *Evaluator) { e.Daemon = nil }, string(domain.ReconnectReasonDaemonUnreachable)}, {"mission", func(_ *Request, e *Evaluator) { e.Profiles = profiles{} }, string(domain.ReconnectReasonMissionNotFound)}, {"delivery", func(r *Request, e *Evaluator) {
		e.Delivery = delivery{ok: true, v: domain.HarnessDeliveryBlock{CommandID: "cmd", CommandClass: "turn", DispatchGeneration: 1, ObservedAt: r.Now}}
		e.Pairing = nil
	}, string(domain.ReconnectReasonDeliveryUnknown)}, {"pairing", func(_ *Request, e *Evaluator) { e.Pairing = nil }, string(domain.ReconnectReasonPairingFactsUnavailable)}, {"stale", func(r *Request, e *Evaluator) {
		p := e.Pairing.(pairing)
		p.v.ObservedAt = r.Now.Add(-2 * time.Minute)
		e.Pairing = p
	}, string(domain.ReconnectReasonObservationStale)}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, e := fixture()
			tc.mutate(&r, &e)
			got, err := e.Evaluate(context.Background(), r)
			if err != nil || got.State != domain.HarnessActionNeeded || got.Reason != tc.reason || got.Repair == "" {
				t.Fatalf("got %+v err=%v", got, err)
			}
		})
	}
}
func TestPairingExplicitUnavailable(t *testing.T) {
	r, e := fixture()
	e.Pairing = pairing{err: domain.ErrHarnessPairingFactsUnavailable}
	got, err := e.Evaluate(context.Background(), r)
	if err != nil || got.Reason != string(domain.ReconnectReasonPairingFactsUnavailable) {
		t.Fatalf("got %+v err %v", got, err)
	}
}
func TestCancelled(t *testing.T) {
	r, e := fixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := e.Evaluate(ctx, r)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
}

var _ ports.HarnessConnectionStore = connections{}
