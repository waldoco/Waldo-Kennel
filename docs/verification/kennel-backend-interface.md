# Kennel backend → Mission Control interface note

Date: 2026-09-10. Author: the backend completion task
(`codex/kennel-execution-completion`, base `670a4223`).

This note is for the parallel Mission Control renderer task. It states what the
daemon already serves, what is missing, and the exact shape and arrival order of
the operations being added. **Do not hand-write TypeScript substitutes for any
of these.** Every type below reaches the renderer through
`frontend/src/api/schema.ts`, regenerated from the Go API source by
`npm run api`, and committed by this task.

Until an operation's implementation slice lands, its route exists and answers
honestly with a typed `501` carrying a stable code. A `501` is a real product
state — render "unavailable" with the reason — never a reason to fabricate
local state or to fall back to a legacy session launch.

## 1. Already usable today

| Need | Operation | Notes |
|---|---|---|
| Outcomes of one Project | `GET /api/v1/projects/{id}/outcomes` | `OutcomesEnvelope`; for the derived Board state of the same Outcomes see §2.1 |
| One Outcome + full contract history | `GET /api/v1/outcomes/{outcomeId}` | `Outcome`, `Current`, `History`, `LatestPlan` |
| Current Plan | `GET /api/v1/outcomes/{outcomeId}/plan` | |
| Schedule, dependency reasons, custody | `GET /api/v1/outcomes/{outcomeId}/plans/{planId}/schedule` | Per-unit `state` (`blocked`/`runnable`/`executing`/`proven`/`retryable`/`paused`), `blockedReason` (`awaiting_dependency_proof`/`custody_held`/`upstream_artifact_unavailable`) plus `blockedDetail` naming the specific upstream refusal, `blockingDependencies`, `nextRunnableId`, `custodyHeldBy`, `noRunnableReason` (`all_units_proven`/`attempt_executing`/`attempt_paused`/`awaiting_proof`). **This is the canonical graph overlay — do not recompute eligibility in the renderer.** |
| Proof / evidence / verification / acceptance | `GET /api/v1/outcomes/{outcomeId}/proof` | criterion-bound; carries the proof horizon |
| Attempts | `GET /api/v1/outcomes/{outcomeId}/attempts`, `.../attempts/{attemptId}` | |
| Start one Attempt | `POST /api/v1/outcomes/{outcomeId}/attempts` | takes an idempotency `requestKey`; this stays the low-level operation. Owner-facing Start is §2.2 |
| Cancel / recover one Attempt | `.../attempts/{attemptId}/cancel`, `.../recovery` | |
| Contract revision, plan proposal/replan/approval | `POST .../revisions`, `.../plans`, `.../plans/replan`, `.../plans/{planId}/approval` | approval binds the exact revision; approval and start remain distinct operations |
| Composition / decomposition | `.../composition`, `.../decomposition*` | |
| Acceptance | `POST .../acceptance-decisions` | owner-only; nothing else creates one |

### Refusals the UI must render truthfully today

`UPSTREAM_MATERIALIZATION_UNAVAILABLE` no longer exists: successor provisioning
is implemented, so a dependent WorkUnit now really starts with its
predecessors' retained bytes. Its replacement is
`UPSTREAM_MATERIALIZATION_FAILED` (409), which means validated predecessor
results could not be placed in the successor's workspace — and asserts that
**no provider was launched**. The Attempt is ended and the reason is recorded;
the owner may retry deliberately after repairing the upstream result.

A unit in this situation is also visible before Start: its schedule entry is
`blocked` with `blockedReason: upstream_artifact_unavailable` and a
`blockedDetail` naming which refusal applies. Also expect `UPSTREAM_ARTIFACT_MISSING`,
`UPSTREAM_ARTIFACT_INCOMPLETE`, `UPSTREAM_ARTIFACT_UNREVIEWED`,
`UPSTREAM_LINEAGE_MISMATCH`, `PLAN_NOT_APPROVED`, `PLAN_BRIEF_INVALIDATED`,
`ATTEMPT_FENCE_HELD`, `AGENT_BINARY_NOT_FOUND`, `AGENT_PROFILE_NOT_READY`,
`ATTEMPT_EXECUTION_POLICY_UNSUPPORTED`, `ATTEMPT_START_UNRESOLVED`,
`ATTEMPT_ACTIVATION_UNRESOLVED`.

