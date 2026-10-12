import { FormEvent, useEffect, useMemo, useRef, useState } from "react";
import { FactList, StateBadge, type StateTone } from "@pin4sf/kennel-product-ui";
import { ArrowUp, Square } from "lucide-react";
import { useTranslation } from "react-i18next";

import {
	planningRequestKey,
	useCancelPlanning,
	useFinalizePlanning,
	usePlanningCandidates,
	usePlanningSession,
	useSendPlanningMessage,
	useStartPlanning,
	type PlanningContextMode,
	type PlanningTurn,
} from "../../hooks/usePlanning";
import { Button } from "../ui/button";

import { PlanningAgentPicker } from "./PlanningAgentPicker";
import { PlanningContextGrant } from "./PlanningContextGrant";

export function MissionPlanningConversation({
	outcomeId,
	contractRevision,
	disabled = false,
}: {
	outcomeId: string;
	contractRevision: number;
	disabled?: boolean;
}) {
	const { t } = useTranslation();
	const candidatesQuery = usePlanningCandidates(outcomeId, contractRevision);
	const planningQuery = usePlanningSession(outcomeId, contractRevision);
	const planning = planningQuery.planning;
	const session = planning?.session;
	const [candidateId, setCandidateId] = useState("");
	const [contextMode, setContextMode] = useState<PlanningContextMode>("repository_read");
	const [editingContext, setEditingContext] = useState(false);
	const [message, setMessage] = useState("");
	const [ignoredSessionId, setIgnoredSessionId] = useState<string>();
	const requestKeys = useRef<Record<string, { fingerprint: string; key: string; expectedRevision?: number }>>({});
	const visiblePlanning = session && session.id === ignoredSessionId ? undefined : planning;
	const activeSession = visiblePlanning?.session;
	const start = useStartPlanning(outcomeId, contractRevision);
	const send = useSendPlanningMessage(outcomeId, contractRevision, activeSession?.id);
	const finalize = useFinalizePlanning(outcomeId, contractRevision, activeSession?.id);
	const cancel = useCancelPlanning(outcomeId, contractRevision, activeSession?.id);
	const pending = start.pending || send.pending || finalize.pending || cancel.pending;
	const conversationPending = send.pending || finalize.pending;
	const readyCandidates = useMemo(() => candidatesQuery.candidates.filter((candidate) => candidate.ready), [candidatesQuery.candidates]);
	const selectedCandidateReady = Boolean(candidateId) && readyCandidates.some((candidate) => candidate.id === candidateId);
	const startDisabled = disabled || !candidateId || !contextMode || pending || Boolean(candidatesQuery.failure) || !selectedCandidateReady;
	let startDisabledReason: string | undefined;
	if (startDisabled && !pending && !candidatesQuery.isLoading) {
		if (disabled) startDisabledReason = t("mission.offline");
		else if (candidatesQuery.failure) startDisabledReason = t("planning.startUnavailable.discoveryFailed");
		else if (readyCandidates.length === 0) startDisabledReason = t("planning.startUnavailable.noReadyAgent");
		else if (!candidateId) startDisabledReason = t("planning.startUnavailable.selectAgent");
		else if (!selectedCandidateReady) startDisabledReason = t("planning.startUnavailable.agentNotReady");
		else if (!contextMode) startDisabledReason = t("planning.startUnavailable.noContext");
	}
	const actionError = start.failure ?? send.failure ?? finalize.failure ?? cancel.failure ?? candidatesQuery.failure ?? planningQuery.failure;
	const failedAction = start.failure ? "start" : send.failure ? "message" : finalize.failure ? "proposal" : cancel.failure ? "cancel" : undefined;
	function beginNewAttempt() {
		if (!failedAction) return;
		delete requestKeys.current[failedAction];
		if (failedAction === "start") start.reset();
		if (failedAction === "message") send.reset();
		if (failedAction === "proposal") finalize.reset();
		if (failedAction === "cancel") cancel.reset();
	}

	function startAnotherPlanningSession() {
		if (!activeSession) return;
		setIgnoredSessionId(activeSession.id);
		setCandidateId("");
		setContextMode("repository_read");
		setEditingContext(false);
		setMessage("");
		delete requestKeys.current.message;
		delete requestKeys.current.proposal;
	}

	useEffect(() => {
		setCandidateId("");
		setContextMode("repository_read");
		setEditingContext(false);
		setMessage("");
		setIgnoredSessionId(undefined);
		requestKeys.current = {};
	}, [outcomeId, contractRevision]);

	// A single admitted candidate is already the owner's configured choice.
	// Select it automatically so the normal path is one clear Start action;
	// multiple candidates still require an explicit choice.
	useEffect(() => {
		if (!activeSession && !candidateId && !candidatesQuery.isLoading && readyCandidates.length === 1) {
			setCandidateId(readyCandidates[0].id);
		}
	}, [activeSession, candidateId, candidatesQuery.isLoading, readyCandidates]);

	function stableRequestKey(action: string, fingerprint: string, expectedRevision?: number) {
		const existing = requestKeys.current[action];
		if (existing?.fingerprint === fingerprint) return existing.key;
		const key = planningRequestKey(`planning-${action}`);
		requestKeys.current[action] = { fingerprint, key, expectedRevision };
		return key;
	}

	function submitMessage(event: FormEvent) {
		event.preventDefault();
		const text = message.trim();
		if (!text || !activeSession || pending || activeSession.waitingOn === "provider") return;
		const requestKey = stableRequestKey("message", `${activeSession.id}:${text}`, activeSession.revision);
		const expectedSessionRevision = requestKeys.current.message?.expectedRevision ?? activeSession.revision;
		void send.mutate({ expectedSessionRevision, text, requestKey })
			.then(() => {
				setMessage("");
				delete requestKeys.current.message;
			})
			.catch(() => undefined);
	}

	return (
		<section className="flex flex-col gap-4 rounded-lg hairline border-wd-hairline bg-wd-surface p-5" data-testid="mission-planning">
			<div className="flex flex-col gap-1.5">
				<h3 className="wd-display-4 text-wd-primary">{t("planning.title")}</h3>
				<p className="text-sm leading-body text-wd-secondary">{t("planning.intro")}</p>
			</div>

			{actionError && <div className="flex flex-wrap items-center gap-2 text-sm text-critical" role="alert"><span>{t("planning.actionFailed", { message: actionError.message })}</span>{failedAction && <Button onClick={beginNewAttempt} size="sm" type="button" variant="secondary">{t("planning.startAnother")}</Button>}{candidatesQuery.failure && <Button onClick={candidatesQuery.refetch} size="sm" type="button" variant="secondary">{t("mission.refresh")}</Button>}{planningQuery.failure && <Button onClick={planningQuery.refetch} size="sm" type="button" variant="secondary">{t("mission.refresh")}</Button>}</div>}
			{planningQuery.isLoading && <p className="text-sm text-wd-secondary">{t("planning.loading")}</p>}
			{!activeSession && !planningQuery.isLoading && !planningQuery.failure && (
				<>
					<PlanningAgentPicker candidates={candidatesQuery.candidates} failure={candidatesQuery.failure?.message} loading={candidatesQuery.isLoading} onRetry={candidatesQuery.refetch} selectedId={candidateId} onChange={setCandidateId} disabled={pending || candidatesQuery.isLoading} />
					<div className="flex items-center gap-2 text-xs text-wd-secondary">
						<span>{t("planning.scope", { scope: contextMode === "repository_read" ? t("planning.repositoryScope") : t("planning.packetScope") })}</span>
						<Button className="h-auto px-1 py-0 text-xs" onClick={() => setEditingContext((current) => !current)} size="sm" type="button" variant="ghost">{t("planning.change")}</Button>
					</div>
					{editingContext && <PlanningContextGrant value={contextMode} onChange={setContextMode} disabled={pending} />}
					<Button
						aria-busy={start.pending || undefined}
						className="self-start"
						data-testid="planning-start"
						disabled={startDisabled}
						onClick={() => {
							if (!candidateId) return;
							void start.mutate({ candidateId, contextMode, requestKey: stableRequestKey("start", `${contractRevision}:${candidateId}:${contextMode}`) })
								.then(() => delete requestKeys.current.start)
								.catch(() => undefined);
						}}
						type="button"
					>
						{t("planning.start")}
					</Button>
					{startDisabledReason && (
						<p className="-mt-2 text-xs text-wd-secondary" data-testid="planning-start-disabled-reason">{startDisabledReason}</p>
					)}
				</>
			)}

			{activeSession && visiblePlanning && (
				<>
					{activeSession.waitingOn === "owner" && activeSession.lastFailureCode && (
						<div className="flex flex-wrap items-center gap-2 text-sm text-critical" role="alert" data-testid="planning-provider-failure">
							<span>{activeSession.lastFailureDetail ?? activeSession.lastFailureCode}</span>
							<Button onClick={startAnotherPlanningSession} size="sm" type="button" variant="secondary">{t("planning.startAnother")}</Button>
							</div>
					)}
					<div className="flex flex-wrap items-center gap-2 text-xs" data-testid="planning-session-status">
						<StateBadge label={t(`planning.status.${activeSession.status}`)} tone={PLANNING_STATUS_TONE[activeSession.status] ?? "neutral"} />
						{activeSession.waitingOn === "provider" && <span className="text-wd-secondary">{t("planning.waiting")}</span>}
					</div>
					<div className="flex items-center gap-2 text-xs text-wd-secondary">
						<span>{t("planning.scope", { scope: activeSession.contextMode === "repository_read" ? t("planning.repositoryScope") : t("planning.packetScope") })}</span>
						<Button className="h-auto px-1 py-0 text-xs" onClick={() => undefined} size="sm" type="button" variant="ghost" disabled>{t("planning.change")}</Button>
					</div>
					<details className="text-xs text-wd-secondary">
						<summary className="cursor-pointer rounded-sm hover:text-wd-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)">{t("planning.details")}</summary>
						<div className="mt-2 flex flex-col gap-1 rounded-md bg-wd-inset p-3">
							<p>{t("planning.grantDigest")}: <span className="break-all tabular-nums text-wd-primary">{activeSession.planningGrantDigest}</span></p>
							<p>{t("planning.binding")}: {activeSession.binding.provider} · {activeSession.binding.modelSelection === "explicit" ? activeSession.binding.model ?? t("planning.explicitModel") : t("planning.providerDefault")}</p>
							{activeSession.effectiveProvider && <p>{t("planning.effective")}: {activeSession.effectiveProvider}{activeSession.effectiveModel ? ` · ${activeSession.effectiveModel}` : ""}</p>}
							<PlanningContextGrant value={activeSession.contextMode} locked suppliedPacketAvailable={activeSession.contextMode === "supplied_packet"} />
						</div>
					</details>
					<div className="flex flex-col gap-4" data-testid="planning-turns">
						{visiblePlanning.turns.map((turn) => <PlanningTurnView key={turn.id} turn={turn} onSuggestion={setMessage} />)}
						{visiblePlanning.turns.length === 0 && <p className="text-sm text-wd-secondary">{activeSession.waitingOn === "owner" ? t("planning.preparing") : t("planning.waiting")}</p>}
					</div>
					{activeSession.status === "active" && (
						<form className="flex flex-col gap-2" onSubmit={submitMessage}>
							{/* Composer contract: inset field with a hairline, the system
							    focus ring, send in ink. */}
							<textarea aria-label={t("planning.messageLabel")} className="min-h-20 w-full resize-none rounded-lg hairline border-wd-hairline bg-wd-inset px-4 py-3 text-sm leading-body text-wd-primary outline-none placeholder:text-wd-tertiary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus) disabled:text-wd-secondary" disabled={pending || Boolean(planningQuery.failure) || activeSession.waitingOn === "provider"} onChange={(event) => setMessage(event.target.value)} placeholder={t("planning.messagePlaceholder")} value={message} />
							<div className="flex flex-wrap items-center gap-2">
								<Button disabled={!message.trim() || conversationPending || cancel.pending || Boolean(planningQuery.failure) || activeSession.waitingOn === "provider"} size="sm" type="submit"><ArrowUp aria-hidden="true" className="size-3.5" /> {t("planning.send")}</Button>
								<Button disabled={conversationPending || cancel.pending || Boolean(planningQuery.failure) || activeSession.waitingOn === "provider"} onClick={() => { const requestKey = stableRequestKey("proposal", activeSession.id, activeSession.revision); const expectedSessionRevision = requestKeys.current.proposal?.expectedRevision ?? activeSession.revision; void finalize.mutate({ expectedSessionRevision, requestKey }).catch(() => undefined); }} size="sm" type="button" variant="secondary">{t("planning.prepare")}</Button>
								<Button disabled={cancel.pending || Boolean(planningQuery.failure)} onClick={() => void cancel.mutate({ expectedSessionRevision: activeSession.revision }).catch(() => undefined)} size="sm" type="button" variant="ghost"><Square aria-hidden="true" className="size-3.5" /> {t("planning.cancel")}</Button>
							</div>
						</form>
					)}
					{activeSession.status === "proposal_ready" && visiblePlanning.proposedPlan && <p className="text-sm text-wd-secondary">{t("planning.proposalReady")}</p>}
					{["cancelled", "superseded", "proposal_ready"].includes(activeSession.status) && (
						<Button className="self-start" onClick={startAnotherPlanningSession} size="sm" type="button" variant="secondary">{t("planning.startAnother")}</Button>
					)}
				</>
			)}
		</section>
	);
}

