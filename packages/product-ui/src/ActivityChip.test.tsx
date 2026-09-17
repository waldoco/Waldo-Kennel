import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ActivityChip } from "./ActivityChip";

describe("ActivityChip", () => {
	it("renders its label as secondary, muted text", () => {
		render(<ActivityChip label="3 checks passed" />);
		expect(screen.getByTestId("activity-chip")).toHaveTextContent("3 checks passed");
		expect(screen.getByTestId("activity-chip")).toHaveClass("text-passive");
	});

	it("renders without an icon when none is supplied", () => {
		render(<ActivityChip label="2 unresolved comments" />);
		expect(screen.getByTestId("activity-chip").querySelector("svg")).not.toBeInTheDocument();
	});

	it("renders a supplied icon, hidden from assistive tech since the label carries the meaning", () => {
		render(<ActivityChip icon={<svg data-testid="evidence-icon" />} label="Evidence attached" />);
		expect(screen.getByTestId("evidence-icon")).toBeInTheDocument();
		expect(screen.getByTestId("evidence-icon").parentElement).toHaveAttribute("aria-hidden", "true");
	});
});
