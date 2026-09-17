# Persistent runtime review reconciliation

Status: review package only; no implementation acceptance or cutover.

| Review gap | Frozen rule | Implementation stage | Decisive falsifier |
|---|---|---|---|
| Moving verification input | Close turns/children, fence writes, content-address snapshot; each check/publication/review binds the exact snapshot it attests; the integrated snapshot retains ordered input lineage | 2, 6, 11 | File changes during checks appear under an earlier pass |
| Artifact application/integration | Serial dependents start from verified predecessor tree; fan-in is explicit integration; final checks use frozen integrated tree | 6, 11 | B lacks A API, or separate passes publish failed assembly |
| Plugin auth and reconnect | Pair adapter, bind capability/digest/version/mission; proposals are not authority; routine current answers avoid dialogs; reconnect rotates capability without daemon restart | 3, 7, 10, 11 | Plugin self-authorizes, answers cannot land, dialog spam, reconnect forks mission |
| Native coding compatibility | `codex_native_worktree_v1` inspect-edit-fail-repair-rerun-steer loop; inherited config provenance; FS/network/effect boundaries | 1, 5, 11 | Chat persists while coding is disabled or denied unexpectedly |
| Intake and mission skill | Multi-question/follow-up intake; installed verified command; automatic first planning turn; schema/approval/error feedback | 4, 10, 11 | One-question cap, empty composer, docs-only command |
| Rework and Supervisor failure | Artifact-edge invalidation; revised Result lineage; explicit auto-rework grant; Supervisor is fail-open for usability but fail-closed for its commands | 6, 8, 9, 11 | Stale verification survives or Supervisor failure blocks owner/healthy workers |

## Owner-review decisions preserved

- Keep the persistent mission architecture and dependency-led build order.
- Kennel owns deterministic authority; harnesses and Supervisor can propose but cannot authorize material changes.
- Installation and version drift are product flows, not setup notes.
- Implementation remains paused until this reconciled package is accepted.

## Review checklist

1. Does each material transition name the actor, authority source, revision/generation, idempotency key, and acknowledgement/reconciliation state?
2. Can every check and displayed/published file be traced to the exact WorkUnit or integrated snapshot it attests, with integrated input lineage?
3. Can every WorkUnit input be reconstructed from a source tree plus exact verified dependencies?
4. Does a reworked upstream output mechanically stale every consumer and assembled Result?
5. Can a new desktop run reconnect without mission duplication or a healthy-daemon restart?
6. Can an installed adapter only propose/transport within its paired capabilities?
7. Do routine answers avoid confirmation while every authority delta is reviewable?
8. Does the first live proof perform real coding with declared config, network, filesystem, and effect rules?
9. Are intake, mission registration, automatic planning, approval, pending, and error states packaged behavior?
10. Can all safe deterministic and owner paths function with the Supervisor unavailable?
