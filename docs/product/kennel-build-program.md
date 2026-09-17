# Kennel builds Kennel — kernel implementation program

> **Classification (2026-09-15):** Historical build program; its execution order is superseded by the persistent-session execution map. Use the [canonical documentation map](../README.md).


This program orders implementation of the v1 kernel. The public
[`ROADMAP.md`](../../ROADMAP.md) places that kernel work inside launch and
post-launch product milestones. The roadmap does not change the ontology or
authorize implementation; [`../STATUS.md`](../STATUS.md) remains the record of
what is integrated on `beta`.

- **Status:** Active implementation program
- **Date:** 2026-09-04
- **Baseline:** `beta` after merged provider-core PR #92
- **Goal:** reach a truthful self-hosting local Kennel kernel that can implement real Kennel work while the user supervises Outcomes rather than provider transcripts
- **Architecture authority:** `kennel-v1-product-architecture.md`, ADR 0008, ADR 0009

This program replaces the earlier broad “12-week v1”/first-Outcome delivery sequencing for the Work kernel. Home, personal Memory, capture, mobile, and learned automation remain separately governed future/parallel lanes and do not block this program.

## Program rule

Do not optimize for feature count or a polished graph before the runtime can support it.

Each slice must:

1. start from current `beta` (or explicitly documented dependent branch);
2. read the canonical architecture + relevant ADR + `STATUS.md`;
3. produce a concrete current-code delta map;
4. run baseline tests for the touched subsystem;
5. implement backend/domain truth before frontend projection;
6. add/extend narrow conformance/invariant tests;
7. regenerate sqlc/OpenAPI/frontend contracts when source contracts change;
8. run touched-area and repo-wide gates;
9. commit a coherent checkpoint;
10. stop instead of weakening an invariant to “finish the slice.”

No single agent should attempt the entire program as one patch.

## Slice 0 — provider-core convergence — DONE

Merged PR #92 established:

- exactly five active first-class providers: Codex, Claude Code, OpenCode, Cursor, Pi;
- machine-aware readiness;
- dynamic provider-selection surfaces;
- no hidden Codex fallback as product policy;
- removal of the broad donor provider registry from active new-work paths;
- preserved historical provider compatibility where required.

Remaining provider role depth belongs to Slice 4.

## Slice 1 — architecture/docs gate — THIS PR

### Objective

Make the repository teach one product/kernel architecture before runtime migration starts.

### Deliverables

- canonical `AGENTS.md` read order and hard rules;
- concise canonical v1 Work/kernel architecture;
- ADR 0008: responsibility composition vs WorkUnit execution DAG;
- ADR 0009: scheduler, WorkspaceLease, effect fencing;
- factual `STATUS.md`;
- aligned control-plane and interaction specs;
- self-hosting acceptance matrix;
- runtime/provider reference index;
- removal of stale handoffs/prototypes/delivery plans that directly contradict the new authority.

### Exit gate

A new coding session can read the authority chain without encountering a current instruction that says `PlanRevision` must remain one WorkUnit or that dependencies belong only between contributing Outcomes.

## Slice 2 — domain + persistence: Project Brief and WorkUnit DAG

### Objective

Make the canonical objects real before building concurrency/UI on top.

### Domain work

- add immutable/versioned `ProjectBriefRevision`;
- widen `PlanRevision` from exactly one `direct` WorkUnit to a bounded ordered WorkUnit set + dependency edges;
- give each WorkUnit stable identity and requirements needed by the scheduler/evidence path;
- deterministic validation for duplicate IDs, unknown/self dependencies, cycles, capability requirements, and invalid graph shape;
- preserve direct-vs-decomposed Outcome exclusivity;
- preserve ADR 0007 contribution/authority/proof semantics;
- update RunBrief core digest so the exact authorized WorkUnit and relevant dependency inputs are frozen independently;
- define Plan/Contract staleness for multiple WorkUnits without rewriting historical Attempts/Evidence.

### Persistence/API work

