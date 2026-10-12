import { StateLabel, StateMark, type StateTone } from "@pin4sf/kennel-product-ui";
import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import type { MessageKey } from "../../i18n/messages";
import { useOutcome } from "../../hooks/useOutcome";
import {
	DECOMPOSITION_STATUS,
	OUTCOME_SHAPES,
	useAcceptContributorBatch,
	useAuthorizeDecomposition,
	useBatchEligibility,
	useOutcomeComposition,
	useOutcomeDecomposition,
	useProposeDecomposition,
	useWaiveContributionDependency,
	useAskForDecomposition,
	useDecompositionRequest,
	parseRawProposal,
	DECOMPOSITION_REQUEST_STATUS,
	type ContributorRecord,
} from "../../hooks/useOutcomeComposition";
import type { components } from "../../../api/schema";
import { DecompositionEditor, NO_AUTHORITY } from "./DecompositionEditor";
import { DecompositionGraph } from "./DecompositionGraph";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { cn } from "../../lib/utils";

type OutcomeMissionControlProps = {
	outcomeId: string;
	/** Opens the provider session behind one contributor's attempt. */
	onInspectContributor?: (contributorId: string) => void;
};

/**
 * Outcome Mission Control: one Project-level Outcome and the contributing
 * Outcomes it is pursued through.
 *
 * The topology is rendered as an ordered LIST rather than a graph. That is a
 * deliberate, temporary choice: the attempt fence is currently project-wide, so
 * contributors cannot actually run in parallel (see the Phase 3 note in the
 * Composed Outcomes plan). A graph here would draw concurrency the daemon will
 * refuse, and drawing a promise the system cannot keep is worse than drawing
 * less. The list carries the same dependency information and stays keyboard
 * accessible.
 */
export function OutcomeMissionControl({ outcomeId, onInspectContributor }: OutcomeMissionControlProps) {
	const { t } = useTranslation();
	const { outcome } = useOutcome(outcomeId);
	const { composition, isLoading, failure, refetch } = useOutcomeComposition(outcomeId);
	const { decomposition } = useOutcomeDecomposition(outcomeId);

	if (isLoading) return <p className="text-sm text-wd-secondary">{t("outcome.mission.loading")}</p>;
	if (failure) {
		return (
			<div className="flex flex-col items-start gap-2">
				<p className="text-sm text-wd-secondary">{failure.message}</p>
				<Button onClick={refetch} size="sm" variant="secondary">
					{t("outcome.mission.retry")}
				</Button>
			</div>
		);
	}
	if (!composition) return null;

	const isDecomposed = composition.shape === OUTCOME_SHAPES.decomposed;

	return (
		<section aria-label={t("outcome.mission.label")} className="flex h-full flex-col gap-6 overflow-y-auto">
			<header className="flex flex-col gap-2">
				{/* The Outcome's name is the headline, set in Mottle: words only -
				    counts and revisions stay in the facts line below. */}
				<h2 className="wd-display-3 text-balance text-wd-primary">{outcome?.title ?? t("outcome.mission.heading")}</h2>
				{outcome?.currentRevision.goal ? (
					<p className="max-w-prose text-sm leading-body text-wd-secondary">{outcome.currentRevision.goal}</p>
				) : null}
				<span className="flex flex-wrap items-center gap-2">
					<Badge variant="neutral">
						{isDecomposed ? t("outcome.mission.shapeDecomposed") : t("outcome.mission.shapeDirect")}
					</Badge>
					{isDecomposed ? (
						<Badge className="tabular-nums" variant="neutral">
							{t("outcome.mission.acceptedOf", {
								accepted: composition.attention.acceptedOf,
								total: composition.attention.contributors,
							})}
						</Badge>
					) : null}
				</span>
			</header>

			{isDecomposed ? (
				<>
					<CoveragePanel composition={composition} />
					<ContributorList
						composition={composition}
						decomposition={decomposition}
						decompositionStatus={decomposition?.status}
						onInspectContributor={onInspectContributor}
						outcomeId={outcomeId}
					/>
					<AcceptanceBatchPanel outcomeId={outcomeId} revision={outcome?.currentRevisionNumber ?? 0} />
				</>
			) : (
				<DecomposePanel outcomeId={outcomeId} revision={outcome?.currentRevisionNumber ?? 0} />
			)}
		</section>
	);
}

