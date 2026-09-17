# Waldo Kennel v1 — canonical kernel and Work product architecture

> **Historical design (superseded for execution/session topology on 2026-09-15).** Use [Persistent mission runtime](../architecture/persistent-mission-runtime.md). Retained for product rationale and provenance.


- **Status:** Canonical authority for v1 Work/kernel implementation
- **Date:** 2026-09-04
- **Target repository:** `waldoco/Waldo-Kennel`
- **Integration branch:** `beta`
- **Provider baseline:** PR #92 merged; Codex, Claude Code, OpenCode, Cursor, Pi
- **Supersedes for Work/kernel semantics:** earlier review packets, prototypes, first-outcome handoffs, and the old interpretation that Outcome composition replaces a WorkUnit graph

This document defines the product objects, responsibility boundaries, user hierarchy, orchestration model, provider boundary, scheduler target, continuity system, and implementation non-goals for the first self-hosting Waldo Kennel kernel.

## 1. Product thesis

Coding agents are increasingly capable of producing code, running tests, researching a codebase, opening PRs, and spawning their own children. As execution becomes cheaper and more parallel, the user inherits coordination debt:

- which session was responsible for what;
- what the original intent was after several context rollovers;
- whether two agents duplicated or contradicted work;
- whether “done” means the user's desired state is actually true;
- what survived a provider/daemon crash;
- what failure needs attention versus automatic recovery;
- what artifact, decision, or proof actually changed the Project;
- what should persist after twenty sessions disappear into history.

Kennel moves the durable unit above the provider session:

> **The user manages Outcomes. Kennel manages the sessions required to make those Outcomes true.**

Kennel is not primarily a Kanban, terminal multiplexer, provider launcher, prompt router, or recursively prompting coordinator model. Those are implementation surfaces/primitives. The product is responsibility transfer with continuity, scheduling, isolation, recovery, attention compression, evidence, verification, and human closure.

## 2. Waldo / Kennel / provider boundary

Waldo is the durable user-facing intelligence and relationship. Kennel is the local deterministic control plane. Providers are replaceable executors.

```text
Waldo proposes / interprets / recommends
              ↓
Kennel validates / schedules / records / enforces
              ↓
Providers execute
```

Waldo may help draft Contracts, recommend decomposition and plans, explain state, summarize receipts, ask the smallest material question, propose follow-up Outcomes, and propose Project learning candidates.

Kennel owns canonical IDs/revisions, policy validation, authority intersection, dependency validation, scheduling, workspace custody, idempotency, recovery, effect fencing, evidence lineage, and attention projections.

Providers own their native sessions, internal reasoning, tools, and provider-native children. A provider's claim of completion is evidence/observation, never responsibility truth.

## 3. Canonical ontology

### 3.1 Project

A Project is a durable work/responsibility context, usually backed by a local repository or working folder. A Project does not become complete because one milestone finishes.

```text
Project
├── ProjectBriefRevision*
├── runtime/config/readiness facts
├── active top-level Outcomes*
├── accepted/archived Outcomes*
├── observed Project activity*
└── promoted Project Context*
```

### 3.2 ProjectBriefRevision

The Project Brief is persistent user-governed context: purpose/direction, product/technical context, architecture summary, conventions, important constraints, setup/run/test expectations, provenance, and revision history.

It **grounds** Outcome creation and planning. It is not a hidden Project Contract.

A Project Brief update does not silently widen authority, rewrite a current Contract, invalidate proof by itself, accept/reopen an Outcome, or mutate execution. If a Brief change materially conflicts with an active Outcome, Waldo proposes an explicit Contract/Plan revision.

### 3.3 Outcome

An Outcome is a finite responsibility: a desired state that can become true, be proved, be reviewed, be accepted, be reopened, or be archived.

Good examples:

- “Make provider readiness reliable.”
- “Ship Mission Control graph backed by scheduler truth.”
- “Remove the donor OutcomeTask overlay.”

Not Outcomes:

- “Waldo-Kennel project” — Project context;
- “Codex session 123” — execution trace;
- “Investigate terminal architecture” — usually a WorkUnit unless the investigation itself has independent success/acceptance criteria.

There is **no ontology-level Primary Project Outcome**. A UI may Pin/Focus an ordinary Outcome for navigation only.

### 3.4 ContractRevision

The Contract is the immutable governing definition of one Outcome revision. Semantically it contains:

- desired state/goal;
- stable success criteria;
- evidence expectations;
- review/verification expectation;
- constraints and non-goals;
- authority ceiling;
- stop/escalation conditions;
- material clarifications/assumptions;
- optional time conditions/facets.

