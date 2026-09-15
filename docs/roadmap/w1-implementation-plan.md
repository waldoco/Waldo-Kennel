# W1 implementation plan: one dependable Codex Outcome loop

**Plan baseline:** `outcome-loop` at `3e5d8f45ef28cc695134882ba0f705db3525a491`
**Milestone:** make the complete Codex-first Ask -> Approve -> Watch -> Decide loop reviewable around September 18, 2026.
**Rule:** the date forces integration and decisions. It does not turn missing evidence into readiness.

This is the canonical implementation plan for W1.0-W1.6. The [September 18 milestone](launch-2026-09-18.md) is the concise status and dependency map. The [admission and event contract](../architecture/admission-and-events.md) owns detailed admission vocabulary. The [decision log](../decisions/product-and-architecture.md) owns product and architecture rationale. [STATUS.md](../STATUS.md) records observed evidence, not planned claims.

## Program contract

### Why

Kennel must make a coding harness safer and easier to operate without hiding its power. One Go-owned control plane must decide whether work is admissible, preserve exact owner authority, supervise Attempts, recover without duplicate effects, show one truthful state, prove the result, and keep Acceptance separate from machine confidence.

### Current state

- W1.0-W1.2 are accepted on `outcome-loop` at the plan baseline.
- The first W1.3 prototype is excluded. It mechanically accepted replacement commands from an unauthenticated loopback request and therefore could not prove owner approval.
- S1, the local-owner command authority prerequisite, is planned before W1.3 and is not accepted at this baseline.
- W1.4-W1.6 have existing components and historical evidence, but not one final integrated, packaged, current proof.

### Program invariants

1. Renderer text, provider output, HTTP body fields, loopback locality, CORS, and `Host` headers are never owner authority.
2. Approval rejects structurally impossible work before Plan authority exists. It causes no filesystem or process side effects.
3. Attempt start consumes the exact approved spec, rechecks current authority, then binds runtime facts without widening authority.
4. An Attempt owns one immutable identity, workspace binding, launch packet, budget lineage, and custody history.
5. Unknown effects retain custody and surface attention. Suspicion is never rewritten as failure, success, or cleanup proof.
6. A replacement is a new Attempt. It is never a mutation or continuation of its predecessor.
7. Provider completion is not Verification; Verification is not owner Acceptance.
8. All user surfaces derive from canonical durable state. Preview stores, provider prose, and component-local guesses cannot establish product truth.
9. Every accepted slice includes negative, restart, replay, and concurrency evidence appropriate to its effects.
10. No production wall-time, token, or retry defaults or ceilings are inferred. Their exact values remain an open product and operating-policy decision.

### Program dependency order

```text
W1.0 -> W1.1 -> W1.2 -> S1 -> W1.3 -> W1.4 -> W1.5 -> W1.6
                                      ^          |
                                      |----------|
                           UI may expose only accepted semantics
```

W1.5 may prepare isolated visual work against frozen contracts, but it cannot claim integration until W1.4 is accepted. W1.6 is proof of the integrated result, not a substitute for the earlier gates.

## W1.0 - freeze the admission and execution contract

### Current state

Accepted foundation at the plan baseline.

### Dependencies

Audit of `beta` at `5304d569aa2e29f0cc77d57539aa2f65a4d01484`.

### Why

Planning, approval, routing, and Attempt start previously had nearby but non-identical meanings. Implementation could not safely proceed while verdicts, reason codes, budget semantics, lifecycle terms, and approval-versus-launch facts were fluid.

### Scope

- Freeze `AdmissionVerdict`, required checks, stable reason codes, resolved execution-budget shape, and lifecycle/event vocabulary.
- Define `ApprovedExecutableSpec` as approval authority without a concrete workspace.
- Define `WorkspaceBoundLaunchPacket` as the approved spec plus exact Attempt, fence, session, canonical root, retained inputs, readiness receipts, and effective launch facts.
- Define deterministic digests and cross-validation that allow binding or narrowing but never widening.
- Map the contract to existing Outcome, Contract, Plan, WorkUnit, Attempt, routing, policy, and migration identities.

### Non-goals

No service wiring, persistence, generated API, UI relabeling, full event projection, workspace reservation at approval, or production budget numbers.

### Design and primary files

- `backend/internal/domain/admission_contract.go`
- `backend/internal/domain/execution_budget.go`
- `backend/internal/domain/lifecycle_projection.go`
- `backend/internal/domain/execution_policy.go`
- `docs/architecture/admission-and-events.md`
- `docs/decisions/product-and-architecture.md`

