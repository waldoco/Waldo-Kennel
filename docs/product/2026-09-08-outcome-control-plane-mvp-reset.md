# Waldo Kennel — Outcome Control Plane MVP Reset

> **Historical implementation reset.** Its accepted foundations remain, but the active runtime and order are [Persistent mission runtime](../architecture/persistent-mission-runtime.md) and the [persistent-session execution map](../roadmap/persistent-session-execution-map.md).


**Work launch clarification (2026-09-09):** Read [Work launch experience and execution handoff](2026-09-09-kennel-work-launch-experience.md) for general Outcome scope, Board/List and Mission Graph behavior, delivery, focused launch navigation and assignment gates. Existing architecture/ADR boundaries remain authoritative.

> Post-PR99 note (2026-09-08): this document describes the target product. Current implementation facts and remaining tasks are in [STATUS](../STATUS.md) and the [execution plan](../superpowers/plans/2026-09-08-outcome-control-plane-mvp-reset.md). ADR0012 supersedes older fallback guidance.

- **Status:** Current MVP product/program amendment
- **Date:** 2026-09-08
- **Authority:** ADR 0010 + ADR 0011 + ADR 0012
- **Long-term architecture remains:** `kennel-v1-product-architecture.md`, ADR 0008, ADR 0009
- **Implementation plan:** `../superpowers/plans/2026-09-08-outcome-control-plane-mvp-reset.md`

## 1. Why this reset exists

Kennel already contains much of the target Outcome ontology and a substantial amount of Outcome-oriented UI, but the end-to-end experience still falls back to inherited Agent Orchestrator semantics:

```text
Project
→ choose worker/orchestrator
→ enter task/outcome
→ provider session appears
→ user lands in CLI/chat
```

That flow makes a provider process the center of the product. It prevents Kennel from feeling like an intelligent system that understands a desired outcome, establishes what success means, proposes a plan, gets authority, coordinates execution, and carries the responsibility until proof/acceptance.

This amendment resets the MVP around the actual Waldo mental model.

## 2. Product thesis for the MVP

> **Waldo Kennel is a durable Outcome control plane. It turns user intent into an explicit Contract, derives and authorizes a Plan, coordinates heterogeneous agents to execute the Plan, gathers evidence, verifies progress, and maintains continuity until the Outcome is accepted or requires human judgment. Agent sessions are ephemeral execution resources, never the unit of user intent.**

The MVP is successful when a user can complete that loop once, truthfully, on a real repository.

It is not necessary for the first loop to support every planned concurrency, recursive decomposition, or personal-agent capability.

## 3. Mental model

```text
                         USER INTENT
                              │
                              ▼
                           OUTCOME
                  "What should become true?"
                              │
                              ▼
                    OUTCOME UNDERSTANDING
              context / questions / assumptions
                              │
                              ▼
                      CONTRACT REVISION
          desired state • criteria • constraints • authority
                              │
                        owner confirms
                              │
                              ▼
                        PLAN REVISION
       work units • dependencies • routing • verification intent
                              │
                        owner approves
                              │
                              ▼
                       MISSION CONTROL
                              │
                 ┌────────────┼────────────┐
                 ▼            ▼            ▼
              WorkUnit     WorkUnit     WorkUnit
                 │            │            │
              Attempt       Attempt       Attempt
                 │            │            │
             Codex/CC      OpenCode       Cursor
                 │            │            │
             Session       Session       Session
                 └────────────┼────────────┘
                              ▼
                           EVIDENCE
                              │
                              ▼
                         VERIFICATION
                              │
                              ▼
                    ACCEPTANCE DECISION
```

For the first MVP the scheduler may run WorkUnits serially. The model above remains valid; concurrency is an execution capability, not the definition of an Outcome.

## 4. Three planes

### Intelligence plane

Purpose: improve understanding and planning.

Examples:

- derive a Contract proposal from the user's statement and repository context;
- ask a material clarification;
- draft a Plan;
- critique/explain a proposal;
- recommend worker/provider/model choices.

Output is structured proposal data plus provenance. Intelligence has **no direct execution authority**.

### Control plane

The Go daemon is Waldo Kennel's actual orchestrator.

It owns canonical state, validation, revisioning, approval, routing admission, scheduling, exact binding, retries, recovery, effect fencing, evidence linkage, verification admission, and acceptance authority.

### Execution plane

