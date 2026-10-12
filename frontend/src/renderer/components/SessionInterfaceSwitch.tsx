import { ArrowRightLeft, Clock, MessageSquare, SquareTerminal, TriangleAlert, X } from "lucide-react";
import type { ReactNode } from "react";
import type {
	SessionInterfaceMode,
	SessionInterfaceTransition,
	SessionInterfaceTransitionPolicy,
} from "../hooks/useSessionInterfaceTransition";
import {
	interfaceTransitionIsActive,
	interfaceTransitionIsCancellable,
} from "../hooks/useSessionInterfaceTransition";
import { cn } from "../lib/utils";
import { TopbarButton } from "./TopbarButton";
import { Button } from "./ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "./ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "./ui/tooltip";

function targetLabel(target: SessionInterfaceMode): string {
	return target === "chat" ? "chat UI" : "terminal UI";
}

function targetTitleLabel(target: SessionInterfaceMode): string {
	return target === "chat" ? "Chat UI" : "Terminal UI";
}

const phaseCopy: Record<SessionInterfaceTransition["phase"], string> = {
	requested: "Preparing switch…",
	preflighting: "Checking interface…",
	draining: "Waiting to switch…",
	source_stopping: "Stopping controller…",
	source_stopped: "Controller stopped…",
	target_starting: "Resuming agent…",
	activating: "Activating interface…",
	completed: "Interface switched",
	failed: "Interface switch failed",
	cancelled: "Interface switch cancelled",
	recovery_required: "Interface switch needs recovery",
};

export function SessionInterfaceSwitchButton({
	target,
	supported,
	disabledReason,
	pending,
	transition,
	cancelling,
	cancelError,
	onClick,
	onCancel,
	className,
}: {
	target: SessionInterfaceMode;
	supported: boolean;
	disabledReason?: string;
	pending?: boolean;
	transition?: SessionInterfaceTransition;
	cancelling?: boolean;
	cancelError?: string;
	onClick: () => void;
	onCancel?: () => void;
	className?: string;
}) {
	if (transition && interfaceTransitionIsActive(transition)) {
		const cancellable = interfaceTransitionIsCancellable(transition) && Boolean(onCancel);
		return (
			<div
				role="status"
				aria-live="polite"
				className={cn(
					"flex h-7 items-center gap-1.5 rounded-full bg-wd-inset pl-2.5 text-xs text-wd-secondary",
					cancellable ? "pr-0.5" : "pr-3",
					className,
				)}
				title={cancelError || `${phaseCopy[transition.phase]} Switching to ${targetTitleLabel(transition.targetMode)}.`}
			>
				{/* Live work is a static accent dot beside a specific line, never a spinner. */}
				<span aria-hidden="true" className="size-1.5 shrink-0 rounded-full bg-wd-accent" />
				<span className="whitespace-nowrap">
					{phaseCopy[transition.phase]} <span className="text-wd-primary">{targetTitleLabel(transition.targetMode)}</span>
				</span>
				{cancellable ? (
					<Button
						type="button"
						size="sm"
						variant="ghost"
						className="ml-0.5 h-6 gap-1 px-2 text-[11px] text-wd-secondary hover:text-wd-primary"
						disabled={cancelling}
						onClick={onCancel}
						aria-label={`Cancel switch to ${targetTitleLabel(transition.targetMode)}`}
					>
						<X aria-hidden="true" className="size-3" />
						{cancelling ? "Cancelling" : "Cancel"}
					</Button>
				) : null}
				{cancelError ? (
					<span role="alert" className="ml-1 whitespace-nowrap pr-1.5 text-[11px] text-critical">
						Cancel failed
					</span>
				) : null}
			</div>
		);
	}

	const label = `Switch to ${targetLabel(target)}`;
	const tooltipLabel = supported ? `${label} using this agent's native conversation` : disabledReason || label;
	const TargetIcon = target === "chat" ? MessageSquare : SquareTerminal;
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<span className="inline-flex">
					<TopbarButton
						aria-label={label}
						aria-busy={pending || undefined}
						className={className}
						disabled={!supported || pending}
						onClick={onClick}
						title={tooltipLabel}
						type="button"
						variant="icon"
					>
						<TargetIcon aria-hidden="true" className="size-icon-md" />
					</TopbarButton>
				</span>
			</TooltipTrigger>
			<TooltipContent side="bottom">{tooltipLabel}</TooltipContent>
		</Tooltip>
	);
}

