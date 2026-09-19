import { OutcomeRunControls } from "./OutcomeRunControls";
import { ShieldAlert } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import type { MessageKey } from "../../i18n/messages";
import {
	useAttemptAction,
	useAttemptRecovery,
	useOutcomeAttempts,
	useOutcomeMission,
	useOutcomePlan,
	useOutcomeProof,
	useOutcomeSchedule,
	useStartOutcomeAttempt,
	type AttemptRecord,
} from "../../hooks/useOutcome";
import { useUiStore } from "../../stores/ui-store";
import { MissionPlanView } from "./MissionPlanView";
import { selectMissionCanvasRenderer } from "../../lib/mission-canvas-config";
import { MissionCanvas } from "./MissionCanvas";
import { MissionViewSwitch } from "./MissionViewSwitch";
import { MissionWorkUnitList } from "./MissionWorkUnitList";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { newestAttempt } from "./OutcomeRunBoardAdapters";

type OutcomeRunSurfaceProps = {
	outcomeId: string;
	/** Stale authority or reconnect uncertainty blocks admission, never containment. */
	admissionBlocked?: boolean;
	onReviewProof?: () => void;
};

/**
 * The daemon's derived phase vocabulary, mirrored as constants so controls
 * never key off scattered literals. Source of truth: AttemptPresentationResponse.
 */
export { ATTEMPT_PHASES } from "./attemptPhases";
import { ATTEMPT_PHASES } from "./attemptPhases";

const STATUS_BADGE_KEYS: Record<string, MessageKey> = {
	queued: "outcome.run.badgeQueued",
	running: "outcome.run.badgeRunning",
	paused: "outcome.run.badgePaused",
	succeeded: "outcome.run.badgeSucceeded",
	failed: "outcome.run.badgeFailed",
	cancelled: "outcome.run.badgeCancelled",
	lost: "outcome.run.badgeLost",
	reconciled: "outcome.run.badgeReconciled",
};

function statusBadgeKey(status: string): MessageKey | undefined {
	return STATUS_BADGE_KEYS[status];
}

/**
 * Act & Observe (#31): one truthful run surface over the daemon's attempt
 * lineage. The renderer derives NOTHING here — stored status, derived
 * presentation (including `unconfirmed` and ended-unclassified), and next
 * actions all arrive computed by the daemon from durable facts. Provider
 * completion is never presented as success, transcripts are never read, and
 * no provider name is treated as a policy.
 */
