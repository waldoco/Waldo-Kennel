# Kennel PR Convergence and Architecture Gate Implementation Plan

> **Classification for vNext (2026-09-15):** historical implementation plan, not current order. Use [persistent mission runtime](../../architecture/persistent-mission-runtime.md) and the [persistent-session execution map](../../roadmap/persistent-session-execution-map.md).


> **Sequencing amendment (2026-08-21):** [ADR 0004](../../adr/0004-parallel-home-personal-agent-and-required-capture.md) permits a separately owned Home/Personal Agent lane to proceed in parallel and makes governed desktop screen/audio capture required product capabilities. The Work PR sequence below remains unchanged, but Work evaluation is no longer a prerequisite for starting Home, and capture is no longer an optional launch+1-only idea. Durable admitted Memory remains separately gated.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve and accept F0-F6, replace PR #11 with bounded post-foundation cleanup, and establish a clean base and ordered vertical-slice sequence for the accepted Waldo Kennel desktop launch.

**Architecture:** F0-F6 remains an indivisible prerequisite. PR #11 is not merged or rebased wholesale: accepted cleanup is reapplied on new branches from the accepted foundation, while its incompatible Outcome schema/store, premature provider deletion, CLI authority assumptions, and locale deletion are omitted. The product architecture resolves the local v0 dogfood gates: Codex admission/recovery through a provider-neutral adapter seam, grounded provider-neutral RunBrief and hybrid orchestration, autonomy-preserving leases/fences, privacy-preserving causal trace, dogfood thresholds, and the consequential-effect ceiling. Provider admission is capability-based (amended 2026-08-24: Codex ready-by-default, DeepSeek Harness admitted for workers after profile readiness); v1's provider set is TBD, while evaluator results use a locked provider-neutral independence classification. Each product feature still requires its own approved issue-sized vertical-slice plan.

**Tech Stack:** Git/GitHub, Go daemon and SQLite, Electron/React/TypeScript, generated OpenAPI/sqlc artifacts, npm foundation scripts

**Spec:** `docs/product/kennel-v1-product-architecture.md`

## Execution status — 2026-08-21

The prerequisite portion of this plan is complete. PR #1 landed F0-F6, PR #12 removed the legacy import surface, PR #13 added provider-neutral Codex admission while preserving historical identities/recovery reads, and PR #14 narrowed public CLI discovery while preserving direct invocation. PR #11 was closed unmerged as a superseded donor. Tasks 1-5 below are retained as the decision and verification record; do not execute their historical branch/SHA commands again. Tasks 6-8 and the separate [First Outcome execution handoff](2026-08-20-first-outcome-execution-handoff.md) define the remaining architecture/implementation gate.

## Global Constraints

- Never place post-foundation cleanup or feature edits on `codex/foundation-gate-f0-f6`.
- Do not merge, deploy, publish, release, force-push, or rewrite public history without explicit authorization.
- Preserve user work and use a separate `codex/` branch/worktree for every implementation slice.
- The primary daemon listener remains unauthenticated and loopback-only.
- All application state remains under `~/.kennel`.
- Durable observable changes use SQLite-triggered `change_log` CDC.
- Already-merged migrations are immutable; new migrations are additive.
- The Electron renderer remains a thin daemon client.
- Generated OpenAPI and frontend TypeScript contracts travel together.
- Provider completion and verification never create user acceptance.

---

### Task 1: Accept the F0-F6 prerequisite without product scope — completed by PR #1

**Files:**
- Verify: `docs/foundation-acceptance-2026-08-18.md`
- Verify: `scripts/test-foundation.sh`
- Verify: `.github/workflows/ci.yml`
- Verify: `.github/workflows/security.yml`

**Interfaces:**
- Consumes: PR #1 at `codex/foundation-gate-f0-f6`
- Produces: one accepted post-foundation base commit for all later branches

- [ ] **Step 1: Confirm the PR head and clean checkout**

```bash
gh pr view 1 --repo Pin4sf/Waldo-Kennel --json state,isDraft,headRefName,headRefOid,baseRefName,mergeable,mergeStateStatus,statusCheckRollup
git status --short --branch
```

Expected: the checkout is clean; the PR head and expected base are explicit. Do not infer acceptance from `mergeable` alone.

