# Product amendment: clarification, native Codex execution, and budget posture

**Status:** owner-directed plan amendment, documentation only. Implementation is not started by this change.

**Owner direction:** September 15, 2026. This amendment updates the canonical W1 plan. It does not reopen accepted evidence or rewrite stored history. Where it conflicts with an accepted W1.0-W1.2 contract, the compatibility work below must land as a new reviewed migration and contract version.

## Decisions

### Clarification is a short dialogue, not a one-question gate

Kennel asks a small batch of related load-bearing questions, then follows up in bounded rounds until it can produce a coherent Contract and Plan/orchestration map. It still grounds answers available from the project, explains why each requested answer matters, records answer provenance, and does not create Plan authority before clarification is sufficient.

The first planning turn starts automatically after Outcome capture. A synthetic owner-waiting kickoff is not a valid substitute for a real intelligence request. A round may return another compact related batch when an answer exposes a new load-bearing dependency. The UI shows which answers are still open and which Contract or Plan element each answer affects.

Clarification rounds are bounded by a reviewed policy. If those rounds end while any load-bearing answer remains unresolved, the Outcome stays in Clarify with owner attention required and approval disabled. Kennel does not fill the gap by assumption, freeze a partial Contract as sufficient, or create Plan authority automatically.

### Codex uses a native, open-ended coding profile

The first complete execution path is a native Codex coding session in an isolated worktree. It may inspect and edit repository files, use ordinary development commands, maintain context, receive steering, interrupt, resume, and perform controlled worker/session orchestration. Kennel should preserve useful native Codex session behavior rather than reduce it to one-shot prompt execution.

"Open-ended" describes coding affordances inside the assigned isolated worktree. It does not remove Kennel's lifecycle, custody, evidence, provenance, owner authorization, or external-effect boundaries. The daemon still owns Contract and Plan revision authority, WorkUnit/Attempt identity, worktree lease and fence, launch/recovery truth, Result evidence, and owner Accept. A provider cannot authorize a Plan, expand the Outcome, release successors, claim acceptance, or silently perform an owner-controlled external effect.

Relevant development commands include ordinary project work such as editing, inspecting, building, testing, linting, and using local Git and project tools inside the assigned worktree. This does not create a special destructive revert feature. Push, publish, deploy, pull-request creation, release, remote mutation, and other owner-controlled external effects are excluded from the native coding profile unless separately authorized through the applicable owner-command boundary. Destructive rollback remains an explicit owner action under the existing authority and custody boundaries.

### Budgets guide execution and prevent runaway work

Budgets are explicit planning, observability, and runaway-protection controls. Kennel shows time, token, retry, and progress use when supported; warns as the plan approaches its intended budget; and never runs silently without a bound or owner-visible posture.

A budget may contain:

- an intended target used for planning and warnings;
- an explicit owner hard limit, when the owner chooses one;
- a reviewed emergency runaway ceiling owned by product policy;
- an unsupported-accounting marker when the harness cannot report a metric truthfully.

An intended target is not a terminal stop. Kennel must not invent aggressive hard defaults merely because a budget field exists. Hard-stop machinery runs only for an explicit owner limit or reviewed emergency ceiling. Exact production targets, warning thresholds, and emergency ceilings remain unresolved and must not be invented in code or documentation.

## Accepted contracts this supersedes

| Accepted contract/invariant | Amendment | Required compatibility work |
|---|---|---|
| Experience contract: "asks one load-bearing question at a time" | Small related batches with follow-up rounds | Version the clarification request/response shape for multiple questions, ordered answers, partial answers, and follow-up lineage. Keep legacy single-question records readable as one-item rounds. |
| W1.0/W1.1 admission: every executable WorkUnit has resolved hard wall-time/token/retry limits; missing budgets and above-ceiling values fail admission | Every WorkUnit has an explicit budget posture, but planning targets do not imply hard stops | Version `ExecutionBudget`, `AdmissionPolicy`, `ApprovedExecutableSpec`, digests, and reason vocabulary to distinguish target, owner hard limit, emergency ceiling, and unsupported metric. Preserve v1 digest verification and historical receipts. New proposals use v2 only after migration. |
| W1.2: wall/token limit always claims a terminal stop; retry allowance is always a hard execution cap | Only explicit hard limits or emergency ceilings stop; targets warn | Keep accepted durable stop, machine-proof, terminal evidence, and lineage accounting for hard limits. Add warning/target events and projection states. Do not reinterpret old stop records. Decide retry semantics explicitly in the v2 budget contract before implementation. |
| Admission requires requested permissions to map exactly to a supported provider sandbox; unsupported capability fails closed | A named native coding profile grants a reviewed broad repository-development envelope instead of arbitrary per-command capability combinations | Add a versioned execution-profile identity and digest to Plan, admission receipt, executable spec, launch packet, and runtime policy. Legacy exact-grant specs remain verifiable/readable. New native-profile proposals must be replanned; no migration widens an already approved Plan. |
| An Attempt cannot gain permissions or sandbox access beyond its authorized WorkUnit | The WorkUnit authorizes a native coding profile rather than a narrow command list | Preserve the invariant at profile level: the Attempt cannot exceed its exact profile, worktree, Outcome, external-effect, and orchestration envelope. Profile changes invalidate admission and require owner review. |
| Daemon-run approved checks do not grant provider `worktree.exec` | Native coding execution may run development commands | Keep acceptance/evidence checks distinct and daemon-attributed. Provider-run development commands are activity, not daemon proof. The check runner remains the source of governed criterion evidence. |
| W1.1/W1.2 current Codex execution can be represented as exact one-shot spawn plus observations | Native session create/send/steer/interrupt/resume is primary | Extend the adapter contract and durable event model before UI claims interaction. Preserve launch packet, session identity, custody, and crash ambiguity rules. |