/** Every criterion is someone's job, or the surface says so plainly. */
function CoveragePanel({ composition }: { composition: NonNullable<ReturnType<typeof useOutcomeComposition>["composition"]> }) {
	const { t } = useTranslation();
	if (composition.coverage.length === 0) return null;
	return (
		<section className="rounded-lg hairline border-wd-hairline bg-wd-surface p-4" data-testid="mission-coverage">
			<h3 className="text-sm font-medium text-wd-primary">{t("outcome.mission.coverageHeading")}</h3>
			<ul className="mt-2 flex flex-col">
				{composition.coverage.map((claim) => (
					<li className="flex flex-col gap-0.5 hairline-b border-wd-hairline py-2 last:border-b-0" key={claim.criterionId}>
						<span className="text-sm leading-body text-wd-primary">{claim.text}</span>
						{claim.claimedBy.length > 0 ? (
							<span className="text-xs text-wd-secondary">{t("outcome.mission.claimedBy", { n: claim.claimedBy.length })}</span>
						) : (
							<StateLabel className="text-xs" label={t("outcome.mission.unclaimed")} tone="caution" />
						)}
					</li>
				))}
			</ul>
			{composition.unclaimedCriteria.length > 0 ? (
				<Notice className="mt-2" tone="caution">
					{t("outcome.mission.unclaimedWarning", { n: composition.unclaimedCriteria.length })}
				</Notice>
			) : null}
		</section>
	);
}

function ContributorList({
	composition,
	outcomeId,
	decomposition,
	decompositionStatus,
	onInspectContributor,
}: {
	composition: NonNullable<ReturnType<typeof useOutcomeComposition>["composition"]>;
	outcomeId: string;
	decomposition?: components["schemas"]["DecompositionResponse"];
	decompositionStatus?: string;
	onInspectContributor?: (contributorId: string) => void;
}) {
	const { t } = useTranslation();
	return (
		<section className="flex flex-col gap-3" data-testid="mission-contributors">
			{/* The dependency order, drawn. The list below stays: it carries the
			    per-contributor detail and actions, and remains the accessible
			    path through the same information. */}
			{decomposition ? (
				<DecompositionGraph
					contributors={composition.contributors}
					decomposition={decomposition}
					onInspect={onInspectContributor}
				/>
			) : null}
			<h3 className="text-sm font-medium text-wd-primary">{t("outcome.mission.contributorsHeading")}</h3>
			{/* Execution is currently serialized by the project-wide attempt
			    fence; saying so beats letting the surface imply concurrency. */}
			<p className="-mt-2 text-xs leading-body text-wd-secondary">{t("outcome.mission.serialNote")}</p>
			<ul className="flex flex-col gap-2">
				{composition.attention.items.map((item) => {
					const contributor = composition.contributors.find((entry) => entry.outcome.id === item.outcomeId);
					return (
						<ContributorRow
							contributor={contributor}
							key={item.outcomeId}
							kind={item.kind}
							nextAction={item.nextAction}
							onInspect={onInspectContributor}
							outcomeId={outcomeId}
							reason={item.reason}
							title={item.title}
							waivable={decompositionStatus === DECOMPOSITION_STATUS.authorized}
						/>
					);
				})}
			</ul>
		</section>
	);
}

