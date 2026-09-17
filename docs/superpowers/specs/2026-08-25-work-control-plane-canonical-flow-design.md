# Work Control Plane — canonical flow and daemon contract

> **Superseded for vNext (2026-09-15):** Historical control-plane specification; its session/planner topology is superseded. Use [persistent mission runtime](../../architecture/persistent-mission-runtime.md) and the [persistent-session execution map](../../roadmap/persistent-session-execution-map.md). Retained for provenance.


- **Status:** Canonical Work control-plane companion specification
- **Original date:** 2026-08-25
- **Consolidated:** 2026-09-04
- **Parent authority:** `docs/product/kennel-v1-product-architecture.md`, ADR 0008, ADR 0009
- **Implementation truth:** `docs/STATUS.md`

This document describes how a user responsibility moves through the Kennel control plane. It does not claim every state is already implemented.

## 1. Control-plane principle

There is one durable Waldo relationship, one local Kennel daemon authority, and replaceable provider execution.

```text
User intent
  ↓
Waldo understanding / recommendation
  ↓
canonical daemon validation + state
  ↓
WorkUnit scheduler / WorkspaceLease / provider Attempt
  ↓
receipts / artifacts / evidence / verification
  ↓
user Acceptance or Reopen
```

The daemon is the always-running coordinator **control plane**. A planner/coordinator LLM is only a bounded provider role inside an Attempt; it is not the canonical scheduler or writer.

## 2. Project entry

The user may:

- open an existing Git repository;
- open a working folder;
- start a new Project.

Project registration creates/records Project identity and runtime/config facts. It does not create an immortal Project Outcome.

### 2.1 Project Brief

A Project has versioned `ProjectBriefRevision`s containing durable context such as direction, architecture, conventions, setup/run/test expectations, and provenance.

The Brief grounds Waldo's understanding but does not grant runtime authority or acceptance. Enforcement-critical criteria/constraints/effects belong in finite Outcome Contracts.

### 2.2 Workspace capability

Git-backed Projects support the v1 parallel-write isolation path. A non-Git folder may be used for context/research/single-writer work but cannot claim parallel worktree isolation. Starting a new Project can initialize Git before parallel writes are enabled.

## 3. Provider readiness and Project preferences

Project onboarding reads provider inventory/readiness from the daemon for exactly the current first-class identities: Codex, Claude Code, OpenCode, Cursor, Pi.

A Project default provider is a preference, not permanent execution identity.

The UI may show non-ready providers with:

- Ready;
- needs authentication/configuration;
- not installed;
- installed but missing a required capability.

An explicit selected provider never silently falls back to another provider.

Mission/WorkUnit admission intersects:

```text
user/Project preference
∩ installed + locally ready providers
∩ required provider capabilities
∩ WorkUnit role/runtime needs
∩ authority/effect policy
∩ concurrency/budget policy
```

Current provider role depth is reported by `STATUS.md`; do not assume all five providers have identical structured control.

## 4. Start an Outcome

The user states what should become true inside a selected Project.

The initial UI should be compact: Project + desired result + Analyze/Continue. Submission begins bounded intake/understanding; it does not need to force the user through ontology fields first.

Waldo/analysis may classify the request, ask one material question at a time, and propose a Contract. A structured proposal reaches the daemon API/callback and passes the same validation as a hand-authored proposal. Model text itself is not canonical state.

## 5. Contracting

`ContractRevision` freezes what the finite Outcome means:

- desired state;
- observable success criteria;
- Evidence expectations;
- review/verification expectation;
- constraints/non-goals;
- authority ceiling;
- stop/escalation conditions;
- material clarification/assumptions.

The user may edit/re-analyze before confirmation. Confirmation atomically creates/advances the Outcome + immutable Contract revision according to the service contract.

Provider assignment is not Contract truth; it belongs in execution planning.

## 6. Decide whether responsibility or execution splits

Waldo can propose topology, but the semantic decision follows ADR 0008.

### Responsibility split

Use contributing Outcomes when pieces have independently meaningful desired states, Contracts, proof, authority, and acceptance.

```text
Parent Outcome
└── DecompositionRevision
    ├── Contributing Outcome A
    ├── Contributing Outcome B
    └── Contributing Outcome C
```

Contribution remains criterion-bound and child authority cannot widen parent authority.

### Execution split

Use WorkUnits when one direct Outcome needs multiple execution nodes under one Contract/acceptance.

```text
Direct Outcome
└── PlanRevision
    ├── WorkUnit A
    ├── WorkUnit B depends A
    ├── WorkUnit C
    └── WorkUnit D depends B,C
```

A decomposed parent does not also own direct WorkUnits in v1. Each direct child may own its own DAG.

## 7. Plan proposal and authorization

Waldo/provider may propose the smallest sufficient Plan topology. Deterministic daemon validation checks:

- WorkUnit identity and shape;
- dependency existence/self-dependency/cycles;
- overlap/conflicting write/effect scope where knowable;
- provider capability/readiness;
- Contract authority containment;
- grants/effects/budget;
- evidence/verification obligations;
- workspace requirements;
- stale revision binding.

Authorization freezes the current Plan revision. A material topology/dependency/provider-role/authority/workspace/effect change becomes a new PlanRevision rather than a silent graph edit.

