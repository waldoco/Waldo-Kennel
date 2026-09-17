import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import {
	ProjectAgentsSettingsView,
	ProjectGeneralSettingsView,
	ProjectSettingsFormView,
	ProjectSettingsSection,
	ProjectWorkflowSettingsView,
	validateProjectSettings,
} from "@pin4sf/kennel-product-ui";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { useEffect, useState } from "react";
import { Pencil, RefreshCw } from "lucide-react";
import type { components } from "../../api/schema";
import {
	agentModelsQueryKey,
	agentModelsQueryOptions,
	refreshAgentModels,
	revalidateAgentModels,
	type AgentModelCatalog,
} from "../hooks/useAgentModelsQuery";
import { agentsQueryKey, agentsQueryOptions, refreshAgents } from "../hooks/useAgentsQuery";
import { useWorkspaceQuery, workspaceQueryKey } from "../hooks/useWorkspaceQuery";
import { apiClient, apiErrorMessage } from "../lib/api-client";
import { captureOrchestratorReplacementFailure } from "../lib/orchestrator-replacement-telemetry";
import { OrchestratorSpawnError, spawnOrchestrator } from "../lib/spawn-orchestrator";
import { captureRendererEvent } from "../lib/telemetry";
import { cn } from "../lib/utils";
import { type OrchestratorReplacementFailure, useUiStore } from "../stores/ui-store";
import { newestActiveOrchestrator } from "../types/workspace";
import { RequiredAgentField } from "./CreateProjectAgentSheet";
import { buildIntake, deriveGitHubRepo, IntakeFields, type IntakeForm } from "./IntakeFields";
import { ProductExternalLink } from "./ProductExternalLink";
import { ReviewerSelect, reviewerTrustWarning } from "./ReviewerSelect";
import { CodexPairingSection } from "./settings/CodexPairingSection";
import { AgentModelCombobox } from "./settings/AgentModelCombobox";
import { SettingsOptionMenu } from "./settings/SettingsOptionMenu";
import { SettingsRow } from "./settings/SettingsRow";

type Project = components["schemas"]["Project"];
type ProjectConfig = components["schemas"]["ProjectConfig"];
type TrackerIntakeConfig = components["schemas"]["TrackerIntakeConfig"];

const PERMISSION_MODE_VALUES = ["default", "accept-edits", "auto", "bypass-permissions"] as const;
const DEFAULT_BRANCH_AUTO = "auto";

const projectQueryKey = (id: string) => ["project", id] as const;

type SettingsSaveResult = {
	replacementError: string | null;
	replacementSessionId: string | null;
	replacementFailure: OrchestratorReplacementFailure | null;
	spawnError: unknown;
};

export type ProjectSettingsSection = "general" | "agents" | "workflow" | "intake";
export interface ProjectSettingsSaveState {
	isPending: boolean;
	showSaving: boolean;
	validationError: string | null;
	mutationError: string | null;
	saved: boolean;
	replacementError: string | null;
}

export function ProjectSettingsForm({
	projectId,
	section = "general",
	onSaveState,
}: {
	projectId: string;
	section?: ProjectSettingsSection;
	onSaveState?: (state: ProjectSettingsSaveState) => void;
}) {
	const { t } = useTranslation();
	const queryClient = useQueryClient();

	const query = useQuery({
		queryKey: projectQueryKey(projectId),
		queryFn: async () => {
			const { data, error } = await apiClient.GET("/api/v1/projects/{id}", {
				params: { path: { id: projectId } },
			});
			if (error) throw new Error(apiErrorMessage(error));
			if (data?.status !== "ok") throw new Error(t("settings.project.degraded"));
			return data.project as Project;
		},
	});

	return (
		<>
			{query.isLoading ? (
				<p className="text-sm text-settings-muted">{t("settings.project.loading")}</p>
			) : query.isError || !query.data ? (
				<p className="text-sm text-error">
					{query.error instanceof Error ? query.error.message : t("settings.project.loadFailed")}
				</p>
			) : (
				<SettingsBody
					key={projectId}
					project={query.data}
					onSaved={() =>
						queryClient.invalidateQueries({ queryKey: workspaceQueryKey }).catch(() => {
							// Saving succeeds even if the cache refresh fails.
						})
					}
					projectId={projectId}
					section={section}
					onSaveState={onSaveState}
				/>
			)}
		</>
	);
}

