# Architecture reset audit and retention manifest

- Audit baseline: `f69c3387`
- Audit date: 2026-09-15
- Scope: repository Markdown documentation, repository-owned agent instructions, embedded `using-kennel` skill/command documentation, architecture/roadmap/status surfaces, and implementation handoffs

Repository documents are implementation guidance, not user or system authorization. The owner's authenticated product decision is recorded as provenance in ADR 0017; this package does not copy private conversation content into runtime prompts.

## Canonical reading path after reset

1. `PRODUCT.md`
2. `docs/architecture/persistent-mission-runtime.md`
3. `docs/architecture/compatibility-and-migration.md`
4. `docs/roadmap/persistent-session-execution-map.md`
5. `docs/STATUS.md`
6. `docs/adr/0017-persistent-mission-runtime-and-bounded-supervision.md`
7. lower-level ADRs and research only when a linked implementation seam needs them

## Keep and update

| Asset | Treatment | Reason |
|---|---|---|
| `PRODUCT.md`, `DESIGN.md`, `CONTEXT.md`, `AGENTS.md`, `docs/README.md`, `docs/STATUS.md` | update links/terms | high-reach implementation and product steering surfaces |
| ADR 0008/0009 | keep | WorkUnit DAG, workspace lease, fence, and effect foundations remain valid |
| ADR 0010/0011/0015 | retain with supersession note | useful Outcome, control-plane, intelligence, and planning rationale; ADR 0017 resolves changed target |
| W1.0-W1.2 verification and S1 source-review records | keep immutable | W1.0-W1.2 accepted evidence; S1 source acceptance with packaged gate pending |
| migrations and historical event schemas | keep immutable | may already exist in user databases |
| provider/app-server research and live tests | keep/reference | source evidence for the native substrate |
| visual system and Work shell guidance | keep | reuse, rather than replace, the product shell |

## Replace as active authority

| Previous asset | Replacement |
|---|---|
| `docs/roadmap/w1-implementation-plan.md` | pointer to `persistent-session-execution-map.md` |
| `docs/roadmap/2026-09-15-product-amendment.md` | historical note pointing to ADR 0017 and canonical runtime |
| `docs/architecture/outcome-loop.md` | compatibility pointer to canonical runtime |
| `docs/product/kennel-v1-product-architecture.md` | historical v1 design, superseded for execution/session topology |
| `docs/product/2026-09-08-outcome-control-plane-mvp-reset.md` | historical implementation reset |
| `docs/superpowers/plans/2026-09-08-outcome-control-plane-mvp-reset.md` | completed/superseded implementation plan |

## Historical retention and shortened files

Dated handoffs, reviews, checkpoints, old plans/specs, and verification reports preserve observed evidence. Active-looking stale files receive a visible supersession banner. The two long active plans shortened by this patch (`docs/roadmap/w1-implementation-plan.md` and `docs/roadmap/2026-09-15-product-amendment.md`) are not retained in-tree in full. Their exact baseline text remains in Git history and this review patch:

```bash
git show f69c3387:docs/roadmap/w1-implementation-plan.md
git show f69c3387:docs/roadmap/2026-09-15-product-amendment.md
```

Baseline permalinks:

- https://github.com/waldoco/Waldo-Kennel/blob/f69c3387b13b00a1395c7acf7bd468163e6ae023/docs/roadmap/w1-implementation-plan.md
- https://github.com/waldoco/Waldo-Kennel/blob/f69c3387b13b00a1395c7acf7bd468163e6ae023/docs/roadmap/2026-09-15-product-amendment.md

## Delete now

No migration, accepted record, evidence report, research source, or licensed reference is deleted. Shortening the two plans removes them from the active reading path while Git history preserves their full provenance. Other physical cleanup waits for the serial packaged proof and a compatibility review.

## Code keep/delete/defer matrix

