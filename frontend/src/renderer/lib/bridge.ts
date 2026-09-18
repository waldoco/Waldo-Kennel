import type { AoBridge } from "../../preload";
import type { DaemonStatus } from "../../shared/daemon-status";
import { usesLiveDaemonPreview } from "./preview-mode";
import { coerceLocale } from "../../shared/ui-locale";
export type { FeatureBuild } from "../../main/feature-builds";

export const aoBridge: AoBridge =
	window.kennel ??
	({
		app: {
			getVersion: async () => "0.0.0-preview",
			chooseDirectory: async () => null,
			openExternal: async (url: string) => {
				window.open(url, "_blank", "noopener,noreferrer");
			},
			installTmux: async () => ({ status: "failed" as const, message: "Installing tmux requires the desktop app." }),
			scanImportFolder: async ({ path }) => ({ path, repos: [] }),
			checkAncestorRepo: async () => undefined,
			approveAttemptReplacement: async () => {
				throw new Error("Attempt replacement approval requires the desktop app.");
			},
			approveHarnessAuthority: async () => {
				throw new Error("Harness connection approval requires the desktop app.");
			},
			discoverCodex: async () => ({ state: "not_found", message: "Codex discovery requires the desktop app." }),
			getCodexPairing: async () => ({ state: "unpaired" }),
			pairCodex: async () => {
				throw new Error("Codex pairing requires the desktop app.");
			},
			onNewSessionShortcut: () => () => undefined,
			onKeyboardShortcutsHelp: () => () => undefined,
			onNewShellTerminalShortcut: () => () => undefined,
			onCloseShellTerminalShortcut: () => () => undefined,
			setCloseShellTerminalShortcutEnabled: () => undefined,
			onOpenSettingsShortcut: () => () => undefined,
			onPreviousSessionShortcut: () => () => undefined,
			onNextSessionShortcut: () => () => undefined,
			onPreviousTabShortcut: () => () => undefined,
			onNextTabShortcut: () => () => undefined,
			onFocusTerminalShortcut: () => () => undefined,
		},
		terminal: {
			saveDroppedFile: async () => "",
			setFocused: () => undefined,
			onFontSizeShortcut: () => () => undefined,
		},
		window: {
			setOverlay: async () => undefined,
			isFullScreen: async () => false,
			onFullScreen: () => () => undefined,
		},
		island: {
			getState: async () => ({
				supported: false,
				enabled: false,
				visible: false,
				shortcut: "⌘`",
			}),
			setVisible: async (visible) => ({
				supported: false,
				enabled: visible,
				visible: false,
				shortcut: "⌘`",
			}),
			openSettings: async () => ({ open: false }),
			onState: () => () => undefined,
		},
		theme: {
			set: async () => undefined,
		},
		menu: {
			action: async () => undefined,
			notifyShellFocus: () => undefined,
		},
		clipboard: {
			writeText: async (text: string) => {
				if (navigator.clipboard?.writeText) {
					await navigator.clipboard.writeText(text);
				}
			},
			readText: async () => (navigator.clipboard?.readText ? navigator.clipboard.readText() : ""),
		},
		daemon: {
			// Without Electron there is no supervisor to ask, so readiness is
			// probed over HTTP instead. In live preview the daemon sits behind
			// the dev server's proxy, and answering "stopped" — as this used to,
			// unconditionally — made every surface wait forever on a daemon that
			// was in fact running.
			getStatus: async (): Promise<DaemonStatus> => {
				if (!usesLiveDaemonPreview) {
					return { state: "stopped", message: "Electron preload is not available in browser preview." };
				}
				try {
					const response = await fetch("/api/v1/projects", { method: "GET" });
					if (!response.ok) {
						return { state: "error", message: `The daemon answered ${response.status}.` };
					}
					// No port: requests stay same-origin so the proxy is used.
					return { state: "ready" };
				} catch {
					return { state: "stopped", message: "No daemon is reachable through the dev server proxy." };
				}
			},
			start: async () => ({ state: "starting" }),
			stop: async () => ({ state: "stopped" }),
			restart: async () => ({ state: "starting" }),
			onStatus: () => () => undefined,
		},
		telemetry: {
			getBootstrap: async () => null,
		},
		browser: {
			nativeCompositionEnabled: false,
			ensure: async (sessionId: string) => ({
				viewId: `preview:${sessionId}`,
				url: "",
				title: "",
				canGoBack: false,
				canGoForward: false,
				isLoading: false,
			}),
			setBounds: () => undefined,
			setOverlayOpen: () => undefined,
			navigate: async ({ viewId, url }) => ({
				viewId,
				url,
				title: "",
				canGoBack: false,
				canGoForward: false,
				isLoading: false,
			}),
			clear: async (viewId: string) => ({
				viewId,
				url: "",
				title: "",
				canGoBack: false,
				canGoForward: false,
				isLoading: false,
			}),
			goBack: async (viewId: string) => ({
				viewId,
				url: "",
				title: "",
				canGoBack: false,
				canGoForward: false,
				isLoading: false,
			}),
			goForward: async (viewId: string) => ({
				viewId,
				url: "",
				title: "",
				canGoBack: false,
				canGoForward: false,
				isLoading: false,
			}),
			reload: async (viewId: string) => ({
				viewId,
				url: "",
				title: "",
				canGoBack: false,
				canGoForward: false,
				isLoading: false,
			}),
			stop: async (viewId: string) => ({
				viewId,
				url: "",
				title: "",
				canGoBack: false,
				canGoForward: false,
				isLoading: false,
			}),
			getTabs: async (viewId: string) => ({ viewId, activeTabId: "t1", tabs: [] }),
			selectTab: async ({ viewId, tabId }) => ({ viewId, activeTabId: tabId, tabs: [] }),
			closeTab: async ({ viewId }) => ({ viewId, activeTabId: "", tabs: [] }),
			openTab: async ({ viewId }) => ({ viewId, activeTabId: "", tabs: [] }),
			devtools: async ({ viewId, operation }) => ({
				viewId,
				open: operation !== "close",
				activeTabId: "",
			}),
			destroy: () => undefined,
			setAnnotationMode: async () => undefined,
			onNavState: () => () => undefined,
			onTabsState: () => () => undefined,
			onAgentActivity: () => () => undefined,
			onDevToolsState: () => () => undefined,
			onAnnotationSubmit: () => () => undefined,
			onAnnotationCancel: () => () => undefined,
		},
		notifications: {
			show: async () => undefined,
			setBadge: async () => undefined,
			devBounce: async () => undefined,
			onClick: () => () => undefined,
		},
		tray: {
			setAttentionState: () => undefined,
			onOpenSession: () => () => undefined,
		},
		appState: {
			getMigration: async () => ({ status: "pending" }),
			setMigration: async () => undefined,
		},
		updateSettings: {
			get: async () => ({ enabled: false, channel: "latest", nightlyAck: false, feature: null }),
			set: async () => undefined,
		},
		uiSettings: {
			get: async () => ({ locale: "en" as const }),
			set: async (settings) => ({ locale: coerceLocale(settings.locale) }),
		},
		keybindings: {
			get: async () => ({}),
			set: async (overrides) => overrides,
			setRecording: async () => undefined,
		},
		updates: {
			getStatus: async () => ({ state: "idle" }),
			check: async () => undefined,
			returnHome: async () => undefined,
			download: async () => undefined,
			install: async () => undefined,
			onStatus: () => () => undefined,
			onTelemetry: () => () => undefined,
		},
		featureBuilds: {
			list: async () => [],
			getActive: async () => null,
		},
		cloud: {
			getSession: async () => null,
			signIn: async () => undefined,
			signOut: async () => undefined,
			onSessionChanged: () => () => undefined,
		},
	} satisfies AoBridge);
