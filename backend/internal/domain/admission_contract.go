package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

type AdmissionStatus string

const (
	AdmissionAdmitted AdmissionStatus = "admitted"
	AdmissionRejected AdmissionStatus = "rejected"
	AdmissionStale    AdmissionStatus = "stale"
)

func (s AdmissionStatus) Valid() bool {
	return s == AdmissionAdmitted || s == AdmissionRejected || s == AdmissionStale
}

type AdmissionReasonCode string

const (
	AdmissionContractRevisionMissing         AdmissionReasonCode = "contract_revision_missing"
	AdmissionPlanRevisionMissing             AdmissionReasonCode = "plan_revision_missing"
	AdmissionWorkUnitMissing                 AdmissionReasonCode = "workunit_missing"
	AdmissionVerdictStale                    AdmissionReasonCode = "verdict_stale"
	AdmissionRevisionSuperseded              AdmissionReasonCode = "revision_superseded"
	AdmissionProviderUnverified              AdmissionReasonCode = "provider_unverified"
	AdmissionProviderUnavailable             AdmissionReasonCode = "provider_unavailable"
	AdmissionProviderProfileMissing          AdmissionReasonCode = "provider_profile_missing"
	AdmissionCapabilityMissing               AdmissionReasonCode = "capability_missing"
	AdmissionSandboxUnrepresentable          AdmissionReasonCode = "sandbox_unrepresentable"
	AdmissionPlatformUnsupported             AdmissionReasonCode = "platform_unsupported"
	AdmissionIntentPermissionConflict        AdmissionReasonCode = "intent_permission_conflict"
	AdmissionOutputPermissionConflict        AdmissionReasonCode = "output_permission_conflict"
	AdmissionCheckPermissionConflict         AdmissionReasonCode = "check_permission_conflict"
	AdmissionCheckUncompilable               AdmissionReasonCode = "check_uncompilable"
	AdmissionDependencyMissing               AdmissionReasonCode = "dependency_missing"
	AdmissionDependencyCycle                 AdmissionReasonCode = "dependency_cycle"
	AdmissionWorkspaceRequirementUnsupported AdmissionReasonCode = "workspace_requirement_unsupported"
	AdmissionAuthorityExceeded               AdmissionReasonCode = "authority_exceeded"
	AdmissionExternalEffectUnapproved        AdmissionReasonCode = "external_effect_unapproved"
	AdmissionTimeBudgetMissing               AdmissionReasonCode = "time_budget_missing"
	AdmissionTokenBudgetMissing              AdmissionReasonCode = "token_budget_missing"
	AdmissionRetryBudgetMissing              AdmissionReasonCode = "retry_budget_missing"
	AdmissionBudgetExceedsPolicy             AdmissionReasonCode = "budget_exceeds_policy"
	AdmissionBindingChanged                  AdmissionReasonCode = "binding_changed"
	AdmissionCapabilitySnapshotChanged       AdmissionReasonCode = "capability_snapshot_changed"
	AdmissionWorkspaceUnavailable            AdmissionReasonCode = "workspace_unavailable"
	AdmissionFenceConflict                   AdmissionReasonCode = "fence_conflict"
)

var allAdmissionReasonCodes = []AdmissionReasonCode{AdmissionContractRevisionMissing, AdmissionPlanRevisionMissing, AdmissionWorkUnitMissing, AdmissionVerdictStale, AdmissionRevisionSuperseded, AdmissionProviderUnverified, AdmissionProviderUnavailable, AdmissionProviderProfileMissing, AdmissionCapabilityMissing, AdmissionSandboxUnrepresentable, AdmissionPlatformUnsupported, AdmissionIntentPermissionConflict, AdmissionOutputPermissionConflict, AdmissionCheckPermissionConflict, AdmissionCheckUncompilable, AdmissionDependencyMissing, AdmissionDependencyCycle, AdmissionWorkspaceRequirementUnsupported, AdmissionAuthorityExceeded, AdmissionExternalEffectUnapproved, AdmissionTimeBudgetMissing, AdmissionTokenBudgetMissing, AdmissionRetryBudgetMissing, AdmissionBudgetExceedsPolicy, AdmissionBindingChanged, AdmissionCapabilitySnapshotChanged, AdmissionWorkspaceUnavailable, AdmissionFenceConflict}

