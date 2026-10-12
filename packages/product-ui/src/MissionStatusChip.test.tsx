import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { MissionAttentionStrip } from "./MissionAttentionStrip";
import { MissionStatusChip } from "./MissionStatusChip";

describe("MissionStatusChip", () => {
	it("renders the label together with a visible icon, never color alone", () => {
		render(
			<MissionStatusChip
				className="text-status-ready"
				icon={<svg data-testid="status-icon" />}
				indicatorClassName="bg-status-ready"
				label="Ready"
			/>,
		);
		expect(screen.getByText("Ready")).toBeInTheDocument();
		expect(screen.getByTestId("status-icon")).toBeInTheDocument();
	});

	it("renders optional detail text (an attempt/session label) alongside the status", () => {
		render(
			<MissionStatusChip
				className="text-status-working"
				detail="Attempt 2"
				icon={<svg />}
				indicatorClassName="bg-status-working"
				label="Running"
			/>,
		);
		expect(screen.getByText("Attempt 2")).toBeInTheDocument();
	});
});

describe("MissionAttentionStrip", () => {
	it("counts approval, choice, and input separately", () => {
		render(
			<MissionAttentionStrip
				items={[
					{ className: "", count: 2, icon: <svg />, indicatorClassName: "", kind: "needs_approval", label: "Needs approval" },
					{ className: "", count: 1, icon: <svg />, indicatorClassName: "", kind: "needs_choice", label: "Needs choice" },
					{ className: "", count: 3, icon: <svg />, indicatorClassName: "", kind: "needs_input", label: "Needs input" },
				]}
			/>,
		);
		const kind = (name: string) => document.querySelector(`[data-mission-attention-kind="${name}"]`);
		expect(kind("needs_approval")).toHaveTextContent(/^Needs approval\s*2$/);
		expect(kind("needs_choice")).toHaveTextContent(/^Needs choice\s*1$/);
		expect(kind("needs_input")).toHaveTextContent(/^Needs input\s*3$/);
		// Quiet needs-you state, never a bordered coloured pill.
		expect(kind("needs_approval")).toHaveAttribute("data-state-tone", "needsYou");
		expect(kind("needs_approval")?.className).not.toMatch(/\bborder\b|rounded-full/);
	});

	it("renders nothing visible for a zero count", () => {
		render(
			<MissionAttentionStrip
				items={[{ className: "", count: 0, icon: <svg />, indicatorClassName: "", kind: "needs_approval", label: "Needs approval" }]}
			/>,
		);
		expect(screen.queryByText(/Needs approval/)).not.toBeInTheDocument();
	});
});
