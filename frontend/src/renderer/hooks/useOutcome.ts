/**
 * Outcome contract reads and writes for the Understand stage.
 *
 * The renderer is a thin surface over the daemon's canonical Outcome API: it
 * never derives contract state locally, never writes SQLite, and never claims
 * a save the daemon has not answered. Every mutation returns the authoritative
 * OutcomeEnvelope, and the caller renders from that response — there are no
 * optimistic Outcome claims here.
 */

import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { useRef } from "react";

import type { components } from "../../api/schema";
import { apiClient, apiErrorCode, apiErrorMessage } from "../lib/api-client";
import { usesPreviewWorkspaceData } from "../lib/preview-mode";
import {
	approvePreviewPlan,
	createPreviewOutcome,
	getPreviewOutcome,
	getPreviewPlan,
	listPreviewOutcomes,
	proposePreviewPlan,
	revisePreviewOutcome,
} from "../lib/preview-outcome-store";

export type OutcomeRecord = components["schemas"]["OutcomeResponse"];
export type ContractRevisionRecord = components["schemas"]["ContractRevisionResponse"];
export type CreateOutcomeRequest = components["schemas"]["CreateOutcomeRequest"];
export type ReviseOutcomeContractRequest = components["schemas"]["ReviseOutcomeContractRequest"];

type OutcomeEnvelope = components["schemas"]["OutcomeEnvelope"];
type OutcomesEnvelope = components["schemas"]["OutcomesEnvelope"];
type ApiErrorBody = components["schemas"]["APIError"];

export function outcomeQueryKey(outcomeId: string | undefined) {
	return ["outcome", outcomeId ?? ""] as const;
}

export function projectOutcomesQueryKey(projectId: string | undefined) {
	return ["project-outcomes", projectId ?? ""] as const;
}

/** The daemon's typed refusal when the Outcome does not exist. */
export const OUTCOME_NOT_FOUND = "OUTCOME_NOT_FOUND";
/** The daemon's typed refusal when a revision races another writer. */
export const OUTCOME_CONTRACT_CONFLICT = "OUTCOME_CONTRACT_CONFLICT";

/**
 * Codes that describe a permanently unactionable request: retrying the same
 * bytes cannot help, so the query/mutation must surface them instead of
 * spinning. Everything else is treated as transient.
 */
const PERMANENT_CODES = new Set([
	OUTCOME_NOT_FOUND,
	"PROJECT_NOT_FOUND",
	"PROJECT_REQUIRED",
	"REQUEST_KEY_REQUIRED",
	"EXPECTED_REVISION_REQUIRED",
	"OUTCOME_TITLE_REQUIRED",
	"OUTCOME_TITLE_TOO_LONG",
	"OUTCOME_GOAL_REQUIRED",
	"OUTCOME_CRITERIA_REQUIRED",
	"OUTCOME_REVIEW_REQUIRED",
	"PLAN_ID_REQUIRED",
	"PLAN_CAPABILITY_MISSING",
]);

/**
 * Classify a mutation/query failure for the Understand surface.
 *
 * - `conflict` — the daemon answered 409 OUTCOME_CONTRACT_CONFLICT; the
 *   details carry the expected/current revision numbers.
 * - `permanent` — a typed validation refusal; retrying cannot help.
 * - `offline` — the request never reached the daemon (no trusted base URL or
 *   a transport-level throw), so there is no code at all.
 * - otherwise `retryable` — the daemon (or network) failed transiently.
 */
export interface OutcomeFailure {
	kind: "conflict" | "permanent" | "offline" | "retryable";
	code?: string;
	message: string;
	expectedRevision?: number;
	currentRevision?: number;
}

function readRevisionDetail(details: unknown, key: string): number | undefined {
	if (typeof details !== "object" || details === null) return undefined;
	const value = (details as Record<string, unknown>)[key];
	return typeof value === "number" ? value : undefined;
}

export function classifyOutcomeFailure(error: unknown): OutcomeFailure {
	const code = apiErrorCode(error);
	const message = apiErrorMessage(error);
	if (code === OUTCOME_CONTRACT_CONFLICT) {
		const details = (typeof error === "object" && error !== null ? (error as ApiErrorBody).details : undefined);
		return {
			kind: "conflict",
			code,
			message,
			expectedRevision: readRevisionDetail(details, "expectedRevision"),
			currentRevision: readRevisionDetail(details, "currentRevision"),
		};
	}
	if (code && PERMANENT_CODES.has(code)) {
		return { kind: "permanent", code, message };
	}
	if (!code) {
		// Every daemon error envelope carries a code. No code means the request
		// never reached the daemon: untrusted base URL (synthetic 503) or a
		// transport failure.
		return { kind: "offline", message };
	}
	return { kind: "retryable", code, message };
}

