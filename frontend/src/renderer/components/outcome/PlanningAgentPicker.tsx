import { StateBadge } from "@pin4sf/kennel-product-ui";
import { cn } from "../../lib/utils";
import { Button } from "../ui/button";
import type { PlanningCandidate } from "../../hooks/usePlanning";
import { useTranslation } from "react-i18next";

export function PlanningAgentPicker({
	candidates,
	selectedId,
	onChange,
	onRetry,
	loading = false,
	failure,
	disabled = false,
}: {
	candidates: PlanningCandidate[];
	selectedId: string;
	onChange: (candidateId: string) => void;
	onRetry?: () => void;
	loading?: boolean;
	failure?: string;
	disabled?: boolean;
}) {
	const { t } = useTranslation();
	return (
		<fieldset className="flex flex-col gap-2" disabled={disabled} data-testid="planning-agent-picker">
			<legend className="text-sm font-medium text-wd-primary">{t("planning.providerLegend")}</legend>
			<p className="text-xs leading-body text-wd-secondary">{t("planning.providerHelp")}</p>
			{loading ? (
				<p className="rounded-md bg-wd-inset p-3 text-sm text-wd-secondary">{t("planning.checkingCandidates")}</p>
			) : failure ? (
				<div className="flex flex-wrap items-center justify-between gap-2 rounded-md bg-wd-inset p-3 text-sm text-wd-primary"><span>{failure}</span>{onRetry && <Button onClick={onRetry} size="sm" type="button" variant="secondary">{t("mission.refresh")}</Button>}</div>
			) : candidates.length === 0 ? (
				<p className="rounded-md bg-wd-inset p-3 text-sm text-wd-secondary" data-testid="planning-no-candidates">
					{t("planning.noCandidates")}
				</p>
			) : (
				<div className="flex flex-col gap-1.5">
					{candidates.map((candidate) => {
						const binding = candidate.binding;
						const identity = binding.modelSelection === "explicit" ? binding.model ?? t("planning.explicitModel") : t("planning.providerDefault");
						return (
							<label
								// ChoiceList row: full width, hairline, the radio mark in
								// accent; the chosen row takes the quiet selected fill.
								className={cn(
									"flex cursor-pointer items-start gap-2.5 rounded-md hairline px-3 py-2.5 text-sm transition-colors duration-fast motion-reduce:transition-none has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-(--border-focus)",
									selectedId === candidate.id ? "border-wd-strong bg-wd-selected" : "border-wd-hairline hover:bg-wd-hover",
									!candidate.ready && "cursor-not-allowed text-wd-secondary hover:bg-transparent",
								)}
								key={candidate.id}
							>
								<input
									className="mt-0.5 accent-(--wd-accent) focus-visible:outline-none"
									checked={selectedId === candidate.id}
									disabled={!candidate.ready}
									name="planning-candidate"
									onChange={() => onChange(candidate.id)}
									type="radio"
								/>
								<span className="min-w-0 flex-1">
									<span className={cn("flex flex-wrap items-center gap-2 font-medium", candidate.ready ? "text-wd-primary" : "text-wd-secondary")}>
										{binding.provider} · {identity} · {binding.mode === "native_harness" ? t("planning.subscriptionAgent") : t("planning.apiReasoner")}
										<StateBadge label={candidate.ready ? t("planning.available") : t("planning.unavailable")} tone={candidate.ready ? "good" : "neutral"} />
									</span>
									{!candidate.ready && (
										<span className="mt-1 block text-xs text-wd-secondary">
											{candidate.unavailableCode ?? t("planning.unavailableCode")}: {candidate.unavailableDetail ?? t("planning.unavailableDetail")}
										</span>
									)}
								</span>
							</label>
						);
					})}
				</div>
			)}
		</fieldset>
	);
}
