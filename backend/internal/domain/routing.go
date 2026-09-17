package domain

import (
	"sort"
	"strings"
)

// RoutingRole keeps worker and coordinator admission independent.
type RoutingRole string

const (
	// RoutingRoleWorker identifies ordinary execution admission.
	RoutingRoleWorker RoutingRole = "worker"
	// RoutingRoleCoordinator identifies coordinator admission.
	RoutingRoleCoordinator RoutingRole = "coordinator"
)

// CapabilitySupport is normalized adapter truth. Unknown is not supported.
type CapabilitySupport string

const (
	// CapabilitySupported records confirmed adapter capability.
	CapabilitySupported CapabilitySupport = "supported"
	// CapabilityUnsupported records a confirmed capability gap.
	CapabilityUnsupported CapabilitySupport = "unsupported"
	// CapabilityUnknown records an unconfirmed capability.
	CapabilityUnknown CapabilitySupport = "unknown"
)

// RoutingPreference is planning input only. An explicit model is local to this
// provider; it is never interpreted as a global model requirement.
type RoutingPreference struct {
	Provider       string
	ModelSelection ExecutionPreferenceModelSelection
	Model          string
}

// RoutingRequirements are deterministic hard requirements derived by Kennel.
type RoutingRequirements struct {
	Role             RoutingRole
	HardCapabilities []string
	Preference       *RoutingPreference
}

// RoutingCandidate is normalized harness inventory. ModelSelection/Model describe
// the candidate's executable fallback semantics when no explicit preference wins.
type RoutingCandidate struct {
	ID                       string
	Provider                 string
	ModelSelection           ExecutionBindingModelSelection
	Model                    string
	WorkerEligible           bool
	CoordinatorEligible      bool
	Readiness                CapabilitySupport
	Capabilities             map[string]CapabilitySupport
	Models                   map[string]CapabilitySupport
	ExecutionTokenAccounting CapabilitySupport
}

// RoutingCandidateEvaluation records admission results for one candidate.
type RoutingCandidateEvaluation struct {
	CandidateID    string   `json:"candidateId"`
	Provider       string   `json:"provider"`
	Admissible     bool     `json:"admissible"`
	Preferred      bool     `json:"preferred,omitempty"`
	RejectionCodes []string `json:"rejectionCodes,omitempty"`
	Reasons        []string `json:"reasons,omitempty"`
}

// RoutingDecisionStatus is the result state of a routing recommendation.
type RoutingDecisionStatus string

const (
	// RoutingDecisionRecommended identifies a usable recommendation.
	RoutingDecisionRecommended RoutingDecisionStatus = "recommended"
	// RoutingDecisionNoValidCandidate records fail-closed routing.
	RoutingDecisionNoValidCandidate RoutingDecisionStatus = "no_valid_candidate"
)

// RoutingPolicyVersion identifies the deterministic routing policy revision.
const RoutingPolicyVersion = "wt3-v2-deterministic"

// RoutingDecision is recommendation provenance, never execution authority.
type RoutingDecision struct {
	Status                    RoutingDecisionStatus          `json:"status"`
	PolicyVersion             string                         `json:"policyVersion"`
	CapabilitySnapshot        string                         `json:"capabilitySnapshot,omitempty"`
	Role                      RoutingRole                    `json:"role"`
	EffectivePreference       *RoutingPreference             `json:"effectivePreference,omitempty"`
	Requirements              RoutingRequirements            `json:"requirements"`
	RecommendedCandidateID    string                         `json:"recommendedCandidateId,omitempty"`
	RecommendedProvider       string                         `json:"recommendedProvider,omitempty"`
	RecommendedModelSelection ExecutionBindingModelSelection `json:"recommendedModelSelection,omitempty"`
	RecommendedModel          string                         `json:"recommendedModel,omitempty"`
	Evaluations               []RoutingCandidateEvaluation   `json:"evaluations"`
}

// RecommendedBinding converts recommendation provenance into the exact binding
// the WorkUnit must persist. A no-candidate decision cannot become authority.
func (d RoutingDecision) RecommendedBinding() (ExecutionBinding, bool) {
	if d.Status != RoutingDecisionRecommended || strings.TrimSpace(d.RecommendedProvider) == "" {
		return ExecutionBinding{}, false
	}
	binding := ExecutionBinding{
		Provider:       AgentHarness(d.RecommendedProvider),
		ModelSelection: d.RecommendedModelSelection,
		Model:          d.RecommendedModel,
	}
	if err := binding.ValidateForNewWork(); err != nil {
		return ExecutionBinding{}, false
	}
	return binding, true
}

