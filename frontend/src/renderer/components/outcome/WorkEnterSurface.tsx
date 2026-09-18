import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { apiClient, apiErrorMessage } from "../../lib/api-client";
import { createProjectConfig } from "../../lib/create-project-config";
import { useShellMaybe } from "../../lib/shell-context";
import { aoBridge } from "../../lib/bridge";
import { mockWorkspaces } from "../../lib/mock-data";
import { usesPreviewWorkspaceData, usesWorkLaunchMode } from "../../lib/preview-mode";
import { CreateProjectFlow, type CreateProjectInput } from "../CreateProjectFlow";
import { Button } from "../ui/button";
import { OutcomeLifecycleShell } from "./OutcomeLifecycleShell";

// v0 dogfood provider. Codex-only is a *testing constraint*, not a v1 provider
// decision, so this stays one named constant rather than provider branching.
const V0_PROVIDER_ID = "codex";

type EnterDestination = "undecided" | "work";

type Project = { id: string; name: string; path: string };
type AgentInfo = { id: string; name?: string };
type AgentInventory = { supported?: AgentInfo[]; installed?: AgentInfo[]; authorized?: AgentInfo[] };

async function fetchProjects(): Promise<Project[]> {
	const { data, error } = await apiClient.GET("/api/v1/projects");
	if (error) throw new Error(apiErrorMessage(error));
	return ((data as { projects?: Project[] } | undefined)?.projects ?? []) as Project[];
}

async function fetchAgents(): Promise<AgentInventory> {
	const { data, error } = await apiClient.GET("/api/v1/agents");
	if (error) throw new Error(apiErrorMessage(error));
	return (data ?? {}) as AgentInventory;
}

/**
 * Enter: "What responsibility are we taking on, and where does it belong?"
 *
 * Work-first is the v0 dogfood *recommendation*; Home stays an equal path that
 * never blocks Work and is never silently created by arriving here. Choosing a
 * destination is navigation, not a durable mutation — this surface performs no
 * Outcome or PersonalHome writes.
 */
