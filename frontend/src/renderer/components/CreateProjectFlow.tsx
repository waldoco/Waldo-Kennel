import * as Dialog from "@radix-ui/react-dialog";
import { useTranslation } from "react-i18next";
import { CheckCircle2, ChevronRight, Folder, FolderPlus, X, XCircle } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import type { ImportFolderScan } from "../../preload";
import { aoBridge } from "../lib/bridge";
import { cn } from "../lib/utils";
import type { ProjectKind } from "../types/workspace";
import { CreateProjectAgentSheet, type CreateProjectAgentSelection } from "./CreateProjectAgentSheet";
import { Button } from "./ui/button";

export type CreateProjectInput = { path: string; asWorkspace?: boolean } & CreateProjectAgentSelection;

type CreateProjectFlowMode = ProjectKind | "choose";

// Shared project entry: new or existing work, location, then reviewed setup.
// All entry points retain the same registration and Git recovery path.
export function CreateProjectFlow({
	children,
	embedded = false,
	idleLabel,
	mode = "single_repo",
	onCreateProject,
	onInitializeProject,
	openSignal,
}: {
	children?: (state: { choosePath: () => void; disabled: boolean; error: string | null; label: string }) => ReactNode;
	// When true, render the Workspace/Project chooser inline (start page) instead
	// of behind a trigger + dialog. Folder validation + agent sheet stay modal.
	embedded?: boolean;
	idleLabel?: string;
	mode?: CreateProjectFlowMode;
	onCreateProject: (input: CreateProjectInput) => Promise<void>;
	onInitializeProject: (path: string) => Promise<void>;
	// Monotonic counter: each new value opens the flow programmatically (the ⌘N
	// "no project in scope" fallback). Lets the shortcut reuse the sidebar's own
	// create-project flow instead of a separate delegating component.
	openSignal?: number;
}) {
	const { t } = useTranslation();
	const resolvedIdleLabel = idleLabel ?? t("createProject.newProject");
	const [error, setError] = useState<string | null>(null);
	const [modePickerOpen, setModePickerOpen] = useState(false);
	const [folderPickerOpen, setFolderPickerOpen] = useState(false);
	const [selectedKind, setSelectedKind] = useState<ProjectKind>(mode === "workspace" ? "workspace" : "single_repo");
	const [selectedPath, setSelectedPath] = useState<string | null>(null);
	const [validationScan, setValidationScan] = useState<ImportFolderScan | null>(null);
	const [isChoosingPath, setIsChoosingPath] = useState(false);
	const [isCreating, setIsCreating] = useState(false);
	const [isInitializing, setIsInitializing] = useState(false);
	const [repositorySetup, setRepositorySetup] = useState<"NOT_A_GIT_REPO" | "PROJECT_UNBORN" | null>(null);
	const [repositorySetupWarning, setRepositorySetupWarning] = useState<string | null>(null);

	const hasModePicker = mode === "choose";
	const isBusy = isChoosingPath || isCreating || isInitializing;

	const openFolderStep = (kind: ProjectKind, startNew = false) => {
		// Keep the selector mounted behind the native picker. Closing it first
		// exposes a blank compositor frame on Windows before Explorer takes focus.
		void chooseDirectory(kind, startNew);
	};

	const chooseDirectory = async (kind: ProjectKind, startNew = false) => {
		setError(null);
		setValidationScan(null);
		setRepositorySetup(null);
		setRepositorySetupWarning(null);
		setSelectedKind(kind);
		setIsChoosingPath(true);
		try {
			const path = await aoBridge.app.chooseDirectory(
				startNew ? t("createProject.chooseNewFolder") : kind === "workspace" ? t("createProject.chooseWorkspace") : t("createProject.chooseRepo"),
			);
			if (path && kind === "single_repo") {
				const preflight = await projectRepositoryPreflight(path);
				if (preflight.blockingError) {
					setError(preflight.blockingError);
					setValidationScan(preflight.scan);
					setModePickerOpen(false);
					setFolderPickerOpen(true);
					return;
				}
				setRepositorySetup(preflight.setupCode);
				setRepositorySetupWarning(preflight.setupWarning);
			}
			if (path && kind === "workspace") {
				try {
					const warning = await aoBridge.app.checkAncestorRepo(path);
					if (warning) {
						setRepositorySetupWarning(warning);
						setRepositorySetup("NOT_A_GIT_REPO");
					}
				} catch {
					// Ancestor check failed — proceed without warning
				}
			}
			if (path) {
				setModePickerOpen(false);
				setSelectedPath(path);
				setFolderPickerOpen(false);
			}
		} catch (err) {
			setError(err instanceof Error ? err.message : t("createProject.couldNotAdd"));
		} finally {
			setIsChoosingPath(false);
		}
	};

	const startFlow = () => {
		if (hasModePicker) {
			setError(null);
			setModePickerOpen(true);
			return;
		}
		void chooseDirectory(mode);
	};

	// Seed with the current value so we never open on mount; open when it changes.
	const lastOpenSignal = useRef(openSignal);
	useEffect(() => {
		if (openSignal === undefined || openSignal === lastOpenSignal.current) return;
		lastOpenSignal.current = openSignal;
		startFlow();
	}, [openSignal]);

	const createProject = async (selection: CreateProjectAgentSelection) => {
		if (!selectedPath) return;
		setError(null);
		setIsCreating(true);
		try {
			if (selectedKind === "single_repo" && repositorySetup) {
				setIsCreating(false);
				setIsInitializing(true);
				await onInitializeProject(selectedPath);
				setRepositorySetup(null);
				setRepositorySetupWarning(null);
				setIsInitializing(false);
				setIsCreating(true);
			}
			await onCreateProject({ path: selectedPath, asWorkspace: selectedKind === "workspace", ...selection });
			setSelectedPath(null);
		} catch (err) {
			const code = err instanceof Error && "code" in err ? (err.code as string | undefined) : undefined;
			const message = err instanceof Error ? err.message : t("createProject.couldNotAdd");
			if (selectedKind === "single_repo" && isRepositorySetupRecoveryCode(code)) setRepositorySetup(code);
			setError(message);
			if (hasModePicker) {
				if (shouldScanCreateFailure(message)) {
					try {
						const scan = await aoBridge.app.scanImportFolder({
							path: selectedPath,
							mode: selectedKind === "workspace" ? "workspace" : "project",
						});
						setValidationScan(scan);
					} catch {
						setValidationScan({ path: selectedPath, repos: [] });
					}
				} else {
					setValidationScan(null);
				}
				setSelectedPath(null);
				setFolderPickerOpen(true);
			}
		} finally {
			setIsCreating(false);
			setIsInitializing(false);
		}
	};

	const label = isChoosingPath
		? t("createProject.opening")
		: isInitializing
			? hasModePicker
				? t("createProject.initializing")
				: t("createProject.settingUp")
			: isCreating
				? t("createProject.creating")
				: resolvedIdleLabel;

	return (
		<>
			{!embedded &&
				children?.({
					choosePath: startFlow,
					disabled: isBusy,
					error,
					label,
				})}
			{hasModePicker && embedded && !modePickerOpen && (
				<div className="flex w-full flex-col items-center gap-3">
					<ImportModePicker disabled={isBusy} onSelect={openFolderStep} />
					{error && !folderPickerOpen && selectedPath === null && (
						<p className="wd-body-s text-critical" data-testid="create-project-error" role="status">
							{error}
						</p>
					)}
				</div>
			)}
			{hasModePicker && (
				<>
					<CreateProjectModeDialog
						disabled={isBusy}
						open={modePickerOpen}
						onOpenChange={(open) => !isBusy && setModePickerOpen(open)}
						onSelect={openFolderStep}
					/>
					<CreateProjectFolderDialog
						disabled={isBusy}
						error={error}
						kind={selectedKind}
						open={folderPickerOpen}
						scan={validationScan}
						onBack={() => {
							setError(null);
							setValidationScan(null);
							setFolderPickerOpen(false);
							if (!embedded) {
								window.requestAnimationFrame(() => setModePickerOpen(true));
							}
						}}
						onChooseFolder={() => void chooseDirectory(selectedKind)}
						onOpenChange={(open) => {
							if (!isBusy) {
								setFolderPickerOpen(open);
								if (!open) {
									setError(null);
									setValidationScan(null);
								}
							}
						}}
					/>
				</>
			)}
			<CreateProjectAgentSheet
				error={error}
				isCreating={isCreating}
				isInitializing={isInitializing}
				kind={selectedKind}
				onOpenChange={(open) => {
					if (!open) {
						setSelectedPath(null);
						if (!folderPickerOpen) {
							setError(null);
						}
					}
				}}
				onSubmit={createProject}
				open={selectedPath !== null}
				path={selectedPath}
				repositorySetupNeeded={repositorySetup !== null}
				repositorySetupWarning={repositorySetupWarning}
			/>
			{error && !hasModePicker && (
				<span className="sr-only" role="status">
					{error}
				</span>
			)}
		</>
	);
}

