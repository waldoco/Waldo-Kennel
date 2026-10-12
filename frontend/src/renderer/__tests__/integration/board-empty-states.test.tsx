import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";

// Drives the real useWorkspaceQuery + SessionsBoard end to end for the two
// first-run states, mocking only the HTTP client, the router, and the native
// folder picker: an empty daemon shows the import chooser (no column shells), a
// fresh project shows the task invitation, and any session brings the columns back.
const { getMock, navigateMock, chooseDirectoryMock, spawnOrchestratorMock } = vi.hoisted(() => ({
	getMock: vi.fn(),
	navigateMock: vi.fn(),
	chooseDirectoryMock: vi.fn(),
	spawnOrchestratorMock: vi.fn(),
}));

vi.mock("../../lib/spawn-orchestrator", () => ({
	isChatPreflightError: (error: unknown) =>
		error instanceof Error && (error as Error & { code?: string }).code === "CHAT_DRIVER_UNAVAILABLE",
	isTmuxPrerequisiteError: () => false,
	spawnOrchestrator: spawnOrchestratorMock,
}));

vi.mock("../../lib/api-client", () => ({
	apiClient: { GET: getMock, POST: vi.fn() },
	apiErrorCode: () => undefined,
	apiErrorMessage: (e: unknown) => (e instanceof Error ? e.message : "error"),
	hasTrustedApiBaseUrl: () => true,
}));

vi.mock("../../lib/bridge", () => ({
	aoBridge: { app: { chooseDirectory: chooseDirectoryMock } },
}));

vi.mock("@tanstack/react-router", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@tanstack/react-router")>();
	return { ...actual, useNavigate: () => navigateMock };
});

import { SessionsBoard } from "../../components/SessionsBoard";
import { ShellProvider, type ShellContextValue } from "../../lib/shell-context";
import { useUiStore } from "../../stores/ui-store";

type Project = { id: string; name: string; path: string; orchestratorAgent?: string };
type Session = Record<string, unknown>;

function respondWith(
	projects: Project[],
	sessions: Session[],
	conversation?: { controller: string; messages: Array<{ role: string; text: string }> },
	outcomes: Session[] = [],
) {
	getMock.mockImplementation(async (url: string) => {
		if (url === "/api/v1/projects") return { data: { projects }, error: undefined };
		if (url === "/api/v1/projects/{id}/outcomes") return { data: { outcomes }, error: undefined };
		if (url === "/api/v1/sessions") return { data: { sessions }, error: undefined };
		if (url === "/api/v1/sessions/{sessionId}/conversation") {
			return { data: conversation ?? { controller: "ready", messages: [] }, error: undefined };
		}
		return { data: undefined, error: undefined };
	});
}

const project: Project = {
	id: "proj-1",
	name: "my-app",
	path: "/repo/my-app",
	orchestratorAgent: "claude-code",
};

const workerSession: Session = {
	id: "sess-1",
	projectId: "proj-1",
	displayName: "fix the bug",
	harness: "claude-code",
	kind: "worker",
	status: "working",
	isTerminated: false,
	updatedAt: "2026-07-04T10:00:00Z",
	prs: [],
};

const orchestratorSession: Session = {
	id: "proj-1-orchestrator",
	projectId: "proj-1",
	displayName: "orchestrator",
	harness: "claude-code",
	kind: "orchestrator",
	status: "working",
	isTerminated: false,
	updatedAt: "2026-07-04T10:00:00Z",
	prs: [],
};

const createProjectMock = vi.fn().mockResolvedValue(undefined);
const initializeProjectRepositoryMock = vi.fn().mockResolvedValue(undefined);

// Kept from the latest renderBoard call so tests can rerender with the same
// providers (e.g. simulating a projectId route-param change on a mounted board).
let lastQueryClient: QueryClient | null = null;
let lastShell: ShellContextValue | null = null;

function renderBoard(ui: ReactNode) {
	lastQueryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
	lastShell = {
		daemonStatus: { state: "ready" } as ShellContextValue["daemonStatus"],
		workspaceStartupState: "ready",
		createProject: createProjectMock,
		initializeProjectRepository: initializeProjectRepositoryMock,
	};
	return render(
		<QueryClientProvider client={lastQueryClient}>
			<ShellProvider value={lastShell}>{ui}</ShellProvider>
		</QueryClientProvider>,
	);
}

