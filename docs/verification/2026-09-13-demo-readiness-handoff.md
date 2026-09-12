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
| 1 | Packaged UI reaches a reviewable Plan or an actionable terminal failure; no indefinite waiting; provider/model and frozen grant remain exact. | Passed for verified native Codex path; actionable-failure and restart lineage remain covered by focused tests. |
| 2 | One live post-work invocation plus restart yields one authoritative observation for unchanged Attempt/artifact/check identity; changed input forces a distinct check; uncertainty blocks duplicate effects. | Daemon-owned path implemented; live post-work rehearsal remains Phase 5. |
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

## Phase 4 checkpoint

- Local packaged candidate built successfully from code checkpoint
  `7f2ef4e9c4ed2fd580576a571814a0cb09fc1d65`; package identity remains
  `in.heywaldo.kennel`, `Kennel`, version `0.10.3`.
- Embedded daemon SHA-256:
  `b4c5d0b2159bb527418bcb677a908fde45e5b0d58d9cc1aba33505326ca19ae3`.
- `app.asar` SHA-256:
  `03a2d426e8f93b9cf61a7af625aa0deb2ea840b09d0cb5b380de6a12a0989f2d`.
- Disposable package launch reached a healthy loopback daemon and rendered
  the configured-unavailable reasoning state (`PROVIDER_NOT_READY`) without
  fallback. The full Project→Plan→execution journey was not rerun on this
  fresh profile in this checkpoint; no packaged acceptance is claimed.
- The candidate was stopped through its own launch session. No unrelated
  Kennel process was stopped and no release artifact was published.

## Phase 1 packaged checkpoint

- Rebuilt the candidate after the daemon-owned check boundary change. Identity
  remains `in.heywaldo.kennel`, `Kennel`, version `0.10.3`.
- Embedded daemon SHA-256:
  `88089d54ba39d6d5f3aaa3ef0bcd2046df88a2c6d22c5453747239408bc7e525`.
- `app.asar` SHA-256 remains:
  `03a2d426e8f93b9cf61a7af625aa0deb2ea840b09d0cb5b380de6a12a0989f2d`.
- Disposable profile and repository:
  `/tmp/kennel-demo-readiness-electron-20260913b`,
  `/tmp/kennel-demo-readiness-data-20260913b`,
  `/tmp/kennel-demo-readiness-planning-repo-20260913`.
- The owner-selected Codex App Server path was explicitly configured and
  owner-verified through the packaged daemon (`POST /settings/reasoning/verification`;
  200, `verified:true`). Native candidate discovery then returned the exact
  binding `native_harness:codex-app-server:provider_default::`, `ready:true`.
- The first start attempt correctly failed closed because the Contract had no
  repository-read authority (`PLANNING_REPOSITORY_READ_REQUIRED`). An explicit
  owner Contract revision 2 granted `readWorkspace:true`; the same exact UI
  planning requests then created `planning-c0b5efa0-b5d6-4af4-a517-5627f4189197`.
- The real provider turn completed in 7.14s (HTTP duration), persisted
  `proposal_ready`, `effectiveProvider:codex-app-server`,
  `effectiveModel:gpt-5.3-codex-spark`, `proposedPlan.id=plan-82840312-cc5a-4de0-9cad-ebb9276b70ee`, and one planner turn. The packaged UI subsequently rendered the Outcome as `Ready to authorize` with `Plan 1` and the exact read-only permission projection.
- This proves the supported native path reaches a reviewable Plan. It does not
  claim Plan approval, execution, Verification, or Acceptance. The earlier
  unconfigured profile still truthfully rendered `PROVIDER_NOT_READY`; no
  provider fallback was used.

## Phase 2 checkpoint

- Approved checks are now daemon-owned post-termination work. The provider MCP
  server no longer advertises or accepts `run_approved_check`; its attempted
  invocation is rejected as an ungranted tool. Codex launch construction no
  longer enables that provider-side tool, and the RunBrief tells the provider
  that Kennel runs the exact frozen check after termination.
- The existing canonical daemon path remains the only executor: it reserves
  `(AttemptID, checkID, artifactVersion)` before invocation, records one
  immutable observation, marks interrupted reservations `unknown`, and reuses
  observed/unknown rows on reconciliation. No competing SQLite writer,
  fabricated identity, migration, or generated contract was added.
- Focused checks passed for `governedtools`, Codex launch construction, and
  Outcome attempt/prompt behavior. The unprivileged daemon/controller suite
  still needs the already-established host-permission rerun because its
  sandbox blocks seatbelt and loopback tests; that is environmental and not
  used as a code failure claim.
