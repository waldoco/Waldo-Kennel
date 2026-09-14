# Admission and event contract

**Status: proposed W1.0 contract.** This document freezes the review seam before implementation. Names may change during W1.0 review, but Contract analysis, Plan approval, routing, Attempt start, and every client must consume the same semantics.

## AdmissionVerdict

Admission answers one question: **Can Kennel execute this exact immutable Plan revision, with these bindings and limits, as represented to the owner?**

The Go daemon owns the decision. A verdict is immutable evidence tied to the inputs it evaluated. Any relevant change makes it stale and requires recomputation.

```text
AdmissionVerdict
  verdict_id
  outcome_id
  contract_revision_id
  plan_revision_id
  project_revision_or_snapshot
  evaluated_at
  status: admitted | rejected | stale
  workunit_verdicts[]
  required_owner_actions[]
  reason_codes[]
  policy_version
  capability_snapshot_ids[]
  provider_verification_snapshot_ids[]
```

Each `workunit_verdict` contains:

```text
workunit_id
status: admitted | rejected
selected_harness
selected_profile_or_model, when policy makes it load-bearing
provider_verified
provider_available
required_capabilities[]
available_capabilities[]
requested_permissions[]
resolved_sandbox
workspace_requirements
expected_outputs[]
compiled_checks[]
budget
reason_codes[]
```

The verdict must be deterministic for the stored inputs. Human-readable explanations are projections of reason codes, not a second policy system.

## Required checks

Before `admitted`, every WorkUnit must pass:

1. exact Outcome, Contract, Plan, WorkUnit, project, and provider references exist;
2. the selected harness/profile is verified and currently available;
3. required capabilities are present;
4. requested permissions map exactly to a supported provider sandbox;
5. intent, permissions, expected outputs, and checks are semantically coherent;
6. dependencies are valid and acyclic;
7. time, token, and retry budgets are present and within policy;
8. workspace lease, platform, and project constraints are representable;
9. external effects do not exceed the Contract or owner authority;
10. no newer revision or snapshot invalidates the inputs.

Approval consumes an admitted verdict. Attempt start revalidates freshness and binding, but does not reinterpret the policy differently.

## Reason codes

Reason codes are stable machine values. More than one may apply.

### Identity and freshness

- `contract_revision_missing`
- `plan_revision_missing`
- `workunit_missing`
- `project_snapshot_missing`
- `verdict_stale`
- `revision_superseded`

### Provider and capability

- `provider_unverified`
- `provider_unavailable`
- `provider_profile_missing`
- `capability_missing`
- `sandbox_unrepresentable`
- `platform_unsupported`

### WorkUnit coherence

- `intent_permission_conflict`
- `output_permission_conflict`
- `check_permission_conflict`
- `check_uncompilable`
- `dependency_missing`
- `dependency_cycle`
- `workspace_requirement_unsupported`

### Authority and limits

- `authority_exceeded`
- `external_effect_unapproved`
- `time_budget_missing`
- `token_budget_missing`
- `retry_budget_missing`
- `budget_exceeds_policy`

### Runtime revalidation

- `binding_changed`
- `capability_snapshot_changed`
- `workspace_unavailable`
- `fence_conflict`

W1.0 must reconcile these proposed values with existing API, generated code, database constraints, and migration history before implementation. Do not add aliases for convenience. One code per meaning is the goal.

## Budgets

Every executable WorkUnit has explicit limits:

```text
Budget
  wall_time_limit
  token_limit, when the harness reports usable token accounting
  retry_limit
  optional no_progress_limit
```

Budget use belongs to the Attempt event stream and mission projection. Reaching a hard limit stops that Attempt. It never silently starts another Attempt. A retry or replacement spends another authorized retry slot and creates lineage.

When a harness cannot report a metric reliably, the verdict must say that the metric is unsupported and apply an available enclosing limit. Kennel must not display invented precision.

## Shared lifecycle vocabulary

These are product projection states, not a demand to rename every stored enum immediately.

### WorkUnit state

- `ready`: dependencies and admission pass; no live Attempt
- `running`: current Attempt is executing
- `waiting`: current Attempt is waiting on a known non-owner condition
- `blocked`: execution cannot continue without a state or dependency change
- `needs_you`: one owner decision or input is required
- `verifying`: daemon checks or criterion evaluation are running
- `done`: required execution and checks passed for this WorkUnit
- `failed`: the current path ended unsuccessfully and no automatic continuation is active
- `stopped`: execution was deliberately stopped

`retrying` and `recovering` are visible transitions attached to lineage, not permanent board columns.

### Outcome state

Outcome state is derived from the Contract, Plan, WorkUnit, Verification, Result, and owner decision records. Clients do not infer it from a single session status.

## Typed event envelope

