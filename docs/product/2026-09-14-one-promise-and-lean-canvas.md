# One promise, and what to cut

- **Status:** Working answer to "should Kennel exist, and what do we simplify" — not a locked decision
- **Date:** 2026-09-14
- **Reads against:** `docs/product/2026-09-08-positioning-and-competitive-thesis.md` (locked positioning — this doc
  doesn't restate it, only adds deltas) and `docs/handoffs/2026-09-13-work-mode-production-build-order/HANDOFF.md`
  (the current build-order audit)

## Short answer

Kennel's thesis is right. The last three merged PRs contradict it.

Positioning §12 lists claims to avoid: *"a better agent kanban," "the easiest way to run agents in parallel."*
§5.2 concedes Warp Oz already owns *"multi-harness control plane"* as their language. That doc is correct that
Board/DAG/terminal/lineage UI is commodity — every competitor listed in §5.1/5.2 already has it.

PRs #176, #177, #178 built exactly that commodity layer (Evidence UI, Mission Control DAG, historical lineage
inspection) while #115 — the one thing that makes any of it show real data instead of a `needs_you` dead end —
has zero comments, zero assignee, zero PR. That's not a sequencing slip. It's the team's actual priorities
disagreeing with the locked positioning. Say that plainly internally, because it will happen again unless the
build order enforces it (it currently does — Phase 1 is #115 first — so the fix is to actually hold that order,
not to write a new one).

## The loop to prove already exists — it's positioning §10

Don't invent a fresh "most important loop." The positioning doc already wrote seven falsification tests for the
USP. Score them against the live build today:

| §10 test | Status today |
| --- | --- |
| After 2+ Attempts, understand Outcome state without reading transcripts | ❌ can't — no real Attempt completes |
| Provider/model switch mid-WorkUnit doesn't break continuity | Untested — nothing to switch between yet |
| A failed/retried Attempt doesn't rewrite prior history | ✅ #178 (lineage) looks solid on this |
| System explains why work is blocked or needs approval | 🟡 partial — surfaces `needs_you`, not *why* it's structurally stuck |
| "Ready for review" is evidence-backed, not inferred from session end | ❌ can't — no artifact, no evidence |
| Owner can reject/rework without manually reconstructing the task | Untested |
| Supervision time is materially lower than manual coordination | Unmeasurable — nothing to measure yet |

Two of seven pass. That's the real state of the product, independent of how much UI has shipped. #115 is the
single item that unblocks four of the failing five.

## What to actually remove (not relabel)

The user's 9-step loop is a fine internal model. Collapsing it to "4–5 steps" would just rename it — the ask was
what to cut. Two real cuts:

1. **Merge Clarify (step 2) and Authorize (step 5) into one approval gate.** For a solo user on their own repo,
   two separate human stops before anything runs is the exact friction that makes Kennel feel heavier than just
   prompting Codex directly. One screen: here's the Contract, here's the Plan, approve or edit — not two.
