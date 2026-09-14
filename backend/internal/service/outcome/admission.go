package outcome

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Pin4sf/Waldo-Kennel/backend/internal/domain"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/ports"
	"github.com/google/uuid"
)

const (
	admissionPolicyVersion         = "w1.1-v1"
	admissionCompilerPolicyVersion = "runbrief-v1"
	admissionNativeMappingVersion  = "local-harness-v1"
)

// admissionEvaluator is the one policy interpretation used for proposal
// eligibility, routing, approval, and start-time freshness checks.
type admissionEvaluator struct {
	now    func() time.Time
	policy *domain.AdmissionPolicy
}

type admissionInput struct {
	outcome       domain.Outcome
	contract      domain.ContractRevision
	plan          *domain.PlanRevision
	workspaceKind domain.WorkspaceKind
	leaseSubject  string
	document      *domain.OutcomeDocumentContext
	snapshots     map[domain.WorkUnitID]ports.RoutingInventorySnapshot
}

func (e admissionEvaluator) evaluateContract(projectID domain.ProjectID, snapshot ports.RoutingInventorySnapshot) domain.AdmissionVerdict {
	verdict := domain.AdmissionVerdict{ID: "adm-" + uuid.NewString(), OutcomeID: "intake-" + domain.OutcomeID(projectID), EvaluatedAt: e.now().UTC(), PolicyVersion: admissionPolicyVersion, Status: domain.AdmissionRejected, Reasons: []domain.AdmissionReasonCode{domain.AdmissionProviderUnavailable}}
	for _, candidate := range snapshot.Candidates {
		if candidate.Readiness == domain.CapabilitySupported && (candidate.WorkerEligible || candidate.CoordinatorEligible) {
			verdict.Reasons = []domain.AdmissionReasonCode{domain.AdmissionPlanRevisionMissing}
			break
		}
	}
	return verdict
}

