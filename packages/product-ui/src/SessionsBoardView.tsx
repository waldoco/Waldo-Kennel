import {
	Fragment,
	forwardRef,
	memo,
	startTransition,
	useEffect,
	useState,
	type HTMLAttributes,
	type ReactElement,
	type ReactNode,
} from "react";
import { motion, useReducedMotion } from "motion/react";
import type { ExternalLinkComponent } from "./external-link";
import { ChevronIcon, GitBranchIcon } from "./icons";
import { StateMark, type StateTone } from "./StateBadge";
import {
	attentionZone,
	getAgentActivityView,
	getSessionStatusView,
	type AttentionZone,
	type AttentionZoneView,
	type ProductUITranslator,
} from "./session-presentation";
import type { SessionActivity, SessionStatus } from "./session-models";
import { cn } from "./utils";

export type BoardSessionPresentation = {
	activity?: SessionActivity;
	branch?: string;
	id: string;
	provider: string;
	status: SessionStatus;
	statusPresentation?: BoardSessionStatusPresentation;
	/** One or two lines on what the agent is actually doing inside the session.
	 *  Optional: a card without one closes up rather than reserving empty space. */
	summary?: string;
	title: string;
	trackerIssueId?: string;
	updatedAt: string;
};

export type BoardSessionStatusPresentation = {
	className: string;
	indicatorClassName: string;
	label: string;
};

export type BoardPullRequestState = "closed" | "open" | "draft" | "merged";

export type BoardPullRequestPresentation = {
	number: number;
	state: BoardPullRequestState;
	url: string;
};

export type BoardUsagePresentation = {
	accessibleLabel: string;
	compactLabel: string;
};

export type BoardPullRequestLabels = {
	short: string;
	states: Record<BoardPullRequestState, string>;
};

export type BoardSplitLaneLabels = {
	columnAria: (label: string) => string;
	countSessions: (count: number, label: string) => string;
	idleWorkingAria: string;
	laneSummary: (primary: string, secondary: string) => string;
	readyMergedAria: string;
	tones: {
		idle: BoardSplitLaneToneLabels;
		merged: BoardSplitLaneToneLabels;
		ready: BoardSplitLaneToneLabels;
		working: BoardSplitLaneToneLabels;
	};
};

export type BoardSplitLaneToneLabels = {
	countLabel: string;
	label: string;
	regionLabel: string;
};

export type SessionsBoardGridViewProps<
	TSession extends BoardSessionPresentation = BoardSessionPresentation,
> = {
	columns: AttentionZoneView[];
	labels: BoardSplitLaneLabels;
	renderSessionCard: (session: TSession) => ReactNode;
	sessions: TSession[];
};

export function SessionsBoardGridView<TSession extends BoardSessionPresentation>({
	columns,
	labels,
	renderSessionCard,
	sessions,
}: SessionsBoardGridViewProps<TSession>) {
	const byZone = new Map<AttentionZone, TSession[]>();
	for (const session of sessions) {
		const zone = attentionZone(session.status);
		const sessionsForZone = byZone.get(zone);
		if (sessionsForZone) sessionsForZone.push(session);
		else byZone.set(zone, [session]);
	}

	return (
		<div
			className="board-horizontal-scrollbar h-full overflow-x-auto overflow-y-hidden"
			data-testid="board-horizontal-scroll"
		>
			{/* At the 1,455px desktop frame, the board surface is 1,184px wide:
			    10px side insets around four 285px lanes separated by exact 8px
			    gutters. The minimum keeps those proportions before horizontal scroll
			    takes over; wider frames continue to share the available width. */}
			<div className="relative grid h-full min-w-[74rem] grid-cols-4 gap-2 px-2.5 pb-2.5 xl:min-w-0">
				{columns.map((column) => (
					<BoardColumnView
						column={column}
						key={column.zone}
						labels={labels}
						renderSessionCard={renderSessionCard}
						sessions={byZone.get(column.zone) ?? []}
					/>
				))}
			</div>
		</div>
	);
}

