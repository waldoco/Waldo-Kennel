import type { TFunction } from "i18next";

import type {
	MissionAttentionRecord,
	MissionEdgeRecord,
	MissionNodeRecord,
	MissionRecord,
} from "../../hooks/useOutcome";
import type { MessageKey } from "../../i18n/messages";

/**
 * F2's single closed presentation adapter (packet scope item 4). `/mission`
 * is the sole runtime graph contract — this module maps its raw,
 * loosely-typed fields (`scheduleState` and `nextAction` arrive as bare
 * strings, not enums) onto the product status canon. Every map here fails
 * closed to an explicit "unavailable" reading on an unrecognized value; none
 * of them nearest-match. Nothing in this file infers authority, session
 * liveness, or progress beyond what the projection states.
 */

// backend/internal/service/outcome/scheduler.go WorkUnitScheduleState constants,
// named once so an unrecognized value (a schema drift, not a valid state) is
// distinguishable at the type level from every state this renderer knows.
const SCHEDULE_STATES = ["blocked", "runnable", "executing", "proven", "retryable", "paused"] as const;
export type MissionScheduleState = (typeof SCHEDULE_STATES)[number];
export type MissionCanonStatus = MissionScheduleState | "unavailable";

export interface MissionStatusView {
	status: MissionCanonStatus;
	label: string;
	className: string;
	indicatorClassName: string;
}

const STATUS_LABEL_KEYS: Record<MissionScheduleState, MessageKey> = {
	blocked: "mission.status.blocked",
	runnable: "mission.status.ready",
	executing: "mission.status.running",
	proven: "mission.status.proven",
	retryable: "mission.status.retryable",
	paused: "mission.status.paused",
};

const STATUS_TONES: Record<MissionScheduleState, { className: string; indicatorClassName: string }> = {
	blocked: { className: "text-status-idle", indicatorClassName: "bg-status-idle" },
	runnable: { className: "text-status-ready", indicatorClassName: "bg-status-ready" },
	executing: { className: "text-status-working", indicatorClassName: "bg-status-working animate-status-pulse" },
	proven: { className: "text-status-merged", indicatorClassName: "bg-status-merged" },
	retryable: { className: "text-status-needs-you", indicatorClassName: "bg-status-needs-you" },
	paused: { className: "text-status-idle", indicatorClassName: "bg-status-idle" },
};

function isMissionScheduleState(value: string): value is MissionScheduleState {
	return (SCHEDULE_STATES as readonly string[]).includes(value);
}

/** `scheduleState` is a bare string in the DTO. An unrecognized value (a
 *  future backend addition this renderer has not been taught) fails closed
 *  to "State unavailable" — it never nearest-matches to the closest-looking
 *  known state. */
export function missionStatusView(scheduleState: string, t: TFunction): MissionStatusView {
	if (!isMissionScheduleState(scheduleState)) {
		return {
			status: "unavailable",
			label: t("mission.status.unavailable" satisfies MessageKey),
			className: "text-status-unknown",
			indicatorClassName: "bg-status-unknown",
		};
	}
	return {
		status: scheduleState,
		label: t(STATUS_LABEL_KEYS[scheduleState]),
		...STATUS_TONES[scheduleState],
	};
}

export type MissionAttentionKind = "needs_approval" | "needs_choice" | "needs_input";
/** The visual grouping invariant D calls for: needs_approval and needs_choice
 *  both dominate as an owner-decision cue; needs_input dominates as an input
 *  cue. This groups color/icon only — the typed kind itself is never
 *  collapsed, so a strip can still count the three separately. */
export type MissionAttentionVisualCategory = "owner_decision" | "input";

export interface MissionAttentionView {
	kind: MissionAttentionKind;
	visualCategory: MissionAttentionVisualCategory;
	label: string;
	summary: string;
	className: string;
	indicatorClassName: string;
	/** One-time disclosure marker key. `attention.generation` (a string) is the
	 *  Needs-You question's own opaque token — it changes exactly when the
	 *  underlying question changes, which is the transition a one-time
	 *  disclosure must key off. This is distinct from the node/projection
	 *  int64 `generation` (a content digest of the whole node/graph) and from
	 *  the session ref's `generation` (actually its Seq). */
	markerKey: string;
}

const ATTENTION_VISUAL_CATEGORY: Record<MissionAttentionKind, MissionAttentionVisualCategory> = {
	needs_approval: "owner_decision",
	needs_choice: "owner_decision",
	needs_input: "input",
};

const ATTENTION_LABEL_KEYS: Record<MissionAttentionKind, MessageKey> = {
	needs_approval: "mission.attention.needsApproval",
	needs_choice: "mission.attention.needsChoice",
	needs_input: "mission.attention.needsInput",
};

function isMissionAttentionKind(value: string): value is MissionAttentionKind {
	return value === "needs_approval" || value === "needs_choice" || value === "needs_input";
}

