# Kennel core-loop implementation handoff

Date: 2026-09-15. Status: DRAFT FOR OWNER REVIEW — IMPLEMENTATION NOT AUTHORIZED. No product implementation or new runtime verification claimed.

Owner instruction: do not start workers. Read and discuss this plan first. All dispatch prompts and sequences below are future templates only. A separate explicit owner go-ahead is required before implementation, runtime probes, installation, cleanup or worker dispatch. The completed documentation investigation was read-only.

## Product decision

Kennel turns a software Outcome into a planned, supervised run and a reviewable result. The user states the result, answers material questions, approves the approach, intervenes when needed, and accepts the result. Kennel manages decomposition, provider sessions, dependencies, context delivery, progress, recovery, and evidence.

The nine internal stages remain useful. Present four user stages: Define, Approve, Run, Review. A graph and session controls support the Outcome; they do not require the user to become an orchestration engineer.

The first target user is a developer or technical founder who already uses a coding harness but spends substantial effort coordinating multi-step work. Initial use cases: a bounded feature with tests, a reproducible bug fix, and a repository documentation change. Do not claim market demand or willingness to pay is proven. After runtime proof, compare three real tasks with the user's ordinary harness workflow: supervised minutes, repeated instructions, interventions, correctness, and whether they choose Kennel again.

## Read without loading the entire repository history

1. Read repository AGENTS.md and its canonical read order, stopping as instructed when sufficient context is obtained. This handoff does not override that authority.
2. Read ../2026-09-15-core-loop-reset.md for source-confirmed causes and the ten owner decisions.
3. Read only your assigned section in PACKETS.md; frontend workers also read UI-AND-CONTEXT.md.
4. Inspect current source and tests at the coordinator's pinned baseline. The findings in the reset refer to 5304d569aa2e29f0cc77d57539aa2f65a4d01484, not necessarily today's remote beta.
5. Consult historical audit material only to reproduce the assigned failure. Never load every dated plan by default.

This directory is the implementation dispatch index. The reset remains the decision/evidence background. Canonical architecture and docs/STATUS.md retain their existing jobs. Do not create another PRD, competing state machine, or global memory system.

## Freeze these decisions before coding

P0 answers the following through installed-provider probes and source inspection. Defaults below are proposals, not verified capabilities.

| Decision | Proposed default | Required proof / output |
| --- | --- | --- |
| Interactive control | Existing Codex App Server, with separately labelled native PTY mode | Start, send, steer, interrupt, reuse idle thread, reconnect; no dual active controller |
| Coding permission profile | Normal local edits and development commands in an isolated worktree | Installed sandbox actually supports it; disclose breadth; do not silently widen old Plans |
| Completion boundary | Structured ready-for-verification handoff; daemon quiesces mutation then verifies | Ordinary turn completion leaves session reusable; verification cannot race writes |
| Initial planning | Automatic first provider turn after Start planning | One durable kickoff across repeated clicks/restart; no synthetic user-authored message |
| Clarification | Material question batches plus follow-up rounds | Stable question IDs, partial answers and migration compatibility |
| Parallelism | One proven interactive writer first; independently leased branches next | Two independent units overlap safely and integration waits for both |
| Native subagents | Remain inside provider unless independent ownership/retry/proof needed | No fake graph nodes for unobservable provider internals |
| Plugin | Thin mission client over the same daemon operations | Host-supported invocation, shared IDs/revisions/events; no second scheduler |

If a proposed decision conflicts with a canonical rule, record the exact conflict and obtain owner ratification for the scoped architecture change. Do not bypass the conflict in prompts or silently delete a guard. Research ends with a supported choice or a precise blocker, not another broad research backlog.

## Dispatch order

```text
P0 baseline + interface decisions
  ├─ P1 interactive coding ─ P3 continuation/recovery ─ P5 dependency execution
  ├─ P2 clarification + automatic planning ────────── P6 mission plugin
  └─ P4 UI primitives (isolated demo states)

P1 + P3 + P4 + P5 ─ P7 live Mission UI
P1 + P3 + P4      ─ P8 compact Result/rework
all implemented packets ─ P9 integration review + packaged gate
P10 documentation reconciliation runs at each integrated slice, finalized after P9
```

