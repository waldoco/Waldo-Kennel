#!/usr/bin/env node
// Packaged macOS Outcome journey harness — orchestration entry point.
//
//   node scripts/outcome-journey/run-outcome-journey.mjs [--app <path/to/Kennel.app>] [--work-dir <dir>]
//
// Owner-run only (docs/handoffs/2026-09-16-macos-outcome-journey-harness-plan.md
// §0.8 item 4 / amendment 7): this drives a live, already-authenticated local
// Codex CLI through the real packaged app. CI never runs this — it only runs
// this file's own pure-function unit tests and the schema/typecheck gates.
//
// Shape mirrors frontend/scripts/e2e-mac-update.mjs: dependency-free ESM, pure
// helpers exported and unit-tested, side effects confined to run()/main().
//
// What this script does:
//  1. Preconditions: macOS host, an authenticated Codex CLI on PATH, no stale
//     Kennel processes, no reused profile.
//  2. Packages the app at the current commit (or accepts an already-packaged
//     --app), then gates on frontend/scripts/assert-package-identity.mjs.
//  3. Creates an isolated profile + disposable git fixture repo.
//  4. Runs the Playwright journey spec (test/macos-outcome-journey), which
//     drives the real UI end to end through the pre-execution boundary and
//     writes a journey-result.json fragment into the evidence directory.
//  5. Assembles manifest.json + ux-flaws.json/.md from that fragment plus the
//     build/profile/fixture facts collected here, validates the manifest, and
//     exits nonzero on anything but "passed".

