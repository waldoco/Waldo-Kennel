# Project Registration + Provider Admission Implementation Plan

> **Superseded for vNext (2026-09-15):** Historical implementation plan; retained for provider-binding provenance. Use [persistent mission runtime](../../architecture/persistent-mission-runtime.md) and the [persistent-session execution map](../../roadmap/persistent-session-execution-map.md). Retained for provenance.


> **For implementers:** Execute this plan test-first. Preserve the canonical `Project -> Outcome -> ContractRevision -> PlanRevision -> WorkUnit -> Attempt -> AgentSessionRef` lineage and the five-provider surface (Codex, Claude Code, OpenCode, Cursor, Pi). Do not add hidden provider defaults or fallback chains.

**Goal:** Make Project registration succeed independently of provider readiness, bind the exact worker provider into the immutable authorized WorkUnit, and make Attempt execution admit only that bound provider with truthful remediation when setup is incomplete.

**Architecture:** Project existence and mutable provider configuration remain separate from execution authority. `ProjectConfig.Worker` is the explicit mutable default for future direct work; `ProjectConfig.Orchestrator` is an optional explicit coordinator and is never inferred from the worker. Plan proposal reads the registered Project configuration, freezes the selected worker into `WorkUnit.Provider`, and includes that provider in the RunBrief core digest. Attempt start executes only that frozen provider. Legacy plans without a provider stay readable but cannot execute; callers must configure a worker and create a fresh plan. Provider installation/auth/profile readiness remains a runtime admission concern enforced by the existing agent/session machinery.

**Tech Stack:** Go 1.x daemon/domain/services, SQLite + sqlc, chi/OpenAPI codegen, React 19 + TypeScript + TanStack Query/Router, Vitest/Testing Library, Electron Forge.

## Spec

- Selecting a valid repository creates the Project before provider selection or readiness checks.
- A registered Project may have neither worker nor coordinator configured.
- Provider setup remains available after Project creation and reuses `/api/v1/agents`, `/api/v1/agents/refresh`, `/api/v1/agents/{agent}/probe`, and Project config update routes.
- `ProjectConfig.Worker.Harness` is the only Project-level default used for direct WorkUnit provider binding. No Codex/default-first/installed-first fallback is permitted.
- `ProjectConfig.Orchestrator.Harness` is independent and optional. An absent coordinator never reuses the worker.
- New direct plan proposals require an explicit worker provider. The resulting `WorkUnit.Provider` is immutable and is included in `RunBriefCoreDigest`.
- Legacy persisted WorkUnits with an empty provider remain readable. Starting them fails before Attempt persistence or provider spawn with a typed provider-unbound error and recovery guidance to configure a worker and propose/approve a fresh plan.
- An optional legacy `StartAttemptInput.Harness` may remain as a compatibility assertion for this PR: empty means execute the WorkUnit binding; equal means execute it; different means fail before durable writes. It never selects the runtime provider.
- Provider readiness is checked only for the bound provider immediately before durable Attempt admission. No fallback provider is attempted.
- `ResolveMissionRoles` must not manufacture Codex assignments when Project preferences/role selections are absent.
- Project Settings must preserve explicit non-Codex worker/coordinator selections and must not label them retired or normalize them to Codex.

## Global Constraints

- Target branch is `beta`; implementation branch is `fix/project-registration-provider-admission`, created from `9c15272d489841c119f3e5a8ef0811a5aa5c711f`.
- Add a new SQLite migration; never edit migration `0100_outcome_plan_authority.sql` or other merged migrations.
- Edit sqlc source query/schema/migration inputs only; regenerate `backend/internal/storage/sqlite/gen/*` with `npm run sqlc` rather than hand-editing generated files.
- If DTO/API response shapes change, regenerate OpenAPI and `frontend/src/api/schema.ts` together with `npm run api`.
- Backend truth lands before frontend projection changes.
- No unrelated Home, Work List, scheduler/DAG, WorkspaceLease, Island, Mission Control, reviewer-policy, or provider-adapter redesign.

## Task 1: RED — freeze provider identity into RunBrief semantics

**Files:**
- Modify: `backend/internal/domain/outcome_plan_test.go`
- Later modify: `backend/internal/domain/outcome_plan.go`

