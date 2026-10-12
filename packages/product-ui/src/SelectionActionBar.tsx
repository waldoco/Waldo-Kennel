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
				"flex min-w-0 items-center gap-3 rounded-lg hairline border-wd-hairline bg-wd-surface px-4 py-2.5",
				className,
			)}
			data-testid="selection-action-bar"
		>
			<span className="min-w-0 flex-1 truncate text-sm font-medium text-wd-primary">{selectionLabel}</span>
			<div className="flex shrink-0 items-center gap-1.5">
				{actions.map((action) => (
					<button
						className={cn(
							"inline-flex h-8 items-center rounded-full px-3 text-sm font-medium transition-colors duration-fast motion-reduce:transition-none",
							"focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus)",
							"disabled:pointer-events-none disabled:bg-wd-inset disabled:text-wd-disabled",
							action.destructive
								? "bg-critical-bg text-critical hover:bg-[color-mix(in_oklab,var(--critical-bg),var(--critical)_10%)]"
								: "text-wd-primary hover:bg-wd-hover",
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
