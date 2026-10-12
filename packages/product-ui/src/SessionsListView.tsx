import { Fragment, type ReactNode } from "react";
import type { ExternalLinkComponent } from "./external-link";
import { ChevronIcon, GitBranchIcon } from "./icons";
import { laneMarkClassName } from "./SessionsBoardView";
import { StateMark } from "./StateBadge";
import {
	attentionZone,
	getAgentActivityView,
	getSessionStatusView,
	type AttentionZone,
	type AttentionZoneView,
	type ProductUITranslator,
} from "./session-presentation";
import {
	groupBoardPullRequests,
	type BoardPullRequestLabels,
	type BoardPullRequestPresentation,
	type BoardSessionPresentation,
	type BoardSplitLaneLabels,
	type BoardUsagePresentation,
} from "./SessionsBoardView";
import { cn } from "./utils";

/**
 * List is the board's other reading: same lanes, same lane headings, but every
 * session collapsed to a single scannable line. The board answers "what is the
 * shape of my queue"; the list answers "what is in it, in order". They share the
 * lane model deliberately — switching views must never re-sort a person's work.
 */

export type SessionsListViewProps<
	TSession extends BoardSessionPresentation = BoardSessionPresentation,
> = {
	columns: AttentionZoneView[];
	labels: BoardSplitLaneLabels;
	renderSessionRow: (session: TSession) => ReactNode;
	sessions: TSession[];
};

export function SessionsListView<TSession extends BoardSessionPresentation>({
	columns,
	labels,
	renderSessionRow,
	sessions,
}: SessionsListViewProps<TSession>) {
	const byZone = new Map<AttentionZone, TSession[]>();
	for (const session of sessions) {
		const zone = attentionZone(session.status);
		const sessionsForZone = byZone.get(zone);
		if (sessionsForZone) sessionsForZone.push(session);
		else byZone.set(zone, [session]);
	}

	return (
		<div className="board-scrollbar h-full overflow-y-auto px-2.5 pb-2.5" data-testid="board-list-scroll">
			<div className="flex flex-col gap-2.5">
				{columns.map((column) => (
					<ListLaneView
						column={column}
						key={column.zone}
						labels={labels}
						renderSessionRow={renderSessionRow}
						sessions={byZone.get(column.zone) ?? []}
					/>
				))}
			</div>
		</div>
	);
}

function ListLaneView<TSession extends BoardSessionPresentation>({
	column,
	labels,
	renderSessionRow,
	sessions,
}: {
	column: AttentionZoneView;
	labels: BoardSplitLaneLabels;
	renderSessionRow: (session: TSession) => ReactNode;
	sessions: TSession[];
}) {
	return (
		<section
			aria-label={labels.columnAria(column.label)}
			className="flex flex-col gap-1 rounded-lg bg-shell p-1.5"
			data-column={column.zone}
			data-testid="board-list-lane"
		>
			<div className="flex shrink-0 items-center justify-between gap-2 px-2.5 py-1.5">
				<div className="flex min-w-0 items-center gap-2">
					<StateMark className={laneMarkClassName(column.stateTone)} tone={column.stateTone} />
					<span className="truncate text-sm font-medium text-wd-primary">{column.label}</span>
				</div>
				<span
					aria-label={labels.countSessions(sessions.length, column.label)}
					className="shrink-0 text-[13px] leading-[18px] tabular-nums text-wd-tertiary"
				>
					{sessions.length}
				</span>
			</div>
			{/* One plate for the whole lane, not one per row: the rows are a table,
			    and a table has a single edge. */}
			{sessions.length > 0 ? (
				<div className="overflow-hidden rounded-md hairline border-wd-hairline bg-card">
					{sessions.map((session) => (
						<Fragment key={session.id}>{renderSessionRow(session)}</Fragment>
					))}
				</div>
			) : null}
		</section>
	);
}

