import { useQuery, useQueryClient } from "@tanstack/react-query";
import { LoaderCircle, Repeat2, TriangleAlert, X } from "lucide-react";
import { type FormEvent, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
	agentSwitchesQueryKey,
	agentSwitchNeedsRecovery,
	agentSwitchNeedsSourceStopRecovery,
	agentSwitchNeedsSourceRestore,
	isTerminalAgentSwitch,
} from "../hooks/useAgentSwitches";
import { agentsQueryOptions } from "../hooks/useAgentsQuery";
import {
	createSwitchAgentIdempotencyKey,
	clearSwitchAgentState,
	type SwitchAgentHarness,
	useSwitchAgent,
	useRecoverAgentSwitch,
	useSwitchAgentState,
} from "../hooks/useSwitchAgent";
import { workspaceQueryKey } from "../hooks/useWorkspaceQuery";
import { agentLabel } from "../lib/agent-options";
import {
	admitsRole,
	buildRankedAgentOptions,
	singleReadyProvider,
	type RankedAgentOption,
} from "../lib/agent-select-options";
import type { WorkspaceSession } from "../types/workspace";
import { AgentAvatar } from "./AgentAvatar";
import { AgentModelPicker } from "./AgentModelPicker";
import { AgentSelectMenuItem } from "./settings/AgentSelectMenuItem";
import { SettingsOptionMenu } from "./settings/SettingsOptionMenu";
import { Button } from "./ui/button";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogTitle,
} from "./ui/dialog";

export const SWITCH_AGENT_OPTIONS = ["codex", "claude-code", "opencode"] as const satisfies ReadonlyArray<SwitchAgentHarness>;

export function isRecognizedSwitchSourceHarness(value: string): boolean {
	return value === "codex" || value === "claude-code" || value === "opencode";
}

export function isSelectableSwitchTargetHarness(value: string): value is SwitchAgentHarness {
	return value === "codex" || value === "claude-code" || value === "opencode";
}

function SwitchTargetPicker({
	currentHarness,
	disabled,
	onChange,
	options,
	value,
}: {
	currentHarness: string;
	disabled: boolean;
	onChange: (value: SwitchAgentHarness) => void;
	options: RankedAgentOption[];
	value: SwitchAgentHarness | "";
}) {
	const { t } = useTranslation();
	const menuOptions = options.map((option) => ({
		value: option.id,
		label: option.label,
		disabled: option.disabled || option.id === currentHarness,
	}));
	const selected = options.find((option) => option.id === value);
	return (
		<SettingsOptionMenu
			aria-label={t("switchAgent.targetLabel")}
			disabled={disabled}
			menuAlign="start"
			menuClassName="settings-agent-menu-surface"
			menuItemClassName="settings-agent-menu-item"
			onChange={(nextValue) => {
				if (isSelectableSwitchTargetHarness(nextValue) && nextValue !== currentHarness) onChange(nextValue);
			}}
			options={menuOptions}
			placeholder={t("switchAgent.selectTarget", { defaultValue: "Select agent" })}
			renderMenuItem={(option, selectedOption) => {
				const candidate = options.find((entry) => entry.id === option.value);
				if (!candidate) return option.label;
				const current = candidate.id === currentHarness;
				return (
					<AgentSelectMenuItem
						agentId={candidate.id}
						label={candidate.label}
						selected={selectedOption}
						status={current ? t("switchAgent.current") : candidate.status}
						statusTone={current ? "muted" : candidate.statusTone}
						disabled={option.disabled}
					/>
				);
			}}
			renderTrigger={(_selected, placeholder) => (
				<span className="flex min-w-0 items-center gap-2">
					{selected ? <AgentAvatar className="size-icon-base" decorative provider={selected.id} /> : null}
					<span className="min-w-0 truncate text-sm text-wd-primary" title={selected?.label ?? placeholder}>
						{selected?.label ?? placeholder}
					</span>
				</span>
			)}
			triggerClassName="composer-chip composer-toolbar-option w-full justify-between"
			value={value}
		/>
	);
}

type SwitchAgentDialogProps = {
	container: HTMLElement;
	open: boolean;
	session: WorkspaceSession;
	onOpenChange: (open: boolean) => void;
};