function ContributorRow({
	contributor,
	title,
	kind,
	reason,
	nextAction,
	outcomeId,
	waivable,
	onInspect,
}: {
	contributor?: ContributorRecord;
	title: string;
	kind: string;
	reason: string;
	nextAction?: string;
	outcomeId: string;
	waivable: boolean;
	onInspect?: (contributorId: string) => void;
}) {
	const { t } = useTranslation();
	const blocked = contributor?.blockedBy ?? [];
	const waived = contributor?.waived ?? [];

	return (
		<li className="flex flex-col gap-3 rounded-lg hairline border-wd-hairline bg-wd-surface p-4" data-contributor-id={contributor?.outcome.id}>
			<div className="flex items-start justify-between gap-3">
				<div className="flex min-w-0 flex-col gap-1">
					<span className="text-sm font-medium text-wd-primary">{title}</span>
					<AttentionLine kind={kind} reason={reason} />
				</div>
				{/* The daemon already says what this contribution needs next.
				    Rendering that as prose beside an "Inspect" button meant the
				    named step — "Start work on this contribution" — was the one
				    thing a person could not click. It is the button's label now,
				    so the sentence and the control are the same thing. */}
				{contributor && onInspect ? (
					<Button
						className="shrink-0"
						onClick={() => onInspect(contributor.outcome.id)}
						size="sm"
						variant={nextAction ? "secondary" : "ghost"}
					>
						{nextAction ?? t("outcome.mission.inspect")}
					</Button>
				) : nextAction ? (
					// Nowhere to send them — but what the daemon says is needed is
					// still worth saying, so the row never loses it.
					<span className="shrink-0 text-xs text-wd-secondary">{nextAction}</span>
				) : null}
			</div>

			{contributor?.stale ? (
				<Notice tone="caution">{t("outcome.mission.staleBinding")}</Notice>
			) : null}

			{blocked.length > 0 ? (
				<div className="flex flex-col gap-1">
					{blocked.map((block) => (
						<BlockedRow
							block={block}
							key={block.ref}
							outcomeId={outcomeId}
							toRef={contributorRefFor(contributor)}
							waivable={waivable}
						/>
					))}
				</div>
			) : null}

			{/* A waived dependency is overridden, not forgotten. */}
			{waived.length > 0 ? (
				<p className="text-xs text-wd-tertiary">
					{t("outcome.mission.waivedCount", { n: waived.length })}
				</p>
			) : null}
		</li>
	);
}

/** The contributor's handle inside its decomposition, needed to waive. */
function contributorRefFor(contributor?: ContributorRecord): string | undefined {
	return contributor?.outcome.id;
}

function BlockedRow({
	block,
	outcomeId,
	toRef,
	waivable,
}: {
	block: { ref: string; title?: string; reason: string };
	outcomeId: string;
	toRef?: string;
	waivable: boolean;
}) {
	const { t } = useTranslation();
	const [reason, setReason] = useState("");
	const [open, setOpen] = useState(false);
	const waiver = useWaiveContributionDependency(outcomeId);

	return (
		<div className="flex flex-col gap-2 rounded-md bg-wd-inset p-3">
			<span className="text-xs text-wd-primary">
				{t("outcome.mission.blockedBy", { title: block.title || block.ref, reason: block.reason })}
			</span>
			{waivable && toRef ? (
				open ? (
					<form
						className="flex flex-col gap-2"
						onSubmit={(event) => {
							event.preventDefault();
							void waiver.submit({ fromRef: block.ref, toRef, reason }).then(() => setOpen(false));
						}}
					>
						{/* The reason is durable: a waiver nobody can explain later is
						    indistinguishable from a mistake. */}
						<input
							aria-label={t("outcome.mission.waiveReasonLabel")}
							className={FIELD_CLASS}
							onChange={(event) => setReason(event.target.value)}
							placeholder={t("outcome.mission.waiveReasonPlaceholder")}
							value={reason}
						/>
						<div className="flex items-center justify-between gap-2">
							<Button onClick={() => setOpen(false)} size="sm" type="button" variant="ghost">
								{t("outcome.mission.waiveCancel")}
							</Button>
							<Button disabled={waiver.pending || reason.trim() === ""} size="sm" type="submit">
								{t("outcome.mission.waiveConfirm")}
							</Button>
						</div>
						{waiver.failure ? (
							<span className="text-xs text-critical" role="alert">{waiver.failure.message}</span>
						) : null}
					</form>
				) : (
					<Button className="self-start" onClick={() => setOpen(true)} size="sm" variant="ghost">
						{t("outcome.mission.waive")}
					</Button>
				)
			) : null}
		</div>
	);
}

