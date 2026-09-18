package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// PlanRevisionID identifies one immutable plan revision of an Outcome.
type PlanRevisionID string

// IsZero reports whether the plan revision ID is unset.
func (id PlanRevisionID) IsZero() bool   { return strings.TrimSpace(string(id)) == "" }
func (id PlanRevisionID) String() string { return string(id) }

// WorkUnitID identifies one unit of planned work inside a PlanRevision.
type WorkUnitID string

// IsZero reports whether the work unit ID is unset.
func (id WorkUnitID) IsZero() bool   { return strings.TrimSpace(string(id)) == "" }
func (id WorkUnitID) String() string { return string(id) }

// CapabilityGrantID identifies one scoped capability grant of a PlanRevision.
type CapabilityGrantID string

// IsZero reports whether the capability grant ID is unset.
func (id CapabilityGrantID) IsZero() bool   { return strings.TrimSpace(string(id)) == "" }
func (id CapabilityGrantID) String() string { return string(id) }

// PlanStatus is the lifecycle state of a plan revision.
type PlanStatus string

const (
	// PlanStatusProposed is a plan awaiting owner approval.
	PlanStatusProposed PlanStatus = "proposed"
	// PlanStatusApproved is a plan authorized for execution.
	PlanStatusApproved PlanStatus = "approved"
)

// Valid reports whether the plan status is supported.
func (s PlanStatus) Valid() bool {
	switch s {
	case PlanStatusProposed, PlanStatusApproved:
		return true
	}
	return false
}

const (
	// CapabilityWorktreeRead permits reading the project worktree.
	CapabilityWorktreeRead = "worktree.read"
	// CapabilityWorktreeWrite permits writing the project worktree.
	CapabilityWorktreeWrite = "worktree.write"
	// CapabilityWorktreeExec permits executing local worktree commands.
	CapabilityWorktreeExec = "worktree.exec"
)

// MaxCanonicalPlanWorkUnits bounds one direct Plan against pathological model
// output. It is operational policy, not a responsibility-model invariant.
const MaxCanonicalPlanWorkUnits = 32

// WorkUnitKind identifies the execution shape of a work unit.
type WorkUnitKind string

// WorkUnitDirect is the v1 direct execution shape.
const WorkUnitDirect WorkUnitKind = "direct"

// Valid reports whether the work unit kind is supported.
func (k WorkUnitKind) Valid() bool { return k == WorkUnitDirect }

// RequiresExclusiveWorktreeAccess reports whether a WorkUnit with these
// required capabilities must hold the project's exclusive worktree fence
// rather than run concurrently with sibling Attempts. Only a WorkUnit whose
// required capabilities are read-only (worktree.read, and nothing else) may
// share worktree access with concurrent siblings; an empty/unspecified
// capability set is treated as exclusive so unclassified WorkUnits keep
// today's serialized guarantee.
func requiresExclusiveWorktreeAccess(capabilities []string) bool {
	if len(capabilities) == 0 {
		return true
	}
	for _, capability := range capabilities {
		if capability != CapabilityWorktreeRead {
			return true
		}
	}
	return false
}

// WorkUnit is immutable unit-level execution authority once its Plan is approved.
// Dependencies and criterion coverage are canonical graph/proof identity; routing
// recommendation is stored separately and must agree with this exact binding.
type WorkUnit struct {
	ID                      WorkUnitID
	Kind                    WorkUnitKind
	Title                   string
	ContractRevisionNumber  int64
	Provider                AgentHarness
	ModelSelection          ExecutionBindingModelSelection
	Model                   string
	OutputSummary           string
	EvidenceChecks          []string
	VerificationRequirement string
	StopConditions          []string
	DependsOn               []WorkUnitID
	CriterionIDs            []CriterionID
	RequiredCapabilities    []string
	// Checks are the deterministic commands the owner authorized for this
	// unit. EvidenceChecks above stay prose for the provider to read; these
	// are what Kennel itself runs and records as independent observation.
	// Empty is valid: not every WorkUnit can be proved by a command.
	Checks []ApprovedCheck
}

// RequiresExclusiveWorktreeAccess reports whether this WorkUnit must hold the
// project's exclusive worktree fence rather than run concurrently with
// sibling Attempts (ADR 0009 §6: read/reason-only work may run concurrently;
// writes, exec, and unclassified WorkUnits remain exclusive).
func (w WorkUnit) RequiresExclusiveWorktreeAccess() bool {
	return requiresExclusiveWorktreeAccess(w.RequiredCapabilities)
}

