# Build order: Kennel desktop Work mode → production ready

Date: 2026-09-13
Based on: the verified `beta` audit at `c1ba9f78` (report under
`~/.kennel/ux-audits/20260913T053349Z-latest-beta-c1ba9f78/`) plus a live check
of GitHub issue state on 2026-09-13.

This is the order I'd build in, not just a list. Each phase assumes the ones
above it are done, because the later work is either blocked on it or would be
wasted polishing a surface that's about to change underneath it.

## Phase 0 — Fix the three known correctness/consistency bugs (small, do first)

These are cheap, low-risk, and any demo or QA pass right now will keep tripping
over them.

1. Governed RunBrief tells the provider "approved checks" are a governed tool,
   but checks are daemon-only. `backend/internal/service/outcome/attempt.go:769`
   vs `backend/internal/governedtools/server.go:136`.
2. The embedded `using-kennel` skill prompt still says "ao CLI" / "When using
   ao" instead of `kennel`. `backend/internal/session_manager/manager.go:3516`.
3. The T0/P0 Playwright gate targets the old session-board route and
   `data-testid="board"`, and two elements share `daemon-status` test IDs. It's
   currently 8 passed / 15 failed — not because the app is broken, but because
   the harness wasn't updated for the Outcome-first Work route. Fix this before
   relying on it as a merge gate for the phases below, or every subsequent PR
   will look red for the wrong reason.

## Phase 1 — Make governed execution actually work: #115

**Derive and deliver bounded repository capabilities for governed WorkUnits.**

This is the real blocker, not a polish item: a live Attempt right now comes
back `needs_you` because the provider gets the validator command but no
repository-inspection capability, so it never authors a report. Nothing
downstream (#35's Evidence, #38's end-to-end proof) can be honestly
demonstrated until a governed WorkUnit can actually read/write/exec inside its
leased workspace and produce an artifact. Do this first.

## Phase 2 — Finish Evidence and Result: #35

**Complete WorkUnit-scoped Evidence and Result experience.**

Depends on #115 — you need real artifacts and check runs coming out of a
governed Attempt before you can bind them to Contract criteria and render a
truthful Result/rework screen. Once #115 lands, this is the piece that turns
"the provider exited" into "here's what was checked, what changed, and what's
next," and makes reject/rework/restart safe (no duplicate proof, no
auto-acceptance).

## Phase 3 — Make Mission Control tell the truth: #78

**Complete direct WorkUnit DAG Mission Control and Board/List navigation.**

Mostly UI/composition work (PRs #84, #103–#105, #110 already landed the
shell). Can start in parallel with Phase 1/2, but should be functionally done
before Phase 4, since #116 explicitly consumes this composition. The key gap:
render the real PlanRevision WorkUnit DAG and daemon-derived
runnable/waiting/blocked state instead of a substitute graph, and keep
Board/List as pure projections with no renderer-side lifecycle authority.

## Phase 4 — Historical lineage and restart: #116 (blocked on #78 **and** #82)

**Inspect and engage historical Outcome sessions from Mission Control.**

This is what proves the "rework creates new attributed execution, not
rewritten history" story from the demo script. It has two upstream
dependencies: #78 (Mission Control composition, Phase 3) and **#82** ("Wire
governed Project Waldo replies and bounded continuation"), which is not in the
Work milestone and isn't currently scheduled alongside it — worth pulling
forward or at least explicitly scoping down, since #116 can't honestly ship
without knowing what "resumable" vs. "ended" means at that boundary.

## Phase 5 — Scoped accessibility/reduced-motion: #16 (Work-mode slice only)

Not all of #16 — it also covers Home/brand polish that isn't on the critical
path. Just the packaged accessibility and reduced-motion coverage that the
Outcome journey (Mission Control, Session Inspector, Evidence/Result) needs to
be honestly demoable to a keyboard/screen-reader user and to a reduced-motion
system setting. Do this after Phase 3/4 land, since it's cheapest to audit UI
that's no longer being restructured.

## Phase 6 — The gate: #38

**Prove one autonomous Outcome end to end.**

This is the umbrella acceptance milestone, not new build work — it's the
proof pass (fresh packaged profile, real repo, restart app/daemon mid-flight,
no manual DB/terminal injection) that Phases 1–5 actually compose. This is
also the point where the recommended video script (Contract → Plan →
authorize → Attempt → rework → Acceptance → restart → historical lineage)
becomes something you can record for real, because every step it depends on
(#115, #35, #78, #116) is now true rather than staged.

Doing #38 before the phases above land just reproduces the same
`needs_you`/no-capability failure and other gaps the audit already found.

---

## Separate track: public-release readiness (does not block the internal video)

These gate *downloadable/launch-ready*, not the Work-mode Outcome journey
itself. Sequence them after Phase 6, or in parallel on a different track if
someone else owns them:

- **#118** — security reporting + repository protection (private vuln
  reporting is currently disabled, `beta` unprotected, `main` has no required
  checks/reviews).
- **#62** — activate hosted launch CI gates. Two open PRs against `beta`
  (#102, #89) currently have no hosted checks.
- **#117** — publish/verify the install and update path.
- **#22** / **#27** — finish detaching and removing reviewed AO donor
  surfaces (repo hygiene).
- 166 open Dependabot alerts (92 high) sit under this track too — not a
  Work-mode blocker, but not something to launch with either.

## One doc-hygiene note

`docs/STATUS.md:3` still describes the checkpoint as PR #128 /
`f12b9b62b`; live `beta` is now at `c1ba9f781` (PR #129 merged). Fix this
whenever it's next touched so it doesn't mislead the next person reading it.
