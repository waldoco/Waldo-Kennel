import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	ProjectSetupFormView,
	ProjectSetupHeaderView,
} from "@pin4sf/kennel-product-ui";
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "./ui/accordion";
import { useTranslation } from "react-i18next";
import * as Dialog from "@radix-ui/react-dialog";
import { TriangleAlert, X, type LucideIcon } from "lucide-react";
import { memo, useEffect, useMemo, useState, type ReactNode } from "react";
import type { components } from "../../api/schema";
import { agentsQueryKey, agentsQueryOptions, refreshAgents } from "../hooks/useAgentsQuery";
import {
	admitsRole,
	buildRankedAgentOptions,
	CORE_PROVIDER_IDS,
	singleReadyProvider,
	type RankedAgentOption,
} from "../lib/agent-select-options";
import { agentLabel } from "../lib/agent-options";
import { cn } from "../lib/utils";
import { AgentAvatar } from "./AgentAvatar";
import { AgentModelPicker } from "./AgentModelPicker";
import { FieldDefaultHint } from "./FieldDefaultHint";
import { buildIntake, type IntakeForm, IntakeFields, intakeNeedsRule } from "./IntakeFields";
import { AgentSelectMenuItem } from "./settings/AgentSelectMenuItem";
import { SettingsRow } from "./settings/SettingsRow";
import { SettingsOptionMenu } from "./settings/SettingsOptionMenu";
import type { ProjectKind } from "../types/workspace";
import { Label } from "./ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./ui/select";
import { appI18n } from "../i18n";
import { useUiStore } from "../stores/ui-store";

type TrackerIntakeConfig = components["schemas"]["TrackerIntakeConfig"];
type AgentInfo = components["schemas"]["AgentInfo"];

export type CreateProjectAgentSelection = {
	// Empty is an explicit "configure later" choice. Project registration is
	// independent of provider installation/auth/profile readiness.
	workerAgent: string;
	// Optional preferences are scoped to their selected provider. Empty means
	// let the provider/Kennel choose; it is never a hidden provider or model lock.
	workerModel?: string;
	workerMode?: string;
	// Optional explicit coordinator override. Empty means no coordinator is
	// configured; the worker is never promoted into this role by omission.
	orchestratorAgent?: string;
	orchestratorModel?: string;
	orchestratorMode?: string;
	trackerIntake?: TrackerIntakeConfig;
};

const EMPTY_INTAKE: IntakeForm = { enabled: false, repo: "", assignee: "" };
const CORE_FALLBACK_AGENTS: AgentInfo[] = CORE_PROVIDER_IDS.map((id) => ({
	id,
	label: agentLabel(id),
	roles: {
		worker: true,
		coordinator: id === "codex" || id === "claude-code" || id === "opencode",
		switchTarget: id === "codex" || id === "claude-code" || id === "opencode",
	},
}));

type CreateProjectAgentSheetProps = {
	error?: string | null;
	isCreating: boolean;
	isInitializing?: boolean;
	kind: ProjectKind;
	onOpenChange: (open: boolean) => void;
	onSubmit: (selection: CreateProjectAgentSelection) => Promise<void>;
	open: boolean;
	path: string | null;
	repositorySetupNeeded?: boolean;
	repositorySetupWarning?: string | null;
};

type SheetError = {
	title: string;
	message: string;
	tone: "warning" | "error";
};

