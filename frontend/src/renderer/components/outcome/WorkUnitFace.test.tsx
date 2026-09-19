import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { WorkUnitFace } from "./WorkUnitFace";
import type { MissionNodeView } from "./mission-presentation";

function nodeView(overrides: Partial<MissionNodeView> = {}): MissionNodeView {
	return {
		workUnitId: "wu-1",
		title: "Bind resume to the same worktree lease",
		status: { status: "executing", label: "Running", className: "text-status-working", indicatorClassName: "bg-status-working" },
		attention: {
			kind: "needs_choice",
			visualCategory: "owner_decision",
			label: "Decision needed",
			summary: "Owner decision needed: keep the retry budget at 2?",
			className: "text-status-needs-you",
			indicatorClassName: "bg-status-needs-you",
			markerKey: "wu-1:g:needs_choice",
		},
		attemptLabel: "Attempt 2",
		criteria: { kind: "ready", ready: 1, total: 3 },
		responsibilityLabel: "Agent",
		dependencyUnavailable: false,
		dependencyCount: 1,
		dependentCount: 2,
		generation: 4,
		...overrides,
	};
}

describe("WorkUnitFace", () => {
	it("node layout renders the bounded card: rail, clamped title, status chip, readiness, one attention marker", () => {
		render(<WorkUnitFace layout="node" view={nodeView()} />);
		const face = screen.getByTestId("mission-node-face-wu-1");
		expect(face.querySelector("[aria-hidden='true']")).toHaveClass("bg-status-working");
		expect(screen.getByText("Bind resume to the same worktree lease")).toHaveClass("line-clamp-2");
		expect(screen.getByText("Running")).toBeInTheDocument();
		expect(screen.getByText("Attempt 2")).toBeInTheDocument();
		expect(screen.getByText("1/3 criteria ready")).toBeInTheDocument();
		expect(screen.getByTestId("mission-row-attention")).toHaveTextContent("Decision needed");
	});

	it("node layout renders no action even when the view carries one - the canvas is read-only", () => {
		render(<WorkUnitFace layout="node" view={nodeView({ action: { kind: "start", label: "Start" } })} />);
		expect(screen.queryByTestId("mission-row-action")).not.toBeInTheDocument();
		expect(screen.queryByRole("button")).not.toBeInTheDocument();
	});

	it("node layout keeps unavailable criteria honest instead of reading zero", () => {
		render(<WorkUnitFace layout="node" view={nodeView({ criteria: { kind: "unavailable" } })} />);
		expect(screen.getByText("Criterion unavailable")).toBeInTheDocument();
		expect(screen.queryByText(/criteria ready/)).not.toBeInTheDocument();
	});

	it("row layout renders the same bounded atoms the List row has always shown", () => {
		render(<WorkUnitFace layout="row" view={nodeView()} />);
		expect(screen.getByText("Running")).toBeInTheDocument();
		expect(screen.getByText("Bind resume to the same worktree lease")).toHaveClass("truncate");
		expect(screen.getByText("1/3 criteria ready")).toBeInTheDocument();
		expect(screen.getByTestId("mission-row-attention")).toHaveTextContent("Decision needed");
	});

	it("row layout renders no action either - the action belongs to the List row container", () => {
		render(<WorkUnitFace layout="row" view={nodeView({ action: { kind: "start", label: "Start" } })} />);
		expect(screen.queryByTestId("mission-row-action")).not.toBeInTheDocument();
	});

	it("never shows raw IDs in either layout", () => {
		render(<WorkUnitFace layout="node" view={nodeView()} />);
		render(<WorkUnitFace layout="row" view={nodeView()} />);
		expect(screen.queryByText(/wu-1/)).not.toBeInTheDocument();
	});

	it("shows the dependency-unavailable marker in both layouts when the node flags it", () => {
		render(<WorkUnitFace layout="node" view={nodeView({ dependencyUnavailable: true })} />);
		render(<WorkUnitFace layout="row" view={nodeView({ dependencyUnavailable: true })} />);
		expect(screen.getAllByTestId("mission-row-dependency-unavailable")).toHaveLength(2);
	});

	it("row layout emits the exact prior List atom classes - no node-only nowrap/shrink hardening leaks into the shipped row", () => {
		const { container } = render(<WorkUnitFace layout="row" view={nodeView({ dependencyUnavailable: true })} />);
		expect(container.querySelector(".whitespace-nowrap")).toBeNull();
		// The chip root carries exactly the view's tone class - the node-only
		// shrink/nowrap hardening must not land on it (the dot's own internal
		// shrink-0 inside MissionStatusChip predates this slice and stays).
		const chipRoot = screen.getByText("Running").closest("[class*='text-status-working']");
		expect(chipRoot).toHaveClass("text-status-working");
		expect(chipRoot).not.toHaveClass("shrink-0");
		expect(chipRoot).not.toHaveClass("whitespace-nowrap");
		expect(screen.getByTestId("mission-row-attention")).toHaveClass("inline-flex items-center gap-1 text-xs font-medium text-status-needs-you", { exact: true });
		expect(screen.getByTestId("mission-row-dependency-unavailable")).toHaveClass("text-xs font-medium text-status-unknown", { exact: true });
		expect(screen.getByText("1/3 criteria ready")).toHaveClass("text-muted-foreground text-xs", { exact: true });
		expect(screen.getByText("Bind resume to the same worktree lease")).toHaveClass("min-w-0 flex-1 truncate text-sm font-medium", { exact: true });
	});

	it("node layout applies the nowrap/shrink hardening only to its own card", () => {
		const { container } = render(<WorkUnitFace layout="node" view={nodeView({ dependencyUnavailable: true })} />);
		expect(container.querySelectorAll(".whitespace-nowrap").length).toBeGreaterThan(0);
	});

	it("renders the explicit unavailable status face for an unrecognized daemon state", () => {
		render(<WorkUnitFace layout="node" view={nodeView({ status: { status: "unavailable", label: "State unavailable", className: "text-status-unknown", indicatorClassName: "bg-status-unknown" } })} />);
		expect(screen.getByText("State unavailable")).toBeInTheDocument();
		expect(screen.getByTestId("mission-node-face-wu-1").querySelector("[aria-hidden='true']")).toHaveClass("bg-status-unknown");
	});
});
