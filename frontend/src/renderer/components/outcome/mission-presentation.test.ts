import { describe, expect, it } from "vitest";

import type { MissionNodeRecord, MissionRecord } from "../../hooks/useOutcome";
import {
	missionActionView,
	missionAttentionView,
	missionCriteriaView,
	missionDeterministicSuccessor,
	missionGraphView,
	missionResponsibilityLabel,
	missionSessionStatusLabel,
	missionStatusView,
	missionTopologyIdentity,
	sameMissionTopology,
} from "./mission-presentation";

// A minimal stand-in for react-i18next's TFunction: returns the key itself
// (optionally with interpolated values appended) so assertions read exactly
// what key was chosen, not translated prose.
const t = ((key: string, values?: Record<string, unknown>) =>
	values ? `${key}:${JSON.stringify(values)}` : key) as unknown as Parameters<typeof missionStatusView>[1];

function node(overrides: Partial<MissionNodeRecord> = {}): MissionNodeRecord {
	return {
		workUnitId: "wu-1",
		planRevisionId: "plan-1",
		title: "Ship the thing",
		dependsOn: [],
		scheduleState: "runnable",
		blockingDependencies: [],
		criterionIds: ["c1", "c2"],
		criterionReady: { c1: true, c2: false },
		responsibility: "unconfirmed",
		updatedAt: "2026-09-17T00:00:00Z",
		generation: 1,
		...overrides,
	};
}

function mission(overrides: Partial<MissionRecord> = {}): MissionRecord {
	return {
		version: 1,
		outcomeId: "out-1",
		missionId: "mission-1",
		contractRevisionNumber: 1,
		planRevisionId: "plan-1",
		planRevisionNumber: 1,
		topologyFingerprint: "fp-1",
		topologyGeneration: 1,
		generation: 1,
		updatedAt: "2026-09-17T00:00:00Z",
		nodes: [node()],
		edges: [],
		...overrides,
	};
}

describe("missionStatusView", () => {
	it.each(["blocked", "runnable", "executing", "proven", "retryable", "paused"] as const)(
		"maps every returned scheduleState %s to a distinct, recognized status",
		(scheduleState) => {
			const view = missionStatusView(scheduleState, t);
			expect(view.status).toBe(scheduleState);
			expect(view.label).not.toBe("mission.status.unavailable");
		},
	);

	it("fails closed to State unavailable on an unrecognized scheduleState, never nearest-matching", () => {
		const view = missionStatusView("some_future_state", t);
		expect(view.status).toBe("unavailable");
		expect(view.label).toBe("mission.status.unavailable");
	});
});

describe("missionAttentionView", () => {
	it.each(["needs_approval", "needs_choice", "needs_input"] as const)(
		"resolves the %s attention kind distinctly, keyed by its own generation",
		(kind) => {
			const view = missionAttentionView(
				"wu-1",
				{ kind, summary: "Because reasons", questionId: "q-1", generation: "gen-abc" },
				t,
			);
			expect(view?.kind).toBe(kind);
			expect(view?.markerKey).toBe(`wu-1:gen-abc:${kind}`);
		},
	);

	it("groups needs_approval and needs_choice under the owner-decision visual category, and needs_input under input", () => {
		expect(
			missionAttentionView("wu-1", { kind: "needs_approval", summary: "", generation: "g" }, t)?.visualCategory,
		).toBe("owner_decision");
		expect(
			missionAttentionView("wu-1", { kind: "needs_choice", summary: "", generation: "g" }, t)?.visualCategory,
		).toBe("owner_decision");
		expect(
			missionAttentionView("wu-1", { kind: "needs_input", summary: "", generation: "g" }, t)?.visualCategory,
		).toBe("input");
	});

	it("is undefined when no attention is present", () => {
		expect(missionAttentionView("wu-1", undefined, t)).toBeUndefined();
	});

	it("fails closed (undefined, not a guessed kind) on an unrecognized attention kind", () => {
		expect(
			missionAttentionView(
				"wu-1",
				// @ts-expect-error deliberately outside the typed union to exercise runtime drift
				{ kind: "needs_something_new", summary: "", generation: "g" },
				t,
			),
		).toBeUndefined();
	});
});

describe("missionResponsibilityLabel", () => {
	it.each(["agent", "owner", "unconfirmed"] as const)("recognizes %s", (value) => {
		expect(missionResponsibilityLabel(value, t)).toBe(`mission.responsibility.${value}`);
	});

	it("fails closed on an unrecognized responsibility value", () => {
		expect(missionResponsibilityLabel("some_future_value", t)).toBe("mission.responsibility.unavailable");
	});
});

describe("missionSessionStatusLabel", () => {
	it("renders exactly Unknown for the one status the backend can return today, never Running", () => {
		expect(missionSessionStatusLabel("unknown", t)).toBe("mission.session.unknown");
	});

	it("fails closed to an explicit unavailable reading on any other raw value, distinct from Unknown", () => {
		expect(missionSessionStatusLabel("running", t)).toBe("mission.session.unavailable");
		expect(missionSessionStatusLabel(undefined, t)).toBe("mission.session.unavailable");
	});
});