P6 also depends on the P0 command contract and P1 proven session binding. P7 cannot claim concurrency until P5 passes. P8 and P7 may run concurrently only after their shared hooks/API ownership is assigned. P2/P1 shared DTO edits are serialized by the coordinator.

Use at most three implementation workers at once initially. Workers may reason/edit in parallel in isolated worktrees; only one test suite/build/provider conformance probe at a time on this Mac. Coordinator owns integration and test scheduling. Do not launch workers directly into the dirty primary checkout.

## Model and delegation policy

Packets are model-neutral. Confirm installed model names, tool support and credentials before dispatch; this plan does not certify availability of any example model. Codex and Claude are not open-source model families merely because an associated tool may have public source.

- Strong planning/review model: P0, lifecycle design, permission changes, P9; independently challenge assumptions.
- Economical coding model that passes a small repo test: bounded P2/P4/P6/P10 slices, tests and mechanical updates.
- Runtime/migration workers: select by demonstrated repository competence, not brand or cheap token price alone.
- Use subagents for bounded investigation/review with independent context. Use separate WorkUnits for independently scheduled effects/artifacts. Use parallel workers only with non-overlapping ownership and explicit integration.
- No automatic provider/model fallback. No hidden model router or exhaustive provider scoring system in this release.

The first worker should implement one slice, return evidence, and wait for integration. Do not give a cheaper model an instruction to rewrite all of Kennel unattended.

## Copy-paste dispatch prompt

> Implement packet Pn from docs/handoffs/2026-09-15-core-loop-workers/PACKETS.md. Read START-HERE.md and repository AGENTS.md first, then only the packet's relevant source and canonical references. Use the coordinator-provided baseline SHA and isolated worktree. Reproduce the target failure before editing. Respect dependencies and assigned file ownership. Do not push, open/modify PRs or issues, merge, change release state, delete user data, or edit protected authority documents without scoped approval. Run tests only when the coordinator assigns the serial test slot. Return the completion receipt below. If the packet cannot work without changing a shared interface, report that delta before expanding scope. Do not mark runtime or owner Acceptance passed from unit tests.

Coordinator must replace Pn, baseline, worktree, and allowed paths with actual values. An unfilled dispatch is not ready to execute.

## Completion receipt and review

Each worker returns: packet ID; baseline and final commit/diff; touched paths; baseline reproduction; changes/removals; exact commands and exit/results; evidence paths; unresolved gaps; migration/rollback implications; dependencies for next worker. Raw logs stay in an evidence directory, secrets redacted. Summaries link evidence rather than copying entire transcripts.

Coordinator spotchecks the critical path and reviews the diff. Independent reviewer first tries the packet's falsifier. An implementer cannot self-award the integrated gate. Integrate one bounded slice at a time, then rebase dependent work. On failure, return a targeted correction packet rather than regenerating the whole plan.

Use one coordinator-owned execution ledger recording packet status, SHA, owner, evidence and blocker. Reuse an existing active ledger if one is present. Workers submit receipts; they do not simultaneously rewrite the ledger. This is project delivery bookkeeping, not a second Kennel runtime store.

## Final gate

Follow kennel-issue38-launch-gate-handoff.md exactly after locating and verifying the authoritative file. Preserve the original negative matrix and add the packet falsifiers; do not substitute a shorter smoke test. Package with npm --prefix frontend run package followed by package:identity. Use isolated documented environment overrides, a disposable repository and a bare main remote created before Attempts. Full journey is UI-only; API evidence inspection is read-only.

Required coverage: batch clarification, automatic planning, owner approval, useful execution, live input, dependency branch/integration, planning and active-execution restart, post-execution restart, stopped-session restore fence, failed check, bounded rework, provider/readiness failure, duplicate recovery, stale revision, clean quit, Result and owner Accept. Preserve exact SHA, versions/hashes, IDs, digests, screenshots/recording, logs and all blocked/unaccepted rows. Every rescue is declared and the rescued case does not pass. Acceptance belongs to the owner; record who clicked and any explicit instruction.

No suites or packaged journey were run for this documentation task. The engineering standards selector was not invoked; workers must record any applicable repository standards and unavailable checks in P0 rather than inventing a pass.
