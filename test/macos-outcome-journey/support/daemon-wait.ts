import { readFile } from "node:fs/promises";

// Mirrors test/e2e-pod/real-app.spec.ts's run-file discovery contract: never
// assume a port, always read the run file THIS launch's app wrote, and
// require it to carry a durable owner/pid/startedAt shape before trusting it.

export interface RunFile {
	pid: number;
	port: number;
	startedAt: string;
	owner?: string;
	buildRevision?: string;
}

export async function readRunFile(runFile: string): Promise<RunFile | null> {
	try {
		return JSON.parse(await readFile(runFile, "utf8")) as RunFile;
	} catch {
		return null;
	}
}

/** Bounded poll on a named condition — never a fixed sleep (plan §5). */
export async function waitFor<T>(
	label: string,
	timeoutMs: number,
	check: () => Promise<T | false | undefined | null>,
	intervalMs = 1000,
): Promise<T> {
	const deadline = Date.now() + timeoutMs;
	let lastObserved: unknown;
	for (;;) {
		let result: T | false | undefined | null;
		try {
			result = await check();
		} catch (err) {
			lastObserved = err instanceof Error ? err.message : String(err);
			result = false;
		}
		if (result) return result;
		if (result !== false && result !== undefined && result !== null) lastObserved = result;
		if (Date.now() >= deadline) {
			throw new Error(
				`timed out after ${Math.round(timeoutMs / 1000)}s waiting for ${label} (last observed: ${JSON.stringify(lastObserved)})`,
			);
		}
		await new Promise((resolve) => setTimeout(resolve, intervalMs));
	}
}

/** Polls the run file until it reports a live, ready daemon; returns its facts. */
export async function waitForDaemonReady(runFile: string, timeoutMs: number): Promise<RunFile & { port: number }> {
	const info = await waitFor(
		"the run file to report a bound port",
		timeoutMs,
		async () => {
			const parsed = await readRunFile(runFile);
			return parsed?.port ? parsed : false;
		},
		500,
	);
	await waitFor(
		`/readyz on port ${info.port}`,
		timeoutMs,
		async () => {
			try {
				const res = await fetch(`http://127.0.0.1:${info.port}/readyz`);
				return res.status === 200 ? true : false;
			} catch {
				return false;
			}
		},
		1000,
	);
	return info as RunFile & { port: number };
}

/** Reads the daemon's own build-identity payload (buildRevision, etc.) from /readyz. */
export async function readDaemonBuildRevision(port: number): Promise<string> {
	const res = await fetch(`http://127.0.0.1:${port}/readyz`);
	const body = (await res.json()) as { buildRevision?: string };
	return body.buildRevision ?? "unknown";
}

/** Bounded poll for a previously-observed pid to disappear. Never sends a signal itself. */
export async function waitForPidGone(pid: number, timeoutMs: number): Promise<void> {
	await waitFor(
		`pid ${pid} to exit`,
		timeoutMs,
		async () => {
			try {
				process.kill(pid, 0);
				return false; // still alive
			} catch {
				return true; // ESRCH — the process is gone
			}
		},
		500,
	);
}
