# Kennel product contract

This document is the stable product promise. The canonical runtime is [Persistent mission runtime](docs/architecture/persistent-mission-runtime.md), current implementation truth is [STATUS](docs/STATUS.md), and the only active build order is the [persistent-session execution map](docs/roadmap/persistent-session-execution-map.md).

## One promise

**Turn one software goal into a finished, verified change without babysitting the agents.**

Kennel plans, runs, supervises, checks, and recovers an AI coding mission until it is ready for the owner's decision. Native harnesses reason and edit. Kennel holds the Outcome, authority, execution state, custody, recovery, evidence, and final decision path.

## First customer

Kennel starts with founder-engineers, staff-level builders, and small technical teams who already use Codex for repository-level work and do not want to remain the scheduler, context router, recovery manager, and integrator for every run.

## Product model

The owner experiences one timeline:

1. **Ask:** clarify intent in a Contract conversation and freeze a Contract revision.
2. **Approve:** a fresh planning conversation proposes a Plan and WorkUnit DAG; the owner approves an exact revision.
3. **Watch:** the daemon and Mission Supervisor coordinate persistent WorkUnit sessions, attention, checks, rework, and recovery.
4. **Decide:** verified WorkUnits form a Result; the owner Accepts, requests rework, or revises the Plan.

The internal hierarchy is:

```text
Outcome
  Contract revision
  Plan revision and WorkUnit DAG
  Mission Supervisor thread
  WorkUnit
    Attempt = exclusive worktree lease = Kennel Session
      persistent primary Codex thread
        many turns
```

The Mission Supervisor is the mission-level intelligence layer coupled to the deterministic daemon through typed events and commands. Its default automatic set is context delivery and reversible guidance from an explicit daemon allowlist, bound to exact revisions, session generation, budget, and idempotency. It may use only approved facts inside unchanged scope, criteria, and approach, with no external effect. It cannot change scope, dependencies, permissions, external effects, replacement, or final Accept.

## Product invariants

- Outcome state is above provider session state.
- Workers share versioned artifacts and decisions, never private transcripts or hidden reasoning.
- `needs_you` waits in the same Attempt/session/thread.
- A finished turn or exited process is not completion. A worker claims readiness; Kennel runs checks.
- Replacement is explicit or follows proven irrecoverability.
- Native development work happens in an exclusive worktree under a negotiated coding profile. External effects require separate authority.
- Verification binds an immutable tree after worker and child-command write custody is closed; publication and owner review use that same tree.
- Installed harness adapters are paired and capability-checked. They may propose; only Kennel’s authenticated owner path may authorize material changes.
- Budgets guide warnings and recommendations. Hard limits are explicit, not arbitrary defaults.
- New execution is Codex-first and persistent. Historical one-shot Attempts remain readable under their original model.

## Readiness standard

The first shippable loop is one clean-machine, serial, three-WorkUnit Codex Outcome that proves installation/pairing, multi-round intake, automatic mission planning, approval, a native inspect-edit-fail-repair-test loop, persistent turns, bounded automatic supervision, attention and answer without confirmation spam, desktop/daemon/app-server restart and reconnect, exact predecessor-tree handoff, immutable verification, stale-lineage rework, Supervisor failure tolerance, verified integrated Result, owner Accept, harness drift handling, and rollback. Parallel independent worktrees and other providers follow this proof.

## Decision rule

Prefer the smallest design that strengthens completion, owner trust, native harness depth, truthful recovery, and one coherent product surface. Do not add a second scheduler, database, session engine, hidden transcript bus, or provider abstraction without evidence from the packaged loop.