- add the next additive SQLite migration after inspecting the current ledger; never edit merged migrations;
- add queries/store/service methods for Project Brief revisions, WorkUnits, dependency edges, and current Plan projection;
- maintain trigger-backed CDC;
- expose typed daemon DTO/API operations;
- regenerate sqlc, OpenAPI, and frontend schema.

### Tests

- Project Brief revision immutability/current projection;
- direct Plan with independent branches;
- duplicate/unknown/self/cycle rejection;
- stable graph ordering/digest behavior;
- Contract revision stales affected Plan but does not invent completion/death;
- decomposed parent still cannot own direct Plan execution;
- contributing child can own its own WorkUnit DAG.

### Exit gate

The daemon can persist/read/authorize a direct Outcome with at least four WorkUnits and two independent branches, while all existing Outcome/Acceptance invariants still pass.

## Slice 3 — WorkspaceLease + dependency scheduler

### Objective

Turn the DAG into truthful execution rather than a stored diagram.

### Runtime work

- introduce durable/inspectable WorkspaceLease state;
- implement staged repository/worktree provisioning with stage-specific failure;
- replace the Project-wide execution fence with narrower write/integration/effect boundaries;
- add dependency-aware runnable calculation and concurrency budget;
- create Attempts only after admission gates pass;
- track workspace/process/preview/cleanup custody;
- reconcile after daemon/provider/renderer failure;
- preserve `unknown`/`unconfirmed` rather than guessing;
- implement conservative cleanup/debris inspection.

### Git/non-Git rule

- Git-backed Projects can run independent write WorkUnits in isolated worktrees;
- new Projects may initialize Git before parallel writes are enabled;
- non-Git folders remain useful for context/research/single-writer work and show the limitation truthfully.

### Tests

- two independent write WorkUnits run concurrently in separate worktrees;
- downstream node waits for dependencies;
- transient Git lock retry is bounded/safe;
- dirty/unknown worktree is not force-deleted;
- daemon restart does not duplicate Attempt;
- surviving provider session can be reconciled when identity is trustworthy;
- unknown effect/Attempt blocks unsafe retry;
- shared integration boundary serializes only integration, not all Project work.

### Exit gate

A direct Outcome with two independent write branches genuinely runs concurrently and can survive deliberate process failure without corruption or duplicate Attempts.

## Slice 4 — structured provider drivers and capability-derived role admission

### Objective

Normalize control at the capability boundary without flattening all providers to the weakest interface.

### Provider targets

- **Codex:** prefer structured app-server control; pin/test the exact protocol surface used.
- **Claude Code:** let the user's Claude installation own authentication; harden structured/headless/session/hook behavior without turning Kennel into a subscription credential broker.
- **OpenCode:** use server/SDK/ACP/plugin surfaces where they provide stronger structured state.
- **Cursor:** add/finish ACP driver conformance before enabling coordinator/switch/review roles.
- **Pi:** add/finish RPC/SDK structured driver conformance; Kennel owns sandbox/workspace/effect containment.

### Capability model

Conformance should cover only features Kennel actually admits, such as start, stable identity, resume/reconcile, steer/send, interrupt/cancel, approvals, events, child identity, usage/structured output, and external ingress hooks.

Do not hardcode permanent role membership by provider name. Registry/provider manifests expose capability; policy admits a role only after tests prove it.

### Exit gate

A single-provider user can run the self-hosting DAG with any supported provider whose required role capabilities are proven, and an explicit unavailable capability fails with a truthful explanation rather than fallback.

## Slice 5 — SessionReceipt, WorkUnitReceipt, artifacts, evidence, Outcome brief

### Objective

Make transcript reconstruction unnecessary for routine supervision.

### Work

- normalize structured provider events + workspace facts into provenance-bearing `SessionReceipt`;
- aggregate retained Attempt result(s) into `WorkUnitReceipt`;
- record artifacts and checks with exact Attempt/WorkUnit lineage;
- propose/bind Evidence to current criterion identity;
- preserve contradictions and stale-subject proof;
- produce concise Outcome current brief/ledger from canonical state + receipts;
- create Project Context learning candidates with provenance, without automatic promotion.

### Exit gate

For a five-WorkUnit Outcome with retries, Waldo can explain what changed, what was retained, what failed/recovered, and what remains to prove without the user opening provider transcripts.

