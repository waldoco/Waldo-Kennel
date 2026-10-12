import { useQueryClient } from "@tanstack/react-query";
import { Repeat2, TriangleAlert } from "lucide-react";
import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { clearSwitchAgentState } from "../hooks/useSwitchAgent";
import type { AgentSwitchPresentation } from "../lib/agent-switch-presentation";
import { cn } from "../lib/utils";
import { sessionIsActive, type WorkspaceSession } from "../types/workspace";
import { isRecognizedSwitchSourceHarness, SwitchAgentDialog } from "./SwitchAgentDialog";
import { TopbarButton } from "./TopbarButton";
import { Tooltip, TooltipContent, TooltipTrigger } from "./ui/tooltip";

type TerminalSwitchAgentButtonProps = {
	container: HTMLElement | null | undefined;
	onOpenChange: ((open: boolean) => void) | undefined;
	open: boolean;
	presentation?: AgentSwitchPresentation;
	session: WorkspaceSession;
	switchError: string | null;
};

export function TerminalSwitchAgentButton({
	container,
	onOpenChange,
	open,
	presentation,
	session,
	switchError,
}: TerminalSwitchAgentButtonProps) {
	const { t } = useTranslation();
	const queryClient = useQueryClient();
	const controlPresentation = presentation?.outcome === "success" ? undefined : presentation;
	const switching = controlPresentation?.outcome === "in_progress";
	const warning = controlPresentation?.outcome === "failure" || controlPresentation?.outcome === "recovery";
	const blocksNewSwitch = switching;

	useEffect(() => {
		if (switchError) onOpenChange?.(true);
	}, [onOpenChange, switchError]);

	if (
		session.kind !== "worker" ||
		session.isTerminated ||
		session.provider === "codex" ||
		!isRecognizedSwitchSourceHarness(session.provider) ||
		(!controlPresentation && !sessionIsActive(session))
	) {
		return null;
	}

	const label = controlPresentation
		? t(controlPresentation.compactLabelKey, controlPresentation.values)
		: t("switchAgent.action");
	const handleOpenChange = (nextOpen: boolean) => {
		onOpenChange?.(nextOpen);
		if (!nextOpen && switchError) {
			clearSwitchAgentState(queryClient, session.id);
		}
	};

	return (
		<>
			<Tooltip>
				<TooltipTrigger asChild>
					<TopbarButton
						aria-busy={switching && controlPresentation?.animate ? true : undefined}
						aria-label={label}
						className={cn(
							warning && "text-caution hover:bg-caution-bg hover:text-caution",
						)}
						disabled={blocksNewSwitch}
						onClick={() => {
							if (!open) handleOpenChange(true);
						}}
						onPointerDown={(event) => {
							if (!open) return;
							event.preventDefault();
							event.stopPropagation();
						}}
						type="button"
						variant="icon"
					>
						{warning ? (
							<TriangleAlert aria-hidden="true" className="size-icon-sm" />
						) : switching ? (
							// Live work is a static accent dot on the control, never a spinner;
							// the tooltip and accessible name carry the specific line.
							<span className="relative inline-flex">
								<Repeat2 aria-hidden="true" className="size-4 stroke-[1.8]" />
								<span
									aria-hidden="true"
									className="absolute -right-0.5 -top-0.5 size-1.5 rounded-full bg-wd-accent"
									data-testid="agent-switch-live-dot"
								/>
							</span>
						) : (
							<Repeat2 aria-hidden="true" className="size-4 stroke-[1.8]" />
						)}
					</TopbarButton>
				</TooltipTrigger>
				<TooltipContent>{label}</TooltipContent>
			</Tooltip>
			{open && container ? (
				<SwitchAgentDialog container={container} onOpenChange={handleOpenChange} open session={session} />
			) : null}
		</>
	);
}
