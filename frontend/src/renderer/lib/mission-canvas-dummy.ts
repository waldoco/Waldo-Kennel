import type {
	MissionProjection,
	MissionProjectionNode,
	PlanRevision,
	Schedule,
	ScheduleWorkUnit,
} from "./mission-canvas-model";
import type { components } from "../../api/schema";

/**
 * Dummy data for canvas tests and visual evidence. Every builder returns the
 * FULL backend DTO shape from src/api/schema.ts - the same objects the live
 * hooks return - so the swap to real data is a no-op: tests exercise
 * modelFromMissionProjection / modelFromPlan with these, production calls
 * them with hook results. Builders deep-copy shared seed arrays so a caller
 * mutating one result cannot contaminate later calls.
 *
 * Deterministic by construction: fixed ids, fixed timestamps, fixed states.
 * The scenario is a realistic planning output for "fix session resume race":
 * a fan-out DAG with proven roots, an executing unit with a running attempt,
 * a blocked unit naming its upstream, a paused unit with an owner-attention
 * marker, and one retryable unit.
 */

export type AttemptRecord = components["schemas"]["AttemptResponse"];

const TS = "2026-09-19T02:00:00.000Z";
const OUTCOME_ID = "outcome-resume-race";
const PLAN_ID = "plan-resume-race-3";

type UnitSeed = {
	id: string;
	title: string;
	role: string;
	dependsOn: string[];
	criteria: number;
};

const UNITS: UnitSeed[] = [
	{ id: "wu-schema", title: "Compile lease evidence schema", role: "main", dependsOn: [], criteria: 2 },
	{ id: "wu-binding", title: "Bind resume to the same worktree lease", role: "main", dependsOn: ["wu-schema"], criteria: 3 },
	{ id: "wu-handshake", title: "Refuse duplicate resume for one session id", role: "codex", dependsOn: ["wu-schema"], criteria: 3 },
	{ id: "wu-sweep", title: "Sweep orphaned leases on daemon restart", role: "beta", dependsOn: ["wu-binding"], criteria: 2 },
	{ id: "wu-island", title: "Bind Island polling to loopback session state", role: "beta", dependsOn: ["wu-binding", "wu-handshake"], criteria: 2 },
	{ id: "wu-tests", title: "Cover resume race in regression tests", role: "codex", dependsOn: ["wu-handshake"], criteria: 2 },
	{ id: "wu-docs", title: "Document lease invariants for operators", role: "main", dependsOn: ["wu-sweep"], criteria: 1 },
	{ id: "wu-doctor", title: "Report zero orphaned leases from kennel doctor", role: "codex", dependsOn: ["wu-sweep", "wu-island", "wu-tests"], criteria: 3 },
];

function criterionIds(count: number): string[] {
	return Array.from({ length: count }, (_, index) => `criterion-${index + 1}`);
}

function planWorkUnits(): PlanRevision["workUnits"] {
	return UNITS.map((unit, index) => ({
		approvedChecks: [],
		contractRevisionNumber: 7,
		criterionIds: criterionIds(unit.criteria),
		dependsOn: [...unit.dependsOn],
		evidenceChecks: criterionIds(unit.criteria),
		id: unit.id,
		inputs: [],
		kind: "implementation",
		outputSummary: "",
		position: index + 1,
		requiredCapabilities: [],
		role: unit.role,
		stopConditions: [],
		title: unit.title,
		verificationRequirement: "objective",
	}));
}

export function dummyPlanRevision(): PlanRevision {
	return {
		assumptions: [],
		blockers: [],
		contractRevisionNumber: 7,
		createdAt: TS,
		grants: [],
		id: PLAN_ID,
		number: 3,
		outcomeId: OUTCOME_ID,
		routingDecisions: [],
		runBriefCoreDigest: "digest",
		status: "approved",
		summary: "Fix the session resume race without duplicating sessions.",
		workUnits: planWorkUnits(),
	};
}

type LiveState = {
	state: ScheduleWorkUnit["state"];
	ready?: number;
	blockedReason?: ScheduleWorkUnit["blockedReason"];
	blockedDetail?: string;
	blockingDependencies?: string[];
};

const LIVE_STATES: Record<string, LiveState> = {
	"wu-schema": { state: "proven", ready: 2 },
	"wu-binding": { state: "proven", ready: 3 },
	"wu-handshake": { state: "executing", ready: 1 },
	"wu-sweep": { state: "runnable", ready: 0 },
	"wu-island": {
		state: "blocked",
		ready: 0,
		blockedReason: "awaiting_dependency_proof",
		blockedDetail: "Needs wu-handshake proven before the loopback binding can be verified.",
		blockingDependencies: ["wu-handshake"],
	},
	"wu-tests": { state: "paused", ready: 1 },
	"wu-docs": { state: "retryable", ready: 0 },
	"wu-doctor": { state: "runnable", ready: 0 },
};

