import { describe, expect, it, vi } from "vitest";

// Preview mode (browser demo fixtures, no daemon) has no scheduler and no
// runtime facts — it must never fabricate Mission authority from the static
// Plan (F2 forbidden scope: no Plan-to-runtime substitution). This exercises
// the exact code path a demo/dev-fixture build reaches.
vi.mock("../lib/preview-mode", () => ({ usesPreviewWorkspaceData: true }));

const planFixture = {
	id: "plan-1",
	outcomeId: "out-1",
	number: 1,
	contractRevisionNumber: 1,
	status: "approved",
	summary: "Two dependent WorkUnits",
	workUnits: [
		{
			id: "wu-1",
			kind: "direct",
			title: "First WorkUnit",
			contractRevisionNumber: 1,
			dependsOn: [],
			criterionIds: ["wu-1:crit-1"],
			outputSummary: "",
			evidenceChecks: [],
			verificationRequirement: "",
			stopConditions: [],
		},
		{
			id: "wu-2",
			kind: "direct",
			title: "Second WorkUnit",
			contractRevisionNumber: 1,
			dependsOn: ["wu-1"],
			criterionIds: ["wu-2:crit-1"],
			outputSummary: "",
			evidenceChecks: [],
			verificationRequirement: "",
			stopConditions: [],
		},
	],
	grants: [],
	runBriefCoreDigest: "a".repeat(64),
	createdAt: "2026-08-24T09:00:00Z",
};

vi.mock("../lib/preview-outcome-store", () => ({
	getPreviewPlan: vi.fn(() => planFixture),
	getPreviewOutcome: vi.fn(),
	listPreviewOutcomes: vi.fn(),
	createPreviewOutcome: vi.fn(),
	revisePreviewOutcome: vi.fn(),
	proposePreviewPlan: vi.fn(),
	approvePreviewPlan: vi.fn(),
}));

import { fetchOutcomeMission } from "./useOutcome";
import { missionGraphView } from "../components/outcome/mission-presentation";

const t = ((key: string) => key) as unknown as Parameters<typeof missionGraphView>[1];

describe("fetchOutcomeMission preview fixture", () => {
	it("never fabricates a runnable/blocked schedule split by WorkUnit order", async () => {
		const mission = await fetchOutcomeMission("out-1", "plan-1");
		expect(mission.nodes.map((node) => node.scheduleState)).toEqual(["unavailable", "unavailable"]);
	});

	it("never invents a nextAction (no Start, ever) or a top-level next-runnable/custody claim", async () => {
		const mission = await fetchOutcomeMission("out-1", "plan-1");
		expect(mission.nodes.every((node) => node.nextAction === undefined)).toBe(true);
		expect(mission.nextRunnableWorkUnitId).toBeUndefined();
		expect(mission.custodyHeldByWorkUnitId).toBeUndefined();
	});

	it("never fabricates per-criterion readiness — criterionReady stays null (unresolved), not a guessed boolean", async () => {
		const mission = await fetchOutcomeMission("out-1", "plan-1");
		expect(mission.nodes.every((node) => node.criterionReady === null)).toBe(true);
	});

	it("never synthesizes a blocking-dependency chain from WorkUnit order", async () => {
		const mission = await fetchOutcomeMission("out-1", "plan-1");
		expect(mission.nodes.every((node) => node.blockingDependencies.length === 0)).toBe(true);
	});

	it("end to end: the presentation adapter reads it as State unavailable everywhere, with no actionable node", async () => {
		const mission = await fetchOutcomeMission("out-1", "plan-1");
		const view = missionGraphView(mission, t);
		expect(view.nodes.every((node) => node.status.status === "unavailable")).toBe(true);
		expect(view.nodes.every((node) => !node.action)).toBe(true);
		expect(view.nodes.every((node) => node.criteria.kind === "unavailable")).toBe(true);
	});

	it("still reflects real, statically-known topology (title, dependsOn, edges) — only runtime facts are withheld", async () => {
		const mission = await fetchOutcomeMission("out-1", "plan-1");
		expect(mission.nodes.map((node) => node.title)).toEqual(["First WorkUnit", "Second WorkUnit"]);
		expect(mission.edges).toEqual([{ from: "wu-1", to: "wu-2" }]);
	});
});
