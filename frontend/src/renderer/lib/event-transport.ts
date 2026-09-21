import type { QueryClient } from "@tanstack/react-query";
import { aoBridge } from "./bridge";
import { getApiBaseUrl, hasTrustedApiBaseUrl, subscribeApiBaseUrl } from "./api-client";
import { setEventsConnectionState } from "./events-connection";
import { workspaceQueryKey } from "../hooks/useWorkspaceQuery";
import { sessionScmSummaryQueryKey } from "../hooks/useSessionScmSummary";
import { conversationQueryKey } from "../hooks/useConversation";
import { agentSwitchesQueryRoot } from "../hooks/useAgentSwitches";
import { sessionUsageQueryRoot } from "../hooks/useSessionUsageSummaries";
import { outcomeScheduleQueryKey, outcomeMissionQueryKey } from "../hooks/useOutcome";

export type EventTransport = {
	connect: () => () => void;
};

const INVALIDATE_DEBOUNCE_MS = 150;
// How long to wait before rebuilding an EventSource the browser gave up on
// (readyState CLOSED — e.g. the daemon answered with a non-SSE response).
const SSE_RETRY_MS = 5_000;
// EventSource.CLOSED, referenced numerically so test stubs without the static
// constants still work.
const EVENTSOURCE_CLOSED = 2;

// CDC event types the daemon pushes over the SSE stream (see
// backend/internal/cdc/event.go). The SSE writer tags each frame with
// `event: <type>`, so named events bypass EventSource.onmessage and must be
// subscribed explicitly. Every one of these can change the project/session list
// the sidebar renders, so they all trigger a (debounced) workspace refetch.
const CDC_EVENT_TYPES = [
	"session_created",
	"session_updated",
	"pr_created",
	"pr_updated",
	"pr_check_recorded",
	"pr_session_changed",
	"pr_review_thread_added",
	"pr_review_thread_resolved",
	"outcome_created",
	"outcome_updated",
	"outcome_contract_revised",
	"outcome_plan_proposed",
	"outcome_plan_approved",
	"outcome_attempt_started",
	"outcome_attempt_updated",
	"outcome_attempt_session_bound",
	"outcome_attempt_observed",
	"outcome_attempt_recovered",
	"outcome_evidence_recorded",
	"outcome_verification_recorded",
	"outcome_acceptance_decided",
	"outcome_correction_recorded",
	"intake_captured",
	"intake_updated",
	"intake_proposal_revised",
	"intake_confirmed",
	"responsibility_link_created",
	"responsibility_link_ended",
	"waldo_conversation_created",
	"waldo_conversation_episode_opened",
	"waldo_conversation_episode_sealed",
	"waldo_conversation_turn_appended",
	"waldo_conversation_context_attached",
	"waldo_conversation_context_detached",
	"waldo_conversation_continuation_prepared",
	"waldo_conversation_continuation_progressed",
	"waldo_conversation_continuation_recorded",
	"outcome_contribution_bound",
	"outcome_decomposition_proposed",
	"outcome_decomposition_authorized",
	"outcome_contribution_dependency_waived",
	"outcome_decomposition_requested",
	"outcome_decomposition_request_answered",
	"project_brief_revised",
 "outcome_run_intent_changed",
 "outcome_attempt_retained",
 "outcome_delivery_changed",
	"harness_pairing_intent_changed",
	"harness_connection_changed",
] as const;

/**
 * Wires live server state into the TanStack Query cache. Two sources feed it:
 *   - daemon lifecycle over Electron IPC (coming up/down changes session availability)
 *   - the backend CDC stream over SSE (project/session/PR changes)
 * Both invalidate the ["workspaces"] query so the UI refetches. Invalidations are
 * debounced because a single user action can emit a burst of CDC events.
 */
