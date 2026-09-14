import {
	SessionCardView,
	SessionRowView,
	type BoardSessionPresentation,
	type BoardSplitLaneLabels,
	type ProductUITranslator,
} from "@pin4sf/kennel-product-ui";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

import type { AttemptRecord, PlanRecord } from "../../hooks/useOutcome";
import { useSessionScmSummary } from "../../hooks/useSessionScmSummary";
import { useWorkspaceQuery } from "../../hooks/useWorkspaceQuery";
import type { MessageKey } from "../../i18n/messages";
import { formatTimeCompact } from "../../lib/format-time";
import { prBrowserUrl, sessionPRDisplaySummaries } from "../../lib/pr-display";
import type { AttentionZone } from "../../lib/session-presentation";
import type { SessionStatus } from "../../types/workspace";
import { AgentAvatar } from "../AgentAvatar";
import { ProductExternalLink } from "../ProductExternalLink";

/**
 * Adapts one Outcome's attempt lineage onto the same Board/List building blocks
 * `SessionsBoard`/`SessionsBoardAdapters` already use for the Sessions surface
 * (DESIGN.md "Board screen 2948:15618" reference implementation) — same lane
 * hues, hairline card/row plates, and segmented view switch, reused rather than
 * re-derived, per DESIGN.md.
 */
export type AttemptBoardPresentation = BoardSessionPresentation & {
	attempt: AttemptRecord;
	isCurrent: boolean;
};

/** Keep board, WorkShell, and Mission Control on the same durable lineage
 * winner even when a compatibility payload is not already sorted. */
export function newestAttempt(attempts: AttemptRecord[]): AttemptRecord | undefined {
	return attempts.reduce<AttemptRecord | undefined>((newest, attempt) => {
		if (!newest) return attempt;
		if (attempt.number !== newest.number) return attempt.number > newest.number ? attempt : newest;
		if (attempt.updatedAt !== newest.updatedAt) return attempt.updatedAt > newest.updatedAt ? attempt : newest;
		return attempt.id > newest.id ? attempt : newest;
	}, undefined);
}

/** Resolve the latest AgentSessionRef without relying on JSON/list order. */
export function newestAttemptSession(attempt: AttemptRecord): AttemptRecord["sessions"][number] | undefined {
	return attempt.sessions.reduce<AttemptRecord["sessions"][number] | undefined>((newest, session) => {
		if (!newest) return session;
		if (session.seq !== newest.seq) return session.seq > newest.seq ? session : newest;
		if (session.boundAt !== newest.boundAt) return session.boundAt > newest.boundAt ? session : newest;
		return session.id > newest.id ? session : newest;
	}, undefined);
}

type AttemptPhase = AttemptRecord["presentation"]["phase"];

/**
 * Where one attempt's daemon-derived phase lands on the board's four lanes.
 * A phase that needs the owner to pick among recovery verbs (contain /
 * reconcile / replace / the two-step owner-stop) lands in "Needs Choice"; a
 * live question from the agent lands in "Needs Input"; anything still moving
 * is "Running"; a finished attempt is "Ready" for Prove & Close. This is a
 * presentation-only grouping of a real daemon-derived field (`presentation.phase`),
 * mirroring how `attentionZone()` already buckets session status client-side.
 */
const ATTEMPT_ZONE: Record<AttemptPhase, AttentionZone> = {
	succeeded: "merge",
	needs_input: "pending",
	awaiting_start: "working",
	executing: "working",
	suspended: "working",
	unconfirmed: "action",
	ended_unclassified: "action",
	halted_failed: "action",
	halted_cancelled: "action",
	suspect_lost: "action",
};

/**
 * `SessionsBoardGridView`/`SessionsListView` bucket cards purely from
 * `session.status` via `attentionZone()`. These are the SessionStatus values
 * whose own zone equals the attempt zone above — a driver value, not a claim
 * that an attempt IS that session status. The visible state text always comes
 * from `statusPresentation.label` (the attempt's real `nextAction`), never from
 * this status's default label.
 */