func (c AdmissionReasonCode) Valid() bool {
	for _, v := range allAdmissionReasonCodes {
		if c == v {
			return true
		}
	}
	return false
}
func SortedAdmissionReasonCodes() []AdmissionReasonCode {
	o := append([]AdmissionReasonCode(nil), allAdmissionReasonCodes...)
	sort.Slice(o, func(i, j int) bool { return o[i] < o[j] })
	return o
}

type AdmissionOwnerAction string

const (
	AdmissionActionReviseContract      AdmissionOwnerAction = "revise_contract"
	AdmissionActionRevisePlan          AdmissionOwnerAction = "revise_plan"
	AdmissionActionChooseHarness       AdmissionOwnerAction = "choose_harness"
	AdmissionActionAuthenticateHarness AdmissionOwnerAction = "authenticate_harness"
	AdmissionActionFreeWorkspace       AdmissionOwnerAction = "free_workspace"
	AdmissionActionReevaluate          AdmissionOwnerAction = "reevaluate"
)

func (a AdmissionOwnerAction) Valid() bool {
	switch a {
	case AdmissionActionReviseContract, AdmissionActionRevisePlan, AdmissionActionChooseHarness, AdmissionActionAuthenticateHarness, AdmissionActionFreeWorkspace, AdmissionActionReevaluate:
		return true
	}
	return false
}

type AdmissionBudget struct {
	PolicyVersion            string        `json:"policyVersion"`
	AccountingVersion        string        `json:"accountingVersion"`
	WallTimeLimit            time.Duration `json:"wallTimeLimit"`
	TokenLimit               int64         `json:"tokenLimit"`
	TokenAccountingSupported bool          `json:"tokenAccountingSupported"`
	RetryLimit               *int          `json:"retryLimit"`
	RetryLineageScope        string        `json:"retryLineageScope"`
}

const AdmissionRetryLineageWorkUnit = "workunit_attempt_successors"

func (b AdmissionBudget) Validate() error {
	if strings.TrimSpace(b.PolicyVersion) == "" || strings.TrimSpace(b.AccountingVersion) == "" {
		return fmt.Errorf("budget versions are required")
	}
	if b.WallTimeLimit <= 0 {
		return fmt.Errorf("wall time limit is required")
	}
	if b.TokenLimit < 0 {
		return fmt.Errorf("token limit cannot be negative")
	}
	if b.TokenAccountingSupported != (b.TokenLimit > 0) {
		return fmt.Errorf("token accounting support and limit disagree")
	}
	if b.RetryLimit == nil || *b.RetryLimit < 0 {
		return fmt.Errorf("non-negative retry limit is required")
	}
	if b.RetryLineageScope != AdmissionRetryLineageWorkUnit {
		return fmt.Errorf("unsupported retry lineage scope %q", b.RetryLineageScope)
	}
	return nil
}

type RoutingAdmissionReceipt struct {
	GenerationID string             `json:"generationId"`
	SnapshotID   string             `json:"snapshotId"`
	Preference   RoutingPreference  `json:"preference"`
	Candidates   []RoutingCandidate `json:"candidates"`
	Digest       string             `json:"digest"`
}

func (r RoutingAdmissionReceipt) payload() RoutingAdmissionReceipt {
	r.Digest = ""
	r.Candidates = append([]RoutingCandidate(nil), r.Candidates...)
	sort.Slice(r.Candidates, func(i, j int) bool { return r.Candidates[i].ID < r.Candidates[j].ID })
	return r
}
func (r RoutingAdmissionReceipt) ComputedDigest() (string, error) {
	raw, err := json.Marshal(r.payload())
	if err != nil {
		return "", err
	}
	return string(DigestSHA256(raw)), nil
}
func (r RoutingAdmissionReceipt) Validate() error {
	if strings.TrimSpace(r.GenerationID) == "" || strings.TrimSpace(r.SnapshotID) == "" {
		return fmt.Errorf("routing receipt identity incomplete")
	}
	want, err := r.ComputedDigest()
	if err != nil {
		return err
	}
	if want != r.Digest {
		return fmt.Errorf("routing receipt digest mismatch")
	}
	return nil
}

