import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { createFixtureRepo, readFixtureCommitSha, removeFixtureRepo } from "./fixture-repo.mjs";

test("createFixtureRepo: produces a git repo with a README and one commit", () => {
	const root = mkdtempSync(join(tmpdir(), "kennel-fixture-repo-test-"));
	const dir = join(root, "fixture");
	try {
		const { path, initialCommitSha } = createFixtureRepo(dir);
		assert.equal(path, dir);
		assert.match(initialCommitSha, /^[0-9a-f]{40}$/);
		assert.ok(existsSync(join(dir, ".git")));
		assert.ok(existsSync(join(dir, "README.md")));
		assert.match(readFileSync(join(dir, "README.md"), "utf8"), /Outcome journey fixture repository/);
		assert.equal(readFixtureCommitSha(dir), initialCommitSha);
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("createFixtureRepo: the initial commit SHA is reproducible across two independent fixtures", () => {
	const root = mkdtempSync(join(tmpdir(), "kennel-fixture-repo-test-"));
	try {
		const first = createFixtureRepo(join(root, "one"));
		const second = createFixtureRepo(join(root, "two"));
		// Same content, same fixed author/committer identity and date -> the
		// git object hash is identical regardless of the containing directory.
		assert.equal(first.initialCommitSha, second.initialCommitSha);
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("createFixtureRepo: refuses to reuse an already-existing directory", () => {
	const root = mkdtempSync(join(tmpdir(), "kennel-fixture-repo-test-"));
	const dir = join(root, "fixture");
	try {
		createFixtureRepo(dir);
		assert.throws(() => createFixtureRepo(dir));
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

test("removeFixtureRepo: deletes the fixture directory entirely", () => {
	const root = mkdtempSync(join(tmpdir(), "kennel-fixture-repo-test-"));
	const dir = join(root, "fixture");
	try {
		createFixtureRepo(dir);
		removeFixtureRepo(dir);
		assert.equal(existsSync(dir), false);
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});