function BoardColumnView<TSession extends BoardSessionPresentation>({
	column,
	labels,
	renderSessionCard,
	sessions,
}: {
	column: AttentionZoneView;
	labels: BoardSplitLaneLabels;
	renderSessionCard: (session: TSession) => ReactNode;
	sessions: TSession[];
}) {
	return (
		<section
			aria-label={labels.columnAria(column.label)}
			className={boardLaneShellClassName}
			data-testid="board-column"
			data-column={column.zone}
		>
			<BoardLaneHeader
				count={sessions.length}
				countLabel={labels.countSessions(sessions.length, column.label)}
				label={column.label}
				tone={column.stateTone}
			/>
			<div className="board-scrollbar min-h-0 flex-1 overflow-y-auto">
				<div className="flex min-h-full flex-col gap-2">
					{sessions.map((session) => (
						<Fragment key={session.id}>{renderSessionCard(session)}</Fragment>
					))}
				</div>
			</div>
		</section>
	);
}

/**
 * The plate a lane's cards sit on: the shell tone, radius-lg, flat. No glow
 * gradient and no coloured top - the lane's state lives in its heading mark.
 */
const boardLaneShellClassName = "flex min-w-0 flex-col gap-2 overflow-hidden rounded-lg bg-shell p-1.5";

/**
 * Lane heading: the state's shape cue (Needs you / Running dot, Ready check,
 * In review clock), the lane word in sentence case, and a quiet tabular count.
 */
function BoardLaneHeader({
	count,
	countLabel,
	label,
	tone,
	menu,
}: {
	count: number;
	countLabel?: string;
	label: string;
	tone: StateTone;
	menu?: ReactNode;
}) {
	return (
		<div className="flex shrink-0 items-center justify-between gap-2 px-2.5 py-1.5">
			<div className="flex min-w-0 items-center gap-2">
				<StateMark className={laneMarkClassName(tone)} tone={tone} />
				<span className="truncate text-sm font-medium text-wd-primary">{label}</span>
			</div>
			<div className="flex shrink-0 items-center gap-1">
				<span aria-label={countLabel} className="text-[13px] leading-[18px] tabular-nums text-wd-tertiary">
					{count}
				</span>
				{menu}
			</div>
		</div>
	);
}

/** Glyph marks take their state colour; dots carry their own fill. */
export function laneMarkClassName(tone: StateTone): string | undefined {
	if (tone === "good") return "text-good";
	if (tone === "caution") return "text-caution";
	if (tone === "critical") return "text-critical";
	return undefined;
}

export type SessionCardViewProps = {
	action?: ReactNode;
	/** Decisions the card can carry inline (merge, instruct, pause). Rendered as
	 *  the card's last row so the eye reaches them after the context, never before. */
	actions?: ReactNode;
	/** Optional primary work artifact (document, preview, build, or result).
	 *  It sits beside usage in the evidence row shown in the Figma board. */
	artifact?: ReactNode;
	branchAction?: ReactNode;
	branchIcon?: ReactNode;
	error?: string;
	externalLink: ExternalLinkComponent;
	footer?: ReactNode;
	interactive?: boolean;
	labels: {
		formatTime: (timestamp: string) => string;
		intakeIssue: (id: string) => string;
		pr: BoardPullRequestLabels;
		updatedAt: (timestamp: string) => string;
	};
	onOpen?: () => void;
	overlay?: ReactNode;
	prs?: BoardPullRequestPresentation[];
	renderAvatar: (provider: string) => ReactNode;
	renderUsage?: (usage: BoardUsagePresentation) => ReactNode;
	session: BoardSessionPresentation;
	translate?: ProductUITranslator;
	usage?: BoardUsagePresentation;
};

