import { useState } from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import type { OutcomeRecord } from "../../hooks/useOutcome";
vi.mock("./OutcomesOverviewSurface", () => ({
	OutcomesOverviewSurface: ({
		onOpenOutcome,
	}: {
		onOpenOutcome: (project: string, outcome: OutcomeRecord, stage: string) => void;
	}) => {
		const [query, setQuery] = useState("");
		return (
			<div>
				<input aria-label="Search" value={query} onChange={(event) => setQuery(event.target.value)} />
				<button onClick={() => onOpenOutcome("project", { id: "out" } as OutcomeRecord, "decide_authorize")}>
					Open Outcome
				</button>
			</div>
		);
	},
}));
vi.mock("./OutcomeMissionPanel", () => ({
	OutcomeMissionPanel: ({
		onClose,
		onExpand,
		expanded,
	}: {
		onClose: () => void;
		onExpand: () => void;
		expanded: boolean;
	}) => (
		<div>
			<button onClick={onClose}>Close Mission</button>
			<button onClick={onExpand}>{expanded ? "Restore" : "Expand"}</button>
		</div>
	),
}));
import { OutcomeMissionWorkspace } from "./OutcomeMissionWorkspace";
function Harness() {
	const [selected, setSelected] = useState(false);
	return (
		<OutcomeMissionWorkspace
			outcomeId={selected ? "out" : undefined}
			projectId={selected ? "project" : undefined}
			onOpenOutcome={() => setSelected(true)}
			onClose={() => setSelected(false)}
		/>
	);
}
it("restores portfolio input and opener focus after expanded Mission closes", async () => {
	const user = userEvent.setup();
	render(<Harness />);
	await user.type(screen.getByRole("textbox"), "source");
	await user.click(screen.getByRole("button", { name: "Open Outcome" }));
	await user.click(screen.getByRole("button", { name: "Expand" }));
	await user.click(screen.getByRole("button", { name: "Close Mission" }));
	expect(screen.getByRole("textbox")).toHaveValue("source");
	await waitFor(() => expect(screen.getByRole("button", { name: "Open Outcome" })).toHaveFocus());
	await user.click(screen.getByRole("button", { name: "Open Outcome" }));
	expect(screen.getByRole("button", { name: "Expand" })).toBeInTheDocument();
});
it("keeps the split-pane resize affordance available at common desktop widths", async () => {
	render(<Harness />);
	await userEvent.click(screen.getByRole("button", { name: "Open Outcome" }));
	const divider = screen.getByRole("separator");
	expect(divider).toHaveClass("@[960px]/mission:block");
	expect(divider).not.toHaveClass("@[1050px]/mission:block");
});
it("resizes with keyboard and clamps at the panel bounds", async () => {
	render(<Harness />);
	await userEvent.click(screen.getByRole("button", { name: "Open Outcome" }));
	const divider = screen.getByRole("separator");
	fireEvent.keyDown(divider, { key: "ArrowLeft" });
	expect(divider).toHaveAttribute("aria-valuenow", "62");
	fireEvent.keyDown(divider, { key: "End" });
	fireEvent.keyDown(divider, { key: "ArrowLeft" });
	expect(divider).toHaveAttribute("aria-valuenow", "75");
	fireEvent.doubleClick(divider);
	expect(divider).toHaveAttribute("aria-valuenow", "60");
});