`WORK_UNIT_NOT_RUNNABLE` (409) refuses an explicit Start naming any unit that
is not the scheduler's next dependency-ready unit. It is distinct from
`NO_RUNNABLE_WORK_UNIT`, which reports that scheduling itself has nothing to
offer (every unit is already running or still waiting on a dependency or
required proof), and from `WORK_UNIT_INPUT_ARTIFACT_MISSING`, which is the
post-selection upstream-artifact gate.

## 2. Missing operations being added by this task

All paths are under `/api/v1`. All responses are the repository's standard
envelope; errors are `envelope.APIError` with a stable `code`.

### 2.1 Board projection — `GET /projects/{id}/outcome-run-states`

Board and List are views over the same Outcomes, so the derived Mission state
each card shows is served by the daemon rather than recomputed per card in the
renderer. One call returns every row for one Project.

Query: `scope` = `top_level` (default) | `all`. `top_level` omits contributing
Outcomes, which belong inside their parent's Mission.

`OutcomeRunStatesEnvelope { runStates: OutcomeRunState[], observedAt }`, where
each entry is exactly the §2.2 `OutcomeRunState` shape. There is deliberately
no second board-only DTO: a card and its Mission header must never be able to
disagree.

Fields most useful to a card: `title`, `projectId`, `state`, `attentionReason`,
`blocker`, `planStatus`, `planBindsCurrentContract`, `activeAttemptStatus`,
`provenCriteria`/`requiredCriteria` (counts, **not** a percentage) and
`acceptedAt`.

Errors: `400`, `500`, and `501` if the daemon has no Outcome service wired.
**This operation ships implemented in the API commit.**

### 2.2 Run state and eligible actions — `GET /outcomes/{outcomeId}/run`

`OutcomeRunStateEnvelope { runState: OutcomeRunState }`, where `OutcomeRunState`
carries `outcomeId`, `projectId`, `title`, `parentOutcomeId`, `state`,
`attentionReason`, `intent`, `eligibleActions`, `blocker`, `freshness`,
`planStatus`, `planBindsCurrentContract`, `activeAttemptId`,
`activeAttemptStatus`, `provenCriteria`, `requiredCriteria` and `acceptedAt`.

- `state`: `define` | `ready_to_authorize` | `in_progress` | `needs_you` |
  `ready_for_review` | `accepted`, derived on every read and never stored.
  `attentionReason` is a stable code, non-empty exactly when `state` is
  `needs_you`, so "Needs you" always carries a concrete reason.
- `intent`: absent until the owner has authorized anything, then
  `{ generation, desired: "idle"|"running"|"paused"|"cancelled", planRevisionId,
  requestedAt, acknowledgedAt|null, activeAttemptId|null, lastError|null }`.
  `generation` is the optimistic-concurrency token for §2.3.
  `acknowledgedAt` is what distinguishes an **acknowledged** cancellation from a
  **requested** one; render "cancelling…" until it is set.
- `eligibleActions`: `[{ action, available, reason }]` over the fixed vocabulary
  `clarify`, `propose_plan`, `review_plan`, `approve_plan`, `start`, `pause`,
  `resume`, `cancel`, `review_result`, `request_changes`, `accept`, `export`.
  `reason` is a stable code, present whenever `available` is `false`.
- `blocker`: `{ code, message, detail }` or `null` — the single concrete reason
  a "Needs you" Outcome needs the owner.
- `freshness`: `{ observedAt, contractRevisionNumber, planRevisionId,
  proofGeneration }`. `proofGeneration` is the append-only proof record count
  the projection was computed from; a mutation that returns a lower value than
  one you already hold is stale and must be discarded.

Errors: `404 OUTCOME_NOT_FOUND`, `500`, and `501` only when the daemon has no
Outcome service wired at all. An absent `intent` with refused pause/resume is a
truthful answer, not an error.

### 2.3 Run intent commands — `POST /outcomes/{outcomeId}/run`

Body: `{ action: "start"|"pause"|"resume"|"cancel", planRevisionId,
expectedContractRevisionNumber, expectedGeneration?, requestKey }`.

`requestKey` is required and is the replay identity: repeating the same key
returns the same state rather than acting twice. Double-click and reconnect
retry are therefore safe.

Response `200 OutcomeRunStateEnvelope`.

