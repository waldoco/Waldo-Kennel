import { test, expect, _electron as electron, type ElectronApplication, type Page, type Response } from "@playwright/test";
import net from "node:net";
import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";

import { readDaemonBuildRevision, waitFor, waitForDaemonReady, waitForPidGone } from "./support/daemon-wait";
import { captureCheckpoint, type CapturedArtifact } from "./support/screenshot";
import { stubFolderPicker } from "./support/dialog-stub";
import { redactHeaders } from "../../scripts/outcome-journey/manifest.mjs";
import { TEARDOWN_GRACE_MS } from "../../scripts/outcome-journey/timeouts.mjs";

// Packaged macOS Outcome journey: the one-command proof that Contract
// creation, one Contract revision (surviving a real restart), a live-agent
// Plan proposal, and Plan approval all happen through the REAL packaged Work
// UI, with the daemon as the authoritative source of truth at every step, and
// that the run stops at the real pre-execution boundary without ever
// starting a persistent Attempt.
//
// This is the revision after Instinct's round-2 review of f88d499e4
// (2026-09-17): every fix below is numbered to that review, not invented
// fresh, so a re-reviewer can check this file against that list directly.
// Round-1 fixes (numbered separately) remain in place; see the file history
// and docs/handoffs/2026-09-16-macos-outcome-journey-harness-plan.md.

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
const OVERALL_TIMEOUT_MS = Number(env.KENNEL_JOURNEY_OVERALL_TIMEOUT_MS ?? 600_000);

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

type ResultKind = "passed" | "failed" | "blocked" | "ambiguous";