function isRepositorySetupRecoveryCode(code: string | undefined): code is "NOT_A_GIT_REPO" | "PROJECT_UNBORN" {
	return code === "NOT_A_GIT_REPO" || code === "PROJECT_UNBORN";
}

type RepositorySetupCode = "NOT_A_GIT_REPO" | "PROJECT_UNBORN";

type ProjectRepositoryPreflight = {
	blockingError: string | null;
	scan: ImportFolderScan | null;
	setupCode: RepositorySetupCode | null;
	setupWarning: string | null;
};

async function projectRepositoryPreflight(path: string): Promise<ProjectRepositoryPreflight> {
	try {
		const scan = await aoBridge.app.scanImportFolder({ path, mode: "project" });
		const repo = scan.repos[0];
		const reason = repo?.reason ?? "";
		if (reason.startsWith("Selected folder is inside Kennel's internal data directory.")) {
			return {
				blockingError: reason,
				scan,
				setupCode: null,
				setupWarning: null,
			};
		}
		if (!repo) {
			return { blockingError: null, scan, setupCode: "NOT_A_GIT_REPO", setupWarning: scan.setupWarning ?? null };
		}
		return {
			blockingError: null,
			scan,
			setupCode: repositorySetupCodeFor(repo),
			setupWarning: scan.setupWarning ?? null,
		};
	} catch {
		return { blockingError: null, scan: null, setupCode: null, setupWarning: null };
	}
}

