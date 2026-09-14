# Waldo Kennel roadmap

This is the public roadmap for Kennel. It describes direction and exit gates,
not promised dates. [`docs/STATUS.md`](docs/STATUS.md) records what is actually
implemented on `beta`; the canonical architecture and ADRs define accepted
semantics. A roadmap item is not shipped merely because a design, branch, test,
provider session, or pull request exists.

The invariant across every milestone is:

> **The user manages Outcomes. Kennel manages the sessions required to make
> those Outcomes true.**

## Milestone overview

| Priority | Milestone | Depends on | Exit signal |
| --- | --- | --- | --- |
| Launch | One trustworthy Outcome loop | Current Outcome/Contract/Plan/Attempt foundation | A packaged real-repository journey reaches owner review/closure and survives restart |
| Next | Project understanding and source context | Launch context disclosure and revision lineage | Incremental, source-linked, owner-correctable Project Briefs work at repository scale |
| Next | Grounded Outcome suggestions | Project understanding and current Project evidence | Suggestions cite evidence, remain proposals, and improve owner prioritization |
| Next | Owner-correctable Memory | Source/candidate/admission and deletion contracts | Governed retrieval improves work without turning inference into truth or authority |
| Next | Parallel and capability-driven orchestration | Launch reliability plus ADR 0009 leases, dependency materialization and recovery | Single-provider multi-session and mixed-provider DAGs pass safety and efficiency benchmarks |
| After learning proof | Cross-session learning and learned routing | Trustworthy result labels, project learning baseline and owner promotion | A project-scoped procedure improves held-out work before any learned orchestration policy activates |
| Ongoing | Experience, accessibility and brand | Truthful daemon projections | Packaged user journeys are clear, accessible, responsive and visually coherent |

## Launch: one trustworthy Outcome loop

The launch milestone is a small, reliable loop rather than the entire future
platform:

```text
Project -> grounded Contract -> interactive Plan -> owner authorization
        -> serial WorkUnit execution -> attached provider Session
        -> retained evidence and verification -> Ready for Review
        -> explicit owner acceptance or rework
```

Both configured direct-API reasoning and the supported native Codex reasoning
path should fail clearly when unavailable. Provider identity, model provenance,
credentials, repository context limitations, and execution state must remain
truthful; there is no silent fallback.

Exit gates:

- a fresh packaged Electron profile completes the loop against a real
  repository, with frontend and daemon state agreeing after restart;
- the executing session receives the exact frozen WorkUnit, assigned Contract
  criteria, validation requirements, constraints, and authority needed to act;
- every Attempt is attached to its WorkUnit and is reachable through the
  existing Mission Control session Kanban;
- completion, evidence, verification, Ready for Review, and user Acceptance
  remain distinct states;
- credentials remain in the daemon secret store, logs and work records contain
  no secrets, and provider/API failures are recoverable and understandable;
- installation, update behavior, navigation, loading, saved-state feedback,
  keyboard use, and notification behavior pass a real desktop journey;
- release documentation identifies supported platforms, known limitations, a
  verified private security-reporting route, and reproducible contributor
  checks.

Current launch truth and known gaps belong in [`docs/STATUS.md`](docs/STATUS.md),
not in this roadmap.

### PR #110 launch checkpoint

Merged PR #110 closes several launch foundations: exact
Contract/check/context delivery in the RunBrief, strict repository-context
settings, replacement/reconciliation corrections, packaged hook/runfile and
Codex sidecar discovery, provider-correct Attempt cards, actionable attention,
and separate Plan/graph views. At its tested head `5a58434ad`, a packaged Codex
Attempt initialized through Codex App Server, emitted Kennel hooks, exited zero
and reconciled.

That canary did not complete the Outcome. Its WorkUnit exposed the validator
command but no repository-inspection capability or equivalent tool affordance,
so the agent truthfully returned `needs_you` and authored no report. The next
launch slices, in order, are:

1. derive and deliver the correct bounded repository capabilities/tool
   affordances for an approved WorkUnit;
2. produce a real artifact, run governed checks, bind WorkUnit-scoped Evidence
   and Verification, and reach separate owner review/Acceptance or rework;
3. finish automated artifact/check collection, result review and downstream
   workspace handoff;
4. complete the direct WorkUnit DAG projection and integrated Board/List
   navigation;
5. decide and implement inspection/engagement for historical sessions beyond
   the currently engageable active session;
