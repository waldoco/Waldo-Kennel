import { OutcomeDeletionControls } from "./OutcomeDeletionControls";
import { MissionContractEditor } from "./MissionContractEditor";
import { useOutcomeRunState } from "../../hooks/useOutcomeRunState";
import { X, Maximize2, Minimize2, History } from "lucide-react";
import { runStateAttention } from "../../lib/mission-attention";
import { boardLane, BOARD_LANE_TONE } from "../../lib/mission-lane-tone";
import { cn } from "../../lib/utils";
import { MissionUsage } from "./MissionUsage";
import { useSettings } from "../../hooks/useSettings";
import { ReasoningSettingsSection } from "../settings/ReasoningSettingsSection";
import { useState } from "react";
import { useNavigateToSession } from "../../lib/navigate-to-session";
import { useTranslation } from "react-i18next";
import { useOutcome, useOutcomePlan, useOutcomeProof } from "../../hooks/useOutcome";
import { useEventsConnection } from "../../hooks/useEventsConnection";
import { useWorkspaceQuery } from "../../hooks/useWorkspaceQuery";
import { Button } from "../ui/button";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "../ui/sheet";
import { OutcomeDecideAuthorizeSurface } from "./OutcomeDecideAuthorizeSurface";
import { OutcomeRunSurface } from "./OutcomeRunSurface";
import { OutcomeProveCloseSurface } from "./OutcomeProveCloseSurface";
import { OutcomeDeliveryPanel } from "./OutcomeDeliveryPanel";
import { OutcomeDocumentsPanel } from "./OutcomeDocumentsPanel";
import type { OutcomeDestinationStage } from "../../lib/outcome-tree";

/** Locked 2026-09-17 surface: four tabs (overview / plan / run / result); Decision history is a drawer, never a tab. */
type MissionTab = "overview" | "plan" | "run" | "result";

