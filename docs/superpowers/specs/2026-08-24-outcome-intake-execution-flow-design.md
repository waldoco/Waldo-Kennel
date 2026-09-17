# Natural-language Outcome intake to session execution

> **Historical evidence:** retained for provenance. It is not current architecture or implementation order; use the [canonical documentation map](../../README.md).


- **Status:** Approved written specification; implementation planning pending
- **Date:** 2026-08-24
- **Scope:** Codex-first Work flow from natural-language intent through contextual contract, orchestration proposal, authorization, and linked session execution
- **Implementation status:** Not shipped; this document does not authorize merge, push, release, or deployment

The approved [Work control plane and canonical user flow](2026-08-25-work-control-plane-canonical-flow-design.md) is the integration authority for board-first navigation, persistent Project conversation, stable-core adaptive Contracts, capability-based Project defaults and Mission roles, bounded session continuation, Mission Control, Session Inspector, and governed Project learning. This document remains the narrower first executable natural-language intake slice.

## 1. Decision

Kennel will implement one complete Work flow:

```text
Natural-language intent
  -> contextual contract draft
  -> user correction
  -> orchestration proposal
  -> explicit authorization
  -> session execution
```

The first implementation may execute through the Project's admitted default harness, with Codex remaining the recommended ready-by-default path. Its durable contracts remain provider-neutral and collection-shaped so later separately reviewed changes can add multiple agents, multiple Work Units, dependency topology, Claude Code, DeepSeek, and other admitted providers without replacing the Outcome lineage. The actual role/provider assignment is shown in the Mission proposal and never silently falls back.

“Keep it simple” applies to the first analyzers and orchestration policy. It does not remove a stage, merge authorization into inference, or return to a session-first task composer.

## 2. Product invariants

1. The initial statement creates an `IntakeSession`, not an `Outcome`.
2. Workspace or session analysis may propose contract fields; it cannot create canonical responsibility.
3. The user can inspect and correct every proposed contract field before confirmation.
4. Confirming the draft creates one `Outcome` and immutable `ContractRevision 1` through the daemon.
5. The orchestration proposal is a versioned `PlanRevision`, not model prose or a provider session.
6. Authorization is an explicit user decision bound to the current Contract and Plan revisions.
7. Session execution begins only after authorization and successful provider admission.
8. Execution creates `Attempt -> AgentSessionRef` lineage. A provider session is never the Outcome.
9. An approved plan with a failed or ambiguous start is displayed as approved plus Action Required or Unconfirmed; it is never displayed as running without confirmed session identity.
10. Electron remains a thin daemon client. SQLite, analyzers, provider admission, and execution wiring stay behind typed daemon APIs.

## 3. User experience

### Project entry

Hovering or keyboard-focusing a Project row reveals an accessible `+` action. Its tooltip and accessible name are **New Outcome**. It is separate from the existing Project menu and orchestrator control.

Activating it opens a compact modal scoped to that Project:

- heading: **What would you like to make true?**
- one natural-language text area;
- the selected Project name;
- primary action: **Analyze outcome**;
- cancel action;
- no contract fields, agent controls, topology, or permissions yet.

Submitting creates a non-canonical intake. The UI moves to the Work lifecycle route immediately and shows recoverable progress rather than holding the modal open for a model response.

### Contextual contract draft

The Understand stage renders these states:

- **Analyzing:** Kennel identifies the workspace and context sources being considered.
- **Needs clarification:** one material question at a time, only when its answer changes meaning, success, timing, authority, or review.
- **Draft ready:** the existing contract fields are populated with proposed title, goal, criteria, review, constraints, and non-goals.
- **Analysis unavailable:** the original intent remains editable and the user may retry or fill the contract manually.

Each populated section is visibly a suggestion until confirmation. The user may edit fields directly or select **Re-analyze draft**. Re-analysis uses the original intent plus current user edits as higher-precedence guidance and creates a new proposal revision. It does not overwrite an already saved ContractRevision. If the visible draft has unsaved edits, the UI confirms before replacing it with a newer proposal.

Material questions come from the analysis result. The current hard-coded “today” question becomes a fixture/test case, not an unconditional field.

Selecting **Save contract** atomically confirms the current proposal, creates the Outcome and ContractRevision, and advances to Decide & Authorize.

### Orchestration proposal

Kennel automatically prepares a proposal from the confirmed Contract. The first policy proposes one smallest-sufficient Codex Work Unit using the Project's configured Codex agent and model. The data model and API continue to expose arrays of Work Units, dependencies, and agent assignments.

The Mission projection shows, even for one Work Unit:

- the intended output;
- assigned provider, agent, and model;
- dependency and workspace ownership;
- required capabilities and consequential effects;
- Evidence requirements;
- Verification method;
- stop and recovery conditions;
- known cost or an explicit unknown-cost label.