6. publish and test the release/update path, then complete applicable mobile
   and manual reduced-motion acceptance.

The scheduler concurrency remains intentionally `1`. WorkspaceLease-based
parallel execution stays in its later milestone, and no graph should imply that
authority before it exists.

Current slice status against that list — including what #115, #176–#178 and
the protocol-negotiation slice have since delivered — is tracked with dated
evidence in the
[production-readiness program](docs/product/2026-09-14-production-readiness-program.md);
the list above is the historical PR #110 checkpoint, not live status.

### Public release readiness

Repository and release administration is part of launch readiness. This list is
an operator checklist, not evidence that unchecked items are configured:

- [x] Apache-2.0 [`LICENSE`](LICENSE) and AO-derived attribution in
  [`NOTICE`](NOTICE) are present and must remain intact.
- [x] Focused pull-request, bug-report and feature-request templates are present.
- [x] Contributor guidance points to `beta`, the authority chain, current status
  and issue-sized work without promising an unverified community schedule.
- [ ] Enable and verify a private vulnerability-reporting route, then add a
  minimal `SECURITY.md` that names only the route actually available. GitHub
  private vulnerability reporting was disabled when checked on 2026-09-12; do
  not ask reporters to publish sensitive details in an issue.
- [ ] Verify required CI checks and branch protections on `beta` and `main`
  against the documented build/test/generation gates.
- [ ] Publish and test platform-specific installation artifacts from `main`;
  verify release, update, website, support and download links from a clean user
  profile before adding them to the README or onboarding.
- [ ] Use screenshots and recordings only from the release candidate against a
  real daemon-backed state, clearly labeling any fixture/demo material.
- [ ] Complete the launch naming, icon, copy, accessibility and website/release
  asset pass without removing the required upstream provenance.

## Milestone: project understanding and source context

New Projects should become useful without asking the owner to explain the same
repository repeatedly. Kennel will build an incremental, inspectable Project
Brief from repository structure, governing instructions, selected source,
history, checks, and owner corrections.

This is bounded context compilation, not indiscriminate whole-repository prompt
injection. Source revisions, exclusions, truncation, staleness, and parsing
failures remain visible. Secret, generated, dependency, ignored, binary, and
out-of-scope material stays excluded by default.

Exit gates:

- initial ingestion and incremental refresh are cancellable, resumable, and
  measured on small and large repositories;
- every derived statement can point back to source material and revision;
- the owner can correct, supersede, exclude, or delete Project context;
- planning uses a bounded context packet and records exactly what was disclosed;
- stale or partial context cannot silently widen a Contract or execution grant.

The durable context model starts with
`ProjectBriefRevision`; richer Memory follows the separate governance gate
below.

## Milestone: grounded Outcome suggestions

Once Kennel can understand current Project sources and state, Waldo can propose
useful next Outcomes without waiting for cross-session learning. Suggestions may
come from codebase gaps, failing checks, stale documentation, accepted Outcome
follow-ups, unresolved evidence, or explicit Project direction.

Exit gates:

- every suggestion cites current Project evidence and separates observation
  from model inference;
- the owner can create, edit, dismiss or correct a suggestion, and dismissal
  does not silently become a durable preference;
- a suggestion creates no Outcome, Contract, authority or execution until the
  owner explicitly chooses it;
- usefulness, duplication, staleness and review burden are measured against a
  no-suggestion baseline.

Historical-session personalization can improve this lane later, but is governed
by the cross-session learning milestone.

## Milestone: owner-correctable Memory

Memory will preserve useful project knowledge across Outcomes while keeping
sources, inference, and owner-confirmed truth separate. Raw transcripts, model
summaries, repeated behavior, and successful sessions are observations; none
automatically becomes durable truth.

Exit gates:

- source records, candidates, admitted revisions, corrections, counter-evidence,
  retrieval receipts, expiry, revocation, and deletion have durable lineage;
- inferred or identity-shaping claims require explicit owner review;
- retrieval rehydrates canonical state and excludes stale, deleted, unapproved,
  or unauthorized candidates;
- Memory can inform a Plan but cannot create authority, evidence, verification,
  an Outcome, or Acceptance;
- correction/deletion, privacy, adversarial-source, latency, token-cost, and
  review-burden evaluations pass.

The detailed future design remains in
[`docs/superpowers/specs/2026-08-21-home-personal-agent-memory-design.md`](docs/superpowers/specs/2026-08-21-home-personal-agent-memory-design.md)
and the memory research linked from [`docs/README.md`](docs/README.md).