### Invariants and trust boundaries

Approval authority contains no runtime allocation. The approved spec can authorize only its exact attribution, capabilities, checks, binding, and budgets. A launch packet must cite that spec digest and may bind or narrow it, never widen it.

### Ordered implementation sequence

1. Inventory current meanings and persisted identities.
2. Freeze closed reason codes, verdict fields, and budget semantics.
3. Define approved-spec and launch-packet digest inputs.
4. Add intrinsic and cross-artifact validation.
5. Add deterministic mutation and duplicate-meaning review tests.

### Failure and test matrix

- missing/mismatched Outcome, Contract, Plan, or WorkUnit identity;
- unsupported reason/status/event values;
- missing or over-ceiling budgets without inventing defaults;
- concrete workspace/session/fence identity in approval authority;
- launch/spec attribution, digest, capability, check, or budget widening;
- serialization order and one-field mutation sensitivity.

### Evidence gate

Domain tests prove digest determinism, mutation sensitivity, reason-code closure, attribution, budget validation, launch/spec cross-validation, and no concrete workspace in approval authority. Review confirms no duplicate meaning against existing API, storage, or migration history.

### Rollout and rollback

W1.0 is additive domain vocabulary. Rollback removes only unconsumed types. Once later migrations persist these meanings, migration history is append-only and rollback must preserve readable history.

### Open decisions

Exact production budget defaults and ceilings remain unresolved.

## W1.1 - one admission evaluator and a crash-safe launch boundary

### Current state

Accepted at the plan baseline.

### Dependencies

W1.0.

### Why

A Plan must not look approvable through one entry point and fail as structurally impossible through another. Runtime allocation also needs a durable boundary before provider effects can exist.

### Scope

- Use one Go-owned evaluator for planning/routing eligibility, approval, and Attempt-start freshness.
- Atomically persist one admitted verdict and one approved executable spec per WorkUnit at approval.
- At start, consume the exact current verdict/spec and atomically create the queued Attempt plus exclusive project fence.
- Recheck volatile readiness, prepare the workspace, canonicalize and bind it once, persist the launch packet before provider launch, then launch that exact packet.
- Classify crashes before packet persistence, after packet persistence, and after provider launch without inventing effects.
- Fail closed on legacy/missing/tampered packets while preserving custody when a provider may exist.

### Non-goals

No richer semantic intent coherence, terminal budget enforcement, retry/replacement, complete event projection, generated API, or UI integration.

### Design and primary files

- `backend/internal/service/outcome/admission.go`
- `backend/internal/service/outcome/admission_recovery.go`
- `backend/internal/service/outcome/attempt.go`
- `backend/internal/session_manager/manager.go`
- `backend/internal/ports/admission_evaluator.go`
- `backend/internal/ports/admission_store.go`
- `backend/internal/ports/attempt_execution.go`
- `backend/internal/storage/sqlite/store/admission_store.go`
- `backend/internal/storage/sqlite/migrations/0137_admission_packets.sql`
- `backend/internal/daemon/admission_policy.go`

### Invariants and trust boundaries

The evaluator is the semantic owner. Storage atomically checks identity/currentness. The session manager may bind runtime facts but cannot widen approved capability, checks, model, or budget. Provider code receives only the persisted packet.

### Ordered implementation sequence

1. Implement one pure evaluator and stable verdict/spec persistence.
2. Route planning/routing eligibility and approval through it.
3. Add atomic Attempt/fence consumption with currentness checks.
4. Split workspace preparation from launch and persist the launch packet first.
5. Add restart reconciliation for each crash boundary.
6. Redirect every Attempt-start caller and remove semantic bypasses.

### Failure and test matrix

- stale Contract, Plan, WorkUnit, routing, mapping, or capability snapshot;
- approval with unsupported stable capability or workspace requirement;
- project-fence contention and readiness drift before provider effects;
- crash after Attempt/fence but before workspace;
- crash after workspace but before packet;
- crash after packet but before launch confirmation;
- crash after launch but before Running transition;
- canonical-root/symlink change, packet tamper, and request replay;
- legacy session with no packet and uncertain provider state.

### Evidence gate

Focused domain/service/store/session-manager tests, migration ledger and upgrade tests, `git diff --check`, and explicit exclusions. No packaged claim is made by this backend slice.

### Rollout and rollback