- [ ] **Step 2: Install the normalized dependency sets**

```bash
npm run bootstrap
```

Expected: exit 0. Record audit warnings as known development-toolchain debt; do not run broad automatic audit fixes.

- [ ] **Step 3: Run the complete foundation gate**

```bash
npm run test:foundation
npm run audit:production
npm run lint
cd backend && go test -race ./...
```

Expected: every command exits 0. If the fake-agent timing test fails under whole-suite load, rerun it three times and fix or quarantine the deterministic timing assumption before accepting the foundation; do not label a failed full gate green.

- [ ] **Step 4: Record manual evidence while hosted Actions are unavailable**

Record command, timestamp, commit SHA, exit code, and intentional skips in the PR description or maintainer checklist. Do not claim GitHub checks exist when `statusCheckRollup` is empty.

- [ ] **Step 5: Obtain maintainer approval before merge**

Do not merge as part of this task unless the user separately authorizes it. The deliverable is a verified, reviewable foundation prerequisite.

### Task 2: Create the post-foundation cleanup branches — completed

**Files:**
- Inspect: all 71 paths changed by PR #11
- Do not copy: `backend/internal/domain/outcome.go`
- Do not copy: `backend/internal/storage/sqlite/migrations/0099_outcome_control_plane.sql`
- Do not copy: `backend/internal/storage/sqlite/queries/outcomes.sql`
- Do not copy: `backend/internal/storage/sqlite/store/outcome_store.go`
- Do not copy: generated Outcome sqlc artifacts

**Interfaces:**
- Consumes: the accepted F0-F6 base and PR #11 as a donor diff only
- Produces: issue-sized post-foundation branches with no permanent Outcome schema

- [ ] **Step 1: Create an isolated branch from the accepted foundation**

```bash
git fetch origin --prune
task_worktree_root=$(mktemp -d /tmp/waldo-kennel-cleanup.XXXXXX)
git worktree add "$task_worktree_root/worktree" -b codex/remove-legacy-user-import ef0baffd7850702480cba84ae97e2790367eab12
```

Expected: a clean branch whose merge base is the accepted foundation, not the pre-foundation `main` used by PR #11.

- [ ] **Step 2: Capture the donor diff without applying it**

```bash
git diff --stat b1bcd37fd0911a86da5bb21c07ac3a0b9adecb3b...dbebb0956d2e039fefc77bb963c6ef9bc0c7e28b
git diff --name-status b1bcd37fd0911a86da5bb21c07ac3a0b9adecb3b...dbebb0956d2e039fefc77bb963c6ef9bc0c7e28b
```

Expected: all donor files are visible for classification. Do not cherry-pick the PR #11 commit.

- [ ] **Step 3: Classify every donor file**

Give each path exactly one disposition: `reapply`, `rewrite`, `defer`, or `drop`. Outcome persistence files are `drop`; locale deletion is `drop`; provider catalog deletion and public CLI narrowing are `defer` until their contracts are approved.

- [ ] **Step 4: Keep one issue per branch and PR**

Legacy-import removal, provider admission, and later CLI narrowing are separate review units. Do not recreate PR #11 as another mixed cleanup/feature commit.

### Task 3: Remove only the user-facing legacy import flow — completed by PR #12

**Files:**
- Delete: `backend/internal/cli/import.go`
- Delete: `backend/internal/cli/import_test.go`
- Delete: `backend/internal/httpd/controllers/imports.go`
- Delete: `backend/internal/httpd/controllers/imports_test.go`
- Delete: `backend/internal/legacyimport/`
- Delete: `backend/internal/service/importer/`
- Delete: `frontend/src/renderer/components/MigrationPopup.tsx`
- Delete: `frontend/src/renderer/components/MigrationPopup.test.tsx`
- Delete: `frontend/src/renderer/components/MigrationSection.tsx`
- Delete: `frontend/src/renderer/hooks/useMigrationOffer.ts`
- Modify: `backend/internal/cli/root.go`
- Modify: `backend/internal/httpd/api.go`
- Modify: `backend/internal/httpd/apispec/specgen/build.go`
- Modify: `backend/internal/httpd/controllers/dto.go`
- Modify: `frontend/src/renderer/main.tsx`
- Modify: `frontend/src/renderer/components/settings/GeneralSettingsSection.tsx`
- Modify: `backend/internal/skillassets/using-ao/SKILL.md`
- Modify: `backend/internal/skillassets/using-ao/references.md`
- Modify: `frontend/src/landing/content/docs/cli.mdx`
- Modify: `frontend/src/landing/content/docs/plugins/index.mdx`
- Regenerate: `backend/internal/httpd/apispec/openapi.yaml`
- Regenerate: `frontend/src/api/schema.ts`

