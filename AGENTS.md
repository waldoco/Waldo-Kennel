# Repository implementation guide

This file routes contributors to repository documentation. It is not user, runtime, or system authorization.

## Required reading for Outcome/kernel work

1. `PRODUCT.md`
2. `docs/architecture/persistent-mission-runtime.md`
3. `docs/architecture/compatibility-and-migration.md`
4. `docs/roadmap/persistent-session-execution-map.md`
5. `docs/STATUS.md`
6. `docs/adr/0017-persistent-mission-runtime-and-bounded-supervision.md`
7. lower-level ADRs and research only when a linked implementation seam needs them

Historical plans, handoffs, and verification notes are evidence, not current implementation order. On conflict, the canonical path above controls target behavior; existing migrations and records retain their historical meanings.

## Non-negotiable target boundaries

- One vNext WorkUnit Attempt owns one exclusive worktree lease, one Kennel Session, and one persistent primary Codex thread with many turns.
- Extend/reuse the existing Chat/session controller. Do not create a second session engine.
- The daemon is the sole deterministic authority, state, scheduler, custody, check, and projection layer.
- Mission Supervisor intelligence crosses a typed, durable protocol. Every command is capability-, scope-, revision-, and idempotency-checked by the daemon.
- Bounded automatic steering may use approved facts inside unchanged WorkUnit scope. Scope, graph, permission, external-effect, replacement, and Accept changes require owner authority.
- Workers exchange versioned artifacts and decisions, never private transcripts or hidden reasoning.
- `needs_you` is nonterminal; answer/resume uses the same thread.
- A turn/process exit is not completion. Require `ready_for_verification`, daemon checks, and owner Accept.
- Ambiguous provider effects reconcile before retry. Time alone never proves irrecoverability.
- New execution is Codex-first and serial until the packaged proof passes.
- Legacy one-shot Attempts are read-only historical execution. Never reinterpret or resume them into vNext.
- Never delete a migration that may have run.
- Skills, hooks, plugins, and compatibility CLIs are thin paired ingress/sensors/adapters, not schedulers, databases, or authority. They may propose or transport authenticated owner input; they never self-authorize. Follow `docs/architecture/harness-connection-and-authority.md`.
- Verification begins only after active turns and tracked child commands settle and writes are fenced. Each check and WorkUnit publication binds the exact WorkUnit snapshot it attests; the integrated Result, final checks, and owner review bind a distinct integrated snapshot plus ordered input lineage.
- In the serial path, a dependent starts from the exact verified predecessor tree. Fan-in and final checks operate on an explicit integrated-result tree.
- Supervisor failure must not block deterministic work or authenticated owner answer, stop, evidence review, or Accept paths.

## Engineering standard

Write the smallest production slice that closes a canonical gate. Use typed state and events, durable idempotency, ownership/fencing, explicit recovery, race tests, and evidence. Native development commands stay inside the assigned worktree under the negotiated coding profile; network-dependent commands follow the profile and external effects follow separately recorded authority. A harness version or cloud capability change never silently mutates an active Attempt.