// The scanner marks plain folders, unborn repositories, and committed
// repositories without a remote as needsGitInit. Only the first two need
// Kennel's initializer before registration; a committed repository registers
// directly even without a remote.
function repositorySetupCodeFor(repo: ImportFolderScan["repos"][number]): RepositorySetupCode | null {
	if (!repo.needsGitInit) return null;
	if (repo.reason === "Repository must have at least one commit.") return "PROJECT_UNBORN";
	return repo.branch === "" ? "NOT_A_GIT_REPO" : null;
}

function shouldScanCreateFailure(message: string): boolean {
	if (/daemon|server|conflict|already exists|not ready|start|orchestrator|permission denied/i.test(message))
		return false;
	if (/\b(?:PATH|ID)_ALREADY_REGISTERED\b/i.test(message) || /already registered/i.test(message)) return false;
	return /workspace|repo|repository|git|path|folder|worktree|bare|branch|commit|remote/i.test(message);
}

function CreateProjectModeDialog({
	disabled,
	onOpenChange,
	onSelect,
	open,
}: {
	disabled: boolean;
	onOpenChange: (open: boolean) => void;
	onSelect: (kind: ProjectKind, startNew?: boolean) => void;
	open: boolean;
}) {
	return (
		<Dialog.Root open={open} onOpenChange={onOpenChange}>
			<Dialog.Portal>
				<Dialog.Overlay className="dialog-overlay data-[state=open]:animate-overlay-in" />
				<Dialog.Content className="fixed left-1/2 top-1/2 z-overlay w-[min(var(--size-import-modal-max),calc(100vw-24px))] -translate-x-1/2 -translate-y-1/2 border-0 bg-transparent p-0 shadow-none outline-none data-[state=open]:animate-modal-in">
					<ImportModePicker disabled={disabled} onClose={() => onOpenChange(false)} onSelect={onSelect} dialog />
				</Dialog.Content>
			</Dialog.Portal>
		</Dialog.Root>
	);
}

