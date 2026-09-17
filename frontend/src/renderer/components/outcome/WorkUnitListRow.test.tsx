import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { WorkUnitListRow } from "./WorkUnitListRow";
import type { MissionNodeView } from "./mission-presentation";

function nodeView(overrides: Partial<MissionNodeView> = {}): MissionNodeView {
	return {
		workUnitId: "wu-1",
		title: "Ship the thing",
		status: { status: "runnable", label: "Ready", className: "text-status-ready", indicatorClassName: "bg-status-ready" },
		criteria: { kind: "ready", ready: 1, total: 2 },
		responsibilityLabel: "Unconfirmed",
		dependencyUnavailable: false,
		dependencyCount: 0,
		dependentCount: 0,
		generation: 1,
		...overrides,
	};
}

describe("WorkUnitListRow", () => {
	it("renders the bounded node face: title, status, criteria — and only a returned action", () => {
		const onStart = vi.fn();
		render(
			<WorkUnitListRow
				node={nodeView({ action: { kind: "start", label: "Start" } })}
				onSelect={() => {}}
				onStart={onStart}
				selected={false}
			/>,
		);
		expect(screen.getByText("Ship the thing")).toBeInTheDocument();
		expect(screen.getByText("Ready")).toBeInTheDocument();
		expect(screen.getByText("1/2 criteria ready")).toBeInTheDocument();
		expect(screen.getByTestId("mission-row-action")).toHaveTextContent("Start");
	});

	it("never shows raw IDs in the visible face", () => {
		render(<WorkUnitListRow node={nodeView()} onSelect={() => {}} selected={false} />);
		expect(screen.queryByText(/wu-1/)).not.toBeInTheDocument();
	});

	it("renders no action button when the node has none (a retryable node without a returned action stays non-actionable)", () => {
		render(<WorkUnitListRow node={nodeView({ status: { status: "retryable", label: "Needs retry", className: "", indicatorClassName: "" } })} onSelect={() => {}} selected={false} />);
		expect(screen.queryByTestId("mission-row-action")).not.toBeInTheDocument();
	});

	it("shows the dependency-unavailable marker only when the node flags it", () => {
		render(<WorkUnitListRow node={nodeView({ dependencyUnavailable: true })} onSelect={() => {}} selected={false} />);
		expect(screen.getByTestId("mission-row-dependency-unavailable")).toBeInTheDocument();
	});

	it("calls onSelect on click and on Enter/Space", async () => {
		const user = userEvent.setup();
		const onSelect = vi.fn();
		render(<WorkUnitListRow node={nodeView()} onSelect={onSelect} selected={false} />);
		await user.click(screen.getByTestId("mission-list-row-wu-1"));
		expect(onSelect).toHaveBeenCalledTimes(1);
		screen.getByTestId("mission-list-row-wu-1").focus();
		await user.keyboard("{Enter}");
		expect(onSelect).toHaveBeenCalledTimes(2);
	});

	it("clicking Start does not also trigger row selection twice from event bubbling in a way that double-fires", async () => {
		const user = userEvent.setup();
		const onSelect = vi.fn();
		const onStart = vi.fn();
		render(
			<WorkUnitListRow
				node={nodeView({ action: { kind: "start", label: "Start" } })}
				onSelect={onSelect}
				onStart={onStart}
				selected={false}
			/>,
		);
		await user.click(screen.getByTestId("mission-row-action"));
		expect(onStart).toHaveBeenCalledTimes(1);
	});
});
