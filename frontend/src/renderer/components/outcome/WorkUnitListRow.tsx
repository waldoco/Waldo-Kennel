import { useTranslation } from "react-i18next";

import type { MessageKey } from "../../i18n/messages";
import { cn } from "../../lib/utils";
import { Button } from "../ui/button";
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
				// One row of a hairline-separated list: selection is the quiet
				// selected fill, never a coloured ring.
				"flex cursor-pointer items-center gap-3 hairline-b border-wd-hairline px-3 py-2.5 transition-colors duration-fast last:border-b-0 hover:bg-wd-hover focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-(--border-focus) motion-reduce:transition-none",
				selected && "bg-wd-selected hover:bg-wd-selected",
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
				<Button
					aria-disabled={actionDisabled || undefined}
					className="shrink-0"
					data-testid="mission-row-action"
					disabled={actionDisabled}
					onClick={(event) => {
						event.stopPropagation();
						if (!actionDisabled) onStart?.();
					}}
					size="sm"
					type="button"
					variant="primary"
				>
					{node.action.label}
				</Button>
			)}
			<span className="sr-only">{dependencySummary}</span>
		</div>
	);
}
