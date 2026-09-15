# Scoped agent packets

Read START-HERE.md first. Source paths are discovery entry points, not blanket authorization to rewrite entire directories. Coordinator freezes exact owned files after P0. Preserve existing user changes. Use the reset's ISC criteria; the probes below refine them rather than define a second acceptance system.

## P0 — Baseline and interface lock

- Role: coordinator plus bounded read-only provider investigator. Dependency: none.
- Read: canonical architecture, ADR 0008/0009, STATUS, runtime reference index; Codex adapters, attempt wiring, planning, recover/run_intent; installed Codex version/protocol. Revalidate exact beta instead of using primary checkout's stale status.
- Work: pin source and package baseline; reproduce singleton clarification, blank kickoff, batch control and replacement refusal; specify native profile mapping, explicit transport binding, turn/session/Attempt boundaries, mutation-to-verification handoff, request identity and event projection. Inventory existing API fields before introducing new ones. Produce a small current-code delta and interface decision table, not another framework.
- Output: baseline evidence, assigned files, API/migration owner, stop/restart semantics, short required architecture amendment proposal. Resolve any owner-level authority decision before dependent code.
- Gate: one conformance experiment can distinguish running turn, idle reusable thread, ended process and closed Attempt. No speculative claim that all four mean done.

## P1 — One useful interactive coding session

- Role: runtime implementer. Depends: P0. Read: adapters/agent/codex, adapters/chatdriver/codexappserver, adapters/codexpolicy, service/outcome/attempt.go, daemon/attempt_wiring.go, session manager and focused tests.
- Work: admit the approved supported native profile through App Server; bind exact transport; deliver full RunBrief automatically; implement input/steering/interrupt using native IDs. Permit development tests. Keep idle conversation reusable. Introduce structured readiness plus quiescence before final checks. Preserve narrow mode as explicitly different if retained. Reuse existing session chat/PTY surfaces for the canary.
- Remove: false steering capability on batch mode; unconditional rejection on the proven profile; conflicting tool suppression and checks-only-after-process-death instructions on that profile. Do not just flip OneShot.
- Tests: file edit; failing development test repaired and rerun; user message during and between turns reaches same native thread; question answered without successor; reconnect no duplicated kickoff; closed Attempt cannot silently resume writes. Anti: old narrow approval is not broadened.
- Gate: one real UI-launched WorkUnit passes those controls and becomes ready for review. Return transcript/events and diff/check evidence. No fanout yet.

## P2 — Useful clarification and planning

- Role: intake/planning vertical-slice worker. Depends: P0. Coordinate shared DTO ownership with P1.
- Read: domain/intake.go, service/intake, service/intelligence/llm.go, service/outcome/interactive_planning.go, intake SQL/migrations, MissionPlanningConversation and relevant hooks.
- Work: additive migration for plural questions/rounds; stable IDs, partial answers and explicit unresolved status; prompts/schema/service agree. Start planning enqueues exactly one real first provider turn; first reply offers material questions or approach draft. Shared readiness policy for Contract/Plan, explicit verification result and reason. Context is inspected before asking avoidable questions.
- Remove: singleton DB uniqueness/count law and API/prompt limit, fake user kickoff, unconditional answer-one-question placeholder, unavailable context controls.
- Tests: three questions plus partial answers; second round; old record migration; reload preserves answers; repeated start one turn; readiness failure clearly rendered; Plan tied to current Contract; stale reply cannot overwrite newer revision.
- Gate: user reaches a useful proposal without guessing what to type; no implicit execution authorization. Covers ISC-1/2.

## P3 — Continue and recover reliably

- Role: lifecycle worker. Depends: P1 integrated lifecycle contract.
- Read: service/outcome/recover.go and run_intent.go, recovery_execution.go, storage receipts, run-state hooks.
- Work: live-session continuation first. For ended execution persist one continuation decision and successor intent; account for predecessor, supersede matching admission refusal, then reconcile launch. Refresh all run/session projections. Surface one current blocker when continuation cannot happen.
- Remove: receipt-only success, fresh duplicate receipts on retry, stale latched failure after explicit retry, frontend-only multi-step recovery chain.
- Tests: double click one successor; crash before/after spawn acknowledgment; unknown surviving writer blocks duplicate; released fence updates; refusal remains truthful when cause persists; no silent replay of finished work.
- Gate: exact prior no-successor reproduction now yields one linked new session or an accurate refusal. Do not hold SQLite transaction over provider startup. ISC-7/10.

## P4 — Interaction primitives and visual reference study

- Role: frontend worker. Depends: P0 for state vocabulary; may run beside P1/P2 with isolated component scope.
- Read: UI-AND-CONTEXT.md, existing product-ui, Motion/Radix primitives, renderer design tokens.
- Work: prototype OperationStatus, QuestionBatch, compact ApproachReview, SessionActivityCard and ResultSummary in existing conventions. Map idle/pending/acknowledged/waiting/error/success/reconnecting to explicit props. Provide keyboard and reduced-motion behavior. Verify reference code licenses before reuse and retain notices. No new global component framework.
- Remove: duplicated spinner/button variants in touched surfaces once callers migrate; ornamental loader variants, unnecessary instructional cards. Do not replace unrelated app components.
- Tests: empty/long/error/offline states, rapid repeated clicks, focus retention, reduced motion, narrow and desktop layout. Rendered proof required; mock states prove only presentation.
- Gate: accessible component gallery/screenshots and before/after reasons, no fake lifecycle transitions. Live integration belongs to P7/P8.

