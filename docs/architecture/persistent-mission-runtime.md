# Persistent mission runtime

- Status: canonical target architecture
- Decision date: 2026-09-15
- Baseline: `outcome-loop` at `f69c3387b13b00a1395c7acf7bd468163e6ae023`
- Scope: Codex-first Outcome planning, supervision, execution, recovery, and owner experience

This is the canonical architecture for new Outcome execution. It supersedes one-shot, session-first, hidden-transcript, and model-as-scheduler designs. [Runtime status](../STATUS.md) distinguishes accepted target behavior from code that exists today. [The execution map](../roadmap/persistent-session-execution-map.md) is the only active implementation order.

## Product architecture

```mermaid
flowchart TB
    U[Owner] --> T[One product timeline]
    T --> C[Contract conversation]
    C --> CA[Frozen Contract revision]
    CA --> P[Fresh mission planning conversation]
    P --> PA[Approved Plan revision and WorkUnit DAG]
    PA --> D[Kernel daemon]
    D <--> MS[Mission Supervisor]
    D --> W1[WorkUnit A Attempt]
    D --> W2[WorkUnit B Attempt]
    D --> W3[WorkUnit C Attempt]
    W1 --> R[Verified Result]
    W2 --> R
    W3 --> R
    R --> A{Owner Accept}
```

The UI is one timeline. Its internal conversations have distinct authority:

- The Contract conversation clarifies intent and freezes a Contract artifact.
- A fresh `/mission` planning conversation receives that artifact and proposes a Plan and WorkUnit DAG.
- The owner approves a specific Plan revision before execution is released.
- The Mission Supervisor provides mission-level intelligence during execution.
- WorkUnit threads execute bounded work.
- The daemon is the deterministic authority, state, scheduler, custody, check, and projection layer.

Codex proposes and executes. The daemon authorizes, schedules, records, checks, and projects. The owner controls scope, material authority, external effects, replacement, and final Accept.

## Runtime topology

```mermaid
flowchart LR
    UI[Desktop / Web / CLI] -->|typed owner command| S1[S1 local owner command authority]
    PL[Thin plugins, skills, hooks] -->|typed ingress or sensor event| S1
    S1 -->|authenticated command| D[(Kernel daemon + durable store)]
    D -->|mission event packet| MS[Persistent Mission Supervisor thread]
    MS -->|typed recommendation or bounded steering command| D
    D -->|authorized session command| SC[Unified session controller]
    SC --> A1[Attempt A: exclusive worktree + primary Codex thread]
    SC --> B1[Attempt B: exclusive worktree + primary Codex thread]
    A1 -->|events, claim, artifacts| SC
    B1 -->|events, claim, artifacts| SC
    SC --> D
    D -->|MissionProjection| UI
```

The Supervisor and daemon are deliberately coupled through a typed protocol, not shared memory or hidden control:

1. the daemon publishes versioned mission events and bounded context;
2. the Supervisor interprets them and emits typed recommendations or commands;
3. the daemon validates every command against current revision, scope, authority, custody, and idempotency rules;
4. only the daemon mutates durable mission state or releases work.

The default automatic set is only context delivery and reversible guidance. Every automatic command must be on a daemon-owned allowlist and bind the exact Plan revision, WorkUnit, approved profile, Attempt/session generation, budget bound, and idempotency key. It may use only approved facts, must stay inside unchanged scope, acceptance criteria, and approach, and may cause no external effect. Anything else remains a recommendation for owner or Plan review. It cannot change Contract or Plan scope, dependencies, permissions, external effects, replacement, or Result Accept.

## Session and thread cardinality

