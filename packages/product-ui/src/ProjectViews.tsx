import {
	type ComponentType,
	type FormEvent,
	type HTMLAttributes,
	type KeyboardEvent,
	type ReactNode,
	useEffect,
	useRef,
	useState,
} from "react";
import { cn } from "./utils";
import type {
	ProjectKind,
	ProjectRepositorySummary,
} from "./project-models";

type ProjectExternalLink = ComponentType<{
	children: ReactNode;
	className?: string;
	href: string;
	title?: string;
}>;

export type ProjectModePickerLabels = {
	title: string;
	description: string;
	workspace: string;
	workspaceDescription: string;
	project: string;
	projectDescription: string;
	close: string;
	workspaceExample: string;
	workspaceRepositories: [string, string, string];
	projectExample: string;
	projectBranchExample: string;
};

export type ProjectModePickerViewProps = {
	closeIcon?: ReactNode;
	dialog?: boolean;
	disabled: boolean;
	folderIcon?: ReactNode;
	labels: ProjectModePickerLabels;
	onClose?: () => void;
	onSelect: (kind: Exclude<ProjectKind, "scratch">) => void;
};

export function ProjectModePickerView({
	closeIcon,
	dialog = false,
	disabled,
	folderIcon,
	labels,
	onClose,
	onSelect,
}: ProjectModePickerViewProps) {
	return (
		<div
			className={cn(
				"relative isolate flex w-full max-w-(--size-import-modal-max) flex-col items-stretch gap-8 rounded-xl hairline border-wd-hairline bg-wd-surface p-(--size-import-modal-padding)",
				// Only an overlay earns a shadow; inline the picker sits flat on the page.
				dialog && "shadow-[var(--shadow-import-modal)]",
			)}
			role={dialog ? undefined : "group"}
			aria-label={dialog ? undefined : labels.title}
		>
			<div className={cn("relative z-[1] flex flex-col items-start gap-1", onClose && "pr-10")}>
				<h2 className="import-title">{labels.title}</h2>
				<p className="import-description">{labels.description}</p>
			</div>
			<div className="relative z-[2] flex flex-row items-stretch justify-center gap-6 self-stretch">
				<ProjectModeButton
					description={labels.workspaceDescription}
					disabled={disabled}
					folderIcon={folderIcon}
					kind="workspace"
					labels={labels}
					onClick={() => onSelect("workspace")}
				/>
				<ProjectModeButton
					description={labels.projectDescription}
					disabled={disabled}
					kind="single_repo"
					labels={labels}
					onClick={() => onSelect("single_repo")}
				/>
			</div>
			{onClose && (
				<button
					type="button"
					className="import-close-button"
					aria-label={labels.close}
					disabled={disabled}
					onClick={onClose}
				>
					{closeIcon}
				</button>
			)}
		</div>
	);
}

