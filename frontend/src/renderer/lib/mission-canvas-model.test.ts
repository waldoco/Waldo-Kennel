import { describe, expect, it } from "vitest";

import {
	dummyAttempts,
	dummyMissionProjection,
	dummyPlanRevision,
	dummySchedule,
} from "./mission-canvas-dummy";
import { modelFromMissionProjection, modelFromPlan } from "./mission-canvas-model";

describe("modelFromMissionProjection", () => {
	it("maps the canonical projection into canvas nodes with live execution truth", () => {
		const model = modelFromMissionProjection(dummyMissionProjection());
		expect(model.source).toBe("projection");
		expect(model.topologyKey).toBe("topology-resume-race-3");
		expect(model.nodes).toHaveLength(8);

		const byId = new Map(model.nodes.map((node) => [node.workUnitId, node]));
		expect(byId.get("wu-schema")?.state).toBe("proven");
		expect(byId.get("wu-handshake")?.state).toBe("executing");
		expect(byId.get("wu-handshake")?.attemptNumber).toBe(2);
		expect(byId.get("wu-handshake")?.attemptStatus).toBe("running");
		expect(byId.get("wu-handshake")?.holdsCustody).toBe(true);
		expect(byId.get("wu-sweep")?.isNextRunnable).toBe(true);
		expect(byId.get("wu-island")?.state).toBe("blocked");
		expect(byId.get("wu-island")?.blockedReason).toBe("awaiting_dependency_proof");
		expect(byId.get("wu-island")?.blockedDetail).toContain("wu-handshake");
		expect(byId.get("wu-tests")?.attentionSummary).toContain("retry budget");
		expect(byId.get("wu-tests")?.responsibility).toBe("owner");
		expect(byId.get("wu-docs")?.state).toBe("retryable");
	});

	it("reads criterion readiness as a count, not raw criterion text", () => {
		const model = modelFromMissionProjection(dummyMissionProjection());
		const handshake = model.nodes.find((node) => node.workUnitId === "wu-handshake");
		expect(handshake?.criterionReadiness).toEqual({ available: true, ready: 1 });
		expect(handshake?.criterionTotal).toBe(3);
	});

	it("keeps unavailable readiness (null map) distinguishable from resolved all-false", () => {
		const projection = dummyMissionProjection();
		projection.nodes = projection.nodes.map((node) =>
			node.workUnitId === "wu-sweep" ? { ...node, criterionReady: null } : node,
		);
		const model = modelFromMissionProjection(projection);
		const sweep = model.nodes.find((node) => node.workUnitId === "wu-sweep");
		expect(sweep?.criterionReadiness).toEqual({ available: false });
		const island = model.nodes.find((node) => node.workUnitId === "wu-island");
		expect(island?.criterionReadiness).toEqual({ available: true, ready: 0 });
		expect(sweep?.criterionReadiness).not.toEqual(island?.criterionReadiness);
	});

	it("keeps a schedule entry with null readiness unavailable while resolved maps stay known", () => {
		const schedule = dummySchedule();
		schedule.workUnits = schedule.workUnits.map((entry) =>
			entry.workUnit.id === "wu-docs" ? { ...entry, criterionReady: null } : entry,
		);
		const model = modelFromPlan(dummyPlanRevision(), schedule);
		const docs = model.nodes.find((node) => node.workUnitId === "wu-docs");
		expect(docs?.state).toBe("retryable");
		expect(docs?.criterionReadiness).toEqual({ available: false });
	});

	it("carries per-node generation so a state-only refresh can update one node", () => {
		const model = modelFromMissionProjection(dummyMissionProjection());
		const generations = model.nodes.map((node) => node.generation);
		expect(new Set(generations).size).toBe(model.nodes.length);
	});

	it("sorts edges deterministically and keeps upstream ids aligned with dependsOn", () => {
		const model = modelFromMissionProjection(dummyMissionProjection());
		const keys = model.edges.map((edge) => `${edge.from}:${edge.to}`);
		expect(keys).toEqual([...keys].sort());
		const island = model.nodes.find((node) => node.workUnitId === "wu-island");
		expect(island?.upstream).toEqual(["wu-binding", "wu-handshake"]);
	});

	it("treats node/edge permutation as the same topology: key and canonical edges unchanged", () => {
		// The daemon's node array order is its canonical presentation order, so
		// the model keeps it (ELK placement and deterministic-successor selection
		// depend on it). What permutation must NEVER change is the topology
		// identity and the canonical edge order - otherwise a reshuffled
		// payload would force a spurious relayout/refit.
		const projection = dummyMissionProjection();
		const baseline = modelFromMissionProjection(projection);
		const permuted = modelFromMissionProjection({
			...projection,
			edges: [...projection.edges].reverse(),
			nodes: [...projection.nodes].reverse(),
		});
		expect(permuted.topologyKey).toBe(baseline.topologyKey);
		expect(permuted.edges).toEqual(baseline.edges);
		expect(permuted.nodes.map((node) => node.workUnitId)).toEqual([...baseline.nodes.map((node) => node.workUnitId)].reverse());
	});

	it("maps an unrecognized daemon state to unknown with its raw label, never a guess", () => {
		const projection = dummyMissionProjection();
		projection.nodes = projection.nodes.map((node) =>
			node.workUnitId === "wu-sweep" ? { ...node, scheduleState: "awaiting_fence" } : node,
		);
		const model = modelFromMissionProjection(projection);
		const sweep = model.nodes.find((node) => node.workUnitId === "wu-sweep");
		expect(sweep?.state).toBe("unknown");
		expect(sweep?.rawState).toBe("awaiting_fence");
	});
});

