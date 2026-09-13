# Kennel documentation

Read only the documents relevant to your change. Current implementation, target
architecture and historical evidence serve different purposes.

| Need | Start here |
| --- | --- |
| Understand or run Kennel | [Project README](../README.md), [development guide](development.md) |
| Find contribution work | [Contributing](../CONTRIBUTING.md), [issues](https://github.com/waldoco/Waldo-Kennel/issues), [milestones](https://github.com/waldoco/Waldo-Kennel/milestones) |
| Know what works and remains open | [Current status](STATUS.md) |
| Understand direction | [Public roadmap](../ROADMAP.md) |
| Change product or kernel behavior | [AGENTS.md authority order](../AGENTS.md#canonical-read-order) |
| Understand the control plane | [Product architecture](product/kennel-v1-product-architecture.md), [MVP reset](product/2026-09-08-outcome-control-plane-mvp-reset.md), [technical architecture](architecture.md) |
| Work on providers or runtime | [Runtime reference index](research/2026-09-04-kernel-runtime-reference-index.md), [CLI](cli/README.md) |
| Inspect launch evidence | [PR #110 handoff](handoffs/2026-09-12-pr110-launch-fixes/HANDOFF.md), [execution ledger](handoffs/2026-09-12-pr110-launch-fixes/EXECUTION-LEDGER.md) |

## Authority and scope

[AGENTS.md](../AGENTS.md) maintains the canonical read order; this index does not
keep a competing copy. ADRs 0010–0012 establish Outcome authority and
non-authoritative owner-configured reasoning; ADR 0015 adds contract-bound
interactive planning. ADR 0008 separates contributing Outcomes from execution
WorkUnits, and ADR 0009 defines scheduling, workspace custody and effect fencing.
Earlier ADRs remain applicable inside their scope unless explicitly superseded.

The [Work flow](superpowers/specs/2026-08-25-work-control-plane-canonical-flow-design.md)
and [screen interaction](superpowers/specs/2026-08-25-work-experience-screen-interaction-spec.md)
specifications are implementation companions. Target designs do not prove
shipped behavior; use [STATUS.md](STATUS.md) for that boundary.

## Future lanes and history

The [roadmap](../ROADMAP.md) links active future work. Relevant designs include
[owner-correctable Memory](superpowers/specs/2026-08-21-home-personal-agent-memory-design.md),
[governed learning](superpowers/specs/2026-08-21-waldo-learning-skill-evolution-design.md)
and [ADR 0005](adr/0005-governed-project-learning-and-skill-evolution.md).
Memory research includes the [infrastructure benchmark](research/2026-08-21-agent-memory-infrastructure-benchmark.md)
and [personal-agent benchmark](research/2026-08-21-personal-agent-memory-research-benchmark.md).
Home, mobile, capture and hosted attachment are separately scoped work; their
presence here does not make them launch commitments.

Dated plans and verification records describe their own revisions and test
boundaries. They are not instructions to resume historical assignments. Preserve
referenced evidence and decisions; remove obsolete disconnected handoffs instead
of turning this directory into a second task tracker. Git history retains removed
material. See the [cleanup record](maintenance/2026-09-12-public-docs-cleanup.md).