const PLANNING_STATUS_TONE: Record<string, StateTone> = {
	active: "live",
	proposal_ready: "good",
	superseded: "neutral",
	cancelled: "neutral",
};

/**
 * One turn, per the Message contract: the planner writes on the page under a
 * quiet attribution line; the owner speaks in a right-aligned inset bubble.
 * A clarification's reason and suggestion are inline facts, and its
 * alternatives are reply chips.
 */
function PlanningTurnView({ turn, onSuggestion }: { turn: PlanningTurn; onSuggestion: (suggestion: string) => void }) {
	const { t } = useTranslation();
	if (turn.role !== "planner") {
		return (
			<article className="ml-auto max-w-[85%] rounded-lg bg-wd-inset px-3.5 py-2.5" data-testid={`planning-turn-${turn.sequence}`}>
				<p className="sr-only">{t("planning.owner")}</p>
				<p className="whitespace-pre-wrap text-sm leading-body text-wd-primary">{turn.clarification?.question ?? turn.text}</p>
			</article>
		);
	}
	return (
		<article className="flex max-w-prose flex-col gap-1.5" data-testid={`planning-turn-${turn.sequence}`}>
			<p className="text-xs text-wd-secondary">{t("planning.planner")}</p>
			<p className="whitespace-pre-wrap text-base leading-relaxed text-wd-primary">{turn.clarification?.question ?? turn.text}</p>
			{turn.clarification && (
				<div className="mt-1 flex flex-col gap-2.5 rounded-md bg-wd-inset p-3">
					<FactList
						facts={[
							{ label: t("planning.why"), value: turn.clarification.reason },
							{ label: t("planning.suggested"), value: turn.clarification.recommendation },
						]}
					/>
					{turn.clarification.alternatives?.length ? (
						<div className="flex flex-wrap gap-1.5">
							{turn.clarification.alternatives.map((alternative) => <Button key={alternative} onClick={() => onSuggestion(alternative)} size="sm" type="button" variant="outline">{alternative}</Button>)}
						</div>
					) : null}
				</div>
			)}
		</article>
	);
}
