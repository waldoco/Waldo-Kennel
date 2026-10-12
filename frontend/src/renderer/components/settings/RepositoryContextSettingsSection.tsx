import { Check, ChevronDown, FolderSearch } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import type { components } from "../../../api/schema";
import { useSettings, useUpdateRepositoryContextLimits } from "../../hooks/useSettings";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { SettingsRow, settingsRowInputClass } from "./SettingsRow";
import { cn } from "../../lib/utils";
import { SettingsSection } from "./SettingsSection";

type LimitName = "maxFiles" | "maxBytes" | "maxVisited";
type LimitDraft = Record<LimitName, string>;
type UpdateRequest = components["schemas"]["ControllersUpdateRepositoryContextLimitsRequest"];

const limitNames: LimitName[] = ["maxFiles", "maxBytes", "maxVisited"];

function draftFromSettings(settings: ReturnType<typeof useSettings>["settings"]): LimitDraft {
	return {
		maxFiles: settings?.repositoryContext.maxFiles == null ? "" : String(settings.repositoryContext.maxFiles),
		maxBytes: settings?.repositoryContext.maxBytes == null ? "" : String(settings.repositoryContext.maxBytes),
		maxVisited: settings?.repositoryContext.maxVisited == null ? "" : String(settings.repositoryContext.maxVisited),
	};
}

function parseLimit(value: string): number | null | undefined {
	if (value.trim() === "") return null;
	const parsed = Number(value);
	if (!Number.isSafeInteger(parsed) || parsed < 0) return undefined;
	return parsed;
}

export function RepositoryContextSettingsSection({ titleHidden }: { titleHidden?: boolean }) {
	const { t } = useTranslation();
	const { settings, isLoading, error: loadError } = useSettings();
	const { update, saving, error: saveError, reset: resetMutation } = useUpdateRepositoryContextLimits();
	const [draft, setDraft] = useState<LimitDraft>(() => draftFromSettings(settings));
	const [dirty, setDirty] = useState<Set<LimitName>>(new Set());
	const [saved, setSaved] = useState(false);

	useEffect(() => {
		if (!settings || dirty.size > 0) return;
		setDraft(draftFromSettings(settings));
	}, [settings, dirty.size]);

	const invalid = useMemo(
		() => limitNames.some((name) => dirty.has(name) && parseLimit(draft[name]) === undefined),
		[draft, dirty],
	);

	const change = (name: LimitName, value: string) => {
		setDraft((current) => ({ ...current, [name]: value }));
		setDirty((current) => new Set(current).add(name));
		setSaved(false);
		resetMutation();
	};

	const save = async () => {
		const patch: UpdateRequest = {};
		for (const name of dirty) {
			const value = parseLimit(draft[name]);
			if (value === undefined) return;
			patch[name] = value;
		}
		await update(patch);
		setDirty(new Set());
		setSaved(true);
	};

	const labels: Record<LimitName, string> = {
		maxFiles: t("settings.repositoryContext.maxFiles"),
		maxBytes: t("settings.repositoryContext.maxBytes"),
		maxVisited: t("settings.repositoryContext.maxVisited"),
	};
	const defaults: Record<LimitName, number> = {
		maxFiles: 32,
		maxBytes: 96 * 1024,
		maxVisited: 20_000,
	};

	return (
		<SettingsSection title={t("settings.repositoryContext.title")} titleHidden={titleHidden} grouped>
			<details className="group">
				<summary className="settings-row-bar cursor-pointer list-none transition-colors duration-fast hover:bg-wd-hover focus-visible:outline-offset-[-2px] [&::-webkit-details-marker]:hidden">
					<span className="flex min-w-0 items-center gap-3">
						<span className="settings-row-icon grid size-7 shrink-0 place-items-center rounded-sm bg-wd-inset text-wd-secondary" aria-hidden="true">
							<FolderSearch className="size-4" />
						</span>
						<span className="wd-body truncate text-wd-primary">{t("settings.repositoryContext.advanced")}</span>
					</span>
					<ChevronDown aria-hidden="true" className="size-4 shrink-0 text-wd-tertiary transition-transform duration-fast group-open:rotate-180" />
				</summary>
				<div className="flex flex-col border-t-[length:var(--stroke-hairline)] border-wd-hairline [&>*+*]:border-t-[length:var(--stroke-hairline)] [&>*+*]:border-wd-hairline">
					<p className="wd-body-s px-4 py-3 text-wd-secondary">
						{t("settings.repositoryContext.help")}
					</p>
					{limitNames.map((name) => (
						<SettingsRow key={name} label={labels[name]}>
							<Input
								aria-label={labels[name]}
								className={cn(settingsRowInputClass, "w-32")}
								disabled={isLoading || saving}
								inputMode="numeric"
								min={0}
								onChange={(event) => change(name, event.target.value)}
								placeholder={t("settings.repositoryContext.default", { value: defaults[name] })}
								type="number"
								value={draft[name]}
							/>
						</SettingsRow>
					))}
					<div className="flex items-center justify-between gap-3 px-4 py-3">
						<p
							aria-live="polite"
							className={cn("wd-body-s", invalid || loadError || saveError ? "text-critical" : "text-wd-secondary")}
							role="status"
						>
							{loadError ?? saveError ?? (invalid ? t("settings.repositoryContext.invalid") : t("settings.repositoryContext.zeroHelp"))}
						</p>
						<Button
							disabled={saving || isLoading || invalid || dirty.size === 0}
							onClick={() => void save().catch(() => undefined)}
							type="button"
							variant="primary"
						>
							{saving ? (
								t("settings.repositoryContext.saving")
							) : saved ? (
								<>
									<Check aria-hidden="true" className="size-3.5" />
									{t("settings.repositoryContext.saved")}
								</>
							) : (
								t("settings.repositoryContext.save")
							)}
						</Button>
					</div>
					<div className="px-2 py-2">
					<Button
						className="self-start"
						disabled={saving || isLoading}
						onClick={() => {
							setDraft({ maxFiles: "", maxBytes: "", maxVisited: "" });
							setDirty(new Set(limitNames));
							setSaved(false);
							resetMutation();
						}}
						type="button"
						variant="ghost"
					>
						{t("settings.repositoryContext.reset")}
					</Button>
					</div>
				</div>
			</details>
		</SettingsSection>
	);
}