```mermaid
flowchart TD
    O[One Outcome] --> C[One current frozen Contract revision]
    O --> P[One current approved Plan revision]
    P --> S[One Mission Supervisor thread per active Plan revision]
    P --> WA[WorkUnit A]
    P --> WB[WorkUnit B]
    WA --> AA[Attempt A1]
    WB --> BA[Attempt B1]
    AA --> AL[One exclusive worktree lease]
    AA --> AS[One Kennel Session]
    AS --> AT[One persistent primary Codex app-server thread]
    AT --> AT1[Turn 1]
    AT --> AT2[Turn 2]
    AT --> ATN[Turn N]
    BA --> BL[One exclusive worktree lease]
    BA --> BS[One Kennel Session]
    BS --> BT[One persistent primary Codex app-server thread]
```

For vNext, one Attempt equals one exclusive worktree lease, one Kennel Session, and one persistent primary Codex app-server thread with many turns. A Codex-native child agent is provider-internal activity under that Attempt. It is not a WorkUnit, scheduler, or independent authority.

A material Plan revision gets a fresh Supervisor thread seeded from canonical records. Small steering inside unchanged WorkUnit scope stays in the same Attempt and thread. Replacement is exceptional: explicit owner choice, material scope/authority change, or proven irrecoverability.

## Context and artifact lifecycle

```mermaid
flowchart LR
    CR[Contract rev] --> IM[Input manifest]
    PR[Plan rev] --> IM
    PS[Project snapshot] --> IM
    OD[Applicable owner decisions] --> IM
    UP[Verified upstream output manifest] --> IM
    IM -->|version + hash| A[Attempt / Codex thread]
    A --> OM[Output manifest]
    OM --> CK[Daemon checks]
    CK -->|pass| VO[Verified frozen output]
    CK -->|fail evidence| RW[Same-thread rework]
    RW --> OM
    VO -->|exact Plan edge| DIM[Dependent input manifest]
```

Before a session starts, the daemon compiles and hashes an input manifest containing the exact Contract and Plan revisions, WorkUnit goal and criteria, approved project snapshot, relevant facts and artifacts, applicable owner decisions, profile/worktree/effect limits, checks, and expected evidence. Codex receives a readable packet; the manifest remains canonical.

A worker publishes an output manifest: base and result revisions, changed files, API/schema/type changes, decisions, checks, retained artifacts, risks, and a concise handoff. Downstream workers receive the exact approved output version named by a Plan edge. They never inherit another worker's private transcript, hidden reasoning, scratch environment, or uncommitted worktree state.

Every context delivery records source, version, target, reason, content hash, and acknowledged or delivery-unknown state. Large repository context stays in the assigned worktree for native inspection. Packets select mission-specific context rather than copying the repository or repeatedly injecting the whole mission.

## Command and authority flow

```mermaid
sequenceDiagram
    actor U as Owner
    participant UI as UI / plugin
    participant S1 as S1 authority
    participant D as Daemon
    participant MS as Mission Supervisor
    participant W as Worker thread
    U->>UI: command / steer / answer
    UI->>S1: local authenticated request
    S1->>D: typed command + actor + revision + idempotency key
    D->>D: validate authority, scope, custody, current generation
    D-->>MS: typed mission event
    MS-->>D: recommendation or bounded steering command
    D->>D: validate Supervisor capability and current scope
    D->>W: authorized turn / steer / answer / interrupt
    W-->>D: normalized events, artifacts, attention, claim
    D-->>UI: one durable MissionProjection
```

Thin plugins, skills, and hooks may provide ingress, sensors, rendering, or provider adaptation. They may not become a scheduler, database, authority source, or second session engine. Native development commands are allowed inside the exclusive worktree. Network, deploy, merge, publish, payment, messaging, or other external effects require separately recorded authority.

## Attention, steering, stop, and replacement

- A finished turn is not WorkUnit completion.
- `needs_you` is nonterminal. The Attempt, worktree lease, Session, and Codex thread remain the same.
- An authenticated answer targets the exact question generation. A recorded answer is not considered delivered until acknowledgement or reconciliation proves it.
- Steering while a turn is active uses native steering when supported; steering while idle begins the next turn.
- Interrupt stops the current turn only.
- Cancel ends an Attempt only after an acknowledged or proven stop. An ambiguous provider effect retains custody and enters reconciliation; it is never blindly replayed.
- Time alone does not prove death. Atomic replacement requires explicit selection or proven irrecoverability.

