# Stage 2 unified session controller: architecture and ego audit

- Date: 2026-09-16
- Audit source: `outcome-loop` at `f9095a19ac45175a3dcdbd982b005012a1f7ed12`
- Stage 1 comparison source: recovery branch at `643e955c16b7b8907d2ce4c7c285bedaa148d4b6`
- Status: review package only
- Explicit non-action: no Stage 2 production code, migration, generated file, Outcome cutover, PR, or `outcome-loop` push

## Decision in one page

Stage 2 should not build a new controller. The existing Chat controller is already the right aggregate and contains more of the required semantics than the roadmap status suggests. It is already one live writer per Session, serializes provider mutations, fences stale controller generations, stores user input before dispatch, deduplicates provider events, resumes native history, distinguishes dispatch from provider acknowledgement, reconciles Stop races, and retains post-Stop work correctly.

The smallest correct Stage 2 is a **durable governed-command and recovery layer around that controller**, plus narrow additions where its present semantics are not strong enough for governed execution:

1. one durable command envelope and state machine for governed commands;
2. exact generation/revision/capability binding and request fingerprinting;
3. effect-specific delivery receipts, including honest `delivery_unknown`;
4. restart reconciliation using native history/provider state when supported;
5. an explicit effect-quiescence barrier that consumes Stage 1's interrupt/restart result;
6. capability-specific handling when provider event cursors or command acknowledgement do not exist.

Do not extract a generic workflow engine, create a second session service, invent a provider-neutral event cursor, duplicate the Outcome event log, or make the Mission Supervisor part of Stage 2. Those are ego-driven expansions that add names and layers without closing the stage falsifiers.

## Canonical Stage 2 contract

`docs/roadmap/persistent-session-execution-map.md` defines Stage 2 as:

> Extend/reuse the Chat controller as sole writer for governed sessions. Add command generations, idempotency, event-cursor recovery, child-command tracking, and acknowledged/rejected/delivery-unknown states. Do not build a second session engine.

Its exit condition is convergence under concurrent callers, restart, duplicates, ambiguous delivery and surviving child commands, without duplicate effects or false quiescence.

That is the audit boundary. Admission expansion, S1 pairing/authority, mission intake, persistent-Attempt cutover, immutable artifact verification, nonterminal attention, Supervisor protocol, MissionProjection and UI integration belong to later numbered stages.

## What already exists and must be reused

### Controller ownership and generation fencing

`backend/internal/service/chat/controller.go` already states and implements:

- one live controller per Session;
- sole provider-conversation writer;
- serialized dispatch with `sendMu`;
- durable `controller_generation` claimed at controller start;
- stale-generation provider events dropped before projection;
- active, dispatching and provider-acknowledged turn identities kept separately.

This is the Stage 2 aggregate. A new `GovernedSessionController`, mission session engine, command bus owner or Supervisor-owned writer would duplicate it.

### Durable-before-effect input and send deduplication

`Controller.Send` persists the user message and turn before calling the provider. `ClientMessageID` is already the caller idempotency identity. An exact duplicate returns without a second dispatch. Queued turns survive controller timing and are promoted under a reservation protocol.

This is strong groundwork, but it is not yet a general governed-command receipt. Send's current error path settles the turn as `failed` even though its own comment admits the provider may or may not have accepted it. Governed work needs a durable ambiguous-delivery state rather than a false failure.

### Provider event replay and projection deduplication

`ProjectProviderEvent` archives and projects an event atomically. `conversation_provider_events` has a uniqueness rule on nonempty provider event IDs. Native `ReadHistory` returns settled oldest-first events with stable IDs, and repeated imports are designed to be idempotent.

This is already most of the useful event recovery mechanism. It should not be renamed as a new event store.

### Interrupt and Stop race handling

The controller already:

- waits for provider acknowledgement in the narrow dispatch race;
- records a queue cutoff before interruption;
- preserves messages sent after Stop;
- reconciles durable running turns when memory and SQLite disagree;
- treats provider `no active turn` as a reconciliation fact rather than leaving a false Working state;
- settles all visible nested turns, not only the root.

