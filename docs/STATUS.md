# Kennel status

Checkpoint dated 2026-09-13, based on `beta` revision
`c1ba9f78110734fff5f661bc0d8ccdf02f4484f2` (PR #129), with native-planning
failure feedback, daemon-only approved checks and a partial Result projection.
The [demo-readiness handoff](verification/2026-09-13-demo-readiness-handoff.md)
records each tested source revision and package hash. Branch publication is
separate from packaged runtime verification and release readiness.
This page separates implementation, recorded verification and remaining launch
work. It does not claim a release or owner Acceptance.

## Current implementation

- Go loopback daemon and SQLite canonical state with additive migrations,
  trigger-backed changes and generated HTTP/TypeScript contracts; thin CLI and
  Electron/React supervisor.
- Outcome, immutable Contract/Plan, bounded WorkUnit dependency graph, Attempt,
  Session, Evidence, Verification and explicit user Acceptance foundations.
- Owner-configured OpenAI/Anthropic reasoning and native Codex packet-mode
  reasoning. Contract-bound planning uses bounded disclosed context and creates
  no execution authority or Attempt. Missing configuration fails explicitly;
  there is no offline proposal floor or hidden provider fallback.
- Approved provider/model bindings, capability admission, serial scheduling and
  restart/replay fences. The normal frontend Start request uses the approved
  Plan identity and request key, not a mutable Project provider preference.
- Codex governed Attempts receive a private, frozen-policy repository surface:
  bounded listing/text reads and scoped text writes when granted. The allocated
  workspace root is frozen into the admission snapshot; provider-native
  execution remains read-only, while a required positively allowlisted Kennel
  MCP carries narrow repository authority. Approved checks run only through
  the daemon's post-termination reservation/observation path; the provider MCP
  neither advertises nor accepts `run_approved_check`. Generic shell,
  unified-exec, web and plugin surfaces are disabled; unsupported policy shapes
  and unverified App Server injection fail closed.
- Codex App Server compatibility is negotiated against the installed build's
  own declared protocol surface at session start, not assumed from a version
  number: a required-method floor fails closed with the missing methods named,
  optional capabilities degrade individually, and negotiated provenance is
  persisted per provider session (append-only, surfacing digest drift and
  degraded capabilities on the Attempt inspection view in Mission Control).
  The generated bindings stay pinned to one provider build for
  reproducible conformance tests, and a scheduled drift CI regenerates them
  from the latest published CLI (ADR 0016).
- Work is the default destination. Plan/graph views and attached Attempt
  supervision expose daemon facts; provider Sessions remain technical detail.
  Kennel Island starts with the desktop when the display supports it; its
  persisted visibility preference remains owner-controlled in Settings.
- Mission Control exposes durable planning failure details and a fresh-session
  action while preserving prior lineage. Selecting a WorkUnit on the execution
  graph opens its daemon-derived state, blocker, criterion proof readiness and
  attempt lineage with session engagement. The Result surface projects daemon
  artifact/check/criterion facts, measured per-file changes (bounded, with an
  explicit truncation note), uncertainty and the next safe action. Technical
  proof forms and explicit owner review decisions remain available; rework and
  reopen target daemon-provided Contract, Plan, WorkUnit and Attempt identities
  rather than typed raw ids.
- Codex, Claude Code, OpenCode, Cursor and Pi are active execution-provider
  identities; this does not establish every role's live conformance.

The scheduler has concurrency **1** behind a Project custody fence. WorkUnit
WorkspaceLeases and safe parallel execution remain later work under ADR 0009.
Retained-artifact, governed-check, supplied-document, proof and handoff primitives
exist, but complete integration and live closure are still open below.

## Recorded launch verification

The [2026-09-13 handoff](verification/2026-09-13-demo-readiness-handoff.md)
records packaged native Codex planning reaching `proposal_ready` through
API-assisted configuration, Contract setup and planning requests. An earlier
UI inspection rendered the resulting Plan as `Ready to authorize`; the final
package's second UI inspection was blocked by the locked Mac. This is not a
UI-only planning/execution/Result/rework journey or five-harness conformance.
The handoff records frontend gates and package hashes at their tested commits;
subsequent review repairs require their own exact-HEAD checks. Live post-work
check provenance, changed-input/negative paths, full restart/return behavior and
the real-repository demo remain open. Historical checkpoints below retain their
original scopes and do not extend those claims.

The checkpoint includes direct OpenAI/Anthropic reasoning configuration,
Contract-bound interactive planning, and native Codex read-only packet-mode
reasoning. It does not silently fall back between providers. Repository-context
settings have strict PATCH/persistence semantics and an advanced Settings UI;
settings-read failures fail closed. Generated RunBriefs carry exact Contract
criteria, approved check argv, review command and bounded context requirements.

The packaged macOS application and package identity passed. In an isolated
profile, a real Codex Attempt initialized through Codex App Server, found the
packaged sidecar/hook path, emitted Kennel activity hooks, exited zero and
reconciled. Plan and graph are separate views; Execution shows one Work graph
followed by Attempt lineage, correct provider branding, current-session Engage,
exact attention reasons and replacement controls. This closes the historical
tested-path blocker where a symlinked Codex CLI could not find
`codex-code-mode-host`. It does not establish native tool/plugin/approval parity
or provider conformance across every installation.

The Issue #115 packaged canary closed that repository-affordance blocker for
Codex. A real approved Outcome repaired `report.md` only in its leased worktree,
then ran the exact frozen SHA-256 check through Kennel. The original checkout
remained unchanged. A prelaunch workspace failure held custody until explicit
replacement, and restart retained the two historical Attempts without replaying
the succeeded provider session. The then-current pre-repair packaged daemon was
re-probed under native read-only Codex: it repaired the disposable report, passed the
same exact check, and refused mixed-case `.GIT` custody and traversal writes.
Exact IDs, hashes and refusal evidence are in
the [Issue #115 verification record](verification/2026-09-12-issue-115-governed-repository-tools.md).
This proves the bounded execution slice, not retained-artifact automation,
WorkUnit-scoped Verification or an owner `AcceptanceDecision`.

Post-review correctness repairs now preserve executable modes across governed
text replacement and durably fence write/check effects when approved-check
termination is unknown, including private-server restart and Outcome recovery
before ordinary liveness observation. One final-code pre-remote Attempt remains
unconfirmed because the environment refused the explicit owner assertion needed
to replace it; that case was not bypassed.

An independent corrected-code canary then started with a resolvable local
remote before execution. The real packaged Electron UI imported the disposable
repository, selected Codex, showed the Outcome in progress, and later rendered
it `Ready for review`. One Codex Attempt modified only its retained leased
worktree, passed the exact approved check under the macOS seatbelt runner, and
created canonical deterministic Evidence and Verification. Restart with the
same isolated profile/data preserved exactly one succeeded Attempt and one
terminated session without automatic duplication. Post-restart session
inspection subsequently exposed a restore/duplicate-session error and app-exit
disposal warning; those observations were left open at that checkpoint and are
not folded into Issue #115 completion. No owner Acceptance was created.

A [post-canary lifecycle follow-up](verification/2026-09-12-post-canary-lifecycle-and-issue-35-delta.md)
now blocks manual, resume, and startup restoration of a terminal governed
Attempt and guards destroyed-window composition disposal. At source revision
`f7d57d80c3229053d1f3ce96ab398b7d172fca2d`, full Go tests,
focused Electron lifecycle tests, typecheck, isolated-cache lint, and a fresh
package build passed, as recorded in that follow-up. That package was not
launched. These results do not prove the fixes through the combined UI lane;
UI-driven quit and restart behavior remains runtime-unverified. The follow-up
also records the
then-current Issue #35 delta. Its duplicate provider/reconciler check path
has since been removed in source; the live post-work/restart canary for the
single daemon-owned path remains open in the newer handoff.

The [combined desktop verification](verification/2026-09-12-demo-integration.md)
records a separate packaged UI run at code revision
`b2c7ec908a6da2f020806ab71f77fece42cfa275`, before the lifecycle fixes.
Onboarding, repository import, editable Contract and explicit provider
verification worked. Native planning stayed “Waiting for the agent”; the
daemon exited, no Plan revision was created, and execution was not reached.
Restart restored the Project and Contract once. This is not a complete
UI-driven Plan/execution/proof/rework/Acceptance journey. The earlier successful
execution canary used API-created Contract/Plan state and remains distinct.
Fresh combined-source Go tests (including the full race suite), build/vet,
frontend typecheck and five selected
frontend suites passed. Full lint passed with isolated caches and zero issues;
the combined verification record retains the exact source provenance.

The [PR #110 handoff](handoffs/2026-09-12-pr110-launch-fixes/HANDOFF.md)
and [execution ledger](handoffs/2026-09-12-pr110-launch-fixes/EXECUTION-LEDGER.md)
retain exact command scopes and live-run provenance. These are recorded results
at the tested revision, not checks rerun by this documentation cleanup.

Earlier scoped evidence remains in the
[Wednesday follow-up](verification/2026-09-09-wednesday-followup.md),
[completion ledger](superpowers/plans/2026-09-10-luna-kennel-work-completion.md),
and [launch usability record](verification/2026-09-11-launch-usability-iteration.md).
Their historical missing-host, branch-integration and unlaunched-package notes
must not override the later PR #110 canary. Full provider permission, recovery,
packaged journey and owner-acceptance gates remain distinct.

## Remaining launch and roadmap gaps

| Priority | Area | Current gap |
| --- | --- | --- |
| 1 | Native planning and daemon recovery | API-assisted packaged native Codex planning reached a reviewable Plan; actionable failure feedback is implemented. Prove the complete UI-only path and negative/restart lineage on the final package before claiming the demo journey. |
| 2 | Autonomous proof and closure | Automatic artifact retention and deterministic Evidence/Verification passed one earlier bounded canary. Checks now have one daemon-owned executor in source; prove its live post-work/restart and changed-input/unknown-effect behavior, complete provenance and negative/rework paths, and preserve separate owner Acceptance. |
| 3 | Evidence and Result experience | A partial daemon-derived artifact/check/criterion Result summary exists. Complete ordinary rework beyond raw non-contract target IDs and verify the packaged Result/rework path; prove retained downstream WorkUnit output materialization in a real multi-unit canary. |
| 4 | Mission Control | Complete the direct WorkUnit DAG projection and integrated Board/List navigation while retaining the session Kanban beneath the graph. |
| 5 | Parallel scheduling | Replace the intentional concurrency-`1` Project fence only after durable WorkspaceLease, dependency, integration, recovery and cleanup gates prove safe. |
| 6 | Session continuity | Decide and implement historical-session inspection or engagement beyond the currently engageable active session. |
| 7 | Release/update | Publish and test actual install/update artifacts. Package identity is verified, but the updater reports no published GitHub versions. |
| 8 | Remaining manual acceptance | Complete applicable mobile rendering and manually verify reduced-motion behavior. |

General non-repository Outcomes, supplied-document wiring and durable delivery
also need their own integration/verification evidence; repository planning alone
does not establish those paths. Model-backed decomposition proposals remain
outside the current reasoning surface.

The launch gate is: real repository → grounded Contract → approved Plan →
bounded execution → retained artifact and governed checks → understandable
proof → owner acceptance or rework, including restart without duplicate work.
A zero-exit provider session is only one part of this journey.

## Public release readiness

The live `gh release list` check on 2026-09-13 returned no releases; there is
no downloadable GitHub release at this checkpoint. The DMG maker
attempt stalled; a packaged application build does not establish a completed
installer, signing/notarization, or an install/update release. Installation/update
publication and signing/notarization still need maintainer work. Hosted CI gates
are enforced and private vulnerability reporting is enabled;
negative/cancellation/retention CI evidence and a private-report response drill
remain open. Follow the
[launch checklist](../ROADMAP.md#public-release-readiness) and [open issues](https://github.com/waldoco/Waldo-Kennel/issues).
The [roadmap](../ROADMAP.md) defines later milestones. Contributors start from
current `beta`; maintainers promote tested work to `main` separately.