A new Contract revision may stale affected Plans/decomposition/proof. It never edits history in place.

### 3.5 Responsibility decomposition vs execution decomposition

There are two different questions:

1. Has the **responsibility** split into independently meaningful results? Use contributing Outcomes.
2. How should one responsibility be **executed**? Use a WorkUnit DAG inside its PlanRevision.

> **Create another Outcome when responsibility splits. Create another WorkUnit when execution splits.**

ADR 0008 defines the exact boundary.

### 3.6 Direct Outcome

```text
Direct Outcome
└── ContractRevision
    └── PlanRevision
        └── WorkUnit DAG
            ├── WorkUnit A
            ├── WorkUnit B  depends A
            ├── WorkUnit C
            └── WorkUnit D  depends B,C
```

A PlanRevision is immutable and versioned. Material topology/provider/authority/workspace/effect changes produce a new revision rather than mutating the approved graph invisibly.

### 3.7 Decomposed Outcome

```text
Decomposed Outcome
└── ContractRevision
    └── DecompositionRevision
        ├── Contributing Outcome A
        │   └── Contract → Plan → WorkUnit DAG
        ├── Contributing Outcome B
        │   └── Contract → Plan → WorkUnit DAG
        └── Contributing Outcome C
            └── Contract → Plan → WorkUnit DAG
```

A decomposed parent does not simultaneously own direct WorkUnits in v1. If integration is itself an independently meaningful responsibility, represent it as a contributing Outcome or parent verification/acceptance activity rather than inventing a hybrid shape.

Composition remains capped at one contributing layer in v1. Contribution is criterion-bound. Child authority may narrow but never widen parent authority. Child proof/acceptance remains independently inspectable.

### 3.8 WorkUnit

A WorkUnit is the smallest Kennel-schedulable execution node. It exists when work needs an independent combination of:

- dependencies;
- provider capability/role;
- workspace or write isolation;
- retry/recovery semantics;
- artifacts/outputs;
- evidence responsibility;
- authority/effect boundary;
- scheduling/concurrency state.

Do not mirror every provider-native subagent into a WorkUnit. Native child agents remain beneath the provider session unless they need the Kennel-level boundary above.

### 3.9 Attempt and AgentSessionRef

An Attempt is one concrete try to execute one WorkUnit. Retry, provider handoff, recovery, or fresh independent verification creates a new Attempt.

An `AgentSessionRef` identifies the provider-native execution session for that Attempt. Sessions belong to Attempts; they are never durable Outcome/task identity.

Cross-provider switching preserves Kennel responsibility/context through a new Attempt + continuation/handoff receipt. It does not claim provider-internal hidden state was losslessly migrated.

### 3.10 Receipts, artifacts, evidence, verification, acceptance

```text
provider events + workspace facts
        ↓
SessionReceipt
        ↓
WorkUnitReceipt + ArtifactRef*
        ↓
criterion-bound EvidenceItem*
        ↓
VerificationRun*
        ↓
Ready for Review
        ↓
explicit user AcceptanceDecision
```

Plan execution and proof are independent. A valid state is:

```text
Plan  6 / 6
Proof 5 / 7
```

The Outcome is not Accepted.

Only the user creates `AcceptanceDecision`. Verifier success, CI green, commit/PR merge, or provider completion can support readiness; none closes the responsibility.

## 4. Outcome logical state and attention projection

Canonical logical progression:

```text
Draft
  ↓
Contracting
  ↓
Authorized
  ↓
In Progress ───────► Waiting
   │                  │
   ├────────► Needs You
   │
   ▼
Ready for Review
   │
   ├────────► Reopened ─► In Progress
   ▼
Accepted
   ↓
Archived
```

Cancelled/Stopped is a separate terminal path. The Board is an attention projection, not a second lifecycle database.

Recommended active Board columns:

```text
NEEDS YOU | IN PROGRESS | WAITING | READY FOR REVIEW
```

Accepted/Archived work leaves the active frontier and remains searchable/history-visible.

## 5. User experience hierarchy

```text
                    WALDO ISLAND
              ambient attention projection
                         │
                         ▼
              GLOBAL WORK / PROJECT
                  Board  ↔  List
                    top-level Outcomes
                         │
                         ▼
                  MISSION CONTROL
                   Contract | Graph
                         │
                         ▼
                     WorkUnit
                         │
                         ▼
                  Attempt / Session
                  SESSION INSPECTOR
```

### 5.1 Board/List

Board/List show top-level active Outcomes by default. Contributing Outcomes are primarily explored inside Mission Control; parent attention rolls up child decisions/blockers without duplicating the whole topology on the Board.

Outcome cards prioritize:

- desired result/title;
- Waldo one-line current state/next action;
- attention state;
- Plan progress;
- proof coverage;
- blockers/recovery;
- subordinate provider/session summary only when useful.

Do not show guessed provider percentage-complete.

### 5.2 Mission Control

Mission Control is one selected Outcome's operational representation.

**Contract** answers: what are we trying to make true, under what criteria/constraints/authority?

**Graph** answers: how is the authorized Plan/decomposition actually progressing?

For a direct Outcome, Graph renders the WorkUnit DAG. For a decomposed Outcome, it first renders the contribution layer and then the selected child's WorkUnit DAG. Attempts/Sessions appear as subordinate chips/details, not graph peers by default.

Every relationship in Graph must be backed by daemon state. Do not ship a concurrency visualization the scheduler cannot truthfully support.

### 5.3 Session Inspector

Session Inspector is the deepest technical escape hatch. It can expose transcript/native chat, terminal, diff, worktree, branch/commit, browser/preview, runtime/logs, process controls, and provider-native children when available.

Normal product supervision should not require opening it.

### 5.4 Waldo scope by navigation depth

The same Waldo is available everywhere; working context narrows with the surface:

| Surface | Typical question |
| --- | --- |
| Global Board | What needs me? |
| Project | What changed today? |
| Outcome Contract | Why is this criterion here? |
| Mission Graph | Can these WorkUnits run in parallel? |
| WorkUnit | Why is this blocked? |
| Session | What did this Attempt actually change? |

There are not separate assistant identities per surface.

## 6. Project onboarding and workspace modes

Work supports three useful entry patterns:

- **Open repository** — full Git-backed capabilities when valid;
- **Open folder** — usable for context/research/single-writer work; parallel write isolation remains unavailable until a supported workspace strategy exists;
- **Start new** — create directory/project and offer/perform Git initialization before unlocking parallel write WorkUnits.

For v1, Git worktrees are the normal isolation primitive for parallel writes. Do not build a second copy-based workspace engine only to make non-Git folders look equivalent.

Provider readiness shown during onboarding comes from daemon inventory. Non-ready providers may remain visible with truthful remediation; only admitted/ready capabilities can execute new work. Never silently replace the user's explicit provider choice.

## 7. Scheduler and WorkspaceLease

A WorkUnit is runnable only when:

```text
dependencies satisfied
AND required authority exists
AND provider capability exists
AND provider is locally ready
AND workspace lease can be acquired
AND concurrency budget exists
AND no ambiguous/unreconciled prior Attempt exists
AND write/effect constraints permit execution
```

Then:

```text
create Attempt
→ allocate/provision workspace
→ compile bounded RunBrief
→ launch provider
→ subscribe/observe
→ collect receipt + evidence candidates
→ reconcile terminal state
```

`WorkspaceLease` is durable/inspectable execution custody, including Project/Outcome/WorkUnit/Attempt identity, base revision, branch/worktree path, setup/environment revision, allocated ports/runtime processes, write lease, effect fence, preview endpoints, and cleanup state.

Target concurrency:

- reasoning/read-only work may overlap;
- independent writes use isolated worktrees/leases;
- repository integration/merge is separately serialized/controlled;
- consequential external effects use their own authority/idempotency fence;
- unknown prior effects/Attempts block duplicates until reconciled;
- cleanup errors preserve debris for inspection rather than deleting uncertain work.

ADR 0009 is authoritative.

## 8. Provider architecture

First-class identities:

- Codex
- Claude Code
- OpenCode
- Cursor
- Pi

The daemon uses capability-driven provider drivers rather than provider-name branching spread throughout the product.

Conceptual capability set:

```text
structured_session_control
resume
fork
interrupt
steering
approvals
tool_events
file_edit_events
child_session_identity
native_subagents
usage
structured_output
project_hooks_or_plugins
mcp_ingress
worktree_awareness
```

Conceptual driver surface:

```text
Probe / readiness
StartAttempt
Resume / reconcile
Send / steer
Interrupt / cancel
SubscribeEvents
SnapshotSession
ListChildren (optional)
Export receipt inputs
Explain unavailable capability
```

Use native structured protocols when stronger, ACP where appropriate, CLI/PTY as a fallback/inspection path, and provider hooks/plugins for external ingress. ACP is an adapter protocol, **not** Kennel's domain model.

Do not enable a role merely because a protocol nominally exposes an event. Pin/negotiate versions and pass conformance for the exact Kennel capability.

## 9. Context and continuity

Project continuity does not mean replaying every transcript. Compile bounded, purpose-specific context from:

1. current user intent and exact canonical revisions;
2. current Project Brief/runtime config;
3. repository rules/setup facts;
4. relevant Outcome/WorkUnit/dependency state;
5. attributed receipts/artifacts/evidence;
6. explicitly promoted Project context/skills;
7. provenance/freshness/omissions.

Canonical continuity path:

```text
SessionReceipt
→ WorkUnitReceipt
→ Outcome current brief / ledger
→ Project Context candidate
→ explicit governed promotion when appropriate
```

A provider context rollover can be mechanical only when responsibility, provider/model/profile/role, authority, budget/effect policy, workspace ownership, and material context remain compatible and old execution is safely fenced/reconciled. Otherwise create a user-visible decision/new Attempt/Plan as required.

## 10. External provider activity

Every integrated external session is one of:

- **Governed:** launched/bound under an explicit Kennel Outcome/WorkUnit/Attempt envelope.
- **Observed:** Kennel can observe Project/session activity but has no canonical responsibility binding.
- **Untracked:** no integration/consent/capability exists; Kennel makes no supervision claim.

Never fuzzy-auto-attach external activity. Waldo may propose “attach as research,” “create WorkUnit,” “create Outcome,” or “save Project learning candidate,” but lineage changes become explicit daemon mutations.

Rich state is impossible for arbitrary already-running processes with no provider hook/protocol. `unknown` is valid and must remain visible rather than guessed.

Provider/plugin code never writes SQLite directly; it calls daemon APIs enforcing lineage, authority, stale revision, idempotency, and effect rules.

## 11. Waldo Island

Island and desktop app are projections of the same canonical daemon/event state. There is no Island execution database.

Island prioritizes consequences:

- not “Codex exited 1”;
- but “Graph renderer Attempt failed; Waldo safely started recovery.”

Collapsed Island may summarize how many Outcomes need the user and how many are active. Expanded Island remains Outcome-first; Session detail is subordinate/deep-linkable.

## 12. Product/runtime principles borrowed from benchmark systems

Kennel should reuse mature runtime discipline without inheriting competitor ontology:

- **Emdash:** staged, inspectable, recoverable workspace provisioning;
- **Conductor:** setup/run/environment/ports are workspace state, not prompt text;
- **Herdr:** important runtime/attention states have explainable provenance;
- **Paseo/Superset/cmux:** UI, CLI, agents, and automation share one daemon/API control surface;
- **OpenHands:** runtime/workspace life is detached from the renderer client;
- **Xirp/Portal/LifeOS:** structured, provenance-bearing context beats transcript accumulation;
- **AO/Vibe Kanban:** terminal/worktree/diff/PR supervision surfaces are useful, while task/session identity is not the final user ontology;
- **provider-native subagents:** useful execution optimizations beneath a Kennel WorkUnit.

See [`../research/2026-09-04-kernel-runtime-reference-index.md`](../research/2026-09-04-kernel-runtime-reference-index.md).

## 13. Explicit v1 non-goals / deferred work

The first self-hosting kernel does not require:

- provider diversity for a useful mission;
- learned/opaque provider routing;
- automatic skill promotion;
- full personal/global Memory;
- hosted/remote workspaces;
- team/multiplayer governance;
- deep recursive Outcome decomposition;
- automatic acceptance;
- a perfect mirror of provider-native subagent graphs;
- lossless observation of arbitrary external sessions;
- Health/mobile/Relationship product surfaces.

Single-provider users must be first-class. If only Codex is ready, four independent WorkUnits may all use Codex in isolated Attempts/workspaces without warnings that multi-provider is required.

## 14. Figma semantic anchors

These node IDs freeze **meaning**, not pixel-perfect implementation. Re-open the actual Figma file before visual implementation.

- Board: `3253:35386`
- Contract/List drawer: `3253:36397` — canonical toggle is `Contract | Graph`
- Mission Graph: `3264:37187`
- Session Inspector AO-derived area: around `3144:26031`

Board cards are Outcomes. Mission Graph nodes are contributing Outcomes and/or WorkUnits backed by scheduler truth. Sessions remain subordinate execution traces.

## 15. Acceptance target

The architecture is validated when Kennel can use this lineage to implement real Kennel work while the user primarily supervises Outcomes, survives failures/restarts without duplicate/unknown work being misreported, and reaches review with criterion-bound proof without requiring transcript reconstruction.

See:

- [`kennel-build-program.md`](kennel-build-program.md)
- [`kennel-dogfood-acceptance-matrix.md`](kennel-dogfood-acceptance-matrix.md)
- [`../superpowers/plans/2026-09-04-kennel-builds-kennel.md`](../superpowers/plans/2026-09-04-kennel-builds-kennel.md)
