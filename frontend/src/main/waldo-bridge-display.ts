// Native display for Waldo bridge notify_local. The daemon owns the bridge session but
// no UI, so it hands display-only text to this main-process loop over the
// authenticated loopback bridge API. The daemon reports "delivered" to Waldo only when
// this loop acknowledges shown=true, which happens only on the OS notification's
// "show" event, and never on macOS, where "show" is not display evidence; refusal,
// failure, timeout or a malformed request never claims it.

export type WaldoBridgeDisplayRequest = { requestId: string; title: string; body: string; severity: "info" | "warning" | "error" };

type NotificationLike = {
	once(event: "show" | "failed" | "close", listener: () => void): unknown;
	show(): void;
};
export type NotificationFactory = { isSupported(): boolean; create(options: { title: string; body: string }): NotificationLike };

export type WaldoBridgeDisplayDeps = {
	getDaemonConnection: () => { port: number } | null;
	bridgeLocalToken: () => string;
	fetch: typeof globalThis.fetch;
	show: (title: string, body: string) => Promise<boolean>;
	sleep?: (ms: number, signal: AbortSignal) => Promise<void>;
};

const SHOW_TIMEOUT_MS = 5_000;
const RETRY_MS = 2_000;
const UNMOUNTED_RETRY_MS = 30_000;
const RETAIN_MS = 5 * 60_000;

const utf8Bytes = (value: string) => Buffer.byteLength(value, "utf8");

/** Accepts exactly the daemon's display request shape and the contract's text bounds. */
export function parseDisplayRequest(raw: unknown): WaldoBridgeDisplayRequest | null {
	if (!raw || typeof raw !== "object" || Array.isArray(raw)) return null;
	const v = raw as Record<string, unknown>;
	if (Object.keys(v).sort().join(",") !== "body,request_id,severity,title") return null;
	const { request_id: requestId, title, body, severity } = v;
	if (typeof requestId !== "string" || !/^[0-9a-f]{32}$/.test(requestId)) return null;
	if (typeof title !== "string" || utf8Bytes(title) < 1 || utf8Bytes(title) > 120) return null;
	if (typeof body !== "string" || utf8Bytes(body) < 1 || utf8Bytes(body) > 1024) return null;
	if (severity !== "info" && severity !== "warning" && severity !== "error") return null;
	return { requestId, title, body, severity };
}

/**
 * Resolves true only when the OS reports the notification shown and that report is
 * evidence of display. On macOS it is not: Electron emits "show" even when the app's
 * notifications are disabled or Focus suppresses them (live check 2026-10-12, Electron
 * 33.4.11), so the banner is still shown but never claimed until a reliable signal exists.
 */
export function createNotificationShower(factory: NotificationFactory, timeoutMs = SHOW_TIMEOUT_MS, platform: NodeJS.Platform = process.platform) {
	const showIsDisplayEvidence = platform !== "darwin";
	// Hold references so a pending notification is not collected before it reports.
	const retained = new Set<NotificationLike>();
	return (title: string, body: string): Promise<boolean> => {
		if (!factory.isSupported()) return Promise.resolve(false);
		return new Promise((resolve) => {
			let settled = false;
			const notification = factory.create({ title, body });
			retained.add(notification);
			const release = setTimeout(() => retained.delete(notification), RETAIN_MS);
			const settle = (shown: boolean) => {
				if (settled) return;
				settled = true;
				clearTimeout(timer);
				resolve(shown);
			};
			const timer = setTimeout(() => settle(false), timeoutMs);
			notification.once("show", () => settle(showIsDisplayEvidence));
			notification.once("failed", () => settle(false));
			notification.once("close", () => {
				clearTimeout(release);
				retained.delete(notification);
			});
			try {
				notification.show();
			} catch {
				settle(false);
			}
		});
	};
}

const abortableSleep = (ms: number, signal: AbortSignal) =>
	new Promise<void>((resolve) => {
		if (signal.aborted) return resolve();
		const timer = setTimeout(resolve, ms);
		signal.addEventListener("abort", () => { clearTimeout(timer); resolve(); }, { once: true });
	});

export function createWaldoBridgeDisplayLoop(deps: WaldoBridgeDisplayDeps) {
	const sleep = deps.sleep ?? abortableSleep;
	let controller: AbortController | null = null;
	let running: Promise<void> | null = null;

	const pollOnce = async (signal: AbortSignal): Promise<number> => {
		const daemon = deps.getDaemonConnection();
		const token = deps.bridgeLocalToken();
		if (!daemon || !token) return RETRY_MS;
		const base = `http://127.0.0.1:${daemon.port}/internal/bridge/display`;
		const response = await deps.fetch(`${base}/next`, { method: "GET", headers: { Authorization: `KennelBridge ${token}` }, signal });
		if (response.status === 204) return 0;
		if (response.status === 404) return UNMOUNTED_RETRY_MS;
		if (!response.ok) return RETRY_MS;
		// A malformed request gets no answer; the daemon times out to delivery_unknown.
		const request = parseDisplayRequest(await response.json().catch(() => null));
		if (!request) return RETRY_MS;
		const shown = await deps.show(request.title, request.body).catch(() => false);
		await deps.fetch(`${base}/ack`, {
			method: "POST",
			headers: { "Content-Type": "application/json", Authorization: `KennelBridge ${token}` },
			body: JSON.stringify({ request_id: request.requestId, shown }),
			signal,
		});
		return 0;
	};

	return {
		start(): void {
			if (controller) return;
			const current = new AbortController();
			controller = current;
			running = (async () => {
				while (!current.signal.aborted) {
					let wait: number;
					try {
						wait = await pollOnce(current.signal);
					} catch {
						wait = RETRY_MS;
					}
					if (wait > 0) await sleep(wait, current.signal);
				}
			})();
		},
		async stop(): Promise<void> {
			const current = controller;
			controller = null;
			current?.abort();
			await running;
			running = null;
		},
	};
}
