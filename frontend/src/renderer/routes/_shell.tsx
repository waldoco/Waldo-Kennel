import { createProjectConfig } from "../lib/create-project-config";
import { createFileRoute, Outlet, useMatchRoute, useNavigate, useParams, useRouterState } from "@tanstack/react-router";
import { isCancelledError, useQueryClient } from "@tanstack/react-query";
import { type CSSProperties, useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { CommandPalette } from "../components/CommandPalette";
import { CenterPanelShell } from "../components/CenterPanelShell";
import { DaemonFailureBanner } from "../components/DaemonFailureBanner";
import { NotificationRuntime } from "../components/NotificationCenter";
import { OnboardingTour } from "../components/OnboardingTour";
import { TrayRuntime } from "../components/TrayRuntime";
import { GlobalNewTaskDialog } from "../components/GlobalNewTaskDialog";
import { WaldoLauncher } from "../components/waldo/WaldoLauncher";
import {
	WaldoRailProvider,
	WaldoShortcutRuntime,
	useWaldoRail,
} from "../components/waldo/WaldoRailContext";
import { WaldoShellRail } from "../components/waldo/WaldoShellRail";
import { SettingsDialog } from "../components/SettingsDialog";
import { KeyboardShortcutsDialog } from "../components/KeyboardShortcutsDialog";
import { KeyboardShortcutsSettingsDialog } from "../components/settings/KeyboardShortcutsSettingsDialog";
import { ShellTopbar } from "../components/ShellTopbar";
import { SessionTopbarProvider } from "../components/SessionTopbarPortal";
import { OrchestratorReplacementDialog } from "../components/OrchestratorReplacementDialog";
import { Sidebar } from "../components/Sidebar";
import { SidebarProvider } from "../components/ui/sidebar";
import { TitlebarNav } from "../components/TitlebarNav";
import { WindowTitlebar } from "../components/WindowTitlebar";
import { TerminalCacheProvider } from "../components/TerminalPane";
import { agentsQueryKey, agentsQueryOptions, refreshAgents } from "../hooks/useAgentsQuery";
import { agentModelsQueryOptions } from "../hooks/useAgentModelsQuery";
import { useDaemonStatus } from "../hooks/useDaemonStatus";
import { useOutcome } from "../hooks/useOutcome";
import { useOpenShellTerminal } from "../hooks/useShellTerminals";
import { useWindowFullScreen } from "../hooks/useWindowFullScreen";
import { useWorkspaceQuery, workspaceQueryKey, workspaceQueryOptions } from "../hooks/useWorkspaceQuery";
import { apiClient, apiErrorCode, apiErrorMessage, hasTrustedApiBaseUrl } from "../lib/api-client";
import { refreshDaemonStatus } from "../lib/daemon-status";
import { usesPreviewWorkspaceData, usesWaldoUiPreview, usesWorkLaunchMode } from "../lib/preview-mode";
import { addRendererExceptionStep, captureRendererEvent, captureRendererException } from "../lib/telemetry";
import { ShellProvider } from "../lib/shell-context";
import { restartProjectOrchestrator } from "../lib/restart-orchestrator";
import { captureOrchestratorReplacementFailure } from "../lib/orchestrator-replacement-telemetry";
import { applyDocumentTheme, applyDocumentThemeStyle } from "../lib/theme";
import { aoBridge } from "../lib/bridge";
import { handleModifierLinkClick } from "../lib/external-link-policy";
import { cn } from "../lib/utils";
import {
	isLinuxPlatform,
	isMacPlatform,
	isWindowsPlatform,
	usesFramedAppTopbar,
	hidesShellTopbar,
} from "../lib/platform";
import { useUiStore } from "../stores/ui-store";
import { matchesRendererShortcut } from "../stores/keybindings-store";
import { sessionIsActive, toProjectKind, type WorkspaceSummary } from "../types/workspace";
import type { components } from "../../api/schema";
import { useAgentInventoryTelemetry } from "../hooks/useAgentInventoryTelemetry";

export const Route = createFileRoute("/_shell")({
	loader: async ({ context }) => {
		await refreshDaemonStatus().catch(() => undefined);
		if (!usesPreviewWorkspaceData && !hasTrustedApiBaseUrl()) return;
		return context.queryClient.ensureQueryData(workspaceQueryOptions);
	},
	component: ShellRoute,
});

function ShellRoute() {
	return (
		<WaldoRailProvider>
			<ShellLayout />
		</WaldoRailProvider>
	);
}

function errorMessage(error: unknown) {
	return error instanceof Error ? error.message : "Could not load projects";
}

export { createProjectConfig } from "../lib/create-project-config";

const isMac = isMacPlatform();
const isWindows = isWindowsPlatform();
const isLinux = isLinuxPlatform();
const framedAppTopbar = usesFramedAppTopbar();
const shellTopbarHiddenByPlatform = hidesShellTopbar();

function ShellLayout() {
	const { t } = useTranslation();
	const waldo = useWaldoRail();
	useAgentInventoryTelemetry();
	const navigate = useNavigate();
	const matchRoute = useMatchRoute();
	const queryClient = useQueryClient();
	const workspaceQuery = useWorkspaceQuery();
	const workspaces = workspaceQuery.data ?? [];
	const daemonStatus = useDaemonStatus(queryClient);
	const [workspaceStartupState, setWorkspaceStartupState] = useState<"loading" | "ready" | "error">("loading");
	const workspaceStartupBaselineRef = useRef(0);
	const agentCatalogPortRef = useRef<number | undefined>(undefined);
	const { themePreference, resolvedTheme, themeStyle, isSidebarOpen, toggleSidebar } = useUiStore();
	const syncSystemTheme = useUiStore((state) => state.syncSystemTheme);
	const requestNewTask = useUiStore((state) => state.requestNewTask);
	const requestCreateProject = useUiStore((state) => state.requestCreateProject);
	const requestNewShellTerminal = useUiStore((state) => state.requestNewShellTerminal);
	const newShellTerminalNonce = useUiStore((state) => state.newShellTerminalNonce);
	const setActiveShellTerminal = useUiStore((state) => state.setActiveShellTerminal);
	const openShellTerminal = useOpenShellTerminal();
	const isFullScreen = useWindowFullScreen();
	const [trafficLightDragActive, setTrafficLightDragActive] = useState(isMac);
	const leftFullScreenRef = useRef(false);
	useEffect(() => {
		if (!isMac) return;
		if (isFullScreen) {
			leftFullScreenRef.current = true;
			setTrafficLightDragActive(false);
			return;
		}
		if (!leftFullScreenRef.current) {
			setTrafficLightDragActive(true);
			return;
		}
		const reducedMotion =
			typeof window !== "undefined" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
		if (reducedMotion) {
			setTrafficLightDragActive(true);
			return;
		}
		const timer = window.setTimeout(() => setTrafficLightDragActive(true), 200);
		return () => window.clearTimeout(timer);
	}, [isFullScreen]);
	const handledShellNonceRef = useRef(newShellTerminalNonce);
	const isKeyboardShortcutsOpen = useUiStore((state) => state.isKeyboardShortcutsOpen);
	const setIsKeyboardShortcutsOpen = useUiStore((state) => state.setKeyboardShortcutsOpen);
	const [isKeyboardShortcutsSettingsOpen, setIsKeyboardShortcutsSettingsOpen] = useState(false);
	const [isSidebarPeekOpen, setIsSidebarPeekOpen] = useState(false);
	const sidebarPeekCloseTimerRef = useRef<number | undefined>(undefined);
	const routeParams = useParams({ strict: false }) as { projectId?: string; sessionId?: string };
	const location = useRouterState({ select: (state) => state.location });
	const workSearch = location.search as { project?: unknown; outcome?: unknown };
	const workProjectId =
		location.pathname === "/work" && typeof workSearch.project === "string" ? workSearch.project : undefined;
	const workOutcomeId =
		location.pathname === "/work" && typeof workSearch.outcome === "string" ? workSearch.outcome : undefined;
	const setInspectorOpen = useUiStore((state) => state.setInspectorOpen);
	const suppressedInspectorRef = useRef<{ sessionId: string; wasOpen: boolean } | null>(null);
	useEffect(() => {
		const sessionId = routeParams.sessionId;
		const suppressed = suppressedInspectorRef.current;
		if (!waldo.isOpen || !sessionId) {
			if (suppressed?.wasOpen) setInspectorOpen(suppressed.sessionId, true);
			suppressedInspectorRef.current = null;
			return;
		}

		if (suppressed?.sessionId === sessionId) return;
		if (suppressed?.wasOpen) setInspectorOpen(suppressed.sessionId, true);
		const current = useUiStore.getState().inspectorSessions[sessionId];
		const wasOpen = current?.isOpen ?? true;
		suppressedInspectorRef.current = { sessionId, wasOpen };
		if (wasOpen) setInspectorOpen(sessionId, false);
	}, [routeParams.sessionId, setInspectorOpen, waldo.isOpen]);
	useEffect(() => {
		document.addEventListener("click", handleModifierLinkClick);
		return () => document.removeEventListener("click", handleModifierLinkClick);
	}, []);
	const scopedProjectId = routeParams.projectId
		? routeParams.projectId
		: routeParams.sessionId
			? workspaces.find((workspace) => workspace.sessions.some((session) => session.id === routeParams.sessionId))?.id
			: workProjectId;
	const scopedProjectName = scopedProjectId
		? workspaces.find((workspace) => workspace.id === scopedProjectId)?.name ?? scopedProjectId
		: undefined;
	const scopedOutcome = useOutcome(workOutcomeId).outcome;
	useEffect(() => {
		if (!scopedProjectId) return;
		const projectQueryKey = ["project", scopedProjectId];
		void queryClient
			.prefetchQuery({
				queryKey: projectQueryKey,
				queryFn: async () => {
					const { data, error: apiError } = await apiClient.GET("/api/v1/projects/{id}", {
						params: { path: { id: scopedProjectId } },
					});
					if (apiError) throw new Error(apiErrorMessage(apiError));
					if (data?.status !== "ok") throw new Error("Project config unavailable");
					return data.project as components["schemas"]["Project"];
				},
			})
			.then(() => {
				const project = queryClient.getQueryData<components["schemas"]["Project"]>(projectQueryKey);
				const defaultWorkerAgent = project?.config?.worker?.agent || project?.agent || "";
				if (defaultWorkerAgent) {
					void queryClient.prefetchQuery(agentModelsQueryOptions(defaultWorkerAgent, scopedProjectId));
				}
			});
	}, [queryClient, scopedProjectId]);
	const isRootBoardRoute = Boolean(matchRoute({ to: "/" }));
	const isProjectBoardRoute = Boolean(matchRoute({ to: "/projects/$projectId" }));
	const isProjectSessionRoute = Boolean(matchRoute({ to: "/projects/$projectId/sessions/$sessionId" }));
	const isOutcomeWorkRoute = Boolean(matchRoute({ to: "/work" }));
	const isWelcomeBoard =
		isRootBoardRoute &&
		workspaceStartupState === "ready" &&
		workspaceQuery.isSuccess &&
		workspaces.length === 0;
	const usesWorkProjectShell =
		((isRootBoardRoute || isProjectBoardRoute) && !isWelcomeBoard) ||
		isProjectSessionRoute ||
		isOutcomeWorkRoute;
	const isSettingsRoute =
		Boolean(matchRoute({ to: "/settings", fuzzy: true })) ||
		Boolean(matchRoute({ to: "/projects/$projectId/settings", fuzzy: true }));
	const isHomeRoute = Boolean(matchRoute({ to: "/home", fuzzy: true }));
	const waldoWorkContext = routeParams.sessionId
		? t("waldo.rail.workSessionContext")
		: t("waldo.rail.workContext");
	const selfFramedCenterPanel = isWelcomeBoard || isSettingsRoute || isHomeRoute;
	const hideShellTopbar = selfFramedCenterPanel || shellTopbarHiddenByPlatform;
	const setProjectRestarting = useUiStore((state) => state.setProjectRestarting);
	const orchestratorReplacementErrors = useUiStore((state) => state.orchestratorReplacementErrors);
	const setOrchestratorReplacementError = useUiStore((state) => state.setOrchestratorReplacementError);
	const setOrchestratorStartupError = useUiStore((state) => state.setOrchestratorStartupError);
	const replacementErrorProjectId = Object.keys(orchestratorReplacementErrors)[0] ?? null;
	const isStartupLoading =
		!usesPreviewWorkspaceData &&
		!daemonStatus.code &&
		(daemonStatus.state !== "ready" || workspaceStartupState === "loading");
	const cancelSidebarPeekClose = useCallback(() => {
		if (sidebarPeekCloseTimerRef.current === undefined) return;
		window.clearTimeout(sidebarPeekCloseTimerRef.current);
		sidebarPeekCloseTimerRef.current = undefined;
	}, []);

	const previewSidebar = useCallback(() => {
		if (isSidebarOpen) return;
		cancelSidebarPeekClose();
		setIsSidebarPeekOpen(true);
	}, [cancelSidebarPeekClose, isSidebarOpen]);

	const scheduleSidebarPeekClose = useCallback(() => {
		if (isSidebarOpen) return;
		cancelSidebarPeekClose();
		sidebarPeekCloseTimerRef.current = window.setTimeout(() => {
			setIsSidebarPeekOpen(false);
			sidebarPeekCloseTimerRef.current = undefined;
		}, 140);
	}, [cancelSidebarPeekClose, isSidebarOpen]);

	const navigateSession = useCallback(
		(direction: -1 | 1) => {
			if (!scopedProjectId) return;
			const sessions = (workspaces.find((workspace) => workspace.id === scopedProjectId)?.sessions ?? []).filter(
				sessionIsActive,
			);
			if (sessions.length === 0) return;
			const currentIndex = sessions.findIndex((session) => session.id === routeParams.sessionId);
			const nextIndex =
				currentIndex === -1
					? direction === 1
						? 0
						: sessions.length - 1
					: (currentIndex + direction + sessions.length) % sessions.length;
			const session = sessions[nextIndex];
			if (!session || session.id === routeParams.sessionId) return;
			void navigate({
				to: "/projects/$projectId/sessions/$sessionId",
				params: { projectId: scopedProjectId, sessionId: session.id },
			});
		},
		[navigate, routeParams.sessionId, scopedProjectId, workspaces],
	);

	const updateWorkspaces = useCallback(
		(updater: (workspaces: WorkspaceSummary[]) => WorkspaceSummary[]) => {
			queryClient.setQueryData<WorkspaceSummary[]>(workspaceQueryKey, (current = []) => updater(current));
		},
		[queryClient],
	);

	const createProject = useCallback(
		async (input: {
			path: string;
			workerAgent: string;
			workerModel?: string;
			workerMode?: string;
			orchestratorAgent?: string;
			orchestratorModel?: string;
			orchestratorMode?: string;
			trackerIntake?: components["schemas"]["TrackerIntakeConfig"];
			asWorkspace?: boolean;
		}) => {
			void addRendererExceptionStep("Project add requested", {
				source: "project-add",
				operation: "project_add",
				surface: "project_board",
			});
			void captureRendererEvent("kennel.renderer.project_add_requested");
			const status = await refreshDaemonStatus();
			if (status.state !== "ready" || !status.port) {
				throw new Error(status.message || "Kennel daemon is not ready.");
			}
			const { data, error } = await apiClient.POST("/api/v1/projects", {
				body: {
					path: input.path,
					asWorkspace: input.asWorkspace || undefined,
					config: createProjectConfig(input),
				},
			});
			if (error) {
				const failure = new Error(apiErrorMessage(error)) as Error & { code?: string };
				failure.code = apiErrorCode(error);
				void captureRendererException(failure, {
					source: "project-add",
					operation: "project_add",
					surface: "project_board",
				});
				throw failure;
			}
			if (!data?.project) throw new Error("Project creation returned no project");

			const workspace: WorkspaceSummary = {
				id: data.project.id,
				name: data.project.name,
				kind: toProjectKind(data.project.kind),
				path: data.project.path,
				workspaceRepos: data.project.workspaceRepos,
				type: "main",
				orchestratorAgent: input.orchestratorAgent as WorkspaceSummary["orchestratorAgent"],
				sessions: [],
			};
			void captureRendererEvent("kennel.renderer.project_add_succeeded", { project_id: workspace.id });
			updateWorkspaces((current) => [workspace, ...current.filter((item) => item.id !== workspace.id)]);
			setOrchestratorStartupError(workspace.id, null);

			// Project existence is independent of provider setup. If no coordinator
			// was explicitly chosen, registration is complete and there is nothing
			// to spawn. In particular, never send an empty harness and never reuse
			// the worker as an implicit coordinator.
			if (!input.orchestratorAgent) {
				await queryClient.invalidateQueries({ queryKey: workspaceQueryKey });
				void navigate({ to: "/projects/$projectId", params: { projectId: workspace.id } });
				return;
			}

			try {
				void captureRendererEvent("kennel.renderer.orchestrator_spawn_requested", {
					project_id: workspace.id,
					source: "project_add",
				});
				const {
					data: spawnData,
					error: spawnError,
					response: spawnResponse,
				} = await apiClient.POST("/api/v1/sessions", {
					body: {
						projectId: workspace.id,
						kind: "orchestrator",
						harness: input.orchestratorAgent as components["schemas"]["SpawnSessionRequest"]["harness"],
					},
				});
				if (spawnError || !spawnData?.session?.id) {
					const message = spawnError
						? apiErrorMessage(spawnError, `Failed to spawn orchestrator (${spawnResponse.status})`)
						: `Failed to spawn orchestrator (${spawnResponse.status})`;
					throw new Error(message);
				}
				void captureRendererEvent("kennel.renderer.orchestrator_spawn_succeeded", {
					project_id: workspace.id,
					source: "project_add",
				});
				await queryClient.invalidateQueries({ queryKey: workspaceQueryKey });
				void navigate({ to: "/projects/$projectId", params: { projectId: workspace.id } });
			} catch (spawnError) {
				void captureRendererEvent("kennel.renderer.orchestrator_spawn_failed", {
					project_id: workspace.id,
					source: "project_add",
				});
				void navigate({ to: "/projects/$projectId", params: { projectId: workspace.id } });
				const message = spawnError instanceof Error ? spawnError.message : "Could not start orchestrator";
				const startupMessage = `Project added, but orchestrator did not start: ${message}`;
				setOrchestratorStartupError(workspace.id, startupMessage);
			}
		},
		[navigate, queryClient, setOrchestratorStartupError, updateWorkspaces],
	);

	const initializeProjectRepository = useCallback(async (path: string) => {
		const { error } = await apiClient.POST("/api/v1/projects/initialize", {
			body: { path },
		});
		if (error) {
			const failure = new Error(apiErrorMessage(error)) as Error & { code?: string };
			failure.code = apiErrorCode(error);
			throw failure;
		}
	}, []);

	const removeProject = useCallback(
		async (projectId: string) => {
			const isLastWorkspace = workspaces.length === 1 && workspaces[0]?.id === projectId;
			void addRendererExceptionStep("Project removal requested", {
				source: "project-remove",
				operation: "project_remove",
				surface: "project_board",
				project_id: projectId,
			});
			const { error } = await apiClient.DELETE("/api/v1/projects/{id}", {
				params: { path: { id: projectId } },
			});
			if (error) {
				const failure = new Error(apiErrorMessage(error)) as Error & { code?: string };
				failure.code = apiErrorCode(error);
				void captureRendererException(failure, {
					source: "project-remove",
					operation: "project_remove",
					surface: "project_board",
					project_id: projectId,
				});
				throw failure;
			}
			void captureRendererEvent("kennel.renderer.project_removed", { project_id: projectId });
			updateWorkspaces((current) => current.filter((item) => item.id !== projectId));
			if (isLastWorkspace) {
				void navigate({ to: "/" });
			}
		},
		[navigate, updateWorkspaces, workspaces],
	);

	const restartOrchestrator = useCallback(
		async (projectId: string, mode?: "chat" | "tui") => {
			await restartProjectOrchestrator({
				projectId,
				queryClient,
				navigate,
				setProjectRestarting,
				setOrchestratorReplacementError,
				mode,
				onError: (error) => {
					captureOrchestratorReplacementFailure(error, projectId);
				},
			});
		},
		[navigate, queryClient, setOrchestratorReplacementError, setProjectRestarting],
	);

	useEffect(() => {
		applyDocumentTheme(resolvedTheme);
	}, [resolvedTheme]);

	useEffect(() => {
		applyDocumentThemeStyle(themeStyle);
	}, [themeStyle]);

	useEffect(() => {
		let active = true;
		if (usesPreviewWorkspaceData) {
			workspaceStartupBaselineRef.current = 0;
			setWorkspaceStartupState("ready");
			return () => {
				active = false;
			};
		}
		if (daemonStatus.state !== "ready" || !daemonStatus.port) {
			workspaceStartupBaselineRef.current = 0;
			setWorkspaceStartupState("loading");
			return () => {
				active = false;
			};
		}

		workspaceStartupBaselineRef.current =
			queryClient.getQueryState(workspaceQueryKey)?.dataUpdatedAt ?? 0;
		setWorkspaceStartupState("loading");
		void queryClient
			.fetchQuery({ ...workspaceQueryOptions, staleTime: 0 })
			.then(() => {
				if (active) setWorkspaceStartupState("ready");
			})
			.catch((error) => {
				if (active && !isCancelledError(error)) setWorkspaceStartupState("error");
			});

		return () => {
			active = false;
		};
	}, [daemonStatus.port, daemonStatus.state, queryClient]);

	useEffect(() => {
		if (
			usesPreviewWorkspaceData ||
			daemonStatus.state !== "ready" ||
			workspaceStartupState === "ready" ||
			!workspaceQuery.isSuccess ||
			workspaceQuery.dataUpdatedAt <= workspaceStartupBaselineRef.current
		) {
			return;
		}
		setWorkspaceStartupState("ready");
	}, [
		daemonStatus.state,
		workspaceQuery.dataUpdatedAt,
		workspaceQuery.isSuccess,
		workspaceStartupState,
	]);

	useEffect(() => {
		void aoBridge.theme?.set(themePreference);
	}, [themePreference]);

	useEffect(() => {
		if (!isSidebarOpen) return;
		cancelSidebarPeekClose();
		setIsSidebarPeekOpen(false);
	}, [cancelSidebarPeekClose, isSidebarOpen]);

	useEffect(() => cancelSidebarPeekClose, [cancelSidebarPeekClose]);

	useEffect(() => {
		if (!isSidebarPeekOpen || isSidebarOpen) return;

		const handlePointerMove = (event: PointerEvent) => {
			const target = event.target instanceof Element ? event.target : null;
			const isInSidebarPortal = Boolean(target?.closest('[role="dialog"], [role="listbox"], [role="menu"]'));
			const isInTitlebarChrome = Boolean(
				target?.closest("[data-slot='titlebar-nav'], .window-titlebar"),
			);
			const sidebar = document.querySelector<HTMLElement>('[data-slot="sidebar-container"]');
			const bounds = sidebar?.getBoundingClientRect();
			const isInSidebar = Boolean(
				bounds &&
				event.clientX >= bounds.left &&
				event.clientX <= bounds.right &&
				event.clientY >= bounds.top &&
				event.clientY <= bounds.bottom,
			);

			if (isInSidebar || isInSidebarPortal || isInTitlebarChrome) {
				cancelSidebarPeekClose();
				return;
			}
			scheduleSidebarPeekClose();
		};

		window.addEventListener("pointermove", handlePointerMove);
		return () => window.removeEventListener("pointermove", handlePointerMove);
	}, [cancelSidebarPeekClose, isSidebarOpen, isSidebarPeekOpen, scheduleSidebarPeekClose]);

	useEffect(() => {
		if (daemonStatus.state !== "ready" || !daemonStatus.port) return;
		if (agentCatalogPortRef.current === daemonStatus.port) return;

		agentCatalogPortRef.current = daemonStatus.port;
		void queryClient.invalidateQueries({ queryKey: agentsQueryKey });
		void queryClient.fetchQuery({ ...agentsQueryOptions, queryFn: refreshAgents });
	}, [daemonStatus.port, daemonStatus.state, queryClient]);

	useEffect(() => {
		if (themePreference !== "system") return;

		const mediaQuery = window.matchMedia("(prefers-color-scheme: light)");
		const handleChange = () => syncSystemTheme();
		mediaQuery.addEventListener("change", handleChange);
		return () => mediaQuery.removeEventListener("change", handleChange);
	}, [themePreference, syncSystemTheme]);

	useEffect(() => {
		const handleKeyDown = (event: KeyboardEvent) => {
			if (matchesRendererShortcut("toggle-sidebar", event)) {
				event.preventDefault();
				toggleSidebar();
				return;
			}
			if (matchesRendererShortcut("open-project", event)) {
				const workspace = workspaces[Number(event.key) - 1];
				if (workspace) {
					event.preventDefault();
					void navigate({ to: "/projects/$projectId", params: { projectId: workspace.id } });
				}
			}
		};
		window.addEventListener("keydown", handleKeyDown);
		return () => window.removeEventListener("keydown", handleKeyDown);
	}, [navigate, toggleSidebar, workspaces]);

	useEffect(
		() =>
			aoBridge.app.onNewSessionShortcut(() => {
				if (scopedProjectId) {
					requestNewTask(scopedProjectId);
				} else {
					requestCreateProject();
				}
			}),
		[scopedProjectId, requestNewTask, requestCreateProject],
	);

	useEffect(() => aoBridge.app.onKeyboardShortcutsHelp(() => setIsKeyboardShortcutsOpen(true)), []);

	useEffect(() => aoBridge.app.onNewShellTerminalShortcut(() => requestNewShellTerminal()), [requestNewShellTerminal]);

	useEffect(() => {
		if (handledShellNonceRef.current === newShellTerminalNonce) return;
		handledShellNonceRef.current = newShellTerminalNonce;
		openShellTerminal.mutate(
			{ projectId: scopedProjectId, sessionId: routeParams.sessionId },
			{
				onSuccess: (shell) => {
					setActiveShellTerminal(shell.handleId);
					if (!routeParams.sessionId) {
						void navigate({ to: "/terminals" });
					}
				},
			},
		);
	}, [
		newShellTerminalNonce,
		openShellTerminal,
		scopedProjectId,
		routeParams.sessionId,
		navigate,
		setActiveShellTerminal,
	]);

	useEffect(
		() => aoBridge.app.onOpenSettingsShortcut(() => useUiStore.getState().openGlobalSettings()),
		[],
	);

	useEffect(() => {
		const disposePrevious = aoBridge.app.onPreviousSessionShortcut(() => navigateSession(-1));
		const disposeNext = aoBridge.app.onNextSessionShortcut(() => navigateSession(1));
		return () => {
			disposePrevious();
			disposeNext();
		};
	}, [navigateSession]);

	useEffect(
		() =>
			aoBridge.app.onFocusTerminalShortcut(() => {
				document
					.querySelector<HTMLElement>(
						"[data-terminal-activation-phase='visible'] .xterm-helper-textarea, " +
							"[data-testid='session-terminal-slot'] .xterm-helper-textarea",
					)
					?.focus();
			}),
		[],
	);

	return (
		<ShellProvider value={{ daemonStatus, workspaceStartupState, createProject, initializeProjectRepository }}>
			<SessionTopbarProvider>
				<NotificationRuntime />
				<WaldoShortcutRuntime />
				<OnboardingTour daemonReady={daemonStatus.state === "ready"} />
				<TrayRuntime />
				<GlobalNewTaskDialog />
				<SettingsDialog />
				<KeyboardShortcutsDialog
					open={isKeyboardShortcutsOpen}
					onOpenChange={setIsKeyboardShortcutsOpen}
					onCustomize={() => {
						setIsKeyboardShortcutsOpen(false);
						setIsKeyboardShortcutsSettingsOpen(true);
					}}
				/>
				<KeyboardShortcutsSettingsDialog
					open={isKeyboardShortcutsSettingsOpen}
					onOpenChange={setIsKeyboardShortcutsSettingsOpen}
				/>
				<TerminalCacheProvider daemonReady={daemonStatus.state === "ready"} theme={resolvedTheme}>
					<div
						className={cn(
							"flex h-screen min-h-0 flex-col bg-wd-canvas text-wd-primary",
							usesWorkProjectShell && "figma-board-shell",
							isWindows && "platform-windows",
							isLinux && "platform-linux",
							isFullScreen && "native-fullscreen",
						)}
					>
						<WindowTitlebar onSidebarPreviewEnter={previewSidebar} />
						{!usesWorkProjectShell && !framedAppTopbar && !hideShellTopbar && !routeParams.sessionId ? <ShellTopbar /> : null}
						<SidebarProvider
							className="min-h-0 flex-1 flex-col overflow-x-hidden"
							keyboardShortcut={false}
							onOpenChange={(open) => {
								cancelSidebarPeekClose();
								setIsSidebarPeekOpen(false);
								if (open !== isSidebarOpen) toggleSidebar();
							}}
							open={!isStartupLoading && (usesWorkProjectShell || isSidebarOpen || isSidebarPeekOpen)}
							style={
								{
									"--sidebar-width": usesWorkProjectShell
										? "271px"
										: "var(--ao-sidebar-w, var(--size-sidebar-default))",
									"--sidebar-width-icon": "var(--size-sidebar-icon)",
								} as CSSProperties
							}
						>
							<div className="relative flex min-h-0 w-full flex-1 overflow-x-hidden" data-testid="shell-content-row">
								<Sidebar
									figmaBoard={usesWorkProjectShell}
									hideEdgeBorder={isWelcomeBoard || usesWorkProjectShell}
									isOverlay={isSidebarPeekOpen && !isSidebarOpen}
									onPreviewLeave={scheduleSidebarPeekClose}
									underTopbar={!usesWorkProjectShell && (isMac || isWindows || isLinux)}
									topbarOffset={isWindows ? "titlebar" : hideShellTopbar ? "trafficLights" : "toolbar"}
									onCreateProject={createProject}
									onInitializeProject={initializeProjectRepository}
									onRemoveProject={removeProject}
									workspaceError={workspaceQuery.isError ? errorMessage(workspaceQuery.error) : undefined}
									workspaces={workspaces}
								/>
								<main
									className={cn(
										"relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden",
										!usesWorkProjectShell && !isSidebarOpen && "sidebar-hidden",
																				!isHomeRoute && !usesWorkLaunchMode && "waldo-launcher-reserved",
									)}
								>
									<div className="min-h-0 flex-1 overflow-x-hidden">
										{usesWorkProjectShell ? (
											<CenterPanelShell className="center-panel-shell--figma-board">
												{hideShellTopbar || isOutcomeWorkRoute ? null : <ShellTopbar />}
												<div className="flex min-h-0 flex-1 flex-col">
													<Outlet />
												</div>
											</CenterPanelShell>
										) : hideShellTopbar ? (
											selfFramedCenterPanel ? (
												<Outlet />
											) : (
												<CenterPanelShell className={routeParams.sessionId ? "center-panel-shell--session" : undefined}>
													<div className="flex min-h-0 flex-1 flex-col">
														<Outlet />
													</div>
												</CenterPanelShell>
											)
										) : framedAppTopbar ? (
											<CenterPanelShell className={routeParams.sessionId ? "center-panel-shell--session" : undefined}>
												{routeParams.sessionId ? null : <ShellTopbar />}
												<div className="flex min-h-0 flex-1 flex-col">
													<Outlet />
												</div>
											</CenterPanelShell>
										) : (
											<CenterPanelShell className={routeParams.sessionId ? "center-panel-shell--session" : undefined}>
												<div className="flex min-h-0 flex-1 flex-col">
													<Outlet />
												</div>
											</CenterPanelShell>
										)}
									</div>
									{!usesWorkLaunchMode ? <div className="pointer-events-none absolute right-2 top-1.5 z-titlebar">
										<WaldoLauncher className="pointer-events-auto" />
									</div> : null}
									{!isHomeRoute && !usesWorkLaunchMode ? (
										<WaldoShellRail
											contextLabel={waldoWorkContext}
											daemonReady={daemonStatus.state === "ready"}
											onOpenHome={() => {
												waldo.close();
												void navigate({ to: "/home" });
											}}
											onReturnToInspector={routeParams.sessionId
												? () => setInspectorOpen(routeParams.sessionId!, true)
												: undefined}
											previewEnabled={usesWaldoUiPreview}
											outcomeId={workOutcomeId}
											outcomeTitle={scopedOutcome?.title}
											projectId={scopedProjectId}
											projectName={scopedProjectName}
										/>
									) : null}
								</main>
							</div>
							<DaemonFailureBanner status={daemonStatus} />
							{hideShellTopbar && isMac ? (
								<div
									aria-hidden="true"
									className={cn(
										"fixed top-0 left-0 z-chrome w-(--sidebar-width) transition-[height] duration-200 ease-out motion-reduce:transition-none",
										isFullScreen ? "pointer-events-none h-0" : "h-traffic-light-clearance",
									)}
									style={trafficLightDragActive ? ({ WebkitAppRegion: "drag" } as CSSProperties) : undefined}
								/>
							) : null}
							{usesWorkProjectShell ? null : (
								<TitlebarNav
									hasSessionTopbar={Boolean(routeParams.sessionId)}
									historyLocked={isWelcomeBoard}
									isFullScreen={isFullScreen}
									onSidebarPreviewEnter={previewSidebar}
								/>
							)}
						</SidebarProvider>
						<OrchestratorReplacementDialog
							error={replacementErrorProjectId ? orchestratorReplacementErrors[replacementErrorProjectId] : undefined}
							onOpenChange={(open) => {
								if (!open && replacementErrorProjectId) setOrchestratorReplacementError(replacementErrorProjectId, null);
							}}
							onRetry={(projectId) => void restartOrchestrator(projectId)}
							onRetryAsTui={(projectId) => void restartOrchestrator(projectId, "tui")}
							projectId={replacementErrorProjectId}
							workspaces={workspaces}
						/>
						<CommandPalette />
					</div>
				</TerminalCacheProvider>
			</SessionTopbarProvider>
		</ShellProvider>
	);
}
