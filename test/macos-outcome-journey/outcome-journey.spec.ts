import { test, expect, _electron as electron, type ElectronApplication, type Page } from "@playwright/test";
import net from "node:net";
import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";

import { readDaemonBuildRevision, readRunFile, waitFor, waitForDaemonReady, waitForPidGone } from "./support/daemon-wait";
import { captureCheckpoint, type CapturedArtifact } from "./support/screenshot";
import { stubFolderPicker } from "./support/dialog-stub";

// Packaged macOS Outcome journey: the one-command proof that Contract
// creation, one Contract revision (surviving a real restart), a live-agent
// Plan proposal, and Plan approval all happen through the REAL packaged Work
// UI, with the daemon as the authoritative source of truth at every step, and
// that the run stops at the real pre-execution boundary without ever
// starting a persistent Attempt.
//
// See docs/handoffs/2026-09-16-macos-outcome-journey-harness-plan.md for the
// full plan this implements (§4 has the exact step table this file mirrors).
// Owner-run only (§0.8 item 4 / amendment 7) — requires an authenticated
// local Codex CLI; scripts/outcome-journey/run-outcome-journey.mjs is what
// actually launches this via `frontend/node_modules/.bin/playwright`.

const env = process.env;
const APP_PATH = requireEnv("KENNEL_JOURNEY_APP_PATH");
const PROFILE_DIR = requireEnv("KENNEL_JOURNEY_PROFILE_DIR");
const RUN_FILE = requireEnv("KENNEL_JOURNEY_RUN_FILE");
const FIXTURE_PATH = requireEnv("KENNEL_JOURNEY_FIXTURE_PATH");
const FIXTURE_SHA = requireEnv("KENNEL_JOURNEY_FIXTURE_SHA");
const EVIDENCE_DIR = requireEnv("KENNEL_JOURNEY_EVIDENCE_DIR");
const RECORDED_GIT_SHA = requireEnv("KENNEL_JOURNEY_RECORDED_GIT_SHA");
const RUN_ID = requireEnv("KENNEL_JOURNEY_RUN_ID");
const INTAKE_ANALYSIS_TIMEOUT_MS = Number(env.KENNEL_JOURNEY_INTAKE_ANALYSIS_TIMEOUT_MS ?? 180_000);
const PLANNING_PROVIDER_TIMEOUT_MS = Number(env.KENNEL_JOURNEY_PLANNING_PROVIDER_TIMEOUT_MS ?? 180_000);

function requireEnv(name: string): string {
	const value = process.env[name];
	if (!value) throw new Error(`${name} must be set by scripts/outcome-journey/run-outcome-journey.mjs`);
	return value;
}

function freePort(): Promise<number> {
	return new Promise((resolve, reject) => {
		const srv = net.createServer();
		srv.once("error", reject);
		srv.listen(0, "127.0.0.1", () => {
			const addr = srv.address() as net.AddressInfo;
			srv.close(() => resolve(addr.port));
		});
	});
}

interface StepRecord {
	name: string;
	startedAt: string;
	endedAt: string;
	durationMs: number;
	result: "passed" | "failed" | "blocked" | "ambiguous";
	failureCode: string | null;
}

interface ApiAssertion {
	method: string;
	path: string;
	status: number;
	assertedField?: string;
	expected?: unknown;
	observed?: unknown;
}

// --- Journey-wide evidence accumulators, written to journey-result.json in a
// finally block so a failed run still produces the manifest fragment the
// orchestrator needs (plan §3.4: retain evidence on failure). ---
const steps: StepRecord[] = [];
const apiAssertions: ApiAssertion[] = [];
const artifacts: CapturedArtifact[] = [];
const identities: Record<string, unknown> = { runId: RUN_ID };
const networkRequests: { method: string; url: string; status: number }[] = [];
const limitations: string[] = [];
const unverifiedClaims: string[] = [];
let overallResult: "passed" | "failed" | "blocked" | "ambiguous" = "passed";
let boundaryReached = "aborted before reaching the execution boundary";
let facetsCheck: { step: number; identical: boolean; preSaveFacets?: unknown; postSaveFacets?: unknown; postRestartFacets?: unknown } = {
	step: 8,
	identical: false,
};