export function SwitchAgentDialog({ container, open, session, onOpenChange }: SwitchAgentDialogProps) {
	const { t } = useTranslation();
	const queryClient = useQueryClient();
	const agentsQuery = useQuery({ ...agentsQueryOptions, enabled: open });
	const agentInventory = agentsQuery.data;
	const switchOptions = useMemo(
		() =>
			buildRankedAgentOptions({
				supported: agentInventory?.supported ?? [],
				installed: agentInventory?.installed ?? [],
				authorized: agentInventory?.authorized ?? [],
				fallbackAgents: [],
				filter: (candidate) => admitsRole(candidate, "switchTarget"),
			}),
		[agentInventory?.authorized, agentInventory?.installed, agentInventory?.supported],
	);
	const readyOtherTargets = useMemo(
		() => switchOptions.filter((option) => option.id !== session.provider),
		[session.provider, switchOptions],
	);
	const defaultTarget = singleReadyProvider(readyOtherTargets);
	const [targetHarness, setTargetHarness] = useState<SwitchAgentHarness | "">("");
	const [model, setModel] = useState("");
	const [mode, setMode] = useState("");
	const [modelWarning, setModelWarning] = useState<string | undefined>();
	const switchAgent = useSwitchAgent();
	const recoverAgentSwitch = useRecoverAgentSwitch();
	const switchMutation = useSwitchAgentState(session.id);
	const admissionPending = switchMutation.isPending;
	const durableSwitch = session.activeAgentSwitch;
	const recoveryRequired = durableSwitch ? agentSwitchNeedsRecovery(durableSwitch) : false;
	const sourceStopRecoveryRequired = durableSwitch ? agentSwitchNeedsSourceStopRecovery(durableSwitch) : false;
	const sourceRestoreRequired = durableSwitch ? agentSwitchNeedsSourceRestore(durableSwitch) : false;
	const sourceRecoveryRequired = sourceStopRecoveryRequired || sourceRestoreRequired;
	const sourceLabel = durableSwitch ? agentLabel(durableSwitch.fromHarness) : agentLabel(session.provider);
	const recoveryTitleKey = sourceStopRecoveryRequired
		? "switchAgent.sourceStopRecovery.title"
		: sourceRestoreRequired
			? "switchAgent.sourceRecovery.title"
			: "switchAgent.recovery.title";
	const recoveryDescriptionKey = sourceStopRecoveryRequired
		? "switchAgent.sourceStopRecovery.description"
		: sourceRestoreRequired
			? "switchAgent.sourceRecovery.description"
			: "switchAgent.recovery.description";
	const sourceRecoveryActionKey = sourceStopRecoveryRequired
		? recoverAgentSwitch.isPending
			? "switchAgent.sourceStopRecovery.checking"
			: "switchAgent.sourceStopRecovery.action"
		: recoverAgentSwitch.isPending
			? "switchAgent.sourceRecovery.restoring"
			: "switchAgent.sourceRecovery.action";
	const durableSwitching = Boolean(durableSwitch && !isTerminalAgentSwitch(durableSwitch) && !recoveryRequired);
	const [refreshingRecovery, setRefreshingRecovery] = useState(false);
	const operationPending = admissionPending || recoverAgentSwitch.isPending;

	useEffect(() => {
		setTargetHarness(isSelectableSwitchTargetHarness(defaultTarget) ? defaultTarget : "");
		setModel("");
		setMode("");
		setModelWarning(undefined);
	}, [defaultTarget, session.provider]);
	useEffect(() => {
		if (open && durableSwitching) onOpenChange(false);
	}, [durableSwitching, onOpenChange, open]);

	const clearFailedAttempt = () => {
		if (!switchMutation.error) return;
		clearSwitchAgentState(queryClient, session.id);
	};

	const changeTarget = (nextTarget: SwitchAgentHarness) => {
		clearFailedAttempt();
		setTargetHarness(nextTarget);
		setModel("");
		setMode("");
		setModelWarning(undefined);
	};

	const submit = (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		if (admissionPending || durableSwitching || recoveryRequired || !targetHarness) return;
		switchAgent.mutate(
			{
				session,
				targetHarness,
				model: model.trim() || mode.trim(),
				idempotencyKey: createSwitchAgentIdempotencyKey(),
			},
			{ onSuccess: () => onOpenChange(false) },
		);
	};

	const error = switchMutation.error;
	const refreshRecovery = async () => {
		setRefreshingRecovery(true);
		try {
			await Promise.all([
				queryClient.invalidateQueries({ queryKey: agentSwitchesQueryKey(session.id) }),
				queryClient.invalidateQueries({ queryKey: workspaceQueryKey }),
			]);
		} finally {
			setRefreshingRecovery(false);
		}
	};

	return (
		<Dialog
			modal={false}
			onOpenChange={(nextOpen) => {
				if (!nextOpen && operationPending) return;
				onOpenChange(nextOpen);
			}}
			open={open}
		>
			<DialogContent
				portalContainer={container}
				overlay={
					<div
						aria-hidden="true"
						className="agent-switch-terminal-scrim absolute inset-0 z-20 animate-overlay-in motion-reduce:animate-none"
						data-testid="switch-agent-terminal-backdrop"
					/>
				}
				showCloseButton={false}
				className="absolute left-1/2 top-1/2 z-overlay w-[min(var(--size-dialog-md),calc(100%-var(--space-8)))] max-w-none -translate-x-1/2 -translate-y-1/2 gap-0 overflow-hidden rounded-xl border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-raised p-0 text-wd-primary shadow-[var(--shadow-overlay)] data-[state=open]:animate-modal-in data-[state=closed]:animate-modal-out motion-reduce:animate-none"
			>
				<DialogClose asChild>
					<button
						aria-label={t("switchAgent.close")}
						className="settings-dialog-close-button settings-close-button"
						disabled={operationPending}
						type="button"
					>
						<X className="size-icon-base" aria-hidden="true" />
					</button>
				</DialogClose>
				<DialogTitle className="settings-dialog-title px-6 pr-14 pt-6">{t("switchAgent.title")}</DialogTitle>
				<DialogDescription className="wd-body-s px-6 pr-14 pt-1 text-wd-secondary">
					{t("switchAgent.description", { current: agentLabel(session.provider) })}
				</DialogDescription>

				{recoveryRequired ? (
					<div className="flex flex-col gap-4 px-6 pb-6 pt-4">
						<div className="flex items-start gap-3 rounded-md bg-caution-bg px-4 py-3">
							<TriangleAlert aria-hidden="true" className="mt-0.5 size-5 shrink-0 text-caution" />
							<div className="min-w-0">
								<p className="wd-label text-wd-primary">{t(recoveryTitleKey, { source: sourceLabel })}</p>
								<p className="wd-body-s mt-1 text-wd-secondary">{t(recoveryDescriptionKey, { source: sourceLabel })}</p>
								{sourceRecoveryRequired && recoverAgentSwitch.error instanceof Error ? (
									<p className="wd-body-s mt-2 text-critical" role="alert">{recoverAgentSwitch.error.message}</p>
								) : null}
							</div>
						</div>
						{sourceRecoveryRequired && durableSwitch ? (
							<Button
								className="self-end"
								disabled={recoverAgentSwitch.isPending}
								onClick={() => recoverAgentSwitch.mutate({ sessionId: session.id, switchId: durableSwitch.id })}
								type="button"
								variant="secondary"
							>
								{t(sourceRecoveryActionKey, { source: sourceLabel })}
							</Button>
						) : (
							<Button
								className="self-end"
								disabled={refreshingRecovery}
								onClick={() => void refreshRecovery()}
								type="button"
								variant="secondary"
							>
								{t("settings.project.refresh")}
							</Button>
						)}
					</div>
				) : (
					<form className="flex flex-col gap-3 px-6 pb-6 pt-4" onSubmit={submit}>
						{error || modelWarning ? (
							<div>
								{error ? <p className="wd-body-s text-critical" role="alert">{error}</p> : null}
								{!error && modelWarning ? <p className="wd-body-s text-caution">{modelWarning}</p> : null}
							</div>
						) : null}

						{/*
						  An empty target list is not the same as "no switch is possible", and a
						  disabled picker with nothing in it says neither. Name the reason: the
						  catalog failed to load, or it loaded and no other provider can continue
						  this conversation.
						*/}
						{!agentsQuery.isFetching && readyOtherTargets.length === 0 ? (
							<p className="wd-body-s text-caution" role="status">
								{agentsQuery.isError
									? t("switchAgent.catalogUnavailable")
									: t("switchAgent.noContinuationTargets")}
							</p>
						) : null}

						<div className="composer-toolbar p-0!">
							<div className="composer-run-controls" role="group" aria-label={t("newTask.runsWith")}>
								<div className="composer-toolbar-slot">
									<SwitchTargetPicker
										currentHarness={session.provider}
										disabled={admissionPending || agentsQuery.isFetching}
										onChange={changeTarget}
										options={switchOptions}
										value={targetHarness}
									/>
								</div>
								<span className="composer-toolbar-divider" aria-hidden="true" />
								<div className="composer-toolbar-slot">
									{targetHarness ? (
										<AgentModelPicker
											agentId={targetHarness}
											agentLabel={agentLabel(targetHarness)}
											disabled={admissionPending}
											mode={mode}
											onModeChange={(value) => {
												clearFailedAttempt();
												setMode(value);
												setModel("");
											}}
											onModelChange={(value) => {
												clearFailedAttempt();
												setModel(value);
												setMode("");
											}}
											onWarningChange={setModelWarning}
											projectId={session.workspaceId}
											value={model}
										/>
									) : (
										<button
											aria-label={t("newTask.model")}
											className="composer-chip composer-toolbar-option w-full justify-between text-wd-disabled"
											disabled
											type="button"
										>
											<span>{t("newTask.model")}</span>
										</button>
									)}
								</div>
							</div>
							<Button
								aria-label={admissionPending ? t("newTask.starting") : t("switchAgent.confirm")}
								className="size-(--size-settings-action-height)"
								disabled={admissionPending || !targetHarness}
								size="none"
								title={admissionPending ? t("newTask.starting") : t("switchAgent.confirm")}
								type="submit"
								variant="primary"
							>
								{admissionPending ? <LoaderCircle className="size-icon-base animate-spin" aria-hidden="true" /> : <Repeat2 className="size-4 stroke-[1.8]" aria-hidden="true" />}
							</Button>
						</div>
					</form>
				)}
			</DialogContent>
		</Dialog>
	);
}
