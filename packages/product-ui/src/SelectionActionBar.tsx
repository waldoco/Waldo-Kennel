import { cn } from "./utils";

export type SelectionAction = {
	id: string;
	label: string;
	onClick: () => void;
	disabled?: boolean;
	/** Visually and semantically distinct — never styled the same as a normal action. */
	destructive?: boolean;
};

export type SelectionActionBarProps = {
	/** Describes what's selected, e.g. "3 WorkUnits selected" — the caller owns selection state. */
	selectionLabel: string;
	actions: SelectionAction[];
	className?: string;
};

/**
 * Contextual actions for whatever the host has selected. This component owns
 * none of the selection state and performs no effect itself — every action
 * fires the host's own callback, nothing more.
 */
export function SelectionActionBar({ selectionLabel, actions, className }: SelectionActionBarProps) {
	if (actions.length === 0) return null;

	return (
		<div
			className={cn(
				"flex min-w-0 items-center gap-2 rounded-md hairline border-border bg-card px-3 py-2",
				className,
			)}
			data-testid="selection-action-bar"
		>
			<span className="min-w-0 flex-1 truncate text-2xs font-medium text-foreground">{selectionLabel}</span>
			<div className="flex shrink-0 items-center gap-1.5">
				{actions.map((action) => (
					<button
						className={cn(
							"rounded-md border px-2.5 py-1 text-2xs font-medium transition-colors motion-reduce:transition-none",
							"focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/70",
							"disabled:pointer-events-none disabled:opacity-50",
							action.destructive
								? "border-error/40 text-error hover:bg-error/10"
								: "border-border text-foreground hover:bg-interactive-hover",
						)}
						data-destructive={action.destructive || undefined}
						disabled={action.disabled}
						key={action.id}
						onClick={action.onClick}
						type="button"
					>
						{action.label}
					</button>
				))}
			</div>
		</div>
	);
}
