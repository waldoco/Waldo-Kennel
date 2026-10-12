import { OutcomeTrash } from "./OutcomeDeletionControls";
import { Button } from "../ui/button";
import { useMissionAttention } from "../../hooks/useMissionAttention";
import { type MissionAttention } from "../../lib/mission-attention";
import { attentionTone, boardLane, BOARD_LANE_STATE } from "../../lib/mission-lane-tone";
import { Fragment, useMemo, useState } from "react";
import { Flag, Network, Plus, Search, SlidersHorizontal } from "lucide-react";
import { useTranslation } from "react-i18next";

import { useProjectOutcomes, type OutcomeRecord } from "../../hooks/useOutcome";
import { useWorkspaceQuery } from "../../hooks/useWorkspaceQuery";
import { cn } from "../../lib/utils";
import { deriveOutcomeDashboardPresentation } from "../../lib/outcome-dashboard-presentation";
import { formatTimeCompact } from "../../lib/format-time";
import { buildOutcomeTree, outcomeDestinationStage, type OutcomeDestinationStage } from "../../lib/outcome-tree";
import type { WorkspaceSummary } from "../../types/workspace";
import { useUiStore } from "../../stores/ui-store";

import { FactList, SessionsViewSwitch, StateBadge, StateMark } from "@pin4sf/kennel-product-ui";

type OutcomesOverviewSurfaceProps = {
	projectId?: string;
	onProjectFilterChange?: (id?: string) => void;
	selectedOutcomeId?: string;
	onOpenOutcome: (projectId: string, outcome: OutcomeRecord, stage: OutcomeDestinationStage) => void;
	onNewOutcome?: (projectId: string) => void;
};