const BATCH_SUMMARY_FALLBACK: MessageKey = "outcome.mission.batchSummaryPlaceholder";

/**
 * One sitting over every eligible contributor. The daemon writes one decision
 * per Outcome and reports whatever it withheld; this panel decides nothing and
 * never hides an exclusion.
 */
function AcceptanceBatchPanel({ outcomeId, revision }: { outcomeId: string; revision: number }) {
	const { t } = useTranslation();
	const { contributors, isLoading } = useBatchEligibility(outcomeId);
	const batch = useAcceptContributorBatch(outcomeId);
	const [summary, setSummary] = useState("");
	const [acceptParent, setAcceptParent] = useState(false);

	if (isLoading) return null;
	const eligible = contributors.filter((verdict) => verdict.eligible);
	const withheld = contributors.filter((verdict) => !verdict.eligible);
	if (contributors.length === 0) return null;

	return (
		<section className="flex flex-col gap-3 rounded-lg hairline border-wd-hairline bg-wd-surface p-4" data-testid="mission-acceptance-batch">
			<div className="flex flex-col gap-0.5">
				<h3 className="text-sm font-medium text-wd-primary">{t("outcome.mission.batchHeading")}</h3>
				<p className="text-xs leading-body text-wd-secondary">{t("outcome.mission.batchExplainer")}</p>
			</div>

			{eligible.length === 0 ? (
				<p className="text-sm text-wd-secondary">{t("outcome.mission.batchNoneEligible")}</p>
			) : (
				<form
					className="flex flex-col gap-3"
					onSubmit={(event) => {
						event.preventDefault();
						void batch.submit({
							expectedContractRevision: revision,
							summary,
							acceptParent,
							resourceDisposition: "retain",
							requestKey: `batch-${outcomeId}-${revision}-${summary.length}-${eligible.length}`,
						});
					}}
				>
					<ul className="flex flex-col gap-1 rounded-md bg-wd-inset p-3">
						{eligible.map((verdict) => (
							<li className="text-sm text-wd-primary" key={verdict.outcomeId}>
								{verdict.title}
							</li>
						))}
					</ul>
					<input
						aria-label={t("outcome.mission.batchSummaryLabel")}
						className={FIELD_CLASS}
						onChange={(event) => setSummary(event.target.value)}
						placeholder={t(BATCH_SUMMARY_FALLBACK)}
						value={summary}
					/>
					<label className="flex items-center gap-2 text-sm text-wd-primary">
						<input
							checked={acceptParent}
							onChange={(event) => setAcceptParent(event.target.checked)}
							type="checkbox"
						/>
						{t("outcome.mission.batchAcceptParent")}
					</label>
					<Button className="self-start" disabled={batch.pending || summary.trim() === ""} size="sm" type="submit">
						{t("outcome.mission.batchAccept", { n: eligible.length })}
					</Button>
					{batch.failure ? <span className="text-xs text-critical" role="alert">{batch.failure.message}</span> : null}
				</form>
			)}

			{withheld.length > 0 ? (
				<div className="flex flex-col gap-1" data-testid="mission-batch-withheld">
					<h4 className="text-xs font-medium text-wd-secondary">{t("outcome.mission.batchWithheld")}</h4>
					<ul className="flex flex-col">
						{withheld.map((verdict) => (
							<li className="flex flex-col gap-0.5 hairline-b border-wd-hairline py-2 last:border-b-0" key={verdict.outcomeId}>
								<span className="text-sm text-wd-primary">{verdict.title}</span>
								<span className="text-xs leading-body text-wd-secondary">{verdict.reason}</span>
								{verdict.remedy ? (
									<span className="text-xs leading-body text-wd-secondary">{verdict.remedy}</span>
								) : null}
							</li>
						))}
					</ul>
				</div>
			) : null}
		</section>
	);
}

/**
 * A direct Outcome can be decomposed. Nothing here creates a responsibility
 * until the owner authorizes: a proposal, however it was authored, is an offer.
 */
