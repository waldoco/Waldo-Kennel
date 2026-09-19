import { cn } from "../../lib/utils";
import type { MissionViewMode } from "../../stores/ui-store";

export type MissionViewSwitchLabels = {
	ariaLabel: string;
	graph: string;
	list: string;
};

/**
 * The visible List/Graph reading switch for the Mission surface, mirroring
 * the Sessions board/list switch (same tablist semantics and chrome) so the
 * two switches read as one product pattern. Switching views never re-sorts,
 * re-fetches, or re-authorizes anything - it only re-renders the same
 * projection through the other face.
 */
export function MissionViewSwitch({
	disabled = false,
	labels,
	onChange,
	value,
}: {
	disabled?: boolean;
	labels: MissionViewSwitchLabels;
	onChange: (mode: MissionViewMode) => void;
	value: MissionViewMode;
}) {
	return (
		<div
			aria-disabled={disabled || undefined}
			aria-label={labels.ariaLabel}
			className={cn("inline-flex h-control-segment shrink-0 items-center rounded-lg bg-shell", disabled && "opacity-50")}
			data-testid="mission-view-switch"
			role="tablist"
		>
			<MissionViewSwitchItem active={value === "list"} disabled={disabled} label={labels.list} mode="list" onSelect={onChange} />
			<MissionViewSwitchItem active={value === "graph"} disabled={disabled} label={labels.graph} mode="graph" onSelect={onChange} />
		</div>
	);
}

function MissionViewSwitchItem({
	active,
	disabled,
	label,
	mode,
	onSelect,
}: {
	active: boolean;
	disabled: boolean;
	label: string;
	mode: MissionViewMode;
	onSelect: (mode: MissionViewMode) => void;
}) {
	return (
		<button
			aria-selected={active}
			className={cn(
				"inline-flex h-full items-center justify-center rounded-lg px-3 text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/60",
				disabled && "cursor-not-allowed",
				active ? "hairline border-border bg-card font-medium text-foreground" : "text-passive hover:text-foreground",
			)}
			data-testid={`mission-view-${mode}`}
			disabled={disabled}
			onClick={() => {
				if (!disabled) onSelect(mode);
			}}
			role="tab"
			type="button"
		>
			{label}
		</button>
	);
}
