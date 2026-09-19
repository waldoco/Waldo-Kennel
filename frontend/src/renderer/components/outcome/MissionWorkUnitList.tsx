import { MissionAttentionStrip, type MissionAttentionCount } from "@pin4sf/kennel-product-ui";
import { CircleCheck, GitBranch, MessageSquare } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { useEventsConnection } from "../../hooks/useEventsConnection";
import type { MissionNodeRecord, MissionRecord, useOutcomeMission } from "../../hooks/useOutcome";
import type { MessageKey } from "../../i18n/messages";
import { Button } from "../ui/button";
import { OutcomeInspector } from "./OutcomeInspector";
import { WorkUnitListRow } from "./WorkUnitListRow";
import {
	missionDeterministicSuccessor,
	missionGraphView,
	missionTopologyIdentity,
	sameMissionTopology,
	type MissionAttentionKind,
	type MissionTopologyIdentity,
} from "./mission-presentation";

const ATTENTION_ICONS: Record<MissionAttentionKind, typeof MessageSquare> = {
	needs_approval: CircleCheck,
	needs_choice: GitBranch,
	needs_input: MessageSquare,
};

export type MissionWorkUnitListProps = {
	/** The result of `useOutcomeMission`, passed in rather than called here so
	 *  the caller controls when a Plan id is authorized/known. */
	missionQuery: ReturnType<typeof useOutcomeMission>;
	/** True once an authorized Plan is known — governs the neutral
	 *  Plan-only-pending reading while Mission has not resolved yet. */
	planApproved: boolean;
	planWorkUnits?: readonly { id: string; title: string; dependsOn: readonly string[] }[];
	initialWorkUnitId?: string;
	onStart?: (workUnitId: string) => void;
	onOpenSession?: (sessionId: string) => void;
};

/**
 * The Mission-projection-driven WorkUnit List (F2). Canvas is not built in
 * this slice — `@xyflow/react` is not an approved/installed dependency, and
 * the packet's own rule is to ship the proven List rather than a half-correct
 * hand-rolled graph — so List is the sole reading here, always available,
 * never a fallback of something else.
 */
