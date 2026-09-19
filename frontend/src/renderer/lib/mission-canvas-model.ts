import type { components } from "../../api/schema";

/**
 * The one view model the mission graph canvas reads. Both production inputs
 * land here: the canonical daemon MissionProjection (live execution truth)
 * and a Plan revision with its optional Schedule (proposed topology before
 * and after authorization). The canvas never sees a DTO: it sees this model,
 * so swapping between test fixtures and the live hooks is a data-layer
 * concern only.
 *
 * Truthfulness rule carried from the board: every state, blocker and reason
 * is a daemon fact mapped through, never a renderer-derived guess. An
 * unrecognized schedule state maps to "unknown" and keeps its raw label
 * rather than being rounded into a state it is not. Criterion readiness
 * mirrors the production contract (missionCriteriaView, useOutcome): a null
 * criterionReady map means "unavailable / not resolved yet" and stays
 * distinguishable from a resolved map with zero ready criteria.
 */

export type MissionProjection = components["schemas"]["ControllersMissionProjectionResponse"];
export type MissionProjectionNode = components["schemas"]["ControllersMissionNodeResponse"];
export type PlanRevision = components["schemas"]["PlanRevisionResponse"];
export type Schedule = components["schemas"]["ScheduleResponse"];
export type ScheduleWorkUnit = components["schemas"]["ScheduleWorkUnitResponse"];

export type MissionCanvasNodeState =
	| "proposed"
	| "proven"
	| "executing"
	| "paused"
	| "retryable"
	| "blocked"
	| "runnable"
	| "unknown";

export type MissionCanvasNode = {
	workUnitId: string;
	title: string;
	state: MissionCanvasNodeState;
	/** Raw daemon label when state is "unknown"; the face shows words, not guesses. */
	rawState?: string;
	/** Who must act next, per the projection: agent, owner, or unconfirmed. */
	responsibility?: "agent" | "owner" | "unconfirmed";
	/** Harness/role label from the Plan (e.g. codex, beta). Plan truth, not a badge we invent. */
	role?: string;
	upstream: string[];
	attentionSummary?: string;
	blockedReason?: string;
	blockedDetail?: string;
	attemptNumber?: number;
	attemptStatus?: string;
	/**
	 * Daemon criterion readiness, preserving the production contract: null
	 * means the daemon has not resolved readiness for this unit yet. That is
	 * different from a resolved map where every criterion is false, and the
	 * face must not render one as the other.
	 */
	criterionReadiness: { available: true; ready: number } | { available: false };
	criterionTotal: number;
	isNextRunnable: boolean;
	holdsCustody: boolean;
	/** Projection node generation: a state-only refresh bumps this without touching topology. */
	generation?: number;
};

export type MissionCanvasEdge = { from: string; to: string };

export type MissionCanvasModel = {
	nodes: MissionCanvasNode[];
	edges: MissionCanvasEdge[];
	/**
	 * Cache key for layout: identical topology -> identical key -> cached
	 * positions. From the projection's own fingerprint when live; derived
	 * from the Plan revision identity when proposed.
	 */
	topologyKey: string;
	topologyGeneration?: number;
	noRunnableReason?: string;
	source: "projection" | "plan";
};

const KNOWN_STATES: ReadonlySet<string> = new Set(["proven", "executing", "paused", "retryable", "blocked", "runnable"]);

function normalizeState(raw: string | undefined): { state: MissionCanvasNodeState; rawState?: string } {
	if (raw && KNOWN_STATES.has(raw)) return { state: raw as MissionCanvasNodeState };
	if (raw) return { state: "unknown", rawState: raw };
	return { state: "proposed" };
}

function readiness(
	criterionReady: Record<string, boolean> | null | undefined,
): { available: true; ready: number } | { available: false } {
	if (criterionReady == null) return { available: false };
	return { available: true, ready: Object.values(criterionReady).filter(Boolean).length };
}

function sortEdges(edges: MissionCanvasEdge[]): MissionCanvasEdge[] {
	return [...edges].sort((left, right) => `${left.from}:${left.to}`.localeCompare(`${right.from}:${right.to}`));
}


