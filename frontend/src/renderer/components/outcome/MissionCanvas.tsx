import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
	Background,
	Controls,
	Handle,
	Position,
	ReactFlow,
	ReactFlowProvider,
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
import { buildCanvasLayers, canvasLineage, canvasTopologyNeighbor, type CanvasNavigationDirection } from "../../lib/mission-canvas-navigation";
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
	const { t } = useTranslation();
	const view = data.view;
	return (
		<>
			{/* Anchor points only - routing edges needs handle positions even
			    though nothing here is user-connectable (nodesConnectable=false). */}
			<Handle className="opacity-0" position={Position.Left} type="target" />
			{/* Hover/focus PREVIEW only: title, status, the concise attention
			    reason, and dependency counts. It is never the only path to these
			    facts - selection opens the same and more in the inspector - and
			    no action ever lives here. Reveal is CSS on the RF node wrapper
			    (the "group" class): hover for pointer, focus-within for keyboard,
			    instant under reduced motion. */}
			<div
				aria-hidden="true"
				className="pointer-events-none absolute bottom-full left-1/2 z-50 mb-2 w-64 -translate-x-1/2 rounded-md hairline border-border bg-card px-3 py-2 opacity-0 shadow-lg motion-safe:transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
				data-testid={`mission-node-preview-${view.workUnitId}`}
			>
				<p className="truncate text-sm font-medium text-foreground">{view.title}</p>
				<p className={cn("mt-0.5 text-xs", view.status.className)}>{view.status.label}</p>
				{view.attention ? (
					<p className="mt-0.5 line-clamp-2 text-muted-foreground text-xs">
						{view.attention.label}: {view.attention.summary}
					</p>
				) : null}
				<p className="mt-0.5 text-2xs text-passive">
					{t("mission.row.dependencySummary" satisfies MessageKey, {
						downstream: view.dependentCount,
						upstream: view.dependencyCount,
					})}
				</p>
			</div>
			<WorkUnitFace
				className={cn(selected && "ring-1 ring-ring", "focus-visible:outline-none")}
				layout="node"
				view={view}
			/>
			<Handle className="opacity-0" position={Position.Right} type="source" />
		</>
	);
});

