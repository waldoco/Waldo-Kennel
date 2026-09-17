import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ContractCoverage } from "./ContractCoverage";

describe("ContractCoverage", () => {
	it("renders an explicit unavailable reading instead of zero-ready when unresolved", () => {
		render(<ContractCoverage unavailable unavailableLabel="Criterion unavailable" />);
		expect(screen.getByTestId("contract-coverage-unavailable")).toHaveTextContent("Criterion unavailable");
	});

	it("renders each criterion by position, never a raw ID", () => {
		render(
			<ContractCoverage
				criterionLabel={(position) => `Criterion ${position}`}
				items={[
					{ position: 1, ready: true },
					{ position: 2, ready: false },
				]}
				pendingLabel="Pending"
				readyLabel="Ready"
			/>,
		);
		expect(screen.getByText("Criterion 1")).toBeInTheDocument();
		expect(screen.getByText("Criterion 2")).toBeInTheDocument();
		expect(screen.getAllByText("Ready")).toHaveLength(1);
		expect(screen.getAllByText("Pending")).toHaveLength(1);
	});
});
