import { memo, useEffect, useRef, useState, type MouseEvent } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import {
	SessionsArchiveView,
	SessionsBoardGridView,
	SessionsListView,
	SessionsViewSwitch,
	archiveToggleOffsetClassName,
} from "@pin4sf/kennel-product-ui";
import { AlertTriangle, LayoutDashboard, Plus, RotateCw } from "lucide-react";
import {
	type WorkspaceSession,
	hasConfiguredOrchestratorAgent,
	newestActiveOrchestrator,
	orchestratorHealth,
} from "../types/workspace";
import {
	boardAttentionZoneOrder,
	getAttentionZoneViewForZone,
	type AttentionZoneView,
} from "../lib/session-presentation";
import {
	useSessionUsageSummaries,
	type SessionUsageSummary,
} from "../hooks/useSessionUsageSummaries";
import { useProjectOutcomes } from "../hooks/useOutcome";
import { useRestoreSession } from "../hooks/useRestoreSession";
import { useTerminateSession } from "../hooks/useTerminateSession";
import { useWorkspaceQuery, workspaceQueryKey } from "../hooks/useWorkspaceQuery";
import { NotificationCenter } from "./NotificationCenter";
import { BoardWelcome, ProjectBoardEmpty } from "./BoardEmptyStates";
import { OrchestratorIcon } from "./icons";
import { OrchestratorActivityIndicator } from "./OrchestratorActivityIndicator";
import { TopbarButton, TopbarKillError, topbarProjectLabelClass } from "./TopbarButton";
import { isChatPreflightError, isTmuxPrerequisiteError, spawnOrchestrator } from "../lib/spawn-orchestrator";
import { aoBridge } from "../lib/bridge";
import { restartProjectOrchestrator } from "../lib/restart-orchestrator";
import { usesPreviewWorkspaceData } from "../lib/preview-mode";
import { isLinuxPlatform, isMacPlatform, usesBoardActionsInPanel } from "../lib/platform";
import { cn } from "../lib/utils";
import { useUiStore } from "../stores/ui-store";
import { RestoreUnavailableDialog } from "./RestoreUnavailableDialog";
import { DaemonStartupLoader } from "./DaemonStartupLoader";
import { useShellMaybe } from "../lib/shell-context";
import {
	ArchivedSessionCardAdapter,
	BoardSessionCardAdapter,
	BoardSessionRowAdapter,
	sessionsBoardLabels,
} from "./SessionsBoardAdapters";
import { Tooltip, TooltipContent, TooltipTrigger } from "./ui/tooltip";
import { NewShellTerminalButton } from "./NewShellTerminalButton";

type SessionsBoardProps = {
	/** When set, the board shows only this project's sessions. */
	projectId?: string;
};

type UsageBySession = ReadonlyMap<string, SessionUsageSummary>;
const emptyUsageBySession: UsageBySession = new Map();

// Live merged sessions remain in-flow. A terminated runtime is archived even
// when its SCM outcome remains `merged`.
function isArchivedSession(session: WorkspaceSession): boolean {
	return session.isTerminated === true || session.status === "terminated";
}

const isMac = isMacPlatform();
const dragStyle = isMac ? ({ WebkitAppRegion: "drag" } as React.CSSProperties) : undefined;
const noDragStyle = isMac ? ({ WebkitAppRegion: "no-drag" } as React.CSSProperties) : undefined;