// Validate checks the work unit's structural and binding invariants.
func (w WorkUnit) Validate() error {
	if w.ID.IsZero() {
		return fmt.Errorf("work unit id is required")
	}
	if !w.Kind.Valid() {
		return fmt.Errorf("unsupported work unit kind %q", w.Kind)
	}
	if strings.TrimSpace(w.Title) == "" {
		return fmt.Errorf("work unit title is required")
	}
	if w.ContractRevisionNumber < 1 {
		return fmt.Errorf("work unit contract revision number must be at least 1")
	}
	// Historical provider/model-unbound rows remain readable. New execution
	// admission calls ExecutionBindingForNewWork explicitly.
	if w.Provider != "" || w.ModelSelection != "" || strings.TrimSpace(w.Model) != "" {
		if _, err := w.ExecutionBinding(); err != nil {
			return err
		}
	}
	if strings.TrimSpace(w.OutputSummary) == "" {
		return fmt.Errorf("work unit output summary is required")
	}
	if len(w.EvidenceChecks) == 0 {
		return fmt.Errorf("work unit requires at least one evidence check")
	}
	if err := validateNonBlankStrings("evidence check", w.EvidenceChecks); err != nil {
		return err
	}
	if strings.TrimSpace(w.VerificationRequirement) == "" {
		return fmt.Errorf("work unit verification requirement is required")
	}
	if err := validateNonBlankStrings("stop condition", w.StopConditions); err != nil {
		return err
	}
	if err := validateWorkUnitIDs("dependency", w.ID, w.DependsOn); err != nil {
		return err
	}
	if err := validateCriterionIDs(w.CriterionIDs); err != nil {
		return err
	}
	if err := ValidateApprovedChecks(w.Checks, w.CriterionIDs); err != nil {
		return err
	}
	if err := validateUniqueCapabilityNames(w.RequiredCapabilities); err != nil {
		return err
	}
	return nil
}

func validateNonBlankStrings(kind string, values []string) error {
	for i, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s %d is blank", kind, i+1)
		}
	}
	return nil
}

