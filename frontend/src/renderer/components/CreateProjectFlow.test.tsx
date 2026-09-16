// Focused render test for the stable selectors added for the packaged
// Outcome journey harness (docs/handoffs/2026-09-16-macos-outcome-journey-
// harness-plan.md §6): CreateProjectFlow had no data-testids before this
// change, and its buttons are the harness's only way to register the
// disposable fixture project.
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { it, expect, vi } from "vitest";
import { CreateProjectFlow } from "./CreateProjectFlow";

it("exposes stable entry-type selectors for the packaged Outcome journey harness", () => {
	// CreateProjectAgentSheet is always mounted (visibility is Radix's "open"
	// prop, not conditional rendering) and calls useQuery unconditionally, so a
	// QueryClientProvider is required even though this test never opens it.
	const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
	render(
		<QueryClientProvider client={client}>
			<CreateProjectFlow
				embedded
				mode="choose"
				onCreateProject={vi.fn().mockResolvedValue(undefined)}
				onInitializeProject={vi.fn().mockResolvedValue(undefined)}
			/>
		</QueryClientProvider>,
	);
	expect(screen.getByTestId("create-project-new")).toBeInTheDocument();
	expect(screen.getByTestId("create-project-import-existing")).toBeInTheDocument();
	expect(screen.getByTestId("create-project-import-workspace")).toBeInTheDocument();
});
