# ADR 0017: persistent mission runtime and bounded supervision

- Status: accepted by owner on 2026-09-15
- Date: 2026-09-15
- Supersedes: one-shot execution as the target Outcome runtime; any reading of ADR 0010/0011/0015 that excludes persistent WorkUnit threads or a bounded mission intelligence layer

## Context

The existing governed Codex path chooses `codex exec` one-shot execution even though Codex app-server and Kennel's Chat controller already support persistent threads, multiple turns, interruption, and resume. Treating process exit as completion and replacing a healthy session for every interaction breaks clarification, steering, attention, recovery, and native harness value.

The owner accepted twelve linked decisions in the authenticated inbound WhatsApp reply received 2026-09-15 at 1:41:46 PM IST (message `wamid.HBgMOTE3NTU4NjU5OTMxFQIAEhggQUMzMEFDMzFBQ0M2NzFGREEzQTZEMDNGN0I2MTVDRjUA`): advisory mission supervision with bounded automatic steering, event-driven supervision, separate Contract and planning threads, explicit completion plus daemon checks, explicit replanning, artifact-based context, controlled native subagents, historical one-shot compatibility, precise stop/cancel semantics, serial proof before concurrency, warning-led budgets, and a graph-first product surface.

## Decision

Adopt the architecture in [persistent mission runtime](../architecture/persistent-mission-runtime.md). A vNext WorkUnit Attempt owns one exclusive worktree lease, one Kennel Session, and one persistent primary Codex app-server thread with many turns. A persistent Mission Supervisor thread supplies mission-level intelligence for the active Plan revision. The daemon remains deterministic authority and validates typed Supervisor commands. The automatic command allowlist contains only context delivery and reversible guidance. Each command binds exact Plan/WorkUnit/profile/session revisions, budget, and idempotency; uses approved facts; preserves scope, criteria, and approach; and causes no external effect. Periodic summaries come from daemon-owned durable cadence events, never Supervisor self-scheduling.

Workers exchange versioned artifacts and decisions through explicit Plan edges, never private transcripts or hidden reasoning. `needs_you` is nonterminal. Completion requires an explicit claim and daemon checks. Replacement is explicit or follows proven irrecoverability. Historical one-shot records are never resumed into vNext.

## Consequences

- Reuse and extend the existing persistent Chat/session controller rather than create another runtime.
- Separate intelligence from authority while coupling them through durable typed events and commands.
- Add context/input/output manifests, capability checks, execution-model generation, and reconciliation evidence.
- Keep the first proof serial and Codex-only; concurrency and other providers follow evidence.
- Update repository documentation and steering assets so this ADR and its linked architecture are the sole target.
