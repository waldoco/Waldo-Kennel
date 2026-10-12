import { useEffect, useRef, useState, type FocusEvent, type MouseEvent, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import {
	SessionCardView,
	SessionRowView,
	SessionUsageMetricView,
	StateLabel,
	type StateTone,
	type BoardPullRequestLabels,
	type BoardSessionPresentation,
	type BoardSplitLaneLabels,
	type BoardUsagePresentation,
	type ProductUITranslator,
} from "@pin4sf/kennel-product-ui";
import { ArrowRight, Check, Copy, GitBranch, LoaderCircle, RotateCcw, Trash2 } from "lucide-react";
import type { MessageKey } from "../i18n";
import { aoBridge } from "../lib/bridge";
import { formatTimeCompact } from "../lib/format-time";
import { formatTokenCount } from "../lib/format-token-count";
import { prBrowserUrl, sessionPRDisplaySummaries } from "../lib/pr-display";
import {
	agentSwitchStatusVisual,
	deriveSessionAgentSwitchPresentation,
} from "../lib/agent-switch-presentation";
import type { WorkspaceSession } from "../types/workspace";
import { canonicalTrackerIssueId } from "../types/workspace";
import { useSessionScmSummary } from "../hooks/useSessionScmSummary";
import type { SessionUsageSummary } from "../hooks/useSessionUsageSummaries";
import {
	clearTerminateSessionState,
	useTerminateSessionState,
} from "../hooks/useTerminateSession";
import { cn } from "../lib/utils";
import { attentionZone, getAttentionZoneViewForZone, type AttentionZone } from "../lib/session-presentation";
import { Button } from "./ui/button";
import { ProductExternalLink } from "./ProductExternalLink";
import { SessionTerminationPopover } from "./SessionTerminationPopover";
import { Popover, PopoverContent, PopoverTrigger } from "./ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "./ui/tooltip";
import waldoAgentMarkUrl from "../../../../packages/kennel-island/public/figma/agent-waldo.svg";
import artifactLinkIconUrl from "../../../../packages/kennel-island/public/figma/icon-link.svg";
import artifactProgressUrl from "../../../../packages/kennel-island/public/figma/progress-active.svg";

export function toBoardSessionPresentation(
	session: WorkspaceSession,
	t?: TFunction,
): BoardSessionPresentation {
	const switchPresentation = deriveSessionAgentSwitchPresentation(session);
	const switchVisual = switchPresentation ? agentSwitchStatusVisual(switchPresentation) : undefined;
	return {
		activity: session.activity,
		branch: session.branch,
		id: session.id,
		provider: session.provider,
		status: session.status,
		statusPresentation:
			t && switchPresentation && switchVisual
				? {
						className: switchVisual.className,
						// Waldo states never pulse: an in-flight switch is a still dot and its words.
						indicatorClassName: switchVisual.indicatorClassName,
						label: t(switchPresentation.compactLabelKey, switchPresentation.values),
					}
				: undefined,
		summary: sessionSummary(session),
		title: session.title,
		trackerIssueId: canonicalTrackerIssueId(session.issueId),
		updatedAt: session.updatedAt,
	};
}

export function sessionsBoardLabels(t: TFunction): BoardSplitLaneLabels {
	return {
		columnAria: (label) => t("shell.sessionsAria", { label }),
		countSessions: (count, label) => t("shell.countSessionsAria", { count, label }),
		idleWorkingAria: t("shell.idleWorkingSessions"),
		laneSummary: (primary, secondary) => t("shell.laneSummaryAria", { primary, secondary }),
		readyMergedAria: t("shell.readyMergedSessions"),
		tones: {
			idle: {
				label: t("status.idle"),
				countLabel: t("shell.countLabel.idle"),
				regionLabel: t("shell.idleSessions"),
			},
			working: {
				label: t("status.working"),
				countLabel: t("shell.countLabel.working"),
				regionLabel: t("shell.workingSessions"),
			},
			ready: {
				label: t("zone.merge"),
				countLabel: t("shell.countLabel.readyToMerge"),
				regionLabel: t("shell.readyToMergeSessions"),
			},
			merged: {
				label: t("status.merged"),
				countLabel: t("shell.countLabel.merged"),
				regionLabel: t("shell.mergedSessions"),
			},
		},
	};
}

export function BoardSessionCardAdapter({
	onOpen,
	onTerminate,
	session,
	usage,
}: {
	onOpen: () => void;
	onTerminate: () => void;
	session: WorkspaceSession;
	usage?: SessionUsageSummary;
}) {
	return (
		<DesktopSessionCard
			onOpen={onOpen}
			onTerminate={onTerminate}
			session={session}
			usage={usage}
		/>
	);
}

/**
 * The list view's row. It shares the card's data and labels wholesale — the two
 * views must never disagree about what a session says — and differs only in
 * layout and in using Choose to open a truthful, session-derived Waldo brief.
 */
export function BoardSessionRowAdapter({
	onOpen,
	session,
	usage,
}: {
	onOpen: () => void;
	session: WorkspaceSession;
	usage?: SessionUsageSummary;
}) {
	const { t } = useTranslation();
	const summaries = sessionPRDisplaySummaries(session, useSessionScmSummary(session.id).data);
	const prLabels = pullRequestLabels(t);
	const artifact = sessionArtifact(session, summaries, onOpen, prLabels, t);
	const termination = useTerminateSessionState(session.id);
	const translate: ProductUITranslator = (key, values) => t(key as MessageKey, values);
	const [previewOpen, setPreviewOpen] = useState(false);
	const [pinnedOpen, setPinnedOpen] = useState(false);
	const openTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
	const closeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
	const open = previewOpen || pinnedOpen;
	const zone = attentionZone(session.status);

	const cancelOpen = () => {
		if (openTimerRef.current !== null) {
			clearTimeout(openTimerRef.current);
			openTimerRef.current = null;
		}
	};
	const cancelClose = () => {
		if (closeTimerRef.current !== null) {
			clearTimeout(closeTimerRef.current);
			closeTimerRef.current = null;
		}
	};
	// A brief hover-intent delay before the preview opens: without it, simply
	// scanning down a list opens (and, via the backdrop below, darkens the
	// whole screen for) every row the pointer passes over on the way — a rapid
	// flash per row rather than a deliberate pause on one.
	const previewBrief = () => {
		cancelClose();
		if (pinnedOpen || previewOpen) return;
		cancelOpen();
		openTimerRef.current = setTimeout(() => {
			openTimerRef.current = null;
			setPreviewOpen(true);
		}, 150);
	};
	const schedulePreviewClose = () => {
		cancelOpen();
		cancelClose();
		if (pinnedOpen) return;
		closeTimerRef.current = setTimeout(() => setPreviewOpen(false), 100);
	};
	const dismissBrief = () => {
		cancelOpen();
		cancelClose();
		setPinnedOpen(false);
		setPreviewOpen(false);
	};

	useEffect(
		() => () => {
			if (openTimerRef.current !== null) clearTimeout(openTimerRef.current);
			if (closeTimerRef.current !== null) clearTimeout(closeTimerRef.current);
		},
		[],
	);

	return (
		<Popover
			onOpenChange={(nextOpen) => {
				if (!nextOpen) dismissBrief();
			}}
			open={open}
		>
			{/* The dimmed backdrop is a modal affordance — it belongs to the
			    explicitly pinned brief (a "Choose" click), never to the transient
			    hover preview. Tying it to `open` (preview included) darkened the
			    whole viewport on every row a person's pointer merely passed over
			    while scanning the list, which read as a rapid full-screen flicker. */}
			{pinnedOpen && typeof document !== "undefined"
				? createPortal(
						<div aria-hidden="true" className="pointer-events-none fixed inset-0 z-[49] bg-(--bg-scrim)" data-testid="waldo-brief-backdrop" />,
						document.body,
					)
				: null}
			<PopoverTrigger asChild>
				<div
					className={cn("relative", open && "z-[50]")}
					onBlurCapture={(event: FocusEvent<HTMLDivElement>) => {
						if (!event.currentTarget.contains(event.relatedTarget as Node | null)) schedulePreviewClose();
					}}
					onFocusCapture={previewBrief}
					onPointerEnter={previewBrief}
					onPointerLeave={schedulePreviewClose}
				>
					<SessionRowView
						action={
							<ChooseSessionButton
								onChoose={() => {
									cancelOpen();
									cancelClose();
									setPinnedOpen(true);
									setPreviewOpen(true);
								}}
								sessionTitle={session.title}
							/>
						}
						artifact={artifact}
						branchIcon={<GitBranch aria-hidden="true" className="size-icon-2xs shrink-0" />}
						error={termination.error ?? undefined}
						externalLink={ProductExternalLink}
						labels={{
							formatTime: formatTimeCompact,
							intakeIssue: (id) => t("shell.intakeIssue", { id }),
							pr: prLabels,
							updatedAt: (time) => t("shell.updatedAt", { time }),
						}}
						onOpen={onOpen}
						prs={(artifact ? [] : summaries).map((pr) => ({
							number: pr.number,
							state: pr.state,
							url: prBrowserUrl(pr),
						}))}
						renderAvatar={(provider) => <WaldoAgentMark provider={provider} />}
						renderUsage={(presentation) => <DesktopUsageMetric usage={presentation} />}
						session={toBoardSessionPresentation(session, t)}
						translate={translate}
						usage={toUsagePresentation(usage, t)}
					/>
				</div>
			</PopoverTrigger>
			<PopoverContent
				align="center"
				aria-label={t("shell.waldoBrief.aria", { title: session.title })}
				className="z-[60] w-[34rem] max-w-[calc(100vw-2rem)] rounded-lg p-4 shadow-lg"
				collisionPadding={16}
				onOpenAutoFocus={(event) => event.preventDefault()}
				onFocusCapture={previewBrief}
				onPointerEnter={previewBrief}
				onPointerLeave={schedulePreviewClose}
				role="dialog"
				side="bottom"
				sideOffset={8}
			>
				<WaldoBrief
					onDismiss={dismissBrief}
					onOpenSession={() => {
						dismissBrief();
						onOpen();
					}}
					session={session}
					summary={sessionSummary(session, summaries)}
					zone={zone}
				/>
			</PopoverContent>
		</Popover>
	);
}

function WaldoBrief({
	onDismiss,
	onOpenSession,
	session,
	summary,
	zone,
}: {
	onDismiss: () => void;
	onOpenSession: () => void;
	session: WorkspaceSession;
	summary?: string;
	zone: AttentionZone;
}) {
	const { t } = useTranslation();
	const zoneTone: Record<AttentionZone, StateTone> = {
		action: "needsYou",
		pending: "caution",
		merge: "good",
		working: "live",
		done: "neutral",
	};
	const fallbackSummaryKey: Record<AttentionZone, MessageKey> = {
		action: "shell.waldoBrief.actionSummary",
		pending: "shell.waldoBrief.pendingSummary",
		merge: "shell.waldoBrief.readySummary",
		working: "shell.waldoBrief.runningSummary",
		done: "shell.waldoBrief.doneSummary",
	};
	const recommendationKey: Record<AttentionZone, MessageKey> = {
		action: "shell.waldoBrief.actionRecommendation",
		pending: "shell.waldoBrief.pendingRecommendation",
		merge: "shell.waldoBrief.readyRecommendation",
		working: "shell.waldoBrief.runningRecommendation",
		done: "shell.waldoBrief.doneRecommendation",
	};

	// NeedsYouCard shape: the subject, what is true now, the recommendation on
	// an inset, then the actions - the quiet way out leading, the effect trailing.
	return (
		<div className="flex flex-col gap-3">
			<div className="flex flex-col gap-1">
				<span className="flex items-center gap-2 text-xs text-wd-secondary">
					<StateLabel className="font-medium" label={getAttentionZoneViewForZone(zone, t).label} tone={zoneTone[zone]} />
					<span aria-hidden="true">·</span>
					<span>{t(summary ? "shell.waldoBrief.review" : "shell.waldoBrief.status")}</span>
				</span>
				<h3 className="text-base font-medium leading-snug text-wd-primary">{session.title}</h3>
				{summary ? null : <p className="text-sm leading-5 text-wd-secondary">{t("shell.waldoBrief.noSummary")}</p>}
				<p className="text-sm leading-5 text-wd-secondary">{summary ?? t(fallbackSummaryKey[zone])}</p>
			</div>
			<div className="rounded-md bg-wd-inset px-3 py-2.5">
				<p className="text-xs font-medium text-wd-secondary">{t("shell.waldoBrief.recommendation")}</p>
				<p className="mt-1 text-sm leading-5 text-wd-primary">{t(recommendationKey[zone])}</p>
			</div>
			<div className="flex items-center justify-between gap-2">
				<Button aria-label={t("shell.waldoBrief.holdOff")} onClick={onDismiss} size="sm" variant="ghost">
					{t("shell.waldoBrief.holdOff")}
				</Button>
				<Button aria-label={t("shell.waldoBrief.openSession")} onClick={onOpenSession} size="sm" variant="primary">
					{t("shell.waldoBrief.openSession")}
					<ArrowRight aria-hidden="true" className="size-icon-sm" />
				</Button>
			</div>
		</div>
	);
}

export function ArchivedSessionCardAdapter({
	isRestoreDisabled,
	isRestoring,
	restoreAction,
	restoreError,
	session,
	usage,
}: {
	isRestoreDisabled: boolean;
	isRestoring: boolean;
	restoreAction: (event: MouseEvent<HTMLButtonElement>) => void;
	restoreError?: string;
	session: WorkspaceSession;
	usage?: SessionUsageSummary;
}) {
	const branch = session.branch ?? "";
	return (
		<DesktopSessionCard
			action={
				<ArchiveRestoreButton
					isDisabled={isRestoreDisabled}
					isRestoring={isRestoring}
					label={`Restore ${session.title}`}
					onClick={restoreAction}
				/>
			}
			branchAction={branch ? <CopyActionButton label={`branch ${branch}`} value={branch} /> : undefined}
			footer={<ArchiveRestoreError message={restoreError} />}
			interactive={false}
			session={session}
			usage={usage}
		/>
	);
}

function DesktopSessionCard({
	action,
	branchAction,
	footer,
	interactive = true,
	onOpen,
	onTerminate,
	session,
	usage,
}: {
	action?: ReactNode;
	branchAction?: ReactNode;
	footer?: ReactNode;
	interactive?: boolean;
	onOpen?: () => void;
	onTerminate?: () => void;
	session: WorkspaceSession;
	usage?: SessionUsageSummary;
}) {
	const { t } = useTranslation();
	const queryClient = useQueryClient();
	const [confirmOpen, setConfirmOpen] = useState(false);
	const summaries = sessionPRDisplaySummaries(session, useSessionScmSummary(session.id).data);
	const prLabels = pullRequestLabels(t);
	// The Figma artifact treatment belongs to the live board/list. Archived
	// cards retain their explicit PR lifecycle history and restore affordance.
	const artifact = interactive ? sessionArtifact(session, summaries, onOpen, prLabels, t) : undefined;
	const termination = useTerminateSessionState(session.id);
	const showTerminate = interactive && session.isTerminated !== true && onTerminate;
	const keepTerminateVisible = session.status === "merged";
	const usagePresentation = toUsagePresentation(usage, t);
	const translate: ProductUITranslator = (key, values) => t(key as MessageKey, values);

	const terminationOverlay = showTerminate ? (
		<SessionTerminationPopover
			onConfirm={() => {
				setConfirmOpen(false);
				onTerminate();
			}}
			onOpenChange={setConfirmOpen}
			open={confirmOpen}
			session={session}
			trigger={
				<button
					aria-label={
						termination.isPending
							? t("shell.killingNamedAria", { title: session.title })
							: t("shell.terminateNamed", { title: session.title })
					}
					className={cn(
						"absolute right-2 top-1.5 z-10 inline-flex size-control-md items-center justify-center rounded-full text-wd-tertiary transition-[color,background-color,opacity] duration-fast hover:bg-critical-bg hover:text-critical focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)",
						keepTerminateVisible || termination.isPending
							? "opacity-100"
							: "pointer-events-none opacity-0 group-hover:pointer-events-auto group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:opacity-100",
					)}
					onClick={(event) => {
						event.stopPropagation();
						clearTerminateSessionState(queryClient, session.id);
					}}
					disabled={termination.isPending}
					title={termination.isPending ? t("shell.killingSession") : t("shell.terminateSession")}
					type="button"
				>
					{termination.isPending ? (
						<LoaderCircle className="size-icon-sm animate-spin" aria-hidden="true" />
					) : (
						<Trash2 className="size-icon-sm" aria-hidden="true" />
					)}
				</button>
			}
		/>
	) : undefined;

	return (
		<SessionCardView
			action={action}
			actions={cardActions(session, summaries, t, onOpen)}
			artifact={artifact}
			branchAction={branchAction}
			branchIcon={<GitBranch aria-hidden="true" className="size-icon-2xs shrink-0" />}
			error={termination.error ?? undefined}
			externalLink={ProductExternalLink}
			footer={footer}
			interactive={interactive}
			labels={{
				formatTime: formatTimeCompact,
				intakeIssue: (id) => t("shell.intakeIssue", { id }),
				pr: prLabels,
				updatedAt: (time) => t("shell.updatedAt", { time }),
			}}
			onOpen={onOpen}
			overlay={terminationOverlay}
			prs={(artifact ? [] : summaries).map((pr) => ({
				number: pr.number,
				state: pr.state,
				url: prBrowserUrl(pr),
			}))}
			renderAvatar={(provider) => <WaldoAgentMark className="mt-0.5" provider={provider} />}
			renderUsage={(presentation) => <DesktopUsageMetric usage={presentation} />}
			session={toBoardSessionPresentation(session, t)}
			translate={translate}
			usage={usagePresentation}
		/>
	);
}

type SessionArtifactPR = ReturnType<typeof sessionPRDisplaySummaries>[number];

function sessionSummary(session: WorkspaceSession, summaries: SessionArtifactPR[] = []): string | undefined {
	const commit = session.commitMessage?.trim();
	const changedCount = session.changedFiles?.length ?? 0;
	if (commit) {
		const sentence = summarySentence(commit);
		return changedCount > 0
			? `${sentence}. ${changedCount} ${changedCount === 1 ? "file" : "files"} changed in this session.`
			: `${sentence}.`;
	}
	if (changedCount > 0) {
		return `${changedCount} ${changedCount === 1 ? "file is" : "files are"} ready to review.`;
	}

	const primaryPR = summaries[0];
	const prTitle = primaryPR?.title.trim();
	const prChangedCount = primaryPR?.changedFiles ?? 0;
	if (prTitle) {
		const sentence = summarySentence(prTitle);
		return prChangedCount > 0
			? `${sentence}. ${prChangedCount} ${prChangedCount === 1 ? "file" : "files"} changed in this pull request.`
			: `${sentence}.`;
	}
	if (prChangedCount > 0) {
		return `${prChangedCount} ${prChangedCount === 1 ? "file is" : "files are"} changed in the pull request and ready to review.`;
	}
	return undefined;
}

function summarySentence(value: string): string {
	return `${value.charAt(0).toUpperCase()}${value.slice(1)}`.replace(/[.\s]+$/, "");
}

function WaldoAgentMark({ provider, className }: { provider: string; className?: string }) {
	const { t } = useTranslation();
	return (
		<span
			aria-label={t("shell.agentAria", { provider })}
			className={cn("inline-flex h-7 w-[30px] shrink-0 items-center justify-center", className)}
			role="img"
		>
			<img alt="" aria-hidden="true" className="block h-7 w-[30px]" src={waldoAgentMarkUrl} />
		</span>
	);
}

function sessionArtifact(
	session: WorkspaceSession,
	summaries: SessionArtifactPR[],
	onOpen?: () => void,
	prLabels?: BoardPullRequestLabels,
	t?: TFunction,
): ReactNode | undefined {
	const primaryPR = summaries[0];
	if (primaryPR) {
		const label =
			primaryPR.title.trim() && primaryPR.title.trim() !== session.title.trim()
				? primaryPR.title.trim()
				: `Pull request #${primaryPR.number}`;
		return (
			<ProductExternalLink
				ariaLabel={prLabels ? `#${primaryPR.number} ${prLabels.states[primaryPR.state]}` : undefined}
				className="relative z-10 inline-flex max-w-full items-center gap-2 text-brand font-medium text-foreground"
				href={prBrowserUrl(primaryPR)}
				stopPropagation
			>
				<img alt="" aria-hidden="true" className="size-[13.6px] shrink-0" src={artifactLinkIconUrl} />
				<span className="min-w-0 truncate underline decoration-link underline-offset-2">{label}</span>
				<img alt="" aria-hidden="true" className="size-[24.3px] shrink-0" src={artifactProgressUrl} />
			</ProductExternalLink>
		);
	}

	const changedPath = session.changedFiles?.[0]?.path;
	if (!changedPath) return undefined;
	const label = changedPath.split("/").at(-1) || changedPath;
	return (
		<button
			aria-label={t?.("shell.openArtifactInSession", { artifact: label, title: session.title })}
			className="relative z-10 inline-flex max-w-full items-center gap-2 text-brand font-medium text-foreground"
			onClick={(event) => {
				event.stopPropagation();
				onOpen?.();
			}}
			type="button"
		>
			<img alt="" aria-hidden="true" className="size-[13.6px] shrink-0" src={artifactLinkIconUrl} />
			<span className="min-w-0 truncate underline decoration-link underline-offset-2">{label}</span>
			<img alt="" aria-hidden="true" className="size-[24.3px] shrink-0" src={artifactProgressUrl} />
		</button>
	);
}

function ChooseSessionButton({ onChoose, sessionTitle }: { onChoose: () => void; sessionTitle: string }) {
	const { t } = useTranslation();
	return (
		<button
			aria-label={t("shell.chooseSession", { title: sessionTitle })}
			className="relative z-10 inline-flex h-[30px] min-w-[72px] items-center justify-center rounded-full bg-wd-inset px-3 text-sm font-medium text-wd-primary transition-colors duration-fast hover:bg-wd-pressed focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)"
			onClick={(event) => {
				event.stopPropagation();
				onChoose();
			}}
			type="button"
		>
			{t("shell.choose")}
		</button>
	);
}

function cardActions(
	session: WorkspaceSession,
	summaries: SessionArtifactPR[],
	t: TFunction,
	onOpen?: () => void,
): ReactNode | undefined {
	if (session.status !== "approved" && session.status !== "mergeable") return undefined;
	const primaryPR = summaries[0];
	return (
		<>
			{onOpen ? (
				<button
					className="relative z-10 inline-flex h-7 items-center justify-center rounded-full bg-wd-inverse px-3 text-xs font-medium text-wd-on-inverse transition-opacity duration-fast hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)"
					onClick={(event) => {
						event.stopPropagation();
						onOpen();
					}}
					type="button"
				>
					{t("shell.instruct")}
				</button>
			) : null}
			{primaryPR ? (
				<ProductExternalLink
					className="relative z-10 inline-flex h-7 items-center justify-center rounded-full bg-wd-inset px-3 text-xs font-medium text-wd-primary transition-colors duration-fast hover:bg-wd-pressed focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)"
					href={prBrowserUrl(primaryPR)}
					stopPropagation
				>
					{t("pr.merge.action")}
				</ProductExternalLink>
			) : null}
		</>
	);
}

function pullRequestLabels(t: TFunction): BoardPullRequestLabels {
	return {
		short: t("pr.short"),
		states: {
			closed: t("pr.state.closed"),
			draft: t("pr.state.draft"),
			merged: t("pr.state.merged"),
			open: t("pr.state.open"),
		},
	};
}

function toUsagePresentation(
	usage: SessionUsageSummary | undefined,
	t: TFunction,
): BoardUsagePresentation | undefined {
	if (!usage || usage.totalTokens <= 0) return undefined;
	return {
		accessibleLabel: t("shell.usageTokens", {
			count: usage.totalTokens.toLocaleString("en-US"),
		}),
		compactLabel: formatTokenCount(usage.totalTokens),
	};
}

function DesktopUsageMetric({ usage }: { usage: BoardUsagePresentation }) {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<SessionUsageMetricView usage={usage} />
			</TooltipTrigger>
			<TooltipContent side="top">{usage.accessibleLabel}</TooltipContent>
		</Tooltip>
	);
}

