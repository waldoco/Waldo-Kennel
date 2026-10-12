import {
	type ClipboardEvent,
	type DragEvent,
	type FormEvent,
	type ReactNode,
	useId,
	useRef,
	useState,
} from "react";
import {
		FileTextIcon as FileText,
	PaperclipIcon as Paperclip,
	XIcon as X,
} from "./icons";

/** Mirrors the daemon's AgentInfo DTO; keep in sync when the API schema changes. */
export type TaskComposerAgentOption = {
	authStatus?: "authorized" | "unauthorized" | "unknown";
	id: string;
	label: string;
	ready?: boolean | null;
	readyDetail?: string;
	requiresProfile?: boolean;
	roles: { coordinator: boolean; switchTarget: boolean; worker: boolean };
};

export type TaskComposerAgentControl = {
	authorized?: TaskComposerAgentOption[];
	disabled: boolean;
	id: string;
	installed?: TaskComposerAgentOption[];
	label: string;
	onChange: (value: string) => void;
	placeholder: string;
	supported?: TaskComposerAgentOption[];
	value: string;
};

export type TaskComposerModelOption = {
	id: string;
	isDefault?: boolean;
	label: string;
	provider?: string;
};

export type TaskComposerModelCatalog = {
	allowCustom: boolean;
	models: TaskComposerModelOption[];
	selectionMode: "catalog" | "text" | "mode";
};

export type TaskComposerModelControl = {
	agentId: string;
	agentLabel: string;
	catalog?: TaskComposerModelCatalog;
	fetching: boolean;
	id: string;
	loading: boolean;
	mode: string;
	onModeChange: (value: string) => void;
	onModelChange: (value: string) => void;
	onRefresh?: () => void;
	projectId: string;
	refreshing: boolean;
	value: string;
};

export type TaskComposerAttachment = {
	id: string;
	name: string;
	previewUrl?: string;
};

export type TaskComposerAttachments = {
	error?: string | null;
	items: TaskComposerAttachment[];
	onAddFiles: (files: File[]) => void;
	onRemove: (id: string) => void;
};

export type TaskComposerSubmission = {
	canCreateAsTui: boolean;
	error?: string;
	isSubmitting: boolean;
	modelWarning?: string;
	onSubmit: () => void;
	onSubmitAsTui: () => void;
};

export type TaskComposerLabels = {
	addFile: string;
	createAsTui: string;
	removeFile: (name: string) => string;
	runsWith: string;
	start: string;
	starting: string;
	task: string;
	taskPlaceholder: string;
};

export type TaskComposerViewProps = {
	agent: Omit<TaskComposerAgentControl, "id">;
	attachments: TaskComposerAttachments;
	autoFocusPrompt?: boolean;
	canSubmit: boolean;
	labels: TaskComposerLabels;
	model: Omit<TaskComposerModelControl, "id">;
	onPromptChange: (value: string) => void;
	prompt: string;
	/** Inline setup guidance shown under the run controls, e.g. a missing launch profile. */
	profileHint?: string;
	renderAgentControl: (control: TaskComposerAgentControl) => ReactNode;
	renderModelControl: (control: TaskComposerModelControl) => ReactNode;
	submission: TaskComposerSubmission;
};