S1 owner-command authentication, W1.3 replacement lineage, immutable decisions, atomic fence transfer, stop-proof requirements, and Result acceptance are not superseded.

## Migration and compatibility sequence

1. Add versioned clarification rounds and migrate readers before changing intake behavior. Existing one-question episodes decode as one-question rounds.
2. Define the native Codex execution profile as a typed, versioned contract. Inventory its repository operations, development commands, session controls, orchestration limits, external-effect exclusions, and platform behavior. Do not use a wildcard string as authority.
3. Add profile identity to admission/spec/launch/runtime digests. Preserve all v1 verification paths and require a fresh Plan revision to move to the native profile.
4. Define budget v2 with separate target, hard-limit, emergency-ceiling, and unsupported fields. Add new reason/event codes rather than changing the meaning of accepted codes. Historical v1 hard budgets and stop evidence retain their original meaning.
5. Add interactive transport and durable requested/acknowledged/unknown states for send, steer, interrupt, resume, and stop. Every interactive owner command passes S1 authentication, binds the exact Outcome/Contract/Plan revision, native-profile version, Attempt and session, and carries idempotency/replay protection. A UI request receipt is not provider acknowledgement.
6. Redirect the W1.4 projection and W1.5 surfaces only after contract and migration tests pass. Shadow or fixture-compare old and new readers where practical.
7. Prove the single native Codex loop in W1.6 before enabling parallel WorkUnits or a Mission plugin execution entry.

## Dependency-map additions

- **Before W1.4 schema freeze:** clarification-round contract; execution-profile contract; budget-v2 contract; interactive command/event vocabulary.
- **W1.4:** project requested, acknowledged, reconnecting, unknown, last-event age, and owner-attention states from durable records. One next action remains canonical. Interactive commands project their authenticated command identity and exact revision/profile/Attempt/session binding without exposing secrets.
- **W1.5 Ask:** immediate acknowledgement, automatic first planning turn, compact related question batches, partial answer state, follow-up rounds.
- **W1.5 Watch:** live send/steer/interrupt/resume controls; requested versus acknowledged stop; reconnecting/unknown state; last-event age; jump to latest; no animation-as-progress.
- **W1.5 Approve:** native-profile summary and meaningful external-effect boundaries; budget targets, warnings, and explicit hard limits shown separately.
- **W1.6:** packaged restart matrix at clarification, packet-ready, provider-starting, running, interactive command in flight, needs-you, Result, and Accept; no duplicate commands/effects; UI-reference source and license ledger.
- **After the single loop is proven:** parallel WorkUnit release and Mission plugin implementation. Before then, research may inventory adapter/plugin contracts, licensing, and non-overlapping fixtures only.

## Evidence gates added

- Multi-question ordering, partial answers, follow-up causality, stale rounds, restart, automatic-first-turn, and bounded-round exhaustion tests. Exhaustion with any unresolved load-bearing answer must preserve Clarify/needs-owner-attention and disable approval.
- Real Codex worktree edit and development-command canary, plus steer, interrupt, resume, reconnect, and lost-acknowledgement cases. Every send/steer/interrupt/resume/stop path includes valid S1 command coverage and forged, stale-revision, wrong-profile/session, and replayed-command negatives proving no provider mutation.
- Budget target warnings that do not stop; explicit hard limits that use the accepted durable stop protocol; unsupported token accounting without fake precision; no silent unbounded posture.
- Packaged screenshots for requested/acknowledged stop, reconnecting/unknown, last-event age, and jump-to-latest behavior.
- A source/license record for every absorbed UI reference before code or assets are copied.

## Deferred and unresolved

- Exact production budget targets, warning thresholds, emergency ceilings, and retry posture.
- Exact native-profile development-command envelope and controlled spawning limits.
- Parallel WorkUnit execution and Mission plugin implementation, until W1.6 proves one native Codex loop.
