// Disposable local git fixture for the packaged macOS Outcome journey harness.
//
// The fixture is a minimal, deterministic, local, safe-to-delete repository —
// never the owner's real Kennel project. Author/committer identity and dates
// are fixed so the initial commit SHA is byte-identical across runs on the
// same harness version, regardless of where on disk the fixture lands (git's
// object hash covers tree content and commit metadata, never the working
// directory path) — that reproducible SHA is what the manifest records and
// what a rerun (§11 gate 6) can compare against.

import { execFileSync } from "node:child_process";
import { mkdirSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const FIXTURE_AUTHOR_NAME = "Kennel Outcome Journey Harness";
const FIXTURE_AUTHOR_EMAIL = "outcome-journey-harness@kennel.invalid";
// Fixed, not "now" — this is what makes the initial commit SHA reproducible.
const FIXTURE_COMMIT_DATE = "2026-01-01T00:00:00Z";
const FIXTURE_COMMIT_MESSAGE = "Initial fixture commit (kennel outcome-journey harness)";

const README_CONTENTS = [
	"# Outcome journey fixture repository",
	"",
	"This repository exists only to prove the packaged macOS Outcome journey.",
	"It is disposable, local, and safe to delete.",
	"",
].join("\n");

const SOURCE_FILE_CONTENTS = [
	"// A trivial file so the fixture repository is not empty.",
	"export const FIXTURE_MARKER = \"kennel-outcome-journey-fixture\";",
	"",
].join("\n");

function git(args, cwd, extraEnv = {}) {
	return execFileSync("git", args, {
		cwd,
		encoding: "utf8",
		env: {
			...process.env,
			GIT_AUTHOR_NAME: FIXTURE_AUTHOR_NAME,
			GIT_AUTHOR_EMAIL: FIXTURE_AUTHOR_EMAIL,
			GIT_AUTHOR_DATE: FIXTURE_COMMIT_DATE,
			GIT_COMMITTER_NAME: FIXTURE_AUTHOR_NAME,
			GIT_COMMITTER_EMAIL: FIXTURE_AUTHOR_EMAIL,
			GIT_COMMITTER_DATE: FIXTURE_COMMIT_DATE,
			...extraEnv,
		},
	}).trim();
}

/**
 * Creates the disposable fixture repository at `dir` (which must not already
 * exist — a reused path would violate the "clean, deterministic setup"
 * requirement) and returns its path and initial commit SHA.
 *
 * @param {string} dir absolute path to create the fixture at
 * @returns {{ path: string, initialCommitSha: string }}
 */
export function createFixtureRepo(dir) {
	mkdirSync(dir, { recursive: false });
	git(["init", "--initial-branch=main", "--quiet"], dir);
	// Disable any global/system git config the host machine might have that
	// would otherwise leak into the fixture's identity or signing behavior.
	git(["config", "user.name", FIXTURE_AUTHOR_NAME], dir);
	git(["config", "user.email", FIXTURE_AUTHOR_EMAIL], dir);
	git(["config", "commit.gpgsign", "false"], dir);
	writeFileSync(join(dir, "README.md"), README_CONTENTS);
	writeFileSync(join(dir, "fixture.mjs"), SOURCE_FILE_CONTENTS);
	git(["add", "README.md", "fixture.mjs"], dir);
	git(["commit", "--quiet", "--message", FIXTURE_COMMIT_MESSAGE], dir);
	return { path: dir, initialCommitSha: readFixtureCommitSha(dir) };
}

/** Reads the fixture's current HEAD commit SHA. */
export function readFixtureCommitSha(dir) {
	return git(["rev-parse", "HEAD"], dir);
}

/** Permanently deletes the fixture repository. Only ever called on the harness's own temp path. */
export function removeFixtureRepo(dir) {
	rmSync(dir, { recursive: true, force: true });
}