| Area | Keep now | Delete after cutover evidence | Defer |
|---|---|---|---|
| Outcome/Contract/Plan/WorkUnit/Attempt/Result storage | yes | no | n/a |
| W1.1 scheduler, leases, fences, launch ownership, recovery | yes | obsolete duplicate branches only | concurrency enablement |
| W1.2 budget accounting/idempotency | yes | arbitrary default-stop policy paths if any | policy tuning |
| S1 local authority | source implementation retained; packaged macOS/runtime proof pending | compatibility routes after migration gate | wider remote authority |
| Chat controller and Codex app-server driver | yes, extend | duplicate session writers | other-provider parity |
| governed TUI `OneShot` path | historical compatibility only | production admission after vNext proof | n/a |
| legacy AO orchestrator/session CLI | compatibility/read-only as documented | when usage and migration gate prove safe | no new features |
| current needs-you replacement logic | preserve until replacement path lands | replace atomically after tests | n/a |
| Mission Supervisor | new typed protocol | hidden/untyped model-control experiments | global supervisor/swarm |
| UI Work shell/components | reuse | duplicate stale screens after parity | broad redesign |

## Repository-owned prompt, skill, and handoff audit

- `AGENTS.md`, `CONTEXT.md`, and `CLAUDE.md`: high-reach; canonical links and terminology updated.
- `backend/internal/skillassets/using-kennel/**`: current CLI help is a compatibility surface. It must not tell a provider to self-schedule or treat a process/session as the Outcome authority. Existing command facts remain until the CLI changes; target-runtime language points to the canonical architecture.
- Dated handoffs under `docs/handoffs/**` and `docs/verification/**`: retained as historical evidence, not canonical instructions.
- No platform/system prompt is in scope or modified.

## Link and terminology checks required before review

- local Markdown links resolve;
- Mermaid fences are balanced and render in GitHub Markdown;
- active canonical files contain no active one-shot target or W1.3-first instruction;
- every "current" claim in `docs/STATUS.md` is backed by the baseline;
- historical files that can be mistaken for active authority carry a supersession marker;
- UI reference assets have provenance and license treatment recorded.

## UI reference adoption and license ledger

| Reference | Adoption | License/provenance treatment |
|---|---|---|
| existing Kennel Work shell and repository screenshots/assets | layout and product continuity only | repository-owned; retain existing notices/history |
| external agent-management UI references cited by historical design docs | interaction lessons only, no copied code/assets | keep as references; verify source license before any code or asset adoption |
| Codex app-server protocol | behavior/protocol evidence | implementation dependency/reference; do not copy third-party assets |
| Mermaid diagrams in this reset | repository-authored | covered by repository license |

No new external code, image, font, or UI asset is introduced by this package.

## Unresolved implementation details, not ontology forks

The product ontology is locked. Implementation still must specify and independently review exact typed command/event schemas, capability tokens, context size/redaction policy, check runner interface, provider acknowledgement evidence, Supervisor summary cadence, and measured budget targets. These choices may not weaken the locked authority and cardinality rules.

## Reset v2 consistency manifest

- Canonical reading order is identical in `AGENTS.md`, `docs/README.md`, and this audit: Product, runtime, compatibility, execution map, Status, ADR 0017, then linked lower-level evidence.
- W1.0-W1.2 are accepted foundations. S1 is source-accepted; packaged macOS/runtime proof remains open.
- The execution map contains an explicit dependency-ordered Context and Artifact Protocol before attention, Supervisor, projection, and UI work.
- Supervisor automatic commands are limited to allowlisted context delivery and reversible guidance with exact revisions/generation, budget, and idempotency; other actions are recommendations.
- Periodic summary is a daemon-owned durable cadence event. The Supervisor never self-schedules.
- Worker outputs route through daemon verification and named Plan edges. Direct worker command/context arrows are forbidden.
- The packaged proof is serial. Independent concurrency starts only after it passes.
- Mermaid remains the canonical Markdown diagram format. Eight repository-authored SVG companions provide rendered review evidence.
