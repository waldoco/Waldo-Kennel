import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { SessionResponsibilityChip } from "./SessionResponsibilityChip";

describe("SessionResponsibilityChip", () => {
	it("renders the icon and text together, never icon alone", () => {
		render(
			<SessionResponsibilityChip
				icon={<svg data-testid="agent-icon" />}
				label="Agent is working"
				responsibility="agent"
			/>,
		);
		expect(screen.getByTestId("agent-icon")).toBeInTheDocument();
		expect(screen.getByTestId("session-responsibility-chip")).toHaveTextContent("Agent is working");
	});

	(["agent", "owner", "shared"] as const).forEach((responsibility) => {
		it(`exposes "${responsibility}" as a data attribute`, () => {
			render(<SessionResponsibilityChip icon={<svg />} label={responsibility} responsibility={responsibility} />);
			expect(screen.getByTestId("session-responsibility-chip")).toHaveAttribute("data-responsibility", responsibility);
		});
	});

	it("hides the icon from assistive tech since the label already carries the meaning", () => {
		render(<SessionResponsibilityChip icon={<svg />} label="Owner must decide" responsibility="owner" />);
		const iconWrapper = screen.getByTestId("session-responsibility-chip").firstElementChild;
		expect(iconWrapper).toHaveAttribute("aria-hidden", "true");
	});
});