Migration `0137` is additive and immutable. Old records remain readable but cannot silently satisfy new admission. Rollback may stop new writers, but must not renumber/delete the shipped migration or reinterpret stored verdicts/packets.

### Open decisions

OS-specific owner-ID enforcement for the operator-managed admission policy remains follow-up hardening. Production budget numbers remain unresolved.

## W1.2 - coherent WorkUnits and terminal execution budgets

### Current state

Accepted at the plan baseline.

### Dependencies

W1.1.

### Why

A structurally valid packet can still represent impossible work if intent, capability, output authority, checks, or budgets disagree. Limits are not real if they reset on replacement or release custody before provider stop is proven.

### Scope

- Freeze `WorkUnitIntent` through persistence, RunBrief, approved spec, and runtime policy.
- Reject intent/capability conflicts, output/Contract authority conflicts, unexecutable checks, and permission gaps before authority.
- Persist provider-neutral execution usage as idempotent deltas from cumulative counters.
- Aggregate token use across the entire WorkUnit Attempt lineage.
- Require trustworthy adapter accounting before admitting token-capped work.
- Atomically enforce retry allowance before a new Attempt/fence.
- On wall/token limit, durably claim stop, prove provider termination, persist terminal evidence, mark the Attempt failed, and release custody atomically.
- Keep Running and retain custody when stop is not proven. Never auto-retry.

### Non-goals

No exact production values, no claim of token precision for unsupported adapters, no owner-approved replacement path, no Mission Control projection, and no UI integration.

### Design and primary files

- `backend/internal/domain/execution_budget.go`
- `backend/internal/domain/execution_usage.go`
- `backend/internal/domain/attempt_budget_stop.go`
- `backend/internal/service/outcome/admission.go`
- `backend/internal/service/outcome/attempt.go`
- `backend/internal/service/outcome/recover.go`
- `backend/internal/storage/sqlite/store/attempt_budget_stop_store.go`
- `backend/internal/storage/sqlite/store/attempt_store.go`
- `backend/internal/storage/sqlite/migrations/0138_work_unit_intent_and_execution_usage.sql`

### Invariants and trust boundaries

Intent, capability, output, checks, and budget are one frozen authority packet. Unsupported accounting cannot claim precision. Usage and retry limits accumulate across successor lineage. Only proven stop permits terminal budget release.

### Ordered implementation sequence

1. Freeze WorkUnit intent through persistence, RunBrief, approved spec, and runtime policy.
2. Add deterministic coherence rejection before approval.
3. Add provider-neutral cumulative-counter normalization and lineage aggregation.
4. Gate token-capped work on trustworthy adapter support.
5. Add atomic retry admission.
6. Add durable stop claims, proven machine stop, terminal evidence, and restart convergence.

### Failure and test matrix

- intent drift and legacy `legacy_unknown` refusal;
- missing/widened capabilities or outputs and uncompilable/unexecutable checks;
- accounting unsupported, duplicate/out-of-order counters, counter reset, and SQLite contention;
- exact retry replay versus exhausted lineage;
- wall/token boundary race with completion or cancellation;
- provider stop proven versus unproven;
- crash after stop claim, after machine result, and before terminal transaction;
- restart convergence without duplicate stop or released uncertain custody.

### Evidence gate

Focused domain, agent, intelligence, outcome, SQLite, and session-manager suites; adversarial budget tests; migration checks; clean diff. No full packaged or adapter-token claim is implied.

### Rollout and rollback

Legacy WorkUnits remain readable but fail fresh approval until replanned. Usage and stop records are append-only evidence. Rollback cannot reset lineage usage, reuse a retry, or delete migration history.

### Open decisions

Production default and ceiling values for wall time, token use, and retry count; which adapter first supplies trustworthy execution-token accounting.

## S1 - authenticated local-owner command authority

### Current state

Prerequisite under implementation review and not accepted at the plan baseline. The paused implementation is not part of this documentation-only package.

### Dependencies

W1.0-W1.2 authority and lineage identities. S1 unblocks W1.3 replacement approval.

### Why

The current renderer can call the loopback daemon API, but loopback locality, CORS, `Host`, and a body field such as `actorType=user` do not prove the owner approved a replacement. Another same-user process or compromised renderer content could imitate that request. W1.3 cannot attribute an owner command to a caller-supplied label.

### Scope

