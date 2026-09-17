package outcome

import (
	"context"
	"fmt"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

type LaunchRecoveryState string

const (
	LaunchLegacy          LaunchRecoveryState = "legacy"
	LaunchPrepared        LaunchRecoveryState = "prepared"
	LaunchPacketPersisted LaunchRecoveryState = "packet_persisted"
	LaunchUnconfirmed     LaunchRecoveryState = "launch_unconfirmed"
	LaunchRunning         LaunchRecoveryState = "running"
)

func (s *Service) launchRecoveryState(ctx context.Context, attempt domain.Attempt) (LaunchRecoveryState, error) {
	if s.admission == nil {
		return LaunchLegacy, nil
	}
	packet, found, err := s.admission.GetWorkspaceBoundLaunchPacket(ctx, attempt.ID)
	if err != nil {
		return "", err
	}
	if !found {
		if _, bound, refErr := s.store.LatestAttemptSessionRef(ctx, attempt.ID); refErr != nil {
			return "", refErr
		} else if bound {
			return LaunchLegacy, nil
		}
		verdict, admitted, verdictErr := s.admission.GetAdmittedVerdict(ctx, attempt.PlanRevisionID)
		if verdictErr != nil {
			return "", verdictErr
		}
		spec, specified, specErr := s.admission.GetApprovedExecutableSpec(ctx, attempt.PlanRevisionID, attempt.WorkUnitID)
		if specErr != nil {
			return "", specErr
		}
		if admitted && specified && verdict.OutcomeID == attempt.OutcomeID && spec.PlanRevisionID == attempt.PlanRevisionID && spec.WorkUnitID == attempt.WorkUnitID {
			return LaunchPrepared, nil
		}
		return LaunchLegacy, nil
	}
	ref, bound, err := s.store.LatestAttemptSessionRef(ctx, attempt.ID)
	if err != nil {
		return "", err
	}
	if !bound {
		return LaunchPacketPersisted, nil
	}
	if ref.SessionID != packet.SessionID {
		return "", fmt.Errorf("attempt session and launch packet identities disagree")
	}
	if attempt.Status == domain.AttemptRunning {
		return LaunchRunning, nil
	}
	return LaunchUnconfirmed, nil
}
