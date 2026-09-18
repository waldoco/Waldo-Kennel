import { OutcomeTrash } from "./OutcomeDeletionControls";
import { useMissionAttention } from "../../hooks/useMissionAttention";
import { type MissionAttention } from "../../lib/mission-attention";
import { Fragment, useMemo, useState } from "react";
import { Flag, Network } from "lucide-react";
import { useTranslation } from "react-i18next";

import { useProjectOutcomes, type OutcomeRecord } from "../../hooks/useOutcome";
import { useWorkspaceQuery } from "../../hooks/useWorkspaceQuery";
import { cn } from "../../lib/utils";
import { deriveOutcomeDashboardPresentation } from "../../lib/outcome-dashboard-presentation";
import { buildOutcomeTree, outcomeDestinationStage, type OutcomeDestinationStage } from "../../lib/outcome-tree";
import type { WorkspaceSummary } from "../../types/workspace";
import { useUiStore } from "../../stores/ui-store";

type OutcomesOverviewSurfaceProps = {
	projectId?: string;
	onProjectFilterChange?: (id?: string) => void;
	selectedOutcomeId?: string;
	onOpenOutcome: (projectId: string, outcome: OutcomeRecord, stage: OutcomeDestinationStage) => void;
};

export function OutcomesOverviewSurface({
	onOpenOutcome,
	selectedOutcomeId,
	projectId,
	onProjectFilterChange,
}: OutcomesOverviewSurfaceProps) {
	const { t } = useTranslation();
	const workspaceQuery = useWorkspaceQuery();
	const workspaces = workspaceQuery.data ?? [];
	const view = useUiStore((state) => state.outcomeRunViewMode);

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

	return (
		<div className="flex h-full min-h-0 flex-col gap-3 overflow-y-auto" data-testid="outcomes-overview-surface">
			<div className="flex flex-wrap items-center justify-between gap-3">
				<div className="max-w-xl">
					<h2 className="text-base font-medium">{t("outcome.overview.heading")}</h2>
				</div>
				<div className="flex flex-wrap items-center gap-2">
					<input
						aria-label={t("shell.search")}
						className="h-8 rounded-md border border-border bg-card px-2 text-xs outline-hidden focus-visible:ring-2 focus-visible:ring-ring/70"
						onChange={(event) => setQuery(event.target.value)}
						placeholder={t("shell.search")}
						value={query}
					/>
					<details className="relative text-xs">
						<summary className="cursor-pointer rounded-lg border border-border px-3 py-2 hover:bg-interactive-hover">
							{t("mission.filters")}
						</summary>
						<div className="absolute right-0 top-full z-20 mt-2 flex w-64 flex-col gap-3 rounded-xl border border-border bg-card p-3 shadow-lg">
							<select
								aria-label={t("command.group.projects")}
								className="h-8 rounded-md border border-border bg-card px-2 text-xs"
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
								className="h-8 rounded-md border border-border bg-card px-2 text-xs"
								value={attentionFilter}
								onChange={(event) => setAttentionFilter(event.target.value)}
							>
								<option value="active">{t("mission.activeOutcomes")}</option>
								<option value="needsYou">{t("mission.lane.needsYou")}</option>
								<option value="history">{t("mission.includeHistory")}</option>
							</select>
							<label className="flex items-center gap-1 text-xs text-muted-foreground">
								<input
									type="checkbox"
									checked={includeContributors}
									onChange={(event) => setIncludeContributors(event.target.checked)}
								/>
								{t("mission.allOutcomes")}
							</label>
						</div>
					</details>
				</div>
			</div>

			{workspaceQuery.isLoading ? (
				<p className="text-muted-foreground text-sm" data-testid="outcomes-overview-loading">
					{t("outcome.overview.loading")}
				</p>
			) : workspaces.length === 0 ? (
				<p className="text-muted-foreground text-sm" data-testid="outcomes-overview-empty">
					{t("outcome.overview.noProjects")}
				</p>
			) : (
				<div className="flex flex-col gap-5">
					{visibleWorkspaces.map((workspace) => (
						<ProjectOutcomesGroup
							key={workspace.id}
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

function ProjectOutcomesGroup({
	attentionFilter,
	includeContributors,
	selectedOutcomeId,
	workspace,
	onOpenOutcome,
	query,
	view,
}: {
	showEmpty: boolean;
	workspace: WorkspaceSummary;
	includeContributors: boolean;
	attentionFilter: string;
	selectedOutcomeId?: string;
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
	// Board buckets group derived states; acceptance remains a daemon fact.
	// Canon lanes (locked 2026-09-17, ruling 2026-09-18): review is its own
	// "Ready" lane - finished work awaiting acceptance, not an input ask.
	const boardLane = (lane?: string) => lane === "accepted" ? "accepted" : lane === "review" ? "review" : lane === "observe" ? "observe" : lane === "define" || lane === "authorize" ? "define" : "needsYou";
	const filteredNodes = visibleNodes.filter((node) => {
		const lane = attention.get(node.outcome.id)?.lane;
		return attentionFilter === "history" || (attentionFilter === "needsYou"
			? boardLane(lane) === "needsYou" || boardLane(lane) === "review"
			: lane !== "accepted");
	});
	const lanes = view === "board" ? (["define", "needsYou", "observe", "review", "accepted"] as const) : [undefined];


	// Keep the project heading and Trash reachable after its final Outcome is removed.

	return (
		<section className="flex flex-col gap-2" data-testid="outcomes-overview-project">
			<div className="flex items-center justify-between"><h3 className="text-sm font-medium text-foreground">{workspace.name}</h3><OutcomeTrash projectId={workspace.id}/></div>
			{outcomesQuery.failure ? (
				<div
					className="flex items-center gap-3 rounded-md hairline border-border bg-card px-3 py-2 text-xs text-muted-foreground"
					role="alert"
				>
					<span className="min-w-0 flex-1">{t("outcome.dashboard.loadFailed")}</span>
					<button className="font-medium text-foreground hover:underline" onClick={outcomesQuery.refetch} type="button">
						{t("outcome.understand.retry")}
					</button>
				</div>
			) : outcomesQuery.isLoading ? (
				<p className="text-muted-foreground text-xs">{t("outcome.overview.loading")}</p>
			) : (
				<div className="flex flex-col gap-2">
					{filteredNodes.length === 0 && (
						<p className="px-3 py-2 text-xs text-muted-foreground">{t("mission.noMatchingOutcomes")}</p>
					)}
					<div className={cn(view === "board" ? "flex gap-3 overflow-x-auto pb-2" : "flex flex-col")}>
						{lanes.map((lane) => (
							<section
								key={lane ?? "list"}
								className={cn(
									"min-w-0",
									view === "board" && "min-w-[220px] flex-1 rounded-2xl bg-surface/50 p-1 min-h-80",
								)}
							>
								{lane && (
									<h4 className="flex h-10 items-center gap-2 px-3 text-xs font-medium">
										<span
											aria-hidden="true"
											className={cn(
												"size-2 rounded-full",
												lane === "define"
													? "bg-status-needs-you"
													: lane === "needsYou"
														? "bg-status-in-review"
														: lane === "review"
															? "bg-status-ready"
															: lane === "observe"
																? "bg-status-working"
																: "bg-muted-foreground",
											)}
										/>
										{t(`mission.boardLane.${lane}`)}
										<span className="ml-auto tabular-nums text-muted-foreground">
											{filteredNodes.filter((node) => boardLane(attention.get(node.outcome.id)?.lane) === lane).length}
										</span>
									</h4>
								)}
								<ul className="flex flex-col gap-2">
									{filteredNodes
										.filter((node) => !lane || boardLane(attention.get(node.outcome.id)?.lane) === lane)
										.map((node) => (
											<Fragment key={node.outcome.id}>
												<OutcomeOverviewRow
													attention={attention.get(node.outcome.id)}
													selected={selectedOutcomeId === node.outcome.id}
												onOpen={() => onOpenOutcome(workspace.id, node.outcome, outcomeDestinationStage(node, attention.get(node.outcome.id)?.lane))}
												onOpenMissionControl={() => onOpenOutcome(workspace.id, node.outcome, outcomeDestinationStage(node, attention.get(node.outcome.id)?.lane))}
													outcome={node.outcome}
												/>
												{includeContributors &&
													node.contributors.map((contributor) => (
														<OutcomeOverviewRow
															contributor
															key={contributor.id}
															onOpen={() => onOpenOutcome(workspace.id, contributor, "decide_authorize")}
															outcome={contributor}
														/>
													))}
											</Fragment>
										))}
								</ul>
							</section>
						))}
					</div>
					{outcomeTree.length > limit && (
						<button
							type="button"
							className="self-start text-xs hover:underline"
							onClick={() => setLimit((value) => value + 24)}
						>
							{t("mission.moreOutcomes")}
						</button>
					)}
				</div>
			)}
		</section>
	);
}

// `contributor` indents a contributing Outcome under the parent that claims
// it. The Mission Control action sits outside the row's own button rather than
// inside it — a button cannot nest, and the two go to different places.
function OutcomeOverviewRow({
	attention,
	selected,
	outcome,
	contributor = false,
	onOpen,
	onOpenMissionControl,
}: {
	outcome: OutcomeRecord;
	attention?: MissionAttention;
	selected?: boolean;
	contributor?: boolean;
	onOpen: () => void;
	onOpenMissionControl?: () => void;
}) {
	const { t } = useTranslation();
	const presentation = deriveOutcomeDashboardPresentation(outcome);
	const board = useUiStore((state) => state.outcomeRunViewMode === "board");
	return (
		<li className={cn(contributor && "pl-6")}>
			<div
				className={cn(
					"group/outcome-overview-row flex w-full min-w-0 items-center hairline border-border bg-card transition-colors duration-150 motion-reduce:transition-none hover:bg-interactive-hover focus-within:bg-interactive-hover",
					board ? "rounded-[18px]" : "rounded-lg",
					selected && "ring-1 ring-ring/60",
				)}
			>
				<button
					className={cn(
						"flex min-w-0 flex-1 items-center gap-2.5 text-left outline-hidden focus-visible:ring-2 focus-visible:ring-ring/70",
						board ? "flex-col items-start rounded-[18px] p-[18px]" : "rounded-lg px-3.5 py-3",
					)}
					data-testid="outcomes-overview-row"
					data-outcome-id={outcome.id}
					aria-current={selected ? "true" : undefined}
					onClick={onOpen}
					type="button"
				>
					<span className="flex items-center gap-2 text-xs text-muted-foreground">
						<Flag aria-hidden="true" className="size-icon-sm shrink-0" />
						{t("mission.revisions", {
							contract: outcome.currentRevisionNumber,
							plan: outcome.latestPlan?.number ?? t("mission.noPlan"),
						})}
					</span>
					<span className={cn("flex min-w-0 flex-1 flex-col gap-2 text-foreground", board ? "text-base" : "text-sm")}>
						<span className="line-clamp-3 break-words font-medium">{outcome.title}</span>
						<span
							className={cn(
								"text-xs leading-relaxed",
								board && "order-first",
								attention?.lane === "needsYou" || attention?.lane === "authorize"
									? "text-orange-400"
									: "text-muted-foreground",
							)}
						>
							{attention?.reason
								? t(`mission.reason.${attention.reason}`, { defaultValue: attention.reason })
								: attention
									? t(`mission.next.${attention.lane}`)
									: t(presentation.nextActionKey)}
						</span>
					</span>
					<span className="text-xs text-muted-foreground">
						{attention ? t(`mission.lane.${attention.lane}`) : t(presentation.stateKey)}
					</span>
				</button>
				{onOpenMissionControl ? (
					<button
						aria-label={t("outcome.dashboard.missionControlAria", { title: outcome.title })}
						data-outcome-mission-control-id={outcome.id}
						className={cn(
							"mr-2 grid size-7 shrink-0 place-items-center rounded-md text-muted-foreground opacity-0",
							"transition-[background-color,color,opacity] hover:bg-interactive-hover hover:text-foreground",
							"focus-visible:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent/50",
							"group-hover/outcome-overview-row:opacity-100 group-focus-within/outcome-overview-row:opacity-100",
						)}
						onClick={onOpenMissionControl}
						type="button"
					>
						<Network aria-hidden="true" className="size-icon-sm" />
					</button>
				) : null}
			</div>
		</li>
	);
}
