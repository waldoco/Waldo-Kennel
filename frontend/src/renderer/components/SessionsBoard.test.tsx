import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceSession, WorkspaceSummary } from "../types/workspace";
import { appI18n } from "../i18n";
import { useUiStore } from "../stores/ui-store";

// Instant motion updates so height tweens do not leave tests waiting on timers.
vi.mock("motion/react", async (importOriginal) => {
	const actual = await importOriginal<typeof import("motion/react")>();
	return {
		...actual,
		AnimatePresence: ({ children }: { children: React.ReactNode }) => children,
	};
});

const {
	navigateMock,
	notificationShowMock,
	getMock,
	postMock,
	workspaceQueryMock,
	usageQueryMock,
	projectOutcomesQueryMock,
	boardActionsInPanelMock,
} = vi.hoisted(() => ({
	navigateMock: vi.fn(),
	notificationShowMock: vi.fn(),
	getMock: vi.fn(),
	postMock: vi.fn(),
	workspaceQueryMock: vi.fn(),
	usageQueryMock: vi.fn(),
	projectOutcomesQueryMock: vi.fn(),
	boardActionsInPanelMock: vi.fn(() => false),
}));

vi.mock("@tanstack/react-router", () => ({
	useNavigate: () => navigateMock,
}));

vi.mock("../hooks/useWorkspaceQuery", () => ({
	workspaceQueryKey: ["workspaces"],
	useWorkspaceQuery: workspaceQueryMock,
}));

vi.mock("../hooks/useSessionUsageSummaries", () => ({
	useSessionUsageSummaries: usageQueryMock,
}));

vi.mock("../hooks/useOutcome", () => ({
	useProjectOutcomes: projectOutcomesQueryMock,
}));

vi.mock("../lib/api-client", () => ({
	apiClient: {
		GET: (...args: unknown[]) => getMock(...args),
		POST: (...args: unknown[]) => postMock(...args),
	},
	apiErrorMessage: (_error: unknown, fallback: string) => fallback,
}));

vi.mock("../lib/bridge", () => ({
	aoBridge: {
		clipboard: {
			writeText: vi.fn(),
		},
		notifications: {
			show: (...args: unknown[]) => notificationShowMock(...args),
		},
	},
}));

vi.mock("../lib/platform", async (importOriginal) => {
	const actual = await importOriginal<typeof import("../lib/platform")>();
	return {
		...actual,
		usesBoardActionsInPanel: () => boardActionsInPanelMock(),
		isLinuxPlatform: () => false,
	};
});

import { archiveToggleHeightClassName, archiveToggleOffsetClassName } from "@pin4sf/kennel-product-ui";
import { SessionsBoard } from "./SessionsBoard";
import { TooltipProvider } from "./ui/tooltip";

function renderBoard(projectId?: string) {
	const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
	renderBoardWithClient(queryClient, projectId);
	return queryClient;
}

function renderBoardWithClient(queryClient: QueryClient, projectId?: string) {
	return render(
		<QueryClientProvider client={queryClient}>
			<TooltipProvider>
				<SessionsBoard projectId={projectId} />
			</TooltipProvider>
		</QueryClientProvider>,
	);
}

/** Archive cards mount on the next frame via startTransition — wait for the list. */
async function expandArchive() {
	await userEvent.click(screen.getByRole("button", { name: /archive/i }));
	return screen.findByRole("list", { name: "Archived sessions" });
}

beforeEach(() => {
	navigateMock.mockReset();
	notificationShowMock.mockReset().mockResolvedValue(undefined);
	getMock.mockReset().mockResolvedValue({ data: { controller: "ready", messages: [] } });
	postMock.mockReset().mockResolvedValue({ data: {} });
	workspaceQueryMock.mockReset().mockReturnValue({ data: [], isError: false });
	usageQueryMock.mockReset().mockReturnValue({ data: new Map() });
	projectOutcomesQueryMock.mockReset().mockReturnValue({ outcomes: [], isLoading: false });
	window.localStorage.removeItem("kennel.board.archive.layout");
	window.localStorage.removeItem("kennel.sessions.viewMode");
	useUiStore.setState({ sessionsViewMode: "board" });
	boardActionsInPanelMock.mockReset().mockReturnValue(false);
});

