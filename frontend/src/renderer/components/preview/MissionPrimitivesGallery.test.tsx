import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { MissionPrimitivesGallery } from "./MissionPrimitivesGallery";

describe("MissionPrimitivesGallery (dev/preview only)", () => {
	it("renders a section per primitive, driven only by typed fixtures", () => {
		render(<MissionPrimitivesGallery />);
		expect(screen.getByText(/MissionStatusChip/)).toBeInTheDocument();
		expect(screen.getByText(/SessionResponsibilityChip/)).toBeInTheDocument();
		expect(screen.getByText(/ActivityChip/)).toBeInTheDocument();
		expect(screen.getByText(/TaskRow/)).toBeInTheDocument();
		expect(screen.getByText(/ApprovalCard/)).toBeInTheDocument();
		expect(screen.getByText(/BoundedComposer/)).toBeInTheDocument();
		expect(screen.getByText(/SelectionActionBar/)).toBeInTheDocument();
		expect(screen.getByText(/InspectorShell/)).toBeInTheDocument();
	});

	it("shows every MissionStatusChip tone, each with visible text", () => {
		render(<MissionPrimitivesGallery />);
		expect(screen.getByText("Idle")).toBeInTheDocument();
		expect(screen.getByText("Executing")).toBeInTheDocument();
		expect(screen.getAllByText("Needs you").length).toBeGreaterThan(0);
		expect(screen.getAllByText("Blocked").length).toBeGreaterThan(0);
	});

	it("shows a disabled composer and a pending composer as distinct states", () => {
		render(<MissionPrimitivesGallery />);
		const submitButtons = screen.getAllByTestId("bounded-composer-submit");
		expect(submitButtons.some((button) => button.textContent === "Sending…")).toBe(true);
		expect(submitButtons.some((button) => button.hasAttribute("disabled"))).toBe(true);
	});

	it("shows a destructive selection action distinctly from a normal one", () => {
		render(<MissionPrimitivesGallery />);
		expect(screen.getByRole("button", { name: "Delete Contract draft" })).toHaveAttribute("data-destructive", "true");
		expect(screen.getByRole("button", { name: "Assign" })).not.toHaveAttribute("data-destructive");
	});
});