function ProjectModeButton({
	description,
	disabled,
	folderIcon,
	kind,
	labels,
	onClick,
}: {
	description: string;
	disabled: boolean;
	folderIcon?: ReactNode;
	kind: Exclude<ProjectKind, "scratch">;
	labels: ProjectModePickerLabels;
	onClick: () => void;
}) {
	const isWorkspace = kind === "workspace";
	const title = isWorkspace ? labels.workspace : labels.project;
	return (
		<button
			type="button"
			aria-label={title}
			className="flex min-h-(--size-import-mode-card-min) w-full flex-1 flex-col justify-start gap-6 self-stretch rounded-lg hairline border-wd-hairline bg-wd-surface p-6 text-left transition-colors duration-fast motion-reduce:transition-none hover:bg-wd-hover disabled:pointer-events-none disabled:opacity-40 sm:min-h-(--size-import-mode-card-min-sm)"
			disabled={disabled}
			onClick={onClick}
		>
			<span className="flex w-full flex-col items-start">
				<span
					className={cn(
						"flex h-(--size-import-mode-illustration) w-full justify-center",
						isWorkspace ? "items-start" : "items-center",
					)}
				>
					{isWorkspace ? (
						<span className="flex h-(--size-import-mode-illustration) w-full max-w-[240px] flex-col items-start gap-3 rounded-md bg-wd-inset p-4">
							<span className="flex items-center gap-2 text-sm leading-5 text-wd-secondary">
								{folderIcon}
								{labels.workspaceExample}
							</span>
							<span className="flex w-full flex-col items-start gap-2">
								{labels.workspaceRepositories.map((repo) => (
									<span key={repo} className="flex w-full items-center px-3 py-2">
										<span className="mr-2 size-2 shrink-0 rounded-full bg-wd-strong" aria-hidden="true" />
										<span className="text-xs font-medium leading-4 text-wd-primary">
											{repo}
										</span>
									</span>
								))}
							</span>
						</span>
					) : (
						<span className="flex h-[50px] w-fit items-center rounded-md bg-wd-inset px-4 py-3">
							<span className="mr-2 size-2 shrink-0 rounded-full bg-wd-strong" aria-hidden="true" />
							<span className="text-sm font-medium leading-5 text-wd-primary">
								{labels.projectExample}
							</span>
							<span className="px-1 text-base leading-6 text-wd-tertiary" aria-hidden="true">
								·
							</span>
							<span className="text-sm font-normal leading-5 text-wd-secondary">
								{labels.projectBranchExample}
							</span>
						</span>
					)}
				</span>
			</span>
			<span className="mt-auto flex w-full flex-col items-start gap-2">
				<span className="text-base font-medium leading-6 text-wd-primary">
					{title}
				</span>
				<span className="text-sm font-normal leading-5 text-wd-secondary">
					{description}
				</span>
			</span>
		</button>
	);
}

export type ProjectSetupHeaderTextProps = {
	children: ReactNode;
	className?: string;
};

export function ProjectSetupHeaderView({
	CloseButton,
	Description,
	Title,
	closeIcon,
	closeLabel,
	disabled,
	path,
	title,
}: {
	CloseButton: ComponentType<{ "aria-label": string; children: ReactNode; disabled: boolean }>;
	Description: ComponentType<ProjectSetupHeaderTextProps>;
	Title: ComponentType<ProjectSetupHeaderTextProps>;
	closeIcon: ReactNode;
	closeLabel: string;
	disabled: boolean;
	path: string;
	title: string;
}) {
	return (
		<div className="flex items-start justify-between gap-4 hairline-b border-wd-hairline p-(--size-import-dialog-padding)">
			<div className="min-w-0">
				<Title className="text-subtitle font-medium text-wd-primary">
					{title}
				</Title>
				<Description className="mt-1 break-all text-xs text-wd-secondary">
					{path}
				</Description>
			</div>
			<CloseButton aria-label={closeLabel} disabled={disabled}>
				{closeIcon}
			</CloseButton>
		</div>
	);
}

export type ProjectSetupAlert = {
	icon?: ReactNode;
	message: string;
	title: string;
	tone: "warning" | "error";
};