type ReadinessReceipt struct {
	Producer  string `json:"producer"`
	Version   string `json:"version"`
	ReceiptID string `json:"receiptId"`
	Digest    string `json:"digest"`
}

func (r ReadinessReceipt) Validate() error {
	if strings.TrimSpace(r.Producer) == "" || strings.TrimSpace(r.Version) == "" || strings.TrimSpace(r.ReceiptID) == "" || strings.TrimSpace(r.Digest) == "" {
		return fmt.Errorf("readiness receipt identity is incomplete")
	}
	return nil
}

// WorkspaceRequirements freezes approval-time workspace semantics without
// reserving or naming a concrete runtime workspace.
type WorkspaceRequirements struct {
	Kind         WorkspaceKind `json:"kind"`
	LeaseSubject string        `json:"leaseSubject"`
}

func (w WorkspaceRequirements) Validate() error {
	if !w.Kind.Valid() || strings.TrimSpace(w.LeaseSubject) == "" {
		return fmt.Errorf("workspace requirements are incomplete")
	}
	return nil
}

// ApprovedExecutableSpec is immutable approval authority. It deliberately has
// no Attempt, fence, session, concrete root, or runtime-input identity.
type ApprovedExecutableSpec struct {
	CompilerPolicyVersion   string                  `json:"compilerPolicyVersion"`
	OutcomeID               OutcomeID               `json:"outcomeId"`
	ContractRevisionNumber  int64                   `json:"contractRevisionNumber"`
	PlanRevisionID          PlanRevisionID          `json:"planRevisionId"`
	WorkUnitID              WorkUnitID              `json:"workUnitId"`
	RunBriefCoreDigest      string                  `json:"runBriefCoreDigest"`
	Binding                 ExecutionBinding        `json:"binding"`
	NativeMappingVersion    string                  `json:"nativeMappingVersion"`
	RequiredCapabilities    []string                `json:"requiredCapabilities"`
	Grants                  []CapabilityGrant       `json:"grants"`
	ApprovedChecks          []ApprovedCheck         `json:"approvedChecks,omitempty"`
	Workspace               WorkspaceRequirements   `json:"workspace"`
	Budget                  AdmissionBudget         `json:"budget"`
	AdmissionReceipts       []ReadinessReceipt      `json:"admissionReceipts"`
	RoutingReceipt          RoutingAdmissionReceipt `json:"routingReceipt"`
	DocumentContextID       DocumentContextID       `json:"documentContextId,omitempty"`
	DocumentContextRevision int64                   `json:"documentContextRevision,omitempty"`
	DocumentContextDigest   string                  `json:"documentContextDigest,omitempty"`
	Digest                  string                  `json:"digest"`
}

