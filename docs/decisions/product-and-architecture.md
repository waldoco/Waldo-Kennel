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