export function OutcomesOverviewSurface({
	onOpenOutcome,
	selectedOutcomeId,
	projectId,
	onProjectFilterChange,
	onNewOutcome,
}: OutcomesOverviewSurfaceProps) {
	const { t } = useTranslation();
	const workspaceQuery = useWorkspaceQuery();
	const workspaces = workspaceQuery.data ?? [];
	const view = useUiStore((state) => state.outcomeRunViewMode);
	const setView = useUiStore((state) => state.setOutcomeRunViewMode);

	const [attentionFilter, setAttentionFilter] = useState("history");
	const [includeContributors, setIncludeContributors] = useState(false);
	const [query, setQuery] = useState("");
	const [localProjectFilter, setLocalProjectFilter] = useState("all");
	const projectFilter = onProjectFilterChange ? (projectId ?? "all") : localProjectFilter;
	const setProjectFilter = (id: string) =>
		onProjectFilterChange ? onProjectFilterChange(id === "all" ? undefined : id) : setLocalProjectFilter(id);
	const visibleWorkspaces = useMemo(
		() => workspaces.filter((workspace) => projectFilter === "all" || workspace.id === projectFilter),
		[projectFilter, workspaces],
	);
	// The New Outcome entry point exists only when the view is scoped to one
	// established project: the workspace query has settled and the filter id
	// is that workspace's real id. A stale or unknown route id, a loading
	// query, or the all-projects view must never launch intake for a project
	// the query never established.
	const scopedWorkspace =
		!workspaceQuery.isLoading && visibleWorkspaces.length === 1 && visibleWorkspaces[0].id === projectFilter
			? visibleWorkspaces[0]
			: undefined;

	return (
		<div className="flex h-full min-h-0 flex-col gap-6 overflow-y-auto" data-testid="outcomes-overview-surface">
			<header className="flex flex-wrap items-center justify-between gap-x-4 gap-y-3">
				<h2 className="wd-display-3 min-w-0 text-wd-primary">{t("outcome.overview.heading")}</h2>
				<div className="flex flex-wrap items-center gap-2">
					{scopedWorkspace && onNewOutcome ? (
						<Button onClick={() => onNewOutcome(scopedWorkspace.id)} size="sm" variant="primary">
							<Plus aria-hidden="true" className="size-icon-sm" />
							{t("outcome.dashboard.newOutcome")}
						</Button>
					) : null}
					<SessionsViewSwitch
						labels={{
							ariaLabel: t("mission.portfolioView"),
							board: t("outcome.run.viewBoard"),
							list: t("outcome.run.viewList"),
						}}
						onChange={setView}
						value={view}
					/>
					<label className="relative flex h-8 items-center">
						<Search aria-hidden="true" className="pointer-events-none absolute left-2.5 size-icon-sm text-wd-tertiary" />
						<input
							aria-label={t("shell.search")}
							className="h-8 w-44 rounded-full bg-wd-inset pl-8 pr-3 text-sm text-wd-primary outline-none placeholder:text-wd-tertiary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)"
							onChange={(event) => setQuery(event.target.value)}
							placeholder={t("shell.search")}
							type="search"
							value={query}
						/>
					</label>
					<details className="group/filters relative text-sm">
						<summary className="flex h-8 cursor-pointer list-none items-center gap-1.5 rounded-full px-3 font-medium text-wd-primary transition-colors duration-fast hover:bg-wd-hover focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus) group-open/filters:bg-wd-selected [&::-webkit-details-marker]:hidden">
							<SlidersHorizontal aria-hidden="true" className="size-icon-sm" />
							{t("mission.filters")}
						</summary>
						<div className="absolute right-0 top-full z-20 mt-2 flex w-64 flex-col gap-3 rounded-lg hairline border-wd-hairline bg-wd-raised p-3 shadow-lg">
							<select
								aria-label={t("command.group.projects")}
								className={filterSelectClassName}
								onChange={(event) => setProjectFilter(event.target.value)}
								value={projectFilter}
							>
								<option value="all">{t("command.group.projects")}</option>
								{workspaces.map((workspace) => (
									<option key={workspace.id} value={workspace.id}>
										{workspace.name}
									</option>
								))}
							</select>
							<select
								aria-label={t("mission.statusFilter")}
								className={filterSelectClassName}
								value={attentionFilter}
								onChange={(event) => setAttentionFilter(event.target.value)}
							>
								<option value="active">{t("mission.activeOutcomes")}</option>
								<option value="needsYou">{t("mission.lane.needsYou")}</option>
								<option value="history">{t("mission.includeHistory")}</option>
							</select>
							<label className="flex items-center gap-2 text-sm text-wd-secondary">
								<input
									className="size-4 accent-(--wd-accent)"
									type="checkbox"
									checked={includeContributors}
									onChange={(event) => setIncludeContributors(event.target.checked)}
								/>
								{t("mission.allOutcomes")}
							</label>
						</div>
					</details>
				</div>
			</header>

			{workspaceQuery.isLoading ? (
				<div
					aria-label={t("outcome.overview.loading")}
					className="flex flex-col gap-6"
					data-testid="outcomes-overview-loading"
					role="status"
				>
					<p aria-hidden="true" className="text-sm text-wd-secondary">
						{t("outcome.overview.loading")}
					</p>
					{[0, 1].map((groupIndex) => (
						<div className="flex flex-col gap-3" key={groupIndex}>
							<SkeletonBlock className="h-4 w-40" />
							<OutcomeCardSkeleton board={view === "board"} />
							<OutcomeCardSkeleton board={view === "board"} />
						</div>
					))}
				</div>
			) : workspaces.length === 0 ? (
				<div className="flex flex-col gap-1 py-10" data-testid="outcomes-overview-empty">
					<p className="text-sm text-wd-primary">{t("outcome.overview.noProjects")}</p>
				</div>
			) : (
				<div className="flex flex-col gap-8">
					{visibleWorkspaces.map((workspace) => (
						<ProjectOutcomesGroup
							key={workspace.id}
							onNewOutcome={scopedWorkspace?.id === workspace.id ? onNewOutcome : undefined}
							showEmpty={projectFilter !== "all"}
							attentionFilter={attentionFilter}
							includeContributors={includeContributors}
							selectedOutcomeId={selectedOutcomeId}
							onOpenOutcome={onOpenOutcome}
							query={query}
							view={view}
							workspace={workspace}
						/>
					))}
				</div>
			)}
		</div>
	);
}

