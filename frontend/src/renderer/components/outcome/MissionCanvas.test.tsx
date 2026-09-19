import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactFlowInstance } from "@xyflow/react";

import { MissionCanvas } from "./MissionCanvas";
import { resetCanvasLayoutStateForTests, setCanvasLayoutHooksForTests } from "../../lib/mission-canvas-layout";
import { dummyMissionProjection } from "../../lib/mission-canvas-dummy";
import type { useOutcomeMission } from "../../hooks/useOutcome";

const { connectionState } = vi.hoisted(() => ({ connectionState: { value: "connected" as "connected" | "disconnected" | "idle" } }));
vi.mock("../../hooks/useEventsConnection", () => ({ useEventsConnection: () => connectionState.value }));

type MissionQuery = ReturnType<typeof useOutcomeMission>;

function missionQuery(overrides: Partial<MissionQuery> = {}): MissionQuery {
	return {
		mission: dummyMissionProjection(),
		isLoading: false,
		isFetching: false,
		failure: undefined,
		refetch: vi.fn(),
		...overrides,
	};
}

function withNodeState(mission: MissionQuery["mission"], workUnitId: string, scheduleState: string) {
	if (!mission) return mission;
	return {
		...mission,
		generation: mission.generation + 1,
		nodes: mission.nodes.map((node) =>
			node.workUnitId === workUnitId ? { ...node, scheduleState, generation: node.generation + 100 } : node,
		),
	};
}

beforeEach(() => {
	resetCanvasLayoutStateForTests();
	connectionState.value = "connected";
});

afterEach(() => {
	vi.unstubAllGlobals();
});