export interface OutcomeQueryResult {
	outcome?: OutcomeRecord;
	isLoading: boolean;
	/** Set when the daemon answers that this Outcome does not exist. */
	unavailable?: { code: string; message: string };
	failure?: OutcomeFailure;
	refetch: () => void;
}

async function fetchOutcomeRecord(outcomeId: string): Promise<OutcomeRecord> {
	if (usesPreviewWorkspaceData) {
		const outcome = getPreviewOutcome(outcomeId);
		if (!outcome) throw { code: OUTCOME_NOT_FOUND, message: "The preview Outcome does not exist." };
		return outcome;
	}
	const { data, error } = await apiClient.GET("/api/v1/outcomes/{outcomeId}", {
		params: { path: { outcomeId } },
	});
	if (error) throw error;
	return (data as OutcomeEnvelope).outcome;
}

/**
 * Read the current authoritative Outcome through the shared cache, e.g. after
 * a conflict tells the surface its view is stale.
 */
export function refetchOutcome(queryClient: QueryClient, outcomeId: string): Promise<OutcomeRecord> {
	return queryClient.fetchQuery({
		queryKey: outcomeQueryKey(outcomeId),
		queryFn: () => fetchOutcomeRecord(outcomeId),
	});
}

export function useOutcome(outcomeId: string | undefined): OutcomeQueryResult {
	const query = useQuery({
		queryKey: outcomeQueryKey(outcomeId),
		enabled: Boolean(outcomeId),
		queryFn: () => fetchOutcomeRecord(outcomeId as string),
		retry: (attempt, error) => {
			const code = apiErrorCode(error);
			if (code && PERMANENT_CODES.has(code)) return false;
			return attempt < 2;
		},
	});

	if (query.error) {
		const failure = classifyOutcomeFailure(query.error);
		return {
			isLoading: query.isLoading,
			failure,
			refetch: () => {
				void query.refetch();
			},
			unavailable:
				failure.kind === "permanent"
					? { code: failure.code ?? "", message: failure.message }
					: undefined,
		};
	}

	return {
		outcome: query.data,
		isLoading: query.isLoading,
		refetch: () => {
			void query.refetch();
		},
	};
}

export interface ProjectOutcomesQueryResult {
	outcomes: OutcomeRecord[];
	isLoading: boolean;
	failure?: OutcomeFailure;
	refetch: () => void;
}

/** Durable Outcomes owned by one WorkProject responsibility space. */
export function useProjectOutcomes(projectId: string | undefined): ProjectOutcomesQueryResult {
	const query = useQuery({
		queryKey: projectOutcomesQueryKey(projectId),
		enabled: Boolean(projectId),
		queryFn: async () => {
			if (usesPreviewWorkspaceData) return listPreviewOutcomes(projectId as string);
			const { data, error } = await apiClient.GET("/api/v1/projects/{id}/outcomes", {
				params: { path: { id: projectId as string } },
			});
			if (error) throw error;
			return (data as OutcomesEnvelope).outcomes;
		},
		retry: (attempt, error) => {
			const code = apiErrorCode(error);
			if (code && PERMANENT_CODES.has(code)) return false;
			return attempt < 2;
		},
	});

	return {
		outcomes: query.data ?? [],
		isLoading: query.isLoading,
		failure: query.error ? classifyOutcomeFailure(query.error) : undefined,
		refetch: () => void query.refetch(),
	};
}

export interface OutcomeWriteState<TInput> {
	/** The daemon has accepted the write and returned the authoritative Outcome. */
	succeeded: boolean;
	pending: boolean;
	failure?: OutcomeFailure;
	reset: () => void;
	save: (input: TInput) => Promise<OutcomeRecord>;
}

function useOutcomeWrite<TInput>(mutationFn: (input: TInput) => Promise<OutcomeRecord>): OutcomeWriteState<TInput> {
	const queryClient = useQueryClient();
	const mutation = useMutation({
		mutationFn,
		// Cache the authoritative answer so a remounted surface reads the saved
		// contract instead of re-asking, and refetch nothing optimistically.
		onSuccess: (outcome) => {
			queryClient.setQueryData(outcomeQueryKey(outcome.id), outcome);
		},
	});

	return {
		pending: mutation.isPending,
		failure: mutation.error ? classifyOutcomeFailure(mutation.error) : undefined,
		reset: () => mutation.reset(),
		/** Runs the write and resolves with the daemon's authoritative Outcome. */
		save: mutation.mutateAsync,
		succeeded: mutation.isSuccess,
	};
}

/**
 * Create the Outcome with its first immutable ContractRevision.
 *
 * The caller supplies the idempotency `requestKey`; replaying the same key
 * after an ambiguous failure returns the original Outcome instead of writing
 * twice, which is why retries must reuse the key rather than mint a new one.
 */
