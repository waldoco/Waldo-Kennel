import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
	Background,
	Controls,
	Handle,
	Position,
	ReactFlow,
	ReactFlowProvider,
	applyNodeChanges,
	type Edge,
	type Node,
	type NodeChange,
	type NodeProps,
	type ReactFlowInstance,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { useTranslation } from "react-i18next";

import type { MissionNodeRecord, useOutcomeMission } from "../../hooks/useOutcome";
import type { MessageKey } from "../../i18n/messages";
import { requestCanvasLayout, type CanvasPositions } from "../../lib/mission-canvas-layout";
import { modelFromMissionProjection } from "../../lib/mission-canvas-model";
import { cn } from "../../lib/utils";
import { useEventsConnection } from "../../hooks/useEventsConnection";
import { useUiStore } from "../../stores/ui-store";
import { Button } from "../ui/button";
import { OutcomeInspector } from "./OutcomeInspector";
import { WorkUnitFace } from "./WorkUnitFace";
import { missionDeterministicSuccessor, missionGraphView, missionTopologyIdentity, sameMissionTopology, type MissionNodeView } from "./mission-presentation";

/**
 * The production mission canvas: the daemon MissionProjection, laid out by
 * ELK in a worker (cached by the daemon's own topology fingerprint), rendered
 * through the shared WorkUnit face. Read-only by contract: no drag, no
 * connect, no actions - selection is the only interaction, and it opens the
 * one shared OutcomeInspector, never a canvas-specific detail system.
 *
 * Stability invariants (tested): a state-only refresh never recomputes
 * layout, never refits the viewport, and never drops selection; only a
 * topology-fingerprint swap relayouts and refits. A layout that resolves
 * after a newer topology arrived is discarded as stale.
 */

type FlowNodeData = { view: MissionNodeView };

const MissionCanvasFlowNode = memo(function MissionCanvasFlowNode({ data, selected }: NodeProps<Node<FlowNodeData>>) {
	return (
		<>
			{/* Anchor points only - routing edges needs handle positions even
			    though nothing here is user-connectable (nodesConnectable=false). */}
			<Handle className="opacity-0" position={Position.Left} type="target" />
			<WorkUnitFace
				className={cn(selected && "ring-1 ring-ring", "focus-visible:outline-none")}
				layout="node"
				view={data.view}
			/>
			<Handle className="opacity-0" position={Position.Right} type="source" />
		</>
	);
});

const NODE_TYPES = { workunit: MissionCanvasFlowNode };

function usePrefersReducedMotion(): boolean {
	const [reduced, setReduced] = useState(
		() => typeof window !== "undefined" && Boolean(window.matchMedia?.("(prefers-reduced-motion: reduce)").matches),
	);
	useEffect(() => {
		if (typeof window === "undefined" || !window.matchMedia) return;
		const query = window.matchMedia("(prefers-reduced-motion: reduce)");
		const listener = () => setReduced(query.matches);
		query.addEventListener("change", listener);
		return () => query.removeEventListener("change", listener);
	}, []);
	return reduced;
}

export type MissionCanvasProps = {
	/** The result of `useOutcomeMission`, passed in rather than called here so
	 *  the caller controls when a Plan id is authorized/known. */
	missionQuery: ReturnType<typeof useOutcomeMission>;
	planApproved: boolean;
	planWorkUnits?: readonly { id: string; title: string; dependsOn: readonly string[] }[];
	/** Test-only hook: exposes the live ReactFlowInstance so a test can spy on
	 *  `fitView`. Never used by production callers. */
	onInstanceReady?: (instance: ReactFlowInstance<Node<FlowNodeData>, Edge>) => void;
};

export function MissionCanvas(props: MissionCanvasProps) {
	return (
		<ReactFlowProvider>
			<MissionCanvasInner {...props} />
		</ReactFlowProvider>
	);
}

