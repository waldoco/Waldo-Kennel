# Plan: packaged macOS Outcome journey harness + UX flaw audit

Status: **plan only, not yet approved, not yet implemented.** Instinct's independent verification (2026-09-16) returned 10 amendments; all 10 are applied below (§0.5/§0.8/§3.1/§3.3/§3.4/§4/§5/§6/§8/§9/§10/§12 all changed). This revision is returned for a short re-check before implementation, per the working protocol.

Baseline to branch from once approved: `origin/outcome-loop` at `7b09a71fe0016489ed6b728ca09f7322cf4d8211` (current local `outcome-loop` tracking ref matches). New branch name: `instinct/*candidate*` off that SHA, per the protocol — not chosen here.

---

## 0. Load-bearing facts resolved during investigation

These change the shape of the journey and are called out explicitly per the "no guessed ambiguity" instruction.

### 0.1 Contract creation is genuinely model-gated in the real UI — there is no deterministic shortcut reachable from Work

- The only Work-UI path to create an Outcome is `WorkEnterSurface` → `AdaptiveIntakeSurface` (`frontend/src/renderer/components/outcome/AdaptiveIntakeSurface.tsx`) → intake capture → intake analysis → confirm.
- Intake analysis (`POST /api/v1/intakes/{intakeId}/analysis`) is called from the renderer with `repositoryToolUse: false` and **no `offline` field** (`AdaptiveIntakeSurface.tsx:117,151`). Server-side, `AnalyzeInput.Offline` defaults to `false`, which selects the live analyzer (`backend/internal/service/intake/service.go`, `chooseAnalyzer`). There is a deterministic "offline floor" analyzer in the backend (`AnalyzeInput.Offline`, doc comment: "stops waiting for an agent... never asks an agent anything"), **but no control in the renderer ever sets it**. This is a real, reproducible product gap — see flaw candidate UX-01 below — not a design choice I am inventing.
- `outcomes.create()` / `OutcomeService.Create` (`backend/internal/httpd/controllers/outcomes.go:329`) is a plain deterministic API endpoint (no model call) but it is **not wired to any current Work UI control** I could find (no call site in the renderer). Comment in the handler ("the direct-create path must be able to state one exactly as the Understand surfaces do") confirms it's a parallel/API-only path, not what a real owner clicks.
- **Consequence for the plan:** the journey's Contract-creation step is a real, owner-authored intake statement, analyzed by whatever agent the daemon is configured with (v0 constant `V0_PROVIDER_ID = "codex"`, `WorkEnterSurface.tsx:17`), through the real UI, with no way to avoid a live model round trip while staying inside the actual product surface. The harness must treat **a locally installed and authenticated Codex CLI** as a hard precondition, detect its absence up front, and return `blocked` (not fabricate a Contract) if it is missing. This is consistent with the honesty boundary ("If the current application cannot complete a required pre-execution step, the correct result is `blocked`").

### 0.2 Plan proposal is also live-agent-gated, and goes through a planning *conversation*, not the direct endpoint

- `OutcomeDecideAuthorizeSurface` renders `MissionPlanningConversation` (`frontend/src/renderer/components/outcome/MissionPlanningConversation.tsx`) whenever there is no plan yet. That component uses `usePlanning` hooks (`useStartPlanning`, `useSendPlanningMessage`, `useFinalizePlanning`, `usePlanningCandidates`) which hit `/outcomes/{outcomeId}/planning-sessions`, `/planning-sessions/{id}/messages`, `/planning-sessions/{id}/proposal` — an agent conversation with a selectable candidate (`PlanningAgentPicker`) and a context-grant step (`PlanningContextGrant`, `contextMode: "repository_read"`).
- `useProposeOutcomePlan` (`frontend/src/renderer/hooks/useOutcome.ts:436`, backing `POST /outcomes/{outcomeId}/plans`) has **zero call sites in the renderer** — it is not the real UI path today.
- **Consequence:** the journey must drive the planning conversation (pick a candidate, grant repository-read context, send at least one turn, finalize) exactly as production does, per the "acceptable to invoke the current intelligence-backed Plan proposal seam exactly as production does" instruction. This is the second and only other place the journey depends on live model turns. Both dependencies use the same already-authorized Codex CLI, so there is exactly one external precondition to check, not two.

### 0.3 The real navigation model is one tabbed panel, not route-driven lifecycle stages — corrected after further reading

The first draft of this plan assumed `/work?stage=decide_authorize`, `stage=act_observe`, etc. rendered separate route bodies (`OutcomeDecideAuthorizeSurface`, `OutcomeRunSurface`) directly, per `frontend/src/renderer/routes/_shell.work.tsx`'s `renderStageBody`. That is wrong for the case that matters here (an Outcome selected) and is corrected now, not discovered mid-implementation:

- `renderStageBody`'s first branch — `if (view === "outcomes" || (outcome && project && stage !== "decompose"))` — fires whenever both `outcome` and `project` are set (true for this whole journey from step 6 onward), **before** any of the later `stage === "decide_authorize"` / `"act_observe"` branches are ever reached. Those later branches are live only for the `!outcome` deep-link fallback case (no Outcome selected yet) — they are not what this journey exercises once an Outcome exists.
- The real container is `OutcomeMissionWorkspace` → (once `outcomeId && projectId`) its right-hand panel `OutcomeMissionPanel` (`data-testid="outcome-mission-panel"`, `frontend/src/renderer/components/outcome/OutcomeMissionPanel.tsx`). This is **one panel with five internal tabs** (`contract`, `plan`, `execution`, `result`, `history`) switched by local `useState`, not by URL — the `stage` search param only seeds the tab's *initial* value via `useState(stage === "prove_close" ? "result" : stage === "act_observe" ? "execution" : "contract")`. Every other `stage` value, **including `"decide_authorize"`**, falls through to the `"contract"` initial tab — a real mismatch between the URL's claimed stage and the visibly active tab, noted as a UX finding candidate (UX-04, low: URL/visual-state drift, not a functional blocker).
- Intake confirmation's `openOutcome()` (`AdaptiveIntakeSurface.tsx:37-40`) navigates to `/work?project&portfolio&stage=decide_authorize&outcome=<id>` — per the point above, this actually lands on the **contract tab**, which is exactly what this journey needs next (step 7).
- Tab switching in the real UI happens via: (a) the tab nav buttons inside `OutcomeMissionPanel` (`aria-pressed`, text `t(mission.${view})`, **no `data-testid` today** — gap, see §6); (b) the in-tab "Review plan" shortcut button in the contract tab (`t("outcome.dashboard.reviewPlan")`, **no testid today**); (c) `onReviewContract`/`onReviewWork` callbacks passed into `OutcomeDecideAuthorizeSurface`/wired from `OutcomeMissionPanel` itself (`setTab("contract")`/`setTab("execution")`), which fire from buttons already covered by existing testids (`outcome-review-work`) or the mission-glance CTA (`mission-glance`, its own button has no testid — gap).
- **Corrected step 7 onward:** the journey does not navigate to a new URL/stage to reach the Contract editor, Plan tab, or Execution tab — it clicks tabs (or their equivalent CTAs) inside the one already-open `outcome-mission-panel`. The `/work` URL's `stage` param is set once (by `openOutcome`) and is not otherwise touched by this journey after that.

### 0.4 The post-approval execution boundary is a real, inert control — confirmed safe to observe without triggering