// RouteExecution hard-gates normalized candidates, uses an admissible explicit
// preference when possible, otherwise chooses the lexicographically-stable
// candidate ID. No provider brand or invented quality weight participates.
func RouteExecution(req RoutingRequirements, candidates []RoutingCandidate, snapshot string) RoutingDecision {
	decision := RoutingDecision{
		Status:              RoutingDecisionNoValidCandidate,
		PolicyVersion:       RoutingPolicyVersion,
		CapabilitySnapshot:  snapshot,
		Role:                req.Role,
		EffectivePreference: req.Preference,
		Requirements:        req,
		Evaluations:         make([]RoutingCandidateEvaluation, 0, len(candidates)),
	}

	type admitted struct {
		candidate RoutingCandidate
		binding   ExecutionBinding
		preferred bool
	}
	var admittedCandidates []admitted

	ordered := append([]RoutingCandidate(nil), candidates...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	for _, candidate := range ordered {
		eval := RoutingCandidateEvaluation{CandidateID: candidate.ID, Provider: candidate.Provider}
		reject := func(code, reason string) {
			eval.RejectionCodes = append(eval.RejectionCodes, code)
			eval.Reasons = append(eval.Reasons, reason)
		}

		switch req.Role {
		case RoutingRoleWorker:
			if !candidate.WorkerEligible {
				reject("ROLE_INELIGIBLE", "candidate is not eligible for worker execution")
			}
		case RoutingRoleCoordinator:
			if !candidate.CoordinatorEligible {
				reject("ROLE_INELIGIBLE", "candidate is not eligible for coordinator execution")
			}
		default:
			reject("ROLE_UNKNOWN", "requested role is unsupported")
		}

		if candidate.Readiness != CapabilitySupported {
			code := "READINESS_UNKNOWN"
			if candidate.Readiness == CapabilityUnsupported {
				code = "NOT_READY"
			}
			reject(code, "candidate readiness is not confirmed")
		}
		for _, capability := range req.HardCapabilities {
			support := candidate.Capabilities[capability]
			if support != CapabilitySupported {
				code := "CAPABILITY_UNKNOWN"
				if support == CapabilityUnsupported {
					code = "CAPABILITY_UNSUPPORTED"
				}
				reject(code, "mandatory capability "+capability+" is not confirmed")
			}
		}

		preferred := req.Preference != nil && candidate.Provider == req.Preference.Provider
		eval.Preferred = preferred
		binding := ExecutionBinding{
			Provider:       AgentHarness(candidate.Provider),
			ModelSelection: candidate.ModelSelection,
			Model:          candidate.Model,
		}
		if preferred {
			switch req.Preference.ModelSelection {
			case ExecutionPreferenceModelExplicit:
				support := candidate.Models[req.Preference.Model]
				if support != CapabilitySupported {
					code := "MODEL_UNKNOWN"
					if support == CapabilityUnsupported {
						code = "MODEL_UNSUPPORTED"
					}
					reject(code, "preferred model is not supported by the preferred provider")
				} else {
					binding.ModelSelection = ExecutionBindingModelExplicit
					binding.Model = req.Preference.Model
				}
			case ExecutionPreferenceModelProviderDefault:
				if candidate.ModelSelection != ExecutionBindingModelProviderDefault {
					reject("MODEL_BINDING_INVALID", "preferred provider does not expose provider-default model semantics")
				} else {
					binding.ModelSelection = ExecutionBindingModelProviderDefault
					binding.Model = ""
				}
			default:
				reject("PREFERENCE_INVALID", "execution preference model semantics are invalid")
			}
		}

		if !preferred {
			if err := binding.ValidateForNewWork(); err != nil {
				reject("MODEL_BINDING_INVALID", err.Error())
			} else if binding.ModelSelection == ExecutionBindingModelExplicit {
				support := candidate.Models[binding.Model]
				if support != CapabilitySupported {
					code := "MODEL_UNKNOWN"
					if support == CapabilityUnsupported {
						code = "MODEL_UNSUPPORTED"
					}
					reject(code, "candidate explicit model support is not confirmed")
				}
			}
		}

		eval.Admissible = len(eval.RejectionCodes) == 0
		decision.Evaluations = append(decision.Evaluations, eval)
		if eval.Admissible {
			admittedCandidates = append(admittedCandidates, admitted{candidate: candidate, binding: binding, preferred: preferred})
		}
	}

	if len(admittedCandidates) == 0 {
		return decision
	}
	winner := admittedCandidates[0]
	for _, candidate := range admittedCandidates {
		if candidate.preferred {
			winner = candidate
			break
		}
	}
	decision.Status = RoutingDecisionRecommended
	decision.RecommendedCandidateID = winner.candidate.ID
	decision.RecommendedProvider = string(winner.binding.Provider)
	decision.RecommendedModelSelection = winner.binding.ModelSelection
	decision.RecommendedModel = winner.binding.Model
	return decision
}
