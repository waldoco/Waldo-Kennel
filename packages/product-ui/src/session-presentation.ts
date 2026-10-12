import type {
	SessionActivity,
	SessionActivityState,
	SessionStatus,
	SessionStatusModel,
} from "./session-models";
import type { StateTone } from "./StateBadge";

export type SessionPresentationMessageKey =
	| `activity.${SessionActivityState}`
	| `status.${SessionStatus}`
	| `zone.${AttentionZone}`
	| `timeline.${SessionTimelinePillStatus}`;

export type ProductUITranslator = (
	key: SessionPresentationMessageKey,
	values?: Readonly<Record<string, string | number>>,
) => string;

const englishLabels: Record<SessionPresentationMessageKey, string> = {
	"activity.active": "Working",
	"activity.idle": "Idle",
	"activity.waiting_input": "Input needed",
	"activity.blocked": "Awaiting decision",
	"activity.exited": "Exited",
	"activity.unknown": "Unknown",
	"status.working": "Working",
	"status.idle": "Idle",
	"status.needs_input": "Input needed",
	"status.exited": "Exited",
	"status.no_signal": "No signal",
	"status.ci_failed": "CI failed",
	"status.changes_requested": "Changes requested",
	"status.review_pending": "Review pending",
	"status.draft": "Draft PR",
	"status.pr_open": "PR open",
	"status.approved": "Approved",
	"status.mergeable": "Ready",
	"status.merged": "Merged",
	"status.terminated": "Terminated",
	"status.unknown": "Unknown status",
	"zone.merge": "Ready",
	"zone.action": "Needs choice",
	"zone.pending": "Needs input",
	"zone.working": "Running",
	"zone.done": "Terminated",
	"timeline.no_signal": "No signal",
	"timeline.ci_failed": "CI failed",
	"timeline.changes_requested": "Changes requested",
};

export const defaultProductUITranslator: ProductUITranslator = (key) => englishLabels[key];

export type AgentActivityView = {
	state: SessionActivityState;
	label: string;
	tone: string;
	dotClassName: string;
	indicatorClassName: string;
	/** True for live work. Hosts may mark it with a static halo; it never
	 *  animates — Waldo states don't pulse. */
	breathe: boolean;
	/** The Waldo state tone (shape cue + colour) for this activity. */
	stateTone: StateTone;
};

type AgentActivityBase = Omit<AgentActivityView, "label" | "indicatorClassName"> & {
	labelKey: SessionPresentationMessageKey;
};

const agentActivityBases: Record<SessionActivityState, AgentActivityBase> = {
	active: {
		state: "active",
		labelKey: "activity.active",
		tone: "var(--color-status-working)",
		dotClassName: "bg-status-working",
		breathe: true,
		stateTone: "live",
	},
	idle: {
		state: "idle",
		labelKey: "activity.idle",
		tone: "var(--color-status-idle)",
		dotClassName: "bg-status-idle",
		breathe: false,
		stateTone: "neutral",
	},
	waiting_input: {
		state: "waiting_input",
		labelKey: "activity.waiting_input",
		tone: "var(--color-status-needs-you)",
		dotClassName: "bg-status-needs-you",
		breathe: false,
		stateTone: "needsYou",
	},
	blocked: {
		state: "blocked",
		labelKey: "activity.blocked",
		tone: "var(--color-status-needs-you)",
		dotClassName: "bg-status-needs-you",
		breathe: false,
		stateTone: "needsYou",
	},
	exited: {
		state: "exited",
		labelKey: "activity.exited",
		tone: "var(--color-status-exited)",
		dotClassName: "bg-status-exited",
		breathe: false,
		stateTone: "critical",
	},
	unknown: {
		state: "unknown",
		labelKey: "activity.unknown",
		tone: "var(--color-status-unknown)",
		dotClassName: "bg-status-unknown",
		breathe: false,
		stateTone: "neutral",
	},
};

export function getAgentActivityView(
	activity?: SessionActivity | null,
	translate: ProductUITranslator = defaultProductUITranslator,
): AgentActivityView {
	const state = activity?.state ?? "unknown";
	const base = agentActivityBases[state] ?? agentActivityBases.unknown;
	const { labelKey, ...view } = base;
	return {
		...view,
		label: translate(labelKey),
		// No pulse: live work is a still dot plus its word.
		indicatorClassName: view.dotClassName,
	};
}

export function isAgentActivityWorking(activity?: SessionActivity | null): boolean {
	return activity?.state === "active";
}

export type SessionStatusView = {
	label: string;
	className: string;
	cardClassName?: string;
	/** The Waldo state tone (shape cue + colour) for this status. */
	stateTone: StateTone;
};

const sessionStatusStyles: Record<SessionStatus, Omit<SessionStatusView, "label">> = {
	working: { className: "text-status-working", stateTone: "live" },
	idle: { className: "text-status-idle", stateTone: "neutral" },
	needs_input: { className: "text-status-needs-you", stateTone: "needsYou" },
	exited: { className: "text-status-exited", stateTone: "critical" },
	no_signal: { className: "text-status-unknown", stateTone: "neutral" },
	ci_failed: { className: "text-status-exited", stateTone: "critical" },
	changes_requested: { className: "text-status-needs-you", stateTone: "needsYou" },
	review_pending: { className: "text-status-in-review", stateTone: "caution" },
	draft: { className: "text-status-in-review", stateTone: "neutral" },
	pr_open: { className: "text-status-in-review", stateTone: "caution" },
	approved: { className: "text-status-ready", stateTone: "good" },
	mergeable: { className: "text-status-ready", stateTone: "good" },
	merged: { className: "text-status-merged", stateTone: "good" },
	terminated: {
		className: "text-status-terminated-foreground",
		cardClassName: "session-card-terminated",
		stateTone: "neutral",
	},
	unknown: { className: "text-status-unknown", stateTone: "neutral" },
};

