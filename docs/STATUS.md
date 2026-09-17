# Current implementation status

- Current promoted head: `outcome-loop` at `2c469cb11681da14a1abfb62f6171b2b7c4b5d44`
- Target: [persistent mission runtime](architecture/persistent-mission-runtime.md)
- Build order: [persistent-session execution map](roadmap/persistent-session-execution-map.md)

This file separates promoted implementation from partial and target behavior.

## Stage status

| Stage | Status | Promoted evidence and remaining work |
|---|---|---|
| 0 Contract freeze | Done | Canonical lineage from `f31cdca4ea55fb25903f7af55ec45c7abccdcc04`. |
| 1 Native Codex persistent substrate | Done | Through `643e955c16b7b8907d2ce4c7c285bedaa148d4b6`: persistent thread, steer, typed answer, interrupt/quiescence, fresh-process resume and Linux live proof. |
| 2 Codex compatibility negotiation / unified controller | Done | Through `6bf0e39c`: S2.1-S2.4, including crash/restart/concurrency/adoption matrix and honest delivery-unknown. |
| 3 Durable owner-command authority | Done | `2c469cb1`: pairing, owner-proof target v2 and transactional post-pair command ingress/recovery. Production listener composition is reviewed separately at candidate `18a77408`, not promoted. |
| 4 Harness installation and connection | Partial | Trusted install/drift, pairing, connection kernel and reconnect exist. Public approval/revocation UI API, truthful connection projection, packaged journey, listener promotion and Mac live proof remain. |
| 5 Contract intake | Partial | Durable versioned Contracts, adaptive intake, context and one clarification boundary exist. Multi-question rounds and packaged first-run completion remain. |
| 6 Mission planning | Partial | Durable PlanningSession, multi-turn UI/service, typed proposal and approval exist. The literal installed/provider-verified `/mission` command, automatic first turn through it and packaged proof do not. |
| 7 Serial WorkUnit execution | Partial | Scheduler, exact admission, custody fence and session spawn exist. Typed Needs-You Q&A and clean-machine three-unit serial proof remain. |
| 8 Verification custody and artifact lineage | Partial | Retention, digests, receipts and proof ledgers exist. Complete manifests and integrated-result immutable-tree proof remain. |
| 9 Integration and Result | Partial | Result/proof APIs and UI exist. Canonical MissionProjection, typed next action/freshness and renderer-friendly Result helpers remain. |
| 10 Mission Supervisor | Not started | Persistent per-Plan supervisor protocol, typed events/commands, budgets and degraded rebuild remain target behavior. OS process supervision is not this stage. |
| 11 Recovery/correction packaged proof | Not started | Lower-layer recovery tests exist; the full clean-machine cross-stage matrix does not. |
| 12 Expansion | Not started | Parallel units, broader harness expansion and cleanup wait for Stage 11. |

## Current P0 backend gaps

1. A generation-safe typed Needs-You question/choice projection and answer command with queued, acknowledged, refused and delivery-unknown states.
2. A public renderer-safe harness pairing, connection, authority and revocation API with durable receipts.
3. A server-owned WorkUnit MissionProjection mapping current Attempt/session, attention, proof, next action and freshness onto each WorkUnit.

Transition-triggered cached summaries, typed freshness/next-action vocabulary and Result helpers are Stage 9 production-clarity work. They remain non-authoritative.

## Honest boundaries

- Orchestration logic is substantial: deterministic scheduling, admission, fences, spawn, handoff and recovery exist. Final serial/integration proof remains.
- Planning service and UI exist. An installed `/mission` command does not.
- Skill materialization and adapter install machinery exist; general plugin lifecycle and installed mission-command verification remain partial.
- Loop mechanics exist, but the governed Mission Supervisor loop is Stage 10 and not implemented.
- Frozen repository/supplied-document context, grant digests, snapshots and predecessor artifacts exist; mission-command visibility, Supervisor packets and final lineage proof remain.

Legacy one-shot records stay readable and labeled. They gain no right to resume through the persistent runtime.
