import { FactList, StateBadge, StateMark } from "@pin4sf/kennel-product-ui";
import { useQueryClient } from "@tanstack/react-query";
import {
	BadgeCheck,
	ChevronDown,
	Crosshair,
	FileText,
	ListChecks,
	PauseCircle,
	ShieldCheck,
	Sparkles,
} from "lucide-react";
import { type ReactNode, useCallback, useId, useMemo, useState } from "react";
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
import { Button } from "../ui/button";
import { cn } from "../../lib/utils";

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
				<h2 className="text-base font-medium text-wd-primary">{t("outcome.decide.heading")}</h2>
				<p className="text-sm text-wd-secondary">{t("outcome.decide.intro")}</p>
			</div>

			{showDbg && (
				<pre data-testid="decide-debug">{JSON.stringify({ ...__dbg, failureRaw: failure ?? null })}</pre>
			)}

			{plan && <PlanReviewCard outcome={outcomeQuery.outcome} outcomeId={outcomeId} plan={plan} />}

			{outcomeQuery.outcome && (plan ? (
				<details className="mx-auto w-full max-w-2xl rounded-lg hairline border-wd-hairline bg-wd-surface px-4 py-3">
					<summary className="cursor-pointer rounded-sm text-sm font-medium text-wd-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)">{t("planning.details")}</summary>
					<div className="mt-3">
						<MissionPlanningConversation
							contractRevision={outcomeQuery.outcome.currentRevisionNumber}
							disabled={disabled}
							outcomeId={outcomeId}
						/>
					</div>
				</details>
			) : (
				<MissionPlanningConversation
					contractRevision={outcomeQuery.outcome.currentRevisionNumber}
					disabled={disabled}
					outcomeId={outcomeId}
				/>
			))}

			{!plan && !planQuery.isLoading && !failure && !outcomeQuery.outcome && null}

			{plan?.status === "proposed" && !isStaleConflict && !isAuthorityBlocked && (
				<AuthorizeDecisionCard
					onApprove={() => void approvePlan()}
					onReviewContract={onReviewContract}
					pending={pending}
					plan={plan}
				/>
			)}

			{plan?.status === "approved" && onReviewWork && (
				<div className="mx-auto flex w-full max-w-2xl flex-col items-end gap-2">
					<Button data-testid="outcome-review-work" onClick={onReviewWork} type="button" variant="secondary">
						{t("outcome.decide.startSessionsCta")}
					</Button>
					<p className="text-xs text-wd-secondary">{t("outcome.decide.reviewWorkNote")}</p>
				</div>
			)}

			{isStaleConflict && (
				<div className="flex max-w-xl flex-col gap-2 rounded-lg hairline border-wd-hairline bg-wd-surface p-4" data-testid="outcome-plan-conflict">
					<StateBadge label={t("outcome.verdict.stale")} tone="caution" className="self-start" />
					<h3 className="text-sm font-medium text-wd-primary">{t("outcome.decide.staleTitle")}</h3>
					<p className="text-sm text-wd-secondary">{t("outcome.decide.staleBody")}</p>
					<Button
						className="mt-1 self-start"
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
				<div className="flex max-w-xl flex-col gap-1 rounded-lg hairline border-wd-hairline bg-wd-surface p-4" data-testid="outcome-authority-blocked">
					<h3 className="flex items-center gap-1.5 text-sm font-medium text-wd-primary">
						<StateMark className="text-critical" tone="critical" />
						{t("outcome.decide.blockedTitle")}
					</h3>
					<p className="text-sm text-wd-secondary">{failure?.message}</p>
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
			<div className="flex flex-wrap items-center justify-between gap-3 rounded-lg hairline border-wd-hairline bg-wd-surface px-4 py-3" data-testid="outcome-plan-summary">
				<div className="flex min-w-0 items-center gap-3">
					<h3 className="min-w-0 truncate text-sm font-medium text-wd-primary">{plan.summary || unit?.title}</h3>
					<StateBadge
						label={plan.status === "approved"
							? t("outcome.decide.badgeApproved", { number: plan.number })
							: t("outcome.decide.badgeProposed", { number: plan.number })}
						tone={plan.status === "approved" ? "good" : "neutral"}
					/>
				</div>
				<div aria-label={t("mission.plan")} className="flex shrink-0 gap-1" role="group">
					{(["plan", "graph"] as const).map((mode) => (
						<Button
							aria-pressed={view === mode}
							className={cn(view === mode && "bg-wd-selected hover:bg-wd-selected")}
							key={mode}
							onClick={() => setView(mode)}
							size="sm"
							variant="ghost"
						>
							{t(`mission.${mode}`)}
						</Button>
					))}
				</div>
			</div>
			{view === "graph" ? (
				/* Proposed topology has no schedule before authorization. The graph
				   remains a distinct view and never inherits the text-plan sections. */
				<div className="rounded-lg hairline border-wd-hairline bg-wd-surface p-4" data-testid="outcome-plan-graph">
					{plan.status !== "approved" || scheduleQuery.schedule ? (
						<MissionPlanView
							criterionText={criterionText}
							graphOnly
							schedule={scheduleQuery.schedule}
							workUnits={plan.workUnits}
						/>
					) : <p className="text-xs text-wd-secondary">{scheduleQuery.failure?.message || t("mission.scheduleUnavailable")}</p>}
				</div>
			) : <div className="contents" data-testid="outcome-plan-details">
			<Accordion className="flex flex-col gap-2" defaultValue={PLAN_SECTION_VALUES} type="multiple">
				<PlanSection icon={<Sparkles aria-hidden="true" className="size-3.5" />} label={t("outcome.decide.factsSummary")} value="summary">
					<p className="text-sm leading-body text-wd-primary">{plan.summary || unit?.title || t("outcome.decide.notReported")}</p>
				</PlanSection>
				<PlanSection icon={<Crosshair aria-hidden="true" className="size-3.5" />} label={t("outcome.decide.factsDesiredState")} value="desired-state">
					{boundRevision ? (
						<>
							<p className="whitespace-pre-wrap text-sm leading-body text-wd-primary">{boundRevision.goal || t("outcome.decide.notReported")}</p>
							{(boundRevision.successCriteria ?? []).length > 0 && (
								<ul className="mt-2 flex list-disc flex-col gap-1 pl-4 text-xs leading-body text-wd-secondary">
									{boundRevision.successCriteria.map((criterion, index) => (
										<li key={index}>{criterion}</li>
									))}
								</ul>
							)}
						</>
					) : (
						<p className="text-xs text-wd-secondary">{t("outcome.decide.notReported")}</p>
					)}
				</PlanSection>
				<PlanSection icon={<ListChecks aria-hidden="true" className="size-3.5" />} label={t("outcome.decide.factsEvidence")} value="evidence">
					{boundRevision && (boundRevision.criteria ?? []).length > 0 ? (
						<ul className="flex flex-col gap-2">
							{(boundRevision.criteria ?? []).map((criterion, index) => (
								<li className="text-sm" key={criterion.criterionId ?? index}>
									<p className="text-wd-primary">{criterion.text}</p>
									{(evidenceByCriterion.get(criterion.criterionId) ?? []).length > 0 && (
										<ul className="mt-1 flex list-disc flex-col gap-1 pl-4 text-xs leading-body text-wd-secondary">
											{(evidenceByCriterion.get(criterion.criterionId) ?? []).map((line, lineIndex) => (
												<li key={lineIndex}>{line}</li>
											))}
										</ul>
									)}
								</li>
							))}
						</ul>
					) : (
						<p className="text-xs text-wd-secondary">{t("outcome.decide.notReported")}</p>
					)}
				</PlanSection>
				<PlanSection icon={<BadgeCheck aria-hidden="true" className="size-3.5" />} label={t("outcome.decide.factsVerification")} value="verification">
					{boundRevision?.review ? (
						<p className="whitespace-pre-wrap text-sm leading-body text-wd-primary">{boundRevision.review}</p>
					) : (
						<p className="text-xs text-wd-secondary">{t("outcome.decide.notReported")}</p>
					)}
					{plan.workUnits.some((workUnit) => workUnit.verificationRequirement?.trim()) && (
						<ul className="mt-2 flex list-disc flex-col gap-1 pl-4 text-xs leading-body text-wd-secondary">
							{plan.workUnits.filter((workUnit) => workUnit.verificationRequirement?.trim()).map((workUnit) => (
								<li key={workUnit.id}>{workUnit.title}: {workUnit.verificationRequirement}</li>
							))}
						</ul>
					)}
				</PlanSection>
				<PlanSection icon={<PauseCircle aria-hidden="true" className="size-3.5" />} label={t("outcome.decide.factsStops")} value="pause-trigger">
					{pauseTriggers.length > 0 ? (
						<ul className="flex list-disc flex-col gap-1 pl-4 text-sm leading-body text-wd-primary">
							{pauseTriggers.map((line, index) => (
								<li key={index}>{line}</li>
							))}
						</ul>
					) : (
						<p className="text-xs text-wd-secondary">{t("outcome.decide.notReported")}</p>
					)}
				</PlanSection>
				{/* Read-only on purpose: no verified grants mutation API exists, so
				    the card reports the plan's grants and says "Not reported" when
				    the plan carried none. */}
				<PlanSection icon={<ShieldCheck aria-hidden="true" className="size-3.5" />} label={t("outcome.decide.factsGrants")} value="permissions">
					{grants.length > 0 ? (
						<ul className="flex flex-col gap-1.5">
							{grants.map((grant) => (
								<GrantRow grant={grant} key={grant.id} />
							))}
						</ul>
					) : (
						<p className="text-xs text-wd-secondary" data-testid="outcome-plan-grants-empty">{t("outcome.decide.notReported")}</p>
					)}
				</PlanSection>
				<PlanSection icon={<FileText aria-hidden="true" className="size-3.5" />} label={t("outcome.decide.factsBrief")} value="brief">
					<p className="break-all text-xs leading-body tabular-nums text-wd-secondary">{plan.runBriefCoreDigest}</p>
					{assumptions.length > 0 && (
						<p className="mt-2 flex items-start gap-1.5 text-xs text-wd-primary">
							<StateMark className="mt-0.5 text-caution" tone="caution" />
							<span><span className="text-wd-secondary">{t("outcome.decide.assumptionsLabel")}: </span>{assumptions.join(" · ")}</span>
						</p>
					)}
					{blockers.length > 0 && (
						<p className="mt-2 flex items-start gap-1.5 text-xs text-wd-primary">
							<StateMark className="mt-0.5 text-critical" tone="critical" />
							<span><span className="text-wd-secondary">{t("outcome.decide.blockersLabel")}: </span>{blockers.join(" · ")}</span>
						</p>
					)}
					{routingDecisions.length > 0 && <p className="mt-2 text-xs text-wd-secondary">{t("outcome.decide.routingLabel")}: {routingDecisions.map((decision) => `${decision.workUnitId}: ${decision.recommendedProvider || "no candidate"}`).join(" · ")}</p>}
				</PlanSection>
			</Accordion>

			<div className="grid gap-2" data-testid="outcome-plan-work-units">
				{plan.workUnits.map((workUnit, index) => (
					<div className="rounded-md hairline border-wd-hairline bg-wd-surface px-3.5 py-3" key={workUnit.id}>
						<div className="flex items-center justify-between gap-3">
							<span className="text-sm font-medium text-wd-primary"><span className="tabular-nums">{index + 1}.</span> {workUnit.title}</span>
							<span className="text-xs text-wd-secondary">{workUnit.modelSelection === "provider_default" ? t("outcome.missionGraph.providerDefault") : workUnit.model || t("outcome.decide.modelUnreported")}</span>
						</div>
						<div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-wd-secondary">
							{/* Criteria and dependencies read as words, not ids: a raw
							    identifier is technical detail, not primary content. */}
							<span>{t("outcome.decide.criteriaLabel")}: {criteriaLabel(workUnit.criterionIds, criterionText) || t("outcome.decide.criteriaNotRecorded")}</span>
							<span>{t("outcome.decide.dependsOnLabel")}: {(workUnit.dependsOn ?? []).length > 0 ? (workUnit.dependsOn ?? []).map((id) => titleOf.get(id) ?? id).join(", ") : t("outcome.decide.dependsOnNone")}</span>
							<span>{t("outcome.decide.providerLabel")}: {workUnit.provider || t("outcome.decide.providerUnbound")}</span>
						</div>
					</div>
				))}
			</div>

			<p className="px-1 text-xs tabular-nums text-wd-tertiary">
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
		<AccordionItem className="overflow-hidden rounded-lg hairline border-wd-hairline bg-wd-surface" value={value}>
			<AccordionTrigger
				className="group justify-between gap-2 px-3 py-3 text-left text-sm font-medium text-wd-primary"
				headerClassName="px-1"
			>
				<span className="flex min-w-0 items-center gap-2 text-wd-secondary">
					{icon}
					<span className="truncate text-wd-primary">{label}</span>
				</span>
				<ChevronDown
					aria-hidden="true"
					className="size-3.5 shrink-0 text-wd-tertiary transition-transform duration-fast motion-reduce:transition-none group-data-[state=open]:rotate-180"
				/>
			</AccordionTrigger>
			<AccordionContent className="px-3 pb-3">
				<div className="rounded-md bg-wd-inset p-3">{children}</div>
			</AccordionContent>
		</AccordionItem>
	);
}

type PlanGrant = NonNullable<PlanRecord["grants"]>[number];

function GrantRow({ grant }: { grant: PlanGrant }) {
	return (
		<li className="flex min-w-0 items-center gap-2.5 text-sm">
			<ShieldCheck aria-hidden="true" className="size-3.5 shrink-0 text-wd-tertiary" />
			<code className="text-xs text-wd-primary">{grant.name}</code>
			<span className="truncate text-xs text-wd-tertiary">{grant.scope}</span>
		</li>
	);
}

/**
 * The single owner decision on this screen, on Waldo's Needs You anatomy:
 * the exact action, the exact permissions it grants, the facts in words,
 * then Update (quiet, leading) and Authorize (Spot, trailing). Authorize stays
 * on the existing approve mutation; Update stays on the revise path. Neither
 * gains new authority.
 */
function AuthorizeDecisionCard({
	onApprove,
	onReviewContract,
	pending,
	plan,
}: {
	onApprove: () => void;
	onReviewContract?: () => void;
	pending: boolean;
	plan: PlanRecord;
}) {
	const { t } = useTranslation();
	const subjectId = useId();
	const grants = plan.grants ?? [];
	return (
		<section
			aria-labelledby={subjectId}
			className="mx-auto flex w-full max-w-2xl flex-col gap-3 rounded-lg hairline border-wd-hairline bg-wd-surface p-5"
			data-testid="outcome-authorize-card"
		>
			<div className="flex min-w-0 items-center justify-between gap-2">
				<StateBadge label={t("outcome.attention.needsYou")} tone="needsYou" />
				<span className="truncate text-xs tabular-nums text-wd-tertiary">
					{t("mission.planBinding", { plan: plan.number, contract: plan.contractRevisionNumber })}
				</span>
			</div>
			<h3 className="text-lg font-medium leading-6 text-wd-primary" id={subjectId}>
				{t("outcome.decide.authorizeSubject", { number: plan.number })}
			</h3>
			<div className="flex flex-col gap-1.5 rounded-md bg-wd-inset p-3">
				<p className="text-xs text-wd-secondary">{t("outcome.decide.factsGrants")}</p>
				{grants.length > 0 ? (
					<ul className="flex flex-col gap-1.5">
						{grants.map((grant) => (
							<GrantRow grant={grant} key={grant.id} />
						))}
					</ul>
				) : (
					<p className="text-sm text-wd-secondary">{t("outcome.decide.notReported")}</p>
				)}
			</div>
			<FactList
				facts={[
					{ label: t("outcome.needsYou.whyNow"), value: t("outcome.decide.whyNow") },
					{ label: t("outcome.needsYou.ifApprove"), value: t("outcome.decide.approveNote") },
					{ label: t("outcome.needsYou.ifNot"), value: t("outcome.decide.ifNot") },
					{ label: t("outcome.needsYou.basedOn"), value: t("outcome.decide.basedOn", { contractRevision: plan.contractRevisionNumber }) },
				]}
			/>
			<div className="flex items-center justify-between gap-3 pt-1">
				{onReviewContract ? (
					<Button data-testid="outcome-plan-update" onClick={onReviewContract} size="sm" type="button" variant="ghost">
						{t("outcome.decide.updateCta")}
					</Button>
				) : <span />}
				<Button
					aria-busy={pending || undefined}
					data-testid="outcome-approve-plan"
					disabled={pending}
					onClick={onApprove}
					size="sm"
					variant="spot"
				>
					<ShieldCheck aria-hidden="true" className="size-3.5" />
					{pending ? t("outcome.decide.authorizing") : t("outcome.decide.authorizeCta")}
				</Button>
			</div>
		</section>
	);
}

function PlanFailureBanners({ failure, onRetry }: { failure: OutcomeFailure; onRetry: () => void }) {
	const { t } = useTranslation();
	if (failure.kind === "offline") {
		return (
			<div className="flex max-w-xl flex-col gap-1 rounded-lg hairline border-wd-hairline bg-wd-surface p-4" data-testid="outcome-plan-offline" role="alert">
				<h3 className="text-sm font-medium text-wd-primary">{t("outcome.understand.offlineTitle")}</h3>
				<p className="text-sm text-wd-secondary">{t("outcome.understand.offlineBody")}</p>
				<Button className="mt-2 self-start" onClick={onRetry} size="sm" type="button" variant="outline">
					{t("outcome.understand.retry")}
				</Button>
			</div>
		);
	}
	if (failure.kind === "retryable") {
		return (
			<div className="flex max-w-xl flex-col gap-1 rounded-lg hairline border-wd-hairline bg-wd-surface p-4" data-testid="outcome-plan-retryable" role="alert">
				<h3 className="text-sm font-medium text-wd-primary">{t("outcome.plan.failedTitle")}</h3>
				<p className="text-sm text-wd-secondary">{failure.code === "PLAN_DRAFT_CHECK_INVALID" ? t("outcome.plan.invalidCheck") : failure.message}</p>
				{failure.code === "PLAN_DRAFT_CHECK_INVALID" && <p className="mt-1 break-words text-xs text-wd-tertiary">{failure.message}</p>}
				<Button className="mt-2 self-start" onClick={onRetry} size="sm" type="button" variant="outline">
					{t("outcome.understand.retry")}
				</Button>
			</div>
		);
	}
	return (
		<p className="text-sm text-critical" role="alert">
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