- Mint a high-entropy per-app-run capability in Electron main.
- Transfer it once to an app-spawned daemon over a private inherited startup channel, never argv, a secret-valued environment variable, file, run record, log, or renderer JavaScript.
- Expose a narrow context-isolated preload method that proposes only a closed replacement-decision shape.
- Validate the sender is the primary Kennel window/main frame.
- Show a native owner confirmation containing the exact Outcome, predecessor Attempt, Plan, WorkUnit, Contract revision, and run-intent generation.
- Have Electron main, not renderer JavaScript, authenticate to a loopback-only daemon endpoint.
- Persist an immutable `AttemptReplacementDecision` with authenticated app-run principal, exact semantic bindings, request key/fingerprint, and timestamp.
- Atomically verify predecessor bindings and current run/Contract generation before insertion.
- Exact replay returns the same decision; semantic reuse of a key conflicts.
- Leave replacement creation to W1.3.

### Non-goals

No general local account system, remote/mobile owner commands, provider continuation, successor creation, fence transfer, auto-retry, or claim that OS same-user isolation alone proves a human click.

### Design and likely files

- `frontend/src/main.ts`: capability mint/handoff, main-frame IPC handler, native confirmation, authenticated daemon call.
- `frontend/src/preload.ts`: narrow typed method without bearer exposure.
- `backend/internal/ownercommand/*`: startup envelope, constant-time bearer validation, principal derivation.
- `backend/internal/httpd/router.go` and a focused owner-command handler: loopback-only route.
- `backend/internal/domain/attempt_replacement_decision.go`
- `backend/internal/ports/attempt_replacement_decision.go`
- `backend/internal/storage/sqlite/store/attempt_replacement_decision_store.go`
- a new append-only SQLite migration and migration-ledger entry.
- `docs/architecture/local-owner-command-authority.md` or an equivalent accepted architecture section.

### Invariants and trust boundaries

- Renderer content never receives the bearer and cannot silently turn a proposal into approval.
- Native confirmation defaults to cancel and displays every action-driving identity.
- The daemon route is absent when no app-run authority exists and unreachable through the LAN listener.
- Authentication uses constant-time comparison and error/log output never includes the bearer.
- Persisted principal is derived from authenticated daemon state, never request JSON.
- A keep-alive daemon cannot give a later app run the old capability. Owner commands remain unavailable until a deliberate authenticated rebind/restart design is proven.

### Ordered implementation sequence

1. Add domain/port records and validation without wiring a route.
2. Add append-only persistence with immutable update/delete guards and atomic binding/currentness checks.
3. Add startup capability envelope and daemon authority object.
4. Add a route only when that authority is present; retain physical LAN blocking of `/internal/`.
5. Add Electron mint and private one-shot handoff.
6. Add the closed preload proposal and main-frame/native-confirmation handler.
7. Add negative security, replay, stale-binding, migration, startup, and IPC tests.
8. Review the complete diff before any push. Do not wire W1.3 consumption into this package.

### Failure and test matrix

- standalone daemon, missing/malformed startup envelope, unavailable stdin;
- missing, malformed, wrong, reused, or logged bearer;
- renderer/subframe/secondary-window call and cancelled native dialog;
- direct loopback, forged `Host`, forged forwarding headers, cross-origin, and LAN call;
- wrong Outcome/predecessor/Plan/WorkUnit/Contract/run generation;
- stale run intent between proposal and persistence;
- exact replay and changed-fingerprint conflict under concurrency;
- crash before decision insert, after commit, and before renderer receives response;
- app exit with keep-alive daemon and a new app run;
- migration upgrade plus immutable update/delete attempts.

### Evidence gate

Backend focused suites and migration gates; frontend typecheck and focused IPC tests; review showing the bearer appears only in Electron main and daemon private startup/auth code; an induced direct-loopback rejection; a real native confirmation screenshot in dark/light modes; packaged validation that argv, environment values, files, run records, logs, and renderer globals do not contain the bearer. No W1.3 successor may be created in this evidence.

### Rollout and rollback

Mount the command route only for an app-spawned daemon with a valid capability. Keep existing non-owner APIs unchanged. Rollback disables new decision creation but retains the migration and readable immutable decisions. Do not delete, renumber, or reinterpret persisted decisions.

### Open decisions

- Whether a future persistent-daemon rebind uses an owner-only local socket, an OS credential check, or an explicit daemon restart. For W1, fail closed rather than infer authority.
- Whether S1 later generalizes to pause/cancel/accept. W1 scopes it to replacement decisions only.

## W1.3 - terminal `needs_you` and atomic replacement

### Current state