- `OutcomeRunSurface` (`frontend/src/renderer/components/outcome/OutcomeRunSurface.tsx`) and `OutcomeRunControls` (`frontend/src/renderer/components/outcome/OutcomeRunControls.tsx`) are read-only over daemon state (`useOutcomePlan`, `useOutcomeAttempts`, `useOutcomeRunState`) until a button is clicked. The boundary control is `data-testid="outcome-run-start"` (`OutcomeRunControls.tsx:77`, action `"start"`, i18n `mission.run.action.start` = "Start approved Plan"), which calls `POST /outcomes/{outcomeId}/run`. **The journey must assert this control is present/eligible and must never click it or any `outcome-run-{action}` button.** This is the literal `executionStarted: false` boundary the manifest must record.
- Copy at this boundary is already reasonably honest: `outcome.run.needsPlanTitle`/`Body` = "No authorized plan yet" / "Execution only ever starts from an approved plan."; `mission.run.idle` = "Start authorizes automatic continuation of the approved Plan." One softer spot: `outcome.decide.approveNote` = "Execution starts in the next stage" (said immediately after Approve, before the owner has clicked Start) — a candidate low/medium UX finding to confirm live, not a blocker.

### 0.5 Corrected: the real Contract Save path preserves facets today — the first draft's claim was wrong

**This corrects and retracts the first draft's claim of a `critical` finding. Do not approve or build on the earlier claim — read this section as the replacement, not an addendum to it.**

What went wrong: the first draft read `MissionContractEditor.tsx`'s *form fields* (goal/criteria/evidence/review/constraints/nonGoals/stops/authority — no `facets` textarea) and concluded from their absence that the save payload omits facets too. That was reading the form, not the submit handler. A full read of the actual `onSubmit` shows the save call is `void mutation.save({..., facets: contract.facets})` (`MissionContractEditor.tsx:41-47`, specifically line 46) — `contract` is the immutable prior revision passed in as a prop (`ContractDraft`'s `contract` param, ultimately `outcome.currentRevision` from the parent), so the current facets are read back and resubmitted verbatim on every save even though the form has no control to edit them. The component's own test seeds a nonempty facet and asserts it round-trips through the submitted payload — this is exercised, not accidental. **The real Contract tab's Save button does not drop facets.**

There remains one narrower, real observation worth keeping, one layer down and not filed as a finding (see below): `ReviseContract` (`backend/internal/service/outcome/service.go:396-459`) implements wholesale replacement, not a merge — `Facets: append([]domain.ContractFacet(nil), in.Facets...)`. Nothing in this slice's journey exercises that gap (the real UI always resends the current facets), so it is not evidence of anything broken today; it is only worth a one-line note that a *different, hypothetical* client omitting the field would have no server-side merge to protect it.

**Consequence for step 8 (see amended assertion below, §4):** the journey still does not assume the outcome from source reading alone — it captures the accepted Contract's exact facet array from the live API, drives an unrelated edit through the real Save control, and diff-checks after. Given the correction above, PASS (facets identical) is now the *expected* result, not a hopeful one; a live, reproduced mismatch would be surprising and would need its own fresh investigation before any severity claim, not an automatic escalation to `critical` — that label was itself part of the retracted claim.

### 0.6 The folder picker is a native OS dialog, not a web control

- `CreateProjectFlow.tsx`'s "choose folder" button (`onChooseFolder`) calls into `frontend/src/main.ts`'s `dialog.showOpenDialog` (main.ts:1806) over IPC. Playwright cannot drive a native macOS Open panel.
- **Resolution:** use `electronApp.evaluate(({ dialog }) => { dialog.showOpenDialog = async () => ({ canceled: false, filePaths: [fixturePath] }) })` before clicking the real "choose folder" button. This substitutes only the OS picker's *return value* (what folder the owner picked) — every subsequent action is still the real button, the real IPC round trip, and the real `POST /api/v1/projects` call. This is the standard, narrowly-scoped Electron/Playwright technique for native dialogs and does not fall under "UI actions to direct storage writes" (forbidden) because no state is written directly; the app's own project-registration code path runs unchanged.

### 0.7 Daemon teardown is already governed by an existing, tested rule — the harness must not fight it

- `frontend/src/main/daemon-owner.ts`: `keepDaemonAlive(env)` is true only if `KENNEL_KEEP_DAEMON` is an explicit truthy value. The harness must never set that var. Default behavior: an app-owned daemon self-stops shortly after the Electron app quits. Teardown assertion: read the daemon pid from the run file at launch, assert `owner === "app"`, close the app, then bounded-poll (never signal/kill) for that exact pid to be gone.

### 0.8 Ambiguities from the first draft — resolved by Instinct's amendments (2026-09-16)

All four are now settled; kept here as a record of what was asked and how it was answered, not as open questions.

1. **Resolved (amendment 4): UI-only, no exception.** The journey never calls `POST /outcomes/{id}/plans`, `useProposeOutcomePlan`'s backing route, or any other state-advancing endpoint directly — every state transition happens only through the real UI. API calls are read-only assertions/evidence exactly as originally proposed, *plus* a new requirement: the harness captures every network call the UI itself emits (method/path/status/correlation id) and asserts none of them are a "shortcut" endpoint outside the UI-driven flow (see §4's amended negative assertion). Endpoint-contract coverage of `ProposePlan`/`Create` in isolation, if ever wanted, belongs in a separate deterministic backend integration test — not this journey, and not this slice.
2. **Resolved (amendment 5): fixed ceilings, not owner-tunable defaults.** 30s Codex preflight, 180s intake-analysis ceiling, 180s per planning-provider wait, 10-minute overall journey ceiling. Poll interval 1-2s with state-change logging. See §5 for the full replacement of the first draft's provisional 360s/600s numbers.
3. **Resolved (amendment 6, corrected by final amendment 1): click through it, but prove the boundary out-of-band, not from the UI's own pixels.** `PlanningContextGrant`'s real UI shows only generic `repository_read` mode text — it does not and cannot display which repository is bound, so the journey cannot assert a fixture path from that screen. The boundary is instead proven two ways that do not depend on the grant screen showing anything: (a) before authorizing, assert the UI shows and has `repository_read` selected (mode only, no path); (b) independently of that screen, `GET /projects/{id}` and assert `.path === fixturePath` — proving which repository the *session* is actually scoped to, from the daemon's own record, not from what the grant screen displays. After authorizing, record the context mode/digest and assert the planning session's own durable state recorded `repository_read` with that bounded context. The missing path display in the grant UI is itself recorded as a UX finding once the live pixels confirm it (§8, new candidate). See §4/§6 for the added assertion steps.
4. **Resolved (amendment 7): owner-run-only, by design, not by CI limitation alone.** The live packaged journey runs only on the owner's Mac against an already-authenticated local Codex. CI validates schemas, typecheck, and the harness's own deterministic self-tests (§11 gates 1-4) — CI never drives a live Codex session, on principle (credential/cost exposure on shared runners), not merely because no macOS runner exists today. A live-journey-in-CI lane is an explicit future opt-in requiring its own credential and cost policy, not a default this slice assumes.

---

## 1. What already exists and will be reused, not rebuilt

