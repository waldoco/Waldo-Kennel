import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { MissionRecord } from "../../hooks/useOutcome";
import { MissionWorkUnitList } from "./MissionWorkUnitList";

vi.mock("../../hooks/useEventsConnection", () => ({ useEventsConnection: () => "connected" }));

const NODE_COUNT = 75;

function scaleMission(overrides: { generation?: number; touchedScheduleState?: string } = {}): MissionRecord {
	return {
		version: 1,
		outcomeId: "out-1",
		missionId: "mission-1",
		contractRevisionNumber: 1,
		planRevisionId: "plan-1",
		planRevisionNumber: 1,
		topologyFingerprint: "fp-1",
		topologyGeneration: 1,
		generation: overrides.generation ?? 1,
		updatedAt: "2026-09-17T00:00:00Z",
		nodes: Array.from({ length: NODE_COUNT }, (_, index) => ({
			workUnitId: `wu-${String(index).padStart(3, "0")}`,
			planRevisionId: "plan-1",
			title: `WorkUnit ${index}`,
			dependsOn: index > 0 ? [`wu-${String(index - 1).padStart(3, "0")}`] : [],
			scheduleState: index === 40 && overrides.touchedScheduleState ? overrides.touchedScheduleState : "blocked",
			blockingDependencies: [],
			criterionIds: [],
			criterionReady: {},
			inputs: [],
			links: [],
			role: "worker",
			responsibility: "unconfirmed",
			updatedAt: "2026-09-17T00:00:00Z",
			generation: index === 40 && overrides.generation ? overrides.generation : 1,
		})),
		edges: Array.from({ length: NODE_COUNT - 1 }, (_, index) => ({
			from: `wu-${String(index).padStart(3, "0")}`,
			to: `wu-${String(index + 1).padStart(3, "0")}`,
		})),
	};
}

describe("Mission at 75-node scale", () => {
	it("renders all 75 nodes as list rows", () => {
		render(<MissionWorkUnitList missionQuery={{ mission: scaleMission(), isLoading: false, isFetching: false, failure: undefined, refetch: vi.fn() }} planApproved />);
		expect(screen.getAllByRole("option")).toHaveLength(NODE_COUNT);
	});

	it("a state-only update (topology identity unchanged) patches node data without remounting unrelated rows", () => {
		const { rerender } = render(
			<MissionWorkUnitList missionQuery={{ mission: scaleMission(), isLoading: false, isFetching: false, failure: undefined, refetch: vi.fn() }} planApproved />,
		);
		const untouchedRowBefore = screen.getByTestId("mission-list-row-wu-000");
		const touchedRowBefore = screen.getByTestId("mission-list-row-wu-040");

		rerender(
			<MissionWorkUnitList
				missionQuery={{
					mission: scaleMission({ generation: 999, touchedScheduleState: "executing" }),
					isLoading: false,
					isFetching: false,
					failure: undefined,
					refetch: vi.fn(),
				}}
				planApproved
			/>,
		);

		// The topology (ids/edges) is unchanged, so React reconciles by key —
		// every row DOM node survives in place; only the touched row's content
		// (its status) changes.
		expect(screen.getByTestId("mission-list-row-wu-000")).toBe(untouchedRowBefore);
		expect(screen.getByTestId("mission-list-row-wu-040")).toBe(touchedRowBefore);
		expect(screen.getAllByRole("option")).toHaveLength(NODE_COUNT);
		expect(screen.getByTestId("mission-list-row-wu-040")).toHaveTextContent("Running");
	});
});
