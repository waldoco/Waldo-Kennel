package domain

import (
	"fmt"
	"sort"
	"strings"
)

// MaxPlanDraftWorkUnits is an operational bound on untrusted planning output,
// not a law of Outcomes or the scheduler.
const MaxPlanDraftWorkUnits = 16

// WorkUnitIntent describes the kind of local work intelligence believes a
// WorkUnit requires. It is deliberately NOT a capability or grant: the Go
// control plane maps this bounded intent to the minimum capability set and
// then checks that minimum against the confirmed Contract ceiling and current
// daemon policy.
type WorkUnitIntent string

const (
	// WorkUnitIntentLegacy marks rows created before intent was frozen. It is readable history, never approvable.
	WorkUnitIntentLegacy WorkUnitIntent = "legacy_unknown"
	// WorkUnitIntentInspect requests read-only inspection work.
	WorkUnitIntentInspect WorkUnitIntent = "inspect"
	// WorkUnitIntentModify requests workspace mutation without execution.
	WorkUnitIntentModify WorkUnitIntent = "modify"
	// WorkUnitIntentExecute requests local execution without mutation.
	WorkUnitIntentExecute WorkUnitIntent = "execute"
	// WorkUnitIntentModifyAndExecute requests mutation followed by execution.
	WorkUnitIntentModifyAndExecute WorkUnitIntent = "modify_and_execute"
)

// Valid reports whether the work intent is supported.
func (i WorkUnitIntent) Valid() bool {
	switch i {
	case WorkUnitIntentInspect, WorkUnitIntentModify, WorkUnitIntentExecute, WorkUnitIntentModifyAndExecute:
		return true
	default:
		return false
	}
}

// RequiredCapabilities deterministically maps non-authoritative work intent to
// the least local authority Kennel must grant for that unit. Intelligence never
// names capability strings directly.
func (i WorkUnitIntent) RequiredCapabilities() ([]string, error) {
	switch i {
	case WorkUnitIntentInspect:
		return []string{CapabilityWorktreeRead}, nil
	case WorkUnitIntentModify:
		return []string{CapabilityWorktreeRead, CapabilityWorktreeWrite}, nil
	case WorkUnitIntentExecute:
		return []string{CapabilityWorktreeRead, CapabilityWorktreeExec}, nil
	case WorkUnitIntentModifyAndExecute:
		return []string{CapabilityWorktreeRead, CapabilityWorktreeWrite, CapabilityWorktreeExec}, nil
	default:
		return nil, fmt.Errorf("unsupported work unit intent %q", i)
	}
}

// PlanDraftProposal is non-authoritative intelligence output. It describes what
// work probably needs doing; deterministic Kennel compilation derives authority,
// routing requirements, mandatory stops, and verification obligations.
type PlanDraftProposal struct {
	Summary     string
	WorkUnits   []PlanDraftWorkUnit
	Assumptions []string
	Blockers    []string
}

// PlanDraftWorkUnit deliberately contains no provider, model, capability grant,
// stop policy, or canonical verification authority. CriteriaCovered uses stable
// model-facing aliases (for example C1/C2) that the control plane maps to the
// Contract's canonical CriterionIDs. Intent is bounded non-authoritative work
// classification; the control plane alone maps it to capability requirements.
type PlanDraftWorkUnit struct {
	Key             string
	Title           string
	Intent          WorkUnitIntent
	OutputSummary   string
	CriteriaCovered []string
	DependsOn       []string
	EvidenceIdeas   []string
	// CheckCommands are proposed deterministic checks. Like everything else
	// in a draft they are a suggestion: the control plane resolves the
	// criterion alias, validates the command shape and bounds the timeout
	// before any of it becomes approved authority.
	CheckCommands []PlanDraftCheck
}

// MaxPlanDraftChecksPerWorkUnit bounds pathological planning output. It is
// named operational policy, not a law of Outcomes.
const MaxPlanDraftChecksPerWorkUnit = 8

// PlanDraftCheck is one proposed deterministic check, still in model-facing
// terms: it names a criterion by alias, not by internal identity.
type PlanDraftCheck struct {
	CriterionAlias string
	Argv           []string
	TimeoutSeconds int64
}