function DecomposePanel({ outcomeId, revision }: { outcomeId: string; revision: number }) {
	const { t } = useTranslation();
	const { outcome } = useOutcome(outcomeId);
	const { decomposition, refetch } = useOutcomeDecomposition(outcomeId);
	const { request, pending: agentWorking, refetch: refetchRequest } = useDecompositionRequest(outcomeId);
	const propose = useProposeDecomposition(outcomeId);
	const authorize = useAuthorizeDecomposition(outcomeId);
	const ask = useAskForDecomposition(outcomeId);
	const [editing, setEditing] = useState(false);
	const [draft, setDraft] = useState<ReturnType<typeof parseRawProposal>>(undefined);

	const proposal = decomposition?.status === DECOMPOSITION_STATUS.proposed ? decomposition : undefined;
	const criteria = outcome?.currentRevision.criteria ?? [];
	const refused = request?.status === DECOMPOSITION_REQUEST_STATUS.rejected ? request : undefined;

	if (editing && outcome) {
		return (
			<section className="flex flex-col gap-3" data-testid="mission-decompose">
				<DecompositionEditor
					criteria={criteria}
					draft={draft}
					existing={proposal}
					expectedContractRevision={revision}
					failureMessage={propose.failure?.message}
					onCancel={() => {
						propose.reset();
						setDraft(undefined);
						setEditing(false);
					}}
					onPropose={(request) => {
						void propose.submit(request).then(() => {
							setDraft(undefined);
							setEditing(false);
							refetch();
						});
					}}
					parentAuthority={outcome.currentRevision.authorityCeiling ?? NO_AUTHORITY}
					pending={propose.pending}
				/>
			</section>
		);
	}

	return (
		<section className="flex flex-col gap-3 rounded-lg hairline border-wd-hairline bg-wd-surface p-4" data-testid="mission-decompose">
			<div className="flex flex-col gap-0.5">
				<h3 className="text-sm font-medium text-wd-primary">{t("outcome.mission.decomposeHeading")}</h3>
				<p className="text-sm text-wd-secondary">{t("outcome.mission.decomposeExplainer")}</p>
			</div>

			{/* An agent is working. There is nothing to await here — the answer
			    arrives on the daemon's callback route — so this reports rather
			    than pretends to block. */}
			{agentWorking ? (
				<p className="flex items-start gap-2 rounded-md bg-wd-inset p-3 text-sm text-wd-primary" data-testid="decompose-agent-working">
					<StateMark className="mt-1.5" tone="live" />
					{t("outcome.mission.agentWorking")}
				</p>
			) : null}

			{request?.status === DECOMPOSITION_REQUEST_STATUS.expired ? (
				<p className="text-sm text-wd-secondary" data-testid="decompose-agent-expired">
					{t("outcome.mission.agentExpired")}
				</p>
			) : null}

			{/* A refused draft is kept so one field can be fixed instead of
			    regenerating. Reopening it is the point of retaining it. */}
			{refused ? (
				<div className="flex flex-col gap-2" data-testid="decompose-agent-refused">
					<Notice tone="critical">
						{t("outcome.mission.agentRefused", { reason: refused.refusalReason ?? "" })}
					</Notice>
					{parseRawProposal(refused.rawProposal) ? (
						<Button
							className="self-start"
							onClick={() => {
								setDraft(parseRawProposal(refused.rawProposal));
								setEditing(true);
							}}
							size="sm"
							type="button"
							variant="secondary"
						>
							{t("outcome.mission.openRefusedDraft")}
						</Button>
					) : null}
				</div>
			) : null}

			{proposal ? (
				<div className="flex flex-col gap-2">
					{proposal.stale ? (
						<Notice tone="caution">{t("outcome.mission.proposalStale")}</Notice>
					) : null}
					<p className="text-sm leading-body text-wd-primary">{proposal.rationale}</p>
					{/* The shape, before it is authorized. This is where the order
					    matters most: a flat list of titles does not show that one
					    contribution has to finish before another can start, which
					    is exactly what the owner is being asked to approve. */}
					<DecompositionGraph contributors={[]} decomposition={proposal} />
					<div className="flex items-center justify-between gap-2">
						<Button onClick={() => setEditing(true)} size="sm" type="button" variant="ghost">
							{t("outcome.mission.edit")}
						</Button>
						<Button
							data-testid="decompose-authorize"
							disabled={authorize.pending || proposal.stale}
							onClick={() => void authorize.submit(proposal.id)}
							size="sm"
						>
							{t("outcome.mission.authorize", { n: proposal.contributors.length })}
						</Button>
					</div>
					{authorize.failure ? (
						<span className="text-xs text-critical" role="alert">{authorize.failure.message}</span>
					) : null}
				</div>
			) : (
				<div className="flex flex-wrap items-center gap-2">
					<Button
						aria-busy={ask.pending || undefined}
						data-testid="decompose-ask-agent"
						disabled={ask.pending || agentWorking || criteria.length === 0}
						onClick={() => void ask.submit({ expectedContractRevision: revision }).then(refetchRequest)}
						size="sm"
					>
						{t("outcome.mission.askAgent")}
					</Button>
					<Button
						disabled={propose.pending || criteria.length === 0}
						onClick={() => void propose.submit({ expectedContractRevision: revision }).then(refetch)}
						size="sm"
						variant="secondary"
					>
						{t("outcome.mission.propose")}
					</Button>
					<Button disabled={criteria.length === 0} onClick={() => setEditing(true)} size="sm" type="button" variant="ghost">
						{t("outcome.mission.authorMine")}
					</Button>
				</div>
			)}
			{ask.failure ? <span className="text-xs text-critical" role="alert">{ask.failure.message}</span> : null}
			{propose.failure ? <span className="text-xs text-critical" role="alert">{propose.failure.message}</span> : null}
		</section>
	);
}

