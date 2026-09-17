import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { MissionStatusChip } from "./MissionStatusChip";
import { TaskRow } from "./TaskRow";

describe("TaskRow", () => {
	it("renders identity, status, and freshness", () => {
		render(
			<TaskRow
				freshnessLabel="2h ago"
				statusChip={<MissionStatusChip label="Blocked" tone="warning" />}
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
				statusChip={<MissionStatusChip label="Paused" tone="neutral" />}
				title="Migrate rows"
			/>,
		);
		expect(screen.getAllByTestId("task-row-next-action")).toHaveLength(1);
		expect(screen.getByRole("button", { name: "Resume" })).toBeInTheDocument();
	});

	it("renders with no next action when the host supplies none", () => {
		render(<TaskRow statusChip={<MissionStatusChip label="Idle" tone="neutral" />} title="Document the format" />);
		expect(screen.queryByTestId("task-row-next-action")).not.toBeInTheDocument();
	});

	it("truncates a long title rather than wrapping the row", () => {
		const longTitle = "A very long WorkUnit title that keeps going well past a reasonable row width";
		render(<TaskRow statusChip={<MissionStatusChip label="Runnable" tone="info" />} title={longTitle} />);
		expect(screen.getByText(longTitle)).toHaveClass("truncate");
	});

	it("renders with no freshness label when the host supplies none", () => {
		render(<TaskRow statusChip={<MissionStatusChip label="Unknown" tone="neutral" />} title="Undated task" />);
		expect(screen.queryByText(/ago$/)).not.toBeInTheDocument();
	});
});