describe("SessionsBoard", () => {
	// The board no longer shows its own Outcomes summary card — WorkShell (the
	// persistent Work chrome) plus the sidebar's project/outcome tree and its
	// Outcomes destination replaced it, so a second, disconnected navigation
	// path here would only duplicate (and could drift from) the real one.
	it("never shows its own Outcomes summary card, even with outcomes present", () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([])],
			isError: false,
			isSuccess: true,
		});
		projectOutcomesQueryMock.mockReturnValue({
			outcomes: [
				{
					id: "outcome-1",
					title: "Explain Waldo clearly",
					currentRevisionNumber: 1,
				},
				{
					id: "outcome-approved",
					title: "Ship the dashboard",
					currentRevisionNumber: 1,
					latestPlan: { contractRevisionNumber: 1, status: "approved" },
				},
			],
			isLoading: false,
		});

		renderBoard("p1");

		expect(screen.queryByText("Explain Waldo clearly")).toBeNull();
		expect(screen.queryByText("Ship the dashboard")).toBeNull();
		expect(screen.queryByText("Authorized · Execution not connected")).toBeNull();
		expect(screen.getByRole("tablist", { name: "Session view" })).toBeInTheDocument();
	});

	it("shows a live orchestrator as project activity and switches it between Board and List", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({
						id: "orch-needs-input",
						title: "Outcome: explain Waldo",
						kind: "orchestrator",
						status: "needs_input",
						activity: { state: "idle", lastActivityAt: "2026-08-24T10:00:00Z" },
					}),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		expect(screen.getByText("Outcome: explain Waldo")).toBeInTheDocument();
		expect(screen.getByRole("tablist", { name: "Session view" })).toBeInTheDocument();
		expect(screen.queryByText("Start by defining an outcome")).not.toBeInTheDocument();

		await userEvent.click(screen.getByRole("tab", { name: "List" }));
		expect(screen.getByRole("tab", { name: "List" })).toHaveAttribute("aria-selected", "true");
		expect(screen.getByText("Outcome: explain Waldo")).toBeInTheDocument();
	});

	it("previews Waldo's review and recommendation from a List row without navigating", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({
						id: "s-review",
						title: "Resolve reviewer feedback",
						status: "changes_requested",
						commitMessage: "polish the terminal review flow",
						changedFiles: [
							{ path: "frontend/src/renderer/components/TerminalPane.tsx", additions: 12, deletions: 3 },
							{ path: "frontend/src/renderer/styles.css", additions: 8, deletions: 1 },
						],
					}),
				]),
			],
			isError: false,
			isSuccess: true,
		});
		useUiStore.setState({ sessionsViewMode: "list" });

		renderBoard("p1");
		const row = screen.getByTestId("board-session-row");
		await userEvent.hover(row);

		const brief = await screen.findByRole("dialog", { name: "Waldo brief for Resolve reviewer feedback" });
		expect(brief).toHaveTextContent("Polish the terminal review flow. 2 files changed in this session.");
		expect(brief).toHaveTextContent("Review the blocker with the agent before any consequential next step.");
		expect(navigateMock).not.toHaveBeenCalled();

		await userEvent.click(screen.getByRole("button", { name: "Hold off" }));
		expect(screen.queryByRole("dialog", { name: "Waldo brief for Resolve reviewer feedback" })).not.toBeInTheDocument();

		await userEvent.click(screen.getByRole("button", { name: "Choose Resolve reviewer feedback" }));
		expect(await screen.findByRole("dialog", { name: "Waldo brief for Resolve reviewer feedback" })).toBeInTheDocument();
		expect(navigateMock).not.toHaveBeenCalled();
		await userEvent.click(screen.getByRole("button", { name: "Open session" }));
		expect(navigateMock).toHaveBeenCalledWith({
			to: "/projects/$projectId/sessions/$sessionId",
			params: { projectId: "p1", sessionId: "s-review" },
		});
		navigateMock.mockClear();

		act(() => screen.getByRole("button", { name: /^Resolve reviewer feedback$/ }).focus());
		expect(await screen.findByRole("dialog", { name: "Waldo brief for Resolve reviewer feedback" })).toBeInTheDocument();
		expect(navigateMock).not.toHaveBeenCalled();
	});

	it("summarizes real daemon SCM facts in the Waldo brief", async () => {
		getMock.mockImplementation(async (path: string) =>
			path === "/api/v1/sessions/{sessionId}/pr"
				? {
						data: {
							prs: [
								{
									additions: 20,
									author: "waldo",
									changedFiles: 2,
									ci: { autoInjectCI: true, failingChecks: [], state: "passing" },
									deletions: 4,
									headSha: "abc123",
									htmlUrl: "https://github.com/acme/kennel/pull/68",
									mergeability: { conflictFiles: [], reasons: [], state: "mergeable" },
									number: 68,
									provider: "github",
									repo: "kennel",
									review: { decision: "changes_requested", hasUnresolvedHumanComments: true, unresolvedBy: [] },
									sourceBranch: "work/review",
									state: "open",
									targetBranch: "beta",
									title: "polish the terminal review flow",
									updatedAt: "2026-08-25T12:00:00Z",
									url: "https://api.github.com/repos/acme/kennel/pulls/68",
								},
							],
						},
					}
				: { data: { controller: "ready", messages: [] } },
		);
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([boardSession({ id: "s-scm", title: "Resolve reviewer feedback", status: "changes_requested" })])],
			isError: false,
			isSuccess: true,
		});
		useUiStore.setState({ sessionsViewMode: "list" });

		renderBoard("p1");
		await userEvent.click(screen.getByRole("button", { name: "Choose Resolve reviewer feedback" }));

		await waitFor(() => {
			expect(screen.getByRole("dialog", { name: "Waldo brief for Resolve reviewer feedback" })).toHaveTextContent(
				"Polish the terminal review flow. 2 files changed in this pull request.",
			);
		});
	});

	it("discloses when the Waldo brief has status guidance but no attributed work summary", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([boardSession({ id: "s-status", title: "Wait for direction", status: "needs_input" })])],
			isError: false,
			isSuccess: true,
		});
		useUiStore.setState({ sessionsViewMode: "list" });

		renderBoard("p1");
		await userEvent.click(screen.getByRole("button", { name: "Choose Wait for direction" }));

		const brief = await screen.findByRole("dialog", { name: "Waldo brief for Wait for direction" });
		expect(brief).toHaveTextContent("Current status");
		expect(brief).toHaveTextContent("No attributed work summary is available yet.");
		expect(brief).toHaveTextContent("This session needs your judgment before it can move safely.");
	});

	it("never renders outcome questions from transcript markers over the active board", async () => {
		// The marker intake is retired: Understand owns Outcome questions over
		// the daemon contract, so marker text in an orchestrator transcript must
		// not spawn an overlay here — the lanes stay exactly as they are.
		const questionSet = {
			questions: [
				{
					id: "scope",
					prompt: "Wants to create DES-4 now in team design, project Island Design Revamp?",
					description: "Waldo's insight on what is happening inside the session.",
					options: [
						{ id: "yes", label: "Yes, Create it", description: "Create it now", recommended: true },
						{ id: "wait", label: "Hold off", description: "Wait for now" },
					],
				},
			],
		};
		getMock.mockResolvedValue({
			data: {
				controller: "ready",
				messages: [
					{
						role: "user",
						text: "KENNEL OUTCOME INTAKE\n\nThe user wants this outcome:\nShip DES-4\n\nDo not spawn workers or begin implementation yet.",
					},
					{
						role: "assistant",
						text: `KENNEL_OUTCOME_QUESTIONS_JSON: ${JSON.stringify(questionSet)}`,
					},
				],
			},
		});
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({
						id: "orch-questions",
						title: "Project orchestrator",
						status: "idle",
						kind: "orchestrator",
						mode: "chat",
					}),
					boardSession({ id: "s-running", title: "Work already in progress", status: "working" }),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		await screen.findByText("Work already in progress");
		expect(screen.queryByRole("dialog", { name: /Wants to create DES-4/ })).not.toBeInTheDocument();
		expect(screen.getByRole("region", { name: "Running sessions" })).toHaveTextContent(
			"Work already in progress",
		);
	});

	it("localizes dynamic card actions and pull request lifecycle labels", async () => {
		await appI18n.changeLanguage("zh-CN");
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({
						id: "s-localized",
						title: "localized worker",
						status: "pr_open",
						prs: [
							{
								url: "https://github.com/acme/repo/pull/42",
								number: 42,
								state: "open",
								ci: "passing",
								review: "approved",
								mergeability: "mergeable",
								reviewComments: false,
								updatedAt: "2026-01-01T00:00:00Z",
							},
						],
					}),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		try {
			renderBoard("p1");
			expect(screen.getByRole("button", { name: "终止 localized worker" })).toBeInTheDocument();
			expect(screen.getByLabelText("#42 已打开")).toHaveAttribute(
				"href",
				"https://github.com/acme/repo/pull/42",
			);
		} finally {
			await appI18n.changeLanguage("en");
		}
	});

	it("does not show an agent setup warning on the board", () => {
		renderBoard();

		expect(screen.queryByText(/reload agents/i)).not.toBeInTheDocument();
	});

	it("shows the Board identity and compact actions in the in-panel board chrome", () => {
		boardActionsInPanelMock.mockReturnValue(true);
		workspaceQueryMock.mockReturnValue({
			data: [
				{
					id: "p1",
					name: "solkit-ui",
					path: "/tmp/solkit-ui",
					sessions: [
						{
							id: "s1",
							workspaceId: "p1",
							workspaceName: "solkit-ui",
							title: "test",
							provider: "codex",
							branch: "ao/dev/solkit-ui-5/root",
							status: "running",
							activity: { state: "working", lastActivityAt: "2026-01-01T00:00:00Z" },
							updatedAt: "2026-01-01T00:00:00Z",
							prs: [],
						},
					],
				},
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		expect(screen.getByTestId("board-topbar-label").textContent).toContain("Board");
		expect(screen.queryByText("solkit-ui")).toBeNull();
		expect(screen.getByRole("button", { name: "Define outcome" }).closest(".center-panel-titlebar")).toHaveClass(
			"workspace-topbar-container",
		);
		expect(
			within(screen.getByRole("button", { name: "Define outcome" }))
				.getByText("Define outcome")
				.hasAttribute("data-compact-label"),
		).toBe(true);
		expect(screen.queryByRole("button", { name: /Orchestrator/ })).not.toBeInTheDocument();
	});

	it.each(["active", "idle"] as const)("hides the %s orchestrator control in the in-panel board toolbar", (state) => {
		boardActionsInPanelMock.mockReturnValue(true);
		workspaceQueryMock.mockReturnValue({
			data: [
				{
					id: "p1",
					name: "solkit-ui",
					path: "/tmp/solkit-ui",
					sessions: [
						{
							id: "orch-1",
							workspaceId: "p1",
							workspaceName: "solkit-ui",
							title: "orchestrator",
							provider: "codex",
							kind: "orchestrator",
							branch: "main",
							status: "working",
							activity: { state, lastActivityAt: "2026-01-01T00:00:00Z" },
							updatedAt: "2026-01-01T00:00:00Z",
							prs: [],
						},
					],
				},
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		expect(screen.queryByRole("button", { name: /Orchestrator/ })).not.toBeInTheDocument();
	});

	it("shows the Board crumb on the root board when actions live in the panel", () => {
		boardActionsInPanelMock.mockReturnValue(true);
		workspaceQueryMock.mockReturnValue({
			data: [
				{
					id: "p1",
					name: "solkit-ui",
					path: "/tmp/solkit-ui",
					sessions: [],
				},
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard();

		// The crumb and the view switch both read "Board" — assert the crumb itself.
		expect(screen.getByTestId("board-topbar-label")).toHaveTextContent("Board");
	});

	it("labels an idle session as Idle, not Working", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				{
					id: "p1",
					name: "radic",
					path: "/tmp/radic",
					sessions: [
						{
							id: "s1",
							workspaceId: "p1",
							workspaceName: "radic",
							title: "brand-font-pipeline",
							provider: "claude-code",
							branch: "ao/radic-5",
							status: "idle",
							activity: { state: "idle", lastActivityAt: "2026-01-01T00:00:00Z" },
							updatedAt: "2026-01-01T00:00:00Z",
							prs: [],
						},
					],
				},
			],
			isError: false,
		});

		renderBoard("p1");

		const idleCard = screen
			.getByText("brand-font-pipeline")
			.closest('[data-testid="board-session-card"]') as HTMLElement;
		expect(within(idleCard).getByText("Idle")).toBeInTheDocument();
		const terminateButton = within(idleCard).getByRole("button", { name: "Terminate brand-font-pipeline" });
		expect(terminateButton).toHaveClass("opacity-0", "group-hover:opacity-100", "group-focus-within:opacity-100");
		expect(terminateButton.querySelector("svg")).toHaveClass("lucide-trash-2");
		expect(within(idleCard).getByText("Idle").parentElement?.parentElement).toHaveClass("flex");
		expect(within(idleCard).getByText("brand-font-pipeline")).toHaveClass("font-medium", "line-clamp-3");
	});

	it("shows compact token usage on active and archived cards and hides empty totals", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({ id: "s-active", title: "active worker", status: "idle" }),
					boardSession({ id: "s-empty", title: "empty worker", status: "idle" }),
					terminatedSession(),
				]),
			],
			isError: false,
			isSuccess: true,
		});
		usageQueryMock.mockReturnValue({
			data: new Map([
				[
					"s-active",
					{
						sessionId: "s-active",
						totalTokens: 12_400,
						incomplete: false,
					},
				],
				[
					"s-empty",
					{
						sessionId: "s-empty",
						totalTokens: 0,
						incomplete: false,
					},
				],
				[
					"s-dead",
					{
						sessionId: "s-dead",
						totalTokens: 2_000,
						incomplete: true,
					},
				],
			]),
		});

		renderBoard("p1");

		const activeUsage = screen.getByText("12.4K tok");
		expect(activeUsage).toHaveAttribute("aria-label", "12,400 tokens");
		expect(screen.queryByText("0 tok")).not.toBeInTheDocument();
		expect(usageQueryMock).toHaveBeenCalledWith("p1");
		await userEvent.hover(activeUsage);
		expect((await screen.findAllByText("12,400 tokens")).length).toBeGreaterThan(0);

		const archive = await expandArchive();
		const archivedUsage = within(archive).getByText("2K tok");
		expect(archivedUsage).toHaveAttribute("aria-label", "2,000 tokens");
	});

	it("marks an actively working session card with the still live dot", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({
						id: "s-active",
						title: "active-card-task",
						status: "working",
						activity: { state: "active", lastActivityAt: "2026-01-01T00:00:00Z" },
					}),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");
		const card = screen.getByText("active-card-task").closest('[data-testid="board-session-card"]') as HTMLElement;
		const working = within(card).getByText("Working").parentElement as HTMLElement;
		const mark = working.querySelector('[data-state-mark="live"]');
		expect(mark).toHaveClass("bg-wd-accent");
		expect(mark).not.toHaveClass("animate-status-pulse");
	});

	it("keeps a spawning card labeled Working when raw activity has not become active", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({
						id: "s-spawning",
						title: "spawning-card-task",
						status: "working",
						activity: { state: "exited", lastActivityAt: "2026-01-01T00:00:00Z" },
					}),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");
		const card = screen.getByText("spawning-card-task").closest('[data-testid="board-session-card"]') as HTMLElement;
		expect(within(card).getByText("Working")).toBeInTheDocument();
		expect(within(card).queryByText("Exited")).not.toBeInTheDocument();
	});

	it("shows switch progress instead of the exited source on a card", () => {
		const worker = boardSession({
			id: "s-switching",
			title: "switching worker",
			status: "exited",
			activity: {
				state: "exited",
				lastActivityAt: "2026-01-01T00:00:00Z",
			},
		});
		worker.activeAgentSwitch = activeAgentSwitch(worker.id);
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([worker])],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		const card = screen.getByText("switching worker").closest('[data-testid="board-session-card"]') as HTMLElement;
		const status = within(card).getByText("Switching to Codex").parentElement as HTMLElement;
		expect(status).toHaveClass("text-status-working");
		// In-flight switch keeps its dot, still - nothing pulses.
		expect(status.querySelector("span")).toHaveClass("rounded-full");
		expect(status.querySelector("span")).not.toHaveClass("animate-status-pulse");
		expect(within(card).queryByText("Exited")).not.toBeInTheDocument();
	});

	it("uses distinct card badge tones for idle, no signal, and draft PR sessions", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				{
					id: "p1",
					name: "radic",
					path: "/tmp/radic",
					sessions: [
						{
							id: "s0",
							workspaceId: "p1",
							workspaceName: "radic",
							title: "idle-card-task",
							provider: "claude-code",
							branch: "ao/radic-5",
							status: "idle",
							activity: { state: "idle", lastActivityAt: "2026-01-01T00:00:00Z" },
							updatedAt: "2026-01-01T00:00:00Z",
							prs: [],
						},
						{
							id: "s1",
							workspaceId: "p1",
							workspaceName: "radic",
							title: "no-signal-card-task",
							provider: "claude-code",
							branch: "ao/radic-6",
							status: "no_signal",
							activity: { state: "idle", lastActivityAt: "2026-01-01T00:00:00Z" },
							updatedAt: "2026-01-01T00:00:00Z",
							prs: [],
						},
						{
							id: "s2",
							workspaceId: "p1",
							workspaceName: "radic",
							title: "draft-card-task",
							provider: "claude-code",
							branch: "ao/radic-7",
							status: "draft",
							activity: { state: "idle", lastActivityAt: "2026-01-01T00:00:00Z" },
							updatedAt: "2026-01-01T00:00:00Z",
							prs: [],
						},
					],
				},
			],
			isError: false,
		});

		renderBoard("p1");
		const idleCard = screen.getByText("idle-card-task").closest('[data-testid="board-session-card"]') as HTMLElement;
		const noSignalCard = screen.getByText("no-signal-card-task").closest('[data-testid="board-session-card"]') as HTMLElement;
		const draftCard = screen.getByText("draft-card-task").closest('[data-testid="board-session-card"]') as HTMLElement;

		expect(within(idleCard).getByText("Idle").parentElement).toHaveClass("text-status-idle");
		expect(within(noSignalCard).getByText("No signal").parentElement).toHaveClass("text-status-unknown");
		expect(within(draftCard).getByText("Draft PR").parentElement).toHaveClass("text-status-in-review");
	});

	it("places an exited live session in Needs choice with an Exited badge", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					{
						id: "s-exited",
						workspaceId: "p1",
						workspaceName: "radic",
						title: "agent-exited-task",
						provider: "codex",
						branch: "ao/exited",
						status: "exited",
						activity: { state: "exited", lastActivityAt: "2026-01-01T00:00:00Z" },
						updatedAt: "2026-01-01T00:00:00Z",
						prs: [],
					},
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		const needsYouColumn = screen.getByRole("region", { name: "Needs choice sessions" });
		// A flat plate on the shell tone: no glow gradient.
		expect(needsYouColumn).toHaveClass("rounded-lg", "bg-shell");
		expect(needsYouColumn).not.toHaveClass("column-shell");
		expect(needsYouColumn.firstElementChild).toHaveClass("items-center", "justify-between");
		expect(within(needsYouColumn).getByText("agent-exited-task")).toBeInTheDocument();
		expect(within(needsYouColumn).getByText("Exited").parentElement).toHaveClass("text-status-exited");
	});

	it("renders idle and active sessions together in the single Running lane", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({
						id: "s-active",
						title: "active-task",
						status: "working",
						activity: { state: "active", lastActivityAt: "2026-01-01T00:00:00Z" },
					}),
					boardSession({
						id: "s-idle-1",
						title: "idle-no-pr-task",
						status: "idle",
						activity: { state: "idle", lastActivityAt: "2026-01-01T00:00:00Z" },
					}),
					boardSession({
						id: "s-idle-2",
						title: "second-idle-task",
						status: "idle",
						activity: { state: "idle", lastActivityAt: "2026-01-01T00:00:00Z" },
					}),
					boardSession({
						id: "s-review",
						title: "idle-with-pr-task",
						status: "pr_open",
						activity: { state: "idle", lastActivityAt: "2026-01-01T00:00:00Z" },
						prs: [
							{
								number: 7,
								url: "https://github.com/acme/radic/pull/7",
								state: "open",
								ci: "unknown",
								review: "none",
								mergeability: "unknown",
								reviewComments: false,
								updatedAt: "2026-01-01T00:00:00Z",
							},
						],
					}),
				]),
			],
			isError: false,
		});

		renderBoard("p1");

		const workLane = screen.getByRole("region", { name: "Running sessions" });
		const reviewLane = screen.getByRole("region", { name: "Needs input sessions" });

		expect(within(workLane).getByLabelText("3 Running sessions")).toHaveTextContent("3");
		expect(workLane.querySelectorAll(".overflow-y-auto")).toHaveLength(1);
		expect(within(workLane).getByText("idle-no-pr-task")).toBeInTheDocument();
		expect(within(workLane).getByText("second-idle-task")).toBeInTheDocument();
		expect(within(workLane).getByText("active-task")).toBeInTheDocument();
		expect(within(reviewLane).getByText("idle-with-pr-task")).toBeInTheDocument();
		expect(within(workLane).queryByText("idle-with-pr-task")).not.toBeInTheDocument();

		const idleCard = screen.getByText("idle-no-pr-task").closest('[data-testid="board-session-card"]') as HTMLElement;
		const badge = within(idleCard).getByText("Idle").parentElement;
		expect(badge).toHaveClass("text-status-idle");
		expect(badge).not.toHaveClass("text-status-working");
	});

	it("uses one shared scrollbar for all Running sessions", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({
						id: "s-idle",
						title: "single-idle-task",
						status: "idle",
						activity: { state: "idle", lastActivityAt: "2026-01-01T00:00:00Z" },
					}),
					...Array.from({ length: 7 }, (_, index) =>
						boardSession({
							id: `s-working-${index + 1}`,
							title: `working-task-${index + 1}`,
							status: "working",
							activity: { state: "active", lastActivityAt: "2026-01-01T00:00:00Z" },
						}),
					),
				]),
			],
			isError: false,
		});

		renderBoard("p1");

		const workLane = screen.getByRole("region", { name: "Running sessions" });
		const laneScrollers = Array.from(workLane.querySelectorAll<HTMLElement>(".overflow-y-auto"));

		expect(laneScrollers).toHaveLength(1);
		expect(laneScrollers[0]).toHaveClass("board-scrollbar", "overflow-y-auto");
		expect(within(workLane).getByLabelText("8 Running sessions")).toHaveTextContent("8");
		expect(within(workLane).getByText("single-idle-task")).toBeInTheDocument();
		expect(within(workLane).getAllByTestId("board-session-card")).toHaveLength(8);
	});

	it("lets an idle-only session fill the Running lane", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({
						id: "s-idle",
						title: "idle-task",
						status: "idle",
						activity: { state: "idle", lastActivityAt: "2026-01-01T00:00:00Z" },
					}),
				]),
			],
			isError: false,
		});

		renderBoard("p1");

		const workLane = screen.getByRole("region", { name: "Running sessions" });
		expect(within(workLane).getByLabelText("1 Running session")).toHaveTextContent("1");
		expect(within(workLane).getByText("idle-task")).toBeInTheDocument();
	});

	it("lets active-only sessions fill the Running lane", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({
						id: "s-working-1",
						title: "first-working-task",
						status: "working",
						activity: { state: "active", lastActivityAt: "2026-01-01T00:00:00Z" },
					}),
					boardSession({
						id: "s-working-2",
						title: "second-working-task",
						status: "working",
						activity: { state: "active", lastActivityAt: "2026-01-01T00:00:00Z" },
					}),
				]),
			],
			isError: false,
		});

		renderBoard("p1");

		const workLane = screen.getByRole("region", { name: "Running sessions" });
		expect(within(workLane).getByLabelText("2 Running sessions")).toHaveTextContent("2");
		expect(workLane.querySelectorAll(".overflow-y-auto")).toHaveLength(1);
		expect(within(workLane).getByText("first-working-task")).toBeInTheDocument();
		expect(within(workLane).getByText("second-working-task")).toBeInTheDocument();
	});

	it("updates the single Running lane when navigating between project boards", () => {
		const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
		workspaceQueryMock.mockReturnValue({
			data: [
				{
					id: "p1",
					name: "radic",
					path: "/tmp/radic",
					sessions: [
						{
							id: "p1-active",
							workspaceId: "p1",
							workspaceName: "radic",
							title: "p1 active",
							provider: "claude-code",
							branch: "ao/radic-active",
							status: "working",
							activity: { state: "active", lastActivityAt: "2026-01-01T00:00:00Z" },
							updatedAt: "2026-01-01T00:00:00Z",
							prs: [],
						},
						{
							id: "p1-idle",
							workspaceId: "p1",
							workspaceName: "radic",
							title: "p1 idle",
							provider: "claude-code",
							branch: "ao/radic-idle",
							status: "idle",
							activity: { state: "idle", lastActivityAt: "2026-01-01T00:00:00Z" },
							updatedAt: "2026-01-01T00:00:00Z",
							prs: [],
						},
					],
				},
				{
					id: "p2",
					name: "other",
					path: "/tmp/other",
					sessions: [
						{
							id: "p2-active",
							workspaceId: "p2",
							workspaceName: "other",
							title: "p2 active",
							provider: "claude-code",
							branch: "ao/other-active",
							status: "working",
							activity: { state: "active", lastActivityAt: "2026-01-01T00:00:00Z" },
							updatedAt: "2026-01-01T00:00:00Z",
							prs: [],
						},
						{
							id: "p2-idle",
							workspaceId: "p2",
							workspaceName: "other",
							title: "p2 idle",
							provider: "claude-code",
							branch: "ao/other-idle",
							status: "idle",
							activity: { state: "idle", lastActivityAt: "2026-01-01T00:00:00Z" },
							updatedAt: "2026-01-01T00:00:00Z",
							prs: [],
						},
					],
				},
			],
			isError: false,
		});
		const view = renderBoardWithClient(queryClient, "p1");

		const p1Lane = screen.getByRole("region", { name: "Running sessions" });
		expect(p1Lane).toHaveTextContent("p1 idle");
		expect(p1Lane).toHaveTextContent("p1 active");

		view.rerender(
			<QueryClientProvider client={queryClient}>
				<TooltipProvider>
					<SessionsBoard projectId="p2" />
				</TooltipProvider>
			</QueryClientProvider>,
		);

		const p2Lane = screen.getByRole("region", { name: "Running sessions" });
		expect(screen.queryByText("p1 idle")).not.toBeInTheDocument();
		expect(p2Lane).toHaveTextContent("p2 idle");
		expect(p2Lane).toHaveTextContent("p2 active");
	});

	it("shows a static archive card with a persistent restore action", async () => {
		const archivedSession = terminatedSession();
		const mergedPr = archivedSession.prs[0];
		if (!mergedPr) throw new Error("Archived-session fixture requires a pull request");
		archivedSession.prs = [
			{
				...mergedPr,
				number: 41,
				state: "open",
				url: "https://github.com/example/radic/pull/41",
			},
			mergedPr,
		];
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([archivedSession])],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		const archiveButton = screen.getByRole("button", { name: /archive/i });
		expect(archiveButton).toHaveClass(archiveToggleHeightClassName, "w-full", "py-0");
		const archiveLabel = within(archiveButton).getByText("Archive");
		expect(archiveLabel).not.toHaveClass("font-mono", "uppercase");
		expect(archiveLabel).toHaveClass("text-2xs", "font-medium");
		// Expanded archive overlays the board instead of shrinking lanes (which would
		// force a persistent Needs You column scrollbar gutter).
		expect(archiveButton.parentElement).toHaveClass("absolute", "inset-x-0", "bottom-0", "bg-background");
		expect(screen.getByTestId("board")).toHaveClass("relative");
		expect(screen.getByTestId("board").querySelector(":scope > .min-h-0.flex-1")).toHaveClass(
			archiveToggleOffsetClassName,
		);
		const archive = await expandArchive();
		expect(archive).toHaveClass("scrollbar-none", "overflow-y-auto", "max-h-[28vh]");
		const terminatedCard = within(archive).getByText("dead worker").closest<HTMLElement>("[role='listitem']");
		expect(terminatedCard).not.toBeNull();
		expect(terminatedCard).toHaveAttribute("data-testid", "board-session-card");
		expect(terminatedCard).not.toHaveClass("min-h-28");
		expect(within(terminatedCard!).queryByRole("button", { name: "Open dead worker" })).not.toBeInTheDocument();
		expect(within(terminatedCard!).getByText("Terminated")).toBeInTheDocument();
		// Agent shown as its brand logo with an accessible name (not a text label).
		expect(within(terminatedCard!).getByRole("img", { name: "claude-code agent" })).toBeInTheDocument();
		expect(screen.getByText("ao/dead-worker")).toBeInTheDocument();
		expect(screen.getByText("github:INT-17")).toBeInTheDocument();
		const prStatus = screen.getByLabelText("#42 merged");
		expect(prStatus).toHaveTextContent("PR#42merged");
		expect(within(prStatus).getByText("merged")).toHaveClass("text-status-merged");
		const openPrStatus = screen.getByLabelText("#41 open");
		expect(openPrStatus.parentElement).toBe(prStatus.parentElement);
		expect(prStatus.parentElement).toHaveClass("flex-wrap");
		expect(within(terminatedCard!).getByRole("link", { name: "#42" })).toHaveAttribute(
			"href",
			"https://github.com/example/radic/pull/42",
		);
		expect(within(terminatedCard!).getByRole("button", { name: "Copy branch ao/dead-worker" })).toBeInTheDocument();
		// Provenance leads, review state trails: the card reads whose work this is
		// before it reads where the work stands.
		expect(
			screen.getByText("ao/dead-worker").compareDocumentPosition(prStatus) & Node.DOCUMENT_POSITION_FOLLOWING,
		).not.toBe(0);
		expect(screen.getByRole("button", { name: "Restore dead worker" })).toBeInTheDocument();

		expect(screen.queryByRole("group", { name: "Archive layout" })).not.toBeInTheDocument();
	});

	it("keeps archive cards mounted after collapse so reopen does not remount them", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([terminatedSession()])],
			isError: false,
			isSuccess: true,
		});
		renderBoard("p1");

		const archiveButton = screen.getByRole("button", { name: /archive/i });
		const archive = await expandArchive();
		const card = within(archive).getByText("dead worker");

		await userEvent.click(archiveButton);
		expect(archiveButton).toHaveAttribute("aria-expanded", "false");
		expect(archive).toBeInTheDocument();
		expect(archive).toHaveAttribute("aria-hidden", "true");
		expect(archive).toHaveAttribute("inert");
		expect(archive).toHaveClass("pointer-events-none");
		expect(screen.queryByRole("list", { name: "Archived sessions" })).not.toBeInTheDocument();

		await userEvent.click(archiveButton);
		expect(archiveButton).toHaveAttribute("aria-expanded", "true");
		const reopened = screen.getByRole("list", { name: "Archived sessions" });
		expect(reopened).toBe(archive);
		expect(within(reopened).getByText("dead worker")).toBe(card);
		expect(reopened).not.toHaveAttribute("inert");
		expect(reopened).not.toHaveClass("pointer-events-none");
	});

	it("renders archived sessions as a grid even when rows were previously saved", async () => {
		window.localStorage.setItem("kennel.board.archive.layout", "rows");
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([terminatedSession()])],
			isError: false,
			isSuccess: true,
		});
		renderBoard("p1");

		await expandArchive();
		expect(screen.queryByRole("group", { name: "Archive layout" })).not.toBeInTheDocument();
		const archive = screen.getByRole("list", { name: "Archived sessions" });
		expect(archive).toHaveClass("grid");
		const restore = screen.getByRole("button", { name: "Restore dead worker" });
		expect(restore.closest("[role='listitem']")).toContainElement(screen.getByText("Terminated"));
		expect(screen.queryByRole("button", { name: "Open dead worker" })).not.toBeInTheDocument();
	});

	it("restores a terminated session, refreshes workspace data, and opens the restored terminal", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([terminatedSession()])],
			isError: false,
			isSuccess: true,
		});
		const queryClient = renderBoard("p1");
		const invalidate = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue(undefined);

		await expandArchive();
		await userEvent.click(screen.getByRole("button", { name: "Restore dead worker" }));

		await waitFor(() =>
			expect(postMock).toHaveBeenCalledWith("/api/v1/sessions/{sessionId}/restore", {
				params: { path: { sessionId: "s-dead" } },
			}),
		);
		expect(invalidate).toHaveBeenCalledWith({ queryKey: ["workspaces"] });
		expect(navigateMock).toHaveBeenCalledWith({
			to: "/projects/$projectId/sessions/$sessionId",
			params: { projectId: "p1", sessionId: "s-dead" },
		});
	});

	it("shows a toast when restore falls back to a saved-prompt conversation", async () => {
		postMock.mockResolvedValueOnce({ data: { restoreMode: "saved_prompt" } });
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([terminatedSession()])],
			isError: false,
			isSuccess: true,
		});
		renderBoard("p1");

		await expandArchive();
		await userEvent.click(screen.getByRole("button", { name: "Restore dead worker" }));

		await waitFor(() =>
			expect(notificationShowMock).toHaveBeenCalledWith(
				expect.objectContaining({
					title: "Started from saved prompt",
					body: expect.stringContaining("started a new conversation from the saved prompt"),
				}),
			),
		);
	});

	it("does not show a fallback toast when restore uses native resume", async () => {
		postMock.mockResolvedValueOnce({ data: { restoreMode: "native" } });
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([terminatedSession()])],
			isError: false,
			isSuccess: true,
		});
		renderBoard("p1");

		await expandArchive();
		await userEvent.click(screen.getByRole("button", { name: "Restore dead worker" }));

		await waitFor(() => expect(postMock).toHaveBeenCalled());
		expect(notificationShowMock).not.toHaveBeenCalled();
	});

	it("keeps restore actions visible and disables siblings while one session is restoring", async () => {
		let finishRestore: ((value: { data: Record<string, never> }) => void) | undefined;
		postMock.mockReturnValueOnce(
			new Promise((resolve) => {
				finishRestore = resolve;
			}),
		);
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([terminatedSession(), terminatedSession({ id: "s-other", title: "other worker" })])],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		await expandArchive();
		await userEvent.click(screen.getByRole("button", { name: "Restore dead worker" }));

		const restoringButton = screen.getByRole("button", { name: "Restore dead worker" });
		const otherButton = screen.getByRole("button", { name: "Restore other worker" });
		expect(restoringButton.querySelector("svg")).toHaveClass("animate-spin");
		expect(otherButton).toBeDisabled();
		expect(otherButton).not.toHaveClass("opacity-0");

		await act(async () => {
			finishRestore?.({ data: {} });
		});
	});

	it("opens the restore-unavailable dialog when a session is not resumable", async () => {
		postMock.mockResolvedValueOnce({ error: { code: "SESSION_NOT_RESUMABLE" } });
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([terminatedSession()])],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		await expandArchive();
		await userEvent.click(screen.getByRole("button", { name: "Restore dead worker" }));

		expect(await screen.findByText("Session can no longer be restored")).toBeInTheDocument();
	});

	it("shows an archive row error when restore fails", async () => {
		postMock.mockResolvedValueOnce({ error: { code: "RESTORE_FAILED", message: "boom" } });
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([terminatedSession()])],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		await expandArchive();
		await userEvent.click(screen.getByRole("button", { name: "Restore dead worker" }));

		expect(await screen.findByText("Unable to restore session")).toBeInTheDocument();
		expect(navigateMock).not.toHaveBeenCalled();
	});

	it("does not navigate when the static archive card is clicked", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([terminatedSession()])],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		await expandArchive();
		await userEvent.click(screen.getByText("dead worker"));

		expect(postMock).not.toHaveBeenCalled();
		expect(navigateMock).not.toHaveBeenCalled();
	});

	it("ignores restore completion after navigating to another project board", async () => {
		let finishRestore: ((value: { data: Record<string, never> }) => void) | undefined;
		postMock.mockReturnValueOnce(
			new Promise((resolve) => {
				finishRestore = resolve;
			}),
		);
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([terminatedSession()]),
				{
					id: "p2",
					name: "other",
					path: "/tmp/other",
					sessions: [],
				},
			],
			isError: false,
			isSuccess: true,
		});
		const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
		const view = renderBoardWithClient(queryClient, "p1");

		await expandArchive();
		await userEvent.click(screen.getByRole("button", { name: "Restore dead worker" }));

		view.rerender(
			<QueryClientProvider client={queryClient}>
				<TooltipProvider>
					<SessionsBoard projectId="p2" />
				</TooltipProvider>
			</QueryClientProvider>,
		);
		await act(async () => {
			finishRestore?.({ data: {} });
		});

		expect(navigateMock).not.toHaveBeenCalled();
		expect(screen.queryByText("Session can no longer be restored")).not.toBeInTheDocument();
	});

	it("ignores restore-unavailable completion after navigating to another project board", async () => {
		let finishRestore: ((value: { error: { code: string } }) => void) | undefined;
		postMock.mockReturnValueOnce(
			new Promise((resolve) => {
				finishRestore = resolve;
			}),
		);
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([terminatedSession()]),
				{
					id: "p2",
					name: "other",
					path: "/tmp/other",
					sessions: [],
				},
			],
			isError: false,
			isSuccess: true,
		});
		const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
		const view = renderBoardWithClient(queryClient, "p1");

		await expandArchive();
		await userEvent.click(screen.getByRole("button", { name: "Restore dead worker" }));

		view.rerender(
			<QueryClientProvider client={queryClient}>
				<TooltipProvider>
					<SessionsBoard projectId="p2" />
				</TooltipProvider>
			</QueryClientProvider>,
		);
		await act(async () => {
			finishRestore?.({ error: { code: "SESSION_NOT_RESUMABLE" } });
		});

		expect(navigateMock).not.toHaveBeenCalled();
		expect(screen.queryByText("Session can no longer be restored")).not.toBeInTheDocument();
	});

	it("shows a merged-only card in Ready and opens it without showing restore", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([boardSession({ id: "s-merged", title: "merged worker", status: "merged" })])],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		const mergeLane = screen.getByRole("region", { name: "Ready sessions" });
		expect(within(mergeLane).getByLabelText("1 Ready session")).toHaveTextContent("1");
		expect(within(mergeLane).getByText("merged worker")).toBeInTheDocument();
		expect(screen.queryByRole("button", { name: /archive/i })).not.toBeInTheDocument();
		expect(screen.queryByRole("button", { name: "Restore merged worker" })).not.toBeInTheDocument();

		await userEvent.click(screen.getByText("merged worker"));

		expect(postMock).not.toHaveBeenCalled();
		expect(navigateMock).toHaveBeenCalledWith({
			to: "/projects/$projectId/sessions/$sessionId",
			params: { projectId: "p1", sessionId: "s-merged" },
		});
	});

	it("keeps ready and merged sessions together in the Ready lane", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({ id: "s-ready", title: "ready worker", status: "mergeable" }),
					boardSession({ id: "s-merged", title: "merged worker", status: "merged" }),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		const mergeLane = screen.getByRole("region", { name: "Ready sessions" });
		expect(within(mergeLane).getByLabelText("2 Ready sessions")).toHaveTextContent("2");
		expect(mergeLane.querySelectorAll(".overflow-y-auto")).toHaveLength(1);
		expect(within(mergeLane).getByText("ready worker")).toBeInTheDocument();
		expect(within(mergeLane).getByText("merged worker")).toBeInTheDocument();
		expect(screen.queryByRole("button", { name: /archive/i })).not.toBeInTheDocument();
	});

	it("uses the shared minimal scrollbar styling for every Kanban lane", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({ id: "s-idle", title: "idle worker", status: "idle" }),
					boardSession({ id: "s-working", title: "working worker", status: "working" }),
					boardSession({ id: "s-action", title: "action worker", status: "needs_input" }),
					boardSession({ id: "s-review", title: "review worker", status: "review_pending" }),
					boardSession({ id: "s-ready", title: "ready worker", status: "mergeable" }),
					boardSession({ id: "s-merged", title: "merged worker", status: "merged" }),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		const laneScrollers = screen
			.getAllByTestId("board-column")
			.flatMap((column) => Array.from(column.querySelectorAll<HTMLElement>(".overflow-y-auto")));
		expect(laneScrollers).toHaveLength(4);
		for (const scroller of laneScrollers) {
			expect(scroller).toHaveClass("board-scrollbar", "overflow-y-auto");
		}
	});

	it("archives a terminated merged runtime without duplicating it in the Ready lane", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({ id: "s-live-merged", title: "live merged worker", status: "merged" }),
					terminatedSession({ id: "s-archived-merged", title: "archived merged worker", status: "merged" }),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		const readyLane = screen.getByRole("region", { name: "Ready sessions" });
		expect(within(readyLane).getByText("live merged worker")).toBeInTheDocument();
		expect(within(readyLane).queryByText("archived merged worker")).not.toBeInTheDocument();

		await expandArchive();
		const archive = screen.getByRole("list", { name: "Archived sessions" });
		const archivedMergedCard = within(archive)
			.getByText("archived merged worker")
			.closest<HTMLElement>("[role='listitem']");
		expect(archivedMergedCard).not.toBeNull();
		expect(
			within(archivedMergedCard!).queryByRole("button", { name: "Open archived merged worker" }),
		).not.toBeInTheDocument();
		expect(
			within(archivedMergedCard!).queryByRole("button", { name: "Terminate archived merged worker" }),
		).not.toBeInTheDocument();
		expect(within(archivedMergedCard!).getByText("Merged").parentElement).toHaveClass("text-status-merged");
		expect(within(archive).getByRole("button", { name: "Restore archived merged worker" })).toBeInTheDocument();
	});

	it("asks for confirmation when terminating an ordinary live session from its card", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([boardSession({ id: "s-idle", title: "idle worker", status: "idle" })])],
			isError: false,
			isSuccess: true,
		});
		renderBoard("p1");

		await userEvent.click(screen.getByRole("button", { name: "Terminate idle worker" }));

		expect(navigateMock).not.toHaveBeenCalled();
		expect(screen.getByRole("dialog", { name: "Terminate idle worker?" })).toBeInTheDocument();
	});

	it("terminates a live merged session from its card without opening the session", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([boardSession({ id: "s-merged", title: "merged worker", status: "merged" })])],
			isError: false,
			isSuccess: true,
		});
		renderBoard("p1");

		const terminateButton = screen.getByRole("button", { name: "Terminate merged worker" });
		expect(terminateButton).toHaveClass("opacity-100");
		expect(terminateButton).not.toHaveClass("opacity-0");
		await userEvent.click(terminateButton);
		expect(navigateMock).not.toHaveBeenCalled();
		const dialog = screen.getByRole("dialog", { name: "Terminate merged worker?" });
		await userEvent.click(within(dialog).getByRole("button", { name: "Yes, terminate session" }));

		await waitFor(() =>
			expect(postMock).toHaveBeenCalledWith("/api/v1/sessions/{sessionId}/kill", {
				params: { path: { sessionId: "s-merged" } },
			}),
		);
		expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
		expect(navigateMock).not.toHaveBeenCalled();
	});

	it("keeps only the targeted card disabled while its termination is pending", async () => {
		let finishKill!: (value: { data: { ok: boolean; sessionId: string }; error: undefined }) => void;
		postMock.mockReturnValueOnce(
			new Promise((resolve) => {
				finishKill = resolve;
			}),
		);
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({ id: "s-one", title: "worker one", status: "working" }),
					boardSession({ id: "s-two", title: "worker two", status: "merged" }),
				]),
			],
			isError: false,
			isSuccess: true,
		});
		renderBoard("p1");

		await userEvent.click(screen.getByRole("button", { name: "Terminate worker one" }));
		await userEvent.click(
			within(screen.getByRole("dialog")).getByRole("button", { name: "Yes, terminate session" }),
		);

		expect(screen.getByRole("button", { name: "Killing worker one" })).toBeDisabled();
		expect(screen.getByRole("button", { name: "Killing worker one" })).toHaveClass("opacity-100");
		expect(screen.getByRole("button", { name: "Terminate worker two" })).toBeEnabled();
		expect(postMock).toHaveBeenCalledTimes(1);

		finishKill({ data: { ok: true, sessionId: "s-one" }, error: undefined });
		await waitFor(() => expect(screen.getByRole("button", { name: "Terminate worker one" })).toBeEnabled());
	});

	it("keeps the merged-card confirmation dismissed and surfaces termination failures", async () => {
		postMock.mockResolvedValueOnce({ error: { message: "runtime failed" }, response: { status: 500 } });
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([boardSession({ id: "s-merged", title: "merged worker", status: "merged" })])],
			isError: false,
			isSuccess: true,
		});
		renderBoard("p1");

		await userEvent.click(screen.getByRole("button", { name: "Terminate merged worker" }));
		await userEvent.click(
			within(screen.getByRole("dialog")).getByRole("button", { name: "Yes, terminate session" }),
		);

		await waitFor(() => expect(postMock).toHaveBeenCalledTimes(1));
		expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
		expect(await screen.findByRole("alert")).toHaveTextContent("Failed to terminate session (500)");
		expect(screen.getByRole("button", { name: "Terminate merged worker" })).toBeEnabled();
	});
});

