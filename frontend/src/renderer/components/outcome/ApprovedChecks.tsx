import { EvidenceRow } from "@pin4sf/kennel-product-ui";
import { useTranslation } from "react-i18next";
import type { components } from "../../../api/schema";

/**
 * The deterministic checks a WorkUnit's approved plan binds. Each is an
 * evidence row whose verdict is honestly "Not run" here: these are the checks
 * Kennel will run, not results. The command is shown as code; the timeout is
 * the provenance caption.
 */
export function ApprovedChecks({
	unit,
	criterionText,
}: {
	unit: components["schemas"]["PlanWorkUnitResponse"];
	criterionText?: (id: string) => string | undefined;
}) {
	const { t } = useTranslation();
	return (
		<section className="mt-3 flex flex-col gap-1" aria-label={t("mission.approvedChecks")}>
			<h4 className="text-sm font-medium text-wd-primary">{t("mission.approvedChecks")}</h4>
			{!unit.approvedChecks?.length ? (
				<p className="text-xs text-wd-secondary">{t("mission.noApprovedChecks")}</p>
			) : (
				<ul className="flex flex-col">
					{unit.approvedChecks.map((check) => (
						<EvidenceRow
							assertion={criterionText?.(check.criterionId) ?? t("mission.criterionUnavailable")}
							className="py-2.5"
							key={check.id}
							provenance={<span className="tabular-nums">{t("mission.checkTimeout", { seconds: check.timeoutSeconds })}</span>}
							verdict="notRun"
							verdictLabel={t("outcome.verdict.notRun")}
						>
							<pre className="mt-1 overflow-auto whitespace-pre-wrap break-all rounded-sm bg-wd-inset p-2 font-mono text-xs text-wd-primary">
								{JSON.stringify(check.argv)}
							</pre>
						</EvidenceRow>
					))}
				</ul>
			)}
		</section>
	);
}