/** Deterministic first-wins de-duplication. The daemon projection should be
 * unique, but the renderer must never create duplicate React Flow identities
 * or duplicate topology edges from a malformed/replayed payload. */
function firstByIdentity<T>(values: readonly T[], identity: (value: T) => string): T[] {
	const seen = new Set<string>();
	return values.filter((value) => {
		const key = identity(value);
		if (!key || seen.has(key)) return false;
		seen.add(key);
		return true;
	});
}

/** Live path: the canonical daemon projection is the single source of truth. */
export function modelFromMissionProjection(projection: MissionProjection): MissionCanvasModel {
	const nodes: MissionCanvasNode[] = firstByIdentity(
		(projection.nodes ?? []).filter((node) => Boolean(node?.workUnitId)),
		(node) => node.workUnitId,
	)
		.map((node) => {
			const { state, rawState } = normalizeState(node.scheduleState);
			return {
				workUnitId: node.workUnitId,
				title: node.title,
				state,
				rawState,
				responsibility: node.responsibility,
				role: node.role || undefined,
				upstream: [...(node.dependsOn ?? [])].sort(),
				attentionSummary: node.attention?.summary,
				blockedReason: node.blockedReason,
				blockedDetail: node.blockedDetail,
				attemptNumber: node.currentAttempt?.number,
				attemptStatus: node.currentAttempt?.status,
				criterionReadiness: readiness(node.criterionReady),
				criterionTotal: node.criterionIds?.length ?? 0,
				isNextRunnable: projection.nextRunnableWorkUnitId === node.workUnitId,
				holdsCustody: projection.custodyHeldByWorkUnitId === node.workUnitId,
				generation: node.generation,
			};
		});
	return {
		nodes,
		edges: sortEdges(
			firstByIdentity(
				(projection.edges ?? []).map((edge) => ({ from: edge.from, to: edge.to })),
				(edge) => `${edge.from}->${edge.to}`,
			),
		),
		topologyKey: projection.topologyFingerprint ?? `projection:${projection.missionId}@${projection.topologyGeneration}`,
		topologyGeneration: projection.topologyGeneration,
		noRunnableReason: projection.noRunnableReason,
		source: "projection",
	};
}

/**
 * Proposed path: Plan topology with an optional Schedule overlay. Mirrors the
 * contract MissionWorkUnitGraph already renders - before authorization there
 * is no state, and a proposal is not a runnable schedule.
 */
export function modelFromPlan(plan: PlanRevision, schedule?: Schedule): MissionCanvasModel {
	const entries = new Map(
		(schedule?.workUnits ?? [])
			.filter((entry) => Boolean(entry?.workUnit?.id))
			.map((entry) => [entry.workUnit.id, entry]),
	);
	const nodes: MissionCanvasNode[] = firstByIdentity(
		(plan.workUnits ?? []).filter((unit) => Boolean(unit?.id)),
		(unit) => unit.id,
	)
		.sort((left, right) => left.position - right.position)
		.map((unit) => {
			const entry = entries.get(unit.id);
			const { state, rawState } = entry ? normalizeState(entry.state) : { state: "proposed" as const, rawState: undefined };
			return {
				workUnitId: unit.id,
				title: unit.title,
				state,
				rawState,
				role: unit.role || undefined,
				upstream: [...(unit.dependsOn ?? [])].sort(),
				blockedReason: entry?.blockedReason,
				blockedDetail: entry?.blockedDetail,
				criterionReadiness: entry ? readiness(entry.criterionReady) : { available: false },
				criterionTotal: unit.criterionIds?.length ?? 0,
				isNextRunnable: schedule?.nextRunnableWorkUnitId === unit.id,
				holdsCustody: schedule?.custodyHeldByWorkUnitId === unit.id,
			};
		});
	return {
		nodes,
		edges: sortEdges(
			firstByIdentity(
				nodes.flatMap((node) => node.upstream.map((from) => ({ from, to: node.workUnitId }))),
				(edge) => `${edge.from}->${edge.to}`,
			),
		),
		topologyKey: `plan:${plan.id}@${plan.number}`,
		noRunnableReason: schedule?.noRunnableReason,
		source: "plan",
	};
}