## 8. WorkUnit admission and scheduling

A WorkUnit is runnable only when the ADR 0009 gates pass:

```text
dependencies satisfied
AND authority exists
AND provider capability exists
AND provider locally ready
AND workspace lease available
AND concurrency budget available
AND prior Attempts reconciled
AND write/effect constraints allow execution
```

Then the daemon creates an Attempt, provisions/allocates the workspace, compiles a bounded RunBrief, launches the provider, confirms/records provider-native identity, observes events, and reconciles the result.

The scheduler never asks an LLM whether a dependency cycle is acceptable or whether an unknown effect can be repeated.

## 9. Workspace and effect custody

Each write-capable Attempt operates inside an explicit WorkspaceLease. Independent writes may overlap in isolated Git worktrees.

Repository integration/merge is a separate boundary. Consequential external effects are separately frozen/authorized/fenced.

A provider session process is not the lease authority. Renderer death does not release an Attempt. Missing provider events do not prove death/completion.

## 10. RunBrief and bounded context

Every WorkUnit/Attempt receives purpose-built context rather than the full Project transcript.

Inputs include:

1. exact Project/Outcome/Contract/Plan/WorkUnit IDs and revisions;
2. WorkUnit objective/output expectations;
3. relevant Project Brief/setup/rules;
4. admitted dependency receipts/artifacts;
5. exact grants/authority/effect limits;
6. workspace/base revision facts;
7. evidence/verification expectations;
8. provenance/freshness/known omissions.

Provider-specific adapters compile that provider-neutral brief into the appropriate native form.

## 11. Provider session continuity and handoff

Provider sessions are replaceable executors.

### Same-provider continuation

Use native resume/reconcile when the provider capability is proven and canonical scope/authority/workspace/effects remain compatible.

### Context rollover

A mechanical context refresh may occur without a user decision only if Project/Outcome/Contract/Plan/WorkUnit/provider/model/profile/role/authority/budget/workspace/effect policy remain materially unchanged, old execution is safely contained/reconciled, and an attributed continuation packet can be produced.

### Cross-provider handoff

Create a new Attempt and pass a bounded receipt/handoff packet. Do not claim hidden provider state was migrated losslessly.

A provider/model/role/authority/cost/workspace/effect change that is material becomes the appropriate explicit user/Plan decision.

## 12. Act & Observe

The Board shows responsibility. Mission Control shows the selected Outcome Contract/graph. Session Inspector shows deep provider execution.

Provider completion, commit, check, PR, or process exit is an observation. It may support a WorkUnit receipt/evidence candidate but does not close the Outcome.

Important runtime state must be explainable: blocked by which dependency, waiting on which lease, which prior Attempt is unknown, what recovery superseded what failure.

## 13. Receipts and evidence

Structured provider events + workspace facts produce `SessionReceipt`. Retained Attempt results aggregate into `WorkUnitReceipt`.

Receipts carry provenance and may generate Evidence candidates. Evidence binds to exact current criterion/revision identity. Contradictory or stale evidence remains visible; it is not silently repurposed.

Verification declares its actual independence class. A verifier/model cannot create Acceptance.

## 14. Prove & Close

Outcome Plan execution may complete while proof remains incomplete.

The daemon derives `Ready for Review` only when the current Contract's required proof/review conditions are satisfied according to policy.

The user can:

- Accept;
- request rework / create recovery or revised Plan;
- revise the Contract when responsibility changed;
- Reopen later;
- create a successor Outcome.

Only explicit user action creates the immutable AcceptanceDecision.

## 15. Composed Outcome close

Contributing Outcomes retain independent proof and Acceptance decisions. Parent readiness rolls up criterion-bound contribution and any parent-retained proof.

The product may batch the user's review interaction, but it does not fan one decision into N immutable Acceptance records automatically. All contributors accepted makes the parent ready for its own owner decision; it does not imply parent Acceptance.

## 16. External provider activity

Integrated external activity is explicitly:

- Governed — exact Kennel binding;
- Observed — Project/session known, no canonical work binding;
- Untracked — no integration/consent/capability.

Waldo may recommend binding/creating work/learning, but no fuzzy auto-attachment mutates lineage.

Provider integrations call daemon APIs. They do not write SQLite directly.

## 17. Project continuity and learning boundary

Canonical continuity path:

```text
SessionReceipt
→ WorkUnitReceipt
→ Outcome brief/ledger
→ Project Context candidate
→ explicit governed promotion
```

The Project Waldo conversation is a durable bounded interaction history, not one immortal model context and not the source of Outcome truth.

ADR 0005 governs later learning/skill promotion. No trace automatically becomes an active rule or skill.

## 18. Recovery invariants

- renderer crash does not end execution;
- daemon restart reconciles before retry;
- provider crash creates truthful failed/interrupted/unconfirmed state according to evidence;
- unknown is never silently completed;
- dirty/unknown workspaces are not force-deleted;
- retries create new Attempts;
- recovery never changes Outcome identity;
- only a responsibility change creates/revises Outcome/Contract semantics.

The first self-hosting acceptance cases are listed in `docs/product/kennel-dogfood-acceptance-matrix.md`.
