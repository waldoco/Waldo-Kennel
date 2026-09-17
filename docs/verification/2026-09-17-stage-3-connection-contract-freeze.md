# Stage 3 harness-connection contract freeze

- Baseline: `outcome-loop` at `6bf0e39c9fb3ce652439f399adfb0a4ac2527365`
- Slice: 3.0, contract and file ownership only
- Next slice: 3.1, Codex-first harness-connection identity and capability kernel

## Root failure

Protocol negotiation proves what an installed harness can do, but it does not authenticate the adapter carrying commands. The current app-run owner bearer authenticates Electron main and the browser/supervisor verifiers authenticate other narrow transports; none pairs a harness adapter or binds its transport rights to one installation, adapter build, harness, app run, mission, generation, expiry, and command class. Treating negotiation or plugin content as authority would let an unpaired, stale, or substituted adapter widen actions.

## Frozen vocabulary

`HarnessConnection` is the durable pairing and capability identity. It is not a Session, provider thread, owner identity, or source of owner authority.

Connection state is derived, never independently written:

- `connected`: paired, unexpired, unrevoked, current generation; installation, adapter digest, harness, app run, mission, provider version/protocol fingerprint, and all required capability classes match current verified facts.
- `degraded`: the binding is authentic and current, but an optional negotiated capability is missing. The repair action identifies the missing capability.
- `action_needed`: pairing/authentication/current-generation or required compatibility facts fail. No harness command is accepted.

Initial capability classes are a closed Codex-first transport set: `turn`, `steer`, `answer`, `interrupt`, `cancel`, `replace`, `approval`, and `accept`. Possessing one class grants only transport for that class. It never authenticates owner content or grants scope, policy, filesystem, network, external-effect, replacement, approval, or Result-Accept authority.

Stable action-needed reasons and repair actions:

| Reason | Repair action |
| --- | --- |
| `unpaired` | `pair_adapter` |
| `expired` | `rotate_capability` |
| `revoked` | `repair_pairing` |
| `stale_generation` | `reconnect_adapter` |
| `binding_mismatch` | `repair_pairing` |
| `required_capability_missing` | `repair_compatibility` |
| `protocol_drift` | `repair_compatibility` |

Optional capability loss uses `degraded` with reason `optional_capability_missing` and repair `review_degraded_capability`.

## S3.1 file ownership

Allowed production ownership:

- `backend/internal/domain/harness_connection.go`
- `backend/internal/ports/harness_connection.go`
- `backend/internal/harnessconnection/`
- `backend/internal/storage/sqlite/migrations/0145_harness_connections.sql`
- `backend/internal/storage/sqlite/queries/harness_connections.sql`
- generated sqlc outputs required by that query
- `backend/internal/storage/sqlite/store/harness_connection_store.go`
- focused tests for those paths and the migration ledger entry

No Chat controller, HTTP route, Electron/plugin transport, installer, owner-command route, Outcome admission, Session schema, or UI is changed in S3.1.

## S3.1 invariants

1. Pairing authenticates adapter transport only. Adapter content is never owner authority.
2. The random bearer is returned once and never persisted. Durable state stores only a one-way verifier.
3. Authentication binds the entire tuple: connection ID, installation identity, adapter digest, harness, app run, mission, generation, expiry, and requested capability class.
4. Issue, rotate, revoke, and authenticate are fail-closed. Exact retries converge; changed semantics conflict.
5. One current generation wins under concurrent writers. A rotated or revoked bearer cannot become valid again.
6. Restart reads the durable verifier and binding; it neither mints a replacement bearer nor marks a connection connected without current verified facts.
7. Provider version and protocol fingerprint are provenance and compatibility inputs, never identity or authority by themselves.
8. API/log/domain string forms must not expose the verifier. S3.1 adds no API exposure.

## Blocked-unknown operator route

Stage 3 freezes the route shape but does not weaken Stage 2 delivery uncertainty. A governed command blocked in `delivery_unknown` must expose a stable operator action-needed reason, retained command/correlation/reconciliation evidence, and only these decisions:

- attach positive acknowledgement evidence and reconcile;
- attach positive rejection/no-effect evidence and reconcile;
- keep blocked when evidence is insufficient.

Time, absence from history, adapter reconnect, or operator preference cannot prove no effect. Unsafe redelivery remains blocked. Stage 7 owns the full retained-custody question/answer/resume experience; Stage 3 connection state must surface `action_needed` rather than hide the blocker or claim liveness.

## S3.1 gate

Focused domain/store/authority tests must prove issue/authenticate, wrong tuple and undeclared class rejection, expiry, revocation, rotation, stale generation, exact replay/conflict behavior, concurrent generation winner, restart authentication from a durable verifier, derived connected/degraded/action-needed states, migration upgrade/rollback, no bearer persistence, and no secret-bearing string/JSON shape. The relevant SQLite and full backend gates must pass before promotion.
