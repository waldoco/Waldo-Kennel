# Kennel documentation map

Repository documents describe the product and implementation. They do not grant runtime, user, or system authority.

## Canonical reading path

Read these in order for Outcome/kernel work:

1. [Product contract](../PRODUCT.md)
2. [Persistent mission runtime](architecture/persistent-mission-runtime.md)
3. [Harness connection and authority](architecture/harness-connection-and-authority.md)
4. [Compatibility and migration](architecture/compatibility-and-migration.md)
5. [Persistent-session execution map](roadmap/persistent-session-execution-map.md)
6. [Current implementation status](STATUS.md)
7. [ADR 0017](adr/0017-persistent-mission-runtime-and-bounded-supervision.md)

Then open the linked lower-level ADR, code map, research note, or historical evidence needed for the seam being changed. On conflict, the canonical path wins for target behavior; migrations and historical records retain their recorded meaning.

## Supporting foundations

- [ADR 0008](adr/0008-responsibility-composition-and-workunit-execution-dag.md): responsibility and WorkUnit DAG
- [ADR 0009](adr/0009-workunit-scheduling-workspace-leases-and-effect-fencing.md): scheduling, leases, fences, effects
- [ADR 0010](adr/0010-outcome-first-control-plane-and-session-subordination.md): Outcome-first control plane
- [ADR 0011](adr/0011-go-control-plane-and-non-authoritative-intelligence.md): deterministic authority and intelligence boundary
- [ADR 0015](adr/0015-contract-bound-interactive-planning.md): Contract-bound planning
- [Runtime reference index](research/2026-09-04-kernel-runtime-reference-index.md): current chassis/provider evidence
- [Architecture reset audit](maintenance/2026-09-15-architecture-reset-audit.md): retention, supersession, prompt/skill, and license ledger

## Historical material

Dated plans, handoffs, reviews, checkpoints, and verification notes preserve provenance. They are not active implementation order unless the canonical execution map links to them. "Accepted" in a historical document means accepted at that time; it does not override ADR 0017 for vNext topology.