describe("MissionCanvas", () => {
	it("renders every projection node through the shared WorkUnit face after layout resolves", async () => {
		render(<MissionCanvas missionQuery={missionQuery()} planApproved />);
		// Layout is async (worker or main-thread fallback): an honest pending
		// reading shows first, never a half-laid-out graph.
		expect(screen.getByTestId("mission-canvas-layout-pending")).toBeInTheDocument();
		await waitFor(() => expect(screen.queryAllByTestId(/^mission-node-face-/).length).toBe(8));
		expect(screen.getByText("Bind resume to the same worktree lease")).toBeInTheDocument();
		expect(screen.queryByTestId("mission-canvas-layout-pending")).not.toBeInTheDocument();
	});

	it("never sends a missing-endpoint edge to React Flow", async () => {
		let instance: ReactFlowInstance<any, any> | undefined;
		const mission = dummyMissionProjection();
		const sourceWorkUnitId = mission.nodes[0]?.workUnitId;
		expect(sourceWorkUnitId).toBeDefined();
		if (!sourceWorkUnitId) throw new Error("dummy mission needs a source node");
		mission.edges = [...mission.edges, { from: sourceWorkUnitId, to: "ghost-node" }];
		render(
			<MissionCanvas
				missionQuery={missionQuery({ mission })}
				onInstanceReady={(ready) => {
					instance = ready;
				}}
				planApproved
			/>,
		);
		await waitFor(() => expect(instance).toBeDefined());
		await waitFor(() => expect(instance!.getEdges().length).toBeGreaterThan(0));
		expect(instance!.getEdges().some((edge) => edge.source === "ghost-node" || edge.target === "ghost-node")).toBe(false);
	});

	it("selects a node into the shared OutcomeInspector; the canvas exposes no execution surface", async () => {
		render(<MissionCanvas missionQuery={missionQuery()} planApproved />);
		await waitFor(() => expect(screen.queryAllByTestId(/^mission-node-face-/).length).toBe(8));
		const node = screen.getByTestId("mission-node-face-wu-sweep");
		// fireEvent rather than userEvent: userEvent's synthetic mousedown omits
		// `view`, which d3-zoom's pane-level handler dereferences - a jsdom-only
		// gap, not a browser behavior.
		fireEvent.click(node);
		await waitFor(() => expect(screen.getByTestId("outcome-inspector")).toBeInTheDocument());
		expect(screen.queryByTestId("mission-row-action")).not.toBeInTheDocument();
	});

	it("keeps viewport and selection on a state-only refresh, refits only on a topology swap", async () => {
		let instance: ReactFlowInstance<any, any> | undefined;
		const query = missionQuery();
		const { rerender } = render(
			<MissionCanvas
				missionQuery={query}
				onInstanceReady={(ready) => {
					instance = ready;
				}}
				planApproved
			/>,
		);
		await waitFor(() => expect(screen.queryAllByTestId(/^mission-node-face-/).length).toBe(8));
		await waitFor(() => expect(instance).toBeDefined());
		const fitViewSpy = vi.spyOn(instance!, "fitView");
		await waitFor(() => expect(instance!.getNodes()).toHaveLength(8));
		await new Promise((resolve) => setTimeout(resolve, 50));
		const fitCallsBeforeStateRefresh = fitViewSpy.mock.calls.length;

		fireEvent.click(screen.getByTestId("mission-node-face-wu-sweep"));
		await waitFor(() => expect(screen.getByTestId("outcome-inspector")).toBeInTheDocument());

		// State-only refresh: same topology fingerprint, one node's state and
		// generation changed. No refit, selection intact.
		const stateOnlyQuery = { ...query, mission: withNodeState(query.mission, "wu-docs", "runnable") };
		rerender(<MissionCanvas missionQuery={stateOnlyQuery} onInstanceReady={() => {}} planApproved />);
		await waitFor(() => expect(screen.getByTestId("mission-node-face-wu-docs")).toHaveTextContent("Ready"));
		expect(fitViewSpy).toHaveBeenCalledTimes(fitCallsBeforeStateRefresh);
		expect(screen.getByTestId("outcome-inspector")).toBeInTheDocument();

		// Topology swap uses a wholly different ID set and extent. At the exact
		// moment fitView runs, React Flow must already expose only those nodes.
		const replacement = query.mission!.nodes[0];
		const swapped = {
			...query.mission!,
			topologyFingerprint: "topology-replacement-only",
			topologyGeneration: 4,
			nodes: [{ ...replacement, workUnitId: "wu-replacement", title: "Replacement unit", dependsOn: [] }],
			edges: [],
			nextRunnableWorkUnitId: undefined,
		};
		const idsAtFit: string[][] = [];
		fitViewSpy.mockImplementation(() => {
			idsAtFit.push(instance!.getNodes().map((node) => node.id).sort());
			return Promise.resolve(true);
		});
		rerender(<MissionCanvas missionQuery={{ ...query, mission: swapped }} onInstanceReady={() => {}} planApproved />);
		await waitFor(() => expect(fitViewSpy).toHaveBeenCalledTimes(fitCallsBeforeStateRefresh + 1));
		expect(idsAtFit.at(-1)).toEqual(["wu-replacement"]);
		expect(screen.getByTestId("mission-node-face-wu-replacement")).toBeInTheDocument();
		expect(screen.queryByTestId("mission-node-face-wu-sweep")).not.toBeInTheDocument();
	});

	it("skips the fit animation under reduced motion", async () => {
		vi.stubGlobal("matchMedia", (query: string) => ({
			matches: query.includes("prefers-reduced-motion"),
			media: query,
			onchange: null,
			addEventListener: () => undefined,
			removeEventListener: () => undefined,
			addListener: () => undefined,
			removeListener: () => undefined,
			dispatchEvent: () => false,
		}));
		let instance: ReactFlowInstance<any, any> | undefined;
		const query = missionQuery();
		const { rerender } = render(
			<MissionCanvas
				missionQuery={query}
				onInstanceReady={(ready) => {
					instance = ready;
				}}
				planApproved
			/>,
		);
		await waitFor(() => expect(screen.queryAllByTestId(/^mission-node-face-/).length).toBe(8));
		await waitFor(() => expect(instance).toBeDefined());
		const fitViewSpy = vi.spyOn(instance!, "fitView");
		await waitFor(() => expect(instance!.getNodes()).toHaveLength(8));
		fitViewSpy.mockClear();
		const swapped = { ...dummyMissionProjection(), topologyFingerprint: "topology-other", topologyGeneration: 9 };
		rerender(<MissionCanvas missionQuery={{ ...query, mission: swapped }} onInstanceReady={() => {}} planApproved />);
		await waitFor(() => expect(fitViewSpy).toHaveBeenCalledWith(expect.objectContaining({ duration: 0 })));
	});

	it("explains a no-runnable reason instead of leaving a quiet graph", async () => {
		const mission = { ...dummyMissionProjection(), noRunnableReason: "awaiting_proof", nextRunnableWorkUnitId: undefined };
		render(<MissionCanvas missionQuery={missionQuery({ mission })} planApproved />);
		await waitFor(() => expect(screen.getByTestId("mission-graph-no-runnable")).toBeInTheDocument());
	});

	it("freezes the last confirmed projection with a stale banner when disconnected", async () => {
		render(<MissionCanvas missionQuery={missionQuery()} planApproved />);
		await waitFor(() => expect(screen.queryAllByTestId(/^mission-node-face-/).length).toBe(8));
		connectionState.value = "disconnected";
		render(<MissionCanvas missionQuery={missionQuery()} planApproved />);
		await waitFor(() => expect(screen.getByTestId("mission-canvas-stale-banner")).toBeInTheDocument());
	});

	it("renders the honest error state with retry when the projection failed and nothing is confirmed", () => {
		const refetch = vi.fn();
		render(
			<MissionCanvas
				missionQuery={missionQuery({ mission: undefined, failure: { message: "daemon unreachable" } as MissionQuery["failure"], refetch })}
				planApproved
			/>,
		);
		expect(screen.getByTestId("mission-canvas-error")).toHaveTextContent("daemon unreachable");
		fireEvent.click(screen.getByTestId("mission-canvas-retry"));
		expect(refetch).toHaveBeenCalled();
	});

	it("surfaces a layout failure honestly and the retry explicitly resets layout, no refetch involved", async () => {
		class FatalWorker {
			onmessage: ((event: MessageEvent) => void) | null = null;
			onerror: ((event: { message?: string }) => void) | null = null;
			onmessageerror: (() => void) | null = null;
			postMessage(): void {
				queueMicrotask(() => this.onerror?.({ message: "worker script failed" }));
			}
			terminate(): void {}
		}
		setCanvasLayoutHooksForTests({
			workerCtor: FatalWorker as unknown as new (url: string) => Worker,
			elkLoader: async () => ({
				default: class {
					layout(): Promise<unknown> {
						return Promise.reject(new Error("engine unavailable"));
					}
				},
			}),
		});
		const refetch = vi.fn();
		render(<MissionCanvas missionQuery={missionQuery({ refetch })} planApproved />);
		await waitFor(() => expect(screen.getByTestId("mission-canvas-layout-error")).toHaveTextContent("engine unavailable"));
		// Restore a working engine, then the retry must recover through the
		// explicit layout reset alone - refetch is never called.
		resetCanvasLayoutStateForTests();
		fireEvent.click(screen.getByTestId("mission-canvas-layout-retry"));
		await waitFor(() => expect(screen.queryAllByTestId(/^mission-node-face-/).length).toBe(8));
		expect(refetch).not.toHaveBeenCalled();
	});

	it("renders nothing before a Plan is authorized", () => {
		const { container } = render(<MissionCanvas missionQuery={missionQuery()} planApproved={false} />);
		expect(container).toBeEmptyDOMElement();
	});
});