Codex, Claude Code, OpenCode, Cursor, Pi, and future worker runtimes execute approved WorkUnits. Their provider-native sessions are subordinate to Attempt lineage.

## 5. User journey

### Step A — Register Project

The user adds/imports a repository.

Project setup may capture:

- coordinator/intelligence preference;
- worker execution preference;
- provider/model preference;
- project-level capability/effect policy.

These are mutable baselines, not permission to launch work.

### Step B — Create Outcome

The user writes a desired result in natural language.

Example:

> Ship a public downloadable beta of Kennel that a clean Apple Silicon Mac can install and use through the core Outcome flow.

At capture time:

- persist exact user intent;
- create no execution Attempt;
- create no `AgentSessionRef`;
- navigate to the Understand surface.

### Step C — Understand

Kennel performs an `IntelligenceRun(kind=contract_analysis)` when intelligence is available.

The run may inspect bounded Project/repository context and either:

1. propose a Contract; or
2. ask a material clarification whose answer changes the Contract.

The product should prefer proposing with explicit assumptions over turning intake into a questionnaire.

The user sees requested/effective reasoning provenance when known. Under ADR0012 there is no deterministic/offline proposal floor; unavailable reasoning is an actionable setup/retry state.

### Step D — Contract review and confirmation

The proposal is editable before confirmation.

The Contract should capture at least:

- title;
- desired state;
- stable success criteria;
- expected evidence per criterion;
- review method;
- constraints;
- non-goals;
- stop conditions;
- authority ceiling;
- relevant facets/context;
- execution preference when supplied at Outcome level.

Confirmation freezes a new immutable `ContractRevision` and creates/advances the durable Outcome.

Confirmation still creates **no execution Attempt/session**.

### Step E — Plan drafting

Kennel performs an `IntelligenceRun(kind=plan_draft)` or the explicit deterministic/manual floor.

The Plan proposal contains bounded WorkUnits with:

- intent/description;
- expected output/evidence;
- dependencies when needed;
- required capabilities;
- verification intent;
- routing decision/provenance;
- recommended provider/model binding semantics.

For the MVP, one or a small number of serial WorkUnits is acceptable. Do not fabricate parallelism before the scheduler supports it.

### Step F — Decide & Authorize

The user reviews the Plan before execution.

The UI must make visible:

- what Kennel plans to do;
- what it will not do;
- why each worker/provider/model was selected;
- capability/effect implications;
- anything blocked/unavailable;
- expected proof.

**Plan approval is the execution authority boundary.**

Approval freezes the exact WorkUnit execution binding required for new work. Mutable Project preferences cannot silently change an approved WorkUnit afterward.

### Step G — Mission Control / Act & Observe

After approval, the Outcome opens Mission Control.

Mission Control shows the Outcome, not a generic agent terminal:

- Contract summary;
- current Plan and WorkUnit progress;
- current Attempt;
- exact provider/model binding;
- status/elapsed activity;
- blockers and decisions;
- evidence produced so far;
- verification state;
- drill-down controls such as **Open terminal** or **View provider session**.

An execution session may now exist because it belongs to an authorized Attempt.

### Step H — Prove & Close

Execution completion does not equal Outcome completion.

Kennel gathers EvidenceItems, runs/records VerificationRuns, and presents the result against Contract criteria.

The user chooses the AcceptanceDecision. A provider/session cannot autonomously mark the Outcome accepted.

## 6. Domain-state projection

Do not persist a second UI lifecycle just to drive screens. The following is the intended conceptual projection from canonical facts:

```text
DRAFT / CAPTURED
    ↓
ANALYZING
    ↔ NEEDS_INPUT
    ↓
CONTRACT_READY
    ↓ owner confirms Contract
PLANNING
    ↓
PLAN_READY / ACTION_REQUIRED
    ↓ owner approves Plan
EXECUTING
    ↔ NEEDS_ACTION
    ↓
VERIFYING
    ↓
AWAITING_ACCEPTANCE
    ↓ owner decides
COMPLETED
```

Orthogonal terminal/supersession facts may include `CANCELLED`, `FAILED`, and `SUPERSEDED` where the existing domain vocabulary supports them.

The existing Work UX stages map onto those facts:

| Work stage | Meaning |
|---|---|
| Enter | choose/register Project and capture intent |
| Understand | intake, analysis, clarification, Contract review |
| Decide & Authorize | Plan drafting/review/routing/approval |
| Act & Observe | Mission Control, WorkUnits, Attempts, blockers |
| Prove & Close | evidence, verification, owner acceptance |