func (e admissionEvaluator) evaluate(in admissionInput) domain.AdmissionVerdict {
	verdict := domain.AdmissionVerdict{ID: "adm-" + uuid.NewString(), OutcomeID: in.outcome.ID, EvaluatedAt: e.now().UTC(), PolicyVersion: admissionPolicyVersion}
	reject := func(code domain.AdmissionReasonCode) domain.AdmissionVerdict {
		verdict.Status = domain.AdmissionRejected
		verdict.WorkUnits = nil
		verdict.Reasons = []domain.AdmissionReasonCode{code}
		return verdict
	}
	if in.contract.Number < 1 || in.contract.Number != in.outcome.CurrentRevisionNumber {
		return reject(domain.AdmissionContractRevisionMissing)
	}
	verdict.ContractRevisionNumber = &in.contract.Number
	if in.plan == nil {
		return reject(domain.AdmissionPlanRevisionMissing)
	}
	plan := in.plan
	verdict.PlanRevisionID = &plan.ID
	if !plan.BindsCurrentContract(in.outcome.CurrentRevisionNumber) {
		return reject(domain.AdmissionRevisionSuperseded)
	}
	if err := plan.ValidateForApproval(in.contract); err != nil {
		return reject(domain.AdmissionIntentPermissionConflict)
	}
	kind := in.workspaceKind
	if !kind.Valid() {
		kind = domain.WorkspaceGitWorktree
	}
	verdict.Status = domain.AdmissionAdmitted
	for _, unit := range plan.WorkUnits {
		snapshot, ok := in.snapshots[unit.ID]
		if !ok || strings.TrimSpace(snapshot.SnapshotID) == "" || strings.TrimSpace(snapshot.GenerationID) == "" {
			return reject(domain.AdmissionProviderUnavailable)
		}
		frozen := mustBinding(unit)
		preference := &domain.RoutingPreference{Provider: string(frozen.Provider)}
		if frozen.ModelSelection == domain.ExecutionBindingModelExplicit {
			preference.ModelSelection = domain.ExecutionPreferenceModelExplicit
			preference.Model = frozen.Model
		} else {
			preference.ModelSelection = domain.ExecutionPreferenceModelProviderDefault
		}
		decision := domain.RouteExecution(domain.RoutingRequirements{Role: domain.RoutingRoleWorker, HardCapabilities: append([]string(nil), unit.RequiredCapabilities...), Preference: preference}, snapshot.Candidates, snapshot.SnapshotID)
		binding, ok := decision.RecommendedBinding()
		if !ok || binding != frozen {
			return reject(domain.AdmissionCapabilityMissing)
		}
		policy, err := domain.BuildAttemptExecutionPolicy(in.outcome.ID, *plan, unit, plan.RunBriefCoreDigest)
		if err != nil {
			return reject(domain.AdmissionIntentPermissionConflict)
		}
		if e.policy == nil || e.policy.Validate() != nil {
			return reject(domain.AdmissionTimeBudgetMissing)
		}
		if unit.ExecutionBudget.WallTimeLimit <= 0 {
			return reject(domain.AdmissionTimeBudgetMissing)
		}
		if unit.ExecutionBudget.RetryLimit < 0 {
			return reject(domain.AdmissionRetryBudgetMissing)
		}
		if unit.ExecutionBudget.TokenAccounting == domain.TokenAccountingEnforced && unit.ExecutionBudget.TokenLimit <= 0 {
			return reject(domain.AdmissionTokenBudgetMissing)
		}
		if unit.ExecutionBudget.Validate() != nil {
			return reject(domain.AdmissionTimeBudgetMissing)
		}
		if unit.ExecutionBudget.PolicyID != e.policy.ID || unit.ExecutionBudget.PolicyVersion != e.policy.Version || unit.ExecutionBudget.PolicyDigest != e.policy.Digest {
			return reject(domain.AdmissionBudgetExceedsPolicy)
		}
		if unit.ExecutionBudget.WallTimeLimit > e.policy.MaxWallTime || unit.ExecutionBudget.RetryLimit > e.policy.MaxRetries || (unit.ExecutionBudget.TokenAccounting == domain.TokenAccountingEnforced && (e.policy.MaxTokens <= 0 || unit.ExecutionBudget.TokenLimit > e.policy.MaxTokens)) {
			return reject(domain.AdmissionBudgetExceedsPolicy)
		}
		retry := unit.ExecutionBudget.RetryLimit
		receipt := domain.ReadinessReceipt{Producer: "routing_inventory", Version: domain.RoutingPolicyVersion, ReceiptID: snapshot.SnapshotID, Digest: snapshot.SnapshotID}
		routingReceipt := domain.RoutingAdmissionReceipt{GenerationID: snapshot.GenerationID, SnapshotID: snapshot.SnapshotID, Preference: *preference, Candidates: append([]domain.RoutingCandidate(nil), snapshot.Candidates...)}
		routingReceipt.Digest, err = routingReceipt.ComputedDigest()
		if err != nil {
			return reject(domain.AdmissionIntentPermissionConflict)
		}
		spec := domain.ApprovedExecutableSpec{CompilerPolicyVersion: admissionCompilerPolicyVersion, OutcomeID: in.outcome.ID, ContractRevisionNumber: in.contract.Number, PlanRevisionID: plan.ID, WorkUnitID: unit.ID, RunBriefCoreDigest: plan.RunBriefCoreDigest, Binding: binding, NativeMappingVersion: admissionNativeMappingVersion, RequiredCapabilities: append([]string(nil), policy.RequiredCapabilities...), Grants: append([]domain.CapabilityGrant(nil), policy.Grants...), ApprovedChecks: append([]domain.ApprovedCheck(nil), policy.ApprovedChecks...), Workspace: domain.WorkspaceRequirements{Kind: kind, LeaseSubject: in.leaseSubject}, Budget: domain.AdmissionBudget{PolicyVersion: unit.ExecutionBudget.PolicyVersion, AccountingVersion: string(unit.ExecutionBudget.TokenAccounting), WallTimeLimit: unit.ExecutionBudget.WallTimeLimit, TokenLimit: unit.ExecutionBudget.TokenLimit, TokenAccountingSupported: unit.ExecutionBudget.TokenAccounting == domain.TokenAccountingEnforced, RetryLimit: &retry, RetryLineageScope: domain.AdmissionRetryLineageWorkUnit}, AdmissionReceipts: []domain.ReadinessReceipt{receipt}, RoutingReceipt: routingReceipt, DocumentContextID: func() domain.DocumentContextID {
			if in.document != nil {
				return in.document.ID
			}
			return ""
		}(), DocumentContextRevision: func() int64 {
			if in.document != nil {
				return in.document.Revision
			}
			return 0
		}(), DocumentContextDigest: func() string {
			if in.document != nil {
				return in.document.Digest
			}
			return ""
		}()}
		spec.Digest, err = spec.ComputedDigest()
		if err != nil {
			return reject(domain.AdmissionIntentPermissionConflict)
		}
		if err = spec.Validate(); err != nil {
			return reject(domain.AdmissionIntentPermissionConflict)
		}
		verdict.WorkUnits = append(verdict.WorkUnits, domain.WorkUnitAdmissionVerdict{WorkUnitID: unit.ID, Status: domain.AdmissionAdmitted, Executable: &spec})
	}
	sort.Slice(verdict.WorkUnits, func(i, j int) bool { return verdict.WorkUnits[i].WorkUnitID < verdict.WorkUnits[j].WorkUnitID })
	return verdict
}

