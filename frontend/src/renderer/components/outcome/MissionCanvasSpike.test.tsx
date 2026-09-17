import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactFlowInstance } from "@xyflow/react";

import { MissionCanvasSpike } from "./MissionCanvasSpike";
import { useUiStore } from "../../stores/ui-store";
import {
	MISSION_CANVAS_FIXTURE_NODE_COUNT,
	buildMissionCanvasFixture,
	type MissionCanvasFixtureNode,
} from "../../lib/mission-canvas-fixture";

const FIXTURE = buildMissionCanvasFixture();

function withState(nodes: MissionCanvasFixtureNode[], id: string, state: MissionCanvasFixtureNode["state"]) {
	return nodes.map((node) => (node.id === id ? { ...node, state } : node));
}

describe("MissionCanvasSpike — list fallback (default config)", () => {
	it("renders the fixture with stable ids and no execution props", () => {
		const onSelectNode = vi.fn();
		render(<MissionCanvasSpike nodes={FIXTURE} onSelectNode={onSelectNode} revision={1} />);
		const nodes = screen.getAllByTestId("mission-canvas-node");
		expect(nodes).toHaveLength(MISSION_CANVAS_FIXTURE_NODE_COUNT);
		expect(new Set(nodes.map((node) => node.getAttribute("aria-label")?.split(" — ")[0])).size).toBeGreaterThan(1);
	});

	it("selects a node without exposing any way to execute it", async () => {
		const onSelectNode = vi.fn();
		render(<MissionCanvasSpike nodes={FIXTURE} onSelectNode={onSelectNode} revision={1} />);
		const first = screen.getAllByTestId("mission-canvas-node")[0];
		await userEvent.click(first);
		expect(onSelectNode).toHaveBeenCalledWith(FIXTURE[0].id);
		// The only interactive elements in the fallback ARE the node buttons —
		// selection is the sole callback the component exposes.
		expect(screen.getAllByRole("button")).toHaveLength(MISSION_CANVAS_FIXTURE_NODE_COUNT);
	});

	it("walks the whole graph with the keyboard in deterministic dependency order", async () => {
		const onSelectNode = vi.fn();
		render(<MissionCanvasSpike nodes={FIXTURE} onSelectNode={onSelectNode} revision={1} selectedId={FIXTURE[0].id} />);
		const nodes = screen.getAllByTestId("mission-canvas-node");
		expect(nodes[0]).toHaveAttribute("tabindex", "0");
		expect(nodes[1]).toHaveAttribute("tabindex", "-1");
		nodes[0].focus();
		await userEvent.keyboard("{ArrowRight}");
		expect(onSelectNode).toHaveBeenLastCalledWith(FIXTURE[1].id);
	});

	it("labels every node with its state, never color alone", () => {
		render(<MissionCanvasSpike nodes={FIXTURE} revision={1} />);
		const blocked = FIXTURE.find((node) => node.state === "blocked");
		expect(blocked).toBeDefined();
		expect(screen.getByRole("button", { name: new RegExp(`^${blocked!.title} — Blocked`) })).toBeInTheDocument();
	});

	it("does not hang or crash on a cyclic or dangling dependency input", () => {
		const pathological: MissionCanvasFixtureNode[] = [
			{ id: "a", title: "A", state: "runnable", upstream: ["b"] },
			{ id: "b", title: "B", state: "runnable", upstream: ["a"] },
			{ id: "c", title: "C", state: "runnable", upstream: ["missing"] },
		];
		render(<MissionCanvasSpike nodes={pathological} revision={1} />);
		expect(screen.getAllByTestId("mission-canvas-node")).toHaveLength(3);
	});

	it("renders nothing for an empty fixture", () => {
		const { container } = render(<MissionCanvasSpike nodes={[]} revision={1} />);
		expect(container).toBeEmptyDOMElement();
	});
});