## Slice 6 — canonical Board/List + Project Brief UI + donor overlay removal

### Objective

Make the default product surface match canonical responsibility objects.

### Work

- Board/List project top-level active Outcomes from daemon facts;
- global Work and Project-filtered Work use the same Outcome store;
- Project Brief view/edit/revision history;
- no special Primary Outcome card;
- remove donor `OutcomeTask` / `completed` overlay and marker-derived lifecycle leftovers;
- sidebar shows Projects + a small attention/pinned Outcome shortlist, not a recursive session tree;
- cards show Plan progress and proof separately;
- provider/session indicators remain subordinate.

### Exit gate

A user can create/update Project Brief, create several independent Outcomes, and supervise their attention state with no donor task lifecycle or PrimaryOutcome semantics.

## Slice 7 — Mission Control `Contract | Graph`

### Objective

Expose one Outcome's governing Contract and real execution topology.

### Work

- Contract projection: desired state, criteria/proof, constraints/non-goals, authority, pause triggers, current Plan or contributing Outcomes;
- Graph projection backed only by daemon topology/scheduler state;
- direct Outcome opens on WorkUnit DAG;
- decomposed Outcome shows contributing layer then child DAG;
- WorkUnit node expands Attempts/Sessions only on demand;
- progress ring is Plan structure/progress, not guessed agent completion;
- Session Inspector remains the deep technical escape hatch.

### Exit gate

Graph exactly matches scheduler truth during a real parallel dogfood Outcome, including failure/retry without creating fake Outcome nodes.

## Slice 8 — external provider ingress

### Objective

Let provider sessions cooperate with Kennel without inventing lineage.

### Work

- define a versioned execution/activity envelope;
- one daemon-owned agent-facing control contract (MCP-like where suitable);
- thin provider shims/plugins/hooks;
- explicit `Governed | Observed | Untracked` state;
- explicit binding to Project/Outcome/WorkUnit/Attempt where authorized;
- no fuzzy auto-attachment;
- observed research can propose attach/new Outcome/new WorkUnit/Project learning candidate.

### Exit gate

An external integrated session can be observed and explicitly attached/promoted without provider/plugin code writing canonical storage directly.

## Slice 9 — Waldo Island consequence projection

### Objective

Make ambient supervision Outcome-first.

### Work

- project existing daemon attention/receipt/recovery projections into Island;
- collapsed state prioritizes “needs you” and active Outcome count;
- expanded state names consequence and smallest user decision;
- deep links open the relevant Outcome/WorkUnit/Session;
- no Island-specific execution database.

### Exit gate

Closing/reopening the renderer/Island does not change Attempt truth, and Island notifications describe responsibility consequences rather than raw provider events.

## Slice 10 — Kennel builds Kennel dogfood

### Objective

Use the kernel to implement a real Kennel change end to end.

Recommended first self-hosting candidate: remove/finish removal of the donor `OutcomeTask` overlay or another real beta issue whose success can be criterion-tested.

Required flow:

```text
Project Brief
→ define Outcome
→ Contract
→ Plan with real WorkUnit DAG
→ user authorization
→ parallel/scheduled Attempts
→ deliberate failure/restart exercise
→ receipts + artifacts
→ Evidence + Verification
→ Ready for Review
→ explicit user Acceptance
```

The user should primarily operate Board/Mission Control and open Session Inspector only for exceptional debugging.

### Exit gate

Pass the acceptance matrix in `kennel-dogfood-acceptance-matrix.md` over repeated real work. If supervision cost does not improve, treat that as product evidence rather than adding ontology to hide it.

## Contribution/release gate after dogfood

Do not market the kernel as self-building or invite broad architecture contributions until:

- the dogfood invariants have run repeatedly;
- verification commands are reproducible;
- `beta` merge quality is protected by an agreed CI/review policy;
- contributor docs point to this authority chain;
- provider/version gaps are labeled;
- failure/recovery debris can be inspected rather than silently discarded.

The product earns expansion by reducing supervision and preserving truth, not by supporting the largest number of providers or graph nodes.
