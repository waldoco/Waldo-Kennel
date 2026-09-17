package ports

import (
	"context"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

// AdmissionStore is the durable W1.1 boundary. Approval and its exact specs
// commit together; launch packets commit after workspace preparation and before
// a provider process may exist.
type AdmissionStore interface {
	GetAdmittedVerdict(context.Context, domain.PlanRevisionID) (domain.AdmissionVerdict, bool, error)
	AppendAdmissionEvaluation(context.Context, domain.AdmissionVerdict) error
	ApprovePlanWithAdmission(context.Context, domain.OutcomeID, domain.PlanRevisionID, domain.AdmissionVerdict) (domain.PlanRevision, bool, error)
	GetApprovedExecutableSpec(context.Context, domain.PlanRevisionID, domain.WorkUnitID) (domain.ApprovedExecutableSpec, bool, error)
	PersistWorkspaceBoundLaunchPacket(context.Context, domain.WorkspaceBoundLaunchPacket) error
	GetWorkspaceBoundLaunchPacket(context.Context, domain.AttemptID) (domain.WorkspaceBoundLaunchPacket, bool, error)
}