export function getSessionStatusView(
	status: SessionStatus,
	translate: ProductUITranslator = defaultProductUITranslator,
): SessionStatusView {
	const normalizedStatus = sessionStatusStyles[status] ? status : "unknown";
	return {
		...sessionStatusStyles[normalizedStatus],
		label: translate(`status.${normalizedStatus}`),
	};
}

export type AttentionZone = "merge" | "action" | "pending" | "working" | "done";

export type AttentionZoneView = {
	zone: AttentionZone;
	label: string;
	glow: string;
	dot: string;
	dotGlow: boolean;
	titleClassName: string;
	dotClassName: string;
	/** The Waldo state tone for the lane heading's shape cue. */
	stateTone: StateTone;
};

type AttentionZoneBase = Omit<AttentionZoneView, "label"> & {
	labelKey: SessionPresentationMessageKey;
};

const attentionZoneBases: Record<AttentionZone, AttentionZoneBase> = {
	working: {
		zone: "working",
		labelKey: "zone.working",
		glow: "color-mix(in srgb, var(--color-status-working) 7%, transparent)",
		dot: "var(--color-status-working)",
		dotGlow: true,
		titleClassName: "text-status-working",
		dotClassName: "bg-status-working",
		stateTone: "live",
	},
	action: {
		zone: "action",
		labelKey: "zone.action",
		glow: "color-mix(in srgb, var(--color-status-needs-you) 6%, transparent)",
		dot: "var(--color-status-needs-you)",
		dotGlow: true,
		titleClassName: "text-status-needs-you",
		dotClassName: "bg-status-needs-you",
		stateTone: "needsYou",
	},
	pending: {
		zone: "pending",
		labelKey: "zone.pending",
		glow: "color-mix(in srgb, var(--color-status-in-review) 5%, transparent)",
		dot: "var(--color-status-in-review)",
		dotGlow: false,
		titleClassName: "text-status-in-review",
		dotClassName: "bg-status-in-review",
		stateTone: "caution",
	},
	merge: {
		zone: "merge",
		labelKey: "zone.merge",
		glow: "color-mix(in srgb, var(--color-status-ready) 7%, transparent)",
		dot: "var(--color-status-ready)",
		dotGlow: true,
		titleClassName: "text-status-ready",
		dotClassName: "bg-status-ready",
		stateTone: "good",
	},
	done: {
		zone: "done",
		labelKey: "zone.done",
		glow: "var(--color-overlay-faint)",
		dot: "var(--color-status-terminated)",
		dotGlow: false,
		titleClassName: "text-status-terminated-foreground",
		dotClassName: "bg-status-terminated",
		stateTone: "neutral",
	},
};

export const attentionZoneOrder: AttentionZone[] = ["merge", "action", "pending", "working", "done"];
/**
 * Board lane order, left to right: what is blocked on a person first, what is
 * waiting on someone else next, what is finished after that, and what the agents
 * are still doing last. Reading the board left to right is reading your queue in
 * priority order — running work is the column you glance at, not the one you act on.
 */
export const boardAttentionZoneOrder: AttentionZone[] = ["action", "pending", "merge", "working"];

export function attentionZone(input: SessionStatus | SessionStatusModel): AttentionZone {
	const status = typeof input === "string" ? input : input.status;
	switch (status) {
		case "merged":
		case "approved":
		case "mergeable":
			return "merge";
		case "terminated":
			return "done";
		case "needs_input":
		case "exited":
		case "no_signal":
		case "ci_failed":
		case "changes_requested":
		case "unknown":
			return "action";
		case "review_pending":
		case "pr_open":
		case "draft":
			return "pending";
		case "working":
		case "idle":
			return "working";
	}
}

export function getAttentionZoneView(
	status: SessionStatus,
	translate: ProductUITranslator = defaultProductUITranslator,
): AttentionZoneView {
	return getAttentionZoneViewForZone(attentionZone(status), translate);
}

export function getAttentionZoneViewForZone(
	zone: AttentionZone,
	translate: ProductUITranslator = defaultProductUITranslator,
): AttentionZoneView {
	const { labelKey, ...view } = attentionZoneBases[zone];
	return { ...view, label: translate(labelKey) };
}

export type SessionTimelinePillStatus = Extract<
	SessionStatus,
	"no_signal" | "ci_failed" | "changes_requested"
>;

export type SessionTimelinePillView = {
	label: string;
	tone: string;
	breathe: boolean;
};

const sessionTimelinePillBases: Record<
	SessionTimelinePillStatus,
	{ labelKey: SessionPresentationMessageKey; tone: string; breathe: boolean }
> = {
	no_signal: {
		labelKey: "timeline.no_signal",
		tone: "var(--color-status-unknown)",
		breathe: false,
	},
	ci_failed: {
		labelKey: "timeline.ci_failed",
		tone: "var(--color-status-exited)",
		breathe: false,
	},
	changes_requested: {
		labelKey: "timeline.changes_requested",
		tone: "var(--color-status-needs-you)",
		breathe: false,
	},
};

export function getSessionTimelinePillView(
	status: SessionTimelinePillStatus,
	translate: ProductUITranslator = defaultProductUITranslator,
): SessionTimelinePillView {
	const base = sessionTimelinePillBases[status];
	return {
		label: translate(base.labelKey),
		tone: base.tone,
		breathe: base.breathe,
	};
}

export function isSessionIdle(session: SessionStatusModel): boolean {
	return session.status === "idle";
}