export function SessionCardView({
	action,
	actions,
	artifact,
	branchAction,
	branchIcon,
	error,
	externalLink,
	footer,
	interactive = true,
	labels,
	onOpen,
	overlay,
	prs = [],
	renderAvatar,
	renderUsage = (usage) => <SessionUsageMetricView usage={usage} />,
	session,
	translate,
	usage,
}: SessionCardViewProps) {
	const badge = getSessionStatusView(session.status, translate);
	const activity = getAgentActivityView(session.activity, translate);
	const showLiveActivity = session.status === "working" && activity.state === "active";
	const statusPresentation = session.statusPresentation;
	const branch = session.branch ?? "";
	const showBranch = branch !== "" && !sameLabel(branch, session.title) && !sameLabel(branch, session.id);

	return (
		<div
			onClick={interactive ? onOpen : undefined}
			role={interactive ? undefined : "listitem"}
			className={cn(
				"group relative flex w-full flex-col gap-4 rounded-lg hairline p-4 text-left transition-[background-color,border-color] duration-fast ease-out motion-reduce:transition-none",
				badge.cardClassName ?? "border-wd-hairline bg-wd-surface",
				interactive &&
					"cursor-pointer hover:bg-wd-hover active:bg-wd-pressed focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-(--border-focus)",
			)}
			data-testid="board-session-card"
			data-session-id={session.id}
		>
			{interactive && onOpen ? (
				<button
					aria-label={session.title}
					className="pointer-events-none absolute inset-0 rounded-lg outline-none"
					type="button"
				/>
			) : null}
			{overlay}
			{action ? <div className="absolute right-2 top-1.5 z-10">{action}</div> : null}

			{/* Provenance first: which agent, which branch, how long ago. The card
			    answers "whose work is this" before it answers "what is it". */}
			<div className="flex flex-col gap-3">
				<div className="flex min-w-0 items-center gap-2">
					{renderAvatar(session.provider)}
					{showBranch ? (
						<span
							className="inline-flex min-w-0 max-w-branch-chip items-center gap-1.25 rounded-xs bg-wd-inset px-2 py-0.5 text-xs text-wd-secondary"
							title={branch}
						>
							{branchIcon ?? <GitBranchIcon aria-hidden="true" className="size-icon-2xs shrink-0" />}
							<span className="truncate text-branch">{branch}</span>
							{branchAction}
						</span>
					) : null}
					<span className="ml-auto shrink-0 whitespace-nowrap text-xs tabular-nums text-wd-tertiary" title={labels.updatedAt(session.updatedAt)}>
						{labels.formatTime(session.updatedAt)}
					</span>
					{interactive ? (
						<ChevronIcon
							aria-hidden="true"
							className="size-icon-sm shrink-0 text-wd-tertiary"
							direction="right"
						/>
					) : null}
				</div>

				<div className="flex flex-col gap-2">
					{/* The state line is a word with its shape cue, not a pill: on a lane
					    that already carries the state, the card names what the agent is
					    doing right now. Live work is a still blue dot - nothing pulses. */}
					<span
						className={cn(
							"flex min-w-0 items-center gap-1.5 text-xs font-medium",
							statusPresentation?.className ?? badge.className,
						)}
						style={!statusPresentation && showLiveActivity ? { color: activity.tone } : undefined}
					>
						{statusPresentation?.indicatorClassName ? (
							<span
								aria-hidden="true"
								className={cn("size-1.5 shrink-0 rounded-full", statusPresentation.indicatorClassName)}
							/>
						) : (
							<StateMark tone={showLiveActivity ? "live" : badge.stateTone} />
						)}
						<span className="min-w-0 truncate">{statusPresentation?.label ?? badge.label}</span>
					</span>
					<div
						className={cn(
							"line-clamp-3 overflow-hidden text-base font-medium leading-snug text-wd-primary",
							(overlay || action) && "pr-6",
						)}
						title={session.title}
					>
						{session.title}
					</div>
					{session.summary ? (
						<p className="line-clamp-3 overflow-hidden text-sm leading-5 text-wd-secondary">
							{session.summary}
						</p>
					) : null}
				</div>
			</div>

			{prs.length > 0 || session.trackerIssueId || artifact || usage ? (
				<div className="flex flex-col gap-2.5">
					{prs.length > 0 && (
						<div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-wd-tertiary">
							{groupBoardPullRequests(prs).map((group) => (
								<BoardPullRequestGroup
									externalLink={externalLink}
									group={group}
									key={group.state}
									labels={labels.pr}
								/>
							))}
						</div>
					)}
					{session.trackerIssueId && (
						<span
							className="inline-flex max-w-branch-chip items-center self-start truncate rounded-xs bg-wd-inset px-2 py-0.5 text-xs text-wd-secondary"
							title={labels.intakeIssue(session.trackerIssueId)}
						>
							{session.trackerIssueId}
						</span>
					)}
					{artifact || usage ? (
						<div className="flex min-w-0 items-center gap-2">
							{artifact ? (
								<div className="min-w-0 flex-1 overflow-hidden">{artifact}</div>
							) : (
								<span className="flex-1" />
							)}
							{usage ? renderUsage(usage) : null}
						</div>
					) : null}
				</div>
			) : null}

			{actions ? <div className="flex flex-wrap items-start gap-1.25">{actions}</div> : null}

			{error ? (
				<div className="text-xs text-critical" role="alert">
					{error}
				</div>
			) : null}
			{footer}
		</div>
	);
}