// The kanban columns render as <section> elements; the empty states render none.
const columnCount = () => document.querySelectorAll('[data-testid="board-column"]').length;

beforeEach(() => {
	vi.clearAllMocks();
	createProjectMock.mockResolvedValue(undefined);
	initializeProjectRepositoryMock.mockResolvedValue(undefined);
	useUiStore.setState({
		newTaskRequest: null,
		orchestratorReplacementErrors: {},
		orchestratorStartupErrors: {},
		restartingProjectIds: new Set(),
		settingsModal: null,
	});
});

describe("global board first launch", () => {
	it("shows the startup loader instead of import while the daemon is booting", async () => {
		respondWith([], []);
		lastQueryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
		lastShell = {
			daemonStatus: { state: "starting" } as ShellContextValue["daemonStatus"],
			workspaceStartupState: "loading",
			createProject: createProjectMock,
			initializeProjectRepository: initializeProjectRepositoryMock,
		};
		render(
			<QueryClientProvider client={lastQueryClient}>
				<ShellProvider value={lastShell}>
					<SessionsBoard />
				</ShellProvider>
			</QueryClientProvider>,
		);

		expect(await screen.findByTestId("daemon-startup-loader")).toHaveClass("ao-startup-screen");
		expect(screen.getByTestId("waldo-brand-mark")).toHaveAttribute("data-brand", "waldo");
		expect(screen.getByRole("status", { name: "Kennel is starting" })).toBeInTheDocument();
		expect(screen.getByText("Kennel")).toBeInTheDocument();
		expect(screen.getByText("Starting the Kennel daemon…")).toHaveAttribute("aria-hidden", "true");
		expect(screen.queryByText("Add a project")).not.toBeInTheDocument();
		expect(columnCount()).toBe(0);
	});

	it("shows the import chooser instead of empty columns when no projects exist", async () => {
		respondWith([], []);
		renderBoard(<SessionsBoard />);

		expect(await screen.findByText("Add a project")).toBeInTheDocument();
		expect(screen.getByText("Start something new or bring your existing work into Kennel.")).toBeInTheDocument();
		expect(screen.getByRole("button", { name: /^Start a new project/ })).toBeInTheDocument();
		expect(screen.getByRole("button", { name: /^Import existing work/ })).toBeInTheDocument();
		expect(columnCount()).toBe(0);
		// The welcome carries its own orientation — no dangling "Board" header.
		expect(screen.queryByText("Board")).not.toBeInTheDocument();
	});

	it("opens the native folder picker from the Project card", async () => {
		respondWith([], []);
		chooseDirectoryMock.mockResolvedValue(null);
		renderBoard(<SessionsBoard />);

		await userEvent.click(await screen.findByRole("button", { name: /^Import existing work/ }));
		expect(chooseDirectoryMock).toHaveBeenCalledTimes(1);
		expect(chooseDirectoryMock).toHaveBeenCalledWith("Choose a project repository");
	});

	it("opens the native folder picker from the Workspace card", async () => {
		respondWith([], []);
		chooseDirectoryMock.mockResolvedValue(null);
		renderBoard(<SessionsBoard />);

		await userEvent.click(await screen.findByText("Importing multiple repositories?"));
		await userEvent.click(screen.getByRole("button", { name: "Import workspace" }));
		expect(chooseDirectoryMock).toHaveBeenCalledTimes(1);
		expect(chooseDirectoryMock).toHaveBeenCalledWith("Choose a workspace folder");
	});

	it("shows a visible error when the folder picker fails", async () => {
		respondWith([], []);
		chooseDirectoryMock.mockRejectedValue(new Error("dialog unavailable"));
		renderBoard(<SessionsBoard />);

		await userEvent.click(await screen.findByRole("button", { name: /^Import existing work/ }));
		const messages = await screen.findAllByText("dialog unavailable");
		expect(messages.some((el) => !el.classList.contains("sr-only"))).toBe(true);
	});

	it("keeps the columns once a project exists", async () => {
		respondWith([project], [workerSession]);
		renderBoard(<SessionsBoard />);

		expect(await screen.findByText("fix the bug")).toBeInTheDocument();
		expect(screen.queryByText("Add a project")).not.toBeInTheDocument();
		expect(columnCount()).toBe(4);
	});

	it("keeps populated columns visible after the daemon reports a startup failure", async () => {
		respondWith([project], [workerSession]);
		lastQueryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
		lastShell = {
			daemonStatus: { state: "stopped", code: "exited" } as ShellContextValue["daemonStatus"],
			workspaceStartupState: "loading",
			createProject: createProjectMock,
			initializeProjectRepository: initializeProjectRepositoryMock,
		};
		render(
			<QueryClientProvider client={lastQueryClient}>
				<ShellProvider value={lastShell}>
					<SessionsBoard />
				</ShellProvider>
			</QueryClientProvider>,
		);

		expect(await screen.findByText("fix the bug")).toBeInTheDocument();
		expect(screen.queryByTestId("daemon-startup-loader")).not.toBeInTheDocument();
		expect(columnCount()).toBe(4);
	});
});