## 7. Current code we should reuse

The frontend already contains useful Outcome-first surfaces. They should be wired to canonical state rather than replaced wholesale:

- `frontend/src/renderer/components/outcome/AdaptiveIntakeSurface.tsx`
- `IntakeAnalysisWaiting.tsx`
- `IntakeContractReview.tsx`
- `OutcomeLifecycleShell.tsx`
- `OutcomeDecideAuthorizeSurface.tsx`
- `OutcomeMissionControl.tsx`
- `OutcomeRunSurface.tsx`
- `OutcomeAttemptTerminalPanel.tsx`
- `OutcomeProveCloseSurface.tsx`
- `OutcomesOverviewSurface.tsx`
- `WorkShell.tsx`
- `frontend/src/renderer/routes/_shell.work.tsx`

The existing intake service also already has useful durable mechanics:

- exact intent capture before analysis;
- immutable proposal revisions;
- optimistic revision checks;
- a material clarification path;
- callback/refusal/expiry handling;
- explicit reasoning failure/retry state (ADR0012 removed the old deterministic floor);
- user confirmation before Outcome creation.

Those mechanics should survive the reset.

## 8. Current code whose authority must be removed/gated

Audit and remove from the normal new-work path:

- `TaskComposer.tsx` direct task/session behavior;
- `NewTaskDialog.tsx` / `GlobalNewTaskDialog.tsx` paths that start sessions;
- generic `SessionsBoard` as a primary Work destination;
- session-first sidebar Outcome navigation;
- `OrchestratorActivityIndicator` / replacement UX when it represents a provider process as the orchestrator;
- direct Project worker/orchestrator launch assumptions;
- service code that re-reads mutable Project worker selection at Plan approval/Attempt start;
- historical provider-only WorkUnit bindings as executable new-work authority.

Keep session inspector, terminal/native chat, diff/browser, provider runtime, auth/readiness, worktrees, git, and recovery as deep execution infrastructure.

## 9. Intelligence implementation strategy

### Implemented boundary and remaining work

PR99 merged the provider-neutral `IntelligenceProvider` / `LLMClient` ports, IntelligenceRun storage and Anthropic/OpenAI adapters. Evolve these existing seams; do not create a second reasoning service or restore the removed session-spawn analyzer.

ADR0012 selects owner-configured model-backed reasoning with no deterministic floor. Current keys/models resolve from startup environment; secure settings, readiness, bounded repository context, complete provenance and interrupted-call reconciliation remain implementation work. Keys must not be persisted in plaintext canonical Work tables or logs.

See L2/L3 of the [current execution plan](../superpowers/plans/2026-09-08-outcome-control-plane-mvp-reset.md). Any older optional-key/fallback guidance is superseded by ADR0012. Pre-authorization reasoning remains non-authoritative and cannot spawn execution or mutate Project/external state.

## 10. WT3 integration

PR99 merged preference-aware routing into Plan formation/authorization. Preserve it and finish the remaining UI/runtime integration in the current execution plan.

Canonical rules remain:

- preference != recommendation != authority;
- Outcome execution preference overrides mutable Project baseline;
- provider empty/model empty means no Outcome override / Project baseline;
- provider + provider-default model semantics is valid;
- provider + explicit model is valid;
- model without provider is invalid;
- no preference means no implicit Codex;
- unknown capability/readiness cannot satisfy a hard requirement;
- router is provider-neutral, deterministic, and explainable;
- no valid candidate becomes Action Required / `NO_VALID_CANDIDATE`;
- approved WorkUnit freezes exact provider/model semantics;
- Attempt executes that binding and never re-routes from mutable Project settings.

WT3 is merged into beta. Its exact-binding and graph foundations must be preserved; do not follow pre-merge branch/migration instructions. Remaining work is enumerated by the post-PR99 execution plan.

## 11. MVP cut line

### Must work end-to-end