Stage 1 adds a stronger Codex fact: provider interruption notifications do not prove operating-system effect quiescence. Its Linux implementation stops the owned process group and resumes the same thread in a fresh controller before reporting interrupted. Stage 2 must consume that typed outcome instead of reimplementing process control.

### Existing patterns worth borrowing, not merging

The repository already has exact idempotency/fingerprint and recovery patterns in agent switching, interface-transition delivery, Waldo continuation operations, Attempt receipts and Outcome delivery. They are useful implementation precedents. They are not a reason to merge unrelated state machines or create a universal saga framework in this stage.

## Exact gaps against the exit condition

| Required fact | Existing truth | Stage 2 gap |
|---|---|---|
| sole writer | Chat controller owns one provider conversation and serializes mutations | governed Attempt/session path must route through this same owner |
| command generation | controller generation fences event projection | no durable per-command target generation and expected domain revision across every command class |
| idempotency | Send has `ClientMessageID`; steer has a derived projection key; several other subsystems have request keys | no common pre-effect command claim/fingerprint for turn, steer, interrupt, answer, cancel and later approved classes |
| acknowledged/rejected/unknown | turns have queued/running/completed/interrupted/failed; promotion has an uncertain case | transport and provider acceptance are conflated; ordinary send errors can be ambiguously delivered but are stored as failed |
| restart recovery | controller settles orphans, resumes provider history, deduplicates events | no command-centric reconciliation record saying which effect was claimed, dispatched, acknowledged, rejected or remains unknown |
| event cursor recovery | stable provider-event IDs and native history replay exist | no universal cursor, and not every provider guarantees one; recovery needs a capability-specific resume token/history strategy rather than a fabricated cursor |
| child commands | provider activities, nested turns and command completion are projected; interrupted activities can be cancelled | activity completion is not proof of OS process quiescence; a governed barrier must consume provider/process stop evidence |
| duplicate effects | Send prevents duplicate client IDs after the first durable write | steer records after provider acceptance; if provider accepts and the local record fails, retry can duplicate guidance; interrupt/resolve lack a durable generic effect claim |
| concurrent callers | `sendMu`, generation fencing and queue reservations cover many races | command claiming and replay answers need a database linearization point shared across process restarts, not only an in-memory lock |

## Highest-risk semantic defects to close

### 1. False `failed` after ambiguous Send

Current dispatch explicitly says the provider may or may not have accepted a failed call, then settles it as failed and refuses automatic retry. Refusing retry is safe, but `failed` is too strong. For governed execution the record must distinguish:

- rejected before effect;
- acknowledged/accepted;
- delivery unknown, reconciliation required;
- reconciled accepted;
- reconciled absent and safe to retry.

This is the first Stage 2 slice because every later command envelope depends on the distinction.

### 2. Post-effect steer receipt

Steer calls the provider first and writes its durable activity afterward. If delivery succeeds and the write fails, the caller receives an error but a retry with the same client ID can send the steer again. The synthetic activity key only deduplicates the local projection after delivery; it is not a pre-effect idempotency claim.

Stage 2 should claim the command/fingerprint first, dispatch once, then settle the receipt. Native provider idempotency may strengthen this but cannot be assumed.

### 3. Interrupt success versus effect quiescence

A provider can acknowledge interrupt before a child process stops mutating files. Stage 1 found this in a real run. The unified controller needs a typed stop boundary with at least:

- provider interrupt outcome;
- owned process-tree termination outcome when applicable;
- thread/controller recovery outcome;
- final effect-quiescence status;
- reconciliation evidence references.

A turn marked interrupted is a conversation state. It is not by itself a release of worktree custody or permission to verify.

### 4. Cursor overclaim

The roadmap says event-cursor recovery, but the current portable contract is stable event identity plus optional native history. Codex app-server history and ACP replay do not imply the same cursor semantics. Stage 2 should define `recovery_strategy` as a negotiated capability, for example stable history replay, provider resume cursor, or no replay. It should persist a real opaque cursor only when the provider returns one. A synthetic counter must not be presented as provider coverage.