export function createEventTransport(queryClient: QueryClient): EventTransport {
	return {
		connect() {
			let debounce: ReturnType<typeof setTimeout> | undefined;
			const pendingConversationSessions = new Set<string>();
			const pendingOutcomeSchedules = new Set<readonly [string, string, string]>();
			let allOutcomeSchedulesInvalidationPending = false;
			let workspaceInvalidationPending = false;
			let outcomeFactsInvalidationPending = false;
			let retryTimer: ReturnType<typeof setTimeout> | undefined;
			let source: EventSource | undefined;
			let sourceBaseUrl: string | undefined;
			const refreshWorkspaces = (event?: Event) => {
				let conversationOnly = false;
				const eventType = event?.type ?? "";
				if (!event || eventType.startsWith("outcome_")) outcomeFactsInvalidationPending = true;
				if (event && "data" in event) {
					try {
						const decoded = JSON.parse(String((event as MessageEvent).data)) as {
							sessionId?: unknown;
							outcomeId?: unknown;
							planId?: unknown;
							planRevisionId?: unknown;
							payload?: unknown;
						};
						// The SSE endpoint sends the complete durable CDC event. Routing
						// fields such as sessionId live on that envelope, while trigger-built
						// details such as conversationId live inside its payload. Do not
						// mistake the payload for the entire event: doing so refreshes the
						// sidebar but leaves a Chat timeline frozen on its pre-turn snapshot.
						const payload =
							typeof decoded.payload === "object" && decoded.payload !== null
								? (decoded.payload as { conversationId?: unknown })
								: undefined;
						if (
							typeof decoded.sessionId === "string" &&
							decoded.sessionId &&
							typeof payload?.conversationId === "string" &&
							payload.conversationId
						) {
							pendingConversationSessions.add(decoded.sessionId);
							conversationOnly = true;
						}
						if (eventType.startsWith("outcome_")) {
							const outcomeId =
								typeof decoded.outcomeId === "string"
									? decoded.outcomeId
									: typeof (payload as { outcomeId?: unknown } | undefined)?.outcomeId === "string"
										? ((payload as { outcomeId: string }).outcomeId)
										: "";
							const planId =
								typeof decoded.planId === "string"
									? decoded.planId
									: typeof decoded.planRevisionId === "string"
										? decoded.planRevisionId
										: typeof (payload as { planId?: unknown } | undefined)?.planId === "string"
											? ((payload as { planId: string }).planId)
											: typeof (payload as { planRevisionId?: unknown } | undefined)?.planRevisionId === "string"
												? ((payload as { planRevisionId: string }).planRevisionId)
												: "";
							if (outcomeId && planId) {
								pendingOutcomeSchedules.add(outcomeScheduleQueryKey(outcomeId, planId));
								pendingOutcomeSchedules.add(outcomeMissionQueryKey(outcomeId, planId));
							} else if (outcomeId) {
								pendingOutcomeSchedules.add(outcomeScheduleQueryKey(outcomeId));
								pendingOutcomeSchedules.add(outcomeMissionQueryKey(outcomeId));
							} else {
								allOutcomeSchedulesInvalidationPending = true;
							}
						}
					} catch {
						if (eventType.startsWith("outcome_")) allOutcomeSchedulesInvalidationPending = true;
						// A malformed CDC payload still invalidates workspaces; it simply
						// cannot target a conversation cache precisely.
					}
				}
				if (!conversationOnly) workspaceInvalidationPending = true;
				if (debounce) clearTimeout(debounce);
				debounce = setTimeout(() => {
					if (outcomeFactsInvalidationPending) {
						// A connected stream does not make cached responsibility facts current.
						// Refresh the Mission and portfolio together after CDC or a reconnect gap.
						for (const root of ["project-outcomes", "outcome", "outcome-plan", "outcome-attempts", "outcome-proof", "outcome-schedule", "outcome-mission", "outcome-run-state", "project-run-states", "outcome-planning-session", "outcome-planning-candidates"]) {
							void queryClient.invalidateQueries({ queryKey: [root] });
						}
						outcomeFactsInvalidationPending = false;
					}
					if (workspaceInvalidationPending) {
						void queryClient.invalidateQueries({ queryKey: workspaceQueryKey });
						void queryClient.invalidateQueries({ queryKey: agentSwitchesQueryRoot });
						void queryClient.invalidateQueries({ queryKey: sessionScmSummaryQueryKey() });
						void queryClient.invalidateQueries({ queryKey: sessionUsageQueryRoot });
						workspaceInvalidationPending = false;
					}
					if (allOutcomeSchedulesInvalidationPending) {
						void queryClient.invalidateQueries({ queryKey: ["outcome-schedule"] });
						void queryClient.invalidateQueries({ queryKey: ["outcome-mission"] });
						void queryClient.invalidateQueries({ queryKey: ["outcome-run-state"] });
						void queryClient.invalidateQueries({ queryKey: ["project-run-states"] });
						allOutcomeSchedulesInvalidationPending = false;
					}
					for (const queryKey of pendingOutcomeSchedules) {
						void queryClient.invalidateQueries({ queryKey });
					}
					pendingOutcomeSchedules.clear();
					for (const sessionId of pendingConversationSessions) {
						void queryClient.invalidateQueries({ queryKey: conversationQueryKey(sessionId) });
					}
					pendingConversationSessions.clear();
				}, INVALIDATE_DEBOUNCE_MS);
			};

			const scheduleRetry = () => {
				if (retryTimer) return;
				retryTimer = setTimeout(() => {
					retryTimer = undefined;
					connectSource();
				}, SSE_RETRY_MS);
			};

			const connectSource = () => {
				// EventSource is unavailable in jsdom (tests) and some preview surfaces; guard it.
				if (typeof EventSource === "undefined") return;
				if (!hasTrustedApiBaseUrl()) {
					source?.close();
					source = undefined;
					sourceBaseUrl = undefined;
					setEventsConnectionState("disconnected");
					return;
				}
				const baseUrl = getApiBaseUrl();
				// Keep a still-usable source on the same base URL; replace one the
				// browser abandoned (CLOSED) or one bound to a stale port.
				if (source && sourceBaseUrl === baseUrl && source.readyState !== EVENTSOURCE_CLOSED) return;
				source?.close();
				source = undefined;
				sourceBaseUrl = baseUrl;
				try {
					source = new EventSource(`${baseUrl.replace(/\/+$/, "")}/api/v1/events`);
					source.onopen = () => {
						setEventsConnectionState("connected");
						// Events emitted during the gap were lost; refetch once on (re)open.
						refreshWorkspaces();
					};
					source.onerror = () => {
						// While readyState is CONNECTING the browser retries on its own;
						// either way the stream is not delivering, so surface it instead
						// of looping silently against a dead daemon.
						setEventsConnectionState("disconnected");
						if (source?.readyState === EVENTSOURCE_CLOSED) scheduleRetry();
					};
					source.onmessage = refreshWorkspaces; // unnamed events, if any
					for (const type of CDC_EVENT_TYPES) {
						source.addEventListener(type, refreshWorkspaces);
					}
					// EventSource auto-reconnects and resumes via Last-Event-ID while
					// CONNECTING; scheduleRetry only covers the terminal CLOSED state.
				} catch {
					source = undefined;
				}
			};

			const removeDaemonListener = aoBridge.daemon.onStatus(() => {
				connectSource();
				refreshWorkspaces();
			});
			// Rebind when the daemon comes back on a different port, independent of
			// status-event ordering.
			const removeBaseUrlListener = subscribeApiBaseUrl(connectSource);
			connectSource();

			return () => {
				if (debounce) clearTimeout(debounce);
				if (retryTimer) clearTimeout(retryTimer);
				removeDaemonListener();
				removeBaseUrlListener();
				source?.close();
				setEventsConnectionState("idle");
			};
		},
	};
}
