# Kennel technical chassis architecture

> **Classification (2026-09-15):** Runtime chassis reference. Legacy AO/session topology is current-code history, not vNext product architecture. Use the [canonical documentation map](README.md).


- **Status:** Current technical reference for the daemon/runtime chassis and Outcome-control-plane boundary
- **Updated:** 2026-09-08
- **Product/kernel authority:** [`product/kennel-v1-product-architecture.md`](product/kennel-v1-product-architecture.md)
- **Current MVP amendment:** [`product/2026-09-08-outcome-control-plane-mvp-reset.md`](product/2026-09-08-outcome-control-plane-mvp-reset.md)
- **Authority reset ADRs:** [ADR 0010](adr/0010-outcome-first-control-plane-and-session-subordination.md), [ADR 0011](adr/0011-go-control-plane-and-non-authoritative-intelligence.md)
- **Implemented reality:** [`STATUS.md`](STATUS.md)

This document describes Kennel's technical chassis: daemon boundaries, ports/adapters, persistence/CDC, provider/runtime lifecycle, terminal/browser supervision, recovery foundations, and the boundary between model intelligence and deterministic orchestration.

It is **not** a license to promote legacy session/task machinery into product authority. Outcome/Contract/Plan/WorkUnit semantics and approval policy live in the canonical product architecture and accepted ADRs.

Historical AO-derived names may still exist in source comments or compatibility seams. Kennel is a standalone product. Do not restore AO product identity or its session-first authority model from old history.

## 1. System boundary

Kennel is local-first. The Go daemon is the canonical local control process and SQLite is the canonical Work-state writer.

```text
Electron / CLI / Island
          │
          ▼
  loopback daemon API
          │
  services + domain policy
          │
 ports ─ adapters/runtime
          │
 SQLite + change_log
```

The renderer is a client/projection. Closing it does not define whether an Outcome, Attempt, IntelligenceRun, or provider process exists.

## 2. Three logical planes

ADR 0011 makes the authority boundary explicit:

```text
┌─────────────────────────────────────────────────────┐
│                 INTELLIGENCE PLANE                  │
│ Contract analysis · clarification · plan drafting   │
│ critique · recommendation · explanation             │
└──────────────────────┬──────────────────────────────┘
                       │ structured proposals
                       ▼
┌─────────────────────────────────────────────────────┐
│                   CONTROL PLANE                     │
│                     Go daemon                       │
│ validate · version · authorize · route · schedule   │
│ retry · fence · reconcile · verify · accept-gate    │
└──────────────────────┬──────────────────────────────┘
                       │ approved exact work
                       ▼
┌─────────────────────────────────────────────────────┐
│                  EXECUTION PLANE                    │
│ Codex · Claude Code · OpenCode · Cursor · Pi · ... │
│ PTY/native protocol · worktree · git · browser      │
└─────────────────────────────────────────────────────┘
```

The actual orchestrator is Kennel's deterministic control plane. A coordinator/model may analyze and propose; it does not own lifecycle, approval, routing authority, retries, effect fencing, verification admission, or acceptance.

## 3. Canonical responsibility hierarchy

For new work:

```text
Outcome
→ ContractRevision
→ PlanRevision
→ WorkUnit
→ Attempt
→ AgentSessionRef
→ EvidenceItem
→ VerificationRun
→ AcceptanceDecision
```

An execution provider session is subordinate to an Attempt.

Pre-execution model work is represented separately:

```text
IntelligenceRun
  kind = contract_analysis | plan_draft | ...
  ↓ structured proposal
ContractRevision / PlanRevision
  ↓ owner confirmation / approval
Attempt
```

`IntelligenceRun` is not an Attempt and any provider-native process it uses is provenance rather than responsibility truth.

## 4. Repository layout

```text
backend/
  cmd/kennel/                  CLI/daemon entrypoint
  internal/domain/             canonical vocabulary/invariants
  internal/ports/              adapter/runtime/storage contracts
  internal/service/            application/control-plane services
  internal/httpd/              REST/SSE/terminal endpoints + spec source
  internal/storage/sqlite/     migrations, queries, stores, trigger CDC
  internal/adapters/           provider/runtime/workspace/SCM adapters
  internal/lifecycle/          reconciliation/recovery logic
  internal/daemon/             production wiring

frontend/                      Electron + React client/supervisor
packages/kennel-island/        ambient projection inside desktop architecture
docs/                          architecture/ADRs/specs/programs
```