Errors: `400 RUN_ACTION_INVALID`; `404 OUTCOME_NOT_FOUND`; `409` with
`RUN_INTENT_STALE` (a newer generation exists), `RUN_ACTION_UNAVAILABLE` (the
action is not in the eligible set — the response detail names why),
`PLAN_NOT_APPROVED`, `PLAN_BRIEF_INVALIDATED`, `ATTEMPT_FENCE_HELD`,
`RUN_CUSTODY_UNKNOWN` (surviving execution of unknown status blocks new work);
`501 RUN_INTENT_UNAVAILABLE` only from a daemon with no run-intent storage.

Two behaviours worth relying on:

- **Replay is resolved first.** A repeated `requestKey` returns the generation
  it already authorized without re-validating against the state its own first
  copy created — so a double-clicked Start is a no-op, not "already running".
- **A pause applies to every admission path**, including the low-level
  `POST .../attempts`. That call answers `409 RUN_ACTION_UNAVAILABLE` while the
  run is paused or cancelled, so a pause cannot be clicked past.

Semantics the renderer can rely on:

- **Start** authorizes serial continuation across the approved Plan. It does not
  itself launch; it records intent, and the daemon admits each eligible WorkUnit
  exactly once.
- **Pause** prevents *subsequent* admission. It does **not** stop work already
  running: the active Attempt continues, and `acknowledgedAt` is set once no
  admission can follow. Show "pausing after current work" until then.
- **Cancel** prevents subsequent admission *and* requests termination of the
  active Attempt. `acknowledgedAt` is set only once the provider stop is proven;
  an unproven stop leaves the request visible and the Outcome in `needs_you`.
- **Resume** creates a new generation from `paused`; it is refused from
  `cancelled`, where a fresh Start is required.

### 2.4 Delivery — `POST/GET /outcomes/{outcomeId}/deliveries`, `GET .../deliveries/{deliveryId}`

Create body: `{ attemptId, artifactVersion, destination, disposition:
"accepted"|"draft", acceptanceDecisionId?, requestKey }`.

`OutcomeDeliveryEnvelope { delivery }` where `delivery` is
`{ id, outcomeId, attemptId, workUnitId, artifactVersion, disposition,
destination, state: "pending"|"succeeded"|"failed"|"cancelled", manifestPath,
fileCount, byteCount, failureCode, failureDetail, requestedAt, completedAt }`.

Errors: `400 DELIVERY_REQUEST_INVALID`; `404`; `409` with
`DELIVERY_ARTIFACT_MISMATCH` (the named version is not the reviewed/accepted
one), `DELIVERY_ARTIFACT_MISSING`, `DELIVERY_NOT_ACCEPTED` (accepted disposition
without a current acceptance for that exact artifact),
`DELIVERY_DESTINATION_CONFLICT` (existing files — the owner must choose),
`DELIVERY_DESTINATION_UNSAFE` (traversal or symlink escape),
`DELIVERY_MANIFEST_COLLISION`; `501 DELIVERY_UNAVAILABLE` when this capability
is not wired in a degraded daemon.

The production daemon wires this boundary to the SQLite delivery ledger and
artifact store. A pending row is never reported as delivered; a restart marks
it `failed` with `DELIVERY_INTERRUPTED`, and a failed atomic staging attempt
does not claim a partial destination. Degraded daemons may still return the
501 capability response.

Delivery is **transfer of one exact artifact**, never merge, PR, deploy or
publication, and never implies acceptance. A draft export is labelled `draft`
and the renderer must show it as such.

### 2.5 Attributed usage — `GET /outcomes/{outcomeId}/usage`

`OutcomeUsageEnvelope { outcomeId, planning, totals, workUnits[], attempts[], observedAt }`
where each usage block is `AttributedUsage`:
`{ totals: UsageTotals, requests?, costUsd?, costEstimated, pricingProvenance }`.
`UsageTotals` is the existing normalized telemetry shape already used by
`/usage/sessions` — `inputTokens`, `uncachedInputTokens`, `cacheReadTokens`,
`cacheWriteTokens`, `outputTokens`, `reasoningTokens` — reused rather than
duplicated. Planning usage is reported separately from execution so reasoning
spend is never presented as work a provider did.

Every numeric is **nullable and null means unknown** — do not coerce to zero.
`costEstimated` is `true` unless `pricingProvenance` names a real price source;
without provenance no exact cost is claimed and the renderer must not print one.

Errors: `404`; `501 USAGE_ATTRIBUTION_UNAVAILABLE` until the slice lands.

## 2.6 Contract delta already shipped: approved checks on Plan WorkUnits