describe("missionCriteriaView", () => {
	it("counts ready criteria against the total", () => {
		expect(missionCriteriaView(node({ criterionIds: ["c1", "c2", "c3"], criterionReady: { c1: true, c2: true, c3: false } }))).toEqual({
			kind: "ready",
			ready: 2,
			total: 3,
		});
	});

	it("treats a null criterionReady as unavailable, not zero-ready", () => {
		expect(missionCriteriaView(node({ criterionReady: null }))).toEqual({ kind: "unavailable" });
	});
});

describe("missionActionView", () => {
	it("enables exactly one action, only for the returned start value", () => {
		expect(missionActionView("start", t)).toEqual({ kind: "start", label: "mission.action.start" });
	});

	it("stays non-actionable — never a renderer-invented Retry — when no action is returned, even on a retryable node", () => {
		expect(missionActionView(undefined, t)).toBeUndefined();
	});

	it("fails closed on an unrecognized nextAction value instead of guessing an action", () => {
		expect(missionActionView("some_future_action", t)).toBeUndefined();
	});
});

describe("missionGraphView", () => {
	it("bijects with mission.nodes and drops edges with an unknown endpoint", () => {
		const fixture = mission({
			nodes: [
				node({ workUnitId: "a", dependsOn: [] }),
				node({ workUnitId: "b", dependsOn: ["a"] }),
				node({ workUnitId: "c", dependsOn: ["missing-node"] }),
			],
			edges: [
				{ from: "a", to: "b" },
				{ from: "missing-node", to: "c" },
			],
		});
		const view = missionGraphView(fixture, t);
		expect(view.nodes).toHaveLength(3);
		expect(view.edges).toEqual([{ from: "a", to: "b" }]);
		expect(view.nodesByWorkUnitId.get("c")?.dependencyUnavailable).toBe(true);
		expect(view.nodesByWorkUnitId.get("b")?.dependencyUnavailable).toBe(false);
	});

	it("renders a four-node fork/join with exactly its dependency edges and no extras", () => {
		const fixture = mission({
			nodes: [
				node({ workUnitId: "root", dependsOn: [] }),
				node({ workUnitId: "left", dependsOn: ["root"] }),
				node({ workUnitId: "right", dependsOn: ["root"] }),
				node({ workUnitId: "join", dependsOn: ["left", "right"] }),
			],
			edges: [
				{ from: "root", to: "left" },
				{ from: "root", to: "right" },
				{ from: "left", to: "join" },
				{ from: "right", to: "join" },
			],
		});
		const view = missionGraphView(fixture, t);
		expect(view.nodes.map((n) => n.workUnitId)).toEqual(["root", "left", "right", "join"]);
		expect(view.edges).toHaveLength(4);
		expect(view.nodes.every((n) => !n.dependencyUnavailable)).toBe(true);
		expect(view.nodesByWorkUnitId.get("root")).toMatchObject({ dependencyCount: 0, dependentCount: 2 });
		expect(view.nodesByWorkUnitId.get("join")).toMatchObject({ dependencyCount: 2, dependentCount: 0 });
	});

	it("never counts a dropped (unknown-endpoint) edge toward either node's dependency summary", () => {
		const fixture = mission({
			nodes: [node({ workUnitId: "a", dependsOn: [] }), node({ workUnitId: "b", dependsOn: ["missing"] })],
			edges: [{ from: "missing", to: "b" }],
		});
		const view = missionGraphView(fixture, t);
		expect(view.nodesByWorkUnitId.get("b")).toMatchObject({ dependencyUnavailable: true, dependencyCount: 0 });
	});
});

describe("topology identity", () => {
	it("treats a state-only change (generation moves, identity fields hold) as the same topology", () => {
		const a = missionTopologyIdentity(mission({ generation: 1 }));
		const b = missionTopologyIdentity(mission({ generation: 2 }));
		expect(sameMissionTopology(a, b)).toBe(true);
	});

	it("is unaffected by same-millisecond generation collisions because it never reads generation at all", () => {
		const a = missionTopologyIdentity(mission({ generation: 42, updatedAt: "2026-09-17T00:00:00.000Z" }));
		const b = missionTopologyIdentity(mission({ generation: 42, updatedAt: "2026-09-17T00:00:00.000Z" }));
		expect(sameMissionTopology(a, b)).toBe(true);
	});

	it("treats any of planRevisionId/topologyFingerprint/topologyGeneration moving as a topology swap", () => {
		const base = missionTopologyIdentity(mission());
		expect(sameMissionTopology(base, missionTopologyIdentity(mission({ planRevisionId: "plan-2" })))).toBe(false);
		expect(sameMissionTopology(base, missionTopologyIdentity(mission({ topologyFingerprint: "fp-2" })))).toBe(false);
		expect(sameMissionTopology(base, missionTopologyIdentity(mission({ topologyGeneration: 2 })))).toBe(false);
	});
});

describe("missionDeterministicSuccessor", () => {
	it("picks the lexicographically smallest surviving id, not insertion order", () => {
		expect(missionDeterministicSuccessor([{ workUnitId: "zzz" }, { workUnitId: "aaa" }, { workUnitId: "mmm" }])).toBe(
			"aaa",
		);
	});

	it("is undefined when nothing survives", () => {
		expect(missionDeterministicSuccessor([])).toBeUndefined();
	});
});
