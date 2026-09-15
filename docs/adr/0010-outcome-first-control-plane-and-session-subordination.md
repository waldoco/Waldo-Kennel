# ADR 0010 — Outcome-first control plane and session subordination

> **Supersession note (2026-09-15):** Outcome-first authority remains accepted. [ADR 0017](0017-persistent-mission-runtime-and-bounded-supervision.md) supersedes any one-shot or session-lifecycle reading and adds persistent WorkUnit threads plus bounded Mission Supervisor intelligence.


- **Status:** Accepted
- **Date:** 2026-09-08
- **Decision owners:** Waldo Kennel product/kernel
- **Amends:** ADR 0008, ADR 0009, `docs/product/kennel-v1-product-architecture.md`, and the pre-execution planner restriction in `docs/superpowers/specs/2026-08-25-work-control-plane-canonical-flow-design.md`
- **Does not supersede:** the Outcome/Contract/Plan/WorkUnit/Attempt lineage, user-only final acceptance, additive migrations, or deterministic authority checks

## Context

Kennel's canonical documents already place `Outcome` above `ContractRevision`, `PlanRevision`, `WorkUnit`, `Attempt`, and `AgentSessionRef`. The implemented product still contains inherited Agent Orchestrator paths that make a provider session behave like the primary unit of work:

- creating or starting work can still fall through to a provider/session-first path;
- project worker/orchestrator settings can be interpreted as launch decisions instead of preferences;
- sidebar/session routes can make an Outcome resolve to a CLI/chat session;
- Contract analysis is currently implemented by spawning an ordinary worker session/worktree;
- plan generation is still largely a deterministic direct-WorkUnit constructor rather than a distinct non-authoritative planning phase;
- Mission Control and the Outcome surfaces exist, but legacy session machinery can bypass them.

The result is a product that uses Outcome vocabulary while behaving like a session manager.

Kennel's product requirement is different. The user expresses a desired state of the world and should be able to supervise the path from intent to verified result without treating an agent process as the responsibility itself.

## Decision

### 1. Outcome is the primary durable work object

For new work, the canonical responsibility hierarchy is:

```text
Outcome
  ↓
ContractRevision
  ↓
PlanRevision
  ↓
WorkUnit
  ↓
Attempt
  ↓
AgentSessionRef
  ↓
EvidenceItem
  ↓
VerificationRun
  ↓
AcceptanceDecision
```

A provider session is an execution resource underneath an Attempt. It is never the unit of user intent and never the canonical destination for an Outcome.

### 2. The product flow is Contract-first and Plan-authorized

The normal direct-Outcome lifecycle is:

```text
capture intent
  ↓
understand / analyze
  ↔ material clarification when necessary
  ↓
proposed ContractRevision
  ↓
owner review + confirmation
  ↓
plan drafting
  ↓
proposed PlanRevision
  ↓
owner approval
  ↓
Mission Control / execution
  ↓
Attempts + sessions
  ↓
evidence + verification
  ↓
owner acceptance
```

Creating an intake or confirming a Contract MUST NOT create an execution Attempt or `AgentSessionRef`.

A Plan may be proposed before execution, but execution authority begins only after explicit Plan approval. Approval freezes the WorkUnit authorization inputs, including exact execution binding semantics required by the routing ADR/WT3 work.

### 3. Mission Control is an Outcome projection, not a separate responsibility

Mission Control is the execution/supervision view of one Outcome. It presents:

- the approved Contract and current Plan;
- WorkUnit progress and dependencies;
- current and prior Attempts;
- exact provider/model bindings and routing provenance;
- decisions, blockers, evidence, verification, and acceptance state;
- drill-down to a terminal/native provider session when needed.

Mission Control does not own a second lifecycle or database.

### 4. Outcome navigation never resolves to a session destination

Selecting an Outcome from Board/List/sidebar opens the Outcome workspace at the stage derived from canonical facts.

Terminal/chat/session UI is available only as a deliberate drill-down from a WorkUnit/Attempt or via historical/debug tooling. Generic session boards may remain for compatibility/inspection during migration, but they are not the primary new-work navigation model.

### 5. Pre-execution intelligence is allowed, but is not execution

The previous canonical-flow specification said planner/coordinator LLM work may exist only inside an `Attempt`. That restriction is amended.

Kennel may perform bounded, lineage-tracked **Intelligence Runs** before Plan approval for purposes such as:

- Contract analysis;
- asking one or more material clarification questions under product policy;
- repository/context inspection needed to propose success criteria;
- Plan drafting;
- plan explanation or critique.

The canonical logical object is `IntelligenceRun`, with a kind such as `contract_analysis` or `plan_draft`.

An Intelligence Run:

- proposes data; it does not create or mutate execution authority;
- is linked to the exact Intake/Outcome and source revision it analyzed;
- records provider/model/provenance when model-backed;
- must have a bounded input and a structured, validated output;
- cannot create a WorkUnit Attempt, capability grant, external effect, acceptance decision, or execution binding by itself;
- defaults to read-only analysis authority;
- must fail visibly or fall back to an explicit offline/manual proposal path rather than silently starting a different provider;
- may have a provider-native process/session underneath it, but that process is not an `AgentSessionRef` for execution and is not the user's Outcome destination.