import { execFileSync, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { createFixtureRepo } from "./fixture-repo.mjs";
import {
	createManifest,
	deriveJourneyOutcome,
	finishStep,
	isoNow,
	startStep,
	validateManifest,
} from "./manifest.mjs";
import { checkBuildIdentity, findStaleKennelProcesses } from "./process-guard.mjs";
import { ORCHESTRATOR_EXTRA_MARGIN_MS, TEARDOWN_GRACE_MS } from "./timeouts.mjs";
import { renderMarkdown, toJSON as flawsToJSON } from "./ux-flaws.mjs";

const scriptDir = dirname(fileURLToPath(import.meta.url));
export const REPO_ROOT = resolve(scriptDir, "..", "..");
export const FRONTEND_ROOT = join(REPO_ROOT, "frontend");
export const PLAYWRIGHT_CONFIG = join(REPO_ROOT, "test", "macos-outcome-journey", "playwright.config.ts");

const DEFAULTS = {
	codexPreflightMs: 30_000,
	intakeAnalysisMs: 180_000,
	planningProviderMs: 180_000,
	overallJourneyMs: 10 * 60_000,
};

class UsageError extends Error {}

/** Pure CLI argument parsing — exported and unit tested without touching disk/process. */
export function parseArgs(argv) {
	const opts = { ...DEFAULTS };
	for (let i = 0; i < argv.length; i += 1) {
		const flag = argv[i];
		const value = argv[i + 1];
		const needsValue = () => {
			if (value === undefined || value.startsWith("--")) throw new UsageError(`${flag} needs a value`);
			i += 1;
			return value;
		};
		switch (flag) {
			case "--app":
				opts.app = needsValue();
				break;
			case "--work-dir":
				opts.workDir = needsValue();
				break;
			case "--codex-preflight-timeout":
				opts.codexPreflightMs = positiveMs(flag, needsValue());
				break;
			case "--intake-analysis-timeout":
				opts.intakeAnalysisMs = positiveMs(flag, needsValue());
				break;
			case "--planning-provider-timeout":
				opts.planningProviderMs = positiveMs(flag, needsValue());
				break;
			case "--overall-timeout":
				opts.overallJourneyMs = positiveMs(flag, needsValue());
				break;
			default:
				throw new UsageError(`unknown flag: ${flag}`);
		}
	}
	return opts;
}

function positiveMs(flag, raw) {
	const seconds = Number(raw);
	if (!Number.isFinite(seconds) || seconds <= 0) throw new UsageError(`${flag} must be a positive number of seconds`);
	return seconds * 1000;
}

function hashFile(path) {
	return createHash("sha256").update(readFileSync(path)).digest("hex");
}

/** Locates the packaged .app under frontend/out/Kennel-darwin-*, matching assert-package-identity.mjs's own resolution. */
export function resolvePackagedApp(frontendRoot) {
	const outDir = join(frontendRoot, "out");
	if (!existsSync(outDir)) return null;
	const matches = readdirSync(outDir)
		.filter((name) => name.startsWith("Kennel-darwin-"))
		.map((name) => join(outDir, name, "Kennel.app"))
		.filter(existsSync);
	return matches.length === 1 ? matches[0] : null;
}

function readExecutableName(appPath) {
	return execFileSync("/usr/bin/plutil", ["-extract", "CFBundleExecutable", "raw", "-o", "-", join(appPath, "Contents", "Info.plist")], {
		encoding: "utf8",
	}).trim();
}

function readBundleVersion(appPath) {
	return execFileSync("/usr/bin/plutil", ["-extract", "CFBundleShortVersionString", "raw", "-o", "-", join(appPath, "Contents", "Info.plist")], {
		encoding: "utf8",
	}).trim();
}

/** True if a Codex CLI is on PATH and answers --version within the preflight ceiling. Never throws. */
export function probeCodexAvailable(timeoutMs) {
	const result = spawnSync("codex", ["--version"], { encoding: "utf8", timeout: timeoutMs });
	return result.status === 0;
}

function listKennelProcesses() {
	const result = spawnSync("ps", ["-A", "-o", "pid=,command="], { encoding: "utf8" });
	return result.status === 0 ? result.stdout : "";
}

/** Seeds an update-settings.json exactly matching frontend/scripts/e2e-mac-update.mjs's shape, so the first-run update dialog never blocks a non-interactive launch. */
function seedUpdateSettings(settingsDir) {
	mkdirSync(settingsDir, { recursive: true });
	const settings = { enabled: false, channel: "latest", nightlyAck: false, feature: null };
	writeFileSync(join(settingsDir, "update-settings.json"), `${JSON.stringify(settings, null, 2)}\n`, { mode: 0o600 });
}

async function main(argv) {
	let opts;
	try {
		opts = parseArgs(argv.slice(2));
	} catch (err) {
		process.stderr.write(`${err.message}\n`);
		process.stderr.write(
			"usage: node run-outcome-journey.mjs [--app <path>] [--work-dir <dir>] [--codex-preflight-timeout s] [--intake-analysis-timeout s] [--planning-provider-timeout s] [--overall-timeout s]\n",
		);
		process.exit(2);
	}
	const exitCode = await run(opts);
	process.exit(exitCode);
}

async function run(opts) {
	const runId = `${Date.now().toString(36)}-${process.pid}`;
	const startedAt = isoNow();
	const steps = [];
	let blocked = false;
	let failed = false;
	const limitations = [];

	function step(name, fn) {
		const record = startStep(name);
		try {
			const result = fn();
			steps.push(finishStep(record, { result: "passed" }));
			return result;
		} catch (err) {
			steps.push(finishStep(record, { result: err.blocked ? "blocked" : "failed", failureCode: err.code ?? "error" }));
			if (err.blocked) blocked = true;
			else failed = true;
			throw err;
		}
	}

	function blockedError(code, message) {
		const err = new Error(message);
		err.blocked = true;
		err.code = code;
		return err;
	}

	let manifest = null;
	let evidenceDir = null;

	try {
		// --- Preconditions (§3.3) ---
		step("macos-host-check", () => {
			if (process.platform !== "darwin") throw blockedError("wrong_platform", `macOS required, host is ${process.platform}`);
		});

		const gitSha = step("record-git-sha", () =>
			execFileSync("git", ["rev-parse", "HEAD"], { cwd: REPO_ROOT, encoding: "utf8" }).trim(),
		);
		const gitDirty = step("record-git-dirty", () =>
			execFileSync("git", ["status", "--porcelain"], { cwd: REPO_ROOT, encoding: "utf8" }).trim().length > 0,
		);

		// Cheap, fast-fail PATH sanity check only (Instinct review item 7):
		// `codex --version` proves the binary is on PATH, nothing about
		// authentication. It exists to avoid spending 10+ minutes packaging an
		// app when Codex isn't even installed. The real authentication gate
		// runs inside the Playwright spec, against the daemon's own agent
		// inventory, after the daemon is up and before any UI action.
		step("codex-path-sanity-check", () => {
			if (!probeCodexAvailable(opts.codexPreflightMs)) {
				throw blockedError("codex_unavailable", "No Codex CLI found on PATH within the preflight ceiling");
			}
		});

		step("stale-process-check", () => {
			const stale = findStaleKennelProcesses(listKennelProcesses(), { ownedPids: [] });
			if (stale.length > 0) {
				throw blockedError("stale_process", `Stale Kennel process(es) found: ${stale.map((p) => p.pid).join(", ")}`);
			}
		});

		// --- Build/package (§3.1) ---
		let appPath = opts.app;
		if (!appPath) {
			step("build-daemon", () => {
				const r = spawnSync("npm", ["run", "build:daemon"], { cwd: FRONTEND_ROOT, stdio: "inherit" });
				if (r.status !== 0) throw new Error("npm run build:daemon failed");
			});
			step("package-app", () => {
				const r = spawnSync("npm", ["run", "package"], { cwd: FRONTEND_ROOT, stdio: "inherit" });
				if (r.status !== 0) throw new Error("npm run package failed");
			});
			appPath = step("resolve-packaged-app", () => {
				const resolved = resolvePackagedApp(FRONTEND_ROOT);
				if (!resolved) throw new Error("expected exactly one packaged Kennel.app under frontend/out/Kennel-darwin-*");
				return resolved;
			});
		}

		// Fail-closed (Instinct review item 3a): a mismatched identity aborts
		// HERE, before any fixture/profile/Playwright work — it must never fall
		// through to "continue with a failed flag set", which risks the rest of
		// the run proceeding against a build that is not what it claims to be.
		const packageIdentityCheck = step("package-identity-gate", () => {
			const r = spawnSync("node", [join(FRONTEND_ROOT, "scripts", "assert-package-identity.mjs"), appPath], { encoding: "utf8" });
			if (r.status !== 0) {
				throw new Error(`package-identity gate failed for ${appPath}: ${(r.stderr || r.stdout || "").trim()}`);
			}
			return "passed";
		});

		const executableName = step("read-executable-name", () => readExecutableName(appPath));
		const executablePath = join(appPath, "Contents", "MacOS", executableName);
		const executableSha256 = step("hash-executable", () => hashFile(executablePath));
		const daemonPath = join(appPath, "Contents", "Resources", "daemon", "kennel");
		const daemonSha256 = existsSync(daemonPath) ? step("hash-daemon", () => hashFile(daemonPath)) : null;
		const appVersion = step("read-app-version", () => readBundleVersion(appPath));

		// --- Isolated profile + fixture (§3.2, §3.4) ---
		const workRoot = opts.workDir ?? mkdtempSync(join(tmpdir(), "kennel-outcome-journey-"));
		mkdirSync(workRoot, { recursive: true });
		evidenceDir = join(workRoot, "evidence");
		mkdirSync(evidenceDir, { recursive: true });
		const profileDir = join(workRoot, "profile");
		const runFile = join(profileDir, "running.json");
		mkdirSync(profileDir, { recursive: true });
		seedUpdateSettings(profileDir);
		const fixtureDir = join(workRoot, "fixture-repo");
		const fixture = step("create-fixture-repo", () => createFixtureRepo(fixtureDir));

		// --- Drive the real journey via Playwright ---
		step("run-playwright-journey", () => {
			const env = {
				...process.env,
				// test/macos-outcome-journey/ is not under frontend/ (§2's testDir-
				// collision fix), so Node's own upward node_modules walk from the
				// config file's location never finds @playwright/test — the only
				// package.json that declares it is frontend/package.json. NODE_PATH
				// is what makes that resolvable without a second install.
				NODE_PATH: join(FRONTEND_ROOT, "node_modules"),
				KENNEL_JOURNEY_APP_PATH: appPath,
				KENNEL_JOURNEY_PROFILE_DIR: profileDir,
				KENNEL_JOURNEY_RUN_FILE: runFile,
				KENNEL_JOURNEY_FIXTURE_PATH: fixture.path,
				KENNEL_JOURNEY_FIXTURE_SHA: fixture.initialCommitSha,
				KENNEL_JOURNEY_EVIDENCE_DIR: evidenceDir,
				KENNEL_JOURNEY_RECORDED_GIT_SHA: gitSha,
				KENNEL_JOURNEY_RUN_ID: runId,
				KENNEL_JOURNEY_INTAKE_ANALYSIS_TIMEOUT_MS: String(opts.intakeAnalysisMs),
				KENNEL_JOURNEY_PLANNING_PROVIDER_TIMEOUT_MS: String(opts.planningProviderMs),
				KENNEL_JOURNEY_OVERALL_TIMEOUT_MS: String(opts.overallJourneyMs),
			};
			// The playwright binary lives under frontend/node_modules (the only
			// package.json in this repo that declares @playwright/test) — spawned
			// directly rather than via `npx`, which would otherwise try to resolve
			// or install a package from the npm registry when run from REPO_ROOT.
			const playwrightBin = join(FRONTEND_ROOT, "node_modules", ".bin", "playwright");
			if (!existsSync(playwrightBin)) {
				throw new Error(`playwright binary not found at ${playwrightBin} — run \`npm install\` under frontend/`);
			}
			// Review round 2 item 4: this MUST exceed the spec's own outer
			// test.setTimeout (overallJourneyMs + TEARDOWN_GRACE_MS), or the
			// orchestrator can kill the child mid-teardown/evidence-write before
			// the spec's own grace window closes. Both sides import
			// TEARDOWN_GRACE_MS from the same module so they cannot silently drift
			// apart again; ORCHESTRATOR_EXTRA_MARGIN_MS covers process spawn/
			// signal-delivery overhead on top of that.
			const r = spawnSync(playwrightBin, ["test", "-c", PLAYWRIGHT_CONFIG], {
				cwd: REPO_ROOT,
				env,
				stdio: "inherit",
				timeout: opts.overallJourneyMs + TEARDOWN_GRACE_MS + ORCHESTRATOR_EXTRA_MARGIN_MS,
			});
			// A nonzero Playwright exit is not itself the manifest's classification —
			// journey-result.json (written by the spec even on failure/timeout) is
			// the source of truth for pass/fail/blocked/ambiguous. A nonzero exit
			// with no fragment at all means the spec crashed before writing
			// anything, which the fragment-read step below turns into `failed`.
			return r.status;
		});

		const fragmentPath = join(evidenceDir, "journey-result.json");
		const fragment = existsSync(fragmentPath)
			? JSON.parse(readFileSync(fragmentPath, "utf8"))
			: {
					result: "failed",
					steps: [],
					identities: {},
					apiAssertions: [],
					networkLedger: { shortcutEndpointsObserved: [] },
					facetsCheck: { step: 8, identical: false },
					artifacts: [],
					cleanup: { daemonPidObservedGone: false, retained: [] },
					executionStarted: false,
					boundaryReached: "unknown — no journey-result.json was written",
					limitations: ["Playwright spec produced no evidence fragment; treat as a failed proof."],
					unverifiedClaims: [],
				};

		if (fragment.result === "blocked") blocked = true;
		if (fragment.result === "failed") failed = true;
		const ambiguous = fragment.result === "ambiguous";

		const identityMatch = checkBuildIdentity({
			recordedGitSha: gitSha,
			liveBuildRevision: fragment.identities?.daemonBuildRevision ?? "unknown",
		}).matches;
		if (!identityMatch) failed = true;

		manifest = createManifest({
			runId,
			startedAt,
			completedAt: isoNow(),
			result: deriveJourneyOutcome({ blocked, ambiguous, failed }).result,
			build: {
				gitSha,
				gitBranch: execFileSync("git", ["rev-parse", "--abbrev-ref", "HEAD"], { cwd: REPO_ROOT, encoding: "utf8" }).trim(),
				gitDirty,
				appVersion,
				bundleId: "in.heywaldo.kennel",
				executablePath,
				executableSha256,
				daemonSha256,
				daemonBuildRevision: fragment.identities?.daemonBuildRevision ?? "unknown",
				identityMatch,
				signingIdentity: process.env.APPLE_SIGNING_IDENTITY ? process.env.APPLE_SIGNING_IDENTITY : "unsigned",
				signingVerified: false,
				macosVersion: execFileSync("sw_vers", ["-productVersion"], { encoding: "utf8" }).trim(),
				arch: process.arch,
				packageIdentityCheck,
			},
			profile: { profileDir, dataDir: join(profileDir, "data"), runFile, port: fragment.identities?.port ?? null },
			fixture: { repoPath: fixture.path, initialCommitSha: fixture.initialCommitSha },
			steps: [...steps, ...(fragment.steps ?? [])],
			identities: fragment.identities ?? {},
			apiAssertions: fragment.apiAssertions ?? [],
			networkLedger: fragment.networkLedger ?? { shortcutEndpointsObserved: [] },
			facetsCheck: fragment.facetsCheck ?? { step: 8, identical: false },
			artifacts: fragment.artifacts ?? [],
			cleanup: fragment.cleanup ?? { daemonPidObservedGone: false, retained: [] },
			executionStarted: false,
			boundaryReached: fragment.boundaryReached ?? "unknown",
			limitations: [...limitations, ...(fragment.limitations ?? [])],
			unverifiedClaims: fragment.unverifiedClaims ?? [],
		});
	} catch (err) {
		failed = failed || !blocked;
		limitations.push(`Orchestration aborted: ${err.message}`);
		manifest = createManifest({
			runId,
			startedAt,
			completedAt: isoNow(),
			result: deriveJourneyOutcome({ blocked, failed: !blocked }).result,
			build: { identityMatch: false },
			profile: {},
			fixture: {},
			steps,
			identities: {},
			apiAssertions: [],
			networkLedger: { shortcutEndpointsObserved: [] },
			facetsCheck: { step: 8, identical: false },
			artifacts: [],
			cleanup: { daemonPidObservedGone: false, retained: [] },
			executionStarted: false,
			boundaryReached: "aborted before reaching a checkpoint",
			limitations,
			unverifiedClaims: [],
		});
	}

	const { valid, errors } = validateManifest(manifest);
	if (!valid) {
		failed = true;
		manifest.unverifiedClaims = [...manifest.unverifiedClaims, ...errors];
		manifest.result = deriveJourneyOutcome({ blocked, failed: true }).result;
	}

	if (evidenceDir) {
		mkdirSync(evidenceDir, { recursive: true });
		writeFileSync(join(evidenceDir, "manifest.json"), `${JSON.stringify(manifest, null, 2)}\n`);
		const flawsPath = join(evidenceDir, "ux-flaws");
		const flawsFragmentPath = join(evidenceDir, "journey-result.json");
		const flaws = existsSync(flawsFragmentPath)
			? (JSON.parse(readFileSync(flawsFragmentPath, "utf8")).findings ?? [])
			: [];
		writeFileSync(`${flawsPath}.json`, `${JSON.stringify(flawsToJSON(flaws), null, 2)}\n`);
		writeFileSync(`${flawsPath}.md`, `${renderMarkdown(flaws)}\n`);
	}

	console.log(`KENNEL_OUTCOME_JOURNEY_RESULT ${JSON.stringify({ result: manifest.result, runId, evidenceDir })}`);
	return deriveJourneyOutcome({ blocked, ambiguous: manifest.result === "ambiguous", failed }).exitCode;
}

export { run };

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
	main(process.argv).catch((err) => {
		process.stderr.write(`${err.stack || err}\n`);
		process.exit(1);
	});
}
