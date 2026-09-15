# Kennel Work experience

The product presents one mission timeline over the [persistent mission runtime](../architecture/persistent-mission-runtime.md).

## The four moments

### Ask

The owner states a software goal. A Contract conversation clarifies desired result, constraints, authority, and acceptance criteria. Kennel freezes a Contract revision.

### Approve

A fresh `/mission` planning conversation uses the frozen Contract and project snapshot to propose a Plan, WorkUnit DAG, checks, profiles, context edges, and external-effect limits. The owner approves an exact revision.

### Watch

Mission Control shows:

1. Outcome and Contract/Plan revision;
2. graph above the WorkUnit/current-Attempt board;
3. mission Supervisor summary and recommendations;
4. attention, checks, evidence, and one true next action;
5. session activity in WorkUnit detail;
6. raw provider transcript only on drill-down.

The Supervisor automatically handles bounded, low-risk, in-scope steering and context. Scope or authority changes come to the owner.

### Decide

Workers claim readiness. Kennel runs checks. Failed checks return to the same thread for bounded rework. Verified WorkUnits form a Result. The owner Accepts, asks for rework, or approves a Plan revision.

## UX invariants

- One seamless timeline, with clear internal authority boundaries.
- `needs_you` is a question/context card, not a dead session or forced replacement.
- Waiting, running, checking, reconciling, blocked, verified, and historical states are visually distinct.
- A completed turn never appears as completed work.
- Legacy one-shot records are visible and labeled, not mixed with resumable vNext sessions.
- Reuse the existing Work shell, cards, graph, visual tokens, responsive behavior, keyboard access, and error/loading/empty states.
