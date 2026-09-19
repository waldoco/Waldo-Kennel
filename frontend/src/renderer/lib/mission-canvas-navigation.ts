/**
 * Topology-aware keyboard navigation for the mission canvas. Layers come from
 * the ELK layout itself (one x per layer, y order within a layer), so arrow
 * keys move the way the graph is drawn: Left/Right cross to the adjacent
 * dependency layer (nearest y, the way an eye tracks a join), Up/Down move
 * between siblings inside the same layer. Pure and DOM-free so the movement
 * rules are testable without React Flow.
 */

export type CanvasLayerEntry = { id: string; y: number };
export type CanvasLayer = { x: number; entries: CanvasLayerEntry[] };
export type CanvasNavigationDirection = "left" | "right" | "up" | "down";

export function buildCanvasLayers(positions: ReadonlyMap<string, { x: number; y: number }>): CanvasLayer[] {
	const byX = new Map<number, CanvasLayerEntry[]>();
	for (const [id, position] of positions) {
		const entries = byX.get(position.x) ?? [];
		entries.push({ id, y: position.y });
		byX.set(position.x, entries);
	}
	return [...byX.entries()]
		.sort((a, b) => a[0] - b[0])
		.map(([x, entries]) => ({ entries: entries.sort((a, b) => a.y - b.y), x }));
}

/** The node an arrow key should move focus to, or undefined when the graph
 *  ends in that direction (focus stays put - never wraps, never jumps). */
export function canvasTopologyNeighbor(
	layers: readonly CanvasLayer[],
	currentId: string,
	direction: CanvasNavigationDirection,
): string | undefined {
	const layerIndex = layers.findIndex((layer) => layer.entries.some((entry) => entry.id === currentId));
	if (layerIndex === -1) return undefined;
	const entries = layers[layerIndex].entries;
	const entryIndex = entries.findIndex((entry) => entry.id === currentId);
	if (direction === "up") return entries[entryIndex - 1]?.id;
	if (direction === "down") return entries[entryIndex + 1]?.id;
	const target = direction === "left" ? layers[layerIndex - 1] : layers[layerIndex + 1];
	if (!target) return undefined;
	const currentY = entries[entryIndex].y;
	let nearest: CanvasLayerEntry | undefined;
	for (const entry of target.entries) {
		if (!nearest || Math.abs(entry.y - currentY) < Math.abs(nearest.y - currentY)) nearest = entry;
	}
	return nearest?.id;
}

export type CanvasLineage = {
	/** The selected node plus every ancestor and dependent, both directions. */
	related: ReadonlySet<string>;
	/** Drawable edge ids ("from->to") with both endpoints inside the lineage. */
	relatedEdgeIds: ReadonlySet<string>;
};

/** The selected node's full dependency lineage for focus dimming, or
 *  undefined when nothing is selected (nothing dims). Pure: the canvas maps
 *  the result onto node/edge class names. */
export function canvasLineage(edges: readonly { from: string; to: string }[], selectedId: string | undefined): CanvasLineage | undefined {
	if (!selectedId) return undefined;
	const related = new Set<string>([selectedId]);
	const adjacency = (direction: "from" | "to") => {
		const map = new Map<string, string[]>();
		for (const edge of edges) {
			const key = direction === "from" ? edge.from : edge.to;
			const value = direction === "from" ? edge.to : edge.from;
			map.set(key, [...(map.get(key) ?? []), value]);
		}
		return map;
	};
	const walk = (adjacent: Map<string, string[]>) => {
		const queue = [selectedId];
		while (queue.length > 0) {
			const id = queue.shift() as string;
			for (const next of adjacent.get(id) ?? []) {
				if (!related.has(next)) {
					related.add(next);
					queue.push(next);
				}
			}
		}
	};
	walk(adjacency("from"));
	walk(adjacency("to"));
	const relatedEdgeIds = new Set(
		edges.filter((edge) => related.has(edge.from) && related.has(edge.to)).map((edge) => `${edge.from}->${edge.to}`),
	);
	return { related, relatedEdgeIds };
}
