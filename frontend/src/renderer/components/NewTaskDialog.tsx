import * as Dialog from "@radix-ui/react-dialog";
import { useTranslation } from "react-i18next";
import { TaskComposer } from "./TaskComposer";

type NewTaskDialogProps = {
	open: boolean;
	projectId?: string;
	initialPrompt?: string;
	onCreated: (sessionId: string) => void;
	onOpenChange: (open: boolean) => void;
};

export function NewTaskDialog({ open, projectId, initialPrompt, onCreated, onOpenChange }: NewTaskDialogProps) {
	const { t } = useTranslation();
	return (
		<Dialog.Root open={open} onOpenChange={onOpenChange}>
			<Dialog.Portal>
				<Dialog.Overlay className="dialog-overlay data-[state=open]:animate-overlay-in data-[state=closed]:animate-overlay-out" />
				<Dialog.Content className="fixed left-1/2 top-1/2 z-overlay w-dialog-xl -translate-x-1/2 -translate-y-1/2 overflow-hidden rounded-xl border-[length:var(--stroke-hairline)] border-wd-hairline bg-wd-raised p-0 text-wd-primary shadow-[var(--shadow-overlay)] data-[state=open]:animate-modal-in data-[state=closed]:animate-modal-out motion-reduce:animate-none">
					{/* One title line names the dialog, styled like every other settings-style
					    modal; everything else stays the composer's surface, no bordered header. */}
					<Dialog.Title className="settings-dialog-title px-6 pt-6">{t("newTask.title")}</Dialog.Title>
					<Dialog.Description className="sr-only">{t("newTask.description")}</Dialog.Description>
					<TaskComposer
						projectId={projectId}
						initialPrompt={initialPrompt}
						autoFocusTitle
						onCreated={(sessionId) => {
							onCreated(sessionId);
							onOpenChange(false);
						}}
					/>
				</Dialog.Content>
			</Dialog.Portal>
		</Dialog.Root>
	);
}