export function ProjectSetupFormView({
	agentControls,
	agents,
	alert,
	canSubmit,
	intakeControl,
	isBusy,
	onCancel,
	onSubmit,
	setupNotice,
	submitLabel,
	cancelLabel,
}: {
	agentControls: { worker: ReactNode; orchestrator: ReactNode };
	agents: {
		cacheMessage: string;
		error?: string | null;
		loading: boolean;
		loadingMessage: string;
		onRefresh: () => void;
		refreshLabel: string;
		refreshing: boolean;
		retryLabel: string;
	};
	alert?: ProjectSetupAlert | null;
	canSubmit: boolean;
	intakeControl: ReactNode;
	isBusy: boolean;
	onCancel: () => void;
	onSubmit: () => void;
	setupNotice?: { message: string; warning?: string | null } | null;
	submitLabel: string;
	cancelLabel: string;
}) {
	const submit = (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		if (canSubmit) onSubmit();
	};
	return (
		<form className="space-y-5 p-(--size-import-dialog-padding)" onSubmit={submit}>
			<div className="grid gap-4 sm:grid-cols-2">
				{agentControls.worker}
				{agentControls.orchestrator}
			</div>

			{agents.loading && (
				<p className="text-xs leading-row text-wd-secondary" role="status">
					{agents.loadingMessage}
				</p>
			)}

			<div className="flex items-center justify-between gap-3 text-xs leading-row text-wd-secondary">
				<span>{agents.cacheMessage}</span>
				<button
					type="button"
					className="shrink-0 rounded-xs font-medium text-wd-primary underline-offset-2 hover:underline disabled:pointer-events-none disabled:opacity-40"
					disabled={agents.refreshing}
					onClick={agents.onRefresh}
				>
					{agents.refreshLabel}
				</button>
			</div>

			{agents.error && (
				<div
					className="flex items-center justify-between gap-3 rounded-md bg-critical-bg px-3 py-2 text-xs leading-row text-critical"
					role="alert"
				>
					<span>{agents.error}</span>
					<button
						type="button"
						className="shrink-0 rounded-xs font-medium text-wd-primary underline-offset-2 hover:underline disabled:pointer-events-none disabled:opacity-40"
						disabled={agents.refreshing}
						onClick={agents.onRefresh}
					>
						{agents.retryLabel}
					</button>
				</div>
			)}

			<div className="hairline-t border-wd-hairline pt-5">{intakeControl}</div>

			{setupNotice && (
				<div className="rounded-md bg-wd-inset px-3 py-2.5 text-xs leading-body-md text-wd-secondary">
						<p>{setupNotice.message}</p>
						{setupNotice.warning && <p className="mt-2 text-caution">{setupNotice.warning}</p>}
				</div>
			)}

			{alert && (
				<div
					role="alert"
					className={
						alert.tone === "warning"
							? "flex gap-2 rounded-md bg-caution-bg px-3 py-2.5 text-xs leading-body-md text-caution"
								: "flex gap-2 rounded-md bg-critical-bg px-3 py-2.5 text-xs leading-body-md text-critical"
					}
				>
					{alert.icon}
					<div className="min-w-0 space-y-0.5">
						<p
							className={
								alert.tone === "warning"
									? "font-medium text-wd-primary"
									: "font-medium text-critical"
							}
						>
							{alert.title}
						</p>
						<p className="text-wd-secondary">{alert.message}</p>
					</div>
				</div>
			)}

			<div className="flex items-center justify-end gap-3 pt-1">
				<button
					className="settings-footer-button disabled:pointer-events-none disabled:opacity-40"
					type="button"
					disabled={isBusy}
					onClick={onCancel}
				>
					{cancelLabel}
				</button>
				<button
					className="settings-footer-button settings-footer-button-primary disabled:pointer-events-none disabled:opacity-40"
					type="submit"
					disabled={!canSubmit}
				>
					{submitLabel}
				</button>
			</div>
		</form>
	);
}

export function ProjectSettingsSection({
	children,
	grouped,
	title,
	titleHidden,
}: {
	children: ReactNode;
	grouped?: boolean;
	title: string;
	titleHidden?: boolean;
}) {
	return (
		<section className="flex w-full flex-col items-stretch gap-(--size-settings-section-inner-gap)">
			{!titleHidden && (
				<h2 className="px-3 text-xs font-medium leading-4 text-wd-secondary">
					{title}
				</h2>
			)}
			<div
				className={cn(
					"w-full",
					grouped
						? "settings-grouped-rows flex w-full flex-col"
						: "flex w-full flex-col gap-1.5",
				)}
			>
				{children}
			</div>
		</section>
	);
}

export function ProjectSettingsRow({
	children,
	className,
	icon,
	label,
}: {
	children: ReactNode;
	className?: string;
	icon?: ReactNode;
	label: string;
}) {
	return (
		<div className={cn("settings-row-bar", className)}>
			<div className="flex shrink-0 items-center gap-(--size-settings-row-icon-gap)">
				{icon}
				<span className="whitespace-nowrap text-sm leading-5 text-wd-primary">{label}</span>
			</div>
			<div className="flex min-w-0 flex-1 items-center justify-end">{children}</div>
		</div>
	);
}

