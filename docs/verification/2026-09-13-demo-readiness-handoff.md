# Demo-readiness implementation handoff

Date: 2026-09-13

## Baseline

- Fresh branch: `codex/demo-readiness-20260913`
- Base: `origin/beta` at `f12b9b62b09379247384f6c791070090d5ab3ff4` (PR #128 merge)
- Scope: native planning, one governed-check invocation/provenance path, Result/rework, packaged verification, and one honest rehearsal.
- No publication, push, merge, release, signing, external effect, or owner AcceptanceDecision is authorized.

Initial baseline checks are intentionally narrow and load-bearing:

| Area | Current baseline | Interpretation |
| --- | --- | --- |
| Interactive planning service | Existing unit coverage in `service/outcome/interactive_planning_test.go` | Durable owner/provider turns, proposal validation, cancellation and recovery seams exist. Live packaged planning previously stalled at “Waiting for the agent”. |
| Native Codex planning adapter | `chatdriver/codexappserver/intelligence.go` | Bounded two-minute call, structured schema, no native repository-tool authority, provider/model provenance checks. Live failure/exit correlation is not yet captured in a durable user-facing result. |
| Governed checks | `daemon/attempt_checks.go`, `service/outcome/checks.go`, `governedtools/server.go` | Durable reservation prevents concurrent duplicate reconciliation, but provider `run_approved_check` executes outside `attempt_check_runs`; terminal reconciliation can execute the same check again. |
| Result/rework | `service/outcome/proof.go`, `OutcomeProveCloseSurface.tsx` | Evidence/Verification/Acceptance primitives exist; no cohesive changed-artifact/result summary and ordinary rework still exposes technical identity in places. |
| Mission Control | Existing Board/List, Contract/Plan/Execution/Proof surfaces | Daemon projections and selection/return paths exist; direct DAG/result integration remains partial. |
| Shutdown/restore | PR #127/#128 fixes are present | Terminal governed restore is fenced and window disposal is guarded in source; packaged runtime confirmation remains part of Phase 4. |

## Dependency order and acceptance matrix

1. Phase 1: establish truthful native planning completion/failure lifecycle. No later phase may claim a UI Plan without this gate.
2. Phase 2: make the daemon-owned check invocation authoritative, including provider calls, reconciliation, restart and artifact identity.
3. Phase 3: project those facts into Result/rework without changing acceptance authority.
4. Phase 4: build one immutable package and run the complete packaged journey plus negative/restart states.
5. Phase 5: rehearse a reversible real repository improvement and record only observed evidence.

| Phase | Acceptance gate | Status at handoff |
| --- | --- | --- |
| 1 | Packaged UI reaches a reviewable Plan or an actionable terminal failure; no indefinite waiting; provider/model and frozen grant remain exact. | Open; reproduce and trace first. |
| 2 | One live post-work invocation plus restart yields one authoritative observation for unchanged Attempt/artifact/check identity; changed input forces a distinct check; uncertainty blocks duplicate effects. | Open; duplicate provider/reconciler path identified. |
| 3 | Result summarizes changed artifacts, checks, criterion verdicts, uncertainty and next safe action; rework creates successor lineage; acceptance remains owner-only. | Open. |
| 4 | Combined package proves UI journey, restart, offline/unavailable, failed check, return/focus, restore fence, and clean shutdown. | Open. |
| 5 | Reversible real repo improvement is demonstrated from user Outcome through Result with no hidden setup and explicit owner decision left open. | Open. |

This document is updated at phase boundaries with exact evidence and blockers;
it is not a substitute for packaged acceptance.

## Phase 1 checkpoint

- Live `TestLiveCodexPacketIntelligence` passed against the signed-in Codex
  app-server path in 5.47s, proving the native structured transport can return
  a bounded packet response with provider/model/session provenance.
- Focused service, daemon, governed-tools, Codex app-server, SQLite store and
  controller tests pass with host permissions. The unprivileged baseline
  falsely failed sandbox/socket tests, so those results are not used as code
  failures.
- Fixed the owner-facing gap where a recovered or failed provider turn left
  `lastFailureCode`/`lastFailureDetail` durable but invisible. Mission Control
  now shows the actionable failure and offers a fresh planning session while
  preserving the old session and its request lineage.
- Renderer regression: `MissionPlanningConversation.test.tsx` — 9 tests pass.
- Remaining Phase 1 gate: reproduce the full packaged UI native planning path
  and obtain either a validated Plan proposal or a correlated terminal failure;
  this commit does not claim that live packaged gate yet.

## Phase 3 checkpoint

- Added a compact Result summary to the existing Prove & Close surface. It is
  derived only from the daemon proof response: criterion coverage, recorded
  evidence count, verification outcomes, explicit criterion gaps, and the
  daemon's next safe action.
- Technical evidence/verification forms remain available below the summary;
  Acceptance, request-rework and reopen remain explicit owner actions. No raw
  Attempt/Plan identity is added to the summary or used to infer acceptance.
- Focused renderer tests: 14 passed across planning and Prove & Close;
  frontend typecheck passes after locale-catalog parity updates.
- Phase 2 remains the dependency gap: the summary cannot truthfully report a
  single authoritative governed-check invocation until the provider/reconciler
  duplication is resolved.
