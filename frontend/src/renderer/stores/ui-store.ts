import { create } from "zustand";
import type { TerminalTarget } from "../types/terminal";
import {
	applyDocumentTheme,
	applyDocumentThemeStyle,
	readStoredThemePreference,
	readStoredThemeStyle,
	resolveTheme,
	runThemeTransition,
	systemTheme,
	themeStorageKey,
	themeStyleStorageKey,
	type Theme,
	type ThemePreference,
	type ThemeStyle,
} from "../lib/theme";

export type { Theme, ThemePreference, ThemeStyle } from "../lib/theme";
export { readStoredThemePreference, readStoredThemeStyle, resolveTheme } from "../lib/theme";

export type SettingsModal =
	| { scope: "global" }
	| {
			scope: "project";
			projectId: string;
	  };

/** Worker detail view toggles — Changes (Git rail) is the default. */
export type WorkbenchTab = "changes" | "files" | "terminal";
/** Board or List reading of the same session lanes. */
export type SessionsViewMode = "board" | "list";
/** List or Graph reading of the Mission WorkUnit projection. */
export type MissionViewMode = "list" | "graph";
export type InspectorView = "summary" | "reviews" | "browser" | "files";

export type InspectorSessionState = {
	isOpen: boolean;
	view: InspectorView;
	/** The current non-empty browser content lifecycle has already been revealed. */
	browserContentRevealed?: boolean;
	/** Real browser activity occurred while Browser was not visible. */
	browserUnseen?: boolean;
};