export function MissionWorkUnitList({ missionQuery, planApproved, planWorkUnits, initialWorkUnitId, onStart, onOpenSession }: MissionWorkUnitListProps) {
	const { t } = useTranslation();
	const connection = useEventsConnection();
	const [selectedWorkUnitId, setSelectedWorkUnitId] = useState<string | undefined>(initialWorkUnitId);
	useEffect(() => { if (initialWorkUnitId) setSelectedWorkUnitId(initialWorkUnitId); }, [initialWorkUnitId]);
	const [announcement, setAnnouncement] = useState("");

	const selectedRef = useRef(selectedWorkUnitId);
	selectedRef.current = selectedWorkUnitId;

	const lastConfirmedRef = useRef<MissionRecord | undefined>(undefined);
	const previousIdentityRef = useRef<MissionTopologyIdentity | undefined>(undefined);
	const previousNodeIdsRef = useRef<ReadonlySet<string>>(new Set());

	const mission = missionQuery.mission;
	if (mission) lastConfirmedRef.current = mission;
	const confirmedMission = lastConfirmedRef.current;

	// Disconnect freezes the last confirmed graph — item 9/invariant H — for
	// as long as the SSE stream is down, regardless of whether React Query
	// still happens to hold the same (or any) cached `mission` value: once
	// disconnected we can no longer trust that further CDC-driven refetches
	// will reach us, so the display and every eligibility-sensitive action
	// freeze to the last thing we actually confirmed. A transient failure
	// with no prior confirmed graph at all still surfaces as the honest
	// error state below, not a fabricated freeze.
	const frozen = connection === "disconnected" && Boolean(confirmedMission);
	const displayMission = confirmedMission;
	// Connected (or unknown) but the last refresh attempt over already-shown
	// data failed — a different condition from `frozen`, and never silent:
	// the retained graph stays visible, but the failure is surfaced too.
	const refreshFailed = !frozen && Boolean(missionQuery.failure) && Boolean(confirmedMission);

	const graph = useMemo(() => (displayMission ? missionGraphView(displayMission, t) : undefined), [displayMission, t]);

	useEffect(() => {
		if (!displayMission) return;
		const identity = missionTopologyIdentity(displayMission);
		const nodeIds = new Set(displayMission.nodes.map((node) => node.workUnitId));
		const previousIdentity = previousIdentityRef.current;
		if (previousIdentity && !sameMissionTopology(previousIdentity, identity)) {
			const previousIds = previousNodeIdsRef.current;
			const added = [...nodeIds].filter((id) => !previousIds.has(id)).length;
			const removed = [...previousIds].filter((id) => !nodeIds.has(id)).length;
			if (added || removed) {
				setAnnouncement(t("mission.list.topologyChanged" satisfies MessageKey, { added, removed }));
			}
			const selected = selectedRef.current;
			if (selected && !nodeIds.has(selected)) {
				setSelectedWorkUnitId(missionDeterministicSuccessor(displayMission.nodes));
			}
		}
		previousIdentityRef.current = identity;
		previousNodeIdsRef.current = nodeIds;
		// A state-only change (identity unchanged) intentionally does nothing
		// here beyond updating the refs — selection, and everything the rows
		// render from `graph`, simply reflects the new node data in place.
	}, [displayMission, t]);

	const attentionCounts: MissionAttentionCount[] = useMemo(() => {
		if (!graph) return [];
		const counts: Record<MissionAttentionKind, number> = { needs_approval: 0, needs_choice: 0, needs_input: 0 };
		for (const node of graph.nodes) {
			if (node.attention) counts[node.attention.kind] += 1;
		}
		return (Object.keys(counts) as MissionAttentionKind[]).map((kind) => ({
			kind,
			label: t(
				(kind === "needs_approval"
					? "mission.attention.needsApproval"
					: kind === "needs_choice"
						? "mission.attention.needsChoice"
						: "mission.attention.needsInput") satisfies MessageKey,
			),
			count: counts[kind],
			className: kind === "needs_input" ? "text-status-in-review border-status-in-review/40" : "text-status-needs-you border-status-needs-you/40",
			indicatorClassName: kind === "needs_input" ? "bg-status-in-review" : "bg-status-needs-you",
			icon: (() => {
				const Icon = ATTENTION_ICONS[kind];
				return <Icon aria-hidden="true" />;
			})(),
		}));
	}, [graph, t]);

	const selectedNode: MissionNodeRecord | undefined = displayMission?.nodes.find(
		(node) => node.workUnitId === selectedWorkUnitId,
	);
	const selectedView = selectedNode && graph?.nodesByWorkUnitId.get(selectedNode.workUnitId);

	function selectRow(workUnitId: string) {
		setSelectedWorkUnitId((current) => (current === workUnitId ? current : workUnitId));
	}

	function handleRowKeyNavigation(event: React.KeyboardEvent<HTMLDivElement>) {
		if (!graph || (event.key !== "ArrowDown" && event.key !== "ArrowUp")) return;
		event.preventDefault();
		const ids = graph.nodes.map((node) => node.workUnitId);
		const currentIndex = selectedWorkUnitId ? ids.indexOf(selectedWorkUnitId) : -1;
		const nextIndex =
			event.key === "ArrowDown"
				? Math.min(ids.length - 1, currentIndex + 1)
				: Math.max(0, currentIndex === -1 ? 0 : currentIndex - 1);
		const nextId = ids[nextIndex];
		if (nextId) selectRow(nextId);
	}

	// --- Honest loading/error/empty/pending states (item 10) ---

	if (missionQuery.failure && !displayMission) {
		return (
			<div
				className="rounded-group hairline border-warning/40 bg-warning/5 px-4.5 py-3.5"
				data-testid="mission-list-error"
			>
				<h3 className="text-sm font-medium">{t("mission.list.error.title" satisfies MessageKey)}</h3>
				<p className="mt-1 text-muted-foreground text-sm">{missionQuery.failure.message}</p>
				<Button className="mt-2" data-testid="mission-list-retry" onClick={missionQuery.refetch} size="sm" variant="outline">
					{t("mission.list.retry" satisfies MessageKey)}
				</Button>
			</div>
		);
	}

	if (!planApproved) {
		return null;
	}

	if (!displayMission && missionQuery.isLoading) {
		if (planWorkUnits && planWorkUnits.length > 0) {
			return (
				<div className="rounded-group hairline border-border bg-card px-4.5 py-3.5" data-testid="mission-list-plan-pending">
					<h3 className="text-sm font-medium">{t("mission.list.planPending.title" satisfies MessageKey)}</h3>
					<p className="mt-1 text-muted-foreground text-sm">{t("mission.list.planPending.body" satisfies MessageKey)}</p>
				</div>
			);
		}
		return (
			<p className="text-muted-foreground text-sm" data-testid="mission-list-loading">
				{t("mission.list.loading" satisfies MessageKey)}
			</p>
		);
	}

	if (!displayMission || !graph) return null;

	if (graph.nodes.length === 0) {
		return (
			<div className="rounded-group hairline border-border bg-card px-4.5 py-3.5" data-testid="mission-list-empty">
				<h3 className="text-sm font-medium">{t("mission.list.empty.title" satisfies MessageKey)}</h3>
				<p className="mt-1 text-muted-foreground text-sm">{t("mission.list.empty.body" satisfies MessageKey)}</p>
			</div>
		);
	}

	return (
		<div className="flex min-h-0 flex-1 flex-col gap-2.5" data-testid="mission-work-unit-list">
			<span aria-live="polite" className="sr-only">
				{announcement}
			</span>

			{frozen && (
				<div className="flex items-center justify-between gap-2 rounded-md hairline border-warning/40 bg-warning/5 px-3 py-2" data-testid="mission-list-stale-banner">
					<p className="text-muted-foreground text-xs">
						{t("mission.list.stale.banner" satisfies MessageKey, { time: displayMission.updatedAt })}
					</p>
					<Button data-testid="mission-list-refresh" onClick={missionQuery.refetch} size="sm" variant="outline">
						{t("mission.list.stale.refresh" satisfies MessageKey)}
					</Button>
				</div>
			)}

			{refreshFailed && (
				<div className="flex items-center justify-between gap-2 rounded-md hairline border-warning/40 bg-warning/5 px-3 py-2" data-testid="mission-list-refresh-failed-banner">
					<p className="text-muted-foreground text-xs">
						{t("mission.list.refreshFailed.banner" satisfies MessageKey, { message: missionQuery.failure?.message ?? "" })}
					</p>
					<Button data-testid="mission-list-refresh-failed-retry" onClick={missionQuery.refetch} size="sm" variant="outline">
						{t("mission.list.retry" satisfies MessageKey)}
					</Button>
				</div>
			)}

			<MissionAttentionStrip items={attentionCounts} />

			<div className="flex min-h-0 flex-1 gap-3">
				<div
					aria-label={t("mission.list.heading" satisfies MessageKey)}
					className="flex min-h-0 flex-1 flex-col gap-1.5 overflow-y-auto"
					data-testid="mission-list-listbox"
					onKeyDown={handleRowKeyNavigation}
					role="listbox"
				>
					{graph.nodes.map((node) => (
						<WorkUnitListRow
							actionDisabled={frozen}
							key={node.workUnitId}
							node={node}
							onSelect={() => selectRow(node.workUnitId)}
							onStart={node.action ? () => onStart?.(node.workUnitId) : undefined}
							selected={node.workUnitId === selectedWorkUnitId}
						/>
					))}
				</div>

				{selectedNode && selectedView && (
					<div className="w-[22rem] shrink-0">
						<OutcomeInspector node={selectedNode} onClose={() => setSelectedWorkUnitId(undefined)} onOpenSession={onOpenSession} view={selectedView} />
					</div>
				)}
			</div>
		</div>
	);
}
