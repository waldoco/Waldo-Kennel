import { beforeEach, describe, expect, it, vi } from "vitest";

import { setCanvasLayoutHooksForTests } from "./mission-canvas-layout";

import { dummyMissionProjection, dummyPlanRevision, dummySchedule } from "./mission-canvas-dummy";
import {
	MISSION_CANVAS_NODE_HEIGHT,
	MISSION_CANVAS_NODE_WIDTH,
	buildElkGraph,
	cachedCanvasPositions,
	extractPositions,
	requestCanvasLayout,
	resetCanvasLayoutStateForTests,
} from "./mission-canvas-layout";
import { modelFromMissionProjection, modelFromPlan, type MissionCanvasModel } from "./mission-canvas-model";

function tinyModel(key: string): MissionCanvasModel {
	return {
		nodes: [
			{ workUnitId: `${key}-a`, title: "A", state: "runnable", upstream: [], criterionReadiness: { available: false }, criterionTotal: 0, isNextRunnable: false, holdsCustody: false },
			{ workUnitId: `${key}-b`, title: "B", state: "blocked", upstream: [`${key}-a`], criterionReadiness: { available: false }, criterionTotal: 0, isNextRunnable: false, holdsCustody: false },
		],
		edges: [{ from: `${key}-a`, to: `${key}-b` }],
		topologyKey: key,
		source: "projection",
	};
}

beforeEach(() => {
	resetCanvasLayoutStateForTests();
});

describe("buildElkGraph", () => {
	it("builds a layered RIGHT graph with explicit west/east ports from the dummy projection", () => {
		const graph = buildElkGraph(modelFromMissionProjection(dummyMissionProjection()));
		expect(graph.layoutOptions["elk.algorithm"]).toBe("layered");
		expect(graph.layoutOptions["elk.direction"]).toBe("RIGHT");
		expect(graph.children).toHaveLength(8);
		for (const child of graph.children) {
			expect(child.width).toBe(MISSION_CANVAS_NODE_WIDTH);
			expect(child.height).toBe(MISSION_CANVAS_NODE_HEIGHT);
			expect(child.ports.map((port) => port.properties["port.side"])).toEqual(["WEST", "EAST"]);
		}
		expect(graph.edges.length).toBeGreaterThan(0);
		for (const edge of graph.edges) {
			expect(edge.sources[0]).toMatch(/\/out$/);
			expect(edge.targets[0]).toMatch(/\/in$/);
		}
	});

	it("drops edges whose endpoint is absent from the node set - never sends an undrawable edge to ELK", () => {
		const model = tinyModel("dangling");
		model.edges.push({ from: "dangling-a", to: "ghost" });
		const graph = buildElkGraph(model);
		expect(graph.edges).toHaveLength(1);
		expect(graph.edges[0].id).toBe("dangling-a->dangling-b");
	});
});

describe("extractPositions", () => {
	it("maps ELK children to positions and defaults unplaced nodes to the origin", () => {
		const positions = extractPositions({ children: [{ id: "a", x: 12, y: 34 }, { id: "b" }] });
		expect(positions.get("a")).toEqual({ x: 12, y: 34 });
		expect(positions.get("b")).toEqual({ x: 0, y: 0 });
	});
});

