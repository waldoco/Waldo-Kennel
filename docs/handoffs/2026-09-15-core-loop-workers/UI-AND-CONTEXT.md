# UI direction and documentation cleanup proposal

Status: DRAFT FOR OWNER REVIEW. No UI changes, dependency installation, documentation retirement or deletion authorized. References inspected on 2026-09-15 through published pages; animations have not been visually tested in Kennel.

## Four surfaces, one Outcome

Define: outcome composer, selected repository/provider readiness, material question batch, concise agreed result. Questions may come in several rounds; avoid ones answered by repository context.

Approve: short approach and main work steps, meaningful scope/permission consequence, Approve and run. Internally record authorization before execution; a combined button may orchestrate both without conflating their facts. Unsupported scope is explained here, before launch. Full Contract, graph details and provenance expand.

Run: Mission graph above Active sessions Kanban. Graph communicates dependencies; session cards identify real bound sessions. A compact status strip answers what is happening, what needs the user and what comes next. A session drawer supports direct conversation or the actual native terminal. History is available without filling the active board.

Review: what changed, diff/preview, check summary, unresolved exceptions; Accept or Request changes. Automated evidence collection avoids technical entry forms. Successful checks never hide limitations or imply automatic acceptance.

## Reference mapping

| Kennel surface | Reference inspiration | Adaptation |
| --- | --- | --- |
| Pending operation | Beautiful UI loading state and task rows | Specific phase, elapsed time, last event; subtle indicator |
| Clarification / Needs You | Beautiful UI approval card | Several material questions, partial answers, free text; real reply target |
| Session engagement | Beautiful UI chat, prompt bar and tool chips | Actual streamed messages, compact tool activity, send/interrupt; no invented reasoning |
| Review | Beautiful UI diff/code presentation | Existing real diff renderer with concise summary and exceptions |
| Open session / details | Transitions panel reveal and accordion | Short interruptible transition retaining focus and selection |
| Verification / action feedback | Transitions state swaps and spinner-to-check | Success transition only on acknowledged success |
| Initial data fetch | Transitions skeleton/reveal | Skeleton only before data; preserve existing content during refresh |

Sources: https://www.beautifului.dev/ and https://transitions.dev/ . Beautiful UI publishes an MIT license at https://www.beautifului.dev/license; retain applicable notices when copying. Transitions source is https://github.com/Jakubantalik/transitions.dev and includes free and authenticated Pro recipes. Verify the exact selected recipe's terms before copying; public source visibility alone is not permission. Do not install its live relay, skill, or Pro tooling as part of planning.

Implementation research should select a few specific examples, record source/version/license and capture their behavior. No full-site clone. The current inspected frontend already uses Motion, Radix, xterm and packages/product-ui. Inventory their components at the pinned beta before deciding whether any dependency is missing. Prefer adapting working primitives over replacing every component.

## Feedback contract

| Event | Immediate local feedback | Durable feedback |
| --- | --- | --- |
| User presses action | Pressed/pending state; retain input | Request acknowledged or actionable error |
| Planning starts | Starting planning | Inspecting / preparing / awaiting answers / proposal ready |
| Provider verification | Verifying selected configuration | Verified configuration or reason for failure; changes invalidate visibly |
| Run starts | Starting run | Queued / provisioning / running, reflecting daemon facts |
| User sends message | Pending message with retry identity | Acknowledged, responded, or failed; never silently dropped |
| Stop requested | Stopping | Stopped only after acknowledgment; unknown if disconnected |
| Connection lost | Reconnecting with last known state | Reconciled state; no duplicate start |
| Check completes | Checking | Passed, failed or unavailable; never a generic completion tick |

Proposed responsiveness target: visible local acknowledgment within 100 ms on the test Mac. Measure it; this is a target, not a result. Never add artificial minimum waits or delay useful content for animation. Backend/model latency still exists: disclose it, stream real progress and keep navigation responsive. Do not fabricate percentages, tool steps or hidden chain-of-thought to appear alive.

Use brief opacity/transform transitions for spatial continuity; avoid per-token animation, looping graph motion, blur-heavy effects, moving text while reading, decorative counters and confetti for routine completion. Respect reduced motion. Async operations must remain understandable without animation or color.

Render desktop and narrow-window states, keyboard-only navigation, visible focus, long text, empty/error/offline states, rapid repeated input and reduced motion. Preserve scroll position when user reads older transcript; offer jump-to-latest rather than forcing scroll. Test live daemon state after component fixtures. Screenshot/recording evidence and timing measurements belong to implementation packets, not this proposal.

## Context cleanup: change navigation before deleting history

| Target | Proposed action after owner review |
| --- | --- |
| docs/STATUS.md | Reconcile from the pinned implementation checkout; separate implemented, runtime-tested, accepted and proposed. Primary checkout is older than inspected beta. |
| docs/handoffs/2026-09-12-claude-launch-stabilization/HANDOFF.md | Keep historical evidence; mark batch-only execution guidance superseded once the replacement decision is ratified. Do not execute old cherry-pick instructions. |
| docs/handoffs/2026-09-12-codex-electron-launch-audit/ | Historical audit, excluded from default worker context; retain source/evidence links. |
| KENNEL-GAP-ANALYSIS.md | Historical diagnosis, not current state; exclude from startup reading. Untracked owner file: do not move/delete. |
| backend/internal/skillassets/using-kennel/ | Replace active spawn-first guidance with mission entry after implementation; preserve valid advanced command references. |
| docs/product/kennel-build-program.md and docs/superpowers/plans/2026-09-04-kennel-builds-kennel.md | Point current implementation order to the approved bounded sequence; keep unrelated future work in roadmap. |
| docs/superpowers/specs/2026-08-25-work-experience-screen-interaction-spec.md | Update approved interaction changes with their implementing slice, not ahead as shipped behavior. |
| docs/product/ao-legacy-retirement-audit.md | Treat as historical candidate inventory; recheck callers and compatibility before deletion. |
| docs/README.md and already-retired docs/HANDOFF.md | Existing authority/retirement pointers are useful; avoid churn. |

Keep AGENTS authority, ADR history, license/NOTICE attribution, merged migrations, historical audit evidence, receipts, user profiles and user work. Do not remove a rule merely because it contains permission or fence: establish what failure it prevents, whether the mechanism is duplicated or obsolete, and the tested replacement.

Removal sequence: inventory callers and guidance → approve replacement semantics → implement and test replacement → migrate active callers → remove dead paths → mark historical guidance superseded → verify links and startup reading. No independent delete-everything-AO packet.

## Prevent new context rot

- One active dispatch index: START-HERE.md; one coordinator delivery ledger; packet-specific completion receipts.
- Record status and source SHA on each receipt. Never copy an older checkout's STATUS over a newer one.
- Give each worker only canonical minimum plus its own packet/source/tests. Do not attach the full chat by default.
- Label claims as observed, source-confirmed, inferred, proposed or verified. Link original evidence instead of repeated paraphrases.
- Preserve rejected options as short decision notes, not active instructions repeated throughout the repository.
- Update canonical documentation with reviewed implementation; keep future ambitions out of current behavior claims.
- After this program completes, mark this dispatch package completed/superseded and retain its evidence links. Do not let it become an immortal second roadmap.

## Owner review checkpoints

Before execution, review: product promise and four surfaces; supported native coding profile and direct-session controls; first interactive-session gate; graph/Kanban layout; which parallelism is required for launch; minimal review; documentation-retirement policy. Resolve disagreements here, then dispatch one bounded slice at a time.