// Waldo ChoiceList row: surface, hairline, radius-lg; the row is the target.
const choiceRowClass =
	"flex w-full items-center gap-3 rounded-lg border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-surface p-4 text-left transition-colors duration-fast hover:bg-wd-hover active:bg-wd-pressed disabled:pointer-events-none disabled:text-wd-disabled";

/** Shared project intent chooser. */
function ImportModePicker({
	dialog = false,
	disabled,
	onClose,
	onSelect,
}: {
	dialog?: boolean;
	disabled: boolean;
	onClose?: () => void;
	onSelect: (kind: ProjectKind, startNew?: boolean) => void;
}) {
	const { t } = useTranslation();
	return (
		<>
			{dialog && (
				<>
					<Dialog.Title className="sr-only">{t("createProject.addTitle")}</Dialog.Title>
					<Dialog.Description className="sr-only">{t("createProject.addDescription")}</Dialog.Description>
				</>
			)}
			<div
				className={cn(
					"relative flex w-full flex-col gap-6 rounded-xl p-6",
					dialog
						? "border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-raised shadow-[var(--shadow-overlay)]"
						: "bg-transparent",
				)}
			>
				<div className="flex flex-col gap-1 pr-10">
					<h2 className="wd-title text-wd-primary">{t("createProject.addTitle")}</h2>
					<p className="wd-body-s text-wd-secondary">{t("createProject.addDescription")}</p>
				</div>
				{onClose && <button type="button" className="settings-dialog-close-button settings-close-button" aria-label={t("createProject.closeDialog")} onClick={onClose}><X className="size-4" aria-hidden="true" /></button>}
				{/* Mode choice as a ChoiceList: each row is the whole target. */}
				<div className="flex flex-col gap-2">
					<button data-testid="create-project-new" type="button" disabled={disabled} onClick={() => onSelect("single_repo", true)} className={choiceRowClass}>
						<span className="grid size-9 shrink-0 place-items-center rounded-sm bg-wd-inset text-wd-secondary" aria-hidden="true">
							<FolderPlus className="size-5" />
						</span>
						<span className="flex min-w-0 flex-1 flex-col gap-0.5">
							<span className="wd-body font-medium text-wd-primary">{t("createProject.startNew")}</span>
							<span className="wd-body-s text-wd-secondary">{t("createProject.startNewDescription")}</span>
						</span>
						<ChevronRight className="size-4 shrink-0 text-wd-tertiary" aria-hidden="true" />
					</button>
					<button data-testid="create-project-import-existing" type="button" disabled={disabled} onClick={() => onSelect("single_repo")} className={choiceRowClass}>
						<span className="grid size-9 shrink-0 place-items-center rounded-sm bg-wd-inset text-wd-secondary" aria-hidden="true">
							<Folder className="size-5" />
						</span>
						<span className="flex min-w-0 flex-1 flex-col gap-0.5">
							<span className="wd-body font-medium text-wd-primary">{t("createProject.importExisting")}</span>
							<span className="wd-body-s text-wd-secondary">{t("createProject.importExistingDescription")}</span>
						</span>
						<ChevronRight className="size-4 shrink-0 text-wd-tertiary" aria-hidden="true" />
					</button>
				</div>
				<details className="group wd-body-s text-wd-secondary">
					<summary className="inline-flex cursor-pointer list-none items-center gap-1.5 rounded-full px-1 hover:text-wd-primary [&::-webkit-details-marker]:hidden">
						<ChevronRight className="size-3.5 transition-transform duration-fast group-open:rotate-90" aria-hidden="true" />
						{t("createProject.multipleRepositories")}
					</summary>
					<Button data-testid="create-project-import-workspace" variant="secondary" className="mt-3" disabled={disabled} onClick={() => onSelect("workspace")}>{t("createProject.importWorkspace")}</Button>
				</details>
			</div>
		</>
	);
}

