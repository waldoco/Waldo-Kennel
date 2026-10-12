import type { ReactElement } from "react";
import { useTranslation } from "react-i18next";
import type { WorkspaceSession } from "../types/workspace";
import { Button } from "./ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "./ui/popover";

// Waldo ConfirmSheet contract in a popover: the title names the object, the
// body names what stops and what is kept, the destructive button names the
// effect, and the quiet way out comes last.
export function SessionTerminationPopover({
	onConfirm,
	onOpenChange,
	open,
	session,
	trigger,
}: {
	onConfirm: () => void;
	onOpenChange: (open: boolean) => void;
	open: boolean;
	session?: WorkspaceSession;
	trigger: ReactElement;
}) {
	const { t } = useTranslation();
	const title = session?.title;
	return (
		<Popover onOpenChange={onOpenChange} open={open}>
			<PopoverTrigger asChild>{trigger}</PopoverTrigger>
			<PopoverContent
				align="end"
				aria-label={title ? t("termination.dialogNamed", { title }) : t("termination.dialog")}
				className="flex w-72 max-w-[calc(100vw-1rem)] flex-col gap-3 rounded-lg p-4 shadow-(--shadow-overlay)"
				collisionPadding={8}
				onClick={(event) => event.stopPropagation()}
				role="dialog"
				side="bottom"
				sideOffset={6}
			>
				<p className="wd-headline break-words text-wd-primary">
					{title ? t("termination.dialogNamed", { title }) : t("termination.dialog")}
				</p>
				<dl className="flex flex-col gap-2">
					<div className="flex flex-col gap-0.5">
						<dt className="wd-caption text-wd-secondary">{t("termination.stopsLabel")}</dt>
						<dd className="wd-body-s text-wd-primary">{t("termination.stops")}</dd>
					</div>
					<div className="flex flex-col gap-0.5">
						<dt className="wd-caption text-wd-secondary">{t("termination.keptLabel")}</dt>
						<dd className="wd-body-s text-wd-primary">{t("termination.body")}</dd>
					</div>
				</dl>
				<div className="flex flex-col gap-1.5">
					<Button
						aria-label={t("termination.confirmAria")}
						className="w-full"
						onClick={onConfirm}
						size="sm"
						type="button"
						variant="critical"
					>
						{t("shell.terminateSession")}
					</Button>
					<Button className="w-full" onClick={() => onOpenChange(false)} size="sm" type="button" variant="ghost">
						{t("termination.keep")}
					</Button>
				</div>
			</PopoverContent>
		</Popover>
	);
}