export function useCreateOutcome(projectId: string | undefined) {
	const queryClient = useQueryClient();
	const write = useOutcomeWrite(async (input: CreateOutcomeRequest) => {
		if (usesPreviewWorkspaceData) return createPreviewOutcome(projectId as string, input);
		const { data, error } = await apiClient.POST("/api/v1/projects/{id}/outcomes", {
			params: { path: { id: projectId as string } },
			body: input,
		});
		if (error) throw error;
		return (data as OutcomeEnvelope).outcome;
	});

	return {
		...write,
		save: async (input: CreateOutcomeRequest) => {
			const outcome = await write.save(input);
			// A successful canonical write must stay successful even if the
			// dashboard refresh is temporarily unavailable.
			void queryClient.invalidateQueries({ queryKey: projectOutcomesQueryKey(projectId) });
			return outcome;
		},
	};
}

/**
 * Append the next immutable ContractRevision. `expectedRevision` must name
 * the current revision; a stale pointer comes back as a typed conflict with
 * both revision numbers in the details.
 */
export function useReviseOutcomeContract(outcomeId: string | undefined) {
	const queryClient = useQueryClient();
	const write = useOutcomeWrite(async (input: ReviseOutcomeContractRequest) => {
		if (usesPreviewWorkspaceData) return revisePreviewOutcome(outcomeId as string, input);
		const { data, error } = await apiClient.POST("/api/v1/outcomes/{outcomeId}/revisions", {
			params: { path: { outcomeId: outcomeId as string } },
			body: input,
		});
		if (error) throw error;
		return (data as OutcomeEnvelope).outcome;
	});
	return {
		...write,
		save: async (input: ReviseOutcomeContractRequest) => {
			const outcome = await write.save(input);
			if (usesPreviewWorkspaceData) queryClient.removeQueries({ queryKey: planQueryKey(outcomeId) });
			void queryClient.invalidateQueries({ queryKey: outcomeScheduleQueryKey(outcomeId) });
			return outcome;
		},
	};
}

/* ----------------------------- Plans (#26) ------------------------------ */

export type PlanRecord = components["schemas"]["PlanRevisionResponse"];
type PlanEnvelope = components["schemas"]["PlanEnvelope"];

export const PLAN_NOT_FOUND = "PLAN_NOT_FOUND";
/** The plan binds a superseded contract revision: a fresh brief is required. */
export const PLAN_CONTRACT_STALE = "PLAN_CONTRACT_STALE";
/** Authority layers no longer allow every capability the plan freezes. */
export const PLAN_CAPABILITY_UNAUTHORIZED = "PLAN_CAPABILITY_UNAUTHORIZED";

export function planQueryKey(outcomeId: string | undefined) {
	return ["outcome-plan", outcomeId ?? ""] as const;
}

export function outcomeScheduleQueryKey(outcomeId: string | undefined, planId?: string | undefined) {
	return ["outcome-schedule", outcomeId ?? "", planId ?? ""] as const;
}

async function fetchLatestPlan(outcomeId: string): Promise<PlanRecord> {
	if (usesPreviewWorkspaceData) {
		const plan = getPreviewPlan(outcomeId);
		if (!plan) throw { code: PLAN_NOT_FOUND, message: "No preview plan exists yet." };
		return plan;
	}
	const { data, error } = await apiClient.GET("/api/v1/outcomes/{outcomeId}/plan", {
		params: { path: { outcomeId } },
	});
	if (error) throw error;
	return (data as PlanEnvelope).plan;
}

export interface OutcomePlanQueryResult {
	/** Undefined when no plan exists yet — absence is an answer, not an error. */
	plan?: PlanRecord;
	isLoading: boolean;
	failure?: OutcomeFailure;
	refetch: () => void;
}

export type ScheduleRecord = components["schemas"]["ScheduleResponse"];
type ScheduleEnvelope = components["schemas"]["ScheduleEnvelope"];

export async function fetchOutcomeSchedule(outcomeId: string, planId: string): Promise<ScheduleRecord> {
	if (usesPreviewWorkspaceData) {
		const plan = getPreviewPlan(outcomeId);
		if (!plan || plan.id !== planId) throw { code: PLAN_NOT_FOUND, message: "No preview plan exists yet." };
		const next = plan.workUnits[0]?.id;
		return {
			outcomeId,
			plan,
			workUnits: plan.workUnits.map((workUnit, index) => ({
				workUnit,
				state: index === 0 ? "runnable" : "blocked",
				attempts: [],
				blockingDependencies: index === 0 ? [] : [plan.workUnits[index - 1]?.id ?? ""],
				criterionReady: Object.fromEntries(workUnit.evidenceChecks.map((_, criterionIndex) => [`criterion-${criterionIndex + 1}`, false])),
			})),
			nextRunnableWorkUnitId: next,
		};
	}
	const { data, error } = await apiClient.GET("/api/v1/outcomes/{outcomeId}/plans/{planId}/schedule", {
		params: { path: { outcomeId, planId } },
	});
	if (error) throw error;
	return (data as ScheduleEnvelope).schedule;
}