function workspaceWithSessions(sessions: WorkspaceSession[]): WorkspaceSummary {
	return {
		id: "p1",
		name: "radic",
		path: "/tmp/radic",
		sessions,
	};
}

function boardSession(
	overrides: Pick<WorkspaceSession, "id" | "title" | "status"> & Partial<WorkspaceSession>,
): WorkspaceSession {
	return {
		workspaceId: "p1",
		workspaceName: "radic",
		provider: "claude-code",
		branch: `ao/${overrides.id}`,
		updatedAt: "2026-01-01T00:00:00Z",
		prs: [],
		...overrides,
	};
}

function activeAgentSwitch(
	sessionId: string,
	overrides: Partial<NonNullable<WorkspaceSession["activeAgentSwitch"]>> = {},
): NonNullable<WorkspaceSession["activeAgentSwitch"]> {
	return {
		agentHandoffStatus: "received",
		fromHarness: "claude-code",
		id: `switch-${sessionId}`,
		state: "starting_target",
		targetHarness: "codex",
		...overrides,
	};
}

function terminatedSession(overrides: Partial<WorkspaceSession> = {}): WorkspaceSession {
	return {
		id: "s-dead",
		workspaceId: "p1",
		workspaceName: "radic",
		title: "dead worker",
		issueId: "github:INT-17",
		provider: "claude-code",
		kind: "worker",
		branch: "ao/dead-worker",
		status: "terminated",
		isTerminated: true,
		updatedAt: "2026-01-01T00:00:00Z",
		prs: [
			{
				url: "https://github.com/example/radic/pull/42",
				number: 42,
				state: "merged",
				ci: "passing",
				review: "approved",
				mergeability: "mergeable",
				reviewComments: false,
				updatedAt: "2026-01-01T00:00:00Z",
			},
		],
		...overrides,
	};
}