**Interfaces:**
- Consumes: existing thin CLI and code-first HTTP contract
- Produces: no user-facing probing/import of legacy AO state while preserving developer import and documented source/project compatibility seams

- [ ] **Step 1: Write failing boundary assertions**

Add or update tests proving `kennel import` and import API operations are absent, the migration offer is not rendered, and active skill/landing documentation contains no `ao import` instruction. Preserve `devimport`, `.kennel/attachments`, `.kennel/launch.json`, `backend/cmd/kennel`, Go-module, and provenance/sync seams.

- [ ] **Step 2: Run the focused tests and confirm failure**

```bash
cd backend
go test ./internal/cli ./internal/httpd/... ./internal/skillassets/...
cd ..
npm --prefix frontend test -- --run Migration
rg -n 'ao import|kennel import' backend/internal/skillassets frontend/src/landing/content/docs
```

Expected: at least one assertion or source scan fails before implementation because the current user-facing import flow exists.

- [ ] **Step 3: Remove the bounded implementation and references**

Delete only the listed user-facing import packages, routes, DTOs, UI, tests, and active instructions. Do not read, move, or delete any user legacy data.

- [ ] **Step 4: Regenerate API artifacts**

```bash
npm run api
```

Expected: OpenAPI and frontend schema no longer expose import operations and remain mutually generated.

- [ ] **Step 5: Run focused verification**

```bash
cd backend
go test ./internal/cli ./internal/httpd/... ./internal/skillassets/...
cd ..
npm --prefix frontend run typecheck
npm --prefix frontend test -- --run Migration
test -z "$(rg -l 'ao import|kennel import' backend/internal/skillassets frontend/src/landing/content/docs)"
```

Expected: every command exits 0.

- [ ] **Step 6: Commit the cleanup**

```bash
git add backend frontend
git commit -m "chore: remove legacy user import flow"
```

### Task 4: Establish provider-neutral admission and historical recovery — completed by PR #13

**Files:**
- Modify: `backend/internal/domain/harness.go`
- Modify: `backend/internal/domain/harness_test.go`
- Modify: `backend/internal/storage/sqlite/store/agent_switching_store.go`
- Test: `backend/internal/storage/sqlite/store/agent_switching_store_test.go`
- Modify after provider approval: `backend/internal/adapters/agent/registry/registry.go`
- Modify after provider approval: `backend/internal/adapters/reviewer/registry.go`
- Modify after provider approval: `frontend/src/renderer/lib/agent-select-options.ts`
- Modify after provider approval: `frontend/src/renderer/lib/reviewer-harnesses.ts`

**Interfaces:**
- Consumes: the approved provider capability/admission matrix
- Produces: separate recognition of immutable historical provider identities, per-Attempt capability admission, and selectability for new work

- [ ] **Step 1: Implement the approved provider boundary faithfully**

The **v0 local dogfood** new-work set is Codex only. This is not a locked v1 provider decision. Admission is live, fail-closed, and bound to every Attempt start or resume. A provider adapter exposes its required and optional capability profile; required capabilities block admission while optional capabilities only enable dependent routing choices. Compatibility is capability-first with known-bad blocking. Keep provider-neutral domain/orchestration contracts and compile an immutable RunBrief core into an adapter-specific execution form that may narrow but never widen it. Do not treat this as permission to delete inherited provider identities or historical decoder paths: identity remains immutable and readable. A historical session may resume only when its adapter is later admitted, supports recovery, and passes fresh reconciliation/readmission; otherwise it is inspect-only and hands off through a provenance-bearing packet to a new Attempt on an admitted provider.

- [ ] **Step 2: Write the historical recovery test**

