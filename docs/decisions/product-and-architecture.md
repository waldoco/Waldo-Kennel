# Product and architecture decision log

This log records choices most likely to regress. Accepted ADRs retain deeper rationale. Update this file only when the product contract or core architecture changes.

## D1. One promise, four moments

**Decision:** Kennel promises a finished, verified software change without agent babysitting. The user experiences Ask, Approve, Watch, and Decide.

**Why:** The nine internal crossings are necessary for correctness but become product ceremony when exposed as nine chores.

## D2. Outcome and WorkUnit are the product objects

**Decision:** Outcome is the responsibility. WorkUnit is the operational card. Attempt and provider session live in lineage/detail.

**Why:** Sessions are replaceable execution mechanisms. Promoting them loses Contract, dependency, authority, recovery, and proof context.

## D3. One Go-owned control plane

**Decision:** The Go daemon owns lifecycle, policy, scheduling, events, evidence, and Result. Desktop, CLI, and plugins are projections and command surfaces.

**Why:** Multiple authorities create contradictory states and unsafe effects.

## D4. One shared admission verdict

**Decision:** Contract/Plan reasoning, approval, routing, and Attempt start consume one immutable, freshness-bound AdmissionVerdict.

**Why:** A Plan that becomes impossible only at Attempt start wastes trust, time, and tokens.

## D5. Local harnesses first

**Decision:** Authenticated local harnesses are the primary intelligence and execution path. Codex is first. An owner-supplied OpenAI key is explicit Contract/Plan fallback only, never silent WorkUnit execution.

**Why:** Kennel should preserve native account, skills, plugins, and session power while adding governance above it.

## D6. Desktop and plugin parity

**Decision:** An Outcome invoked from a plugin and one started in the desktop enter the same daemon-owned loop.

**Why:** Entry point must not create a second product, authority path, or scheduler.

## D7. Durable typed events and real recovery

**Decision:** The mission view derives from durable canonical records and typed events. Retry/replacement creates attributed Attempt lineage atomically or fails visibly.

**Why:** Receipts, cached fences, and provider text cannot establish current truth.

## D8. Evidence before acceptance

**Decision:** Daemon-run checks and evidence map to Contract criteria. The owner alone accepts.

**Why:** Activity and model confidence are not proof, and proof is not owner approval.

## D9. Ponytail is a review rubric

**Decision:** Use minimum-correct-change discipline: trace callers and lifecycle, prefer deletion/reuse, avoid speculative abstractions and formatter churn, and comment only on non-obvious invariants, safety, or deliberate tradeoffs. Do not import Ponytail runtime skills, hooks, prompts, modes, or plugin machinery.

**Why:** Simplicity should reduce implementation risk without creating another framework or weakening correctness, security, performance, accessibility, or approved evidence.

## D10. Extend after the core is dependable

**Decision:** More providers, parallel orchestration, portable memory, teams, and richer channels extend the same loop only after their dependencies are proven.

**Why:** Future product value should remain visible without entering or weakening the core path prematurely.

## D13. Resolved execution budgets carry policy provenance

**Decision:** Every proposed WorkUnit carries an immutable resolved execution budget with source and policy id/version/digest. A named, versioned daemon policy owns defaults and ceilings. Missing budgets reject rather than borrowing hidden runtime defaults; token precision is omitted when negotiated accounting is unsupported, while wall-time and retry bounds remain required.

**Why:** Replacements must not reset lineage limits, and approval must show the exact effective values it authorized. Production default and ceiling numbers remain intentionally unresolved until the reviewed policy table lands; W1.1 contains no invented numeric defaults.

## D14. Persistent Codex sessions are the vNext execution substrate

**Decision:** One vNext WorkUnit Attempt owns one exclusive worktree lease, one Kennel Session, and one persistent primary Codex app-server thread with many turns. A turn or process exit is not completion.

**Why:** The app-server already supports native persistence, steering/interruption, and resume. One-shot was an implementation choice that broke healthy interaction and recovery.

## D15. Mission Supervisor intelligence is coupled through typed authority

**Decision:** One Mission Supervisor thread per active Plan revision interprets mission events and may automatically send approved, low-risk, in-scope steering/context. The daemon validates its typed commands and remains the only authority, scheduler, state, custody, check, and projection layer.

**Why:** Missions need higher-level intelligence without making graph mutation, permissions, or durable state probabilistic.

## D16. Context is a versioned artifact graph

**Decision:** Attempts consume hashed input manifests and produce structured output manifests. Plan edges pass exact verified artifacts/decisions, never private transcripts or hidden reasoning.

**Why:** Explicit versions and provenance prevent stale coupling, accidental disclosure, and summaries becoming false authority.

## D17. Attention and completion retain session continuity

**Decision:** `needs_you` is nonterminal and resumes the same thread. Completion begins with `ready_for_verification`, daemon checks, and owner Accept. Replacement is explicit or follows proven irrecoverability.

**Why:** Healthy waiting and bounded rework are part of one Attempt; replacement should not be a conversational primitive.

## D18. Serial Codex proof precedes concurrency and provider breadth

**Decision:** Build graph/worktree safety now, run the first packaged proof serially, then enable independent concurrency. Other providers follow protocol evidence from Codex.

**Why:** One complete loop removes more risk than speculative abstraction and parallel orchestration.

### Decision provenance

D14-D18 follow the owner's authenticated reply received Tuesday, 2026-09-15 at 1:41:46 PM IST: "I think I agree with all 12 recommendations that you have given," "The supervisor should be allowed to do that and not wait on the user," and "lock all these 12 and update all the documentation accordingly." Source: inbound WhatsApp message `wamid.HBgMOTE3NTU4NjU5OTMxFQIAEhggQUMzMEFDMzFBQ0M2NzFGREEzQTZEMDNGN0I2MTVDRjUA`. This exact source ID came from the live owner channel; observation indexing lag is not negative evidence. External projects, UI references, provider protocol observations, and research notes are supporting evidence only; none grants product or runtime authority.