function MissionCanvasInner({ missionQuery, planApproved, planWorkUnits, onInstanceReady }: MissionCanvasProps) {
	const { t } = useTranslation();
	const connection = useEventsConnection();
	const [selectedWorkUnitId, setSelectedWorkUnitId] = useState<string | undefined>();
	const selectedRef = useRef(selectedWorkUnitId);
	selectedRef.current = selectedWorkUnitId;

	const lastConfirmedRef = useRef<ReturnType<typeof useOutcomeMission>["mission"] | undefined>(undefined);
	const previousIdentityRef = useRef<ReturnType<typeof missionTopologyIdentity> | undefined>(undefined);

	const mission = missionQuery.mission;
	if (mission) lastConfirmedRef.current = mission;
	const confirmedMission = lastConfirmedRef.current;

	// Same freeze contract as the List: while the SSE stream is down, the last
	// confirmed projection stays on screen and the banner says so - the canvas
	// never silently shows data it can no longer trust to refresh.
	const frozen = connection === "disconnected" && Boolean(confirmedMission);
	const displayMission = confirmedMission;
	const refreshFailed = !frozen && Boolean(missionQuery.failure) && Boolean(confirmedMission);

	const model = useMemo(() => (displayMission ? modelFromMissionProjection(displayMission) : undefined), [displayMission]);
	const graph = useMemo(() => (displayMission ? missionGraphView(displayMission, t) : undefined), [displayMission, t]);

	// Selection survives a state-only refresh; a topology swap that removes
	// the selected node moves selection to the deterministic successor - the
	// same contract the List implements.
	useEffect(() => {
		if (!displayMission) return;
		const identity = missionTopologyIdentity(displayMission);
		const previousIdentity = previousIdentityRef.current;
		if (previousIdentity && !sameMissionTopology(previousIdentity, identity)) {
			const nodeIds = new Set(displayMission.nodes.map((node) => node.workUnitId));
			const selected = selectedRef.current;
			if (selected && !nodeIds.has(selected)) {
				setSelectedWorkUnitId(missionDeterministicSuccessor(displayMission.nodes));
			}
		}
		previousIdentityRef.current = identity;
	}, [displayMission]);

	// Layout: cached by the daemon's topology fingerprint. A state-only
	// refresh keeps the same key and never reaches requestCanvasLayout's
	// miss path; a topology swap cancels the stale request via the sequence
	// guard inside the layout module.
	const [layout, setLayout] = useState<{ key: string; positions: CanvasPositions } | undefined>();
	const [layoutFailure, setLayoutFailure] = useState<string | undefined>();
	// Explicit layout-reset signal for the layout-error retry: bumping it
	// re-runs the layout effect for the CURRENT model, so retrying never
	// depends on a refetch or on object identities changing.
	const [layoutRetrySeq, setLayoutRetrySeq] = useState(0);
	useEffect(() => {
		if (!model) return;
		let alive = true;
		setLayoutFailure(undefined);
		requestCanvasLayout(model)
			.then((result) => {
				if (alive && result.status === "ready") {
					setLayout({ key: model.topologyKey, positions: result.positions });
				}
			})
			.catch((error: unknown) => {
				// Layout failed on both the worker and the main-thread retry: say
				// so, with a retry, rather than hanging on the pending reading.
				if (alive) setLayoutFailure(error instanceof Error ? error.message : String(error));
			});
		return () => {
			alive = false;
		};
	}, [model, layoutRetrySeq]);

	const reducedMotion = usePrefersReducedMotion();
	const instanceRef = useRef<ReactFlowInstance<Node<FlowNodeData>, Edge> | null>(null);
	const fittedKeyRef = useRef<string | undefined>(undefined);

	// Refit exactly when a NEW topology's layout lands - never on a state-only
	// refresh (the key is unchanged and the guard holds).
	useEffect(() => {
		if (!layout || !instanceRef.current) return;
		if (fittedKeyRef.current === layout.key) return;
		fittedKeyRef.current = layout.key;
		instanceRef.current.fitView({ duration: reducedMotion ? 0 : 200 });
	}, [layout, reducedMotion]);

	const [flowNodes, setFlowNodes] = useState<Node<FlowNodeData>[]>([]);
	useEffect(() => {
		if (!model || !graph || !layout || layout.key !== model.topologyKey) return;
		setFlowNodes((current) => {
			const measured = new Map(current.map((node) => [node.id, node.measured]));
			return model.nodes.map((node) => ({
				ariaLabel: `${node.title} - ${graph.nodesByWorkUnitId.get(node.workUnitId)?.status.label ?? node.state}`,
				data: { view: graph.nodesByWorkUnitId.get(node.workUnitId) as MissionNodeView },
				draggable: false,
				connectable: false,
				id: node.workUnitId,
				measured: measured.get(node.workUnitId),
				position: layout.positions.get(node.workUnitId) ?? { x: 0, y: 0 },
				selected: node.workUnitId === selectedWorkUnitId,
				type: "workunit" as const,
			}));
		});
	}, [model, graph, layout, selectedWorkUnitId]);

	const flowEdges = useMemo<Edge[]>(() => {
		if (!model || !layout || layout.key !== model.topologyKey) return [];
		return model.edges.map((edge) => ({
			focusable: false,
			id: `${edge.from}->${edge.to}`,
			selectable: false,
			source: edge.from,
			target: edge.to,
			type: "smoothstep",
		}));
	}, [model, layout]);

	const onNodesChange = useCallback((changes: NodeChange<Node<FlowNodeData>>[]) => {
		setFlowNodes((current) => applyNodeChanges(changes, current));
		for (const change of changes) {
			if (change.type === "select") {
				setSelectedWorkUnitId((selectedNow) => {
					if (change.selected) return change.id;
					return selectedNow === change.id ? undefined : selectedNow;
				});
			}
		}
	}, []);

	const handleInit = useCallback(
		(instance: ReactFlowInstance<Node<FlowNodeData>, Edge>) => {
			instanceRef.current = instance;
			onInstanceReady?.(instance);
		},
		[onInstanceReady],
	);

	// Bind to the store's RESOLVED theme, never the preference: under "system"
	// an OS flip updates resolvedTheme and only that rerenders this component.
	const colorMode = useUiStore((state) => state.resolvedTheme);

	const selectedNode: MissionNodeRecord | undefined = displayMission?.nodes.find((node) => node.workUnitId === selectedWorkUnitId);
	const selectedView = selectedNode && graph?.nodesByWorkUnitId.get(selectedNode.workUnitId);

	// --- Honest loading/error/empty/pending states, mirroring the List's contract ---

	if (missionQuery.failure && !displayMission) {
		return (
			<div className="rounded-group hairline border-warning/40 bg-warning/5 px-4.5 py-3.5" data-testid="mission-canvas-error">
				<h3 className="text-sm font-medium">{t("mission.list.error.title" satisfies MessageKey)}</h3>
				<p className="mt-1 text-muted-foreground text-sm">{missionQuery.failure.message}</p>
				<Button className="mt-2" data-testid="mission-canvas-retry" onClick={missionQuery.refetch} size="sm" variant="outline">
					{t("mission.list.retry" satisfies MessageKey)}
				</Button>
			</div>
		);
	}

	if (!planApproved) return null;

	if (!displayMission && missionQuery.isLoading) {
		if (planWorkUnits && planWorkUnits.length > 0) {
			return (
				<div className="rounded-group hairline border-border bg-card px-4.5 py-3.5" data-testid="mission-canvas-plan-pending">
					<h3 className="text-sm font-medium">{t("mission.list.planPending.title" satisfies MessageKey)}</h3>
					<p className="mt-1 text-muted-foreground text-sm">{t("mission.list.planPending.body" satisfies MessageKey)}</p>
				</div>
			);
		}
		return (
			<p className="text-muted-foreground text-sm" data-testid="mission-canvas-loading">
				{t("mission.list.loading" satisfies MessageKey)}
			</p>
		);
	}

	if (!displayMission || !model || !graph) return null;

	if (model.nodes.length === 0) {
		return (
			<div className="rounded-group hairline border-border bg-card px-4.5 py-3.5" data-testid="mission-canvas-empty">
				<h3 className="text-sm font-medium">{t("mission.list.empty.title" satisfies MessageKey)}</h3>
				<p className="mt-1 text-muted-foreground text-sm">{t("mission.list.empty.body" satisfies MessageKey)}</p>
			</div>
		);
	}

	const layoutReady = layout && layout.key === model.topologyKey;

	return (
		<div className="flex min-h-0 flex-1 flex-col gap-2.5" data-testid="mission-canvas">
			{frozen && (
				<div className="flex items-center justify-between gap-2 rounded-md hairline border-warning/40 bg-warning/5 px-3 py-2" data-testid="mission-canvas-stale-banner">
					<p className="text-muted-foreground text-xs">
						{t("mission.list.stale.banner" satisfies MessageKey, { time: displayMission.updatedAt })}
					</p>
					<Button data-testid="mission-canvas-refresh" onClick={missionQuery.refetch} size="sm" variant="outline">
						{t("mission.list.stale.refresh" satisfies MessageKey)}
					</Button>
				</div>
			)}

			{refreshFailed && (
				<div className="flex items-center justify-between gap-2 rounded-md hairline border-warning/40 bg-warning/5 px-3 py-2" data-testid="mission-canvas-refresh-failed-banner">
					<p className="text-muted-foreground text-xs">
						{t("mission.list.refreshFailed.banner" satisfies MessageKey, { message: missionQuery.failure?.message ?? "" })}
					</p>
					<Button data-testid="mission-canvas-refresh-failed-retry" onClick={missionQuery.refetch} size="sm" variant="outline">
						{t("mission.list.retry" satisfies MessageKey)}
					</Button>
				</div>
			)}

			{displayMission.noRunnableReason && (
				<p className="text-2xs leading-body text-passive" data-testid="mission-graph-no-runnable">
					{t(`outcome.missionGraph.noRunnable.${displayMission.noRunnableReason}` as MessageKey)}
				</p>
			)}

			<div className="flex min-h-0 flex-1 gap-3">
				<div className="min-h-0 min-w-0 flex-1 rounded-group hairline border-border bg-card" data-testid="mission-canvas-viewport">
					{layoutReady ? (
						<ReactFlow
							aria-label={t("mission.list.heading" satisfies MessageKey)}
							colorMode={colorMode}
							edges={flowEdges}
							edgesFocusable={false}
							fitView
							fitViewOptions={{ duration: reducedMotion ? 0 : 200 }}
							nodes={flowNodes}
							nodesConnectable={false}
							nodesDraggable={false}
							nodeTypes={NODE_TYPES}
							onInit={handleInit}
							onNodesChange={onNodesChange}
						>
							<Background gap={24} />
							<Controls showInteractive={false} />
						</ReactFlow>
					) : layoutFailure ? (
						<div className="px-4.5 py-3.5" data-testid="mission-canvas-layout-error">
							<p className="text-muted-foreground text-sm">{layoutFailure}</p>
							<Button
								className="mt-2"
								data-testid="mission-canvas-layout-retry"
								onClick={() => {
									setLayout(undefined);
									setLayoutFailure(undefined);
									setLayoutRetrySeq((seq) => seq + 1);
								}}
								size="sm"
								variant="outline"
							>
								{t("mission.list.retry" satisfies MessageKey)}
							</Button>
						</div>
					) : (
						<p className="px-4.5 py-3.5 text-muted-foreground text-sm" data-testid="mission-canvas-layout-pending">
							{t("mission.list.loading" satisfies MessageKey)}
						</p>
					)}
				</div>

				{selectedNode && selectedView && (
					<div className="w-[22rem] shrink-0">
						<OutcomeInspector node={selectedNode} onClose={() => setSelectedWorkUnitId(undefined)} view={selectedView} />
					</div>
				)}
			</div>
		</div>
	);
}
