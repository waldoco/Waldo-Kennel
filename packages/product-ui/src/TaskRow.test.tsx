import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { MissionStatusChip } from "./MissionStatusChip";
import { TaskRow } from "./TaskRow";

// The F2 chip takes pre-resolved presentation; these tests assert TaskRow's
// layout, not the chip, so one neutral resolved face stands in for all.
function statusChip(label: string) {
	return <MissionStatusChip className="text-status-idle" icon={<svg />} indicatorClassName="bg-status-idle" label={label} />;
}

describe("TaskRow", () => {
	it("renders identity, status, and freshness", () => {
		render(
			<TaskRow
				freshnessLabel="2h ago"
				statusChip={statusChip("Blocked")}
				title="Persist the parsed rows"
			/>,
		);
		expect(screen.getByText("Persist the parsed rows")).toBeInTheDocument();
		expect(screen.getByText("Blocked")).toBeInTheDocument();
		expect(screen.getByText("2h ago")).toBeInTheDocument();
	});

	it("exposes exactly one next-action area, never more", () => {
		render(
			<TaskRow
				nextAction={<button type="button">Resume</button>}
				statusChip={statusChip("Paused")}
				title="Migrate rows"
			/>,
		);
		expect(screen.getAllByTestId("task-row-next-action")).toHaveLength(1);
		expect(screen.getByRole("button", { name: "Resume" })).toBeInTheDocument();
	});

	it("renders with no next action when the host supplies none", () => {
		render(<TaskRow statusChip={statusChip("Idle")} title="Document the format" />);
		expect(screen.queryByTestId("task-row-next-action")).not.toBeInTheDocument();
	});

	it("truncates a long title rather than wrapping the row", () => {
		const longTitle = "A very long WorkUnit title that keeps going well past a reasonable row width";
		render(<TaskRow statusChip={statusChip("Runnable")} title={longTitle} />);
		expect(screen.getByText(longTitle)).toHaveClass("truncate");
	});

	it("renders with no freshness label when the host supplies none", () => {
		render(<TaskRow statusChip={statusChip("Unknown")} title="Undated task" />);
		expect(screen.queryByText(/ago$/)).not.toBeInTheDocument();
	});
});