| Need | Existing seam | File |
|---|---|---|
| Real packaged Electron launch + isolated port/data-dir/run-file + pid-attributed daemon readiness | `real-app.spec.ts` pattern (`_electron.launch`, `freePort`, `readRunFile`, poll `/readyz`) | `test/e2e-pod/real-app.spec.ts` (Linux pod today; same technique, new macOS-targeted config) |
| Direct-spawn env contract for a packaged bundle (`KENNEL_RUN_FILE`, `KENNEL_DATA_DIR`, `KENNEL_PORT`), bundle version/executable introspection via `plutil`, bounded `waitFor` polling | `e2e-mac-update.mjs` | `frontend/scripts/e2e-mac-update.mjs` |
| Update-settings pre-seed to dodge the first-run update dialog | `seedUpdateSettings` | `frontend/scripts/e2e-mac-update.mjs` |
| Packaged build identity assertion (bundle ID, executable, protocol, updater target, daemon name, state namespace, release repo) | `package:identity` | `frontend/scripts/assert-package-identity.mjs` |
| macOS artifact signing/notarization check (not required for a local unsigned dev build; usable if the owner runs against a signed build) | `verify-mac-artifact.sh` | `scripts/verify-mac-artifact.sh` |
| Pure verdict-classification precedent (`passed`/`app_failed`/`infra`, tested in isolation) — model for this harness's `passed`/`failed`/`blocked` | `deriveGateOutcome` | `scripts/kennel-e2e-pod-gate.mjs` |
| Dependency-free `.mjs` harness convention with pure exported functions unit-tested by vitest | `e2e-mac-update.mjs` + `.test.mjs` | `frontend/scripts/*.test.mjs` |
| Outcome/Contract/Plan API surface (exact routes to assert against) | `OutcomesController.Register` | `backend/internal/httpd/controllers/outcomes.go:81-123` |
| Intake API surface | `IntakesController.Register` | `backend/internal/httpd/controllers/intakes.go:46-62` |
| Existing stable `data-testid`s on most Outcome/Work surfaces | see §4 selector table | `frontend/src/renderer/components/outcome/*.tsx` |
| Work route/stage model (`understand` → `decide_authorize` → `act_observe` → `prove_close`) | `WorkRoute` | `frontend/src/renderer/routes/_shell.work.tsx` |

Nothing here requires a second e2e framework, package launcher, daemon manager, or Outcome client. The plan extends the Linux pod's real-app pattern to a macOS-targeted Playwright project and reuses the mac-specific scripts already written for the update harness.

---

## 2. High-level architecture