## P5 — Small, truthful orchestration topology

- Role: scheduler worker. Depends: P1/P3. Read: ADR 0008/0009, current scheduler/lease/integration and Plan validators.
- Work: first audit what exists; add only missing seams. Retain selected harness and model binding. Planner proposes bounded units with objective, expected output, relevant context, dependency, supported profile and checks; daemon validates. Use one unit for cohesive changes; independent branches get separate leased writers; integration/check step waits for outputs. Store a concise routing rationale and capability snapshot, not hidden model reasoning.
- Remove: globally-newest-session assumptions in scheduler consumers; unnecessary promotion of native child agents; untested parallel capability advertising. No optimizer or swarm framework.
- Tests: sequential chain; two independent fixture writes safely overlap in separate worktrees; dependent integration waits; dependency failure stops descendants; cycle/stale Plan rejected; cancellation/restart no duplicate writer. Do not run test suites concurrently even when a single conformance test intentionally exercises two workers.
- Gate: real two-branch join works, or parallel feature stays unavailable with explicit launch gap. One successful unit does not prove fanout. ISC-6/10.

## P6 — Mission plugin and app continuity

- Role: thin-client/plugin worker. Depends: P2 and P1 shared binding, P0 interface lock.
- Read: embedded using-kennel skill, CLI root, marketplace assets, outcome/planning controllers and generated API.
- Work: implement actual distributable mission entry assets and thin CLI/MCP-facing operations over daemon; use host-supported invocation. Enter Outcome intake, answer, inspect proposal, request permitted actions, return app navigation and canonical IDs. Respect host approvals and preserve owner-only acceptance. Sync updates both directions.
- Remove: spawn-first AO guidance from active entry; missing plugin directory references; alternate file-based Plan/approval stores. Preserve useful low-level commands behind advanced inspection.
- Tests: plugin-created mission visible app; answer in app reflected client; repeated request no duplicate; reconnect same IDs; installation packaged assets; no direct SQLite/provider spawning by CLI.
- Gate: same Outcome/Contract/Plan and exact revisions visible in both entry points. ISC-5.

## P7 — Live Mission Control

- Role: frontend integration worker. Depends: P1/P3/P4/P5. Own Mission execution surface and session selection hooks.
- Read: OutcomeRunSurface, OutcomeAttemptTerminalPanel, useOutcome, shared session board, graph projection.
- Work: graph above Active sessions Kanban; cards bind individual Attempt/session IDs; history collapsed per WorkUnit. Selecting a card opens that session without affecting others. Needs You shows actual question/approval and reply. Persistent progress with last-event age, advisory budget and explicit reconnect/unknown states; show acknowledged stop separately from requested stop.
- Remove: one global current Attempt controlling all cards, history dominating active view, fictitious session cards for queued WorkUnits, empty disabled controls.
- Tests: two cards route input correctly; same selection survives other-session updates; navigation/reload preserves pending work; stale/disconnected event stream; keyboard graph/list alternative; small window; real daemon evidence.
- Gate: user can explain what is running, what is waiting and what they can do without logs. ISC-4/6/8.

## P8 — Result and bounded rework

- Role: result vertical-slice worker. Depends: P1/P3/P4; coordinate shared hooks with P7.
- Read: Result, verification, evidence, acceptance services and renderer projections.
- Work: automate evidence collection; lead with change summary/diff or preview, checks and unresolved exceptions. Accept and Request changes are primary actions. Rework carries specific correction and criterion context through existing authority; changed scope proposes new approval. Retain exact evidence revisions and invalidate stale verification after edits.
- Remove: ordinary review requiring raw IDs, hidden failures, process-exit-as-success copy, technical proof entry as default.
- Tests: failed check cannot appear passed; no changes cannot appear successful artifact delivery; rework updates evidence; stale Result cannot accept; user decision retained exactly once. Do not automate owner decision for live gate.
- Gate: non-expert reviewer can inspect result, request correction and explicitly Accept. ISC-9/10.

## P9 — Independent integrated review

- Role: independent reviewer, coordinator runs serial verification. Depends: all required slices.
- Work: review immutable integrated diff, rerun original failures and adversarial tests, check removed callers/docs, full touched-area repository gates then packaged issue #38 journey. Capture crashes, renderer errors, provider events, native input, restart and shutdown behavior.
- Tests: use original runbook and START-HERE final gate; exact suite commands resolved from current package scripts/AGENTS. Include backend build/test/race/vet, frontend tests/typecheck, lint, generated contract parity, package and identity as applicable, one suite at a time.
- Gate: criterion-by-criterion pass/fail/blocked/not-run ledger. Unit success, screenshots and process exit never imply Acceptance. Return concise failure packets for regressions, not release claims.

## P10 — Documentation and dead-path reconciliation

- Role: documentation worker, coordinator integrates. Depends: each slice's reviewed evidence; initial inventory may run immediately.
- Read: UI-AND-CONTEXT cleanup policy and actual canonical links/callers. Do not read every historical transcript.
- Work: update STATUS with exact code versus runtime evidence; replace active rejected guidance; add supersession pointers to historical plans; route source manifest to current docs. Propose scoped authority amendments for owner ratification. Remove dead code only in the owning tested implementation slice.
- Tests: links and references resolve, no active single-question/one-shot-interactive claims remain, future worker finds baseline and unresolved launch rows without chat history. Preserve migrations, historic receipts/audit evidence and dirty files.
- Gate: one active dispatch index and no competing completion ledger. Documentation never claims target behavior is shipped before evidence.