Persist a session using one retired provider identity, load it successfully, expose it as unavailable for new work, and prove it can hand off to an admitted provider without rewriting its original identity. Add a separate test that makes recovery selectable only when the adapter is conformant, declares recovery support, and passes fresh readmission.

- [ ] **Step 3: Verify the test fails under PR #11 semantics**

```bash
cd backend
go test ./internal/domain ./internal/storage/sqlite/store -run 'Historical|Retired|Selectable' -count=1
```

Expected: failure demonstrates that one `IsKnown` predicate cannot represent both historical decoding and new-work admission.

- [ ] **Step 4: Introduce explicit predicates**

Use domain vocabulary equivalent to `IsRecognizedPersisted` and `IsSelectable`. Do not delete retired constants or historical decoder paths.

- [ ] **Step 5: Apply the approved launch matrix consistently**

Update backend registries, frontend options, reviewer availability, and readiness presentation together. A provider missing one required capability must expose an honest degraded/admission result rather than implied parity. Do not hard-code Codex-specific execution semantics into the domain, RunBrief core, recovery record, or routing policy; Claude and other providers must be addable through the adapter/conformance boundary.

- [ ] **Step 6: Run focused verification**

```bash
cd backend
go test ./internal/domain ./internal/adapters/agent/registry ./internal/adapters/reviewer ./internal/storage/sqlite/store
cd ../frontend
npm run typecheck
```

Expected: every command exits 0, including historical-row recovery.

- [ ] **Step 7: Commit the bounded provider change**

```bash
git add backend frontend
git commit -m "feat: add provider-neutral admission boundary"
```

### Task 5: Run the post-foundation cleanup gate — completed through PRs #13-#14

**Files:**
- Verify: all files changed by Tasks 3 and 4
- Verify: generated artifacts have no drift

**Interfaces:**
- Consumes: issue-sized cleanup commits
- Produces: reviewable PR candidates with no product ontology migration

- [ ] **Step 1: Inspect the final diff and prohibited scope**

```bash
git diff --check ef0baffd7850702480cba84ae97e2790367eab12...HEAD
git diff --name-status ef0baffd7850702480cba84ae97e2790367eab12...HEAD
test -z "$(git diff --name-only ef0baffd7850702480cba84ae97e2790367eab12...HEAD | rg 'outcome_control_plane|outcome_store|queries/outcomes|gen/outcomes')"
```

Expected: no whitespace errors and no PR #11 Outcome schema/store artifacts.

- [ ] **Step 2: Run the complete repository gate**

```bash
npm run test:foundation
npm run audit:production
npm run lint
cd backend && go test -race ./...
```

Expected: every command exits 0. Record exact local evidence if hosted Actions remain unavailable.

- [ ] **Step 3: Prepare PR descriptions without publishing**

Each description names the accepted foundation SHA, linked issue, exact omissions from PR #11, verification evidence, historical compatibility behavior, and why no Outcome migration is included. Opening or merging a PR requires separate user authorization.

### Task 6: Accept the converged first Outcome contract and handoff

**Files:**
- Read: `docs/product/kennel-v1-product-architecture.md`
- Read: `docs/product/kennel-v0-first-outcome-slice.md`
- Read: `docs/superpowers/plans/2026-08-20-first-outcome-execution-handoff.md`

**Interfaces:**
- Consumes: accepted ResponsibilitySpace/Outcome/OpenLoop ontology plus v0 Codex dogfood admission, provider-neutral RunBrief/admission, observability, and effect-ceiling decisions
- Produces: one explicitly authorized first issue from the complete five-stage handoff

- [ ] **Step 1: Verify the resolved design gates are represented without drift**

Confirm the implementation plan preserves gates 1-5 exactly as recorded in the canonical architecture: one fenced writer per worktree with autonomy-preserving `unconfirmed` recovery, smallest-sufficient hybrid orchestration, no silent provider fallback, grounded frozen provider-neutral RunBrief core plus adapter-compiled form, multidimensional budget, truthfully classified evaluator independence, metadata-first trace, objective falsifiers, and no autonomous remote effects. Do not claim provider/model independence unless it actually exists. Hosted attachment gate 6 remains deferred and must not leak into the local slice.

- [x] **Step 2: Select one end-to-end dogfood Outcome**