export function ProjectSettingsInputRow({
	editIcon,
	editLabel,
	icon,
	id,
	label,
	onChange,
	placeholder,
	value,
}: {
	editIcon?: ReactNode;
	editLabel: string;
	icon?: ReactNode;
	id: string;
	label: string;
	onChange: (value: string) => void;
	placeholder?: string;
	value: string;
}) {
	const [editing, setEditing] = useState(false);
	const inputRef = useRef<HTMLInputElement>(null);

	useEffect(() => {
		if (!editing) return;
		inputRef.current?.focus();
		inputRef.current?.select();
	}, [editing]);

	const finishEditing = () => setEditing(false);
	const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
		if (event.key === "Enter") {
			event.preventDefault();
			finishEditing();
		} else if (event.key === "Escape") {
			event.preventDefault();
			event.stopPropagation();
			finishEditing();
		}
	};

	return (
		<ProjectSettingsRow icon={icon} label={label}>
			{editing ? (
				<input
					ref={inputRef}
					id={id}
					aria-label={label}
					className="settings-inline-edit-input"
					value={value}
					onChange={(event) => onChange(event.target.value)}
					onBlur={finishEditing}
					onKeyDown={onKeyDown}
					placeholder={placeholder}
				/>
			) : (
				<button
					type="button"
					className="settings-inline-edit-trigger"
					aria-label={editLabel}
					onClick={() => setEditing(true)}
				>
					<span className="settings-row-value" title={value || placeholder}>
						{value || placeholder}
					</span>
					{editIcon}
				</button>
			)}
		</ProjectSettingsRow>
	);
}

export function ProjectSettingsValueRow({
	externalLink: ExternalLink,
	href,
	icon,
	label,
	value,
}: {
	externalLink?: ProjectExternalLink;
	href?: string;
	icon?: ReactNode;
	label: string;
	value: string;
}) {
	return (
		<ProjectSettingsRow icon={icon} label={label}>
			{href && ExternalLink ? (
				<ExternalLink
					href={href}
					className="settings-row-value text-wd-primary underline-offset-2 hover:underline"
					title={value}
				>
					{value}
				</ExternalLink>
			) : (
				<span className="settings-row-value" title={value}>
					{value}
				</span>
			)}
		</ProjectSettingsRow>
	);
}

export function ProjectGeneralSettingsView({
	displayName,
	externalLink,
	icons,
	labels,
	onDisplayNameChange,
	project,
}: {
	displayName: string;
	externalLink?: ProjectExternalLink;
	icons?: Partial<Record<"edit" | "name" | "id" | "kind" | "path" | "repo" | "workspaceRepo", ReactNode>>;
	labels: {
		title: string;
		name: string;
		id: string;
		kind: string;
		path: string;
		repo: string;
		workspaceRepos: string;
		workspaceReposEmpty: string;
		editName: string;
	};
	onDisplayNameChange: (value: string) => void;
	project: {
		id: string;
		kindLabel: string;
		path: string;
		pathHref?: string;
		repo: string;
		repoHref?: string;
		workspaceRepos?: ProjectRepositorySummary[];
	};
}) {
	return (
		<>
			<ProjectSettingsSection title={labels.title} titleHidden grouped>
				<ProjectSettingsInputRow
					editIcon={icons?.edit}
					editLabel={labels.editName}
					icon={icons?.name}
					label={labels.name}
					id="projectName"
					value={displayName}
					onChange={onDisplayNameChange}
				/>
				<ProjectSettingsValueRow icon={icons?.id} label={labels.id} value={project.id} />
				<ProjectSettingsValueRow icon={icons?.kind} label={labels.kind} value={project.kindLabel} />
				<ProjectSettingsValueRow externalLink={externalLink} href={project.pathHref} icon={icons?.path} label={labels.path} value={project.path} />
				<ProjectSettingsValueRow externalLink={externalLink} href={project.repoHref} icon={icons?.repo} label={labels.repo} value={project.repo || "—"} />
			</ProjectSettingsSection>
			{project.workspaceRepos && (
				<ProjectSettingsSection title={labels.workspaceRepos} grouped>
					{project.workspaceRepos.length > 0 ? (
						project.workspaceRepos.map((repo) => (
							<ProjectSettingsRow key={repo.name} icon={icons?.workspaceRepo} label={repo.name}>
								<span className="settings-row-value">
									{repo.relativePath}
									{repo.repo ? ` · ${repo.repo}` : ""}
								</span>
							</ProjectSettingsRow>
						))
					) : (
						<p className="px-1 text-xs text-wd-secondary">{labels.workspaceReposEmpty}</p>
					)}
				</ProjectSettingsSection>
			)}
		</>
	);
}