// Selection (which project/session is open) now lives in the URL — the router
// is the single source of truth, read via route params. This store holds only
// ephemeral UI: theme, sidebar collapse, command palette, per-session inspector
// state, and the active workbench tab within a session.
type UiState = {
	workbenchTab: WorkbenchTab;
	/** Which reading of the sessions lanes is showing. Sticky across launches:
	 *  a person picks board or list once and expects it to stay picked. */
	sessionsViewMode: SessionsViewMode;
	/** Board or List reading of one Outcome's attempt lineage (Act & Observe).
	 *  Sticky across launches, same as sessionsViewMode, but scoped separately
	 *  since it governs a different lane model (attempts, not sessions). */
	outcomeRunViewMode: SessionsViewMode;
	/** Which reading of the Mission graph is showing. Sticky across launches:
	 *  List is the default until the canvas cutover; a stored preference is the
	 *  only way Graph renders. */
	missionViewMode: MissionViewMode;
	/** The current attempt's drill-in terminal panel (Figma's terminal-toggle
	 *  icon in the Work destination's persistent top bar). Ephemeral like the
	 *  command palette: it opens on demand and never persists across launches,
	 *  and it is shared between the WorkShell toggle button and the Board/List
	 *  "Instruct"/"Engage" card actions so both open the exact same panel. */
	isOutcomeAttemptPanelOpen: boolean;
	/** Explicit WorkUnit selection for the attempt panel; null means the
	 * persistent WorkShell toggle resolves the newest Attempt. */
	outcomeAttemptPanelAttemptId: string | null;
	/** Agent seeded into new project and session forms. Empty means "let the app
	 *  pick", which is what a person who skipped the setup tour gets. */
	defaultAgentId: string;
	/** The setup tour has run to the end (or been skipped) at least once. */
	hasCompletedOnboarding: boolean;
	isOnboardingOpen: boolean;
	isSidebarOpen: boolean;
	inspectorSessions: Record<string, InspectorSessionState>;
	isCommandPaletteOpen: boolean;
	/** The global keyboard-shortcuts reference dialog. Lives here (not local
	 *  component state) so any surface — the ⌘/ shortcut, the settings menu,
	 *  WorkShell's bottom-left help button — can open the exact same dialog. */
	isKeyboardShortcutsOpen: boolean;
	settingsModal: SettingsModal | null;
	themePreference: ThemePreference;
	/** Resolved light/dark for React consumers; may track OS while preference is system. */
	resolvedTheme: Theme;
	/** Named color style theme (e.g. "catppuccin", "nord") — independent of light/dark mode. */
	themeStyle: ThemeStyle;
	/** When true, developer-only release controls are available. Default off. */
	developerMode: boolean;
	restartingProjectIds: ReadonlySet<string>;
	orchestratorReplacementErrors: Record<string, OrchestratorReplacementFailure>;
	orchestratorStartupErrors: Record<string, string>;
	// Transient "open the New Task dialog for this project" signal. The nonce
	// bumps on every request so a repeat press (even for the same project) still
	// re-fires; the always-mounted GlobalNewTaskDialog consumes it. Selection
	// still lives in the URL — this is a one-shot action, not persisted state.
	newTaskRequest: { projectId: string; nonce: number; initialPrompt?: string } | null;
	// Bumps to ask the sidebar's create-project flow to open (the ⌘N fallback
	// when no project is in scope).
	createProjectNonce: number;
	// Bumps to ask for a new standalone shell terminal. Like newTaskRequest this
	// is a one-shot signal, not state: the tab-strip + button and Ctrl+Shift+` both
	// raise it so they cannot drift apart, and a repeat press re-fires because
	// the nonce always changes. The shell layout is its single consumer — it is
	// mounted on every route, so the request is honoured from anywhere in the app.
	newShellTerminalNonce: number;
	// The shell terminal the user most recently opened or selected. Both the
	// session view (tabs beside the session's pane) and the standalone terminals
	// view read it, so whichever one is on screen shows the same shell.
	activeShellTerminalHandleId: string | null;
	// Which terminal each mounted session is actually showing. The session pane
	// renders one terminal at a time, so opening a shell or the reviewer swaps
	// the agent's terminal off screen even though the route still points at that
	// session. Surfaces outside the session subtree (the notification runtime)
	// need that distinction, and SessionView's own target is local state.
	visibleTerminalKindBySession: Record<string, TerminalTarget["kind"]>;
	setWorkbenchTab: (tab: WorkbenchTab) => void;
	setSessionsViewMode: (mode: SessionsViewMode) => void;
	setOutcomeRunViewMode: (mode: SessionsViewMode) => void;
	setMissionViewMode: (mode: MissionViewMode) => void;
	openOutcomeAttemptPanel: (attemptId?: string) => void;
	closeOutcomeAttemptPanel: () => void;
	toggleOutcomeAttemptPanel: () => void;
	setDefaultAgentId: (agentId: string) => void;
	openOnboarding: () => void;
	closeOnboarding: () => void;
	setThemePreference: (theme: ThemePreference) => void;
	setThemeStyle: (style: ThemeStyle) => void;
	setDeveloperMode: (enabled: boolean) => void;
	openGlobalSettings: () => void;
	openProjectSettings: (projectId: string) => void;
	closeSettings: () => void;
	/** Refresh resolvedTheme from OS without writing light/dark to storage. */
	syncSystemTheme: () => void;
	toggleSidebar: () => void;
	setInspectorOpen: (sessionId: string, isOpen: boolean) => void;
	toggleInspector: (sessionId: string) => void;
	setInspectorView: (sessionId: string, view: InspectorView) => void;
	setBrowserContentRevealed: (sessionId: string, revealed: boolean) => void;
	setBrowserUnseen: (sessionId: string, unseen: boolean) => void;
	setCommandPaletteOpen: (open: boolean) => void;
	setKeyboardShortcutsOpen: (open: boolean) => void;
	setProjectRestarting: (projectId: string, restarting: boolean) => void;
	setOrchestratorReplacementError: (projectId: string, failure: OrchestratorReplacementFailure | null) => void;
	setOrchestratorStartupError: (projectId: string, message: string | null) => void;
	requestNewTask: (projectId: string, initialPrompt?: string) => void;
	requestCreateProject: () => void;
	requestNewShellTerminal: () => void;
	setActiveShellTerminal: (handleId: string | null) => void;
	setVisibleTerminalKind: (sessionId: string, kind: TerminalTarget["kind"]) => void;
	clearVisibleTerminalKind: (sessionId: string) => void;
};

