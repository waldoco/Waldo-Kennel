import { useTranslation } from "react-i18next";

import type { MessageKey } from "../../i18n/messages";
import { cn } from "../../lib/utils";
import { WorkUnitFace } from "./WorkUnitFace";
import type { MissionNodeView } from "./mission-presentation";

/**
 * One WorkUnit, rendered identically whether it comes from Canvas or List
 * (F2 invariant A/scope item 7 — the shared node face). The bounded atoms
 * live in WorkUnitFace; this row owns the List container: selection,
 * keyboard activation, the (sole, projection-confirmed) action, and the
 * assistive dependency summary.
 */
export type WorkUnitListRowProps = {
	node: MissionNodeView;
	selected: boolean;
	onSelect: () => void;
	onStart?: () => void;
	/** True while the graph is frozen (disconnected, showing stale data) —
	 *  the action stays visible but is explicitly disabled, never a click
	 *  that silently does nothing (eligibility may have changed since the
	 *  last confirmed truth). */
	actionDisabled?: boolean;
};

export function WorkUnitListRow({ node, selected, onSelect, onStart, actionDisabled = false }: WorkUnitListRowProps) {
	const { t } = useTranslation();
	const dependencySummary = t("mission.row.dependencySummary" satisfies MessageKey, {
		upstream: node.dependencyCount,
		downstream: node.dependentCount,
	});

	return (
		<div
			aria-selected={selected}
			className={cn(
				"flex cursor-pointer items-center gap-3 rounded-md hairline border-border bg-card px-3 py-2.5 outline-none",
				selected && "ring-1 ring-ring",
			)}
			data-selected={selected || undefined}
			data-testid={`mission-list-row-${node.workUnitId}`}
			data-work-unit-id={node.workUnitId}
			id={`mission-row-${node.workUnitId}`}
			onClick={onSelect}
			onKeyDown={(event) => {
				if (event.key === "Enter" || event.key === " ") {
					event.preventDefault();
					onSelect();
				}
			}}
			role="option"
			tabIndex={selected ? 0 : -1}
		>
			<WorkUnitFace layout="row" view={node} />
			{node.action && (
				<button
					aria-disabled={actionDisabled || undefined}
					className={cn(
						"inline-flex h-[26px] shrink-0 items-center justify-center rounded-md border border-border-strong bg-popover px-2.5 text-xs font-medium text-foreground transition-colors hover:bg-white/10",
						actionDisabled && "cursor-not-allowed opacity-50 hover:bg-popover",
					)}
					data-testid="mission-row-action"
					disabled={actionDisabled}
					onClick={(event) => {
						event.stopPropagation();
						if (!actionDisabled) onStart?.();
					}}
					type="button"
				>
					{node.action.label}
				</button>
			)}
			<span className="sr-only">{dependencySummary}</span>
		</div>
	);
}