**Steps:**
1. Add a test proving two otherwise-identical WorkUnits with different providers produce different `ComputeRunBriefCoreDigest` values.
2. Add a compatibility test proving an empty provider WorkUnit remains structurally readable/valid so pre-migration plans are not corrupted into unreadable history.
3. Run `cd backend && go test ./internal/domain -run 'Test.*RunBrief|Test.*WorkUnit'` and confirm the provider-digest test fails for the intended reason.
4. Do not touch production code until that failure is observed in CI/local execution.

## Task 2: GREEN — add durable WorkUnit provider storage and digest binding

**Files:**
- Modify: `backend/internal/domain/outcome_plan.go`
- Create: `backend/internal/storage/sqlite/migrations/0112_work_unit_provider.sql`
- Modify: `backend/internal/storage/sqlite/queries/outcomes.sql`
- Modify: `backend/internal/storage/sqlite/store/outcome_store.go`
- Modify: `backend/internal/storage/sqlite/store/outcome_store_test.go`
- Modify: `backend/internal/storage/sqlite/migrate_outcome_plan_test.go` or add a focused `migrate_work_unit_provider_test.go`
- Regenerate: `backend/internal/storage/sqlite/gen/outcomes.sql.go`
- Regenerate: `backend/internal/storage/sqlite/gen/models.go`

**Steps:**
1. Add `Provider AgentHarness` to `domain.WorkUnit` without making empty provider fail generic `Validate`; legacy rows must remain readable.
2. Add provider to the canonical `runBriefCore` serialization and update the comment: provider identity is now part of the frozen authorization core.
3. Add migration `0112_work_unit_provider.sql` with a non-destructive `provider TEXT NOT NULL DEFAULT ''` column on `work_units`. Empty means legacy/unbound, not Codex.
4. Update sqlc create/list WorkUnit query source to write/read `provider`.
5. Run `npm run sqlc` and commit generated output.
6. Update store mapping to persist/read `WorkUnit.Provider`.
7. Add store/migration round-trip tests for a non-Codex provider and for legacy empty-provider readability.
8. Run narrow domain/storage tests, then `cd backend && go test ./internal/domain ./internal/storage/sqlite/...`.

## Task 3: RED — plan proposal requires and freezes explicit Project worker

**Files:**
- Modify: `backend/internal/service/outcome/plan_test.go`
- Modify if shared fake lives elsewhere: `backend/internal/service/outcome/*_test.go`
- Later modify: `backend/internal/service/outcome/service.go`
- Later modify: `backend/internal/service/outcome/plan.go`

**Steps:**
1. Extend the test fake/store seam so a Project record can be returned for an Outcome's Project ID.
2. Add a failing test: Project with `Worker.Harness = HarnessClaudeCode` proposes a plan whose sole WorkUnit binds Claude Code.
3. Add a failing test: Project with no worker configured returns typed `PLAN_PROVIDER_UNBOUND` and persists no new plan.
4. Add a failing test: an existing legacy proposed plan with empty provider is not replayed after a worker is configured; a new provider-bound proposal is created instead.
5. Add a failing test: provider readiness is not required to propose a plan; only explicit capability-admitted Project configuration is required.
6. Run `cd backend && go test ./internal/service/outcome -run 'Test.*ProposePlan.*Provider|Test.*Legacy.*Plan'` and confirm intended failures.

## Task 4: GREEN — resolve Project worker at proposal time

**Files:**
- Modify: `backend/internal/service/outcome/service.go`
- Modify: `backend/internal/service/outcome/plan.go`
- Modify: `backend/internal/daemon/daemon.go` only if explicit wiring is clearer than structural store detection

**Steps:**
1. Add a narrow Project-config lookup interface (`GetProject(ctx, id)`) to the Outcome service and wire the production SQLite store into it. Prefer explicit `WithProjectConfigSource`/constructor wiring if it keeps test dependencies obvious; do not broaden `ports.OutcomeStore` with unrelated Project methods.
2. During `ProposePlan`, resolve the Outcome's Project ID, read the registered Project, and take exactly `Project.Config.Worker.Harness`.
3. If empty, return typed `PLAN_PROVIDER_UNBOUND` with recovery guidance; do not select another provider.
4. Validate that the configured harness is known and selectable for worker work; reject impossible persisted config rather than substituting.
5. Set `unit.Provider` before computing the RunBrief digest.
6. Replay an existing proposed plan only if it already carries the same explicit provider binding. A legacy unbound proposal must not block creation of a new bound proposal.
7. Run the Task 3 tests until green.