const filterSelectClassName =
	"h-8 rounded-full bg-wd-inset px-3 text-sm text-wd-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)";

const BOARD_LANES = ["define", "needsYou", "observe", "review", "accepted"] as const;

/** Lane heading: the state's shape cue, the lane word, and a quiet count. */
function LaneHeading({ lane, count }: { lane: (typeof BOARD_LANES)[number]; count?: number }) {
	const { t } = useTranslation();
	return (
		<h4 className="flex h-9 items-center gap-2 px-2.5 text-sm font-medium text-wd-primary">
			<StateMark tone={BOARD_LANE_STATE[lane]} className={laneMarkClassName(lane)} />
			{t(`mission.boardLane.${lane}`)}
			{count === undefined ? null : <span className="wd-data ml-auto text-wd-tertiary">{count}</span>}
		</h4>
	);
}

/** Glyph marks pick up their state colour; dots already carry their own fill. */
function laneMarkClassName(lane: (typeof BOARD_LANES)[number]) {
	const tone = BOARD_LANE_STATE[lane];
	return tone === "good" ? "text-good" : tone === "neutral" ? "text-wd-tertiary" : undefined;
}

function ProjectOutcomesGroup({
	attentionFilter,
	includeContributors,
	selectedOutcomeId,
	workspace,
	onNewOutcome,
	onOpenOutcome,
	query,
	view,
}: {
	showEmpty: boolean;
	workspace: WorkspaceSummary;
	includeContributors: boolean;
	attentionFilter: string;
	selectedOutcomeId?: string;
	onNewOutcome?: (projectId: string) => void;
	onOpenOutcome: (projectId: string, outcome: OutcomeRecord, stage: OutcomeDestinationStage) => void;
	query: string;
	view: "board" | "list";
}) {
	const { t } = useTranslation();
	const outcomesQuery = useProjectOutcomes(workspace.id);
	const outcomes = outcomesQuery.outcomes;
	const [limit, setLimit] = useState(24);
	const outcomeTree = buildOutcomeTree(outcomes).filter((node) => {
		const needle = query.trim().toLocaleLowerCase();
		return !needle || node.outcome.title.toLocaleLowerCase().includes(needle);
	});

	const visibleNodes = outcomeTree.slice(0, limit);
	const attention = useMissionAttention(visibleNodes.map((node) => node.outcome), workspace.id);
	const filteredNodes = visibleNodes.filter((node) => {
		const lane = attention.get(node.outcome.id)?.lane;
		return attentionFilter === "history" || (attentionFilter === "needsYou"
			? boardLane(lane) === "needsYou" || boardLane(lane) === "review"
			: lane !== "accepted");
	});
	const lanes = view === "board" ? BOARD_LANES : [undefined];


	// Keep the project heading and Trash reachable after its final Outcome is removed.

	return (
		<section className="flex flex-col gap-3" data-testid="outcomes-overview-project">
			<div className="flex items-center justify-between gap-3">
				<h3 className="wd-headline min-w-0 truncate text-wd-primary">{workspace.name}</h3>
				<OutcomeTrash projectId={workspace.id} />
			</div>
			{outcomesQuery.failure ? (
				<div
					className="flex items-center gap-3 rounded-lg hairline border-wd-hairline bg-wd-surface px-4 py-3 text-sm text-wd-primary"
					role="alert"
				>
					<span className="min-w-0 flex-1">{t("outcome.dashboard.loadFailed")}</span>
					<Button onClick={outcomesQuery.refetch} size="sm" variant="ghost">
						{t("outcome.understand.retry")}
					</Button>
				</div>
			) : outcomesQuery.isLoading ? (
				// Skeleton cards inside the real lane shells: lane names and marks
				// are static canon, so the board's shape is honest before the
				// counts arrive. No pulse - the motion vocabulary bans endless
				// shimmer, so the skeleton is static and content simply appears.
				<div aria-label={t("outcome.overview.loading")} className="flex flex-col gap-2" role="status">
					{view === "board" ? (
						<div className="flex gap-3 overflow-x-auto pb-2">
							{BOARD_LANES.map((lane) => (
								<section className={laneShellClassName} key={lane}>
									<LaneHeading lane={lane} />
									<OutcomeCardSkeleton board />
								</section>
							))}
						</div>
					) : (
						<div className={listShellClassName}>
							<OutcomeCardSkeleton board={false} />
							<OutcomeCardSkeleton board={false} />
							<OutcomeCardSkeleton board={false} />
						</div>
					)}
				</div>
			) : outcomes.length === 0 ? (
				// A project with zero Outcomes gets an invitation, not five empty
				// columns: the Figma creation frame (8572) is a single centered
				// prompt, and "setup ends with a first Outcome". The CTA only
				// renders where the scoped New Outcome entry point is established;
				// everywhere else the compact line points at the project instead.
				onNewOutcome ? (
					<ProjectOutcomesEmptyState projectName={workspace.name} onNewOutcome={() => onNewOutcome(workspace.id)} />
				) : (
					<p className="text-sm text-wd-secondary">{t("outcome.empty.compact")}</p>
				)
			) : (
				<div className="flex flex-col gap-3">
					{filteredNodes.length === 0 && (
						<p className="text-sm text-wd-secondary">{t("mission.noMatchingOutcomes")}</p>
					)}
					<div className={cn(view === "board" ? "flex gap-3 overflow-x-auto pb-2" : "flex flex-col")}>
						{lanes.map((lane) => {
							const laneNodes = filteredNodes.filter(
								(node) => !lane || boardLane(attention.get(node.outcome.id)?.lane) === lane,
							);
							if (!lane && laneNodes.length === 0) return null;
							return (
								<section key={lane ?? "list"} className={lane ? laneShellClassName : listShellClassName}>
									{lane && <LaneHeading count={laneNodes.length} lane={lane} />}
									<ul className={cn("flex flex-col", lane && "gap-2")}>
										{laneNodes.map((node) => (
											<Fragment key={node.outcome.id}>
												<OutcomeOverviewRow
													attention={attention.get(node.outcome.id)}
													workspaceName={workspace.name}
													selected={selectedOutcomeId === node.outcome.id}
													onOpen={() => onOpenOutcome(workspace.id, node.outcome, outcomeDestinationStage(node, attention.get(node.outcome.id)?.lane))}
													onOpenMissionControl={() => onOpenOutcome(workspace.id, node.outcome, outcomeDestinationStage(node, attention.get(node.outcome.id)?.lane))}
													outcome={node.outcome}
												/>
												{includeContributors &&
													node.contributors.map((contributor) => (
														<OutcomeOverviewRow
															contributor
															workspaceName={workspace.name}
															key={contributor.id}
															onOpen={() => onOpenOutcome(workspace.id, contributor, "decide_authorize")}
															outcome={contributor}
														/>
													))}
											</Fragment>
										))}
									</ul>
								</section>
							);
						})}
					</div>
					{outcomeTree.length > limit && (
						<Button className="self-start" onClick={() => setLimit((value) => value + 24)} size="sm" variant="ghost">
							{t("mission.moreOutcomes")}
						</Button>
					)}
				</div>
			)}
		</section>
	);
}