export function useOutcomeSchedule(outcomeId: string | undefined, planId: string | undefined) {
	const query = useQuery({
		queryKey: outcomeScheduleQueryKey(outcomeId, planId),
		enabled: Boolean(outcomeId && planId),
		queryFn: () => fetchOutcomeSchedule(outcomeId as string, planId as string),
		retry: (attempt, error) => {
			const code = apiErrorCode(error);
			if (code === PLAN_NOT_FOUND || code === PLAN_NOT_APPROVED || code === PLAN_BRIEF_INVALIDATED) return false;
			return attempt < 2;
		},
	});
	return {
		schedule: query.data,
		isLoading: query.isLoading,
		failure: query.error ? classifyOutcomeFailure(query.error) : undefined,
		refetch: () => void query.refetch(),
	};
}

/**
 * The authoritative, read-only Mission WorkUnit graph (F2). This is the sole
 * runtime contract for Canvas/List rendering — never joined with Plan,
 * Schedule, Attempt, session, proof, or Needs You responses to recreate
 * authority. `planRevisionId + topologyFingerprint + topologyGeneration`
 * is the topology identity; `generation` (projection and per-node) is an
 * opaque change token, never a timestamp.
 */
export type MissionRecord = components["schemas"]["ControllersMissionProjectionResponse"];
export type MissionNodeRecord = components["schemas"]["ControllersMissionNodeResponse"];
export type MissionEdgeRecord = components["schemas"]["ControllersMissionEdgeResponse"];
export type MissionAttentionRecord = components["schemas"]["ControllersMissionAttentionResponse"];
export type MissionAttemptRecord = components["schemas"]["ControllersMissionAttemptResponse"];
export type MissionSessionRecord = components["schemas"]["ControllersMissionSessionResponse"];
type MissionEnvelope = components["schemas"]["ControllersMissionEnvelope"];

export function outcomeMissionQueryKey(outcomeId: string | undefined, planId?: string | undefined) {
	return ["outcome-mission", outcomeId ?? "", planId ?? ""] as const;
}

/**
 * Preview mode (browser demo fixtures, no daemon) has no scheduler and no
 * runtime facts to report — only the static Plan is known. It must never
 * invent runnability, blockers, or actions from WorkUnit order: every node's
 * `scheduleState` is the explicit `"unavailable"` sentinel (outside the
 * closed schedule-state set, so the presentation adapter's own fail-closed
 * mapping renders it as "State unavailable" — nothing new to teach it),
 * `criterionReady` is `null` (unresolved, never a fabricated boolean), and no
 * node ever carries a `nextAction` or `currentAttempt`. Only static topology
 * genuinely known from the Plan (title, dependsOn, criterionIds, and the
 * edges they imply) is reflected — the same source the real backend derives
 * its own edges from.
 */
export async function fetchOutcomeMission(outcomeId: string, planId: string): Promise<MissionRecord> {
	if (usesPreviewWorkspaceData) {
		const plan = getPreviewPlan(outcomeId);
		if (!plan || plan.id !== planId) throw { code: PLAN_NOT_FOUND, message: "No preview plan exists yet." };
		const now = new Date().toISOString();
		return {
			version: 1,
			outcomeId,
			missionId: `${outcomeId}:mission`,
			contractRevisionNumber: plan.contractRevisionNumber,
			planRevisionId: plan.id,
			planRevisionNumber: plan.number,
			topologyFingerprint: `preview:${plan.id}:${plan.number}`,
			topologyGeneration: plan.number,
			generation: plan.number,
			updatedAt: now,
			nodes: plan.workUnits.map((workUnit) => ({
				workUnitId: workUnit.id,
				planRevisionId: plan.id,
				title: workUnit.title,
				dependsOn: workUnit.dependsOn,
				scheduleState: "unavailable",
				blockingDependencies: [],
				criterionIds: workUnit.criterionIds,
				criterionReady: null,
				inputs: workUnit.inputs,
				links: [],
				role: workUnit.role,
				responsibility: "unconfirmed",
				updatedAt: now,
				generation: 0,
			})),
			edges: plan.workUnits.flatMap((workUnit) =>
				workUnit.dependsOn.map((dependsOnId) => ({ from: dependsOnId, to: workUnit.id })),
			),
		};
	}
	const { data, error } = await apiClient.GET("/api/v1/outcomes/{outcomeId}/plans/{planId}/mission", {
		params: { path: { outcomeId, planId } },
	});
	if (error) throw error;
	return (data as MissionEnvelope).mission;
}

