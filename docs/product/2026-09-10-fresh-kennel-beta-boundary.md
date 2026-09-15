# Fresh Kennel beta and Island integration boundary

> **Classification (2026-09-15):** Historical beta boundary; retained for compatibility evidence. Use the [canonical documentation map](../README.md).


The owner chose a fresh installation on 2026-09-10. Compatibility with AO profiles is not a product requirement. This decision does not authorize deleting local files or rewriting published Git history. Use fresh isolated profiles for validation.

## Product ownership

Kennel's responsibility model is Project → Outcome → Contract → Plan/WorkUnits → Attempts → retained result → evidence/verification → owner acceptance → delivery. Provider sessions are implementation resources beneath Attempts. Do not introduce a second session-first task model in Island or new Work UI.

The root application route enters Work. Focused Work mode is the default; renderer experiments may opt out with `VITE_KENNEL_WORK_LAUNCH=0`. Island does not start in Electron unless the developer explicitly sets `KENNEL_ENABLE_ISLAND=1`. Island's team may develop the ambient projection independently without making it the source of execution authority.

AO commit-author/default-branch inference and AO-managed-gitignore adoption are removed. New repositories rely on explicit Kennel registration/default-branch records. Existing generic Git, process, provider and storage utilities are not evidence of AO responsibility semantics and should not be rewritten merely to change provenance. Any remaining session-first product path must be assessed and retired explicitly; this checkpoint is not a claim that every inherited module has been replaced.

## Island team contract

- Read daemon-owned Outcome, current Contract/Plan, schedule, Attempt and proof projections through existing generated API clients.
- Subscribe to canonical trigger-backed CDC and refetch on reconnect. Never infer completion from silence, a terminal transcript or a process exit.
- Display Outcome title, current next decision/blocker and freshness. Drill into Work Mission Control using Outcome identity. Session inspection is optional detail.
- Send explicit user actions through the daemon's existing authorization/replay boundary. Island must not directly launch providers, write SQLite, create acceptance automatically or duplicate the scheduler.
- Show missing provider, unsupported policy, missing artifact and materialization-unavailable states truthfully. Never replace a refused action with a legacy session launch.
- Use fixture state for independent UI development, but label that evidence separately from a real daemon/provider journey.

## Current execution boundary

The check sandbox refuses unsupported scopes and confines file contents to the workspace plus declared operating-system runtime resources. Network remains denied. Proof finalization uses an append-only database proof generation, including owner decisions/corrections, rather than wall-clock arrival assumptions.

Dependent WorkUnits are refused with `UPSTREAM_MATERIALIZATION_UNAVAILABLE` after valid upstream receipt checks until canonical provisioning is complete. This is intentionally fail-closed. Do not remove this refusal until the successor receives verified retained bytes, base, modes and deletions before provider launch and input versions are recorded durably.

This beta foundation is not the completed launch loop. Still required: C-13 materialization, production deterministic checks/evidence, durable run intent/rework, full Board/Mission supervision, supplied-document wiring, durable delivery and packaged/live-provider acceptance. Island work may proceed against the canonical projections while these backend slices continue.

## Next core implementation checkpoint

1. Materialize retained dependency results at the owned workspace provisioning seam and bind input versions into admission/replay.
2. Prove A → daemon restart → B content continuity, including conflict, corruption, cancellation and unknown custody cases.
3. Route checks through exact policy enforcement and write independently observed artifact-bound proof.
4. Persist run intent so Pause/Cancel and rework survive restart without duplicate Attempts.
5. Complete document and delivery paths, then run the real-daemon and packaged matrix before launch claims.