// Validate checks the bounded, non-authoritative draft shape.
func (p PlanDraftProposal) Validate() error {
	if strings.TrimSpace(p.Summary) == "" {
		return fmt.Errorf("plan draft summary is required")
	}
	if len(p.WorkUnits) == 0 {
		return fmt.Errorf("plan draft requires at least one work unit")
	}
	if len(p.WorkUnits) > MaxPlanDraftWorkUnits {
		return fmt.Errorf("plan draft has %d work units; maximum planning output is %d", len(p.WorkUnits), MaxPlanDraftWorkUnits)
	}

	units := make(map[string]PlanDraftWorkUnit, len(p.WorkUnits))
	for i, unit := range p.WorkUnits {
		key := strings.TrimSpace(unit.Key)
		if key == "" {
			return fmt.Errorf("plan draft work unit %d key is required", i+1)
		}
		if _, duplicate := units[key]; duplicate {
			return fmt.Errorf("plan draft work unit key %q is duplicated", key)
		}
		if strings.TrimSpace(unit.Title) == "" {
			return fmt.Errorf("plan draft work unit %q title is required", key)
		}
		if !unit.Intent.Valid() {
			return fmt.Errorf("plan draft work unit %q has unsupported intent %q", key, unit.Intent)
		}
		if strings.TrimSpace(unit.OutputSummary) == "" {
			return fmt.Errorf("plan draft work unit %q output summary is required", key)
		}
		if len(unit.CriteriaCovered) == 0 {
			return fmt.Errorf("plan draft work unit %q must cover at least one contract criterion", key)
		}
		if err := validateUniqueNonBlankPlanDraftList("criterion alias", unit.CriteriaCovered); err != nil {
			return fmt.Errorf("plan draft work unit %q: %w", key, err)
		}
		if err := validateUniqueNonBlankPlanDraftList("evidence idea", unit.EvidenceIdeas); err != nil {
			return fmt.Errorf("plan draft work unit %q: %w", key, err)
		}
		if len(unit.CheckCommands) > MaxPlanDraftChecksPerWorkUnit {
			return fmt.Errorf("plan draft work unit %q proposes %d checks; maximum is %d", key, len(unit.CheckCommands), MaxPlanDraftChecksPerWorkUnit)
		}
		for i, check := range unit.CheckCommands {
			if strings.TrimSpace(check.CriterionAlias) == "" {
				return fmt.Errorf("plan draft work unit %q check %d names no criterion", key, i+1)
			}
			if len(check.Argv) == 0 {
				return fmt.Errorf("plan draft work unit %q check %d has no command", key, i+1)
			}
		}
		units[key] = unit
	}

	for key, unit := range units {
		seenDependencies := map[string]struct{}{}
		for _, raw := range unit.DependsOn {
			dependency := strings.TrimSpace(raw)
			if dependency == "" {
				return fmt.Errorf("plan draft work unit %q has a blank dependency", key)
			}
			if dependency == key {
				return fmt.Errorf("plan draft work unit %q cannot depend on itself", key)
			}
			if _, exists := units[dependency]; !exists {
				return fmt.Errorf("plan draft work unit %q depends on unknown work unit %q", key, dependency)
			}
			if _, duplicate := seenDependencies[dependency]; duplicate {
				return fmt.Errorf("plan draft work unit %q repeats dependency %q", key, dependency)
			}
			seenDependencies[dependency] = struct{}{}
		}
	}
	if _, err := p.TopologicalOrder(); err != nil {
		return err
	}
	if err := validateUniqueNonBlankPlanDraftList("assumption", p.Assumptions); err != nil {
		return err
	}
	if err := validateUniqueNonBlankPlanDraftList("blocker", p.Blockers); err != nil {
		return err
	}
	return nil
}

// TopologicalOrder derives deterministic serial execution order from dependency
// truth. Array serialization order is deliberately irrelevant.
func (p PlanDraftProposal) TopologicalOrder() ([]string, error) {
	units := make(map[string]PlanDraftWorkUnit, len(p.WorkUnits))
	proposalPosition := make(map[string]int, len(p.WorkUnits))
	indegree := make(map[string]int, len(p.WorkUnits))
	dependents := make(map[string][]string, len(p.WorkUnits))
	for _, unit := range p.WorkUnits {
		key := strings.TrimSpace(unit.Key)
		if key == "" {
			return nil, fmt.Errorf("plan draft contains a blank work unit key")
		}
		if _, duplicate := units[key]; duplicate {
			return nil, fmt.Errorf("plan draft work unit key %q is duplicated", key)
		}
		units[key] = unit
		proposalPosition[key] = len(proposalPosition)
		indegree[key] = 0
	}
	for key, unit := range units {
		seen := map[string]struct{}{}
		for _, raw := range unit.DependsOn {
			dependency := strings.TrimSpace(raw)
			if dependency == "" || dependency == key {
				return nil, fmt.Errorf("plan draft contains an invalid dependency for %q", key)
			}
			if _, exists := units[dependency]; !exists {
				return nil, fmt.Errorf("plan draft work unit %q depends on unknown work unit %q", key, dependency)
			}
			if _, duplicate := seen[dependency]; duplicate {
				return nil, fmt.Errorf("plan draft work unit %q repeats dependency %q", key, dependency)
			}
			seen[dependency] = struct{}{}
			indegree[key]++
			dependents[dependency] = append(dependents[dependency], key)
		}
	}

	var ready []string
	for key, degree := range indegree {
		if degree == 0 {
			ready = append(ready, key)
		}
	}
	sort.SliceStable(ready, func(i, j int) bool { return proposalPosition[ready[i]] < proposalPosition[ready[j]] })
	order := make([]string, 0, len(units))
	for len(ready) > 0 {
		key := ready[0]
		ready = ready[1:]
		order = append(order, key)
		next := append([]string(nil), dependents[key]...)
		sort.SliceStable(next, func(i, j int) bool { return proposalPosition[next[i]] < proposalPosition[next[j]] })
		for _, dependent := range next {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
				sort.SliceStable(ready, func(i, j int) bool { return proposalPosition[ready[i]] < proposalPosition[ready[j]] })
			}
		}
	}
	if len(order) != len(units) {
		return nil, fmt.Errorf("plan draft work unit dependencies contain a cycle")
	}
	return order, nil
}

func validateUniqueNonBlankPlanDraftList(kind string, values []string) error {
	seen := map[string]struct{}{}
	for i, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return fmt.Errorf("%s %d is blank", kind, i+1)
		}
		if _, duplicate := seen[trimmed]; duplicate {
			return fmt.Errorf("%s %q is duplicated", kind, trimmed)
		}
		seen[trimmed] = struct{}{}
	}
	return nil
}