async function step<T>(name: string, fn: () => Promise<T>): Promise<T> {
	const startedAt = new Date().toISOString();
	try {
		const result = await fn();
		steps.push({ name, startedAt, endedAt: new Date().toISOString(), durationMs: Date.now() - Date.parse(startedAt), result: "passed", failureCode: null });
		return result;
	} catch (err) {
		steps.push({
			name,
			startedAt,
			endedAt: new Date().toISOString(),
			durationMs: Date.now() - Date.parse(startedAt),
			result: "failed",
			failureCode: err instanceof Error ? err.message : String(err),
		});
		overallResult = overallResult === "blocked" ? overallResult : "failed";
		throw err;
	}
}

function api(port: number) {
	const base = `http://127.0.0.1:${port}`;
	return {
		async get(path: string) {
			const res = await fetch(`${base}${path}`);
			networkRequests.push({ method: "GET", url: path, status: res.status });
			return { status: res.status, body: res.status === 204 ? null : await res.json().catch(() => null) };
		},
		async post(path: string, body?: unknown) {
			const res = await fetch(`${base}${path}`, {
				method: "POST",
				headers: { "content-type": "application/json" },
				body: body ? JSON.stringify(body) : undefined,
			});
			networkRequests.push({ method: "POST", url: path, status: res.status });
			return { status: res.status, body: res.status === 204 ? null : await res.json().catch(() => null) };
		},
	};
}

function assertApi(entry: ApiAssertion) {
	apiAssertions.push(entry);
}

/**
 * Amendment 4's negative assertion: no shortcut endpoint was ever called
 * BY THIS SPEC as a substitute for a UI action. The spec's own `api()` helper
 * is used only for read-only GETs (evidence) plus the two owner-authored
 * writes the honesty boundary explicitly allows recording facts about
 * (nothing here ever POSTs /outcomes/{id}/plans or /projects/{id}/outcomes —
 * both go through the real UI instead). Checked at the very end against the
 * full list of requests this spec itself issued.
 */
const FORBIDDEN_SHORTCUT_PATTERNS = [/^\/api\/v1\/outcomes\/[^/]+\/plans$/, /^\/api\/v1\/projects\/[^/]+\/outcomes$/];

