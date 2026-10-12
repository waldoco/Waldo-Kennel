import * as Dialog from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useWorkspaceQuery } from "../hooks/useWorkspaceQuery";
import { spawnOrchestrator } from "../lib/spawn-orchestrator";
import { useUiStore } from "../stores/ui-store";
import { hasConfiguredOrchestratorAgent, isOrchestratorSession } from "../types/workspace";
import type { WorkspaceSession } from "../types/workspace";
import { Button } from "./ui/button";
import {
	settingsDialogBodyClass,
	settingsDialogContentClass,
	settingsDialogFooterClass,
	settingsDialogHeaderClass,
} from "./ui/dialog";

type RestoreUnavailableDialogProps = {
	open: boolean;
	session: WorkspaceSession;
	onOpenChange: (open: boolean) => void;
	onRecreated: (newOrchestratorId: string) => void;
};

export function RestoreUnavailableDialog({ open, session, onOpenChange, onRecreated }: RestoreUnavailableDialogProps) {
	const { t } = useTranslation();
	const workspaceQuery = useWorkspaceQuery();
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState<string | undefined>();
	const orchestrator = isOrchestratorSession(session);
	const workspace = workspaceQuery.data?.find((candidate) => candidate.id === session.workspaceId);
	const hasOrchestratorAgent = hasConfiguredOrchestratorAgent(workspace);
	const checkingProject = workspaceQuery.isLoading && workspaceQuery.data === undefined;

	const recreate = async () => {
		if (checkingProject) return;
		if (!hasOrchestratorAgent) {
			onOpenChange(false);
			useUiStore.getState().openProjectSettings(session.workspaceId);
			return;
		}
		setBusy(true);
		setError(undefined);
		try {
			const id = await spawnOrchestrator(session.workspaceId, "restore_dialog", true);
			onOpenChange(false);
			onRecreated(id);
		} catch (err) {
			setError(err instanceof Error ? err.message : t("restoreUnavailable.createFailed"));
		} finally {
			setBusy(false);
		}
	};

	return (
		<Dialog.Root open={open} onOpenChange={onOpenChange}>
			<Dialog.Portal>
				<Dialog.Overlay className="dialog-overlay data-[state=open]:animate-overlay-in" />
				<Dialog.Content
					className={`${settingsDialogContentClass} fixed left-1/2 top-1/2 w-dialog-md -translate-x-1/2 -translate-y-1/2 data-[state=open]:animate-modal-in`}
				>
					<button
						type="button"
						className="settings-dialog-close-button settings-close-button"
						aria-label={t("common.close")}
						disabled={busy}
						onClick={() => onOpenChange(false)}
					>
						<X className="size-5" aria-hidden="true" />
					</button>
					<div className={`${settingsDialogHeaderClass} border-b-0 pb-2 pr-14`}>
						<Dialog.Title className="settings-dialog-title">{t("restoreUnavailable.title")}</Dialog.Title>
						<Dialog.Description className="wd-body-s text-wd-secondary">
							{orchestrator ? t("restoreUnavailable.orchestratorBody") : t("restoreUnavailable.sessionBody")}
						</Dialog.Description>
					</div>
					{error ? (
						<div className={`${settingsDialogBodyClass} py-2`}>
							<p role="alert" className="wd-body-s text-critical">{error}</p>
						</div>
					) : null}
					<div className={`${settingsDialogFooterClass} border-t-0 pt-4`}>
						<Button type="button" variant="ghost" onClick={() => onOpenChange(false)} disabled={busy}>
							{orchestrator ? t("confirm.cancel") : t("restoreUnavailable.close")}
						</Button>
						{orchestrator ? (
							<Button
								type="button"
								variant="primary"
								onClick={recreate}
								disabled={busy || checkingProject}
							>
																{busy
									? t("restoreUnavailable.creating")
									: checkingProject
									? t("restoreUnavailable.checkingProject")
									: hasOrchestratorAgent
										? t("restoreUnavailable.createOrchestrator")
										: t("restoreUnavailable.configureOrchestrator")}
							</Button>
						) : null}
					</div>
				</Dialog.Content>
			</Dialog.Portal>
		</Dialog.Root>
	);
}