Use the local-only Focus Ledger Outcome from the architecture handoff. The slice demonstrates contract definition, smallest sufficient execution topology, local authority, provider Attempt, evidence, verification, and owner acceptance without account, cloud sync, analytics, or deployment.

- [x] **Step 3: Write the separate vertical-slice specification and implementation handoff**

The Focus Ledger contract is frozen in `docs/product/kennel-v0-first-outcome-slice.md`. The exact domain, port, service, migration/query, CDC, controller/DTO, generated API, frontend, recovery, and evaluation ownership is in `docs/superpowers/plans/2026-08-20-first-outcome-execution-handoff.md`. Do not reuse PR #11's schema by default.

- [ ] **Step 4: Obtain vertical-slice approval before feature implementation**

Feature execution requires the user's explicit approval of that vertical-slice plan.

### Task 7: Sequence the first milestone on the five-stage lifecycle

**Files:**
- Read: `docs/product/kennel-v1-product-architecture.md`
- Read: `docs/product/kennel-v1-team-review-packet.md`
- Review: `docs/product/kennel-v1-review-prototype.html`

**Interfaces:**
- Consumes: accepted foundation and completed bounded cleanup
- Produces: one independently reviewable end-to-end PR for each lifecycle stage, together forming one complete Outcome proof

- [ ] **PR A — Enter**: Work-first onboarding, Project readiness, Outcome entry, and honest unavailable/offline states.
- [ ] **PR B — Understand**: minimal responsibility/Outcome contract model, immutable ContractRevision, grounding, causal trace, and one material clarification.
- [ ] **PR C — Decide & Authorize**: one-WorkUnit plan, authority/effect/budget/placement preview, approval, and provider-neutral RunBrief compilation.
- [ ] **PR D — Act & Observe**: authoritative v0 Codex admission, fenced Attempt, derived attention, containment, reconciliation, and safe retry/re-entry.
- [ ] **PR E — Prove & Close**: criterion-bound Evidence, truthfully labeled Verification, explicit AcceptanceDecision, Adaptive Close, resource disposition, and successor lineage.
- [ ] **Evaluation**: run the first-slice conformance/failure protocol and five paired direct-Codex trials; record `continue to Home`, `revise the Outcome core`, or `stop/falsified`.
- [ ] **Parallel Home milestone under ADR 0004**: Personal Home/OpenLoop persistence, Quick Capture, Daily Snapshot, and immutable many-to-many ResponsibilityLink proceed in a separately owned lane. Home and Work retain separate closure/acceptance lineages.
- [ ] **Separate gates**: one-account Gmail Communication Loops beta; per-modality activation of the required governed desktop screen/audio capabilities; and later durable Memory admission. No automatic Evidence, Open Loop, Memory, rule, or skill promotion.

Every PR owns every domain, service, storage/migration, trigger-based CDC, API/generated type, UI, recovery, and evaluation change required by its user-visible truth boundary, reusing accepted foundation APIs when no new durable truth is needed. A horizontal schema, incomplete daemon-only layer, or deceptive screen-only PR does not satisfy a stage. Every issue requires explicit approval; this plan does not authorize implementation.

### Task 8: Apply the launch falsification gate

**Interfaces:**
- Consumes: a release-candidate Work core and any enabled beta
- Produces: launch, dogfood-only, revise, or stop decision backed by recorded evidence

- [ ] Compare at least 20 representative Outcomes with direct Codex use.
- [ ] Verify median active supervision is at least 30% lower.
- [ ] Verify full transcript reconstruction is needed in no more than 20% of Outcomes.
- [ ] Verify attention precision is at least 80%.
- [ ] Verify 100% current Evidence/Verification coverage for accepted criteria.
- [ ] Verify false-ready/reopen is no more than 10% for omitted known material facts.
- [ ] Verify at least 90% of injected recoverable failures contain/reconcile safely.
- [ ] Verify zero unauthorized, duplicated, widened, or blindly retried consequential effects.
- [ ] Verify median Re-entry under 60 seconds.
- [ ] If Gmail beta is enabled, verify zero auto-created canonical commitments and zero auto-sent messages.

Failing a threshold is a product signal, not a reason to relabel the run successful. Record the failure, narrow or revise the product, and rerun the relevant gate.