func (s ApprovedExecutableSpec) payload() ApprovedExecutableSpec { s.Digest = ""; return s }
func (s ApprovedExecutableSpec) ComputedDigest() (string, error) {
	raw, err := json.Marshal(s.payload())
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func (s ApprovedExecutableSpec) Validate() error {
	if strings.TrimSpace(s.CompilerPolicyVersion) == "" || strings.TrimSpace(s.NativeMappingVersion) == "" || s.OutcomeID.IsZero() || s.ContractRevisionNumber < 1 || s.PlanRevisionID.IsZero() || s.WorkUnitID.IsZero() || strings.TrimSpace(s.RunBriefCoreDigest) == "" {
		return fmt.Errorf("approved executable spec identity is incomplete")
	}
	if err := s.Binding.ValidateForNewWork(); err != nil {
		return err
	}
	policy := AttemptExecutionPolicy{OutcomeID: s.OutcomeID, PlanRevisionID: s.PlanRevisionID, WorkUnitID: s.WorkUnitID, ContractRevisionNumber: s.ContractRevisionNumber, RunBriefCoreDigest: s.RunBriefCoreDigest, RequiredCapabilities: s.RequiredCapabilities, Grants: s.Grants, ApprovedChecks: s.ApprovedChecks}
	if err := policy.Validate(); err != nil {
		return fmt.Errorf("approved policy: %w", err)
	}
	if err := s.Workspace.Validate(); err != nil {
		return err
	}
	if (s.DocumentContextID.IsZero()) != (s.DocumentContextRevision == 0 || strings.TrimSpace(s.DocumentContextDigest) == "") {
		return fmt.Errorf("document context identity is incomplete")
	}
	if !s.DocumentContextID.IsZero() && (s.DocumentContextRevision < 1 || strings.TrimSpace(s.DocumentContextDigest) == "") {
		return fmt.Errorf("document context identity is incomplete")
	}
	if err := s.Budget.Validate(); err != nil {
		return err
	}
	if err := s.RoutingReceipt.Validate(); err != nil {
		return err
	}
	if len(s.AdmissionReceipts) == 0 {
		return fmt.Errorf("admission receipts are required")
	}
	if err := validateReceipts(s.AdmissionReceipts); err != nil {
		return err
	}
	want, err := s.ComputedDigest()
	if err != nil {
		return err
	}
	if s.Digest != want {
		return fmt.Errorf("approved executable spec digest mismatch")
	}
	return nil
}

type LaunchFacts struct {
	AttemptID               AttemptID          `json:"attemptId"`
	FenceID                 string             `json:"fenceId"`
	SessionID               string             `json:"sessionId"`
	CanonicalWorkspaceRoot  string             `json:"canonicalWorkspaceRoot"`
	SpecDigest              string             `json:"specDigest"`
	ReadinessReceipts       []ReadinessReceipt `json:"readinessReceipts"`
	DocumentContextID       DocumentContextID  `json:"documentContextId,omitempty"`
	DocumentContextRevision int64              `json:"documentContextRevision,omitempty"`
	DocumentContextDigest   string             `json:"documentContextDigest,omitempty"`
	InputArtifactVersions   []string           `json:"inputArtifactVersions"`
	PolicyDigest            string             `json:"policyDigest"`
}

func (f LaunchFacts) Digest() (string, error) {
	raw, err := json.Marshal(f)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// WorkspaceBoundLaunchPacket binds one approved spec to exact runtime facts.
// The spec remains approval authority; this packet can only bind or narrow it.
type WorkspaceBoundLaunchPacket struct {
	Spec                     ApprovedExecutableSpec `json:"spec"`
	SpecDigest               string                 `json:"specDigest"`
	AttemptID                AttemptID              `json:"attemptId"`
	FenceID                  string                 `json:"fenceId"`
	SessionID                string                 `json:"sessionId"`
	CanonicalWorkspaceRoot   string                 `json:"canonicalWorkspaceRoot"`
	InputArtifactVersions    []string               `json:"inputArtifactVersions"`
	CurrentReadinessReceipts []ReadinessReceipt     `json:"currentReadinessReceipts"`
	LaunchFacts              LaunchFacts            `json:"launchFacts"`
	LaunchFactsDigest        string                 `json:"launchFactsDigest"`
	Policy                   AttemptExecutionPolicy `json:"policy"`
	Digest                   string                 `json:"digest"`
}

func (p WorkspaceBoundLaunchPacket) payload() WorkspaceBoundLaunchPacket { p.Digest = ""; return p }
func (p WorkspaceBoundLaunchPacket) ComputedDigest() (string, error) {
	raw, err := json.Marshal(p.payload())
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func (p WorkspaceBoundLaunchPacket) Validate() error {
	if err := p.Spec.Validate(); err != nil {
		return err
	}
	if p.SpecDigest != p.Spec.Digest {
		return fmt.Errorf("launch spec digest mismatch")
	}
	if p.AttemptID.IsZero() || strings.TrimSpace(p.FenceID) == "" || strings.TrimSpace(p.SessionID) == "" || strings.TrimSpace(p.CanonicalWorkspaceRoot) == "" || strings.TrimSpace(p.LaunchFactsDigest) == "" {
		return fmt.Errorf("runtime binding is incomplete")
	}
	wantFacts := LaunchFacts{AttemptID: p.AttemptID, FenceID: p.FenceID, SessionID: p.SessionID, CanonicalWorkspaceRoot: p.CanonicalWorkspaceRoot, SpecDigest: p.SpecDigest, ReadinessReceipts: p.CurrentReadinessReceipts, DocumentContextID: p.Spec.DocumentContextID, DocumentContextRevision: p.Spec.DocumentContextRevision, DocumentContextDigest: p.Spec.DocumentContextDigest, InputArtifactVersions: p.InputArtifactVersions}
	policyDigest, err := p.Policy.Digest()
	if err != nil {
		return err
	}
	wantFacts.PolicyDigest = policyDigest
	factsDigest, err := p.LaunchFacts.Digest()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(p.LaunchFacts, wantFacts) || p.LaunchFactsDigest != factsDigest {
		return fmt.Errorf("launch facts mismatch")
	}
	if len(p.CurrentReadinessReceipts) == 0 {
		return fmt.Errorf("current readiness receipts are required")
	}
	if err := validateReceipts(p.CurrentReadinessReceipts); err != nil {
		return err
	}
	if err := p.Policy.Validate(); err != nil {
		return err
	}
	if err := p.Policy.ValidateWorkspaceRoot(p.CanonicalWorkspaceRoot); err != nil {
		return err
	}
	if p.Policy.OutcomeID != p.Spec.OutcomeID || p.Policy.ContractRevisionNumber != p.Spec.ContractRevisionNumber || p.Policy.PlanRevisionID != p.Spec.PlanRevisionID || p.Policy.WorkUnitID != p.Spec.WorkUnitID || p.Policy.RunBriefCoreDigest != p.Spec.RunBriefCoreDigest {
		return fmt.Errorf("launch attribution does not match approved spec")
	}
	if !equalStrings(p.Policy.RequiredCapabilities, p.Spec.RequiredCapabilities) || !equalGrants(p.Policy.Grants, p.Spec.Grants) || !equalChecks(p.Policy.ApprovedChecks, p.Spec.ApprovedChecks) {
		return fmt.Errorf("launch policy widens or changes approved authority")
	}
	want, err := p.ComputedDigest()
	if err != nil {
		return err
	}
	if p.Digest != want {
		return fmt.Errorf("workspace-bound launch packet digest mismatch")
	}
	return nil
}
func validateReceipts(rs []ReadinessReceipt) error {
	seen := map[string]struct{}{}
	for _, r := range rs {
		if err := r.Validate(); err != nil {
			return err
		}
		k := r.Producer + "\x00" + r.Version + "\x00" + r.ReceiptID
		if _, ok := seen[k]; ok {
			return fmt.Errorf("duplicate readiness receipt")
		}
		seen[k] = struct{}{}
	}
	return nil
}
func equalGrants(a, b []CapabilityGrant) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func equalChecks(a, b []ApprovedCheck) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].CriterionID != b[i].CriterionID || a[i].TimeoutSeconds != b[i].TimeoutSeconds || !equalStrings(a[i].Argv, b[i].Argv) {
			return false
		}
	}
	return true
}