export function TaskComposerView({
	agent,
	attachments,
	autoFocusPrompt,
	canSubmit,
	labels,
	model,
	onPromptChange,
	prompt,
	profileHint,
	renderAgentControl,
	renderModelControl,
	submission,
}: TaskComposerViewProps) {
	const promptId = useId();
	const modelId = useId();
	const agentId = useId();
	const fileInputRef = useRef<HTMLInputElement>(null);
	const [isDragging, setIsDragging] = useState(false);

	const submit = (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		submission.onSubmit();
	};

	const handlePaste = (event: ClipboardEvent<HTMLTextAreaElement>) => {
		const files = Array.from(event.clipboardData?.files ?? []);
		if (files.length === 0) return;
		event.preventDefault();
		attachments.onAddFiles(files);
	};

	const handleDrop = (event: DragEvent<HTMLFormElement>) => {
		event.preventDefault();
		setIsDragging(false);
		const files = Array.from(event.dataTransfer?.files ?? []);
		if (files.length > 0) attachments.onAddFiles(files);
	};

	const handleDragOver = (event: DragEvent<HTMLFormElement>) => {
		if (Array.from(event.dataTransfer?.items ?? []).some((item) => item.kind === "file")) {
			event.preventDefault();
			setIsDragging(true);
		}
	};

	return (
		<form
			onSubmit={submit}
			className="composer-prompt-surface flex flex-col gap-3 transition-[background-color] duration-fast motion-reduce:transition-none"
			data-dragging={isDragging || undefined}
			onDrop={handleDrop}
			onDragOver={handleDragOver}
			onDragLeave={(event) => {
				const nextTarget = event.relatedTarget;
				if (!(nextTarget instanceof Node) || !event.currentTarget.contains(nextTarget)) setIsDragging(false);
			}}
		>
			<label className="sr-only" htmlFor={promptId}>
				{labels.task}
			</label>
			<textarea
				id={promptId}
				autoFocus={autoFocusPrompt}
				className="mx-3 mt-3 min-h-(--size-composer-prompt-min) resize-none rounded-md hairline border-wd-hairline bg-wd-inset px-4 py-3 text-md leading-relaxed text-wd-primary placeholder:text-wd-tertiary"
				placeholder={labels.taskPlaceholder}
				value={prompt}
				onChange={(event) => onPromptChange(event.target.value)}
				onPaste={handlePaste}
				onKeyDown={(event) => {
					if (event.key === "Enter" && !event.shiftKey && !event.altKey && !event.nativeEvent.isComposing) {
						event.preventDefault();
						event.currentTarget.form?.requestSubmit();
					}
				}}
			/>

			{attachments.items.length > 0 && (
				<ul className="scrollbar-none flex max-h-24 flex-wrap gap-2 overflow-y-auto px-3">
					{attachments.items.map((attachment) => (
						<li
							key={attachment.id}
							className="flex min-w-0 max-w-48 items-center gap-2 rounded-sm bg-wd-inset px-1.5 py-1 text-xs text-wd-primary"
						>
							{attachment.previewUrl ? (
								<img src={attachment.previewUrl} alt="" className="size-7 shrink-0 rounded-xs object-cover" />
							) : (
								<FileText
									className="size-7 shrink-0 rounded-xs bg-wd-surface p-1.5 text-wd-secondary"
									aria-hidden="true"
								/>
							)}
							<span className="min-w-0 flex-1 truncate font-medium">{attachment.name}</span>
							<button
								type="button"
								className="grid size-5 shrink-0 place-items-center rounded-full text-wd-secondary transition-colors duration-fast motion-reduce:transition-none hover:bg-wd-hover hover:text-wd-primary"
								aria-label={labels.removeFile(attachment.name)}
								onClick={() => attachments.onRemove(attachment.id)}
							>
								<X className="size-icon-sm" aria-hidden="true" />
							</button>
						</li>
					))}
				</ul>
			)}
			<input
				ref={fileInputRef}
				type="file"
				multiple
				className="hidden"
				onChange={(event) => {
					if (event.target.files) attachments.onAddFiles(Array.from(event.target.files));
					event.target.value = "";
				}}
			/>
			{attachments.error && (
				<p className="px-4 text-caption text-critical" role="alert">{attachments.error}</p>
			)}

			{(submission.error || submission.modelWarning) && (
				<div className="px-3">
					{submission.error && (
						<div
							className="flex items-center justify-between gap-3 rounded-sm bg-critical-bg px-3 py-2 text-xs text-critical"
							role="alert"
						>
							<span>{submission.error}</span>
							{submission.canCreateAsTui ? (
								<button
									type="button"
									disabled={submission.isSubmitting}
									onClick={submission.onSubmitAsTui}
									className="inline-flex h-control-md shrink-0 items-center justify-center rounded-full bg-wd-inverse px-3 text-xs font-medium text-wd-on-inverse transition-opacity duration-fast motion-reduce:transition-none hover:opacity-90 disabled:pointer-events-none disabled:opacity-40"
								>
									{labels.createAsTui}
								</button>
							) : null}
						</div>
					)}
					{!submission.error && submission.modelWarning && (
						<p className="text-caption text-caution" role="status">{submission.modelWarning}</p>
					)}
				</div>
			)}

			<div className="composer-toolbar">
				<div className="composer-run-controls" role="group" aria-label={labels.runsWith}>
					<div className="composer-toolbar-slot">
						{renderAgentControl({ ...agent, id: agentId })}
					</div>
					<span className="composer-toolbar-divider" aria-hidden="true" />
					<div className="composer-toolbar-slot">
						{renderModelControl({ ...model, id: modelId })}
					</div>
				</div>
				<button
					type="button"
					className="grid size-(--size-settings-action-height) place-items-center rounded-full text-wd-secondary transition-colors duration-fast motion-reduce:transition-none hover:bg-wd-hover hover:text-wd-primary"
					aria-label={labels.addFile}
					onClick={() => fileInputRef.current?.click()}
				>
					<Paperclip className="size-icon-base" aria-hidden="true" />
				</button>
				<button
					type="submit"
					disabled={submission.isSubmitting || !canSubmit}
					className="inline-flex h-(--size-settings-action-height) min-w-(--size-composer-start-button) shrink-0 items-center justify-center gap-1.5 rounded-full bg-wd-inverse px-3.5 text-xs font-medium text-wd-on-inverse transition-opacity duration-fast motion-reduce:transition-none hover:opacity-90 disabled:pointer-events-none disabled:opacity-40"
				>
					{submission.isSubmitting ? labels.starting : labels.start}
					{!submission.isSubmitting && (
						<kbd className="composer-keycap" aria-hidden="true">
							↵
						</kbd>
					)}
				</button>
			</div>
			{profileHint && (
				<p className="px-3 pb-3 text-caption text-caution" role="status">{profileHint}</p>
			)}
		</form>
	);
}
