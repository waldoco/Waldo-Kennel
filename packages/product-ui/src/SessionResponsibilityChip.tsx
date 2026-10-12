import type { ReactNode } from "react";
import { cn } from "./utils";

/**
 * Who owns the next move: the agent is working it, the owner must decide, or
 * responsibility is shared/handed off. Distinct from MissionStatusChip's
 * tone vocabulary because "who" and "what state" are different questions —
 * an owner-responsible item can be any status tone underneath it.
 */
export type SessionResponsibility = "agent" | "owner" | "shared";

const RESPONSIBILITY_CLASS: Record<SessionResponsibility, string> = {
	agent: "bg-wd-inset text-wd-primary",
	owner: "bg-spot-subtle text-spot-text",
	shared: "bg-wd-inset text-wd-secondary",
};

export type SessionResponsibilityChipProps = {
	responsibility: SessionResponsibility;
	/** Always rendered as visible text alongside the icon. */
	label: string;
	icon: ReactNode;
	className?: string;
};

/** A typed "who owns the next move" chip: icon + text together, never icon alone. */
export function SessionResponsibilityChip({ responsibility, label, icon, className }: SessionResponsibilityChipProps) {
	return (
		<span
			className={cn(
				"inline-flex h-5 max-w-full items-center gap-1.5 rounded-xs px-1.5 text-xs font-medium leading-none",
				RESPONSIBILITY_CLASS[responsibility],
				className,
			)}
			data-responsibility={responsibility}
			data-testid="session-responsibility-chip"
		>
			<span aria-hidden="true" className="flex shrink-0 [&_svg]:size-icon-2xs">
				{icon}
			</span>
			<span className="truncate">{label}</span>
		</span>
	);
}
