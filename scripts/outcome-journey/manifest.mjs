// Typed manifest builder for the packaged macOS Outcome journey harness.
//
// Pure functions only: no filesystem, no process, no network. The
// orchestration script (run-outcome-journey.mjs) fills the shape this module
// defines and validates; this module never decides HOW evidence is collected,
// only what a complete, honest manifest must contain.
//
// Result classification is passed/failed/blocked/ambiguous (docs/handoffs/
// 2026-09-16-macos-outcome-journey-harness-plan.md §9, final amendment 2) —
// four states, not three: an ambiguous run (a mutating request was accepted
// but its effect never settled within its ceiling) is neither an ordinary
// failure nor a blocked precondition, and collapsing it into "failed" would
// hide exactly the class of uncertainty this harness exists to surface
// honestly. All three non-"passed" values must still exit nonzero.

export const SCHEMA_VERSION = "1.0.0";

export const RESULTS = Object.freeze(["passed", "failed", "blocked", "ambiguous"]);

const SECRET_HEADER_NAMES = new Set(["authorization", "cookie", "set-cookie", "proxy-authorization"]);

/**
 * Redacts secret-bearing header names from a captured request/response
 * ledger entry. IDs, status codes and timing are never touched — the plan's
 * redaction rule is "never remove the IDs/state needed to review the proof."
 */
export function redactHeaders(headers = {}) {
	const out = {};
	for (const [key, value] of Object.entries(headers)) {
		out[key] = SECRET_HEADER_NAMES.has(key.toLowerCase()) ? "[redacted]" : value;
	}
	return out;
}

/** Redacts one network-ledger entry (§7.3), leaving method/path/status/ids intact. */
export function redactLedgerEntry(entry) {
	if (!entry || typeof entry !== "object") return entry;
	const next = { ...entry };
	if (next.requestHeaders) next.requestHeaders = redactHeaders(next.requestHeaders);
	if (next.responseHeaders) next.responseHeaders = redactHeaders(next.responseHeaders);
	return next;
}

/**
 * Maps the harness's own observed facts to a result + exit code.
 *
 * Precedence matches the honesty boundary: an unmet precondition (blocked)
 * is checked before an ambiguous accepted-but-unsettled mutation, which is
 * checked before an ordinary assertion failure. In a real run at most one of
 * these should ever be true; the order exists so a caller that sets more than
 * one by mistake still gets the most conservative classification, not the
 * most optimistic one.
 */
export function deriveJourneyOutcome({ blocked = false, ambiguous = false, failed = false } = {}) {
	if (blocked) return { result: "blocked", exitCode: 1 };
	if (ambiguous) return { result: "ambiguous", exitCode: 1 };
	if (failed) return { result: "failed", exitCode: 1 };
	return { result: "passed", exitCode: 0 };
}

const REQUIRED_TOP_LEVEL = [
	"schemaVersion",
	"runId",
	"startedAt",
	"result",
	"build",
	"profile",
	"fixture",
	"steps",
	"identities",
	"apiAssertions",
	"networkLedger",
	"facetsCheck",
	"artifacts",
	"cleanup",
	"executionStarted",
	"boundaryReached",
	"limitations",
	"unverifiedClaims",
];

/**
 * Validates a completed manifest against the schema in the approved plan
 * (§7.1). Structural completeness only — it does not re-derive whether the
 * journey passed, only whether the manifest honestly represents what
 * happened. A run with missing required evidence is a failed proof per the
 * plan's honesty boundary, and this is the function that catches that.
 *
 * @returns {{valid: boolean, errors: string[]}}
 */
export function validateManifest(manifest) {
	const errors = [];
	if (!manifest || typeof manifest !== "object") {
		return { valid: false, errors: ["manifest must be an object"] };
	}
	for (const key of REQUIRED_TOP_LEVEL) {
		if (!(key in manifest)) errors.push(`missing required field: ${key}`);
	}
	if (manifest.schemaVersion !== SCHEMA_VERSION) {
		errors.push(`schemaVersion must be ${SCHEMA_VERSION}, got ${manifest.schemaVersion}`);
	}
	if (!RESULTS.includes(manifest.result)) {
		errors.push(`result must be one of ${RESULTS.join("/")}, got ${manifest.result}`);
	}
	// Hard invariant: this slice never starts a persistent Attempt. A manifest
	// that ever set this true would be claiming execution happened, which the
	// honesty boundary forbids outright regardless of how the run otherwise went.
	if (manifest.executionStarted !== false) {
		errors.push("executionStarted must be false — this slice never starts a persistent Attempt");
	}
	if (!Array.isArray(manifest.steps) || manifest.steps.length === 0) {
		errors.push("steps must be a nonempty array");
	}
	if (!Array.isArray(manifest.artifacts) || manifest.artifacts.length === 0) {
		errors.push("artifacts must include at least one captured screenshot/log — missing evidence is a failed proof, not a warning");
	}
	if (manifest.networkLedger) {
		if (!Array.isArray(manifest.networkLedger.shortcutEndpointsObserved)) {
			errors.push("networkLedger.shortcutEndpointsObserved must be an array");
		} else if (manifest.networkLedger.shortcutEndpointsObserved.length > 0) {
			errors.push("networkLedger.shortcutEndpointsObserved must be empty — the journey is UI-only (amendment 4)");
		}
	} else {
		errors.push("missing networkLedger");
	}
	if (manifest.build) {
		if (manifest.build.identityMatch !== true) {
			errors.push(
				"build.identityMatch must be true — the daemon's live buildRevision must match the recorded git SHA before any UI step (§3.1 amendment 3)",
			);
		}
	}
	return { valid: errors.length === 0, errors };
}

/** ISO-8601 UTC timestamp. Isolated so tests can substitute a fixed clock. */
export function isoNow() {
	return new Date().toISOString();
}

/** Starts a named step record. `now` is injectable for deterministic tests. */
export function startStep(name, now = isoNow) {
	return { name, startedAt: now(), endedAt: null, durationMs: null, result: null, failureCode: null };
}

/** Closes a step record with its outcome, computing duration from its own startedAt. */
export function finishStep(step, { result, failureCode = null }, now = isoNow) {
	const endedAt = now();
	const durationMs = Math.max(0, Date.parse(endedAt) - Date.parse(step.startedAt));
	return { ...step, endedAt, durationMs, result, failureCode };
}

/** Builds the manifest's top-level shape from already-assembled sections. */
export function createManifest({
	runId,
	startedAt,
	completedAt,
	result,
	build,
	profile,
	fixture,
	steps,
	identities,
	apiAssertions,
	networkLedger,
	facetsCheck,
	artifacts,
	cleanup,
	executionStarted,
	boundaryReached,
	limitations,
	unverifiedClaims,
}) {
	return {
		schemaVersion: SCHEMA_VERSION,
		runId,
		startedAt,
		completedAt,
		result,
		build,
		profile,
		fixture,
		steps,
		identities,
		apiAssertions,
		networkLedger,
		facetsCheck,
		artifacts,
		cleanup,
		executionStarted,
		boundaryReached,
		limitations,
		unverifiedClaims,
	};
}