describe("modelFromPlan", () => {
	it("maps a bare Plan revision to proposed topology with no invented state", () => {
		const model = modelFromPlan(dummyPlanRevision());
		expect(model.source).toBe("plan");
		expect(model.topologyKey).toBe("plan:plan-resume-race-3@3");
		expect(model.nodes).toHaveLength(8);
		expect(model.nodes.every((node) => node.state === "proposed")).toBe(true);
		expect(model.nodes.every((node) => !node.criterionReadiness.available)).toBe(true);
	});

	it("overlays Schedule state without changing topology", () => {
		const bare = modelFromPlan(dummyPlanRevision());
		const live = modelFromPlan(dummyPlanRevision(), dummySchedule());
		expect(live.topologyKey).toBe(bare.topologyKey);
		expect(live.edges).toEqual(bare.edges);
		const byId = new Map(live.nodes.map((node) => [node.workUnitId, node]));
		expect(byId.get("wu-schema")?.state).toBe("proven");
		expect(byId.get("wu-island")?.state).toBe("blocked");
		expect(byId.get("wu-island")?.blockedReason).toBe("awaiting_dependency_proof");
		expect(byId.get("wu-sweep")?.isNextRunnable).toBe(true);
		expect(byId.get("wu-handshake")?.holdsCustody).toBe(true);
		expect(live.noRunnableReason).toBeUndefined();
	});
});

