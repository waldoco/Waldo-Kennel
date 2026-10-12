import type { ReactNode } from "react";
import { ChevronIcon } from "./icons";
import { cn } from "./utils";

export type InspectorShellProps = {
	title: string;
	/** The summary layer — always visible, rendered first in DOM/focus order. */
	summary: ReactNode;
	/** The nested activity/session layer. Omit entirely when there's nothing to disclose. */
	activity?: ReactNode;
	activityLabel?: string;
	/**
	 * Disclosure state is injected by the host, not owned here — so a parent
	 * list re-render can't cause this shell to silently open or steal focus.
	 * When omitted, `activity` (if present) renders open and non-collapsible.
	 */
	activityOpen?: boolean;
	onActivityOpenChange?: (open: boolean) => void;
	/** Explicit action footer — never auto-focused. */
	footer?: ReactNode;
	className?: string;
};

/**
 * Structural inspector shell: a summary layer, an optional nested
 * activity/session layer, and an explicit footer. No data fetching, no
 * internal disclosure state, and nothing here ever calls `.focus()` — a
 * caller controls "open" for the activity layer if it wants it collapsible.
 */
export function InspectorShell({
	title,
	summary,
	activity,
	activityLabel,
	activityOpen,
	onActivityOpenChange,
	footer,
	className,
}: InspectorShellProps) {
	const collapsible = activity !== undefined && onActivityOpenChange !== undefined;
	const open = collapsible ? Boolean(activityOpen) : true;

	return (
		<section className={cn("flex flex-col gap-4 rounded-lg hairline border-wd-hairline bg-wd-surface p-4", className)} data-testid="inspector-shell">
			<h3 className="text-sm font-medium text-wd-primary">{title}</h3>

			<div data-testid="inspector-shell-summary">{summary}</div>

			{activity !== undefined ? (
				<div className="flex flex-col gap-2 hairline-t border-wd-hairline pt-3">
					{collapsible ? (
						<button
							aria-expanded={open}
							className="flex items-center gap-1.5 rounded-xs text-sm font-medium text-wd-secondary transition-colors duration-fast hover:text-wd-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)"
							onClick={() => onActivityOpenChange!(!open)}
							type="button"
						>
							<ChevronIcon className="size-icon-2xs" direction={open ? "down" : "right"} />
							{activityLabel ?? "Activity"}
						</button>
					) : activityLabel ? (
						<span className="text-sm font-medium text-wd-secondary">{activityLabel}</span>
					) : null}
					{open ? <div data-testid="inspector-shell-activity">{activity}</div> : null}
				</div>
			) : null}

			{footer ? (
				<div className="flex items-center justify-end gap-2 hairline-t border-wd-hairline pt-3" data-testid="inspector-shell-footer">
					{footer}
				</div>
			) : null}
		</section>
	);
}