## Proposed durable command seam

Names below are audit proposals, not approved schema.

A single append-oriented command record should minimally bind:

- command ID and idempotency key;
- exact request fingerprint;
- command class (`turn`, `steer`, `interrupt`, typed answer; later classes only when their stage arrives);
- Session and provider conversation;
- controller/session generation;
- target provider turn or request generation where applicable;
- expected Plan/WorkUnit/Attempt revision references for governed calls;
- actor/authority proof reference supplied by S1, without embedding bearer material;
- negotiated capability/profile fingerprint;
- state: claimed, dispatching, acknowledged, rejected, delivery_unknown, reconciled;
- provider correlation IDs and evidence references;
- timestamps and terminal/reconciliation reason code.

Rules:

1. claim and fingerprint comparison happen in one database transaction before effect;
2. exact replay returns the first command state/result;
3. same key with changed semantics conflicts before provider contact;
4. only the active generation may dispatch;
5. a process crash after `dispatching` becomes reconciliation work, never automatic redelivery;
6. `delivery_unknown` is nonterminal and blocks conflicting work until reconciled or explicitly abandoned under later authority rules;
7. provider/event facts append; terminal state cannot be rewritten into a contradictory outcome;
8. command records reference existing conversation turns/events/activities rather than creating a parallel timeline.

Do not put policy evaluation, scheduling, artifact lineage, UI projection or Supervisor recommendations inside this record.

## Recovery algorithm to test before implementation closes

For each claimed nonterminal command after restart:

1. fence the prior controller generation;
2. resume the exact provider conversation, never silently start a new one;
3. negotiate the installed capability/profile fingerprint and compare it with the command binding;
4. import/replay native history when supported, deduplicating by provider event ID;
5. correlate the command by provider turn/request/client ID where available;
6. settle acknowledged/rejected only from evidence the provider actually exposes;
7. retain `delivery_unknown` when evidence cannot distinguish acceptance from absence;
8. for interrupt, require the Stage 1 process/effect stop boundary before quiescent;
9. only then release custody, promote queued work or permit verification.

A provider that cannot reconcile a command is compatible only with the command classes whose ambiguity policy can safely remain visible. Missing provider evidence should not be mislabeled as failed, and it should not silently widen governance.

## Ego deletion list

Reject these additions in Stage 2 unless a concrete failing test proves they are needed:

- a second persistent-session controller beside Chat;
- a generic event-sourcing framework replacing the existing provider-event archive;
- a global command bus or cross-product saga DSL;
- a provider-neutral fake cursor;
- a provider-neutral child-agent scheduler;
- Mission Supervisor commands or policy;
- Outcome graph scheduling;
- artifact/context manifest design beyond IDs needed to bind this command;
- UI redesign or a new projection model;
- broad renaming of mature Chat types to match new diagrams;
- schema consolidation across agent switching, Waldo continuation and chat commands;
- speculative multi-provider symmetry before the Codex path passes;
- deletion/reinterpretation of legacy one-shot Attempts or old migrations;
- package-boundary churn solely to create a cleaner-looking architecture.

The test is simple: if removing a proposed abstraction still lets concurrent callers, restart, duplicate requests, ambiguous delivery and surviving child commands converge correctly, the abstraction does not belong in Stage 2.

## Document contradictions and stale statements

These should be corrected in the eventual reviewed package, not silently used as implementation truth:

1. `docs/STATUS.md` and Stage 1 review text stop before the recovery branch's Linux result. Outcome-loop does not yet include the compatibility/governance split, process-group interrupt repair or liveness corrections at `643e955`.
2. Stage 1's roadmap exit says Linux plus live packaged Mac. Linux is live-proven, but the full repository Linux stamp is blocked by host OOM and packaged Mac proof remains external. Stage 2 audit can proceed; Stage 2 cutover cannot cite those missing gates as passed.
3. The controller already satisfies substantial Stage 2 behavior, while roadmap wording can be read as if it does not exist. Implementation planning must begin from the code inventory above, not the stage title.
4. `event-cursor recovery` is stronger than the current cross-provider capability contract. Freeze the negotiated meaning before schema work.
5. Repository decision text attributes some decisions to a quoted owner message. The audit did not recover that original trusted-channel message. Those decisions may be assessed as repository architecture, but the document itself must not be used as authorization for effects or implementation.