export type OrchestratorReplacementFailure = {
	message: string;
	code?: string;
	requestId?: string;
};

const sidebarStorageKey = "kennel.sidebar.open";
const sessionsViewModeStorageKey = "kennel.sessions.viewMode";
const outcomeRunViewModeStorageKey = "kennel.outcome.runViewMode";
const missionViewModeStorageKey = "kennel.mission.viewMode";
const defaultAgentStorageKey = "kennel.agent.default";
const onboardingStorageKey = "kennel.onboarding.completed";
const developerModeStorageKey = "kennel.developerMode";
function getLocalStorage() {
	if (typeof window === "undefined" || !window.localStorage) return null;
	return window.localStorage;
}

function initialSidebarOpen() {
	return getLocalStorage()?.getItem(sidebarStorageKey) !== "false";
}

function initialSessionsViewMode(): SessionsViewMode {
	return getLocalStorage()?.getItem(sessionsViewModeStorageKey) === "list" ? "list" : "board";
}

function initialOutcomeRunViewMode(): SessionsViewMode {
	return getLocalStorage()?.getItem(outcomeRunViewModeStorageKey) === "list" ? "list" : "board";
}

function initialMissionViewMode(): MissionViewMode {
	// List-first by design: only an explicit stored "graph" opts into the canvas.
	return getLocalStorage()?.getItem(missionViewModeStorageKey) === "graph" ? "graph" : "list";
}

function initialDefaultAgentId() {
	return getLocalStorage()?.getItem(defaultAgentStorageKey) ?? "";
}

function initialHasCompletedOnboarding() {
	return getLocalStorage()?.getItem(onboardingStorageKey) === "true";
}

function initialDeveloperMode() {
	return getLocalStorage()?.getItem(developerModeStorageKey) === "true";
}

function inspectorState(sessions: Record<string, InspectorSessionState>, sessionId: string): InspectorSessionState {
	return sessions[sessionId] ?? { isOpen: true, view: "summary" };
}

const initialThemePreference = readStoredThemePreference();
const initialThemeStyle = readStoredThemeStyle();