function projectSheetError(error: string): SheetError {
	const setupMessage = error.replace(/^Setup failed:\s*/i, "").trim();
	const codeMatch = setupMessage.match(/\(([A-Z0-9_]+)\)\s*$/);
	const code = codeMatch?.[1];
	const message = codeMatch ? setupMessage.slice(0, codeMatch.index).trim() : setupMessage;

	switch (code) {
		case "PROJECT_PATH_NOT_REPO_ROOT":
			return {
				title: appI18n.t("createProject.error.notRepoRootTitle"),
				message: appI18n.t("createProject.error.notRepoRootBody"),
				tone: "warning",
			};
		case "PROJECT_BARE_REPOSITORY":
			return {
				title: appI18n.t("createProject.error.bareTitle"),
				message: appI18n.t("createProject.error.bareBody"),
				tone: "warning",
			};
		case "UNSUPPORTED_GIT_REPO":
			return {
				title: appI18n.t("createProject.error.unsupportedTitle"),
				message: appI18n.t("createProject.error.unsupportedBody"),
				tone: "warning",
			};
		default:
			return {
				title: error.toLowerCase().startsWith("setup failed:")
					? appI18n.t("createProject.error.setupFailedTitle")
					: appI18n.t("createProject.error.createFailedTitle"),
				message: message || appI18n.t("createProject.error.tryAgain"),
				tone: "error",
			};
	}
}

