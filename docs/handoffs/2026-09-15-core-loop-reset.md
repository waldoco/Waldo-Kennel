# Kennel core loop: conversation decisions and implementation packet

Date: 2026-09-15. Status: proposed implementation sequence, source investigation completed; product changes and launch verification pending.

## Purpose and evidence

Make one software Outcome flow through useful clarification, an executable Plan, interactive Codex sessions, visible Mission Control, simple review, and owner Acceptance. Preserve this conversation's user corrections so subsequent agents do not recreate rejected UX.

Source baseline inspected: immutable Git object `5304d569aa2e29f0cc77d57539aa2f65a4d01484`, also the locally recorded origin/beta. Primary working copy is dirty at `9c15272d489841c119f3e5a8ef0811a5aa5c711f`. Remote beta was not refreshed during this investigation; verify it before implementation. The old /private/tmp/kennel-issue38-20260914 checkout no longer exists. The September 14 packaged audit is historical evidence, not a live September 15 retest. See the existing local audit directory `~/.kennel/ux-audits/20260914T144231Z-issue38-beta-5304d569-5304d569/`.

This packet extends that audit and existing architecture; it does not replace the canonical Outcome/Plan/Attempt identities. Architecture changes described below must be reflected in the governing docs in the same reviewed implementation slice.

## Decisions from the owner (all ten annotations)

1. Clarification has no one-question-per-Outcome limit. Ask all material questions, grouped into a small coherent batch when helpful. Permit follow-ups when answers expose a new uncertainty. Inspect available repository context first. Do not ask irrelevant questions to meet a quota.
2. Approval is compact: a short approach, important scope/permission consequence, and primary approval action. Details expand. Never bury a consequential change in collapsed content.
3. Mission Control shows the Mission graph first and the session Kanban immediately below it. The outer board continues to show Outcomes. Selecting a node highlights its real sessions; selecting a session opens its live interaction surface.
4. Review leads with the result, an inspectable change summary or preview, checks summary, and any unresolved exception. Two actions: Accept and Request changes. Detailed criteria, logs, and provenance are expandable. Kennel collects evidence automatically.
5. Remove redundant restrictions that prevent ordinary native coding workflows. Replace arbitrary capability combinations with explicit, supported execution profiles. Do not silently change the meaning of old approvals or present prompt-only restrictions as enforced sandbox boundaries.
6. Provide a Kennel mission skill/plugin entry in the existing harness. Codex is the first integration to prove; other harness identities stay capability-aware. A Claude slash command and a Codex namespaced skill may use different invocation syntax.
7. Session interaction is a primary feature: send, steer, interrupt, reply to approvals/questions, and continue when supported. Automatically deliver the complete approved WorkUnit context. A WorkUnit title alone is insufficient.
8. Continue the existing live session when possible. Create replacement Attempts only for a genuine new execution attempt. Recovery has one owner action and one reliable backend operation; historical receipts remain inspectable rather than crowding the active board.
9. Time/token estimates are advisory by default. Show warnings and allow continued work. Only explicit owner hard limits, provider limits, or actual permission/custody failures stop work. Lack of recent file edits is not proof of a stalled reasoning task.
10. Plugin and frontend share daemon commands, canonical IDs, and events. Neither maintains a separate Plan, task graph, approval state, or session database.

## Source-confirmed causes

### Clarification and planning

- `backend/internal/domain/intake.go:154-171` limits clarification count to one.
- `backend/internal/service/intake/service.go:692-694` refuses a second clarification.
- `backend/internal/storage/sqlite/migrations/0104*` constrains the count to 0..1; the intake SQL query also assumes count=0. Use an additive migration, never edit a merged migration.
- `backend/internal/service/intelligence/llm.go:38` requests ONE question and its schema is singular.
- `backend/internal/service/outcome/interactive_planning.go:210-222` creates an owner-waiting planning session without issuing an initial provider turn; finalize supplies the kickoff text.

### Session control

- The governed Codex TUI path sets `OneShot=true` and launches `codex exec`, despite advertising steering. This explains an output terminal that ends instead of an interactive conversation.
- The existing Codex App Server driver has interactive primitives but rejects governed execution in `ValidateExecutionPolicy` (`driver.go:91-104`).
- Governed repository configuration disables native shell/unified execution/plugins/multi-agent and forces native read-only execution. This is a deliberate policy layer, not a missing graphical spinner.
- A raw CLI PTY and an App Server conversation are different transports. Do not promise they are two controls for the same live process unless provider conformance actually proves attachment.
- Exact transport sources: `backend/internal/adapters/agent/codex/codex.go:157-180` selects one-shot execution; `backend/internal/adapters/chatdriver/codexappserver/driver.go:91-104` rejects governed execution. Existing steering capability claims must be tested against the selected mode.
- `backend/internal/adapters/codexpolicy/policy.go:28-37` rejects read+write based on native sandbox shape, although `agent/codex/codex.go:263-285` actually grants writes through private MCP under native read-only. This mismatch needs conformance tests; it does not establish that all safety boundaries must be removed.
- `backend/internal/service/outcome/attempt.go:774-807` already supplies more than a WorkUnit title, but forbids in-session development checks. Completion is tied to process exit in `agent/codex/codex.go:67-70`. Interactive execution must change these together, rather than merely setting OneShot=false.

