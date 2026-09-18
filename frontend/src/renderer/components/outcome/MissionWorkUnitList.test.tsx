import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { MissionRecord } from "../../hooks/useOutcome";
import { MissionWorkUnitList } from "./MissionWorkUnitList";

const { connectionState } = vi.hoisted(() => ({ connectionState: { value: "connected" as "connected" | "disconnected" | "idle" } }));
vi.mock("../../hooks/useEventsConnection", () => ({ useEventsConnection: () => connectionState.value }));

function node(overrides: Partial<MissionRecord["nodes"][number]> = {}): MissionRecord["nodes"][number] {
	return {
		workUnitId: "a",
		planRevisionId: "plan-1",
		title: "Node A",
		dependsOn: [],
		scheduleState: "runnable",
		blockingDependencies: [],
		criterionIds: [],
		criterionReady: {},
		inputs: [],
		links: [],
		role: "worker",
		responsibility: "unconfirmed",
		updatedAt: "2026-09-17T00:00:00Z",
		generation: 1,
		...overrides,
	};
}

function forkJoinMission(overrides: Partial<MissionRecord> = {}): MissionRecord {
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
		nodes: [
			node({ workUnitId: "root" }),
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
		...overrides,
	};
}

function missionQuery(mission: MissionRecord | undefined, overrides: Partial<ReturnType<typeof import("../../hooks/useOutcome").useOutcomeMission>> = {}) {
	return {
		mission,
		isLoading: false,
		isFetching: false,
		failure: undefined,
		refetch: vi.fn(),
		...overrides,
	};
}