Inspect the current tree before relying on a path not listed here.

## 5. Core technical principles

### 5.1 Ports/adapters

Domain/control-plane services depend on narrow ports rather than provider SDK/runtime implementations. Provider capabilities and model catalogs belong at adapter/manifest boundaries and are admitted through deterministic policy.

The same rule applies to model intelligence: `IntelligenceProvider`-style adapters return proposals; they do not receive general control-plane mutation authority.

### 5.2 Durable facts, derived projections

Persist canonical facts and derive UI/attention state from them. Do not persist a second display lifecycle merely because a frontend needs `planning`, `working`, `waiting`, or `needs_you`.

Outcome/Contract/Plan/WorkUnit/Attempt/Evidence/Verification/Acceptance plus Intake/IntelligenceRun facts drive Work/Mission/Island projections.

### 5.3 Trigger-backed CDC

SQLite mutations flow through database triggers into `change_log`; the daemon tails/broadcasts changes to clients. Do not add a second manual event authority from stores without an explicit architecture decision.

```text
SQLite mutation
   ↓ trigger
change_log
   ↓ poll/broadcast
SSE / clients
```

### 5.4 Thin clients

CLI, frontend, Island, and provider integrations call daemon APIs. They do not open canonical SQLite directly or recreate authority/routing policy locally.

### 5.5 Proposal is not authority

Model output is data. Contract proposals, Plan proposals, routing recommendations, verifier commentary, and provider completion all pass through deterministic validation and explicit authority boundaries.

## 6. Outcome-first product boundary

The normal new-work path is:

```text
capture intent
→ understand/analyze
→ Contract review + owner confirmation
→ Plan draft/review
→ owner approval
→ Mission Control
→ Attempts/sessions
→ evidence/verification
→ owner acceptance
```

Creating an intake or confirming a Contract must not create an execution Attempt/session. Plan approval is the execution authority boundary.

Selecting an Outcome opens its Outcome workspace. Terminal/native chat is a deliberate Attempt/session drill-down, not the canonical Outcome destination.

## 7. Daemon/network boundary

The primary daemon listener remains loopback-only (`127.0.0.1`) under the repository's existing security decision. The separately governed opt-in LAN listener remains subject to ADR 0001 and its authentication/control-route restrictions.

Do not broaden the network surface as part of unrelated kernel work.

## 8. Persistence and generated contracts

SQLite schema changes are additive. Never edit already-merged migrations.

When changing storage:

1. add the next migration;
2. update source queries/schema;
3. run `npm run sqlc`;
4. keep trigger/change-log behavior intact.

When changing daemon API DTO/routes:

1. edit controller/spec source;
2. run `npm run api`;
3. commit generated OpenAPI + frontend schema together;
4. run route/spec parity tests.

Generated files are outputs, not editing surfaces.

Operational secrets such as direct intelligence API keys are not canonical domain data and must not be stored in plaintext Outcome/Contract/Plan rows, logs, or prompts.

## 9. Existing execution chassis

The repository already supports Project/session lifecycle, provider adapters, auth/readiness, terminal/native chat modes, Git worktrees, browser preview, PR/check/review observation, process/reaper foundations, and recovery facts.

Those are retained as execution infrastructure.

What is being removed is the legacy authority implication that a Project worker/orchestrator session is the user's responsibility itself.

## 10. Provider boundary

The active first-class execution provider surface remains:

- Codex
- Claude Code
- OpenCode
- Cursor
- Pi

Readiness and role admission are capability-derived and machine-aware. Historical provider identities may remain readable for compatibility/recovery but are not active new-work providers.

Native structured protocols/ACP/RPC/CLI are adapter concerns. None becomes the Kennel domain model.

Project coordinator/worker provider choices are preferences/baselines. They do not directly authorize execution.

See [`research/2026-09-04-kernel-runtime-reference-index.md`](research/2026-09-04-kernel-runtime-reference-index.md).

## 11. Intelligence adapter boundary

Contract/Plan intelligence may be backed by:

- a provider runtime with a proven read-only/effect-limited mode;
- a direct model API supplied with bounded repository/context material;
- deterministic/manual fallback.

Do not treat a prompt saying “do not modify files” as an enforcement boundary.

No intelligence adapter may silently switch provider/model. Direct API credentials, if used, power the intelligence plane only unless a later ADR explicitly widens their role.

## 12. Routing and exact execution binding