/** One identity boundary for all direct-Outcome views and their local drafts. */
export function OutcomeMissionPanel({
	outcomeId,
	projectId,
	stage,
	expanded,
	onExpand,
	onClose,
}: {
	outcomeId: string;
	projectId: string;
	stage?: OutcomeDestinationStage | "act_observe" | "prove_close";
	expanded: boolean;
	onExpand: () => void;
	onClose: () => void;
}) {
	const { t } = useTranslation();
	const navigateToSession = useNavigateToSession();
	const query = useOutcome(outcomeId);
	const planQuery = useOutcomePlan(outcomeId);
	const proofQuery = useOutcomeProof(outcomeId);
	const connection = useEventsConnection();
	const settings = useSettings();
	const reasoningUnavailable = settings.settings && !settings.settings.reasoning.ready;
	const projects = useWorkspaceQuery();
	const [tab, setTab] = useState<MissionTab>(
		stage === "prove_close" ? "result" : stage === "act_observe" ? "run" : "overview",
	);
	const [historyOpen, setHistoryOpen] = useState(false);
	const outcome = query.outcome;
	const plan = planQuery.plan;
	const runQuery = useOutcomeRunState(outcomeId);
 const attention = runStateAttention(runQuery.data, runQuery.error?.message);
	const stale = Boolean(plan && outcome && plan.contractRevisionNumber !== outcome.currentRevisionNumber);
	return (
		<section
			className="mx-auto flex h-full min-h-0 min-w-0 w-full max-w-5xl flex-col px-4 py-3"
			aria-label={outcome?.title ?? t("mission.overview")}
			data-testid="outcome-mission-panel"
		>
			<header className="flex shrink-0 flex-col gap-2 border-b border-border pb-3">
				<div className="flex flex-wrap justify-between gap-2">
					<span className="text-xs text-muted-foreground">
						{projects.data?.find((project) => project.id === projectId)?.name ?? projectId}
					</span>
					<div className="flex gap-1">
						<Button data-testid="mission-history-open" size="sm" variant="ghost" onClick={() => setHistoryOpen(true)}>
							<History aria-hidden="true" className="size-4" />
							{t("mission.history")}
						</Button>
 <OutcomeDeletionControls outcomeId={outcomeId} onRemoved={onClose} />
						<Button className="hidden @[1050px]/mission:inline-flex" size="sm" variant="ghost" onClick={onExpand}>
							{expanded ? (
								<Minimize2 aria-hidden="true" className="size-3.5" />
							) : (
								<Maximize2 aria-hidden="true" className="size-3.5" />
							)}
							{t(expanded ? "mission.restore" : "mission.expand")}
						</Button>
						<Button size="sm" variant="ghost" onClick={onClose}>
							<X aria-hidden="true" className="size-4" />
							{t("mission.close")}
						</Button>
					</div>
				</div>
				<h2 className="break-words text-base font-medium">{outcome?.title ?? t("outcome.overview.loading")}</h2>
				{outcome && (
					<p className="text-xs text-muted-foreground">
						{t("mission.revisions", {
							contract: outcome.currentRevisionNumber,
							plan: plan?.number ?? t("mission.noPlan"),
						})}{" "}
						· {t("mission.updated", { time: new Date(outcome.updatedAt).toLocaleString() })}
					</p>
				)}
				<p role="status" className="text-xs text-muted-foreground">
					{t(connection === "connected" ? "mission.connected" : "mission.offline")}
				</p>
				{connection !== "connected" && (
					<Button
						className="self-start"
						size="sm"
						variant="outline"
						onClick={() => {
							query.refetch();
                            runQuery.refetch();
							planQuery.refetch();
						}}
					>
						{t("mission.refresh")}
					</Button>
				)}
				{stale && (
					<p role="alert" className="text-xs text-warning">
						{t("mission.stale")}
					</p>
				)}
			</header>
			{query.failure ? (
				<div role="alert" className="p-3 text-sm">
					{query.failure.message}
					<Button onClick={query.refetch}>{t("outcome.understand.retry")}</Button>
				</div>
			) : (
				outcome && (
					<>
						<MissionGlance
							attention={attention}
							connection={connection}
							goal={outcome.currentRevision.goal}
							loading={runQuery.isLoading}
							onRefresh={() => {
								void query.refetch();
								void runQuery.refetch();
								void planQuery.refetch();
							}}
							onSelect={setTab}
						/>
						<nav
							aria-label={t("outcome.dashboard.missionControlAria", { title: outcome.title })}
							className="flex shrink-0 flex-wrap gap-1 py-2"
						>
							{(["overview", "plan", "run", "result"] as const).map((view) => (
								<Button
									data-testid={`mission-tab-${view}`}
									key={view}
									size="sm"
									variant={tab === view ? "secondary" : "ghost"}
									aria-pressed={tab === view}
									onClick={() => setTab(view)}
								>
									{t(`mission.${view}`)}
								</Button>
							))}
						</nav>
						<div className="min-h-0 flex-1 overflow-y-auto overscroll-contain pr-2 pb-6">
							<div hidden={tab !== "overview"}>
								<MissionContractEditor key={outcomeId} outcomeId={outcomeId} contract={outcome.currentRevision} disabled={connection !== "connected"} />
								<ContractOverview contract={outcome.currentRevision} />
								<OutcomeDocumentsPanel outcomeId={outcomeId} />
								<Button data-testid="mission-review-plan-cta" className="mt-4" onClick={() => setTab("plan")}>
									{t("outcome.dashboard.reviewPlan")}
								</Button>
							</div>
							<div hidden={tab !== "plan"}>
								<p className="mb-3 text-xs text-muted-foreground">{t("mission.authorization")}</p>
								{reasoningUnavailable && (
									<section className="mb-4 rounded-md border border-border p-3">
										<p className="mb-2 text-sm">
											{settings.settings?.reasoning.error || t("settings.reasoning.missing")}
										</p>
										<details>
											<summary className="cursor-pointer text-sm">{t("settings.reasoning.title")}</summary>
											<ReasoningSettingsSection />
										</details>
									</section>
								)}
								<fieldset
									disabled={connection !== "connected" || Boolean(reasoningUnavailable && !plan)}
									className="min-w-0"
								>
									<OutcomeDecideAuthorizeSurface
										onReviewContract={() => setTab("overview")}
										onReviewWork={() => setTab("run")}
										outcomeId={outcomeId}
									/>
								</fieldset>
							</div>
							<div hidden={tab !== "run"}>
								<OutcomeRunSurface
									projectId={projectId}
									onOpenSession={(sessionId) => navigateToSession(projectId, sessionId)}
									outcomeId={outcomeId}
									admissionBlocked={stale || connection !== "connected"}
									onReviewProof={() => setTab("result")}
								/>
								<MissionUsage outcomeId={outcomeId} projectId={projectId} />
							</div>
							<div hidden={tab !== "result"}>
								<fieldset disabled={connection !== "connected"} className="min-w-0">
									<OutcomeProveCloseSurface outcomeId={outcomeId} />
								</fieldset>
								<OutcomeDeliveryPanel outcomeId={outcomeId} />
							</div>
						</div>
						<Sheet open={historyOpen} onOpenChange={setHistoryOpen}>
							<SheetContent aria-describedby={undefined} className="w-full sm:max-w-md" side="right">
								<SheetHeader>
									<SheetTitle>{t("mission.history")}</SheetTitle>
								</SheetHeader>
								<div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 pb-6" data-testid="mission-history-drawer">
									<MissionHistory decisions={proofQuery.proof?.decisions ?? []} history={outcome.history} />
								</div>
							</SheetContent>
						</Sheet>
					</>
				)
			)}
		</section>
	);
}

