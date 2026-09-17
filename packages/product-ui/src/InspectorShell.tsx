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
		<section className={cn("flex flex-col gap-3 rounded-md hairline border-border bg-card p-3", className)} data-testid="inspector-shell">
			<h3 className="text-xs font-semibold text-foreground">{title}</h3>

			<div data-testid="inspector-shell-summary">{summary}</div>

			{activity !== undefined ? (
				<div className="flex flex-col gap-1.5 border-t border-border/70 pt-2.5">
					{collapsible ? (
						<button
							aria-expanded={open}
							className="flex items-center gap-1.5 text-2xs font-medium text-muted-foreground hover:text-foreground"
							onClick={() => onActivityOpenChange!(!open)}
							type="button"
						>
							<ChevronIcon className="size-icon-2xs" direction={open ? "down" : "right"} />
							{activityLabel ?? "Activity"}
						</button>
					) : activityLabel ? (
						<span className="text-2xs font-medium uppercase tracking-wide text-passive">{activityLabel}</span>
					) : null}
					{open ? <div data-testid="inspector-shell-activity">{activity}</div> : null}
				</div>
			) : null}

			{footer ? (
				<div className="flex items-center justify-end gap-2 border-t border-border/70 pt-2.5" data-testid="inspector-shell-footer">
					{footer}
				</div>
			) : null}
		</section>
	);
}