Blocked on accepted S1. The earlier mechanically working but unauthenticated prototype is excluded.

### Dependencies

W1.1, W1.2, and accepted S1.

### Why

A provider that explicitly needs owner input has stopped making autonomous progress. Keeping its Attempt indefinitely Running is false, but killing it and releasing custody before durable evidence and proven stop is unsafe. Replacement must produce one attributed successor or a visible failure, never an empty receipt or duplicate Attempt.

### Scope

- Treat explicit governed-provider `waiting_input`/`blocked` as a `needs_you` stop request for that Attempt.
- Durably claim the stop before machine effects.
- Capture bounded provider question/context as evidence, never authority.
- Stop the provider and prove termination before terminalizing.
- Atomically persist `needs_you` evidence, mark the current Attempt `failed`, release its fence, and present the terminal reason.
- If stop is unproven, keep status/custody and surface `needs_attention`.
- Require one immutable S1 `AttemptReplacementDecision` ID for `replace`.
- In one transaction, revalidate that decision and current authority, charge the WorkUnit retry allowance, create exactly one queued successor and new fence, and write the predecessor replacement receipt naming the real successor.
- Launch the successor only through the ordinary W1.1 post-commit path.

### Non-goals

No provider-text authorization, automatic reply/continuation, auto-retry, in-place Attempt mutation, empty successor receipt, new production retry number, parallel WorkUnits, or remote/mobile approval.

### Design and likely files

- `backend/internal/domain/outcome_attempt.go` and focused terminal/replacement records.
- `backend/internal/ports/outcome_store.go` and narrow needs-you/replacement ports.
- `backend/internal/service/outcome/recover.go`, `attempt.go`, and `run_state.go`.
- `backend/internal/storage/sqlite/store/attempt_store.go` plus focused stop/replacement stores.
- append-only migrations for durable stop claims/receipts if existing tables cannot represent them exactly.
- `backend/internal/httpd/controllers/outcomes.go`: consume a decision ID, never owner labels.
- `frontend/src/renderer/components/outcome/OutcomeRunControls.tsx` and Attempt presentation only after backend semantics pass.

### Invariants and trust boundaries

The current Attempt terminalizes as `failed` with a stable `needs_you` observation. `lost` is reserved for uncertain custody; `reconciled` is not owner input. Stop proof precedes release. A replacement decision is consumed by exact identity and semantics. Transaction failure creates no successor, fence, retry charge, or receipt. Exact replay returns the same successor. Launch failure leaves a truthful queued/failed successor record; it never rolls lineage back.

### Ordered implementation sequence

1. Add needs-you claim/evidence domain and persistence.
2. Detect explicit provider attention only from normalized session facts.
3. Implement stop-proof convergence and recovery before terminal mutation.
4. Add terminal projection for failed-with-needs-you.
5. Extend recovery input to require S1 decision ID and idempotency key.
6. Add one atomic storage operation for decision consumption, retry charge, successor, fence, and receipt.
7. Route the committed successor through ordinary launch/recovery.
8. Add API/UI presentation after backend tests pass.
9. Run adversarial review and exclude any path that trusts caller-supplied actor data.

### Failure and test matrix

- attention signal races completion/cancel/budget stop;
- crash before stop claim, after claim, after proven stop, and before terminal commit;
- stop unproven, session missing, dirty/uncertain workspace, and restart convergence;
- forged/missing/wrong/stale/already-consumed S1 decision;
- concurrent replace calls with same or conflicting keys;
- retry exhaustion and fence contention;
- transaction failure at every write boundary;
- commit succeeds but response is lost, then exact replay;
- successor commit followed by readiness/preparation/launch failure;
- predecessor receipt always names the committed successor.

### Evidence gate

Interruption/restart/concurrency/adversarial tests prove one successor or visible failure; no empty/duplicate receipt; no released uncertain custody; exact decision attribution; full relevant backend suites; focused API/renderer tests; visual review of needs-you, stop-unproven, replacement confirmation, queued successor, and launch-failure states.

### Rollout and rollback

Feature-gate the UI until S1 and backend semantics are accepted. Preserve legacy receipts as readable history but forbid new empty-successor writes. Rollback disables new replacement commands while retaining decisions, stop claims, receipts, and migration history.

### Open decisions

No semantic decision remains about terminal status or atomic successor creation. Exact retry values remain part of the unresolved production budget policy.

## W1.4 - one durable mission projection and one true next action

### Current state

Not started as an accepted slice. Existing projection and presentation helpers are inputs to inventory, not proof of one canonical projection.