function ArchiveRestoreButton({
	label,
	onClick,
	isRestoring,
	isDisabled,
}: {
	label: string;
	onClick: (event: MouseEvent<HTMLButtonElement>) => void;
	isRestoring: boolean;
	isDisabled: boolean;
}) {
	const { t } = useTranslation();
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<button
					aria-label={label}
					className="grid size-control-board-sm shrink-0 place-items-center rounded-full text-wd-secondary transition-colors duration-fast hover:bg-wd-hover hover:text-wd-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus) disabled:cursor-not-allowed disabled:text-wd-disabled"
					disabled={isDisabled}
					onClick={onClick}
					type="button"
				>
					<RotateCcw className={cn("size-icon-md", isRestoring && "animate-spin")} aria-hidden="true" />
				</button>
			</TooltipTrigger>
			<TooltipContent side="top">
				{isRestoring ? t("shell.restoringSession") : t("shell.restoreSession")}
			</TooltipContent>
		</Tooltip>
	);
}

function ArchiveRestoreError({ message }: { message?: string }) {
	return message ? (
		<div className="hairline-t border-wd-hairline px-2 py-1.5 text-xs text-critical" role="alert">
			{message}
		</div>
	) : null;
}

function CopyActionButton({ label, value }: { label: string; value: string }) {
	const [copied, setCopied] = useState(false);
	const copiedTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
	useEffect(
		() => () => {
			if (copiedTimeoutRef.current !== null) clearTimeout(copiedTimeoutRef.current);
		},
		[],
	);
	const buttonLabel = copied ? `Copied ${label}` : `Copy ${label}`;
	const copyValue = async (event: MouseEvent<HTMLButtonElement>) => {
		event.stopPropagation();
		try {
			await aoBridge.clipboard.writeText(value);
		} catch {
			return;
		}
		setCopied(true);
		if (copiedTimeoutRef.current !== null) clearTimeout(copiedTimeoutRef.current);
		copiedTimeoutRef.current = setTimeout(() => {
			setCopied(false);
			copiedTimeoutRef.current = null;
		}, 1_500);
	};
	return (
		<button
			aria-label={buttonLabel}
			className="inline-flex size-4 shrink-0 items-center justify-center rounded-xs text-wd-tertiary transition-colors duration-fast hover:bg-wd-hover hover:text-wd-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)"
			onClick={(event) => void copyValue(event)}
			title={buttonLabel}
			type="button"
		>
			{copied ? (
				<Check className="size-icon-2xs text-good" aria-hidden="true" />
			) : (
				<Copy className="size-icon-2xs" aria-hidden="true" />
			)}
		</button>
	);
}
