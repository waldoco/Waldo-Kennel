import { FormEvent, useEffect, useMemo, useRef, useState } from "react";
import { Loader2, Send, Square } from "lucide-react";
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
import { Badge } from "../ui/badge";
import { ContractChangeProposalCard } from "./ContractChangeProposalCard";
import { PlanningAgentPicker } from "./PlanningAgentPicker";
import { PlanningContextGrant } from "./PlanningContextGrant";

export function MissionPlanningConversation({
	outcomeId,
	contractRevision,
	onReviewContract,
}: {
	outcomeId: string;
	contractRevision: number;
	onReviewContract?: () => void;
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
	const startDisabled = !candidateId || !contextMode || pending || Boolean(candidatesQuery.failure) || !selectedCandidateReady;
	let startDisabledReason: string | undefined;
	if (startDisabled && !pending && !candidatesQuery.isLoading) {
		if (candidatesQuery.failure) startDisabledReason = t("planning.startUnavailable.discoveryFailed");
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
		<section className="flex flex-col gap-4 rounded-group hairline border-border bg-card px-4.5 py-3.5" data-testid="mission-planning">
			<div>
			<h3 className="text-sm font-medium">{t("planning.title")}</h3>
			<p className="mt-1 text-xs text-muted-foreground">{t("planning.intro")}</p>
		</div>

			{actionError && <div className="flex flex-wrap items-center gap-2 text-sm text-destructive" role="alert"><span>{t("planning.actionFailed", { message: actionError.message })}</span>{failedAction && <Button onClick={beginNewAttempt} size="sm" type="button" variant="outline">{t("planning.startAnother")}</Button>}{candidatesQuery.failure && <Button onClick={candidatesQuery.refetch} size="sm" type="button" variant="outline">{t("mission.refresh")}</Button>}{planningQuery.failure && <Button onClick={planningQuery.refetch} size="sm" type="button" variant="outline">{t("mission.refresh")}</Button>}</div>}
			{planningQuery.isLoading && <p className="text-xs text-muted-foreground">{t("planning.loading")}</p>}
			{!activeSession && !planningQuery.isLoading && !planningQuery.failure && (
				<>
					<PlanningAgentPicker candidates={candidatesQuery.candidates} failure={candidatesQuery.failure?.message} loading={candidatesQuery.isLoading} onRetry={candidatesQuery.refetch} selectedId={candidateId} onChange={setCandidateId} disabled={pending || candidatesQuery.isLoading} />
					<div className="flex items-center gap-2 text-xs text-muted-foreground">
						<span>{t("planning.scope", { scope: contextMode === "repository_read" ? t("planning.repositoryScope") : t("planning.packetScope") })}</span>
						<Button className="h-auto px-1 py-0 text-xs" onClick={() => setEditingContext((current) => !current)} size="sm" type="button" variant="ghost">{t("planning.change")}</Button>
					</div>
					{editingContext && <PlanningContextGrant value={contextMode} onChange={setContextMode} disabled={pending} />}
					<Button
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
						{start.pending && <Loader2 aria-hidden="true" className="size-3.5 animate-spin" />}
						{t("planning.start")}
					</Button>
					{startDisabledReason && (
						<p className="text-xs text-muted-foreground" data-testid="planning-start-disabled-reason">{startDisabledReason}</p>
					)}
				</>
			)}

			{activeSession && visiblePlanning && (
				<>
					{activeSession.waitingOn === "owner" && activeSession.lastFailureCode && (
						<div className="flex flex-wrap items-center gap-2 text-sm text-destructive" role="alert" data-testid="planning-provider-failure">
							<span>{activeSession.lastFailureDetail ?? activeSession.lastFailureCode}</span>
							<Button onClick={startAnotherPlanningSession} size="sm" type="button" variant="outline">{t("planning.startAnother")}</Button>
						</div>
					)}
					<div className="flex flex-wrap items-center gap-2 text-xs" data-testid="planning-session-status">
						<Badge variant={activeSession.status === "proposal_ready" ? "success" : "accent"}>{t(`planning.status.${activeSession.status}`)}</Badge>
						{activeSession.waitingOn === "provider" && <span className="text-muted-foreground">{t("planning.waiting")}</span>}
					</div>
					<div className="flex items-center gap-2 text-xs text-muted-foreground">
						<span>{t("planning.scope", { scope: activeSession.contextMode === "repository_read" ? t("planning.repositoryScope") : t("planning.packetScope") })}</span>
						<Button className="h-auto px-1 py-0 text-xs" onClick={() => undefined} size="sm" type="button" variant="ghost" disabled>{t("planning.change")}</Button>
					</div>
					<details className="text-xs text-muted-foreground">
						<summary className="cursor-pointer">{t("planning.details")}</summary>
						<div className="mt-2 flex flex-col gap-1 rounded-md border border-border bg-background/40 px-3 py-2">
							<p>{t("planning.grantDigest")}: <code>{activeSession.planningGrantDigest}</code></p>
							<p>{t("planning.binding")}: {activeSession.binding.provider} · {activeSession.binding.modelSelection === "explicit" ? activeSession.binding.model ?? t("planning.explicitModel") : t("planning.providerDefault")}</p>
							{activeSession.effectiveProvider && <p>{t("planning.effective")}: {activeSession.effectiveProvider}{activeSession.effectiveModel ? ` · ${activeSession.effectiveModel}` : ""}</p>}
							<PlanningContextGrant value={activeSession.contextMode} locked suppliedPacketAvailable={activeSession.contextMode === "supplied_packet"} />
						</div>
					</details>
					<div className="flex flex-col gap-2" data-testid="planning-turns">
						{visiblePlanning.turns.map((turn) => <PlanningTurnView key={turn.id} turn={turn} onReviewContract={onReviewContract} onSuggestion={setMessage} />)}
						{visiblePlanning.turns.length === 0 && <p className="text-sm text-muted-foreground">{activeSession.waitingOn === "owner" ? t("planning.preparing") : t("planning.waiting")}</p>}
					</div>
					{activeSession.status === "active" && (
						<form className="flex flex-col gap-2" onSubmit={submitMessage}>
							<textarea aria-label={t("planning.messageLabel")} className="min-h-20 w-full rounded-md border border-border bg-background p-2 text-sm" disabled={pending || Boolean(planningQuery.failure) || activeSession.waitingOn === "provider"} onChange={(event) => setMessage(event.target.value)} placeholder={t("planning.messagePlaceholder")} value={message} />
							<div className="flex flex-wrap gap-2">
								<Button disabled={!message.trim() || conversationPending || cancel.pending || Boolean(planningQuery.failure) || activeSession.waitingOn === "provider"} size="sm" type="submit"><Send aria-hidden="true" className="size-3.5" /> {t("planning.send")}</Button>
								<Button disabled={conversationPending || cancel.pending || Boolean(planningQuery.failure) || activeSession.waitingOn === "provider"} onClick={() => { const requestKey = stableRequestKey("proposal", activeSession.id, activeSession.revision); const expectedSessionRevision = requestKeys.current.proposal?.expectedRevision ?? activeSession.revision; void finalize.mutate({ expectedSessionRevision, requestKey }).catch(() => undefined); }} size="sm" type="button" variant="secondary">{t("planning.prepare")}</Button>
								<Button disabled={cancel.pending || Boolean(planningQuery.failure)} onClick={() => void cancel.mutate({ expectedSessionRevision: activeSession.revision }).catch(() => undefined)} size="sm" type="button" variant="ghost"><Square aria-hidden="true" className="size-3.5" /> {t("planning.cancel")}</Button>
							</div>
						</form>
					)}
					{activeSession.status === "proposal_ready" && visiblePlanning.proposedPlan && <p className="text-sm text-muted-foreground">{t("planning.proposalReady")}</p>}
					{["cancelled", "superseded", "proposal_ready"].includes(activeSession.status) && (
						<Button onClick={startAnotherPlanningSession} size="sm" type="button" variant="outline">{t("planning.startAnother")}</Button>
					)}
				</>
			)}
		</section>
	);
}

function PlanningTurnView({ turn, onReviewContract, onSuggestion }: { turn: PlanningTurn; onReviewContract?: () => void; onSuggestion: (suggestion: string) => void }) {
	const { t } = useTranslation();
	return (
		<article className={`rounded-md border px-3 py-2 ${turn.role === "planner" ? "border-accent/40 bg-accent/5" : "border-border"}`} data-testid={`planning-turn-${turn.sequence}`}>
			<p className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">{turn.role === "planner" ? t("planning.planner") : t("planning.owner")}</p>
			<p className="mt-1 whitespace-pre-wrap text-sm">{turn.clarification?.question ?? turn.text}</p>
			{turn.clarification && (
				<div className="mt-2 flex flex-col gap-2 rounded-md bg-background/60 p-2 text-xs">
					<p><span className="font-medium">{t("planning.why")}:</span> {turn.clarification.reason}</p>
					{turn.clarification.recommendation && <p><span className="font-medium">{t("planning.suggested")}:</span> {turn.clarification.recommendation}</p>}
					<div className="flex flex-wrap gap-1.5">
						{turn.clarification.alternatives.map((alternative) => <Button key={alternative} onClick={() => onSuggestion(alternative)} size="sm" type="button" variant="outline">{alternative}</Button>)}
					</div>
				</div>
			)}
			{turn.contractChange && <ContractChangeProposalCard onReviewContract={onReviewContract} turn={turn} />}
		</article>
	);
}