### Dependencies

Accepted W1.1-W1.3.

### Why

The graph, board, detail view, notifications, and controls cannot each infer lifecycle state independently. A user should never see Running in one place, needs-you elsewhere, and an approval action that is no longer valid.

### Scope

- Define one versioned mission projection from durable Contracts, Plans, WorkUnits, Attempts, verdicts/specs/launches, typed observations, stop/replacement lineage, checks, evidence, Verification, Result, rework, and Acceptance.
- Compute one stable `next_action` with reason and target identity.
- Rebuild deterministically from the event/history record on restart.
- Serve graph, board, Outcome detail, badges, and notifications from the same projection.
- Preserve unknown/legacy states visibly; do not synthesize current truth from cached preview records or provider prose.

### Non-goals

No redesign, no parallel scheduling, no provider-specific projection, no event-bus rewrite without migration need, and no deletion of historical records merely because the projection supersedes old readers.

### Design and likely files

- `backend/internal/domain/lifecycle_projection.go`
- `backend/internal/service/outcome/run_state.go`, `scheduler.go`, and a focused projection service.
- `backend/internal/storage/sqlite/store/*` for ordered projection inputs/checkpoints.
- `backend/internal/httpd/controllers/outcomes.go` and OpenAPI/schema generation if the response shape changes.
- `frontend/src/renderer/hooks/useOutcomeRunState.ts`
- `frontend/src/renderer/lib/outcome-dashboard-presentation.ts`, `mission-attention.ts`, and `outcome-tree.ts`
- `frontend/src/renderer/components/outcome/OutcomeRunBoardAdapters.tsx`, `OutcomeMissionControl.tsx`, and overview/detail surfaces.

### Invariants and trust boundaries

Projection is a read model, never authority. Reducer order is deterministic and idempotent. Every next action cites durable state/reason and is revalidated at the command boundary. Unknown custody outranks optimistic progress. Acceptance remains an owner decision.

### Ordered implementation sequence

1. Inventory every current Outcome/WorkUnit/Attempt reader and competing derived-state helper.
2. Freeze the projection schema, reducer ordering, reason codes, and next-action precedence.
3. Build pure reducer fixtures for every accepted lifecycle and legacy/unknown case.
4. Add store replay/checkpoint support only if needed; retain full rebuild as oracle.
5. Serve one backend projection and redirect canonical hooks/readers.
6. Compare graph, board, detail, notification, and control states against the same fixtures.
7. Remove or quarantine preview/local inference only after callers are enumerated and redirected.

### Failure and test matrix

- duplicate/out-of-order delivery and replay from zero;
- crash during checkpoint/update and stale checkpoint recovery;
- current versus superseded Contract/Plan/verdict;
- queued/preparing/running/unconfirmed/needs-you/budget-failed/lost/reconciled Attempts;
- predecessor/successor lineage and replacement launch failure;
- checks/evidence/Verification/Result/rework/Accept transitions;
- unknown legacy events and deleted/trashed Outcome visibility;
- simultaneous reads while new events commit;
- every surface returns the same next action for the same durable record.

### Evidence gate

Golden reducer fixtures, property/idempotency tests, database rebuild and restart comparison, API contract/generation gates, and renderer tests proving graph/board/detail/control parity. No visual acceptance until state parity passes.

### Rollout and rollback

Run old and new projections in shadow comparison first if practical. Switch readers as a bounded set. Keep source events/history and a rebuild path. Rollback redirects readers without deleting the new projection or history.

### Open decisions

Whether to persist a checkpoint/materialized table or derive on read after performance measurement; retention/compaction only after rebuild and audit requirements are measured.

## W1.5 - integrate Ask, Approve, Watch, and Decide

### Current state

Not started as an accepted integrated slice. Reviewed U2.1 visual/interaction work exists separately and cannot claim truthful integration yet.

### Dependencies

Accepted W1.4 plus reviewed U2.1 visual/interaction work.

### Why

A polished preview is not the product if it is disconnected from admission, recovery, proof, and Acceptance. The four moments must show exactly what Kennel knows, what it proposes, what authority is being granted, and what the owner can safely do next.

### Scope