export function ProjectAgentsSettingsView({
	error,
	missingRequiredMessage,
	orchestratorArea,
	orchestratorModelArea,
	permissions,
	refresh,
	title,
	workerArea,
	workerModelArea,
}: {
	error?: string | null;
	missingRequiredMessage?: string | null;
	orchestratorArea: ReactNode;
	orchestratorModelArea: ReactNode;
	permissions: { control: ReactNode; icon?: ReactNode; label: string };
	refresh: {
		actionIcon?: ReactNode;
		disabled: boolean;
		label: string;
		onClick: () => void;
		rowIcon?: ReactNode;
		value: string;
	};
	title: string;
	workerArea: ReactNode;
	workerModelArea: ReactNode;
}) {
	return (
		<ProjectSettingsSection title={title} titleHidden grouped>
			{workerArea}
			{workerModelArea}
			{orchestratorArea}
			{orchestratorModelArea}
			<ProjectSettingsRow icon={permissions.icon} label={permissions.label}>
				{permissions.control}
			</ProjectSettingsRow>
			<ProjectSettingsRow icon={refresh.rowIcon} label={refresh.label}>
				<button
					type="button"
					aria-label={refresh.label}
					className="settings-option-trigger inline-flex items-center gap-1.5 disabled:pointer-events-none disabled:opacity-40"
					disabled={refresh.disabled}
					onClick={refresh.onClick}
				>
					{refresh.actionIcon}
					{refresh.value}
				</button>
			</ProjectSettingsRow>
			{error && <p className="px-1 text-xs leading-row text-critical" role="alert">{error}</p>}
			{missingRequiredMessage && (
				<p className="px-1 text-xs leading-row text-critical" role="alert">{missingRequiredMessage}</p>
			)}
		</ProjectSettingsSection>
	);
}

export function ProjectWorkflowSettingsView({
	branch,
	icons,
	labels,
	onBranchChange,
	onPrefixChange,
	prefix,
	reviewerControl,
	reviewerWarning,
}: {
	branch: string;
	icons?: Partial<Record<"branch" | "edit" | "prefix" | "reviewer", ReactNode>>;
	labels: {
		worktrees: string;
		defaultBranch: string;
		sessionPrefix: string;
		reviewers: string;
		defaultReviewer: string;
		editDefaultBranch: string;
		editSessionPrefix: string;
	};
	onBranchChange: (value: string) => void;
	onPrefixChange: (value: string) => void;
	prefix: string;
	reviewerControl: ReactNode;
	reviewerWarning?: string | null;
}) {
	return (
		<>
			<ProjectSettingsSection title={labels.worktrees} grouped>
				<ProjectSettingsInputRow
					editIcon={icons?.edit}
					editLabel={labels.editDefaultBranch}
					icon={icons?.branch}
					label={labels.defaultBranch}
					id="defaultBranch"
					value={branch}
					placeholder="auto"
					onChange={onBranchChange}
				/>
				<ProjectSettingsInputRow
					editIcon={icons?.edit}
					editLabel={labels.editSessionPrefix}
					icon={icons?.prefix}
					label={labels.sessionPrefix}
					id="sessionPrefix"
					value={prefix}
					placeholder="kennel"
					onChange={onPrefixChange}
				/>
			</ProjectSettingsSection>
			<ProjectSettingsSection title={labels.reviewers} grouped>
				<ProjectSettingsRow icon={icons?.reviewer} label={labels.defaultReviewer}>
					{reviewerControl}
				</ProjectSettingsRow>
				{reviewerWarning && (
					<p className="px-1 text-xs leading-row text-caution" role="status">
						{reviewerWarning}
					</p>
				)}
			</ProjectSettingsSection>
		</>
	);
}

export type ProjectSettingsFormProps = Omit<HTMLAttributes<HTMLFormElement>, "onSubmit"> & {
	onSubmit: () => void;
};

export function ProjectSettingsFormView({
	children,
	className,
	onSubmit,
	...props
}: ProjectSettingsFormProps) {
	return (
		<form
			{...props}
			className={cn("flex w-full flex-col gap-(--size-settings-section-gap)", className)}
			onSubmit={(event) => {
				event.preventDefault();
				onSubmit();
			}}
		>
			{children}
		</form>
	);
}