/** Board lane plate: the shell tone, radius-lg, no glow and no coloured top. */
const laneShellClassName = "flex min-h-80 min-w-[232px] flex-1 flex-col gap-1 rounded-lg bg-shell p-1.5";
/** List view: one surface, rows separated by hairlines. */
const listShellClassName = "min-w-0 overflow-hidden rounded-lg hairline border-wd-hairline bg-wd-surface";

// The Outcome card (Waldo OutcomeCard): eyebrow with the project and the state
// badge, the title in the person's words, then the next move with its actor
// named. Never agent counts or percent complete. The whole card is the press
// target. `contributor` indents a contributing Outcome under the parent that
// claims it. The Mission Control action sits outside the card's own button
// rather than inside it — a button cannot nest, and the two go to different
// places.
function OutcomeOverviewRow({
	attention,
	selected,
	outcome,
	workspaceName,
	contributor = false,
	onOpen,
	onOpenMissionControl,
}: {
	outcome: OutcomeRecord;
	attention?: MissionAttention;
	selected?: boolean;
	workspaceName?: string;
	contributor?: boolean;
	onOpen: () => void;
	onOpenMissionControl?: () => void;
}) {
	const { t } = useTranslation();
	const presentation = deriveOutcomeDashboardPresentation(outcome);
	const board = useUiStore((state) => state.outcomeRunViewMode === "board");
	const tone = attentionTone(attention);
	const stateLabel = attention ? t(`mission.lane.${attention.lane}`) : t(presentation.stateKey);
	const next = attention?.reason
		? t(`mission.reason.${attention.reason}`, { defaultValue: attention.reason })
		: attention
			? t(`mission.next.${attention.lane}`)
			: t(presentation.nextActionKey);
	const updated = formatTimeCompact(outcome.updatedAt);
	const revisions = t("mission.revisions", {
		contract: outcome.currentRevisionNumber,
		plan: outcome.latestPlan?.number ?? t("mission.noPlan"),
	});

	const missionControl = onOpenMissionControl ? (
		<button
			aria-label={t("outcome.dashboard.missionControlAria", { title: outcome.title })}
			data-outcome-mission-control-id={outcome.id}
			className={cn(
				"grid size-7 shrink-0 place-items-center rounded-full text-wd-secondary opacity-0",
				"transition-[background-color,color,opacity] duration-fast hover:bg-wd-hover hover:text-wd-primary motion-reduce:transition-none",
				"focus-visible:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)",
				"group-hover/outcome-overview-row:opacity-100 group-focus-within/outcome-overview-row:opacity-100",
				board ? "absolute bottom-3 right-3" : "mr-2",
			)}
			onClick={onOpenMissionControl}
			type="button"
		>
			<Network aria-hidden="true" className="size-icon-sm" />
		</button>
	) : null;

	if (board) {
		return (
			<li className={cn(contributor && "pl-6")}>
				<div
					className={cn(
						"group/outcome-overview-row relative rounded-lg hairline transition-[background-color,border-color] duration-fast ease-out motion-reduce:transition-none",
						selected
							? "border-wd-strong bg-wd-selected"
							: "border-wd-hairline bg-wd-surface hover:bg-wd-hover active:bg-wd-pressed",
					)}
				>
					<button
						className="flex w-full min-w-0 flex-col items-start gap-2 rounded-lg p-4 text-left focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)"
						data-testid="outcomes-overview-row"
						data-outcome-id={outcome.id}
						aria-current={selected ? "true" : undefined}
						onClick={onOpen}
						type="button"
					>
						<span className="flex w-full min-w-0 items-center gap-2 text-xs text-wd-secondary">
							<Flag aria-hidden="true" className="size-icon-sm shrink-0" />
							{workspaceName ? <span className="min-w-0 truncate">{workspaceName}</span> : null}
							<StateBadge className="ml-auto" label={stateLabel} tone={tone} />
						</span>
						<span className="wd-headline line-clamp-3 w-full break-words text-wd-primary">{outcome.title}</span>
						<FactList className="w-full text-[13px] leading-[18px]" facts={[{ label: t("outcome.card.next"), value: next }]} />
						<span className={cn("flex w-full min-w-0 items-center gap-1.5 text-wd-tertiary", onOpenMissionControl && "pr-8")}>
							<span className="wd-data shrink-0">{updated}</span>
							<span aria-hidden="true" className="wd-data">·</span>
							<span className="wd-data min-w-0 truncate">{revisions}</span>
						</span>
					</button>
					{missionControl}
				</div>
			</li>
		);
	}

	return (
		<li className={cn("hairline-b border-wd-hairline last:border-b-0", contributor && "pl-6")}>
			<div
				className={cn(
					"group/outcome-overview-row flex w-full min-w-0 items-center transition-colors duration-fast ease-out motion-reduce:transition-none",
					selected ? "bg-wd-selected" : "hover:bg-wd-hover active:bg-wd-pressed",
				)}
			>
				<button
					className="flex min-w-0 flex-1 items-center gap-3 px-4 py-3 text-left focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-(--border-focus)"
					data-testid="outcomes-overview-row"
					data-outcome-id={outcome.id}
					aria-current={selected ? "true" : undefined}
					onClick={onOpen}
					type="button"
				>
					<span className="flex min-w-0 flex-1 flex-col gap-1">
						<span className="line-clamp-2 break-words text-sm font-medium text-wd-primary">{outcome.title}</span>
						<FactList className="text-[13px] leading-[18px]" facts={[{ label: t("outcome.card.next"), value: next }]} />
					</span>
					<span className="flex shrink-0 flex-col items-end gap-1">
						<StateBadge label={stateLabel} tone={tone} />
						<span className="wd-data text-wd-tertiary">{updated}</span>
					</span>
				</button>
				{missionControl}
			</div>
		</li>
	);
}