export const SessionUsageMetricView = forwardRef<
	HTMLSpanElement,
	{ usage: BoardUsagePresentation } & HTMLAttributes<HTMLSpanElement>
>(({ className, usage, ...props }, ref) => (
	<span
		{...props}
		aria-label={usage.accessibleLabel}
		className={cn(
			"inline-flex shrink-0 items-center gap-1 whitespace-nowrap text-xs tabular-nums text-wd-tertiary",
			className,
		)}
		ref={ref}
	>
		{usage.compactLabel}
	</span>
));
SessionUsageMetricView.displayName = "SessionUsageMetricView";

type BoardPullRequestGroupModel = {
	prs: BoardPullRequestPresentation[];
	state: BoardPullRequestState;
};

function BoardPullRequestGroup({
	externalLink: ExternalLink,
	group,
	labels,
}: {
	externalLink: ExternalLinkComponent;
	group: BoardPullRequestGroupModel;
	labels: BoardPullRequestLabels;
}) {
	const statusLabel = labels.states[group.state];
	return (
		<span
			aria-label={`${group.prs.map((pr) => `#${pr.number}`).join(", ")} ${statusLabel}`}
			className="inline-flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-1"
		>
			<span>{labels.short}</span>
			{group.prs.map((pr, index) => (
				<span className="inline-flex items-center" key={pr.url || pr.number}>
					<ExternalLink
						className="text-wd-tertiary underline-offset-2 transition-colors hover:text-wd-primary hover:underline"
						href={pr.url}
						stopPropagation
					>
						#{pr.number}
					</ExternalLink>
					{index < group.prs.length - 1 ? "," : null}
				</span>
			))}
			<span className={cn("font-medium", lifecycleClassName(group.state))}>{statusLabel}</span>
		</span>
	);
}

export function groupBoardPullRequests(
	prs: BoardPullRequestPresentation[],
): BoardPullRequestGroupModel[] {
	const groups = new Map<BoardPullRequestState, BoardPullRequestGroupModel>();
	for (const pr of prs) {
		const group = groups.get(pr.state);
		if (group) group.prs.push(pr);
		else groups.set(pr.state, { state: pr.state, prs: [pr] });
	}
	return Array.from(groups.values());
}

function lifecycleClassName(state: BoardPullRequestState): string {
	switch (state) {
		case "draft":
			return "text-wd-tertiary";
		case "merged":
			return "text-status-merged";
		case "closed":
			return "text-critical";
		case "open":
			return "text-wd-primary";
	}
}

/**
 * Collapsed archive toggle height. The overlay bar and the board's bottom
 * padding must stay in lockstep so the archive neither overlaps lanes nor
 * leaves a gap.
 */
export const ARCHIVE_TOGGLE_HEIGHT_PX = 58;
export const archiveToggleHeightClassName = "h-[58px]";
export const archiveToggleOffsetClassName = "pb-[58px]";

/**
 * Archive lives in its own memo'd component so expand/collapse state does not
 * re-render the kanban columns. Card mount is deferred via startTransition on
 * first open; after that the sheet stays mounted and open/close only tweens
 * Motion height 0↔auto (collapsed: inert / non-interactive). Overlay
 * positioning keeps lane height stable while expanded.
 */
export const SessionsArchiveView = memo(function SessionsArchiveView<
	TSession extends BoardSessionPresentation,
>({
	labels,
	renderSessionCard,
	resetKey,
	sessions,
}: {
	labels: {
		archive: string;
		archiveAria: string;
		archivedSessions: string;
	};
	renderSessionCard: (session: TSession) => ReactNode;
	/** Collapse and drop deferred cards when the board scope changes (e.g. projectId). */
	resetKey?: string;
	sessions: TSession[];
}) {
	const prefersReducedMotion = useReducedMotion();
	const [expanded, setExpanded] = useState(false);
	const [cardsReady, setCardsReady] = useState(false);

	useEffect(() => {
		setExpanded(false);
		setCardsReady(false);
	}, [resetKey]);

	useEffect(() => {
		if (!expanded || cardsReady) return;
		let cancelled = false;
		const id = requestAnimationFrame(() => {
			startTransition(() => {
				if (!cancelled) setCardsReady(true);
			});
		});
		return () => {
			cancelled = true;
			cancelAnimationFrame(id);
		};
	}, [expanded, cardsReady]);

	if (sessions.length === 0) return null;

	return (
		<div className="absolute inset-x-0 bottom-0 z-20 hairline-t border-wd-hairline bg-background px-3">
			{/* Full-row hit target: the control stretches edge-to-edge so empty
			    space beside the label toggles archive too. Height must match
			    archiveToggleOffsetClassName on the board. */}
			<button
				aria-expanded={expanded}
				aria-label={labels.archiveAria}
				className={cn(
					"group flex w-full min-w-0 items-center gap-2 py-0 text-wd-secondary transition-colors hover:text-wd-primary",
					archiveToggleHeightClassName,
					expanded ? "min-h-11" : "min-h-row-md",
				)}
				onClick={() => setExpanded((open) => !open)}
				type="button"
			>
				<ChevronIcon
					className={cn(
						"size-icon-2xs shrink-0 transition-transform duration-[140ms] ease-[cubic-bezier(0.25,0.46,0.45,0.94)]",
						prefersReducedMotion && "transition-none",
						expanded && "rotate-90",
					)}
					direction="right"
				/>
				<span className="text-2xs font-medium">{labels.archive}</span>
				<span className="ml-1.5 text-xs tabular-nums text-wd-tertiary">{sessions.length}</span>
			</button>
			{/* Keep the sheet mounted after first open; height tracks `expanded`. */}
			{cardsReady ? (
				<motion.div
					initial={prefersReducedMotion ? false : { height: 0 }}
					animate={{ height: expanded ? "auto" : 0 }}
					transition={
						prefersReducedMotion
							? { duration: 0 }
							: { duration: 0.14, ease: [0.25, 0.46, 0.45, 0.94] }
					}
					style={{ overflow: "hidden" }}
				>
					<div
						aria-hidden={!expanded}
						aria-label={expanded ? labels.archivedSessions : undefined}
						className={cn(
							"scrollbar-none grid max-h-[28vh] grid-cols-[repeat(auto-fill,minmax(17rem,1fr))] gap-2 overflow-y-auto pb-3",
							!expanded && "pointer-events-none",
						)}
						inert={!expanded ? true : undefined}
						role="list"
					>
						{sessions.map((session) => (
							<Fragment key={session.id}>{renderSessionCard(session)}</Fragment>
						))}
					</div>
				</motion.div>
			) : null}
		</div>
	);
}) as <TSession extends BoardSessionPresentation>(props: {
	labels: {
		archive: string;
		archiveAria: string;
		archivedSessions: string;
	};
	renderSessionCard: (session: TSession) => ReactNode;
	resetKey?: string;
	sessions: TSession[];
}) => ReactElement | null;

function sameLabel(a: string, b: string): boolean {
	const normalize = (value: string) =>
		value
			.toLowerCase()
			.replace(/^(feat|fix|chore|refactor|session)\//, "")
			.replace(/[^a-z0-9]+/g, "");
	return normalize(a) === normalize(b);
}