export function OutcomeRunSurface({ outcomeId, onReviewProof, admissionBlocked = false }: OutcomeRunSurfaceProps) {
	const missionViewMode = useUiStore((state) => state.missionViewMode);
	const setMissionViewMode = useUiStore((state) => state.setMissionViewMode);
	const { t } = useTranslation();
	const planQuery = useOutcomePlan(outcomeId);
	const attemptsQuery = useOutcomeAttempts(outcomeId);
	const action = useAttemptAction(outcomeId);
	const recovery = useAttemptRecovery(outcomeId);

	const pending = action.pending || recovery.pending;
	const plan = planQuery.plan;
	const planApproved = plan?.status === "approved";
	const scheduleQuery = useOutcomeSchedule(outcomeId, planApproved ? plan?.id : undefined);
	const schedule = scheduleQuery.schedule;
	const missionQuery = useOutcomeMission(outcomeId, planApproved ? plan?.id : undefined);
	const startAttempt = useStartOutcomeAttempt(outcomeId);
	const proofQuery = useOutcomeProof(outcomeId);
	const criterionText = useCallback(
		(criterionId: string) =>
			proofQuery.proof?.criteria.find((criterion) => criterion.criterionId === criterionId)?.text,
		[proofQuery.proof],
	);
	const failure = action.failure ?? recovery.failure ?? attemptsQuery.failure ?? scheduleQuery.failure;
	const attempts = attemptsQuery.attempts ?? [];
	const current = newestAttempt(attempts);

	// The Mission WorkUnit graph carries no workUnitId-scoped start endpoint —
	// `POST /attempts` only ever takes `planRevisionId` and the daemon itself
	// picks the next runnable WorkUnit, which is exactly the node the
	// projection already marked with the returned `start` action (invariant
	// G: only the server-selected action is ever actionable).
	const startNextRunnable = useCallback(() => {
		if (!plan || startAttempt.pending) return;
		void startAttempt.start({ planRevisionId: plan.id });
	}, [plan, startAttempt]);

	// Instruct (Board) / Choose / Engage (List) drills into the current
	// attempt's real bound session in a terminal panel. The panel itself is
	// docked by WorkShell (components/outcome/WorkShell.tsx) beside every
	// Work stage — not just this one — so its open state lives in the shared
	// ui-store slice rather than local state, and the same terminal-toggle
	// button in the persistent top bar opens the exact same panel.
	const openAttemptPanel = useUiStore((state) => state.openOutcomeAttemptPanel);
	const closeAttemptPanel = useUiStore((state) => state.closeOutcomeAttemptPanel);
	useEffect(() => {
		closeAttemptPanel();
	}, [current?.id, closeAttemptPanel]);
	const engageAttempt = (attempt: AttemptRecord) => openAttemptPanel(attempt.id);


	async function act(actionName: "cancel") {
		if (!current || pending) return;
		try {
			await action.act(current.id, actionName);
		} catch {
			// Failure state derives from the mutation's typed error.
		}
	}

	async function recover(
		actionName: "contain" | "reconcile" | "replace" | "attention",
		confirmStopped = false,
	) {
		if (!current || pending) return;
		try {
			await recovery.recover(current.id, actionName, { confirmProviderStopped: confirmStopped });
		} catch {
			// Failure state derives from the mutation's typed error.
		}
	}

	return (
		<div className="flex h-full min-h-0 flex-col gap-5" data-testid="outcome-run-surface">
			<div className="max-w-xl">
				<h2 className="text-base font-medium">{t("outcome.run.heading")}</h2>
				<p className="text-muted-foreground text-sm">{t("outcome.run.intro")}</p>
			</div>

			{!planApproved && !planQuery.isLoading && (
				<div className="max-w-xl rounded-group hairline border-border bg-card px-4.5 py-3.5" data-testid="outcome-run-needs-plan">
					<h3 className="text-sm font-medium">{t("outcome.run.needsPlanTitle")}</h3>
					<p className="mt-1 text-muted-foreground text-sm">{t("outcome.run.needsPlanBody")}</p>
				</div>
			)}

			<OutcomeRunControls outcomeId={outcomeId} admissionBlocked={admissionBlocked} />

			{/* The daemon-derived execution graph. Every state, blocker and
			    reason here is a schedule fact; this only renders it. */}
			{planApproved && schedule && (
				<section className="max-w-2xl rounded-group hairline border-border bg-card px-4.5 py-3.5" data-testid="outcome-run-schedule">
					<MissionPlanView
						attempts={attempts}
						changes={proofQuery.proof?.result?.changes}
						criterionText={criterionText}
						graphOnly
						onOpenAttempt={engageAttempt}
						onReviewProof={onReviewProof}
							schedule={schedule}
						workUnits={schedule.workUnits.map((entry) => entry.workUnit)}
					/>
				</section>
			)}

			{/* The Mission WorkUnit graph (F2): the authenticated `/mission`
			    projection only, never joined with Plan/Schedule/Attempt/session
			    responses to recreate its authority. List and Canvas render the
			    same projection through the shared WorkUnit face; the
			    remembered List/Graph preference feeds the mission-canvas config
			    seam; List is the default and remains the complete reading until
			    the cutover slice. */}
			<div className="flex items-center justify-between gap-2">
				<h3 className="text-sm font-medium text-foreground">{t("mission.list.heading")}</h3>
				<MissionViewSwitch
					labels={{
						ariaLabel: t("mission.view.switchAria" satisfies MessageKey),
						graph: t("mission.view.graph" satisfies MessageKey),
						list: t("mission.view.list" satisfies MessageKey),
					}}
					onChange={setMissionViewMode}
					value={missionViewMode}
				/>
			</div>
			<div className="min-h-0 flex-1">
				{selectMissionCanvasRenderer({ renderer: missionViewMode === "graph" ? "flow" : "list" }) === "flow" ? (
					<MissionCanvas missionQuery={missionQuery} planApproved={planApproved} planWorkUnits={plan?.workUnits} />
				) : (
					<MissionWorkUnitList missionQuery={missionQuery} onStart={startNextRunnable} planApproved={planApproved} planWorkUnits={plan?.workUnits} />
				)}
			</div>

			{current && <CurrentAttemptCard
				attempt={current}
				onAct={(name) => void act(name)}
				onRecover={(name, confirmStopped) => void recover(name, confirmStopped)}
				pending={pending}
			/>}

			{current && onReviewProof && (
				<div className="max-w-xl rounded-group hairline border-border bg-card px-4.5 py-3.5" data-testid="outcome-run-proof-card">
					<h3 className="text-sm font-medium">{t("outcome.run.proofTitle")}</h3>
					<p className="mt-1 text-muted-foreground text-sm">{t("outcome.run.proofBody")}</p>
					<Button className="mt-3" data-testid="outcome-run-review-proof" onClick={onReviewProof} variant="outline">
						{t("outcome.run.proofCta")}
					</Button>
				</div>
			)}

			{failure && (
				<div
					className="max-w-xl rounded-group hairline border-warning/40 bg-warning/5 px-4.5 py-3.5"
					data-testid="outcome-run-failure"
				>
					<h3 className="text-sm font-medium">{t("outcome.run.errorTitle")}</h3>
					<p className="mt-1 text-muted-foreground text-sm">{failure.message}</p>
				</div>
			)}
		</div>
	);
}

