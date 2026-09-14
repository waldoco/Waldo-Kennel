# Kennel roadmap

This roadmap is quality-gated. Dates may force integration and decisions, but they do not define the product or excuse an incomplete core loop.

- [Product contract](PRODUCT.md)
- [Outcome architecture](docs/architecture/outcome-loop.md)
- [Current evidence and implementation status](docs/STATUS.md)
- [September 18 integration milestone](docs/roadmap/launch-2026-09-18.md)

## North star

A serious coding-harness power user can give Kennel a software Outcome and receive a finished, verified change with less operational work and more confidence than their manual orchestration workflow.

## Core roadmap

### R1. One dependable Codex Outcome loop

Deliver Ask -> Approve -> Watch -> Decide through one Go-owned control plane:

```text
Outcome -> clarification -> Contract -> Plan -> authorization
        -> WorkUnits and Attempts -> observation and recovery
        -> Verification and Result -> rework or owner Accept
```

Exit gate:

- one shared, fresh AdmissionVerdict prevents impossible approval;
- WorkUnits have coherent intent, permissions, outputs, checks, dependencies, and budgets;
- Codex execution has truthful typed progress and bounded resource use;
- retry/replacement is atomic, idempotent, restart-safe lineage;
- Mission Control has one durable projection and one true next action;
- evidence maps to Contract criteria;
- the complete packaged macOS journey reaches explicit owner Accept without hidden rescue;
- Ask, Approve, Watch, and Decide pass accessibility, responsive, reduced-motion, and real-pixel review.

### R2. Native-harness depth and safe parallelism

Extend the proven adapter and scheduler semantics to Claude Code, OpenCode, Pi, and parallel WorkUnit execution.

Entry gate: R1 is dependable and provider conformance covers verification, capabilities, sandbox representation, native session lifecycle, typed events, usage, and provenance.

Exit gate:

- capability-aware routing never silently degrades;
- serial and parallel paths share authority and recovery semantics;
- leases, fences, dependency release, consolidation, and restart recovery pass induced failures;
- provider-specific power remains available without leaking provider policy into the core.

### R3. Portable context and memory

Add source-linked Context Packets, Result capsules, project understanding, and cross-harness memory.

Entry gate: Result and evidence contracts are stable.

Exit gate:

- context is attributed, bounded, inspectable, owner-correctable, and erasable;
- inference never becomes authority or hidden truth;
- transfer improves completion quality without leaking unrelated data;
- every harness receives equivalent load-bearing context through its native format.

### R4. Teams and wider Waldo surfaces

Add shared Outcomes, roles, policy, review, audit, team memory, and richer channels/product surfaces.

Entry gate: single-owner authority, acceptance, audit, and erasure are proven.

Exit gate:

- every action has a clear principal and authority source;
- ownership transfer, review, conflict, and erasure are explicit;
- channel and desktop views project the same Outcome truth;
- team value is measured in completion quality and reduced coordination, not agent activity.

## Extension rule

A proposed feature enters the active core only when it improves Outcome quality, autonomous completion, visibility, recovery, proof, or owner control and its dependencies are ready. Otherwise record it as a future direction, not a current claim or parallel source of truth.

## Removal rule

For legacy code or documentation:

1. enumerate callers and references;
2. identify persisted records, migrations, audit, and rollback needs;
3. redirect to the canonical path;
4. prove focused and full gates;
5. remove in a small reviewable change.

Hide obsolete UI before deleting history. Do not delete something because its name or date looks old.