2. **Cut multi-harness routing for v1.** Step 6 ("deterministically assigns each WorkUnit to a compatible local
   harness") implies a capability-snapshot + binding-record subsystem for choosing between harnesses. Positioning
   §5.1 already concedes model routing is Factory's commodity claim, and the user's own framing has Codex as the
   primary harness with the OpenAI API only as a Contract/Plan fallback. Pin execution to Codex. Delete the
   routing-decision machinery from the critical path — it can come back once there's a second harness worth
   routing to.

Everything else in the 9 steps stays; the domain rigor (Contract, Evidence, Verification, Acceptance) is the
product, not the overhead.

## Lean canvas, answered against the real ICP

1. **Value proposition** — Turn a stated outcome into a governed, autonomous Codex run with visible state and
   evidence you can actually accept, instead of babysitting a session or re-reading a diff to reconstruct what
   happened.
2. **Customer segments** — Two different people, see the fork below. Both are Codex/Claude Code power users whose
   work has outgrown one-prompt-to-PR.
3. **Channels** — Wherever the ICP already is when they're using a harness: invoked from inside Codex itself
   (distribution rides a tool they open daily), dev X/Twitter, agent-building Discords/r/ClaudeAI, founder-led
   build-in-public demos.
4. **Customer relationships** — High-touch design partnership early (direct Discord/Slack with the first ~10
   users, iterate on their specific broken loop). Self-serve only after the §10 tests pass repeatedly, not before.
5. **Key resources** — The Outcome/Contract/Evidence domain model and ledger (the actual IP), the governed
   execution layer (repository capabilities, daemon), and founder credibility with the ICP as a fellow power user,
   not a tooling vendor selling to them.
6. **Key activities** — (1) land #115 so one governed Attempt is honestly trustworthy end to end, (2) hold the
   build order so commodity UI never again gets built ahead of the thing that makes its data real, (3) get real
   power users to run real Outcomes and say out loud whether "Accept" ever felt earned.
7. **Key partnerships** — OpenAI/Codex and Anthropic/Claude Code, since the whole model depends on being allowed
   to sit above their sessions rather than being redundant with them; early design-partner users as case studies.
8. **Revenue streams** — Not the near-term question (see WTP below for why). When it is: usage/outcome-based
   (priced per governed Outcome, or seat for teams) — the thing being sold is trust and coordination, not tokens,
   so per-token pricing undersells it.
9. **Cost structure** — Model/token cost of governance overhead (Contract drafting, Plan generation, verification
   runs) plus engineering cost of keeping the governed-capability layer working across harness updates. This is
   the same number as the willingness-to-pay objection below — it's one line item, not two separate questions.

## Willingness to pay is a three-way cost stack, not a yes/no

The ICP already pays for Codex or Claude Max. Kennel adds a second subscription. Governance adds token overhead
on top of that (Contract drafting + Plan generation + verification runs, all before any code gets written). So
the real question isn't "will they pay for Kennel" — it's **"will they pay twice, plus overhead, for less time
spent reviewing."** That only clears if verification/Contract-drafting costs less in tokens+time than the
review time it replaces. Right now that trade can't even be measured, because #115 means no governed Attempt
produces a real artifact to review in the first place. This is the same fact as the §10 gap above, restated as
an economic constraint instead of a product one.

## The ICP fork — name it, don't hedge it

The user's own description of a Codex/Claude Code power user — brainstorm skill, decompose, one orchestrator,
fan out to workers, consolidate, review — describes someone who **already built this loop by hand.** They feel
the coordination pain most acutely, and they are also the hardest to switch, because switching means abandoning
a personal workflow they spent real effort tuning. Productizing their exact loop competes with their own taste.

The easier segment is one tier down: developers who *tried* running parallel/multi-step agent work, found the
coordination overhead not worth it, and retreated to one session at a time. They have felt the problem exists
but don't have (or want to build) the expert's workaround. For them, Kennel isn't competing with a tuned personal
practice — it's the first thing that makes the multi-step loop viable for them at all.

This is a real fork, not a hedge to avoid picking one:

- **Productize the expert's practice** → sell orchestration sophistication, target power users, compete with
  their own hand-built loop.
- **Give the non-expert the expert's loop** → sell trust and a floor of coordination they've never had, target
  people who bounced off multi-agent work once already.

Recommendation: build for the second segment first. It's the segment where "governed, evidence-backed, safe to
accept" is the actual unlock rather than a nice-to-have on top of a loop they'd tune themselves anyway — and it's
the segment where the §10 tests, once they pass, are sufficient to win the user rather than merely matching what
they already built for themselves.

## Bottom line

The thesis doesn't need to change. The build order (Phase 1 = #115 first) already encodes the right priority —
the discipline needed is holding it, not rewriting it. Two loop steps are worth actually cutting (merge
Clarify+Authorize, cut multi-harness routing for v1). And the willingness-to-pay question won't have a real
answer until #115 lands and one governed Attempt can produce evidence worth charging for.