describe("dummy builders", () => {
	it("are deterministic byte-for-byte across calls", () => {
		expect(dummyMissionProjection()).toEqual(dummyMissionProjection());
		expect(dummyPlanRevision()).toEqual(dummyPlanRevision());
		expect(dummySchedule()).toEqual(dummySchedule());
		expect(dummyAttempts()).toEqual(dummyAttempts());
	});

	it("share no mutable arrays across calls, so caller mutation cannot contaminate later calls", () => {
		const first = dummyMissionProjection();
		first.nodes.forEach((node) => {
			node.dependsOn.length = 0;
			node.blockingDependencies.push("caller-mutation");
			if (node.criterionReady) for (const key of Object.keys(node.criterionReady)) delete node.criterionReady[key];
		});
		const firstPlan = dummyPlanRevision();
		firstPlan.workUnits.forEach((unit) => {
			unit.dependsOn.length = 0;
		});
		const firstSchedule = dummySchedule();
		firstSchedule.workUnits.forEach((entry) => {
			entry.blockingDependencies.push("caller-mutation");
		});
		expect(dummyMissionProjection()).toEqual(dummyMissionProjection());
		const islandProjection = dummyMissionProjection().nodes.find((node) => node.workUnitId === "wu-island");
		expect(islandProjection?.dependsOn).toEqual(["wu-binding", "wu-handshake"]);
		expect(islandProjection?.blockingDependencies).toEqual(["wu-handshake"]);
		expect(dummyPlanRevision()).toEqual(dummyPlanRevision());
		expect(dummyPlanRevision().workUnits.find((unit) => unit.id === "wu-island")?.dependsOn).toEqual(["wu-binding", "wu-handshake"]);
		expect(dummySchedule()).toEqual(dummySchedule());
		expect(dummySchedule().workUnits.find((entry) => entry.workUnit.id === "wu-island")?.blockingDependencies).toEqual(["wu-handshake"]);
	});

	it("produce DTOs the mappers accept with no dropped units", () => {
		expect(modelFromMissionProjection(dummyMissionProjection()).nodes).toHaveLength(8);
		expect(modelFromPlan(dummyPlanRevision(), dummySchedule()).nodes).toHaveLength(8);
		const attempts = dummyAttempts();
		expect(attempts.map((attempt) => attempt.status)).toEqual(["failed", "running"]);
		expect(attempts.every((attempt) => attempt.workUnitId === "wu-handshake")).toBe(true);
	});
});

describe("deterministic identity fences", () => {
	it("keeps the first projection node and first edge for duplicate identities", () => {
		const projection = dummyMissionProjection();
		const firstNode = projection.nodes[0];
		const firstEdge = projection.edges[0];
		expect(firstNode).toBeDefined();
		expect(firstEdge).toBeDefined();
		if (!firstNode || !firstEdge) throw new Error("dummy mission needs a node and edge");
		projection.nodes = [firstNode, { ...firstNode, title: "duplicate must lose" }, ...projection.nodes.slice(1)];
		projection.edges = [firstEdge, { ...firstEdge }, ...projection.edges.slice(1)];

		const model = modelFromMissionProjection(projection);
		expect(model.nodes.filter((node) => node.workUnitId === firstNode.workUnitId)).toHaveLength(1);
		expect(model.nodes.find((node) => node.workUnitId === firstNode.workUnitId)?.title).toBe(firstNode.title);
		expect(model.edges.filter((edge) => edge.from === firstEdge.from && edge.to === firstEdge.to)).toHaveLength(1);
	});

	it("keeps the first Plan work unit and de-duplicates repeated dependencies", () => {
		const plan = dummyPlanRevision();
		const firstUnit = plan.workUnits[0];
		const dependent = plan.workUnits.find((unit) => unit.dependsOn.length > 0);
		expect(firstUnit).toBeDefined();
		expect(dependent).toBeDefined();
		if (!firstUnit || !dependent) throw new Error("dummy plan needs units and a dependency");
		plan.workUnits = [firstUnit, { ...firstUnit, title: "duplicate must lose" }, ...plan.workUnits.slice(1)];
		dependent.dependsOn = [dependent.dependsOn[0], dependent.dependsOn[0]];

		const model = modelFromPlan(plan);
		expect(model.nodes.filter((node) => node.workUnitId === firstUnit.id)).toHaveLength(1);
		expect(model.nodes.find((node) => node.workUnitId === firstUnit.id)?.title).toBe(firstUnit.title);
		expect(new Set(model.edges.map((edge) => `${edge.from}->${edge.to}`)).size).toBe(model.edges.length);
	});
});