export function SessionsBoard({ projectId }: SessionsBoardProps) {
	const { t } = useTranslation();
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const columns: AttentionZoneView[] = boardAttentionZoneOrder.map((zone) => getAttentionZoneViewForZone(zone, t));
	const workspaceQuery = useWorkspaceQuery();
	const shell = useShellMaybe();
	const usageBySession = useSessionUsageSummaries(projectId).data ?? emptyUsageBySession;
	const projectOutcomesQuery = useProjectOutcomes(projectId);
	const projectOutcomes = projectOutcomesQuery.outcomes;
	// Evaluated at render so platform mocks in tests can flip the in-panel chrome.
	const boardActionsInPanel = usesBoardActionsInPanel();
	/** Bell lives in the board action row when the shell topbar does not host it. */
	const boardOwnsNotificationCenter = isLinuxPlatform() || boardActionsInPanel;
	const all = workspaceQuery.data ?? [];
	const workspaces = projectId ? all.filter((workspace) => workspace.id === projectId) : all;
	const workspace = projectId ? workspaces[0] : undefined;
	// Board chrome stays route-oriented; project context remains in the sidebar.
	const boardLabel = t("shell.board");
	// The board is the project's operational projection, so every live Kennel
	// session belongs here. Orchestrators are not Outcome truth, but hiding them
	// made an active, waiting Outcome look empty and removed the Board/List switch.
	const sessions = workspaces.flatMap((workspace) => workspace.sessions);
	const orchestrator = projectId ? newestActiveOrchestrator(workspaces[0]?.sessions ?? []) : undefined;
	// Outcome shaping no longer lives on this board: the Understand stage owns it
	// through the daemon contract, so the board derives only what its own durable
	// facts say — an active orchestrator session — and never parses transcripts.
	const orchestratorBusy = orchestrator?.activity?.state === "active";
	const isExploringCodebase = Boolean(orchestrator && orchestratorBusy);
	const [isSpawning, setIsSpawning] = useState(false);
	const [spawnError, setSpawnError] = useState<string | null>(null);
	const [canCreateAsTui, setCanCreateAsTui] = useState(false);
	const [canInstallTmux, setCanInstallTmux] = useState(false);
	const [isInstallingTmux, setIsInstallingTmux] = useState(false);
	const [tmuxInstallMessage, setTmuxInstallMessage] = useState<string | null>(null);
	const restartingProjectIds = useUiStore((state) => state.restartingProjectIds);
	const orchestratorStartupError = useUiStore((state) =>
		projectId ? (state.orchestratorStartupErrors[projectId] ?? null) : null,
	);
	const setProjectRestarting = useUiStore((state) => state.setProjectRestarting);
	const setOrchestratorReplacementError = useUiStore((state) => state.setOrchestratorReplacementError);
	const setOrchestratorStartupError = useUiStore((state) => state.setOrchestratorStartupError);
	const sessionsViewMode = useUiStore((state) => state.sessionsViewMode);
	const setSessionsViewMode = useUiStore((state) => state.setSessionsViewMode);
	const isProjectRestarting = projectId ? restartingProjectIds.has(projectId) : false;
	const health = workspace ? orchestratorHealth(workspace, isProjectRestarting) : { state: "ok" as const };
	const visibleSpawnError = spawnError ?? orchestratorStartupError;

	// The board instance survives project-to-project navigation (same route,
	// new param), so a spawn failure must not follow the user to another board.
	useEffect(() => {
		setSpawnError(null);
		setCanCreateAsTui(false);
		setCanInstallTmux(false);
		setTmuxInstallMessage(null);
	}, [projectId]);
	const previousProjectIdRef = useRef(projectId);
	useEffect(() => {
		const previousProjectId = previousProjectIdRef.current;
		if (previousProjectId && previousProjectId !== projectId) {
			setOrchestratorStartupError(previousProjectId, null);
		}
		previousProjectIdRef.current = projectId;
	}, [projectId, setOrchestratorStartupError]);
	useEffect(() => {
		if (projectId && orchestrator && orchestratorStartupError) {
			setOrchestratorStartupError(projectId, null);
		}
	}, [orchestrator, orchestratorStartupError, projectId, setOrchestratorStartupError]);

	const archived = sessions
		.filter(isArchivedSession)
		.sort((left, right) => right.updatedAt.localeCompare(left.updatedAt));
	const activeSessions = sessions.filter((candidate) => !isArchivedSession(candidate));
	const boardLabels = sessionsBoardLabels(t);
	// First-run orientation replaces the empty column shells (only once the
	// query has resolved, so the welcome never flashes over real data): the
	// global board teaches the app before any project exists, and a fresh
	// project board invites the first task instead of showing four zeros.
	const isDaemonReady = usesPreviewWorkspaceData || (shell ? shell.daemonStatus.state === "ready" : true);
	const daemonHasFailed = Boolean(shell?.daemonStatus.code);
	const workspaceStartupState = shell?.workspaceStartupState ?? "ready";
	const isLoaded = isDaemonReady && workspaceStartupState === "ready" && workspaceQuery.isSuccess;
	const showStartup =
		shell !== null &&
		!daemonHasFailed &&
		(!isDaemonReady || workspaceStartupState === "loading" || (!workspaceQuery.isSuccess && !workspaceQuery.isError));
	const showWelcome = !projectId && isLoaded && all.length === 0;
	// Archived sessions remain available below the board, but they must not hide
	// the active Outcome intake/review surface when no work is currently running.
	const showProjectEmpty =
		projectId !== undefined &&
		isLoaded &&
		workspaces.length > 0 &&
		activeSessions.length === 0 &&
		projectOutcomes.length === 0 &&
		!projectOutcomesQuery.isLoading &&
		!projectOutcomesQuery.failure;
	const hasArchive = archived.length > 0;
	const terminateSession = useTerminateSession();
	const activeProjectIdRef = useRef(projectId);
	activeProjectIdRef.current = projectId;

	const openSession = (session: WorkspaceSession) =>
		void navigate({
			to: "/projects/$projectId/sessions/$sessionId",
			params: { projectId: session.workspaceId, sessionId: session.id },
		});
	const openOutcomeUnderstand = () => {
		if (!projectId) return;
		void navigate({ to: "/work", search: { project: projectId } });
	};
	const openOrchestrator = async (mode?: "tui") => {
		if (!projectId || isProjectRestarting) return;
		if (orchestrator) {
			void navigate({
				to: "/projects/$projectId/sessions/$sessionId",
				params: { projectId, sessionId: orchestrator.id },
			});
			return;
		}
		if (!hasConfiguredOrchestratorAgent(workspace)) {
			if (workspace) {
				useUiStore.getState().openProjectSettings(projectId);
			}
			return;
		}
		setSpawnError(null);
		setCanCreateAsTui(false);
		setCanInstallTmux(false);
		setTmuxInstallMessage(null);
		setOrchestratorStartupError(projectId, null);
		setIsSpawning(true);
		try {
			const sessionId = await spawnOrchestrator(projectId, "board", false, mode);
			await queryClient.invalidateQueries({ queryKey: workspaceQueryKey });
			setOrchestratorStartupError(projectId, null);
			void navigate({
				to: "/projects/$projectId/sessions/$sessionId",
				params: { projectId, sessionId },
			});
		} catch (error) {
			// Never fail silently: the daemon's message (e.g. a worktree/branch
			// conflict) is the only actionable signal the user gets.
			console.error("Failed to spawn orchestrator:", error);
			setSpawnError(error instanceof Error ? error.message : t("shell.couldNotSpawn"));
			setCanCreateAsTui(isChatPreflightError(error));
			setCanInstallTmux(isTmuxPrerequisiteError(error));
		} finally {
			setIsSpawning(false);
		}
	};

	const installTmuxAndRetry = async () => {
		setIsInstallingTmux(true);
		setTmuxInstallMessage("Installing tmux via Homebrew. This may take a few minutes.");
		try {
			const result = await aoBridge.app.installTmux();
			if (result.status === "installed") {
				await openOrchestrator();
				return;
			}
			if (result.status === "failed") {
				setSpawnError(result.message ?? t("shell.couldNotSpawn"));
				setTmuxInstallMessage(null);
			}
			if (result.status === "cancelled") setTmuxInstallMessage(null);
		} finally {
			setIsInstallingTmux(false);
		}
	};

	const restartOrchestrator = async () => {
		if (!projectId) return;
		await restartProjectOrchestrator({
			projectId,
			queryClient,
			navigate,
			setProjectRestarting,
			setOrchestratorReplacementError,
		});
	};

	const actions = projectId ? (
		<>
			{visibleSpawnError && !showProjectEmpty && (
				<TopbarKillError className="max-w-content-max truncate" title={visibleSpawnError}>
					{visibleSpawnError}
				</TopbarKillError>
			)}
			{visibleSpawnError && canCreateAsTui && !showProjectEmpty ? (
				<TopbarButton disabled={isSpawning || isProjectRestarting} onClick={() => void openOrchestrator("tui")}>
					{t("newTask.createAsTui")}
				</TopbarButton>
			) : null}
			{visibleSpawnError && canInstallTmux && isMacPlatform() && !showProjectEmpty ? (
				<TopbarButton disabled={isSpawning || isInstallingTmux || isProjectRestarting} onClick={() => void installTmuxAndRetry()}>
					{isInstallingTmux ? "Installing tmux…" : "Install tmux via Homebrew"}
				</TopbarButton>
			) : null}
			{tmuxInstallMessage && !showProjectEmpty ? (
				<span className="max-w-content-max truncate text-xs text-wd-secondary" role="status">
					{tmuxInstallMessage}
				</span>
			) : null}
			<Tooltip>
				<TooltipTrigger asChild>
					<span className="inline-flex">
						<TopbarButton
							aria-label={t("shell.newTask")}
							className="topbar-control--labeled outcome-primary-action"
							data-priority="primary"
							disabled={isProjectRestarting}
							onClick={openOutcomeUnderstand}
							variant="primary"
						>
							<Plus className="size-icon-md" aria-hidden="true" />
							<span data-compact-label>{t("shell.newTask")}</span>
						</TopbarButton>
					</span>
				</TooltipTrigger>
				<TooltipContent side="bottom">{t("shell.newTask")}</TooltipContent>
			</Tooltip>
			<NewShellTerminalButton />
			{orchestrator ? <Tooltip>
				<TooltipTrigger asChild>
					<span className="inline-flex">
						<TopbarButton
							aria-label={t("shell.openOrchestrator")}
							className="topbar-control--labeled"
							data-priority="secondary"
							disabled={isSpawning || isProjectRestarting}
							onClick={() => void openOrchestrator()}
							variant="accent"
						>
							<OrchestratorIcon className="size-icon-md" aria-hidden="true" />
							<span data-compact-label>{t("shell.orchestrator")}</span>
							{orchestrator ? <OrchestratorActivityIndicator session={orchestrator} /> : null}
						</TopbarButton>
					</span>
				</TooltipTrigger>
				<TooltipContent side="bottom">
					{isProjectRestarting
						? t("shell.restarting")
						: isSpawning
							? t("shell.spawning")
							: orchestrator
								? t("shell.openOrchestrator")
								: t("shell.spawnOrchestrator")}
				</TooltipContent>
			</Tooltip> : null}
			{boardOwnsNotificationCenter ? (
				<>
					<span aria-hidden="true" className="workspace-topbar__utility-separator" />
					<NotificationCenter />
				</>
			) : null}
		</>
	) : boardOwnsNotificationCenter ? (
		<NotificationCenter />
	) : undefined;

	return (
		<div className="relative flex h-full min-h-0 flex-col bg-background text-foreground" data-testid="board">
			{/* macOS: shell topbar is hidden on board routes, so the project/"Board"
			    crumb + New task / Orchestrator / bell live in this in-panel row.
			    Win/Linux keep the crumb and actions in the framed ShellTopbar.
			    Welcome skips the row — a dangling "Board" above the import
			    chooser was review feedback on #2432. */}
			{!showWelcome && !showStartup && boardActionsInPanel && (boardLabel || actions) ? (
				<div
					className="workspace-topbar-container center-panel-titlebar flex h-toolbar shrink-0 items-center gap-2 hairline-b border-wd-hairline pr-4"
					style={dragStyle}
				>
					{boardLabel ? (
						<span
							className={cn(topbarProjectLabelClass, "inline-flex items-center gap-1.5")}
							data-testid="board-topbar-label"
						>
							<LayoutDashboard aria-hidden="true" className="size-icon-md" />
							{boardLabel}
						</span>
					) : null}
					<div className="min-w-0 flex-1" />
					{actions ? (
						<div className="workspace-topbar-actions flex shrink-0 items-center" style={noDragStyle}>
							{actions}
						</div>
					) : null}
				</div>
			) : null}

			{/* Reserve only the collapsed archive bar. Expanded archive overlays the
			    board so lane height (and Needs You scrollbars) stay stable. */}
			<div className={cn("min-h-0 flex-1 overflow-hidden", hasArchive && archiveToggleOffsetClassName)}>
				{projectId && health.state !== "ok" ? (
					<div className="mx-3 my-3 flex items-center gap-3 rounded-md bg-caution-bg px-3 py-2 text-sm text-wd-primary">
						<AlertTriangle className="size-icon-base shrink-0 text-caution" aria-hidden="true" />
						<span className="min-w-0 flex-1">{health.message}</span>
						{health.state === "restart_needed" || health.state === "duplicates" ? (
							<TopbarButton disabled={isProjectRestarting} onClick={() => void restartOrchestrator()} variant="primary">
								<RotateCw className="size-3.5" aria-hidden="true" />
								{t("shell.restart")}
							</TopbarButton>
						) : null}
					</div>
				) : null}
				{showStartup ? (
					<DaemonStartupLoader />
				) : workspaceStartupState === "error" || workspaceQuery.isError ? (
					<p className="py-10 text-center text-sm text-wd-secondary">{t("shell.couldNotLoadSessions")}</p>
				) : showWelcome ? (
					<BoardWelcome />
				) : showProjectEmpty ? (
					<ProjectBoardEmpty
						isExploring={isExploringCodebase}
						isSpawning={isSpawning}
						isInstallingTmux={isInstallingTmux}
						isProjectRestarting={isProjectRestarting}
						onNewTask={openOutcomeUnderstand}
						onInstallTmux={canInstallTmux && isMacPlatform() ? () => void installTmuxAndRetry() : undefined}
						tmuxInstallMessage={tmuxInstallMessage}
						spawnError={visibleSpawnError}
					/>
				) : (
					<div className="flex h-full min-h-0 flex-col">
						{/* This board no longer shows its own Outcomes summary: WorkShell
						    (components/outcome/WorkShell.tsx) is the persistent Work
						    chrome now, with the sidebar's project/outcome tree and the
						    Outcomes destination reachable from its top bar — a second,
						    disconnected "Outcomes" card here duplicated that navigation
						    and, before WorkShell existed, was the only way back into an
						    authorized plan. `projectOutcomes`/`projectOutcomesQuery` are
						    still read below only to decide whether this project's board
						    is genuinely empty (no sessions AND no outcomes). */}
						{/* The view switch heads the lanes rather than the window: it
						    changes what is below it, so it belongs to that region. */}
						<div className="flex shrink-0 items-center gap-2 px-2.5 pb-2.5 pt-2.5">
							<SessionsViewSwitch
								labels={{
									ariaLabel: t("shell.viewSwitchAria"),
									board: t("shell.viewBoard"),
									list: t("shell.viewList"),
								}}
								onChange={setSessionsViewMode}
								value={sessionsViewMode}
							/>
						</div>
						<div className="min-h-0 flex-1">
							{sessionsViewMode === "list" ? (
								<SessionsListView
									columns={columns}
									key={`list-${projectId ?? "all"}`}
									labels={boardLabels}
									renderSessionRow={(session) => (
										<BoardSessionRowAdapter
											onOpen={() => openSession(session)}
											session={session}
											usage={usageBySession.get(session.id)}
										/>
									)}
									sessions={activeSessions}
								/>
							) : (
								<SessionsBoardGridView
									columns={columns}
									key={projectId ?? "all"}
									labels={boardLabels}
									renderSessionCard={(session) => (
										<BoardSessionCardAdapter
											onOpen={() => openSession(session)}
											onTerminate={() => terminateSession.mutate(session)}
											session={session}
											usage={usageBySession.get(session.id)}
										/>
									)}
									sessions={activeSessions}
								/>
							)}
						</div>
					</div>
				)}
			</div>

			{hasArchive ? (
				<BoardArchivePanel
					activeProjectIdRef={activeProjectIdRef}
					projectId={projectId}
					sessions={archived}
					usageBySession={usageBySession}
				/>
			) : null}
		</div>
	);
}

