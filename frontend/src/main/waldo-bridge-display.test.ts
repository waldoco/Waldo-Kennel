import { EventEmitter } from "node:events";
import { describe, expect, it, vi } from "vitest";
import { createNotificationShower, createWaldoBridgeDisplayLoop, parseDisplayRequest } from "./waldo-bridge-display";

const id = "0123456789abcdef0123456789abcdef";
const neutral = { request_id: id, title: "Waldo status", body: "Your Mac is connected.", severity: "info" };

describe("parseDisplayRequest", () => {
	it("accepts exactly the daemon shape within contract bounds", () => {
		expect(parseDisplayRequest(neutral)).toEqual({ requestId: id, title: "Waldo status", body: "Your Mac is connected.", severity: "info" });
	});
	it("rejects anything else", () => {
		for (const bad of [
			null, [], "x",
			{ ...neutral, extra: 1 },
			{ title: neutral.title, body: neutral.body, severity: neutral.severity },
			{ ...neutral, request_id: id.toUpperCase() },
			{ ...neutral, request_id: "short" },
			{ ...neutral, title: "" },
			{ ...neutral, title: "x".repeat(121) },
			{ ...neutral, body: "x".repeat(1025) },
			{ ...neutral, severity: "critical" },
		]) expect(parseDisplayRequest(bad)).toBeNull();
	});
});

class FakeNotification extends EventEmitter {
	shown = 0;
	constructor(readonly options: { title: string; body: string }, private readonly outcome: "show" | "failed" | "silent" | "throw") { super(); }
	show() {
		this.shown++;
		if (this.outcome === "throw") throw new Error("boom");
		if (this.outcome !== "silent") queueMicrotask(() => this.emit(this.outcome));
	}
}
const factory = (outcome: "show" | "failed" | "silent" | "throw", supported = true) => {
	const created: FakeNotification[] = [];
	return { created, factory: { isSupported: () => supported, create: (options: { title: string; body: string }) => { const n = new FakeNotification(options, outcome); created.push(n); return n; } } };
};

describe("createNotificationShower", () => {
	it("reports shown only on the OS show event, with the exact text", async () => {
		const f = factory("show");
		await expect(createNotificationShower(f.factory)("Waldo status", "Your Mac is connected.")).resolves.toBe(true);
		expect(f.created[0].options).toEqual({ title: "Waldo status", body: "Your Mac is connected." });
	});
	it("never claims display on failure, timeout, exception or missing OS support", async () => {
		await expect(createNotificationShower(factory("failed").factory)("t", "b")).resolves.toBe(false);
		await expect(createNotificationShower(factory("silent").factory, 20)("t", "b")).resolves.toBe(false);
		await expect(createNotificationShower(factory("throw").factory)("t", "b")).resolves.toBe(false);
		const unsupported = factory("show", false);
		await expect(createNotificationShower(unsupported.factory)("t", "b")).resolves.toBe(false);
		expect(unsupported.created).toHaveLength(0);
	});
});

type Call = { url: string; init: RequestInit };
function daemon(responses: Array<() => Response>) {
	const calls: Call[] = [];
	const fetch = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
		calls.push({ url: String(url), init: init ?? {} });
		const next = responses.shift();
		if (!next) {
			// Park further polls until the loop is stopped.
			await new Promise<void>((resolve) => init?.signal?.addEventListener("abort", () => resolve(), { once: true }));
			throw new Error("aborted");
		}
		return next();
	});
	return { calls, fetch: fetch as unknown as typeof globalThis.fetch };
}
const json = (body: unknown) => () => new Response(JSON.stringify(body), { status: 200, headers: { "Content-Type": "application/json" } });
const empty = (status: number) => () => new Response(null, { status });
const settle = () => new Promise((resolve) => setTimeout(resolve, 20));

describe("createWaldoBridgeDisplayLoop", () => {
	const base = { getDaemonConnection: () => ({ port: 4321 }), bridgeLocalToken: () => "tok" };

	it("shows a requested notification and acknowledges the OS answer", async () => {
		for (const shown of [true, false]) {
			const d = daemon([empty(204), json(neutral), empty(204)]);
			const show = vi.fn(async () => shown);
			const loop = createWaldoBridgeDisplayLoop({ ...base, fetch: d.fetch, show, sleep: async () => {} });
			loop.start();
			await settle();
			await loop.stop();
			expect(show).toHaveBeenCalledExactlyOnceWith("Waldo status", "Your Mac is connected.");
			const ack = d.calls.find((c) => c.url.endsWith("/display/ack"));
			expect(ack?.url).toBe("http://127.0.0.1:4321/internal/bridge/display/ack");
			expect(ack?.init.method).toBe("POST");
			expect(JSON.parse(String(ack?.init.body))).toEqual({ request_id: id, shown });
			for (const call of d.calls) expect((call.init.headers as Record<string, string>).Authorization).toBe("KennelBridge tok");
		}
	});

	it("never shows or acknowledges a malformed request", async () => {
		const d = daemon([json({ ...neutral, extra: true })]);
		const show = vi.fn(async () => true);
		const loop = createWaldoBridgeDisplayLoop({ ...base, fetch: d.fetch, show, sleep: async () => {} });
		loop.start();
		await settle();
		await loop.stop();
		expect(show).not.toHaveBeenCalled();
		expect(d.calls.some((c) => c.url.endsWith("/display/ack"))).toBe(false);
	});

	it("backs off when the bridge is not mounted or the daemon is unavailable", async () => {
		const waits: number[] = [];
		const d = daemon([empty(404)]);
		const loop = createWaldoBridgeDisplayLoop({ ...base, fetch: d.fetch, show: async () => true, sleep: async (ms) => { waits.push(ms); } });
		loop.start();
		await settle();
		await loop.stop();
		expect(waits[0]).toBe(30_000);

		const offline = vi.fn();
		const idle = createWaldoBridgeDisplayLoop({ getDaemonConnection: () => null, bridgeLocalToken: () => "tok", fetch: offline as unknown as typeof globalThis.fetch, show: async () => true });
		idle.start();
		await settle();
		await idle.stop();
		expect(offline).not.toHaveBeenCalled();
	});
});
