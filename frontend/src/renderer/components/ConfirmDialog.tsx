import { X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { Button } from "./ui/button";
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogTitle, settingsDialogContentClass } from "./ui/dialog";

type ConfirmDialogProps = {
	open: boolean;
	/** Names the object as a question ("Remove kennel-web?"), never "Are you sure?". */
	title: string;
	/** States the consequence: what stops, what is kept. */
	description: React.ReactNode;
	/** Spells out the effect in the title's words ("Remove project"), never "Confirm". */
	confirmLabel: string;
	/** The quiet way out, named for the status quo where it reads well ("Keep project"). */
	cancelLabel?: string;
	destructive?: boolean;
	busy?: boolean;
	error?: string | null;
	onConfirm: () => void;
	onOpenChange: (open: boolean) => void;
};

// Waldo ConfirmSheet as a centred dialog: raised plane, radius-xl, space-6
// padding, a .wd-title question, the consequence in text-secondary, then the
// quiet way out and the action spelled out — `critical` when it destroys.
export function ConfirmDialog({
	open,
	title,
	description,
	confirmLabel,
	cancelLabel,
	destructive,
	busy,
	error,
	onConfirm,
	onOpenChange,
}: ConfirmDialogProps) {
	const { t } = useTranslation();
	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent
				showCloseButton={false}
				className={cn(settingsDialogContentClass, "w-[min(var(--size-dialog-lg),calc(100vw-var(--space-8)))] gap-4 p-6")}
			>
				<DialogClose asChild>
					<button
						type="button"
						disabled={busy}
						className="settings-dialog-close-button settings-close-button disabled:pointer-events-none disabled:text-wd-disabled"
						aria-label={t("confirm.close")}
						title={t("confirm.closeEsc")}
					>
						<X className="size-5" aria-hidden="true" />
					</button>
				</DialogClose>

				<div className="flex flex-col gap-2 pr-8">
					<DialogTitle className="text-wd-primary">{title}</DialogTitle>
					<DialogDescription asChild>
						<div className="text-sm text-wd-secondary">{description}</div>
					</DialogDescription>
				</div>

				{error ? (
					<p role="alert" className="text-sm text-critical">
						{error}
					</p>
				) : null}

				<div className="flex flex-wrap items-center justify-end gap-2 pt-2">
					<DialogClose asChild>
						<Button type="button" variant="ghost" disabled={busy}>
							{cancelLabel ?? t("confirm.cancel")}
						</Button>
					</DialogClose>
					<Button
						type="button"
						variant={destructive ? "critical" : "primary"}
						disabled={busy}
						onClick={onConfirm}
					>
						{confirmLabel}
					</Button>
				</div>
			</DialogContent>
		</Dialog>
	);
}
