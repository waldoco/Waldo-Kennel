import { Compass, Flag, Keyboard } from "lucide-react";
import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { GeneralSettingsSection } from "./settings/GeneralSettingsSection";
import { IslandSettingsSection } from "./settings/IslandSettingsSection";
import { ReportProblemDialog } from "./settings/ReportProblemDialog";
import { SettingsLinkRow } from "./settings/SettingsRow";
import { SettingsSection } from "./settings/SettingsSection";
import { UpdatesSection } from "./settings/UpdatesSection";
import { ReasoningSettingsSection } from "./settings/ReasoningSettingsSection";
import { RepositoryContextSettingsSection } from "./settings/RepositoryContextSettingsSection";
import { useUiStore } from "../stores/ui-store";

import { WaldoConnectionSection } from "./settings/WaldoConnectionSection";

export type GlobalSettingsSection = "general" | "reasoning" | "updates" | "help" | "waldo" | "all";

export function GlobalSettingsForm({
	section = "all",
	onOpenKeyboardShortcuts,
	waldoOpen = true,
	waldoPreview,
}: {
	section?: GlobalSettingsSection;
	onOpenKeyboardShortcuts?: () => void;
	waldoOpen?: boolean;
	waldoPreview?: ReactNode;
}) {
	const { t } = useTranslation();
	const [reportProblemOpen, setReportProblemOpen] = useState(false);
	const closeSettings = useUiStore((state) => state.closeSettings);
	const openOnboarding = useUiStore((state) => state.openOnboarding);
	// One section per page means the dialog header already names it, so the
	// page's leading heading would just repeat that title. Only "all" (no
	// single-page header) shows every section's own heading.
	const leadingTitleHidden = section !== "all";

	return (
		<>
			<div
				aria-label={t("settings.title")}
				className="flex w-full flex-col gap-(--size-settings-section-gap)"
				data-testid="settings-page"
			>
				{(section === "all" || section === "general") && (
					<>
						<GeneralSettingsSection
							titleHidden={leadingTitleHidden}
						/>
						<SettingsSection title={t("settings.preferences")} grouped>
							<SettingsLinkRow
								icon={Keyboard}
								label={t("settings.keyboardShortcuts")}
								onClick={() => onOpenKeyboardShortcuts?.()}
							/>
							{/* Settings has to get out of the way first: the tour is a modal,
							    and two stacked modals leave no way back to the one beneath. */}
							<SettingsLinkRow
								icon={Compass}
								label={t("settings.welcomeTour")}
								onClick={() => {
									closeSettings();
									openOnboarding();
								}}
							/>
						</SettingsSection>
						<IslandSettingsSection />
					</>
				)}
				{(section === "all" || section === "waldo") && (waldoPreview ?? <WaldoConnectionSection open={waldoOpen} titleHidden={leadingTitleHidden} />)}
				{(section === "all" || section === "updates") && <UpdatesSection titleHidden={leadingTitleHidden} />}
				{(section === "all" || section === "reasoning") && (
					<>
						<ReasoningSettingsSection titleHidden={leadingTitleHidden} />
						<RepositoryContextSettingsSection titleHidden={leadingTitleHidden} />
					</>
				)}
				{(section === "all" || section === "help") && (
					<SettingsSection title={t("settings.getHelp")} titleHidden={leadingTitleHidden} grouped>
						<SettingsLinkRow icon={Flag} label={t("settings.reportProblem")} onClick={() => setReportProblemOpen(true)} />
					</SettingsSection>
				)}
			</div>
			<ReportProblemDialog open={reportProblemOpen} onOpenChange={setReportProblemOpen} />
		</>
	);
}