export type SessionRowViewProps = {
	action?: ReactNode;
	/** Optional primary work artifact shown in the artifact column. */
	artifact?: ReactNode;
	branchIcon?: ReactNode;
	error?: string;
	externalLink: ExternalLinkComponent;
	interactive?: boolean;
	labels: {
		formatTime: (timestamp: string) => string;
		intakeIssue: (id: string) => string;
		pr: BoardPullRequestLabels;
		updatedAt: (timestamp: string) => string;
	};
	onOpen?: () => void;
	prs?: BoardPullRequestPresentation[];
	renderAvatar: (provider: string) => ReactNode;
	renderUsage?: (usage: BoardUsagePresentation) => ReactNode;
	session: BoardSessionPresentation;
	translate?: ProductUITranslator;
	usage?: BoardUsagePresentation;
};

/**
 * A session as one line. Every cell truncates into the row plate rather than
 * ellipsing mid-word — at a glance the eye is scanning column positions, not
 * reading each cell to its end, and a hard clip keeps the columns aligned.
 */
export function SessionRowView({
	action,
	artifact,
	branchIcon,
	error,
	externalLink,
	interactive = true,
	labels,
	onOpen,
	prs = [],
	renderAvatar,
	renderUsage,
	session,
	translate,
	usage,
}: SessionRowViewProps) {
	const badge = getSessionStatusView(session.status, translate);
	const activity = getAgentActivityView(session.activity, translate);
	const showLiveActivity = session.status === "working" && activity.state === "active";
	const statusPresentation = session.statusPresentation;
	const branch = session.branch ?? "";
	const showBranch = branch !== "" && branch !== session.title && branch !== session.id;
	const prGroups = groupBoardPullRequests(prs);

	return (
		<div
			className={cn(
				"group/row relative flex min-h-row-list w-full items-center gap-2.5 px-2.5 py-1 text-left transition-colors duration-fast hairline-b border-wd-hairline last:border-b-0",
				interactive && "cursor-pointer hover:bg-wd-hover focus-within:bg-wd-hover",
			)}
			data-session-id={session.id}
			data-testid="board-session-row"
			onClick={interactive ? onOpen : undefined}
			role={interactive ? undefined : "listitem"}
		>
			{interactive && onOpen ? (
				<button aria-label={session.title} className="pointer-events-none absolute inset-0 outline-none" type="button" />
			) : null}

			<span className="shrink-0">{renderAvatar(session.provider)}</span>

			<ListCell className="basis-[16.25rem]">
				<span className="whitespace-nowrap text-sm font-medium text-wd-primary" title={session.title}>
					{session.title}
				</span>
			</ListCell>

			<ListCell className="basis-[13.5rem]">
				<span
					className={cn(
						"inline-flex items-center gap-1.5 whitespace-nowrap text-sm font-medium",
						statusPresentation?.className ?? badge.className,
					)}
					style={!statusPresentation && showLiveActivity ? { color: activity.tone } : undefined}
				>
					{statusPresentation?.indicatorClassName ? (
						<span aria-hidden="true" className={cn("size-1.5 shrink-0 rounded-full", statusPresentation.indicatorClassName)} />
					) : (
						<StateMark tone={showLiveActivity ? "live" : badge.stateTone} />
					)}
					<span>{statusPresentation?.label ?? badge.label}</span>
				</span>
			</ListCell>

			<ListCell className="basis-[8.5rem]">
				{showBranch ? (
					<span
						className="inline-flex items-center gap-1.25 whitespace-nowrap rounded-xs bg-wd-inset px-2 py-0.5 text-xs text-wd-secondary"
						title={branch}
					>
						{branchIcon ?? <GitBranchIcon aria-hidden="true" className="size-icon-2xs shrink-0" />}
						<span className="text-branch">{branch}</span>
					</span>
				) : null}
			</ListCell>

			<ListCell className="basis-[11rem]">
				{artifact ??
					(prGroups.length > 0 ? (
						<span className="flex items-center gap-1.5 whitespace-nowrap text-xs text-wd-tertiary">
							{prGroups.map((group) => (
								<Fragment key={group.state}>
									{group.prs.map((pr) => {
										const ExternalLink = externalLink;
										return (
											<ExternalLink
												className="text-wd-primary underline decoration-link underline-offset-2 transition-colors hover:text-wd-secondary"
												href={pr.url}
												key={pr.url || pr.number}
												stopPropagation
											>
												{labels.pr.short}
												{pr.number}
											</ExternalLink>
										);
									})}
								</Fragment>
							))}
						</span>
					) : session.trackerIssueId ? (
						<span
							className="whitespace-nowrap text-xs text-wd-tertiary"
							title={labels.intakeIssue(session.trackerIssueId)}
						>
							{session.trackerIssueId}
						</span>
					) : null)}
			</ListCell>

			{usage && renderUsage ? <span className="shrink-0">{renderUsage(usage)}</span> : null}

			<span
				className="w-list-time shrink-0 whitespace-nowrap text-xs tabular-nums text-wd-tertiary"
				title={labels.updatedAt(session.updatedAt)}
			>
				{labels.formatTime(session.updatedAt)}
			</span>

			{error ? (
				<span className="shrink-0 whitespace-nowrap text-xs text-critical" role="alert">
					{error}
				</span>
			) : null}

			<span className="ml-auto shrink-0">
				{action ?? (
					<ChevronIcon aria-hidden="true" className="size-icon-sm text-wd-tertiary" direction="right" />
				)}
			</span>
		</div>
	);
}