The user can return to Understand and revise the Contract. Doing so makes the old proposal stale and requires a new PlanRevision. The first version does not provide arbitrary graph editing; it provides a trustworthy proposal and revision boundary.

### Authorization and execution

The primary action is **Authorize and start**. It is one user gesture but two durable transitions:

1. approve the current PlanRevision against the current ContractRevision;
2. request execution admission for its first Work Unit.

The daemon validates Codex availability, authentication, mode, model, workspace ownership, capabilities, and authority. It then creates an Attempt, compiles the frozen RunBrief, starts or delegates the Codex session, confirms provider-native session identity, and records an AgentSessionRef.

On success, the UI opens the running session and the dashboard shows the Outcome with its linked Plan, Attempt, and Session. On startup refusal, the approved Plan remains inspectable and execution displays Action Required. On ambiguous startup, the Attempt is Unconfirmed and must reconcile before retry; the UI must not offer an unsafe duplicate start.

## 4. Context policy for the first version

The analyzer receives the smallest available Project-scoped packet:

1. the user's current intent and explicit edits;
2. Project identity, configured agents, and workspace path;
3. repository rules and high-signal entry documents such as `AGENTS.md`, `README.md`, and the docs index when present;
4. a current read-only workspace snapshot sufficient for the analyzer to inspect the repository;
5. recent Project session identity, title, kind, activity, and timestamps;
6. provider-produced or daemon-held session summaries only when they already exist and can be attributed.

The first version does not build a Paxel-style longitudinal analyzer, scrape every raw transcript, infer personal memory, or retrieve cross-Project context. Missing summaries are omitted and disclosed; they are not reconstructed silently. The Codex analyzer may inspect the current workspace read-only, but it receives no write, commit, network, deployment, or external-effect authority.

Precedence is:

```text
current user intent and edits
  > explicit Project policy and repository rules
  > current workspace facts
  > attributed prior-session summaries
  > session metadata
```

Lower-precedence context may fill gaps or raise a clarification. It cannot override the user's statement.

A later Paxel-style pipeline may provide normalized Project episodes, decisions, unresolved questions, and approved knowledge through the same context compiler. Those records remain provenance-bearing context and never become the Outcome, Contract, Plan, Evidence, or Acceptance by themselves.

## 5. Daemon architecture

### Intake service

Add a daemon-owned intake service with an analyzer port. The service owns lifecycle, idempotency, revision checks, context compilation, proposal validation, and confirmation. The Codex adapter performs model analysis behind that port.

The renderer never sends prompts directly to a provider and never parses marker text from a transcript.

The accepted product ontology supplies the durable concepts:

- `IntakeSession` owns Project, source intent, status, material clarification, and current proposal revision;
- `ClarificationRequest` records one question, reason, recommendation, choices, and deferral consequence;
- `ResponsibilityProposal` contains the proposed Outcome Contract and source receipt;
- confirmation creates the canonical `Outcome` and `ContractRevision`.

Recommended Intake states are `captured`, `analyzing`, `needs_user`, `ready`, `confirmed`, and `analysis_failed`. Cancellation is an explicit disposition rather than deletion. Proposal revisions are immutable so restart, retry, and user correction do not erase what was previously suggested.

### Planning service

Extend the Plan proposal seam to consume the current ContractRevision, Project agent inventory, provider capability facts, and the intake context receipt. The planner returns structured provider-neutral Work Units and assignments. The daemon validates domain shape, capability names, dependencies, authority, and revision binding before persisting a proposed PlanRevision.

The Codex-first policy may cap accepted proposals to one schedulable Work Unit while preserving the collection-shaped contract. Provider choice and topology are policy inputs, not strings trusted from model output.

### Execution service

Add the missing approved-Plan-to-execution seam. It must:

1. re-read current Outcome, Contract, and Plan facts;
2. refuse stale or unapproved Plans;
3. perform live Codex admission;
4. create an Attempt before requesting provider work;
5. compile and persist provider-neutral and Codex-specific RunBrief digests;
6. start the provider through the existing session/runtime boundary;
7. confirm and persist AgentSessionRef correlation;
8. expose running, refused, failed, and unconfirmed facts to dashboard projections;
9. make retry create a replacement Attempt rather than overwriting history.

The existing legacy `/orchestrators/delegate` behavior may be reused behind this service as an adapter seam, but the Outcome flow must not rename an orchestrator and present that session as the Outcome. The frozen RunBrief, not the original intake text alone, is the execution input.

## 6. Typed API surface

Exact naming may follow nearby controller conventions, but the API must provide these capabilities:

- create an idempotent Project-scoped IntakeSession from natural-language intent;
- read the current Intake and proposal revision;
- answer a material clarification;
- request re-analysis against an expected proposal revision;
- confirm an expected proposal revision into one idempotent Outcome/ContractRevision;
- propose and read PlanRevisions;
- approve a PlanRevision against an expected ContractRevision;
- request execution of an approved Plan;
- read Attempt and AgentSessionRef facts needed by the dashboard.

All errors use existing daemon envelopes and request IDs. Validation, stale revisions, analyzer unavailability, provider admission refusal, and ambiguous startup receive typed error codes. OpenAPI and frontend TypeScript artifacts are regenerated together.

## 7. Persistence and recovery

New canonical data uses an additive SQLite migration, sqlc queries, the Outcome/Intake stores, and trigger-backed `change_log` events. The renderer stores only disposable presentation state.

After app or daemon restart:

- an unconfirmed Intake reopens at its latest proposal or recoverable analysis state;
- a confirmed Intake links to its Outcome and cannot create a duplicate;
- a proposed Plan remains reviewable;
- an approved Plan with no Attempt remains approved and ready for explicit start/retry;
- an Attempt with unknown provider startup becomes Unconfirmed and reconciles before replacement;
- a confirmed AgentSessionRef restores exact session navigation.

## 8. Failure behavior

- **Analyzer unavailable:** preserve intent; allow retry or manual contract entry; create no Outcome.
- **Malformed model output:** reject it at the analyzer boundary and retain the last valid proposal.
- **Concurrent edit:** return a typed revision conflict and offer reload; never silently overwrite.
- **Contract revised after Plan proposal:** mark the Plan stale and require re-proposal.
- **Codex unavailable or unauthorized:** refuse execution before claiming a running Attempt.
- **Start result unknown:** record Unconfirmed, reconcile, and disable duplicate start.
- **Session later exits or fails:** preserve Attempt/session lineage and show the smallest safe next action; do not mark the Outcome accepted or closed.
- **Dashboard refresh unavailable:** keep the confirmed mutation successful and retry the read projection separately.

## 9. Testing and acceptance

### Backend

- domain validation for Intake, proposal revisions, Attempt, and AgentSessionRef;
- store round-trip, idempotency, restart recovery, and trigger-backed CDC tests;
- service tests for context precedence, malformed analyzer output, clarification, re-analysis, confirmation, stale revisions, plan approval, provider refusal, ambiguous start, and replacement Attempt;
- controller tests for happy paths, missing arguments, typed daemon errors, request IDs, and idempotent replay;
- fake Codex analyzer and runtime adapters; no new network calls in tests.

### Frontend

- hover and keyboard access to New Outcome;
- simple modal validation and Project scoping;
- analyzing, clarification, draft-ready, manual fallback, retry, and conflict states;
- generated fields remain editable and non-canonical until Save contract succeeds;
- plan/Mission presentation and stale-contract behavior;
- explicit authorization and truthful provider-admission failures;
- dashboard linkage between Outcome, Attempt, and Session;
- all supported locale catalogs remain structurally complete.

### End-to-end proof

A deterministic fake-provider test must demonstrate:

```text
Project +
  -> natural-language intent
  -> contextual proposal
  -> user correction
  -> confirmed ContractRevision 1
  -> Codex PlanRevision
  -> explicit authorization
  -> Attempt + AgentSessionRef
  -> linked running session on dashboard
```

The same test restarts the daemon after contract confirmation and again during provider startup, proving that neither a duplicate Outcome nor an unsafe duplicate Attempt is created.

## 10. Deferred work and extension contract

This design deliberately defers:

- Paxel-style normalized longitudinal session analysis;
- raw-transcript-wide analysis and cross-Project retrieval;
- admitted personal or Project memory;
- editable multi-agent Mission graphs;
- parallel Work Unit scheduling and multi-worktree integration;
- Claude Code, DeepSeek, and other provider adapters;
- provider fallback or provider switching;
- the later Evidence, Verification, Acceptance, reopen, and Prove & Close implementation.

The next multi-agent/multi-provider change extends the Plan's Work Unit assignments and execution adapter registry. It must not change the intake confirmation rule, canonical Outcome lineage, revision binding, explicit authorization, Attempt replacement semantics, or the requirement that every provider session be referenced through an AgentSessionRef.

## 11. Completion boundary

This slice is complete when a user can start from the Project-row `+`, state an Outcome naturally, receive and correct a contextual contract proposal, confirm it, review a Codex orchestration proposal, authorize it explicitly, start a real Codex session, and see the durable Outcome and linked running session together after restart.

It is not complete if any stage depends on parsing transcript markers, if a session title stands in for an Outcome, if model output becomes canonical without confirmation, or if the UI claims execution before the daemon records confirmed Attempt/session lineage.
