# Kennel Builds Kennel — implementation plan

> **Superseded for vNext (2026-09-15):** Historical implementation plan; do not execute its phases as current order. Use [persistent mission runtime](../../architecture/persistent-mission-runtime.md) and the [persistent-session execution map](../../roadmap/persistent-session-execution-map.md). Retained for provenance.


> **Execution rule:** implement this program in separate verified branches/PRs. Do not execute all tasks as one patch. At the start of every slice, inspect current `beta` and update the delta map before editing.

**Goal:** build the first truthful self-hosting Waldo Kennel kernel: a user defines Outcomes, Kennel schedules/reconciles bounded WorkUnits and provider Attempts, receipts/evidence explain retained results, and only the user accepts closure.

**Architecture:** current Go daemon + SQLite remain canonical. Direct Outcomes gain WorkUnit DAGs under versioned Plans. A dependency scheduler admits WorkUnits into isolated WorkspaceLeases, provider drivers expose capability/readiness, and all UI/Island surfaces project daemon truth. Provider sessions remain replaceable Attempt-level executors.

**Stack:** Go daemon/domain/services/adapters, SQLite/sqlc/trigger CDC, OpenAPI, Electron/React, Git worktrees, provider-native structured protocols/ACP/RPC as adapters.

---

## Before any implementation session

Read:

1. `AGENTS.md`
2. `docs/product/kennel-v1-product-architecture.md`
3. ADR 0008 and ADR 0009
4. `docs/STATUS.md`
5. the relevant Work spec
6. the runtime reference index only for touched runtime/provider areas

Then:

```bash
npm run bootstrap
npm run lint
npm run frontend:typecheck
cd backend && go test -race ./...
```

Record any pre-existing failures. Do not weaken tests or architecture to make a long run complete.

## Phase A — make the domain capable of expressing the kernel

### Task A1: add Project Brief domain and persistence

**Inspect first:**

- `backend/internal/domain/project.go`
- existing Project service/port/store/controller paths
- latest SQLite migration number and Project schema/queries
- trigger/change-log conventions

**Create/update:**

- add a domain file for immutable `ProjectBriefRevision` (name/path should follow existing domain conventions);
- add the next additive SQLite migration + queries;
- expose service/port operations for create revision, get current, list history;
- add HTTP DTO/routes and generated contracts;
- add Project Brief tests at domain/store/service/controller boundaries.

**Required invariants:**

- revision identity is immutable;
- Project has at most one current revision projection, derived from revision facts;
- Brief has no runtime capability grants or Acceptance semantics;
- editing Brief does not silently mutate existing Outcome Contracts.

**Verification:**

```bash
npm run sqlc
npm run api
cd backend && go test ./internal/domain/... ./internal/storage/sqlite/... ./internal/service/... ./internal/httpd/...
```

Commit as one coherent Project Brief vertical slice.

### Task A2: widen PlanRevision to a bounded WorkUnit DAG

**Primary source:** `backend/internal/domain/outcome_plan.go`.

Replace the current exactly-one-WorkUnit invariant with:

- non-empty bounded WorkUnit set;
- stable WorkUnit IDs;
- explicit dependency IDs/edges;
- deterministic duplicate/unknown/self/cycle validation;
- stable ordering/canonical serialization for digests;
- WorkUnit-specific evidence/verification/stop/provider-capability requirements where appropriate;
- immutable Plan revision semantics.

Do **not** remove composed Outcomes. Preserve direct-vs-decomposed exclusivity.

Write failing tests first for:

1. two independent WorkUnits accepted;
2. dependency chain accepted;
3. diamond DAG accepted;
4. duplicate ID rejected;
5. unknown dependency rejected;
6. self dependency rejected;
7. cycle rejected;
8. decomposed parent cannot own Plan execution;
9. contributing child may own its own DAG.

Then update storage/service/API to persist/project the graph and regenerate generated contracts.

### Task A3: make RunBrief binding WorkUnit-specific

The current digest path was designed around one WorkUnit. Refactor so every Attempt receives a bounded RunBrief binding:

- exact Outcome/Contract/Plan revision;
- exact WorkUnit identity/objective;
- dependency outputs/receipts actually admitted;
- exact grants/authority/effect limits;
- workspace/base revision facts;
- evidence/verification expectations;
- known omissions/provenance.

A material input change creates a new Plan/Attempt boundary as defined by the architecture; never silently mutate the approved digest.

## Phase B — make the graph executable and recoverable

### Task B1: introduce WorkspaceLease semantics

Before editing, map the current worktree/session/runtime creation and cleanup call chain. Reuse existing runtime/worktree primitives rather than creating a parallel workspace subsystem.

Implement a durable lease/custody model binding workspace to Project/Outcome/WorkUnit/Attempt and environment/setup revision.

Provisioning must be a staged state machine:

```text
inspect
→ resolve base
→ allocate path
→ create worktree/branch
→ install Kennel hooks/config
→ run setup
→ verify
→ ready
```

Each failure records its stage. Rollback only resources definitely created by this provisioning Attempt.

Write tests for transient Git lock handling, invalid target, setup failure, verify failure, dirty debris, and idempotent release.

### Task B2: implement dependency/runnable calculation

Use deterministic graph state, not model advice.

Conceptual algorithm:

```text
for each WorkUnit in authorized Plan:
  if already terminal/retained -> skip
  if any required dependency not satisfied -> waiting
  else if authority/capability/readiness absent -> blocked/needs-user
  else if prior Attempt ambiguous -> unconfirmed
  else if workspace/concurrency/effect gate unavailable -> waiting
  else -> runnable candidate
```

