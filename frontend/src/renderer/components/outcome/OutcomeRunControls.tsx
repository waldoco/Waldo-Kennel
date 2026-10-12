import { StateMark, type StateTone } from "@pin4sf/kennel-product-ui";
import { useTranslation } from "react-i18next";
import { useOutcomeRunCommand, useOutcomeRunState } from "../../hooks/useOutcomeRunState";
import { useEventsConnection } from "../../hooks/useEventsConnection";
import { Button } from "../ui/button";

export function OutcomeRunControls({
	outcomeId,
	admissionBlocked = false,
}: {
	outcomeId: string;
	admissionBlocked?: boolean;
}) {
	const { t } = useTranslation();
	const query = useOutcomeRunState(outcomeId);
	const mutation = useOutcomeRunCommand(outcomeId);
	const connection = useEventsConnection();
	const state = query.data;
	const unavailable = query.isError || !state || query.isFetching || connection !== "connected";
	const intent = state?.intent;
	const startEligibility = state?.eligibleActions.find((item) => item.action === "start");
	const resumeEligibility = state?.eligibleActions.find((item) => item.action === "resume");
	const statusTone: StateTone = query.error
		? "critical"
		: !state || !intent
			? "neutral"
			: intent.desired === "running" || !intent.acknowledgedAt
				? "live"
				: "neutral";
	const available = (action: "start" | "pause" | "resume" | "cancel") =>
		Boolean(state?.eligibleActions.find((item) => item.action === action)?.available);
	return (
		<section className="flex flex-col gap-2 rounded-lg hairline border-wd-hairline bg-wd-surface p-4" aria-label={t("mission.run.controls")}>
			<p role="status" className="flex items-center gap-2 text-sm text-wd-primary">
				<StateMark className={statusTone === "critical" ? "text-critical" : undefined} tone={statusTone} />
				{query.error?.message ??
					(!state
						? t("mission.run.loading")
						: intent
							? !intent.acknowledgedAt && intent.desired === "paused"
								? t("mission.run.pausedPending")
								: !intent.acknowledgedAt && intent.desired === "cancelled"
									? t("mission.run.cancelledPending")
									: t(`mission.run.${intent.desired}`)
							: t("mission.run.idle"))}
			</p>
			{state?.blocker && <p className="text-sm text-wd-secondary">{state.blocker.message}</p>}
			{intent && !intent.bindsCurrentPlan && (
				<p className="flex items-start gap-1.5 text-sm text-wd-primary">
					<StateMark className="mt-1 text-caution" tone="caution" />
					{t("mission.run.supersededIntent")}
				</p>
			)}
			{state?.blocker?.detail && (
				<details className="text-xs text-wd-secondary">
					<summary className="cursor-pointer rounded-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)">{t("mission.run.blockerDetail")}</summary>
					<div className="mt-1 space-y-1 pl-3">
						{Object.entries(state.blocker.detail).map(([key, value]) => (
							<p key={key}>
								<span className="text-wd-tertiary">{key}:</span>{" "}
								{typeof value === "string" || typeof value === "number" || typeof value === "boolean" ? String(value) : JSON.stringify(value)}
							</p>
						))}
					</div>
				</details>
			)}
			{startEligibility && !startEligibility.available && startEligibility.reason && (
				<p className="text-xs text-wd-secondary" data-testid="outcome-run-start-refusal">
					{t("mission.run.actionUnavailable", {
						action: t("mission.run.action.start"),
						reason: t(`mission.run.reason.${startEligibility.reason}`, { defaultValue: startEligibility.reason }),
					})}
				</p>
			)}
			{resumeEligibility && !resumeEligibility.available && resumeEligibility.reason && (
				<p className="text-xs text-wd-secondary" data-testid="outcome-run-resume-refusal">
					{t("mission.run.actionUnavailable", {
						action: t("mission.run.action.resume"),
						reason: t(`mission.run.reason.${resumeEligibility.reason}`, { defaultValue: resumeEligibility.reason }),
					})}
				</p>
			)}
			{unavailable && <p className="text-xs text-wd-secondary">{t("mission.run.unavailable")}</p>}
			{admissionBlocked && (available("start") || available("resume")) && (
				<p className="text-xs text-wd-secondary" data-testid="outcome-run-admission-blocked">{t("mission.run.admissionBlocked")}</p>
			)}
			<div className="mt-1 flex flex-wrap gap-2">
				{(["start", "pause", "resume", "cancel"] as const).map((action) => {
					const eligibility = state?.eligibleActions.find((item) => item.action === action);
					if (!eligibility?.available) return null;
					return (
						<Button
							key={action}
							data-testid={`outcome-run-${action}`}
							variant={action === "start" || action === "resume" ? "primary" : action === "cancel" ? "critical" : "ghost"}
							disabled={
								unavailable || mutation.isPending || (admissionBlocked && (action === "start" || action === "resume"))
							}
							onClick={() => void mutation.command(action, state!).catch(() => {})}
						>
							{t(`mission.run.action.${action}`)}
						</Button>
					);
				})}
				{query.isError && (
					<Button variant="outline" onClick={() => void query.refetch()}>
						{t("mission.refresh")}
					</Button>
				)}
			</div>
			{available("pause") && <p className="text-xs text-wd-secondary">{t("mission.run.pauseConsequence")}</p>}
			{available("cancel") && <p className="text-xs text-wd-secondary">{t("mission.run.cancelConsequence")}</p>}
			{mutation.error && (
				<p role="alert" className="text-sm text-critical">
					{mutation.error.message}
				</p>
			)}
		</section>
	);
}
