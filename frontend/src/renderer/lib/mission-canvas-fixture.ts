import { layerByDependency } from "./dependency-layers";

/**
 * Read-only fixture data for the React Flow spike (F0). This is deliberately
 * NOT wired to any daemon DTO: the spike evaluates a rendering library against
 * a realistic shape, not a production data path.
 *
 * States reuse the vocabulary MissionWorkUnitGraph already renders, so a
 * reviewer comparing the spike to production is comparing the same state
 * space rather than an invented one.
 */
export type MissionCanvasNodeState = "proven" | "executing" | "paused" | "retryable" | "blocked" | "runnable";

export type MissionCanvasFixtureNode = {
	id: string;
	title: string;
	state: MissionCanvasNodeState;
	/** Ids that must finish before this node starts. */
	upstream: string[];
	blockedReason?: string;
};

export type MissionCanvasFixtureEdge = {
	id: string;
	source: string;
	target: string;
};

const STATES: readonly MissionCanvasNodeState[] = ["proven", "executing", "paused", "retryable", "blocked", "runnable"];

// Deterministic word lists so titles read as plausible WorkUnits without any
// randomness — a fixture has to render byte-identical nodes on every run for
// the "stable IDs" test to mean anything.
const VERBS = ["Parse", "Persist", "Validate", "Migrate", "Render", "Schedule", "Reconcile", "Document", "Provision", "Verify"];
const NOUNS = ["config", "rows", "schema", "worktree", "manifest", "checkpoint", "credentials", "fixture", "artifact", "revision"];

export const MISSION_CANVAS_FIXTURE_NODE_COUNT = 75;

// Layer widths sum to MISSION_CANVAS_FIXTURE_NODE_COUNT: a narrow start and
// end with a wide middle, the shape a real Plan DAG tends to fan out into.
const LAYER_WIDTHS = [1, 4, 8, 10, 12, 12, 10, 8, 6, 4] as const;

function titleFor(layerIndex: number, indexInLayer: number): string {
	const verb = VERBS[(layerIndex * 3 + indexInLayer) % VERBS.length];
	const noun = NOUNS[(layerIndex + indexInLayer * 2) % NOUNS.length];
	return `${verb} ${noun} ${layerIndex}.${indexInLayer}`;
}

function stateFor(layerIndex: number, indexInLayer: number): MissionCanvasNodeState {
	return STATES[(layerIndex + indexInLayer) % STATES.length];
}

/**
 * Builds the 75-node fixture in dependency order (layer by layer, so the
 * array order already matches keyboard/tab order). Every node in layer L>0
 * depends on one or two deterministically chosen nodes from layer L-1, so
 * the graph is realistic (branching, converging) without being random.
 */
export function buildMissionCanvasFixture(): MissionCanvasFixtureNode[] {
	const layers: MissionCanvasFixtureNode[][] = [];
	let previousLayer: MissionCanvasFixtureNode[] = [];

	LAYER_WIDTHS.forEach((width, layerIndex) => {
		const layer: MissionCanvasFixtureNode[] = [];
		for (let indexInLayer = 0; indexInLayer < width; indexInLayer += 1) {
			const id = `wu-${layerIndex}-${indexInLayer}`;
			const upstream: string[] = [];
			if (previousLayer.length > 0) {
				const primary = previousLayer[indexInLayer % previousLayer.length];
				upstream.push(primary.id);
				// Roughly a third of nodes converge two upstream branches.
				if (indexInLayer % 3 === 0 && previousLayer.length > 1) {
					const secondary = previousLayer[(indexInLayer + 1) % previousLayer.length];
					if (secondary.id !== primary.id) upstream.push(secondary.id);
				}
			}
			const state = stateFor(layerIndex, indexInLayer);
			layer.push({
				id,
				title: titleFor(layerIndex, indexInLayer),
				state,
				upstream,
				blockedReason: state === "blocked" ? "awaiting_dependency_proof" : undefined,
			});
		}
		layers.push(layer);
		previousLayer = layer;
	});

	return layers.flat();
}

/** Derives dependency edges from a node list's own `upstream` fields. */
export function buildMissionCanvasEdges(nodes: MissionCanvasFixtureNode[]): MissionCanvasFixtureEdge[] {
	return nodes.flatMap((node) => node.upstream.map((from) => ({ id: `${from}->${node.id}`, source: from, target: node.id })));
}

/** Layers the fixture using the same longest-path layering the production graphs share. */
export function layerMissionCanvasFixture(nodes: MissionCanvasFixtureNode[]): MissionCanvasFixtureNode[][] {
	return layerByDependency(nodes);
}
