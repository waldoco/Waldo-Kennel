import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useUiStore } from "../../stores/ui-store";
import { TooltipProvider } from "../ui/tooltip";

// Locked contract under test: WorkShell is the persistent chrome every Work
// stage renders inside — it must always show real navigation (back/forward,
// sidebar toggle, search, notifications, List/Board, terminal toggle,
// Outcomes), never an inert control masquerading as one, and the terminal
// toggle must reflect real attempt data, never local guesswork.
const { navigateMock, attemptsQueryMock, historyBackMock, historyForwardMock, canGoBackRef } = vi.hoisted(() => ({
	navigateMock: vi.fn(),
	attemptsQueryMock: vi.fn(),
	historyBackMock: vi.fn(),
	historyForwardMock: vi.fn(),
	canGoBackRef: { current: false },
}));

vi.mock("@tanstack/react-router", () => ({
	useNavigate: () => navigateMock,
	useCanGoBack: () => canGoBackRef.current,
	useRouter: () => ({
		history: {
			back: historyBackMock,
			forward: historyForwardMock,
			location: { state: { __TSR_index: 0 } },
			subscribe: () => () => {},
		},
	}),
}));

vi.mock("../../hooks/useOutcome", () => ({
	useOutcomeAttempts: attemptsQueryMock,
}));

vi.mock("./OutcomeAttemptTerminalPanel", () => ({
	OutcomeAttemptTerminalPanel: ({ attempt, onClose }: { attempt: { id: string }; onClose: () => void }) => (
		<div data-testid="mock-attempt-panel">
			<span>{attempt.id}</span>
			<button onClick={onClose} type="button">
				close
			</button>
		</div>
	),
}));

vi.mock("../NotificationCenter", () => ({
	NotificationCenter: () => <div data-testid="mock-notification-center" />,
}));

import { WorkShell } from "./WorkShell";

function renderShell(props: Partial<React.ComponentProps<typeof WorkShell>> = {}) {
	const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
	return render(
		<QueryClientProvider client={queryClient}>
			<TooltipProvider>
				<WorkShell projectId="proj-1" {...props}>
					<div data-testid="stage-body">stage content</div>
				</WorkShell>
			</TooltipProvider>
		</QueryClientProvider>,
	);
}