```text
MissionEvent
  event_id
  outcome_id
  workunit_id, optional
  attempt_id, optional
  sequence
  occurred_at
  kind
  actor_type: owner | daemon | intelligence | harness | check_runner
  actor_ref
  payload_version
  payload
  provenance
```

Events are append-only facts. The current mission projection may be rebuilt from canonical records and events. Provider prose is payload, not an event kind.

## Event kinds

### Plan and admission

- `contract_revision_proposed`
- `contract_revision_confirmed`
- `plan_revision_proposed`
- `admission_evaluated`
- `plan_authorized`
- `plan_revision_rejected`

### Scheduling and execution

- `workunit_ready`
- `attempt_created`
- `harness_bound`
- `attempt_started`
- `activity_observed`
- `file_change_observed`
- `usage_observed`
- `attempt_waiting`
- `owner_input_requested`
- `attempt_interrupted`
- `attempt_stopped`
- `attempt_failed`
- `attempt_completed`

### Checks, recovery, and result

- `check_started`
- `check_completed`
- `budget_threshold_reached`
- `budget_exhausted`
- `recovery_requested`
- `attempt_replaced`
- `attempt_retry_created`
- `evidence_attached`
- `verification_started`
- `verification_completed`
- `result_proposed`
- `rework_requested`
- `outcome_accepted`
- `outcome_rejected`

W1.0 must map every existing stored event and receipt to this vocabulary, identify events that require new durable records, and reject duplicate meanings before generated API or UI work begins.

## One true next action

The daemon projection may expose zero or one `next_action`:

```text
kind
reason_code
label
valid_commands[]
required_input_schema, optional
target revision/workunit/attempt
```

If no action is valid, the UI must not manufacture a button. If several commands are valid, one may be recommended, but all remain tied to the same blocker and authority.

## W1.0 implemented review seam

W1.0 implements a narrow domain seam for review without replacing the future contract above:

- `AdmissionVerdict` permits Contract- or Plan-missing rejection before those identities exist. Admitted verdicts require complete Outcome/Contract/Plan/WorkUnit attribution and only admitted WorkUnits. Rejected and stale verdicts forbid admitted executable work.
- Each admitted WorkUnit carries one `ApprovedExecutableSpec` at approval. Its digest covers compiler-policy version, local harness/model binding, native enforcement mapping, workspace requirements and lease subject, typed/versioned admission receipt identities, versioned budget/accounting semantics, checks, capability grants, and all enclosing attribution. It contains no concrete root or runtime allocation. At Attempt start, `WorkspaceBoundLaunchPacket` references that spec and binds the canonical leased root plus exact Attempt, fence, session, input, current-readiness, and launch identities without widening the approved spec.
- `BuildAttemptExecutionPolicy` rejects duplicate required capabilities after normalization and duplicate grant names instead of silently deduplicating or overwriting them. Daemon-run approved checks do not grant the provider `worktree.exec`.
- Retry limit counts successor Attempts after the initial Attempt in one WorkUnit lineage (`workunit_attempt_successors`). Owner actions are closed machine commands.
- Reasoning-only providers remain separate from `AgentHarness`; tests assert every shipped harness is selectable local execution and that an owner-key OpenAI identity cannot bind execution.
- Current stored lifecycle facts map separately in `lifecycle_projection.go`. The exhaustive mapping consumes `EmittedAttemptObservationKinds`, the enumerable list beside the emitted observation constants. It preserves `blocked` and `waiting_input` as owner attention, keeps ambiguous/live-custody observations unconfirmed rather than lost, maps provider exit to reconciled rather than completed, and maps proof classification to verified.

This slice does not persist verdicts, wire approval or Attempt start, add generated API, change current Work UI labels, implement the full future event envelope, or implement `next_action`. The future requirements in this document remain the target for W1.1-W1.4.

### Approval and launch artifacts

W1.0 chooses approval without workspace reservation. Approval consumes an immutable `ApprovedExecutableSpec`. Its digest binds Outcome/Contract/Plan/WorkUnit attribution, RunBrief core digest, local harness/model, compiler and native-mapping versions, normalized required capabilities and grants, daemon-run checks, workspace kind and lease subject, versioned budget/accounting semantics, and stable admission receipt identities. The spec contains no concrete root, Attempt, fence, or session identity.

Attempt start derives an immutable `WorkspaceBoundLaunchPacket`. Its digest binds the approved spec and spec digest to the exact Attempt, fence, session, canonical workspace root, input artifact versions, current typed readiness receipts, launch facts, and workspace-bound `AttemptExecutionPolicy`. Validation cross-checks attribution and permits only binding or narrowing; the launch packet cannot widen capabilities, grants, checks, or budgets. No W1.0 persistence, service, or port wiring is implied.

## W1.1 shared evaluator and launch crash boundary

