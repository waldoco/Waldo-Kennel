// Deadline/abort concurrency primitives for the packaged macOS Outcome
// journey harness, extracted so the two review-round-3 defects they exist to
// fix are actually unit-testable (with fake short timings, no Electron/
// Playwright/live Codex needed) rather than only structurally verified.
//
// Defect 1 (CRITICAL): a bare abort-gated step wrapper must never be reused
// for teardown — teardown has to run on every exit path, including the exact
// path (a fired deadline) that gates ordinary journey steps.
//
// Defect 2 (HIGH): `Promise.race([fn(), deadline])` alone settles the moment
// the deadline promise rejects; it does nothing to stop `fn()` from
// continuing to run "abandoned" in the background, where it can keep
// mutating shared state concurrently with whatever the caller does next
// (e.g. teardown reading/serializing that same state). raceWithDeadline
// waits for `fn` to actually settle after a deadline, bounded by
// settleGraceMs so a genuinely non-cooperative `fn` cannot hang the harness
// forever either.

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

/**
 * Runs `fn(signal)` racing an overall deadline of `ms`. `signal` is aborted
 * the instant the deadline fires, so any signal-aware code inside `fn` can
 * stop cooperatively.
 *
 * - If `fn` settles before the deadline: returns `{ ok: true, value }`, or
 *   rethrows if `fn` itself rejected (a real failure, not a timeout).
 * - If the deadline fires first: waits for `fn` to actually settle (bounded
 *   by `settleGraceMs`) before returning, so the caller never starts
 *   teardown concurrently with a still-running journey body. Returns
 *   `{ ok: false, deadlineExceeded: true, settledInTime, error }`.
 *
 * @template T
 * @param {(signal: AbortSignal) => Promise<T>} fn
 * @param {number} ms
 * @param {{ signal: AbortController, settleGraceMs?: number }} opts
 * @returns {Promise<{ok: true, value: T} | {ok: false, deadlineExceeded: true, settledInTime: boolean, error: Error}>}
 */
export async function raceWithDeadline(fn, ms, { signal, settleGraceMs = 30_000 } = {}) {
	if (!signal) throw new Error("raceWithDeadline requires an AbortController as `signal`");
	const journeyPromise = fn(signal.signal);
	let deadlineExceeded = false;
	let timer;
	const deadline = new Promise((_, reject) => {
		timer = setTimeout(() => {
			deadlineExceeded = true;
			const err = new Error(`overall journey ceiling of ${ms}ms exceeded`);
			signal.abort(err);
			reject(err);
		}, ms);
	});
	try {
		const value = await Promise.race([journeyPromise, deadline]);
		return { ok: true, value };
	} catch (err) {
		if (!deadlineExceeded) throw err; // fn's own failure — not a deadline, propagate as-is
		let settledInTime = false;
		void journeyPromise.then(
			() => (settledInTime = true),
			() => (settledInTime = true),
		);
		await Promise.race([journeyPromise.catch(() => undefined), sleep(settleGraceMs)]);
		return { ok: false, deadlineExceeded: true, settledInTime, error: err };
	} finally {
		clearTimeout(timer);
	}
}

/** Refuses to invoke `fn` at all once `signal` is already aborted — the
 * ordinary journey-step gate. Never use this for teardown (see unguardedStep). */
export async function guardedStep(signal, fn) {
	if (signal?.aborted) {
		throw new Error(`skipped: signal already aborted (${signal.reason instanceof Error ? signal.reason.message : String(signal.reason)})`);
	}
	return fn();
}

/** Always invokes `fn`, regardless of any abort signal. Teardown must use
 * this, never guardedStep, or cleanup silently never runs on the deadline
 * path (review round 3 defect 1). */
export async function unguardedStep(fn) {
	return fn();
}