## Milestone: parallel and capability-driven orchestration

Kennel should choose an execution strategy for each Outcome from the user's
available, authenticated, capable harnesses and explicit preferences. One
provider may supply several isolated sessions; multiple providers may be mixed
when the handoff cost is justified. Small work should remain a single session.

Mission Control will show the approved WorkUnit DAG and its real dependency and
scheduler state. The existing Kanban beneath it remains the direct view of all
attached Attempts/Sessions across providers. The graph must never imply
parallelism the daemon cannot safely perform.

Routing preferences must distinguish existing harness/subscription access from
separately billed API use. Kennel may prefer an authenticated local harness when
the owner asks it to conserve API spend, or use a configured API when explicitly
allowed. It must report unavailable or unknown capability/quota information
rather than guessing subscription limits, prices, or remaining usage.

Exit gates:

- read-only branches may run concurrently within explicit resource budgets;
- independent writes use durable, isolated `WorkspaceLease`s and Git worktrees;
- dependency materialization, integration custody, cancellation, retry,
  cleanup, and restart reconciliation are deterministic and inspectable;
- consequential external effects have separate authority and idempotency fences;
- Codex, Claude Code, OpenCode, Cursor, and Pi are admitted by tested
  capabilities rather than provider-name rules;
- provider switches create attributed Attempts and bounded handoff packets;
- single-provider, multi-session and mixed-provider benchmark scenarios report
  verified completion, elapsed time, token/API cost, retries, context overhead,
  user interventions, and recovery behavior.

ADR 0009 is authoritative for scheduling, workspace and effect semantics.

## Milestone: cross-session learning and learned routing

With owner consent, Kennel can normalize bounded history from the coding
harnesses the user already has and learn project procedures, recurring context
omissions, failure patterns, and useful next actions. Paxel is a clean-room
inspiration for attributable episode analysis; it is not a source of private
algorithms or a reason to build opaque user/provider scores.

The accepted sequence from ADR 0005 is:

```text
Observer -> Experimenter -> Governor
```

The first active proof is one project-scoped procedural skill. Context rules
and learned orchestration-policy optimization follow only after that proof.
Capability-driven routing based on explicit machine facts does not need to wait
for learned optimization.

Exit gates:

- imported history is consented, minimized, attributable, scoped, revocable,
  and deletable without requiring raw transcripts as canonical Memory;
- learning produces candidates with evidence and uncertainty, never automatic
  activation;
- a locked evaluator compares a baseline, held-in, held-out, adversarial, and
  recovery scenarios under explicit budgets;
- the owner promotes, scopes, rolls back, or revokes the exact learned revision;
- historically personalized suggestions cite the learning evidence and explain
  what changed beyond the source-only suggestion baseline;
- learned routing must beat the explicit capability-driven baseline without
  weakening authority, privacy, reliability, or cost limits.

See [ADR 0005](docs/adr/0005-governed-project-learning-and-skill-evolution.md)
and the [learning design](docs/superpowers/specs/2026-08-21-waldo-learning-skill-evolution-design.md).

## Ongoing lane: experience, accessibility and brand

The product should make supervision calm and compact: clear transitions while
Waldo is reasoning, immediate saved-state feedback, useful empty/error states,
legible graph navigation, fast movement between Outcome, WorkUnit and Session,
and progressive disclosure of technical detail. Visual identity, motion,
illustration, website/release assets, accessibility, performance and responsive
behavior evolve alongside the milestones above and must not fabricate backend
state.

Exit gates are journey-based: keyboard and screen-reader operation, contrast and
reduced-motion behavior, large-graph usability, understandable failure recovery,
and measured interaction performance in the packaged desktop app.

## Later lanes

Hosted Waldo attachment, cross-device continuity, mobile, ambient capture,
health data, integrations, marketplace/team learning, and broad personal-agent
features remain separately governed future work. Their existing ADRs and
research do not make them launch commitments. Each requires explicit custody,
consent, threat-model, deletion, evaluation, and release gates before it can
change product claims.

## How roadmap work enters the repository

Roadmap milestones are deliberately larger than pull requests. Contributors
should turn one falsifiable slice into an issue, read the current authority
chain, branch from current `beta`, implement backend truth before dependent UI,
and attach the relevant verification evidence. Maintainers update
[`docs/STATUS.md`](docs/STATUS.md) when integrated behavior changes; they update
this roadmap only when direction or milestone gates change.
