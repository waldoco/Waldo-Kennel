# Kennel product contract

This document defines what Kennel is building and how product decisions are judged. It is intentionally stable. Current implementation status belongs in [docs/STATUS.md](docs/STATUS.md), and the active build sequence belongs in [ROADMAP.md](ROADMAP.md).

## One promise

**Turn one software goal into a finished, verified change without babysitting the agents.**

Kennel plans, runs, checks, and recovers an AI coding job until it is ready for the owner's decision.

Kennel is not another coding model or a prettier terminal. It is the operating layer around native coding harnesses. The harness reasons and edits. Kennel holds the Outcome, authority, execution state, recovery, proof, and final decision.

## First customer

Kennel starts with founder-engineers, staff-level builders, and small technical teams who:

- already use Codex or another coding harness most days;
- run repository-level work rather than toy prompts;
- understand diffs, checks, and pull requests;
- have built personal prompt, skill, plugin, tmux, or worktree workflows;
- do not want to remain the scheduler, context router, recovery manager, and integrator for every run.

The first customer is not an occasional autocomplete user, a nontechnical app builder, or an enterprise that requires every provider and governance integration immediately.

## Problem and value

Native harnesses can code, but a serious user still has to:

- translate a goal into an executable plan;
- decide which session should do each part;
- carry context and constraints between workers;
- notice stalls, impossible work, and wasted tokens;
- recover interrupted or confused sessions;
- reconcile changes and rerun checks;
- reconstruct whether the result meets the original goal.

That work is frequent and costly for harness power users. Kennel makes the expert workflow repeatable: one durable Outcome, a feasible Plan, bounded execution, legible progress, truthful recovery, and evidence against the agreed criteria.

Kennel is valuable only when that loop is materially better than the user's manual workflow. A polished wrapper around one opaque session does not meet the promise.

## The four moments

The product exposes four moments. The internal lifecycle remains detailed so correctness is not lost.

### Ask

The owner describes an Outcome. Kennel gathers repository facts and asks only questions that change success, scope, authority, feasibility, or proof.

### Approve

One review surface combines the Contract, Plan, permissions, provider admission, budgets, stop conditions, and expected proof. The owner authorizes an exact immutable Plan revision. Inadmissible work cannot be approved.

### Watch

Mission Control shows the dependency graph, active WorkUnits, current Attempts, harness bindings, progress, budgets, changes, checks, blockers, recovery, and one true next action. Silence, waiting, interruption, and retry are explicit states.

### Decide

The Result maps evidence to every Contract criterion. The owner can Accept, reject, or request bounded rework. Kennel never turns a passing provider message into owner acceptance.

## Readiness standard

The core product is ready when a serious harness power user can run a real repository Outcome through Ask, Approve, Watch, and Decide with less operational work and more confidence than their manual process.

The proof must include:

- a feasible, useful Plan with no manual prompt engineering after Ask;
- exact authority and no unapproved effects;
- no impossible Plan reaching approval;
- clear progress with no unexplained silence;
- bounded time, token, and retry use;
- truthful, restart-safe recovery and Attempt lineage;
- daemon-run checks and evidence mapped to the Contract;
- bounded rework and explicit owner Accept;
- a packaged desktop run with no hidden rescue steps.

September 18 is a forcing function for integration, not the product definition. Date pressure cannot remove the complete core loop, strong Watch and Decide experiences, accessibility, trust-boundary validation, or proof.

## Two horizons

### Core product

Build and prove the complete single-owner Outcome loop, Codex-first, with one authoritative Go control plane and truthful desktop/plugin projections.

### Wider Waldo/Kennel direction

After the core loop is dependable, extend the same invariants to:

- more native harnesses and capability-aware mixed-provider routing;
- safe parallel WorkUnit execution;
- portable, owner-correctable context and cross-harness memory;
- teams, shared policy, review, and audit;
- richer project understanding, learning, channels, and product surfaces.

These are extensions of the core, not alternate product definitions and not excuses to weaken it.

## Decision rule

A change belongs in the core when it improves Outcome quality, autonomous completion, visibility, recovery, proof, or the owner's control of authority. Otherwise it is an extension, evidence, or noise.
