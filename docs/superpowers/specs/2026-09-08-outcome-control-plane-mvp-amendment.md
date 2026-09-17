# Outcome Control Plane MVP Amendment to Canonical Flow

> **Superseded for vNext (2026-09-15):** Historical MVP amendment; not current topology or order. Use [persistent mission runtime](../../architecture/persistent-mission-runtime.md) and the [persistent-session execution map](../../roadmap/persistent-session-execution-map.md). Retained for provenance.


- **Status:** Accepted amendment
- **Date:** 2026-09-08
- **Amends:** `2026-08-25-work-control-plane-canonical-flow-design.md`
- **Authority:** ADR 0010 and ADR 0011

## Why this amendment exists

The August 25 canonical-flow design correctly rejected hidden LLM authority, but one restriction is now too strong for the desired product: it treated planner/coordinator LLM activity as valid only inside an execution `Attempt`.

That rule forces Contract understanding and Plan drafting to masquerade as worker execution. The current intake implementation demonstrates the problem by spawning an ordinary worker session/worktree solely to propose a Contract.

The product needs model intelligence **before** execution authority exists, without allowing that intelligence to become execution authority.

## Superseding rule

Replace any requirement equivalent to:

> planner/coordinator model work may only occur inside an Attempt

with:

> model-backed analysis/planning may run before Plan approval as a bounded, durable, non-authoritative `IntelligenceRun`. An IntelligenceRun proposes structured data and records provenance. It cannot create execution authority, create an Attempt, grant capabilities, perform external effects, or accept an Outcome. Execution remains gated by an approved PlanRevision and exact WorkUnit binding.

## Canonical distinction

```text
IntelligenceRun
  contract analysis / clarification / plan draft
  ↓ structured proposal only

ContractRevision / PlanRevision
  canonical reviewed facts
  ↓ owner confirmation / approval

Attempt
  authorized execution of one WorkUnit
  ↓
AgentSessionRef
```

`IntelligenceRun` and `Attempt` are siblings under different authority regimes; one is never a synonym for the other.

## Effect boundary

Pre-authorization intelligence defaults to read-only context access.

A prompt instruction such as “do not modify files” is not sufficient enforcement for a coding-agent runtime. Where a harness cannot prove an appropriate read-only/effect-limited mode, use a direct intelligence adapter with bounded context or the explicit deterministic/manual floor.

A provider-native process used to service an IntelligenceRun may be retained as provenance/reaping metadata, but it must not be persisted or rendered as the Outcome's execution `AgentSessionRef`.

## What remains unchanged from the original spec

The following August 25 principles remain authoritative:

- Outcome/Contract/Plan/WorkUnit/Attempt lineage is canonical;
- models propose; deterministic policy validates and authorizes;
- no model output is trusted merely because a model produced it;
- authority is explicit and narrow;
- retries/recovery preserve lineage;
- evidence/verification remain attributable;
- user acceptance remains distinct from provider completion;
- provider/runtime internals do not become the product domain model.

## MVP sequencing amendment

The first usable vertical slice is allowed to serialize approved WorkUnits while preserving truthful state and authority. Full WorkUnit DAG concurrency and WorkspaceLease scheduling remain the accepted target under ADR 0008/0009 and follow once the end-to-end Outcome loop is proven.
