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
			// Segmented control: an inset pill track with one ink thumb on the
			// selected reading.
			className={cn("inline-flex h-9 shrink-0 items-center gap-1 rounded-full bg-wd-inset p-1", disabled && "opacity-40")}
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
				"inline-flex h-full items-center justify-center whitespace-nowrap rounded-full px-3 text-sm font-medium transition-[background-color,color] duration-fast focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--border-focus) motion-reduce:transition-none",
				disabled && "cursor-not-allowed",
				active ? "bg-wd-inverse text-wd-on-inverse" : "text-wd-secondary hover:text-wd-primary",
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
