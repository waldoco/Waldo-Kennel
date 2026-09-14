# Product experience

This is the canonical screen and interaction contract for Kennel's Outcome experience. It projects the [daemon-owned Outcome loop](../architecture/outcome-loop.md); it does not define a second state machine.

## Experience hierarchy

- **Outcome dashboard:** Where does each job stand?
- **Ask, Approve, Watch, Decide:** What does the owner need now?
- **Mission graph:** How are the pieces connected and what unlocks next?
- **WorkUnit board:** What is actively happening?
- **Detail drawer:** What did this exact WorkUnit and Attempt do?

Progressive detail lets a new user follow one Outcome without understanding provider internals, while a power user can inspect every binding, event, change, check, and lineage record.

## Global shell

The primary product object is an Outcome. The dashboard supports compact Kanban and list projections of Outcomes and exposes current state, progress, blocker, risk, and next action. It does not promote raw sessions to peer status with Outcomes.

Inside an Outcome, the durable navigation is:

- **Work:** Ask and Approve, then the live mission view;
- **Result:** Verification, evidence, rework, and Accept;
- **History:** revisions, decisions, Attempt lineage, events, and audit detail.

Watch is a state of Work, not a separate architecture. Empty or unavailable sections collapse rather than showing placeholder architecture.

## Ask

Ask starts with one Outcome composer. The owner states the desired result and may attach or select a project and source material.

Kennel then:

- acknowledges the Outcome immediately;
- inspects available project context;
- shows reasoning activity without exposing hidden chain-of-thought;
- asks one load-bearing question at a time;
- explains why an answer changes success, scope, authority, feasibility, or proof;
- preserves answers as attributed decisions.

Do not turn Contract fields into a form the user must complete. Do not ask questions whose answers can be grounded from the project.

## Approve

Approve is one coherent review, not separate Contract, Plan, permission, and provider-readiness chores.

It shows:

- the Outcome and success criteria;
- constraints, non-goals, stop conditions, and proof requirements;
- the WorkUnit dependency plan in plain language;
- harness choice and capability fit;
- requested permissions and external effects;
- time, token, and retry budgets;
- AdmissionVerdict status and actionable reason codes;
- what will be checked and what evidence will be returned.

The primary action authorizes one exact Contract and Plan revision. If admission fails or becomes stale, approval is disabled and the one true corrective action is shown. Internal IDs, bindings, and raw receipts are available in detail, not the default view.

## Watch: Mission Control graph

The mission graph sits above the board and visualizes the approved WorkUnit DAG.

- each node is a WorkUnit;
- edges are dependencies;
- parallel branches appear side by side;
- every node shows its current projected state;
- the active path is emphasized without relying on motion alone;
- selecting a node filters and focuses the board and detail drawer;
- blocked successors explain which dependency or decision prevents release.

The graph answers: how will this Outcome finish, what can run now, what is blocking the rest, and how close are we?

## Watch: WorkUnit and Attempt board

The card is the WorkUnit. Its current Attempt and harness are shown on the card. Replaced and historical Attempts stay in lineage.

Default columns:

- Ready
- Running
- Needs you
- Verifying
- Done

Waiting and Blocked appear when populated. Empty columns collapse. Retrying and recovering are visible transitions inside the card rather than permanent columns.

Each card shows:

- task name and short expected result;
- assigned verified harness;
- current Attempt number and session health;
- current action in plain language;
- elapsed time and available budget signals;
- changed files and check progress;
- dependency or blocker;
- one true next action when input is required.

Never show unsupported token precision. Never imply progress from animation alone.

## Shared detail drawer

Selecting a graph node or WorkUnit card opens one drawer with:

### Overview

Intent, criteria, dependencies, permissions, budgets, admission, assigned harness, and current binding.

### Live activity

Typed lifecycle events: inspecting, planning, tool call, editing, running a check, waiting, owner input requested, interrupted, recovering, and completed. Summaries can be human-friendly, but raw provenance remains inspectable.

### Changes

Files, diff summary, generated artifacts, and attribution.

### Checks

Commands, status, duration, output, and the criterion each check supports.

### Lineage

Attempt 1 -> needs you -> replaced by Attempt 2. Provider session IDs, resume/fork information, fences, and replacement provenance stay here rather than cluttering the board.

## Decide: Result

Result is organized by the Contract, not by provider chronology.

For each criterion, show:

- pass, fail, partial, or unverified;
- linked evidence and checks;
- changed files or artifacts;
- material caveats;
- which Attempt produced the evidence.

Then show the consolidated diff/artifacts and remaining risk. The owner can:

- **Accept** the Result;
- **Request rework** with a bounded correction that creates new attributed execution;
- **Reject or stop** without deleting history.

Accept is explicit and cannot be inferred from checks, model confidence, or inactivity.

## Loading, progress, and failure

Every asynchronous action acknowledges immediately and provides:

- what Kennel is doing;
- elapsed time when useful;
- whether work is active, queued, waiting, or blocked;
- a cancel/interrupt action when valid;
- retained progress after navigation and restart;
- a specific recovery action for terminal failure.

Use loading states, thinking summaries, tool chips, task rows, diffs, context cards, and approval cards selectively where they explain real state. Generic spinners, fake percentages, silent waits, and preview-only success states are not acceptable.

## Accessibility and responsive behavior

- All commands work by keyboard and expose clear focus.
- Graph and board information has an equivalent list/text representation.
- State never depends on color, icon, or animation alone.
- Status changes use appropriate live-region behavior without narrating every event.
- Long names, paths, diffs, and error text wrap or scroll without hiding actions.
- Narrow layouts preserve the decision order; secondary detail moves into the drawer.
- Contrast, target size, labels, headings, and semantic controls meet the repository's accessibility gate.
- Reduced motion is honored across graph, cards, drawers, progress, and transitions.

## Motion and performance

Motion explains causality: a WorkUnit becoming ready, an Attempt starting, a dependency releasing, progress arriving, a Result becoming reviewable. Prefer transform and opacity. Avoid large blur, mask, shimmer, or filter loops that repaint the Electron renderer. Stop ambient loops offscreen.

Decorative AI glow, copied design-system shells, and animation added before reachable lifecycle state are out of scope.

## Visual evidence gate

A UI package is not ready until actual pixels are inspected in compiled, reachable states. Review dark, light, narrow, long-content, loading, blocked, needs-you, recovering, verifying, Result, reduced-motion, keyboard, and screen-reader semantics. Preview fixtures may support development but cannot be the only evidence for a claimed lifecycle.
