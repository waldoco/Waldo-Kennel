package ports

import (
	"context"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
)

type AdmissionStage string

const (
	AdmissionStageContract AdmissionStage = "contract_intelligence"
	AdmissionStageProposal AdmissionStage = "proposal_routing"
	AdmissionStageApproval AdmissionStage = "approval"
	AdmissionStageStart    AdmissionStage = "attempt_start"
)

type AdmissionStageInput struct {
	Stage           AdmissionStage
	ProjectID       domain.ProjectID
	Outcome         *domain.Outcome
	Contract        *domain.ContractRevision
	Plan            *domain.PlanRevision
	RoutingSnapshot *RoutingInventorySnapshot
}
type AdmissionStageResult struct {
	Eligible bool
	Verdict  domain.AdmissionVerdict
	Snapshot *RoutingInventorySnapshot
}
type AdmissionStageEvaluator interface {
	EvaluateAdmissionStage(context.Context, AdmissionStageInput) (AdmissionStageResult, error)
}
