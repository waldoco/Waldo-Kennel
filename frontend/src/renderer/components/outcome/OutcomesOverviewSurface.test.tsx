import { useUiStore } from "../../stores/ui-store";
vi.mock("../../hooks/useMissionAttention", () => ({
	useMissionAttention: (outcomes: Array<{ id: string }>) =>
		new Map(
			outcomes.map((outcome) => [outcome.id, outcome.id.startsWith("needs")
				? { lane: "needsYou", reason: "The provider needs a decision about the requested permission." }
				: { lane: outcome.id.startsWith("accepted") ? "accepted" : outcome.id.startsWith("review") ? "review" : "define" }]),
		),
}));
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { WorkspaceSummary } from "../../types/workspace";

// The Outcomes button in WorkShell must lead somewhere real. This surface is
// that destination: every Outcome across every project, read through the
// exact hooks the sidebar's project tree already uses — no new API calls, no
// locally derived stage/state.
const { workspaceQueryMock, projectOutcomesQueryMock } = vi.hoisted(() => ({
	workspaceQueryMock: vi.fn(),
	projectOutcomesQueryMock: vi.fn(),
}));

vi.mock("../../hooks/useWorkspaceQuery", () => ({
	useWorkspaceQuery: workspaceQueryMock,
}));

vi.mock("../../hooks/useOutcome", () => ({
	useProjectOutcomes: projectOutcomesQueryMock,
}));

import { OutcomesOverviewSurface } from "./OutcomesOverviewSurface";

function workspace(id: string, name: string): WorkspaceSummary {
	return { id, name, kind: "single_repo", path: `/repo/${id}`, type: "main", sessions: [] };
}

function outcome(id: string, title: string, parentId?: string) {
	return { id, title, currentRevisionNumber: 1, latestPlan: undefined, parentId } as never;
}

function renderSurface(onOpenOutcome = vi.fn()) {
	const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
	render(
		<QueryClientProvider client={queryClient}>
			<OutcomesOverviewSurface onOpenOutcome={onOpenOutcome} />
		</QueryClientProvider>,
	);
	return onOpenOutcome;
}

