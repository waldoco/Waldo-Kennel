import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import { agentsQueryKey, refreshAgentsIfStale, useAgentsQuery } from "../../hooks/useAgentsQuery";
import { useProjectAgentRoles } from "../../hooks/useProjectAgentRoles";
import { useWorkspaceQuery } from "../../hooks/useWorkspaceQuery";
import { spawnOrchestrator } from "../../lib/spawn-orchestrator";
import { newestActiveOrchestrator } from "../../types/workspace";
import { useQueryClient } from "@tanstack/react-query";
import { ConfirmDialog } from "../ConfirmDialog";
import { RequiredAgentField } from "../CreateProjectAgentSheet";

/**
 * Worker and orchestrator pickers docked in the Outcome intake composer.
 *
 * These are NOT a third copy of agent selection: they read and write the same
 * durable `config.worker.agent` / `config.orchestrator.agent` fields Project
 * Settings owns, through the same endpoint, so the two surfaces cannot drift.
 * Putting them beside the submit button answers "who will do this?" at the
 * moment the person is deciding what to ask for, instead of sending them to
 * Settings and back.
 *
 * Role admission comes from the daemon's inventory (`roles.worker`,
 * `roles.coordinator`) — never from provider names — matching the rule the
 * settings form already follows.
 */
export function OutcomeIntakeAgentRoles({ projectId }: { projectId: string }) {
	const { t } = useTranslation();
	const queryClient = useQueryClient();
	const roles = useProjectAgentRoles(projectId);
	const agentsQuery = useAgentsQuery();
	const workspaceQuery = useWorkspaceQuery();
	const catalog = agentsQuery.data;

	// An agent installed or authenticated after the daemon booted stays
	// invisible until something re-probes, and this is a surface that asks the
	// person to pick one. Throttled inside the helper.
	useEffect(() => {
		void refreshAgentsIfStale().then((next) => {
			if (next) queryClient.setQueryData(agentsQueryKey, next);
		});
	}, [queryClient]);

	const supported = catalog?.supported ?? [];
	const coordinatorCapable = new Set(
		supported.filter((agent) => agent.roles?.coordinator).map((agent) => agent.id),
	);
	const workerCapable = new Set(supported.filter((agent) => agent.roles?.worker).map((agent) => agent.id));

	const workspace = workspaceQuery.data?.find((item) => item.id === projectId);
	const activeOrchestrator = newestActiveOrchestrator(workspace?.sessions ?? []);

	// Replacing a running orchestrator tears down a live session, so the
	// pending pick is held here until the person confirms it — a dropdown is a
	// much lighter gesture than saving a settings form, which is where the
	// same replacement otherwise happens.
	const [pendingOrchestrator, setPendingOrchestrator] = useState<string | null>(null);
	const [replacing, setReplacing] = useState(false);
	const [replacementError, setReplacementError] = useState<string | null>(null);

	if (!roles.available) return null;

	async function chooseOrchestrator(agentId: string) {
		if (agentId === roles.orchestrator) return;
		if (activeOrchestrator) {
			setReplacementError(null);
			setPendingOrchestrator(agentId);
			return;
		}
		await roles.setRole("orchestrator", agentId).catch(() => undefined);
	}

	async function confirmOrchestrator() {
		if (!pendingOrchestrator) return;
		setReplacing(true);
		setReplacementError(null);
		try {
			await roles.setRole("orchestrator", pendingOrchestrator);
			// The config write alone would leave the running session on the old
			// agent, so the replacement is part of the same confirmed action.
			await spawnOrchestrator(projectId, "outcome_intake", true);
			setPendingOrchestrator(null);
		} catch (cause) {
			setReplacementError(cause instanceof Error ? cause.message : t("outcome.intake.roles.replaceFailed"));
		} finally {
			setReplacing(false);
		}
	}

	const disabled = roles.saving || replacing;

	return (
		<>
			<div className="flex min-w-0 items-center gap-2.5" data-testid="intake-agent-roles">
				{/* Two chips that differ only by the agent inside them would be
				    unreadable, so each carries its role in plain text rather than
				    hiding it in a tooltip or an icon the reader has to learn. */}
				<span aria-hidden="true" className="text-xs text-wd-secondary">
					{t("outcome.intake.roles.workerShort")}
				</span>
				<RequiredAgentField
					authorized={catalog?.authorized}
					disabled={disabled}
					id="intake-worker-agent"
					installed={catalog?.installed}
					label={t("outcome.intake.roles.worker")}
					onChange={(agentId) => void roles.setRole("worker", agentId).catch(() => undefined)}
					placeholder={t("outcome.intake.roles.workerPlaceholder")}
					selectableIds={workerCapable.size > 0 ? workerCapable : undefined}
					supported={catalog?.supported}
					triggerClassName="h-7 w-auto gap-1.5 px-2"
					value={roles.worker}
					variant="chip"
				/>
				<span aria-hidden="true" className="text-xs text-wd-secondary">
					{t("outcome.intake.roles.orchestratorShort")}
				</span>
				<RequiredAgentField
					authorized={catalog?.authorized}
					disabled={disabled}
					id="intake-orchestrator-agent"
					installed={catalog?.installed}
					label={t("outcome.intake.roles.orchestrator")}
					onChange={(agentId) => void chooseOrchestrator(agentId)}
					placeholder={t("outcome.intake.roles.orchestratorPlaceholder")}
					selectableIds={coordinatorCapable.size > 0 ? coordinatorCapable : undefined}
					supported={catalog?.supported}
					triggerClassName="h-7 w-auto gap-1.5 px-2"
					value={roles.orchestrator}
					variant="chip"
				/>
			</div>
			{roles.saveError && !pendingOrchestrator ? (
				<p className="sr-only" role="alert">
					{roles.saveError}
				</p>
			) : null}
			<ConfirmDialog
				busy={replacing}
				confirmLabel={t("outcome.intake.roles.replaceConfirm")}
				description={t("outcome.intake.roles.replaceBody", {
					agent:
						supported.find((agent) => agent.id === pendingOrchestrator)?.label ?? pendingOrchestrator ?? "",
				})}
				error={replacementError}
				onConfirm={() => void confirmOrchestrator()}
				onOpenChange={(open) => {
					if (open) return;
					setPendingOrchestrator(null);
					setReplacementError(null);
				}}
				open={pendingOrchestrator !== null}
				title={t("outcome.intake.roles.replaceTitle")}
			/>
		</>
	);
}
