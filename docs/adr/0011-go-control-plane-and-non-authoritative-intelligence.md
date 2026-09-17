# ADR 0011 — Go control plane and non-authoritative intelligence adapters

> **Supersession note (2026-09-15):** deterministic daemon authority remains accepted. [ADR 0017](0017-persistent-mission-runtime-and-bounded-supervision.md) defines the coupled, typed Mission Supervisor intelligence layer and bounded automatic steering.


- **Status:** Accepted
- **Date:** 2026-09-08
- **Decision owners:** Waldo Kennel product/kernel
- **Reaffirms:** ADR 0003 local-first Waldo Core, ADR 0010 Outcome-first control plane

## Context

The Outcome-first reset raises two coupled questions:

1. should the local backend remain a Go daemon or be rewritten in Node/Python because many agent products use those ecosystems;
2. how should model intelligence participate in Contract analysis and Plan drafting without becoming hidden execution authority.

The current daemon's core responsibilities are process- and state-heavy: SQLite ownership, child-process/session supervision, PTYs, Git/worktrees, readiness, cancellation, event streaming, recovery/reconciliation, scheduling, and effect fencing. Those responsibilities are not improved merely by moving them to a language with a larger LLM framework ecosystem.

At the same time, Contract and Plan quality benefit from model intelligence. The current intake analyzer obtains that intelligence by spawning an ordinary coding-agent worker session and asking it to read the repository and call back. This proves the usefulness of a model-backed analysis seam, but it couples reasoning to the execution/session substrate too tightly.

## Decision

### 1. Keep the Go daemon as the canonical local control plane

The local Go daemon remains the sole canonical process for Work-state ownership and deterministic orchestration.

It owns:

- canonical SQLite writes and migrations;
- Outcome/Contract/Plan/WorkUnit/Attempt lifecycle transitions;
- approval and authority gates;
- provider/model routing admission;
- process/session supervision;
- worktree/workspace ownership;
- scheduling, cancellation, retries, and recovery;
- event/change projection;
- evidence/verification/acceptance policy.

No Node or Python rewrite is planned for the MVP.

### 2. Separate three logical planes

```text
┌─────────────────────────────────────────────────────┐
│                 INTELLIGENCE PLANE                  │
│                                                     │
│ Contract analysis · clarification · plan drafting   │
│ critique · recommendation · explanation             │
│                                                     │
│ Output: structured proposals + provenance           │
└──────────────────────┬──────────────────────────────┘
                       │ untrusted/non-authoritative data
                       ▼
┌─────────────────────────────────────────────────────┐
│                   CONTROL PLANE                     │
│                     Go daemon                       │
│                                                     │
│ validate · version · authorize · route · schedule   │
│ retry · fence · reconcile · verify · accept-gate    │
└──────────────────────┬──────────────────────────────┘
                       │ approved exact work
                       ▼
┌─────────────────────────────────────────────────────┐
│                  EXECUTION PLANE                    │
│                                                     │
│ Codex · Claude Code · OpenCode · Cursor · Pi · ... │
│ PTY/native protocol · worktree · git · browser      │
└─────────────────────────────────────────────────────┘
```

An implementation may host more than one logical plane in the same OS process, but the authority boundary must remain explicit in code and persistence.

### 3. Introduce a narrow intelligence adapter boundary

The control plane depends on an interface for structured reasoning rather than a provider SDK/framework directly.

The exact Go name may evolve, but the semantic contract is:

```go
type IntelligenceProvider interface {
    AnalyzeContract(context.Context, ContractAnalysisInput) (ContractAnalysisOutput, IntelligenceProvenance, error)
    DraftPlan(context.Context, PlanDraftInput) (PlanDraftOutput, IntelligenceProvenance, error)
}
```

The interface must not expose methods such as `StartAttempt`, `ApprovePlan`, `AcceptOutcome`, or unrestricted daemon mutation.

Inputs are bounded snapshots/digests of canonical data. Outputs are structured proposals that pass deterministic domain validation before being persisted as proposal/revision facts.

### 4. Multiple intelligence adapters are allowed

Kennel may support, behind the same port:

- an existing authenticated provider/harness adapter when it can satisfy the intelligence-run effect boundary;
- a direct model API adapter for reliable structured analysis/planning;
- a deterministic/offline/manual fallback.

The architecture does not require a Waldo-funded model key for execution. A direct API key, when configured, powers the intelligence plane only unless a later explicit ADR changes that.

No adapter may silently switch to a different provider/model because the configured one is unavailable. Fallback must be an explicit policy and visible in provenance. The offline/manual floor is acceptable because it changes quality, not authority.

