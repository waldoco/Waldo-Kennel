# Compatibility and migration policy

- Status: canonical compatibility policy for the persistent mission runtime
- Baseline: `outcome-loop` at `f69c3387`

## Accepted foundations retained

W1.0-W1.2 remain accepted foundations, not discarded work:

- W1.0: durable Outcome, Contract, Plan, WorkUnit, Attempt, approval, event, and Result identity/state foundations.
- W1.1: authoritative scheduling, admission, lease/fence, launch ownership, and recovery foundations.
- W1.2: durable budget accounting, idempotency, and cross-process race protections.
- S1: source-accepted local owner-command implementation, actor binding, authenticated commands, and compatibility ingress; packaged macOS/runtime proof remains open.

Their schemas and historical events retain their original meanings. vNext extends them through additive revisions, typed events, and compatibility readers. It does not rewrite accepted rows to make them look like persistent sessions.

## Historical execution records

Every Attempt records an execution-model generation:

- `legacy_one_shot`: existing records and new records created before cutover;
- `persistent_codex_v1`: new records admitted after the cutover gate.

If older data lacks this field, the reader deterministically classifies it as `legacy_one_shot`; a migration may materialize that classification without changing any other semantics. Legacy Attempts are readable, auditable, and linkable to Results. They cannot be resumed through `thread/resume`, receive vNext commands, or be silently replaced by a persistent Attempt.

No dual-write spans the two execution models. In-flight legacy Attempts finish, cancel under their original semantics, or remain historical. A user who wants to continue the intent creates an explicit new vNext Attempt with predecessor lineage and a fresh approved input manifest.

## S1 compatibility

S1 is the source-accepted owner-command authority boundary; packaged macOS/runtime proof remains an open cutover gate. Existing routes may remain as compatibility ingress while callers move to the unified command envelope. Compatibility adapters must bind the authenticated actor, target generation, expected revision, and idempotency key before reaching the daemon. They do not bypass vNext validation or write provider state directly.

## Storage and migration rules

- Never delete a migration that may have run.
- Never reinterpret accepted events in place.
- Prefer additive tables/columns and versioned payloads.
- Preserve unknown payload fields for round trips when required.
- Backfills must be deterministic, restartable, and evidence-producing.
- A compatibility reader may project old records into Mission Control, but its display must identify the older execution model.
- Provider thread IDs, context hashes, command generations, and event cursors are written only for vNext records.
- Cutover requires rollback instructions and a pre-cutover database backup test.

## Cutover gates

1. Native Codex app-server start, multi-turn, interrupt, and fresh-process resume are proven.
2. The unified session controller has sole-writer and idempotent command tests.
3. One Attempt/session/thread/worktree cardinality is enforced.
4. Delivery-unknown answer, steer, cancel, and hard-stop reconciliation is proven.
5. MissionProjection renders both historical and vNext records truthfully.
6. A serial packaged Outcome passes restart, attention, check, rework, and Accept tests.
7. Independent review accepts the patch and migration/rollback evidence.
