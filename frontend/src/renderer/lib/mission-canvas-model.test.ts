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
