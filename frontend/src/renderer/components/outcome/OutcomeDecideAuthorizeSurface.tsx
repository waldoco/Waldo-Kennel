import { useQueryClient } from "@tanstack/react-query";
import {
	BadgeCheck,
	CheckCircle2,
	ChevronDown,
	Crosshair,
	FileText,
	ListChecks,
	PauseCircle,
	ShieldCheck,
	Sparkles,
	Loader2,
} from "lucide-react";
import { type ReactNode, useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

import {
	PLAN_CAPABILITY_UNAUTHORIZED,
	PLAN_CONTRACT_STALE,
	refetchOutcome,
	useApproveOutcomePlan,
	useOutcome,
	useOutcomePlan,
	useOutcomeProof,
	useOutcomeSchedule,
	type OutcomeFailure,
	type PlanRecord,
} from "../../hooks/useOutcome";
import type { components } from "../../../api/schema";
import { MissionPlanningConversation } from "./MissionPlanningConversation";
import { MissionPlanView } from "./MissionPlanView";
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "../ui/accordion";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";

type OutcomeDecideAuthorizeSurfaceProps = {
	outcomeId: string;
	/** Returns to observed session activity without claiming an Attempt was started. */
	onReviewWork?: () => void;
	/** Returns to the Contract editor when planning proposes a Contract change. */
	onReviewContract?: () => void;
	/** Disable owner decisions while live facts cannot be kept current. */
	disabled?: boolean;
};

type ContractRevisionRecord = components["schemas"]["ContractRevisionResponse"];

/** Every plan section stays open by default — the plan is short enough that
 *  collapsing loses more than it saves, and each section still toggles. */
const PLAN_SECTION_VALUES = ["summary", "desired-state", "evidence", "verification", "pause-trigger", "permissions", "brief"];

/**
 * Decide & Authorize: "What exactly may the agent do, and who says so?"
 *
 * The surface renders only what the daemon answers. Proposing is a read-mostly
 * operation (intelligence proposes against the current contract),
 * and Approve is the owner's authority gate: nothing executes until it lands,
 * and a contract that moved ahead forces a fresh brief instead of a silent
 * authority transfer.
 */
export function OutcomeDecideAuthorizeSurface({ outcomeId, onReviewWork, onReviewContract, disabled = false }: OutcomeDecideAuthorizeSurfaceProps) {
	const { t } = useTranslation();
	const queryClient = useQueryClient();

	const outcomeQuery = useOutcome(outcomeId);
	const planQuery = useOutcomePlan(outcomeId);
	const approve = useApproveOutcomePlan(outcomeId);
	const [approving, setApproving] = useState(false);

	const pending = approve.pending || approving;
	const failure = approve.failure ?? planQuery.failure;
	const plan = planQuery.plan;

	async function approvePlan() {
		const outcome = outcomeQuery.outcome;
		if (!plan || !outcome || pending) return;
		setApproving(true);
		try {
			await approve.approve({
				planId: plan.id,
				expectedContractRevision: outcome.currentRevisionNumber,
			});
		} catch {
			// Failure state derives from the mutation's typed error.
		} finally {
			setApproving(false);
		}
	}

	async function reloadCurrentFacts() {
		approve.reset();
		if (outcomeQuery.outcome) {
			try {
				await refetchOutcome(queryClient, outcomeQuery.outcome.id);
			} catch {
				// Keep the conflict card up; the retry stays available.
			}
		}
		planQuery.refetch();
	}

	// eslint-disable-next-line @typescript-eslint/no-unsafe-assignment
	const __dbg = {
		hasPlan: Boolean(plan),
		isLoadingPlan: planQuery.isLoading,
		failureKind: failure ? `${failure.kind}:${failure.code ?? ""}` : "",
		outcomeLoaded: Boolean(outcomeQuery.outcome),
		fetchStatusPlan: (planQuery as unknown as { fetchStatus?: string }).fetchStatus,
		statusPlan: (planQuery as unknown as { status?: string }).status,
		errorPlan: planQuery.failure ? String((planQuery.failure as OutcomeFailure).message) : null,
	} as const;
	const showDbg = new URLSearchParams(typeof window !== "undefined" ? window.location.search : "").has("__dbg");
	const isStaleConflict = failure?.code === PLAN_CONTRACT_STALE || Boolean(plan && outcomeQuery.outcome && plan.contractRevisionNumber !== outcomeQuery.outcome.currentRevisionNumber);
	const isAuthorityBlocked = failure?.code === PLAN_CAPABILITY_UNAUTHORIZED;

	return (
		<div className="flex flex-col gap-5">
			<div className="max-w-xl">
				<h2 className="text-base font-medium">{t("outcome.decide.heading")}</h2>
				<p className="text-muted-foreground text-sm">{t("outcome.decide.intro")}</p>
			</div>

			{showDbg && (
				<pre data-testid="decide-debug">{JSON.stringify({ ...__dbg, failureRaw: failure ?? null })}</pre>
			)}

			{plan && <PlanReviewCard outcome={outcomeQuery.outcome} outcomeId={outcomeId} plan={plan} />}

			{outcomeQuery.outcome && (plan ? (
				<details className="mx-auto w-full max-w-2xl rounded-group hairline border-border bg-card px-4.5 py-3.5">
					<summary className="cursor-pointer text-sm font-medium">{t("planning.details")}</summary>
					<div className="mt-3">
						<MissionPlanningConversation
							contractRevision={outcomeQuery.outcome.currentRevisionNumber}
							disabled={disabled}
							onReviewContract={onReviewContract}
							outcomeId={outcomeId}
						/>
					</div>
				</details>
			) : (
				<MissionPlanningConversation
					contractRevision={outcomeQuery.outcome.currentRevisionNumber}
					disabled={disabled}
					onReviewContract={onReviewContract}
					outcomeId={outcomeId}
				/>
			))}

			{!plan && !planQuery.isLoading && !failure && !outcomeQuery.outcome && null}

			{plan?.status === "proposed" && !isStaleConflict && !isAuthorityBlocked && (
				<div className="mx-auto flex w-full max-w-2xl flex-col gap-2">
					<div className="flex items-center justify-between gap-3">
						{/* Update stays on the existing revise path; Authorize stays on
						    the existing approve mutation. Neither gains new authority. */}
						{onReviewContract ? (
							<Button data-testid="outcome-plan-update" onClick={onReviewContract} type="button" variant="ghost">
								{t("outcome.decide.updateCta")}
							</Button>
						) : <span />}
						<Button data-testid="outcome-approve-plan" disabled={pending} onClick={() => void approvePlan()} variant="primary">
							{pending && <Loader2 aria-hidden="true" className="size-3.5 animate-spin" />}
							<ShieldCheck aria-hidden="true" className="size-3.5" />
							{t("outcome.decide.authorizeCta")}
						</Button>
					</div>
					<p className="text-muted-foreground text-xs">{t("outcome.decide.approveNote")}</p>
				</div>
			)}

			{plan?.status === "approved" && onReviewWork && (
				<div className="mx-auto flex w-full max-w-2xl flex-col items-end gap-2">
					<Button data-testid="outcome-review-work" onClick={onReviewWork} type="button" variant="secondary">
						{t("outcome.decide.startSessionsCta")}
					</Button>
					<p className="text-muted-foreground text-xs">{t("outcome.decide.reviewWorkNote")}</p>
				</div>
			)}

			{isStaleConflict && (
				<div className="max-w-xl rounded-group hairline border-warning/40 bg-warning/5 px-4.5 py-3.5" data-testid="outcome-plan-conflict">
					<h3 className="text-sm font-medium">{t("outcome.decide.staleTitle")}</h3>
					<p className="mt-1 text-muted-foreground text-sm">{t("outcome.decide.staleBody")}</p>
					<Button
						className="mt-3"
						data-testid="outcome-plan-reload"
						onClick={() => void reloadCurrentFacts()}
						size="sm"
						type="button"
						variant="outline"
					>
						{t("outcome.decide.reloadCta")}
					</Button>
				</div>
			)}

			{isAuthorityBlocked && (
				<div className="max-w-xl rounded-group hairline border-border bg-card px-4.5 py-3.5" data-testid="outcome-authority-blocked">
					<h3 className="text-sm font-medium">{t("outcome.decide.blockedTitle")}</h3>
					<p className="mt-1 text-muted-foreground text-sm">{failure?.message}</p>
				</div>
			)}

			{!isStaleConflict && !isAuthorityBlocked && failure && failure.code !== PLAN_CONTRACT_STALE && (
				<PlanFailureBanners failure={failure} onRetry={() => planQuery.refetch()} />
			)}
		</div>
	);
}

function PlanReviewCard({
	outcome,
	outcomeId,
	plan,
}: {
	outcome?: components["schemas"]["OutcomeResponse"];
	outcomeId: string;
	plan: PlanRecord;
}) {
	const { t } = useTranslation();
	const [view, setView] = useState<"plan" | "graph">("plan");
	const scheduleQuery = useOutcomeSchedule(outcomeId, plan.status === "approved" ? plan.id : undefined);
	// Criterion text comes from the canonical proof read, so nodes and rows show
	// the owner's own words instead of criterion ids.
	const proofQuery = useOutcomeProof(outcomeId);
	const criterionText = useCallback(
		(criterionId: string) =>
			proofQuery.proof?.criteria.find((criterion) => criterion.criterionId === criterionId)?.text,
		[proofQuery.proof],
	);
	const titleOf = useMemo(
		() => new Map(plan.workUnits.map((workUnit) => [workUnit.id, workUnit.title])),
		[plan.workUnits],
	);
	// The plan binds one contract revision; the cards read THAT revision, never
	// the newer one the Outcome may have moved to. A revision the daemon no
	// longer returns renders as "not reported", not as silently current facts.
	const boundRevision: ContractRevisionRecord | undefined = useMemo(() => {
		const revisions = [outcome?.currentRevision, ...(outcome?.history ?? [])].filter(
			(revision): revision is ContractRevisionRecord => Boolean(revision),
		);
		return revisions.find((revision) => revision.number === plan.contractRevisionNumber);
	}, [outcome, plan.contractRevisionNumber]);
	const evidenceByCriterion = useMemo(() => {
		const map = new Map<string, string[]>();
		for (const expectation of boundRevision?.evidenceExpectations ?? []) {
			map.set(expectation.criterionId, expectation.descriptions);
		}
		return map;
	}, [boundRevision]);
	// Contract-level stop conditions win; per-unit conditions are the fallback
	// the daemon actually enforces when the contract stayed silent.
	const pauseTriggers = useMemo(() => {
		const contractStops = (boundRevision?.stopConditions ?? []).filter((line) => line.trim());
		if (contractStops.length > 0) return contractStops;
		const unitStops = plan.workUnits.flatMap((workUnit) => workUnit.stopConditions ?? []).filter((line) => line.trim());
		return [...new Set(unitStops)];
	}, [boundRevision, plan.workUnits]);
	const unit = plan.workUnits[0];
	const assumptions = plan.assumptions ?? [];
	const blockers = plan.blockers ?? [];
	const routingDecisions = plan.routingDecisions ?? [];
	const grants = plan.grants ?? [];
	return (
		<section className="mx-auto flex w-full max-w-2xl flex-col gap-2" data-testid="outcome-plan-card">
			<div className="flex flex-wrap items-center justify-between gap-3 rounded-lg hairline border-border bg-card px-4 py-3" data-testid="outcome-plan-summary">
				<div className="flex min-w-0 items-center gap-3">
					<h3 className="min-w-0 truncate text-sm font-medium text-foreground">{plan.summary || unit?.title}</h3>
					<Badge variant={plan.status === "approved" ? "success" : "accent"}>
						{plan.status === "approved"
							? t("outcome.decide.badgeApproved", { number: plan.number })
							: t("outcome.decide.badgeProposed", { number: plan.number })}
					</Badge>
				</div>
				<div aria-label={t("mission.plan")} className="flex shrink-0 gap-1" role="group">
					{(["plan", "graph"] as const).map((mode) => (
						<Button
							aria-pressed={view === mode}
							key={mode}
							onClick={() => setView(mode)}
							size="sm"
							variant={view === mode ? "secondary" : "ghost"}
						>
							{t(`mission.${mode}`)}
						</Button>
					))}
				</div>
			</div>
			{view === "graph" ? (
				/* Proposed topology has no schedule before authorization. The graph
				   remains a distinct view and never inherits the text-plan sections. */
				<div className="rounded-lg hairline border-border bg-card px-4.5 py-3.5" data-testid="outcome-plan-graph">
					{plan.status !== "approved" || scheduleQuery.schedule ? (
						<MissionPlanView
							criterionText={criterionText}
							graphOnly
							schedule={scheduleQuery.schedule}
							workUnits={plan.workUnits}
						/>
					) : <p className="text-xs text-muted-foreground">{scheduleQuery.failure?.message || t("mission.scheduleUnavailable")}</p>}
				</div>
			) : <div className="contents" data-testid="outcome-plan-details">
			<Accordion className="flex flex-col gap-2" defaultValue={PLAN_SECTION_VALUES} type="multiple">
				<PlanSection icon={<Sparkles aria-hidden="true" className="size-3.5" />} label={t("outcome.decide.factsSummary")} value="summary">
					<p className="text-sm leading-body text-foreground">{plan.summary || unit?.title || t("outcome.decide.notReported")}</p>
				</PlanSection>
				<PlanSection icon={<Crosshair aria-hidden="true" className="size-3.5" />} label={t("outcome.decide.factsDesiredState")} value="desired-state">
					{boundRevision ? (
						<>
							<p className="whitespace-pre-wrap text-sm leading-body text-foreground">{boundRevision.goal || t("outcome.decide.notReported")}</p>
							{(boundRevision.successCriteria ?? []).length > 0 && (
								<ul className="mt-2 flex list-disc flex-col gap-1 pl-4 text-xs leading-body text-muted-foreground">
									{boundRevision.successCriteria.map((criterion, index) => (
										<li key={index}>{criterion}</li>
									))}
								</ul>
							)}
						</>
					) : (
						<p className="text-xs text-passive">{t("outcome.decide.notReported")}</p>
					)}
				</PlanSection>
				<PlanSection icon={<ListChecks aria-hidden="true" className="size-3.5" />} label={t("outcome.decide.factsEvidence")} value="evidence">
					{boundRevision && (boundRevision.criteria ?? []).length > 0 ? (
						<ul className="flex flex-col gap-2">
							{(boundRevision.criteria ?? []).map((criterion, index) => (
								<li className="text-sm" key={criterion.criterionId ?? index}>
									<p className="text-foreground">{criterion.text}</p>
									{(evidenceByCriterion.get(criterion.criterionId) ?? []).length > 0 && (
										<ul className="mt-1 flex list-disc flex-col gap-1 pl-4 text-xs leading-body text-muted-foreground">
											{(evidenceByCriterion.get(criterion.criterionId) ?? []).map((line, lineIndex) => (
												<li key={lineIndex}>{line}</li>
											))}
										</ul>
									)}
								</li>
							))}
						</ul>
					) : (
						<p className="text-xs text-passive">{t("outcome.decide.notReported")}</p>
					)}
				</PlanSection>
				<PlanSection icon={<BadgeCheck aria-hidden="true" className="size-3.5" />} label={t("outcome.decide.factsVerification")} value="verification">
					{boundRevision?.review ? (
						<p className="whitespace-pre-wrap text-sm leading-body text-foreground">{boundRevision.review}</p>
					) : (
						<p className="text-xs text-passive">{t("outcome.decide.notReported")}</p>
					)}
					{plan.workUnits.some((workUnit) => workUnit.verificationRequirement?.trim()) && (
						<ul className="mt-2 flex list-disc flex-col gap-1 pl-4 text-xs leading-body text-muted-foreground">
							{plan.workUnits.filter((workUnit) => workUnit.verificationRequirement?.trim()).map((workUnit) => (
								<li key={workUnit.id}>{workUnit.title}: {workUnit.verificationRequirement}</li>
							))}
						</ul>
					)}
				</PlanSection>
				<PlanSection icon={<PauseCircle aria-hidden="true" className="size-3.5" />} label={t("outcome.decide.factsStops")} value="pause-trigger">
					{pauseTriggers.length > 0 ? (
						<ul className="flex list-disc flex-col gap-1 pl-4 text-sm leading-body text-foreground">
							{pauseTriggers.map((line, index) => (
								<li key={index}>{line}</li>
							))}
						</ul>
					) : (
						<p className="text-xs text-passive">{t("outcome.decide.notReported")}</p>
					)}
				</PlanSection>
				{/* Read-only on purpose: no verified grants mutation API exists, so
				    the card reports the plan's grants and says "Not reported" when
				    the plan carried none. */}
				<PlanSection icon={<ShieldCheck aria-hidden="true" className="size-3.5" />} label={t("outcome.decide.factsGrants")} value="permissions">
					{grants.length > 0 ? (
						<ul className="flex flex-col gap-1.5">
							{grants.map((grant) => (
								<li className="flex items-center gap-2.5 text-sm" key={grant.id}>
									<CheckCircle2 aria-hidden="true" className="size-3.5 shrink-0 text-success" />
									<code className="text-xs text-foreground">{grant.name}</code>
									<span className="text-2xs text-passive">{grant.scope}</span>
								</li>
							))}
						</ul>
					) : (
						<p className="text-xs text-passive" data-testid="outcome-plan-grants-empty">{t("outcome.decide.notReported")}</p>
					)}
				</PlanSection>
				<PlanSection icon={<FileText aria-hidden="true" className="size-3.5" />} label={t("outcome.decide.factsBrief")} value="brief">
					<code className="block break-all text-xs leading-body text-foreground/80">{plan.runBriefCoreDigest}</code>
					{assumptions.length > 0 && <p className="mt-2 text-xs text-warning">{t("outcome.decide.assumptionsLabel")}: {assumptions.join(" · ")}</p>}
					{blockers.length > 0 && <p className="mt-2 text-xs text-destructive">{t("outcome.decide.blockersLabel")}: {blockers.join(" · ")}</p>}
					{routingDecisions.length > 0 && <p className="mt-2 text-xs text-muted-foreground">{t("outcome.decide.routingLabel")}: {routingDecisions.map((decision) => `${decision.workUnitId} → ${decision.recommendedProvider || "no candidate"}`).join(" · ")}</p>}
				</PlanSection>
			</Accordion>

			<div className="grid gap-2" data-testid="outcome-plan-work-units">
				{plan.workUnits.map((workUnit, index) => (
					<div className="rounded-lg hairline border-border bg-card px-3.5 py-3" key={workUnit.id}>
						<div className="flex items-center justify-between gap-3">
							<span className="text-sm font-medium">{index + 1}. {workUnit.title}</span>
							<span className="text-xs text-muted-foreground">{workUnit.modelSelection === "provider_default" ? t("outcome.missionGraph.providerDefault") : workUnit.model || t("outcome.decide.modelUnreported")}</span>
						</div>
						<div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
							{/* Criteria and dependencies read as words, not ids: a raw
							    identifier is technical detail, not primary content. */}
							<span>{t("outcome.decide.criteriaLabel")}: {criteriaLabel(workUnit.criterionIds, criterionText) || t("outcome.decide.criteriaNotRecorded")}</span>
							<span>{t("outcome.decide.dependsOnLabel")}: {(workUnit.dependsOn ?? []).length > 0 ? (workUnit.dependsOn ?? []).map((id) => titleOf.get(id) ?? id).join(", ") : t("outcome.decide.dependsOnNone")}</span>
							<span>{t("outcome.decide.providerLabel")}: {workUnit.provider || t("outcome.decide.providerUnbound")}</span>
						</div>
					</div>
				))}
			</div>

			<p className="px-1 text-2xs text-passive">
				{t("outcome.decide.bindingNote", { contractRevision: plan.contractRevisionNumber })}
			</p>
			</div>}
		</section>
	);
}

function PlanSection({
	children,
	icon,
	label,
	value,
}: {
	children: ReactNode;
	icon: ReactNode;
	label: string;
	value: string;
}) {
	return (
		<AccordionItem className="overflow-hidden rounded-lg hairline border-border bg-card" value={value}>
			<AccordionTrigger
				className="group justify-between gap-2 px-2.5 py-2.5 text-left text-sm font-medium text-foreground"
				headerClassName="px-1"
			>
				<span className="flex min-w-0 items-center gap-2 text-passive [&>svg]:text-foreground">
					{icon}
					<span className="truncate text-foreground">{label}</span>
				</span>
				<ChevronDown
					aria-hidden="true"
					className="size-3.5 shrink-0 text-passive transition-transform duration-fast group-data-[state=open]:rotate-180"
				/>
			</AccordionTrigger>
			<AccordionContent className="px-2.5 pb-2.5">
				<div className="rounded-md hairline border-border bg-shell px-3.5 py-3">{children}</div>
			</AccordionContent>
		</AccordionItem>
	);
}

function PlanFailureBanners({ failure, onRetry }: { failure: OutcomeFailure; onRetry: () => void }) {
	const { t } = useTranslation();
	if (failure.kind === "offline") {
		return (
			<div className="max-w-xl rounded-group hairline border-border bg-card px-4.5 py-3.5" data-testid="outcome-plan-offline" role="alert">
				<h3 className="text-sm font-medium">{t("outcome.understand.offlineTitle")}</h3>
				<p className="mt-1 text-muted-foreground text-sm">{t("outcome.understand.offlineBody")}</p>
				<Button className="mt-3" onClick={onRetry} size="sm" type="button" variant="outline">
					{t("outcome.understand.retry")}
				</Button>
			</div>
		);
	}
	if (failure.kind === "retryable") {
		return (
			<div className="max-w-xl rounded-group hairline border-border bg-card px-4.5 py-3.5" data-testid="outcome-plan-retryable" role="alert">
				<h3 className="text-sm font-medium">{t("outcome.plan.failedTitle")}</h3>
				<p className="mt-1 text-muted-foreground text-sm">{failure.code === "PLAN_DRAFT_CHECK_INVALID" ? t("outcome.plan.invalidCheck") : failure.message}</p>
				{failure.code === "PLAN_DRAFT_CHECK_INVALID" && <p className="mt-2 break-words text-xs text-muted-foreground">{failure.message}</p>}
				<Button className="mt-3" onClick={onRetry} size="sm" type="button" variant="outline">
					{t("outcome.understand.retry")}
				</Button>
			</div>
		);
	}
	return (
		<p className="text-destructive text-sm" role="alert">
			{failure.message}
		</p>
	);
}

/**
 * Criterion text for one work unit, falling back to nothing rather than to ids.
 *
 * An unresolved criterion is better shown as "not recorded" than as an opaque
 * identifier the owner cannot act on.
 */
function criteriaLabel(
	criterionIds: string[] | undefined,
	criterionText: (criterionId: string) => string | undefined,
): string {
	return (criterionIds ?? [])
		.map((id) => criterionText(id))
		.filter((text): text is string => Boolean(text))
		.join(" · ");
}
