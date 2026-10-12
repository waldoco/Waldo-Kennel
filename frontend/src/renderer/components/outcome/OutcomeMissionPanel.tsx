import { OutcomeDeletionControls } from "./OutcomeDeletionControls";
import { MissionContractEditor } from "./MissionContractEditor";
import { useOutcomeRunState } from "../../hooks/useOutcomeRunState";
import { Check, ChevronRight, Clock, Minus, X, Maximize2, Minimize2, History } from "lucide-react";
import { runStateAttention } from "../../lib/mission-attention";
import { attentionTone } from "../../lib/mission-lane-tone";
import { StateLabel, StateMark } from "@pin4sf/kennel-product-ui";
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
			<header className="flex shrink-0 flex-col gap-2 hairline-b border-wd-hairline pb-4">
				<div className="flex flex-wrap items-center justify-between gap-2">
					<span className="text-sm text-wd-secondary">
						{projects.data?.find((project) => project.id === projectId)?.name ?? projectId}
					</span>
					<div className="flex gap-1">
						<Button data-testid="mission-history-open" size="sm" variant="ghost" onClick={() => setHistoryOpen(true)}>
							<History aria-hidden="true" className="size-4" />
							{t("mission.history")}
						</Button>
 <OutcomeDeletionControls outcomeId={outcomeId} onRemoved={onClose} />
						<Button className="hidden @[960px]/mission:inline-flex" size="sm" variant="ghost" onClick={onExpand}>
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
				{/* The Outcome's name is the screen's headline, set in Mottle: the
				    person's own words. Loading is said plainly, not in display type. */}
				<h2 className={cn("break-words", outcome ? "wd-display-3 text-wd-primary" : "text-sm text-wd-secondary")}>
					{outcome?.title ?? t("outcome.overview.loading")}
				</h2>
				{outcome && (
					<p className="wd-data text-wd-tertiary">
						{t("mission.revisions", {
							contract: outcome.currentRevisionNumber,
							plan: plan?.number ?? t("mission.noPlan"),
						})}{" "}
						· {t("mission.updated", { time: new Date(outcome.updatedAt).toLocaleString() })}
					</p>
				)}
				<p role="status" className="flex items-center gap-1.5 text-xs text-wd-secondary">
					<StateMark
						className={connection === "connected" ? "text-good" : "text-caution"}
						tone={connection === "connected" ? "good" : "caution"}
					/>
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
					<p role="alert" className="flex items-start gap-2 rounded-md bg-caution-bg px-3 py-2 text-sm text-caution">
						<Clock aria-hidden="true" className="mt-0.5 size-icon-sm shrink-0" />
						{t("mission.stale")}
					</p>
				)}
			</header>
			{query.failure ? (
				<div role="alert" className="mt-4 flex items-center gap-3 rounded-lg hairline border-wd-hairline bg-wd-surface px-4 py-3 text-sm text-wd-primary">
					<span className="min-w-0 flex-1">{query.failure.message}</span>
					<Button onClick={query.refetch} size="sm" variant="ghost">{t("outcome.understand.retry")}</Button>
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
						{/* Segmented control: inset track, ink thumb on the open view. */}
						<nav
							aria-label={t("outcome.dashboard.missionControlAria", { title: outcome.title })}
							className="my-3 inline-flex shrink-0 self-start rounded-full bg-wd-inset p-0.5"
						>
							{(["overview", "plan", "run", "result"] as const).map((view) => (
								<button
									data-testid={`mission-tab-${view}`}
									key={view}
									type="button"
									aria-pressed={tab === view}
									className={cn(
										"inline-flex h-7 items-center rounded-full px-3.5 text-sm font-medium transition-colors duration-fast ease-out motion-reduce:transition-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)",
										tab === view ? "bg-wd-inverse text-wd-on-inverse" : "text-wd-secondary hover:text-wd-primary",
									)}
									onClick={() => setTab(view)}
								>
									{t(`mission.${view}`)}
								</button>
							))}
						</nav>
						<div className="min-h-0 flex-1 overflow-y-auto overscroll-contain pr-2 pb-6">
							<div hidden={tab !== "overview"}>
								<MissionContractEditor key={outcomeId} outcomeId={outcomeId} contract={outcome.currentRevision} disabled={connection !== "connected"} />
								<ContractOverview contract={outcome.currentRevision} />
								<OutcomeDocumentsPanel outcomeId={outcomeId} />
								<Button data-testid="mission-review-plan-cta" className="mt-4" onClick={() => setTab("plan")} variant="outline">
									{t("outcome.dashboard.reviewPlan")}
								</Button>
							</div>
							<div hidden={tab !== "plan"}>
								<p className="mb-3 text-sm text-wd-secondary">{t("mission.authorization")}</p>
								{reasoningUnavailable && (
									<section className="mb-4 flex flex-col gap-2 rounded-lg hairline border-wd-hairline bg-wd-surface p-4">
										<p className="text-sm text-wd-primary">
											{settings.settings?.reasoning.error || t("settings.reasoning.missing")}
										</p>
										<details>
											<summary className="cursor-pointer text-sm font-medium text-wd-primary">{t("settings.reasoning.title")}</summary>
											<ReasoningSettingsSection />
										</details>
									</section>
								)}
								<fieldset
									disabled={connection !== "connected" || Boolean(reasoningUnavailable && !plan)}
									className="min-w-0"
								>
									<OutcomeDecideAuthorizeSurface
										disabled={connection !== "connected"}
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
		<section className="mt-4 flex flex-col gap-4 rounded-lg hairline border-wd-hairline bg-wd-surface p-4" data-testid="mission-glance">
			<div className="flex flex-col gap-1">
				<span className="text-xs font-medium text-wd-secondary">{t("mission.objective")}</span>
				<p className="whitespace-pre-wrap break-words text-sm leading-5 text-wd-primary">{goal}</p>
			</div>
			<div className="flex flex-wrap items-end justify-between gap-3">
				<div className="flex min-w-0 flex-1 flex-col gap-1">
					<span className="text-xs font-medium text-wd-secondary">{t("mission.state")}</span>
					{loading ? (
						// A specific line, not a spinner: the run state is being read.
						<p className="text-sm text-wd-secondary">{t("mission.run.loading")}</p>
					) : (
						<p className="text-sm font-medium">
							<StateLabel label={t(`mission.lane.${attention.lane}`)} tone={attentionTone(attention)} />
						</p>
					)}
					{connection !== "connected" ? (
						<p className="text-sm leading-5 text-wd-secondary">{t("mission.offline")}</p>
					) : loading ? null : (
						<p className="text-sm leading-5 text-wd-secondary">{nextDescription}</p>
					)}
				</div>
				<Button
					data-testid="mission-glance-cta"
					className="shrink-0"
					onClick={attention.lane === "unavailable" ? onRefresh : () => onSelect(nextTab)}
					size="sm"
					variant="primary"
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
		return <p className="text-sm text-wd-secondary">{t("mission.none")}</p>;
	}
	return (
		<div className="flex flex-col gap-6">
			{decisions.length > 0 && (
				<ul className="flex flex-col">
					{decisions.map((decision) => (
						<li key={decision.id} className="flex flex-col gap-1 hairline-b border-wd-hairline py-3 last:border-b-0">
							<span className="text-sm font-medium text-wd-primary">
								{decision.kind} · {decision.actorType}
							</span>
							<p className="text-sm text-wd-secondary">{decision.summary}</p>
							<time className="wd-data text-wd-tertiary" dateTime={decision.createdAt}>
								{new Date(decision.createdAt).toLocaleString()}
							</time>
						</li>
					))}
				</ul>
			)}
			{history.length > 0 && (
				<ol className="flex flex-col">
					{history.map((revision) => (
						<li key={revision.id} className="flex flex-col gap-1 hairline-b border-wd-hairline py-3 last:border-b-0">
							<h3 className="text-sm font-medium text-wd-primary">
								{t("mission.historyRevision", {
									number: revision.number,
									time: new Date(revision.createdAt).toLocaleString(),
								})}
							</h3>
							<p className="whitespace-pre-wrap text-sm text-wd-secondary">{revision.goal}</p>
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
		<div className="mt-4 divide-y-[length:var(--stroke-hairline)] divide-wd-hairline rounded-lg hairline border-wd-hairline bg-wd-surface text-sm [&>div]:px-4 [&>div]:py-3">
			<div>
				<h3 className="text-sm font-medium text-wd-secondary">{t("mission.goal")}</h3>
				<div className="mt-1 whitespace-pre-wrap break-words text-wd-primary">{contract.goal}</div>
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
					<details className="group/contract-section" open={label === "criteria"}>
						<summary className="flex cursor-pointer list-none items-center gap-2 font-medium text-wd-primary [&::-webkit-details-marker]:hidden">
							<ChevronRight aria-hidden="true" className="size-icon-sm text-wd-tertiary transition-transform duration-fast group-open/contract-section:rotate-90 motion-reduce:transition-none" />
							{t(`mission.${label}`)}
							<span className="wd-data ml-auto text-wd-tertiary">{values.length}</span>
						</summary>
						<div className="mt-2 pl-6 text-wd-secondary">
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
				<h3 className="mb-1 text-sm font-medium text-wd-secondary">{t("mission.review")}</h3>
				<div className="text-wd-primary">{contract.review || t("mission.none")}</div>
			</div>
			<div>
				<h3 className="mb-2 text-sm font-medium text-wd-secondary">{t("mission.permissions")}</h3>
				<div className="text-sm text-wd-primary">
					{contract.authorityCeiling ? (
						<ul className="grid grid-cols-2 gap-x-4 gap-y-1.5">
							{Object.entries(contract.authorityCeiling).map(([name, allowed]) => (
								<li key={name} className="flex items-center gap-1.5">
									{allowed ? (
										<Check aria-hidden="true" className="size-icon-sm shrink-0 text-wd-primary" />
									) : (
										<Minus aria-hidden="true" className="size-icon-sm shrink-0 text-wd-tertiary" />
									)}
									<span className={allowed ? "text-wd-primary" : "text-wd-secondary"}>
										{t(`mission.permission.${name}`, { defaultValue: name })}:{" "}
										{t(allowed ? "mission.allowed" : "mission.denied")}
									</span>
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
