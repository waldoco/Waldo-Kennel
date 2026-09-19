import { MissionStatusChip } from "@pin4sf/kennel-product-ui";
import { Ban, Circle, CircleAlert, CircleCheck, GitBranch, Loader2, MessageSquare, Pause, RotateCcw } from "lucide-react";
import { useTranslation } from "react-i18next";

import type { MessageKey } from "../../i18n/messages";
import { cn } from "../../lib/utils";
import type { MissionAttentionKind, MissionCanonStatus, MissionNodeView } from "./mission-presentation";

const STATUS_ICONS: Record<MissionCanonStatus, typeof Circle> = {
	blocked: Ban,
	runnable: Circle,
	executing: Loader2,
	proven: CircleCheck,
	retryable: RotateCcw,
	paused: Pause,
	unavailable: CircleAlert,
};

const ATTENTION_ICONS: Record<MissionAttentionKind, typeof MessageSquare> = {
	needs_approval: CircleCheck,
	needs_choice: GitBranch,
	needs_input: MessageSquare,
};

export type WorkUnitFaceProps = {
	/** The closed presentation view from mission-presentation — the face
	 *  renders view state only, never a raw DTO. */
	view: MissionNodeView;
	/**
	 * row  - the atoms only, no wrapper: the List row owns the container,
	 *        selection and the (sole) action. DOM-identical to the atoms the
	 *        row has always rendered.
	 * node - the canvas card: bounded face with a left status rail and a
	 *        two-line title. Read-only by design (no action, no toolbar);
	 *        the canvas wrapper owns selection, focus and handles.
	 */
	layout: "row" | "node";
	className?: string;
};

/**
 * The shared WorkUnit face (canvas build, slice C2): one bounded rendering of
 * title, canon status chip, optional attempt/session label, criterion
 * readiness and at most one attention marker - the same atoms in the List
 * row and the graph node, per the node-anatomy budget. No raw IDs,
 * provider/model, blocker prose, diff counts, or actions live here.
 */
export function WorkUnitFace({ view, layout, className }: WorkUnitFaceProps) {
	const { t } = useTranslation();
	const StatusIcon = STATUS_ICONS[view.status.status];
	// The row layout emits exactly the classes the List row has always
	// rendered - flex pressure in the shipped row is pinned by test, so any
	// nowrap/shrink hardening stays node-only.
	const nodeOnly = layout === "node";
	const criteriaLabel =
		view.criteria.kind === "unavailable"
			? t("mission.criteria.unavailable" satisfies MessageKey)
			: t("mission.criteria.readyCount" satisfies MessageKey, { ready: view.criteria.ready, total: view.criteria.total });

	const chip = (
		<MissionStatusChip
			className={nodeOnly ? cn("shrink-0 whitespace-nowrap", view.status.className) : view.status.className}
			detail={view.attemptLabel}
			icon={<StatusIcon aria-hidden="true" className={view.status.status === "executing" ? "animate-spin" : undefined} />}
			indicatorClassName={view.status.indicatorClassName}
			label={view.status.label}
		/>
	);
	const attention = view.attention && (
		<span
			className={
				nodeOnly
					? cn("inline-flex items-center gap-1 whitespace-nowrap text-xs font-medium", view.attention.className)
					: cn("inline-flex items-center gap-1 text-xs font-medium", view.attention.className)
			}
			data-testid="mission-row-attention"
		>
			{(() => {
				const AttentionIcon = ATTENTION_ICONS[view.attention.kind];
				return <AttentionIcon aria-hidden="true" className="size-3.5" />;
			})()}
			{view.attention.label}
		</span>
	);
	const dependencyUnavailable = view.dependencyUnavailable && (
		<span
			className={nodeOnly ? "whitespace-nowrap text-xs font-medium text-status-unknown" : "text-xs font-medium text-status-unknown"}
			data-testid="mission-row-dependency-unavailable"
		>
			{t("mission.dependency.unavailable" satisfies MessageKey)}
		</span>
	);
	const criteria = (
		<span className={nodeOnly ? "whitespace-nowrap text-muted-foreground text-xs" : "text-muted-foreground text-xs"}>{criteriaLabel}</span>
	);

	if (layout === "node") {
		return (
			<div
				className={cn("relative w-64 rounded-md hairline border-border bg-card py-2.5 pl-4 pr-3", className)}
				data-testid={`mission-node-face-${view.workUnitId}`}
			>
				<span
					aria-hidden="true"
					className={cn("absolute bottom-1.5 left-1 top-1.5 w-0.5 rounded-full", view.status.indicatorClassName)}
				/>
				<p className="line-clamp-2 text-sm font-medium leading-snug">{view.title}</p>
				<div className="mt-1.5 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
					{chip}
					{attention}
					{dependencyUnavailable}
					{criteria}
				</div>
			</div>
		);
	}

	// Row layout: atoms only - the List row's flex container, selection ring,
	// keyboard handling, action and assistive summary stay in the row.
	return (
		<>
			{chip}
			<span className="min-w-0 flex-1 truncate text-sm font-medium">{view.title}</span>
			{attention}
			{dependencyUnavailable}
			{criteria}
		</>
	);
}