export function CreateProjectAgentSheet({
	error,
	isCreating,
	isInitializing = false,
	kind,
	onOpenChange,
	onSubmit,
	open,
	path,
	repositorySetupNeeded = false,
	repositorySetupWarning = null,
}: CreateProjectAgentSheetProps) {
	const { t } = useTranslation();
	const queryClient = useQueryClient();
	const agentsQuery = useQuery({
		...agentsQueryOptions,
		enabled: open,
	});
	const refreshAgentsMutation = useMutation({
		mutationFn: refreshAgents,
		onSuccess: (next) => queryClient.setQueryData(agentsQueryKey, next),
	});
	const agents = agentsQuery.data;
	const installedAgents = agents?.installed ?? [];
	const authorizedAgents = agents?.authorized ?? [];
	const supportedAgents = agents?.supported ?? CORE_FALLBACK_AGENTS;
	const preferredAgentId = useUiStore((state) => state.defaultAgentId);
	const workerOptions = useMemo(
		() =>
			buildRankedAgentOptions({
				supported: supportedAgents,
				installed: installedAgents,
				authorized: authorizedAgents,
				fallbackAgents: CORE_FALLBACK_AGENTS,
				filter: (candidate) => admitsRole(candidate, "worker"),
			}),
		[authorizedAgents, installedAgents, supportedAgents],
	);
	const coordinatorCapable = useMemo(
		() => new Set(supportedAgents.filter((candidate) => admitsRole(candidate, "coordinator")).map((candidate) => candidate.id)),
		[supportedAgents],
	);
	const isLoadingAgents = agents === undefined && agentsQuery.isFetching;
	const agentsError = agentsQuery.isError
		? agentsQuery.error instanceof Error
			? agentsQuery.error.message
			: t("createProject.couldNotLoadAgents")
		: null;
	const displayError = refreshAgentsMutation.isError
		? refreshAgentsMutation.error instanceof Error
			? refreshAgentsMutation.error.message
			: t("createProject.couldNotRefreshAgents")
		: agentsError;
	const [workerAgent, setWorkerAgent] = useState("");
	const [workerModel, setWorkerModel] = useState("");
	const [workerMode, setWorkerMode] = useState("");
	const [workerModelWarning, setWorkerModelWarning] = useState<string | undefined>();
	const [orchestratorAgent, setOrchestratorAgent] = useState("");
	const [orchestratorModel, setOrchestratorModel] = useState("");
	const [orchestratorMode, setOrchestratorMode] = useState("");
	const [orchestratorModelWarning, setOrchestratorModelWarning] = useState<string | undefined>();
	const [workerAgentTouched, setWorkerAgentTouched] = useState(false);
	const isBusy = isCreating || isInitializing;
	const [intake, setIntake] = useState<IntakeForm>(EMPTY_INTAKE);
	const intakeIncomplete = intakeNeedsRule(intake);
	// Provider and model inventory/readiness must never block durable Project
	// creation. Empty provider/model values mean configure or route later.
	const canSubmit = !intakeIncomplete && !isBusy;
	const sheetError = error ? projectSheetError(error) : null;
	const providerSetupNeeded =
		agents !== undefined && !isLoadingAgents && workerOptions.length > 0 && workerOptions.every((option) => option.disabled);
	const providerSetupAlert: SheetError | null = providerSetupNeeded
		? {
				title: t("createProject.providerSetupRequiredTitle", { defaultValue: "Agent setup required" }),
				message: t("createProject.providerSetupRequiredBody", {
					defaultValue:
						"No coding provider is ready yet. Finish authentication or configuration in the provider CLI, then refresh agents. You can still create this project now and configure the provider later.",
				}),
				tone: "warning",
			}
		: null;
	const effectiveAlert = sheetError ?? providerSetupAlert;

	useEffect(() => {
		if (!open || workerAgentTouched) return;
		const nextWorker = preferredDefaultAgent(workerOptions, preferredAgentId);
		if (workerAgent === nextWorker) return;
		setWorkerAgent(nextWorker);
		setWorkerModel("");
		setWorkerMode("");
		setWorkerModelWarning(undefined);
	}, [open, preferredAgentId, workerAgent, workerAgentTouched, workerOptions]);

	useEffect(() => {
		if (!open) {
			setWorkerAgent("");
			setWorkerModel("");
			setWorkerMode("");
			setWorkerModelWarning(undefined);
			setOrchestratorAgent("");
			setOrchestratorModel("");
			setOrchestratorMode("");
			setOrchestratorModelWarning(undefined);
			setWorkerAgentTouched(false);
			setIntake(EMPTY_INTAKE);
		}
	}, [open, path]);

	const modelPreferenceHint = t("createProject.modelPreferenceHint", {
		defaultValue: "Optional preference. Kennel can recommend a different model for an Outcome.",
	});

	return (
		<Dialog.Root open={open} onOpenChange={(next) => !isBusy && onOpenChange(next)}>
			<Dialog.Portal>
				<Dialog.Overlay className="dialog-overlay data-[state=open]:animate-overlay-in" />
				<Dialog.Content className="fixed left-1/2 top-1/2 z-overlay w-[min(480px,calc(100vw-32px))] -translate-x-1/2 -translate-y-1/2 rounded-xl border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-raised p-0 text-wd-primary shadow-[var(--shadow-overlay)] data-[state=open]:animate-modal-in">
					<ProjectSetupHeaderView
						CloseButton={ProjectSheetCloseButton}
						Description={Dialog.Description}
						Title={Dialog.Title}
						closeIcon={<X className="size-icon-base" aria-hidden="true" />}
						closeLabel={t("createProject.closeAgents")}
						disabled={isBusy}
						path={path ?? ""}
						title={kind === "workspace" ? t("createProject.workspaceAgents") : t("createProject.projectAgents")}
					/>
					<ProjectSetupFormView
						agentControls={{
							worker: (
								<div className="flex flex-col gap-3">
									<RequiredAgentField
										id="newProjectWorkerAgent"
										label={t("createProject.defaultCodingAgent")}
										placeholder={t("createProject.selectWorker")}
										value={workerAgent}
										authorized={authorizedAgents}
										installed={installedAgents}
										supported={supportedAgents}
										disabled={isLoadingAgents}
										labelClassName="agents-sheet-label"
										triggerClassName="agents-sheet-control"
										contentClassName="agents-sheet-menu"
										onChange={(value) => {
											setWorkerAgent(value);
											setWorkerAgentTouched(true);
											setWorkerModel("");
											setWorkerMode("");
											setWorkerModelWarning(undefined);
										}}
									/>
									{workerAgent ? (
										<ModelPreferenceField
											agentId={workerAgent}
											label={t("createProject.preferredModel", { defaultValue: "Preferred model" })}
											model={workerModel}
											mode={workerMode}
											hint={modelPreferenceHint}
											warning={workerModelWarning}
											disabled={isBusy}
											onModelChange={setWorkerModel}
											onModeChange={setWorkerMode}
											onWarningChange={setWorkerModelWarning}
										/>
									) : null}
								</div>
							),
							orchestrator: (
								<Accordion type="single" collapsible className="rounded-lg border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-surface">
									<AccordionItem value="advanced" className="border-none">
										<AccordionTrigger className="wd-label px-4 text-wd-primary">
											{t("createProject.advancedSettings")}
										</AccordionTrigger>
										<AccordionContent className="flex flex-col gap-4 px-4 pb-4">
											<p className="wd-body-s text-wd-secondary">
												{t("createProject.orchestratorAutoNotice")}
											</p>
											<RequiredAgentField
												id="newProjectOrchestratorAgent"
												selectableIds={coordinatorCapable}
												label={t("createProject.orchestratorAgent")}
												placeholder={t("createProject.selectOrchestrator")}
												value={orchestratorAgent}
												authorized={authorizedAgents}
												installed={installedAgents}
												supported={supportedAgents}
												disabled={isLoadingAgents}
												labelClassName="agents-sheet-label"
												triggerClassName="agents-sheet-control"
												contentClassName="agents-sheet-menu"
												onChange={(value) => {
													setOrchestratorAgent(value);
													setOrchestratorModel("");
													setOrchestratorMode("");
													setOrchestratorModelWarning(undefined);
												}}
											/>
											{orchestratorAgent ? (
												<ModelPreferenceField
													agentId={orchestratorAgent}
													label={t("createProject.preferredCoordinatorModel", {
														defaultValue: "Preferred coordinator model",
													})}
													model={orchestratorModel}
													mode={orchestratorMode}
													hint={modelPreferenceHint}
													warning={orchestratorModelWarning}
													disabled={isBusy}
													onModelChange={setOrchestratorModel}
													onModeChange={setOrchestratorMode}
													onWarningChange={setOrchestratorModelWarning}
												/>
											) : null}
										</AccordionContent>
									</AccordionItem>
								</Accordion>
							),
						}}
						agents={{
							cacheMessage: t("createProject.agentsCached"),
							error: displayError,
							loading: isLoadingAgents,
							loadingMessage: t("createProject.loadingAgents"),
							onRefresh: () => refreshAgentsMutation.mutate(),
							refreshLabel: refreshAgentsMutation.isPending
								? t("createProject.refreshing")
								: t("createProject.refreshAgents"),
							refreshing: refreshAgentsMutation.isPending,
							retryLabel: t("createProject.retry"),
						}}
						alert={
							effectiveAlert
								? {
										...effectiveAlert,
										icon: (
											<TriangleAlert
												className={
													effectiveAlert.tone === "warning"
														? "mt-0.5 size-icon-sm shrink-0 text-caution"
														: "mt-0.5 size-icon-sm shrink-0 text-critical"
												}
												aria-hidden="true"
											/>
										),
									}
								: null
						}
						canSubmit={canSubmit}
						cancelLabel={t("createProject.cancel")}
						intakeControl={
							<IntakeFields
								form={intake}
								onChange={(patch) => setIntake((f) => ({ ...f, ...patch }))}
								compact
								controlClassName="agents-sheet-control"
								labelClassName="agents-sheet-label"
							/>
						}
						isBusy={isBusy}
						onCancel={() => onOpenChange(false)}
						onSubmit={() =>
							void onSubmit({
								workerAgent,
								...(workerModel ? { workerModel } : {}),
								...(workerMode ? { workerMode } : {}),
								...(orchestratorAgent ? { orchestratorAgent } : {}),
								...(orchestratorAgent && orchestratorModel ? { orchestratorModel } : {}),
								...(orchestratorAgent && orchestratorMode ? { orchestratorMode } : {}),
								trackerIntake: buildIntake(intake),
							})
						}
						setupNotice={
							repositorySetupNeeded
								? { message: t("createProject.gitSetupNotice"), warning: repositorySetupWarning }
								: null
						}
						submitLabel={
							isInitializing
								? t("createProject.settingUp")
								: isCreating
									? t("createProject.creating")
									: kind === "workspace"
										? t("createProject.createWorkspaceAndStart")
										: t("createProject.createAndStart")
						}
					/>
				</Dialog.Content>
			</Dialog.Portal>
		</Dialog.Root>
	);
}