/** Returns undefined for both "no attention" and an unrecognized attention
 *  kind — a schema drift never displays as some *other* typed attention. */
export function missionAttentionView(
	workUnitId: string,
	attention: MissionAttentionRecord | undefined,
	t: TFunction,
): MissionAttentionView | undefined {
	if (!attention || !isMissionAttentionKind(attention.kind)) return undefined;
	const visualCategory = ATTENTION_VISUAL_CATEGORY[attention.kind];
	const tone =
		visualCategory === "owner_decision"
			? { className: "text-status-needs-you", indicatorClassName: "bg-status-needs-you" }
			: { className: "text-status-in-review", indicatorClassName: "bg-status-in-review" };
	return {
		kind: attention.kind,
		visualCategory,
		label: t(ATTENTION_LABEL_KEYS[attention.kind]),
		summary: attention.summary,
		...tone,
		markerKey: `${workUnitId}:${attention.generation}:${attention.kind}`,
	};
}

export type MissionResponsibility = "agent" | "owner" | "unconfirmed";

const RESPONSIBILITY_LABEL_KEYS: Record<MissionResponsibility, MessageKey> = {
	agent: "mission.responsibility.agent",
	owner: "mission.responsibility.owner",
	unconfirmed: "mission.responsibility.unconfirmed",
};

export function missionResponsibilityLabel(value: string, t: TFunction): string {
	if (value === "agent" || value === "owner" || value === "unconfirmed") {
		return t(RESPONSIBILITY_LABEL_KEYS[value]);
	}
	return t("mission.responsibility.unavailable" satisfies MessageKey);
}

/**
 * The backend's `MissionSessionResponse.status` enum is currently the
 * singleton `"unknown"` — there is no confirmed-live session status this
 * renderer is allowed to invent. A recognized "unknown" reads as Unknown; any
 * other raw value (a future backend addition, or a schema drift) reads as an
 * explicit unavailable rather than being coerced into Unknown *or* guessed as
 * Running — this is invariant B/item 7 in one function.
 */
export function missionSessionStatusLabel(value: string | undefined, t: TFunction): string {
	if (value === "unknown") return t("mission.session.unknown" satisfies MessageKey);
	return t("mission.session.unavailable" satisfies MessageKey);
}

export type MissionCriteriaView = { kind: "unavailable" } | { kind: "ready"; ready: number; total: number };

/** `criterionReady` arrives as `{...} | null`. Null is an explicit "not
 *  resolved yet", never zero-readiness. */
export function missionCriteriaView(node: MissionNodeRecord): MissionCriteriaView {
	if (!node.criterionReady) return { kind: "unavailable" };
	const readyMap = node.criterionReady;
	const total = node.criterionIds.length;
	const ready = node.criterionIds.filter((id) => readyMap[id] === true).length;
	return { kind: "ready", ready, total };
}

export interface MissionActionView {
	kind: "start";
	label: string;
}

/**
 * `nextAction` is only ever `"start"`. This alone is NOT trusted to enable a
 * node's action — invariant C/G require a projection-wide singleton, so this
 * only labels the action once `missionGraphView` has independently confirmed,
 * from the projection's own `nextRunnableWorkUnitId`/`custodyHeldByWorkUnitId`,
 * that this is *the* canonical actionable WorkUnit. A retryable node with no
 * returned action stays non-actionable; this never renders a
 * renderer-invented "Retry" — a returned Start on a retryable node stays
 * labeled Start.
 */
export function missionActionView(nextAction: string | undefined, t: TFunction): MissionActionView | undefined {
	if (nextAction !== "start") return undefined;
	return { kind: "start", label: t("mission.action.start" satisfies MessageKey) };
}

/**
 * The one WorkUnit id (if any) this graph may render an action for, derived
 * solely from the projection's own top-level fields — never from a node's
 * own claim in isolation. Custody currently held by a *different* WorkUnit
 * means something is already running under the serial execution fence, so
 * nothing is actionable regardless of what `nextRunnableWorkUnitId` (or any
 * node's `nextAction`) says. This is the single choke point invariant C/G
 * requires: at most one action, graph-wide, ever.
 */
function canonicalActionableWorkUnitId(mission: MissionRecord): string | undefined {
	const nextRunnable = mission.nextRunnableWorkUnitId;
	if (!nextRunnable) return undefined;
	const custodyHeldBy = mission.custodyHeldByWorkUnitId;
	if (custodyHeldBy && custodyHeldBy !== nextRunnable) return undefined;
	return nextRunnable;
}