type WorkUnitAdmissionVerdict struct {
	WorkUnitID   WorkUnitID              `json:"workUnitId"`
	Status       AdmissionStatus         `json:"status"`
	Reasons      []AdmissionReasonCode   `json:"reasons,omitempty"`
	OwnerActions []AdmissionOwnerAction  `json:"ownerActions,omitempty"`
	Executable   *ApprovedExecutableSpec `json:"executable,omitempty"`
}

func (v WorkUnitAdmissionVerdict) Validate() error {
	if v.WorkUnitID.IsZero() {
		return fmt.Errorf("work unit id is required")
	}
	if v.Status != AdmissionAdmitted && v.Status != AdmissionRejected {
		return fmt.Errorf("work unit status must be admitted or rejected")
	}
	if err := validateAdmissionMeta(v.Status, v.Reasons, v.OwnerActions); err != nil {
		return err
	}
	if v.Status == AdmissionRejected {
		if v.Executable != nil {
			return fmt.Errorf("rejected work unit cannot carry executable packet")
		}
		return nil
	}
	if v.Executable == nil {
		return fmt.Errorf("admitted work unit requires executable packet")
	}
	if v.Executable.WorkUnitID != v.WorkUnitID {
		return fmt.Errorf("work unit attribution mismatch")
	}
	return v.Executable.Validate()
}

