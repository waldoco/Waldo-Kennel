import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";

import { MissionWorkUnitGraph } from "./MissionWorkUnitGraph";

function unit(id: string, title: string, dependsOn: string[] = [], extra: Record<string, unknown> = {}) {
	return {
		id,
		title,
		dependsOn,
		kind: "direct",
		contractRevisionNumber: 1,
		criterionIds: [`crit-${id}`],
		evidenceChecks: ["evidence"],
		outputSummary: `${title} output`,
		requiredCapabilities: ["worktree.read"],
		stopConditions: ["stop"],
		verificationRequirement: "verify",
		provider: "codex",
		modelSelection: "provider_default",
		...extra,
	};
}

// The plan deliberately serializes B before A, with B depending on A. This is
// the assignment's own falsifier: dependency order must not come from array
// order.
const WORK_UNITS = [
	unit("wu-b", "Persist the parsed rows", ["wu-a"]),
	unit("wu-a", "Write the parser"),
	unit("wu-c", "Document the format"),
];

function scheduleFor(
	states: Record<string, string>,
	overrides: Record<string, unknown> = {},
	perUnit: Record<string, Record<string, unknown>> = {},
) {
	return {
		outcomeId: "out-1",
		plan: { id: "plan-1" },
		nextRunnableWorkUnitId: "wu-a",
		workUnits: WORK_UNITS.map((workUnit) => ({
			workUnit,
			state: states[workUnit.id] ?? "runnable",
			attempts: [],
			blockingDependencies: [],
			criterionReady: null,
			...(perUnit[workUnit.id] ?? {}),
		})),
		...overrides,
	} as never;
}

const criterionText = (id: string) => ({ "crit-wu-a": "The parser handles quoted fields" }[id]);

it("shows every approved work unit, in dependency order despite the array order", () => {
	render(<MissionWorkUnitGraph schedule={scheduleFor({})} workUnits={WORK_UNITS as never} />);
	const nodes = screen.getAllByTestId("mission-graph-node");
	expect(nodes).toHaveLength(3);
	// A and C are independent, so both sit on step 1; B is below A.
	expect(nodes.map((node) => node.textContent)).toEqual([
		expect.stringContaining("Write the parser"),
		expect.stringContaining("Document the format"),
		expect.stringContaining("Persist the parsed rows"),
	]);
});

it("draws proposed topology with no state overlay before authorization", () => {
	render(<MissionWorkUnitGraph workUnits={WORK_UNITS as never} />);
	for (const node of screen.getAllByTestId("mission-graph-node")) {
		expect(node).toHaveAttribute("data-state", "proposed");
	}
	// A proposal must not be presented as a runnable schedule.
	expect(screen.getByText(/Nothing is scheduled until you authorize/)).toBeInTheDocument();
});

// Waiting on a dependency's proof and waiting on the serial fence are different
// situations. The graph must say which, using titles rather than raw ids.
it("distinguishes waiting for proof from the workspace being held, by title", () => {
	render(
		<MissionWorkUnitGraph
			schedule={scheduleFor(
				{ "wu-a": "executing", "wu-b": "blocked", "wu-c": "blocked" },
				{ custodyHeldByWorkUnitId: "wu-a", noRunnableReason: "attempt_executing" },
				{
					"wu-b": { blockedReason: "awaiting_dependency_proof", blockingDependencies: ["wu-a"] },
					"wu-c": { blockedReason: "custody_held" },
				},
			)}
			workUnits={WORK_UNITS as never}
		/>,
	);
	expect(screen.getByText("Waiting for proof from: Write the parser")).toBeInTheDocument();
	expect(screen.getByText(/another work unit holds the workspace/)).toBeInTheDocument();
	// A raw WorkUnit id is technical detail and must not be the primary label.
	expect(screen.queryByText(/wu-a/)).not.toBeInTheDocument();
});

it("explains an empty runnable set instead of showing nothing", () => {
	render(
		<MissionWorkUnitGraph
			schedule={scheduleFor(
				{ "wu-a": "proven", "wu-b": "proven", "wu-c": "proven" },
				{ nextRunnableWorkUnitId: "", noRunnableReason: "all_units_proven" },
			)}
			workUnits={WORK_UNITS as never}
		/>,
	);
	expect(screen.getByTestId("mission-graph-no-runnable")).toHaveTextContent(
		/Every work unit is proved/,
	);
});

it("names an independent unit as waiting rather than as running alongside", () => {
	render(
		<MissionWorkUnitGraph
			schedule={scheduleFor(
				{ "wu-a": "executing", "wu-c": "blocked" },
				{ custodyHeldByWorkUnitId: "wu-a" },
				{ "wu-c": { blockedReason: "custody_held" } },
			)}
			workUnits={WORK_UNITS as never}
		/>,
	);
	const nodes = screen.getAllByTestId("mission-graph-node");
	const executing = nodes.filter((node) => node.getAttribute("data-state") === "executing");
	// Serial execution: exactly one unit may show as working.
	expect(executing).toHaveLength(1);
	expect(screen.getByText(/Kennel runs one work unit at a time/)).toBeInTheDocument();
});

