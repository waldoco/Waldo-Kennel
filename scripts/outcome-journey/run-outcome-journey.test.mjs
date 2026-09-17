// Unit tests for the pure parts of the orchestrator. The harness itself needs
// a real macOS host, a packaged build, and an authenticated Codex CLI, so it
// cannot run here (owner-run only, plan §0.8 item 4 / amendment 7). What IS
// testable anywhere is the flag contract and the packaged-app path
// resolution, mirroring frontend/scripts/e2e-mac-update.test.mjs's approach.

import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { parseArgs, resolvePackagedApp } from "./run-outcome-journey.mjs";

test("parseArgs: defaults every ceiling and accepts no flags", () => {
	const opts = parseArgs([]);
	assert.equal(opts.codexPreflightMs, 30_000);
	assert.equal(opts.intakeAnalysisMs, 180_000);
	assert.equal(opts.planningProviderMs, 180_000);
	assert.equal(opts.overallJourneyMs, 600_000);
	assert.equal(opts.app, undefined);
	assert.equal(opts.workDir, undefined);
});

test("parseArgs: accepts --app and --work-dir", () => {
	const opts = parseArgs(["--app", "/Applications/Kennel.app", "--work-dir", "/tmp/run1"]);
	assert.equal(opts.app, "/Applications/Kennel.app");
	assert.equal(opts.workDir, "/tmp/run1");
});

test("parseArgs: overrides a ceiling in seconds", () => {
	const opts = parseArgs(["--intake-analysis-timeout", "60"]);
	assert.equal(opts.intakeAnalysisMs, 60_000);
});

test("parseArgs: rejects an unknown flag", () => {
	assert.throws(() => parseArgs(["--bogus"]), /unknown flag/);
});

test("parseArgs: rejects a flag missing its value", () => {
	assert.throws(() => parseArgs(["--app"]), /needs a value/);
});

test("parseArgs: rejects a non-positive timeout", () => {
	assert.throws(() => parseArgs(["--overall-timeout", "0"]), /positive number of seconds/);
	assert.throws(() => parseArgs(["--overall-timeout", "-5"]), /positive number of seconds/);
});

test("resolvePackagedApp: returns null when out/ does not exist", () => {
	const root = mkdtempSync(join(tmpdir(), "kennel-journey-resolve-test-"));
	try {
		assert.equal(resolvePackagedApp(root), null);
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("resolvePackagedApp: finds exactly one Kennel-darwin-* bundle", () => {
	const root = mkdtempSync(join(tmpdir(), "kennel-journey-resolve-test-"));
	try {
		mkdirSync(join(root, "out", "Kennel-darwin-arm64", "Kennel.app"), { recursive: true });
		assert.equal(resolvePackagedApp(root), join(root, "out", "Kennel-darwin-arm64", "Kennel.app"));
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("resolvePackagedApp: returns null when more than one bundle is found (ambiguous)", () => {
	const root = mkdtempSync(join(tmpdir(), "kennel-journey-resolve-test-"));
	try {
		mkdirSync(join(root, "out", "Kennel-darwin-arm64", "Kennel.app"), { recursive: true });
		mkdirSync(join(root, "out", "Kennel-darwin-x64", "Kennel.app"), { recursive: true });
		assert.equal(resolvePackagedApp(root), null);
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});
