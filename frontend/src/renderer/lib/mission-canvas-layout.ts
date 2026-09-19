import type { MissionCanvasModel } from "./mission-canvas-model";

/**
 * ELK layered layout for the mission canvas, computed off the renderer
 * thread in a Web Worker and cached by the model's topologyKey. The cache is
 * the stability rule in code: a state-only refresh reuses one topologyKey, so
 * positions never recompute and the viewport never moves; only a real
 * topology swap (new fingerprint) pays for layout.
 *
 * A layout request that resolves after a newer request for a DIFFERENT
 * topology was issued answers "stale" and is never cached or applied - a
 * topology swap can never paint a mixed revision.
 *
 * When Worker is unavailable (tests, non-DOM runtimes) the same ELK graph is
 * laid out on the main thread via a lazily imported bundled ELK. The pure
 * graph builder and the cache are shared by both paths.
 */

export const MISSION_CANVAS_NODE_WIDTH = 256;
export const MISSION_CANVAS_NODE_HEIGHT = 84;

export type CanvasPosition = { x: number; y: number };
export type CanvasPositions = ReadonlyMap<string, CanvasPosition>;

export type ElkGraphSpec = {
	id: string;
	layoutOptions: Record<string, string>;
	children: { id: string; width: number; height: number; ports: { id: string; properties: Record<string, string> }[] }[];
	edges: { id: string; sources: string[]; targets: string[] }[];
};

/** Pure: model -> ELK JSON. Edges with an endpoint absent from the node set
 *  are dropped here too - an edge the graph cannot draw is never sent to the
 *  layout engine. Deterministic: input model edges are already sorted. */
export function buildElkGraph(model: MissionCanvasModel): ElkGraphSpec {
	const nodeIds = new Set(model.nodes.map((node) => node.workUnitId));
	return {
		id: "mission",
		layoutOptions: {
			"elk.algorithm": "layered",
			"elk.direction": "RIGHT",
			"elk.edgeRouting": "ORTHOGONAL",
			// Stable placement across state refreshes starts with honoring the
			// model's node order wherever crossings allow.
			"elk.layered.considerModelOrder.strategy": "NODES_AND_EDGES",
			"elk.layered.spacing.nodeNodeBetweenLayers": "96",
			"elk.spacing.nodeNode": "36",
			"elk.layered.spacing.edgeNodeBetweenLayers": "24",
			"elk.hierarchyHandling": "INCLUDE_CHILDREN",
		},
		children: model.nodes.map((node) => ({
			id: node.workUnitId,
			width: MISSION_CANVAS_NODE_WIDTH,
			height: MISSION_CANVAS_NODE_HEIGHT,
			ports: [
				{ id: `${node.workUnitId}/in`, properties: { "port.side": "WEST" } },
				{ id: `${node.workUnitId}/out`, properties: { "port.side": "EAST" } },
			],
		})),
		edges: model.edges
			.filter((edge) => nodeIds.has(edge.from) && nodeIds.has(edge.to))
			.map((edge) => ({
				id: `${edge.from}->${edge.to}`,
				sources: [`${edge.from}/out`],
				targets: [`${edge.to}/in`],
			})),
	};
}

type LaidOutElkNode = {
	children?: { id: string; x?: number; y?: number }[];
};

/** Pure: ELK result -> positions. A node ELK could not place lands at the
 *  origin deterministically rather than being dropped (it stays visible and
 *  selectable; overlap is a layout defect, invisibility is a truth defect). */
export function extractPositions(laid: LaidOutElkNode): Map<string, CanvasPosition> {
	const positions = new Map<string, CanvasPosition>();
	for (const child of laid.children ?? []) {
		positions.set(child.id, { x: child.x ?? 0, y: child.y ?? 0 });
	}
	return positions;
}

const positionCache = new Map<string, CanvasPositions>();
const MAX_CACHED_TOPOLOGIES = 8;

export function cachedCanvasPositions(topologyKey: string): CanvasPositions | undefined {
	return positionCache.get(topologyKey);
}

