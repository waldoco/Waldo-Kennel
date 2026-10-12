import { useTranslation } from "react-i18next";
import { useOutcomeAttempts } from "../../hooks/useOutcome";
import { useSessionUsageSummaries } from "../../hooks/useSessionUsageSummaries";

export function MissionUsage({ outcomeId, projectId }: { outcomeId: string; projectId: string }) {
	const { t } = useTranslation();
	const attempts = useOutcomeAttempts(outcomeId);
	const usage = useSessionUsageSummaries(projectId);
	const sessions = new Map(
		(attempts.attempts ?? []).flatMap((attempt) =>
			(attempt.sessions ?? []).map((session) => [session.sessionId, session] as const),
		),
	);
	return (
		<section className="mt-4 flex flex-col gap-2 hairline-t border-wd-hairline pt-4">
			<h3 className="text-sm font-medium text-wd-primary">{t("mission.usage")}</h3>
			<p className="text-xs text-wd-secondary">{t("mission.usageScope")}</p>
			{sessions.size ? (
				<ul className="flex flex-col gap-1 text-xs text-wd-primary">
					{[...sessions.values()].map((session) => {
						const recorded = usage.data?.get(session.sessionId);
						return (
							<li key={session.sessionId}>
								{session.harness} ·{" "}
								{recorded ? (
									<span className="tabular-nums">{t("mission.tokens", { amount: recorded.totalTokens.toLocaleString() })}</span>
								) : (
									<span className="text-wd-secondary">{t("mission.unknown")}</span>
								)}
								{recorded?.incomplete ? <span className="text-wd-secondary">{` · ${t("mission.incomplete")}`}</span> : ""}
							</li>
						);
					})}
				</ul>
			) : (
				<p className="text-xs text-wd-secondary">{t("mission.unknown")}</p>
			)}
			{usage.error && (
				<p role="alert" className="text-xs text-wd-primary">
					{t("mission.usageUnavailable")}
				</p>
			)}
		</section>
	);
}
