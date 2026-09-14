# Outcome loop

This is the canonical architecture for Kennel's governed Outcome lifecycle. It describes intended product semantics. [docs/STATUS.md](../STATUS.md) records which parts are integrated and proven.

## One core, several projections

The Go daemon owns the authoritative state machine, policy decisions, Attempt lineage, events, evidence, and Result. SQLite is its durable record. The desktop, CLI, and provider plugins are clients of that core. They may request actions and project state; they do not invent lifecycle state or authority.

```text
Outcome -> Contract revision -> Plan revision -> WorkUnits
        -> authorization -> Attempts -> evidence -> Verification
        -> Result -> rework or owner Accept
```

A provider session is attached to an Attempt. It is never the top-level product object and never accepts the Outcome.

## Nine correctness crossings

The product groups these crossings into Ask, Approve, Watch, and Decide. They remain distinct daemon transitions because each changes what Kennel may truthfully claim.

1. **Outcome**: record the desired result, project, initiating surface, and provenance.
2. **Clarify**: resolve only uncertainty that changes success, scope, authority, feasibility, or proof.
3. **Contract**: freeze observable criteria, constraints, non-goals, stop conditions, minimum authority, and required evidence in an immutable revision.
4. **Plan**: decompose the Contract into dependency-linked WorkUnits whose intent, permissions, outputs, checks, provider requirements, and budgets are coherent.
5. **Authorize**: admit the exact Plan against current verified capabilities, then let the owner authorize that immutable revision and its effects.
6. **Route and execute**: release only ready WorkUnits, bind each Attempt to a compatible verified harness, workspace lease, fence, budget, and approved permission set.
7. **Observe and recover**: retain typed activity, changes, checks, waiting and blockers; stop on terminal conditions; retry or replace by creating real attributed lineage.
8. **Verify and review**: run approved checks under daemon authority, collect evidence, map it to Contract criteria, and allow bounded rework without rewriting history.
9. **Accept**: the owner accepts or rejects the Result. Acceptance closes the Outcome and preserves provenance; no model or check can substitute for it.

## Authority invariants

- The owner controls external effects, Plan authorization, Result acceptance, and erasure.
- Waldo and provider intelligence may propose. They do not authorize.
- The Go daemon validates every transition and runs the approved checks.
- A Plan is approvable only when one current AdmissionVerdict says its WorkUnits are executable as represented.
- An Attempt cannot gain permissions, sandbox access, budget, or scope beyond its authorized WorkUnit.
- `needs_you`, budget exhaustion, failed checks, interruption, and replacement are explicit durable states, not optimistic UI receipts.
- Retry and replacement create Attempt lineage atomically or fail visibly. History is not rewritten.
- Evidence is attributed to the Attempt and criterion it supports.
- Desktop, CLI, and plugins display the same mission projection and issue the same valid commands.

## Intelligence and harness policy

Local authenticated harnesses are primary. They preserve the user's native account, model behavior, skills, plugins, and session primitives. Codex is the first complete path.

An owner-supplied OpenAI API key may be used only as an explicit fallback for Contract and Plan reasoning when local reasoning is unavailable or the owner selects it. It is not a silent fallback and is not a WorkUnit executor. Execution always uses an admitted harness binding.

Provider-specific behavior stays behind adapters, but the abstraction must not erase useful native semantics. A provider adapter must translate:

- verification and availability;
- capabilities and exact sandbox representation;
- session create, resume, interrupt, and replacement behavior;
- typed activity and usage events;
- result and failure provenance.

Unsupported capability is an admission failure, not a prompt suggestion.

## Desktop and plugin parity

An Outcome may begin in the desktop or from a supported provider/plugin entry point. Both create or attach to the same daemon-owned objects. The initiating surface may optimize presentation, but it cannot create a second Contract, Plan, authorization path, scheduler, or status vocabulary.

Mission Control is a projection, not a scheduler. One daemon event updates the dependency graph, WorkUnit board, and detail drawer. Dragging a card cannot rewrite lifecycle state. Valid commands include answer, approve, interrupt, retry, replace, revise, request rework, Accept, and stop, subject to current authority.

## Scheduling

The core model supports a WorkUnit DAG. A WorkUnit becomes ready only when its dependencies and admission conditions pass. Serial execution is acceptable while workspace, lease, fence, consolidation, and recovery guarantees are being proven. Parallel execution is an extension of the same scheduler, not a different architecture.

The supervisor owns dependency release, routing, consolidation, and downstream readiness. Workers cannot release their own successors or expand scope.

## Durable mission projection

The daemon derives one projection from canonical records:

```text
Outcome
  current Contract and Plan revisions
  WorkUnit DAG and dependency state
  current Attempt per WorkUnit and full lineage
  harness/session health and budgets
  typed activity, changes, checks, evidence
  blocker or one true next action
  Verification, Result, and owner decision
```

Clients consume that projection rather than deriving competing states from receipts, cached fences, provider text, or local timers.

## Related contracts

- [Admission and events](admission-and-events.md)
- [Product experience](../product/experience.md)
- [Decision log](../decisions/product-and-architecture.md)
- Accepted ADRs remain the record for narrower decisions. If an ADR conflicts with this loop, reconcile it explicitly rather than silently choosing one.