function MissionGlance({
	attention,
	connection,
	goal,
	loading,
	onRefresh,
	onSelect,
}: {
	attention: ReturnType<typeof runStateAttention>;
	connection: ReturnType<typeof useEventsConnection>;
	goal: string;
	loading: boolean;
	onRefresh: () => void;
	onSelect: (tab: MissionTab) => void;
}) {
	const { t } = useTranslation();
	const nextTab: MissionTab = attention.lane === "define"
		? "overview"
		: attention.lane === "authorize"
			? "plan"
			: attention.lane === "review" || attention.lane === "accepted"
				? "result"
				: "run";
	const nextLabel = attention.lane === "unavailable"
		? t("mission.refresh")
		: t(`mission.${nextTab}`);
	const nextDescription = attention.reason
		? t(`mission.reason.${attention.reason}`, { defaultValue: attention.reason })
		: t(`mission.next.${attention.lane}`);

	return (
		<section className="flex flex-col gap-3 rounded-card hairline border-border bg-card px-3.5 py-3" data-testid="mission-glance">
			<div className="flex flex-col gap-1">
				<span className="text-2xs font-medium uppercase tracking-wide text-muted-foreground">{t("mission.objective")}</span>
				<p className="whitespace-pre-wrap break-words text-sm leading-body">{goal}</p>
			</div>
			<div className="flex flex-wrap items-end justify-between gap-3">
				<div className="min-w-0 flex-1">
					<span className="text-2xs font-medium uppercase tracking-wide text-muted-foreground">{t("mission.state")}</span>
					{loading ? (
						<p className="text-sm font-medium text-muted-foreground">
							{t("outcome.overview.loading")}
						</p>
					) : (
						<p
							className={cn(
								"flex items-center gap-1.5 text-sm font-medium",
								BOARD_LANE_TONE[boardLane(attention.lane)].text,
							)}
						>
							<span
								aria-hidden="true"
								className={cn(
									"size-2 shrink-0 rounded-full",
									BOARD_LANE_TONE[boardLane(attention.lane)].dot,
								)}
							/>
							{t(`mission.lane.${attention.lane}`)}
						</p>
					)}
					<p className="text-xs leading-body text-muted-foreground">
						{connection !== "connected" ? t("mission.offline") : loading ? t("outcome.overview.loading") : nextDescription}
					</p>
				</div>
				<Button
					data-testid="mission-glance-cta"
					className="shrink-0"
					onClick={attention.lane === "unavailable" ? onRefresh : () => onSelect(nextTab)}
					size="sm"
					variant="outline"
				>
					{nextLabel}
				</Button>
			</div>
		</section>
	);
}

