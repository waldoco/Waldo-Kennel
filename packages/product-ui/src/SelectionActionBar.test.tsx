import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { SelectionActionBar } from "./SelectionActionBar";

describe("SelectionActionBar", () => {
	it("renders nothing when there are no actions", () => {
		const { container } = render(<SelectionActionBar actions={[]} selectionLabel="3 selected" />);
		expect(container).toBeEmptyDOMElement();
	});

	it("renders the selection label and each action", () => {
		render(
			<SelectionActionBar
				actions={[
					{ id: "assign", label: "Assign", onClick: vi.fn() },
					{ id: "remove", label: "Remove", onClick: vi.fn() },
				]}
				selectionLabel="3 WorkUnits selected"
			/>,
		);
		expect(screen.getByText("3 WorkUnits selected")).toBeInTheDocument();
		expect(screen.getByRole("button", { name: "Assign" })).toBeInTheDocument();
		expect(screen.getByRole("button", { name: "Remove" })).toBeInTheDocument();
	});

	it("fires only the caller's own callback for an action — no built-in effect", () => {
		const onClick = vi.fn();
		render(<SelectionActionBar actions={[{ id: "a", label: "Assign", onClick }]} selectionLabel="1 selected" />);
		fireEvent.click(screen.getByRole("button", { name: "Assign" }));
		expect(onClick).toHaveBeenCalledTimes(1);
	});

	it("visually and semantically distinguishes a destructive action", () => {
		render(
			<SelectionActionBar
				actions={[{ id: "delete", label: "Delete", onClick: vi.fn(), destructive: true }]}
				selectionLabel="1 selected"
			/>,
		);
		const button = screen.getByRole("button", { name: "Delete" });
		expect(button).toHaveAttribute("data-destructive", "true");
		expect(button).toHaveClass("text-error");
	});

	it("respects a disabled action", () => {
		render(
			<SelectionActionBar
				actions={[{ id: "assign", label: "Assign", onClick: vi.fn(), disabled: true }]}
				selectionLabel="1 selected"
			/>,
		);
		expect(screen.getByRole("button", { name: "Assign" })).toBeDisabled();
	});
});
