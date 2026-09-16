import { test } from "node:test";
import assert from "node:assert/strict";

import {
	assertFreshProfile,
	checkBuildIdentity,
	findStaleKennelProcesses,
	isPidGone,
	parsePsListing,
} from "./process-guard.mjs";

const SAMPLE_PS = [
	"501 /Applications/Kennel.app/Contents/MacOS/kennel",
	"502 /usr/sbin/some-other-daemon",
	"12345 /private/tmp/kennel-outcome-journey/Kennel.app/Contents/MacOS/kennel",
].join("\n");

test("parsePsListing: parses pid/command pairs and skips blank lines", () => {
	const rows = parsePsListing(`\n${SAMPLE_PS}\n\n`);
	assert.equal(rows.length, 3);
	assert.equal(rows[0].pid, 501);
	assert.equal(rows[0].command, "/Applications/Kennel.app/Contents/MacOS/kennel");
});

test("findStaleKennelProcesses: flags a Kennel process not owned by this run", () => {
	const stale = findStaleKennelProcesses(SAMPLE_PS, { ownedPids: [12345] });
	assert.equal(stale.length, 1);
	assert.equal(stale[0].pid, 501);
});

test("findStaleKennelProcesses: an empty listing (nothing owned) reports no stale process", () => {
	const stale = findStaleKennelProcesses("501 /Applications/Kennel.app/Contents/MacOS/kennel", { ownedPids: [501] });
	assert.equal(stale.length, 0);
});

test("findStaleKennelProcesses: does not misclassify an unrelated process containing 'kennel'", () => {
	const stale = findStaleKennelProcesses("999 /usr/local/bin/kennel-cli-unrelated", { ownedPids: [] });
	assert.equal(stale.length, 0);
});

test("findStaleKennelProcesses: matches on an explicit executablePath when given", () => {
	const stale = findStaleKennelProcesses(SAMPLE_PS, {
		ownedPids: [],
		executablePath: "/private/tmp/kennel-outcome-journey/Kennel.app/Contents/MacOS/kennel",
	});
	assert.equal(stale.length, 1);
	assert.equal(stale[0].pid, 12345);
});

test("assertFreshProfile: rejects the owner's real ~/.kennel", () => {
	const result = assertFreshProfile({ profileDir: "/Users/owner/.kennel", homeKennelDir: "/Users/owner/.kennel" });
	assert.equal(result.fresh, false);
	assert.match(result.reason, /real ~\/\.kennel/);
});

test("assertFreshProfile: rejects a data dir that already existed", () => {
	const result = assertFreshProfile({ dataDirExistedBeforeLaunch: true });
	assert.equal(result.fresh, false);
	assert.match(result.reason, /already existed/);
});

test("assertFreshProfile: accepts a genuinely fresh, non-home profile dir", () => {
	const result = assertFreshProfile({
		profileDir: "/tmp/kennel-outcome-journey-run1",
		homeKennelDir: "/Users/owner/.kennel",
		dataDirExistedBeforeLaunch: false,
	});
	assert.equal(result.fresh, true);
});

test("checkBuildIdentity: matches when the recorded SHA equals the live buildRevision", () => {
	const result = checkBuildIdentity({ recordedGitSha: "abc123", liveBuildRevision: "abc123" });
	assert.equal(result.matches, true);
});

test("checkBuildIdentity: flags a mismatch (stale build or wrong tree)", () => {
	const result = checkBuildIdentity({ recordedGitSha: "abc123", liveBuildRevision: "def456" });
	assert.equal(result.matches, false);
});

test("isPidGone: true once the pid no longer appears", () => {
	assert.equal(isPidGone("999 /some/other/process", 12345), true);
	assert.equal(isPidGone(SAMPLE_PS, 12345), false);
});