export interface MissionNodeView {
	workUnitId: string;
	title: string;
	status: MissionStatusView;
	attention?: MissionAttentionView;
	attemptLabel?: string;
	sessionStatusLabel?: string;
	action?: MissionActionView;
	criteria: MissionCriteriaView;
	responsibilityLabel: string;
	/** True when this node declares a dependency on a WorkUnit id absent from
	 *  the projection's own node set — an edge endpoint the graph cannot draw. */
	dependencyUnavailable: boolean;
	/** Drawn edge counts only (post unknown-endpoint filtering) — for an
	 *  accessible dependency *summary* ("2 upstream, 1 downstream"), never
	 *  prose naming other WorkUnits. The bounded node face shows neither;
	 *  this feeds only the assistive-tech description. */
	dependencyCount: number;
	dependentCount: number;
	generation: number;
}

function missionNodeViewWithoutDependencyFlag(node: MissionNodeRecord, t: TFunction, isCanonicalActionable: boolean): MissionNodeView {
	return {
		workUnitId: node.workUnitId,
		title: node.title,
		status: missionStatusView(node.scheduleState, t),
		attention: missionAttentionView(node.workUnitId, node.attention, t),
		attemptLabel: node.currentAttempt
			? t("mission.attempt.number" satisfies MessageKey, { number: node.currentAttempt.number })
			: undefined,
		sessionStatusLabel: node.currentAttempt?.session
			? missionSessionStatusLabel(node.currentAttempt.session.status, t)
			: undefined,
		// Never this node's own `nextAction` in isolation — only the one
		// WorkUnit `missionGraphView` independently confirmed as canonical
		// gets to turn its (agreeing) `nextAction` into a rendered action.
		action: isCanonicalActionable ? missionActionView(node.nextAction, t) : undefined,
		criteria: missionCriteriaView(node),
		responsibilityLabel: missionResponsibilityLabel(node.responsibility, t),
		dependencyUnavailable: false,
		dependencyCount: 0,
		dependentCount: 0,
		generation: node.generation,
	};
}

export interface MissionGraphView {
	nodes: MissionNodeView[];
	/** Only edges whose both endpoints exist among `nodes` — an edge with an
	 *  unknown endpoint is never drawn (invariant/scope item 6). */
	edges: MissionEdgeRecord[];
	nodesByWorkUnitId: ReadonlyMap<string, MissionNodeView>;
}

/** The one place raw `/mission` JSON becomes render-safe view state. Bijects
 *  with `mission.nodes`/`mission.edges` (invariant A): every returned node
 *  becomes exactly one view, every drawable edge is kept, nothing is added. */
export function missionGraphView(mission: MissionRecord, t: TFunction): MissionGraphView {
	const nodeIds = new Set(mission.nodes.map((node) => node.workUnitId));
	const edges = mission.edges.filter((edge) => nodeIds.has(edge.from) && nodeIds.has(edge.to));
	const dependencyCounts = new Map<string, number>();
	const dependentCounts = new Map<string, number>();
	for (const edge of edges) {
		dependentCounts.set(edge.from, (dependentCounts.get(edge.from) ?? 0) + 1);
		dependencyCounts.set(edge.to, (dependencyCounts.get(edge.to) ?? 0) + 1);
	}
	const canonicalActionableWorkUnit = canonicalActionableWorkUnitId(mission);
	const nodes = mission.nodes.map((node) => ({
		...missionNodeViewWithoutDependencyFlag(node, t, node.workUnitId === canonicalActionableWorkUnit),
		dependencyUnavailable: node.dependsOn.some((dependsOnId) => !nodeIds.has(dependsOnId)),
		dependencyCount: dependencyCounts.get(node.workUnitId) ?? 0,
		dependentCount: dependentCounts.get(node.workUnitId) ?? 0,
	}));
	return { nodes, edges, nodesByWorkUnitId: new Map(nodes.map((node) => [node.workUnitId, node])) };
}

export interface MissionTopologyIdentity {
	planRevisionId: string;
	topologyFingerprint: string;
	topologyGeneration: number;
}

export function missionTopologyIdentity(mission: MissionRecord): MissionTopologyIdentity {
	return {
		planRevisionId: mission.planRevisionId,
		topologyFingerprint: mission.topologyFingerprint,
		topologyGeneration: mission.topologyGeneration,
	};
}

/** Topology identity, not projection `generation` — a state-only refetch
 *  changes `generation` (and node generations) while every field here holds
 *  still; only an authorized Plan swap moves any of these three. */
export function sameMissionTopology(a: MissionTopologyIdentity, b: MissionTopologyIdentity): boolean {
	return (
		a.planRevisionId === b.planRevisionId &&
		a.topologyFingerprint === b.topologyFingerprint &&
		a.topologyGeneration === b.topologyGeneration
	);
}

/** Deterministic fallback focus when the selected WorkUnit disappears across
 *  a topology swap: the lexicographically smallest surviving id. Never
 *  insertion/server order — that is a transport detail, not a stable UI
 *  contract the same swap must reproduce on every client. */
export function missionDeterministicSuccessor(nodes: readonly { workUnitId: string }[]): string | undefined {
	if (nodes.length === 0) return undefined;
	return [...nodes].map((node) => node.workUnitId).sort()[0];
}
