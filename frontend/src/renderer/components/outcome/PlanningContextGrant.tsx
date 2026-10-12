import type { PlanningContextMode } from "../../hooks/usePlanning";
import { useTranslation } from "react-i18next";

export function PlanningContextGrant({
	value,
	onChange,
	disabled = false,
	locked = false,
	suppliedPacketAvailable = false,
}: {
	value: PlanningContextMode | null;
	onChange?: (mode: PlanningContextMode) => void;
	disabled?: boolean;
	locked?: boolean;
	suppliedPacketAvailable?: boolean;
}) {
	const { t } = useTranslation();
	return (
		<fieldset className="flex flex-col gap-2" disabled={disabled || locked} data-testid="planning-context-grant">
			<legend className="text-sm font-medium text-wd-primary">{t("planning.contextLegend")}</legend>
			<p className="text-xs text-wd-secondary">{t("planning.contextHelp")}</p>
			<label className={optionClass(value === "repository_read")}>
				<input checked={value === "repository_read"} className="mt-0.5 accent-(--text-primary)" name="planning-context" onChange={() => onChange?.("repository_read")} type="radio" />
				<span><span className="font-medium text-wd-primary">{t("planning.repositoryLabel")}</span><span className="mt-0.5 block text-xs text-wd-secondary">{t("planning.repositoryHelp")}</span></span>
			</label>
			{suppliedPacketAvailable && <label className={optionClass(value === "supplied_packet")}>
				<input checked={value === "supplied_packet"} className="mt-0.5 accent-(--text-primary)" name="planning-context" onChange={() => onChange?.("supplied_packet")} type="radio" />
				<span><span className="font-medium text-wd-primary">{t("planning.packetLabel")}</span><span className="mt-0.5 block text-xs text-wd-secondary">{t("planning.packetHelp")}</span></span>
			</label>}
			{locked && <p className="text-xs text-wd-secondary">{t("planning.scopeFrozen")}</p>}
		</fieldset>
	);
}

/** Selection is neutral (bg-selected + strong hairline), never a coloured ring. */
function optionClass(selected: boolean): string {
	return `flex cursor-pointer items-start gap-2 rounded-md hairline px-3 py-2 text-sm transition-colors duration-fast motion-reduce:transition-none ${
		selected ? "border-wd-strong bg-wd-selected" : "border-wd-hairline hover:bg-wd-hover"
	}`;
}