describe("requestCanvasLayout", () => {
	// jsdom has no Worker, so these exercise the honest main-thread fallback -
	// real ELK layout, same code path the worker answers with.

	it("lays out the dummy projection: every node placed, sources left of targets", async () => {
		const model = modelFromMissionProjection(dummyMissionProjection());
		const result = await requestCanvasLayout(model);
		expect(result.status).toBe("ready");
		if (result.status !== "ready") return;
		expect(result.positions.size).toBe(8);
		for (const edge of model.edges) {
			expect(result.positions.get(edge.from)!.x).toBeLessThan(result.positions.get(edge.to)!.x);
		}
	});

	it("caches by topologyKey: a repeat request is the same Map, never a relayout", async () => {
		const model = modelFromMissionProjection(dummyMissionProjection());
		const first = await requestCanvasLayout(model);
		const second = await requestCanvasLayout(model);
		expect(first.status).toBe("ready");
		expect(second.status).toBe("ready");
		if (first.status === "ready" && second.status === "ready") {
			expect(second.positions).toBe(first.positions);
		}
		expect(cachedCanvasPositions(model.topologyKey)).toBeDefined();
	});

	it("keeps concurrent consumers independent when two uncached topologies resolve out of order", async () => {
		const resolvers: ((value: unknown) => void)[] = [];
		setCanvasLayoutHooksForTests({
			elkLoader: async () => ({
				default: class {
					layout(graph: { children: { id: string }[] }): Promise<unknown> {
						return new Promise((resolve) => resolvers.push(() => resolve({ children: graph.children.map((child, index) => ({ ...child, x: index * 10, y: 0 })) })));
					}
				},
			}),
		});
		const earlier = requestCanvasLayout(tinyModel("earlier-canvas"));
		const later = requestCanvasLayout(tinyModel("later-canvas"));
		await vi.waitFor(() => expect(resolvers).toHaveLength(2));
		resolvers[1]({});
		const laterResult = await later;
		resolvers[0]({});
		const earlierResult = await earlier;
		expect(earlierResult.status).toBe("ready");
		expect(laterResult.status).toBe("ready");
		expect(cachedCanvasPositions("earlier-canvas")).toBeDefined();
		expect(cachedCanvasPositions("later-canvas")).toBeDefined();
	});

	it("lays out a proposed plan model through the same path", async () => {
		const model = modelFromPlan(dummyPlanRevision(), dummySchedule());
		const result = await requestCanvasLayout(model);
		expect(result.status).toBe("ready");
		if (result.status === "ready") expect(result.positions.size).toBe(8);
	});

	it("settles on a fatal worker error (crash before reply) via the main-thread retry, never hanging", async () => {
		class FatalWorker {
			onmessage: ((event: MessageEvent) => void) | null = null;
			onerror: ((event: { message?: string }) => void) | null = null;
			onmessageerror: (() => void) | null = null;
			postMessage(): void {
				// Crash before answering any layout.
				queueMicrotask(() => this.onerror?.({ message: "worker script failed" }));
			}
			terminate(): void {}
		}
		setCanvasLayoutHooksForTests({ workerCtor: FatalWorker as unknown as new (url: string) => Worker });
		const result = await requestCanvasLayout(tinyModel("fatal-worker"));
		expect(result.status).toBe("ready");
		if (result.status === "ready") expect(result.positions.size).toBe(2);
	});

	it("settles on a worker that never replies: the bounded timeout rejects and retries on the main thread", async () => {
		class SilentWorker {
			onmessage: ((event: MessageEvent) => void) | null = null;
			onerror: ((event: { message?: string }) => void) | null = null;
			onmessageerror: (() => void) | null = null;
			postMessage(): void {}
			terminate(): void {}
		}
		setCanvasLayoutHooksForTests({
			workerCtor: SilentWorker as unknown as new (url: string) => Worker,
			layoutTimeoutMs: 25,
		});
		const started = Date.now();
		const result = await requestCanvasLayout(tinyModel("silent-worker"));
		expect(result.status).toBe("ready");
		expect(Date.now() - started).toBeLessThan(5000);
	});

	it("terminates a worker whose protocol rejects before retrying on the main thread", async () => {
		let terminated = 0;
		let construction = 0;
		class RejectingWorker {
			onmessage: ((event: MessageEvent) => void) | null = null;
			onerror: ((event: { message?: string }) => void) | null = null;
			onmessageerror: (() => void) | null = null;
			postMessage(): void {}
			terminate(): void {
				terminated += 1;
			}
		}
		setCanvasLayoutHooksForTests({
			workerCtor: RejectingWorker as unknown as new (url: string) => Worker,
			elkLoader: async () => ({
				default: class {
					private readonly workerBacked = construction++ === 0;
					layout(graph: { children: { id: string }[] }): Promise<unknown> {
						if (this.workerBacked) return Promise.reject(new Error("worker protocol rejected"));
						return Promise.resolve({ children: graph.children.map((child) => ({ ...child, x: 0, y: 0 })) });
					}
				},
			}),
		});
		const result = await requestCanvasLayout(tinyModel("protocol-rejection"));
		expect(result.status).toBe("ready");
		expect(terminated).toBe(1);
	});

	it("propagates when the worker path AND the main-thread retry both fail", async () => {
		class FatalWorker {
			onmessage: ((event: MessageEvent) => void) | null = null;
			onerror: ((event: { message?: string }) => void) | null = null;
			onmessageerror: (() => void) | null = null;
			postMessage(): void {
				queueMicrotask(() => this.onerror?.({ message: "worker script failed" }));
			}
			terminate(): void {}
		}
		setCanvasLayoutHooksForTests({
			workerCtor: FatalWorker as unknown as new (url: string) => Worker,
			elkLoader: async () => ({
				default: class {
					layout(): Promise<unknown> {
						return Promise.reject(new Error("engine unavailable"));
					}
				},
			}),
		});
		await expect(requestCanvasLayout(tinyModel("both-fail"))).rejects.toThrow("engine unavailable");
		expect(cachedCanvasPositions("both-fail")).toBeUndefined();
	});

	it("evicts the oldest cached topology past the cache bound", async () => {
		await requestCanvasLayout(tinyModel("topology-0"));
		for (let index = 1; index <= 8; index += 1) {
			await requestCanvasLayout(tinyModel(`topology-${index}`));
		}
		expect(cachedCanvasPositions("topology-0")).toBeUndefined();
		expect(cachedCanvasPositions("topology-8")).toBeDefined();
	});
});
