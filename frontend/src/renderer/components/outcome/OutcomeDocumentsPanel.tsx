import { FileText, Loader2, RefreshCw } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import {
	useApproveOutcomeDocuments,
	useOutcomeDocumentContext,
	useSelectOutcomeDocuments,
} from "../../hooks/useOutcomeArtifacts";
import { Badge } from "../ui/badge";
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
		<section className="mt-5 rounded-md border border-border p-4" data-testid="outcome-documents-panel">
			<div className="flex flex-wrap items-start justify-between gap-3">
				<div>
					<div className="flex items-center gap-2">
						<FileText aria-hidden="true" className="size-4 text-muted-foreground" />
						<h3 className="text-sm font-medium">{t("mission.documents.heading")}</h3>
						{context && <Badge variant={context.state === "approved" ? "success" : "outline"}>{context.state}</Badge>}
						{!context && !contextQuery.isLoading && !failure && (
							<Badge variant="outline">{t("mission.documents.optional")}</Badge>
						)}
					</div>
					<p className="mt-1 text-xs text-muted-foreground">{t("mission.documents.intro")}</p>
					{!context && !contextQuery.isLoading && !failure && (
						<p className="mt-1 text-xs text-muted-foreground" data-testid="outcome-documents-optional">
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

			<label className="mt-3 flex flex-col gap-1 text-xs text-muted-foreground" htmlFor="outcome-document-picker">
				<span>{t("mission.documents.choose")}</span>
				<input
					accept=".md,.markdown,.txt,.pdf,.doc,.docx,.json,.yaml,.yml,.csv"
					className="block w-full rounded-md border border-input bg-background px-2 py-2 text-xs text-foreground file:mr-3 file:rounded file:border-0 file:bg-muted file:px-2 file:py-1 file:text-xs"
					disabled={pending}
					id="outcome-document-picker"
					multiple
					onChange={(event) => onFilesSelected(event.currentTarget.files)}
					type="file"
				/>
			</label>
			<p className="mt-1 text-2xs text-muted-foreground">{t("mission.documents.pathNote")}</p>

			{paths.length > 0 && (
				<div className="mt-3 rounded border border-border p-2">
					<p className="text-xs font-medium">{t("mission.documents.pendingSelection", { count: paths.length })}</p>
					<ul className="mt-1 space-y-1 text-2xs text-muted-foreground">
						{paths.map((path) => <li className="break-all" key={path}>{path}</li>)}
					</ul>
					<Button className="mt-2" disabled={pending} onClick={() => void select()} size="sm" variant="outline">
						{selectMutation.pending && <Loader2 aria-hidden="true" className="size-3.5 animate-spin" />}
						{t("mission.documents.select")}
					</Button>
				</div>
			)}

			{pickerMessage && <p className="mt-2 text-xs text-warning">{pickerMessage}</p>}

			{context && (
				<div className="mt-3 rounded border border-border p-3" data-testid="outcome-document-context">
					<div className="flex items-center justify-between gap-2">
						<h4 className="text-xs font-medium">{t("mission.documents.selected")}</h4>
						<span className="text-2xs text-muted-foreground">{t("mission.documents.revision", { revision: context.revision })}</span>
					</div>
					<ul className="mt-2 space-y-2">
						{context.sources.map((source) => (
							<li className="text-xs" key={source.id}>
								<p className="font-medium">{source.name}</p>
								<p className="break-all text-2xs text-muted-foreground">
									{source.sourcePath} · {t("mission.documents.bytes", { size: source.sizeBytes })}
								</p>
							</li>
						))}
					</ul>
					{context.changedSources.length > 0 && (
						<p className="mt-2 text-xs text-warning">{t("mission.documents.changed", { count: context.changedSources.length })}</p>
					)}
					{context.state === "selected" && (
						<Button className="mt-3" disabled={pending || context.changedSources.length > 0} onClick={() => void approve()} size="sm">
							{approveMutation.pending && <Loader2 aria-hidden="true" className="size-3.5 animate-spin" />}
							{t("mission.documents.approve")}
						</Button>
					)}
					{context.state === "approved" && <p className="mt-2 text-xs text-status-ready">{t("mission.documents.approved")}</p>}
				</div>
			)}

			{contextQuery.isLoading && <p className="mt-2 text-xs text-muted-foreground">{t("mission.documents.loading")}</p>}
			{failure && <p className="mt-2 text-xs text-destructive" role="alert">{failure.message}</p>}
		</section>
	);
}
