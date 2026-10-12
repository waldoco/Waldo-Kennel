import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Bot, Check, CircleCheck, CircleX, Clock, Cpu, Gauge, KeyRound } from "lucide-react";
import { useSettings, useUpdateReasoning, useVerifyReasoning } from "../../hooks/useSettings";
import { SettingsOptionMenu, type SettingsOption } from "./SettingsOptionMenu";
import { SettingsRow, settingsRowInputClass } from "./SettingsRow";
import { cn } from "../../lib/utils";
import { SettingsSection } from "./SettingsSection";
import { Input } from "../ui/input";
import { Button } from "../ui/button";
import { normalizeReasoningErrorCode, reasoningStatusMessage } from "../../lib/reasoning-status";

const providers = [
	{ value: "anthropic", label: "Anthropic / Claude API" },
	{ value: "openai", label: "OpenAI API" },
	{ value: "codex", label: "Codex App Server (sign-in)" },
] satisfies SettingsOption<"anthropic" | "openai" | "codex">[];

export function ReasoningSettingsSection({ titleHidden }: { titleHidden?: boolean }) {
	const { t } = useTranslation();
	const { settings, isLoading, error: loadError } = useSettings();
	const { update, saving, error: saveError } = useUpdateReasoning();
	const { verify, verifying, error: verifyError, errorCode: verifyErrorCode, reset: resetVerification } = useVerifyReasoning();
	const [provider, setProvider] = useState<"anthropic" | "openai" | "codex">("anthropic");
	const [model, setModel] = useState("");
	const [effort, setEffort] = useState("");
	const [apiKey, setApiKey] = useState("");
	const [draftDirty, setDraftDirty] = useState(false);
	const [saved, setSaved] = useState(false);

	useEffect(() => {
		const value = settings?.reasoning;
		if (!value || draftDirty) return;
		if (value.provider === "anthropic" || value.provider === "openai" || value.provider === "codex")
			setProvider(value.provider);
		setModel(value.model);
		setEffort(value.effort);
	}, [settings?.reasoning, draftDirty]);

	const status = settings?.reasoning;
	const codexNeedsSetup =
		provider === "codex" &&
		status?.provider === "codex" &&
		status.configured &&
		!status.ready &&
		!saveError &&
		!verifyError &&
		!loadError;
	const statusWithVerifyError = verifyError
		? status
			? { ...status, errorCode: normalizeReasoningErrorCode(verifyErrorCode) ?? "REASONING_UNAVAILABLE" }
			: { provider, configured: true, ready: true, verified: false, errorCode: normalizeReasoningErrorCode(verifyErrorCode) ?? "REASONING_UNAVAILABLE" }
		: status;
	const message =
		saveError ??
		loadError ??
		(draftDirty
			? t("settings.reasoning.draftChanged")
			: verifyError
				? reasoningStatusMessage(statusWithVerifyError, provider, t)
				: reasoningStatusMessage(status, provider, t));

	const isFailure = Boolean(saveError || loadError || !status?.ready);
	const statusTone = isFailure
		? { icon: CircleX, tile: "bg-critical-bg text-critical" }
		: status?.verified && !draftDirty
			? { icon: CircleCheck, tile: "bg-good-bg text-good" }
			: { icon: Clock, tile: "bg-wd-inset text-wd-secondary" };
	const StatusIcon = statusTone.icon;

	return (
		<SettingsSection title={t("settings.reasoning.title")} titleHidden={titleHidden} grouped footer={t("settings.reasoning.privacy")}>
			<SettingsRow icon={Bot} label={t("settings.reasoning.provider")}>
				<SettingsOptionMenu
					aria-label={t("settings.reasoning.provider")}
					value={provider}
					options={providers}
					onChange={(value) => {
						setSaved(false);
						setDraftDirty(true);
						setProvider(value);
					}}
					disabled={isLoading || saving || verifying}
				/>
			</SettingsRow>
			<SettingsRow icon={Cpu} label={t("settings.reasoning.model")}>
				<Input
					value={model}
					onChange={(event) => {
						setSaved(false);
						setDraftDirty(true);
						setModel(event.target.value);
					}}
					placeholder={t("settings.reasoning.providerDefault")}
					disabled={saving}
					className={settingsRowInputClass}
					aria-label={t("settings.reasoning.model")}
				/>
			</SettingsRow>
			<SettingsRow icon={Gauge} label={t("settings.reasoning.effort")}>
				<Input
					value={effort}
					onChange={(event) => {
						setSaved(false);
						setDraftDirty(true);
						setEffort(event.target.value);
					}}
					placeholder={t("settings.reasoning.default")}
					disabled={saving}
					className={settingsRowInputClass}
					aria-label={t("settings.reasoning.effort")}
				/>
			</SettingsRow>
			{provider !== "codex" && (
				<SettingsRow icon={KeyRound} label={t("settings.reasoning.apiKey")}>
					<Input
						type="password"
						value={apiKey}
						onChange={(event) => {
						setSaved(false);
						setDraftDirty(true);
						setApiKey(event.target.value);
						}}
						placeholder={
							status?.provider === provider && status.keyConfigured
								? t("settings.reasoning.keyConfigured")
								: t("settings.reasoning.keyMissing")
						}
						disabled={saving}
						className={settingsRowInputClass}
						aria-label={t("settings.reasoning.apiKey")}
						autoComplete="off"
					/>
				</SettingsRow>
			)}
			<div className="settings-row-bar flex-col items-stretch gap-3">
				<div className="flex items-start gap-3">
					<span className={cn("grid size-7 shrink-0 place-items-center rounded-sm", statusTone.tile)} aria-hidden="true">
						<StatusIcon className="size-4" />
					</span>
					<div className="flex min-w-0 flex-col gap-1">
						<p
							className={cn("wd-body-s", isFailure ? "text-critical" : "text-wd-secondary")}
							aria-live="polite"
							role="status"
						>
							{message}
						</p>
						{codexNeedsSetup && (
							<p className="wd-body-s text-critical" role="note">
								{t("settings.reasoning.codexSetup")}
							</p>
						)}
					</div>
				</div>
				<div className="flex items-center justify-end gap-2">
					<Button
						type="button"
						variant="ghost"
						disabled={!status?.ready || !status?.configured || saving || verifying || draftDirty}
						onClick={() => void verify().catch(() => undefined)}
					>
						{verifying ? t("settings.reasoning.verifying") : t("settings.reasoning.verify")}
					</Button>
					<Button
						type="button"
						variant="primary"
						disabled={saving || verifying}
						onClick={() => {
							setSaved(false);
							void update({ provider, model, effort, ...(apiKey.trim() ? { apiKey: apiKey.trim() } : {}) })
								.then(() => {
									setApiKey("");
									setDraftDirty(false);
									setSaved(true);
									resetVerification();
								})
								.catch(() => undefined);
						}}
					>
						{saving ? t("settings.reasoning.saving") : saved ? <><Check aria-hidden="true" className="size-3.5" />{t("settings.reasoning.saved")}</> : t("settings.reasoning.save")}
					</Button>
				</div>
			</div>
		</SettingsSection>
	);
}