function ModelPreferenceField({
	agentId,
	disabled,
	hint,
	label,
	model,
	mode,
	onModelChange,
	onModeChange,
	onWarningChange,
	warning,
}: {
	agentId: string;
	disabled: boolean;
	hint: string;
	label: string;
	model: string;
	mode: string;
	onModelChange: (value: string) => void;
	onModeChange: (value: string) => void;
	onWarningChange: (warning: string | undefined) => void;
	warning?: string;
}) {
	return (
		<div className="flex flex-col gap-2">
			<div className="flex min-w-0 items-baseline gap-1.5">
				<Label className="agents-sheet-label">{label}</Label>
				<FieldDefaultHint text={hint} />
			</div>
			<AgentModelPicker
				agentId={agentId}
				agentLabel={agentLabel(agentId)}
				projectId=""
				value={model}
				mode={mode}
				disabled={disabled}
				onModelChange={onModelChange}
				onModeChange={onModeChange}
				onWarningChange={onWarningChange}
			/>
			{warning ? <p className="wd-body-s text-caution">{warning}</p> : null}
		</div>
	);
}

function ProjectSheetCloseButton({
	children,
	disabled,
	"aria-label": ariaLabel,
}: {
	children: ReactNode;
	disabled: boolean;
	"aria-label": string;
}) {
	return (
		<Dialog.Close asChild>
			<button
				type="button"
				className="settings-close-button"
				aria-label={ariaLabel}
				disabled={disabled}
			>
				{children}
			</button>
		</Dialog.Close>
	);
}

