import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { MissionStatusChip } from "./MissionStatusChip";

describe("MissionStatusChip", () => {
	it("always renders visible text alongside the tone, never color alone", () => {
		render(<MissionStatusChip label="Needs you" tone="warning" />);
		expect(screen.getByTestId("mission-status-chip")).toHaveTextContent("Needs you");
	});

	it("renders a default glyph plus text when no icon is supplied", () => {
		render(<MissionStatusChip label="Blocked" tone="danger" />);
		const chip = screen.getByTestId("mission-status-chip");
		expect(chip.querySelector("svg")).toBeInTheDocument();
		expect(chip).toHaveTextContent("Blocked");
	});

	it("renders a caller-supplied icon instead of the default dot", () => {
		render(<MissionStatusChip icon={<span data-testid="custom-icon" />} label="Proven" tone="positive" />);
		expect(screen.getByTestId("custom-icon")).toBeInTheDocument();
	});

	(["neutral", "info", "positive", "warning", "danger"] as const).forEach((tone) => {
		it(`exposes the "${tone}" tone as a data attribute for styling, not as the only signal`, () => {
			render(<MissionStatusChip label={`tone ${tone}`} tone={tone} />);
			expect(screen.getByTestId("mission-status-chip")).toHaveAttribute("data-tone", tone);
		});
	});

	it("truncates a long label rather than overflowing its container", () => {
		render(<MissionStatusChip label="A very long status label that should not blow out the layout" tone="neutral" />);
		expect(screen.getByText(/A very long status label/)).toHaveClass("truncate");
	});
});