export function dummySchedule(plan: PlanRevision = dummyPlanRevision()): Schedule {
	return {
		custodyHeldByWorkUnitId: "wu-handshake",
		nextRunnableWorkUnitId: "wu-sweep",
		outcomeId: OUTCOME_ID,
		plan,
		workUnits: plan.workUnits.map((unit) => {
			const live = LIVE_STATES[unit.id];
			return {
				attempts: [],
				blockedDetail: live?.blockedDetail,
				blockedReason: live?.blockedReason,
				blockingDependencies: [...(live?.blockingDependencies ?? [])],
				criterionReady: Object.fromEntries(
					unit.criterionIds.map((criterion, index) => [criterion, index < (live?.ready ?? 0)]),
				),
				state: live?.state ?? "runnable",
				workUnit: unit,
			};
		}),
	};
}

function missionNode(unit: UnitSeed, generation: number): MissionProjectionNode {
	const live = LIVE_STATES[unit.id];
	const executing = live.state === "executing";
	const paused = live.state === "paused";
	return {
		...(paused
			? {
					attention: {
						generation: String(generation),
						kind: "needs_choice",
						questionId: "q-retry-budget",
						summary: "Owner decision needed: keep the resume retry budget at 2?",
					},
				}
			: {}),
		blockedDetail: live.blockedDetail,
		blockedReason: live.blockedReason,
		blockingDependencies: [...(live.blockingDependencies ?? [])],
		criterionIds: criterionIds(unit.criteria),
		criterionReady: Object.fromEntries(
			criterionIds(unit.criteria).map((criterion, index) => [criterion, index < (live.ready ?? 0)]),
		),
		...(executing
			? {
					currentAttempt: {
						attemptId: "attempt-resume-2",
						createdAt: TS,
						number: 2,
						session: {
							attemptSessionRefId: "asr-resume-2",
							boundAt: TS,
							generation,
							harness: "codex",
							mode: "worker",
							sessionId: "session-resume-2",
							status: "unknown" as const,
						},
						status: "running",
						updatedAt: TS,
					},
				}
			: {}),
		dependsOn: [...unit.dependsOn],
		generation,
		inputs: [],
		links: [],
		planRevisionId: PLAN_ID,
		responsibility: paused ? ("owner" as const) : ("agent" as const),
		role: unit.role,
		scheduleState: live.state,
		title: unit.title,
		updatedAt: TS,
		workUnitId: unit.id,
	};
}

export function dummyMissionProjection(): MissionProjection {
	const nodes = UNITS.map((unit, index) => missionNode(unit, index + 1));
	return {
		contractRevisionNumber: 7,
		custodyHeldByWorkUnitId: "wu-handshake",
		edges: UNITS.flatMap((unit) => unit.dependsOn.map((from) => ({ from, to: unit.id }))),
		generation: 12,
		missionId: "mission-resume-race",
		missionLabel: "Fix the session resume race",
		nextRunnableWorkUnitId: "wu-sweep",
		nodes,
		outcomeId: OUTCOME_ID,
		planRevisionId: PLAN_ID,
		planRevisionNumber: 3,
		topologyFingerprint: "topology-resume-race-3",
		topologyGeneration: 3,
		updatedAt: TS,
		version: 12,
	};
}

export function dummyAttempts(): AttemptRecord[] {
	const base = {
		contractRevisionNumber: 7,
		outcomeId: OUTCOME_ID,
		planRevisionId: PLAN_ID,
		workUnitId: "wu-handshake",
	};
	return [
		{
			...base,
			createdAt: TS,
			id: "attempt-resume-1",
			number: 1,
			observations: [],
			presentation: { endedUnclassified: false, nextAction: "Retry with the lease fence fix", phase: "halted_failed", unconfirmed: false },
			receipts: [],
			sessions: [],
			status: "failed",
			updatedAt: TS,
		},
		{
			...base,
			createdAt: TS,
			id: "attempt-resume-2",
			number: 2,
			observations: [],
			presentation: { endedUnclassified: false, nextAction: "Executing", phase: "executing", unconfirmed: false },
			receipts: [],
			sessions: [],
			status: "running",
			updatedAt: TS,
		},
	];
}
