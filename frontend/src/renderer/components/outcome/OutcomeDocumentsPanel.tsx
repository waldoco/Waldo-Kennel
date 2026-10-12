import { StateBadge, StateMark } from "@pin4sf/kennel-product-ui";
import { FileText, RefreshCw } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import {
	useApproveOutcomeDocuments,
	useOutcomeDocumentContext,
	useSelectOutcomeDocuments,
} from "../../hooks/useOutcomeArtifacts";
import { Button } from "../ui/button";

type LocalFile = File & { path?: string };

/**
 * The document context is a durable Outcome input, not a chat attachment.
 * Selection snapshots the exact local paths through the daemon; approval is a
 * separate owner decision, so a changed source can never be silently reused.
 */
export function OutcomeDocumentsPanel({ outcomeId }: { outcomeId: string }) {
	const { t } = useTranslation();
	const contextQuery = useOutcomeDocumentContext(outcomeId);
	const selectMutation = useSelectOutcomeDocuments(outcomeId);
	const approveMutation = useApproveOutcomeDocuments(outcomeId);
	const [paths, setPaths] = useState<string[]>([]);
	const [pickerMessage, setPickerMessage] = useState<string>();

	const context = contextQuery.context;
	const failure = contextQuery.failure ?? selectMutation.failure ?? approveMutation.failure;
	const pending = selectMutation.pending || approveMutation.pending;

	function onFilesSelected(files: FileList | null) {
		const selected = Array.from(files ?? [])
			.map((file) => (file as LocalFile).path?.trim())
			.filter((path): path is string => Boolean(path));
		const unique = [...new Set(selected)];
		setPaths(unique);
		setPickerMessage(
			unique.length > 0
				? undefined
				: t("mission.documents.pathUnavailable"),
		);
	}

	async function select() {
		if (paths.length === 0 || pending) return;
		try {
			await selectMutation.select({ paths });
			setPickerMessage(undefined);
		} catch {
			// The mutation exposes the daemon's typed refusal below.
		}
	}

	async function approve() {
		if (!context || context.state !== "selected" || pending) return;
		try {
			await approveMutation.approve({ expectedDigest: context.digest });
		} catch {
			// The mutation exposes the daemon's typed refusal below.
		}
	}

	return (
		<section className="mt-5 flex flex-col gap-3 rounded-lg hairline border-wd-hairline bg-wd-surface p-5" data-testid="outcome-documents-panel">
			<div className="flex flex-wrap items-start justify-between gap-3">
				<div className="flex min-w-0 flex-col gap-1">
					<div className="flex items-center gap-2">
						<FileText aria-hidden="true" className="size-4 text-wd-secondary" />
						<h3 className="text-sm font-medium text-wd-primary">{t("mission.documents.heading")}</h3>
						{context && (
							<StateBadge
								label={context.state === "approved" ? t("mission.documents.state.approved") : t("mission.documents.state.selected")}
								tone={context.state === "approved" ? "good" : "neutral"}
							/>
						)}
						{!context && !contextQuery.isLoading && !failure && (
							<StateBadge label={t("mission.documents.optional")} tone="neutral" />
						)}
					</div>
					<p className="text-xs text-wd-secondary">{t("mission.documents.intro")}</p>
					{!context && !contextQuery.isLoading && !failure && (
						<p className="text-xs text-wd-secondary" data-testid="outcome-documents-optional">
							{t("mission.documents.optionalEmpty")}
						</p>
					)}
				</div>
				{contextQuery.failure && (
					<Button onClick={contextQuery.refetch} size="sm" variant="ghost">
						<RefreshCw aria-hidden="true" className="size-3.5" />
						{t("mission.refresh")}
					</Button>
				)}
			</div>

			<label className="flex flex-col gap-1 text-xs text-wd-secondary" htmlFor="outcome-document-picker">
				<span>{t("mission.documents.choose")}</span>
				<input
					accept=".md,.markdown,.txt,.pdf,.doc,.docx,.json,.yaml,.yml,.csv"
					className="block w-full rounded-md bg-wd-inset px-3 py-2 text-xs text-wd-primary file:mr-3 file:rounded-full file:border-0 file:bg-wd-surface file:px-3 file:py-1 file:text-xs file:font-medium file:text-wd-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)"
					disabled={pending}
					id="outcome-document-picker"
					multiple
					onChange={(event) => onFilesSelected(event.currentTarget.files)}
					type="file"
				/>
			</label>
			<p className="-mt-2 text-xs text-wd-tertiary">{t("mission.documents.pathNote")}</p>

			{paths.length > 0 && (
				<div className="flex flex-col gap-2 rounded-md bg-wd-inset p-3">
					<p className="text-sm font-medium text-wd-primary">{t("mission.documents.pendingSelection", { count: paths.length })}</p>
					<ul className="flex flex-col gap-1 font-mono text-xs text-wd-secondary">
						{paths.map((path) => <li className="break-all" key={path}>{path}</li>)}
					</ul>
					<Button aria-busy={selectMutation.pending || undefined} className="self-start" disabled={pending} onClick={() => void select()} size="sm" variant="secondary">
						{selectMutation.pending ? t("mission.documents.selecting") : t("mission.documents.select")}
					</Button>
				</div>
			)}

			{pickerMessage && (
				<p className="flex items-start gap-1.5 text-xs text-wd-primary">
					<StateMark className="mt-0.5 text-caution" tone="caution" />
					{pickerMessage}
				</p>
			)}

			{context && (
				<div className="flex flex-col gap-2 rounded-md bg-wd-inset p-3" data-testid="outcome-document-context">
					<div className="flex items-center justify-between gap-2">
						<h4 className="text-sm font-medium text-wd-primary">{t("mission.documents.selected")}</h4>
						<span className="text-xs tabular-nums text-wd-tertiary">{t("mission.documents.revision", { revision: context.revision })}</span>
					</div>
					<ul className="flex flex-col gap-2">
						{context.sources.map((source) => (
							<li className="flex flex-col gap-0.5 text-xs" key={source.id}>
								<p className="text-sm text-wd-primary">{source.name}</p>
								<p className="break-all text-wd-tertiary">
									<span className="font-mono">{source.sourcePath}</span> · <span className="tabular-nums">{t("mission.documents.bytes", { size: source.sizeBytes })}</span>
								</p>
							</li>
						))}
					</ul>
					{context.changedSources.length > 0 && (
						<p className="flex items-start gap-1.5 text-xs text-wd-primary">
							<StateMark className="mt-0.5 text-caution" tone="caution" />
							{t("mission.documents.changed", { count: context.changedSources.length })}
						</p>
					)}
					{context.state === "selected" && (
						<Button aria-busy={approveMutation.pending || undefined} className="mt-1 self-start" disabled={pending || context.changedSources.length > 0} onClick={() => void approve()} size="sm">
							{approveMutation.pending ? t("mission.documents.approving") : t("mission.documents.approve")}
						</Button>
					)}
					{context.state === "approved" && (
						<p className="flex items-center gap-1.5 text-xs text-wd-primary">
							<StateMark className="text-good" tone="good" />
							{t("mission.documents.approved")}
						</p>
					)}
				</div>
			)}

			{contextQuery.isLoading && <p className="text-xs text-wd-secondary" role="status">{t("mission.documents.loading")}</p>}
			{failure && (
				<p className="flex items-start gap-1.5 text-xs text-wd-primary" role="alert">
					<StateMark className="mt-0.5 text-critical" tone="critical" />
					{failure.message}
				</p>
			)}
		</section>
	);
}