func admissionWorkspaceKind(kind domain.ProjectKind) (domain.WorkspaceKind, error) {
	switch kind.WithDefault() {
	case domain.ProjectKindSingleRepo, domain.ProjectKindWorkspace:
		return domain.WorkspaceGitWorktree, nil
	case domain.ProjectKindScratch:
		return domain.WorkspaceStagedFolder, nil
	default:
		return "", fmt.Errorf("unsupported project kind %q", kind)
	}
}

func mustBinding(unit domain.WorkUnit) domain.ExecutionBinding {
	b, _ := unit.ExecutionBindingForNewWork()
	return b
}

func (s *Service) evaluatePlanAdmission(ctx context.Context, outcome domain.Outcome, contract domain.ContractRevision, plan domain.PlanRevision, projectID domain.ProjectID) (domain.AdmissionVerdict, error) {
	if s.routing == nil {
		return domain.AdmissionVerdict{}, fmt.Errorf("admission routing inventory is unavailable")
	}
	_, project, err := s.projectForOutcome(ctx, outcome.ID)
	if err != nil {
		return domain.AdmissionVerdict{}, err
	}
	if len(plan.WorkUnits) == 0 {
		return admissionEvaluator{now: s.clock, policy: s.AdmissionPolicy}.evaluate(admissionInput{outcome: outcome, contract: contract, plan: &plan}), nil
	}
	snapshots := make(map[domain.WorkUnitID]ports.RoutingInventorySnapshot, len(plan.WorkUnits))
	byPreference := make(map[string][]domain.WorkUnitID)
	preferences := make(map[string]*domain.RoutingPreference)
	for _, unit := range plan.WorkUnits {
		binding, bindErr := unit.ExecutionBindingForNewWork()
		if bindErr != nil {
			return domain.AdmissionVerdict{}, bindErr
		}
		preference := &domain.RoutingPreference{Provider: string(binding.Provider), ModelSelection: domain.ExecutionPreferenceModelProviderDefault}
		if binding.ModelSelection == domain.ExecutionBindingModelExplicit {
			preference.ModelSelection = domain.ExecutionPreferenceModelExplicit
			preference.Model = binding.Model
		}
		key := preference.Provider + "\x00" + string(preference.ModelSelection) + "\x00" + preference.Model
		preferences[key] = preference
		byPreference[key] = append(byPreference[key], unit.ID)
	}
	keys := make([]string, 0, len(preferences))
	for key := range preferences {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	coherentGeneration := ""
	for _, key := range keys {
		snapshot, snapshotErr := s.routing.RoutingSnapshot(ctx, projectID, preferences[key])
		if snapshotErr != nil {
			return domain.AdmissionVerdict{}, snapshotErr
		}
		generation := snapshot.GenerationID
		if coherentGeneration == "" {
			coherentGeneration = generation
		} else if generation != coherentGeneration {
			v := (admissionEvaluator{now: s.clock, policy: s.AdmissionPolicy}).evaluate(admissionInput{outcome: outcome, contract: contract, plan: &plan})
			v.Status = domain.AdmissionRejected
			v.WorkUnits = nil
			v.Reasons = []domain.AdmissionReasonCode{domain.AdmissionCapabilitySnapshotChanged}
			return v, nil
		}
		for _, unitID := range byPreference[key] {
			snapshots[unitID] = snapshot
		}
	}
	kind, err := admissionWorkspaceKind(project.Kind)
	if err != nil {
		return domain.AdmissionVerdict{}, err
	}
	var document *domain.OutcomeDocumentContext
	if s.documents != nil {
		if current, found, err := s.documents.CurrentDocumentContext(ctx, outcome.ID); err != nil {
			return domain.AdmissionVerdict{}, err
		} else if found {
			kind = domain.WorkspaceStagedFolder
			document = &current
		}
	}
	return admissionEvaluator{now: s.clock, policy: s.AdmissionPolicy}.evaluate(admissionInput{outcome: outcome, contract: contract, plan: &plan, workspaceKind: kind, leaseSubject: domain.FenceSubjectForProject(projectID), snapshots: snapshots, document: document}), nil
}

func (e admissionEvaluator) revalidate(approved domain.ApprovedExecutableSpec, current domain.AdmissionVerdict) domain.AdmissionVerdict {
	if current.Status != domain.AdmissionAdmitted {
		return current
	}
	currentSpec, ok := admittedSpec(current, approved.WorkUnitID)
	if ok && currentSpec.Digest == approved.Digest {
		return current
	}
	current.Status = domain.AdmissionStale
	current.Reasons = []domain.AdmissionReasonCode{domain.AdmissionVerdictStale}
	current.WorkUnits = nil
	return current
}

func admittedSpec(verdict domain.AdmissionVerdict, unitID domain.WorkUnitID) (domain.ApprovedExecutableSpec, bool) {
	for _, u := range verdict.WorkUnits {
		if u.WorkUnitID == unitID && u.Executable != nil {
			return *u.Executable, true
		}
	}
	return domain.ApprovedExecutableSpec{}, false
}

// EvaluateAdmissionStage is the shared staged boundary used before reasoning,
// while routing a proposal, at approval, and at Attempt start.
func (s *Service) EvaluateAdmissionStage(ctx context.Context, in ports.AdmissionStageInput) (ports.AdmissionStageResult, error) {
	if in.Stage == ports.AdmissionStageContract {
		if s.routing == nil {
			return ports.AdmissionStageResult{}, fmt.Errorf("admission routing inventory is unavailable")
		}
		snapshot, err := s.routing.RoutingSnapshot(ctx, in.ProjectID, nil)
		if err != nil {
			return ports.AdmissionStageResult{}, err
		}
		v := (admissionEvaluator{now: s.clock, policy: s.AdmissionPolicy}).evaluateContract(in.ProjectID, snapshot)
		return ports.AdmissionStageResult{Eligible: v.Reasons[0] != domain.AdmissionProviderUnavailable, Verdict: v, Snapshot: &snapshot}, nil
	}
	if in.Outcome == nil || in.Contract == nil || in.Plan == nil {
		return ports.AdmissionStageResult{}, fmt.Errorf("%s stage requires outcome, contract and plan", in.Stage)
	}
	var v domain.AdmissionVerdict
	var err error
	if in.Stage == ports.AdmissionStageProposal && in.RoutingSnapshot != nil {
		snapshots := make(map[domain.WorkUnitID]ports.RoutingInventorySnapshot, len(in.Plan.WorkUnits))
		for _, unit := range in.Plan.WorkUnits {
			snapshots[unit.ID] = *in.RoutingSnapshot
		}
		_, project, projectErr := s.projectForOutcome(ctx, in.Outcome.ID)
		if projectErr != nil {
			return ports.AdmissionStageResult{}, projectErr
		}
		kind, kindErr := admissionWorkspaceKind(project.Kind)
		if kindErr != nil {
			return ports.AdmissionStageResult{}, kindErr
		}
		var document *domain.OutcomeDocumentContext
		if s.documents != nil {
			if current, found, readErr := s.documents.CurrentDocumentContext(ctx, in.Outcome.ID); readErr != nil {
				return ports.AdmissionStageResult{}, readErr
			} else if found {
				kind = domain.WorkspaceStagedFolder
				document = &current
			}
		}
		v = (admissionEvaluator{now: s.clock, policy: s.AdmissionPolicy}).evaluate(admissionInput{outcome: *in.Outcome, contract: *in.Contract, plan: in.Plan, workspaceKind: kind, leaseSubject: domain.FenceSubjectForProject(in.ProjectID), snapshots: snapshots, document: document})
	} else {
		v, err = s.evaluatePlanAdmission(ctx, *in.Outcome, *in.Contract, *in.Plan, in.ProjectID)
	}
	if err != nil {
		return ports.AdmissionStageResult{}, err
	}
	return ports.AdmissionStageResult{Eligible: v.Status == domain.AdmissionAdmitted, Verdict: v}, nil
}