function CreateProjectFolderDialog({
	disabled,
	error,
	kind,
	onBack,
	onChooseFolder,
	onOpenChange,
	open,
	scan,
}: {
	disabled: boolean;
	error: string | null;
	kind: ProjectKind;
	onBack: () => void;
	onChooseFolder: () => void;
	onOpenChange: (open: boolean) => void;
	open: boolean;
	scan: ImportFolderScan | null;
}) {
	const { t } = useTranslation();
	const isWorkspace = kind === "workspace";
	const failedRepos = scan?.repos.filter((repo) => (repo.status === "error" || !repo.hasRemote) && !repo.needsGitInit) ?? [];
	const hasScan = scan !== null;
	const footerMessage =
		failedRepos.length > 0
			? t("createProject.footerResolve", { count: failedRepos.length })
			: hasScan
				? t("createProject.footerReview")
				: t("createProject.footerChoose");
	return (
		<Dialog.Root open={open} onOpenChange={onOpenChange}>
			<Dialog.Portal>
				<Dialog.Overlay className="dialog-overlay data-[state=open]:animate-overlay-in" />
				<Dialog.Content className="fixed left-1/2 top-1/2 z-overlay flex max-h-[min(var(--size-import-folder-dialog),calc(100svh-24px))] w-[min(var(--size-import-folder-dialog),calc(100vw-24px))] -translate-x-1/2 -translate-y-1/2 flex-col overflow-hidden rounded-xl border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-raised p-0 text-wd-primary shadow-[var(--shadow-overlay)] data-[state=open]:animate-modal-in">
					<div className="flex shrink-0 items-start gap-3 p-6 pb-4 sm:gap-4">
						<Button
							type="button"
							variant="ghost"
							size="icon"
							aria-label={t("createProject.backToType")}
							disabled={disabled}
							onClick={onBack}
						>
							<ChevronRight className="size-4 rotate-180" aria-hidden="true" />
						</Button>
						<div className="min-w-0 flex-1">
							<Dialog.Title className="wd-title text-wd-primary">
								{isWorkspace ? t("createProject.importWorkspace") : t("createProject.importProject")}
							</Dialog.Title>
							<Dialog.Description className="wd-body-s mt-1 max-w-[520px] text-wd-secondary">
								{isWorkspace ? t("createProject.importWorkspaceDesc") : t("createProject.importProjectDesc")}
							</Dialog.Description>
						</div>
						<Dialog.Close asChild>
							<button
								type="button"
								className="settings-close-button"
								aria-label={t("createProject.closeImport")}
								disabled={disabled}
							>
								<X className="size-4" aria-hidden="true" />
							</button>
						</Dialog.Close>
					</div>
					<div className="min-h-0 overflow-y-auto px-6 py-2">
						{hasScan ? (
							<div className="flex flex-col gap-4">
								<div className="flex items-center gap-3 rounded-lg border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-surface p-4">
									<span className="grid size-9 shrink-0 place-items-center rounded-sm bg-wd-inset text-wd-secondary" aria-hidden="true">
										<Folder className="size-5" />
									</span>
									<div className="min-w-0 flex-1">
										<div className="wd-body truncate font-medium text-wd-primary">
											{displayImportPath(scan.path)}
										</div>
										<div className="wd-body-s mt-0.5 text-wd-secondary">
											{isWorkspace ? t("createProject.workspaceRoot") : t("createProject.projectFolder")}
										</div>
									</div>
									<Button data-testid="create-project-choose-folder" type="button" variant="secondary" disabled={disabled} onClick={onChooseFolder}>
										{t("createProject.change")}
									</Button>
								</div>

								{error && (
									<div className="overflow-hidden rounded-lg bg-critical-bg">
										<div className="wd-label flex items-center gap-2 px-4 pt-3 text-critical">
											<XCircle className="size-4 shrink-0" aria-hidden="true" />
											{isWorkspace ? t("createProject.importFailedWorkspace") : t("createProject.importFailedProject")}
										</div>
										<div className="wd-body-s px-4 pb-3 pt-1 text-wd-primary">{error}</div>
										{failedRepos.length > 0 && (
											<div className="border-t-[length:var(--stroke-hairline)] border-wd-hairline">
												{failedRepos.map((repo) => (
													<ImportRepoRow key={repo.path} repo={repo} failed />
												))}
											</div>
										)}
									</div>
								)}

							{scan.repos
								.filter((repo) => (repo.status !== "error" && repo.hasRemote) || repo.needsGitInit)
								.map((repo) => (
										<div
											key={repo.path}
											className="rounded-lg border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-surface"
										>
											<ImportRepoRow repo={repo} />
										</div>
									))}

								{scan.repos.length === 0 && (
									<div className="wd-body-s rounded-lg border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-surface p-4 text-wd-secondary">
										{t("createProject.noRepos")}
									</div>
								)}
							</div>
						) : (
							<button
								data-testid="create-project-choose-folder"
								type="button"
								className="flex min-h-[132px] w-full flex-col items-center justify-center rounded-lg border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-surface p-6 text-center transition-colors duration-fast hover:bg-wd-hover disabled:pointer-events-none disabled:text-wd-disabled sm:min-h-[160px]"
								disabled={disabled}
								onClick={onChooseFolder}
							>
								<span className="mb-4 grid size-11 place-items-center rounded-sm bg-wd-inset text-wd-secondary">
									<FolderPlus className="size-5" aria-hidden="true" />
								</span>
								<span className="wd-body font-medium text-wd-primary">
									{isWorkspace ? t("createProject.chooseFolder") : t("createProject.chooseProjectFolder")}
								</span>
								<span className="wd-body-s mt-1 max-w-full text-pretty text-wd-secondary">
									{isWorkspace ? t("createProject.pickerWorkspaceHint") : t("createProject.pickerProjectHint")}
								</span>
							</button>
						)}
						{error && !hasScan && (
							<div
								className={cn(
									"wd-body-s mt-4 rounded-md bg-critical-bg px-4 py-3 text-critical",
								)}
							>
								{error}
							</div>
						)}
					</div>
					<div className="flex shrink-0 flex-col gap-3 p-6 pt-4 sm:flex-row sm:items-center sm:justify-between">
						<p className="wd-body-s text-wd-secondary">{footerMessage}</p>
						<div className="flex flex-wrap items-center justify-end gap-3">
							<Button type="button" variant="ghost" disabled={disabled} onClick={() => onOpenChange(false)}>
								{t("createProject.cancel")}
							</Button>
						</div>
					</div>
				</Dialog.Content>
			</Dialog.Portal>
		</Dialog.Root>
	);
}