function SettingsBody({
	project,
	projectId,
	onSaved,
	section = "general",
	onSaveState,
}: {
	project: Project;
	projectId: string;
	onSaved: () => Promise<void>;
	section?: ProjectSettingsSection;
	onSaveState?: (state: ProjectSettingsSaveState) => void;
}) {
	const { t } = useTranslation();
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const closeSettings = useUiStore((state) => state.closeSettings);
	const setOrchestratorReplacementError = useUiStore((state) => state.setOrchestratorReplacementError);
	const workspaceQuery = useWorkspaceQuery();
	const config = project.config ?? {};
	const isScratchProject = project.kind === "scratch";
	const workspace = workspaceQuery.data?.find((item) => item.id === projectId);
	const activeOrchestrator = newestActiveOrchestrator(workspace?.sessions ?? []);
	const intake: TrackerIntakeConfig = config.trackerIntake ?? {};
	const [form, setForm] = useState({
		displayName: project.name,
		defaultBranch: config.defaultBranch ?? DEFAULT_BRANCH_AUTO,
		sessionPrefix: config.sessionPrefix ?? "",
		workerAgent: config.worker?.agent ?? "",
		orchestratorAgent: config.orchestrator?.agent ?? "",
		workerModel: config.worker?.agentConfig?.model ?? "",
		orchestratorModel: config.orchestrator?.agentConfig?.model ?? "",
		workerMode: config.worker?.agentConfig?.mode ?? "",
		workerProfile: config.worker?.agentConfig?.profile ?? "",
		orchestratorMode: config.orchestrator?.agentConfig?.mode ?? "",
		orchestratorProfile: config.orchestrator?.agentConfig?.profile ?? "",
		permissions: config.agentConfig?.permissions ?? "",
		agentPreferences: {
			defaultWorker: config.agentPreferences?.defaultWorker ?? "",
			analyzer: config.agentPreferences?.analyzer ?? "",
			coordinator: config.agentPreferences?.coordinator ?? "",
			verifier: config.agentPreferences?.verifier ?? "",
		},
		reviewerHarness: config.reviewers?.[0]?.harness ?? "",
		intakeEnabled: intake.enabled ?? false,
		intakeRepo: intake.repo ?? "",
		intakeAssignee: intake.assignee ?? "",
	});
	const [savedAt, setSavedAt] = useState<number | null>(null);
	const [showSaving, setShowSaving] = useState(false);
	const [replacementError, setReplacementError] = useState<string | null>(null);
	const [validationError, setValidationError] = useState<string | null>(null);
	const initialOrchestratorAgent = config.orchestrator?.agent ?? "";
	const agentsQuery = useQuery(agentsQueryOptions);
	const agentCatalog = agentsQuery.data;
	// The daemon-resolved Mission-role proposal: stored preferences enriched
	// with live adapter admission. Advisory read-only truth for this project.
	const resolvedRolesQuery = useQuery({
		queryKey: ["project", projectId, "resolved-mission-roles"],
		queryFn: async () => {
			const { data, error } = await apiClient.GET("/api/v1/projects/{id}/resolved-mission-roles", {
				params: { path: { id: projectId } },
			});
			if (error) throw new Error(apiErrorMessage(error));
			return data.roles;
		},
	});
	const patchPreference = (role: "defaultWorker" | "analyzer" | "coordinator" | "verifier", value: string) =>
		setForm((f) => ({ ...f, agentPreferences: { ...f.agentPreferences, [role]: value } }));
	const refreshAgentsMutation = useMutation({
		mutationFn: refreshAgents,
		onSuccess: (next) => queryClient.setQueryData(agentsQueryKey, next),
	});

	const intakeForm: IntakeForm = {
		enabled: form.intakeEnabled,
		repo: form.intakeRepo,
		assignee: form.intakeAssignee,
	};
	const patchIntake = (patch: Partial<IntakeForm>) =>
		setForm((f) => ({
			...f,
			intakeEnabled: patch.enabled ?? f.intakeEnabled,
			intakeRepo: patch.repo ?? f.intakeRepo,
			intakeAssignee: patch.assignee ?? f.intakeAssignee,
		}));
	const effectiveIntakeRepo = form.intakeRepo.trim() || deriveGitHubRepo(project.repo);
	const reviewerWarning = reviewerTrustWarning(form.reviewerHarness);

	// Role admission and profile requirements come from the daemon's agent
	// inventory; the UI never re-derives them from provider names. Optional
	// chaining keeps stale catalogs (or old fixtures) from crashing the form.
	const supportedAgents = agentCatalog?.supported ?? [];
	const coordinatorCapable = new Set(
		supportedAgents.filter((agent) => agent.roles?.coordinator).map((agent) => agent.id),
	);
	const workerSelectable = supportedAgents.filter((agent) => agent.roles?.worker).map((agent) => agent.id);
	// Preference menus present catalog labels (never raw ids) and only
	// admission-admitted harnesses, mirroring what SetConfig will accept.
	const toPreferenceOptions = (ids: string[]) =>
		ids.map((id) => ({ id, label: supportedAgents.find((agent) => agent.id === id)?.label ?? id }));
	const workerRequiresProfile =
		supportedAgents.find((agent) => agent.id === form.workerAgent)?.requiresProfile === true;

	const mutation = useMutation({
		mutationFn: async () => {
			void captureRendererEvent("kennel.renderer.settings_save_requested", { project_id: projectId });
			const displayName = form.displayName.trim();
			const {
				model: _legacyModel,
				mode: _legacyMode,
				...sharedAgentConfig
			} = config.agentConfig ?? {};
			const worker = buildRoleOverride(
				config.worker,
				form.workerAgent,
				form.workerModel,
				form.workerMode,
				form.workerProfile,
			);
			const orchestrator = buildRoleOverride(
				config.orchestrator,
				form.orchestratorAgent,
				form.orchestratorModel,
				form.orchestratorMode,
				form.orchestratorProfile,
			);
			const next: ProjectConfig = isScratchProject
				? {
						...scratchSupportedConfig(config),
						worker,
						orchestrator,
						agentConfig: blankToUndefined({
							...sharedAgentConfig,
							permissions: form.permissions || undefined,
						}),
						agentPreferences: preferencesPayload(form.agentPreferences),
					}
				: {
						...config,
						defaultBranch:
							form.defaultBranch.trim() === DEFAULT_BRANCH_AUTO
								? undefined
								: form.defaultBranch || undefined,
						sessionPrefix: form.sessionPrefix || undefined,
						worker,
						orchestrator,
						agentConfig: blankToUndefined({
							...sharedAgentConfig,
							permissions: form.permissions || undefined,
						}),
						agentPreferences: preferencesPayload(form.agentPreferences),
						reviewers: form.reviewerHarness ? [{ harness: form.reviewerHarness }] : undefined,
						trackerIntake: buildIntake(intakeForm),
					};
			const { error } = await apiClient.PUT("/api/v1/projects/{id}", {
				params: { path: { id: projectId } },
				body: { displayName, config: next },
			});
			if (error) throw new Error(apiErrorMessage(error));
			// Coordinator configuration is optional. Only an explicitly selected
			// coordinator may trigger a replacement/start; clearing or leaving the
			// role empty never reuses the worker and never manufactures a provider.
			if (
				form.orchestratorAgent !== "" &&
				(form.orchestratorAgent !== initialOrchestratorAgent ||
					Boolean(activeOrchestrator && activeOrchestrator.provider !== form.orchestratorAgent))
			) {
				try {
					const sessionId = await spawnOrchestrator(projectId, "settings", true);
					return {
						replacementError: null,
						replacementSessionId: sessionId,
						replacementFailure: null,
						spawnError: null,
					} satisfies SettingsSaveResult;
				} catch (error) {
					const replacementFailure: OrchestratorReplacementFailure = {
						message:
							error instanceof Error ? error.message : t("settings.project.replaceOrchestratorFailed"),
						...(error instanceof OrchestratorSpawnError
							? { code: error.code, requestId: error.requestId }
							: {}),
					};
					return {
						replacementError: replacementFailure.message,
						replacementSessionId: null,
						replacementFailure,
						spawnError: error,
					} satisfies SettingsSaveResult;
				}
			}
			return {
				replacementError: null,
				replacementSessionId: null,
				replacementFailure: null,
				spawnError: null,
			} satisfies SettingsSaveResult;
		},
		onSuccess: async (result) => {
			void captureRendererEvent("kennel.renderer.settings_save_succeeded", { project_id: projectId });
			setSavedAt(Date.now());
			setReplacementError(result.replacementError);
			setValidationError(null);
			void queryClient.invalidateQueries({ queryKey: ["project", projectId] });
			const workspaceRefresh = onSaved();

			if (result.replacementSessionId) {
				await workspaceRefresh;
				closeSettings();
				void navigate({
					to: "/projects/$projectId/sessions/$sessionId",
					params: { projectId, sessionId: result.replacementSessionId },
				});
				return;
			}

			if (result.replacementFailure) {
				closeSettings();
				setOrchestratorReplacementError(projectId, result.replacementFailure);
				if (result.spawnError) {
					captureOrchestratorReplacementFailure(result.spawnError, projectId);
				}
			}
		},
		onError: () => {
			void captureRendererEvent("kennel.renderer.settings_save_failed", { project_id: projectId });
		},
	});

	useEffect(() => {
		if (!mutation.isPending) {
			setShowSaving(false);
			return;
		}
		const timeout = window.setTimeout(() => setShowSaving(true), 200);
		return () => window.clearTimeout(timeout);
	}, [mutation.isPending]);

	useEffect(() => {
		onSaveState?.({
			isPending: mutation.isPending,
			showSaving,
			validationError,
			mutationError: mutation.isError
				? mutation.error instanceof Error
					? mutation.error.message
					: t("settings.project.saveFailed")
				: null,
			saved: savedAt !== null && !mutation.isPending && !mutation.isError,
			replacementError:
				replacementError && !mutation.isPending && !mutation.isError ? replacementError : null,
		});
	}, [
		mutation.error,
		mutation.isError,
		mutation.isPending,
		onSaveState,
		replacementError,
		savedAt,
		showSaving,
		t,
		validationError,
	]);

	useEffect(() => {
		if (savedAt === null) return;
		const timeout = window.setTimeout(() => setSavedAt(null), 1800);
		return () => window.clearTimeout(timeout);
	}, [savedAt]);

	return (
		<ProjectSettingsFormView
			id="project-settings-form"
			onSubmit={() => {
				setSavedAt(null);
				setReplacementError(null);
				const validation = validateProjectSettings(form, { validateIntake: !isScratchProject });
				if (validation) {
					setValidationError(
						validation === "name_required"
							? t("settings.project.nameRequired")
							: validation === "intake_assignee_required"
								? t("settings.project.intakeAssigneeRequired")
								: t("settings.project.agentsRequired"),
					);
					return;
				}
				setValidationError(null);
				mutation.mutate();
			}}
		>
			{section === "general" && (
				<>
					<ProjectGeneralSettingsView
						displayName={form.displayName}
						externalLink={ProductExternalLink}
						icons={{
							edit: <Pencil className="settings-inline-edit-icon" aria-hidden="true" />,
						}}
						onDisplayNameChange={(displayName) => setForm((f) => ({ ...f, displayName }))}
						labels={{
							title: t("settings.project.identity"),
							name: t("settings.project.name"),
							id: t("settings.project.id"),
							kind: t("settings.project.kind"),
							path: t("settings.project.path"),
							repo: t("settings.project.repo"),
							workspaceRepos: t("settings.project.workspaceRepos"),
							workspaceReposEmpty: t("settings.project.childReposEmpty"),
							editName: t("settings.field.edit", { label: t("settings.project.name") }),
						}}
						project={{
							id: project.id,
							kindLabel: projectKindLabel(project.kind, t),
							path: project.path,
							pathHref: `file://${encodeURI(project.path)}`,
							repo: project.repo,
							repoHref: project.repo ? repositoryHref(project.repo) : undefined,
							workspaceRepos: project.kind === "workspace" ? project.workspaceRepos ?? [] : undefined,
						}}
					/>
				</>
			)}

			{section === "agents" && (
				<>
					<CodexPairingSection projectId={projectId} />
					<ProjectAgentsSettingsView
						title={t("settings.project.agents")}
						workerArea={
							<RequiredAgentField
								id="workerAgent"
								variant="settings-row"
								value={form.workerAgent}
								placeholder={t("settings.project.selectWorker")}
								label={t("settings.project.defaultWorker")}
								authorized={agentCatalog?.authorized}
								installed={agentCatalog?.installed}
								supported={agentCatalog?.supported}
								disabled={agentsQuery.isFetching && agentCatalog === undefined}
								onChange={(v) =>
									setForm((f) => ({
										...f,
										workerAgent: v,
										workerModel: "",
										workerMode: "",
										workerProfile: "",
									}))
								}
							/>
						}
						workerModelArea={
							<AgentModelField
								role="worker"
								agentId={form.workerAgent}
								projectId={projectId}
								model={form.workerModel}
								mode={form.workerMode}
								profile={form.workerProfile}
								requiresProfile={workerRequiresProfile}
								onModelChange={(workerModel) => setForm((f) => ({ ...f, workerModel }))}
								onModeChange={(workerMode) => setForm((f) => ({ ...f, workerMode }))}
								onProfileChange={(workerProfile) => setForm((f) => ({ ...f, workerProfile }))}
							/>
						}
						orchestratorArea={
							<RequiredAgentField
								id="orchestratorAgent"
								selectableIds={coordinatorCapable.size > 0 ? coordinatorCapable : undefined}
								variant="settings-row"
								value={form.orchestratorAgent}
								placeholder={t("settings.project.selectOrchestrator")}
								label={t("settings.project.defaultOrchestrator")}
								authorized={agentCatalog?.authorized}
								installed={agentCatalog?.installed}
								supported={agentCatalog?.supported}
								disabled={agentsQuery.isFetching && agentCatalog === undefined}
								onChange={(v) =>
									setForm((f) => ({
										...f,
										orchestratorAgent: v,
										orchestratorModel: "",
										orchestratorMode: "",
										orchestratorProfile: "",
									}))
								}
							/>
						}
						orchestratorModelArea={
							<AgentModelField
								role="orchestrator"
								agentId={form.orchestratorAgent}
								projectId={projectId}
								model={form.orchestratorModel}
								mode={form.orchestratorMode}
								profile={form.orchestratorProfile}
								onModelChange={(orchestratorModel) => setForm((f) => ({ ...f, orchestratorModel }))}
								onModeChange={(orchestratorMode) => setForm((f) => ({ ...f, orchestratorMode }))}
								onProfileChange={(orchestratorProfile) => setForm((f) => ({ ...f, orchestratorProfile }))}
							/>
						}
						permissions={{
							control: (
								<PermissionModeSelect
									value={form.permissions}
									onChange={(v) => setForm((f) => ({ ...f, permissions: v }))}
								/>
							),
							label: t("settings.project.permissionMode"),
						}}
						refresh={{
							actionIcon: (
								<RefreshCw
									className={cn(
										"size-icon-base",
										refreshAgentsMutation.isPending && "animate-spin",
									)}
									aria-hidden="true"
								/>
							),
							disabled: refreshAgentsMutation.isPending,
							label: t("settings.project.refreshAgents"),
							onClick: () => refreshAgentsMutation.mutate(),
							value: refreshAgentsMutation.isPending
								? t("settings.project.refreshing")
								: t("settings.project.refresh"),
						}}
						error={
							refreshAgentsMutation.isError
								? refreshAgentsMutation.error instanceof Error
									? refreshAgentsMutation.error.message
									: t("settings.project.refreshFailed")
								: null
						}
						missingRequiredMessage={null}
					/>
					<div className="mt-5 rounded-card border border-border bg-card p-4 hairline">
						<p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
							{t("settings.project.missionRoles")}
						</p>
						<p className="mt-1 text-xs leading-body text-muted-foreground">
							{t("settings.project.missionRolesHint")}
						</p>
						<div className="mt-3 grid gap-3 sm:grid-cols-2">
							<MissionRolePreference
								label={t("settings.project.missionRoleWorker")}
								value={form.agentPreferences.defaultWorker}
								options={toPreferenceOptions(workerSelectable)}
								onChange={(v) => patchPreference("defaultWorker", v)}
							/>
							<MissionRolePreference
								label={t("settings.project.missionRoleAnalyzer")}
								value={form.agentPreferences.analyzer}
								options={toPreferenceOptions([...coordinatorCapable])}
								onChange={(v) => patchPreference("analyzer", v)}
							/>
							<MissionRolePreference
								label={t("settings.project.missionRoleCoordinator")}
								value={form.agentPreferences.coordinator}
								options={toPreferenceOptions([...coordinatorCapable])}
								onChange={(v) => patchPreference("coordinator", v)}
							/>
							<MissionRolePreference
								label={t("settings.project.missionRoleVerifier")}
								value={form.agentPreferences.verifier}
								options={toPreferenceOptions([...coordinatorCapable])}
								onChange={(v) => patchPreference("verifier", v)}
							/>
						</div>
						{resolvedRolesQuery.data ? (
							<ResolvedMissionRolesList roles={resolvedRolesQuery.data} />
						) : null}
					</div>
				</>
			)}

			{section === "workflow" && (
				<>
					{!isScratchProject ? (
						<>
							<ProjectWorkflowSettingsView
								branch={form.defaultBranch}
								icons={{
									edit: <Pencil className="settings-inline-edit-icon" aria-hidden="true" />,
								}}
								prefix={form.sessionPrefix}
								onBranchChange={(defaultBranch) => setForm((f) => ({ ...f, defaultBranch }))}
								onPrefixChange={(sessionPrefix) => setForm((f) => ({ ...f, sessionPrefix }))}
								labels={{
									worktrees: t("settings.project.worktrees"),
									defaultBranch: t("settings.project.defaultBranch"),
									sessionPrefix: t("settings.project.sessionPrefix"),
									reviewers: t("settings.project.reviewers"),
									defaultReviewer: t("settings.project.defaultReviewer"),
									editDefaultBranch: t("settings.field.edit", {
										label: t("settings.project.defaultBranch"),
									}),
									editSessionPrefix: t("settings.field.edit", {
										label: t("settings.project.sessionPrefix"),
									}),
								}}
								reviewerControl={
									<ReviewerSelect
										value={form.reviewerHarness}
										onChange={(v) => setForm((f) => ({ ...f, reviewerHarness: v }))}
										ariaLabel={t("settings.project.defaultReviewer")}
										authorized={agentCatalog?.authorized}
										defaultOptionLabel={t("settings.project.default")}
										defaultTriggerLabel={t("settings.project.default")}
										installed={agentCatalog?.installed}
										supported={agentCatalog?.supported}
										disabled={agentsQuery.isFetching && agentCatalog === undefined}
									/>
								}
								reviewerWarning={reviewerWarning}
							/>
						</>
					) : (
						<p className="px-1 text-xs text-settings-muted">{t("settings.project.workflow")}</p>
					)}
				</>
			)}

			{section === "intake" && (
				<>
					{!isScratchProject ? (
						<ProjectSettingsSection title={t("settings.project.trackerIntake")} grouped>
							<IntakeFields
								variant="settings"
								form={intakeForm}
								onChange={patchIntake}
								repoPreview={{ value: effectiveIntakeRepo }}
							/>
						</ProjectSettingsSection>
					) : (
						<p className="px-1 text-xs text-settings-muted">{t("settings.project.trackerIntake")}</p>
					)}
				</>
			)}
		</ProjectSettingsFormView>
	);
}

