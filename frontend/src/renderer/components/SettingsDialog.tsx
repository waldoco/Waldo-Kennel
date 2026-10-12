import { Bot, CircleHelp, GitBranch, Inbox, Link2, MonitorCog, RefreshCw, Settings2, TriangleAlert, X } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { GlobalSettingsForm, type GlobalSettingsSection } from "./GlobalSettingsForm";
import {
	ProjectSettingsForm,
	type ProjectSettingsSaveState,
	type ProjectSettingsSection,
} from "./ProjectSettingsForm";
import { KeyboardShortcutsSettingsDialog } from "./settings/KeyboardShortcutsSettingsDialog";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
	settingsDialogBodyClass,
	settingsDialogContentClass,
	settingsDialogHeaderClass,
} from "./ui/dialog";
import { type SettingsModal, useUiStore } from "../stores/ui-store";
import { cn } from "../lib/utils";
import { Button } from "./ui/button";

function initialProjectSaveState(): ProjectSettingsSaveState {
	return {
		isPending: false,
		showSaving: false,
		validationError: null,
		mutationError: null,
		saved: false,
		replacementError: null,
	};
}

export function SettingsDialog({ waldoPreview, initialGlobalSection = "general" }: { waldoPreview?: ReactNode; initialGlobalSection?: Exclude<GlobalSettingsSection, "all"> } = {}) {
	const { t } = useTranslation();
	const settingsModal = useUiStore((state) => state.settingsModal);
	const closeSettings = useUiStore((state) => state.closeSettings);
	const openGlobalSettings = useUiStore((state) => state.openGlobalSettings);
	const openProjectSettings = useUiStore((state) => state.openProjectSettings);
	const [keyboardShortcutsOpen, setKeyboardShortcutsOpen] = useState(false);
	const keyboardShortcutsRestoreRef = useRef<SettingsModal | null>(null);

	// Keep the last non-null settings so the content stays rendered during the
	// exit animation (when settingsModal is already null but the dialog hasn't
	// finished animating out). Using a ref updated inline avoids the one-frame
	// gap that would occur with a useEffect-based approach: if DialogContent is
	// not rendered on the same frame that Dialog becomes open={true}, Radix's
	// DismissableLayer never registers and outside-click detection breaks.
	const lastSettingsRef = useRef<SettingsModal | null>(settingsModal);
	if (settingsModal !== null) lastSettingsRef.current = settingsModal;
	const displaySettings = lastSettingsRef.current;

	const globalSections: Array<{ id: Exclude<GlobalSettingsSection, "all">; label: string; icon: typeof Settings2 }> = [
		{ id: "general", label: t("settings.general"), icon: Settings2 },
		{ id: "waldo", label: t("settings.waldo.title"), icon: Link2 },
		{ id: "reasoning", label: t("settings.reasoning.navigation"), icon: Bot },
		{ id: "updates", label: t("settings.updates"), icon: RefreshCw },
		{ id: "help", label: t("settings.help"), icon: CircleHelp },
	];

	const projectSections: Array<{ id: ProjectSettingsSection; label: string; icon: typeof Settings2 }> = [
		{ id: "general", label: t("settings.project.identity"), icon: MonitorCog },
		{ id: "agents", label: t("settings.project.agents"), icon: Bot },
		{ id: "workflow", label: t("settings.project.workflow"), icon: GitBranch },
		{ id: "intake", label: t("settings.project.intake"), icon: Inbox },
	];

	const isProjectSettings = displaySettings?.scope === "project";
	const [activeSection, setActiveSection] = useState<Exclude<GlobalSettingsSection, "all">>(initialGlobalSection);
	const isWaldoSettings = displaySettings?.scope === "global" && activeSection === "waldo";
	const [activeProjectSection, setActiveProjectSection] = useState<ProjectSettingsSection>("general");
	const [projectSaveState, setProjectSaveState] = useState<ProjectSettingsSaveState>(initialProjectSaveState);

	const activeLabel = isProjectSettings
		? (projectSections.find((s) => s.id === activeProjectSection)?.label ?? t("settings.project.identity"))
		: (globalSections.find((section) => section.id === activeSection)?.label ?? t("settings.general"));

	const openKeyboardShortcuts = () => {
		if (!settingsModal) return;
		keyboardShortcutsRestoreRef.current = settingsModal;
		setKeyboardShortcutsOpen(true);
		closeSettings();
	};

	const restoreSettings = () => {
		const previousSettings = keyboardShortcutsRestoreRef.current;
		keyboardShortcutsRestoreRef.current = null;
		if (!previousSettings) return;
		if (previousSettings.scope === "global") openGlobalSettings();
		else openProjectSettings(previousSettings.projectId);
	};

	const closeSettingsDialog = () => {
		if (isProjectSettings && projectSaveState.isPending) return;
		closeSettings();
	};

	useEffect(() => {
		if (settingsModal?.scope === "global") setActiveSection(initialGlobalSection);
		if (settingsModal?.scope === "project") {
			setActiveProjectSection("general");
			setProjectSaveState(initialProjectSaveState());
		}
	}, [settingsModal, initialGlobalSection]);

	return (
		<>
			<Dialog open={settingsModal !== null} onOpenChange={(open) => !open && closeSettingsDialog()}>
			<DialogContent
				className={cn(
					settingsDialogContentClass,
					"h-(--size-settings-dialog-height) w-(--size-settings-dialog-wide) max-h-none origin-center overflow-hidden p-0",
					isWaldoSettings && "h-[min(var(--size-settings-dialog-height),calc(100svh-var(--space-4)))] w-[min(var(--size-settings-dialog-wide),calc(100vw-var(--space-4)))]",
				)}
				showCloseButton={false}
			>
				{displaySettings && (
					<div className={cn("flex h-full min-h-0", isWaldoSettings && "max-sm:flex-col")}>
						<aside className={cn("flex w-52 shrink-0 flex-col border-r-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-surface", isWaldoSettings && "max-sm:w-full max-sm:border-r-0 max-sm:border-b")}>
						<p className="wd-label px-5 pb-2 pt-5 text-wd-tertiary">{t("settings.title")}</p>
						<nav aria-label={t("settings.navSectionsAria")} className={cn("flex flex-col gap-0.5 px-3 pb-3", isWaldoSettings && "max-sm:flex-row max-sm:overflow-x-auto max-sm:[&_button]:w-auto max-sm:[&_button]:shrink-0")}>
							{isProjectSettings
								? projectSections.map(({ id, label, icon }) => (
										<SettingsNavItem
											active={activeProjectSection === id}
											icon={icon}
											key={id}
											label={label}
											onClick={() => setActiveProjectSection(id)}
										/>
									))
								: globalSections.map(({ id, label, icon }) => (
										<SettingsNavItem
											active={activeSection === id}
											icon={icon}
											key={id}
											label={label}
											onClick={() => setActiveSection(id)}
										/>
									))}
						</nav>
						{isProjectSettings && (
							<div className="mt-auto flex flex-col gap-2 border-t-[length:var(--stroke-hairline)] border-wd-hairline p-4">
								<Button
									type="submit"
									form="project-settings-form"
									variant={projectSaveState.validationError || projectSaveState.mutationError ? "critical" : "primary"}
									className="w-full"
									disabled={projectSaveState.isPending}
									aria-live="polite"
									title={
										projectSaveState.validationError ??
										projectSaveState.mutationError ??
										(projectSaveState.replacementError
											? t("settings.project.restartFailed", { error: projectSaveState.replacementError })
											: undefined)
									}
								>
									{projectSaveState.showSaving ? (
										t("settings.project.saving")
									) : projectSaveState.saved ? (
										t("settings.project.saved")
									) : projectSaveState.validationError || projectSaveState.mutationError ? (
										<>
											<TriangleAlert className="size-4" aria-hidden="true" />
											{t("settings.project.saveFailed")}
										</>
									) : (
										t("settings.project.saveChanges")
									)}
								</Button>
								<span className="sr-only" role="status" aria-live="polite">
									{projectSaveState.validationError ?? projectSaveState.mutationError ??
										(projectSaveState.saved ? t("settings.project.saved") : "")}
								</span>
							</div>
						)}
					</aside>

					{/* Main area — same bg as the app page */}
					<div className={cn("flex min-w-0 flex-1 flex-col bg-wd-canvas", isWaldoSettings && "min-h-0")}>
						<DialogHeader className={cn(settingsDialogHeaderClass, "flex h-auto shrink-0 flex-row items-center justify-between border-b-0 px-8 pb-2 pt-8")}>
							<DialogTitle className="wd-display-3 font-display font-normal text-wd-primary">{activeLabel}</DialogTitle>
							<DialogDescription className="sr-only">
								{isProjectSettings ? t("settings.project.dialogDescription") : t("settings.dialogDescription", { section: activeLabel.toLowerCase() })}
							</DialogDescription>
							<DialogClose
								aria-label={t("settings.close")}
								className="settings-close-button"
								disabled={isProjectSettings && projectSaveState.isPending}
							>
								<X aria-hidden="true" className="size-4" />
							</DialogClose>
						</DialogHeader>
						<div className={cn(settingsDialogBodyClass, "flex-1 px-8 pb-10 pt-4 [&>*]:mx-auto [&>*]:w-full [&>*]:max-w-[40rem]")}>
							{displaySettings?.scope === "project" ? (
								<ProjectSettingsForm
									projectId={displaySettings.projectId}
									section={activeProjectSection}
									onSaveState={setProjectSaveState}
								/>
							) : (
								<GlobalSettingsForm
									section={activeSection}
									waldoOpen={settingsModal !== null}
									waldoPreview={waldoPreview}
									onOpenKeyboardShortcuts={openKeyboardShortcuts}
								/>
							)}
						</div>
					</div>
					</div>
				)}
		</DialogContent>
			</Dialog>
			<KeyboardShortcutsSettingsDialog
				open={keyboardShortcutsOpen}
				onOpenChange={(open) => {
					setKeyboardShortcutsOpen(open);
					if (!open) restoreSettings();
				}}
			/>
		</>
	);
}

function SettingsNavItem({
	active,
	icon: Icon,
	label,
	onClick,
}: {
	active: boolean;
	icon: typeof Settings2;
	label: string;
	onClick: () => void;
}) {
	return (
		<button
			aria-current={active ? "page" : undefined}
			className={cn(
				"wd-label flex h-9 w-full items-center gap-2.5 rounded-md px-3 text-left transition-[background-color,color,transform] duration-fast ease-out active:scale-press focus-visible:outline-offset-[-2px]",
				active
					? "bg-wd-selected text-wd-primary"
					: "text-wd-secondary hover:bg-wd-hover hover:text-wd-primary",
			)}
			onClick={onClick}
			type="button"
		>
			<Icon aria-hidden="true" className="size-4 shrink-0" />
			{label}
		</button>
	);
}