type AdmissionVerdict struct {
	ID                     string                     `json:"id"`
	OutcomeID              OutcomeID                  `json:"outcomeId"`
	ContractRevisionNumber *int64                     `json:"contractRevisionNumber,omitempty"`
	PlanRevisionID         *PlanRevisionID            `json:"planRevisionId,omitempty"`
	EvaluatedAt            time.Time                  `json:"evaluatedAt"`
	Status                 AdmissionStatus            `json:"status"`
	PolicyVersion          string                     `json:"policyVersion"`
	WorkUnits              []WorkUnitAdmissionVerdict `json:"workUnits,omitempty"`
	Reasons                []AdmissionReasonCode      `json:"reasons,omitempty"`
	OwnerActions           []AdmissionOwnerAction     `json:"ownerActions,omitempty"`
}

func (v AdmissionVerdict) Validate() error {
	if strings.TrimSpace(v.ID) == "" || v.OutcomeID.IsZero() || v.EvaluatedAt.IsZero() || strings.TrimSpace(v.PolicyVersion) == "" {
		return fmt.Errorf("verdict identity is incomplete")
	}
	if !v.Status.Valid() {
		return fmt.Errorf("invalid status")
	}
	if err := validateAdmissionMeta(v.Status, v.Reasons, v.OwnerActions); err != nil {
		return err
	}
	if v.Status == AdmissionAdmitted && len(v.OwnerActions) > 0 {
		return fmt.Errorf("admitted verdict cannot require owner actions")
	}
	seenWorkUnits := map[WorkUnitID]struct{}{}
	for _, unit := range v.WorkUnits {
		if unit.WorkUnitID.IsZero() {
			continue
		}
		if _, duplicate := seenWorkUnits[unit.WorkUnitID]; duplicate {
			return fmt.Errorf("duplicate work unit %s", unit.WorkUnitID)
		}
		seenWorkUnits[unit.WorkUnitID] = struct{}{}
	}
	if v.Status != AdmissionAdmitted {
		if len(v.WorkUnits) > 0 {
			for _, u := range v.WorkUnits {
				if u.Status == AdmissionAdmitted || u.Executable != nil {
					return fmt.Errorf("non-admitted verdict cannot carry admitted executable work")
				}
			}
		}
		return nil
	}
	if v.ContractRevisionNumber == nil || *v.ContractRevisionNumber < 1 || v.PlanRevisionID == nil || v.PlanRevisionID.IsZero() || len(v.WorkUnits) == 0 {
		return fmt.Errorf("admitted verdict requires contract, plan, and work units")
	}
	for _, u := range v.WorkUnits {
		if err := u.Validate(); err != nil {
			return err
		}
		if u.Status != AdmissionAdmitted {
			return fmt.Errorf("admitted verdict requires admitted work units")
		}
		p := u.Executable
		if p.OutcomeID != v.OutcomeID || p.ContractRevisionNumber != *v.ContractRevisionNumber || p.PlanRevisionID != *v.PlanRevisionID {
			return fmt.Errorf("packet attribution does not match verdict")
		}
	}
	return nil
}
func validateAdmissionMeta(s AdmissionStatus, reasons []AdmissionReasonCode, actions []AdmissionOwnerAction) error {
	seen := map[AdmissionReasonCode]struct{}{}
	for _, r := range reasons {
		if !r.Valid() {
			return fmt.Errorf("unknown reason")
		}
		if _, ok := seen[r]; ok {
			return fmt.Errorf("duplicate reason")
		}
		seen[r] = struct{}{}
	}
	if s == AdmissionAdmitted && len(reasons) > 0 {
		return fmt.Errorf("admitted cannot carry reasons")
	}
	if s != AdmissionAdmitted && len(reasons) == 0 {
		return fmt.Errorf("non-admitted requires reason")
	}
	sa := map[AdmissionOwnerAction]struct{}{}
	for _, a := range actions {
		if !a.Valid() {
			return fmt.Errorf("unknown owner action")
		}
		if _, ok := sa[a]; ok {
			return fmt.Errorf("duplicate owner action")
		}
		sa[a] = struct{}{}
	}
	return nil
}