const NODE_TYPES = { workunit: MissionCanvasFlowNode };
// Wide mission topologies must remain complete in the viewport. A 0.25 floor
// is bounded enough to avoid illegible extreme zoom while allowing the full
// graph to fit in the narrower pane beside the inspector.
const MISSION_CANVAS_MIN_ZOOM = 0.25;

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
	const [instanceReady, setInstanceReady] = useState(false);
	const fittedFrameRef = useRef<string | undefined>(undefined);
	const [viewportSize, setViewportSize] = useState("");

	// Selecting a node lights up its full dependency lineage - every ancestor
	// and every dependent over drawable edges - and dims the rest.
	const lineage = useMemo(() => canvasLineage(model?.edges ?? [], selectedWorkUnitId), [model, selectedWorkUnitId]);

	// Positioned nodes are derived synchronously from one model/layout pair.
	// React never commits a new layout key alongside the previous topology.
	const flowNodes = useMemo<Node<FlowNodeData>[]>(() => {
		if (!model || !graph || !layout || layout.key !== model.topologyKey) return [];
		return model.nodes.map((node) => ({
			ariaLabel: `${node.title} - ${graph.nodesByWorkUnitId.get(node.workUnitId)?.status.label ?? node.state}`,
			className: cn(
				"group motion-safe:transition-opacity focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background",
				lineage && !lineage.related.has(node.workUnitId) && "opacity-40",
			),
			data: { view: graph.nodesByWorkUnitId.get(node.workUnitId) as MissionNodeView },
			draggable: false,
			connectable: false,
			id: node.workUnitId,
			position: layout.positions.get(node.workUnitId) ?? { x: 0, y: 0 },
			selected: node.workUnitId === selectedWorkUnitId,
			type: "workunit" as const,
		}));
	}, [model, graph, layout, selectedWorkUnitId, lineage]);

	// React Flow first commits node IDs and only later measures their DOM bounds.
	// Fitting against the ID-only phase produces an unreadably small, off-centre
	// graph. Wait for every expected node to have usable measured bounds. The
	// framing key also includes viewport dimensions, so a real container resize
	// preserves the whole topology without refitting state-only refreshes. The
	// inspector overlays this stable frame and never changes its dimensions.
	useEffect(() => {
		if (!layout || !model || !instanceRef.current) return;
		if (layout.key !== model.topologyKey) return;
		const frameKey = `${layout.key}:${viewportSize}`;
		if (fittedFrameRef.current === frameKey) return;
		const expectedIds = model.nodes.map((node) => node.workUnitId).sort();
		let cancelled = false;
		let retryTimer: ReturnType<typeof setTimeout> | undefined;
		const fitMeasuredTopology = () => {
			if (cancelled || !instanceRef.current) return;
			const renderedIds = instanceRef.current.getNodes().map((node) => node.id).sort();
			const exactTopology = renderedIds.length === expectedIds.length && renderedIds.every((id, index) => id === expectedIds[index]);
			// In a controlled flow React Flow keeps DOM measurement in its
			// internal-node store. Dimension changes do not populate the user-facing
			// node objects unless the app applies them, and this read-only canvas has
			// no reason to duplicate that state. Read the owning store directly.
			const allMeasured = expectedIds.every((id) => {
				const measured = instanceRef.current?.getInternalNode(id)?.measured;
				return typeof measured?.width === "number" && measured.width > 0 && typeof measured.height === "number" && measured.height > 0;
			});
			if (!exactTopology || !allMeasured) {
				retryTimer = setTimeout(fitMeasuredTopology, 16);
				return;
			}
			instanceRef.current.fitView({ duration: reducedMotion ? 0 : 200, minZoom: MISSION_CANVAS_MIN_ZOOM, padding: 0.08 });
			fittedFrameRef.current = frameKey;
		};
		retryTimer = setTimeout(fitMeasuredTopology, 0);
		return () => {
			cancelled = true;
			if (retryTimer !== undefined) clearTimeout(retryTimer);
		};
	}, [flowNodes, instanceReady, layout, model, reducedMotion, viewportSize]);

	const flowEdges = useMemo<Edge[]>(() => {
		if (!model || !layout || layout.key !== model.topologyKey) return [];
		const nodeIds = new Set(model.nodes.map((node) => node.workUnitId));
		return model.edges
			.filter((edge) => nodeIds.has(edge.from) && nodeIds.has(edge.to))
			.map((edge) => {
				const id = `${edge.from}->${edge.to}`;
				const related = lineage?.relatedEdgeIds.has(id);
				return {
					className: lineage ? (related ? "mission-edge-related" : "mission-edge-dimmed") : undefined,
					focusable: false,
					id,
					selectable: false,
					source: edge.from,
					target: edge.to,
					type: "smoothstep",
				};
			});
	}, [model, layout, lineage]);

	// Arrow-key movement follows the drawn topology, never arbitrary DOM order.
	const canvasLayers = useMemo(
		() => (model && layout && layout.key === model.topologyKey ? buildCanvasLayers(layout.positions) : []),
		[model, layout],
	);
	const viewportRef = useRef<HTMLDivElement>(null);
	useEffect(() => {
		const viewport = viewportRef.current;
		if (!viewport || typeof ResizeObserver === "undefined") return;
		const observer = new ResizeObserver(([entry]) => {
			if (!entry) return;
			const nextSize = `${Math.round(entry.contentRect.width)}x${Math.round(entry.contentRect.height)}`;
			setViewportSize((current) => current === nextSize ? current : nextSize);
		});
		observer.observe(viewport);
		return () => observer.disconnect();
	}, []);
	const handleCanvasKeyDown = useCallback(
		(event: React.KeyboardEvent<HTMLDivElement>) => {
			const focusedId =
				document.activeElement instanceof Element
					? (document.activeElement.closest(".react-flow__node")?.getAttribute("data-id") ?? undefined)
					: undefined;
			if (event.key === "Escape") {
				if (selectedRef.current) setSelectedWorkUnitId(undefined);
				if ((focusedId || selectedRef.current) && document.activeElement instanceof HTMLElement) {
					document.activeElement.blur();
					viewportRef.current?.querySelector<HTMLElement>(".react-flow__pane")?.focus();
					event.preventDefault();
					event.stopPropagation();
				}
				return;
			}
			const direction: CanvasNavigationDirection | undefined =
				event.key === "ArrowLeft"
					? "left"
					: event.key === "ArrowRight"
						? "right"
						: event.key === "ArrowUp"
							? "up"
							: event.key === "ArrowDown"
								? "down"
								: undefined;
			if (direction) {
				if (!focusedId) return;
				const next = canvasTopologyNeighbor(canvasLayers, focusedId, direction);
				if (next) {
					viewportRef.current?.querySelector<HTMLElement>(`.react-flow__node[data-id="${next}"]`)?.focus();
					event.preventDefault();
					event.stopPropagation();
				}
				return;
			}
			if (event.key === "Enter" && focusedId) {
				setSelectedWorkUnitId(focusedId);
				event.preventDefault();
				event.stopPropagation();
			}
		},
		[canvasLayers],
	);

	const onNodesChange = useCallback((changes: NodeChange<Node<FlowNodeData>>[]) => {
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
			setInstanceReady(true);
			// onInit precedes reliable node measurement. The measurement-aware
			// effect above owns all automatic framing.
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

			<div className="relative flex min-h-0 flex-1">
				<div
					aria-hidden={selectedNode ? true : undefined}
					className="min-h-0 min-w-0 flex-1 rounded-group hairline border-border bg-card"
					inert={selectedNode ? true : undefined}
					data-testid="mission-canvas-viewport"
					onKeyDownCapture={handleCanvasKeyDown}
					ref={viewportRef}
				>
					{layoutReady ? (
						<ReactFlow
							aria-label={t("mission.list.heading" satisfies MessageKey)}
							colorMode={colorMode}
							edges={flowEdges}
							edgesFocusable={false}
							nodes={flowNodes}
							nodesConnectable={false}
							nodesDraggable={false}
							nodesFocusable
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
					<div
						className="absolute inset-0 z-overlay flex justify-end bg-background/45 backdrop-blur-[1px]"
						data-testid="mission-canvas-inspector-overlay"
						onKeyDown={(event) => {
							if (event.key !== "Escape") return;
							setSelectedWorkUnitId(undefined);
							if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
							viewportRef.current?.querySelector<HTMLElement>(".react-flow__pane")?.focus();
							event.preventDefault();
							event.stopPropagation();
						}}
					>
						<div className="h-full w-[min(22rem,calc(100%-1rem))] bg-card shadow-xl">
							<OutcomeInspector node={selectedNode} onClose={() => setSelectedWorkUnitId(undefined)} view={selectedView} />
						</div>
					</div>
				)}
			</div>
		</div>
	);
}