Where a provider runtime cannot enforce the requested read-only boundary reliably, Kennel should prefer an intelligence adapter that can operate on explicitly supplied context rather than granting a coding-agent worktree broad write/execute capability merely to draft a Contract or Plan.

### 6. Kennel, not an LLM, is the orchestrator

The deterministic control plane owns:

- lifecycle transitions;
- revision identity and optimistic concurrency;
- authority and approval gates;
- capability ceilings and grants;
- provider/model admission;
- routing constraints;
- dependency readiness;
- scheduling;
- retry/cancel/recovery policy;
- effect fencing;
- evidence linkage;
- verification admission;
- acceptance authority.

Models/providers may analyze, propose, critique, recommend, and execute authorized WorkUnits. Their output is input data to deterministic policy, not a command that bypasses the control plane.

### 7. Project agent selections are preferences, not execution authority

Project-level coordinator/worker selections are mutable baselines used by Intelligence Runs and routing. Outcome/Contract-specific execution preferences may override the Project baseline according to the preference-routing ADR/WT3 semantics.

No Project setting may directly authorize a new Attempt simply because an Outcome was created.

### 8. Legacy AO authority paths are removed for new work

The migration SHALL remove or gate new-work paths that:

- create a provider session directly from a task/outcome composer;
- treat project worker/orchestrator selection as the provider to launch without Plan authority;
- navigate an Outcome directly to a session route;
- derive responsibility status primarily from provider session status;
- allow historical provider-only binding to execute as if it were a newly approved exact binding.

Historical sessions and compatibility data remain readable. Migration must not destructively rewrite history.

## UX-stage mapping

The existing five Work surfaces remain useful. They map to domain facts rather than becoming a second persisted state machine:

| Work surface | Canonical facts represented |
|---|---|
| Enter | Project selection / initial capture |
| Understand | Intake captured, Intelligence Run active, clarification needed, Contract proposal ready |
| Decide & Authorize | Contract confirmed, Plan drafting/proposed, Plan approval/action required |
| Act & Observe | Approved Plan, WorkUnits/Attempts executing or blocked |
| Prove & Close | Evidence/Verification, owner review, AcceptanceDecision |

The UI derives the destination from canonical facts. Do not persist these labels as a competing lifecycle.

## MVP execution scope

The first usable Outcome-control-plane MVP does **not** require the full ADR 0008/0009 parallel DAG scheduler before proving the product loop.

The MVP may execute approved WorkUnits serially, provided it is truthful about that constraint and still preserves:

- explicit Plan approval;
- exact WorkUnit provider/model binding;
- no hidden provider fallback;
- one canonical Attempt lineage;
- restart/idempotency safety;
- evidence/verification/acceptance.

ADR 0008/0009 remain the accepted target for bounded DAG concurrency, WorkspaceLease ownership, and narrower effect fences after the vertical MVP is working.

## Consequences

### Positive

- the visible product and durable domain share one mental model;
- provider sessions stop dictating navigation and lifecycle;
- Contract/Plan intelligence can improve without granting execution authority;
- WT3 routing has a correct authority boundary in which to live;
- the MVP can be tested end-to-end before the full parallel scheduler lands;
- later personal-agent surfaces can reuse Outcome/Contract semantics beyond coding.

### Costs

- legacy task/session entry points must be audited and removed/gated;
- the current session-backed intake analyzer must be refactored or reclassified behind an IntelligenceRun adapter;
- plan drafting requires an explicit intelligence seam rather than only a deterministic one-WorkUnit constructor;
- frontend route/sidebar tests need to assert Outcome-first behavior;
- persistence/API changes are required for IntelligenceRun provenance and planning status.

## Rejected alternatives

### Keep the current session-first flow and add more Outcome chrome

Rejected. It preserves the exact mismatch this ADR exists to remove.

### Make the coordinator LLM the orchestrator

Rejected. A model cannot be the authority for approval, retries, idempotency, effect fencing, or acceptance. It may advise the orchestrator; it is not the orchestrator.

### Block the MVP on the full parallel DAG/WorkspaceLease scheduler

Rejected. The target architecture remains valid, but requiring every concurrency feature before the first truthful end-to-end Outcome loop delays product learning without improving the authority model.

### Delete all AO-derived runtime code

Rejected. Provider adapters, PTY/process supervision, worktrees, auth/readiness, model discovery, session inspection, git/browser surfaces, and recovery foundations remain valuable execution machinery. The authority paths are being replaced, not the useful runtime chassis.

## Acceptance tests implied by this ADR

At minimum the implementation must prove:

1. creating an Outcome/intake creates no execution Attempt/session;
2. model-backed Contract analysis may run, ask a material question, and return a validated proposal without gaining execution authority;
3. confirming the Contract creates/advances the Outcome but still creates no execution Attempt/session;
4. a Plan is visible and editable/re-proposable before approval;
5. no WorkUnit starts before explicit Plan approval;
6. after approval, Mission Control starts/observes an Attempt using the exact frozen provider/model binding;
7. selecting the Outcome always opens its Outcome workspace, never the provider session route;
8. terminal/chat is reachable only as Attempt/session drill-down;
9. verification/evidence are attributable to the WorkUnit/Attempt lineage;
10. only the user creates the final AcceptanceDecision;
11. restart/replay does not duplicate an authorized Attempt;
12. historical session data remains readable without becoming valid new-work authority.
