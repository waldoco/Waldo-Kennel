import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { InspectorShell } from "./InspectorShell";

describe("InspectorShell", () => {
	it("renders title, summary, and no activity/footer sections by default", () => {
		render(<InspectorShell summary={<p>Summary content</p>} title="WorkUnit wu-1" />);
		expect(screen.getByText("WorkUnit wu-1")).toBeInTheDocument();
		expect(screen.getByTestId("inspector-shell-summary")).toHaveTextContent("Summary content");
		expect(screen.queryByTestId("inspector-shell-footer")).not.toBeInTheDocument();
	});

	it("renders the activity layer open and non-collapsible when no open state is supplied", () => {
		render(<InspectorShell activity={<p>Session events</p>} activityLabel="Activity" summary={<p>Summary</p>} title="t" />);
		expect(screen.getByTestId("inspector-shell-activity")).toHaveTextContent("Session events");
		expect(screen.queryByRole("button", { name: /Activity/ })).not.toBeInTheDocument();
	});

	it("lets the host control disclosure open/closed — this component holds no disclosure state itself", () => {
		const onActivityOpenChange = vi.fn();
		const { rerender } = render(
			<InspectorShell
				activity={<p>Session events</p>}
				activityLabel="Activity"
				activityOpen={false}
				onActivityOpenChange={onActivityOpenChange}
				summary={<p>Summary</p>}
				title="t"
			/>,
		);
		expect(screen.queryByTestId("inspector-shell-activity")).not.toBeInTheDocument();
		fireEvent.click(screen.getByRole("button", { name: /Activity/ }));
		expect(onActivityOpenChange).toHaveBeenCalledWith(true);

		// The click alone does not open it — only the host re-rendering with activityOpen=true does.
		expect(screen.queryByTestId("inspector-shell-activity")).not.toBeInTheDocument();
		rerender(
			<InspectorShell
				activity={<p>Session events</p>}
				activityLabel="Activity"
				activityOpen
				onActivityOpenChange={onActivityOpenChange}
				summary={<p>Summary</p>}
				title="t"
			/>,
		);
		expect(screen.getByTestId("inspector-shell-activity")).toBeInTheDocument();
	});

	it("renders an explicit footer with no auto-focus on anything inside it", () => {
		render(
			<InspectorShell
				footer={<button type="button">Accept</button>}
				summary={<p>Summary</p>}
				title="t"
			/>,
		);
		const footer = screen.getByTestId("inspector-shell-footer");
		expect(footer).toHaveTextContent("Accept");
		expect(document.activeElement).toBe(document.body);
	});

	it("orders summary before the activity disclosure before the footer in the DOM", () => {
		render(
			<InspectorShell
				activity={<p>Events</p>}
				activityLabel="Activity"
				footer={<button type="button">Accept</button>}
				summary={<p data-testid="summary-marker">Summary</p>}
				title="t"
			/>,
		);
		const shell = screen.getByTestId("inspector-shell");
		const summaryIndex = Array.from(shell.children).findIndex((el) => el.contains(screen.getByTestId("summary-marker")));
		const footerIndex = Array.from(shell.children).findIndex((el) => el === screen.getByTestId("inspector-shell-footer"));
		expect(summaryIndex).toBeLessThan(footerIndex);
	});
});