interface StepRecord {
	name: string;
	startedAt: string;
	endedAt: string;
	durationMs: number;
	result: ResultKind;
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

/** Review round 1 item 1: one ledger, every entry tagged by source, populated
 * from the moment the page exists — never a shortcut for the harness to
 * pretend the UI did something the harness itself did instead. */
interface NetworkEntry {
	source: "renderer" | "harness";
	method: string;
	path: string;
	status: number;
	requestKey?: string;
	requestHeaders?: Record<string, string>;
	requestBody?: unknown;
	responseBody?: unknown;
	observedAt: number;
}

// --- Journey-wide evidence accumulators, written to journey-result.json in a
// finally block so a failed run still produces the manifest fragment the
// orchestrator needs (plan §3.4: retain evidence on failure). ---
const steps: StepRecord[] = [];
const apiAssertions: ApiAssertion[] = [];
const artifacts: CapturedArtifact[] = [];
const identities: Record<string, unknown> = { runId: RUN_ID };
const networkEntries: NetworkEntry[] = [];
const limitations: string[] = [];
const unverifiedClaims: string[] = [];
let overallResult: ResultKind = "passed";
let boundaryReached = "aborted before reaching the execution boundary";
const facetsCheck: { step: number; identical: boolean | null; preSaveFacets?: unknown; postSaveFacets?: unknown; postRestartFacets?: unknown } = {
	step: 8,
	identical: null,
};

// Round-1 item 3: these three are the ONLY way overallResult moves away from
// "passed", and each is a deliberate, named classification — never a bare
// catch-and-mark-failed. blocked wins over ambiguous wins over failed, so a
// later, less-specific catch never downgrades an earlier honest classification.
function markBlocked(reason: string) {
	overallResult = "blocked";
	unverifiedClaims.push(reason);
}
function markAmbiguous(reason: string) {
	if (overallResult !== "blocked") overallResult = "ambiguous";
	unverifiedClaims.push(reason);
}
function markFailed(reason: string) {
	if (overallResult !== "blocked" && overallResult !== "ambiguous") overallResult = "failed";
	unverifiedClaims.push(reason);
}

/**
 * Round 2 item 4: a real AbortController, not just a losing Promise.race
 * branch. `Promise.race` alone does not stop the losing promise from
 * continuing to run — once the deadline fires, every in-flight and future
 * poll must observe `signal.aborted` and stop cooperatively, or the
 * "abandoned" journey body keeps mutating the same steps/networkEntries
 * arrays the teardown code is concurrently reading to write the evidence
 * fragment. This signal is threaded through every wait/act helper below.
 */
const overallAbort = new AbortController();

async function step<T>(name: string, fn: () => Promise<T>): Promise<T> {
	if (overallAbort.signal.aborted) {
		throw new Error(`skipped (overall deadline already exceeded): ${name}`);
	}
	const startedAt = new Date().toISOString();
	try {
		const result = await fn();
		steps.push({ name, startedAt, endedAt: new Date().toISOString(), durationMs: Date.now() - Date.parse(startedAt), result: "passed", failureCode: null });
		return result;
	} catch (err) {
		const message = err instanceof Error ? err.message : String(err);
		if (overallResult === "passed") markFailed(message);
		steps.push({
			name,
			startedAt,
			endedAt: new Date().toISOString(),
			durationMs: Date.now() - Date.parse(startedAt),
			result: overallResult,
			failureCode: message,
		});
		throw err;
	}
}

/** Round 2 item 4: fires overallAbort (not just a rejected promise) the
 * moment the deadline elapses, so every signal-aware poll in flight stops
 * immediately instead of running out its own separately-configured timeout. */
async function withOverallDeadline<T>(ms: number, fn: (signal: AbortSignal) => Promise<T>): Promise<T> {
	let timer: ReturnType<typeof setTimeout>;
	const deadline = new Promise<never>((_, reject) => {
		timer = setTimeout(() => {
			const err = new Error(`overall journey ceiling of ${Math.round(ms / 1000)}s exceeded`);
			overallAbort.abort(err);
			reject(err);
		}, ms);
	});
	try {
		return await Promise.race([fn(overallAbort.signal), deadline]);
	} finally {
		clearTimeout(timer!);
	}
}

function safeJson<T>(value: unknown): T | undefined {
	try {
		return typeof value === "string" ? (JSON.parse(value) as T) : (value as T);
	} catch {
		return undefined;
	}
}

/** Registered once per launch, immediately after the page exists and before
 * any UI action (round-1 item 1) — this is the harness's only source of truth
 * for what the renderer actually did on the wire. */
function attachNetworkCapture(page: Page): void {
	page.on("response", (response: Response) => {
		void recordRendererResponse(response).catch(() => undefined);
	});
}

async function recordRendererResponse(response: Response): Promise<void> {
	const url = new URL(response.url());
	if (!url.pathname.startsWith("/api/v1/")) return;
	const request = response.request();
	const requestBody = request.postData() ? safeJson<Record<string, unknown>>(request.postData()) : undefined;
	let responseBody: unknown;
	try {
		responseBody = await response.json();
	} catch {
		responseBody = undefined;
	}
	// Headers are redacted regardless of whether anything sensitive is present
	// today — the daemon is loopback-trusted, not bearer-authenticated (plan
	// §0.7's sibling finding) — but this stays true if that ever changes.
	// manifest.mjs has no type annotations (plain JS), so its inferred return
	// type here is looser than reality — the runtime shape is exactly
	// Record<string, string>, matching Playwright's own Request.headers().
	const requestHeaders = redactHeaders(request.headers()) as Record<string, string>;
	networkEntries.push({
		source: "renderer",
		method: request.method(),
		path: url.pathname,
		status: response.status(),
		requestKey: typeof requestBody?.requestKey === "string" ? requestBody.requestKey : undefined,
		requestHeaders,
		requestBody,
		responseBody,
		observedAt: Date.now(),
	});
}

/**
 * Round-1 item 2: performs a UI action, then finds the renderer response THAT
 * ACTION produced in the real capture (not a re-fetch pretending to be it).
 * Throws if no matching response ever appears — a UI action with no observed
 * network effect is itself a finding, not something to paper over.
 */
async function actAndCapture(
	act: () => Promise<void>,
	pathPattern: RegExp,
	label: string,
	signal: AbortSignal,
	timeoutMs = 30_000,
): Promise<NetworkEntry> {
	const startedAt = Date.now();
	await act();
	return waitFor(
		`a captured renderer response for ${label}`,
		timeoutMs,
		async () => {
			const match = [...networkEntries]
				.reverse()
				.find((e) => e.source === "renderer" && pathPattern.test(e.path) && e.observedAt >= startedAt);
			return match ?? false;
		},
		250,
		signal,
	);
}

/**
 * Round-1 item 4: the settlement wait tied to one already-observed 2xx
 * mutation. A non-2xx mutation is an ordinary failure. A 2xx mutation whose
 * effect never settles within its ceiling is ambiguous — never retried,
 * never silently reclassified as an ordinary failure.
 */
async function settleAfterMutation<T>(
	label: string,
	mutation: NetworkEntry,
	timeoutMs: number,
	check: () => Promise<T | false>,
	signal: AbortSignal,
	intervalMs = 1500,
): Promise<T> {
	if (mutation.status < 200 || mutation.status >= 300) {
		throw new Error(`${label}: the mutation itself was not accepted (status ${mutation.status})`);
	}
	try {
		return await waitFor(`${label} to settle`, timeoutMs, check, intervalMs, signal);
	} catch (err) {
		markAmbiguous(`${label}: accepted (status ${mutation.status}) but its effect never settled — ${err instanceof Error ? err.message : String(err)}`);
		throw err;
	}
}

// Round 2 item 3: the daemon's agent probe is a read-only local diagnostic
// (it runs e.g. `codex --version`/an auth check locally) — it mutates no
// Outcome/Project/Contract/Plan domain record, so it is allowlisted as the
// one harness-issued non-GET call, distinct from a "shortcut" that would
// substitute for a real UI action.
const HARNESS_ALLOWED_MUTATIONS = [/^\/api\/v1\/agents\/[^/]+\/probe$/];

function api(port: number) {
	const base = `http://127.0.0.1:${port}`;
	return {
		async get(path: string) {
			const res = await fetch(`${base}${path}`);
			const body = res.status === 204 ? null : await res.json().catch(() => null);
			networkEntries.push({ source: "harness", method: "GET", path, status: res.status, responseBody: body, observedAt: Date.now() });
			return { status: res.status, body };
		},
		/** Round 2 item 3: a FRESH, non-cached probe of one agent's local auth
		 * state — never the stale-prone GET /agents list. */
		async probeAgent(agentId: string) {
			const path = `/api/v1/agents/${agentId}/probe`;
			const res = await fetch(`${base}${path}`, { method: "POST" });
			const body = res.status === 204 ? null : await res.json().catch(() => null);
			networkEntries.push({ source: "harness", method: "POST", path, status: res.status, responseBody: body, observedAt: Date.now() });
			return { status: res.status, body };
		},
	};
}

/** Round 2 item 2: records the assertion for the ledger AND enforces it —
 * an "assertion" that never actually compares expected vs. observed is not
 * an assertion. */
function assertApi(entry: ApiAssertion) {
	apiAssertions.push(entry);
	if (entry.expected !== undefined && !deepEqual(entry.expected, entry.observed)) {
		throw new Error(
			`API assertion failed: ${entry.method} ${entry.path} expected ${entry.assertedField ?? "field"}=${JSON.stringify(entry.expected)}, observed ${JSON.stringify(entry.observed)}`,
		);
	}
}

// Round-1 item 1's final check, corrected per round 2 item 1: a "shortcut"
// endpoint is only forbidden as a MUTATION. Both /outcomes/{id}/plans and
// /projects/{id}/outcomes have entirely legitimate GET siblings this harness
// itself calls (list Plans is not used here, but list Outcomes for a project
// is, at lines around step 6 and the pre-existing-Outcome check) — flagging
// by path alone would fail every good run the moment those real, allowed GETs
// happen to share a path prefix with the forbidden POST.
const FORBIDDEN_SHORTCUT_MUTATIONS = [/^\/api\/v1\/outcomes\/[^/]+\/plans$/, /^\/api\/v1\/projects\/[^/]+\/outcomes$/];
const MUTATING_METHODS = new Set(["POST", "PUT", "PATCH", "DELETE"]);
const EXPECTED_RENDERER_MUTATIONS = [
	/^\/api\/v1\/projects$/,
	/^\/api\/v1\/projects\/initialize$/,
	/^\/api\/v1\/projects\/[^/]+\/intakes$/,
	/^\/api\/v1\/intakes\/[^/]+\/analysis$/,
	/^\/api\/v1\/intakes\/[^/]+\/proposals$/,
	/^\/api\/v1\/intakes\/[^/]+\/confirmation$/,
	/^\/api\/v1\/outcomes\/[^/]+\/revisions$/,
	/^\/api\/v1\/outcomes\/[^/]+\/planning-sessions$/,
	/^\/api\/v1\/outcomes\/[^/]+\/planning-sessions\/[^/]+\/messages$/,
	/^\/api\/v1\/outcomes\/[^/]+\/planning-sessions\/[^/]+\/proposal$/,
	/^\/api\/v1\/outcomes\/[^/]+\/plans\/[^/]+\/approval$/,
];

function isForbiddenShortcut(entry: NetworkEntry): boolean {
	return MUTATING_METHODS.has(entry.method) && FORBIDDEN_SHORTCUT_MUTATIONS.some((p) => p.test(entry.path));
}

function verifyNetworkLedger(): void {
	const shortcutsObserved = networkEntries.filter(isForbiddenShortcut).map((e) => `${e.source}: ${e.method} ${e.path}`);
	const harnessMutations = networkEntries
		.filter((e) => e.source === "harness" && e.method !== "GET" && !HARNESS_ALLOWED_MUTATIONS.some((p) => p.test(e.path)))
		.map((e) => `${e.method} ${e.path}`);
	const unexpectedRendererMutations = networkEntries
		.filter((e) => e.source === "renderer" && MUTATING_METHODS.has(e.method) && !EXPECTED_RENDERER_MUTATIONS.some((p) => p.test(e.path)))
		.map((e) => `${e.method} ${e.path}`);
	if (shortcutsObserved.length > 0) markFailed(`shortcut mutation(s) observed: ${shortcutsObserved.join(", ")}`);
	if (harnessMutations.length > 0) markFailed(`the harness itself issued unallowed non-GET request(s): ${harnessMutations.join(", ")}`);
	if (unexpectedRendererMutations.length > 0) markFailed(`renderer issued unexpected mutating request(s): ${unexpectedRendererMutations.join(", ")}`);
}

test("packaged macOS Outcome journey: Contract through Plan approval, stopping at the execution boundary", async () => {
	test.setTimeout(OVERALL_TIMEOUT_MS + TEARDOWN_GRACE_MS);
	let app: ElectronApplication | undefined;
	let page: Page | undefined;
	let daemonPidActuallyGone = false;
	const retained: { path: string; reason: string }[] = [];

	try {
		await withOverallDeadline(OVERALL_TIMEOUT_MS, async (signal) => {
			const port = await freePort();
			identities.port = port;

			app = await step("launch-packaged-app", () =>
				electron.launch({
					executablePath: APP_PATH,
					env: { ...process.env, KENNEL_PORT: String(port), KENNEL_RUN_FILE: RUN_FILE, KENNEL_DATA_DIR: join(PROFILE_DIR, "data") },
				}),
			);
			page = await app.firstWindow();
			attachNetworkCapture(page); // round-1 item 1: before ANYTHING else touches the UI

			const origin = await page.evaluate(() => location.origin);
			expect(origin).toBe("app://renderer");

			const runInfo = await step("wait-daemon-ready", () => waitForDaemonReady(RUN_FILE, 40_000, signal));
			identities.daemonPid = runInfo.pid;

			const daemonBuildRevision = await readDaemonBuildRevision(port);
			identities.daemonBuildRevision = daemonBuildRevision;
			if (daemonBuildRevision !== RECORDED_GIT_SHA) {
				const reason = `daemon buildRevision ${daemonBuildRevision} does not match recorded git SHA ${RECORDED_GIT_SHA}`;
				markFailed(reason);
				boundaryReached = `aborted: ${reason}`;
				throw new Error(reason);
			}

			// Round 2 item 3: a FRESH probe, not the stale-prone cached
			// GET /api/v1/agents list ("Advisory and stale-prone; spawn may still
			// fail" — backend/internal/service/agent/service.go's own doc comment
			// on Inventory.Authorized). Even this fresh probe stays advisory per
			// that same file's Info.AuthStatus doc — "spawn remains the
			// authoritative validation point" — so a pass here is a fast-fail
			// gate against the obvious "never logged in" case, not proof the
			// later live turn will succeed; that proof is the intake-analysis
			// settle-wait actually reaching a non-"analyzing" status.
			await step("codex-authentication-check", async () => {
				const probe = await api(port).probeAgent("codex");
				const authStatus = (probe.body as { agent?: { authStatus?: string }; installed?: boolean } | null)?.agent?.authStatus;
				const installed = (probe.body as { installed?: boolean } | null)?.installed;
				if (!installed || authStatus !== "authorized") {
					const reason = `Codex failed the fresh local probe (installed=${installed}, authStatus=${authStatus})`;
					markBlocked(reason);
					boundaryReached = `aborted: ${reason}`;
					throw new Error(reason);
				}
			});
			limitations.push(
				"Codex authentication was checked via a fresh local probe (POST /api/v1/agents/codex/probe), which the daemon's own service code documents as advisory — spawn (the first real provider turn, during intake analysis below) remains the authoritative validation point.",
			);

			artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "01-clean-entry", { app, native: true })));

			// --- Step 2: register the fixture project ---
			const projectMutation = await step("register-fixture-project", async () => {
				await page!.getByRole("button", { name: /add project/i }).click();
				await page!.getByTestId("create-project-import-existing").click();
				await stubFolderPicker(app!, FIXTURE_PATH);
				await page!.getByTestId("create-project-choose-folder").click();
				const dialog = page!.getByRole("dialog");
				// Round-1 item 8: scoped to the setup dialog, not a page-wide regex
				// — the submit control lives in packages/product-ui's
				// ProjectSetupFormView, outside this slice's file list (plan §12).
				const submit = dialog.getByRole("button", { name: /create.*start/i });
				await expect(submit).toBeEnabled({ timeout: 30_000 });
				return actAndCapture(() => submit.click(), /^\/api\/v1\/projects$/, "project registration", signal);
			});
			if (projectMutation.status < 200 || projectMutation.status >= 300) {
				throw new Error(`project registration was not accepted (status ${projectMutation.status})`);
			}
			artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "02-project-registered")));

			const projectsRead = await api(port).get("/api/v1/projects");
			const project = (projectsRead.body?.projects ?? []).find((p: { path: string }) => p.path === FIXTURE_PATH);
			if (!project) throw new Error(`registered project with path ${FIXTURE_PATH} not found via GET /api/v1/projects`);
			assertApi({ method: "GET", path: "/api/v1/projects", status: projectsRead.status, assertedField: "path", expected: FIXTURE_PATH, observed: project.path });
			identities.projectId = project.id;

			// GET-only, never a mutation source: proving no pre-existing Outcome is
			// a legitimate read, and its path happens to match
			// FORBIDDEN_SHORTCUT_MUTATIONS by prefix — isForbiddenShortcut()
			// requires a mutating method too, so this GET is correctly exempt.
			const preExisting = await api(port).get(`/api/v1/projects/${project.id}/outcomes`);
			if ((preExisting.body?.outcomes ?? []).length > 0) throw new Error("unexpected pre-existing Outcome on the freshly registered fixture project");

			await step("open-registered-project", () => page!.getByRole("button", { name: new RegExp(project.name, "i") }).click());

			// --- Steps 3-6: intake capture -> live analysis -> proposal review -> confirm ---
			artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "03-outcome-create-form")));
			const statement = `Prove the packaged macOS Outcome journey (${RUN_ID})`;
			const captureMutation = await step("submit-intake-capture", () =>
				actAndCapture(
					async () => {
						await page!.getByTestId("intake-statement-input").fill(statement);
						await page!.getByTestId("intake-capture-submit").click();
					},
					/^\/api\/v1\/projects\/[^/]+\/intakes$/,
					"intake capture",
					signal,
				),
			);
			const intakeId = (captureMutation.responseBody as { intake?: { session?: { id?: string } } } | undefined)?.intake?.session?.id;
			if (!intakeId) throw new Error("intake capture response carried no intake session id");
			identities.intakeId = intakeId;

			artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "04-intake-analysis-waiting")));
			await step("wait-intake-analysis", () =>
				settleAfterMutation(
					"intake analysis",
					captureMutation,
					INTAKE_ANALYSIS_TIMEOUT_MS,
					async () => {
						const read = await api(port).get(`/api/v1/intakes/${intakeId}`);
						return read.body?.intake?.session?.status && read.body.intake.session.status !== "analyzing" ? read.body : false;
					},
					signal,
				),
			);

			artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "05-intake-proposal-review")));
			const confirmMutation = await step("confirm-outcome", async () => {
				await expect(page!.getByTestId("intake-confirm")).toBeEnabled({ timeout: 15_000 });
				return actAndCapture(() => page!.getByTestId("intake-confirm").click(), /^\/api\/v1\/intakes\/[^/]+\/confirmation$/, "outcome confirmation", signal);
			});
			const outcomeId = (confirmMutation.responseBody as { intake?: { confirmedOutcome?: { id?: string } } } | undefined)?.intake?.confirmedOutcome?.id;
			if (!outcomeId) throw new Error("confirmation response carried no confirmedOutcome id");
			identities.outcomeId = outcomeId;

			// Round 2 item 2: the CAPTURED confirmation response is the primary
			// evidence for revision 1 — the GET below is durability confirmation,
			// not the sole proof.
			const outcomeAfterCreate = await api(port).get(`/api/v1/outcomes/${outcomeId}`);
			assertApi({
				method: "POST",
				path: `/api/v1/intakes/${intakeId}/confirmation`,
				status: confirmMutation.status,
				assertedField: "currentRevisionNumber (durable GET, cross-checked)",
				expected: 1,
				observed: outcomeAfterCreate.body?.outcome?.currentRevisionNumber,
			});

			// Round-1 item 9 (project-scoping half): the created Outcome must
			// actually belong to the project it was created under — asserted via
			// the daemon's own project-scoped listing (a legitimate GET, not a
			// forbidden mutation — see isForbiddenShortcut above), since
			// Outcome/domain "space" ids are not the same namespace as project ids.
			const projectOutcomes = await api(port).get(`/api/v1/projects/${project.id}/outcomes`);
			const belongsToProject = (projectOutcomes.body?.outcomes ?? []).some((o: { id: string }) => o.id === outcomeId);
			if (!belongsToProject) throw new Error(`created Outcome ${outcomeId} does not appear under GET /projects/${project.id}/outcomes`);

			artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "06-outcome-created")));

			// --- Step 7: contract tab is already open (plan §0.3 — not a navigation) ---
			await expect(page.getByTestId("outcome-mission-panel")).toBeVisible({ timeout: 15_000 });
			artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "07-contract-tab-open")));

			// --- Step 8: evidence-first facets diff. Round 2 item 2: the CAPTURED
			// revision response is the primary evidence for what the save did;
			// the subsequent GET is durability confirmation only, cross-checked
			// against it, never a substitute for it. A mismatch anywhere is a hard
			// failure (round-1 item 3b), not a note. ---
			const preSaveSnapshot = (await api(port).get(`/api/v1/outcomes/${outcomeId}`)).body?.outcome?.currentRevision;
			facetsCheck.preSaveFacets = preSaveSnapshot?.facets ?? null;
			const revisedGoal = `${preSaveSnapshot?.goal ?? ""} (revised ${RUN_ID})`;

			const revisionMutation = await step("edit-and-save-contract-revision", () =>
				actAndCapture(
					async () => {
						await page!.getByTestId("contract-edit").click();
						await page!.getByTestId("contract-goal").fill(revisedGoal);
						await page!.getByTestId("contract-save").click();
					},
					/^\/api\/v1\/outcomes\/[^/]+\/revisions$/,
					"contract revision save",
					signal,
				),
			);
			const submittedFacets = (revisionMutation.requestBody as { facets?: unknown } | undefined)?.facets;
			const revisionResponseOutcome = (revisionMutation.responseBody as { outcome?: { currentRevisionNumber?: number; currentRevision?: { facets?: unknown } } } | undefined)?.outcome;

			// Primary hard-check, from the captured POST response itself.
			assertApi({
				method: "POST",
				path: `/api/v1/outcomes/${outcomeId}/revisions`,
				status: revisionMutation.status,
				assertedField: "currentRevisionNumber (captured response)",
				expected: 2,
				observed: revisionResponseOutcome?.currentRevisionNumber,
			});
			facetsCheck.postSaveFacets = revisionResponseOutcome?.currentRevision?.facets ?? null;
			const preVsPost = deepEqual(facetsCheck.preSaveFacets, facetsCheck.postSaveFacets);
			const submittedVsPre = deepEqual(submittedFacets, facetsCheck.preSaveFacets);
			facetsCheck.identical = preVsPost;
			if (!preVsPost || !submittedVsPre) {
				throw new Error(
					`facets did not round-trip through the real Contract Save path — submitted=${JSON.stringify(submittedFacets)} preSave=${JSON.stringify(facetsCheck.preSaveFacets)} postSave(captured response)=${JSON.stringify(facetsCheck.postSaveFacets)} (this contradicts the source-reading correction in plan §0.5 and needs its own investigation)`,
				);
			}

			// Durability/settlement evidence ONLY (round 2 item 2) — cross-checked
			// against the captured response above, never the primary proof.
			const durableAfterSave = await step("wait-contract-revision-2-durable", () =>
				settleAfterMutation(
					"contract revision durability",
					revisionMutation,
					15_000,
					async () => {
						const read = await api(port).get(`/api/v1/outcomes/${outcomeId}`);
						return read.body?.outcome?.currentRevisionNumber === 2 ? read.body.outcome : false;
					},
					signal,
				),
			);
			if (!deepEqual(revisionResponseOutcome?.currentRevision?.facets, durableAfterSave.currentRevision?.facets)) {
				throw new Error("facets differ between the captured POST response and the durable GET immediately after — a durability regression, not just a save-time one");
			}
			identities.contractRevisions = [1, 2];
			artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "08-contract-revised")));

			// --- Step 9: restart against the SAME profile, prove durability ---
			const preRestartPid = runInfo.pid;
			await step("close-app", () => app!.close());
			await step("wait-daemon-exit-before-restart", () => waitForPidGone(preRestartPid, 20_000));

			const secondPort = await freePort();
			app = await step("relaunch-packaged-app", () =>
				electron.launch({
					executablePath: APP_PATH,
					env: { ...process.env, KENNEL_PORT: String(secondPort), KENNEL_RUN_FILE: RUN_FILE, KENNEL_DATA_DIR: join(PROFILE_DIR, "data") },
				}),
			);
			page = await app.firstWindow();
			attachNetworkCapture(page);
			const relaunchInfo = await step("wait-daemon-ready-after-restart", () => waitForDaemonReady(RUN_FILE, 40_000, signal));
			identities.relaunchDaemonPid = relaunchInfo.pid;
			identities.relaunchPort = relaunchInfo.port;
			const activePort = relaunchInfo.port;

			await step("navigate-back-to-outcome", () => page!.goto(`app://renderer/work?project=${project.id}&outcome=${outcomeId}`));
			await expect(page.getByTestId("outcome-mission-panel")).toBeVisible({ timeout: 15_000 });

			const postRestartOutcome = (await api(activePort).get(`/api/v1/outcomes/${outcomeId}`)).body?.outcome;
			if (postRestartOutcome?.currentRevisionNumber !== 2) throw new Error(`expected currentRevisionNumber 2 after restart, observed ${postRestartOutcome?.currentRevisionNumber}`);
			if (postRestartOutcome?.currentRevision?.goal !== revisedGoal) throw new Error("revised goal did not survive the restart");
			facetsCheck.postRestartFacets = postRestartOutcome?.currentRevision?.facets ?? null;
			if (!deepEqual(facetsCheck.postSaveFacets, facetsCheck.postRestartFacets)) {
				throw new Error(`facets changed across restart: postSave=${JSON.stringify(facetsCheck.postSaveFacets)} postRestart=${JSON.stringify(facetsCheck.postRestartFacets)}`);
			}
			artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "09-contract-revised-after-restart")));

			// --- Step 10: move to the Plan tab (tab-model explicit assertion) ---
			const stageBefore = new URL(page.url()).searchParams.get("stage");
			await step("open-plan-tab", () => page!.getByTestId("mission-tab-plan").click());
			const stageAfter = new URL(page.url()).searchParams.get("stage");
			expect(stageAfter).toBe(stageBefore); // tab switch is local state, never a route change (plan §0.3)
			artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "10-plan-tab-open")));

			// --- Step 11: grant-boundary proof (generic mode text only, no path
			// display — final amendment 1) ---
			await expect(page.getByText(/repository/i).first()).toBeVisible({ timeout: 15_000 });
			artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "11-grant-boundary-review")));

			// --- Step 12: the live planning conversation, scoped under
			// mission-planning throughout (round-1 item 8) ---
			const planningRoot = page.getByTestId("mission-planning");
			const startMutation = await step("start-planning-conversation", () =>
				actAndCapture(
					async () => {
						await expect(planningRoot.getByTestId("planning-start")).toBeEnabled({ timeout: 30_000 });
						await planningRoot.getByTestId("planning-start").click();
					},
					/^\/api\/v1\/outcomes\/[^/]+\/planning-sessions$/,
					"planning session start",
					signal,
				),
			);
			await step("wait-planning-session-active", () =>
				settleAfterMutation(
					"planning session start",
					startMutation,
					PLANNING_PROVIDER_TIMEOUT_MS,
					async () => {
						const visible = await planningRoot.getByTestId("planning-session-status").isVisible().catch(() => false);
						return visible ? true : false;
					},
					signal,
				),
			);
			artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "12-plan-proposing")));

			const sendMutation = await step("send-planning-turn", () =>
				actAndCapture(
					async () => {
						await planningRoot.getByRole("textbox").fill("Proceed with the smallest correct plan for this fixture.");
						await planningRoot.getByRole("button", { name: /send/i }).click();
					},
					/^\/api\/v1\/outcomes\/[^/]+\/planning-sessions\/[^/]+\/messages$/,
					"planning message",
					signal,
				),
			);
			// Round-1 item 8: wait for the provider's turn to actually land
			// (waitingOn === "owner") before clicking Prepare — never fire it
			// immediately after Send.
			await step("wait-provider-turn", () =>
				settleAfterMutation(
					"planning provider turn",
					sendMutation,
					PLANNING_PROVIDER_TIMEOUT_MS,
					async () => {
						const read = await api(activePort).get(`/api/v1/outcomes/${outcomeId}/planning-session`);
						return read.body?.planning?.session?.waitingOn === "owner" ? read.body.planning.session : false;
					},
					signal,
				),
			);

			const finalizeMutation = await step("finalize-plan-proposal", () =>
				actAndCapture(
					() => planningRoot.getByRole("button", { name: /prepare/i }).click(),
					/^\/api\/v1\/outcomes\/[^/]+\/planning-sessions\/[^/]+\/proposal$/,
					"planning finalize",
					signal,
				),
			);

			const planReady = await step("wait-plan-proposed", () =>
				settleAfterMutation(
					"plan proposal",
					finalizeMutation,
					PLANNING_PROVIDER_TIMEOUT_MS,
					async () => {
						const planRead = await api(activePort).get(`/api/v1/outcomes/${outcomeId}/plan`);
						if (planRead.body?.plan?.status !== "proposed") return false;
						const sessionRead = await api(activePort).get(`/api/v1/outcomes/${outcomeId}/planning-session`);
						const session = sessionRead.body?.planning?.session;
						// Round-1 item 9 (grant-evidence half): required, not defaulted.
						// Missing or wrong evidence fails here — it never falls back to
						// an assumed "repository_read" success.
						if (session?.contextMode !== "repository_read") return false;
						if (!session?.planningGrantDigest) return false;
						return { plan: planRead.body.plan, session };
					},
					signal,
				),
			);
			identities.planId = planReady.plan.id;
			identities.planningContextGrant = { mode: planReady.session.contextMode, digest: planReady.session.planningGrantDigest };
			if (planReady.plan.contractRevisionNumber !== 2) throw new Error(`plan bound to contract revision ${planReady.plan.contractRevisionNumber}, expected 2`);
			artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "13-plan-proposed")));

			// --- Step 13: plan review ---
			await expect(page.getByTestId("outcome-plan-card")).toBeVisible({ timeout: 15_000 });
			artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "14-plan-review")));

			// --- Step 14: approve (immediately before commit) ---
			await expect(page.getByTestId("outcome-approve-plan")).toBeVisible({ timeout: 15_000 });
			artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "15-approve-before-commit", { app, native: true })));

			// --- Step 15: approved. Round 2 item 2: hard-checked directly against
			// the CAPTURED approval response (assertApi now actually enforces —
			// this was previously recorded but never verified). ---
			const approveMutation = await step("approve-plan", () =>
				actAndCapture(() => page!.getByTestId("outcome-approve-plan").click(), /^\/api\/v1\/outcomes\/[^/]+\/plans\/[^/]+\/approval$/, "plan approval", signal),
			);
			const approvedStatus = (approveMutation.responseBody as { plan?: { status?: string } } | undefined)?.plan?.status;
			assertApi({
				method: "POST",
				path: `/api/v1/outcomes/${outcomeId}/plans/${planReady.plan.id}/approval`,
				status: approveMutation.status,
				assertedField: "status (captured response)",
				expected: "approved",
				observed: approvedStatus,
			});
			// Durability confirmation only — the hard-check above already passed.
			await step("wait-plan-approved-durable", () =>
				settleAfterMutation(
					"plan approval durability",
					approveMutation,
					15_000,
					async () => {
						const read = await api(activePort).get(`/api/v1/outcomes/${outcomeId}/plan`);
						return read.body?.plan?.status === "approved" ? read.body.plan : false;
					},
					signal,
				),
			);
			artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "16-plan-approved")));

			// --- Step 16: the real, honest execution boundary — hard-fail if
			// Start is not present/eligible (round-1 item 3c). ---
			await step("open-execution-tab", () => page!.getByTestId("mission-tab-execution").click());
			await expect(page.getByTestId("outcome-run-surface")).toBeVisible({ timeout: 15_000 });
			const startControl = page.getByTestId("outcome-run-start");
			const startVisible = await startControl.isVisible().catch(() => false);
			if (!startVisible) throw new Error("outcome-run-start is not visible/eligible after an approved plan — the execution boundary was not reached honestly");
			const attemptsRead = await api(activePort).get(`/api/v1/outcomes/${outcomeId}/attempts`);
			if ((attemptsRead.body?.attempts ?? []).length !== 0) throw new Error("unexpected pre-existing Attempt at the execution boundary");
			boundaryReached = "act_observe: outcome-run-start visible/eligible, attempts=[] — not clicked";
			artifacts.push(...(await captureCheckpoint(page, EVIDENCE_DIR, "17-execution-boundary", { app, native: true })));

			verifyNetworkLedger();

			limitations.push(
				"Contract and Plan proposal both required a live authorized Codex CLI turn; not a fully model-free deterministic proof.",
				"Signing/notarization not verified for this build (see manifest.build.signingVerified).",
			);
		});
	} catch (err) {
		if (overallResult === "passed") markFailed(err instanceof Error ? err.message : String(err));
		if (page) {
			await captureCheckpoint(page, EVIDENCE_DIR, `failure-${steps.length}-${steps.at(-1)?.name ?? "unknown"}`)
				.then((a) => artifacts.push(...a))
				.catch(() => undefined);
		}
	} finally {
		// Round-1 item 5: close FIRST, wait boundedly for the exact owned pid,
		// THEN write the cleanup fragment from what was actually observed — never
		// a proxy off overallResult. Teardown itself is NOT gated by
		// overallAbort — a deadline-exceeded journey still gets a real,
		// bounded teardown, covered by TEARDOWN_GRACE_MS on both this test's own
		// setTimeout and the orchestrator's parent process timeout.
		const lastKnownPid = (identities.relaunchDaemonPid ?? identities.daemonPid) as number | undefined;
		if (app) {
			await step("close-app-final", () => app!.close()).catch(() => undefined);
		}
		if (lastKnownPid) {
			try {
				await waitForPidGone(lastKnownPid, 30_000);
				daemonPidActuallyGone = true;
			} catch (err) {
				daemonPidActuallyGone = false;
				markFailed(`daemon pid ${lastKnownPid} did not exit within teardown's bounded wait: ${err instanceof Error ? err.message : String(err)}`);
			}
		}
		retained.push(
			{ path: EVIDENCE_DIR, reason: "evidence retained for review" },
			{ path: FIXTURE_PATH, reason: "fixture repo retained (harness does not delete it)" },
			{ path: PROFILE_DIR, reason: "isolated profile retained (harness does not delete it)" },
		);

		await mkdir(EVIDENCE_DIR, { recursive: true });
		await writeFile(
			join(EVIDENCE_DIR, "journey-result.json"),
			JSON.stringify(
				{
					result: overallResult,
					steps,
					identities,
					apiAssertions,
					networkLedger: {
						shortcutEndpointsObserved: networkEntries.filter(isForbiddenShortcut).map((e) => `${e.source}: ${e.method} ${e.path}`),
						entries: networkEntries,
					},
					facetsCheck,
					artifacts,
					cleanup: { daemonPidObservedGone: daemonPidActuallyGone, retained },
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
	}

	// Propagate a non-"passed" result to Playwright's own exit code too, so a
	// direct `playwright test` invocation (bypassing the orchestrator) still
	// fails visibly.
	expect(overallResult, unverifiedClaims.join("; ")).toBe("passed");
});

function deepEqual(a: unknown, b: unknown): boolean {
	return JSON.stringify(a) === JSON.stringify(b);
}