- Ask: create/clarify an Outcome and show missing information without hidden Plan authority.
- Approve: display Contract, Plan, WorkUnits, provider/model, capability/grants, checks, budgets, admission reasons, and exact owner effect.
- Watch: show direct WorkUnit graph, Attempt phase, provider/session attribution, budgets, checks, evidence, and truthful recovery attention from W1.4.
- Decide: show Result mapped to criteria, Verification separately, retained artifacts/diffs/checks, bounded rework, and explicit owner Accept.
- Integrate the reviewed U2.1 package only after replacing preview/local state with canonical API/projection state.
- Make all reachable states keyboard/screen-reader usable, responsive, reduced-motion safe, and visually reviewed in dark, light, wide, and narrow layouts.

### Non-goals

No decorative motion without state meaning, no preview-only success claims, no mobile parity claim without evidence, no broad product rename, no parallel execution UI, and no hiding unknown/error states to improve a demo.

### Design and likely files

- `frontend/src/renderer/components/outcome/OutcomeMissionControl.tsx`
- `AdaptiveIntakeSurface.tsx`, `IntakeContractReview.tsx`, `OutcomeDecideAuthorizeSurface.tsx`
- `OutcomeRunSurface.tsx`, `OutcomeRunControls.tsx`, `MissionWorkUnitGraph.tsx`
- `OutcomeProveCloseSurface.tsx`, `OutcomeDeliveryPanel.tsx`
- `OutcomeMissionWorkspace.tsx`, `OutcomeLifecycleShell.tsx`, `WorkShell.tsx`
- Outcome hooks/API schema plus shared presentation helpers.
- backend controller/OpenAPI changes only where W1.4 lacks exact data; no duplicate UI-owned authority.

### Invariants and trust boundaries

Buttons describe their exact effect and revalidate at the daemon. Disabled controls explain why. Approval never claims transient resources are reserved. Running appears only after immutable launch evidence and confirmed launch. Needs-you and unknown custody are visibly distinct. Verification and Acceptance are separate. User-visible status never comes solely from provider prose.

### Ordered implementation sequence

1. Make a reachable-state inventory from the W1.4 fixture matrix.
2. Map each state to one primary next action and bounded secondary actions.
3. Redirect U2.1 components to canonical hooks and remove preview assumptions.
4. Integrate Ask and Approve first, including admission failure and stale approval.
5. Integrate Watch, budgets, needs-you, replacement, restart, and unknown custody.
6. Integrate Result, Verification, rework, and Accept.
7. Run renderer/accessibility tests before pixel work.
8. Render and inspect all required states in dark/light/wide/narrow and reduced motion.
9. Prove the same surfaces in the packaged app, not only Vite/web preview.

### Failure and test matrix

- clarification required, analysis failed/expired, and Contract revision changed;
- rejected/stale admission and unsupported capability/accounting;
- fence contention, preparation failure, unconfirmed launch, restart recovery;
- budget stop, needs-you, unproven stop, replacement queued/failed;
- check pass/fail/unknown, missing evidence, Verification fail, bounded rework;
- stale Accept/rework request and concurrent backend transition;
- keyboard-only, screen reader names/live regions/focus return;
- 320-ish narrow layout, zoom/text growth, dark/light contrast, and reduced motion;
- refresh/deep-link/restart without state regression.

### Evidence gate

Reachable-state renderer tests, API contract tests, axe/manual accessibility review, responsive and reduced-motion checks, and attached real-pixel screenshots for dark/light/wide/narrow. Packaged screenshots must prove native composition and canonical daemon state. Preview evidence alone does not pass.

### Rollout and rollback

Land by four bounded moments behind the current Work route, retaining truthful existing fallback until each moment passes. Redirect callers before deleting preview/legacy helpers. Rollback must not remove durable state or expose an obsolete approval path.

### Open decisions

Final copy and density may change after real-user review. Mobile rendering remains evidence-gated. Broad motion/polish stays deferred until every motion corresponds to reachable lifecycle state.

## W1.6 - prove the packaged Codex loop

### Current state

Not started as final current proof. Historical canaries and packaged evidence remain useful leads but do not prove the final reviewed lineage.

### Dependencies

Accepted W1.1-W1.5 and an exact reviewed candidate commit.

### Why

Unit, renderer, and source-level integration tests cannot prove packaging, native IPC, login, local harness discovery, workspace custody, restart, or owner understanding. The milestone passes only when the shipped shape completes the real journey without hidden rescue.

### Scope

