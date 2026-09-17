// These tests actually exercise the deadline/abort path — not just the
// structural "does the spec parse" check — using short real timings (tens of
// milliseconds) so they stay fast while still proving the two review-round-3
// behaviors: teardown is never gated by abort (defect 1), and a fired
// deadline waits for the in-flight body to settle before returning instead
// of racing ahead of it (defect 2).

import { test } from "node:test";
import assert from "node:assert/strict";

import { guardedStep, raceWithDeadline, unguardedStep } from "./deadline-control.mjs";

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

test("raceWithDeadline: returns the value when fn settles before the deadline", async () => {
	const controller = new AbortController();
	const result = await raceWithDeadline(async () => "done", 200, { signal: controller });
	assert.deepEqual(result, { ok: true, value: "done" });
	assert.equal(controller.signal.aborted, false);
});

test("raceWithDeadline: propagates fn's own rejection untouched when it is not a deadline", async () => {
	const controller = new AbortController();
	await assert.rejects(
		raceWithDeadline(
			async () => {
				throw new Error("real failure");
			},
			200,
			{ signal: controller },
		),
		/real failure/,
	);
});

test("raceWithDeadline: defect 2 — waits for a cooperative fn to actually settle after the deadline fires, not just after the race rejects", async () => {
	const controller = new AbortController();
	let bodySettledAt = 0;
	const startedAt = Date.now();

	const result = await raceWithDeadline(
		async (signal) => {
			// A well-behaved journey body: poll until it observes the abort,
			// mirroring how waitFor()/step() in the real spec check signal.aborted.
			while (!signal.aborted) {
				await sleep(10);
			}
			await sleep(30); // simulate one bounded, non-abort-aware action finishing
			bodySettledAt = Date.now();
			return "abandoned-but-cooperative";
		},
		50, // deadline fires quickly
		{ signal: controller, settleGraceMs: 500 },
	);

	assert.equal(result.ok, false);
	assert.equal(result.deadlineExceeded, true);
	assert.equal(result.settledInTime, true);
	assert.equal(controller.signal.aborted, true);
	// The call must not have returned before the body actually finished settling
	// — this is the literal defect: a bare Promise.race would have returned
	// the instant the 50ms deadline fired, long before bodySettledAt is set.
	const returnedAt = Date.now();
	assert.ok(bodySettledAt > 0, "the journey body must have actually run to completion");
	assert.ok(returnedAt >= bodySettledAt, `raceWithDeadline returned (${returnedAt - startedAt}ms) before the body settled (${bodySettledAt - startedAt}ms)`);
});

test("raceWithDeadline: defect 2 — a non-cooperative fn is bounded by settleGraceMs, not waited on forever", async () => {
	const controller = new AbortController();
	const started = Date.now();
	const result = await raceWithDeadline(
		async () => {
			// Never observes the abort signal at all — simulates a stuck native
			// action. The harness must not hang forever waiting for it. Kept short
			// (not e.g. 10s) purely so this test file doesn't hold the process
			// open waiting for the abandoned timer to clear; the ratio to
			// settleGraceMs is what matters, not the absolute duration.
			await sleep(1000);
			return "too-late";
		},
		50,
		{ signal: controller, settleGraceMs: 150 },
	);
	const elapsed = Date.now() - started;
	assert.equal(result.ok, false);
	assert.equal(result.settledInTime, false);
	// Bounded near settleGraceMs (plus the 50ms deadline), nowhere near the
	// fn's own 1s sleep.
	assert.ok(elapsed < 500, `expected a bounded wait, took ${elapsed}ms`);
});

test("guardedStep: defect 1 context — refuses to invoke fn once the signal is already aborted", async () => {
	const controller = new AbortController();
	controller.abort(new Error("deadline already exceeded"));
	let called = false;
	await assert.rejects(
		guardedStep(controller.signal, async () => {
			called = true;
		}),
		/skipped: signal already aborted/,
	);
	assert.equal(called, false, "guardedStep must not invoke fn once already aborted");
});

test("guardedStep: invokes fn normally when the signal is not aborted", async () => {
	const controller = new AbortController();
	let called = false;
	const result = await guardedStep(controller.signal, async () => {
		called = true;
		return "ok";
	});
	assert.equal(called, true);
	assert.equal(result, "ok");
});

test("unguardedStep: defect 1 — invokes fn even when the signal is already aborted (teardown must never be skipped)", async () => {
	const controller = new AbortController();
	controller.abort(new Error("deadline already exceeded"));
	let called = false;
	const result = await unguardedStep(async () => {
		called = true;
		return "closed";
	});
	assert.equal(called, true, "unguardedStep (used for teardown, e.g. app.close()) must run regardless of the abort signal");
	assert.equal(result, "closed");
});
