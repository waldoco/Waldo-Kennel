import type { ReactNode } from "react";
import { cn } from "./utils";

export type ActivityChipProps = {
	/** e.g. "3 checks passed", "2 unresolved comments" — secondary context, never a primary status claim. */
	label: string;
	icon?: ReactNode;
	className?: string;
};

/**
 * A quiet, secondary signal (activity or evidence count) — deliberately less
 * prominent than MissionStatusChip so a reader can't mistake "3 checks" for
 * "this is done". No tone prop: activity chips are informational, not a
 * status claim, so they always render in the same muted register.
 */
export function ActivityChip({ label, icon, className }: ActivityChipProps) {
	return (
		<span
			className={cn(
				"inline-flex max-w-full items-center gap-1 text-2xs text-passive",
				className,
			)}
			data-testid="activity-chip"
		>
			{icon ? (
				<span aria-hidden="true" className="flex shrink-0 [&_svg]:size-icon-2xs">
					{icon}
				</span>
			) : null}
			<span className="truncate">{label}</span>
		</span>
	);
}