### Recovery

- `recover.go:234-283` releases custody and records replacement intent but does not start a successor.
- `recordReceipt` creates another UUID on each request.
- `run_intent.go:517-520` skips generations with retained admission failure; release alone does not clear or supersede the latched refusal.
- The frontend calls recovery without completing that command chain and misses run-state invalidation. The visible stale fence is a consequence of that gap.

## Architecture to implement

Use the existing Go daemon, SQLite, generated API, React/Electron UI, and provider adapters. Add no new general orchestration framework for this repair.

The plugin skill supplies workflow guidance and invokes daemon commands. The selected harness reasons and executes. The daemon owns launch, bindings, lifecycle, dependency release, recovery, and result records. The frontend projects the same events.

### Supported execution profiles

Planning/review: read and reason through a real supported native mode. Coding: edit and run local development tools inside an allocated worktree under the provider's actual supported sandbox/approval configuration. Show the real breadth of that profile once during approval. Optional narrower restrictions are admitted only when the selected transport truly enforces them.

Do not request arbitrary read/write subsets then emulate them by stripping the coding harness of its useful tools. Do not silently reinterpret existing narrow Contracts as a broader coding profile. Affected existing Plans receive an understandable revision/approval action. External effects remain separate from local worktree coding; unsupported boundaries must be described honestly.

### Session transport gate

First prove both what the user can see and what they can control. Preferred app integration: existing App Server for events, input, questions/approvals, active turn steering, interruption, and continuation. Preserve an actual interactive PTY mode for users choosing the native terminal; launch the interactive CLI rather than batch `exec`. Keep distinct mode labels and identities. Do not write a second terminal parser to reconstruct canonical completion from text.

Choose the default only after a disposable-repository test demonstrates full context arrival, a real file edit/check, a mid-run owner message, and a second conversational turn. App Server launch must admit the chosen native coding profile; retaining its unconditional rejection defeats this slice.

Separate turn completion, reusable session idle, worker-ready-for-verification, process exit, and closed Attempt. Allow development checks during coding. An explicit structured handoff stops further worker mutation before final verification; final checks remain independently recorded. A finished turn is neither a closed Attempt nor an accepted Outcome. Never let two active controllers drive one session.

### Recovery operation

Implement a restart-safe state transition with an idempotent request, durable successor linkage and a recoverable launch intent. An OS process launch cannot be made atomic with SQLite: persist intent, launch, record identity/acknowledgment, and reconcile uncertain outcomes. Do not hold a database transaction across provider startup. If the current session is alive, steer/continue it. If it is terminal, create one successor only after accounting for its custody. Recompute current blockers and notify both clients.

### Budget behavior

Use estimated and observed elapsed time/token usage with warning thresholds. An advisory limit never automatically kills work. A user-selected hard cap is distinct. Show unknown cost as unknown; token counts are not a dollar estimate. Repeated deterministic admission failure should not spawn additional model calls.

## Research and reuse

- Codex App Server: https://learn.chatgpt.com/docs/app-server — existing thread/turn events, turn/steer, turn/interrupt, and explicit skills are appropriate for interactive UI. Pin/negotiate the installed protocol; do not copy current documentation calls blindly into older generated bindings.
- Codex skills: https://developers.openai.com/codex/skills/ — package a mission workflow that calls Kennel's existing daemon API. Verify installed invocation syntax.
- Superpowers: https://github.com/obra/superpowers — useful source for repository-first brainstorming, concrete implementation tasks, bounded delegation, and separate specification/code review. Adapt the workflow; its question cadence does not override the owner's batch-clarification requirement. MIT licensing requires attribution for copied code.
- Medley: https://github.com/Spine-AI/medley — useful mission/plugin/dashboard entry pattern. Its README documents `$medley:mission` for Codex and `/mission` for Claude. The plugin shim is MIT; the downloaded mission engine is proprietary. Do not plan to reuse an unavailable open-source engine or install it as part of this investigation.

There is no evidence that another planning library alone fixes the observed failures. Existing lifecycle methods are disconnected or deliberately blocked. Repair those seams first.

## Fan-out sequence and agent packets

All workers start from the same newly verified beta SHA in separate worktrees. No simultaneous test suites on this Mac. Every packet returns source SHA, bounded diff, tests and results, omitted behavior, and a small reproduction. Shared API/schema owners merge before dependent UI workers rebase. The coordinator reviews each slice, integrates it, and runs the combined packaged journey. No worker may claim launch readiness from component tests.

