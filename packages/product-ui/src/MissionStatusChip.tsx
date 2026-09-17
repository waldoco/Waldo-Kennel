import type { ReactNode } from "react";
import { cn } from "./utils";

/**
 * A single node/row's status reading: an icon and label always travel
 * together with color (F2 invariant J — status is never conveyed by color
 * alone). Every field here arrives pre-resolved by the caller's closed
 * presentation adapter; this component makes no decision about what a raw
 * status string means.
 */
export type MissionStatusChipProps = {
	label: string;
	className: string;
	indicatorClassName: string;
	icon: ReactNode;
	/** Small supplementary text, e.g. an attempt/session label. Never raw IDs. */
	detail?: string;
};

export function MissionStatusChip({ label, className, indicatorClassName, icon, detail }: MissionStatusChipProps) {
	return (
		<span className={cn("inline-flex items-center gap-1.5 text-xs font-medium", className)}>
			<span aria-hidden="true" className={cn("size-dot-sm shrink-0 rounded-full", indicatorClassName)} />
			<span aria-hidden="true" className="inline-flex size-3.5 shrink-0 items-center [&>svg]:size-3.5">
				{icon}
			</span>
			<span>{label}</span>
			{detail && <span className="text-muted-foreground font-normal">{detail}</span>}
		</span>
	);
}