function ImportRepoRow({ failed = false, repo }: { failed?: boolean; repo: ImportFolderScan["repos"][number] }) {
	const { t } = useTranslation();
	return (
		<div className="flex items-center gap-3 p-4">
			{failed ? (
				<XCircle className="size-5 shrink-0 text-critical" aria-hidden="true" />
			) : (
				<CheckCircle2 className="size-5 shrink-0 text-good" aria-hidden="true" />
			)}
			<div className="min-w-0 flex-1">
				<div className="wd-body truncate font-medium text-wd-primary">{repo.name}</div>
				<div className="wd-body-s mt-0.5 truncate text-wd-secondary">
					{displayImportPath(repo.path)}
				</div>
			</div>
			<div className="wd-body-s hidden max-w-[260px] shrink-0 truncate text-right text-wd-secondary sm:block">
				{repo.needsGitInit
					? t("createProject.needsGitInit")
					: failed
						? (repo.reason ?? t("createProject.repoCannotImport"))
						: `${repo.branch} ${remoteDisplay(repo.remote)}`}
			</div>
		</div>
	);
}

function displayImportPath(value: string) {
	return value.replace(/^\/Users\/[^/]+/, "~");
}

function remoteDisplay(remote: string) {
	const ssh = remote.match(/^[^@]+@([^:]+):(.+)$/);
	if (ssh?.[1] && ssh[2]) return `${ssh[1]}/${ssh[2].replace(/\.git$/, "")}`;
	try {
		const url = new URL(remote);
		return `${url.host}${url.pathname.replace(/\.git$/, "")}`;
	} catch {
		return remote.replace(/^https?:\/\//, "").replace(/\.git$/, "");
	}
}