export interface OutcomeMissionQueryResult {
	mission?: MissionRecord;
	isLoading: boolean;
	isFetching: boolean;
	failure?: OutcomeFailure;
	refetch: () => void;
}

export function useOutcomeMission(outcomeId: string | undefined, planId: string | undefined): OutcomeMissionQueryResult {
	const query = useQuery({
		queryKey: outcomeMissionQueryKey(outcomeId, planId),
		enabled: Boolean(outcomeId && planId),
		queryFn: () => fetchOutcomeMission(outcomeId as string, planId as string),
		retry: (attempt, error) => {
			const code = apiErrorCode(error);
			if (code === OUTCOME_NOT_FOUND || code === PLAN_NOT_FOUND) return false;
			return attempt < 2;
		},
		// The topology protocol (F2 invariant E/F) depends on a stable previous
		// value while a refetch is in flight — never render a transient empty
		// graph as if the mission had none.
		placeholderData: (previous) => previous,
	});
	return {
		mission: query.data,
		isLoading: query.isLoading,
		isFetching: query.isFetching,
		failure: query.error ? classifyOutcomeFailure(query.error) : undefined,
		refetch: () => void query.refetch(),
	};
}

/**
 * The newest plan of any status for one Outcome.
 */
export function useOutcomePlan(outcomeId: string | undefined): OutcomePlanQueryResult {
	const query = useQuery({
		queryKey: planQueryKey(outcomeId),
		enabled: Boolean(outcomeId),
		queryFn: () => fetchLatestPlan(outcomeId as string),
		retry: (attempt, error) => {
			const code = apiErrorCode(error);
			if (code === PLAN_NOT_FOUND || code === OUTCOME_NOT_FOUND) return false;
			return attempt < 2;
		},
	});

	if (query.error) {
		if (apiErrorCode(query.error) === PLAN_NOT_FOUND) {
			return { isLoading: false, refetch: () => void query.refetch() };
		}
		return { isLoading: query.isLoading, failure: classifyOutcomeFailure(query.error), refetch: () => void query.refetch() };
	}
	return { plan: query.data, isLoading: query.isLoading, refetch: () => void query.refetch() };
}

export interface ProposePlanState {
	pending: boolean;
	failure?: OutcomeFailure;
	reset: () => void;
	propose: (input: { expectedContractRevision: number }) => Promise<PlanRecord>;
}

/** Deterministic proposal for one contract revision; replays server-side. */
export function useProposeOutcomePlan(outcomeId: string | undefined): ProposePlanState {
	const queryClient = useQueryClient();
	const mutation = useMutation({
		mutationFn: async (input: { expectedContractRevision: number }) => {
			if (usesPreviewWorkspaceData) {
				return proposePreviewPlan(outcomeId as string, input.expectedContractRevision);
			}
			const { data, error } = await apiClient.POST("/api/v1/outcomes/{outcomeId}/plans", {
				params: { path: { outcomeId: outcomeId as string } },
				body: { expectedContractRevision: input.expectedContractRevision },
			});
			if (error) throw error;
			return (data as PlanEnvelope).plan;
		},
		onSuccess: (plan) => {
			queryClient.setQueryData(planQueryKey(outcomeId), plan);
			void queryClient.invalidateQueries({ queryKey: outcomeScheduleQueryKey(outcomeId) });
			if (usesPreviewWorkspaceData) {
				queryClient.setQueryData(outcomeQueryKey(outcomeId), getPreviewOutcome(outcomeId as string));
				void queryClient.invalidateQueries({ queryKey: ["project-outcomes"] });
			}
		},
	});
	return {
		pending: mutation.isPending,
		failure: mutation.error ? classifyOutcomeFailure(mutation.error) : undefined,
		reset: () => mutation.reset(),
		propose: mutation.mutateAsync,
	};
}

export interface ApprovePlanState {
	pending: boolean;
	failure?: OutcomeFailure;
	reset: () => void;
	approve: (input: { planId: string; expectedContractRevision: number }) => Promise<PlanRecord>;
}