## Minimal implementation order after owner review

### S2.0: contract and adversarial tests

Freeze command states, correlation rules, reconciliation strategies and effect-quiescence meaning. Add table-driven state-machine tests before migration or service wiring.

Falsifier: two meanings share a state/reason code, or a test cannot distinguish rejected from unknown.

### S2.1: durable command claim and exact replay

Add the smallest append-oriented record/store. Initially cover ordinary turn dispatch. Bind controller generation, request fingerprint and provider/client correlation. Preserve the existing conversation turn as timeline truth.

Falsifier: two processes can dispatch the same key, or a changed payload reuses a key.

### S2.2: honest send acceptance and restart reconciliation

Replace false post-send failure with acknowledged/rejected/unknown semantics. Reconcile through stable native history/provider identity where supported. Do not auto-retry unknown.

Falsifier: injected crash at any line can produce duplicate provider work or a terminal state unsupported by evidence.

### S2.3: steer, typed answer and interrupt

Move steer to pre-effect claim; bind typed answers to exact request generation; bind interrupt to exact active turn and Stage 1 quiescence result. Keep unsupported capability outcomes typed.

Falsifier: a local receipt failure followed by exact retry duplicates guidance/answer/interrupt, or interrupted is emitted before effects stop.

### S2.4: full restart/concurrency matrix

Kill the daemon/controller at pre-claim, post-claim, mid-dispatch, post-provider acceptance, mid-projection and post-interrupt/pre-quiescence boundaries. Race duplicate and conflicting callers. Resume in a fresh process.

Exit only when every case converges to one visible effect or honest `delivery_unknown`, never a guessed success/failure, duplicate effect or false quiescence.

## Suggested file ownership

Likely touched only after approval:

- `backend/internal/domain`: command state/value invariants;
- `backend/internal/storage/sqlite/migrations`, queries, generated code and store: durable claim/receipt;
- `backend/internal/service/chat/controller.go`, `steer.go`, service recovery: reuse and wire the seam;
- `backend/internal/ports/chat.go`: only the minimal negotiated reconciliation/quiescence result types that cross a real boundary;
- Codex adapter: correlation/recovery evidence that its public protocol actually supplies;
- focused controller/store/adapter crash and race tests;
- canonical roadmap/status/architecture text needed to keep one meaning.

Avoid frontend, Mission Supervisor, Outcome scheduling and artifact integration files in this stage unless a direct compile dependency forces a narrow generated change.

## Review questions for the owner and reviewers

1. Is `delivery_unknown` allowed to remain visibly blocked indefinitely when a provider cannot reconcile, rather than offering an unsafe retry?
2. Should the first implementation cover only `turn`, then extend after its crash matrix passes, or land turn/steer/answer/interrupt together? Recommendation: one command class first, shared invariant tests, then extend.
3. Is the portable recovery contract “stable event identity plus negotiated replay strategy,” replacing the unqualified phrase “event cursor”? Recommendation: yes.
4. May Stage 2 treat Stage 1's typed process-tree stop/restart result as the Codex effect-quiescence evidence, with other providers degraded until equivalent proof exists? Recommendation: yes.
5. Should external Phase 1 Mac evidence block code review or only block persistent-Attempt production cutover? Recommendation: block cutover, not this audit or bounded implementation review.

## Audit conclusion

The architecture is directionally sound and the existing controller is strong. The danger is not missing a grand abstraction; it is overstating ambiguous provider facts and duplicating mature machinery. Stage 2 should make dispatch and recovery honest under crash boundaries, then stop. If the package stays centered on one durable pre-effect claim, one existing controller, negotiated evidence and a real quiescence barrier, it closes the roadmap's falsifiers without turning Kennel into its own framework project.