| Wave | Owner | Allowed scope / deliverable | Dependency | Main review probe |
| --- | --- | --- | --- | --- |
| 0 | Coordinator | Pin beta; record policy decision and installed Codex capabilities; baseline failing session/clarification/recovery tests | None | Failure reproduced at exact source |
| 1A | Session agent | Codex transport, supported coding profile, full RunBrief, live input and continuation; isolated repo canary | 0 | Owner message reaches exact active session; edit and check succeed |
| 1B | Intake agent | Additive question-batch persistence/API, multiple clarification rounds, renderer questionnaire; remove one-question cap | 0; coordinate shared DTO generation | Three real questions answered in batch; new follow-up persists after reload |
| 1C | Recovery agent | Idempotent successor intent, linked receipt, admission-failure refresh, restart reconciliation | 0; agree session contract with 1A | Double-click and interrupted launch create at most one successor |
| 2A | Plugin/planning agent | mission skill/tool entry, automatic first planning turn, shared Plan revision/event stream | 1A, 1B | Start in harness; same mission appears in app; reply in app is visible to harness |
| 2B | Mission UI agent | Graph above sessions Kanban, selected-node filtering, real session engagement, status and advisory budgets | 1A, 1C | Each running card opens its own session; reload restores current state |
| 2C | Result agent | Automatic evidence summary, compact review, request changes, owner Accept | 1A, 1C | Failed check visible; bounded rework updates exact criterion; no automatic acceptance |
| 3 | Independent reviewer + coordinator | Source review, integrated regression, packaged issue #38 gate, removal of now-unused competing paths | All | Real UI-only Outcome through rework and owner decision, restart and negative paths |

Use whatever implementation models the owner selects; the packets are model-neutral. Do not substitute a model for an explicitly requested one. Stronger review belongs on transport, lifecycle, migrations and permission changes; narrow copy/UI tasks can use cheaper workers.

## Removal ledger

Remove or replace: hard one-question count; empty planning kickoff composer; batch execution disguised as interactive terminal; unconditional interactive execution refusal after supported profile is proven; redundant native-tool suppression on the selected coding path; receipt-only Replace command; latched stale run blockers; repeated provider selection; mandatory raw technical proof forms; unsupported capability claims; active-board clutter from historical Attempts.

Keep and simplify: existing PTY/event infrastructure where working; SQLite canonical IDs and revision history; worktree isolation; native provider permissions; one recovery state transition; provider identity; owner approval and Accept; evidence linked to the files/checks actually observed.

Defer: broad multi-provider conformance, elaborate routing optimizers, automatic skill promotion, unrelated Home/Island expansion, simultaneous parallel writers before lease/integration behavior is tested. Native subagents can remain within a session; do not inflate every child into a Mission node.

AO origin alone is not a removal criterion. For each proposed deletion name the conflicting responsibility, replacement path, remaining callers, compatibility needs, and regression test. No wholesale rewrite or history deletion.

Concrete inherited cleanup: `backend/internal/skillassets/using-kennel/SKILL.md` teaches spawn/session orchestration rather than the Outcome loop. Replace its entry guidance and old AO command examples with the shared mission client. The marketplace references a plugin directory absent from the inspected tree. Ship and test the actual plugin assets. Neither finding justifies deleting useful PTY/worktree infrastructure.

## Acceptance criteria and falsifiers

- ISC-1: Material clarification accepts multiple questions and rounds; falsified by a count=1 refusal or lost answer after restart.
- ISC-2: Planning starts without a magic kickoff message; falsified by an empty unanswered composer while the system waits for owner input.
- ISC-3: One approved coding profile permits its intended native tools; falsified by read-only/missing-write failure for authorized coding.
- ISC-4: Every active session is engageable through its actual transport; falsified by a batch terminal presented as an interactive session or a message sent to the wrong thread.
- ISC-5: Plugin and app show the same mission and revision; falsified by duplicate Plans or divergent approval state.
- ISC-6: Graph above Kanban reflects real bindings and dependencies; falsified by fabricated parallel activity or a ready node that cannot be admitted.
- ISC-7: Recovery creates one linked successor and refreshes the blocker; falsified by duplicate launches, empty successor receipts after success, or released custody reported as held.
- ISC-8: Advisory budget warnings do not terminate work; falsified by an estimated-time timeout killing an otherwise healthy run without a configured hard cap.
- ISC-9: Review summarizes real diffs/checks and allows rework; falsified by manual ID entry needed for ordinary use or hidden failed criteria.
- ISC-10: Anti: no silent permission broadening, provider switch, overwrite of dirty user work, or automatic owner Acceptance.
- ISC-11: Packaged UI-only fixture journey includes mid-planning restart, post-execution restart, direct engagement, rework, and owner Accept. A rescue is recorded and the rescued case rerun cleanly before passing.

## First proof to run

Use a disposable Git repository with a bare main remote established before execution. Request a small behavior change with a test, such as adding a CSV export with explicit quoting behavior. Ask a mid-run clarification, observe Codex apply it, inspect the diff and check, request one bounded rework, then let the owner Accept. Also test wrong provider readiness, unsupported profile, repeated recovery, provider exit, stale Plan, and reconnect. Run package + package:identity and all suites serially. Preserve screenshot and event evidence for every failed transition.

## Completion status

This packet records the corrected product direction and agent work boundaries. Source review is evidence for causes; runtime fixes, migrations, UI changes, and launch acceptance remain to be implemented and verified. No remote writes or release actions are authorized by this packet.
