# Work E Prove and Close Implementation Plan

> **Classification for vNext (2026-09-15):** historical implementation plan, not current order. Use [persistent mission runtime](../../architecture/persistent-mission-runtime.md) and the [persistent-session execution map](../../roadmap/persistent-session-execution-map.md).


> Issue: #35. Branch: `codex/issue-35-prove-close`. Dated base: `origin/beta` at `869c42639aef7a850b5d3ff758a629a7d777137e`.

## Boundary

Deliver the first complete Work proof boundary without absorbing Mission Control, shared Intake, multi-WorkUnit planning, provider-role expansion, or evaluation work. SQLite and the Outcome service remain canonical; the controller and renderer submit typed intent and render daemon-derived truth.

The implementation preserves:

```text
Outcome -> ContractRevision -> PlanRevision -> WorkUnit -> Attempt
  -> AgentSessionRef -> EvidenceItem -> VerificationRun -> AcceptanceDecision
```

Provider completion, session exit, observations, commits, checks, screenshots, and verifier output never create Acceptance. Only the explicit user decision route can append an `AcceptanceDecision`.

## Contract decisions

1. Add stable `ContractCriterion` identity. The canonical binding is `(ContractRevisionID, CriterionID)`. Migration `0103` backfills existing JSON criteria into immutable criterion rows in original order; new revisions write both the compatibility JSON projection and canonical criterion rows atomically.
2. Add append-only `EvidenceItem`, `VerificationRun`, `AcceptanceDecision`, and `OutcomeCorrection` records. Evidence and Verification bind the exact contract criterion plus an explicit subject type, identity, and revision.
3. Verification records its actual independence class: `deterministic`, `producer_self_check`, `separate_session`, `cross_provider`, or `owner_walkthrough`. The service rejects impossible independence claims such as a same-session separate-session review or same-provider cross-provider review.
4. Derive `active`, `ready_for_acceptance`, `accepted`, and `rework_required` at read time. No display status is stored. Proof for superseded contracts stays historical and cannot make the current contract ready.
5. A `request_rework` or `reopen` decision appends correction lineage with a required target (`attempt`, `work_unit`, `plan`, or `contract`). Proof created before that correction remains history and cannot immediately restore readiness.
6. Acceptance requires current criterion-bound supporting Evidence plus a current passed Verification or explicit Verification exception for every criterion, with no unresolved contradiction. The immutable decision records resource disposition as `retain`, `cleanup_later`, or `not_applicable`; no worktree deletion is performed by this slice.
7. Every mutation is idempotent by request key and detects a conflicting replay by request fingerprint. Optimistic contract identity guards reject writes against a moved current revision.

## TDD delivery sequence

- [ ] Domain red/green: stable criterion identity; proof record validation; actual independence; decision and correction invariants; derived proof state after acceptance, rework, reopen, contradiction, and contract revision changes.
- [ ] Migration red/green: `0103` backfill, append-only guards, foreign-key criterion binding, CDC vocabulary/triggers, and rollback shape.
- [ ] Store red/green: atomic criterion writes, restart read-back, idempotent replay/conflict, ordered proof history, and trigger-backed CDC.
- [ ] Service red/green: stale contract refusal, evidence-to-criterion/subject validation, verification independence, readiness, explicit user acceptance, rejection/reopen/correction lineage, and truthful next action.
- [ ] Controller red/green: GET proof plus evidence, verification, and acceptance-decision mutations; validation/conflict/not-found envelopes retain request IDs.
- [ ] Renderer red/green: daemon-backed criterion review, Evidence and Verification forms, Accept, Request rework, Reopen, resource disposition, and exact re-entry. Add `prove_close` route handling and a bounded Act-to-proof action.
- [ ] Regenerate only from sources with `npm run sqlc` and `npm run api`; prove zero generated drift.
- [ ] Run focused backend/frontend tests, restart/CDC coverage, typecheck/build, repo lint/foundation checks, and the real desktop preview in proportion to touched files.

## Expected files

- Domain/service/ports: `backend/internal/domain/outcome.go`, new `outcome_proof.go`, `backend/internal/ports/outcome_proof_store.go`, and new Outcome proof service/tests.
- Storage: migration `0103`, Outcome/proof queries, store implementation/tests, migration tests, and CDC restoration.
- HTTP: Outcome proof routes/controller tests, DTOs, API dependency wiring, operation registry, generated OpenAPI and TypeScript schema.
- Renderer: Outcome hooks, `OutcomeProveCloseSurface`, route/run integration, focused component tests, and required localized copy.

## Verification evidence to retain

- Observed red and green command output for each layer.
- Installed dependency: `modernc.org/sqlite v1.51.0`, bundling SQLite 3.53.1 on this platform. Official SQLite documents `json_each()` as a table-valued function producing one row per top-level array member and row-level triggers as automatic database operations; those facts justify the additive backfill and trigger-backed CDC shape.
- Final `git diff`, generated-artifact zero drift, fresh `origin/beta` comparison, and exact command exit results before the local commit.