function AgentModelField({
	role,
	agentId,
	projectId,
	model,
	mode,
	profile,
	requiresProfile = false,
	onModelChange,
	onModeChange,
	onProfileChange,
}: {
	role: "worker" | "orchestrator";
	agentId: string;
	projectId: string;
	model: string;
	mode: string;
	profile: string;
	/** Daemon-declared launch requirement: this adapter boots a user-selected profile (AgentConfig.Profile). */
	requiresProfile?: boolean;
	onModelChange: (value: string) => void;
	onModeChange: (value: string) => void;
	onProfileChange: (value: string) => void;
}) {
	const { t } = useTranslation();
	const queryClient = useQueryClient();
	const [customAgentId, setCustomAgentId] = useState<string | null>(null);
	const query = useQuery(agentModelsQueryOptions(agentId, projectId));
	const catalog: AgentModelCatalog | undefined = query.data;
	const revalidationQuery = useQuery({
		queryKey: ["agent-model-revalidation", agentId, projectId, catalog?.validatedAt ?? ""],
		queryFn: () => revalidateAgentModels(agentId, projectId),
		enabled: agentId !== "" && catalog?.refreshRecommended === true,
		staleTime: Number.POSITIVE_INFINITY,
		retry: false,
	});
	useEffect(() => {
		if (revalidationQuery.data) {
			queryClient.setQueryData(agentModelsQueryKey(agentId, projectId), revalidationQuery.data);
		}
	}, [agentId, projectId, queryClient, revalidationQuery.data]);
	const refreshMutation = useMutation({
		mutationFn: () => refreshAgentModels(agentId, projectId),
		onSuccess: (catalog) => queryClient.setQueryData(agentModelsQueryKey(agentId, projectId), catalog),
	});
	const isMode = catalog?.selectionMode === "mode";
	const label = t(`settings.models.${role}${isMode ? "Mode" : "Model"}`);
	const datalistID = `${role}-model-options`;
	const warning =
		(refreshMutation.isError
			? refreshMutation.error instanceof Error
				? refreshMutation.error.message
				: t("settings.models.refreshFailed")
			: undefined) ??
		(revalidationQuery.isError
			? revalidationQuery.error instanceof Error
				? revalidationQuery.error.message
				: t("settings.models.validateFailed")
			: undefined) ??
		catalog?.warning ??
		(query.isError ? (query.error instanceof Error ? query.error.message : t("settings.models.loadFailed")) : undefined);

	if (isMode) {
		const options = [
			{ value: "__default__", label: t("settings.models.agentDefault") },
			...(catalog.models ?? []).map((item) => ({ value: item.id, label: item.label })),
		];
		return (
			<>
				<SettingsRow label={label}>
					<div className="flex min-w-0 items-center gap-2">
						<ModelRefreshButton
							label={label}
							pending={refreshMutation.isPending}
							disabled={agentId === ""}
							onClick={() => refreshMutation.mutate()}
						/>
						<SettingsOptionMenu
							aria-label={label}
							value={mode || "__default__"}
							options={options}
							triggerClassName="justify-end"
							onChange={(value) => {
								onModeChange(value === "__default__" ? "" : value);
								onModelChange("");
							}}
						/>
					</div>
				</SettingsRow>
				{warning && <p className="px-1 text-xs leading-row text-warning">{warning}</p>}
			</>
		);
	}

	const hasCatalog = catalog?.selectionMode === "catalog" && (catalog.models?.length ?? 0) > 0;
	const modelIsInCatalog = catalog?.models?.some((item) => item.id === model) ?? false;
	const showCustomInput = hasCatalog && (customAgentId === agentId || (model !== "" && !modelIsInCatalog));
	// Profile-required harnesses carry their launch profile in AgentConfig.Profile,
	// entered as free text beside the model field.
	const profileLabel = t("settings.project.profile");
	const selectCatalogModel = (value: string) => {
		setCustomAgentId(null);
		onModelChange(value);
		onModeChange("");
	};
	const selectCustomModel = (value: string) => {
		setCustomAgentId(agentId);
		onModelChange(value);
		onModeChange("");
	};
	return (
		<>
			<SettingsRow label={label}>
				<div className="flex min-w-0 items-center gap-2">
					<ModelRefreshButton
						label={label}
						pending={refreshMutation.isPending}
						disabled={agentId === ""}
						onClick={() => refreshMutation.mutate()}
					/>
					{hasCatalog && !showCustomInput ? (
						<AgentModelCombobox
							aria-label={label}
							value={model}
							models={catalog.models}
							allowCustom={catalog.allowCustom}
							onChange={selectCatalogModel}
							onCustom={selectCustomModel}
							triggerClassName="justify-end"
						/>
					) : (
						<>
							<input
								id={datalistID}
								aria-label={label}
								className="settings-inline-input settings-model-control"
								value={model}
								disabled={agentId === ""}
								onChange={(event) => {
									onModelChange(event.target.value);
									onModeChange("");
								}}
								placeholder={query.isFetching ? t("settings.models.loading") : t("settings.project.agentDefault")}
							/>
							{hasCatalog && (
								<AgentModelCombobox
									aria-label={t("settings.models.optionsAria", { label })}
									value={model}
									models={catalog.models}
									allowCustom={catalog.allowCustom}
									onChange={selectCatalogModel}
									onCustom={selectCustomModel}
									triggerLabel={t("settings.models.browse")}
									triggerClassName="shrink-0"
								/>
							)}
						</>
					)}
				</div>
			</SettingsRow>
			{requiresProfile && (
				<SettingsRow label={profileLabel}>
					<input
						id={`${role}-profile`}
						aria-label={profileLabel}
						className="settings-inline-input settings-model-control"
						value={profile}
						disabled={agentId === ""}
						onChange={(event) => onProfileChange(event.target.value)}
					/>
				</SettingsRow>
			)}
			{warning && <p className="px-1 text-xs leading-row text-warning">{warning}</p>}
		</>
	);
}