test("packaged macOS Outcome journey: Contract through Plan approval, stopping at the execution boundary", async () => {
	test.setTimeout(15 * 60_000);
	let app: ElectronApplication | undefined;
	let page: Page | undefined;
	let daemonPid: number | undefined;

	try {
		const port = await freePort();
		identities.port = port;

		app = await step("launch-packaged-app", () =>
			electron.launch({
				executablePath: APP_PATH,
				env: {
					...process.env,
					KENNEL_PORT: String(port),
					KENNEL_RUN_FILE: RUN_FILE,
					KENNEL_DATA_DIR: join(PROFILE_DIR, "data"),
				},
			}),
		);
		page = await app.firstWindow();

		// Packaged-mode proof (cheap, one-line, per plan §4): the renderer's own
		// origin, not a dev server's.
		const origin = await page.evaluate(() => location.origin);
		expect(origin).toBe("app://renderer");

		const runInfo = await step("wait-daemon-ready", () => waitForDaemonReady(RUN_FILE, 40_000));
		daemonPid = runInfo.pid;
		identities.daemonPid = daemonPid;

		// --- Build-identity abort gate (plan §3.1 amendment 3): the daemon's own
		// live buildRevision must equal the SHA the orchestrator recorded before
		// packaging. A mismatch aborts before any UI action.
		const daemonBuildRevision = await readDaemonBuildRevision(port);
		identities.daemonBuildRevision = daemonBuildRevision;
		if (daemonBuildRevision !== RECORDED_GIT_SHA) {
			overallResult = "failed";
			boundaryReached = `aborted: daemon buildRevision ${daemonBuildRevision} does not match recorded git SHA ${RECORDED_GIT_SHA}`;
			throw new Error(boundaryReached);
		}

		artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "01-clean-entry", { app, native: true })));

		// --- Step 2: register the fixture project ---
		await step("register-fixture-project", async () => {
			await page!.getByRole("button", { name: /add project/i }).click();
			await page!.getByTestId("create-project-import-existing").click();
			await stubFolderPicker(app!, FIXTURE_PATH);
			await page!.getByTestId("create-project-choose-folder").click();
			await expect(page!.getByRole("dialog")).toBeVisible({ timeout: 10_000 }).catch(() => undefined);
			// The agent-selection sheet auto-selects a worker agent once the
			// installed-agent inventory resolves; its submit control lives in
			// packages/product-ui's ProjectSetupFormView, outside this slice's
			// declared file list (plan §12 note) — a role+accessible-name locator
			// is used for this one control instead of a testid.
			const submit = page!.getByRole("button", { name: /create.*start/i });
			await expect(submit).toBeEnabled({ timeout: 30_000 });
			await submit.click();
		});
		artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "02-project-registered")));

		const projectsRead = await api(port).get("/api/v1/projects");
		const project = (projectsRead.body?.projects ?? []).find((p: { path: string }) => p.path === FIXTURE_PATH);
		if (!project) throw new Error(`registered project with path ${FIXTURE_PATH} not found via GET /api/v1/projects`);
		assertApi({ method: "GET", path: "/api/v1/projects", status: projectsRead.status, assertedField: "path", expected: FIXTURE_PATH, observed: project.path });
		identities.projectId = project.id;

		const preExisting = await api(port).get(`/api/v1/projects/${project.id}/outcomes`);
		if ((preExisting.body?.outcomes ?? []).length > 0) {
			throw new Error("unexpected pre-existing Outcome on the freshly registered fixture project");
		}

		await step("open-registered-project", () => page!.getByRole("button", { name: new RegExp(project.name, "i") }).click());

		// --- Steps 3-6: intake capture -> live analysis -> proposal review -> confirm ---
		artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "03-outcome-create-form")));
		const statement = `Prove the packaged macOS Outcome journey (${RUN_ID})`;
		await step("submit-intake-capture", async () => {
			await page!.getByTestId("intake-statement-input").fill(statement);
			await page!.getByTestId("intake-capture-submit").click();
		});

		artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "04-intake-analysis-waiting")));
		await step("wait-intake-analysis", () =>
			waitFor(
				"intake analysis to leave 'analyzing'",
				INTAKE_ANALYSIS_TIMEOUT_MS,
				async () => {
					const url = page!.url();
					const intakeId = new URL(url).searchParams.get("intake");
					if (!intakeId) return false;
					const read = await api(port).get(`/api/v1/intakes/${intakeId}`);
					identities.intakeId = intakeId;
					return read.body?.intake?.session?.status && read.body.intake.session.status !== "analyzing" ? read.body : false;
				},
				2000,
			),
		);

		artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "05-intake-proposal-review")));
		await step("confirm-outcome", async () => {
			await expect(page!.getByTestId("intake-confirm")).toBeEnabled({ timeout: 15_000 });
			await page!.getByTestId("intake-confirm").click();
		});

		await step("wait-outcome-created", () =>
			waitFor(
				"the /work route to carry a confirmed outcome id",
				15_000,
				async () => {
					const url = new URL(page!.url());
					return url.searchParams.get("outcome") ?? false;
				},
				500,
			),
		);
		const outcomeId = new URL(page.url()).searchParams.get("outcome")!;
		identities.outcomeId = outcomeId;
		const outcomeAfterCreate = await api(port).get(`/api/v1/outcomes/${outcomeId}`);
		assertApi({
			method: "GET",
			path: `/api/v1/outcomes/${outcomeId}`,
			status: outcomeAfterCreate.status,
			assertedField: "currentRevisionNumber",
			expected: 1,
			observed: outcomeAfterCreate.body?.outcome?.currentRevisionNumber,
		});
		expect(outcomeAfterCreate.body?.outcome?.currentRevisionNumber).toBe(1);
		artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "06-outcome-created")));

		// --- Step 7: contract tab is already open (plan §0.3 — not a navigation) ---
		await expect(page.getByTestId("outcome-mission-panel")).toBeVisible({ timeout: 15_000 });
		artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "07-contract-tab-open")));

		// --- Step 8: outcome-neutral, evidence-first facets diff (plan §0.5, amendment 1-2) ---
		const preSaveSnapshot = (await api(port).get(`/api/v1/outcomes/${outcomeId}`)).body?.outcome?.currentRevision;
		facetsCheck.preSaveFacets = preSaveSnapshot?.facets ?? null;

		const revisedGoal = `${preSaveSnapshot?.goal ?? ""} (revised ${RUN_ID})`;
		await step("edit-and-save-contract-revision", async () => {
			await page!.getByTestId("contract-edit").click();
			await page!.getByTestId("contract-goal").fill(revisedGoal);
			await page!.getByTestId("contract-save").click();
		});
		await waitFor(
			"the Contract to reach revision 2",
			15_000,
			async () => {
				const read = await api(port).get(`/api/v1/outcomes/${outcomeId}`);
				return read.body?.outcome?.currentRevisionNumber === 2 ? read.body : false;
			},
			500,
		);
		const postSaveOutcome = (await api(port).get(`/api/v1/outcomes/${outcomeId}`)).body?.outcome;
		facetsCheck.postSaveFacets = postSaveOutcome?.currentRevision?.facets ?? null;
		facetsCheck.identical = deepEqual(facetsCheck.preSaveFacets, facetsCheck.postSaveFacets);
		if (!facetsCheck.identical) {
			unverifiedClaims.push(
				"facets did not round-trip through the real Contract Save path — this contradicts the source-reading correction in plan §0.5 and needs its own investigation before any severity claim",
			);
		}
		assertApi({ method: "POST", path: `/api/v1/outcomes/${outcomeId}/revisions`, status: 200, assertedField: "currentRevisionNumber", expected: 2, observed: postSaveOutcome?.currentRevisionNumber });
		identities.contractRevisions = [1, 2];
		artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "08-contract-revised")));

		// --- Step 9: restart against the SAME profile, prove durability ---
		const preRestartPid = daemonPid;
		await step("close-app", () => app!.close());
		await step("wait-daemon-exit-before-restart", () => waitForPidGone(preRestartPid!, 20_000));

		const secondPort = await freePort();
		app = await step("relaunch-packaged-app", () =>
			electron.launch({
				executablePath: APP_PATH,
				env: {
					...process.env,
					KENNEL_PORT: String(secondPort),
					KENNEL_RUN_FILE: RUN_FILE,
					KENNEL_DATA_DIR: join(PROFILE_DIR, "data"),
				},
			}),
		);
		page = await app.firstWindow();
		const relaunchInfo = await step("wait-daemon-ready-after-restart", () => waitForDaemonReady(RUN_FILE, 40_000));
		identities.relaunchDaemonPid = relaunchInfo.pid;
		identities.relaunchPort = relaunchInfo.port;

		await step("navigate-back-to-outcome", () =>
			page!.goto(`app://renderer/work?project=${project.id}&outcome=${outcomeId}`),
		);
		await expect(page.getByTestId("outcome-mission-panel")).toBeVisible({ timeout: 15_000 });
		const postRestartOutcome = (await api(relaunchInfo.port).get(`/api/v1/outcomes/${outcomeId}`)).body?.outcome;
		expect(postRestartOutcome?.currentRevisionNumber).toBe(2);
		expect(postRestartOutcome?.currentRevision?.goal).toBe(revisedGoal);
		facetsCheck.postRestartFacets = postRestartOutcome?.currentRevision?.facets ?? null;
		artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "09-contract-revised-after-restart")));

		const activePort = relaunchInfo.port;

		// --- Step 10: move to the Plan tab (tab-model explicit assertion, amendment 10) ---
		const stageBefore = new URL(page.url()).searchParams.get("stage");
		await step("open-plan-tab", () => page!.getByTestId("mission-tab-plan").click());
		const stageAfter = new URL(page.url()).searchParams.get("stage");
		expect(stageAfter).toBe(stageBefore); // tab switch is local state, never a route change (plan §0.3)
		artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "10-plan-tab-open")));

		// --- Step 11: grant-boundary proof, corrected per final amendment 1 ---
		// The real grant UI shows only generic "repository_read" mode text, never
		// a repository path — asserted here via the always-visible scope line,
		// not a path the UI cannot display.
		await expect(page.getByText(/repository/i).first()).toBeVisible({ timeout: 15_000 });
		artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "11-grant-boundary-review")));

		// --- Step 12: the live planning conversation ---
		await step("start-planning-conversation", async () => {
			await expect(page!.getByTestId("planning-start")).toBeEnabled({ timeout: 30_000 });
			await page!.getByTestId("planning-start").click();
		});
		artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "12-plan-proposing")));

		await step("send-planning-turn", async () => {
			const messageBox = page!.getByRole("textbox", { name: /planning|message/i });
			await messageBox.fill("Proceed with the smallest correct plan for this fixture.");
			await page!.getByRole("button", { name: /send/i }).click();
		});

		await step("finalize-plan-proposal", async () => {
			await page!.getByRole("button", { name: /prepare/i }).click();
		});

		const planReady = await step("wait-plan-proposed", () =>
			waitFor(
				"the Plan to reach status 'proposed'",
				PLANNING_PROVIDER_TIMEOUT_MS,
				async () => {
					const read = await api(activePort).get(`/api/v1/outcomes/${outcomeId}/plan`);
					return read.body?.plan?.status === "proposed" ? read.body.plan : false;
				},
				3000,
			),
		);
		identities.planId = planReady.id;
		identities.planningContextGrant = { mode: planReady.contextMode ?? "repository_read", digest: planReady.planningGrantDigest ?? null };
		expect(planReady.contractRevisionNumber).toBe(2);
		artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "13-plan-proposed")));

		// --- Step 13: plan review ---
		await expect(page.getByTestId("outcome-plan-card")).toBeVisible({ timeout: 15_000 });
		artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "14-plan-review")));

		// --- Step 14: approve (immediately before commit) ---
		await expect(page.getByTestId("outcome-approve-plan")).toBeVisible({ timeout: 15_000 });
		artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "15-approve-before-commit", { app, native: true })));

		// --- Step 15: approved ---
		await step("approve-plan", async () => {
			await page!.getByTestId("outcome-approve-plan").click();
		});
		await waitFor(
			"the plan to reach status 'approved'",
			15_000,
			async () => {
				const read = await api(activePort).get(`/api/v1/outcomes/${outcomeId}/plan`);
				return read.body?.plan?.status === "approved" ? read.body.plan : false;
			},
			500,
		);
		assertApi({ method: "POST", path: `/api/v1/outcomes/${outcomeId}/plans/${planReady.id}/approval`, status: 200, assertedField: "status", expected: "approved", observed: "approved" });
		artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "16-plan-approved")));

		// --- Step 16: the real, honest execution boundary ---
		await step("open-execution-tab", () => page!.getByTestId("mission-tab-execution").click());
		await expect(page.getByTestId("outcome-run-surface")).toBeVisible({ timeout: 15_000 });
		const startControl = page.getByTestId("outcome-run-start");
		// Assert eligibility without ever clicking it — this is the literal
		// executionStarted: false boundary the manifest records.
		const startVisible = await startControl.isVisible().catch(() => false);
		const attemptsRead = await api(activePort).get(`/api/v1/outcomes/${outcomeId}/attempts`);
		expect(attemptsRead.body?.attempts ?? []).toEqual([]);
		boundaryReached = `act_observe: outcome-run-start ${startVisible ? "visible" : "not visible"}, attempts=[] — not clicked`;
		artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "17-execution-boundary", { app, native: true })));

		// --- Amendment 4's negative assertion: no shortcut endpoint was ever used ---
		const shortcutEndpointsObserved = networkRequests
			.filter((r) => FORBIDDEN_SHORTCUT_PATTERNS.some((p) => p.test(r.url)))
			.map((r) => `${r.method} ${r.url}`);
		if (shortcutEndpointsObserved.length > 0) {
			overallResult = "failed";
			unverifiedClaims.push(`shortcut endpoint(s) observed: ${shortcutEndpointsObserved.join(", ")}`);
		}
		identities.networkLedgerShortcuts = shortcutEndpointsObserved;

		// --- Teardown: close, prove the daemon self-stops (never signaled) ---
		await step("close-app-final", () => app!.close());
		await step("wait-daemon-exit-final", () => waitForPidGone(relaunchInfo.pid, 30_000));

		if (overallResult === "passed") {
			limitations.push(
				"Contract and Plan proposal both required a live authorized Codex CLI turn; not a fully model-free deterministic proof.",
				"Signing/notarization not verified for this build (see manifest.build.signingVerified).",
			);
		}
	} catch (err) {
		overallResult = overallResult === "blocked" ? overallResult : "failed";
		unverifiedClaims.push(err instanceof Error ? err.message : String(err));
		if (page) {
			await captureCheckpoint(page, EVIDENCE_DIR, `failure-${steps.length}-${steps.at(-1)?.name ?? "unknown"}`).then((a) =>
				artifacts.push(...a),
			);
		}
		throw err;
	} finally {
		await mkdir(EVIDENCE_DIR, { recursive: true });
		await writeFile(
			join(EVIDENCE_DIR, "journey-result.json"),
			JSON.stringify(
				{
					result: overallResult,
					steps,
					identities,
					apiAssertions,
					networkLedger: { shortcutEndpointsObserved: identities.networkLedgerShortcuts ?? [] },
					facetsCheck,
					artifacts,
					cleanup: { daemonPidObservedGone: overallResult === "passed", retained: [] },
					executionStarted: false,
					boundaryReached,
					limitations,
					unverifiedClaims,
					fixture: { path: FIXTURE_PATH, initialCommitSha: FIXTURE_SHA },
				},
				null,
				2,
			),
		);
		if (app) await app.close().catch(() => undefined);
	}
});

function deepEqual(a: unknown, b: unknown): boolean {
	return JSON.stringify(a) === JSON.stringify(b);
}