export function ProjectOutcomesEmptyState({
	projectName,
	onNewOutcome,
}: {
	projectName: string;
	onNewOutcome: () => void;
}) {
	const { t } = useTranslation();
	return (
		<div
			className="flex flex-col items-start gap-2 rounded-lg hairline border-wd-hairline bg-wd-surface p-6"
			data-testid="outcomes-empty-project"
		>
			<p className="text-base font-medium text-wd-primary">{t("outcome.empty.title", { project: projectName })}</p>
			<p className="max-w-md text-sm leading-5 text-wd-secondary">
				{t("outcome.empty.body", { project: projectName })}
			</p>
			{/* Creating the first Outcome is this screen's single purpose, so its
			    one action may carry Spot. */}
			<Button className="mt-2" onClick={onNewOutcome} size="sm" variant="spot">
				<Plus aria-hidden="true" className="size-icon-sm" />
				{t("outcome.dashboard.newOutcome")}
			</Button>
		</div>
	);
}

function SkeletonBlock({ className }: { className: string }) {
	return <div aria-hidden="true" className={cn("rounded-xs bg-wd-inset", className)} />;
}

export function OutcomeCardSkeleton({ board }: { board: boolean }) {
	return (
		<div
			aria-hidden="true"
			className={cn(
				board
					? "rounded-lg hairline border-wd-hairline bg-wd-surface p-4"
					: "hairline-b border-wd-hairline px-4 py-3 last:border-b-0",
			)}
		>
			<div className="flex flex-col gap-2.5">
				{board ? (
					<div className="flex items-center gap-2">
						<SkeletonBlock className="size-3 rounded-full" />
						<SkeletonBlock className="h-2.5 w-24" />
						<SkeletonBlock className="ml-auto h-4 w-14" />
					</div>
				) : null}
				<SkeletonBlock className="h-3.5 w-3/4" />
				<SkeletonBlock className="h-2.5 w-full" />
				{board ? <SkeletonBlock className="h-2.5 w-1/2" /> : null}
			</div>
		</div>
	);
}