- Build the candidate from the exact reviewed commit and record artifact identity.
- Install/launch on the target macOS environment with a real Codex account and real repository.
- Complete Ask, clarification, Contract review, Plan/admission review, owner authorization, workspace preparation, execution, Watch, governed checks, retained evidence, Verification, Result review, bounded rework where induced, and explicit owner Accept.
- Induce at least one safe recovery case, including app/daemon restart at a documented phase, and prove no duplicate work/effects.
- Record machine-readable logs/IDs and user-visible screenshots without secrets.
- Keep signing, notarization, updater, and publication claims separate unless actually completed.

### Non-goals

No Claude/OpenCode/Pi parity, no parallel WorkUnits, no generalized document Outcome, no release claim based only on a packaged directory, and no manual database/API repair hidden from the evidence record.

### Design and likely files

- `frontend/e2e/*` for deterministic regression coverage, supplemented by a real-harness canary rather than mocked bridge proof.
- `frontend/forge.config.ts`, build/package scripts, daemon identity and discovery code.
- `docs/verification/*` for the immutable execution ledger, artifact hashes, exact steps, failures, screenshots, and exclusions.
- existing Outcome service/store/API/UI paths, changed only when the canary exposes a root cause that returns to the owning earlier slice.

### Invariants and trust boundaries

The tested artifact is built from the accepted commit. No secret enters screenshots or logs. Native Codex account/session semantics are preserved. No shell/API/database rescue counts as user-flow success. Every effect belongs to an approved WorkUnit/Attempt. Acceptance is an explicit owner action after evidence review.

### Ordered implementation sequence

1. Freeze candidate commit, target OS/hardware, fixture repository, Contract, expected diff, and safe induced failures.
2. Run full source gates and package; record artifact hash and daemon/frontend identities.
3. Install in a clean or documented profile and verify first launch/discovery/login.
4. Run the happy path to Result and owner Accept, recording IDs and timestamps.
5. Run negative admission and stale-authority cases.
6. Induce needs-you/replacement and a restart boundary; verify exact lineage and no duplicate effects.
7. Verify governed checks and evidence against the final workspace/diff.
8. Exercise request rework and a new bounded Attempt when applicable.
9. Inspect dark/light/narrow/reduced-motion states in the packaged app.
10. Publish a verification ledger that distinguishes passed, failed, not run, and out of scope.

### Failure and test matrix

- missing binary/profile/login, wrong provider/model, and unsupported capability;
- installation path, spaces/symlinks, data-dir permissions, stale run file, and ephemeral port;
- app exit, daemon exit, keep-alive behavior, and restart at queued/preparing/launched/running/needs-you;
- repository dirty state, worktree contention, failed check, retained artifact mismatch;
- lost response and replay for approval/start/replacement/accept;
- network unavailable where nonessential, token accounting unavailable, and budget stop;
- packaged IPC/native confirmation and bearer non-disclosure;
- rework and Accept against stale Result;
- uninstall/update/signing/notarization only if those are part of the claimed artifact.

### Evidence gate

A real packaged macOS journey from repository selection through explicit owner Accept, with artifact hash, exact commit, environment, durable Outcome/Plan/Attempt/decision/Result IDs, final diff/check evidence, restart and replacement lineage, attached screenshots, and an explicit no-hidden-rescue statement. Full source and packaged smoke gates must be recorded. Any missing signing/notarization/updater evidence remains a separate release blocker.

### Rollout and rollback

Do not promote `outcome-loop` based on a partial canary. Fix failures in the owning slice, independently review, rebuild from a new exact commit, and rerun affected evidence. Promotion is a guarded ref change after acceptance; rollback points to the last proven artifact without rewriting migrations or evidence.

### Open decisions

- Exact target macOS versions/hardware and whether signing/notarization/publication are required for the September 18 review versus the later public release gate.
- Exact production budget values.
- Which safe failure is induced if a real needs-you signal cannot be triggered deterministically without provider-dependent behavior.

## Review and change-control checklist

Every implementation package must include:

- baseline and head SHAs, changed-file list, patch checksum, and dependency statement;
- root cause and smallest correct change;
- domain/storage/API/UI compatibility notes;
- new and changed migrations, generated artifacts, and upgrade/rollback treatment;
- positive, negative, race, replay, crash, and restart evidence;
- visual evidence for spatial/UI work and packaged evidence for native claims;
- security review for trust-boundary changes;
- exact tests run, failures, timeouts, and untested claims;
- explicit unresolved decisions and deferred work;
- updated milestone status only after independent acceptance.

No slice is pushed merely because its local tests pass. Independent review accepts the package first; the guarded push then advances only the reviewed lineage while `beta`, issues, PRs, and releases remain untouched unless separately authorized.