describe("OutcomesOverviewSurface", () => {
	beforeEach(() => {
		workspaceQueryMock.mockReset();
		projectOutcomesQueryMock.mockReset().mockReturnValue({ outcomes: [], isLoading: false, refetch: vi.fn() });
	});

	it("shows an honest empty state when there are no projects at all", () => {
		workspaceQueryMock.mockReturnValue({ data: [], isLoading: false });
		renderSurface();
		expect(screen.getByTestId("outcomes-overview-empty")).toBeInTheDocument();
	});

	it("lists every Outcome across every project, grouped by project", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspace("proj-1", "Waldo Kennel"), workspace("proj-2", "Kennel Island")],
			isLoading: false,
		});
		projectOutcomesQueryMock.mockImplementation((projectId: string) => ({
			outcomes: projectId === "proj-1" ? [outcome("out-1", "Ship the release")] : [outcome("out-2", "Fix the notch")],
			isLoading: false,
			refetch: vi.fn(),
		}));

		renderSurface();

		await waitFor(() => {
			expect(screen.getByRole("heading", { name: "Waldo Kennel" })).toBeInTheDocument();
			expect(screen.getByRole("heading", { name: "Kennel Island" })).toBeInTheDocument();
		});
		expect(screen.getByText("Ship the release")).toBeInTheDocument();
		expect(screen.getByText("Fix the notch")).toBeInTheDocument();
	});

	it("never no-ops on a click — opening a row calls back with the real project and outcome", async () => {
		workspaceQueryMock.mockReturnValue({ data: [workspace("proj-1", "Waldo Kennel")], isLoading: false });
		projectOutcomesQueryMock.mockReturnValue({
			outcomes: [outcome("out-1", "Ship the release")],
			isLoading: false,
			refetch: vi.fn(),
		});
		const user = userEvent.setup();
		const onOpenOutcome = renderSurface();

		await user.click(await screen.findByText("Ship the release"));
		expect(onOpenOutcome).toHaveBeenCalledTimes(1);
		expect(onOpenOutcome.mock.calls[0][0]).toBe("proj-1");
		expect(onOpenOutcome.mock.calls[0][1]).toMatchObject({ id: "out-1", title: "Ship the release" });
		expect(onOpenOutcome.mock.calls[0][2]).toBe("decide_authorize");
	});

	it("states why an Outcome needs the owner and opens the execution detail that can resolve it", async () => {
		workspaceQueryMock.mockReturnValue({ data: [workspace("proj-1", "Waldo Kennel")], isLoading: false });
		projectOutcomesQueryMock.mockReturnValue({
			outcomes: [outcome("needs-owner", "Resolve the provider question")],
			isLoading: false,
			refetch: vi.fn(),
		});
		const user = userEvent.setup();
		const onOpenOutcome = renderSurface();

		expect(await screen.findByText("The provider needs a decision about the requested permission.")).toBeVisible();
		await user.click(screen.getByText("Resolve the provider question"));
		expect(onOpenOutcome).toHaveBeenCalledWith(
			"proj-1",
			expect.objectContaining({ id: "needs-owner" }),
			"act_observe",
		);
	});

	it("opens a decomposed parent and shows contributors only on request", async () => {
		workspaceQueryMock.mockReturnValue({ data: [workspace("proj-1", "Waldo Kennel")], isLoading: false });
		projectOutcomesQueryMock.mockReturnValue({
			outcomes: [outcome("parent-1", "Ship the importer"), outcome("child-1", "Parse the archive", "parent-1")],
			isLoading: false,
			refetch: vi.fn(),
		});
		const user = userEvent.setup();
		const onOpenOutcome = renderSurface();

		await user.click(await screen.findByText("Ship the importer"));
		expect(onOpenOutcome).toHaveBeenLastCalledWith("proj-1", expect.objectContaining({ id: "parent-1" }), "decompose");

		// A contributor answers for its own contract, so it keeps the ordinary
		// destination — and is indented under the parent that claims it.
		expect(screen.queryByText("Parse the archive")).not.toBeInTheDocument();
		await user.click(screen.getByText("Filters"));
		await user.click(screen.getByRole("checkbox", { name: "Include contributing Outcomes" }));
		const contributor = screen.getByText("Parse the archive");
		expect(contributor.closest("li")).toHaveClass("pl-6");
		await user.click(contributor);
		expect(onOpenOutcome).toHaveBeenLastCalledWith(
			"proj-1",
			expect.objectContaining({ id: "child-1" }),
			"decide_authorize",
		);
	});

	it("reaches Mission Control for an Outcome nobody has decomposed yet", async () => {
		workspaceQueryMock.mockReturnValue({ data: [workspace("proj-1", "Waldo Kennel")], isLoading: false });
		projectOutcomesQueryMock.mockReturnValue({
			outcomes: [outcome("out-1", "Ship the release")],
			isLoading: false,
			refetch: vi.fn(),
		});
		const user = userEvent.setup();
		const onOpenOutcome = renderSurface();

		await user.click(await screen.findByRole("button", { name: "Mission control for Ship the release" }));
		expect(onOpenOutcome).toHaveBeenCalledWith("proj-1", expect.objectContaining({ id: "out-1" }), "decide_authorize");
	});

	it("offers no decomposition action on a contributing Outcome", async () => {
		workspaceQueryMock.mockReturnValue({ data: [workspace("proj-1", "Waldo Kennel")], isLoading: false });
		projectOutcomesQueryMock.mockReturnValue({
			outcomes: [outcome("parent-1", "Ship the importer"), outcome("child-1", "Parse the archive", "parent-1")],
			isLoading: false,
			refetch: vi.fn(),
		});
		renderSurface();

		expect(await screen.findByRole("button", { name: "Mission control for Ship the importer" })).toBeInTheDocument();
		// The depth limit is two levels: a contributor cannot be decomposed
		// again, so it must not offer the action.
		expect(screen.queryByRole("button", { name: "Mission control for Parse the archive" })).not.toBeInTheDocument();
	});

	it("surfaces a failed project's load error with a real retry, not a silent gap", async () => {
		workspaceQueryMock.mockReturnValue({ data: [workspace("proj-1", "Waldo Kennel")], isLoading: false });
		const refetch = vi.fn();
		projectOutcomesQueryMock.mockReturnValue({
			outcomes: [],
			isLoading: false,
			failure: { kind: "retryable", message: "boom" },
			refetch,
		});
		const user = userEvent.setup();
		renderSurface();

		const retry = await screen.findByRole("button", { name: /retry/i });
		await user.click(retry);
		expect(refetch).toHaveBeenCalledTimes(1);
	});
});

it("shows accepted Outcomes in Finished on the board", async () => {
	workspaceQueryMock.mockReturnValue({ data: [workspace("p", "Project")], isLoading: false });
	projectOutcomesQueryMock.mockReturnValue({
		outcomes: [outcome("active", "Current work"), outcome("accepted-one", "Accepted work")],
		isLoading: false,
		refetch: vi.fn(),
	});
	renderSurface();
	expect(screen.getByText("Current work")).toBeInTheDocument();
	expect(screen.getByText("Accepted work")).toBeInTheDocument();
	await userEvent.click(screen.getByText("Filters"));
	await userEvent.selectOptions(screen.getByRole("combobox", { name: "Outcome status" }), "history");
	expect(screen.getByText("Accepted work")).toBeInTheDocument();
});

