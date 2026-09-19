import { describe, expect, it } from "vitest";

import { buildCanvasLayers, canvasLineage, canvasTopologyNeighbor } from "./mission-canvas-navigation";

/**
 * The diamond used throughout: compile -> {bind, refuse} -> sweep. ELK layered
 * lays predecessors to the LEFT, so layer x grows with dependency depth.
 */
const positions = new Map<string, { x: number; y: number }>([
	["wu-compile", { x: 0, y: 100 }],
	["wu-bind", { x: 260, y: 40 }],
	["wu-refuse", { x: 260, y: 160 }],
	["wu-sweep", { x: 520, y: 100 }],
]);

describe("buildCanvasLayers", () => {
	it("groups nodes by layer x and orders each layer by y", () => {
		const layers = buildCanvasLayers(positions);
		expect(layers.map((layer) => layer.x)).toEqual([0, 260, 520]);
		expect(layers[1].entries.map((entry) => entry.id)).toEqual(["wu-bind", "wu-refuse"]);
	});
});

describe("canvasTopologyNeighbor", () => {
	const layers = buildCanvasLayers(positions);

	it("moves left/right across layers to the nearest y, the way an eye tracks a join", () => {
		expect(canvasTopologyNeighbor(layers, "wu-compile", "right")).toBe("wu-bind");
		expect(canvasTopologyNeighbor(layers, "wu-sweep", "left")).toBe("wu-bind");
		expect(canvasTopologyNeighbor(layers, "wu-refuse", "right")).toBe("wu-sweep");
		expect(canvasTopologyNeighbor(layers, "wu-bind", "left")).toBe("wu-compile");
	});

	it("moves up/down between siblings inside one layer", () => {
		expect(canvasTopologyNeighbor(layers, "wu-bind", "down")).toBe("wu-refuse");
		expect(canvasTopologyNeighbor(layers, "wu-refuse", "up")).toBe("wu-bind");
	});

	it("never wraps and never jumps: the graph edge keeps focus put", () => {
		expect(canvasTopologyNeighbor(layers, "wu-compile", "left")).toBeUndefined();
		expect(canvasTopologyNeighbor(layers, "wu-sweep", "right")).toBeUndefined();
		expect(canvasTopologyNeighbor(layers, "wu-bind", "up")).toBeUndefined();
		expect(canvasTopologyNeighbor(layers, "wu-refuse", "down")).toBeUndefined();
	});

	it("returns undefined for a node the layout does not know", () => {
		expect(canvasTopologyNeighbor(layers, "wu-missing", "right")).toBeUndefined();
	});
});

describe("canvasLineage", () => {
	// The dummy scenario's drawable edges.
	const edges = [
		{ from: "wu-schema", to: "wu-binding" },
		{ from: "wu-schema", to: "wu-handshake" },
		{ from: "wu-binding", to: "wu-sweep" },
		{ from: "wu-binding", to: "wu-island" },
		{ from: "wu-handshake", to: "wu-island" },
		{ from: "wu-handshake", to: "wu-tests" },
		{ from: "wu-sweep", to: "wu-docs" },
		{ from: "wu-sweep", to: "wu-doctor" },
		{ from: "wu-island", to: "wu-doctor" },
		{ from: "wu-tests", to: "wu-doctor" },
	];

	it("collects ancestors and dependents in both directions", () => {
		const lineage = canvasLineage(edges, "wu-binding");
		expect(lineage).toBeDefined();
		expect([...lineage!.related].sort()).toEqual(["wu-binding", "wu-docs", "wu-doctor", "wu-island", "wu-schema", "wu-sweep"]);
		expect(lineage!.related.has("wu-handshake")).toBe(false);
		expect(lineage!.related.has("wu-tests")).toBe(false);
	});

	it("lights exactly the edges with both endpoints inside the lineage", () => {
		const lineage = canvasLineage(edges, "wu-binding")!;
		expect([...lineage.relatedEdgeIds].sort()).toEqual([
			"wu-binding->wu-island",
			"wu-binding->wu-sweep",
			"wu-island->wu-doctor",
			"wu-schema->wu-binding",
			"wu-sweep->wu-docs",
			"wu-sweep->wu-doctor",
		]);
	});

	it("is undefined with no selection, so nothing dims", () => {
		expect(canvasLineage(edges, undefined)).toBeUndefined();
	});
});