function ModelRefreshButton({
	label,
	pending,
	disabled,
	onClick,
}: {
	label: string;
	pending: boolean;
	disabled: boolean;
	onClick: () => void;
}) {
	const { t } = useTranslation();
	return (
		<button
			type="button"
			aria-label={t("settings.models.refreshAria", { label: label.toLocaleLowerCase() })}
			title={t("settings.models.refreshAria", { label: label.toLocaleLowerCase() })}
			className="settings-option-trigger shrink-0 disabled:pointer-events-none disabled:opacity-50"
			disabled={disabled || pending}
			onClick={onClick}
		>
			<RefreshCw className={cn("size-icon-sm", pending && "animate-spin")} aria-hidden="true" />
		</button>
	);
}

function PermissionModeSelect({ value, onChange }: { value: string; onChange: (value: string) => void }) {
	const { t } = useTranslation();
	const options = [
		{ value: "__default__", label: t("settings.project.default") },
		...PERMISSION_MODE_VALUES.map((value) => ({
			value,
			label:
				value === "default"
					? t("settings.project.permissionDefault")
					: value === "accept-edits"
						? t("settings.project.permissionAcceptEdits")
						: value === "auto"
							? t("settings.project.permissionAuto")
							: t("settings.project.permissionBypass"),
		})),
	];

	return (
		<SettingsOptionMenu
			aria-label={t("settings.project.permissionMode")}
			value={value || "__default__"}
			options={options}
			onChange={(v) => onChange(v === "__default__" ? "" : v)}
		/>
	);
}