it("groups the board into the canon columns and keeps the same Outcomes in List", async () => {
	workspaceQueryMock.mockReturnValue({ data: [workspace("p", "Project")], isLoading: false });
	projectOutcomesQueryMock.mockReturnValue({
		outcomes: [outcome("active", "Current work")],
		isLoading: false,
		refetch: vi.fn(),
	});
	useUiStore.setState({ outcomeRunViewMode: "board" });
	renderSurface();
	expect(screen.getByRole("heading", { name: /Needs choice/ })).toBeInTheDocument();
	expect(screen.getByRole("heading", { name: /Needs input/ })).toBeInTheDocument();
	expect(screen.getByRole("heading", { name: /Ready/ })).toBeInTheDocument();
	expect(screen.getByRole("heading", { name: /Running/ })).toBeInTheDocument();
	expect(screen.getByRole("heading", { name: /Finished/ })).toBeInTheDocument();
	expect(screen.queryByRole("heading", { name: /To do/ })).not.toBeInTheDocument();
	expect(screen.queryByRole("heading", { name: /In progress/ })).not.toBeInTheDocument();
	expect(screen.queryByRole("heading", { name: /Needs you$/ })).not.toBeInTheDocument();
	expect(screen.getByText("Filters").closest("details")).not.toHaveAttribute("open");
	act(() => useUiStore.setState({ outcomeRunViewMode: "list" }));
	expect(screen.getAllByTestId("outcomes-overview-row")).toHaveLength(1);
	expect(screen.getByText("Current work")).toBeInTheDocument();
});

it("hosts the List/Board switch in the overview header, wired to the shared store slice", async () => {
	workspaceQueryMock.mockReturnValue({ data: [workspace("p", "Project")], isLoading: false });
	projectOutcomesQueryMock.mockReturnValue({
		outcomes: [outcome("active", "Being defined")],
		isLoading: false, refetch: vi.fn(),
	});
	useUiStore.setState({ outcomeRunViewMode: "list" });
	renderSurface();
	const user = userEvent.setup();
	const switchEl = await screen.findByTestId("sessions-view-switch");
	expect(switchEl).toBeInTheDocument();
	await user.click(screen.getByRole("tab", { name: "Board" }));
	expect(useUiStore.getState().outcomeRunViewMode).toBe("board");
	await user.click(screen.getByRole("tab", { name: "List" }));
	expect(useUiStore.getState().outcomeRunViewMode).toBe("list");
});

it("places review Outcomes in the Ready lane, separate from Needs input", async () => {
	workspaceQueryMock.mockReturnValue({ data: [workspace("p", "Project")], isLoading: false });
	projectOutcomesQueryMock.mockReturnValue({
		outcomes: [outcome("review-one", "Ready for owner review"), outcome("needs-one", "Waiting on an answer"), outcome("active", "Being defined")],
		isLoading: false, refetch: vi.fn(),
	});
	useUiStore.setState({ outcomeRunViewMode: "board" });
	renderSurface();
	const readyHeading = await screen.findByRole("heading", { name: /Ready/ });
	const readyLane = readyHeading.closest("section");
	expect(readyLane).not.toBeNull();
	expect(readyLane!.textContent).toContain("Ready for owner review");
	expect(readyLane!.textContent).not.toContain("Waiting on an answer");
	const inputHeading = screen.getByRole("heading", { name: /Needs input/ });
	const inputLane = inputHeading.closest("section");
	expect(inputLane!.textContent).toContain("Waiting on an answer");
	expect(inputLane!.textContent).not.toContain("Ready for owner review");
});

it("keeps reviewable Outcomes in the Needs you filter and applies Active consistently", async () => {
	workspaceQueryMock.mockReturnValue({ data: [workspace("p", "Project")], isLoading: false });
	projectOutcomesQueryMock.mockReturnValue({
		outcomes: [outcome("review-one", "Ready for owner review"), outcome("accepted-one", "Finished work")],
		isLoading: false, refetch: vi.fn(),
	});
	useUiStore.setState({ outcomeRunViewMode: "board" });
	renderSurface();
	expect(screen.getByText("Finished work")).toBeInTheDocument();
	await userEvent.click(screen.getByText("Filters"));
	const filter = screen.getByRole("combobox", { name: "Outcome status" });
	await userEvent.selectOptions(filter, "needsYou");
	expect(screen.getByText("Ready for owner review")).toBeInTheDocument();
	expect(screen.queryByText("Finished work")).not.toBeInTheDocument();
	await userEvent.selectOptions(filter, "active");
	expect(screen.queryByText("Finished work")).not.toBeInTheDocument();
	act(() => useUiStore.setState({ outcomeRunViewMode: "list" }));
	expect(screen.getByText("Ready for owner review")).toBeInTheDocument();
	expect(screen.queryByText("Finished work")).not.toBeInTheDocument();
});