const ZONE_DRIVER_STATUS: Record<AttentionZone, SessionStatus> = {
	action: "needs_input",
	pending: "review_pending",
	merge: "mergeable",
	working: "working",
	done: "terminated",
};

const ZONE_TEXT_CLASSNAME: Record<AttentionZone, string> = {
	action: "text-status-needs-you",
	pending: "text-status-in-review",
	merge: "text-status-ready",
	working: "text-status-working",
	done: "text-status-terminated-foreground",
};

export function attemptZone(phase: AttemptPhase): AttentionZone {
	return ATTEMPT_ZONE[phase] ?? "action";
}

export function toAttemptBoardPresentation(
	attempt: AttemptRecord,
	plan: PlanRecord | undefined,
	isCurrent: boolean,
	t: TFunction,
): AttemptBoardPresentation {
	const zone = attemptZone(attempt.presentation.phase);
	const title =
		plan?.workUnits.find((unit) => unit.id === attempt.workUnitId)?.title ??
		t("outcome.run.attemptFallbackTitle", { number: attempt.number });
	return {
		attempt,
		id: attempt.id,
		isCurrent,
		provider: newestAttemptSession(attempt)?.harness ?? "",
		status: ZONE_DRIVER_STATUS[zone],
		statusPresentation: {
			className: ZONE_TEXT_CLASSNAME[zone],
			// A dot only where it carries motion: an executing attempt is the one
			// live thing on this board (DESIGN.md — the state line carries a dot
			// only when it is moving).
			indicatorClassName: attempt.presentation.phase === "executing" ? "bg-status-working animate-status-pulse" : "",
			label: attempt.presentation.nextAction,
		},
		title,
		updatedAt: attempt.updatedAt,
	};
}

/** Column chrome (aria labels only — never rendered as visible copy) reuses the
 *  same wording the Sessions board already ships and every locale already
 *  translates; attempts share the identical lane/board grammar. */
export function outcomeRunBoardLabels(t: TFunction): BoardSplitLaneLabels {
	return {
		columnAria: (label) => t("shell.sessionsAria", { label }),
		countSessions: (count, label) => t("shell.countSessionsAria", { count, label }),
		idleWorkingAria: t("shell.idleWorkingSessions"),
		laneSummary: (primary, secondary) => t("shell.laneSummaryAria", { primary, secondary }),
		readyMergedAria: t("shell.readyMergedSessions"),
		tones: {
			idle: { countLabel: t("shell.countLabel.idle"), label: t("status.idle"), regionLabel: t("shell.idleSessions") },
			merged: { countLabel: t("shell.countLabel.merged"), label: t("status.merged"), regionLabel: t("shell.mergedSessions") },
			ready: { countLabel: t("shell.countLabel.readyToMerge"), label: t("zone.merge"), regionLabel: t("shell.readyToMergeSessions") },
			working: { countLabel: t("shell.countLabel.working"), label: t("status.working"), regionLabel: t("shell.workingSessions") },
		},
	};
}

/** No PR concept applies to an attempt card — `prs` stays empty, so this
 *  content is never actually rendered; it only satisfies the shared prop shape. */
const EMPTY_PR_LABELS = { short: "", states: { closed: "", draft: "", merged: "", open: "" } };

function attemptCardLabels(t: TFunction) {
	return {
		formatTime: formatTimeCompact,
		intakeIssue: (id: string) => id,
		pr: EMPTY_PR_LABELS,
		updatedAt: (time: string) => t("shell.updatedAt", { time }),
	};
}

function EngageAttemptButton({ onEngage }: { onEngage: () => void }) {
	const { t } = useTranslation();
	return (
		<button
			aria-label={t("outcome.run.engageCta")}
			className="relative z-10 inline-flex h-[30px] min-w-[72px] items-center justify-center rounded-md border border-border-strong bg-popover px-2.5 text-brand font-medium text-foreground transition-colors hover:bg-white/10"
			onClick={(event) => {
				event.stopPropagation();
				onEngage();
			}}
			type="button"
		>
			{t("outcome.run.engageCta")}
		</button>
	);
}

