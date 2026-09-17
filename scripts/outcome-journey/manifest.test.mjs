import { test } from "node:test";
import assert from "node:assert/strict";

import {
	SCHEMA_VERSION,
	createManifest,
	deriveJourneyOutcome,
	finishStep,
	redactHeaders,
	redactLedgerEntry,
	startStep,
	validateManifest,
} from "./manifest.mjs";

function baseManifest(overrides = {}) {
	return createManifest({
		runId: "run-1",
		startedAt: "2026-09-16T00:00:00.000Z",
		completedAt: "2026-09-16T00:05:00.000Z",
		result: "passed",
		build: { gitSha: "abc123", identityMatch: true },
		profile: { profileDir: "/tmp/profile" },
		fixture: { repoPath: "/tmp/fixture", initialCommitSha: "def456" },
		steps: [{ name: "clean-entry", result: "passed", failureCode: null }],
		identities: { projectId: "p1", outcomeId: "o1" },
		apiAssertions: [],
		networkLedger: { shortcutEndpointsObserved: [] },
		facetsCheck: { step: 8, identical: true },
		artifacts: [{ path: "screenshots/01-clean-entry.png", sha256: "…" }],
		cleanup: { daemonPidObservedGone: true, retained: [] },
		executionStarted: false,
		boundaryReached: "act_observe: outcome-run-start eligible, not clicked",
		limitations: [],
		unverifiedClaims: [],
		...overrides,
	});
}

test("deriveJourneyOutcome: passed is the default with a zero exit code", () => {
	assert.deepEqual(deriveJourneyOutcome({}), { result: "passed", exitCode: 0 });
});

test("deriveJourneyOutcome: blocked, ambiguous and failed each exit nonzero and stay distinct", () => {
	assert.equal(deriveJourneyOutcome({ blocked: true }).result, "blocked");
	assert.equal(deriveJourneyOutcome({ blocked: true }).exitCode, 1);
	assert.equal(deriveJourneyOutcome({ ambiguous: true }).result, "ambiguous");
	assert.equal(deriveJourneyOutcome({ ambiguous: true }).exitCode, 1);
	assert.equal(deriveJourneyOutcome({ failed: true }).result, "failed");
	assert.equal(deriveJourneyOutcome({ failed: true }).exitCode, 1);
});

test("deriveJourneyOutcome: blocked takes precedence over ambiguous and failed", () => {
	const o = deriveJourneyOutcome({ blocked: true, ambiguous: true, failed: true });
	assert.equal(o.result, "blocked");
});

test("deriveJourneyOutcome: ambiguous takes precedence over failed", () => {
	const o = deriveJourneyOutcome({ ambiguous: true, failed: true });
	assert.equal(o.result, "ambiguous");
});

test("redactHeaders: strips Authorization and Cookie, keeps everything else", () => {
	const redacted = redactHeaders({
		Authorization: "Bearer secret",
		"Set-Cookie": "session=abc",
		"content-type": "application/json",
	});
	assert.equal(redacted.Authorization, "[redacted]");
	assert.equal(redacted["Set-Cookie"], "[redacted]");
	assert.equal(redacted["content-type"], "application/json");
});

test("redactLedgerEntry: leaves method/path/status/ids untouched", () => {
	const entry = {
		method: "POST",
		path: "/api/v1/outcomes/o1/revisions",
		status: 200,
		requestHeaders: { Authorization: "Bearer x" },
	};
	const redacted = redactLedgerEntry(entry);
	assert.equal(redacted.method, "POST");
	assert.equal(redacted.path, "/api/v1/outcomes/o1/revisions");
	assert.equal(redacted.status, 200);
	assert.equal(redacted.requestHeaders.Authorization, "[redacted]");
});

test("startStep/finishStep: computes duration from the injected clock", () => {
	let now = "2026-09-16T00:00:00.000Z";
	const step = startStep("register-project", () => now);
	now = "2026-09-16T00:00:02.500Z";
	const finished = finishStep(step, { result: "passed" }, () => now);
	assert.equal(finished.durationMs, 2500);
	assert.equal(finished.result, "passed");
	assert.equal(finished.failureCode, null);
});

test("validateManifest: a complete manifest is valid", () => {
	const { valid, errors } = validateManifest(baseManifest());
	assert.deepEqual(errors, []);
	assert.equal(valid, true);
});

test("validateManifest: rejects a wrong schema version", () => {
	// createManifest always stamps SCHEMA_VERSION itself and ignores an
	// override, so a mismatched version is built by hand here — simulating a
	// hand-tampered manifest or one from a stale harness build, which is
	// exactly the case validateManifest must catch.
	const manifest = { ...baseManifest(), schemaVersion: "0.9.0" };
	const { valid, errors } = validateManifest(manifest);
	assert.equal(valid, false);
	assert.ok(errors.some((e) => e.includes("schemaVersion")));
	assert.equal(SCHEMA_VERSION, "1.0.0");
});

test("validateManifest: rejects executionStarted: true unconditionally", () => {
	const { valid, errors } = validateManifest(baseManifest({ executionStarted: true }));
	assert.equal(valid, false);
	assert.ok(errors.some((e) => e.includes("executionStarted")));
});

test("validateManifest: rejects an empty artifacts array (missing evidence is a failed proof)", () => {
	const { valid, errors } = validateManifest(baseManifest({ artifacts: [] }));
	assert.equal(valid, false);
	assert.ok(errors.some((e) => e.includes("artifacts")));
});

test("validateManifest: rejects a nonempty shortcutEndpointsObserved (amendment 4)", () => {
	const manifest = baseManifest({
		networkLedger: { shortcutEndpointsObserved: ["POST /api/v1/outcomes/o1/plans"] },
	});
	const { valid, errors } = validateManifest(manifest);
	assert.equal(valid, false);
	assert.ok(errors.some((e) => e.includes("shortcutEndpointsObserved")));
});

test("validateManifest: rejects build.identityMatch !== true (amendment 3)", () => {
	const manifest = baseManifest({ build: { gitSha: "abc123", identityMatch: false } });
	const { valid, errors } = validateManifest(manifest);
	assert.equal(valid, false);
	assert.ok(errors.some((e) => e.includes("identityMatch")));
});

test("validateManifest: reports every missing required field, not just the first", () => {
	const { valid, errors } = validateManifest({});
	assert.equal(valid, false);
	assert.ok(errors.length > 5);
});