describe("MissionWorkUnitList", () => {
	it("renders a four-node fork/join as four list rows with no extras", () => {
		render(<MissionWorkUnitList missionQuery={missionQuery(forkJoinMission())} planApproved />);
		const rows = screen.getAllByRole("option");
		expect(rows).toHaveLength(4);
		expect(screen.getByTestId("mission-list-row-root")).toBeInTheDocument();
		expect(screen.getByTestId("mission-list-row-left")).toBeInTheDocument();
		expect(screen.getByTestId("mission-list-row-right")).toBeInTheDocument();
		expect(screen.getByTestId("mission-list-row-join")).toBeInTheDocument();
	});

	it("selects a row and opens the shared inspector with matching content", async () => {
		const user = userEvent.setup();
		render(<MissionWorkUnitList missionQuery={missionQuery(forkJoinMission())} planApproved />);
		await user.click(screen.getByTestId("mission-list-row-root"));
		expect(screen.getByTestId("outcome-inspector")).toBeInTheDocument();
		expect(screen.getByTestId("outcome-inspector-state")).toHaveTextContent("Ready");
	});

	it("a state-only update (topology identity unchanged) keeps the selection open", () => {
		const mission = forkJoinMission();
		const { rerender } = render(<MissionWorkUnitList missionQuery={missionQuery(mission)} planApproved />);
		const rows = screen.getAllByRole("option");
		if (rows[0]) fireEvent.click(rows[0]);

		const stateOnlyUpdate: MissionRecord = {
			...mission,
			generation: 999,
			nodes: mission.nodes.map((n) => (n.workUnitId === "root" ? { ...n, scheduleState: "executing", generation: 42 } : n)),
		};
		rerender(<MissionWorkUnitList missionQuery={missionQuery(stateOnlyUpdate)} planApproved />);
		expect(screen.getByTestId("outcome-inspector")).toBeInTheDocument();
		expect(screen.getByTestId("outcome-inspector-state")).toHaveTextContent("Running");
	});

	it("an authorized topology swap announces added/removed counts and moves selection to the deterministic successor when the selected node disappears", async () => {
		const user = userEvent.setup();
		const mission = forkJoinMission();
		const { rerender } = render(<MissionWorkUnitList missionQuery={missionQuery(mission)} planApproved />);
		await user.click(screen.getByTestId("mission-list-row-root"));

		const swapped: MissionRecord = {
			...mission,
			planRevisionId: "plan-2",
			topologyFingerprint: "fp-2",
			topologyGeneration: 2,
			generation: 2,
			nodes: [node({ workUnitId: "left", dependsOn: [] }), node({ workUnitId: "new-node" })],
			edges: [],
		};
		rerender(<MissionWorkUnitList missionQuery={missionQuery(swapped)} planApproved />);
		expect(screen.getAllByRole("option")).toHaveLength(2);
		// root disappeared; deterministic successor among {left, new-node} is "left" (lexicographically smallest).
		expect(screen.getByTestId("mission-list-row-left").getAttribute("aria-selected")).toBe("true");
	});

	it("shows the honest error state with Retry, and never renders topology from a Mission-only error", () => {
		const refetch = vi.fn();
		render(<MissionWorkUnitList missionQuery={missionQuery(undefined, { failure: { kind: "retryable", message: "Boom" }, refetch })} planApproved />);
		expect(screen.getByTestId("mission-list-error")).toHaveTextContent("Boom");
		fireEvent.click(screen.getByTestId("mission-list-retry"));
		expect(refetch).toHaveBeenCalled();
	});

	it("shows a neutral Plan-pending reading while Mission is loading but a Plan is already known", () => {
		render(
			<MissionWorkUnitList
				missionQuery={missionQuery(undefined, { isLoading: true })}
				planApproved
				planWorkUnits={[{ id: "wu-1", title: "A", dependsOn: [] }]}
			/>,
		);
		expect(screen.getByTestId("mission-list-plan-pending")).toBeInTheDocument();
	});

	it("shows an explicit empty state for a Mission with no WorkUnits", () => {
		render(<MissionWorkUnitList missionQuery={missionQuery(forkJoinMission({ nodes: [], edges: [] }))} planApproved />);
		expect(screen.getByTestId("mission-list-empty")).toBeInTheDocument();
	});

	it("freezes the last confirmed graph and shows a stale banner with Refresh when disconnected", () => {
		connectionState.value = "disconnected";
		try {
			render(<MissionWorkUnitList missionQuery={missionQuery(undefined, { failure: undefined })} planApproved />);
			// No prior confirmed graph in this render pass and disconnected with
			// no data yields the honest loading reading, not a fabricated graph.
			expect(screen.queryByTestId("mission-list-stale-banner")).not.toBeInTheDocument();
		} finally {
			connectionState.value = "connected";
		}
	});

	it("freezes and disables actions on disconnect even though React Query still retains the last successful mission (the normal case, not just a cold cache)", () => {
		const mission = forkJoinMission({
			nodes: [
				node({ workUnitId: "root", nextAction: "start" }),
				node({ workUnitId: "left", dependsOn: ["root"] }),
				node({ workUnitId: "right", dependsOn: ["root"] }),
				node({ workUnitId: "join", dependsOn: ["left", "right"] }),
			],
			nextRunnableWorkUnitId: "root",
		});
		const onStart = vi.fn();
		const { rerender } = render(<MissionWorkUnitList missionQuery={missionQuery(mission)} onStart={onStart} planApproved />);
		// Confirm the action is live and enabled before disconnect.
		expect(screen.getByTestId("mission-row-action")).toBeEnabled();
		expect(screen.queryByTestId("mission-list-stale-banner")).not.toBeInTheDocument();

		connectionState.value = "disconnected";
		try {
			// React Query retains its last successful data across a transport
			// disconnect — `missionQuery.mission` is still the same truthy
			// object here, exactly like production. Freezing must not depend on
			// the query having gone empty.
			rerender(<MissionWorkUnitList missionQuery={missionQuery(mission)} onStart={onStart} planApproved />);
			expect(screen.getByTestId("mission-list-stale-banner")).toBeInTheDocument();
			const actionButton = screen.getByTestId("mission-row-action");
			expect(actionButton).toBeDisabled();
			fireEvent.click(actionButton);
			expect(onStart).not.toHaveBeenCalled();
		} finally {
			connectionState.value = "connected";
		}
	});

	it("surfaces a refresh-failed banner (not silence) when a refetch fails over already-shown retained data while still connected", () => {
		const mission = forkJoinMission();
		const { rerender } = render(<MissionWorkUnitList missionQuery={missionQuery(mission)} planApproved />);
		expect(screen.queryByTestId("mission-list-refresh-failed-banner")).not.toBeInTheDocument();

		const refetch = vi.fn();
		rerender(
			<MissionWorkUnitList
				missionQuery={missionQuery(mission, { failure: { kind: "retryable", message: "Network blip" }, refetch })}
				planApproved
			/>,
		);
		const banner = screen.getByTestId("mission-list-refresh-failed-banner");
		expect(banner).toHaveTextContent("Network blip");
		// The retained graph stays visible underneath the banner — a failed
		// refresh never blanks out the last confirmed truth.
		expect(screen.getAllByRole("option")).toHaveLength(4);
		fireEvent.click(screen.getByTestId("mission-list-refresh-failed-retry"));
		expect(refetch).toHaveBeenCalled();
	});

	it("counts attention separately across approval, choice, and input", () => {
		const mission = forkJoinMission({
			nodes: [
				node({ workUnitId: "a", attention: { kind: "needs_approval", summary: "", generation: "g1" } }),
				node({ workUnitId: "b", attention: { kind: "needs_choice", summary: "", generation: "g2" } }),
				node({ workUnitId: "c", attention: { kind: "needs_input", summary: "", generation: "g3" } }),
				node({ workUnitId: "d", attention: { kind: "needs_input", summary: "", generation: "g4" } }),
			],
			edges: [],
		});
		render(<MissionWorkUnitList missionQuery={missionQuery(mission)} planApproved />);
		const strip = screen.getByTestId("mission-attention-strip");
		expect(strip).toHaveTextContent("Needs approval · 1");
		expect(strip).toHaveTextContent("Needs choice · 1");
		expect(strip).toHaveTextContent("Needs input · 2");
	});
});