function MergeAttemptLink({ href }: { href: string }) {
	const { t } = useTranslation();
	return (
		<ProductExternalLink
			className="relative z-10 inline-flex h-[30px] items-center justify-center rounded-md border border-border-strong bg-popover px-2.5 text-brand font-medium text-foreground transition-colors hover:bg-white/10"
			href={href}
			stopPropagation
		>
			{t("pr.merge.action")}
		</ProductExternalLink>
	);
}

/**
 * Resolves the current attempt's most recently bound session to a real pull
 * request, so the Ready lane (a succeeded attempt) can offer a genuine Merge
 * action next to Engage — reusing the exact hooks and URL helpers the generic
 * Sessions board already uses for the identical job (`useSessionScmSummary`,
 * `sessionPRDisplaySummaries`, `prBrowserUrl`), not a new API call. An
 * Attempt carries no PR field of its own; only its bound WorkspaceSession does.
 */
function useAttemptMergeHref(attempt: AttemptRecord): string | undefined {
	const latestBinding = newestAttemptSession(attempt);
	const sessionId = latestBinding?.sessionId;
	const workspaceQuery = useWorkspaceQuery();
	const scmQuery = useSessionScmSummary(sessionId);
	const session = (workspaceQuery.data ?? [])
		.flatMap((workspace) => workspace.sessions)
		.find((candidate) => candidate.id === sessionId);
	if (!session) return undefined;
	const [primaryPR] = sessionPRDisplaySummaries(session, scmQuery.data);
	return primaryPR ? prBrowserUrl(primaryPR) : undefined;
}

function AttemptCardActions({ onEngage, presentation }: { onEngage: () => void; presentation: AttemptBoardPresentation }) {
	const mergeHref = useAttemptMergeHref(presentation.attempt);
	const showMerge = attemptZone(presentation.attempt.presentation.phase) === "merge" && Boolean(mergeHref);
	if (!showMerge) return <EngageAttemptButton onEngage={onEngage} />;
	return (
		<span className="relative z-10 inline-flex items-center gap-1.5">
			<EngageAttemptButton onEngage={onEngage} />
			<MergeAttemptLink href={mergeHref as string} />
		</span>
	);
}

/**
 * The board card for one attempt. Only the current attempt is interactive
 * (Engage scrolls the actionable detail panel into view) — historical attempts
 * in the lineage render read-only, the same way `ArchivedSessionCardAdapter`
 * keeps terminated sessions informational. A succeeded current attempt with a
 * real pull request additionally offers Merge, matching the Ready lane's
 * Instruct+Merge pairing on the generic Sessions board.
 */
export function AttemptCardAdapter({
	onEngage,
	presentation,
}: {
	onEngage: () => void;
	presentation: AttemptBoardPresentation;
}) {
	const { t } = useTranslation();
	const translate: ProductUITranslator = (key, values) => t(key as MessageKey, values);
	return (
		<SessionCardView
			action={presentation.isCurrent ? <AttemptCardActions onEngage={onEngage} presentation={presentation} /> : undefined}
			externalLink={ProductExternalLink}
			interactive
			labels={attemptCardLabels(t)}
			onOpen={onEngage}
			renderAvatar={(provider) => <AgentAvatar className="h-7 w-[30px]" provider={provider} />}
			session={presentation}
			translate={translate}
		/>
	);
}

export function AttemptRowAdapter({
	onEngage,
	presentation,
}: {
	onEngage: () => void;
	presentation: AttemptBoardPresentation;
}) {
	const { t } = useTranslation();
	const translate: ProductUITranslator = (key, values) => t(key as MessageKey, values);
	return (
		<SessionRowView
			action={
				presentation.isCurrent ? (
					<AttemptCardActions onEngage={onEngage} presentation={presentation} />
				) : (
					<span aria-hidden="true" />
				)
			}
			externalLink={ProductExternalLink}
			interactive
			labels={attemptCardLabels(t)}
			onOpen={onEngage}
			renderAvatar={(provider) => <AgentAvatar className="h-7 w-[30px]" provider={provider} />}
			session={presentation}
			translate={translate}
		/>
	);
}