export const useUiStore = create<UiState>((set, get) => ({
	workbenchTab: "changes",
	sessionsViewMode: initialSessionsViewMode(),
	outcomeRunViewMode: initialOutcomeRunViewMode(),
	missionViewMode: initialMissionViewMode(),
	isOutcomeAttemptPanelOpen: false,
	outcomeAttemptPanelAttemptId: null,
	defaultAgentId: initialDefaultAgentId(),
	hasCompletedOnboarding: initialHasCompletedOnboarding(),
	isOnboardingOpen: false,
	isSidebarOpen: initialSidebarOpen(),
	inspectorSessions: {},
	isCommandPaletteOpen: false,
	isKeyboardShortcutsOpen: false,
	settingsModal: null,
	themePreference: initialThemePreference,
	resolvedTheme: resolveTheme(initialThemePreference),
	themeStyle: initialThemeStyle,
	developerMode: initialDeveloperMode(),
	restartingProjectIds: new Set<string>(),
	orchestratorReplacementErrors: {},
	orchestratorStartupErrors: {},
	newTaskRequest: null,
	createProjectNonce: 0,
	newShellTerminalNonce: 0,
	activeShellTerminalHandleId: null,
	visibleTerminalKindBySession: {},
	setWorkbenchTab: (workbenchTab) => set({ workbenchTab }),
	setSessionsViewMode: (sessionsViewMode) => {
		getLocalStorage()?.setItem(sessionsViewModeStorageKey, sessionsViewMode);
		set({ sessionsViewMode });
	},
	setOutcomeRunViewMode: (outcomeRunViewMode) => {
		getLocalStorage()?.setItem(outcomeRunViewModeStorageKey, outcomeRunViewMode);
		set({ outcomeRunViewMode });
	},
	setMissionViewMode: (missionViewMode) => {
		getLocalStorage()?.setItem(missionViewModeStorageKey, missionViewMode);
		set({ missionViewMode });
	},
	openOutcomeAttemptPanel: (outcomeAttemptPanelAttemptId) =>
		set({ isOutcomeAttemptPanelOpen: true, outcomeAttemptPanelAttemptId: outcomeAttemptPanelAttemptId ?? null }),
	closeOutcomeAttemptPanel: () => set({ isOutcomeAttemptPanelOpen: false, outcomeAttemptPanelAttemptId: null }),
	toggleOutcomeAttemptPanel: () =>
		set((state) => ({
			isOutcomeAttemptPanelOpen: !state.isOutcomeAttemptPanelOpen,
			// The global toggle means "newest Attempt" for this Outcome.
			outcomeAttemptPanelAttemptId: null,
		})),
	setDefaultAgentId: (defaultAgentId) => {
		getLocalStorage()?.setItem(defaultAgentStorageKey, defaultAgentId);
		set({ defaultAgentId });
	},
	openOnboarding: () => set({ isOnboardingOpen: true }),
	// Closing is the completion event whichever exit was used: a person who
	// dismisses the tour has answered it, and should not meet it again on relaunch.
	closeOnboarding: () => {
		getLocalStorage()?.setItem(onboardingStorageKey, "true");
		set({ isOnboardingOpen: false, hasCompletedOnboarding: true });
	},
	setThemePreference: (themePreference) => {
		if (get().themePreference === themePreference) return;
		runThemeTransition(() => {
			const resolvedTheme = resolveTheme(themePreference);
			getLocalStorage()?.setItem(themeStorageKey, themePreference);
			applyDocumentTheme(resolvedTheme);
			set({ themePreference, resolvedTheme });
		});
	},
	setThemeStyle: (themeStyle) => {
		if (get().themeStyle === themeStyle) return;
		runThemeTransition(() => {
			getLocalStorage()?.setItem(themeStyleStorageKey, themeStyle);
			applyDocumentThemeStyle(themeStyle);
			set({ themeStyle });
		});
	},
	setDeveloperMode: (developerMode) => {
		getLocalStorage()?.setItem(developerModeStorageKey, String(developerMode));
		set({ developerMode });
	},
	openGlobalSettings: () => set({ settingsModal: { scope: "global" } }),
	openProjectSettings: (projectId) => set({ settingsModal: { scope: "project", projectId } }),
	closeSettings: () => set({ settingsModal: null }),
	syncSystemTheme: () => {
		const { themePreference, resolvedTheme } = get();
		if (themePreference !== "system") return;
		const next = systemTheme();
		if (next === resolvedTheme) return;
		runThemeTransition(() => {
			applyDocumentTheme(next);
			set({ resolvedTheme: next });
		});
	},
	toggleSidebar: () =>
		set((state) => {
			const isSidebarOpen = !state.isSidebarOpen;
			getLocalStorage()?.setItem(sidebarStorageKey, String(isSidebarOpen));
			return { isSidebarOpen };
		}),
	setInspectorOpen: (sessionId, isOpen) =>
		set((state) => {
			const current = inspectorState(state.inspectorSessions, sessionId);
			return {
				inspectorSessions: {
					...state.inspectorSessions,
					[sessionId]: { ...current, isOpen },
				},
			};
		}),
	toggleInspector: (sessionId) =>
		set((state) => {
			const current = inspectorState(state.inspectorSessions, sessionId);
			return {
				inspectorSessions: {
					...state.inspectorSessions,
					[sessionId]: { ...current, isOpen: !current.isOpen },
				},
			};
		}),
	setInspectorView: (sessionId, view) =>
		set((state) => {
			const current = inspectorState(state.inspectorSessions, sessionId);
			const browserUnseen = view === "browser" ? false : current.browserUnseen;
			return {
				inspectorSessions: {
					...state.inspectorSessions,
					[sessionId]: { ...current, view, browserUnseen },
				},
			};
		}),
	setBrowserContentRevealed: (sessionId, browserContentRevealed) =>
		set((state) => {
			const current = inspectorState(state.inspectorSessions, sessionId);
			if (Boolean(current.browserContentRevealed) === browserContentRevealed) return state;
			return {
				inspectorSessions: {
					...state.inspectorSessions,
					[sessionId]: {
						...current,
						browserContentRevealed,
						browserUnseen: browserContentRevealed ? current.browserUnseen : false,
					},
				},
			};
		}),
	setBrowserUnseen: (sessionId, browserUnseen) =>
		set((state) => {
			const current = inspectorState(state.inspectorSessions, sessionId);
			if (Boolean(current.browserUnseen) === browserUnseen) return state;
			return {
				inspectorSessions: {
					...state.inspectorSessions,
					[sessionId]: { ...current, browserUnseen },
				},
			};
		}),
	setCommandPaletteOpen: (isCommandPaletteOpen) => set({ isCommandPaletteOpen }),
	setKeyboardShortcutsOpen: (isKeyboardShortcutsOpen) => set({ isKeyboardShortcutsOpen }),
	setProjectRestarting: (projectId, restarting) =>
		set((state) => {
			const restartingProjectIds = new Set(state.restartingProjectIds);
			if (restarting) {
				restartingProjectIds.add(projectId);
			} else {
				restartingProjectIds.delete(projectId);
			}
			return { restartingProjectIds };
		}),
	setOrchestratorReplacementError: (projectId, failure) =>
		set((state) => {
			const orchestratorReplacementErrors = { ...state.orchestratorReplacementErrors };
			if (failure) {
				orchestratorReplacementErrors[projectId] = failure;
			} else {
				delete orchestratorReplacementErrors[projectId];
			}
			return { orchestratorReplacementErrors };
		}),
	setOrchestratorStartupError: (projectId, message) =>
		set((state) => {
			const orchestratorStartupErrors = { ...state.orchestratorStartupErrors };
			if (message) {
				orchestratorStartupErrors[projectId] = message;
			} else {
				delete orchestratorStartupErrors[projectId];
			}
			return { orchestratorStartupErrors };
		}),
	requestNewTask: (projectId, initialPrompt) =>
		set((state) => ({
			newTaskRequest: {
				projectId,
				nonce: (state.newTaskRequest?.nonce ?? 0) + 1,
				...(initialPrompt ? { initialPrompt } : {}),
			},
		})),
	requestCreateProject: () => set((state) => ({ createProjectNonce: state.createProjectNonce + 1 })),
	requestNewShellTerminal: () => set((state) => ({ newShellTerminalNonce: state.newShellTerminalNonce + 1 })),
	setActiveShellTerminal: (activeShellTerminalHandleId) => set({ activeShellTerminalHandleId }),
	setVisibleTerminalKind: (sessionId, kind) =>
		set((state) =>
			state.visibleTerminalKindBySession[sessionId] === kind
				? state
				: { visibleTerminalKindBySession: { ...state.visibleTerminalKindBySession, [sessionId]: kind } },
		),
	clearVisibleTerminalKind: (sessionId) =>
		set((state) => {
			if (!(sessionId in state.visibleTerminalKindBySession)) return state;
			const visibleTerminalKindBySession = { ...state.visibleTerminalKindBySession };
			delete visibleTerminalKindBySession[sessionId];
			return { visibleTerminalKindBySession };
		}),
}));

export function useResolvedTheme(): Theme {
	return useUiStore((state) => state.resolvedTheme);
}