export const RequiredAgentField = memo(function RequiredAgentField({
	authorized,
	disabled = false,
	hint,
	icon,
	id,
	invalid = false,
	installed,
	label,
	onChange,
	placeholder,
	supported,
	selectableIds,
	triggerClassName,
	labelClassName,
	contentClassName,
	value,
	variant = "stacked",
}: {
	authorized?: AgentInfo[];
	disabled?: boolean;
	hint?: string;
	icon?: LucideIcon;
	id: string;
	invalid?: boolean;
	installed?: AgentInfo[];
	label: string;
	onChange: (value: string) => void;
	placeholder: string;
	supported?: AgentInfo[];
	triggerClassName?: string;
	labelClassName?: string;
	contentClassName?: string;
	value: string;
	variant?: "stacked" | "settings-row" | "chip";
	selectableIds?: ReadonlySet<string>;
}) {
	const baseSupported = supported ?? CORE_FALLBACK_AGENTS;
	const filteredSupported = selectableIds
		? baseSupported.filter((candidate) => selectableIds.has(candidate.id))
		: baseSupported;
	const filteredFallback = selectableIds
		? CORE_FALLBACK_AGENTS.filter((candidate) => selectableIds.has(candidate.id))
		: CORE_FALLBACK_AGENTS;
	const options = buildRankedAgentOptions({
		supported: filteredSupported,
		installed,
		authorized,
		fallbackAgents: filteredFallback,
	});

	if (variant === "settings-row") {
		const menuOptions = options.map((agent) => ({
			value: agent.id,
			label: agent.label,
			disabled: agent.disabled,
		}));

		return (
			<SettingsRow icon={icon} label={label}>
				<SettingsOptionMenu
					aria-label={label}
					value={value}
					placeholder={placeholder}
					options={menuOptions}
					disabled={disabled}
					onChange={onChange}
					triggerClassName={invalid ? "text-critical" : undefined}
					menuClassName="settings-agent-menu-surface"
					menuItemClassName="settings-agent-menu-item"
					renderTrigger={(selected, triggerPlaceholder) => (
						<>
							{selected ? <AgentAvatar provider={selected.value} className="size-icon-lg" /> : null}
							<span className="min-w-0 truncate">{selected?.label ?? triggerPlaceholder}</span>
						</>
					)}
					renderMenuItem={(option, selected) => {
						const agent = options.find((entry) => entry.id === option.value);
						if (!agent) return option.label;
						return (
							<AgentSelectMenuItem
								agentId={agent.id}
								label={agent.label}
								selected={selected}
								status={agent.status}
								statusTone={agent.statusTone}
								disabled={agent.disabled}
							/>
						);
					}}
				/>
			</SettingsRow>
		);
	}

	const selectedOption = options.find((agent) => agent.id === value);

	if (variant === "chip") {
		const menuOptions = options.map((agent) => ({
			value: agent.id,
			label: agent.label,
			disabled: agent.disabled,
		}));

		return (
			<SettingsOptionMenu
				aria-label={label}
				value={value}
				placeholder={placeholder}
				options={menuOptions}
				disabled={disabled}
				onChange={onChange}
				menuAlign="start"
				triggerClassName={cn(
					"composer-chip composer-toolbar-option w-full justify-between",
					invalid && "text-error",
					triggerClassName,
				)}
				menuClassName={contentClassName}
				renderTrigger={() => (
					<span className="flex min-w-0 items-center gap-2">
						{selectedOption ? (
							<AgentAvatar provider={selectedOption.id} className="size-icon-base" decorative />
						) : null}
						<span
							className="min-w-0 truncate text-sm text-wd-primary"
							title={selectedOption?.label ?? placeholder}
						>
							{selectedOption?.label ?? placeholder}
						</span>
					</span>
				)}
				renderMenuItem={(option, selected) => {
					const agent = options.find((entry) => entry.id === option.value);
					if (!agent) return option.label;
					return (
						<AgentSelectMenuItem
							agentId={agent.id}
							label={agent.label}
							selected={selected}
							status={agent.status}
							statusTone={agent.statusTone}
							disabled={agent.disabled}
						/>
					);
				}}
			/>
		);
	}

	return (
		<div className="flex flex-col gap-2">
			<div className="flex min-w-0 items-baseline gap-1.5">
				<Label htmlFor={id} className={cn("wd-label text-wd-secondary", labelClassName)}>
					{label}
				</Label>
				{hint && <FieldDefaultHint text={hint} />}
			</div>
			<Select value={value} onValueChange={onChange} disabled={disabled}>
				<SelectTrigger
					id={id}
					size="sm"
					className={cn("w-full text-sm aria-invalid:border-critical", triggerClassName)}
					aria-label={label}
					aria-invalid={invalid || undefined}
				>
					<SelectValue placeholder={placeholder}>
						{selectedOption ? (
							<span className="flex min-w-0 items-center gap-3">
								<AgentAvatar provider={selectedOption.id} className="size-icon-lg" decorative />
								<span className="min-w-0 truncate">{selectedOption.label}</span>
							</span>
						) : null}
					</SelectValue>
				</SelectTrigger>
				<SelectContent
					position="popper"
					side="bottom"
					align="start"
					sideOffset={4}
					className={cn("max-h-select-menu-max!", contentClassName)}
				>
					{options.map((agent) => (
						<SelectItem
							key={agent.id}
							value={agent.id}
							disabled={agent.disabled}
							className="[&>span:last-child]:w-full"
						>
							<AgentSelectMenuItem
								agentId={agent.id}
								label={agent.label}
								selected={value === agent.id}
								status={agent.status}
								statusTone={agent.statusTone}
								disabled={agent.disabled}
							/>
						</SelectItem>
					))}
				</SelectContent>
			</Select>
		</div>
	);
});

export function preferredDefaultAgent(options: RankedAgentOption[], preferredAgentId: string): string {
	if (preferredAgentId) {
		const preferred = options.find((option) => option.id === preferredAgentId);
		if (preferred && !preferred.disabled) return preferred.id;
	}
	return singleReadyProvider(options);
}
