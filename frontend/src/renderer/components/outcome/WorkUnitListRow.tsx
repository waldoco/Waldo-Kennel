import { MissionStatusChip } from "@pin4sf/kennel-product-ui";
import { Ban, Circle, CircleAlert, CircleCheck, GitBranch, Loader2, MessageSquare, Pause, RotateCcw } from "lucide-react";
import { useTranslation } from "react-i18next";

import type { MessageKey } from "../../i18n/messages";
import { cn } from "../../lib/utils";
import type {
	MissionAttentionKind,
	MissionCanonStatus,
	MissionNodeView,
} from "./mission-presentation";

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

/**
 * One WorkUnit, rendered identically whether it comes from Canvas or List
 * (F2 invariant A/scope item 7 — the shared node face). The visible face
 * stays bounded: title, canon status, an optional attempt/session label, at
 * most one action, and criterion readiness. No raw IDs, provider/model,
 * evidence text, or dependency prose — a dependency *summary* exists only in
 * the accessible description, per the Playwright VoiceOver requirement.
 */
export type WorkUnitListRowProps = {
	node: MissionNodeView;
	selected: boolean;
	onSelect: () => void;
	onStart?: () => void;
};

export function WorkUnitListRow({ node, selected, onSelect, onStart }: WorkUnitListRowProps) {
	const { t } = useTranslation();
	const StatusIcon = STATUS_ICONS[node.status.status];
	const criteriaLabel =
		node.criteria.kind === "unavailable"
			? t("mission.criteria.unavailable" satisfies MessageKey)
			: t("mission.criteria.readyCount" satisfies MessageKey, { ready: node.criteria.ready, total: node.criteria.total });
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
			<MissionStatusChip
				className={node.status.className}
				detail={node.attemptLabel}
				icon={<StatusIcon aria-hidden="true" className={node.status.status === "executing" ? "animate-spin" : undefined} />}
				indicatorClassName={node.status.indicatorClassName}
				label={node.status.label}
			/>
			<span className="min-w-0 flex-1 truncate text-sm font-medium">{node.title}</span>
			{node.attention && (
				<span className={cn("inline-flex items-center gap-1 text-xs font-medium", node.attention.className)} data-testid="mission-row-attention">
					{(() => {
						const AttentionIcon = ATTENTION_ICONS[node.attention.kind];
						return <AttentionIcon aria-hidden="true" className="size-3.5" />;
					})()}
					{node.attention.label}
				</span>
			)}
			{node.dependencyUnavailable && (
				<span className="text-xs font-medium text-status-unknown" data-testid="mission-row-dependency-unavailable">
					{t("mission.dependency.unavailable" satisfies MessageKey)}
				</span>
			)}
			<span className="text-muted-foreground text-xs">{criteriaLabel}</span>
			{node.action && (
				<button
					className="inline-flex h-[26px] shrink-0 items-center justify-center rounded-md border border-border-strong bg-popover px-2.5 text-xs font-medium text-foreground transition-colors hover:bg-white/10"
					data-testid="mission-row-action"
					onClick={(event) => {
						event.stopPropagation();
						onStart?.();
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
