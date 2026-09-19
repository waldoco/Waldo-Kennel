import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
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

function exposeMeasuredNodes(instance: ReactFlowInstance<any, any>): void {
	const getInternalNode = instance.getInternalNode.bind(instance);
	vi.spyOn(instance, "getInternalNode").mockImplementation((id) => {
		const node = getInternalNode(id);
		return node ? { ...node, measured: { width: 256, height: 84 } } : undefined;
	});
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
		expect(screen.getByTestId("mission-node-face-wu-binding")).toHaveTextContent("Bind resume to the same worktree lease");
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
					exposeMeasuredNodes(ready);
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

	it("waits for every internal node measurement before fitting the controlled flow", async () => {
		let instance: ReactFlowInstance<any, any> | undefined;
		let measured = false;
		const boundsAtFit: { id: string; width?: number; height?: number }[][] = [];
		render(
			<MissionCanvas
				missionQuery={missionQuery()}
				onInstanceReady={(ready) => {
					instance = ready;
					const getInternalNode = ready.getInternalNode.bind(ready);
					vi.spyOn(ready, "getInternalNode").mockImplementation((id) => {
						const node = getInternalNode(id);
						return node ? { ...node, measured: measured ? { width: 256, height: 84 } : { width: undefined, height: undefined } } : undefined;
					});
					vi.spyOn(ready, "fitView").mockImplementation(() => {
						boundsAtFit.push(ready.getNodes().map((node) => ({ id: node.id, ...ready.getInternalNode(node.id)?.measured })));
						return Promise.resolve(true);
					});
				}}
				planApproved
			/>,
		);
		await waitFor(() => expect(instance?.getNodes()).toHaveLength(8));
		await new Promise((resolve) => setTimeout(resolve, 40));
		expect(instance?.fitView).not.toHaveBeenCalled();
		measured = true;
		await waitFor(() => expect(instance?.fitView).toHaveBeenCalledWith(expect.objectContaining({ minZoom: 0.25, padding: 0.08 })));
		expect(boundsAtFit.at(-1)?.every((node) => Number(node.width) > 0 && Number(node.height) > 0)).toBe(true);
	});

	it("keeps viewport and selection on a state-only refresh, refits only on a topology swap", async () => {
		let instance: ReactFlowInstance<any, any> | undefined;
		const query = missionQuery();
		const { rerender } = render(
			<MissionCanvas
				missionQuery={query}
				onInstanceReady={(ready) => {
					instance = ready;
					exposeMeasuredNodes(ready);
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
		// Opening the inspector is a frame resize and intentionally refits once.
		await waitFor(() => expect(fitViewSpy.mock.calls.length).toBeGreaterThanOrEqual(fitCallsBeforeStateRefresh));
		const fitCallsAfterInspector = fitViewSpy.mock.calls.length;

		// State-only refresh: same topology fingerprint, one node's state and
		// generation changed. No refit, selection intact.
		const stateOnlyQuery = { ...query, mission: withNodeState(query.mission, "wu-docs", "runnable") };
		rerender(<MissionCanvas missionQuery={stateOnlyQuery} onInstanceReady={() => {}} planApproved />);
		await waitFor(() => expect(screen.getByTestId("mission-node-face-wu-docs")).toHaveTextContent("Ready"));
		expect(fitViewSpy).toHaveBeenCalledTimes(fitCallsAfterInspector);
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
		await waitFor(() => expect(fitViewSpy).toHaveBeenCalledTimes(fitCallsAfterInspector + 1));
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
					exposeMeasuredNodes(ready);
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
		const { rerender } = render(<MissionCanvas missionQuery={missionQuery()} planApproved />);
		await waitFor(() => expect(screen.queryAllByTestId(/^mission-node-face-/).length).toBe(8));
		connectionState.value = "disconnected";
		rerender(<MissionCanvas missionQuery={missionQuery()} planApproved />);
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

describe("MissionCanvas interactions", () => {
	it("reveals a bounded hover/focus preview on every node - title, status, attention reason, dependency counts", async () => {
		render(<MissionCanvas missionQuery={missionQuery()} planApproved />);
		await waitFor(() => expect(screen.queryAllByTestId(/^mission-node-face-/).length).toBe(8));
		expect(screen.queryAllByTestId(/^mission-node-preview-/).length).toBe(8);

		const preview = screen.getByTestId("mission-node-preview-wu-tests");
		expect(preview).toHaveTextContent("Cover resume race in regression tests");
		expect(preview).toHaveTextContent("Paused");
		expect(preview).toHaveTextContent("Owner decision needed: keep the resume retry budget at 2?");
		expect(preview).toHaveTextContent("1 upstream, 1 downstream");
		// The preview never carries an action and never hides its facts from one
		// input mode: reveal is CSS on the RF wrapper for BOTH pointer hover and
		// keyboard focus-within.
		// The RF wrapper carries a meaningful label ("title - status"), never a bare "node".
		const wrapper = document.querySelector('.react-flow__node[data-id="wu-tests"]');
		expect(wrapper?.getAttribute("aria-label")).toBe("Cover resume race in regression tests - Paused");
		expect(preview.className).toContain("group-hover:opacity-100");
		expect(preview.className).toContain("group-focus-within:opacity-100");
		expect(preview).toHaveAttribute("aria-hidden", "true");
		expect(within(preview).queryByRole("button")).not.toBeInTheDocument();
	});

	it("moves focus by topology with the arrow keys, selects with Enter, returns focus with Escape", async () => {
		const { container } = render(<MissionCanvas missionQuery={missionQuery()} planApproved />);
		await waitFor(() => expect(screen.queryAllByTestId(/^mission-node-face-/).length).toBe(8));
		const nodeEl = (id: string) => {
			const el = container.querySelector<HTMLElement>(`.react-flow__node[data-id="${id}"]`);
			expect(el, `node ${id}`).not.toBeNull();
			return el as HTMLElement;
		};
		const focusedId = () => (document.activeElement instanceof HTMLElement ? document.activeElement.dataset.id : undefined);

		// The RF node wrapper is the single focus owner.
		nodeEl("wu-schema").focus();
		expect(focusedId()).toBe("wu-schema");
		expect(nodeEl("wu-schema")).toHaveClass("focus-visible:ring-2", "focus-visible:ring-ring", "focus-visible:ring-offset-2");

		// Right crosses into the successor layer; layer 0 holds exactly one node,
		// so only the landing sibling is topology-dependent.
		fireEvent.keyDown(nodeEl("wu-schema"), { key: "ArrowRight" });
		const landed = focusedId();
		expect(["wu-binding", "wu-handshake"]).toContain(landed);

		// Down moves to the sibling inside the layer.
		fireEvent.keyDown(document.activeElement as HTMLElement, { key: "ArrowDown" });
		expect(["wu-binding", "wu-handshake"]).toContain(focusedId());
		expect(focusedId()).not.toBe(landed);

		// Left returns across layers; layer 0's single node makes the target exact.
		fireEvent.keyDown(document.activeElement as HTMLElement, { key: "ArrowLeft" });
		expect(focusedId()).toBe("wu-schema");

		// The graph edge never wraps or jumps: no left layer, no sibling - focus stays.
		fireEvent.keyDown(nodeEl("wu-schema"), { key: "ArrowLeft" });
		fireEvent.keyDown(nodeEl("wu-schema"), { key: "ArrowUp" });
		expect(focusedId()).toBe("wu-schema");

		// Enter on a focused node performs the click's selection.
		fireEvent.keyDown(nodeEl("wu-schema"), { key: "Enter" });
		await waitFor(() => expect(screen.getByTestId("outcome-inspector")).toBeInTheDocument());

		// Escape dismisses the modal overlay and returns focus toward the pane.
		fireEvent.keyDown(screen.getByTestId("outcome-inspector"), { key: "Escape" });
		expect(focusedId()).not.toBe("wu-schema");
		expect(screen.queryByTestId("outcome-inspector")).not.toBeInTheDocument();
	});

	it("overlays the inspector without resizing or refitting the inert graph", async () => {
		let instance: ReactFlowInstance<any, any> | undefined;
		render(<MissionCanvas missionQuery={missionQuery()} onInstanceReady={(ready) => { instance = ready; exposeMeasuredNodes(ready); }} planApproved />);
		await waitFor(() => expect(instance).toBeDefined());
		await waitFor(() => expect(screen.queryAllByTestId(/^mission-node-face-/)).toHaveLength(8));
		const fitViewSpy = vi.spyOn(instance!, "fitView").mockResolvedValue(true);
		fireEvent.click(screen.getByTestId("mission-node-face-wu-binding"));
		await waitFor(() => expect(screen.getByTestId("mission-canvas-inspector-overlay")).toBeInTheDocument());
		expect(screen.getByTestId("mission-canvas-viewport")).toHaveAttribute("inert");
		await new Promise((resolve) => setTimeout(resolve, 40));
		expect(fitViewSpy).not.toHaveBeenCalled();
		fireEvent.click(screen.getByTestId("outcome-inspector-close"));
		await waitFor(() => expect(screen.queryByTestId("mission-canvas-inspector-overlay")).not.toBeInTheDocument());
		expect(screen.getByTestId("mission-canvas-viewport")).not.toHaveAttribute("inert");
		expect(fitViewSpy).not.toHaveBeenCalled();
	});

	it("highlights the selected node's lineage and dims the rest until selection clears", async () => {
		const { container } = render(<MissionCanvas missionQuery={missionQuery()} planApproved />);
		await waitFor(() => expect(screen.queryAllByTestId(/^mission-node-face-/).length).toBe(8));
		fireEvent.click(screen.getByTestId("mission-node-face-wu-binding"));
		await waitFor(() => expect(screen.getByTestId("outcome-inspector")).toBeInTheDocument());

		const nodeClass = (id: string) => container.querySelector(`.react-flow__node[data-id="${id}"]`)?.className ?? "";
		// Lineage of wu-binding: ancestor wu-schema; descendants sweep, island, docs, doctor.
		for (const id of ["wu-schema", "wu-binding", "wu-sweep", "wu-island", "wu-docs", "wu-doctor"]) {
			expect(nodeClass(id), id).not.toContain("opacity-40");
		}
		// The handshake subtree is not on binding's lineage: it steps back.
		for (const id of ["wu-handshake", "wu-tests"]) {
			expect(nodeClass(id), id).toContain("opacity-40");
		}
		// Edge classification is pure (canvasLineage) and unit-tested in the
		// navigation lib: jsdom renders no edge SVG, so there is no edge DOM to
		// assert here.
		fireEvent.click(screen.getByTestId("outcome-inspector-close"));
		await waitFor(() => expect(screen.queryByTestId("outcome-inspector")).not.toBeInTheDocument());
		expect(container.querySelector(".react-flow__node.opacity-40")).toBeNull();
	});
});