/** Owner approval against the revision the approver was looking at. */
export function useApproveOutcomePlan(outcomeId: string | undefined): ApprovePlanState {
	const queryClient = useQueryClient();
	const mutation = useMutation({
		mutationFn: async (input: { planId: string; expectedContractRevision: number }) => {
			if (usesPreviewWorkspaceData) {
				return approvePreviewPlan(outcomeId as string, input.planId, input.expectedContractRevision);
			}
			const { data, error } = await apiClient.POST(
				"/api/v1/outcomes/{outcomeId}/plans/{planId}/approval",
				{
					params: { path: { outcomeId: outcomeId as string, planId: input.planId } },
					body: { expectedContractRevision: input.expectedContractRevision },
				},
			);
			if (error) throw error;
			return (data as PlanEnvelope).plan;
		},
		onSuccess: (plan) => {
			queryClient.setQueryData(planQueryKey(outcomeId), plan);
			void queryClient.invalidateQueries({ queryKey: outcomeScheduleQueryKey(outcomeId, plan.id) });
			if (usesPreviewWorkspaceData) {
				queryClient.setQueryData(outcomeQueryKey(outcomeId), getPreviewOutcome(outcomeId as string));
				void queryClient.invalidateQueries({ queryKey: ["project-outcomes"] });
			}
		},
	});
	return {
		pending: mutation.isPending,
		failure: mutation.error ? classifyOutcomeFailure(mutation.error) : undefined,
		reset: () => mutation.reset(),
		approve: mutation.mutateAsync,
	};
}

/* --------------------------- Attempts (#31) ----------------------------- */

export type AttemptRecord = components["schemas"]["AttemptResponse"];
export type AttemptPresentationRecord = components["schemas"]["AttemptPresentationResponse"];
export type AttemptObservationRecord = components["schemas"]["AttemptObservationResponse"];
export type RecoveryReceiptRecord = components["schemas"]["RecoveryReceiptResponse"];

type AttemptEnvelope = components["schemas"]["AttemptEnvelope"];
type AttemptListEnvelope = components["schemas"]["AttemptListEnvelope"];
type AttemptRecoveryEnvelope = components["schemas"]["AttemptRecoveryEnvelope"];

/** The daemon's typed refusal for an unknown attempt. */
export const ATTEMPT_NOT_FOUND = "ATTEMPT_NOT_FOUND";
/** Another attempt still holds worktree custody; reconcile before replacing. */
export const ATTEMPT_FENCE_HELD = "ATTEMPT_FENCE_HELD";
export const ATTEMPT_LIVENESS_UNPROVEN = "ATTEMPT_LIVENESS_UNPROVEN";
export const PLAN_NOT_APPROVED = "PLAN_NOT_APPROVED";
export const PLAN_BRIEF_INVALIDATED = "PLAN_BRIEF_INVALIDATED";

// The refusal codes specific to starting an attempt join the shared permanent
// set so a stale surface reports them instead of spinning.
const ATTEMPT_PERMANENT_CODES = new Set([
	...PERMANENT_CODES,
	ATTEMPT_NOT_FOUND,
	PLAN_NOT_APPROVED,
	PLAN_BRIEF_INVALIDATED,
	"OBSERVATION_KIND_REQUIRED",
	"RECOVERY_ACTION_INVALID",
]);

function attemptsQueryKey(outcomeId: string | undefined) {
	return ["outcome-attempts", outcomeId ?? ""] as const;
}

async function fetchOutcomeAttempts(outcomeId: string): Promise<AttemptRecord[]> {
	const { data, error } = await apiClient.GET("/api/v1/outcomes/{outcomeId}/attempts", {
		params: { path: { outcomeId } },
	});
	if (error) throw error;
	return (data as AttemptListEnvelope).attempts;
}

export interface OutcomeAttemptsQueryResult {
	/**
	 * The full attempt lineage in ascending order. Undefined while loading;
	 * an EMPTY array means no attempt has ever been admitted.
	 */
	attempts?: AttemptRecord[];
	isLoading: boolean;
	failure?: OutcomeFailure;
	refetch: () => void;
}

/**
 * Read one Outcome's attempt lineage with its derived presentation. The
 * renderer never derives attempt state locally: `unconfirmed`,
 * `endedUnclassified`, and every next-action hint arrive computed by the
 * daemon from durable facts.
 */
export function useOutcomeAttempts(outcomeId: string | undefined): OutcomeAttemptsQueryResult {
	const query = useQuery({
		queryKey: attemptsQueryKey(outcomeId),
		enabled: Boolean(outcomeId),
		queryFn: () => fetchOutcomeAttempts(outcomeId as string),
		retry: (attempt, error) => {
			const code = apiErrorCode(error);
			if (code && ATTEMPT_PERMANENT_CODES.has(code)) return false;
			return attempt < 2;
		},
	});

	if (query.error) {
		return {
			isLoading: query.isLoading,
			failure: classifyOutcomeFailure(query.error),
			refetch: () => void query.refetch(),
		};
	}
	return {
		attempts: query.data,
		isLoading: query.isLoading,
		refetch: () => void query.refetch(),
	};
}