function MissionHistory({
	decisions,
	history,
}: {
	decisions: NonNullable<ReturnType<typeof useOutcomeProof>["proof"]>["decisions"];
	history: NonNullable<ReturnType<typeof useOutcome>["outcome"]>["history"];
}) {
	const { t } = useTranslation();
	if (decisions.length === 0 && history.length === 0) {
		return <p className="text-xs text-muted-foreground">{t("mission.none")}</p>;
	}
	return (
		<div className="flex flex-col gap-6">
			{decisions.length > 0 && (
				<ul className="space-y-3">
					{decisions.map((decision) => (
						<li key={decision.id} className="border-b border-border pb-2 text-xs">
							<span className="font-medium">
								{decision.kind} · {decision.actorType}
							</span>
							<p>{decision.summary}</p>
							<time dateTime={decision.createdAt}>{new Date(decision.createdAt).toLocaleString()}</time>
						</li>
					))}
				</ul>
			)}
			{history.length > 0 && (
				<ol className="space-y-4">
					{history.map((revision) => (
						<li key={revision.id}>
							<h3 className="text-sm font-medium">
								{t("mission.historyRevision", {
									number: revision.number,
									time: new Date(revision.createdAt).toLocaleString(),
								})}
							</h3>
							<p className="whitespace-pre-wrap text-xs text-muted-foreground">{revision.goal}</p>
						</li>
					))}
				</ol>
			)}
		</div>
	);
}

function ContractOverview({
	contract,
}: {
	contract: NonNullable<ReturnType<typeof useOutcome>["outcome"]>["currentRevision"];
}) {
	const { t } = useTranslation();
	return (
		<div className="divide-y divide-border rounded-xl border border-border bg-card text-sm [&>div]:px-4 [&>div]:py-3">
			<div>
			<h3 className="font-medium">{t("mission.goal")}</h3>
			<div className="mt-2 whitespace-pre-wrap break-words">{contract.goal}</div>
			</div>
			{(
				[
					["criteria", contract.criteria.map((criterion) => criterion.text)],
					["constraints", contract.constraints],
					["nonGoals", contract.nonGoals],
					["stops", contract.stopConditions ?? []],
				] as const
			).map(([label, values]) => (
				<div key={label}>
					<details open={label === "criteria"}>
					<summary className="cursor-pointer font-medium">{t(`mission.${label}`)} <span className="text-muted-foreground">({values.length})</span></summary>
					<div className="text-muted-foreground">
						{values.length ? (
							<ul className="list-disc space-y-1 pl-4">
								{values.map((value, index) => (
									<li key={index} className="whitespace-pre-wrap break-words">
										{value}
									</li>
								))}
							</ul>
						) : (
							t("mission.none")
						)}
					</div>
					</details>
				</div>
			))}
			<div>
				<h3 className="mb-1 font-medium">{t("mission.review")}</h3>
				<div>{contract.review || t("mission.none")}</div>
			</div>
			<div>
				<h3 className="mb-1 font-medium">{t("mission.permissions")}</h3>
				<div className="text-xs text-muted-foreground">
					{contract.authorityCeiling ? (
						<ul className="grid grid-cols-2 gap-2">
							{Object.entries(contract.authorityCeiling).map(([name, allowed]) => (
								<li key={name}>
									{t(`mission.permission.${name}`, { defaultValue: name })}:{" "}
									{t(allowed ? "mission.allowed" : "mission.denied")}
								</li>
							))}
						</ul>
					) : (
						t("mission.unknown")
					)}
				</div>
			</div>
		</div>
	);
}