function projectKindLabel(kind: string, t: TFunction): string {
	switch (kind) {
		case "single_repo":
			return t("settings.project.kind.singleRepo");
		case "workspace":
			return t("settings.project.kind.workspace");
		case "scratch":
			return t("settings.project.kind.scratch");
		default:
			return kind || t("settings.project.kind.unknown");
	}
}

function repositoryHref(repository: string): string {
	if (/^https?:\/\//i.test(repository)) return repository;
	if (repository.startsWith("git@")) {
		const [host, path] = repository.slice(4).split(":", 2);
		return `https://${host}/${path.replace(/\.git$/, "")}`;
	}
	if (repository.startsWith("ssh://")) {
		try {
			const parsed = new URL(repository);
			return `https://${parsed.hostname}${parsed.pathname.replace(/\.git$/, "")}`;
		} catch {
			return repository;
		}
	}
	return repository;
}

function scratchSupportedConfig(config: ProjectConfig): ProjectConfig {
	const {
		defaultBranch: _defaultBranch,
		reviewers: _reviewers,
		autoReview: _legacyAutoReview,
		trackerIntake: _trackerIntake,
		...supported
	} = config as ProjectConfig & { autoReview?: unknown };
	return supported;
}

function blankToUndefined<T extends object>(obj: T): T | undefined {
	return Object.values(obj).some((v) => v !== undefined) ? obj : undefined;
}

function preferencesPayload(prefs: {
	defaultWorker: string;
	analyzer: string;
	coordinator: string;
	verifier: string;
}): components["schemas"]["ProjectAgentPreferences"] | undefined {
	const next = {
		defaultWorker: prefs.defaultWorker || undefined,
		analyzer: prefs.analyzer || undefined,
		coordinator: prefs.coordinator || undefined,
		verifier: prefs.verifier || undefined,
	};
	return Object.values(next).some((v) => v !== undefined) ? next : undefined;
}

// MissionRolePreference edits one optional role preference. An empty value
// means no preference; the daemon may inherit the matching explicit Project
// role selection, otherwise the role remains unassigned.
function MissionRolePreference({
	label,
	value,
	options,
	onChange,
}: {
	label: string;
	value: string;
	options: { id: string; label: string }[];
	onChange: (value: string) => void;
}) {
	const { t } = useTranslation();
	const menuOptions = [
		{ value: "__none__", label: t("settings.project.missionRoleNoPreference") },
		...options.map((option) => ({ value: option.id, label: option.label })),
	];
	return (
		<label className="flex flex-col gap-1 text-xs font-medium">
			{label}
			<SettingsOptionMenu
				aria-label={label}
				value={value || "__none__"}
				options={menuOptions}
				onChange={(v) => onChange(v === "__none__" ? "" : v)}
			/>
		</label>
	);
}

// ResolvedMissionRolesList renders the daemon's advisory resolution for each
// Mission role, including unassigned state and readiness reasons. It never
// re-derives admission client-side.
function ResolvedMissionRolesList({ roles }: { roles: components["schemas"]["ResolvedMissionRoles"] }) {
	const { t } = useTranslation();
	const rows = [
		{ label: t("settings.project.defaultWorker"), role: roles.worker },
		{ label: t("settings.project.missionRoleAnalyzer"), role: roles.analyzer },
		{ label: t("settings.project.missionRoleCoordinator"), role: roles.coordinator },
		{ label: t("settings.project.missionRoleVerifier"), role: roles.verifier },
	];
	return (
		<dl className="mt-4 grid gap-2 border-t border-border pt-3 text-xs leading-snug">
			{rows.map(({ label, role }) => {
				const unassigned = !role.harness;
				return (
					<div key={label} className="flex items-baseline justify-between gap-3">
						<dt className="text-muted-foreground">{label}</dt>
						<dd className="flex items-baseline gap-2 text-right">
							<span>{role.harness || t("settings.project.missionRoleNoPreference")}</span>
							<span
								className={
									role.source === "preference"
										? "rounded-xs bg-[--color-status-ready] px-1.5 py-0.5 text-2xs text-background"
										: "rounded-xs bg-popover px-1.5 py-0.5 text-2xs text-muted-foreground"
								}
							>
								{role.source === "preference"
									? t("settings.project.missionRoleFromPreference")
									: unassigned
										? t("settings.project.missionRoleNoPreference")
										: t("settings.project.missionRoleFromDefault")}
							</span>
							{!role.ready && role.reason ? (
								<span className="text-2xs text-muted-foreground">{role.reason}</span>
							) : null}
						</dd>
					</div>
				);
			})}
		</dl>
	);
}

function buildRoleOverride(
	existing: components["schemas"]["RoleOverride"] | undefined,
	agent: string,
	model: string,
	mode: string,
	profile: string,
): components["schemas"]["RoleOverride"] | undefined {
	if (!agent) return undefined;
	return {
		...existing,
		agent,
		agentConfig: buildRoleAgentConfig(existing?.agentConfig, model, mode, profile),
	};
}

function buildRoleAgentConfig(
	existing: components["schemas"]["AgentConfig"] | undefined,
	model: string,
	mode: string,
	profile: string,
): components["schemas"]["AgentConfig"] | undefined {
	const next = { ...existing };
	if (model) next.model = model;
	else delete next.model;
	if (mode) next.mode = mode;
	else delete next.mode;
	if (profile) next.profile = profile;
	else delete next.profile;
	return Object.keys(next).length > 0 ? next : undefined;
}
