import type { ReactNode } from "react";
import { cn } from "../../lib/utils";

/**
 * Waldo ListGroup: an optional sentence-case `.wd-label` header, one rounded
 * surface of rows (`grouped`), and an optional footer that states the
 * consequence of the settings inside it.
 * `sectionId` (optional) marks the section for the renderer smoke suite as
 * [data-testid="settings-section"][data-section=<id>].
 */
export function SettingsSection({
	title,
	sectionId,
	titleHidden,
	grouped,
	footer,
	children,
}: {
	title: string;
	sectionId?: string;
	titleHidden?: boolean;
	grouped?: boolean;
	/** One sentence, `.wd-caption` tertiary, below the group. */
	footer?: ReactNode;
	children: ReactNode;
}) {
	return (
		<section
			className="flex w-full flex-col items-stretch gap-2"
			data-testid={sectionId ? "settings-section" : undefined}
			data-section={sectionId}
		>
			{!titleHidden && <h2 className="wd-label px-4 text-wd-secondary">{title}</h2>}
			<div className={cn("w-full", grouped ? "settings-grouped-rows flex w-full flex-col" : "flex w-full flex-col gap-2")}>
				{children}
			</div>
			{footer ? <p className="wd-caption px-4 text-wd-tertiary">{footer}</p> : null}
		</section>
	);
}
