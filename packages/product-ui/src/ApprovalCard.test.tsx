import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ApprovalCard } from "./ApprovalCard";

describe("ApprovalCard", () => {
	it("renders the requested action, reason, and scope as plain facts", () => {
		render(
			<ApprovalCard
				denyLabel="Deny"
				onDeny={vi.fn()}
				onPrimary={vi.fn()}
				primaryLabel="Approve"
				reason="Needed to install the missing dependency"
				requestedAction="Add @xyflow/react to frontend/package.json"
				scopeLabel="frontend/package.json only"
			/>,
		);
		expect(screen.getByText("Add @xyflow/react to frontend/package.json")).toBeInTheDocument();
		expect(screen.getByText("Needed to install the missing dependency")).toBeInTheDocument();
		expect(screen.getByText("frontend/package.json only")).toBeInTheDocument();
	});

	it("approves directly when there are no bounded options", () => {
		const onPrimary = vi.fn();
		render(
			<ApprovalCard
				denyLabel="Deny"
				onDeny={vi.fn()}
				onPrimary={onPrimary}
				primaryLabel="Approve"
				reason="r"
				requestedAction="a"
				scopeLabel="s"
			/>,
		);
		fireEvent.click(screen.getByTestId("approval-card-primary"));
		expect(onPrimary).toHaveBeenCalledWith(undefined);
	});

	it("disables the primary action until a bounded option is selected — no prose-to-authority inference", () => {
		const onPrimary = vi.fn();
		const onSelectOption = vi.fn();
		render(
			<ApprovalCard
				denyLabel="Deny"
				onDeny={vi.fn()}
				onPrimary={onPrimary}
				onSelectOption={onSelectOption}
				options={[
					{ id: "merge", label: "Merge" },
					{ id: "rework", label: "Request rework" },
				]}
				primaryLabel="Approve"
				reason="r"
				requestedAction="a"
				scopeLabel="s"
			/>,
		);
		expect(screen.getByTestId("approval-card-primary")).toBeDisabled();
		fireEvent.click(screen.getByRole("radio", { name: /Merge/ }));
		expect(onSelectOption).toHaveBeenCalledWith("merge");
	});

	it("calls onPrimary with the selected option id once one is chosen", () => {
		const onPrimary = vi.fn();
		render(
			<ApprovalCard
				denyLabel="Deny"
				onDeny={vi.fn()}
				onPrimary={onPrimary}
				options={[{ id: "merge", label: "Merge" }]}
				primaryLabel="Approve"
				reason="r"
				requestedAction="a"
				scopeLabel="s"
				selectedOptionId="merge"
			/>,
		);
		fireEvent.click(screen.getByTestId("approval-card-primary"));
		expect(onPrimary).toHaveBeenCalledWith("merge");
	});

	it("keeps deny explicit and independent from the primary action", () => {
		const onDeny = vi.fn();
		const onPrimary = vi.fn();
		render(
			<ApprovalCard
				denyLabel="Deny"
				onDeny={onDeny}
				onPrimary={onPrimary}
				primaryLabel="Approve"
				reason="r"
				requestedAction="a"
				scopeLabel="s"
			/>,
		);
		fireEvent.click(screen.getByTestId("approval-card-deny"));
		expect(onDeny).toHaveBeenCalledTimes(1);
		expect(onPrimary).not.toHaveBeenCalled();
	});

	it("walks bounded options with the keyboard", () => {
		const onSelectOption = vi.fn();
		render(
			<ApprovalCard
				denyLabel="Deny"
				onDeny={vi.fn()}
				onPrimary={vi.fn()}
				onSelectOption={onSelectOption}
				options={[
					{ id: "a", label: "Option A" },
					{ id: "b", label: "Option B" },
				]}
				primaryLabel="Approve"
				reason="r"
				requestedAction="a"
				scopeLabel="s"
			/>,
		);
		const radios = screen.getAllByRole("radio");
		fireEvent.keyDown(radios[0], { key: "ArrowDown" });
		expect(onSelectOption).toHaveBeenCalledWith("b");
	});
});