function cachePositions(topologyKey: string, positions: CanvasPositions): void {
	// Bounded cache: oldest key evicted first. Plan swaps are rare and usually
	// revisit at most one prior revision, so a small Map is enough.
	if (positionCache.size >= MAX_CACHED_TOPOLOGIES) {
		const oldest = positionCache.keys().next().value;
		if (oldest !== undefined) positionCache.delete(oldest);
	}
	positionCache.set(topologyKey, positions);
}

export type CanvasLayoutResult = { status: "ready"; positions: CanvasPositions } | { status: "stale" };

type ElkLike = { layout(graph: unknown): Promise<unknown> };
type ElkModule = { default: new (options?: Record<string, unknown>) => ElkLike };

/** Injectable seams (tests only): the worker constructor, the ELK module
 *  loader, and the per-layout timeout. Production defaults are the real
 *  Worker, elkjs bundled, and a generous bound for a few hundred nodes. */
let workerCtor: (new (url: string) => Worker) | undefined;
let elkLoader: () => Promise<ElkModule> = () => import("elkjs/lib/elk.bundled.js") as Promise<ElkModule>;
let layoutTimeoutMs = 15000;

const DEFAULT_LAYOUT_TIMEOUT_MS = 15000;

type ElkInstance = {
	elk: ElkLike;
	worker?: Worker;
	/** Fatal worker notifications this instance broadcasts to every in-flight
	 *  layout. elkjs's PromisedWorker only sets onmessage - it has no
	 *  onerror/onmessageerror/timeout - so this boundary owns the worker
	 *  lifecycle: a worker-script load failure, a crash before reply, a
	 *  malformed message, or an external termination settles every pending
	 *  request instead of hanging the canvas forever. */
	fatalHandlers: Set<(error: Error) => void>;
};

/**
 * One shared ELK instance. With a real Worker available it runs elkjs's own
 * worker protocol (elk-worker.min.js, vite-served asset) so layout stays off
 * the renderer thread; without one (tests, non-DOM runtimes) the bundled ELK
 * computes on the main thread. A fatal worker event disables the worker path
 * for the session and the failed layout retries once on bundled main-thread
 * ELK - an empty canvas is never the answer to a layout-engine problem.
 */
let elkInstance: ElkInstance | undefined;
let workerDisabled = false;
let requestSeq = 0;

function fatalWorkerError(instance: ElkInstance, error: Error): void {
	workerDisabled = true;
	try {
		instance.worker?.terminate();
	} catch {
		// Termination is best-effort; the instance is already dead to us.
	}
	if (elkInstance === instance) elkInstance = undefined;
	for (const handler of instance.fatalHandlers) handler(error);
	instance.fatalHandlers.clear();
}

async function createElk(): Promise<ElkInstance> {
	const { default: ELK } = await elkLoader();
	const ctor = workerCtor ?? (typeof Worker !== "undefined" ? Worker : undefined);
	if (!workerDisabled && ctor) {
		try {
			const workerUrl = (await import("elkjs/lib/elk-worker.min.js?url")).default;
			const instance: ElkInstance = { elk: undefined as unknown as ElkLike, fatalHandlers: new Set() };
			const worker = new ctor(workerUrl);
			worker.onerror = (event) => {
				fatalWorkerError(instance, new Error(event.message || "Mission graph layout worker failed"));
			};
			worker.onmessageerror = () => {
				fatalWorkerError(instance, new Error("Mission graph layout worker returned an unreadable message"));
			};
			instance.worker = worker;
			instance.elk = new ELK({ workerFactory: () => worker });
			return instance;
		} catch {
			// Worker construction unsupported here - fall through to the
			// bundled main-thread engine.
		}
	}
	return { elk: new ELK(), fatalHandlers: new Set() };
}

async function getElk(): Promise<ElkInstance> {
	if (!elkInstance) elkInstance = await createElk();
	return elkInstance;
}

