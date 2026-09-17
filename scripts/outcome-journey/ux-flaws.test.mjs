import { test } from "node:test";
import assert from "node:assert/strict";

import { clusterByCategory, countBySeverity, createFinding, rankFindings, renderMarkdown, toJSON } from "./ux-flaws.mjs";

function finding(overrides = {}) {
	return createFinding({
		id: "UX-01",
		severity: "medium",
		category: "dead end",
		step: 4,
		observedBehavior: "no offline analysis control",
		whyItHarms: "forces a live model dependency",
		expectedBehavior: "an owner-visible skip control",
		reproPath: "open Understand, submit a statement",
		evidence: ["screenshots/04-intake-analysis-waiting.png"],
		buildSha: "abc123",
		reproducesConsistently: true,
		proposedOwner: "multi-round intake",
		fixedInThisSlice: false,
		...overrides,
	});
}

test("createFinding: throws on a missing required field", () => {
	assert.throws(() => createFinding({ id: "UX-01" }), /missing required field/);
});

test("createFinding: throws on an invalid severity", () => {
	assert.throws(() => finding({ severity: "urgent" }), /severity must be one of/);
});

test("createFinding: throws when fixedInThisSlice is not a boolean", () => {
	assert.throws(() => finding({ fixedInThisSlice: "false" }), /fixedInThisSlice must be a boolean/);
});

test("rankFindings: orders critical > high > medium > low", () => {
	const findings = [finding({ id: "A", severity: "low" }), finding({ id: "B", severity: "critical" }), finding({ id: "C", severity: "medium" })];
	const ranked = rankFindings(findings);
	assert.deepEqual(ranked.map((f) => f.id), ["B", "C", "A"]);
});

test("clusterByCategory: groups by category, most-populous cluster first", () => {
	const findings = [
		finding({ id: "A", category: "selectors" }),
		finding({ id: "B", category: "copy" }),
		finding({ id: "C", category: "selectors" }),
	];
	const clusters = clusterByCategory(findings);
	assert.equal(clusters[0].category, "selectors");
	assert.equal(clusters[0].items.length, 2);
	assert.equal(clusters[1].category, "copy");
});

test("countBySeverity: zero-fills every severity even when absent", () => {
	const counts = countBySeverity([finding({ severity: "critical" })]);
	assert.deepEqual(counts, { critical: 1, high: 0, medium: 0, low: 0 });
});

test("renderMarkdown: includes a ranked top five and per-category clusters", () => {
	const findings = [finding({ id: "UX-01", severity: "critical" }), finding({ id: "UX-02", severity: "low" })];
	const markdown = renderMarkdown(findings);
	assert.match(markdown, /Ranked top five/);
	assert.match(markdown, /1\. \*\*UX-01\*\*/);
	assert.match(markdown, /Clusters/);
});

test("renderMarkdown: an empty catalog says so plainly rather than omitting the section", () => {
	const markdown = renderMarkdown([]);
	assert.match(markdown, /No findings recorded for this run\./);
});

test("toJSON: findings are ranked, not insertion-ordered", () => {
	const findings = [finding({ id: "A", severity: "low" }), finding({ id: "B", severity: "critical" })];
	const json = toJSON(findings);
	assert.deepEqual(json.findings.map((f) => f.id), ["B", "A"]);
});