One new Playwright config + spec pair, driven by one new dependency-free orchestration script (mirroring `e2e-mac-update.mjs`'s shape), producing one evidence directory per run.

```
scripts/outcome-journey/
  run-outcome-journey.mjs        # CLI entry: build check, profile setup, launch, teardown, manifest write, exit code
  run-outcome-journey.test.mjs   # vitest: pure helpers (arg parsing, manifest shape, redaction, exit-code mapping)
  fixture-repo.mjs               # disposable local git fixture creation
  fixture-repo.test.mjs
  process-guard.mjs              # stale process / wrong-identity / reused-profile detection
  process-guard.test.mjs
  manifest.mjs                   # typed manifest builder + schema version + redaction
  manifest.test.mjs
  ux-flaws.mjs                   # typed flaw record builder + ranked synthesis + markdown renderer
  ux-flaws.test.mjs

test/e2e-pod/                    # reused, NOT duplicated — see note below
  playwright.electron.config.ts  # existing Linux pod config, untouched

test/macos-outcome-journey/
  playwright.config.ts          # new: no webServer, one project, one spec, macOS-only guard
  outcome-journey.spec.ts       # the Playwright test that drives the actual journey steps
  support/
    dialog-stub.ts              # electronApp.evaluate dialog.showOpenDialog override (§0.6)
    daemon-wait.ts              # run-file + /readyz/healthz polling, reused from real-app.spec.ts shape
    screenshot.ts               # named checkpoint capture (renderer + native screencapture)
```

Why `test/macos-outcome-journey/` and specifically **not** `frontend/e2e/`: `frontend/playwright.config.ts` has `testDir: "e2e"` with a `"work"` project whose only exclusion is `grepInvert: /@legacy-board/` — it recursively globs everything under `frontend/e2e/`, with no `testMatch`/`testIgnore` narrowing. A spec placed anywhere under `frontend/e2e/` (including a new subfolder) would be silently picked up by the existing `npm run test:e2e` "work" project, launched against the `dev:web` Vite dev server with no Electron at all, and fail there — breaking verification gate 3 (`npm --prefix frontend test`) for a reason unrelated to this slice. Root-level `test/` already holds `test/e2e-pod/`, the repo's existing home for "real packaged app, no webServer" Playwright configs, distinct from `frontend/e2e/`'s dev-web browser suite — so a sibling `test/macos-outcome-journey/` directory matches that existing convention instead of colliding with `frontend/playwright.config.ts`'s glob. Not reusing `test/e2e-pod/` itself: that harness is Linux/xvfb/.deb-specific (`APP_BIN` env, `--no-sandbox`), and its config's `testMatch` is pinned to `real-app.spec.ts`; a macOS `.app` journey needing real window compositing (for `screencapture`) is different enough that forcing it in would mean conditional branches inside a script whose header explicitly says "do not simplify" the existing Linux assumptions. The *pattern* (isolated port/data-dir/run-file, direct executable spawn, pid-attributed readiness) is reused verbatim as a shared helper, not reinvented.

The orchestration script (`scripts/outcome-journey/run-outcome-journey.mjs`) is the thing a human or CI runs. It:
1. Validates preconditions (git SHA, clean-enough tree note, macOS host, Codex CLI present+authenticated, no stale Kennel processes, no port 3001 or ephemeral port collision).
2. Builds/packages at the recorded SHA if not already built (`npm run build:daemon && npm run package` under `frontend/`), or accepts a pre-built `.app` path via `--app` flag (so re-runs don't always rebuild).
3. Runs `package:identity` (reused) as the build-identity gate.
4. Creates the fixture repo and isolated profile dir.
5. Invokes Playwright (`playwright test -c test/macos-outcome-journey/playwright.config.ts`) with env pointing at the exact `.app`, fixture path, and profile dir.
6. Collects the Playwright JSON reporter output + native `screencapture` files + daemon/app logs into the evidence directory.
7. Writes `manifest.json` and `ux-flaws.json`/`ux-flaws.md`.
8. Exits non-zero on anything but `passed`.

---

## 3. Clean, deterministic setup

### 3.1 Build identity
- Record `git rev-parse HEAD` and `git status --porcelain` (dirty flag) before packaging.
- `cd frontend && npm run build:daemon && npm run package` (unsigned local build unless `APPLE_SIGNING_IDENTITY`/`CSC_LINK` are set in the environment — record which).
- Run `npm run package:identity` (`frontend/scripts/assert-package-identity.mjs`) and fail closed if it fails — this is the single existing gate for "wrong build identity."
- Resolve the produced bundle path deterministically: `frontend/out/Kennel-darwin-${arch}/Kennel.app` (from `PRODUCT_NAME="Kennel"`, `frontend/src/shared/product-identity.ts`). Read `CFBundleExecutable` via `plutil` rather than hardcoding `kennel` (mirrors `e2e-mac-update.mjs`'s `readExecutableName`).
- Hash the `.app`'s `Contents/MacOS/<exe>` and the extracted daemon binary under `Contents/Resources/daemon` (SHA-256) for the manifest.
- Record signing status honestly: `codesign -dv` output presence/absence; do **not** gate on `verify-mac-artifact.sh` (that script requires notarization/stapling, which a local dev build will never have). `verify-mac-artifact.sh` is offered as an optional flag for an owner testing an actual signed release build, recorded as a separate manifest field (`signingVerified: true|false|"not_attempted"`).

**Hard build-identity preflight (amendment 3), a mandatory abort gate, not just a manifest field:** `frontend/scripts/build-daemon.mjs` already embeds the exact building `git rev-parse HEAD` into the daemon binary via `-ldflags -X .../daemonmeta.BuildRevision=<sha>` (line 68), and the daemon's own `/healthz`/`/readyz` already return it as `buildRevision` (`backend/internal/httpd/router.go:343`). The harness reuses this existing mechanism rather than inventing one: after the daemon reaches ready (§5), fetch `buildRevision` from `/readyz` and compare it byte-for-byte against the `git rev-parse HEAD` the orchestration script recorded before packaging. **Any mismatch aborts the run as `failed`, before any UI action is attempted** — this is exactly the class of error (inspecting a build that does not correspond to the tree actually read) that produced the first draft's incorrect facets conclusion, and this gate exists specifically to make that mistake structurally impossible for the harness itself, not just for a human reviewer reading source. Record `build.gitSha` (recorded pre-package), `build.daemonBuildRevision` (read live from `/readyz`), and `build.identityMatch: true|false` in the manifest (§7.1).

### 3.2 Isolated profile + fixture repo
- Profile root: `mkdtemp` under the OS temp dir (or an owner-supplied `--work-dir`), e.g. `.../kennel-outcome-journey-<runId>/`.
  - `KENNEL_DATA_DIR = <profile>/data`
  - `KENNEL_RUN_FILE = <profile>/running.json`
  - `KENNEL_PORT = <free ephemeral port>` (via the same `net.createServer().listen(0, ...)` trick as `real-app.spec.ts`)
  - Pre-seed `<dirname(KENNEL_RUN_FILE)>/update-settings.json` via the exact `seedUpdateSettings` shape from `e2e-mac-update.mjs` (updates disabled: `{enabled: false, channel: "latest", nightlyAck: false, feature: null}`) so the first-run update dialog never blocks a non-interactive run.
  - **Never** set `KENNEL_KEEP_DAEMON` (see §0.7).
- Fixture repo: a fresh directory under the same profile root (e.g. `<profile>/fixture-repo`), `git init`, one deterministic seed commit (a README + a trivial source file) authored with fixed `GIT_AUTHOR_*`/`GIT_COMMITTER_*` env so the SHA is reproducible across runs on the same harness version. Record the fixture path and its initial commit SHA in the manifest.
- Run IDs: one `runId` (ULID or timestamp+random) minted once, used as the profile/fixture directory suffix and every screenshot/log filename prefix, so every artifact is attributable to the run that produced it. **Correction after reading `AdaptiveIntakeSurface.tsx` in full (§6 item 2):** the harness does not mint its own `RequestKey` for intake capture/confirm — the real UI already mints and caches one per action (`requestKey("capture")`/`requestKey("confirm")`, refs at lines 36/41/171/222) so a UI-level retry is already idempotent by construction. The harness's job is to *read* that key off the captured request for its ledger (§7.3), not supply it. `CreateInput.RequestKey`/`outcomes.go:353` (the Outcome direct-create endpoint, `POST /projects/{id}/outcomes` — distinct from `POST /projects`, the real project-registration endpoint the journey does use) is not called by this journey at all, per §0.8's amendment-4 resolution (UI-only, no shortcut endpoints).

### 3.3 Detection / fail-fast checks (before touching the UI)
- Stale process: `pgrep -f Kennel.app` (or equivalent) — if any Kennel process is running outside this harness's own child tree, fail with `stale_process` rather than silently colliding.
- Wrong build identity (bundle metadata): `package:identity` result (§3.1).
- Wrong build identity (source tree): live daemon `buildRevision` (from `/readyz`) equals the recorded `git rev-parse HEAD` (§3.1 amendment 3) — abort before any UI step if it does not.
- Reused profile data: assert the profile dir is freshly created by this run (mkdtemp guarantees this) and `KENNEL_DATA_DIR` did not exist before launch.
- Wrong repository: after project registration, `GET /api/v1/projects/{id}` and assert its path equals the fixture path exactly.
- Missing auth/capability: probe Codex CLI presence (`which codex` equivalent, or the daemon's own `GET /api/v1/agents` inventory — `AgentInventory.installed`/`authorized`, already read by `WorkEnterSurface.tsx`'s `fetchAgents`) before launching the UI flow at all. Missing/unauthorized → `blocked`, not attempted.
- Unexpected pre-existing Outcome: after project registration, `GET /projects/{id}/outcomes` must return empty before intake starts.

### 3.4 Cleanup and idempotent reruns (amendment 8)

- Every run gets its own temp repo/profile/evidence-output directory (`runId`-suffixed, §3.2) — never a shared or reused path across runs.
- On failure, the evidence directory (screenshots/logs/manifest so far) is **retained**, not deleted — a failed run's whole value is in what it captured; the manifest's `cleanup.retained` list (§7.1) records exactly what was kept and why.
- Teardown kills only processes this run's orchestration script itself spawned (the Electron app it launched, tracked by pid from the moment `_electron.launch` returns) — never a broader `pkill`/pattern match that could catch an unrelated Kennel instance the owner is running.
- The harness never mutates the developer's normal profile: `KENNEL_DATA_DIR`/`KENNEL_RUN_FILE`/`KENNEL_PORT` always point inside the run's own temp directory (§3.2), never at `~/.kennel` or any ambient default.
- The manifest's `cleanup` object (§7.1) is the run's own report of this: `daemonPidObservedGone`, `retained` (paths kept, with a reason for each), and nothing else — no silent state left behind unaccounted for.
- A second clean-profile rerun immediately after a first (§11 gate 6) must produce a distinct `runId`/Outcome/Plan with zero dependency on the first run's leftover state — this is the concrete proof that reruns are actually idempotent, not just asserted to be.

---

## 4. Journey steps, UI actions, API assertions, screenshots

Each step names: the real UI action, the `data-testid` (or the gap, with the proposed minimal fix), the API read used as the authoritative assertion, and the screenshot filename.

| # | Step | UI action (real Work UI) | Selector | API assertion | Screenshot |
|---|---|---|---|---|---|
| 1 | Clean entry | Launch app, land on Enter/Work | `work-shell`, `enter-blocked-daemon`/`enter-blocked-provider` (must be absent) | `GET /healthz`, `GET /readyz` on the discovered port; daemon pid from run file matches launch | `01-clean-entry.png` |
| 2 | Register fixture project | `CreateProjectFlow` → "create new" or "import workspace" → stub `dialog.showOpenDialog` (§0.6) → submit | **gap:** no `data-testid` on the flow's primary buttons today (aria-label/i18n text only) — add minimal testids (§6) | `POST /api/v1/projects` response `id`/`path`; `GET /api/v1/projects/{id}` matches fixture path | `02-project-registered.png` |
| 3 | Outcome creation form (before submit) | `AdaptiveIntakeSurface` intake capture form: type a fixed statement into `<textarea id="outcome-statement">`, click the icon-only submit button | `intake-project-switcher` present; `intake-statement-input`/`intake-capture-submit` (added, §6) | n/a (pre-submit) | `03-outcome-create-form.png` |
| 4 | Intake analysis (live agent) | Submit capture → `POST .../analysis` fires automatically per current UI wiring (confirmed in `AdaptiveIntakeSurface.tsx`'s capture-effect, no separate "analyze" click exists) | `intake-analysis-waiting` (`IntakeAnalysisWaiting.tsx`) while pending | Poll `GET /intakes/{intakeId}` until `session.status` leaves `analyzing`; record wall-clock duration | `04-intake-analysis-waiting.png` |
| 5 | Contract proposal review | `IntakeContractReview` renders the agent's proposal | `intake-contract-review`, `intake-proposal-summary`, `intake-authority-editor` | `GET /intakes/{intakeId}` proposal payload | `05-intake-proposal-review.png` |
| 6 | Confirm → Outcome + initial Contract created | Click the confirm button (`onClick={() => void confirm()}`, `AdaptiveIntakeSurface.tsx:384-386`) — `confirm()` itself calls `openOutcome()` on success, so the app navigates automatically, no separate "open Outcome" click needed | `intake-confirm` (added, §6) | `POST /intakes/{intakeId}/confirmation` response `confirmedOutcome.id`; then `GET /outcomes/{outcomeId}` — record `currentRevisionNumber` (expect `1`) | `06-outcome-created.png` |
| 7 | Contract tab (default, already open) | `openOutcome()` lands directly on `outcome-mission-panel` with the `contract` tab active — no navigation click needed | `outcome-mission-panel`; **gap:** zero `data-testid`s in `MissionContractEditor.tsx` today — add minimal set (§6) | n/a (editor open, pre-save) | `07-contract-tab-open.png` |
| 8 | Edit + save revision (outcome-neutral, evidence-first — §0.5, amendment 2) | Before touching the UI: `GET /outcomes/{outcomeId}` and record the accepted Contract's exact `facets` array (and `authorityCeiling`/`stopConditions`/`temporalCondition`) verbatim. Then click `contract-edit`, make one **unrelated** edit (goal/criterion text only — nothing that touches facets, since the form has no facets control anyway), click `contract-save`, and capture the exact revision request body the UI sent over the wire | added testids (§6) | `POST /outcomes/{outcomeId}/revisions` with `ExpectedRevision: 1` → response `currentRevisionNumber === 2`. Fetch the new revision and deep-compare `facets` (and the other unedited fields) against the pre-save snapshot, plus record the captured request body's own `facets` field alongside it. **PASS** (no finding) if identical — this is the expected, default outcome per the §0.5 correction, which found the real Save path preserves facets from source. If the live diff ever shows a mismatch, that is unexpected and gets its own fresh investigation and severity judgment at that point — it is not pre-labeled `critical` here, since that label was part of the retracted first-draft claim | `08-contract-revised.png` |
| 9 | Restart, prove durable revision | **Close and relaunch** the packaged app against the *same* `KENNEL_DATA_DIR`/profile (not just a page reload — proves SQLite durability + run-file/port rediscovery, per advisor guidance), then re-navigate to `/work?project=X&outcome=Y` | `work-shell` ready again, then `outcome-mission-panel` (contract tab, its default) | `GET /outcomes/{outcomeId}` returns `currentRevisionNumber === 2` and the edited text, from the fresh process; re-run the same facets/authority/stop-condition diff from step 8 to confirm the post-restart state matches what step 8 observed (not further changed); record both launch identities (pid/port) in the manifest | `09-contract-revised-after-restart.png` |
| 10 | Move to the Plan tab (tab model made an explicit assertion — amendment 10) | Click the `plan` tab nav button (added testid, §6) or the in-tab "Review plan" CTA (added testid, §6) | `outcome-mission-panel` tab now `plan` | n/a beyond the tab-model assertion itself: **capture the `/work` URL's `stage` query param immediately before and after the click and assert it is unchanged** — this is the harness's own proof that tab switching is local `useState`, not a route navigation (§0.3), reached by driving the real tab controls rather than by navigating to a `stage=` URL directly | `10-plan-tab-open.png` |
| 11 | Prove the grant boundary before authorizing it (amendment 6, corrected by final amendment 1) | `MissionPlanningConversation`: pick candidate (`PlanningAgentPicker`), open `PlanningContextGrant` — **before clicking to authorize, assert the UI shows and has `repository_read` mode selected** (the real grant screen shows only generic mode text, no repository path — do not assert a path from these pixels) | `mission-planning`; grant-review UI (exact selector confirmed during implementation, add a testid if the mode text has none) | **Independently of the UI**, `GET /api/v1/projects/{id}` and assert `.path === fixturePath` — this is what actually proves the session is bound to the disposable fixture, not the grant screen's own display | `11-grant-boundary-review.png` |
| 12 | Plan proposal (live agent conversation) | Click through the `repository_read` grant, click `planning-start`, exchange ≥1 turn, finalize | `planning-start`, `planning-session-status`, `planning-turns`, `planning-turn-{n}` | Poll `GET /outcomes/{outcomeId}/plan` until `status: "proposed"`; record planning session id and turn count. **Afterward (amendment 6, second half):** record the context mode/digest the grant produced and assert the planning session's own durable state recorded `repository_read` with the expected bounded context — not just that the button was clicked | `12-plan-proposing.png` (mid-conversation) and `13-plan-proposed.png` (finalized) |
| 13 | Plan review | `PlanReviewCard`/`MissionPlanView` renders the proposed Plan | `outcome-plan-card`, `outcome-plan-graph` or `outcome-plan-work-units` | `GET /outcomes/{outcomeId}/plan` — record `id`, `contractRevisionNumber` (must equal 2), `workUnits[].id` | `14-plan-review.png` |
| 14 | Approve (immediately before commit) | Focus/hover the Approve button, capture just before click | `outcome-approve-plan` | n/a (pre-click) | `15-approve-before-commit.png` |
| 15 | Approved | Click `outcome-approve-plan` | same testid, now disabled/gone; `outcome-review-work` appears | `POST /outcomes/{outcomeId}/plans/{planId}/approval` with `expectedContractRevision: 2`; response plan `status: "approved"`; record approval/decision id if the response exposes one | `16-plan-approved.png` |
| 16 | Real execution boundary | Click `outcome-review-work` (fires `onReviewWork` → `setTab("execution")` inside `OutcomeMissionPanel` — a tab switch, not a route navigation, per §0.3) | `outcome-run-surface`, `outcome-run-schedule`; **must assert `outcome-run-start` is present/eligible and must NOT click it** | `GET /outcomes/{outcomeId}/run` (or the schedule read) confirms plan approved, zero attempts (`GET /outcomes/{outcomeId}/attempts` → `[]`) | `17-execution-boundary.png` (full window, plus a cropped detail of the Start control showing it untouched) |
| — | Every distinct failure/dead-end encountered | Whatever state actually occurs | n/a | n/a | `failure-<n>-<slug>.png`, captured opportunistically whenever a step's expected next-state check fails |

**UI-only negative assertion (amendment 4), checked across the whole journey, not one step:** the harness captures every network request the packaged renderer itself emits (method, path, status, correlation/request key) into the ledger (§7.3) and asserts none of them is a state-advancing call outside this table's own API-assertion column — specifically, `POST /outcomes/{id}/plans` (the unused direct `ProposePlan` route, §0.2) and `POST /projects/{id}/outcomes` (the unused direct `Create` route, §0.1) must never appear as a request the harness itself issued. Reads (`GET`) are unrestricted, since every API call in this table's assertion column is a read used as evidence, never a substitute action.

Packaged-mode proof (cheap, one-line, applied at every checkpoint's assertion, not just once): renderer origin must be `app://renderer` (not `http://127.0.0.1:5173`), `daemon-status` `data-state="ready"`, and no preview-mode markers (`usesPreviewWorkspaceData`/mock workspace text absent). This is the advisor's suggested strongest single assertion that this is not a dev renderer, and it's cheap to assert on every screenshot step, not just the first.

---

## 5. Waiting, not sleeping

Every wait is a bounded poll on a named condition, mirroring `waitFor` in `e2e-mac-update.mjs`. Ceilings are now fixed (amendment 5), replacing the first draft's provisional/owner-TBD numbers:

- Codex preflight (§3.3's missing-auth/capability check): 30s.
- Daemon ready: poll run-file existence → port → `/readyz` (as `real-app.spec.ts` already does), timeout 40s.
- Intake analysis settling: poll `GET /intakes/{intakeId}` for `session.status` to leave `analyzing` (proposed/needs-clarification/failed), timeout **180s**, poll interval 1-2s, log every state change observed.
- Planning conversation reaching a finalized proposal: poll `GET /outcomes/{outcomeId}/plan` for `status: "proposed"`, timeout **180s per provider wait** (a provider wait is any single request/response round trip inside the conversation — start, each message, finalize — not the whole conversation), poll interval 1-2s, log every state change observed.
- Contract revision / plan approval: these are synchronous HTTP responses — no polling needed beyond the request itself; the *UI* re-render is asserted via `expect.poll` on the relevant testid becoming visible, timeout 15s.
- App relaunch liveness: identical run-file→port→`/healthz` poll as step 9.
- Daemon exit on teardown: poll `process signal 0` (or `ps -p <pid>`) for the recorded pid to disappear, timeout 30s, never send a signal ourselves.
- **Overall journey ceiling: 10 minutes**, wall-clock from launch to the execution-boundary screenshot, independent of the per-step ceilings above — the run aborts if the sum of steps runs long even when no single step timed out.

**On any timeout (amendment 5 and 9):** record the condition being waited on, elapsed time, last-observed value, last API response body, a daemon log tail, and a screenshot — this is what "useful timeout evidence" means operationally here. **No silent extension of a ceiling, and no retry of a live mutation.** One run makes exactly one attempt per request key (the UI's own key, per §3.2's correction) — if a request was already accepted (2xx) and the *effect* of that acceptance (e.g., the analysis actually settling) then times out, the correct manifest state is **ambiguous, not retried**: record the last durable state observed and stop, per amendment 9's BLOCKED/FAILED/ambiguous split (§9).

---

## 6. Selector additions (exact minimal set, attribute-only)

All additions are a bare `data-testid` on an existing element — no new components, no behavior change, each covered by a focused render test (existing `.test.tsx` sibling file for that component).

1. `CreateProjectFlow.tsx`: `data-testid="create-project-new"` / `create-project-import"` on the two entry buttons (`onClick={() => onSelect(...)}`), `create-project-choose-folder` on the folder button, `create-project-submit` on the final submit button, `create-project-error` on the error `<p role="status">`.
2. `AdaptiveIntakeSurface.tsx` (confirmed by full read, not a guess): `data-testid="intake-statement-input"` on the capture `<textarea id="outcome-statement">` (line ~266), `intake-capture-submit` on the icon-only capture submit button (`type="submit"`, line ~289-301), `intake-confirm` on the confirm button (`onClick={() => void confirm()}`, line ~384-386). Confirmed while reading: request keys (`requestKey("capture")`, `requestKey("confirm")`) are minted **client-side by the UI itself** and cached in a ref so a retry reuses the same key (lines 36, 41, 171, 222) — the harness does not need to supply its own `RequestKey`; it only needs to read whatever key the real UI generated off the wire for its request/response ledger (§7.3). This corrects §3.2 below.
3. `MissionContractEditor.tsx`: `data-testid="contract-edit"` (the "edit" trigger button), `contract-goal"`, `contract-criterion-{index}"`, `contract-evidence-{index}"` on the corresponding textareas, `contract-save"` on the submit button, `contract-discard"` on discard.
4. `OutcomeMissionPanel.tsx`: `data-testid="mission-tab-{view}"` on each of the five tab-nav buttons (`contract`/`plan`/`execution`/`result`/`history`, line ~139-148), `mission-review-plan-cta` on the in-contract-tab "Review plan" button (line ~155), `mission-glance-cta` on `MissionGlance`'s next-action button (line ~281-288). These are the real navigation controls discovered in §0.3 — without them the journey has no reliable way to move between tabs other than brittle i18n text matching.

No selector change alters runtime behavior (attribute-only diffs); each gets a one-line addition to that component's existing test file asserting the attribute renders on the right element, not new behavioral coverage.

---

## 7. Evidence packet

### 7.1 Manifest (`manifest.json`)

Typed via a small JSDoc/TS shape in `manifest.mjs`, versioned:

```jsonc
{
  "schemaVersion": "1.0.0",
  "runId": "…",
  "startedAt": "…", "completedAt": "…",
  "result": "passed | failed | blocked | ambiguous",
  "build": {
    "gitSha": "…", "gitBranch": "…", "gitDirty": true,
    "appVersion": "…", "bundleId": "in.heywaldo.kennel",
    "executablePath": "…/Kennel.app/Contents/MacOS/kennel",
    "executableSha256": "…", "daemonSha256": "…",
    "daemonBuildRevision": "… (live, from /readyz's buildRevision)",
    "identityMatch": true,
    "signingIdentity": "unsigned | <identity> | \"not_attempted\"",
    "signingVerified": false,
    "macosVersion": "…", "arch": "arm64 | x86_64",
    "packageIdentityCheck": "passed | failed"
  },
  "profile": { "profileDir": "…", "dataDir": "…", "runFile": "…", "port": 12345 },
  "fixture": { "repoPath": "…", "initialCommitSha": "…" },
  "steps": [
    // failureCode is one of: null (passed), "blocked_<reason>", "failed_<reason>",
    // or "ambiguous_after_accept" (§9's third classification — a mutating
    // request was accepted but its effect never settled within its ceiling).
    { "name": "register-project", "startedAt": "…", "endedAt": "…", "durationMs": 0, "result": "passed", "failureCode": null }
  ],
  "identities": {
    "projectId": "…", "outcomeId": "…",
    "contractRevisions": [1, 2],
    "planId": "…", "planningSessionId": "…",
    "planningContextGrant": { "mode": "repository_read", "digest": "…" },
    "approvalDecisionId": "…",
    "requestKeys": ["…"],
    "observedEventIds": ["…"]
  },
  "apiAssertions": [
    { "method": "POST", "path": "/api/v1/outcomes/{id}/revisions", "status": 200, "assertedField": "currentRevisionNumber", "expected": 2, "observed": 2 }
  ],
  "networkLedger": {
    "shortcutEndpointsObserved": [],
    "note": "amendment 4: must always be empty — POST /outcomes/{id}/plans and POST /projects/{id}/outcomes are never called by this journey"
  },
  "facetsCheck": {
    "step": 8,
    "identical": true,
    "preSaveFacets": "…", "postSaveFacets": "…", "postRestartFacets": "…",
    "note": "§0.5/amendment 1-2: identical=true is the expected default (source-confirmed the real Save path preserves facets); a false value here is unexpected and would need its own investigation before any severity claim, not an automatic critical"
  },
  "artifacts": [
    { "path": "screenshots/06-outcome-created.png", "sha256": "…" }
  ],
  "cleanup": { "daemonPidObservedGone": true, "retained": ["fixture-repo (kept: false)"], "reason": null },
  "executionStarted": false,
  "boundaryReached": "act_observe: outcome-run-start eligible, not clicked",
  "limitations": [
    "Contract and Plan proposal both required a live authorized Codex CLI turn; not a fully model-free deterministic proof.",
    "Signing/notarization not verified for this (local/unsigned) build."
  ],
  "unverifiedClaims": []
}
```

Missing required evidence, a screenshot capture failure, wrong packaged identity, a build-revision mismatch (§3.1 amendment 3), an unexpected preview/dev route, an observed shortcut-endpoint call, or an unverified authoritative state sets `result: "failed"` (`"blocked"` if the cause is a precondition, or `"ambiguous"` if a mutating request was accepted but its effect never settled — per §9's taxonomy and final amendment 2, this is its own top-level `result` value, not flattened into `"failed"`), never a silently-passing manifest with a hidden warning. The orchestration script's exit code stays nonzero for all three of `failed`/`blocked`/`ambiguous` (§2 step 8) — only `passed` exits zero.

### 7.2 Visual evidence
- Renderer-level `page.screenshot()` per checkpoint in §4, **plus** native `screencapture -l <windowNumber>` (via `app.evaluate` reading the BrowserWindow's native handle, or `screencapture -o -R` of the known window bounds) for at minimum: clean entry, approval-immediately-before-commit, and the execution boundary — proving this is the packaged app's real window frame, not just web content.
- Every screenshot is inspected (by the implementer, then independently by Instinct) for clipping, overlap, contrast, stale state, wrong navigation, misleading copy before the run is called passed — recorded as a checked/unchecked list in the closeout, not asserted by the manifest alone.

### 7.3 Logs
- Packaged app stdout/stderr (Electron main process log).
- Daemon log (the app's captured `daemonOutput`, or the daemon's own log file if one exists under the data dir — confirm during implementation).
- A sanitized request/response ledger: every API call in §4's assertion column, with headers redacted (`Authorization`, cookies) but IDs/status/timing intact.
- Process/launch inventory: pid, port, executable path, launch timestamp for both the initial launch and the step-9 relaunch.

---

## 8. UX flaw catalog

`ux-flaws.json` (typed records) + `ux-flaws.md` (rendered for humans), produced every run regardless of pass/fail, via `ux-flaws.mjs`.

Seed findings already evidenced by source reading (to be confirmed live during the actual run, not asserted from reading alone):

- **UX-01** (category: dead end / forces API-only path): the backend's deterministic "offline" intake-analysis floor (`AnalyzeInput.Offline`) has no UI control anywhere in `AdaptiveIntakeSurface.tsx`; every real Contract creation depends on a live agent with no owner-visible way to skip it. Severity: candidate **medium** (friction/inaccessible action, not data loss). Proposed owner: multi-round intake slice.
- **UX-02** (category: missing/unstable selectors): `CreateProjectFlow.tsx`, `AdaptiveIntakeSurface.tsx`'s capture/confirm controls, `MissionContractEditor.tsx`, and — most consequential — `OutcomeMissionPanel.tsx`'s five tab-nav buttons (the *only* way to move between Contract, Plan, and Execution, carrying nothing but `aria-pressed` and translated text) have no `data-testid`s; automation and any future audit must key off translated i18n text that changes per locale. Severity: candidate **low-to-medium** (the tab nav specifically is a **medium** instance of this category — it's the sole navigation spine for the whole Outcome lifecycle, not a one-off button). `fixedInThisSlice: true` for the four files this slice actually touches (§6) — narrower than the rubric's "almost always false" default, but defensible here because attribute-only testid additions are explicitly inside the brief's "tiny selectors ... when the journey cannot select them reliably otherwise" allowance, not a UI fix being smuggled in under a different name.
- **UX-03** (category: confusing copy / mismatched mental model): `outcome.decide.approveNote` = "Execution starts in the next stage," shown immediately after Approve, before the owner has clicked the separate "Start approved Plan" control in Act & Observe. Live run to confirm whether this reads as misleading in context. Severity: candidate **medium** pending live confirmation.
- **UX-04** (category: stale/misleading status — §0.3): `openOutcome()` navigates with `stage=decide_authorize` in the URL, but `OutcomeMissionPanel`'s tab-seeding ternary has no case for that value, so the visibly active tab is `contract`, not `plan` — a URL/visual-state drift. Severity: candidate **low** (no functional harm, but a `stage` param that doesn't describe what's on screen is exactly the kind of drift that erodes trust in deep links). Proposed owner: elevated-experience pass.
- **UX-05** (category: missing/misleading status at an authority boundary — final amendment 1): `PlanningContextGrant`'s real UI shows only generic `repository_read` mode text before the owner authorizes it — it does not display which repository the grant actually binds. An owner authorizing agent access to their own repository has no on-screen way to confirm it's the repository they think it is; the journey itself has to reach past the UI (`GET /projects/{id}`) to prove that binding, which is exactly the kind of authority-transparency gap the owner cannot do from the product. To be confirmed by live pixels during the run — file only if the screenshot from step 11 actually shows no path/repo identifier anywhere in the grant surface. Severity: candidate **medium** (an authorization step that doesn't show what's being authorized, though not itself a wrong mutation). Proposed owner: elevated-experience pass, alongside the Plan-authority surfaces generally.

**Not filed as a catalog entry (corrected per §0.5 / amendment 1):** the first draft's claim that `MissionContractEditor.tsx` drops facets on save was investigated and is **not reproduced from source** — the real Save path round-trips `contract.facets` verbatim (line 46), covered by an existing component test. It stays out of the catalog for that reason, not because it was downgraded after the fact. Step 8's live before/after/after-restart diff (§4, §7.1's `facetsCheck`) is what would file it — at whatever severity the live evidence actually supports — if the packaged build somehow behaves differently from source; it is not pre-filed at any severity now.

The rest of the catalog is populated live during the actual proof run (loading states, error rendering, accessibility, hierarchy/spacing, etc., per the categories in the brief) — it cannot be fully authored during planning since it requires observing the packaged app's actual pixels and timing.

---

## 9. Result classification: BLOCKED vs FAILED vs ambiguous (amendment 9)

An explicit taxonomy, not left as scattered prose — every step outcome and every run-level `result` must be classified against exactly one of these:

- **BLOCKED** — an environment precondition this slice does not control was not met: no authenticated Codex CLI, no macOS host, a required tool missing. This is not a product flaw and is not filed in the UX flaw catalog. Detected by §3.3's preflight checks, before any UI action.
- **FAILED** — a UI/API invariant broke while the environment was sound: an assertion in §4 did not hold, a required screenshot could not be captured, a manifest field could not be populated, or a live diff (e.g., step 8's facets check) actually showed a mismatch. Every FAILED run gets a corresponding flaw-catalog entry (§8) describing what broke, even if the run's headline classification only needed to say "failed."
- **Ambiguous (ends the run, never retried)** — a mutating request was already accepted (HTTP 2xx observed) but the *effect* of that acceptance did not settle within its ceiling (§5). This is neither BLOCKED (the environment answered) nor cleanly FAILED (the request itself succeeded) — it is preserved as its own manifest state, distinct from an ordinary failure (final amendment 2): top-level `result: "ambiguous"` (not `"failed"`), with `steps[].failureCode: "ambiguous_after_accept"` and the last durable state attached, and the run stops. The orchestration script's exit code stays nonzero for `ambiguous` exactly as it does for `failed`/`blocked` — the distinct enum value is for the manifest reader, not the exit code. **Never retry a request whose acceptance is already ambiguous** — a retry risks a duplicate mutation the harness cannot then attribute correctly, which is exactly the class of problem the UI's own per-action request keys (§3.2) exist to prevent on the product side; the harness must not defeat that protection by inventing a second attempt with a second key.

This taxonomy governs every `result`/`failureCode` field in the manifest (§7.1) and every negative-proof scenario (§10).

---

## 10. Negative-proof scenarios (required verification gate #7)

1. **Wrong build identity (bundle metadata):** `frontend/scripts/assert-package-identity.mjs` reads the *built* bundle directly (`Info.plist` via `plutil`, `app-update.yml` contents) rather than comparing source constants, so two independent levers both work: (a) hand-edit `Contents/Info.plist`'s `CFBundleIdentifier` on an already-packaged `.app` before running the gate, or (b) rebuild with `KENNEL_RELEASE_REPO` set to a fork/mismatched `owner/repo` (`frontend/forge.config.ts`'s `parseReleaseRepo`), which bakes a mismatched `app-update.yml` the script's `owner: waldoco` check will reject. Either way, expect `failed`/`blocked` at the `package:identity` gate, never a silent pass.
2. **Wrong build identity (source tree — a distinct gate from #1, §3.1 amendment 3):** run against a packaged daemon built from a different commit than the one the orchestration script recorded before packaging. `package:identity` checks bundle metadata only and would pass; the separate live `buildRevision`-vs-`git rev-parse HEAD` check (read from `/readyz`) must be the one that catches this and aborts before any UI step — proving the two identity gates are independent, not redundant.
3. **Unavailable daemon/API:** launch with `KENNEL_PORT` pointed at a port nothing will ever bind, or kill the daemon child immediately after spawn → expect the daemon-readiness poll to time out and the run to report `failed` with the exact timeout evidence, not hang.
4. **Deliberately missing required screenshot/state assertion:** delete one expected screenshot file (or stub the capture function to no-op for one checkpoint) in a test double of the manifest writer → expect manifest validation to reject the run as `failed`, proving the "missing required evidence is a failed proof" rule is enforced in code, not just documented.
5. **Missing auth/capability classifies as BLOCKED, not FAILED:** run the preflight with no Codex CLI on PATH (or an unauthorized one) → expect the run to stop at §3.3's preflight with `result: "blocked"`, zero UI actions attempted, and no flaw-catalog entry filed for it (per §9's taxonomy) — this proves BLOCKED and FAILED are not conflated in the actual exit-code/manifest logic.

---

## 11. Verification gates (concrete commands)

1. `npm --prefix frontend test -- <new *.test.tsx files>` — selector-only render tests for the four touched components.
2. `node --test scripts/outcome-journey/*.test.mjs` (or `npx vitest run` if the repo's convention prefers vitest for `.mjs` — match `frontend/scripts/*.test.mjs`'s existing runner) — pure-function unit tests: manifest shape/versioning, redaction, exit-code mapping (`passed/failed/blocked/ambiguous` mirroring `deriveGateOutcome`'s pattern, extended to four states per final amendment 2 — all three non-`passed` values must map to a nonzero exit), fixture/profile helpers, process-guard detection logic.
3. `npm run frontend:typecheck` and `npm --prefix frontend test` (full suite) — must stay green after the selector additions.
4. `cd backend && go test ./...` — should be unaffected (no backend production changes proposed), run anyway as the standard gate.
5. `node scripts/outcome-journey/run-outcome-journey.mjs --app <freshly packaged .app>` on a real Mac with an authenticated Codex CLI — the actual packaged proof. **Owner-run**, matching the existing S1 packaged-proof pattern (`docs/STATUS.md`'s "Fresh authenticated Mac evidence remains required" note) — this repo's CI is Linux-only for backend/foundation gates today (`.github/workflows/`), so CI cannot execute this step.
6. A second clean-profile rerun (`--work-dir` pointed at a fresh empty temp root) immediately after run 5, same `.app`, asserting a distinct `runId`/Outcome/Plan and no dependency on run 5's leftover state.
7. The five negative-proof runs from §10.
8. Manual pixel inspection of every required screenshot in both runs 5 and 6 (checklist in the closeout).
9. Independent review of changed files, manifest, logs, and flaw catalog by Instinct before merge (no merge performed by this work).

CI-backed vs owner-Mac evidence, stated explicitly in the closeout: gates 1–4 and the pure parts of the unit tests are CI-backed (Linux-safe); gates 5–8 are owner-run macOS evidence attached to the PR with exact SHA attribution, since this repository's CI has no macOS runner today.

---

## 12. Forbidden-scope checklist (self-audit against the brief)

- No touch to `backend/internal/service/chat`, governed command/control stores/migrations, delivery DTOs, or the delivery-block frontend slice — confirmed zero references to any of those in the file list above.
- No `startAttempt`/Attempt lifecycle action ever invoked — §4 step 16 explicitly asserts-without-clicking. More broadly, **no `outcome-run-{action}` button of any kind is ever clicked** — `OutcomeRunControls.tsx:71` renders `start`/`pause`/`resume`/`cancel` from the same eligibility-filtered map, so pause/resume/cancel are equally out of bounds, not just start.
- No fake proposal/evidence/Result/acceptance injection — Contract and Plan both come from real live-agent turns (§0.1, §0.2); nothing is seeded into storage directly.
- No direct state-advancing API call as a substitute for a UI action (amendment 4) — the journey is UI-only end to end; §4's negative assertion checks this in code, not only in this checklist.
- No redesign of Outcome UI — the four selector additions in §6 are attribute-only.
- No `beta`, no issues/releases, no merge performed by this work.

---

## 13. Changed-file list (proposed, subject to Instinct's amendment)

New:
- `scripts/outcome-journey/run-outcome-journey.mjs` (+ `.test.mjs`)
- `scripts/outcome-journey/fixture-repo.mjs` (+ `.test.mjs`)
- `scripts/outcome-journey/process-guard.mjs` (+ `.test.mjs`)
- `scripts/outcome-journey/manifest.mjs` (+ `.test.mjs`)
- `scripts/outcome-journey/ux-flaws.mjs` (+ `.test.mjs`)
- `test/macos-outcome-journey/playwright.config.ts`
- `test/macos-outcome-journey/outcome-journey.spec.ts`
- `test/macos-outcome-journey/support/dialog-stub.ts`
- `test/macos-outcome-journey/support/daemon-wait.ts`
- `test/macos-outcome-journey/support/screenshot.ts`
- `docs/` runbook for this journey (prerequisites, invocation, outputs, honest limitations) — path TBD, likely `docs/development.md` addendum or a new `docs/outcome-journey-harness.md` linked from it.

Changed (attribute-only, each with a root cause stated in the actual implementation commit, not here):
- `frontend/src/renderer/components/CreateProjectFlow.tsx`
- `frontend/src/renderer/components/outcome/AdaptiveIntakeSurface.tsx`
- `frontend/src/renderer/components/outcome/MissionContractEditor.tsx`
- `frontend/src/renderer/components/outcome/OutcomeMissionPanel.tsx`
- Their sibling `.test.tsx` files, one assertion each.

Nothing else. No changes to `outcomes.go`, `intakes.go`, or any backend service — this slice is a proof/harness, not a production behavior change.
