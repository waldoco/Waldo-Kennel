import type { ReactNode } from "react";
import { cn } from "./utils";

export type TaskRowProps = {
	/** The row's identity — a WorkUnit title, a task name, whatever the host is listing. */
	title: string;
	/** A typed status chip (MissionStatusChip, SessionResponsibilityChip, or the host's own) — never bare color. */
	statusChip: ReactNode;
	/** e.g. "2h ago", "just now" — host-formatted, no i18n singleton here. */
	freshnessLabel?: string;
	/**
	 * Exactly one next-action slot. TaskRow renders whatever the host passes,
	 * but only ever in this one place — a row does not grow a second action
	 * area, so "the next thing to do" stays unambiguous.
	 */
	nextAction?: ReactNode;
	className?: string;
};

/** One row: identity, typed state, freshness, and a single next-action slot. */
export function TaskRow({ title, statusChip, freshnessLabel, nextAction, className }: TaskRowProps) {
	return (
		<div
			className={cn(
				"flex min-w-0 items-center gap-2.5 rounded-md hairline border-border bg-card px-3 py-2",
				className,
			)}
			data-testid="task-row"
		>
			<div className="flex min-w-0 flex-1 flex-col gap-0.5">
				<span className="truncate text-xs font-medium text-foreground" title={title}>
					{title}
				</span>
				<div className="flex min-w-0 flex-wrap items-center gap-1.5">
					{statusChip}
					{freshnessLabel ? <span className="shrink-0 text-2xs text-passive">{freshnessLabel}</span> : null}
				</div>
			</div>
			{nextAction ? (
				<div className="flex shrink-0 items-center" data-testid="task-row-next-action">
					{nextAction}
				</div>
			) : null}
		</div>
	);
}