## Task 5: RED — Attempt admission executes only the frozen provider

**Files:**
- Modify: `backend/internal/service/outcome/attempt_test.go`
- Modify: `backend/internal/httpd/controllers/outcomes_attempts_test.go` if request compatibility needs HTTP coverage
- Later modify: `backend/internal/service/outcome/attempt.go`

**Steps:**
1. Update the happy-path fixture so approved plans carry a provider binding instead of relying on empty-input Codex behavior.
2. Add failing test: empty request harness starts exactly the provider frozen in WorkUnit.
3. Add failing test: request harness different from WorkUnit provider returns `ATTEMPT_PROVIDER_MISMATCH`, creates zero Attempt rows/fences/session refs, and makes zero spawner calls.
4. Add failing test: legacy approved WorkUnit with empty provider returns `PLAN_PROVIDER_UNBOUND`, creates zero durable rows, and makes zero spawner calls/readiness calls.
5. Keep/extend the existing not-ready test to prove the exact bound provider is checked and no fallback is attempted.
6. Run `cd backend && go test ./internal/service/outcome -run 'TestStartAttempt.*Provider|TestStartAttemptAdmissionOrdering|TestStartAttemptFailClosedQuartetLeavesZeroRows'` and confirm intended failures.

## Task 6: GREEN — remove runtime provider selection/fallback

**Files:**
- Modify: `backend/internal/service/outcome/attempt.go`
- Modify: `backend/internal/httpd/controllers/outcomes.go` comments only if behavior wording is stale
- Modify: `backend/internal/httpd/controllers/dto.go` only if schema descriptions need clarification

**Steps:**
1. Add typed error constants `PLAN_PROVIDER_UNBOUND` and `ATTEMPT_PROVIDER_MISMATCH` (names may be adjusted only if an existing repository vocabulary is found before coding).
2. Read the sole approved WorkUnit provider and reject empty binding before readiness checks.
3. Treat `StartAttemptInput.Harness` only as an optional equality assertion; remove the `HarnessCodex` default.
4. Probe readiness for the frozen provider and spawn exactly that provider.
5. Ensure all new refusal paths remain before `CreateAttemptWithFence`.
6. Run Task 5 tests until green, then all Outcome service/controller tests.

## Task 7: RED/GREEN — remove hidden Codex role resolution

**Files:**
- Modify: `backend/internal/domain/projectconfig_test.go`
- Modify: `backend/internal/domain/projectconfig.go`
- Modify service/controller tests that assert resolved Mission roles as needed

**Steps:**
1. Add tests proving empty Project preferences + empty role overrides resolve to unassigned roles, not Codex.
2. Add tests proving explicit worker and explicit coordinator remain distinct and preserve non-Codex identities.
3. Add a test proving a worker-only provider is never promoted to coordinator.
4. Change `ResolveMissionRoles` so absence stays unresolved with `Eligible/Ready` false and a truthful reason. Preserve explicit capability-admitted selections.
5. Run `cd backend && go test ./internal/domain -run 'Test.*ResolveMissionRoles'` plus affected service/controller tests.

## Task 8: RED — Project creation persists before provider setup

**Files:**
- Create or modify: `frontend/src/renderer/components/CreateProjectFlow.test.tsx`
- Modify: `frontend/src/renderer/components/CreateProjectAgentSheet.test.tsx`
- Modify/create shell helper tests around `createProjectConfig` and project creation if present
- Later modify: `frontend/src/renderer/components/CreateProjectFlow.tsx`
- Later modify: `frontend/src/renderer/routes/_shell.tsx`

**Steps:**
1. Add a failing flow test: selecting a validated repository invokes Project creation even when no worker is ready/selected.
2. Add a failing test: creating with no coordinator does not issue an orchestrator session spawn.
3. Add a failing config-builder test: empty worker/coordinator yields a provider-less Project config rather than Codex/default preferences.
4. Add/retain tests proving explicit Codex, Claude Code, OpenCode, Cursor, and Pi worker identities round-trip without rewriting.
5. Run `cd frontend && npm test -- --run CreateProjectFlow CreateProjectAgentSheet` (or exact Vitest file paths) and confirm intended failures.

