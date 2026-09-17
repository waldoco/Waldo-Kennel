# Outcome Control Plane MVP — post-PR99 execution plan

> **Superseded implementation plan.** Retained as historical execution evidence. Do not use as current work order; use the [persistent-session execution map](../../roadmap/persistent-session-execution-map.md).


**Updated:** 2026-09-09. **Source baseline:** `0f5def7ce3823487eeab89401f9cd5fd10d26cc2` (`beta`, merged PR #99).
**Target:** a usable Outcome Continuity product; Saturday 2026-09-12 is a conditional launch target, not permission to skip gates.
**Status:** execution-ready plan; product implementation below remains open.

This replaces the pre-merge checklist formerly in this file. Do not execute the obsolete instructions to stay on WT3, confirm PR99 is unmerged, recreate intelligence/routing, or rewrite migration 0114. Migrations through **0115** are merged history. Fetch beta and choose the next unused number for any new migration.


## Scope correction and Work experience companion — 2026-09-09

Read [Kennel Work launch experience and execution handoff](../../product/2026-09-09-kennel-work-launch-experience.md) before implementing a slice. It specifies general Outcomes, Board/List, the direct WorkUnit Mission Graph, focused launch navigation, delivery and owner guidance. Its packets refine L2–L7 below; ADR/domain authority remains unchanged.

Fresh inventory: remote beta is `9396c3844`; local Wednesday HEAD `c83684c11` contains five additional commits. L2 and repository-focused L3 implementation exist; L4 Plan/schedule integration exists but the direct WorkUnit Mission Graph remains incomplete. Assess existing behavior and complete gaps; do not restart or classify code as absent because the original assignment was narrower. Live verification and owner acceptance remain separate gates.

## 1. Start here: execution contract for an implementer

1. Read `AGENTS.md`, ADRs 0010/0011/0012, `docs/STATUS.md`, this plan, and the relevant source packet below. ADR0012 supersedes older deterministic/offline fallback guidance. No silent canned or alternate-model fallback.
2. Fetch current beta; record its SHA, branch, clean/dirty status and baseline tests. Preserve unrelated work. Create a `codex/` feature branch from latest beta in an isolated worktree; target beta. Never force-push or merge without user authorization.
3. Implement **one named slice**. Do not execute the entire document in one patch. Before editing, report existing behavior, missing behavior and exact files to change. Source may have advanced: do not recreate a fix already merged.
4. For a bug, reproduce the failing behavior with a meaningful test first. Use real store/HTTP/runtime boundaries where those are the subject; a mock that echoes inputs is not evidence of runtime enforcement.
5. Reuse existing ports and canonical writers. Change domain/API/storage only when this slice requires it. Additive migrations; edit sqlc source then generate; code-first DTO/spec then generate OpenAPI and TypeScript together.
6. Keep existing five-stage Work UX, design tokens, i18n, keyboard/focus and reduced-motion behavior. Backend owns derived state. No new wizard, task model, parallel Work shell or transcript-derived authority.
7. Run the narrow checks specified below, then the affected-area gates in section 6. Capture actual results and limitations. Do not weaken assertions, delete failing tests or call unsupported behavior complete.
8. Update the slice row below and STATUS with evidence; provide the handoff template in section 8. A commit/test pass is implementation evidence, not owner acceptance or release acceptance.

**Hard stop:** cannot enforce approved effects; ambiguous prior process/effect; stale authority; contradictory or wrong-lineage proof; unexpected migration changes; real user data at risk. Preserve state and report the specific blocker. Do not invent broader permission to meet the date.

## 2. Product cut and architecture

One returning builder can answer: What did I ask for? What changed? What is proved? What remains? What needs my decision? The builder can accept or continue from the Outcome without reconstructing individual sessions.

Keep Go/SQLite, immutable Contract/Plan, WorkUnit DAG, Attempt fences, existing runtime/worktree machinery and proof/acceptance model. Ship a **serial** graph through the real scheduler boundary. Concurrency one is a scheduling limit, not a one-WorkUnit Plan schema. Preserve five provider identities; admit only capability/model/runtime combinations with conformance evidence. Do not claim uniform five-provider support from inventory alone.

Include: real Git Project registration, reasoning setup, grounded editable Contract, full Plan review/replan, exact-bound execution, artifact continuity, checks, re-entry/rework, explicit acceptance, owner-triggered result export and restart recovery. General Outcomes include local document/research artifacts as well as software; advertise only tested execution capabilities. Missing credentials/readiness must be remediable.

Exclude from launch unless already proved: full parallel workspace scheduling, deployments/sending/PR mutation, automatic Git integration into the user's branch, composed-Outcome model proposer, personal Home expansion, general memory platform and provider-wide feature parity. Preserve historical readability. Unsupported normal controls must be hidden/disabled with a truthful reason.

### Source-confirmed delta (not live UX acceptance)

| ID | Current source at baseline | Required correction |
|---|---|---|
| D1 | `ports/attempt_execution.go` carries provider/model/prompt, not structured WorkUnit grants; `daemon/attempt_wiring.go` spawns a worker; `session_manager/exact_execution_binding.go` merges non-model Project settings | Carry frozen normalized execution policy to runtime; prove least privilege, not just prompt wording |
| D1a | `Manager.Spawn` never calls `prepareSpawnExecution`; readiness uses exact-binding helper but launch merges Project model. A Manager-boundary regression fails for explicit and provider-default semantics | **First fix:** wire exact config before readiness and TUI/chat branching; assert real adapter launch input |
| D2 | `OutcomeRunSurface.tsx` and `OutcomeDecideAuthorizeSurface.tsx` use `plan.workUnits[0]`; `outcome/scheduler.go` computes topological runnable units; no production GetSchedule HTTP caller found | Full Plan review and daemon-derived schedule projection/advancement |
| D3 | Run surface sends mutable Project harness; `useOutcome.ts` says ignored; `outcome/attempt.go` actually rejects mismatch | Remove redundant normal-client harness input; preserve exact binding |
| D4 | ProveClose records `subjectType: outcome`; scheduler accepts exact WorkUnit/Attempt proof; UI digest hashes metadata | Collect exact-lineage proof and actual artifacts; do not relax scheduler validation |
| D5 | `intelligence/llm.go` sends statement/one answer/previous title; no repo context input; every answer becomes TemporalCondition | Bounded repository context, substantive prior context and semantic clarification handling |
| D6 | reasoning config is startup environment; nonterminal IntelligenceRun listing has no runtime consumer; error terminalization can be ignored | Setup/readiness and interrupted-call reconciliation; adapter/service behavioral tests |
| D7 | `modelcatalog/catalog.go` static Codex catalog says official-catalog; LLM usage is returned then dropped; generic waldo-llm provenance | Honest source/provider/model/usage reporting and measured optimization |
| D8 | STATUS and old plan described already-merged work as pending and promised removed fallback | This documentation refresh; correct remaining stale comments during touched-code cleanup |
| D9 | ProposePlan reuses latest proposed plan for unchanged preference; draft assumptions/blockers are not copied into canonical Plan construction | Explicit replan semantics; preserve material assumptions/blockers for informed approval |
| D10 | `renderRunBriefPrompt` contains goal/unit/checks/stops, no structured upstream artifact or prior-attempt packet | Prove downstream workspace/artifact continuity and bounded recovery context |

Backend tests cover important foundations, including frozen binding and scheduler lineage. They do not exercise the disconnected desktop path or prove live permission enforcement. D1/D6/D10 require runtime characterization before selecting implementation details.

## 3. Dependency and ownership ledger

| Slice | Depends on | Status | Primary boundary |
|---|---|---|---|
| L0 baseline/source reconciliation | none | implemented in isolated `codex/l0-baseline-cleanup`; narrow/full frontend, frontend typecheck, HTTP/spec parity, full Go race, and full lint green; generated contracts repaired after the L0 rename; L1a-owned unused helper findings resolved; foundation wrapper reaches but is blocked by unrelated cloud schema drift | this plan, STATUS, existing product companions |
| L1a exact model launch | L0 source baseline | implemented in isolated `codex/l0-baseline-cleanup`; Manager-boundary TUI and Chat regression tests green, targeted race/vet green; live provider conformance and restart/recovery remain open | session service → Manager → actual launch config |
| L1b governed capabilities/replay | L1a | implemented in isolated `codex/l1b-capability-replay`; normalized attributed policy reaches readiness and TUI/Chat adapter boundaries; Codex now rejects restricted scopes/unknown capabilities, pins the full sandbox boundary at fresh and recovered TUI/Chat execution payloads, and fail-closes governed recovery when durable Attempt evidence is missing/invalid; service/SQLite replay and recovery/adapter tests green; live canary and other-provider conformance remain open because the local code-mode host is missing | approved capabilities → adapter/runtime + idempotency |
| L2 reasoning setup and recovery | L0 | assigned L2 implementation present locally; follow-up review and live provider conformance remain open | settings/secrets → LLM → durable IntelligenceRun |
| L3 grounded proposals and replan | L2 | repository-focused implementation at `684b9c0a9` plus fixes; local only; general context and live journey gaps remain | context → Contract/Plan proposal/revision |
| L4 complete Plan/Mission projection | L1, L3 | partial local implementation at `170230bf0` plus fixes; Plan/schedule present; direct WorkUnit graph and integrated desktop journey incomplete | generated schedule API → existing Work UI |
| L5 proof, artifacts and serial continuation | L1, L4 | open | runtime facts → proof → next WorkUnit |
| L6 re-entry and Outcome navigation | L5 | open | owner supervision/rework/history |
| L7 release rehearsal and optimization | L2–L6 | open | packaged desktop + real repo/provider |

References to L1 below mean both L1a and L1b. Default execution is sequential. L1/L2 can use separate implementers only when explicitly assigned and their shared ports/daemon files have a single integration owner. Do not run overlapping storage/API edits concurrently. After each merge, rebase the next slice on current beta and recheck the delta.

## 4. Detailed slice packets

### L0 — Restore a trustworthy test baseline before UI implementation

The fresh suite has 23 failures: NewTaskDialog (13), TaskComposer (7), Sidebar (2), SwitchAgentDialog (1). Exact test names and observed errors are in the verification record. Many assertions expect Codex/default-model selection; that suggests stale provider fixtures but does not prove all failures are fixture-only.

1. Run these four files directly and classify each failure against current provider-neutral behavior and its real entry path.
2. Correct obsolete expectations without restoring hidden defaults. Preserve tests for keyboard submission, errors, explicit selection, legacy readability and role admission. If production behavior is wrong, fix the narrow defect with a regression test.
3. Triage the **190 lint issues** recorded in the baseline: errcheck 1, goimports 20, govet 1, nilerr 1, revive 151, sqlclosecheck 3, staticcheck 7, unconvert 2, unused 4. Distinguish an intentional readiness-negative return from swallowed failure before changing nilerr behavior. Inspect SQL rows lifetime, unused exact-binding helpers and dead code before mechanical fixes. Formatting-only cleanup may be a separate small commit. Do not add paragraph comments to trivial getters just to appease a rule; keep exported API rationale concise or unexport private helpers where appropriate. Do not disable rules globally or use blanket suppressions.
4. Do not blanket-delete/skip the files or rewrite snapshots to match output. A retired entry path may have its launch tests replaced only alongside proof of the new Outcome path and retained historical behavior.
5. Run the full frontend suite and lint after narrow fixes; require zero unexplained failures before calling a UI slice merge-ready. L1's backend investigation can proceed while this is being classified, but release acceptance cannot.

From frontend: `npx vitest run --config vite.renderer.config.ts src/renderer/components/NewTaskDialog.test.tsx src/renderer/components/TaskComposer.test.tsx src/renderer/components/Sidebar.test.tsx src/renderer/components/SwitchAgentDialog.test.tsx`.

### L1a — Fix exact model semantics in the actual launch path (first code PR)

**Confirmed failure:** a temporary test calls real `Manager.Spawn` with an ExactExecutionBinding and the existing recording adapter/runtime fakes. Both explicit `approved-model` and provider-default incorrectly reach the adapter as `mutable-project-model`. Reproduction source/output is in the [verification record](../../verification/2026-09-08-post-pr99-launch-baseline.md). This is a Manager-boundary regression, not live-provider conformance.

**Allowed scope:** `backend/internal/service/session/attempt_spawn.go`, `backend/internal/session_manager/{manager,exact_execution_binding,attempt_readiness}.go`, their tests, and chat-launch plumbing only if needed to preserve the binding. No schema/UI/routing redesign for this slice.

1. Recreate the recorded regression as a permanent behavioral test. Add the provider-default case; it must clear mutable Project model values, not substitute a magic default model string.
2. Trace `Service.SpawnExactAttempt` → `Service.Spawn` → `Manager.Spawn`. Wire the existing config resolution before readiness and TUI/chat branching, using a request-local Project configuration copy. Preserve ordinary legacy spawn behavior when no exact binding is supplied.
3. Inspect later TUI/chat config merging: it must not restore the mutable Project model after resolution. Test both actual launch/config paths, not only the pure helper. Persisted Project config must remain unchanged.
4. Test explicit model, provider-default, Project preference mutation, wrong/invalid historical binding, and provider-local non-model config. Invalid binding must reject before durable session/worktree/runtime creation. Check restore/recovery of this governed session does not reinterpret approved model semantics; if restoration needs a separate durable change, report and split it before claiming restart correctness.
5. Remove `normalizedExactModel` only after proving no caller needs it. Do **not** delete unused `prepareSpawnExecution` merely to appease lint: its missing production call is the defect. Consolidate it only if the replacement is actually wired and tested.

**Done:** both recorded red cases become green at the Manager boundary; chat/TUI and existing ordinary-spawn tests pass; actual adapter input preserves approved semantics. Record RED/GREEN output and limitations. Run backend session_manager, service/session, daemon and outcome tests; `go test -race` on touched packages and lint. If global lint still fails, report baseline versus new issues; L0 must clear it before release.

### L1b — Enforce the approved capabilities and replay semantics

**Read:** ADR0009 and AGENTS capability sections; `backend/internal/ports/attempt_execution.go`, `service/outcome/{attempt,gating,provider_binding}.go`, `daemon/attempt_wiring.go`, `session_manager/exact_execution_binding.go`, `session_manager/manager.go`, `ports` spawn/agent contracts and the relevant `adapters/agent/` / `adapters/chatdriver/` implementations.

**Existing tests:** `service/outcome/attempt_provider_test.go`, `attempt_gating_test.go`, `attempt_test.go`; `session_manager/exact_execution_binding_test.go`; real SQLite Attempt admission tests.

Implementation:

1. Trace one inspect and one modify-and-execute WorkUnit from approved Plan through both supported TUI/chat paths. Document effective permission resolution and which external effects each runtime can actually fence. Do not label a CLI permission string capability enforcement without a behavioral probe.
2. Extend the existing immutable Attempt spawn input with the minimum normalized policy derived from the approved unit and its grants, plus attribution needed for enforcement. Do not pass mutable Project policy as replacement authority. Bind policy to the admission snapshot/digest where required; historical missing policy is readable, never silently synthesized for new work.
3. Have readiness and spawn validate the same policy. Map normalized policy to provider-specific mechanisms inside adapters. If a provider cannot enforce a requirement, refuse before launching with an actionable typed reason. Do not broaden authority or silently pick another provider.
4. Test Project permission changes after approval as well as model/provider changes. Inference/auth traffic and arbitrary tool effects must not be conflated. Exec capability alone must not imply deploy, push or unrestricted external effects.
5. Audit idempotency-key replay: same key/same Outcome/Plan/unit semantics returns the same Attempt; same key with different semantics conflicts rather than returning another Outcome's Attempt. Check both service fast path and concurrent SQLite admission path. Preserve the existing Project fence until narrower custody is proved.
6. Delete inaccurate permission/harness comments in touched files. Do not change provider-independent domain rules into brand-specific branches.

**Tests/falsifiers:** inspect attempts to write a canary file and is denied; denied network/effect probe produces no external write; broad Project config cannot widen a narrow unit; mismatch historical binding launches zero sessions; concurrent identical start produces one Attempt; conflicting replay returns conflict; unavailable enforcement returns blocked with zero spawn. Use a disposable repo and controlled local test endpoint, not a real deployment.

**Narrow commands:** from backend, `go test ./internal/service/outcome ./internal/session_manager ./internal/daemon ./internal/storage/sqlite/store`; run affected adapter tests and `go test -race` on those packages. Live conformance records actual binary/model/mode and the enforced effects. A fake spawn test alone cannot close L1.

### L2 — Make reasoning configurable, recoverable and attributable

**Read:** ADR0012; `daemon/waldo_reasoning.go`, `daemon/daemon.go`, `ports/{llm,intelligence_provider,intelligence_run_store}.go`, `service/intelligence/{intake_adapter,llm}.go`, `service/outcome/plan_intelligence.go`, `service/intake/service.go`, `storage/sqlite/store/intelligence_run_store.go`; existing settings/credential mechanisms before adding one.

Implementation:

1. Add reasoning readiness/configuration to the existing settings/onboarding flow. Explicit provider, model and configured/ready/error state; never return a stored secret to the renderer. Reuse a secure local secret abstraction; if none exists, implement and document one under application-state policy. Environment remains a development override with explicit precedence. Never store plaintext keys in Work rows, prompts, logs or git.
2. Missing/invalid key is an actionable retryable setup state. No deterministic floor or alternate-provider fallback. Keep inference billing ownership clear in setup. Test packaged launch without shell environment inheritance.
3. Use controllable HTTP clients/test servers for both LLM adapters. Test actual serialized request, structured response, refusal, invalid JSON/domain output, incomplete response, 401, 429, timeout and cancellation. Keep vendor API differences at the edge; verify current official provider docs when changing SDK request semantics.
4. Persist known requested/effective provider/model and usage/duration without overwriting historical provenance. Generic `waldo-llm` may remain the implementation identity; do not infer vendor solely from model text. Unknown usage must not be presented as measured zero cost. Add fields only through canonical migration/API paths when necessary.
5. Consume nonterminal intelligence runs during boot reconciliation. A daemon-owned synchronous call from a dead process cannot remain displayed as actively thinking. Terminalize/reconcile with a stable reason, preserve input/proposal binding and allow an explicit retry. Do not automatically retry paid calls after an ambiguous result or create execution Attempts.
6. Ensure status-persistence failures are surfaced/logged safely and leave inspectable recovery state. Link the actual run to its produced proposal/intake revision through existing binding seams where applicable; an orphan digest is not enough for user-facing provenance.

**Tests/falsifiers:** no-key fresh profile has a recovery action; canary key absent from database/logs/API output; timeout/cancel/crash can be retried without duplicate proposal binding; late response cannot overwrite a newer revision or terminal run; provider A failure never calls B. Explain whether SDK retries remain enabled and bound them explicitly.

**Narrow commands:** `go test ./internal/daemon ./internal/service/intake ./internal/service/intelligence ./internal/service/outcome ./internal/adapters/llm/... ./internal/storage/sqlite/store` from backend; settings/Understand renderer tests and typecheck. Finish with one controlled real reasoning call per advertised adapter, not with fixtures alone.

### L3 — Ground Contract/Plan proposals and support meaningful replan

**Read:** `ports/intelligence_provider.go`, `service/intelligence/llm.go`, `service/outcome/{plan,plan_intelligence}.go`, `domain/plan_draft.go`, Project Brief and intake proposal/revision stores, `IntakeContractReview.tsx`, `OutcomeDecideAuthorizeSurface.tsx`.

Implementation:

1. Add a bounded repository-context snapshot to the existing intelligence requests: selected repo identity/revision, dirty-state indication, applicable instructions, Project Brief, relevant file excerpts and discovered check commands. Use explicit allowed roots; exclude secrets, ignored dependency/build trees, binary/oversized files; do not follow symlinks outside the allowed root. Bound bytes/files/time with named operational policy. Reading a package script is not authorization to run it.
2. Include substantive previous proposal and relevant clarification/conversation context, not merely its title or opaque refs. Preserve source attribution and distinguish inspected facts from model assumptions. Record a digest of the actual bounded model input. Avoid a new indexing/vector platform for launch.
3. Remove unconditional clarification-answer → TemporalCondition coercion. Carry the answer in clarification context; only explicit temporal semantics populate a temporal field. Test a non-temporal answer such as “preserve email login”.
4. Retain material Plan assumptions/blockers in canonical proposal review data, or reject unsupported blockers explicitly; do not silently discard them. Approval must surface unresolved blockers rather than authorizing a plan that merely omitted them. Preserve criterion coverage and least-privilege compilation.
5. Add explicit replan/revise semantics through the existing Plan service/API. Ordinary reload remains idempotent; deliberate replan with feedback creates a new immutable proposal. Carry expected revision/idempotency. Never mutate an approved Plan in place or launch work on edit. Keep scope small: conversational feedback/replan is sufficient; a graph editor is not required.
6. Test missing context honestly: fail/clarify where material, or show assumptions; never claim repo inspection when none occurred. Pre-authorization context gathering must not write the repo or run provider execution.

**Tests/falsifiers:** a small repo with a distinctive test command produces a request containing that inspected command/path; ignored canary secret is absent; symlink escape is excluded; same revision reload does not call the model again; explicit feedback generates a new proposal with history; stale response cannot bind current authority; cycles/missing criterion coverage still reject. No permanent runtime state may come from prose parsing.

**Narrow commands:** intelligence/intake/outcome/domain/store tests plus affected renderer tests. Record at least one live Contract/Plan grounded in a disposable real repo with zero pre-approval execution sessions.

### L4 — Review the whole Plan and project scheduler truth into Mission Control

**Read:** `service/outcome/scheduler.go`, `httpd/controllers/{outcomes,dto}.go`, operation/spec sources, `frontend/src/renderer/hooks/useOutcome.ts`, `components/outcome/{OutcomeDecideAuthorizeSurface,OutcomeRunSurface,OutcomeMissionControl,WorkShell}.tsx`.

Implementation:

1. Expose the existing GetSchedule derived view through a read-only Outcome API. Return approved Plan identity/current revision, per-unit state and reasons, dependencies, active Attempt, next runnable unit, and relevant exact binding/proof summary. Extend existing controller service interfaces; do not build another scheduler in HTTP or React.
2. Generate OpenAPI/TS together and add HTTP/spec parity tests. Validate stale/unapproved/wrong-Outcome requests. Schedule reads must have no execution side effects.
3. Render all WorkUnits in Plan review with criterion coverage, dependency order, provider/model selection semantics, routing explanation and authorization. Show provider-default as that semantic; effective model may remain unknown until reported. Do not imply discovery from a bundled model list.
4. Replace first-array-entry start with daemon-selected runnable identity. Remove the normal frontend harness field and Project-role query. Keep deliberate legacy API validation if still needed; correct the false “daemon ignores harness” comment. Test provider preference mutation from the actual UI request through controller.
5. Keep one Mission Control product concept for direct/composed shapes using existing components. Serial states must explain executing, dependency proof pending, custody held, no candidate, failure and unknown. Session board visuals may be reused only as subordinate Attempt history, not primary units of responsibility.
6. Query invalidation follows existing CDC/query patterns. Do not persist a second stage or poll the whole Project to infer eligibility. L5 owns automatic progression; until integrated, Start/Continue must truthfully name the next eligible WorkUnit and never silently restart a finished unit.

**Tests/falsifiers:** JSON unit order B,A with B→A dependency displays/runs A first; all approved units are visible; blocked B cannot start; preference A→B after approval still starts bound A; double click uses idempotency; API unknown/custody state disables unsafe action; wrong-plan schedule is rejected. Inspect actual rendered daemon-backed screen and keyboard flow.

**Narrow commands:** backend outcome/controller/apispec tests, `npm run api`, frontend typecheck and existing Plan/Run/Mission tests. Keep generated parity clean.

### L5 — Connect artifacts, verification, serial advancement and delivery

**Read:** ADR0009; `service/outcome/{scheduler,proof,attempt,recover}.go`, existing proof ports/domain/store, runtime/workspace observations, `OutcomeProveCloseSurface.tsx`, Attempt run brief and session/worktree creation.

Implementation:

1. Trace where each Attempt workspace starts. Prove whether a downstream unit sees upstream changes. Choose the smallest existing custody-compatible mechanism for serial artifact handoff (retained governed workspace or an explicit attributed artifact/base transfer). Do not assume a new worktree contains A's edits. Do not implicitly merge into the user's main branch. Record base/result revision and dirty artifacts; stop on ambiguous ownership/conflict.
2. Capture bounded execution receipts using existing canonical facts and storage conventions: producer Attempt/unit/Plan/Contract, workspace/base/result, changes/artifact references, command/results, unresolved items and termination facts. Introduce only missing receipt persistence, not a parallel status database. Provider claims remain claims; process exit is not proof of the criterion.
3. Add an authorized deterministic check runner using approved check specifications. Model-suggested commands require the applicable execution authority. Retain executable/cwd/args, start/end/exit, output artifact/digest and timeout/cancel result. Run in the intended workspace through the same authority boundary, not via unrestricted HTTP shell execution.
4. Write Evidence and Verification through existing canonical service methods with exact unit/Attempt lineage and content digest of the retained artifact. Manual owner observation remains available and labeled as owner evidence. A digest of summary+URL is metadata, not artifact integrity. Never relabel manually typed “passed” as an observed automated check.
5. Reuse criterionReady/proof validation for the schedule. Do not accept Outcome-only or wrong-lineage evidence just to unblock dependencies. Show contradictions and stale proof explicitly; preserve independent verification classification.
6. Advance eligible approved work from a daemon-owned reconciliation trigger, not React useEffect. Reuse StartAttempt admission/idempotency/fences; deterministic start identity scoped to approved Plan+unit+authorized run generation. Pause/failure/retry/replan are explicit states: no automatic infinite retries of an unproved unit. Unknown process/effect blocks advancement. Define cancel as stopping continuation, not merely killing a process then immediately respawning it.
7. Integrate ProveClose with captured evidence. Select actual artifacts/criteria; eliminate routine producer-ID typing and “latest evidence” assumptions. Keep acceptance owner-only.

**Tests/falsifiers:** A creates a file; B reads that exact artifact after daemon restart; A's verified proof admits B once; missing artifact/wrong revision blocks B; failed/contradictory check blocks completion; forged producer label cannot manufacture independent observed verification; cancellation prevents respawn; duplicate receipt ingestion is idempotent; provider success cannot accept. Test in real Git worktrees and SQLite, then real provider execution.

**Narrow commands:** outcome/proof/store/workspace/runtime tests plus race on touched concurrency packages; ProveClose/Run renderer tests and generated parity. No live external deployment needed.

### L6 — Make return, rework and normal navigation Outcome-first; focus launch and document owner use

**Read:** `renderRunBriefPrompt`, recovery/acceptance services, current Project Brief/context, `_shell.work.tsx`, Sidebar/TaskComposer/NewTaskDialog entry points, WorkShell and Outcome surfaces.

Implementation:

1. Build a bounded continuation packet from canonical Contract/current Plan, previous Attempt receipt, retained artifacts, verified/failed criteria, open questions and current authority. Use it for replacement/rework. Do not replay immortal transcripts or invent accepted facts from a summary.
2. Surface a returning-owner summary: desired result, current activity/blocker, changed artifacts, check outcomes, unresolved scope and next owner decision. Models may phrase it; daemon facts determine state/actions.
3. Request rework/reopen with understandable targets selected from existing units/Plan/Contract; no raw-ID form for ordinary work. Preserve immutable history, proof horizon and stale-parent rules. A newer revision must invalidate old proof where required, not erase it.
4. Audit every normal new-work entry and Outcome click. New work goes through Outcome→Contract→Plan approval. Keep legacy sessions as explicit inspection/history and preserve deep links. Project registration/configuration must not start a persistent orchestrator. Hide unsupported Home/decomposition/external-effect controls as appropriate.
5. Clean only demonstrably obsolete touched code, duplicate queries and misleading comments. Require caller/historical-read evidence before deleting compatibility code. Keep safety/quirk rationale; avoid new abstractions that merely restate the architecture.

**Tests/falsifiers:** restart/reopen restores context with no transcript assembly; owner requests changes and next Attempt receives prior artifact/failure context; accepted Outcome stays inspectable; Outcome clicks never auto-open a provider route; zero-provider Project registers without session creation; unknown state has an action rather than fake success. Verify desktop keyboard, back/forward, focus, empty/error states and reduced motion.

### L7 — Release rehearsal and measured optimization

Run section 6 on the integrated SHA, then the real-daemon desktop acceptance matrix in section 7. Follow `docs/development.md` and the repo-local `kennel-ux-auditor` skill for a dedicated UX audit; use isolated state and a disposable repo, never the user's live profile. Store shareable evidence with secrets redacted.

Measure baseline before optimization: Contract/Plan latency and tokens; readiness/spawn latency; daemon memory/query volume and UI responsiveness with representative Outcome/Attempt history. Record hardware, runtime/model, input size, sample count and before/after. Remove duplicate fetches, bound context, reuse unchanged-revision snapshots and fix measured hotspots. Do not remove authority checks or introduce caching that reuses stale proof/bindings.

Test fresh packaged installation without inherited shell env, upgrade from pre-PR99 state, app/daemon restart during reasoning/execution/checks, offline/provider unavailable states, rework and accepted history. Every advertised provider capability needs its own conformance row. An installed CLI is not proof of enforced execution or effective-model reporting.

## 5. Timing and launch decisions

| Target | Milestone | Decision if missed |
|---|---|---|
| First implementation session | L1 runtime boundary proved; L2 configuration underway | Limit provider/capability breadth; never weaken authority |
| Wednesday 9 Sep | grounded proposals and full Plan/schedule UI | Re-estimate; do not add parallelism or redesign |
| Thursday 10 Sep | real two-unit artifact/proof/continuation loop | Without this, there is no evidence the hero feature works |
| Friday 11 Sep | freeze; independent packaged rehearsal | Authority, duplicate execution, false proof or broken install is release no-go |
| Saturday 12 Sep | founder-led onboarding and fixes | Supervised pilot only if public-release gates remain incomplete; say what is unsupported |

Dates may slip; invariants do not. Completion is an external owner reaching evidence-backed acceptance, then voluntarily bringing another Outcome within seven days. Track where founder intervention was necessary.

## 6. Verification commands and evidence levels

Run from repository root after `npm run bootstrap` (use Node version in `.nvmrc`). Baseline results are in [the source-check record](../../verification/2026-09-08-post-pr99-launch-baseline.md); they are not future slice evidence.

```bash
npm run frontend:typecheck
npm --prefix frontend test
npm run lint
npm run test:foundation
npm run sqlc
npm run api
git diff --exit-code -- backend/internal/storage/sqlite/gen backend/internal/httpd/apispec/openapi.yaml frontend/src/api/schema.ts
npx @redwoodjs/agent-ci run --all
```

After changing generated contracts, commit expected generated changes first, then rerun generation to prove no drift. Do not interpret expected uncommitted generated changes as drift from the intended new source.

```bash
cd backend
go build ./...
go test ./...
go test -race ./...
go vet ./...
```

```bash
cd frontend
npm run typecheck
npm run build
npm run package:identity
```

A missing runtime/network/credential/dependency is **blocked**, not pass. Fixture tests, real SQLite/HTTP tests, live adapter conformance, real Electron journeys and owner acceptance are different evidence levels; report them separately. Do not launch external paid/provider work without the user's applicable authorization/configuration.

## 7. Integrated acceptance matrix

All are open until observed on the release SHA. The expected falsifier is any deviation from the stated result.

| ID | Probe | Expected result / evidence |
|---|---|---|
| ISC1 | Fresh profile, zero execution providers, register Git Project | Project persists after restart; zero execution/orchestrator sessions |
| ISC2 | No reasoning key, then configure one | Clear remediation; real editable Contract; zero execution before approval |
| ISC3 | Repo-specific intent + clarification | Inspected context identifiable; non-temporal answer stays non-temporal; constraints preserved |
| ISC4 | Plan with B serialized before dependency A | Both reviewed, dependencies/routing/grants visible; A starts first |
| ISC5 | Change Project provider/model/permissions after approval | Frozen execution semantics preserved; no frontend mismatch and no authority widening |
| ISC6 | Duplicate start and conflicting request-key replay | Exactly one matching Attempt; conflicting semantics refused |
| ISC7 | Inspect-only worker attempts write/external effect | Denied at runtime, controlled canaries unchanged; no prompt-only safety claim |
| ISC8 | A creates artifact, passes check; restart before B | B consumes exact retained artifact and starts once after scoped proof |
| ISC9 | Failed/stale/wrong-lineage/contradictory evidence | No downstream or acceptance readiness based on that evidence |
| ISC10 | Kill renderer/daemon, quiet provider, cancel | No duplicate Attempt; ambiguity stays unknown; cancel does not respawn |
| ISC11 | Return, review, request rework | Goal/changes/proof/open work visible without terminal; bounded context survives |
| ISC12 | Provider exits successfully | No automatic AcceptanceDecision; owner can accept only through governed UI/API |
| ISC13 | Reopen/upgrade historical state | History readable, stale proof correctly scoped, no migration rewrite/data loss |
| ISC14 | Desktop navigation/accessibility | Outcome primary, keyboard/focus/back/forward/error/reduced motion work |

Use a tiny real Git repo with a meaningful two-step change: A changes behavior and emits an artifact; B verifies/uses that artifact. Include a seeded failing test and later correction. Inspect actual file contents and retained check output, not just provider messages. Record unit/Attempt/Plan/revision IDs and compare counts before/after restart. Do not create evidence claiming a check ran merely to advance the UI.

## 8. Required slice handoff

```text
Slice: Lx
Base SHA / head SHA / branch / PR:
Current-code delta (what already existed):
Changed behavior and files:
Canonical API/storage changes and generated artifacts:
Regression reproduced before fix:
Commands + exit codes + evidence paths:
Live provider / runtime / model / capability tested:
ISC rows: pass / fail / blocked, with evidence:
Known limitations / compatibility retained:
STATUS and ledger updated:
Next slice and unresolved dependency:
```

Suggested prompt for the next implementer:

> Implement L1a only from docs/superpowers/plans/2026-09-08-outcome-control-plane-mvp-reset.md. Start from latest beta in an isolated branch. Read AGENTS and the L1a source packet; first produce the current-code delta and reproduce the documented Manager.Spawn exact-model failure. Preserve approved binding, custody, migrations and user-only acceptance. Run the specified tests and real runtime conformance for any capability you claim. Return the required slice handoff; do not merge or expand to other slices without authorization.

Subsequent prompts substitute a ready slice ID; never omit its dependencies. A simpler model should have a small bounded task and concrete falsifiers, not a request to “finish the whole MVP.”