export function SessionInterfaceSwitchDialog({
	open,
	target,
	waitingForInput,
	busy,
	error,
	onOpenChange,
	onChoose,
}: {
	open: boolean;
	target: SessionInterfaceMode;
	waitingForInput?: boolean;
	busy?: boolean;
	error?: string;
	onOpenChange: (open: boolean) => void;
	onChoose: (policy: SessionInterfaceTransitionPolicy) => void;
}) {
	const targetName = targetTitleLabel(target);
	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="max-w-md gap-0 overflow-hidden rounded-xl p-0">
				<DialogHeader className="hairline-b border-wd-hairline px-5 py-4">
					<DialogTitle className="wd-headline flex items-center gap-2 text-wd-primary">
						<ArrowRightLeft aria-hidden="true" className="size-4 text-wd-secondary" />
						Switch to {targetName}?
					</DialogTitle>
					<DialogDescription className="wd-body-s pt-1 text-wd-secondary">
						The same Kennel session, worktree, and agent-native conversation continue in the other interface.
						Completed messages and tool work stay in the agent's context.
					</DialogDescription>
				</DialogHeader>

				<div className="grid gap-2 px-5 py-4">
					<button
						type="button"
						className="rounded-md bg-wd-inset px-4 py-3 text-left transition-colors duration-fast hover:bg-wd-hover disabled:text-wd-disabled"
						disabled={busy}
						onClick={() => onChoose("drain")}
					>
						<span className="wd-label block text-wd-primary">Finish work, then switch</span>
						<span className="wd-caption mt-1 block text-wd-secondary">
							Wait for the running turn and anything already queued to finish. New Kennel messages wait safely
							for {targetName}.
						</span>
					</button>
					<button
						type="button"
						className="rounded-md bg-wd-inset px-4 py-3 text-left transition-colors duration-fast hover:bg-wd-hover disabled:text-wd-disabled"
						disabled={busy}
						onClick={() => onChoose("interrupt")}
					>
						<span className="wd-label block text-wd-primary">Stop now and switch</span>
						<span className="wd-caption mt-1 block text-wd-secondary">
							Cancel the running turn before switching. Files already changed remain in the worktree, but
							unfinished output and queued Chat turns are cancelled.
						</span>
					</button>
					{waitingForInput ? (
						<p className="wd-caption flex items-start gap-1.5 text-wd-primary">
							<Clock aria-hidden="true" className="mt-0.5 size-3.5 shrink-0 text-caution" />
							This turn is waiting for your input. “Finish work” will wait until you answer it; use “Stop
							now” to switch immediately.
						</p>
					) : null}
					{error ? (
						<p role="alert" className="wd-caption text-critical">
							{error}
						</p>
					) : null}
				</div>

				<DialogFooter className="hairline-t border-wd-hairline px-5 py-3">
					<Button type="button" variant="ghost" disabled={busy} onClick={() => onOpenChange(false)}>
						Keep current interface
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

export function SessionInterfaceTransitionNotice({
	transition,
	onDismiss,
}: {
	transition?: SessionInterfaceTransition;
	onDismiss: () => void;
}) {
	if (!transition || (transition.phase !== "failed" && transition.phase !== "recovery_required")) {
		return null;
	}
	return (
		<div className="absolute left-1/2 top-3 z-20 flex w-[min(34rem,calc(100%-1.5rem))] -translate-x-1/2 items-start gap-2 rounded-lg bg-wd-raised px-3 py-2.5 shadow-(--shadow-overlay)">
		<TriangleAlert aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-caution" />
		<div className="min-w-0 flex-1">
			<span className="wd-label block text-wd-primary">{phaseCopy[transition.phase]}</span>
			<p className="wd-caption mt-0.5 text-wd-secondary">
				{transition.errorDetail || "The original interface remains available. You can retry the switch."}
			</p>
		</div>
		<button
			type="button"
			className="grid size-6 shrink-0 place-items-center rounded-full text-wd-secondary transition-colors duration-fast hover:bg-wd-hover hover:text-wd-primary"
			onClick={onDismiss}
			aria-label="Dismiss interface switch message"
		>
			<X aria-hidden="true" className="size-3.5" />
		</button>
	</div>
	);
}

export function SessionInterfaceActionGroup({ children }: { children: ReactNode }) {
	return <div className="inline-flex shrink-0 items-center gap-px">{children}</div>;
}