export interface StartAttemptState {
	pending: boolean;
	failure?: OutcomeFailure;
	reset: () => void;
	/**
	 * Admit the authorized plan. The idempotency key is minted HERE and held
	 * for the retry so an ambiguous network answer replays the same request
	 * instead of admitting twice.
	 */
	start: (input: { planRevisionId: string }) => Promise<AttemptRecord>;
}

export function useStartOutcomeAttempt(outcomeId: string | undefined): StartAttemptState {
	const queryClient = useQueryClient();
	const requestKeyRef = useRef<string | undefined>(undefined);
	const mutation = useMutation({
		mutationFn: async (input: { planRevisionId: string }) => {
			if (!requestKeyRef.current) {
				requestKeyRef.current = crypto.randomUUID();
			}
			const requestKey = requestKeyRef.current;
			const { data, error } = await apiClient.POST("/api/v1/outcomes/{outcomeId}/attempts", {
				params: { path: { outcomeId: outcomeId as string } },
				body: { planRevisionId: input.planRevisionId, requestKey },
			});
			if (error) throw error;
			return (data as AttemptEnvelope).attempt;
		},
		onSuccess: (_attempt, input) => {
			requestKeyRef.current = undefined;
			void queryClient.invalidateQueries({ queryKey: attemptsQueryKey(outcomeId) });
			void queryClient.invalidateQueries({ queryKey: outcomeScheduleQueryKey(outcomeId, input.planRevisionId) });
		},
	});
	return {
		pending: mutation.isPending,
		failure: mutation.error ? classifyOutcomeFailure(mutation.error) : undefined,
		reset: () => mutation.reset(),
		start: (input) => mutation.mutateAsync(input),
	};
}

// Cancel is the only owner action that routes to the provider seam today;
// pause/resume return with a real provider-control contract (ADR 0007).
export type AttemptAction = "cancel";

export interface AttemptActionState {
	pending: boolean;
	failure?: OutcomeFailure;
	reset: () => void;
	act: (attemptId: string, action: AttemptAction) => Promise<AttemptRecord>;
}

/** Owner controls over one active attempt. */
export function useAttemptAction(outcomeId: string | undefined): AttemptActionState {
	const queryClient = useQueryClient();
	const mutation = useMutation({
		mutationFn: async ({ attemptId, action }: { attemptId: string; action: AttemptAction }) => {
			const { data, error } = await apiClient.POST(
				"/api/v1/outcomes/{outcomeId}/attempts/{attemptId}/cancel",
				{
					params: { path: { outcomeId: outcomeId as string, attemptId } },
				},
			);
			if (error) throw error;
			void action; // single-action union today; kept for call-site stability
			return (data as AttemptEnvelope).attempt;
		},
		onSuccess: () => {
			void queryClient.invalidateQueries({ queryKey: attemptsQueryKey(outcomeId) });
			void queryClient.invalidateQueries({ queryKey: outcomeScheduleQueryKey(outcomeId) });
		},
	});
	return {
		pending: mutation.isPending,
		failure: mutation.error ? classifyOutcomeFailure(mutation.error) : undefined,
		reset: () => mutation.reset(),
		act: (attemptId, action) => mutation.mutateAsync({ attemptId, action }),
	};
}

// There is deliberately no "resume": nothing can command a provider to
// resume yet, so the verb does not exist on the wire (ADR 0007 will own real
// provider pause/resume). Reconcile proves an already-running provider alive.
export type AttemptRecoveryAction = "contain" | "reconcile" | "replace" | "attention";

export interface AttemptRecoveryState {
	pending: boolean;
	failure?: OutcomeFailure;
	reset: () => void;
	recover: (
		attemptId: string,
		action: AttemptRecoveryAction,
		options?: { confirmProviderStopped?: boolean },
	) => Promise<{ attempt: AttemptRecord; receipt?: RecoveryReceiptRecord }>;
}

/** Custody-safe recovery verbs over one attempt. */
export function useAttemptRecovery(outcomeId: string | undefined): AttemptRecoveryState {
	const queryClient = useQueryClient();
	const mutation = useMutation({
		mutationFn: async ({
			attemptId,
			action,
			...options
		}: {
			attemptId: string;
			action: AttemptRecoveryAction;
			confirmProviderStopped?: boolean;
		}) => {
			const { data, error } = await apiClient.POST(
				"/api/v1/outcomes/{outcomeId}/attempts/{attemptId}/recovery",
				{
					params: { path: { outcomeId: outcomeId as string, attemptId } },
					body: { action, confirmProviderStopped: options?.confirmProviderStopped ?? false },
				},
			);
			if (error) throw error;
			const envelope = data as AttemptRecoveryEnvelope;
			return { attempt: envelope.attempt, receipt: envelope.receipt };
		},
		onSuccess: () => {
			void queryClient.invalidateQueries({ queryKey: attemptsQueryKey(outcomeId) });
			void queryClient.invalidateQueries({ queryKey: outcomeScheduleQueryKey(outcomeId) });
		},
	});
	return {
		pending: mutation.isPending,
		failure: mutation.error ? classifyOutcomeFailure(mutation.error) : undefined,
		reset: () => mutation.reset(),
		recover: (attemptId, action, options) =>
			mutation.mutateAsync({ attemptId, action, ...options }),
	};
}

