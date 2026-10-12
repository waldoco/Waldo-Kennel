import { StateLabel } from "@pin4sf/kennel-product-ui";
import { useTranslation } from "react-i18next";

import type { MessageKey } from "../../i18n/messages";
import { cn } from "../../lib/utils";
import type { MissionNodeView } from "./mission-presentation";

export type WorkUnitFaceProps = {
	/** The closed presentation view from mission-presentation — the face
	 *  renders view state only, never a raw DTO. */
	view: MissionNodeView;
	/**
	 * row  - the atoms only, no wrapper: the List row owns the container,
	 *        selection and the (sole) action. DOM-identical to the atoms the
	 *        row has always rendered.
	 * node - the canvas card: a small hairline card with a two-line title
	 *        and one line of facts. Read-only by design (no action, no
	 *        toolbar); the canvas wrapper owns selection, focus and handles.
	 */
	layout: "row" | "node";
	className?: string;
};

/**
 * The shared WorkUnit face (canvas build, slice C2): one bounded rendering of
 * title, typed state (written inline with its mark), optional attempt label,
 * criterion readiness and at most one Needs-You marker - the same atoms in
 * the List row and the graph node, per the node-anatomy budget. No raw IDs,
 * provider/model, blocker prose, diff counts, or actions live here. Nothing
 * here animates: a running unit is a still blue dot and the word Running.
 */
export function WorkUnitFace({ view, layout, className }: WorkUnitFaceProps) {
	const { t } = useTranslation();
	// Node-only nowrap/shrink hardening: the List row's flex pressure is pinned
	// by test, so it never leaks into the row atoms.
	const nodeOnly = layout === "node";
	const criteriaLabel =
		view.criteria.kind === "unavailable"
			? t("mission.criteria.unavailable" satisfies MessageKey)
			: t("mission.criteria.readyCount" satisfies MessageKey, { ready: view.criteria.ready, total: view.criteria.total });

	const state = (
		<span
			className={cn("inline-flex min-w-0 items-center gap-1.5 text-xs font-medium", nodeOnly && "shrink-0 whitespace-nowrap")}
			data-mission-status={view.status.status}
			data-testid="mission-status"
		>
			<StateLabel label={view.status.label} tone={view.status.tone} />
			{view.attemptLabel ? <span className="font-normal tabular-nums text-wd-tertiary">{view.attemptLabel}</span> : null}
		</span>
	);
	// Every typed question (approval, choice, input) means the owner has the
	// next move, so each is the Needs-you mark with its own word.
	const attention = view.attention && (
		<span
			className={cn("inline-flex items-center text-xs font-medium", nodeOnly && "whitespace-nowrap")}
			data-mission-attention-kind={view.attention.kind}
			data-testid="mission-row-attention"
		>
			<StateLabel label={view.attention.label} tone={view.attention.tone} />
		</span>
	);
	const dependencyUnavailable = view.dependencyUnavailable && (
		<span
			className={cn("text-xs text-wd-secondary", nodeOnly && "whitespace-nowrap")}
			data-testid="mission-row-dependency-unavailable"
		>
			{t("mission.dependency.unavailable" satisfies MessageKey)}
		</span>
	);
	const criteria = (
		<span className={cn("text-xs tabular-nums text-wd-tertiary", nodeOnly && "whitespace-nowrap")}>{criteriaLabel}</span>
	);

	if (layout === "node") {
		return (
			<div
				className={cn(
					"w-64 rounded-md hairline border-wd-hairline bg-wd-surface px-3 py-2.5 text-wd-primary transition-colors duration-fast motion-reduce:transition-none",
					className,
				)}
				data-testid={`mission-node-face-${view.workUnitId}`}
			>
				<p className="line-clamp-2 text-sm font-medium leading-snug text-wd-primary">{view.title}</p>
				<div className="mt-1.5 flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-1">
					{state}
					{attention}
					{dependencyUnavailable}
					{criteria}
				</div>
			</div>
		);
	}

	// Row layout: atoms only - the List row's flex container, selection,
	// keyboard handling, action and assistive summary stay in the row.
	return (
		<>
			{state}
			<span className="min-w-0 flex-1 truncate text-sm font-medium text-wd-primary">{view.title}</span>
			{attention}
			{dependencyUnavailable}
			{criteria}
		</>
	);
}