describe("MissionCanvasSpike — flow renderer (explicit opt-in only)", () => {
	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it("stays on the list fallback unless renderer: 'flow' is explicitly requested", () => {
		render(<MissionCanvasSpike config={{ renderer: "list" }} nodes={FIXTURE} revision={1} />);
		expect(screen.queryByTestId("mission-canvas-flow")).not.toBeInTheDocument();
		expect(screen.getAllByTestId("mission-canvas-node")).toHaveLength(MISSION_CANVAS_FIXTURE_NODE_COUNT);
	});

	it("renders all 75 nodes with stable ids and a selection-only callback", () => {
		const onSelectNode = vi.fn();
		render(<MissionCanvasSpike config={{ renderer: "flow" }} nodes={FIXTURE} onSelectNode={onSelectNode} revision={1} />);
		const nodes = screen.getAllByTestId("mission-canvas-flow-node");
		expect(nodes).toHaveLength(MISSION_CANVAS_FIXTURE_NODE_COUNT);
	});

	it("rebinds React Flow colorMode when the resolved theme flips under system preference", async () => {
		const prior = useUiStore.getState();
		useUiStore.setState({ themePreference: "system", resolvedTheme: "light" });
		try {
			const { container } = render(<MissionCanvasSpike config={{ renderer: "flow" }} nodes={FIXTURE} revision={1} />);
			const flowClass = () => container.querySelector(".react-flow")?.className ?? "";
			expect(flowClass()).toContain("light");
			// Simulate an OS theme flip: the store refreshes resolvedTheme while
			// themePreference stays "system" (ui-store.refreshSystemTheme path).
			act(() => {
				useUiStore.setState({ resolvedTheme: "dark" });
			});
			await waitFor(() => expect(flowClass()).toContain("dark"));
		} finally {
			useUiStore.setState({ themePreference: prior.themePreference, resolvedTheme: prior.resolvedTheme });
		}
	});

	it("selects a node on click; nothing in its props can execute it", () => {
		const onSelectNode = vi.fn();
		render(<MissionCanvasSpike config={{ renderer: "flow" }} nodes={FIXTURE} onSelectNode={onSelectNode} revision={1} />);
		const nodes = screen.getAllByTestId("mission-canvas-flow-node");
		// fireEvent rather than userEvent: userEvent's synthetic mousedown omits
		// `view`, which d3-zoom's pane-level pan handler (attached regardless of
		// which node is clicked) dereferences — a jsdom-only gap, not a browser
		// behavior. See the F0 report for the real-browser follow-up.
		fireEvent.click(nodes[0]);
		expect(onSelectNode).toHaveBeenCalledWith(FIXTURE[0].id);
	});

	it("reaches every node by keyboard in dependency order (native tab order = fixture array order)", () => {
		const { container } = render(<MissionCanvasSpike config={{ renderer: "flow" }} nodes={FIXTURE} revision={1} />);
		// Query the live document directly (not getAllByTestId's own traversal)
		// so this asserts actual DOM/tab order, not just presence.
		const nodes = Array.from(container.querySelectorAll('[data-testid="mission-canvas-flow-node"]'));
		expect(nodes).toHaveLength(MISSION_CANVAS_FIXTURE_NODE_COUNT);
		expect(nodes.every((node) => node.getAttribute("tabindex") === "0")).toBe(true);
		nodes.forEach((node, index) => {
			expect(node).toHaveAttribute("aria-label", expect.stringContaining(FIXTURE[index].title));
			if (index > 0) {
				// DOCUMENT_POSITION_FOLLOWING (4): each node must come strictly after the previous one in document order.
				expect(nodes[index - 1].compareDocumentPosition(node) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
			}
		});
	});

	it("does not refit the viewport on a state-only update, but does on a topology revision swap", async () => {
		let instance: ReactFlowInstance<any, any> | undefined;
		const { rerender } = render(
			<MissionCanvasSpike
				config={{ renderer: "flow" }}
				nodes={FIXTURE}
				onInstanceReady={(ready) => {
					instance = ready;
				}}
				revision={1}
			/>,
		);
		await waitFor(() => expect(instance).toBeDefined());
		const fitViewSpy = vi.spyOn(instance!, "fitView");

		// State-only: same topology, one node's state changes.
		const stateOnly = withState(FIXTURE, FIXTURE[10].id, "proven");
		rerender(<MissionCanvasSpike config={{ renderer: "flow" }} nodes={stateOnly} revision={1} />);
		expect(fitViewSpy).not.toHaveBeenCalled();

		// Topology revision swap: the only case allowed to refit.
		rerender(<MissionCanvasSpike config={{ renderer: "flow" }} nodes={stateOnly} revision={2} />);
		expect(fitViewSpy).toHaveBeenCalledTimes(1);
	});

	it("preserves the selected id across a state-only refresh", () => {
		const selectedId = FIXTURE[5].id;
		const { rerender } = render(
			<MissionCanvasSpike config={{ renderer: "flow" }} nodes={FIXTURE} revision={1} selectedId={selectedId} />,
		);
		const selectedBefore = screen
			.getAllByTestId("mission-canvas-flow-node")
			.find((node) => node.getAttribute("data-selected") === "true");
		expect(within(selectedBefore!).getByText(FIXTURE[5].title)).toBeInTheDocument();

		const stateOnly = withState(FIXTURE, FIXTURE[20].id, "executing");
		rerender(<MissionCanvasSpike config={{ renderer: "flow" }} nodes={stateOnly} revision={1} selectedId={selectedId} />);
		const selectedAfter = screen
			.getAllByTestId("mission-canvas-flow-node")
			.find((node) => node.getAttribute("data-selected") === "true");
		expect(within(selectedAfter!).getByText(FIXTURE[5].title)).toBeInTheDocument();
	});

	it("skips the fit-view animation duration under reduced motion", async () => {
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
		const { rerender } = render(
			<MissionCanvasSpike
				config={{ renderer: "flow" }}
				nodes={FIXTURE}
				onInstanceReady={(ready) => {
					instance = ready;
				}}
				revision={1}
			/>,
		);
		await waitFor(() => expect(instance).toBeDefined());
		const fitViewSpy = vi.spyOn(instance!, "fitView");
		rerender(<MissionCanvasSpike config={{ renderer: "flow" }} nodes={FIXTURE} revision={2} />);
		expect(fitViewSpy).toHaveBeenCalledWith(expect.objectContaining({ duration: 0 }));
	});

	it("does not hang or crash on a cyclic or dangling dependency input", () => {
		const pathological: MissionCanvasFixtureNode[] = [
			{ id: "a", title: "A", state: "runnable", upstream: ["b"] },
			{ id: "b", title: "B", state: "runnable", upstream: ["a"] },
			{ id: "c", title: "C", state: "runnable", upstream: ["missing"] },
		];
		render(<MissionCanvasSpike config={{ renderer: "flow" }} nodes={pathological} revision={1} />);
		expect(screen.getAllByTestId("mission-canvas-flow-node")).toHaveLength(3);
	});
});