`PlanWorkUnit` in the generated schema now carries `approvedChecks`, and
**authorization must show them**. They are the commands Kennel itself will run
and record as independent observation; approving a Plan approves them, so a
Plan review that does not display them is not an informed authorization.

```
approvedChecks: {
  id: string            // minted by the daemon, never by the proposal
  criterionId: string   // a criterion this WorkUnit owns
  argv: string[]        // discrete arguments, never a command line
  timeoutSeconds: number
}[]
```

Rendering notes:

- Show `argv` as a command, but do not join and re-split it: the vector is
  exact, and an argument may legitimately contain spaces.
- `criterionId` binds the check to the criterion it proves. Show the criterion
  text, not the raw id.
- The array is frequently **empty**, and that is a real state, not a loading
  one: not every WorkUnit can be proved by a command, and no live provider has
  yet proposed one. An empty list means those criteria will not be proved by a
  deterministic check.
- `timeoutSeconds` is a bound the daemon chose or clamped, not the model's
  request.

Results appear through the existing proof projection: one `EvidenceItem` with
`sourceType: deterministic_check` and `producerType: tool`, plus a
`VerificationRun` with `independenceClass: deterministic`, both bound to the
criterion, the producing Attempt and the exact artifact version. Three
verification results matter and must read differently:

| Result | Meaning |
|---|---|
| `passed` | the command ran under an enforcement mechanism and exited zero |
| `failed` | it ran and exited non-zero — a problem with the work |
| `inconclusive` | it never ran, could not be confirmed stopped, or changed the result it was checking — usually a problem with the host or the check, **not** the work |

Do not collapse `inconclusive` into `failed`. A host with no sandbox and a red
test send the owner to fix different things.

## 3. Freshness and reconnect

There is one canonical change feed: trigger-backed CDC in `change_log`, streamed
over the existing `/api/v1/events` SSE endpoint. Events carry a monotonic `seq`;
dedupe by it. No parallel status authority is being added.

Existing event types relevant to Mission Control: `outcome_created`,
`outcome_updated`, `outcome_contract_revised`, `outcome_plan_proposed`,
`outcome_plan_approved`, `outcome_attempt_started`, `outcome_attempt_updated`,
`outcome_attempt_session_bound`, `outcome_attempt_observed`,
`outcome_attempt_recovered`, `outcome_evidence_recorded`,
`outcome_verification_recorded`, `outcome_acceptance_decided`,
`outcome_correction_recorded`.

New event types arriving with their slices: `outcome_run_intent_changed`,
`outcome_attempt_retained` (a receipt was written or frozen), and
`outcome_delivery_changed`.

Invalidation map:

| Event | Refetch |
|---|---|
| `outcome_created`, `outcome_updated`, `outcome_contract_revised` | board, outcome, run state |
| `outcome_plan_proposed`, `outcome_plan_approved` | outcome, plan, schedule, run state |
| `outcome_attempt_*` | attempts, schedule, run state, board row |
| `outcome_attempt_retained` | attempt, schedule, proof, delivery eligibility |
| `outcome_evidence_recorded`, `outcome_verification_recorded`, `outcome_correction_recorded` | proof, schedule, run state |
| `outcome_acceptance_decided` | proof, run state, board row, delivery eligibility |
| `outcome_run_intent_changed` | run state, schedule, board row |
| `outcome_delivery_changed` | deliveries, run state |

On reconnect: refetch every open query rather than replaying missed events, and
show the reconnecting state until the first successful refetch lands. Silence is
not completion — never infer a terminal state from an absent event.

Ordering rule for optimistic writes: compare `freshness.proofGeneration` and
`intent.generation` on every mutation response; a response older than state you
already hold is a stale response and must be discarded, not rendered.

## 4. Arrival order

| Operation | Lands with |
|---|---|
| §2.1 board projection | the API commit (implemented immediately) |
| §2.2 run state | the API commit (implemented; `intent` is **absent**, actions derived from existing facts) |
| §2.3 run commands | **implemented** — a daemon without run-intent storage still answers `501 RUN_INTENT_UNAVAILABLE` |
| §2.4 delivery | delivery slice — `501 DELIVERY_UNAVAILABLE` until then |
| §2.5 usage | usage slice — `501 USAGE_ATTRIBUTION_UNAVAILABLE` until then |

The API commit's SHA is reported to the owner separately. Take it as an explicit
dependency; do not cherry-pick unreviewed backend work or read this task's
working tree.
