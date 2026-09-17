# Documentation consolidation ledger

**Status:** proposed treatment, prepared against `beta` at `5304d569aa2e29f0cc77d57539aa2f65a4d01484` on 2026-09-14.

This ledger prevents cleanup by filename or age. The first package creates the canonical set, rewrites the competing roadmap, and labels status authority. Historical evidence and accepted decisions remain. Further removals or redirects require reference and migration proof.

## Treatment rules

- **Canonical:** maintained current product, architecture, sequence, experience, or status truth.
- **Supporting:** current technical/operator detail under the canonical contract.
- **Historical evidence:** immutable dated proof for its named baseline. Never current status by itself.
- **Redirect:** a short compatibility pointer retained because live references still target the old path.
- **Delete after proof:** content is already superseded and no live reference or audit need remains. Git history is not by itself enough when repository links still depend on the path.

## This review package

| File | Treatment | Reason |
| --- | --- | --- |
| `PRODUCT.md` | add, canonical | one promise, ICP, problem/value, four moments, readiness, two horizons |
| `ROADMAP.md` | rewrite, canonical | replace date/feature lanes with quality-gated core and extension dependencies |
| `docs/architecture/outcome-loop.md` | add, canonical | one Go-owned loop and nine crossings |
| `docs/architecture/admission-and-events.md` | add, proposed W1.0 seam | one admission/event meaning before code fans out |
| `docs/product/experience.md` | add, canonical | complete Ask/Approve/Watch/Decide UI contract |
| `docs/decisions/product-and-architecture.md` | add, canonical | concise choices likely to regress |
| `docs/roadmap/launch-2026-09-18.md` | add, milestone view | date-specific dependency/evidence view without redefining product |
| `docs/handoffs/worker-slice-template.md` | add, reusable | bounded worker scope and evidence handback |
| `docs/README.md` | rewrite | one documentation map and authority rule |
| `docs/STATUS.md` | add authority notice only | preserve evidence ledger; do not rewrite unverified shipped claims |

## Keep as supporting truth

- `docs/architecture.md`: technical chassis, repository layout, ports/adapters, persistence, and generated contracts. Reconcile terminology in a later focused pass; do not delete a file with broad live references.
- `CONTEXT.md`: domain glossary. Later trim only entries contradicted by the canonical loop.
- accepted `docs/adr/*`: durable narrow decisions and migration rationale.
- `docs/contracts/*`: detailed delivery, run-state, and rework contracts until implementation and ADR mapping show they are fully absorbed.
- operational guides for development, daemon, CLI, telemetry, security, support, release, harnesses, and users.
- `docs/verification/*`: dated evidence. Preserve exact baseline and tested-layer truth.
- research and benchmark records: references, not product authority.

## Stop treating as current authority

These remain in the first package because they contain referenced rationale, migration details, or evidence. The documentation map now classifies them as historical/supporting.

| File or family | Current problem | Next treatment |
| --- | --- | --- |
| `docs/product/2026-09-14-production-readiness-program.md` | active-program language and checkpoint ledger compete with `ROADMAP.md` and `STATUS.md`; baseline predates current beta | replace with a short redirect after extracting still-open evidence references into STATUS/milestone; no new status updates here |
| `docs/product/kennel-build-program.md` | older build sequence still has many ADR/spec references | preserve as historical program, then redirect only after inbound references move to canonical roadmap/ADRs |
| `docs/product/kennel-v1-product-architecture.md` | large product architecture overlaps Outcome loop but is heavily referenced | retain for migration/domain detail; reconcile inbound authority links before replacing with redirect |
| `docs/product/kennel-dogfood-acceptance-matrix.md` | older acceptance source overlaps the new readiness and W1.6 gate | extract unique scenarios into the packaged proof plan, then redirect |
| dated product reset, positioning, UX audit, launch experience, beta-boundary, and interactive-planning files | conclusions are useful history but some target state is stale | mark historical in their headers only when next edited; move current requirements to canonical docs, never update their status |
| `docs/superpowers/plans/*` and `docs/superpowers/specs/*` | many old plans appear actionable | preserve as historical design/implementation evidence; canonical roadmap must explicitly link any still-active slice |
| `docs/handoffs/2026-09-12-pr110-launch-fixes/*` | one PR baseline, not live launch truth | preserve as historical execution evidence; no current status edits |
| `DESIGN.md` | design-system guidance still contains supersession banners and session/spawn-worker framing | rewrite in a separate visual-token package after U2.1 disposition; keep tokens/proven accessibility rules, remove competing product navigation/state claims |

## Redirect or delete only after reference proof

- `docs/product/kennel-v1-team-review-packet.md` is already a five-line superseded pointer. Keep until its remaining research/plan references are redirected; then delete.
- `docs/product/kennel-v0-first-outcome-slice.md` is already a superseded pointer with live references. Keep until those references move; then delete.
- disconnected dated handoffs with no external artifact/audit dependency may be deleted in small batches after `rg` reference checks and confirmation that STATUS/ADR/verification links preserve their evidence.
- compatibility, migration, and generated documentation must follow the same caller/record/migration proof as code. Do not batch-delete it during product cleanup.

## Never collapse

- `PRODUCT.md` and `STATUS.md`: intent and shipped evidence must remain separate.
- architecture contracts and ADRs: current whole and accepted rationale serve different jobs.
- current status and dated verification: a live ledger and immutable proof serve different jobs.
- launch milestone and canonical roadmap: a date view cannot become product scope.

## Follow-up doc slices

1. Reconcile W1.0 names against generated API/storage and update the proposed admission contract.
2. Update `AGENTS.md` canonical read order after this package is approved; it is an agent-operating file and was intentionally not mixed into product consolidation without review.
3. Reconcile `docs/architecture.md` and `CONTEXT.md` terminology without removing chassis detail.
4. After U2.1 disposition, rewrite `DESIGN.md` around current tokens and reachable Ask/Approve/Watch/Decide states.
5. Extract unique evidence links from the three competing program docs, replace them with redirects, and update inbound links in one mechanical package.
