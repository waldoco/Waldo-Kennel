// Shared timeout constants for the packaged macOS Outcome journey harness.
//
// Both the orchestrator (run-outcome-journey.mjs, which spawns the Playwright
// process and enforces its own kill-timeout on that child) and the Playwright
// spec itself (test/macos-outcome-journey/outcome-journey.spec.ts, which sets
// its own test.setTimeout) need the SAME teardown/evidence-write grace period.
// Duplicating the number in both places let them silently drift (review round
// 2 item 4: the orchestrator was killing the child 30s before the spec's own
// internal grace window closed) — importing one constant from here makes that
// class of bug impossible instead of merely commented against.

/** Time the spec's own teardown (close app, wait for pid exit, write the
 * evidence fragment) is allowed beyond the core journey's own deadline. */
export const TEARDOWN_GRACE_MS = 90_000;

/** Extra margin the ORCHESTRATOR's process-level kill-timeout adds beyond the
 * spec's own (overall + TEARDOWN_GRACE_MS) test.setTimeout, so Playwright's
 * own timeout always fires first and produces a clean report — the
 * orchestrator's hard kill is a last resort for a genuinely wedged process,
 * not the expected path. */
export const ORCHESTRATOR_EXTRA_MARGIN_MS = 30_000;