## Completion, checks, and Accept

```mermaid
sequenceDiagram
    participant W as Worker thread
    participant D as Daemon
    participant MS as Mission Supervisor
    actor U as Owner
    W->>D: ready_for_verification + output manifest
    D->>D: freeze claim and run approved checks
    alt checks fail
        D->>MS: failed-check evidence
        MS->>D: bounded rework recommendation
        D->>W: same-thread rework turn
    else checks pass
        D->>D: verify WorkUnit and release ready dependents
        D->>MS: verified output event
    end
    D->>D: assemble Result when required WorkUnits verify
    D->>U: Result + evidence + one true next action
    U->>D: Accept / request rework / revise Plan
```

The Supervisor interprets evidence and recommends action. Deterministic checks decide their own pass/fail. The owner alone Accepts the Result.

## Restart and recovery

```mermaid
sequenceDiagram
    participant DB as Durable store
    participant D as Restarted daemon
    participant P as Codex app-server
    participant UI as Mission Control
    D->>DB: load Outcome, revisions, DAG, Attempts, leases, commands, cursors
    D->>P: thread/resume(stored primary thread ID)
    P-->>D: resumed thread state / normalized events
    D->>D: reconcile in-flight commands and provider cursor
    D->>DB: persist reconciled state before new effects
    D-->>UI: rebuild one MissionProjection
```

Durable state includes Outcome, Contract and Plan revisions, graph, approval, WorkUnit and Attempt state, Session and provider thread IDs, worktree lease/fence, command/question generation, provider event cursor, context and artifact hashes, check evidence, Result, and lineage. The app-server process may restart. The primary thread resumes by stored ID. Delivery-unknown commands reconcile before any retry. Kennel stores structured events and allowlisted evidence, not hidden model reasoning.

## Mission intelligence lifecycle

The Supervisor is persistent for the active Plan revision and event-driven. It wakes for completion claims, `needs_you`, failed checks, stalls or ambiguous state, artifact publication, drift, and owner steering. A compact periodic summary is triggered only by a daemon-owned durable cadence event while work is active. The Supervisor never self-schedules, polls, or creates timers.

It can:

- maintain a mission-level interpretation from canonical events;
- prepare bounded context packets and low-risk in-scope steering;
- summarize attention requests without erasing source provenance;
- diagnose failed checks and recommend same-thread rework;
- identify drift and propose an explicit Plan revision.

It cannot silently rewrite the graph or treat a recommendation as durable truth. Replanning is a new planning episode or Plan revision proposal. Existing verified artifacts retain lineage; new or materially changed WorkUnits wait for owner approval.

## Serial proof, then safe concurrency

The graph and worktree design support concurrency, but the first packaged proof releases one WorkUnit at a time. After custody, restart, context handoff, attention, and verification pass end to end, independent ready WorkUnits may run concurrently in separate worktrees. Integration is an explicit WorkUnit or reviewed merge step. No uncommitted filesystem state crosses workers.

## Budget policy

Budgets are targets, warnings, and Supervisor recommendation inputs. Kennel may recommend narrowing, steering, or replanning. There is no arbitrary aggressive default stop and no silent unlimited mode. A hard stop exists only for an explicit owner cap or a separately reviewed emergency policy, and it follows acknowledged-stop/reconciliation semantics.

## Compatibility boundary

This architecture applies only to new vNext execution records after cutover. Historical one-shot Attempts remain readable under their recorded execution model. They are never resumed into a persistent thread, rewritten, or dual-written. See [compatibility and migration](compatibility-and-migration.md).

## Rendered review companions

GitHub renders every Mermaid block above. Repository-authored SVG companions for offline and independent visual review are indexed at [architecture review diagrams](../assets/review/README.md). They are review views of this document, not a second architecture source.