describe("WorkShell", () => {
	beforeEach(() => {
		navigateMock.mockClear();
		historyBackMock.mockClear();
		historyForwardMock.mockClear();
		canGoBackRef.current = false;
		attemptsQueryMock.mockReset().mockReturnValue({ attempts: [], isLoading: false, refetch: vi.fn() });
		useUiStore.setState({
			isOutcomeAttemptPanelOpen: false,
			outcomeAttemptPanelAttemptId: null,
			outcomeRunViewMode: "board",
			isCommandPaletteOpen: false,
			isKeyboardShortcutsOpen: false,
			isSidebarOpen: true,
		});
	});

	afterEach(() => {
		useUiStore.setState({
			isOutcomeAttemptPanelOpen: false,
			outcomeAttemptPanelAttemptId: null,
			outcomeRunViewMode: "board",
			isCommandPaletteOpen: false,
			isKeyboardShortcutsOpen: false,
			isSidebarOpen: true,
		});
	});

	it("renders the stage body it wraps", () => {
		renderShell();
		expect(screen.getByTestId("stage-body")).toBeInTheDocument();
	});

	it("renders the List/Board control, wired to the shared store slice", async () => {
		const user = userEvent.setup();
		renderShell();
		expect(useUiStore.getState().outcomeRunViewMode).toBe("board");
		await user.click(screen.getByRole("tab", { name: /list/i }));
		expect(useUiStore.getState().outcomeRunViewMode).toBe("list");
	});

	it("disables the List/Board control on Act & Observe instead of leaving it a dead click — Mission's graph is List-only until Canvas ships", async () => {
		const user = userEvent.setup();
		renderShell({ stage: "act_observe" });
		expect(screen.getByTestId("sessions-view-switch")).toHaveAttribute("aria-disabled", "true");
		expect(screen.getByRole("tab", { name: /board/i })).toBeDisabled();
		expect(screen.getByRole("tab", { name: /list/i })).toHaveAttribute("aria-selected", "true");
		await user.click(screen.getByRole("tab", { name: /board/i }));
		// The shared store slice never moves — the click was refused, not
		// silently accepted and then ignored by the stage body.
		expect(useUiStore.getState().outcomeRunViewMode).toBe("board");
	});

	it("disables the terminal toggle when there is no current attempt", () => {
		renderShell({ outcomeId: "out-1" });
		expect(screen.getByTestId("work-shell-terminal-toggle")).toBeDisabled();
	});

	it("enables the terminal toggle and opens the real attempt panel once an attempt exists", async () => {
		attemptsQueryMock.mockReturnValue({
			attempts: [{ id: "att-1", number: 1 }],
			isLoading: false,
			refetch: vi.fn(),
		});
		const user = userEvent.setup();
		renderShell({ outcomeId: "out-1" });

		const toggle = screen.getByTestId("work-shell-terminal-toggle");
		expect(toggle).toBeEnabled();
		expect(screen.queryByTestId("mock-attempt-panel")).toBeNull();

		await user.click(toggle);
		expect(screen.getByTestId("mock-attempt-panel")).toHaveTextContent("att-1");

		await user.click(screen.getByRole("button", { name: "close" }));
		expect(screen.queryByTestId("mock-attempt-panel")).toBeNull();
	});

	it("uses a selected WorkUnit Attempt only for an explicit Engage target; the global toggle opens newest", async () => {
		attemptsQueryMock.mockReturnValue({
			attempts: [{ id: "att-old", number: 1 }, { id: "att-new", number: 2 }],
			isLoading: false,
			refetch: vi.fn(),
		});
		useUiStore.setState({ outcomeAttemptPanelAttemptId: "att-old" });
		const user = userEvent.setup();
		renderShell({ outcomeId: "out-1" });
		expect(screen.queryByTestId("mock-attempt-panel")).toBeNull();
		await user.click(screen.getByTestId("work-shell-terminal-toggle"));
		expect(screen.getByTestId("mock-attempt-panel")).toHaveTextContent("att-new");
	});

	it("keeps an explicitly engaged historical Attempt when a newer Attempt arrives", () => {
		attemptsQueryMock.mockReturnValue({
			attempts: [{ id: "att-old", number: 1 }, { id: "att-new", number: 2 }],
			isLoading: false,
			refetch: vi.fn(),
		});
		renderShell({ outcomeId: "out-1" });
		act(() => useUiStore.getState().openOutcomeAttemptPanel("att-old"));

		expect(screen.getByTestId("mock-attempt-panel")).toHaveTextContent("att-old");
	});

	it("does not fall back to a different Attempt when an explicit target is temporarily absent", () => {
		attemptsQueryMock.mockReturnValue({
			attempts: [{ id: "att-new", number: 2 }],
			isLoading: false,
			refetch: vi.fn(),
		});
		renderShell({ outcomeId: "out-1" });
		act(() => useUiStore.getState().openOutcomeAttemptPanel("att-old"));

		expect(screen.queryByTestId("mock-attempt-panel")).not.toBeInTheDocument();
		expect(screen.getByTestId("work-shell-terminal-toggle")).toBeDisabled();
	});

	it("never lets a dead button stand in for the Outcomes destination", async () => {
		const user = userEvent.setup();
		renderShell();
		await user.click(screen.getByTestId("work-shell-outcomes"));
		expect(navigateMock).toHaveBeenCalledWith({ to: "/work", search: { view: "outcomes" } });
	});

	it("opens the command palette from the search trigger", async () => {
		const user = userEvent.setup();
		renderShell();
		expect(useUiStore.getState().isCommandPaletteOpen).toBe(false);
		await user.click(screen.getByRole("button", { name: /search/i }));
		expect(useUiStore.getState().isCommandPaletteOpen).toBe(true);
	});

	it("leaves the relationship graph disabled — a bonus, not a promised destination", () => {
		renderShell();
		expect(screen.getByTestId("work-shell-graph")).toBeDisabled();
	});

	it("groups the sidebar toggle, back/forward, search, and notifications in one left cluster", () => {
		renderShell();
		expect(screen.getByRole("button", { name: /collapse sidebar|expand sidebar/i })).toBeInTheDocument();
		expect(screen.getByRole("button", { name: /go back/i })).toBeInTheDocument();
		expect(screen.getByRole("button", { name: /go forward/i })).toBeInTheDocument();
		expect(screen.getByTestId("mock-notification-center")).toBeInTheDocument();
	});

	it("toggles the sidebar from its own inline control", async () => {
		const user = userEvent.setup();
		renderShell();
		expect(useUiStore.getState().isSidebarOpen).toBe(true);
		await user.click(screen.getByRole("button", { name: /collapse sidebar|expand sidebar/i }));
		expect(useUiStore.getState().isSidebarOpen).toBe(false);
	});

	it("disables back/forward when there is nowhere to go, and calls real router history otherwise", async () => {
		canGoBackRef.current = true;
		const user = userEvent.setup();
		renderShell();
		const back = screen.getByRole("button", { name: /go back/i });
		const forward = screen.getByRole("button", { name: /go forward/i });
		expect(back).toBeEnabled();
		expect(forward).toBeDisabled();
		await user.click(back);
		expect(historyBackMock).toHaveBeenCalledTimes(1);
	});

	it("opens the shared keyboard-shortcuts dialog from the bottom-left help button", async () => {
		const user = userEvent.setup();
		renderShell();
		expect(useUiStore.getState().isKeyboardShortcutsOpen).toBe(false);
		await user.click(screen.getByTestId("work-shell-help"));
		expect(useUiStore.getState().isKeyboardShortcutsOpen).toBe(true);
	});
});