Preference-aware routing belongs during Plan formation/authorization, not task/session creation.

Required invariants include:

- no implicit Codex;
- provider/model preference semantics are explicit;
- unknown readiness/capability/model support cannot satisfy hard requirements;
- router is provider-neutral, deterministic, and explainable;
- no valid candidate becomes Action Required and starts nothing;
- approved WorkUnit freezes exact provider/model binding semantics;
- Attempt uses that binding and does not re-read mutable Project preference to choose execution.

## 13. Workspace/runtime evolution boundary

The existing session/worktree machinery remains the current substrate. ADR 0008/0009 still target WorkUnit DAG scheduling and explicit `WorkspaceLease` ownership.

The first Outcome-control-plane MVP may serialize approved WorkUnits rather than blocking the vertical product loop on full concurrency.

Until full scheduling lands:

- do not claim WorkUnit concurrency the runtime cannot provide;
- preserve existing recovery safety while replacing fences in verified slices;
- never infer provider death/completion from a failed or missing probe;
- never force-delete dirty/unknown worktrees.

## 14. Session interface continuity

Where a provider has proven compatible native identities/protocol behavior, Kennel may support same-provider controller/interface transition or resume without changing higher-level responsibility identity.

Across providers, canonical behavior remains a new Attempt plus attributed handoff/continuation unless exact provider-native transition is proven. Do not describe cross-provider handoff as lossless hidden-state migration.

## 15. Observation and reconciliation

External/process/provider observations are inputs to canonical state, not truth by themselves.

Reconciliation distinguishes:

- observed alive/active;
- confirmed terminal;
- interrupted/failed;
- unknown/unconfirmed.

On restart, load non-terminal canonical facts, probe the strongest available provider/workspace/runtime identity, reconcile, fence duplicates/effects, and only then create recovery work when policy permits.

The same principle applies to non-terminal IntelligenceRuns: recover/expire/reconcile the durable run rather than assuming a provider process completed because the UI disappeared.

## 16. Browser/terminal/PR surfaces

Terminal, native chat, browser/preview, diff, branch/commit, PR/check/review observation, and provider-native child details remain valuable deep execution surfaces. They belong primarily under WorkUnit/Attempt/Session Inspector context, not as the default Work responsibility view.

## 17. Current architectural transition

The transition is intentionally a domain/control-flow rewrite on top of the existing chassis, not a backend-language rewrite:

```text
existing Go daemon + SQLite + provider/runtime chassis
                    ↓
Outcome-first navigation + remove session bypasses
                    ↓
IntelligenceRun → Contract → Plan
                    ↓
preference-aware routing + exact binding
                    ↓
Plan approval → serial truthful Mission Control MVP
                    ↓
Evidence → Verification → owner Acceptance
                    ↓
full WorkUnit DAG + WorkspaceLease scheduler
                    ↓
receipts / richer continuity / external ingress
```

For current sequencing use:

- [`product/2026-09-08-outcome-control-plane-mvp-reset.md`](product/2026-09-08-outcome-control-plane-mvp-reset.md)
- [`superpowers/plans/2026-09-08-outcome-control-plane-mvp-reset.md`](superpowers/plans/2026-09-08-outcome-control-plane-mvp-reset.md)
- [`superpowers/plans/2026-09-07-wt3-preference-aware-routing.md`](superpowers/plans/2026-09-07-wt3-preference-aware-routing.md)
- [`STATUS.md`](STATUS.md)

## 18. Non-negotiable technical invariants

- daemon/SQLite remain canonical; frontend/plugins/models do not become competing writers;
- Kennel's deterministic control plane is the orchestrator; models propose or execute bounded authorized work;
- Outcome creation/Contract confirmation do not launch execution Attempts;
- Plan approval is the execution authority boundary;
- deterministic dependency/authority/idempotency/effect/recovery checks remain deterministic;
- session/provider completion does not accept an Outcome;
- failed/unknown probes do not prove death;
- retries/recovery create traceable Attempt lineage rather than rewriting history;
- approved exact provider/model binding is not silently rerouted from mutable Project preference;
- dirty/unknown workspaces are preserved for inspection;
- migrations are additive;
- generated contracts are regenerated from source;
- UI concurrency must not outrun scheduler truth;
- API secrets are not persisted as canonical Work data.

This file should remain a concise chassis reference. Deep AO history is available in Git history if needed; it should not be loaded by default into future kernel coding sessions.
