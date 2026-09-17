// Typed UX flaw catalog builder for the packaged macOS Outcome journey harness.
//
// Pure functions only, matching manifest.mjs's shape. `ux-flaws.json` and
// `ux-flaws.md` are a required deliverable of every run, pass or fail — this
// module is what makes that catalog typed and consistently ranked instead of
// free-form prose assembled by hand each time.

export const SEVERITIES = Object.freeze(["critical", "high", "medium", "low"]);
const SEVERITY_RANK = Object.freeze({ critical: 0, high: 1, medium: 2, low: 3 });

const REQUIRED_FIELDS = [
	"id",
	"severity",
	"category",
	"step",
	"observedBehavior",
	"whyItHarms",
	"expectedBehavior",
	"reproPath",
	"evidence",
	"buildSha",
	"reproducesConsistently",
	"proposedOwner",
	"fixedInThisSlice",
];

/**
 * Builds one typed finding record and validates it against the required shape
 * (docs/handoffs/2026-09-16-macos-outcome-journey-harness-plan.md §8).
 * Throws on a missing required field or an invalid severity — a malformed
 * finding is a harness bug, not something to silently drop.
 */
export function createFinding(fields) {
	const missing = REQUIRED_FIELDS.filter((key) => fields[key] === undefined);
	if (missing.length > 0) {
		throw new Error(`createFinding: missing required field(s): ${missing.join(", ")}`);
	}
	if (!SEVERITIES.includes(fields.severity)) {
		throw new Error(`createFinding: severity must be one of ${SEVERITIES.join("/")}, got ${fields.severity}`);
	}
	if (typeof fields.fixedInThisSlice !== "boolean") {
		throw new Error("createFinding: fixedInThisSlice must be a boolean");
	}
	return { ...fields };
}

/** Sorts findings most-severe first; stable for equal severities (input order preserved). */
export function rankFindings(findings) {
	return [...findings].sort((a, b) => SEVERITY_RANK[a.severity] - SEVERITY_RANK[b.severity]);
}

/** Groups findings by category, most-populous cluster first. */
export function clusterByCategory(findings) {
	const byCategory = new Map();
	for (const finding of findings) {
		const bucket = byCategory.get(finding.category) ?? [];
		bucket.push(finding);
		byCategory.set(finding.category, bucket);
	}
	return [...byCategory.entries()]
		.map(([category, items]) => ({ category, items: rankFindings(items) }))
		.sort((a, b) => b.items.length - a.items.length);
}

/** Counts findings by severity, always including every severity key (zero-filled). */
export function countBySeverity(findings) {
	const counts = Object.fromEntries(SEVERITIES.map((s) => [s, 0]));
	for (const finding of findings) counts[finding.severity] += 1;
	return counts;
}

/**
 * Renders the reviewer-facing ux-flaws.md content: a ranked table, the
 * required top-five synthesis, and category clusters — never a flat list.
 */
export function renderMarkdown(findings, { topFive = 5 } = {}) {
	const ranked = rankFindings(findings);
	const clusters = clusterByCategory(findings);
	const counts = countBySeverity(findings);
	const lines = [];
	lines.push("# UX flaw catalog");
	lines.push("");
	lines.push(
		`${findings.length} findings — critical: ${counts.critical}, high: ${counts.high}, medium: ${counts.medium}, low: ${counts.low}.`,
	);
	lines.push("");
	lines.push("## Ranked top five");
	lines.push("");
	if (ranked.length === 0) {
		lines.push("No findings recorded for this run.");
	} else {
		ranked.slice(0, topFive).forEach((finding, index) => {
			lines.push(`${index + 1}. **${finding.id}** (${finding.severity}, ${finding.category}) — ${finding.observedBehavior}`);
		});
	}
	lines.push("");
	lines.push("## Clusters");
	lines.push("");
	for (const cluster of clusters) {
		lines.push(`### ${cluster.category} (${cluster.items.length})`);
		lines.push("");
		for (const finding of cluster.items) {
			lines.push(`- **${finding.id}** [${finding.severity}] step ${finding.step}: ${finding.observedBehavior}`);
			lines.push(`  - Expected: ${finding.expectedBehavior}`);
			lines.push(`  - Repro: ${finding.reproPath}`);
			lines.push(`  - Evidence: ${Array.isArray(finding.evidence) ? finding.evidence.join(", ") : finding.evidence}`);
			lines.push(
				`  - Build ${finding.buildSha}, reproduces consistently: ${finding.reproducesConsistently}, proposed owner: ${finding.proposedOwner}, fixed in this slice: ${finding.fixedInThisSlice}`,
			);
		}
		lines.push("");
	}
	return lines.join("\n");
}

/** Serializes findings to the ux-flaws.json shape: ranked, not insertion-ordered. */
export function toJSON(findings) {
	return { schemaVersion: "1.0.0", findings: rankFindings(findings) };
}