## Task 9: GREEN — Project-first UI with truthful provider remediation

**Files:**
- Modify: `frontend/src/renderer/components/CreateProjectFlow.tsx`
- Modify: `frontend/src/renderer/components/CreateProjectAgentSheet.tsx`
- Modify: `frontend/src/renderer/routes/_shell.tsx`
- Modify: locale strings only where new copy is required

**Steps:**
1. Change the creation callback contract so repository registration can be invoked with no provider configuration and returns/retains the created Project identity needed for post-create setup.
2. Persist the Project immediately after repository validation.
3. Do not auto-spawn an orchestrator when `orchestratorAgent` is absent.
4. Reuse the existing Agent inventory in the post-create setup surface. Distinguish `Not installed`, `Authentication required`, `Profile setup required`, and ready states from daemon metadata.
5. Keep `Refresh`/`Check again` wired to existing agent refresh/probe behavior. Do not invent provider login APIs.
6. Allow `Configure later`; the Project remains registered and selectable.
7. When the user explicitly chooses a worker/coordinator after creation, persist those selections through the existing Project config route. Do not recreate the Project/reselect the folder.
8. Run the Task 8 tests until green.

## Task 10: RED/GREEN — Project Settings preserves five-provider choices

**Files:**
- Modify: `frontend/src/renderer/components/ProjectSettingsForm.test.tsx`
- Modify: `frontend/src/renderer/components/ProjectSettingsForm.tsx`

**Steps:**
1. Add failing tests for persisted Claude Code/OpenCode/Cursor/Pi worker selections and a distinct coordinator selection.
2. Add a failing test for an unconfigured worker/coordinator showing empty/unconfigured rather than Codex.
3. Remove Codex-only `hasRetiredProviderConfig` / `hasRetiredRoleConfig` assumptions for worker/coordinator.
4. Initialize worker/coordinator from persisted Project config or empty string, never a brand default.
5. Preserve reviewer behavior unless a focused regression demonstrates this form save path rewrites it unintentionally; reviewer policy is out of scope.
6. Run the focused Project Settings tests and frontend typecheck.

## Task 11: Regenerate contracts and verify storage/API drift

**Files:**
- Regenerate as required: `backend/internal/httpd/apispec/openapi.yaml`
- Regenerate as required: `frontend/src/api/schema.ts`
- Generated sqlc files from Task 2

**Steps:**
1. If WorkUnit is part of API response DTOs, expose `provider` in the WorkUnit response mapping/DTO and run `npm run api`.
2. Run `npm run sqlc` and confirm `git diff` contains only expected generated changes.
3. Run HTTP/spec parity tests covering Plan/Attempt serialization.
4. Run `npm run frontend:typecheck`.

## Task 12: Focused regression + repo verification

**Files:**
- Modify: `docs/STATUS.md` only if current implementation claims would otherwise be stale.

**Steps:**
1. Run narrow backend tests for domain, Outcome service, project service/resolution, controllers, and SQLite migration/store.
2. Run narrow frontend tests for creation flow, provider setup, Project Settings, and shell config/spawn behavior.
3. Run `cd backend && go build ./...`.
4. Run `cd backend && go test ./...`.
5. Run `cd backend && go test -race ./...`.
6. Run `cd backend && go vet ./...`.
7. Run `npm run frontend:typecheck`.
8. Run `npm run lint`.
9. Run `npm run api` and `npm run sqlc`; verify generated trees are clean afterward.
10. Run `npx @redwoodjs/agent-ci run --all` when the environment supports it.
11. For the user-visible creation/setup change, run the real daemon + Electron renderer path and verify: valid repository with zero ready providers registers; setup status is truthful; selecting a ready provider persists; no coordinator means no orchestrator spawn; provider-specific failures do not fallback.

## Task 13: Review and PR

**Steps:**
1. Inspect `git diff beta...fix/project-registration-provider-admission` / GitHub compare for scope creep and provider-name defaults.
2. Search changed code for `HarnessCodex`, literal `"codex"`, `default`, and fallback logic; each remaining occurrence in touched execution paths must be intentional and documented.
3. Run the verification-before-completion checklist and request code review.
4. Address review findings with tests first where behavior changes.
5. Open a focused PR targeting `beta` with: diagnosis, architecture change, migration/backward-compat behavior, exact tests, and intentional omissions.
6. Do not merge automatically.