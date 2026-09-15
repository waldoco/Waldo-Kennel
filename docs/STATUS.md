# Current implementation status

- Baseline: `outcome-loop` at `f69c3387b13b00a1395c7acf7bd468163e6ae023`
- Target architecture: [persistent mission runtime](architecture/persistent-mission-runtime.md)
- Active order: [persistent-session execution map](roadmap/persistent-session-execution-map.md)

This file separates implemented truth from accepted target behavior.

## Implemented and retained

- Durable Outcome/Contract/Plan/WorkUnit/Attempt/Result and event foundations from W1.0.
- Scheduler, workspace lease/fence, launch ownership, recovery, and effect boundaries from W1.1.
- Durable budget/idempotency and cross-process race protections from W1.2.
- S1 local owner-command source implementation and compatibility ingress are source-accepted; independent packaged macOS/runtime proof remains open.
- Existing Chat driver/controller contracts with stored provider conversation IDs, serialized turns, interrupt, request resolution, and normalized events.
- Codex app-server adapter support for `thread/start`, `thread/resume`, `turn/start`, and `turn/interrupt`; live evidence shows a fresh process can resume a thread.
- Current Outcome admission/execution and Mission Control surfaces documented by the historical verification set.

## Implemented behavior that is not the vNext target

- Governed Codex TUI execution sets one-shot mode and launches `codex exec`.
- Historical completion/replacement/attention paths were built around that execution model.
- Some legacy AO/session CLI and repository skill language remains as compatibility documentation.
- Existing one-shot Attempts have no right to resume through the persistent runtime.

These facts remain readable and testable. They are not design authority for new execution.

## Accepted target, not yet implemented

- Separate Contract and fresh `/mission` planning threads in one UI timeline.
- One Mission Supervisor thread per active Plan revision.
- One WorkUnit Attempt = one exclusive worktree lease = one Kennel Session = one persistent primary Codex thread with many turns.
- Typed daemon/Supervisor event and command protocol with bounded automatic steering and failure isolation.
- Paired, capability-negotiated harness adapters with install/upgrade, reconnect, and connected/degraded/action-needed states.
- Multi-round Contract intake and a verified installed mission command with automatic first planning turn.
- A named native coding profile proving inspect/edit/fail/repair/rerun/steer with explicit filesystem/network/effect limits.
- Versioned, hashed input/output manifests, verified predecessor-tree handoff, explicit integration, immutable verification snapshots, and mechanical stale-lineage invalidation.
- Nonterminal `needs_you` with same-thread answer/resume.
- Acknowledged/reconciled answer, steer, cancel, and hard-stop effects.
- Explicit readiness claim, daemon checks, same-thread rework, and owner Accept.
- Unified MissionProjection and full UI integration.
- Serial packaged proof and subsequent independent-worktree concurrency.

## Compatibility promise

W1.0-W1.2 remain accepted foundations. S1 is source-accepted, with its packaged macOS/runtime gate still pending. Migrations and historical events are not deleted or reinterpreted. Legacy one-shot Attempts stay visible as `legacy_one_shot`. vNext uses a distinct `persistent_codex_v1` generation and no dual-write. Full policy: [compatibility and migration](architecture/compatibility-and-migration.md).

## Current gate

The architecture is retained, but final review reconciliation for verification snapshots, artifact application, authenticated harness connection, native coding compatibility, mission entry, and rework/failure transitions is in progress. Code implementation, deletion, and push are paused until review accepts the canonical package. The preserved W1.3 nonterminal contract patch is evidence/input, not an accepted patch or active implementation order.