### 5. Direct API use is preferred when the coding-agent runtime cannot enforce read-only reasoning

A coding-agent harness can be useful for repository-aware analysis, but a prompt saying "do not modify files" is not an enforcement boundary.

For pre-authorization Contract/Plan intelligence, the preferred order is:

1. use a provider/runtime mode with a proven read-only/effect-limited boundary;
2. otherwise provide bounded repository/context material to a direct model API adapter;
3. otherwise use the deterministic/manual proposal floor.

Do not grant ordinary execution/worktree authority merely to obtain a Contract or Plan proposal.

### 6. Intelligence runs have durable provenance

Every asynchronous/model-backed intelligence operation must be attributable as an `IntelligenceRun` or equivalent durable fact with at least:

- stable run ID;
- kind (`contract_analysis`, `plan_draft`, etc.);
- Intake/Outcome and source revision identity;
- requested provider/model semantics when known;
- effective provider/model semantics when known;
- input digest/snapshot version;
- output digest or proposal identity;
- status (`requested`, `running`, `fulfilled`, `failed`, `cancelled`, `expired`, or equivalent);
- timestamps;
- failure/refusal detail;
- optional provider-native process/session reference for inspection/reaping.

The provider-native process identity is provenance, not responsibility truth.

### 7. Secrets do not become domain data

Direct API credentials are operational secrets.

They must not be written as plaintext into canonical Outcome/Contract/Plan/SQLite domain rows, logs, prompts, or generated briefs.

The MVP should consume secrets through an existing secure local mechanism where available (OS credential store/secret abstraction) or process environment for development. Adding a persistent credential mechanism requires an explicit secure implementation rather than a convenience column in SQLite.

### 8. TypeScript remains the renderer/client language

Electron/React/TypeScript remains appropriate for the desktop UI and generated API client. Sharing a language between renderer and daemon is not sufficient reason to move canonical state/process supervision into Node.

### 9. Python remains optional for specialized intelligence/research, not kernel authority

Python may later host model evaluation, retrieval, research pipelines, or cloud intelligence services behind explicit ports. It does not own local canonical Work state or process authority in this decision.

## Why Go remains the right daemon choice

The dominant daemon workload is:

```text
long-lived local process
+ SQLite transaction/state ownership
+ concurrent subprocess supervision
+ PTY/stream handling
+ filesystem/Git/worktrees
+ cancellation/timeouts
+ scheduler/recovery loops
+ network/event streaming
+ provider adapters
```

Go is a strong fit for this workload and the repository already has meaningful, tested infrastructure in it. The current product problem comes from inherited authority/session semantics, not from Go.

A language rewrite would consume time, reset operational maturity, and still require the same domain redesign.

## Consequences

### Positive

- avoids an unrelated backend rewrite during the Outcome reset;
- keeps durable orchestration deterministic and model-independent;
- lets the team choose the best intelligence transport without changing domain authority;
- makes direct API keys optional and scoped;
- removes the need for an ordinary coding session to masquerade as Contract/Plan reasoning;
- makes model provenance inspectable and testable.

### Costs

- requires a new intelligence port and adapter(s);
- repository-aware direct API analysis needs a bounded context-gathering strategy;
- existing session-backed intake analysis must be refactored/reclassified;
- structured output schemas and validation need explicit versioning/tests;
- secret configuration needs disciplined handling.

## Rejected alternatives

### Rewrite the daemon in Node/TypeScript

Rejected for the MVP. It improves ecosystem proximity but not the core process/state problem, and would discard mature Go runtime/storage/recovery work.

### Rewrite the daemon in Python

Rejected for the MVP. Python is useful for model/research code but does not offer enough benefit for the local process-supervision kernel to justify the migration cost.

### Let provider agents call daemon control endpoints directly

Rejected. Intelligence output must return through a bounded proposal interface and deterministic validation. Provider agents do not gain general control-plane authority.

### Require one vendor API key for all Kennel operation

Rejected. Kennel execution should continue to support the user's authenticated local provider tooling. Direct inference is an optional intelligence adapter, not a new execution lock-in.

## Implementation gate for the first direct API adapter

Before introducing a direct API dependency in the MVP implementation session:

1. inspect whether the existing configured coordinator/harness can enforce a real read-only intelligence boundary;
2. if not, implement the provider-neutral `IntelligenceProvider` port first;
3. implement one direct API adapter behind it with structured output and bounded context;
4. request/configure the user's key only at that point;
5. retain deterministic/manual fallback so a missing key cannot create hidden provider fallback or corrupt authority semantics.