it("shows a paused attempt as paused, not as working", () => {
	render(
		<MissionWorkUnitGraph
			schedule={scheduleFor({ "wu-a": "paused" }, { noRunnableReason: "attempt_paused" })}
			workUnits={WORK_UNITS as never}
		/>,
	);
	const paused = screen.getAllByTestId("mission-graph-node").find((node) => node.getAttribute("data-state") === "paused");
	expect(paused).toBeDefined();
	expect(within(paused as HTMLElement).getByText("Paused")).toBeInTheDocument();
	expect(screen.getByTestId("mission-graph-no-runnable")).toHaveTextContent(/Resume or cancel it/);
});

it("selects a node without starting anything", async () => {
	const onSelect = vi.fn();
	render(
		<MissionWorkUnitGraph
			onSelectWorkUnit={onSelect}
			schedule={scheduleFor({})}
			workUnits={WORK_UNITS as never}
		/>,
	);
	await userEvent.click(screen.getAllByTestId("mission-graph-node")[0]);
	expect(onSelect).toHaveBeenCalledWith("wu-a");
	// Selecting is inspection. The graph renders no action controls at all
	// beyond its three view controls and the nodes themselves, so neither
	// clicking nor navigating can begin execution — the graph has no mutation
	// callback in its props to begin with.
	const viewControls = within(screen.getByRole("group", { name: "Graph view" })).getAllByRole("button");
	expect(viewControls).toHaveLength(3);
	expect(screen.getAllByRole("button")).toHaveLength(viewControls.length + 3);
});

it("walks the whole graph with the keyboard in dependency order", async () => {
	const onSelect = vi.fn();
	render(
		<MissionWorkUnitGraph
			onSelectWorkUnit={onSelect}
			schedule={scheduleFor({})}
			selectedWorkUnitId="wu-a"
			workUnits={WORK_UNITS as never}
		/>,
	);
	const nodes = screen.getAllByTestId("mission-graph-node");
	// Roving tabindex: only the selected node is in the tab order.
	expect(nodes[0]).toHaveAttribute("tabindex", "0");
	expect(nodes[1]).toHaveAttribute("tabindex", "-1");

	nodes[0].focus();
	await userEvent.keyboard("{ArrowRight}");
	expect(onSelect).toHaveBeenLastCalledWith("wu-c");
	await userEvent.keyboard("{ArrowLeft}");
	expect(onSelect).toHaveBeenLastCalledWith("wu-a");
});

it("labels each node with its state and blocker for assistive technology", () => {
	render(
		<MissionWorkUnitGraph
			schedule={scheduleFor(
				{ "wu-b": "blocked" },
				{},
				{ "wu-b": { blockedReason: "awaiting_dependency_proof", blockingDependencies: ["wu-a"] } },
			)}
			workUnits={WORK_UNITS as never}
		/>,
	);
	expect(
		screen.getByRole("button", {
			name: /Persist the parsed rows — Waiting\. Waiting for proof from: Write the parser/,
		}),
	).toBeInTheDocument();
});

// provider_default is a real selection semantic. Inventing a concrete model
// name for it would misreport what was approved.
it("shows provider default as that semantic rather than a made-up model", () => {
	render(<MissionWorkUnitGraph schedule={scheduleFor({})} workUnits={WORK_UNITS as never} />);
	expect(screen.getAllByText(/codex · provider default/).length).toBeGreaterThan(0);
});

it("shows criterion text rather than criterion ids", () => {
	render(
		<MissionWorkUnitGraph
			criterionText={criterionText}
			schedule={scheduleFor({})}
			workUnits={WORK_UNITS as never}
		/>,
	);
	expect(screen.getByText("The parser handles quoted fields")).toBeInTheDocument();
	expect(screen.queryByText(/crit-wu-a/)).not.toBeInTheDocument();
});

it("keeps a selection that is still in the graph across a schedule refresh", () => {
	const { rerender } = render(
		<MissionWorkUnitGraph schedule={scheduleFor({})} selectedWorkUnitId="wu-b" workUnits={WORK_UNITS as never} />,
	);
	const selected = () => screen.getAllByTestId("mission-graph-node").find((node) => node.getAttribute("aria-current") === "true");
	expect(selected()).toHaveTextContent("Persist the parsed rows");
	// A CDC-driven refetch delivers new state for the same units.
	rerender(
		<MissionWorkUnitGraph
			schedule={scheduleFor({ "wu-a": "proven", "wu-b": "runnable" })}
			selectedWorkUnitId="wu-b"
			workUnits={WORK_UNITS as never}
		/>,
	);
	expect(selected()).toHaveTextContent("Persist the parsed rows");
});

it("renders nothing when the plan has no work units", () => {
	const { container } = render(<MissionWorkUnitGraph workUnits={[]} />);
	expect(container).toBeEmptyDOMElement();
});