1. add/register Project;
2. set coordinator/worker preferences;
3. capture Outcome intent;
4. model-backed Contract analysis with actionable failure/retry;
5. material clarification when necessary;
6. editable Contract review;
7. explicit Contract confirmation;
8. model-backed Plan drafting with actionable failure/retry;
9. provider/model routing with visible explanation;
10. explicit Plan approval;
11. Mission Control opens;
12. one or more approved WorkUnits execute serially;
13. each Attempt uses exact frozen provider/model binding;
14. terminal/session is available only as drill-down;
15. Evidence/Verification render against criteria;
16. user can accept/reject/request more work;
17. restart/reload preserves durable state without duplicate Attempts;
18. historical sessions remain inspectable.

### May remain after MVP

- parallel WorkUnit execution;
- complete WorkspaceLease scheduler;
- arbitrary DAG fan-out/fan-in;
- advanced learned routing;
- automatic skill learning/promotion;
- cross-project/personal Memory;
- hosted/cloud execution;
- team/multiplayer governance;
- recursive Outcome decomposition beyond current capability;
- full provider role parity;
- autonomous final acceptance.

ADR 0008/0009 remain the architecture target for parallel scheduling after this vertical slice.

## 12. Navigation acceptance

For the MVP:

- primary Work navigation is Outcome-first;
- selecting an Outcome opens `/work?...outcome=<id>` at the derived lifecycle stage;
- the user is never automatically redirected from an Outcome to `/sessions/<id>`;
- generic sessions remain an inspection/compatibility surface, not the main Work board;
- terminal/native provider UI appears under an Attempt drill-down;
- unfinished Home surfaces should be hidden/disabled from the primary beta navigation rather than presented as complete;
- empty Outcomes CTAs must route through Project registration/selection and Outcome capture, never through a generic new-session dialog.

## 13. Detailed acceptance scenario

Use this as the principal dogfood scenario in the implementation session.

### Given

- a clean or isolated Kennel beta profile;
- a real local Git repository registered as a Project;
- at least one ready worker provider;
- owner reasoning credential configured and the selected adapter ready; missing configuration is an explicit remediation state.

### When

The user creates:

> Add a small visible README section that explains how to run this project locally, verify the command works, and leave the repository ready for review.

### Then

Before Contract confirmation:

- no execution Attempt exists;
- no execution `AgentSessionRef` exists;
- Understand displays an analysis/proposal or a material question;
- the Contract is editable and shows provenance.

After Contract confirmation but before Plan approval:

- Outcome exists;
- Plan proposal is visible;
- routing/provider/model recommendation is visible;
- no execution Attempt/session exists.

After Plan approval:

- Mission Control is the destination;
- an Attempt starts only for an approved WorkUnit;
- its provider/model equals the frozen binding;
- changing the Project preference does not mutate the running/approved binding;
- the provider session is available as drill-down, not the Outcome itself.

At completion:

- evidence identifies the changed README and verification command/result;
- verification maps to Contract criteria;
- provider completion alone does not accept the Outcome;
- the user explicitly accepts or requests further work.

After daemon/app restart:

- the same Outcome/Contract/Plan/Attempt lineage is recovered;
- no duplicate execution is started merely because the UI reopened.

## 14. Reference products by layer

Use references selectively rather than adopting another product's ontology wholesale:

| Reference | Borrow |
|---|---|
| Factory Missions | goal → clarification → plan → approval → Mission Control experience |
| Devin | understand/ask/spec before execution |
| Linear | durable work navigation and hierarchy |
| Temporal | durable execution/retry/recovery mental model |
| Conductor OSS | model proposals as data under deterministic control |
| Superset / Conductor desktop / AO / Emdash / Nimbalyst | workspace/session/runtime mechanics |
| OpenHands | separation of UI, agent service, automation/runtime layers |

Kennel's differentiating layer is the Outcome/Contract/authority/evidence continuity above those execution mechanics.

## 15. Implementation sequence

The detailed plan is authoritative for file-level execution, but the product sequence is:

```text
M0  architecture/docs + preflight truth
 ↓
M1  remove Outcome→Session bypasses
 ↓
M2  IntelligenceRun + Contract intelligence
 ↓
M3  Contract confirmation → Plan intelligence
 ↓
M4  WT3 routing + exact immutable binding
 ↓
M5  Plan approval → truthful Mission Control + serial scheduler
 ↓
M6  Attempt execution + evidence + verification + acceptance
 ↓
M7  remove/gate remaining AO authority paths
 ↓
M8  full verification + real-daemon dogfood
```

Only after the vertical loop works should the build program resume the full ADR 0008/0009 parallel DAG and WorkspaceLease scheduler as the next kernel expansion.
