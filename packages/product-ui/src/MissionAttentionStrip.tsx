import type { ReactNode } from "react";
import { StateMark } from "./StateBadge";

/**
 * The Mission-wide Needs You summary: approval, choice, and input are counted
 * separately (never collapsed into one "attention" total), because they are
 * different typed questions with different owner actions.
 *
 * Each kind reads as a quiet inline state — the Needs you dot, the kind in
 * primary text, the count in tabular figures — never a coloured pill.
 */
export type MissionAttentionCount = {
	kind: string;
	label: string;
	count: number;
	/** Legacy tone classes from older hosts; the strip now carries the
	 *  needs-you tone itself and ignores them. */
	className?: string;
	indicatorClassName?: string;
	icon?: ReactNode;
};

export type MissionAttentionStripProps = {
	items: MissionAttentionCount[];
	/** Announced once per render when the visible counts change (topology
	 *  swap or CDC refetch) — a polite live region, never a toast that steals
	 *  focus from an owner mid-type. */
	announcement?: string;
};

export function MissionAttentionStrip({ items, announcement }: MissionAttentionStripProps) {
	const visible = items.filter((item) => item.count > 0);
	return (
		<div className="flex flex-wrap items-center gap-x-4 gap-y-1" data-testid="mission-attention-strip">
			<span aria-live="polite" className="sr-only">
				{announcement ?? ""}
			</span>
			{visible.map((item) => (
				<span
					className="inline-flex min-w-0 items-center gap-1.5 text-sm text-wd-primary"
					data-mission-attention-kind={item.kind}
					data-state-tone="needsYou"
					key={item.kind}
				>
					<StateMark tone="needsYou" />
					<span className="truncate">{item.label}</span>
					<span className="tabular-nums text-wd-tertiary">{item.count}</span>
				</span>
			))}
		</div>
	);
}