/**
 * Fixed-proportion cell that clips its content behind a fade instead of an
 * ellipsis, so neighbouring rows keep their column edges aligned.
 */
function ListCell({ children, className }: { children: ReactNode; className?: string }) {
	return (
		<span className={cn("relative min-w-0 shrink grow-0 overflow-hidden", className)}>
			<span className="block overflow-hidden">{children}</span>
			<span
				aria-hidden="true"
				className="list-cell-fade pointer-events-none absolute inset-y-0 right-0 w-10 opacity-100 transition-opacity duration-fast ease-out group-hover/row:opacity-0 motion-reduce:transition-none"
			/>
		</span>
	);
}

export type SessionsViewMode = "board" | "list";

export type SessionsViewSwitchLabels = {
	ariaLabel: string;
	board: string;
	list: string;
};

/**
 * List / Board switch. A two-item segmented control rather than an icon toggle:
 * the two views are peers, and naming both makes the alternative discoverable
 * without a hover. `disabled` renders it inert *visibly* — muted, non-clickable,
 * `aria-disabled` — for a caller with only one real view behind it right now:
 * a control that silently ignores clicks (present but wired to nothing) is
 * never an acceptable reading, only an explicitly disabled one is.
 */
export function SessionsViewSwitch({
	disabled = false,
	labels,
	onChange,
	value,
}: {
	disabled?: boolean;
	labels: SessionsViewSwitchLabels;
	onChange: (mode: SessionsViewMode) => void;
	value: SessionsViewMode;
}) {
	return (
		<div
			aria-disabled={disabled || undefined}
			aria-label={labels.ariaLabel}
			className={cn("inline-flex h-control-segment shrink-0 items-center rounded-full bg-wd-inset p-0.5", disabled && "cursor-not-allowed")}
			data-testid="sessions-view-switch"
			role="tablist"
		>
			<SessionsViewSwitchItem
				active={value === "list"}
				disabled={disabled}
				label={labels.list}
				mode="list"
				onSelect={onChange}
			/>
			<SessionsViewSwitchItem
				active={value === "board"}
				disabled={disabled}
				label={labels.board}
				mode="board"
				onSelect={onChange}
			/>
		</div>
	);
}

function SessionsViewSwitchItem({
	active,
	disabled,
	label,
	mode,
	onSelect,
}: {
	active: boolean;
	disabled: boolean;
	label: string;
	mode: SessionsViewMode;
	onSelect: (mode: SessionsViewMode) => void;
}) {
	return (
		<button
			aria-selected={active}
			className={cn(
				"inline-flex h-full items-center justify-center rounded-full px-3 text-sm font-medium transition-colors duration-fast ease-out motion-reduce:transition-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)",
				disabled && "cursor-not-allowed text-wd-disabled",
				!disabled && (active ? "bg-wd-inverse text-wd-on-inverse" : "text-wd-secondary hover:text-wd-primary"),
			)}
			disabled={disabled}
			onClick={() => {
				if (!disabled) onSelect(mode);
			}}
			role="tab"
			type="button"
		>
			{label}
		</button>
	);
}