/** One layout against one instance, guarded: a bounded timeout and the
 *  instance's fatal broadcast both reject the outstanding promise, so no
 *  request can outlive the worker meant to answer it. */
function layoutWithGuards(instance: ElkInstance, graph: unknown): Promise<unknown> {
	return new Promise((resolve, reject) => {
		const cleanup = () => {
			clearTimeout(timer);
			instance.fatalHandlers.delete(onFatal);
		};
		const onFatal = (error: Error) => {
			cleanup();
			reject(error);
		};
		const timer = setTimeout(() => {
			cleanup();
			// A timed-out worker is treated as failed: the same fallback path
			// applies, and the next request does not inherit a stuck engine.
			fatalWorkerError(instance, new Error("Mission graph layout timed out"));
			reject(new Error("Mission graph layout timed out"));
		}, layoutTimeoutMs);
		instance.fatalHandlers.add(onFatal);
		instance.elk.layout(graph).then(
			(laid) => {
				cleanup();
				resolve(laid);
			},
			(layoutError: unknown) => {
				cleanup();
				reject(layoutError instanceof Error ? layoutError : new Error(String(layoutError)));
			},
		);
	});
}

/** Request positions for one topology. Resolves "stale" when a newer request
 *  for a different topology superseded this one before it finished. Results
 *  are cached by topologyKey, so a repeat request for a seen topology is a
 *  Map read, never a layout. Throws only when both the worker path and the
 *  main-thread retry fail - the caller surfaces that as an honest error. */
export async function requestCanvasLayout(model: MissionCanvasModel): Promise<CanvasLayoutResult> {
	const cached = cachedCanvasPositions(model.topologyKey);
	if (cached) return { status: "ready", positions: cached };

	const seq = ++requestSeq;
	const isCurrent = () => seq === requestSeq;
	const graph = buildElkGraph(model);

	const instance = await getElk();
	const usedWorker = Boolean(instance.worker);
	try {
		const laid = await layoutWithGuards(instance, graph);
		if (!isCurrent()) return { status: "stale" };
		const positions = extractPositions(laid as LaidOutElkNode);
		cachePositions(model.topologyKey, positions);
		return { status: "ready", positions };
	} catch (error) {
		// One retry on the bundled main-thread engine when the WORKER path
		// just failed (fatal event, timeout, or protocol rejection). A failure
		// with no worker involved, or a failed retry, propagates so the canvas
		// can say so instead of hanging.
		if (usedWorker) {
			workerDisabled = true;
			elkInstance = undefined;
			const laid = await layoutWithGuards(await getElk(), graph);
			if (!isCurrent()) return { status: "stale" };
			const positions = extractPositions(laid as LaidOutElkNode);
			cachePositions(model.topologyKey, positions);
			return { status: "ready", positions };
		}
		throw error;
	}
}

/** Test hook: clears the module-level cache and worker state between specs. */
export function resetCanvasLayoutStateForTests(): void {
	positionCache.clear();
	elkInstance = undefined;
	workerDisabled = false;
	requestSeq = 0;
	workerCtor = undefined;
	elkLoader = () => import("elkjs/lib/elk.bundled.js") as Promise<ElkModule>;
	layoutTimeoutMs = DEFAULT_LAYOUT_TIMEOUT_MS;
}

/** Test hook: replaces the worker constructor, the ELK module loader, and/or
 *  the layout timeout. resetCanvasLayoutStateForTests restores all defaults. */
export function setCanvasLayoutHooksForTests(hooks: {
	workerCtor?: new (url: string) => Worker;
	elkLoader?: () => Promise<ElkModule>;
	layoutTimeoutMs?: number;
}): void {
	if (hooks.workerCtor !== undefined) workerCtor = hooks.workerCtor;
	if (hooks.elkLoader !== undefined) elkLoader = hooks.elkLoader;
	if (hooks.layoutTimeoutMs !== undefined) layoutTimeoutMs = hooks.layoutTimeoutMs;
}