describe("project board with no sessions", () => {
	it("shows the task invitation instead of empty columns", async () => {
		respondWith([project], []);
		renderBoard(<SessionsBoard projectId="proj-1" />);

		expect(await screen.findByText("Start by defining an outcome")).toBeInTheDocument();
		expect(screen.queryByRole("button", { name: "Spawn Orchestrator" })).not.toBeInTheDocument();
		expect(screen.getAllByRole("button", { name: "Define outcome" }).length).toBeGreaterThan(0);
		expect(screen.queryByText("Add a project")).not.toBeInTheDocument();
		expect(columnCount()).toBe(0);
	});

	it("opens durable Outcome Understand from the empty project invitation", async () => {
		respondWith([project], []);
		renderBoard(<SessionsBoard projectId="proj-1" />);

		await userEvent.click(await screen.findByRole("button", { name: "Define outcome" }));

		expect(navigateMock).toHaveBeenCalledWith({
			to: "/work",
			search: { project: "proj-1" },
		});
		expect(useUiStore.getState().newTaskRequest).toBeNull();
	});

	it("does not treat a project as empty when a daemon-backed Outcome exists, even before a worker session does", async () => {
		// The board no longer shows its own Outcomes summary card (WorkShell,
		// the sidebar's project/outcome tree, and its Outcomes destination
		// replaced it) — this test's remaining job is the empty-state gate
		// itself: an Outcome existing must still keep the board out of its
		// "Start by defining an outcome" empty state.
		respondWith([project], [], undefined, [
			{
				id: "outcome-1",
				title: "Explain Waldo clearly",
				currentRevisionNumber: 1,
			},
		]);
		renderBoard(<SessionsBoard projectId="proj-1" />);

		await waitFor(() => expect(screen.getByRole("tablist", { name: "Session view" })).toBeInTheDocument());
		expect(screen.queryByText("Start by defining an outcome")).not.toBeInTheDocument();
		expect(screen.queryByText("Explain Waldo clearly")).not.toBeInTheDocument();
		expect(columnCount()).toBe(4);
	});

	it("shows the active orchestrator in the running board", async () => {
		// The daemon-backed session is the operational projection. It stays
		// visible while active instead of being replaced by an empty-state card.
		respondWith(
			[project],
			[{ ...orchestratorSession, harness: "codex", mode: "chat", activity: { state: "active", lastActivityAt: "2026-07-04T10:00:00Z" } }],
		);
		renderBoard(<SessionsBoard projectId="proj-1" />);

		await waitFor(() => expect(document.querySelector('[data-session-id="proj-1-orchestrator"]')).not.toBeNull());
		expect(screen.getByRole("tablist", { name: "Session view" })).toBeInTheDocument();
		expect(columnCount()).toBe(4);
	});

	it("never turns transcript markers into Outcome suggestions", async () => {
		// The marker flow is retired: Understand owns Outcome intake through the
		// daemon contract, so suggestion markers in a transcript are inert text.
		respondWith(
			[project],
			[{ ...orchestratorSession, harness: "codex", mode: "chat" }],
			{
				controller: "ready",
				messages: [{ role: "assistant", text: "- KENNEL_OUTCOME_SUGGESTION: Add resilient offline recovery" }],
			},
		);
		renderBoard(<SessionsBoard projectId="proj-1" />);

		await waitFor(() => expect(document.querySelector('[data-session-id="proj-1-orchestrator"]')).not.toBeNull());
		expect(screen.queryByRole("button", { name: "Add resilient offline recovery" })).not.toBeInTheDocument();
		expect(useUiStore.getState().newTaskRequest).toBeNull();
	});

	it("never renders clarifying questions from transcript markers", async () => {
		// Structured questions come from the Understand stage over the daemon
		// API now; a KENNEL_OUTCOME_QUESTIONS_JSON marker must not resurrect the
		// retired intake panel here.
		respondWith(
			[project],
			[{ ...orchestratorSession, harness: "codex", mode: "chat" }],
			{
				controller: "ready",
				messages: [
					{
						role: "assistant",
						text: 'KENNEL_OUTCOME_QUESTIONS_JSON: {"questions":[{"id":"scope","prompt":"Which offline scope should ship first?","options":[{"id":"read","label":"Read-only cache","description":"Lower risk","recommended":true},{"id":"full","label":"Full offline editing","description":"Broader capability"}]}]}',
					},
				],
			},
		);
		renderBoard(<SessionsBoard projectId="proj-1" />);

		await waitFor(() => expect(document.querySelector('[data-session-id="proj-1-orchestrator"]')).not.toBeNull());
		expect(screen.queryByText("Which offline scope should ship first?")).not.toBeInTheDocument();
		expect(screen.queryByRole("button", { name: /Read-only cache/ })).not.toBeInTheDocument();
	});

	it.skip("legacy orchestrator spawner surfaces daemon failures", async () => {
		respondWith([project], []);
		spawnOrchestratorMock.mockRejectedValue(new Error("branch is already checked out in another worktree"));
		renderBoard(<SessionsBoard projectId="proj-1" />);

		await screen.findByText("Start by defining an outcome");
		const [spawnButton] = screen.getAllByRole("button", { name: "Spawn Orchestrator" });
		await userEvent.click(spawnButton);

		expect(await screen.findByText(/branch is already checked out/)).toBeInTheDocument();
	});

	it.skip("legacy orchestrator spawner offers a Terminal UI fallback", async () => {
		respondWith([project], []);
		const preflightError = Object.assign(new Error("Claude Code is unavailable"), {
			code: "CHAT_DRIVER_UNAVAILABLE",
		});
		spawnOrchestratorMock.mockRejectedValueOnce(preflightError).mockResolvedValueOnce("proj-1-orchestrator");
		renderBoard(<SessionsBoard projectId="proj-1" />);

		await screen.findByText("Start by defining an outcome");
		const [spawnButton] = screen.getAllByRole("button", { name: "Spawn Orchestrator" });
		await userEvent.click(spawnButton);
		await userEvent.click(await screen.findByRole("button", { name: "Create as Terminal UI" }));

		expect(spawnOrchestratorMock).toHaveBeenNthCalledWith(1, "proj-1", "board", false, undefined);
		expect(spawnOrchestratorMock).toHaveBeenNthCalledWith(2, "proj-1", "board", false, "tui");
	});

	it.skip("legacy orchestrator spawner opens project settings when unconfigured", async () => {
		const unconfiguredProject = { ...project, orchestratorAgent: undefined };
		respondWith([unconfiguredProject], []);
		renderBoard(<SessionsBoard projectId="proj-1" />);

		await screen.findByText("Start by defining an outcome");
		const [spawnButton] = screen.getAllByRole("button", { name: "Spawn Orchestrator" });
		await userEvent.click(spawnButton);

		expect(useUiStore.getState().settingsModal).toEqual({ scope: "project", projectId: "proj-1" });
		expect(navigateMock).not.toHaveBeenCalled();
		expect(spawnOrchestratorMock).not.toHaveBeenCalled();
	});

	it("shows the project creation startup error after navigating to the project board", async () => {
		respondWith([project], []);
		useUiStore
			.getState()
			.setOrchestratorStartupError(
				"proj-1",
				"Project added, but orchestrator did not start: branch is already checked out in another worktree",
			);
		renderBoard(<SessionsBoard projectId="proj-1" />);

		expect(await screen.findByText(/Project added, but orchestrator did not start/)).toBeInTheDocument();
		expect(screen.getByText(/branch is already checked out/)).toBeInTheDocument();
	});

	it.skip("legacy orchestrator spawner clears project startup errors on retry", async () => {
		respondWith([project], []);
		useUiStore
			.getState()
			.setOrchestratorStartupError(
				"proj-1",
				"Project added, but orchestrator did not start: branch is already checked out in another worktree",
			);
		spawnOrchestratorMock.mockResolvedValue("proj-1-orchestrator");
		renderBoard(<SessionsBoard projectId="proj-1" />);

		await screen.findByText(/Project added, but orchestrator did not start/);
		const [spawnButton] = screen.getAllByRole("button", { name: "Spawn Orchestrator" });
		await userEvent.click(spawnButton);

		await waitFor(() =>
			expect(screen.queryByText(/Project added, but orchestrator did not start/)).not.toBeInTheDocument(),
		);
		expect(useUiStore.getState().orchestratorStartupErrors["proj-1"]).toBeUndefined();
	});

	it("clears a project creation startup error when switching projects", async () => {
		const otherProject: Project = { id: "proj-2", name: "other-app", path: "/repo/other-app" };
		respondWith([project, otherProject], []);
		useUiStore
			.getState()
			.setOrchestratorStartupError(
				"proj-1",
				"Project added, but orchestrator did not start: branch is already checked out in another worktree",
			);
		const { rerender } = renderBoard(<SessionsBoard projectId="proj-1" />);

		await screen.findByText(/Project added, but orchestrator did not start/);
		rerender(
			<QueryClientProvider client={lastQueryClient!}>
				<ShellProvider value={lastShell!}>
					<SessionsBoard projectId="proj-2" />
				</ShellProvider>
			</QueryClientProvider>,
		);

		await screen.findByText("Start by defining an outcome");
		await waitFor(() => expect(useUiStore.getState().orchestratorStartupErrors["proj-1"]).toBeUndefined());
		expect(screen.queryByText(/Project added, but orchestrator did not start/)).not.toBeInTheDocument();
	});

	it("clears a project creation startup error once an orchestrator exists", async () => {
		respondWith([project], [orchestratorSession]);
		useUiStore
			.getState()
			.setOrchestratorStartupError(
				"proj-1",
				"Project added, but orchestrator did not start: branch is already checked out in another worktree",
			);
		renderBoard(<SessionsBoard projectId="proj-1" />);

		await waitFor(() => expect(document.querySelector('[data-session-id="proj-1-orchestrator"]')).not.toBeNull());
		await waitFor(() => expect(useUiStore.getState().orchestratorStartupErrors["proj-1"]).toBeUndefined());
		expect(screen.queryByText(/Project added, but orchestrator did not start/)).not.toBeInTheDocument();
	});

	it.skip("legacy orchestrator spawner clears stale errors between projects", async () => {
		const otherProject: Project = { id: "proj-2", name: "other-app", path: "/repo/other-app" };
		respondWith([project, otherProject], []);
		spawnOrchestratorMock.mockRejectedValue(new Error("branch is already checked out in another worktree"));
		const { rerender } = renderBoard(<SessionsBoard projectId="proj-1" />);

		await screen.findByText("Start by defining an outcome");
		const [spawnButton] = screen.getAllByRole("button", { name: "Spawn Orchestrator" });
		await userEvent.click(spawnButton);
		await screen.findByText(/branch is already checked out/);

		rerender(
			<QueryClientProvider client={lastQueryClient!}>
				<ShellProvider value={lastShell!}>
					<SessionsBoard projectId="proj-2" />
				</ShellProvider>
			</QueryClientProvider>,
		);
		await screen.findByText("Start by defining an outcome");
		expect(screen.queryByText(/branch is already checked out/)).not.toBeInTheDocument();
	});

	it("keeps the columns once the project has a session", async () => {
		respondWith([project], [workerSession]);
		renderBoard(<SessionsBoard projectId="proj-1" />);

		expect(await screen.findByText("fix the bug")).toBeInTheDocument();
		expect(screen.queryByText("Start by defining an outcome")).not.toBeInTheDocument();
		expect(columnCount()).toBe(4);
	});
});
