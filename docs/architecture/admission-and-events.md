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
