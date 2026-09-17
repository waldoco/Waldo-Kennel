// Process ownership, identity, and profile-freshness detection for the
// packaged macOS Outcome journey harness.
//
// Pure functions only: every check here takes already-collected facts (a `ps`
// listing, a recorded SHA, a live daemon response) and returns a verdict. The
// orchestration script is responsible for actually collecting those facts
// (spawning `ps`, calling `/readyz`) — keeping the decision logic pure is what
// makes "does this classify as a stale process" testable without a real Mac.

/**
 * Parses `ps -A -o pid=,command=` output into { pid, command } rows. Pure
 * string parsing so a fixture `ps` listing can exercise it without a real
 * process table.
 */
export function parsePsListing(psOutput) {
	return psOutput
		.split("\n")
		.map((line) => line.trim())
		.filter(Boolean)
		.map((line) => {
			const match = /^(\d+)\s+(.*)$/.exec(line);
			if (!match) return null;
			return { pid: Number(match[1]), command: match[2] };
		})
		.filter((row) => row !== null);
}

/**
 * Finds Kennel processes in a `ps` listing that are not among this run's own
 * spawned pids. Matches on the packaged bundle's executable path or the
 * "Kennel.app" bundle name, not a bare "kennel" substring — a stray directory
 * or unrelated process named "kennel-something" must not be misclassified as
 * a stale instance of the app under test.
 *
 * @param {string} psOutput raw `ps -A -o pid=,command=` output
 * @param {{ ownedPids?: number[], executablePath?: string }} opts
 * @returns {{ pid: number, command: string }[]} stale entries, if any
 */
export function findStaleKennelProcesses(psOutput, { ownedPids = [], executablePath } = {}) {
	const owned = new Set(ownedPids);
	const pattern = executablePath
		? new RegExp(escapeForRegExp(executablePath))
		: /Kennel\.app\/Contents\/MacOS\/kennel\b/;
	return parsePsListing(psOutput).filter((row) => !owned.has(row.pid) && pattern.test(row.command));
}

function escapeForRegExp(value) {
	return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/**
 * Asserts a profile directory is genuinely fresh for this run — never a
 * reused path from a previous run or the owner's real `~/.kennel`.
 * @returns {{ fresh: boolean, reason?: string }}
 */
export function assertFreshProfile({ dataDirExistedBeforeLaunch, profileDir, homeKennelDir }) {
	if (profileDir && homeKennelDir && profileDir === homeKennelDir) {
		return { fresh: false, reason: "profile directory must never be the owner's real ~/.kennel" };
	}
	if (dataDirExistedBeforeLaunch) {
		return { fresh: false, reason: "KENNEL_DATA_DIR already existed before this run's first launch" };
	}
	return { fresh: true };
}

/**
 * Compares the recorded pre-package git SHA against the daemon's live
 * `buildRevision` (from `/readyz`, see docs/handoffs/2026-09-16-macos-outcome-
 * journey-harness-plan.md §3.1 amendment 3). A mismatch means the packaged
 * build under test does not correspond to the tree the harness read, and the
 * whole run must abort before any UI action — this is the specific class of
 * error a stale build/tree mismatch produces.
 * @returns {{ matches: boolean, recordedGitSha: string, liveBuildRevision: string }}
 */
export function checkBuildIdentity({ recordedGitSha, liveBuildRevision }) {
	return { matches: recordedGitSha === liveBuildRevision, recordedGitSha, liveBuildRevision };
}

/** True once a previously-observed pid no longer appears in a fresh `ps` listing. */
export function isPidGone(psOutput, pid) {
	return !parsePsListing(psOutput).some((row) => row.pid === pid);
}