/** Shared attention presentation, keyed off the daemon's vocabulary. */
export const ATTENTION_LABEL_KEYS: Record<string, MessageKey> = {
	needs_you: "outcome.attention.needsYou",
	action_required: "outcome.attention.actionRequired",
	ready_for_acceptance: "outcome.attention.readyForAcceptance",
	waiting: "outcome.attention.waiting",
	running: "outcome.attention.running",
	accepted: "outcome.attention.accepted",
};

/**
 * Attention as Waldo's typed state: the owner's next move takes the Spot dot,
 * running takes the live accent, ready and accepted are good, waiting is a
 * neutral word. The word always travels with the mark.
 */
export const ATTENTION_TONE: Record<string, StateTone> = {
	needs_you: "needsYou",
	action_required: "needsYou",
	ready_for_acceptance: "good",
	waiting: "neutral",
	running: "live",
	accepted: "good",
};

export function AttentionLine({ kind, reason }: { kind: string; reason: string }) {
	const { t } = useTranslation();
	const key = ATTENTION_LABEL_KEYS[kind];
	return (
		<span className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 text-xs" data-attention-kind={kind}>
			<StateLabel className="font-medium" label={key ? t(key) : kind} tone={ATTENTION_TONE[kind] ?? "neutral"} />
			<span className="leading-body text-wd-secondary">{reason}</span>
		</span>
	);
}

/** TextField contract: inset fill, radius-md, no heavy border, the system
 *  focus ring. */
const FIELD_CLASS =
	"rounded-md border border-transparent bg-wd-inset px-3 py-2 text-sm text-wd-primary outline-none placeholder:text-wd-tertiary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)";

/** A full sentence with its state mark leading, so colour is never the only
 *  signal and long text never truncates the way an inline label would. */
function Notice({ tone, className, children }: { tone: StateTone; className?: string; children: ReactNode }) {
	const markColour = tone === "caution" ? "text-caution" : tone === "critical" ? "text-critical" : tone === "good" ? "text-good" : undefined;
	return (
		<p className={cn("flex items-start gap-1.5 text-xs leading-body text-wd-primary", className)}>
			<StateMark className={cn("mt-0.5", markColour)} tone={tone} />
			<span className="min-w-0">{children}</span>
		</p>
	);
}