export { attemptsQueryKey as outcomeAttemptsQueryKey };

/* ------------------------- Prove & Close (#35) ------------------------- */

export type OutcomeProofRecord = components["schemas"]["OutcomeProofResponse"];
export type RecordEvidenceRequest = components["schemas"]["RecordEvidenceRequest"];
export type RecordVerificationRequest = components["schemas"]["RecordVerificationRequest"];
export type DecideAcceptanceRequest = components["schemas"]["DecideAcceptanceRequest"];

type OutcomeProofEnvelope = components["schemas"]["OutcomeProofEnvelope"];

export function outcomeProofQueryKey(outcomeId: string | undefined) {
	return ["outcome-proof", outcomeId ?? ""] as const;
}

export async function fetchOutcomeProof(outcomeId: string): Promise<OutcomeProofRecord> {
	if (usesPreviewWorkspaceData) {
		throw { code: "OUTCOME_PROOF_UNAVAILABLE", message: "Proof is available only from a running Kennel daemon." };
	}
	const { data, error } = await apiClient.GET("/api/v1/outcomes/{outcomeId}/proof", {
		params: { path: { outcomeId } },
	});
	if (error) throw error;
	return (data as OutcomeProofEnvelope).proof;
}

export function useOutcomeProof(outcomeId: string | undefined) {
	const query = useQuery({
		queryKey: outcomeProofQueryKey(outcomeId),
		enabled: Boolean(outcomeId),
		queryFn: () => fetchOutcomeProof(outcomeId as string),
		retry: (attempt, error) => {
			const code = apiErrorCode(error);
			if (code === OUTCOME_NOT_FOUND || code === "OUTCOME_PROOF_UNAVAILABLE") return false;
			return attempt < 2;
		},
	});
	return {
		proof: query.data,
		isLoading: query.isLoading,
		failure: query.error ? classifyOutcomeFailure(query.error) : undefined,
		refetch: () => void query.refetch(),
	};
}

function useProofMutation<TInput>(
	outcomeId: string | undefined,
	mutationFn: (outcomeId: string, input: TInput) => Promise<OutcomeProofRecord>,
) {
	const queryClient = useQueryClient();
	const mutation = useMutation({
		mutationFn: async (input: TInput) => {
			if (usesPreviewWorkspaceData) {
				throw { code: "OUTCOME_PROOF_UNAVAILABLE", message: "Proof writes require a running Kennel daemon." };
			}
			return mutationFn(outcomeId as string, input);
		},
		onSuccess: (proof) => {
			queryClient.setQueryData(outcomeProofQueryKey(outcomeId), proof);
			void queryClient.invalidateQueries({ queryKey: outcomeScheduleQueryKey(outcomeId) });
		},
	});
	return {
		pending: mutation.isPending,
		failure: mutation.error ? classifyOutcomeFailure(mutation.error) : undefined,
		reset: () => mutation.reset(),
		submit: mutation.mutateAsync,
	};
}

export function useRecordOutcomeEvidence(outcomeId: string | undefined) {
	return useProofMutation(outcomeId, async (id, input: RecordEvidenceRequest) => {
		const { data, error } = await apiClient.POST("/api/v1/outcomes/{outcomeId}/evidence", {
			params: { path: { outcomeId: id } }, body: input,
		});
		if (error) throw error;
		return (data as OutcomeProofEnvelope).proof;
	});
}

export function useRecordOutcomeVerification(outcomeId: string | undefined) {
	return useProofMutation(outcomeId, async (id, input: RecordVerificationRequest) => {
		const { data, error } = await apiClient.POST("/api/v1/outcomes/{outcomeId}/verifications", {
			params: { path: { outcomeId: id } }, body: input,
		});
		if (error) throw error;
		return (data as OutcomeProofEnvelope).proof;
	});
}

export function useDecideOutcomeAcceptance(outcomeId: string | undefined) {
	return useProofMutation(outcomeId, async (id, input: DecideAcceptanceRequest) => {
		const { data, error } = await apiClient.POST("/api/v1/outcomes/{outcomeId}/acceptance-decisions", {
			params: { path: { outcomeId: id } }, body: input,
		});
		if (error) throw error;
		return (data as OutcomeProofEnvelope).proof;
	});
}
