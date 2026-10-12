import { PanelTop, SlidersHorizontal } from "lucide-react";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import type { IslandVisibilityState } from "../../../shared/island";
import { aoBridge } from "../../lib/bridge";
import { Switch } from "../ui/switch";
import { SettingsLinkRow, SettingsRow } from "./SettingsRow";
import { SettingsSection } from "./SettingsSection";

type IslandError = "load" | "save" | "open" | null;

export function IslandSettingsSection() {
	const { t } = useTranslation();
	const [state, setState] = useState<IslandVisibilityState | null>(null);
	const [error, setError] = useState<IslandError>(null);
	const [saving, setSaving] = useState(false);

	useEffect(() => {
		let active = true;
		let receivedEvent = false;
		const dispose = aoBridge.island.onState((next) => {
			receivedEvent = true;
			if (!active) return;
			setState(next);
			setError(null);
			setSaving(false);
		});

		void aoBridge.island
			.getState()
			.then((next) => {
				if (active && !receivedEvent) setState(next);
			})
			.catch(() => {
				if (active) setError("load");
			});

		return () => {
			active = false;
			dispose();
		};
	}, []);

	const supported = state?.supported === true;
	const controlsDisabled = !supported || state === null || saving;

	const setVisible = (visible: boolean) => {
		if (!state || !supported) return;
		const previous = state;
		setError(null);
		setSaving(true);
		// The main process remains authoritative, but the switch should respond in
		// the same frame instead of waiting for the window lifecycle round-trip.
		setState({ ...state, enabled: visible, visible });
		void aoBridge.island
			.setVisible(visible)
			.then((next) => {
				setState(next);
				setSaving(false);
			})
			.catch(() => {
				setState(previous);
				setSaving(false);
				setError("save");
			});
	};

	const openSettings = () => {
		setError(null);
		void aoBridge.island.openSettings().catch(() => setError("open"));
	};

	const helperCopy = error
		? t(`settings.island.${error}Failed`, {
				defaultValue:
					error === "load"
						? "Island controls are unavailable right now."
						: error === "save"
							? "Could not update Island visibility."
							: "Could not open Island settings.",
			})
		: state && !state.supported
			? t("settings.island.unsupported", {
					defaultValue: "Island is available on Macs with a built-in display notch.",
				})
			: t("settings.island.description", {
					shortcut: state?.shortcut ?? "⌘`",
					defaultValue: "Keep session activity visible at your MacBook notch. Toggle it anytime with {{shortcut}}.",
				});

	return (
		<SettingsSection
			title={t("settings.island.title", { defaultValue: "Island" })}
			sectionId="island"
			grouped
			footer={
				<span role={error ? "alert" : undefined} className={error ? "text-critical" : undefined}>
					{helperCopy}
				</span>
			}
		>
			<SettingsRow icon={PanelTop} label={t("settings.island.visibility", { defaultValue: "Show Island" })}>
				<Switch
					aria-label={t("settings.island.visibility", { defaultValue: "Show Island" })}
					checked={state?.enabled ?? false}
					disabled={controlsDisabled}
					onCheckedChange={setVisible}
				/>
			</SettingsRow>
			<SettingsLinkRow
				icon={SlidersHorizontal}
				label={t("settings.island.openSettings", { defaultValue: "Open Island settings" })}
				disabled={!supported}
				onClick={openSettings}
			/>
		</SettingsSection>
	);
}