func validateWorkUnitIDs(kind string, self WorkUnitID, ids []WorkUnitID) error {
	seen := map[WorkUnitID]struct{}{}
	for _, id := range ids {
		if id.IsZero() {
			return fmt.Errorf("work unit %s is blank", kind)
		}
		if id == self {
			return fmt.Errorf("work unit %s cannot reference itself", kind)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("work unit repeats %s %q", kind, id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validateCriterionIDs(ids []CriterionID) error {
	seen := map[CriterionID]struct{}{}
	for _, id := range ids {
		if id.IsZero() {
			return fmt.Errorf("work unit criterion id is blank")
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("work unit repeats criterion %q", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validateUniqueCapabilityNames(names []string) error {
	seen := map[string]struct{}{}
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			return fmt.Errorf("work unit required capability is blank")
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("work unit repeats required capability %q", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

// ExecutionBinding returns the exact binding represented by the WorkUnit. A
// provider-bound historical row with no model semantics is historical_unbound.
func (w WorkUnit) ExecutionBinding() (ExecutionBinding, error) {
	if strings.TrimSpace(string(w.Provider)) == "" {
		return ExecutionBinding{}, fmt.Errorf("execution binding provider is required")
	}
	selection := w.ModelSelection
	if selection == "" {
		selection = ExecutionBindingModelHistoricalUnbound
	}
	binding := ExecutionBinding{Provider: w.Provider, ModelSelection: selection, Model: w.Model}
	if err := binding.ValidateReadable(); err != nil {
		return ExecutionBinding{}, err
	}
	return binding, nil
}

// ExecutionBindingForNewWork returns the exact binding admitted for new work.
func (w WorkUnit) ExecutionBindingForNewWork() (ExecutionBinding, error) {
	binding, err := w.ExecutionBinding()
	if err != nil {
		return ExecutionBinding{}, err
	}
	if err := binding.ValidateForNewWork(); err != nil {
		return ExecutionBinding{}, err
	}
	return binding, nil
}

// BindExecution persists an exact execution binding on the work unit.
func (w *WorkUnit) BindExecution(binding ExecutionBinding) error {
	if err := binding.ValidateForNewWork(); err != nil {
		return err
	}
	w.Provider = binding.Provider
	w.ModelSelection = binding.ModelSelection
	w.Model = binding.Model
	return nil
}

// CapabilityGrant records the authority granted to one work unit.
type CapabilityGrant struct {
	ID    CapabilityGrantID
	Name  string
	Scope string
}

// Validate checks the capability grant's scope and authority.
func (g CapabilityGrant) Validate() error {
	if g.ID.IsZero() {
		return fmt.Errorf("capability grant id is required")
	}
	if strings.TrimSpace(g.Name) == "" {
		return fmt.Errorf("capability grant name is required")
	}
	if strings.TrimSpace(g.Scope) == "" {
		return fmt.Errorf("capability grant scope is required")
	}
	return nil
}

// WorkUnitRoutingDecision records why one WorkUnit received its persisted
// execution binding. Recommendation evidence and authority must agree exactly.
type WorkUnitRoutingDecision struct {
	WorkUnitID WorkUnitID      `json:"workUnitId"`
	Decision   RoutingDecision `json:"decision"`
}

// ValidateAgainst checks routing provenance against the exact work unit binding.
func (r WorkUnitRoutingDecision) ValidateAgainst(unit WorkUnit) error {
	if r.WorkUnitID.IsZero() || r.WorkUnitID != unit.ID {
		return fmt.Errorf("routing decision work unit %q does not match %q", r.WorkUnitID, unit.ID)
	}
	recommended, ok := r.Decision.RecommendedBinding()
	if !ok {
		return fmt.Errorf("work unit %s has no valid routing recommendation", unit.ID)
	}
	bound, err := unit.ExecutionBindingForNewWork()
	if err != nil {
		return err
	}
	if recommended != bound {
		return fmt.Errorf("routing recommendation for work unit %s does not match persisted execution binding", unit.ID)
	}
	return nil
}

// PlanRevision is immutable recommendation state until explicit owner approval.
type PlanRevision struct {
	ID                      PlanRevisionID
	OutcomeID               OutcomeID
	Number                  int64
	ContractRevisionNumber  int64
	Status                  PlanStatus
	Summary                 string
	Assumptions             []string
	Blockers                []string
	WorkUnits               []WorkUnit
	Grants                  []CapabilityGrant
	RoutingDecisions        []WorkUnitRoutingDecision
	RunBriefCoreDigest      string
	RunBriefCompiledDigest  string
	PlanningSessionID       PlanningSessionID
	SourceIntelligenceRunID IntelligenceRunID
	CreatedAt               time.Time
}

// Validate checks the plan revision's graph, authority, and proof bindings.
func (p PlanRevision) Validate() error {
	if p.ID.IsZero() {
		return fmt.Errorf("plan revision id is required")
	}
	if p.OutcomeID.IsZero() {
		return fmt.Errorf("plan revision outcome id is required")
	}
	if p.Number < 1 {
		return fmt.Errorf("plan revision number must be at least 1")
	}
	if p.ContractRevisionNumber < 1 {
		return fmt.Errorf("plan revision must bind a contract revision of at least 1")
	}
	if !p.Status.Valid() {
		return fmt.Errorf("unsupported plan status %q", p.Status)
	}
	if strings.TrimSpace(p.Summary) == "" {
		return fmt.Errorf("plan revision summary is required")
	}
	if err := validatePlanReviewStrings("assumption", p.Assumptions); err != nil {
		return err
	}
	if err := validatePlanReviewStrings("blocker", p.Blockers); err != nil {
		return err
	}
	if len(p.WorkUnits) == 0 {
		return fmt.Errorf("plan revision requires at least one work unit")
	}
	if len(p.WorkUnits) > MaxCanonicalPlanWorkUnits {
		return fmt.Errorf("plan revision has %d work units; maximum is %d", len(p.WorkUnits), MaxCanonicalPlanWorkUnits)
	}

	units := make(map[WorkUnitID]WorkUnit, len(p.WorkUnits))
	for i, unit := range p.WorkUnits {
		if err := unit.Validate(); err != nil {
			return fmt.Errorf("work unit %d: %w", i+1, err)
		}
		if unit.ContractRevisionNumber != p.ContractRevisionNumber {
			return fmt.Errorf("work unit %s binds contract revision %d, plan binds %d", unit.ID, unit.ContractRevisionNumber, p.ContractRevisionNumber)
		}
		if _, duplicate := units[unit.ID]; duplicate {
			return fmt.Errorf("duplicate work unit id %q", unit.ID)
		}
		units[unit.ID] = unit
	}
	for _, unit := range p.WorkUnits {
		for _, dependency := range unit.DependsOn {
			if _, exists := units[dependency]; !exists {
				return fmt.Errorf("work unit %s depends on unknown work unit %s", unit.ID, dependency)
			}
		}
	}
	if _, err := p.TopologicalWorkUnits(); err != nil {
		return err
	}

	seenGrants := make(map[string]bool, len(p.Grants))
	for i, grant := range p.Grants {
		if err := grant.Validate(); err != nil {
			return fmt.Errorf("grant %d: %w", i, err)
		}
		if seenGrants[grant.Name] {
			return fmt.Errorf("duplicate capability grant %q", grant.Name)
		}
		seenGrants[grant.Name] = true
	}
	if !isSHA256Hex(p.RunBriefCoreDigest) {
		return fmt.Errorf("plan revision requires a SHA-256 run brief core digest")
	}
	if p.PlanningSessionID.IsZero() != p.SourceIntelligenceRunID.IsZero() {
		return fmt.Errorf("planning Plan provenance must be complete")
	}
	return nil
}

func validatePlanReviewStrings(kind string, values []string) error {
	seen := map[string]struct{}{}
	for index, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return fmt.Errorf("plan %s %d is blank", kind, index+1)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("plan %s %q is duplicated", kind, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

// TopologicalWorkUnits derives deterministic serial order from dependency truth.
func (p PlanRevision) TopologicalWorkUnits() ([]WorkUnit, error) {
	units := map[WorkUnitID]WorkUnit{}
	indegree := map[WorkUnitID]int{}
	dependents := map[WorkUnitID][]WorkUnitID{}
	for _, unit := range p.WorkUnits {
		if unit.ID.IsZero() {
			return nil, fmt.Errorf("plan contains blank work unit id")
		}
		if _, duplicate := units[unit.ID]; duplicate {
			return nil, fmt.Errorf("duplicate work unit id %q", unit.ID)
		}
		units[unit.ID] = unit
		indegree[unit.ID] = 0
	}
	for _, unit := range p.WorkUnits {
		seen := map[WorkUnitID]struct{}{}
		for _, dependency := range unit.DependsOn {
			if dependency.IsZero() || dependency == unit.ID {
				return nil, fmt.Errorf("work unit %s has invalid dependency %s", unit.ID, dependency)
			}
			if _, exists := units[dependency]; !exists {
				return nil, fmt.Errorf("work unit %s depends on unknown work unit %s", unit.ID, dependency)
			}
			if _, duplicate := seen[dependency]; duplicate {
				return nil, fmt.Errorf("work unit %s repeats dependency %s", unit.ID, dependency)
			}
			seen[dependency] = struct{}{}
			indegree[unit.ID]++
			dependents[dependency] = append(dependents[dependency], unit.ID)
		}
	}
	var ready []WorkUnitID
	for id, degree := range indegree {
		if degree == 0 {
			ready = append(ready, id)
		}
	}
	sort.Slice(ready, func(i, j int) bool { return ready[i] < ready[j] })
	order := make([]WorkUnit, 0, len(units))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		order = append(order, units[id])
		next := append([]WorkUnitID(nil), dependents[id]...)
		sort.Slice(next, func(i, j int) bool { return next[i] < next[j] })
		for _, dependent := range next {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
				sort.Slice(ready, func(i, j int) bool { return ready[i] < ready[j] })
			}
		}
	}
	if len(order) != len(units) {
		return nil, fmt.Errorf("plan work unit dependencies contain a cycle")
	}
	return order, nil
}

// ValidateAgainstContract requires exact current-revision criterion coverage.
func (p PlanRevision) ValidateAgainstContract(revision ContractRevision) error {
	if p.OutcomeID != revision.OutcomeID || p.ContractRevisionNumber != revision.Number {
		return fmt.Errorf("plan does not bind the supplied contract revision")
	}
	valid := map[CriterionID]struct{}{}
	for _, criterion := range revision.Criteria {
		valid[criterion.ID] = struct{}{}
	}
	if len(valid) == 0 {
		return fmt.Errorf("contract revision has no canonical criteria")
	}
	covered := map[CriterionID]struct{}{}
	for _, unit := range p.WorkUnits {
		for _, criterionID := range unit.CriterionIDs {
			if _, exists := valid[criterionID]; !exists {
				return fmt.Errorf("work unit %s covers criterion %s outside the bound contract revision", unit.ID, criterionID)
			}
			covered[criterionID] = struct{}{}
		}
	}
	var missing []string
	for criterionID := range valid {
		if _, ok := covered[criterionID]; !ok {
			missing = append(missing, criterionID.String())
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf("plan does not cover required contract criteria: %s", strings.Join(missing, ", "))
	}
	return nil
}

// ValidateForApproval applies the invariants that turn recommendation into
// execution authority: complete criterion coverage, exact routing/binding
// agreement, executable model semantics, and least-privilege grants.
func (p PlanRevision) ValidateForApproval(revision ContractRevision) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := p.ValidateAgainstContract(revision); err != nil {
		return err
	}

	routing := make(map[WorkUnitID]WorkUnitRoutingDecision, len(p.RoutingDecisions))
	for _, record := range p.RoutingDecisions {
		if _, duplicate := routing[record.WorkUnitID]; duplicate {
			return fmt.Errorf("duplicate routing decision for work unit %s", record.WorkUnitID)
		}
		routing[record.WorkUnitID] = record
	}
	for _, unit := range p.WorkUnits {
		if _, err := unit.ExecutionBindingForNewWork(); err != nil {
			return fmt.Errorf("work unit %s is not executable: %w", unit.ID, err)
		}
		record, ok := routing[unit.ID]
		if !ok {
			return fmt.Errorf("work unit %s has no routing decision", unit.ID)
		}
		if err := record.ValidateAgainst(unit); err != nil {
			return err
		}
		if missing := MissingCapabilitiesForWorkUnit(p.Grants, unit); len(missing) > 0 {
			return fmt.Errorf("work unit %s is missing required capability grants: %s", unit.ID, strings.Join(missing, ", "))
		}
	}
	if len(routing) != len(p.WorkUnits) {
		return fmt.Errorf("plan has routing decisions for unknown work units")
	}
	return nil
}

// BindsCurrentContract reports whether the plan targets the current contract.
func (p PlanRevision) BindsCurrentContract(currentRevision int64) bool {
	return p.ContractRevisionNumber == currentRevision
}

// AuthorityIntersection returns capabilities present in every non-empty layer.
func AuthorityIntersection(layers ...[]string) []string {
	var intersection []string
	for i, layer := range layers {
		allowed := make(map[string]bool, len(layer))
		for _, name := range layer {
			allowed[name] = true
		}
		if i == 0 {
			intersection = make([]string, 0, len(allowed))
			for name := range allowed {
				intersection = append(intersection, name)
			}
			sort.Strings(intersection)
			continue
		}
		kept := intersection[:0]
		for _, name := range intersection {
			if allowed[name] {
				kept = append(kept, name)
			}
		}
		intersection = kept
	}
	sort.Strings(intersection)
	return intersection
}

// GrantsFailClosed rejects grants that exceed the authoritative capability set.
func GrantsFailClosed(grants []CapabilityGrant, authoritative []string) error {
	allowed := make(map[string]bool, len(authoritative))
	for _, name := range authoritative {
		allowed[name] = true
	}
	for _, grant := range grants {
		if !allowed[grant.Name] {
			return fmt.Errorf("capability %q is not authorized by every authority layer", grant.Name)
		}
	}
	return nil
}

// MissingCapabilitiesForWorkUnit compares actual grants with this WorkUnit's
// derived minimum; there is deliberately no universal read/write/exec trio.
func MissingCapabilitiesForWorkUnit(grants []CapabilityGrant, unit WorkUnit) []string {
	present := make(map[string]bool, len(grants))
	for _, grant := range grants {
		present[grant.Name] = true
	}
	var missing []string
	for _, name := range unit.RequiredCapabilities {
		if !present[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

type runBriefWorkUnit struct {
	ID                      string   `json:"id"`
	Title                   string   `json:"title"`
	Provider                string   `json:"provider,omitempty"`
	ModelSelection          string   `json:"modelSelection,omitempty"`
	Model                   string   `json:"model,omitempty"`
	Output                  string   `json:"output"`
	EvidenceChecks          []string `json:"evidenceChecks"`
	VerificationRequirement string   `json:"verificationRequirement"`
	StopConditions          []string `json:"stopConditions"`
	DependsOn               []string `json:"dependsOn,omitempty"`
	CriterionIDs            []string `json:"criterionIds,omitempty"`
	RequiredCapabilities    []string `json:"requiredCapabilities,omitempty"`
	Checks                  []string `json:"approvedChecks,omitempty"`
}

type runBriefCore struct {
	ContractRevisionNumber int64              `json:"contractRevisionNumber"`
	Goal                   string             `json:"goal"`
	SuccessCriteria        []string           `json:"successCriteria"`
	Review                 string             `json:"review"`
	Constraints            []string           `json:"constraints"`
	NonGoals               []string           `json:"nonGoals"`
	Clarification          string             `json:"clarification"`
	WorkUnits              []runBriefWorkUnit `json:"workUnits"`
	Grants                 []string           `json:"grants"`
}

// ComputePlanRunBriefCoreDigest freezes the complete approved execution graph,
// exact provider/model semantics, criterion coverage, and authority grants.
func ComputePlanRunBriefCoreDigest(revision ContractRevision, units []WorkUnit, grants []CapabilityGrant) (string, error) {
	if err := revision.Validate(); err != nil {
		return "", fmt.Errorf("run brief contract: %w", err)
	}
	plan := PlanRevision{ID: "digest-plan", OutcomeID: revision.OutcomeID, Number: 1, ContractRevisionNumber: revision.Number, Status: PlanStatusProposed, Summary: "digest", WorkUnits: units, Grants: grants, RunBriefCoreDigest: strings.Repeat("0", 64)}
	if err := plan.Validate(); err != nil {
		return "", fmt.Errorf("run brief plan: %w", err)
	}
	ordered, err := plan.TopologicalWorkUnits()
	if err != nil {
		return "", err
	}

	briefUnits := make([]runBriefWorkUnit, 0, len(ordered))
	for _, unit := range ordered {
		dependencies := make([]string, 0, len(unit.DependsOn))
		for _, id := range unit.DependsOn {
			dependencies = append(dependencies, id.String())
		}
		sort.Strings(dependencies)
		criteria := make([]string, 0, len(unit.CriterionIDs))
		for _, id := range unit.CriterionIDs {
			criteria = append(criteria, id.String())
		}
		sort.Strings(criteria)
		briefUnits = append(briefUnits, runBriefWorkUnit{
			ID: unit.ID.String(), Title: unit.Title, Provider: string(unit.Provider),
			ModelSelection: string(unit.ModelSelection), Model: unit.Model, Output: unit.OutputSummary,
			EvidenceChecks: sortedTrimmed(unit.EvidenceChecks), VerificationRequirement: unit.VerificationRequirement,
			StopConditions: sortedTrimmed(unit.StopConditions), DependsOn: dependencies, CriterionIDs: criteria,
			RequiredCapabilities: sortedTrimmed(unit.RequiredCapabilities),
			// Approved checks are frozen authority: if the command that will
			// be run could change after approval, the owner did not approve
			// what runs.
			Checks: runBriefChecks(unit.Checks),
		})
	}
	grantNames := make([]string, 0, len(grants))
	for _, grant := range grants {
		grantNames = append(grantNames, grant.Name+"@"+grant.Scope)
	}
	sort.Strings(grantNames)
	core := runBriefCore{
		ContractRevisionNumber: revision.Number, Goal: revision.Goal,
		SuccessCriteria: sortedTrimmed(revision.SuccessCriteria), Review: revision.Review,
		Constraints: sortedTrimmed(revision.Constraints), NonGoals: sortedTrimmed(revision.NonGoals),
		Clarification: revision.Clarification, WorkUnits: briefUnits, Grants: grantNames,
	}
	encoded, err := json.Marshal(core)
	if err != nil {
		return "", fmt.Errorf("encode run brief core: %w", err)
	}
	return DigestSHA256(encoded).String(), nil
}

// ComputeRunBriefCoreDigest remains a compatibility helper for historical
// one-unit callers; new Plan formation uses ComputePlanRunBriefCoreDigest.
func ComputeRunBriefCoreDigest(revision ContractRevision, unit WorkUnit, grants []CapabilityGrant) (string, error) {
	return ComputePlanRunBriefCoreDigest(revision, []WorkUnit{unit}, grants)
}

// runBriefChecks renders approved checks in a stable order for the digest.
func runBriefChecks(checks []ApprovedCheck) []string {
	out := make([]string, 0, len(checks))
	for _, check := range checks {
		out = append(out, fmt.Sprintf("%s|%s|%d|%s", check.ID, check.CriterionID, check.TimeoutSeconds, strings.Join(check.Argv, "\x00")))
	}
	sort.Strings(out)
	return out
}

func sortedTrimmed(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
