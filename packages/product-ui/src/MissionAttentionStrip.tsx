import type { ReactNode } from "react";
import { cn } from "./utils";

/**
 * The Mission-wide Needs You summary: approval, choice, and input are counted
 * separately (never collapsed into one "attention" total), because they are
 * different typed questions with different owner actions.
 */
export type MissionAttentionCount = {
	kind: string;
	label: string;
	count: number;
	className: string;
	indicatorClassName: string;
	icon: ReactNode;
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
		<div className="flex flex-wrap items-center gap-2" data-testid="mission-attention-strip">
			<span aria-live="polite" className="sr-only">
				{announcement ?? ""}
			</span>
			{visible.length === 0 ? null : (
				<>
					{visible.map((item) => (
						<span
							className={cn("inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium", item.className)}
							data-mission-attention-kind={item.kind}
							key={item.kind}
						>
							<span aria-hidden="true" className={cn("size-dot-sm shrink-0 rounded-full", item.indicatorClassName)} />
							<span aria-hidden="true" className="inline-flex size-3.5 shrink-0 items-center [&>svg]:size-3.5">
								{item.icon}
							</span>
							<span>
								{item.label} · {item.count}
							</span>
						</span>
					))}
				</>
			)}
		</div>
	);
}