/**
 * Restore state lives here so expand/collapse in SessionsArchiveView does not
 * re-render the kanban columns. In-flight restores are invalidated on project
 * change or unmount so completion cannot navigate after the user left.
 */
const BoardArchivePanel = memo(function BoardArchivePanel({
	activeProjectIdRef,
	projectId,
	sessions,
	usageBySession,
}: {
	activeProjectIdRef: React.MutableRefObject<string | undefined>;
	projectId?: string;
	sessions: WorkspaceSession[];
	usageBySession: UsageBySession;
}) {
	const { t } = useTranslation();
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const restoreSessionById = useRestoreSession();
	const [restoringSessionId, setRestoringSessionId] = useState<string | undefined>();
	const [restoreErrors, setRestoreErrors] = useState<Record<string, string>>({});
	const [restoreUnavailableSession, setRestoreUnavailableSession] = useState<WorkspaceSession | undefined>();
	const restoreGenerationRef = useRef(0);

	useEffect(() => {
		setRestoringSessionId(undefined);
		setRestoreErrors({});
		setRestoreUnavailableSession(undefined);
		restoreGenerationRef.current += 1;
	}, [projectId]);

	useEffect(() => {
		const generation = restoreGenerationRef.current;
		return () => {
			// Invalidate in-flight restores if this panel unmounts (e.g. project with
			// no archive) so completion cannot navigate after the user left.
			if (restoreGenerationRef.current === generation) {
				restoreGenerationRef.current += 1;
			}
		};
	}, []);

	const restoreArchivedSession = async (event: MouseEvent<HTMLButtonElement>, session: WorkspaceSession) => {
		event.stopPropagation();
		if (restoringSessionId) return;
		const restoreProjectId = projectId;
		const generation = restoreGenerationRef.current;
		const isStillActiveProject = () =>
			generation === restoreGenerationRef.current &&
			(!restoreProjectId || activeProjectIdRef.current === restoreProjectId);
		setRestoringSessionId(session.id);
		setRestoreErrors((current) => {
			const next = { ...current };
			delete next[session.id];
			return next;
		});
		try {
			const result = await restoreSessionById(session.id);
			if (!isStillActiveProject()) return;
			if (result.status === "success") {
				void navigate({
					to: "/projects/$projectId/sessions/$sessionId",
					params: { projectId: session.workspaceId, sessionId: session.id },
				});
				return;
			}
			if (result.status === "not_resumable") {
				setRestoreUnavailableSession(session);
				return;
			}
			setRestoreErrors((current) => ({ ...current, [session.id]: result.message }));
		} finally {
			if (isStillActiveProject()) {
				setRestoringSessionId(undefined);
			}
		}
	};

	return (
		<>
			<SessionsArchiveView
				labels={{
					archive: t("shell.archive"),
					archiveAria: t("shell.archiveSessionsAria", { count: sessions.length }),
					archivedSessions: t("shell.archivedSessions"),
				}}
				renderSessionCard={(session) => (
					<ArchivedSessionCardAdapter
						isRestoreDisabled={restoringSessionId !== undefined}
						isRestoring={restoringSessionId === session.id}
						restoreAction={(event) => void restoreArchivedSession(event, session)}
						restoreError={restoreErrors[session.id]}
						session={session}
						usage={usageBySession.get(session.id)}
					/>
				)}
				resetKey={projectId}
				sessions={sessions}
			/>
			{restoreUnavailableSession ? (
				<RestoreUnavailableDialog
					open={true}
					session={restoreUnavailableSession}
					onOpenChange={(open) => {
						if (!open) setRestoreUnavailableSession(undefined);
					}}
					onRecreated={async () => {
						await queryClient.invalidateQueries({ queryKey: workspaceQueryKey });
					}}
				/>
			) : null}
		</>
	);
});