W1.1 implements one Go-owned admission evaluator. The same evaluation semantics are used to decide Plan intelligence/routing eligibility, whether approval is enabled, and whether an approved Plan remains fresh at Attempt start. Approval atomically persists the admitted `AdmissionVerdict` and one immutable `ApprovedExecutableSpec` per WorkUnit. Runtime readiness may only reject or stale that authority, never widen it.

Attempt admission first creates the Attempt and fence. Session preparation then creates the session identity, canonical workspace, approved inputs, and workspace-bound policy without starting a provider. A mandatory callback persists the separately digested `WorkspaceBoundLaunchPacket`, referencing the approved spec digest, before either the TUI runtime or Chat controller can start. Persistence failure rolls back prepared session/workspace state and launches no provider. There is no pre-approval lease.

W1.1 adds migration `0137_admission_packets.sql`. It adds no generated API or UI surface. Budget enforcement, richer semantic coherence, retry policy, and event projection remain W1.2-W1.4 work; the W1.1 evaluator only fails closed on the invariants the current canonical records can prove.

### W1.1 budget policy seam

Each proposed WorkUnit freezes a resolved `ExecutionBudget`: wall-time per Attempt, retry limit per WorkUnit successor lineage, and either an enforced token limit or an explicit unsupported-accounting state. It also freezes source (`policy_default` or `user_override`) and policy id/version/digest. The immutable daemon policy owns recommended defaults and ceilings; evaluator admission rejects missing budgets, policy identity mismatch, or values above ceilings. Token use is cumulative across every Attempt in the WorkUnit lineage. W1.1 records the shape and provenance; W1.2 enforces limits and emits threshold events.

The production default/ceiling table remains intentionally unresolved. Until a named policy with reviewed numeric values is wired, current proposed Plans receive no resolved budget and admission rejects them with the budget reason codes. Tests use a test-only policy and do not establish product values.

The launch packet's `session_id` intentionally has no foreign key. Session seed rows are rollback/GC state, while launch evidence must remain append-only across session cleanup. Every read cross-checks SQL identity columns against the JSON packet and validates the embedded spec. Recovery reads this packet alongside session refs: no packet means preparation did not cross the launch boundary; a valid packet without a bound/running session means launch is unconfirmed; a matching session ref plus runtime facts determines running or later states.

The daemon loads the production policy only from the durable data-directory file `admission-policy.json`. The file must decode to a valid named/versioned `AdmissionPolicy`; absence leaves the policy nil and all budget-bearing proposal admission fails closed. No numeric fallback is compiled into the daemon.

For heterogeneous Plans, admission groups WorkUnits by distinct frozen provider/model preference, asks the routing inventory for each preference, and requires every returned snapshot to carry the same coherent snapshot identity. A mid-evaluation inventory-version change rejects instead of mixing views.

`admission-policy.json` is read once during daemon startup; edits take effect only after restart. The daemon account must own the file and operators should restrict it to owner read/write (0600). W1.1 does not mutate or chmod operator-managed configuration; it rejects unsafe file type or mode at startup. OS-specific numeric owner-ID enforcement remains a configuration-hardening follow-up.

Routing snapshots expose both a coherent base `generation_id` and a preference-specific `snapshot_id`. Heterogeneous provider/model checks may have different filtered snapshot IDs, but every view in one admission evaluation must share the exact generation ID; model support, defaults, candidate identity and capability facts remain inside each persisted receipt/spec input.

The policy loader rejects symlinks, non-regular files, unknown JSON fields, trailing JSON values, and any group/other permission bits. Operators must create the restart-only file as daemon-owned mode 0600.

AdmissionPolicy.digest is the SHA-256 of the complete canonical typed policy payload with both policy digest fields blanked. Validation recomputes it and requires the resolved default budget to bind that exact digest.

Approved specs include a typed routing receipt containing the nonempty generation ID, filtered snapshot ID, exact preference and normalized candidate identity/readiness/capability/default/model-support facts. Its digest is recomputable and candidate ordering is canonical. Snapshot ID never substitutes for generation identity.

Workspace-bound packets carry typed canonical LaunchFacts rather than an opaque hash claim. Validation reconstructs the facts from packet attribution, receipts, document identity, workspace, policy and ordered input versions, then recomputes both the facts digest and outer packet digest.

`LaunchPrepared` is machine proof only for an exact W1.1-admitted Plan/WorkUnit with durable verdict/spec evidence and no bound session or launch packet. A no-packet Attempt with any bound session is classified legacy and retains beta liveness/stop-proof custody semantics; packet absence alone can never release a possibly live legacy writer.

Legacy/unknown launch state never treats owner confirmation as provider-stop proof. Reconcile and replace require machine-terminated bound-session evidence, including for succeeded Attempts with complete retained results. An unavailable admission store classifies as legacy/unknown, never prepared.
