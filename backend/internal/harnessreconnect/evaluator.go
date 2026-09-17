package harnessreconnect

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
)

type Request struct {
	MissionID, AppRunID, ExpectedDaemonInstanceID string
	Now                                           time.Time
	MaxObservationAge                             time.Duration
}
type Evaluator struct {
	Daemon      ports.HarnessDaemonAttacher
	Profiles    ports.HarnessMissionProfileReader
	Connections ports.HarnessConnectionStore
	Delivery    ports.HarnessDeliveryBlockReader
	Pairing     ports.HarnessPairingFactsReader
}

func (e Evaluator) Evaluate(ctx context.Context, r Request) (domain.HarnessReconnectEvaluation, error) {
	if err := ctx.Err(); err != nil {
		return domain.HarnessReconnectEvaluation{}, err
	}
	if strings.TrimSpace(r.MissionID) == "" || strings.TrimSpace(r.AppRunID) == "" || r.Now.IsZero() {
		return domain.HarnessReconnectEvaluation{}, domain.ErrHarnessReconnectInvalid
	}
	if r.MaxObservationAge <= 0 {
		r.MaxObservationAge = 30 * time.Second
	}
	result := domain.HarnessReconnectEvaluation{State: domain.HarnessActionNeeded, MissionID: r.MissionID, EvaluatedAt: r.Now.UTC()}
	if e.Daemon == nil {
		return set(result, domain.ReconnectReasonDaemonUnreachable, domain.ReconnectRepairStartOrRepairDaemon), nil
	}
	daemon, err := e.Daemon.AttachHarnessDaemon(ctx)
	if err != nil {
		return set(result, domain.ReconnectReasonDaemonUnreachable, domain.ReconnectRepairStartOrRepairDaemon), nil
	}
	if daemon.Validate() != nil || !daemon.APICompatible || r.ExpectedDaemonInstanceID != "" && daemon.InstanceID != r.ExpectedDaemonInstanceID {
		return set(result, domain.ReconnectReasonDaemonIdentityMismatch, domain.ReconnectRepairDaemonEndpoint), nil
	}
	if stale(r.Now, daemon.ObservedAt, r.MaxObservationAge) {
		return set(result, domain.ReconnectReasonObservationStale, domain.ReconnectRepairRefreshObservation), nil
	}
	result.DaemonInstanceID = daemon.InstanceID
	if e.Profiles == nil {
		return set(result, domain.ReconnectReasonMissionProfileMissing, domain.ReconnectRepairMissionProfile), nil
	}
	profile, ok, err := e.Profiles.ReadHarnessMissionProfile(ctx, r.MissionID)
	if err != nil {
		return result, err
	}
	if !ok {
		return set(result, domain.ReconnectReasonMissionNotFound, domain.ReconnectRepairChooseMission), nil
	}
	if profile.Validate() != nil || profile.MissionID != r.MissionID {
		return set(result, domain.ReconnectReasonMissionProfileMissing, domain.ReconnectRepairMissionProfile), nil
	}
	if e.Delivery != nil {
		block, blocked, err := e.Delivery.ReadHarnessDeliveryBlock(ctx, r.MissionID)
		if err != nil {
			return result, err
		}
		if blocked {
			if block.Validate() != nil {
				return result, domain.ErrHarnessReconnectInvalid
			}
			return set(result, domain.ReconnectReasonDeliveryUnknown, domain.ReconnectRepairDeliveryEvidence), nil
		}
	}
	if e.Pairing == nil {
		e.Pairing = ports.UnavailableHarnessPairingFacts{}
	}
	observed, err := e.Pairing.ReadHarnessPairingFacts(ctx, profile.ConnectionID)
	if errors.Is(err, domain.ErrHarnessPairingFactsUnavailable) {
		return set(result, domain.ReconnectReasonPairingFactsUnavailable, domain.ReconnectRepairPairAdapter), nil
	}
	if err != nil {
		return result, err
	}
	if observed.Validate() != nil {
		return result, domain.ErrHarnessReconnectInvalid
	}
	if stale(r.Now, observed.ObservedAt, r.MaxObservationAge) {
		return set(result, domain.ReconnectReasonObservationStale, domain.ReconnectRepairRefreshObservation), nil
	}
	if e.Connections == nil {
		return set(result, domain.ReconnectReasonPairingFactsUnavailable, domain.ReconnectRepairPairAdapter), nil
	}
	connection, ok, err := e.Connections.GetHarnessConnection(ctx, profile.ConnectionID)
	if err != nil {
		return result, err
	}
	if !ok {
		return fromConnection(result, domain.EvaluateHarnessConnection(nil, domain.HarnessConnectionFacts{})), nil
	}
	facts := domain.HarnessConnectionFacts{InstallationID: observed.InstallationID, AdapterDigest: observed.AdapterDigest, HarnessIdentity: observed.HarnessIdentity, MissionID: observed.MissionID, AppRunID: observed.AppRunID, ProtocolFingerprint: observed.ProtocolFingerprint, Generation: observed.Generation, RequiredCapabilities: profile.RequiredCapabilities, OptionalCapabilities: profile.OptionalCapabilities, Now: r.Now.UTC()}
	if observed.ProviderVersion != profile.ProviderVersion || profile.InstallationID != observed.InstallationID || profile.AdapterDigest != observed.AdapterDigest || profile.ProtocolFingerprint != observed.ProtocolFingerprint || profile.Generation != observed.Generation {
		return fromConnection(result, domain.HarnessConnectionEvaluation{State: domain.HarnessActionNeeded, Reason: domain.HarnessReasonBindingMismatch, Repair: domain.HarnessRepairPairing}), nil
	}
	result.ConnectionGeneration = connection.Generation
	return fromConnection(result, domain.EvaluateHarnessConnection(&connection, facts)), nil
}
func stale(now, observed time.Time, max time.Duration) bool {
	return observed.After(now) || now.Sub(observed) > max
}
func set(r domain.HarnessReconnectEvaluation, reason domain.HarnessReconnectReason, repair domain.HarnessReconnectRepair) domain.HarnessReconnectEvaluation {
	r.Reason = string(reason)
	r.Repair = string(repair)
	return r
}
func fromConnection(r domain.HarnessReconnectEvaluation, c domain.HarnessConnectionEvaluation) domain.HarnessReconnectEvaluation {
	r.State = c.State
	r.Reason = string(c.Reason)
	r.Repair = string(c.Repair)
	return r
}