Scheduling order among simultaneously runnable candidates may use explicit priority/policy, but must never violate dependencies/authority or hardcode provider-brand folklore.

### Task B3: replace Project-wide Attempt fence

Do not simply delete the old fence. Introduce narrower boundaries first:

- WorkspaceLease for repository writes;
- integration/merge lease for shared target integration;
- external Effect fence/idempotency for consequential actions;
- concurrency budget.

Then remove Project-wide serialization only after concurrent-work tests demonstrate no cross-worktree corruption and recovery remains fail-closed.

### Task B4: restart/reconciliation tests

Add adversarial tests:

- renderer dies, daemon/provider continue;
- daemon dies, provider survives;
- provider dies, daemon survives;
- daemon restarts while workspace exists;
- provider identity is ambiguous;
- external effect outcome is unknown;
- cleanup partially failed.

Expected rule: `unknown` / `unconfirmed` is preserved until reconciled. Never infer completion from silence.

## Phase C — make provider capability truthful

### Task C1: define/centralize ProviderCapabilities

Inspect current registry/manifests from PR #92. Add/extend a canonical capability projection rather than scattering provider-name checks.

Capabilities should be granular enough to admit only implemented behavior: worker start, structured session control, resume/reconcile, steer/send, interrupt/cancel, approvals, event subscription, child identity, review, switch/handoff, external hooks/MCP, etc.

### Task C2: provider conformance suites

For Codex, Claude Code, OpenCode, Cursor, Pi:

- probe installed/auth/config readiness without consuming user work unnecessarily;
- start a bounded test session in a test workspace when integration tests permit;
- establish native session identity;
- verify structured events used by receipts;
- test resume/reconcile/interrupt/steer only where claimed;
- verify unsupported capability explains itself and remains unadmitted.

Pin/negotiate protocol versions. Treat unknown events forward-compatibly; do not make ACP the canonical Kennel object model.

No role becomes selectable because a provider brand is “known.” It becomes selectable because the required capability set is ready and conformance-proven.

## Phase D — make execution understandable without transcripts

### Task D1: SessionReceipt

Define a provenance-bearing receipt from structured provider events + workspace/runtime facts. Minimum semantics:

- Attempt/session identity;
- provider/version/model/profile when known;
- start/end/reconciliation state;
- retained file/artifact/change summary;
- checks/tests/tool outcomes used as evidence candidates;
- provider-native child summary when exposed;
- usage/cost only when trustworthy;
- failures/interruptions/unknowns;
- source/event version provenance.

Do not store an LLM summary as the sole truth. A model may summarize structured facts; facts remain independently inspectable.

### Task D2: WorkUnitReceipt

Aggregate retained Attempts for one WorkUnit:

- which Attempt result was retained;
- failed/superseded/recovery lineage;
- dependency inputs consumed;
- artifacts and checks;
- unresolved contradictions;
- evidence candidates.

### Task D3: Outcome current brief

Compile a concise projection from current Contract/Plan/WorkUnits/receipts/Evidence/Verification/Decisions:

- what is true;
- what is running/waiting/blocked;
- what failed and whether it recovered;
- what needs the user;
- what remains unproved;
- inspect links/provenance.

The brief should survive twenty sessions without replaying twenty transcripts.

## Phase E — make the product surface canonical

### Task E1: Project Brief + Board/List

Frontend reads daemon projections; it does not maintain an alternative lifecycle.

Implement:

- Project Brief view/edit/history;
- active Board columns `Needs You | In Progress | Waiting | Ready for Review`;
- List over the same top-level Outcome set;
- accepted/archive history outside active Board;
- composed-parent attention roll-up;
- Plan progress separate from proof coverage;
- remove donor `OutcomeTask`/`completed` semantics.

### Task E2: Mission Control `Contract | Graph`

Only after Phase B concurrency is real.

Graph rendering rules:

- direct Outcome: WorkUnit DAG;
- decomposed Outcome: contribution layer + selected child DAG;
- WorkUnit card shows state + subordinate current provider/Attempt summary;
- expand for Attempt history and click through to Session Inspector;
- no fake percentages or fake parallel animation.

### Task E3: consequence-first Island

Project the same attention state into Island. Test that closing the Island/window does not alter daemon execution.

## Phase F — external ingress

Define one versioned agent-facing Kennel control surface. Provider-specific integrations are thin shims.

Potential tool families:

```text
kennel.project.current
kennel.outcome.list / inspect
kennel.work.inspect / propose / spawn / wait / send / cancel
kennel.evidence.add
kennel.activity.bind / observe
kennel.context.propose_learning
```

Every mutation goes through daemon validation. No provider plugin writes SQLite directly.

External sessions are explicitly Governed, Observed, or Untracked. Test that a plausible related Observed session never silently acquires Outcome lineage.

## Phase G — self-hosting experiment

Choose one real Kennel issue with objective criteria. Run the entire flow in Kennel itself.

Required adversarial events:

- at least two independent WorkUnits;
- at least one dependency edge;
- at least one retry/recovery or intentional provider restart;
- inspect proof separate from Plan completion;
- explicit user Acceptance only at the end.

Record active supervision time and whether raw provider transcripts were necessary. Repeat across enough real work to evaluate the acceptance matrix rather than drawing conclusions from one demo.

## Final verification before each merge

Use the repo commands from `AGENTS.md`, plus task-specific integration tests. Before claiming a slice complete:

- fetch/inspect the final diff;
- confirm generated contracts are committed when relevant;
- confirm no old one-WorkUnit/project-fence assumption was reintroduced into active docs;
- confirm current `STATUS.md` is updated if shipped reality changed;
- confirm the implementation satisfies a named dogfood acceptance case.