export function WorkEnterSurface() {
	const { t } = useTranslation();
	const navigate = useNavigate();
	const shell = useShellMaybe();
	const [destination, setDestination] = useState<EnterDestination>(usesWorkLaunchMode ? "work" : "undecided");

	const daemonQuery = useQuery({
		queryKey: ["daemon-status", "enter"],
		queryFn: () => aoBridge.daemon.getStatus(),
		enabled: !usesPreviewWorkspaceData && !shell,
		refetchInterval: 2_000,
	});
	const projectsQuery = useQuery({
		queryKey: ["projects", "enter"],
		queryFn: () =>
			usesPreviewWorkspaceData
				? Promise.resolve(mockWorkspaces.map(({ id, name, path }) => ({ id, name, path })))
				: fetchProjects(),
		enabled: destination === "work",
	});
	const agentsQuery = useQuery({
		queryKey: ["agents", "enter"],
		queryFn: () =>
			usesPreviewWorkspaceData
				? Promise.resolve({ authorized: [{ id: V0_PROVIDER_ID, name: "Codex" }] })
				: fetchAgents(),
		enabled: destination === "work" && !usesWorkLaunchMode,
	});

	// The daemon owns every canonical fact this surface would act on, so an
	// unavailable daemon is stated plainly rather than rendered as an empty list.
	const daemonReady = usesPreviewWorkspaceData || (shell?.daemonStatus ?? daemonQuery.data)?.state === "ready";

	// Catalog readiness is an advisory local probe, never a spawn precheck. An
	// unauthorized provider is therefore Action Required — an exact human-only
	// step — and it must not hide Projects or pretend admission already failed.
	const providerReady =
		agentsQuery.data?.authorized?.some((agent) => agent.id === V0_PROVIDER_ID) ?? !agentsQuery.isSuccess;

	async function createProject(input: CreateProjectInput): Promise<void> {
		if (shell) {
			await shell.createProject(input);
			await projectsQuery.refetch();
			return;
		}
		const { error } = await apiClient.POST("/api/v1/projects", {
			body: { path: input.path, asWorkspace: input.asWorkspace, config: createProjectConfig(input) },
		});
		if (error) throw new Error(apiErrorMessage(error));
		await projectsQuery.refetch();
	}

	async function initializeProject(path: string): Promise<void> {
		const { error } = await apiClient.POST("/api/v1/projects/initialize", { body: { path } });
		if (error) throw new Error(apiErrorMessage(error));
	}

	return (
		<OutcomeLifecycleShell stage="enter">
			<div className="mx-auto flex w-full max-w-3xl flex-col gap-8 py-8 sm:py-12">
				{!daemonReady && (
					<div data-testid="enter-blocked-daemon" className="rounded-md border border-border p-4">
						<h3 className="text-sm font-medium">{t("work.enter.daemonOffline.title")}</h3>
						<p className="text-muted-foreground text-sm">{t("work.enter.daemonOffline.body")}</p>
					</div>
				)}

				{destination === "undecided" && (
					<div className="flex flex-col gap-3">
						<h2 className="text-base font-medium">{t("work.enter.chooseTitle")}</h2>
						<div className="flex flex-wrap gap-3">
							<Button data-recommended="true" onClick={() => setDestination("work")}>
								{t("work.enter.startWithWork")}
							</Button>
							{/* Equal alternative: enabled, never gated behind Work, and it
							    creates nothing until the user explicitly captures something. */}
							<Button variant="outline" onClick={() => void navigate({ to: "/home" })}>
								{t("work.enter.setUpHome")}
							</Button>
						</div>
						<p className="text-muted-foreground text-xs">{t("work.enter.recommendationNote")}</p>
					</div>
				)}

				{destination === "work" && (
					<div className="flex flex-col gap-5">
						<h2 className="text-balance text-center text-2xl font-medium leading-tight tracking-wide-sm sm:text-[28px]">{t(usesWorkLaunchMode ? "work.enter.outcomeProject" : "work.enter.selectProject")}</h2>

						{!usesWorkLaunchMode && !providerReady && (
							<div data-testid="enter-blocked-provider" className="rounded-md border border-border p-4">
								<h4 className="text-sm font-medium">{t("work.enter.providerActionRequired.title")}</h4>
								<p className="text-muted-foreground text-sm">{t("work.enter.providerActionRequired.body")}</p>
							</div>
						)}

						<ul className="grid gap-2 rounded-lg border border-border bg-card p-2 shadow-sm">
							{(projectsQuery.data ?? []).map((project) => (
								<li key={project.id}>
									{/* Selecting a project advances Enter -> Understand on the
									    same /work route; the daemon owns everything else. */}
									<button
										className="flex min-h-control-form w-full items-center gap-3 rounded-md px-3 py-2 text-left transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
										onClick={() => void navigate({ to: "/work", search: { project: project.id } })}
										type="button"
									>
										<span className="text-sm">{project.name}</span>
										<span className="ml-auto min-w-0 truncate text-muted-foreground text-xs">{project.path}</span>
									</button>
								</li>
							))}
						</ul>
						<p className="text-muted-foreground text-xs">{t("work.enter.pickProjectHint")}</p>

						{!usesPreviewWorkspaceData ? (
							<CreateProjectFlow
								mode="choose"
								idleLabel={t("work.enter.addProject")}
								onCreateProject={createProject}
								onInitializeProject={initializeProject}
							>
								{({ choosePath, disabled, error, label }) => (
									<div className="flex flex-col gap-2">
										<Button variant="outline" disabled={disabled} onClick={choosePath}>
											{label}
										</Button>
										{error && (
											<p data-testid="enter-error-folder" className="text-destructive text-sm">
												{error}
											</p>
										)}
									</div>
								)}
							</CreateProjectFlow>
						) : null}
					</div>
				)}
			</div>
		</OutcomeLifecycleShell>
	);
}
