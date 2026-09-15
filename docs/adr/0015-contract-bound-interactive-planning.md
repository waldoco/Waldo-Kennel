# ADR 0015: Contract-bound interactive planning

> **Supersession note (2026-09-15):** Contract-bound planning remains accepted. [ADR 0017](0017-persistent-mission-runtime-and-bounded-supervision.md) fixes the thread topology: Contract conversation, fresh planning conversation, Supervisor thread, and persistent worker threads.


**Status:** Accepted

**Date:** 2026-09-11

## Context

A confirmed Contract is authoritative enough to define what success and permitted effects mean, but a one-shot Plan generator forces the owner to translate repository discoveries and ambiguities through repeated forms. The normal Work experience needs a short discussion with a chosen planning agent before Kennel compiles a reviewable Plan.

This discussion is pre-execution intelligence. It must not become an alternate provider session hierarchy, an implicit Contract editor, or a way to acquire execution authority.

## Decision

Kennel owns a durable, Outcome-scoped `PlanningSession` bound to one exact `ContractRevision`, one owner-selected planning provider/model semantic, and one frozen context snapshot. Its normalized owner/planner turns are a bounded product record; raw provider transcripts remain provider data.

The initial implementation uses direct Anthropic/OpenAI APIs. Repository reading means the daemon builds the existing bounded `RepositoryContextSnapshot` and sends that packet to the API. The model receives no filesystem or tool handle. Commands, writes, arbitrary network tools, commits, pull requests, deployment, and external effects are not planning grants.

The owner explicitly chooses a ready candidate. Kennel freezes that binding and rejects later configuration drift; it never silently substitutes a provider or model. `provider_default` remains an exact selection semantic even when no concrete model is frozen.

Each planner reply is exactly one of:

- a clarification;
- a proposed Contract change, which cannot mutate or reconfirm the Contract;
- a structured Plan proposal.

Every provider turn records an `IntelligenceRun`. A Plan proposal passes the existing criterion coverage, DAG, capability derivation, Contract ceiling, routing, persistence, and owner-approval gates. It creates no `Attempt` or `AgentSessionRef`. The PlanningSession records the resulting proposed `PlanRevision`; the existing approval boundary remains the only way to authorize it.

A Contract revision makes an active PlanningSession stale. Continuation fails closed and the session is superseded; the owner starts a new conversation against the new revision. Requests and turns use idempotency keys plus optimistic session revisions.

If the daemon restarts while a provider reply is outstanding, the session returns to owner control with an explicit ambiguous-reply state. Kennel does not automatically replay a request that may already have reached a billed provider. Reusing the original request key remains a read-only replay; the owner sends a new message to try again.

Native Codex planning is a dependent slice. It remains visibly unavailable until its durable conversational continuity and effect confinement are proven at the actual runtime boundary. ADR 0013's packet-only Codex reasoning path is not treated as that proof and no existing readiness gate is widened here.

## UX projection

Interactive planning lives inside Mission Control's existing **Decide & Authorize** stage, not in a new wizard or navigation hierarchy:

1. Show the confirmed Outcome/Contract summary.
2. Show only available planning-agent choices, with a short setup action for an unavailable choice.
3. Default context to **Read this repository** and explain the separate denied effects in one compact disclosure.
4. Open one inline conversation. Render clarification and Contract-change replies as short action cards, not raw structured data.
5. When ready, replace the composer emphasis with one reviewable Plan card and the existing explicit approval action.
6. After approval, the normal serial WorkUnit schedule becomes the supervision view.

The frontend renders daemon state (`status`, `waitingOn`, turns, and optional proposed Plan). It does not infer lifecycle state, choose a fallback provider, approve a Contract/Plan, or launch execution.

## Consequences

- Planning can be conversational without making an intelligence provider authoritative.
- Repository investigation is useful by default while effects stay separately governed.
- A retry cannot duplicate an owner turn or canonical Plan.
- Restart recovery is visible and recoverable without risking an implicit duplicate provider call.
- Direct APIs do not preserve native provider conversation identity; Kennel supplies normalized continuity explicitly on each turn.
- Live provider behavior, native Codex confinement, Electron usability, and owner acceptance remain separate verification gates.