type CurrentAttemptCardProps = {
	attempt: AttemptRecord;
	pending: boolean;
	onAct: (action: "cancel") => void;
	onRecover: (action: "contain" | "reconcile" | "replace" | "attention", confirmStopped: boolean) => void;
};

function CurrentAttemptCard({
	attempt,
	pending,
	onAct,
	onRecover,
}: CurrentAttemptCardProps) {
	const { t } = useTranslation();
	const badgeKey = statusBadgeKey(attempt.status);
	const phase = attempt.presentation.phase;
	// The owner-containment assertion is armed in two steps on purpose: the
	// first click only reveals WHAT will be asserted and what it costs.
	const [ownerStopArmed, setOwnerStopArmed] = useState(false);

	return (
		<div
			className="flex max-w-xl flex-col gap-3 rounded-card hairline border-border bg-card p-4.5"
			data-testid={`outcome-run-attempt-${attempt.id}`}
		>
			<div className="flex items-center gap-2">
				{badgeKey && (
					<Badge data-testid="outcome-run-status" variant="outline">
						{t(badgeKey, { number: attempt.number })}
					</Badge>
				)}
				{phase === ATTEMPT_PHASES.executing && (
					<span aria-hidden="true" className="size-dot-sm shrink-0 rounded-full bg-status-working animate-status-pulse" />
				)}
				<span aria-live="polite" className="text-muted-foreground text-xs" data-testid="outcome-run-next-action">
					{attempt.presentation.nextAction}
				</span>
			</div>

			{phase === ATTEMPT_PHASES.needsInput && (
				<section
					className="rounded-md hairline border-warning/40 bg-warning/5 p-3"
					data-testid="outcome-run-needs-input"
				>
					<h3 className="flex items-center gap-1.5 text-sm font-medium">
						<ShieldAlert aria-hidden="true" className="size-3.5" />
						{attempt.presentation.attention === "blocked"
							? t("outcome.run.blockedTitle")
							: t("outcome.run.waitingInputTitle")}
					</h3>
					<p className="mt-1 text-muted-foreground text-sm">
						{attempt.presentation.attention === "blocked"
							? t("outcome.run.blockedBody")
							: t("outcome.run.waitingInputBody")}
					</p>
				</section>
			)}

			{phase === ATTEMPT_PHASES.unconfirmed && (
				<section
					className="rounded-md hairline border-warning/40 bg-warning/5 p-3"
					data-testid="outcome-run-needs-you"
				>
					<h3 className="flex items-center gap-1.5 text-sm font-medium">
						<ShieldAlert aria-hidden="true" className="size-3.5" />
						{t("outcome.run.unconfirmedTitle")}
					</h3>
					<p className="mt-1 text-muted-foreground text-sm">{t("outcome.run.unconfirmedBody")}</p>
					<div className="mt-2 flex gap-2">
						<Button
							data-testid="outcome-run-contain"
							disabled={pending}
							onClick={() => onRecover("contain", false)}
							size="sm"
							variant="outline"
						>
							{t("outcome.run.ctaContain")}
						</Button>
						<Button
							data-testid="outcome-run-reconcile"
							disabled={pending}
							onClick={() => onRecover("reconcile", false)}
							size="sm"
						>
							{t("outcome.run.ctaReconcile")}
						</Button>
						{!ownerStopArmed && (
							<Button
								data-testid="outcome-run-owner-stop"
								disabled={pending}
								onClick={() => setOwnerStopArmed(true)}
								size="sm"
								variant="outline"
							>
								{t("outcome.run.ownerStopCta")}
							</Button>
						)}
					</div>
					{ownerStopArmed && (
						<div
							className="mt-3 rounded-md hairline border-destructive/40 bg-destructive/5 p-3"
							data-testid="outcome-run-owner-stop-confirm"
						>
							<h4 className="text-sm font-medium">{t("outcome.run.ownerStopTitle")}</h4>
							<p className="mt-1 text-muted-foreground text-sm">{t("outcome.run.ownerStopBody")}</p>
							<div className="mt-2 flex flex-wrap gap-2">
								<Button
									data-testid="outcome-run-owner-stop-assert"
									disabled={pending}
									onClick={() => onRecover("reconcile", true)}
									size="sm"
								>
									{t("outcome.run.ownerStopAssertCta")}
								</Button>
								<Button
									data-testid="outcome-run-owner-stop-back"
									disabled={pending}
									onClick={() => setOwnerStopArmed(false)}
									size="sm"
									variant="outline"
								>
									{t("outcome.run.ownerStopBackCta")}
								</Button>
							</div>
						</div>
					)}
				</section>
			)}

			{phase === ATTEMPT_PHASES.endedUnclassified && (
				<section
					className="rounded-md hairline border-border p-3"
					data-testid="outcome-run-ended-unclassified"
				>
					<h3 className="text-sm font-medium">{t("outcome.run.unclassifiedTitle")}</h3>
					<p className="mt-1 text-muted-foreground text-sm">{t("outcome.run.unclassifiedBody")}</p>
					{attempt.status === "running" && (
						<Button
							className="mt-2"
							data-testid="outcome-run-reconcile"
							disabled={pending}
							onClick={() => onRecover("reconcile", false)}
							size="sm"
						>
							{t("outcome.run.ctaReconcile")}
						</Button>
					)}
					{attempt.status !== "running" && (
						<Button
							className="mt-2"
							data-testid="outcome-run-replace"
							disabled={pending}
							onClick={() => onRecover("replace", false)}
							size="sm"
						>
							{t("outcome.run.ctaReplace")}
						</Button>
					)}
				</section>
			)}

			{(phase === ATTEMPT_PHASES.haltedFailed || phase === ATTEMPT_PHASES.suspectLost || phase === ATTEMPT_PHASES.haltedCancelled) && (
				<section
					className="rounded-md hairline border-destructive/40 bg-destructive/5 p-3"
					data-testid="outcome-run-action-required"
				>
					<h3 className="text-sm font-medium">{t("outcome.run.actionRequiredTitle")}</h3>
					<p className="mt-1 text-muted-foreground text-sm">
						{phase === ATTEMPT_PHASES.haltedCancelled
							? t("outcome.run.cancelledBody")
							: t("outcome.run.actionRequiredBody")}
					</p>
					<div className="mt-2 flex flex-wrap gap-2">
						<Button
							data-testid="outcome-run-replace"
							disabled={pending}
							onClick={() => onRecover("replace", false)}
							size="sm"
						>
							{t("outcome.run.ctaReplace")}
						</Button>
						<Button
							data-testid="outcome-run-replace-confirm"
							disabled={pending}
							onClick={() => onRecover("replace", true)}
							size="sm"
							variant="outline"
						>
							{t("outcome.run.confirmStoppedCta")}
						</Button>
					</div>
				</section>
			)}

			{phase === ATTEMPT_PHASES.executing && (
				<p className="text-muted-foreground text-sm" data-testid="outcome-run-waiting">
					{t("outcome.run.waitingBody")}
				</p>
			)}

			{attempt.status === "running" && phase !== ATTEMPT_PHASES.unconfirmed && (
				<Button
					data-testid="outcome-run-cancel"
					disabled={pending}
					onClick={() => onAct("cancel")}
					size="sm"
					variant="outline"
				>
					{t("outcome.run.ctaCancel")}
				</Button>
			)}




			<p className="text-muted-foreground text-xs">
				{t("outcome.run.observationCount", { total: attempt.observations.length })}
			</p>
		</div>
	);
}
